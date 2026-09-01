package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the P1-d class-B bond-registration state-root recompute
// (floorbox_recompute_stateroot_bondreg_v5.go).
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//     (B CERTIFIED-in-direction; carries the R-B-displacement residual — a derivation-correctness/
//      liveness burden, fold-caught, never a wrong-accept).
//
// R3 (execution-derived drift guard, MANDATORY): the box's derived B write-set + digest + due-bucket
// reconstruction is checked against the REAL apply() + StateRootForVersion(5) over a FRESH reg, a
// RENEW (same id, resize+version bump, moving the due-bucket), and a DISPLACEMENT (proof beats a
// genesis squatter). Ablated RED on a mis-derived delta (skip the displacement) and a forged screen.
// Ground truth is real execution (the session-7 scar).

// bondFixture is a v5 chain with a proposer + a genesis squatter on a shared root, advanced to h=0,
// with prevStateRoot + a Prover captured at genesis.
type bondFixture struct {
	c          *Chain
	prevRoot   ports.Hash
	prover     *statehash.Prover
	proposer   ed25519.PrivateKey
	squatter   ed25519.PrivateKey
	sharedRoot ports.Hash
}

func buildBondFixture(t *testing.T) bondFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(81001)
	squatter := key(81002)
	shared := ports.HashBytes([]byte("shared-plot-81"))
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(squatter, shared, 4<<20, ports.Hash{}, 5, 2), // genesis-declared (unproven) on shared root
	)
	Sign(g, prop)
	c.apply(*g)

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("fixture pre-root mismatch: prover=%x chain=%x", prevRoot, sr)
	}
	return bondFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, squatter: squatter, sharedRoot: shared}
}

func (f bondFixture) preIDsBonded() []ports.NodeID    { return sortIDs(mapIDs(f.c.bonded)) }
func (f bondFixture) preIDsQualified() []ports.NodeID { return sortIDs(mapIDs(f.c.qualified)) }
func (f bondFixture) preIDsSlashed() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.slashed))
	for id := range f.c.slashed {
		out = append(out, id)
	}
	return sortIDs(out)
}

func (f bondFixture) digestWitness(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	wit, err := f.prover.Prove(statehash.Key(tag, nil))
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

func (f bondFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f bondFixture) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
	t.Helper()
	old := f.preValue(wr.key)
	if wr.newValue == nil {
		wit, sibs, err := f.prover.ProveWithSiblings(wr.key)
		if err != nil {
			t.Fatalf("ProveWithSiblings(%x): %v", wr.key, err)
		}
		return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit, DeleteSiblings: sibs}
	}
	wit, err := f.prover.Prove(wr.key)
	if err != nil {
		t.Fatalf("Prove(%x): %v", wr.key, err)
	}
	return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit}
}

