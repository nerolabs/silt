package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #402 — the launch anchor gate is the LAUNCH FACE of the intersecting-quorum
// invariant (I1): a block may be FINALIZED only on a quorum that intersects over
// that phase's real validator set, so at most one block per height can finalize.
//
// The field run (4faaee8-22913) ran with the gate OFF (`-anchor-quorum` unset →
// default 0 → inert; see issue #402 attribution correction), so a two-sybil-
// signature quorum committed a sybil-side fork with NO anchor at all. Once the
// gate is configured, the consult's proposed `AnchorQuorum=⌈A/2⌉` (=2 for A=4) is
// STILL insufficient (research certification 2026-08-14): two sybil-proposed
// competing blocks with the four anchors split 2-2 as attesters each satisfy
// AnchorQuorum=2, so BOTH pass ValidateCommit and the finality gate then CEMENTS a
// permanent conflicting-finalization partition.
//
// Certified fix (this file is failing-first for it):
//   - Encoding (B): launch proposing is ANCHOR-ONLY (sybils drain via
//     MsgSubmitBondReg submit-don't-propose, #397) — removes the sybil-proposed
//     fork at the source.
//   - Derived STRICT ANCHOR MAJORITY `⌊A/2⌋+1` (=3 for A=4), counting the
//     proposer-if-anchor, sybils excluded — enforced in OBJECTIVE mode DERIVED from
//     len(Anchors), independent of the AnchorQuorum knob, so a missing/low config
//     can never disable intersection. Two 3-of-4 anchor sets must share ≥1 honest
//     anchor → unique launch finalization.
//
// The tests below assert the FIXED behavior. Run against pre-fix HEAD they are RED
// (the gate admits the fork / a sybil proposes); after the fix they are GREEN.

// launch402 builds a fresh objective launch network: A anchors + S bonded sybils
// (one shared domain), all banked at genesis, wheels engaged (never matures). It
// returns the chain plus the anchor and sybil keys. AnchorQuorum is set to `aq` to
// let a test model the OLD knob; the fixed objective rule ignores it (derived).
func launch402(t *testing.T, nAnchors, nSybils, aq int) (*Chain, []ed25519.PrivateKey, []ed25519.PrivateKey) {
	t.Helper()
	const bond = int64(64) << 20
	ak := make([]ed25519.PrivateKey, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9000 + i))
		anchors[idOf(ak[i])] = true
	}
	sk := make([]ed25519.PrivateKey, nSybils)
	for i := range sk {
		sk[i] = key(int64(9100 + i))
	}
	const sybilDomain = uint64(0x5ceb11)

	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: aq, MatureValidators: 99, // never matures: the gate stays engaged
		OperatorMargin: 2,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range ak {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bond, ports.Hash{}, uint64(i+1)))
	}
	for _, k := range sk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bond, ports.Hash{}, sybilDomain))
	}
	Sign(g, ak[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if c.handedOff() {
		t.Fatalf("setup: network must be young (wheels engaged), handedOff=%v", c.handedOff())
	}
	return c, ak, sk
}

// TestBothSybilProposed22SplitCannotBothFinalize402 is the CERTIFIED failing-first
// repro: the consult's `AnchorQuorum=2` admits a both-sybil-proposed 2-2 anchor
// split (each fork gets 2 anchor attesters) → two conflicting blocks both pass
// ValidateCommit → permanent partition. The certified rule refuses both (sybils
// can't propose AND neither reaches a 3-anchor majority), while the honest
// 3-of-4-anchor block still commits.
func TestBothSybilProposed22SplitCannotBothFinalize402(t *testing.T) {
	c, ak, sk := launch402(t, 4, 8, 2) // AnchorQuorum=2 — the consult's proposed (insufficient) rule
	prev, _ := c.Head()

	// The honest block: anchor a0 proposes, a1 + a2 attest → 3 anchors (proposer +
	// 2). Commits under the derived ⌊4/2⌋+1=3 rule; 1 anchor (a3) may be down.
	honest := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(honest, ak[0])
	honest.Atts = []Attestation{Attest(honest, ak[1]), Attest(honest, ak[2])}
	if err := c.ValidateCommit(honest); err != nil {
		t.Fatalf("honest 3-of-4 anchor commit must pass: %v", err)
	}

	// The 2-2 split: two SYBIL-proposed competing blocks, anchors split as
	// attesters {a0,a1}→forkA, {a2,a3}→forkB. Under AnchorQuorum=2 both satisfy the
	// old gate (2 anchor attesters each). The fix must let AT MOST ONE pass.
	forkA := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(forkA, sk[0])
	forkA.Atts = []Attestation{Attest(forkA, ak[0]), Attest(forkA, ak[1]), Attest(forkA, sk[1]), Attest(forkA, sk[2])}

	forkB := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(forkB, sk[3])
	forkB.Atts = []Attestation{Attest(forkB, ak[2]), Attest(forkB, ak[3]), Attest(forkB, sk[4]), Attest(forkB, sk[5])}

	passA := c.ValidateCommit(forkA) == nil
	passB := c.ValidateCommit(forkB) == nil
	if passA || passB {
		t.Fatalf("#402 (I1): a both-sybil-proposed 2-2 anchor split must NOT both-finalize; "+
			"forkA passed=%v forkB passed=%v — AnchorQuorum=2 is insufficient (the certified rule is ⌊A/2⌋+1)", passA, passB)
	}
}

// TestSybilCannotProposeAtLaunch402 asserts encoding (B): during the launch window
// a bonded SYBIL may not propose, even with a full anchor quorum attesting. It
// drains its bond via MsgSubmitBondReg instead (submit-don't-propose). RED pre-fix
// (sybils propose), GREEN post.
func TestSybilCannotProposeAtLaunch402(t *testing.T) {
	c, ak, sk := launch402(t, 4, 8, 2)
	prev, _ := c.Head()

	b := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b, sk[0]) // a sybil proposes
	// Even a full anchor majority attesting can't launder a sybil-proposed block.
	b.Atts = []Attestation{Attest(b, ak[0]), Attest(b, ak[1]), Attest(b, ak[2])}
	if err := c.ValidateProposal(b); !errors.Is(err, ErrLowReputation) {
		t.Fatalf("#402 encoding-B: a sybil-proposed launch block must be refused at ValidateProposal (anchor-only proposing); got %v", err)
	}
	if err := c.ValidateCommit(b); err == nil {
		t.Fatal("#402 encoding-B: a sybil-proposed launch block must not commit")
	}
}

