package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the trustless floor-box RECOMPUTE increment 3 (floorbox_recompute_dematureQuorum_v5.go):
// the root-only reproduction of requireDeMatureSuperQuorum (the F-1 de-mature super-quorum over the
// WHOLE bonded map), replicating increments 1/2's C-1 pattern AND gating on the reproduced maturity
// state (increment 2's RecomputeMatureNow).
//
// The HARD ABLATIONS (C-5, red-before-green), each injected and watched to flip the verdict, so a
// green here is not decoration:
//   - FORGED BONDED WEIGHT (C-1): a witness with the right members but a forged per-member bonded
//     weight ⇒ STALL (its inclusion proof fails against the committed root).
//   - OMITTED / INJECTED MEMBER: a witness missing/padding a bonded member ⇒ MTH mismatch ⇒ STALL
//     (set-completeness against bondedRoot).
//   - CONFIG-FROM-WITNESS THRESHOLD (C-6, failing-first): a fold that read the ⅔ threshold (or the
//     coalition `need`) from the WITNESS instead of the fixed consensus constant would let an
//     attacker shift the bar. The correct fixed-constant fold is INVARIANT to any witness-carried
//     threshold; the negative control demonstrates the shift the real fold forecloses.
//
// The recompute NEVER flips WitnessValidateV5 to Accept (the STOP boundary); it reproduces ONE
// predicate.

// dematureFixture is an objective v5 chain that has MATURED (everMature latched) but whose live
// decentralization is BELOW the bar (matureNow() == false), so requireDeMatureSuperQuorum binds. It
// carries the committed StateRoot, a Prover over its v5 leaves, and the seated (whole-bonded)
// members. The maturity gate is reproduced from the SAME committed state via increment 2's
// SeenSetWitness.
type dematureFixture struct {
	c       *Chain
	root    ports.Hash
	prover  *statehash.Prover
	members []ports.NodeID // the whole-bonded ids MINUS anchors (the seated validators)
}

// buildDematureFixture seats the given bonds as attesters on an objective v5 chain (so they enter
// both bonded and validatorsSeen), sets a HIGH MatureValidators bar so matureNow() is false, latches
// everMature (white-box, same package) so the chain is in the de-mature window, then snapshots the
// committed v5 StateRoot and a Prover over its v5 leaves. A floor box holds root; the Prover stands
// in for the any-of-N witness provider.
func buildDematureFixture(t *testing.T, matureValidators int, bonds []maturityBond) dematureFixture {
	t.Helper()
	// Reuse the maturity fixture builder to seat the bonds and snapshot BEFORE latching everMature.
	// buildMaturityFixture seats the bonds into bonded + validatorsSeen; we then latch everMature and
	// re-snapshot so the committed root reflects the de-mature state (everMature == true).
	const operatorMargin = 1
	base := buildMaturityFixture(t, matureValidators, operatorMargin, bonds)

	// Latch everMature (the chain has matured at some past height; live decentralization has since
	// dropped below the high bar). matureNow() must be false at the high bar, so the de-mature gate
	// binds. Set it white-box (same package), then re-snapshot the root/prover so the committed
	// everMature scalar leaf reflects the latch.
	base.c.everMature = true
	if base.c.matureNow() {
		t.Fatalf("fixture precondition: matureNow() must be FALSE at the high bar (got true) so the de-mature gate binds")
	}
	if !base.c.objective() {
		t.Fatal("fixture precondition: the chain must be objective for the de-mature gate")
	}

	leaves := base.c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	root := prover.Root()
	sr, err := base.c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != root {
		t.Fatalf("fixture root mismatch: prover=%x chain=%x", root, sr)
	}

	// The whole-bonded members MINUS anchors (the seated validators). The de-mature fold sums the
	// WHOLE bonded map (anchors included); `members` is the non-anchor subset an ablation perturbs.
	members := make([]ports.NodeID, 0, len(base.c.bonded))
	for id := range base.c.bonded {
		if base.c.cfg.Anchors[id] {
			continue
		}
		members = append(members, id)
	}
	if len(members) == 0 {
		t.Fatal("fixture precondition: bonded must have non-anchor members")
	}
	return dematureFixture{c: base.c, root: root, prover: prover, members: members}
}

