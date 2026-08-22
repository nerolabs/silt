package node

// #517 regressions: the repair trigger is minimum-filtered (network-durability
// §3) — one over-slack probe observation never fires a repair (a just-armed
// caretaker's first sweep races record propagation and reads live shards as
// missing; the captured #514 run "repaired" 3 never-lost shards and placed the
// rebuilds at replication N). Two consecutive observations fire; a clean sweep
// in between resets the count.

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

type confirmRig struct {
	sched  *simclock.Scheduler
	net    *simnet.Network
	nodes  []*Node
	reg    ports.Registry
	m      *manifest.Manifest
	h      link.Handle
	care   *Node
	log    *captureLog
	victim *Node // a holder carrying > RepairSlack columns
}

func newConfirmRig(t *testing.T) *confirmRig {
	t.Helper()
	const N = 16
	sched := simclock.New()
	net := simnet.New(sched, 31, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1

	var nodes []*Node
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(5000 + i)).NodeID()
		nodes = append(nodes, New(id, cfg, sched, net.Endpoint(id), memstore.New()))
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

	data := make([]byte, 80<<10)
	for i := range data {
		data[i] = byte(i*23 + 7)
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

	care := nodes[1]
	care.reg = reg
	lg := &captureLog{clock: sched}
	care.SetLogger(lg)

	// The victim: the smallest holder carrying > RepairSlack distinct COLUMNS.
	// Each column contributes one shard per stripe, so killing a 3-column
	// holder makes EVERY stripe miss 3 > slack 2 — the repair must fire —
	// while reconstruction stays plentiful (13 of 16 columns survive).
	leaves := m.Leaves()
	needColumns := cfg.RepairSlack + 1
	var victim *Node
	victimCols := 1 << 30
	for i, nd := range nodes {
		if i <= 1 {
			continue
		}
		cols := map[int]bool{}
		for li, id := range leaves {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				cols[columnAt(li, len(m.Chunks), m.K, m.N)] = true
			}
		}
		if len(cols) >= needColumns && len(cols) < victimCols {
			victim, victimCols = nd, len(cols)
		}
	}
	if victim == nil {
		t.Fatalf("rig: no holder carries ≥%d columns", needColumns)
	}
	return &confirmRig{sched: sched, net: net, nodes: nodes, reg: reg, m: m, h: h, care: care, log: lg, victim: victim}
}

func (r *confirmRig) sweep(t *testing.T) {
	t.Helper()
	r.care.sweepEpoch++
	done := false
	r.care.repairRoot(r.h.Care(), func() { done = true })
	r.sched.Run()
	if !done {
		t.Fatal("sweep never completed")
	}
}

func (r *confirmRig) count(event string) int {
	c := 0
	for _, e := range r.log.events {
		if e == event {
			c++
		}
	}
	return c
}

// TestRepairNeedsTwoConsecutiveObservations: a genuinely over-slack loss does
// NOT repair on the first sweep that sees it (the observation could be an
// unconverged record view — #514's captured false repair); the second
// consecutive sweep fires.
func TestRepairNeedsTwoConsecutiveObservations(t *testing.T) {
	r := newConfirmRig(t)
	r.sweep(t) // healthy baseline sweep (also converges the caretaker's view)
	if got := r.care.Stats.Repairs; got != 0 {
		t.Fatalf("rig: healthy sweep repaired (%d) — placement/records unconverged in the rig itself", got)
	}

	r.net.Kill(r.victim.ID())

	r.sweep(t)
	if r.care.Stats.Repairs != 0 {
		t.Fatalf("#517: the FIRST over-slack observation fired a repair — one probe sample must never trigger (network-durability §3)")
	}
	if r.count("stripe repair pending confirmation — one more sweep must agree") == 0 {
		t.Fatal("#517: the deferred repair did not narrate 'pending confirmation'")
	}

	r.sweep(t)
	if r.care.Stats.Repairs == 0 {
		t.Fatal("#517: the SECOND consecutive observation must fire the repair — the gate is stuck, a real loss goes unrepaired")
	}
}

// TestRepairConfirmResetsOnCleanSweep: an over-slack blip that clears (the
// holder was reachable again by the next sweep) resets the counter — the next
// loss event needs two fresh consecutive observations, not one.
func TestRepairConfirmResetsOnCleanSweep(t *testing.T) {
	r := newConfirmRig(t)
	r.sweep(t) // converge

	r.net.Kill(r.victim.ID())
	r.sweep(t) // observation 1 → pending
	if r.care.Stats.Repairs != 0 {
		t.Fatal("rig: first observation repaired — gate broken (covered by the sibling test)")
	}

	// The blip clears: the holder comes back. A real restarted daemon
	// re-announces at boot (proof of life clears its corpse entry); the sim
	// endpoint just revives silently, so recovery here rides the documented
	// cooldown-expiry re-admission (#69) — advance past HolderCooldown so the
	// walk re-finds the sole holder's records.
	r.net.Restart(r.victim.ID())
	lapsed := false
	r.sched.AfterFunc(31*ports.Second, func() { lapsed = true })
	r.sched.Run()
	if !lapsed {
		t.Fatal("sim gap never elapsed")
	}
	r.sweep(t) // clean → reset
	if r.care.Stats.Repairs != 0 {
		t.Fatal("#517: a clean sweep after the blip still repaired")
	}

	// A NEW loss event: one observation alone must not fire (the counter was
	// reset by the clean sweep), the second must.
	r.net.Kill(r.victim.ID())
	r.sweep(t)
	if r.care.Stats.Repairs != 0 {
		t.Fatal("#517: the counter survived a clean sweep — a single fresh observation fired")
	}
	r.sweep(t)
	if r.care.Stats.Repairs == 0 {
		t.Fatal("#517: the second fresh observation must fire")
	}
}
