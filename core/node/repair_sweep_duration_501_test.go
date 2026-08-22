package node

// #501 measurement rig: how long does ONE repair sweep take when holders die,
// and where does the time go? The field attribution (runs f58d599-17479 /
// 86dd6a4-9492) measured ~2.1s healthy → ~3-4 min after killing 2 holders, and
// named the suspect coarsely: probe/lookup timeouts toward the dead. This rig
// reproduces the shape deterministically in sim time (instant wall clock) under
// DAEMON-faithful transport settings (cmd/silt/daemon.go flag defaults, not the
// sim's 500ms/no-retry DefaultConfig), and attributes the duration by phase via
// the sweep narration plus request-layer counters. The eventual #501 fix turns
// the measured numbers into a bound assertion here.

import (
	"bytes"
	"sort"
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

// captureLog records every narration line so the test can read the #501 phase
// fields (manifest-heal-ms / probe-ms / repair-ms) off the sweep-complete lines.
type captureLog struct {
	clock  ports.Clock
	events []string
	kvs    [][]any
	at     []ports.Time
}

func (c *captureLog) Enabled(ports.LogLevel) bool { return true }
func (c *captureLog) Log(_ ports.LogLevel, event string, kv ...any) {
	c.events = append(c.events, event)
	c.kvs = append(c.kvs, append([]any(nil), kv...))
	c.at = append(c.at, c.clock.Now())
}

// dumpSince prints every captured transport-verdict line (timeout / abandon /
// retry-exhaustion) after mark, with sim-time offsets — the walk-wall autopsy.
func (c *captureLog) dumpSince(t *testing.T, mark int, origin ports.Time) {
	t.Helper()
	for i := mark; i < len(c.events); i++ {
		switch c.events[i] {
		case "request timeout", "request abandoned: peer negative-cached mid-ladder",
			"repair sweep complete", "repair pass complete", "repair below k", "stripe repaired":
			t.Logf("  +%6dms %s %v", msBetween(origin, c.at[i]), c.events[i], c.kvs[i])
		}
	}
}

// last returns the kv map of the most recent occurrence of event, or nil.
func (c *captureLog) last(event string) map[string]any {
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i] == event {
			m := map[string]any{}
			kv := c.kvs[i]
			for j := 0; j+1 < len(kv); j += 2 {
				if k, ok := kv[j].(string); ok {
					m[k] = kv[j+1]
				}
			}
			return m
		}
	}
	return nil
}

// daemonFaithfulConfig mirrors cmd/silt/daemon.go's transport flag defaults —
// the settings the field runs actually ran under (the fleet raises
// request-timeout to 8s; 5s is the shipped default). The sim DefaultConfig's
// 500ms/no-retry timeouts would hide the walk-ladder cost entirely.
func daemonFaithfulConfig() Config {
	cfg := DefaultConfig()
	cfg.RequestTimeout = 5 * ports.Second
	cfg.RequestRetries = 3
	cfg.RequestBackoff = 250 * ports.Millisecond
	cfg.HolderDialTimeout = 2 * ports.Second
	cfg.HolderCooldown = 30 * ports.Second
	cfg.DHTDomainCap = 2 // the daemon/client always run the diversity sweep leg
	// Field-faithful placement: the measured runs published at replication 1
	// (30 placements for 30 chunks — the economy wire-loop fact "a killed node
	// takes ALL its columns"). Higher replication masks the kill entirely: the
	// survivors still cover every shard and no repair ever fires.
	cfg.Replication = 1
	return cfg
}

type sweepRig struct {
	sched *simclock.Scheduler
	net   *simnet.Network
	nodes []*Node
	reg   ports.Registry
	m     *manifest.Manifest
	h     link.Handle
	care  *Node
	log   *captureLog
}

func newSweepRig(t *testing.T) *sweepRig {
	t.Helper()
	const N = 24
	sched := simclock.New()
	net := simnet.New(sched, 42, simnet.DefaultConfig())
	reg := registry.New()
	cfg := daemonFaithfulConfig()

	var nodes []*Node
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(2000 + i)).NodeID()
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

	// Field-scale object: the measured runs published ~30 chunks (29 shards +
	// manifest). 80 KiB at 4 KiB chunks = 20 data + 12 parity = 32 shards over
	// k=10/n=16 — big enough for two stripes, small enough that every column
	// finds a distinct closest holder and the healthy baseline probes clean.
	data := make([]byte, 80<<10)
	for i := range data {
		data[i] = byte(i*7 + 3)
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
	return &sweepRig{sched: sched, net: net, nodes: nodes, reg: reg, m: m, h: h, care: care, log: lg}
}

