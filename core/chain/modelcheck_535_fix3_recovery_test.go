package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — #535 fix (3): the operator-directed weak-subjectivity
// liveness-floor escape (design: docs/thinking/2026-08-24-535-fix3-design.md;
// certification: silt-reviews/research/research-outcome/
// 535-epoch-boundary-liveness-cliff-RESEARCH-CERTIFICATION-2026-08-23.md).
//
// The certified recovery stack for the #535 boundary wedge is (4) + (3).
// Fix (4) (R-gate restore exemption, shipped) heals a RETURNING member. Fix (3)
// is the recovery for a genuine > ⅓-of-frozen-weight loss that does NOT return:
// Config.LivenessRecoveryHeight, an operator-coordinated epoch-boundary height
// at which the mature-epoch validation re-bases proposer/attester/quorum
// against the LIVE qualified bonded set instead of the frozen epochSet — one
// boundary only, off by default, never automatic. The trust anchor is the
// HUMAN who confirms the loss is a real outage (the WSCheckpoint trust class);
// automatic re-basing was REFUTED (fix (2),
// modelcheck_535_fix2_rebasing_test.go — excluding possibly-honest lapsed
// weight raises the Byzantine fraction and reopens I1).
//
// This test asserts the design's provable properties:
//  1. RECOVERY WORKS: with LivenessRecoveryHeight = H set, a boundary wedged
//     under the frozen set (> ⅓ lapsed) COMMITS against the live set — filled
//     from the SAME live set it is sized over (a non-frozen live member's
//     attestation counts; the #402 size-set == membership-set law) — and
//     rotates, so the chain resumes.
//  2. SAFE DEFAULT: with LivenessRecoveryHeight = 0, or set to a DIFFERENT
//     boundary, the bled boundary STALLS exactly as the shipped no-re-basing
//     rule demands (the modelcheck_535_boundary_wedge stall, unchanged).
//     A non-boundary LivenessRecoveryHeight never fires at all.
//  3. DETERMINISM: the re-based set is a pure function of committed state +
//     genesis config — a replica replaying the same blocks with the same
//     directive computes the identical head.
//  4. NOT PROVABLE (the accepted residual, by design): the operator's
//     judgment. If recovery is invoked when the "loss" is actually a Byzantine
//     partition, the re-base can fork — exactly the fix (2) counterexample.
//     That residual is the documented weak-subjectivity trust, identical to
//     accepting a bad WSCheckpoint; no test here claims otherwise.
//
// Topology (the wedge test's, plus one fresh joiner): 4 anchors + 4 maturers
// at 64M, 4 sybils at 1M — all 12 frozen at the genesis boundary (516 MiB).
// A FRESH validator bonds 256M mid-epoch (h1), so it is live-qualified but NOT
// in the frozen set. Then 3 maturers LAPSE (bond TTL-expired while offline —
// the fix (4) test's surgery): frozen live-and-willing weight is 324 of 516,
// below the 344 (> ⅔) bar — the boundary at h2 wedges. The live qualified set
// at h2 is 580 MiB (256 anchors + 64 maturer + 4 sybils + 256 fresh), so the
// re-based bar is 386.67: the frozen survivors alone (324) do NOT clear it —
// the fresh joiner's attestation is load-bearing, which is what pins predicate
// threading (attester qualification at H must consult the SAME set the
// denominator sums, or the quorum is sized over one set and filled from
// another — the #402 trap).
func setup535Fix3(t *testing.T, recoveryHeight uint64) (c *Chain, ak, mk, yk []ed25519.PrivateKey, fresh ed25519.PrivateKey) {
	t.Helper()
	const bigBond = int64(64) << 20
	const sybilBond = int64(1) << 20
	const freshBond = int64(256) << 20

	ak = make([]ed25519.PrivateKey, 4) // anchors 64M
	mk = make([]ed25519.PrivateKey, 4) // maturers 64M
	yk = make([]ed25519.PrivateKey, 4) // sybils 1M
	for i := range ak {
		ak[i] = key(int64(54000 + i))
		mk[i] = key(int64(54100 + i))
		yk[i] = key(int64(54200 + i))
	}
	fresh = key(54300)

	cfg := Config{Quorum: 2, MinBond: sybilBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 2, LivenessRecoveryHeight: recoveryHeight}
	c = New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis seats all 12 in the frozen epoch (MatureValidators=0 hands off at
	// genesis; boundary h0). The fresh joiner is NOT here — it bonds at h1.
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	dom := uint64(1)
	for _, k := range ak {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bigBond, ports.Hash{}, dom))
		dom++
	}
	for _, k := range mk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bigBond, ports.Hash{}, dom))
		dom++
	}
	for _, k := range yk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, sybilBond, ports.Hash{}, 0))
	}
	Sign(g, ak[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if n := c.validatorSetSize(); n != 12 {
		t.Fatalf("setup: frozen epoch must seat all 12, got %d", n)
	}

	// h1 (mid-epoch): everyone is still up; the block carries the FRESH
	// validator's 256M bond. The bond banks at commit; frozen membership is
	// untouched until a rotation (I3).
	prev, _ := c.Head()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondRegDom(fresh, freshBond, prev, 9)}}
	Sign(b1, ak[0])
	b1.Atts = append(b1.Atts,
		Attest(b1, ak[1]), Attest(b1, ak[2]), Attest(b1, ak[3]),
		Attest(b1, mk[0]), Attest(b1, mk[1]), Attest(b1, mk[2]), Attest(b1, mk[3]))
	if err := c.Append(*b1); err != nil {
		t.Fatalf("h1: %v", err)
	}
	if c.bonded[idOf(fresh)] != freshBond {
		t.Fatal("setup: fresh joiner's bond must be committed at h1")
	}
	if _, frozen := c.epochSet[idOf(fresh)]; frozen {
		t.Fatal("setup: fresh joiner must NOT be in the frozen epoch set (I3)")
	}

	// THE LAPSE (the #535 mechanism, the fix (4) test's surgery): maturers 1-3
	// go offline and their bonds TTL-expire — gone from the live bonded set,
	// but their 192 MiB stays in the FROZEN denominator for the whole epoch.
	for _, k := range mk[1:] {
		if c.bonded[idOf(k)] < c.cfg.MinBond {
			t.Fatal("setup: maturer should be bonded before we model its lapse")
		}
		delete(c.bonded, idOf(k))
	}
	return c, ak, mk, yk, fresh
}

