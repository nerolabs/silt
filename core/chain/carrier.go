package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) LastCommit ATTESTATION CARRIER — R-BOX-ATTESTS, owner call O1, ratified 2026-09-03.
//
// THE DEFECT THIS CLOSES (converged verdict §2.3, RED-captured as gate G1). apply() wrote
// validatorsSeen from b.Atts (the attestation loop below's sub-v5 twin, chain.go), but Hash()
// EXCLUDES Atts and the era-3/era-4 root predicate re-runs the real apply() over the ATTACHED
// certificate (era3validity.go validateEra3Roots → postApplyRoots). A proposer populates its
// committed roots BEFORE it gathers precommits (core/node/chainrole.go proposeBlock), so a
// certificate that would seat a NEW attester makes the recomputed root differ from the root the
// proposer signed, and EVERY replica rejects that block. Two consequences, both HIGH:
//
//	(a) THE MEASUREMENT FREEZES. Only blocks that seat NOBODY commit, so validatorsSeen is
//	    constant from the first root-checked block onward. C2Metric ranges only validatorsSeen,
//	    so MatureCoefficient — the quantity the maturity shed gates on — is permanently CEILINGED
//	    by the set that attested before activation. No operator who joins after activation is
//	    ever counted, and a de-maturation window can be closed only by re-bonding identities that
//	    had already attested (never by new arrivals). Corner 3's "shed on MEASURED
//	    decentralization" clause is broken.
//	(b) THE CHAIN STALLS INTERMITTENTLY. Every round whose first-to-quorum prefix carries a
//	    qualified never-seen attester kills that height for that round ("commit rejected by own
//	    replica", chainrole.go finishPC). Connected, all-honest, unbounded in expectation. It
//	    clears only by the certificate DROPPING the new attester — i.e. it clears into (a).
//
// THE FIX — a HASH-COVERED carrier. Block h+1 republishes block h's precommits in LastCommit,
// which IS folded into Hash(). The proposer of h+1 holds those bytes before it signs, so the root
// it signs is the root the block commits. The seat lands ONE BLOCK LATE — monotone, benign, and
// disclosed in O1. CometBFT's LastCommit is the settled analogue; silt deliberately omits the
// quorum re-check, because the carrier is a SEATING WITNESS, not a commit proof (that keeps I1 /
// #402's one-function-two-callers quorum stack untouched — the carrier counts NOTHING).
//
// ERA GATING. The carrier transition fires ONLY for v5 (BlockVersionWitnessable). The frozen
// era-3 (v4) and era-2 (v2) transition rule — the b.Atts seating loop in apply() — is left
// BYTE-FOR-BYTE and now runs only for sub-v5 blocks. The era-3 format freeze (#632) is not
// touched: statehash.go's leaf tags and encodings are unchanged. The carrier changes WHEN and
// FROM WHAT the transition writes validatorsSeen, not the leaf.
//
// NOT IN THIS ROUND: the fork-choice weight decision (owner call O3) — blockWeight/heavier are
// untouched. The carrier is designed so a later round COULD derive block h's weight from block
// h+1's hash-covered LastCommit, but no weight code is added here.

var (
	// ErrCarrierNotWitnessable is a block BELOW the current open era (v5) carrying the LastCommit
	// field. O1: "a pre-v5 block carrying the field is invalid." Without this rule a v4 block
	// could carry carrier bytes that the frozen era-3 transition ignores but Hash() covers —
	// silently version-dependent semantics for the same bytes.
	ErrCarrierNotWitnessable = errors.New("chain: block below era-4 (v5) carries a LastCommit attestation carrier")
	// ErrCarrierAtHeightOne is a height-1 block with a non-empty carrier. O1: "height 1's carrier
	// is empty BY RULE." Height 1's parent is the genesis, which is DECLARED, not agreed — it has
	// no precommits, and a declared genesis certificate must never pre-seat validatorsSeen.
	ErrCarrierAtHeightOne = errors.New("chain: height-1 block carries a non-empty LastCommit (the carrier is empty by rule at height 1)")
	// ErrCarrierBadSignature is a carrier entry that is not a genuine PhasePrecommit signature
	// over b.Prev. Verified at the entry's OWN declared round: O1 binds the rule to
	// (PhasePrecommit, b.Prev) and deliberately NOT to CommitRound, which Hash() does not cover.
	ErrCarrierBadSignature = errors.New("chain: LastCommit entry is not a genuine PhasePrecommit signature over the parent block hash")
	// ErrCarrierDuplicateID is two carrier entries for the same attester id. O1: "distinct ids".
	// The seating write is idempotent, so a duplicate cannot change the transition — but it can
	// pad the block, and a rule that admits it would make the carrier's size unbounded by the
	// R-membership set bound the flip already owes.
	ErrCarrierDuplicateID = errors.New("chain: LastCommit carries two entries for the same attester id")
	// ErrGenesisLastCommit is a genesis block carrying a LastCommit carrier. Height 0 has no
	// parent to attest, and the carrier is the hash-covered v5 seating input, so a declared
	// genesis carrying one is an authored pre-seating of validatorsSeen. O1 refuses it by rule.
	// The UNSIGNED slot (Atts) is deliberately NOT covered by this error: its disposal on
	// genesis is research-gated (R-CARRIER-GENESIS-DISPOSAL) and AppendGenesis keeps main's
	// pre-carrier behaviour for it.
	ErrGenesisLastCommit = errors.New("chain: genesis block carries a LastCommit carrier — refused by rule; height 0 has no parent to attest and a declared genesis must not pre-seat validatorsSeen")
)

