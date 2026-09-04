package credit

// R0.4b C3 re-break — credit-tier regression gates. Inversions of the red-team probes
// core/credit/rt_c3b_credit_test.go (RT-C3B-1 … RT-C3B-5), archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.
// Each keeps the probe's attack verbatim and asserts the CLOSE.

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// failingStore is a PaidSerialStore whose Append always fails — the disk-full /
// read-only-filesystem case. The ledger must refuse to PAY when it cannot record.
type failingStore struct{ mem *memStore }

func (f *failingStore) Load() ([]ports.PaidSerial, error) { return f.mem.Load() }
func (f *failingStore) Append(ports.PaidSerial) error     { return errors.New("disk full") }
func (f *failingStore) Compact(l []ports.PaidSerial) error {
	return f.mem.Compact(l)
}

// memStore is the in-memory PaidSerialStore double. core/credit cannot import
// adapters (hexagonal rule), so the seam is exercised with a local double that has
// the same semantics as adapters/guardstore.Mem.
type memStore struct{ entries []ports.PaidSerial }

func (m *memStore) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), m.entries...), nil
}
func (m *memStore) Append(p ports.PaidSerial) error {
	m.entries = append(m.entries, p)
	return nil
}
func (m *memStore) Compact(live []ports.PaidSerial) error {
	m.entries = append([]ports.PaidSerial(nil), live...)
	return nil
}

// ---------------------------------------------------------------------------
// RT-C3B-1 CLOSED. A restart is not an eviction: the same serial, presented to a
// FRESH ledger sharing the durable store, pays 0.
//
// The probe measured the break as "serial re-paid 43750 after a process restart;
// conserved sum moved +550000 with NO second withdrawal fee behind it".
// ---------------------------------------------------------------------------
func TestRTC3_RestartDoesNotEvictTheGuard(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	srv, fetcher, obj := id(1), id(2), id(7)
	s := testSerial(1)
	store := &memStore{}

	// Boot 1.
	l1 := New(fee, 500_000)
	l1.SetPaidSerialStore(store)
	if err := l1.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if got := l1.RedeemDeliveryCredit(srv, fetcher, obj, s, 3); got != wantPay {
		t.Fatalf("setup: first redeem must pay %d, got %d", wantPay, got)
	}
	if got := l1.RedeemDeliveryCredit(srv, fetcher, obj, s, 3); got != 0 {
		t.Fatalf("setup: the in-process guard must refuse the second redeem, got %d", got)
	}

	// RESTART: a byte-identical fresh ledger over the SAME durable store — what the
	// daemon does at boot (cmd/silt/daemon.go).
	l2 := New(fee, 500_000)
	l2.SetPaidSerialStore(store)
	if err := l2.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	before := sumConserved(l2)
	paid, why := l2.RedeemDeliveryCreditReason(srv, fetcher, obj, s, 3)
	if paid != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("BREAK RT-C3B-1 REOPENED: after a restart the same serial paid %d (%s). "+
			"Σ conserved moved %+d with no second withdrawal fee behind it.",
			paid, why, sumConserved(l2)-before)
	}
}

// TestRTC3_RedeemBeforeLoadIsRefusedNotPaid: the window between attaching the store
// and reading it must not be a paying window. A ledger that does not yet know what it
// already paid must not pay.
func TestRTC3_RedeemBeforeLoadIsRefusedNotPaid(t *testing.T) {
	store := &memStore{}
	l0 := New(50_000, 500_000)
	l0.SetPaidSerialStore(store)
	if err := l0.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	l0.RedeemDeliveryCredit(id(1), id(2), id(7), testSerial(4), 1)

	l := New(50_000, 500_000)
	l.SetPaidSerialStore(store) // attached, NOT loaded
	paid, why := l.RedeemDeliveryCreditReason(id(1), id(2), id(7), testSerial(4), 1)
	if paid != 0 || why != ReasonGuardUnloaded {
		t.Fatalf("a redeem before the guard is loaded paid %d (%s) — it must be refused, "+
			"not paid: the ledger does not yet know what it already paid", paid, why)
	}
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if paid, why := l.RedeemDeliveryCreditReason(id(1), id(2), id(7), testSerial(4), 1); paid != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("after the load the guard must remember the serial, got %d (%s)", paid, why)
	}
}

// TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves: the ORDER is the property. If the
// durable write fails, nothing is paid — a crash between the two can leave a guard
// entry for a payout that never happened (an under-pay) and never the reverse.
func TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves(t *testing.T) {
	l := New(50_000, 500_000)
	l.SetPaidSerialStore(&failingStore{mem: &memStore{}})
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	// Register both accounts first: acct() auto-registers with the grant, which would
	// otherwise show up in the conservation delta as a faucet, not a payout.
	l.Register(id(1))
	l.Register(id(2))
	before := sumConserved(l)
	paid, why := l.RedeemDeliveryCreditReason(id(1), id(2), id(7), testSerial(5), 0)
	if paid != 0 || why != ReasonGuardStore {
		t.Fatalf("a redeem whose guard entry could not be persisted paid %d (%s)", paid, why)
	}
	if got := sumConserved(l); got != before {
		t.Fatalf("Σ conserved moved %+d on a refused redeem", got-before)
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-2 CLOSED. "Evicted ⇒ expired" now holds PER TOKEN. The probe's attack: one
// serial, two valid tokens at two epochs (the withdrawer picks both). The guard was
// keyed by the serial alone with the FIRST redeem's epoch, so the low-epoch entry
// expired and the guard forgot a serial for which a still-in-window token existed —
// "server B collected 43750 off the evicted serial".
// ---------------------------------------------------------------------------
func TestRTC3_GuardEntryExpiresOnItsOwnIssueEpoch(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim
	srvA, srvB, srvC, fetcher, obj := id(1), id(2), id(4), id(3), id(7)
	s := testSerial(9)

	l := New(fee, 0)
	// Token A, issued epoch 0, redeemed at epoch 4 (in window, W=4).
	if got := l.RedeemDeliveryCredit(srvA, fetcher, obj, s, 0); got != wantPay {
		t.Fatalf("setup: token A must pay, got %d", got)
	}
	// Token B: the SAME serial, issued epoch 4 — a DIFFERENT token, funded by its own
	// withdrawal fee. Keyed by the token it is its own entry, so it pays once.
	if got := l.RedeemDeliveryCredit(srvB, fetcher, obj, s, 4); got != wantPay {
		t.Fatalf("token B (same serial, epoch 4) must pay its own conserved leg, got %d — "+
			"it has its own withdrawal fee behind it", got)
	}

	// The window advances past epoch 0 only. A's entry expires; B's must NOT.
	l.sweepExpiredSerials(5)
	if _, still := l.paidSerial[paidKey(0, s)]; still {
		t.Fatalf("the epoch-0 entry survived past its own window")
	}
	if _, live := l.paidSerial[paidKey(4, s)]; !live {
		t.Fatalf("BREAK RT-C3B-2 REOPENED: the sweep at epoch 5 forgot the entry for a token " +
			"issued at epoch 4, which is STILL IN WINDOW. 'evicted ⇒ expired ⇒ un-redeemable' " +
			"is false again, and a second server re-collects off the evicted serial.")
	}
	// And the still-in-window token cannot be collected a second time, at any epoch
	// inside its window.
	for cur := uint64(5); cur <= 8; cur++ {
		l.sweepExpiredSerials(cur)
		if paid, why := l.RedeemDeliveryCreditReason(srvC, fetcher, obj, s, 4); paid != 0 {
			t.Fatalf("epoch %d: server C re-collected %d (%s) off token B", cur, paid, why)
		}
	}
}

// TestRTC3_EveryEvictedEntryIsExpired is the coupling condition stated directly over
// the data structure, swept across the whole window: nothing the sweep removes can
// still be in window at the epoch it was removed.
func TestRTC3_EveryEvictedEntryIsExpired(t *testing.T) {
	l := New(50_000, 0)
	s := testSerial(11)
	// One serial, a token at every epoch in 0..8 — the shape the old MIN-epoch keying
	// collapsed into a single entry.
	for e := uint64(0); e <= 8; e++ {
		l.RedeemDeliveryCredit(id(byte(e)+20), id(2), id(7), s, e)
	}
	for cur := uint64(0); cur <= 20; cur++ {
		before := map[string]paidSerialEntry{}
		for k, v := range l.paidSerial {
			before[k] = v
		}
		l.sweepExpiredSerials(cur)
		for k, v := range before {
			if _, still := l.paidSerial[k]; still {
				continue
			}
			if cur <= paidSerialWindow || v.epoch >= cur-paidSerialWindow {
				t.Fatalf("epoch %d: the sweep evicted an entry issued at epoch %d, which is "+
					"still inside the W=%d window — evicted ⇒ expired is false",
					cur, v.epoch, paidSerialWindow)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-3 (control, must keep HOLDING). Two withdrawals, two fees, two payouts is
// CONSERVED. The re-key must not have turned the under-pay it removed into a mint.
// ---------------------------------------------------------------------------
func TestRTC3_SameSerialTwoEpochsIsStillConserved(t *testing.T) {
	const fee = 50_000
	srvA, srvB, fetcher, obj := id(1), id(2), id(3), id(7)
	s := testSerial(9)
	l := New(fee, 0)
	l.Register(fetcher)
	l.accounts[fetcher].balance = 1 << 40

	before := sumConserved(l)
	if err := l.ChargePublish(fetcher); err != nil {
		t.Fatal(err)
	}
	if err := l.ChargePublish(fetcher); err != nil {
		t.Fatal(err)
	}
	l.RedeemDeliveryCredit(srvA, fetcher, obj, s, 0)
	l.sweepExpiredSerials(5)
	l.RedeemDeliveryCredit(srvB, fetcher, obj, s, 4)
	if after := sumConserved(l); after != before {
		t.Fatalf("MINT: Σ conserved moved %+d across two fees and two payouts", after-before)
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-5 (DISCLOSED, quantified — no fix, by design). Cap griefing costs the skim,
// and the skim funds the griefer's own escrow. This gate PINS the disclosed number so
// the disclosure cannot silently go stale: if a change makes the grief cheaper or the
// refusal different, it reddens. Held-in-tension in
// docs/thinking/2026-09-02-r0.4b-c3-close-design.md; the production close is grant = 0.
// ---------------------------------------------------------------------------
func TestRTC3_CapGriefCostsExactlyTheSkim(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	griefer, victim, fetcher := id(1), id(2), id(3)
	grieferRoot, victimRoot := id(70), id(71)

	l := New(fee, 0)
	l.Register(fetcher)
	l.accounts[fetcher].balance = 1 << 50

	grieferBefore := l.acct(griefer).balance
	for i := 0; i < maxPaidSerial; i++ {
		if err := l.ChargePublish(fetcher); err != nil {
			t.Fatalf("fee %d: %v", i, err)
		}
		if paid := l.RedeemDeliveryCredit(griefer, fetcher, grieferRoot, testSerial(i), 0); paid == 0 {
			t.Fatalf("grief fill stalled at %d", i)
		}
	}
	if len(l.paidSerial) != maxPaidSerial {
		t.Fatalf("fill: guard holds %d, want %d", len(l.paidSerial), maxPaidSerial)
	}
	netCost := int64(maxPaidSerial)*fee - (l.acct(griefer).balance - grieferBefore) - l.escrowFor(grieferRoot).balance
	if netCost != 0 {
		t.Fatalf("the disclosed grief economics changed: net cost %d (was 0 — the skim "+
			"lands in the escrow of the griefer's OWN root). Re-read the disclosure.", netCost)
	}
	paid, why := l.RedeemDeliveryCreditReason(victim, fetcher, victimRoot, testSerial(999999), 0)
	if paid != 0 || why != ReasonGuardFull {
		t.Fatalf("at the cap an honest redeem must be REFUSED (never a live eviction), "+
			"got %d (%s)", paid, why)
	}
	t.Logf("per-entry net cost to the griefer = skim = %d of a %d fee (%.1f%%); victims are "+
		"refused with %q until the window advances (disclosed, grant=0 in production)",
		skim, int64(fee), 100*float64(skim)/float64(fee), ReasonGuardFull)
}
