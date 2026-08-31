package chain

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-d,
// CLASS B (bond registrations) — the THIRD delta-derivable changed-whole-set-digest class.
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//     (B CERTIFIED-in-direction; carries the R-B-displacement residual — the displacement branch is
//      a screen the delta MUST reproduce exactly; under/over-reproduction is FOLD-CAUGHT, so it is a
//      liveness/derivation-correctness burden, never a wrong-accept.)
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block whose committed-state effect is entries/revocations (E/R) PLUS a set of
// on-chain bond registrations (B). It stalls loud on every other class.
//
// WHY B IS THE RICHEST DELTA. A single accepted reg (chain.go:3228-3265) touches up to EIGHT
// committed leaves for the registrant plus TWO for a displaced squatter, and moves the id's TTL
// due-bucket:
//   - bondRootOwner||Root   = EncodeID(id)          (ADD fresh / CHANGE on displacement)
//   - bondRootProven||Root  = EncodeBool(true)      (ADD, only if proven = height>0)
//   - bonded||id            = EncodeInt64(size)      (ADD fresh / CHANGE renew)
//   - bondRegHeight||id     = EncodeUint64(height)   (ADD / CHANGE)
//   - regVersion||id        = EncodeUint8(version)   (ADD / CHANGE)
//   - bondDomain||id        = EncodeUint64(domain)   (ADD / CHANGE)
//   - qualified||id         = EncodeInt64(size)      (ADD/CHANGE iff size>=MinBond && !slashed)
//   - dueBucket moves       : DELETE id from old bucket (renew) + INSERT id into new bucket
//   - DISPLACEMENT           : delete(bonded,oldOwner) + qualifiedMaintain(oldOwner) — an id NOT in
//                              the payload, read from bondRootOwner[Root]
// Across the block it changes the bondedRoot / qualifiedRoot whole-set digests (iff membership
// changed — a pure same-id renew does NOT change either digest, since the id-set is unchanged).
//
// THE DELTA DERIVATION (R-B-displacement). The box reproduces apply()'s screens FROM ITS OWN CFG,
// never a witness scalar (C-1/C-6):
//   1. canonicalBondRegs(b.BondRegs)  — the same-id last-writer canonicalization (chain.go:3345),
//      so the derived per-id winner is order-free and identical to apply().
//   2. per-reg screens: len==ed25519 size, Size>=MinBondBytes, !slashed[id] (from the anchored
//      pre-slashed set), and the per-root displacement branch (proof-beats-declaration) read from
//      the committed bondRootOwner/bondRootProven leaves (per-key witnesses).
//   3. the surviving winners drive the per-member write-set + the bonded/qualified/dueBucket deltas.
// A mis-reproduced screen under/over-produces the delta → wrong post-set → the FOLD CATCHES IT
// (postRoot != StateRoot ⇒ stall), never a wrong-accept.
//
// COST — HONEST (R-cost-wholeset, R-membership). NOT O(payload). Reconstructing bondedRoot/
// qualifiedRoot needs the WHOLE post-set id-list, so class B is O(payload) + O(|bonded|) +
// O(|qualified|) + O(|touched buckets|) ≈ O(registry) per touched digest. Rides R-membership.

// StateRootBondRegScreen carries, for ONE bond-reg root, the committed pre-state ownership the
// displacement branch reads: whether the root is already claimed, by whom, and whether that prior
// claim was PROVEN. It is UNTRUSTED — the box does not fold on these directly; it derives the delta
// from them, and any wrong derivation diverges the recomputed root (fold-caught). The box witnesses
// the bondRootOwner / bondRootProven leaves the same way it witnesses any changed leaf, so a forged
// screen either mismatches the committed pre-value (its own changed-leaf proof fails) or diverges
// the post-root.
type StateRootBondRegScreen struct {
	// Root is the reg's bond Root (b.BondRegs[i].Root).
	Root ports.Hash
	// PriorOwner is the committed bondRootOwner[Root] pre-state (zero if unclaimed).
	PriorOwner ports.NodeID
	// Claimed reports whether bondRootOwner[Root] is present pre-state.
	Claimed bool
	// PriorProven is the committed bondRootProven[Root] pre-state (false if unclaimed or unproven).
	PriorProven bool
}

