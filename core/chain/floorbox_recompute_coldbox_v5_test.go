package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// THE COLD-BOX TIER (R-COLD-BOX-HARNESS)
// =============================================================================
//
// Research cert: floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md
// (§"What lifts the gate" items 2 and 3).
//
// WHY THIS TIER EXISTS. Every recompute gate in this package runs the recompute ON THE CHAIN THAT
// APPLIED THE HISTORY — the fixtures build `c`, `c.apply(...)` the blocks, and then call
// `c.RecomputeStateRootEntriesRevocations(...)`. The 29 `cloneForDryRun()` clones copy `matureEpoch`
// by value (era3validity.go). So every existing gate ran against a box whose live latch fields
// happened to match the committed pre-state. The test tier SHARED THE PRODUCER'S BLIND SPOT.
//
// The deployment target does not look like that. It "holds NO registry and replays NO apply()"
// (floorbox_recompute_stateroot_v5.go). Its `matureEpoch` / `everMature` are NEVER written — the
// only production writers are apply→rotateEpoch and adopt. That made the class-A mature-epoch branch
// UNREACHABLE on the real target and screened every mature-epoch block under the pre-maturity rule:
// a wrong-accept of a mid-epoch joiner against an attacker's root, and a false stall on an honest
// block, with every witness proof passing.
//
// Third occurrence of this shape in the floor-box spine (R1.3 fold-caught premise, class-P
// suppression, now live-state reads). The Tester's third-time rule: encode it as a permanent TIER,
// not a one-off test. `coldBox` is that tier — a box built by `New(cfg)` + `SetBondVerifier` that has
// NEVER applied a block, holds no registry, and is fed ONLY (prevStateRoot, committedStateRoot, b, w).
// Any gate re-run through `coldRecompute` is a gate that cannot be passing by live-state coincidence.

// coldBox returns a FRESH box: New(cfg) + the wired bond verifier, with NO apply(), NO adopt(), NO
// registry, and every live accelerator field at its zero value. It asserts that state so a future
// refactor that gives `New` a warm start cannot silently re-mask the tier.
func coldBox(t *testing.T, cfg Config) *Chain {
	t.Helper()
	// rep() is the LEGACY qualification source; a cold box must never reach it (the class-A screen
	// asserts objective mode). Return a value that WOULD qualify, so if the legacy branch is ever
	// reached the test fails by producing a wrong write-set, not by a coincidental zero.
	c := New(cfg, func(ports.NodeID) int64 { return 1 << 30 })
	c.SetBondVerifier(objectiveVerify)

	if c.matureEpoch {
		t.Fatalf("COLD-BOX BROKEN: a fresh New(cfg) has matureEpoch=true — the tier is vacuous")
	}
	if c.everMature {
		t.Fatalf("COLD-BOX BROKEN: a fresh New(cfg) has everMature=true — the tier is vacuous")
	}
	if c.handedOff() {
		t.Fatalf("COLD-BOX BROKEN: a fresh New(cfg) reports handedOff() — the tier is vacuous")
	}
	if len(c.bonded) != 0 || len(c.slashed) != 0 || len(c.epochSet) != 0 || len(c.validatorsSeen) != 0 {
		t.Fatalf("COLD-BOX BROKEN: a fresh New(cfg) holds registry state (bonded=%d slashed=%d epochSet=%d validatorsSeen=%d)",
			len(c.bonded), len(c.slashed), len(c.epochSet), len(c.validatorsSeen))
	}
	if !c.objective() {
		t.Fatalf("COLD-BOX MISCONFIGURED: cfg gives objective()=false (MinBond=%d) — the class-A screen would stall on the legacy assert",
			cfg.MinBond)
	}
	return c
}

// coldRecompute drives the REAL recompute entry on a cold box carrying only `cfg` — the exact
// trustless contract: the verdict must be a function of (prevStateRoot, committedStateRoot, b, w,
// own-cfg) and NOTHING else. Any gate that passes warm but not cold was passing on live state.
func coldRecompute(t *testing.T, cfg Config, prevStateRoot, committedStateRoot ports.Hash, b Block, w StateRootWitness) error {
	t.Helper()
	return coldBox(t, cfg).RecomputeStateRootEntriesRevocations(prevStateRoot, committedStateRoot, b, w)
}

