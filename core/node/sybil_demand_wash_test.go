package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestSelfDealtDemandIsAStrictLossAndBuysNoStanding — RC item 2, the demand axis, driven.
//
// The canon prices the demand axis by cost-to-wash, never as wash-free (TENETS: "Wash is
// re-priced, not proven away"), and keeps bandwidth credit out of standing ("can never become
// standing", VISION). This drives the wash a Sybil would run — one operator fetching from itself
// through a second identity it controls, paying for the delivery anchors, settling every
// increment — on the real node path, and measures both halves:
//
//   - THE WASH HAPPENED (the positive control on this axis): the server witnesses every
//     increment, so a fixture that failed to settle cannot pass as a denial.
//   - IT IS A STRICT LOSS: the operator's two identities end with less credit than they started
//     with — exactly the skim, which went to the object's repair escrow and not to them.
//   - IT BUYS NO STANDING: structurally, core/chain cannot read a witnessed increment or a balance
//     (internal/depcheck's standing firewall, which now names core/demand); standing stays the
//     bonded size, as the chain census measures.
//
// Ablations that must redden: settle without the skim (the loss clause), count a refused
// settlement as witnessed (the control), import core/demand into core/chain (the firewall).
func TestSelfDealtDemandIsAStrictLossAndBuysNoStanding(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	puppetID := identity.FromSeed(7301) // the operator's second identity, posing as a fetcher
	serverID := identity.FromSeed(7302) // the operator's serving node
	puppet := New(puppetID.NodeID(), DefaultConfig(), sched, net.Endpoint(puppetID.NodeID()), memstore.New())
	puppet.SetSigner(puppetID.Signer())
	server := New(serverID.NodeID(), DefaultConfig(), sched, net.Endpoint(serverID.NodeID()), memstore.New())
	const fee, grant = 50_000, 200_000
	ledger := credit.New(fee, grant)
	ledger.Register(serverID.NodeID())
	ledger.Register(puppetID.NodeID())
	server.SetLedger(ledger)
	commitSelfDemandKey(t, server, serverID, cachedRSAKey(t, 0))
	server.EnableDemandBank(server.id)
	server.EnableDeliverySessions(r29Idle)
	puppet.Bootstrap([]ports.NodeID{serverID.NodeID()}, func() {})
	server.Bootstrap([]ports.NodeID{puppetID.NodeID()}, func() {})
	sched.Run()

	before := ledger.Balance(server.id) + ledger.Balance(puppet.id)

	// The puppet PAYS for its delivery anchor, exactly as an honest fetcher would.
	const k = 1 // a session opens on one anchor
	for i := 0; i < k; i++ {
		if err := ledger.ChargePublish(puppet.id); err != nil {
			t.Fatalf("FIXTURE: the puppet cannot pay for anchor %d: %v", i, err)
		}
	}
	handle, m := openOverWire(t, puppet, server, sched, mintDemandTokensFor(t, server, 0, k))

	// It acknowledges every increment the anchors fund, on one object the operator controls.
	obj := ports.HashBytes([]byte("self-dealt"))
	budget := int64(k * fee)
	settled, err := settleOverWire(t, puppet, server, sched, handle, m, obj, uint64(budget))
	if err != nil || settled != budget {
		t.Fatalf("FIXTURE: the wash did not settle its whole budget: (%d, %v), want %d", settled, err, budget)
	}

	// The positive control: the wash was real and witnessed, increment for increment.
	if got := server.WitnessedIncrements(obj); got != budget {
		t.Fatalf("CONTROL: the server witnessed %d increments of a %d-increment wash; a denial "+
			"measured on a wash that never happened would be vacuous", got, budget)
	}

	// The loss: what the two identities paid in, less what came back, is the skim — to the
	// object's escrow, which is neither of them.
	after := ledger.Balance(server.id) + ledger.Balance(puppet.id)
	skim := budget * credit.SkimNum / credit.SkimDen
	if skim <= 0 {
		t.Fatalf("FIXTURE: the skim is %d, so this ledger cannot price a wash at all", skim)
	}
	if loss := before - after; loss != skim {
		t.Fatalf("the operator's two identities lost %d credits on a %d-credit wash, want exactly the "+
			"skim %d", loss, budget, skim)
	}
	if esc := ledger.EscrowBalance(obj); esc != skim {
		t.Fatalf("the object's escrow holds %d, want the skim %d", esc, skim)
	}
	t.Logf("a self-dealt wash of %d increments was witnessed in full and cost its operator %d "+
		"credits (the skim, to the object's escrow); standing cannot read either number "+
		"(internal/depcheck standing firewall)", budget, skim)
}
