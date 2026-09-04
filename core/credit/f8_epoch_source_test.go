package credit

// R2.10 / F8 — G-F8-2 (Tester, 2026-09-04): a source that FALLS lowers nothing and
// re-admits nothing. Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §3.3 (read the source ONCE at entry, raise the watermark by max, sweep and screen
// against the WATERMARK) and §6 G-F8-2 (delivery arm + relay arm).
//
// The cert's §6 row reads "redeem serial S issued at 5 → paid" at source 10. That is
// an arithmetic slip against its own §3.3 (5 + W = 9 < 10 ⇒ ReasonBackdated at 10):
// S is PAID at source 9 (in-window, 5 + 4 = 9) and SWEPT when the source reaches 10
// (floor 6). The gate follows §3.3.
//
// RED on main: the calls below use the five-argument (no-epoch) form, so this file
// does not compile until the Builder removes the epoch parameters (G-F8-1). After the
// build the stubs in f8_epochsource_stub.go must be replaced by a real
// SetEpochSource / Epoch, or (i) reads the watermark only and the source arms fail.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// mockEpochSource is the MUTABLE mock: the test moves it, the ledger reads it.
type mockEpochSource struct{ e uint64 }

func (m *mockEpochSource) Epoch() uint64 { return m.e }

var _ ports.EpochSource = (*mockEpochSource)(nil)

// f8AnchorSerial is a distinct, correctly-sized relay anchor serial.
func f8AnchorSerial(n int) []byte {
	s := make([]byte, relayAnchorSerialSize)
	s[0], s[1], s[2] = 'f', byte(n), byte(n>>8)
	return s
}

// TestF8_FallingSourceLowersNothingAndReadmitsNothing_Delivery is the delivery arm.
func TestF8_FallingSourceLowersNothingAndReadmitsNothing_Delivery(t *testing.T) {
	const fee = 50_000
	wantPay := int64(fee) - int64(fee)*SkimNum/SkimDen
	server, fetcher, obj := id(1), id(3), id(7)

	src := &mockEpochSource{e: 9}
	l := New(fee, 0)
	l.SetEpochSource(src)

	// Setup: S issued at 5 is in-window at source 9 (5 + W = 9) and pays.
	S := testSerial(1)
	if got := l.RedeemDeliveryCredit(server, fetcher, obj, S, 5); got != wantPay {
		t.Fatalf("setup: at source 9 a serial issued at 5 is in-window and must pay %d, got %d", wantPay, got)
	}
	if _, ok := l.paidSerial[paidKey(5, S)]; !ok {
		t.Fatal("setup: the paid serial S is not in the guard")
	}
	if l.Epoch() != 9 {
		t.Fatalf("setup: Epoch() must report the source's 9, got %d — the ledger is not reading the injected source", l.Epoch())
	}

	// Source → 10: the band advance sweeps floor 6, so S (epoch 5) leaves the map.
	src.e = 10
	if got := l.RedeemDeliveryCredit(server, fetcher, obj, testSerial(2), 10); got != wantPay {
		t.Fatalf("setup: the epoch-10 redeem must pay %d, got %d", wantPay, got)
	}
	if l.epochWatermark != 10 {
		t.Fatalf("setup: the redeem at source 10 must raise the watermark to 10, got %d", l.epochWatermark)
	}
	if _, still := l.paidSerial[paidKey(5, S)]; still {
		t.Fatal("setup: the band advance to 10 (floor 6) must sweep S issued at 5; it is still in the guard")
	}

	// THE GATE: the source falls 10 → 6.
	src.e = 6
	before := sumConserved(l)

	// (i) the watermark stays at 10.
	paid, why := l.RedeemDeliveryCreditReason(server, fetcher, obj, S, 5)
	if l.epochWatermark != 10 {
		t.Fatalf("(i) a source that fell 10 → 6 lowered the watermark to %d, want 10 (R-F8-LATCH: "+
			"epochWatermark = max(epochWatermark, source.Epoch()))", l.epochWatermark)
	}
	if l.Epoch() != 10 {
		t.Fatalf("(i) Epoch() reports %d after the source fell to 6, want the latched 10", l.Epoch())
	}
	// (ii) the swept serial is NOT re-admitted, and Σ does not move.
	if paid != 0 || why != ReasonBackdated {
		t.Fatalf("(ii) re-presenting the swept epoch-5 serial after the source fell to 6 returned (%d, %q), "+
			"want (0, %q) — a falling source re-admitted a serial the ledger already retired",
			paid, why, ReasonBackdated)
	}
	if after := sumConserved(l); after != before {
		t.Fatalf("(ii) a refused backdated redeem must move nothing: Σ moved by %+d", after-before)
	}
	// (iii) a fresh serial issued at 7 is in-window AT THE WATERMARK (7 + 4 ≥ 10) and pays.
	paid, why = l.RedeemDeliveryCreditReason(server, fetcher, obj, testSerial(3), 7)
	if paid != wantPay || why != ReasonPaid {
		t.Fatalf("(iii) a serial issued at 7 with the source at 6 and the watermark at 10 returned (%d, %q), "+
			"want (%d, %q) — the screens must run against the watermark, not the raw source",
			paid, why, wantPay, ReasonPaid)
	}
	if l.epochWatermark != 10 {
		t.Fatalf("(iii) the watermark moved to %d during the in-window redeem, want 10", l.epochWatermark)
	}
}