// livePreForProbe builds a stateRootHandoffPre from a chain's LIVE latch state. It exists ONLY for
// unit probes that call an internal screen (attesterQualifiedFromScreen / stateRootAttWriteSet /
// attOps) DIRECTLY, bypassing the entry that anchors the pre-state from prevStateRoot. Production
// never does this: assembleStateRootRecomputeOps always supplies the Resolved handoffPreState. Never
// use it inside non-test code — the fold-file allowlist pin
// (floorbox_recompute_foldlivestate_pin_v5_test.go) reddens if a live latch read returns to a fold
// file.
func livePreForProbe(c *Chain) stateRootHandoffPre {
	return stateRootHandoffPre{everMature: c.everMature, matureEpoch: c.matureEpoch, handedOff: c.handedOff()}
}

// =============================================================================
// PART 1 — the EXISTING gates, re-run cold
// =============================================================================

// coldCase is one existing recompute scenario, rebuilt so it can be driven on a cold box. Each
// builder returns the cfg the box is built from plus the exact trustless inputs.
type coldCase struct {
	name  string
	build func(t *testing.T) (cfg Config, prevRoot, committed ports.Hash, b Block, w StateRootWitness)
}

func coldCases() []coldCase {
	return []coldCase{
		{
			// Class A: a mature-epoch chain, a qualified in-epochSet attester. WARM this passes on the
			// epochSet branch. COLD (pre-fix) the box has matureEpoch=false and takes the bonded branch;
			// the attester is ALSO bonded ≥ MinBond, so the verdict coincides and this case stays GREEN.
			// It is in the tier to prove the tier does not spuriously redden.
			name: "classA/qualified-att",
			build: func(t *testing.T) (Config, ports.Hash, ports.Hash, Block, StateRootWitness) {
				f := buildAttFixture(t)
				b := f.attBlock()
				return f.c.cfg, f.prevRoot, f.applyAndCommittedRoot(t, b), b, f.witnessForAtt(t, b)
			},
		},
		{
			// Class M: the OFF-boundary everMature crossing (pre-latch everMature=false).
			name: "classM/off-boundary-crossing",
			build: func(t *testing.T) (Config, ports.Hash, ports.Hash, Block, StateRootWitness) {
				f := buildOffBoundaryMaturityFixture(t)
				b := f.crossingBlock()
				return f.c.cfg, f.prevRoot, f.committedRoot(t, b), b, f.witnessForCrossing(t, b)
			},
		},
		{
			// Class M + P: the ON-boundary handoff block — the crossing AND the first mature rotation.
			// This is the case the cert singles out: apply() screens THIS block's atts under the PRE
			// value (rotate-LAST), and a box reading a post-handoff live field would screen them under
			// the post value.
			name: "classM+P/on-boundary-handoff",
			build: func(t *testing.T) (Config, ports.Hash, ports.Hash, Block, StateRootWitness) {
				f := buildHandoffFixture(t)
				b := f.handoffBoundaryBlock()
				return f.c.cfg, f.prevRoot, f.committedRoot(t, b), b, f.witnessForHandoff(t, b)
			},
		},
		{
			// Class P: a mature-epoch boundary rotation (freeze + tallies + rotate scalars).
			name: "classP/boundary-rotation",
			build: func(t *testing.T) (Config, ports.Hash, ports.Hash, Block, StateRootWitness) {
				f := buildRotateFixture(t)
				b := f.boundaryBlock(nil)
				return f.c.cfg, f.prevRoot, f.applyAndCommittedRoot(t, b), b, f.witnessForBoundary(t, b)
			},
		},
	}
}

// TestColdBox_ExistingGatesAgreeCold re-runs the class-A / class-M / class-P agreement gates on a
// box that never applied the history. Each must reach the SAME verdict the warm box reaches — that
// is the whole trustless contract. A case that is GREEN warm and RED cold is a live-state read.
func TestColdBox_ExistingGatesAgreeCold(t *testing.T) {
	for _, tc := range coldCases() {
		t.Run(tc.name, func(t *testing.T) {
			cfg, prevRoot, committed, b, w := tc.build(t)
			if err := coldRecompute(t, cfg, prevRoot, committed, b, w); err != nil {
				t.Fatalf("COLD-BOX RED: the recompute AGREES warm but STALLS on a cold box: %v\n"+
					"  A cold box is the deployment target (no apply(), no registry). A verdict that differs\n"+
					"  warm-vs-cold means the recompute read live box state, not the certified inputs.", err)
			}
		})
	}
}

