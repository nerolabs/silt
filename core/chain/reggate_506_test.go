package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #506 — the reg-inclusion rate bound (the #503 Q3 R-rule) behind its version
// gate, per the research certification 2026-08-22: readiness is rule-aware
// bonded WEIGHT over the frozen epoch (never heads), lock-in needs the same >⅔
// super-quorum finality uses, enforcement starts at the NEXT epoch boundary
// (H_act, chain-derived), activation is monotonic, and the pre-latch trusted
// fleet declares the boundary as genesis config. The signal rides the BOND REG
// (hash-covered, conditionally signed like Domain, prune-surviving) — an
// attestation-borne signal is strippable by any re-serving peer because
// Block.Hash does not commit Atts.

// bondRegV is bondReg with the #506 readiness signal stamped and signed.
func bondRegV(priv ed25519.PrivateKey, size int64, prev ports.Hash, version uint8) BondReg {
	pub := priv.Public().(ed25519.PublicKey)
	r := BondReg{
		Validator: append([]byte(nil), pub...),
		Root:      ports.HashBytes(pub),
		Size:      size,
		Answer:    []byte("valid"),
		Version:   version,
	}
	r.Sig = ed25519.Sign(priv, r.signingBytes(BondRegNonce(prev)))
	return r
}

// preLatchChain builds an objective chain of prop + 3 attesters with the
// pre-latch genesis activation boundary set (no epochs — the trusted-fleet
// deployment mode). BondTTLBlocks 40 ⇒ R = max(40/4, K+2) = 10 < TTL/2 = 20.
func preLatchChain(t *testing.T, activation uint64) (*Chain, ed25519.PrivateKey, []ed25519.PrivateKey) {
	t.Helper()
	prop := key(50601)
	atts := []ed25519.PrivateKey{key(50602), key(50603), key(50604)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		BondTTLBlocks: 40, RegGateActivationHeight: activation}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondReg(prop, twoMiB, ports.Hash{}))
	for _, a := range atts {
		g.BondRegs = append(g.BondRegs, bondReg(a, twoMiB, ports.Hash{}))
	}
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, prop, atts
}

// commit appends one block with the given regs, proposed by prop and attested
// by every att (whole-weight coalition, so the weight quorum never interferes).
func commit(t *testing.T, c *Chain, prop ed25519.PrivateKey, atts []ed25519.PrivateKey, regs ...BondReg) *Block {
	t.Helper()
	b := tryCommit(t, c, prop, atts, regs...)
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit height %d: %v", b.Height, err)
	}
	return b
}

// tryCommit builds + signs the next block without appending it.
func tryCommit(t *testing.T, c *Chain, prop ed25519.PrivateKey, atts []ed25519.PrivateKey, regs ...BondReg) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{Version: 1, Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, BondRegs: regs}
	Sign(b, prop)
	for _, a := range atts {
		b.Atts = append(b.Atts, Attest(b, a))
	}
	return b
}

// TestRegGatePreLatchEnforcesRRule506: with the genesis-declared boundary, the
// R-rule governs every block of height > RegGateActivationHeight — strictly
// greater, so the boundary block itself is the last old-rules block — and each
// of the rule's three clauses refuses a storm shape the old rules accept:
// a rapid re-registration inside R, a slashed identity's reg (R∞, the #503
// Defect-A commit path), and two regs for one identity in a single block (the
// one-block storm that validation-against-parent-state alone cannot see).
// First registrations stay exempt, and a legitimate TTL/2 renewal passes.
func TestRegGatePreLatchEnforcesRRule506(t *testing.T) {
	c, prop, atts := preLatchChain(t, 2)
	v1 := atts[0]

	// Height 1 ≤ boundary: a re-reg 1 block after genesis (distance ≪ R) is
	// still OLD-rules valid — the gate must not reach before its boundary.
	commit(t, c, prop, atts, bondReg(v1, twoMiB, mustHead(c)))
	// Height 2 == boundary: still old rules (strictly greater).
	commit(t, c, prop, atts, bondReg(v1, twoMiB, mustHead(c)))

	// Height 3 > boundary: the same shape is now INVALID — v1's last committed
	// reg is 1 block old, R = 10.
	storm := tryCommit(t, c, prop, atts, bondReg(v1, twoMiB, mustHead(c)))
	if err := c.Append(*storm); !errors.Is(err, ErrRegGate) {
		t.Fatalf("#506: a re-reg %d blocks after the last must be refused past the gate (R=%d), got %v",
			1, c.regMinInterval(), err)
	}

	// A FIRST registration is exempt (bondRegHeight unset).
	joiner := key(50699)
	commit(t, c, prop, atts, bondReg(joiner, twoMiB, mustHead(c)))

	// One identity, two regs, one block: refused even though the parent state
	// alone is clean for both.
	prev := mustHead(c)
	dup := tryCommit(t, c, prop, atts, bondReg(key(50698), twoMiB, prev), bondReg(key(50698), twoMiB, prev))
	if err := c.Append(*dup); !errors.Is(err, ErrRegGate) {
		t.Fatalf("#506: two regs for one identity in one block must be refused, got %v", err)
	}

	// Advance past R without regs, then a legitimate renewal passes.
	for c.Len() < 16 {
		commit(t, c, prop, atts)
	}
	commit(t, c, prop, atts, bondReg(v1, twoMiB, mustHead(c))) // last reg h2, now ≥ R later

	// A slashed identity's reg is refused forever (R∞). Slash v1 via a
	// committed slash record, then try its reg.
	slashed := atts[1]
	commitSlash(t, c, prop, atts, slashed)
	reg := tryCommit(t, c, prop, atts, bondReg(slashed, twoMiB, mustHead(c)))
	if err := c.Append(*reg); !errors.Is(err, ErrRegGate) {
		t.Fatalf("#506: a slashed identity's reg must be refused past the gate (R∞), got %v", err)
	}
}

