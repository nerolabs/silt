package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// R1.5 — the floor-box RESOLVE-path SCHEDULING ORACLE (model-check tier, test-only).
//
// SEAT: Builder — 2026-09-02. Toward Boulder 1's accept-flip (#657 / R1.8).
// Refs:
//   R1.4 cert: floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md
//   design:    docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md
//
// WHY THIS EXISTS. The consensus model-check has ZERO coverage of the floor-box Resolve
// path because WitnessValidateV5 short-circuits at (IndeterminateTrustlessly,
// ErrRecomputeGated) BEFORE the recompute (floorbox_v5.go:244 — the never-Accept STOP
// boundary certified in R1.4-Q5). This oracle exercises the recompute DIRECTLY, calling
// RecomputeStateRootEntriesRevocations (the box's Resolve function) and bypassing the
// never-Accept gate. It runs PRE-FLIP: it tests the recompute itself, not the wired box.
//
// "ACCEPT" in this oracle = the recompute returns nil (it AGREES the committed root is the
// one a full node would compute). A "stall" = a non-nil return (the box refuses to agree).
// The accept-flip (R1.8) would make a nil return terminal; today it does not. So the
// invariants below are stated over the recompute VERDICT, the precondition the flip needs.
//
// TWO ADDITIONS (task R1.5):
//   1. Box-as-I1-participant scheduling oracle (TestScheduleOracle_I1_DisjointBoxesNoConflictingAccept,
//      TestScheduleOracle_I5_HonestNeverSlashed). An adversarial scheduler delivers honest
//      witnesses to some boxes and forged witnesses (via adversarialCommittedRoot) to others,
//      under adversarial delivery order / partition. Asserts I1 (no two disjoint boxes emit
//      Accept for conflicting blocks at one height) and I5 (honest is never slashed).
//   2. Multi-block Resolve schedule (TestScheduleOracle_MultiBlockResolveStableUnderReorder,
//      TestScheduleOracle_ForgedWitnessDoesNotPoisonNextPrevRoot). Consecutive epoch-boundary
//      blocks under adversarially-ordered witness delivery. Asserts each box's Resolve verdict
//      is stable under reorder (Resolve is pure over prevStateRoot + block), and a forged
//      witness at height h does NOT poison prevStateRoot for h+1's Resolve (I3-adjacent).
//
// COMPOUND-SHAPE BREAKS this oracle DRIVES — now CLOSED (classP-anchoring cert 2026-09-02):
//   (a) class-P activation-lock LockedIn.OldValue unanchored wrong-accept. A forged
//       GateLockedIn.OldValue=true SUPPRESSED the activation tally; the suppressed tally emitted no
//       lock-in op, the forged OldValue was never fold-checked, and the box wrong-accepted a block
//       that OMITTED a mandatory lock-in. FIXED by DIRECTION A (rotateTallyOps anchors each lock-in
//       bool against prevStateRoot before the branch read). Driven by
//       TestScheduleOracle_OpenBreak_A_ForgedLockInOldValueSuppression — now asserts the STALL.
//   (b) RegVersion in-block cross-check gap. apply()'s rotate tally reads the JUST-WRITTEN
//       regVersion of an in-block bond (chain.go:3444); the box anchored regVersion against PRE-state
//       only (absent for a fresh in-block bond → RegVersionKnown=false → excluded → false-stall).
//       FIXED by DIRECTION B (regVerWrites → anchorRotateMember in-block cross-check). Driven by
//       TestScheduleOracle_OpenBreak_B_InBlockRegVersionTallyDivergence — now asserts AGREE-on-honest
//       + STALL-on-suppressed.
//
// A blind Tester verifies the oracle BITES: each gate asserts the box now STALLS the forgery (and,
// for (b), AGREES on the honest in-block boundary). The box STILL never Accepts (WitnessValidateV5 →
// Gated); these are recompute-verdict gates, stall-adding only.

// =============================================================================
// Shared fixture builders (self-contained; no dependency on the R1.6 branch files)
// =============================================================================

