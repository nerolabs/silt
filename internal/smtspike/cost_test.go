package smtspike

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/pokt-network/smt"
)

// keyAt derives the i-th synthetic registry key. Real byRoot keys are
// H(content) under a field tag, so a 32-byte pseudorandom body is the faithful
// shape.
func keyAt(i int) []byte {
	var ctr [8]byte
	binary.BigEndian.PutUint64(ctr[:], uint64(i))
	body := sha256.Sum256(ctr[:])
	return stateKey(tagByRoot, body[:])
}

// buildTrie fills a fresh trie with n keys and commits once, which is the
// era-3 activation replay and the boot-rebuild shape both: O(state), one pass.
func buildTrie(tb testing.TB, n int) (*smt.SMT, []byte) {
	tb.Helper()
	trie, _ := buildTrieWithStore(tb, n)
	return trie, trie.Root()
}

// buildTrieWithStore is buildTrie plus the backing node store, so the profile
// can size what a disk-backed MapStore would actually have to hold.
func buildTrieWithStore(tb testing.TB, n int) (*smt.SMT, *countingStore) {
	tb.Helper()
	store := newCountingStore()
	trie := smt.NewSparseMerkleTrie(store, sha256.New())
	for i := 0; i < n; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			tb.Fatalf("Update(%d): %v", i, err)
		}
	}
	if err := trie.Commit(); err != nil {
		tb.Fatalf("Commit: %v", err)
	}
	return trie, store
}

// costScales are the registry sizes the keystone must survive. byRoot is the
// "∀ content ever" term (cert §Q3 table), so the top of the sweep is the one
// that decides rebuild-vs-persist (Q6).
var costScales = []int{1_000, 10_000, 100_000, 1_000_000}

// TestFloorBoxProfile is the build-immutable #8 measurement: produce, proof,
// verify, and resident cost at registry scale. It is the gate on the Q6
// rebuild-vs-persist choice and on the dependency itself.
//
// Heavy by design — run it explicitly:
//
//	SILT_SMT_PROFILE=1 go test ./internal/smtspike/ -run TestFloorBoxProfile -v -timeout 60m
//
// Report the numbers from the 1 vCPU / 2 GB floor box, never from a dev laptop:
// a laptop number is the shape, not the gate.
func TestFloorBoxProfile(t *testing.T) {
	if os.Getenv("SILT_SMT_PROFILE") == "" {
		t.Skip("set SILT_SMT_PROFILE=1 to run the floor-box cost profile")
	}

	t.Logf("host: GOARCH=%s GOOS=%s NumCPU=%d", runtime.GOARCH, runtime.GOOS, runtime.NumCPU())
	t.Logf("%-10s %10s %9s %8s %8s %8s %9s %9s %7s",
		"n", "build", "per-key", "heapMB", "nodes/k", "storeB/k", "prove", "verify", "proofB")

	for _, n := range costScales {
		func() {
			var before runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)

			start := time.Now()
			trie, store := buildTrieWithStore(t, n)
			buildNS := time.Since(start).Nanoseconds()
			root := trie.Root()
			nodes := store.Len()

			var after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&after)
			heapMB := float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)

			// Proof production and verification for a present key, averaged
			// over a spread of positions so one lucky shallow path cannot
			// flatter the number.
			const samples = 200
			proveNS, verifyNS, proofBytes := proofCost(t, trie, root, n, samples)

			t.Logf("%-10d %9.2fs %8.1fus %8.1f %8.2f %8.0f %8.1fus %8.1fus %7d",
				n,
				float64(buildNS)/1e9,
				float64(buildNS)/float64(n)/1e3,
				heapMB,
				float64(nodes)/float64(n),
				float64(store.setBytes)/float64(n),
				float64(proveNS)/1e3,
				float64(verifyNS)/1e3,
				proofBytes,
			)
		}()
	}
}

// proofCost returns mean prove ns, mean verify ns, and mean serialized proof
// size over `samples` present keys.
func proofCost(tb testing.TB, trie *smt.SMT, root []byte, n, samples int) (int64, int64, int) {
	tb.Helper()
	step := n / samples
	if step < 1 {
		step = 1
	}
	var proveNS, verifyNS int64
	var totalBytes, count int

	for i := 0; i < n && count < samples; i += step {
		k := keyAt(i)

		s := time.Now()
		proof, err := trie.Prove(k)
		proveNS += time.Since(s).Nanoseconds()
		if err != nil {
			tb.Fatalf("Prove(%d): %v", i, err)
		}

		s = time.Now()
		ok, err := smt.VerifyProof(proof, root, k, present, trie.Spec())
		verifyNS += time.Since(s).Nanoseconds()
		if err != nil || !ok {
			tb.Fatalf("membership verify failed at %d (ok=%v, err=%v)", i, ok, err)
		}

		compact, err := smt.CompactProof(proof, trie.Spec())
		if err != nil {
			tb.Fatalf("CompactProof(%d): %v", i, err)
		}
		bz, err := compact.Marshal()
		if err != nil {
			tb.Fatalf("Marshal(%d): %v", i, err)
		}
		totalBytes += len(bz)
		count++
	}
	if count == 0 {
		tb.Fatal("no samples taken")
	}
	return proveNS / int64(count), verifyNS / int64(count), totalBytes / count
}

// BenchmarkBuild measures the O(state) full build — the era-3 activation
// replay and the Q6 boot rebuild are both this shape.
func BenchmarkBuild(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				buildTrie(b, n)
			}
		})
	}
}

// BenchmarkApplyBlock measures the hot-path cost the cert's Q4 constrains:
// applying one block's changed keys to an existing tree of size n. The claim
// under test is O(changed · log n) — cost must track `changed`, and grow only
// logarithmically in n. A result that tracks n is the #555 scar returning.
func BenchmarkApplyBlock(b *testing.B) {
	const changed = 100
	for _, n := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("n=%d/changed=%d", n, changed), func(b *testing.B) {
			trie, _ := buildTrie(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				base := n + i*changed
				for j := 0; j < changed; j++ {
					if err := trie.Update(keyAt(base+j), present); err != nil {
						b.Fatalf("Update: %v", err)
					}
				}
				if err := trie.Commit(); err != nil {
					b.Fatalf("Commit: %v", err)
				}
			}
		})
	}
}

// BenchmarkProveVerify separates the light-path costs. Proofs are produced
// only on the pruned/sharded/light path (cert Q4), so these gate that path,
// not the full-node hot path.
func BenchmarkProveVerify(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 100_000} {
		trie, root := buildTrie(b, n)
		k := keyAt(n / 2)

		b.Run(fmt.Sprintf("prove/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := trie.Prove(k); err != nil {
					b.Fatalf("Prove: %v", err)
				}
			}
		})

		proof, err := trie.Prove(k)
		if err != nil {
			b.Fatalf("Prove: %v", err)
		}
		b.Run(fmt.Sprintf("verify/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ok, err := smt.VerifyProof(proof, root, k, present, trie.Spec())
				if err != nil || !ok {
					b.Fatalf("verify failed: ok=%v err=%v", ok, err)
				}
			}
		})
	}
}
