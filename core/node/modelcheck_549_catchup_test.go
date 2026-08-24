package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #549 catch-up-target oracle — the deterministic RED/GREEN home the research
// certification (549-h68-view-synchronization-stall-RESEARCH-CERTIFICATION,
// 2026-08-24) mandates before the synchronizer rule moves.
//
// THE CERTIFIED DEFECT (rounds.go maybeCatchUpRound): the responsive catch-up
// UNIONED round-change senders across ALL rounds above the current one, checked
// the weight threshold on that UNION, then jumped to the SMALLEST such round.
// Because the round-duration ladder is keyed to the round NUMBER
// (duration(r) = base + r(r+1)/2), targeting the smallest above-round PINS the
// effective round low, so the increasing duration never outruns 3-region WAN +
// 30s timer skew, so the after-GST convergence guarantee never engages — the
// field's h68 26-minute r1-congestion thrash.
//
// THE CERTIFIED FIX: jump to the HIGHEST round that INDIVIDUALLY meets the
// catch-up weight threshold (coalesce the weight at the LEADING edge, let the
// ladder climb), not the smallest round of the cross-round-aggregated union.
//
// WHY THIS DIRECT ORACLE (not a full timed sim): a full skewed-timer sim
// advances mostly via TIMEOUTS (maybeAdvanceRound), which reach a high round
// regardless of the catch-up target — so it does not ISOLATE the target choice
// (empirically GREEN both ways). This oracle drives the real maybeCatchUpRound
// over the exact adversarial input the defect mishandles — a smear with a
// LOW-weight trailing round and a QUORUM-weight leading round — and asserts the
// jump target. It is the precise deterministic distinguisher: RED (jumps to the
// low trailing round, which cannot commit) on the union-smallest rule, GREEN
// (jumps to the qualifying leading round) on the fix.

func TestModelCheck_549_CatchUpJumpsToHighestQualifyingRound(t *testing.T) {
	nodes, ids, _, _, _ := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	// all[0:4] = anchors (64M), all[4:8] = maturers (64M), all[8:12] = sybils (1M);
	// frozen epoch total 516M, so >⅓ (the catch-up threshold RoundCatchupMet
	// applies) is 172M. A node works on head+1; put it at round 1, below the smear.
	nd := nodes[0]
	rs := nd.roundsFor()
	rs.Round = 1

	// THE SMEAR (the field's shape): a LOW trailing round (r2) carrying only
	// sub-threshold sybil weight, and a HIGH leading round (r3) carrying a
	// super-threshold heavy coalition — where the weight has actually gathered.
	trailing := map[ports.NodeID]bool{all[8]: true, all[9]: true}                // 2 sybils = 2M  (< ⅓, cannot anchor a QC)
	leading := map[ports.NodeID]bool{all[1]: true, all[2]: true, all[3]: true}   // 3 anchors = 192M (> ⅓, the leading edge)

	// Sanity: the threshold sees exactly what the oracle intends — r2 does NOT
	// individually qualify, r3 DOES; and the UNION qualifies (which is what let
	// the old rule fire and then mis-target the smallest).
	if nd.chain.RoundCatchupMet(trailing) {
		t.Fatal("setup: the trailing (sybil) round must NOT individually meet the catch-up weight threshold")
	}
	if !nd.chain.RoundCatchupMet(leading) {
		t.Fatal("setup: the leading (heavy) round MUST individually meet the catch-up weight threshold")
	}
	union := map[ports.NodeID]bool{}
	for id := range trailing {
		union[id] = true
	}
	for id := range leading {
		union[id] = true
	}
	if !nd.chain.RoundCatchupMet(union) {
		t.Fatal("setup: the union must meet the threshold (this is what fired the old union-smallest rule)")
	}

	// Inject the smear as recorded round-changes: maybeCatchUpRound reads only the
	// SENDER SETS of rs.Changes[r] (the envelopes were verified when recorded), so
	// stub payloads suffice to exercise the target-selection logic.
	rs.Changes[2] = map[ports.NodeID][]byte{}
	for id := range trailing {
		rs.Changes[2][id] = []byte("rc")
	}
	rs.Changes[3] = map[ports.NodeID][]byte{}
	for id := range leading {
		rs.Changes[3][id] = []byte("rc")
	}

	// FIRE the real catch-up.
	nd.maybeCatchUpRound(rs)

	// THE ASSERTION: the node must jump to the HIGHEST individually-qualifying
	// round (r3 — where the weight can form a QC), not the SMALLEST round of the
	// union (r2 — a sub-threshold dead end that pins the ladder low).
	if rs.Round == 2 {
		t.Fatalf("#549 REPRODUCED: catch-up jumped to the SMALLEST round of the union (r2) — a sub-threshold round that cannot form a QC, pinning the duration ladder low (the certified defect). It must jump to the highest INDIVIDUALLY-qualifying round (r3).")
	}
	if rs.Round != 3 {
		t.Fatalf("#549: catch-up must jump to the highest individually-qualifying round r3; landed at round %d", rs.Round)
	}

	// NARROWNESS — the fix does not overshoot and does not fire on sub-threshold
	// weight. With ONLY the trailing sub-threshold round present, catch-up must
	// NOT fire at all (no honest member provably ahead).
	rs2 := nodes[1].roundsFor()
	rs2.Round = 1
	rs2.Changes[2] = map[ports.NodeID][]byte{}
	for id := range trailing {
		rs2.Changes[2][id] = []byte("rc")
	}
	nodes[1].maybeCatchUpRound(rs2)
	if rs2.Round != 1 {
		t.Fatalf("#549: catch-up must NOT fire on a sub-threshold round alone (weight %d < ⅓) — a Byzantine minority must not drag the round forward; landed at round %d", 2, rs2.Round)
	}
}