// StateRootBucketWitness carries, for ONE affected TTL due-bucket, the claimed pre-state member
// id-list plus the inclusion proof of that bucket's committed MTH leaf against prevStateRoot, and
// (for a bucket that empties) the off-path delete siblings. It is UNTRUSTED: the box reconstructs
// dueBucketMTH(PreMembers) and requires it equals the committed bucket MTH (via the FoldOp
// OldValue, verified against prevStateRoot), so a short/padded pre-set stalls.
type StateRootBucketWitness struct {
	// DueHeight is the bucket key (dueBucket[DueHeight]). Key = Key(tagDueBucket, uint64BE(DueHeight)).
	DueHeight uint64
	// PreMembers is the CLAIMED pre-state member id-list of the bucket (empty if the bucket is absent
	// pre-state — a fresh insert into a new bucket, an ADD). The box requires
	// dueBucketMTH(PreMembers) == the committed bucket MTH (or the bucket is proven-absent).
	PreMembers []ports.NodeID
	// Proof is the inclusion (or non-membership) proof of the bucket leaf against prevStateRoot.
	Proof statehash.Witness
	// DeleteSiblings are the off-path siblings a bucket-emptying DELETE resolves (empty otherwise).
	DeleteSiblings []statehash.FoldSibling
}

// bondRegDelta is the fully-reproduced effect of one block's bond regs on the committed leaf set,
// derived by reproducing apply()'s canonicalization + screens + displacement + due-bucket moves.
type bondRegDelta struct {
	writes      []stateRootWrite          // per-member leaf writes (bonded/bondRegHeight/regVersion/bondDomain/owner/proven adds+changes, displaced deletes)
	bucketMoves map[uint64]bucketMove     // due-height → the insert/delete on that bucket
	postBonded  map[ports.NodeID]struct{} // bonded id-set AFTER the block (for bondedRoot)
	postQual    map[ports.NodeID]struct{} // qualified id-set AFTER the block (for qualifiedRoot)
	qualWrites  map[ports.NodeID][]byte   // per-id qualified leaf write (value or nil=delete)
}

// bucketMove records the id-set inserts/deletes on one due-bucket this block.
type bucketMove struct {
	inserts map[ports.NodeID]struct{}
	deletes map[ports.NodeID]struct{}
}

