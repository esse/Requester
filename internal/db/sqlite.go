package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func newSQLiteSnapshotter(connString string, tables []string) (Snapshotter, error) {
	db, err := sql.Open("sqlite3", connString)
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
		dbType:           "sqlite",
	}, nil
}
