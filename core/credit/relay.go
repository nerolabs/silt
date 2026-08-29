package credit

// PoD relay lane — the conserved relay/gateway bandwidth-compensation
// settlement (docs/design/pod.md §7.3, certified 2026-08-30). It is the sibling
// of RedeemDeliveryCredit (delivery.go): a conserved balance transfer, never a
// mint, that never touches Reputation().
//
// THE MECHANISM: a relay forwards content-blind bytes toward a fetcher and
// cannot sign a completed-delivery receipt (it never holds a verifiable object).
// It is paid as-it-goes by a sender-funded PayWord hash chain (core/relaypay):
// the fetcher commits a chain root once, reveals one preimage per forwarded
// increment, and the relay redeems the highest preimage it holds for
// count × increment at epoch net-settlement. This entry point performs that
// settlement into the relay operator's BALANCE.
//
// CONSERVATION: the chain value is drawn from the fetcher's already-paid blind
// credit — the balance the fetcher funded at blind withdrawal under a FRESH
// EPHEMERAL identity (client.WithdrawDemandTokenPrivately). It is a transfer
// from the fetcher's balance to the relay's, never a mint. A colluding
// fetcher+relay pair under one operator rebuys its own fee at break-even (or a
// strict loss with a skim) — the identical no-money-pump argument as the
// delivery leg (cert §2a). The feared "fabricated dispute mints an adjudicated
// credit" vector does not exist, because no dispute exists (cert §1).
//
// NEVER STANDING: this moves the balance economy only. No field it touches is
// read by Reputation — asserted structurally by the Invariant-A guard
// (invariant_a_test.go classifies RedeemRelayCredit `neutral`) and the direct
// firewall test (relay_test.go TestRelayCreditNeverTouchesStanding). A PayWord
// chain is fundable with zero object bytes by certified design (it pays for
// forwarding, which is unprovable), so relay credit buying even one unit of
// standing would convert funded chains into consensus weight — the γ→1/N hole.
//
// NO RELAY SKIM in v1 (design §6): the conservation bound already makes wash a
// strict non-gain, and a skim is a one-line follow-on (compose exactly as the
// delivery skim, delivery.go:120-124) if a deterrent floor is later wanted.

import "github.com/nerolabs/silt/ports"

// RedeemRelayCredit settles a PayWord relay chain: it transfers chainValue from
// the fetcher's already-paid blind credit into the relay operator's balance.
// chainValue is the settled amount — count × increment for the highest preimage
// the relay holds (the caller computes it from the core/relaypay verifier).
// Returns the credit paid to the relay. Self-relay pays nothing, the same guard
// as RedeemDeliveryCredit. It never touches standing.
//
// It is a pure conserved transfer: the fetcher's balance decreases by exactly
// what the relay's increases. The fetcher's balance is its paid-in blind credit;
// a redeem beyond that credit is the caller's responsibility to prevent (the
// relay stops forwarding when the chain is exhausted — self-enforcing, cert §2c).
func (l *Ledger) RedeemRelayCredit(relay, fetcher ports.NodeID, chainValue int64) int64 {
	if relay == fetcher {
		return 0 // self-relay earns nothing (the cheapest gaming, blocked)
	}
	if chainValue <= 0 {
		return 0
	}
	l.acct(fetcher).balance -= chainValue // drawn from the fetcher's paid-in blind credit
	l.acct(relay).balance += chainValue   // into the relay operator's balance
	return chainValue
}
