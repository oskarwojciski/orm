package orm

import (
	"database/sql"
	"time"

	"github.com/juju/errors"
)

const counterDBAll = "db.all"
const counterDBTransaction = "db.transaction"
const counterDBQuery = "db.query"
const counterDBExec = "db.exec"

type DBConfig struct {
	dataSourceName string
	code           string
	databaseName   string
	db             *sql.DB
	autoincrement  uint64
}

type ExecResult interface {
	LastInsertId() uint64
	RowsAffected() uint64
}

type execResult struct {
	r sql.Result
}

func (e *execResult) LastInsertId() uint64 {
	id, err := e.r.LastInsertId()
	if err != nil {
		panic(err)
	}
	return uint64(id)
}

func (e *execResult) RowsAffected() uint64 {
	id, err := e.r.RowsAffected()
	if err != nil {
		panic(err)
	}
	return uint64(id)
}

type sqlClient interface {
	Begin() error
	Commit() error
	Rollback() (bool, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) SQLRow
	Query(query string, args ...interface{}) (SQLRows, error)
}

type dbClientQuery interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

type dbClient interface {
	dbClientQuery
	Begin() (*sql.Tx, error)
}

type dbClientTX interface {
	dbClientQuery
	Commit() error
	Rollback() error
}

type standardSQLClient struct {
	db dbClient
	tx dbClientTX
}

func (db *standardSQLClient) Begin() error {
	if db.tx != nil {
		return errors.Errorf("transaction already started")
	}
	tx, err := db.db.Begin()
	if err != nil {
		return errors.Trace(err)
	}
	db.tx = tx
	return nil
}

func (db *standardSQLClient) Commit() error {
	if db.tx == nil {
		return errors.Errorf("transaction not started")
	}
	err := db.tx.Commit()
	if err != nil {
		return errors.Trace(err)
	}
	db.tx = nil
	return nil
}

func (db *standardSQLClient) Rollback() (bool, error) {
	if db.tx == nil {
		return false, nil
	}
	err := db.tx.Rollback()
	if err != nil {
		return true, errors.Trace(err)
	}
	db.tx = nil
	return true, nil
}

func (db *standardSQLClient) Exec(query string, args ...interface{}) (sql.Result, error) {
	if db.tx != nil {
		res, err := db.tx.Exec(query, args...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return res, nil
	}
	res, err := db.db.Exec(query, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return res, nil
}

func (db *standardSQLClient) QueryRow(query string, args ...interface{}) SQLRow {
	if db.tx != nil {
		return db.tx.QueryRow(query, args...)
	}
	return db.db.QueryRow(query, args...)
}

func (db *standardSQLClient) Query(query string, args ...interface{}) (SQLRows, error) {
	if db.tx != nil {
		rows, err := db.tx.Query(query, args...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return rows, nil
	}
	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rows, nil
}

type SQLRows interface {
	Next() bool
	Err() error
	Close() error
	Scan(dest ...interface{}) error
	Columns() ([]string, error)
}

type Rows interface {
	Next() bool
	Scan(dest ...interface{})
	Columns() []string
}

type rowsStruct struct {
	sqlRows SQLRows
}

func (r *rowsStruct) Next() bool {
	return r.sqlRows.Next()
}

func (r *rowsStruct) Columns() []string {
	columns, err := r.sqlRows.Columns()
	if err != nil {
		panic(err)
	}
	return columns
}

func (r *rowsStruct) Scan(dest ...interface{}) {
	err := r.sqlRows.Scan(dest...)
	if err != nil {
		panic(err)
	}
}

type SQLRow interface {
	Scan(dest ...interface{}) error
}

type DB struct {
	engine        *Engine
	client        sqlClient
	code          string
	databaseName  string
	autoincrement uint64
}

func (db *DB) GetDatabaseName() string {
	return db.databaseName
}

func (db *DB) GetPoolCode() string {
	return db.code
}

func (db *DB) Begin() {
	start := time.Now()
	err := db.client.Begin()
	if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
		db.fillLogFields("[ORM][MYSQL][BEGIN]", start, "transaction", "START TRANSACTION", nil, err)
		db.engine.dataDog.incrementCounter(counterDBAll, 1)
		db.engine.dataDog.incrementCounter(counterDBTransaction, 1)
	}
	if err != nil {
		panic(err)
	}
}

func (db *DB) Commit() {
	start := time.Now()
	err := db.client.Commit()
	if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
		db.fillLogFields("[ORM][MYSQL][COMMIT]", start, "transaction", "COMMIT", nil, err)
	}
	db.engine.dataDog.incrementCounter(counterDBAll, 1)
	db.engine.dataDog.incrementCounter(counterDBTransaction, 1)
	if err != nil {
		panic(err)
	}
	if db.engine.afterCommitLocalCacheSets != nil {
		for cacheCode, pairs := range db.engine.afterCommitLocalCacheSets {
			cache := db.engine.GetLocalCache(cacheCode)
			cache.MSet(pairs...)
		}
	}
	db.engine.afterCommitLocalCacheSets = nil
	if db.engine.afterCommitRedisCacheDeletes != nil {
		for cacheCode, keys := range db.engine.afterCommitRedisCacheDeletes {
			cache := db.engine.GetRedis(cacheCode)
			cache.Del(keys...)
		}
	}
	db.engine.afterCommitRedisCacheDeletes = nil
}

func (db *DB) Rollback() {
	start := time.Now()
	has, err := db.client.Rollback()
	if has {
		if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
			db.fillLogFields("[ORM][MYSQL][ROLLBACK]", start, "transaction", "ROLLBACK", nil, err)
		}
		db.engine.dataDog.incrementCounter(counterDBAll, 1)
		db.engine.dataDog.incrementCounter(counterDBTransaction, 1)
	}
	if err != nil {
		panic(errors.Trace(err))
	}
	db.engine.afterCommitLocalCacheSets = nil
	db.engine.afterCommitRedisCacheDeletes = nil
}

