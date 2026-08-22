package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// deadHolderRig wires a fetcher, a live provider that holds `chunks`, and a
// DEAD holder — an endpoint that is registered (so a dial to it is NOT a
// fast synchronous "no route" error) but whose handler never replies, so a
// dial to it can only end in a RequestTimeout. deadSends counts fetch dials
// aimed at the corpse. Returns the fetcher, the [dead, live] provider list,
// the scheduler, and a pointer to the dial counter.
func deadHolderRig(t *testing.T, cfg Config, chunks ...ports.Chunk) (*Node, []ports.NodeID, *simclock.Scheduler, *int) {
	t.Helper()
	sched := simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	var fID, liveID, deadID ports.NodeID
	fID[0], liveID[0], deadID[0] = 1, 2, 3

	deadSends := 0
	ln.dropFetch = func(from, to ports.NodeID) bool {
		if to == deadID {
			deadSends++ // a dial actually left for the corpse
		}
		return false // never a synchronous refusal: the corpse just goes silent → timeout
	}

	fEnd := &linkEnd{net: ln, id: fID}
	liveEnd := &linkEnd{net: ln, id: liveID}
	deadEnd := &linkEnd{net: ln, id: deadID}
	deadEnd.h = func(from ports.NodeID, msg ports.Message) {} // registered but silent
	ln.ends[fID], ln.ends[liveID], ln.ends[deadID] = fEnd, liveEnd, deadEnd

	fetcher := New(fID, cfg, sched, fEnd, memstore.New())
	liveStore := memstore.New()
	for _, c := range chunks {
		liveStore.Put(bg(), c)
	}
	New(liveID, cfg, sched, liveEnd, liveStore) // serves via the real handler

	// Dead first: the order that costs a full RequestTimeout before the live
	// provider is even tried — the shape a stale provider record produces.
	return fetcher, []ports.NodeID{deadID, liveID}, sched, &deadSends
}

// TestFetchNegativeCachesDeadHolder is the #226 regression: a holder that
// times out once is negative-cached, so a LATER fetch skips it instead of
// eating another full RequestTimeout on the same corpse. Without this, a
// churny swarm re-dials dead provider records every sweep and the serial
// repair fetch starves on timeouts.
func TestFetchNegativeCachesDeadHolder(t *testing.T) {
	c1 := ports.NewChunk([]byte("chunk one — pays the timeout"))
	c2 := ports.NewChunk([]byte("chunk two — must skip the corpse"))
	fetcher, provs, sched, deadSends := deadHolderRig(t, DefaultConfig(), c1, c2)

	var got1, got2, done bool
	fetcher.fetchFrom(c1.ID, provs, func(ok bool) {
		got1 = ok
		// The second fetch runs only after the first has timed out on the
		// dead holder and stamped the negative cache — so it must skip it.
		fetcher.fetchFrom(c2.ID, provs, func(ok bool) { got2, done = ok, true })
	})
	sched.Run()

	if !done || !got1 || !got2 {
		t.Fatalf("both fetches should succeed via the live provider (got1=%v got2=%v done=%v)", got1, got2, done)
	}
	if *deadSends != 1 {
		t.Fatalf("dead holder must be dialed once then negative-cached: got %d dials, want 1", *deadSends)
	}
}

// TestFetchDialsSoleCooledHolder is the #69 regression: the negative cache
// must NEVER be the reason a fetch fails. A required chunk (e.g. a manifest
// chunk) can have a single provider that timed out transiently — a node that
// restarted and is already re-announcing — and if it's the only candidate it
// must be dialed, not skipped-to-empty and reported unreachable. Broke the
// cross-NAT reprovide test (#69) before the anyLive guard.
func TestFetchDialsSoleCooledHolder(t *testing.T) {
	c := ports.NewChunk([]byte("sole provider, recovered after a transient timeout"))
	fetcher, provID, sched := twoNode(t, DefaultConfig(), c, nil)
	// The fetcher recently failed to reach provID, so it's in cooldown — but
	// provID has since recovered and now serves the chunk.
	fetcher.dead[provID] = corpse{until: fetcher.clock.Now().Add(fetcher.cfg.HolderCooldown)}

	var got, done bool
	fetcher.fetchFrom(c.ID, []ports.NodeID{provID}, func(ok bool) { got, done = ok, true })
	sched.Run()

	if !done || !got {
		t.Fatalf("a sole cooled-but-recovered provider must be dialed, not skipped to empty (#69): done=%v got=%v", done, got)
	}
}

// TestFetchWithoutCooldownRedialsDeadHolder is the inverted control: disable
// the negative cache (HolderCooldown=0) and the SAME dead holder is re-dialed
// on every fetch — the starvation the fix removes. It proves the cache, not
// some other effect, is what suppresses the re-dial.
func TestFetchWithoutCooldownRedialsDeadHolder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HolderCooldown = 0 // negative cache OFF
	c1 := ports.NewChunk([]byte("chunk one — pays the timeout"))
	c2 := ports.NewChunk([]byte("chunk two — pays it AGAIN without the cache"))
	fetcher, provs, sched, deadSends := deadHolderRig(t, cfg, c1, c2)

	var done bool
	fetcher.fetchFrom(c1.ID, provs, func(bool) {
		fetcher.fetchFrom(c2.ID, provs, func(bool) { done = true })
	})
	sched.Run()

	if !done {
		t.Fatalf("both fetches should still complete")
	}
	if *deadSends != 2 {
		t.Fatalf("with the negative cache disabled the corpse is re-dialed every fetch: got %d dials, want 2", *deadSends)
	}
}
