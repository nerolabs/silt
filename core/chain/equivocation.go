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
//
// chainID is the VERIFIER'S OWN network identity (Chain.ChainID). From era 4 the consensus
// preimage binds it, so a signature released on another silt network cannot verify here and
// cannot be evidence here. That matters because the genesis moved on 2026-09-07: an operator
// who carries one identity key across two silt networks and honestly precommits a different
// block at the same (height, round) on each was, without this field, producing a valid
// equivocation proof against itself on both. See consensusSigBytesV5 and
// TestGPRE6_CrossChainHonestSignaturesAreNotEvidence.
//
// floor is the VERIFIER'S OWN era floor (M2) — see EraFloor and CheckEquivocation.
func VerifyEquivocation(e *Equivocation, chainID ports.Hash, floor EraFloor) bool {
	return CheckEquivocation(e, chainID, floor) == nil
}

// EraFloor reports the MINIMUM BlockVersion that evidence at height h must carry to be
// admissible — the version the chain itself requires a proposer to stamp at that height. It is
// (*Chain).MintVersion on a node, and v5EraFloorAt over a StateView inside the accept
// composition; both read committed-or-genesis-covered state, so every replica computes the same
// floor (canon rule 8, divergence-free).
//
// WHY IT IS A REQUIRED PARAMETER AND NOT A METHOD BESIDE THE FREE FUNCTION. The rule must land
// at every site that decides admissibility, and there are FOUR on two different paths:
// v5ValidateSlashes (P8, the LIVE ACCEPT path of an era-4 block), (*Chain).validateSlashes (the
// own-disk RELOAD path, and the live accept path for eras 1-3), FindEquivocations (detection)
// and (*Node).slashEquivocators (the on-chain queue). A signature change turns a missed site
// into a compile error; a method beside the old one is how the fourth site was missed on paper.
// A method could not have served site 1 at all: the accept composition takes a StateView and is
// SOURCE-GATED against holding a *Chain (TestColdAuditor_NoTrustFloorOnTheContractSurface).
//
// WHY IT IS HEIGHT-INDEXED AND EVALUATED INSIDE THE GATE. FindEquivocations iterates heights and
// has no single scalar to supply. More importantly, a pre-evaluated scalar makes "the caller
// evaluated the floor at the wrong height" EXPRESSIBLE, and a compile error catches an omission
// but never a wrong value. CheckEquivocation evaluates the supplier at e.A.Height itself — legal
// because e.A.Height == e.B.Height is established before the call — so the wrong-height failure
// is inexpressible rather than gated.
//
// WHY IT IS NEVER A PARAMETER OF THE ACCEPT COMPOSITION. The floor is a fact the VERIFIER holds;
// a floor LOWERED by a caller re-admits exactly the evidence this rule refuses (wrong-accept).
// That is the same hazard direction as the pruned-TRUST floor the StateView already names
// "floor" (PrunedTolerated) — a trust floor RAISED by a caller makes the reader skip proof
// verification. They are different quantities sharing one hazard, and both stay off the contract
// surface: ValidateCommitV5/ValidateProposalV5 take exactly (StateView, *Block) and no bare
// uint64, pinned by TestColdAuditor_NoTrustFloorOnTheContractSurface.
//
// Certification: NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11 §1.5c
// (conditions C-1, C-2, C-3), Layer 1 GATED -> G-1b.
type EraFloor func(height uint64) uint64

