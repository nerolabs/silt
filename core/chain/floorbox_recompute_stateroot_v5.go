package chain

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-a,
// O(payload) HYBRID (certified 2026-08-31, superseding the O(whole-state) P1-a).
//
// This file reproduces validateEra3Roots' StateRoot equality check (era3validity.go) — the
// committed StateRoot MUST equal the SMT recomputed over the POST-APPLY committed leaf set —
// TRUSTLESSLY, from two committed roots + CHANGED-PATH witnesses ALONE, at O(payload) cost (NOT
// O(whole-state)). It is the FIRST sub-increment of the Path-1 recompute (PACE:
// docs/thinking/2026-08-31-floorbox-recompute-Rfold-options.md); the full validateEra3Roots
// recompute spans eight apply() transition classes. P1-a lands the ROOT-EQUALITY SPINE every
// later class reuses, on the two classes that are pure payload-driven set writes with NO
// membership screen: entries (byRoot / spent) and revocations / un-revocations (revoked).
//
// THE CERTIFIED HYBRID (replaces the whole-pre-state transfer). The box:
//  1. DERIVES the E/R write-set from the block payload itself — it runs the write-set generator
//     (applyEntriesRevocationsWriteSet), NOT the prover. For E/R the changed KEYS are a pure
//     function of b.Entries / b.Revocations / b.Unrevocations, so the derived set is complete by
//     construction (research cert sub-Q1): there is no un-named leaf the prover can hide.
//  2. WITNESSES each changed leaf with a pre-state proof against prevStateRoot (O(|write-set| ·
//     log N), NOT the whole pre-state).
//  3. FOLDS only the changed paths to COMPUTE the post-state root (statehash.FoldChangedPaths —
//     the R-fold primitive, pinned byte-exact against statehash.Root over the structural
//     cross-product, fold_test.go).
//  4. Requires the computed root == b.StateRoot. Deriving the write-set closes completeness
//     (nothing else changed); the fold catches any extra / omitted / mis-valued change (an
//     un-named change diverges the honest root from the forged committed root).
//
// THE SCOPE GATE, RE-ANCHORED ON dueBucket (or O(payload) is false). The superseded whole-state
// P1-a scanned the WHOLE bondRegHeight map to detect a firing TTL expiry — O(whole-state), which
// would force a whole-state witness even under the fold. This box re-anchors that decision on the
// dueBucket[h] accelerator: a TTL expiry fires at height h IFF dueBucket[uint64BE(h)] is OCCUPIED
// (chain.go:3274, readset_v5.go:605). The box tests it with ONE non-membership witness of
// dueBucket[uint64BE(b.Height)] against prevStateRoot — O(1), no whole-map scan.
//
// It is ADDITIVE: it calls no full-node accept path, mutates nothing, and changes NO
// consensus/validity rule. A full node still recomputes the root by cloneForDryRun + apply +
// StateRootForVersion (era3validity.go, chain.go untouched). This is a SEPARATE root-only path a
// semi-stateless box calls INSTEAD of cloning the whole state and replaying apply().
//
// STOP BOUNDARY (this sub-increment). It reproduces the root-equality MECHANISM on classes E + R
// only; classes S/A (screens), T (TTL), B (bond regs), P (rotation), M (maturity) are later
// sub-increments. It does NOT flip #657 WitnessValidateV5 to Accept — the box STILL never-Accepts
// (research cert R-scope: do not flip Accept for E/R until R-fold is fully pinned AND owner-
// ratified; this increment keeps never-Accept). It changes NO apply() rule.

