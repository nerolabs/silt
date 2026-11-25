package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestFalseRepairClaimIsSlashedOnChain: a judge that holds a validator's signed false
// repair claim sends the proof to the validators, and the claimant is slashed on every
// replica: evicted, and no longer bond-qualified, so no judge pays its claims again. The
// judge here is a non-voting follower, the shape a repair judge takes. A forged proof
// (the same claim, true) is refused at the validators and slashes nobody.
func TestFalseRepairClaimIsSlashedOnChain(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 6, simnet.DefaultConfig())

	ids := []*identity.Identity{identity.FromSeed(8140), identity.FromSeed(8141), identity.FromSeed(8142)}
	jid := identity.FromSeed(8143)
	anchors := map[ports.NodeID]bool{}
	for _, id := range ids {
		anchors[id.NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-false-repair")}}
	chain.Sign(g, ids[0].Signer())
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
	var vals []*Node
	for _, id := range ids {
		vals = append(vals, mk(id, false))
	}
	judge := mk(jid, true)
	var all []ports.NodeID
	for _, id := range ids {
		all = append(all, id.NodeID())
	}
	for _, v := range vals[1:] {
		v.Bootstrap([]ports.NodeID{all[0]}, func() {})
	}
	judge.Bootstrap([]ports.NodeID{all[0]}, func() {})
	sched.Run()
	for i, v := range vals {
		var seed []ports.NodeID
		for j := range all {
			if j != i {
				seed = append(seed, all[j])
			}
		}
		v.StartChainSync(seed, nil)
	}
	judge.StartChainSync(all, nil)
	sched.RunUntil(sched.Now().Add(vals[0].cfg.ChainSyncInterval * 8))

	culprit := ids[2]
	if !judge.Chain().BondQualified(culprit.NodeID()) {
		t.Fatal("setup: the claimant's bond never committed, so there is nothing to slash")
	}

	// The claimant's signed claim names a shard the object does not commit at the leaf.
	leaves := make([]ports.Hash, 16)
	for i := range leaves {
		leaves[i] = ports.HashBytes([]byte{byte(i), 'f'})
	}
	const leaf = 11
	pr, _ := manifest.Prove(leaves, leaf)
	claim := repairproof.RepairClaim{Root: manifest.MerkleRoot(leaves), Stripe: 0, ShardPos: leaf, LeafIndex: leaf,
		ShardID: ports.ChunkID(ports.HashBytes([]byte("copied, not rebuilt"))), Holder: ports.NodeID{7}}
	proofFor := func(c repairproof.RepairClaim) chain.Equivocation {
		signed, err := repairproof.SignClaim(culprit.Signer(), judge.chainID(), c)
		if err != nil {
			t.Fatal(err)
		}
		return chain.Equivocation{Culprit: signed.Pub, FalseRepair: &chain.FalseRepairProof{
			Claim: signed, Leaf: leaves[leaf], Total: pr.Total, Path: pr.Path}}
	}

	// A forged accusation first: the same claim, but TRUE. Refused, slashes nobody.
	honest := claim
	honest.ShardID = ports.ChunkID(leaves[leaf])
	judge.SubmitSlash(proofFor(honest))
	sched.RunUntil(sched.Now().Add(vals[0].cfg.ChainSyncInterval * 6))
	if judge.Chain().IsSlashed(culprit.NodeID()) {
		t.Fatal("a proof of a TRUE claim slashed the claimant")
	}

	judge.SubmitSlash(proofFor(claim))
	sched.RunUntil(sched.Now().Add(vals[0].cfg.ChainSyncInterval * 12))
	for _, n := range append(vals, judge) {
		if !n.Chain().IsSlashed(culprit.NodeID()) {
			_, h := n.Chain().Head()
			t.Fatalf("%x at head %d: the false claimant is not slashed on this replica", n.ID().String()[:8], h)
		}
		if n.Chain().BondQualified(culprit.NodeID()) {
			t.Fatalf("%x: the slashed claimant still qualifies as bonded, so judges would pay it", n.ID().String()[:8])
		}
	}
}