// TestColdBox_ExistingAdversarialGatesStallCold re-runs the adversarial-root gates cold: a forged
// committed root must STALL on a cold box too. (Warm-only adversarial gates could pass because the
// warm box's live state happened to make the forge visible.)
func TestColdBox_ExistingAdversarialGatesStallCold(t *testing.T) {
	for _, tc := range coldCases() {
		t.Run(tc.name+"/tampered-committed-root", func(t *testing.T) {
			cfg, prevRoot, committed, b, w := tc.build(t)
			tampered := committed
			tampered[0] ^= 0xff
			err := coldRecompute(t, cfg, prevRoot, tampered, b, w)
			if err == nil {
				t.Fatalf("COLD-BOX WRONG-ACCEPT: a tampered committed StateRoot was accepted cold")
			}
			if !errors.Is(err, ErrRecomputeStateRootMismatch) && !errors.Is(err, ErrRecomputeStateRootMaturity) &&
				!errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootDigest) {
				t.Fatalf("COLD-BOX: expected a recompute stall for a tampered root, got %v", err)
			}
		})
	}
}

// =============================================================================
// PART 2 — the D1 divergence class, DRIVEN on a cold box
// =============================================================================
//
// D1 (cert Q1) is the set of attesters the two branches disagree about:
//   {bonded ≥ MinBond ∧ ∉ epochSet} ∪ {∈ epochSet ∧ bonded < MinBond} ∪ {cfg anchor ∧ ∉ epochSet}
// The canonical member is the I3 mid-epoch joiner: it "banks mid-epoch, gains zero weight until the
// next finalized boundary" (consensus-invariants.md I3). apply() (chain.go attesterQualifiedAt)
// EXCLUDES it under the mature-epoch rule; the pre-maturity rule INCLUDES it.

// midEpochJoiner is the D1 fixture: a mature-from-genesis chain (matureEpoch latched at the genesis
// rotation) plus an attester `Z` bonded ≥ MinBond at h=1 — AFTER the freeze — so Z is bonded but NOT
// in the frozen epochSet. The test block at h=2 carries Z's attestation.
type attScenario struct {
	c        *Chain
	cfg      Config
	prover   *statehash.Prover
	prevRoot ports.Hash
	preValue func([]byte) []byte
	b        Block
	// zID is the attester whose qualification the two branches DISAGREE about (the D1 member).
	zID ports.NodeID
}

func buildMidEpochJoiner(t *testing.T) attScenario {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(76001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)
	if !c.matureEpoch {
		t.Fatalf("fixture: matureEpoch must be latched post-genesis (MatureValidators=0)")
	}

	// Z bonds at h=1 — after the genesis freeze, so it is bonded but NOT in the frozen epochSet.
	z := key(76002)
	zID := ports.HashBytes(pubOf(z))
	prev1, h1 := c.Head()
	bBond := Block{Version: BlockVersionWitnessable, Height: h1, Prev: prev1}
	bBond.BondRegs = append(bBond.BondRegs, bondRegFull(z, ports.HashBytes(pubOf(z)), 4<<20, ports.Hash{}, 5, 9))
	Sign(&bBond, prop)
	c.apply(bBond)

	if _, inES := c.epochSet[zID]; inES {
		t.Fatalf("D1 fixture VACUOUS: Z must NOT be in the frozen epochSet")
	}
	if c.bonded[zID] < cfg.MinBond {
		t.Fatalf("D1 fixture VACUOUS: Z must be bonded >= MinBond (got %d, MinBond %d)", c.bonded[zID], cfg.MinBond)
	}
	if c.validatorsSeen[zID] {
		t.Fatalf("D1 fixture VACUOUS: Z must not already be in validatorsSeen")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	prev2, h2 := c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(55)}}
	b.LastCommit = append(b.LastCommit, carrierEntry(c, z))
	Sign(&b, prop)

	return attScenario{c: c, cfg: cfg, prover: prover, prevRoot: prover.Root(),
		preValue: preValue, b: b, zID: zID}
}

