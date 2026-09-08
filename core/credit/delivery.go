package credit

// PoD neutral lane — the conserved delivery-credit consumer and the supersede
// rule (docs/design/pod.md §3, certified 2026-08-26; the certification is the
// authority on why this shape). Two rules, both load-bearing:
//
// CONSERVATION: no credit is minted by a receipt. The credit paid here is the
// retrieval fee the fetcher already paid at blind withdrawal (tokenChargeFor →
// ChargePublish), less the durability skim — a transfer, never a mint. The
// certified wash bound follows: a colluding fetcher+server pair pays fee F for
// the token and redeems F − skim, a strict loss of F·SkimNum/SkimDen per loop.
//
// SUPERSEDE: a delivery paid by a redeemed receipt is never ALSO self-credited.
// The serve path self-records one credit per Dλ bytes as it serves (RecordServeToObject,
// two floors over the lane's byte accumulator since G-R212-7)
// — an unfunded self-mint that predates PoD and is exactly the "per-receipt
// subsidy" conservation bans, if it stacked with the receipt credit. So every
// object-aware serve is tracked as PROVISIONAL per (requester, root), and a
// redeem for that delivery reverses the provisional self-credit before applying
// the conserved one (the certification's form (i) — robust when the server
// cannot know at serve time whether a receipt will follow). A serve never
// redeemed keeps its self-record: that is the legitimate unwitnessed bilateral
// fallback, unchanged.
//
// What supersede does NOT reverse: servedBytes/fetchedBytes. The bytes moved;
// the observables stay honest (S5) — only the PAYMENT is superseded.
//
// BOUNDED (build-immutable #8): the provisional map is capped; at the cap the
// oldest entry is FIFO-evicted (deterministic — no map iteration, B2). The
// FIFO order slice (provOrder) is kept in lockstep with the map at BOTH removal
// sites: eviction pops the front, and a redeem tombstones the lane's slot in
// O(1) via provIndex. An amortized-O(1) compaction caps the slice at
// 2*maxProvisional, so the redeem-heavy path (where the eviction loop never
// fires) can neither grow provOrder without bound nor let a stale key survive
// to reverse a re-served lane (the RT-DELIV-1/1b/2 fix, red-team 2026-09-01;
// verified by TestProvOrderStaysBoundedAcrossRedeems /
// TestRedeemDoesNotLeaveDuplicateOrderEntry). Eviction
// REVERSES the evicted lane's eager self-mint before forgetting it (A4 fix,
// Boulder 0, R0.4a — laneFor below), so an evicted lane is left in the
// same accounting state as "never served". A receipt redeemed after its lane was
// evicted therefore pays the conserved leg ONLY and mints nothing — no
// double-pay. The give is the unwitnessed bilateral fallback: an evicted,
// never-redeemed serve loses its self-record. That is an under-pay at the
// >maxProvisional tail, never an over-pay and never a denial (rule (b): one
// delivery, one payment). The deeper fix ("witnessing all cross-operator serves
// would subsume the self-mint") stays the tracked (b)-full follow-on.
//
// NEVER STANDING: everything here moves the balance economy only. No field this
// file touches is read by Reputation — asserted structurally by the Invariant-A
// guard (invariant_a_test.go) and the delivery firewall test.

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// maxProvisional caps the supersede-tracking map (see the bounding note above).
const maxProvisional = 8192

