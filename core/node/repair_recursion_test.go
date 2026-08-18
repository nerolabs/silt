package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// aloneNode builds a single node with no live peers (an empty routing table), so every
// DHT walk converges SYNCHRONOUSLY — the exact "node alone / peers dead" condition under
// which the provider-resolution + announce continuations used to recurse inline.
func aloneNode(t *testing.T, chunks int) (*Node, *simclock.Scheduler) {
	t.Helper()
	store := memstore.New()
	for i := 0; i < chunks; i++ {
		store.Put(bg(), ports.NewChunk([]byte{byte(i), byte(i >> 8), 0xA5}))
	}
	sched := simclock.New()
	var id ports.NodeID
	id[0] = 7
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	end := &linkEnd{net: ln, id: id}
	ln.ends[id] = end
	return New(id, DefaultConfig(), sched, end, store), sched
}

// TestAnnounceHeldTrampolinesWalkTerminal is the flixz load-test crash (silt-findings-loadtest
// Finding 2 / handoff bug B): announceAll's per-key IterativeFindNode and the
// resolveProviders⇄probeShard⇄sweepProviders repair cycle invoke their terminal callbacks
// INLINE whenever a walk converges synchronously (no live peer to defer to the loop). Over a
// large held set (193K keys, box alone) the frames pile up O(keys) deep and overflow the
// goroutine stack — "fatal error: stack overflow", watchdog kill, exit 2 — and it blocked
// publish of the two largest films.
//
// The structural guard: a walk's terminal callback MUST be trampolined through the event loop,
// so completion is DEFERRED (fires only after a scheduler step) and stack depth is O(1) across
// keys regardless of sync-vs-async completion. Here, with no live peers, AnnounceHeld would
// otherwise run its whole chain inline and finish BEFORE the loop is drained.
//
// RED before the fix: `done` is true immediately (the chain ran inline on the caller's stack).
// GREEN after: `done` is false until the loop is drained (each walk terminal was posted).
func TestAnnounceHeldTrampolinesWalkTerminal(t *testing.T) {
	n, sched := aloneNode(t, 64)

	done := false
	n.AnnounceHeld(func(int) { done = true })

	if done {
		t.Fatal("AnnounceHeld completed INLINE (synchronously) on the caller's stack — the walk terminal " +
			"is not trampolined, so over a large held set the announce/resolve/repair chain recurses " +
			"O(keys) deep and overflows the goroutine stack (flixz 193K-key crash)")
	}

	sched.Run()
	if !done {
		t.Fatal("AnnounceHeld did not complete after draining the loop")
	}
}

// TestResolveProvidersTrampolinesWalkTerminal pins the same guard on the provider-resolution
// entrypoint the repair cycle re-enters (probeShard → resolveProviders): with no live peers the
// walk converges synchronously and its done must be DEFERRED, not run on the caller's stack.
func TestResolveProvidersTrampolinesWalkTerminal(t *testing.T) {
	n, sched := aloneNode(t, 1)

	var key ports.Hash
	key[0] = 0xC0
	done := false
	n.resolveProviders(ports.ChunkID(key), func([]ports.NodeID) { done = true })

	if done {
		t.Fatal("resolveProviders completed INLINE — the walk terminal re-enters repair on the same " +
			"stack, the resolveProviders⇄probeShard⇄sweepProviders cycle that overflowed the stack")
	}
	sched.Run()
	if !done {
		t.Fatal("resolveProviders did not complete after draining the loop")
	}
}
