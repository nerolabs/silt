package credit

// The PoD relay-lane ledger tests. Relay compensation settles a PayWord chain
// into the relay operator's BALANCE, bounded by the prepayment anchors the
// fetcher's durable identity burned on THIS ledger (settled ≤ Σ face). It is
// the sibling of RedeemDeliveryCredit: never a mint, never touches
// Reputation.
//
// The three pair-total tests (formerly TestRelayCreditIsConserved,
// TestRelayWashLoopIsAWashNeverAGain, TestRelayRedeemDrawsFromFetcherPaidCredit and
// TestRelayRedeemCannotExceedPaidInBudget) are REWRITTEN to the LEDGER-TOTAL oracle
// (sumConserved, T-2's oracle), not extended
// and §10. A pair total (relay +
// fetcher) is blind to a mint that lands on a THIRD account and to a burn that happened
// on a durable buyer; the ledger total is not. None of the rewritten tests call fund:
// the buyer's only balance is the shipped faucet grant and the burn is the REAL
// ChargePublish (buyAnchors, relay_anchor_test.go). The stubs those tests depend on
// (spendRelayAnchors et al.) live in relay_anchor_test.go.
//
// The interim had re-specified these to "pays 0"; that was the recorded goalpost
// move of a NARROWING. This is the recorded goalpost move of the FIX: the lane
// pays again, bounded by the anchor. Each test keeps its name.
//
// TestSelfRelayPaysNothing is UNCHANGED. TestRelayCreditNeverTouchesStanding has
// its `paid > 0` precondition RESTORED (the interim's recorded obligation
// build-checked).

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// fund gives a node a starting balance for the interim tests
// (relay_interim_test.go), which model a funded-but-unanchored fetcher. It
// reaches into the ledger's own package internals (white-box). The gates never
// call it.
func fund(l *Ledger, n ports.NodeID, amount int64) {
	l.acct(n).balance += amount
}

// TestRelayCreditNeverTouchesStanding is the §7.3 firewall test, the sibling of
// TestDeliveryCreditNeverTouchesStanding: a relay that settles a huge volume of
// PayWord increments sees its Reputation unchanged. A PayWord chain is fundable
// with zero object bytes by design (it pays for forwarding, which is
// unprovable), so relay credit buying even one unit of standing would convert
// funded chains into consensus weight — the γ→1/N firewall.
//
// RESTORES THE `paid > 0` PRECONDITION: under the interim this test pressed
// a return-0 body and its standing assertion was VACUOUS. Now every settle must
// pay (> 0) or the firewall was never exercised. It also presses the NEW neutral
// method (SpendRelayAnchors, via the stub) once, and pins that a settlement
// touches no account but the relay's (the ephemeral is never registered).
//
// The `budget` argument is passed directly per settle: at the ledger tier the
// budget is the node's Σ face, trusted by contract. Pressing one
// anchor's face 65,537 times is NOT a conservation claim (that is the gate); it is the
// volume the firewall must hold under.
func TestRelayCreditNeverTouchesStanding(t *testing.T) {
	l := New(shippedFee, shippedGrant)
	relay, buyer, eph := id(1), id(2), id(3)

	before := l.Reputation(relay)
	l.Register(buyer)
	anchors := buyAnchors(t, l, buyer, 0, 0, 1)
	face, reason := l.SpendRelayAnchors(anchors) // the new neutral press
	if face != shippedFee || reason != "" {
		t.Fatalf("SpendRelayAnchors = (%d, %q), want (%d, \"\") — the anchored session never opened, so the firewall would be pressed by a pay-0 body again", face, reason, shippedFee)
	}
	if got := l.Reputation(relay); got != before {
		t.Fatalf("the anchor spend moved standing: Reputation %d → %d", before, got)
	}
	accountsBefore := len(l.accounts) // relay (via Reputation) and buyer are registered; settlement must add nothing

	// Iterate ABOVE bondUnit (64<<10 = 65,536). Reputation is integer bonded
	// points = bondedBytes / bondUnit, so a hypothetical +1-byte-per-call
	// standing leak stays sub-threshold and invisible under 1,000
	// iterations. Running past bondUnit forces any per-call accumulating
	// leak to cross one whole point and redden the assertion — a
	// sub-threshold leak cannot hide.
	const iters = (64 << 10) + 1 // 65,537 > bondUnit
	for i := 0; i < iters; i++ {
		if paid := l.RedeemRelayCredit(relay, eph, shippedFee, face); paid <= 0 {
			t.Fatalf("iteration %d: RedeemRelayCredit paid %d against budget %d, want > 0 — the firewall test is vacuous on a pay-0 body (the obligation)", i, paid, face)
		}
	}
	if got := l.Reputation(relay); got != before {
		t.Fatalf("relay credit moved standing: Reputation %d → %d — the γ→1/N firewall is breached", before, got)
	}
	if got := len(l.accounts); got != accountsBefore {
		t.Fatalf("settlement changed the account set %d → %d — only acct(relay), already registered, may be touched", accountsBefore, got)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatal("the fetcher's ephemeral was registered by a settlement — acct(ephID) was touched")
	}
}

