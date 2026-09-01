package credit

// The PoD neutral-lane ledger tests (docs/design/pod.md §3–§4, certified
// 2026-08-26). Four properties, each load-bearing for the certified B3 close:
// the §7.1 firewall (a heavy deliverer's standing never moves), conservation
// (credit = fee − skim, a transfer never a mint), the supersede rule (a
// witnessed receipt replaces the serve self-record, never stacks on it), and
// the wash bound (a colluding pair's loop is a strict loss of the skim).

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestDeliveryCreditNeverTouchesStanding is the certified spec's §7.1 firewall
// test, verbatim: a big deliverer's Reputation() is unchanged across the
// reward. This is the direct guard on the γ→1/N firewall — a receipt is
// mintable with zero object bytes by certified design, so delivery credit
// buying even one unit of standing would convert forged receipts into
// consensus weight.
func TestDeliveryCreditNeverTouchesStanding(t *testing.T) {
	l := New(50_000, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	before := l.Reputation(server)
	for i := 0; i < 1_000; i++ { // a heavy delivery volume
		if paid := l.RedeemDeliveryCredit(server, fetcher, obj); paid <= 0 {
			t.Fatalf("redeem %d paid %d, want > 0", i, paid)
		}
	}
	if got := l.Reputation(server); got != before {
		t.Fatalf("delivery credit moved standing: Reputation %d → %d — the γ→1/N firewall is breached", before, got)
	}
}

// TestDeliveryCreditIsConserved pins the conservation arithmetic: one redeem
// pays the server exactly fee − skim and routes exactly skim into the object's
// escrow — the fetcher's withdrawal fee moved, nothing minted.
func TestDeliveryCreditIsConserved(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := id(7)

	skim := int64(fee) * SkimNum / SkimDen
	paid := l.RedeemDeliveryCredit(server, fetcher, obj)
	if want := int64(fee) - skim; paid != want {
		t.Fatalf("redeem paid %d, want fee−skim = %d", paid, want)
	}
	if got := l.Balance(server); got != paid {
		t.Fatalf("server balance %d, want %d (exactly the conserved credit)", got, paid)
	}
	if got := l.EscrowBalance(obj); got != skim {
		t.Fatalf("object escrow %d, want the skim %d", got, skim)
	}
}

// TestSelfDeliveryPaysNothing: the same-identity guard, mirroring RecordServe —
// redeeming a delivery to yourself moves nothing.
func TestSelfDeliveryPaysNothing(t *testing.T) {
	l := New(50_000, 0)
	n := id(1)
	if paid := l.RedeemDeliveryCredit(n, n, id(7)); paid != 0 {
		t.Fatalf("self-delivery paid %d, want 0", paid)
	}
	if got := l.Balance(n); got != 0 {
		t.Fatalf("self-delivery moved balance to %d, want 0", got)
	}
}

// TestWitnessedReceiptSupersedesServeSelfRecord is the certification's
// load-bearing Q1b correction: the serve path self-records 1 credit/byte as it
// serves; a witnessed receipt for the same delivery must REPLACE that
// self-credit, not stack on it. After serve + redeem, the server's balance is
// exactly the conserved credit — as if the self-record never happened — and the
// escrow holds exactly the fee-skim (the serve-skim was reversed too).
func TestWitnessedReceiptSupersedesServeSelfRecord(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj, chunk := id(7), id(9)

	// The serve happens first: the server self-records bytes − skim.
	const bytes = 1 << 20
	l.RecordServeToObject(server, fetcher, obj, chunk, bytes)
	if got := l.Balance(server); got != bytes-bytes*SkimNum/SkimDen {
		t.Fatalf("setup: self-record balance %d", got)
	}

	// The witnessed receipt lands: supersede, then pay conserved.
	paid := l.RedeemDeliveryCredit(server, fetcher, obj)
	feeSkim := int64(fee) * SkimNum / SkimDen
	if want := int64(fee) - feeSkim; paid != want {
		t.Fatalf("redeem paid %d, want %d", paid, want)
	}
	if got := l.Balance(server); got != paid {
		t.Fatalf("balance after supersede = %d, want exactly the conserved credit %d "+
			"(the self-record must not survive a witnessed receipt — the banned double-pay)", got, paid)
	}
	if got := l.EscrowBalance(obj); got != feeSkim {
		t.Fatalf("escrow after supersede = %d, want the fee-skim %d (the serve-skim reversed)", got, feeSkim)
	}
	// servedBytes stays: the bytes moved; only the payment was superseded (S5).
	if got := l.ServedBytes(server); got != bytes {
		t.Fatalf("servedBytes = %d, want %d (observability must survive supersession)", got, bytes)
	}
}

// TestWashLoopIsAStrictLoss pins the certified anti-wash bound end-to-end in
// the ledger: a colluding pair (one operator, two identities, one ledger view)
// that pays the withdrawal fee, self-serves, and redeems its own receipt ends
// the loop worse off by exactly the fee-skim — wash is strictly loss-making,
// per loop, forever. This is the economic half of the B3 close.
func TestWashLoopIsAStrictLoss(t *testing.T) {
	const fee = 50_000
	l := New(fee, fee*10) // both identities granted working capital
	server, fetcher := id(1), id(2)
	obj, chunk := id(7), id(9)

	pairBefore := l.Balance(server) + l.Balance(fetcher)

	// The wash loop: fetcher pays the withdrawal fee (ChargePublish is what
	// tokenChargeFor invokes on a legacy-path withdrawal), the pair fakes or
	// performs the delivery (serve shown here — the worst case for the
	// defender, since it also mints the serve self-record), and the server
	// redeems the receipt.
	if err := l.ChargePublish(fetcher); err != nil {
		t.Fatalf("withdrawal fee: %v", err)
	}
	l.RecordServeToObject(server, fetcher, obj, chunk, 1<<10)
	l.RedeemDeliveryCredit(server, fetcher, obj)

	pairAfter := l.Balance(server) + l.Balance(fetcher)
	loss := pairBefore - pairAfter
	if wantMin := int64(fee) * SkimNum / SkimDen; loss < wantMin {
		t.Fatalf("wash loop lost the pair only %d, want ≥ the fee-skim %d — wash must be strictly loss-making", loss, wantMin)
	}
	if pairAfter >= pairBefore {
		t.Fatalf("wash loop was profitable or free (pair %d → %d) — conservation is broken", pairBefore, pairAfter)
	}
}

// TestUnwitnessedServeKeepsSelfRecord pins the fallback lane: with no receipt,
// the serve self-record stands untouched — the bilateral unwitnessed economy is
// unchanged by the PoD wiring.
func TestUnwitnessedServeKeepsSelfRecord(t *testing.T) {
	l := New(50_000, 0)
	server, fetcher := id(1), id(2)
	obj, chunk := id(7), id(9)

	const bytes = 1 << 20
	l.RecordServeToObject(server, fetcher, obj, chunk, bytes)
	want := int64(bytes) - int64(bytes)*SkimNum/SkimDen
	if got := l.Balance(server); got != want {
		t.Fatalf("unwitnessed serve balance %d, want %d", got, want)
	}
}

// TestPaidBountyIsNotRecoverableBySupersede is the PE-prescribed guard on the
// ESCROW skim-routing decision (RULING-PoD-keystone-owner-knobs-2026-08-26,
// knob 1). Escrow-routing is safe *because* the supersede reversal is floored at
// what the reserve still holds: credits already paid out as a repair bounty are
// real durability work and can never be clawed back. If that floor ever
// regressed, escrow would start minting recoverable balance — and burn would
// become the correct routing instead. This test is why the floor cannot regress
// silently (build-immutable #2).
func TestPaidBountyIsNotRecoverableBySupersede(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher, repairer := id(1), id(2), id(3)
	obj, chunk := id(7), id(9)

	// A serve routes its skim into the object's reserve…
	const bytes = 1 << 20
	l.RecordServeToObject(server, fetcher, obj, chunk, bytes)
	serveSkim := int64(bytes) * SkimNum / SkimDen
	if got := l.EscrowBalance(obj); got != serveSkim {
		t.Fatalf("setup: escrow %d, want %d", got, serveSkim)
	}

	// …and a repair bounty spends it before any receipt arrives. That credit is
	// now in the repairer's hands for real durability work.
	paidOut := l.PayBounty(obj, repairer, serveSkim)
	if paidOut != serveSkim || l.EscrowBalance(obj) != 0 {
		t.Fatalf("setup: bounty paid %d, escrow left %d", paidOut, l.EscrowBalance(obj))
	}
	repairerBal := l.Balance(repairer)

	// The witnessed receipt lands late. Supersede must NOT claw back the spent
	// bounty: the escrow reversal floors at the (now empty) reserve.
	l.RedeemDeliveryCredit(server, fetcher, obj)

	if got := l.Balance(repairer); got != repairerBal {
		t.Fatalf("supersede clawed back a paid bounty: repairer %d → %d — real repair work must be non-recoverable", repairerBal, got)
	}
	feeSkim := int64(fee) * SkimNum / SkimDen
	if got := l.EscrowBalance(obj); got != feeSkim {
		t.Fatalf("escrow = %d, want exactly the new fee-skim %d (no negative reserve, no clawback)", got, feeSkim)
	}
	if got := l.EscrowBalance(obj); got < 0 {
		t.Fatalf("escrow went negative (%d) — the reversal floor is gone", got)
	}
}

// TestPaidBountyIsNotRecoverableByEviction is the sibling guard on the NEW
// eviction-reversal site (A4 fix, Boulder 0). The eviction reversal shares the
// same floored-skim logic as redeem, so the same invariant must hold there: a
// repair bounty paid out between serve and EVICTION is real durability work and
// can never be clawed back. If the eviction reversal ever dropped the floor,
// evicting a lane whose skim was already spent would drive the reserve negative.
// This makes the floor at the eviction site non-regressible (build-immutable #2),
// exactly as TestPaidBountyIsNotRecoverableBySupersede does for the redeem site.
func TestPaidBountyIsNotRecoverableByEviction(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, repairer := id(1), id(3)
	obj, chunk := ports.HashBytes([]byte("evicted-floor-obj")), id(9)

	// Lane 0 serves and routes its skim into the object's reserve…
	first := ports.NodeID(ports.HashBytes([]byte("first-requester")))
	const bytes = 1 << 20
	l.RecordServeToObject(server, first, obj, chunk, bytes)
	serveSkim := int64(bytes) * SkimNum / SkimDen
	if got := l.EscrowBalance(obj); got != serveSkim {
		t.Fatalf("setup: escrow %d, want %d", got, serveSkim)
	}

	// …a repair bounty spends it before the lane is evicted. That credit is now in
	// the repairer's hands for real durability work.
	paidOut := l.PayBounty(obj, repairer, serveSkim)
	if paidOut != serveSkim || l.EscrowBalance(obj) != 0 {
		t.Fatalf("setup: bounty paid %d, escrow left %d", paidOut, l.EscrowBalance(obj))
	}
	repairerBal := l.Balance(repairer)

	// Flood the map past the cap so lane 0 is FIFO-evicted. Eviction reverses lane
	// 0's mint; its skim reversal must FLOOR at the (now empty) reserve — not claw
	// back the paid bounty and not drive the reserve negative. Use a fresh object
	// per flood lane so their skims don't refill obj's reserve.
	for i := 0; i < maxProvisional; i++ {
		req := ports.NodeID(ports.HashBytes([]byte{'r', byte(i), byte(i >> 8), byte(i >> 16)}))
		floodObj := ports.HashBytes([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		l.RecordServeToObject(server, req, floodObj, chunk, 8)
	}
	if _, present := l.provisional[provKey{requester: first, root: obj}]; present {
		t.Fatal("lane 0 was not evicted — flood did not push it out")
	}

	if got := l.Balance(repairer); got != repairerBal {
		t.Fatalf("eviction clawed back a paid bounty: repairer %d → %d — real repair work must be non-recoverable", repairerBal, got)
	}
	if got := l.EscrowBalance(obj); got != 0 {
		t.Fatalf("escrow = %d, want 0 (floored at the empty reserve, no clawback, no negative)", got)
	}
}

// TestProvisionalCapIsBoundedAndDeterministic drives the supersede tracker past
// its cap (build-immutable #8: bounded before fast) and pins the CORRECT rule (b)
// behavior after the A4 fix: eviction REVERSES the evicted lane's self-mint, so
// an evicted lane's later redeem pays the conserved leg ONLY and mints nothing —
// an evicted-then-redeemed lane equals a never-existed redeem. It also pins the
// bound (map never exceeds the cap) and FIFO determinism (lane 0 is the one
// evicted). It was previously an encoding of the buggy rule (c) — evicted redeem
// pays conserved WITHOUT reversing the retained mint, the double-pay — and is
// flipped here per the A4 design.
func TestProvisionalCapIsBoundedAndDeterministic(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server := id(1)
	obj, chunk := ports.HashBytes([]byte("evicted-obj")), id(9)

	// Lane 0 — the one that will be evicted.
	first := ports.NodeID(ports.HashBytes([]byte("first-requester")))
	const bytes = 1 << 10
	skim := int64(bytes) * SkimNum / SkimDen
	net := int64(bytes) - skim // the eager self-mint lane 0 recorded

	l.RecordServeToObject(server, first, obj, chunk, bytes)
	if got := l.Balance(server); got != net {
		t.Fatalf("setup: lane-0 serve credited %d, want the self-mint %d", got, net)
	}

	// Flood distinct lanes past the cap. Lane 0 (FIFO-oldest) is evicted; its
	// self-mint must be reversed at eviction under rule (b).
	for i := 0; i < maxProvisional; i++ {
		req := ports.NodeID(ports.HashBytes([]byte{'r', byte(i), byte(i >> 8), byte(i >> 16)}))
		l.RecordServeToObject(server, req, ports.HashBytes([]byte{byte(i), byte(i >> 8), byte(i >> 16)}), chunk, 8)
	}
	if got := len(l.provisional); got > maxProvisional {
		t.Fatalf("provisional map grew to %d, cap is %d — unbounded state on the floor box", got, maxProvisional)
	}
	// FIFO determinism: lane 0 is the evicted one, not some other lane.
	if _, present := l.provisional[provKey{requester: first, root: obj}]; present {
		t.Fatal("lane 0 was not FIFO-evicted — eviction order is non-deterministic")
	}

	// Rule (b): eviction reversed lane 0's self-mint. A redeem for the evicted
	// lane finds nothing to reverse and pays the conserved leg only. So the
	// server's balance moves by exactly +paid, and the pre-existing lane-0 mint
	// (net) is no longer on the books — the evicted-then-redeemed total equals a
	// never-existed redeem. Under the OLD bug the lane-0 mint would still be on
	// the balance, so balBefore would carry net and the redeem would stack on top.
	balBefore := l.Balance(server)
	paid := l.RedeemDeliveryCredit(server, first, obj)
	if got := l.Balance(server); got != balBefore+paid {
		t.Fatalf("evicted-lane redeem changed balance by %d, want +%d only (conserved leg, no double-pay)", got-balBefore, paid)
	}
	// The lane-0 self-mint must have been cleared by eviction, not still resident.
	// Compare against a fresh ledger where lane 0 NEVER served: same flood, same
	// redeem, same terminal server balance. Evicted-then-redeemed == never-existed.
	ref := New(fee, 0)
	for i := 0; i < maxProvisional; i++ {
		req := ports.NodeID(ports.HashBytes([]byte{'r', byte(i), byte(i >> 8), byte(i >> 16)}))
		ref.RecordServeToObject(server, req, ports.HashBytes([]byte{byte(i), byte(i >> 8), byte(i >> 16)}), chunk, 8)
	}
	refPaid := ref.RedeemDeliveryCredit(server, first, obj)
	if l.Balance(server) != ref.Balance(server) || paid != refPaid {
		t.Fatalf("evicted-then-redeemed server balance %d (paid %d) != never-existed %d (paid %d) — lane-0 mint was not cleared by eviction",
			l.Balance(server), paid, ref.Balance(server), refPaid)
	}
}
