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

// TestEquivocateRetriesUncommittedLeg378 guards the SECOND #378 wedge (the one the
// first resumable-placement fix exposed under WAN delay): a target ATTESTS a fork
// block but cannot yet COMMIT it, because its OWN attestation is not yet qualified
// from its local view. The honest propose handler attests on the PROPOSER's
// standing, but ValidateCommit counts the ATTESTER's — so under warm-up a target
// routinely attests Y and then acks the commit not-OK. A driver that latched a leg on
// the round-trip alone stranded Z chasing an uncommitted Y forever. The fix latches a
// leg only on a CONFIRMED commit and retries an attested-but-uncommitted leg.
//
// This stages it deterministically with reputation (no delay needed): honestYZ can
// attest Y (the proposer is qualified) but cannot commit it (its own reputation is
// below the attester bar) until it is bumped — exactly the wire warm-up, frozen.
func TestEquivocateRetriesUncommittedLeg378(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 13, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(id ports.NodeID) int64 { return ledger.Reputation(id) }
	qualify := func(id ports.NodeID) { ledger.RecordBondChallenge(id, ports.HashBytes(id[:]), 64<<20, true, 1) }

	idA := identity.FromSeed(791) // honestX — commits X immediately
	idB := identity.FromSeed(792) // honestYZ — attests but can't commit until qualified
	idC := identity.FromSeed(793) // the adversary (proposer of X/Y/Z)

	// Legacy (reputation) consensus: quorum 1, attester/proposer bar 100.
	cfg := chain.Config{Quorum: 1, MinProposerRep: 100, MinAttesterRep: 100}
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(ledger)
		ch := chain.New(cfg, repFn)
		g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
		chain.Sign(g, idC.Signer()) // C is the proposer of the whole synthetic history
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		return nd
	}
	a, b, c := mk(idA), mk(idB), mk(idC)
	b.Bootstrap([]ports.NodeID{idA.NodeID()}, func() {})
	c.Bootstrap([]ports.NodeID{idA.NodeID()}, func() {})
	sched.Run()

	// C (proposer) and A (honestX attester) are qualified; B is NOT yet — so B will
	// attest Y (proposer C is qualified) but cannot commit its own attestation.
	qualify(idC.NodeID())
	qualify(idA.NodeID())

	// Attempt 1: the heavier fork Y is placed FIRST on B — B attests it (proposer C
	// is qualified) but acks the commit not-OK (B below the attester bar), so the
	// drill reports an error rather than latching Y. X (placed last) is never reached.
	var err1 error
	done1 := false
	c.Equivocate(idA.NodeID(), idB.NodeID(), func(e error) { err1, done1 = e, true })
	sched.Run()
	if !done1 || err1 == nil {
		t.Fatalf("attempt 1 must not complete while honestYZ cannot commit Y: done=%v err=%v", done1, err1)
	}
	if _, ok := b.Chain().LookupRoot(advEntry("Y").Root); ok {
		t.Fatal("Y must NOT be committed on honestYZ yet (it could attest but not commit)")
	}
	if _, ok := a.Chain().LookupRoot(advEntry("X").Root); ok {
		t.Fatal("X must NOT be placed yet (it is the LAST leg, after the heavier fork)")
	}

	// Now qualify B: its own attestation counts, so Y (and then Z) can commit.
	qualify(idB.NodeID())

	// The retry RESUMES: Y now commits, Z extends it, then X lands on A last.
	var err2 error
	done2 := false
	c.Equivocate(idA.NodeID(), idB.NodeID(), func(e error) { err2, done2 = e, true })
	sched.Run()
	if !done2 || err2 != nil {
		t.Fatalf("#378: once honestYZ can commit, the retry must resume and complete: %v", err2)
	}
	if _, ok := b.Chain().LookupRoot(advEntry("Y").Root); !ok {
		t.Fatal("Y must be committed on honestYZ after it qualified")
	}
	if _, ok := b.Chain().LookupRoot(advEntry("Z").Root); !ok {
		t.Fatal("Z must extend Y on honestYZ (the heavier fork)")
	}
	if _, ok := a.Chain().LookupRoot(advEntry("X").Root); !ok {
		t.Fatal("X must be committed on honestX as the final leg")
	}
}
