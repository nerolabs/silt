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
// it outlives one object, and what the receipt binds are the node's (core/node). The
// G-R212-8 certification
// (silt-reviews/research/research-outcome/R2.9-G-R212-8-delivery-anchor-quantization-RESEARCH-CERTIFICATION-2026-09-06.md
// §3) fixes the shape the node half must build: a session keyed on (server, durable
// fetcher), one anchor, a byte ceiling DERIVED from the face, spanning objects, settled
// INCREMENTALLY against a monotone counter (settle-once is REFUTED, §6.1): the node
// calls SettleDelivery with the DELTA count and the REMAINING budget, so this entry
// point is per-settlement arithmetic and holds no session state. The remainder is
// therefore accounted ONCE, at CloseDeliverySession, never here (gate
// TestRemainderIsAccountedOnceAtCloseNotPerSettlement) — as a DEPOSIT released to the
// fetcher's existing account when the session's anchors leave the guard window
// (D-R2.9-NODE-HALF-CALLS call 1 amended 1′, 2026-09-07; M1 + M2). The flat leg stays
// callable until the node half retires the un-anchored receipt (gate B-9).
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
	return l.spendAnchors(server, anchors, laneDelivery)
}

// SettleDelivery is ONE settlement step of an anchored delivery session (settle-
// monotone, G-R212-8 cert §6.1 C5): the fetcher's cumulative receipt acknowledged
// count NEW increments of DeliveryIncrementBytes since the last step (the node passes
// the DELTA), and budget is what the session has NOT yet settled of its Σ face (the
// node passes the REMAINING budget, and lowers it by the credits this returns plus the
// skim). It reverses the provisional self-mint of the lane (server, fetcher, root)
// for min(count·U, B) bytes, pays min(count·p, budget) — in WHOLE increments — less
// the durability skim into the server's balance, and routes the skim into the object's
// escrow. prior is the session's credits settled BEFORE this step (the node's own
// monotone counter): the skim is the CUMULATIVE floor ⌊(prior+value)/8⌋ − ⌊prior/8⌋,
// never ⌊value/8⌋ of one step — under settle-monotone the fetcher chooses the step, and a
// per-step floor let a face settled in deltas ≤ 7 fund the escrow with NOTHING while the
// serve-time skim was clawed back (net negative; certification
// silt-reviews/research/research-outcome/R2.9-settlement-skim-under-fetcher-chosen-deltas-RESEARCH-CERTIFICATION-2026-09-06.md
// §3–4: the G-λ-7 discipline applied to the witnessed leg; no remainder is stored — it is
// implicit in settled mod 8 and dies with the session; the ratified 1/8 does not move).
// It returns the GROSS credits settled out of the budget (what the node subtracts from
// the session's remaining budget — read from the ledger, never recomputed), the credits
// paid to the server, and the reason. It burns NOTHING:
// the unsettled remainder is accounted once, at CloseDeliverySession. An unanchored
// session (budget ≤ 0) pays 0 and leaves the self-mint alone — the unwitnessed
// bilateral fallback (B-3, B-9). A session settles as many times as it has deltas
// (B-10 as REFUTED); the conserved property Σ settled ≤ Σ face is the node's monotone
// counter, which this step cannot see and does not need to.
//
// The payout never reads l.fee: the budget is what was spent, on this ledger, at open
// (cert G-3; the R2.14 lesson that a pin over the constants never holds a seam whose
// runtime value is read elsewhere).
func (l *Ledger) SettleDelivery(server, fetcher ports.NodeID, root ports.Hash, count, budget, prior int64) (settled, paid int64, reason string) {
	if server == fetcher {
		return 0, 0, ReasonSelfDelivery // self-delivery earns nothing (the cheapest gaming, blocked)
	}
	if budget <= 0 {
		return 0, 0, ReasonNoAnchor // nothing was spent into this session: touch no account
	}
	if count <= 0 {
		return 0, 0, ReasonNoIncrement
	}
	// value = min(count, ⌊budget/p⌋)·p — WHOLE increments only, so a budget that is not
	// a multiple of p never over-pays the acknowledged count by a fraction of one (blind
	// PE item 4, 2026-09-06: exact at p = 1, armed on the next re-price); the fractional
	// tail stays in the remainder and is accounted at close. Computed without
	// overflowing on an attacker-sized count (the node bounds count by the session
	// ceiling; the ledger does not trust it).
	whole := budget / DeliveryIncrementCredit
	if count < whole {
		whole = count
	}
	value := whole * DeliveryIncrementCredit
	if value <= 0 {
		return 0, 0, ReasonNoIncrement // a budget below one increment funds nothing more
	}

	// Per-increment reversal of the provisional lane (B-2). If the lane was already
	// evicted its mint was reversed at eviction and there is nothing here to reverse:
	// the settlement pays the conserved leg only (rule (b), one delivery one payment).
	k := provKey{server: server, requester: fetcher, root: root}
	if p, ok := l.provisional[k]; ok {
		// The reversal is denominated in the SAME budget-capped whole increments the
		// payment is (G-DEM-8, R-ACK-USES-UNTRUSTED-COUNT closed): a count the budget
		// cannot fund must not extinguish more of the server's mint than it pays for —
		// that would be the "server strictly worse off than suppression" shape G-1
		// exists to prevent. Saturating: a whole whose bytes overflow acknowledges the
		// whole lane.
		ack := p.bytes
		if whole <= math.MaxInt64/DeliveryIncrementBytes && whole*DeliveryIncrementBytes < p.bytes {
			ack = whole * DeliveryIncrementBytes
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
	// skim; the unsettled remainder is NOT touched here — it becomes the session's deposit
	// at CloseDeliverySession (no account and no escrow receives it at settlement).
	// acct() REGISTERS an unknown account (and hands it the grant), so it is taken
	// here at the payment and not above: a refusal must not conjure an account.
	if prior < 0 {
		prior = 0
	}
	skim := (prior+value)*SkimNum/SkimDen - prior*SkimNum/SkimDen // 0 ≤ skim ≤ value (G-SKIM-2)
	l.acct(server).balance += value - skim
	e := l.escrowFor(root)
	e.balance += skim
	e.funded += skim
	l.deliverySettlements++
	l.deliverySettledCredits += value
	l.deliverySettledIncrements += value / DeliveryIncrementCredit
	return value, value - skim, ReasonPaid
}

// pendingRefund is one session's unsettled remainder, a DEPOSIT waiting for its anchor
// to leave the guard window (M2). {durable fetcher, amount, release epoch} is inside
// Don't #3 on all three prongs (cert §6.2): every field is read by the release, the
// record dies when it fires, and it is server-local process memory. The fetcher identity
// is NEVER joined to the anchor serial and never persisted (G-6R-8).
type pendingRefund struct {
	fetcher      ports.NodeID
	amount       int64
	releaseEpoch uint64
}

// CloseDeliverySession accounts the close of one anchored delivery session
// (D-R2.9-NODE-HALF-CALLS call 1, amended 1′; certification
// silt-reviews/research/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md
// §3.3, §3.6, §4.5). remaining is the budget the session never settled (Σ face −
// settled). It is a DEPOSIT, not a burn: booked here as one pending record released to
// the fetcher when the session's anchors leave the guard window — releaseEpoch =
// maxAnchorEpoch + W + 1 — or now, if that epoch has already passed at close
// ("whichever is LATER", M2). Release-at-close alone is REFUTED: a bearer anchor passed
// down a chain of fresh keypairs would buy unlimited guard slots for zero net credits;
// locking the deposit for the anchor's lifetime bounds live occupancy by stock/f, the
// same bound the burn gave (T-DEPOSIT), and keeps R2.12's start-up assertion exact.
//
// Booking and accounting are ONE ledger call (cert §3.3): the pending table is bounded at
// the guard cap and REFUSES-never-evicts — at the cap the remainder is BURNED and counted,
// never a live record dropped (build-immutable #8, G-6R-6). The caller deletes the
// session before calling, so a second close of the same session cannot happen.
// Returns the credits booked as pending (0 when nothing remained or the cap burned it).
func (l *Ledger) CloseDeliverySession(fetcher ports.NodeID, remaining int64, maxAnchorEpoch uint64) int64 {
	if remaining <= 0 {
		return 0
	}
	l.deliverySessionsClosed++
	l.advanceEpoch() // read the source once; the band advance sweeps and releases what is due
	if len(l.pendingRefunds) >= maxPaidSerial {
		l.deliveryBurnedCredits += remaining
		l.deliveryRefundsBurnedAtCap++
		return 0
	}
	rel := maxAnchorEpoch + paidSerialWindow + 1
	l.pendingRefunds = append(l.pendingRefunds, pendingRefund{fetcher: fetcher, amount: remaining, releaseEpoch: rel})
	l.deliveryPendingCredits += remaining
	if rel <= l.epochWatermark {
		l.releaseDueRefunds() // "whichever is LATER": an anchor already outside the window releases now (blind PE item 6: scan only when something is due)
	}
	return remaining
}

// releaseDueRefunds pays every pending record whose release epoch the WATERMARK has
// reached (never the raw source — R-F8-LATCH), in booking order (B2: no map iteration).
// M1, the payee rule: the amount is credited iff the fetcher's account ALREADY EXISTS on
// this ledger — never through acct(), which would register a fresh identity and, on an
// unconfigured faucet, hand it the whole grant (the RT-RELAY-1 phantom). The honest
// fetcher always has an account here: it bought its anchor through ChargePublish on this
// ledger. A fetcher with none forfeits the remainder (burned, counted:
// R-REFUND-NEEDS-AN-ACCOUNT, ≤ f per session).
func (l *Ledger) releaseDueRefunds() {
	if len(l.pendingRefunds) == 0 {
		return
	}
	keep := l.pendingRefunds[:0]
	for _, p := range l.pendingRefunds {
		if p.releaseEpoch > l.epochWatermark {
			keep = append(keep, p)
			continue
		}
		l.deliveryPendingCredits -= p.amount
		if a, ok := l.accounts[p.fetcher]; ok {
			a.balance += p.amount
			l.deliveryRefundedCredits += p.amount
		} else {
			l.deliveryBurnedCredits += p.amount
			l.deliveryRefundsBurnedNoAccount++
		}
	}
	for i := len(keep); i < len(l.pendingRefunds); i++ {
		l.pendingRefunds[i] = pendingRefund{}
	}
	l.pendingRefunds = keep
}

// ReleaseDueRefunds advances the ledger's epoch watermark from its source and releases
// every deposit whose anchor has left the window. The node's session sweep calls it so a
// silent server still returns deposits on time; every guarded ledger operation releases
// on the same band advance anyway.
func (l *Ledger) ReleaseDueRefunds() {
	l.advanceEpoch()
	l.releaseDueRefunds()
}

// DeliverySettlementStats is the R2.9 settlement telemetry: node-wide aggregates of
// what anchored delivery sessions paid and burned. Instrument-class; never joined to
// an identity axis (Don't #3).
type DeliverySettlementStats struct {
	Settlements       int64 // settlements (deltas) that paid a non-zero amount out of an anchor budget
	SettledCredits    int64 // credits paid out of anchor budgets, gross of the skim
	SessionsClosed    int64 // sessions closed with an unsettled remainder
	SettledIncrements int64 // increments of DeliveryIncrementBytes those credits acknowledged
	// The remainder's three destinations (remainder = Refunded + Pending + Burned):
	RefundedCredits        int64 // deposits released to the fetcher's existing account after the anchor expired (M1 + M2)
	PendingRefundCredits   int64 // deposits booked, anchors still inside the guard window
	BurnedCredits          int64 // GENUINE burns only: no account at release (M1), or the pending table at its cap
	RefundsBurnedNoAccount int64 // releases that found no account (R-REFUND-NEEDS-AN-ACCOUNT)
	RefundsBurnedAtCap     int64 // remainders burned at the pending-table cap (refuse-never-evict)
	// RestoredGuardEntries counts the paid-serial guard entries restored from disk at the
	// last LoadPaidSerials: entries, not credits, and BOTH lanes (the durable store carries
	// no lane — R-GUARD-RESTORE-LANE-UNKNOWN), so it is an UPPER BOUND on the delivery
	// sessions whose unsettled face or pending deposit did not survive the restart
	// (R-DELIVERY-SESSION-EPHEMERAL, G-6R-9). Zero means no deposit can have been lost.
	RestoredGuardEntries int64
	// GuardFullRefusals is the delivery lane's share of GuardFullRefusalsByLane: opens and
	// funds refused because the paid-serial guard was full of LIVE entries — the operator's
	// one number for a serve rate above the bound the cap was derived against (blind PE,
	// 2026-09-07: the marker alone is not a surface). Monotone.
	GuardFullRefusals int64
}

// DeliverySettlementStats reads the settlement telemetry. Reading moves nothing.
func (l *Ledger) DeliverySettlementStats() DeliverySettlementStats {
	return DeliverySettlementStats{Settlements: l.deliverySettlements, SettledCredits: l.deliverySettledCredits,
		SessionsClosed: l.deliverySessionsClosed, SettledIncrements: l.deliverySettledIncrements,
		RefundedCredits: l.deliveryRefundedCredits, PendingRefundCredits: l.deliveryPendingCredits, BurnedCredits: l.deliveryBurnedCredits,
		RefundsBurnedNoAccount: l.deliveryRefundsBurnedNoAccount, RefundsBurnedAtCap: l.deliveryRefundsBurnedAtCap,
		RestoredGuardEntries: l.deliveryRestartOrphans, GuardFullRefusals: l.guardFullRefusalsDelivery}
}

// ProvisionalLaneForTest reports whether a provisional lane is live and its byte
// accumulator. Test seam for the node tier; reading moves nothing.
func (l *Ledger) ProvisionalLaneForTest(server, requester ports.NodeID, root ports.Hash) (bytes int64, live bool) {
	p, ok := l.provisional[provKey{server: server, requester: requester, root: root}]
	if !ok {
		return 0, false
	}
	return p.bytes, true
}

// LivePaidSerialsRaw exports the guard's live entries (the durable store's view). Test
// seam; reading moves nothing.
func (l *Ledger) LivePaidSerialsRaw() []ports.PaidSerial { return l.livePaidSerials() }
