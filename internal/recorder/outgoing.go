package recorder

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

// OutgoingProxy is a forward HTTP proxy that captures outgoing requests made
// by the service under test. During recording, the service should be configured
// to route its outgoing HTTP traffic through this proxy (e.g., via HTTP_PROXY).
type OutgoingProxy struct {
	mu            sync.Mutex
	calls         []snapshot.OutgoingRequest
	listener      net.Listener
	server        *http.Server
	ignoreHeaders map[string]bool
	client        *http.Client
}

// NewOutgoingProxy creates a forward proxy that captures outgoing HTTP requests.
func NewOutgoingProxy(ignoreHeaders []string) *OutgoingProxy {
	ignore := make(map[string]bool)
	for _, h := range ignoreHeaders {
		ignore[strings.ToLower(h)] = true
	}
	return &OutgoingProxy{
		ignoreHeaders: ignore,
		client:        &http.Client{},
	}
}

// Start launches the outgoing proxy. If port is 0, a random port is chosen.
// Returns the listener address (e.g., "127.0.0.1:12345").
func (p *OutgoingProxy) Start(port int) (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var err error
	p.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("starting outgoing proxy: %w", err)
	}

	p.server = &http.Server{Handler: p}
	go p.server.Serve(p.listener)

	return p.listener.Addr().String(), nil
}

// Stop shuts down the outgoing proxy.
func (p *OutgoingProxy) Stop() {
	if p.server != nil {
		p.server.Close()
	}
}

// Drain returns all captured outgoing requests and resets the internal buffer.
// This should be called after each incoming request cycle to collect the
// outgoing requests associated with that incoming request.
func (p *OutgoingProxy) Drain() []snapshot.OutgoingRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := p.calls
	p.calls = nil
	return calls
}

// ServeHTTP handles forward proxy requests. It forwards the request to the
// actual destination, captures both the request and response, and stores them.
func (p *OutgoingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CONNECT method (HTTPS tunneling) is not supported for capture
	if r.Method == http.MethodConnect {
		http.Error(w, "HTTPS tunneling (CONNECT) not supported for outgoing capture; use plain HTTP", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	var reqBodyRaw []byte
	if r.Body != nil {
		var err error
		reqBodyRaw, err = io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed to read request body", "component", "outgoing_proxy", "error", err)
			http.Error(w, "failed to read request body", http.StatusBadGateway)
			return
		}
	}

	// Build the outgoing request to the actual destination
	targetURL := r.URL.String()
	if !r.URL.IsAbs() {
		// If the URL is not absolute, construct from Host header
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		targetURL = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.RequestURI())
	}

	outReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(reqBodyRaw))
	if err != nil {
		slog.Error("failed to create forwarded request", "component", "outgoing_proxy", "error", err)
		http.Error(w, "failed to create request", http.StatusBadGateway)
		return
	}

	// Copy headers, skip hop-by-hop headers and proxy headers
	for k, vv := range r.Header {
		lower := strings.ToLower(k)
		if isHopByHopHeader(lower) {
			continue
		}
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}

	// Forward the request
	resp, err := p.client.Do(outReq)
	if err != nil {
		slog.Error("failed to forward request", "component", "outgoing_proxy", "url", targetURL, "error", err)
		http.Error(w, fmt.Sprintf("failed to reach upstream: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBodyRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read response body", "component", "outgoing_proxy", "error", err)
		http.Error(w, "failed to read response body", http.StatusBadGateway)
		return
	}

	// Build captured headers (filtering ignored ones)
	reqHeaders := p.filterHeaders(r.Header)
	respHeaders := p.filterHeaders(resp.Header)

	// Parse bodies using content-type-aware encoding
	reqContentType := r.Header.Get(snapshot.HeaderContentType)
	parsedReqBody := snapshot.ParseBody(reqBodyRaw, reqContentType)

	respContentType := resp.Header.Get(snapshot.HeaderContentType)
	parsedRespBody := snapshot.ParseBody(respBodyRaw, respContentType)

	// Record the outgoing request
	outgoing := snapshot.OutgoingRequest{
		Method:  r.Method,
		URL:     r.URL.RequestURI(),
		Headers: reqHeaders,
		Body:    parsedReqBody,
		Response: &snapshot.Response{
			Status:  resp.StatusCode,
			Headers: respHeaders,
			Body:    parsedRespBody,
		},
	}

	p.mu.Lock()
	p.calls = append(p.calls, outgoing)
	p.mu.Unlock()

	slog.Debug("outgoing request captured", "method", r.Method, "url", r.URL.RequestURI(), "status", resp.StatusCode)

	// Write response back to the service
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBodyRaw)
}

func (p *OutgoingProxy) filterHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if !p.ignoreHeaders[strings.ToLower(k)] {
			result[k] = strings.Join(v, ", ")
		}
	}
	return result
}

func isHopByHopHeader(h string) bool {
	switch h {
	case "connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer",
		"transfer-encoding", "upgrade":
		return true
	}
	return false
}