var (
	// ErrRecomputeStateRootScopeStall marks a stall where the block carries a transition class this
	// P1-a sub-increment does not reproduce (a BondReg, a Slash, a validatorsSeen-writing Att, a
	// firing TTL expiry, or an epoch boundary). The box never-Accepts an out-of-scope block; it
	// stalls loud so a later sub-increment's absence can never be a silent wrong-Accept.
	ErrRecomputeStateRootScopeStall = errors.New("chain: floor-box O(payload) state-root recompute — block carries a transition class outside P1-a scope (bond reg / slash / seen-writing att / firing TTL expiry / epoch boundary); the box stalls, never Accepts")

	// ErrRecomputeStateRootTTLWitness marks a stall where the dueBucket[h] scope-gate witness is
	// not a verified NON-MEMBERSHIP proof against prevStateRoot: the box cannot prove NO TTL expiry
	// fires at b.Height, so it cannot rule the block E/R-only. It stalls (never guesses absence).
	ErrRecomputeStateRootTTLWitness = errors.New("chain: floor-box O(payload) state-root recompute — dueBucket TTL scope-gate witness did not prove non-membership against prevStateRoot; cannot rule the block E/R-only, stalling")

	// ErrRecomputeStateRootFold marks a stall from the R-fold primitive: a changed-leaf proof failed
	// to verify against prevStateRoot, or the library replay of a payload write failed. The box
	// stalls rather than recompute over an unverified change.
	ErrRecomputeStateRootFold = errors.New("chain: floor-box O(payload) state-root recompute — the changed-path fold could not compute a post-state root (a changed-leaf proof failed to verify, or a payload write replay failed)")

	// ErrRecomputeStateRootMismatch marks the terminal stall: the fold's recomputed post-state root
	// does not equal the block's committed StateRoot. This is validateEra3Roots'
	// ErrEra3StateRootMismatch reproduced root-only — a forged committed leaf, an omitted / injected
	// payload write, or a tampered committed StateRoot all land here.
	ErrRecomputeStateRootMismatch = errors.New("chain: floor-box O(payload) state-root recompute — recomputed post-state root does not equal the block's committed StateRoot")

	// ErrRecomputeStateRootMaturity marks a stall in the class-M everMature latch reconstruction: the
	// class-M maturity witness is missing (so the box cannot rule out an everMature crossing), or the
	// maturity recompute (RecomputeMatureNow) could not verify the SeenSet witness against the committed
	// root. The box will not fold — or omit — the tagEverMature write on an unverified maturity verdict;
	// it stalls (never-Accept preserved).
	ErrRecomputeStateRootMaturity = errors.New("chain: floor-box state-root recompute — the class-M everMature latch could not be reconstructed (no maturity witness, or the maturity recompute could not verify the SeenSet against the committed root)")

	// ErrRecomputeStateRootDigest marks a stall in the class-S changed-whole-set-digest reconstruction
	// (P1-b): a touched digest (slashedRoot / bondedRoot / qualifiedRoot) with no supplied pre-set
	// witness, or a pre-set id-list that does not reconstruct the committed pre-digest. The box will
	// not fold an unwitnessed / uncompleteness-anchored digest change; it stalls.
	ErrRecomputeStateRootDigest = errors.New("chain: floor-box state-root recompute — a class-S touched whole-set digest (slashed/bonded/qualified root) is missing its pre-set witness or its pre-set id-list does not reconstruct the committed pre-digest")

	// ErrRecomputeBoxWiring marks a stall at the BOX ENTRY: the injected bond verifier is not wired,
	// so objective() / epochsEnabled() are false for a WIRING reason rather than a config one
	// (R-VERIFYBOND-WIRING, R-FOLD-LIVE-STATE-READS cert 2026-09-02 Q4 row 3).
	//
	// This is the #572 replay shape, recorded and fixed once already (PR #582): a chain replayed its
	// history BEFORE the verifier was wired, so objective() was false during replay, qualification fell
	// to the legacy rep path over an empty ledger, and the everMature latch never tripped. In the box
	// the same mis-wiring silently flips the class-A screen branch and the isBoundary scope gate. It is
	// stall-only either way, but a fold mismatch three classes later is the WRONG place to learn it —
	// the entry asserts it so the failure is LOUD and names the cause.
	ErrRecomputeBoxWiring = errors.New("chain: floor-box state-root recompute — the box has no bond verifier wired (SetBondVerifier), so objective()/epochsEnabled() would silently take the legacy branch; the box stalls at the entry rather than screening under the wrong rule (#572 replay shape)")
)

// StateRootChangedLeafWitness is the pre-state proof for ONE payload-changed leaf, supplied to the
// O(payload) box by an any-of-N provider. It is UNTRUSTED: RecomputeStateRootEntriesRevocations
// verifies each proof against prevStateRoot (via the R-fold) before folding it, so a forged /
// omitted / mis-valued proof stalls.
type StateRootChangedLeafWitness struct {
	// Key is the field-tagged leaf key of the changed leaf (statehash.Key(tag, rawKey)). The box
	// DERIVES the expected key set from the payload and matches each witness to a derived key; a
	// witness for a key not in the derived set is ignored (the derived set is authoritative).
	Key []byte
	// OldValue is the CLAIMED committed pre-state value of Key: nil/empty for a key absent pre-apply
	// (an add — a non-membership proof), non-nil (Present) for a present key (a delete or an
	// idempotent overwrite of a set-marker — a membership proof). The fold VERIFIES this claim
	// against prevStateRoot, so a false pre-state claim stalls. The box additionally enforces the
	// claim is CONSISTENT with the derived write: an un-revocation delete MUST claim a present
	// pre-state (you cannot delete what is absent).
	OldValue []byte
	// Proof is the pre-state witness of Key against prevStateRoot: membership when OldValue is
	// non-empty, non-membership when empty.
	Proof statehash.Witness
	// DeleteSiblings are the off-path sibling nodes a delete's replay resolves (empty for a
	// non-delete). Supplied by the provider; verified faithful by the fold's final root equality.
	DeleteSiblings []statehash.FoldSibling
}

