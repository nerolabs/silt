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
// The serve path self-records 1 credit/byte as it serves (RecordServeToObject)
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
// Boulder 0, R0.4a — trackProvisional below), so an evicted lane is left in the
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

import "github.com/nerolabs/silt/ports"

// maxProvisional caps the supersede-tracking map (see the bounding note above).
const maxProvisional = 8192

// The R0.4b paid-serial guard's DERIVED cap (economist advisory §3, residual
// R-ECON-2). The cap must DOMINATE the honest live set so that EXPIRY — not the cap
// — does the eviction work. A live paid serial is one whose issuing epoch is still
// in the validity window, so the live set is bounded by what this server can itself
// serve in that time: serveRate x W x EpochBlocks. Keeping a bare 8192 alongside a
// 4-epoch window is the ONE combination to avoid: the cap would then evict a
// still-in-window (still-redeemable) lane, which is exactly the refuted design.
//
// At ~90 B/serial the 65,536 floor is ~5.6 MB — three orders of magnitude inside the
// 2 GB floor box, so the cap is sized to NEVER deny an honest lane rather than to
// save bytes that do not need saving (build-immutable #8 is satisfied by the BOUND,
// not by making it small).
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
	// maxServeTrackedPerBlock is the per-block object-aware serve rate the cap is
	// sized for.
	maxServeTrackedPerBlock = 256
	// maxPaidSerialFloor is the generous minimum the derived cap is floored at.
	maxPaidSerialFloor = 65_536
)

// maxPaidSerial is the derived cap: W x EpochBlocks x maxServeTrackedPerBlock,
// floored at maxPaidSerialFloor. Derived, never a bare constant — see above.
const maxPaidSerial = max(maxPaidSerialFloor,
	int(paidSerialWindow)*paidSerialEpochBlocks*maxServeTrackedPerBlock)

// paidSerialEntry is one guarded serial: the server that collected its single
// conserved payout, and the epoch whose issuer key signed the token. The epoch is the
// EXPIRY key — the only thing eviction is allowed to act on. See credit.go's
// paidSerial note and sweepExpiredSerials.
type paidSerialEntry struct {
	server ports.NodeID
	epoch  uint64
}

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
	net    int64
	skim   int64
}

// reverseProvisional undoes a lane's eager self-mint: it debits the server's
// balance by p.net and reduces the object's escrow by p.skim, floored at what
// the reserve still holds (a bounty paid out between serve and reversal is real
// durability work, never recoverable — build-immutable #2). It is the single
// reversal used by BOTH terminal-reversal sites: redeem-in-window and eviction.
// Keeping one implementation is what makes the escrow floor identical at both.
func (l *Ledger) reverseProvisional(server ports.NodeID, root ports.Hash, p *provisionalServe) {
	l.acct(server).balance -= p.net
	if e, eok := l.escrow[root]; eok {
		r := p.skim
		if r > e.balance {
			r = e.balance
		}
		e.balance -= r
		e.funded -= r
		if e.funded < 0 {
			e.funded = 0
		}
	}
}

