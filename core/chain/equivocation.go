// Equivocation is the consensus analogue of a storage liar: a validator that
// signs two DIFFERENT blocks at the SAME height, trying to make two competing
// histories both look supported. Fork-choice already means only the heavier
// fork stands (see Reconcile), and an honest validator refuses to double-sign
// (core/node); this file adds the PENALTY — a compact, self-verifying proof
// that any node can check and act on, so a proven double-sign costs the actor
// its standing (D2, §3e).
package chain

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/nerolabs/silt/ports"
)

// Equivocation is self-verifying evidence of a double-sign. It carries the two
// conflicting blocks; any node recomputes their hashes, confirms they are the
// SAME height but DIFFERENT blocks, and that the culprit's signature — as
// proposer OR attester — appears and verifies in BOTH. An honest validator
// signing sequential heights is never implicated (the heights differ); a
// forged accusation fails (the signatures won't verify under the culprit key).
type Equivocation struct {
	Culprit []byte `cbor:"1,keyasint"` // the equivocator's ed25519 public key
	A       Block  `cbor:"2,keyasint"`
	B       Block  `cbor:"3,keyasint"`
}

// CulpritID is the NodeID (hash of the public key) of the equivocator.
func (e *Equivocation) CulpritID() ports.NodeID { return sha256.Sum256(e.Culprit) }

// VerifyEquivocation reports whether e is valid, self-verifying proof of a
// double-sign — no external state needed. Era-gated (#432 rounds):
//
//   - Both blocks era 1: the legacy rule — the culprit signed two different
//     blocks at one HEIGHT (proposer or attester signature alike).
//   - Both blocks era 2: the culprit released two consensus signatures at the
//     same (HEIGHT, ROUND, PHASE) over different hashes. A cross-round
//     different-hash signature is HONEST (a lock-change under a POL — the
//     certification's I5 requirement), and a bare-hash ProposerSig is
//     authorship, not a consensus vote (re-proposing fresh at a higher round
//     after a lock-free view-change is honest), so neither is evidence.
//   - Mixed eras: not evidence (conservative — the upgrade boundary must never
//     manufacture an honest slash; refusing is the fail-safe direction).
func VerifyEquivocation(e *Equivocation) bool {
	if len(e.Culprit) != ed25519.PublicKeySize {
		return false
	}
	if e.A.Height != e.B.Height {
		return false // sequential signing is not equivocation
	}
	ha, hb := e.A.Hash(), e.B.Hash()
	if ha == hb {
		return false // the same block signed twice is not a conflict
	}
	av2, bv2 := e.A.Version >= BlockVersionRounds, e.B.Version >= BlockVersionRounds
	if av2 != bv2 {
		return false // mixed eras: never slashable (fail-safe)
	}
	if av2 {
		// Era 2: the two signatures must share (round, phase).
		for _, sa := range consensusSigScopes(e.Culprit, &e.A, ha) {
			for _, sb := range consensusSigScopes(e.Culprit, &e.B, hb) {
				if sa == sb {
					return true
				}
			}
		}
		return false
	}
	return signedBlock(e.Culprit, &e.A, ha) && signedBlock(e.Culprit, &e.B, hb)
}

// sigScope identifies one consensus signature's slot: the (round, phase) it
// was released at. Height is shared by construction (checked above).
type sigScope struct {
	Round uint64
	Phase uint8
}

// consensusSigScopes collects the verified (round, phase) slots at which pub
// released an era-2 consensus signature in b — across BOTH certificate sets
// (PrepareQC and Atts). The bare-hash ProposerSig is authorship, not a vote,
// and is deliberately excluded (see VerifyEquivocation).
func consensusSigScopes(pub []byte, b *Block, h ports.Hash) []sigScope {
	var out []sigScope
	for _, set := range [][]Attestation{b.PrepareQC, b.Atts} {
		for _, a := range set {
			if a.Phase == PhaseLegacy {
				continue // a legacy-shaped sig inside an era-2 block is not a vote slot
			}
			if bytes.Equal(a.PubKey, pub) && verifyAtt(a, h) {
				out = append(out, sigScope{Round: a.Round, Phase: a.Phase})
			}
		}
	}
	return out
}

// signedBlock reports whether pub's ERA-1 signature over h appears in b — as
// its proposer or as one of its attesters — and verifies.
func signedBlock(pub []byte, b *Block, h ports.Hash) bool {
	if bytes.Equal(b.Proposer, pub) && ed25519.Verify(ed25519.PublicKey(pub), h[:], b.ProposerSig) {
		return true
	}
	for _, a := range b.Atts {
		if bytes.Equal(a.PubKey, pub) && ed25519.Verify(ed25519.PublicKey(a.PubKey), h[:], a.Sig) {
			return true
		}
	}
	return false
}

// FindEquivocations scans two competing histories for validators who signed a
// DIFFERENT block at the SAME height in each — provable double-signers. When a
// node sees a fork (e.g. reconciling to a heavier one), the two chains are the
// evidence: anyone who backed both sides at a shared height equivocated.
// Returns one proof per distinct culprit.
func FindEquivocations(a, b []Block) []Equivocation {
	byHeight := make(map[uint64]*Block, len(a))
	for i := range a {
		byHeight[a[i].Height] = &a[i]
	}
	var out []Equivocation
	caught := make(map[ports.NodeID]bool)
	for i := range b {
		bb := &b[i]
		ab, ok := byHeight[bb.Height]
		if !ok || ab.Hash() == bb.Hash() {
			continue // no block at this height on the other side, or the same block
		}
		for _, pub := range signers(ab) {
			id := ports.NodeID(sha256.Sum256(pub))
			if caught[id] {
				continue
			}
			e := Equivocation{Culprit: pub, A: *ab, B: *bb}
			if VerifyEquivocation(&e) {
				caught[id] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// signers returns the public keys that signed b, across EVERY signing role the
// equivocation verifier checks: proposer, PrepareQC (prepare), and Atts
// (precommit). Candidate SELECTION must match VERIFICATION coverage — this set
// feeds FindEquivocations, and a culprit omitted here is never even tested by
// VerifyEquivocation. The #496 seam (research-certified 2026-08-21): this
// function read proposer+Atts only, so an era-2 equivocator whose signature in
// the canonical block sat ONLY in PrepareQC — the objective-mode island
// adversary at the genesis child, where the culprit is reliably prepare-only —
// was unslashable even though the verifier would have convicted it. Widening
// the candidate set cannot manufacture a false slash: VerifyEquivocation
// remains the gate, with its honest exemptions (sequential heights, cross-round
// lock-change under a POL, bare-hash authorship) intact.
// Certification: silt-reviews/research/research-outcome/
// 496-height1-equivocation-undetected-RESEARCH-CERTIFICATION-2026-08-21.md.
func signers(b *Block) [][]byte {
	out := make([][]byte, 0, 1+len(b.PrepareQC)+len(b.Atts))
	if len(b.Proposer) == ed25519.PublicKeySize {
		out = append(out, b.Proposer)
	}
	for _, set := range [][]Attestation{b.PrepareQC, b.Atts} {
		for _, a := range set {
			if len(a.PubKey) == ed25519.PublicKeySize {
				out = append(out, a.PubKey)
			}
		}
	}
	return out
}