// StateRootWitness is the complete O(payload) witness bundle: the per-changed-leaf proofs plus the
// dueBucket TTL scope-gate non-membership proof. It carries NO whole-pre-state (that was the
// superseded O(whole-state) anchor). Its size is O(|payload write-set| · log N) + O(log N), flat in
// the total state size.
type StateRootWitness struct {
	// ChangedLeaves is one witness per payload-derived changed leaf. The box derives the changed-key
	// SET from the payload and requires a matching verified witness for each; a missing witness
	// stalls the fold (an unverified change is never folded).
	ChangedLeaves []StateRootChangedLeafWitness
	// DueBucketProof proves dueBucket[uint64BE(b.Height)] is ABSENT under prevStateRoot — i.e. NO
	// TTL expiry fires at b.Height, so the block can be E/R-only. A membership proof (an occupied
	// bucket) or a missing/failed proof stalls the scope gate. Supplied only when BondTTLBlocks > 0.
	DueBucketProof statehash.Witness
	// DigestPreSets carries, for each touched whole-set digest scalar (slashedRoot / bondedRoot /
	// qualifiedRoot), the claimed pre-state member id-list + the digest leaf inclusion proof against
	// prevStateRoot. Empty for an E/R-only block (no digest scalar changes). The box reconstructs
	// each pre-set MTH and requires it equals the committed pre-digest (completeness anchor), then
	// folds the payload/accelerator-derived post-digest (class S slashes, class B bond regs, class T
	// TTL sweep).
	DigestPreSets []StateRootDigestWitness
	// TTLSweep carries the class-T accelerator delta: the members of dueBucket[b.Height] (the expired
	// set) + the bucket MTH inclusion proof + the bucket-delete off-path siblings. Present only for a
	// firing TTL sweep block (P1-c). The box reconstructs dueBucketMTH(Members) and requires it equals
	// the committed bucket value (the CRUX completeness anchor).
	TTLSweep *StateRootTTLWitness
	// BondRegScreens carries, per bond-reg Root, the committed pre-state ownership the class-B
	// displacement branch reads (bondRootOwner / bondRootProven). Present only for a bond-reg block
	// (P1-d). The box derives the B delta from these + its own cfg screens (R-B-displacement).
	BondRegScreens []StateRootBondRegScreen
	// BondRegBuckets carries, per affected TTL due-height, the pre-state bucket member id-list + the
	// bucket leaf proof against prevStateRoot. Present only for a bond-reg block with TTL enabled
	// (the dueBucketMoveOnReg leaf effect). The box reconstructs each affected bucket's MTH.
	BondRegBuckets []StateRootBucketWitness
	// AttScreens carries, per non-proposer attester, the committed pre-state qualification inputs the
	// class-A screen reads (slashed / frozen epochSet membership / bonded). Present only for a block
	// with non-proposer atts (P1-e). The box computes qualification itself from own-cfg over these,
	// then reconstructs the validatorsSeenRoot digest. See floorbox_recompute_stateroot_atts_v5.go.
	AttScreens []StateRootAttScreen
	// ParentProposer / ParentProposerSig carry the PARENT block's proposer public key and its
	// proposer signature — the one class-A input that is NOT committed state. The era-4 carrier
	// transition excludes id == parent.ProposerID() (R-BOX-ATTESTS O1), and the box holds no
	// parent block (WitnessValidateV5 receives only b + parentStateRoot), so the identity must
	// be witnessed. It is ANCHORED, not trusted: the box requires
	// ed25519.Verify(ParentProposer, b.Prev[:], ParentProposerSig) — the same bare-hash
	// proposer-signature arithmetic the chain uses — and b.Prev is hash-covered. Required
	// whenever b.LastCommit is non-empty; a missing or non-verifying pair STALLS (never falls
	// through to "no exclusion", which would seat the parent's proposer). See
	// carrierParentProposerFromWitness (carrier.go) for the exact residual this leaves
	// (R-CARRIER-PARENTPROPOSER: it bounds a forgery to dropping the forger's OWN seat).
	ParentProposer    []byte
	ParentProposerSig []byte
	// Rotate carries the class-P epoch-boundary witness: the pre-qualified id-set (the freeze source),
	// the per-frozen-member regVersion (the activation tallies), the prior epochSet, and the rotate
	// scalar pre-values (epochStart / matureEpoch / the three lock-in scalars). Present only for an
	// epoch-boundary block (P1-e). The box applies the same block's S/B/T qualified deltas FIRST, then
	// freezes (rotate-LAST). See floorbox_recompute_stateroot_rotate_v5.go. The everMature latch is NOT
	// homed here — it is class M (Maturity below), the boundary-independent single owner.
	Rotate *StateRootRotateWitness
	// Maturity carries the class-M everMature latch witness AND the committed young→mature HANDOFF
	// pre-state: the everMature + matureEpoch scalar proofs against prevStateRoot, plus the SeenSet
	// maturity witness. REQUIRED on every block. Class M is the SINGLE owner of the tagEverMature
	// write; it fires on ANY block whose maturity latch flips (boundary-independent), and threads the
	// post-latch everMature into class P for its freeze gate. The matureEpoch pre-value is the CLASS-A
	// screen's branch selector, anchored by handoffPreState before any class dispatches — the box never
	// reads its own latch fields (R-FOLD-LIVE-STATE-READS cert 2026-09-02). See
	// floorbox_recompute_stateroot_maturitylatch_v5.go.
	Maturity *StateRootMaturityWitness
}

