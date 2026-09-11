package node

// R-CARRIER-QC-BURST-VALUE — the residual #823 left open, DRIVEN.
//
// #823 closed the UNBONDED arm of the MsgPrepareQC flood with a sender screen
// ((*Node).handleChain, ports.MsgPrepareQC: Objective() &&
// !AttesterEligibleAt(from, height)). A BONDED sender passes that screen by
// construction, so the flood survives for anyone already in the governing set.
// The per-sender rate budget that would bound it is NOT shipped: its burst
// constant is a security parameter and is research-gated, so no number was
// guessed (chainrole.go, the ports.MsgPrepareQC arm).
//
// THIS FILE EXISTS BECAUSE A RESIDUAL'S FAILURE MODE IS ITSELF A CLAIM. The
// obvious statement of the residual — "a bonded validator replays its own
// signature to buy 131,072 verifies" — is FALSE, and a different check is why:
// (*Chain).collectQuorumSigs sets seen[id] at the BOTTOM of the loop, after the
// qualification test, so the dedup short-circuit `if seen[id] { continue }`
// fires only for ids that already qualified. That makes the behaviour
// ASYMMETRIC, and the asymmetry is the whole residual:
//
//   - repeats of a QUALIFIED id are deduplicated BEFORE verifyAtt — the
//     attacker's own bonded key buys exactly ONE verify, however often it is
//     repeated;
//   - repeats of an UNQUALIFIED id are NOT deduplicated — each one pays a full
//     verifyAtt.
//
// So the bond does not supply the payload; it supplies PASSAGE through the
// sender screen. The flood entries must be signed by an identity that is NOT in
// the governing set, which costs one offline ed25519.Sign and no bond at all.
//
// The gate below drives both halves at once with a corrupt-signature entry
// placed LAST. A corrupt entry is FATAL to collectQuorumSigs, so ErrBadSignature
// is a positive observation that the loop WALKED to that entry, and its absence
// is a positive observation that the loop stopped short. That observable needs
// no timer and no verify counter, so it cannot go flaky.
//
// If someone later moves `seen[id] = true` above the qualification test, or caps
// len(qc), this test goes RED — which is the point. A residual whose mechanism
// has silently closed must not keep its row.

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// floodCorrupt returns a copy of a with its signature broken. PubKey, Phase and
// Round are untouched, so the entry is still the SAME attester id at the SAME
// (phase, round) — it fails only at verifyAtt, which is the step we are probing
// for.
func floodCorrupt(a chain.Attestation) chain.Attestation {
	c := a
	s := make([]byte, len(a.Sig))
	copy(s, a.Sig)
	s[0] ^= 0xff
	c.Sig = s
	return c
}

// TestQC_BurstValue_DedupIsAsymmetricAcrossQualification drives the mechanism
// R-CARRIER-QC-BURST-VALUE discloses.
func TestQC_BurstValue_DedupIsAsymmetricAcrossQualification(t *testing.T) {
	nodes, ids, _, g, _ := tier2AnchorNet(t, 4)
	author := ids[0]

	b := &chain.Block{Version: chain.BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Entries: []ports.Entry{mkEntry("qc-burst-value")}}
	chain.Sign(b, author.Signer())
	if err := nodes[0].chain.ValidateProposal(b); err != nil {
		t.Fatalf("premise: the author's block must be a valid proposal: %v", err)
	}
	cid := nodes[0].chainID()

	// The author's own self-prepare — the seed (*Chain).requireProposerPrepare
	// demands. Present in both arms so neither arm is refused for a reason
	// other than the one under test.
	self := chain.AttestAt(b, author.Signer(), 0, chain.PhasePrepare, cid)

	// ARM 1 — a QUALIFIED attester, repeated, corrupt copy last.
	//
	// ids[1] is an anchor, so attesterQualifiedAt(ids[1]) is true and the FIRST
	// entry sets seen[id]. Every later entry with that id — including the
	// corrupt one — is skipped by `if seen[id] { continue }` before verifyAtt.
	// The refusal must therefore NOT be ErrBadSignature: the verifier never
	// reached the broken entry.
	qual := chain.AttestAt(b, ids[1].Signer(), 0, chain.PhasePrepare, cid)
	err1 := nodes[0].chain.VerifyPrepareQC(b, []chain.Attestation{self, qual, qual, floodCorrupt(qual)}, 0)
	if errors.Is(err1, chain.ErrBadSignature) {
		t.Fatalf("R-CARRIER-QC-BURST-VALUE HAS CHANGED SHAPE: a repeated QUALIFIED id reached the "+
			"corrupt entry, so seen[id] no longer short-circuits before verifyAtt. The residual's "+
			"disclosed mechanism says the attacker's own bonded key buys exactly ONE verify; that is "+
			"now false and the ROADMAP row must be re-derived. err=%v", err1)
	}
	if err1 == nil {
		t.Fatalf("premise: a QC carrying one qualified attester must not reach quorum, got nil")
	}

	// ARM 2 — an UNQUALIFIED attester, repeated, corrupt copy last.
	//
	// A freshly generated identity is in no anchor set and holds no bond, so
	// attesterQualifiedAt is false and seen[id] is NEVER set. Nothing
	// deduplicates the repeats, so the loop pays verifyAtt for each and walks
	// all the way to the corrupt entry — which is exactly the work an attacker
	// buys, and the reason the entry count is the quantity that needs a bound.
	outsider, err := identity.Generate(rand.Reader)
	if err != nil {
		t.Fatalf("generate outsider: %v", err)
	}
	unqual := chain.AttestAt(b, outsider.Signer(), 0, chain.PhasePrepare, cid)
	err2 := nodes[0].chain.VerifyPrepareQC(b, []chain.Attestation{self, unqual, unqual, floodCorrupt(unqual)}, 0)
	if !errors.Is(err2, chain.ErrBadSignature) {
		t.Fatalf("R-CARRIER-QC-BURST-VALUE IS CLOSED OR HAS MOVED: a repeated UNQUALIFIED id did NOT "+
			"reach the corrupt entry, so something now bounds or deduplicates the list before "+
			"verifyAtt. If a cap or a dedup shipped, close the register row rather than leaving it "+
			"open. err=%v", err2)
	}
}

// TestQC_BurstValue_TheSenderScreenAdmitsAQualifiedFlooder records the other
// half of the residual: what #823's screen does and does not do.
//
// (*Chain).AttesterEligibleAt is the exact predicate the ports.MsgPrepareQC arm
// screens on. It is a property of the SENDER, not of the list the sender
// carries, so a bonded validator passes it and reaches VerifyPrepareQC with a
// list of any length the CBOR decoder admits. This asserts the predicate
// directly, so the disclosure's "a bonded validator still passes" clause is
// driven rather than asserted in prose.
func TestQC_BurstValue_TheSenderScreenAdmitsAQualifiedFlooder(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	n := nodes[0]
	h := n.roundsFor().Height

	if !n.chain.AttesterEligibleAt(ids[1].NodeID(), h) {
		t.Fatalf("premise: an anchor must be eligible at height %d — the screen #823 added is "+
			"what this residual reports a bonded sender passing", h)
	}

	outsider, err := identity.Generate(rand.Reader)
	if err != nil {
		t.Fatalf("generate outsider: %v", err)
	}
	if n.chain.Objective() && n.chain.AttesterEligibleAt(outsider.NodeID(), h) {
		t.Fatal("R-CARRIER-QC-BURST-VALUE's boundary has moved: an unbonded identity is now eligible, " +
			"so #823's sender screen no longer separates the closed arm from the open one")
	}
}