// The R0.4b paid-serial guard's DERIVED cap (economist advisory §3, residual
// R-ECON-2; re-derived for R2.9, Researcher certification
// R2.9-build-questions-domain-rescale-guard-2026-09-04 §4.2, and corrected by the
// G-R212-8 certification of 2026-09-06, T-QUANT). The guard holds TWO populations,
// both spent at session OPEN since R2.14 / R2.9: delivery anchors and relay anchors. The
// cap must DOMINATE the honest live set so that EXPIRY — not the cap — does the
// eviction work. Two bounds on that live set, and which one binds:
//
//  1. The BYTE-RATE corner (φ = 1: every anchor fully consumed). One anchor funds
//     DeliveryBytesPerAnchor / RelayBytesPerAnchor bytes, and an entry lives (W+1)
//     epochs of E blocks of T_b seconds, so at a serve rate R
//
//     live_φ=1 = (W+1)·E·T_b·R · (1/DeliveryBytesPerAnchor + 1/RelayBytesPerAnchor)
//
//     T_b is UNMEASURED and not a constant (blocks are proposed on demand —
//     core/node/chainrole.go — never on a clock; owed to the Tester with
//     R-REAPER-FORFEIT), so this corner takes an upper bound on it. At 125 MiB/s and a
//     one-hour interval it is 2,160 entries; 4× headroom is 8,640. This term bounds
//     ONLY the φ = 1 corner (T-QUANT): under spend-at-open an anchor is consumed per
//     SESSION, and the certified per-session face consumption on the real fetch path is
//     φ ≈ 5×10⁻⁴ … 8×10⁻³, which puts the byte-rate form 2.7×–43× OVER the floor.
//     It is kept as the derivation's lower corner, not as a dominance claim.
//
//  2. The SESSION-COUNT bound, which is what actually caps occupancy under G-6
//     (remainder burned): a guard slot costs one face, so an identity holds at most
//     ⌊g/f⌋ = 10 live anchors, and occupancy is `A · (g/f) · (W+1)` of the
//     fresh-identity arrival rate A the faucet bounds (R2.12). That relation is
//     enforced at start-up — `capacity × (grant/fee) × (W+1) ≤ MaxPaidSerial/4`,
//     cmd/silt/daemon.go, gated by cmd/silt/r212_faucet_test.go — against the
//     RUNTIME faucet capacity, which no compile-time constant here can see. Under the
//     RATIFIED refund of the delivery remainder (D-R2.9-NODE-HALF-CALLS 1′, 2026-09-07)
//     the refund is released at ANCHOR EXPIRY — the same window this guard sweeps on —
//     so a face is accounting-neutral and the (g/f) term keeps bounding CONCURRENCY:
//     the R2.12 assertion survives verbatim (T-DEPOSIT, refund certification §3–4). The
//     per-identity live-anchor cap once proposed here is REFUTED (a bearer anchor passed
//     down fresh keypairs evades it for zero credits) and is NOT built
//     (R-DELIVERY-BURN-PRICES-THE-GUARD re-priced, R-STOCK-RENEWABLE-OCCUPANCY open).
//
// So: the cap is max(the 65,536 floor, headroom × the φ = 1 corner) in code, and the
// floor is what binds today; dominance under quantization is re-derived with the node
// half against the session-count bound, never claimed from the byte term
// (TestPaidSerialCapDominatesBothPopulations pins the corner arithmetic so the retired
// "256 object-aware serves per block" COUNT reddens it).
//
// At ~90 B/entry the 65,536 floor is ~5.6 MB — three orders of magnitude inside the
// 2 GB floor box, so the cap is sized to NEVER deny an honest lane rather than to
// save bytes that do not need saving (build-immutable #8 is satisfied by the BOUND,
// not by making it small). Keeping a bare 8192 alongside a 4-epoch window is the ONE
// combination to avoid: the cap would then evict a still-in-window (still-redeemable)
// entry, which is exactly the refuted design.
//
// R-GUARD-SHARED-FILL (liveness only): a faucet-funded flood of opens can fill the
// shared guard for ≤ W+1 epochs and both lanes REFUSE, never evict. R2.12 re-prices
// it through bound 2; the composed relation `C ≤ maxPaidSerial · (f/r) / ((W+1) ·
// B_floor)` (Researcher certification R2.12-faucet-rate-tier-and-grant-ratio-composition
// §3.2) is what closes it. Per-lane refusal counters and live counts
// (GuardFullRefusalsByLane, LivePaidSerialsByLane) show an operator WHICH population
// fills the guard (2026-09-04 cert §4.4).
const (
	// paidSerialWindow is W in epochs — the demand-token validity window. It MUST
	// equal demand.DefaultWindow: if this one is SMALLER the guard sweeps a serial the
	// demand layer will still verify, and the self-financing eviction pump re-opens
	// (measured: at window 2 vs 4 a second server re-collects an evicted serial for a
	// full payout). It is duplicated rather than imported because core/credit carries
	// no PRODUCTION dependency on core/demand; the two copies are pinned together by
	// TestPaidSerialWindowMatchesDemandWindow and by the behavioural seam gate
	// TestGuardLifetimeMatchesDemandKeysetLifetime, both in
	// paidserial_window_pin_test.go, which import core/demand in TEST code only.
	paidSerialWindow = uint64(4)
	// paidSerialEpochBlocks is the epoch cadence the cap is derived against
	// (DerivedEpochBlocks).
	paidSerialEpochBlocks = 8
	// maxPaidSerialFloor is the generous minimum the derived cap is floored at.
	maxPaidSerialFloor = 65_536

	// The two anchor faces the populations are quantized in. CapAnchorFace is the
	// shipped fee (relaypay.ShippedAnchorFace: face = Fee(), an identity with the burn)
	// and CapRelayBytesPerCredit the relay lane's price
	// (relaypay.RelayIncrementBytes/RelayIncrementCredit). Both are duplicated literals —
	// core/credit imports neither core/relaypay nor cmd/silt — pinned to their sources
	// by TestPaidSerialCapLiteralsMatchTheirSources in cmd/silt, the one package that
	// can import all three.
	CapAnchorFace          = 50_000
	CapRelayBytesPerCredit = 524_288
	// capServeRateBytesPerSec is the target serve rate the φ = 1 corner is sized for:
	// 125 MiB/s (131,072,000 B/s, ≈ 1.05 Gbit/s).
	capServeRateBytesPerSec = 125 << 20
	// capBlockIntervalBoundSec is the T_b upper bound the derivation must dominate
	// under (one hour). Not a measurement — a bound the gate proves generous.
	capBlockIntervalBoundSec = 3600
	// capHeadroom is the multiple of the honest live set the cap must clear (cert §4.2: ≥ 4).
	capHeadroom = 4
)

