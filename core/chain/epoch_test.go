package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #357 research-certification Conditions A + B — the mature-phase epoch machinery.
//
// The §3 finality gate makes a super-quorum-committed block irreversible, but
// finality is quorum-INTERSECTION safety: two super-quorums are only guaranteed to
// share an honest validator when both are taken over the SAME set N. Before this
// change the mature phase recomputed N (validatorSetSize → qualifiedCount) and
// attester qualification (bonded[id] ≥ MinBond) LIVE from the churning bonded map
// (joins, renewals, TTL expiry), so two conflicting commits could each gather a
// "super-quorum" of two DIFFERENT sets — conflicting finalization, the exact D-1
// violation — and RequiredQuorum drifting mid-formation recreates the §2 stall one
// regime up. Condition A freezes the mature validator set per epoch, rotating only
// at an epoch-boundary block (itself super-quorum-final under §3); Condition B
// makes the young→mature handoff the FIRST such rotation, so the weight-meaning
// change is rooted at a finalized base and can never reach back across the boundary.
//
// These are the failing-first regressions: each asserts the frozen-set semantics
// that fail on the live-recompute code and pass with the epoch snapshot.

func TestEpochQuorumFrozenAcrossMidEpochJoin(t *testing.T) {
	prop, v1, v2, v3 := key(21), key(22), key(23), key(24)
	joiner := key(25)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(v1, twoMiB, ports.Hash{}),
		bondReg(v2, twoMiB, ports.Hash{}),
		bondReg(v3, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}

	// The genesis boundary (height 0) snapshots the founding set of 4. The
	// Byzantine bar in a mature epoch is the >⅔ frozen-WEIGHT super-majority
	// (B2, research certification 2026-08-13) — RequiredQuorum returns only the
	// Config.Quorum count floor, and the weight rule carries the escalation.
	if n := c.validatorSetSize(); n != 4 {
		t.Fatalf("epoch snapshot at genesis: validatorSetSize = %d, want 4", n)
	}
	if rq := c.RequiredQuorum(); rq != 1 {
		t.Fatalf("RequiredQuorum in a mature epoch is the count floor: got %d, want Quorum=1 (the Byzantine bar is weight-counted)", rq)
	}

	// Block 1 (mid-epoch): a NEW validator registers a bond. Condition A: the join
	// must NOT move N, the quorum, or qualification until the next epoch boundary —
	// a live recompute here is exactly the churning-set finalization unsoundness.
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(joiner, twoMiB, prev)}}
	Sign(b1, prop)
	b1.Atts = []Attestation{Attest(b1, v1), Attest(b1, v2)}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit block 1 (mid-epoch join): %v", err)
	}
	if n := c.validatorSetSize(); n != 4 {
		t.Fatalf("Condition A: mid-epoch join must not resize the finality set: validatorSetSize = %d, want 4 (frozen)", n)
	}
	// Condition A in WEIGHT terms: the join must not move the quorum DENOMINATOR
	// mid-epoch. Over the frozen 4×2 MiB set, proposer + 1 attester is 4/8 = ½
	// — refused; the committed b1 above (proposer + 2 = ¾) is what clears >⅔.
	// (Under the joiner's live weight the ½ coalition would be 4/10 — the
	// refusal must come from the FROZEN denominator, asserted after rotation.)
	under := &Block{Version: 1, Height: 2, Prev: b1.Hash(), Entries: []ports.Entry{entry(90)}}
	Sign(under, prop)
	under.Atts = []Attestation{Attest(under, v1)}
	if err := c.Append(*under); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("Condition A: a ½-of-frozen-weight coalition must be refused mid-epoch, got: %v", err)
	}
	if c.attesterQualified(idOf(joiner)) {
		t.Fatal("Condition A: a mid-epoch joiner must not be attester-qualified until the next rotation")
	}
	if c.proposerQualified(idOf(joiner)) {
		t.Fatal("Condition A: a mid-epoch joiner must not be proposer-qualified until the next rotation")
	}

	// Blocks 2..4: plain commits up to the boundary. The boundary block itself
	// still validates under the OLD epoch's quorum; rotation applies on commit.
	for h := uint64(2); h <= 4; h++ {
		prev, _ = c.Head()
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, prop)
		b.Atts = []Attestation{Attest(b, v1), Attest(b, v2)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit block %d: %v", h, err)
		}
	}

	// The height-4 boundary rotates the epoch: the join integrates, and the
	// weight DENOMINATOR moves exactly once, at a finalized block. Over the new
	// 5×2 MiB set, proposer + 2 attesters is 6/10 = 60% — refused where the same
	// coalition cleared ¾ of the old epoch; adding the joiner (8/10) commits.
	if n := c.validatorSetSize(); n != 5 {
		t.Fatalf("epoch rotation at height 4 must integrate the join: validatorSetSize = %d, want 5", n)
	}
	if !c.attesterQualified(idOf(joiner)) {
		t.Fatal("a joiner must be attester-qualified after the boundary rotation")
	}
	prev, _ = c.Head()
	short := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(91)}}
	Sign(short, prop)
	short.Atts = []Attestation{Attest(short, v1), Attest(short, v2)}
	if err := c.Append(*short); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("post-rotation, the integrated join must raise the weight denominator (60%% refused), got: %v", err)
	}
	full := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(92)}}
	Sign(full, prop)
	full.Atts = []Attestation{Attest(full, v1), Attest(full, v2), Attest(full, joiner)}
	if err := c.Append(*full); err != nil {
		t.Fatalf("a >⅔ coalition over the rotated set must commit: %v", err)
	}
}

