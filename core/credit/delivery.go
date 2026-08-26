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
// oldest entry is forgotten (deterministic FIFO — no map iteration, B2) and that
// serve stays permanently on the self-record lane. The residual — a receipt
// redeemed after its provisional record was evicted double-pays that one
// delivery by its bytes-net — is bounded by the cap and is strictly smaller
// than the pre-existing self-mint exposure the certification holds separately
// ("the deeper flag": witnessing all cross-operator serves would subsume the
// self-mint; not this slice).
//
// NEVER STANDING: everything here moves the balance economy only. No field this
// file touches is read by Reputation — asserted structurally by the Invariant-A
// guard (invariant_a_test.go) and the delivery firewall test.

import "github.com/nerolabs/silt/ports"

// maxProvisional caps the supersede-tracking map (see the bounding note above).
const maxProvisional = 8192

// provKey identifies one delivery lane: this ledger's owner served object root
// to requester. The receipt's Fetcher key hashes to the requester NodeID
// (NodeID = sha256(pubkey)), which is what lets a redeem find the serve.
type provKey struct {
	requester ports.NodeID
	root      ports.Hash
}

// provisionalServe is the self-credit a serve recorded before any receipt: the
// net balance the server credited itself and the skim it routed to the object's
// escrow. A redeem reverses both, then pays the conserved credit instead.
type provisionalServe struct {
	net  int64
	skim int64
}

// trackProvisional records an object-aware serve's self-credit for later
// supersession. Called by RecordServeToObject only.
func (l *Ledger) trackProvisional(requester ports.NodeID, root ports.Hash, net, skim int64) {
	k := provKey{requester: requester, root: root}
	p, ok := l.provisional[k]
	if !ok {
		for len(l.provisional) >= maxProvisional && len(l.provOrder) > 0 {
			old := l.provOrder[0]
			l.provOrder = l.provOrder[1:]
			delete(l.provisional, old) // oldest lane falls back to self-record forever
		}
		p = &provisionalServe{}
		l.provisional[k] = p
		l.provOrder = append(l.provOrder, k)
	}
	p.net += net
	p.skim += skim
}

// RedeemDeliveryCredit settles a WITNESSED delivery (a banked, verified
// receipt): it supersedes the provisional self-record for (fetcher, root), then
// pays the server the conserved credit — the withdrawal fee less the durability
// skim, with the skim routed to the object's escrow (escrow-routing is the
// certified-safe default; burn is the owner's airtight alternative — see the
// owner-knobs consult). Returns the credits paid to the server. Self-delivery
// pays nothing, same guard as RecordServe. It never touches standing.
func (l *Ledger) RedeemDeliveryCredit(server, fetcher ports.NodeID, root ports.Hash) int64 {
	if server == fetcher {
		return 0 // self-delivery earns nothing (the cheapest gaming, blocked)
	}
	s := l.acct(server)

	// Supersede: reverse this delivery's provisional self-credit. The escrow
	// reversal is floored at what the reserve still holds — a bounty paid out
	// between serve and redeem is real durability work, not recoverable.
	k := provKey{requester: fetcher, root: root}
	if p, ok := l.provisional[k]; ok {
		s.balance -= p.net
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
		delete(l.provisional, k)
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