// mustHead returns the current head hash.
func mustHead(c *Chain) ports.Hash { h, _ := c.Head(); return h }

// commitSlash commits a block carrying a self-verifying equivocation slash for
// culprit (two conflicting signed headers at one height — the F2 evidence shape).
func commitSlash(t *testing.T, c *Chain, prop ed25519.PrivateKey, atts []ed25519.PrivateKey, culprit ed25519.PrivateKey) {
	t.Helper()
	prev, next := c.Head()
	a := &Block{Version: 1, Height: 99, Prev: prev, Entries: []ports.Entry{entry(201)}}
	b := &Block{Version: 1, Height: 99, Prev: prev, Entries: []ports.Entry{entry(202)}}
	Sign(a, culprit)
	Sign(b, culprit)
	ev := Equivocation{Culprit: append([]byte(nil), culprit.Public().(ed25519.PublicKey)...), A: *a, B: *b}
	blk := &Block{Version: 1, Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, Slashes: []Equivocation{ev}}
	Sign(blk, prop)
	for _, at := range atts {
		blk.Atts = append(blk.Atts, Attest(blk, at))
	}
	if err := c.Append(*blk); err != nil {
		t.Fatalf("commit slash: %v", err)
	}
}

// TestRegGateLockInIsWeightCountedAndBoundaryExact506 is the post-latch
// activation path end to end, pinning the three certified properties at once:
//
//  1. WEIGHT, never heads: three of four validators signalling readiness is a
//     ¾ head-majority but only 6/14 of the frozen weight — no lock-in. A
//     cheap-bond cohort must not be able to fake-signal an activation
//     (the same C1/C2 reason the commit quorum is weight-counted).
//  2. Lock-in at the boundary that first clears >⅔ weight; H_act = that
//     boundary + one epoch (a finalized epoch of notice).
//  3. Enforcement is exact: a storm reg in the H_act block itself is the last
//     old-rules acceptance; the identical shape one block later is refused.
//
// A version-less reg (a pre-gate binary's, Version absent) reads as
// not-rule-aware throughout — the safe default.
func TestRegGateLockInIsWeightCountedAndBoundaryExact506(t *testing.T) {
	whale := key(50611)
	minnows := []ed25519.PrivateKey{key(50612), key(50613), key(50614)}
	const whaleMiB = 8 << 20
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis: whale 8 MiB NOT signalling; minnows 2 MiB each SIGNALLING v3.
	// Ready weight 6/14 (43%) but ready heads ¾ (75%).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondReg(whale, whaleMiB, ports.Hash{}))
	for _, m := range minnows {
		g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionRegGate))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Two full epochs commit (boundaries at 4 and 8): heads-majority readiness
	// must NOT lock the gate in.
	for c.Len() < 9 {
		commit(t, c, whale, minnows)
	}
	if c.gateLockedIn {
		t.Fatal("#506: a ¾ HEADS majority at 43% of the weight locked the gate in — readiness must be weight-counted")
	}

	// The whale's binary upgrades: its renewal signals v3. Ready weight 14/14
	// at the next boundary (12) → lock-in there; H_act = 12 + 4 = 16.
	commit(t, c, whale, minnows, bondRegV(whale, whaleMiB, mustHead(c), BlockVersionRegGate))
	for c.Len() < 13 {
		commit(t, c, whale, minnows)
	}
	if !c.gateLockedIn {
		t.Fatal("#506: full ready weight at an epoch boundary must lock the gate in")
	}
	if c.gateHeight != 16 {
		t.Fatalf("#506: H_act must be the NEXT boundary after lock-in (want 16), got %d", c.gateHeight)
	}

	// Storm shape: a minnow re-reg 1 block after its last. At H_act (16) it is
	// the last old-rules acceptance; at 17 the identical shape is refused.
	for c.Len() < 15 {
		commit(t, c, whale, minnows)
	}
	commit(t, c, whale, minnows, bondRegV(minnows[0], twoMiB, mustHead(c), BlockVersionRegGate)) // height 15
	commit(t, c, whale, minnows, bondRegV(minnows[0], twoMiB, mustHead(c), BlockVersionRegGate)) // height 16 == H_act: old rules
	storm := tryCommit(t, c, whale, minnows, bondRegV(minnows[0], twoMiB, mustHead(c), BlockVersionRegGate))
	if err := c.Append(*storm); !errors.Is(err, ErrRegGate) {
		t.Fatalf("#506: the storm shape at H_act+1 must be refused, got %v", err)
	}

	// Monotonic: the SAME state on later boundaries never unlatches, even as
	// signals churn (the whale's next renewal reverts to a version-less binary —
	// ready weight collapses below ⅔). Un-tightening would itself be a hard fork.
	for c.Len() < 27 {
		commit(t, c, whale, minnows)
	}
	commit(t, c, whale, minnows, bondReg(whale, whaleMiB, mustHead(c))) // version-less renewal, ≥ R after its last
	for c.Len() < 33 {
		commit(t, c, whale, minnows)
	}
	if !c.gateLockedIn || c.gateHeight != 16 {
		t.Fatalf("#506: activation must be monotonic across a ready-weight collapse (lockedIn=%v H_act=%d)",
			c.gateLockedIn, c.gateHeight)
	}
	storm2 := tryCommit(t, c, whale, minnows, bondRegV(minnows[1], twoMiB, mustHead(c), BlockVersionRegGate),
		bondRegV(minnows[1], twoMiB, mustHead(c), BlockVersionRegGate))
	if err := c.Append(*storm2); !errors.Is(err, ErrRegGate) {
		t.Fatalf("#506: enforcement must survive a later ready-weight collapse, got %v", err)
	}
}

