package chain

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — CLASS B (bond registrations) — the THIRD delta-derivable changed-whole-set-digest class.
//
// research: floorbox-Rboundary-writeset-digest-reconstruction-
// (B: carries the residual — the displacement branch is
// a screen the delta MUST reproduce exactly; under/over-reproduction is FOLD-CAUGHT, so it is a
// liveness/derivation-correctness burden, never a wrong-accept.)
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block whose committed-state effect is entries/revocations (E/R) PLUS a set of
// on-chain bond registrations (B). It stalls loud on every other class.
//
// WHY B IS THE RICHEST DELTA. A single accepted reg (chain.go) touches up to EIGHT
// committed leaves for the registrant plus TWO for a displaced squatter, and moves the id's TTL
// due-bucket:
// - bondRootOwner||Root = EncodeID(id) (ADD fresh / CHANGE on displacement)
// - bondRootProven||Root = EncodeBool(true) (ADD, only if proven = height>0)
// - bonded||id = EncodeInt64(size) (ADD fresh / CHANGE renew)
// - bondRegHeight||id = EncodeUint64(height) (ADD / CHANGE)
// - regVersion||id = EncodeUint8(version) (ADD / CHANGE)
// - bondDomain||id = EncodeUint64(domain) (ADD / CHANGE)
// - qualified||id = EncodeInt64(size) (ADD/CHANGE iff size>=MinBond && !slashed)
// - dueBucket moves: DELETE id from old bucket (renew) + INSERT id into new bucket
// - DISPLACEMENT: delete(bonded,oldOwner) + qualifiedMaintain(oldOwner) — an id NOT in
// The payload, read from bondRootOwner[Root] Across the block it changes the bondedRoot /
// qualifiedRoot whole-set digests (iff membership changed — a pure same-id renew does NOT
// change either digest, since the id-set is unchanged).
//
// THE DELTA DERIVATION. The box reproduces apply's screens FROM ITS OWN CFG,
// never a witness scalar:
// 1. canonicalBondRegs(b.BondRegs) — the same-id last-writer canonicalization (chain.go),
// So the derived per-id winner is order-free and identical to apply.
// 2. per-reg screens: len==ed25519 size, Size>=MinBondBytes, !slashed[id] (from the anchored
// pre-slashed set, and the per-root displacement branch (proof-beats-declaration) read from
// the committed bondRootOwner/bondRootProven leaves (per-key witnesses).
// 3. the surviving winners drive the per-member write-set + the bonded/qualified/dueBucket deltas.
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

	// WITNESS-SOUNDNESS ANCHORS (per-root proofs against prevStateRoot). The
	// displacement branch reads PriorOwner/Claimed/PriorProven to decide whether to strip a squatter's
	// standing; a forged read flips the decision (the ForgedPriorOwner/Claimed/PriorProven attacks). Each
	// is anchored the same way a fold-written leaf is — by a proof the box VERIFIES against prevStateRoot
	// before the read is trusted. A nil/forged proof yields NoWitness ⇒ stall (never a false read).
	//
	// OwnerProof anchors PriorOwner/Claimed: present-proof of bondRootOwner||root → EncodeID(PriorOwner)
	// (Claimed=true) OR non-membership proof (Claimed=false).
	OwnerProof statehash.Witness
	// ProvenProof anchors PriorProven: present-proof of bondRootProven||root → EncodeBool(true)
	// (PriorProven=true) OR non-membership proof (PriorProven=false — apply writes the proven leaf only
	// when true, so an unproven claimed root has no proven leaf). Read only when Claimed=true.
	ProvenProof statehash.Witness
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
// derived by reproducing apply's canonicalization + screens + displacement + due-bucket moves.
type bondRegDelta struct {
	writes       []stateRootWrite          // per-member leaf writes (bonded/bondRegHeight/regVersion/bondDomain/owner/proven adds+changes, displaced deletes)
	bucketMoves  map[uint64]bucketMove     // due-height → the insert/delete on that bucket
	postBonded   map[ports.NodeID]struct{} // bonded id-set AFTER the block (for bondedRoot)
	postQual     map[ports.NodeID]struct{} // qualified id-set AFTER the block (for qualifiedRoot)
	qualWrites   map[ports.NodeID][]byte   // per-id qualified leaf write (value or nil=delete)
	regVerWrites map[ports.NodeID]uint8    // per-id regVersion||id write (in-block bond; fold-anchored via the class-B changed leaf)
	registered   map[ports.NodeID]struct{} // the ids whose registration survived the screens — the TTL clocks this block reset
}

