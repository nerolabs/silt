package chain

import (
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-e,
// CLASS A (the LastCommit carrier → validatorsSeen) — the FOURTH changed-whole-set-digest class.
//
// RE-POINTED 2026-09-03 by R-BOX-ATTESTS owner call O1 (gate G8). The class-A input source is the
// block's HASH-COVERED LastCommit carrier — the PARENT's precommits, verified over b.Prev — not
// the block's own uncovered b.Atts. Before the re-point the box derived class A from b.Atts, so a
// peer serving a same-hash copy of a committed block with a legitimately DIFFERENT certificate (a
// same-round superset, shape S5) made the box compute a different post-set and Reject a canonical
// block. With the carrier the derivation is a pure function of hash-covered content, so every
// replica and the box agree on the class-A write-set by construction; the prevStateRoot anchoring
// the screen already had is then exactly right, because for the child prevStateRoot IS the
// parent'"'"'s committed post-state, which is the state the chain'"'"'s applyCarrier screens against.
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
// WHY A NEEDS THE DIGEST PRIMITIVE. The A-write (applyCarrier, carrier.go) fires for every carried
// signer `id` that is not the PARENT'"'"'s proposer and has `attesterQualified(id)` against the
// child'"'"'s pre-state: it sets `validatorsSeen[id] = true` (ADD-ONLY —
// apply never deletes validatorsSeen). Measured leaf diff (docs/thinking/2026-08-31-...-AP-options):
//   - validatorsSeen||id  ADD (Present)      per qualifying carried signer
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
// against prevStateRoot OR yields a wrong post-set ⇒ post-root != StateRoot ⇒ stall.
//
// THE INPUT AND THE SCREEN ARE ANCHORED BY TWO DIFFERENT THINGS. This paragraph used to end "No
// wrong-accept", which was true of the SCREEN and false of the INPUT (red-team RT-CARRIER-1 /
// RT-CARRIER-12, 2026-09-03; PE ruling RULING-floorbox-predicate-rederivation-structure-2026-09-03.md
// §7 merge-condition 3). What is true now:
//
//	INPUT  — b.LastCommit is anchored by the SHARED validity rule. assembleStateRootRecomputeOps
//	         calls validateCarrier(&b) (carrier.go) before any class dispatches: the same function,
//	         on the same bytes, that ValidateProposal and appendStructural run on the full node —
//	         one function, three callers. Every entry must be a genuine PhasePrecommit signature over
//	         the hash-covered b.Prev, ids distinct, no sub-v5 carrier, no height-1 carrier. A carrier
//	         the node refuses can no longer produce a class-A write-set at all; the box stalls with
//	         ErrRecomputeCarrierInvalid. BEFORE that call the write-set was derived from
//	         b.LastCommit[i].AttesterID() with no signature check, so a carrier of PUBLIC keys and
//	         zero-byte signatures — no key material — seated arbitrary ids against the attacker's own
//	         apply()-computed root while every full node rejected the block.
//	SCREEN — the per-signer qualification inputs (slashed / epochSet / bonded) are anchored by their
//	         own point proofs against prevStateRoot, and the write is fold-caught. That is the claim
//	         this paragraph always supported.
//	RESIDUAL — one class-A input is still anchored only PARTIALLY: the parent-proposer exclusion
//	         (R-CARRIER-PARENTPROPOSER, ADD direction). See the ParentProposer field doc in
//	         floorbox_recompute_stateroot_v5.go. It is a named flip precondition, not covered here.
//
// The box's verdict remains a STALL-or-agree: box.Accept ⇒ node.Accept, never the biconditional
// (PE ruling O-2). The box is permitted to stall where the node accepts; it must never agree where
// the node rejects.
//
// COST — HONEST. O(|b.LastCommit|) screen + O(|validatorsSeen|) digest. The DIGEST term is
// ≈ O(registry) and rides R-membership (OPEN, load-bearing for the #657 accept-flip). The SCREEN
// term does NOT: validateCarrier applies no qualification screen, so |b.LastCommit| is bounded by
// nothing but the transport frame (~1.3M entries in 132 MiB), and an earlier version of this line
// that folded it into "≈ O(registry)" was wrong. See R-CARRIER-BYTES (ROADMAP.md, Boulder 1
// carry-list) — a size rule on hash-covered content is a v5 validity rule and needs certification.

// StateRootAttScreen carries, for ONE carried non-parent-proposer signer, the committed pre-state qualification
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

	// R1.2 WITNESS-SOUNDNESS ANCHORS (per-field proofs against prevStateRoot). Each predicate the
	// screen reads is anchored the same way a fold-written leaf is: by a proof the box VERIFIES
	// against prevStateRoot before the read is trusted. Without them a forged screen flips
	// qualification and emits a spurious validatorsSeen||id ADD (class-A source of the class-M
	// poisoning, PE ruling Q2). The box resolves each proof and STALLS (MustStall) on a nil/forged
	// proof — never falls through to a false/absent read (C-7 §104 banned move).
	//
	// SlashedProof anchors Slashed: present-proof of slashed||id (Slashed=true) OR non-membership
	// proof (Slashed=false), against prevStateRoot.
	SlashedProof statehash.Witness
	// EpochSetProof anchors InEpochSet: present-proof of epochSet||id at the committed frozen weight
	// (InEpochSet=true) OR non-membership proof (InEpochSet=false). Membership is all the mature-epoch
	// screen needs (weight discarded, R-A-membership-source); the box requires the presence/absence
	// proof to verify and does not read the weight.
	EpochSetProof statehash.Witness
	// EpochSetValue is the committed epochSet||id leaf value (the frozen weight) the EpochSetProof
	// proves present when InEpochSet=true. Nil/empty when InEpochSet=false (an absence proof).
	EpochSetValue []byte
	// BondedProof anchors BondedPresent/BondedSize: present-proof of bonded||id at EncodeInt64(BondedSize)
	// (BondedPresent=true) OR non-membership proof (BondedPresent=false), against prevStateRoot.
	BondedProof statehash.Witness
}

