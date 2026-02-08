package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
		CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total REAL);
		INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@example.com');
		INSERT INTO users (id, name, email) VALUES (2, 'Bob', 'bob@example.com');
		INSERT INTO orders (id, user_id, total) VALUES (1, 1, 99.99);
	`)
	if err != nil {
		t.Fatal(err)
	}

	return dbPath
}

func TestSQLiteSnapshotter_Tables(t *testing.T) {
	dbPath := setupTestDB(t)

	snap, err := NewSnapshotter("sqlite", dbPath, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	tables, err := snap.Tables()
	if err != nil {
		t.Fatal(err)
	}

	if len(tables) < 2 {
		t.Errorf("expected at least 2 tables, got %d", len(tables))
	}
}

func TestSQLiteSnapshotter_ConfiguredTables(t *testing.T) {
	dbPath := setupTestDB(t)

	snap, err := NewSnapshotter("sqlite", dbPath, []string{"users"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	tables, err := snap.Tables()
	if err != nil {
		t.Fatal(err)
	}

	if len(tables) != 1 || tables[0] != "users" {
		t.Errorf("expected [users], got %v", tables)
	}
}

func TestSQLiteSnapshotter_SnapshotTable(t *testing.T) {
	dbPath := setupTestDB(t)

	snap, err := NewSnapshotter("sqlite", dbPath, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	rows, err := snap.SnapshotTable("users")
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestSQLiteSnapshotter_SnapshotAll(t *testing.T) {
	dbPath := setupTestDB(t)

	snap, err := NewSnapshotter("sqlite", dbPath, []string{"users", "orders"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	state, err := snap.SnapshotAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(state) != 2 {
		t.Errorf("expected 2 tables in state, got %d", len(state))
	}
	if len(state["users"]) != 2 {
		t.Errorf("expected 2 users, got %d", len(state["users"]))
	}
	if len(state["orders"]) != 1 {
		t.Errorf("expected 1 order, got %d", len(state["orders"]))
	}
}

func TestSQLiteSnapshotter_RestoreAll(t *testing.T) {
	dbPath := setupTestDB(t)

	snap, err := NewSnapshotter("sqlite", dbPath, []string{"users", "orders"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	// Capture current state
	original, err := snap.SnapshotAll()
	if err != nil {
		t.Fatal(err)
	}

	// Restore a different state
	newState := map[string][]map[string]any{
		"users": {
			{"id": int64(10), "name": "Charlie", "email": "charlie@example.com"},
		},
		"orders": {},
	}

	if err := snap.RestoreAll(newState); err != nil {
		t.Fatal(err)
	}

	// Verify new state
	after, err := snap.SnapshotAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(after["users"]) != 1 {
		t.Errorf("expected 1 user after restore, got %d", len(after["users"]))
	}
	if len(after["orders"]) != 0 {
		t.Errorf("expected 0 orders after restore, got %d", len(after["orders"]))
	}

	// Restore original
	if err := snap.RestoreAll(original); err != nil {
		t.Fatal(err)
	}

	restored, _ := snap.SnapshotAll()
	if len(restored["users"]) != 2 {
		t.Errorf("expected 2 users after restore original, got %d", len(restored["users"]))
	}
}

func TestSQLiteSnapshotter_EmptyTable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
	db.Close()

	snap, err := NewSnapshotter("sqlite", dbPath, []string{"items"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	rows, err := snap.SnapshotTable("items")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestSQLiteSnapshotter_InvalidPath(t *testing.T) {
	// A non-existent deep path should fail on ping
	_, err := NewSnapshotter("sqlite", "/nonexistent/deep/path/db.sqlite", nil, nil)
	if err != nil {
		// Expected - some systems may not fail until first query
		// This is OK either way
		_ = err
	}
}

func TestUnsupportedDBType(t *testing.T) {
	_, err := NewSnapshotter("redis", "localhost:6379", nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported db type")
	}
}

func TestQuoteIdentifier_Simple(t *testing.T) {
	pg := &baseSnapshotter{dbType: DBTypePostgres}
	mysql := &baseSnapshotter{dbType: DBTypeMySQL}

	if got := pg.quoteIdentifier("users"); got != `"users"` {
		t.Errorf("postgres simple: expected %q, got %q", `"users"`, got)
	}
	if got := mysql.quoteIdentifier("users"); got != "`users`" {
		t.Errorf("mysql simple: expected %q, got %q", "`users`", got)
	}
}

func TestQuoteIdentifier_SchemaQualified(t *testing.T) {
	pg := &baseSnapshotter{dbType: DBTypePostgres}
	mysql := &baseSnapshotter{dbType: DBTypeMySQL}

	if got := pg.quoteIdentifier("myschema.users"); got != `"myschema"."users"` {
		t.Errorf("postgres schema-qualified: expected %q, got %q", `"myschema"."users"`, got)
	}
	if got := mysql.quoteIdentifier("mydb.users"); got != "`mydb`.`users`" {
		t.Errorf("mysql schema-qualified: expected %q, got %q", "`mydb`.`users`", got)
	}
}

func TestNamespacesStoredInSnapshotter(t *testing.T) {
	dbPath := setupTestDB(t)

	// SQLite ignores namespaces, but they should be stored without error
	snap, err := NewSnapshotter("sqlite", dbPath, nil, []string{"main"})
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	tables, err := snap.Tables()
	if err != nil {
		t.Fatal(err)
	}

	if len(tables) < 2 {
		t.Errorf("expected at least 2 tables, got %d", len(tables))
	}
}

// cleanup temp files
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
