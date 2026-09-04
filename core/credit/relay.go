package credit

// PoD relay lane — the relay/gateway bandwidth-compensation settlement
// (docs/design/pod.md §7.3). It is the sibling of RedeemDeliveryCredit
// (delivery.go).
//
// STATUS — R2.14 BUILT (2026-09-04), the relay-lane prepayment ANCHOR. The
// R0.7 interim (pays 0, 2026-09-03) is retired by it. The certified facts:
//
//  1. Settlement runs on the RELAY's own ledger (core/node/relaytransport.go
//     SettleRelaySession), and the fetcher it forwards for is, by M0 mandate, a
//     FRESH EPHEMERAL identity this ledger has never seen. So settlement debits
//     NOBODY at settle time: the payment was made at ISSUANCE, when the
//     fetcher's DURABLE identity bought k anchors from this relay through the
//     ordinary blind withdrawal (ChargePublish on THIS ledger, refusable), and
//     the relay verified and spent them at session open (SpendRelayAnchors,
//     relayanchor.go). budget is the ledger's own Σ face of those spent
//     anchors, never a fetcher-declared number and never S × increment.
//  2. Settlement pays min(chainValue, budget) to acct(relay) — the relay is
//     registered on its own ledger, so acct() is safe HERE only — and touches
//     no other account. acct(ephID) is never called: on this ledger it would
//     Register the ephemeral with the faucet grant, the phantom balance the
//     RT-RELAY-1 mint was drawn from.
//  3. The unconsumed remainder budget − paid is BURNED (R-ANCHOR-STALL ≡
//     R-ANCHOR-GRANULARITY, cert §7: ≤ 300,000 credits per 1 GiB session; the
//     relay gains nothing from a stall; an owner-accepted v1 residual; the
//     certified follow-on is a MsgRelayFund top-up with FRESH anchors — "present
//     k, spend lazily" is REFUTED on guard (ii)).
//  4. Conservation per session on this ledger: Δ Σ_L = settled − Σ face ≤ 0,
//     equality iff fully consumed (INV-RELAY-CONS; cert C-1 withdrew the older
//     "unchanged" corollary). Collusion (the operator buys anchors from itself
//     and settles them back) is a WASH at full consumption, not a strict loss —
//     there is no relay skim in v1 (R-RELAY-WASH-ZERO-LOSS, an owner call
//     before R2.4).
//  5. BUILT ≠ LIVE. An anchor verifies only under a chain-committed per-epoch
//     key (a v5 IssuerKeyReg), so the lane is DARK until era-4 activation; until
//     then every open is refused with a named reason and nothing is paid (the
//     correct direction; cert §8).
//
// Certification (binding):
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md;
// direction:
// silt-reviews/research/research-outcome/RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md;
// build shape:
// silt-reviews/crypto-specialist/ADVISORY-R2.14-relay-prepayment-anchor-build-2026-09-04.md.
// Deliberation: docs/thinking/2026-09-04-r2.14-relay-prepayment-anchor-design.md.
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
// TestRelayCreditNeverTouchesStanding, whose paid > 0 precondition R2.14
// restores). A PayWord chain is fundable with zero object bytes by certified
// design (it pays for forwarding, which is unprovable), so relay credit buying
// even one unit of standing would convert funded chains into consensus weight —
// the γ→1/N hole.
//
// NO RELAY SKIM in v1 (design §6): the conserved transfer now exists to skim
// from; whether to is the owner's call before R2.4.

import "github.com/nerolabs/silt/ports"

// RedeemRelayCredit is the settlement entry point for a PayWord relay chain.
// chainValue is count × increment for the highest preimage the relay holds
// (the caller computes it from the core/relaypay verifier); budget is the
// ledger's own Σ face of the anchors SpendRelayAnchors recorded at open. It pays
// min(chainValue, budget) into acct(relay) and returns it; the remainder is
// burned (STATUS point 3). An unanchored session has budget 0 and pays 0
// without touching any account.
//
// Gates: TestRelayLaneConservesTotalSupplyOnOnePerNodeLedger (T-2),
// TestRelaySettlementRefusesUnanchoredSession (T-1),
// TestRelaySettlementNeverLeavesAnAccountNegative (T-5), TestSelfRelayPaysNothing,
// TestRelayCreditNeverTouchesStanding.
func (l *Ledger) RedeemRelayCredit(relay, fetcher ports.NodeID, chainValue, budget int64) int64 {
	if relay == fetcher {
		return 0 // self-relay earns nothing (the cheapest gaming, blocked)
	}
	if budget <= 0 || chainValue <= 0 {
		// Nothing was spent into this session, or nothing was forwarded. Do not
		// touch either account: acct() would Register the fetcher's fresh
		// ephemeral with the faucet grant (the RT-RELAY-1 phantom).
		return 0
	}
	paid := min(chainValue, budget)
	l.acct(relay).balance += paid
	return paid
}
