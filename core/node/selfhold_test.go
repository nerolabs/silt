package node

// (a-domain-fresh) repair self-hold — the new hosting primitive (PE ruling
// 2026-08-19, RULING-repair-payee-fork). When the economy is on and the
// paramedic's own failure domain is unused by a stripe, it KEEPS the shard it
// rebuilt (funding the node that bore the reconstruction cost/RAM) instead of
// pushing it to a stranger — but only when self-holding cannot reduce
// failure-domain diversity. hostShardLocally is the store+provider+proof path
// that makes the paramedic a real, audit-answerable holder.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

func selfHoldNode(t *testing.T, seed int64) *Node {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	id := identity.FromSeed(seed)
	return New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
}

// TestHostShardLocally: a well-formed shard becomes genuinely held — stored, and
// registered as a provider so the swarm (and an audit) can find it here.
func TestHostShardLocally(t *testing.T) {
	nd := selfHoldNode(t, 9900)
	data := []byte("a rebuilt shard's bytes")
	id := ports.HashBytes(data)

	if !nd.hostShardLocally(id, data, nil) {
		t.Fatal("hostShardLocally refused a well-formed shard")
	}
	if ok, _ := nd.store.Has(bg(), id); !ok {
		t.Fatal("shard was not stored")
	}
	// The node must now advertise itself as a provider of the shard's key, or a
	// reader/auditor could never find the copy it holds.
	if recs := nd.provs.Live(ports.Hash(id), int64(nd.clock.Now())); len(recs) == 0 {
		t.Fatal("no provider record planted — the self-held shard is invisible")
	}
}

// TestHostShardLocallyRefusesJunk: never host bytes that don't hash to the id
// (B3 — a later audit could not defend them). Nothing is stored.
func TestHostShardLocallyRefusesJunk(t *testing.T) {
	nd := selfHoldNode(t, 9901)
	id := ports.HashBytes([]byte("the committed content"))
	junk := []byte("different bytes entirely")

	if nd.hostShardLocally(id, junk, nil) {
		t.Fatal("hostShardLocally accepted bytes that don't hash to the id")
	}
	if ok, _ := nd.store.Has(bg(), id); ok {
		t.Fatal("junk was stored despite the hash mismatch")
	}
}

// TestSelfHoldEligible is the S2-safety invariant of (a-domain-fresh): a paramedic
// self-holds a rebuilt shard ONLY when the economy is on AND its own failure domain
// is not already holding a shard of the stripe — so keeping the shard can never
// reduce failure-domain diversity. This is the gate that makes fork (a-domain-fresh)
// dispersal-safe (vs unconstrained (a), which the PE rejected for trading S2 away).
func TestSelfHoldEligible(t *testing.T) {
	const selfDomain = 7
	fresh := map[uint64]int{3: 1, 5: 2}    // stripe uses domains 3 and 5, NOT 7
	occupied := map[uint64]int{3: 1, 7: 1} // stripe already uses domain 7

	cases := []struct {
		name     string
		economy  bool
		domain   uint64
		used     map[uint64]int
		eligible bool
	}{
		{"on-fresh-domain", true, selfDomain, fresh, true},
		{"on-occupied-domain", true, selfDomain, occupied, false}, // S2: would reduce diversity
		{"off-even-if-fresh", false, selfDomain, fresh, false},    // opt-in: economy off never self-holds
		{"on-unset-domain", true, 0, fresh, false},                // domain 0 can't prove freshness
	}
	for _, c := range cases {
		nd := selfHoldNode(t, 9910)
		nd.cfg.RepairEconomy = c.economy
		nd.domainID = c.domain
		if got := nd.selfHoldEligible(c.used); got != c.eligible {
			t.Fatalf("%s: selfHoldEligible = %v, want %v", c.name, got, c.eligible)
		}
	}
}
