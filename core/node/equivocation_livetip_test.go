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

// TestEquivocateAtLiveTipSlashesOnAdvancedChain345 is the #345 regression guard:
// Node.Equivocate must double-sign at the CURRENT uncommitted tip, not a hardcoded
// height 1. On a chain that has already committed past height 1 — a warmed
// field-test network — a height-1 double-sign is STALE: an honest target refuses to
// attest a proposal that is not at head+1, so proposeAndCommitTo fails, the forks
// are never placed, and the culprit is never slashed. That is exactly what the GCP
// #286 re-cert saw (the adversary set -equivocate but never logged "equivocation
// complete", and no slash fired within the window).
//
// This test advances the chain past height 1, runs the REAL Equivocate path, and
// asserts the double-sign is placed at the live tip, detected when a peer syncs the
// heavier fork, and recorded on-chain as an eviction. With the old hardcoded
// height-1 code the placement fails on the advanced chain, so Equivocate returns an
// error and this test fails — the guard the drill lacked.
func TestEquivocateAtLiveTipSlashesOnAdvancedChain345(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())

	idA := identity.FromSeed(701)
	idB := identity.FromSeed(702)
	idC := identity.FromSeed(703) // the culprit — a genesis-bonded validator
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

	// Advance the chain PAST height 1: A proposes two entries; the whole set commits.
	for _, tag := range []string{"e1", "e2"} {
		done := false
		a.ProposeEntry(mkEntry(tag), []ports.NodeID{idB.NodeID(), idC.NodeID()}, all, cfg.Quorum, func(error) { done = true })
		sched.Run()
		if !done {
			t.Fatalf("advance %q: propose did not complete", tag)
		}
	}
	// Bring B and C to A's head so an Equivocate at C's tip is head+1 for both targets.
	b.SyncChain([]ports.NodeID{idA.NodeID()}, func(int, error) {})
	c.SyncChain([]ports.NodeID{idA.NodeID()}, func(int, error) {})
	sched.Run()
	if _, next := c.Chain().Head(); next <= 2 {
		t.Fatalf("setup: chain should be past height 1 (head+1=%d, want >2)", next)
	}

	// C double-signs at the LIVE tip and places the forks on A (X) and B (Y, Z).
	var eqErr error
	eqDone := false
	c.Equivocate(idA.NodeID(), idB.NodeID(), func(e error) { eqErr, eqDone = e, true })
	sched.Run()
	if !eqDone || eqErr != nil {
		t.Fatalf("Equivocate must place the forks at the live tip (the old hardcoded height-1 code fails here): %v", eqErr)
	}

	// A detects C's double-sign when it syncs B's heavier fork (slashEquivocators runs
	// in the sync path), then records the eviction in the block it proposes.
	a.SyncChain([]ports.NodeID{idB.NodeID()}, func(int, error) {})
	sched.Run()
	done := false
	a.ProposeEntry(mkEntry("post-slash"), []ports.NodeID{idB.NodeID()}, all, cfg.Quorum, func(error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("A should commit a block carrying the on-chain slash")
	}
	if !a.Chain().IsSlashed(idC.NodeID()) || a.Chain().BondedSize(idC.NodeID()) != 0 {
		t.Fatal("#345: a live-tip equivocator must be detected and evicted from the objective set")
	}
}
