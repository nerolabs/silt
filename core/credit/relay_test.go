package credit

// The PoD relay-lane ledger tests. Relay compensation settles a PayWord chain
// into the relay operator's BALANCE, bounded by the prepayment anchors the
// fetcher's durable identity burned on THIS ledger, less the relay burn:
// settled = RelayNet(min(chainValue, Σ face)) < Σ face. It is the sibling of
// the delivery settlement: never a mint, never touches Reputation.
//
// TestRelayCreditIsConserved, TestRelayWashLoopIsAStrictLoss,
// TestRelayRedeemDrawsFromFetcherPaidCredit and
// TestRelayRedeemCannotExceedPaidInBudget use the LEDGER-TOTAL oracle
// (sumConserved), not a pair total. A pair total (relay + fetcher) is blind to a
// mint that lands on a THIRD account and to a burn that happened on a durable
// buyer; the ledger total is not. None of them call fund: the buyer's only
// balance is the shipped faucet grant and the burn is the REAL ChargePublish
// (buyAnchors, relay_anchor_test.go).

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
// Every settle must pay (> 0): on a pay-0 body the standing assertion would be
// vacuous and the firewall never exercised. It also presses the NEW neutral
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
// relay receives exactly RelayNet(min(c, face)); the buyer's balance is unchanged
// by the settle (the burn already happened at issuance); the ephemeral is never
// registered; and supply is accounted for exactly: before = after + relay burn +
// unconsumed remainder, so Δ Σ_L = settled − face < 0.
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
	burnedBefore := l.RelayBurnedCredits()

	paid := l.RedeemRelayCredit(relay, eph, chainValue, face)
	burned := l.RelayBurnedCredits() - burnedBefore
	if want := RelayNet(chainValue); paid != want {
		t.Fatalf("redeem paid %d, want RelayNet(consumed chain value %d) = %d (≤ face %d)", paid, chainValue, want, face)
	}
	if burned != chainValue-paid {
		t.Fatalf("relay burn moved by %d, want chain value − paid = %d", burned, chainValue-paid)
	}
	if got := l.Balance(relay); got != relayBefore+paid {
		t.Fatalf("relay balance %d, want %d + %d", got, relayBefore, paid)
	}
	if got := l.Balance(buyer); got != buyerAfterBurn {
		t.Fatalf("the settle moved the buyer's balance %d → %d — settlement draws from the anchor, not from a live account", buyerAfterBurn, got)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatal("the ephemeral was registered by the settlement")
	}
	after := sumConserved(l)
	if before != after+burned+(shippedFee-chainValue) {
		t.Fatalf("supply before %d != after %d + relay burn %d + unconsumed remainder %d (the remainder is burned, never re-minted)", before, after, burned, shippedFee-chainValue)
	}
	if got := after - before; got != paid-shippedFee {
		t.Fatalf("Δ Σ_L = %d, want settled − face = %d", got, paid-shippedFee)
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

// TestRelayWashLoopIsAStrictLoss (ledger-total form): the collusion loop — the
// relay operator's durable identity D buys anchors on the relay's own ledger and
// the operator's ephemeral settles them back — is a strict loss at EVERY
// consumption level: Δ = settled − Σ face < 0. The relay burn takes at least one
// credit from every non-zero settlement, so even full consumption and an
// over-long chain lose RelayBurnNum/RelayBurnDen of the face (rounded up); a
// partial consumption also loses the unconsumed remainder. A wash that breaks
// even would be free volume.
func TestRelayWashLoopIsAStrictLoss(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    int64
	}{
		{"full consumption is a strict loss", shippedFee},
		{"over-long chain is a strict loss", shippedFee + 12_345},
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
			burnedBefore := l.RelayBurnedCredits()
			paid := l.RedeemRelayCredit(relay, operatorEph, tc.c, face)
			burned := l.RelayBurnedCredits() - burnedBefore
			gross := minI64(tc.c, shippedFee)
			want := RelayNet(gross)
			if paid != want {
				t.Fatalf("wash-loop redeem paid %d, want RelayNet(min(c, face)) = %d", paid, want)
			}
			if burned != gross-paid || burned < 1 {
				t.Fatalf("relay burn moved by %d, want gross − paid = %d ≥ 1", burned, gross-paid)
			}
			delta := sumConserved(l) - before
			if delta != want-shippedFee {
				t.Fatalf("Δ Σ_L = %d, want settled − face = %d", delta, want-shippedFee)
			}
			if pairAfter := l.Balance(relay) + l.Balance(operatorDurable); pairAfter-pairBefore != delta {
				t.Fatalf("the operator's combined position moved %d but the ledger moved %d — credit appeared on or vanished from a third account", pairAfter-pairBefore, delta)
			}
			if delta >= 0 {
				t.Fatalf("the collusion loop must be a STRICT loss at every consumption level, got Δ = %d (c = %d)", delta, tc.c)
			}
		})
	}
}

