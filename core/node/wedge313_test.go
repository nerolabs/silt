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

// Repro for #313: on the GCP cert (4 objective validators, -quorum 2, Byzantine
// quorum ON, anchors via allowlist, NO genesis bonds) the chain commits block 1
// then WEDGES — every later publish hangs, chain stays at height 1, C2 shows
// "0 bonds". This drives the SAME parameters in-process (no latency) to see
// whether the wedge is a consensus/proposal-logic bug (would reproduce here) or
// purely a cross-region timing effect (would NOT reproduce here — commits fine).
func TestWedge313_ObjectiveByzantineMultiBlock(t *testing.T) {
	const bondSize = int64(64) << 20
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())

	// 4 objective validators, all in the anchor allowlist (mirrors the cloudtest
	// -anchors val-a,val-b,val-c,val-d). val-a is the boot proposer.
	ids := []*identity.Identity{
		identity.FromSeed(6001), identity.FromSeed(6002),
		identity.FromSeed(6003), identity.FromSeed(6004),
	}
	anchors := map[ports.NodeID]bool{}
	for _, id := range ids {
		anchors[id.NodeID()] = true
	}

	// Genesis carries NO bond regs (genesis.Build does the same on the real daemon):
	// the objective chain starts at qualifiedCount=0, anchors bootstrap via the
	// allowlist, validators self-register their bond via F6 ("proposing IS registering").
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis")}}
	chain.Sign(g, ids[0].Signer())

	cfg := chain.Config{
		Quorum:           2,    // cloudtest quorum = max(1, n_val-2) = 2
		ByzantineQuorum:  true, // objective default ON
		MinBond:          1 << 20,
		Anchors:          anchors,
		MatureValidators: 4, // cloudtest -mature-validators n_val
	}

	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		nodes[i] = mk(id)
	}
	a := nodes[0]

	// Mesh: b,c,d bootstrap to a.
	for i := 1; i < len(nodes); i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0].NodeID()}, func() {})
	}
	sched.Run()

	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	broadcast := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

	// Propose several blocks in a row — the real network wedged at block 2. Use a
	// UNIQUE entry per block (the real warm-up publishes 4KiB of /dev/urandom, so
	// every root differs — a duplicate root would be a test artifact, not the wedge).
	//
	// The bond registration carries the (potentially ~1.5MB, #299) space-time proof.
	// With the fix, ONLY the first proposal (when a is not yet bonded) may carry it;
	// every later block must be lean (no bond reg) until a TTL renewal is due — here
	// BondTTLBlocks is unset, so exactly ONE block should ever carry a reg.
	regBlocks := 0
	for h := 1; h <= 5; h++ {
		var propErr error
		var committed *chain.Block
		done := false
		a.ProposeEntry(mkEntry(fmt.Sprintf("entry-%d", h)), attesters, broadcast, cfg.Quorum, func(e error) { propErr, done = e, true })
		sched.Run()
		if !done {
			t.Fatalf("block %d: ProposeEntry never completed (WEDGE — the real symptom)", h)
		}
		if propErr != nil {
			t.Fatalf("block %d: proposal failed: %v (bonds on-chain: a=%d)", h, propErr, a.Chain().BondedSize(ids[0].NodeID()))
		}
		// Inspect the block just committed at height h.
		committed = blockAtHeight(a.Chain(), h)
		nRegs := 0
		if committed != nil {
			nRegs = len(committed.BondRegs)
		}
		if nRegs > 0 {
			regBlocks++
		}
		_, next := a.Chain().Head()
		t.Logf("block %d committed; bondRegs=%d; next height=%d; a bonded on-chain=%d qualifiedCount=%d",
			h, nRegs, next, a.Chain().BondedSize(ids[0].NodeID()), countBonded(a.Chain(), ids))
	}
	// The core of the fix: a bonded proposer must NOT re-embed its bond proof in
	// every block. With no TTL set, exactly one registration should ever appear.
	if regBlocks != 1 {
		t.Fatalf("expected exactly ONE bond-registration block (the first), got %d — a re-registering proposer bloats every block and wedges cross-region (#313)", regBlocks)
	}
	if a.Chain().BondedSize(ids[0].NodeID()) == 0 {
		t.Fatal("proposer should be bonded on-chain after its first registration")
	}
}

// The fix must NOT break the release-and-coast defense (H2/RT-2, bond-ttl): a
// validator with a TTL still has to renew with a FRESH proof before it lapses, so
// a released plot decays out. With BondTTLBlocks set, registrations must reappear
// on the TTL cadence (renewal point = half the TTL) — not vanish after the first.
func TestWedge313_RenewalStillHappensUnderTTL(t *testing.T) {
	const bondSize = int64(64) << 20
	const ttl = uint64(6)
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())

	ids := []*identity.Identity{
		identity.FromSeed(7001), identity.FromSeed(7002),
		identity.FromSeed(7003), identity.FromSeed(7004),
	}
	anchors := map[ports.NodeID]bool{}
	for _, id := range ids {
		anchors[id.NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 2, ByzantineQuorum: true, MinBond: 1 << 20, Anchors: anchors, MatureValidators: 4, BondTTLBlocks: ttl}

	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		nodes[i] = mk(id)
	}
	a := nodes[0]
	for i := 1; i < len(nodes); i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0].NodeID()}, func() {})
	}
	sched.Run()
	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	broadcast := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

	regHeights := []int{}
	for h := 1; h <= 10; h++ {
		done := false
		var propErr error
		a.ProposeEntry(mkEntry(fmt.Sprintf("e-%d", h)), attesters, broadcast, cfg.Quorum, func(e error) { propErr, done = e, true })
		sched.Run()
		if !done || propErr != nil {
			t.Fatalf("block %d proposal failed: %v", h, propErr)
		}
		if b := blockAtHeight(a.Chain(), h); b != nil && len(b.BondRegs) > 0 {
			regHeights = append(regHeights, h)
		}
	}
	t.Logf("registration heights under TTL=%d: %v", ttl, regHeights)
	// First registration at height 1, then renewals on the ~TTL/2 cadence — must be
	// MORE than one (renewal happens) but far FEWER than all 10 (not every block).
	if len(regHeights) < 2 {
		t.Fatalf("TTL renewal broken: only %d registration(s) in 10 blocks — a released plot would never decay out (release-and-coast defense lost)", len(regHeights))
	}
	if len(regHeights) >= 10 {
		t.Fatalf("still re-registering every block (%d) — the #313 bloat is not fixed", len(regHeights))
	}
}

func blockAtHeight(ch *chain.Chain, h int) *chain.Block {
	for _, b := range ch.Blocks(0) {
		if int(b.Height) == h {
			bb := b
			return &bb
		}
	}
	return nil
}

func countBonded(ch *chain.Chain, ids []*identity.Identity) int {
	n := 0
	for _, id := range ids {
		if ch.BondedSize(id.NodeID()) > 0 {
			n++
		}
	}
	return n
}
