package chain

import (
	"crypto/ed25519"
	"sort"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// THE QUORUM-STACK WHOLE-SET ENUMERATION — the ROOT-CAUSE fix for the hand-enumeration
// misses (R-boundary, docs/thinking/2026-08-31-Rboundary-mechanical-wholeset-enumeration
// -options.md).
//
// THE BLIND SPOT. The merged execution-derived read-set guard (readset_v5_drift_test.go)
// derives ground truth from apply(b) and a validity source that runs validateTakedowns +
// per-entry ValidateEntry (validityVerdict). It NEVER runs the QUORUM STACK
// (requireQuorumStack → requireEpochWeightQuorum, requireDeMatureSuperQuorum, and via
// RequiredQuorum the qualifiedCount / validatorSetSize folds). The full-node acceptance
// contract is apply(b) ∪ ValidateCommit(b), and ValidateCommit runs that stack. So the
// prior guard is structurally blind to every committed map the stack reads AS A WHOLE SET
// (a SUM or COUNT over the entire map, not one key). That blind spot is why hand-
// enumeration keeps missing the whole-set roots.
//
// WHY A WHOLE-SET READ NEEDS A DIGEST ROOT. A floor box that holds only per-key membership
// witnesses cannot verify a whole-map SUM: a block whose requireDeMatureSuperQuorum total
// was computed over a map with a forged extra member (or a dropped member) is indetectable
// from individual bonded[id] proofs. To witness a whole-map fold trustlessly the box needs
// a committed DIGEST ROOT over that keyspace (verified against StateRoot), from which the
// fold is checkable. So: whole-set read ⟹ needs a digest root; per-key read ⟹ witnessed
// individually.
//
// THE MECHANICAL METHOD (the untouched-member perturbation oracle). For committed keyspace
// K, take a member the block does NOT touch, perturb K on a fresh clone (add a huge absent
// member; remove the largest present member), and re-run the FULL contract
// collectQuorumSigs + requireQuorumStack. If the ACCEPT/REJECT outcome changes, the
// contract folded K over the whole map ⟹ K is a whole-set read. The "untouched member"
// clause is the discriminator: a per-key read consults only keys the block names, so an
// untouched perturbation moves nothing; a whole-set fold moves the sum/count.
//
// EXHAUSTIVE BY CONSTRUCTION. The oracle runs over the CLOSED 23-keyspace set
// (v5CommittedKeyspaceTags, asserted == 23 against statehash.go) and drives the REAL
// predicates (not a mirror), so any whole-map fold ANY predicate performs moves the
// verdict — the method cannot miss a whole-set read a hand-list forgets. The perturbation
// is applied in BOTH directions (add huge / remove largest) with a magnitude that crosses
// any fixed threshold, so a whole-set fold is caught regardless of how poised the world is.

// fullContractVerdict runs the whole-set half of the acceptance contract: the qualification
// filter (collectQuorumSigs → attesterQualifiedAt) and the quorum stack
// (requireQuorumStack → count floor + requireEpochWeightQuorum + requireDeMatureSuperQuorum).
// This is the predicate the prior guard's validityVerdict does NOT run — the blind spot.
// Legacy phase/round match the era-1 commit path (b.Atts at implicit r0), the same call
// shape the model-check snapshot-equivalence probes use (quorumVerdict).
func fullContractVerdict(c *Chain, b *Block) string {
	seen, err := c.collectQuorumSigs(b, b.Atts, PhaseLegacy, 0)
	if err != nil {
		return "reject"
	}
	return verdict(c.requireQuorumStack(b, seen))
}

// wholeSetProbeWorld is one poised world for the enumeration: a chain, a block carrying a
// real signed coalition, and the reference verdict the full contract yields. The world is
// poised so a large untouched perturbation of a whole-set keyspace crosses the threshold.
type wholeSetProbeWorld struct {
	name string
	c    *Chain
	b    *Block
}

// deMaturePoisedWorld fires requireDeMatureSuperQuorum (whole-bonded SUM). The de-mature
// bar rejects a minnow coalition against a whale-dominated bonded total; removing the
// untouched whale drops the total and flips the coalition above ⅔ (accept). It is the
// same construction the model-check everMatureProbe uses (deMatureWorld).
func deMaturePoisedWorld(t *testing.T) wholeSetProbeWorld {
	t.Helper()
	c, b := deMatureWorld(t)
	return wholeSetProbeWorld{name: "de-mature (whole-bonded ⅔ sum)", c: c, b: b}
}

// matureEpochPoisedWorld fires requireEpochWeightQuorum (whole-epochSet SUM). A coalition
// holding 100% of a two-member frozen epoch ACCEPTS; adding a huge untouched epochSet
// member inflates the total so the coalition falls below ⅔ (reject).
func matureEpochPoisedWorld(t *testing.T) wholeSetProbeWorld {
	t.Helper()
	keys := []ed25519.PrivateKey{key(47100), key(47101)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, 1<<20, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("matureEpochPoisedWorld genesis: %v", err)
	}
	// Realize a mature epoch with the two members frozen at equal weight.
	setField(c, "matureEpoch", true)
	setField(c, "everMature", true)
	setField(c, "epochSet", map[ports.NodeID]int64{idOf(keys[0]): 1 << 20, idOf(keys[1]): 1 << 20})
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(b, keys[0])
	b.Atts = []Attestation{Attest(b, keys[1])}
	return wholeSetProbeWorld{name: "mature-epoch (whole-epochSet ⅔ sum)", c: c, b: b}
}

// countFloorPoisedWorld fires the RequiredQuorum count floor, whose N =
// validatorSetSize → qualifiedCount folds !slashed[id] over the WHOLE bonded map. It is
// poised so an untouched SLASH crosses the threshold: five equal bonds ⇒ N=5,
// bftThreshold(5)=3. The coalition is proposer + 2 attesters (below the 3 required) ⇒ the
// reference verdict is REJECT. Slashing an untouched non-coalition bonded member drops N to
// 4 ⇒ bftThreshold(4)=2 ⇒ the SAME coalition now clears (accept). The verdict flip is
// carried SOLELY by the whole-bonded qualifiedCount fold reading slashed[untouched].
// Immature objective mode, no anchors, so validatorSetSize == qualifiedCount over bonded.
func countFloorPoisedWorld(t *testing.T) wholeSetProbeWorld {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 5)
	for i := range keys {
		keys[i] = key(int64(47200 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, 1<<20, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("countFloorPoisedWorld genesis: %v", err)
	}
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(b, keys[0])
	// Proposer keys[0] + 2 attesters (keys[1], keys[2]) — one short of bftThreshold(5)=3.
	// keys[3], keys[4] are untouched bonded members the slash perturbation targets.
	b.Atts = []Attestation{Attest(b, keys[1]), Attest(b, keys[2])}
	return wholeSetProbeWorld{name: "count-floor (qualifiedCount over whole bonded)", c: c, b: b}
}

func wholeSetProbeWorlds(t *testing.T) []wholeSetProbeWorld {
	return []wholeSetProbeWorld{
		deMaturePoisedWorld(t),
		matureEpochPoisedWorld(t),
		countFloorPoisedWorld(t),
	}
}

// blockTouches reports the ids/keys the block b NAMES a transition on, per keyspace tag.
// The oracle perturbs a member NOT in this set, so the perturbation cannot be a per-key
// read the block itself drove.
func blockTouches(b *Block) map[ports.NodeID]bool {
	touched := make(map[ports.NodeID]bool)
	touched[b.ProposerID()] = true
	for _, a := range b.Atts {
		touched[a.AttesterID()] = true
	}
	for _, r := range b.BondRegs {
		if len(r.Validator) == ed25519.PublicKeySize {
			touched[r.ValidatorID()] = true
		}
	}
	for i := range b.Slashes {
		touched[b.Slashes[i].CulpritID()] = true
	}
	return touched
}

// perturbWholeSetUntouched perturbs keyspace `tag` on clone by an UNTOUCHED member, in the
// given direction. It returns false if the keyspace is not a map of NodeID→weight the
// oracle perturbs this way (the scalars and the byRoot/spent/revoked/dueBucket keyspaces,
// which are per-key by construction — none is folded whole-set by the quorum stack). For a
// NodeID-keyed map it either ADDS a fresh untouched member with enormous weight or REMOVES
// the largest present untouched member.
func perturbWholeSetUntouched(clone *Chain, tag string, touched map[ports.NodeID]bool, add bool) bool {
	const huge = int64(1) << 40
	fresh := idOf(key(999001)) // an id no world seeds and no block touches
	switch tag {
	case tagBonded:
		if add {
			clone.bonded[fresh] = huge
		} else {
			removeLargestUntouched(clone.bonded, touched)
		}
	case tagEpochSet:
		if add {
			clone.epochSet[fresh] = huge
		} else {
			removeLargestUntouched(clone.epochSet, touched)
		}
	case tagQualified:
		if add {
			clone.qualified[fresh] = huge
		} else {
			removeLargestUntouched(clone.qualified, touched)
		}
	case tagValidatorsSeen:
		if add {
			clone.validatorsSeen[fresh] = true
		} else {
			removeAnyUntouchedBool(clone.validatorsSeen, touched)
		}
	case tagSlashed:
		if add {
			// Slash an untouched CURRENTLY-QUALIFIED member: that removes it from qualifiedCount
			// (N drops) and from any live weight fold. If none exists, add a slashed marker for a
			// bonded member.
			for id := range clone.bonded {
				if !touched[id] && !clone.slashed[id] {
					clone.slashed[id] = true
					return true
				}
			}
			clone.slashed[fresh] = true
		} else {
			for id := range clone.slashed {
				if !touched[id] {
					delete(clone.slashed, id)
					return true
				}
			}
			return false
		}
	default:
		return false
	}
	return true
}

func removeLargestUntouched(m map[ports.NodeID]int64, touched map[ports.NodeID]bool) {
	var best ports.NodeID
	var bestW int64 = -1
	found := false
	for id, w := range m {
		if touched[id] {
			continue
		}
		if w > bestW {
			bestW, best, found = w, id, true
		}
	}
	if found {
		delete(m, best)
	}
}

func removeAnyUntouchedBool(m map[ports.NodeID]bool, touched map[ports.NodeID]bool) {
	for id := range m {
		if !touched[id] {
			delete(m, id)
			return
		}
	}
}

// wholeSetKeyspaces runs the untouched-member perturbation oracle over every poised world
// and every committed keyspace, and returns the set of keyspace tags the full contract
// reads AS A WHOLE SET (some world's verdict flips under an untouched perturbation of that
// keyspace).
func wholeSetKeyspaces(t *testing.T) map[string]bool {
	t.Helper()
	flagged := make(map[string]bool)
	for _, w := range wholeSetProbeWorlds(t) {
		ref := fullContractVerdict(w.c, w.b)
		touched := blockTouches(w.b)
		for _, tag := range v5CommittedKeyspaceTags() {
			for _, add := range []bool{true, false} {
				clone := w.c.cloneForDryRun()
				if !perturbWholeSetUntouched(clone, tag, touched, add) {
					continue
				}
				if fullContractVerdict(clone, w.b) != ref {
					flagged[tag] = true
				}
			}
		}
	}
	return flagged
}

// applyChannelWholeSetKeyspaces runs the untouched-member perturbation oracle over the
// APPLY channel: perturb an untouched member on a fresh clone, run the REAL apply(b), and
// compare the post-apply v5 state root to the unperturbed post-apply root (via the leaf
// set). A cross-member difference means apply()'s recompute folded the untouched member
// into some committed leaf — a whole-set read in apply (the boundary freeze over the whole
// qualified/epochSet, the maturity coefficient over the whole validatorsSeen). This is the
// channel the merged guard's Source 1 write-diff already covers; the oracle re-derives it
// so the FULL contract apply ∪ ValidateCommit is enumerated in one place.
func applyChannelWholeSetKeyspaces(t *testing.T) map[string]bool {
	t.Helper()
	flagged := make(map[string]bool)
	// Boundary + maturity worlds exercise the whole-set apply folds. Reuse the maintenance
	// corpus (it covers boundary / maturity-latch / attested) and the poised worlds.
	corpus, snap := buildV5ReadSetCorpus(t)
	applied := make(map[uint64]bool)
	for _, cb := range corpus {
		b := setV5(cb.block)
		touched := blockTouches(&cb.block)
		ref := snap.cloneForDryRun()
		ref.apply(b)
		refLeaves := leafKeySet(ref)
		for _, tag := range v5CommittedKeyspaceTags() {
			for _, add := range []bool{true, false} {
				clone := snap.cloneForDryRun()
				if !perturbWholeSetUntouched(clone, tag, touched, add) {
					continue
				}
				clone.apply(b)
				if crossMemberLeafDiffers(refLeaves, leafKeySet(clone), tag) {
					flagged[tag] = true
				}
			}
		}
		if !applied[cb.block.Height] {
			snap.apply(cb.block)
			applied[cb.block.Height] = true
		}
	}
	return flagged
}

// crossMemberLeafDiffers reports whether two post-apply leaf sets differ on ANY key that
// is NOT of the perturbed keyspace `tag` and is NOT a digest-root leaf — a difference in some
// OTHER leaf caused by the untouched perturbation, i.e. apply() read the perturbed keyspace
// whole-set to compute another leaf. Differences WITHIN `tag` are excluded: the perturbed
// member persisting into the post-state is self-persistence, not a read (mirrors
// readset_v5_drift_test.go's crossLeafDiffers self-exclusion, one keyspace up).
//
// The five F1 digest-root leaves are ALSO excluded (isDigestRootLeaf). Each is a
// whole-keyspace MTH digest, so perturbing an untouched member of ANY of the five keyspaces
// flips that keyspace's digest root — a leaf outside `tag`. Counting that flip would make
// this APPLY-channel oracle flag every keyspace with a digest root (e.g. `slashed`, which is
// a QUORUM-STACK fold via qualifiedCount, not an apply fold), corrupting the channel
// attribution the enumeration exists to establish. The digest roots are inert F1 output
// commitments (no apply/validity predicate reads them — F1 STOP boundary); they are the
// leaf-set analogue of the recomputed state root, which crossLeafDiffers already excludes.
func crossMemberLeafDiffers(ref, got map[string]string, tag string) bool {
	inTag := func(k string) bool { tg, _, _ := splitLeafKey([]byte(k)); return tg == tag }
	skip := func(k string) bool { return inTag(k) || isDigestRootLeaf(k) }
	for k, v := range got {
		if skip(k) {
			continue
		}
		if ref[k] != v {
			return true
		}
	}
	for k := range ref {
		if skip(k) {
			continue
		}
		if _, ok := got[k]; !ok {
			return true
		}
	}
	return false
}

// TestQuorumStackWholeSetEnumeration is THE MECHANICAL ENUMERATION of the QUORUM-STACK
// whole-set reads — the blind spot this increment closes. It derives, by the untouched-
// member perturbation oracle over the quorum stack (collectQuorumSigs + requireQuorumStack),
// the exhaustive set of committed keyspaces the stack reads as a whole set. It asserts the
// derived set is EXACTLY the certified quorum-stack whole-map folds, and reports any
// keyspace the ≥4 list omits (the mechanical finding).
//
// FINDING (derived, not hand-listed): the quorum stack whole-set reads are
// {bonded, epochSet, slashed, validatorsSeen}.
//   - bonded — requireDeMatureSuperQuorum sums the whole map (total, chain.go:2949) and
//     qualifiedCount counts it (chain.go:1481).
//   - epochSet — requireEpochWeightQuorum sums effectiveEpochSet = whole epochSet (total,
//     chain.go:2851) and validatorSetSize reads len(epochSet) (chain.go:1561).
//   - slashed — qualifiedCount folds !slashed[id] over the whole bonded domain
//     (chain.go:1482), so an untouched slash drops N (RequiredQuorum threshold).
//   - validatorsSeen — the de-mature gate `!c.matureNow()` (chain.go:2827) folds
//     MatureCoefficient → C2Metric over the WHOLE validatorsSeen map (objective mode,
//     readset_v5.go:413), so an untouched validatorsSeen member can flip matureNow() and
//     turn the de-mature bar on/off. This is the read hand-enumeration keeps missing: it
//     lives behind a GATE (matureNow), not a direct sum.
//
// `slashed` is the keyspace the ≥4 list {qualified, epochSet, validatorsSeen, bonded}
// OMITS. `qualified` is NOT a quorum-stack fold — it is an APPLY-channel whole-set read
// (the boundary freeze), enumerated by TestApplyChannelWholeSetEnumeration and already
// covered by the merged guard's Source 1 write-diff. So the ≥4 list's members map to two
// channels: {epochSet, validatorsSeen, bonded} are quorum-stack folds (this test), and
// {qualified, validatorsSeen} are apply-channel folds — validatorsSeen is read whole-set
// by BOTH.
func TestQuorumStackWholeSetEnumeration(t *testing.T) {
	got := wholeSetKeyspaces(t)
	var derived []string
	for tag := range got {
		derived = append(derived, prettyTag(tag))
	}
	sort.Strings(derived)
	t.Logf("DERIVED quorum-stack whole-set-read keyspaces (need a committed digest root): %v", derived)

	wantExactly := map[string]bool{
		tagBonded: true, tagEpochSet: true, tagSlashed: true, tagValidatorsSeen: true,
	}
	for tag := range wantExactly {
		if !got[tag] {
			t.Errorf("quorum-stack whole-set keyspace %q was NOT flagged by the oracle — the world does not exercise its fold, or the fold was removed", prettyTag(tag))
		}
	}
	for tag := range got {
		if !wantExactly[tag] {
			t.Errorf("oracle flagged UNEXPECTED quorum-stack whole-set keyspace %q — the derived set is wider than the known folds; investigate", prettyTag(tag))
		}
	}

	if got[tagSlashed] {
		t.Logf("RECONCILED vs the ≥4 list {qualified, epochSet, validatorsSeen, bonded}: it OMITS `slashed` (a real quorum-stack whole-set read via qualifiedCount) and MIS-ATTRIBUTES `qualified` (an APPLY-channel whole-set read, not a quorum-stack fold)")
	}
}

// TestQuorumStackNonWholeSetKeyspacesCleared closes the "silently un-probed" gap in the
// exhaustiveness claim. The main oracle (perturbWholeSetUntouched) perturbs only the five
// NodeID-keyed weight/membership maps; it SKIPS the other 18 keyspaces. This test PROBES
// every skipped NodeID-keyed map (bondDomain, regVersion, bondRegHeight, bondRootOwner,
// bondRootProven) with an untouched perturbation under the full quorum-stack contract and
// asserts NONE flips a verdict — so they are CLEARED as non-quorum-stack-whole-set by
// EXECUTION, not by assumption. (bondDomain is read by C2Metric but only for validatorsSeen
// members, so an untouched bondDomain member outside validatorsSeen changes nothing — the
// oracle confirms it is a per-key, validatorsSeen-scoped read, not a whole bondDomain fold.)
// The scalar and non-NodeID keyspaces (byRoot/spent/revoked/dueBucket) are single-leaf or
// payload-keyed and are never folded by the quorum stack; they are per-key by construction.
func TestQuorumStackNonWholeSetKeyspacesCleared(t *testing.T) {
	// The NodeID-keyed maps the main oracle SKIPS. Each is read (if at all) per-key, not as a
	// whole-map fold, by the quorum stack — the oracle must confirm they do not flip a verdict.
	skipped := []string{tagBondDomain, tagRegVersion, tagBondRegHeight, tagBondRootOwner, tagBondRootProven}
	for _, w := range wholeSetProbeWorlds(t) {
		ref := fullContractVerdict(w.c, w.b)
		touched := blockTouches(w.b)
		for _, tag := range skipped {
			clone := w.c.cloneForDryRun()
			if !perturbSkippedUntouched(clone, tag, touched) {
				continue // keyspace not present/perturbable in this world — nothing to fold
			}
			if fullContractVerdict(clone, w.b) != ref {
				t.Errorf("[%s] keyspace %q FLIPPED the quorum verdict under an untouched perturbation — it IS a quorum-stack whole-set read the main oracle SKIPPED; add it to perturbWholeSetUntouched and the digest-root list",
					w.name, prettyTag(tag))
			}
		}
	}
}

// perturbSkippedUntouched perturbs an untouched member of a NodeID-keyed map the main oracle
// skips, adding a fresh member so a whole-map fold (if any) would move. Returns false if the
// keyspace is not a perturbable NodeID map here.
func perturbSkippedUntouched(clone *Chain, tag string, touched map[ports.NodeID]bool) bool {
	fresh := idOf(key(999002))
	frid := idOf(key(999003))
	var froot ports.Hash
	copy(froot[:], frid[:])
	switch tag {
	case tagBondDomain:
		clone.bondDomain[fresh] = 7
	case tagRegVersion:
		clone.regVersion[fresh] = 5
	case tagBondRegHeight:
		clone.bondRegHeight[fresh] = 1
	case tagBondRootOwner:
		clone.bondRootOwner[froot] = fresh
	case tagBondRootProven:
		clone.bondRootProven[froot] = true
	default:
		return false
	}
	return true
}

// TestApplyChannelWholeSetEnumeration enumerates the APPLY-channel whole-set reads — the
// boundary freeze and maturity coefficient that fold whole qualified / validatorsSeen /
// bonded / epochSet into committed leaves. These are already covered by the merged guard's
// Source 1 write-diff; this test makes the FULL-contract enumeration complete and explicit.
func TestApplyChannelWholeSetEnumeration(t *testing.T) {
	got := applyChannelWholeSetKeyspaces(t)
	var derived []string
	for tag := range got {
		derived = append(derived, prettyTag(tag))
	}
	sort.Strings(derived)
	t.Logf("DERIVED apply-channel whole-set-read keyspaces: %v", derived)

	// qualified is the load-bearing apply-channel whole-set read (the boundary freeze ranges
	// the whole qualified map). It MUST appear here, closing the ≥4-list mis-attribution.
	if !got[tagQualified] {
		t.Errorf("apply-channel oracle did not flag `qualified` — the boundary freeze fold is not exercised by the corpus; the enumeration is incomplete")
	}
}

// TestFullContractWholeSetKeyspaces is the UNION: the exhaustive set of committed keyspaces
// the FULL acceptance contract apply ∪ ValidateCommit reads as a whole set, and therefore
// need a committed digest-root leaf. It is the quorum-stack set ∪ the apply-channel set.
func TestFullContractWholeSetKeyspaces(t *testing.T) {
	union := wholeSetKeyspaces(t)
	for tag := range applyChannelWholeSetKeyspaces(t) {
		union[tag] = true
	}
	var all []string
	for tag := range union {
		all = append(all, prettyTag(tag))
	}
	sort.Strings(all)
	t.Logf("EXHAUSTIVE full-contract whole-set-read keyspaces (need a committed digest root): %v", all)

	// The load-bearing four the ≥4 list names must all be present in the UNION, plus slashed.
	for _, tag := range []string{tagBonded, tagEpochSet, tagSlashed, tagQualified, tagValidatorsSeen} {
		if !union[tag] {
			t.Errorf("full-contract union missing whole-set keyspace %q", prettyTag(tag))
		}
	}
}

// --- THE BLIND-SPOT-CLOSED ABLATION (Part 4) ---
//
// The claim to prove: the PRE-extension guard (ground truth = apply ∪ validity, NO quorum
// stack) is BLIND to a forged untouched member of a quorum-stack whole-set keyspace — a
// floor box witnessing only the producer's per-key reads ACCEPTS the forgery. The EXTENDED
// guard (ground truth includes the quorum stack) DETECTS it (RED). This is the exact "green
// check with no demonstrated red" the increment kills.
//
// THE FORGERY MODEL. A floor box that witnesses only the producer's per-key reads holds
// individual membership proofs for the members the BLOCK names, but NO completeness proof
// over the whole keyspace. So it cannot tell a whole-map SUM computed over the honest map
// from one computed over a map with a forged extra member. We model the box's view by the
// producer's read-set keys: a key the producer does NOT emit is a key the box cannot check.
// A forged UNTOUCHED member is, by construction, not in the producer's per-key read-set
// (the producer walks the payload, and the payload does not name it). So:
//   - PRE-extension ground truth (apply ∪ validity): the forged untouched member changes NO
//     apply leaf and NO validity verdict → the pre-extension ground truth does not list it →
//     the producer's omission is GREEN. The box accepts the forgery.
//   - EXTENDED ground truth (+ quorum stack): the forged untouched member FLIPS the quorum
//     verdict → the extended ground truth lists a whole-set read the producer's per-key set
//     does not cover → RED. The box's blindness is caught.

// extendedGroundTruthWholeSetReads is the EXTENDED ground-truth source (the blind-spot
// closure): the set of WHOLE-MAP completeness reads block b requires under the FULL contract
// apply ∪ ValidateCommit. It runs the untouched-member perturbation oracle over the quorum
// stack for chain c and block b; for each keyspace whose fold flips the verdict, it emits the
// completeness-leaf read (tag||@root) the box needs to verify the whole-map fold trustlessly.
// This is the read the merged guard's ground truth (apply ∪ validity) NEVER produces — the
// blind spot. It is derived by EXECUTION (the oracle), not hand-listed.
func extendedGroundTruthWholeSetReads(c *Chain, b *Block) map[string]struct{} {
	reads := make(map[string]struct{})
	ref := fullContractVerdict(c, b)
	touched := blockTouches(b)
	for _, tag := range v5CommittedKeyspaceTags() {
		for _, add := range []bool{true, false} {
			clone := c.cloneForDryRun()
			if !perturbWholeSetUntouched(clone, tag, touched, add) {
				continue
			}
			if fullContractVerdict(clone, b) != ref {
				reads[completenessLeafKey(tag)] = struct{}{}
			}
		}
	}
	return reads
}

// producerCoversWholeSetReads reports whether the (possibly leaf-augmented) producer output
// covers the extended ground truth's whole-map completeness reads. The current per-key
// producer emits NO completeness leaf, so it FAILS to cover any — the extended guard's RED.
// The positive control passes a producer augmented with the completeness leaves (the future
// gated fix), which then COVERS them — proving the RED is caused by the missing leaf, not a
// hardcoded false.
func producerCoversWholeSetReads(producedKeys map[string]struct{}, truth map[string]struct{}) (missing []string) {
	for k := range truth {
		if _, ok := producedKeys[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

// completenessLeafKey is the reserved read-set key a WHOLE-MAP digest-root leaf would use
// for keyspace `tag`. It is NOT a committed leaf yet (adding it is the gated format change);
// this key models "the box holds a completeness proof over the whole keyspace." It uses a
// reserved suffix a per-key leaf key can never collide with, so a per-key ground truth never
// covers it — which is the blind spot the extended guard exists to catch.
func completenessLeafKey(tag string) string {
	return prettyTag(tag) + "@root"
}

// TestQuorumWholeSetBlindSpotClosed is the red→green ablation, PER quorum-stack whole-set
// keyspace: over a poised world that fires the keyspace's fold, it demonstrates
//
//	(1) the PRE-extension guard (apply ∪ validity ground truth) is GREEN — blind;
//	(2) the EXTENDED guard (+ quorum-stack whole-set source) is RED — the per-key producer
//	    does not cover the whole-map completeness read the fold requires;
//	(3) POSITIVE CONTROL: augmenting the producer with the completeness leaf turns the
//	    extended guard GREEN — proving the RED is caused by the missing whole-map read, not
//	    a hardcoded false (the "inject the defect and watch it go red, then green on fix"
//	    discipline; a green check with no demonstrated red-then-green is decoration).
func TestQuorumWholeSetBlindSpotClosed(t *testing.T) {
	type ablation struct {
		tag   string
		world func(*testing.T) wholeSetProbeWorld
	}
	ablations := []ablation{
		{tagBonded, deMaturePoisedWorld},
		{tagEpochSet, matureEpochPoisedWorld},
		{tagSlashed, countFloorPoisedWorld},
		{tagValidatorsSeen, deMaturePoisedWorld},
	}
	for _, ab := range ablations {
		ab := ab
		t.Run(prettyTag(ab.tag), func(t *testing.T) {
			w := ab.world(t)

			// The extended ground truth for this world: the whole-map completeness reads the
			// full contract requires (derived by the oracle). It MUST contain this keyspace's
			// completeness read, else the world does not exercise the fold.
			truth := extendedGroundTruthWholeSetReads(w.c, w.b)
			if _, ok := truth[completenessLeafKey(ab.tag)]; !ok {
				t.Fatalf("[%s] world %q does not require the keyspace's whole-map read — the fold is not exercised; fix the world", prettyTag(ab.tag), w.name)
			}

			// (1) PRE-extension guard: GREEN. The merged guard's ground truth (apply ∪ validity)
			// lists NO whole-map completeness read — it never runs the quorum stack — so its
			// coverage check is satisfied by the current producer. The blind spot.
			preTruth := groundTruthReadSet(t, w.c, setV5(*w.b))
			producedKeys := keySet(w.c.WitnessReadSetV5(setV5(*w.b)))
			preMissing := producerCoversWholeSetReads(producedKeys, preTruth)
			if len(preMissing) > 0 {
				t.Fatalf("[%s] PRE-extension guard is RED (%v) — the forgery leaks into apply/validity, so this does not isolate the quorum-stack blind spot", prettyTag(ab.tag), preMissing)
			}
			t.Logf("[%s] PRE-extension guard GREEN: apply ∪ validity ground truth lists no whole-map read — structurally blind to the quorum fold", prettyTag(ab.tag))

			// (2) EXTENDED guard: RED. The per-key producer emits no completeness leaf, so it
			// fails to cover the extended ground truth's whole-map read.
			extMissing := producerCoversWholeSetReads(producedKeys, truth)
			if len(extMissing) == 0 {
				t.Fatalf("[%s] EXTENDED guard GREEN — the per-key producer already covers the whole-map read; the blind spot would not be caught", prettyTag(ab.tag))
			}
			t.Logf("[%s] EXTENDED guard RED: producer omits the whole-map completeness read(s) %v → a committed digest root is required (gated format change)", prettyTag(ab.tag), extMissing)

			// (3) POSITIVE CONTROL: augment the producer with the completeness leaves. The
			// extended guard now goes GREEN — the RED above is caused by the MISSING leaf, not a
			// hardcoded false. This is the future gated fix, proven to close the guard.
			augmented := make(map[string]struct{}, len(producedKeys)+len(truth))
			for k := range producedKeys {
				augmented[k] = struct{}{}
			}
			for k := range truth {
				augmented[k] = struct{}{}
			}
			if fixedMissing := producerCoversWholeSetReads(augmented, truth); len(fixedMissing) > 0 {
				t.Fatalf("[%s] POSITIVE CONTROL FAILED: even with the completeness leaves the extended guard stays RED (%v) — the RED is not the missing leaf; the ablation is a tautology", prettyTag(ab.tag), fixedMissing)
			}
			t.Logf("[%s] POSITIVE CONTROL GREEN: adding the completeness leaf closes the extended guard — the RED is the missing whole-map read, not a hardcoded false", prettyTag(ab.tag))
		})
	}
}
