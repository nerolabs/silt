package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — lane-1 Part B core, increment 3.
//
// This file reproduces a THIRD validity predicate — the F-1 DE-MATURE SUPER-QUORUM
// requireDeMatureSuperQuorum (chain.go:2947) — trustlessly, from the committed StateRoot +
// witnesses ALONE. It replicates increment 1's C-1 pattern (floorbox_recompute_v5.go,
// RecomputeEpochWeightQuorum) over a DIFFERENT keyspace: the WHOLE bonded map (the R-membership
// budget path), rather than the frozen epochSet.
//
// It is ADDITIVE: it calls no full-node accept path, mutates nothing, and changes NO
// consensus/validity rule. A full node still computes requireDeMatureSuperQuorum from its own
// in-memory bonded map (chain.go untouched). This is a SEPARATE root-only path a semi-stateless
// box calls INSTEAD of holding the tree — the same posture the two prior increments hold.
//
// THE PREDICATE. Once a chain has matured (everMature) but its live decentralization has since
// dropped below the bar (everMature && objective() && !matureNow(), chain.go:2827), a commit
// must be carried by a real-bond SUPER-MAJORITY: the committing coalition (proposer + the
// distinct qualified attesters `seen`) must control >= ⌈2·total/3⌉ of the WHOLE live bonded
// weight (Σ bonded), no anchor sign-off. Any two such super-quorums intersect in > ⅓ of the
// weight, so they share honest bond — the center-less replacement for the retired anchor net.
//
// THE MATURITY GATE (the increment-3-specific piece). requireDeMatureSuperQuorum fires ONLY
// when !matureNow(). So the trustless reproduction gates on the REPRODUCED maturity state:
// RecomputeDeMatureSuperQuorum calls RecomputeMatureNow (increment 2) first and folds the
// super-quorum ONLY when the maturity recompute returns mature == false. When mature == true
// the full node does not run this predicate, so the recompute returns (met=true, nil) — a
// no-op that matches the full node's skip. The maturity state is itself PROVEN from the
// committed root (increment 2), so a producer cannot trick the box into enforcing (or skipping)
// the de-mature bar in the wrong maturity state.
//
// THE THREE-PART PROOF (RecomputeDeMatureSuperQuorum, the super-quorum fold):
//  1. SET-COMPLETENESS: reconstruct nodeSetMTH(witnessedIDs) over the whole-bonded id-list;
//     require it equals the committed bondedRoot leaf (proven present against the StateRoot).
//     One omitted (or injected) member ⇒ a different MTH ⇒ mismatch ⇒ stall. This is the F1
//     bondedRoot digest FINALLY READ (F1 committed it inert; increment 3 consumes it for bonded).
//  2. PER-MEMBER WEIGHT (C-1): for EVERY id in the reconstructed set, Resolve the bonded[id]
//     value leaf against the committed StateRoot. A forged weight fails smt.VerifyProof ⇒ stall.
//     The digest gave membership; the inclusion proofs give the values.
//  3. THRESHOLD (C-6): the ⅔ ratio is a fixed consensus constant, NOT a genesis knob, so this
//     predicate's fold reads no per-deployment config value — there is nothing here an attacker
//     could shift via the witness (exactly like increment 1's requireEpochWeightQuorum). The C-6
//     obligation is nonetheless exercised, not skipped: the recompute takes NO threshold from w.
//
// Then the fold + threshold, byte-for-byte the full node's (chain.go:2949-2963):
// total = Σ verified whole-bonded weights; committed = bonded[proposer] + Σ_{id∈seen} bonded[id];
// need = ⌈2·total/3⌉; met = committed >= need.
//
// STOP BOUNDARY (this increment). It reproduces ONE predicate. It does NOT flip #657
// WitnessValidateV5 to Accept — that is the final increment, only after ALL predicates are
// reproduced. The box STILL never-Accepts. requireDeMatureSuperQuorum folds the WHOLE bonded map
// directly — it does NOT consult effectiveEpochSet/liveQualifiedSet, so the #535 recovery
// boundary does NOT change this fold (there is no boundary case to carve out for it). The
// boundary remains the ratified trust-the-directive carve-out (cert C-2) for the sets it DOES
// touch (epochSet, increment 1), out of scope here.

