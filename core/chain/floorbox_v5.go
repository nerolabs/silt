package chain

import "errors"

// era-4 (v5) trustless floor-box validation — lane-1 Part B, increment B1 (the sound,
// additive slice).
//
// A floor box is a SEMI-STATELESS witness-validating client: it holds the two committed
// roots (StateRoot / LogRoot), not the tree, and validates a v5 block trustlessly by
// verifying witnesses against those roots (the #600 posture, decisions.md 2026-08-28). This
// file is the ADDITIVE validation MODE — a SEPARATE path a root-only client calls INSTEAD of
// holding the tree. It does NOT modify apply(), validateEra3Roots, postApplyRoots, any
// validity predicate, or any consensus invariant I1–I5. A full node's acceptance path is
// unchanged; a full node never calls WitnessValidateV5.
//
// WHAT THIS INCREMENT SHIPS (the sound, non-gated slice — see the PACE deliberation,
// docs/thinking/2026-08-30-lane1-partB-witness-validation-options.md):
//   - the #535 cold-auditor recovery-boundary policy (RATIFIED, decisions.md 2026-08-30
//     item 3): box-LOCAL directive drives recovery-boundary validation; a directive ABSENT
//     at an ambiguous recovery boundary yields a LOUD IndeterminateTrustlessly (default =
//     do NOT accept, never trust the proposer); live-follower is an OPT-IN flip;
//   - the additive entry point WitnessValidateV5, wired to apply the #535 decision FIRST and
//     then — on the trustless path — return IndeterminateTrustlessly with ErrRecomputeGated,
//     because the bounded witnessable RECOMPUTE (the accept core) is research-gated and does
//     NOT yet exist.
//
// WHAT THIS INCREMENT DELIBERATELY DOES NOT SHIP (routed to the research gate): the bounded
// witnessable recompute that decides Accept/Reject. The PE ruling confirms this recompute
// "does not yet exist in the tree (Part B)"
// (RULING-lane1-partA-readset-v5-producer-2026-08-30, premise 1), and building it soundly is
// blocked on two verified obstructions (PACE doc, Option 3): (1) apply() iterates WHOLE
// committed maps (the bondRegHeight TTL sweep, chain.go:3272) that the BOUNDED read-set does
// not witness in full, so re-running apply on a witness-seeded clone computes the WRONG
// write-set; (2) some read-set leaves are DIGESTS (dueBucket[h] is an MTH over its id set,
// statehash.go:224), not the typed data apply iterates, so the box cannot enumerate the
// expiring members from the witness. A correct bounded recompute is a NEW, soundness-critical
// computation that must provably match apply()'s v5 post-state root — a research-gated
// consensus/published-claim surface. This increment REFUSES to guess it: WitnessValidateV5
// never returns Accept. The safe default (stall/indeterminate) holds until a certified
// recompute lands, at which point its verdict slots into the marked seam below.
//
// CERTIFIED / RATIFIED basis:
//   - decisions.md 2026-08-30 (lane-1 increment 3): the #535 recovery directive is box-LOCAL,
//     default cold-auditor, live-follower opt-in; the read-set identity is the 23-keyspace
//     amended form (cited in readset_v5.go).
//   - era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30 (R2: the
//     recovery-boundary observable keys on cfg.LivenessRecoveryHeight, a non-committed
//     operator config the box cannot witness — the exact ambiguity this policy governs).
//   - C-7 §104 (the banned move: no/over-budget witness → accept is forbidden). The gated
//     seam here honors it: absent a certified recompute, the box does not accept.
//
// #535 CLOSURE GATE (NOT claimed closed here): the #535 residual's certifiable closure is
// gated on the #603 bonded/epochSet keystone probes (decisions.md 2026-08-30 item 3). This
// file ships the policy MECHANISM; it does not mark the residual closed.

// FloorBoxOutcome is the three-valued verdict of the trustless floor-box validation mode. It
// is deliberately three-valued, mirroring the witness accessor's Outcome (core/statehash):
// a box that cannot decide TRUSTLESSLY must say so LOUDLY, never fall through to accept.
type FloorBoxOutcome uint8

const (
	// IndeterminateTrustlessly is the SAFE DEFAULT and the zero value BY DESIGN. The box
	// could not reach a trustless accept/reject verdict — either the #535 recovery directive
	// was absent at an ambiguous boundary (cold-auditor: do not accept, never trust the
	// proposer), or the bounded witnessable recompute is not yet available. A caller MUST NOT
	// read Indeterminate as accept. Making it the zero value means a forgotten or
	// mis-constructed outcome stalls, never silently accepts (the C-7 §104 safe-default shape).
	IndeterminateTrustlessly FloorBoxOutcome = iota

	// Accept means the block validates trustlessly: it is exactly what a full node would
	// accept, established from the committed roots + witnesses alone. NOT PRODUCED IN THIS
	// INCREMENT — the bounded witnessable recompute that would justify it is research-gated.
	// A caller that observes Accept from this build has a bug; the tests assert it never
	// occurs here.
	Accept

	// Reject means the block is trustlessly INVALID (a forged root, a tampered leaf, or a
	// malformed block — e.g. a non-v5 block handed to the v5 mode). A Reject is a positive
	// disproof, distinct from Indeterminate (no verdict). In this increment Reject is produced
	// only for the malformed-input cases the mode can decide without the recompute.
	Reject
)

