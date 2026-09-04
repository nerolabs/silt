package main

// R2.10 / F8 — G-F8-6, the RUNTIME half (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §6 G-F8-6: "the daemon wires the ledger's source to the node's chainEpoch() ...
// never to a constant or a clock. After boot, advancing the chain by one epoch
// advances ledger.Epoch()."
//
// cmdDaemon has no callable seam (its wiring sits inside the flag-driven block), so
// this test drives the SEAM the daemon must call — wireLedgerEpochSource(ledger, nd)
// — on a real chain + node built in-process, and the SOURCE gate in
// f8_source_gates_test.go pins that cmdDaemon calls that seam exactly once with the
// daemon's own ledger and node. RED on main: the seam is an inert stub.

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

func TestF8_LedgerEpochFollowsTheChainThroughTheDaemonSeam(t *testing.T) {
	const EB = 2
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(9500)
	attester := identity.FromSeed(9501)

	ch := chain.New(chain.Config{Quorum: 1, EpochBlocks: EB}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{Version: chain.BlockVersionWitnessable, Height: 0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("f8-daemon-genesis"))}}}
	chain.Sign(&g, ident.Signer())
	if err := ch.AppendGenesis(g); err != nil {
		t.Fatal(err)
	}
	nd := node.New(ident.NodeID(), node.DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	nd.EnableChain(ch, ident.Signer())
	ledger := credit.New(relaypay.ShippedAnchorFace, 500_000) // the daemon's constructor line
	nd.SetLedger(ledger)

	// The seam under test.
	wireLedgerEpochSource(ledger, nd)

	if got, want := ledger.Epoch(), nd.DemandEpoch(); got != want || want != 0 {
		t.Fatalf("after boot ledger.Epoch() = %d, node DemandEpoch() = %d, want both 0", got, want)
	}
	appendBlock := func(i int) {
		t.Helper()
		prev, next := ch.Head()
		b := &chain.Block{Version: 1, Height: next, Prev: prev,
			Entries: []ports.Entry{{
				Root:           ports.HashBytes([]byte(fmt.Sprintf("f8-daemon-%d", i))),
				ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte(fmt.Sprintf("f8-daemon-%d/m", i)))},
			}}}
		chain.Sign(b, ident.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, attester.Signer())}
		if err := ch.Append(*b); err != nil {
			t.Fatalf("append block %d: %v", next, err)
		}
	}
	for epoch := uint64(1); epoch <= 2; epoch++ {
		for i := 0; i < EB; i++ {
			appendBlock(int(epoch)*10 + i)
		}
		if got := nd.DemandEpoch(); got != epoch {
			t.Fatalf("precondition: after %d blocks the node's epoch is %d, want %d", epoch*EB, got, epoch)
		}
		if got := ledger.Epoch(); got != epoch {
			t.Fatalf("the chain advanced to epoch %d but ledger.Epoch() = %d — the ledger's source is not the "+
				"node's chainEpoch() (a constant, a clock, or nothing: R-F8-SOURCE)", epoch, got)
		}
	}
}
