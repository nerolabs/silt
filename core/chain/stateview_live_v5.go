package chain

import (
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// liveView is the FULL NODE's StateView: the adapter that lets a node run the ONE accept
// composition (ValidateProposalV5 / ValidateCommitV5) over its own live committed state.
//
// EVERY ACCESSOR RETURNS Present. A full node holds the whole state, so there is no read it
// cannot see. That fact is a DIAGNOSTIC, never a premise: the dispatch in chain.go maps
// IndeterminateTrustlessly to a refusal on the node, so a bug in THIS adapter costs a refusal,
// never an acceptance. G-5 (TestG5_LiveViewNeverAnswersNoWitness) pins the diagnostic directly.
//
// It is a value type wrapping *Chain, constructed at the dispatch site. It holds no state of its
// own and mutates nothing.
type liveView struct{ c *Chain }

var _ StateView = liveView{}

func (liveView) sealedStateView() {}

// ---- class 3: own config / capability ----

func (v liveView) Params() Params {
	return Params{Config: v.c.cfg, TokenQuorum: v.c.tokenQuorum, IssuerKey: v.c.issuerKey}
}

func (v liveView) Objective() bool { return v.c.objective() }

func (v liveView) VerifyBond(pub []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool {
	if v.c.verifyBond == nil {
		return false // unreachable behind Objective(), which requires a wired verifier (#572)
	}
	return v.c.verifyBond(pub, root, size, nonce, answer)
}

// WitnessBudget is UNLIMITED on a node: it holds the state, so it pays no witness amplification
// and a BG-3 ceiling here would be a new node-side validity rule. This is the ONLY view that
// returns UnlimitedBudget() (M-4, G-D10).
func (v liveView) WitnessBudget() Budget { return UnlimitedBudget() }

func (v liveView) TrustFloor() uint64 { return v.c.trustFloor() }

// ---- class 3: position ----

// Head reproduces Chain.Head() exactly — (zero, 0) on an empty chain, else (last.Hash(),
// last.Height + 1) — and adds the parent's proposer and its committed roots by pointer, so a
// parent that committed no root reads as nil, never as a zero hash.
func (v liveView) Head() HeadRef {
	prev, next := v.c.Head()
	h := HeadRef{Hash: prev, NextHeight: next}
	n := len(v.c.blocks)
	if n == 0 {
		h.Empty = true
		return h
	}
	last := v.c.blocks[n-1]
	h.ProposerID = last.ProposerID()
	if last.StateRoot != nil {
		sr := *last.StateRoot
		h.StateRoot = &sr
	}
	if last.LogRoot != nil {
		lr := *last.LogRoot
		h.LogRoot = &lr
	}
	return h
}

// Ancestors is the header window recentBondRegNonces walks, expressed as hashes: Head().Hash then
// its Prev-linked predecessors, ending at genesis or at a parent this chain does not hold.
func (v liveView) Ancestors(k int) ([]ports.Hash, Availability) {
	if k <= 0 {
		return nil, Present
	}
	out := make([]ports.Hash, 0, k)
	cur, _ := v.c.Head()
	for len(out) < k {
		out = append(out, cur)
		blk, ok := v.c.blockByHash(cur)
		if !ok || blk.Height == 0 {
			break // genesis or not-yet-committed parent: the window ends here
		}
		cur = blk.Prev
	}
	return out, Present
}

// ---- LEGACY (M-1) ----

// Rep is the node's local reputation view — the operand of the !objective() branch of
// proposerQualifiedAt (MinProposerRep) and attesterQualifiedAt (MinAttesterRep). Always Present
// on a node: legacy mode is an operator-reachable production regime (cmd/silt/daemon.go
// useObjective := *objective && *minRep > 0), and the composition must take that branch
// faithfully or it refuses a v5 block the node accepts today.
func (v liveView) Rep(id ports.NodeID) (int64, Availability) { return v.c.rep(id), Present }

// ---- class 2: whole-set reads ----

func (v liveView) EpochSet() (map[ports.NodeID]int64, Availability) { return v.c.epochSet, Present }

func (v liveView) Qualified() (map[ports.NodeID]int64, Availability) { return v.c.qualified, Present }

func (v liveView) Bonded() (map[ports.NodeID]int64, Availability) { return v.c.bonded, Present }

func (v liveView) ValidatorsSeen() (map[ports.NodeID]struct{}, Availability) {
	out := make(map[ports.NodeID]struct{}, len(v.c.validatorsSeen))
	for id, seen := range v.c.validatorsSeen {
		if seen {
			out[id] = struct{}{}
		}
	}
	return out, Present
}

func (v liveView) SlashedSet() (map[ports.NodeID]struct{}, Availability) {
	out := make(map[ports.NodeID]struct{}, len(v.c.slashed))
	for id, s := range v.c.slashed {
		if s {
			out[id] = struct{}{}
		}
	}
	return out, Present
}

// ---- class 2: point reads ----

func (v liveView) Slashed(id ports.NodeID) (bool, Availability) { return v.c.slashed[id], Present }

func (v liveView) BondedOf(id ports.NodeID) (int64, Availability) { return v.c.bonded[id], Present }

func (v liveView) ByRoot(r ports.Hash) (bool, Availability) {
	_, ok := v.c.byRoot[r]
	if !ok {
		return false, ProvenAbsent
	}
	return true, Present
}

func (v liveView) Spent(serial []byte) (bool, Availability) {
	return v.c.spent[string(serial)], Present
}

func (v liveView) Revoked(r ports.Hash) (bool, Availability) { return v.c.revoked[r], Present }

func (v liveView) BondRootOwner(r ports.Hash) (ports.NodeID, Availability) {
	owner, ok := v.c.bondRootOwner[r]
	if !ok {
		return ports.NodeID{}, ProvenAbsent
	}
	return owner, Present
}

func (v liveView) BondRegHeight(id ports.NodeID) (uint64, Availability) {
	h, ok := v.c.bondRegHeight[id]
	if !ok {
		return 0, ProvenAbsent
	}
	return h, Present
}

func (v liveView) BondDomain(id ports.NodeID) (uint64, Availability) {
	d, ok := v.c.bondDomain[id]
	if !ok {
		return 0, ProvenAbsent
	}
	return d, Present
}

// ---- class 2: scalars, in their committed leaf encoding ----

func (v liveView) Scalar(tag string) ([]byte, Availability) {
	switch tag {
	case tagEverMature:
		return statehash.EncodeBool(v.c.everMature), Present
	case tagMatureEpoch:
		return statehash.EncodeBool(v.c.matureEpoch), Present
	case tagGateLockedIn:
		return statehash.EncodeBool(v.c.gateLockedIn), Present
	case tagGateHeight:
		return statehash.EncodeUint64(v.c.gateHeight), Present
	case tagEra3LockedIn:
		return statehash.EncodeBool(v.c.era3LockedIn), Present
	case tagEra3Height:
		return statehash.EncodeUint64(v.c.era3Height), Present
	case tagEra4LockedIn:
		return statehash.EncodeBool(v.c.era4LockedIn), Present
	case tagEra4Height:
		return statehash.EncodeUint64(v.c.era4Height), Present
	case tagEpochStart:
		return statehash.EncodeUint64(v.c.epochStart), Present
	}
	return nil, NoWitness // an unknown tag is a programming error: STALL, never a default
}

// CommittedRoots is the node's half of the ONE substituted step (P13a ∧ P13b): the real
// clone-and-apply recompute of BOTH roots, validateEra3Roots — nil-reject, StateRoot equality,
// LogRoot equality, in the node's own order. See StateView.CommittedRoots for why this seam is an
// equivalence with gates rather than a shared function.
func (v liveView) CommittedRoots(b *Block) (FloorBoxOutcome, error) {
	if err := v.c.validateEra3Roots(b); err != nil {
		return Reject, err
	}
	return Accept, nil
}
