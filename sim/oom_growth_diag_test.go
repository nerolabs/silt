package sim

import (
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// TestConsensusMemoryGrowth is a DIAGNOSTIC (opt-in: SILT_OOM_DIAG=1), not a
// pass/fail assertion — it attributes the MATURING consensus-node memory
// footprint that OOM-crash-loops the field cohort (the fix #464 did NOT resolve;
// the crash-looping nodes hold ~no chunks, so the proof map was never their hog —
// silt-oom-NOT-the-proof-map-FINDING-2026-08-17). It drives a mature-epoch
// network for many heights in-process and logs resident-heap + per-node state
// growth per height. Run with a heap profile to attribute the dominant alloc:
//
//	SILT_OOM_DIAG=1 SILT_OOM_HEIGHTS=300 go test ./sim/ -run TestConsensusMemoryGrowth -memprofile /tmp/heap.out -v
//	go tool pprof -inuse_space -top /tmp/heap.out
//
// If HeapInuse grows ~linearly with height, a per-height/per-block structure
// (chain block history? per-round attestations? accumulating bond regs?) is the
// leak; the profile names it. If it plateaus, the in-process core is not the
// hog and the growth is in the daemon's transport/registry adapters (escalate to
// the -debug-addr daemon repro).
func TestConsensusMemoryGrowth(t *testing.T) {
	if os.Getenv("SILT_OOM_DIAG") != "1" {
		t.Skip("diagnostic; set SILT_OOM_DIAG=1 to run (attributes the MATURING OOM)")
	}
	heights := 300
	if v := os.Getenv("SILT_OOM_HEIGHTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			heights = n
		}
	}

	const (
		seed       = int64(11)
		honestN    = 4
		sybilN     = 9
		honestBond = int64(64) << 20
		minBond    = int64(1) << 20
		sybilDom   = uint64(0x5ceb11)
	)
	cfg := chain.Config{Quorum: 2, MinBond: minBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 4}
	verify := func(_ []byte, _ ports.Hash, _ int64, _ uint64, answer []byte) bool {
		return string(answer) == "valid"
	}

	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())

	total := honestN + sybilN
	idents := make([]*identity.Identity, total)
	ids := make([]ports.NodeID, total)
	for i := range idents {
		idents[i] = identity.FromSeed(seed*1000 + int64(i))
		ids[i] = idents[i].NodeID()
	}
	honest := ids[:honestN]

	g := &chain.Block{Version: 1, Height: 0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("genesis")), ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("gm"))}}}}
	for i := 0; i < honestN; i++ {
		g.BondRegs = append(g.BondRegs, chain.NewBondReg(idents[i].Signer(),
			ports.HashBytes(ids[i][:]), honestBond, []byte("valid"), ports.Hash{}, uint64(i+1)))
	}
	for i := honestN; i < total; i++ {
		g.BondRegs = append(g.BondRegs, chain.NewBondReg(idents[i].Signer(),
			ports.HashBytes(ids[i][:]), minBond, []byte("valid"), ports.Hash{}, sybilDom))
	}
	chain.Sign(g, idents[0].Signer())

	nodes := make([]*node.Node, total)
	for i := range nodes {
		nd := node.New(ids[i], node.DefaultConfig(), sched, net.Endpoint(ids[i]), memstore.New())
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(verify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("node %d: genesis: %v", i, err)
		}
		nd.EnableChain(ch, idents[i].Signer())
		nodes[i] = nd
	}
	for i := 1; i < total; i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
	}
	sched.Run()

	var m runtime.MemStats
	sample := func(h int) {
		runtime.GC()
		runtime.ReadMemStats(&m)
		_, tip := nodes[0].Chain().Head()
		t.Logf("h=%-4d HeapInuse=%6.1f MiB HeapObjects=%-9d chainLen=%d tip=%d",
			h, float64(m.HeapInuse)/(1<<20), m.HeapObjects, nodes[0].Chain().Len(), tip)
	}
	sample(0)

	// Drive `heights` real commits through the wire gather. nodes[0] proposes
	// each; the other honest validators attest. Height climbs one per iteration.
	for h := 1; h <= heights; h++ {
		e := ports.Entry{
			Root:           ports.HashBytes([]byte("blk-" + strconv.Itoa(h))),
			ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("m-" + strconv.Itoa(h)))},
		}
		committed := false
		var cerr error
		nodes[0].ProposeEntry(e, honest[1:], ids, cfg.Quorum, func(err error) { cerr, committed = err, true })
		sched.Run()
		if !committed || cerr != nil {
			t.Fatalf("h=%d did not commit: done=%v err=%v", h, committed, cerr)
		}
		if h%25 == 0 {
			sample(h)
		}
	}
	sample(heights)

	// Capture a LIVE heap profile at peak (state still referenced by `nodes`),
	// unlike -memprofile which samples at process exit after teardown.
	runtime.KeepAlive(nodes)
	if path := os.Getenv("SILT_OOM_HEAP"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Logf("live heap profile at h=%d → %s", heights, path)
	}
}
