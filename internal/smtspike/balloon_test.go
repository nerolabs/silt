package smtspike

import (
	"os"
	"runtime"
	"strconv"
	"testing"
)

// The coexistence balloon: the instrument that makes the floor-box RSS number
// mean something. The store profile (docs/thinking/2026-08-27-disk-backed-
// mapstore-options.md) recorded bbolt at 1M keys as heap 305 MB / RSS 1328 MB,
// but on an OTHERWISE-IDLE box — so the kernel never reclaimed the clean mmap'd
// page cache, and 1328 MB over-counts the coexistence risk by exactly its
// evictable portion. The decisive #600 question is whether that cache SHEDS
// toward the ~305 MB unevictable heap floor under a real daemon's memory
// pressure, or the box OOMs. This balloon supplies the missing pressure.
//
// The physics: on a NO-SWAP box, anonymous memory cannot be paged out. So a
// resident anonymous buffer genuinely competes for physical RAM against bbolt's
// page cache — that competition IS the measurement. The trap is that a
// make([]byte, n) that is never touched is NOT resident (Go zero-fills lazily
// via demand paging), so it would create no pressure — a fake balloon, a false
// measurement, a wrong backend call. inflateBalloon defeats that by writing
// every page and PROVING it did (touched-page count + checksum, cross-platform;
// the residentMB() jump, Linux only).

// heldBalloon keeps a live reference to the ballooned buffer for the entire test
// process lifetime, so the GC can never reclaim it and its pages stay resident.
// Package-level on purpose: a function-local slice would become collectable the
// moment the profile row finished.
var heldBalloon []byte

// inflateBalloon allocates mb MiB of anonymous memory, writes a non-zero byte to
// every page to fault it fully resident, and pins it via heldBalloon so GC never
// frees it. It returns the number of pages it touched and a checksum over one
// touched byte per page — the cross-platform proof the pages were really written
// (works on a macOS dev box that has no /proc for residentMB() to read).
//
// mb <= 0 is a no-op returning (0, 0): SILT_COEXIST_BALLOON_MB unset/0 leaves the
// profile byte-for-byte as it was.
func inflateBalloon(mb int) (touchedPages int, checksum uint64) {
	if mb <= 0 {
		return 0, 0
	}
	pageSize := os.Getpagesize()
	buf := make([]byte, mb<<20)
	for off := 0; off < len(buf); off += pageSize {
		// A non-zero, offset-dependent write. Non-zero so the page is a real
		// dirty physical page (a zero page could in principle be deduplicated to
		// the shared zero page on some kernels); offset-dependent so the checksum
		// detects a stride bug that skips pages.
		buf[off] = byte(off>>12) | 1
		checksum += uint64(buf[off]) * uint64(off/pageSize+1)
		touchedPages++
	}
	// Read a byte back from every page after writing, so a fully dead-code-
	// eliminating compiler still cannot drop the writes: the sum is observed.
	var sink uint64
	for off := 0; off < len(buf); off += pageSize {
		sink += uint64(buf[off])
	}
	_ = sink
	heldBalloon = buf // pin: live reference for the process lifetime
	return touchedPages, checksum
}

// balloonMB reads SILT_COEXIST_BALLOON_MB. Unset or 0 => balloon DISABLED.
func balloonMB(t *testing.T) int {
	raw := os.Getenv("SILT_COEXIST_BALLOON_MB")
	if raw == "" {
		return 0
	}
	mb, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("SILT_COEXIST_BALLOON_MB: %q: %v", raw, err)
	}
	return mb
}

// TestBalloonResident is the failing-first proof that the balloon pins REAL
// resident memory, catchable locally in seconds. It is always on (no env gate)
// so the mechanism is verified on every run, not only during the billable
// profile.
//
//   - Cross-platform (incl. macOS dev box): inflateBalloon must touch exactly one
//     page per stride and produce a non-zero checksum — proof the pages were
//     allocated AND written, without which the balloon is a no-op that creates no
//     pressure.
//   - Linux (the floor box): residentMB() must JUMP by ~balloon size when the
//     balloon is inflated, proving the touched pages are actually resident and not
//     merely reserved. This is the load-bearing assertion; it silently no-ops off
//     Linux where /proc is absent.
func TestBalloonResident(t *testing.T) {
	// Keep the unit-test balloon small so it runs anywhere without stressing the
	// dev box; the physics is identical at 1060 MB.
	const mb = 64
	pageSize := os.Getpagesize()
	wantPages := (mb << 20) / pageSize

	linux := residentMB() > 0 // /proc/self/statm readable => Linux floor box
	var rssBefore float64
	if linux {
		rssBefore = residentMB()
	}

	touched, checksum := inflateBalloon(mb)

	if touched != wantPages {
		t.Fatalf("touched %d pages, want %d (%d MiB / %d-byte pages) — the balloon "+
			"did not write every page, so it is not fully resident", touched, wantPages, mb, pageSize)
	}
	if checksum == 0 {
		t.Fatalf("checksum 0 after touching %d pages — writes were elided; a "+
			"malloc'd-but-untouched buffer creates NO memory pressure and is a false balloon", touched)
	}
	if len(heldBalloon) != mb<<20 {
		t.Fatalf("heldBalloon holds %d bytes, want %d — the live reference is not "+
			"pinned, GC can reclaim the balloon mid-run", len(heldBalloon), mb<<20)
	}

	t.Logf("balloon: %d MiB, touched %d pages, checksum=%d, linux=%v", mb, touched, checksum, linux)

	if linux {
		runtime.GC()
		rssAfter := residentMB()
		jump := rssAfter - rssBefore
		// Require most of the balloon to be resident. Allow slack for pages the
		// kernel may not have faulted yet and measurement noise, but a real
		// balloon must move RSS by the bulk of its size — a fake one moves it ~0.
		if jump < float64(mb)*0.8 {
			t.Fatalf("residentMB jumped %.1f MiB for a %d MiB balloon (before=%.1f "+
				"after=%.1f) — the pages are NOT resident; the balloon creates no real "+
				"pressure and the coexistence measurement would be false", jump, mb, rssBefore, rssAfter)
		}
		t.Logf("resident jump: %.1f MiB (before=%.1f after=%.1f) — balloon is genuinely resident",
			jump, rssBefore, rssAfter)
	} else {
		t.Logf("residentMB()==0 (no /proc): RSS jump unverifiable off Linux; " +
			"touched-page + checksum proof stands, floor-box run asserts the RSS jump")
	}

	// Release for the rest of the suite: this is the unit-proof, not the profile.
	heldBalloon = nil
}
