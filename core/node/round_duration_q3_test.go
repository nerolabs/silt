package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #549 Q3 derivation guard (research certification 2026-08-24,
// docs/thinking/2026-08-24-549-q3-round-duration.md): roundAdvanceSweeps is not
// a magic constant — it is the SMALLEST base that reliably outruns the
// cross-region sweep-timer skew, which is structurally bounded by
// ChainSyncInterval (each node sweeps once per interval at an arbitrary phase,
// so two nodes' round-change timeout triggers differ by < ChainSyncInterval;
// WAN delivery is negligible on top). This test pins that derivation so the
// parameter can never silently regress below the skew (build-immutables #5
// derived-not-magic, #7 evidence).
//
// #555 checked the OTHER candidate lower bound — the two-phase gather latency
// G (the #555 research certification held the base pending a measured G). The
// field measurement resolved it: intrinsic G ≈ 10 s at 12-seat WAN (95d39e8-deep
// h74: new-view → commit in 9.6 s), well inside the 60 s base; the apparent
// 90–150 s "gather" was event-loop saturation from O(depth × window × scan)
// Block.Hash re-computation on the chain-sync path, fixed by the hash memo
// (chain.TestReconcileHashWorkIsLinear_555). Skew therefore REMAINS the binding
// lower bound and the base stays the certified-minimal 2. See
// docs/thinking/2026-08-25-555-crawl-attribution.md.
func TestRoundBaseOutrunsSkew(t *testing.T) {
	interval := DefaultConfig().ChainSyncInterval
	if interval <= 0 {
		t.Fatal("ChainSyncInterval must be positive for the skew derivation to hold")
	}
	baseDuration := ports.Duration(roundAdvanceSweeps) * interval
	// The max cross-region skew is strictly < ChainSyncInterval (a structural
	// bound: a node sweeps at least once per interval), so ChainSyncInterval is
	// its supremum. The base round duration must STRICTLY exceed it, or a
	// worst-case-late member has zero overlap inside the round and no round can
	// hold a quorum simultaneously — the #451 after-GST convergence guarantee.
	maxSkew := interval
	if baseDuration <= maxSkew {
		t.Fatalf("#549 Q3 VIOLATION: base round duration %v (roundAdvanceSweeps=%d × ChainSyncInterval=%v) must STRICTLY exceed the max sweep-timer skew (%v) — roundAdvanceSweeps=1 (= the skew) has zero overlap margin and the round cannot reliably hold a quorum; the #451 convergence guarantee is at risk. Do not lower roundAdvanceSweeps below 2 without re-deriving against the measured skew.",
			baseDuration, roundAdvanceSweeps, interval, maxSkew)
	}
	// The "not larger" half (the certification's M1 guidance): the base must be
	// the SMALLEST reliable value — 2× the skew. A larger base slows recovery
	// and adds round-change churn on the 2 GB box for no convergence gain. A
	// deliberate raise (e.g. a measured skew increase) updates this guard with
	// the justification.
	if roundAdvanceSweeps != 2 {
		t.Fatalf("#549 Q3: roundAdvanceSweeps=%d is not the certified-minimal 2 (base %v vs the %v skew). Lower risks under-running the skew; higher is 'larger than necessary' (slower recovery + churn on the 2 GB box). If a measurement justifies a change, update the derivation (docs/thinking/2026-08-24-549-q3-round-duration.md) and this guard together.",
			roundAdvanceSweeps, baseDuration, interval)
	}
}