// DeliveryBytesPerAnchor is ⌊f/p⌋·U: the bytes ONE delivery anchor funds at one server
// (12.21 GiB at the ratified price). RelayBytesPerAnchor is the relay twin, f × the
// relay price (24.4 GiB). Both exported read-only for the session ceilings in core/node.
const (
	DeliveryBytesPerAnchor = int64(CapAnchorFace/DeliveryIncrementCredit) * DeliveryIncrementBytes
	RelayBytesPerAnchor    = int64(CapAnchorFace) * CapRelayBytesPerCredit
)

// capWindowSeconds is the guard entry's lifetime in seconds at the T_b bound; the two
// φ = 1 populations are that many seconds of serving / relaying at the target rate,
// quantized by their anchors (bound 1 above — the corner, not the binding bound).
const (
	capWindowSeconds      = int64(paidSerialWindow+1) * paidSerialEpochBlocks * capBlockIntervalBoundSec
	capLiveHonestDelivery = capWindowSeconds * capServeRateBytesPerSec / DeliveryBytesPerAnchor
	capLiveHonestRelay    = capWindowSeconds * capServeRateBytesPerSec / RelayBytesPerAnchor
	derivedPaidSerialCap  = capHeadroom * (capLiveHonestDelivery + capLiveHonestRelay)
)

// maxPaidSerial is the derived cap, floored at maxPaidSerialFloor. Derived, never a
// bare constant — see above.
const maxPaidSerial = int(max(maxPaidSerialFloor, derivedPaidSerialCap))

// MaxPaidSerial and PaidSerialWindow are exported READ-ONLY for the R2.12 start-up
// assertion in cmd/silt: `capacity × (grant/fee) × (W+1) ≤ MaxPaidSerial/4` ties the
// faucet's burst, the grant, the fee and the guard cap — four constants in three packages
// re-tuned by three different processes — so raising any one of them past the guard's
// cliff refuses to start instead of silently opening a hole (economist §2.3).
const (
	MaxPaidSerial    = maxPaidSerial
	PaidSerialWindow = paidSerialWindow
)

// paidSerialEntry is one guarded serial: the server that collected its single
// conserved payout, and the epoch whose issuer key signed the token. The epoch is the
// EXPIRY key — the only thing eviction is allowed to act on. See credit.go's
// paidSerial note and sweepExpiredSerials.
type paidSerialEntry struct {
	server ports.NodeID
	epoch  uint64
	lane   guardLane // which population the entry belongs to (observability only)
}

// guardLane names the population a guard entry belongs to, for the per-lane counters
// the 2026-09-04 certification §4.4 requires. Never read by an accounting rule.
type guardLane uint8

const (
	laneDelivery guardLane = iota // a paid delivery serial (flat redeem) or a delivery anchor (R2.9)
	laneRelay                     // a relay anchor (R2.14)
)

// provKey identifies one delivery lane: server served object root to requester.
// The receipt's Fetcher key hashes to the requester NodeID (NodeID =
// sha256(pubkey)), which is what lets a redeem find the serve.
//
// server is part of the identity so distinct servers writing the SAME (root,
// requester) into ONE ledger get distinct lanes (RT-DELIV-3). Per-node prod has
// a single server per ledger (server = n.id), so this field is constant there
// and the shape is unchanged in effect. But the shared-ledger SIM routes every
// operator's serves into one Ledger; without server in the key, serves of the
// same object to the same fetcher by two servers collide on one lane, and the
// terminal reversal (redeem or eviction) then debits ONE server's provisional
// mint for work the other did — a conservation break that reverses or pays the
// wrong account. server in the key keeps each (server, requester, root) its own
// lane, so every reversal hits the exact account that was credited.
type provKey struct {
	server    ports.NodeID
	requester ports.NodeID
	root      ports.Hash
}

