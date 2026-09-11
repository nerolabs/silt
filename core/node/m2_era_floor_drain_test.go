package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// testEraFloor is the node tier's only EraFloor supplier, and like core/chain's it reads the floor
// off a real chain's (*Chain).EraFloor rather than writing a closure by hand. A hand-written
// supplier could express a floor no genesis produces — including uint64(0), the fail-open value
// CheckEquivocation's nil check exists to refuse. at = 0 is the latch route with nothing latched
// (v2 at every height); at = 1 is the RC shape (v5 above the genesis).
func testEraFloor(at uint64) chain.EraFloor {
	return chain.New(chain.Config{Quorum: 1, Era3ActivationHeight: at, Era4ActivationHeight: at},
		func(ports.NodeID) int64 { return 0 }).EraFloor()
}

// ---------------------------------------------------------------------------
// G-EF-7 — THE PROPOSER DRAIN RE-CHECKS ADMISSIBILITY.
// ---------------------------------------------------------------------------
//
// Detection and drain are separated in time. slashEquivocators queues a proof; proposeBlock
// embeds it some blocks later, re-checking only IsSlashed and the per-proof byte cap. On the
// LATCH route the era floor at a FIXED height RISES when era4LockedIn latches, so a proof that
// was admissible when queued can be inadmissible when drained — and a node that embeds one makes
// its OWN P8 reject its OWN proposal, for every proposal it ever makes again. That is the
// permanent proposer self-wedge the over-cap drop beside it already guards.
//
// WHAT THIS GATE CONSTRUCTS AND WHAT IT DOES NOT. It does not construct the drain's re-check;
// it constructs the PRECONDITION — a queued proof that the node's own accept path now refuses —
// and drives the real proposeBlock over it. The precondition is reached on the latch route by the
// floor rising; here it is reached by queueing a sub-era-4 proof on an era-4 chain, which is the
// same state by a cheaper road. Driving rotateEpoch's readiness tally from the node tier would
// add a swarm to a test whose subject is one loop in one function.
//
// VACUOUS ON THE RATIFIED RC ROUTE, and that is said rather than discovered: with
// Era4ActivationHeight = 1 the floor is a genesis constant, MintVersion is time-invariant, and
// admissible-at-detection implies admissible-at-drain. The guard is built anyway because the
// latch route is reachable by configuration, and "it cannot happen on our genesis" is not a
// property of the code.
//
// RED before the drain guard landed: the proof is embedded, and arm (c) below shows what that
// costs — the node's own chain refuses the block it just built.
func TestGEF7_ProposerDrainRefusesEvidenceBelowTheEraFloor(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)

	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	tn := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
	tn.SetLedger(ledger)

	// An era-4 chain: the floor at every height above the genesis is v5.
	prop := identity.FromSeed(2)
	cfg := chain.DefaultConfig()
	cfg.Era3ActivationHeight, cfg.Era4ActivationHeight = 1, 1
	ch := chain.New(cfg, func(n ports.NodeID) int64 { return ledger.Reputation(n) })
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, prop.Signer())
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	tn.EnableChain(ch, ti.Signer())
	if got := ch.EraFloor()(9); got != chain.BlockVersionWitnessable {
		t.Fatalf("fixture: the floor at the evidence height must be v%d, got v%d",
			chain.BlockVersionWitnessable, got)
	}

	// A genuine double-sign by the culprit, in the SUB-ERA-4 form — the form a node running
	// before the latch would have detected and queued.
	culprit := identity.FromSeed(77)
	ck := culprit.Signer()
	leg := func(e string) chain.Block {
		b := chain.Block{Version: chain.BlockVersionStateRoot, Height: 9, Prev: g.Hash(),
			Entries: []ports.Entry{mkEntry(e)}}
		chain.Sign(&b, prop.Signer())
		b.Atts = []chain.Attestation{chain.AttestAt(&b, ck, 0, chain.PhasePrecommit, ch.ChainID())}
		return b
	}
	stale := chain.Equivocation{Culprit: append([]byte(nil), culprit.Signer().Public().(ed25519.PublicKey)...),
		A: leg("x"), B: leg("y")}

	// (a) THE PRECONDITION IS REAL: the pair genuinely convicts at a pre-era-4 floor, so what the
	// drain drops below is a proof that WAS evidence, not a malformed one.
	if err := chain.CheckEquivocation(&stale, ch.ChainID(), testEraFloor(0)); err != nil {
		t.Fatalf("fixture: the queued pair must be a genuine double-sign at a sub-era-4 floor, else this "+
			"gate drops something no detector would ever have queued; got %v", err)
	}
	// ... and is inadmissible on THIS chain.
	if err := chain.CheckEquivocation(&stale, ch.ChainID(), ch.EraFloor()); err == nil {
		t.Fatal("fixture: on this era-4 chain the queued pair must be below the floor, or there is nothing " +
			"for the drain to catch")
	}

	tn.pendingSlashes = []chain.Equivocation{stale}
	tn.slashQueued = map[ports.NodeID]bool{stale.CulpritID(): true}

	blk := &chain.Block{Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("carrier")}}
	deadID := identity.FromSeed(9).NodeID()
	_ = net.Endpoint(deadID)
	tn.proposeBlock(blk, []ports.NodeID{deadID}, nil, 1, func(error) {})
	sched.Run()

	// (b) THE DRAIN DROPPED IT, and released the on-chain latch so a later admissible proof for
	// the same culprit can still be queued.
	if len(blk.Slashes) != 0 {
		t.Fatalf("G-EF-7 RED: the proposal embedded %d proof(s) the node's own v5ValidateSlashes refuses. "+
			"Every proposal this node makes from now on is invalid and the only exit is IsSlashed, which "+
			"never flips — the permanent proposer self-wedge", len(blk.Slashes))
	}
	if len(tn.pendingSlashes) != 0 {
		t.Fatalf("the inadmissible proof must be DROPPED, not requeued: a proof the floor refuses at a "+
			"fixed height is refused forever (the floor never falls), so requeueing it re-runs the same "+
			"drop every proposal; queue length %d", len(tn.pendingSlashes))
	}
	if tn.slashQueued[stale.CulpritID()] {
		t.Fatal("the on-chain latch must be RELEASED on a drop, or a later ADMISSIBLE proof against the " +
			"same culprit is never queued and the culprit keeps its seat for good")
	}

	// (c) WHAT THE DROP BOUGHT, measured rather than asserted: had the proof ridden, this node's
	// own chain would have refused the block it just built.
	doomed := *blk
	doomed.Slashes = []chain.Equivocation{stale}
	if err := ch.ValidateProposal(&doomed); err == nil {
		t.Fatal("NON-VACUITY BROKEN: a block carrying the dropped proof is ACCEPTED by this node's own " +
			"chain, so the drop protected nothing and this gate would pass on a node that never checked")
	}
}
