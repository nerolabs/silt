package chain

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — lane-1 Part B core, increment 2.
//
// This file reproduces a SECOND validity predicate — the MATURITY LATCH metric matureNow
// (chain.go:2178) via C2Metric (chain.go:2300-2382) — trustlessly, from the committed
// StateRoot + witnesses ALONE. It replicates increment 1's structure
// (floorbox_recompute_v5.go, RecomputeEpochWeightQuorum) and, crucially, it is the FIRST
// predicate whose fold READS GENESIS CONFIG, so it is where the C-6 obligation finally has
// TEETH (see the C-6 note below and the ablation in the test).
//
// It is ADDITIVE: it calls no full-node accept path, mutates nothing, and changes NO
// consensus/validity rule. A full node still computes matureNow from its own in-memory
// validatorsSeen/bonded/bondDomain maps (chain.go untouched). This is a SEPARATE root-only
// path a semi-stateless box calls INSTEAD of holding the tree — the same posture
// floorbox_v5.go and floorbox_recompute_v5.go already hold.
//
// THE PREDICATE. matureNow gates the de-mature super-quorum: a mature-and-objective chain
// whose live decentralization has since dropped below the bar (everMature && objective() &&
// !matureNow(), chain.go:2827) must commit under a real-bond super-majority instead of the
// retired anchor net. matureNow (objective branch) is MatureCoefficient() >= MatureValidators,
// where MatureCoefficient = min(NakamotoOperators, NakamotoDomains) from C2Metric — the
// operator-and-domain-distinct bonded-distinctness count over the COMMITTED ledger.
//
// C2Metric is a WHOLE-SET fold over validatorsSeen. For every member id it reads:
//   - membership of validatorsSeen (set-completeness, the F1 validatorsSeenRoot digest);
//   - c.cfg.Anchors[id]  — OWN genesis config (skip anchors) — C-6;
//   - c.slashed[id]      — committed keyspace (skip slashed) — C-1;
//   - c.bonded[id]       — committed per-member weight — C-1;
//   - c.bondDomain[id]   — committed per-member declared domain (absent = domain 0) — C-1;
//   - c.cfg.MinBond      — OWN genesis config (eligibility screen sz >= MinBond) — C-6;
//   - c.cfg.OperatorMargin (operatorMargin) — OWN genesis config (coefficient divisor) — C-6;
//   - c.cfg.MatureValidators — OWN genesis config (the threshold) — C-6.
//
// THE THREE-PART PROOF (RecomputeMatureNow):
//  1. SET-COMPLETENESS: reconstruct nodeSetMTH(witnessedIDs); require it equals the committed
//     validatorsSeenRoot leaf (proven present against the StateRoot). One omitted (or injected)
//     member ⇒ a different MTH ⇒ mismatch ⇒ stall. This is the F1 validatorsSeenRoot digest
//     FINALLY READ (F1 committed it inert; this increment consumes it for validatorsSeen).
//  2. PER-MEMBER VALUES (C-1): for EVERY id in the reconstructed set, Resolve its slashed[id]
//     membership, bonded[id] weight, and bondDomain[id] domain against the committed StateRoot.
//     A forged weight/domain/slashed-bit fails smt.VerifyProof ⇒ NoWitness ⇒ stall. The digest
//     gave membership; the inclusion proofs give the values.
//  3. GENESIS CONFIG (C-6): MinBond, Anchors, OperatorMargin, MatureValidators are read from the
//     box's OWN cfg (c.cfg.*), NEVER from any witness. This predicate is the FIRST whose fold
//     reads genesis knobs; each is threshold-shifting if an attacker controls it (a lower MinBond
//     admits cheap members, a lower margin inflates the coefficient, a lower MatureValidators
//     lowers the bar). Reading own config forecloses every such shift — the C-6 teeth increment 1
//     could not exercise.
//
// Then the fold + threshold, byte-for-byte the full node's (chain.go:2300-2382, 2207-2214,
// 2194): min(NakamotoOperators, NakamotoDomains) >= MatureValidators.
//
// STOP BOUNDARY (this increment). It reproduces ONE predicate. It does NOT flip #657
// WitnessValidateV5 to Accept — that is the final increment, only after ALL predicates are
// reproduced. The box STILL never-Accepts. It reproduces the OBJECTIVE branch of matureNow
// (objective() = MinBond>0 && verifyBond!=nil, true for any untrusted deployment); the
// non-objective launch branch is the trusted-anchor phase, out of scope.