// bucketMove records the id-set inserts/deletes on one due-bucket this block.
type bucketMove struct {
	inserts map[ports.NodeID]struct{}
	deletes map[ports.NodeID]struct{}
}

// stateRootBondRegWriteSet reproduces apply's bond-reg loop (chain.go) LEAF EFFECT and
// returns the full delta. It reads the box's OWN cfg (MinBondBytes, BondTTLBlocks) for the screens
// — never a witness scalar. preBonded / preQualified / preSlashed are the anchored pre-state sets
// (from the digest witnesses); ownership is the per-root committed screen (bondRootOwner/Proven).
//
// The delta is the reproduction: canonicalize same-id regs, drop below-floor /
// malformed / slashed regs, resolve the per-root proof-beats-declaration displacement, then emit the
// surviving winners' per-member writes + the bonded/qualified/due-bucket membership changes. proven =
// b.Height > 0 (a height>0 reg went through validateBondRegs; genesis is declared).
func (c *Chain) stateRootBondRegWriteSet(
	prevStateRoot ports.Hash,
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
	// ANCHOR the per-root displacement inputs against prevStateRoot BEFORE reading them. The displacement branch (below) reads owner[root]/claimed[root]/provenRoot[root] to
	// decide whether to strip a squatter's standing. A forged PriorOwner/Claimed/PriorProven flips
	// that decision (the ForgedPriorOwner/Claimed/PriorProven attacks). Each is trusted only after
	// its proof Resolves against prevStateRoot; a nil/forged proof yields NoWitness ⇒ stall.
	for root, sc := range screens {
		ownerKey := statehash.Key(tagBondRootOwner, root[:])
		if sc.Claimed {
			// Claimed=true ⇒ bondRootOwner||root present at EncodeID(PriorOwner).
			ownerRes := statehash.Resolve(prevStateRoot, ownerKey, statehash.EncodeID(sc.PriorOwner), sc.OwnerProof)
			if !ownerRes.IsProvenPresent() {
				return bondRegDelta{}, fmt.Errorf("%w: bondReg root %x Claimed=true owner %x not proven present against prevStateRoot",
					ErrRecomputeStateRootDigest, root[:], sc.PriorOwner[:])
			}
			// PriorProven: true ⇒ bondRootProven||root present at EncodeBool(true); false ⇒ absent (apply
			// writes the proven leaf only when true, so an unproven claimed root has NO proven leaf).
			provenKey := statehash.Key(tagBondRootProven, root[:])
			if sc.PriorProven {
				if !statehash.Resolve(prevStateRoot, provenKey, statehash.EncodeBool(true), sc.ProvenProof).IsProvenPresent() {
					return bondRegDelta{}, fmt.Errorf("%w: bondReg root %x PriorProven=true not proven present against prevStateRoot",
						ErrRecomputeStateRootDigest, root[:])
				}
			} else {
				if !statehash.Resolve(prevStateRoot, provenKey, nil, sc.ProvenProof).IsProvenAbsent() {
					return bondRegDelta{}, fmt.Errorf("%w: bondReg root %x PriorProven=false not proven absent against prevStateRoot",
						ErrRecomputeStateRootDigest, root[:])
				}
			}
			owner[root] = sc.PriorOwner
			claimed[root] = true
			provenRoot[root] = sc.PriorProven
		} else {
			// Claimed=false ⇒ bondRootOwner||root ABSENT (an unclaimed root). A forged Claimed=false for a
			// truly-claimed root (the ForgedClaimed attack) fails the absence proof ⇒ stall.
			if !statehash.Resolve(prevStateRoot, ownerKey, nil, sc.OwnerProof).IsProvenAbsent() {
				return bondRegDelta{}, fmt.Errorf("%w: bondReg root %x Claimed=false owner not proven absent against prevStateRoot",
					ErrRecomputeStateRootDigest, root[:])
			}
		}
	}

	var writes []stateRootWrite
	qualWrites := map[ports.NodeID][]byte{}
	regVerWrites := map[ports.NodeID]uint8{}
	registered := map[ports.NodeID]struct{}{}
	bucketMoves := map[uint64]bucketMove{}
	touchBucket := func(d uint64) bucketMove { return touchBucketMove(bucketMoves, d) }
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
			continue // apply's malformed guard
		}
		if r.Size < c.cfg.MinBondBytes {
			continue // below the objective anti-release floor
		}
		id := r.ValidatorID()
		if _, isSlashed := preSlashed[id]; isSlashed {
			continue // a slashed equivocator cannot re-earn standing (F2)
		}
		if o, isClaimed := owner[r.Root]; isClaimed && o != id {
			// PROOF BEATS DECLARATION : a verified reg displaces an unproven genesis claim.
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
		// Record the just-written regVersion so the
		// class-P freeze can cross-check an in-block bond's tally regVersion against this fold-anchored
		// value (the regVersion||id leaf is in `writes`, verified by the class-B fold), rather than the
		// PRE-state Resolve (which is absent for a fresh in-block bond and forces the id to count 0).
		regVerWrites[id] = r.Version
		registered[id] = struct{}{}
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
			// A resize BELOW MinBond drops a previously-qualified id.
			delete(postQual, id)
			qualWrites[id] = nil
			writes = append(writes, stateRootWrite{key: statehash.Key(tagQualified, id[:]), newValue: nil})
		}
	}

	return bondRegDelta{
		writes:       writes,
		bucketMoves:  bucketMoves,
		postBonded:   postBonded,
		postQual:     postQual,
		qualWrites:   qualWrites,
		regVerWrites: regVerWrites,
		registered:   registered,
	}, nil
}

