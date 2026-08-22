package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/core/denylist"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/ports"
)

// These tests pin the recursion/re-entrancy audit (#467, PE flixz Finding 2 audit
// extension): a continuation chain that can complete SYNCHRONOUSLY must post its
// next step through the event loop, never recurse on the caller's stack. The #471
// trampoline bounded the DHT walk terminal; these cover the sibling chains the
// audit found still recursing inline when their fast paths complete synchronously:
//
//   - repairStripes' healthy-stripe walk (O(stripes) deep per sweep)
//   - FetchChunk's already-held fast path (fetchAll/fetchColumn chains, O(ids))
//   - fetchFrom's no-provider exit (same chains, fresh unsettled roots)
//   - request's synchronous send-failure (every request-crossing chain)
//   - repairTick's root walk over synchronously-skipped (denied) roots
//
// Each asserts the #471 contract: completion is DEFERRED — the done callback must
// not have fired when the entry call returns, and must fire once the loop drains.

// syntheticLayout builds a layout of `stripes` erasure stripes with distinct
// fake chunk ids — enough structure for storedShards/repairStripes, no bytes.
func syntheticLayout(stripes int, p erasure.Params) *manifest.Layout {
	m := &manifest.Layout{K: p.K, N: p.N, ChunkSize: 16}
	for i := 0; i < stripes*p.K; i++ {
		var h ports.Hash
		h[0], h[1], h[2] = byte(i), byte(i>>8), 0x11
		m.Chunks = append(m.Chunks, h[:])
	}
	for i := 0; i < stripes*p.ParityShards(); i++ {
		var h ports.Hash
		h[0], h[1], h[2] = byte(i), byte(i>>8), 0x22
		m.Parity = append(m.Parity, h[:])
	}
	return m
}

// TestRepairStripesTrampolinesHealthyStripeWalk: a caretaker sweeping a large
// HEALTHY file (every stripe within slack — the common case, every sweep) walked
// all stripes by inline recursion: next() called repairStripes(stripe+1) on the
// same stack, O(stripes) frames deep, all in one loop task. The stripe advance
// must be posted through the loop instead.
func TestRepairStripesTrampolinesHealthyStripeWalk(t *testing.T) {
	n, sched := aloneNode(t, 0)
	p := erasure.Params{K: 2, N: 3}
	m := syntheticLayout(64, p)
	refs := storedShards(m, p)
	reachable := make(map[ports.ChunkID]bool, len(refs))
	for _, r := range refs {
		reachable[r.id] = true
	}

	done := false
	n.repairStripes(m, p, refs, reachable, map[ports.ChunkID]map[uint64]bool{}, 0, nil, func() { done = true })

	if done {
		t.Fatal("repairStripes walked every healthy stripe INLINE on the caller's stack — " +
			"O(stripes) recursion depth in one loop task (the #467 disease on the stripe axis)")
	}
	sched.Run()
	if !done {
		t.Fatal("repairStripes did not complete after draining the loop")
	}
}

// TestFetchChunkHeldFastPathTrampolines: FetchChunk on an already-held chunk
// returned inline, so a fetchAll/fetchColumn chain over a fully-held id list
// (a caretaker or consumer re-fetching content it hosts) recursed O(ids) deep.
func TestFetchChunkHeldFastPathTrampolines(t *testing.T) {
	n, sched := aloneNode(t, 0)
	c := ports.NewChunk([]byte("already held"))
	n.store.Put(bg(), c)

	done := false
	n.FetchChunk(c.ID, func(err error) {
		if err != nil {
			t.Errorf("held chunk fetch failed: %v", err)
		}
		done = true
	})

	if done {
		t.Fatal("FetchChunk completed INLINE for a held chunk — a fetch chain over a " +
			"fully-held list recurses O(ids) deep on the caller's stack")
	}
	sched.Run()
	if !done {
		t.Fatal("FetchChunk did not complete after draining the loop")
	}
}

// TestFetchFromNoProvidersTrampolines: fetchFrom with an empty provider set
// reported failure inline — the fresh-root condition (#467: providers not yet
// settled), where a per-column chain recursed O(ids) deep through back-to-back
// empty sweeps.
func TestFetchFromNoProvidersTrampolines(t *testing.T) {
	n, sched := aloneNode(t, 0)
	var id ports.ChunkID
	id[0] = 0xF7

	done := false
	n.fetchFrom(id, nil, func(ok bool) {
		if ok {
			t.Error("fetchFrom reported success with no providers")
		}
		done = true
	})

	if done {
		t.Fatal("fetchFrom completed INLINE with no providers — a fetch chain over an " +
			"unsettled root recurses O(ids) deep on the caller's stack")
	}
	sched.Run()
	if !done {
		t.Fatal("fetchFrom did not complete after draining the loop")
	}
}

// TestRequestSendFailureDefersCallback: request's callback fired INLINE when the
// transport rejected the send synchronously (no route / dead adapter). That makes
// every per-item continuation chain that crosses request — probe try-chains,
// announce send-chains, credit mints — conditionally recursive: a transport
// failing all sends turns them back into O(items) inline recursion. The callback
// must be deferred through the loop on the send-error path too.
func TestRequestSendFailureDefersCallback(t *testing.T) {
	n, sched := aloneNode(t, 0)
	var unknown ports.NodeID
	unknown[0] = 99 // not in the linkNet: Send fails synchronously

	fired := false
	n.request(unknown, ports.Message{Kind: ports.MsgHasChunk}, func(_ ports.Message, err error) {
		if err == nil {
			t.Error("expected a send error")
		}
		fired = true
	})

	if fired {
		t.Fatal("request callback fired INLINE on a synchronous send failure — every " +
			"request-crossing continuation chain is one dead transport from unbounded recursion")
	}
	sched.Run()
	if !fired {
		t.Fatal("request callback never fired after draining the loop")
	}
}

// TestRepairTickYieldsBetweenDeniedRoots: a denied root completes its sweep step
// synchronously, so a caretaker whose cared set is (mass-)denied walked ALL roots
// inline in one loop task. With the advance posted through the loop, processing
// N such roots takes N zero-time scheduler steps; inline, the first pending event
// is already the next sweep, a full RepairInterval away.
func TestRepairTickYieldsBetweenDeniedRoots(t *testing.T) {
	n, sched := aloneNode(t, 0)
	deny := denylist.New()
	for i := 1; i <= 3; i++ {
		var root ports.Hash
		root[0] = byte(i)
		deny.Add(root)
		n.care = append(n.care, link.CareHandle{Root: root})
	}
	n.SetDenylist(deny)

	n.repairTick()

	zeroSteps := 0
	for sched.Step() && sched.Now() == 0 {
		zeroSteps++
		if zeroSteps > 100 {
			t.Fatal("loop did not quiesce at t=0")
		}
	}
	if zeroSteps == 0 {
		t.Fatal("repairTick processed every denied root INLINE in one loop task — the root " +
			"walk must yield to the loop between roots")
	}
}

// simclock import is exercised via aloneNode's scheduler; keep the linter honest.
var _ = simclock.New
