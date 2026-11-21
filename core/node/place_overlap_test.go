package node

// A publish waits on placement before it can register, so placement's wall
// time is publish latency. These tests pin the two overlaps that keep it to a
// few round trips: independent placement groups run together, and a chunk's
// copies are asked for at once.

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

// fixedHop is the one-way delay of every message, so a round trip costs
// exactly 2*fixedHop and elapsed time counts round trips.
const fixedHop = 50 * ports.Millisecond

func newFixedHopSwarm(t *testing.T, n int, cfg Config) (*simclock.Scheduler, []*Node) {
	return newFixedHopSwarmIn(t, n, cfg, nil)
}

// newFixedHopSwarmIn puts node i in failure domain domainOf(i) when domainOf is
// non-nil.
func newFixedHopSwarmIn(t *testing.T, n int, cfg Config, domainOf func(i int) string) (*simclock.Scheduler, []*Node) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 11, simnet.Config{LatencyMin: fixedHop, LatencyMax: fixedHop})
	var nodes []*Node
	for i := 0; i < n; i++ {
		id := identity.FromSeed(int64(5100 + i)).NodeID()
		c := cfg
		if domainOf != nil {
			c.Domain = domainOf(i)
		}
		nodes = append(nodes, New(id, c, sched, net.Endpoint(id), memstore.New()))
	}
	for i, nd := range nodes {
		if i == 0 {
			continue
		}
		var seeds []ports.NodeID
		for j := 0; j < i && j < 3; j++ {
			seeds = append(seeds, nodes[j].ID())
		}
		nd.Bootstrap(seeds, func() {})
	}
	sched.Run()
	return sched, nodes
}

// An 8 KiB coded object is one data shard plus six parity shards, each its own
// column, plus a manifest chunk: eight placement groups, each a lookup and
// three copies. One after another that is at least 8 × (1 + 3) = 32 round
// trips; overlapped it is about one lookup and one send.
func TestDistributeOverlapsPlacementGroups(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Replication = 3
	sched, nodes := newFixedHopSwarm(t, 16, cfg)

	data := make([]byte, 8<<10)
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	reg := registry.New()
	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: 64 << 10, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	m, err := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ids := append(append([]ports.ChunkID(nil), entry.ManifestChunks...), m.Leaves()...)
	if len(ids) != 8 {
		t.Fatalf("object has %d chunks, the arithmetic below assumes 8", len(ids))
	}

	start := sched.Now()
	var end ports.Time
	var distErr error
	finished := false
	nodes[0].Distribute(entry, m, false, func(_ int, err error) {
		end, distErr, finished = sched.Now(), err, true
	})
	sched.Run()
	if !finished || distErr != nil {
		t.Fatalf("distribute finished=%v err=%v", finished, distErr)
	}

	// Shards with identical bytes share an id, and each of their columns may
	// place it, so a chunk needs at least Replication holders, not exactly.
	for _, id := range ids {
		holders := 0
		for _, nd := range nodes[1:] {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				holders++
			}
		}
		if holders < cfg.Replication {
			t.Errorf("chunk %s has %d holders, want at least %d", id, holders, cfg.Replication)
		}
	}

	rtt := 2 * fixedHop
	took := ports.Duration(end - start)
	if took > 8*rtt {
		t.Fatalf("placing 8 groups took %d round trips; overlapped it should take at most 8 (serially at least 32)",
			took/rtt)
	}
	t.Logf("placed 8 groups × %d copies in %.1f round trips", cfg.Replication, float64(took)/float64(rtt))
}

// Columns placed together must still spread across failure domains: one
// stripe's columns over eight domains, one copy each, put no more than an even
// share of its shards in any one domain, as placing them one after another does. Steering on
// landed holders alone would let every column in flight see the same empty
// tally and pile into whichever domains sit closest to their keys.
func TestColumnsInFlightSpreadAcrossDomains(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Replication = 1
	// A lookup returns K nodes, and a column can only prefer among the domains
	// those cover. With K above the swarm size every domain is a candidate for
	// every column, so an even spread is reachable and anything worse is the
	// steering's doing.
	cfg.K = 32
	const domains = 8
	sched, nodes := newFixedHopSwarmIn(t, 24, cfg, func(i int) string { return fmt.Sprintf("dom%d", i%domains) })

	data := make([]byte, 32<<10)
	for i := range data {
		data[i] = byte(i*13 + 1)
	}
	reg := registry.New()
	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: 4 << 10, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	m, err := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	leaves := m.Leaves()
	if len(leaves) <= domains || len(leaves) > erasure.DefaultParams.N {
		t.Fatalf("object has %d shards, the bound below assumes one stripe wider than %d domains", len(leaves), domains)
	}
	share := (len(leaves) + domains - 1) / domains
	var distErr error
	nodes[0].Distribute(entry, m, false, func(_ int, err error) { distErr = err })
	sched.Run()
	if distErr != nil {
		t.Fatalf("distribute: %v", distErr)
	}

	perDomain := map[int]int{}
	for _, id := range leaves {
		for i, nd := range nodes[1:] {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				perDomain[(i+1)%domains]++
			}
		}
	}
	worst := 0
	for _, c := range perDomain {
		worst = max(worst, c)
	}
	if worst > share {
		t.Fatalf("a domain holds %d of the stripe's %d shards, want at most %d (spread %v)", worst, len(leaves), share, perDomain)
	}
}

// The first refusal hands its slot to the next candidate: the copies land on
// the same holders a one-at-a-time walk would reach, never on more than want,
// and in two round trips rather than four.
func TestPlaceAtAsksForCopiesAtOnce(t *testing.T) {
	cfg := DefaultConfig()
	sched, nodes := newFixedHopSwarm(t, 6, cfg)
	nodes[1].SetFreeload(true) // refuses every store

	c := ports.NewChunk([]byte("one chunk, three copies"))
	cands := []ports.NodeID{nodes[1].ID(), nodes[0].ID(), nodes[2].ID(), nodes[3].ID(), nodes[4].ID(), nodes[5].ID()}

	start := sched.Now()
	var end ports.Time
	got := -1
	var acceptedBy []ports.NodeID
	nodes[0].placeAt(c.ID, c.Data, nil, cands, 3,
		func(id ports.NodeID) { acceptedBy = append(acceptedBy, id) },
		func(placed int) { end, got = sched.Now(), placed })
	sched.Run()

	if got != 3 || len(acceptedBy) != 3 {
		t.Fatalf("placed %d (accepted %d), want 3", got, len(acceptedBy))
	}
	for i, nd := range nodes {
		held, _ := nd.Store().Has(bg(), c.ID)
		want := i == 2 || i == 3 || i == 4
		if held != want {
			t.Errorf("node %d holds=%v, want %v", i, held, want)
		}
	}
	if rtt := 2 * fixedHop; ports.Duration(end-start) != 2*rtt {
		t.Fatalf("placement took %.1f round trips, want 2 (the refusal, then its replacement)",
			float64(end-start)/float64(rtt))
	}
}