// RecomputeStateRootEntriesRevocations reproduces validateEra3Roots' StateRoot equality check
// TRUSTLESSLY for a v5 block whose committed-state effect is its entries and revocations/
// un-revocations (classes E + R) and/or its on-chain equivocation slashes (class S, P1-b). It
// returns nil iff the block's committed StateRoot equals the SMT over the post-apply committed leaf
// set a full node would compute — the same verdict validateEra3Roots reaches — and a stall reason
// otherwise. It NEVER returns "valid" for an out-of-scope block: it stalls loud, never-Accepts.
//
// COST is O(payload) for a pure E/R block; a class-S block is O(payload) + O(|keyspace|) per touched
// whole-set digest (slashed/bonded/qualified) ≈ O(registry) per digest — the digest reconstruction
// is a whole-list MTH fold, NOT O(payload) (R-cost-wholeset). See the P1-b file doc-comment.
//
// prevStateRoot is the previous block's committed StateRoot (the pre-state the changed-leaf proofs
// verify against). committedStateRoot is b.StateRoot. The box holds both roots (attester-signed)
// and the O(payload) witness bundle; it holds NO registry and replays NO apply().
//
// It reads EpochBlocks / epochsEnabled / BondTTLBlocks from the box's OWN cfg (C-6) for the scope
// gate — never from the witness. This does NOT flip WitnessValidateV5 to Accept (the STOP boundary).
func (c *Chain) RecomputeStateRootEntriesRevocations(
	prevStateRoot ports.Hash,
	committedStateRoot ports.Hash,
	b Block,
	w StateRootWitness,
) (reason error) {
	// (1-3) Assemble the fold ops for every reproduced transition class (the write-set → FoldOp
	// pipeline, including the scope gate). A stall here is a never-Accept.
	ops, err := c.assembleStateRootRecomputeOps(prevStateRoot, committedStateRoot, b, w)
	if err != nil {
		return err
	}

	// (4) FOLD the changed paths to compute the post-state root. FoldChangedPaths verifies each
	// changed-leaf proof against prevStateRoot before folding it; a forged / omitted / mis-valued
	// proof stalls here.
	postRoot, err := statehash.FoldChangedPaths(prevStateRoot, ops)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRecomputeStateRootFold, err)
	}

	// (5) POST-STATE EQUALITY. Require the recomputed post-root equals the block's committed
	// StateRoot. This IS validateEra3Roots' equality check, reproduced root-only. A forged committed
	// leaf, an omitted/injected payload write, or a tampered b.StateRoot all diverge the recomputed
	// root ⇒ stall. It does NOT Accept — the caller consumes this verdict under the never-Accept
	// scaffold.
	if postRoot != committedStateRoot {
		return fmt.Errorf("%w: recomputed %x != committed %x",
			ErrRecomputeStateRootMismatch, postRoot, committedStateRoot)
	}
	return nil
}

