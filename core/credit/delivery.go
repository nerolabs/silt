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

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// maxProvisional caps the supersede-tracking map (see the bounding note above).
const maxProvisional = 8192

// The R0.4b paid-serial guard's DERIVED cap (economist advisory §3, residual
// R-ECON-2). Since R2.14 the guard holds TWO populations — paid delivery serials AND
// spent relay anchors (k ≤ 6 per relay session) — so "the honest live set" below is
// serves + anchored sessions × k; the derivation was not re-priced for the second
// population (R-GUARD-SHARED-FILL, ROADMAP R2.14: a faucet-funded flood of relay opens
// can fill the shared guard for ≤ W+1 epochs and both lanes REFUSE, never evict —
// liveness only; closes with R2.12). The cap must DOMINATE the honest live set so that
// EXPIRY — not the cap — does the eviction work. A live paid serial is one whose issuing epoch is still
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
	// nothing.
	//
	// THE ORDERING INVARIANT (research certification 2026-09-03, gates G-4 and G-E):
	//
	//	EACH PROVISIONAL LANE INSTANCE IS REVERSED AT MOST ONCE, SO REVERSALS NEVER
	//	EXCEED SERVES. A RECEIPT REVERSES AT MOST ONCE PER LANE INSTANCE.
	//
	// The supersede deletes the lane, so a re-presentation with no intervening serve
	// moves nothing. A receipt that was REFUSED (and therefore never recorded in the
	// guard) can reverse again after a fresh serve of the same lane — that is the
	// accepted, purely-subtractive revenue grief, not a second payout. A receipt that
	// was PAID is recorded, so ReasonAlreadyPaid returns above the supersede and it can
	// never reverse again. What is bounded is the LANE INSTANCE, not the receipt.
	//
	// Only ReasonAlreadyPaid and ReasonSelfDelivery return above the supersede. Every
	// OTHER refusal returns below it, so a refused receipt pays 0 AND gives up the
	// eager 1-credit/byte self-mint RecordServeToObject took at serve time.
	//
	// WHY. The conserved leg is FLAT (fee − skim = 43,750 at the shipped fee) and the
	// self-mint is BYTE-PROPORTIONAL (0.875·B). B IS THE WHOLE ACCUMULATED LANE, not
	// one chunk (PE ruling §6, 2026-09-03): trackProvisional does `p.net += net` on an
	// existing lane, and the serve call site fires PER CHUNK (core/node/node.go), so B
	// is the total bytes this server has served for this (server, requester, root) —
	// the whole object. Above B = 50,000 bytes a refusal was therefore worth MORE than
	// being paid, so EVERY object above 50 KB was already past break-even; a 64 MiB
	// object is 1,342× past it. (The production chunk is 64 KiB —
	// pipeline.DefaultChunkSize — but the chunk size is not the relevant number, and
	// naming it understated the exposure.) The operator can trigger a refusal itself
	// by filling its own guard
	// with junk serials (Receipt.Object is attacker-chosen, so distinct roots are
	// free). Keeping the mint on a refusal was a profitable, operator-triggerable
	// supersede-disable on the whole of Boulder 0's conservation rule: RecordServe's
	// 1-credit/byte is an UNFUNDED SELF-MINT — the banned per-receipt subsidy — so a
	// witnessed receipt must REVERSE it. The root cause is the flat fee against a
	// byte-proportional mint (residual R-FLAT-FEE); re-pricing is a D-POD-KNOBS
	// change needing its own certification. This closes the lever, not the cause.
	//
	// WHY ReasonAlreadyPaid STAYS ABOVE IT. The lane key is (server, requester, root)
	// — a LANE, not a delivery — and a receipt names (serial, object, server), so it
	// cannot say which serve it acknowledges. The guard's own record is what stops a
	// re-presented receipt from reversing a RE-SERVED lane's fresh self-mint
	// (RT-DELIV-1/1b/2). Hoisting the supersede above the AlreadyPaid screen would
	// remove that bound. Below it, the reversal is bounded by the lane deletion: a
	// second presentation with no intervening serve finds no lane and moves nothing.
	// (Ungated branch, owed as R-REFUSED-RESERVE-REVERSAL: serve → refusal → serve
	// again → the same receipt reverses the SECOND lane's fresh mint, exactly once.)
	//
	// The reversal itself is PURELY SUBTRACTIVE (reverseProvisional only debits, and
	// floors the escrow claw-back at what the reserve still holds), so the worst case
	// on any refusal path is an UNDER-pay, never a mint.
	//
	// An UNWITNESSED or malformed receipt never reaches here at all: the bank rejects
	// it and core/node/demandrole.go never calls the ledger, so the self-mint STAYS
	// until a valid receipt arrives — the legitimate unwitnessed bilateral fallback,
	// unchanged (core/node TestG4_UnwitnessedReceiptLeavesTheSelfMintAlone).
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
			// SWEEP ON THE BAND ADVANCE, NOT ONLY AT THE CAP (crypto-specialist
			// advisory C-7, 2026-09-03). The cap-only trigger meant that on any node
			// below 65,536 live serials — which is every node most of the time —
			// expired guard entries were retained ON DISK indefinitely, far past the
			// W-epoch window that is their whole justification. Soundness was never
			// affected (refuse-not-evict is the correct choice and is unchanged), but
			// state whose justification is a 4-epoch window must not outlive it: this
			// is the data-minimisation answer, and it is the shape every
			// epoch-scoped online e-cash since Brands uses — an epoch-partitioned
			// spent list dropped at rollover rather than a database swept under load.
			// It is also CHEAPER than the cap-triggered scan: O(cap) per epoch instead
			// of O(cap) per refused redeem. The sweptEpoch latch makes it at most one
			// scan per epoch however many callers drive it.
			l.sweepIfEpochAdvanced(l.epochWatermark)
		}
		if _, paid := l.paidSerial[paidKey(issuedEpoch, serial)]; paid {
			return 0, ReasonAlreadyPaid // this TOKEN already funded one conserved payout — mint 0
		}
	}

	// Supersede: reverse this delivery's provisional self-credit, then forget the
	// lane. The reversal (escrow floored at what the reserve still holds — a bounty
	// paid out between serve and redeem is real durability work, not recoverable)
	// is shared verbatim with the eviction site. If the lane was already evicted,
	// its mint was reversed at eviction, so there is nothing here to reverse: the
	// redeem pays the conserved leg only (rule (b), one delivery one payment).
	//
	// This runs BEFORE every refusal below it — see the ordering invariant above.
	k := provKey{server: server, requester: fetcher, root: root}
	if p, ok := l.provisional[k]; ok {
		l.reverseProvisional(server, root, p)
		delete(l.provisional, k)
		l.removeFromProvOrder(k) // keep provOrder in sync (RT-DELIV-1/1b/2 fix)
	}

	// The remaining guard refusals. Each pays 0; none keeps the self-mint, because
	// the supersede above already reversed it.
	if len(serial) > 0 {
		if l.paidStore != nil && !l.guardLoaded {
			// A durable store is attached but not yet loaded, so this ledger does not
			// know what it already paid. Refuse rather than pay — this is exactly the
			// restart window the store exists to close (red-team re-break F2).
			return 0, ReasonGuardUnloaded
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

	// Conservation: pay the fee the fetcher already paid in, less the skim.
	fee := l.fee
	if fee <= 0 {
		return 0, ReasonNoFee
	}

	// RECORD THE GUARD ENTRY, DURABLY, BEFORE ANY CREDIT MOVES (red-team re-break F2).
	// The ordering is the whole property, and it is SignMarkStore's: a crash between
	// the two leaves a guard entry for a payout that never happened (an under-pay,
	// self-healing when the window advances), never a payout with no guard entry —
	// which a restart would let a second server collect all over again. A store that
	// cannot write refuses the payout for the same reason.
	//
	// Like every refusal below the supersede, this one leaves the lane having already
	// given up its unwitnessed self-credit. That direction is safe: the supersede is
	// purely subtractive (it reverses a self-mint), so the outcome is an under-pay —
	// never an over-pay, and never a mint.
	if len(serial) > 0 {
		if err := l.addPaidSerial(serial, server, issuedEpoch); err != nil {
			return 0, ReasonGuardStore
		}
	}

	// acct() REGISTERS an unknown account (and hands it the grant), so it is taken
	// here at the payment and not above: a refusal must not conjure an account.
	skim := fee * SkimNum / SkimDen
	l.acct(server).balance += fee - skim
	e := l.escrowFor(root)
	e.balance += skim
	e.funded += skim

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
	// itself. A store that is actually broken reports it the one way the redeem path
	// already refuses on: Append fails, and RedeemDeliveryCreditReason returns
	// ReasonGuardStore. Refusing on every Compact error instead was REFUSED by the
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
		out = append(out, ports.PaidSerial{Serial: serialOfPaidKey(k), Server: e.server, Epoch: e.epoch})
	}
	return out
}

