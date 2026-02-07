package replayer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

// mockSnapshotter implements db.Snapshotter for testing.
type mockSnapshotter struct {
	state       map[string][]map[string]any
	restoreErr  error
	snapshotErr error
	closed      bool
}

func (m *mockSnapshotter) Tables() ([]string, error) {
	tables := make([]string, 0, len(m.state))
	for t := range m.state {
		tables = append(tables, t)
	}
	return tables, nil
}

func (m *mockSnapshotter) SnapshotTable(table string) ([]map[string]any, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.state[table], nil
}

func (m *mockSnapshotter) SnapshotAll() (map[string][]map[string]any, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.state, nil
}

func (m *mockSnapshotter) RestoreTable(table string, rows []map[string]any) error {
	if m.restoreErr != nil {
		return m.restoreErr
	}
	m.state[table] = rows
	return nil
}

func (m *mockSnapshotter) RestoreAll(state map[string][]map[string]any) error {
	if m.restoreErr != nil {
		return m.restoreErr
	}
	m.state = state
	return nil
}

func (m *mockSnapshotter) Close() error {
	m.closed = true
	return nil
}

func newTestConfig(baseURL string) *config.Config {
	return &config.Config{
		Service: config.ServiceConfig{
			Name:       "test-service",
			BaseURL:    baseURL,
			MockEnvVar: "SNAPSHOT_MOCK_URL",
		},
		Database: config.DatabaseConfig{
			Type:             "sqlite",
			ConnectionString: ":memory:",
		},
		Recording: config.RecordingConfig{
			SnapshotDir: "./snapshots",
			Format:      "json",
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 5000,
		},
	}
}

func TestReplayOne_Success(t *testing.T) {
	// Set up mock service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "Alice"})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	// The mock snapshotter doesn't modify state on HTTP requests, so
	// DBStateBefore and DBStateAfter must be identical for the test to pass.
	dbState := map[string][]map[string]any{
		"users": {{"id": float64(1), "name": "Alice"}},
	}

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: map[string][]map[string]any{}},
	}

	snap := &snapshot.Snapshot{
		ID:            "test123",
		Service:       "test-service",
		DBStateBefore: dbState,
		Request: snapshot.Request{
			Method: "GET",
			URL:    "/api/users/1",
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"id": float64(1), "name": "Alice"},
		},
		// After RestoreAll sets state to DBStateBefore, SnapshotAll returns that same state
		DBStateAfter: dbState,
	}

	result := r.ReplayOne(snap, "/test/path.json")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.Passed {
		t.Errorf("expected test to pass, got diffs: %v", result.Diffs)
	}
	if result.SnapshotID != "test123" {
		t.Errorf("expected snapshot ID test123, got %s", result.SnapshotID)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestReplayOne_ResponseMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "name": "Bob"})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	dbState := map[string][]map[string]any{"users": {}}

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: dbState},
	}

	snap := &snapshot.Snapshot{
		ID:            "test456",
		DBStateBefore: dbState,
		Request: snapshot.Request{
			Method: "GET",
			URL:    "/api/users/1",
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"id": float64(1), "name": "Alice"},
		},
		DBStateAfter: dbState,
	}

	result := r.ReplayOne(snap, "/test/path.json")

	if result.Passed {
		t.Error("expected test to fail due to response mismatch")
	}
	if len(result.Diffs) == 0 {
		t.Error("expected diffs to be non-empty")
	}
}

func TestReplayOne_DBRestoreError(t *testing.T) {
	cfg := newTestConfig("http://localhost:9999")

	r := &Replayer{
		config: cfg,
		snapshotter: &mockSnapshotter{
			state:      map[string][]map[string]any{},
			restoreErr: fmt.Errorf("connection refused"),
		},
	}

	snap := &snapshot.Snapshot{
		ID:            "test789",
		DBStateBefore: map[string][]map[string]any{"users": {}},
		Request:       snapshot.Request{Method: "GET", URL: "/api/users"},
		Response:      snapshot.Response{Status: 200},
		DBStateAfter:  map[string][]map[string]any{"users": {}},
	}

	result := r.ReplayOne(snap, "/test/path.json")

	if result.Error == "" {
		t.Error("expected error from DB restore failure")
	}
	if result.Passed {
		t.Error("expected test to fail")
	}
}

func TestReplayOne_RequestError(t *testing.T) {
	// Use an unreachable URL to trigger request error
	cfg := newTestConfig("http://127.0.0.1:1")
	cfg.Replay.TimeoutMs = 100

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: map[string][]map[string]any{}},
	}

	snap := &snapshot.Snapshot{
		ID:            "testerr",
		DBStateBefore: map[string][]map[string]any{},
		Request:       snapshot.Request{Method: "GET", URL: "/api/test"},
		Response:      snapshot.Response{Status: 200},
		DBStateAfter:  map[string][]map[string]any{},
	}

	result := r.ReplayOne(snap, "/test/path.json")

	if result.Error == "" {
		t.Error("expected error from request failure")
	}
}

