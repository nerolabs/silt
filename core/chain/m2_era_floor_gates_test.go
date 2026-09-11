package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// M2 — THE ERA-FLOOR EVIDENCE REFUSAL. The gates for the rule that CheckEquivocation refuses
// evidence whose FORM is below the chain's committed era floor at the evidence's own height.
//
// Certification: NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11
// (Layer 1, GATED: G-1b the four-site landing, G-1c these RED-first probes). The face being
// closed is DRIVEN on main today by TestGPRE6_CrossChainHonestSignaturesAreNotEvidence's
// pre-era-4 arm: an honest validator that precommitted ONCE on network X is convicted on
// network X using a leg harvested from network Y, because the era-2 attestation form binds no
// chain id and era 2 is frozen forever.

// eraFloorOf is THE test supplier of an EraFloor, and it is the only one in the tree: it builds a
// SEPARATE chain whose genesis commits Era3/Era4ActivationHeight = at, and reads the floor off
// (*Chain).EraFloor — i.e. MintVersion, the one site of the era -> version map. Three properties
// make it the right shape:
//
//   - THE GUARD DOES NOT COME FROM ITS SUBJECT. The chain holds no blocks, is never appended to,
//     and never sees the evidence it judges. Re-deriving "h >= at" inline would be a second copy
//     of the era map, which is the #397 drift shape this whole arc is paying back.
//   - A FIXTURE CANNOT CONSTRUCT AN UNREACHABLE CHAIN. Every floor a test can express is a floor
//     some real genesis produces. In particular MintVersion never returns v1, so no argument to
//     this function re-admits era-1 evidence — which is the rule, not a gap in the helper.
//   - at = 0 is the latch route with nothing latched: the floor is v2 at EVERY height, the
//     weakest floor any silt chain can have. at = 1 is the RC shape: v5 at every height above
//     the genesis.
func eraFloorOf(at uint64) EraFloor {
	return New(Config{Quorum: 1, Era3ActivationHeight: at, Era4ActivationHeight: at},
		func(ports.NodeID) int64 { return 0 }).EraFloor()
}

// m2CrossNetworkPair builds the two legs of the cross-network FALSE slash: ONE honest
// precommit by the victim on each of two silt networks, at one (height, round, step), over two
// different bodies. Neither act is a protocol violation. At `version` below era 4 the
// attestation form carries no chain id, so the two legs are portable between networks and the
// pair convicts on either.
//
// THE FIXTURE IS NON-UNIFORM BY CONSTRUCTION: the two blocks have different proposers, different
// entries and different chain ids, so an accidental self-comparison cannot report green.
func m2CrossNetworkPair(t *testing.T, version, height uint64, cidX, cidY ports.Hash) (Equivocation, []byte) {
	t.Helper()
	victim, propX, propY := key(97001), key(97002), key(97003)
	pub := []byte(victim.Public().(ed25519.PublicKey))
	build := func(e byte, prop ed25519.PrivateKey, cid ports.Hash) Block {
		b := Block{Version: version, Height: height, Prev: ports.HashBytes([]byte("shared-prev")),
			Entries: []ports.Entry{entry(e)}}
		if version >= BlockVersionWitnessable {
			b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
			setD3Digests(&b)
		}
		Sign(&b, prop)
		b.Atts = []Attestation{AttestAt(&b, victim, 1, PhasePrecommit, cid)}
		return b
	}
	a, b := build(1, propX, cidX), build(2, propY, cidY)
	if cidX != cidY && version < BlockVersionWitnessable {
		// The premise of the harvest: below era 4 the leg is BIT-IDENTICAL across networks.
		same := build(2, propY, cidX)
		if string(same.Atts[0].Sig) != string(b.Atts[0].Sig) {
			t.Fatal("PREMISE FALSE: the sub-era-4 precommit differs across chain ids, so it is not " +
				"chain-blind and this pair is not the cross-network harvest it claims to be")
		}
	}
	return Equivocation{Culprit: pub, A: a, B: b}, pub
}

