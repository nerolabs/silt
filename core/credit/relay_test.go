package credit

// The PoD relay-lane ledger tests (docs/design/pod.md §7.3, certified
// 2026-08-30). Relay compensation settles a PayWord chain into the relay
// operator's BALANCE, drawn from the fetcher's already-paid blind credit. It is
// the sibling of RedeemDeliveryCredit: a conserved transfer, never a mint, that
// never touches Reputation(). These tests mirror the delivery-lane suite:
// the firewall (a heavy relay's standing never moves), conservation (credit ≤
// the chain value, a transfer never a mint), and the wash bound (a colluding
// pair's loop is a strict loss).

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
		if paid := l.RedeemRelayCredit(relay, fetcher, fee, fee); paid <= 0 {
			t.Fatalf("redeem %d paid %d, want > 0", i, paid)
		}
	}
	if got := l.Reputation(relay); got != before {
		t.Fatalf("relay credit moved standing: Reputation %d → %d — the γ→1/N firewall is breached", before, got)
	}
}

// TestRelayCreditIsConserved pins the conservation arithmetic: one redeem pays
// the relay exactly the chain value drawn from the fetcher's paid-in credit,
// nothing minted. v1 ships without a relay skim (design §6), so the relay
// receives the full chainValue and the fetcher's tab decreases by the same.
func TestRelayCreditIsConserved(t *testing.T) {
	const fee = 50_000
	const chainValue = 30_000
	l := New(fee, 0)
	relay, fetcher := id(1), id(2)

	// The fetcher funds the chain from its already-paid blind credit: model the
	// paid-in budget as a positive fetcher balance the redeem draws down.
	fund(l, fetcher, chainValue)
	fetcherBefore := l.Balance(fetcher)

	paid := l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if paid != chainValue {
		t.Fatalf("redeem paid %d, want the chain value %d (no skim in v1)", paid, chainValue)
	}
	if got := l.Balance(relay); got != chainValue {
		t.Fatalf("relay balance %d, want exactly the conserved credit %d", got, chainValue)
	}
	if got := l.Balance(fetcher); got != fetcherBefore-chainValue {
		t.Fatalf("fetcher balance %d, want %d (the chain value drawn from its paid-in credit)", got, fetcherBefore-chainValue)
	}
	// Conservation: the pair's total is unchanged — a transfer, never a mint.
	if got := l.Balance(relay) + l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("relay+fetcher total %d, want %d — relay credit minted or destroyed value", got, fetcherBefore)
	}
}

// TestSelfRelayPaysNothing: the same-identity guard, mirroring RedeemDelivery —
// relaying to yourself moves nothing.
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

// TestRelayWashLoopIsAStrictLoss pins the anti-wash bound: a colluding pair
// (one operator, two identities, one ledger view) that funds a chain and
// redeems it against itself ends no better off — the chain value is drawn from
// the fetcher's paid-in credit and lands in the relay's, a strict conservation
// zero-sum with no mint. There is no way for the loop to profit.
func TestRelayWashLoopIsAStrictLoss(t *testing.T) {
	const chainValue = 30_000
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)

	// The fetcher paid the withdrawal fee for the blind credit that funds the
	// chain; model that already-paid budget.
	fund(l, fetcher, chainValue)
	pairBefore := l.Balance(relay) + l.Balance(fetcher)

	l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)

	pairAfter := l.Balance(relay) + l.Balance(fetcher)
	// Conservation: the loop never mints. The pair is no better off than before
	// (break-even on a neutral balance; the withdrawal fee they already paid for
	// the credit is the strict loss, exactly as the delivery leg).
	if pairAfter > pairBefore {
		t.Fatalf("relay wash loop was profitable (pair %d → %d) — conservation is broken", pairBefore, pairAfter)
	}
}

// TestRelayRedeemDrawsFromFetcherPaidCredit: the settlement moves balance ONLY
// and is drawn from the fetcher's paid-in credit — never a mint. This is the
// certification's §2a application to the relay leg.
func TestRelayRedeemDrawsFromFetcherPaidCredit(t *testing.T) {
	const chainValue = 20_000
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)
	fund(l, fetcher, chainValue)

	total := l.Balance(relay) + l.Balance(fetcher)
	l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if got := l.Balance(relay) + l.Balance(fetcher); got != total {
		t.Fatalf("relay redeem changed the pair total %d → %d — it must be a pure transfer", total, got)
	}
}

// TestRelayRedeemCannotExceedPaidInBudget is the conservation-cap test (PE
// ruling H-1/Q2 coupling): RedeemRelayCredit must never let the relay redeem
// more than the fetcher actually funded. The chain's committed budget is
// S × increment, itself bounded by the fetcher's paid-in blind credit. A redeem
// whose chainValue exceeds that budget must be rejected (pay 0), so a future
// transport caller cannot over-redeem and drive a balance negative. Removing the
// cap in RedeemRelayCredit turns this RED.
func TestRelayRedeemCannotExceedPaidInBudget(t *testing.T) {
	const budget = 20_000 // the fetcher's paid-in blind credit = S × increment
	l := New(50_000, 0)
	relay, fetcher := id(1), id(2)
	fund(l, fetcher, budget)

	relayBefore := l.Balance(relay)
	fetcherBefore := l.Balance(fetcher)

	// A relay claims a chain value ABOVE the funded budget — over-redemption.
	over := int64(budget + 1)
	if paid := l.RedeemRelayCredit(relay, fetcher, over, budget); paid != 0 {
		t.Fatalf("redeem of %d against budget %d paid %d, want 0 — the relay over-redeemed past the fetcher's paid-in credit", over, budget, paid)
	}
	// No value moved: the cap rejected, it did not partially settle.
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved to %d on a rejected over-redeem, want %d", got, relayBefore)
	}
	if got := l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("fetcher balance moved to %d on a rejected over-redeem, want %d", got, fetcherBefore)
	}
	// At exactly the budget it settles — the cap is inclusive, not off-by-one.
	if paid := l.RedeemRelayCredit(relay, fetcher, budget, budget); paid != budget {
		t.Fatalf("redeem at exactly the budget %d paid %d, want %d", budget, paid, budget)
	}
}