// sweepOnce drives exactly one repair sweep of the cared root to completion and
// returns its sim-time duration in milliseconds.
func (r *sweepRig) sweepOnce(t *testing.T) int64 {
	t.Helper()
	r.care.sweepEpoch++ // one driven sweep = one tick, exactly as repairTick does
	start := r.sched.Now()
	var end ports.Time
	finished := false
	r.care.repairRoot(r.h.Care(), func() { end, finished = r.sched.Now(), true })
	r.sched.Run()
	if !finished {
		t.Fatal("repair sweep never completed")
	}
	return msBetween(start, end)
}

// shardHolders ranks the nodes (publisher and caretaker excluded) by how many
// of the object's data+parity shards they physically hold.
func (r *sweepRig) shardHolders() []int {
	ids := append(append([]ports.ChunkID(nil), r.m.ChunkIDs()...), r.m.ParityIDs()...)
	type held struct{ idx, count int }
	var hs []held
	for i, nd := range r.nodes {
		if i <= 1 {
			continue
		}
		c := 0
		for _, id := range ids {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				c++
			}
		}
		if c > 0 {
			hs = append(hs, held{i, c})
		}
	}
	sort.Slice(hs, func(a, b int) bool { return hs[a].count > hs[b].count })
	out := make([]int, len(hs))
	for i, h := range hs {
		out[i] = h.idx
	}
	return out
}

// phaseReport pulls the #501 phase attribution off the caretaker's narration.
func (r *sweepRig) phaseReport(t *testing.T) (manifestMS, probeMS, repairMS, reachable any) {
	t.Helper()
	sweep := r.log.last("repair sweep complete")
	pass := r.log.last("repair pass complete")
	if sweep == nil || pass == nil {
		t.Fatal("sweep narration lines missing (repair sweep complete / repair pass complete)")
	}
	return sweep["manifest-heal-ms"], sweep["probe-ms"], pass["repair-ms"], sweep["reachable"]
}