// m2MintCarrying mints the chain's next block carrying evs, the way an honest proposer does:
// the slashes are set BEFORE the era-4 roots are populated (the roots cover the slash's own
// transition effect) and therefore before the block is signed.
func m2MintCarrying(t *testing.T, c *Chain, keys []ed25519.PrivateKey, evs ...Equivocation) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, Slashes: evs}
	b.LastCommit = c.HeadCarrier()
	if err := c.PopulateEra4Roots(b); err != nil {
		t.Fatalf("populate era-4 roots at height %d: %v", next, err)
	}
	if b.Version != BlockVersionWitnessable {
		t.Fatalf("GATE VACUOUS: the carrier block must be era-4 form, got v%d", b.Version)
	}
	twoPhaseSign(b, keys, c.ChainID())
	return b
}

// ---------------------------------------------------------------------------
// G-EF-5 — THE LIVE-ACCEPT / RELOAD SPLIT. The probe whose absence let the omission through.
// ---------------------------------------------------------------------------
//
// An era-4 block carrying a cross-network proof is decided by TWO different functions on two
// different paths, and the rule must land on BOTH:
//
//   - LIVE ACCEPT: ValidateCommit -> ValidateCommitV5 -> P8 v5ValidateSlashes (also the path of
//     ValidateProposal, Append, Reconcile and the cold auditor's (*Box).Validate);
//   - RELOAD: appendStructural -> validateStructural -> (*Chain).validateSlashes, the own-disk
//     replay a restart runs.
//
// The gate asserts the two verdicts AGREE and that both REFUSE. It therefore reddens on three
// distinct defects: (i) no floor anywhere — the I5 violation open on main, both paths ACCEPT;
// (ii) the floor at the reload site only — ACCEPT live, REFUSE on restart, which is one operator
// diverging from ITSELF across a restart, a failure mode created by the fix; (iii) the floor at
// the live site only — the same split mirrored.
//
// THE GUARD DOES NOT COME FROM THE SUBJECT: the floor is read from the chain's own MintVersion,
// and the evidence blocks are free-standing (never appended, never linked to this chain).
func TestGEF5_EraFloorRefusesOnBothTheLiveAcceptAndReloadPaths(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)  // RC shape: Era4ActivationHeight = 1
	fresh, _ := era4AnchorChain(t, 1, 1) // the same genesis, replayed from disk
	if c.ChainID() != fresh.ChainID() {
		t.Fatal("fixture: the reload chain must be the SAME network, or the two paths judge different chains")
	}
	const evidenceHeight = 12
	if got := c.MintVersion(evidenceHeight); got != BlockVersionWitnessable {
		t.Fatalf("GATE OUT OF SCOPE: the era floor at height %d must be v%d on an RC-shaped chain, got v%d",
			uint64(evidenceHeight), BlockVersionWitnessable, got)
	}
	cidY := ports.HashBytes([]byte("another silt network's genesis"))
	if cidY == c.ChainID() {
		t.Fatal("fixture: the two networks must differ")
	}

	// The harvested pair: sub-era-4 form, at a height where the chain's floor is era 4.
	false_, _ := m2CrossNetworkPair(t, BlockVersionStateRoot, evidenceHeight, c.ChainID(), cidY)
	if false_.A.Atts[0].Phase != PhasePrecommit {
		t.Fatalf("GATE VACUOUS: the harvested leg must carry the era-2 wire phase, got %d", false_.A.Atts[0].Phase)
	}
	blk := m2MintCarrying(t, c, keys, false_)

	live := c.ValidateCommit(blk)
	reload := fresh.appendStructural(*blk)
	switch {
	case live == nil && reload == nil:
		t.Fatalf("G-EF-5 RED (the I5 violation, both paths): an era-4 block carrying a proof whose " +
			"sub-era-4 leg is bit-identical to one harvested from another silt network is ACCEPTED by " +
			"BOTH the live accept path (ValidateCommit -> P8 v5ValidateSlashes) and the own-disk reload " +
			"path (appendStructural -> validateSlashes). The honest validator is convicted and its bond " +
			"goes to zero. Closer: the era-floor conjunct in CheckEquivocation, at all four sites.")
	case live == nil:
		t.Fatalf("G-EF-5 RED (the SPLIT, and it is worse than the defect): the block is ACCEPTED live "+
			"and REFUSED from this node's own disk on restart (%v). One operator diverges from ITSELF "+
			"across a restart and truncates to the longest valid prefix. The floor landed at "+
			"(*Chain).validateSlashes but not at v5ValidateSlashes.", reload)
	case reload == nil:
		t.Fatalf("G-EF-5 RED (the SPLIT, mirrored): the block is REFUSED live (%v) and ACCEPTED from "+
			"disk on reload. The floor landed at v5ValidateSlashes but not at (*Chain).validateSlashes, "+
			"so a restart re-admits what the live path refused.", live)
	}
	if !errors.Is(live, ErrBadSlash) || !errors.Is(reload, ErrBadSlash) {
		t.Fatalf("both paths refuse, but not as a bad slash: live=%v reload=%v", live, reload)
	}

	// NON-VACUITY, on BOTH paths: a GENUINE era-4-form double-sign on THIS network at the same
	// height must still convict and the carrier block must still commit. Without this arm the
	// refusal above could be "P8 rejects every block with a Slashes field".
	real, _ := m2CrossNetworkPair(t, BlockVersionWitnessable, evidenceHeight, c.ChainID(), c.ChainID())
	if err := CheckEquivocation(&real, c.ChainID(), c.EraFloor()); err != nil {
		t.Fatalf("NON-VACUITY BROKEN: a genuine era-4 double-sign on ONE network must still be evidence; got %v", err)
	}
	good := m2MintCarrying(t, c, keys, real)
	if err := c.ValidateCommit(good); err != nil {
		t.Fatalf("NON-VACUITY BROKEN on the live accept path: a block carrying a genuine era-4 proof was refused: %v", err)
	}
	if err := fresh.appendStructural(*good); err != nil {
		t.Fatalf("NON-VACUITY BROKEN on the reload path: a block carrying a genuine era-4 proof was refused: %v", err)
	}
}