// assembleStateRootRecomputeOps runs the scope gate, derives the per-class write-set, and matches
// each derived write to its pre-state witness — returning the complete FoldOp set the recompute
// folds for block b (or a stall reason). It is the op-assembly half of
// RecomputeStateRootEntriesRevocations, split out so the emission-keyed completeness guard
// (floorbox_recompute_leafdiff_v5_test.go) can compare the FOLDED key-set against the ground-truth
// committed-leaf diff a real apply() produces — the property that ends the one-at-a-time discovery of
// unreproduced writes. The returned ops carry the folded keys; the caller folds them and checks the
// root.
func (c *Chain) assembleStateRootRecomputeOps(
	prevStateRoot ports.Hash,
	committedStateRoot ports.Hash,
	b Block,
	w StateRootWitness,
) ([]statehash.FoldOp, error) {
	// (0) WIRING ASSERTION — LOUD, at the box entry (R-VERIFYBOND-WIRING). objective() and
	// epochsEnabled() are read as branch predicates by the class-A screen and the isBoundary scope
	// gate; both depend on the INJECTED verifyBond, not on committed state or genesis cfg. An unwired
	// box takes the legacy branch everywhere and fails later as an opaque fold mismatch. Assert once,
	// here, so the #572 replay shape fails at the entry naming its cause.
	if c.verifyBond == nil {
		return nil, ErrRecomputeBoxWiring
	}

	// (0b) HANDOFF PRE-STATE — the DIRECTION-A anchor of the committed everMature / matureEpoch pair
	// against prevStateRoot, run UNCONDITIONALLY before ANY class dispatches (R-FOLD-LIVE-STATE-READS
	// cert 2026-09-02, Q3 step 2). Hoisted ahead of class A because the class-A screen's BRANCH
	// SELECTOR is the pre-state matureEpoch and its anchor-eligibility input is the pre-state handoff;
	// class M below consumes the already-anchored pre-everMature (one Resolve per scalar, not two).
	// Unconditional means a block with no atts and no boundary still anchors, so no branch can
	// suppress the anchor.
	pre, preErr := c.handoffPreState(prevStateRoot, w)
	if preErr != nil {
		return nil, preErr
	}

	// (1) SCOPE GATE. Stall (never-Accept) on any transition class P1-a does not reproduce. The
	// payload-visible classes (BondReg / Slash / non-proposer Att) and the epoch boundary are
	// checked from the block + own cfg; the TTL-expiry class is checked from the O(1) dueBucket
	// non-membership witness (NOT a whole-state scan).
	if reason := c.stateRootScopeGate(prevStateRoot, b, w); reason != nil {
		return nil, reason
	}

	// (2) DERIVE the write-set from the block payload. The box runs the generator, not the prover —
	// so the changed-key set is complete by construction (no un-named leaf escapes). E/R gives the
	// byRoot/spent/revoked leaves; class S (P1-b) gives the slashed/bonded/qualified per-member leaves
	// PLUS the three changed whole-set digest scalars, reconstructed via the certified changed-digest
	// primitive. The digest ops are built FIRST because they anchor the pre-bonded / pre-qualified
	// membership the S per-member write-set consumes (so the per-member delta and the digest delta
	// agree on the pre-state, and neither trusts a witness scalar — C-1).
	writeSet := applyEntriesRevocationsWriteSet(b)
	var digestOps []statehash.FoldOp
	// postQualified is the POST-apply qualified id-SET a boundary (class P) freezes (rotate-LAST,
	// R-P-sameblock-order). It is reconstructed by a DEDICATED pass in apply() order (B → T → S) on the
	// anchored pre-qualified set, AFTER the digest ops (whose emission order is irrelevant — each
	// touched digest is a pure function of pre + its own delta). Built only when a boundary needs it.
	isBoundary := c.epochsEnabled() && c.cfg.EpochBlocks > 0 && b.Height%c.cfg.EpochBlocks == 0

	// Class S (slashes, P1-b): reconstruct the three touched digests + the per-member write-set.
	if len(b.Slashes) > 0 {
		dOps, preBonded, preQualified, dErr := stateRootSlashDigestOps(b, w.DigestPreSets)
		if dErr != nil {
			return nil, dErr
		}
		digestOps = append(digestOps, dOps...)
		writeSet = append(writeSet, stateRootSlashWriteSet(b, preBonded, preQualified)...)
	}
	// Class B (bond regs, P1-d): derive the delta from b.BondRegs + own-cfg screens + the per-root
	// displacement witnesses, then reconstruct the touched digests + affected dueBucket leaves.
	if len(b.BondRegs) > 0 {
		bOps, bWrites, bErr := c.bondRegOps(prevStateRoot, b, w)
		if bErr != nil {
			return nil, bErr
		}
		digestOps = append(digestOps, bOps...)
		writeSet = append(writeSet, bWrites...)
	}
	// Class T (TTL sweep, P1-c): derive the expired set from the dueBucket[b.Height] accelerator
	// witness, then reconstruct the touched digests + the bucket DELETE.
	if w.TTLSweep != nil {
		tOps, preBonded, preQualified, expired, tErr := stateRootTTLDigestOps(*w.TTLSweep, w.DigestPreSets)
		if tErr != nil {
			return nil, tErr
		}
		digestOps = append(digestOps, tOps...)
		writeSet = append(writeSet, stateRootTTLWriteSet(expired, w.TTLSweep.Height, preQualified)...)
		_ = preBonded
	}
	// Class A (the LastCommit carrier → validatorsSeen, P1-e): screen each carried signer from
	// own-cfg over the per-attester witnesses, derive the validatorsSeen ADDs, reconstruct
	// validatorsSeenRoot. The source is the HASH-COVERED carrier (R-BOX-ATTESTS O1), not b.Atts.
	if hasCarrierSigners(b) {
		aOps, aWrites, aErr := c.attOps(prevStateRoot, b, w, pre)
		if aErr != nil {
			return nil, aErr
		}
		digestOps = append(digestOps, aOps...)
		writeSet = append(writeSet, aWrites...)
	}
	// Class M (everMature maturity latch): the BOUNDARY-INDEPENDENT single owner of the tagEverMature
	// write. apply() latches everMature false→true on ANY block where !everMature && Mature()
	// (chain.go:3303-3305), BEFORE the boundary gate — so M dispatches every block, not only at a
	// boundary. It reuses RecomputeMatureNow over committedStateRoot (#668, not a rebuild), emits the
	// tagEverMature op on the crossing, and reports the POST-latch everMature the block commits. The
	// post value is threaded into class P (below) so P can gate its freeze on it WITHOUT re-emitting
	// (single owner — no double-emit at a boundary-coincident crossing). See
	// floorbox_recompute_stateroot_maturitylatch_v5.go.
	mOps, postEverMature, mErr := c.maturityLatchOps(committedStateRoot, w, pre)
	if mErr != nil {
		return nil, mErr
	}
	digestOps = append(digestOps, mOps...)

	// Class P (epoch rotation, P1-e): rotate runs LAST. Reconstruct the POST-apply qualified set in
	// apply() order (B → T → S), freeze it into epochSet, reconstruct epochSetRoot + per-member
	// epochSet leaves, run the three activation tallies over per-member regVersion witnesses (own-cfg
	// thresholds + activation guards), and reconstruct the rotate scalars. R-P-sameblock-order. The
	// post-latch everMature (from class M) gates the freeze; P does NOT emit the tagEverMature leaf.
	if isBoundary {
		postQualified, qualWrites, regVerWrites, pqErr := c.reconstructPostQualifiedWithWrites(prevStateRoot, b, w)
		if pqErr != nil {
			return nil, pqErr
		}
		pOps, pErr := c.rotateOps(prevStateRoot, b, w, postQualified, qualWrites, regVerWrites, postEverMature)
		if pErr != nil {
			return nil, pErr
		}
		digestOps = append(digestOps, pOps...)
	}

	// (3) MATCH each derived write to its supplied pre-state witness and build the fold ops. A
	// derived write with no matching witness stalls the fold (an unverified change is never folded).
	witByKey := make(map[string]*StateRootChangedLeafWitness, len(w.ChangedLeaves))
	for i := range w.ChangedLeaves {
		witByKey[string(w.ChangedLeaves[i].Key)] = &w.ChangedLeaves[i]
	}
	ops := make([]statehash.FoldOp, 0, len(writeSet)+len(digestOps))
	ops = append(ops, digestOps...)
	for _, wr := range writeSet {
		wit, ok := witByKey[string(wr.key)]
		if !ok || wit.Proof.IsNil() {
			return nil, fmt.Errorf("%w: no witness for derived changed key %x", ErrRecomputeStateRootFold, wr.key)
		}
		// Consistency of the CLAIMED pre-state with the derived write: a delete (un-revocation) must
		// claim a PRESENT pre-state — you cannot delete an absent leaf. A false claim here would let a
		// witness turn a delete into a no-op; the check forecloses it (and the fold verifies the claim
		// against prevStateRoot regardless).
		if wr.newValue == nil && len(wit.OldValue) == 0 {
			return nil, fmt.Errorf("%w: un-revocation delete of key %x claims an absent pre-state", ErrRecomputeStateRootFold, wr.key)
		}
		ops = append(ops, statehash.FoldOp{
			Key:            wr.key,
			OldValue:       wit.OldValue, // the claimed pre-state; the fold verifies it against prevStateRoot
			NewValue:       wr.newValue,
			Proof:          wit.Proof,
			DeleteSiblings: wit.DeleteSiblings,
		})
	}
	return ops, nil
}

