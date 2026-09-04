package dht

import "github.com/nerolabs/silt/ports"

// Table is the k-bucket routing table. Bucket i holds peers whose
// highest differing bit from self is i — i.e. peers at distance
// [2^i, 2^(i+1)). Close buckets are tiny neighborhoods the node knows
// exhaustively; far buckets each cover half the network in k samples.
// That asymmetry is the whole O(log N) trick.
type Table struct {
	self    ports.NodeID
	k       int
	buckets [256][]ports.NodeID // each ordered oldest → newest
	// Failure-domain diversity (M0 H5-B / Memo 08): when domainOf is set and
	// perDomainCap > 0, a bucket admits at most perDomainCap peers sharing one
	// domain, so an adversary minting many NodeIDs in ONE domain (a cheap key-
	// surround from a single /24) can't fill the buckets near a key and crowd out
	// the honest, domain-diverse peers a lookup needs. Domain 0 (unknown) is never
	// capped — an unknown peer is assumed independent (like preferFreshDomain).
	//
	// KNOWN HOLE, owned (R4.3a → R4.3b, 2026-09-03): the domain is a free self-declaration
	// off the wire, so "unknown ⇒ never capped" is an exemption an adversary gets by
	// omitting -domain, and "N random labels ⇒ N domains" defeats the cap for free
	// either way. Capping the unknown pool was BUILT and then REFUTED by the red-team:
	// in the default (domainless) swarm it caps every honest peer too, so two early
	// undeclared Sybils lock a K=8 bucket (exclusion cost 8 → 2 identities). No
	// declared-label design prices an eclipse. The close is R4.3b: key this cap on the
	// OBSERVED contacted-at address, as geth and Bitcoin Core do (ROADMAP R4.3b;
	// red-team RED-TEAM-R4.3b-dht-eclipse-keying-2026-09-03.md). Gates:
	// core/node/r43a_dht_domain0_test.go.
	domainOf     func(ports.NodeID) uint64
	perDomainCap int
	// R4.3b: the observed-address cap (address.go). meta is the class stored WITH
	// each tabled entry (cert C-4): captured at admission, deleted on Remove.
	addr addressRule
	meta map[ports.NodeID]entryMeta
}

func NewTable(self ports.NodeID, k int) *Table {
	return &Table{self: self, k: k, meta: make(map[ports.NodeID]entryMeta)}
}

func (t *Table) Self() ports.NodeID { return t.self }

// SetDiversity turns on the per-bucket domain cap (H5-B). domainOf resolves a
// peer's failure domain (0 = unknown); perDomainCap ≤ 0 disables the cap.
func (t *Table) SetDiversity(domainOf func(ports.NodeID) uint64, perDomainCap int) {
	t.domainOf = domainOf
	t.perDomainCap = perDomainCap
}

// domainSaturated reports whether bucket b already holds perDomainCap peers in
// id's (known) domain — in which case a NEW peer from that domain is not admitted.
// Domain 0 (unknown) is exempt: a KNOWN HOLE (see the Table doc), kept deliberately
// because capping the unknown pool locks buckets in the default domainless swarm;
// closed by R4.3b's observed-address keying, never by a declared label.
func (t *Table) domainSaturated(b []ports.NodeID, id ports.NodeID) bool {
	if t.domainOf == nil || t.perDomainCap <= 0 {
		return false
	}
	d := t.domainOf(id)
	if d == 0 {
		return false // unknown domain: assumed independent, never capped (R4.3b closes this)
	}
	cnt := 0
	for _, e := range b {
		if t.domainOf(e) == d {
			cnt++
		}
	}
	return cnt >= t.perDomainCap
}

// Observe records that a peer was seen alive (a completed conversation, or a
// seed). Known peers move to the back (newest). New peers join if the bucket has
// room and the address rule admits them (address.go); if the bucket is full the
// newcomer is dropped — Kademlia's insight is that the oldest live peer is
// statistically the most reliable, so incumbents win. (The textbook refinement —
// ping the oldest and evict only if it's dead — can slot in here when repair
// lands in M4.) Reply-learned ids go through ObserveIntroduced; configured seeds
// through ObserveStatic.
func (t *Table) Observe(id ports.NodeID) { t.observe(id, nil, false) }

// Remove drops a peer (observed dead, e.g. a request timed out).
func (t *Table) Remove(id ports.NodeID) {
	i := BucketIndex(t.self, id)
	if i < 0 {
		return
	}
	b := t.buckets[i]
	for j, existing := range b {
		if existing == id {
			t.buckets[i] = append(b[:j:j], b[j+1:]...)
			delete(t.meta, id) // the class lives with the entry (C-4)
			return
		}
	}
}

// Closest returns up to n known peers nearest to target.
func (t *Table) Closest(target ports.Hash, n int) []ports.NodeID {
	var all []ports.NodeID
	for _, b := range t.buckets {
		all = append(all, b...)
	}
	SortByDistance(target, all)
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// Size returns the number of known peers.
func (t *Table) Size() int {
	n := 0
	for _, b := range t.buckets {
		n += len(b)
	}
	return n
}
