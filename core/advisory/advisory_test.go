package advisory

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func keys(n int) ([]ed25519.PrivateKey, [][]byte) {
	var privs []ed25519.PrivateKey
	var pubs [][]byte
	for i := 0; i < n; i++ {
		seed := make([]byte, ed25519.SeedSize)
		seed[0] = byte(i + 1)
		p := ed25519.NewKeyFromSeed(seed)
		privs = append(privs, p)
		pubs = append(pubs, p.Public().(ed25519.PublicKey))
	}
	return privs, pubs
}

// TestOnlyAThresholdOfDistinctMaintainersDeclaresAFloor: R4's "no single key may declare it",
// and item 19's "no advisory below the signing threshold changes anything" — at the type that
// every later rule rests on. Below the threshold, a repeated signer, a stranger's key, another
// network's signature, and a threshold of one all fail; the threshold of distinct maintainers passes.
func TestOnlyAThresholdOfDistinctMaintainersDeclaresAFloor(t *testing.T) {
	privs, pubs := keys(3)
	stranger := ed25519.NewKeyFromSeed(append([]byte{99}, make([]byte, 31)...))
	chainID := ports.HashBytes([]byte("net"))
	a := Signed{Advisory: Advisory{Seq: 1, Floor: 2, Tier: TierEnforce, GraceBlocks: 10, Note: "upgrade to release 2"}}
	sign := func(s Signed, ps ...ed25519.PrivateKey) Signed {
		for _, p := range ps {
			var err error
			if s, err = Sign(s, p, chainID); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}

	if err := Verify(sign(a, privs[0], privs[1]), chainID, pubs, 2); err != nil {
		t.Fatalf("two of three maintainers must declare a floor at threshold 2: %v", err)
	}
	for name, tc := range map[string]struct {
		s         Signed
		chainID   ports.Hash
		threshold int
	}{
		"one maintainer":                  {sign(a, privs[0]), chainID, 2},
		"one maintainer signing twice":    {sign(a, privs[0], privs[0]), chainID, 2},
		"a maintainer and a stranger":     {sign(a, privs[0], stranger), chainID, 2},
		"signed for another network":      {sign(a, privs[0], privs[1]), ports.HashBytes([]byte("other")), 2},
		"a threshold of one":              {sign(a, privs[0], privs[1]), chainID, 1},
		"a threshold above the key count": {sign(a, privs[0], privs[1], privs[2]), chainID, 4},
	} {
		if err := Verify(tc.s, tc.chainID, pubs, tc.threshold); !errors.Is(err, ErrBelowThreshold) {
			t.Errorf("%s: want ErrBelowThreshold, got %v", name, err)
		}
	}

	// A signature does not survive a change to what it signed.
	tampered := sign(a, privs[0], privs[1])
	tampered.Advisory.Floor = 3
	if err := Verify(tampered, chainID, pubs, 2); !errors.Is(err, ErrBelowThreshold) {
		t.Fatalf("a floor changed after signing must not verify, got %v", err)
	}
}

// TestAdvisoryBoundsAndTheHeightClock: the malformed shapes are refused before any signature is
// read, and enforcement starts at committed height + grace, by the chain's clock.
func TestAdvisoryBoundsAndTheHeightClock(t *testing.T) {
	for name, a := range map[string]Advisory{
		"sequence 0":          {Seq: 0, Floor: 2, Tier: TierNotice},
		"unknown tier":        {Seq: 1, Floor: 2, Tier: 9},
		"recall with a floor": {Seq: 1, Floor: 2, Tier: TierRecall},
		"floor of 0":          {Seq: 1, Floor: 0, Tier: TierEnforce},
		"unbounded grace":     {Seq: 1, Floor: 2, Tier: TierEnforce, GraceBlocks: MaxGraceBlocks + 1},
		"note over the bound": {Seq: 1, Floor: 2, Tier: TierNotice, Note: string(make([]byte, MaxNoteBytes+1))},
	} {
		if err := a.Validate(); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: want ErrMalformed, got %v", name, err)
		}
	}
	if err := (Advisory{Seq: 2, Tier: TierRecall}).Validate(); err != nil {
		t.Fatalf("a recall is well formed: %v", err)
	}
	e := Advisory{Seq: 1, Floor: 2, Tier: TierEnforce, GraceBlocks: 10}
	if Enforcing(e, 100, 109) || !Enforcing(e, 100, 110) {
		t.Fatal("enforcement must start exactly at committed height + grace")
	}
	if Enforcing(Advisory{Seq: 1, Floor: 2, Tier: TierUrgent}, 0, 1<<30) {
		t.Fatal("only the enforce tier enforces")
	}
}
