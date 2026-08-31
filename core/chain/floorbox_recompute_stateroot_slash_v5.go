package chain

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-b,
// CLASS S (slashes) — the FIRST delta-derivable CHANGED-WHOLE-SET-DIGEST class.
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//   PE ruling: RULING-floorbox-recompute-P1b-SA-digest-scope-2026-08-31.md
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block whose committed-state effect is entries/revocations (E/R) PLUS a set
// of on-chain equivocation slashes (S). It stalls loud on every other class.
//
// WHY S NEEDS MORE THAN THE E/R FOLD. E/R touches only byRoot/spent/revoked per-member leaves,
// none of which has a committed WHOLE-SET DIGEST scalar. A slash of `culprit` (chain.go:3285-3290)
// changes THREE whole-set digest scalars in addition to three per-member leaves:
//   - slashed[culprit]=true    → adds a slashed||culprit leaf     + changes slashedRoot
//   - delete(bonded,culprit)   → deletes a bonded||culprit leaf   + changes bondedRoot
//   - qualifiedMaintain(culprit) → deletes qualified||culprit IF it was qualified + changes qualifiedRoot
// Each digest scalar (nodeSetMTHFromInt64/Bool, statehash.go:262-266) is an RFC-6962 MTH over the
// WHOLE post-state id-set of its keyspace. nodeSetMTH is a whole-list fold with NO incremental
// update, so reproducing postRoot==StateRoot requires reconstructing each touched digest from the
// whole post-set id-list.
//
// THE CERTIFIED CHANGED-DIGEST PRIMITIVE (stateRootDigestOp). For each touched digest scalar:
//   1. Reconstruct the PRE-state id-set from the witness; anchor it against the pre-digest COMMITTED
//      UNDER prevStateRoot (NOT StateRoot — that is circular; residual R-anchor-prevroot). The op's
//      OldValue = preDigest is verified against prevStateRoot by FoldChangedPaths' own per-op
//      VerifyProof, and the box requires nodeSetMTH(preIDs) == preDigest (completeness anchor,
//      cert sub-Q2): an omitted/injected pre-member yields a different MTH ⇒ stall.
//   2. Apply the payload-DERIVED membership delta to the pre-set → post-set.
//   3. NewValue = nodeSetMTH(post-set).
//   4. Emit the digest scalar as ONE FoldOp at Key(tag, nil).
// The existing FoldChangedPaths + the terminal postRoot==StateRoot equality then close it: a wrong
// derivation diverges the recomputed root from the committed StateRoot ⇒ stall. No new fold
// primitive; no apply()/consensus change.
//
// COST — HONEST (R-cost-wholeset, R-membership). NOT O(payload). Reconstructing a digest needs the
// WHOLE post-set id-list, so class S is O(payload leaves) + O(|keyspace|) per touched digest =
// O(|slashed|)+O(|bonded|)+O(|qualified|) ≈ O(registry) per touched digest. It rides directly on
// R-membership (no code-enforced bound on total bonded/qualified/slashed membership — OPEN,
// load-bearing for the #657 accept-flip). Box-fits at RegCap-era populations (kilobytes per
// digest), degrades to megabytes-per-block at 100k-member populations.

// StateRootDigestWitness carries, for ONE touched whole-set digest scalar, the claimed pre-state
// member id-list plus the inclusion proof of that digest's committed leaf against prevStateRoot.
// It is UNTRUSTED: the box reconstructs nodeSetMTH(PreIDs) and requires it equals the committed
// pre-digest (which FoldChangedPaths verifies against prevStateRoot), so a short or padded id-list
// stalls.
type StateRootDigestWitness struct {
	// Tag is the digest tag (tagBondedRoot / tagQualifiedRoot / tagSlashedRoot). The box derives the
	// SET of touched digests from the payload and matches each to a witness by tag; a witness for a
	// tag not in the derived set is ignored (the derived set is authoritative).
	Tag string
	// PreIDs is the CLAIMED pre-state member id-list of the keyspace. The box requires
	// nodeSetMTH(PreIDs) == the committed pre-digest, so an omitted/injected member stalls.
	PreIDs []ports.NodeID
	// Proof is the inclusion proof of the digest leaf Key(Tag, nil) against prevStateRoot. Its proven
	// value is the committed pre-digest — routed as the FoldOp's OldValue, verified against
	// prevStateRoot by FoldChangedPaths (R-anchor-prevroot).
	Proof statehash.Witness
}