// buildHandoffWindow builds the #357 Condition-B WINDOW: everMature is LATCHED but matureEpoch is
// NOT yet — the deterministic ≤ EpochBlocks stretch between the maturity crossing and the first
// mature rotation that sheds the anchors (chain.go handedOff / launchAnchor). It is the regime where
// the two class-A branches disagree in the OTHER direction: epochSet is still EMPTY, so the
// frozen-set branch disqualifies EVERYONE while the pre-maturity branch qualifies every bonded
// member.
//
// It exists so the forged-MatureEpoch-true polarity isolates the DIRECTION-A ANCHOR: with
// everMature=true committed, the matureEpoch ⇒ everMature cross-check cannot fire, so only the
// anchor can catch the forge.
func buildHandoffWindow(t *testing.T) attScenario {
	t.Helper()
	f := buildOffBoundaryMaturityFixture(t)
	crossing := f.crossingBlock()
	// R-BOX-ATTESTS O1: the carrier fold excludes the PARENT's proposer. The crossing block IS the
	// window block's parent, so it must be proposed by someone OTHER than Z (= f.proposer) or Z
	// could never be carried into a seat and this gate would be VACUOUS. Re-sign it as att1; the
	// carrier it already holds is over ITS parent and is unaffected by its own proposer.
	Sign(&crossing, f.att1)
	f.c.apply(crossing) // the off-boundary maturity crossing: everMature false→true, NO rotation

	if !f.c.everMature {
		t.Fatalf("window fixture: everMature must be latched after the crossing block")
	}
	if f.c.matureEpoch {
		t.Fatalf("window fixture VACUOUS: matureEpoch must NOT be latched (no boundary has fired)")
	}
	if len(f.c.epochSet) != 0 {
		t.Fatalf("window fixture VACUOUS: epochSet must still be EMPTY before the first mature rotation")
	}

	cfg := f.c.cfg
	leaves := f.c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// The D1 member: the original proposer. It is bonded ≥ MinBond, NOT in epochSet (empty), and NOT
	// in validatorsSeen (apply() skips a block's own proposer's attestation, so it was never seated).
	// The next block is proposed by att1 so the proposer becomes an ordinary non-proposer attester.
	zID := ports.HashBytes(pubOf(f.proposer))
	if f.c.bonded[zID] < cfg.MinBond {
		t.Fatalf("window fixture VACUOUS: Z must be bonded >= MinBond")
	}
	if f.c.validatorsSeen[zID] {
		t.Fatalf("window fixture VACUOUS: Z must not already be in validatorsSeen (the ADD would be idempotent)")
	}

	prev, h := f.c.Head()
	if h%cfg.EpochBlocks == 0 {
		t.Fatalf("window fixture: h=%d must be OFF-boundary", h)
	}
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(91)}}
	b.LastCommit = append(b.LastCommit, carrierEntry(f.c, f.proposer))
	Sign(&b, f.att1)

	return attScenario{c: f.c, cfg: cfg, prover: prover, prevRoot: prover.Root(),
		preValue: preValue, b: b, zID: zID}
}

// witness builds the HONEST witness for the D1 test block: Z's screen carries the true committed
// facts (bonded PRESENT at its real size, epochSet ABSENT, slashed ABSENT), each with the proof that
// really verifies against prevStateRoot. The attacker forges NOTHING — that is the point of the
// finding. `seatZ` selects whether the witness also carries the validatorsSeen||Z write-target leaf
// (needed only when the box is expected to emit that ADD).
func (f attScenario) witness(t *testing.T, seatZ bool) StateRootWitness {
	t.Helper()
	var w StateRootWitness
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		return StateRootChangedLeafWitness{Key: k, OldValue: f.preValue(k), Proof: mustProve(f.prover, k)}
	}
	for _, wr := range applyEntriesRevocationsWriteSet(f.b) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	if seatZ {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(statehash.Key(tagValidatorsSeen, f.zID[:])))
	}
	sz, bp := f.c.bonded[f.zID]
	w.AttScreens = []StateRootAttScreen{{
		Attester: f.zID, Slashed: false, InEpochSet: false,
		BondedSize: sz, BondedPresent: bp,
		SlashedProof:  mustProve(f.prover, statehash.Key(tagSlashed, f.zID[:])),
		EpochSetProof: mustProve(f.prover, statehash.Key(tagEpochSet, f.zID[:])),
		BondedProof:   mustProve(f.prover, statehash.Key(tagBonded, f.zID[:])),
	}}
	w.ParentProposer, w.ParentProposerSig = f.c.CarrierParentProposerWitness()
	var preSeen []ports.NodeID
	for id := range f.c.validatorsSeen {
		preSeen = append(preSeen, id)
	}
	w.DigestPreSets = []StateRootDigestWitness{{
		Tag: tagValidatorsSeenRoot, PreIDs: sortIDs(preSeen),
		Proof: mustProve(f.prover, statehash.Key(tagValidatorsSeenRoot, nil)),
	}}
	if f.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], f.b.Height)
		w.DueBucketProof = mustProve(f.prover, statehash.Key(tagDueBucket, hk[:]))
	}
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	return w
}

