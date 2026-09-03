package guardstore

// R2.13 — R-COMPACT-ORPHAN. Gate G-CO-1 (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §6, Tester assignment).
//
// THE MECHANISM (ruling §1, "the one measurement I ran", P7) — as it was BEFORE the fix.
// Compact renamed the temp file onto d.path, THEN re-opened the append handle on the new
// path. If that re-open failed, Compact returned the error — the caller was
// told — but d.f still points at the file's PREVIOUS inode, which the rename has just
// unlinked. Writes and fsyncs through that stale handle succeed (POSIX keeps an open
// unlinked inode alive), so a later Append returns nil while its record never reaches
// any path a fresh Open/Load can see. The failure mode is silent: Compact's own error
// return is the only signal, and today's one caller (core/credit/delivery.go:559)
// discards it unconditionally.
//
// SOURCE SEAM (docs/build-process.md rule 8 — a gate may only claim what it measures,
// so the mechanism creating the injection point is named here, not hidden in a helper).
// guardstore.go now routes both OpenFile calls that open/re-open the append handle
// (Open, and the tail of Compact) through the package var `openAppend = os.OpenFile`.
// That is the ONE non-test line this gate required; it is behaviour-preserving in
// production (the var's value is os.OpenFile unless a test overrides it) and touches no
// other call site — `realign`'s OpenFile (a O_WRONLY truncate-only open, never the
// append handle) is deliberately left alone.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestG_CO1_PostRenameOpenFailureOrphansTheAppendHandle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}

	// Force the open of the new append handle to fail. On the PRE-fix tree that open
	// ran AFTER the rename (the handle swap broke, the ruling's measurement); on the
	// fixed tree it runs BEFORE the rename. The gate asserts the PORT PROPERTY either
	// way: after a failed Compact, Append is non-nil OR its record is visible to a fresh
	// Load — never nil-and-invisible.
	orig := openAppend
	openAppend = func(string, int, os.FileMode) (*os.File, error) {
		return nil, errors.New("injected: open of the new append handle failed")
	}
	defer func() { openAppend = orig }()

	compactErr := d.Compact([]ports.PaidSerial{entry(1, 1)})
	if compactErr == nil {
		t.Fatalf("Compact must report the forced append-handle open failure")
	}

	// Append through whatever handle Compact left behind. Today that is the STALE
	// handle on the unlinked temp inode: the write and fsync succeed, so this returns
	// nil — the orphaning is invisible from the caller's side of THIS call.
	second := entry(2, 2)
	appendErr := d.Append(second)

	// The only way to observe the orphaning honestly: open a FRESH handle on the same
	// path, exactly what a process restart does, and see what actually persisted.
	openAppend = orig
	d2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	visible := false
	for _, e := range got {
		if string(e.Serial) == string(second.Serial) && e.Epoch == second.Epoch {
			visible = true
		}
	}

	if appendErr == nil && !visible {
		t.Fatalf("R-COMPACT-ORPHAN: Compact failed post-rename (%v), then Append(serial=%q) "+
			"returned nil AND a fresh Load of the same path never saw it. The missing port "+
			"contract clause (ruling §1): after a Compact that returns an error, the store "+
			"MUST either remain appendable with Append still durable-and-reachable, or MUST "+
			"fail every subsequent Append. It must never return nil from an Append whose "+
			"record a later Load cannot see. Fresh Load returned %d entries: %+v",
			compactErr, second.Serial, len(got), got)
	}
}