// stateRootSlashWriteSet derives the class-S per-member committed-leaf write-set for block b,
// reproducing apply()'s slash loop (chain.go:3285-3290) LEAF EFFECT:
//   - slashed||culprit  ADD (value Present)   — always
//   - bonded||culprit   DELETE                 — the pre-state bonded[culprit] leaf, if present
//   - qualified||culprit DELETE                — the pre-state qualified[culprit] leaf, if present
//
// Like applyEntriesRevocationsWriteSet, the KEY set is a pure function of the payload (the
// culprits) — completeness bound. The oldValue is left nil here; the matched witness carries the
// pre-state claim and the fold verifies it against prevStateRoot. A bonded/qualified DELETE fires
// only if the culprit was present pre-state; the box reads that presence from the anchored pre-set
// id-lists (the digest witnesses), NOT from a witness scalar (C-1) — see stateRootSlashDigestOps.
// A culprit that was neither bonded nor qualified changes only slashed (and slashedRoot).
func stateRootSlashWriteSet(b Block, preBonded, preQualified map[ports.NodeID]struct{}) []stateRootWrite {
	type wr struct {
		newValue []byte
		isDelete bool
	}
	byKey := map[string]wr{}
	order := []string{}
	remember := func(key []byte, newValue []byte, isDelete bool) {
		k := string(key)
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = wr{newValue: newValue, isDelete: isDelete}
	}
	for i := range b.Slashes {
		culprit := b.Slashes[i].CulpritID()
		// slashed: ADD (the culprit becomes slashed). Idempotent if already slashed.
		remember(statehash.Key(tagSlashed, culprit[:]), statehash.Present, false)
		// bonded: DELETE if the culprit was bonded pre-state (else no bonded leaf to remove).
		if _, ok := preBonded[culprit]; ok {
			remember(statehash.Key(tagBonded, culprit[:]), nil, true)
		}
		// qualified: DELETE if the culprit was qualified pre-state. Post-slash the culprit is slashed
		// ⇒ idQualifies is false ⇒ it must not be in post-qualified; the leaf effect is a delete of the
		// pre-state qualified leaf. Membership is taken from the anchored pre-qualified set (C-1), not a
		// witness claim.
		if _, ok := preQualified[culprit]; ok {
			remember(statehash.Key(tagQualified, culprit[:]), nil, true)
		}
	}
	out := make([]stateRootWrite, 0, len(order))
	for _, k := range order {
		v := byKey[k]
		nv := v.newValue
		if v.isDelete {
			nv = nil
		}
		out = append(out, stateRootWrite{key: []byte(k), newValue: nv})
	}
	return out
}

