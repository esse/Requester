package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "json")

	snap := &Snapshot{
		ID:        "test123",
		Timestamp: time.Date(2026, 2, 7, 14, 30, 0, 0, time.UTC),
		Service:   "my-api",
		Tags:      []string{"users", "happy-path"},
		DBStateBefore: map[string][]map[string]any{
			"users": {{"id": 1, "name": "Alice"}},
		},
		Request: Request{
			Method:  "POST",
			URL:     "/users",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"name": "Bob"},
		},
		Response: Response{
			Status:  201,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"id": float64(2), "name": "Bob"},
		},
		DBStateAfter: map[string][]map[string]any{
			"users": {
				{"id": 1, "name": "Alice"},
				{"id": float64(2), "name": "Bob"},
			},
		},
		DBDiff: map[string]TableDiff{
			"users": {
				Added:    []map[string]any{{"id": float64(2), "name": "Bob"}},
				Removed:  []map[string]any{},
				Modified: []ModifiedRow{},
			},
		},
	}

	path, err := store.Save(snap)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Snapshot file not created at %s", path)
	}

	loaded, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ID != snap.ID {
		t.Errorf("expected ID %q, got %q", snap.ID, loaded.ID)
	}
	if loaded.Service != snap.Service {
		t.Errorf("expected Service %q, got %q", snap.Service, loaded.Service)
	}
	if loaded.Request.Method != snap.Request.Method {
		t.Errorf("expected Method %q, got %q", snap.Request.Method, loaded.Request.Method)
	}
	if loaded.Response.Status != snap.Response.Status {
		t.Errorf("expected Status %d, got %d", snap.Response.Status, loaded.Response.Status)
	}
}

func TestStoreLoadAll(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "json")

	for i := 0; i < 3; i++ {
		snap := &Snapshot{
			ID:      GenerateID(),
			Service: "test-svc",
			Request: Request{Method: "GET", URL: "/items"},
			Response: Response{Status: 200},
			DBStateBefore: map[string][]map[string]any{},
			DBStateAfter:  map[string][]map[string]any{},
			DBDiff:        map[string]TableDiff{},
		}
		if _, err := store.Save(snap); err != nil {
			t.Fatalf("Save %d failed: %v", i, err)
		}
	}

	all, paths, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(all))
	}
	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(paths))
	}
}

func TestStoreLoadByTag(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "json")

	snap1 := &Snapshot{
		ID:      "a1",
		Service: "svc",
		Tags:    []string{"smoke"},
		Request: Request{Method: "GET", URL: "/a"},
		Response: Response{Status: 200},
		DBStateBefore: map[string][]map[string]any{},
		DBStateAfter:  map[string][]map[string]any{},
		DBDiff:        map[string]TableDiff{},
	}
	snap2 := &Snapshot{
		ID:      "b2",
		Service: "svc",
		Tags:    []string{"regression"},
		Request: Request{Method: "GET", URL: "/b"},
		Response: Response{Status: 200},
		DBStateBefore: map[string][]map[string]any{},
		DBStateAfter:  map[string][]map[string]any{},
		DBDiff:        map[string]TableDiff{},
	}

	store.Save(snap1)
	store.Save(snap2)

	filtered, _, err := store.LoadByTag([]string{"smoke"})
	if err != nil {
		t.Fatalf("LoadByTag failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 snapshot with tag 'smoke', got %d", len(filtered))
	}
}

func TestStoreYAMLFormat(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "yaml")

	snap := &Snapshot{
		ID:      "yaml1",
		Service: "svc",
		Request: Request{Method: "GET", URL: "/test"},
		Response: Response{Status: 200},
		DBStateBefore: map[string][]map[string]any{},
		DBStateAfter:  map[string][]map[string]any{},
		DBDiff:        map[string]TableDiff{},
	}

	path, err := store.Save(snap)
	if err != nil {
		t.Fatalf("Save YAML failed: %v", err)
	}

	if filepath.Ext(path) != ".yaml" {
		// Check the full suffix
		if !matchesSuffix(path, ".snapshot.yaml") {
			t.Errorf("expected .snapshot.yaml extension, got %s", path)
		}
	}

	loaded, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load YAML failed: %v", err)
	}
	if loaded.ID != "yaml1" {
		t.Errorf("expected ID 'yaml1', got %q", loaded.ID)
	}
}

func TestStoreUpdate(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "json")

	snap := &Snapshot{
		ID:      "upd1",
		Service: "svc",
		Request: Request{Method: "GET", URL: "/update"},
		Response: Response{Status: 200, Body: "old"},
		DBStateBefore: map[string][]map[string]any{},
		DBStateAfter:  map[string][]map[string]any{},
		DBDiff:        map[string]TableDiff{},
	}

	path, err := store.Save(snap)
	if err != nil {
		t.Fatal(err)
	}

	snap.Response.Body = "new"
	if err := store.Update(path, snap); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Response.Body != "new" {
		t.Errorf("expected updated body 'new', got %v", loaded.Response.Body)
	}
}

func matchesSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
