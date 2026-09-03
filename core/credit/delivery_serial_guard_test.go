package credit

// Regression tests for the R0.4b cross-server double-redeem guard: one demand token
// (one serial, one fee) funds exactly ONE conserved delivery payout.
//
// The eviction gates below are the RED-TEAM's (2026-09-02, against the FIFO-bounded
// guard at commit fcbab7e). They are the discriminator between this design and the
// refuted one, so they are permanent:
//
//	TestSerialGuard_EvictThenReRedeemMintsZero
//	TestSerialGuard_EvictionPumpIsNotSelfFinancing
//
// Both must hold TOGETHER WITH TestSerialGuard_SetIsBounded. That triple is what a
// FIFO-alone design cannot satisfy: FIFO buys the bound by forgetting a
// still-redeemable serial, and re-collecting a forgotten window is self-financing
// (each flood serial is itself a paid delivery the colluding operator collects, so
// advancing the FIFO by one window costs nothing). Expiry-only eviction satisfies all
// three, because a forgotten serial is always an EXPIRED one, and an expired token
// verifies under no in-window issuer key.

import (
	"fmt"
	"testing"
)

// testSerial builds a distinct, non-empty token serial. A distinct serial per
// delivery is the real shape (one blind withdrawal = one serial).
func testSerial(n int) []byte {
	return []byte{'s', byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}
}

// sumConserved is Sigma(balances)+Sigma(escrow), the closed-system conservation
// quantity.
func sumConserved(l *Ledger) int64 {
	var total int64
	for _, a := range l.accounts {
		total += a.balance
	}
	for _, e := range l.escrow {
		total += e.balance
	}
	return total
}

// TestSerialGuard_SecondServerSameSerialMintsZero is the core distinguisher: the
// FIRST completed redeem of a serial pays fee-skim; a SECOND server presenting a
// receipt on the SAME serial mints 0. This is the exact cross-server pump surface.
func TestSerialGuard_SecondServerSameSerialMintsZero(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	serverA, serverB, fetcher := id(1), id(2), id(3)
	obj := id(7)
	serial := []byte("shared-token-serial")

	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	if paid := l.RedeemDeliveryCredit(serverA, fetcher, obj, serial, 0, 0); paid != wantPay {
		t.Fatalf("first redeem paid %d, want fee-skim=%d", paid, wantPay)
	}
	if paid := l.RedeemDeliveryCredit(serverB, fetcher, obj, serial, 0, 0); paid != 0 {
		t.Fatalf("second server on the same serial paid %d, want 0 - the cross-server pump is open", paid)
	}
	if got := l.Balance(serverB); got != 0 {
		t.Fatalf("serverB balance %d, want 0", got)
	}
	if got := l.Balance(serverA); got != wantPay {
		t.Fatalf("serverA balance %d, want the single payout %d", got, wantPay)
	}
}

// TestSerialGuard_DistinctSerialsEachPay is the liveness floor: distinct tokens
// (distinct serials) each fund their own payout. The guard blocks re-redeem of ONE
// serial, never legitimate distinct deliveries. This is the property that keeps
// honest abort-retry paying.
func TestSerialGuard_DistinctSerialsEachPay(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	const n = 100
	for i := 0; i < n; i++ {
		if paid := l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(i), 0, 0); paid != wantPay {
			t.Fatalf("distinct-serial redeem %d paid %d, want fee-skim=%d - the guard blocked a legit delivery", i, paid, wantPay)
		}
	}
	if got := l.Balance(server); got != int64(n)*wantPay {
		t.Fatalf("server balance %d after %d distinct deliveries, want %d", got, n, int64(n)*wantPay)
	}
}

// TestSerialGuard_SameServerSameSerialIsIdempotent: a re-submit of the SAME
// (serial, server) also mints 0. One completed delivery, one payment.
func TestSerialGuard_SameServerSameSerialIsIdempotent(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)
	serial := []byte("one-delivery-serial")

	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, serial, 0, 0); paid != wantPay {
		t.Fatalf("first redeem paid %d, want %d", paid, wantPay)
	}
	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, serial, 0, 0); paid != 0 {
		t.Fatalf("re-submit of the same (serial, server) paid %d, want 0", paid)
	}
	if got := l.Balance(server); got != wantPay {
		t.Fatalf("server balance %d, want the single payout %d", got, wantPay)
	}
}

// TestSerialGuard_SetIsBounded drives the guard set past its cap and asserts it stays
// bounded (build-immutable #8).
//
// This is the third leg of the triple. It is GREEN under FIFO too — that is the
// point: it is the constraint that made FIFO look acceptable, and it must stay GREEN
// alongside the two eviction gates that FIFO fails.
func TestSerialGuard_SetIsBounded(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	// Drive well past the cap, all at ONE epoch so nothing can expire. The set must
	// still never exceed maxPaidSerial.
	cycles := maxPaidSerial * 3
	for i := 0; i < cycles; i++ {
		l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(i), 0, 0)
	}
	if got := len(l.paidSerial); got > maxPaidSerial {
		t.Fatalf("paidSerial map grew to %d after %d distinct serials, cap is %d - unbounded state on the floor box (build-immutable #8)",
			got, cycles, maxPaidSerial)
	}
}

