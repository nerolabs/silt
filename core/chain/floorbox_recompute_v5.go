package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — lane-1 Part B core, increment 1.
//
// This file reproduces ONE weighted validity predicate — requireEpochWeightQuorum, the
// mature-phase >⅔ frozen-WEIGHT super-quorum (chain.go:2845) — trustlessly, from the
// committed StateRoot + witnesses ALONE, proving the C-1 weight-composition pattern the
// v5-wholeset-digest-root cert (2026-08-31) names as the load-bearing gap. It is ADDITIVE:
// it calls no full-node accept path, mutates nothing, and changes NO consensus/validity
// rule. A full node still computes requireEpochWeightQuorum from its own in-memory epochSet
// (chain.go untouched). This is a SEPARATE root-only path a semi-stateless box calls INSTEAD
// of holding the tree — the same posture floorbox_v5.go already holds.
//
// THE C-1 GAP THIS CLOSES (cert Q2 / C-1). The five F1 digest roots bind MEMBERSHIP only.
// requireEpochWeightQuorum folds Σ epochSet WEIGHT, not a membership count. So set-
// completeness alone (the digest) certifies the WRONG thing: a prover could witness the
// complete id-set yet hand FORGED per-member weights and the tally would be forgeable. The
// recompute is sound ONLY as the COMPOSITION:
//
//	digest (reconstruct epochSetRoot from the id-list ⇒ set-completeness)
//	  ∪ per-member value proof (Resolve epochSet[id] against the committed root ⇒ the weights)
//	  ∪ genesis config from OWN cfg, never the witness (C-6 ⇒ threshold un-shiftable)
//
// THE THREE-PART PROOF (recomputeEpochWeightQuorum):
//  1. SET-COMPLETENESS: reconstruct nodeSetMTH(witnessedIDs); require it equals the committed
//     epochSetRoot leaf (proven present against the StateRoot by an SMT inclusion proof). One
//     omitted frozen member ⇒ a different MTH ⇒ mismatch ⇒ stall. This is the F1 epochSetRoot
//     digest FINALLY READ (F1 committed it inert; this increment consumes it for epochSet).
//  2. PER-MEMBER WEIGHT (C-1): for EVERY id in the reconstructed set, Resolve the epochSet[id]
//     value leaf against the committed StateRoot. A forged weight fails smt.VerifyProof ⇒
//     NoWitness ⇒ stall. The digest gave membership; the inclusion proofs give the values.
//  3. GENESIS CONFIG (C-6): MinBond is read from the box's OWN cfg (c.cfg.MinBond), NEVER from
//     any witness. The eligibility screen a box applies before folding a member into the tally
//     is threshold-shifting if an attacker controls it; reading own config forecloses the shift.
//
// Then the fold + threshold, byte-for-byte the full node's (chain.go:2850-2864):
// total = Σ verified weights; support = proposer + Σ seen verified weights; 3*support > 2*total.
//
// STOP BOUNDARY (this increment). It reproduces ONE predicate. It does NOT flip #657
// the box to Accept — that is the final increment, only after ALL predicates are
// reproduced. The box STILL never-Accepts. It reproduces the NON-boundary epochSet fold only;
// the #535 recovery boundary (effectiveEpochSet = liveQualifiedSet) stays the ratified trust-
// the-directive carve-out (cert C-2), governed by the #535 policy floorbox_v5.go already ships.