// TestRelayRedeemDrawsFromFetcherPaidCredit (ledger-total form): the settle
// draws from the anchor's ALREADY-BURNED face, not from any live account. After
// the burn, no balance but the relay's moves at settle; the ephemeral (the
// fresh identity) is never registered; and the total's only move at settle is
// +settled, with settled = RelayNet(chain value).
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
	if want := RelayNet(chainValue); paid != want {
		t.Fatalf("redeem paid %d, want RelayNet(%d) = %d", paid, chainValue, want)
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
// ledger's Σ face. A chain value ABOVE it settles for exactly RelayNet(budget) (the
// remainder of the chain is unfunded — min(c, budget)), never more; at exactly
// the budget it settles in full (the cap is inclusive). In every case the whole
// budget is consumed, so the only supply that leaves the ledger is the relay
// burn: before = after + burned.
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
			burnedBefore := l.RelayBurnedCredits()

			paid := l.RedeemRelayCredit(relay, eph, tc.c, budget)
			burned := l.RelayBurnedCredits() - burnedBefore
			if want := RelayNet(budget); paid != want {
				t.Fatalf("redeem of chain value %d against budget %d paid %d, want exactly RelayNet(budget) = %d", tc.c, budget, paid, want)
			}
			if got := l.Balance(relay); got != relayBefore+paid {
				t.Fatalf("relay balance %d, want %d + paid %d", got, relayBefore, paid)
			}
			if burned != budget-paid {
				t.Fatalf("relay burn moved by %d, want budget − paid = %d", burned, budget-paid)
			}
			if after := sumConserved(l); before != after+burned {
				t.Fatalf("supply before %d != after %d + relay burn %d at full consumption", before, after, burned)
			}
		})
	}
}

// TestRelayBurnIsAtLeastOneCreditOnEverySettlement pins RelayNet across small and
// large gross values: the relay is paid strictly less than gross, never a
// negative amount, every non-zero settlement burns at least one credit, and the
// burn never exceeds one eighth of gross rounded up.
func TestRelayBurnIsAtLeastOneCreditOnEverySettlement(t *testing.T) {
	var grosses []int64
	for g := int64(1); g <= 64; g++ {
		grosses = append(grosses, g)
	}
	grosses = append(grosses, shippedFee-1, shippedFee, shippedFee+1, 6*shippedFee, 1<<40, 1<<59)
	for _, gross := range grosses {
		paid := RelayNet(gross)
		burned := gross - paid
		maxBurn := gross / 8
		if gross%8 != 0 {
			maxBurn++
		}
		if paid < 0 || paid >= gross {
			t.Fatalf("RelayNet(%d) = %d, want 0 ≤ paid < gross", gross, paid)
		}
		if burned < 1 {
			t.Fatalf("gross %d burned %d, want ≥ 1", gross, burned)
		}
		if burned > maxBurn {
			t.Fatalf("gross %d burned %d, want ≤ ceil(gross/8) = %d", gross, burned, maxBurn)
		}
	}
	if got := RelayNet(0); got != 0 {
		t.Fatalf("RelayNet(0) = %d, want 0", got)
	}
}
