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

// TestC1TimingIsSoftNotAHardGate pins the CORRECTED C1 partial-storage residual
// (owned-residual A5; build-immutable #3, "never fuse transport with security").
//
// A prover that deleted part of its plot must RECOMPUTE the dropped blocks on
// demand, which past the DRSample work knee is a sequential cost that shows up as
// reply latency. The OLD design gated standing on a single wall-clock reply
// deadline (`!late`) — but reply-latency is transport (RTT + jitter + loss) PLUS
// compute, and gating security on the sum is unsound on the open internet: it read
// network jitter/loss as a cheat and starved durability (#289). Network delay is
// one-sided (it can only ADD latency), so "slow ⇒ cheat" cannot be concluded
// through noise the prover doesn't control.
//
// So the timing signal is now SOFT: a valid answer earns standing regardless of
// how slow it arrives; the deterrent is the windowed-MINIMUM (low-quantile) of a
// peer's reply latencies — filtering the one-sided noise — flagged only when it is
// SUSTAINED above the deadline (a partial-storage prover recomputes on EVERY
// challenge, so its floor stays elevated; an honest node on a bad path is only
// randomly slow). The flag is disclosed, never a standing gate.
func TestC1TimingIsSoftNotAHardGate(t *testing.T) {
	const bondSize = int64(8) << 20 // clears the (unset) floor

	// standingUnderLatency runs `rounds` peer audits where every hop takes `hop`
	// and the challenger's answer deadline is `deadline`, returning the standing the
	// prover earned and the challenger's soft floor read for it.
	standingUnderLatency := func(t *testing.T, hop, deadline ports.Duration, rounds int) (int64, ports.Duration, bool) {
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

		chal.Bootstrap([]ports.NodeID{proverID.NodeID()}, func() {})
		sched.Run()
		for i := 0; i < rounds; i++ {
			chal.AuditBondsOnce()
			sched.Run()
			// Real bond audits are one per BondAuditInterval; advance a full window
			// between rounds so each is its own audit window. Without this the tight
			// loop compresses latWindowSize+2 audits into a single window, where the
			// prover's per-challenger rate-limit (#424) refuses the excess — an
			// artifact of the test's time compression, not the behavior under the
			// real once-per-interval cadence this test means to model.
			sched.AfterFunc(ccfg.BondAuditInterval, func() {})
			sched.Run()
		}
		floor, sustained, _ := chal.BondLatencyFloor(proverID.NodeID())
		_ = floor
		return chalLedger.Reputation(proverID.NodeID()), floor, sustained
	}

	// Fast honest reply, deadline enforced: earns standing (unchanged).
	if got, _, _ := standingUnderLatency(t, 1 /*hop*/, 1000 /*deadline*/, 1); got <= 0 {
		t.Fatalf("an honest prover replying inside the deadline must earn standing, got %d", got)
	}
	// SLOW reply past the deadline: the same valid answer STILL earns standing — the
	// hard gate is gone (#289). This is the assertion the old BREAK-1 test had
	// inverted; a bad network path must never cost standing.
	if got, _, _ := standingUnderLatency(t, 500 /*hop → ~1000 round-trip*/, 200 /*deadline*/, 1); got <= 0 {
		t.Fatalf("a valid but slow answer must STILL earn standing (timing is soft, not a gate), got %d", got)
	}
	// Deadline 0 (the sim/test default) disables the signal entirely.
	if got, _, _ := standingUnderLatency(t, 500 /*slow*/, 0 /*off*/, 1); got <= 0 {
		t.Fatalf("with the timing signal off (deadline 0) a valid answer must earn standing, got %d", got)
	}
	// SUSTAINED slowness (a warm window all past the deadline): the soft floor is
	// flagged for disclosure, but the prover STILL earns standing — the flag never
	// gates. This is the partial-storage prover's fingerprint, surfaced without
	// punishing an honest slow path.
	got, floor, sustained := standingUnderLatency(t, 500 /*slow every round*/, 200 /*deadline*/, latWindowSize+2)
	if got <= 0 {
		t.Fatalf("a sustained-slow but valid prover must STILL earn standing (soft signal is non-gating), got %d", got)
	}
	if !sustained || floor <= 200 {
		t.Fatalf("a consistently-slow peer must raise the soft sustained-latency flag (floor %d > deadline 200), sustained=%v", floor, sustained)
	}
}
