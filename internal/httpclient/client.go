package httpclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

// FireRequest sends an HTTP request to the given base URL and returns the parsed response.
// This is the shared implementation used by both the replayer and the CLI update command.
func FireRequest(baseURL string, req snapshot.Request, timeoutMs int) (*snapshot.Response, error) {
	fullURL := baseURL + req.URL

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
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
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

	headers := make(map[string]string)
	for k, v := range resp.Header {
		headers[k] = v[0]
	}

	var parsedBody any
	if len(respBody) > 0 {
		respContentType := resp.Header.Get(snapshot.HeaderContentType)
		parsedBody = snapshot.ParseBody(respBody, respContentType)
	}

	return &snapshot.Response{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    parsedBody,
	}, nil
}
