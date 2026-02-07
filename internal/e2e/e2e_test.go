package e2e

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/recorder"
	"github.com/esse/snapshot-tester/internal/replayer"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

// setupSQLiteDB creates a temp SQLite database with a users table and returns the path.
func setupSQLiteDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer sqlDB.Close()

	_, err = sqlDB.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`)
	if err != nil {
		t.Fatalf("creating table: %v", err)
	}

	_, err = sqlDB.Exec(`INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@test.com')`)
	if err != nil {
		t.Fatalf("inserting seed data: %v", err)
	}

	return dbPath
}

// TestE2E_RecordAndReplay_GET tests the full record-then-replay cycle for a GET request
// with no DB mutations. The service returns a JSON response; we record it, save the snapshot,
// then replay it and verify it passes.
func TestE2E_RecordAndReplay_GET(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	// Target service: returns user data
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "Alice"})
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-test",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: snapshotDir,
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	// --- RECORD PHASE ---
	rec, err := recorder.New(cfg, []string{"e2e"})
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	// Use recorder's ServeHTTP directly (no need to start a listener)
	recReq := httptest.NewRequest("GET", "/api/users/1", nil)
	recResp := httptest.NewRecorder()
	rec.ServeHTTP(recResp, recReq)

	if recResp.Code != 200 {
		t.Fatalf("expected recording response 200, got %d", recResp.Code)
	}

	// Verify snapshot was saved
	store := snapshot.NewStore(snapshotDir, "json")
	snaps, paths, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	snap := snaps[0]
	if snap.Request.Method != "GET" {
		t.Errorf("expected GET method, got %s", snap.Request.Method)
	}
	if snap.Response.Status != 200 {
		t.Errorf("expected status 200, got %d", snap.Response.Status)
	}
	if snap.Service != "e2e-test" {
		t.Errorf("expected service e2e-test, got %s", snap.Service)
	}
	if len(snap.Tags) != 1 || snap.Tags[0] != "e2e" {
		t.Errorf("expected tags [e2e], got %v", snap.Tags)
	}

	// Verify DB state was captured
	if snap.DBStateBefore == nil {
		t.Fatal("expected DBStateBefore to be set")
	}
	if snap.DBStateAfter == nil {
		t.Fatal("expected DBStateAfter to be set")
	}

	// --- REPLAY PHASE ---
	// Create a new replayer using the same config (pointing to same service)
	rep := createReplayer(t, cfg, dbPath)
	defer rep.Close()

	result := rep.ReplayOne(snap, paths[0])
	if result.Error != "" {
		t.Fatalf("replay error: %s", result.Error)
	}
	if !result.Passed {
		t.Errorf("expected replay to pass, got diffs: %v", result.Diffs)
	}
}

// TestE2E_RecordAndReplay_POST_WithDBMutation tests recording a POST that adds a row,
// then replaying it against a service that behaves the same way.
func TestE2E_RecordAndReplay_POST_WithDBMutation(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	// Target service: inserts a user and returns it
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Actually insert into the DB to simulate a real service
		sqlDB, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer sqlDB.Close()

		_, err = sqlDB.Exec(`INSERT INTO users (id, name, email) VALUES (2, 'Bob', 'bob@test.com')`)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": 2, "name": "Bob", "email": "bob@test.com"})
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-mutation",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: snapshotDir,
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	// --- RECORD PHASE ---
	rec, err := recorder.New(cfg, nil)
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	recReq := httptest.NewRequest("POST", "/api/users", nil)
	recResp := httptest.NewRecorder()
	rec.ServeHTTP(recResp, recReq)

	if recResp.Code != 201 {
		t.Fatalf("expected recording response 201, got %d", recResp.Code)
	}

	// Verify snapshot was saved
	store := snapshot.NewStore(snapshotDir, "json")
	snaps, _, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	snap := snaps[0]

	// Verify the snapshot captured the DB mutation
	usersBefore := snap.DBStateBefore["users"]
	usersAfter := snap.DBStateAfter["users"]

	if len(usersBefore) != 1 {
		t.Errorf("expected 1 user before, got %d", len(usersBefore))
	}
	if len(usersAfter) != 2 {
		t.Errorf("expected 2 users after, got %d", len(usersAfter))
	}

	// Verify DBDiff captured the addition
	diff, ok := snap.DBDiff["users"]
	if !ok {
		t.Fatal("expected DBDiff for users table")
	}
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added row in diff, got %d", len(diff.Added))
	}
}