// TestMeasure_501_SweepDurationUnderDeadHolders is the #501 attribution
// instrument: one healthy sweep, then the FIRST sweep after two shard holders
// die (empty negative cache — every dead contact pays its full discovery cost),
// then an immediate second sweep (caches warm, inside HolderCooldown). It
// prints durations, phase attribution, and request-layer counters. Measurement,
// not yet a bound: the #501 fix adds the assertion.
func TestMeasure_501_SweepDurationUnderDeadHolders(t *testing.T) {
	r := newSweepRig(t)

	// Census: is every shard physically SOMEWHERE? Separates a distribution
	// shortfall (shards never placed) from a discovery shortfall (placed but
	// the probe can't find them) — the two pollute the baseline differently.
	ids := append(append([]ports.ChunkID(nil), r.m.ChunkIDs()...), r.m.ParityIDs()...)
	present := 0
	for _, id := range ids {
		for _, nd := range r.nodes {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				present++
				break
			}
		}
	}
	t.Logf("census after distribute: %d/%d shards physically present in the swarm", present, len(ids))

	// Converge the healthy baseline: sweep until reachability stops improving
	// (early sweeps may repair a distribution shortfall; the field baseline was
	// a stable 29/29 at ~2.1s). The LAST converged sweep is the healthy number.
	var healthyMS int64
	var mh, pr, rp, reach any
	prevReach := -1
	for i := 0; i < 6; i++ {
		healthyMS = r.sweepOnce(t)
		mh, pr, rp, reach = r.phaseReport(t)
		t.Logf("healthy sweep %d: %d ms (manifest-heal=%v probe=%v repair=%v reachable=%v)", i, healthyMS, mh, pr, rp, reach)
		cur, _ := reach.(int)
		if cur == prevReach {
			break
		}
		prevReach = cur
	}

	holders := r.shardHolders()
	if len(holders) < 3 {
		t.Fatalf("distribution too concentrated to stage the kill: %d shard holders", len(holders))
	}
	killed := holders[:2] // the field run killed 2 holders (3 columns, 7 shards)
	killedShards := 0
	for _, i := range killed {
		for _, id := range ids {
			if ok, _ := r.nodes[i].Store().Has(bg(), id); ok {
				killedShards++
			}
		}
		r.net.Kill(r.nodes[i].ID())
	}
	t.Logf("killed %d holders carrying %d/%d shards", len(killed), killedShards, len(ids))

	statsBefore := r.care.Stats
	mark := len(r.log.events)
	origin := r.sched.Now()
	firstDeadMS := r.sweepOnce(t)
	mh, pr, rp, reach = r.phaseReport(t)
	d := r.care.Stats
	t.Logf("first sweep after kill: %d ms (manifest-heal=%v probe=%v repair=%v reachable=%v)", firstDeadMS, mh, pr, rp, reach)
	t.Logf("  queries=%d timeouts=%d dials-skipped=%d probes=%d",
		d.QueriesSent-statsBefore.QueriesSent, d.Timeouts-statsBefore.Timeouts,
		d.HolderDialsSkipped-statsBefore.HolderDialsSkipped, d.Probes-statsBefore.Probes)
	r.log.dumpSince(t, mark, origin)

	statsBefore = d
	warmDeadMS := r.sweepOnce(t)
	mh, pr, rp, reach = r.phaseReport(t)
	d = r.care.Stats
	t.Logf("second (cache-warm) sweep: %d ms (manifest-heal=%v probe=%v repair=%v reachable=%v)", warmDeadMS, mh, pr, rp, reach)
	t.Logf("  queries=%d timeouts=%d dials-skipped=%d probes=%d",
		d.QueriesSent-statsBefore.QueriesSent, d.Timeouts-statsBefore.Timeouts,
		d.HolderDialsSkipped-statsBefore.HolderDialsSkipped, d.Probes-statsBefore.Probes)

	statsBefore = d
	steadyMS := r.sweepOnce(t)
	mh, pr, rp, reach = r.phaseReport(t)
	d = r.care.Stats
	t.Logf("third (steady-state, object healed) sweep: %d ms (manifest-heal=%v probe=%v repair=%v reachable=%v)", steadyMS, mh, pr, rp, reach)
	t.Logf("  queries=%d timeouts=%d dials-skipped=%d probes=%d",
		d.QueriesSent-statsBefore.QueriesSent, d.Timeouts-statsBefore.Timeouts,
		d.HolderDialsSkipped-statsBefore.HolderDialsSkipped, d.Probes-statsBefore.Probes)

	if healthyMS <= 0 {
		t.Fatal("healthy sweep reported no duration — the phase clocks are broken")
	}

	// #501 bound 1 — the first sweep after a kill is BOUNDED: ≤ one full
	// discovery ladder per corpse (~22s each at these settings, phases meet the
	// corpses disjointly). Measured pre-fix: 159.2s (intra-sweep cooldown
	// lapses re-paid ladders five times over); post-fix with the #517
	// confirmation gate: 45.3s (walls only — the repair itself fires on the
	// SECOND consecutive observation, measured 72.8s with warm caches). The
	// bound sits between, with headroom for seed drift.
	if firstDeadMS > 100_000 {
		t.Fatalf("#501: first sweep after kill took %d ms — the sweep-scoped corpse gate is not bounding it (pre-fix behavior was ~159s)", firstDeadMS)
	}

	// #501 bound 2 — the re-discovery tax DECAYS. Pre-fix, any sweep starting
	// past the flat 30s cooldown re-paid the full double ladder wall (~45s,
	// ~40 timeouts), forever, even on a fully healed object. With the decaying
	// cooldown each re-discovery doubles the quiet period, so driving sweeps at
	// a fixed 35s gap must go quiet within a few rounds and STAY quiet.
	gap := func() {
		fired := false
		r.sched.AfterFunc(35*ports.Second, func() { fired = true })
		r.sched.Run()
		if !fired {
			t.Fatal("sim gap never elapsed")
		}
	}
	var tail []int64
	for i := 0; i < 6; i++ {
		gap()
		before := r.care.Stats.Timeouts
		ms := r.sweepOnce(t)
		t.Logf("decay-drive sweep %d (35s gap): %d ms, %d timeouts", i, ms, r.care.Stats.Timeouts-before)
		if i >= 4 {
			tail = append(tail, ms)
		}
	}
	for _, ms := range tail {
		if ms > 10_000 {
			t.Fatalf("#501: a 35s-gap sweep still costs %d ms after six rounds — the corpse cooldown is not decaying (pre-fix: every such sweep re-paid ~45s)", ms)
		}
	}
}