// ---------------------------------------------------------------------------
// G-EF-6 — THE TWO FLOOR DERIVATIONS AGREE. The #397 drift shape, bounded.
// ---------------------------------------------------------------------------
//
// Sites 2-4 derive the floor from (*Chain).MintVersion. Site 1 cannot: the accept composition
// takes a StateView and is source-gated against holding a *Chain, so it derives the same floor
// from v5EraFloorAt. That is TWO COPIES OF ONE MAPPING — the drift shape this whole arc is paying
// back — and it is not eliminable, because the box's independence from the node is worth more
// than the duplication costs. So it is BOUNDED: the equality is driven here rather than rested on
// v5EraActive's doc claim that it "mirrors Chain.era3Active / Chain.era4Active", because a doc
// claim decays exactly like a cited test name.
//
// BOTH ROUTES ARE COVERED, and they are different code. On the genesis-config route v5EraActive
// short-circuits on v.Params() and reads no committed scalar. On the LATCH route it reads
// tagEra4LockedIn / tagEra4Height, which is a different derivation with a different failure mode
// (an unwitnessed scalar). A matrix over only the first route would report green while the second
// drifted.
//
// ABLATION (run it before believing this gate): flip v5EraFloorAt's era-4 test to `if !era4` or
// change v5EraActive's `h >= cfgHeight` to `h > cfgHeight` — the boundary height row goes RED.
func TestGEF6_TheTwoEraFloorDerivationsAgree(t *testing.T) {
	heights := []uint64{0, 1, 2, 3, 4, 5, 8, 63, 64, 65, 1 << 20}

	// --- route 1: the genesis-config boundaries, including the RC shape and a late one. ---
	for _, at := range []struct{ era3, era4 uint64 }{{1, 1}, {2, 4}, {64, 64}, {1, 64}} {
		c, _ := era4AnchorChain(t, at.era3, at.era4)
		v := liveView{c}
		for _, h := range heights {
			want := c.MintVersion(h)
			got, out, err := v5EraFloorAt(v, h)
			if out != Accept {
				t.Fatalf("config route (H_era3=%d H_era4=%d) height %d: the floor must be DETERMINATE — "+
					"v5EraActive short-circuits on v.Params() and reads no Scalar here, so a stall means "+
					"the short-circuit is gone and P8 gained a stall site nobody priced: %s %v",
					at.era3, at.era4, h, out, err)
			}
			if got != want {
				t.Fatalf("FLOOR DRIFT (config route, H_era3=%d H_era4=%d) at height %d: the node says v%d "+
					"(MintVersion) and the composition says v%d (v5EraFloorAt). Two copies of one era map "+
					"disagreeing is the #397 shape, and here it decides a SLASH: the node would accept a "+
					"block the box refuses, or the reverse", at.era3, at.era4, h, want, got)
			}
		}
	}

	// --- route 2: the LATCH, where the floor comes from committed scalars, not config. ---
	whale := key(53421)
	minnows := []ed25519.PrivateKey{key(53422), key(53423), key(53424)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, BlockVersionWitnessable))
	for _, m := range minnows {
		g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionWitnessable))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("latch rig genesis: %v", err)
	}
	for c.Len() < 4 {
		commit(t, c, whale, minnows)
	}
	if !c.era4LockedIn || c.cfg.Era4ActivationHeight != 0 {
		t.Fatalf("RIG DEAD: this arm must exercise the LATCH derivation, not the config one "+
			"(lockedIn=%v cfgHeight=%d). Without a latched chain the second derivation is never run and "+
			"this arm re-tests route 1", c.era4LockedIn, c.cfg.Era4ActivationHeight)
	}
	v := liveView{c}
	for _, h := range heights {
		want := c.MintVersion(h)
		got, out, err := v5EraFloorAt(v, h)
		if out != Accept {
			t.Fatalf("latch route height %d: the live view witnesses its own committed scalars, so the "+
				"floor must be determinate here: %s %v", h, out, err)
		}
		if got != want {
			t.Fatalf("FLOOR DRIFT (latch route) at height %d (H_era4=%d): node says v%d, composition says "+
				"v%d", h, c.era4Height, want, got)
		}
	}
	// NON-VACUITY: the latched boundary must actually MOVE the floor across the heights probed,
	// or the agreement above is agreement on one constant.
	if c.MintVersion(0) == c.MintVersion(1<<20) {
		t.Fatalf("ARM VACUOUS: the floor is v%d at every probed height on the latch rig, so node and "+
			"composition would agree even if neither read the latch", c.MintVersion(0))
	}
}

