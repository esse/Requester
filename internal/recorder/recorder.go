package recorder

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/snapshot"
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
	log.Printf("Outgoing capture proxy listening on %s (set HTTP_PROXY=http://%s on service)", outAddr, outAddr)

	addr := fmt.Sprintf(":%d", r.config.Recording.ProxyPort)
	log.Printf("Recording proxy listening on %s, forwarding to %s", addr, r.config.Service.BaseURL)
	log.Printf("Snapshots will be saved to %s", r.config.Recording.SnapshotDir)

	var handler http.Handler = r
	if r.config.Recording.ProxyAuthToken != "" {
		handler = r.withAuth(r.config.Recording.ProxyAuthToken, r)
		log.Printf("Proxy authentication enabled (Bearer token required)")
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
		log.Printf("ERROR: Failed to snapshot DB before request: %v", err)
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
		log.Printf("ERROR: Failed to snapshot DB after request: %v", err)
		return
	}

	// 7. Build snapshot
	snap := r.buildSnapshot(req, reqBody, recorder, dbBefore, dbAfter, outgoingRequests)

	// 8. Save snapshot
	path, err := r.store.Save(snap)
	if err != nil {
		log.Printf("ERROR: Failed to save snapshot: %v", err)
		return
	}

	outCount := len(outgoingRequests)
	log.Printf("Recorded: %s %s -> %d [%s] (%d outgoing request(s))", req.Method, req.URL.Path, recorder.statusCode, path, outCount)
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
	reqContentType := req.Header.Get("Content-Type")
	parsedReqBody := snapshot.ParseBody(reqBody, reqContentType)

	// Parse response body (handles JSON, text, and binary/RPC payloads like protobuf)
	respContentType := resp.Header().Get("Content-Type")
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

	return &snapshot.Snapshot{
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
}

// Close cleans up resources.
func (r *Recorder) Close() error {
	r.outgoingProxy.Stop()
	return r.snapshotter.Close()
}

// withAuth wraps a handler with Bearer token authentication.
func (r *Recorder) withAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="snapshot-tester"`)
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}
		const prefix = "Bearer "
		if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
			http.Error(w, "Invalid authorization scheme, expected Bearer", http.StatusUnauthorized)
			return
		}
		if auth[len(prefix):] != token {
			http.Error(w, "Invalid token", http.StatusForbidden)
			return
		}
		// Strip the auth header before proxying so it doesn't leak to the service
		req.Header.Del("Authorization")
		next.ServeHTTP(w, req)
	})
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
