package sim

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// TestTrainingWheelsShedThroughTheNodeLoop is the integration tier for the
// launch-window training wheels (risk 15): the anchor requirement is enforced
// through the real propose → attest → commit → apply flow, and the maturity
// metric accumulates from actually-committed blocks — so the wheels shed in a
// running swarm, mechanically, once enough independent validators participate.
func TestTrainingWheelsShedThroughTheNodeLoop(t *testing.T) {
	const seed = int64(9)
	const total = 6 // [0]=proposer, [1]=anchor, [2..5]=independent validators

	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }

	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())

	idents := make([]*identity.Identity, total)
	ids := make([]ports.NodeID, total)
	for i := range idents {
		idents[i] = identity.FromSeed(seed*1000 + int64(i))
		ids[i] = idents[i].NodeID()
	}
	cfg := chain.Config{
		MinProposerRep: 100, MinAttesterRep: 100, Quorum: 2,
		Anchors:          map[ports.NodeID]bool{ids[1]: true},
		AnchorQuorum:     1,
		MatureValidators: 3,
	}
	for _, id := range ids { // everyone is bonded (clears the bar)
		ledger.RecordBondChallenge(id, synthRoot(id), 8<<20, true, 1)
	}

	var nodes []*node.Node
	for i := 0; i < total; i++ {
		st := memstore.New()
		nd := node.New(ids[i], node.DefaultConfig(), sched, net.Endpoint(ids[i]), st)
		nd.SetLedger(ledger)
		ch := chain.New(cfg, repFn)
		if gb, _, _, gerr := genesis.Build(st); gerr == nil {
			ch.AppendGenesis(gb)
		}
		nd.EnableChain(ch, idents[i].Signer())
		nodes = append(nodes, nd)
	}
	for i := 1; i < total; i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
	}
	sched.Run()

	prop := nodes[0]
	all := ids

	// OUTCOME 1: young network — an anchorless coalition is refused. Post-#402 the
	// proposer's GATHER enforces the launch anchor requirement (SupportMeetsQuorum
	// shares requiredLaunchAnchors/countAnchorSupport with ValidateCommit), so a
	// no-anchor attester set can't even ASSEMBLE a committable coalition — it fails at
	// the gather (ErrNoQuorum) rather than gathering to count-quorum and then
	// self-rejecting at Append (ErrAnchorRequired, the pre-#402 path). Either way the
	// launch window can't be captured by a Sybil quorum, and refusing at the gather is
	// the tighter behavior (no doomed block is ever built). Uses throwaway independents
	// ids[4]/ids[5] as attesters AND a throwaway PROPOSER (nodes[3]): a signature at a
	// height is final for proposer and attester alike (#397), so this failed attempt at
	// height 1 spends its signers' height-1 signatures. nodes[0] must stay unspent to
	// propose the real height-1 commit below; ids[3] only ever attests at height 2, so
	// its height-1 signature is expendable here.
	if err := propose(nodes[3], "young-no-anchor", []ports.NodeID{ids[4], ids[5]}, all, cfg.Quorum, sched); !errors.Is(err, chain.ErrNoQuorum) {
		t.Fatalf("immature commit without an anchor must be refused at the gather (ErrNoQuorum); got %v", err)
	}

	// OUTCOME 2: young network, quorum WITH the anchor — commits. Accumulate
	// distinct independents (ids[2], then ids[3], then ids[4]) via anchor-backed
	// commits until the network is mature.
	for i, name := range []string{"young-anchor", "acc1", "acc2"} {
		indep := ids[2+i]
		if err := propose(prop, name, []ports.NodeID{ids[1], indep}, all, cfg.Quorum, sched); err != nil {
			t.Fatalf("anchor-backed commit %q should succeed; got %v", name, err)
		}
	}
	if !prop.Chain().Mature() {
		t.Fatal("network should be mature after 3 distinct independent validators attested")
	}

	// OUTCOME 3: mature network — a quorum with NO anchor commits. The wheels
	// shed on measured decentralization, through the running swarm.
	if err := propose(prop, "mature-no-anchor", []ports.NodeID{ids[2], ids[4]}, all, cfg.Quorum, sched); err != nil {
		t.Fatalf("mature commit without anchors should succeed (wheels shed); got %v", err)
	}
}
