package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Snapshotter captures and restores database state.
type Snapshotter interface {
	// Tables returns the list of tables to snapshot.
	Tables() ([]string, error)
	// SnapshotTable reads all rows from a table.
	SnapshotTable(table string) ([]map[string]any, error)
	// SnapshotAll reads all configured tables.
	SnapshotAll() (map[string][]map[string]any, error)
	// RestoreTable truncates a table and inserts the given rows.
	RestoreTable(table string, rows []map[string]any) error
	// RestoreAll restores all tables from the given state.
	RestoreAll(state map[string][]map[string]any) error
	// Close closes the database connection.
	Close() error
}

// NewSnapshotter creates a Snapshotter for the given database type.
func NewSnapshotter(dbType, connString string, tables []string) (Snapshotter, error) {
	switch dbType {
	case "postgres":
		return newPostgresSnapshotter(connString, tables)
	case "mysql":
		return newMySQLSnapshotter(connString, tables)
	case "sqlite":
		return newSQLiteSnapshotter(connString, tables)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// baseSnapshotter provides shared logic for SQL-based snapshotters.
type baseSnapshotter struct {
	db             *sql.DB
	configuredTables []string
	dbType         string
}

func (b *baseSnapshotter) Close() error {
	return b.db.Close()
}

func (b *baseSnapshotter) SnapshotAll() (map[string][]map[string]any, error) {
	tables, err := b.Tables()
	if err != nil {
		return nil, err
	}

	state := make(map[string][]map[string]any)
	for _, table := range tables {
		rows, err := b.SnapshotTable(table)
		if err != nil {
			return nil, fmt.Errorf("snapshotting table %s: %w", table, err)
		}
		state[table] = rows
	}
	return state, nil
}

func (b *baseSnapshotter) RestoreAll(state map[string][]map[string]any) error {
	// Disable FK checks during restore
	if err := b.disableFKChecks(); err != nil {
		return fmt.Errorf("disabling FK checks: %w", err)
	}
	defer func() {
		if err := b.enableFKChecks(); err != nil {
			// Log the error but don't return it since we're in defer
			fmt.Printf("Warning: Failed to re-enable FK checks: %v\n", err)
		}
	}()

	for table, rows := range state {
		if err := b.RestoreTable(table, rows); err != nil {
			return fmt.Errorf("restoring table %s: %w", table, err)
		}
	}
	return nil
}

func (b *baseSnapshotter) Tables() ([]string, error) {
	if len(b.configuredTables) > 0 {
		return b.configuredTables, nil
	}
	return b.discoverTables()
}

func (b *baseSnapshotter) SnapshotTable(table string) ([]map[string]any, error) {
	quotedTable := b.quoteIdentifier(table)
	rows, err := b.db.Query("SELECT * FROM " + quotedTable)
	if err != nil {
		return nil, fmt.Errorf("querying table %s: %w", table, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for readability
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result, rows.Err()
}

// RestoreTable truncates a table and inserts the given rows.
// Security: This function uses parameterized queries for all data values to prevent SQL injection.
// Table and column names are quoted using quoteIdentifier() to handle special characters safely.
func (b *baseSnapshotter) RestoreTable(table string, rows []map[string]any) error {
	quotedTable := b.quoteIdentifier(table)

	// Truncate (using DELETE instead of TRUNCATE for better compatibility)
	if _, err := b.db.Exec("DELETE FROM " + quotedTable); err != nil {
		return fmt.Errorf("truncating table %s: %w", table, err)
	}

	// Insert rows
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		columns := make([]string, 0, len(row))
		placeholders := make([]string, 0, len(row))
		values := make([]any, 0, len(row))

		i := 0
		for col, val := range row {
			columns = append(columns, b.quoteIdentifier(col))
			placeholders = append(placeholders, b.placeholder(i))
			values = append(values, val)
			i++
		}

		// Use parameterized query for values (SQL injection safe)
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quotedTable,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "))

		if _, err := b.db.Exec(query, values...); err != nil {
			return fmt.Errorf("inserting into %s: %w", table, err)
		}
	}
	return nil
}

func (b *baseSnapshotter) discoverTables() ([]string, error) {
	switch b.dbType {
	case "postgres":
		return b.queryStrings("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	case "mysql":
		return b.queryStrings("SHOW TABLES")
	case "sqlite":
		return b.queryStrings("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	default:
		return nil, fmt.Errorf("unsupported db type for discovery: %s", b.dbType)
	}
}

func (b *baseSnapshotter) queryStrings(query string) ([]string, error) {
	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// quoteIdentifier properly quotes database identifiers (table/column names) to prevent SQL injection.
// Different databases use different quoting mechanisms:
//   - MySQL: backticks `table_name`
//   - PostgreSQL/SQLite: double quotes "table_name"
//
// Security: This function is critical for SQL injection prevention when identifiers come from
// user input (e.g., configured table names). The function escapes quote characters by doubling them,
// which is the standard SQL escaping mechanism.
//
// Note: While this provides basic protection, table/column names should ideally come from
// trusted configuration only, not directly from user input.
func (b *baseSnapshotter) quoteIdentifier(name string) string {
	switch b.dbType {
	case "mysql":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}

func (b *baseSnapshotter) placeholder(index int) string {
	switch b.dbType {
	case "postgres":
		return fmt.Sprintf("$%d", index+1)
	default:
		return "?"
	}
}

func (b *baseSnapshotter) disableFKChecks() error {
	switch b.dbType {
	case "postgres":
		_, err := b.db.Exec("SET session_replication_role = 'replica'")
		return err
	case "mysql":
		_, err := b.db.Exec("SET FOREIGN_KEY_CHECKS = 0")
		return err
	case "sqlite":
		_, err := b.db.Exec("PRAGMA foreign_keys = OFF")
		return err
	}
	return nil
}

func (b *baseSnapshotter) enableFKChecks() error {
	var err error
	switch b.dbType {
	case "postgres":
		_, err = b.db.Exec("SET session_replication_role = 'origin'")
	case "mysql":
		_, err = b.db.Exec("SET FOREIGN_KEY_CHECKS = 1")
	case "sqlite":
		_, err = b.db.Exec("PRAGMA foreign_keys = ON")
	}
	return err
}
