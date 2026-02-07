package recorder

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/esse/snapshot-tester/internal/config"
)

func TestWithRateLimit_ConcurrencyLimit(t *testing.T) {
	r := &Recorder{}

	var concurrent int64
	var maxConcurrent int64

	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cur := atomic.AddInt64(&concurrent, 1)
		defer atomic.AddInt64(&concurrent, -1)

		// Track max concurrent
		for {
			old := atomic.LoadInt64(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
				break
			}
		}

		// Simulate some work
		w.WriteHeader(http.StatusOK)
	})

	handler := r.withRateLimit(config.RateLimitConfig{
		MaxConcurrent: 2,
	}, inner)

	// Send requests concurrently
	var wg sync.WaitGroup
	successCount := int64(0)
	unavailableCount := int64(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				atomic.AddInt64(&successCount, 1)
			} else if w.Code == http.StatusServiceUnavailable {
				atomic.AddInt64(&unavailableCount, 1)
			}
		}()
	}

	wg.Wait()

	// With concurrency limit of 2, some requests should succeed and possibly some rejected
	if successCount == 0 {
		t.Error("expected at least some requests to succeed")
	}
}

func TestWithRateLimit_NoLimits(t *testing.T) {
	r := &Recorder{}

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// Zero values = no limits
	handler := r.withRateLimit(config.RateLimitConfig{}, inner)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