// appliedSeats reports whether the ORACLE (real apply()) seats Z in validatorsSeen for this block.
func (f attScenario) appliedSeats(t *testing.T) bool {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(f.b)
	return clone.validatorsSeen[f.zID]
}

// honestRoot is the root a full node commits: apply() screens Z under the mature-epoch rule and
// SKIPS it (Z ∉ epochSet), so validatorsSeen is untouched.
func (f attScenario) honestRoot(t *testing.T) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(f.b)
	root, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	return root
}

// --- D1 GATE 1 (SAFETY): the adversarial mid-epoch-joiner attestation must STALL cold. ---
//
// The attack needs NO forgery. The attacker commits a root that includes validatorsSeen[Z] — a root
// full nodes REJECT (validateEra3Roots' dry-run apply() omits Z ⇒ root mismatch). A cold box that
// screened Z under the pre-maturity rule would prove `bonded[Z] ≥ MinBond` against prevStateRoot —
// a TRUE committed fact — emit the ADD, fold, and match the attacker's root: WRONG-ACCEPT, with
// every Resolve passing. The spurious validatorsSeen member is then the class-M poisoning entry that
// mis-sizes RecomputeMatureNow's C2Metric.
//
// RED BEFORE THE FIX: at e7a8aa4 this returns nil (wrong-accept).
func TestColdBox_D1_MidEpochJoinerAdversarialRootStalls(t *testing.T) {
	f := buildMidEpochJoiner(t)
	if f.appliedSeats(t) {
		t.Fatalf("ORACLE BROKEN: apply() seated a mid-epoch joiner — I3 says it banks mid-epoch and gains zero weight until the next finalized boundary")
	}
	forged := spuriousSeatedRoot(t, f.c, f.b, f.zID)
	if forged == f.honestRoot(t) {
		t.Fatalf("GATE VACUOUS: the spuriously-seated root equals the honest root")
	}
	err := coldRecompute(t, f.cfg, f.prevRoot, forged, f.b, f.witness(t, true))
	if err == nil {
		t.Fatalf("COLD-BOX WRONG-ACCEPT (D1 safety): the box ACCEPTED a root seating a mid-epoch joiner\n"+
			"  that a full node REJECTS. Z=%x is bonded >= MinBond but NOT in the frozen epochSet (I3).\n"+
			"  Nothing was forged: the box picked the PRE-maturity branch because it read its own\n"+
			"  (unset) c.matureEpoch instead of the committed pre-state.", f.zID[:4])
	}
	t.Logf("GATE GREEN (D1 safety): cold box STALLS on the mid-epoch-joiner root: %v", err)
}

// --- D1 GATE 2 (LIVENESS): the same attestation in an HONEST block must AGREE cold. ---
//
// An unqualified attestation is legal — apply() ignores it, never rejects the block. The honest
// committed root omits Z. A cold box on the pre-maturity branch EMITS Z ⇒ mismatch ⇒ false stall on
// an honest block. Under the flip that is Reject-where-the-network-Accepts: the fork-vs-halt hazard.
//
// RED BEFORE THE FIX: at e7a8aa4 this stalls with ErrRecomputeStateRootMismatch.
func TestColdBox_D1_MidEpochJoinerHonestBlockAgrees(t *testing.T) {
	f := buildMidEpochJoiner(t)
	if err := coldRecompute(t, f.cfg, f.prevRoot, f.honestRoot(t), f.b, f.witness(t, false)); err != nil {
		t.Fatalf("COLD-BOX FALSE STALL (D1 liveness): the box stalled on an HONEST block carrying a\n"+
			"  mid-epoch joiner's attestation (Z=%x). apply() ignores an unqualified att and commits a\n"+
			"  root without it; the box must reach the same verdict. Got: %v", f.zID[:4], err)
	}
}

