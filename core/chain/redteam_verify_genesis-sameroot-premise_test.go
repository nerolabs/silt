package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The NAMED-PREMISE guard for residual R-G (PE ruling
// RULING-618-updated-sameroot-dedup-fix-2026-08-28, residual R-G;
// deliberation docs/thinking/2026-08-28-genesis-sameroot-residual.md).
//
// #618 shipped the certified seenRoot per-root distinct-ID dedup in
// validateBondRegs. AppendGenesis (chain.go:2730) does NOT run validateBondRegs
// — it goes straight to apply() — so the dedup does NOT cover the genesis path.
// Genesis apply() IS order-dependent for two distinct-ID UNPROVEN same-root regs.
//
// This is safe TODAY by an EXTERNAL invariant, NOT by the guard: the production
// genesis is a byte-identical shared constant across every node (declared, not
// agreed — chain.go:2724-2727) and it carries NO BondRegs at all
// (genesis.Build → core/genesis/genesis.go:79, Entries only). With no per-node
// choice of slice order there is nothing to diverge on.
//
// The era-3 SMT freeze property is unconditional: for any block B, apply(B)
// produces the same state regardless of B.BondRegs order. Genesis is a block B
// that does NOT satisfy this in isolation. The freeze's order-independence claim
// is therefore whole ONLY IF you also hold the premise "genesis regs are never
// re-ordered / never per-node / carry no distinct-ID same-root collision." That
// premise lives OUTSIDE apply() and outside the dedup.
//
// These two tests NAME that premise as an executable, pinned fact so it cannot
// silently break — the future era-3 freeze gate inherits a named check rather
// than an unstated assumption. NO change to genesis validity: this is the
// record-it half of the PE's "close it OR record it," inside the already-certified
// envelope.

// TestGenesisSameRootApplyIsOrderDependent pins the un-guarded seam: genesis
// apply() commits a DIFFERENT owner depending on the BondReg slice order, because
// AppendGenesis never runs the seenRoot dedup. This asserts the ORDER-DEPENDENCE
// exists — the opposite polarity of the height>0 tests, on purpose.
//
// Teeth (the two ways this test flips RED, each forcing a conscious re-derivation
// of the premise):
//   - If a future change ADDS the dedup to the genesis path (making genesis
//     order-INDEPENDENT / rejecting the collision), the owners stop differing and
//     this test fails. That is exactly option (b) of the deliberation — a
//     consensus-rule change to genesis validity — which is research-gated. The
//     failure forces it to be a conscious, routed decision, not a silent slip.
//   - If genesis apply()'s bond-reg resolution is refactored such that the
//     order-dependence changes shape, this test fails and re-opens the premise.
func TestGenesisSameRootApplyIsOrderDependent(t *testing.T) {
	const minBond = int64(2) << 20
	keyA, keyB := key(9301), key(9302)
	shared := ports.HashBytes([]byte("genesis-sameroot-premise-shared-plot"))

	// build applies a genesis carrying two distinct-ID UNPROVEN regs on one shared
	// root, in the requested slice order. AppendGenesis does not validate BondRegs,
	// so both reach apply() and "first in the slice wins" the root.
	build := func(aFirst bool) *Chain {
		cfg := Config{Quorum: 1, MinBond: minBond, MatureValidators: 100}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		regA := bondRegAt(keyA, shared, minBond, ports.Hash{})
		regB := bondRegAt(keyB, shared, minBond, ports.Hash{})
		regs := []BondReg{regA, regB}
		if !aFirst {
			regs = []BondReg{regB, regA}
		}
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: regs}
		Sign(g, keyA)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis (aFirst=%v): %v", aFirst, err)
		}
		return c
	}

	a := build(true)  // slice [A,B]
	b := build(false) // slice [B,A]

	idA, idB := idOf(keyA), idOf(keyB)
	ownerA, ownerB := a.bondRootOwner[shared], b.bondRootOwner[shared]

	// The premise, asserted: at genesis (unguarded), owner is decided by slice order.
	if ownerA == ownerB {
		t.Fatalf("PREMISE CHANGED: genesis apply() is now ORDER-INDEPENDENT for two "+
			"distinct-ID unproven same-root regs (owner=%x in both orderings).\n\n"+
			"Residual R-G was safe by genesis byte-identity, NOT by a guard. If the "+
			"genesis path has been made order-independent (e.g. the seenRoot dedup was "+
			"extended into AppendGenesis, or apply() now rejects the collision), that is "+
			"option (b) of docs/thinking/2026-08-28-genesis-sameroot-residual.md — a "+
			"consensus-rule change to genesis VALIDITY, which is research-gated and "+
			"human-ratified. Route it; then update this named premise to match the new "+
			"rule. Do NOT simply relax this assertion.", ownerA[:6])
	}
	// And it is genuinely order-dependent both directions (owner tracks the slice head).
	if ownerA != idA || ownerB != idB {
		t.Fatalf("expected owner to track the slice head: aFirst owner=%x want A=%x; "+
			"bFirst owner=%x want B=%x", ownerA[:6], idA[:6], ownerB[:6], idB[:6])
	}
	t.Logf("premise pinned: genesis apply() owner is slice-order-dependent "+
		"(A-first→%x, B-first→%x) — safe only by genesis byte-identity", idA[:6], idB[:6])
}