// boundary535Fix3 builds the h2 boundary block attested by the full live
// coalition: anchors 1-3, the surviving maturer, all 4 sybils — and, when
// withFresh, the non-frozen fresh joiner.
func boundary535Fix3(c *Chain, ak, mk, yk []ed25519.PrivateKey, fresh ed25519.PrivateKey, withFresh bool, tag byte) *Block {
	prev, _ := c.Head()
	b := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(100 + tag)}}
	Sign(b, ak[0])
	b.Atts = append(b.Atts,
		Attest(b, ak[1]), Attest(b, ak[2]), Attest(b, ak[3]),
		Attest(b, mk[0]))
	for _, k := range yk {
		b.Atts = append(b.Atts, Attest(b, k))
	}
	if withFresh {
		b.Atts = append(b.Atts, Attest(b, fresh))
	}
	return b
}

func coalition535Fix3(ak, mk, yk []ed25519.PrivateKey, fresh ed25519.PrivateKey, withFresh bool) []ports.NodeID {
	ids := []ports.NodeID{idOf(ak[1]), idOf(ak[2]), idOf(ak[3]), idOf(mk[0]),
		idOf(yk[0]), idOf(yk[1]), idOf(yk[2]), idOf(yk[3])}
	if withFresh {
		ids = append(ids, idOf(fresh))
	}
	return ids
}

// Property 2 — SAFE DEFAULT: with no recovery directive (and with a directive
// naming a DIFFERENT boundary) the bled boundary stalls, exactly the certified
// no-re-basing rule. The escape is NEVER automatic.
func TestModelCheck_535_Fix3_SafeDefaultStalls(t *testing.T) {
	// LivenessRecoveryHeight = 0 (off, the default).
	c, ak, mk, yk, fresh := setup535Fix3(t, 0)
	b := boundary535Fix3(c, ak, mk, yk, fresh, true, 0)
	if err := c.Append(*b); err == nil {
		t.Fatal("#535 fix (3) SAFE DEFAULT BROKEN: the bled boundary committed with LivenessRecoveryHeight unset — the re-base must NEVER be automatic")
	} else if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("the stall must be the frozen-weight rule, got: %v", err)
	}

	// A directive for a DIFFERENT boundary (h4) leaves h2 stalled.
	c2, ak2, mk2, yk2, fresh2 := setup535Fix3(t, 4)
	b2 := boundary535Fix3(c2, ak2, mk2, yk2, fresh2, true, 1)
	if err := c2.Append(*b2); err == nil {
		t.Fatal("#535 fix (3): a recovery directive for a DIFFERENT boundary must not re-base this one")
	} else if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("the stall must be the frozen-weight rule, got: %v", err)
	}

	// A NON-boundary directive never fires: the effective set at a mid-epoch
	// height equals the frozen snapshot even if the height matches the
	// directive (the re-base is defined at an epoch boundary ONLY — a mid-epoch
	// set change is exactly the churning-set unsoundness I3 forbids).
	c2.cfg.LivenessRecoveryHeight = 3 // 3 % EpochBlocks(2) != 0
	if len(c2.effectiveEpochSet(3)) != len(c2.epochSet) {
		t.Fatal("#535 fix (3): a non-boundary LivenessRecoveryHeight must never re-base (I3)")
	}
}