// --- D1 GATE 3 (ANCHOR): a forged MatureEpoch.OldValue must STALL, in BOTH polarities. ---
//
// The Direction-A suppress-gate shape (matching the class-P scalar suppression gates). The new
// branch selector is only as sound as its anchor: if MatureEpoch.OldValue were trusted unresolved,
// an attacker would flip the class-A qualification branch at will — the same wrong-accept, now
// witness-driven instead of state-driven. The leaf is a bool, so each polarity is a DISTINCT forge
// and each needs a fixture whose HONEST pre-value is the opposite:
//
//   - forged-false: the mid-epoch-joiner fixture commits matureEpoch=true. Forging false flips the
//     box onto the pre-maturity branch, which would seat the mid-epoch joiner Z.
//   - forged-true: the handoff fixture commits matureEpoch=false (the boundary block IS the first
//     mature rotation, and rotate-LAST means the att loop still screens under the PRE value).
//     Forging true flips the box onto the frozen-epochSet branch.
func TestColdBox_D1_ForgedMatureEpochOldValueStalls(t *testing.T) {
	t.Run("forged-false/honest-pre-is-true", func(t *testing.T) {
		f := buildMidEpochJoiner(t)
		w := f.witness(t, false)
		if !decodeBoolLeaf(w.Maturity.MatureEpoch.OldValue) {
			t.Fatalf("GATE VACUOUS: this fixture must commit matureEpoch=true for the forge to be a forge")
		}
		w.Maturity.MatureEpoch.OldValue = statehash.EncodeBool(false)
		assertForgedMatureEpochStalls(t, coldRecompute(t, f.cfg, f.prevRoot, f.honestRoot(t), f.b, w), false)
	})
	t.Run("forged-true/honest-pre-is-false", func(t *testing.T) {
		// The Condition-B window: everMature=true, matureEpoch=false. everMature=true committed means
		// the matureEpoch ⇒ everMature cross-check CANNOT fire, so this polarity isolates the anchor.
		f := buildHandoffWindow(t)
		w := f.witness(t, true)
		if decodeBoolLeaf(w.Maturity.MatureEpoch.OldValue) {
			t.Fatalf("GATE VACUOUS: the window fixture must commit matureEpoch=false pre-state")
		}
		if !decodeBoolLeaf(w.Maturity.EverMature.OldValue) {
			t.Fatalf("GATE VACUOUS: the window fixture must commit everMature=true, or the cross-check catches the forge instead of the anchor")
		}
		if !f.appliedSeats(t) {
			t.Fatalf("GATE VACUOUS: the oracle must SEAT Z under the pre-maturity rule, or forging the branch changes nothing")
		}
		w.Maturity.MatureEpoch.OldValue = statehash.EncodeBool(true)
		assertForgedMatureEpochStalls(t, coldRecompute(t, f.cfg, f.prevRoot, f.honestRoot(t), f.b, w), true)
	})

	// The COMPLEMENT of the window forge: with the HONEST pre-state the same block must AGREE. This is
	// the Condition-B liveness case — epochSet is still empty, so a box that took the frozen-set branch
	// would drop every attester and stall on every honest block in the window.
	t.Run("window/honest-pre-agrees", func(t *testing.T) {
		f := buildHandoffWindow(t)
		if err := coldRecompute(t, f.cfg, f.prevRoot, f.honestRoot(t), f.b, f.witness(t, true)); err != nil {
			t.Fatalf("COLD-BOX FALSE STALL: an honest Condition-B window block (everMature latched, epochSet\n"+
				"  still empty) must AGREE with apply(). Got: %v", err)
		}
	})
}

func assertForgedMatureEpochStalls(t *testing.T, err error, forged bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("COLD-BOX WRONG-ACCEPT: a forged MatureEpoch.OldValue=%v was not stalled.\n"+
			"  The Direction-A anchor must Resolve it PRESENT against prevStateRoot UNCONDITIONALLY,\n"+
			"  before the class-A branch decision — a branch predicate the fold never verifies on its own.", forged)
	}
	if !errors.Is(err, ErrRecomputeStateRootMaturity) {
		t.Fatalf("expected the Direction-A pre-state anchor stall (ErrRecomputeStateRootMaturity), got %v", err)
	}
	t.Logf("GATE GREEN: forged MatureEpoch.OldValue=%v STALLS at the Direction-A anchor: %v", forged, err)
}

