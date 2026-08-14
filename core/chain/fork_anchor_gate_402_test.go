package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #402 Q-A — the anchor gate is satisfied by ANY ONE qualified anchor's
// signature, and the honest side commits with a bare count quorum that leaves
// a "free" anchor unspent at each height. So a Sybil-side fork that collects
// that one free anchor's attestation PASSES the wheels-engaged commit gate —
// on the Sybil replicas — producing the two competing block-11s the field run
// (4faaee8-22913) showed: anchors on one fork, Sybils + one free anchor on the
// other.
//
// This is the CHAIN-LEVEL isolation of the field question "whose signatures
// land on the Sybil fork". It builds the launch (wheels-engaged) state — 4
// anchors + a bonded Sybil cohort, AnchorQuorum 1, quorum 2 — commits an honest
// block with anchors {a0,a1,a2} (a3 free), then builds a COMPETING block at the
// same height whose only anchor signature is a3's, plus Sybil attestations, and
// asks ValidateCommit whether the gate lets it through.
//
// A PASS here means: the anchor gate at AnchorQuorum=1 does NOT prevent a fork —
// it only prevents a fork with ZERO anchor sign-off. One non-participating
// anchor is enough to bless a Sybil-extended fork. That is not a C2 CAPTURE
// (the fork still needs a real anchor; a pure Sybil quorum is still refused),
// but it is a launch-phase FORK-CREATION / liveness vector, and it is the
// mechanism behind the field "CAPTURE" false-positive. The fix is
// research-gated (it touches the anchor-gate rule + interacts with the #397
// watermark), so this test asserts the mechanism as it stands today.
func TestForkPassesAnchorGateWithOneFreeAnchor402(t *testing.T) {
	const bond = int64(64) << 20
	ak := make([]ed25519.PrivateKey, 4)
	aid := make([]ports.NodeID, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9000 + i))
		aid[i] = idOf(ak[i])
		anchors[aid[i]] = true
	}
	sk := make([]ed25519.PrivateKey, 8)
	sid := make([]ports.NodeID, 8)
	for i := range sk {
		sk[i] = key(int64(9100 + i))
		sid[i] = idOf(sk[i])
	}
	const sybilDomain = uint64(0x5ceb11)

	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99, // never matures: the anchor gate stays engaged
		OperatorMargin: 2,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis banks every bond (anchors 64 MiB each; Sybils 64 MiB each, one
	// shared domain). Genesis is anchor-proposed and self-attested minimally.
	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
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
	if !c.attesterQualified(sid[0]) || !c.attesterQualified(aid[3]) {
		t.Fatal("setup: Sybils and the free anchor must all be attester-qualified")
	}

	prev, _ := c.Head() // height 0 head; the contested height is 1

	// HONEST commit at height 1: anchors a0(proposer) + a1 + a2 attest. a3 is
	// left UNSPENT — the bare count quorum (2) plus the free anchor is exactly
	// the field shape (3-of-4). This is the block the anchors' chain adopts.
	honest := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(honest, ak[0])
	honest.Atts = []Attestation{Attest(honest, ak[1]), Attest(honest, ak[2])}
	if err := c.ValidateCommit(honest); err != nil {
		t.Fatalf("honest 3-of-4 anchor commit must pass the gate: %v", err)
	}

	// COMPETING fork at the SAME height 1, proposed by a Sybil, attested by the
	// other Sybils AND the one free anchor a3. Distinct entry ⇒ distinct hash ⇒
	// a genuine fork, not a duplicate.
	fork := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(fork, sk[0])
	for _, k := range sk[1:] {
		fork.Atts = append(fork.Atts, Attest(fork, k))
	}
	fork.Atts = append(fork.Atts, Attest(fork, ak[3])) // the free anchor blesses the fork

	err := c.ValidateCommit(fork)
	if err != nil {
		t.Fatalf("#402 Q-A: the Sybil fork carrying ONE free-anchor attestation was REFUSED (%v) — "+
			"if this is now the intended behavior the anchor-gate rule changed; today's rule (AnchorQuorum=1) "+
			"is expected to ACCEPT it, which is the fork-creation vector the field run hit", err)
	}

	// Confirm the boundary: WITHOUT the free anchor, the same Sybil quorum is
	// refused — so the property that holds is "no anchor ⇒ no commit", NOT "no
	// fork". This is the exact C2 line: a pure Sybil quorum cannot capture; one
	// free anchor can fork.
	noAnchor := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(noAnchor, sk[0])
	for _, k := range sk[1:] {
		noAnchor.Atts = append(noAnchor.Atts, Attest(noAnchor, k))
	}
	if err := c.ValidateCommit(noAnchor); err == nil {
		t.Fatal("#402: a pure Sybil quorum with NO anchor attestation must still be refused (ErrAnchorRequired) — that is C2")
	}

	t.Logf("#402 Q-A CONFIRMED: AnchorQuorum=1 accepts a Sybil-extended fork blessed by one free anchor, " +
		"while refusing a zero-anchor Sybil quorum. The field 'CAPTURE' was a fork (needs an anchor), not a capture (no anchor).")
}

