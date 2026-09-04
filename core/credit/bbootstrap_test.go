package credit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R2.9a — the B_bootstrap export: per-requester fetched bytes vs identity age.
// These gates pin the SHAPE (what the series says) and the PRIVACY FLOOR (what it
// must never be able to say). They are instrumentation gates: nothing here moves
// credit or standing.

// advanceLedgerEpoch moves the ledger's own epoch clock by presenting a redeem with
// a serial (delivery.go's watermark advance — the ledger's clock on this base; under
// R2.10 the chain-anchored source drives the same number).
func advanceLedgerEpoch(l *Ledger, to uint64) {
	l.RedeemDeliveryCredit(id(200), id(201), ports.HashBytes([]byte("epoch-tick")),
		[]byte(fmt.Sprintf("tick-%d", to)), to, to)
}

func rowFor(t *testing.T, rows []RequesterFetch, bytes int64) RequesterFetch {
	t.Helper()
	for _, r := range rows {
		if r.FetchedBytes == bytes {
			return r
		}
	}
	t.Fatalf("no row with fetchedBytes = %d in %+v", bytes, rows)
	return RequesterFetch{}
}

// TestR29a_TwoRequestersCarryTheirOwnBytesAndAge is the primary shape gate: two
// requesters that first fetched at DIFFERENT epochs and fetched DIFFERENT volumes
// appear as two rows carrying exactly their own numbers.
func TestR29a_TwoRequestersCarryTheirOwnBytesAndAge(t *testing.T) {
	l := New(100, 0)
	server := id(1)
	early, late := id(2), id(3)

	// early first touches the ledger at epoch 0 and fetches 1,000 bytes.
	l.RecordServe(server, early, ports.HashBytes([]byte("c1")), 1_000)

	advanceLedgerEpoch(l, 7)

	// late first touches at epoch 7 and fetches 250 bytes.
	l.RecordServe(server, late, ports.HashBytes([]byte("c2")), 250)

	rows := l.FetchedBytesByRequester()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (only requesters with fetched bytes) — got %+v", len(rows), rows)
	}
	// Order: largest fetcher first (the truncation rule).
	if rows[0].FetchedBytes != 1_000 || rows[1].FetchedBytes != 250 {
		t.Fatalf("rows not ordered by fetchedBytes desc: %+v", rows)
	}
	if got := rowFor(t, rows, 1_000).FirstSeenEpoch; got != 0 {
		t.Fatalf("early requester FirstSeenEpoch = %d, want 0", got)
	}
	if got := rowFor(t, rows, 250).FirstSeenEpoch; got != 7 {
		t.Fatalf("late requester FirstSeenEpoch = %d, want 7 (the ledger's epoch at its first touch)", got)
	}
	if rows[0].SaltedRequester == rows[1].SaltedRequester {
		t.Fatal("two distinct requesters share one salted id")
	}
	requesters, epoch := l.FetchedRequesters()
	if requesters != 2 || epoch != 7 {
		t.Fatalf("FetchedRequesters = (%d, %d), want (2, 7)", requesters, epoch)
	}
}

// TestR29a_FirstSeenEpochIsWrittenOnceSoAgeAdvancesWithTheLedgerEpoch: the age is the
// DIFFERENCE between the ledger's epoch now and the requester's first touch, so a
// later fetch must not reset the identity's age back to zero.
func TestR29a_FirstSeenEpochIsWrittenOnceSoAgeAdvancesWithTheLedgerEpoch(t *testing.T) {
	l := New(100, 0)
	server, fetcher := id(1), id(2)

	advanceLedgerEpoch(l, 3)
	l.RecordServe(server, fetcher, ports.HashBytes([]byte("c1")), 500)

	if got := l.FetchedBytesByRequester()[0].FirstSeenEpoch; got != 3 {
		t.Fatalf("FirstSeenEpoch = %d, want 3", got)
	}
	_, epoch := l.FetchedRequesters()
	if epoch != 3 {
		t.Fatalf("ledger epoch = %d, want 3", epoch)
	}

	advanceLedgerEpoch(l, 11)
	l.RecordServe(server, fetcher, ports.HashBytes([]byte("c2")), 500)

	row := l.FetchedBytesByRequester()[0]
	if row.FirstSeenEpoch != 3 {
		t.Fatalf("FirstSeenEpoch = %d after a second fetch at epoch 11, want 3 (written ONCE at first touch)", row.FirstSeenEpoch)
	}
	if row.FetchedBytes != 1_000 {
		t.Fatalf("fetchedBytes = %d, want 1000 (both fetches)", row.FetchedBytes)
	}
	if _, epoch := l.FetchedRequesters(); epoch != 11 {
		t.Fatalf("ledger epoch = %d, want 11 — age (11-3 = 8 epochs) must advance with the clock", epoch)
	}
}