// stateRootAttWriteSet derives the class-A per-member committed-leaf write-set for block b,
// reproducing applyCarrier'"'"'s LEAF EFFECT (carrier.go): one validatorsSeen||id ADD (Present) per
// carried signer the box computes to be qualified, excluding the parent'"'"'s proposer. The carrier
// fold runs BEFORE this block'"'"'s bond regs / TTL / slashes in apply(), which is why the screen'"'"'s
// prevStateRoot anchoring is the correct pre-state. It is ADD-ONLY (apply
// never deletes validatorsSeen). An attester already in validatorsSeen pre-state is an idempotent
// re-set (still emitted; the leaf value is unchanged, so it does not move validatorsSeenRoot — the
// digest emit is gated on the post-SET differing from the pre-SET, see stateRootAttDigestOp).
//
// The screen is computed from own-cfg over the anchored pre-slashed set + the per-attester point
// witnesses (C-1) — never a witness verdict. It returns the post-validatorsSeen id-set for the
// digest reconstruction and the write-set for the fold.
func (c *Chain) stateRootAttWriteSet(
	prevStateRoot ports.Hash,
	b Block,
	preValidatorsSeen map[ports.NodeID]struct{},
	screens map[ports.NodeID]StateRootAttScreen,
	pre stateRootHandoffPre,
	parentProposerPub, parentProposerSig []byte,
) ([]stateRootWrite, map[ports.NodeID]struct{}, error) {
	// R-A-legacy: the legacy branch falls to rep(id), which is NOT a committed leaf, so the box
	// cannot reproduce it from committed state. A v5 block is objective by construction, but assert
	// it and STALL otherwise (never guess legacy qualification).
	if !c.objective() {
		return nil, nil, fmt.Errorf("%w: class-A screen requires objective mode (legacy rep(id) is not a committed leaf)",
			ErrRecomputeStateRootScopeStall)
	}

	// R-BOX-ATTESTS (O1): the v5 seating source is the HASH-COVERED LastCommit carrier, never
	// the block's own uncovered Atts. The box is v5-only; a sub-v5 block's frozen b.Atts rule is
	// not reproduced here (it is the defect the carrier closes) — stall rather than guess.
	if b.Version < BlockVersionWitnessable {
		return nil, nil, fmt.Errorf("%w: class-A reproduces the era-4 (v5) carrier rule; block is v%d",
			ErrRecomputeStateRootScopeStall, b.Version)
	}
	// The excluded id is the PARENT's proposer (the carrier republishes precommits over b.Prev,
	// so the block being attested is the parent). Anchored against the hash-covered b.Prev by the
	// parent's own proposer signature; a missing/forged pair stalls.
	var parentProposer ports.NodeID
	if len(b.LastCommit) > 0 {
		var pErr error
		parentProposer, pErr = carrierParentProposerFromWitness(b.Prev, parentProposerPub, parentProposerSig)
		if pErr != nil {
			return nil, nil, pErr
		}
	}
	postSeen := cloneIDSet(preValidatorsSeen)
	seen := map[ports.NodeID]struct{}{} // dedup: a block may repeat an attester id
	var writes []stateRootWrite
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if id == parentProposer {
			continue // the parent's proposer does not seat itself off its own block (applyCarrier)
		}
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		sc, ok := screens[id]
		if !ok {
			return nil, nil, fmt.Errorf("%w: no attester screen witness for carried non-proposer signer %x", ErrRecomputeStateRootDigest, id[:])
		}
		qualified, qErr := c.attesterQualifiedFromScreen(prevStateRoot, sc, pre)
		if qErr != nil {
			return nil, nil, qErr // a screen predicate could not be anchored against prevStateRoot ⇒ stall
		}
		if !qualified {
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
//
// `pre` is the box's ANCHORED committed pre-state handoff view (handoffPreState). Every value this
// screen consults is now either witness-Resolved against prevStateRoot or own-cfg (C-6) — NO live
// box state decides a branch or a value (R-FOLD-LIVE-STATE-READS cert 2026-09-02, Q1/Q3; pinned by
// the fold-file c.<selector> allowlist, floorbox_recompute_foldlivestate_pin_v5_test.go).
//
// R1.2: every screen predicate is ANCHORED against prevStateRoot BEFORE it is read (PE ruling Q1
// invariant #2, the poisoning entry point). A forged Slashed/InEpochSet/BondedPresent/BondedSize
// flips qualification and emits a spurious validatorsSeen||id ADD (class-M poisoning source). Each
// read is trusted only after its proof Resolves against prevStateRoot; a nil/forged proof yields
// NoWitness ⇒ stall (never a false/absent read, C-7 §104). Returns (qualified, stall-reason).
func (c *Chain) attesterQualifiedFromScreen(prevStateRoot ports.Hash, sc StateRootAttScreen, pre stateRootHandoffPre) (bool, error) {
	id := sc.Attester

	// F2 gate — the slashed[id] pre-state. Anchor Slashed either way: present ⇒ inclusion proof of
	// slashed||id → Present; absent ⇒ non-membership proof. A forged Slashed=false for a truly-slashed
	// attester (the ForgedSlashed attack) fails the absence proof against prevStateRoot ⇒ stall.
	slashedKey := statehash.Key(tagSlashed, id[:])
	var slashedVal []byte
	if sc.Slashed {
		slashedVal = statehash.Present
	}
	slashedRes := statehash.Resolve(prevStateRoot, slashedKey, slashedVal, sc.SlashedProof)
	if sc.Slashed && !slashedRes.IsProvenPresent() {
		return false, fmt.Errorf("%w: attester %x Slashed=true not proven present against prevStateRoot", ErrRecomputeStateRootDigest, id[:])
	}
	if !sc.Slashed && !slashedRes.IsProvenAbsent() {
		return false, fmt.Errorf("%w: attester %x Slashed=false not proven absent against prevStateRoot", ErrRecomputeStateRootDigest, id[:])
	}
	if sc.Slashed {
		return false, nil // F2: the one live mid-epoch disqualification
	}

	// THE BRANCH SELECTOR — the ANCHORED committed pre-state, never c.matureEpoch
	// (R-FOLD-LIVE-STATE-READS cert 2026-09-02). apply()'s attestation loop (chain.go:3293-3298) runs
	// BEFORE rotateEpoch (chain.go:3306, rotate-LAST), so the value a full node screens against is the
	// PRE-state matureEpoch committed in prevStateRoot — exactly pre.matureEpoch, Resolved present by
	// handoffPreState before this dispatch. A box that replays no apply() has no c.matureEpoch to read;
	// reading it made every mature-epoch block take the pre-maturity branch (wrong-accept of a
	// mid-epoch joiner against an attacker root, false stall on an honest one).
	if c.epochsEnabled() && pre.matureEpoch {
		// R-A-membership-source: the mature-epoch screen is FROZEN epochSet membership, weight
		// discarded. Anchor InEpochSet: present ⇒ inclusion proof of epochSet||id at its committed
		// weight; absent ⇒ non-membership proof. A forged InEpochSet=true for a non-member (the
		// ForgedInEpochSet attack) fails the presence proof ⇒ stall.
		epochSetKey := statehash.Key(tagEpochSet, id[:])
		var epochSetVal []byte
		if sc.InEpochSet {
			epochSetVal = sc.EpochSetValue // the committed frozen weight; membership is what the screen uses
		}
		epochSetRes := statehash.Resolve(prevStateRoot, epochSetKey, epochSetVal, sc.EpochSetProof)
		if sc.InEpochSet && !epochSetRes.IsProvenPresent() {
			return false, fmt.Errorf("%w: attester %x InEpochSet=true not proven present against prevStateRoot", ErrRecomputeStateRootDigest, id[:])
		}
		if !sc.InEpochSet && !epochSetRes.IsProvenAbsent() {
			return false, fmt.Errorf("%w: attester %x InEpochSet=false not proven absent against prevStateRoot", ErrRecomputeStateRootDigest, id[:])
		}
		return sc.InEpochSet, nil
	}

	// Pre-maturity objective: committed bonded size clears MinBond OR a launch anchor bootstraps. Anchor
	// BondedPresent/BondedSize: present ⇒ inclusion proof of bonded||id → EncodeInt64(BondedSize); absent
	// ⇒ non-membership proof. A forged BondedPresent=true / BondedSize>=MinBond for an under/un-bonded
	// attester (the ForgedBondedSize / ForgedBondedPresent attacks) fails its proof ⇒ stall. launchAnchor
	// is own-cfg (C-6), never a witness — no anchor needed.
	bondedKey := statehash.Key(tagBonded, id[:])
	var bondedVal []byte
	if sc.BondedPresent {
		bondedVal = statehash.EncodeInt64(sc.BondedSize)
	}
	bondedRes := statehash.Resolve(prevStateRoot, bondedKey, bondedVal, sc.BondedProof)
	if sc.BondedPresent && !bondedRes.IsProvenPresent() {
		return false, fmt.Errorf("%w: attester %x BondedPresent=true (size %d) not proven present against prevStateRoot", ErrRecomputeStateRootDigest, id[:], sc.BondedSize)
	}
	if !sc.BondedPresent && !bondedRes.IsProvenAbsent() {
		return false, fmt.Errorf("%w: attester %x BondedPresent=false not proven absent against prevStateRoot", ErrRecomputeStateRootDigest, id[:])
	}
	// launchAnchorGiven is the SAME predicate the live node uses (chain.go), parameterized on the
	// handoff bool rather than reading c.handedOff() — the #402 non-fork rule, one function, two
	// callers. The box supplies the ANCHORED pre-state handoff (matureEpoch with epochs enabled,
	// everMature without); the live node supplies c.handedOff().
	return (sc.BondedPresent && sc.BondedSize >= c.cfg.MinBond) || c.launchAnchorGiven(sc.Attester, pre.handedOff), nil
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
// from the validatorsSeenRoot digest witness, screens each carried signer (own-cfg over the
// per-attester witnesses), derives the write-set + post-set, and reconstructs the touched digest.
// It returns the digest FoldOps and the per-member write-set the caller folds together.
func (c *Chain) attOps(prevStateRoot ports.Hash, b Block, w StateRootWitness, pre stateRootHandoffPre) ([]statehash.FoldOp, []stateRootWrite, error) {
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
	writes, postSeen, err := c.stateRootAttWriteSet(prevStateRoot, b, preSeen, screens, pre, w.ParentProposer, w.ParentProposerSig)
	if err != nil {
		return nil, nil, err
	}
	ops, err := stateRootAttDigestOp(preSeen, postSeen, w.DigestPreSets)
	if err != nil {
		return nil, nil, err
	}
	return ops, writes, nil
}
