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

// A validator that has SIGNED ITS OWN PROPOSAL at a height must refuse to
// attest a different block at that same height — the proposer's signature is a
// signature like any other, and signing a competitor afterwards is exactly the
// double-sign the slash rule treats as proven malice.
//
// Field evidence (cloud run b88245d-3496, 2026-08-14, flow 5-convergence FAIL):
// val-a and val-b both proposed an empty renewal block at height 6 (their
// genesis-aligned bond-renewal clocks made both eligible, and the drain
// designated-proposer fallback let both take over after 3 idle sweeps). Each
// then ATTESTED the other's competing block-6 — the attest guard consults only
// n.attested, which proposeBlock never writes — so each honestly signed two
// different blocks at height 6. The cross-fork scan then correctly slashed
// BOTH anchors on both branches ("chain: slashed equivocator … double-signed
// at height 6", repeating every sweep), and with 2 of 4 anchors slashed under
// anchor-quorum the chain wedged permanently at height 6 (2-2 fork,
// 98e587… vs d2e3307…). This test reproduces the signing half of that event
// deterministically: propose at height 1, then receive a competitor at height
// 1 — the attest reply must be a refusal.
func TestProposerRefusesToAttestCompetingBlock(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }

	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	tn := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
	tn.SetLedger(ledger)

	ch := chain.New(chain.DefaultConfig(), repFn)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	rival := identity.FromSeed(2)
	chain.Sign(g, rival.Signer())
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	tn.EnableChain(ch, ti.Signer())

	// Standing for both the target (to propose) and the rival (to have its
	// competing proposal pass ValidateProposal at the target).
	ledger.RecordBondChallenge(tid, ports.HashBytes([]byte{1}), 64<<20, true, 1)
	ledger.RecordBondChallenge(rival.NodeID(), ports.HashBytes([]byte{2}), 64<<20, true, 1)

	// 1) The target proposes its own block at height 1. The gather is sent to an
	//    attester that never replies — the proposal will not commit, but the
	//    target has SIGNED it, and a signature at a height is final whether or
	//    not the block lands (that finality is what makes a double-sign proof
	//    evidence of malice rather than accident).
	deadID := identity.FromSeed(9).NodeID()
	_ = net.Endpoint(deadID) // exists on the net, never replies
	own := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("own")}}
	tn.proposeBlock(own, []ports.NodeID{deadID}, nil, 1, func(error) {})
	sched.Run()

	// 2) A rival's competing block at the same height arrives for attestation.
	comp := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("competitor")}}
	chain.Sign(comp, rival.Signer())

	prid := identity.FromSeed(3).NodeID()
	pr := net.Endpoint(prid)
	var replies []bool
	pr.SetHandler(func(_ ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgAttestReply {
			replies = append(replies, msg.OK)
		}
	})
	_ = pr.Send(tid, ports.Message{Kind: ports.MsgProposeBlock, Data: chain.Encode(comp)})
	sched.Run()

	if len(replies) != 1 {
		t.Fatalf("expected exactly one attest reply, got %v", replies)
	}
	if replies[0] {
		t.Fatal("a proposer must REFUSE to attest a competing block at the height it signed — attesting it manufactures an honest double-sign (cloud run b88245d-3496: both anchors slashed, chain wedged at height 6)")
	}
}
