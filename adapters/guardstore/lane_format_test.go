package guardstore

// R-GUARD-RESTORE-LANE-UNKNOWN, the ADAPTER half: the record has to carry the lane,
// and a file written before the record grew one has to be refused rather than
// re-framed. PE ruling
// /Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R2.9-deposit-at-anchor-expiry-4d4a90c-2026-09-07.md
// §4 (the (3, 0) measurement). The ledger half is
// core/credit/guard_restore_lane_test.go.
//
// ABLATIONS (each run RED once, 2026-09-08):
//   - drop the lane byte from encode/decode → TestRecordCarriesTheLane fails
//     ("record 1 read back as delivery, want relay").
//   - drop the header check from Open (accept a headerless file) →
//     TestPreLaneFormatIsRefusedNotSilentlyReframed fails: the 4-record legacy file
//     (292 bytes) is not a multiple of 74, so Load reports ErrCorrupt on a torn tail
//     it silently accepted — and at 74 legacy records it loads SILENTLY with wrong
//     contents (the F2 shape the realign comment already names).

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestRecordCarriesTheLane: both populations round-trip through the disk store with
// their lane intact. Without it every restored entry is a delivery entry, and the
// per-lane live counts (and RestoredGuardEntries, the operator's lost-deposit bound)
// conflate the two populations after every restart.
func TestRecordCarriesTheLane(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	del, rel := entry(1, 1), entry(2, 2)
	rel.Relay = true
	if err := d.Append(del); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(rel); err != nil {
		t.Fatal(err)
	}
	// Through a re-open, so the assertion is on the BYTES and not on a struct the
	// process still holds.
	d2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Relay || !got[1].Relay {
		t.Fatalf("read back %+v, want record 0 delivery and record 1 relay", got)
	}
	// Compaction rewrites the whole file: the lane must survive that path too.
	if err := d2.Compact(got); err != nil {
		t.Fatal(err)
	}
	got2, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 2 || got2[0].Relay || !got2[1].Relay {
		t.Fatalf("after a compaction, read back %+v, want record 0 delivery and record 1 relay", got2)
	}
}

// TestPreLaneFormatIsRefusedNotSilentlyReframed is the compatibility rule: a store
// written before the lane byte existed is a REFUSE-TO-START error naming the file,
// never a file read as records of the new width.
//
// Why refuse and not migrate: the pre-bump record has no lane, so a migration would
// have to guess one, and the only available guess (delivery) is exactly the
// mis-count this change closes — baked into the durable format at the next
// compaction instead of into one boot. Refusing says it out loud at the one moment
// the operator can still act.
func TestPreLaneFormatIsRefusedNotSilentlyReframed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	// The pre-bump record: length byte, serial, server, epoch. No header, no lane.
	const legacyRecSize = 1 + maxSerialBytes + 32 + 8
	blob := make([]byte, 0, 4*legacyRecSize)
	for i := byte(1); i <= 4; i++ {
		var rec [legacyRecSize]byte
		rec[0] = 2
		rec[1], rec[2] = 's', i
		binary.BigEndian.PutUint64(rec[1+maxSerialBytes+32:], uint64(i))
		blob = append(blob, rec[:]...)
	}
	if err := os.WriteFile(p, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(p)
	if err == nil {
		d.Close()
		t.Fatal("a pre-lane store opened cleanly — its records are re-framed at the new width, which mis-frames every guard entry (the F2 double-pay shape)")
	}
	if !errors.Is(err, ErrLegacyFormat) {
		t.Fatalf("Open returned %v, want ErrLegacyFormat naming the file", err)
	}
}

// TestFreshStoreWritesItsHeader: the detector above only works if every store this
// build creates carries the header, including one created by a compaction.
func TestFreshStoreWritesItsHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Compact([]ports.PaidSerial{entry(1, 1)}); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) != headerSize+recSize {
		t.Fatalf("the compacted log is %d bytes, want %d (header) + %d (one record)", len(blob), headerSize, recSize)
	}
	if binary.BigEndian.Uint32(blob[0:]) != magic || binary.BigEndian.Uint32(blob[4:]) != version {
		t.Fatalf("the compacted log carries no format header: % x", blob[:headerSize])
	}
	// The magic's first byte must be impossible as a pre-bump serial-length byte —
	// that is what makes "no header ⇒ pre-bump" exact rather than probabilistic.
	if blob[0] <= maxSerialBytes {
		t.Fatalf("magic byte 0 is %d, which is a legal pre-bump serial length — a legacy file could pass the header check", blob[0])
	}
	// And it re-opens.
	if _, err := Open(p); err != nil {
		t.Fatalf("a store this build wrote does not re-open: %v", err)
	}
}
