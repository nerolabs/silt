//go:build bbootstrap

package main

// R2.9a — the ONE /api/status snapshot gate that reads the B_bootstrap block, and
// therefore the one that can only run under the `bbootstrap` build tag
// (D-BB-BUILD-TAG). Its siblings in r29a_status_surface_test.go are untagged, because
// the cache, its staleness stamps, the invalidation hook and the F2 token gate are
// properties of the status endpoint in EVERY build — a default binary has no histogram
// to cache, and it still must not recompute the O(R) + O(chunks) document per
// unauthenticated GET.

import (
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// --- G-BB-26 / BB-21: the snapshot is not recomputed per request -------------------

// TestR29aBB21StatusSnapshotIsCachedForOneInterval is Tester gate BB-21. Two GET
// /api/status calls inside one refresh interval, WITH A FETCH INTERLEAVED, return
// byte-identical bBootstrap blocks; a third call past the interval sees the fetch.
//
// WHY IT IS REQUIRED (RESEARCH CERTIFICATION 2026-09-05 section 3.5, two independent
// grounds). The R-BB-DELTA-TRAJECTORY residual was disclosed as "bounded by the poll
// rate", and the poll rate is the READER's own choice — there is no rate limiter
// anywhere on the UI server, so that was not a bound at all. And the recompute is an
// O(R) walk over a never-evicted account set plus the whole chunk store, INSIDE the
// node's event loop, per unauthenticated GET: build-immutable #8, "an unbounded system
// on a small box is not inefficient, it is unsafe."
func TestR29aBB21StatusSnapshotIsCachedForOneInterval(t *testing.T) {
	s, led, clk := r29aServer(t, true)
	s.now = nil // this gate drives the clock itself; see r29aServer's per-poll advance
	for i := 0; i < credit.BBootstrapMinRequesters; i++ {
		r29aFetch(led, i, 4096)
	}
	clk.now = ports.Time(3600 * 1e9)

	base := s.started
	first := statusAt(t, s, base, false)

	// The interleaved fetch: a brand-new identity, a byte count in a different bin.
	// This is exactly the observation the trajectory attack needs, and it must not be
	// visible until the interval turns over.
	r29aFetch(led, 999, 1<<20)
	clk.now = ports.Time(3600*1e9 + int64(statusSnapshotInterval/2))

	second := statusAt(t, s, base.Add(statusSnapshotInterval-time.Millisecond), false)

	fb, sb := statusKey(t, first, "bBootstrap"), statusKey(t, second, "bBootstrap")
	if string(fb) != string(sb) {
		t.Fatalf("two reads inside one refresh interval returned DIFFERENT bBootstrap blocks — the snapshot is being recomputed per request, so the disclosed bound is the reader's poll rate and the O(R) census walk runs once per unauthenticated GET on the event loop.\nfirst:  %s\nsecond: %s", fb, sb)
	}

	// Past the interval the instrument is live again: the interleaved fetch appears.
	third := statusAt(t, s, base.Add(statusSnapshotInterval+time.Second), false)
	if tb := statusKey(t, third, "bBootstrap"); string(tb) == string(fb) {
		t.Fatalf("the block did not change a full interval after a new identity fetched — the cache never expires, which is not a snapshot, it is a freeze: %s", tb)
	}
}
