package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
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
// direction (1) a no-op there.
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
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.DefaultConfig())
		net.EnableHeldDelivery()

		ids := make([]*identity.Identity, 4)
		anchors := map[ports.NodeID]bool{}
		for i := range ids {
			ids[i] = identity.FromSeed(int64(8700 + i))
			anchors[ids[i].NodeID()] = true
		}
		g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-h43-8e-above")}}
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
			nodes[i] = nd
		}
		// Under direction (1) RequiredQuorum() is the derived bftThreshold(4)=2
		// and IGNORES cfg.Quorum; the caller's own gather-target argument
		// (cfg.Quorum=3) must therefore be the binding term inside proposeBlockAt,
		// which only ever RAISES to RequiredQuorum(), never lowers.
		if got := nodes[0].chain.RequiredQuorum(); got >= cfg.Quorum {
			t.Fatalf("premise: this half needs RequiredQuorum() (%d) STRICTLY BELOW cfg.Quorum (%d) so cfg.Quorum "+
				"is the binding gather term; the derived bar for 4 anchors should be bftThreshold(4)=2", got, cfg.Quorum)
		}

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
