package replayer

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/esse/snapshot-tester/internal/asserter"
	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/httpclient"
	"github.com/esse/snapshot-tester/internal/mock"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

// TestResult represents the result of replaying a single snapshot.
type TestResult struct {
	SnapshotID   string
	SnapshotPath string
	Passed       bool
	Diffs        []asserter.Diff
	Duration     time.Duration
	Error        string
}

// Replayer replays snapshots against a running service.
type Replayer struct {
	config      *config.Config
	snapshotter db.Snapshotter
}

// New creates a new Replayer.
func New(cfg *config.Config) (*Replayer, error) {
	connStr := cfg.Database.ConnectionString
	if cfg.Replay.TestDatabase.ConnectionString != "" {
		connStr = cfg.Replay.TestDatabase.ConnectionString
	}

	snapshotter, err := db.NewSnapshotter(cfg.Database.Type, connStr, cfg.Database.Tables)
	if err != nil {
		return nil, fmt.Errorf("connecting to test database: %w", err)
	}

	return &Replayer{
		config:      cfg,
		snapshotter: snapshotter,
	}, nil
}

// ReplayOne replays a single snapshot and returns the result.
func (r *Replayer) ReplayOne(snap *snapshot.Snapshot, path string) TestResult {
	start := time.Now()
	result := TestResult{
		SnapshotID:   snap.ID,
		SnapshotPath: path,
	}

	// 1. Restore db_state_before
	if err := r.snapshotter.RestoreAll(snap.DBStateBefore); err != nil {
		result.Error = fmt.Sprintf("Failed to restore DB state: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// 2. Start mock server if there are outgoing requests
	var mockServer *mock.Server
	var svc *managedService
	if len(snap.OutgoingRequests) > 0 {
		mockServer = mock.NewServer(snap.OutgoingRequests)
		addr, err := mockServer.Start()
		if err != nil {
			result.Error = fmt.Sprintf("Failed to start mock server: %v", err)
			result.Duration = time.Since(start)
			return result
		}
		defer mockServer.Stop()

		mockURL := fmt.Sprintf("http://%s", addr)
		envVar := r.config.Service.MockEnvVar
		log.Printf("Mock server at %s (injecting as %s=%s)", mockURL, envVar, mockURL)

		// If a service command is configured, start the service with the mock URL injected
		if r.config.Service.Command != "" {
			svc, err = startService(r.config, []string{
				fmt.Sprintf("%s=%s", envVar, mockURL),
			})
			if err != nil {
				result.Error = fmt.Sprintf("Failed to start service: %v", err)
				result.Duration = time.Since(start)
				return result
			}
			defer svc.Stop()
		}
	}

	// 3. Fire the request
	actualResp, err := r.fireRequest(snap.Request)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to send request: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// 4. Snapshot DB after
	actualDBAfter, err := r.snapshotter.SnapshotAll()
	if err != nil {
		result.Error = fmt.Sprintf("Failed to snapshot DB after: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// 5. Compare response
	opts := &asserter.Options{
		IgnoreFields: r.config.Recording.IgnoreFields,
	}

	expectedResp := map[string]any{
		"status": snap.Response.Status,
		"body":   snap.Response.Body,
	}
	actualRespMap := map[string]any{
		"status": actualResp.Status,
		"body":   actualResp.Body,
	}

	respDiffs := asserter.AssertResponse(expectedResp, actualRespMap, opts)
	dbDiffs := asserter.AssertDBState(snap.DBStateAfter, actualDBAfter, opts)

	result.Diffs = append(respDiffs, dbDiffs...)
	result.Passed = len(result.Diffs) == 0
	result.Duration = time.Since(start)

	return result
}

// ReplayAll replays multiple snapshots and returns all results.
// If config.Replay.Parallel is true, snapshots are replayed concurrently.
func (r *Replayer) ReplayAll(snapshots []*snapshot.Snapshot, paths []string) []TestResult {
	results := make([]TestResult, len(snapshots))

	if r.config.Replay.Parallel && len(snapshots) > 1 {
		var wg sync.WaitGroup
		wg.Add(len(snapshots))
		for i, snap := range snapshots {
			go func(idx int, s *snapshot.Snapshot, p string) {
				defer wg.Done()
				results[idx] = r.ReplayOne(s, p)
			}(i, snap, paths[i])
		}
		wg.Wait()
	} else {
		for i, snap := range snapshots {
			results[i] = r.ReplayOne(snap, paths[i])
		}
	}

	return results
}

// Close cleans up resources.
func (r *Replayer) Close() error {
	return r.snapshotter.Close()
}

func (r *Replayer) fireRequest(req snapshot.Request) (*snapshot.Response, error) {
	return httpclient.FireRequest(r.config.Service.BaseURL, req, r.config.Replay.TimeoutMs)
}
