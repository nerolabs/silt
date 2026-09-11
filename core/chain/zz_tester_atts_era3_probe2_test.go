package chain

// TESTER PROBE 2 — R-ATTS-ERA3-ROOT-COUPLING, the LIVE-NETWORK scenario:
// a new validator bonds in AFTER the era-3 boundary and then attests. No attacker.

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// signWith fills PrepareQC/Atts from an explicit signer list (twoPhaseSign's variant).
func signWith(b *Block, proposer ed25519.PrivateKey, signers []ed25519.PrivateKey) {
	Sign(b, proposer)
	for _, k := range signers {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
	}
	for _, k := range signers {
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
	}
}

// TestProbeD_NewValidatorJoinsAfterEra3Boundary: the joiner bonds at a v4 height, becomes
// qualified, and its precommit lands in the next block's certificate. Drive both arms:
//
//	D1 — certificate INCLUDES the joiner  -> ?
//	D2 — certificate EXCLUDES the joiner  -> ?
func TestProbeD_NewValidatorJoinsAfterEra3Boundary(t *testing.T) {
	c, keys := era3AnchorChain(t, 2) // heights >=2 are v4
	joiner := key(88001)

	// h1 (v2), h2 (v4), h3 (v4) — plain blocks, anchors only.
	for i := 0; i < 3; i++ {
		b := mintNext(t, c, keys)
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d v%d: %v", b.Height, b.Version, err)
		}
		t.Logf("h=%d v%d validatorsSeen=%d", b.Height, b.Version, len(c.validatorsSeen))
	}

	// h4 (v4): the joiner's bond registration commits. Anchors alone sign it.
	headHash, _ := c.Head()
	reg := bondReg(joiner, twoMiB, headHash)
	b4 := mintNext(t, c, keys, reg)
	if err := c.Append(*b4); err != nil {
		t.Fatalf("append bond-reg block h%d: %v", b4.Height, err)
	}
	t.Logf("h=%d v%d BONDED joiner: bonded=%d qualified(joiner)=%v seen(joiner)=%v validatorsSeen=%d",
		b4.Height, b4.Version, c.bonded[idOf(joiner)], c.attesterQualified(idOf(joiner)),
		c.validatorsSeen[idOf(joiner)], len(c.validatorsSeen))

	// --- D1: the joiner's precommit is in the certificate. ---
	prev, next := c.Head()
	b5 := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra3Roots(b5); err != nil {
		t.Fatalf("roots: %v", err)
	}
	signWith(b5, keys[0], append(append([]ed25519.PrivateKey(nil), keys...), joiner))
	errIncl := c.Append(*b5)
	t.Logf("D1 certificate INCLUDES joiner (Atts=%d): Append err=%v  StateRootMismatch=%v",
		len(b5.Atts), errIncl, errors.Is(errIncl, ErrEra3StateRootMismatch))

	// --- D2: identical height, joiner DROPPED from the certificate. ---
	b5b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra3Roots(b5b); err != nil {
		t.Fatalf("roots: %v", err)
	}
	signWith(b5b, keys[0], keys)
	errExcl := c.Append(*b5b)
	t.Logf("D2 certificate EXCLUDES joiner (Atts=%d): Append err=%v", len(b5b.Atts), errExcl)
	t.Logf("AFTER: validatorsSeen=%d seen(joiner)=%v", len(c.validatorsSeen), c.validatorsSeen[idOf(joiner)])

	// --- D3: can the joiner EVER be seated in era-3? Try 5 more heights including it. ---
	stalls := 0
	for i := 0; i < 5; i++ {
		prev, next := c.Head()
		b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
		if err := c.PopulateEra3Roots(b); err != nil {
			t.Fatalf("roots: %v", err)
		}
		signWith(b, keys[0], append(append([]ed25519.PrivateKey(nil), keys...), joiner))
		if err := c.Append(*b); err != nil {
			stalls++
		}
	}
	t.Logf("D3 five further heights with the joiner in the certificate: %d/5 REJECTED; seen(joiner)=%v",
		stalls, c.validatorsSeen[idOf(joiner)])
}
