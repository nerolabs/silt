package chain

import (
	"crypto/ed25519"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// CONSENSUS-RULE COVERING PROBE (2026-08-28, the #506-gate order-independence
// increment) — the RED-then-GREEN gate for the certified canonicalization fix in
// apply() (cert sameid-twoversion-intrablock-bondreg-contention 2026-08-28,
// resolution §3(a1): fold same-id multi-reg to one canonical winner by a total
// order — largest Size, then Version, then Domain, then Sig).
//
// This REPLACES the earlier finding-repro (TestRegVersionIntraBlockOrderFinding),
// which ASSERTED the buggy divergence so the finding was on the record while it was
// routed to the Researcher. The fix has landed; this probe now asserts the FIXED
// property: two same-id regs with distinct Version/Domain/Size in ONE pre-gate block
// commit BYTE-IDENTICAL regVersion/bondDomain/bonded across BOTH intra-block slice
// orderings.
//
// MECHANISM the probe pins (chain.go):
//   - apply() folds b.BondRegs through canonicalBondRegs() before the bond loop, so
//     for a single id carrying multiple regs a canonical winner is chosen by
//     bondRegLess (largest Size, then Version, then Domain, then Sig) and ALL its
//     fields (regVersion, bondDomain, bonded, bondRootOwner, …) are applied. The
//     winner is a pure function of block content, not slice position.
//   - Before the fix apply() took the LAST reg in slice order, so flipping the slice
//     flipped the committed regVersion/bondDomain/bonded — an order-dependent
//     history-independent SMT root (a #618-class fork). regVersion feeds the #506
//     lock-in tally (rotateEpoch), so gateLockedIn/gateHeight inherited the split.
//
// ABLATION (session-7 "inject the defect" rule): stash the canonicalBondRegs fold
// in apply() (revert to `for _, r := range b.BondRegs`) and this test goes RED — the
// two orderings diverge (regVersion 3 vs 2, distinct bondDomain, distinct bonded).
// With the fold in place it is GREEN. The RED-then-GREEN transcript is pasted in the
// PR / the docs/thinking note.
func TestRegVersionIntraBlockOrderIndependent(t *testing.T) {
	// build commits ONE pre-gate block carrying two regs for validator v on its own
	// root, with DISTINCT Version, Domain, AND Size, so the canonical total order is
	// exercised on all three primary keys. hiFirst controls the intra-block slice
	// order — the ONLY variable between the two chains.
	//
	// The canonical winner (largest Size, then Version, then Domain, then Sig): the
	// larger-size reg wins. Here `hi` carries the larger size (2*twoMiB) AND the
	// higher version/domain, so `hi` is the winner under BOTH orderings.
	build := func(hiFirst bool) (*Chain, error) {
		v := key(90001)
		gov := []ed25519.PrivateKey{key(90002), key(90003)} // committing quorum
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 2}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		all := append([]ed25519.PrivateKey{v}, gov...)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		for _, k := range all {
			g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
		}
		Sign(g, all[0])
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}

		prev := g.Hash()
		rootV := ports.HashBytes(v.Public().(ed25519.PublicKey))
		// Two regs for v on its OWN root, distinct in ALL three total-order keys:
		//   lo:  size twoMiB,     version 2,                domain 0x11
		//   hi:  size 2*twoMiB,   version BlockVersionRegGate(3), domain 0x22
		// hi has the larger size, so hi is the canonical winner in both orderings.
		lo := bondRegFull(v, rootV, twoMiB, prev, 2, 0x11)
		hi := bondRegFull(v, rootV, 2*twoMiB, prev, BlockVersionRegGate, 0x22)
		regs := []BondReg{lo, hi} // hi last
		if hiFirst {
			regs = []BondReg{hi, lo} // hi first
		}
		b := &Block{Version: BlockVersionRounds, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		commitRounds(b, all, 0)
		return c, c.Append(*b)
	}

	a, errA := build(true)  // hi first
	b, errB := build(false) // hi last

	// Premise: the block is ADMISSIBLE in BOTH orderings — the certified fix
	// CANONICALIZES the commit, it does NOT reject the block (reject was refuted,
	// it breaks the legal resize). If a future validity change rejects it, this
	// guard fires and names the premise break.
	if errA != nil || errB != nil {
		t.Fatalf("the same-id two-version block is no longer ADMISSIBLE in both orderings "+
			"(errA=%v errB=%v) — the certified fix canonicalizes at APPLY, it does not "+
			"reject at validity. A rejection here means the resolution drifted from the "+
			"cert (§3(a): canonicalize, not reject); STOP and re-derive.", errA, errB)
	}

	v := idOf(key(90001))
	// The certified property: the committed same-id-slot fields are BYTE-IDENTICAL
	// across the two intra-block orderings — the fold makes the winner a pure
	// function of block content, not slice position.
	if a.regVersion[v] != b.regVersion[v] {
		t.Fatalf("regVersion DIVERGED across intra-block orderings (%d vs %d) — the "+
			"canonicalBondRegs fold in apply() is not order-independent. This is the "+
			"#618-class fork the cert gates on; the fix has regressed.",
			a.regVersion[v], b.regVersion[v])
	}
	if a.bondDomain[v] != b.bondDomain[v] {
		t.Fatalf("bondDomain DIVERGED across intra-block orderings (%#x vs %#x) — same "+
			"as above; the fold must select ONE winner and take ALL its fields.",
			a.bondDomain[v], b.bondDomain[v])
	}
	if a.bonded[v] != b.bonded[v] {
		t.Fatalf("bonded DIVERGED across intra-block orderings (%d vs %d) — same as above",
			a.bonded[v], b.bonded[v])
	}

	// The winner is the largest-size reg (hi): version 3, domain 0x22, size 2*twoMiB.
	// This pins the total order's semantics, not merely convergence — a fold that
	// converged on the WRONG reg (e.g. smallest size) would pass the equality checks
	// above but fail here.
	if a.regVersion[v] != BlockVersionRegGate {
		t.Fatalf("canonical winner is not the largest-size reg: regVersion=%d want %d "+
			"(hi: version 3) — the total order (largest Size, then Version, …) picked wrong",
			a.regVersion[v], BlockVersionRegGate)
	}
	if a.bondDomain[v] != 0x22 {
		t.Fatalf("canonical winner domain=%#x want 0x22 (hi) — total order picked wrong", a.bondDomain[v])
	}
	if a.bonded[v] != 2*twoMiB {
		t.Fatalf("canonical winner size=%d want %d (hi, the larger reg — resize is monotone)",
			a.bonded[v], 2*twoMiB)
	}
	t.Logf("same-id two-version intra-block reg is order-INDEPENDENT: both orderings "+
		"commit regVersion=%d bondDomain=%#x bonded=%d (the largest-Size winner)",
		a.regVersion[v], a.bondDomain[v], a.bonded[v])

	// Sanity: the whole committed bond state matches, not just the two-version
	// validator's fields — confirms the fold did not perturb the governors.
	for _, name := range []string{"regVersion", "bondDomain", "bonded", "bondRootOwner",
		"bondRootProven", "bondRegHeight"} {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("bond field %q DIFFERS across the two orderings beyond the two-version "+
				"validator — the canonicalization is order-sensitive somewhere else:\n"+
				"  hiFirst: %v\n  hiLast:  %v", name, fieldValue(a, name), fieldValue(b, name))
		}
	}
}