// stateRootSlashDigestOps reconstructs the THREE touched whole-set digest scalars (slashedRoot,
// bondedRoot, qualifiedRoot) as FoldOps, via the certified changed-digest primitive. For each:
// verify the claimed pre-set id-list reconstructs the committed pre-digest, apply the payload-
// derived S membership delta to the pre-set, and fold the post-digest as the changed leaf.
//
// The pre-set membership the S per-member write-set needs (was the culprit bonded / qualified) is
// derived HERE from the anchored pre-sets and returned to the caller — so the per-member delta and
// the digest delta agree on the pre-state by construction, and neither trusts a witness scalar (C-1).
//
// It returns the digest FoldOps, plus the pre-bonded / pre-qualified membership sets the per-member
// write-set consumes. A missing/short/padded pre-set id-list stalls (nodeSetMTH != committed
// pre-digest). A touched digest with no supplied witness stalls (the box will not fold an
// unwitnessed digest change).
func stateRootSlashDigestOps(
	b Block,
	digestWits []StateRootDigestWitness,
) (ops []statehash.FoldOp, preBonded, preQualified map[ports.NodeID]struct{}, err error) {
	byTag := make(map[string]*StateRootDigestWitness, len(digestWits))
	for i := range digestWits {
		byTag[digestWits[i].Tag] = &digestWits[i]
	}

	// Reconstruct + anchor each touched pre-set once. slashed / bonded / qualified are ALWAYS touched
	// by a non-empty slash block (slashed always changes; bonded/qualified change iff any culprit was
	// a member — but the digest scalar leaf is committed every block and its VALUE changes whenever
	// the set changes, so the box must reconstruct all three to fold the block's committed StateRoot).
	preSlashed, err := anchoredPreSet(byTag, tagSlashedRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	bondedSet, err := anchoredPreSet(byTag, tagBondedRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	qualifiedSet, err := anchoredPreSet(byTag, tagQualifiedRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	preBonded = bondedSet
	preQualified = qualifiedSet

	// Apply the payload-derived S delta to each pre-set → post-set (copy, don't mutate the pre-sets;
	// the per-member write-set still needs the pre-membership).
	postSlashed := cloneIDSet(preSlashed)
	postBonded := cloneIDSet(bondedSet)
	postQualified := cloneIDSet(qualifiedSet)
	for i := range b.Slashes {
		culprit := b.Slashes[i].CulpritID()
		postSlashed[culprit] = struct{}{} // slashed: ADD
		delete(postBonded, culprit)       // bonded: evict (no-op if absent)
		delete(postQualified, culprit)    // qualified: post-slash unqualified ⇒ evict (no-op if absent)
	}

	ops = []statehash.FoldOp{
		digestFoldOp(tagSlashedRoot, byTag, postSlashed),
		digestFoldOp(tagBondedRoot, byTag, postBonded),
		digestFoldOp(tagQualifiedRoot, byTag, postQualified),
	}
	return ops, preBonded, preQualified, nil
}

// anchoredPreSet reconstructs one digest's pre-state id-set from its witness. It requires a witness
// for the tag. The COMPLETENESS ANCHOR is enforced downstream by digestFoldOp + FoldChangedPaths:
// the FoldOp's OldValue is nodeSetMTH(PreIDs), and FoldChangedPaths verifies that OldValue against
// prevStateRoot via the digest leaf inclusion proof. So a PreIDs that is not the true committed
// pre-set yields an OldValue != the committed pre-digest ⇒ the fold's VerifyProof fails ⇒ stall.
// The pre-set the delta is applied to is therefore the completeness-anchored set (cert sub-Q2).
func anchoredPreSet(byTag map[string]*StateRootDigestWitness, tag string) (map[ports.NodeID]struct{}, error) {
	w, ok := byTag[tag]
	if !ok || w.Proof.IsNil() {
		return nil, fmt.Errorf("%w: no digest witness for touched %s", ErrRecomputeStateRootDigest, tag)
	}
	set := make(map[ports.NodeID]struct{}, len(w.PreIDs))
	for _, id := range w.PreIDs {
		set[id] = struct{}{}
	}
	return set, nil
}

// digestFoldOp builds the FoldOp for one touched digest scalar: OldValue = the committed pre-digest
// (nodeSetMTH over the witnessed pre-set, verified against prevStateRoot by the fold), NewValue =
// nodeSetMTH over the post-set, Key = Key(tag, nil), Proof = the digest leaf inclusion proof.
func digestFoldOp(tag string, byTag map[string]*StateRootDigestWitness, postSet map[ports.NodeID]struct{}) statehash.FoldOp {
	w := byTag[tag] // present: anchoredPreSet already required it
	preDigest := nodeSetMTH(w.PreIDs)
	post := make([]ports.NodeID, 0, len(postSet))
	for id := range postSet {
		post = append(post, id)
	}
	return statehash.FoldOp{
		Key:      statehash.Key(tag, nil),
		OldValue: preDigest,
		NewValue: nodeSetMTH(post),
		Proof:    w.Proof,
	}
}

// cloneIDSet returns a shallow copy of an id-set.
func cloneIDSet(m map[ports.NodeID]struct{}) map[ports.NodeID]struct{} {
	out := make(map[ports.NodeID]struct{}, len(m))
	for id := range m {
		out[id] = struct{}{}
	}
	return out
}

// sortIDs returns the ids sorted ascending by raw bytes — the canonical order nodeSetMTH imposes,
// exposed for tests that build a pre-set id-list deterministically.
func sortIDs(ids []ports.NodeID) []ports.NodeID {
	out := append([]ports.NodeID(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
	return out
}
