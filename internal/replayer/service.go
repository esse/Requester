package replayer

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/esse/snapshot-tester/internal/config"
)

// managedService manages the lifecycle of a service started via config command.
type managedService struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// startService launches the service as a subprocess with the given environment
// variables injected. It waits for startupTimeMs before returning.
func startService(cfg *config.Config, extraEnv []string) (*managedService, error) {
	if cfg.Service.Command == "" {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cfg.Service.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cfg.Service.Command)
	}

	// Inherit current environment and add extras
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting service command %q: %w", cfg.Service.Command, err)
	}

	log.Printf("Started service (pid=%d): %s", cmd.Process.Pid, cfg.Service.Command)
	for _, env := range extraEnv {
		log.Printf("  env: %s", env)
	}

	// Wait for service startup
	startupTime := time.Duration(cfg.Service.StartupTimeMs) * time.Millisecond
	time.Sleep(startupTime)

	return &managedService{cmd: cmd, cancel: cancel}, nil
}

// Stop terminates the managed service.
func (s *managedService) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	// Wait briefly for graceful shutdown
	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Printf("Service stopped (pid=%d)", s.cmd.Process.Pid)
	case <-time.After(5 * time.Second):
		log.Printf("Service did not stop gracefully, killing (pid=%d)", s.cmd.Process.Pid)
		s.cmd.Process.Kill()
	}
}
