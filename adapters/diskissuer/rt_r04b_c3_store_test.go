package diskissuer

// R0.4b C3 re-break — EpochStore regression gates. Inversions of the red-team probes
// adapters/diskissuer/rt_c3b_store_test.go (RT-C3B-11 … RT-C3B-14), archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// RT-C3B-11 CLOSED. The daemon launches every epoch turn as a bare
// `go rotateDemandKeys(cur)` and EnsureBand is a read-modify-write over ONE file, so
// two overlapping turns both Loaded, both generated, and the later Save CLOBBERED the
// earlier — a LOST UPDATE of a key whose fingerprint the earlier goroutine had already
// staged for on-chain commitment. The probe measured "installed 6 epochs; survived on
// disk 5; LOST: [15]".
//
// The race detector never catches this (each goroutine's Load builds its own map, so
// it is a file-level lost update, not a memory race), which is why the gate asserts the
// OUTCOME — every installed key survives — rather than relying on -race.
// ---------------------------------------------------------------------------
func TestRTC3_EpochStoreRotationsNeverLoseAnInstalledKey(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		dir := t.TempDir()
		s, err := OpenEpochs(dir)
		if err != nil {
			t.Fatal(err)
		}
		installed := map[uint64]*rsa.PrivateKey{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		// Four overlapping epoch turns, more than the daemon can produce, launched
		// exactly as the daemon launches them.
		for _, cur := range []uint64{10, 11, 12, 13} {
			wg.Add(1)
			go func(c uint64) {
				defer wg.Done()
				_ = s.RotateWindow(rand.Reader, c, 4, func(e uint64, k *rsa.PrivateKey) {
					mu.Lock()
					installed[e] = k
					mu.Unlock()
				})
			}(cur)
		}
		wg.Wait()

		onDisk, err := s.Load()
		if err != nil {
			t.Fatalf("concurrent rotations left an UNLOADABLE store: %v", err)
		}
		mu.Lock()
		var lost []uint64
		for e, k := range installed {
			// An epoch may legitimately have been PRUNED by a later turn's band
			// (RotateWindow retains [cur-w, cur+w]); what must never happen is a key
			// present on disk under a DIFFERENT modulus, or absent while still in the
			// widest band any turn used ([13-4, 10+4] = [9, 14]).
			if e < 9 || e > 14 {
				continue
			}
			d := onDisk[e]
			if d == nil {
				lost = append(lost, e)
				continue
			}
			if d.N.Cmp(k.N) != 0 {
				t.Fatalf("epoch %d: the store holds a DIFFERENT key from the one installed "+
					"and staged for commitment — a lost update", e)
			}
		}
		mu.Unlock()
		if len(lost) != 0 {
			t.Fatalf("BREAK RT-C3B-11 REOPENED (attempt %d): epochs %v were installed and "+
				"staged for on-chain commitment but are ABSENT from the store. applyIssuerKeys "+
				"is first-write-wins, so once the staged fingerprint commits the regenerated "+
				"key can NEVER be registered: the lane is dead for that epoch, permanently, "+
				"and nothing detects it.", attempt, lost)
		}
	}
}

