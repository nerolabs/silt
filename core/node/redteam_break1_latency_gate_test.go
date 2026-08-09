package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestBreak1_LateBondAnswerEarnsNoStanding pins the enforcing leg of the C1
// partial-storage-recompute residual (BREAK 1, red-team 2026-08-08; owned-residuals
// A5). A prover that deleted part of its plot must RECOMPUTE the dropped blocks on
// demand; the recomputed bytes are content-identical to stored ones, so no CONTENT
// check can catch it (verified against the code) — enforcement is necessarily the
// TIME leg: past the DRSample graph's ~0.25 work knee that recompute is a large
// sequential cost, so a reply slower than BondMaxAnswerLatency earns no standing.
//
// The gate is a reply-latency bound on the LIVE peer audit. This test drives it
// deterministically over the sim network (whose latency stands in for the prover's
// answer time — the sim has no wall-clock compute): with the round-trip inside the
// deadline the honest prover earns standing; with it past the deadline the same
// valid answer earns none. It also confirms the gate is OFF when the deadline is 0
// (the default the deterministic sim/tests rely on).
func TestBreak1_LateBondAnswerEarnsNoStanding(t *testing.T) {
	const bondSize = int64(8) << 20 // clears the (unset) floor

	// standingUnderLatency runs one peer audit where every hop takes `hop` and the
	// challenger's answer deadline is `deadline`, returning the standing the prover
	// earned on the challenger's ledger.
	standingUnderLatency := func(t *testing.T, hop, deadline ports.Duration) int64 {
		t.Helper()
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.Config{LatencyMin: hop, LatencyMax: hop})

		proverID := identity.FromSeed(311)
		chalID := identity.FromSeed(312)

		prover := New(proverID.NodeID(), DefaultConfig(), sched, net.Endpoint(proverID.NodeID()), memstore.New())
		prover.SetLedger(credit.New(50_000, 0))
		prover.EnableBond(proverID.Signer(), bondSize) // seals a REAL, honest plot

		ccfg := DefaultConfig()
		ccfg.BondMaxAnswerLatency = deadline
		chal := New(chalID.NodeID(), ccfg, sched, net.Endpoint(chalID.NodeID()), memstore.New())
		chalLedger := credit.New(50_000, 0)
		chal.SetLedger(chalLedger)

		// The challenger learns the prover's advertised bond off the wire (BondRoot
		// rides every message), then challenges it.
		chal.Bootstrap([]ports.NodeID{proverID.NodeID()}, func() {})
		sched.Run()
		chal.AuditBondsOnce()
		sched.Run()
		return chalLedger.Reputation(proverID.NodeID())
	}

	// Fast honest reply, deadline enforced: the round-trip fits, so the valid answer
	// earns standing.
	if got := standingUnderLatency(t, 1 /*hop*/, 1000 /*deadline*/); got <= 0 {
		t.Fatalf("an honest prover replying inside the deadline must earn standing, got %d", got)
	}
	// Slow reply, deadline enforced: the same valid answer arrives past the deadline
	// (as a materially-short recompute prover's would past the knee) → scored as a
	// failed challenge, so NO (positive) standing — like a liar that can't prove
	// sustained possession.
	if got := standingUnderLatency(t, 500 /*hop → ~1000 round-trip*/, 200 /*deadline*/); got > 0 {
		t.Fatalf("BREAK 1: a bond answer slower than the deadline must earn NO standing, got %d", got)
	}
	// Deadline 0 (the sim/test default) disables the gate: even a slow reply earns
	// standing, so the deterministic sim is unaffected.
	if got := standingUnderLatency(t, 500 /*slow*/, 0 /*gate off*/); got <= 0 {
		t.Fatalf("with the gate off (deadline 0) a valid answer must still earn standing, got %d", got)
	}
}