var (
	// ErrRecomputeBondedSetIncomplete marks a stall where the witnessed whole-bonded id-list does
	// not reconstruct the committed bondedRoot: a member was omitted (or an extra injected), so the
	// MTH over the witnessed list differs from the committed digest. The box CANNOT trust an
	// incomplete set to fold a whole-set super-quorum — it stalls, never folds a short set.
	ErrRecomputeBondedSetIncomplete = errors.New("chain: floor-box de-mature recompute — witnessed bonded id-list does not reconstruct the committed bondedRoot (a member was omitted or injected)")

	// ErrRecomputeBondedRootUnproven marks a stall where the committed bondedRoot leaf itself could
	// not be proven present against the committed StateRoot (no/failed inclusion witness). Without
	// the committed digest the box has nothing to compare the reconstructed MTH to.
	ErrRecomputeBondedRootUnproven = errors.New("chain: floor-box de-mature recompute — committed bondedRoot leaf not proven present against the committed StateRoot")

	// ErrRecomputeBondedMemberWeightUnproven marks a stall where a per-member bonded[id] weight leaf
	// could not be proven present against the committed StateRoot (no/failed/forged inclusion
	// witness). This is the C-1 closure: a forged weight cannot verify, so it stalls the fold rather
	// than letting a forgeable super-quorum tally through.
	ErrRecomputeBondedMemberWeightUnproven = errors.New("chain: floor-box de-mature recompute — a per-member bonded weight leaf not proven present against the committed StateRoot (C-1: the weight is forged or missing)")
)

// BondedSetWitness is the witnessed input a floor box supplies to reproduce
// requireDeMatureSuperQuorum: the claimed COMPLETE id-list of the whole bonded map, the SMT
// inclusion proof of the committed bondedRoot digest leaf, and one per-member inclusion proof +
// claimed weight for every id in the list. It is UNTRUSTED input from an any-of-N provider;
// every field becomes meaningful only after verification against the committed StateRoot.
type BondedSetWitness struct {
	// IDs is the id-list the prover claims is the COMPLETE bonded set. Completeness is not trusted:
	// the recompute reconstructs nodeSetMTH(IDs) and compares it to the committed bondedRoot. A
	// short (member-omitted) or padded (member-injected) list yields a different MTH and stalls.
	// Order does not matter — nodeSetMTH canonically sorts.
	IDs []ports.NodeID

	// BondedRootWitness is the SMT inclusion proof of the committed bondedRoot leaf
	// (Key(tagBondedRoot, nil) → the committed MTH value) against the committed StateRoot. The
	// recompute proves the committed digest present, then compares it to the reconstructed MTH.
	BondedRootWitness statehash.Witness

	// BondedRootValue is the committed bondedRoot leaf value the BondedRootWitness proves. It is
	// the MTH the recompute compares nodeSetMTH(IDs) against; a mismatch is set-incompleteness.
	BondedRootValue []byte

	// MemberWeights maps each id in IDs to its claimed per-member bonded weight and the SMT
	// inclusion proof of the bonded[id] value leaf. The recompute Resolves each against the
	// committed StateRoot: a forged weight fails verification and stalls (C-1). Every id in IDs
	// MUST have an entry, else the recompute cannot verify that member's weight and stalls.
	MemberWeights map[ports.NodeID]MemberWeightWitness
}