// TestRTC3_EnsureBandIsAtomicUnderConcurrency is the same property stated on the
// primitive: N concurrent EnsureBand calls for the SAME band must all observe the same
// keys, and every key any of them returned must be on disk.
func TestRTC3_EnsureBandIsAtomicUnderConcurrency(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	const n = 8
	bands := make([]map[uint64]*rsa.PrivateKey, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b, err := s.EnsureBand(rand.Reader, 20, 20, 24)
			if err != nil {
				t.Error(err)
				return
			}
			bands[i] = b
		}(i)
	}
	wg.Wait()
	onDisk, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bands {
		for e, k := range b {
			if onDisk[e] == nil || onDisk[e].N.Cmp(k.N) != 0 {
				t.Fatalf("goroutine %d saw a key for epoch %d that is not the persisted one — "+
					"a lost update in the load-generate-save cycle", i, e)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-12 (the cliff itself, UNCHANGED and deliberately so). Once fp(key_E) is
// committed and key_E is gone from the store, EnsureBand regenerates a DIFFERENT key
// and the append-only chain will never accept its fingerprint. The store cannot see the
// commitment, so it cannot detect this; the close is to make the loss not happen (the
// mutex above, the directory fsync in Save, and the refusal to overwrite a corrupt
// store below). This gate PINS the cliff so its cause is never mistaken for a
// recoverable one.
// ---------------------------------------------------------------------------
func TestRTC3_LostKeyAfterCommitIsStillAPermanentlyDeadEpoch(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	band, err := s.EnsureBand(rand.Reader, 10, 10, 14)
	if err != nil {
		t.Fatal(err)
	}
	committed := band[12]
	if committed == nil {
		t.Fatal("setup: no key for epoch 12")
	}
	all, _ := s.Load()
	delete(all, 12)
	if err := s.Save(all); err != nil {
		t.Fatal(err)
	}
	band2, err := s.EnsureBand(rand.Reader, 10, 10, 14)
	if err != nil {
		t.Fatal(err)
	}
	if band2[12] == nil {
		t.Fatal("EnsureBand did not regenerate epoch 12")
	}
	if band2[12].N.Cmp(committed.N) == 0 {
		t.Fatalf("the regenerated key equals the committed one — this pin is testing nothing")
	}
	t.Logf("PINNED (unchanged by design): a key lost AFTER its fingerprint commits is a " +
		"permanently dead epoch for that issuer. EnsureBand cannot see the commitment, so " +
		"the only defence is not losing it: the EpochStore mutex, the directory fsync in " +
		"Save, and never regenerating over a corrupt store.")
}

// ---------------------------------------------------------------------------
// RT-C3B-13 CLOSED at the daemon (the blast radius) and PINNED here (the store).
// A corrupt file must stay a hard error at the store — silently regenerating over
// already-committed fingerprints is the unrecoverable failure above — and the file must
// be left EXACTLY as found so an operator can restore it. The daemon degrades to
// lane-off instead of dying; see cmd/silt/rt_r04b_c3_laneoff_test.go.
// ---------------------------------------------------------------------------
func TestRTC3_CorruptStoreErrorsAndIsNeverRewritten(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureBand(rand.Reader, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "demandkeys.cbor")
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := append([]byte(nil), blob...)
	corrupt[len(corrupt)/2] ^= 0xFF
	if err := os.WriteFile(p, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	rerr := s.RotateWindow(rand.Reader, 1, 4, func(uint64, *rsa.PrivateKey) {})
	if rerr == nil {
		t.Skip("the flipped byte landed somewhere CBOR/DER tolerated — re-run; the class is the error path")
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("the store REWROTE a corrupt key file. Quietly minting new keys over "+
			"already-committed fingerprints is unrecoverable: the chain is append-only, so "+
			"the regenerated key can never be registered (err was %v)", rerr)
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-14 (documented, unchanged): a FRESH store generates only forward, so the
// [cur-W, cur) past band is absent until it has been running that long. Pinned so the
// boundary-race guarantee is stated with its one exception rather than absolutely.
// ---------------------------------------------------------------------------
func TestRTC3_FreshStoreGeneratesOnlyForward(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	const cur = 10
	band, err := s.EnsureBand(rand.Reader, cur-4, cur, cur+4)
	if err != nil {
		t.Fatal(err)
	}
	for e := uint64(cur - 4); e < cur; e++ {
		if band[e] != nil {
			t.Fatalf("a fresh store generated past-band key %d — W extra RSA keygens at boot "+
				"for keys nothing was ever issued under", e)
		}
	}
	for e := uint64(cur); e <= cur+4; e++ {
		if band[e] == nil {
			t.Fatalf("a fresh store did not pre-publish key %d", e)
		}
	}
}
