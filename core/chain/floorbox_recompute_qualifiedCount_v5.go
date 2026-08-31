package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — lane-1 Part B core, increment 4.
//
// This file reproduces a FOURTH validity predicate — qualifiedCount (chain.go:1479), the
// distinct-qualified-validator COUNT N that sizes the count-quorum floor — trustlessly, from the
// committed StateRoot + witnesses ALONE. It replicates increments 1-3's C-1 pattern
// (floorbox_recompute_v5.go / _maturity_v5.go / _dematureQuorum_v5.go) over the WHOLE bonded map,
// and closes the `slashed`-over-bonded quorum-stack whole-set read the #664 enumeration named as
// the keyspace the earlier hand-lists OMITTED.
//
// It is ADDITIVE: it calls no full-node accept path, mutates nothing, and changes NO
// consensus/validity rule. A full node still computes qualifiedCount from its own in-memory bonded
// and slashed maps (chain.go untouched). This is a SEPARATE root-only path a semi-stateless box
// calls INSTEAD of holding the tree — the same posture the three prior increments hold.
//
// THE PREDICATE. qualifiedCount (chain.go:1479-1487) folds the WHOLE bonded map:
//
//	N = count over id ∈ bonded of ( bonded[id] >= MinBond && !slashed[id] )
//
// N is the size of the distinct-qualified validator set the count-quorum is sized against:
// validatorSetSize (chain.go:1563, the fall-through when NOT the anchor window and NOT a mature
// epoch) → RequiredQuorum (chain.go:1526-1537, the count floor) → the requireQuorumStack count leg
// (chain.go:2779). An untouched bonded member's slash drops N and can move bftThreshold(N) — the
// count floor a commit must clear (readset_v5_quorum_wholeset_test.go:108-138, the oracle world
// countFloorPoisedWorld proves the flip).
//
// THE FOUR-PART PROOF (RecomputeQualifiedCount):
//  1. SET-COMPLETENESS over BONDED: reconstruct nodeSetMTH(witnessedIDs) over the whole-bonded
//     id-list; require it equals the committed bondedRoot leaf (proven present against the
//     StateRoot). One omitted (or injected) member ⇒ a different MTH ⇒ mismatch ⇒ stall. This
//     reuses the F1 bondedRoot digest increment 3 already reads (the completeness anchor).
//  2. PER-MEMBER BONDED WEIGHT (C-1): for EVERY id in the reconstructed set, Resolve the bonded[id]
//     value leaf against the committed StateRoot. The weight is the `>= MinBond` screen operand; a
//     forged weight fails smt.VerifyProof ⇒ stall.
//  3. PER-MEMBER SLASHED BIT (C-1): for EVERY id, Resolve the slashed[id] membership leaf —
//     inclusion when claimed slashed, non-inclusion when claimed unslashed. A prover cannot silently
//     drop a slash (that would INFLATE N) nor inject one (that would DEFLATE N): the bit is verified
//     either way, exactly as the maturity increment does (floorbox_recompute_maturity_v5.go:197-210).
//     This is the `slashed`-over-bonded read this increment consumes.
//  4. OWN CONFIG (C-6): MinBond is read from the box's OWN cfg (c.cfg.MinBond), NEVER from any
//     witness. It is the eligibility screen; a lower MinBond admits cheap members and inflates N (a
//     C1-discount lever), so reading own config forecloses the shift. The C-6 ablation asserts a
//     witness-carried MinBond cannot move N.
//
// Then the count, byte-for-byte the full node's (chain.go:1479-1487):
// N = |{ id ∈ bonded : bonded[id] >= MinBond && !slashed[id] }|.
//
// WHY `slashedRoot` IS NOT READ (and stays inert). qualifiedCount iterates the BONDED domain and
// reads slashed[id] PER-MEMBER; its completeness is anchored on bondedRoot, not slashedRoot. A
// member slashed but NOT bonded is invisible to the count (the loop never reaches it), so the WHOLE
// slashed set is never folded — no predicate reads slashedRoot whole-set (grep: the only
// whole-slashed iterations are clone/restore/emit, none a predicate). So this increment reads
// bondedRoot (already non-inert, increment 3) + per-member bonded[id]/slashed[id], and adds NO new
// digest-root read. slashedRoot remains a legitimately-inert derived commitment.
//
// STOP BOUNDARY (this increment). It reproduces ONE predicate. It does NOT flip #657
// WitnessValidateV5 to Accept — that is the final increment, only after ALL predicates are
// reproduced. The box STILL never-Accepts. It reproduces the raw COUNT N (and the derived
// bftThreshold(N) count floor); the anchor-window and mature-epoch legs of RequiredQuorum are
// governed by other predicates (the launch anchor gate; requireEpochWeightQuorum, increment 1), out
// of scope here.

