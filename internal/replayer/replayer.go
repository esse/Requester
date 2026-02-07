package replayer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/esse/snapshot-tester/internal/asserter"
	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/db"
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
	if len(snap.OutgoingRequests) > 0 {
		mockServer = mock.NewServer(snap.OutgoingRequests)
		addr, err := mockServer.Start()
		if err != nil {
			result.Error = fmt.Sprintf("Failed to start mock server: %v", err)
			result.Duration = time.Since(start)
			return result
		}
		defer mockServer.Stop()
		_ = addr // Mock server address available for service configuration
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
func (r *Replayer) ReplayAll(snapshots []*snapshot.Snapshot, paths []string) []TestResult {
	results := make([]TestResult, len(snapshots))
	for i, snap := range snapshots {
		results[i] = r.ReplayOne(snap, paths[i])
	}
	return results
}

// Close cleans up resources.
func (r *Replayer) Close() error {
	return r.snapshotter.Close()
}

type capturedResponse struct {
	Status  int
	Headers map[string]string
	Body    any
}

func (r *Replayer) fireRequest(req snapshot.Request) (*capturedResponse, error) {
	fullURL := r.config.Service.BaseURL + req.URL

	// Decode body using snapshot encoding (handles JSON, text, and binary/RPC payloads)
	var bodyReader io.Reader
	if req.Body != nil {
		data, err := snapshot.DecodeBody(req.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequest(req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: time.Duration(r.config.Replay.TimeoutMs) * time.Millisecond,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	captured := &capturedResponse{
		Status:  resp.StatusCode,
		Headers: make(map[string]string),
	}

	for k, v := range resp.Header {
		captured.Headers[k] = v[0]
	}

	// Parse response body using content-type-aware encoding
	if len(respBody) > 0 {
		respContentType := resp.Header.Get("Content-Type")
		captured.Body = snapshot.ParseBody(respBody, respContentType)
	}

	return captured, nil
}
