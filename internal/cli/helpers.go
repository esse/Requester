package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/esse/snapshot-tester/internal/config"
	dbpkg "github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

func newSnapshotterForUpdate(cfg *config.Config, connStr string) (dbpkg.Snapshotter, error) {
	return dbpkg.NewSnapshotter(cfg.Database.Type, connStr, cfg.Database.Tables)
}

func fireRequestForUpdate(cfg *config.Config, req snapshot.Request) (*snapshot.Response, error) {
	fullURL := cfg.Service.BaseURL + req.URL

	var bodyReader io.Reader
	if req.Body != nil {
		data, err := snapshot.DecodeBody(req.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequest(req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: time.Duration(cfg.Replay.TimeoutMs) * time.Millisecond,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		headers[k] = v[0]
	}

	var parsedBody any
	if len(respBody) > 0 {
		respContentType := resp.Header.Get("Content-Type")
		parsedBody = snapshot.ParseBody(respBody, respContentType)
	}

	return &snapshot.Response{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    parsedBody,
	}, nil
}

func computeDiffForUpdate(before, after map[string][]map[string]any) map[string]snapshot.TableDiff {
	return dbpkg.ComputeDiff(before, after)
}
