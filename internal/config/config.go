package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration for snapshot-tester.
type Config struct {
	Service   ServiceConfig   `yaml:"service"`
	Database  DatabaseConfig  `yaml:"database"`
	Recording RecordingConfig `yaml:"recording"`
	Replay    ReplayConfig    `yaml:"replay"`
}

type ServiceConfig struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base_url"`
}

type DatabaseConfig struct {
	Type             string   `yaml:"type"` // postgres | mysql | sqlite
	ConnectionString string   `yaml:"connection_string"`
	Tables           []string `yaml:"tables"`
}

type RecordingConfig struct {
	ProxyPort     int      `yaml:"proxy_port"`
	SnapshotDir   string   `yaml:"snapshot_dir"`
	Format        string   `yaml:"format"` // json | yaml
	IgnoreHeaders []string `yaml:"ignore_headers"`
	IgnoreFields  []string `yaml:"ignore_fields"`
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
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply defaults
	if cfg.Recording.SnapshotDir == "" {
		cfg.Recording.SnapshotDir = "./snapshots"
	}
	if cfg.Recording.Format == "" {
		cfg.Recording.Format = "json"
	}
	if cfg.Recording.ProxyPort == 0 {
		cfg.Recording.ProxyPort = 8080
	}
	if cfg.Replay.TimeoutMs == 0 {
		cfg.Replay.TimeoutMs = 5000
	}

	return cfg, nil
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
	case "postgres", "mysql", "sqlite":
		// ok
	default:
		return fmt.Errorf("unsupported database type: %s (must be postgres, mysql, or sqlite)", c.Database.Type)
	}
	if c.Database.ConnectionString == "" {
		return fmt.Errorf("database.connection_string is required")
	}
	if c.Recording.Format != "" && c.Recording.Format != "json" && c.Recording.Format != "yaml" {
		return fmt.Errorf("recording.format must be json or yaml")
	}
	return nil
}