// trackProvisional records an object-aware serve's self-credit for later
// supersession. Called by RecordServeToObject only.
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
func (l *Ledger) trackProvisional(server, requester ports.NodeID, root ports.Hash, net, skim int64) {
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
	p.net += net
	p.skim += skim
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

// RedeemDeliveryCredit settles a WITNESSED delivery (a banked, verified
// receipt): it supersedes the provisional self-record for (fetcher, root), then
// pays the server the conserved credit — the withdrawal fee less the durability
// skim, with the skim routed to the object's escrow (DECIDED 2026-08-26,
// D-POD-KNOBS: escrow over burn, for the cross-tier funding loop — edge
// delivery skim funds that content's durability on the persistent tier;
// safety rests on the reversal floor below). Returns the credits paid to the
// server. Self-delivery pays nothing, same guard as RecordServe. It never
// touches standing.
// serial is the redeemed receipt's token serial and issuedEpoch is the epoch whose
// issuer key signed that token (discovered at demand.Bank.Redeem — it rides no field
// on the token). currentEpoch is the consensus epoch at the head. Together they close
// the CROSS-SERVER DOUBLE-REDEEM: one demand token (one blind withdrawal, one serial,
// one fee) funds exactly ONE conserved payout, so K colluding servers sharing one
// token can no longer mint (K−1)·fee. See the paidSerial note in credit.go.
func (l *Ledger) RedeemDeliveryCredit(server, fetcher ports.NodeID, root ports.Hash,
	serial []byte, issuedEpoch, currentEpoch uint64) int64 {
	paid, _ := l.RedeemDeliveryCreditReason(server, fetcher, root, serial, issuedEpoch, currentEpoch)
	return paid
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
)

// GuardFullRefusals is how many redeems this ledger refused because the paid-serial
// guard was full of still-live serials — the counter behind ReasonGuardFull.
// Observability only; monotone.
func (l *Ledger) GuardFullRefusals() int64 { return l.guardFullRefusals }

// SerialSweeps is how many times the guard's expiry sweep has actually scanned the
// map. Bounded to one per epoch (see the sweptEpoch note in credit.go); the gate on
// that bound counts sweeps, not time.
func (l *Ledger) SerialSweeps() int64 { return l.sweeps }

// RedeemDeliveryCreditReason is RedeemDeliveryCredit plus the reason it paid
// nothing. See the Reason* constants.
func (l *Ledger) RedeemDeliveryCreditReason(server, fetcher ports.NodeID, root ports.Hash,
	serial []byte, issuedEpoch, currentEpoch uint64) (int64, string) {
	if server == fetcher {
		return 0, ReasonSelfDelivery // self-delivery earns nothing (the cheapest gaming, blocked)
	}

	// R0.4b cross-server double-redeem guard. The FIRST completed redeem of a serial
	// pays; any later redeem of the SAME serial — by ANY server on this ledger,
	// including a second colluding server holding its own self-named receipt — mints
	// nothing. A refused redeem returns BEFORE the supersede/pay block, so the
	// object-aware serve keeps its unwitnessed self-record (the legitimate bilateral
	// fallback) exactly as if no valid receipt had arrived.
	//
	// Legit abort-retry survives: an aborted server delivers nothing, banks nothing,
	// and never reaches here, so the honest completion at the retry server is the
	// FIRST redeem of that serial and pays (core/demand TestAbortLeavesTokenReusable).
	// The distinguisher is completed server-distinct redeems off ONE serial, not "was
	// the token reused".
	//
	// A missing serial (len == 0) is unguarded: no production caller redeems without
	// the receipt serial (core/node/demandrole.go), and this keeps the legacy /
	// unwitnessed test paths that never carried one working while the witnessed path
	// — the only pump surface — stays fully gated.
	if len(serial) > 0 {
		// R0.4b-5: advance the ledger's monotone epoch watermark, then run the whole
		// guard against IT rather than against this caller's view. On a shared ledger
		// a laggard redeemer must not be able to un-sweep or re-admit what a further-
		// ahead redeemer has already retired.
		if currentEpoch > l.epochWatermark {
			l.epochWatermark = currentEpoch
		}
		if _, paid := l.paidSerial[string(serial)]; paid {
			return 0, ReasonAlreadyPaid // this serial already funded one conserved payout — mint 0
		}
		// Backdated redeem: the issuing epoch has left the window measured at the
		// watermark, so some redeemer on this ledger is already past it and the
		// serial may have been swept. Refuse rather than pay a second time. Under-pay
		// only — an honest in-window redeem is unaffected because the watermark equals
		// its own current epoch.
		if issuedEpoch+paidSerialWindow < l.epochWatermark {
			return 0, ReasonBackdated
		}
		if !l.reservePaidSerial(l.epochWatermark) {
			// The guard set is full of STILL-LIVE (in-window, still-redeemable)
			// serials. Forgetting one to make room is exactly the refuted FIFO
			// design — it would re-open the self-financing eviction pump. Refuse to
			// pay instead: an UNDER-pay, never an over-pay and never a mint, and it
			// self-heals as the window advances. Reaching here requires a serve rate
			// above the modeled bound the cap was derived against, and the counter
			// plus the typed reason are what make that visible to an operator instead
			// of surfacing as an unexplained credit=0.
			l.guardFullRefusals++
			return 0, ReasonGuardFull
		}
	}

	s := l.acct(server)

	// Supersede: reverse this delivery's provisional self-credit, then forget the
	// lane. The reversal (escrow floored at what the reserve still holds — a bounty
	// paid out between serve and redeem is real durability work, not recoverable)
	// is shared verbatim with the eviction site. If the lane was already evicted,
	// its mint was reversed at eviction, so there is nothing here to reverse: the
	// redeem pays the conserved leg only (rule (b), one delivery one payment).
	k := provKey{server: server, requester: fetcher, root: root}
	if p, ok := l.provisional[k]; ok {
		l.reverseProvisional(server, root, p)
		delete(l.provisional, k)
		l.removeFromProvOrder(k) // keep provOrder in sync (RT-DELIV-1/1b/2 fix)
	}

	// Conservation: pay the fee the fetcher already paid in, less the skim.
	fee := l.fee
	if fee <= 0 {
		return 0, ReasonNoFee
	}
	skim := fee * SkimNum / SkimDen
	s.balance += fee - skim
	e := l.escrowFor(root)
	e.balance += skim
	e.funded += skim

	// Record this serial as funded so no OTHER server (and no re-submit) can redeem
	// the same token again. Recorded ONLY on the paying path — a fee<=0 no-op never
	// marks a serial, so an honest retry after a non-paying attempt is not wrongly
	// blocked.
	l.addPaidSerial(serial, server, issuedEpoch)
	return fee - skim, ReasonPaid
}

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
	for k, p := range l.paidSerial {
		if p.epoch < floor {
			delete(l.paidSerial, k)
		}
	}
}