// TestRelayCreditIsConserved (ledger-total form): one anchor bought by the durable
// buyer through the real burn, spent, then a partially consumed chain settled. The
// relay receives exactly min(c, face); the buyer's balance is unchanged by the
// settle (the burn already happened at issuance); the ephemeral is never
// registered; and Δ Σ_L = settled − face < 0 (partial consumption is a burn of the
// remainder). RED on main (pays 0).
func TestRelayCreditIsConserved(t *testing.T) {
	const chainValue = int64(30_000) // < face: partially consumed
	l := New(shippedFee, shippedGrant)
	relay, buyer, eph := id(1), id(2), id(3)
	l.Register(relay)
	l.Register(buyer)
	before := sumConserved(l)

	anchors := buyAnchors(t, l, buyer, 0, 0, 1)
	buyerAfterBurn := l.Balance(buyer)
	face, reason := l.SpendRelayAnchors(anchors)
	if face != shippedFee || reason != "" {
		t.Fatalf("SpendRelayAnchors = (%d, %q), want (%d, \"\")", face, reason, shippedFee)
	}
	relayBefore := l.Balance(relay)

	paid := l.RedeemRelayCredit(relay, eph, chainValue, face)
	if paid != chainValue {
		t.Fatalf("redeem paid %d, want the consumed chain value %d (≤ face %d)", paid, chainValue, face)
	}
	if got := l.Balance(relay); got != relayBefore+chainValue {
		t.Fatalf("relay balance %d, want %d + %d", got, relayBefore, chainValue)
	}
	if got := l.Balance(buyer); got != buyerAfterBurn {
		t.Fatalf("the settle moved the buyer's balance %d → %d — settlement draws from the anchor, not from a live account", buyerAfterBurn, got)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatal("the ephemeral was registered by the settlement")
	}
	if got := sumConserved(l) - before; got != chainValue-shippedFee {
		t.Fatalf("Δ Σ_L = %d, want settled − face = %d (the unconsumed remainder is burned, never re-minted)", got, chainValue-shippedFee)
	}
}

// TestSelfRelayPaysNothing: the same-identity guard, mirroring RedeemDelivery —
// relaying to yourself moves nothing. UNCHANGED across the interim and.
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

// TestRelayWashLoopIsAWashNeverAGain (ledger-total form; the name is kept, the
// property is CORRECTED per /): the collusion loop — the relay operator's
// durable identity D buys anchors on the relay's own ledger and the operator's
// ephemeral settles them back — is NEVER a gain: Δ Σ_L = settled − Σ face ≤ 0.
// At full consumption it is a WASH (Δ = 0, no relay skim in v1, anbefore); at
// any partial consumption it is a strict loss. What it must never be is Δ > 0.
// RED on main (pays 0, so the full-consumption case is −face, not 0).
func TestRelayWashLoopIsAWashNeverAGain(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    int64
	}{
		{"full consumption is a wash", shippedFee},
		{"over-long chain is a wash", shippedFee + 12_345},
		{"partial consumption is a strict loss", shippedFee - 1},
		{"stall after admit is a strict loss", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := New(shippedFee, shippedGrant)
			relay, operatorDurable, operatorEph := id(1), id(2), id(3)
			l.Register(relay)
			l.Register(operatorDurable)
			before := sumConserved(l)
			pairBefore := l.Balance(relay) + l.Balance(operatorDurable)

			anchors := buyAnchors(t, l, operatorDurable, 0, 0, 1)
			face, _ := l.SpendRelayAnchors(anchors)
			paid := l.RedeemRelayCredit(relay, operatorEph, tc.c, face)
			want := minI64(tc.c, shippedFee)
			if paid != want {
				t.Fatalf("wash-loop redeem paid %d, want min(c, face) = %d", paid, want)
			}
			delta := sumConserved(l) - before
			if delta > 0 {
				t.Fatalf("the collusion loop GAINED %d on the ledger — a mint, the banned dual", delta)
			}
			if delta != want-shippedFee {
				t.Fatalf("Δ Σ_L = %d, want settled − face = %d", delta, want-shippedFee)
			}
			if pairAfter := l.Balance(relay) + l.Balance(operatorDurable); pairAfter-pairBefore != delta {
				t.Fatalf("the operator's combined position moved %d but the ledger moved %d — credit appeared on or vanished from a third account", pairAfter-pairBefore, delta)
			}
			if want == shippedFee && delta != 0 {
				t.Fatalf("full consumption is a WASH (Δ = 0, no v1 skim), got Δ = %d", delta)
			}
			if want < shippedFee && delta >= 0 {
				t.Fatalf("partial consumption must be a STRICT loss, got Δ = %d", delta)
			}
		})
	}
}

