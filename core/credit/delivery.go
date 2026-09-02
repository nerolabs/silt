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
func (l *Ledger) RedeemDeliveryCredit(server, fetcher ports.NodeID, root ports.Hash) int64 {
	if server == fetcher {
		return 0 // self-delivery earns nothing (the cheapest gaming, blocked)
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
		return 0
	}
	skim := fee * SkimNum / SkimDen
	s.balance += fee - skim
	e := l.escrowFor(root)
	e.balance += skim
	e.funded += skim
	return fee - skim
}