// reservePaidSerial makes room for one more guarded serial, or reports that it
// cannot. It sweeps expired entries first and only then checks the cap; it NEVER
// evicts a live entry. false means "the cap is full of still-redeemable serials",
// which the caller turns into a refusal to pay (see RedeemDeliveryCredit).
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
func (l *Ledger) addPaidSerial(serial []byte, server ports.NodeID, issuedEpoch uint64) error {
	if len(serial) == 0 {
		return nil
	}
	key := paidKey(issuedEpoch, serial)
	if _, ok := l.paidSerial[key]; ok {
		return nil // already recorded (the guard above already refused a re-redeem)
	}
	if l.paidStore != nil {
		if err := l.paidStore.Append(ports.PaidSerial{Serial: serial, Server: server, Epoch: issuedEpoch}); err != nil {
			return err
		}
	}
	l.paidSerial[key] = paidSerialEntry{server: server, epoch: issuedEpoch}
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
	for _, e := range entries {
		if len(e.Serial) == 0 {
			continue
		}
		fresh[paidKey(e.Epoch, e.Serial)] = paidSerialEntry{server: e.Server, epoch: e.Epoch}
	}
	if len(fresh) > maxPaidSerial {
		return fmt.Errorf("credit: persisted paid-serial guard holds %d entries, cap is %d",
			len(fresh), maxPaidSerial)
	}
	l.paidSerial = fresh
	l.guardLoaded = true
	return nil
}
