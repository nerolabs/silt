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
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338")}}
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
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338b")}}
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
// committed at quorum 2. Config.Quorum WAS a hard FLOOR on ValidateCommit
// (max(Quorum, bftThreshold)) in every objective regime until #380 direction
// (1); it remains one ONLY in regime (c), this fixture's, so when the sybil re-validates the anchors'
// honestly-committed 2-attestation blocks inside Reconcile under its own floor of
// 5, every block fails ErrNoQuorum, the whole fork is rejected, and it is stranded
// at genesis (head 0) forever — even though its transport, static peers, and sync
// targets are all correct. The fix is CONFIGURATION (a uniform quorum floor across
// the objective swarm, topology.py), because in objective mode the quorum is
// bftThreshold over committed bond, not a per-node knob.
//
// REGIME (c), and why this test now STAYS this way (Researcher certification
// `CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`
// §1 row (c), §5 arm 8a): this fixture's `anchorCfg`/`highFloorCfg` never set
// `ByzantineQuorum`, so it defaults false — the EXPLICIT trusted opt-out
// regime, where `RequiredQuorum()` stays `cfg.Quorum` VERBATIM even after
// direction (1) ships ("no derived rule to defer to... a `-byzantine-quorum
// =false` operator has declared a trusted swarm and owns its own config
// uniformity"). This test is therefore the REGIME-(c) CONTROL: it documents
// that `max(Quorum, bftThreshold)` — now more precisely, the BARE `cfg.Quorum`
// floor with no `bftThreshold` involvement at all — governs ONLY in this
// opt-out regime, and a divergent local floor there is expected, permanent
// stranding, both before AND after the fix lands. (The product question the
// old version of this comment left open — "should objective mode ignore the
// local floor when validating committed blocks?" — is answered for the
// DEFAULT `ByzantineQuorum: true` regimes by owner call 20
// (`docs/decisions.md` D-CONSENSUS-ARMING (20)); the regime-(a) analogue of
// THIS fixture, where the answer is "must sync", is
// TestG_H43_8a_DivergentQuorumFloorSyncsUnderDerivedFloor below.)
func TestDivergentQuorumFloorStrandsSyncingNode338(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())

	a1id, a2id := identity.FromSeed(8130), identity.FromSeed(8131)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-338c")}}
	chain.Sign(g, a1id.Signer())

	// REGIME (c): ByzantineQuorum is deliberately left unset (false) — the anchors
	// commit at a bare quorum-1 floor; the syncing node runs a HIGHER bare floor of 3.
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
	// reproduced cause of the field GAP; in this REGIME-(c) opt-out config a
	// uniform floor (the operator's own responsibility, topology.py) is the
	// only fix — direction (1) deliberately does not touch this regime.
	if sh == ah {
		t.Fatalf("expected the divergent-floor node to be STRANDED (the #338 cloud mechanism, REGIME (c)), "+
			"but it synced to head %d — has the regime-(c) opt-out quorum-floor semantics changed? "+
			"if so, update topology.py and this test's premise", sh)
	}
}

