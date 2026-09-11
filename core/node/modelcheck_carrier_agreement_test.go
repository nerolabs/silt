package node

import (
	"fmt"
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
// Consensus model-check — CARRIER SEATING AGREEMENT AT THE NODE TIER
// (R-CARRIER-MODELCHECK, freeze-manifest item 14; tier 2)
// =============================================================================
//
// Tier 1 (core/chain/modelcheck_carrier_seating_test.go) proves the seating REDUCER over
// modelled replicas: cloneForDryRun + apply. This file drives SEVERAL REAL *Node replicas
// through the real propose -> gather -> commit -> broadcast loop over held delivery, where
// the driver chooses the message order, and asks the question a replica actually faces:
// after the same committed history, does every replica accept the same block?
//
// WHY ACCEPTANCE IS THE WITNESS, AND WHY IT IS STRONGER THAN A COUNT. validatorsSeenRoot is
// a committed v5 state leaf (core/chain/statehash.go, tagValidatorsSeenRoot) and the era-4
// root predicate re-runs the transition on every replica before it appends. A replica that
// seated a DIFFERENT set therefore recomputes a different state root and REFUSES the block.
// So "every replica's head is the same hash at the same height" is a cryptographic statement
// about the seated SET, not a head count that two different sets of the same size would pass.
//
// THE POSITIVE CONTROL IS THE POINT. An all-honest agreement assertion is satisfied by a
// build where nothing is seated at all, and by one where the oracle cannot see a split. The
// control arm below injects a replica whose ONLY difference is its launch-anchor set — which
// changes attesterQualified for one unbonded anchor and NOTHING committed at genesis — and
// requires that replica to FALL BEHIND. That is the demonstrated red for this file: it proves
// the agreement arm is capable of failing.
//
// EPOCH CONFIGURATION, STATED (the mask-3 discipline of lastcommit_carrier_node_test.go):
// EpochBlocks = 0, so epochsEnabled() is false and attesterQualifiedAt screens
// bonded >= MinBond || launchAnchor. MatureValidators = 99, so the network never matures and
// launchAnchor stays live for the whole run. Both are asserted, not merely commented.
//
// BUILT AGAINST origin/main a28a5b5, and it edits nothing: a FORMAT branch holds
// core/node/chainrole.go and core/node/rounds.go. This file calls proposeBlock and reads
// Chain().Regime() — symbols, not coordinates. If that branch changes the gather's
// first-to-quorum prefix, the CARRIER MEMBERSHIP below moves and the agreement property does
// not: replicas must still agree on whatever prefix the proposer actually carried.

// carrierAgreeNet wires nAnchors UNBONDED launch anchors plus nBonded genesis-bonded
// non-anchor nodes onto a held-delivery network sharing one genesis, v5 from height 1.
//
// dropAnchor >= 0 makes replica index `dissenter` run with anchor `dropAnchor` MISSING from
// its launch-anchor set. That replica's committed genesis state is IDENTICAL to everyone
// else's — an anchor holds no bond, so it appears in no state leaf — while its
// attesterQualified answer for that one id is FALSE. It is a pure SEATING divergence.
func carrierAgreeNet(t *testing.T, nAnchors, nBonded, dissenter, dropAnchor int) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors+nBonded)
	anchors := map[ports.NodeID]bool{}
	var regs []chain.BondReg
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8900 + i))
		if i < nAnchors {
			anchors[ids[i].NodeID()] = true
			continue // an anchor holds NO bond: its qualification route is launchAnchor only
		}
		// Non-uniform bonds: a weight-blind or count-blind reducer cannot hide behind a
		// uniform population.
		regs = append(regs, chain.BondReg{Validator: pubOf(ids[i]),
			Root: ports.HashBytes(pubOf(ids[i])), Size: int64(2+i) << 20})
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-carrier-agree")}, BondRegs: regs}
	chain.Sign(g, ids[0].Signer())

	base := chain.Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		// AnchorQuorum 2, deliberately: the control arm below runs a replica that does not
		// recognise ONE of the three anchors in the winning prefix. At AnchorQuorum 3 that
		// replica would refuse HEIGHT 1 on the anchor-quorum rule and the control would prove
		// only that two differently-configured chains disagree. At 2 it commits height 1 with
		// everyone and can diverge ONLY at height 2, where the carrier is.
		AnchorQuorum:         2,
		MatureValidators:     99, // never mature: launchAnchor stays live
		EpochBlocks:          0,  // MASK 3, STATED: epochs DISABLED => the bonded || launchAnchor branch
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}
	if base.EpochBlocks != 0 || base.MatureValidators == 0 {
		t.Fatal("fixture VACUOUS (mask 3): epochs must be DISABLED and the network must stay immature, " +
			"or attesterQualifiedAt screens frozen epochSet membership and the anchor route is never taken")
	}

	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		cfg := base
		if i == dissenter && dropAnchor >= 0 {
			drop := map[ports.NodeID]bool{}
			for a, v := range anchors {
				drop[a] = v
			}
			delete(drop, ids[dropAnchor].NodeID())
			cfg.Anchors = drop
		}
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis on replica %d: %v", i, err)
		}
		if ch.MintVersion(1) != chain.BlockVersionWitnessable {
			t.Fatalf("fixture: height 1 must mint v5 on replica %d, got v%d", i, ch.MintVersion(1))
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	return nodes, ids, net, g
}