func (o FloorBoxOutcome) String() string {
	switch o {
	case Accept:
		return "ACCEPT"
	case Reject:
		return "REJECT"
	default:
		return "INDETERMINATE_TRUSTLESSLY"
	}
}

var (
	// ErrRecomputeGated marks an IndeterminateTrustlessly whose cause is the not-yet-built,
	// research-gated bounded witnessable recompute. It is the honest seam: the box got past
	// every check it CAN perform and stalled because the accept core is gated, not because the
	// block is bad. A caller distinguishes "I cannot decide yet (gated)" from
	// "recovery-boundary indeterminate" by this reason.
	ErrRecomputeGated = errors.New("chain: floor-box v5 witnessable recompute is research-gated (lane-1 Part B core, not yet built) — trustless accept/reject withheld")

	// ErrRecoveryDirectiveAbsent marks an IndeterminateTrustlessly caused by the #535
	// cold-auditor policy: the height is an ambiguous recovery boundary and the box has NO
	// local directive for it. The box refuses to trust the proposer's recovery re-base; it
	// stalls loudly. Flipping RecoveryDirective.LiveFollower opts into proceeding on the full
	// node's weak-subjectivity residual instead (never the default).
	ErrRecoveryDirectiveAbsent = errors.New("chain: floor-box v5 recovery boundary has no box-local directive (#535 cold-auditor: indeterminate-trustlessly, will not trust the proposer)")

	// ErrNotWitnessableVersion marks a Reject for a sub-v5 block handed to the v5 floor-box
	// mode. The mode validates only v5 blocks (the maintenance-spine committed keyspaces + the
	// bounded read-set exist only at v5); a sub-v5 block's witness story is the era-3 read-set,
	// out of this mode's scope.
	ErrNotWitnessableVersion = errors.New("chain: floor-box v5 mode requires a v5 (witnessable) block")
)

// RecoveryDirective is the box-LOCAL #535 recovery-boundary configuration. It is a
// -ws-checkpoint-class operator config sourced ONLY from the box's own configuration, NEVER
// from the proposer or the block (decisions.md 2026-08-30 item 3; amended cert R2). It is the
// floor-box analogue of Config.LivenessRecoveryHeight (the full node's own operator config):
// the box declares, out-of-band and per its own operator's judgment, which heights it has a
// recovery directive for.
//
// The #535 residual is the weak-subjectivity trust class: at an ambiguous recovery boundary
// the correct qualification set depends on a NON-committed operator decision the box cannot
// witness. A cold auditor (default) refuses to guess and stalls loudly; a live follower opts
// into the full node's existing residual.
type RecoveryDirective struct {
	// Heights is the set of block heights for which this box's operator has a local recovery
	// directive: "at height h, a recovery re-base is authorized, validate trustlessly against
	// the recomputed witnessable set." A height PRESENT here means the box may proceed past the
	// recovery gate for that height. A height ABSENT means the box has no directive; at an
	// AMBIGUOUS boundary that yields IndeterminateTrustlessly (cold-auditor). Built from local
	// config only.
	Heights map[uint64]struct{}

	// LiveFollower is the OPT-IN flip: when true, the box behaves as a live follower and
	// proceeds past an ambiguous recovery boundary on the full node's existing
	// weak-subjectivity residual (as if it inherited the proposer chain's finality), instead
	// of stalling. Default false = cold-auditor (favors full trustlessness). This is an opt-in
	// of the SAME flag class, never the default (decisions.md 2026-08-30 item 3).
	LiveFollower bool
}

// hasDirective reports whether the box has a local recovery directive for height h.
func (d RecoveryDirective) hasDirective(h uint64) bool {
	if d.Heights == nil {
		return false
	}
	_, ok := d.Heights[h]
	return ok
}

// isAmbiguousRecoveryBoundary reports whether height h is an ambiguous recovery boundary for
// this chain's config — the height at which full-node validation WOULD take effectiveEpochSet's
// recovery branch, which the floor box cannot witness (amended cert R2). It mirrors that
// branch's gate EXACTLY (chain.go:1466-1468): recovery fires iff cfg.LivenessRecoveryHeight is
// set, h equals it, epochs are enabled, AND h is an epoch boundary. At any height the branch
// would NOT take, the qualification set is the frozen, witnessable epochSet, so there is no
// ambiguity. Matching the gate exactly avoids flagging a height that would not actually
// trigger the recovery re-base (a false indeterminate would needlessly stall an honest box).
//
// The floor box learns the chain's LivenessRecoveryHeight from the same public consensus
// config a full node uses; the AMBIGUITY is not whether recovery is configured (that is
// public) but whether the box may TRUST the re-base — which is the box-local directive's job.
// A box whose own directive covers h has resolved the ambiguity for itself; a box without one
// has not.
func (c *Chain) isAmbiguousRecoveryBoundary(h uint64) bool {
	return c.cfg.LivenessRecoveryHeight != 0 && h == c.cfg.LivenessRecoveryHeight &&
		c.epochsEnabled() && c.cfg.EpochBlocks != 0 && h%c.cfg.EpochBlocks == 0
}