// stateRootBondRegWriteSet reproduces apply()'s bond-reg loop (chain.go:3228-3265) LEAF EFFECT and
// returns the full delta. It reads the box's OWN cfg (MinBondBytes, BondTTLBlocks) for the screens
// — never a witness scalar. preBonded / preQualified / preSlashed are the anchored pre-state sets
// (from the digest witnesses); ownership is the per-root committed screen (bondRootOwner/Proven).
//
// The delta is the certified R-B-displacement reproduction: canonicalize same-id regs, drop
// below-floor / malformed / slashed regs, resolve the per-root proof-beats-declaration displacement,
// then emit the surviving winners' per-member writes + the bonded/qualified/due-bucket membership
// changes. proven = b.Height > 0 (a height>0 reg went through validateBondRegs; genesis is declared).
func (c *Chain) stateRootBondRegWriteSet(
	b Block,
	preBonded, preQualified, preSlashed map[ports.NodeID]struct{},
	screens map[ports.Hash]StateRootBondRegScreen,
	preBondRegHeight map[ports.NodeID]uint64,
) (bondRegDelta, error) {
	ttl := c.cfg.BondTTLBlocks
	proven := b.Height > 0

	// Post-state sets start from the anchored pre-state; ownership tracks displacement in-block.
	postBonded := cloneIDSet(preBonded)
	postQual := cloneIDSet(preQualified)
	owner := map[ports.Hash]ports.NodeID{}
	claimed := map[ports.Hash]bool{}
	provenRoot := map[ports.Hash]bool{}
	for root, sc := range screens {
		if sc.Claimed {
			owner[root] = sc.PriorOwner
			claimed[root] = true
			provenRoot[root] = sc.PriorProven
		}
	}

	var writes []stateRootWrite
	qualWrites := map[ports.NodeID][]byte{}
	bucketMoves := map[uint64]bucketMove{}
	touchBucket := func(d uint64) bucketMove {
		bm, ok := bucketMoves[d]
		if !ok {
			bm = bucketMove{inserts: map[ports.NodeID]struct{}{}, deletes: map[ports.NodeID]struct{}{}}
			bucketMoves[d] = bm
		}
		return bm
	}
	// regHeight tracks each id's CURRENT bondRegHeight as the block progresses (pre-state + this
	// block's earlier winners), so a due-bucket move computes the OLD due-height correctly.
	regHeight := map[ports.NodeID]uint64{}
	regHeightKnown := map[ports.NodeID]bool{}
	for id, h := range preBondRegHeight {
		regHeight[id] = h
		regHeightKnown[id] = true
	}

	for _, r := range canonicalBondRegs(b.BondRegs) {
		if len(r.Validator) != ed25519.PublicKeySize {
			continue // apply()'s malformed guard
		}
		if r.Size < c.cfg.MinBondBytes {
			continue // below the objective anti-release floor (retest G4)
		}
		id := r.ValidatorID()
		if _, isSlashed := preSlashed[id]; isSlashed {
			continue // a slashed equivocator cannot re-earn standing (F2)
		}
		if o, isClaimed := owner[r.Root]; isClaimed && o != id {
			// PROOF BEATS DECLARATION (retest G3): a verified reg displaces an unproven genesis claim.
			if !(proven && !provenRoot[r.Root]) {
				continue // shared root already backs another identity → no standing
			}
			// Displace the squatter: strip its bonded + qualified standing.
			delete(postBonded, o)
			writes = append(writes, stateRootWrite{key: statehash.Key(tagBonded, o[:]), newValue: nil})
			if _, wasQual := postQual[o]; wasQual {
				delete(postQual, o)
				writes = append(writes, stateRootWrite{key: statehash.Key(tagQualified, o[:]), newValue: nil})
				qualWrites[o] = nil
			}
		}
		// bondRootOwner / bondRootProven writes.
		owner[r.Root] = id
		claimed[r.Root] = true
		writes = append(writes, stateRootWrite{key: statehash.Key(tagBondRootOwner, r.Root[:]), newValue: statehash.EncodeID(id)})
		if proven {
			provenRoot[r.Root] = true
			writes = append(writes, stateRootWrite{key: statehash.Key(tagBondRootProven, r.Root[:]), newValue: statehash.EncodeBool(true)})
		}
		// dueBucket move: remove from the OLD due-height (renew) and insert into the NEW one.
		if ttl > 0 {
			if oldReg, ok := regHeight[id]; ok && regHeightKnown[id] {
				touchBucket(oldReg + ttl + 1).deletes[id] = struct{}{}
			}
			touchBucket(b.Height + ttl + 1).inserts[id] = struct{}{}
		}
		// per-member value leaves.
		writes = append(writes,
			stateRootWrite{key: statehash.Key(tagBonded, id[:]), newValue: statehash.EncodeInt64(r.Size)},
			stateRootWrite{key: statehash.Key(tagBondRegHeight, id[:]), newValue: statehash.EncodeUint64(b.Height)},
			stateRootWrite{key: statehash.Key(tagRegVersion, id[:]), newValue: statehash.EncodeUint8(r.Version)},
			stateRootWrite{key: statehash.Key(tagBondDomain, id[:]), newValue: statehash.EncodeUint64(r.Domain)},
		)
		regHeight[id] = b.Height
		regHeightKnown[id] = true
		postBonded[id] = struct{}{}
		// qualified: filter(bonded, slashed, MinBond) — the id just bonded at r.Size and is not
		// slashed (screened above), so it qualifies iff r.Size >= MinBond.
		if r.Size >= c.cfg.MinBond {
			postQual[id] = struct{}{}
			qualWrites[id] = statehash.EncodeInt64(r.Size)
			writes = append(writes, stateRootWrite{key: statehash.Key(tagQualified, id[:]), newValue: statehash.EncodeInt64(r.Size)})
		} else if _, wasQual := preQualified[id]; wasQual {
			// a resize BELOW MinBond drops a previously-qualified id.
			delete(postQual, id)
			qualWrites[id] = nil
			writes = append(writes, stateRootWrite{key: statehash.Key(tagQualified, id[:]), newValue: nil})
		}
	}

	return bondRegDelta{
		writes:      writes,
		bucketMoves: bucketMoves,
		postBonded:  postBonded,
		postQual:    postQual,
		qualWrites:  qualWrites,
	}, nil
}

