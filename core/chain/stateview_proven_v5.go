package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) PROVEN STATE VIEW — the trustless half of the ONE accept composition.
//
// provenView answers the composition's class-2 reads by RESOLVING each committed leaf against a
// root the answering party does not control. It is the direct analogue of go-ethereum's stateless
// execution: the witness is injected UNDERNEATH the state accessor, so a missing witness is a
// failure of the READ (Availability = NoWitness ⇒ the composition stalls), never a divergence of
// the VERDICT. There is no second implementation of the transition for a root-only client.
//
// R-VIEW-FAITHFULNESS (OPEN, and this round's named red-team target). The view CONCENTRATES the
// defect class into these accessors instead of spreading it over eleven reproduced predicates; it
// does not remove it. The shape to hunt is an accessor that answers "absent" where it should
// answer "I have no witness" — e.g. `value == nil` vs bytes.Equal(nil, ...) on an empty slice.
// Every accessor here therefore routes through resolveLeaf, which has exactly one place to get
// that wrong. M-1 and M-3 widened the surface (Rep, HeadRef.LogRoot, Empty).

// WitnessSource is the witness-DELIVERY seam. It is what a witness server (or, in a gate, a prover
// over a full node's own committed leaves) implements.
//
// IT IS NOT TRUSTED. Everything it returns is verified against the head's committed StateRoot
// before the composition sees it: point leaves by statehash.Resolve, whole sets by the RFC-6962
// MTH over the claimed id-list against the committed v5 digest root. A source that lies produces a
// stall, never an acceptance. A source that is ABSENT (nil) produces a stall for every class-2
// read, which is why a view with no wired source can never Accept.
type WitnessSource interface {
	// Leaf returns the committed value and its inclusion/exclusion proof for one leaf key.
	// ok == false means "I have no witness" — the composition stalls; it does NOT mean absent.
	Leaf(key []byte) (value []byte, w statehash.Witness, ok bool)
	// Members returns the CLAIMED COMPLETE member id-list of a whole-set keyspace. The claim is
	// checked, not believed: provenView requires nodeSetMTH(ids) to equal the committed digest-root
	// leaf for that keyspace, so one omitted or injected id yields a different MTH and stalls.
	Members(digestTag string) ([]ports.NodeID, bool)
	// Ancestors returns up to k parent-linked block hashes ending at the box's head, most recent
	// first — the bounded header window the bond-registration nonce rule walks. Self-authenticating
	// by Prev-linkage.
	Ancestors(k int) ([]ports.Hash, bool)
}

var (
	// ErrRevLogSizeUnauthenticated marks the P13b k ≥ 1 STALL on the proven view: a revocation-
	// bearing block extends the revocation log, and verify-not-recompute of the new LogRoot is
	// sound ONLY with an AUTHENTICATED parent log size m (T-LOGEXT, P-table delta certification
	// §3.3 — a witness-supplied m is a WRONG-ACCEPT through translog.VerifyConsistency's m == 0
	// short-circuit and its isPow2 seeding at m = 1). The closer is the tagRevLogSize committed
	// leaf at the era-4/v5 freeze (R3.4); until then a box stalls at the first takedown after its
	// pin. Named so the stall is attributable, never a generic "no witness".
	ErrRevLogSizeUnauthenticated = errors.New("chain: floor-box cannot verify a revocation-bearing block's LogRoot without an authenticated parent log size (tagRevLogSize, R3.4) — stall")
	// ErrHeadRootAbsent marks the substituted-step STALL when the view's head carries no committed
	// root to compare against — a v5 child of a sub-v4 parent. A stall, not a divergence: the node
	// recomputes from its own state and is unaffected.
	ErrHeadRootAbsent = errors.New("chain: floor-box head record carries no committed root for the parent (sub-v4 parent) — stall")
)

