package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Supported database types (must match db.DBType* constants).
const (
	dbTypePostgres = "postgres"
	dbTypeMySQL    = "mysql"
	dbTypeSQLite   = "sqlite"
)

// Snapshot format identifiers (must match snapshot.Format* constants).
const (
	formatJSON = "json"
	formatYAML = "yaml"
)

// Default configuration values.
const (
	defaultSnapshotDir  = "./snapshots"
	defaultFormat       = formatJSON
	defaultProxyPort    = 8080
	defaultTimeoutMs    = 5000
	defaultMockEnvVar   = "SNAPSHOT_MOCK_URL"
	defaultStartupTimeMs = 2000
)

// Config represents the top-level configuration for snapshot-tester.
type Config struct {
	Service   ServiceConfig   `yaml:"service"`
	Database  DatabaseConfig  `yaml:"database"`
	Recording RecordingConfig `yaml:"recording"`
	Replay    ReplayConfig    `yaml:"replay"`
}

type ServiceConfig struct {
	Name          string `yaml:"name"`
	BaseURL       string `yaml:"base_url"`
	Command       string `yaml:"command"`         // Optional: command to start service as subprocess
	StartupTimeMs int    `yaml:"startup_time_ms"` // Time to wait after starting service (default: 2000)
	MockEnvVar    string `yaml:"mock_env_var"`    // Env var name to inject mock server URL (default: SNAPSHOT_MOCK_URL)
}

type DatabaseConfig struct {
	Type             string   `yaml:"type"` // postgres | mysql | sqlite
	ConnectionString string   `yaml:"connection_string"`
	Tables           []string `yaml:"tables"`
	Namespaces       []string `yaml:"namespaces"` // Schemas (postgres) or databases (mysql) to scan; defaults to public/current
}

type RecordingConfig struct {
	ProxyPort         int             `yaml:"proxy_port"`
	OutgoingProxyPort int             `yaml:"outgoing_proxy_port"` // Port for forward proxy capturing outgoing requests (0 = auto)
	SnapshotDir       string          `yaml:"snapshot_dir"`
	Format            string          `yaml:"format"` // json | yaml
	IgnoreHeaders     []string        `yaml:"ignore_headers"`
	IgnoreFields      []string        `yaml:"ignore_fields"`
	RedactFields      []string        `yaml:"redact_fields"`       // Fields to redact with [REDACTED] during recording
	ProxyAuthToken    string          `yaml:"proxy_auth_token"`    // If set, require Bearer token for proxy access
	RateLimit         RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig configures rate limiting for the recording proxy.
type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"` // Max requests per second (0 = unlimited)
	MaxConcurrent     int     `yaml:"max_concurrent"`      // Max concurrent requests (0 = unlimited)
}

type ReplayConfig struct {
	TestDatabase TestDatabaseConfig `yaml:"test_database"`
	StrictMode   bool               `yaml:"strict_mode"`
	TimeoutMs    int                `yaml:"timeout_ms"`
	Parallel     bool               `yaml:"parallel"`
}

type TestDatabaseConfig struct {
	ConnectionString string `yaml:"connection_string"`
}

// Load reads and parses a YAML configuration file.
// Environment variables in the form ${VAR_NAME} are expanded.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Expand environment variables in configuration
	cfg.expandEnvVars()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply defaults
	if cfg.Recording.SnapshotDir == "" {
		cfg.Recording.SnapshotDir = defaultSnapshotDir
	}
	if cfg.Recording.Format == "" {
		cfg.Recording.Format = defaultFormat
	}
	if cfg.Recording.ProxyPort == 0 {
		cfg.Recording.ProxyPort = defaultProxyPort
	}
	if cfg.Replay.TimeoutMs == 0 {
		cfg.Replay.TimeoutMs = defaultTimeoutMs
	}
	if cfg.Service.MockEnvVar == "" {
		cfg.Service.MockEnvVar = defaultMockEnvVar
	}
	if cfg.Service.StartupTimeMs == 0 {
		cfg.Service.StartupTimeMs = defaultStartupTimeMs
	}

	return cfg, nil
}

// expandEnvVars expands environment variables in configuration values.
// Supports ${VAR_NAME} and $VAR_NAME syntax.
func (c *Config) expandEnvVars() {
	c.Service.Name = os.ExpandEnv(c.Service.Name)
	c.Service.BaseURL = os.ExpandEnv(c.Service.BaseURL)
	c.Service.Command = os.ExpandEnv(c.Service.Command)
	c.Service.MockEnvVar = os.ExpandEnv(c.Service.MockEnvVar)
	c.Database.ConnectionString = os.ExpandEnv(c.Database.ConnectionString)
	c.Recording.SnapshotDir = os.ExpandEnv(c.Recording.SnapshotDir)
	c.Recording.ProxyAuthToken = os.ExpandEnv(c.Recording.ProxyAuthToken)
	c.Replay.TestDatabase.ConnectionString = os.ExpandEnv(c.Replay.TestDatabase.ConnectionString)
}

func (c *Config) validate() error {
	if c.Service.Name == "" {
		return fmt.Errorf("service.name is required")
	}
	if c.Service.BaseURL == "" {
		return fmt.Errorf("service.base_url is required")
	}
	if c.Database.Type == "" {
		return fmt.Errorf("database.type is required")
	}
	switch c.Database.Type {
	case dbTypePostgres, dbTypeMySQL, dbTypeSQLite:
		// ok
	default:
		return fmt.Errorf("unsupported database type: %s (must be postgres, mysql, or sqlite)", c.Database.Type)
	}
	if c.Database.ConnectionString == "" {
		return fmt.Errorf("database.connection_string is required")
	}
	if c.Recording.Format != "" && c.Recording.Format != formatJSON && c.Recording.Format != formatYAML {
		return fmt.Errorf("recording.format must be json or yaml")
	}
	return nil
}