var (
	// ErrRecomputeSetIncomplete marks a stall where the witnessed id-list does not reconstruct
	// the committed epochSetRoot: a member was omitted (or an extra injected), so the MTH over
	// the witnessed list differs from the committed digest. The box CANNOT trust an incomplete
	// set to fold a whole-set weight quorum — it stalls, never folds a short set.
	ErrRecomputeSetIncomplete = errors.New("chain: floor-box recompute — witnessed epochSet id-list does not reconstruct the committed epochSetRoot (a member was omitted or injected)")

	// ErrRecomputeDigestRootUnproven marks a stall where the committed epochSetRoot leaf itself
	// could not be proven present against the committed StateRoot (no/failed inclusion witness).
	// Without the committed digest the box has nothing to compare the reconstructed MTH to.
	ErrRecomputeDigestRootUnproven = errors.New("chain: floor-box recompute — committed epochSetRoot leaf not proven present against the committed StateRoot")

	// ErrRecomputeMemberWeightUnproven marks a stall where a per-member epochSet[id] weight leaf
	// could not be proven present against the committed StateRoot (no/failed/forged inclusion
	// witness). This is the C-1 closure: a forged weight cannot verify, so it stalls the fold
	// rather than letting a forgeable tally through.
	ErrRecomputeMemberWeightUnproven = errors.New("chain: floor-box recompute — a per-member epochSet weight leaf not proven present against the committed StateRoot (C-1: the weight is forged or missing)")

	// ErrRecomputeAuthorSlashed is the N1 refusal: the block's AUTHOR is proven slashed in the
	// committed state. The node refuses such a block at proposerQualifiedAt (P4, chain.go) BEFORE
	// its weight tally ever runs; a standalone reproduction of the tally must refuse it too, or it
	// credits an author the node never lets in.
	ErrRecomputeAuthorSlashed = errors.New("chain: floor-box recompute — the block's author is slashed in the committed state (proposerQualifiedAt refuses it before the weight tally) — refused")
	// ErrRecomputeAuthorUnscreened is the N1 stall: the author's slashed[id] read could not be
	// anchored against the committed StateRoot (no/failed proof). A stall, never a credit.
	ErrRecomputeAuthorUnscreened = errors.New("chain: floor-box recompute — the author's slashed[id] pre-state is not proven either way against the committed StateRoot — stall")
)

// EpochSetWitness is the witnessed input a floor box supplies to reproduce
// requireEpochWeightQuorum: the claimed COMPLETE id-list of the frozen epochSet, the SMT
// inclusion proof of the committed epochSetRoot digest leaf, and one per-member inclusion
// proof + claimed weight for every id in the list. It is UNTRUSTED input from an any-of-N
// provider; every field becomes meaningful only after verification against the committed
// StateRoot (set-completeness for the id-list, Resolve for each proof).
type EpochSetWitness struct {
	// IDs is the id-list the prover claims is the COMPLETE frozen epochSet. Completeness is not
	// trusted: the recompute reconstructs nodeSetMTH(IDs) and compares it to the committed
	// epochSetRoot. A short (member-omitted) or padded (member-injected) list yields a different
	// MTH and stalls. Order does not matter — nodeSetMTH canonically sorts.
	IDs []ports.NodeID

	// DigestRootWitness is the SMT inclusion proof of the committed epochSetRoot leaf
	// (Key(tagEpochSetRoot, nil) → the committed MTH value) against the committed StateRoot. The
	// recompute proves the committed digest present, then compares it to the reconstructed MTH.
	DigestRootWitness statehash.Witness

	// DigestRootValue is the committed epochSetRoot leaf value the DigestRootWitness proves. It
	// is the MTH the recompute compares nodeSetMTH(IDs) against; a mismatch is set-incompleteness.
	DigestRootValue []byte

	// MemberWeights maps each id in IDs to its claimed per-member epochSet weight and the SMT
	// inclusion proof of the epochSet[id] value leaf. The recompute Resolves each against the
	// committed StateRoot: a forged weight fails verification and stalls (C-1). Every id in IDs
	// MUST have an entry, else the recompute cannot verify that member's weight and stalls.
	MemberWeights map[ports.NodeID]MemberWeightWitness

	// AuthorSlashedProof anchors the AUTHOR's slashed[proposer] pre-state against the committed
	// StateRoot: a non-membership proof (the honest case — the author is credited) or an inclusion
	// proof of slashed||proposer → Present (the author is refused, ErrRecomputeAuthorSlashed). A
	// nil or failed proof STALLS (ErrRecomputeAuthorUnscreened) — never a credit by default.
	// This is the N1 screen; see recomputeEpochWeightQuorum for why the tally needs it here and
	// the node's tally does not.
	AuthorSlashedProof statehash.Witness
}

