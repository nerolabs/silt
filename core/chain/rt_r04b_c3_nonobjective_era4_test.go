package chain

// R0.4b C3 final round — the trace behind the e2e re-scope, PINNED AS A TEST.
//
// THE CLAIM (G-8 convergence §1, 2026-09-03): on a `-objective=false` topology the
// era-4 readiness tally can NEVER latch, at ANY readiness stamp, on ANY binary. The
// convergence derived it from source and marked it explicitly as "a trace, not a run",
// asking that it be confirmed by injecting a stamp of 5 and observing MintVersion stay
// below 5. This is that run.
//
// THE MECHANISM, in one line: epochsEnabled() is `EpochBlocks > 0 && objective()`
// (chain.go), apply() calls rotateEpoch ONLY under that predicate, and the era-4
// readiness tally lives INSIDE rotateEpoch — so a non-objective chain never evaluates
// the tally at all, however loudly its validators signal.
//
// WHY IT IS NAMED FOR GATE F. The stamp raise (NewBondReg's Version, pinned by
// gateFStampPin) is the release edit that turns the R0.4b rollout rule live. Whoever
// makes that edit re-reads the gate-F family, and this is the test that tells them
// raising the stamp does NOT green
// e2e TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding: that fixture needs an
// OBJECTIVE, bonded, epoch-enabled topology first (residual R-E2E-ERA4-FIXTURE), or
// the paid lane stays dark no matter what the constant says.

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestGateF_NonObjectiveTopologyCanNeverLatchEra4 injects the readiness stamp the
// release will one day ship (5) onto two chains that differ in ONE property —
// objectivity — and shows the tally is gated on that property and not on the stamp.
func TestGateF_NonObjectiveTopologyCanNeverLatchEra4(t *testing.T) {
	whale := key(94501)
	minnows := []ed25519.PrivateKey{key(94502), key(94503), key(94504)}

	// The stamp under test: BlockVersionWitnessable, injected through the same field
	// NewBondReg stamps (BondReg.Version -> c.regVersion). This is the value a
	// stamp-raising release would put there.
	const injectedStamp = BlockVersionWitnessable

	build := func(t *testing.T, objective bool) *Chain {
		t.Helper()
		cfg := Config{Quorum: 1, EpochBlocks: 4, BondTTLBlocks: 64}
		if objective {
			// The ONLY difference between the two chains. objective() is
			// `MinBond > 0 && verifyBond != nil`; both halves are set here and
			// neither is set below.
			cfg.MinBond = 1 << 20
			cfg.ByzantineQuorum = true
		}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		if objective {
			c.SetBondVerifier(objectiveVerify)
		}
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, injectedStamp))
		for _, m := range minnows {
			g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, injectedStamp))
		}
		Sign(g, whale)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		// The era-4 tally runs at every epoch boundary INCLUDING genesis (boundary 0),
		// so an all-ready set locks in there and H_era4 = 0 + EpochBlocks = 4. The v1
		// commit() helper cannot build a v4/v5 block, so stop below H_era4 — which is
		// enough: the latch has already happened or it never will.
		for c.Len() < 4 {
			commit(t, c, whale, minnows)
		}
		return c
	}

	// --- CONTROL: the objective chain LATCHES. Without this arm the assertion below
	// would pass on a harness that simply never signals, and would prove nothing.
	ctl := build(t, true)
	if !ctl.era4LockedIn {
		t.Fatalf("CONTROL BROKEN: an objective, epoch-enabled chain whose whole set signals "+
			"readiness %d must lock era-4 in — if it does not, the arm below is vacuous and "+
			"this test proves nothing", injectedStamp)
	}
	if v := ctl.MintVersion(ctl.era4Height); v != BlockVersionWitnessable {
		t.Fatalf("CONTROL BROKEN: at H_era4 the objective chain must mint v%d, got v%d",
			BlockVersionWitnessable, v)
	}

	// --- THE ARM: the same set, the same stamp, NOT objective. No latch, ever.
	dark := build(t, false)
	if dark.objective() {
		t.Fatal("fixture: the dark arm is objective — the two arms do not differ in the " +
			"property under test")
	}
	if dark.epochsEnabled() {
		t.Fatal("fixture: epochsEnabled() is true on a non-objective chain — the mechanism " +
			"this test pins has changed and the trace behind the e2e re-scope needs redoing")
	}
	for _, id := range append([]ports.NodeID{idOf(whale)}, idOf(minnows[0]), idOf(minnows[1]), idOf(minnows[2])) {
		if got := dark.regVersion[id]; got != injectedStamp {
			t.Fatalf("fixture: %s signalled readiness %d, want the injected %d — the stamp "+
				"never reached the tally's input, so a null result here is meaningless",
				id, got, injectedStamp)
		}
	}
	if dark.era4LockedIn {
		t.Fatal("era-4 LOCKED IN on a non-objective chain. The tally lives inside " +
			"rotateEpoch, which apply() calls only under epochsEnabled() — if it can latch " +
			"here, a -objective=false daemon can mint v5 and the e2e re-scope's premise is " +
			"wrong")
	}
	// The property the convergence asked to be confirmed by run: MintVersion stays
	// below 5 at every height, including well past where the objective control latched.
	for h := uint64(0); h <= 64; h++ {
		if v := dark.MintVersion(h); v >= BlockVersionWitnessable {
			t.Fatalf("MintVersion(%d) = v%d on a -objective=false chain with every "+
				"validator signalling readiness %d. The G-8 convergence's trace is wrong and "+
				"the e2e paid-lane fixture could have been greened by the stamp raise alone",
				h, v, injectedStamp)
		}
	}
}