// CheckEquivocation is VerifyEquivocation with the refusal named: nil iff e proves a
// double-sign; ErrPrunedEvidence when an evidence block is pruned; ErrNotEquivocation
// for the honest exemptions.
//
// R0.6 (F2-EVIDENCE-RECOMPUTE, certification I5-cross-height-pruned-slash-forgery-
// FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03 §5): the two block hashes are ALWAYS
// recomputed from the bodies (bodyHash) — never read from Block.Pruned and never from
// the hashMemo cache — and a pruned block is refused outright. The height check below
// is sound only because the signed message is a digest OVER the declared Height; for
// a non-pruned body those are the same source. Reading Pruned severed them: two GENUINE
// signatures by an honest validator at two DIFFERENT heights, re-labelled with one
// fictitious height and carrying another block's real hash in Pruned, verified as a
// double-sign and evicted the honest validator through Append (I5 broken, era 1 and 2).
// Refusing pruned evidence is strictly narrowing — it can never manufacture a slash —
// and it is the rule this type's doc comment has always stated ("recomputes their
// hashes"). The cost (a double-sign whose evidence was already payload-pruned is
// unslashable) is R-LATE-REVEAL, owned in docs/decisions.md D-F2-EVIDENCE-RECOMPUTE.
// This is the one gate for EVERY path that decides admissibility, so an honest proposer can
// never queue a proof every replica rejects. There are FOUR, and they are classified by PATH,
// not by name — two of them are called validateSlashes:
//
//   - v5ValidateSlashes (P8 of the one accept composition) — the LIVE ACCEPT path of an era-4
//     block: ValidateProposal, ValidateCommit, Append, Reconcile, and (*Box).Validate;
//   - (*Chain).validateSlashes — the own-disk RELOAD path (appendStructural -> validateStructural)
//     for an era-4 block, and the live accept path for an era-1/2/3 one;
//   - FindEquivocations — detection/selection;
//   - (*Node).slashEquivocators — the on-chain queue.
//
// A rule that lands at some of them and not others is worse than one that lands at none: accept
// live and refuse on reload is ONE OPERATOR DIVERGING FROM ITSELF across a restart.
//
// THE PRUNED REFUSAL AND THE BODY RECOMPUTE STAY, VERBATIM AND FOREVER. era-4 binds the height
// inside the signed message, which makes the R0.6 cross-height forgery structurally
// inexpressible for a v5-form signature. It does NOT make this rule redundant: era-1, era-2 and
// era-3 signatures bind no height, committed history is never re-interpreted, and every height
// below H_era4 exists forever. Deleting ErrPrunedEvidence or the bodyHash recompute "because
// evidence binds its height now" would silently re-open I5 for every pre-era-4 height. Named as
// a DON'T by the owner-call-A certification §2.3.
func CheckEquivocation(e *Equivocation, chainID ports.Hash, floor EraFloor) error {
	if floor == nil {
		// C-3: an ABSENT supplier REFUSES. uint64(0) is a valid-looking floor meaning "no floor"
		// — today's fail-open — and it is what a caller gets for free, on an exported function
		// with an out-of-package caller (core/node). A node that holds no chain computes no
		// floor and must convict nobody, the same direction (*Node).chainID already takes with
		// its zero hash: a node that cannot know which network it is on must not guess. This
		// check precedes the floor READ, so a chainless node never evaluates one.
		return ErrNotEquivocation
	}
	if len(e.Culprit) != ed25519.PublicKeySize {
		return ErrNotEquivocation
	}
	if e.A.IsPruned() || e.B.IsPruned() {
		return ErrPrunedEvidence // a pruned body cannot reproduce its hash: no height is bound
	}
	if e.A.Height != e.B.Height {
		return ErrNotEquivocation // sequential signing is not equivocation
	}
	// THE ERA FLOOR (M2). Evidence whose FORM is below what this chain requires at the evidence's
	// own height is not evidence here: a sub-era-4 attestation binds no chain id (consensusSigBytes
	// is FROZEN and reads no scope), so a leg harvested from another silt network is bit-identical
	// to one released here, and pairing it with the victim's own honest signature convicted the
	// honest — I5, the #397 shape. Evaluated at e.A.Height, which the check above has just
	// established equals e.B.Height.
	//
	// STRICTLY NARROWING, and that is the whole safety argument: this is a CONJUNCT on the accept
	// condition, so it can only decline a slash and can never manufacture one. It is the identical
	// argument form the pruned refusal above carries.
	//
	// WHAT IT COSTS, ratified and not overlooked: a validator that double-signs ACROSS an era
	// boundary (one leg in the old form, one in the new) becomes unslashable. It cannot finalize
	// either way — validateEra4Version refuses a sub-era-4 block at every height at or above the
	// boundary on every disk-write path — and on a genesis committing Era4ActivationHeight = 1
	// there is no such boundary to stand on. Certification §1.9 G-1d, owner-ratified.
	//
	// THE SCOPE — `f >= BlockVersionWitnessable` — IS THE CERTIFICATION'S OWN CLOSURE TABLE (§1.3),
	// AND WHAT IT DEFERS IS NAMED HERE RATHER THAN LEFT SILENT.
	//
	// The rule closes the cross-network face at exactly the heights where the chain requires the
	// CHAIN-BOUND form. §1.3's three rows: (v2,v5) at h >= H_era4 CLOSED; (v2,v2) at h >= H_era4
	// CLOSED; (v2,v2) at h < H_era4 "NOT CLOSED — floor 2 or 4". The third row is not closed by a
	// floor of 2 or 4 and cannot be: below the era-4 boundary the REQUIRED form is itself
	// chain-blind (consensusSigBytes reads no scope and is frozen forever), so an attacker simply
	// harvests a leg of the required form. Refusing a leg below a v2/v4 floor therefore buys ZERO
	// closure by the table's own verdict.
	//
	// An unscoped comparison would additionally refuse (i) every era-1-form pair at every height on
	// every chain, since MintVersion's minimum is v2, and (ii) era-1/era-2-form pairs at era-3
	// heights on a latch-route chain. Both are strictly narrowing and therefore safe — but they are
	// a REPRICING of two artifacts this certification does not name and a builder must not amend:
	// TestModelCheck_I5_CrossHeightPrunedExtension_Era1, the era-1 exhaustive enumeration (v1-form
	// blocks at a v2 floor, which it demands CONVICT), and the T1 variant of R-NEST-GATE, whose own
	// gate says "do not read a RED here as the residual closed; update
	// SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10 §6.1". Deleting this
	// clause is a one-token change and is the right change the day a ruling prices those two; until
	// then the surplus is DEFERRED, not overlooked, and the deferred surface is DRIVEN by
	// TestGEF8_TheDeferredSubEra4SurfaceIsStillAdmissible.
	//
	// On a genesis committing Era4ActivationHeight = 1 — the source-pinned default, G-NET-2/G-NET-3
	// — the scope changes NOTHING: every height above the genesis has floor v5, and AppendGenesis
	// refuses a height-0 slash outright. The two forms of this rule are identical on the RC network.
	if f := floor(e.A.Height); f >= BlockVersionWitnessable && (e.A.Version < f || e.B.Version < f) {
		return ErrNotEquivocation
	}
	ha, hb := e.A.bodyHash(), e.B.bodyHash()
	if ha == hb {
		return ErrNotEquivocation // the same block signed twice is not a conflict
	}
	av2, bv2 := e.A.Version >= BlockVersionRounds, e.B.Version >= BlockVersionRounds
	if av2 != bv2 {
		return ErrNotEquivocation // mixed eras: never slashable (fail-safe)
	}
	if av2 {
		// Era 2 and up: the two signatures must share (round, STEP).
		for _, sa := range consensusSigScopes(e.Culprit, &e.A, chainID, ha) {
			for _, sb := range consensusSigScopes(e.Culprit, &e.B, chainID, hb) {
				if sa == sb {
					return nil
				}
			}
		}
		return ErrNotEquivocation
	}
	if signedBlock(e.Culprit, &e.A, ha) && signedBlock(e.Culprit, &e.B, hb) {
		return nil
	}
	return ErrNotEquivocation
}