// RecomputeDeMatureSuperQuorum reproduces requireDeMatureSuperQuorum (the F-1 de-mature
// super-quorum, chain.go:2947) TRUSTLESSLY, from the committed StateRoot + the witnesses alone.
// It returns (met, nil) where met is the verdict a full node's ValidateCommit would produce for
// the de-mature gate at this state (met == the err==nil case), or (false, reason) when the box
// cannot verify a witness and must stall — NEVER folding an unverified set/weight.
//
// It gates on the REPRODUCED maturity state (increment 2): the super-quorum is folded ONLY when
// the reproduced matureNow is false (the de-mature transition). When the chain is mature the full
// node does not run this predicate, so the recompute returns (met=true, nil) to match the skip.
// seenW is the increment-2 SeenSetWitness proving the maturity state; bondedW is this increment's
// whole-bonded witness proving the super-quorum fold.
//
// It reads the ⅔ threshold from a fixed consensus constant (C-6), never the witness. It
// reproduces the whole-bonded fold; requireDeMatureSuperQuorum consults no epoch set, so the #535
// recovery boundary does not change it (no boundary carve-out for this predicate).
//
// This does NOT flip WitnessValidateV5 to Accept (the STOP boundary): it reproduces ONE predicate.
func (c *Chain) RecomputeDeMatureSuperQuorum(
	committedStateRoot ports.Hash,
	proposer ports.NodeID,
	seen map[ports.NodeID]bool,
	seenW SeenSetWitness,
	bondedW BondedSetWitness,
) (met bool, reason error) {
	// (0) THE MATURITY GATE (increment 2). requireDeMatureSuperQuorum runs only when !matureNow().
	// Reproduce matureNow trustlessly; if the box cannot verify the maturity witness, stall. If the
	// chain is mature, the full node does NOT run the de-mature predicate — return met=true (the
	// gate is vacuous), matching the full node's skip.
	mature, mReason := c.RecomputeMatureNow(committedStateRoot, seenW)
	if mReason != nil {
		return false, mReason
	}
	if mature {
		return true, nil // mature: the de-mature bar does not bind (full node skips the check)
	}

	// (1) SET-COMPLETENESS. Prove the committed bondedRoot leaf present against the committed
	// StateRoot, then require the reconstructed MTH over the witnessed whole-bonded id-list equals
	// it. One omitted (or injected) member yields a different MTH ⇒ mismatch ⇒ stall.
	rootKey := statehash.Key(tagBondedRoot, nil)
	rootRes := statehash.Resolve(committedStateRoot, rootKey, bondedW.BondedRootValue, bondedW.BondedRootWitness)
	if !rootRes.IsProvenPresent() {
		return false, ErrRecomputeBondedRootUnproven
	}
	reconstructed := nodeSetMTH(bondedW.IDs)
	if !bytes.Equal(reconstructed, bondedW.BondedRootValue) {
		return false, fmt.Errorf("%w: reconstructed MTH %x != committed bondedRoot %x",
			ErrRecomputeBondedSetIncomplete, reconstructed, bondedW.BondedRootValue)
	}

	// (2) PER-MEMBER WEIGHT (C-1) + (4) THE FOLD. For every id in the now-completeness-verified
	// whole-bonded set, Resolve its bonded[id] weight leaf against the committed StateRoot (a
	// forged weight fails verification ⇒ stall), then fold exactly as chain.go:2949-2957:
	// total = Σ verified whole-bonded weights; committed = bonded[proposer] + Σ_{id∈seen} bonded[id].
	// A proposer/seen id NOT in the bonded set contributes 0 (it is not folded), matching the full
	// node where c.bonded[id] is 0 for a non-bonded id.
	var total, committed int64
	for _, id := range bondedW.IDs {
		mw, ok := bondedW.MemberWeights[id]
		if !ok {
			// A member in the completeness-verified set has no weight witness: the box cannot verify
			// its weight, so it cannot fold the set. Stall, never fold a partial set.
			return false, fmt.Errorf("%w: id %x has no weight witness", ErrRecomputeBondedMemberWeightUnproven, id[:])
		}
		memberKey := statehash.Key(tagBonded, id[:])
		res := statehash.Resolve(committedStateRoot, memberKey, statehash.EncodeInt64(mw.Weight), mw.Proof)
		if !res.IsProvenPresent() {
			// C-1: a forged weight produces a leaf value the committed root does not commit, so the
			// inclusion proof fails to verify. Stall — the tally would otherwise be forgeable.
			return false, fmt.Errorf("%w: id %x weight %d", ErrRecomputeBondedMemberWeightUnproven, id[:], mw.Weight)
		}
		total += mw.Weight
		if id == proposer || seen[id] {
			committed += mw.Weight
		}
	}

	// (3) THE ≥⅔ SUPER-QUORUM THRESHOLD, byte-for-byte the full node's (chain.go:2952-2963).
	//
	// C-6 (threshold from a fixed constant, never the witness). The ⅔ ratio is a fixed consensus
	// constant, not a genesis knob, so this fold reads no per-deployment config value — there is
	// nothing here an attacker could shift via the witness. The C-6 obligation is exercised, not
	// skipped: the recompute takes NO threshold from bondedW (the config-from-witness ablation
	// asserts a witness-carried threshold cannot move the verdict).
	if total <= 0 {
		return true, nil // no bonded weight to measure against (nothing to protect) — full node returns nil.
	}
	need := (2*total + 2) / 3 // ⌈2·total/3⌉, byte-for-byte chain.go:2959
	return committed >= need, nil
}
