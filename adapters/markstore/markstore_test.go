package markstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #183 red-team coverage caveat C-2: the fsync-before-broadcast durability of
// the never-sign-twice watermark (I2) was verified by INSPECTION only — the
// restart tests all used markstore.NewMem, so nothing asserted the on-disk
// Save/Load contract against the real Disk store, the one place a future
// refactor could silently break I2's crash-safety. These tests close that: they
// drive the real Disk store the daemon uses and assert the mark is durable
// across a simulated process restart (a NEW Disk instance at the same path).

func markAt(h, r uint64, phase uint8, tag string) ports.SignMark {
	return ports.SignMark{Height: h, Round: r, Phase: phase, Hash: ports.HashBytes([]byte(tag))}
}

// TestDiskMarkSurvivesRestart is the core durability contract: a mark Saved by
// one Disk instance is Loaded intact by a FRESH Disk at the same path — the
// process-restart the watermark exists to survive. If Save did not actually
// reach durable storage (e.g. a refactor dropped the rename or fsync), a fresh
// reader would miss it and a restarted validator could re-sign a signed height.
func TestDiskMarkSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signmark.json")
	writer := New(path)

	want := ports.SignMark{Height: 7, Round: 3, Phase: ports.SignMark{}.Phase + 2, Hash: ports.HashBytes([]byte("blk-7")), LockQC: []byte("lock-qc-bytes")}
	if err := writer.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// RESTART: a brand-new Disk instance, as a fresh process would open.
	reader := New(path)
	got, ok, err := reader.Load()
	if err != nil {
		t.Fatalf("Load after restart: %v", err)
	}
	if !ok {
		t.Fatal("I2 durability VIOLATION: a Saved mark was not found after restart (Save did not reach durable storage)")
	}
	if got.Height != want.Height || got.Round != want.Round || got.Phase != want.Phase || got.Hash != want.Hash || string(got.LockQC) != string(want.LockQC) {
		t.Fatalf("mark corrupted across restart: got %+v, want %+v (every field — including the #432 round/phase/lockQC — must survive)", got, want)
	}
}

// TestDiskAtomicOverwrite: Save replaces an existing mark atomically, and the
// LATEST monotone value is what a restart reads. A half-written file (a crash
// mid-Save) must never be observable — the temp→fsync→rename path guarantees
// the reader sees either the old mark or the new, never a torn one.
func TestDiskAtomicOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signmark.json")
	d := New(path)
	if err := d.Save(markAt(1, 0, ports.SignMark{}.Phase, "first")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	newer := markAt(9, 2, ports.SignMark{}.Phase+1, "ninth")
	if err := d.Save(newer); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, ok, err := New(path).Load()
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if got.Height != newer.Height || got.Hash != newer.Hash {
		t.Fatalf("overwrite lost: got %+v, want %+v", got, newer)
	}
	// No stray temp files left behind (the rename consumed the temp).
	dir := filepath.Dir(path)
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if e.Name() != "signmark.json" {
			t.Fatalf("Save left a stray file behind: %q (temp not renamed/cleaned)", e.Name())
		}
	}
}

// TestDiskMissingIsCleanStart: no file → (zero, false, nil), NOT an error — a
// genuinely fresh validator has no mark and must start, while a CORRUPT file is
// a refuse-to-start error (losing a real mark is the crash window this store
// exists to close, so it must never be mistaken for "fresh").
func TestDiskMissingVsCorrupt(t *testing.T) {
	dir := t.TempDir()

	missing := New(filepath.Join(dir, "absent.json"))
	if m, ok, err := missing.Load(); ok || err != nil {
		t.Fatalf("missing mark must be a clean fresh start (zero,false,nil); got %+v ok=%v err=%v", m, ok, err)
	}

	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := New(corruptPath).Load(); ok || err == nil {
		t.Fatal("a corrupt mark must be a refuse-to-start ERROR, never a silent fresh start (that would drop the crash protection)")
	}
}
