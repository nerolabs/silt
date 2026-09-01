package credit

// TestA4MoneyPumpConservation is the RED-first regression gate for the A4
// money-pump (Boulder 0, R0.1). It must go RED on current main and GREEN after
// the (b)-minimal fix lands.
//
// The bug (delivery.go:67-71 + delivery.go:99): when a provisional lane is
// FIFO-evicted, the eager self-mint (server.balance += bytes - skim) is retained
// on the server's balance and never reversed. When a receipt later redeems that
// evicted lane, RedeemDeliveryCredit pays the conserved leg (fee - skim to
// server, skim to escrow) WITHOUT reversing the retained mint. One delivery is
// paid twice: once by the self-mint (retained through eviction) and once by the
// conserved fee. The total credit in the system exceeds what was granted or
// legitimately transferred.
//
// Conservation invariant (what this test measures):
//
//	Σ(all account balances) + Σ(all escrow reserves) == initial_grant
//	    + Σ(legitimate self-mints for UNWITNESSED serves only)
//
// In this test the only legitimate credit movements are:
//   - per-lane self-mints from RecordServeToObject (bytes-skim to balance, skim
//     to escrow) — correct for unwitnessed bilateral serves
//   - the ChargePublish debit on the fetcher (fee leaves the fetcher's balance)
//   - the conserved fee credit at redeem (server receives fee-skim, escrow skim)
//
// When an evicted lane is redeemed, rule (b) requires the self-mint to be
// reversed (at eviction time) before the conserved leg is paid. The self-mint
// and the conserved leg together count as ONE payment. Under the bug, BOTH
// remain on the books, making total credit exceed the closed-system sum by the
// serve size of the evicted lane.
//
// DESIGN REFERENCE: docs/thinking/2026-09-01-a4-provisional-eviction-fix-design.md
// §"Failing-first regression gate".

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestA4MoneyPumpConservation(t *testing.T) {
	const fee = 50_000
	// Grant the fetcher enough to ChargePublish. Server and flood requesters
	// start at 0 — they earn through serves.
	const grant = fee * 2
	l := New(fee, 0)

	server := id(1)
	fetcher := ports.NodeID(ports.HashBytes([]byte("fetcher-a4")))
	obj := ports.HashBytes([]byte("evicted-obj-a4"))
	chunk := id(9)

	// Seed the fetcher with grant so ChargePublish does not fail.
	l.Register(fetcher)
	l.accounts[fetcher].balance = grant

	// sumLedger returns the total of all account balances plus all escrow
	// reserves — the closed-system conservation quantity.
	sumLedger := func() int64 {
		var total int64
		for _, a := range l.accounts {
			total += a.balance
		}
		for _, e := range l.escrow {
			total += e.balance
		}
		return total
	}

	// ── Step 1: record the initial total (fetcher has grant, everyone else 0). ──
	initial := sumLedger() // == grant

	// ── Step 2: serve lane 0 (the lane that will be evicted). ──
	const bytes0 = 1 << 10 // 1024 bytes
	skim0 := int64(bytes0) * SkimNum / SkimDen
	// After this serve: server.balance += bytes0 - skim0; escrow[obj] += skim0.
	// Total increases by bytes0 (the legitimate self-mint for an unwitnessed serve).
	l.RecordServeToObject(server, fetcher, obj, chunk, bytes0)

	// ── Step 3: flood distinct lanes past maxProvisional to evict lane 0. ──
	// Each flood serve uses 8 bytes to a distinct (requester, object) pair.
	// These are all legitimate self-mints — they stay on the books.
	const floodBytes = 8
	for i := 0; i < maxProvisional; i++ {
		req := ports.NodeID(ports.HashBytes([]byte{'r', byte(i), byte(i >> 8), byte(i >> 16)}))
		floodObj := ports.HashBytes([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		l.RecordServeToObject(server, req, floodObj, chunk, floodBytes)
	}

	// Verify lane 0 was evicted (FIFO).
	if _, stillPresent := l.provisional[provKey{requester: fetcher, root: obj}]; stillPresent {
		t.Fatal("setup: lane 0 was NOT evicted — flood did not push it out; test precondition violated")
	}
	if got := len(l.provisional); got > maxProvisional {
		t.Fatalf("setup: provisional map size %d exceeds cap %d", got, maxProvisional)
	}

	// ── Step 4: ChargePublish — the fetcher pays the withdrawal fee. ──
	// This is the legitimate "money in" that funds the conserved leg at redeem.
	// It debits the fetcher's balance by fee, so the total decreases by fee here.
	if err := l.ChargePublish(fetcher); err != nil {
		t.Fatalf("ChargePublish: %v", err)
	}

	// ── Step 5: capture total BEFORE redeem for the error report. ──
	preRedeemTotal := sumLedger()

	// ── Step 6: redeem the evicted lane. ──
	// Under (b)-minimal: eviction already reversed the self-mint (bytes0-skim0
	// off server balance, skim0 off escrow[obj]). Redeem pays conserved: fee-skim
	// to server, skim to escrow. Net change to total from the redeem: fee - bytes0.
	// Combined with ChargePublish debit (-fee): net = -bytes0. The serve-mint was
	// legitimately reversed by eviction, and the conserved fee replaces it.
	//
	// Under the bug: eviction left the mint on the server's balance. Redeem pays
	// conserved on top. Net change from redeem: +fee. Combined with ChargePublish
	// (-fee): net = 0. But the lane-0 self-mint (bytes0) was NEVER reversed, so
	// the total exceeds the expected sum by bytes0.
	paid := l.RedeemDeliveryCredit(server, fetcher, obj)

	// ── Step 7: conservation assertion. ──
	// Expected total under rule (b) — the only correct sum:
	//   initial (grant to fetcher)
	//   + bytes0 (lane-0 self-mint, reversed at eviction under fix, NOT reversed under bug)
	//   + maxProvisional*floodBytes (flood self-mints, all legitimately unwitnessed)
	//   - fee (ChargePublish debit)
	//   + fee (conserved fee credited to server+escrow at redeem)
	//   - bytes0 (eviction reversal of lane-0 mint — present under fix, absent under bug)
	//   = initial + maxProvisional*floodBytes
	//
	// Under the bug (no eviction reversal), bytes0 is never subtracted:
	//   gotTotal = initial + bytes0 + maxProvisional*floodBytes
	//   gotTotal - wantTotal = bytes0 = 1024 — the leaked mint.
	wantTotal := initial + int64(maxProvisional)*floodBytes
	gotTotal := sumLedger()
	if gotTotal != wantTotal {
		delta := gotTotal - wantTotal
		t.Errorf("A4 money-pump conservation VIOLATED:\n"+
			"  Σbalances+Σescrow = %d\n"+
			"  want              = %d\n"+
			"  delta             = %+d (= %+d bytes minted without a counterparty debit)\n"+
			"  preRedeemTotal=%d initial=%d fee=%d bytes0=%d skim0=%d paid=%d\n"+
			"  The evicted lane's self-mint (%d credits = bytes0−skim0) was retained\n"+
			"  through eviction and the conserved leg (%d credits) was added on top.\n"+
			"  One delivery paid twice. Fix: reverse the mint at eviction (delivery.go:67-71).",
			gotTotal, wantTotal, delta, delta,
			preRedeemTotal, initial, int64(fee), int64(bytes0), skim0, paid,
			int64(bytes0)-skim0, paid)
	}
}
