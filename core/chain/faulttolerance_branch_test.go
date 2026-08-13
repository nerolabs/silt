package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// §2 repro (PE ruling 2026-08-13) — NAME THE BRANCH behind the 6-fault-tolerance
// GAP that appears in the SYBILS=8 runs but not the no-sybil run. The PE's point:
// my consult hypothesis ("banked sybil bonds inflate bftThreshold into the honest
// quorum") does NOT match the code — validatorSetSize() already returns
// len(Anchors) in the objective LAUNCH phase (pre-handoff), so 8 banked sybil
// bonds should NOT change the quorum. This test reproduces the exact committed
// state (4 anchors + 8 banked single-domain sybil bonds, pre-maturity, objective,
// epochs enabled) and reports which validatorSetSize BRANCH fires and the actual
// N / RequiredQuorum, then checks whether a commit with one anchor "down"
// (proposer + 2 anchor attesters) still passes. The answer picks the branch:
//
//	(a) validatorSetSize returns 12 (fall-through to qualifiedCount) → real
//	    quorum-sizing bug: sybil bonds ARE inflating the fault budget pre-handoff.
//	(b) validatorSetSize returns 4, RequiredQuorum 2, and the 2-anchor-attested
//	    commit PASSES in-process → the quorum sizing is CORRECT; the cloud GAP is
//	    a gather-LATENCY effect under the 8-sybil load (same family as §1), NOT a
//	    consensus bug. The fix is then latency/liveness, not a quorum rule.
//	(c) something else (config) — surfaced by the logged values.
func TestFaultToleranceBranch_SybilBondsDoNotInflateLaunchQuorum(t *testing.T) {
	const bond = int64(64) << 20
	// 4 anchor-validators; anchor a1 proposes. All 4 in cfg.Anchors.
	a1, a2, a3, a4 := key(9300), key(9301), key(9302), key(9303)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true, idOf(a3): true, idOf(a4): true}
	// The cloud sybil topology config: objective, epochs on (default), anchors set,
	// mature-validators 4, byzantine-quorum on, quorum floor 2 (= n_val-2).
	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 4, OperatorMargin: 2,
		EpochBlocks: 8,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Bank 8 single-domain sybil bonds + the 4 anchors' own bonds into committed
	// state, and mark all as attesting validators (validatorsSeen) — the state after
	// the drain in the cloud run. Anchors are excluded from C2Metric but DO count in
	// qualifiedCount; the sybils count in both qualifiedCount and (if the fall-through
	// fires) the quorum size.
	const sybilDomain = uint64(0x5ceb11)
	for _, aid := range []ports.NodeID{idOf(a1), idOf(a2), idOf(a3), idOf(a4)} {
		c.bonded[aid] = bond
		c.validatorsSeen[aid] = true
	}
	for i := 0; i < 8; i++ {
		sid := idOf(key(int64(9400 + i)))
		c.bonded[sid] = bond
		c.bondDomain[sid] = sybilDomain
		c.validatorsSeen[sid] = true
	}

	// THE MEASUREMENT — which branch does validatorSetSize take, and what quorum?
	n := c.validatorSetSize()
	rq := c.RequiredQuorum()
	mature := c.Mature()
	handed := c.handedOff()
	qc := c.qualifiedCount()
	t.Logf("BRANCH REPORT: validatorSetSize=%d RequiredQuorum=%d | qualifiedCount=%d anchors=%d | Mature=%v handedOff=%v everMature=%v",
		n, rq, qc, len(cfg.Anchors), mature, handed, c.EverMature())

	// The PE's discriminator: pre-handoff, the launch branch must return len(Anchors).
	if n != len(cfg.Anchors) {
		t.Fatalf("BRANCH (a) — quorum-sizing bug: validatorSetSize=%d, expected len(Anchors)=%d pre-handoff. "+
			"Banked sybil bonds ARE inflating the fault budget (fall-through to qualifiedCount=%d hit). "+
			"→ the §2 fix is phase-boundary voting-set discipline (PE ruling a).", n, len(cfg.Anchors), qc)
	}
	if rq != 2 {
		t.Fatalf("BRANCH (a/b): RequiredQuorum=%d, expected bftThreshold(4)=2 over the anchor set", rq)
	}

	// Now the commit check with one anchor "down": a1 proposes, a2 + a3 attest
	// (a4 is the down validator). This is exactly 2 non-proposer attestations = the
	// required quorum. If this PASSES, the quorum sizing is CORRECT and the cloud
	// GAP is a gather-latency effect under load (branch b), not a consensus bug.
	prev, _ := c.Head()
	b1 := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, a1)
	b1.Atts = []Attestation{Attest(b1, a2), Attest(b1, a3)} // a4 "down"
	err := c.Append(*b1)
	if err != nil {
		t.Fatalf("BRANCH (b/other): a commit with proposer + 2 anchor attesters (one anchor down) was REJECTED: %v "+
			"— quorum sizing rejects a live 3-of-4 anchor set. Investigate the attestation arithmetic (PE ruling b).", err)
	}
	t.Logf("RESULT → BRANCH (b): quorum sizing is CORRECT (validatorSetSize=4, RequiredQuorum=2, a 3-of-4 anchor " +
		"commit with one down PASSES in-process). The cloud 6-fault-tolerance GAP is therefore a gather-LATENCY " +
		"effect under the 8-sybil load, NOT a quorum-sizing bug — same family as §1. No consensus rule change; " +
		"the fix is liveness/latency (and the §1 load reduction). Sybil bonds do NOT inflate the launch quorum.")
}