// stateRootScopeGate stalls (returns ErrRecomputeStateRootScopeStall / ...TTLWitness) if block b
// carries any committed-state transition outside P1-a's E + R scope. It reads the box's OWN cfg
// (C-6) for the epoch/TTL parameters and, for the TTL-expiry class, the O(1) dueBucket
// non-membership witness — NEVER a whole-state scan (the O(payload) re-anchor). Every clause maps
// to a later sub-increment; removing a clause is the visible signal that its class's recompute has
// landed.
func (c *Chain) stateRootScopeGate(prevStateRoot ports.Hash, b Block, w StateRootWitness) error {
	// Class B (bond regs, P1-d) / Class S (slashes, P1-b) are IN scope — handled by bondRegOps /
	// stateRootSlashDigestOps. Class A (attestations → validatorsSeen, P1-e) and Class P (epoch
	// rotation, P1-e) are ALSO now IN scope — handled by attOps / rotateOps. None is stalled here.
	//
	// R-A-legacy: a legacy-mode block (a v5 block never is, by construction) cannot reproduce the
	// rep(id) screen from committed state — attOps asserts objective and stalls otherwise.
	// R-P-recovery: the #535 recovery boundary re-bases from liveQualifiedSet(), which the box cannot
	// reconstruct from the qualified digest — rotateOps stalls at that one boundary. Both are stalls in
	// the class dispatch, not here (they need the witness/block, not just the scope predicate).
	//
	// R0.4b (issuer-key registrations): a block carrying IssuerKeys writes the
	// issuerKeyCommit keyspace, which this box does not reproduce. The fold would then
	// compute a post-root missing those leaves and land on ErrRecomputeStateRootMismatch
	// — a stall either way, but one that reads as "forged root". Stall EXPLICITLY so the
	// out-of-scope class is named rather than mis-attributed. The box never-Accepts.
	//
	// THIS PAYLOAD PREDICATE IS EXACT ONLY BECAUSE apply() MAKES IT SO (red-team
	// re-break F1, 2026-09-03). It was NOT exact: applyIssuerKeys used to prune the
	// keyspace by BLOCK HEIGHT on every apply, so a zero-registration block at an epoch
	// turn deleted committed leaves this gate waved through and this fold never
	// reproduced — a two-way box/full-node split (box AGREES with a forged root; box
	// reads an honest block as forged). The close is at the source: the prune now runs
	// only inside the registration-carrying branch (core/chain/issuerkey.go), so
	// "len(b.IssuerKeys) == 0 ⇒ no issuerKeyCommit write" is a property of apply(), not
	// an assumption of the gate. The reason it is closed THERE and not here: the box has
	// no sound way to reproduce a height-driven delete over this keyspace — statehash
	// offers point membership/non-membership only (no range proof), and issuerKeyCommit
	// carries no set-completeness digest leaf (unlike dueBucket's MTH, which is exactly
	// what makes class T's height-driven sweep witnessable). A witness-supplied member
	// list would be omission-forgeable, and the conservative alternative — stall at
	// every epoch boundary — would make every boundary block Indeterminate forever.
	// TestLeafDiff_IssuerKeyCommitIsPayloadOnly and the R0.4b C3 split gates pin it.
	if len(b.IssuerKeys) > 0 {
		return fmt.Errorf("%w: block carries %d demand-issuer key registration(s) (R0.4b, issuerKeyCommit)",
			ErrRecomputeStateRootScopeStall, len(b.IssuerKeys))
	}

	// Class T (TTL sweep, P1-c): an expiry fires at b.Height iff dueBucket[uint64BE(h)] is occupied
	// (chain.go:3274). The box distinguishes the two cases from the dueBucket witness against
	// prevStateRoot:
	//   - PROVEN ABSENT  ⇒ no expiry fires ⇒ the block is E/R(+B/S)-only; w.TTLSweep must be nil.
	//   - PROVEN PRESENT ⇒ a sweep fires ⇒ class T is in scope; w.TTLSweep must carry the expired set.
	// A missing/failed proof, or a witness/scope disagreement (present but no TTLSweep, or absent but
	// a TTLSweep supplied), stalls. dueBucket keys are v5-only, so this clause is inert when
	// BondTTLBlocks == 0.
	if c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		binary.BigEndian.PutUint64(hk[:], b.Height)
		key := statehash.Key(tagDueBucket, hk[:])
		absent := statehash.Resolve(prevStateRoot, key, nil, w.DueBucketProof)
		if absent.IsProvenAbsent() {
			// No sweep fires. A TTLSweep witness for a non-firing height is a scope error.
			if w.TTLSweep != nil {
				return fmt.Errorf("%w: dueBucket[%d] proven absent but a TTL sweep witness was supplied",
					ErrRecomputeStateRootTTLWitness, b.Height)
			}
		} else {
			// Not proven absent ⇒ a sweep may fire ⇒ class T is in scope. Require the expired-set
			// witness keyed at b.Height, and PROVE the bucket PRESENT with the value the witness's
			// member list reconstructs (dueBucketMTH(Members)). A forged/short member list yields a
			// value that does not verify against prevStateRoot ⇒ NoWitness ⇒ stall — so the scope gate
			// also enforces the CRUX completeness anchor, not just the recompute.
			if w.TTLSweep == nil || w.TTLSweep.Height != b.Height || len(w.TTLSweep.Members) == 0 {
				return fmt.Errorf("%w: dueBucket[%d] not proven absent but no matching TTL sweep witness",
					ErrRecomputeStateRootTTLWitness, b.Height)
			}
			bucketMTH := dueBucketMTHFromSlice(w.TTLSweep.Members)
			if !statehash.Resolve(prevStateRoot, key, bucketMTH, w.DueBucketProof).IsProvenPresent() {
				return fmt.Errorf("%w: dueBucket[%d] present-proof did not verify the reconstructed expired-set MTH",
					ErrRecomputeStateRootTTLWitness, b.Height)
			}
		}
	}
	return nil
}

