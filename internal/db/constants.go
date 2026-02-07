package db

// Supported database type identifiers used in configuration and switch statements.
const (
	DBTypePostgres = "postgres"
	DBTypeMySQL    = "mysql"
	DBTypeSQLite   = "sqlite"
)

// SQL driver names used with database/sql.Open().
const (
	DriverPostgres = "postgres"
	DriverMySQL    = "mysql"
	DriverSQLite   = "sqlite3"
)

// Table discovery queries for each database type.
const (
	PostgresDiscoverTablesQuery = "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"
	MySQLDiscoverTablesQuery    = "SHOW TABLES"
	SQLiteDiscoverTablesQuery   = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"
)