func (f bondFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// bondScreen builds the per-root ownership screen from the fixture's committed pre-state.
func (f bondFixture) bondScreen(root ports.Hash) StateRootBondRegScreen {
	owner, claimed := f.c.bondRootOwner[root]
	return StateRootBondRegScreen{
		Root:        root,
		PriorOwner:  owner,
		Claimed:     claimed,
		PriorProven: f.c.bondRootProven[root],
		OwnerProof:  mustProve(f.prover, statehash.Key(tagBondRootOwner, root[:])),
		ProvenProof: mustProve(f.prover, statehash.Key(tagBondRootProven, root[:])),
	}
}

// dueHeightOf returns the due-height for an id currently registered at bondRegHeight[id]. ok=false
// if the id is not registered pre-state.
func (f bondFixture) preDueHeight(id ports.NodeID) (uint64, bool) {
	h, ok := f.c.bondRegHeight[id]
	if !ok {
		return 0, false
	}
	return h + f.c.cfg.BondTTLBlocks + 1, true
}

// bucketWitness proves an affected due-bucket against prevStateRoot and packs its pre-members. If the
// bucket is absent pre-state (a fresh insert), it issues a non-membership proof with empty members.
func (f bondFixture) bucketWitness(t *testing.T, dueHeight uint64) StateRootBucketWitness {
	t.Helper()
	var hk [8]byte
	putUint64BE(hk[:], dueHeight)
	key := statehash.Key(tagDueBucket, hk[:])
	pre := []ports.NodeID{}
	for id := range f.c.dueBucket[dueHeight] {
		pre = append(pre, id)
	}
	pre = sortIDs(pre)
	if len(pre) == 0 {
		// absent bucket → non-membership (ADD)
		wit, err := f.prover.Prove(key)
		if err != nil {
			t.Fatalf("Prove(bucket %d): %v", dueHeight, err)
		}
		return StateRootBucketWitness{DueHeight: dueHeight, PreMembers: nil, Proof: wit}
	}
	// present bucket → membership (+ delete siblings, in case it empties)
	wit, sibs, err := f.prover.ProveWithSiblings(key)
	if err != nil {
		t.Fatalf("ProveWithSiblings(bucket %d): %v", dueHeight, err)
	}
	return StateRootBucketWitness{DueHeight: dueHeight, PreMembers: pre, Proof: wit, DeleteSiblings: sibs}
}

// bondWitness builds the full B witness for a block: the derived per-member proofs, the touched
// digest pre-sets, the per-root screens, the affected due-bucket witnesses, and the E/R + dueBucket
// scope-gate proof. It derives the write-set the same way the box does so the witness set matches.
func (f bondFixture) bondWitness(t *testing.T, b Block, affectedBuckets []uint64) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// Digest pre-sets (always supply all three touched-candidate sets).
	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
	}

	// Per-root screens for each reg root.
	for _, r := range b.BondRegs {
		w.BondRegScreens = append(w.BondRegScreens, f.bondScreen(r.Root))
	}

	// Affected due-bucket witnesses.
	for _, d := range affectedBuckets {
		w.BondRegBuckets = append(w.BondRegBuckets, f.bucketWitness(t, d))
	}

	// The per-member write-set: derive it the box's way so the witness key set matches.
	preBonded := idSet(f.preIDsBonded())
	preQualified := idSet(f.preIDsQualified())
	preSlashed := idSet(f.preIDsSlashed())
	screens := map[ports.Hash]StateRootBondRegScreen{}
	for _, r := range b.BondRegs {
		screens[r.Root] = f.bondScreen(r.Root)
	}
	preBRH := map[ports.NodeID]uint64{}
	for id, h := range f.c.bondRegHeight {
		preBRH[id] = h
	}
	delta, err := f.c.stateRootBondRegWriteSet(f.prevRoot, b, preBonded, preQualified, preSlashed, screens, preBRH)
	if err != nil {
		t.Fatalf("stateRootBondRegWriteSet: %v", err)
	}
	for _, wr := range delta.writes {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// dueBucket TTL scope-gate proof at b.Height (non-membership — a bond-reg block is not a sweep).
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dk := statehash.Key(tagDueBucket, hk[:])
		dp, err := f.prover.Prove(dk)
		if err != nil {
			t.Fatalf("Prove(dueBucket scope): %v", err)
		}
		w.DueBucketProof = dp
	}
	// Class M: mature-from-genesis fixture ⇒ everMature already latched (pre=true), no crossing.
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	return w
}