// provisionalServe is the self-credit a serve recorded before any receipt: the
// net balance the server credited itself and the skim it routed to the object's
// escrow. A redeem reverses both, then pays the conserved credit instead.
//
// server is the account that received the self-mint. The redeem path is told the
// server by its caller, but EVICTION has only the lane, so the server is stored
// here to let the eviction reversal (A4 fix) debit the exact account that was
// credited at serve time.
type provisionalServe struct {
	server ports.NodeID
	bytes  int64 // the lane's byte accumulator (G-R212-7): every serve on this lane, lifetime of the lane
	net    int64 // credits minted to the server so far = ⌊7·bytes/(8·Dλ)⌋ (reversed exactly on supersede/eviction)
	skim   int64 // credits skimmed to the escrow so far = ⌊bytes/(8·Dλ)⌋
}

// reverseProvisional undoes a lane's eager self-mint in full: it debits the server's
// balance by p.net and reduces the object's escrow by p.skim, floored at what the
// reserve still holds. It is the whole-lane form used by BOTH terminal-reversal sites:
// the flat redeem-in-window and eviction. The R2.9 settlement reverses PER INCREMENT
// through reverseLane directly (deliveryanchor.go). Keeping one implementation is
// what makes the escrow floor identical at every site.
func (l *Ledger) reverseProvisional(server ports.NodeID, root ports.Hash, p *provisionalServe) {
	l.reverseLane(server, root, p.net, p.skim)
}

// reverseLane debits net from the server's balance and claws skim back from the
// object's escrow, floored at what the reserve still holds (a bounty paid out between
// serve and reversal is real durability work, never recoverable — build-immutable #2).
// Purely subtractive: the worst case on any path through it is an UNDER-pay.
func (l *Ledger) reverseLane(server ports.NodeID, root ports.Hash, net, skim int64) {
	l.serveReversedCredits += net + skim
	l.acct(server).balance -= net
	if e, eok := l.escrow[root]; eok {
		r := skim
		if r > e.balance {
			r = e.balance
		}
		e.balance -= r
		// A4-1: a reversal only ever claws back the object's own auto-SKIM, never an
		// operator's prepay. Floored at zero defensively; r is already bounded by the
		// lane's recorded skim and by the reserve.
		e.fundedSkim -= r
		if e.fundedSkim < 0 {
			e.fundedSkim = 0
		}
	}
}

// laneFor returns the provisional lane for an object-aware serve, creating it (and
// FIFO-evicting the oldest live lane when the map is at cap) on first touch. Called by
// RecordServeToObject only, which then floors the lane's byte accumulator into credits.
//
// EVICTION REVERSAL (A4 money-pump fix, Boulder 0, R0.4a): when the map is at
// cap and the oldest lane is FIFO-evicted, its eager self-mint is REVERSED
// before the lane is forgotten — not left on the server's balance. This closes
// the double-pay: after eviction an evicted lane is in the same accounting state
// as "never served" (mint gone, skim floored-reversed, entry gone), so a later
// redeem for that lane pays the conserved leg ONLY and mints nothing. Rule (b):
// one delivery, one payment. The cost is the unwitnessed bilateral fallback —
// an evicted, never-redeemed serve loses its self-record — an under-pay at the
// >maxProvisional tail, never an over-pay, never a denial. Verified by
// TestA4MoneyPumpConservation. Design:
// docs/thinking/2026-09-01-a4-provisional-eviction-fix-design.md.
func (l *Ledger) laneFor(server, requester ports.NodeID, root ports.Hash) *provisionalServe {
	k := provKey{server: server, requester: requester, root: root}
	p, ok := l.provisional[k]
	if !ok {
		// FIFO-evict the oldest LIVE lane while the map is at cap. Tombstones
		// (nil, left by a redeem) are skipped, never counted as an eviction —
		// they carry no lane to reverse. The front is advanced by the provHead
		// CURSOR, not by re-slicing (provOrder[1:]). Re-slicing would shift every
		// survivor down one physical position while provIndex still held the old
		// positions — the desync the fuzz caught (seed 0xdeadbeef0002 step
		// 13154), where removeFromProvOrder then tombstoned a live slot and a
		// ghost index entry reversed a re-served lane's self-mint. Advancing the
		// cursor and nilling the dropped slot leaves every survivor's absolute
		// position (and thus provIndex) untouched: amortized O(1), no survivor
		// index rewrite.
		for len(l.provisional) >= maxProvisional {
			for l.provHead < len(l.provOrder) && l.provOrder[l.provHead] == nil {
				l.provHead++
			}
			if l.provHead >= len(l.provOrder) {
				break
			}
			old := *l.provOrder[l.provHead]
			l.provOrder[l.provHead] = nil
			l.provHead++
			delete(l.provIndex, old)
			if evicted, eok := l.provisional[old]; eok {
				l.serveBytesLaneEvicted += evicted.bytes // A2: the confiscated wage (credit.go)
				l.reverseProvisional(evicted.server, old.root, evicted)
				delete(l.provisional, old)
			}
		}
		p = &provisionalServe{server: server}
		l.provisional[k] = p
		kk := k
		l.provOrder = append(l.provOrder, &kk)
		l.provIndex[k] = len(l.provOrder) - 1
		l.compactProvOrder()
	}
	return p
}

