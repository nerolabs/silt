package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// #397 Q1b — the crash variant of the honest double-sign: a validator signs a
// proposal, crashes before it commits, restarts, and receives a COMPETING
// block at the same height. With only an in-memory ledger the restart wipes
// the never-sign-twice mark and the node attests the competitor — equivocating
// against its own pre-crash signature and getting permanently slashed (F2)
// for a crash. With the persisted watermark (research-certified: mark made
// durable BEFORE the signature is released, Tendermint priv_validator_state)
// the restarted node refuses.
func TestRestartedValidatorRefusesToContradictPreCrashSignature397(t *testing.T) {
	mark := markstore.NewMem() // one durable store across the simulated restart

	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }
	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	rival := identity.FromSeed(2)

	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, rival.Signer())

	boot := func(sched *simclock.Scheduler, net *simnet.Network) *Node {
		nd := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
		nd.SetLedger(ledger)
		ch := chain.New(chain.DefaultConfig(), repFn)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, ti.Signer())
		if err := nd.SetSignMarkStore(mark); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		return nd
	}

	// Boot 1: sign a proposal at height 1 (gather to a dead attester — the
	// signature is released, the block never commits), then "crash".
	sched1 := simclock.New()
	net1 := simnet.New(sched1, 1, simnet.DefaultConfig())
	nd1 := boot(sched1, net1)
	ledger.RecordBondChallenge(tid, ports.HashBytes([]byte{1}), 64<<20, true, 1)
	ledger.RecordBondChallenge(rival.NodeID(), ports.HashBytes([]byte{2}), 64<<20, true, 1)
	deadID := identity.FromSeed(9).NodeID()
	_ = net1.Endpoint(deadID)
	own := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("own")}}
	nd1.proposeBlock(own, []ports.NodeID{deadID}, nil, 1, func(error) {})
	sched1.Run()

	// Boot 2: fresh Node (in-memory state gone), same mark store. The rival's
	// competing block at height 1 arrives for attestation.
	sched2 := simclock.New()
	net2 := simnet.New(sched2, 1, simnet.DefaultConfig())
	nd2 := boot(sched2, net2)
	_ = nd2

	comp := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("competitor")}}
	chain.Sign(comp, rival.Signer())

	prid := identity.FromSeed(3).NodeID()
	pr := net2.Endpoint(prid)
	var replies []bool
	pr.SetHandler(func(_ ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgAttestReply {
			replies = append(replies, msg.OK)
		}
	})
	_ = pr.Send(tid, ports.Message{Kind: ports.MsgProposeBlock, Data: chain.Encode(comp)})
	sched2.Run()

	if len(replies) != 1 {
		t.Fatalf("expected exactly one attest reply, got %v", replies)
	}
	if replies[0] {
		t.Fatal("a restarted validator attested a block contradicting its pre-crash signature — the sign mark must survive restart (#397 Q1b)")
	}

	// And the persisted mark still allows normal progress: a block at height 2
	// building on nothing-signed territory would be attestable — the guard is
	// monotonic, not a lockout. (Height 2 needs a valid Prev; assert via
	// signAllowedAt directly to keep the check pure.)
	if !nd2.signAllowedAt(2, 0, chain.PhasePrepare, ports.HashBytes([]byte("any"))) {
		t.Fatal("the watermark must allow signing at heights above the mark")
	}
}