// TestDerivedAnchorMajorityIgnoresConfig402 is the STRUCTURAL guarantee that closes
// the field footgun: in objective launch mode the strict anchor majority is DERIVED
// from len(Anchors), so even AnchorQuorum=0 (exactly the field default that left the
// gate inert) still refuses a sub-majority fork. RED pre-fix (AnchorQuorum=0 → gate
// off → the fork commits), GREEN post.
func TestDerivedAnchorMajorityIgnoresConfig402(t *testing.T) {
	c, ak, sk := launch402(t, 4, 8, 0) // AnchorQuorum=0 — the field footgun
	prev, _ := c.Head()

	// A fork with only ONE anchor (a3) + sybils — below the derived ⌊4/2⌋+1=3
	// majority. Anchor-proposed so encoding-B doesn't short-circuit it: the point
	// here is the DERIVED count, independent of the knob.
	fork := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(fork, ak[3]) // anchor a3 proposes (1 anchor)
	for _, k := range sk[:4] {
		fork.Atts = append(fork.Atts, Attest(fork, k))
	}
	if err := c.ValidateCommit(fork); !errors.Is(err, ErrAnchorRequired) {
		t.Fatalf("#402 structural: with AnchorQuorum=0 the DERIVED ⌊A/2⌋+1 majority must still refuse a 1-anchor fork; got %v", err)
	}
}

// TestHonestThreeOfFourCommitsAndOneDownTolerated402 pins the liveness cost the
// certification accepts: 3-of-4 anchors up commits (1-fault-tolerant); 2-of-4 does
// not. Same before and after the fix for the passing leg — the failing leg is RED
// pre-fix only if the old knob was < 3 (documented in-line).
func TestHonestThreeOfFourCommitsAndOneDownTolerated402(t *testing.T) {
	c, ak, sk := launch402(t, 4, 8, 0)
	prev, _ := c.Head()

	// 3 anchors (a0 proposes, a1+a2 attest) → commits.
	up := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(up, ak[0])
	up.Atts = []Attestation{Attest(up, ak[1]), Attest(up, ak[2]), Attest(up, sk[0])}
	if err := c.ValidateCommit(up); err != nil {
		t.Fatalf("3-of-4 anchors up must commit: %v", err)
	}

	// 2 anchors (a0 proposes, a1 attests; a2,a3 down) → refused (below majority 3).
	down := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(down, ak[0])
	down.Atts = []Attestation{Attest(down, ak[1]), Attest(down, sk[0]), Attest(down, sk[1])}
	if err := c.ValidateCommit(down); !errors.Is(err, ErrAnchorRequired) {
		t.Fatalf("2-of-4 anchors must be refused (strict majority is 3): got %v", err)
	}
}

// TestSupportMeetsQuorumRequiresAnchorMajority402 pins that the proposer's gather
// stop-predicate (SupportMeetsQuorum) demands the SAME strict anchor majority as
// ValidateCommit — so a proposer gathers enough ANCHORS, not just enough heads, and
// never commit-attempts a count-quorum its own Append would reject. RED against a
// pre-fix SupportMeetsQuorum (count-only: a 2-head sybil-heavy coalition returns true).
func TestSupportMeetsQuorumRequiresAnchorMajority402(t *testing.T) {
	c, ak, sk := launch402(t, 4, 8, 0)
	aid := make([]ports.NodeID, len(ak))
	for i := range ak {
		aid[i] = idOf(ak[i])
	}
	sid := make([]ports.NodeID, len(sk))
	for i := range sk {
		sid[i] = idOf(sk[i])
	}

	// Count quorum met (2 attesters) but sub-anchor-majority: a0 + 2 sybils = 1 anchor.
	if c.SupportMeetsQuorum(aid[0], []ports.NodeID{sid[0], sid[1]}) {
		t.Fatal("gather must NOT stop on a count-quorum with only 1 anchor (proposer) — Append would reject it")
	}
	// a0 + (a1, sybil) = 2 anchors < 3 → still short.
	if c.SupportMeetsQuorum(aid[0], []ports.NodeID{aid[1], sid[0]}) {
		t.Fatal("gather must NOT stop at 2 anchors (strict majority is 3)")
	}
	// a0 + (a1, a2) = 3 anchors ≥ 3 → this coalition would commit.
	if !c.SupportMeetsQuorum(aid[0], []ports.NodeID{aid[1], aid[2]}) {
		t.Fatal("gather MUST stop once the coalition carries the strict anchor majority (3)")
	}
}