// removeFromProvOrder drops a lane's FIFO entry in O(1) by tombstoning its slot
// (nil) via the position index. Called at every map removal that is NOT an
// eviction (redeem) so provOrder never retains a key whose live lane is gone.
// The tombstones are reclaimed by compactProvOrder. This is the single sync
// point that closes RT-DELIV-1/1b/2: the map and the live entries of provOrder
// stay in lockstep, and no stale key can survive to reverse a re-served lane.
func (l *Ledger) removeFromProvOrder(k provKey) {
	if i, ok := l.provIndex[k]; ok {
		l.provOrder[i] = nil
		delete(l.provIndex, k)
	}
}

// compactProvOrder keeps provOrder bounded on BOTH churn paths. On the
// redeem-heavy path the tombstones are redeem-left nils; on the eviction-heavy
// path they are the nils the provHead cursor leaves behind as it advances (the
// cursor never re-slices, so the physical slice grows until compaction reclaims
// it). When the slice grows past 2*maxProvisional it is rebuilt with the
// tombstones dropped, the index repointed to the fresh positions, and the
// provHead cursor reset to 0 (the rebuild drops the whole dead prefix, so there
// is no logical front left to skip). Compaction touches at most len(provOrder)
// entries but runs only once every ~maxProvisional appends, so it amortizes to
// O(1) per serve and never scans on the hot redeem path. The slice is thereby
// capped at 2*maxProvisional — bounded state on the floor box (build-immutable
// #8).
func (l *Ledger) compactProvOrder() {
	if len(l.provOrder) <= 2*maxProvisional {
		return
	}
	live := l.provOrder[:0:0]
	for _, kp := range l.provOrder {
		if kp != nil {
			live = append(live, kp)
			l.provIndex[*kp] = len(live) - 1
		}
	}
	l.provOrder = live
	l.provHead = 0
}

// The REASON a delivery redeem paid nothing. Every non-paying path used to return a
// bare 0, so an operator could not tell "this server exceeded the serve rate the
// guard was sized for" from the ordinary no-pay cases — and the node logged the
// refusal under a line reading "delivery receipt banked", which is actively
// misleading because nothing was banked (Tester finding, 2026-09-02).
//
// A plain string, not a typed enum: the value is log output, so a string keeps the
// optional ports-side interface (core/node's deliveryReasoner) free of a core/credit
// type and keeps the log line and the test assertion the same literal. The values
// are an announced observable contract (S5) — do not rename one without treating it
// as a marker change. Observability ONLY: no accounting rule reads them, and
// standing never does.
const (
	// ReasonPaid: the conserved credit was settled.
	ReasonPaid = "paid"
	// ReasonSelfDelivery: server == fetcher.
	ReasonSelfDelivery = "self-delivery"
	// ReasonAlreadyPaid: this serial already funded one conserved payout.
	ReasonAlreadyPaid = "serial-already-paid"
	// ReasonBackdated: the issuing epoch has left the window at the ledger's
	// monotone watermark.
	ReasonBackdated = "token-backdated"
	// ReasonGuardFull: the paid-serial guard is full of STILL-LIVE serials. This is
	// the one that means "the serve rate exceeded the modeled bound the cap was
	// derived against" — the operator-visible signal, counted by GuardFullRefusals.
	ReasonGuardFull = "paid-serial-guard-full"
	// ReasonNoFee: the ledger's fee is zero, so there is nothing to settle.
	ReasonNoFee = "no-fee"
	// ReasonGuardUnloaded: a durable guard store is attached but its contents have
	// not been loaded yet, so this ledger does not know what it already paid. Refuse
	// rather than pay — a redeem before load completes is exactly the restart window
	// the store exists to close (red-team re-break F2).
	//
	// NOT AN OPERATOR SIGNAL — it is UNREACHABLE on the shipped daemon (PE ruling §4,
	// correction 2, 2026-09-03). cmd/silt opens and loads the store BEFORE the node
	// exists and returns a start-up error if the load fails, so no receipt can arrive
	// with the guard attached-but-unloaded. It is correct defence in depth for an
	// EMBEDDER that wires the ledger itself, and the record must not count it as
	// something an operator will ever see.
	ReasonGuardUnloaded = "paid-serial-guard-unloaded"
	// ReasonGuardStore: the durable guard entry could not be written. The entry is
	// persisted BEFORE any credit moves, so a store failure is an under-pay, never a
	// payout with no guard entry.
	ReasonGuardStore = "paid-serial-store-write-failed"
)

