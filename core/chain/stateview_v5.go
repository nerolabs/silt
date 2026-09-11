package chain

import (
	"crypto/rsa"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) STATE VIEW — the read surface the ONE accept composition (ValidateProposalV5 /
// ValidateCommitV5, validate_v5.go) runs over.
//
// WHY THIS TYPE EXISTS. Every floor-box defeat in the 2026-09 cycle was a COMPOSITION error: the
// box reproduced a node predicate's TAIL without the precondition a DIFFERENT validation stage had
// already established (the author screen behind requireEpochWeightQuorum; the (b.Prev, b.Height)
// anchor behind validateCarrier). A shared predicate SET cannot close that class — only a shared
// PATH can. So the accept path is written ONCE, against this interface, and the callers differ only
// in which view they hand it:
//
//	node : ValidateProposalV5(liveView{c}, b) / ValidateCommitV5(liveView{c}, b)
//	       — every accessor returns Present (stateview_live_v5.go)
//	box  : ValidateCommitV5(provenView{...}, b)
//	       — class-2 accessors Resolve against a committed root (stateview_proven_v5.go)
//
// Certified: build-plan certification §2 (the read set, classified) and the P-table delta
// certification (M-1 Rep, M-3 HeadRef, M-4 Budget + the two-conjunct substituted step):
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md
//
// THE CLASSES (build-plan cert §2.2). Every state read reachable from ValidateCommit lands in one:
//
//	class 2  committed leaf   — witnessable; read through a three-valued accessor below
//	class 3  box-owned        — own config, own head record, own trust floor, the injected verifyBond
//	class 4  a fact neither hash-covered nor Resolvable — THE SET IS EMPTY (the parent hash cannot
//	         be a leaf: StateRoot is INSIDE Hash()'s preimage, so no leaf written by apply(P) can
//	         commit P's own hash; it is class 3, HeadRef.Hash)
//	LEGACY   rep(id) — not a committed leaf. M-1: the composition reads it through Rep(); the node's
//	         liveView answers it, the box's provenView answers NoWitness and stalls.

// Availability is the three-valued availability of a committed read.
//
// NoWitness IS THE ZERO VALUE, and that is the whole point (C-7 §104, promoted from per-call-site
// discipline to a type property): a forgotten field, a nil map entry and an un-populated struct all
// read as "I cannot see this" — a STALL — never as "absent". A view accessor that returns
// (zeroValue, zeroAvailability) by accident refuses; it never accepts.
type Availability uint8

const (
	// NoWitness: the view cannot see this read. The composition STALLS
	// (IndeterminateTrustlessly) — it never treats it as absent, and never as a default value.
	NoWitness Availability = iota
	// Present: the read resolved to a committed value (or, for a set, a complete set).
	Present
	// ProvenAbsent: the read is proven ABSENT against the committed root — a positive fact,
	// distinct from "I have no witness".
	ProvenAbsent
)

func (a Availability) String() string {
	switch a {
	case Present:
		return "PRESENT"
	case ProvenAbsent:
		return "PROVEN_ABSENT"
	default:
		return "NO_WITNESS"
	}
}

// HeadRef is the view's OWN head record: the one atomic record the composition binds a block's
// parent against (P1) and the substituted step reads the parent's committed roots from (P13).
// CLASS 3 — box-owned, DERIVED from a block the view holds and has validated, NEVER declared by a
// driver (BG-2). Certified shape: P-table delta certification §3.1 (M-3).
//
// A DRIVER-SUPPLIED parent hash is a class-4 input — it lets the caller choose which chain the box
// is on, and it REMOVES a check — while the view's own head record cannot be chosen by the block's
// author. The fields move together, atomically, or not at all; there is no "I hold a head" boolean,
// because a bool is a caller's claim and a record is a fact.
type HeadRef struct {
	// Hash is the parent's Hash() — Chain.Head()'s first return.
	Hash ports.Hash
	// ChainID is the view's own NETWORK IDENTITY: the height-0 block's Hash() (Chain.ChainID).
	// It is what the era-4 consensus-signature preimage binds (consensusSigBytesV5), so that a
	// signature made on one silt network can never be read as a vote on another.
	//
	// CLASS 3 AND STRICTLY SO (BG-2). A DRIVER-SUPPLIED chain id lets the caller choose which
	// network the box believes it is on — the class-4 shape this type exists to prevent — so it
	// is derived, like the other fields, from what the view itself holds: the node's own genesis
	// for liveView, the box's own config-bearing chain for provenView. It is NOT a witnessed
	// leaf: the v5 witness read-set, the digest set and the SMT tag set are untouched by it.
	//
	// The ZERO hash is not a chain id. verifyAtt refuses every era-4 signature form under it, so
	// an un-populated HeadRef stalls the v5 signature checks rather than verifying them against
	// "network zero" — the same property that makes NoWitness the zero of Availability.
	ChainID ports.Hash
	// NextHeight is THE HEIGHT THE NEXT BLOCK MUST CARRY (parent.Height + 1) — Chain.Head()'s
	// second return. The +1 is derived in ONE place so the two views cannot differ by one, and
	// the name carries the semantics so P1 is never written two ways.
	NextHeight uint64
	// ProposerID is the parent's ProposerID() (R-CARRIER-PARENTPROPOSER: the carrier fold's one
	// excluded id is box-owned, never witnessed).
	ProposerID ports.NodeID
	// StateRoot is the parent's committed StateRoot; nil = the parent committed none (a v5 child
	// of a sub-v4 parent). The substituted step STALLS on nil — a stall, not a divergence.
	StateRoot *ports.Hash
	// LogRoot is the parent's committed LogRoot; nil = the parent committed none. It is what the
	// P13b k = 0 leg compares against (R-LOGROOT-FORMAT-SCOPE, closed for k = 0 with zero format
	// change). Inside the signed preimage (bodyHash), so a box holding a validated parent block
	// derives it exactly as it derives StateRoot.
	LogRoot *ports.Hash
	// Empty is true iff the chain holds no block — Chain.Head() returns (zero, 0). liveView
	// reproduces that; a box refuses it at its entry (a box with no parent cannot bind one).
	Empty bool
}

// Budget is the view's OWN witness/frame BYTE budget (BG-3): ONE ceiling over the block's
// canonical frame PLUS the witness bundle the box is asked to look at. The composition checks the
// frame against it at step 0b, BEFORE any per-entry crypto; the box door checks frame + witness
// against the same ceiling before the composition runs (floorbox_box_v5.go).
//
// THE ZERO VALUE STALLS (M-4). Mirrors NoWitness being the zero of Availability: a forgotten
// budget, an un-populated struct and a driver that "forgot" to set one all read as "this view will
// pay for nothing" — never as "unlimited". Two constructors exist and there is no third way to make
// one: UnlimitedBudget() (what liveView returns — a node holds the whole state and pays no witness
// amplification) and ByteBudget(maxBytes), which refuses a non-positive ceiling. A box derives its
// ByteBudget from its own BoxConfig at construction (NewBox refuses an unset one); no call takes a
// Budget as a parameter, so there is no way to hand a box ∞ per block.
//
// WHY BYTES AND NOT A COUNT (PE ruling §4(c) defect 1). validateCarrier applies no qualification
// screen, so distinct ids are free (carrier.go: "distinctness does NOT bound the carrier … there is
// no size rule here"); a count of payload slices never reads the quantity BG-3 bounds. Measured
// exposure: 2.67 GiB of AttScreens / ~1.79 GiB of readSetAtts for ONE no-op block at registry
// N = 1,048,576, each over a 2 GB pony (build-immutable #8). The value rule (R-CARRIER-BYTES) is a
// v5 VALIDITY change — research-gated, owner-ratified, NOT this round. Until it ships the box STALLS
// above its own budget.
//
// What step 0b measures is the FRAME — the block's canonical encoded size — because no witness
// exists yet at 0b. The witness-side bytes are measured where the witness is looked at — the box
// door, which charges frame + witness against this same ceiling (witnessBytes, floorbox_box_v5.go).
// Three properties make the budget sound without its own certification:
//  1. STALL-ADDING ONLY. It can never turn a node-Reject into a box-Accept.
//  2. BOX-OWNED, never a parameter of the block. A driver-supplied ∞ REMOVES a check.
//  3. It restores O(1) time-to-stall for an out-of-scope block (RT2-CARRIER-15).
//
// The number a box should use is NOT fixed here — the pony measurement is owed (build-plan cert
// §6.3) and this round does not invent one.
type Budget struct {
	maxBytes  int
	unlimited bool
}

// ErrBudgetNotPositive is ByteBudget's refusal of a zero or negative ceiling. A zero ceiling is
// not "unlimited" and not "nothing" — it is the zero Budget, which stalls (M-4).
var ErrBudgetNotPositive = errors.New("chain: floor-box byte budget must be a positive byte count (the zero Budget stalls; use UnlimitedBudget() for a node's view)")

// UnlimitedBudget is the node's budget: it holds the whole state, so a ceiling would be a new
// node-side validity rule. It is the ONLY unlimited Budget, and only liveView returns it (G-D10).
func UnlimitedBudget() Budget { return Budget{unlimited: true} }

// ByteBudget is a box-owned ceiling on frame + witness bytes. Refuses maxBytes <= 0.
func ByteBudget(maxBytes int) (Budget, error) {
	if maxBytes <= 0 {
		return Budget{}, fmt.Errorf("%w: got %d", ErrBudgetNotPositive, maxBytes)
	}
	return Budget{maxBytes: maxBytes}, nil
}

// Unlimited reports whether this budget imposes no ceiling at all — true ONLY for UnlimitedBudget().
func (b Budget) Unlimited() bool { return b.unlimited }

// IsZero reports the zero Budget — the one that stalls.
func (b Budget) IsZero() bool { return !b.unlimited && b.maxBytes == 0 }

// MaxBytes is the ceiling (0 for the zero and the unlimited budgets; check Unlimited()).
func (b Budget) MaxBytes() int { return b.maxBytes }

// Check is THE ONE budget comparison: the zero Budget STALLS by name (M-4), the unlimited budget
// admits everything, a byte budget refuses n above its ceiling. `what` names the measured quantity
// in the refusal so the stall is attributable ("frame", "frame+witness").
func (b Budget) Check(n int, what string) error {
	if b.unlimited {
		return nil
	}
	if b.IsZero() {
		return ErrWitnessBudgetUnset
	}
	if n > b.maxBytes {
		return fmt.Errorf("%w: %s %d bytes (budget %d)", ErrWitnessBudgetExceeded, what, n, b.maxBytes)
	}
	return nil
}

// Params is the view's OWN consensus configuration (class 3, C-6). It is NEVER witnessed and
// NEVER a parameter of the block: an attacker who could shift MinBond or Quorum could move a
// threshold under the composition's feet. Config is EMBEDDED rather than re-listed so the knobs
// have exactly one definition and cannot drift from the node's — that embedding is what carries
// MinProposerRep / MinAttesterRep for the legacy leg (M-1).
//
// TokenQuorum/IssuerKey are the two injected publish-token settings (Chain.RequireTokens); they
// are configuration in every sense that matters here, so they travel with the rest.
type Params struct {
	Config
	TokenQuorum int
	IssuerKey   func(ports.NodeID) *rsa.PublicKey
}

// StateView is the read set of the v5 accept path, three-valued by TYPE.
//
// The WHOLE-SET reads are the ones the node ITERATES; a view that answers them must therefore be
// able to prove SET COMPLETENESS (on the box, the v5 whole-set digest roots), not merely
// membership of the ids it chose to show. The POINT reads are inclusion/non-inclusion. The
// SCALARS are the committed activation/latch leaves.
//
// Two rules are deliberately NOT methods, because two implementations of either is the #402 trap:
//   - effectiveEpochSet(h) lives in the COMPOSITION (validate_v5_predicates.go). Its inputs are
//     Params() plus EpochSet() and Qualified(); putting h into the view would permit a second copy
//     of the #535 substitution rule — the exact seam cert gate G-A was filed on.
//   - liveQualifiedSet() is DERIVED from Qualified() (the era-4 committed accelerator whose
//     equality with filter(bonded, slashed, MinBond) is the era-4 maintenance claim). The
//     composition reads Qualified(); it does not re-filter Bonded() in a second place.
type StateView interface {
	// ---- class 3: own config and own capability. Never witnessed, never a parameter. ----
	Params() Params
	Objective() bool
	VerifyBond(pub []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool
	WitnessBudget() Budget
	// PrunedTolerated answers the Q2 pruned-tolerance QUESTION for height h — "may a payload-pruned
	// block at h be trusted without re-verifying its space-time proofs?" — and never the floor it is
	// derived from. That is deliberate, and it is D0's fourth deliverable (owner call 2; the
	// certification's H-4, which REFUTED its own first draft to get here).
	//
	// A floor is not a benign contract parameter. chain.go's pruned leg SKIPS the space-time
	// re-verify for a block strictly below the floor, so a caller who supplies a RAISED floor makes
	// the reader skip proof verification for everything under it — i.e. accept forged bonded
	// standing. A floor VALUE on this interface is a wrong-accept vector; the question is not.
	//
	// The node's liveView answers (h < c.trustFloor(), Present) — its own rule, unchanged. The box's
	// provenView answers (false, NoWitness) and the composition STALLS: a box cannot audit history
	// below any node's prune floor, which is a held liveness residual, not a defect.
	PrunedTolerated(h uint64) (bool, Availability)

	// ---- class 3: position. No Availability — a view WITHOUT a head cannot be constructed. ----
	Head() HeadRef
	// Ancestors returns up to k parent-linked hashes ending at Head().Hash, most recent first —
	// the bounded header chain recentBondRegNonces walks (K = BondRegHeadWindow). Each hash is
	// authenticated by the Prev linkage of the one before it, so this is self-authenticating and
	// NOT a class-4 fact. NoWitness ⇒ the composition stalls on any block carrying BondRegs.
	Ancestors(k int) ([]ports.Hash, Availability)

	// ---- LEGACY (M-1): the local reputation view. Not a committed leaf. ----
	// The node's liveView answers (c.rep(id), Present); the box's provenView answers
	// (0, NoWitness) and the composition stalls — which is what the box-entry S2 fence achieves,
	// belt and braces. Read ONLY on the !Objective() branch of P4 and P9 (the node's own branch).
	Rep(id ports.NodeID) (int64, Availability)

	// ---- class 2: committed WHOLE-SET reads (the node iterates these) ----
	EpochSet() (map[ports.NodeID]int64, Availability)
	Qualified() (map[ports.NodeID]int64, Availability)
	Bonded() (map[ports.NodeID]int64, Availability)
	ValidatorsSeen() (map[ports.NodeID]struct{}, Availability)
	SlashedSet() (map[ports.NodeID]struct{}, Availability)

	// ---- class 2: committed POINT reads (inclusion / non-inclusion) ----
	Slashed(id ports.NodeID) (bool, Availability)
	BondedOf(id ports.NodeID) (int64, Availability)
	// ByRoot is MEMBERSHIP-ONLY, and deliberately so: the committed leaf tagByRoot carries
	// statehash.Present, not the entry, so an owner-returning accessor would not be witnessable.
	// Both accept-path readers (ValidateEntry's dup-root check, validateTakedowns) ask only
	// "is this root committed?".
	ByRoot(r ports.Hash) (bool, Availability)
	Spent(serial []byte) (bool, Availability)
	Revoked(r ports.Hash) (bool, Availability)
	BondRootOwner(r ports.Hash) (ports.NodeID, Availability)
	BondRegHeight(id ports.NodeID) (uint64, Availability)
	BondDomain(id ports.NodeID) (uint64, Availability)

	// ---- class 2: committed SCALARS, in their committed leaf encoding ----
	// tags: tagEverMature, tagMatureEpoch, tagGateLockedIn, tagGateHeight, tagEra3LockedIn,
	// tagEra3Height, tagEra4LockedIn, tagEra4Height, tagEpochStart.
	Scalar(tag string) ([]byte, Availability)

	// ---- THE ONE SUBSTITUTED STEP (P13 = P13a StateRoot ∧ P13b LogRoot). Not shared, by design. ----
	//
	// The node substitutes validateEra3Roots → postApplyRoots → cloneForDryRun (a real apply on a
	// deep clone, BOTH roots); the box substitutes the certified witness recompute for P13a and
	// the k = 0 LogRoot equality (k ≥ 1 STALLS until tagRevLogSize ships at R3.4) for P13b. This
	// seam stays an EQUIVALENCE WITH GATES, never a shared function — it is the half that has held
	// under four blind rounds, and collapsing it would be a new soundness claim, not a refactor.
	// R-STATEROOT-EQUIVALENCE-SEAM (to be renamed R-COMMITTEDROOTS-EQUIVALENCE-SEAM: held-in-
	// tension over TWO equalities). THIS ROUND IS NOT "one implementation everywhere" and must
	// never be published as such.
	CommittedRoots(b *Block) (FloorBoxOutcome, error)

	// sealedStateView SEALS the interface (PE ruling F-1A F-2, 2026-09-08): an unexported method
	// no type outside this package can declare, so the only views that can drive the exported
	// composition are liveView (the node) and provenView (the box). Without it, an out-of-package
	// caller could implement StateView, answer Accept from CommittedRoots, and take Accept out of
	// ValidateCommitV5 with no (*Box).Validate downgrade in front of it — a second door opened by
	// the same round that closed nine. The inventory gate (TestG6b_ExportedPackageSurfaceInventory)
	// lists every exported entry of the box files with the reason each is permitted.
	sealedStateView()
}