// provenView is a StateView whose class-2 reads are Resolved against a committed root.
type provenView struct {
	params     Params
	objective  bool
	verifyBond func(pub []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool
	budget     Budget
	head       HeadRef
	src        WitnessSource
	// predicate is the box's half of P13a — the certified O(payload) witness recompute of the
	// StateRoot. nil = not wired: P13a stalls with ErrRecomputeGated.
	predicate func(b *Block) error
}

var _ StateView = provenView{}

func (provenView) sealedStateView()  {}
func (v provenView) Params() Params  { return v.params }
func (v provenView) Objective() bool { return v.objective }

func (v provenView) VerifyBond(pub []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool {
	if v.verifyBond == nil {
		return false
	}
	return v.verifyBond(pub, root, size, nonce, answer)
}

// WitnessBudget returns the box's own budget. A provenView constructed with the zero Budget
// stalls at step 0b (M-4): there is no way to hand a box ∞ by omission.
func (v provenView) WitnessBudget() Budget { return v.budget }
func (v provenView) Head() HeadRef         { return v.head }

// TrustFloor is 0 for a proven view: the box entry refuses a pruned block before the composition
// runs, because a box-owned floor the node does not share is the wrong direction (build-plan
// certification §1.4 item 5).
func (v provenView) TrustFloor() uint64 { return 0 }

// Rep is the LEGACY reputation view, which is not a committed leaf: the box has no witness for
// it and the composition stalls on the legacy branch (M-1). This is the S2 mode fence expressed as
// a read, belt and braces with the fence at the box entry.
func (v provenView) Rep(ports.NodeID) (int64, Availability) { return 0, NoWitness }

func (v provenView) Ancestors(k int) ([]ports.Hash, Availability) {
	if k <= 0 {
		return nil, Present
	}
	if v.src == nil {
		return nil, NoWitness
	}
	hs, ok := v.src.Ancestors(k)
	if !ok {
		return nil, NoWitness
	}
	// Self-authentication: the window must START at the head the view already owns. Without this
	// the source could hand an unrelated chain's window and change which nonces are accepted.
	if len(hs) == 0 || hs[0] != v.head.Hash {
		return nil, NoWitness
	}
	return hs, Present
}

// stateRoot is the root every class-2 read is proven against — the PARENT's committed StateRoot.
// A head with no root (sub-v4 parent) proves nothing: every class-2 read stalls.
func (v provenView) stateRoot() (ports.Hash, bool) {
	if v.head.StateRoot == nil {
		return ports.Hash{}, false
	}
	return *v.head.StateRoot, true
}

// resolveLeaf is THE ONE place a committed read becomes three-valued. Every accessor goes through
// it, so the "absent vs no-witness" mislabel has exactly one home to be wrong in.
func (v provenView) resolveLeaf(tag string, rawKey []byte) ([]byte, Availability) {
	if v.src == nil {
		return nil, NoWitness
	}
	root, ok := v.stateRoot()
	if !ok {
		return nil, NoWitness
	}
	key := statehash.Key(tag, rawKey)
	value, w, ok := v.src.Leaf(key)
	if !ok {
		return nil, NoWitness
	}
	res := statehash.Resolve(root, key, value, w)
	switch {
	case res.IsProvenPresent():
		return res.Value(), Present
	case res.IsProvenAbsent():
		return nil, ProvenAbsent
	}
	return nil, NoWitness
}

// members resolves a whole-set keyspace COMPLETELY: the claimed id-list, checked against the
// committed v5 digest root by reconstructing the RFC-6962 MTH. An SMT lets a root-only holder prove
// inclusion of members it was GIVEN but nothing about members WITHHELD; the MTH over the full
// id-list is what closes that, so a short list stalls rather than under-counting a quorum.
func (v provenView) members(digestTag string) ([]ports.NodeID, Availability) {
	if v.src == nil {
		return nil, NoWitness
	}
	rootValue, av := v.resolveLeaf(digestTag, nil)
	if av != Present {
		return nil, NoWitness // the digest root itself must be proven present; C-4 always-emits it
	}
	ids, ok := v.src.Members(digestTag)
	if !ok {
		return nil, NoWitness
	}
	if !bytes.Equal(nodeSetMTH(ids), rootValue) {
		return nil, NoWitness // set-incompleteness: one omitted or injected id yields a different MTH
	}
	return ids, Present
}

// weightedSet is the shared shape of the three value-carrying whole sets (bonded / epochSet /
// qualified): membership proven complete by the digest root, then each member's weight Resolved
// from its own per-member leaf. ONE body, three callers.
func (v provenView) weightedSet(digestTag, memberTag string) (map[ports.NodeID]int64, Availability) {
	ids, av := v.members(digestTag)
	if av != Present {
		return nil, NoWitness
	}
	out := make(map[ports.NodeID]int64, len(ids))
	for _, id := range ids {
		raw, av := v.resolveLeaf(memberTag, id[:])
		if av != Present {
			return nil, NoWitness // a member named by the complete set with no value proof is a stall
		}
		w, ok := decodeInt64Leaf(raw)
		if !ok {
			return nil, NoWitness
		}
		out[id] = w
	}
	return out, Present
}

func (v provenView) EpochSet() (map[ports.NodeID]int64, Availability) {
	return v.weightedSet(tagEpochSetRoot, tagEpochSet)
}

func (v provenView) Qualified() (map[ports.NodeID]int64, Availability) {
	return v.weightedSet(tagQualifiedRoot, tagQualified)
}

func (v provenView) Bonded() (map[ports.NodeID]int64, Availability) {
	return v.weightedSet(tagBondedRoot, tagBonded)
}

func (v provenView) ValidatorsSeen() (map[ports.NodeID]struct{}, Availability) {
	ids, av := v.members(tagValidatorsSeenRoot)
	if av != Present {
		return nil, NoWitness
	}
	out := make(map[ports.NodeID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, Present
}

func (v provenView) SlashedSet() (map[ports.NodeID]struct{}, Availability) {
	ids, av := v.members(tagSlashedRoot)
	if av != Present {
		return nil, NoWitness
	}
	out := make(map[ports.NodeID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, Present
}

// ---- point reads. A Present marker means membership; ProvenAbsent means non-membership. ----

func (v provenView) presenceOf(tag string, rawKey []byte) (bool, Availability) {
	raw, av := v.resolveLeaf(tag, rawKey)
	switch av {
	case Present:
		return bytes.Equal(raw, statehash.Present), Present
	case ProvenAbsent:
		return false, ProvenAbsent
	}
	return false, NoWitness
}

func (v provenView) Slashed(id ports.NodeID) (bool, Availability) {
	return v.presenceOf(tagSlashed, id[:])
}

func (v provenView) ByRoot(r ports.Hash) (bool, Availability) { return v.presenceOf(tagByRoot, r[:]) }

func (v provenView) Spent(serial []byte) (bool, Availability) { return v.presenceOf(tagSpent, serial) }

func (v provenView) Revoked(r ports.Hash) (bool, Availability) {
	return v.presenceOf(tagRevoked, r[:])
}

func (v provenView) BondedOf(id ports.NodeID) (int64, Availability) {
	raw, av := v.resolveLeaf(tagBonded, id[:])
	switch av {
	case Present:
		w, ok := decodeInt64Leaf(raw)
		if !ok {
			return 0, NoWitness
		}
		return w, Present
	case ProvenAbsent:
		return 0, ProvenAbsent
	}
	return 0, NoWitness
}

func (v provenView) BondRootOwner(r ports.Hash) (ports.NodeID, Availability) {
	raw, av := v.resolveLeaf(tagBondRootOwner, r[:])
	switch av {
	case Present:
		if len(raw) != len(ports.NodeID{}) {
			return ports.NodeID{}, NoWitness
		}
		var id ports.NodeID
		copy(id[:], raw)
		return id, Present
	case ProvenAbsent:
		return ports.NodeID{}, ProvenAbsent
	}
	return ports.NodeID{}, NoWitness
}

func (v provenView) BondRegHeight(id ports.NodeID) (uint64, Availability) {
	raw, av := v.resolveLeaf(tagBondRegHeight, id[:])
	switch av {
	case Present:
		h, ok := decodeUint64Leaf(raw)
		if !ok {
			return 0, NoWitness
		}
		return h, Present
	case ProvenAbsent:
		return 0, ProvenAbsent
	}
	return 0, NoWitness
}

func (v provenView) BondDomain(id ports.NodeID) (uint64, Availability) {
	raw, av := v.resolveLeaf(tagBondDomain, id[:])
	switch av {
	case Present:
		d, ok := decodeUint64Leaf(raw)
		if !ok {
			return 0, NoWitness
		}
		return d, Present
	case ProvenAbsent:
		return 0, ProvenAbsent
	}
	return 0, NoWitness
}

// Scalar resolves a committed scalar leaf (raw key empty). A ProvenAbsent scalar is a STALL, not a
// zero: an absent latch leaf on a root that should always commit one means the box is looking at a
// root it does not understand, and defaulting it to false is how a one-way latch silently resets.
func (v provenView) Scalar(tag string) ([]byte, Availability) {
	raw, av := v.resolveLeaf(tag, nil)
	if av != Present {
		return nil, NoWitness
	}
	return raw, Present
}

// CommittedRoots is the box's half of the ONE substituted step: P13a ∧ P13b over the parent's
// committed roots the box OWNS (HeadRef), never over anything the block or a driver supplies.
//
// Order inside the conjunction: nil-reject (the node's era3validity.go:121 rule, verbatim), then
// P13b, then P13a. P13b precedes P13a on this view only because it is O(1) and needs no witness;
// the node runs them the other way round inside validateEra3Roots. Conjunction order is free.
//
// P13b (P-table delta certification §3.3):
//   - k = 0 (no revocation touches the log): require *b.LogRoot == *head.LogRoot. Zero witness,
//     zero format change. This is the conjunct whose absence was a wrong-accept on the cheapest
//     possible mutation (a forged b.LogRoot on any block).
//   - k ≥ 1: STALL with ErrRevLogSizeUnauthenticated. k counts duplicates — the LOG does not dedup
//     a revoke/un-revoke pair the way the STATE write-set does.
func (v provenView) CommittedRoots(b *Block) (FloorBoxOutcome, error) {
	if b.StateRoot == nil || b.LogRoot == nil {
		return Reject, fmt.Errorf("%w: StateRoot=%v LogRoot=%v", ErrEra3RootMissing, b.StateRoot != nil, b.LogRoot != nil)
	}
	// ---- P13b: LogRoot ----
	if v.head.LogRoot == nil {
		return IndeterminateTrustlessly, fmt.Errorf("%w: LogRoot", ErrHeadRootAbsent)
	}
	if k := len(b.Revocations) + len(b.Unrevocations); k > 0 {
		return IndeterminateTrustlessly, fmt.Errorf("%w: %d revocation-log leaves", ErrRevLogSizeUnauthenticated, k)
	}
	if *b.LogRoot != *v.head.LogRoot {
		return Reject, fmt.Errorf("%w: committed %x, parent %x (revocation-free block)", ErrEra3LogRootMismatch, *b.LogRoot, *v.head.LogRoot)
	}
	// ---- P13a: StateRoot ----
	if v.head.StateRoot == nil {
		return IndeterminateTrustlessly, fmt.Errorf("%w: StateRoot", ErrHeadRootAbsent)
	}
	if v.predicate == nil {
		return IndeterminateTrustlessly, ErrRecomputeGated
	}
	if err := v.predicate(b); err != nil {
		// The recompute returns ONE error channel for two outcomes: a proven mismatch (a positive
		// disproof, Reject) and a stall (a read it could not anchor). Only the named mismatch is a
		// Reject; everything else stays a stall, so a witness gap never renders as a disproof.
		if errors.Is(err, ErrRecomputeStateRootMismatch) {
			return Reject, err
		}
		return IndeterminateTrustlessly, err
	}
	return Accept, nil
}

// decodeInt64Leaf / decodeUint64Leaf mirror statehash.EncodeInt64 / EncodeUint64. A wrong-length
// value is NOT coerced — it is reported unusable so the caller stalls.
func decodeInt64Leaf(raw []byte) (int64, bool) {
	if len(raw) != 8 {
		return 0, false
	}
	var u uint64
	for _, c := range raw {
		u = u<<8 | uint64(c)
	}
	return int64(u), true
}

func decodeUint64Leaf(raw []byte) (uint64, bool) {
	if len(raw) != 8 {
		return 0, false
	}
	var u uint64
	for _, c := range raw {
		u = u<<8 | uint64(c)
	}
	return u, true
}