// bondRegDelta derives class B's whole effect on the block's committed leaves: the per-member
// writes, the due-bucket moves, and the post-state bonded / qualified sets. It reads the box's own
// cfg for every screen, the per-root committed ownership for the displacement branch, and each
// registering id's prior bondRegHeight — taken from the supplied changed-leaf witnesses, whose
// OldValue the fold verifies against prevStateRoot, so a forged prior reg-height diverges the
// recomputed bucket MTH rather than moving a bond's clock.
//
// The pre-state sets are passed in already anchored: class B is one step of a composed transition
// (floorbox_recompute_stateroot_compose_v5.go), and the state it hands on is the state the TTL
// sweep and the slashes read.
func (c *Chain) bondRegDelta(
	prevStateRoot ports.Hash,
	b Block,
	w StateRootWitness,
	preBonded, preQualified, preSlashed map[ports.NodeID]struct{},
) (bondRegDelta, error) {
	// A renewing id's bondRegHeight||id leaf carries its pre-state due-clock; the box needs it to
	// derive the OLD due-bucket to vacate. An absent pre-state (a fresh registration) has no old
	// bucket and contributes nothing.
	preBondRegHeight := map[ports.NodeID]uint64{}
	for i := range w.ChangedLeaves {
		cl := &w.ChangedLeaves[i]
		id, ok := idFromTaggedKey(cl.Key, tagBondRegHeight)
		if !ok || len(cl.OldValue) != 8 {
			continue
		}
		preBondRegHeight[id] = binary.BigEndian.Uint64(cl.OldValue) // 8-byte BE (EncodeUint64)
	}
	screens := make(map[ports.Hash]StateRootBondRegScreen, len(w.BondRegScreens))
	for _, sc := range w.BondRegScreens {
		screens[sc.Root] = sc
	}
	return c.stateRootBondRegWriteSet(prevStateRoot, b, preBonded, preQualified, preSlashed, screens, preBondRegHeight)
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
