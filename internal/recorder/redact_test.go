package recorder

import (
	"testing"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

func TestRedactSnapshot_RequestHeader(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method:  "POST",
			URL:     "/api/login",
			Headers: map[string]string{"Authorization": "Bearer secret-token", "Content-Type": "application/json"},
			Body:    map[string]any{"user": "alice"},
		},
		Response: snapshot.Response{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"token": "abc123"},
		},
	}

	redactSnapshot(snap, []string{"request.headers.Authorization"})

	if snap.Request.Headers["Authorization"] != redactedValue {
		t.Errorf("expected Authorization to be redacted, got %q", snap.Request.Headers["Authorization"])
	}
	if snap.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type to be preserved, got %q", snap.Request.Headers["Content-Type"])
	}
}

func TestRedactSnapshot_ResponseBodyField(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method: "GET",
			URL:    "/api/user",
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"name": "Alice", "ssn": "123-45-6789"},
		},
	}

	redactSnapshot(snap, []string{"response.body.ssn"})

	body := snap.Response.Body.(map[string]any)
	if body["ssn"] != redactedValue {
		t.Errorf("expected ssn to be redacted, got %v", body["ssn"])
	}
	if body["name"] != "Alice" {
		t.Errorf("expected name to be preserved, got %v", body["name"])
	}
}

func TestRedactSnapshot_WildcardField(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method: "POST",
			URL:    "/api/register",
			Body:   map[string]any{"username": "alice", "password": "secret123"},
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"message": "ok", "password": "should-also-redact"},
		},
	}

	redactSnapshot(snap, []string{"*.password"})

	reqBody := snap.Request.Body.(map[string]any)
	if reqBody["password"] != redactedValue {
		t.Errorf("expected request password to be redacted, got %v", reqBody["password"])
	}

	respBody := snap.Response.Body.(map[string]any)
	if respBody["password"] != redactedValue {
		t.Errorf("expected response password to be redacted, got %v", respBody["password"])
	}
}

func TestRedactSnapshot_NestedBodyField(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method: "POST",
			URL:    "/api/data",
			Body: map[string]any{
				"user": map[string]any{
					"name":     "Alice",
					"password": "secret",
				},
			},
		},
		Response: snapshot.Response{
			Status: 200,
		},
	}

	redactSnapshot(snap, []string{"request.body.user.password"})

	body := snap.Request.Body.(map[string]any)
	user := body["user"].(map[string]any)
	if user["password"] != redactedValue {
		t.Errorf("expected nested password to be redacted, got %v", user["password"])
	}
	if user["name"] != "Alice" {
		t.Errorf("expected name to be preserved, got %v", user["name"])
	}
}

func TestRedactSnapshot_WildcardRecursive(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method: "POST",
			URL:    "/api/data",
			Body: map[string]any{
				"data": map[string]any{
					"nested": map[string]any{
						"secret": "top-secret-value",
					},
				},
				"secret": "also-secret",
			},
		},
		Response: snapshot.Response{
			Status: 200,
		},
	}

	redactSnapshot(snap, []string{"*.secret"})

	body := snap.Request.Body.(map[string]any)
	if body["secret"] != redactedValue {
		t.Errorf("expected top-level secret to be redacted, got %v", body["secret"])
	}
	nested := body["data"].(map[string]any)["nested"].(map[string]any)
	if nested["secret"] != redactedValue {
		t.Errorf("expected deeply nested secret to be redacted, got %v", nested["secret"])
	}
}

func TestRedactSnapshot_OutgoingRequests(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method: "GET",
			URL:    "/api/fetch",
		},
		Response: snapshot.Response{
			Status: 200,
		},
		OutgoingRequests: []snapshot.OutgoingRequest{
			{
				Method:  "GET",
				URL:     "/external/api",
				Headers: map[string]string{"Authorization": "Bearer ext-token"},
				Body:    map[string]any{"api_key": "key123"},
				Response: &snapshot.Response{
					Status: 200,
					Body:   map[string]any{"api_key": "response-key"},
				},
			},
		},
	}

	redactSnapshot(snap, []string{"*.api_key"})

	outBody := snap.OutgoingRequests[0].Body.(map[string]any)
	if outBody["api_key"] != redactedValue {
		t.Errorf("expected outgoing request api_key to be redacted, got %v", outBody["api_key"])
	}
	respBody := snap.OutgoingRequests[0].Response.Body.(map[string]any)
	if respBody["api_key"] != redactedValue {
		t.Errorf("expected outgoing response api_key to be redacted, got %v", respBody["api_key"])
	}
}

func TestRedactSnapshot_NoMatchDoesNothing(t *testing.T) {
	snap := &snapshot.Snapshot{
		Request: snapshot.Request{
			Method:  "GET",
			URL:     "/api/users",
			Headers: map[string]string{"Accept": "application/json"},
			Body:    map[string]any{"name": "Alice"},
		},
		Response: snapshot.Response{
			Status: 200,
			Body:   map[string]any{"id": 1},
		},
	}

	redactSnapshot(snap, []string{"request.headers.NonExistent", "response.body.nonexistent"})

	// Nothing should change
	if snap.Request.Headers["Accept"] != "application/json" {
		t.Errorf("expected Accept header preserved, got %q", snap.Request.Headers["Accept"])
	}
	body := snap.Request.Body.(map[string]any)
	if body["name"] != "Alice" {
		t.Errorf("expected name preserved, got %v", body["name"])
	}
}