// stateRootBondRegDigestOps reconstructs the touched whole-set digests (bondedRoot, qualifiedRoot)
// and the affected dueBucket MTH leaves as FoldOps, given the derived post-state sets and bucket
// moves. It anchors each pre-set / pre-bucket against its committed value (verified against
// prevStateRoot by the fold), applies the derived delta, and folds the post value.
//
// A whole-set digest is emitted ONLY if its membership actually changed (a pure same-id renew
// leaves bondedRoot/qualifiedRoot unchanged — no touched digest, so no witness required). A bucket
// op is emitted for every affected due-height; a bucket that empties folds to nil (DELETE), a bucket
// that gains its first member folds from the empty MTH (ADD), and a bucket that changes members
// folds old→new (CHANGE).
func stateRootBondRegDigestOps(
	delta bondRegDelta,
	preBonded, preQualified map[ports.NodeID]struct{},
	digestWits []StateRootDigestWitness,
	bucketWits map[uint64]StateRootBucketWitness,
) ([]statehash.FoldOp, error) {
	byTag := make(map[string]*StateRootDigestWitness, len(digestWits))
	for i := range digestWits {
		byTag[digestWits[i].Tag] = &digestWits[i]
	}
	var ops []statehash.FoldOp

	// bondedRoot: emit iff the bonded id-SET changed.
	if !idSetsEqual(preBonded, delta.postBonded) {
		if _, ok := byTag[tagBondedRoot]; !ok {
			return nil, fmt.Errorf("%w: bonded membership changed but no bondedRoot pre-set witness", ErrRecomputeStateRootDigest)
		}
		ops = append(ops, digestFoldOp(tagBondedRoot, byTag, delta.postBonded))
	}
	// qualifiedRoot: emit iff the qualified id-SET changed.
	if !idSetsEqual(preQualified, delta.postQual) {
		if _, ok := byTag[tagQualifiedRoot]; !ok {
			return nil, fmt.Errorf("%w: qualified membership changed but no qualifiedRoot pre-set witness", ErrRecomputeStateRootDigest)
		}
		ops = append(ops, digestFoldOp(tagQualifiedRoot, byTag, delta.postQual))
	}

	// dueBucket leaves: one FoldOp per affected due-height.
	for d, mv := range delta.bucketMoves {
		bw, ok := bucketWits[d]
		if !ok || bw.Proof.IsNil() {
			return nil, fmt.Errorf("%w: no dueBucket witness for affected due-height %d", ErrRecomputeStateRootDigest, d)
		}
		pre := make(map[ports.NodeID]struct{}, len(bw.PreMembers))
		for _, id := range bw.PreMembers {
			pre[id] = struct{}{}
		}
		post := cloneIDSet(pre)
		for id := range mv.deletes {
			delete(post, id)
		}
		for id := range mv.inserts {
			post[id] = struct{}{}
		}
		var hk [8]byte
		binary.BigEndian.PutUint64(hk[:], d)
		key := statehash.Key(tagDueBucket, hk[:])

		var oldValue, newValue []byte
		if len(bw.PreMembers) > 0 {
			oldValue = dueBucketMTHFromSet(pre) // present pre-state (a CHANGE or a DELETE)
		} else {
			oldValue = nil // absent pre-state (an ADD into a new bucket)
		}
		if len(post) > 0 {
			newValue = dueBucketMTHFromSet(post)
		} else {
			newValue = nil // the bucket empties → DELETE
		}
		ops = append(ops, statehash.FoldOp{
			Key:            key,
			OldValue:       oldValue,
			NewValue:       newValue,
			Proof:          bw.Proof,
			DeleteSiblings: bw.DeleteSiblings,
		})
	}
	return ops, nil
}