var (
	// ErrRecomputeSeenSetIncomplete marks a stall where the witnessed id-list does not reconstruct
	// the committed validatorsSeenRoot: a member was omitted (or an extra injected), so the MTH over
	// the witnessed list differs from the committed digest. The box CANNOT trust an incomplete set
	// to fold a whole-set distinctness metric — it stalls, never folds a short set.
	ErrRecomputeSeenSetIncomplete = errors.New("chain: floor-box maturity recompute — witnessed validatorsSeen id-list does not reconstruct the committed validatorsSeenRoot (a member was omitted or injected)")

	// ErrRecomputeSeenRootUnproven marks a stall where the committed validatorsSeenRoot leaf itself
	// could not be proven present against the committed StateRoot (no/failed inclusion witness).
	// Without the committed digest the box has nothing to compare the reconstructed MTH to.
	ErrRecomputeSeenRootUnproven = errors.New("chain: floor-box maturity recompute — committed validatorsSeenRoot leaf not proven present against the committed StateRoot")

	// ErrRecomputeMemberStateUnproven marks a stall where a per-member committed value leaf
	// (bonded weight, bondDomain, or slashed membership) could not be proven present/absent against
	// the committed StateRoot (no/failed/forged witness). This is the C-1 closure: a forged member
	// value cannot verify, so it stalls the fold rather than letting a forgeable coefficient through.
	ErrRecomputeMemberStateUnproven = errors.New("chain: floor-box maturity recompute — a per-member committed value leaf (bonded/bondDomain/slashed) not proven against the committed StateRoot (C-1: forged or missing)")
)

// MemberStateWitness is one validatorsSeen member's claimed committed state plus the SMT
// inclusion/non-inclusion proofs the recompute verifies against the committed StateRoot. Every
// field is UNTRUSTED until Resolve confirms it: a forged weight/domain/slashed-bit produces a
// leaf value the committed root does not commit, so its proof fails and the member is unproven.
type MemberStateWitness struct {
	// Bonded is the claimed committed bonded[id] weight. Verified by Resolving the bonded[id]
	// leaf (encoded EncodeInt64(Bonded)) against the committed root — a forged weight fails (C-1).
	Bonded int64

	// BondedProof is the SMT inclusion proof of Key(tagBonded, id) → EncodeInt64(Bonded).
	BondedProof statehash.Witness

	// Domain is the claimed committed bondDomain[id] declared failure-domain (A axis). Zero means
	// UNSET — an unset domain has NO committed bondDomain leaf, so DomainPresent is false and the
	// proof is a NON-INCLUSION proof. A member with an unset domain forms its own address-diversity
	// group (never aggregated), matching C2Metric's zeroDomainWeights handling.
	Domain uint64

	// DomainPresent reports whether bondDomain[id] is committed (a non-zero declared domain). When
	// true, DomainProof is an inclusion proof of Key(tagBondDomain, id) → EncodeUint64(Domain).
	// When false, DomainProof is a non-inclusion proof (the member declared no domain).
	DomainPresent bool

	// DomainProof is the SMT proof of the bondDomain[id] leaf — inclusion when DomainPresent,
	// non-inclusion otherwise. A forged domain (claimed present with the wrong value, or claimed
	// absent while committed) fails to verify ⇒ stall (C-1).
	DomainProof statehash.Witness

	// Slashed reports whether the member is claimed to be in the committed slashed set. When true,
	// SlashedProof is an inclusion proof of Key(tagSlashed, id) → Present; C2Metric skips the
	// member. When false, SlashedProof is a non-inclusion proof (the member is not slashed).
	Slashed bool

	// SlashedProof is the SMT proof of the slashed[id] membership — inclusion when Slashed,
	// non-inclusion otherwise. A prover cannot silently drop a slashed member (that would shrink
	// the tally): the recompute verifies the slashed bit for every member either way (C-1).
	SlashedProof statehash.Witness
}