// unfiredLockInChain builds a mature-from-genesis, v5-admissible chain whose gate(>=3) and
// era3(>=4) activation lock-in tallies are UNFIRED at genesis (members at regVersion 2), and
// then resizes those members to regVersion 4 at h=1 so the tallies WOULD fire at the h=2
// boundary. Era4ActivationHeight=1 makes v5 admissible independent of lock-in; the gate/era3
// tallies stay LIVE (guarded by *ActivationHeight==0). Head is left at h=2 (the next block is
// a boundary). Returns the chain and the proposer key.
func unfiredLockInChain(t *testing.T) (*Chain, ed25519.PrivateKey) {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0, Era4ActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(77001)
	v2 := key(77002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 2, 1),
		bondRegFull(v2, ports.HashBytes(pubOf(v2)), 4<<20, ports.Hash{}, 2, 2),
	)
	Sign(g, prop)
	c.apply(*g)

	prev, h := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(51)}}
	b1.BondRegs = append(b1.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 4, 1),
		bondRegFull(v2, ports.HashBytes(pubOf(v2)), 4<<20, prev, 4, 2),
	)
	Sign(&b1, prop)
	c.apply(b1)

	if c.gateLockedIn || c.era3LockedIn {
		t.Fatalf("fixture: the gate/era3 tallies must be UNFIRED before the test boundary")
	}
	return c, prop
}

// proverFor builds an SMT prover over c's committed pre-state and returns (prover, prevRoot).
func proverFor(t *testing.T, c *Chain) (*statehash.Prover, ports.Hash) {
	t.Helper()
	prover, err := statehash.NewProver(c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch: prover=%x stateroot=%x", prevRoot, sr)
	}
	return prover, prevRoot
}

// committedRoot dry-runs apply(b) on a clone of c and returns the post-apply committed root —
// the root a full node would accept (the honest committed root).
func committedRoot(t *testing.T, c *Chain, b Block) ports.Hash {
	t.Helper()
	clone := c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("committedRoot: %v", err)
	}
	return sr
}

