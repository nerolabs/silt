package chain

import (
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-e,
// CLASS A (attestations → validatorsSeen) — the FOURTH changed-whole-set-digest class.
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md
//     (A CERTIFIED-in-direction; carries R-A-legacy — assert objective-mode, stall in legacy —
//      and R-A-membership-source — the mature-epoch screen reads the FROZEN epochSet, not live
//      bonded. Both are FOLD-CAUGHT liveness/derivation burdens, never a wrong-accept.)
// Box STILL never-Accepts (R-scope). This reproduces validateEra3Roots' StateRoot equality
// root-only for a v5 block whose committed-state effect is entries/revocations (E/R) PLUS a set
// of non-proposer attestations that write validatorsSeen (A). It stalls loud on every other class.
//
// WHY A NEEDS THE DIGEST PRIMITIVE. The A-write (chain.go:3293-3298) fires for every non-proposer
// attester `id` with `attesterQualified(id)`: it sets `validatorsSeen[id] = true` (ADD-ONLY —
// apply never deletes validatorsSeen). Measured leaf diff (docs/thinking/2026-08-31-...-AP-options):
//   - validatorsSeen||id  ADD (Present)      per qualifying non-proposer attester
//   - validatorsSeenRoot   CHANGE             the whole-set digest over the post-set
// The digest scalar (nodeSetMTHFromBool, statehash.go:266) is an RFC-6962 MTH over the WHOLE
// post-state validatorsSeen id-set — a whole-list fold with no incremental update — so the box
// reconstructs it from the whole post-set id-list, exactly like classes S/B/T.
//
// THE QUALIFICATION SCREEN — O(payload) POINT reads, box-computed (C-1/C-6). The box computes each
// attester's qualification ITSELF from own-cfg over witnessed inputs, never a witness verdict:
//   1. slashed[id]  — the F2 gate (present ⇒ never qualified). Point read, changed-leaf witness.
//   2. mature objective epoch: epochSet[id] MEMBERSHIP (weight discarded) against the FROZEN set
//      (R-A-membership-source — read epochSet, NOT live bonded; a bond that joins mid-epoch is not
//      yet in epochSet and its att does NOT write validatorsSeen). Point membership witness.
//   3. pre-maturity objective: bonded[id] >= MinBond || launchAnchor(id) — own-cfg MinBond +
//      own-cfg Anchors/handedOff. Point read of bonded[id].
//   4. legacy mode: rep(id) — NOT a committed leaf. R-A-legacy: assert objective-mode, STALL.
// The screen is O(|atts|) point proofs; the validatorsSeenRoot reconstruction is O(|validatorsSeen|)
// and dominates (R-cost-wholeset). A forged screen input fails its own changed-leaf/point proof
// against prevStateRoot OR yields a wrong post-set ⇒ post-root != StateRoot ⇒ stall. No wrong-accept.
//
// COST — HONEST. NOT O(payload). O(|atts|) screen + O(|validatorsSeen|) digest ≈ O(registry). Rides
// R-membership (OPEN, load-bearing for the #657 accept-flip).

// StateRootAttScreen carries, for ONE non-proposer attester, the committed pre-state qualification
// inputs the box reads to compute whether the att writes validatorsSeen: the slashed[id] flag, the
// mature-epoch epochSet[id] membership, and (pre-maturity) the bonded[id] presence. It is UNTRUSTED
// — the box computes qualification itself from own-cfg over these, and any wrong claim either fails
// its own changed-leaf proof (for the validatorsSeen write-target) or diverges the post-digest
// (fold-caught). The membership/point reads themselves are proven against prevStateRoot via the
// per-attester witnesses the box already requires for the validatorsSeen write-target and the digest.
type StateRootAttScreen struct {
	// Attester is the attester id (a.AttesterID()).
	Attester ports.NodeID
	// Slashed reports the committed slashed[id] pre-state (F2 gate: true ⇒ not qualified).
	Slashed bool
	// InEpochSet reports the committed epochSet[id] membership pre-state (mature-epoch screen).
	InEpochSet bool
	// BondedSize is the committed bonded[id] pre-state (0 if absent). Pre-maturity screen.
	BondedSize int64
	// BondedPresent reports whether bonded[id] is present pre-state.
	BondedPresent bool
}

// stateRootAttWriteSet derives the class-A per-member committed-leaf write-set for block b,
// reproducing apply()'s attestation loop (chain.go:3293-3298) LEAF EFFECT: one validatorsSeen||id
// ADD (Present) per non-proposer attester the box computes to be qualified. It is ADD-ONLY (apply
// never deletes validatorsSeen). An attester already in validatorsSeen pre-state is an idempotent
// re-set (still emitted; the leaf value is unchanged, so it does not move validatorsSeenRoot — the
// digest emit is gated on the post-SET differing from the pre-SET, see stateRootAttDigestOp).
//
// The screen is computed from own-cfg over the anchored pre-slashed set + the per-attester point
// witnesses (C-1) — never a witness verdict. It returns the post-validatorsSeen id-set for the
// digest reconstruction and the write-set for the fold.
func (c *Chain) stateRootAttWriteSet(
	b Block,
	preValidatorsSeen map[ports.NodeID]struct{},
	screens map[ports.NodeID]StateRootAttScreen,
) ([]stateRootWrite, map[ports.NodeID]struct{}, error) {
	// R-A-legacy: the legacy branch falls to rep(id), which is NOT a committed leaf, so the box
	// cannot reproduce it from committed state. A v5 block is objective by construction, but assert
	// it and STALL otherwise (never guess legacy qualification).
	if !c.objective() {
		return nil, nil, fmt.Errorf("%w: class-A screen requires objective mode (legacy rep(id) is not a committed leaf)",
			ErrRecomputeStateRootScopeStall)
	}

	proposer := b.ProposerID()
	postSeen := cloneIDSet(preValidatorsSeen)
	seen := map[ports.NodeID]struct{}{} // dedup: a block may repeat an attester id
	var writes []stateRootWrite
	for i := range b.Atts {
		id := b.Atts[i].AttesterID()
		if id == proposer {
			continue // apply() skips the proposer's own attestation (chain.go:3295)
		}
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		sc, ok := screens[id]
		if !ok {
			return nil, nil, fmt.Errorf("%w: no attester screen witness for non-proposer att %x", ErrRecomputeStateRootDigest, id[:])
		}
		if !c.attesterQualifiedFromScreen(sc) {
			continue // not qualified ⇒ no validatorsSeen write
		}
		// Qualified non-proposer att ⇒ validatorsSeen||id ADD (Present). Idempotent if already seen.
		writes = append(writes, stateRootWrite{key: statehash.Key(tagValidatorsSeen, id[:]), newValue: statehash.Present})
		postSeen[id] = struct{}{}
	}
	return writes, postSeen, nil
}

// attesterQualifiedFromScreen computes attesterQualifiedAt(id, 0) (chain.go:1279-1303) from the
// witnessed committed pre-state inputs + the box's OWN cfg (C-6) — never a witness verdict. It
// reproduces the height-0 form (height 0 is never a #535 recovery boundary, so effectiveEpochSet(0)
// is the frozen epochSet). Legacy mode is unreachable here (the caller asserts objective).
func (c *Chain) attesterQualifiedFromScreen(sc StateRootAttScreen) bool {
	if sc.Slashed {
		return false // F2: the one live mid-epoch disqualification
	}
	if c.epochsEnabled() && c.matureEpoch {
		// R-A-membership-source: the mature-epoch screen is FROZEN epochSet membership, weight
		// discarded. A bond that joined mid-epoch is NOT yet in epochSet, so its att does not write.
		return sc.InEpochSet
	}
	// Pre-maturity objective: committed bonded size clears MinBond OR a launch anchor bootstraps.
	return (sc.BondedPresent && sc.BondedSize >= c.cfg.MinBond) || c.launchAnchor(sc.Attester)
}

// stateRootAttDigestOp reconstructs the validatorsSeenRoot whole-set digest as a FoldOp, IFF the
// post-validatorsSeen id-SET differs from the anchored pre-set. It anchors the pre-set against the
// committed pre-digest (verified against prevStateRoot by the fold, R-anchor-prevroot), and folds
// the post-digest as the changed leaf. If the set did not change (every attester was already seen,
// or no att qualified), no digest op is emitted (the digest value is unchanged).
func stateRootAttDigestOp(
	preValidatorsSeen, postValidatorsSeen map[ports.NodeID]struct{},
	digestWits []StateRootDigestWitness,
) ([]statehash.FoldOp, error) {
	if idSetsEqual(preValidatorsSeen, postValidatorsSeen) {
		return nil, nil // no membership change ⇒ validatorsSeenRoot unchanged ⇒ no fold op
	}
	byTag := make(map[string]*StateRootDigestWitness, len(digestWits))
	for i := range digestWits {
		byTag[digestWits[i].Tag] = &digestWits[i]
	}
	if _, ok := byTag[tagValidatorsSeenRoot]; !ok {
		return nil, fmt.Errorf("%w: validatorsSeen membership changed but no validatorsSeenRoot pre-set witness", ErrRecomputeStateRootDigest)
	}
	return []statehash.FoldOp{digestFoldOp(tagValidatorsSeenRoot, byTag, postValidatorsSeen)}, nil
}

// attOps is the class-A assembly the recompute entry calls: it anchors the pre-validatorsSeen set
// from the validatorsSeenRoot digest witness, screens each non-proposer attester (own-cfg over the
// per-attester witnesses), derives the write-set + post-set, and reconstructs the touched digest.
// It returns the digest FoldOps and the per-member write-set the caller folds together.
func (c *Chain) attOps(b Block, w StateRootWitness) ([]statehash.FoldOp, []stateRootWrite, error) {
	byTag := make(map[string]*StateRootDigestWitness, len(w.DigestPreSets))
	for i := range w.DigestPreSets {
		byTag[w.DigestPreSets[i].Tag] = &w.DigestPreSets[i]
	}
	preSeen, err := anchoredPreSet(byTag, tagValidatorsSeenRoot)
	if err != nil {
		return nil, nil, err
	}
	screens := make(map[ports.NodeID]StateRootAttScreen, len(w.AttScreens))
	for _, sc := range w.AttScreens {
		screens[sc.Attester] = sc
	}
	writes, postSeen, err := c.stateRootAttWriteSet(b, preSeen, screens)
	if err != nil {
		return nil, nil, err
	}
	ops, err := stateRootAttDigestOp(preSeen, postSeen, w.DigestPreSets)
	if err != nil {
		return nil, nil, err
	}
	return ops, writes, nil
}