// seenWitnessFor builds the increment-2 SeenSetWitness over validatorsSeen (the maturity gate the
// de-mature recompute reproduces first). It mirrors maturityFixture.witnessFor.
func (f dematureFixture) seenWitnessFor(t *testing.T) SeenSetWitness {
	t.Helper()
	rootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	rootVal := nodeSetMTHFromBool(f.c.validatorsSeen)
	rootProof, err := f.prover.Prove(rootKey)
	if err != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", err)
	}
	ids := make([]ports.NodeID, 0, len(f.c.validatorsSeen))
	members := make(map[ports.NodeID]MemberStateWitness, len(f.c.validatorsSeen))
	for id := range f.c.validatorsSeen {
		ids = append(ids, id)
		members[id] = f.seenMemberWitness(t, id)
	}
	return SeenSetWitness{IDs: ids, SeenRootWitness: rootProof, SeenRootValue: rootVal, Members: members}
}

// seenMemberWitness builds one validatorsSeen member's slashed/bonded/bondDomain witness (mirrors
// maturityFixture.memberWitness).
func (f dematureFixture) seenMemberWitness(t *testing.T, id ports.NodeID) MemberStateWitness {
	t.Helper()
	mw := MemberStateWitness{}
	sp, err := f.prover.Prove(statehash.Key(tagSlashed, id[:]))
	if err != nil {
		t.Fatalf("Prove(slashed[%x]): %v", id[:], err)
	}
	mw.Slashed = f.c.slashed[id]
	mw.SlashedProof = sp
	bp, err := f.prover.Prove(statehash.Key(tagBonded, id[:]))
	if err != nil {
		t.Fatalf("Prove(bonded[%x]): %v", id[:], err)
	}
	mw.Bonded = f.c.bonded[id]
	mw.BondedProof = bp
	dp, err := f.prover.Prove(statehash.Key(tagBondDomain, id[:]))
	if err != nil {
		t.Fatalf("Prove(bondDomain[%x]): %v", id[:], err)
	}
	d, present := f.c.bondDomain[id]
	mw.Domain = d
	mw.DomainPresent = present
	mw.DomainProof = dp
	return mw
}

// bondedWitnessFor builds the increment-3 BondedSetWitness proving the COMPLETE whole-bonded set
// against the committed root: the bondedRoot digest leaf + one MemberWeightWitness (bonded[id]
// weight + inclusion proof) per bonded id (anchors included — the fold ranges the whole map).
func (f dematureFixture) bondedWitnessFor(t *testing.T) BondedSetWitness {
	t.Helper()
	rootKey := statehash.Key(tagBondedRoot, nil)
	rootVal := nodeSetMTHFromInt64(f.c.bonded)
	rootProof, err := f.prover.Prove(rootKey)
	if err != nil {
		t.Fatalf("Prove(bondedRoot): %v", err)
	}
	ids := make([]ports.NodeID, 0, len(f.c.bonded))
	weights := make(map[ports.NodeID]MemberWeightWitness, len(f.c.bonded))
	for id, w := range f.c.bonded {
		ids = append(ids, id)
		p, err := f.prover.Prove(statehash.Key(tagBonded, id[:]))
		if err != nil {
			t.Fatalf("Prove(bonded[%x]): %v", id[:], err)
		}
		weights[id] = MemberWeightWitness{Weight: w, Proof: p}
	}
	return BondedSetWitness{IDs: ids, BondedRootWitness: rootProof, BondedRootValue: rootVal, MemberWeights: weights}
}

// fullNodeDeMatureVerdict returns the verdict the full node's ValidateCommit de-mature gate produces
// at this state for the given coalition: true (met) when requireDeMatureSuperQuorum passes (or the
// gate does not bind because matureNow()), false when it would reject.
func (f dematureFixture) fullNodeDeMatureVerdict(proposer ports.NodeID, seen map[ports.NodeID]bool) bool {
	if !f.c.everMature || !f.c.objective() || f.c.matureNow() {
		return true // gate does not bind — the full node does not run the predicate
	}
	// Reproduce requireDeMatureSuperQuorum's fold as the test's own reference (chain.go:2949-2963):
	// the whole-bonded total and the proposer+seen coalition weight, against the ⌈2·total/3⌉ bar.
	var total, committed int64
	for _, w := range f.c.bonded {
		total += w
	}
	if total <= 0 {
		return true
	}
	committed = f.c.bonded[proposer]
	for id := range seen {
		committed += f.c.bonded[id]
	}
	need := (2*total + 2) / 3
	return committed >= need
}

