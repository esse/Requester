package snapshot

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Snapshot represents a complete recording of a single service interaction.
type Snapshot struct {
	ID               string                       `json:"id" yaml:"id"`
	Timestamp        time.Time                    `json:"timestamp" yaml:"timestamp"`
	Service          string                       `json:"service" yaml:"service"`
	Tags             []string                     `json:"tags,omitempty" yaml:"tags,omitempty"`
	DBStateBefore    map[string][]map[string]any  `json:"db_state_before" yaml:"db_state_before"`
	Request          Request                      `json:"request" yaml:"request"`
	OutgoingRequests []OutgoingRequest            `json:"outgoing_requests,omitempty" yaml:"outgoing_requests,omitempty"`
	Response         Response                     `json:"response" yaml:"response"`
	DBStateAfter     map[string][]map[string]any  `json:"db_state_after" yaml:"db_state_after"`
	DBDiff           map[string]TableDiff         `json:"db_diff" yaml:"db_diff"`
}

// Request represents the incoming HTTP request.
type Request struct {
	Method  string            `json:"method" yaml:"method"`
	URL     string            `json:"url" yaml:"url"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body    any               `json:"body,omitempty" yaml:"body,omitempty"`
}

// Response represents the HTTP response from the service.
type Response struct {
	Status  int               `json:"status" yaml:"status"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body    any               `json:"body,omitempty" yaml:"body,omitempty"`
}

// OutgoingRequest represents an outgoing HTTP call made by the service.
type OutgoingRequest struct {
	Method   string            `json:"method" yaml:"method"`
	URL      string            `json:"url" yaml:"url"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body     any               `json:"body,omitempty" yaml:"body,omitempty"`
	Response *Response         `json:"response,omitempty" yaml:"response,omitempty"`
}

// TableDiff represents changes to a single database table.
type TableDiff struct {
	Added    []map[string]any `json:"added" yaml:"added"`
	Removed  []map[string]any `json:"removed" yaml:"removed"`
	Modified []ModifiedRow    `json:"modified" yaml:"modified"`
}

// ModifiedRow represents a row that was changed.
type ModifiedRow struct {
	Before map[string]any `json:"before" yaml:"before"`
	After  map[string]any `json:"after" yaml:"after"`
}

// GenerateID creates a random hex ID.
func GenerateID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
