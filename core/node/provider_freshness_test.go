package node

import (
	"bytes"
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

// TestAFileStaysFindableAfterItsProviderRecordsExpire is the field failure: every film the
// flixz re-import published more than half an hour earlier answered "manifest chunks
// unreachable", although three storage nodes held every piece. Provider records carry a
// 30-minute expiry, and the re-announce sweep over a big store took hours, so the holders'
// own records lapsed and they answered "no providers" for content on their own disks. Here
// the records are left to expire with no reprovide at all; a fresh node must still fetch the
// file, because a holder answers for what it holds.
func TestAFileStaysFindableAfterItsProviderRecordsExpire(t *testing.T) {
	const N = 12
	sched := simclock.New()
	net := simnet.New(sched, 11, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1
	cfg.ProviderRecordTTL = 60 * ports.Second
	var nodes []*Node
	for i := 0; i < N; i++ {
		ident := identity.FromSeed(int64(4100 + i))
		nd := New(ident.NodeID(), cfg, sched, net.Endpoint(ident.NodeID()), memstore.New())
		nd.SetSigner(ident.Signer()) // signed records are the ones that carry an expiry, as in a daemon
		nodes = append(nodes, nd)
	}
	for i, nd := range nodes {
		if i > 0 {
			nd.Bootstrap([]ports.NodeID{nodes[0].ID()}, func() {})
		}
	}
	sched.Run()
	data := bytes.Repeat([]byte("expires "), 10<<10)
	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: 4 << 10, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatal(err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	m, _ := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	nodes[0].Distribute(entry, m, false, func(int, error) {})
	sched.Run()
	for _, id := range append(append([]ports.ChunkID(nil), entry.ManifestChunks...), m.Leaves()...) {
		nodes[0].Store().Delete(bg(), id) // the publisher keeps nothing: only placement holders serve
	}

	// Every record expires; nothing re-announces.
	sched.RunUntil(sched.Now().Add(10 * cfg.ProviderRecordTTL))

	fi := identity.FromSeed(4199)
	fresh := New(fi.NodeID(), cfg, sched, net.Endpoint(fi.NodeID()), memstore.New())
	fresh.SetSigner(fi.Signer())
	fresh.Bootstrap([]ports.NodeID{nodes[1].ID(), nodes[2].ID()}, func() {})
	sched.Run()
	var out bytes.Buffer
	var getErr error
	finished := false
	fresh.NetGet(reg, h, &out, func(err error) { getErr, finished = err, true })
	sched.Run()
	if !finished || getErr != nil || !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("a file whose provider records expired could not be fetched: finished=%v err=%v", finished, getErr)
	}
}
