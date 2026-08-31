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

// StateRootMaturityWitness is the class-M witness: the committed pre-state everMature scalar leaf
// proof (against prevStateRoot) plus the SeenSet maturity witness (against committedStateRoot) the
// box feeds RecomputeMatureNow to decide the latch. It is the SINGLE source of the tagEverMature
// reconstruction — both class M (the write) and class P (the freeze gate) consume the SAME
// post-latch verdict the entry computes from this witness.
type StateRootMaturityWitness struct {
	// EverMature is the committed pre-state everMature scalar: OldValue is the PRE-latch value (the
	// fold's OldValue, verified against prevStateRoot); Proof is its inclusion proof. The box computes
	// the POST value (pre || matureNow) and folds the scalar ONLY on the crossing (pre false → post
	// true).
	EverMature StateRootRotateScalar
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
) (ops []statehash.FoldOp, postEverMature bool, reason error) {
	mw := w.Maturity
	if mw == nil {
		// No class-M witness supplied. The box cannot know the pre-latch everMature or reconstruct
		// matureNow, so it cannot rule the block a non-crossing — it stalls (never-Accept). An E/R-only
		// block on a pre-latched chain still supplies a Maturity witness (pre=true, no SeenSet needed);
		// the entry requires it so the latch is never silently skipped.
		return nil, false, fmt.Errorf("%w: no class-M maturity witness (cannot rule out an everMature crossing)", ErrRecomputeStateRootMaturity)
	}
	preEverMature := decodeBoolLeaf(mw.EverMature.OldValue)
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
