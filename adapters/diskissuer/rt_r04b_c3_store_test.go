package diskissuer

// R0.4b C3 re-break — EpochStore regression gates. Inversions of the red-team probes
// adapters/diskissuer/rt_c3b_store_test.go (RT-C3B-11 … RT-C3B-14), archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// RT-C3B-11 CLOSED, twice over.
//
// Mechanism 1 (memory-level lost update, closed by the EpochStore mutex). The daemon
// launches every epoch turn as a bare `go rotateDemandKeys(cur)` and EnsureBand is a
// read-modify-write over ONE file, so two overlapping turns both Loaded, both generated,
// and the later Save CLOBBERED the earlier.
//
// Mechanism 2 (F6, band-level prune, 2026-09-03 — the Tester found this one still live
// after the mutex landed). ensureBand pruned on the CALLER'S OWN band, `e > genTo` with
// genTo = cur+w, so a rotation for an EARLIER epoch deleted the pre-published keys a
// LATER rotation had already generated AND staged for on-chain commitment. Measured with
// no concurrency at all: RotateWindow(11) to completion, then RotateWindow(10), loses
// epoch 15. The fix makes the retained UPPER EDGE monotone — max(genTo, highest epoch
// already on disk) — so no rotation can ever shrink another's pre-publication.
//
// The race detector never catches either mechanism (each goroutine's Load builds its own
// map, so it is a file-level lost update, not a memory race), which is why these gates
// assert the OUTCOME — every installed key survives — rather than relying on -race.
// ---------------------------------------------------------------------------

// TestRTC3_AnEarlierRotationDoesNotPruneALaterRotationsPrePublishedKey is the F6
// mechanism stated deterministically, with no goroutines: the sequence the Tester
// measured. Ablate the maxEpoch() term in ensureBand's prune and this is RED with
// "LOST=[15]".
func TestRTC3_AnEarlierRotationDoesNotPruneALaterRotationsPrePublishedKey(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	installed := map[uint64]*rsa.PrivateKey{}
	record := func(e uint64, k *rsa.PrivateKey) { installed[e] = k }

	// The LATER turn completes first and pre-publishes [11, 15].
	if err := s.RotateWindow(rand.Reader, 11, 4, record); err != nil {
		t.Fatal(err)
	}
	// Then the EARLIER turn runs. Its own band is [6, 14]; epoch 15 is above its upper
	// edge but was already staged for commitment by the turn above.
	if err := s.RotateWindow(rand.Reader, 10, 4, record); err != nil {
		t.Fatal(err)
	}

	onDisk, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertEveryInstalledKeySurvived(t, installed, onDisk, "rotate(11) then rotate(10)")
}

// TestRTC3_EpochStoreRotationsNeverLoseAnInstalledKey drives the daemon's own launch
// pattern — four overlapping `go rotateDemandKeys(cur)` turns — many times over.
//
// NO EPOCH IS EXEMPTED FROM THE ASSERTION, deliberately. The adopted 2026-09-03 gate
// carried an `if e < 9 || e > 14 { continue }` filter rationalised as "a later turn's
// band may legitimately have pruned it"; the Tester showed the pruning turn is the
// EARLIER one and that the filter carved out the only epoch that was ever lost (17),
// 5 runs out of 5. See [[scar-adopted-probe-narrows-the-assertion]].
//
// The unfiltered assertion is sound here, and here is the arithmetic so nobody
// reintroduces a filter: generation starts at genFrom = cur, so the LOWEST epoch any
// turn can create is min(cur) = 10, while the HIGHEST lower prune edge any turn uses is
// max(cur) - w = 13 - 4 = 9. 9 < 10, so the legitimate lower-edge drain cannot reach a
// key this topology installed. Every absence is a defect.
func TestRTC3_EpochStoreRotationsNeverLoseAnInstalledKey(t *testing.T) {
	// 80 is the count at which the Tester observed the F6 loss on 80/80 runs. It costs
	// ~40s of RSA keygen, so -short takes a smaller sample; the deterministic gate above
	// carries the mechanism when this one is sampled down.
	attempts := 80
	if testing.Short() {
		attempts = 5
	}
	for attempt := 0; attempt < attempts; attempt++ {
		dir := t.TempDir()
		s, err := OpenEpochs(dir)
		if err != nil {
			t.Fatal(err)
		}
		installed := map[uint64]*rsa.PrivateKey{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, cur := range []uint64{10, 11, 12, 13} {
			wg.Add(1)
			go func(c uint64) {
				defer wg.Done()
				if err := s.RotateWindow(rand.Reader, c, 4, func(e uint64, k *rsa.PrivateKey) {
					mu.Lock()
					installed[e] = k
					mu.Unlock()
				}); err != nil {
					t.Error(err)
				}
			}(cur)
		}
		wg.Wait()

		onDisk, err := s.Load()
		if err != nil {
			t.Fatalf("concurrent rotations left an UNLOADABLE store: %v", err)
		}
		mu.Lock()
		assertEveryInstalledKeySurvived(t, installed, onDisk,
			fmt.Sprintf("four overlapping turns {10,11,12,13}, attempt %d/%d", attempt, attempts))
		mu.Unlock()
	}
}

// assertEveryInstalledKeySurvived is the shared consequence statement: a key handed to
// install() has had its fingerprint staged via Node.SetDemandIssuerKey, and
// applyIssuerKeys is first-write-wins, so losing it is a PERMANENT dead epoch.
func assertEveryInstalledKeySurvived(t *testing.T, installed, onDisk map[uint64]*rsa.PrivateKey, topology string) {
	t.Helper()
	var lost []uint64
	for e, k := range installed {
		d := onDisk[e]
		if d == nil {
			lost = append(lost, e)
			continue
		}
		if d.N.Cmp(k.N) != 0 {
			t.Fatalf("epoch %d: the store holds a DIFFERENT key from the one installed "+
				"and staged for commitment — a lost update (%s)", e, topology)
		}
	}
	if len(lost) != 0 {
		sort.Slice(lost, func(i, j int) bool { return lost[i] < lost[j] })
		t.Fatalf("BREAK RT-C3B-11/F6 REOPENED (%s): epochs %v were installed and staged "+
			"for on-chain commitment but are ABSENT from the store (installed %d, on disk %d). "+
			"applyIssuerKeys is first-write-wins, so once the staged fingerprint commits the "+
			"regenerated key can NEVER be registered: the lane is dead for that epoch, "+
			"permanently, and nothing detects it.", topology, lost, len(installed), len(onDisk))
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