// #402 Q-A fix direction (research-rulable): the fork is possible ONLY because
// the honest commit at count-quorum leaves a FREE anchor, and AnchorQuorum=1
// lets that lone free anchor bless the competitor. Raise the anchor gate so two
// disjoint blocks at a height CANNOT both reach it: each anchor signs at most
// one block per height (#397 watermark), so with A anchors the honest block's
// proposer+attesters consume some, and the fork can gather at most the rest.
// With 4 anchors, AnchorQuorum=2 forces the honest side to hold 2 NON-PROPOSER
// anchor attesters (proposer + 2 = 3 anchors up, tolerating 1 down) and leaves
// at most 1 free — below the fork's required 2. This test asserts that fix
// closes the exact fork above, and names its cost (launch commits now need
// 3-of-4 anchors, not 2-of-4). Whether to pay that cost, or scale AnchorQuorum
// as ⌈(A+1)/2⌉ generally, is the consensus-rule question routed to research.
func TestIntersectingAnchorQuorumClosesTheFork402(t *testing.T) {
	const bond = int64(64) << 20
	ak := make([]ed25519.PrivateKey, 4)
	aid := make([]ports.NodeID, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9000 + i))
		aid[i] = idOf(ak[i])
		anchors[aid[i]] = true
	}
	sk := make([]ed25519.PrivateKey, 8)
	for i := range sk {
		sk[i] = key(int64(9100 + i))
	}
	const sybilDomain = uint64(0x5ceb11)

	// The ONLY change from the vector above: AnchorQuorum 2 instead of 1.
	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 2, MatureValidators: 99,
		OperatorMargin: 2,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
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
	prev, _ := c.Head()

	// Honest block now needs 2 non-proposer anchor attesters (a1, a2) — proposer
	// a0 makes 3 anchors, a3 free. Still commits (1 anchor may be down).
	honest := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(honest, ak[0])
	honest.Atts = []Attestation{Attest(honest, ak[1]), Attest(honest, ak[2])}
	if err := c.ValidateCommit(honest); err != nil {
		t.Fatalf("honest 3-of-4 anchor commit must still pass at AnchorQuorum=2: %v", err)
	}

	// The SAME fork (Sybils + the one free anchor a3) now has only 1 anchor
	// attester < AnchorQuorum 2 → REFUSED. No free anchor can bless a fork,
	// because the honest commit consumed all but one.
	fork := &Block{Version: BlockVersion, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(fork, sk[0])
	for _, k := range sk[1:] {
		fork.Atts = append(fork.Atts, Attest(fork, k))
	}
	fork.Atts = append(fork.Atts, Attest(fork, ak[3]))
	if err := c.ValidateCommit(fork); err == nil {
		t.Fatal("#402 fix: at AnchorQuorum=2 a one-free-anchor fork must be REFUSED (the honest commit leaves <2 free anchors)")
	}
	t.Logf("#402 fix direction CONFIRMED: AnchorQuorum=2 closes the one-free-anchor fork while keeping " +
		"honest liveness at 3-of-4 anchors (1-fault-tolerant). Cost + the general ⌈(A+1)/2⌉ rule = the research question.")
}
