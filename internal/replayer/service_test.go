package replayer

import (
	"testing"

	"github.com/esse/snapshot-tester/internal/config"
)

func TestStartService_EmptyCommand(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Command:       "",
			StartupTimeMs: 10,
		},
	}

	svc, err := startService(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc != nil {
		t.Error("expected nil service for empty command")
	}
}

func TestStartService_ShortLivedCommand(t *testing.T) {
	// Even if the command exits quickly, Start() succeeds because sh starts fine.
	// This verifies the service can be started and stopped cleanly for short-lived processes.
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Command:       "echo hello",
			StartupTimeMs: 10,
		},
	}

	svc, err := startService(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	svc.Stop()
}

func TestStartService_WithExtraEnv(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Command:       "sleep 0.01",
			StartupTimeMs: 50,
		},
	}

	svc, err := startService(cfg, []string{"TEST_VAR=test_value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	defer svc.Stop()

	if svc.cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestManagedService_StopNil(t *testing.T) {
	// Stop on nil should not panic
	var svc *managedService
	svc.Stop() // should be a no-op
}

func TestStartService_StopGracefully(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Command:       "sleep 10",
			StartupTimeMs: 10,
		},
	}

	svc, err := startService(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	// Stop should terminate the process
	svc.Stop()
}
