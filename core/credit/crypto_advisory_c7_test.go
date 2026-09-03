package credit

// Crypto-specialist advisory C-7, 2026-09-03: expired guard entries must be retired on
// the EPOCH-BAND ADVANCE, not only when the cap is reached.
//
// THE FINDING (retention, not soundness). reservePaidSerial returns immediately while
// under the 65,536 cap, and compaction happens only inside sweepExpiredSerials — so on
// any node below the cap, which is every node most of the time, expired serials were
// retained ON DISK indefinitely, far past the W-epoch window that is their whole
// justification. Refuse-not-evict is the correct choice and is unchanged; what changes
// is WHEN the expired set is dropped.
//
// PRIOR ART: Chaum-lineage e-cash has exactly this problem, and the deployed answer
// (Brands, and every epoch-scoped online e-cash since) is an epoch-partitioned spent
// list dropped at rollover. It is also cheaper than the cap-triggered scan: O(cap) per
// epoch instead of O(cap) per refused redeem.

import (
	"testing"
)

func TestC7_ExpiredGuardEntriesAreRetiredOnTheBandAdvanceNotOnlyAtTheCap(t *testing.T) {
	const fee = 50_000
	store := &memStore{}
	l := New(fee, 500_000)
	l.SetPaidSerialStore(store)
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	srv, fetcher, obj := id(1), id(2), id(7)
	l.Register(srv)
	l.Register(fetcher)

	// A handful of paid serials at epoch 0 — nowhere near the 65,536 cap, which is the
	// whole point: this is the regime every node lives in.
	const n = 8
	for i := 0; i < n; i++ {
		if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(i), 0, 0); paid == 0 {
			t.Fatalf("setup: serial %d did not pay", i)
		}
	}
	if got := len(l.paidSerial); got != n {
		t.Fatalf("setup: guard holds %d entries, want %d", got, n)
	}
	if got := len(store.entries); got != n {
		t.Fatalf("setup: the durable store holds %d records, want %d", got, n)
	}

	// The band advances past epoch 0 + W. Nothing here is near the cap, and no redeem
	// needs to make room — the ONLY trigger is the watermark moving.
	l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(9_000), paidSerialWindow+1, paidSerialWindow+1)

	for k, e := range l.paidSerial {
		if e.epoch == 0 {
			t.Fatalf("BREAK C-7: an epoch-0 guard entry (%x…) survived the advance to epoch "+
				"%d, with the guard at %d/%d entries — far below the cap, so nothing will "+
				"ever sweep it. State whose entire justification is a %d-epoch window must "+
				"not be retained past it.",
				k[:8], paidSerialWindow+1, len(l.paidSerial), maxPaidSerial, paidSerialWindow)
		}
	}
	// And the DURABLE log was compacted, not just the in-memory map: retention is a
	// property of the file, which is the thing an auditor asks about.
	for _, r := range store.entries {
		if r.Epoch == 0 {
			t.Fatalf("BREAK C-7: the durable guard file still holds an epoch-0 record after "+
				"the band advanced to epoch %d (%d records on disk)",
				paidSerialWindow+1, len(store.entries))
		}
	}
	t.Logf("after the advance: %d live guard entries, %d durable records",
		len(l.paidSerial), len(store.entries))
}

// TestC7_TheSweepStaysAtMostOncePerEpoch: the new trigger must not turn every redeem
// into an O(cap) scan. The sweptEpoch latch is what bounds it, and it is shared by both
// callers (the watermark advance and the cap).
func TestC7_TheSweepStaysAtMostOncePerEpoch(t *testing.T) {
	l := New(50_000, 500_000)
	srv, fetcher, obj := id(1), id(2), id(7)
	l.Register(srv)
	l.Register(fetcher)
	for i := 0; i < 32; i++ {
		l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(i), 10, 10)
	}
	before := l.sweeps
	for i := 0; i < 32; i++ {
		l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(1_000+i), 10, 10)
	}
	if got := l.sweeps - before; got != 0 {
		t.Fatalf("%d redeems within ONE epoch drove %d sweeps; the latch must allow at "+
			"most one per epoch", 32, got)
	}
	l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(2_000), 11, 11)
	if got := l.sweeps - before; got != 1 {
		t.Fatalf("the epoch advance drove %d sweeps, want exactly 1", got)
	}
}
