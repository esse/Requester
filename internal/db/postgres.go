package db

import (
	"database/sql"

	_ "github.com/lib/pq"
)

func newPostgresSnapshotter(connString string, tables []string) (Snapshotter, error) {
	db, err := sql.Open(DriverPostgres, connString)
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
		dbType:           DBTypePostgres,
	}, nil
}
