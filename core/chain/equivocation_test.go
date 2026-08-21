package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func pubOf(p ed25519.PrivateKey) []byte { return []byte(p.Public().(ed25519.PublicKey)) }

// conflicting builds two DIFFERENT blocks at height 1 (different entries) on
// the same genesis, each with the given attesters.
func (w *world) conflicting(g *Block, proposerA, proposerB ed25519.PrivateKey, attA, attB []ed25519.PrivateKey) (a, b *Block) {
	a = &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(a, proposerA)
	for _, v := range attA {
		a.Atts = append(a.Atts, Attest(a, v))
	}
	b = &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(2)}}
	Sign(b, proposerB)
	for _, v := range attB {
		b.Atts = append(b.Atts, Attest(b, v))
	}
	return
}

// A validator that signs two different blocks at the same height is provably an
// equivocator; one signing sequential heights, or accused without a signature,
// is not.
func TestEquivocationProof(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	// vals[0] attests BOTH forks; the two forks have different proposers.
	a, b := w.conflicting(g, w.prop, w.vals[3], []ed25519.PrivateKey{w.vals[0]}, []ed25519.PrivateKey{w.vals[0]})

	if !VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *b}) {
		t.Fatal("a validator signing two different blocks at the same height must be provable")
	}

	// Sequential heights are honest, not equivocation.
	c := &Block{Version: 1, Height: 2, Prev: a.Hash(), Entries: []ports.Entry{entry(3)}}
	Sign(c, w.prop)
	c.Atts = []Attestation{Attest(c, w.vals[0])}
	if VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *c}) {
		t.Fatal("signing sequential heights must not count as equivocation")
	}

	// A validator who signed neither block cannot be framed.
	if VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[1]), A: *a, B: *b}) {
		t.Fatal("a validator who did not sign both blocks must not be implicated")
	}
	// The same block is not a conflict with itself.
	if VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *a}) {
		t.Fatal("the same block is not equivocation")
	}
}

// Given two competing histories, every cross-fork double-signer is caught and
// no honest one-fork signer is.
func TestFindEquivocationsAcrossForks(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	// vals[0], vals[1] attest both forks (equivocators); vals[2] only fork A.
	a, b := w.conflicting(g, w.prop, w.vals[3],
		[]ed25519.PrivateKey{w.vals[0], w.vals[1], w.vals[2]},
		[]ed25519.PrivateKey{w.vals[0], w.vals[1]})

	got := FindEquivocations([]Block{*g, *a}, []Block{*g, *b})
	caught := map[ports.NodeID]bool{}
	for i := range got {
		caught[got[i].CulpritID()] = true
	}
	if !caught[idOf(w.vals[0])] || !caught[idOf(w.vals[1])] {
		t.Fatal("both cross-fork attesters must be caught")
	}
	if caught[idOf(w.vals[2])] {
		t.Fatal("a validator who backed only one fork must not be implicated")
	}
	// Every returned proof is genuinely self-verifying.
	for i := range got {
		if !VerifyEquivocation(&got[i]) {
			t.Fatalf("FindEquivocations returned an unverifiable proof for %s", got[i].CulpritID())
		}
	}
}

// The #496 seam (research-certified 2026-08-21): candidate SELECTION must
// enumerate every signing role the VERIFIER checks. An era-2 equivocator whose
// signature in the canonical block sits ONLY in PrepareQC (it is neither the
// proposer nor a precommit-attester) was invisible to FindEquivocations —
// signers() read proposer+Atts only — while VerifyEquivocation, which scans
// PrepareQC too, would happily have proven the double-sign. Field shape: the
// objective-mode island equivocator at the genesis child (height 1), where the
// culprit is reliably prepare-only, went unslashed for a whole drill window
// (run 1642465-57233) while the identical act at height 2 was always caught.
// Born RED before the signers() widening; GREEN after (V5).
func TestFindEquivocations_PrepareOnlyCulprit(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()

	// Canonical W@1: proposed by w.prop; the culprit vals[0] appears ONLY in
	// PrepareQC. The precommit certificate (Atts) is carried by other validators.
	wblk := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(wblk, w.prop)
	wblk.PrepareQC = []Attestation{
		AttestAt(wblk, w.vals[0], 0, PhasePrepare),
		AttestAt(wblk, w.vals[1], 0, PhasePrepare),
	}
	wblk.Atts = []Attestation{
		AttestAt(wblk, w.vals[1], 0, PhasePrecommit),
		AttestAt(wblk, w.vals[2], 0, PhasePrecommit),
	}

	// Conflicting L@1: the culprit's prepare at the SAME (height, round, prepare)
	// slot over a different block — the self-incrimination the objective-mode
	// adversary plants (PlaceConflictingSigned).
	l := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(2)}}
	Sign(l, w.vals[3])
	l.PrepareQC = []Attestation{AttestAt(l, w.vals[0], 0, PhasePrepare)}

	// The verifier proves it — the double-sign is real and self-verifying.
	if !VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *wblk, B: *l}) {
		t.Fatal("VerifyEquivocation must prove a same-slot prepare double-sign")
	}

	// And the SELECTOR must therefore find it: detection is only as complete as
	// its candidate enumeration.
	got := FindEquivocations([]Block{*g, *wblk}, []Block{*g, *l})
	found := false
	for i := range got {
		if got[i].CulpritID() == idOf(w.vals[0]) {
			found = true
			if !VerifyEquivocation(&got[i]) {
				t.Fatal("the returned prepare-only proof must self-verify")
			}
		}
	}
	if !found {
		t.Fatal("#496: a prepare-only equivocator must be selected by FindEquivocations — a culprit the verifier can convict must never be skipped by the candidate enumeration")
	}
	// And no honest participant is implicated by the widened selection.
	for i := range got {
		if got[i].CulpritID() != idOf(w.vals[0]) {
			t.Fatalf("only the culprit may be implicated; got %s", got[i].CulpritID())
		}
	}
}
