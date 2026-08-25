package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #561 — the a434494-deep 10a-stall-drill FAIL, made deterministic.
//
// FIELD OBSERVATION: with 4 of 12 peers stopped, the honest cohort's first
// h72 round-change came ~8 minutes after the stall began — against a 430 s
// computed bound built on 30 s sweeps. Renewals kept flowing every 30 s
// (proving the tick fired and pending work existed), but the round-escape
// counter (maybeAdvanceRound) lived INSIDE SyncChain's completion callback,
// which fires only after the sequential ask-walk over ALL peers completes —
// and dead peers stretch that walk by their full retry budgets. The escape
// therefore ran at dead-peer speed, not tick speed. This is the #456 class
// (certified fixed for the two-phase gather; SyncChain's walk kept the
// sequential shape) — and it silently BROKE the #549 Q3 derivation, which
// bounds cross-node round-timeout skew by ChainSyncInterval on the premise
// that the escape runs once per tick.
//
// THE ORACLE: with pending consensus work and a peer that NEVER answers
// (held delivery — the walk never completes), the round-escape must still
// fire after roundAdvanceSweeps ticks of the chain-sync clock. RED pre-fix:
// the callback never runs, the counter never increments, the node sits at
// round 0 forever. GREEN: the escape counts TICKS (local state only) and
// advances on schedule; the drain keeps its freshest-head property by
// staying in the callback (#338).
func TestRoundEscapeCountsTicksNotCompletedWalks_561(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	nd := nodes[0]

	// Premise: mature epoch, pending work armed (the escape's trigger), and a
	// network that never delivers — every request parks (the model of a walk
	// stalled on unresponsive peers).
	if !nd.chain.EverMature() {
		t.Fatal("premise: not latched")
	}
	refill()
	if len(nd.pendingBondRegs) == 0 {
		t.Fatal("premise: no pending work — the escape would rightly quiesce (B6)")
	}
	_ = ids
	_ = net // held-delivery mode: matureWorld12 enabled it; nothing is ever delivered here

	if r := nd.roundsFor().Round; r != 0 {
		t.Fatalf("premise: round %d, want 0", r)
	}

	// Start the periodic sweep on the sim clock and run PAST the escape bound:
	// roundAdvanceSweeps ticks (+1 for the immediate boot tick's phase). Every
	// timer fires; no message is ever delivered, so the SyncChain callback
	// never runs.
	nd.StartChainSync(nd.chainSyncSeed, nil)
	deadline := sched.Now().Add((ports.Duration(roundAdvanceSweeps) + 2) * nd.cfg.ChainSyncInterval)
	for sched.Now() < deadline && sched.Step() {
		refill() // keep the queue armed across ticks (renewals do this in the field)
	}

	if r := nd.roundsFor().Round; r == 0 {
		t.Fatalf("#561 REPRODUCED: after %d chain-sync ticks with pending work and an unresponsive peer walk, the node never left round 0 — maybeAdvanceRound runs only in SyncChain's completion callback, so dead/slow peers stall the round-escape indefinitely (field: first round-change ~8min vs the 430s bound). The escape must count TICKS.", roundAdvanceSweeps+2)
	}
}
