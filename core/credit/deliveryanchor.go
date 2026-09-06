package credit

// R2.9 — byte-denominated per-increment delivery settlement: the LEDGER half
// (D-R2.9-DIRECTION, ratified 2026-09-04; build questions certified in
// silt-reviews/research/research-outcome/R2.9-build-questions-domain-rescale-guard-RESEARCH-CERTIFICATION-2026-09-04.md
// §2.5 (the settlement rule), §2.6 (what spend-at-open forces); the price pair
// (U, p) = (DeliveryIncrementBytes, DeliveryIncrementCredit) certified and ratified
// 2026-09-06, numeraire.go). Deliberation:
// docs/thinking/2026-09-06-r2.9-delivery-settlement-quantization.md.
//
// THE MECHANISM. The delivery anchor IS the existing demand token
// (silt/blinddemand/fdh/v2 — no fifth domain, cert §2.2) with ONE timing change: it is
// spent into the shared paid-serial guard at session OPEN, before any service, and
// its face becomes the session's budget (the R2.14 relay shape, relayanchor.go). The
// receipt keeps its three certified roles — unforgeable acknowledgement, the P3b
// bonded-fetcher credential, the supersede of the provisional self-mint — and gains a
// COUNT j of increments the fetcher acknowledges. Settlement per session on the
// server's ledger L (cert §2.5):
//
//	buy k anchors (ChargePublish × k)          Σ_L −k·f
//	open: SpendDeliveryAnchors, budget = k·f    0
//	serve B bytes: self-mint over the lane      +⌊7B/8Dλ⌋ + ⌊B/8Dλ⌋ (provisional)
//	settle count j: reverse min(j·U, B) bytes   −(that mint)
//	                pay min(j·p, budget) − skim to the server, skim to escrow
//
//	Δ Σ_L = min(j·p, k·f) − k·f + mint(B − min(j·U, B))
//
// The first two terms are the anchor conservation ≤ 0 (R2.14 C-1); the last is the
// un-acknowledged TAIL of the bilateral fallback, the mint the owner kept (ruling 3).
// On a fully acknowledged delivery it is 0 and the lane is exactly conserved when the
// budget is consumed. The remainder budget − min(j·p, budget) is BURNED (G-6).
//
// PARTIAL REVERSAL IS LOAD-BEARING (cert §2.5, gate B-2). The flat leg reverses the
// WHOLE lane (RedeemDeliveryCreditReason). Kept here while paying j·p, a fetcher
// acknowledging fewer increments than it fetched would leave the server strictly
// worse off than suppression, and "accept strictly dominates at every size" (G-1)
// would be false for every partial ack. So the reversal is denominated per increment:
// the acknowledged bytes leave the lane's accumulator, the two floors are recomputed,
// and exactly the difference is reversed; the tail keeps its bytes AND its remainder
// (they are bytes no receipt paid for — the same remainder rule as G-λ-5, applied to
// the un-acknowledged part only).
//
// WHAT THIS FILE DOES NOT DECIDE. What a session is keyed on, when it closes, whether
// it outlives one object, and what the receipt binds are the node's (core/node) —
// research-gated under G-R212-8 (the anchor quantization at 12.21 GiB per face on a
// multi-provider fetch path) and built after that verdict. Settle-once is BY SESSION
// and is the node's property exactly as on the relay lane (the handle is deleted
// before this is called; TestNoDoubleSettleReaperAndPump's twin): this entry point
// pays whatever budget it is handed and cannot know a session was already settled.
// The flat leg stays callable until the node half retires the un-anchored receipt
// (gate B-9).
//
// NEVER STANDING: every method here moves the balance economy only. Classified
// neutral in invariant_a_test.go and pressed against a bondless identity on an
// anchored, paying delivery session (B-14).

import (
	"math"

	"github.com/nerolabs/silt/ports"
)

// ReasonNoIncrement: a settlement acknowledging zero increments. Nothing is paid and
// nothing is reversed — the session's self-mint stays as the bilateral fallback.
const ReasonNoIncrement = "no-increment"

// SpendDeliveryAnchors is the ledger half of a delivery session open (R2.9): it
// records k VERIFIED demand-domain anchors as spent in the shared paid-serial guard,
// all-or-nothing, and returns their summed face — the session budget — or 0 and the
// named reason it recorded nothing. The caller (core/node) verified each anchor under
// the server's OWN committed per-epoch key in the DEMAND domain before calling; the
// ledger verifies nothing and guards everything (the relay twin's contract, ports.go
// SpendRelayAnchors). server is the delivering server the session is opened at,
// recorded on the guard entry for the durable store's records.
//
// A top-up on an admitted session is the SAME call with fresh anchors (cert §2.6:
// one all-or-nothing guard spend per top-up; a re-presented anchor is refused with
// ReasonAlreadyPaid and the batch records nothing — gate B-3b).
func (l *Ledger) SpendDeliveryAnchors(server ports.NodeID, anchors []RelayAnchor) (face int64, reason string) {
	return l.spendAnchors(server, anchors)
}

