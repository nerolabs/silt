package chain

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// (d-3) — the two-level v5 block hash: digest-consistency validity.
//
// WHAT THIS ENFORCES AND WHY IT IS THE LOAD-BEARING HALF. From era-4, the v5 preimage commits the
// heavy payloads by DIGEST rather than by value: BondReg.Answer by BondReg.AnswerDigest, and
// Slashes by Block.SlashesDigest (Block.bodyHash). That substitution is what lets a PRUNED v5
// block recompute its own hash from what it retains — which retires `Pruned`, the DECLARED
// identity nothing recomputes, and makes the retained body self-covering. See Block.Hash's comment
// for the defect being closed, and docs/decisions.md D-FREEZE-CALLS-CDEF-2026-09-10.
//
// The substitution is only sound if each digest actually equals its content. Coverage of
// Answer/Slashes on v5 is therefore TRANSITIVE, through THIS predicate — mutate either and the
// block becomes INVALID rather than differently hashed. If this rule is weak, the payloads are
// unbound on v5, which is strictly worse than before (d-3). It is the price of the purchase and it
// is not optional.
//
// NARROWING. Every arm refuses; none admits a block that was previously refused. The rule is
// era-gated: pre-v5 blocks must carry NEITHER digest, so no committed v2/v4 bytes are
// re-interpreted (the pointers are nil there and omitempty omits them — chain.go:565-575).
//
// Gates: G-D3-1..G-D3-6 (d3digests_test.go), ablation red first.

var (
	// ErrD3DigestMismatch is a v5 digest that does not equal sha256 of its own content.
	ErrD3DigestMismatch = errors.New("chain: v5 payload digest does not match its content")
	// ErrD3DigestMissing is a v5 payload present with no digest committing it.
	ErrD3DigestMissing = errors.New("chain: v5 payload carries no digest")
	// ErrD3DigestPreV5 is a pre-v5 block carrying an era-4 digest field.
	ErrD3DigestPreV5 = errors.New("chain: pre-v5 block carries a v5-only digest field")
)

// slashesDigestOf is sha256 over the canonical encoding of s — the same canonical CBOR
// Block.bodyHash uses, so the digest commits exactly the bytes the preimage would have folded.
// Returns the zero hash for an empty slice, which is never committed (a nil Slashes carries a nil
// SlashesDigest).
func slashesDigestOf(s []Equivocation) ports.Hash {
	if len(s) == 0 {
		return ports.Hash{}
	}
	raw, err := encMode.Marshal(s)
	if err != nil {
		panic(err) // canonical encoding of our own struct cannot fail
	}
	return sha256.Sum256(raw)
}

// validateD3Digests is the (d-3) digest-consistency rule. It runs on every disk-write path, beside
// validateEra3Version / validateEra4Version, so the commit path and the own-disk Reload path
// enforce the identical rule — the #572 symmetry discipline.
func validateD3Digests(b *Block) error {
	if b.Version < BlockVersionWitnessable {
		// PRE-v5: neither digest may appear. Refusing here (rather than ignoring) keeps the
		// frozen-format story honest — a v2/v4 block whose bytes carry a v5-only field would
		// hash differently from the same block written by a pre-(d-3) binary.
		for i := range b.BondRegs {
			if b.BondRegs[i].AnswerDigest != nil {
				return fmt.Errorf("%w: v%d block, bondreg %d carries AnswerDigest", ErrD3DigestPreV5, b.Version, i)
			}
		}
		if b.SlashesDigest != nil {
			return fmt.Errorf("%w: v%d block carries SlashesDigest", ErrD3DigestPreV5, b.Version)
		}
		return nil
	}

	// v5: every registration commits its Answer by digest, present or pruned.
	for i := range b.BondRegs {
		r := &b.BondRegs[i]
		if r.AnswerDigest == nil {
			return fmt.Errorf("%w: bondreg %d (validator %s) has no AnswerDigest; a v5 registration commits its space-time proof by digest whether the proof is carried or pruned",
				ErrD3DigestMissing, i, r.ValidatorID())
		}
		if r.Answer == nil {
			// A pruned or digest-only registration: nothing to check the digest against. The
			// digest still had to be present (above), which is what keeps the preimage total.
			continue
		}
		if want := answerDigestOf(r.Answer); *r.AnswerDigest != want {
			return fmt.Errorf("%w: bondreg %d (validator %s) AnswerDigest %x != sha256(Answer) %x",
				ErrD3DigestMismatch, i, r.ValidatorID(), (*r.AnswerDigest)[:8], want[:8])
		}
	}

	// v5: Slashes is committed by exactly one digest, present iff the payload is.
	switch {
	case len(b.Slashes) == 0 && b.SlashesDigest != nil:
		return fmt.Errorf("%w: block carries SlashesDigest with no Slashes — the digest must be absent, not a digest of nothing", ErrD3DigestMismatch)
	case len(b.Slashes) > 0 && b.SlashesDigest == nil:
		return fmt.Errorf("%w: block carries %d slash proof(s) and no SlashesDigest; on v5 the preimage folds the DIGEST, so an absent digest leaves the payload unbound",
			ErrD3DigestMissing, len(b.Slashes))
	case len(b.Slashes) > 0:
		if want := slashesDigestOf(b.Slashes); *b.SlashesDigest != want {
			return fmt.Errorf("%w: SlashesDigest %x != sha256(canonical(Slashes)) %x",
				ErrD3DigestMismatch, (*b.SlashesDigest)[:8], want[:8])
		}
	}
	return nil
}

// setD3Digests populates the v5 digest fields from the block's own payloads. Proposer-side only —
// it is never a substitute for validateD3Digests, which every replica runs.
func setD3Digests(b *Block) {
	if b.Version < BlockVersionWitnessable {
		return
	}
	for i := range b.BondRegs {
		d := answerDigestOf(b.BondRegs[i].Answer)
		b.BondRegs[i].AnswerDigest = &d
	}
	if len(b.Slashes) > 0 {
		d := slashesDigestOf(b.Slashes)
		b.SlashesDigest = &d
	} else {
		b.SlashesDigest = nil
	}
}
