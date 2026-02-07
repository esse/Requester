package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
service:
  name: "test-api"
  base_url: "http://localhost:3000"
database:
  type: "sqlite"
  connection_string: ":memory:"
  tables:
    - users
    - orders
recording:
  proxy_port: 9090
  snapshot_dir: "./test-snapshots"
  format: "json"
  ignore_headers:
    - Authorization
  ignore_fields:
    - "*.created_at"
replay:
  test_database:
    connection_string: ":memory:"
  strict_mode: true
  timeout_ms: 3000
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Service.Name != "test-api" {
		t.Errorf("expected service name 'test-api', got %q", cfg.Service.Name)
	}
	if cfg.Service.BaseURL != "http://localhost:3000" {
		t.Errorf("expected base_url 'http://localhost:3000', got %q", cfg.Service.BaseURL)
	}
	if cfg.Database.Type != "sqlite" {
		t.Errorf("expected db type 'sqlite', got %q", cfg.Database.Type)
	}
	if len(cfg.Database.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(cfg.Database.Tables))
	}
	if cfg.Recording.ProxyPort != 9090 {
		t.Errorf("expected proxy_port 9090, got %d", cfg.Recording.ProxyPort)
	}
	if cfg.Recording.Format != "json" {
		t.Errorf("expected format 'json', got %q", cfg.Recording.Format)
	}
	if !cfg.Replay.StrictMode {
		t.Error("expected strict_mode true")
	}
	if cfg.Replay.TimeoutMs != 3000 {
		t.Errorf("expected timeout_ms 3000, got %d", cfg.Replay.TimeoutMs)
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
service:
  name: "api"
  base_url: "http://localhost:8080"
database:
  type: "postgres"
  connection_string: "postgres://localhost/test"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Recording.SnapshotDir != "./snapshots" {
		t.Errorf("expected default snapshot_dir './snapshots', got %q", cfg.Recording.SnapshotDir)
	}
	if cfg.Recording.Format != "json" {
		t.Errorf("expected default format 'json', got %q", cfg.Recording.Format)
	}
	if cfg.Recording.ProxyPort != 8080 {
		t.Errorf("expected default proxy_port 8080, got %d", cfg.Recording.ProxyPort)
	}
	if cfg.Replay.TimeoutMs != 5000 {
		t.Errorf("expected default timeout_ms 5000, got %d", cfg.Replay.TimeoutMs)
	}
}

func TestLoad_InvalidDBType(t *testing.T) {
	content := `
service:
  name: "api"
  base_url: "http://localhost:8080"
database:
  type: "redis"
  connection_string: "localhost:6379"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unsupported db type")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	content := `
service:
  name: ""
  base_url: "http://localhost:8080"
database:
  type: "postgres"
  connection_string: "postgres://localhost/test"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing service name")
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_DB_USER", "testuser")
	os.Setenv("TEST_DB_PASS", "testpass")
	os.Setenv("TEST_DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("TEST_DB_USER")
		os.Unsetenv("TEST_DB_PASS")
		os.Unsetenv("TEST_DB_NAME")
	}()

	content := `
service:
  name: "test-api"
  base_url: "http://localhost:3000"
database:
  type: "postgres"
  connection_string: "postgres://${TEST_DB_USER}:${TEST_DB_PASS}@localhost:5432/${TEST_DB_NAME}"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	// Use 0o600 to restrict access to owner-only (security best practice for files with credentials)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	expected := "postgres://testuser:testpass@localhost:5432/testdb"
	if cfg.Database.ConnectionString != expected {
		t.Errorf("expected connection string %q, got %q", expected, cfg.Database.ConnectionString)
	}
}
