package node

// AN ADVERSARY THAT CANNOT PLACE A BLOCK PROVES NOTHING.
//
// Every drill in the red-team harness rests on one primitive: the adversary gets a block
// ACCEPTED by an honest peer, and the honest peer's judgement of that block is the
// evidence. A forged proposal must be refused FOR THE FORGERY; an under-bonded one FOR
// THE BOND; a double-sign must be PLACED on two peers so the detector can catch the
// same-slot pair. If the adversary's block is refused for a reason that has nothing to do
// with the property under test, every one of those verdicts is unobtainable — and the two
// REFUSAL drills still report PASS, because a refusal is what they expect. That is a false
// green, which is worse than the red it hides.
//
// THE AXIS THIS GATE HOLDS IS THE BLOCK'S ERA. An honest proposer never picks a block
// version: it asks the chain (chain.MintVersion), which is a pure function of committed
// history, so every honest proposer at one head mints the identical version. An adversary
// that hardcodes a version instead is not modelling a Byzantine validator — it is
// modelling a node running the wrong binary, and the era rule refuses it before any
// consensus property is reached. era-4 activates at height 1 and the daemon REFUSES to
// start with any other activation height, so a hardcoded pre-v5 block is refused at the
// FIRST height an adversary can ever propose. There is no warm-up that fixes it and no
// retry that outlasts it.
//
// STANDING IS CONTROLLED FOR, DELIBERATELY. Both arms qualify the proposer by fiat before
// proposing, so reputation cannot be the reason for any refusal here. That control is the
// point: the wire symptom of this defect was an adversary reporting "not yet standing?" —
// its own guess, from an opaque OK=false — which sent the diagnosis down a false trail for
// a full suite budget. With standing held constant, a refusal can only be the era.

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

// eraMintPair builds a two-node legacy-reputation network on a chain whose era-4
// boundary is the SHIPPED one (activation height 1, the only value the daemon accepts),
// and qualifies every identity so reputation is never the reason for a refusal.
func eraMintPair(t *testing.T, seedAdv, seedTarget int64) (adv, target *Node, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 17, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(id ports.NodeID) int64 { return ledger.Reputation(id) }

	idAdv := identity.FromSeed(seedAdv)
	idTgt := identity.FromSeed(seedTarget)

	// The shipped boundary: -era4-activation-height is 1 and daemon.go refuses to start
	// with anything else, so height 1 — the first block any adversary can propose — is
	// already a v5-required height.
	cfg := chain.Config{Quorum: 1, MinProposerRep: 100, MinAttesterRep: 100, Era4ActivationHeight: 1}
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(ledger)
		ch := chain.New(cfg, repFn)
		g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
		chain.Sign(g, idAdv.Signer())
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		return nd
	}
	adv, target = mk(idAdv), mk(idTgt)
	target.Bootstrap([]ports.NodeID{idAdv.NodeID()}, func() {})
	sched.Run()

	// Standing is NOT the axis under test: grant it to both, so any refusal below names
	// the block, never the proposer.
	for _, id := range []ports.NodeID{idAdv.NodeID(), idTgt.NodeID()} {
		ledger.RecordBondChallenge(id, ports.HashBytes(id[:]), 64<<20, true, 1)
	}
	t.Cleanup(sched.Run)
	return adv, target, sched
}

// TestAdversaryGoodProposalIsAcceptedAtTheChainsEra is the load-bearing one. It is the
// red-team harness's OWN positive control — the proposal an honest target must ACCEPT —
// and every refusal drill is gated on it. If this cannot be placed, the drills that
// follow are asserting refusals they would get for free.
func TestAdversaryGoodProposalIsAcceptedAtTheChainsEra(t *testing.T) {
	adv, target, sched := eraMintPair(t, 8101, 8102)

	var accepted bool
	var perr error
	done := false
	adv.ProposeGoodBlock(target.ID(), func(ok bool, err error) { accepted, perr, done = ok, err, true })
	sched.Run()

	if !done {
		t.Fatal("the good-block proposal never completed")
	}
	if perr != nil {
		t.Fatalf("the harness's own positive control errored: %v", perr)
	}
	if !accepted {
		_, h := adv.Chain().Head()
		t.Fatalf("THE RED-TEAM HARNESS CANNOT PLACE A VALID BLOCK.\n"+
			"  An honest, qualified, correctly-signed proposal at height %d was REFUSED.\n"+
			"  The chain mints v%d at that height (chain.MintVersion); a builder that stamps a\n"+
			"  different version is refused by the era rule before any consensus property is\n"+
			"  reached — so the forged-block and low-bond drills gated on this control are\n"+
			"  asserting refusals they would get for ANY block, and the equivocation drill can\n"+
			"  never place its forks at all. Build adversary blocks at chain.MintVersion(h) and\n"+
			"  populate the era roots, exactly as the honest propose path does.",
			h, adv.Chain().MintVersion(h))
	}
}