// sigScope identifies one consensus signature's SLOT: the (round, step) it was released at.
// Height is shared by construction (checked above).
//
// STEP, NOT WIRE PHASE — and this is T-STEP-VS-FORM applied to the slash rule. From era 4 the
// same step has two wire constants (PhasePrecommit = 2, PhasePrecommitV5 = 4) because the signed
// PREIMAGE changed, not because the slot did. The durable anti-double-sign watermark
// (ports.SignMark) records the canonical step and is era-independent, so an honest validator
// releases exactly one signature per (height, round, step) ACROSS the boundary. If this type
// recorded the wire phase instead, the two halves of one mechanism would disagree about the slot
// again — which is the #397 scar this whole change was bought on — and a validator that
// precommitted a v4 block and a v5 block at one height would become unslashable. The golden
// corpus case `v4-vs-v5-both-rounds-era-same-slot-ACCEPT` is the driven proof.
type sigScope struct {
	Round uint64
	Step  uint8
}

// canonicalStep maps a WIRE phase back to the canonical consensus STEP — the inverse of AttPhase
// on the two steps it renames. Everything else (PhaseLegacy, unknown) passes through, which is
// safe because consensusSigScopes skips PhaseLegacy and verifyAtt refuses unknown phases.
func canonicalStep(phase uint8) uint8 {
	switch phase {
	case PhasePrepareV5:
		return PhasePrepare
	case PhasePrecommitV5:
		return PhasePrecommit
	default:
		return phase
	}
}

