package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// B2 (research certification 2026-08-13): the mature-phase quorum must be
// counted in FROZEN EPOCH WEIGHT, never heads. Epoch admission is deliberately
// unfiltered (every qualified bond becomes a member at rotation — Condition A),
// so under head counting a cohort of MinBond identities riding an honest
// handoff acquired, per head, the same quorum power as a real validator:
//   - 8 cheap members among 4 honest pushed bftThreshold(12)=8 past what the
//     honest side could ever attest — the mature phase born unable to commit
//     (STALL at 8×MinBond, nothing slashable), and
//   - 9 cheap members among 4 honest made bftThreshold(13)=8 reachable by the
//     cohort alone — a commit with ZERO honest attestation (CAPTURE at
//     9×MinBond), persisting into full maturity.
// Both are C1-discounts (control priced per-head at MinBond, not per real
// weight) and C2 quiet-capture breaks. The weight rule prices them correctly:
// stall needs >⅓, capture >⅔, of the epoch's REAL bonded weight.
//
// These drills build the exact topology through the REAL path — genesis commits
// every bond, maturity latches, and rotateEpoch's actual unfiltered admission
// seats the cohort — then attempt the capture and the stall.

func TestMatureQuorumCheapMemberCaptureRefused(t *testing.T) {
	const honestBond = int64(64) << 20
	const minBond = int64(1) << 20
	const sybilDomain = uint64(0x5ceb11)

	h := make([]ports.NodeID, 4)
	hk := make([]ed25519.PrivateKey, 4)
	for i := range hk {
		hk[i] = key(int64(9500 + i))
		h[i] = idOf(hk[i])
	}
	sk := make([]ed25519.PrivateKey, 9)
	s := make([]ports.NodeID, 9)
	for i := range sk {
		sk[i] = key(int64(9600 + i))
		s[i] = idOf(sk[i])
	}

	cfg := Config{Quorum: 2, MinBond: minBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis commits every bond. MatureValidators=0 makes the genesis boundary
	// the handoff (the maturity BAR is not this drill's subject — the counting
	// basis after handoff is; it also keeps matureNow() true so the F-1
	// de-maturation gate stays dormant and cannot mask the weight rule). The
	// epoch snapshot seats all 13 — honest whales and MinBond cohort alike
	// (Condition A admission is unfiltered; that is the drill's honest premise).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range hk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, honestBond, ports.Hash{}, uint64(i+1)))
	}
	for _, k := range sk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, minBond, ports.Hash{}, sybilDomain))
	}
	Sign(g, hk[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if !c.EverMature() || !c.handedOff() {
		t.Fatalf("setup: the network must hand off at genesis (everMature=%v handedOff=%v)", c.EverMature(), c.handedOff())
	}
	if n := c.validatorSetSize(); n != 13 {
		t.Fatalf("setup: epoch set must seat all 13 members, got %d", n)
	}
	if !c.attesterQualified(s[0]) {
		t.Fatal("setup: a MinBond epoch member must be attester-qualified (unfiltered admission — the premise)")
	}

	// THE CAPTURE DRILL: the 9-member cohort proposes and attests ALONE — 8
	// distinct non-proposer attestations, zero honest signatures. Under head
	// counting this was bftThreshold(13)=8 → a valid commit (the certified
	// break). Under weight counting the coalition holds 9 MiB of 265 MiB and
	// must be refused for lack of the frozen-weight super-majority.
	prev, _ := c.Head()
	capture := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(capture, sk[0])
	for _, k := range sk[1:] {
		capture.Atts = append(capture.Atts, Attest(capture, k))
	}
	err := c.Append(*capture)
	if err == nil {
		t.Fatal("CAPTURE: a MinBond-per-head cohort committed with zero honest attestation — the head-counted quorum break (B2)")
	}
	if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("capture refusal should be the weight rule, got: %v", err)
	}
	if c.SupportMeetsQuorum(s[0], s[1:]) {
		t.Fatal("SupportMeetsQuorum must agree with ValidateCommit: the cohort coalition is insufficient")
	}

	// Liveness control: honest weight commits — h0 proposes, h1+h2 attest
	// (192 MiB of 265 MiB ≈ 72% > ⅔), count floor 2 met.
	ok := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(ok, hk[0])
	ok.Atts = []Attestation{Attest(ok, hk[1]), Attest(ok, hk[2])}
	if err := c.Append(*ok); err != nil {
		t.Fatalf("an honest >⅔-weight coalition must commit: %v", err)
	}
	if !c.SupportMeetsQuorum(h[0], []ports.NodeID{h[1], h[2]}) {
		t.Fatal("SupportMeetsQuorum must agree with ValidateCommit for the honest coalition")
	}
}

func TestMatureQuorumHonestWeightCommitsDespiteCheapHeadcount(t *testing.T) {
	const honestBond = int64(64) << 20
	const minBond = int64(1) << 20
	const sybilDomain = uint64(0x5ceb11)

	hk := make([]ed25519.PrivateKey, 4)
	for i := range hk {
		hk[i] = key(int64(9700 + i))
	}
	sk := make([]ed25519.PrivateKey, 8)
	for i := range sk {
		sk[i] = key(int64(9800 + i))
	}

	cfg := Config{Quorum: 2, MinBond: minBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range hk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, honestBond, ports.Hash{}, uint64(i+1)))
	}
	for _, k := range sk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, minBond, ports.Hash{}, sybilDomain))
	}
	Sign(g, hk[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if n := c.validatorSetSize(); n != 12 {
		t.Fatalf("setup: epoch set must seat all 12 members, got %d", n)
	}

	// THE STALL DRILL: the cohort simply DECLINES to attest — nothing
	// slashable. The honest side musters proposer + 3 attesters. Under head
	// counting that is 3 < bftThreshold(12)=8: the mature phase is born unable
	// to commit, a liveness denial priced at 8×MinBond. Under weight counting
	// the honest coalition carries 256 MiB of 264 MiB (~97%) and commits.
	prev, _ := c.Head()
	b := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b, hk[0])
	b.Atts = []Attestation{Attest(b, hk[1]), Attest(b, hk[2]), Attest(b, hk[3])}
	if err := c.Append(*b); err != nil {
		t.Fatalf("STALL: an honest >⅔-weight coalition was refused because cheap heads inflated the count threshold (B2): %v", err)
	}
}

// The super-majority is STRICT: exactly ⅔ of the frozen weight does not commit
// (two exactly-⅔ coalitions need not intersect in any weight); one member past
// it does. Three equal bonds make the boundary exact: proposer + 1 attester =
// 2/3 refused, proposer + 2 = 3/3 commits.
func TestMatureQuorumWeightBoundaryStrict(t *testing.T) {
	const bond = int64(3) << 20
	k1, k2, k3 := key(9900), key(9901), key(9902)

	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondRegDom(k1, bond, ports.Hash{}, 1),
		bondRegDom(k2, bond, ports.Hash{}, 2),
		bondRegDom(k3, bond, ports.Hash{}, 3))
	Sign(g, k1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	prev, _ := c.Head()
	at := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(at, k1)
	at.Atts = []Attestation{Attest(at, k2)} // exactly ⅔ of the weight
	if err := c.Append(*at); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("exactly ⅔ of the frozen weight must NOT commit (strict super-majority), got: %v", err)
	}
	over := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(over, k1)
	over.Atts = []Attestation{Attest(over, k2), Attest(over, k3)}
	if err := c.Append(*over); err != nil {
		t.Fatalf("a strict >⅔ coalition must commit: %v", err)
	}
}
