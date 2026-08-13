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

// #338 — an IDLE young objective network must drain its pending bond
// registrations without depending on unrelated publish traffic.
//
// The #336 design deferred founding bond registrations off the lean genesis "to
// drain over the next blocks" — but pending registrations were only ever folded
// into a block inside proposeBlock, and a proposal was only ever TRIGGERED by a
// publish or a revocation. On a young network with no content traffic nothing
// proposes, so the deferred registrations sat forever: no validator (anchors
// included) ever earned committed bonded standing, the C2 metric read
// `nakamoto 0 bonds`, maturity was unreachable, and a non-anchor validator
// (the local sybil harness's s1/s2; the cloud run's sybil cohort) could never
// bank the committed standing the capture drill needs (the field GAP
// "sybil-1 never synced a committed chain — anchors hadn't banked the Sybil
// bonds"). The fix is a REACTIVE drain: on the existing chain-sync tick, a
// proposer-eligible validator holding pending peer registrations (or whose own
// registration is due) proposes a BondRegs-only block — reacting to pending
// state and quiescing when there is none (B6), under the same #286-L2b byte
// budget as any other block.
//
// This test is the failing-first regression: two anchors + one non-anchor
// bonded validator, genesis committed, NO publishes ever. The non-anchor
// submits its registration over the wire (the H2 non-proposer path, driven by
// its own chain-sync sweep); within a few sync intervals the anchors must have
// drained it — and their own — into committed blocks, and the non-anchor must
// have caught up to the committed chain it now has standing on.
func TestIdleYoungNetworkDrainsPendingBondRegs338(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())

	a1id, a2id, sid := identity.FromSeed(8101), identity.FromSeed(8102), identity.FromSeed(8103)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338")}}
	chain.Sign(g, a1id.Signer())
	// The local sybil harness shape: young forever (the anchor gate is the
	// property under test there), anchor co-sign required, quorum 1.
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}

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
	a1, a2, s := mk(a1id), mk(a2id), mk(sid)

	// Mesh: everyone bootstraps to a1 (the harness/bootstrap shape).
	a2.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	s.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	sched.Run()

	// The real daemon wiring: every validator runs the periodic chain-sync sweep
	// (which also submits due bond renewals to its targets). The anchors seed each
	// other; the non-anchor seeds the anchors (its -attesters analogue).
	a1.StartChainSync([]ports.NodeID{a2id.NodeID()}, nil)
	a2.StartChainSync([]ports.NodeID{a1id.NodeID()}, nil)
	s.StartChainSync([]ports.NodeID{a1id.NodeID(), a2id.NodeID()}, nil)

	// NO publishes. Let the network idle across several sync intervals — the
	// reactive drain must commit the pending registrations on its own.
	deadline := sched.Now().Add(a1.cfg.ChainSyncInterval * 8)
	sched.RunUntil(deadline)

	if got := a1.Chain().BondedSize(sid.NodeID()); got < cfg.MinBond {
		_, h := a1.Chain().Head()
		for i, blk := range a1.Chain().Blocks(0) {
			owners := ""
			for _, r := range blk.BondRegs {
				o := r.ValidatorID()
				owners += o.String()[:8] + " "
			}
			t.Logf("a1 block %d: regs=[%s] proposer=%s", i, owners, blk.ProposerID().String()[:8])
		}
		_, a2h := a2.Chain().Head()
		_, sh := s.Chain().Head()
		t.Logf("heads: a1=%d a2=%d s=%d; bonded on a1: a1=%d a2=%d s=%d; s pending on a1=%d",
			h, a2h, sh, a1.Chain().BondedSize(a1id.NodeID()), a1.Chain().BondedSize(a2id.NodeID()),
			a1.Chain().BondedSize(sid.NodeID()), len(a1.pendingBondRegs))
		t.Fatalf("#338: the idle young network never drained the non-anchor's submitted bond reg "+
			"(bonded %d < MinBond %d on a1, head %d) — pending regs must not wait for publish traffic", got, cfg.MinBond, h)
	}
	if got := a1.Chain().BondedSize(a1id.NodeID()); got < cfg.MinBond {
		t.Fatalf("#338: the proposing anchor never banked its OWN bond registration (bonded %d)", got)
	}
	// The non-anchor validator must also have SYNCED the committed chain it now
	// has standing on (issue #338 finding 2: s1/s2 stuck at an empty chain).
	_, ah := a1.Chain().Head()
	_, sh := s.Chain().Head()
	if sh != ah {
		t.Fatalf("#338: the non-anchor validator never caught up (s head %d, a1 head %d)", sh, ah)
	}
}

// #338 finding 2 (the structural leg): the configured persistent-peer tier IS
// the consensus set, so the chain-sync sweep must target it. A validator whose
// -attesters and gossip view are empty (the cloud sybil cohort: attesters are
// only other sybils, none of whom hold the chain) but which is CONFIGURED with
// the validator set via -persistent-peers must include those peers in its sync
// targets — configure-not-discover (docs/network-durability.md §8).
func TestSyncTargetsIncludeStaticPeers338(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	id := identity.FromSeed(8110)
	vid := identity.FromSeed(8111).NodeID()

	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	ch := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 0 })
	nd.EnableChain(ch, id.Signer())

	nd.AddStaticPeer(vid)
	found := false
	for _, p := range nd.syncTargets() {
		if p == vid {
			found = true
		}
	}
	if !found {
		t.Fatal("#338: a configured static (persistent) consensus peer must be a chain-sync target — " +
			"a validator with no attester seed and no bond gossip otherwise has no path to the committed chain")
	}
}

