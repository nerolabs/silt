package credit

// R2.7 detector A2 — supersede suppression (Economist advisory
// ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07 §1.1).
//
// The three stored node-wide counters and the live-lane sum satisfy one exact
// conservation identity, and that identity IS the test. Coverage is witnessed /
// object-aware; the denominator is object-aware bytes, never total served bytes,
// because plain-path serves (manifest chunks, no root) can never be witnessed.
//
// ABLATIONS run RED before this shipped, one per test, recorded in the PR:
//   - drop `l.serveBytesWitnessed += ack` (deliveryanchor.go) ⇒ test 1 and test 3 red
//   - drop `l.serveBytesLaneEvicted += evicted.bytes` (delivery.go) ⇒ test 2 red

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// assertServeSplit asserts the A2 conservation identity and returns the reading.
func assertServeSplit(t *testing.T, l *Ledger, where string) ServeMintStats {
	t.Helper()
	st := l.ServeMintStats()
	sum := st.WitnessedBytes + st.LaneEvictedBytes + st.InFlightBytes
	if sum != st.ObjectAwareBytes {
		t.Fatalf("%s: object-aware %d != witnessed %d + evicted %d + in-flight %d (= %d) — the A2 split leaks",
			where, st.ObjectAwareBytes, st.WitnessedBytes, st.LaneEvictedBytes, st.InFlightBytes, sum)
	}
	return st
}

// TestServedByteSplitIsExactAcrossEveryTerminalState drives one lane through a partial
// acknowledgement, more serving, and a full acknowledgement, asserting the identity at
// every step and watching receiptCoverage move 0 → 3/4 → 3/8 → 1.
func TestServedByteSplitIsExactAcrossEveryTerminalState(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	U := int64(DeliveryIncrementBytes)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r27-a2-split"))

	l := New(fee, grant)
	l.Register(server)
	l.Register(fetcher)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)

	if st := assertServeSplit(t, l, "baseline"); st.ObjectAwareBytes != 0 || st.ReceiptCoverage != 0 {
		t.Fatalf("baseline: object-aware %d coverage %v, want 0 and 0", st.ObjectAwareBytes, st.ReceiptCoverage)
	}

	// Serve B = 4U on one lane. Nothing witnessed yet: coverage 0, all of it in flight.
	B := 4 * U
	serveLane(l, server, fetcher, root, B, U)
	st := assertServeSplit(t, l, "after serving 4U")
	if st.ObjectAwareBytes != B || st.InFlightBytes != B || st.ReceiptCoverage != 0 {
		t.Fatalf("after serving 4U: object-aware %d in-flight %d coverage %v, want %d, %d, 0", st.ObjectAwareBytes, st.InFlightBytes, st.ReceiptCoverage, B, B)
	}
	// Unwitnessable is the plain-path floor: this node served nothing off-lane.
	if st.UnwitnessableBytes != 0 || st.UnwitnessedBytes != B {
		t.Fatalf("after serving 4U: unwitnessable %d unwitnessed %d, want 0 and %d", st.UnwitnessableBytes, st.UnwitnessedBytes, B)
	}

	// Partial ack: j−1 = 3 increments. 3U bytes are witnessed, U stays on the lane.
	if _, _, why := l.SettleDelivery(server, fetcher, root, 3, budget, 0); why != ReasonPaid {
		t.Fatalf("partial settle reason %q, want %q", why, ReasonPaid)
	}
	st = assertServeSplit(t, l, "after partial ack")
	if st.WitnessedBytes != 3*U || st.InFlightBytes != U || st.ReceiptCoverage != 0.75 {
		t.Fatalf("after partial ack: witnessed %d in-flight %d coverage %v, want %d, %d, 0.75", st.WitnessedBytes, st.InFlightBytes, st.ReceiptCoverage, 3*U, U)
	}

	// Serve 4U more on the same lane: the denominator grows, the numerator does not.
	serveLane(l, server, fetcher, root, 4*U, U)
	st = assertServeSplit(t, l, "after serving 4U more")
	if st.ObjectAwareBytes != 8*U || st.WitnessedBytes != 3*U || st.ReceiptCoverage != 0.375 {
		t.Fatalf("after serving 4U more: object-aware %d witnessed %d coverage %v, want %d, %d, 0.375", st.ObjectAwareBytes, st.WitnessedBytes, st.ReceiptCoverage, 8*U, 3*U)
	}

	// Settle to full: the lane holds 5U, so 5 increments acknowledge all of it.
	if _, _, why := l.SettleDelivery(server, fetcher, root, 5, budget, 0); why != ReasonPaid {
		t.Fatalf("full settle reason %q, want %q", why, ReasonPaid)
	}
	st = assertServeSplit(t, l, "after full ack")
	if st.WitnessedBytes != 8*U || st.InFlightBytes != 0 || st.ReceiptCoverage != 1.0 {
		t.Fatalf("after full ack: witnessed %d in-flight %d coverage %v, want %d, 0, 1.0", st.WitnessedBytes, st.InFlightBytes, st.ReceiptCoverage, 8*U)
	}
	if _, live := l.provisional[provKey{server: server, requester: fetcher, root: root}]; live {
		t.Fatal("a fully acknowledged lane survived the settlement")
	}
}

