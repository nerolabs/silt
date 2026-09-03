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

// =============================================================================
// GATE G2 — the node-tier reply-order reproducer for R-BOX-ATTESTS (verdict §11)
// =============================================================================
//
// THE DEFECT, at the node tier. A proposer populates its committed roots BEFORE it gathers
// (chainrole.go proposeBlock: PopulateEra4Roots → Sign → gather), and pre-carrier apply() wrote
// validatorsSeen from the gathered b.Atts. So any certificate whose first-to-quorum prefix carries
// a qualified attester the chain has never seen made the recomputed root differ from the signed
// one, and the proposer's OWN Append rejected its own block: "commit rejected by own replica"
// (chainrole.go finishPC). At new-view the locked value is re-proposed and can lose again —
// connected, all-honest, unbounded in expectation (converged verdict §2.3(b)).
//
// THE THREE MASKS the reproducer must clear (PE ruling §3, adopted as verdict §4):
//
//	1. REPLY ORDER. finishPC commits at the FIRST satisfying reply and discards later ones, so a
//	   never-seen attester only lands in b.Atts if its reply is inside the winning prefix. Both
//	   orderings are driven below (the fifth node FIRST, then the SAME fixture with it LAST).
//	2. COMMITTED-BOND QUALIFICATION. An att from an unqualified id writes nothing, so the fifth
//	   node must be genuinely qualified. It is genesis-bonded at 2 MiB with MinBond 1 MiB.
//	3. EPOCH-FROZEN QUALIFICATION. In objective mode with epochs enabled AND matureEpoch,
//	   attesterQualifiedAt returns FROZEN epochSet membership, not bonded>=MinBond — a node bonded
//	   mid-epoch is not qualified until the next rotation, and BOTH orderings would be green for
//	   the WRONG reason. THIS FIXTURE STATES ITS EPOCH CONFIGURATION EXPLICITLY: EpochBlocks = 0,
//	   so epochsEnabled() is false and attesterQualifiedAt takes the bonded path. The assertion
//	   below pins it, so a config drift cannot silently re-mask the hazard.

// era4CarrierNet wires nAnchors anchor nodes plus nExtra genesis-bonded NON-anchor nodes onto a
// held-delivery network whose genesis declares era-3 AND era-4 active from height 1, so the very
// first minted block is v5 and the committed-root predicate governs it.
//
// EPOCH CONFIGURATION (mask 3, stated): EpochBlocks = 0 ⇒ epochs DISABLED ⇒ attesterQualifiedAt
// screens bonded >= MinBond || launchAnchor. MatureValidators = 99 ⇒ the network never matures, so
// it stays in the launch/anchor regime for the whole test.
func era4CarrierNet(t *testing.T, nAnchors, nExtra int) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors+nExtra)
	anchors := map[ports.NodeID]bool{}
	var regs []chain.BondReg
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8700 + i))
		if i < nAnchors {
			anchors[ids[i].NodeID()] = true
		}
		regs = append(regs, chain.BondReg{Validator: pubOf(ids[i]), Root: ports.HashBytes(pubOf(ids[i])), Size: 2 << 20})
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-carrier")}, BondRegs: regs}
	chain.Sign(g, ids[0].Signer())

	cfg := chain.Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		MatureValidators:     99,                         // never mature: stay in the launch regime
		EpochBlocks:          0,                          // MASK 3, STATED: epochs DISABLED ⇒ the bonded qualification path
		Era3ActivationHeight: 1, Era4ActivationHeight: 1, // v5 from height 1
	}

	// MASK 3, ASSERTED (not merely commented): epochs disabled ⇒ epochsEnabled() is false ⇒
	// attesterQualifiedAt screens bonded >= MinBond || launchAnchor, NOT frozen epochSet
	// membership. Under the epoch path a mid-epoch-bonded fifth node is unqualified and BOTH
	// reply orderings would be green for the wrong reason (PE ruling §3 / verdict §4).
	if cfg.EpochBlocks != 0 {
		t.Fatal("fixture VACUOUS (mask 3): EpochBlocks must be 0 (epochs disabled)")
	}

	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		if ch.MintVersion(1) != chain.BlockVersionWitnessable {
			t.Fatalf("fixture: height 1 must mint v5, got v%d", ch.MintVersion(1))
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	return nodes, ids, net, g
}