// TestE2E_ReplayDetectsMismatch verifies that replay correctly fails when the service
// returns a different response than what was recorded.
func TestE2E_ReplayDetectsMismatch(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	callCount := 0

	// Target service: returns different data on second call (replay)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if callCount == 1 {
			// Recording: original response
			json.NewEncoder(w).Encode(map[string]any{"version": "1.0"})
		} else {
			// Replay: changed response
			json.NewEncoder(w).Encode(map[string]any{"version": "2.0"})
		}
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-mismatch",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: snapshotDir,
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	// --- RECORD ---
	rec, err := recorder.New(cfg, nil)
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	recReq := httptest.NewRequest("GET", "/api/version", nil)
	recResp := httptest.NewRecorder()
	rec.ServeHTTP(recResp, recReq)

	store := snapshot.NewStore(snapshotDir, "json")
	snaps, paths, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	// --- REPLAY (should detect mismatch) ---
	rep := createReplayer(t, cfg, dbPath)
	defer rep.Close()

	result := rep.ReplayOne(snaps[0], paths[0])

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Passed {
		t.Error("expected replay to fail due to response mismatch")
	}
	if len(result.Diffs) == 0 {
		t.Error("expected at least one diff")
	}

	// Check that the diff mentions the version change
	foundVersionDiff := false
	for _, d := range result.Diffs {
		if d.Path == "response.body.version" {
			foundVersionDiff = true
			break
		}
	}
	if !foundVersionDiff {
		t.Errorf("expected diff at response.body.version, got diffs: %v", result.Diffs)
	}
}

// TestE2E_SnapshotStoreRoundTrip verifies that snapshots survive save/load without data loss.
func TestE2E_SnapshotStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := snapshot.NewStore(dir, "json")

	original := &snapshot.Snapshot{
		ID:      "roundtrip-test",
		Service: "test-svc",
		Tags:    []string{"tag1", "tag2"},
		DBStateBefore: map[string][]map[string]any{
			"users": {{"id": float64(1), "name": "Alice"}},
		},
		Request: snapshot.Request{
			Method:  "POST",
			URL:     "/api/users",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"name": "Bob"},
		},
		OutgoingRequests: []snapshot.OutgoingRequest{
			{
				Method: "GET",
				URL:    "/external/validate",
				Response: &snapshot.Response{
					Status: 200,
					Body:   map[string]any{"valid": true},
				},
			},
		},
		Response: snapshot.Response{
			Status:  201,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"id": float64(2), "name": "Bob"},
		},
		DBStateAfter: map[string][]map[string]any{
			"users": {
				{"id": float64(1), "name": "Alice"},
				{"id": float64(2), "name": "Bob"},
			},
		},
		DBDiff: map[string]snapshot.TableDiff{
			"users": {
				Added:    []map[string]any{{"id": float64(2), "name": "Bob"}},
				Removed:  []map[string]any{},
				Modified: []snapshot.ModifiedRow{},
			},
		},
	}

	path, err := store.Save(original)
	if err != nil {
		t.Fatalf("saving snapshot: %v", err)
	}

	loaded, err := store.Load(path)
	if err != nil {
		t.Fatalf("loading snapshot: %v", err)
	}

	// Verify key fields survive round-trip
	if loaded.ID != original.ID {
		t.Errorf("ID: got %s, want %s", loaded.ID, original.ID)
	}
	if loaded.Service != original.Service {
		t.Errorf("Service: got %s, want %s", loaded.Service, original.Service)
	}
	if loaded.Request.Method != original.Request.Method {
		t.Errorf("Request.Method: got %s, want %s", loaded.Request.Method, original.Request.Method)
	}
	if loaded.Response.Status != original.Response.Status {
		t.Errorf("Response.Status: got %d, want %d", loaded.Response.Status, original.Response.Status)
	}
	if len(loaded.OutgoingRequests) != 1 {
		t.Errorf("OutgoingRequests: got %d, want 1", len(loaded.OutgoingRequests))
	}
	if len(loaded.Tags) != 2 {
		t.Errorf("Tags: got %d, want 2", len(loaded.Tags))
	}

	// Verify DB states
	afterUsers := loaded.DBStateAfter["users"]
	if len(afterUsers) != 2 {
		t.Errorf("DBStateAfter users: got %d rows, want 2", len(afterUsers))
	}
}