// boundaryWitnessFor builds a full boundary witness for block b against chain c's committed
// pre-state (via prover). Standalone version of rotateFixture.witnessForBoundary usable on any
// mature-from-genesis chain (everMature already latched). Handles a boundary+bond-reg compound
// block by porting rotateFixture.addBondRegWitness's class-B witness pieces.
func boundaryWitnessFor(t *testing.T, c *Chain, prover *statehash.Prover, b Block) StateRootWitness {
	t.Helper()
	preValue := func(k []byte) []byte {
		for _, lf := range c.stateRootLeavesV5() {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		wit, err := prover.Prove(k)
		if err != nil {
			t.Fatalf("Prove(%x): %v", k, err)
		}
		return StateRootChangedLeafWitness{Key: k, OldValue: preValue(k), Proof: wit}
	}
	digestWit := func(tag string, ids []ports.NodeID) StateRootDigestWitness {
		wit, err := prover.Prove(statehash.Key(tag, nil))
		if err != nil {
			t.Fatalf("Prove(%s): %v", tag, err)
		}
		return StateRootDigestWitness{Tag: tag, PreIDs: ids, Proof: wit}
	}
	scalarWit := func(tag string) StateRootRotateScalar {
		k := statehash.Key(tag, nil)
		wit, err := prover.Prove(k)
		if err != nil {
			t.Fatalf("Prove(%s): %v", tag, err)
		}
		return StateRootRotateScalar{OldValue: preValue(k), Proof: wit}
	}
	idsOf := func(m map[ports.NodeID]int64) []ports.NodeID {
		out := make([]ports.NodeID, 0, len(m))
		for id := range m {
			out = append(out, id)
		}
		return sortIDs(out)
	}
	slashedIDs := func() []ports.NodeID {
		out := make([]ports.NodeID, 0, len(c.slashed))
		for id := range c.slashed {
			out = append(out, id)
		}
		return sortIDs(out)
	}
	epochIDs := func() []ports.NodeID {
		out := make([]ports.NodeID, 0, len(c.epochSet))
		for id := range c.epochSet {
			out = append(out, id)
		}
		return sortIDs(out)
	}

	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.DigestPreSets = append(w.DigestPreSets,
		digestWit(tagQualifiedRoot, idsOf(c.qualified)),
		digestWit(tagEpochSetRoot, epochIDs()),
	)

	if len(b.BondRegs) > 0 {
		w.DigestPreSets = append(w.DigestPreSets,
			digestWit(tagBondedRoot, idsOf(c.bonded)),
			digestWit(tagSlashedRoot, slashedIDs()),
		)
		for _, r := range b.BondRegs {
			owner, claimed := c.bondRootOwner[r.Root]
			w.BondRegScreens = append(w.BondRegScreens, StateRootBondRegScreen{
				Root: r.Root, PriorOwner: owner, Claimed: claimed, PriorProven: c.bondRootProven[r.Root],
				OwnerProof:  mustProve(prover, statehash.Key(tagBondRootOwner, r.Root[:])),
				ProvenProof: mustProve(prover, statehash.Key(tagBondRootProven, r.Root[:])),
			})
		}
		if c.cfg.BondTTLBlocks > 0 {
			due := b.Height + c.cfg.BondTTLBlocks + 1
			var hk [8]byte
			putUint64BE(hk[:], due)
			bp, err := prover.Prove(statehash.Key(tagDueBucket, hk[:]))
			if err != nil {
				t.Fatalf("Prove(bucket): %v", err)
			}
			w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{DueHeight: due, PreMembers: nil, Proof: bp})
		}
		tmp := StateRootWitness{
			DigestPreSets: []StateRootDigestWitness{
				digestWit(tagBondedRoot, idsOf(c.bonded)),
				digestWit(tagQualifiedRoot, idsOf(c.qualified)),
				digestWit(tagSlashedRoot, slashedIDs()),
			},
			BondRegScreens: w.BondRegScreens,
			BondRegBuckets: w.BondRegBuckets,
		}
		_, bWrites, err := c.bondRegOps(prover.Root(), b, tmp)
		if err != nil {
			t.Fatalf("bondRegOps (witness build): %v", err)
		}
		for _, wr := range bWrites {
			w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
		}
	}

	clone := c.cloneForDryRun()
	clone.apply(b)

	var rw StateRootRotateWitness
	for id, wt := range clone.qualified {
		esKey := statehash.Key(tagEpochSet, id[:])
		esProof, err := prover.Prove(esKey)
		if err != nil {
			t.Fatalf("Prove(epochSet %x): %v", id[:], err)
		}
		// An in-block bonded member carries its POST-write regVersion (from the applied clone) — the
		// DIRECTION B cross-check input; a steady-state member's post == pre so the RegVersionProof
		// still resolves.
		rv, ok := clone.regVersion[id]
		rw.Members = append(rw.Members, StateRootRotateMember{
			ID: id, Weight: wt, RegVersion: rv, RegVersionKnown: ok,
			EpochSetProof: esProof, EpochSetOldValue: preValue(esKey),
			QualifiedProof:  mustProve(prover, statehash.Key(tagQualified, id[:])),
			RegVersionProof: mustProve(prover, statehash.Key(tagRegVersion, id[:])),
		})
	}
	for id := range c.epochSet {
		if _, still := clone.qualified[id]; still {
			continue
		}
		esKey := statehash.Key(tagEpochSet, id[:])
		wit, sibs, err := prover.ProveWithSiblings(esKey)
		if err != nil {
			t.Fatalf("ProveWithSiblings(drop): %v", err)
		}
		rw.PriorEpochSet = append(rw.PriorEpochSet, StateRootRotateMember{
			ID: id, EpochSetOldValue: preValue(esKey), EpochSetProof: wit, EpochSetDeleteSiblings: sibs,
		})
	}
	rw.EpochStart = scalarWit(tagEpochStart)
	rw.MatureEpoch = scalarWit(tagMatureEpoch)
	rw.GateLockedIn = scalarWit(tagGateLockedIn)
	rw.GateHeight = scalarWit(tagGateHeight)
	rw.Era3LockedIn = scalarWit(tagEra3LockedIn)
	rw.Era3Height = scalarWit(tagEra3Height)
	rw.Era4LockedIn = scalarWit(tagEra4LockedIn)
	rw.Era4Height = scalarWit(tagEra4Height)
	w.Rotate = &rw
	w.Maturity = &StateRootMaturityWitness{EverMature: scalarWit(tagEverMature)}

	if c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, err := prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		w.DueBucketProof = dp
	}
	return w
}

// suppressedLockInRoot returns the committed root of a block that OMITTED the gate + era3
// activation lock-ins (the root an attacker commits when it forges the box into skipping the
// tally). It is the honest post-apply state with the lock-in scalars UNDONE.
func suppressedLockInRoot(t *testing.T, c *Chain, b Block) ports.Hash {
	t.Helper()
	clone := c.cloneForDryRun()
	clone.apply(b)
	clone.gateLockedIn = false
	clone.gateHeight = 0
	clone.era3LockedIn = false
	clone.era3Height = 0
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("suppressedLockInRoot: %v", err)
	}
	return sr
}