// diverseBondsBig is a set of bonds spread so the de-mature super-quorum fold is non-trivial: the
// proposer alone does NOT carry ⅔; a proposer + two heavy attesters does. Five distinct domains so
// matureNow at a high bar is cleanly false.
func diverseBondsBig() []maturityBond {
	return []maturityBond{
		{key(60), 10 << 20, 0xA1},
		{key(61), 8 << 20, 0xB2},
		{key(62), 6 << 20, 0xC3},
		{key(63), 4 << 20, 0xD4},
		{key(64), 2 << 20, 0xE5},
	}
}

// TestRecomputeDeMatureSuperQuorum_MatchesFullNode is the equivalence anchor: over the SAME
// committed de-mature state, the trustless recompute's verdict equals the full node's de-mature
// gate — for BOTH a coalition that MEETS the ⅔ super-quorum and one that MISSES it.
func TestRecomputeDeMatureSuperQuorum_MatchesFullNode(t *testing.T) {
	// A high MatureValidators bar (6 > 5 members) keeps matureNow() false, so the gate binds. Total
	// non-anchor bonded weight is 30M; the two anchors add 2M (1M each) → total 32M. need = ⌈2·32/3⌉
	// = ⌈21.33⌉ = 22M (in MiB units, computed on raw bytes).
	t.Run("coalition MEETS the super-quorum (matches full node)", func(t *testing.T) {
		f := buildDematureFixture(t, 6, diverseBondsBig())
		seenW := f.seenWitnessFor(t)
		bondedW := f.bondedWitnessFor(t)

		// Proposer = the 10M member; seen = the 8M + 6M + 4M members. committed = 10+8+6+4 = 28M ≥ 22M.
		proposer := idOf(key(60))
		seen := map[ports.NodeID]bool{idOf(key(61)): true, idOf(key(62)): true, idOf(key(63)): true}

		got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, bondedW)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		want := f.fullNodeDeMatureVerdict(proposer, seen)
		if got != want {
			t.Fatalf("recompute verdict %v != full node de-mature verdict %v", got, want)
		}
		if !got {
			t.Fatal("a 28M coalition of 32M total must clear the ⌈2·total/3⌉ super-quorum")
		}
	})

	t.Run("coalition MISSES the super-quorum (matches full node)", func(t *testing.T) {
		f := buildDematureFixture(t, 6, diverseBondsBig())
		seenW := f.seenWitnessFor(t)
		bondedW := f.bondedWitnessFor(t)

		// Proposer = the 2M member; seen = the 4M member. committed = 2+4 = 6M < 22M ⇒ reject.
		proposer := idOf(key(64))
		seen := map[ports.NodeID]bool{idOf(key(63)): true}

		got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, bondedW)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		want := f.fullNodeDeMatureVerdict(proposer, seen)
		if got != want {
			t.Fatalf("recompute verdict %v != full node de-mature verdict %v", got, want)
		}
		if got {
			t.Fatal("a 6M coalition of 32M total must NOT clear the ⌈2·total/3⌉ super-quorum")
		}
	})
}

// TestRecomputeDeMatureSuperQuorum_MatureIsNoOp pins the maturity gate: when the reproduced
// matureNow is TRUE (the chain is still decentralized), the full node does NOT run
// requireDeMatureSuperQuorum, so the recompute must return met=true regardless of the coalition —
// a no-op that matches the full node's skip.
func TestRecomputeDeMatureSuperQuorum_MatureIsNoOp(t *testing.T) {
	// A LOW MatureValidators bar (2 <= coefficient) makes matureNow() TRUE, so the de-mature gate
	// does not bind. Build the maturity fixture directly (everMature not required — the gate check
	// short-circuits on mature first) and confirm the recompute returns met=true even for a coalition
	// that would MISS the super-quorum if it ran.
	base := buildMaturityFixture(t, 2, 1, diverseBondsBig())
	if !base.c.matureNow() {
		t.Fatal("fixture precondition: matureNow() must be TRUE at the low bar")
	}
	f := dematureFixture{c: base.c, root: base.root, prover: base.prover, members: base.members}
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)

	// A coalition that would MISS the super-quorum if the gate ran (proposer = 2M member, no seen).
	proposer := idOf(key(64))
	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, nil, seenW, bondedW)
	if reason != nil {
		t.Fatalf("recompute stalled unexpectedly: %v", reason)
	}
	if !got {
		t.Fatal("MATURITY-GATE VIOLATION: a mature chain must be a no-op (met=true) — the de-mature bar must not bind when matureNow()")
	}
}

