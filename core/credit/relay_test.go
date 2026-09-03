package credit

// The PoD relay-lane ledger tests (docs/design/pod.md §7.3, certified
// 2026-08-30). Relay compensation settles a PayWord chain into the relay
// operator's BALANCE, drawn from the fetcher's already-paid blind credit. It is
// the sibling of RedeemDeliveryCredit: a conserved transfer, never a mint, that
// never touches Reputation(). These tests mirror the delivery-lane suite:
// the firewall (a heavy relay's standing never moves), conservation (credit ≤
// the chain value, a transfer never a mint), and the wash bound (a colluding
// pair's loop is a strict loss).
//
// R0.7 INTERIM (2026-09-03): the positive-settlement tests below are
// RE-SPECIFIED to "pays 0 until R2.14" per
// RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §9 step 1 — the certified finding that `RedeemRelayCredit`'s conservation
// claim does not hold on the shipped per-node-ledger topology (no anchor
// exists), so the only honest settlement until the R2.14 prepayment anchor
// ships is 0. This is a PRESCRIBED goalpost move (recorded here, not silent):
// each re-specified test keeps its name and now asserts the interim rather
// than its former "conserved transfer" shape. See also
// core/credit/r07_relay_interim_test.go for the new G-RI-1 gates and
// docs/thinking/2026-09-03-r0.7-relay-interim-design.md.
//
// TestSelfRelayPaysNothing and TestRelayCreditNeverTouchesStanding are
// UNCHANGED — both are still true under the interim (self-relay pays 0 either
// way; the firewall property does not depend on how much is paid).

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// fund gives a node a starting balance for the relay-lane tests, modelling the
// blind credit the fetcher already paid for and that funds the PayWord chain.
// It reaches into the ledger's own package internals (this is a white-box test).
func fund(l *Ledger, n ports.NodeID, amount int64) {
	l.acct(n).balance += amount
}

// TestRelayCreditNeverTouchesStanding is the §7.3 firewall test, the sibling of
// TestDeliveryCreditNeverTouchesStanding: a relay that settles a huge volume of
// PayWord increments sees its Reputation() unchanged. A PayWord chain is
// mintable with zero object bytes by certified design (it pays for forwarding,
// which is unprovable), so relay credit buying even one unit of standing would
// convert funded chains into consensus weight — the γ→1/N firewall.
//
// UNCHANGED by the R0.7 interim: the firewall property (Reputation stays flat)
// holds trivially whether RedeemRelayCredit pays >0 or 0, so this test is left
// asserting `paid <= 0` is never > 0 in either shape — it does not depend on
// the pay-0 goalpost move at all. (It still calls RedeemRelayCredit and does
// not assert on its return value beyond that it never raises Reputation.)
func TestRelayCreditNeverTouchesStanding(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	relay, fetcher := id(1), id(2)

	before := l.Reputation(relay)
	// Iterate ABOVE bondUnit (64<<10 = 65,536). Reputation() is integer bonded
	// points = bondedBytes / bondUnit, so a hypothetical +1-byte-per-call standing
	// leak stays sub-threshold and invisible under 1,000 iterations. Running past
	// bondUnit forces any per-call accumulating leak to cross one whole point and
	// redden the assertion (Tester hardening) — a sub-threshold leak cannot hide.
	const iters = (64 << 10) + 1 // 65,537 > bondUnit
	for i := 0; i < iters; i++ { // a heavy relay volume, past the standing quantum
		l.RedeemRelayCredit(relay, fetcher, fee, fee)
	}
	if got := l.Reputation(relay); got != before {
		t.Fatalf("relay credit moved standing: Reputation %d → %d — the γ→1/N firewall is breached", before, got)
	}
}

// TestRelayCreditIsConserved is RE-SPECIFIED to the R0.7 interim (cert §9
// step 1): with no anchor type in existence, RedeemRelayCredit pays 0 and
// moves NOTHING — not even the fetcher's already-paid-in balance modelled by
// `fund()`. Before R2.14: this used to pin "the relay receives exactly the
// chain value, the fetcher's tab falls by the same" — that shape is refuted
// (RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §2.5: the comment's own conservation claim "does not hold on the shipped
// path"). RED now: main still pays the full chainValue.
func TestRelayCreditIsConserved(t *testing.T) {
	const fee = 50_000
	const chainValue = 30_000
	l := New(fee, 0)
	relay, fetcher := id(1), id(2)

	// The fetcher funds the chain from its already-paid blind credit: model the
	// paid-in budget as a positive fetcher balance the redeem WOULD draw down,
	// if an anchor existed. It does not, so nothing may be drawn.
	fund(l, fetcher, chainValue)
	fetcherBefore := l.Balance(fetcher)

	paid := l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if paid != 0 {
		t.Fatalf("redeem paid %d, want 0 (interim: no anchor type exists — R2.14)", paid)
	}
	if got := l.Balance(relay); got != 0 {
		t.Fatalf("relay balance %d, want 0 — the interim moves nothing until the R2.14 anchor lands", got)
	}
	if got := l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("fetcher balance %d, want %d (unanchored settlement must not draw down the fetcher's paid-in credit)", got, fetcherBefore)
	}
}