func TestReplayOne_DBSnapshotAfterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	// Mock that will fail on the second SnapshotAll call
	mock := &mockSnapshotter{state: map[string][]map[string]any{}}

	r := &Replayer{
		config:      cfg,
		snapshotter: mock,
	}

	snap := &snapshot.Snapshot{
		ID:            "testdberr",
		DBStateBefore: map[string][]map[string]any{},
		Request:       snapshot.Request{Method: "GET", URL: "/api/test"},
		Response:      snapshot.Response{Status: 200},
		DBStateAfter:  map[string][]map[string]any{},
	}

	// Set error after restore succeeds but before snapshot after
	result := r.ReplayOne(snap, "/test/path.json")
	// This should succeed since we don't inject the error mid-flight in this simple mock

	// Now test with snapshot error set
	mock.snapshotErr = fmt.Errorf("disk full")
	result = r.ReplayOne(snap, "/test/path.json")
	// RestoreAll calls SnapshotAll... wait, no. RestoreAll doesn't call SnapshotAll.
	// But we set snapshotErr, so SnapshotAll after the request will fail.
	// However, RestoreAll doesn't call SnapshotAll - it's separate.
	// The issue is restoreAll also won't fail since it doesn't snapshot.
	// Let me re-check: mock.restoreErr is nil, so RestoreAll works.
	// Then fireRequest works. Then SnapshotAll fails.
	if result.Error == "" {
		t.Error("expected error from DB snapshot after failure")
	}
}

func TestReplayAll_Sequential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	cfg.Replay.Parallel = false

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: map[string][]map[string]any{}},
	}

	snaps := []*snapshot.Snapshot{
		{
			ID:            "s1",
			DBStateBefore: map[string][]map[string]any{},
			Request:       snapshot.Request{Method: "GET", URL: "/api/1"},
			Response:      snapshot.Response{Status: 200, Body: map[string]any{"ok": true}},
			DBStateAfter:  map[string][]map[string]any{},
		},
		{
			ID:            "s2",
			DBStateBefore: map[string][]map[string]any{},
			Request:       snapshot.Request{Method: "GET", URL: "/api/2"},
			Response:      snapshot.Response{Status: 200, Body: map[string]any{"ok": true}},
			DBStateAfter:  map[string][]map[string]any{},
		},
	}
	paths := []string{"/path/1.json", "/path/2.json"}

	results := r.ReplayAll(snaps, paths)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, res := range results {
		if res.Error != "" {
			t.Errorf("result %d: unexpected error: %s", i, res.Error)
		}
	}
}

func TestReplayAll_Parallel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	cfg.Replay.Parallel = true

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: map[string][]map[string]any{}},
	}

	snaps := []*snapshot.Snapshot{
		{
			ID:            "p1",
			DBStateBefore: map[string][]map[string]any{},
			Request:       snapshot.Request{Method: "GET", URL: "/api/1"},
			Response:      snapshot.Response{Status: 200, Body: map[string]any{"ok": true}},
			DBStateAfter:  map[string][]map[string]any{},
		},
		{
			ID:            "p2",
			DBStateBefore: map[string][]map[string]any{},
			Request:       snapshot.Request{Method: "GET", URL: "/api/2"},
			Response:      snapshot.Response{Status: 200, Body: map[string]any{"ok": true}},
			DBStateAfter:  map[string][]map[string]any{},
		},
		{
			ID:            "p3",
			DBStateBefore: map[string][]map[string]any{},
			Request:       snapshot.Request{Method: "GET", URL: "/api/3"},
			Response:      snapshot.Response{Status: 200, Body: map[string]any{"ok": true}},
			DBStateAfter:  map[string][]map[string]any{},
		},
	}
	paths := []string{"/path/1.json", "/path/2.json", "/path/3.json"}

	results := r.ReplayAll(snaps, paths)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Verify all results have correct snapshot IDs (order preserved)
	for i, res := range results {
		expectedID := fmt.Sprintf("p%d", i+1)
		if res.SnapshotID != expectedID {
			t.Errorf("result %d: expected ID %s, got %s", i, expectedID, res.SnapshotID)
		}
	}
}

func TestReplayOne_WithOutgoingRequests(t *testing.T) {
	// Mock service that calls the mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	dbState := map[string][]map[string]any{}

	r := &Replayer{
		config:      cfg,
		snapshotter: &mockSnapshotter{state: dbState},
	}

	snap := &snapshot.Snapshot{
		ID:            "outgoing1",
		DBStateBefore: dbState,
		Request:       snapshot.Request{Method: "GET", URL: "/api/fetch"},
		OutgoingRequests: []snapshot.OutgoingRequest{
			{
				Method: "GET",
				URL:    "/external/api",
				Response: &snapshot.Response{
					Status: 200,
					Body:   map[string]any{"data": "external"},
				},
			},
		},
		Response:     snapshot.Response{Status: 200, Body: map[string]any{"result": "ok"}},
		DBStateAfter: dbState,
	}

	result := r.ReplayOne(snap, "/test/path.json")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestClose(t *testing.T) {
	mock := &mockSnapshotter{state: map[string][]map[string]any{}}
	r := &Replayer{
		config:      newTestConfig("http://localhost"),
		snapshotter: mock,
	}

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.closed {
		t.Error("expected snapshotter to be closed")
	}
}
