// TESTER PROBE — NOT FOR MERGE. G-PRE-7's "a v2-form and a v5-form signature at ONE slot
// DO convict" and G-PRE-6's "an honest validator on two networks must NOT be slashable"
// are claims about the SAME evidence object. This drives that.
package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestProbe_BoundaryDoubleSignIsByteIdenticalToCrossNetworkHonest(t *testing.T) {
	victim, prop := key(96001), key(96002)
	cidHome := ports.HashBytes([]byte("the REAL silt network genesis"))
	cidEvil := ports.HashBytes([]byte("an attacker-run pre-era-4 silt network"))
	const h, r = 8, 0

	// ONE block body. In the malicious reading it is the v4 block the culprit ALSO
	// precommitted on its own network across the upgrade (G-PRE-7's case). In the honest
	// reading it is a block on a DIFFERENT network the victim validates (G-PRE-6's property).
	mkV4 := func(cid ports.Hash) Block {
		b := Block{Version: BlockVersionStateRoot, Height: h, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(1)}}
		Sign(&b, prop)
		b.Atts = []Attestation{AttestAt(&b, victim, r, PhasePrecommit, cid)}
		return b
	}
	onHome, onEvil := mkV4(cidHome), mkV4(cidEvil)

	// THE CORE FACT: the v2-form signature is a function of (phase, round, blockHash) only.
	if !bytes.Equal(onHome.Atts[0].Sig, onEvil.Atts[0].Sig) {
		t.Fatal("the v2 leg IS chain-bound after all — the whole finding collapses; re-examine verifyAtt")
	}
	t.Log("MEASURED: the victim's v2-form precommit over one body is BYTE-IDENTICAL whichever " +
		"network it was released on. verifyAtt's `case PhasePrepare, PhasePrecommit` ignores the scope.")

	// The v5 leg, released honestly on the real network at the same (h, r).
	v5leg := Block{Version: BlockVersionWitnessable, Height: h, Prev: ports.HashBytes([]byte("p")),
		Entries: []ports.Entry{entry(2)}}
	v5leg.StateRoot, v5leg.LogRoot = &ports.Hash{}, &ports.Hash{}
	setD3Digests(&v5leg)
	Sign(&v5leg, prop)
	v5leg.Atts = []Attestation{AttestAt(&v5leg, victim, r, PhasePrecommit, cidHome)}

	guilty := Equivocation{Culprit: append([]byte(nil), victim.Public().(ed25519.PublicKey)...),
		A: onHome, B: v5leg} // the boundary double-signer G-PRE-7 says MUST convict
	innocent := Equivocation{Culprit: append([]byte(nil), victim.Public().(ed25519.PublicKey)...),
		A: onEvil, B: v5leg} // two honest acts G-PRE-6's property says must NOT convict

	if !bytes.Equal(Encode(&guilty.A), Encode(&innocent.A)) {
		t.Fatal("the two evidence objects differ on the wire — a verifier COULD separate them")
	}
	t.Log("MEASURED: the two Equivocation objects are BYTE-IDENTICAL on the wire (CBOR-encoded " +
		"evidence block A is equal). No verifier can separate the guilty case from the innocent one.")

	gErr := CheckEquivocation(&guilty, cidHome)
	iErr := CheckEquivocation(&innocent, cidHome)
	t.Logf("VERDICT guilty  (G-PRE-7 requires CONVICT)  -> %v", gErr)
	t.Logf("VERDICT innocent(G-PRE-6 property requires REFUSE) -> %v", iErr)
	if gErr == nil && iErr == nil {
		t.Log("CONTRADICTION DRIVEN: both gates are green on this branch and they demand opposite " +
			"verdicts on one object. The shipped rule satisfies G-PRE-7 and violates G-PRE-6's " +
			"stated property; G-PRE-6 only ever instantiates that property on the (v5,v5) pair.")
	}
	if errors.Is(iErr, ErrNotEquivocation) {
		t.Log("NO CONTRADICTION: the innocent pair is refused — report REFUTED")
	}
}
