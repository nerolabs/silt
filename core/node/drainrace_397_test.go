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

// Shared scaffolding: two anchor validators on one sim net, genesis committed,
// objective mode on. Returns both nodes plus the one that is NOT the designated
// drain proposer at the current height (the node whose behavior #397's Q2b
// closures constrain).
func mkDrainPair(t *testing.T) (sched *simclock.Scheduler, designated, other *Node) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())

	i1, i2 := identity.FromSeed(9101), identity.FromSeed(9102)
	anchors := map[ports.NodeID]bool{i1.NodeID(): true, i2.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-397")}}
	chain.Sign(g, i1.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}

	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	n1, n2 := mk(i1), mk(i2)
	n1.Bootstrap([]ports.NodeID{i2.NodeID()}, func() {})
	n2.Bootstrap([]ports.NodeID{i1.NodeID()}, func() {})
	sched.Run()

	// Which of the two is designated at the next height?
	props := n1.Chain().EligibleProposers()
	if len(props) != 2 {
		t.Fatalf("setup: want 2 eligible proposers, got %d", len(props))
	}
	_, height := n1.Chain().Head()
	desID := props[int(height)%len(props)]
	designated, other = n1, n2
	if desID == n2.ID() {
		designated, other = n2, n1
	}
	// Tests observe whether `other` proposed via its bondDrainInFlight latch,
	// which flips true for the duration of a drain gather.
	return sched, designated, other
}

// #397 Q2b-2 (research-certified) — SUBMIT-DON'T-PROPOSE: a NON-designated
// proposer whose only pending work is its OWN due registration must never
// propose for it; the registration reaches the chain through the designated
// proposer's queue (SubmitBondRenewal broadcasts it every sweep). This removes
// the field wedge's driver — genesis-aligned renewal clocks made two anchors
// both propose height 6 and cross-attest into a double slash (b88245d-3496).
func TestOwnRenewalNeverMakesNonDesignatedProposerPropose397(t *testing.T) {
	_, _, other := mkDrainPair(t)

	if !other.chain.BondRenewalDue(other.ID()) {
		t.Fatal("setup: the non-designated node's own registration should be due (not yet bonded)")
	}
	if len(other.pendingBondRegs) != 0 {
		t.Fatal("setup: no peer-submitted regs expected")
	}

	// Many sweeps of the drain decision: own-due alone must never start a
	// proposal, no matter how long the designated proposer takes.
	for i := 0; i < 10; i++ {
		other.maybeProposeBondDrain()
		if other.bondDrainInFlight {
			t.Fatalf("sweep %d: a non-designated proposer proposed for its OWN renewal — must submit, never propose (#397 Q2b-2)", i)
		}
	}
	if other.drainWaitSweeps != 0 {
		t.Fatalf("own-due-only must not accumulate takeover sweeps (got %d) — there is nothing to take over for", other.drainWaitSweeps)
	}
}

// #397 Q2b-1 (research-certified) — STAGGERED TAKEOVER: when the designated
// proposer is idle with peer-submitted work pending, eligible proposers take
// over ONE PER SWEEP in rank order — the old flat 3-sweep window readmitted
// every proposer simultaneously, recreating the race the designated rule
// exists to prevent. Rank distance 1 ⇒ the takeover fires on the sweep after
// the old flat window (3+1), never before.
func TestTakeoverIsStaggeredByProposerRank397(t *testing.T) {
	sched, _, other := mkDrainPair(t)

	// Give the non-designated node pending PEER work (the takeover premise).
	// A zero-value reg suffices to arm the trigger; validity is enforced later
	// at embed time, not at the takeover decision.
	other.pendingBondRegs[identity.FromSeed(9109).NodeID()] = chain.BondReg{}

	dist := 1 // two proposers: the non-designated one is rank distance 1
	for sweep := 0; sweep < 3+dist; sweep++ {
		other.maybeProposeBondDrain()
		if other.bondDrainInFlight {
			t.Fatalf("sweep %d: takeover fired inside the stagger window (want wait of %d sweeps for rank distance %d — one proposer readmitted per sweep, #397 Q2b-1)", sweep, 3+dist, dist)
		}
	}
	other.maybeProposeBondDrain() // sweep 3+dist: this rank's turn
	if !other.bondDrainInFlight {
		t.Fatal("the staggered takeover never fired at its rank's turn — the fallback must preserve liveness when the designated proposer is silent")
	}
	sched.Run()
}
