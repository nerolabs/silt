package guardstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

func entry(n byte, epoch uint64) ports.PaidSerial {
	return ports.PaidSerial{Serial: []byte{'s', n}, Server: ports.HashBytes([]byte{n}), Epoch: epoch}
}

// TestRoundTripAcrossAReopen is the property the whole store exists for: what was
// appended before a "restart" is what a fresh handle reads back.
func TestRoundTripAcrossAReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []ports.PaidSerial{entry(1, 7), entry(2, 8), entry(3, 9)}
	for _, e := range want {
		if err := d.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	d2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("read back %d entries, wrote %d", len(got), len(want))
	}
	for i := range want {
		if string(got[i].Serial) != string(want[i].Serial) || got[i].Server != want[i].Server || got[i].Epoch != want[i].Epoch {
			t.Fatalf("entry %d round-tripped as %+v, wrote %+v", i, got[i], want[i])
		}
	}
}

// TestAbsentStoreIsEmptyNotAnError: first run must not be a boot failure.
func TestAbsentStoreIsEmptyNotAnError(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "nested", "paidserials.log"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("a fresh store loaded %d entries, err=%v", len(got), err)
	}
}

// TestTornTailIsDroppedNotFatal. A crash between the write and the fsync leaves a
// PARTIAL record. Append had not returned, so the ledger had not paid against it —
// dropping it is correct, and refusing to boot over it would be a self-inflicted
// outage. A record with an impossible serial length is a REAL corruption and must be a
// hard error instead (starting empty is the eviction this store exists to prevent).
func TestTornTailIsDroppedNotFatal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(2, 2)); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Chop the last record in half — a torn append.
	if err := os.WriteFile(p, blob[:len(blob)-recSize/2], 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := d.Load()
	if err != nil {
		t.Fatalf("a torn tail must not be a load error: %v", err)
	}
	if len(got) != 1 || string(got[0].Serial) != "s\x01" {
		t.Fatalf("a torn tail dropped the wrong entries: %+v", got)
	}
}

func TestCorruptRecordIsAHardError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(entry(1, 1)); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	blob[0] = 0xFF // an impossible serial length
	if err := os.WriteFile(p, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, lerr := d.Load(); !errors.Is(lerr, ErrCorrupt) {
		t.Fatalf("a corrupt record must be a hard error, got %v", lerr)
	}
}

// TestCompactReplacesAtomicallyAndKeepsAppending: after a sweep the log holds exactly
// the live set, and the handle keeps working.
func TestCompactReplacesAtomicallyAndKeepsAppending(t *testing.T) {
	p := filepath.Join(t.TempDir(), "paidserials.log")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := byte(0); i < 10; i++ {
		if err := d.Append(entry(i, uint64(i))); err != nil {
			t.Fatal(err)
		}
	}
	live := []ports.PaidSerial{entry(8, 8), entry(9, 9)}
	if err := d.Compact(live); err != nil {
		t.Fatal(err)
	}
	got, err := d.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("after compaction the log holds %d entries, want 2", len(got))
	}
	if err := d.Append(entry(11, 11)); err != nil {
		t.Fatalf("the handle stopped working after a compaction: %v", err)
	}
	got, _ = d.Load()
	if len(got) != 3 {
		t.Fatalf("append after compaction produced %d entries, want 3", len(got))
	}
}

// TestOversizedSerialIsRefused: the record is fixed width, so the store is the last
// place an unbounded serial could smuggle bytes into durable state.
func TestOversizedSerialIsRefused(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "paidserials.log"))
	if err != nil {
		t.Fatal(err)
	}
	big := ports.PaidSerial{Serial: make([]byte, maxSerialBytes+1), Epoch: 1}
	if err := d.Append(big); err == nil {
		t.Fatalf("the store accepted a %d-byte serial", len(big.Serial))
	}
	if err := d.Append(ports.PaidSerial{Serial: nil, Epoch: 1}); err == nil {
		t.Fatalf("the store accepted an empty serial")
	}
}

// TestMaxSerialBytesMatchesTheTokenSerial pins the adapter's record width to the one
// definition of a serial. The adapter cannot import core/blindtoken in production
// (it would be an adapter→core coupling for one constant), so the coupling is pinned
// in TEST code — the same convention core/credit's paid-serial window pin uses.
func TestMaxSerialBytesMatchesTheTokenSerial(t *testing.T) {
	if maxSerialBytes != blindtoken.SerialSize {
		t.Fatalf("guardstore.maxSerialBytes=%d but blindtoken.SerialSize=%d. A record too "+
			"narrow silently refuses honest entries (an unguarded serial = a double-pay); "+
			"too wide re-opens the byte-unbounded map key.",
			maxSerialBytes, blindtoken.SerialSize)
	}
}