func TestHandoffOnlyAtEpochBoundary(t *testing.T) {
	a1, a2, a3, a4 := key(31), key(32), key(33), key(34)
	v1, v2, v3 := key(35), key(36), key(37)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true, idOf(a3): true, idOf(a4): true}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 2, MatureValidators: 2, EpochBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}

	// Block 1 (mid-epoch): the founding bonds drain in. Block 2: the bonded
	// validators ATTEST (C2Metric counts only validatorsSeen), so three equal
	// 2 MiB bonds give NakamotoBonds 2 ≥ MatureValidators 2 and the maturity
	// latch trips at height 2 — mid-epoch (the boundary is height 4).
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(v1, twoMiB, prev), bondReg(v2, twoMiB, prev), bondReg(v3, twoMiB, prev)}}
	Sign(b1, a1)
	b1.Atts = []Attestation{Attest(b1, a2), Attest(b1, a3)}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit block 1 (bond drain): %v", err)
	}
	prev, _ = c.Head()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(b2, a1)
	b2.Atts = []Attestation{Attest(b2, a2), Attest(b2, a3), Attest(b2, v1), Attest(b2, v2), Attest(b2, v3)}
	if err := c.Append(*b2); err != nil {
		t.Fatalf("commit block 2 (bonded validators attest): %v", err)
	}
	if !c.EverMature() {
		t.Fatal("setup: the maturity latch should trip at block 2 (Nakamoto 2 over 3 equal attested bonds)")
	}

	// Condition B: the latch is a consensus FACT, but the HANDOFF — anchors shed,
	// weight meaning flips to committed bond, quorum re-sizes onto the bonded set —
	// must wait for the next epoch boundary (a finalized block), not fire mid-epoch.
	if !c.launchAnchor(idOf(a2)) {
		t.Fatal("Condition B: anchors must keep governing after a mid-epoch latch, until the boundary rotation")
	}
	if n := c.validatorSetSize(); n != 4 {
		t.Fatalf("Condition B: quorum stays sized on the anchor set until the boundary: validatorSetSize = %d, want 4", n)
	}
	// The anchor training-wheels sign-off must also still be required: a commit
	// attested only by bonded validators (no anchors) is refused pre-handoff.
	prev, _ = c.Head()
	nb := &Block{Version: 1, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(nb, a1)
	nb.Atts = []Attestation{Attest(nb, v1), Attest(nb, v2)}
	if err := c.Append(*nb); !errors.Is(err, ErrAnchorRequired) {
		t.Fatalf("Condition B: a no-anchor commit before the handoff boundary must fail ErrAnchorRequired, got %v", err)
	}

	// Commit through the boundary (anchor-attested).
	for h := uint64(3); h <= 4; h++ {
		prev, _ = c.Head()
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, a1)
		b.Atts = []Attestation{Attest(b, a2), Attest(b, a3)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit block %d: %v", h, err)
		}
	}

	// The height-4 boundary is the handoff: the first mature rotation. Anchors shed
	// (eligibility, weight, and the anchor-quorum gate), and the finality quorum is
	// now sized on the frozen bonded snapshot {v1,v2,v3}.
	if c.launchAnchor(idOf(a2)) {
		t.Fatal("handoff: anchors must shed at the boundary rotation")
	}
	if n := c.validatorSetSize(); n != 3 {
		t.Fatalf("handoff: validatorSetSize must be the bonded snapshot (3), got %d", n)
	}
	if c.proposerQualified(idOf(a1)) {
		t.Fatal("handoff: a zero-bond anchor must lose proposer qualification at the boundary")
	}
	// Post-handoff, a bonded-validator commit with NO anchor sign-off succeeds.
	prev, _ = c.Head()
	b5 := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(5)}}
	Sign(b5, v1)
	b5.Atts = []Attestation{Attest(b5, v2), Attest(b5, v3)}
	if err := c.Append(*b5); err != nil {
		t.Fatalf("post-handoff bonded commit (no anchors): %v", err)
	}
}