// TestEvictedLaneBytesAreCountedForfeitedNotWitnessed fills the lane map past the cap so
// the oldest lane is FIFO-confiscated, then settles that lane. Its bytes are FORFEITED,
// never witnessed, and the identity still closes.
func TestEvictedLaneBytesAreCountedForfeitedNotWitnessed(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	const per = int64(4096) // bytes per lane; the byte size is irrelevant to the eviction
	server, fetcher := id(1), id(2)

	l := New(fee, grant)
	l.Register(server)
	l.Register(fetcher)

	rootAt := func(i int) ports.Hash { return ports.HashBytes([]byte{byte(i), byte(i >> 8), byte(i >> 16), 'a', '2'}) }
	// maxProvisional lanes fill the map; the next one evicts the oldest (root 0).
	for i := 0; i <= maxProvisional; i++ {
		l.RecordServeToObject(server, fetcher, rootAt(i), ports.ChunkID{byte(i)}, per)
	}
	st := assertServeSplit(t, l, "after overfilling the lane map")
	if st.LaneEvictedBytes != per {
		t.Fatalf("lane-evicted %d, want %d (exactly the one confiscated lane, cap = maxProvisional)", st.LaneEvictedBytes, per)
	}
	if st.ObjectAwareBytes != int64(maxProvisional+1)*per {
		t.Fatalf("object-aware %d, want %d", st.ObjectAwareBytes, int64(maxProvisional+1)*per)
	}
	witnessedBefore := st.WitnessedBytes

	// Settle the EVICTED lane. Its accumulator is gone, so the settlement pays the
	// conserved leg only and acknowledges no served byte.
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	if _, _, why := l.SettleDelivery(server, fetcher, rootAt(0), 1, budget, 0); why != ReasonPaid {
		t.Fatalf("settling the evicted lane: reason %q, want %q", why, ReasonPaid)
	}
	st = assertServeSplit(t, l, "after settling the evicted lane")
	if st.WitnessedBytes != witnessedBefore {
		t.Fatalf("witnessed %d, want %d unchanged — an evicted lane's bytes were counted twice (forfeited AND witnessed)", st.WitnessedBytes, witnessedBefore)
	}
	if st.LaneEvictedBytes != per {
		t.Fatalf("lane-evicted %d, want %d after the settlement", st.LaneEvictedBytes, per)
	}
}

// TestSuppressionShowsAsCoverageBelowOne is the RED-first proof that the detector has
// teeth: two ledgers with identical serve traffic, one banking every receipt and one
// suppressing every receipt, must NOT read the same. Coverage separates them 1.0 vs 0.0,
// and the suppressing server ends STRICTLY poorer (the G-1 direction).
func TestSuppressionShowsAsCoverageBelowOne(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	U := int64(DeliveryIncrementBytes)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r27-a2-suppress"))
	B := 16 * U

	run := func(settle bool) *Ledger {
		l := New(fee, grant)
		l.Register(server)
		l.Register(fetcher)
		budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
		serveLane(l, server, fetcher, root, B, U)
		if settle {
			if _, _, why := l.SettleDelivery(server, fetcher, root, 16, budget, 0); why != ReasonPaid {
				t.Fatalf("settle reason %q, want %q", why, ReasonPaid)
			}
		}
		return l
	}
	bank, suppress := run(true), run(false)

	bst := assertServeSplit(t, bank, "banking ledger")
	sst := assertServeSplit(t, suppress, "suppressing ledger")
	if bst.ObjectAwareBytes != sst.ObjectAwareBytes {
		t.Fatalf("the two ledgers served different amounts (%d vs %d) — the arms are not comparable", bst.ObjectAwareBytes, sst.ObjectAwareBytes)
	}
	if bst.ReceiptCoverage != 1.0 {
		t.Fatalf("banking ledger coverage %v, want 1.0", bst.ReceiptCoverage)
	}
	if sst.ReceiptCoverage != 0.0 {
		t.Fatalf("suppressing ledger coverage %v, want 0.0", sst.ReceiptCoverage)
	}
	if bst.ReceiptCoverage == sst.ReceiptCoverage {
		t.Fatal("coverage reads the same on both arms — the A2 detector is decoration")
	}
	if margin := bank.Balance(server) - suppress.Balance(server); margin <= 0 {
		t.Fatalf("bank − suppress = %d, want STRICTLY > 0 (G-1): suppression must never pay better", margin)
	}
}
