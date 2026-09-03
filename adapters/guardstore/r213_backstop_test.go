package guardstore

// R2.13 — the sticky backstop behind open-before-rename (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1: "keep as backstop,
// not as the fix"). G-CO-1 proves the fix removes the post-rename failure; this file
// proves the backstop is not dead code: the one call still fallible after the rename
// (closing the retired handle) marks the store broken, and a broken store fails every
// later Append and Compact with ErrStoreBroken — it never returns nil for a record a
// fresh Load cannot see.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestR213_BackstopBrokenStoreFailsEveryAppendAndCompact(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	// Make the retired handle's Close fail: close it out from under Compact. This is
	// the only post-rename call that can still return an error.
	if err := d.f.Close(); err != nil {
		t.Fatal(err)
	}
	err = d.Compact([]ports.PaidSerial{entry(1, 1)})
	if !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("a post-rename failure must mark the store broken, got %v", err)
	}
	if err := d.Append(entry(2, 2)); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("Append on a broken store must fail with ErrStoreBroken (sticky), got %v", err)
	}
	if err := d.Compact([]ports.PaidSerial{entry(1, 1)}); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("Compact on a broken store must fail with ErrStoreBroken (sticky), got %v", err)
	}
	// The rename itself went through: the durable contents are the compacted set, and
	// nothing was written past the break.
	d2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got[0].Serial) != string(entry(1, 1).Serial) {
		t.Fatalf("durable contents after a broken compaction must be exactly the compacted set, got %+v", got)
	}
}

// A pre-rename open failure (G-CO-1's injection) leaves the store UNCHANGED and
// healthy: not broken, still appending to the live path. This pins the benign class
// at the adapter, the other half of the port clause.
func TestR213_PreRenameOpenFailureLeavesTheStoreHealthy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(2, 1)); err != nil {
		t.Fatal(err)
	}
	orig := openAppend
	openAppend = func(string, int, os.FileMode) (*os.File, error) {
		return nil, errors.New("injected: open of the temp file failed")
	}
	if err := d.Compact([]ports.PaidSerial{entry(1, 1)}); err == nil {
		t.Fatal("Compact must report the forced open failure")
	}
	openAppend = orig
	if d.broken != nil {
		t.Fatalf("a pre-rename failure is benign and must not mark the store broken: %v", d.broken)
	}
	if err := d.Append(entry(3, 2)); err != nil {
		t.Fatalf("the store must remain appendable, got %v", err)
	}
	got, err := d.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Unchanged by the failed Compact (both original entries survive: a superset of
	// live), and the post-failure append is reachable.
	if len(got) != 3 {
		t.Fatalf("want the 2 original entries + the post-failure append = 3 reachable records, got %d: %+v", len(got), got)
	}
	// No temp file left behind.
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".tmp-paidserials-*"))
	if len(leftovers) != 0 {
		t.Fatalf("a failed Compact must remove its temp file, found %v", leftovers)
	}
}
