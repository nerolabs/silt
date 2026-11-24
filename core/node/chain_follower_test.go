package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestChainFollowerSyncsAndNeverVotes: a node that follows the chain keeps a replica
// that reaches the validators' head, so it can read committed state such as who is
// bonded, while it signs nothing. A proposal sent straight to it is refused, and its
// never-sign-twice mark stays unset for the whole run, so no consensus signature ever
// left it.
func TestChainFollowerSyncsAndNeverVotes(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())

	a1id, a2id, fid := identity.FromSeed(8130), identity.FromSeed(8131), identity.FromSeed(8132)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-follower")}}
	chain.Sign(g, a1id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}

	mk := func(id *identity.Identity, follow bool) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		if follow {
			nd.FollowChain(ch)
		} else {
			nd.EnableBond(id.Signer(), bondSize)
			nd.EnableChain(ch, id.Signer())
		}
		nd.EnableObjectiveChain()
		return nd
	}
	a1, a2, f := mk(a1id, false), mk(a2id, false), mk(fid, true)
	a2.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	f.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	sched.Run()

	a1.StartChainSync([]ports.NodeID{a2id.NodeID()}, nil)
	a2.StartChainSync([]ports.NodeID{a1id.NodeID()}, nil)
	f.StartChainSync([]ports.NodeID{a1id.NodeID(), a2id.NodeID()}, nil)
	sched.RunUntil(sched.Now().Add(a1.cfg.ChainSyncInterval * 8))

	_, ah := a1.Chain().Head()
	_, fh := f.Chain().Head()
	if ah < 1 {
		t.Fatalf("setup: the anchors committed nothing (head %d), so there is nothing to follow", ah)
	}
	if fh != ah {
		t.Fatalf("the follower reached head %d, the validators %d: a judge that cannot keep up reads stale bonds", fh, ah)
	}
	if f.Chain().BondedSize(a1id.NodeID()) == 0 {
		t.Fatal("the follower's replica shows no bond for an anchor that drained its bond registration: it cannot answer who is bonded")
	}

	// A well-formed, correctly signed proposal delivered directly. The control is that
	// the other anchor ATTESTS it, so the follower's refusal is its own rule and not a
	// broken proposal.
	propose := func(to ports.NodeID) (accepted, answered bool) {
		a1.ProposeGoodBlock(to, func(ok bool, err error) { accepted, answered = ok, err == nil })
		sched.RunUntil(sched.Now().Add(5 * ports.Second))
		return
	}
	if ok, ans := propose(a2id.NodeID()); !ans || !ok {
		t.Fatalf("control: a validator did not attest the well-formed proposal (answered=%v accepted=%v)", ans, ok)
	}
	if ok, ans := propose(fid.NodeID()); !ans || ok {
		t.Fatalf("the follower did not refuse a well-formed proposal (answered=%v accepted=%v)", ans, ok)
	}
	if f.signMarkSet {
		t.Fatalf("the follower recorded a consensus signature (mark %+v): a node with no key must never vote", f.signMark)
	}
}