// validateCarrier is the O1 VALIDITY rule for the LastCommit carrier. It is a pure function of
// the block alone (header + signatures) — no committed state, no clone, no apply — so it runs
// BEFORE the block is applied on every disk-write path, exactly like validateEra3Version.
//
// The rule, verbatim from O1:
//
//	every entry verifies over b.Prev's hash at PhasePrecommit at any single round (CommitRound is
//	uncovered, so the rule must not bind to it); distinct ids; a pre-v5 block carrying the field
//	is invalid; height 1's carrier is empty by rule.
//
// "at any single round" is read PER ENTRY: each entry verifies at its OWN declared Round, and the
// rule constrains neither the round nor agreement between entries. Reason: the alternative (all
// entries at one common round) makes O1's own honest-maximal producer rule ("carry everything you
// hold") produce an INVALID block whenever a proposer holds parent precommits from two rounds.
// The two readings agree on every certificate collectQuorumSigs accepts — that function is fatal
// on a mixed-round set — and differ only on the supra-certificate set an honest-maximal proposer
// may hold. Recorded in docs/thinking/2026-09-03-lastcommit-carrier-round-A-design.md §3.2.
//
// It does NOT check quorum, weight, or qualification. The carrier is a SEATING WITNESS: an
// unqualified signer's entry is valid and simply writes nothing (applyCarrier screens it). Adding
// a quorum check here would fork the #402 one-function-two-callers quorum stack.
func (c *Chain) validateCarrier(b *Block) error {
	if len(b.LastCommit) == 0 {
		return nil // the empty carrier is always valid — including at height 1 and on every prior era
	}
	if b.Version < BlockVersionWitnessable {
		return fmt.Errorf("%w: height %d version %d carries %d entries",
			ErrCarrierNotWitnessable, b.Height, b.Version, len(b.LastCommit))
	}
	if b.Height <= 1 {
		return fmt.Errorf("%w: height %d carries %d entries", ErrCarrierAtHeightOne, b.Height, len(b.LastCommit))
	}
	seen := make(map[ports.NodeID]bool, len(b.LastCommit))
	for i := range b.LastCommit {
		a := b.LastCommit[i]
		if a.Phase != PhasePrecommit {
			return fmt.Errorf("%w: entry %d has phase %d, want PhasePrecommit (%d)",
				ErrCarrierBadSignature, i, a.Phase, PhasePrecommit)
		}
		// verifyAtt is the SAME era-aware arithmetic the live commit path uses
		// (collectQuorumSigs, validateStructural) — the #558 fix's shared function, never a
		// second bare-hash copy. At PhasePrecommit it checks
		// consensusSigBytes(PhasePrecommit, a.Round, b.Prev).
		if !verifyAtt(a, b.Prev) {
			return fmt.Errorf("%w: entry %d (round %d) does not verify over parent %s",
				ErrCarrierBadSignature, i, a.Round, b.Prev)
		}
		id := a.AttesterID()
		if seen[id] {
			return fmt.Errorf("%w: %s", ErrCarrierDuplicateID, id)
		}
		seen[id] = true
	}
	return nil
}

// applyCarrier is the O1 TRANSITION rule: for each carried signer with id != parent.ProposerID()
// and attesterQualified(id) evaluated against the CHILD'S PRE-STATE, set validatorsSeen[id].
//
// ORDER (load-bearing, pinned by TestCarrierFoldPrecedesBondRegsInApply): this runs BEFORE this
// block's bond registrations, TTL expiries and slashes. The screen therefore reads the PARENT's
// committed post-state — which is exactly the box's prevStateRoot — so the chain and the trustless
// floor box screen against the SAME state by construction (the S3 divergence the box-entry round
// A screen was already anchored for). Putting it after the bond loop would screen a mid-apply
// state that no committed root names, the sibling of the rotate-LAST hazard (#620).
//
// The child's OWN Atts write NOTHING for a v5 block. That is the whole point: Atts are not
// hash-covered, so they must not be a transition input.
//
// The parent's proposer is excluded, preserving the anti-self-declaration property the seating
// metric exists for (C2Metric's doc: validatorsSeen counts "attested a committed block", so a
// proposer must not seat itself off its own block). Under the carrier the block being attested is
// the PARENT, so the excluded id is the parent's proposer.
func (c *Chain) applyCarrier(b Block, parentProposer ports.NodeID) {
	if b.Version < BlockVersionWitnessable {
		return // prior-era blocks transition under the FROZEN b.Atts rule, byte-for-byte
	}
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if id == parentProposer {
			continue // the parent's proposer does not seat itself off its own block
		}
		if c.attesterQualified(id) {
			c.validatorsSeen[id] = true
		}
	}
}

