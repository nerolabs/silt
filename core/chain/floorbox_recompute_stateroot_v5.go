package chain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — Path-1 state-root recompute, sub-increment P1-a.
//
// This file reproduces validateEra3Roots' StateRoot equality check (era3validity.go:114) — the
// committed StateRoot MUST equal the SMT recomputed over the POST-APPLY committed leaf set —
// TRUSTLESSLY, from two committed roots + a witnessed pre-state ALONE. It is the FIRST
// sub-increment of the Path-1 recompute (PACE: docs/thinking/2026-08-31-floorbox-recompute-
// path1-stateroot-options.md); the full validateEra3Roots recompute spans eight apply()
// transition classes and is decomposed there. P1-a lands the ROOT-EQUALITY SPINE every later
// class reuses, on the two classes that are pure payload-driven set writes with NO membership
// screen: entries (byRoot / spent) and revocations / un-revocations (revoked).
//
// It is ADDITIVE: it calls no full-node accept path, mutates nothing, and changes NO
// consensus/validity rule. A full node still recomputes the root by cloneForDryRun + apply +
// StateRootForVersion (era3validity.go:145, chain.go untouched). This is a SEPARATE root-only
// path a semi-stateless box calls INSTEAD of cloning the whole state and replaying apply() — the
// same posture the increment-1..4 predicate recomputes already hold.
//
// WHY A ROOT-ONLY BOX CANNOT REPLAY apply(). apply() scans whole O(registry) maps (the
// bondRegHeight TTL sweep chain.go:3272; the three rotateEpoch frozen-set tallies
// chain.go:3442/3465/3489). A root-only box holds no registry. It instead reproduces the
// WITNESSABLE recompute: reconstruct the pre-state leaf set (proven complete against the previous
// block's committed StateRoot), apply the block's PAYLOAD transitions to it, and require the SMT
// over the post-state leaf set equals the block's committed StateRoot.
//
// THE COMPLETENESS ANCHOR (the P1-a novelty every later class reuses). A root-only box cannot,
// from single-key proofs alone, know it has ALL of the pre-state leaves — a withholding prover
// hands a short set whose every inclusion proof still verifies. The previous block's committed
// StateRoot closes that gap: it is itself the SMT over the COMPLETE pre-state leaf set. So the
// box requires Root(witnessedPreLeaves) == prevStateRoot. One omitted or injected pre-leaf yields
// a different reconstructed root ⇒ mismatch ⇒ stall. This is the same "recompute the commitment
// from the claimed preimage" argument the CRUX cert (2026-08-30) certifies for dueBucketMTH,
// applied to the WHOLE-STATE root: the pre-state root is the whole-state completeness digest, so
// P1-a needs NO per-keyspace F1 digest read (those anchor SUBSETS; the pre-state root anchors the
// whole set).
//
// THE THREE-PART CHECK (RecomputeStateRootEntriesRevocations):
//  1. PRE-STATE COMPLETENESS: reconstruct Root(w.PreLeaves); require it equals prevStateRoot. A
//     short/padded pre-state yields a different root ⇒ stall. (prevStateRoot is block h-1's
//     committed StateRoot — attester-signed, so trusted the same way b.StateRoot is.)
//  2. SCOPE GATE: stall (never-Accept) if b carries any class OUT of P1-a scope — a BondReg, a
//     Slash, an Att that would write validatorsSeen, a TTL expiry firing at b.Height, or an epoch
//     boundary. The box holds the full pre-state (step 1), so it detects all of these soundly
//     from the witnessed leaves + own config; it never guesses.
//  3. POST-STATE ROOT: apply the E + R payload transitions to the pre-state leaf map, then require
//     Root(postLeaves) == b.StateRoot. This IS validateEra3Roots' equality check, reproduced
//     root-only. A forged committed leaf, an omitted/injected payload write, or a tampered
//     b.StateRoot all diverge the recomputed root ⇒ stall.
//
// STOP BOUNDARY (this sub-increment). It reproduces the root-equality MECHANISM on classes E + R
// only; classes S/A (screens), T (TTL), B (bond regs), P (rotation, consumes qualifiedRoot), and
// M (maturity latch) are the later sub-increments in the PACE decomposition. It does NOT consume
// qualifiedRoot (that is P1-e, the boundary freeze — reading it here would be a decoration read
// with no transition that uses it). It does NOT flip #657 WitnessValidateV5 to Accept — the box
// STILL never-Accepts. It changes NO apply() rule.