// GuardFullRefusals is how many redeems this ledger refused because the paid-serial
// guard was full of still-live serials — the counter behind ReasonGuardFull.
// Observability only; monotone.
func (l *Ledger) GuardFullRefusals() int64 { return l.guardFullRefusals }

// GuardFullRefusalsByLane splits GuardFullRefusals by the population that was refused
// (2026-09-04 certification §4.4; R2.9 gate B-4): delivery = paid delivery serials and
// delivery anchors, relay = relay anchors. Observability only.
func (l *Ledger) GuardFullRefusalsByLane() (delivery, relay int64) {
	return l.guardFullRefusalsDelivery, l.guardFullRefusalsRelay
}

// LivePaidSerialsByLane counts the guard's live entries per population, so an operator
// can see WHICH lane fills a shared guard (R-GUARD-SHARED-FILL). Entries restored from
// the durable store carry no lane and count as delivery. Bounded by the cap; reading
// moves nothing.
func (l *Ledger) LivePaidSerialsByLane() (delivery, relay int64) {
	for _, e := range l.paidSerial {
		if e.lane == laneRelay {
			relay++
		} else {
			delivery++
		}
	}
	return delivery, relay
}

// SerialSweeps is how many times the guard's expiry sweep has actually scanned the
// map. Bounded to one per epoch (see the sweptEpoch note in credit.go); the gate on
// that bound counts sweeps, not time.
func (l *Ledger) SerialSweeps() int64 { return l.sweeps }

// CompactFailures is how many expiry sweeps ended with the durable guard store
// refusing to compact, and LastCompactError is the most recent such error (nil when
// none). Observability only; monotone. A compaction failure is BENIGN for
// accounting (the log stays a superset of the live set, see sweepExpiredSerials) but
// an operator should see it: the file is no longer shrinking, and a store that is
// actually broken also refuses every Append, which surfaces as ReasonGuardStore.
func (l *Ledger) CompactFailures() int64  { return l.compactFailures }
func (l *Ledger) LastCompactError() error { return l.lastCompactErr }

