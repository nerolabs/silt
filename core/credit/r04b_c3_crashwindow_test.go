package credit

// R0.4b C3 — the OTHER half of the supersede/append crash window, MEASURED.
//
// The certification ruled the FIRST half safe: a crash after the supersede and before
// the guard append is an under-pay in every arm (the Tester's arms A/B/C, measured
// 2026-09-03). This file measures the half nothing had measured — the append LANDS and
// the pay is lost — because that half is NOT symmetric, and the asymmetry is a second
// precondition on the FP-2 persisted-ledger flip.
//
// WHAT IS MODELLED. Today's daemon has ONE durable store on this path (the guard) and
// an in-memory ledger, so the window is vacuous: a crash discards the mint, the
// reversal and the payout together. Under FP-2 (a PERSISTED ledger) there are TWO
// durable stores and NO shared transaction between them. The crash then lands in
// between: the guard file is fsynced, the ledger's redeem writes are not.
//
// The model is two ledgers over ONE store, both driven through the identical serve:
//   - `crashed`  runs the redeem. Its append reaches the shared store.
//   - `survivor` is the persisted ledger AS OF the last durable ledger write before the
//     crash — the serve landed, the redeem did not. It then reloads the guard from
//     disk, which is what a restart does, and re-presents the receipt.
//
// See docs/thinking/2026-09-02-r0.4b-c3-close-design.md §9, FP-2.

import (
	"testing"
)

// TestFP2_CrashBetweenTheGuardAppendAndThePayBurnsTheReceipt measures the arm and pins
// the ONE thing that must never change (no double-pay) while recording the residual
// that must not be silently "fixed" (the retained self-mint) as a number.
func TestFP2_CrashBetweenTheGuardAppendAndThePayBurnsTheReceipt(t *testing.T) {
	const fee = 50_000
	const b = int64(64 << 20)
	const wantMint = b - b/8     // 58,720,256 — the eager RecordServe self-mint
	const wantPaid = fee - fee/8 // 43,750 — the conserved leg

	store := &memStore{}
	srv, fetcher, obj := id(31), id(32), id(33)
	serial := testSerial(9_100_001)

	newLedger := func() *Ledger {
		l := New(fee, 0)
		l.SetPaidSerialStore(store)
		if err := l.LoadPaidSerials(); err != nil {
			t.Fatal(err)
		}
		l.Register(srv)
		l.Register(fetcher)
		return l
	}

	crashed := newLedger()
	survivor := newLedger()

	preServe := survivor.Balance(srv)
	for _, l := range []*Ledger{crashed, survivor} {
		l.RecordServeToObject(srv, fetcher, obj, chunkOf(3), b)
	}
	if mint := survivor.Balance(srv) - preServe; mint != wantMint {
		t.Fatalf("setup: the 64 MiB self-mint is %d, want %d", mint, wantMint)
	}

	// The redeem that dies. It completes here so its guard entry reaches the shared
	// durable store; everything it wrote to ITS ledger is then discarded, which is what
	// a process death between delivery.go's addPaidSerial and the payment does when the
	// two stores share no transaction.
	if paid, why := crashed.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 100); paid != wantPaid || why != ReasonPaid {
		t.Fatalf("setup: the pre-crash redeem must pay %d/%s, got %d/%s", wantPaid, ReasonPaid, paid, why)
	}
	if len(store.entries) != 1 {
		t.Fatalf("setup: the guard append did not land durably (%d entries)", len(store.entries))
	}

	// RESTART. The survivor reloads the guard from disk and the receipt is re-presented.
	if err := survivor.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	sigmaBefore := sumConserved(survivor)
	paid, why := survivor.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 100)

	// (1) THE HARD INVARIANT — no double-pay, ever. ReasonAlreadyPaid sits ABOVE the
	// supersede (RT-DELIV-1/1b/2), so the durable guard entry is what burns the receipt.
	if paid != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("BREAK: after a crash between the guard append and the pay, re-presenting "+
			"the SAME receipt paid %d (%s). The durable guard entry must burn it: paying "+
			"here mints credit with no second withdrawal fee behind it (Σ moved %+d).",
			paid, why, sumConserved(survivor)-sigmaBefore)
	}
	if got := sumConserved(survivor) - sigmaBefore; got != 0 {
		t.Fatalf("the re-presentation moved Σ by %+d; a burnt receipt must move nothing", got)
	}

	// (2) THE MEASURED RESIDUAL. ReasonAlreadyPaid returning above the supersede is
	// exactly what stops the reversal, so the eager self-mint stays. That is the G-4
	// lever's shape reached through a crash instead of a refusal.
	residual := survivor.Balance(srv) - preServe
	if residual != wantMint {
		t.Fatalf("the crash-window residual is %+d, was measured at %+d on 2026-09-03. "+
			"If you CHANGED this deliberately you have moved an FP-2 precondition: update "+
			"docs/thinking/2026-09-02-r0.4b-c3-close-design.md §9 and route it to the "+
			"Researcher — the fix is not ledger-local (it needs the guard append and the "+
			"ledger write to share one durable transaction, or an idempotent re-pay keyed "+
			"on the guard entry, which re-opens the RT-DELIV-1/1b/2 bound).",
			residual, wantMint)
	}
	t.Logf("MEASURED (open residual, FP-2): crash between the guard append and the pay — "+
		"re-present paid=0 reason=%q; the server keeps the full %d self-mint with no "+
		"witnessed reversal and no conserved payout, and the receipt is burnt. Under the "+
		"SHIPPED in-memory ledger this is invisible (balances reset at the same restart); "+
		"it becomes live the moment the ledger is persisted.", why, residual)
}
