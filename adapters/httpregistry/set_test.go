package httpregistry_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/ports"
)

func newAsyncReg() *asyncReg {
	return &asyncReg{committed: map[ports.Hash]ports.Entry{}, failed: map[ports.Hash]error{}, commitDelay: 50 * time.Millisecond}
}

func serveReg(t *testing.T, reg ports.Registry) *httpregistry.Client {
	t.Helper()
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(shutdown)
	return httpregistry.NewClient("http://" + addr)
}

// A registry that is gone: its address refuses connections.
func downReg(t *testing.T) *httpregistry.Client {
	t.Helper()
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", newAsyncReg())
	if err != nil {
		t.Fatal(err)
	}
	shutdown()
	return httpregistry.NewClient("http://" + addr)
}

// A registry that rate-limits every request.
func limitedReg(t *testing.T) *httpregistry.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	return httpregistry.NewClient(srv.URL)
}

func holds(r *asyncReg, root ports.Hash) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.committed[root]
	return ok
}

// A publish moves past a registry that never took it, whether it is down or
// rate-limiting, and lands at the next; the accept mark fires once, for the one
// that took it.
func TestSet_PublishMovesPastAnUnavailableRegistry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first func(*testing.T) *httpregistry.Client
	}{{"down", downReg}, {"rate-limited", limitedReg}} {
		t.Run(tc.name, func(t *testing.T) {
			live := newAsyncReg()
			liveClient := serveReg(t, live)
			s := httpregistry.NewSet([]*httpregistry.Client{tc.first(t), liveClient}, 0)
			accepted, by := 0, ""
			s.OnAccepted(func(base string) { accepted++; by = base })
			e := ports.Entry{Root: ports.HashBytes([]byte("set-" + tc.name)), FileSize: 1}
			if err := s.Publish(context.Background(), e); err != nil {
				t.Fatalf("publish through a set with one live registry: %v", err)
			}
			if !holds(live, e.Root) {
				t.Fatal("the live registry does not hold the entry")
			}
			if accepted != 1 {
				t.Fatalf("accept mark fired %d times, want 1", accepted)
			}
			if by != liveClient.Base() {
				t.Fatalf("accept mark named %q, want the live registry %q", by, liveClient.Base())
			}
		})
	}
}

// A refusal is an answer, not an outage: the set returns it and the entry never
// reaches a second registry.
func TestSet_ARefusalIsNotRetriedElsewhere(t *testing.T) {
	refusing := newAsyncReg()
	refusing.refuse = ports.ErrDupPublish
	second := newAsyncReg()
	s := httpregistry.NewSet([]*httpregistry.Client{serveReg(t, refusing), serveReg(t, second)}, 0)
	e := ports.Entry{Root: ports.HashBytes([]byte("set-refused")), FileSize: 1}
	if err := s.Publish(context.Background(), e); !errors.Is(err, ports.ErrDupPublish) {
		t.Fatalf("publish = %v, want the first registry's ErrDupPublish", err)
	}
	time.Sleep(200 * time.Millisecond) // past the second registry's commit delay, had it been sent
	if holds(second, e.Root) {
		t.Fatal("a refused entry was re-sent to the second registry")
	}
}

// Once a registry has accepted the entry, a failure while waiting for its commit
// ends the publish there: re-sending an accepted entry elsewhere would put it in
// two mempools.
func TestSet_AnAcceptedEntryIsNotResentElsewhere(t *testing.T) {
	failing := newAsyncReg()
	failing.gatherFail = errors.New("no quorum this round")
	second := newAsyncReg()
	s := httpregistry.NewSet([]*httpregistry.Client{serveReg(t, failing), serveReg(t, second)}, 0)
	e := ports.Entry{Root: ports.HashBytes([]byte("set-accepted")), FileSize: 1}
	err := s.Publish(context.Background(), e)
	if err == nil || !strings.Contains(err.Error(), "no quorum this round") {
		t.Fatalf("publish = %v, want the accepting registry's commit failure", err)
	}
	time.Sleep(200 * time.Millisecond)
	if holds(second, e.Root) {
		t.Fatal("an accepted entry was re-sent to the second registry")
	}
}

// With every registry unavailable the error says so and still matches ErrUnavailable.
func TestSet_EveryRegistryUnavailable(t *testing.T) {
	s := httpregistry.NewSet([]*httpregistry.Client{downReg(t), limitedReg(t)}, 0)
	err := s.Publish(context.Background(), ports.Entry{Root: ports.HashBytes([]byte("set-none")), FileSize: 1})
	if !errors.Is(err, httpregistry.ErrUnavailable) || !strings.Contains(err.Error(), "every registry was unavailable (2 tried)") {
		t.Fatalf("publish = %v, want every registry unavailable", err)
	}
}

// The first registry tried is the one NewSet is told to start at, so publishers
// that start at different registries spread across them.
func TestSet_StartsAtTheGivenRegistry(t *testing.T) {
	a, b := newAsyncReg(), newAsyncReg()
	clients := []*httpregistry.Client{serveReg(t, a), serveReg(t, b)}
	for first, want := range map[int]*asyncReg{0: a, 1: b, 3: b, -1: b} {
		e := ports.Entry{Root: ports.HashBytes([]byte{byte(first + 10)}), FileSize: 1}
		if err := httpregistry.NewSet(clients, first).Publish(context.Background(), e); err != nil {
			t.Fatal(err)
		}
		if !holds(want, e.Root) {
			t.Fatalf("NewSet(first=%d) did not publish to registry %d first", first, (first%2+2)%2)
		}
	}
}

// Reads move past a registry that is down.
func TestSet_LookupMovesPastADownRegistry(t *testing.T) {
	live := newAsyncReg()
	e := ports.Entry{Root: ports.HashBytes([]byte("set-lookup")), FileSize: 1}
	live.committed[e.Root] = e
	s := httpregistry.NewSet([]*httpregistry.Client{downReg(t), serveReg(t, live)}, 0)
	got, ok, err := s.Lookup(context.Background(), e.Root)
	if err != nil || !ok || got.Root != e.Root {
		t.Fatalf("lookup = %v %v %v, want the live registry's entry", got.Root, ok, err)
	}
}
