package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// #549 repro — the DEEP field run 1f77c5e-deep h68 stall, made deterministic.
//
// FIELD OBSERVATION (attributed, GitHub #549): after the B2 capture drill + WS
// cold-sync mass-restarted 8 of 12 consensus seats, the deep drive reached h67
// (converged) then STALLED at h68 for ~26 minutes. All 9 frozen members were
// actively sending round-changes (nobody silent/unreachable), but they SMEARED
// across rounds (r0=38, r1=231, r2=72, r3=4 — congested at r1) and NO prepare-QC
// ever formed. Ruled out: #535-boundary (h68 non-boundary), memory (RSS 1.22G),
// safety (h67 converged), the natted relay node, and the negative-standing sybil
// (ledger reputation, non-consensus in objective mode).
//
// THE DISTINGUISHING DIMENSION from the existing GREEN #451 staggered-sweep
// oracle (TestModelCheck_451_StaggeredSweepsMustStillConverge): that test
// PARTITIONS the sybils out and gradually smears only the 8 heavy members. The
// field kept the 4 SYBILS ACTIVE in the frozen set, inflating the low-round
// round-change HEAD-COUNT (231 msgs at r1) while the heavy VALIDATOR WEIGHT — the
// weight that must form a >⅔ prepare-QC — scattered across rounds. The trigger
// was a MASS restart (8/12 seats resuming scattered persisted rounds at once),
// not a gradual one-at-a-time smear.
//
// THE ORACLE: with the heavy weight mass-scattered across rounds AND the sybils
// active at the low round, does the synchronizer converge the WEIGHT onto a
// common round to commit within a bounded number of rotations? RED reproduces
// the field stall (the deterministic home for a synchronizer fix — candidates:
// weight-aware round-change quorum/catch-up so sybil head-count cannot dilute
// convergence; consensus-rule-adjacent → research-gated, build-immutable #6).
// GREEN means the in-process model cannot reproduce the field mechanism at this
// shape — itself decisive for the research consult (the trigger is a real-WAN
// scale/timing dimension the untimed model does not capture; the #451 harness
// records this same "GREEN is evidence too" property).

// scatterToRound drives one node's own round clock to `target` in isolation
// (all delivery held), so its round-changes do not cross-record on the others
// during setup — the model of a restarted node resuming at a scattered
// persisted round. maybeAdvanceRound advances when Sweeps ≥ sweepsForRound(r),
// so reaching r costs Σ_{i<r} sweepsForRound(i) local sweeps.
func scatterToRound(t *testing.T, net *simnet.Network, nd *Node, refill func(), target uint64) {
	t.Helper()
	holdAll := func(simnet.HeldMsg) bool { return true }
	for guard := 0; guard < 200; guard++ {
		if rs := nd.roundsFor(); rs.Round >= target {
			return
		}
		refill()
		nd.maybeAdvanceRound()
		drainHeldExcept(t, net, holdAll) // hold everything — no cross-node recording
	}
	t.Fatalf("scatterToRound: node did not reach round %d", target)
}