// SettleDelivery settles one anchored delivery session ONCE: the fetcher acknowledged
// count increments of DeliveryIncrementBytes on the lane (server, fetcher, root), and
// budget is the ledger's own Σ face SpendDeliveryAnchors recorded at open (plus any
// top-ups). It reverses the lane's provisional self-mint for min(count·U, B) bytes,
// pays min(count·p, budget) less the durability skim into the server's balance, routes
// the skim into the object's escrow, and burns the remainder. Returns the credits paid
// to the server and the reason. An unanchored session (budget ≤ 0) pays 0 and leaves
// the self-mint alone — the unwitnessed bilateral fallback (B-3, B-9).
//
// The payout never reads l.fee: the budget is what was spent, on this ledger, at open
// (cert G-3; the R2.14 lesson that a pin over the constants never holds a seam whose
// runtime value is read elsewhere).
func (l *Ledger) SettleDelivery(server, fetcher ports.NodeID, root ports.Hash, count, budget int64) (paid int64, reason string) {
	if server == fetcher {
		return 0, ReasonSelfDelivery // self-delivery earns nothing (the cheapest gaming, blocked)
	}
	if budget <= 0 {
		return 0, ReasonNoAnchor // nothing was spent into this session: touch no account
	}
	if count <= 0 {
		return 0, ReasonNoIncrement
	}
	// value = min(count·p, budget), computed without overflowing on an attacker-sized
	// count (the node bounds count by the session ceiling; the ledger does not trust it).
	value := budget
	if count < budget/DeliveryIncrementCredit {
		value = count * DeliveryIncrementCredit
	}

	// Per-increment reversal of the provisional lane (B-2). If the lane was already
	// evicted its mint was reversed at eviction and there is nothing here to reverse:
	// the settlement pays the conserved leg only (rule (b), one delivery one payment).
	k := provKey{server: server, requester: fetcher, root: root}
	if p, ok := l.provisional[k]; ok {
		ack := p.bytes // saturate: a count whose bytes overflow acknowledges the whole lane
		if count <= math.MaxInt64/DeliveryIncrementBytes && count*DeliveryIncrementBytes < p.bytes {
			ack = count * DeliveryIncrementBytes
		}
		if ack >= p.bytes {
			// Fully acknowledged: the whole lane, exactly as the flat supersede does.
			l.reverseProvisional(server, root, p)
			delete(l.provisional, k)
			l.removeFromProvOrder(k)
		} else {
			// Partially acknowledged: the acknowledged bytes leave the accumulator; the
			// two floors are recomputed over the tail and exactly the difference is
			// reversed. The tail keeps its bytes and its remainder — bytes no receipt
			// paid for (the G-λ-5 rule applied to the un-acknowledged part).
			p.bytes -= ack
			netAfter := p.bytes * (SkimDen - SkimNum) / (SkimDen * ServeMintBytesPerCredit)
			skimAfter := p.bytes * SkimNum / (SkimDen * ServeMintBytesPerCredit)
			l.reverseLane(server, root, p.net-netAfter, p.skim-skimAfter)
			p.net, p.skim = netAfter, skimAfter
		}
	}

	// Conservation: pay out of the budget the fetcher already burned in, less the
	// skim; the remainder is burned (G-6) — no account and no escrow receives it.
	// acct() REGISTERS an unknown account (and hands it the grant), so it is taken
	// here at the payment and not above: a refusal must not conjure an account.
	skim := value * SkimNum / SkimDen
	l.acct(server).balance += value - skim
	e := l.escrowFor(root)
	e.balance += skim
	e.funded += skim
	l.deliverySettlements++
	l.deliverySettledCredits += value
	l.deliveryBurnedCredits += budget - value
	l.deliverySettledIncrements += value / DeliveryIncrementCredit
	return value - skim, ReasonPaid
}

// DeliverySettlementStats is the R2.9 settlement telemetry: node-wide aggregates of
// what anchored delivery sessions paid and burned. Instrument-class; never joined to
// an identity axis (Don't #3).
type DeliverySettlementStats struct {
	Settlements       int64 // anchored sessions settled with a non-zero budget and count
	SettledCredits    int64 // credits paid out of anchor budgets, gross of the skim
	BurnedCredits     int64 // face remainder burned at settle (the anchor-quantization residual, G-R212-8)
	SettledIncrements int64 // increments of DeliveryIncrementBytes those credits acknowledged
}

// DeliverySettlementStats reads the settlement telemetry. Reading moves nothing.
func (l *Ledger) DeliverySettlementStats() DeliverySettlementStats {
	return DeliverySettlementStats{Settlements: l.deliverySettlements, SettledCredits: l.deliverySettledCredits,
		BurnedCredits: l.deliveryBurnedCredits, SettledIncrements: l.deliverySettledIncrements}
}