// sweepExpiredSerials drops every guarded serial whose issuing epoch has left the
// validity window at current. THIS IS THE ONLY EVICTION PATH, and that is the whole
// R0.4b fix.
//
// Why expiry-only is load-bearing: the demand layer rejects a token whose issuing
// epoch has left the window BEFORE any credit path (no held key_E verifies it), so a
// serial dropped here is one no honest or dishonest party can ever redeem again. The
// evicted set and the expired set are the SAME set — the coupling condition the
// R0.4b certification (Verdict 1) requires for "evicted ⇒ expired ⇒ un-redeemable".
//
// The REFUTED alternative was FIFO eviction: it forgets a still-in-window serial, and
// re-collecting a whole evicted window is self-financing (each flood serial is itself
// a paid delivery the colluding operator collects, so advancing the FIFO costs
// nothing). Red-team 2026-09-02; pinned RED-before/GREEN-after by
// TestSerialGuard_EvictThenReRedeemMintsZero and
// TestSerialGuard_EvictionPumpIsNotSelfFinancing, which must both hold WITH
// TestSerialGuard_SetIsBounded — the triple no FIFO-alone design can satisfy.
//
// Deterministic: driven by the epoch stored per entry, never by map iteration order
// (B2). Nothing observable depends on WHICH entries go, only on which epochs expired.
func (l *Ledger) sweepExpiredSerials(current uint64) {
	if current <= paidSerialWindow {
		return // nothing can have left the window yet
	}
	floor := current - paidSerialWindow
	removed := false
	for k, p := range l.paidSerial {
		if p.epoch < floor {
			delete(l.paidSerial, k)
			removed = true
		}
	}
	// The durable log is append-only, so an expiry sweep is the ONE event that shrinks
	// it. Compacting here (and only here) keeps the file within 2x the live cap while
	// costing nothing on the redeem path.
	//
	// TWO ERROR CLASSES, NOT ONE (R2.13, PE ruling
	// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1). A failed
	// compaction is not an accounting error: the port contract (ports.PaidSerialStore,
	// the handle clause) guarantees the store is then EITHER still appendable with the
	// log a superset of the live set — which only ever refuses more — OR failing every
	// Append. So a Compact error is recorded here (the WARN; this package has no
	// logger, counters are its observability surface) and never refuses a payout by
	// itself. A store that is actually broken reports it the one way the anchor spend
	// already refuses on: Append fails, and spendAnchors returns ReasonGuardStore. Refusing on every Compact error instead was REFUSED by the
	// ruling: it is a self-inflicted liveness break at exactly the load where a benign
	// compaction fails. Gates: G-CO-2 (benign failure still pays) and G-CO-3 (broken
	// store pays 0).
	if removed && l.paidStore != nil {
		if err := l.paidStore.Compact(l.livePaidSerials()); err != nil {
			l.compactFailures++
			l.lastCompactErr = err
		}
	}
}

// livePaidSerials is the guard's contents in a DETERMINISTIC order (by key), so a
// compaction never depends on Go map iteration order (B2).
func (l *Ledger) livePaidSerials() []ports.PaidSerial {
	keys := make([]string, 0, len(l.paidSerial))
	for k := range l.paidSerial {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ports.PaidSerial, 0, len(keys))
	for _, k := range keys {
		e := l.paidSerial[k]
		out = append(out, ports.PaidSerial{Serial: serialOfPaidKey(k), Server: e.server, Epoch: e.epoch, Relay: e.lane == laneRelay})
	}
	return out
}

// reservePaidSerial makes room for one more guarded serial, or reports that it
// cannot. It sweeps expired entries first and only then checks the cap; it NEVER
// evicts a live entry. false means "the cap is full of still-redeemable serials",
// which the caller turns into a refusal to open a session (see spendAnchors).
func (l *Ledger) reservePaidSerial(current uint64) bool {
	return l.reservePaidSerials(current, 1)
}

// reservePaidSerials is reservePaidSerial for k entries at once — a relay open
// spends up to relaypay.MaxAnchorsPerSession anchors all-or-nothing, so the whole
// batch must fit or none is recorded (R2.14 SpendRelayAnchors). Same rule: sweep
// expired entries, then check the cap, never evict a live entry.
func (l *Ledger) reservePaidSerials(current uint64, k int) bool {
	if len(l.paidSerial)+k <= maxPaidSerial {
		return true
	}
	l.sweepIfEpochAdvanced(current)
	return len(l.paidSerial)+k <= maxPaidSerial
}

// sweepIfEpochAdvanced runs sweepExpiredSerials AT MOST ONCE PER EPOCH (RT-E).
// Entries expire on the epoch clock, so a second scan within the same epoch can free
// nothing a first scan did not: the swept set is identical and the amortized cost is
// O(cap) per epoch rather than O(cap) per caller. Epoch 0 needs no latch —
// sweepExpiredSerials returns immediately while current <= W, so nothing can expire
// that early.
//
// Two callers drive it: the epoch-watermark advance on the redeem path (advisory C-7,
// the retention bound) and reservePaidSerial at the cap (the liveness bound).
func (l *Ledger) sweepIfEpochAdvanced(current uint64) {
	if current > l.sweptEpoch {
		l.sweptEpoch = current
		l.sweeps++
		l.sweepExpiredSerials(current)
		l.releaseDueRefunds() // R2.9 M2: deposits whose anchors just left the window return now
	}
}