// TestAdversaryForgedProposalIsRefusedForTheForgery is the discrimination arm, and it is
// why the positive control above is not sufficient on its own. A harness whose every
// block is refused passes a refusal drill for the wrong reason. Pairing the two pins the
// refusal to the forgery: the same builder, the same height, the same standing — only the
// signature differs, and only the forged one is refused.
func TestAdversaryForgedProposalIsRefusedForTheForgery(t *testing.T) {
	adv, target, sched := eraMintPair(t, 8103, 8104)

	var refused bool
	var perr error
	done := false
	adv.ProposeBadBlock(target.ID(), true, func(r bool, err error) { refused, perr, done = r, err, true })
	sched.Run()

	if !done {
		t.Fatal("the forged proposal never completed")
	}
	if perr != nil {
		t.Fatalf("forged proposal errored rather than being refused: %v", perr)
	}
	if !refused {
		t.Fatal("an honest target ACCEPTED a block whose proposer signature was corrupted (DEFECT)")
	}
}

// TestEquivocationDrillPlacesBothForksAtTheChainsEra is the gate the red-team suite
// actually depends on, at the era it actually runs. Every existing in-process drill gate
// builds its chain WITHOUT an era-4 activation height, so all of them exercise the legacy
// (v2) block format and none of them can see a mint-era defect. The deployed lane pins
// era-4 activation at height 1 — the daemon refuses to start with any other value — so
// the first block the drill can place is already a v5-required block.
//
// It asserts PLACEMENT, not detection: the double-sign landing on two honest peers is the
// adversary's whole contribution, and it is the step that was silently impossible. What
// the product does afterwards (catch the same-slot pair and evict) is held by the
// detection gates, which were never blocked.
func TestEquivocationDrillPlacesBothForksAtTheChainsEra(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 19, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(id ports.NodeID) int64 { return ledger.Reputation(id) }

	idX := identity.FromSeed(8201)  // honestX: holds the losing fork
	idYZ := identity.FromSeed(8202) // honestYZ: holds the heavier fork
	idA := identity.FromSeed(8203)  // the adversary

	cfg := chain.Config{Quorum: 1, MinProposerRep: 100, MinAttesterRep: 100, Era4ActivationHeight: 1}
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(ledger)
		ch := chain.New(cfg, repFn)
		g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
		chain.Sign(g, idA.Signer())
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		return nd
	}
	x, yz, adv := mk(idX), mk(idYZ), mk(idA)
	yz.Bootstrap([]ports.NodeID{idX.NodeID()}, func() {})
	adv.Bootstrap([]ports.NodeID{idX.NodeID()}, func() {})
	sched.Run()

	// Standing for everyone: the axis under test is the block's era, not reputation.
	for _, id := range []ports.NodeID{idX.NodeID(), idYZ.NodeID(), idA.NodeID()} {
		ledger.RecordBondChallenge(id, ports.HashBytes(id[:]), 64<<20, true, 1)
	}

	var eqErr error
	eqDone := false
	adv.Equivocate(idX.NodeID(), idYZ.NodeID(), func(e error) { eqErr, eqDone = e, true })
	sched.Run()

	if !eqDone {
		t.Fatal("the double-sign drill never reported: it is stuck retrying a placement that cannot succeed")
	}
	if eqErr != nil {
		t.Fatalf("THE DOUBLE-SIGN COULD NOT BE PLACED: %v\n"+
			"  Both forks must land on honest peers, or the same-slot signature pair the slash\n"+
			"  rule catches is never created and the accountability property is unobservable.", eqErr)
	}

	// The forks are DIFFERENT blocks at ONE height — the self-incriminating pair.
	_, xh := x.Chain().Head()
	_, yzh := yz.Chain().Head()
	if xh < 2 {
		t.Fatalf("honestX never committed the losing fork (head+1=%d)", xh)
	}
	if yzh <= xh {
		t.Fatalf("the Y fork must be HEAVIER than the X fork so the detector syncs it: "+
			"honestYZ head+1=%d, honestX head+1=%d", yzh, xh)
	}
	if adv.EquivocateHeight() == 0 {
		t.Fatal("the drill reported success without pinning a double-sign height")
	}
}
