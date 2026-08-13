package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestEquivocateResumesAfterPartialPlacement378 is the #378 regression guard:
// Node.Equivocate must be RESUMABLE across retries. The drill needs two
// conflicting blocks at the SAME height on the SAME base, but the two honest
// peers earn the culprit's standing at INDEPENDENT moments, so a first attempt
// routinely lands X and then has Y refused. The old driver rebuilt X/Y/Z at the
// LIVE head every retry; once X committed on honestX and the culprit synced it
// back (its head advanced), every later attempt re-proposed X with the same
// deterministic root at a new height — which honestX refused forever
// (ErrDupRoot), a permanent wedge (the bimodal RED under WAN delay).
//
// This test reproduces that exact interleaving deterministically with a
// partition: the heavier fork Y,Z lands on B while A is unreachable (X, placed
// last, refused), the culprit then syncs the fork back so its head advances, and
// a RETRY after A is reachable must place X and get the culprit slashed. With the
// old rebuild-at-head code the retry re-proposes the un-placed leg at the
// advanced height and the target refuses (wrong-parent/dup), so the drill never
// completes and no slash fires — this test fails. With resumable placement
// (pinned base + per-leg latch) the retry skips the already-placed Y,Z and drives
// only X, pinned to the original height, so any interleaving converges.
func TestEquivocateResumesAfterPartialPlacement378(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 9, simnet.DefaultConfig())

	idA := identity.FromSeed(781)
	idB := identity.FromSeed(782)
	idC := identity.FromSeed(783) // the culprit
	pub := func(id *identity.Identity) []byte {
		return append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...)
	}

	anchors := map[ports.NodeID]bool{idA.NodeID(): true, idB.NodeID(): true, idC.NodeID(): true}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")},
		BondRegs: []chain.BondReg{
			{Validator: pub(idA), Root: ports.HashBytes(pub(idA)), Size: bondSize},
			{Validator: pub(idB), Root: ports.HashBytes(pub(idB)), Size: bondSize},
			{Validator: pub(idC), Root: ports.HashBytes(pub(idC)), Size: bondSize},
		}}
	chain.Sign(g, idA.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, MatureValidators: 99}

	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	a, b, c := mk(idA), mk(idB), mk(idC)
	all := []ports.NodeID{idA.NodeID(), idB.NodeID(), idC.NodeID()}
	b.Bootstrap([]ports.NodeID{idA.NodeID()}, func() {})
	c.Bootstrap([]ports.NodeID{idA.NodeID()}, func() {})
	sched.Run()

	// PARTITION A (honestX) away from the culprit C: the heavier fork Y,Z can land
	// on B (placed first), but X (bound for A, placed LAST) cannot — modelling A not
	// yet reachable/qualified when the drill first fires.
	net.Partition(idA.NodeID())

	// Attempt 1: Y,Z → B commit; X → A fails (partitioned). Equivocate returns an
	// error, but the heavier fork is now committed on B (the partial placement).
	var err1 error
	done1 := false
	c.Equivocate(idA.NodeID(), idB.NodeID(), func(e error) { err1, done1 = e, true })
	sched.Run()
	if !done1 || err1 == nil {
		t.Fatalf("attempt 1 must fail at X (A partitioned) after placing Y,Z: done=%v err=%v", done1, err1)
	}
	if _, ok := b.Chain().LookupRoot(advEntry("Y").Root); !ok {
		t.Fatal("attempt 1 should have committed Y on B (the partial placement the wedge starts from)")
	}
	if _, ok := b.Chain().LookupRoot(advEntry("Z").Root); !ok {
		t.Fatal("attempt 1 should have committed Z on B (the heavier fork lands first)")
	}

	// The culprit syncs B's heavier fork and its head ADVANCES past the double-sign
	// height — exactly the condition under which the old rebuild-at-head driver
	// re-proposed the un-placed leg at a new height and hit a wrong-parent/dup wedge.
	_, hBefore := c.Chain().Head()
	c.SyncChain([]ports.NodeID{idB.NodeID()}, func(int, error) {})
	sched.Run()
	if _, hAfter := c.Chain().Head(); hAfter <= hBefore {
		t.Fatalf("setup: culprit head should advance after syncing the heavier fork (before=%d after=%d)", hBefore, hAfter)
	}

	// Heal: A is reachable and qualifies C now.
	net.ClearPartition()

	// Attempt 2 (the retry): with resumable placement, Y,Z are latched done and the
	// base is pinned, so this drives only X onto A — pinned at the ORIGINAL height, not
	// rebuilt at the culprit's since-advanced head. It must complete.
	var err2 error
	done2 := false
	c.Equivocate(idA.NodeID(), idB.NodeID(), func(e error) { err2, done2 = e, true })
	sched.Run()
	if !done2 || err2 != nil {
		t.Fatalf("#378: the retry after a partial placement must RESUME and complete (old code rebuilds X at the advanced head and wedges): %v", err2)
	}
	if _, ok := a.Chain().LookupRoot(advEntry("X").Root); !ok {
		t.Fatal("the retry should have placed X on A (the final leg, pinned to the original height)")
	}

	// The double-sign is now placed on both peers (X on A, Y+Z on B). A detects
	// C's equivocation when it syncs B's heavier fork, and records the eviction.
	a.SyncChain([]ports.NodeID{idB.NodeID()}, func(int, error) {})
	sched.Run()
	done := false
	a.ProposeEntry(mkEntry("post-slash"), []ports.NodeID{idB.NodeID()}, all, cfg.Quorum, func(error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("A should commit a block carrying the on-chain slash")
	}
	if !a.Chain().IsSlashed(idC.NodeID()) || a.Chain().BondedSize(idC.NodeID()) != 0 {
		t.Fatal("#378: the resumed double-sign must still be detected and the culprit evicted")
	}
}