func lifo(p []simnet.HeldMsg) int { return len(p) - 1 }

// driveCarrierHeights commits heights 1 and 2 through the real loop, height h proposed by
// nodes[proposers[h-1]], with the driver delivering messages via pick. Height 2 is the block
// that carries height 1's precommits, so it is the one whose acceptance witnesses the seating.
func driveCarrierHeights(t *testing.T, nodes []*Node, ids []*identity.Identity, net *simnet.Network,
	g *chain.Block, attesterOrder []int, proposers [2]int, pick func([]simnet.HeldMsg) int) []*chain.Block {
	t.Helper()
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	var out []*chain.Block
	prev := g.Hash()
	for h := uint64(1); h <= 2; h++ {
		p := proposers[h-1]
		var attesters []ports.NodeID
		for _, i := range attesterOrder {
			if i != p {
				attesters = append(attesters, ids[i].NodeID())
			}
		}
		// A distinct entry per height: two blocks publishing the same root are refused by the
		// local pre-check before any consensus rule is reached.
		b := &chain.Block{Height: h, Prev: prev,
			Entries: []ports.Entry{mkEntry(fmt.Sprintf("carrier-agree-h%d-p%d", h, p))}}
		var done bool
		var commitErr error
		nodes[p].proposeBlock(b, attesters, all, 1, func(err error) { done, commitErr = true, err })
		drainHeld(t, net, pick)
		if !done {
			t.Fatalf("height %d: the gather never completed", h)
		}
		if commitErr != nil {
			t.Fatalf("height %d: the proposer's own replica rejected its own block: %v", h, commitErr)
		}
		prev = b.Hash()
		out = append(out, b)
	}
	return out
}

// TestModelCheck_CarrierAgreement_HonestReplicasSeatTheSameSet is the agreement oracle over
// the real loop. Delivery order and the proposer rotation are varied; both must be irrelevant.
func TestModelCheck_CarrierAgreement_HonestReplicasSeatTheSameSet(t *testing.T) {
	for _, arm := range []struct {
		name      string
		order     []int
		proposers [2]int
		pick      func([]simnet.HeldMsg) int
	}{
		{"fifo/proposer-0-then-0", []int{1, 2, 3, 4, 5}, [2]int{0, 0}, fifo},
		{"lifo/proposer-0-then-0", []int{1, 2, 3, 4, 5}, [2]int{0, 0}, lifo},
		{"fifo/reversed-attester-order", []int{5, 4, 3, 2, 1}, [2]int{0, 0}, fifo},
		{"fifo/proposer-0-then-1", []int{1, 2, 3, 4, 5}, [2]int{0, 1}, fifo},
		{"lifo/proposer-2-then-3", []int{1, 2, 3, 4, 5}, [2]int{2, 3}, lifo},
	} {
		t.Run(arm.name, func(t *testing.T) {
			nodes, ids, net, g := carrierAgreeNet(t, 4, 2, -1, -1)
			h2 := driveCarrierHeights(t, nodes, ids, net, g, arm.order, arm.proposers, arm.pick)[1]

			// The carrier must be NON-EMPTY, or "every replica agreed" is a statement about
			// nothing: an empty carrier seats nobody on every replica for free.
			if len(h2.LastCommit) == 0 {
				t.Fatal("VACUOUS: the height-2 block carries an EMPTY carrier, so no replica seated anything " +
					"and the agreement below has no content")
			}
			seen0 := nodes[0].Chain().Regime().ValidatorsSeen
			if seen0 == 0 {
				t.Fatalf("VACUOUS: nothing was seated after height 2 (carrier had %d entries) — "+
					"agreement on the empty set holds for every reducer", len(h2.LastCommit))
			}

			wantHash := h2.Hash()
			for i, nd := range nodes {
				hh, h := nd.Chain().Head()
				if h != 3 || hh != wantHash {
					t.Fatalf("replica %d did not accept the carrier block every other replica accepted: "+
						"head height=%d hash=%x, want height 3 hash %x — a replica that seated a DIFFERENT "+
						"set recomputes a different validatorsSeenRoot and refuses the block",
						i, h, hh[:8], wantHash[:8])
				}
				if got := nd.Chain().Regime().ValidatorsSeen; got != seen0 {
					t.Fatalf("replica %d seated %d identities, replica 0 seated %d", i, got, seen0)
				}
			}
			t.Logf("%s: carrier %d entries, every one of %d replicas committed height 2, validatorsSeen=%d",
				arm.name, len(h2.LastCommit), len(nodes), seen0)
		})
	}
}

