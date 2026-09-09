package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"reflect"

	"github.com/nerolabs/silt/core/statehash"
)

// era-4 (v5) THE BOX — the trustless floor box's own state and its one door onto the ONE accept
// composition (floor-box structure round 1A, main-only; owner call 16).
//
// WHAT IS BOX-OWNED, AND WHY IT IS A TYPE. Four of the box-entry round's defeats (N2, N3, N6, N7)
// were a driver-supplied FIELD that removed a check; RT2-CARRIER-13/13b/13c was a recompute entry
// with no position of its own, so the carrier was verified over whatever b.Prev the block's author
// chose; R-CARRIER-PARENTPROPOSER was the one class-A input read from the witness, anchored by
// "some key signed b.Prev", which a fresh keypair satisfies. Every one of those inputs is now a
// field of this struct, DERIVED at construction from things the box holds — its own config, its
// own parent block — and none is a parameter of Validate:
//
//	head    HeadRef      derived from the PARENT BLOCK the box holds (BG-2: the view takes a block,
//	                     never a bare hash); P1 binds b.Prev / b.Height to it before anything else
//	                     reads the block, and the class-A fold takes the parent proposer from it.
//	budget  Budget       derived from BoxConfig.BudgetBytes; NewBox REFUSES an unset ceiling and
//	                     there is no way to express ∞ (BG-3, M-4).
//	src     WitnessSource the delivery seam; nil is a legal, never-Accepting box.
//
// WHAT WAS DELETED AT D0 (owner call 2, D-TRUE-UP-CALLS-2026-09-07): BoxConfig.Recovery and the
// RecoveryDirective it carried. A recovery directive was a knob the recompute cannot honour, and a
// box that could be TOLD to proceed past an ambiguous boundary is not a cold auditor. The stall is
// unconditional now, so there is nothing left to configure.
//
// WHAT IS NOT HERE (1B, blocked on the box-entry round; build brief "Do NOT touch"): pin adoption
// (AdoptPin / PinAdoptionInput), the held height and lineage (N2/N3/N6/N7). This box holds ONE
// parent block, given at construction. Whether that parent is TRUSTED is the pin question, which
// this round does not take; what this round closes is that everything downstream of the parent is
// bound to it by the box, not by the driver.
//
// THE DOOR NEVER RETURNS Accept — AND THAT IS A PROPERTY OF (*Box).Validate, NOT OF THE
// COMPOSITION. ValidateCommitV5 over provenView DOES return Accept for an honest block with
// genuine witnesses (TestBoxDoor_HonestBlockReachesTheDowngrade drives it there); the one-line
// downgrade in Validate — Accept ⇒ IndeterminateTrustlessly / ErrRecomputeGated — is the ONLY
// thing between the door and Accept, because flipping the box to Accept is R1.8, a consensus-rule
// change (I1), owner-ratified and research-gated, and not this round. One line, so the flip is one
// line to remove and reviewed on its own. Belt: a nil source stalls at the first class-2 read.
// What keeps the composition from being a SECOND door around this downgrade is that StateView is
// SEALED (stateview_v5.go): only liveView and provenView can drive ValidateCommitV5.

var (
	// ErrBoxLegacyMode is NewBox's refusal of a chain in the LEGACY (non-objective) regime, or one
	// whose bond verifier is not wired (#572 — objective() is MinBond > 0 AND verifyBond != nil).
	// A legacy box cannot reproduce any qualification predicate from committed state: rep(id) is
	// not a leaf. This is the S2 mode fence, at the box entry (P-table delta certification §2.2
	// row 0a: box-owned, node no-op), belt and braces with provenView.Rep answering NoWitness.
	ErrBoxLegacyMode = errors.New("chain: floor box requires the OBJECTIVE regime (MinBond > 0 and a wired bond verifier); legacy rep(id) is not a committed leaf — refused")
	// ErrBoxParentUnsigned is NewBox's refusal of a parent block whose proposer signature does not
	// verify over its own hash. The head record is derived from the parent; a parent that is not
	// even self-consistent cannot anchor anything.
	ErrBoxParentUnsigned = errors.New("chain: floor box parent block's proposer signature does not verify over its hash — refused")
	// ErrBoxParentPruned is NewBox's refusal of a PRUNED parent: its Hash() is a stored token, not
	// a content digest (chain.go Hash()), so a head record derived from it would bind b.Prev to a
	// hash the parent's body no longer reproduces.
	ErrBoxParentPruned = errors.New("chain: floor box parent block is pruned; its hash is a linkage token, not a commitment — refused")
	// ErrPrunedBlockUnreproducible is the door's STALL on a payload-pruned block. validateBondRegs'
	// pruned leg is keyed on the READER's own trust floor (Q2); a box deriving that verdict from a
	// floor the node does not share could accept where the node rejects. Placed at the door, not in
	// the composition, so the composition stays the node's rule.
	ErrPrunedBlockUnreproducible = errors.New("chain: floor box cannot reproduce the pruned-block leg (it is keyed on the reader's own trust floor) — stall")
)

// BoxConfig is the box's OWN operator configuration — set once at construction, never per block.
type BoxConfig struct {
	// BudgetBytes is the BG-3 ceiling over frame + witness bytes for one block. It MUST be positive:
	// NewBox refuses 0 (unset) and there is no value that means "unlimited". The number a
	// deployment should use is not fixed here (the pony measurement is owed, build-plan cert §6.3).
	BudgetBytes int
}

// Box is the trustless floor box: its config-bearing chain (never a state source), its derived
// head record, its derived budget, and its witness-delivery seam.
type Box struct {
	c      *Chain
	head   HeadRef
	budget Budget
	src    WitnessSource
}