// --- Ablation 1: FRESH bond reg AGREES with real apply(). ---
func TestRecomputeStateRootBondRegFreshAgreesWithApply(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	fresh := key(81009)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
	fid := ports.HashBytes(pubOf(fresh))
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	_ = fid
	committed := f.applyAndCommittedRoot(t, b)
	w := f.bondWitness(t, b, []uint64{newDue})

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("FRESH B recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Ablation 2: RENEW (same id, resize+version bump) AGREES with real apply(). The renew moves the
// proposer's due-bucket (old delete + new insert) and CHANGEs bonded/regVersion but NOT the bonded
// or qualified id-SET (so no whole-set digest is touched). ---
func TestRecomputeStateRootBondRegRenewAgreesWithApply(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	pid := ports.HashBytes(pubOf(f.proposer))
	oldDue, ok := f.preDueHeight(pid)
	if !ok {
		t.Fatalf("fixture: proposer not registered pre-state")
	}
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(f.proposer, ports.HashBytes(pubOf(f.proposer)), 16<<20, prev, 6, 1)}}
	committed := f.applyAndCommittedRoot(t, b)
	w := f.bondWitness(t, b, uniqueU64(oldDue, newDue))

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("RENEW B recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Ablation 3: DISPLACEMENT — honest PROVES the genesis-squatted root, displacing the squatter.
// The delta must strip the squatter from bonded+qualified (an id NOT in the payload) AND add the
// honest owner. AGREES with real apply(). ---
func TestRecomputeStateRootBondRegDisplacementAgreesWithApply(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(81003)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
	// Confirm displacement fires in real apply().
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sqid := ports.HashBytes(pubOf(f.squatter))
	if _, still := clone.bonded[sqid]; still {
		t.Fatalf("fixture: displacement did NOT fire — squatter still bonded")
	}
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	committed := f.applyAndCommittedRoot(t, b)
	w := f.bondWitness(t, b, []uint64{newDue})

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("DISPLACEMENT B recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Ablation 4: mis-derived delta — a committed root reflecting the DISPLACEMENT NOT applied (the
// squatter still bonded). We forge a StateRoot where the squatter kept its bonded standing, hand the
// box an HONEST witness. The box derives the CORRECT displacement (strips the squatter), folds the
// honest bondedRoot, which MISMATCHES the buggy committed root ⇒ ErrRecomputeStateRootMismatch. This
// drives the REAL recompute and proves the displacement branch is load-bearing. ---
func TestRecomputeStateRootBondRegAblationDisplacementNotApplied(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(81003)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
	sqid := ports.HashBytes(pubOf(f.squatter))

	buggyClone := f.c.cloneForDryRun()
	preBond := buggyClone.bonded[sqid]
	buggyClone.apply(b)
	if _, still := buggyClone.bonded[sqid]; still {
		t.Fatalf("fixture: apply() did NOT displace — ablation vacuous")
	}
	buggyClone.bonded[sqid] = preBond  // undo the displacement bonded delete
	buggyClone.qualifiedMaintain(sqid) // and its qualified consequence
	buggyCommitted, err := buggyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("buggy StateRootForVersion: %v", err)
	}

	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, buggyCommitted, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a committed root reflecting displacement-not-applied must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected ErrRecomputeStateRootMismatch, got %v", err)
	}
}

// --- Ablation 5: forged screen — claim the shared root is UNCLAIMED pre-state. The box then does NOT
// run the displacement branch, so it neither strips the squatter nor writes the new owner over the
// old ⇒ its recomputed root diverges from the honest committed root ⇒ stall. ---
func TestRecomputeStateRootBondRegAblationForgedScreen(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(81003)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
	committed := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})

	// Forge the screen: claim the shared root is unclaimed (no displacement).
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].Claimed = false
			w.BondRegScreens[i].PriorOwner = ports.NodeID{}
			w.BondRegScreens[i].PriorProven = false
		}
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged (unclaimed) screen must stall, got nil")
	}
	// R1.2: the forged Claimed=false requires a NON-MEMBERSHIP proof of bondRootOwner||sharedRoot, but
	// the honest OwnerProof proves it PRESENT (the squatter owns it), so the class-B anchor stalls
	// (ErrRecomputeStateRootDigest) — a stronger, earlier catch than the pre-R1.2 fold/mismatch.
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected an anchor/fold/mismatch stall, got %v", err)
	}
}

// --- Ablation 6: a bond-reg block at an epoch boundary. Class P is now IN scope (P1-e), so the block
// DISPATCHES to the rotate reconstruction. With an EMPTY witness (no rotate witness, no digest pre-sets)
// the box cannot reconstruct the boundary ⇒ it stalls (never-Accept preserved). ---
func TestRecomputeStateRootBondRegAblationBoundaryOutOfScope(t *testing.T) {
	// A fixture with a small EpochBlocks so a boundary is reachable, and epochs enabled.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(82001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)
	// advance to h=3 so the next block h=4 is a boundary
	for h := uint64(1); h <= 3; h++ {
		prev, _ := c.Head()
		c.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 5, 1)}})
	}
	leaves := c.stateRootLeavesV5()
	prover, _ := statehash.NewProver(leaves)
	prevRoot := prover.Root()

	prev, _ := c.Head()
	fresh := key(82009)
	b := Block{Version: BlockVersionWitnessable, Height: 4, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
	committed, _ := func() (ports.Hash, error) {
		cl := c.cloneForDryRun()
		cl.apply(b)
		return cl.StateRootForVersion(BlockVersionWitnessable)
	}()

	var w StateRootWitness
	err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a boundary bond-reg block with an empty witness must stall, got nil")
	}
	// The block dispatches (B/P in scope); the empty witness fails an anchor — either the TTL
	// scope-gate non-membership proof (dueBucket), the digest anchor, or the fold. All never-Accept.
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootFold) &&
		!errors.Is(err, ErrRecomputeStateRootTTLWitness) {
		t.Fatalf("ABLATION FAILED: expected a ttl/digest/fold stall for an unwitnessed boundary block, got %v", err)
	}
}

// --- 7e: class-B BELOW-MinBond fresh reg (bonded, NOT qualified). ---
//
// A fresh bond reg with MinBondBytes <= Size < MinBond passes the objective anti-release floor
// (chain.go:3232, so it writes bonded/bondRegHeight/regVersion/bondDomain and moves the TTL
// due-bucket) but does NOT enter qualified (chain.go:3264 qualifiedMaintain: size < MinBond ⇒ not
// qualified). This path was unexercised: every prior B fixture registers at or above MinBond, so the
// "bonded-but-not-qualified" branch of the delta (postBonded grows, postQual unchanged, no
// qualifiedRoot digest touched, no qualified||id leaf) had no test. The below-floor fixture sets
// MinBondBytes < MinBond so a mid-range size lands in the gap.