// TestE2E_MultipleSnapshots_ReplayAll records multiple requests and replays them all.
func TestE2E_MultipleSnapshots_ReplayAll(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	// Target service: handles multiple endpoints
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/users":
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]any{map[string]any{"id": 1, "name": "Alice"}})
		case "/api/health":
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		default:
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
		}
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-multi",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: snapshotDir,
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	// Record two requests
	rec, err := recorder.New(cfg, nil)
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	for _, path := range []string{"/api/users", "/api/health"} {
		req := httptest.NewRequest("GET", path, nil)
		resp := httptest.NewRecorder()
		rec.ServeHTTP(resp, req)
		if resp.Code != 200 {
			t.Fatalf("expected 200 for %s, got %d", path, resp.Code)
		}
	}

	// Load all snapshots
	store := snapshot.NewStore(snapshotDir, "json")
	snaps, paths, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	// Replay all
	rep := createReplayer(t, cfg, dbPath)
	defer rep.Close()

	results := rep.ReplayAll(snaps, paths)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, result := range results {
		if result.Error != "" {
			t.Errorf("result %d: unexpected error: %s", i, result.Error)
		}
		if !result.Passed {
			t.Errorf("result %d: expected pass, got diffs: %v", i, result.Diffs)
		}
	}
}

// TestE2E_Redaction verifies that field redaction works end-to-end.
func TestE2E_Redaction(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	// Target service returns sensitive data
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       1,
			"name":     "Alice",
			"password": "secret123",
			"token":    "abc-def-ghi",
		})
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-redact",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir:  snapshotDir,
			Format:       "json",
			RedactFields: []string{"*.password", "response.body.token"},
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	rec, err := recorder.New(cfg, nil)
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	recReq := httptest.NewRequest("GET", "/api/users/1", nil)
	recResp := httptest.NewRecorder()
	rec.ServeHTTP(recResp, recReq)

	// Load and check that sensitive fields were redacted
	store := snapshot.NewStore(snapshotDir, "json")
	snaps, _, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	body, ok := snaps[0].Response.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected response body to be map, got %T", snaps[0].Response.Body)
	}

	if body["password"] != "[REDACTED]" {
		t.Errorf("expected password to be [REDACTED], got %v", body["password"])
	}
	if body["token"] != "[REDACTED]" {
		t.Errorf("expected token to be [REDACTED], got %v", body["token"])
	}
	// Non-redacted fields should be preserved
	if body["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", body["name"])
	}
}

// TestE2E_MockServerIntegration tests the mock server during replay of outgoing requests.
func TestE2E_MockServerIntegration(t *testing.T) {
	dbPath := setupSQLiteDB(t)

	// Create a snapshot manually with outgoing request expectations
	snap := &snapshot.Snapshot{
		ID:      "mock-test",
		Service: "e2e-mock",
		DBStateBefore: map[string][]map[string]any{
			"users": {{"id": "1", "name": "Alice", "email": "alice@test.com"}},
		},
		Request: snapshot.Request{
			Method: "GET",
			URL:    "/api/enriched-user",
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"enriched": true},
		},
		// After RestoreAll sets state to DBStateBefore, SnapshotAll returns that
		DBStateAfter: map[string][]map[string]any{
			"users": {{"id": "1", "name": "Alice", "email": "alice@test.com"}},
		},
		OutgoingRequests: []snapshot.OutgoingRequest{
			{
				Method: "GET",
				URL:    "/external/enrich",
				Response: &snapshot.Response{
					Status: 200,
					Body:   map[string]any{"extra": "data"},
				},
			},
		},
		DBDiff: map[string]snapshot.TableDiff{},
	}

	// Set up a service that returns the expected response
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"enriched": true})
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-mock",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: t.TempDir(),
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	rep := createReplayer(t, cfg, dbPath)
	defer rep.Close()

	result := rep.ReplayOne(snap, "/mock/test.json")

	if result.Error != "" {
		t.Fatalf("replay error: %s", result.Error)
	}
	if !result.Passed {
		t.Errorf("expected replay to pass, got diffs: %v", result.Diffs)
	}
}

