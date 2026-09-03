package guardstore

// R2.13 — the sticky backstop behind open-before-rename (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1: "keep as backstop,
// not as the fix"; re-keyed per RULING-R2.13-compact-orphan-11396f1 finding 1). The
// backstop keys on ONE signal — after the swap the append handle's inode must be the
// inode at d.path — and on nothing else. Two pins: it FIRES when that signal is false
// (sticky ErrStoreBroken on every later Append and Compact), and it does NOT fire when
// only the retired handle's Close fails (a healthy store must never refuse payouts).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestR213_BackstopFiresWhenTheHandleDoesNotReachThePath: the seam opens the append
// handle on a DECOY file instead of the temp inode, so after the rename d.f is not the
// file at d.path — the exact "append succeeds, Load never sees it" shape the port clause
// forbids. The backstop must mark the store broken, stickily.
func TestR213_BackstopFiresWhenTheHandleDoesNotReachThePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	orig := openAppend
	openAppend = func(_ string, flag int, perm os.FileMode) (*os.File, error) {
		return os.OpenFile(filepath.Join(dir, "decoy.log"), flag|os.O_CREATE, perm)
	}
	defer func() { openAppend = orig }()
	err = d.Compact([]ports.PaidSerial{entry(1, 1)})
	if !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("a handle that does not reach the live path must mark the store broken, got %v", err)
	}
	if err := d.Append(entry(2, 2)); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("Append on a broken store must fail with ErrStoreBroken (sticky), got %v", err)
	}
	if err := d.Compact([]ports.PaidSerial{entry(1, 1)}); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("Compact on a broken store must fail with ErrStoreBroken (sticky), got %v", err)
	}
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

// TestR213_RetiredHandleCloseFailureDoesNotBreakTheStore: closing the RETIRED handle
// out from under Compact makes old.Close() fail. That inode is already unlinked; the
// live handle is fine. The store must NOT be marked broken (the PE's probe: SameFile
// held and a fresh Load saw the next append) — marking it broken would refuse every
// payout on a healthy store until restart.
func TestR213_RetiredHandleCloseFailureDoesNotBreakTheStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := d.f.Close(); err != nil { // the retired handle's Close will now fail inside Compact
		t.Fatal(err)
	}
	if err := d.Compact([]ports.PaidSerial{entry(1, 1)}); err != nil {
		t.Fatalf("a retired-handle Close failure is not a broken store, got %v", err)
	}
	if d.broken != nil {
		t.Fatalf("store marked broken on a healthy handle: %v", d.broken)
	}
	if err := d.Append(entry(2, 2)); err != nil {
		t.Fatalf("the store must remain appendable, got %v", err)
	}
	d2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("the post-compaction append must be reachable by a fresh Load, got %d records", len(got))
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