func TestEpochTTLExpiryIntegratesAtRotation(t *testing.T) {
	prop, v1, v2, v3 := key(41), key(42), key(43), key(44)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4, BondTTLBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(v1, twoMiB, ports.Hash{}),
		bondReg(v2, twoMiB, ports.Hash{}),
		bondReg(v3, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}

	// Blocks 1..3: prop/v1/v2 renew every block; v3 never does, so its TTL (2)
	// lapses at height 3 — mid-epoch.
	for h := uint64(1); h <= 3; h++ {
		prev, _ := c.Head()
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))},
			BondRegs: []BondReg{bondReg(prop, twoMiB, prev), bondReg(v1, twoMiB, prev), bondReg(v2, twoMiB, prev)}}
		Sign(b, prop)
		b.Atts = []Attestation{Attest(b, v1), Attest(b, v2)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit block %d: %v", h, err)
		}
	}
	if c.bonded[idOf(v3)] != 0 {
		t.Fatal("setup: v3's bond should have TTL-lapsed from the live ledger by height 3")
	}

	// Condition A: membership is FROZEN for the epoch. A protocol-forced mid-epoch
	// disqualification (TTL expiry) would shrink the honest attester supply below
	// the frozen N and can stall the chain before it ever reaches the boundary to
	// rotate the expiry in — so expiry integrates at the NEXT rotation, bounded by
	// EpochBlocks (an epoch must be well under BondTTLBlocks' renewal margin).
	if !c.attesterQualified(idOf(v3)) {
		t.Fatal("Condition A: a mid-epoch TTL expiry must not disqualify until the boundary rotation")
	}
	if n := c.validatorSetSize(); n != 4 {
		t.Fatalf("Condition A: mid-epoch TTL expiry must not resize the finality set: got %d, want 4", n)
	}

	// The height-4 boundary integrates the expiry.
	prev, _ := c.Head()
	b4 := &Block{Version: 1, Height: 4, Prev: prev, Entries: []ports.Entry{entry(4)},
		BondRegs: []BondReg{bondReg(prop, twoMiB, prev), bondReg(v1, twoMiB, prev), bondReg(v2, twoMiB, prev)}}
	Sign(b4, prop)
	b4.Atts = []Attestation{Attest(b4, v1), Attest(b4, v2)}
	if err := c.Append(*b4); err != nil {
		t.Fatalf("commit boundary block 4: %v", err)
	}
	if c.attesterQualified(idOf(v3)) {
		t.Fatal("rotation: a TTL-lapsed bond must drop out of the epoch set at the boundary")
	}
	if n := c.validatorSetSize(); n != 3 {
		t.Fatalf("rotation: validatorSetSize after integrating the expiry = %d, want 3", n)
	}
}