// buildBelowMinBondFixture is buildBondFixture with a distinct anti-release floor below MinBond, so a
// fresh reg can be bonded without qualifying. It seats only a proposer (no shared-root squatter — the
// below-MinBond path is about the qualification gate, not displacement).
func buildBelowMinBondFixture(t *testing.T) bondFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: 4 << 20, MinBondBytes: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(83001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1)) // proposer qualifies
	Sign(g, prop)
	c.apply(*g)

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("fixture pre-root mismatch: prover=%x chain=%x", prevRoot, sr)
	}
	return bondFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop}
}

// TestRecomputeStateRootBondRegBelowMinBondAgreesWithApply is the POSITIVE 7e test: a fresh reg at a
// size in [MinBondBytes, MinBond) is bonded-but-not-qualified, and the recompute reproduces the
// committed StateRoot byte-exact vs the real apply() + StateRootForVersion(5). It confirms the delta
// grows bonded (touching bondedRoot) WITHOUT touching qualifiedRoot or writing qualified||id.
func TestRecomputeStateRootBondRegBelowMinBondAgreesWithApply(t *testing.T) {
	f := buildBelowMinBondFixture(t)
	prev, h := f.c.Head()
	fresh := key(83009)
	fid := ports.HashBytes(pubOf(fresh))
	size := int64(2 << 20) // MinBondBytes(1MiB) <= 2MiB < MinBond(4MiB)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), size, prev, 5, 9)}}

	// Confirm the fixture actually exercises the bonded-but-not-qualified path in real apply().
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, bonded := clone.bonded[fid]; !bonded {
		t.Fatalf("fixture: below-floor reg not bonded — ablation vacuous")
	}
	if _, qual := clone.qualified[fid]; qual {
		t.Fatalf("fixture: below-MinBond reg WRONGLY qualified — fixture size not in the gap")
	}

	newDue := h + f.c.cfg.BondTTLBlocks + 1
	committed := f.applyAndCommittedRoot(t, b)
	w := f.bondWitness(t, b, []uint64{newDue})

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("below-MinBond B recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// TestRecomputeStateRootBondRegBelowMinBondAblationForgedQualification is the 7e ablation: forge a
// committed StateRoot in which the below-MinBond id WAS wrongly added to qualified. The box derives
// the CORRECT delta (bonded grows, qualified unchanged), folds an honest qualifiedRoot that still
// EXCLUDES the id, and that MISMATCHES the forged committed root ⇒ stall. This drives the REAL
// recompute and proves the qualification gate is load-bearing (a below-floor id can never be smuggled
// into qualified). Red-before-green: the honest committed root (no forged qualification) AGREES
// (the positive test above).
func TestRecomputeStateRootBondRegBelowMinBondAblationForgedQualification(t *testing.T) {
	f := buildBelowMinBondFixture(t)
	prev, h := f.c.Head()
	fresh := key(83009)
	fid := ports.HashBytes(pubOf(fresh))
	size := int64(2 << 20)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), size, prev, 5, 9)}}

	// Forge: apply honestly, then WRONGLY add the below-floor id to qualified (the smuggle) and
	// recompute the committed StateRoot over that buggy post-state.
	buggyClone := f.c.cloneForDryRun()
	buggyClone.apply(b)
	if _, bonded := buggyClone.bonded[fid]; !bonded {
		t.Fatalf("fixture: below-floor reg not bonded — ablation vacuous")
	}
	if _, qual := buggyClone.qualified[fid]; qual {
		t.Fatalf("fixture: below-MinBond reg already qualified — nothing to forge")
	}
	buggyClone.qualified[fid] = size // FORGE the qualification
	buggyCommitted, err := buggyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("buggy StateRootForVersion: %v", err)
	}

	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, buggyCommitted, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a committed root that forged qualification for a below-MinBond id must stall, got nil")
	}
	// The box's honest qualifiedRoot excludes the id; the forged committed root includes it. Depending
	// on whether the box supplies a qualifiedRoot digest op for the (honestly-unchanged) set, the
	// stall surfaces as a fold or a final-equality mismatch — both never-Accept.
	if !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) &&
		!errors.Is(err, ErrRecomputeStateRootDigest) {
		t.Fatalf("ABLATION FAILED: expected a fold/mismatch/digest stall, got %v", err)
	}
}

// uniqueU64 returns the distinct values in order.
func uniqueU64(vs ...uint64) []uint64 {
	seen := map[uint64]struct{}{}
	out := []uint64{}
	for _, v := range vs {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