// ---------------------------------------------------------------------------
// G-EF-8 — WHAT THE ERA FLOOR DELIBERATELY DOES NOT REFUSE, driven.
// ---------------------------------------------------------------------------
//
// The conjunct is SCOPED to heights where the chain requires the chain-bound era-4 form, because
// that is where the certification's closure table (§1.3) says it closes anything. Below that
// boundary the required form is itself chain-blind, so refusing a leg buys no closure — while
// costing the era-1 arm of the I5 accountable-safety model check and the T1 variant of
// R-NEST-GATE, two artifacts this certification does not price and a builder must not amend.
//
// THE DEFERRAL IS DRIVEN, NOT ASSUMED. If a later ruling removes the scope, this gate goes RED and
// names what moved — which is the point. It is the complement of G-EF-5: together they say
// exactly which heights the rule binds at.
func TestGEF8_TheDeferredSubEra4SurfaceIsStillAdmissible(t *testing.T) {
	cidX := ports.HashBytes([]byte("network X genesis"))
	cidY := ports.HashBytes([]byte("network Y genesis"))
	const h = 12

	// (a) an ERA-1-form pair, at the weakest floor a silt chain can have.
	culprit := key(98001)
	pub := []byte(culprit.Public().(ed25519.PublicKey))
	mk1 := func(e byte) Block {
		b := Block{Version: 1, Height: h, Prev: ports.HashBytes([]byte("p")), Entries: []ports.Entry{entry(e)}}
		Sign(&b, culprit)
		return b
	}
	era1 := Equivocation{Culprit: pub, A: mk1(1), B: mk1(2)}
	if got := eraFloorOf(0)(h); got != BlockVersionRounds {
		t.Fatalf("fixture: the weakest floor must be v%d, got v%d", BlockVersionRounds, got)
	}
	if err := CheckEquivocation(&era1, ports.Hash{}, eraFloorOf(0)); err != nil {
		t.Fatalf("DEFERRED SURFACE MOVED: an era-1-form pair at a v%d floor is no longer evidence (%v). "+
			"That is a NARROWING and therefore safe — but it reprices TestModelCheck_I5_AccountableSafety_Exhaustive's "+
			"era-1 arm (which demands these convict) and closes R-NEST-GATE's T1 variant. Neither is priced "+
			"by NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11. Route the ruling, "+
			"then update both artifacts and this gate together", BlockVersionRounds, err)
	}

	// (b) the cross-network era-2-form harvest, at a floor BELOW era 4: still convicts. This is the
	// residual §1.3 row 3 records, and Era4ActivationHeight = 1 buys it off rather than closing it.
	sub, _ := m2CrossNetworkPair(t, BlockVersionStateRoot, h, cidX, cidY)
	if err := CheckEquivocation(&sub, cidX, eraFloorOf(64)); err != nil {
		t.Fatalf("the h < H_era4 residual is CLOSED and nothing here closes it — read this as a CHANGE. "+
			"consensusSigBytes is frozen, so the sub-era-4 form binds no chain id at any height below the "+
			"boundary; got %v", err)
	}

	// (c) and the discriminator: the SAME pair at or above the boundary is refused. Without this,
	// (b) cannot tell "the floor is scoped" from "the floor is not read at all".
	if err := CheckEquivocation(&sub, cidX, eraFloorOf(1)); !errors.Is(err, ErrNotEquivocation) {
		t.Fatalf("the era floor is not binding where it must: the same pair at an era-4 floor must be "+
			"refused; got %v", err)
	}
	t.Logf("SCOPE, driven: the era floor binds at heights whose floor is v%d and defers below it. "+
		"The deferred surface is the era-1 form everywhere and the sub-era-4 form below H_era4 — open, "+
		"bought off on the RC network by Era4ActivationHeight=1, never closed.", BlockVersionWitnessable)
}