func TestSlashDisqualifiesMidEpochWithFrozenN(t *testing.T) {
	prop, v1, v2, v3, v4 := key(51), key(52), key(53), key(54), key(55)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(v1, twoMiB, ports.Hash{}),
		bondReg(v2, twoMiB, ports.Hash{}),
		bondReg(v3, twoMiB, ports.Hash{}),
		bondReg(v4, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	if n, rq := c.validatorSetSize(), c.RequiredQuorum(); n != 5 || rq != 1 {
		t.Fatalf("setup: founding epoch set N=%d (want 5), RequiredQuorum=%d (want the Quorum=1 count floor; the Byzantine bar is weight-counted, B2)", n, rq)
	}

	// v4 provably double-signs (two different blocks at the same height); block 1
	// carries the slash. Slashing is the ONE live mid-epoch disqualification:
	// proven misbehavior is removed immediately (safety), but N stays FROZEN — a
	// shrinking N would lower bftThreshold mid-epoch and weaken quorum
	// intersection in the exact window the freeze exists to protect.
	xa := &Block{Version: 1, Height: 9, Prev: g.Hash(), Entries: []ports.Entry{entry(101)}}
	Sign(xa, v4)
	xb := &Block{Version: 1, Height: 9, Prev: g.Hash(), Entries: []ports.Entry{entry(102)}}
	Sign(xb, v4)
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
		Slashes: []Equivocation{{Culprit: append([]byte(nil), v4.Public().(ed25519.PublicKey)...), A: *xa, B: *xb}}}
	Sign(b1, prop)
	b1.Atts = []Attestation{Attest(b1, v1), Attest(b1, v2), Attest(b1, v3)}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit block 1 (slash): %v", err)
	}

	if c.attesterQualified(idOf(v4)) {
		t.Fatal("a slashed validator must be disqualified immediately, mid-epoch")
	}
	if n := c.validatorSetSize(); n != 5 {
		t.Fatalf("frozen N: a mid-epoch slash must not shrink the finality set size: got %d, want 5", n)
	}
	// Frozen-denominator discipline in WEIGHT terms (B2): the slash removes v4's
	// vote but must NOT shrink the ⅔ base — the denominator stays the frozen
	// 5×2 MiB, so the bar can only get EFFECTIVELY harder, never weaker, exactly
	// the shrink-only rule head-counting had. Proposer + v1 + v2 = 6/10 = 60% is
	// refused (were the denominator live-shrunk to 8 MiB it would be 75% and
	// commit); proposer + v1 + v2 + v3 = 8/10 = 80% commits.
	prev, _ = c.Head()
	under := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(under, prop)
	under.Atts = []Attestation{Attest(under, v1), Attest(under, v2)}
	if err := c.Append(*under); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("the slashed member's weight must stay in the frozen denominator (60%% refused), got: %v", err)
	}
	full := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(full, prop)
	full.Atts = []Attestation{Attest(full, v1), Attest(full, v2), Attest(full, v3)}
	if err := c.Append(*full); err != nil {
		t.Fatalf("a >⅔-of-frozen-weight coalition must commit after the slash: %v", err)
	}
}

