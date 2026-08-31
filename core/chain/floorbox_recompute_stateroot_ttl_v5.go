package chain

import (
	"encoding/binary"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-c,
// CLASS T (TTL sweep) — the SECOND delta-derivable changed-whole-set-digest class.
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//     (T CERTIFIED-in-direction, inherits the CRUX dueBucket reconstruction)
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block whose committed-state effect is entries/revocations (E/R) PLUS a
// firing TTL sweep (T). It stalls loud on every other class.
//
// WHY T NEEDS THE DIGEST PRIMITIVE. The sweep (chain.go:3271-3281) expires every id whose bond
// clock ran out this block. The EXPIRED SET is exactly the members of dueBucket[b.Height]: an id
// registered at regH has due-height regH+ttl+1, and it expires at h iff h-regH > ttl, i.e.
// regH+ttl+1 == h. So the whole bucket keyed at b.Height empties this block — the O(1)/O(bucket)
// accelerator witness, NOT a whole bondRegHeight scan (the whole point of era-4). Each expired id
// deletes FIVE per-member leaves and, across the set, changes TWO whole-set digest scalars:
//   - delete(bonded,id)        → deletes bonded||id     + changes bondedRoot
//   - delete(bondRegHeight,id) → deletes bondRegHeight||id
//   - delete(regVersion,id)    → deletes regVersion||id
//   - qualifiedMaintain(id)    → deletes qualified||id IF it was qualified + changes qualifiedRoot
//   - the bucket itself        → deletes dueBucket||b.Height (its LAST member leaves)
// bondDomain||id is NOT deleted (apply keeps it — chain.go:3275-3277 deletes bonded/bondRegHeight/
// regVersion only). The two touched whole-set digests (bondedRoot, qualifiedRoot) are reconstructed
// by the same changed-digest primitive class S ships (stateRootSlashDigestOps): witness the pre-set
// id-list against prevStateRoot, apply the payload/accelerator-derived DELETE delta, fold the
// post-digest as the changed leaf.
//
// THE ACCELERATOR WITNESS (the T delta source). The expired set is carried as ONE dueBucket
// witness: the members of dueBucket[b.Height], anchored by proving dueBucket||b.Height's committed
// MTH leaf against prevStateRoot and requiring dueBucketMTH(members) == that committed value. A
// short/padded member list yields a different MTH ⇒ stall (the CRUX completeness closure). The
// bucket leaf itself is DELETED (its last member leaves), so its FoldOp is NewValue=nil with the
// off-path delete siblings. Height-contiguity (the dueBucket gate's invariant): the box reads the
// bucket keyed at EXACTLY b.Height — an id in that bucket is due at b.Height by construction of
// dueBucketMoveOnReg (due = regH+ttl+1), so every member of dueBucket[b.Height] expires this block.
//
// COST — HONEST (R-cost-wholeset, R-membership). NOT O(payload). Reconstructing bondedRoot/
// qualifiedRoot needs the WHOLE post-set id-list (MTH is a whole-list fold, no incremental update),
// so class T is O(payload) + O(|bonded|) + O(|qualified|) + O(|bucket|) ≈ O(registry) per touched
// digest. It rides directly on R-membership (OPEN, load-bearing for the #657 accept-flip).

// StateRootTTLWitness carries the TTL-sweep delta source: the members of dueBucket[b.Height] (the
// EXPIRED set) plus the inclusion proof of that bucket's committed MTH leaf against prevStateRoot,
// and the off-path siblings the bucket-leaf DELETE resolves. It is UNTRUSTED: the box reconstructs
// dueBucketMTH(Members) and requires it equals the committed bucket value (via the FoldOp OldValue
// verified against prevStateRoot), so a short/padded expired set stalls.
type StateRootTTLWitness struct {
	// Height is the sweep height (b.Height). The bucket key is Key(tagDueBucket, uint64BE(Height)).
	Height uint64
	// Members is the CLAIMED expired id-list = the members of dueBucket[Height]. The box requires
	// dueBucketMTH(Members) == the committed bucket MTH, so an omitted/injected member stalls.
	Members []ports.NodeID
	// BucketProof is the inclusion proof of the dueBucket||uint64BE(Height) leaf against
	// prevStateRoot. Its proven value is the committed bucket MTH — routed as the bucket FoldOp's
	// OldValue, verified against prevStateRoot by FoldChangedPaths (R-anchor-prevroot).
	BucketProof statehash.Witness
	// BucketDeleteSiblings are the off-path siblings the bucket-leaf DELETE resolves (the bucket
	// empties this block, so its leaf is removed). Verified faithful by the fold's root equality.
	BucketDeleteSiblings []statehash.FoldSibling
}

// stateRootTTLWriteSet derives the class-T per-member committed-leaf DELETE write-set for block b,
// reproducing apply()'s sweep loop (chain.go:3271-3281) LEAF EFFECT for the expired set:
//   - bonded||id         DELETE — always (an expired bond is evicted)
//   - bondRegHeight||id  DELETE — always
//   - regVersion||id     DELETE — always
//   - qualified||id      DELETE — IFF the id was qualified pre-state (from the anchored pre-set, C-1)
//
// bondDomain||id is NOT deleted (apply keeps it). The dueBucket||h leaf DELETE is NOT emitted here —
// it is the bucket FoldOp stateRootTTLDigestOps builds (carrying its own proof + delete siblings),
// so emitting it here too would double-apply the same key in the fold. The expired id-set is the
// accelerator witness (dueBucket[h] members), NOT a whole bondRegHeight scan. Membership in
// qualified is read from the anchored pre-qualified set, never a witness scalar.
func stateRootTTLWriteSet(expired []ports.NodeID, height uint64, preQualified map[ports.NodeID]struct{}) []stateRootWrite {
	_ = height
	out := make([]stateRootWrite, 0, len(expired)*4)
	for _, id := range expired {
		out = append(out,
			stateRootWrite{key: statehash.Key(tagBonded, id[:]), newValue: nil},
			stateRootWrite{key: statehash.Key(tagBondRegHeight, id[:]), newValue: nil},
			stateRootWrite{key: statehash.Key(tagRegVersion, id[:]), newValue: nil},
		)
		if _, ok := preQualified[id]; ok {
			out = append(out, stateRootWrite{key: statehash.Key(tagQualified, id[:]), newValue: nil})
		}
	}
	return out
}

// stateRootTTLDigestOps reconstructs the TWO touched whole-set digest scalars (bondedRoot,
// qualifiedRoot) as FoldOps via the certified changed-digest primitive, AND the dueBucket bucket
// DELETE FoldOp. It first anchors the expired set against the committed dueBucket MTH (the CRUX
// completeness closure), then applies the DELETE delta to the anchored pre-bonded / pre-qualified
// sets and folds each post-digest.
//
// It returns the digest+bucket FoldOps plus the pre-bonded / pre-qualified membership sets and the
// verified expired id-set the per-member write-set consumes — so the per-member delta and the
// digest delta agree on the pre-state by construction, and neither trusts a witness scalar (C-1).
//
// A missing/short/padded expired set stalls (dueBucketMTH(Members) != committed bucket MTH, caught
// by the bucket FoldOp's OldValue verify). A touched digest with no supplied pre-set witness stalls.
func stateRootTTLDigestOps(
	tw StateRootTTLWitness,
	digestWits []StateRootDigestWitness,
) (ops []statehash.FoldOp, preBonded, preQualified map[ports.NodeID]struct{}, expired []ports.NodeID, err error) {
	byTag := make(map[string]*StateRootDigestWitness, len(digestWits))
	for i := range digestWits {
		byTag[digestWits[i].Tag] = &digestWits[i]
	}

	bondedSet, err := anchoredPreSet(byTag, tagBondedRoot)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	qualifiedSet, err := anchoredPreSet(byTag, tagQualifiedRoot)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	preBonded = bondedSet
	preQualified = qualifiedSet
	expired = tw.Members

	// Apply the DELETE delta to each pre-set → post-set (copy; the per-member write-set still needs
	// the pre-membership).
	postBonded := cloneIDSet(bondedSet)
	postQualified := cloneIDSet(qualifiedSet)
	for _, id := range expired {
		delete(postBonded, id)    // an expired bond leaves bonded (no-op if absent)
		delete(postQualified, id) // an expired bond leaves qualified (no-op if absent)
	}

	// The dueBucket bucket FoldOp: OldValue = the committed bucket MTH (verified against
	// prevStateRoot), NewValue = nil (the bucket empties this block). The box requires
	// dueBucketMTH(expired) == the committed bucket MTH — the CRUX completeness anchor — by routing
	// dueBucketMTH(expired) as the FoldOp OldValue, which FoldChangedPaths verifies against
	// prevStateRoot. A short/padded expired list yields a wrong OldValue ⇒ the fold's VerifyProof
	// fails ⇒ stall.
	if tw.BucketProof.IsNil() {
		return nil, nil, nil, nil, fmt.Errorf("%w: no dueBucket witness for TTL sweep at height %d",
			ErrRecomputeStateRootDigest, tw.Height)
	}
	var hk [8]byte
	binary.BigEndian.PutUint64(hk[:], tw.Height)
	bucketOp := statehash.FoldOp{
		Key:            statehash.Key(tagDueBucket, hk[:]),
		OldValue:       dueBucketMTHFromSlice(expired),
		NewValue:       nil, // the bucket empties
		Proof:          tw.BucketProof,
		DeleteSiblings: tw.BucketDeleteSiblings,
	}

	ops = []statehash.FoldOp{
		bucketOp,
		digestFoldOp(tagBondedRoot, byTag, postBonded),
		digestFoldOp(tagQualifiedRoot, byTag, postQualified),
	}
	return ops, preBonded, preQualified, expired, nil
}

// dueBucketMTHFromSlice reconstructs a due-bucket's committed MTH leaf value from a claimed member
// id-slice. It is the same canonical closure dueBucketMTH ships (sorted-ascending, unpadded id
// list), reused so the box's reconstruction is byte-identical to the producer's committed value.
func dueBucketMTHFromSlice(ids []ports.NodeID) []byte {
	set := make(map[ports.NodeID]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return dueBucketMTH(set)
}