var (
	// ErrRecomputeQualifiedBondedSetIncomplete marks a stall where the witnessed whole-bonded id-list
	// does not reconstruct the committed bondedRoot: a member was omitted (or an extra injected), so
	// the MTH over the witnessed list differs from the committed digest. The box CANNOT trust an
	// incomplete set to count a whole-set N — it stalls, never counts a short set.
	ErrRecomputeQualifiedBondedSetIncomplete = errors.New("chain: floor-box qualified-count recompute — witnessed bonded id-list does not reconstruct the committed bondedRoot (a member was omitted or injected)")

	// ErrRecomputeQualifiedBondedRootUnproven marks a stall where the committed bondedRoot leaf itself
	// could not be proven present against the committed StateRoot (no/failed inclusion witness).
	// Without the committed digest the box has nothing to compare the reconstructed MTH to.
	ErrRecomputeQualifiedBondedRootUnproven = errors.New("chain: floor-box qualified-count recompute — committed bondedRoot leaf not proven present against the committed StateRoot")

	// ErrRecomputeQualifiedMemberStateUnproven marks a stall where a per-member committed value leaf
	// (bonded weight or slashed membership) could not be proven present/absent against the committed
	// StateRoot (no/failed/forged witness). This is the C-1 closure: a forged member value cannot
	// verify, so it stalls the count rather than letting a forgeable N through.
	ErrRecomputeQualifiedMemberStateUnproven = errors.New("chain: floor-box qualified-count recompute — a per-member committed value leaf (bonded/slashed) not proven against the committed StateRoot (C-1: forged or missing)")
)

// QualifiedMemberWitness is one bonded member's claimed committed state plus the SMT
// inclusion/non-inclusion proofs the recompute verifies against the committed StateRoot. Every
// field is UNTRUSTED until Resolve confirms it: a forged weight or slashed-bit produces a leaf
// value the committed root does not commit, so its proof fails and the member is unproven.
type QualifiedMemberWitness struct {
	// Bonded is the claimed committed bonded[id] weight — the `>= MinBond` screen operand. Verified
	// by Resolving the bonded[id] leaf (encoded EncodeInt64(Bonded)) against the committed root; a
	// forged weight fails (C-1).
	Bonded int64

	// BondedProof is the SMT inclusion proof of Key(tagBonded, id) → EncodeInt64(Bonded).
	BondedProof statehash.Witness

	// Slashed reports whether the member is claimed to be in the committed slashed set. When true,
	// SlashedProof is an inclusion proof of Key(tagSlashed, id) → Present (the member is NOT counted);
	// when false, SlashedProof is a non-inclusion proof (the member is unslashed, so it may count).
	Slashed bool

	// SlashedProof is the SMT proof of the slashed[id] membership — inclusion when Slashed,
	// non-inclusion otherwise. A prover cannot silently drop a slashed member (that would inflate N)
	// nor inject one (that would deflate N): the recompute verifies the slashed bit for every member
	// either way (C-1).
	SlashedProof statehash.Witness
}

// QualifiedCountWitness is the witnessed input a floor box supplies to reproduce qualifiedCount:
// the claimed COMPLETE id-list of the whole bonded map, the SMT inclusion proof of the committed
// bondedRoot digest leaf, and one QualifiedMemberWitness (bonded weight + slashed membership) per
// id. It is UNTRUSTED input from an any-of-N provider; every field becomes meaningful only after
// verification against the committed StateRoot.
type QualifiedCountWitness struct {
	// IDs is the id-list the prover claims is the COMPLETE bonded set. Completeness is not trusted:
	// the recompute reconstructs nodeSetMTH(IDs) and compares it to the committed bondedRoot. A short
	// (member-omitted) or padded (member-injected) list yields a different MTH and stalls. Order does
	// not matter — nodeSetMTH canonically sorts.
	IDs []ports.NodeID

	// BondedRootWitness is the SMT inclusion proof of the committed bondedRoot leaf
	// (Key(tagBondedRoot, nil) → the committed MTH value) against the committed StateRoot. The
	// recompute proves the committed digest present, then compares it to the reconstructed MTH.
	BondedRootWitness statehash.Witness

	// BondedRootValue is the committed bondedRoot leaf value the BondedRootWitness proves — the MTH
	// the recompute compares nodeSetMTH(IDs) against; a mismatch is set-incompleteness.
	BondedRootValue []byte

	// Members maps each id in IDs to its committed-state witness (bonded weight + slashed membership).
	// Every id in IDs MUST have an entry, else the recompute cannot verify that member's state and
	// stalls.
	Members map[ports.NodeID]QualifiedMemberWitness
}

