package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// ── G-H43-8, arm 8e — the gather target survives (the control) ─────────────
//
// TestG_H43_8e_GatherTargetSurvivesTheDerivedFloor is arm 8e, per the
// predicate certification §5 arm 8e: "In the young window with cfg.Quorum
// BELOW the derived bar, proposeBlockAt still gathers RequiredQuorum()
// (assert the gathered attestation count, not the config). And in a uniform
// swarm with cfg.Quorum ABOVE it, committed blocks still carry cfg.Quorum
// attestations. Both halves are GREEN today and must stay GREEN after; this
// is the control the certification's §4.4 'unobservable under uniform
// config' argument rests on."
//
// WHY THIS IS A CONTROL, NOT a RED-first gate: `chainrole.go:836`
// (`proposeBlockAt`'s `if req := n.chain.RequiredQuorum(); req > quorum {
// quorum = req }`) is UNCHANGED by direction (1) — "it should still raise to
// RequiredQuorum(). This is 'the proposer-side gather target' the ratified
// sentence preserves." Both halves below must stay GREEN whether or not the
// predicate change has landed, because neither depends on WHICH regime
// RequiredQuorum() computes from — only on the fact that `proposeBlockAt`
// always raises to it. Without this control, §4.4's claim that "the change
// is a no-op on the graded topology's accept path" (because every
// correctly-configured swarm already gathers to
// `max(cfg.Quorum, RequiredQuorum())`) has no test carrying it, and
// `chainrole.go:836` would read as dead code once the floor is derived.
//
// Half 1 (BELOW the derived bar): `tier2AnchorNet(t, 4)`'s own config —
// `Quorum: 1, ByzantineQuorum: true`, 4 anchors — has `RequiredQuorum() =
// bftThreshold(4) = 2` (regime (a)), above `cfg.Quorum`. A
// proposal offered 3 non-proposer anchors must commit with >= 2
// attestations (the DERIVED bar), not the bare `cfg.Quorum = 1` the caller
// passed as its own gather-target argument.
//
// Half 2 (ABOVE the derived bar): a uniform swarm with `Quorum: 3` (already
// above `bftThreshold(4) = 2`) must still gather exactly 3 attestations — the
// operator's chosen breadth survives in a uniform swarm, which is what keeps
// direction (1) a no-op there. THREE proposal paths reach proposeBlockAt and
// all three must gather the same target (PE ruling on d0067fd, C1): the
// client-publish path passes the caller's -quorum (chainhost.go →
// ProposeEntry, the sub-case "above_derived_bar_still_gathers_cfg.Quorum"),
// while the h43 round/new-view re-proposal (chainrole.go proposeAtNewView)
// and the bond-reg drain (chainrole.go maybeProposeBondDrain) pass 0. With
// the local floor removed from RequiredQuorum(), a raise to RequiredQuorum()
// ALONE made those two paths gather bftThreshold (2) while publish gathered
// 3 — a path-dependent gather target, and an un-upgraded -quorum 3 peer
// (old rule: max(3, 2) = 3) refuses a 2-attestation block: the #338 strand
// via version skew. proposeBlockAt therefore raises to cfg.Quorum as well
// (the operator's floor), so every path gathers max(cfg.Quorum,
// RequiredQuorum()). The two path sub-cases were RED before that raise
// (committed block carried 2, cfg.Quorum 3) and GREEN after.
func TestG_H43_8e_GatherTargetSurvivesTheDerivedFloor(t *testing.T) {
	t.Run("below_derived_bar_gathers_RequiredQuorum_not_cfg.Quorum", func(t *testing.T) {
		nodes, ids, net, g, cfg := tier2AnchorNet(t, 4)
		if got := nodes[0].chain.RequiredQuorum(); got <= cfg.Quorum {
			t.Fatalf("premise: this half needs RequiredQuorum() (%d) STRICTLY ABOVE cfg.Quorum (%d) — "+
				"tier2AnchorNet's config must have changed; re-derive the premise", got, cfg.Quorum)
		}
		attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
		all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

		b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("g-h43-8e-below")}}
		var done bool
		var commitErr error
		// Deliberately pass the BARE cfg.Quorum (1) as the caller's own gather
		// target, exactly as the substrate test (TestModelCheckTier2_RoundCommitsOverHeldDelivery)
		// does, so any pass here demonstrates the RAISE inside proposeBlockAt,
		// not a caller that was already asking for more.
		nodes[0].proposeBlock(b, attesters, all, cfg.Quorum, func(err error) { done, commitErr = true, err })
		drainHeld(t, net, fifo)
		if !done || commitErr != nil {
			t.Fatalf("round must commit over held-delivery: done=%v err=%v", done, commitErr)
		}
		blk := nodes[0].Chain().Blocks(1)[0]
		// Count NON-PROPOSER attestations only — the block also carries the
		// proposer's own self-signature (present in Atts but excluded from
		// `seen` by every quorum check, RequiredQuorum included, chain.go:3178),
		// so a raw len(blk.Atts) over-counts by exactly one.
		got := nonProposerAttCount(&blk)
		if got < nodes[0].chain.RequiredQuorum() {
			t.Fatalf("G-H43-8 arm 8e CONTROL VIOLATION: committed block carries %d non-proposer attestations, "+
				"below RequiredQuorum() = %d — proposeBlockAt's raise (chainrole.go:836) is not doing its job; "+
				"the gather target must never regress to the bare cfg.Quorum (%d)",
				got, nodes[0].chain.RequiredQuorum(), cfg.Quorum)
		}
		t.Logf("G-H43-8 arm 8e half 1 confirmed: committed block carries %d non-proposer attestations (>= RequiredQuorum()=%d, > cfg.Quorum=%d)",
			got, nodes[0].chain.RequiredQuorum(), cfg.Quorum)
	})

	t.Run("above_derived_bar_still_gathers_cfg.Quorum", func(t *testing.T) {
		nodes, ids, net, g, cfg := uniformQuorum3Net(t, 8700, "g-h43-8e-above")
		// Under direction (1) RequiredQuorum() is the derived bftThreshold(4)=2
		// and IGNORES cfg.Quorum (the helper checks it); the caller's own
		// gather-target argument (cfg.Quorum=3) is the binding term here.
		ids0 := ids[0].NodeID()
		attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
		all := []ports.NodeID{ids0, ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

		b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("g-h43-8e-above-blk")}}
		var done bool
		var commitErr error
		nodes[0].proposeBlock(b, attesters, all, cfg.Quorum, func(err error) { done, commitErr = true, err })
		drainHeld(t, net, fifo)
		if !done || commitErr != nil {
			t.Fatalf("round must commit over held-delivery: done=%v err=%v", done, commitErr)
		}
		blk := nodes[0].Chain().Blocks(1)[0]
		got := nonProposerAttCount(&blk)
		if got != cfg.Quorum {
			t.Fatalf("G-H43-8 arm 8e CONTROL VIOLATION: committed block carries %d non-proposer attestations, "+
				"want exactly cfg.Quorum = %d — a uniform swarm's operator-chosen gather breadth must survive "+
				"unchanged (this is the fact §4.4's 'no-op on a uniform config' argument depends on)",
				got, cfg.Quorum)
		}
		t.Logf("G-H43-8 arm 8e half 2 confirmed: committed block carries exactly cfg.Quorum=%d non-proposer attestations", got)
	})

	// The h43 round / new-view re-proposal path (chainrole.go proposeAtNewView →
	// proposeBlockAt with quorum 0): two peers declare round 1, the round-1
	// designee (holding work) assembles the certificate, fires, and gathers.
	t.Run("new_view_path_gathers_cfg.Quorum", func(t *testing.T) {
		nodes, _, net, _, cfg := uniformQuorum3Net(t, 8710, "g-h43-8e-newview")
		_, height := nodes[0].chain.Head()
		designeeID := nodes[0].designatedProposer(height, 1)
		var designee *Node
		var others []*Node
		for _, nd := range nodes {
			if nd.id == designeeID {
				designee = nd
			} else {
				others = append(others, nd)
			}
		}
		if designee == nil || len(others) != 3 {
			t.Fatalf("premise: designatedProposer(%d, 1) is not one of the four anchors", height)
		}
		designee.pendingEntries = []pendingEntry{{E: mkEntry("g-h43-8e-newview-work"), At: height}}
		rawRoundChange1 := func(nd *Node) []byte {
			nd.advanceToRound(nd.roundsFor(), 1, "test")
			raw := nd.roundsFor().Changes[1][nd.id]
			if raw == nil {
				t.Fatalf("premise: %s did not record its own round-change(1) envelope", nd.id)
			}
			return raw
		}
		raw1, raw2 := rawRoundChange1(others[0]), rawRoundChange1(others[1])
		designee.handleChain(others[0].id, ports.Message{Kind: ports.MsgRoundChange, Data: raw1})
		designee.handleChain(others[1].id, ports.Message{Kind: ports.MsgRoundChange, Data: raw2})
		drainHeld(t, net, fifo)
		if _, h := designee.chain.Head(); h <= height {
			t.Fatalf("premise: the new-view proposal never committed (head %d) — the path under test did not run", h)
		}
		blk := designee.Chain().Blocks(1)[0]
		if got := nonProposerAttCount(&blk); got != cfg.Quorum {
			t.Fatalf("G-H43-8 arm 8e CONTROL VIOLATION (new-view path): the round-1 re-proposal committed with %d "+
				"non-proposer attestations, want cfg.Quorum = %d — proposeAtNewView passes quorum 0, so the gather "+
				"target is whatever proposeBlockAt raises it to; it must be max(cfg.Quorum, RequiredQuorum()=%d) on "+
				"EVERY path, or an un-upgraded -quorum %d peer refuses this block (the #338 strand via version skew)",
				got, cfg.Quorum, designee.chain.RequiredQuorum(), cfg.Quorum)
		}
		t.Logf("G-H43-8 arm 8e new-view path confirmed: committed block carries exactly cfg.Quorum=%d non-proposer attestations", cfg.Quorum)
	})

	// The h43 new-view FORCED leg (chainrole.go proposeAtNewView → gatherTwoPhase
	// DIRECTLY with quorum 0, never through proposeBlockAt; research
	// certification §6.12 G-380-A): two peers LOCKED on a block X by a third
	// node declare round 1 carrying that lock, so the round-1 designee's
	// certificate forces X and it RE-PROPOSES the locked value. The lock is a
	// synthetic prepare-QC (author + two peers, verified by VerifyPrepareQC as
	// a premise) adopted through the node's own adoptLock — the same object a
	// real prepare phase would have persisted.
	t.Run("new_view_forced_leg_gathers_cfg.Quorum", func(t *testing.T) {
		nodes, ids, net, g, cfg := uniformQuorum3Net(t, 8730, "g-h43-8e-forced")
		signerOf := map[ports.NodeID]*identity.Identity{}
		for _, id := range ids {
			signerOf[id.NodeID()] = id
		}
		_, height := nodes[0].chain.Head()
		designeeID := nodes[0].designatedProposer(height, 1)
		var designee *Node
		var others []*Node
		for _, nd := range nodes {
			if nd.id == designeeID {
				designee = nd
			} else {
				others = append(others, nd)
			}
		}
		if designee == nil || len(others) != 3 {
			t.Fatalf("premise: designatedProposer(%d, 1) is not one of the four anchors", height)
		}
		author, s1, s2 := others[0], others[1], others[2]

		// X: authored by `author`, with a round-0 prepare-QC from the author + s1 + s2.
		x := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("g-h43-8e-forced-X")}}
		chain.Sign(x, signerOf[author.id].Signer())
		qc := []chain.Attestation{
			chain.AttestAt(x, signerOf[author.id].Signer(), 0, chain.PhasePrepare, ports.Hash{}),
			chain.AttestAt(x, signerOf[s1.id].Signer(), 0, chain.PhasePrepare, ports.Hash{}),
			chain.AttestAt(x, signerOf[s2.id].Signer(), 0, chain.PhasePrepare, ports.Hash{}),
		}
		if err := designee.chain.VerifyPrepareQC(x, qc, 0); err != nil {
			t.Fatalf("premise: the synthetic round-0 prepare-QC for X must verify: %v", err)
		}
		rawX := chain.Encode(x)
		for _, s := range []*Node{s1, s2} {
			if !s.adoptLock(s.roundsFor(), x, 0, qc, rawX) {
				t.Fatalf("premise: %s could not adopt the X lock", s.id)
			}
		}
		rawRoundChange1 := func(nd *Node) []byte {
			nd.advanceToRound(nd.roundsFor(), 1, "test")
			raw := nd.roundsFor().Changes[1][nd.id]
			if raw == nil {
				t.Fatalf("premise: %s did not record its own round-change(1) envelope", nd.id)
			}
			return raw
		}
		raw1, raw2 := rawRoundChange1(s1), rawRoundChange1(s2)
		forced, err := designee.newViewFor(height, 1, [][]byte{raw1, raw2})
		if err != nil || forced == nil || forced.Hash != x.Hash() {
			t.Fatalf("premise: the 2-envelope certificate must FORCE X (forced=%v err=%v) — otherwise the fresh leg runs, not the forced one", forced != nil, err)
		}

		designee.handleChain(s1.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw1})
		designee.handleChain(s2.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw2})
		drainHeld(t, net, fifo)
		if _, h := designee.chain.Head(); h <= height {
			t.Fatalf("premise: the forced re-proposal never committed (head %d) — the path under test did not run", h)
		}
		blk := designee.Chain().Blocks(1)[0]
		if blk.Hash() != x.Hash() {
			t.Fatalf("premise: the committed block is not the LOCKED value X — the fresh leg ran, not the forced one")
		}
		if got := nonProposerAttCount(&blk); got != cfg.Quorum {
			t.Fatalf("G-H43-8 arm 8e CONTROL VIOLATION (new-view FORCED leg, G-380-A): the locked re-proposal committed with %d "+
				"non-proposer attestations, want cfg.Quorum = %d — proposeAtNewView's forced leg calls gatherTwoPhase with "+
				"quorum 0 and never reaches proposeBlockAt; the raise to max(cfg.Quorum, RequiredQuorum()=%d) must live in "+
				"gatherTwoPhase, the choke point every path shares, or an un-upgraded -quorum %d peer refuses this block",
				got, cfg.Quorum, designee.chain.RequiredQuorum(), cfg.Quorum)
		}
		t.Logf("G-H43-8 arm 8e new-view FORCED leg confirmed: the locked value X was re-proposed and committed with exactly cfg.Quorum=%d non-proposer attestations", cfg.Quorum)
	})

	// The bond-reg drain path (chainrole.go maybeProposeBondDrain → proposeBlock
	// with quorum 0): the height's designee holds pending work and sweeps.
	t.Run("bond_drain_path_gathers_cfg.Quorum", func(t *testing.T) {
		nodes, _, net, _, cfg := uniformQuorum3Net(t, 8720, "g-h43-8e-drain")
		_, height := nodes[0].chain.Head()
		designeeID := nodes[0].designatedProposer(height, 0)
		var designee *Node
		for _, nd := range nodes {
			if nd.id == designeeID {
				designee = nd
			}
		}
		if designee == nil {
			t.Fatalf("premise: designatedProposer(%d, 0) is not one of the four anchors", height)
		}
		designee.pendingEntries = []pendingEntry{{E: mkEntry("g-h43-8e-drain-work"), At: height}}
		designee.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		if _, h := designee.chain.Head(); h <= height {
			t.Fatalf("premise: the drain proposal never committed (head %d) — the path under test did not run", h)
		}
		blk := designee.Chain().Blocks(1)[0]
		if got := nonProposerAttCount(&blk); got != cfg.Quorum {
			t.Fatalf("G-H43-8 arm 8e CONTROL VIOLATION (bond-drain path): the drain proposal committed with %d "+
				"non-proposer attestations, want cfg.Quorum = %d — maybeProposeBondDrain passes quorum 0, so the gather "+
				"target is whatever proposeBlockAt raises it to; it must be max(cfg.Quorum, RequiredQuorum()=%d) on "+
				"EVERY path, or an un-upgraded -quorum %d peer refuses this block (the #338 strand via version skew)",
				got, cfg.Quorum, designee.chain.RequiredQuorum(), cfg.Quorum)
		}
		t.Logf("G-H43-8 arm 8e bond-drain path confirmed: committed block carries exactly cfg.Quorum=%d non-proposer attestations", cfg.Quorum)
	})
}