// Properties 1 + 3 — RECOVERY WORKS, deterministically: with the directive at
// the wedged boundary, the live coalition commits against the live set (sized
// over AND filled from the same set — the fresh joiner is load-bearing), the
// epoch rotates to the lighter set, the chain resumes, and a replica replaying
// the same blocks with the same directive lands on the identical head.
func TestModelCheck_535_Fix3_RecoveryAtDirectedBoundary(t *testing.T) {
	c, ak, mk, yk, fresh := setup535Fix3(t, 2)

	// The frozen survivors ALONE (324 of live 580) do not clear the re-based
	// > ⅔ bar (386.67) — recovery is still a real super-quorum over the live
	// set, not a rubber stamp.
	bNoFresh := boundary535Fix3(c, ak, mk, yk, fresh, false, 2)
	if err := c.Append(*bNoFresh); err == nil {
		t.Fatal("#535 fix (3): the re-based boundary must still demand >⅔ of the LIVE set — 324 of 580 must not commit")
	} else if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("the refusal must be the weight rule, got: %v", err)
	}

	// With the fresh joiner (580 of 580) the boundary COMMITS: a live-qualified
	// member outside the stale frozen set may attest the recovery block (else
	// the quorum is sized over the live set but fillable only from the frozen
	// one — the #402 trap inverted).
	b := boundary535Fix3(c, ak, mk, yk, fresh, true, 3)
	// The proposer's gather predicate must agree with validation (no drift).
	if !c.SupportMeetsQuorum(idOf(ak[0]), coalition535Fix3(ak, mk, yk, fresh, true), 2) {
		t.Fatal("#535 fix (3): SupportMeetsQuorum must accept the live coalition at the recovery boundary — the gather would never assemble what ValidateCommit accepts")
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("#535 fix (3) RECOVERY FAILED: the live coalition must commit the directed boundary, got: %v", err)
	}

	// The rotation the wedge denied has now happened: the next epoch is frozen
	// from the LIVE qualified set (lapsed maturers out, fresh joiner in).
	if _, ok := c.epochSet[idOf(mk[1])]; ok {
		t.Fatal("#535 fix (3): a lapsed maturer must be out of the post-recovery epoch set")
	}
	if _, ok := c.epochSet[idOf(fresh)]; !ok {
		t.Fatal("#535 fix (3): the fresh joiner must be seated in the post-recovery epoch set")
	}
	if n := c.validatorSetSize(); n != 10 {
		t.Fatalf("#535 fix (3): post-recovery epoch must seat the 10 live members, got %d", n)
	}

	// The chain RESUMES on the lighter set — an ordinary mid-epoch block
	// commits under the normal frozen rule (the recovery is a one-boundary
	// event; h3 is business as usual).
	prev, _ := c.Head()
	b3 := &Block{Version: 1, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(b3, ak[0])
	b3.Atts = append(b3.Atts,
		Attest(b3, ak[1]), Attest(b3, ak[2]), Attest(b3, ak[3]), Attest(b3, fresh))
	if err := c.Append(*b3); err != nil {
		t.Fatalf("#535 fix (3): the chain must resume after recovery, h3 got: %v", err)
	}

	// Property 3 — DETERMINISM: a replica with the same genesis config and the
	// same directive replays the identical blocks to the identical head. (The
	// re-based set is a pure function of committed state; the SOCIAL half —
	// every honest operator setting the SAME height — is the operator's
	// responsibility, the WSCheckpoint trust model, and cannot be proven here.)
	r, _, _, _, _ := setup535Fix3(t, 2)
	for _, blk := range []*Block{b, b3} {
		if err := r.Append(*blk); err != nil {
			t.Fatalf("replica replay: %v", err)
		}
	}
	ch, hh := c.Head()
	rh, rhh := r.Head()
	if ch != rh || hh != rhh {
		t.Fatalf("#535 fix (3) DETERMINISM BROKEN: replicas diverge (head %s@%d vs %s@%d)", ch, hh, rh, rhh)
	}
}
