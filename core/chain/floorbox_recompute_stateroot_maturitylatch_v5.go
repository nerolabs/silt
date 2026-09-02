package chain

import (
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, CLASS M
// (the everMature maturity latch), the BOUNDARY-INDEPENDENT reproducer.
//
// WHY THIS CLASS EXISTS (the write-obligation ledger gap, PE ruling 2026-08-31:
//   silt-reviews/principle-engineer/RULING-floorbox-v5-write-obligation-ledger-2026-08-31.md).
// apply() latches everMature false→true at the TOP of EVERY block where !everMature && Mature()
// (chain.go:3303-3305), BEFORE the boundary gate (chain.go:3315). The write can land on ANY
// height. #678 reproduced it ONLY inside class P's rotateOps (boundary-gated), so the GENERIC
// off-boundary maturity crossing (h % EpochBlocks != 0) had no reproducer → the recompute folded
// no tagEverMature op → the recomputed post-root committed everMature=false while the committed
// root committed everMature=true → mismatch → STALL. Safe (never wrong-accept — the box still
// never Accepts) but a liveness gap fatal to the #657 accept-flip (every objective chain crosses
// maturity exactly once, off-boundary is the generic case).
//
// THE SINGLE OWNER. Class M is the ONLY reproducer of the tagEverMature leaf write. Class P's
// emission was REMOVED (#678 put it there); P now only READS the post-latch everMature (threaded
// from the entry) to decide its pre-latch early-return vs the freeze. At a crossing that lands ON
// a boundary, M emits the write once and P reads only — no double-emit.
//
// THE RECONSTRUCTION. post_everMature = pre_everMature || matureNow(thisBlock). matureNow reads
// the POST-apply bonded/seen set, so it verifies against committedStateRoot (the post-apply root),
// NOT prevStateRoot. The box reuses RecomputeMatureNow (#668, floorbox_recompute_maturity_v5.go)
// — NOT a rebuild — over the class-M SeenSet witness. A forged maturity witness cannot verify
// against the committed root ⇒ RecomputeMatureNow stalls ⇒ the box stalls (never wrongly latches,
// never wrongly skips). When pre_everMature is already true there is no crossing: M emits nothing
// and requires no maturity witness.
//
// OBJECTIVE-BRANCH ONLY (same discipline as the other classes). RecomputeMatureNow reproduces the
// OBJECTIVE branch of matureNow (MatureValidators>0). In the non-objective launch phase everMature
// latches at genesis, so pre_everMature is already true for any post-genesis block and M is inert.

// StateRootMaturityWitness is the class-M witness AND the box's carrier for the committed
// young→mature HANDOFF pre-state. It carries the two handoff scalar leaf proofs (everMature and
// matureEpoch, both against prevStateRoot) plus the SeenSet maturity witness (against
// committedStateRoot) the box feeds RecomputeMatureNow to decide the latch.
//
// It is the SINGLE source of the tagEverMature reconstruction — both class M (the write) and class P
// (the freeze gate) consume the SAME post-latch verdict the entry computes from this witness — and,
// since R-FOLD-LIVE-STATE-READS (2026-09-02), the single source of the handoff pre-state the class-A
// screen branches on. The entry REQUIRES it on every block, so both readers are served
// boundary-independently.
type StateRootMaturityWitness struct {
	// EverMature is the committed pre-state everMature scalar: OldValue is the PRE-latch value (the
	// fold's OldValue, verified against prevStateRoot); Proof is its inclusion proof. The box computes
	// the POST value (pre || matureNow) and folds the scalar ONLY on the crossing (pre false → post
	// true).
	EverMature StateRootRotateScalar
	// MatureEpoch is the committed pre-state matureEpoch scalar: OldValue is the PRE-state value
	// (the value apply()'s attestation loop reads, chain.go:3293-3298, which runs BEFORE rotateEpoch
	// flips it — rotate-LAST, #620/PR #703); Proof is its inclusion proof against prevStateRoot.
	//
	// WHY IT IS HOMED HERE and not on the class-P rotate witness (R-FOLD-LIVE-STATE-READS cert
	// 2026-09-02, Q3 step 1). The class-A attestation screen needs this value on EVERY block to pick
	// its qualification branch, and the class-P StateRootRotateWitness is nil off-boundary
	// (floorbox_recompute_stateroot_v5.go, isBoundary). The class-M carrier is already REQUIRED on
	// every block (so the latch is never silently skipped) and already carries the sibling EverMature
	// scalar, so the handoff pair travels together. Class P keeps its OWN anchor + emit of the
	// tagMatureEpoch WRITE at a boundary; both Resolve the same leaf against the same prevStateRoot,
	// so they cannot verify to different values and there is no double-emit.
	//
	// The box NEVER reads c.matureEpoch for this. A box that replays no apply() never has it set, so
	// the live read screened every mature-epoch block under the PRE-maturity rule — reachable as both
	// a wrong-accept (a mid-epoch joiner folded in) and a false stall, with every witness proof
	// passing.
	MatureEpoch StateRootRotateScalar
	// SeenSet is the maturity witness (validatorsSeen id-list + per-member bonded/domain/slashed proofs
	// + the validatorsSeenRoot digest proof) RecomputeMatureNow verifies against committedStateRoot.
	// REQUIRED only when the pre-latch everMature is FALSE (the box must reconstruct matureNow to know
	// whether this block is the crossing); a pre-latched block supplies no SeenSet and never reads it.
	SeenSet SeenSetWitness
}

// maturityLatchOps reconstructs the class-M FoldOps for a block and reports the POST-latch
// everMature the block commits. It is the SINGLE owner of the tagEverMature write. It returns the
// (possibly empty) op set, the post-latch everMature (threaded to class P for its freeze gate), and
// a stall reason.
//
// When the pre-latch everMature is already true, the block cannot cross maturity: no op, post=true,
// no witness read. When it is false, the box reconstructs matureNow(thisBlock) via RecomputeMatureNow
// over committedStateRoot; if mature it emits the tagEverMature false→true FoldOp and reports
// post=true. A missing/forged maturity witness stalls (never-Accept preserved).
func (c *Chain) maturityLatchOps(
	committedStateRoot ports.Hash,
	w StateRootWitness,
	pre stateRootHandoffPre,
) (ops []statehash.FoldOp, postEverMature bool, reason error) {
	// The class-M witness and BOTH handoff scalars were already anchored against prevStateRoot by
	// handoffPreState, which the entry runs UNCONDITIONALLY before any class dispatches
	// (R-FOLD-LIVE-STATE-READS cert 2026-09-02, Q3 step 2). One Resolve per scalar per block, not
	// two: the pre-values arrive here already proven present.
	mw := w.Maturity
	preEverMature := pre.everMature
	if preEverMature {
		// Already latched: no crossing, no write, no maturity recompute. Post stays true.
		return nil, true, nil
	}
	// Unlatched: reconstruct matureNow over the POST-apply committed state. A forged witness cannot
	// verify against committedStateRoot ⇒ RecomputeMatureNow stalls ⇒ the box stalls.
	matureNow, mErr := c.RecomputeMatureNow(committedStateRoot, mw.SeenSet)
	if mErr != nil {
		return nil, false, fmt.Errorf("%w: maturity recompute for the latch decision: %v", ErrRecomputeStateRootMaturity, mErr)
	}
	if !matureNow {
		// No crossing this block: everMature stays false, no leaf write.
		return nil, false, nil
	}
	// The crossing: everMature false→true. Fold the committed scalar (monotonic one-way latch, F-1).
	// scalarFoldOp emits iff post != pre; here pre is false and post is true, so it always emits.
	if op, changed := scalarFoldOp(tagEverMature, mw.EverMature, statehash.EncodeBool(true)); changed {
		ops = append(ops, op)
	}
	return ops, true, nil
}

// stateRootHandoffPre carries the box's COMMITTED pre-state view of the young→mature handoff for
// one block: the two one-way latch scalars, Resolved present against prevStateRoot, plus the
// derived handoff predicate. It replaces every read of c.everMature / c.matureEpoch / c.handedOff()
// in the recompute (R-FOLD-LIVE-STATE-READS cert 2026-09-02).
type stateRootHandoffPre struct {
	// everMature is the committed pre-state tagEverMature value (the class-M latch input).
	everMature bool
	// matureEpoch is the committed pre-state tagMatureEpoch value — the value apply()'s attestation
	// loop reads (chain.go:3293-3298, BEFORE rotateEpoch:3306, rotate-LAST). It is the class-A screen's
	// branch selector.
	matureEpoch bool
	// handedOff is handedOff() (chain.go:1223-1228) evaluated over the anchored pre-values:
	// matureEpoch with epochs enabled, everMature without (R-HANDOFF-EPOCHS-OFF, covered rather than
	// argued unreachable). It is the launch-anchor eligibility input.
	handedOff bool
}

// handoffPreState anchors the committed pre-state handoff scalars against prevStateRoot and returns
// the box's pre-state handoff view. The entry runs it UNCONDITIONALLY, before ANY class dispatches —
// including for a block with no attestations and no boundary — so no branch can suppress the anchor.
//
// WHY (R-FOLD-LIVE-STATE-READS cert 2026-09-02, Q1/Q3). The class-A screen picked its qualification
// branch from c.matureEpoch and its anchor eligibility from c.launchAnchor → c.handedOff(). Those
// fields are written ONLY by apply→rotateEpoch (chain.go:3398) and adopt (chain.go:3911). The
// deployment target "holds NO registry and replays NO apply()", so on a COLD box they are never set
// and the mature-epoch branch is UNREACHABLE: every mature-epoch block is screened under the
// pre-maturity rule. That diverges from what a full node accepts in BOTH directions, with every
// witness proof passing — a mid-epoch joiner (bonded ≥ MinBond, ∉ the frozen epochSet) is folded in
// against an attacker's root (wrong-accept, and the class-M poisoning entry), and the same
// attestation inside an HONEST block stalls the box (false stall). The verdict must be a function of
// (prevStateRoot, committedStateRoot, b, w, own-cfg) ONLY.
//
// Both scalars are committed leaves under both roots (statehash.go tagEverMature/tagMatureEpoch), so
// IsProvenPresent is the correct three-valued check; anchorRotateScalar is the Direction-A primitive
// class P already uses. STALL-ADDING ONLY: a forged/absent OldValue yields NoWitness ⇒ stall, never
// an Accept.
func (c *Chain) handoffPreState(prevStateRoot ports.Hash, w StateRootWitness) (stateRootHandoffPre, error) {
	mw := w.Maturity
	if mw == nil {
		// No class-M witness supplied. The box can neither anchor the handoff pre-state nor rule out an
		// everMature crossing, so it stalls (never-Accept). An E/R-only block on a pre-latched chain
		// still supplies a Maturity witness (pre=true, no SeenSet needed); the entry requires it so the
		// latch is never silently skipped and the class-A branch is never chosen unanchored.
		return stateRootHandoffPre{}, fmt.Errorf("%w: no class-M maturity witness (cannot anchor the committed handoff pre-state, nor rule out an everMature crossing)",
			ErrRecomputeStateRootMaturity)
	}
	// DIRECTION A. preEverMature is read as a BRANCH predicate (a forged OldValue=true returns
	// post=true with NO crossing recompute and NO leaf write — the class-P suppression shape), and
	// preMatureEpoch is read as the class-A branch selector. Anchor BOTH unconditionally, before the
	// branches.
	if err := anchorRotateScalar(prevStateRoot, tagEverMature, mw.EverMature); err != nil {
		return stateRootHandoffPre{}, fmt.Errorf("%w: everMature.OldValue not proven present against prevStateRoot (Direction A anchor): %v",
			ErrRecomputeStateRootMaturity, err)
	}
	if err := anchorRotateScalar(prevStateRoot, tagMatureEpoch, mw.MatureEpoch); err != nil {
		return stateRootHandoffPre{}, fmt.Errorf("%w: matureEpoch.OldValue not proven present against prevStateRoot (Direction A anchor): %v",
			ErrRecomputeStateRootMaturity, err)
	}
	pre := stateRootHandoffPre{
		everMature:  decodeBoolLeaf(mw.EverMature.OldValue),
		matureEpoch: decodeBoolLeaf(mw.MatureEpoch.OldValue),
	}
	// Defensive cross-check (cert Q3 step 4). matureEpoch ⇒ everMature is emergent from rotateEpoch
	// (chain.go:3395-3398 early-returns unless everMature) and pinned by
	// TestMatureEpochImpliesEverMature_InvariantPin. A pre-state that violates it is not one any honest
	// apply() can commit, so the box refuses to screen against it. Stall-adding only.
	if pre.matureEpoch && !pre.everMature {
		return stateRootHandoffPre{}, fmt.Errorf("%w: committed pre-state has matureEpoch=true with everMature=false — no honest apply() can commit that (rotateEpoch latches matureEpoch only past the everMature gate)",
			ErrRecomputeStateRootMaturity)
	}
	// handedOff() (chain.go:1223-1228) over the ANCHORED pre-values. epochsEnabled() is own-cfg +
	// the injected verifier (C-6, asserted wired at the entry), never live state.
	if c.epochsEnabled() {
		pre.handedOff = pre.matureEpoch
	} else {
		pre.handedOff = pre.everMature
	}
	return pre, nil
}