// TestRegGateSignalIsSignedAndPruneSurvives506: the readiness signal must be
// unforgeable and durable. (a) Stripping (or flipping) a signalling reg's
// Version breaks its signature — the conditional signingBytes binding, the
// Domain idiom — so a relay cannot silently un-signal a validator. (b) Prune
// keeps Version (a light field): pruned history still yields the identical
// readiness tally on replay.
func TestRegGateSignalIsSignedAndPruneSurvives506(t *testing.T) {
	c, prop, atts := preLatchChain(t, 0)
	v := key(50621)

	signed := bondRegV(v, twoMiB, mustHead(c), BlockVersionRegGate)
	stripped := signed
	stripped.Version = 0
	b := tryCommit(t, c, prop, atts, stripped)
	if err := c.Append(*b); !errors.Is(err, ErrBadBondReg) {
		t.Fatalf("#506: a version-stripped signalling reg must fail its signature check, got %v", err)
	}
	commit(t, c, prop, atts, signed) // untampered, it commits

	blk := c.Blocks(uint64(c.Len() - 1))[0]
	pruned := blk.Prune()
	if len(pruned.BondRegs) == 0 || pruned.BondRegs[0].Version != BlockVersionRegGate {
		t.Fatal("#506: Prune must keep the readiness signal (a light field, like Domain)")
	}
}

// TestRegGateReplayDerivesIdenticalActivation506: H_act is derived state — a
// fresh replica replaying the identical committed history computes the
// identical activation (certification Q2: every honest node computes H_act
// identically from the committed chain, regardless of when it upgraded).
func TestRegGateReplayDerivesIdenticalActivation506(t *testing.T) {
	whale := key(50631)
	minnows := []ed25519.PrivateKey{key(50632), key(50633), key(50634)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, BlockVersionRegGate))
	for _, m := range minnows {
		g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionRegGate))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	for c.Len() < 10 {
		commit(t, c, whale, minnows)
	}
	if !c.gateLockedIn {
		t.Fatal("rig: an all-ready genesis set must lock in at the first boundary")
	}

	replica := New(cfg, func(ports.NodeID) int64 { return 0 })
	replica.SetBondVerifier(objectiveVerify)
	blocks := c.Blocks(0)
	if err := replica.AppendGenesis(blocks[0]); err != nil {
		t.Fatalf("replica genesis: %v", err)
	}
	for _, b := range blocks[1:] {
		if err := replica.Append(b); err != nil {
			t.Fatalf("replica replay height %d: %v", b.Height, err)
		}
	}
	if replica.gateLockedIn != c.gateLockedIn || replica.gateHeight != c.gateHeight {
		t.Fatalf("#506: replayed activation diverged (live H_act=%d, replica H_act=%d) — the gate must be a pure function of committed history",
			c.gateHeight, replica.gateHeight)
	}
}
