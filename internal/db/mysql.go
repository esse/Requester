package db

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

func newMySQLSnapshotter(connString string, tables, namespaces []string) (Snapshotter, error) {
	db, err := sql.Open(DriverMySQL, connString)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &baseSnapshotter{
		db:               db,
		configuredTables: tables,
		namespaces:       namespaces,
		dbType:           DBTypeMySQL,
	}, nil
}
