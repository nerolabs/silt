package node

// #502 regression: a daemon restart between a repair's survivor fetches and
// its cleanup orphans the working set — record-less, proof-less leaf chunks
// that count against the pledge forever. Care's boot reconciliation must drop
// them, while keeping every legitimate holding: proof-backed leaves and the
// caretaker's bare warm-start manifest copies.
//
// The crash is injected for real (V5): world A drives an actual repair sweep
// against killed holders with simclock.Step() and STOPS mid-window — between
// fetch and drop — then world B boots a fresh Node incarnation on the same
// chunk+proof stores (the store state IS the crash artifact) and Cares the
// root. Two separate sim worlds, because a shared scheduler would let the
// "dead" node's pending callbacks fire and finish its own cleanup — a real
// crash fires nothing.

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

// buildOrphanWorld runs one sim world up to a mid-repair crash: publish,
// distribute (replication 1), kill holders of >RepairSlack columns, then step
// the caretaker's sweep until survivor pulls sit in its store with the sweep
// still unfinished. Returns the caretaker's stores (the crash artifact), the
// payload, and the orphaned leaf ids.
func buildOrphanWorld(t *testing.T) (store *memstore.Store, proofs *memproofs.Store, data []byte, orphans []ports.ChunkID) {
	t.Helper()
	const N = 16
	sched := simclock.New()
	net := simnet.New(sched, 21, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1

	var nodes []*Node
	store = memstore.New()
	proofs = memproofs.New()
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(4000 + i)).NodeID()
		st := memstore.New()
		if i == 1 {
			st = store // the caretaker: its store survives the "crash"
		}
		nd := New(id, cfg, sched, net.Endpoint(id), st)
		if i == 1 {
			nd.SetProofStore(proofs)
		}
		nodes = append(nodes, nd)
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

	data = make([]byte, 80<<10)
	for i := range data {
		data[i] = byte(i*17 + 9)
	}
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
	nodes[0].Distribute(entry, m, false, DerivePorKey(h.LayoutKey()), func(int, error) {})
	sched.Run()

	// Kill the SMALLEST holder carrying >RepairSlack columns: missing exceeds
	// the slack (repair fires) while survivors stay plentiful (reconstruction
	// proceeds), so the sweep's fetch phase pulls many survivor chunks — the
	// crash window. Killing the biggest holder instead leaves stripes below k:
	// repair fails fast and cleans up before a meaty window opens.
	leaves := m.Leaves()
	type held struct{ idx, count int }
	var hs []held
	for i, nd := range nodes {
		if i <= 1 {
			continue
		}
		c := 0
		for _, id := range leaves {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				c++
			}
		}
		if c > 0 {
			hs = append(hs, held{i, c})
		}
	}
	if len(hs) < 3 {
		t.Fatalf("rig: distribution too concentrated (%d holders)", len(hs))
	}
	best := -1
	for i := range hs {
		if hs[i].count >= 6 && (best < 0 || hs[i].count < hs[best].count) {
			best = i
		}
	}
	if best < 0 {
		t.Fatalf("rig: no holder carries ≥3 columns; counts=%v", hs)
	}
	net.Kill(nodes[hs[best].idx].ID())
	t.Logf("world A: killed 1 holder carrying %d/%d shards", hs[best].count, len(leaves))

	// Snapshot what the caretaker legitimately holds before the sweep, then
	// step the sweep and CRASH the world once survivor pulls are in the store.
	care := nodes[1]
	care.reg = reg
	heldBefore := map[ports.ChunkID]bool{}
	for _, id := range leaves {
		if ok, _ := care.Store().Has(bg(), id); ok {
			heldBefore[id] = true
		}
	}
	// The #517 confirmation gate defers a repair until the SECOND consecutive
	// over-slack sweep: run the first (observation) sweep to completion, then
	// crash inside the second — the one that actually fetches survivors.
	care.sweepEpoch++
	obs := false
	care.repairRoot(h.Care(), func() { obs = true })
	sched.Run()
	if !obs {
		t.Fatal("rig: observation sweep never completed")
	}
	care.sweepEpoch++
	finished := false
	care.repairRoot(h.Care(), func() { finished = true })
	for !finished {
		if !sched.Step() {
			break
		}
		orphans = orphans[:0]
		for _, id := range leaves {
			if heldBefore[id] {
				continue
			}
			if ok, _ := care.Store().Has(bg(), id); ok {
				orphans = append(orphans, id)
			}
		}
		if len(orphans) >= 3 {
			// Mid-window: pulls present, cleanup not run. Abandon the world.
			t.Logf("world A: crashed mid-repair with %d survivor pulls in the store", len(orphans))
			return store, proofs, data, orphans
		}
	}
	t.Fatalf("rig: sweep finished (finished=%v) before a crash window with ≥3 pulls opened", finished)
	return nil, nil, nil, nil
}

