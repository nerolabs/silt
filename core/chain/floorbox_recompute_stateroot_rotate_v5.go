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

	// R1.2 WITNESS-SOUNDNESS ANCHORS (per-member proofs against prevStateRoot, BUILD notes D3/D4). The
	// freeze writes EncodeInt64(Weight) into the epochSet||id leaf and the tally reads Weight/RegVersion;
	// a forged Weight moves only the membership-only epochSet digest's per-member leaf (the ForgedFrozenWeight
	// attack), and a forged RegVersion/RegVersionKnown flips a lock-in tally (the ForgedRegVersion attacks).
	// Each is trusted only after its proof Resolves against prevStateRoot; a nil/forged proof stalls.
	//
	// QualifiedProof anchors Weight for a STEADY-STATE member (not bonded/resized this block): present-proof
	// of qualified||id → EncodeInt64(Weight). For an id whose qualified||id this block MUTATED, the box
	// cross-checks Weight against the class-B-derived qualWrites[id] instead (that leaf is anchored by the
	// class-B fold), and QualifiedProof is not read.
	QualifiedProof statehash.Witness
	// RegVersionProof anchors RegVersion/RegVersionKnown: present-proof of regVersion||id → EncodeUint8(RegVersion)
	// (RegVersionKnown=true) OR non-membership proof (RegVersionKnown=false), against prevStateRoot. For an
	// id whose regVersion||id this block MUTATED (bonded in-block), the box cross-checks RegVersion against
	// the class-B write instead and RegVersionProof is not read.
	RegVersionProof statehash.Witness
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
	// The everMature scalar + the SeenSet maturity witness are NOT homed here. The everMature latch is
	// class M (StateRootMaturityWitness on the entry, floorbox_recompute_stateroot_maturitylatch_v5.go),
	// the boundary-independent SINGLE owner of the tagEverMature write. Class P consumes only the
	// post-latch everMature the entry threads in (the freeze gate) — it neither reads the maturity
	// witness nor emits the leaf.
	//
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
func (c *Chain) reconstructPostQualified(prevStateRoot ports.Hash, b Block, w StateRootWitness) (map[ports.NodeID]struct{}, error) {
	post, _, err := c.reconstructPostQualifiedWithWrites(prevStateRoot, b, w)
	return post, err
}