// --- D1 GATE 4 (DIRECTION 2): a LIVE FOLLOWER re-auditing a PRE-handoff block must AGREE. ---
//
// The mirror of the cold case. A follower that already adopted past the handoff has matureEpoch=true
// while the block under audit predates it: apply() screened that block's atts under the bonded rule,
// the box on its live field would screen them under the (then-empty) epochSet and OMIT the
// validatorsSeen write ⇒ mismatch. Anchoring on the committed pre-value fixes both directions at
// once, so this drives a WARM box that is AHEAD of the block, not a cold one.
func TestColdBox_D1_Direction2_LiveFollowerPreHandoffBlockAgrees(t *testing.T) {
	f := buildHandoffFixture(t)
	b := f.handoffBoundaryBlock()
	committed := f.committedRoot(t, b)
	w := f.witnessForHandoff(t, b)

	// Precondition: the committed pre-state is PRE-handoff (matureEpoch=false at prevStateRoot), so the
	// oracle screens this block's atts under the PRE-maturity rule (rotate-LAST: the att loop runs
	// before rotateEpoch flips matureEpoch).
	if decodeBoolLeaf(w.Maturity.MatureEpoch.OldValue) {
		t.Fatalf("GATE VACUOUS: the handoff fixture's pre-state already has matureEpoch=true")
	}

	// The auditor: a box that has ALREADY adopted past the handoff. Simulate the adopt() effect
	// directly (adopt copies everMature/matureEpoch) — this is the ONLY place a test may set these
	// fields, and it is set to the WRONG value for this block on purpose.
	ahead := coldBox(t, f.c.cfg)
	ahead.everMature = true
	ahead.matureEpoch = true
	if !ahead.handedOff() {
		t.Fatalf("GATE VACUOUS: the follower box must report handedOff()=true")
	}

	if err := ahead.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("DIRECTION-2 FALSE STALL: a follower ahead of the handoff stalled re-auditing a\n"+
			"  PRE-handoff block. apply() screens the att loop BEFORE rotateEpoch (rotate-LAST), so the\n"+
			"  box must screen against the COMMITTED pre-value, not its own advanced latch. Got: %v", err)
	}
}

// --- The wiring assertion (R-VERIFYBOND-WIRING): an unwired box stalls LOUD at the entry. ---
//
// The #572 replay shape: objective()/epochsEnabled() depend on the INJECTED verifyBond. An unwired
// box silently takes the legacy branch everywhere and fails later as an opaque fold mismatch. The
// entry names the cause instead.
func TestColdBox_UnwiredBondVerifierStallsAtEntry(t *testing.T) {
	f := buildMidEpochJoiner(t)
	unwired := New(f.cfg, func(ports.NodeID) int64 { return 0 }) // NO SetBondVerifier — the #572 shape
	err := unwired.RecomputeStateRootEntriesRevocations(f.prevRoot, f.honestRoot(t), f.b, f.witness(t, false))
	if !errors.Is(err, ErrRecomputeBoxWiring) {
		t.Fatalf("expected the LOUD entry wiring stall (ErrRecomputeBoxWiring), got %v", err)
	}
}

// --- The defensive cross-check: matureEpoch ⇒ everMature, from the COMMITTED pre-state. ---
//
// The invariant is emergent from rotateEpoch (it early-returns unless everMature) and pinned live by
// TestMatureEpochImpliesEverMature_InvariantPin. A witnessed pre-state that violates it is not one
// any honest apply() can commit, so the box refuses to screen against it. Stall-adding only.
func TestColdBox_MatureEpochWithoutEverMaturePreStateStalls(t *testing.T) {
	f := buildMidEpochJoiner(t)
	w := f.witness(t, false)
	w.Maturity.EverMature.OldValue = statehash.EncodeBool(false)
	w.Maturity.MatureEpoch.OldValue = statehash.EncodeBool(true)
	err := coldRecompute(t, f.cfg, f.prevRoot, f.honestRoot(t), f.b, w)
	if !errors.Is(err, ErrRecomputeStateRootMaturity) {
		t.Fatalf("expected a class-M maturity stall for an impossible pre-state, got %v", err)
	}
}
