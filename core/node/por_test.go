package node

import (
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/por"
)

// TestCtOverheadMatchesGCM pins the constant the auditor uses to recompute a
// full shard's expected leaf count from the layout's plaintext ChunkSize (the
// spot-check tree is over ciphertext = plaintext + GCM tag). If crypto's AEAD
// overhead ever drifts from ctOverhead, the full-shard leaf-count cross-check
// would reject honest shards — so we fail loudly here, at build time, instead of
// in the field.
func TestCtOverheadMatchesGCM(t *testing.T) {
	pt := make([]byte, 1000)
	if _, err := rand.Read(pt); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ct, _, err := crypto.ConvergentEncrypt(pt)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if got := len(ct) - len(pt); got != ctOverhead {
		t.Fatalf("ciphertext overhead is %d, but ctOverhead is %d — the audit's block-count check would reject honest shards", got, ctOverhead)
	}
	// Private mode uses the same AEAD; confirm it agrees.
	var key [crypto.KeySize]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	pct, err := crypto.PrivateEncrypt(key, 0, pt)
	if err != nil {
		t.Fatalf("private encrypt: %v", err)
	}
	if got := len(pct) - len(pt); got != ctOverhead {
		t.Fatalf("private-mode overhead is %d, want %d", got, ctOverhead)
	}
}

// TestShardCommitmentIsKeylessAndReproducible is the wiring's agreement invariant,
// and what replaced a key-agreement one. The publisher commits a shard's spot-check
// root at publish time and an auditor checks openings against it much later; the two
// must arrive at the same root from the bytes alone, with NO key anywhere — which is
// the property that closed the care-link forgery. A storage node computing the root
// over the bytes it holds gets the same value, and that is not a weakness: the root
// is a public commitment, and holding it buys nothing without the bytes it commits.
func TestShardCommitmentIsKeylessAndReproducible(t *testing.T) {
	data := make([]byte, 5000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	published := por.ShardRoot(data, por.SpotLeafBytes)
	audited := por.ShardRoot(data, por.SpotLeafBytes)
	if published != audited {
		t.Fatal("two independent computations of a shard root over the same bytes disagreed — " +
			"a publisher's commitment would not verify at audit time")
	}

	// A holder of the bytes opens the sampled leaves; the auditor verifies with the
	// root and the geometry alone, touching no key and no other state.
	leaves := por.SpotLeaves(len(data), por.SpotLeafBytes)
	seed := [32]byte{0x9c}
	ops, err := por.Open(data, por.SpotLeafBytes, seed, por.SpotSampleCount)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !por.VerifyOpenings(published, leaves, por.SpotLeafBytes, seed, por.SpotSampleCount, ops) {
		t.Fatal("an auditor holding only the committed root rejected an honest holder's openings")
	}

	// AND THE ROOT IS BOUND TO THESE BYTES: one flipped byte moves it, so a
	// commitment cannot be reused across shards.
	altered := append([]byte(nil), data...)
	altered[0] ^= 0xff
	if por.ShardRoot(altered, por.SpotLeafBytes) == published {
		t.Fatal("a one-byte change left the shard root unmoved")
	}
}

// linkLayoutKey returns a fixed layout key for the test — the value both the
// publisher (Handle.LayoutKey) and auditor (CareHandle.LayoutKey) would hold
// for the same file.
func linkLayoutKey(t *testing.T) [32]byte {
	t.Helper()
	return crypto.DeriveKey([32]byte{0x42}, "silt/link/v1/layout")
}
