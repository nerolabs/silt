package chain

import (
	"crypto/ed25519"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// CONSENSUS FINDING (2026-08-28, the #506-gate order-independence increment):
// the committedSet field `regVersion` is ORDER-DEPENDENT on intra-block BondReg
// slice order, and so is `bondDomain`. This is a #618-class fork: two honest
// replicas that apply the IDENTICAL admissible block in a different BondReg slice
// order commit DIFFERENT committed state, hence a different history-independent
// SMT root, hence a consensus split.
//
// MECHANISM (read from source, chain.go):
//   - apply() iterates b.BondRegs in slice order and does, per reg:
//        c.regVersion[id] = r.Version   (chain.go:2842, "latest committed reg governs")
//        c.bondDomain[id] = r.Domain    (chain.go:2843, "latest wins")
//     So for a SINGLE id carrying two regs in one block, LAST-IN-SLICE-ORDER wins.
//   - The same-id-twice-in-one-block guard (seenReg, chain.go:1464-1467, 1493-1496)
//     is GATE-GATED: it is only allocated and checked when regGateActive(b.Height)
//     is true. The gate is active only AFTER lock-in (regGateActive, chain.go:2945).
//     So BEFORE the gate locks, a block with two regs for the same id on its OWN
//     root is ADMISSIBLE on the validated path.
//   - The #618 seenRoot guard (chain.go:1482-1488) does NOT catch this: it rejects
//     DISTINCT ids on the SAME root; a validator re-registering its OWN root
//     (same id: renew/resize) is explicitly legal (F1) and stays admitted.
//
// WHY IT MATTERS: regVersion feeds the #506 gate lock-in tally in rotateEpoch
// (chain.go:2922-2934): `if c.regVersion[id] >= BlockVersionRegGate { ready += w }`.
// A validator whose committed version is order-dependent can therefore make the
// GATE (gateLockedIn / gateHeight — themselves committedSet) order-dependent when
// its weight is the >2/3 swing. Even absent the swing, regVersion and bondDomain
// are directly under the SMT root, so the split is already a fork.
//
// STATUS: ROUTED to Researcher + human (a consensus-rule / validity-layer change,
// above the Builder seat per the research gate). NO rule is changed here. This
// test ASSERTS THE OBSERVED (current) behavior so:
//   (1) the finding is on the record with an executable repro, and
//   (2) the suite stays green until the certified fix lands, at which point this
//       test FLIPS (both orderings converge → the reflect.DeepEqual becomes true)
//       and must be rewritten into the order-independence oracle as coverage.
//
// The certified resolution will most likely mirror #618: reject the divergent
// input at VALIDITY (make the same-id-twice-in-one-block guard UNCONDITIONAL, not
// gate-gated — the same shape as making seenRoot unconditional), so the
// order-dependent commit never enters the chain. That decision is the
// Researcher's + human's, not the Builder's.
func TestRegVersionIntraBlockOrderFinding(t *testing.T) {
	// build commits ONE pre-gate block carrying two regs for validator v on its own
	// root: v=2 and v=BlockVersionRegGate(3). v3Last controls slice order.
	build := func(v3Last bool) (*Chain, error) {
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
		low := bondRegFull(v, rootV, twoMiB, prev, 2, 0x11)
		high := bondRegFull(v, rootV, twoMiB, prev, BlockVersionRegGate, 0x22)
		regs := []BondReg{high, low} // v3 first → v=2 wins
		if v3Last {
			regs = []BondReg{low, high} // v3 last → v=3 wins
		}
		b := &Block{Version: BlockVersionRounds, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		commitRounds(b, all, 0)
		return c, c.Append(*b)
	}

	a, errA := build(true)  // v=3 last  → regVersion[v]=3, bondDomain[v]=0x22
	b, errB := build(false) // v=3 first → regVersion[v]=2, bondDomain[v]=0x11

	// Premise: the block is ADMISSIBLE in BOTH orderings. If a future validity
	// change rejects it, this guard fires and the finding is (correctly) closed —
	// rewrite this test into order-independence coverage at that point.
	if errA != nil || errB != nil {
		t.Fatalf("PREMISE CHANGED: the same-id-two-version block is no longer admissible "+
			"in both orderings (errA=%v errB=%v). If a certified validity fix now rejects "+
			"it, this finding is closed — move regVersion/bondDomain into the "+
			"order-independence oracle as covered fields.", errA, errB)
	}

	v := idOf(key(90001))
	// The OBSERVED divergence — asserted as the current (buggy) truth so the finding
	// is executable and the suite stays green pending the certified fix.
	if a.regVersion[v] == b.regVersion[v] {
		t.Fatalf("regVersion CONVERGED (%d == %d) — the finding appears FIXED. Re-derive: "+
			"the same-id-two-version intra-block slice order should still pick last-in-slice "+
			"unless a validity/apply change landed. If fixed, move this into the oracle.",
			a.regVersion[v], b.regVersion[v])
	}
	if a.bondDomain[v] == b.bondDomain[v] {
		t.Fatalf("bondDomain CONVERGED (%#x == %#x) — same as above", a.bondDomain[v], b.bondDomain[v])
	}
	t.Logf("FINDING REPRODUCED (routed, not fixed here): committedSet fields are "+
		"intra-block-slice-order dependent for a same-id two-version reg —\n"+
		"  regVersion[v]: v3Last=%d  v3First=%d\n"+
		"  bondDomain[v]: v3Last=%#x  v3First=%#x\n"+
		"Two honest replicas applying the identical admissible block in different "+
		"BondReg order commit different SMT-committed state (a #618-class fork).",
		a.regVersion[v], b.regVersion[v], a.bondDomain[v], b.bondDomain[v])

	// Sanity: the divergence is ONLY in the two-version validator's fields — the
	// governors' state matches, confirming the finding is exactly the same-id seam
	// and not a broader fixture asymmetry.
	delete(a.regVersion, v)
	delete(b.regVersion, v)
	if !reflect.DeepEqual(a.regVersion, b.regVersion) {
		t.Fatalf("unexpected divergence beyond the two-version validator: %v vs %v — "+
			"the finding is wider than the same-id seam; re-scope before routing",
			a.regVersion, b.regVersion)
	}
}