// reconstructPostQualifiedWithWrites is reconstructPostQualified that ALSO returns the per-id
// qualified leaf writes class B derived this block (R1.2 class-P Weight anchor, BUILD note D4). For
// an id bonded/resized in THIS block the pre-state qualified||id leaf is stale/absent, so the
// steady-state qualified-leaf anchor is wrong; the box cross-checks the frozen Weight against the
// B-derived qualWrites[id] (itself anchored by the class-B fold) instead. qualWrites is nil for a
// non-bond-reg boundary.
func (c *Chain) reconstructPostQualifiedWithWrites(prevStateRoot ports.Hash, b Block, w StateRootWitness) (map[ports.NodeID]struct{}, map[ports.NodeID][]byte, error) {
	byTag := make(map[string]*StateRootDigestWitness, len(w.DigestPreSets))
	for i := range w.DigestPreSets {
		byTag[w.DigestPreSets[i].Tag] = &w.DigestPreSets[i]
	}
	preQualified, err := anchoredPreSet(byTag, tagQualifiedRoot)
	if err != nil {
		return nil, nil, err
	}
	post := cloneIDSet(preQualified)
	var qualWrites map[ports.NodeID][]byte

	// (1) Bond regs (apply() FIRST): B computes the whole post-qualified set from the same pre-qualified
	// anchor. Adopt it wholesale as the qualified set post-B.
	if len(b.BondRegs) > 0 {
		_, _, bPostQual, bQualWrites, bErr := c.bondRegOpsWithQualWrites(prevStateRoot, b, w)
		if bErr != nil {
			return nil, nil, bErr
		}
		post = cloneIDSet(bPostQual)
		qualWrites = bQualWrites
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
	return post, qualWrites, nil
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
	prevStateRoot ports.Hash,
	b Block,
	w StateRootWitness,
	postQualified map[ports.NodeID]struct{},
	qualWrites map[ports.NodeID][]byte,
	everMature bool,
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

	// THE MATURITY LATCH READ (class M owns the WRITE). apply() latches everMature BEFORE rotateEpoch
	// (chain.go:3303-3316): post_everMature = pre_everMature || matureNow(thisBlock). rotate's
	// early-return + freeze read the POST-latch value. The box computes that value ONCE in the entry
	// (class M, floorbox_recompute_stateroot_maturitylatch_v5.go, the SINGLE owner of the tagEverMature
	// leaf write) and threads it here as everMature — so P gates the freeze on it WITHOUT re-deriving or
	// re-emitting the leaf (no double-emit at a boundary-coincident crossing).

	var ops []statehash.FoldOp

	// epochStart = b.Height — ALWAYS. Fold iff the value changes.
	if op, changed := scalarFoldOp(tagEpochStart, rw.EpochStart, statehash.EncodeUint64(b.Height)); changed {
		ops = append(ops, op)
	}

	if !everMature {
		// Pre-latch boundary: ONLY epochStart is written. No freeze, no tallies. (The tagEverMature write
		// is class M's, not P's — and on a pre-latch boundary there is no crossing, so M emits nothing.)
		return ops, nil
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
		// R1.2: ANCHOR the frozen Weight and RegVersion against prevStateRoot BEFORE they enter the
		// tally / epochSet-leaf freeze (BUILD notes D3/D4). This anchors the INPUTS to the activation
		// quorum; the tally arithmetic (3*ready>2*total) in rotateTallyOps is UNTOUCHED (PE constraint 2,
		// the #402 non-fork rule).
		if err := c.anchorRotateMember(prevStateRoot, m, qualWrites); err != nil {
			return nil, err
		}
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

// anchorRotateMember re-anchors a frozen member's untrusted Weight and RegVersion against
// prevStateRoot (R1.2, BUILD notes D3/D4), so a forged value cannot enter the epochSet-leaf freeze or
// the activation tally. It touches NO tally arithmetic (PE constraint 2 / #402 non-fork).
//
// Weight: a member whose qualified||id leaf this block MUTATED (present in qualWrites) is cross-checked
// against the class-B-derived write (anchored by the class-B fold); a steady-state member's Weight is
// required to be the committed qualified||id value under prevStateRoot (present-proof at EncodeInt64(Weight)).
//
// RegVersion: matched to the PRE-state committed regVersion||id leaf — present-proof at
// EncodeUint8(RegVersion) when RegVersionKnown, non-membership proof otherwise. A fresh in-block bond has
// no pre-state regVersion leaf, so its honest witness sets RegVersionKnown=false and the absence proof
// verifies. A forged RegVersion/RegVersionKnown that claims a pre-state value it cannot prove stalls.
func (c *Chain) anchorRotateMember(prevStateRoot ports.Hash, m StateRootRotateMember, qualWrites map[ports.NodeID][]byte) error {
	// Weight anchor.
	if qw, mutated := qualWrites[m.ID]; mutated {
		// In-block-mutated qualified leaf: cross-check against the class-B write (fold-anchored). A frozen
		// member must have a non-nil (present) qualified write — a delete cannot be frozen.
		if qw == nil {
			return fmt.Errorf("%w: frozen member %x has a class-B qualified DELETE this block (cannot be frozen)",
				ErrRecomputeStateRootDigest, m.ID[:])
		}
		if string(qw) != string(statehash.EncodeInt64(m.Weight)) {
			return fmt.Errorf("%w: frozen member %x Weight %d does not match the class-B qualified write",
				ErrRecomputeStateRootDigest, m.ID[:], m.Weight)
		}
	} else {
		// Steady-state member: the frozen Weight IS the committed qualified||id leaf under prevStateRoot.
		qualKey := statehash.Key(tagQualified, m.ID[:])
		if !statehash.Resolve(prevStateRoot, qualKey, statehash.EncodeInt64(m.Weight), m.QualifiedProof).IsProvenPresent() {
			return fmt.Errorf("%w: frozen member %x Weight %d not proven present in qualified||id against prevStateRoot",
				ErrRecomputeStateRootDigest, m.ID[:], m.Weight)
		}
	}

	// RegVersion anchor (matched to the pre-state committed regVersion||id leaf).
	regKey := statehash.Key(tagRegVersion, m.ID[:])
	if m.RegVersionKnown {
		if !statehash.Resolve(prevStateRoot, regKey, statehash.EncodeUint8(m.RegVersion), m.RegVersionProof).IsProvenPresent() {
			return fmt.Errorf("%w: frozen member %x RegVersion %d not proven present against prevStateRoot",
				ErrRecomputeStateRootDigest, m.ID[:], m.RegVersion)
		}
	} else {
		if !statehash.Resolve(prevStateRoot, regKey, nil, m.RegVersionProof).IsProvenAbsent() {
			return fmt.Errorf("%w: frozen member %x RegVersionKnown=false not proven absent against prevStateRoot",
				ErrRecomputeStateRootDigest, m.ID[:])
		}
	}
	return nil
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
