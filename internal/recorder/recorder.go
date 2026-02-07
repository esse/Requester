package recorder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/snapshot"
	"golang.org/x/time/rate"
)

// Recorder is the recording proxy that intercepts traffic and creates snapshots.
type Recorder struct {
	config        *config.Config
	snapshotter   db.Snapshotter
	store         *snapshot.Store
	proxy         *httputil.ReverseProxy
	tags          []string
	outgoingProxy *OutgoingProxy
}

// New creates a new Recorder.
func New(cfg *config.Config, tags []string) (*Recorder, error) {
	snapshotter, err := db.NewSnapshotter(cfg.Database.Type, cfg.Database.ConnectionString, cfg.Database.Tables)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	store := snapshot.NewStore(cfg.Recording.SnapshotDir, cfg.Recording.Format)

	target, err := url.Parse(cfg.Service.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing service base URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	outgoingProxy := NewOutgoingProxy(cfg.Recording.IgnoreHeaders)

	return &Recorder{
		config:        cfg,
		snapshotter:   snapshotter,
		store:         store,
		proxy:         proxy,
		tags:          tags,
		outgoingProxy: outgoingProxy,
	}, nil
}

// Start begins the recording proxy on the configured port.
func (r *Recorder) Start() error {
	// Start outgoing capture proxy
	outAddr, err := r.outgoingProxy.Start(r.config.Recording.OutgoingProxyPort)
	if err != nil {
		return fmt.Errorf("starting outgoing proxy: %w", err)
	}
	defer r.outgoingProxy.Stop()
	slog.Info("outgoing capture proxy started", "addr", outAddr, "hint", "set HTTP_PROXY=http://"+outAddr+" on service")

	addr := fmt.Sprintf(":%d", r.config.Recording.ProxyPort)
	slog.Info("recording proxy started", "addr", addr, "target", r.config.Service.BaseURL)
	slog.Info("snapshot directory configured", "dir", r.config.Recording.SnapshotDir)

	var handler http.Handler = r

	// Apply rate limiting if configured
	rl := r.config.Recording.RateLimit
	if rl.RequestsPerSecond > 0 || rl.MaxConcurrent > 0 {
		handler = r.withRateLimit(rl, handler)
		slog.Info("rate limiting enabled", "rps", rl.RequestsPerSecond, "max_concurrent", rl.MaxConcurrent)
	}

	if r.config.Recording.ProxyAuthToken != "" {
		handler = r.withAuth(r.config.Recording.ProxyAuthToken, handler)
		slog.Info("proxy authentication enabled")
	}

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	return server.ListenAndServe()
}

// ServeHTTP handles each proxied request.
func (r *Recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 1. Read request body
	var reqBody []byte
	if req.Body != nil {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	// 2. Snapshot DB before
	dbBefore, err := r.snapshotter.SnapshotAll()
	if err != nil {
		slog.Error("failed to snapshot DB before request", "error", err)
		http.Error(w, "Failed to snapshot database", http.StatusInternalServerError)
		return
	}

	// 3. Drain any stale outgoing requests before proxying
	r.outgoingProxy.Drain()

	// 4. Proxy the request and capture the response
	recorder := &responseRecorder{
		ResponseWriter: w,
		statusCode:     200,
	}

	r.proxy.ServeHTTP(recorder, req)

	// 5. Collect outgoing requests made by the service during this request
	outgoingRequests := r.outgoingProxy.Drain()

	// 6. Snapshot DB after
	dbAfter, err := r.snapshotter.SnapshotAll()
	if err != nil {
		slog.Error("failed to snapshot DB after request", "error", err)
		return
	}

	// 7. Build snapshot
	snap := r.buildSnapshot(req, reqBody, recorder, dbBefore, dbAfter, outgoingRequests)

	// 8. Save snapshot
	path, err := r.store.Save(snap)
	if err != nil {
		slog.Error("failed to save snapshot", "error", err)
		return
	}

	outCount := len(outgoingRequests)
	slog.Info("snapshot recorded", "method", req.Method, "path", req.URL.Path, "status", recorder.statusCode, "file", path, "outgoing_count", outCount)
}

func (r *Recorder) buildSnapshot(req *http.Request, reqBody []byte, resp *responseRecorder, dbBefore, dbAfter map[string][]map[string]any, outgoingRequests []snapshot.OutgoingRequest) *snapshot.Snapshot {
	// Build request headers (filtering ignored ones)
	headers := make(map[string]string)
	ignoreSet := make(map[string]bool)
	for _, h := range r.config.Recording.IgnoreHeaders {
		ignoreSet[strings.ToLower(h)] = true
	}
	for k, v := range req.Header {
		if !ignoreSet[strings.ToLower(k)] {
			headers[k] = strings.Join(v, ", ")
		}
	}

	// Parse request body (handles JSON, text, and binary/RPC payloads like protobuf)
	reqContentType := req.Header.Get(snapshot.HeaderContentType)
	parsedReqBody := snapshot.ParseBody(reqBody, reqContentType)

	// Parse response body (handles JSON, text, and binary/RPC payloads like protobuf)
	respContentType := resp.Header().Get(snapshot.HeaderContentType)
	parsedRespBody := snapshot.ParseBody(resp.body, respContentType)

	// Response headers
	respHeaders := make(map[string]string)
	for k, v := range resp.Header() {
		if !ignoreSet[strings.ToLower(k)] {
			respHeaders[k] = strings.Join(v, ", ")
		}
	}

	// Compute diff
	dbDiff := db.ComputeDiff(dbBefore, dbAfter)

	snap := &snapshot.Snapshot{
		ID:        snapshot.GenerateID(),
		Timestamp: time.Now().UTC(),
		Service:   r.config.Service.Name,
		Tags:      r.tags,
		DBStateBefore: dbBefore,
		Request: snapshot.Request{
			Method:  req.Method,
			URL:     req.URL.RequestURI(),
			Headers: headers,
			Body:    parsedReqBody,
		},
		OutgoingRequests: outgoingRequests,
		Response: snapshot.Response{
			Status:  resp.statusCode,
			Headers: respHeaders,
			Body:    parsedRespBody,
		},
		DBStateAfter: dbAfter,
		DBDiff:       dbDiff,
	}

	// Apply field-level redaction if configured
	if len(r.config.Recording.RedactFields) > 0 {
		redactSnapshot(snap, r.config.Recording.RedactFields)
	}

	return snap
}

// Close cleans up resources.
func (r *Recorder) Close() error {
	r.outgoingProxy.Stop()
	return r.snapshotter.Close()
}

// withAuth wraps a handler with Bearer token authentication.
func (r *Recorder) withAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get(snapshot.HeaderAuthorization)
		if auth == "" {
			w.Header().Set(snapshot.HeaderWWWAuthenticate, `Bearer realm="snapshot-tester"`)
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}
		if len(auth) < len(snapshot.AuthSchemeBearer) || !strings.EqualFold(auth[:len(snapshot.AuthSchemeBearer)], snapshot.AuthSchemeBearer) {
			http.Error(w, "Invalid authorization scheme, expected Bearer", http.StatusUnauthorized)
			return
		}
		if auth[len(snapshot.AuthSchemeBearer):] != token {
			http.Error(w, "Invalid token", http.StatusForbidden)
			return
		}
		// Strip the auth header before proxying so it doesn't leak to the service
		req.Header.Del(snapshot.HeaderAuthorization)
		next.ServeHTTP(w, req)
	})
}

