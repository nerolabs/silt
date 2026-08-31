package chain

import (
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-e,
// CLASS P (epoch rotation → epochSet + boundary scalars) — the FIFTH and LAST Path-1 class.
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md
//     (P CERTIFIED-in-direction as a WHOLE-SET reconstruction; carries R-P-boundary-scalars,
//      R-P-tally-regversion, R-P-sameblock-order, R-P-recovery. All FOLD-CAUGHT, never wrong-accept.)
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block at an epoch boundary (rotateEpoch fires). It stalls loud otherwise.
//
// WHY P WRITES FAR MORE THAN epochSet (R-P-boundary-scalars). rotateEpoch (chain.go:3393-3500)
// writes, in order:
//   - epochStart = h                       scalar — ALWAYS (every boundary). Measured: the sole
//                                           change at a steady-state boundary.
//   - early-return if !everMature           — a pre-latch boundary writes ONLY epochStart.
//   - matureEpoch = true                    scalar — once, at the first mature rotation.
//   - epochSet = clone(qualified_POST)      per-member epochSet leaves (ADD new, DELETE removed) +
//     [normal] / liveQualifiedSet() [#535]  the epochSetRoot whole-set digest.
//   - THREE activation tallies over the frozen set, each reading regVersion[id] per member,
//     3*ready > 2*total, each gated on own-cfg *ActivationHeight == 0, each writing a lock-in bool +
//     height scalar: gateLockedIn/gateHeight (#506, regVersion>=3), era3LockedIn/era3Height (>=4),
//     era4LockedIn/era4Height (>=5).
//
// THE FREEZE SOURCE IS THE POST-APPLY qualified SET (R-P-sameblock-order). Measured: a boundary
// block that ALSO carries a bond reg freezes the just-bonded validator into epochSet. rotate runs
// LAST (chain.go:3315), after this block's B/S/T qualified maintenance. So the box:
//   1. anchors the pre-qualified id-set against prevStateRoot (qualifiedRoot pre-digest, C-1);
//   2. applies the SAME block's S/B/T qualified deltas to it FIRST (threaded from the entry);
//   3. freezes epochSet = clone(post-qualified) — reconstructs epochSetRoot as nodeSetMTH(post-set)
//      and the per-member epochSet leaf ADD/DELETEs against the prior epochSet;
//   4. runs the three tallies over the post-qualified set using per-member regVersion WITNESSES and
//      own-cfg thresholds/activation guards (R-P-tally-regversion) → reconstructs the lock-in scalars;
//   5. reconstructs epochStart + matureEpoch scalars.
// Freezing the PRE-delta set is the I3 stale-capture divergence (fold-caught: wrong epochSetRoot ⇒
// post-root != StateRoot ⇒ stall). The #535 recovery boundary re-bases from the box's OWN
// LivenessRecoveryHeight config (C-2, R-P-recovery), never a witness.
//
// SELECTION IS TOTAL — NO ORDERING HAZARD. The freeze is clone(qualified) — a set COPY, no cap, no
// top-N, no tie-break, no sort. Every qualified member is frozen; the committed epochSet leaves are
// per-member (order-free) and epochSetRoot sorts canonically. Byte-exact, reproducible.
//
// COST — HONEST. O(|qualified|) freeze + tallies + O(|prior epochSet|) removals = O(registry). Rides
// R-membership (OPEN, load-bearing for the #657 accept-flip).

// StateRootRotateScalar carries one committed scalar leaf's pre-state value + its inclusion proof
// against prevStateRoot. The box computes the post-value itself (own-cfg + the reconstructed frozen
// set) and folds the scalar ONLY when post != pre — an omitted-but-should-have-changed scalar
// diverges the post-root (fold-caught). Present ⇒ the box may need to fold this scalar's change.
type StateRootRotateScalar struct {
	// OldValue is the committed pre-state scalar value (the fold's OldValue, verified against
	// prevStateRoot).
	OldValue []byte
	// Proof is the scalar leaf's inclusion proof against prevStateRoot.
	Proof statehash.Witness
}

// StateRootRotateMember carries, for ONE frozen-set member, its committed regVersion (for the three
// activation tallies) and its frozen weight (the epochSet leaf value = the qualified weight). The
// box uses the weight to reconstruct the per-member epochSet leaf and regVersion for the tallies.
// UNTRUSTED: a forged regVersion flips a tally verdict ⇒ wrong lock-in scalar ⇒ fold-caught; a
// forged weight diverges the per-member epochSet leaf ⇒ fold-caught.
type StateRootRotateMember struct {
	// ID is the frozen member's id.
	ID ports.NodeID
	// Weight is the member's qualified weight (bonded size) = the epochSet[id] leaf value. It must
	// equal the box's reconstructed post-qualified weight for this id (the freeze copies qualified).
	Weight int64
	// RegVersion is the committed regVersion[id] (the tally readiness signal). Absent (RegVersionKnown
	// false) ⇒ counts as 0 toward every tally.
	RegVersion uint8
	// RegVersionKnown reports whether regVersion[id] is present pre-state.
	RegVersionKnown bool
	// EpochSetProof is the inclusion/non-membership proof of the prior epochSet[id] leaf against
	// prevStateRoot (the freeze WRITE-TARGET). Present when this member's epochSet leaf changes.
	EpochSetProof statehash.Witness
	// EpochSetOldValue is the committed prior epochSet[id] value (nil if the member was not in the
	// prior epochSet — a fresh ADD). The fold verifies it against prevStateRoot.
	EpochSetOldValue []byte
	// EpochSetDeleteSiblings are the off-path siblings a dropped-member epochSet leaf DELETE resolves
	// (empty for an ADD/overwrite; present only in PriorEpochSet entries that leave the set).
	EpochSetDeleteSiblings []statehash.FoldSibling
}

// StateRootRotateWitness is the class-P epoch-boundary witness. It carries the pre-qualified id-set
// (the freeze source, anchored via the qualifiedRoot digest witness in DigestPreSets), the frozen
// members (weight + regVersion + prior epochSet leaf proof), the prior epochSet members that leave
// the set (their DELETE proofs), the epochSetRoot digest witness, and the rotate scalar witnesses.
type StateRootRotateWitness struct {
	// Members is the POST-qualified frozen set: one entry per member of the qualified set AFTER this
	// block's S/B/T maintenance (the freeze source). The box cross-checks this against the
	// entry-threaded post-qualified id-set (derived from the anchored pre-qualified + same-block
	// deltas), so a short/padded member list mismatches the derived set ⇒ stall.
	Members []StateRootRotateMember
	// PriorEpochSet carries, per member of the PRIOR epochSet that is NOT in the new frozen set, the
	// DELETE proof of its epochSet[id] leaf against prevStateRoot (the freeze removes it).
	PriorEpochSet []StateRootRotateMember
	// EpochStart is the epochStart scalar witness (ALWAYS folded — epochStart advances every boundary).
	EpochStart StateRootRotateScalar
	// MatureEpoch is the matureEpoch scalar witness (folded iff the pre-value is false and the box
	// latches it this boundary).
	MatureEpoch StateRootRotateScalar
	// EverMature is the everMature scalar witness. Its OldValue is the PRE-latch everMature; the box
	// computes the POST-latch value (pre || matureNow(thisBlock)) and, at the young→mature HANDOFF
	// boundary (pre false → post true), FOLDS this scalar false→true. everMature is a committed v5 leaf
	// (statehash.go:196) and apply() latches it (class M) BEFORE rotateEpoch (chain.go:3303-3316), so on
	// the handoff the box must reconstruct that write — there is no separate class-M recompute, and the
	// boundary class P is the only class that fires at a boundary. Folded ONLY on the handoff (post !=
	// pre); a pre-latched boundary leaves it unchanged (R-P-sameblock-order, but for the everMature
	// scalar).
	EverMature StateRootRotateScalar
	// SeenSet is the maturity witness (validatorsSeen id-list + per-member bonded/domain/slashed proofs
	// + the validatorsSeenRoot digest proof) the box feeds to RecomputeMatureNow to decide the handoff.
	// REQUIRED only when the pre-latch everMature is FALSE (the box must reconstruct matureNow(thisBlock)
	// to know whether this boundary is the handoff); a pre-latched boundary supplies no SeenSet and never
	// reads it. The maturity recompute verifies every field against the committed (POST-apply) StateRoot,
	// so a forged maturity witness cannot make the box wrongly latch or wrongly skip.
	SeenSet SeenSetWitness
	// GateLockedIn / GateHeight / Era3LockedIn / Era3Height / Era4LockedIn / Era4Height are the six
	// activation-tally scalar witnesses. Each is folded iff the box's tally flips the lock-in this
	// boundary (monotonic; own-cfg gated).
	GateLockedIn StateRootRotateScalar
	GateHeight   StateRootRotateScalar
	Era3LockedIn StateRootRotateScalar
	Era3Height   StateRootRotateScalar
	Era4LockedIn StateRootRotateScalar
	Era4Height   StateRootRotateScalar
}

// reconstructPostQualified rebuilds the POST-apply qualified id-set the class-P freeze copies. It
// replays this block's qualified-mutating classes in apply() ORDER (bond regs → TTL sweep → slashes,
// chain.go:3228-3290) on the anchored pre-qualified set — the same order apply() runs, so a
// pathological compound block (e.g. an id that bonds then is slashed in ONE block) reconstructs
// byte-identically. A wrong order/set diverges epochSetRoot ⇒ fold-caught ⇒ stall.
//
// The pre-qualified anchor is the qualifiedRoot digest witness (verified against prevStateRoot). B's
// full post-qualified set (from bondRegOpsWithQual) is authoritative for B's touched ids; T deletes
// the expired set; S deletes the slashed culprits — matching qualifiedMaintain at each apply() site.
func (c *Chain) reconstructPostQualified(b Block, w StateRootWitness) (map[ports.NodeID]struct{}, error) {
	byTag := make(map[string]*StateRootDigestWitness, len(w.DigestPreSets))
	for i := range w.DigestPreSets {
		byTag[w.DigestPreSets[i].Tag] = &w.DigestPreSets[i]
	}
	preQualified, err := anchoredPreSet(byTag, tagQualifiedRoot)
	if err != nil {
		return nil, err
	}
	post := cloneIDSet(preQualified)

	// (1) Bond regs (apply() FIRST): B computes the whole post-qualified set from the same pre-qualified
	// anchor. Adopt it wholesale as the qualified set post-B.
	if len(b.BondRegs) > 0 {
		_, _, bPostQual, bErr := c.bondRegOpsWithQual(b, w)
		if bErr != nil {
			return nil, bErr
		}
		post = cloneIDSet(bPostQual)
	}
	// (2) TTL sweep (apply() SECOND): each expired id leaves qualified.
	if w.TTLSweep != nil {
		for _, id := range w.TTLSweep.Members {
			delete(post, id)
		}
	}
	// (3) Slashes (apply() THIRD): each slashed culprit leaves qualified (slashed ⇒ never qualified).
	for i := range b.Slashes {
		delete(post, b.Slashes[i].CulpritID())
	}
	return post, nil
}

// hasNonProposerAtt reports whether the block carries any attestation from a non-proposer (the only
// atts that can write validatorsSeen).
func hasNonProposerAtt(b Block) bool {
	proposer := b.ProposerID()
	for i := range b.Atts {
		if b.Atts[i].AttesterID() != proposer {
			return true
		}
	}
	return false
}

// rotateOps reconstructs the class-P boundary FoldOps: the epochSetRoot digest, the per-member
// epochSet leaf ADD/DELETEs, and the rotate scalar changes (epochStart / matureEpoch / lock-ins).
// Every op carries its own pre-state proof from the rotate witness (verified against prevStateRoot
// by the fold), so P is self-contained — it does NOT ride the entry's ChangedLeaves match path.
// postQualified is the entry-threaded POST-apply qualified id-set (pre-qualified + same-block S/B/T
// deltas); the freeze source (R-P-sameblock-order).
//
// R-P-recovery: at the #535 recovery boundary the freeze source is liveQualifiedSet() (a
// bonded/slashed/MinBond re-scan), which the box CANNOT reconstruct from the qualified accelerator
// alone. The recovery re-base is a ratified trust-the-directive carve-out (C-2); the box does not
// reproduce it from committed state, so it STALLS at a recovery boundary (never wrong-accepts — the
// safety-first behavior the operator directive assumes).
func (c *Chain) rotateOps(
	b Block,
	committedStateRoot ports.Hash,
	w StateRootWitness,
	postQualified map[ports.NodeID]struct{},
) ([]statehash.FoldOp, error) {
	rw := w.Rotate
	if rw == nil {
		return nil, fmt.Errorf("%w: epoch boundary but no rotate witness", ErrRecomputeStateRootDigest)
	}
	// R-P-recovery: the box cannot reconstruct liveQualifiedSet() from the qualified digest alone.
	// Stall at a recovery boundary (never wrong-accept).
	if c.cfg.LivenessRecoveryHeight != 0 && b.Height == c.cfg.LivenessRecoveryHeight {
		return nil, fmt.Errorf("%w: height %d is the #535 recovery boundary (liveQualifiedSet re-base is a trust-the-directive carve-out, not reconstructed)",
			ErrRecomputeStateRootScopeStall, b.Height)
	}

	// THE MATURITY LATCH (class M), reconstructed. apply() latches everMature BEFORE rotateEpoch
	// (chain.go:3303-3316): post_everMature = pre_everMature || matureNow(thisBlock). rotate's
	// early-return + freeze read the POST-latch value. The box computes the post value so it can (a) gate
	// the freeze on it and (b) reconstruct the everMature committed-leaf WRITE at the young→mature
	// handoff (the ONE boundary the pre value diverges from the post value). Reusing RecomputeMatureNow
	// (the maturity-latch recompute, floorbox_recompute_maturity_v5.go) — NOT a rebuild.
	preEverMature := decodeBoolLeaf(rw.EverMature.OldValue)
	everMature := preEverMature
	if !preEverMature {
		// Only when unlatched: reconstruct matureNow over the POST-apply committed state (the latch reads
		// the post-block bonded/seen set, so it verifies against committedStateRoot, not prevStateRoot).
		// matureNow's objective branch is what apply()'s Mature() evaluates in the objective phase (the
		// only phase the box reproduces; MatureValidators>0 here, else everMature latches at genesis and
		// preEverMature is already true). A forged maturity witness cannot verify against the committed
		// root ⇒ RecomputeMatureNow stalls ⇒ the box stalls (never wrongly latches).
		matureNow, mErr := c.RecomputeMatureNow(committedStateRoot, rw.SeenSet)
		if mErr != nil {
			return nil, fmt.Errorf("%w: maturity recompute for the handoff decision: %v", ErrRecomputeStateRootDigest, mErr)
		}
		everMature = matureNow
	}

	var ops []statehash.FoldOp

	// epochStart = b.Height — ALWAYS. Fold iff the value changes.
	if op, changed := scalarFoldOp(tagEpochStart, rw.EpochStart, statehash.EncodeUint64(b.Height)); changed {
		ops = append(ops, op)
	}

	if !everMature {
		// Pre-latch boundary: ONLY epochStart is written. No freeze, no tallies, no everMature write.
		return ops, nil
	}

	// everMature = true. Fold the committed everMature scalar iff it flips this boundary (the young→mature
	// HANDOFF: pre false → post true). This is the class-M write, reconstructed by P because the handoff
	// is always a boundary and P is the boundary class. Monotonic one-way latch (F-1).
	if op, changed := scalarFoldOp(tagEverMature, rw.EverMature, statehash.EncodeBool(true)); changed {
		ops = append(ops, op)
	}

	// matureEpoch = true. Fold iff the pre-value was false (monotonic one-way latch).
	if op, changed := scalarFoldOp(tagMatureEpoch, rw.MatureEpoch, statehash.EncodeBool(true)); changed {
		ops = append(ops, op)
	}

	// The freeze: epochSet = clone(post-qualified). Cross-check the witness member set against the
	// entry-threaded post-qualified id-set — a short/padded member list mismatches ⇒ stall.
	frozen := make(map[ports.NodeID]struct{}, len(rw.Members))
	weightByID := make(map[ports.NodeID]int64, len(rw.Members))
	regVersionByID := make(map[ports.NodeID]uint8, len(rw.Members))
	for i := range rw.Members {
		m := rw.Members[i]
		frozen[m.ID] = struct{}{}
		weightByID[m.ID] = m.Weight
		if m.RegVersionKnown {
			regVersionByID[m.ID] = m.RegVersion
		}
	}
	if !idSetsEqual(frozen, postQualified) {
		return nil, fmt.Errorf("%w: rotate member set does not match the reconstructed post-qualified set (freeze source mismatch)",
			ErrRecomputeStateRootDigest)
	}

	// epochSetRoot digest: reconstruct over the frozen id-set.
	byTag := make(map[string]*StateRootDigestWitness, len(w.DigestPreSets))
	for i := range w.DigestPreSets {
		byTag[w.DigestPreSets[i].Tag] = &w.DigestPreSets[i]
	}
	if _, ok := byTag[tagEpochSetRoot]; !ok {
		return nil, fmt.Errorf("%w: no epochSetRoot pre-set witness at the boundary freeze", ErrRecomputeStateRootDigest)
	}
	ops = append(ops, digestFoldOp(tagEpochSetRoot, byTag, frozen))

	// Per-member epochSet leaf FoldOps: overwrite each frozen member at its weight (the freeze copies
	// qualified); DELETE each prior epochSet member not in the frozen set. Each carries its own prior
	// epochSet[id] proof (membership for an overwrite/re-freeze, non-membership for a fresh ADD, and
	// the DELETE proof + siblings for a dropped member).
	memberOps, memErr := rotateEpochSetLeafOps(rw, frozen, weightByID)
	if memErr != nil {
		return nil, memErr
	}
	ops = append(ops, memberOps...)

	// The three activation tallies over the frozen set (own-cfg thresholds + activation guards). Each
	// folds a lock-in bool + height scalar iff it flips this boundary. regVersion is a per-member
	// WITNESS (R-P-tally-regversion); the thresholds (3/4/5) and *ActivationHeight guards are own-cfg.
	ops = append(ops, c.rotateTallyOps(b, rw, frozen, regVersionByID, weightByID)...)

	return ops, nil
}

// rotateEpochSetLeafOps builds the per-member epochSet leaf FoldOps for the freeze. Each frozen
// member's leaf is set to its weight (an overwrite of the prior value, or a fresh ADD); each prior
// epochSet member not re-frozen is DELETEd. Proofs come from the rotate witness's Members (freeze
// write-targets) and PriorEpochSet (dropped members). A frozen member with no epochSet proof stalls.
func rotateEpochSetLeafOps(
	rw *StateRootRotateWitness,
	frozen map[ports.NodeID]struct{},
	weightByID map[ports.NodeID]int64,
) ([]statehash.FoldOp, error) {
	var ops []statehash.FoldOp
	for i := range rw.Members {
		m := rw.Members[i]
		if m.EpochSetProof.IsNil() {
			return nil, fmt.Errorf("%w: frozen member %x has no epochSet write-target proof", ErrRecomputeStateRootDigest, m.ID[:])
		}
		ops = append(ops, statehash.FoldOp{
			Key:      statehash.Key(tagEpochSet, m.ID[:]),
			OldValue: m.EpochSetOldValue, // prior epochSet[id] (nil = fresh ADD); fold verifies vs prevStateRoot
			NewValue: statehash.EncodeInt64(weightByID[m.ID]),
			Proof:    m.EpochSetProof,
		})
	}
	for i := range rw.PriorEpochSet {
		pm := rw.PriorEpochSet[i]
		if _, stillFrozen := frozen[pm.ID]; stillFrozen {
			continue // re-frozen: the overwrite above handles it
		}
		if pm.EpochSetProof.IsNil() {
			return nil, fmt.Errorf("%w: dropped epochSet member %x has no delete proof", ErrRecomputeStateRootDigest, pm.ID[:])
		}
		ops = append(ops, statehash.FoldOp{
			Key:            statehash.Key(tagEpochSet, pm.ID[:]),
			OldValue:       pm.EpochSetOldValue, // prior epochSet[id] weight (present)
			NewValue:       nil,                 // DELETE — dropped from the epoch set
			Proof:          pm.EpochSetProof,
			DeleteSiblings: pm.EpochSetDeleteSiblings,
		})
	}
	return ops, nil
}

// rotateTallyOps reproduces rotateEpoch's three activation tallies (chain.go:3440-3499). For each
// tally it sums the frozen-set weight (total) and the ready weight (regVersion >= threshold), locks
// in iff `!locked && Config.*ActivationHeight == 0 && EpochBlocks > 0 && total > 0 && 3*ready >
// 2*total`, and folds the lock-in bool + height scalar. It folds a scalar only when the box's
// post-value differs from the witnessed pre-value.
func (c *Chain) rotateTallyOps(
	b Block,
	rw *StateRootRotateWitness,
	frozen map[ports.NodeID]struct{},
	regVersionByID map[ports.NodeID]uint8,
	weightByID map[ports.NodeID]int64,
) []statehash.FoldOp {
	var ops []statehash.FoldOp

	tally := func(threshold uint8) (total, ready int64) {
		for id := range frozen {
			w := weightByID[id]
			total += w
			if regVersionByID[id] >= threshold {
				ready += w
			}
		}
		return
	}
	appendIf := func(tag string, wit StateRootRotateScalar, newValue []byte) {
		if op, changed := scalarFoldOp(tag, wit, newValue); changed {
			ops = append(ops, op)
		}
	}
	// #506 gate tally (regVersion >= BlockVersionRegGate == 3).
	if !decodeBoolLeaf(rw.GateLockedIn.OldValue) && c.cfg.RegGateActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
		total, ready := tally(BlockVersionRegGate)
		if total > 0 && 3*ready > 2*total {
			appendIf(tagGateLockedIn, rw.GateLockedIn, statehash.EncodeBool(true))
			appendIf(tagGateHeight, rw.GateHeight, statehash.EncodeUint64(b.Height+c.cfg.EpochBlocks))
		}
	}
	// era-3 tally (regVersion >= BlockVersionStateRoot == 4).
	if !decodeBoolLeaf(rw.Era3LockedIn.OldValue) && c.cfg.Era3ActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
		total, ready := tally(BlockVersionStateRoot)
		if total > 0 && 3*ready > 2*total {
			appendIf(tagEra3LockedIn, rw.Era3LockedIn, statehash.EncodeBool(true))
			appendIf(tagEra3Height, rw.Era3Height, statehash.EncodeUint64(b.Height+c.cfg.EpochBlocks))
		}
	}
	// era-4 tally (regVersion >= BlockVersionWitnessable == 5).
	if !decodeBoolLeaf(rw.Era4LockedIn.OldValue) && c.cfg.Era4ActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
		total, ready := tally(BlockVersionWitnessable)
		if total > 0 && 3*ready > 2*total {
			appendIf(tagEra4LockedIn, rw.Era4LockedIn, statehash.EncodeBool(true))
			appendIf(tagEra4Height, rw.Era4Height, statehash.EncodeUint64(b.Height+c.cfg.EpochBlocks))
		}
	}
	return ops
}

// scalarFoldOp builds a FoldOp for a scalar leaf whose post-value the box computed, reporting whether
// it changed (post != pre). An unchanged scalar is NOT folded (matches the E/R/S/B/T "emit only
// changed leaves" pattern). The op carries the scalar's pre-state proof against prevStateRoot; a
// scalar leaf is always present, so OldValue is a membership value (never nil) and there is no delete.
func scalarFoldOp(tag string, wit StateRootRotateScalar, newValue []byte) (statehash.FoldOp, bool) {
	if string(wit.OldValue) == string(newValue) {
		return statehash.FoldOp{}, false
	}
	return statehash.FoldOp{
		Key:      statehash.Key(tag, nil),
		OldValue: wit.OldValue,
		NewValue: newValue,
		Proof:    wit.Proof,
	}, true
}

// decodeBoolLeaf decodes a committed bool scalar leaf value (statehash.EncodeBool). An absent/empty
// value decodes false.
func decodeBoolLeaf(v []byte) bool {
	return len(v) == 1 && v[0] != 0
}