// consensusSigScopes collects the verified (round, phase) slots at which pub
// released an era-2 consensus signature in b — across BOTH certificate sets
// (PrepareQC and Atts). The bare-hash ProposerSig is authorship, not a vote,
// and is deliberately excluded (see VerifyEquivocation).
func consensusSigScopes(pub []byte, b *Block, chainID ports.Hash, h ports.Hash) []sigScope {
	var out []sigScope
	// The scope is the EVIDENCE BLOCK's own height (CheckEquivocation has already established
	// that e.A.Height == e.B.Height) under the verifier's own chain id.
	s := attScope{ChainID: chainID, Height: b.Height}
	for _, set := range [][]Attestation{b.PrepareQC, b.Atts} {
		for _, a := range set {
			if a.Phase == PhaseLegacy {
				continue // a legacy-shaped sig inside an era-2 block is not a vote slot
			}
			if bytes.Equal(a.PubKey, pub) && verifyAtt(a, s, h) {
				out = append(out, sigScope{Round: a.Round, Step: canonicalStep(a.Phase)})
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
//
// floor is this verifier's era floor and is REQUIRED (M2): detection must refuse exactly what
// the write path refuses, or this node queues evidence its own v5ValidateSlashes rejects and
// every later proposal it makes is invalid — the permanent proposer self-wedge.
func FindEquivocations(a, b []Block, chainID ports.Hash, floor EraFloor) []Equivocation {
	byHeight := make(map[uint64]*Block, len(a))
	for i := range a {
		byHeight[a[i].Height] = &a[i]
	}
	var out []Equivocation
	caught := make(map[ports.NodeID]bool)
	for i := range b {
		bb := &b[i]
		// Candidate selection uses the SAME body-recomputed hash as CheckEquivocation
		// (R0.6 G-6): a Pruned digest or a stale memo must not decide which pairs are
		// even candidates. Cost: two body hashes per block of b that has a same-height
		// partner in a — so CALLERS must pass only the heights that can actually
		// diverge (the served suffix), never a shared genesis-rooted prefix
		// (core/node/chainrole.go, the detection call site; PE ruling F-3 measured
		// 228 ms/sweep at n=600 when the whole chain was passed).
		ab, ok := byHeight[bb.Height]
		if !ok || ab.bodyHash() == bb.bodyHash() {
			continue // no block at this height on the other side, or the same block
		}
		for _, pub := range signers(ab) {
			id := ports.NodeID(sha256.Sum256(pub))
			if caught[id] {
				continue
			}
			e := Equivocation{Culprit: pub, A: *ab, B: *bb}
			if VerifyEquivocation(&e, chainID, floor) {
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
// Certification: silt-agent-memory/researcher/reviews/research-outcome/
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
