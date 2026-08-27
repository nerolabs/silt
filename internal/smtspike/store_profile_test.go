package smtspike

import (
	"crypto/sha256"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pokt-network/smt"
)

// RED home for the PE-ordered node-store measurement (RULING keystone-node-store
// Q1): the disk-backed store must be measured on the 1 vCPU / 2 GB floor box —
// boot rebuild, hot-path apply, resident memory, and on-disk size — before the
// dependency is committed. This harness is that measurement; it runs a real
// disk store rather than the in-memory reference so the numbers reflect the
// configuration that has to ship.
//
// Heavy and disk-bound — run explicitly:
//
//	SILT_STORE_PROFILE=1 go test ./internal/smtspike/ -run TestStoreProfile -v -timeout 60m
//
// Report from the floor box, never a laptop: the whole point is the box that
// OOM-killed the in-memory backend (PR #596). A laptop number is the shape.
func TestStoreProfile(t *testing.T) {
	if os.Getenv("SILT_STORE_PROFILE") == "" {
		t.Skip("set SILT_STORE_PROFILE=1 to run the disk-store floor-box profile")
	}

	scales := parseScales(t) // reuses SILT_SMT_SCALES; default costScales
	const perBlock = 100
	noSync := os.Getenv("SILT_STORE_NOSYNC") != "" // measure with and without fsync

	t.Logf("host: GOARCH=%s GOOS=%s NumCPU=%d fsync=%v",
		runtime.GOARCH, runtime.GOOS, runtime.NumCPU(), !noSync)
	t.Logf("%-10s %10s %10s %9s %8s %10s %11s %10s",
		"n", "build", "per-key", "heapMB", "rssMB", "onDiskMB", "apply100", "reopen")

	for _, n := range scales {
		func() {
			dir := t.TempDir()

			var before runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)

			bs, err := newBoltStore(dir, noSync)
			if err != nil {
				t.Fatalf("newBoltStore: %v", err)
			}
			trie := smt.NewSparseMerkleTrie(bs, sha256.New())

			// Build in per-block batches, flushing once per block — the real
			// commit cadence, and the shape whose cost Q6's rebuild number needs.
			start := time.Now()
			for i := 0; i < n; i++ {
				if err := trie.Update(keyAt(i), present); err != nil {
					t.Fatalf("Update(%d): %v", i, err)
				}
				if (i+1)%perBlock == 0 {
					if err := trie.Commit(); err != nil {
						t.Fatalf("Commit: %v", err)
					}
					if err := bs.Flush(); err != nil {
						t.Fatalf("Flush: %v", err)
					}
				}
			}
			if err := trie.Commit(); err != nil {
				t.Fatalf("final Commit: %v", err)
			}
			if err := bs.Flush(); err != nil {
				t.Fatalf("final Flush: %v", err)
			}
			buildNS := time.Since(start).Nanoseconds()

			var after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&after)
			heapMB := float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)
			rssMB := residentMB() // includes bbolt's mmap; 0 off-Linux

			onDisk, err := bs.onDiskBytes()
			if err != nil {
				t.Fatalf("onDiskBytes: %v", err)
			}

			// Hot path: one block of 100 changed keys against the full tree.
			as := time.Now()
			for j := 0; j < perBlock; j++ {
				if err := trie.Update(keyAt(n+j), present); err != nil {
					t.Fatalf("apply Update: %v", err)
				}
			}
			if err := trie.Commit(); err != nil {
				t.Fatalf("apply Commit: %v", err)
			}
			if err := bs.Flush(); err != nil {
				t.Fatalf("apply Flush: %v", err)
			}
			applyMS := float64(time.Since(as).Nanoseconds()) / 1e6

			// Capture the root AFTER the apply: Commit orphans (deletes) the
			// nodes the apply replaced, so only the current root's nodes remain
			// on disk. Reopening at a stale root would legitimately miss them.
			root := append([]byte(nil), trie.Root()...)

			// Boot rebuild via reopen: close, reopen, Import at the persisted
			// root, prove one key came back from disk. This is the Q6 number —
			// the reopen cost, not a full replay.
			if err := bs.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			rs := time.Now()
			bs2, err := newBoltStore(dir, noSync)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			reopened := smt.ImportSparseMerkleTrie(bs2, sha256.New(), root)
			k := keyAt(n / 2)
			proof, err := reopened.Prove(k)
			if err != nil {
				t.Fatalf("Prove after reopen: %v", err)
			}
			if ok, err := smt.VerifyProof(proof, root, k, present, reopened.Spec()); err != nil || !ok {
				t.Fatalf("reopen proof failed (ok=%v, err=%v)", ok, err)
			}
			reopenMS := float64(time.Since(rs).Nanoseconds()) / 1e6
			bs2.Close()

			t.Logf("%-10d %9.2fs %8.1fus %8.1f %7.1f %9.1f %9.2fms %8.2fms",
				n,
				float64(buildNS)/1e9,
				float64(buildNS)/float64(n)/1e3,
				heapMB,
				rssMB,
				float64(onDisk)/(1<<20),
				applyMS,
				reopenMS,
			)
		}()
	}
}

// parseScales reuses SILT_SMT_SCALES (per-scale isolation so an OOM at the top
// scale cannot destroy lower results), falling back to costScales.
func parseScales(t *testing.T) []int {
	raw := os.Getenv("SILT_SMT_SCALES")
	if raw == "" {
		return costScales
	}
	var out []int
	for _, f := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			t.Fatalf("SILT_SMT_SCALES: %q: %v", f, err)
		}
		out = append(out, n)
	}
	return out
}
