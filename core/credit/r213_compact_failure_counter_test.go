package credit

// R2.13 — the ledger-side WARN for the BENIGN class (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1: "log a WARN for a
// benign compaction failure"). core/credit has no logger; its observability surface is
// exported monotone counters (GuardFullRefusals, SerialSweeps). This pins that a
// benign Compact error is RECORDED — not discarded as it was before R2.13 — while the
// redeem that drove the sweep still pays (G-CO-2 pins the payout; this pins the
// record). The two classes are read on two signals: Compact error → counted only;
// Append error → ReasonGuardStore (G-CO-3, TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves).

import (
	"testing"
)

func TestR213_BenignCompactionFailureIsRecordedNotDiscarded(t *testing.T) {
	const fee = 50_000
	store := &benignCompactFailure{mem: &memStore{}}
	l := New(fee, 500_000)
	l.SetPaidSerialStore(store)
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	srv, fetcher, obj := id(1), id(2), id(7)
	l.Register(srv)
	l.Register(fetcher)
	if l.CompactFailures() != 0 || l.LastCompactError() != nil {
		t.Fatalf("fresh ledger must report no compaction failures")
	}
	if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(1), 0, 0); paid == 0 {
		t.Fatal("setup: epoch-0 redeem did not pay")
	}
	paid, reason := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(2),
		paidSerialWindow+1, paidSerialWindow+1)
	if paid == 0 || reason != ReasonPaid {
		t.Fatalf("benign class must still pay: paid=%d reason=%q", paid, reason)
	}
	if store.calls != 1 {
		t.Fatalf("expected exactly one Compact attempt, got %d", store.calls)
	}
	if l.CompactFailures() != 1 || l.LastCompactError() == nil {
		t.Fatalf("the benign Compact error must be recorded (WARN class), got failures=%d lastErr=%v",
			l.CompactFailures(), l.LastCompactError())
	}
}
