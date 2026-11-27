package node

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/adapters/capstore"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/ports"
)

// TestAFullPledgeStillFetches. A node's pledge is what it offers the network for HOSTING; a fetch's
// working set is not hosting. A node whose pledge is full must still fetch a file the network holds.
// It used to fail as "no shards available at all": each pulled piece went into the capped store,
// the full store refused it, and the refusal read as an unreachable holder (the flixz node at a full
// 5 GB pledge, 2026-11-27). Now the refused pieces are held in memory for the fetch, assembled from,
// and released — never hosted or announced.
func TestAFullPledgeStillFetches(t *testing.T) {
	r := newNetgetRig(t)
	full, err := capstore.Open(memstore.New(), 1) // a one-byte pledge: full before it starts
	if err != nil {
		t.Fatal(err)
	}
	id := identity.FromSeed(3999).NodeID()
	cfg := DefaultConfig()
	cfg.Replication = 1
	consumer := New(id, cfg, r.sched, r.net.Endpoint(id), full)
	consumer.Bootstrap([]ports.NodeID{r.nodes[0].ID(), r.nodes[1].ID(), r.nodes[2].ID()}, func() {})
	r.sched.Run()

	var out bytes.Buffer
	var getErr error
	done := false
	consumer.NetGet(r.reg, r.h, &out, func(err error) { getErr, done = err, true })
	r.sched.Run()
	if !done || getErr != nil {
		t.Fatalf("a node with a full pledge could not fetch a file the network holds: done=%v err=%v", done, getErr)
	}
	if !bytes.Equal(out.Bytes(), r.data) {
		t.Fatal("the fetch returned the wrong bytes")
	}
	if consumer.Stats.WorkingSetOverflow == 0 {
		t.Fatal("CONTROL: no piece went to the overflow, so this did not exercise a full pledge")
	}
	if len(consumer.workOverflow) != 0 || len(consumer.workingWant) != 0 {
		t.Fatalf("the working set outlived the fetch: %d held, %d wanted", len(consumer.workOverflow), len(consumer.workingWant))
	}
	if n := r.heldOf(consumer, r.candidates(t)); n != 0 {
		t.Fatalf("a full pledge ended up hosting %d pieces", n)
	}
}