// reservePaidSerial makes room for one more guarded serial, or reports that it
// cannot. It sweeps expired entries first and only then checks the cap; it NEVER
// evicts a live entry. false means "the cap is full of still-redeemable serials",
// which the caller turns into a refusal to pay (see RedeemDeliveryCredit).
func (l *Ledger) reservePaidSerial(current uint64) bool {
	if len(l.paidSerial) < maxPaidSerial {
		return true
	}
	// AT MOST ONE SWEEP PER EPOCH (RT-E). Entries expire on the epoch clock, so a
	// second scan within the same epoch can free nothing a first scan did not: the
	// swept set is identical and the amortized cost drops from O(cap) per refused
	// redeem to O(cap) per epoch. Epoch 0 needs no latch — sweepExpiredSerials
	// returns immediately while current <= W, so nothing can expire that early.
	if current > l.sweptEpoch {
		l.sweptEpoch = current
		l.sweeps++
		l.sweepExpiredSerials(current)
	}
	return len(l.paidSerial) < maxPaidSerial
}

// addPaidSerial marks serial as having funded one conserved delivery payout on this
// ledger, recording the server that collected it and the token's ISSUING EPOCH (the
// expiry key). A zero-length serial is not recorded — it was unguarded on entry, so
// recording it would waste a slot.
//
// The cap is enforced by the caller's reservePaidSerial before any payment, so this
// never has to choose a victim: by the time it runs there is a free slot.
func (l *Ledger) addPaidSerial(serial []byte, server ports.NodeID, issuedEpoch uint64) {
	if len(serial) == 0 {
		return
	}
	key := string(serial)
	if _, ok := l.paidSerial[key]; ok {
		return // already recorded (the guard above already refused a re-redeem)
	}
	l.paidSerial[key] = paidSerialEntry{server: server, epoch: issuedEpoch}
}
