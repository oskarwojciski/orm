package orm

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type Alter struct {
	Sql  string
	Safe bool
	Pool string
}

func GetAlters() (alters []Alter, err error) {

	tablesInDB := make(map[string]map[string]bool)
	tablesInEntities := make(map[string]map[string]bool)

	for _, pool := range sqlClients {
		poolName := pool.code
		tablesInDB[poolName] = make(map[string]bool)
		tables, err := getAllTables(GetMysql(poolName).db)
		if err != nil {
			return nil, err
		}
		for _, table := range tables {
			tablesInDB[poolName][table] = true
		}
		tablesInEntities[poolName] = make(map[string]bool)
	}
	alters = make([]Alter, 0)
	for _, t := range entities {
		tableSchema := getTableSchema(t)
		tablesInEntities[tableSchema.MysqlPoolName][tableSchema.TableName] = true
		has, newAlters, err := tableSchema.GetSchemaChanges()
		if err != nil {
			return nil, err
		}
		if !has {
			continue
		}
		alters = append(alters, newAlters...)
	}

	for poolName, tables := range tablesInDB {
		for tableName := range tables {
			_, has := tablesInEntities[poolName][tableName]
			if !has {
				dropSql := fmt.Sprintf("DROP TABLE IF EXISTS `%s`.`%s`;", GetMysql(poolName).databaseName, tableName)
				isEmpty, err := isTableEmptyInPool(poolName, tableName)
				if err != nil {
					return nil, err
				}
				if isEmpty {
					alters = append(alters, Alter{Sql: dropSql, Safe: true, Pool: poolName})
				} else {
					alters = append(alters, Alter{Sql: dropSql, Safe: false, Pool: poolName})
				}
			}
		}
	}
	sort.Slice(alters, func(i, j int) bool {
		leftSql := alters[i].Sql
		rightSql := alters[j].Sql

		leftHasDropForeignKey := strings.Index(leftSql, "DROP FOREIGN KEY") > 0
		rightHasDropForeignKey := strings.Index(rightSql, "DROP FOREIGN KEY") > 0
		if leftHasDropForeignKey && !rightHasDropForeignKey {
			return true
		}
		leftHasAddForeignKey := strings.Index(leftSql, "ADD CONSTRAINT") > 0
		rightHasAddForeignKey := strings.Index(rightSql, "ADD CONSTRAINT") > 0
		if !leftHasAddForeignKey && rightHasAddForeignKey {
			return true
		}
		return false

	})
	return
}

func isTableEmptyInPool(poolName string, tableName string) (bool, error) {
	pool := GetMysql(poolName)
	return isTableEmpty(pool.db, tableName)
}

func getAllTables(db *sql.DB) ([]string, error) {
	tables := make([]string, 0)
	results, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	for results.Next() {
		var row string
		err = results.Scan(&row)
		if err != nil {
			return nil, err
		}
		tables = append(tables, row)
	}
	return tables, nil
}

func isTableEmpty(db *sql.DB, tableName string) (bool, error) {
	var lastId uint64
	err := db.QueryRow(fmt.Sprintf("SELECT `Id` FROM `%s` LIMIT 1", tableName)).Scan(&lastId)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
