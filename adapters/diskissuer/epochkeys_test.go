package diskissuer

// R0.4b C3 close — the per-epoch demand key store. The properties that matter are
// RESTART (a regenerated band would be un-committable, because the on-chain binding
// is append-only and backdating is rejected — so the lane would be dead for W epochs)
// and PRUNING (the band must not grow without bound; build-immutable #8).

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
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

// TestEpochStoreSurvivesACrashBeforeRename pins the ATOMICITY of Save, which until
// now was asserted by inspection only (Tester finding, 2026-09-03: zero tests
// referenced demandkeys.cbor or the temp name).
//
// Why it matters more here than for an ordinary cache: a half-written band is
// UNRECOVERABLE. The fingerprints of the keys already on disk are committed on-chain,
// first-write-wins, and backdating is rejected — so a store that comes back partially
// written cannot be repaired by regenerating, and the demand lane is dead for the
// whole window. The committed band must survive any failed write, not just a clean one.
//
// Two failure shapes, both real:
//
//   - CRASH BETWEEN CreateTemp AND Rename. The artifact is a stale .tmp-demandkeys-*
//     file beside an untouched demandkeys.cbor. The store must load the OLD band
//     byte-identically and must not be confused by the leftover. This also pins the
//     one-file design (§3.3): a Load that scanned the directory instead of reading one
//     fixed path would parse this garbage.
//   - A Save THAT CANNOT WRITE. Ablation: replace temp+rename with a direct
//     os.WriteFile(s.path, ...) and the second subtest goes RED — a direct write to an
//     existing 0600 file succeeds in a read-only directory, so the committed band is
//     destroyed and replaced. temp+rename fails at CreateTemp instead, leaving the
//     committed band intact.
func TestEpochStoreSurvivesACrashBeforeRename(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := s.EnsureBand(rand.Reader, 0, 0, smallBand)
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(dir, "demandkeys.cbor")
	before, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("the band was not persisted at %s: %v", storePath, err)
	}

	sameBand := func(t *testing.T, got map[uint64]*rsa.PrivateKey) {
		t.Helper()
		if len(got) != len(committed) {
			t.Fatalf("band came back with %d keys, want the committed %d", len(got), len(committed))
		}
		for e, k := range committed {
			g := got[e]
			if g == nil {
				t.Fatalf("epoch %d vanished — its committed fingerprint can never be re-registered", e)
			}
			if g.N.Cmp(k.N) != 0 || g.D.Cmp(k.D) != 0 || g.PublicKey.E != k.PublicKey.E {
				t.Fatalf("epoch %d came back as a DIFFERENT key — the committed binding is "+
					"append-only, so the lane is dead for the whole window", e)
			}
		}
	}

	t.Run("stale temp file from a crash before rename", func(t *testing.T) {
		// Exactly what a crash between CreateTemp and Rename leaves: a temp file with
		// the store's own prefix, holding a partial (here: unparsable) band.
		tmp, terr := os.CreateTemp(dir, ".tmp-demandkeys-*")
		if terr != nil {
			t.Fatal(terr)
		}
		if _, werr := tmp.Write(before[:len(before)/2]); werr != nil {
			t.Fatal(werr)
		}
		tmp.Close()

		after, rerr := os.ReadFile(storePath)
		if rerr != nil {
			t.Fatalf("demandkeys.cbor must survive the crash: %v", rerr)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("demandkeys.cbor changed while a write was in flight — the write is not atomic")
		}
		s2, oerr := OpenEpochs(dir)
		if oerr != nil {
			t.Fatal(oerr)
		}
		loaded, lerr := s2.Load()
		if lerr != nil {
			t.Fatalf("the committed band must still load past a stale temp file: %v", lerr)
		}
		sameBand(t, loaded)
		// And the restart path itself: EnsureBand must reload, never regenerate.
		reloaded, eerr := s2.EnsureBand(rand.Reader, 0, 0, smallBand)
		if eerr != nil {
			t.Fatal(eerr)
		}
		sameBand(t, reloaded)
		os.Remove(tmp.Name())
	})

	t.Run("a Save that cannot write leaves the committed band intact", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions, so the failure cannot be induced")
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0o700)

		next := map[uint64]*rsa.PrivateKey{}
		for e, k := range committed {
			next[e] = k
		}
		fresh, gerr := generateKey(rand.Reader)
		if gerr != nil {
			t.Fatal(gerr)
		}
		next[smallBand+1] = fresh
		if serr := s.Save(next); serr == nil {
			t.Fatal("Save into a read-only directory reported success — a direct write would, " +
				"temp+rename must not")
		}
		after, rerr := os.ReadFile(storePath)
		if rerr != nil {
			t.Fatalf("a failed Save destroyed the committed band: %v", rerr)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("a failed Save REWROTE demandkeys.cbor — the committed fingerprints on disk " +
				"no longer match the chain, and the binding is append-only")
		}
		loaded, lerr := s.Load()
		if lerr != nil {
			t.Fatalf("the committed band must still load after a failed Save: %v", lerr)
		}
		sameBand(t, loaded)
	})
}