// TestSelfRelayPaysNothing: the same-identity guard, mirroring RedeemDelivery —
// relaying to yourself moves nothing. UNCHANGED: still true under the interim.
func TestSelfRelayPaysNothing(t *testing.T) {
	l := New(50_000, 0)
	n := id(1)
	if paid := l.RedeemRelayCredit(n, n, 30_000, 30_000); paid != 0 {
		t.Fatalf("self-relay paid %d, want 0", paid)
	}
	if got := l.Balance(n); got != 0 {
		t.Fatalf("self-relay moved balance to %d, want 0", got)
	}
}

// TestRelayWashLoopIsAStrictLoss is RE-SPECIFIED to the R0.7 interim (cert §9
// step 1): a colluding pair funds a chain and redeems it against itself; under
// the interim the redeem pays 0, so the pair's total is EXACTLY unchanged (a
// strictly stronger statement than the pre-interim "never profitable" bound,
// which allowed the pair to break even at the chain value). RED now: main
// still pays chainValue, so the pair's total moves (up by chainValue, still
// not counted as "profit" against the fetcher's own prior fund() credit, but
// it is no longer an unchanged total, which is what the interim requires).
func TestRelayWashLoopIsAStrictLoss(t *testing.T) {
	const chainValue = 30_000
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)

	// The fetcher paid the withdrawal fee for the blind credit that funds the
	// chain; model that already-paid budget.
	fund(l, fetcher, chainValue)
	pairBefore := l.Balance(relay) + l.Balance(fetcher)

	paid := l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if paid != 0 {
		t.Fatalf("wash-loop redeem paid %d, want 0 (interim: no anchor exists)", paid)
	}

	pairAfter := l.Balance(relay) + l.Balance(fetcher)
	if pairAfter != pairBefore {
		t.Fatalf("relay wash loop moved the pair total %d → %d — the interim (pay 0, move nothing) is violated", pairBefore, pairAfter)
	}
}

// TestRelayRedeemDrawsFromFetcherPaidCredit is RE-SPECIFIED to the R0.7
// interim (cert §9 step 1): the pair-total invariant this test pins still
// holds under pay-0 (trivially: 0 moved, total unchanged), but the test now
// ALSO asserts paid==0 directly, since "the pair total is unchanged" alone
// would pass equally for a conserved nonzero transfer OR for pay-0 — the
// interim specifically requires the latter. RED now: main pays chainValue.
func TestRelayRedeemDrawsFromFetcherPaidCredit(t *testing.T) {
	const chainValue = 20_000
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)
	fund(l, fetcher, chainValue)

	total := l.Balance(relay) + l.Balance(fetcher)
	paid := l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if paid != 0 {
		t.Fatalf("redeem paid %d, want 0 (interim: no anchor type exists — R2.14)", paid)
	}
	if got := l.Balance(relay) + l.Balance(fetcher); got != total {
		t.Fatalf("relay redeem changed the pair total %d → %d — it must move nothing under the interim", total, got)
	}
}

// TestRelayRedeemCannotExceedPaidInBudget is RE-SPECIFIED to the R0.7 interim
// (cert §9 step 1): the conservation-cap property this test pinned (never
// redeem past the funded budget) is now the WEAKER of two floors — the
// interim floor is stronger (never redeem AT ALL, even at exactly the
// budget). Both the over-budget and the at-budget cases must pay 0. RED now on
// the at-budget case: main still settles it in full.
func TestRelayRedeemCannotExceedPaidInBudget(t *testing.T) {
	const budget = 20_000 // the fetcher's paid-in blind credit = S × increment
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)
	fund(l, fetcher, budget)

	relayBefore := l.Balance(relay)
	fetcherBefore := l.Balance(fetcher)

	// A relay claims a chain value ABOVE the funded budget — over-redemption.
	// This was already rejected before the interim (paid 0) and stays rejected.
	over := int64(budget + 1)
	if paid := l.RedeemRelayCredit(relay, fetcher, over, budget); paid != 0 {
		t.Fatalf("redeem of %d against budget %d paid %d, want 0 — the relay over-redeemed past the fetcher's paid-in credit", over, budget, paid)
	}
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved to %d on a rejected over-redeem, want %d", got, relayBefore)
	}
	if got := l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("fetcher balance moved to %d on a rejected over-redeem, want %d", got, fetcherBefore)
	}

	// AT exactly the budget: pre-interim this settled in full ("the cap is
	// inclusive, not off-by-one"). Under the interim it must ALSO pay 0 — no
	// anchor exists, so being within budget is no longer sufficient.
	if paid := l.RedeemRelayCredit(relay, fetcher, budget, budget); paid != 0 {
		t.Fatalf("redeem at exactly the budget %d paid %d, want 0 (interim: no anchor type exists — R2.14)", budget, paid)
	}
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved to %d on an at-budget unanchored redeem, want %d", got, relayBefore)
	}
	if got := l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("fetcher balance moved to %d on an at-budget unanchored redeem, want %d", got, fetcherBefore)
	}
}