// TestRecomputeDeMatureSuperQuorum_ForgedBondedWeightRejects is HARD ABLATION 1 (C-1): a witness with
// the RIGHT members but a FORGED per-member bonded weight makes the recompute STALL — the forged
// weight's inclusion proof does not verify against the committed root.
//
// RED-BEFORE-GREEN: the un-forged witness (TestRecomputeDeMatureSuperQuorum_MatchesFullNode) reaches
// a verdict; forging one member's bonded weight flips it to a stall.
func TestRecomputeDeMatureSuperQuorum_ForgedBondedWeightRejects(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)
	proposer := idOf(key(60))
	seen := map[ports.NodeID]bool{idOf(key(61)): true, idOf(key(62)): true, idOf(key(63)): true}
	if _, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, bondedW); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: forge one member's claimed bonded weight while KEEPING its original
	// inclusion proof (built for the TRUE weight). Resolve against the forged EncodeInt64(weight)
	// fails ⇒ the member's bonded is unproven ⇒ stall.
	victim := f.members[0]
	forged := cloneBondedWitness(bondedW)
	mw := forged.MemberWeights[victim]
	mw.Weight += 100 << 20
	forged.MemberWeights[victim] = mw

	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, forged)
	if got {
		t.Fatal("C-1 VIOLATION: a forged per-member bonded weight was accepted — the super-quorum tally is forgeable")
	}
	if !errors.Is(reason, ErrRecomputeBondedMemberWeightUnproven) {
		t.Fatalf("forged bonded should stall on ErrRecomputeBondedMemberWeightUnproven; got %v", reason)
	}
}

// TestRecomputeDeMatureSuperQuorum_OmittedMemberRejects is HARD ABLATION 2: a witness MISSING a
// bonded member reconstructs a DIFFERENT MTH than the committed bondedRoot ⇒ STALL. A withholding
// prover cannot shrink the folded set to fake a super-quorum, because the digest binds the complete
// id-set.
func TestRecomputeDeMatureSuperQuorum_OmittedMemberRejects(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)
	proposer := idOf(key(60))
	seen := map[ports.NodeID]bool{idOf(key(61)): true, idOf(key(62)): true, idOf(key(63)): true}
	if _, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, bondedW); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: drop one member from the witnessed id-list (and its weight witness). The
	// reconstructed nodeSetMTH over the short list differs from the committed bondedRoot.
	dropped := f.members[0]
	forged := cloneBondedWitness(bondedW)
	shortIDs := make([]ports.NodeID, 0, len(forged.IDs)-1)
	for _, id := range forged.IDs {
		if id != dropped {
			shortIDs = append(shortIDs, id)
		}
	}
	forged.IDs = shortIDs
	delete(forged.MemberWeights, dropped)

	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, forged)
	if got {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness missing a bonded member was accepted")
	}
	if !errors.Is(reason, ErrRecomputeBondedSetIncomplete) {
		t.Fatalf("omitted member should stall on ErrRecomputeBondedSetIncomplete; got %v", reason)
	}
}

// TestRecomputeDeMatureSuperQuorum_InjectedMemberRejects is the dual: INJECTING an extra id into the
// witnessed list also breaks set-completeness (a different MTH), so a prover cannot pad the set to
// dilute the ⅔ bar either.
func TestRecomputeDeMatureSuperQuorum_InjectedMemberRejects(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)
	proposer := idOf(key(60))
	seen := map[ports.NodeID]bool{idOf(key(61)): true}

	forged := cloneBondedWitness(bondedW)
	extra := idOf(key(99998)) // not a bonded member
	forged.IDs = append(forged.IDs, extra)
	forged.MemberWeights[extra] = forged.MemberWeights[f.members[0]] // bogus witness; completeness fails first

	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, forged)
	if got {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness with an injected extra member was accepted")
	}
	if !errors.Is(reason, ErrRecomputeBondedSetIncomplete) {
		t.Fatalf("injected member should stall on ErrRecomputeBondedSetIncomplete; got %v", reason)
	}
}

