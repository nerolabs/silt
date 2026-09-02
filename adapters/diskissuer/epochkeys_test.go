package diskissuer

// R0.4b C3 close — the per-epoch demand key store. The properties that matter are
// RESTART (a regenerated band would be un-committable, because the on-chain binding
// is append-only and backdating is rejected — so the lane would be dead for W epochs)
// and PRUNING (the band must not grow without bound; build-immutable #8).

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"testing"
)

// smallBand keeps these tests cheap: RSA-2048 keygen dominates, so every case uses
// the narrowest band that still exercises the property.
const smallBand = uint64(1)

func TestEpochBandRoundTripsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s1, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s1.EnsureBand(rand.Reader, 0, 0, smallBand)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != int(smallBand)+1 {
		t.Fatalf("band has %d keys, want %d", len(first), smallBand+1)
	}

	// A fresh Store on the same dir — a restart — must RELOAD, not regenerate. The
	// fingerprints of these keys are already committed on-chain for their epochs, and
	// the binding is first-write-wins: a regenerated key can never be registered.
	s2, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s2.EnsureBand(rand.Reader, 0, 0, smallBand)
	if err != nil {
		t.Fatal(err)
	}
	for e, k := range first {
		got := second[e]
		if got == nil {
			t.Fatalf("epoch %d vanished across the restart", e)
		}
		if got.N.Cmp(k.N) != 0 || got.D.Cmp(k.D) != 0 || got.PublicKey.E != k.PublicKey.E {
			t.Fatalf("epoch %d came back as a DIFFERENT key — its committed fingerprint "+
				"can never be re-registered (append-only, no backdating), so the lane dies "+
				"for the whole window", e)
		}
	}
}

func TestEpochBandPrunesOutsideTheRetainedRange(t *testing.T) {
	s, err := OpenEpochs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureBand(rand.Reader, 0, 0, 2); err != nil { // epochs 0,1,2
		t.Fatal(err)
	}
	// Advance: retain from 2, generate 2..3. Epochs 0 and 1 must be dropped.
	band, err := s.EnsureBand(rand.Reader, 2, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(band) != 2 || band[2] == nil || band[3] == nil {
		t.Fatalf("band = %v epochs, want exactly {2,3}", len(band))
	}
	for _, gone := range []uint64{0, 1} {
		if band[gone] != nil {
			t.Fatalf("epoch %d survived the prune — the band would grow without bound", gone)
		}
	}
	// The prune is DURABLE, not just in-memory.
	reloaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 2 {
		t.Fatalf("on disk the band holds %d keys after the prune, want 2", len(reloaded))
	}
}

// TestRotateWindowUsesTheDemandWindow pins the band arithmetic the daemon relies on:
// retain back to cur−w, pre-publish forward to cur+w. Pre-publishing is what makes a
// withdrawal at an epoch boundary find key_E already committed; retaining backwards
// is what lets an in-window past epoch still be signed (the boundary race).
func TestRotateWindowUsesTheDemandWindow(t *testing.T) {
	s, err := OpenEpochs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const w = uint64(2)
	got := map[uint64]bool{}
	if err := s.RotateWindow(rand.Reader, 0, w, func(e uint64, _ *rsa.PrivateKey) { got[e] = true }); err != nil {
		t.Fatal(err)
	}
	for e := uint64(0); e <= w; e++ {
		if !got[e] {
			t.Fatalf("epoch 0 rotation did not pre-publish key_%d", e)
		}
	}
	// One epoch on: the band slides, the old key is RETAINED (in window), and the new
	// far end is generated.
	got = map[uint64]bool{}
	if err := s.RotateWindow(rand.Reader, 3, w, func(e uint64, _ *rsa.PrivateKey) { got[e] = true }); err != nil {
		t.Fatal(err)
	}
	for e := uint64(1); e <= 5; e++ {
		if !got[e] {
			t.Fatalf("at epoch 3 with w=%d the band is missing key_%d — [cur−w, cur+w] "+
				"is the whole point: forward for pre-publication, backward for the "+
				"epoch-boundary race", w, e)
		}
	}
	if got[0] {
		t.Fatal("key_0 is out of the retained window at epoch 3 and must have been pruned")
	}
}

// A corrupt store is a real error, never a silent regeneration: quietly minting new
// keys over committed fingerprints is unrecoverable, while refusing to start is not.
func TestEpochStoreCorruptIsAnError(t *testing.T) {
	s, err := OpenEpochs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureBand(rand.Reader, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path, []byte("not a key store"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("a corrupt demand key store loaded cleanly — the daemon would mint fresh " +
			"keys over already-committed fingerprints")
	}
}

// No store yet is an empty band, not an error, so a first run generates one.
func TestEpochStoreAbsentIsEmpty(t *testing.T) {
	s, err := OpenEpochs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keys, err := s.Load()
	if err != nil || len(keys) != 0 {
		t.Fatalf("absent store: %d keys, err %v", len(keys), err)
	}
}