// RecomputeQualifiedCount reproduces qualifiedCount (the distinct-qualified validator COUNT N,
// chain.go:1479) TRUSTLESSLY, from the committed StateRoot + the witness alone. It returns (n, nil)
// where n == qualifiedCount()'s value a full node would produce over the committed bonded/slashed
// maps, or (0, reason) when the box cannot verify a witness and must stall — NEVER counting an
// unverified set/value.
//
// It reads MinBond from the box's OWN cfg (C-6), never the witness. The count over the whole bonded
// map is anchored on the committed bondedRoot (set-completeness), with each member's bonded weight
// and slashed bit proven per-member against the committed StateRoot.
//
// This does NOT flip WitnessValidateV5 to Accept (the STOP boundary): it reproduces ONE predicate.
func (c *Chain) RecomputeQualifiedCount(
	committedStateRoot ports.Hash,
	w QualifiedCountWitness,
) (n int, reason error) {
	// (1) SET-COMPLETENESS over BONDED. Prove the committed bondedRoot leaf present against the
	// committed StateRoot, then require the reconstructed MTH over the witnessed whole-bonded id-list
	// equals it. One omitted (or injected) member yields a different MTH ⇒ mismatch ⇒ stall.
	rootKey := statehash.Key(tagBondedRoot, nil)
	rootRes := statehash.Resolve(committedStateRoot, rootKey, w.BondedRootValue, w.BondedRootWitness)
	if !rootRes.IsProvenPresent() {
		return 0, ErrRecomputeQualifiedBondedRootUnproven
	}
	reconstructed := nodeSetMTH(w.IDs)
	if !bytes.Equal(reconstructed, w.BondedRootValue) {
		return 0, fmt.Errorf("%w: reconstructed MTH %x != committed bondedRoot %x",
			ErrRecomputeQualifiedBondedSetIncomplete, reconstructed, w.BondedRootValue)
	}

	// (2) PER-MEMBER BONDED WEIGHT (C-1) + (3) PER-MEMBER SLASHED BIT (C-1) + (4) THE COUNT,
	// byte-for-byte qualifiedCount (chain.go:1479-1487). For every member of the
	// completeness-verified whole-bonded set, verify its bonded weight (the screen operand) and its
	// slashed bit against the committed root, then count it iff bonded[id] >= own MinBond && !slashed.
	minBond := c.cfg.MinBond
	for _, id := range w.IDs {
		mw, ok := w.Members[id]
		if !ok {
			// A member in the completeness-verified set has no state witness: the box cannot verify its
			// committed state, so it cannot count the set. Stall, never count a partial set.
			return 0, fmt.Errorf("%w: id %x has no member state witness", ErrRecomputeQualifiedMemberStateUnproven, id[:])
		}

		// C-1: bonded weight. Inclusion proof of (bonded[id] → EncodeInt64(Bonded)). A forged weight
		// fails ⇒ stall. Every bonded member has a committed weight leaf (bondedRoot's members are
		// exactly the keys of the bonded map), so an honest producer always resolves present.
		bondedKey := statehash.Key(tagBonded, id[:])
		bondedRes := statehash.Resolve(committedStateRoot, bondedKey, statehash.EncodeInt64(mw.Bonded), mw.BondedProof)
		if !bondedRes.IsProvenPresent() {
			return 0, fmt.Errorf("%w: id %x bonded %d", ErrRecomputeQualifiedMemberStateUnproven, id[:], mw.Bonded)
		}

		// C-1: slashed membership. Present ⇒ inclusion proof of (slashed[id] → Present); absent ⇒
		// non-inclusion proof. Either must verify against the committed root, else stall. A prover
		// cannot fake the bit in either direction (inflating N by dropping a slash, or deflating it by
		// injecting one).
		slashedKey := statehash.Key(tagSlashed, id[:])
		var slashedVal []byte
		if mw.Slashed {
			slashedVal = statehash.Present
		}
		slashedRes := statehash.Resolve(committedStateRoot, slashedKey, slashedVal, mw.SlashedProof)
		if mw.Slashed && !slashedRes.IsProvenPresent() {
			return 0, fmt.Errorf("%w: id %x slashed(present)", ErrRecomputeQualifiedMemberStateUnproven, id[:])
		}
		if !mw.Slashed && !slashedRes.IsProvenAbsent() {
			return 0, fmt.Errorf("%w: id %x slashed(absent)", ErrRecomputeQualifiedMemberStateUnproven, id[:])
		}

		// (4) THE COUNT: bonded[id] >= own MinBond (C-6) && !slashed[id]. Byte-for-byte chain.go:1482.
		if mw.Bonded >= minBond && !mw.Slashed {
			n++
		}
	}
	return n, nil
}
