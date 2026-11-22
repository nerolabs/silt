package nodestore

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/statehash"
)

func open(t *testing.T, opts ...Option) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// A Store answers exactly as a map would under any mix of writes, overwrites and
// deletes, across flushes: with a buffer this small nearly every write lands
// while some keys are buffered, others are in the file, and others are cached.
func TestStoreMatchesAMapAcrossFlushes(t *testing.T) {
	s := open(t, WithBufferBytes(512), WithCacheBytes(256))
	ref := map[string][]byte{}
	rng := rand.New(rand.NewSource(3))
	key := func() []byte { return []byte{byte(rng.Intn(40)), byte(rng.Intn(40))} }
	for step := 0; step < 30000; step++ {
		k := key()
		switch rng.Intn(5) {
		case 0, 1:
			if err := s.Delete(k); err != nil {
				t.Fatal(err)
			}
			delete(ref, string(k))
		default:
			v := make([]byte, 1+rng.Intn(80))
			rng.Read(v)
			if err := s.Set(k, v); err != nil {
				t.Fatal(err)
			}
			ref[string(k)] = v
		}
		if step%7 == 0 {
			q := key()
			got, err := s.Get(q)
			want, ok := ref[string(q)]
			if ok != (err == nil) || (ok && !bytes.Equal(got, want)) {
				t.Fatalf("step %d: Get(%x) = %x, %v; map holds %x, %v", step, q, got, err, want, ok)
			}
			if !ok && !errors.Is(err, ErrNotFound) {
				t.Fatalf("step %d: a missing key gave %v, want ErrNotFound", step, err)
			}
		}
		if s.Len() != len(ref) {
			t.Fatalf("step %d: Len %d, map holds %d", step, s.Len(), len(ref))
		}
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	for k, want := range ref {
		if got, err := s.Get([]byte(k)); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("after the last flush, Get(%x) = %x, %v; want %x", k, got, err, want)
		}
	}
}

// A value read from the file belongs to the store, not to the file's mapping:
// it reads the same after flushes have rewritten and remapped the file.
func TestValuesReadFromTheFileOutliveFlushes(t *testing.T) {
	s := open(t, WithBufferBytes(64), WithCacheBytes(0))
	if err := s.Set([]byte("k"), []byte("node")); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000; i++ { // grow the file well past its first mapping
		if err := s.Set([]byte{byte(i), byte(i >> 8), 1}, make([]byte, 512)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if string(got) != "node" {
		t.Fatalf("a value read before the file grew now reads %q", got)
	}
}

// A tree kept in a Store has exactly the roots of the same tree kept in memory,
// block after block, including the views RootWith computes without writing.
func TestTreeInAStoreHasTheInMemoryRoots(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	leaf := func(i int) statehash.Leaf {
		k := sha256.Sum256([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		return statehash.Leaf{Key: k[:], Value: []byte{1, byte(rng.Intn(3) + 1)}}
	}
	var seed []statehash.Leaf
	for i := 0; i < 5000; i++ {
		seed = append(seed, leaf(i))
	}
	mem, err := statehash.NewTree(seed)
	if err != nil {
		t.Fatal(err)
	}
	s := open(t, WithBufferBytes(4096), WithCacheBytes(2048))
	disk, err := statehash.NewTreeIn(s, seed)
	if err != nil {
		t.Fatal(err)
	}
	if mem.Root() != disk.Root() {
		t.Fatalf("built roots differ: memory %s, store %s", mem.Root(), disk.Root())
	}
	next := 5000
	for block := 0; block < 60; block++ {
		var ch []statehash.Change
		for j := 0; j < 50; j++ {
			if rng.Intn(3) == 0 {
				lf := leaf(rng.Intn(next))
				ch = append(ch, statehash.Change{Key: lf.Key})
			} else {
				lf := leaf(next)
				next++
				ch = append(ch, statehash.Change{Key: lf.Key, Value: lf.Value})
			}
		}
		wm, err1 := mem.RootWith(ch)
		wd, err2 := disk.RootWith(ch)
		if err1 != nil || err2 != nil || wm != wd {
			t.Fatalf("block %d: RootWith memory %s (%v), store %s (%v)", block, wm, err1, wd, err2)
		}
		if err := mem.Apply(ch); err != nil {
			t.Fatal(err)
		}
		if err := disk.Apply(ch); err != nil {
			t.Fatal(err)
		}
		if mem.Root() != disk.Root() || disk.Root() != wd {
			t.Fatalf("block %d: memory %s, store %s, view %s", block, mem.Root(), disk.Root(), wd)
		}
	}
}

// Close deletes the file and refuses further use; closing twice is harmless.
func TestCloseRemovesTheFile(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("the file outlived Close: %v", err)
	}
	if _, err := s.Get([]byte("k")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Get after Close: %v, want ErrClosed", err)
	}
	if err := s.Set([]byte("k"), []byte("v")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Set after Close: %v, want ErrClosed", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("a second Close: %v", err)
	}
}

// A Store dropped without Close still releases its file once it is collected,
// so a tree the chain loses track of cannot leave a file behind for the life of
// the process.
func TestADroppedStoreReleasesItsFile(t *testing.T) {
	dir := t.TempDir()
	path := func() string {
		s, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Set([]byte("k"), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if err := s.Flush(); err != nil {
			t.Fatal(err)
		}
		return s.Path()
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime.GC()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a dropped store's file was never released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// RemoveStale deletes store files a crashed process left, and nothing else.
func TestRemoveStaleTakesOnlyStoreFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"nodes-123.db", "nodes-9.db", "chain.cbor", "nodes-notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveStale(dir); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	var left []string
	for _, e := range ents {
		left = append(left, e.Name())
	}
	if len(left) != 2 || left[0] != "chain.cbor" || left[1] != "nodes-notes.txt" {
		t.Fatalf("left %v, want [chain.cbor nodes-notes.txt]", left)
	}
	if err := RemoveStale(filepath.Join(dir, "missing")); err != nil {
		t.Fatalf("a missing directory: %v", err)
	}
}

// A key written and deleted between flushes never reaches the file, and a key
// already in the file is removed from it by the next flush.
func TestBufferAbsorbsChurnAndFlushesDeletes(t *testing.T) {
	s := open(t)
	if err := s.Set([]byte("kept"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := s.Set([]byte("churn"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete([]byte("churn")); err != nil {
		t.Fatal(err)
	}
	if _, buffered := s.pending["churn"]; buffered {
		t.Fatal("a key written and deleted between flushes is still buffered")
	}
	if err := s.Delete([]byte("kept")); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"kept", "churn"} {
		if _, ok := s.onDisk([]byte(k)); ok {
			t.Fatalf("%q is in the file after the flush", k)
		}
	}
	if s.Len() != 0 {
		t.Fatalf("Len %d, want 0", s.Len())
	}
}