// headProposerID returns the ProposerID of this chain's head block — the parent of the block
// about to be applied. Reported absent for an empty chain (before the genesis is applied).
func (c *Chain) headProposerID() (ports.NodeID, bool) {
	if len(c.blocks) == 0 {
		return ports.NodeID{}, false
	}
	head := c.blocks[len(c.blocks)-1]
	return head.ProposerID(), true
}

// HeadCarrier returns the LastCommit carrier a proposer building on this chain's head must
// attach: every genuine PhasePrecommit attestation over the head block that this replica holds.
//
// HONEST-MAXIMAL, AND UNENFORCEABLE AS A RULE. O1's producer rule is "carry everything you hold".
// No replica can know what a proposer held, so this can never be a VALIDITY rule — the discretion
// stays with the proposer. It is DOWNWARD-ONLY: signatures are genuine and unforgeable, so a
// proposer can DELAY a seating by omitting a signer, but can never FORGE one. That is the same
// power a proposer already had by trimming its own certificate (refutation K), so the carrier adds
// no new degree of freedom. An honest proposer that under-carries harms only its own fork.
//
// The source is the head block's stored certificate. Entries are filtered (genuine precommit over
// the head hash) and deduped so the result satisfies validateCarrier by construction; a replica
// that somehow stored a malformed certificate cannot mint an invalid block off it.
//
// Returns nil at height 0 — height 1's carrier is empty BY RULE (the genesis is declared, not
// agreed), so a proposer at height 1 attaches nothing.
func (c *Chain) HeadCarrier() []Attestation {
	head, ok := c.headBlock()
	if !ok || head.Height == 0 {
		return nil
	}
	h := head.Hash()
	seen := make(map[ports.NodeID]bool, len(head.Atts))
	out := make([]Attestation, 0, len(head.Atts))
	for i := range head.Atts {
		a := head.Atts[i]
		if a.Phase != PhasePrecommit || !verifyAtt(a, h) {
			continue
		}
		id := a.AttesterID()
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// headBlock returns this chain's head block by value.
func (c *Chain) headBlock() (Block, bool) {
	if len(c.blocks) == 0 {
		return Block{}, false
	}
	return c.blocks[len(c.blocks)-1], true
}

// carrierParentProposerFromWitness resolves the parent block's proposer id for the trustless
// floor box, which — unlike the chain — holds no parent block: WitnessValidateV5 receives only
// (b, parentStateRoot). The parent's proposer identity is NOT a committed leaf, so it cannot be
// Resolved against prevStateRoot the way every other class-A screen input is.
//
// THE ANCHOR AND ITS EXACT BOUND (R-CARRIER-PARENTPROPOSER, design record §3.3). The witness
// carries the parent's own Proposer public key and ProposerSig, and the box requires
// ed25519.Verify(pub, b.Prev[:], sig) — the SAME bare-hash proposer-signature arithmetic the
// chain uses (ValidateProposal / validateStructural / appendStructural). b.Prev is hash-covered,
// so the challenge is fixed by the block.
//
// This proves "the named key signed b.Prev". It does NOT prove "this key is THE parent's
// proposer" — any key can sign any hash. The residual is therefore bounded to exactly this: an
// attacker holding key K can make the box skip K's OWN seat. Moving any OTHER signer's seat
// requires that signer's signature over b.Prev, which is the ed25519 unforgeability assumption.
// "Drop your own seat" is precisely the downward-only discretion O1 already discloses for the
// proposer, so this anchor reduces an unbounded one-seat forgery to a power the ratified design
// already grants. Held open as a named input to the R1.8 accept-flip; the box never-Accepts today.
//
// A missing or non-verifying parent-proposer witness STALLS (never falls through to "no
// exclusion", which would seat the parent's proposer — the C-7 §104 banned move).
func carrierParentProposerFromWitness(prev ports.Hash, pub, sig []byte) (ports.NodeID, error) {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return ports.NodeID{}, fmt.Errorf("%w: parent-proposer witness is missing or malformed (pub %d bytes, sig %d bytes)",
			ErrRecomputeStateRootDigest, len(pub), len(sig))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), prev[:], sig) {
		return ports.NodeID{}, fmt.Errorf("%w: parent-proposer signature does not verify over b.Prev %s",
			ErrRecomputeStateRootDigest, prev)
	}
	return blockProposerID(pub), nil
}

// blockProposerID is the id derivation Block.ProposerID uses, over a raw public key.
func blockProposerID(pub []byte) ports.NodeID { return (&Block{Proposer: pub}).ProposerID() }

// CarrierParentProposerWitness returns the (public key, signature) pair a witness server attaches
// to StateRootWitness.ParentProposer/ParentProposerSig so a root-only floor box can anchor the
// carrier fold's parent-proposer exclusion: this chain's head block IS the parent of the block the
// box is validating, and its ProposerSig is over its own hash — which is that block's b.Prev.
func (c *Chain) CarrierParentProposerWitness() (pub, sig []byte) {
	head, ok := c.headBlock()
	if !ok {
		return nil, nil
	}
	return head.Proposer, head.ProposerSig
}