// TestF8_FallingSourceLowersNothingAndReadmitsNothing_Relay is the relay arm: the
// same three on SpendRelayAnchors with an epoch-5 anchor (ReasonBackdated after the
// fall) and an epoch-7 anchor (records, face = fee).
func TestF8_FallingSourceLowersNothingAndReadmitsNothing_Relay(t *testing.T) {
	const fee = 50_000
	src := &mockEpochSource{e: 9}
	l := New(fee, 0)
	l.SetEpochSource(src)

	a5 := []RelayAnchor{{Epoch: 5, Serial: f8AnchorSerial(1)}}
	if face, why := l.SpendRelayAnchors(a5); face != fee || why != "" {
		t.Fatalf("setup: at source 9 an epoch-5 anchor is in-window and must record (face %d, reason %q), got (%d, %q)",
			int64(fee), "", face, why)
	}
	if _, ok := l.paidSerial[paidKey(5, a5[0].Serial)]; !ok {
		t.Fatal("setup: the spent epoch-5 anchor is not in the guard")
	}

	src.e = 10
	if face, why := l.SpendRelayAnchors([]RelayAnchor{{Epoch: 10, Serial: f8AnchorSerial(2)}}); face != fee || why != "" {
		t.Fatalf("setup: the epoch-10 spend must record, got (%d, %q)", face, why)
	}
	if l.epochWatermark != 10 {
		t.Fatalf("setup: the spend at source 10 must raise the watermark to 10, got %d", l.epochWatermark)
	}
	if _, still := l.paidSerial[paidKey(5, a5[0].Serial)]; still {
		t.Fatal("setup: the band advance to 10 (floor 6) must sweep the epoch-5 anchor; it is still in the guard")
	}

	src.e = 6
	before := sumConserved(l)
	face, why := l.SpendRelayAnchors(a5)
	if l.epochWatermark != 10 {
		t.Fatalf("(i) a source that fell 10 → 6 lowered the watermark to %d, want 10 (R-F8-LATCH)", l.epochWatermark)
	}
	if face != 0 || why != ReasonBackdated {
		t.Fatalf("(ii) re-presenting the swept epoch-5 anchor after the source fell to 6 returned (%d, %q), "+
			"want (0, %q) — a falling source re-admitted an anchor the ledger already retired",
			face, why, ReasonBackdated)
	}
	if _, readmitted := l.paidSerial[paidKey(5, a5[0].Serial)]; readmitted {
		t.Fatal("(ii) the refused anchor was RECORDED — a refusal must record nothing")
	}
	if after := sumConserved(l); after != before {
		t.Fatalf("(ii) a refused anchor spend must move nothing: Σ moved by %+d", after-before)
	}
	face, why = l.SpendRelayAnchors([]RelayAnchor{{Epoch: 7, Serial: f8AnchorSerial(3)}})
	if face != fee || why != "" {
		t.Fatalf("(iii) an epoch-7 anchor with the source at 6 and the watermark at 10 returned (%d, %q), "+
			"want (%d, %q) — in-window at the watermark", face, why, int64(fee), "")
	}
	if l.epochWatermark != 10 {
		t.Fatalf("(iii) the watermark moved to %d during the in-window spend, want 10", l.epochWatermark)
	}
}

// TestF8_NilSourceReadsAsEpochZero pins the epoch-0 permissive core (R-F8-DISABLED,
// cert §3.1): a ledger that never set a source keeps today's epoch-0 behaviour, so
// every in-process fixture that never sets one is untouched.
func TestF8_NilSourceReadsAsEpochZero(t *testing.T) {
	const fee = 50_000
	wantPay := int64(fee) - int64(fee)*SkimNum/SkimDen
	l := New(fee, 0)
	if got := l.Epoch(); got != 0 {
		t.Fatalf("a ledger with no source must read epoch 0, got %d", got)
	}
	if got := l.RedeemDeliveryCredit(id(1), id(3), id(7), testSerial(1), 0); got != wantPay {
		t.Fatalf("an epoch-0 redeem on a source-less ledger must pay %d, got %d", wantPay, got)
	}
	if l.epochWatermark != 0 {
		t.Fatalf("a source-less ledger's watermark moved to %d", l.epochWatermark)
	}
}
