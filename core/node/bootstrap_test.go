package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// #281: silt's Kademlia join is ONE-SHOT. A node that starts before its bootstrap
// target is listening lands with an EMPTY routing table and, without a retry,
// stays isolated forever — no peers, no consensus, no discovery — until a manual
// restart (found on the 13-node cross-region cloud cold start). StartBootstrapRetry
// re-runs the join while the table is empty, so the node recovers on its own once
// the target comes up.
func TestBootstrapRetryRecoversIsolatedNode(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	cfg := DefaultConfig()

	aID := identity.FromSeed(2810).NodeID()
	bID := identity.FromSeed(2811).NodeID()

	// A's endpoint exists but is DOWN during B's join — the cold-start race: the
	// bootstrap target's listener isn't up yet, so B's FIND_NODE is dropped.
	a := New(aID, cfg, sched, net.Endpoint(aID), memstore.New())
	_ = a
	net.Kill(aID)

	b := New(bID, cfg, sched, net.Endpoint(bID), memstore.New())
	b.StartBootstrapRetry(nil)
	b.Bootstrap([]ports.NodeID{aID}, func() {})

	// Let the initial (failed) join settle — the FIND_NODE to the dead A times out.
	sched.RunUntil(sched.Now().Add(cfg.RequestTimeout).Add(ports.Second))
	if b.Table().Size() != 0 {
		t.Fatalf("precondition: B should be isolated while A is down, got table size %d", b.Table().Size())
	}

	// A's listener comes up. Without the retry loop B would stay stranded here.
	net.Restart(aID)

	// Advance past a few retry intervals; the empty-table retry re-dials A.
	sched.RunUntil(sched.Now().Add(cfg.BootstrapRetryInterval * 4))
	if b.Table().Size() == 0 {
		t.Fatal("#281: re-bootstrap did not recover — B still isolated after A came up")
	}
}

// A SPARSE cold-start mesh must converge, not just become non-empty. The seedless
// boot validator is the worst case: it gets ONE incoming dial and never re-looks-up,
// so it can't discover the rest of the validator set and consensus quorum never
// forms (the deeper gap behind a stalled simultaneous cold start; #281 follow-up).
// The sparse-refresh keeps doing a self-lookup while the table is under-populated.
func TestBootstrapRetryConvergesSparseTable(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	cfg := DefaultConfig()

	// A hub that several peers join, so it learns them (a node Observes the senders
	// of incoming requests).
	hub := identity.FromSeed(2820).NodeID()
	h := New(hub, cfg, sched, net.Endpoint(hub), memstore.New())
	_ = h
	for i := 0; i < 5; i++ {
		id := identity.FromSeed(int64(2821 + i)).NodeID()
		p := New(id, cfg, sched, net.Endpoint(id), memstore.New())
		p.Bootstrap([]ports.NodeID{hub}, func() {})
	}
	sched.RunUntil(sched.Now().Add(cfg.BootstrapRetryInterval))

	// A boot-like node: exactly ONE table entry (the hub), NO seeds — the seedless
	// boot validator that got one incoming dial and would otherwise stall there.
	bootID := identity.FromSeed(2830).NodeID()
	b := New(bootID, cfg, sched, net.Endpoint(bootID), memstore.New())
	b.table.Observe(hub)
	b.StartBootstrapRetry(nil)
	if b.Table().Size() != 1 {
		t.Fatalf("precondition: boot-like node should have exactly 1 entry, got %d", b.Table().Size())
	}

	// The sparse-refresh should query the hub, learn its peers, and converge.
	sched.RunUntil(sched.Now().Add(cfg.BootstrapRetryInterval * 4))
	if b.Table().Size() < 3 {
		t.Fatalf("sparse boot node did not converge via bucket refresh: table=%d (want >= 3)", b.Table().Size())
	}
}

// The retry is a no-op once the node is healthy: a node that joined successfully
// must not have its table disturbed by the retry tick (it only acts on Size()==0).
func TestBootstrapRetryNoopWhenHealthy(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	cfg := DefaultConfig()

	aID := identity.FromSeed(2812).NodeID()
	bID := identity.FromSeed(2813).NodeID()

	a := New(aID, cfg, sched, net.Endpoint(aID), memstore.New())
	_ = a
	b := New(bID, cfg, sched, net.Endpoint(bID), memstore.New())
	b.StartBootstrapRetry(nil)
	b.Bootstrap([]ports.NodeID{aID}, func() {})

	sched.RunUntil(sched.Now().Add(cfg.BootstrapRetryInterval * 3))
	if b.Table().Size() == 0 {
		t.Fatal("healthy join should have populated the table")
	}
}

// BootstrapRetryInterval <= 0 disables the loop entirely (StartBootstrapRetry is a
// no-op), so nothing is scheduled — a stranded node stays stranded (opt-out).
func TestBootstrapRetryDisabled(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	cfg := DefaultConfig()
	cfg.BootstrapRetryInterval = 0

	aID := identity.FromSeed(2814).NodeID()
	bID := identity.FromSeed(2815).NodeID()

	a := New(aID, cfg, sched, net.Endpoint(aID), memstore.New())
	_ = a
	net.Kill(aID)

	b := New(bID, cfg, sched, net.Endpoint(bID), memstore.New())
	b.StartBootstrapRetry(nil) // no-op: interval disabled
	b.Bootstrap([]ports.NodeID{aID}, func() {})
	sched.RunUntil(sched.Now().Add(cfg.RequestTimeout).Add(ports.Second))

	net.Restart(aID)
	sched.RunUntil(sched.Now().Add(DefaultConfig().BootstrapRetryInterval * 4))
	if b.Table().Size() != 0 {
		t.Fatalf("with the retry disabled, the isolated node must stay isolated, got table size %d", b.Table().Size())
	}
}
