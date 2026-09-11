// TESTER PROBE — NOT FOR MERGE. Drives the certification §5.4 UNSETTLED case.
// Diagnosis only; written in the tester's own worktree, never on the held branch.
package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// probeBuild is TestGPRE6_CrossChainHonestSignaturesAreNotEvidence's own `build` helper,
// copied verbatim except that the height/round are parameters.
func probeBuild(t *testing.T, version uint64, h, r uint64, e byte, prop, victim ed25519.PrivateKey, cid ports.Hash) Block {
	t.Helper()
	b := Block{Version: version, Height: h, Prev: ports.HashBytes([]byte("shared-prev")),
		Entries: []ports.Entry{entry(e)}}
	if version >= BlockVersionWitnessable {
		b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
		setD3Digests(&b)
	}
	Sign(&b, prop)
	b.Atts = []Attestation{AttestAt(&b, victim, r, PhasePrecommit, cid)}
	return b
}

// TestProbe_CrossForm drives the two pairs the certification names, one at a time.
func TestProbe_CrossForm(t *testing.T) {
	victim, propX, propY := key(96001), key(96002), key(96003)
	cidX := ports.HashBytes([]byte("silt network X genesis"))
	cidY := ports.HashBytes([]byte("silt network Y genesis"))
	pub := []byte(victim.Public().(ed25519.PublicKey))
	const h, r = 12, 1

	av4 := probeBuild(t, BlockVersionStateRoot, h, r, 1, propX, victim, cidX)
	bv5 := probeBuild(t, BlockVersionWitnessable, h, r, 2, propY, victim, cidY)
	av5 := probeBuild(t, BlockVersionWitnessable, h, r, 1, propX, victim, cidX)
	bv4 := probeBuild(t, BlockVersionStateRoot, h, r, 2, propY, victim, cidY)

	// FIXTURE NON-VACUITY: assert each leg is the FORM it claims to be.
	if av4.Atts[0].Phase != PhasePrecommit || bv4.Atts[0].Phase != PhasePrecommit {
		t.Fatalf("FIXTURE BROKEN: v4 legs must carry the chain-blind v2 form; got %d / %d",
			av4.Atts[0].Phase, bv4.Atts[0].Phase)
	}
	if av5.Atts[0].Phase != PhasePrecommitV5 || bv5.Atts[0].Phase != PhasePrecommitV5 {
		t.Fatalf("FIXTURE BROKEN: v5 legs must carry the era-4 form; got %d / %d",
			av5.Atts[0].Phase, bv5.Atts[0].Phase)
	}
	if bytes.Equal(av4.Atts[0].Sig, bv4.Atts[0].Sig) || bytes.Equal(av5.Atts[0].Sig, bv5.Atts[0].Sig) {
		t.Fatal("FIXTURE BROKEN: the two networks' signatures must differ")
	}
	if av4.bodyHash() == bv5.bodyHash() {
		t.Fatal("FIXTURE BROKEN: the two evidence bodies must differ")
	}

	report := func(name string, e Equivocation, cid ports.Hash, cidName string) {
		err := CheckEquivocation(&e, cid)
		verdict := "ACCEPTED AS EVIDENCE (CheckEquivocation == nil) -> SLASHABLE"
		switch {
		case err == nil:
		case errors.Is(err, ErrNotEquivocation):
			verdict = "REFUSED: ErrNotEquivocation"
		case errors.Is(err, ErrPrunedEvidence):
			verdict = "REFUSED: ErrPrunedEvidence"
		default:
			verdict = "REFUSED: " + err.Error()
		}
		t.Logf("PROBE %-10s verifier=%-35s : %s", name, cidName, verdict)
	}

	// (1) MIXED FORM (v2 leg, v5 leg) — the §5.4 case.
	report("(v2,v5)", Equivocation{Culprit: pub, A: av4, B: bv5}, cidY, "network Y (owner of the v5 leg)")
	report("(v2,v5)", Equivocation{Culprit: pub, A: av4, B: bv5}, cidX, "network X (owner of the v2 leg)")
	// symmetric ordering, to rule out an argument-order artefact
	report("(v5,v2)", Equivocation{Culprit: pub, A: av5, B: bv4}, cidX, "network X (owner of the v5 leg)")
	report("(v5,v2)", Equivocation{Culprit: pub, A: av5, B: bv4}, cidY, "network Y (owner of the v2 leg)")

	// (2) BOTH-V2 FORM — the pre-existing face, independent of call A.
	report("(v2,v2)", Equivocation{Culprit: pub, A: av4, B: bv4}, cidY, "network Y")
	report("(v2,v2)", Equivocation{Culprit: pub, A: av4, B: bv4}, cidX, "network X")
	report("(v2,v2)", Equivocation{Culprit: pub, A: av4, B: bv4}, ports.Hash{}, "zero chain id")

	// (3) BOTH-V5 FORM — the case G-PRE-6 already covers, as a control.
	report("(v5,v5)", Equivocation{Culprit: pub, A: av5, B: bv5}, cidY, "network Y")
	report("(v5,v5)", Equivocation{Culprit: pub, A: av5, B: bv5}, cidX, "network X")
}
