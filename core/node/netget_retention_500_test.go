package node

// #500 regressions: NetGet's pulled chunks are a WORKING SET (dropped after
// assembly, pre-held chunks untouched), and NetGetRetain converts them into
// real, discoverable, audit-answerable hosting (proof minted from the link's
// layout key, record under the placement key, announced to the near nodes).
// Before the fix, NetGet retained everything forever with no record — bytes
// counting against the pledge while invisible to every fetcher (the #497
// records-vs-bytes divergence, S5).

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

type netgetRig struct {
	sched *simclock.Scheduler
	net   *simnet.Network
	nodes []*Node
	reg   ports.Registry
	m     *manifest.Manifest
	h     link.Handle
	data  []byte
}

// newNetgetRig: publisher nodes[0], consumer nodes[1], third-party fetcher
// nodes[2]; replication 1 so every shard has exactly one original holder (a
// killed holder takes its columns — the field placement, and what makes the
// "third node discovers the retainer" assertion sharp).
func newNetgetRig(t *testing.T) *netgetRig {
	t.Helper()
	const N = 16
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1

	var nodes []*Node
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(3000 + i)).NodeID()
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
		data[i] = byte(i*11 + 5)
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
	return &netgetRig{sched: sched, net: net, nodes: nodes, reg: reg, m: m, h: h, data: data}
}

// candidates is every id a NetGet may pull: manifest chunks + all leaves.
func (r *netgetRig) candidates(t *testing.T) []ports.ChunkID {
	t.Helper()
	entry, ok, err := r.reg.Lookup(bg(), r.h.Root)
	if err != nil || !ok {
		t.Fatalf("lookup: %v", err)
	}
	return append(append([]ports.ChunkID(nil), entry.ManifestChunks...), r.m.Leaves()...)
}

func (r *netgetRig) heldOf(nd *Node, ids []ports.ChunkID) int {
	held := 0
	for _, id := range ids {
		if ok, _ := nd.Store().Has(bg(), id); ok {
			held++
		}
	}
	return held
}

// TestNetGetDropsWorkingSet: the default NetGet assembles, verifies, and keeps
// NOTHING it pulled — repair-path symmetry — while a chunk the node hosted
// BEFORE the call survives untouched.
func TestNetGetDropsWorkingSet(t *testing.T) {
	r := newNetgetRig(t)
	consumer := r.nodes[1]

	// Pre-seed one MANIFEST chunk into the consumer (registered under its own
	// id, so a scratch node can FetchChunk it; coded shards live under column
	// keys). The publisher deleted its copies after Distribute, so pull the
	// bytes from the swarm first.
	entry, _, _ := r.reg.Lookup(bg(), r.h.Root)
	preHeld := entry.ManifestChunks[0]
	var fetched bool
	r.nodes[2].FetchChunk(preHeld, func(err error) { fetched = err == nil })
	r.sched.Run()
	if !fetched {
		t.Fatalf("rig: could not fetch the pre-seed chunk %s", preHeld)
	}
	c, err := r.nodes[2].Store().Get(bg(), preHeld)
	if err != nil {
		t.Fatalf("rig: pre-seed read: %v", err)
	}
	if err := consumer.Store().Put(bg(), c); err != nil {
		t.Fatalf("rig: pre-seed put: %v", err)
	}
	r.nodes[2].dropHosted(preHeld) // scratch node keeps nothing

	var out bytes.Buffer
	var getErr error
	done := false
	consumer.NetGet(r.reg, r.h, &out, func(err error) { getErr, done = err, true })
	r.sched.Run()
	if !done || getErr != nil {
		t.Fatalf("netget: done=%v err=%v", done, getErr)
	}
	if !bytes.Equal(out.Bytes(), r.data) {
		t.Fatal("netget returned wrong bytes")
	}

	// Everything pulled is gone; the pre-held chunk survives; no orphan proofs.
	for _, id := range r.candidates(t) {
		ok, _ := consumer.Store().Has(bg(), id)
		if id == preHeld {
			if !ok {
				t.Fatalf("#500: NetGet dropped a chunk the node held BEFORE the call (%s)", id)
			}
			continue
		}
		if ok {
			t.Fatalf("#500: NetGet retained pulled chunk %s — the working set must drop after assembly", id)
		}
	}
	if len(consumer.proofMeta) != 0 {
		t.Fatalf("#500: NetGet left %d orphan proof entries", len(consumer.proofMeta))
	}
}