// TestSerialGuard_ExpiryFreesTheCap is the liveness counterpart to SetIsBounded: the
// cap is not a permanent wall. Once the window advances past an entry's issuing
// epoch, the sweep reclaims its slot and a NEW delivery pays again.
//
// This is what makes "refuse to pay at a full cap" a self-healing back-pressure
// rather than a denial: reaching it requires a serve rate above the modeled bound,
// and one epoch later the slots are back.
func TestSerialGuard_ExpiryFreesTheCap(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	// Fill the guard set at epoch 0.
	for i := 0; i < maxPaidSerial; i++ {
		l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(i), 0, 0)
	}
	if got := len(l.paidSerial); got != maxPaidSerial {
		t.Fatalf("guard set holds %d after filling to cap, want %d", got, maxPaidSerial)
	}
	// Still at epoch 0: every entry is in-window, so a fresh delivery must be
	// REFUSED rather than evict a live entry.
	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(-1), 0, 0); paid != 0 {
		t.Fatalf("at a cap full of LIVE serials the redeem paid %d, want 0 - "+
			"evicting a still-redeemable serial re-opens the eviction pump", paid)
	}
	// Advance past the window: every epoch-0 entry has expired and is now
	// un-redeemable upstream, so its slot is safe to reclaim.
	future := paidSerialWindow + 1
	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(-2), future, future); paid != wantPay {
		t.Fatalf("after the window advanced, a fresh delivery paid %d, want fee-skim=%d - "+
			"expiry must free the cap or the guard becomes a permanent denial", paid, wantPay)
	}
	if got := len(l.paidSerial); got > maxPaidSerial {
		t.Fatalf("guard set grew to %d past the cap after the sweep", got)
	}
}

// TestSerialGuard_EmptySerialUnguarded pins the legacy/unwitnessed path: a redeem
// with no serial is not recorded and not blocked (no production caller redeems
// without the receipt serial). This path is outside the witnessed pump surface by
// construction.
func TestSerialGuard_EmptySerialUnguarded(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, nil, 0, 0); paid != wantPay {
		t.Fatalf("empty-serial redeem 1 paid %d, want %d", paid, wantPay)
	}
	if paid := l.RedeemDeliveryCredit(server, fetcher, obj, nil, 0, 0); paid != wantPay {
		t.Fatalf("empty-serial redeem 2 paid %d, want %d - empty serial must stay unguarded", paid, wantPay)
	}
	if got := len(l.paidSerial); got != 0 {
		t.Fatalf("empty serials leaked into the guard set (%d entries), want 0", got)
	}
}

// --- The red-team's two EVICTION gates (2026-09-02). RED at fcbab7e (FIFO-bounded
// guard), GREEN under expiry-only eviction. ---

// TestSerialGuard_EvictThenReRedeemMintsZero: the single-target eviction pump.
// Server A redeems a target serial (honest first payout). The attacker floods
// maxPaidSerial fresh paid serials to evict the target from the guard, then a second
// colluding server re-redeems the target. It MUST mint 0.
func TestSerialGuard_EvictThenReRedeemMintsZero(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	serverA, serverB, fetcher := id(1), id(2), id(3)
	obj := id(7)
	target := []byte("target-serial")

	if paid := l.RedeemDeliveryCredit(serverA, fetcher, obj, target, 0, 0); paid != wantPay {
		t.Fatalf("setup: honest first redeem paid %d, want %d", paid, wantPay)
	}

	// Flood maxPaidSerial fresh distinct serials to try to push `target` out.
	for i := 0; i < maxPaidSerial; i++ {
		l.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(1_000_000+i), 0, 0)
	}

	// Re-redeem the flooded-against, already-paid target on the second server.
	reMint := l.RedeemDeliveryCredit(serverB, fetcher, obj, target, 0, 0)
	if reMint != 0 {
		t.Fatalf("EVICTION PUMP: re-redeem of an evicted, already-paid serial minted %d, want 0 - "+
			"the cross-server double-redeem guard forgot a STILL-REDEEMABLE serial, re-opening the pump",
			reMint)
	}
}

// TestSerialGuard_EvictionPumpIsNotSelfFinancing: the amplified, self-financing form.
// From a bounded prime, the attacker runs evict+re-collect cycles that must NOT grow
// its aggregate spendable balance. Conservation (Sigma) must not rise across the whole
// attack beyond the fees actually burned.
func TestSerialGuard_EvictionPumpIsNotSelfFinancing(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)

	serverA, serverB, fetcher := id(1), id(2), id(3)
	obj := id(7)

	const W = maxPaidSerial
	l.Register(fetcher)
	l.Register(serverA)
	l.Register(serverB)
	l.accounts[fetcher].balance = int64(3*W) * fee

	injected := sumConserved(l)

	// Victims: serverA honestly redeems a full window of distinct serials.
	victims := make([][]byte, W)
	for i := 0; i < W; i++ {
		victims[i] = []byte(fmt.Sprintf("victim-%d", i))
		if err := l.ChargePublish(fetcher); err != nil {
			t.Fatalf("victim charge %d: %v", i, err)
		}
		l.RedeemDeliveryCredit(serverA, fetcher, obj, victims[i], 0, 0)
	}
	// Flood a fresh window on serverB to try to evict the victim window.
	for i := 0; i < W; i++ {
		if err := l.ChargePublish(fetcher); err != nil {
			t.Fatalf("flood charge %d: %v", i, err)
		}
		l.RedeemDeliveryCredit(serverB, fetcher, obj, testSerial(2_000_000+i), 0, 0)
	}
	// Re-collect every victim on serverB.
	var reMinted int64
	for i := 0; i < W; i++ {
		reMinted += l.RedeemDeliveryCredit(serverB, fetcher, obj, victims[i], 0, 0)
	}

	final := sumConserved(l)
	// Sigma is conserved iff every payout was backed by a charge. The re-collects, if
	// they mint, push final above injected.
	if final > injected {
		t.Fatalf("SELF-FINANCING EVICTION PUMP: sum(balances+escrow) rose from %d to %d (delta +%d) - "+
			"the attacker re-collected %d in evicted-serial payouts with no charge behind them. "+
			"The bounded guard's eviction re-opened the cross-server double-redeem at scale.",
			injected, final, final-injected, reMinted)
	}
}