// ---------------------------------------------------------------------------
// G-EF-9 — AN ABSENT FLOOR REFUSES (certification C-3).
// ---------------------------------------------------------------------------
//
// The supplier is required, but "required" is a compile-time property and a nil interface value
// is not a compile error. uint64(0) is a valid-looking floor meaning "no floor" — today's
// fail-open — and it is what a caller gets for free. CheckEquivocation, VerifyEquivocation and
// FindEquivocations are EXPORTED with an out-of-package caller (core/node), so the absent case is
// reachable and must refuse.
//
// The direction check is the file's own: refusing is strictly narrowing, so the fail-safe costs a
// declined slash and can never manufacture one.
func TestGEF9_AnAbsentEraFloorRefuses(t *testing.T) {
	cid := ports.HashBytes([]byte("G-EF-9 chain"))
	real, _ := m2CrossNetworkPair(t, BlockVersionWitnessable, 7, cid, cid)

	// NON-VACUITY FIRST: with a floor, this pair convicts. Otherwise "refuses without a floor"
	// could be "refuses always".
	if err := CheckEquivocation(&real, cid, eraFloorOf(1)); err != nil {
		t.Fatalf("fixture: the pair must be genuine evidence when a floor IS supplied; got %v", err)
	}
	if err := CheckEquivocation(&real, cid, nil); !errors.Is(err, ErrNotEquivocation) {
		t.Fatalf("C-3 VIOLATED: a nil era-floor supplier must REFUSE. A caller with no chain cannot "+
			"know which network it is on, and guessing is the one thing it must not do; got %v", err)
	}
	if VerifyEquivocation(&real, cid, nil) {
		t.Fatal("C-3 VIOLATED at VerifyEquivocation: the bool wrapper must refuse with no floor too")
	}
	if got := FindEquivocations([]Block{real.A}, []Block{real.B}, cid, nil); len(got) != 0 {
		t.Fatalf("C-3 VIOLATED at FindEquivocations: detection with no floor must select nobody, or a "+
			"chainless node queues evidence every replica rejects; got %d proof(s)", len(got))
	}
	// ... and detection DOES select when a floor is supplied, so the line above is about the floor.
	if got := FindEquivocations([]Block{real.A}, []Block{real.B}, cid, eraFloorOf(1)); len(got) != 1 {
		t.Fatalf("fixture: detection must find the pair when a floor is supplied, got %d", len(got))
	}

	// THE REFUSAL PRECEDES THE READ. A chainless caller computes no floor at all, so the nil check
	// must come before any evaluation — a supplier that panics proves the order.
	panicky := EraFloor(func(uint64) uint64 { panic("the floor was evaluated on a refusal path") })
	bad := Equivocation{Culprit: []byte("too short"), A: real.A, B: real.B}
	if err := CheckEquivocation(&bad, cid, panicky); !errors.Is(err, ErrNotEquivocation) {
		t.Fatalf("a malformed culprit key must refuse before the floor is read; got %v", err)
	}
}