// SeenSetWitness is the witnessed input a floor box supplies to reproduce matureNow: the claimed
// COMPLETE id-list of validatorsSeen, the inclusion proof of the committed validatorsSeenRoot
// digest, and one MemberStateWitness (bonded / bondDomain / slashed proofs) per id. It is
// UNTRUSTED input from an any-of-N provider; every field becomes meaningful only after
// verification against the committed StateRoot.
type SeenSetWitness struct {
	// IDs is the id-list the prover claims is the COMPLETE validatorsSeen set. Completeness is not
	// trusted: the recompute reconstructs nodeSetMTH(IDs) and compares it to the committed
	// validatorsSeenRoot. A short (member-omitted) or padded (member-injected) list yields a
	// different MTH and stalls. Order does not matter — nodeSetMTH canonically sorts.
	IDs []ports.NodeID

	// SeenRootWitness is the SMT inclusion proof of the committed validatorsSeenRoot leaf
	// (Key(tagValidatorsSeenRoot, nil) → the committed MTH) against the committed StateRoot.
	SeenRootWitness statehash.Witness

	// SeenRootValue is the committed validatorsSeenRoot leaf value the SeenRootWitness proves — the
	// MTH the recompute compares nodeSetMTH(IDs) against; a mismatch is set-incompleteness.
	SeenRootValue []byte

	// Members maps each id in IDs to its committed-state witness. Every id in IDs MUST have an
	// entry, else the recompute cannot verify that member's state and stalls.
	Members map[ports.NodeID]MemberStateWitness
}

// SeenSetStreamWitness is the STREAMING form of SeenSetWitness. It carries the SAME anchored
// inputs — the complete id-list, the committed validatorsSeenRoot digest proof — but delivers the
// per-member MemberStateWitness through a PULL provider instead of a resident map. The box requests
// one member's witness, verifies its proofs against the committed root, folds the scalar, and drops
// that member's proof heap before requesting the next. Resident witness drops from O(N·depth) (the
// whole Members map, ~20 KB/member) to O(depth) (one member in flight).
//
// SOUNDNESS: this changes ONLY how the per-member proof bytes are held in memory. WHAT is verified is
// byte-identical to SeenSetWitness — the same committedStateRoot anchor, the same completeness MTH
// over the FULL id-list (line-level identical, IDs stays whole; R-M-STREAM-COMPLETENESS), the same
// per-member predicate, the same own-config screens. It is the certified soundness-neutral memory
// refactor (Candidate 1, research cert 2026-09-02; PE Option 1). It is an in-memory Go type; nothing
// here is serialized, so it is NOT a wire/format change (no v5 leaf, no committed object touched).
type SeenSetStreamWitness struct {
	// IDs is the COMPLETE validatorsSeen id-list — identical role to SeenSetWitness.IDs. It stays
	// resident whole: the completeness MTH nodeSetMTH(IDs) must see the full list (a short list yields
	// a different MTH ⇒ stall). Streaming frees per-member PROOF heaps, NOT the id-list. The id-list is
	// small (32 B/id), so holding it whole is not the resident cost the refactor attacks.
	IDs []ports.NodeID

	// SeenRootWitness / SeenRootValue are identical to SeenSetWitness — the committed validatorsSeenRoot
	// digest proof and the MTH value the reconstructed nodeSetMTH(IDs) is compared against.
	SeenRootWitness statehash.Witness
	SeenRootValue   []byte

	// Member pulls one member's committed-state witness on demand. It returns (witness, true) when the
	// id has a witness, or (_, false) when it does not — which stalls exactly as a missing map entry
	// does in the resident form. The caller (the witness-delivery seam) is free to fetch/decode the
	// proof lazily and let it be freed after the fold consumes it. Member MUST be non-nil.
	Member func(ports.NodeID) (MemberStateWitness, bool)
}

