package guardstore

// R-GUARD-RESTORE-LANE-UNKNOWN, the ADAPTER half: the record has to carry the lane,
// and a file written before the record grew one has to be refused rather than
// re-framed. PE ruling
// /Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R2.9-deposit-at-anchor-expiry-4d4a90c-2026-09-07.md
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
	"bytes"
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

// TestEmptyPreBumpStoreUpgradesButAWrittenOneRefuses is the BLAST RADIUS of the format
// bump, measured (blind PE ruling
// RULING-c1-demand-v2-and-flat-leg-retirement-a290bff-2026-09-08.md §2): which operators
// are actually stopped by it, and which are not.
//
// A pre-bump build wrote no header, so a node that armed a paid lane and never paid left
// a 0-BYTE file. That file is upgraded in place, not refused, and that is the correct
// call: a header is not a record, no Append ever returned for anything in the file, so
// there is nothing to mis-frame and nothing to lose. Refusing it would stop the LARGE
// population — every operator who armed the lane — to protect a state that does not
// exist.
//
// A store carrying at least one written record is refused, and the file is left BYTE-FOR-
// BYTE intact: the operator's remedy (cmd/silt remedyPaidSerials / remedyCreditSpent, one
// of which is "rotate the publish key AND clear the log together") is only available if
// the refusal did not already clear it.
func TestEmptyPreBumpStoreUpgradesButAWrittenOneRefuses(t *testing.T) {
	dir := t.TempDir()

	// Arm 1 — the 0-byte pre-bump file: silent in-place upgrade.
	empty := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(empty)
	if err != nil {
		t.Fatalf("a 0-byte pre-bump store was refused (%v). It holds no record, so refusing it "+
			"stops every operator who armed a paid lane and never paid, to protect nothing", err)
	}
	st, err := os.Stat(empty)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != headerSize {
		t.Fatalf("after Open the 0-byte store is %d bytes, want %d (the header, written in place)", st.Size(), headerSize)
	}
	// And it is a working store, not merely one that opened.
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	got, err := d.Load()
	if err != nil || len(got) != 1 {
		t.Fatalf("the upgraded store loaded %d records (err %v), want 1", len(got), err)
	}
	d.Close()

	// Arm 2 — one written pre-bump record: refused, and the bytes survive the refusal.
	const legacyRecSize = 1 + maxSerialBytes + 32 + 8
	written := filepath.Join(dir, "written.log")
	blob := make([]byte, legacyRecSize)
	blob[0], blob[1], blob[2] = 2, 's', 1
	if err := os.WriteFile(written, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	d2, err := Open(written)
	if err == nil {
		d2.Close()
		t.Fatal("a pre-bump store holding ONE record opened cleanly — its 73-byte record is re-framed " +
			"at the new 74-byte width (the F2 mis-framing)")
	}
	if !errors.Is(err, ErrLegacyFormat) {
		t.Fatalf("Open returned %v, want ErrLegacyFormat", err)
	}
	after, rerr := os.ReadFile(written)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(after, blob) {
		t.Fatalf("the refusal rewrote the file (%d bytes, was %d). A refusal that clears the store "+
			"destroys the state the operator's remedy needs — on creditspent.log the ratified remedy "+
			"is to rotate the publish key AND clear the log together, which is not available once the "+
			"adapter has cleared it unilaterally", len(after), len(blob))
	}
}
