package recorder

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithAuth_ValidToken(t *testing.T) {
	r := &Recorder{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Verify the auth header was stripped
		if auth := req.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected Authorization header to be stripped, got %q", auth)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := r.withAuth("test-secret", inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWithAuth_MissingToken(t *testing.T) {
	r := &Recorder{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Error("handler should not be called without auth")
	})

	handler := r.withAuth("test-secret", inner)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// Should include WWW-Authenticate header
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestWithAuth_WrongToken(t *testing.T) {
	r := &Recorder{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Error("handler should not be called with wrong token")
	})

	handler := r.withAuth("test-secret", inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestWithAuth_InvalidScheme(t *testing.T) {
	r := &Recorder{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Error("handler should not be called with invalid scheme")
	})

	handler := r.withAuth("test-secret", inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