// RecomputeMatureNow reproduces matureNow (the maturity-latch metric, chain.go:2178) TRUSTLESSLY,
// from the committed StateRoot + the witness alone, for the OBJECTIVE phase. It returns (mature,
// nil) where mature == matureNow()'s verdict a full node would produce, or (false, reason) when
// the box cannot verify the witness and must stall — NEVER folding an unverified set/value.
//
// It reads MinBond / Anchors / OperatorMargin / MatureValidators from the box's OWN cfg (C-6),
// never the witness. This is the first predicate whose fold reads genesis config, so the C-6
// obligation has TEETH here (the config-from-witness ablation).
//
// This does NOT flip WitnessValidateV5 to Accept (the STOP boundary): it reproduces ONE predicate.
//
// This is the RESIDENT-MAP adapter over the streaming core: it wraps w.Members in a pull provider and
// delegates to RecomputeMatureNowStreaming, so the fold logic lives in ONE place and the resident and
// streaming paths cannot drift. The verdict is byte-identical to the streaming path.
func (c *Chain) RecomputeMatureNow(committedStateRoot ports.Hash, w SeenSetWitness) (mature bool, reason error) {
	return c.RecomputeMatureNowStreaming(committedStateRoot, SeenSetStreamWitness{
		IDs:             w.IDs,
		SeenRootWitness: w.SeenRootWitness,
		SeenRootValue:   w.SeenRootValue,
		Member: func(id ports.NodeID) (MemberStateWitness, bool) {
			mw, ok := w.Members[id]
			return mw, ok
		},
	})
}