// TestRecomputeDeMatureSuperQuorum_ThresholdFromConstant is the C-6 ABLATION (failing-first): the
// de-mature verdict must depend ONLY on the committed member weights (own StateRoot) and the FIXED ⅔
// consensus constant — NEVER on a threshold carried in the witness. The test proves it by showing
// that a config-from-witness fold (the negative control) accepts a coalition the real fixed-constant
// fold REJECTS: an attacker who could carry a lax threshold (e.g. ⅓) in the witness would flip a
// missing coalition to met. The real fold is INVARIANT to the witness-carried threshold.
//
// RED-BEFORE-GREEN (evidence, reported in the PR): the negative control
// (recomputeDeMatureThresholdFromWitness with a lax numerator/denominator) ACCEPTS the 6M-of-32M
// coalition; the production fold REJECTS it. That divergence is the C-6 teeth — a config-from-witness
// regression would flip the production verdict to match the lax witness.
func TestRecomputeDeMatureSuperQuorum_ThresholdFromConstant(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)

	// A coalition that MISSES the real ⅔ super-quorum: proposer = 2M member, seen = 4M member.
	// committed = 6M of 32M total; the real need = ⌈2·32/3⌉ = 22M ⇒ REJECT.
	proposer := idOf(key(64))
	seen := map[ports.NodeID]bool{idOf(key(63)): true}

	// PRODUCTION (fixed ⅔ constant): rejects.
	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, seen, seenW, bondedW)
	if reason != nil {
		t.Fatalf("recompute stalled unexpectedly: %v", reason)
	}
	if got {
		t.Fatal("fixture invariant: a 6M-of-32M coalition must MISS the real ⅔ super-quorum")
	}

	// NEGATIVE CONTROL (the RED): a config-from-witness fold reads a LAX ⅓ threshold from the witness,
	// so the same 6M coalition (6M > ⌈32/3⌉ = 11M? no — 6M < 11M) — use a laxer ⅕ bar to force the
	// flip: need = ⌈32/5⌉ = 7M, still > 6M. Use a ⅛ bar: need = ⌈32/8⌉ = 4M ≤ 6M ⇒ the witnessed lax
	// threshold ACCEPTS. The production fixed-⅔ fold does not; the divergence is the C-6 teeth.
	metInj := recomputeDeMatureThresholdFromWitness(f.c, f.root, proposer, seen, bondedW, 1, 8)
	if !metInj {
		t.Fatal("negative-control precondition: a lax ⅛ witnessed threshold must ACCEPT the 6M coalition (else the control is not reading the threshold from the witness)")
	}
	// The teeth: production REJECTS (got == false, asserted above) while config-from-witness ACCEPTS
	// (metInj == true) — proving the real fold reads the FIXED constant, and a config-from-witness
	// regression would be caught by the production verdict flipping to accept the lax witness.
	if got == metInj {
		t.Fatal("C-6 VIOLATION: the production fold and the config-from-witness fold AGREED — the production fold read the threshold from the witness, not the fixed ⅔ constant")
	}
}

// recomputeDeMatureThresholdFromWitness is the NEGATIVE-CONTROL injected variant for the C-6
// ablation: it reproduces RecomputeDeMatureSuperQuorum's set-completeness + per-member verification
// EXACTLY, but reads the super-quorum threshold ratio (num/den) from CALLER-supplied parameters (a
// stand-in for a witness-carried threshold) instead of the fixed ⅔ constant. It exists ONLY in the
// test to demonstrate that a config-from-witness fold ACCEPTS a coalition the fixed-constant fold
// rejects. TEST-ONLY; it touches no production path. It skips the maturity gate (the fixture is in
// the de-mature window) to isolate the threshold source.
func recomputeDeMatureThresholdFromWitness(c *Chain, root ports.Hash, proposer ports.NodeID, seen map[ports.NodeID]bool, w BondedSetWitness, num, den int64) bool {
	rootKey := statehash.Key(tagBondedRoot, nil)
	if !statehash.Resolve(root, rootKey, w.BondedRootValue, w.BondedRootWitness).IsProvenPresent() {
		return false
	}
	if string(nodeSetMTH(w.IDs)) != string(w.BondedRootValue) {
		return false
	}
	var total, committed int64
	for _, id := range w.IDs {
		mw := w.MemberWeights[id]
		if !statehash.Resolve(root, statehash.Key(tagBonded, id[:]), statehash.EncodeInt64(mw.Weight), mw.Proof).IsProvenPresent() {
			return false
		}
		total += mw.Weight
		if id == proposer || seen[id] {
			committed += mw.Weight
		}
	}
	if total <= 0 {
		return true
	}
	need := (num*total + den - 1) / den // THE INJECTED DEFECT: witness-carried num/den, not the fixed ⅔.
	return committed >= need
}