// TestModelCheck_CarrierAgreement_ControlADivergentScreenFallsBehind is the DEMONSTRATED RED
// for the oracle above, injected without touching production code.
//
// The dissenting replica differs in exactly one way: one unbonded launch anchor is missing
// from its Anchors set. Nothing committed changes — an unbonded anchor appears in no state
// leaf, so every replica's genesis root is identical — but attesterQualified answers FALSE for
// that id, so when the height-2 carrier carries that anchor's precommit the dissenter seats
// one fewer identity, recomputes a different validatorsSeenRoot, and REFUSES the block.
//
// If this arm ever goes green, the agreement oracle above is decoration: it would mean a
// replica can seat a different set and still accept the block.
func TestModelCheck_CarrierAgreement_ControlADivergentScreenFallsBehind(t *testing.T) {
	const dissenter, dropped = 5, 2 // replica 5 (a bonded non-anchor) does not recognise anchor 2
	nodes, ids, net, g := carrierAgreeNet(t, 4, 2, dissenter, dropped)

	// The dissenter's committed genesis must be IDENTICAL, or the control proves only that two
	// differently-configured chains disagree — which would say nothing about SEATING. An
	// unbonded anchor appears in no state leaf, which is exactly why this knob is invisible here.
	h0Ref, _ := nodes[0].Chain().Head()
	h0Dis, _ := nodes[dissenter].Chain().Head()
	if h0Ref != h0Dis {
		t.Fatalf("control setup: the dissenter's genesis head differs (%x vs %x) — the injected difference "+
			"must be invisible in committed state", h0Dis[:8], h0Ref[:8])
	}
	if nodes[0].Chain().Regime().Bonded != nodes[dissenter].Chain().Regime().Bonded {
		t.Fatal("control setup: the dissenter's committed bonded set differs — the injected difference is not " +
			"confined to the launch-anchor screen")
	}

	// LIFO, and the reason is MEASURED, not stylistic. Under fifo the first-to-quorum prefix is
	// three entries, so a replica that does not recognise one of them is left with ONE qualified
	// attestation against a bar of two and refuses HEIGHT 1 — "prepare-QC: insufficient valid
	// attestations: 1 qualified, need 2". That is a quorum divergence, not a seating one, and the
	// confound guard below caught it on the first run. Under lifo the prefix is five entries, so
	// the dissenter still clears the height-1 bar and can only diverge where the carrier is.
	blocks := driveCarrierHeights(t, nodes, ids, net, g,
		[]int{1, 2, 3, 4, 5}, [2]int{0, 0}, lifo)
	h1, h2 := blocks[0], blocks[1]

	// HEIGHT 1 IS THE CONFOUND CHECK. Its carrier is EMPTY BY RULE, so it seats nothing and the
	// dissenter's screen cannot matter there. If the dissenter fell behind at height 1, the
	// divergence is an anchor-quorum or validity effect and this arm would be attributing it to
	// the carrier — so that is a fixture failure, not a pass.
	// Asked as "does the dissenter's chain CONTAIN height 1", not as "is its head height 2":
	// both heights have already been driven, so a dissenter that agreed throughout is at height
	// 3 and a head-height equality check would misreport that as a confound.
	if bs := nodes[dissenter].Chain().Blocks(1); len(bs) == 0 || bs[0].Hash() != h1.Hash() {
		_, h := nodes[dissenter].Chain().Head()
		t.Fatalf("CONTROL CONFOUNDED: the dissenter never committed HEIGHT 1, whose carrier is empty by "+
			"rule (head height=%d). The injected difference is not isolated to the seating screen; fix "+
			"the fixture rather than reading this as evidence about the carrier", h)
	}
	if len(h1.LastCommit) != 0 {
		t.Fatalf("height 1 must carry an EMPTY carrier by rule, got %d entries", len(h1.LastCommit))
	}

	// The dropped anchor MUST be inside height 2's carrier, or the dissenter's screen was never
	// consulted and this arm proves nothing.
	carriedDropped := false
	for i := range h2.LastCommit {
		if h2.LastCommit[i].AttesterID() == ids[dropped].NodeID() {
			carriedDropped = true
		}
	}
	if !carriedDropped {
		t.Fatalf("CONTROL UNDRIVEN: the height-2 carrier (%d entries) does not carry the anchor the dissenter "+
			"does not recognise, so its screen was never consulted and this arm proves nothing", len(h2.LastCommit))
	}

	wantHash := h2.Hash()
	for i, nd := range nodes {
		if i == dissenter {
			continue
		}
		hh, h := nd.Chain().Head()
		if h != 3 || hh != wantHash {
			t.Fatalf("honest replica %d must still commit height 2: head height=%d hash=%x", i, h, hh[:8])
		}
	}
	hh, h := nodes[dissenter].Chain().Head()
	if h == 3 && hh == wantHash {
		t.Fatalf("CONTROL GREEN — THE ORACLE IS DECORATION: a replica whose attesterQualified answers "+
			"differently for a CARRIED id accepted the same block anyway (head height %d). Either the "+
			"committed root does not cover the seated set, or the root predicate is not re-running the "+
			"transition on the accepting replica.", h)
	}
	t.Logf("control: agreed at height 1 (empty carrier), diverged at height 2 (carrier %d entries) — the "+
		"dissenter stalled at head height %d while honest replicas reached 3", len(h2.LastCommit), h)
}