// paidKey is the guard key: the token, not the serial. It is
// uint64BE(issueEpoch)||serial — the same epoch wire form the demand FDH message and
// the issuerKeyCommit leaf use.
//
// WHY THE EPOCH IS IN THE KEY (red-team re-break F3, 2026-09-03). Keyed by the serial
// alone, the entry's expiry epoch was whatever the FIRST redeem supplied and later
// redeems returned early — i.e. the MINIMUM epoch over the tokens sharing a serial.
// The withdrawer picks both the serial and the epoch, so it can hold two valid tokens
// on ONE serial at two epochs; the low-epoch entry then expired and the guard forgot a
// serial for which a still-in-window token existed. "Evicted ⇒ expired" was false.
// Keyed by the token, an entry is removed only once ITS OWN issue epoch is outside the
// band, which is the coupling condition the certification requires. The pump the guard
// closes is unaffected: the same TOKEN redeemed at two servers is one key either way,
// and a second token on the same serial costs a second withdrawal fee.
func paidKey(issuedEpoch uint64, serial []byte) string {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], issuedEpoch)
	return string(k[:]) + string(serial)
}

// serialOfPaidKey recovers the serial from a guard key (the inverse of paidKey's
// suffix), for the durable store's records.
func serialOfPaidKey(k string) []byte { return []byte(k[8:]) }

// addPaidSerial marks the TOKEN (issue epoch, serial) as having funded one conserved
// delivery payout on this ledger, recording the server that collected it. A
// zero-length serial is not recorded — it was unguarded on entry, so recording it
// would waste a slot.
//
// The cap is enforced by the caller's reservePaidSerial before any payment, so this
// never has to choose a victim: by the time it runs there is a free slot.
//
// It returns the DURABLE-WRITE error, and the caller refuses the payout on one. See
// the call site for why the order matters.
func (l *Ledger) addPaidSerial(serial []byte, server ports.NodeID, issuedEpoch uint64, lane guardLane) error {
	if len(serial) == 0 {
		return nil
	}
	key := paidKey(issuedEpoch, serial)
	if _, ok := l.paidSerial[key]; ok {
		return nil // already recorded (the guard above already refused a re-redeem)
	}
	if l.paidStore != nil {
		if err := l.paidStore.Append(ports.PaidSerial{Serial: serial, Server: server, Epoch: issuedEpoch, Relay: lane == laneRelay}); err != nil {
			return err
		}
	}
	l.paidSerial[key] = paidSerialEntry{server: server, epoch: issuedEpoch, lane: lane}
	return nil
}

// SetPaidSerialStore attaches the durable guard store and marks the guard UNLOADED.
// Until LoadPaidSerials succeeds every guarded redeem is refused (ReasonGuardUnloaded)
// rather than paid: a ledger that does not yet know what it already paid must not pay.
func (l *Ledger) SetPaidSerialStore(s ports.PaidSerialStore) {
	l.paidStore = s
	l.guardLoaded = false
}

// LoadPaidSerials restores the guard from its durable store. It is the RESTORE half of
// "a restart is not an eviction"; the daemon calls it before the node accepts any
// receipt.
//
// A store holding more than the cap is a REFUSE-TO-START error, not a truncation:
// dropping the surplus would be exactly the arbitrary eviction of live entries the
// design refutes, and no ledger writing through this store can produce such a file.
func (l *Ledger) LoadPaidSerials() error {
	if l.paidStore == nil {
		return nil
	}
	entries, err := l.paidStore.Load()
	if err != nil {
		return err
	}
	fresh := make(map[string]paidSerialEntry, len(entries))
	var deliveryEntries int64
	for _, e := range entries {
		if len(e.Serial) == 0 {
			continue
		}
		lane := laneDelivery
		if e.Relay {
			lane = laneRelay
		} else {
			deliveryEntries++
		}
		fresh[paidKey(e.Epoch, e.Serial)] = paidSerialEntry{server: e.Server, epoch: e.Epoch, lane: lane}
	}
	if len(fresh) > maxPaidSerial {
		return fmt.Errorf("credit: persisted paid-serial guard holds %d entries, cap is %d",
			len(fresh), maxPaidSerial)
	}
	l.paidSerial = fresh
	// R2.9 (G-6R-9): each restored DELIVERY entry is an anchor whose session state did not
	// survive the restart (D-FP2-SCOPE: sessions and deposits are ephemeral; the guard is
	// durable so nothing is re-spent), so the count is an upper bound on lost deposits —
	// surfaced at boot and on /api/status so the loss is operator-visible, never silent.
	// Relay anchors are EXCLUDED: they keep the burn and never had a deposit, and the
	// durable record now carries the lane that says which is which
	// (R-GUARD-RESTORE-LANE-UNKNOWN closed; the pre-bump restore counted both).
	l.deliveryRestartOrphans = deliveryEntries
	l.guardLoaded = true
	return nil
}