var (
	// ErrRecomputeStateRootPreStateIncomplete marks a stall where the witnessed pre-state leaf set
	// does not reconstruct the previous block's committed StateRoot: a leaf was omitted, injected,
	// or value-tampered, so the SMT over the witnessed set differs from prevStateRoot. Without a
	// complete, authentic pre-state the box cannot apply the payload to it — it stalls, never
	// recomputes over a partial pre-state.
	ErrRecomputeStateRootPreStateIncomplete = errors.New("chain: floor-box state-root recompute — witnessed pre-state leaf set does not reconstruct the previous block's committed StateRoot (a leaf was omitted, injected, or tampered)")

	// ErrRecomputeStateRootOutOfScope marks a stall where the block carries a transition class this
	// P1-a sub-increment does not yet reproduce (a BondReg, a Slash, a validatorsSeen-writing Att, a
	// TTL expiry at this height, or an epoch boundary). The box never-Accepts an out-of-scope block;
	// it stalls loud so a later sub-increment's absence can never be a silent wrong-Accept.
	ErrRecomputeStateRootOutOfScope = errors.New("chain: floor-box state-root recompute — block carries a transition class outside P1-a scope (bond reg / slash / seen-writing att / TTL expiry / epoch boundary); the box stalls, never Accepts")

	// ErrRecomputeStateRootMismatch marks the terminal stall: the SMT over the recomputed post-state
	// leaf set does not equal the block's committed StateRoot. This is validateEra3Roots'
	// ErrEra3StateRootMismatch reproduced root-only — a forged committed leaf, an omitted/injected
	// payload write, or a tampered committed StateRoot all land here.
	ErrRecomputeStateRootMismatch = errors.New("chain: floor-box state-root recompute — recomputed post-state root does not equal the block's committed StateRoot")

	// ErrRecomputeStateRootDuplicatePreLeaf marks a malformed witness: two pre-state leaves share a
	// key. A duplicate key is not a valid committed state (Root reports it), so the box stalls rather
	// than folding an ambiguous pre-state.
	ErrRecomputeStateRootDuplicatePreLeaf = errors.New("chain: floor-box state-root recompute — witnessed pre-state carries a duplicate leaf key")
)

// StateRootPreWitness is the witnessed pre-state a floor box supplies to reproduce
// validateEra3Roots: the COMPLETE set of committed v5 leaves of the state BEFORE block b is
// applied (the state whose root is the previous block's committed StateRoot). It is UNTRUSTED:
// completeness and authenticity are established in step 1 by requiring Root(PreLeaves) equals the
// previous block's committed StateRoot. No per-key inclusion proofs are needed here — the WHOLE
// pre-state root is the single completeness+authenticity anchor (the P1-a novelty), which is
// strictly stronger than a bag of single-key proofs for the "no leaf withheld" property.
type StateRootPreWitness struct {
	// PreLeaves is the claimed COMPLETE v5 committed leaf set of the pre-apply state. It is the
	// exact set Chain.stateRootLeavesV5 would emit for the pre-state. Completeness is NOT trusted:
	// Root(PreLeaves) must equal prevStateRoot, so an omitted, injected, or value-tampered leaf
	// changes the reconstructed root and stalls.
	PreLeaves []statehash.Leaf
}

