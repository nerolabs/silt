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

// #286 Layer 2b: a fresh multi-validator objective genesis must NOT pile every founding
// validator's ~1.5 MB space-time bond registration into one block. The GCP cert stalled
// exactly there (`gather: starting height=1 bytes=7866154 regs=5`): an ~8 MB block cannot
// gather to quorum over a 3-region WAN before the round churns. The founding set are ANCHORS
// (chain.launchAnchor), so genesis can commit SMALL on anchor attestations at zero committed
// bond while the registrations drain over the next few blocks — every validator still gains
// real bonded weight and the set reaches maturity. Deterministic, in-process: the pile-up and
// the drain are consensus logic, not WAN timing (so this needs no netem).
func TestGenesisBondRegsSpreadAcrossBlocks286L2b(t *testing.T) {
	const bondSize = int64(64) << 20
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
	cfg := chain.Config{Quorum: 2, ByzantineQuorum: true, MinBond: 1 << 20, Anchors: anchors, MatureValidators: 4}

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

	bondedAll := func() bool {
		for _, id := range ids {
			if a.Chain().BondedSize(id.NodeID()) < cfg.MinBond {
				return false
			}
		}
		return true
	}

	// Drive it like the real network: each block, every not-yet-bonded validator (re)submits
	// its bond registration bound to the CURRENT head (a reg is signed over BondRegNonce(prev),
	// so an un-embedded one goes stale when the head moves — the drain is by resubmission, which
	// is exactly the 308 SubmitBondReg retries the cert showed). The proposer embeds at most the
	// cap per block; genesis commits small on the anchor bootstrap.
	maxRegsSeen := 0
	for h := 1; h <= 8 && !bondedAll(); h++ {
		prev, _ := a.Chain().Head()
		for i := 1; i < len(nodes); i++ {
			if a.Chain().BondedSize(ids[i].NodeID()) < cfg.MinBond {
				if reg, ok := nodes[i].RegisterBondReg(prev); ok {
					a.queuePendingBondReg(reg)
				}
			}
		}
		done := false
		var perr error
		a.ProposeEntry(mkEntry(fmt.Sprintf("e-%d", h)), attesters, broadcast, cfg.Quorum, func(e error) { perr, done = e, true })
		sched.Run()
		if !done {
			t.Fatalf("block %d never completed — genesis did not commit (anchor bootstrap broken?)", h)
		}
		if perr != nil {
			t.Fatalf("block %d proposal failed: %v", h, perr)
		}
		blk := blockAtHeight(a.Chain(), h)
		nRegs, regBytes := 0, int64(0)
		if blk != nil {
			nRegs = len(blk.BondRegs)
			for _, r := range blk.BondRegs {
				regBytes += int64(len(bondRegEncode(r)))
			}
		}
		if nRegs > maxRegsSeen {
			maxRegsSeen = nRegs
		}
		// The cap: a block's bond-reg bytes stay within the budget (the ~8 MB regs=5 pile-up
		// is what wedged the WAN cert). Exactly one oversized reg is allowed through so the
		// queue never stalls, so the byte check only bites when >1 reg is present.
		if nRegs > 1 && regBytes > a.cfg.MaxBondRegBytesPerBlock {
			t.Fatalf("block %d carries %d bond regs = %d bytes > budget %d — the ~8 MB genesis pile-up (#286 L2b) is back",
				h, nRegs, regBytes, a.cfg.MaxBondRegBytesPerBlock)
		}
		t.Logf("block %d committed with %d bond reg(s) = %d bytes; bondedAll=%v", h, nRegs, regBytes, bondedAll())
	}

	// Drain + maturity: every founding validator's bond landed on-chain within a few blocks,
	// so the network can reach MatureValidators (research guardrail: don't cap so low that
	// maturity never arrives).
	if !bondedAll() {
		t.Fatalf("bond registrations did not drain — not every validator became bonded (max regs/block seen=%d)", maxRegsSeen)
	}
}