func TestModelCheck_549_MixedWeightScatterMustConverge(t *testing.T) {
	nodes, ids, net, _, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	sybilSet := map[ports.NodeID]bool{}
	for _, id := range all[8:] { // last 4 are the 1M sybils
		sybilSet[id] = true
	}
	var honestSlashed bool
	for i, nd := range nodes {
		if i < 8 {
			nd.OnSlash(func(culprit ports.NodeID, _ uint64) {
				if !sybilSet[culprit] {
					honestSlashed = true
				}
			})
		}
	}

	// Premise: the 12-member mature epoch governs (matureWorld12 drove to the h8
	// boundary). The contested height is head+1; ALL 12 stay active — the field
	// kept the sybils in the set (unlike the #451 staggered test's partition).
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	_, contested := nodes[0].chain.Head()

	// THE MASS-RESTART SCATTER (the field trigger, deterministic): the 8 HEAVY
	// members (4 anchors + 4 maturers, 64M each) resume at SPLIT rounds — half at
	// r1, half at r2 — while the 4 SYBILS pile at r1, inflating the low-round
	// round-change head-count exactly as the field's 231-msgs-at-r1 did. Delivery
	// is held throughout so the scatter is established before any catch-up fires.
	refill()
	for i := 0; i < 4; i++ { // anchors 0-3 → r1
		scatterToRound(t, net, nodes[i], refill, 1)
	}
	for i := 4; i < 8; i++ { // maturers → r2 (the heavy weight is SPLIT r1 vs r2)
		scatterToRound(t, net, nodes[i], refill, 2)
	}
	for i := 8; i < 12; i++ { // sybils → r1 (head-count inflation at the low round)
		scatterToRound(t, net, nodes[i], refill, 1)
	}

	// Sanity: the heavy weight is genuinely split across rounds (no single round
	// holds a >⅔-weight majority yet), and the sybils sit at the low round.
	r1heavy, r2heavy := 0, 0
	for i := 0; i < 8; i++ {
		switch nodes[i].roundsFor().Round {
		case 1:
			r1heavy++
		case 2:
			r2heavy++
		}
	}
	if r1heavy == 0 || r2heavy == 0 {
		t.Fatalf("setup: heavy weight not split (r1heavy=%d r2heavy=%d)", r1heavy, r2heavy)
	}

	// PHASE 1 — SUSTAINED LOSS (the field's WAN reality, deterministic): the
	// mass restart's churn keeps delivery unstable, so half the round-changes are
	// DROPPED ON THE WIRE each rotation. This is the faithful "GST never cleanly
	// arrives" condition — the catch-up sees STALE round positions (a node
	// broadcasts r2, it's lost, recipients act on old state), exactly the field's
	// 26-minute smear where all members participated but never co-round-ed.
	// Every-other round-change is dropped by a deterministic parity toggle.
	lossyRotations := 6
	drop := true
	dropRoundChanges := func() {
		for _, m := range net.Pending() {
			if m.Kind == ports.MsgRoundChange {
				drop = !drop
				if drop {
					net.DropPending(m.ID)
				}
			}
		}
	}
	committed := false
	for rotation := 0; rotation < lossyRotations && !committed; rotation++ {
		refill()
		for _, nd := range nodes {
			nd.maybeProposeBondDrain()
			dropRoundChanges()
			drainHeld(t, net, fifo)
		}
		for _, nd := range nodes {
			nd.maybeAdvanceRound()
			dropRoundChanges()
			drainHeld(t, net, fifo)
			nd.maybeCatchUpRound(nd.roundsFor())
			dropRoundChanges()
			drainHeld(t, net, fifo)
		}
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		if _, h := nodes[0].chain.Head(); h > contested {
			committed = true
			t.Logf("converged UNDER LOSS: h%d committed within lossy rotation %d", contested, rotation)
		}
	}

	// PHASE 2 — GST: delivery stabilizes. The certified after-GST guarantee is
	// that the increasing round duration + weight catch-up now converge the
	// weight within a bounded number of clean rotations.
	const gstBudget = 4
	for rotation := 0; rotation < gstBudget && !committed; rotation++ {
		refill()
		sweepRounds(t, net, func(simnet.HeldMsg) bool { return false }, nodes)
		for _, nd := range nodes {
			nd.maybeCatchUpRound(nd.roundsFor())
			drainHeld(t, net, fifo)
		}
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		if _, h := nodes[0].chain.Head(); h > contested {
			committed = true
			t.Logf("converged: h%d committed at GST rotation %d (after %d lossy rotations)", contested, rotation, lossyRotations)
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the mixed-weight scatter")
	}
	if !committed {
		maxRound := uint64(0)
		roundOf := map[uint64]int{}
		for _, nd := range nodes {
			r := nd.roundsFor().Round
			roundOf[r]++
			if r > maxRound {
				maxRound = r
			}
		}
		t.Fatalf("#549 REPRODUCED IN-PROCESS: the contested height %d did not commit within %d lossy + %d GST rotations with the heavy weight mass-scattered (r1heavy=%d r2heavy=%d) and the sybils active at the low round — no round held >⅔ of the WEIGHT with its designated proposer simultaneously, so no prepare-QC formed (final round histogram %v, max round %d). If this ever goes RED it is the deterministic home for the field h68 stall — a synchronizer-LOGIC fix belongs here.",
			contested, lossyRotations, gstBudget, r1heavy, r2heavy, roundOf, maxRound)
	}
	// GREEN (the observed result): the weight-aware synchronizer COLLAPSES the
	// mixed-weight scatter even under sustained round-change loss. Prove it was a
	// real scatter that had to be converged (committed at round > 0), not a
	// trivial r0 commit — so the GREEN genuinely exercises the convergence.
	blk := nodes[0].Chain().Blocks(contested)
	if len(blk) == 0 {
		t.Fatalf("committed flag set but height %d not in the chain", contested)
	}
	if blk[0].CommitRound == 0 {
		t.Fatalf("the height committed at round 0 — the scatter did not actually exercise the synchronizer (r1heavy=%d r2heavy=%d); the oracle is not testing convergence", r1heavy, r2heavy)
	}
	// NOTE (corrected by the research certification 2026-08-24): this untimed
	// GREEN does NOT mean the synchronizer logic is sound — it means this oracle
	// cannot ISOLATE the catch-up-target defect. Advances here are driven by
	// timeouts (maybeAdvanceRound), which reach a committing round regardless of
	// whether catch-up jumps to the smallest or the highest round; and the
	// untimed sim cannot model the wall-clock timer skew that produced the field
	// smear. The certification found a REAL synchronizer-logic defect (catch-up
	// jumped to the smallest round of a cross-round union — a sub-threshold round
	// that cannot form a QC — pinning the duration ladder low). That defect and
	// its fix are proven deterministically by TestModelCheck_549_CatchUpJumps-
	// ToHighestQualifyingRound. This test remains a useful convergence regression
	// guard (weight-aware catch-up converges a mixed-weight scatter), but it is
	// NOT evidence the logic was correct.
	t.Logf("#549: mixed-weight scatter converged (h%d at round %d) — a convergence regression guard; the catch-up-target defect is isolated by TestModelCheck_549_CatchUpJumpsToHighestQualifyingRound", contested, blk[0].CommitRound)
}