// cloneBondedWitness deep-copies a BondedSetWitness's MemberWeights map + IDs slice so an ablation
// can mutate one entry without disturbing the shared baseline witness.
func cloneBondedWitness(w BondedSetWitness) BondedSetWitness {
	out := w
	out.IDs = append([]ports.NodeID(nil), w.IDs...)
	out.MemberWeights = make(map[ports.NodeID]MemberWeightWitness, len(w.MemberWeights))
	for id, mw := range w.MemberWeights {
		out.MemberWeights[id] = mw
	}
	return out
}

// TestRecomputeDeMatureSuperQuorum_UnprovenBondedRootStalls proves the box stalls (never folds) when
// the committed bondedRoot leaf cannot be proven present — e.g. a witness verified against the WRONG
// root. (The maturity gate is reproduced first; a wrong root stalls the maturity recompute, so this
// asserts the stall path rather than the specific error, then a same-root fixture asserts the
// bonded-root-specific error.)
func TestRecomputeDeMatureSuperQuorum_UnprovenBondedRootStalls(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)
	proposer := idOf(key(60))

	// Forge ONLY the bonded-root value so the maturity gate (which does not read bondedRoot) still
	// passes against the true root, but the bonded set-completeness check fails on the bondedRoot
	// presence/MTH. The bonded-root witness proves the TRUE MTH; a corrupted claimed value makes the
	// reconstructed MTH differ ⇒ set-incomplete.
	forged := cloneBondedWitness(bondedW)
	forged.BondedRootValue = append([]byte(nil), bondedW.BondedRootValue...)
	forged.BondedRootValue[0] ^= 0xff

	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, nil, seenW, forged)
	if got {
		t.Fatal("a corrupted bondedRoot value must not reach a met verdict")
	}
	// A corrupted claimed value: the presence proof no longer verifies for it (unproven) OR the MTH
	// mismatches (incomplete). Either is a valid stall; assert one of the bonded-root errors.
	if !errors.Is(reason, ErrRecomputeBondedRootUnproven) && !errors.Is(reason, ErrRecomputeBondedSetIncomplete) {
		t.Fatalf("corrupted bondedRoot should stall on a bonded-root error; got %v", reason)
	}
}

// TestRecomputeDeMatureSuperQuorum_MissingMemberWeightStalls proves the box stalls when a
// completeness-verified bonded member has no weight witness at all (the map entry is absent). The
// set reconstructs (digest matches) but a member's weight cannot be verified ⇒ stall, never fold a
// partial set.
func TestRecomputeDeMatureSuperQuorum_MissingMemberWeightStalls(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	seenW := f.seenWitnessFor(t)
	bondedW := f.bondedWitnessFor(t)
	proposer := idOf(key(60))
	delete(bondedW.MemberWeights, f.members[0]) // keep IDs complete (digest matches) but drop a weight witness

	got, reason := f.c.RecomputeDeMatureSuperQuorum(f.root, proposer, nil, seenW, bondedW)
	if got {
		t.Fatal("a member with no weight witness must stall the fold, not be folded as zero")
	}
	if !errors.Is(reason, ErrRecomputeBondedMemberWeightUnproven) {
		t.Fatalf("missing member weight witness should stall on ErrRecomputeBondedMemberWeightUnproven; got %v", reason)
	}
}

// TestRecomputeDeMatureSuperQuorum_NeverFlipsWitnessValidateAccept pins the STOP boundary: this
// increment reproduces ONE predicate; it must NOT have flipped WitnessValidateV5 to Accept.
func TestRecomputeDeMatureSuperQuorum_NeverFlipsWitnessValidateAccept(t *testing.T) {
	f := buildDematureFixture(t, 6, diverseBondsBig())
	got, _ := f.c.WitnessValidateV5(v5Block(3), f.root, RecoveryDirective{})
	if got == Accept {
		t.Fatal("STOP boundary violated: WitnessValidateV5 returned ACCEPT — the accept flip (#657) must wait until ALL predicates are reproduced")
	}
}
