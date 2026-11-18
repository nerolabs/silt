package node

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// CATCH-UP ON A PATH SLOWER THAN THE WINDOW ASSUMES.
//
// A node one or two proof-carrying blocks behind asks a peer for the missing suffix.
// The serve window is sized to the configured floor, and the requester's deadline is
// capped. On a path a quarter of the floor, a window of two such blocks cannot cross
// inside the cap however far the deadline backs off, so every sweep times out and the
// node never catches up while the rest of the chain waits on it.
//
// The scale is the field's divided by 16: a 16 KiB/s floor for 256 KiB/s, a 4 KiB/s
// link for the measured quarter, and blocks of ~96 KiB for the ~1.5 MB registration
// blocks. Rates and sizes scale together, so every transfer time is the field's: one
// block takes ~24 s at the link rate and two take ~48, against the unscaled 30 s cap.
// Sixteen, not more, keeps every window above rateSampleMinBytes, below which an
// exchange is not sampled as a rate at all.

const (
	slowFloor = 16 << 10
	slowLink  = 4 << 10
	slowBlock = 96 << 10
)

func slowPathPair(t *testing.T) (*Node, *Node, *identity.Identity, *identity.Identity, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	ncfg := simnet.DefaultConfig()
	ncfg.RateBytesPerSec = slowLink
	net := simnet.New(sched, 4, ncfg)
	a1, a2 := identity.FromSeed(46701), identity.FromSeed(46702)
	anchors := map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-467")}}
	chain.Sign(g, a1.Signer())
	ccfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	mk := func(id *identity.Identity) *Node {
		cfg := DefaultConfig()
		cfg.RequestSizeFloorBytesPerSec = slowFloor
		nd := New(id.NodeID(), cfg, sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(ccfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	n1, n2 := mk(a1), mk(a2)
	n2.Bootstrap([]ports.NodeID{a1.NodeID()}, func() {})
	sched.Run()
	return n1, n2, a1, a2, sched
}

// growHeavy appends blocks of about slowBlock bytes each to n1's chain.
func growHeavy(t *testing.T, n1 *Node, a1, a2 *identity.Identity, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		prev, next := n1.Chain().Head()
		e := mkEntry(fmt.Sprintf("heavy-%d", i))
		for len(e.ManifestChunks)*len(ports.ChunkID{}) < slowBlock {
			e.ManifestChunks = append(e.ManifestChunks, ports.HashBytes([]byte(fmt.Sprintf("heavy-%d/%d", i, len(e.ManifestChunks)))))
		}
		b := &chain.Block{Version: 1, Height: next, Prev: prev, Entries: []ports.Entry{e}}
		chain.Sign(b, a1.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, a2.Signer())}
		if err := n1.Chain().Append(*b); err != nil {
			t.Fatalf("grow block %d: %v", i, err)
		}
	}
}

func TestCatchUpCompletesOnAPathSlowerThanTheWindowAssumes(t *testing.T) {
	n1, n2, a1, a2, sched := slowPathPair(t)
	const behind = 2
	growHeavy(t, n1, a1, a2, behind)

	one := len(chain.EncodeBlocks(n1.Chain().Blocks(1)[:1]))
	if one < slowBlock || one*slowFloor/slowLink <= int(requestSizeExtensionCap/ports.Second)*slowFloor/slowLink/2 {
		t.Fatalf("rig: a %d B block must be heavy enough that two of them outlast the cap at the link rate", one)
	}
	// The requester's own registration is what tells it how large a block a peer can
	// serve. Its contents are never read here, only its size.
	n2.ownBondReg = &chain.BondReg{Answer: make([]byte, one)}

	const sweeps = 12
	h1, _ := n1.Chain().Head()
	for s := 1; s <= sweeps; s++ {
		n2.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) {})
		sched.Run()
		if h2, _ := n2.Chain().Head(); h2 == h1 {
			t.Logf("caught up %d blocks of %d B on a %d B/s path in %d sweep(s), %v of simulated time",
				behind, one, slowLink, s, sched.Now())
			return
		}
	}
	t.Fatalf("after %d sweeps a node %d blocks of %d B behind, on a %d B/s path a quarter of the %d B/s floor, "+
		"has not caught up (len %d of %d): each window outlasts the deadline armed for it, so every sweep times out",
		sweeps, behind, one, slowLink, slowFloor, n2.Chain().Len(), n1.Chain().Len())
}