// TestNetGetRetainHostsAnnouncesAndServes: NetGetRetain keeps the pulls as
// REAL hosting — proofs registered (audit-answerable), records planted — and
// the acid test: after every original holder dies, a third node retrieves the
// whole object from the retainer alone.
func TestNetGetRetainHostsAnnouncesAndServes(t *testing.T) {
	r := newNetgetRig(t)
	consumer := r.nodes[1]

	var out bytes.Buffer
	var getErr error
	done := false
	consumer.NetGetRetain(r.reg, r.h, &out, func(err error) { getErr, done = err, true })
	r.sched.Run()
	if !done || getErr != nil {
		t.Fatalf("netget-retain: done=%v err=%v", done, getErr)
	}
	if !bytes.Equal(out.Bytes(), r.data) {
		t.Fatal("netget-retain returned wrong bytes")
	}

	// Hosting is real for what the call PULLED: NetGet fetches the k data
	// columns and touches parity only when data is missing, so the retained
	// set is the data leaves (parity retention is placement-dependent). Every
	// retained leaf must be audit-answerable: bytes present AND proof
	// registered — a provider that can't defend a challenge is a liability,
	// not redundancy.
	dataLeaves := r.m.ChunkIDs()
	if held := r.heldOf(consumer, dataLeaves); held != len(dataLeaves) {
		t.Fatalf("#500: retainer holds %d/%d data leaves", held, len(dataLeaves))
	}
	for _, id := range r.m.Leaves() {
		if ok, _ := consumer.Store().Has(bg(), id); !ok {
			continue
		}
		if _, proved := consumer.proofMeta[id]; !proved {
			t.Fatalf("#500: retained leaf %s has no registered proof — not audit-answerable", id)
		}
	}

	// The promise, end to end: kill every original holder (replication 1 — the
	// consumer and the never-hosting publisher excepted); a third node must
	// still retrieve the object, which is only possible via the retainer's
	// ANNOUNCED records.
	ids := r.candidates(t)
	for i, nd := range r.nodes {
		if i <= 2 {
			continue
		}
		if r.heldOf(nd, ids) > 0 {
			r.net.Kill(nd.ID())
		}
	}
	var out2 bytes.Buffer
	var err2 error
	done2 := false
	r.nodes[2].NetGet(r.reg, r.h, &out2, func(err error) { err2, done2 = err, true })
	r.sched.Run()
	if !done2 || err2 != nil {
		t.Fatalf("#500: third node could not retrieve from the retainer after all original holders died: done=%v err=%v", done2, err2)
	}
	if !bytes.Equal(out2.Bytes(), r.data) {
		t.Fatal("#500: third node got wrong bytes from the retainer")
	}
}

// TestNetGetRetainKeepsNothingOnFailure: a FAILED retrieval retains nothing —
// the retain promise applies to content actually delivered, and a partial
// working set drops exactly as in plain NetGet.
func TestNetGetRetainKeepsNothingOnFailure(t *testing.T) {
	r := newNetgetRig(t)
	consumer := r.nodes[1]

	// Kill every node holding a manifest chunk: assembly cannot start.
	entry, _, _ := r.reg.Lookup(bg(), r.h.Root)
	for i, nd := range r.nodes {
		if i == 1 {
			continue
		}
		if r.heldOf(nd, entry.ManifestChunks) > 0 {
			r.net.Kill(nd.ID())
		}
	}
	var out bytes.Buffer
	var getErr error
	done := false
	consumer.NetGetRetain(r.reg, r.h, &out, func(err error) { getErr, done = err, true })
	r.sched.Run()
	if !done {
		t.Fatal("netget-retain never completed")
	}
	if getErr == nil {
		t.Fatal("rig: expected the retrieval to fail with the manifest holders dead")
	}
	if held := r.heldOf(consumer, r.candidates(t)); held != 0 {
		t.Fatalf("#500: a failed NetGetRetain kept %d chunks — a failure must retain nothing", held)
	}
}
