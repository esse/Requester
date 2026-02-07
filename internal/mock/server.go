package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

// Server intercepts outgoing HTTP calls during replay and returns recorded responses.
type Server struct {
	expectations map[string]*snapshot.OutgoingRequest
	calls        []RecordedCall
	mu           sync.Mutex
	listener     net.Listener
	server       *http.Server
}

// RecordedCall tracks an intercepted outgoing call for recording mode.
type RecordedCall struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     any
	Response *snapshot.Response
}

// NewServer creates a mock server loaded with expected outgoing requests.
func NewServer(outgoing []snapshot.OutgoingRequest) *Server {
	expectations := make(map[string]*snapshot.OutgoingRequest)
	for i := range outgoing {
		key := requestKey(outgoing[i].Method, outgoing[i].URL)
		expectations[key] = &outgoing[i]
	}
	return &Server{expectations: expectations}
}

// Start launches the mock server on a random port and returns the address.
func (s *Server) Start() (string, error) {
	var err error
	s.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("starting mock server: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.server = &http.Server{Handler: mux}
	go s.server.Serve(s.listener)

	return s.listener.Addr().String(), nil
}

// Stop shuts down the mock server.
func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}

// Calls returns all calls that were made to the mock server.
func (s *Server) Calls() []RecordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RecordedCall{}, s.calls...)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Read body
	var body any
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			if err := json.Unmarshal(data, &body); err != nil {
				body = string(data)
			}
		}
	}

	// Build headers map
	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}

	// Look up expectation
	// Try full URL first, then just path
	key := requestKey(r.Method, r.URL.String())
	exp, ok := s.expectations[key]
	if !ok {
		// Try matching by method + path only
		for eKey, eVal := range s.expectations {
			if strings.HasPrefix(eKey, r.Method+":") && strings.HasSuffix(eKey, r.URL.Path) {
				exp = eVal
				ok = true
				break
			}
		}
	}

	call := RecordedCall{
		Method:  r.Method,
		URL:     r.URL.String(),
		Headers: headers,
		Body:    body,
	}

	if ok && exp.Response != nil {
		call.Response = exp.Response
		s.calls = append(s.calls, call)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(exp.Response.Status)
		if exp.Response.Body != nil {
			data, _ := json.Marshal(exp.Response.Body)
			w.Write(data)
		}
	} else {
		log.Printf("MOCK: Unexpected outgoing request: %s %s", r.Method, r.URL.String())
		s.calls = append(s.calls, call)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error": "no mock expectation matched"}`))
	}
}

func requestKey(method, url string) string {
	return method + ":" + url
}