func TestCareBootReconcilesOrphanedWorkingSet(t *testing.T) {
	store, proofs, data, orphans := buildOrphanWorld(t)

	// World B: a fresh swarm, same content (convergent → same root and chunk
	// ids). The rebooted caretaker joins AFTER the publish (so it cannot be a
	// placement target) on the SAME chunk+proof stores — the crash artifact.
	const N = 16
	sched := simclock.New()
	net := simnet.New(sched, 22, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1

	var nodes []*Node
	for i := 0; i < N; i++ {
		if i == 1 {
			nodes = append(nodes, nil) // the caretaker boots after the publish
			continue
		}
		id := identity.FromSeed(int64(4000 + i)).NodeID()
		nodes = append(nodes, New(id, cfg, sched, net.Endpoint(id), memstore.New()))
	}
	for i, nd := range nodes {
		if i == 0 || nd == nil {
			continue
		}
		var seeds []ports.NodeID
		seeds = append(seeds, nodes[0].ID())
		nd.Bootstrap(seeds, func() {})
	}
	sched.Run()

	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: 4 << 10, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatalf("add B: %v", err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	m, err := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	nodes[0].Distribute(entry, m, false, DerivePorKey(h.LayoutKey()), func(int, error) {})
	sched.Run()

	// One orphan becomes a LEGITIMATE holding: register a proof for it in the
	// persisted backing. The reconciler must keep exactly this one — the
	// proof-backed-vs-proofless discriminator under test.
	legit := orphans[0]
	if err := proofs.Put(legit, ports.StorageProof{Root: h.Root}); err != nil {
		t.Fatalf("seed proof: %v", err)
	}

	// Boot the caretaker incarnation on the crash artifact and Care the root.
	careID := identity.FromSeed(int64(4001)).NodeID()
	care := New(careID, cfg, sched, net.Endpoint(careID), store)
	care.SetProofStore(proofs)
	lg := &captureLog{clock: sched}
	care.SetLogger(lg)
	care.Bootstrap([]ports.NodeID{nodes[0].ID()}, func() {})
	sched.Run()
	manifestHeldBefore := map[ports.ChunkID]bool{}
	for _, id := range entry.ManifestChunks {
		if ok, _ := care.Store().Has(bg(), id); ok {
			manifestHeldBefore[id] = true
		}
	}
	// Care() starts the perpetual repair-tick loop, so sched.Run() would never
	// reach quiescence (the redteam rig's careJudge comment) — drive bounded
	// Steps until the reconcile narrates, then assert on the store state.
	care.Care(reg, h.Care())
	for i := 0; i < 200_000; i++ {
		if !sched.Step() {
			break
		}
		if lg.last("repair working set reconciled") != nil {
			break
		}
	}

	// The orphans are gone — except the proof-backed one.
	for _, id := range orphans {
		ok, _ := care.Store().Has(bg(), id)
		if id == legit {
			if !ok {
				t.Fatalf("#502: reconciliation dropped a PROOF-BACKED leaf %s — the discriminator is broken", id)
			}
			continue
		}
		if ok {
			t.Fatalf("#502: orphaned working-set chunk %s survived the boot reconciliation", id)
		}
	}
	// The warm-start manifest redundancy is untouched.
	for id := range manifestHeldBefore {
		if ok, _ := care.Store().Has(bg(), id); !ok {
			t.Fatalf("#502: reconciliation dropped a warm-start manifest chunk %s", id)
		}
	}
	// And the cleanup narrated itself for the journal.
	rec := lg.last("repair working set reconciled")
	if rec == nil {
		t.Fatal("#502: no 'repair working set reconciled' narration")
	}
	if d, _ := rec["dropped"].(int); d != len(orphans)-1 {
		t.Fatalf("#502: narration reports dropped=%v, want %d", rec["dropped"], len(orphans)-1)
	}
}