func (db *DB) Exec(query string, args ...interface{}) ExecResult {
	start := time.Now()
	rows, err := db.client.Exec(query, args...)
	if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
		db.fillLogFields("[ORM][MYSQL][EXEC]", start, "exec", query, args, err)
	}
	db.engine.dataDog.incrementCounter(counterDBAll, 1)
	db.engine.dataDog.incrementCounter(counterDBExec, 1)
	if err != nil {
		panic(convertToError(err))
	}
	return &execResult{r: rows}
}

func (db *DB) QueryRow(query *Where, toFill ...interface{}) (found bool) {
	start := time.Now()
	row := db.client.QueryRow(query.String(), query.GetParameters()...)

	db.engine.dataDog.incrementCounter(counterDBAll, 1)
	db.engine.dataDog.incrementCounter(counterDBQuery, 1)
	err := row.Scan(toFill...)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
				db.fillLogFields("[ORM][MYSQL][SELECT]", start, "select", query.String(), query.GetParameters(), nil)
			}
			return false
		}
		if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
			db.fillLogFields("[ORM][MYSQL][SELECT]", start, "select", query.String(), query.GetParameters(), err)
		}
		panic(err)
	}
	if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
		db.fillLogFields("[ORM][MYSQL][SELECT]", start, "select", query.String(), query.GetParameters(), nil)
	}
	return true
}

func (db *DB) Query(query string, args ...interface{}) (rows Rows, deferF func()) {
	start := time.Now()
	result, err := db.client.Query(query, args...)
	if db.engine.queryLoggers[QueryLoggerSourceDB] != nil {
		db.fillLogFields("[ORM][MYSQL][SELECT]", start, "select", query, args, err)
	}
	db.engine.dataDog.incrementCounter(counterDBAll, 1)
	db.engine.dataDog.incrementCounter(counterDBQuery, 1)
	if err != nil {
		panic(err)
	}
	return &rowsStruct{result}, func() {
		if result != nil {
			err := result.Err()
			if err != nil {
				panic(err)
			}
			err = result.Close()
			if err != nil {
				panic(err)
			}
		}
	}
}

func (db *DB) fillLogFields(message string, start time.Time, typeCode string, query string, args []interface{}, err error) {
	now := time.Now()
	stop := time.Since(start).Microseconds()
	e := db.engine.queryLoggers[QueryLoggerSourceDB].log.
		WithField("pool", db.code).
		WithField("db", db.databaseName).
		WithField("Query", query).
		WithField("microseconds", stop).
		WithField("target", "mysql").
		WithField("type", typeCode).
		WithField("started", start.UnixNano()).
		WithField("finished", now.UnixNano())
	if args != nil {
		e = e.WithField("args", args)
	}
	if err != nil {
		injectLogError(err, e).Error(message)
	} else {
		e.Info(message)
	}
}