// bondRegOps is the class-B assembly the recompute entry calls: it anchors the pre-bonded /
// pre-qualified / pre-slashed sets from the digest witnesses, extracts each registering id's
// pre-state bondRegHeight from the supplied changed-leaf witnesses (verified against prevStateRoot
// by the fold), derives the full B delta, and reconstructs the touched digests + affected dueBucket
// leaves. It returns the digest FoldOps and the per-member write-set the caller folds together.
func (c *Chain) bondRegOps(b Block, w StateRootWitness) ([]statehash.FoldOp, []stateRootWrite, error) {
	ops, writes, _, err := c.bondRegOpsWithQual(b, w)
	return ops, writes, err
}

// bondRegOpsWithQual is bondRegOps that ALSO returns the POST-apply qualified id-set the class-B
// delta produces. A boundary block's class-P freeze needs this (the freeze copies the post-qualified
// set, R-P-sameblock-order); a non-boundary block ignores the third return.
func (c *Chain) bondRegOpsWithQual(b Block, w StateRootWitness) ([]statehash.FoldOp, []stateRootWrite, map[ports.NodeID]struct{}, error) {
	byTag := make(map[string]*StateRootDigestWitness, len(w.DigestPreSets))
	for i := range w.DigestPreSets {
		byTag[w.DigestPreSets[i].Tag] = &w.DigestPreSets[i]
	}
	preBonded, err := anchoredPreSet(byTag, tagBondedRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	preQualified, err := anchoredPreSet(byTag, tagQualifiedRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	preSlashed, err := anchoredPreSet(byTag, tagSlashedRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	// Prior bondRegHeight for each id, read from the supplied changed-leaf witnesses' OldValue. A
	// renewing id's bondRegHeight||id leaf carries its pre-state due-clock; the box needs it to
	// derive the OLD due-bucket to vacate. The fold verifies each OldValue against prevStateRoot, so
	// a forged prior reg-height diverges the recomputed bucket MTH (fold-caught).
	preBondRegHeight := map[ports.NodeID]uint64{}
	for i := range w.ChangedLeaves {
		cl := &w.ChangedLeaves[i]
		id, ok := idFromTaggedKey(cl.Key, tagBondRegHeight)
		if !ok || len(cl.OldValue) != 8 {
			continue // not a bondRegHeight leaf, or absent pre-state (a fresh reg — no old bucket)
		}
		preBondRegHeight[id] = binary.BigEndian.Uint64(cl.OldValue) // 8-byte BE (EncodeUint64)
	}

	screens := make(map[ports.Hash]StateRootBondRegScreen, len(w.BondRegScreens))
	for _, sc := range w.BondRegScreens {
		screens[sc.Root] = sc
	}
	buckets := make(map[uint64]StateRootBucketWitness, len(w.BondRegBuckets))
	for _, bw := range w.BondRegBuckets {
		buckets[bw.DueHeight] = bw
	}

	delta, err := c.stateRootBondRegWriteSet(b, preBonded, preQualified, preSlashed, screens, preBondRegHeight)
	if err != nil {
		return nil, nil, nil, err
	}
	ops, err := stateRootBondRegDigestOps(delta, preBonded, preQualified, w.DigestPreSets, buckets)
	if err != nil {
		return nil, nil, nil, err
	}
	return ops, delta.writes, delta.postQual, nil
}

// idFromTaggedKey extracts the raw NodeID from a field-tagged leaf key if it carries the given tag.
func idFromTaggedKey(key []byte, tag string) (ports.NodeID, bool) {
	if len(key) != len(tag)+len(ports.NodeID{}) {
		return ports.NodeID{}, false
	}
	if string(key[:len(tag)]) != tag {
		return ports.NodeID{}, false
	}
	var id ports.NodeID
	copy(id[:], key[len(tag):])
	return id, true
}

// dueBucketMTHFromSet reconstructs a bucket's MTH leaf value from an id-SET.
func dueBucketMTHFromSet(set map[ports.NodeID]struct{}) []byte {
	return dueBucketMTH(set)
}

// idSetsEqual reports whether two id-sets have identical membership.
func idSetsEqual(a, b map[ports.NodeID]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}