// RecomputeMatureNowStreaming is the streaming core of the class-M maturity recompute. It reproduces
// the SAME predicate as RecomputeMatureNow with the SAME anchoring against committedStateRoot, but it
// pulls each member's proof witness on demand (w.Member) and lets that member's proof heap be freed
// before the next member is verified. Resident witness is O(depth), not O(N·depth).
//
// SOUNDNESS-NEUTRAL by construction (certified 2026-09-02): every Resolve still verifies against the
// same committedStateRoot; the completeness MTH still consumes the FULL id-list (line-identical); the
// fold accumulates the same order-independent scalars. Freeing a member's proof heap after its Resolve
// returns cannot change any verdict (VerifyProof is a pure function of (proof, root, key, value)).
func (c *Chain) RecomputeMatureNowStreaming(committedStateRoot ports.Hash, w SeenSetStreamWitness) (mature bool, reason error) {
	// (1) SET-COMPLETENESS. Prove the committed validatorsSeenRoot leaf present against the committed
	// StateRoot, then require the reconstructed MTH over the witnessed id-list equals it. One omitted
	// (or injected) member yields a different MTH ⇒ mismatch ⇒ stall.
	//
	// R-M-STREAM-COMPLETENESS: this consumes the FULL w.IDs — streaming frees per-member PROOF heaps,
	// NEVER the id-list. A short/truncated id-list yields a different MTH and stalls here, identically
	// to the resident form. (RED ablation: floorbox_recompute_maturity_streaming_v5_test.go.)
	rootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	rootRes := statehash.Resolve(committedStateRoot, rootKey, w.SeenRootValue, w.SeenRootWitness)
	if !rootRes.IsProvenPresent() {
		return false, ErrRecomputeSeenRootUnproven
	}
	reconstructed := nodeSetMTH(w.IDs)
	if !bytes.Equal(reconstructed, w.SeenRootValue) {
		return false, fmt.Errorf("%w: reconstructed MTH %x != committed validatorsSeenRoot %x",
			ErrRecomputeSeenSetIncomplete, reconstructed, w.SeenRootValue)
	}

	// (2) PER-MEMBER VALUES (C-1) + (3) OWN CONFIG (C-6) + (4) THE FOLD, byte-for-byte C2Metric
	// (chain.go:2300-2382). For every member of the completeness-verified set, PULL its witness, verify
	// its slashed/bonded/bondDomain leaves against the committed root, then fold exactly as C2Metric:
	// skip own-Anchors and slashed members; a bond >= own MinBond joins `sizes` and its domain
	// aggregates (non-zero domain) or forms its own group (unset). Own config governs the screen,
	// the margin, and the threshold — never the witness (C-6). The pulled `mw` (and its proof heaps) is
	// scoped to this iteration: it is freed before the next member is pulled.
	// A nil provider cannot deliver any member's witness, so no member can be verified. The safe
	// default is to stall (never-Accept), exactly as a resident witness with no Members map would.
	if w.Member == nil {
		return false, fmt.Errorf("%w: no member provider (nil Member)", ErrRecomputeMemberStateUnproven)
	}
	minBond := c.cfg.MinBond
	sizes := make([]int64, 0, len(w.IDs))
	domainWeight := make(map[uint64]int64) // A axis: non-zero domains aggregated
	var zeroDomainWeights []int64          // domain 0 (unset) → each its own group
	var total int64
	for _, id := range w.IDs {
		mw, ok := w.Member(id)
		if !ok {
			// A member in the completeness-verified set has no state witness: the box cannot verify
			// its committed state, so it cannot fold the set. Stall, never fold a partial set.
			return false, fmt.Errorf("%w: id %x has no member state witness", ErrRecomputeMemberStateUnproven, id[:])
		}

		// C-1: slashed membership. Present ⇒ inclusion proof of (slashed[id] → Present); absent ⇒
		// non-inclusion proof. Either must verify against the committed root, else stall.
		slashedKey := statehash.Key(tagSlashed, id[:])
		var slashedVal []byte
		if mw.Slashed {
			slashedVal = statehash.Present
		}
		slashedRes := statehash.Resolve(committedStateRoot, slashedKey, slashedVal, mw.SlashedProof)
		if mw.Slashed && !slashedRes.IsProvenPresent() {
			return false, fmt.Errorf("%w: id %x slashed(present)", ErrRecomputeMemberStateUnproven, id[:])
		}
		if !mw.Slashed && !slashedRes.IsProvenAbsent() {
			return false, fmt.Errorf("%w: id %x slashed(absent)", ErrRecomputeMemberStateUnproven, id[:])
		}

		// C-6: own-config Anchor screen + committed slashed screen. C2Metric skips both BEFORE
		// reading the bond. Anchors is OWN genesis config (never the witness); slashed is the
		// committed bit just verified.
		if c.cfg.Anchors[id] || mw.Slashed {
			continue
		}

		// C-1: bonded weight. Inclusion proof of (bonded[id] → EncodeInt64(Bonded)). A forged
		// weight fails ⇒ stall. A member with no committed bond (weight would be 0) still needs a
		// proof; the honest producer emits every member's bonded leaf, so a genuine member always
		// resolves present. (A member of validatorsSeen without a bonded leaf cannot occur: seating
		// requires a bond.)
		bondedKey := statehash.Key(tagBonded, id[:])
		bondedRes := statehash.Resolve(committedStateRoot, bondedKey, statehash.EncodeInt64(mw.Bonded), mw.BondedProof)
		if !bondedRes.IsProvenPresent() {
			return false, fmt.Errorf("%w: id %x bonded %d", ErrRecomputeMemberStateUnproven, id[:], mw.Bonded)
		}

		// C-1: bondDomain. Present ⇒ inclusion proof of (bondDomain[id] → EncodeUint64(Domain));
		// absent ⇒ non-inclusion proof (unset domain). Either must verify, else stall.
		domainKey := statehash.Key(tagBondDomain, id[:])
		var domainVal []byte
		if mw.DomainPresent {
			domainVal = statehash.EncodeUint64(mw.Domain)
		}
		domainRes := statehash.Resolve(committedStateRoot, domainKey, domainVal, mw.DomainProof)
		if mw.DomainPresent && !domainRes.IsProvenPresent() {
			return false, fmt.Errorf("%w: id %x domain(present) %d", ErrRecomputeMemberStateUnproven, id[:], mw.Domain)
		}
		if !mw.DomainPresent && !domainRes.IsProvenAbsent() {
			return false, fmt.Errorf("%w: id %x domain(absent)", ErrRecomputeMemberStateUnproven, id[:])
		}

		// The eligibility screen: own MinBond (C-6). Below it, the member is not counted — exactly
		// C2Metric's `if sz := c.bonded[id]; sz >= c.cfg.MinBond`.
		if mw.Bonded < minBond {
			continue
		}
		sizes = append(sizes, mw.Bonded)
		total += mw.Bonded
		if mw.DomainPresent && mw.Domain != 0 {
			domainWeight[mw.Domain] += mw.Bonded // same declared domain → one group
		} else {
			zeroDomainWeights = append(zeroDomainWeights, mw.Bonded) // unset → independent
		}
	}

	// (5) THE COEFFICIENT + THRESHOLD, byte-for-byte C2Metric + MatureCoefficient + matureNow.
	//
	// C-6: OperatorMargin and MatureValidators are read from own cfg. operatorMargin() resolves an
	// unset/zero margin to 1, exactly as the full node does (chain.go:2388-2393).
	if total == 0 {
		// Degenerate: no qualifying bonded weight. MatureCoefficient = min over empty = 0
		// (NakamotoOperators/NakamotoDomains default to their group counts, which are 0), so
		// matureNow is 0 >= MatureValidators — true only if MatureValidators == 0.
		return 0 >= c.cfg.MatureValidators, nil
	}

	nakamotoBonds := nakamotoCoefficient(sizes, total)
	nakamotoOperators := nakamotoBonds / c.operatorMargin() // ⌊k̂/M⌋, conservative

	groups := make([]int64, 0, len(domainWeight)+len(zeroDomainWeights))
	for _, weight := range domainWeight {
		groups = append(groups, weight)
	}
	groups = append(groups, zeroDomainWeights...)
	nakamotoDomains := nakamotoCoefficient(groups, total)

	matureCoefficient := nakamotoOperators
	if nakamotoDomains < matureCoefficient {
		matureCoefficient = nakamotoDomains
	}
	return matureCoefficient >= c.cfg.MatureValidators, nil
}

// nakamotoCoefficient is the fewest of the given (bond-or-group) weights whose combined value
// EXCEEDS the Byzantine fraction (⌊total/3⌋) of total — the raw cost-to-corrupt coefficient,
// byte-for-byte the C2Metric fold (chain.go:2323-2334, 2348-2357). weights need not be sorted;
// this sorts a copy descending, so the caller's slice is untouched. total > 0 is required
// (the caller guards total == 0). An empty weights yields 0 (len(weights)).
func nakamotoCoefficient(weights []int64, total int64) int {
	sorted := append([]int64(nil), weights...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] }) // largest first
	threshold := total / 3
	coeff := len(sorted) // all needed unless the loop finds fewer
	var cum int64
	for i, sz := range sorted {
		cum += sz
		if cum > threshold {
			coeff = i + 1
			break
		}
	}
	return coeff
}