// RecomputeStateRootEntriesRevocations reproduces validateEra3Roots' StateRoot equality check
// (era3validity.go:128) TRUSTLESSLY for a v5 block whose ONLY committed-state effect is its
// entries and revocations/un-revocations (classes E + R). It returns nil iff the block's committed
// StateRoot equals the SMT over the post-apply committed leaf set a full node would compute — the
// same verdict validateEra3Roots reaches — and a stall reason otherwise. It NEVER returns "valid"
// for an out-of-scope block: it stalls loud, never-Accepts.
//
// prevStateRoot is the previous block's committed StateRoot (the pre-state completeness anchor).
// committedStateRoot is b.StateRoot (the post-state root to check). The box holds both roots
// (attester-signed) and the witnessed pre-state; it holds NO registry and replays NO apply().
//
// It reads EpochBlocks / epochsEnabled / BondTTLBlocks from the box's OWN cfg (C-6) for the scope
// gate — never from the witness — so a witness cannot hide an out-of-scope class by shifting a
// config value. This does NOT flip WitnessValidateV5 to Accept (the STOP boundary).
func (c *Chain) RecomputeStateRootEntriesRevocations(
	prevStateRoot ports.Hash,
	committedStateRoot ports.Hash,
	b Block,
	w StateRootPreWitness,
) (reason error) {
	// (1) PRE-STATE COMPLETENESS. Reconstruct the SMT over the witnessed pre-state leaves and require
	// it equals the previous block's committed StateRoot. A short/padded/tampered pre-state yields a
	// different root ⇒ stall. This anchors the WHOLE pre-state as complete and authentic in one check.
	preLeaves, dup := indexLeaves(w.PreLeaves)
	if dup {
		return ErrRecomputeStateRootDuplicatePreLeaf
	}
	preRoot, err := statehash.Root(w.PreLeaves)
	if err != nil {
		return fmt.Errorf("%w: pre-state root recompute failed: %v", ErrRecomputeStateRootDuplicatePreLeaf, err)
	}
	if preRoot != prevStateRoot {
		return fmt.Errorf("%w: reconstructed %x != prevStateRoot %x",
			ErrRecomputeStateRootPreStateIncomplete, preRoot, prevStateRoot)
	}

	// (2) SCOPE GATE. Stall (never-Accept) on any transition class P1-a does not reproduce. The box
	// holds the complete pre-state (step 1) + its OWN config, so every out-of-scope condition is
	// detected soundly from committed data — never guessed. A later sub-increment REMOVES the
	// corresponding clause as it lands that class's recompute.
	if reason := c.stateRootScopeGate(b, preLeaves); reason != nil {
		return reason
	}

	// (3) POST-STATE ROOT. Apply the E + R payload transitions to a copy of the pre-state leaf map,
	// then require the SMT over the post-state leaf set equals the block's committed StateRoot. This
	// IS validateEra3Roots' equality check (era3validity.go:128), reproduced root-only.
	postLeaves := applyEntriesRevocationsToLeaves(preLeaves, b)
	postRoot, err := statehash.Root(postLeaves)
	if err != nil {
		// A duplicate post-key would be an apply-derivation bug in this file, not valid state.
		return fmt.Errorf("%w: post-state root recompute failed: %v", ErrRecomputeStateRootMismatch, err)
	}
	if postRoot != committedStateRoot {
		return fmt.Errorf("%w: recomputed %x != committed %x",
			ErrRecomputeStateRootMismatch, postRoot, committedStateRoot)
	}
	return nil
}

// stateRootScopeGate stalls (returns ErrRecomputeStateRootOutOfScope) if block b carries any
// committed-state transition outside P1-a's E + R scope. It reads the witnessed pre-state leaf
// index (for the TTL-expiry check) and the box's OWN cfg (C-6) — never the witness — for the
// epoch/TTL parameters. Every clause maps to a later sub-increment (see the PACE decomposition);
// removing a clause is the visible signal that its class's recompute has landed.
func (c *Chain) stateRootScopeGate(b Block, preLeaves map[string][]byte) error {
	// Class B (bond regs), Class S (slashes): payload-visible, deferred to P1-d / P1-b.
	if len(b.BondRegs) > 0 {
		return fmt.Errorf("%w: %d bond reg(s)", ErrRecomputeStateRootOutOfScope, len(b.BondRegs))
	}
	if len(b.Slashes) > 0 {
		return fmt.Errorf("%w: %d slash(es)", ErrRecomputeStateRootOutOfScope, len(b.Slashes))
	}
	// Class A (attestation tracking, P1-b): an att for a non-proposer writes validatorsSeen when the
	// attester is qualified. P1-a does not reproduce attesterQualified's membership screen, so stall
	// if ANY att could write validatorsSeen. Conservative: a non-proposer att id at all trips the
	// gate (a proposer-only att set is a no-op for validatorsSeen). This never wrong-Accepts — it can
	// only stall a block P1-a could in principle have handled, which a later sub-increment covers.
	proposer := b.ProposerID()
	for _, a := range b.Atts {
		if a.AttesterID() != proposer {
			return fmt.Errorf("%w: att by a non-proposer may write validatorsSeen", ErrRecomputeStateRootOutOfScope)
		}
	}
	// Class T (TTL sweep, P1-c): an expiry fires at b.Height when b.Height - bondRegHeight[id] > ttl
	// for some bonded id. The box holds every bondRegHeight leaf (pre-state completeness), so it
	// detects a firing expiry exactly, from committed data + own BondTTLBlocks (C-6).
	if ttl := c.cfg.BondTTLBlocks; ttl > 0 {
		for key, val := range preLeaves {
			if !hasTagPrefix(key, tagBondRegHeight) {
				continue
			}
			regH := decodeUint64Leaf(val)
			if b.Height-regH > ttl {
				return fmt.Errorf("%w: a TTL expiry fires at height %d", ErrRecomputeStateRootOutOfScope, b.Height)
			}
		}
	}
	// Class P (epoch rotation, P1-e): rotateEpoch fires at a boundary. It ALWAYS writes epochStart,
	// and post-latch it writes epochSet + the three lock-in scalars (consuming qualifiedRoot). Stall
	// on any boundary. epochsEnabled/EpochBlocks are read from OWN cfg (C-6).
	if c.epochsEnabled() && c.cfg.EpochBlocks > 0 && b.Height%c.cfg.EpochBlocks == 0 {
		return fmt.Errorf("%w: height %d is an epoch boundary", ErrRecomputeStateRootOutOfScope, b.Height)
	}
	return nil
}