// withRateLimit wraps a handler with rate limiting using a token bucket and concurrency semaphore.
func (r *Recorder) withRateLimit(cfg config.RateLimitConfig, next http.Handler) http.Handler {
	var limiter *rate.Limiter
	if cfg.RequestsPerSecond > 0 {
		limiter = rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), int(cfg.RequestsPerSecond))
	}

	var sem chan struct{}
	if cfg.MaxConcurrent > 0 {
		sem = make(chan struct{}, cfg.MaxConcurrent)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Check rate limit
		if limiter != nil {
			if err := limiter.Wait(context.Background()); err != nil {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}

		// Check concurrency limit
		if sem != nil {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			default:
				http.Error(w, "Too many concurrent requests", http.StatusServiceUnavailable)
				return
			}
		}

		next.ServeHTTP(w, req)
	})
}

const redactedValue = "[REDACTED]"

// redactSnapshot replaces sensitive field values with [REDACTED] in a snapshot.
// Supports paths like "request.headers.Authorization", "response.body.password",
// and wildcard paths like "*.password" that match at any depth.
func redactSnapshot(snap *snapshot.Snapshot, fields []string) {
	for _, field := range fields {
		parts := strings.Split(field, ".")
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {
		case "request":
			redactInRequest(&snap.Request, parts[1:])
		case "response":
			redactInResponse(&snap.Response, parts[1:])
		case "*":
			// Wildcard: redact in both request and response bodies and headers
			redactInRequest(&snap.Request, parts[1:])
			redactInResponse(&snap.Response, parts[1:])
			// Also redact in outgoing requests
			for i := range snap.OutgoingRequests {
				subReq := snapshot.Request{
					Method:  snap.OutgoingRequests[i].Method,
					URL:     snap.OutgoingRequests[i].URL,
					Headers: snap.OutgoingRequests[i].Headers,
					Body:    snap.OutgoingRequests[i].Body,
				}
				redactInRequest(&subReq, parts[1:])
				snap.OutgoingRequests[i].Headers = subReq.Headers
				snap.OutgoingRequests[i].Body = subReq.Body
				if snap.OutgoingRequests[i].Response != nil {
					redactInResponse(snap.OutgoingRequests[i].Response, parts[1:])
				}
			}
		}
	}
}