// TestR29a_TheExportCarriesNoRootAndNoRequesterID is the PRIVACY gate (immutable #4,
// refuse-to-surveil): the exported row must be un-joinable to an identity or an
// object. No ports.Hash-typed field (NodeID is an alias of Hash, so this covers the
// raw requester id AND any object root) can ever appear on it.
func TestR29a_TheExportCarriesNoRootAndNoRequesterID(t *testing.T) {
	hashT := reflect.TypeOf(ports.Hash{})
	rt := reflect.TypeOf(RequesterFetch{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type == hashT || f.Type == reflect.TypeOf([32]byte{}) {
			t.Fatalf("RequesterFetch.%s is %s — a raw ports.Hash/NodeID (or an object root) on the export "+
				"turns the B_bootstrap series into a who-fetched-what log", f.Name, f.Type)
		}
		if f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Map || f.Type.Kind() == reflect.Struct {
			t.Fatalf("RequesterFetch.%s is a composite (%s) — the export is (salted id, bytes, epoch) only", f.Name, f.Type)
		}
		if strings.Contains(strings.ToLower(f.Name), "root") || strings.Contains(strings.ToLower(f.Name), "chunk") ||
			strings.Contains(strings.ToLower(f.Name), "object") {
			t.Fatalf("RequesterFetch.%s names an object — the export carries volume and age, never WHAT was fetched", f.Name)
		}
	}
	// And the emitted id must not BE the requester id (hex or raw).
	l := New(100, 0)
	fetcher := id(9)
	l.RecordServe(id(1), fetcher, ports.HashBytes([]byte("c")), 10)
	row := l.FetchedBytesByRequester()[0]
	if strings.Contains(row.SaltedRequester, fetcher.String()) || row.SaltedRequester == fetcher.String() {
		t.Fatalf("SaltedRequester = %q leaks the raw requester id %q", row.SaltedRequester, fetcher.String())
	}
}

// TestR29a_TheSaltedIDIsStableWithinALedgerAndUnjoinableAcross: an operator can bucket
// by age and volume within one snapshot, but cannot join the series across restarts or
// nodes — a fresh ledger (a restart) salts the SAME requester to a different id.
func TestR29a_TheSaltedIDIsStableWithinALedgerAndUnjoinableAcross(t *testing.T) {
	fetcher := id(4)
	mk := func() *Ledger {
		l := New(100, 0)
		l.RecordServe(id(1), fetcher, ports.HashBytes([]byte("c")), 42)
		return l
	}
	a := mk()
	first := a.FetchedBytesByRequester()[0].SaltedRequester
	again := a.FetchedBytesByRequester()[0].SaltedRequester
	if first != again {
		t.Fatalf("salted id is not stable within one ledger: %q vs %q", first, again)
	}
	b := mk()
	if other := b.FetchedBytesByRequester()[0].SaltedRequester; other == first {
		t.Fatalf("a restart re-salts to the SAME id %q — the series is joinable across processes", other)
	}
}

// TestR29a_TheSnapshotIsBoundedAndReportsTheTotal: the returned snapshot is capped
// (build-immutable #8) and the caller can still tell it was truncated.
func TestR29a_TheSnapshotIsBoundedAndReportsTheTotal(t *testing.T) {
	l := New(100, 0)
	server := id(1)
	const n = MaxRequesterFetchRows + 17
	for i := 0; i < n; i++ {
		l.RecordServe(server, ports.HashBytes([]byte(fmt.Sprintf("fetcher-%d", i))), ports.HashBytes([]byte("c")), int64(i+1))
	}
	rows := l.FetchedBytesByRequester()
	if len(rows) != MaxRequesterFetchRows {
		t.Fatalf("rows = %d, want the cap %d", len(rows), MaxRequesterFetchRows)
	}
	requesters, _ := l.FetchedRequesters()
	if requesters != n {
		t.Fatalf("FetchedRequesters = %d, want %d (the TOTAL, so truncation is visible)", requesters, n)
	}
	// The retained rows are the largest fetchers (the documented truncation bias).
	if rows[0].FetchedBytes != int64(n) {
		t.Fatalf("top row fetchedBytes = %d, want %d (largest-first retention)", rows[0].FetchedBytes, n)
	}
	if rows[len(rows)-1].FetchedBytes <= int64(n-MaxRequesterFetchRows) {
		t.Fatalf("last retained row %d is not above the dropped tail", rows[len(rows)-1].FetchedBytes)
	}
}

// TestR29a_ANodeThatNeverFetchedIsNotInTheSeries: the series is over REQUESTERS, so a
// registered server with zero fetched bytes contributes no row (and no salted id).
func TestR29a_ANodeThatNeverFetchedIsNotInTheSeries(t *testing.T) {
	l := New(100, 500)
	l.Register(id(1))
	l.Register(id(2))
	if rows := l.FetchedBytesByRequester(); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none (nobody fetched)", rows)
	}
	if requesters, _ := l.FetchedRequesters(); requesters != 0 {
		t.Fatalf("FetchedRequesters = %d, want 0", requesters)
	}
}