// applyEntriesRevocationsToLeaves derives the POST-APPLY committed leaf set for a block whose only
// committed-state effect is its entries and revocations/un-revocations, by applying exactly the
// class-E and class-R writes of apply() (chain.go:3187-3203) to a copy of the pre-state leaf map:
//   - each entry adds a byRoot leaf (value Present); an entry carrying a token adds a spent leaf.
//   - each revocation adds a revoked leaf (value Present); each un-revocation removes one.
//
// It returns the post-state leaf SLICE (order-free — Root folds the set). It reproduces the leaf
// EFFECT of apply()'s two classes, not apply() itself: the byRoot/spent/revoked leaves are the
// committed image of those maps (statehash.go:156-164), so writing the leaves directly is
// byte-identical to apply()+stateRootLeavesV5 for these classes. The revLog append (LogRoot, not
// StateRoot) is out of scope — this is the STATE root, era3validity.go:128 checks only StateRoot.
func applyEntriesRevocationsToLeaves(preLeaves map[string][]byte, b Block) []statehash.Leaf {
	post := make(map[string][]byte, len(preLeaves)+len(b.Entries)+len(b.Revocations))
	for k, v := range preLeaves {
		post[k] = v
	}
	// Class E: entries. byRoot[e.Root] = e (committed as a Present set-marker leaf, statehash.go:156);
	// a token entry marks spent[serial] (statehash.go:159).
	for _, e := range b.Entries {
		post[string(statehash.Key(tagByRoot, e.Root[:]))] = statehash.Present
		if e.Token != nil {
			post[string(statehash.Key(tagSpent, e.Token.Serial))] = statehash.Present
		}
	}
	// Class R: revocations add a revoked leaf; un-revocations delete one (apply() deletes so the map
	// stays a clean set — statehash.go:161 emits a leaf only for present keys, so a delete removes the
	// leaf and shrinks the committed set, exactly as apply()'s delete does).
	for _, r := range b.Revocations {
		post[string(statehash.Key(tagRevoked, r[:]))] = statehash.Present
	}
	for _, r := range b.Unrevocations {
		delete(post, string(statehash.Key(tagRevoked, r[:])))
	}
	out := make([]statehash.Leaf, 0, len(post))
	for k, v := range post {
		out = append(out, statehash.Leaf{Key: []byte(k), Value: v})
	}
	return out
}

// indexLeaves builds a key→value index over a witnessed leaf slice, reporting a duplicate key. A
// duplicate is a malformed witness (the same contract statehash.Root enforces): the box stalls
// rather than folding an ambiguous pre-state.
func indexLeaves(leaves []statehash.Leaf) (index map[string][]byte, duplicate bool) {
	index = make(map[string][]byte, len(leaves))
	for _, lf := range leaves {
		k := string(lf.Key)
		if _, ok := index[k]; ok {
			return nil, true
		}
		index[k] = lf.Value
	}
	return index, false
}

// hasTagPrefix reports whether leaf key k begins with the given field tag. The tag is NUL-
// terminated (statehash.Key), so a prefix match uniquely identifies the keyspace — no other tag's
// keys can share it (research cert Q3 injectivity).
func hasTagPrefix(k string, tag string) bool {
	return bytes.HasPrefix([]byte(k), []byte(tag))
}

// decodeUint64Leaf decodes an 8-byte big-endian uint64 leaf value (the EncodeUint64 image used for
// bondRegHeight). A malformed (wrong-length) value decodes to 0, which the scope gate treats
// conservatively (a 0 regHeight at any height>ttl trips the expiry stall) — it never wrong-Accepts.
func decodeUint64Leaf(v []byte) uint64 {
	if len(v) != 8 {
		return 0
	}
	var h uint64
	for _, bb := range v {
		h = h<<8 | uint64(bb)
	}
	return h
}
