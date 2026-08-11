package httpregistry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/ports"
)

// #329 / immutable #5: the registry client's idempotent GET reads are the one client path
// NOT behind the consensus layer's retry. A transient network blip (a 5xx, a dropped
// packet) must be ridden out with a bounded backoff, not fail a swarm-get / root resolution
// on a single sample. A flaky server that 503s the first two attempts then serves the entry
// must still resolve via Lookup.
func TestClientGetRetry_RecoversTransient(t *testing.T) {
	root := ports.HashBytes([]byte("retry-ok"))
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) <= 2 {
			http.Error(w, "transient", http.StatusServiceUnavailable) // 503 twice
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"root":"` + root.String() + `","file_size":1}`))
	}))
	defer srv.Close()

	c := httpregistry.NewClient(srv.URL)
	e, ok, err := c.Lookup(context.Background(), root)
	if err != nil {
		t.Fatalf("Lookup should ride out two transient 503s, got %v", err)
	}
	if !ok || e.Root != root {
		t.Fatalf("Lookup did not resolve after retry: ok=%v root=%v", ok, e.Root)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("expected 3 attempts (2 transient + 1 success), got %d", got)
	}
}

// The retry is BOUNDED: a server that is always down returns an error after the attempts
// are exhausted, not an infinite loop.
func TestClientGetRetry_BoundedGivesUp(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "down", http.StatusInternalServerError) // always 500
	}))
	defer srv.Close()

	c := httpregistry.NewClient(srv.URL)
	if _, _, err := c.Lookup(context.Background(), ports.HashBytes([]byte("dead"))); err == nil {
		t.Fatal("Lookup against an always-500 server must return an error after retries")
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("expected exactly 3 bounded attempts, got %d", got)
	}
}

// A definitive 404 is an ANSWER (not found), not a transient blip — it must NOT be retried,
// so a not-found lookup stays fast.
func TestClientGetRetry_NoRetryOn404(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := httpregistry.NewClient(srv.URL)
	start := time.Now()
	_, ok, err := c.Lookup(context.Background(), ports.HashBytes([]byte("missing")))
	if err != nil || ok {
		t.Fatalf("a 404 lookup should be a clean not-found, got ok=%v err=%v", ok, err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("a 404 must not be retried: expected 1 attempt, got %d", got)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("a 404 must be immediate, not backed-off (took %s)", time.Since(start))
	}
}
