package node

import (
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// A PROVEN EQUIVOCATION MUST ARM A PROPOSAL, or the replicated eviction waits on
// traffic that has nothing to do with it.
//
// slashEquivocators queues the proof so "the OBJECTIVE set evicts the culprit in
// lockstep on every replica (F2), not just this local ledger". But the proof rides
// only a block someone proposes, and the drain sweep's quiescence rule counted
// pending bond registrations, pending entries, an own renewal due and foldable
// issuer keys as work — never a queued slash. On a busy chain an unrelated renewal
// carried it along soon enough to hide that. On an idle one it never landed.
//
// IDLE IS THE CASE THAT MATTERS: an attacker equivocates and then goes quiet, which
// is exactly the chain with no other work to arm the sweep. Measured on a
// three-anchor topology over real containers 2026-09-19 — both honest nodes
// committed, slashed the culprit, then committed nothing for the rest of the
// window, with no sign-slot block, no round advance and no stall line. Not wedged;
// quiescent, holding an eviction it had already proven.
func TestQueuedSlashArmsTheDrainOnAnIdleChain(t *testing.T) {
	nodes, ids, net, _, _ := matureWorld12(t)

	// Find the seat that owns the next height, so the arm under test is the
	// designee's own sweep and not the rank-walk takeover backstop (which has its
	// own timer and would make a pass mean something else).
	_, h := nodes[0].chain.Head()
	var designee *Node
	for i, nd := range nodes {
		if ids[i].NodeID() == nodes[0].designatedProposer(h, 0) {
			designee = nd
			break
		}
	}
	if designee == nil {
		t.Fatal("setup: could not resolve the designee for the next height")
	}

	// THE IDLE PREMISE, ASSERTED NOT ASSUMED. With every queue empty the sweep must
	// send nothing — otherwise the arm below would pass on a chain that was never
	// quiet and this test would prove nothing about slashes.
	designee.pendingBondRegs = nil
	designee.pendingEntries = nil
	designee.pendingSlashes = nil
	before := net.Stats.Kinds[ports.MsgProposeBlock]
	designee.maybeProposeBondDrain()
	if idle := net.Stats.Kinds[ports.MsgProposeBlock] - before; idle != 0 {
		t.Fatalf("PREMISE BROKEN: the designee proposed %d block(s) with every queue empty, so this "+
			"chain is not idle and the arm below cannot attribute a proposal to the slash", idle)
	}

	// Queue a proven equivocation the way slashEquivocators does, and nothing else.
	// It is built by the product's OWN detector over two forks a third validator
	// attested on both sides — a fabricated pair does not survive the drain's
	// admissibility re-check (a bare proposer signature is authorship, not a
	// consensus vote, so it is not evidence), and a proof the fold drops would leave
	// an empty block and make this test fail for a reason that is not the rule.
	chainBlocks := designee.chain.Blocks(0)
	if len(chainBlocks) == 0 {
		t.Fatal("setup: no genesis block to fork from")
	}
	gen := &chainBlocks[0]
	culprit := ids[len(ids)-1]
	fork := func(name string) chain.Block {
		b := chain.Block{Version: chain.BlockVersion, Height: 1, Prev: gen.Hash(),
			Entries: []ports.Entry{mkEntry(name)}}
		chain.Sign(&b, ids[0].Signer())
		// ERA 2 IS THE RULE HERE, and it is stricter than era 1 on purpose: the crime
		// is releasing two CONSENSUS signatures at one (height, round, phase) over
		// different hashes. A bare proposer signature is authorship and a cross-round
		// signature is an honest lock change, so neither is evidence — which is why
		// this attests at a fixed (round, phase) under the node's own chain id.
		b.Atts = append(b.Atts, chain.AttestAt(&b, culprit.Signer(), 0, chain.PhasePrepare, designee.chainID()))
		return b
	}
	evs := chain.FindEquivocations([]chain.Block{*gen, fork("A")}, []chain.Block{*gen, fork("B")},
		designee.chainID(), designee.eraFloor())
	if len(evs) == 0 {
		t.Fatal("setup: the product's own detector found no equivocation in the fixture forks")
	}
	designee.pendingSlashes = []chain.Equivocation{evs[0]}

	// NO refill() HERE, DELIBERATELY. matureWorld12 hands every node a pending bond
	// registration and refill() puts it back; either would arm the drain on its own
	// and the arm below would pass with the fix reverted. That is not a hypothetical
	// — the first cut of this test called refill() here and was VACUOUS: it passed
	// under the ablation. The slash must be the only work on the queue.
	before = net.Stats.Kinds[ports.MsgProposeBlock]
	designee.maybeProposeBondDrain()
	if got := net.Stats.Kinds[ports.MsgProposeBlock] - before; got == 0 {
		t.Fatal("a queued equivocation proof did NOT arm a proposal on an otherwise idle chain. " +
			"The local ledger has evicted the culprit and the committed history has not, so replicas do " +
			"not evict in lockstep (F2) — and an attacker that equivocates and then goes quiet produces " +
			"exactly this chain. The quiescence rule in maybeProposeBondDrain must count pendingSlashes " +
			"as designee work alongside pendingBondRegs and pendingEntries.")
	}
}

// THE TAKEOVER HALF. The arm above drives the seat that OWNS the height, so it
// never reaches the rank-walk branch — and that branch has its own copy of the
// quiescence rule. A non-designee holding the only proof of an equivocation must
// still get the eviction onto the chain when the designee stays silent, or the
// accountability claim rests on one seat being alive.
//
// Ablating the takeover branch alone leaves the arm above GREEN, which is why this
// exists as a separate test rather than more assertions in the same one.
func TestQueuedSlashIsTakeoverWorthyWhenTheDesigneeIsSilent(t *testing.T) {
	nodes, ids, net, _, _ := matureWorld12(t)

	_, h := nodes[0].chain.Head()
	designeeID := nodes[0].designatedProposer(h, 0)
	var holder *Node
	for i, nd := range nodes {
		if ids[i].NodeID() != designeeID && nd.chain.ProposerEligible(nd.id) {
			holder = nd
			break
		}
	}
	if holder == nil {
		t.Fatal("setup: could not find an eligible NON-designee to hold the proof")
	}

	// Every seat idle: the designee stays silent by simply never sweeping, and the
	// holder carries the proof and nothing else.
	for _, nd := range nodes {
		nd.pendingBondRegs = nil
		nd.pendingEntries = nil
		nd.pendingSlashes = nil
	}

	chainBlocks := holder.chain.Blocks(0)
	if len(chainBlocks) == 0 {
		t.Fatal("setup: no genesis block to fork from")
	}
	gen := &chainBlocks[0]
	culprit := ids[len(ids)-1]
	fork := func(name string) chain.Block {
		b := chain.Block{Version: chain.BlockVersion, Height: 1, Prev: gen.Hash(),
			Entries: []ports.Entry{mkEntry(name)}}
		chain.Sign(&b, ids[0].Signer())
		b.Atts = append(b.Atts, chain.AttestAt(&b, culprit.Signer(), 0, chain.PhasePrepare, holder.chainID()))
		return b
	}
	evs := chain.FindEquivocations([]chain.Block{*gen, fork("A")}, []chain.Block{*gen, fork("B")},
		holder.chainID(), holder.eraFloor())
	if len(evs) == 0 {
		t.Fatal("setup: the product's own detector found no equivocation in the fixture forks")
	}
	holder.pendingSlashes = []chain.Equivocation{evs[0]}

	// Sweep past the rank-walk wait window (3+dist sweeps, dist bounded by the set
	// size). A bounded loop, not an unbounded one: if the takeover never fires, the
	// assertion below is what reports it rather than the test hanging.
	before := net.Stats.Kinds[ports.MsgProposeBlock]
	for i := 0; i < 3+len(nodes)+2; i++ {
		holder.maybeProposeBondDrain()
		if net.Stats.Kinds[ports.MsgProposeBlock]-before > 0 {
			return // took over and carried the proof
		}
	}
	t.Fatalf("a non-designee held the ONLY proof of an equivocation through %d sweeps and never took "+
		"over the silent designee. The eviction reaches the chain only if the designated seat happens "+
		"to be alive and proposing, so accountability rests on one seat — the rank-walk branch in "+
		"maybeProposeBondDrain must count pendingSlashes as takeover-worthy work alongside "+
		"pendingEntries.", 3+len(nodes)+2)
}