// uniformQuorum3Net is the arm-8e "above the derived bar" world: four live
// anchors on a held-delivery simnet, every node at Quorum: 3 (above
// bftThreshold(4) = 2), each node's sync targets the other three (so the
// round and drain paths' attester set is the other three anchors), each with
// a sign-mark store (the round path persists its lock).
func uniformQuorum3Net(t *testing.T, seedBase int64, genesisTag string) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, chain.Config) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(seedBase + int64(i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry(genesisTag)}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 3, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	nodes := make([]*Node, 4)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	for i, nd := range nodes {
		for j, other := range nodes {
			if i != j {
				nd.chainSyncSeed = append(nd.chainSyncSeed, other.id)
			}
		}
	}
	if got := nodes[0].chain.RequiredQuorum(); got >= cfg.Quorum {
		t.Fatalf("premise: RequiredQuorum() (%d) must be STRICTLY BELOW cfg.Quorum (%d) so cfg.Quorum is the binding gather term", got, cfg.Quorum)
	}
	return nodes, ids, net, g, cfg
}

// nonProposerAttCount counts the DISTINCT non-proposer attesters in a
// committed block's Atts — the same set every quorum predicate
// (RequiredQuorum/SupportMeetsQuorum, chain.go:3176-3182) actually counts.
// The block's raw Atts also carries the proposer's own self-signature, which
// every quorum check explicitly excludes (`id == proposer`), so a caller
// after "how many attestations did quorum need" must exclude it too.
func nonProposerAttCount(b *chain.Block) int {
	proposer := b.ProposerID()
	seen := map[ports.NodeID]bool{}
	for _, a := range b.Atts {
		if id := a.AttesterID(); id != proposer {
			seen[id] = true
		}
	}
	return len(seen)
}