// TestG_H43_8a_DivergentQuorumFloorSyncsUnderDerivedFloor is G-H43-8's
// CORRECTED arm 8a, per the Researcher's predicate certification
// `CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`
// §5 arm 8a: TestDivergentQuorumFloorStrandsSyncingNode338 above (the ORIGINAL
// #338 fixture) never sets `ByzantineQuorum`, so it exercises REGIME (c) — the
// explicit trusted opt-out the predicate change does NOT touch — and inverting
// its assertion alone would be VACUOUS (the fixture would strand under the
// fixed code exactly as it does today, for the unrelated reason that regime
// (c) has no derived rule to defer to). This test corrects that: both configs
// set `ByzantineQuorum: true` — "also the faithful field posture, since it
// defaults ON for the untrusted objective path" (`daemon.go:2169-2177`) — which
// reaches REGIME (a), §1's row: `!handedOff()` (`MatureValidators: 99`) makes
// `N = len(Anchors) = 2`, so `bftThreshold(2) = 1`; the anchors' 1-attestation
// blocks clear the DERIVED bar, and `requiredLaunchAnchors = ⌊2/2⌋+1 = 2` is
// met by proposer-if-anchor plus one anchor attester (`chain.go:1798-1806`).
// Under the CERTIFIED fix, `RequiredQuorum()` in this regime becomes
// `bftThreshold(validatorSetSize())` UNCONDITIONALLY — the `max` with the
// local `cfg.Quorum` disappears — so the divergent-floor node's local
// `Quorum: 3` no longer raises its own validity bar above what the anchors'
// blocks already clear, and it must sync.
//
// RED-FIRST at e443548: `RequiredQuorum()` still computed
// `max(c.cfg.Quorum, bftThreshold(N))` in this regime — the OLD predicate,
// the certification's "Today" column — so the high-floor node's own
// `RequiredQuorum() = max(3, 1) = 3` exceeded the 1-attestation blocks the
// anchors (`RequiredQuorum() = max(1, 1) = 1`) actually produce,
// `ValidateCommit`/`Reconcile` refused with `ErrNoQuorum`, and the node
// stayed stranded — the SAME mechanism as the regime-(c) control above,
// reached through the regime the fix changes. GREEN under direction (1):
// both sides compute bftThreshold(2) = 1. Ablation: restore the max ⇒ RED.
func TestG_H43_8a_DivergentQuorumFloorSyncsUnderDerivedFloor(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())

	a1id, a2id := identity.FromSeed(8140), identity.FromSeed(8141)
	anchors := map[ports.NodeID]bool{a1id.NodeID(): true, a2id.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-h43-8a")}}
	chain.Sign(g, a1id.Signer())

	// REGIME (a): ByzantineQuorum: true on BOTH configs (the arm-8a fixture
	// correction) — bftThreshold(2 anchors) = 1 governs once the fix lands.
	anchorCfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	highFloorCfg := anchorCfg
	highFloorCfg.Quorum = 3 // the divergent floor — irrelevant once RequiredQuorum() = bftThreshold(N) alone

	mk := func(id *identity.Identity, cfg chain.Config) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	a1, a2 := mk(a1id, anchorCfg), mk(a2id, anchorCfg)
	sidHigh := identity.FromSeed(8142)
	sHigh := mk(sidHigh, highFloorCfg)

	// Premise: both anchor-side chains agree the derived floor in this regime
	// is 1 (bftThreshold(2)), NOT the bare cfg.Quorum — verified against the
	// real function, not hand arithmetic, so a drift in bftThreshold's own
	// formula would fail HERE rather than silently invalidating the fixture.
	if got := a1.Chain().RequiredQuorum(); got != 1 {
		t.Fatalf("premise: the anchors' RequiredQuorum() is bftThreshold(2)=1 (regime (a)), got %d", got)
	}

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
	// G-H43-8 arm 8a (owner call 20): in regime (a), a divergent local
	// -quorum floor must not stop a node from syncing the anchors'
	// derived-bar-clearing blocks. RED-FIRST at e443548: RequiredQuorum() took
	// the max with the local floor, so the high-floor node's own bar (3)
	// exceeded what the anchors' blocks actually carry (1), and Reconcile
	// refused every one of them with ErrNoQuorum.
	if sh != ah {
		t.Fatalf("G-H43-8 arm 8a REGRESSED: the divergent-floor node did NOT sync in regime (a) "+
			"(head %d, anchors at %d) — its own raised local -quorum floor (RequiredQuorum()=max(3,"+
			"bftThreshold(2)=1)=3) still exceeds the anchors' derived-bar-clearing 1-attestation blocks, "+
			"so Reconcile refuses them with ErrNoQuorum and the node stays stranded. RequiredQuorum() must "+
			"drop the max with the local floor in this regime (predicate certification §1 row (a)) before "+
			"this can pass", sh, ah)
	}
}
