package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

func TestFireRequest_GET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/users" {
			t.Errorf("expected /api/users, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"users": []any{}})
	}))
	defer server.Close()

	req := snapshot.Request{
		Method:  "GET",
		URL:     "/api/users",
		Headers: map[string]string{"Accept": "application/json"},
	}

	resp, err := FireRequest(server.URL, req, 5000)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Status != 200 {
		t.Errorf("expected status 200, got %d", resp.Status)
	}
	if resp.Body == nil {
		t.Fatal("expected response body")
	}
}

func TestFireRequest_POST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	defer server.Close()

	req := snapshot.Request{
		Method:  "POST",
		URL:     "/api/users",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    map[string]any{"name": "Alice"},
	}

	resp, err := FireRequest(server.URL, req, 5000)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Status != 201 {
		t.Errorf("expected status 201, got %d", resp.Status)
	}
}

func TestFireRequest_NilBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	req := snapshot.Request{
		Method: "DELETE",
		URL:    "/api/users/1",
	}

	resp, err := FireRequest(server.URL, req, 5000)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Status != 204 {
		t.Errorf("expected status 204, got %d", resp.Status)
	}
}
