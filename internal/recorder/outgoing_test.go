package recorder

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOutgoingProxy_CapturesRequests(t *testing.T) {
	// Start a target server that the proxy will forward to
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
	}))
	defer target.Close()

	// Start the outgoing proxy
	proxy := NewOutgoingProxy([]string{"Authorization"})
	addr, err := proxy.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()

	// Create an HTTP client that uses the outgoing proxy
	proxyURL, _ := url.Parse("http://" + addr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	// Make a request through the proxy to the target server
	resp, err := client.Post(target.URL+"/api/send", "application/json", strings.NewReader(`{"to":"user@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if parsed["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", parsed["result"])
	}

	// Verify the proxy captured the request
	calls := proxy.Drain()
	if len(calls) != 1 {
		t.Fatalf("expected 1 captured call, got %d", len(calls))
	}

	call := calls[0]
	if call.Method != "POST" {
		t.Errorf("expected method POST, got %s", call.Method)
	}
	if call.Response == nil {
		t.Fatal("expected response to be captured")
	}
	if call.Response.Status != 200 {
		t.Errorf("expected response status 200, got %d", call.Response.Status)
	}

	// Verify Authorization header was filtered
	if _, hasAuth := call.Headers["Authorization"]; hasAuth {
		t.Error("expected Authorization header to be filtered")
	}
}

func TestOutgoingProxy_DrainClearsBuffer(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer target.Close()

	proxy := NewOutgoingProxy(nil)
	addr, err := proxy.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()

	proxyURL, _ := url.Parse("http://" + addr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	// Make two requests
	resp1, err := client.Get(target.URL + "/first")
	if err != nil {
		t.Fatal(err)
	}
	resp1.Body.Close()

	resp2, err := client.Get(target.URL + "/second")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// First drain should return 2 calls
	calls := proxy.Drain()
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}

	// Second drain should return 0 calls
	calls = proxy.Drain()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls after drain, got %d", len(calls))
	}
}

func TestOutgoingProxy_ConnectRejected(t *testing.T) {
	proxy := NewOutgoingProxy(nil)
	addr, err := proxy.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()

	// Send a CONNECT request directly
	req, _ := http.NewRequest(http.MethodConnect, "http://"+addr, nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for CONNECT, got %d", resp.StatusCode)
	}
}
