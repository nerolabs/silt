package credit

// PoD relay lane — the relay/gateway bandwidth-compensation settlement
// (docs/design/pod.md §7.3). It is the sibling of RedeemDeliveryCredit
// (delivery.go). THE SHIPPED LANE PAYS 0 (R0.7 interim, 2026-09-03) until the
// R2.14 prepayment anchor lands. Read the STATUS block before the mechanism.
//
// STATUS — R0.7 INTERIM (the certified facts, replacing five false claims):
//
//  1. The shipped lane is NOT conserved. Settlement runs on the RELAY's own
//     ledger (core/node/relaytransport.go SettleRelaySession), and the fetcher
//     it would debit is, by M0 mandate, a FRESH EPHEMERAL identity this ledger
//     has never seen. Debiting it auto-registers a phantom account with the
//     faucet grant (credit.go Register; 500,000 at cmd/silt/daemon.go) and the
//     relay's balance rises by chainValue with nothing binding the chain to a
//     real payment. That is a per-session mint, not a transfer (RT-RELAY-1).
//  2. The fetcher-side balance on this ledger is that phantom auto-grant, not
//     "the fetcher's already-paid blind credit". The fetcher's real payment,
//     if any, landed on a demand-token issuer's ledger, never the relay's.
//  3. The 2026-08-30 relay-lane certification does NOT cover the shipped
//     code's conservation. Its conservation verdict (2026-08-27 Q7) is
//     conditional on the anchor of Q4(a), which was never built: RelayOpen
//     carries a bare fetcher-set Funding int (core/relaypay/wire.go), and no
//     token, credit, balance, or ledger check exists on the open path.
//  4. The budget cap (chainValue <= budget) bounds nothing real: budget is
//     S × increment from the fetcher-declared S (core/node/relayrole.go), so
//     it rejects only a caller that contradicts itself (R2.9 gate G-3).
//  5. Therefore the only honest settlement is 0. RedeemRelayCredit pays 0 and
//     performs NO ledger mutation — not even a debit of the ephemeral, since a
//     debit against a phantom grant is the same fiction as the mint — until an
//     anchor exists. No anchor type exists yet; R2.14 fills this function.
//
// Certification (binding):
// silt-reviews/research/research-outcome/RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §2 (the break re-verified), §9 step 1 (this narrowing), R-RELAY-DOC-TRUTH.
// Break report:
// silt-reviews/red-team/RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md.
// Deliberation: docs/thinking/2026-09-03-r0.7-relay-interim-design.md.
//
// THE MECHANISM (unchanged; what R2.14 completes): a relay forwards
// content-blind bytes toward a fetcher and cannot sign a completed-delivery
// receipt (it never holds a verifiable object). It is paid as-it-goes by a
// sender-funded PayWord hash chain (core/relaypay): the fetcher commits a
// chain root once, reveals one preimage per forwarded increment, and the relay
// redeems the highest preimage it holds for count × increment at settlement.
// What R2.14 adds is the Rivest–Shamir authorization half this build omitted:
// the chain root is anchored to a blind-signed, relay-issued (issuer == relay,
// bilateral) prepayment that the relay verifies and spends once, so
// settled(relay) <= Σ face(spent anchors) on the paying ledger
// (INV-RELAY-CONS, cert §6.2).
//
// NEVER STANDING (holds today, independent of the mint): this entry point
// moves the balance economy only. No field it touches is read by Reputation —
// asserted structurally by the Invariant-A guard (invariant_a_test.go
// classifies RedeemRelayCredit `neutral`) and the direct firewall test
// (relay_test.go TestRelayCreditNeverTouchesStanding). A PayWord chain is
// fundable with zero object bytes by certified design (it pays for forwarding,
// which is unprovable), so relay credit buying even one unit of standing would
// convert funded chains into consensus weight — the γ→1/N hole. The firewall
// is why R0.7 is a balance mint and not a consensus break.
//
// NO RELAY SKIM in v1 (design §6); revisit with R2.14, where a conserved
// transfer first exists to skim from.

import "github.com/nerolabs/silt/ports"

// RedeemRelayCredit is the settlement entry point for a PayWord relay chain.
// chainValue is count × increment for the highest preimage the relay holds
// (the caller computes it from the core/relaypay verifier); budget is the
// committed chain budget S × increment. Returns the credit paid to the relay.
//
// R0.7 INTERIM: it returns 0 and mutates NOTHING. No anchor type exists, so no
// anchor can be presented, so no settlement is honest (STATUS above, point 5).
// The self-relay guard is kept as the first check so the R2.14 body composes
// behind it unchanged. The signature is kept so the callers
// (core/node/relaytransport.go SettleRelaySession, the invariant-A press) are
// untouched; R2.14 adds the anchor parameter and the conserved transfer.
//
// Gates: TestRelayRedeemPaysZeroUntilAnchor (pays 0 AND the ephemeral's
// account is never created), TestRelayRedeemPaysZeroEvenWhenFetcherIsFunded
// (0 is unconditional, not a solvency check), TestSelfRelayPaysNothing,
// TestRelayCreditNeverTouchesStanding.
func (l *Ledger) RedeemRelayCredit(relay, fetcher ports.NodeID, chainValue, budget int64) int64 {
	if relay == fetcher {
		return 0 // self-relay earns nothing (the cheapest gaming, blocked)
	}
	_, _ = chainValue, budget // carried for R2.14; unused until an anchor exists
	// No anchor exists (R2.14), so nothing was paid to this ledger, so nothing
	// is owed from it. Do not touch either account: acct() would Register the
	// fetcher's fresh ephemeral with the faucet grant, conjuring the phantom
	// balance RT-RELAY-1 mints from.
	return 0
}
