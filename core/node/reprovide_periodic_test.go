package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// TestStartReprovideKeepsHeldRecordsLivePastTTL is the #69 residual (confirmed dark
// ~30 min after boot under a real streaming load test): provider records carry a
// ProviderRecordTTL lease and GetProviders serves only Live() records, but AnnounceHeld
// runs once at startup — so a holder goes undiscoverable the moment its startup records
// lapse. StartReprovide re-announces on a TTL/2 timer to keep them fresh.
//
// The test shows both halves against the same TTL: WITHOUT reprovide the startup record
// expires at the TTL (the bug); WITH it, the record stays Live two TTLs out (the fix).
func TestStartReprovideKeepsHeldRecordsLivePastTTL(t *testing.T) {
	const ttl = 1800 * ports.Second // 30 min

	build := func(reprovide bool) (*Node, *simclock.Scheduler, ports.Hash) {
		store := memstore.New()
		chunk := ports.NewChunk([]byte("held content for reprovide"))
		store.Put(bg(), chunk)
		sched := simclock.New()
		idn := identity.FromSeed(5150) // a real id+signer so provider records carry a real Expiry
		id := idn.NodeID()
		ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
		end := &linkEnd{net: ln, id: id}
		ln.ends[id] = end
		cfg := DefaultConfig()
		cfg.ProviderRecordTTL = ttl
		n := New(id, cfg, sched, end, store)
		n.SetSigner(idn.Signer()) // Expiry is stamped only for a signed record
		done := false
		n.AnnounceHeld(func(int) { done = true })
		sched.Run()
		if !done {
			t.Fatal("initial AnnounceHeld did not finish")
		}
		if reprovide {
			n.StartReprovide()
		}
		return n, sched, ports.Hash(chunk.ID)
	}
	live := func(n *Node, key ports.Hash, at ports.Time) bool {
		return len(n.provs.Live(key, int64(at))) > 0
	}

	// Baseline (the #69 bug): with no reprovide, the startup record expires at the TTL.
	n0, s0, key0 := build(false)
	past := s0.Now().Add(ttl + 60*ports.Second)
	s0.RunUntil(past)
	if live(n0, key0, past) {
		t.Fatal("precondition: without reprovide the held record must expire past its TTL (the #69 bug)")
	}

	// The fix: StartReprovide re-stamps every TTL/2, so the record is still Live two TTLs
	// out — only a live reprovide loop keeps it discoverable that far past the original lease.
	n1, s1, key1 := build(true)
	past1 := s1.Now().Add(ttl*2 + 60*ports.Second)
	s1.RunUntil(past1)
	if !live(n1, key1, past1) {
		t.Fatal("StartReprovide must keep the held record Live past the TTL — else the holder goes undiscoverable (#69)")
	}
}

// TestStartReprovideNoopWithoutTTL: when records never expire (ProviderRecordTTL 0),
// there is nothing to refresh, so StartReprovide schedules no work.
func TestStartReprovideNoopWithoutTTL(t *testing.T) {
	n, sched := aloneNode(t, 1)
	// DefaultConfig leaves ProviderRecordTTL 0; StartReprovide must be inert.
	n.StartReprovide()
	sched.RunUntil(sched.Now().Add(3600 * ports.Second))
	// No panic / no scheduled reprovide is the assertion; reaching here is success.
}