// stateRootWrite is one payload-derived committed-leaf change: the leaf key and its post-state
// value (nil = delete). The pre-state value is NOT derived here — it comes from the matched
// witness's claim, which the fold verifies against prevStateRoot.
type stateRootWrite struct {
	key      []byte
	newValue []byte
}

// applyEntriesRevocationsWriteSet derives the class-E and class-R committed-leaf write-set for a
// block, applying exactly the writes of apply() (chain.go:3187-3203):
//   - each entry adds a byRoot leaf (value Present); an entry carrying a token adds a spent leaf.
//   - each revocation adds a revoked leaf (value Present); each un-revocation deletes one.
//
// The KEY set is a pure function of the payload (the completeness bound). The oldValue is left nil
// here — the box does not know the pre-state a priori; the supplied per-key proof (membership vs
// non-membership) carries the pre-state claim, and the fold verifies it against prevStateRoot. For
// an ADD (byRoot/spent/revoked not present) the proof is non-membership (oldValue nil); for a
// re-set of an already-present set-marker (an idempotent overwrite) or an un-revocation delete of a
// present leaf, the proof is membership (oldValue Present). The box reads the oldValue from the
// witness claim, so it derives newValue and key here and takes oldValue from the matched witness.
//
// It reproduces the leaf EFFECT of apply()'s two classes, not apply() itself: the byRoot/spent/
// revoked leaves are the committed image of those maps (statehash.go), so folding these writes is
// byte-identical to apply()+stateRootLeavesV5 for these classes. The revLog append (LogRoot, not
// StateRoot) is out of scope — this is the STATE root.
func applyEntriesRevocationsWriteSet(b Block) []stateRootWrite {
	// Dedup by key: a block may repeat a root/serial; the committed leaf is set-valued, so the last
	// write wins and the leaf is present-once. A revocation followed by an un-revocation of the same
	// root in ONE block nets to the un-revocation (delete) — apply() processes revocations then
	// un-revocations in order (chain.go:3193-3203), so the un-revocation's delete is the final state.
	type wr struct {
		newValue []byte // nil = delete
		isDelete bool
	}
	byKey := map[string]wr{}
	order := []string{}
	remember := func(key []byte, newValue []byte, isDelete bool) {
		k := string(key)
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = wr{newValue: newValue, isDelete: isDelete}
	}
	for _, e := range b.Entries {
		remember(statehash.Key(tagByRoot, e.Root[:]), statehash.Present, false)
		if e.Token != nil {
			remember(statehash.Key(tagSpent, []byte(e.Token.Serial)), statehash.Present, false)
		}
	}
	for _, r := range b.Revocations {
		remember(statehash.Key(tagRevoked, r[:]), statehash.Present, false)
	}
	for _, r := range b.Unrevocations {
		remember(statehash.Key(tagRevoked, r[:]), nil, true)
	}
	out := make([]stateRootWrite, 0, len(order))
	for _, k := range order {
		v := byKey[k]
		nv := v.newValue
		if v.isDelete {
			nv = nil
		}
		out = append(out, stateRootWrite{key: []byte(k), newValue: nv})
	}
	return out
}
