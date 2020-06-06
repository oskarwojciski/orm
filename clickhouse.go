package orm

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

const counterClickHouseAll = "clickhouse.all"
const counterClickHouseQuery = "clickhouse.query"
const counterClickHouseExec = "clickhouse.query"

type ClickHouseConfig struct {
	url  string
	code string
	db   *sqlx.DB
}

type ClickHouse struct {
	engine *Engine
	client *sqlx.DB
	code   string
}

func (c *ClickHouse) Exec(query string, args ...interface{}) sql.Result {
	start := time.Now()
	rows, err := c.client.Exec(query, args...)
	if c.engine.queryLoggers[QueryLoggerSourceClickHouse] != nil {
		c.fillLogFields("[ORM][CLICKHOUSE][EXEC]", start, "exec", query, args, err)
	}
	c.engine.dataDog.incrementCounter(counterClickHouseAll, 1)
	c.engine.dataDog.incrementCounter(counterClickHouseExec, 1)
	if err != nil {
		panic(convertToError(err))
	}
	return rows
}

func (c *ClickHouse) Queryx(query string, args ...interface{}) (rows *sqlx.Rows, deferF func()) {
	start := time.Now()
	rows, err := c.client.Queryx(query, args...)
	if c.engine.queryLoggers[QueryLoggerSourceClickHouse] != nil {
		c.fillLogFields("[ORM][CLICKHOUSE][SELECT]", start, "select", query, args, err)
	}
	c.engine.dataDog.incrementCounter(counterClickHouseAll, 1)
	c.engine.dataDog.incrementCounter(counterClickHouseQuery, 1)
	if err != nil {
		panic(err)
	}
	return rows, func() {
		if rows != nil {
			_ = rows.Close()
		}
	}
}

func (c *ClickHouse) fillLogFields(message string, start time.Time, typeCode string, query string, args []interface{}, err error) {
	now := time.Now()
	stop := time.Since(start).Microseconds()
	e := c.engine.queryLoggers[QueryLoggerSourceClickHouse].log.
		WithField("pool", c.code).
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