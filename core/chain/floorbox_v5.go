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

	// ErrRecoveryBoundaryStall marks the cold auditor's UNCONDITIONAL stall at an ambiguous
	// #535 recovery boundary (D0, owner call 2 of D-TRUE-UP-CALLS-2026-09-07 on direction (a')).
	// There is no directive, no opt-in and no fall-through: the correct qualification set at that
	// height depends on a non-committed operator decision the box cannot witness, so the box
	// declines to have a view of it rather than take one from the proposer.
	//
	// The stall is TERMINAL, not per-block (certification §2.1). The box needs a VERIFIED parent
	// state root for H+1, and its only source is H's committed StateRoot — the exact quantity it
	// declined to reproduce at H. Recovery is the operator's, out of band: a fresh
	// -ws-checkpoint-class H+1:HASH pin, on which the box cold-starts. See the four-clause
	// re-anchor contract on WitnessValidateV5 and docs/design/owned-residuals.md.
	ErrRecoveryBoundaryStall = errors.New("chain: floor-box v5 height is an ambiguous #535 recovery boundary (cold auditor: indeterminate-trustlessly, terminal until the operator re-anchors, will not trust the proposer)")

	// ErrNotWitnessableVersion marks a Reject for a sub-v5 block handed to the v5 floor-box
	// mode. The mode validates only v5 blocks (the maintenance-spine committed keyspaces + the
	// bounded read-set exist only at v5); a sub-v5 block's witness story is the era-3 read-set,
	// out of this mode's scope.
	ErrNotWitnessableVersion = errors.New("chain: floor-box v5 mode requires a v5 (witnessable) block")
)

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

// recoveryBoundaryDecision is the #535 policy unit: whether the box may proceed to trustless
// validation at height h, or must emit IndeterminateTrustlessly. It is a PURE function of the
// height and the chain's public recovery config — it reads NOTHING from the proposer, from the
// block, and (since D0) from any box-local knob.
//
// The policy (D-TRUE-UP-CALLS-2026-09-07 (2), direction (a'), certification Definition 2):
//   - not an ambiguous recovery boundary => proceed: the qualification set there is the frozen,
//     witnessable epochSet, so there is no ambiguity and an honest box is not stalled needlessly;
//   - an ambiguous recovery boundary => STALL, unconditionally and loudly.
//
// (a') is strictly narrowing: it removes the only three paths by which the box could proceed past
// the boundary (directive-present, live-follower, the un-gated fall-through). Removing paths to
// Accept is monotone in the safe direction and cannot manufacture a wrong-accept, which is why the
// certification could preserve I1/I3/I4 in three sentences.
func (c *Chain) recoveryBoundaryDecision(h uint64) (proceed bool, reason error) {
	if c.isAmbiguousRecoveryBoundary(h) {
		return false, ErrRecoveryBoundaryStall
	}
	return true, nil
}

// WitnessValidateV5 is the PRE-STRUCTURE floor-box scaffold, retained under Round 1A (P-table delta
// certification §6). It NEVER returns Accept and it reaches NO recompute: after the version gate,
// the #535 recovery decision and the pruned-block refusal it returns IndeterminateTrustlessly /
// ErrRecomputeGated unconditionally.
//
// THE DOOR IS (*Box).Validate (floorbox_box_v5.go), NOT this function. Do NOT build the trustless
// recompute here. This signature — a bare parentStateRoot PARAMETER and no head record of its own
// — is the RT2-CARRIER-13 shape: a recompute entry with no position lets the block's author choose
// the parent everything downstream is verified against (the carrier over b.Prev, the class-A
// fold's excluded proposer). Round 1A closed that by deriving the head from a parent BLOCK the box
// holds (NewBox → HeadRef) and binding (b.Prev, b.Height) to it at P1 before any other read; the
// recompute is reached only through P13 of the ONE composition (ValidateCommitV5 over provenView).
// A caller that wants a trustless verdict constructs a Box.
//
// What stays here, and why: the version gate (a sub-v5 block is Reject, ErrNotWitnessableVersion),
// the #535 recovery-boundary stall, and the pruned-block refusal — all three of which the door also
// performs, in the same order, so "the box refuses X" is true of the BOX and not of one of its two
// exported entries. parentStateRoot is accepted and ignored so the exported signature stays stable;
// it is NOT a seam for the recompute (M-1A-2, R-SECOND-DOOR-COMMENT).
//
// THE RE-ANCHOR CONTRACT — the four clauses of the certification's §2.3, which the box's operator
// (the S7 driver) is held to. It is the Ethereum weak-subjectivity checkpoint schema, which silt
// already ships as -ws-checkpoint HEIGHT:HASH (cmd/silt/daemon.go), so this is not a new trust
// class:
//
//  1. At an ambiguous recovery boundary the box's role is COLD AUDITOR: it stalls, unconditionally
//     and loudly, and never trusts the proposer.
//  2. Recovery is an OPERATOR ACTION, out of band: supply a fresh H+1:HASH pin over the existing
//     -ws-checkpoint channel.
//  3. An UNREACHABLE pin is a CRITICAL AND IRRECOVERABLE FAILURE. The box must not silently
//     degrade to indeterminate-and-keep-going. This is the clause silt had not written down.
//  4. The re-anchor is a RESTART, not a new mechanism: the box discards its derived state and
//     cold-starts from H+1, which is what checkpoint sync is.
//
// The pin binds StateRoot only for a NON-PRUNED block (certification §2.4): Hash() short-circuits
// on a pruned block and returns a stored token bound to no struct field at all, StateRoot included.
// That is why the pruned refusal below, and NewBox's refusal of a pruned parent, are part of the
// same decision and not a separate hardening.
func (c *Chain) WitnessValidateV5(b Block, parentStateRoot [32]byte) (FloorBoxOutcome, error) {
	// (1) Version gate — v5-only mode.
	if b.Version < BlockVersionWitnessable {
		return Reject, ErrNotWitnessableVersion
	}

	// (2) #535 recovery-boundary decision, FIRST. At an ambiguous boundary the box stalls loudly
	// here, never trusting the proposer, never reaching the recompute seam.
	if proceed, reason := c.recoveryBoundaryDecision(b.Height); !proceed {
		return IndeterminateTrustlessly, reason
	}

	// (3) A pruned block's Hash() is a linkage token, not a commitment: it binds no StateRoot, so
	// nothing downstream could be anchored to it. The box refuses rather than taking a trust floor
	// from its caller (certification §2.5 — a raised floor makes the box skip proof verification).
	if b.IsPruned() {
		return IndeterminateTrustlessly, ErrPrunedBlockUnreproducible
	}

	// (4) NO recompute here. The recompute runs behind P1 in the ONE composition, reached only
	// through (*Box).Validate — a function with a bare parent-root parameter has no position
	// of its own and must not verify anything against it (the doc comment above). Never Accept.
	_ = parentStateRoot // ignored; kept so the exported signature is stable. Not a seam.
	return IndeterminateTrustlessly, ErrRecomputeGated
}