func redactInRequest(req *snapshot.Request, path []string) {
	if len(path) == 0 {
		return
	}
	switch path[0] {
	case "headers":
		if len(path) == 2 && req.Headers != nil {
			if _, ok := req.Headers[path[1]]; ok {
				req.Headers[path[1]] = redactedValue
			}
		}
	case "body":
		if len(path) >= 2 {
			req.Body = redactInBody(req.Body, path[1:])
		}
	default:
		// Treat as a body field name at any depth
		req.Body = redactFieldRecursive(req.Body, path[0])
		if req.Headers != nil {
			if _, ok := req.Headers[path[0]]; ok {
				req.Headers[path[0]] = redactedValue
			}
		}
	}
}

func redactInResponse(resp *snapshot.Response, path []string) {
	if len(path) == 0 {
		return
	}
	switch path[0] {
	case "headers":
		if len(path) == 2 && resp.Headers != nil {
			if _, ok := resp.Headers[path[1]]; ok {
				resp.Headers[path[1]] = redactedValue
			}
		}
	case "body":
		if len(path) >= 2 {
			resp.Body = redactInBody(resp.Body, path[1:])
		}
	default:
		resp.Body = redactFieldRecursive(resp.Body, path[0])
		if resp.Headers != nil {
			if _, ok := resp.Headers[path[0]]; ok {
				resp.Headers[path[0]] = redactedValue
			}
		}
	}
}

func redactInBody(body any, path []string) any {
	if body == nil || len(path) == 0 {
		return body
	}
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if len(path) == 1 {
		if _, exists := m[path[0]]; exists {
			m[path[0]] = redactedValue
		}
		return m
	}
	if nested, exists := m[path[0]]; exists {
		m[path[0]] = redactInBody(nested, path[1:])
	}
	return m
}

func redactFieldRecursive(body any, fieldName string) any {
	if body == nil {
		return body
	}
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if _, exists := m[fieldName]; exists {
		m[fieldName] = redactedValue
	}
	for k, v := range m {
		if nested, isMap := v.(map[string]any); isMap {
			m[k] = redactFieldRecursive(nested, fieldName)
		}
	}
	return m
}

// responseRecorder captures the response for snapshot storage while also writing to the client.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body = append(rr.body, b...)
	return rr.ResponseWriter.Write(b)
}
