package chain

import (
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// era-3 committed state-root validity — build step 2b of the certified sequence.
//
// 2a (chain.go) put StateRoot/LogRoot into the block schema and the signed Hash body
// and widened versionSupported to <= 4, but added NO predicate reading the roots: a v4
// block validates on its era-2 merits alone (the 2a→2b window named in the 2a ruling).
// This file closes that window. It adds the v4-gated validity predicate so that, once
// 2c mints v4, a block whose committed roots do not equal the post-apply recompute is
// INVALID — an honest attester refuses it, a commit carrying it is rejected.
//
// Why the roots must be enforced (not merely carried): the StateRoot's bonded/epochSet
// leaves encode the WEIGHTS summed in the three super-quorum finality predicates. A
// wrong-value leaf is therefore a consensus SAFETY attack on those weight sums, not a
// read bug (research cert Q2; 2a ruling §3, "What 2b MUST carry" (3)). Enforcing
// root == recompute makes the value encoding load-bearing: the equality check IS the
// encoding's enforcement, and cross-node byte-identity is the property the step-1
// determinism oracle already proves (modelcheck_stateroot_determinism_test.go).
//
// STOP boundary (2b): this ADDS a v4-gated rejection only. It does not flip minting to
// v4, add an activation height (both 2c), or alter any era-2 rule. A v2/v3 block skips
// the predicate entirely (era-gating, Decision 4). Invariants: preserves I1–I5, alters
// none; it adds a validity rejection gated to v4 and is a pure function of (block,
// committed state) — every honest replica computes the same verdict (I5). Deliberation:
// docs/thinking/2026-08-29-era3-step2b-validity-predicate.md.

var (
	// ErrEra3RootMissing is a v4 block with a nil StateRoot or LogRoot. 2a's omitempty
	// schema makes a nil-root v4 block a well-formed CBOR object, so the "roots are
	// required" intent (format cert freeze condition 4) is enforced HERE, explicitly —
	// a nil pointer is "no root", named as such, never folded into the equality test
	// (2a ruling "What 2b MUST carry" (1)).
	ErrEra3RootMissing = errors.New("chain: era-3 (v4) block is missing a committed root (StateRoot/LogRoot required)")
	// ErrEra3StateRootMismatch is a v4 block whose committed StateRoot does not equal
	// the SMT recomputed over the post-apply committedSet.
	ErrEra3StateRootMismatch = errors.New("chain: era-3 (v4) block StateRoot does not equal the recomputed post-apply committed state root")
	// ErrEra3LogRootMismatch is a v4 block whose committed LogRoot does not equal the
	// post-apply RFC-6962 revocation-log root.
	ErrEra3LogRootMismatch = errors.New("chain: era-3 (v4) block LogRoot does not equal the recomputed post-apply revocation-log root")
	// ErrEra3VersionRequired is a sub-v4 block at or above the era-3 activation
	// boundary (era3Active). Once era-3 is active, v4 is REQUIRED — a v2/v3 block at
	// that height has no committed roots for a validator to check, which is exactly
	// the silent-mis-validation era-3 exists to prevent (cert Q7). Build step 2c.
	ErrEra3VersionRequired = errors.New("chain: block below era-3 (v4) at or above the era-3 activation height — v4 with committed roots is required")
	// ErrEra4VersionRequired is a sub-v5 block at or above the era-4 activation boundary
	// (era4Active). Once era-4 is active, v5 is REQUIRED — a v4 block at that height does
	// not commit the maintenance-spine keyspaces the era-4 witnesses depend on, so
	// accepting it at/past the boundary would silently drop the witnessable-transition
	// commitments. Build step 4d, mirroring ErrEra3VersionRequired.
	ErrEra4VersionRequired = errors.New("chain: block below era-4 (v5) at or above the era-4 activation height — v5 with the maintenance-spine committed root is required")
)

// validateEra3Version is the era-3 (v4) version-boundary rule (build step 2c). At or
// above the era-3 activation height (era3Active — derived from PRIOR committed history,
// epoch-final so reorg-stable), v4 is REQUIRED: a v2/v3 block carries no committed roots
// for a validator to check, so accepting it at/past the boundary is the silent
// mis-validation era-3 exists to prevent (cert Q7). Below the boundary this never fires
// and era-2 validation is unchanged.
//
// It is a PURE HEADER CHECK: era3Active reads only committed state (era3LockedIn /
// era3Height / cfg), available BEFORE b is applied, and b.Version is a header field. No
// clone, no apply. Both consensus-entry paths run it BEFORE applying b, so a rejected
// block is never left applied (longest-valid-prefix contract): the commit path via
// ValidateProposal, and the own-disk Reload path (appendStructural) directly. This
// symmetry across write paths mirrors validateEra3Roots — "every disk-write path enforces
// the era-3 rules" is UNIFORM across root AND version. The write-set guard
// TestEveryDiskWritePathRunsTheEra3RootCheck pins that both are on every path.
func (c *Chain) validateEra3Version(b *Block) error {
	if c.era3Active(b.Height) && b.Version < BlockVersionStateRoot {
		return fmt.Errorf("%w: height %d version %d", ErrEra3VersionRequired, b.Height, b.Version)
	}
	return nil
}

// validateEra4Version is the era-4 (v5) version-boundary rule (build step 4d), the exact
// mirror of validateEra3Version one era up. At or above the era-4 activation height
// (era4Active — derived from PRIOR committed history, epoch-final so reorg-stable), v5 is
// REQUIRED: a v4 block does not commit the maintenance-spine keyspaces the era-4 witnesses
// depend on, so accepting it at/past the boundary silently drops the witnessable-transition
// commitments. Below the boundary this never fires and era-3/era-2 validation is unchanged.
//
// It is a PURE HEADER CHECK, like validateEra3Version: era4Active reads only committed
// state (era4LockedIn / era4Height / cfg), available BEFORE b is applied. It runs on the
// SAME write paths as validateEra3Version — the commit path via ValidateProposal and the
// own-disk Reload path (appendStructural) — so "every disk-write path enforces the era
// boundary" is uniform across era-3 AND era-4. TestEveryDiskWritePathRunsTheEra4VersionCheck
// pins it on every path.
func (c *Chain) validateEra4Version(b *Block) error {
	if c.era4Active(b.Height) && b.Version < BlockVersionWitnessable {
		return fmt.Errorf("%w: height %d version %d", ErrEra4VersionRequired, b.Height, b.Version)
	}
	return nil
}

// validateEra3Roots is the era-3 (v4) committed-root predicate. For a sub-v4 block it
// is a no-op (era-gating): a v2/v3 block validates under era-2 rules UNCHANGED. For a
// v4 block it enforces, in order:
//
//  1. both roots present (nil-reject);
//  2. the committed StateRoot equals the SMT over the POST-APPLY committedSet;
//  3. the committed LogRoot equals the POST-APPLY revocation-log root.
//
// It is called from ValidateProposal (chain.go), which an honest attester runs before
// signing and which ValidateCommit invokes first — so both consensus-entry paths carry
// the check from one site. The recompute runs on a throwaway clone (postApplyRoots), so
// live chain state is never mutated during validation.
func (c *Chain) validateEra3Roots(b *Block) error {
	if b.Version < BlockVersionStateRoot {
		return nil // era-2/era-1 block: the roots predicate does not fire (Decision 4)
	}
	if b.StateRoot == nil || b.LogRoot == nil {
		return fmt.Errorf("%w: StateRoot=%v LogRoot=%v", ErrEra3RootMissing, b.StateRoot != nil, b.LogRoot != nil)
	}
	wantState, wantLog, err := c.postApplyRoots(*b)
	if err != nil {
		// StateRoot marshals the committed fields under a fixed encoding; a duplicate-key
		// error is a marshalling bug, surfaced loudly rather than silently accepting a
		// block whose root we could not recompute.
		return fmt.Errorf("chain: era-3 root recompute failed: %w", err)
	}
	if *b.StateRoot != wantState {
		return fmt.Errorf("%w: committed %x, recomputed %x", ErrEra3StateRootMismatch, *b.StateRoot, wantState)
	}
	if *b.LogRoot != wantLog {
		return fmt.Errorf("%w: committed %x, recomputed %x", ErrEra3LogRootMismatch, *b.LogRoot, wantLog)
	}
	return nil
}

// postApplyRoots computes the StateRoot and LogRoot this chain WOULD commit after
// applying block b, WITHOUT mutating live state. It clones the accumulating state into
// a scratch Chain, runs the real apply() on the scratch (so the recompute uses the one
// authoritative state-transition function — a value-encoding or apply bug surfaces
// identically on the proposer's and the validator's side), and reads the two roots off
// the scratch. This is the "replay into a fresh replica" shape Reconcile uses
// (chain.go, tmp := New(...)), reduced to a single-block dry run over the current
// committed state (O(state), not O(history)).
func (c *Chain) postApplyRoots(b Block) (state ports.Hash, log ports.Hash, err error) {
	scratch := c.cloneForDryRun()
	scratch.apply(b)
	// era-gated marshaller: a v5 block commits the era-4 maintenance-spine keyspaces
	// (qualified/dueBucket/epochStart); a v4 block commits exactly the 18 era-3 leaves,
	// byte-identical to era-3 (hazard-1). Selecting by b.Version ties the recompute to
	// the era of the block being checked.
	state, err = scratch.StateRootForVersion(b.Version)
	if err != nil {
		return ports.Hash{}, ports.Hash{}, err
	}
	return state, scratch.LogRoot(), nil
}

// cloneForDryRun returns a scratch Chain whose accumulating state is a deep copy of
// this chain's, so apply() on the clone mutates the copy and leaves the live chain
// untouched. Config/injected fields (cfg, rep, verifyBond, tokenQuorum, issuerKey) are
// copied by reference: they are immutable during a validation and apply()'s callees
// (objective, epochsEnabled, Mature, rotateEpoch) must read the SAME config the live
// chain would, or the dry-run apply diverges. Accumulating maps/slices/the revLog are
// deep-copied.
//
// DRIFT PROTECTION (the #558 class): a committed field this clone forgets would make the
// dry-run apply diverge and the recompute silently wrong. TestDryRunCloneCopiesEveryApplied
// Field guards completeness against the reflection-based field classification — a new
// committed/observable field fails that test until copied here, exactly as it fails
// TestStateFieldsAreClassified until classified.
func (c *Chain) cloneForDryRun() *Chain {
	s := &Chain{
		// ---- config / injected: shared by reference (immutable during validation) ----
		cfg:                c.cfg,
		rep:                c.rep,
		verifyBond:         c.verifyBond,
		tokenQuorum:        c.tokenQuorum,
		issuerKey:          c.issuerKey,
		trustFloorOverride: c.trustFloorOverride,
		// ---- scalars: copied by value ----
		everMature:   c.everMature,
		gateLockedIn: c.gateLockedIn,
		gateHeight:   c.gateHeight,
		era3LockedIn: c.era3LockedIn,
		era3Height:   c.era3Height,
		era4LockedIn: c.era4LockedIn,
		era4Height:   c.era4Height,
		epochStart:   c.epochStart,
		matureEpoch:  c.matureEpoch,
	}
	// ---- era-4 (v5) maintenance-spine maps: deep-copied so apply() on the clone
	// maintains the copy, not the live chain (the same #558 drift protection as the
	// era-3 maps). A forgotten copy here reddens TestDryRunCloneCopiesEveryAppliedField. ----
	s.qualified = cloneInt64MapID(c.qualified)
	s.dueBucket = cloneDueBucket(c.dueBucket)
	// ---- input history: apply() appends b to blocks; copy so the live slice is untouched ----
	s.blocks = append([]Block(nil), c.blocks...)
	// ---- committedSet maps: deep-copied ----
	s.byRoot = make(map[ports.Hash]ports.Entry, len(c.byRoot))
	for k, v := range c.byRoot {
		s.byRoot[k] = v
	}
	s.revoked = cloneBoolMapHash(c.revoked)
	s.spent = make(map[string]bool, len(c.spent))
	for k, v := range c.spent {
		s.spent[k] = v
	}
	s.validatorsSeen = cloneBoolMapID(c.validatorsSeen)
	s.slashed = cloneBoolMapID(c.slashed)
	s.bonded = cloneInt64MapID(c.bonded)
	s.epochSet = cloneInt64MapID(c.epochSet)
	s.bondRootOwner = make(map[ports.Hash]ports.NodeID, len(c.bondRootOwner))
	for k, v := range c.bondRootOwner {
		s.bondRootOwner[k] = v
	}
	s.bondRootProven = cloneBoolMapHash(c.bondRootProven)
	s.bondRegHeight = make(map[ports.NodeID]uint64, len(c.bondRegHeight))
	for k, v := range c.bondRegHeight {
		s.bondRegHeight[k] = v
	}
	s.regVersion = make(map[ports.NodeID]uint8, len(c.regVersion))
	for k, v := range c.regVersion {
		s.regVersion[k] = v
	}
	s.bondDomain = make(map[ports.NodeID]uint64, len(c.bondDomain))
	for k, v := range c.bondDomain {
		s.bondDomain[k] = v
	}
	// ---- committedLog: the RFC-6962 transparency log, deep-copied so apply()'s
	// Append hits the clone's log, not the live one ----
	s.revLog = c.revLog.Clone()
	return s
}

func cloneBoolMapHash(m map[ports.Hash]bool) map[ports.Hash]bool {
	out := make(map[ports.Hash]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneBoolMapID(m map[ports.NodeID]bool) map[ports.NodeID]bool {
	out := make(map[ports.NodeID]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneInt64MapID(m map[ports.NodeID]int64) map[ports.NodeID]int64 {
	out := make(map[ports.NodeID]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// cloneDueBucket deep-copies the era-4 due-height index: the outer map AND each inner
// id-set, so the clone's apply() maintains its own buckets without writing through a
// shared inner map into the live chain (a nested-map alias would be the #558 bug the
// dry-run clone exists to avoid).
func cloneDueBucket(m map[uint64]map[ports.NodeID]struct{}) map[uint64]map[ports.NodeID]struct{} {
	out := make(map[uint64]map[ports.NodeID]struct{}, len(m))
	for d, ids := range m {
		inner := make(map[ports.NodeID]struct{}, len(ids))
		for id := range ids {
			inner[id] = struct{}{}
		}
		out[d] = inner
	}
	return out
}
