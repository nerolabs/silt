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
// ledger: `epochWatermark` is the highest epoch the ledger has ever READ, and the sweep
// and the admission check both run against it. Purely subtractive — it can only widen
// what is refused — so the worst case is an under-pay of one server's conserved leg
// during a skew, never an over-pay and never a mint.
//
// R2.10 / F8 (G-F8-4, 2026-09-04): the epoch is no longer a caller parameter — the
// ledger reads ONE injected EpochSource (R-F8-SOURCE). "Two redeemers whose heads
// straddle a boundary" is therefore re-driven as "the SOURCE moved 10 → 9 between
// calls" (a mock or embedder source may fall; the latch is a port contract,
// R-F8-LATCH). The assertions are the SAME as before the re-drive: if either of the
// two gates below must be deleted, the latch was dropped — stop.
//
// These tests isolate the watermark from the source's current value, which is the ONLY
// place the two differ: every gate that reads the raw source alone is blind here,
// because B's presentation is in-window BY THE SOURCE'S OWN VALUE at the time.

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

	// The laggard's presentation: a token issued at epoch 5, redeemed while the source
	// reads 9. In-window by that value (9 − 5 = 4 = W), so the demand layer accepts it
	// and every raw-source check passes.
	const issued, laggardNow, aheadNow = 5, 9, 10

	// CONTROL — no skew. Nothing on this ledger has been past epoch 9, so the call is
	// honest and must pay.
	control := New(fee, 0)
	control.SetEpochSource(&mockEpochSource{e: laggardNow})
	if got := paidOnLane(control, serverB, fetcher, obj, testSerial(1), issued); got != wantPay {
		t.Fatalf("CONTROL IS INERT: without skew the laggard's redeem must pay %d, got %d — "+
			"the gate below would then be refusing for some other reason", wantPay, got)
	}

	// THE GATE — with skew. The source reads 10 when server A redeems, raising the
	// ledger's watermark to 10; the source then falls to 9 for server B's call. Epoch 5
	// has left the window as the LEDGER measures it, so the laggard's redeem must be
	// refused.
	src := &mockEpochSource{e: aheadNow}
	l := New(fee, 0)
	l.SetEpochSource(src)
	if got := paidOnLane(l, serverA, fetcher, obj, testSerial(2), aheadNow); got != wantPay {
		t.Fatalf("setup: the further-ahead redeem must pay %d, got %d", wantPay, got)
	}
	src.e = laggardNow
	before := sumConserved(l)
	if got := paidOnLane(l, serverB, fetcher, obj, testSerial(1), issued); got != 0 {
		t.Fatalf("epoch-skew re-pay: a backdated redeem past the ledger watermark paid %d, want 0 — "+
			"two servers straddling a boundary can mint off one token", got)
	}
	if after := sumConserved(l); after != before {
		t.Fatalf("a refused backdated redeem must move nothing: Σ moved by %+d", after-before)
	}
}

// TestEpochWatermark_IsMonotone pins the direction. A source that falls BEHIND must
// not be able to lower the watermark and re-open the window it already closed —
// otherwise the skew close is defeated by replaying the laggard first.
func TestEpochWatermark_IsMonotone(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	serverA, serverB, fetcher, obj := id(1), id(2), id(3), id(7)

	src := &mockEpochSource{e: 10}
	l := New(fee, 0)
	l.SetEpochSource(src)
	// Ahead: watermark → 10.
	if got := paidOnLane(l, serverA, fetcher, obj, testSerial(10), 10); got != wantPay {
		t.Fatalf("setup: the epoch-10 redeem must pay, got %d", got)
	}
	// The source falls to 8. A behind redeem that is itself in-window at the watermark:
	// allowed, and it must NOT drag the watermark back down.
	src.e = 8
	if got := paidOnLane(l, serverB, fetcher, obj, testSerial(11), 7); got != wantPay {
		t.Fatalf("an in-window backdated redeem (7 + W >= 10) must still pay, got %d", got)
	}
	if l.epochWatermark != 10 {
		t.Fatalf("the watermark must be monotone: got %d after a redeem at epoch 8, want 10", l.epochWatermark)
	}
	// And the out-of-window one is still refused after that.
	if got := paidOnLane(l, serverB, fetcher, obj, testSerial(12), 5); got != 0 {
		t.Fatalf("after a laggard redeem the watermark still governs: paid %d, want 0", got)
	}
}

// TestEpochWatermark_InWindowAnchorIsUnaffected pins the SCOPE of the watermark close:
// it must refuse only what has left the window, never an in-window anchor at a server
// that happens to be behind.
//
// C1 (2026-09-08) re-home. This test was TestEpochWatermark_UnguardedRedeemIsUnaffected
// and its subject was the flat leg's serial-less path — "outside the guard by
// construction, and it must not acquire a new refusal". The anchored lane has no
// serial-less path (a session's budget exists only because an anchor was spent into the
// guard; a mis-sized serial is refused, see
// TestSerialGuard_MalformedSerialIsRefusedAndUnrecorded). The surviving statement of the
// same concern — the close is a NARROWING that must not catch honest traffic — is
// asserted on the guarded path itself.
func TestEpochWatermark_InWindowAnchorIsUnaffected(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	serverA, serverB, fetcher, obj := id(1), id(2), id(3), id(7)

	l := New(fee, 0)
	l.SetEpochSource(&mockEpochSource{e: 10})
	// A redeem at epoch 10 raises the watermark to 10.
	if got := paidOnLane(l, serverA, fetcher, obj, testSerial(20), 10); got != wantPay {
		t.Fatalf("setup: the epoch-10 anchor paid %d, want %d", got, wantPay)
	}
	// An anchor issued at epoch 10 − W is still INSIDE the window at the raised
	// watermark, so it must pay in full.
	if got := paidOnLane(l, serverB, fetcher, obj, testSerial(21), 10-paidSerialWindow); got != wantPay {
		t.Fatalf("an in-window anchor was refused after the watermark rose: paid %d, want %d — the close must narrow, not deny", got, wantPay)
	}
	// One epoch older is OUTSIDE it, and that one is refused: the boundary is real.
	if got, why := settleOnLane(l, serverB, fetcher, obj, testSerial(22), 10-paidSerialWindow-1); got != 0 || why != ReasonBackdated {
		t.Fatalf("the anchor one epoch past the window paid %d (%s), want 0 / %s", got, why, ReasonBackdated)
	}
}
