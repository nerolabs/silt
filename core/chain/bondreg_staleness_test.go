package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Factor (ii) of the MATURING drain-cadence wall (instrumented run 7134711-18163;
// docs/thinking/2026-08-15-maturing-wall-is-coordination-cadence-not-cpu.md): a bond
// registration is signed over BondRegNonce(prev) — bound to ONE specific head — and
// ValidateBondReg / validateBondRegs accept it ONLY against the CURRENT head. So the
// instant the head advances (a heartbeat/other block commits), a reg in flight goes
// STALE and is refused; the submitter must resign over the new head and race the
// moving head again. Over a real WAN — where a proposer proposes on head-advance
// BEFORE the WAN-delayed resubmission arrives — this starves the drain: blocks commit
// empty below the #286 byte cap, and maturity never reaches bar-2 in the window
// (observed ~1 reg/105 s, ~3× slower than the byte cap alone dictates). It never
// showed in single-host sims because there is no propagation delay there.
//
// FAILING-FIRST: this encodes the DESIRED post-fix behavior — a reg only a FEW heads
// stale still validates and commits (fix A: accept BondRegNonce over the last K
// committed heads, K bounded by the anti-release window). It is RED on the current
// single-head rule and turns GREEN with the fix. K is a C1 security parameter (a
// reg's space-time proof must still evidence CURRENT possession), bounded by the
// anti-release/reseal window — research-gated; this test pins the LIVENESS half.
func TestBondRegStaleAfterOneHead_factorII(t *testing.T) {
	const bond = int64(64) << 20
	a1, a2 := key(9000), key(9001)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true}
	// Young, anchor-gated, never-maturing (MatureValidators high) — the MATURING
	// drain regime in miniature; quorum 1 so a single anchor block commits.
	cfg := Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}

	// A validator signs its bond reg over the GENESIS head (the head it saw).
	v := key(9200)
	reg := bondRegDom(v, bond, g.Hash(), 0)

	// Precondition: bound to the CURRENT head, it validates.
	if !c.ValidateBondReg(reg) {
		t.Fatal("precondition: a reg bound to the current head must validate")
	}

	// The head advances by ONE — an empty anchor heartbeat block, exactly the WAN
	// reality where a block commits before the reg reaches a proposer.
	b1 := &Block{Version: BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(b1, a1)
	b1.Atts = []Attestation{Attest(b1, a2)}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("setup: an anchor heartbeat block must commit: %v", err)
	}

	// DESIRED (fix A): a reg only ONE head stale is STILL valid — within the K-head
	// window it can still be drained. RED on the current single-head rule.
	if !c.ValidateBondReg(reg) {
		t.Fatal("factor (ii): a bond reg only ONE head stale is REJECTED (single-head validity) — " +
			"the drain-starving staleness. Fix A: accept BondRegNonce over the last K committed heads.")
	}

	// And it must COMMIT one head later (the drain actually proceeds).
	b2 := &Block{Version: BlockVersion, Height: 2, Prev: b1.Hash(), Entries: []ports.Entry{entry(2)},
		BondRegs: []BondReg{reg}}
	Sign(b2, a1)
	b2.Atts = []Attestation{Attest(b2, a2)}
	if err := c.Append(*b2); err != nil {
		t.Fatalf("a one-head-stale reg must still commit within the K-head window: %v", err)
	}
}

// The SECURITY half of fix A: the head window is BOUNDED, not "accept forever". A
// reg stale beyond BondRegHeadWindow is REJECTED, so its space-time proof still
// evidences recent possession (C1 anti-release) — the window only removes the
// one-head brittleness, it does not let a released-and-replayed old proof persist.
func TestBondRegRejectedBeyondHeadWindow_factorII(t *testing.T) {
	const bond = int64(64) << 20
	a1, a2 := key(9000), key(9001)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true}
	// A tight window of 3 so the test advances only a few heads to cross it.
	cfg := Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, BondRegHeadWindow: 3}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}

	v := key(9200)
	reg := bondRegDom(v, bond, g.Hash(), 0) // signed over genesis

	// Advance the head to height 3 (three heartbeats) — genesis is now 3 heads back,
	// i.e. at the edge of a window of 3 (nonces: h3, h2, h1 → genesis excluded).
	prev := g.Hash()
	for h := uint64(1); h <= 3; h++ {
		blk := &Block{Version: BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(blk, a1)
		blk.Atts = []Attestation{Attest(blk, a2)}
		if err := c.Append(*blk); err != nil {
			t.Fatalf("setup heartbeat h=%d: %v", h, err)
		}
		prev = blk.Hash()
	}

	// Genesis-bound reg is now stale beyond the 3-head window → REJECTED.
	if c.ValidateBondReg(reg) {
		t.Fatal("a reg stale BEYOND BondRegHeadWindow must be rejected — the freshness bound (C1) must hold")
	}
}