// TestDrainWindowOrderingConvergence is the certification's repro-ladder step 2:
// independent per-validator registration-commit ordering across the drain window.
// A lagging replica catches up to a fork that EXTENDS its committed prefix
// (convergence), while a conflicting drain ordering — even a heavier, longer one —
// is refused without dropping committed height (no reorg below a super-quorum-
// committed block; D-1 prefers the stall). Weight stays strictly monotone as the
// drain commits, so fork-choice never decides on the degenerate zero-weight tie.
func TestDrainWindowOrderingConvergence(t *testing.T) {
	a1, a2, a3, a4 := key(61), key(62), key(63), key(64)
	v1, v2, v3 := key(65), key(66), key(67)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true, idOf(a3): true, idOf(a4): true}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 2, MatureValidators: 99, EpochBlocks: 4}
	mk := func() (*Chain, *Block) {
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		Sign(g, a1)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("append genesis: %v", err)
		}
		return c, g
	}
	drainBlock := func(h uint64, prev ports.Hash, e byte, reg BondReg) *Block {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(e)},
			BondRegs: []BondReg{reg}}
		Sign(b, a1)
		b.Atts = []Attestation{Attest(b, a2), Attest(b, a3)}
		return b
	}

	// Replica R1 commits the drain in order v1;v2;v3, one registration per block.
	r1, g := mk()
	var chainX []Block
	chainX = append(chainX, *g)
	lastW := int64(0)
	order := []struct {
		k ports.NodeID
		r func(prev ports.Hash) BondReg
	}{
		{idOf(v1), func(p ports.Hash) BondReg { return bondReg(v1, twoMiB, p) }},
		{idOf(v2), func(p ports.Hash) BondReg { return bondReg(v2, twoMiB, p) }},
		{idOf(v3), func(p ports.Hash) BondReg { return bondReg(v3, twoMiB, p) }},
	}
	for i, o := range order {
		prev, _ := r1.Head()
		b := drainBlock(uint64(i+1), prev, byte(i+1), o.r(prev))
		if err := r1.Append(*b); err != nil {
			t.Fatalf("R1 drain block %d: %v", i+1, err)
		}
		chainX = append(chainX, *b)
		if w := r1.Weight(); w <= lastW {
			t.Fatalf("drain weight must be strictly monotone (anchor bootstrap weight): block %d weight %d, prev %d", i+1, w, lastW)
		} else {
			lastW = w
		}
	}

	// Convergence: a lagging replica (committed only block 1) adopts the full
	// chain — it extends its committed prefix, so finality permits the catch-up.
	r2, _ := mk()
	if err := r2.Append(chainX[1]); err != nil {
		t.Fatalf("R2 commit block 1: %v", err)
	}
	adopted, err := r2.Reconcile(chainX)
	if err != nil || !adopted {
		t.Fatalf("a lagging replica must adopt the extending drain chain (adopted=%v err=%v)", adopted, err)
	}
	_, h1 := r1.Head()
	_, h2 := r2.Head()
	if h1 != h2 {
		t.Fatalf("convergence: replicas must agree on head height (R1 %d, R2 %d)", h1, h2)
	}

	// A CONFLICTING drain ordering (v2;v1 from genesis — different blocks, valid
	// anchor quorum) must not displace R1's committed history, even extended one
	// block LONGER/heavier than R1's chain: committed blocks are final, the
	// conflicting fork is refused, and the head never drops (prefer stall, D-1).
	chainY := []Block{*g}
	yOrder := []func(prev ports.Hash) BondReg{
		func(p ports.Hash) BondReg { return bondReg(v2, twoMiB, p) },
		func(p ports.Hash) BondReg { return bondReg(v1, twoMiB, p) },
		func(p ports.Hash) BondReg { return bondReg(v3, twoMiB, p) },
	}
	prevY := g.Hash()
	for i, mkReg := range yOrder {
		b := drainBlock(uint64(i+1), prevY, byte(0x80+i), mkReg(prevY))
		chainY = append(chainY, *b)
		prevY = b.Hash()
	}
	// One extra committed block makes Y strictly longer and heavier than X.
	b4 := &Block{Version: 1, Height: 4, Prev: prevY, Entries: []ports.Entry{entry(0x90)}}
	Sign(b4, a1)
	b4.Atts = []Attestation{Attest(b4, a2), Attest(b4, a3)}
	chainY = append(chainY, *b4)

	_, before := r1.Head()
	adopted, err = r1.Reconcile(chainY)
	if adopted {
		t.Fatal("finality: a conflicting drain ordering must never displace committed blocks, however heavy")
	}
	if err != nil && !errors.Is(err, ErrPreFinalityReorg) {
		t.Fatalf("conflicting fork refusal: want ErrPreFinalityReorg (or weight refusal), got %v", err)
	}
	if _, after := r1.Head(); after != before {
		t.Fatalf("head must be non-decreasing across a refused conflicting fork: %d → %d", before, after)
	}
}