// TestG2_CarrierNodeTierReplyOrder drives all three arms of gate G2.
//
// RED at d7e4df0 (captured in docs/thinking/2026-09-03-lastcommit-carrier-round-A-design.md):
// every arm failed with "propose: commit rejected by own replica: … StateRoot does not equal the
// recomputed post-apply committed state root".
func TestG2_CarrierNodeTierReplyOrder(t *testing.T) {
	// --- Arm (i): a plain v5 round over held delivery. Nobody is seated yet, so EVERY
	// non-proposer precommit in the certificate is a would-be new seat. ---
	t.Run("i/first-v5-round-commits", func(t *testing.T) {
		nodes, ids, net, g := era4CarrierNet(t, 4, 0)
		attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
		all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

		b := &chain.Block{Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("g2-i")}}
		var done bool
		var commitErr error
		nodes[0].proposeBlock(b, attesters, all, 1, func(err error) { done, commitErr = true, err })
		drainHeld(t, net, fifo)

		if !done {
			t.Fatal("gather never completed")
		}
		if commitErr != nil {
			t.Fatalf("G2(i) RED: the proposer's own replica rejected its own v5 block: %v", commitErr)
		}
		if got := nodes[0].chain.Regime().ValidatorsSeen; got != 0 {
			t.Fatalf("the seat lands one block LATE by design: want 0 seen after height 1, got %d", got)
		}
	})

	// --- Arms (ii): a fifth qualified, never-seen node placed FIRST in the attester list
	// (held-delivery FIFO ⇒ its reply lands in the winning prefix), then the SAME fixture with
	// it LAST (its reply is discarded). Both orderings must commit; the fifth node is seated
	// within 2 heights ONLY in the FIRST arm. ---
	//
	// THE DISCRIMINATOR IS THE COUNT, NOT "seen != 0" (MG-1 of the research certification
	// LASTCOMMIT-CARRIER-round-A-5d3fda0-RESEARCH-CERTIFICATION-2026-09-03, §9). Asserting only
	// `seen != 0` is satisfied by the three pre-existing anchors, so it does not witness the new
	// seat at all: a regression that dropped the fifth node from the carrier would still pass.
	// Each arm therefore pins the EXACT carrier membership and the EXACT seated count.
	//
	// THE ARITHMETIC, and why the two arms differ (R-CARRIER-PREFIX-ONLY, cert §5). Quorum stops
	// at the first satisfying reply and finishPC DISCARDS every later one (chainrole.go:1059-1061
	// returns before `pcs = append`), so height 1's certificate is the FIRST-TO-QUORUM PREFIX,
	// not "everything the proposer received":
	//
	//	FIRST arm: replies arrive fifth, id1, id2, id3 ⇒ prefix closes at
	//	           {proposer, fifth, id1, id2} — 4 entries. Height 2's carrier minus the PARENT's
	//	           proposer (= the proposer of height 1, ids[0]) seats 3, the fifth INCLUDED.
	//	LAST arm:  replies arrive id1, id2, id3, fifth ⇒ prefix closes at {proposer, id1, id2} —
	//	           3 entries; the fifth's reply is discarded. The carrier seats 2, and the fifth
	//	           is STRUCTURALLY NOT SEATED at this height.
	//
	// 3 versus 2 is the discriminator. The LAST arm proves the mask control (it still COMMITS —
	// the R-BOX-ATTESTS defect rejected it); it does NOT prove the fifth is seated, and no
	// assertion here may claim it does. The fifth is seated only once it makes a first-to-quorum
	// prefix at some height.
	for _, arm := range []struct {
		name string
		// first places the fifth node first in the attester list (held-delivery FIFO).
		first bool
		// wantCarrierIdx is the EXACT expected membership of height 2's carrier, as indices into
		// ids (4 = the fifth node). Index 0 is height 1's proposer, which the transition excludes.
		wantCarrierIdx []int
		// wantSeen is the EXACT expected validatorsSeen after height 2.
		wantSeen int
		// wantFifthSeated says whether the fifth node's precommit made the winning prefix.
		wantFifthSeated bool
	}{
		{"ii/fifth-node-FIRST", true, []int{0, 4, 1, 2}, 3, true},
		{"ii/fifth-node-LAST", false, []int{0, 1, 2}, 2, false},
	} {
		t.Run(arm.name, func(t *testing.T) {
			nodes, ids, net, g := era4CarrierNet(t, 4, 1)
			fifth := ids[4].NodeID()
			// MASK 2, ASSERTED: every id (the fifth included) carries a committed genesis bond of
			// 2 MiB against MinBond 1 MiB, so the fifth node is genuinely attesterQualified and its
			// att is a REAL would-be seat, not a no-op.
			if got := nodes[0].chain.Regime().Bonded; got != 5 {
				t.Fatalf("fixture VACUOUS (mask 2): want 5 committed bonds, got %d", got)
			}
			var attesters []ports.NodeID
			if arm.first {
				attesters = []ports.NodeID{fifth, ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
			} else {
				attesters = []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID(), fifth}
			}
			all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID(), fifth}

			b := &chain.Block{Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("g2-ii-a")}}
			var done bool
			var commitErr error
			nodes[0].proposeBlock(b, attesters, all, 1, func(err error) { done, commitErr = true, err })
			drainHeld(t, net, fifo)
			if !done {
				t.Fatal("height-1 gather never completed")
			}
			if commitErr != nil {
				t.Fatalf("G2(ii) RED (%s): height 1 rejected by the proposer's own replica: %v", arm.name, commitErr)
			}

			// Height 2 carries height 1's precommits. The seat must land within 2 heights.
			b2 := &chain.Block{Height: 2, Prev: b.Hash(), Entries: []ports.Entry{mkEntry("g2-ii-b")}}
			done, commitErr = false, nil
			nodes[0].proposeBlock(b2, attesters, all, 1, func(err error) { done, commitErr = true, err })
			drainHeld(t, net, fifo)
			if !done {
				t.Fatal("height-2 gather never completed")
			}
			if commitErr != nil {
				t.Fatalf("G2(ii) RED (%s): height 2 (the carrier block) rejected: %v", arm.name, commitErr)
			}

			// ---- MG-1: the exact carrier membership. ----
			carried := map[ports.NodeID]bool{}
			for i := range b2.LastCommit {
				carried[b2.LastCommit[i].AttesterID()] = true
			}
			if len(carried) != len(b2.LastCommit) {
				t.Fatalf("G2(ii) (%s): carrier has duplicate attester ids: %d entries, %d distinct",
					arm.name, len(b2.LastCommit), len(carried))
			}
			want := map[ports.NodeID]bool{}
			for _, i := range arm.wantCarrierIdx {
				want[ids[i].NodeID()] = true
			}
			if len(carried) != len(want) {
				t.Fatalf("G2(ii) (%s): want a %d-entry carrier (the first-to-quorum prefix), got %d",
					arm.name, len(want), len(carried))
			}
			for id := range want {
				if !carried[id] {
					t.Fatalf("G2(ii) (%s): carrier is missing an expected signer %s", arm.name, id)
				}
			}
			if carried[fifth] != arm.wantFifthSeated {
				t.Fatalf("G2(ii) (%s): fifth node in the carrier = %v, want %v", arm.name, carried[fifth], arm.wantFifthSeated)
			}

			// ---- MG-1: the exact seated COUNT, plus the membership it implies. ----
			// Every id is qualified (mask 2, asserted above) and the ONLY exclusion applyCarrier
			// makes is the PARENT's proposer, so the seated set is exactly `carried` minus ids[0].
			// Deriving it here and requiring it to equal the arm's hard-coded wantSeen means a
			// drift in EITHER the carrier or the transition goes RED — neither can absorb the other.
			parentProposer := ids[0].NodeID()
			derived := 0
			for id := range carried {
				if id != parentProposer {
					derived++
				}
			}
			if derived != arm.wantSeen {
				t.Fatalf("G2(ii) (%s): the carrier minus the parent proposer is %d ids, want %d — the fixture drifted",
					arm.name, derived, arm.wantSeen)
			}
			seen := nodes[0].chain.Regime().ValidatorsSeen
			if seen != arm.wantSeen {
				t.Fatalf("G2(ii) (%s): validatorsSeen = %d after height 2, want EXACTLY %d "+
					"(carrier %v minus the parent's proposer)", arm.name, seen, arm.wantSeen, arm.wantCarrierIdx)
			}
			t.Logf("%s: committed heights 1-2, carrier=%d entries, fifth carried=%v, validatorsSeen=%d",
				arm.name, len(b2.LastCommit), carried[fifth], seen)
		})
	}
}