// RecoveryBoundaryDecision is the #535 policy unit: whether the box may proceed to trustless
// validation at height h, or must emit IndeterminateTrustlessly. It is a PURE function of the
// height, the chain's public recovery config, and the box's LOCAL directive — it reads NOTHING
// from the proposer or the block. Separated out so it is unit-testable in isolation and so
// B2's recompute consumes exactly this decision.
//
// The policy (decisions.md 2026-08-30 item 3):
//   - not an ambiguous recovery boundary ⇒ proceed (proceed=true): normal trustless path.
//   - ambiguous boundary WITH a box-local directive ⇒ proceed: the operator authorized the
//     re-base; validate trustlessly against the recomputed set.
//   - ambiguous boundary WITHOUT a directive, cold-auditor (default) ⇒ do NOT proceed:
//     IndeterminateTrustlessly (ErrRecoveryDirectiveAbsent). Never trust the proposer.
//   - ambiguous boundary WITHOUT a directive, live-follower (opt-in) ⇒ proceed on the weak-
//     subjectivity residual.
func (c *Chain) RecoveryBoundaryDecision(h uint64, d RecoveryDirective) (proceed bool, reason error) {
	if !c.isAmbiguousRecoveryBoundary(h) {
		return true, nil // no ambiguity: the qualification set is the frozen, witnessable epochSet.
	}
	if d.hasDirective(h) {
		return true, nil // the box's own operator authorized the recovery re-base at h.
	}
	if d.LiveFollower {
		return true, nil // opt-in: proceed on the full node's weak-subjectivity residual.
	}
	// Cold-auditor default: no directive at an ambiguous boundary ⇒ loud indeterminate.
	return false, ErrRecoveryDirectiveAbsent
}

// WitnessValidateV5 is the additive trustless floor-box validation entry point for a v5 block.
// Given the block, the parent committed StateRoot the box holds (parentStateRoot — the state
// the block's witnesses are proven against), and the box-LOCAL recovery directive, it returns
// a three-valued FloorBoxOutcome and, for a non-Accept, the reason.
//
// It is ADDITIVE: it calls no full-node accept path and mutates nothing. It reads the chain's
// public config (immutable during validation) to locate the recovery boundary; it does not
// touch live committed state.
//
// ORDER (load-bearing):
//  1. Version gate: a sub-v5 block is Reject (ErrNotWitnessableVersion) — the mode is v5-only.
//  2. #535 recovery-boundary decision FIRST: an ambiguous boundary with no box-local directive
//     (cold-auditor) short-circuits to IndeterminateTrustlessly BEFORE any recompute would run
//     — the box will not even attempt a trustless verdict it cannot ground.
//  3. The trustless recompute seam: this is where the bounded witnessable recompute would
//     verify parentStateRoot + witnesses ⇒ b.StateRoot and return Accept/Reject. It is
//     RESEARCH-GATED and not yet built, so this increment returns IndeterminateTrustlessly
//     with ErrRecomputeGated. It NEVER returns Accept — refusing to guess the soundness-
//     critical recompute is the safe, certified behavior (C-7 §104).
//
// parentStateRoot is accepted now (not deferred to B2) so the signature is stable across the
// gated seam: when the recompute lands it consumes parentStateRoot + the witness bundle here.
func (c *Chain) WitnessValidateV5(b Block, parentStateRoot [32]byte, d RecoveryDirective) (FloorBoxOutcome, error) {
	// (1) Version gate — v5-only mode.
	if b.Version < BlockVersionWitnessable {
		return Reject, ErrNotWitnessableVersion
	}

	// (2) #535 recovery-boundary decision, FIRST. A cold-auditor box with no directive at an
	// ambiguous boundary stalls loudly here, never trusting the proposer, never reaching the
	// recompute seam.
	if proceed, reason := c.RecoveryBoundaryDecision(b.Height, d); !proceed {
		return IndeterminateTrustlessly, reason
	}

	// (3) The trustless witnessable-recompute seam — RESEARCH-GATED, not yet built.
	//
	// When the certified bounded recompute lands (lane-1 Part B core), it goes HERE: verify
	// every read-set witness against parentStateRoot (IngestBlockWitnesses, #634), recompute
	// the v5 post-state root over the witnessed reads WITHOUT scanning O(registry) state, and
	// return Accept iff it equals b.StateRoot (== what a full node would accept), else Reject.
	// Until then the box does NOT accept: the safe default holds.
	_ = parentStateRoot // consumed by the gated recompute; wired now for a stable signature.
	return IndeterminateTrustlessly, ErrRecomputeGated
}
