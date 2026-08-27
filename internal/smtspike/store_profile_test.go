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
	sync := os.Getenv("SILT_STORE_NOSYNC") == "" // fsync on by default; off for the fast path

	backends := []string{"bbolt", "pebble"}
	if b := os.Getenv("SILT_STORE_BACKEND"); b != "" {
		backends = []string{b}
	}

	t.Logf("host: GOARCH=%s GOOS=%s NumCPU=%d fsync=%v",
		runtime.GOARCH, runtime.GOOS, runtime.NumCPU(), sync)
	t.Logf("%-8s %-10s %10s %10s %9s %8s %10s %11s %10s",
		"backend", "n", "build", "per-key", "heapMB", "rssMB", "onDiskMB", "apply100", "reopen")

	for _, backend := range backends {
		for _, n := range scales {
			runOneStoreProfile(t, backend, n, perBlock, sync)
		}
	}
}

// store is the batching contract both adapters share: MapStore plus a per-block
// Flush and a size/close/reopen surface for the profile.
type store interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Len() int
	ClearAll() error
	FlushBlock(sync bool) error
	DiskBytes() (int64, error)
	Close() error
}

// boltStore/pebbleStore already satisfy MapStore; these thin wrappers unify the
// Flush signature (bbolt's Flush takes no arg; pebble's takes sync) and the
// on-disk accessor (pebble needs the dir).
type boltAdapter struct{ *boltStore }

func (b boltAdapter) FlushBlock(bool) error     { return b.Flush() }
func (b boltAdapter) DiskBytes() (int64, error) { return b.onDiskBytes() }

type pebbleAdapter struct {
	*pebbleStore
	dir string
}

func (p pebbleAdapter) FlushBlock(sync bool) error { return p.Flush(sync) }
func (p pebbleAdapter) DiskBytes() (int64, error)  { return p.onDiskBytes(p.dir) }

func openStore(t *testing.T, backend, dir string, sync bool) store {
	t.Helper()
	switch backend {
	case "bbolt":
		bs, err := newBoltStore(dir, !sync)
		if err != nil {
			t.Fatalf("newBoltStore: %v", err)
		}
		return boltAdapter{bs}
	case "pebble":
		ps, err := newPebbleStore(dir, sync)
		if err != nil {
			t.Fatalf("newPebbleStore: %v", err)
		}
		return pebbleAdapter{ps, dir}
	default:
		t.Fatalf("unknown backend %q", backend)
		return nil
	}
}

func runOneStoreProfile(t *testing.T, backend string, n, perBlock int, sync bool) {
	t.Helper()
	dir := t.TempDir()

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	st := openStore(t, backend, dir, sync)
	trie := smt.NewSparseMerkleTrie(st, sha256.New())

	start := time.Now()
	for i := 0; i < n; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update(%d): %v", i, err)
		}
		if (i+1)%perBlock == 0 {
			if err := trie.Commit(); err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if err := st.FlushBlock(sync); err != nil {
				t.Fatalf("Flush: %v", err)
			}
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("final Commit: %v", err)
	}
	if err := st.FlushBlock(sync); err != nil {
		t.Fatalf("final Flush: %v", err)
	}
	buildNS := time.Since(start).Nanoseconds()

	var after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&after)
	// Signed delta: after GC, HeapAlloc can be below the baseline, and an
	// unsigned subtraction would underflow into a garbage number.
	heapMB := (float64(after.HeapAlloc) - float64(before.HeapAlloc)) / (1 << 20)
	rssMB := residentMB()

	onDisk, err := st.DiskBytes()
	if err != nil {
		t.Fatalf("DiskBytes: %v", err)
	}

	as := time.Now()
	for j := 0; j < perBlock; j++ {
		if err := trie.Update(keyAt(n+j), present); err != nil {
			t.Fatalf("apply Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("apply Commit: %v", err)
	}
	if err := st.FlushBlock(sync); err != nil {
		t.Fatalf("apply Flush: %v", err)
	}
	applyMS := float64(time.Since(as).Nanoseconds()) / 1e6

	root := append([]byte(nil), trie.Root()...)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	rs := time.Now()
	st2 := openStore(t, backend, dir, sync)
	reopened := smt.ImportSparseMerkleTrie(st2, sha256.New(), root)
	k := keyAt(n / 2)
	proof, err := reopened.Prove(k)
	if err != nil {
		t.Fatalf("Prove after reopen: %v", err)
	}
	if ok, err := smt.VerifyProof(proof, root, k, present, reopened.Spec()); err != nil || !ok {
		t.Fatalf("reopen proof failed (ok=%v, err=%v)", ok, err)
	}
	reopenMS := float64(time.Since(rs).Nanoseconds()) / 1e6
	st2.Close()

	t.Logf("%-8s %-10d %9.2fs %8.1fus %8.1f %7.1f %9.1f %9.2fms %8.2fms",
		backend, n,
		float64(buildNS)/1e9,
		float64(buildNS)/float64(n)/1e3,
		heapMB, rssMB,
		float64(onDisk)/(1<<20),
		applyMS, reopenMS,
	)
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