// MemberWeightWitness is one member's claimed epochSet weight plus the SMT inclusion proof of
// its epochSet[id] value leaf against the committed StateRoot.
type MemberWeightWitness struct {
	// Weight is the claimed committed epochSet[id] weight. It is NOT trusted: the recompute
	// verifies it by Resolving the epochSet[id] leaf (encoded as EncodeInt64(Weight)) against the
	// committed root — a forged weight produces a leaf value the committed root does not commit,
	// so smt.VerifyProof fails and the member's weight is unproven (C-1).
	Weight int64

	// Proof is the SMT inclusion proof of Key(tagEpochSet, id) → EncodeInt64(Weight) against the
	// committed StateRoot.
	Proof statehash.Witness
}

// recomputeEpochWeightQuorum reproduces requireEpochWeightQuorum (the mature-phase >⅔
// frozen-WEIGHT super-quorum, chain.go:2845) TRUSTLESSLY, from the committed StateRoot + the
// witness alone. It returns (met, nil) where met is the quorum verdict a full node's
// requireEpochWeightQuorum would produce (met == the err==nil case), or (false, reason) when
// the box cannot verify the witness and must stall — NEVER folding an unverified set/weight.
//
// It reads MinBond from the box's OWN cfg (C-6), never the witness. It reproduces the
// NON-boundary epochSet fold; the #535 recovery boundary is the ratified carve-out (cert C-2)
// governed by floorbox_v5.go's policy, out of scope here.
//
// N1 — THE AUTHOR SCREEN, and why it is HERE and not in the node's tally. The node's
// requireEpochWeightQuorum credits `set[proposer]` with NO screen, and that is correct on the
// node, because proposerQualifiedAt (P4, chain.go) has already refused a slashed author before
// the tally runs. A standalone reproduction of the tally has no P4 in front of it, so it must
// screen the author itself — on the ANCHORED committed slashed[proposer] leaf — or it credits an
// author the node never lets in (N1: a live wrong-accept with every witness proof passing). The
// composition (validate_v5.go) does not need this: there P4 precedes the tally, in the node's
// own order.
//
// Unexported (round 1A, step 9): the box's door is (*Box).Validate; a direct caller of a single
// predicate is a second door with no P1–P4 in front of it.
func (c *Chain) recomputeEpochWeightQuorum(
	committedStateRoot ports.Hash,
	proposer ports.NodeID,
	seen map[ports.NodeID]bool,
	w EpochSetWitness,
) (met bool, reason error) {
	// (1) SET-COMPLETENESS. Prove the committed epochSetRoot leaf present against the committed
	// StateRoot, then require the reconstructed MTH over the witnessed id-list equals it. One
	// omitted (or injected) member yields a different MTH ⇒ mismatch ⇒ stall.
	digestKey := statehash.Key(tagEpochSetRoot, nil)
	digestRes := statehash.Resolve(committedStateRoot, digestKey, w.DigestRootValue, w.DigestRootWitness)
	if !digestRes.IsProvenPresent() {
		return false, ErrRecomputeDigestRootUnproven
	}
	reconstructed := nodeSetMTH(w.IDs)
	if !bytes.Equal(reconstructed, w.DigestRootValue) {
		return false, fmt.Errorf("%w: reconstructed MTH %x != committed epochSetRoot %x",
			ErrRecomputeSetIncomplete, reconstructed, w.DigestRootValue)
	}

	// (1b) THE AUTHOR SCREEN (N1). Anchor slashed[proposer] against the committed StateRoot
	// BEFORE anything is credited. Proven absent ⇒ the author is credited below, exactly as the
	// node's tally does after proposerQualifiedAt admitted it. Proven present ⇒ REFUSE by name:
	// the node refuses this block at proposerQualifiedAt (chain.go), the stage that screens, and
	// a slashed-but-frozen author is precisely the case where the tally alone gets it wrong (the
	// author stays in the frozen epochSet, so its weight is still in `set`). Anything else ⇒ stall.
	authorKey := statehash.Key(tagSlashed, proposer[:])
	authorRes := statehash.Resolve(committedStateRoot, authorKey, nil, w.AuthorSlashedProof)
	if !authorRes.IsProvenAbsent() {
		if statehash.Resolve(committedStateRoot, authorKey, statehash.Present, w.AuthorSlashedProof).IsProvenPresent() {
			return false, fmt.Errorf("%w: author %x", ErrRecomputeAuthorSlashed, proposer[:])
		}
		return false, fmt.Errorf("%w: author %x", ErrRecomputeAuthorUnscreened, proposer[:])
	}

	// (2) PER-MEMBER WEIGHT (C-1) + (4) THE FOLD. For every id in the now-completeness-verified
	// set, Resolve its epochSet[id] weight leaf against the committed StateRoot (a forged weight
	// fails verification ⇒ stall), then fold exactly as chain.go:2850-2864: total = Σ verified
	// weights, support = proposer + Σ seen verified weights. The full node does NOT re-screen
	// epochSet members by MinBond in this fold — MinBond screened them at FREEZE time
	// (liveQualifiedSet, chain.go:1352), so every epochSet member already cleared it and
	// contributes its full weight. Re-screening here would DIVERGE from the full node, so the
	// recompute must NOT. The ONE screen the tally carries is the author's, above, and it exists
	// because proposerQualifiedAt is the stage that screens on the node and is not in front of
	// this function. See the C-6 note below for how this increment still reads own config.
	var total, support int64
	for _, id := range w.IDs {
		mw, ok := w.MemberWeights[id]
		if !ok {
			// A member in the completeness-verified set has no weight witness: the box cannot
			// verify its weight, so it cannot fold the set. Stall, never fold a partial set.
			return false, fmt.Errorf("%w: id %x has no weight witness", ErrRecomputeMemberWeightUnproven, id[:])
		}
		memberKey := statehash.Key(tagEpochSet, id[:])
		res := statehash.Resolve(committedStateRoot, memberKey, statehash.EncodeInt64(mw.Weight), mw.Proof)
		if !res.IsProvenPresent() {
			// C-1: a forged weight produces a leaf value the committed root does not commit, so
			// the inclusion proof fails to verify. Stall — the tally would otherwise be forgeable.
			return false, fmt.Errorf("%w: id %x weight %d", ErrRecomputeMemberWeightUnproven, id[:], mw.Weight)
		}
		total += mw.Weight
		if id == proposer || seen[id] {
			support += mw.Weight
		}
	}

	// (3) THE FROZEN-WEIGHT ⅔ THRESHOLD, byte-for-byte the full node's (chain.go:2854-2864).
	//
	// C-6 (genesis config from OWN config, never the witness). The ⅔ ratio itself is a fixed
	// consensus constant, not a genesis knob, so THIS predicate's fold reads no per-deployment
	// config value — there is nothing here an attacker could shift via the witness. The C-6
	// obligation is nonetheless exercised, not skipped: the recompute reads the box's OWN
	// consensus parameters (via c) and treats the WITNESS as carrying committed STATE only
	// (id-list + per-member weights), never a threshold. A predicate that DID read a genesis
	// knob (e.g. a whole-bonded fold that screens by MinBond, a later increment) reads it from
	// c.cfg here, exactly as liveQualifiedSet does (chain.go:1352) — never from w. The C-6
	// ablation asserts this separation: a witness that tries to carry a shifted threshold cannot
	// move the verdict, because the recompute takes no threshold from w.
	if total <= 0 {
		return true, nil // degenerate/trusted: no frozen weight to measure a super-quorum against.
	}
	return 3*support > 2*total, nil
}
