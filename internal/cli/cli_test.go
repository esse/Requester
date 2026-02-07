package cli

import (
	"testing"

	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

func TestFireRequestForUpdate_UsesSharedClient(t *testing.T) {
	// This tests that the helper function delegates properly.
	// A full integration test would need a running service.
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:    "test",
			BaseURL: "http://127.0.0.1:1", // unreachable
		},
		Replay: config.ReplayConfig{
			TimeoutMs: 100,
		},
	}

	req := snapshot.Request{
		Method: "GET",
		URL:    "/api/test",
	}

	_, err := fireRequestForUpdate(cfg, req)
	if err == nil {
		t.Error("expected error for unreachable service")
	}
}

func TestComputeDiffForUpdate(t *testing.T) {
	before := map[string][]map[string]any{
		"users": {
			{"id": float64(1), "name": "Alice"},
		},
	}
	after := map[string][]map[string]any{
		"users": {
			{"id": float64(1), "name": "Alice"},
			{"id": float64(2), "name": "Bob"},
		},
	}

	diff := computeDiffForUpdate(before, after)
	if diff == nil {
		t.Fatal("expected non-nil diff")
	}

	userDiff, ok := diff["users"]
	if !ok {
		t.Fatal("expected diff for users table")
	}
	if len(userDiff.Added) != 1 {
		t.Errorf("expected 1 added row, got %d", len(userDiff.Added))
	}
}

func TestNewSnapshotterForUpdate_InvalidType(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Type:             "invalid",
			ConnectionString: "test",
		},
	}

	_, err := newSnapshotterForUpdate(cfg, "test")
	if err == nil {
		t.Error("expected error for invalid database type")
	}
}
