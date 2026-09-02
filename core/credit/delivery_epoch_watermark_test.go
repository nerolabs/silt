package credit

// R0.4b-5 — the shared-ledger epoch-skew re-pay, and the high-water mark that closes it.
//
// THE HOLE. `sweepExpiredSerials` and the admission check ran against the CALLER's
// `currentEpoch`. Two redeemers sharing ONE ledger whose heads straddle an epoch
// boundary can therefore re-pay one token: A, at current = 10, sweeps a serial issued
// at epoch 5; B, still at current = 9, legitimately holds key_5 (9 − 5 = 4 ≤ W), so
// its own demand layer ACCEPTS the token — and the ledger, having forgotten the
// serial, pays a second time. Bounded to one epoch of serials per skew event, and
// unreachable in today's production topology (one ledger per node, one head), which
// is why it was HELD rather than shipped-blocking.
//
// THE CLOSE. The ledger is the shared resource, so the monotone clock belongs to the
// ledger: `epochWatermark` is the highest `currentEpoch` any redeemer has presented,
// and the sweep and the admission check both run against it. Purely subtractive — it
// can only widen what is refused — so the worst case is an under-pay of one server's
// conserved leg during a skew, never an over-pay and never a mint.
//
// These tests isolate the watermark from the caller's own epoch, which is the ONLY
// place the two differ: every gate that reads `currentEpoch` alone is blind here,
// because B's presentation is in-window BY B'S OWN CLOCK.

import "testing"

// TestEpochWatermark_LaggardRedeemerCannotRePay is the R0.4b-5 gate, with its control.
//
// The control is what makes it a gate rather than an assertion: the IDENTICAL call on
// a ledger that never saw the further-ahead epoch PAYS. That is the pre-fix behaviour,
// so the refusal is measured against the mint it replaced.
func TestEpochWatermark_LaggardRedeemerCannotRePay(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	serverA, serverB, fetcher, obj := id(1), id(2), id(3), id(7)

	// The laggard's presentation: a token issued at epoch 5, redeemed by a server
	// whose head is at epoch 9. In-window by B's own clock (9 − 5 = 4 = W), so B's
	// demand layer accepts it and every currentEpoch-only check passes.
	const issued, laggardNow, aheadNow = 5, 9, 10

	// CONTROL — no skew. Nothing on this ledger has been past epoch 9, so the call is
	// honest and must pay.
	control := New(fee, 0)
	if got := control.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(1), issued, laggardNow); got != wantPay {
		t.Fatalf("CONTROL IS INERT: without skew the laggard's redeem must pay %d, got %d — "+
			"the gate below would then be refusing for some other reason", wantPay, got)
	}

	// THE GATE — with skew. Server A, one epoch ahead, redeems first and raises the
	// ledger's watermark to 10. Epoch 5 has now left the window as the LEDGER measures
	// it, so the laggard's redeem must be refused.
	l := New(fee, 0)
	if got := l.RedeemDeliveryCredit(serverA, fetcher, obj, testSerial(2), aheadNow, aheadNow); got != wantPay {
		t.Fatalf("setup: the further-ahead redeem must pay %d, got %d", wantPay, got)
	}
	before := sumConserved(l)
	if got := l.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(1), issued, laggardNow); got != 0 {
		t.Fatalf("epoch-skew re-pay: a backdated redeem past the ledger watermark paid %d, want 0 — "+
			"two servers straddling a boundary can mint off one token", got)
	}
	if after := sumConserved(l); after != before {
		t.Fatalf("a refused backdated redeem must move nothing: Σ moved by %+d", after-before)
	}
}

// TestEpochWatermark_IsMonotone pins the direction. A redeemer that falls BEHIND must
// not be able to lower the watermark and re-open the window it already closed —
// otherwise the skew close is defeated by replaying the laggard first.
func TestEpochWatermark_IsMonotone(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	serverA, serverB, fetcher, obj := id(1), id(2), id(3), id(7)

	l := New(fee, 0)
	// Ahead: watermark → 10.
	if got := l.RedeemDeliveryCredit(serverA, fetcher, obj, testSerial(10), 10, 10); got != wantPay {
		t.Fatalf("setup: the epoch-10 redeem must pay, got %d", got)
	}
	// A behind redeem that is itself in-window at the watermark: allowed, and it must
	// NOT drag the watermark back down.
	if got := l.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(11), 7, 8); got != wantPay {
		t.Fatalf("an in-window backdated redeem (7 + W >= 10) must still pay, got %d", got)
	}
	if l.epochWatermark != 10 {
		t.Fatalf("the watermark must be monotone: got %d after a redeem at epoch 8, want 10", l.epochWatermark)
	}
	// And the out-of-window one is still refused after that.
	if got := l.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(12), 5, 8); got != 0 {
		t.Fatalf("after a laggard redeem the watermark still governs: paid %d, want 0", got)
	}
}

// TestEpochWatermark_UnguardedRedeemIsUnaffected pins the scope. The legacy /
// unwitnessed path carries no serial, is outside the guard by construction, and must
// not acquire a new refusal — the close is additive to the guarded path only.
func TestEpochWatermark_UnguardedRedeemIsUnaffected(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	serverA, serverB, fetcher, obj := id(1), id(2), id(3), id(7)

	l := New(fee, 0)
	l.RedeemDeliveryCredit(serverA, fetcher, obj, testSerial(20), 10, 10)
	if got := l.RedeemDeliveryCredit(serverB, fetcher, obj, nil, 0, 0); got != wantPay {
		t.Fatalf("an unguarded (serial-less) redeem must be untouched by the watermark: paid %d, want %d", got, wantPay)
	}
}
