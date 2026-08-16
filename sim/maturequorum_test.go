package sim

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// B2 at the WIRE tier (research certification 2026-08-13): the mature-phase
// weight-counted quorum exercised through the real gather — live nodes, real
// MsgProposeBlock round-trips over simnet, the proposer's SupportMeetsQuorum
// loop deciding when the coalition it holds would commit.
//
// Topology: 4 honest 64 MiB validators + 9 MinBond single-domain cohort
// members, ALL live nodes, all seated in the first epoch snapshot (Condition A
// admission is unfiltered). Under the head-counted quorum this exact network
// was born unable to commit honestly (needs bftThreshold(13)=8 attesters; the
// honest side has 3) while the cohort alone COULD commit (9 heads) — the
// certified stall/capture pair, here driven end-to-end.
func TestMatureEpochWeightQuorumOverWire(t *testing.T) {
	const (
		seed       = int64(11)
		honestN    = 4
		sybilN     = 9
		honestBond = int64(64) << 20
		minBond    = int64(1) << 20
		sybilDom   = uint64(0x5ceb11)
	)
	cfg := chain.Config{Quorum: 2, MinBond: minBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 4}
	verify := func(_ []byte, _ ports.Hash, _ int64, _ uint64, answer []byte) bool {
		return string(answer) == "valid"
	}

	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())

	total := honestN + sybilN
	idents := make([]*identity.Identity, total)
	ids := make([]ports.NodeID, total)
	for i := range idents {
		idents[i] = identity.FromSeed(seed*1000 + int64(i))
		ids[i] = idents[i].NodeID()
	}
	honest, sybils := ids[:honestN], ids[honestN:]

	// One genesis block, shared by every replica: all 13 bonds registered, so
	// the genesis boundary (MatureValidators=0) is the handoff and the first
	// epoch snapshot seats honest whales and MinBond cohort alike.
	g := &chain.Block{Version: 1, Height: 0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("genesis")), ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("gm"))}}}}
	for i := 0; i < honestN; i++ {
		g.BondRegs = append(g.BondRegs, chain.NewBondReg(idents[i].Signer(),
			ports.HashBytes(ids[i][:]), honestBond, []byte("valid"), ports.Hash{}, uint64(i+1)))
	}
	for i := honestN; i < total; i++ {
		g.BondRegs = append(g.BondRegs, chain.NewBondReg(idents[i].Signer(),
			ports.HashBytes(ids[i][:]), minBond, []byte("valid"), ports.Hash{}, sybilDom))
	}
	chain.Sign(g, idents[0].Signer())

	nodes := make([]*node.Node, total)
	for i := range nodes {
		nd := node.New(ids[i], node.DefaultConfig(), sched, net.Endpoint(ids[i]), memstore.New())
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(verify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("node %d: genesis: %v", i, err)
		}
		if n := ch.RequiredQuorum(); n != cfg.Quorum {
			t.Fatalf("node %d: mature-epoch RequiredQuorum = %d, want the count floor %d", i, n, cfg.Quorum)
		}
		nd.EnableChain(ch, idents[i].Signer())
		nodes[i] = nd
	}
	for i := 1; i < total; i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
	}
	sched.Run()

	// THE STALL DRILL, END TO END: the cohort declines to attest — the proposer
	// only asks the honest validators. 3 attestations is 3 < bftThreshold(13)=8
	// heads, but 256 MiB of 265 MiB in weight: the gather must decide "enough"
	// by weight (SupportMeetsQuorum) and commit over the wire.
	e1 := ports.Entry{Root: ports.HashBytes([]byte("honest-block")), ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("m1"))}}
	var commitErr error
	committed := false
	nodes[0].ProposeEntry(e1, honest[1:], ids, cfg.Quorum, func(err error) { commitErr, committed = err, true })
	sched.Run()
	if !committed || commitErr != nil {
		t.Fatalf("STALL (wire): an honest >⅔-weight coalition must commit with the cohort declining: done=%v err=%v", committed, commitErr)
	}
	if _, ok := nodes[0].Chain().LookupRoot(e1.Root); !ok {
		t.Fatal("the honest block did not commit on the proposer's replica")
	}

	// THE CAPTURE DRILL, END TO END: the cohort proposes and gathers only from
	// itself. All 8 attestations arrive (the cohort cooperates), the count
	// floor is met — and the coalition still holds 9 MiB of 265 MiB, so the
	// propose must FAIL rather than mint a zero-honest-signature block.
	e2 := ports.Entry{Root: ports.HashBytes([]byte("cohort-block")), ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("m2"))}}
	var capErr error
	capDone := false
	nodes[honestN].ProposeEntry(e2, sybils[1:], ids, cfg.Quorum, func(err error) { capErr, capDone = err, true })
	sched.Run()
	if !capDone || capErr == nil {
		t.Fatalf("CAPTURE (wire): a MinBond cohort-only propose must fail, got done=%v err=%v", capDone, capErr)
	}
	if !errors.Is(capErr, chain.ErrNoQuorum) && !errors.Is(capErr, chain.ErrNoQuorumWeight) {
		t.Fatalf("cohort-only propose should fail for lack of quorum weight, got: %v", capErr)
	}
	for i, nd := range nodes {
		if _, ok := nd.Chain().LookupRoot(e2.Root); ok {
			t.Fatalf("replica %d accepted the cohort-only block — capture on the wire (B2)", i)
		}
	}
}