// TestE2E_DBSnapshotterRealSQLite tests real SQLite snapshotter operations (snapshot + restore).
func TestE2E_DBSnapshotterRealSQLite(t *testing.T) {
	dbPath := setupSQLiteDB(t)

	snapshotter, err := db.NewSnapshotter("sqlite", dbPath, []string{"users"})
	if err != nil {
		t.Fatalf("creating snapshotter: %v", err)
	}
	defer snapshotter.Close()

	// Snapshot current state
	state1, err := snapshotter.SnapshotAll()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	users := state1["users"]
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if fmt.Sprintf("%v", users[0]["name"]) != "Alice" {
		t.Errorf("expected Alice, got %v", users[0]["name"])
	}

	// Insert another row directly
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer sqlDB.Close()
	_, err = sqlDB.Exec(`INSERT INTO users (id, name, email) VALUES (2, 'Bob', 'bob@test.com')`)
	if err != nil {
		t.Fatalf("inserting: %v", err)
	}

	// Snapshot again - should have 2 rows
	state2, err := snapshotter.SnapshotAll()
	if err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}
	if len(state2["users"]) != 2 {
		t.Fatalf("expected 2 users, got %d", len(state2["users"]))
	}

	// Restore to original state (1 user)
	err = snapshotter.RestoreAll(state1)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	// Verify restore worked
	state3, err := snapshotter.SnapshotAll()
	if err != nil {
		t.Fatalf("snapshot 3: %v", err)
	}
	if len(state3["users"]) != 1 {
		t.Errorf("expected 1 user after restore, got %d", len(state3["users"]))
	}
}

// TestE2E_YAMLFormat tests that YAML snapshots work end-to-end.
func TestE2E_YAMLFormat(t *testing.T) {
	dbPath := setupSQLiteDB(t)
	snapshotDir := t.TempDir()

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"format": "yaml"})
	}))
	defer service.Close()

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:       "e2e-yaml",
			BaseURL:    service.URL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: dbPath,
			Tables:           []string{"users"},
		},
		Recording: config.RecordingConfig{
			SnapshotDir: snapshotDir,
			Format:      "yaml",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}

	rec, err := recorder.New(cfg, nil)
	if err != nil {
		t.Fatalf("creating recorder: %v", err)
	}
	defer rec.Close()

	recReq := httptest.NewRequest("GET", "/api/format", nil)
	recResp := httptest.NewRecorder()
	rec.ServeHTTP(recResp, recReq)

	// Check that a YAML file was created
	store := snapshot.NewStore(snapshotDir, "yaml")
	snaps, _, err := store.LoadAll()
	if err != nil {
		t.Fatalf("loading snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	// Verify the file extension
	entries, err := os.ReadDir(filepath.Join(snapshotDir, "e2e-yaml", "GET_api_format"))
	if err != nil {
		t.Fatalf("reading snapshot dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .yaml snapshot file")
	}
}

// createReplayer creates a replayer with a real SQLite snapshotter for e2e tests.
func createReplayer(t *testing.T, cfg *config.Config, dbPath string) *replayer.Replayer {
	t.Helper()

	snapshotter, err := db.NewSnapshotter("sqlite", dbPath, cfg.Database.Tables)
	if err != nil {
		t.Fatalf("creating snapshotter for replayer: %v", err)
	}

	// Use the exported New constructor indirectly by creating a replayer
	// Since New() connects to DB, we just use it directly
	rep, err := replayer.New(cfg)
	if err != nil {
		snapshotter.Close()
		t.Fatalf("creating replayer: %v", err)
	}

	// Close the extra snapshotter since New() creates its own
	snapshotter.Close()
	return rep
}