// NewBox constructs a box over `ch` (its own config + injected bond verifier; NOT a state source),
// bound to `parent` — the block the box holds as its head — with a ceiling derived from cfg and a
// witness-delivery source (nil = never-Accepting). It refuses a legacy/unwired chain, an unset
// budget, a pruned parent and a parent whose proposer signature does not verify.
func NewBox(ch *Chain, parent Block, cfg BoxConfig, src WitnessSource) (*Box, error) {
	if !ch.objective() {
		return nil, ErrBoxLegacyMode
	}
	bud, err := ByteBudget(cfg.BudgetBytes)
	if err != nil {
		return nil, fmt.Errorf("%w (BoxConfig.BudgetBytes is unset)", err)
	}
	if parent.IsPruned() {
		return nil, ErrBoxParentPruned
	}
	if len(parent.Proposer) != ed25519.PublicKeySize {
		return nil, ErrBoxParentUnsigned
	}
	ph := parent.Hash()
	if !ed25519.Verify(ed25519.PublicKey(parent.Proposer), ph[:], parent.ProposerSig) {
		return nil, ErrBoxParentUnsigned
	}
	return &Box{c: ch, head: headRefOf(parent), budget: bud, src: src}, nil
}

// headRefOf derives the box's head record from the parent block it holds — the SAME derivation
// liveView.Head() applies to the node's own head block, so the two views cannot differ by a field.
func headRefOf(parent Block) HeadRef {
	h := HeadRef{Hash: parent.Hash(), NextHeight: parent.Height + 1, ProposerID: parent.ProposerID()}
	if parent.StateRoot != nil {
		sr := *parent.StateRoot
		h.StateRoot = &sr
	}
	if parent.LogRoot != nil {
		lr := *parent.LogRoot
		h.LogRoot = &lr
	}
	return h
}

// Head returns the box's own head record.
func (s *Box) Head() HeadRef { return s.head }

// view builds the StateView the composition runs over. Every class-3 field is the box's own; every
// class-2 read Resolves against head.StateRoot through the source.
func (s *Box) view() provenView {
	return provenView{
		params:     liveView{s.c}.Params(),
		objective:  s.c.objective(),
		verifyBond: s.c.verifyBond,
		budget:     s.budget,
		head:       s.head,
		src:        s.src,
	}
}

// Validate is THE DOOR: the ONE accept composition (ValidateCommitV5) over the box's proven view,
// with the P13a predicate wired to the certified witness recompute. Order, load-bearing:
//
//  1. the byte budget over FRAME + WITNESS, before any crypto and before any read (BG-3);
//  2. the #535 recovery decision (cold auditor: an UNCONDITIONAL loud stall — no directive, no
//     opt-in, no fall-through; never trust the proposer);
//  3. a pruned block stalls (ErrPrunedBlockUnreproducible);
//  4. ValidateCommitV5: P1 binds (b.Prev, b.Height) to the box's OWN head FIRST — so the carrier
//     leg (P12) and the class-A fold (P13a) run over a parent the box chose, not the author;
//  5. the R1.8 downgrade: Accept ⇒ IndeterminateTrustlessly / ErrRecomputeGated.
func (s *Box) Validate(b Block, w StateRootWitness) (FloorBoxOutcome, error) {
	if err := s.budget.Check(len(Encode(&b))+witnessBytes(w), "frame+witness"); err != nil {
		return IndeterminateTrustlessly, err
	}
	if proceed, reason := s.c.recoveryBoundaryDecision(b.Height); !proceed {
		return IndeterminateTrustlessly, reason
	}
	if b.IsPruned() {
		return IndeterminateTrustlessly, ErrPrunedBlockUnreproducible
	}
	v := s.view()
	head := s.head
	v.predicate = func(blk *Block) error {
		// CommittedRoots has already rejected a nil blk.StateRoot and stalled on a nil head root.
		return s.c.recomputeStateRootEntriesRevocations(*head.StateRoot, *blk.StateRoot, *blk, w, head.ProposerID)
	}
	out, err := ValidateCommitV5(v, &b)
	if out == Accept {
		return IndeterminateTrustlessly, ErrRecomputeGated // R1.8 — the flip is not this round
	}
	return out, err
}

// witnessBytes measures a witness bundle STRUCTURALLY — every byte slice, hash, id and scalar
// reachable through the carrier's exported fields, with statehash.Witness reporting its own proof
// bytes — so the budget charges the quantity BG-3 bounds (2.67 GiB of AttScreens at N = 2^20 for
// one no-op block) without re-encoding it. O(witness), allocation-free, and it cannot drift from
// the carrier types: a new field is measured the moment it is added.
func witnessBytes(w StateRootWitness) int { return measureBytes(reflect.ValueOf(w)) }

var witnessType = reflect.TypeOf(statehash.Witness{})

func measureBytes(v reflect.Value) int {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return 0
		}
		return measureBytes(v.Elem())
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Len()
		}
		n := 0
		for i := 0; i < v.Len(); i++ {
			n += measureBytes(v.Index(i))
		}
		return n
	case reflect.Map:
		n := 0
		for it := v.MapRange(); it.Next(); {
			n += measureBytes(it.Key()) + measureBytes(it.Value())
		}
		return n
	case reflect.Struct:
		if v.Type() == witnessType {
			return v.Interface().(statehash.Witness).Bytes()
		}
		n := 0
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				continue // unexported: not a carrier field
			}
			n += measureBytes(v.Field(i))
		}
		return n
	case reflect.String:
		return v.Len()
	case reflect.Bool, reflect.Int8, reflect.Uint8:
		return 1
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64, reflect.Float64:
		return 8
	case reflect.Int32, reflect.Uint32, reflect.Float32:
		return 4
	case reflect.Int16, reflect.Uint16:
		return 2
	}
	return 0 // func and chan values carry no bytes
}