// TestRelayRedeemDrawsFromFetcherPaidCredit (ledger-total form): the settle
// draws from the anchor's ALREADY-BURNED face, not from any live account. After
// the burn, no balance but the relay's moves at settle; the ephemeral (the M0
// fresh identity) is never registered; and the total's only move at settle is
// +settled. RED on main (pays 0).
func TestRelayRedeemDrawsFromFetcherPaidCredit(t *testing.T) {
	const chainValue = int64(20_000)
	l := New(shippedFee, shippedGrant)
	relay, buyer, eph := id(1), id(2), id(3)
	l.Register(relay)
	l.Register(buyer)

	anchors := buyAnchors(t, l, buyer, 0, 0, 1)
	face, _ := l.SpendRelayAnchors(anchors)
	buyerAfterBurn := l.Balance(buyer)
	totalAfterSpend := sumConserved(l)

	paid := l.RedeemRelayCredit(relay, eph, chainValue, face)
	if paid != chainValue {
		t.Fatalf("redeem paid %d, want %d", paid, chainValue)
	}
	if got := l.Balance(buyer); got != buyerAfterBurn {
		t.Fatalf("the buyer's balance moved %d → %d at settle — settlement must draw from the burned face, not a live balance", buyerAfterBurn, got)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatal("the ephemeral was registered by the settlement — the phantom-grant fiction")
	}
	if got := sumConserved(l) - totalAfterSpend; got != paid {
		t.Fatalf("the total moved by %d at settle, want exactly settled = %d", got, paid)
	}
}

// TestRelayRedeemCannotExceedPaidInBudget (ledger-total form): the budget is the
// ledger's Σ face. A chain value ABOVE it settles for exactly the budget (the
// remainder of the chain is unfunded — min(c, budget)), never more;
// at exactly the budget it settles in full (the cap is inclusive). In both cases
// Δ Σ_L = 0 (full consumption). RED on main (pays 0).
func TestRelayRedeemCannotExceedPaidInBudget(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    int64
	}{
		{"above budget settles the budget", shippedFee + 1},
		{"far above budget settles the budget", relayChainMaxCredit},
		{"at budget settles in full", shippedFee},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := New(shippedFee, shippedGrant)
			relay, buyer, eph := id(1), id(2), id(3)
			l.Register(relay)
			l.Register(buyer)
			before := sumConserved(l)

			anchors := buyAnchors(t, l, buyer, 0, 0, 1)
			budget, _ := l.SpendRelayAnchors(anchors)
			relayBefore := l.Balance(relay)

			paid := l.RedeemRelayCredit(relay, eph, tc.c, budget)
			if paid != budget {
				t.Fatalf("redeem of chain value %d against budget %d paid %d, want exactly the budget", tc.c, budget, paid)
			}
			if got := l.Balance(relay); got != relayBefore+budget {
				t.Fatalf("relay balance %d, want %d + budget %d", got, relayBefore, budget)
			}
			if got := sumConserved(l); got != before {
				t.Fatalf("Δ Σ_L = %d at full consumption, want 0 (C-1: equality iff fully consumed)", got-before)
			}
		})
	}
}