// TestNonAttesterSyncsViaStaticPeerOnly338 mirrors the CLOUD sybil topology
// exactly, which the drain test above did NOT: there the non-anchor was handed
// the anchors as its StartChainSync SEED, so it would have synced even without
// the static-tier fix (the same masking the local integration/sybil harness has,
// where the sybils list the anchors in -attesters). Here the non-anchor's sync
// seed is EMPTY and it holds NO bond gossip — its ONLY path to the committed
// chain is the configured static (-persistent-peers) tier. This is the exact
// SYBILS=8 field GAP ("sybil-1 never synced a committed chain, head height 0"):
// if static peers are not actually reconciled against, the node stays stranded
// at genesis forever. Failing-first without the syncTargets static-tier fix.
func TestNonAttesterSyncsViaStaticPeerOnly338(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())

	a1id, a2id, sid := identity.FromSeed(8120), identity.FromSeed(8121), identity.FromSeed(8122)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338b")}}
	chain.Sign(g, a1id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}

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
	a1, a2, s := mk(a1id), mk(a2id), mk(sid)

	a2.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	s.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	sched.Run()

	// The anchors drain their bonds and commit real blocks (the network the sybil
	// must catch up TO). They seed each other normally.
	a1.StartChainSync([]ports.NodeID{a2id.NodeID()}, nil)
	a2.StartChainSync([]ports.NodeID{a1id.NodeID()}, nil)

	// THE SYBIL, cloud-faithful: empty attester seed, NO peer-bond gossip primed —
	// its ONLY configured path to the validators is the static tier.
	s.AddStaticPeer(a1id.NodeID())
	s.AddStaticPeer(a2id.NodeID())
	s.StartChainSync(nil, nil)

	sched.RunUntil(sched.Now().Add(a1.cfg.ChainSyncInterval * 8))

	_, ah := a1.Chain().Head()
	_, sh := s.Chain().Head()
	if ah < 1 {
		t.Fatalf("setup: the anchors never committed a drain block (a1 head %d) — nothing to sync to", ah)
	}
	if sh != ah {
		t.Fatalf("#338 field GAP: a non-anchor validator with an empty attester seed reached head %d, "+
			"anchors at %d — it must sync the committed chain via the static (-persistent-peers) tier alone "+
			"(this is 'sybil-1 never synced a committed chain, head height 0')", sh, ah)
	}
}

// TestDivergentQuorumFloorStrandsSyncingNode338 pins the CLOUD root cause of the
// SYBILS=8 C2 GAP: the sybil ran -quorum 5 (a "self-majority") while the anchors
// committed at quorum 2. Config.Quorum is a hard FLOOR on ValidateCommit
// (max(Quorum, bftThreshold)), so when the sybil re-validates the anchors'
// honestly-committed 2-attestation blocks inside Reconcile under its own floor of
// 5, every block fails ErrNoQuorum, the whole fork is rejected, and it is stranded
// at genesis (head 0) forever — even though its transport, static peers, and sync
// targets are all correct. The fix is CONFIGURATION (a uniform quorum floor across
// the objective swarm, topology.py), because in objective mode the quorum is
// bftThreshold over committed bond, not a per-node knob. This test documents the
// mechanism so it is understood, not re-lost: a node whose floor exceeds the
// committed attestation count cannot sync. (The product question — should objective
// mode ignore the local floor when validating committed blocks, since a divergent
// floor breaks the replica-agreement objective mode promises? — is a consensus-rule
// call filed separately, not decided here.)
func TestDivergentQuorumFloorStrandsSyncingNode338(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())

	a1id, a2id := identity.FromSeed(8130), identity.FromSeed(8131)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338c")}}
	chain.Sign(g, a1id.Signer())

	// The anchors commit at a quorum-2 floor (bftThreshold(2 anchors)=... but with a
	// small set the floor governs); the syncing node runs a HIGHER floor of 3.
	anchorCfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	highFloorCfg := anchorCfg
	highFloorCfg.Quorum = 3 // the divergent floor — higher than the anchors' committed attestation count

	mk := func(id *identity.Identity, cfg chain.Config) *Node {
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
	a1, a2 := mk(a1id, anchorCfg), mk(a2id, anchorCfg)
	sidHigh := identity.FromSeed(8132)
	sHigh := mk(sidHigh, highFloorCfg)

	a2.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	sHigh.Bootstrap([]ports.NodeID{a1id.NodeID()}, func() {})
	sched.Run()

	a1.StartChainSync([]ports.NodeID{a2id.NodeID()}, nil)
	a2.StartChainSync([]ports.NodeID{a1id.NodeID()}, nil)
	sHigh.AddStaticPeer(a1id.NodeID())
	sHigh.AddStaticPeer(a2id.NodeID())
	sHigh.StartChainSync(nil, nil)

	sched.RunUntil(sched.Now().Add(a1.cfg.ChainSyncInterval * 8))

	_, ah := a1.Chain().Head()
	if ah < 1 {
		t.Fatalf("setup: anchors never committed (head %d)", ah)
	}
	_, sh := sHigh.Chain().Head()
	// The mechanism: the high-floor node CANNOT sync — its floor rejects the
	// anchors' lower-attestation blocks in Reconcile. This is the documented,
	// reproduced cause of the field GAP; a uniform floor (the fix) is covered by
	// TestNonAttesterSyncsViaStaticPeerOnly338 above.
	if sh == ah {
		t.Fatalf("expected the divergent-floor node to be STRANDED (the #338 cloud mechanism), "+
			"but it synced to head %d — has the objective quorum-floor semantics changed? "+
			"if so, update topology.py and this test's premise", sh)
	}
}
