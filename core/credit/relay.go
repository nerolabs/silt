package credit

// PoD relay lane — the relay/gateway bandwidth-compensation settlement. It
// is the sibling of the delivery session lane (deliveryanchor.go); both
// spend anchors into the one guard in delivery.go.
//
// STATUS — BUILT, the relay-lane prepayment ANCHOR. The interim (pays 0,
// 2026-09-03) is retired by it. The facts:
//
// 1. Settlement runs on the RELAY's own ledger (core/node/relaytransport.go
// SettleRelaySession, and the fetcher it forwards for is, by M0 mandate, a
// FRESH EPHEMERAL identity this ledger has never seen. So settlement debits
// NOBODY at settle time: the payment was made at ISSUANCE, when the
// fetcher's DURABLE identity bought k anchors from this relay through the
// ordinary blind withdrawal (ChargePublish on THIS ledger, refusable), and
// the relay verified and spent them at session open (SpendRelayAnchors,
// relayanchor.go). budget is the ledger's own Σ face of those spent
// anchors, never a fetcher-declared number and never S × increment.
// 2. Settlement pays RelayNet(min(chainValue, budget)) — the settlement less the
// relay burn — to acct(relay). The relay is
// registered on its own ledger, so acct is safe HERE only — and touches
// no other account. acct(ephID) is never called: on this ledger it would
// Register the ephemeral with the faucet grant, the phantom balance the
// phantom mint was drawn from.
// 3. The unconsumed remainder budget − paid is BURNED (≤ one face = 50,000 credits per 24.4 GiB
// session since the 2026-09-06 re-price, 300,000 per 1 GiB before it; the
// relay gains nothing from a stall; an owner-accepted v1 residual; the
// follow-on is a MsgRelayFund top-up with FRESH anchors — "present k,
// spend lazily" is REFUTED on guard (ii).
// 4. Conservation per session on this ledger: Δ Σ_L = paid − Σ face < 0 for every
// non-empty session: the burn takes at least one credit even at full consumption.
// So collusion (the operator buys anchors from itself and settles them back) is a
// strict loss at every consumption level (TestRelayWashLoopIsAStrictLoss).
// 5. BUILT ≠ LIVE. An anchor verifies only under a chain-committed per-epoch
// key (a v5 IssuerKeyReg), so the lane is DARK until era-4 activation; until
// then every open is refused with a named reason and nothing is paid (the
// correct direction).
//
//
// THE MECHANISM: a relay forwards content-blind bytes toward a fetcher and
// cannot sign a completed-delivery receipt (it never holds a verifiable
// object). It is paid as-it-goes by a sender-funded PayWord hash chain
// (core/relaypay): the fetcher commits a chain root once, reveals one preimage
// per forwarded increment, and the relay redeems the highest preimage it holds
// for count × increment at settlement — capped by the anchors that root was
// committed under. That is Rivest–Shamir's authorization half: the chain root is
// bound (in the ephemeral-signed session-open commitment) to a blind-signed,
// relay-issued (issuer == relay, bilateral) prepayment.
//
// NEVER STANDING: this entry point moves the balance economy only. No field it
// touches is read by Reputation — asserted structurally by the Invariant-A
// guard (invariant_a_test.go classifies RedeemRelayCredit and SpendRelayAnchors
// `neutral`) and the direct firewall test (relay_test.go
// TestRelayCreditNeverTouchesStanding, whose paid > 0 precondition
// restores). A PayWord chain is fundable with zero object bytes by design (it
// pays for forwarding, which is unprovable), so relay credit buying even one
// unit of standing would convert funded chains into consensus weight — the
// γ→1/N hole.
//
// THE RELAY BURN. A relay settlement pays the relay what the fetcher's anchors funded,
// less RelayBurnNum/RelayBurnDen of it, which is destroyed. Without it a colluding
// pair (the relay operator funding anchors and settling them back to itself) breaks
// exactly even, and a wash that costs nothing is free volume. With it every such loop
// is a strict loss. The burn has no object to fund, so unlike the delivery skim it is
// burned, not routed to an escrow. Its rate is the delivery skim's, which keeps the
// relay's net price (524,288 B / (7/8) = 599,186 B per credit) above the serve-mint
// denomination Dλ, so Don't #7 holds on its net reading (numeraire.go).
//
// THE RELAY LANE KEEPS THE BURN (refund: its counterparty is an ephemeral with no
// account, so a refund would be the phantom). One call-graph coupling to know:
// SpendRelayAnchors' epoch advance runs the shared once-per-epoch sweep, which also
// releases the DELIVERY lane's due deposits into existing balances (deliveryanchor.go
// releaseDueRefunds). Balance-only, single-writer event loop; the rule the relay lane
// inherits is nothing, the sweep it shares is one.

import "github.com/nerolabs/silt/ports"

// RedeemRelayCredit is the settlement entry point for a PayWord relay chain.
// chainValue is count × increment for the highest preimage the relay holds
// (the caller computes it from the core/relaypay verifier); budget is the
// ledger's own Σ face of the anchors SpendRelayAnchors recorded at open. It pays
// min(chainValue, budget) into acct(relay) and returns it; the remainder is
// burned (STATUS point 3). An unanchored session has budget 0 and pays 0
// without touching any account.
//
// Gates: TestRelayLaneConservesTotalSupplyOnOnePerNodeLedger,
// TestRelaySettlementRefusesUnanchoredSession,
// TestRelaySettlementNeverLeavesAnAccountNegative, TestSelfRelayPaysNothing,
// TestRelayCreditNeverTouchesStanding.
func (l *Ledger) RedeemRelayCredit(relay, fetcher ports.NodeID, chainValue, budget int64) int64 {
	if relay == fetcher {
		return 0 // self-relay earns nothing (the cheapest gaming, blocked)
	}
	if budget <= 0 || chainValue <= 0 {
		// Nothing was spent into this session, or nothing was
		// forwarded. Do not touch either account: acct would Register
		// the fetcher's fresh ephemeral with the faucet grant (the
		// phantom).
		return 0
	}
	gross := min(chainValue, budget)
	paid := RelayNet(gross)
	l.acct(relay).balance += paid
	l.relayBurnedCredits += gross - paid
	return paid
}

// RelayNet is what a relay is paid for a settlement worth gross: gross less the burn,
// rounded up, so every non-zero settlement burns at least one credit and a wash loop
// is a strict loss at every size.
func RelayNet(gross int64) int64 {
	if gross <= 0 {
		return 0
	}
	return gross - (gross*RelayBurnNum+RelayBurnDen-1)/RelayBurnDen
}

// RelayBurnNum/RelayBurnDen is the share of every relay settlement that is burned.
const (
	RelayBurnNum = SkimNum
	RelayBurnDen = SkimDen
)

// RelayBurnedCredits is the relay lane's lifetime settlement burn on this ledger.
func (l *Ledger) RelayBurnedCredits() int64 { return l.relayBurnedCredits }
