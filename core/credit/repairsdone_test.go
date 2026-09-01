package credit

// Per-node repair-work counter (Boulder 2, R2.1 economy observability slice 6a).
// The per-OBJECT repair count (objectEscrow.repairs) cannot say WHO did the work;
// the repair-work observability needs the per-NODE dual. PayBounty now counts the
// repair against the repairer (RepairsDone) and accumulates the credits it earned
// (BountyEarned). Both are observability only — the Invariant-A guard
// (invariant_a_test.go) proves they raise no standing.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestRepairsDoneCountsPerNode(t *testing.T) {
	l := New(0, 0)
	repairer := ports.NodeID{0xA1}
	other := ports.NodeID{0xB2}
	rootX := ports.Hash{0x01}
	rootY := ports.Hash{0x02}

	// Fund two objects and pay the repairer bounties across BOTH, plus one bounty
	// to a different node — the per-node counter must attribute correctly.
	funder := ports.NodeID{0xF0}
	l.Register(funder)
	l.acct(funder).balance = 10_000
	if err := l.FundEscrow(rootX, funder, 5_000); err != nil {
		t.Fatal(err)
	}
	if err := l.FundEscrow(rootY, funder, 5_000); err != nil {
		t.Fatal(err)
	}

	l.PayBounty(rootX, repairer, 100) // repair 1 for repairer
	l.PayBounty(rootX, repairer, 200) // repair 2 for repairer
	l.PayBounty(rootY, repairer, 300) // repair 3 for repairer, different object
	l.PayBounty(rootY, other, 400)    // a repair by a DIFFERENT node

	if got := l.RepairsDone(repairer); got != 3 {
		t.Fatalf("RepairsDone(repairer)=%d, want 3 (two on rootX, one on rootY)", got)
	}
	if got := l.RepairsDone(other); got != 1 {
		t.Fatalf("RepairsDone(other)=%d, want 1", got)
	}
	if got := l.BountyEarned(repairer); got != 600 {
		t.Fatalf("BountyEarned(repairer)=%d, want 600 (100+200+300)", got)
	}
	if got := l.BountyEarned(other); got != 400 {
		t.Fatalf("BountyEarned(other)=%d, want 400", got)
	}

	// The per-node counter is the DUAL of the per-object count, not a copy of it:
	// rootX funded 2 repairs, rootY funded 2 repairs (one each to repairer/other).
	if got := l.EscrowRepairs(rootX); got != 2 {
		t.Fatalf("EscrowRepairs(rootX)=%d, want 2", got)
	}
	if got := l.EscrowRepairs(rootY); got != 2 {
		t.Fatalf("EscrowRepairs(rootY)=%d, want 2", got)
	}
}

// A bounty that pays NOTHING (empty reserve) must NOT count as a repair — the
// counter tracks paid repair-work, not attempted claims. This is the ablation
// for the "short/empty final payment" edge: without the pay>0 gate the counter
// would over-count, and this test would go red.
func TestRepairsDoneIgnoresEmptyReserve(t *testing.T) {
	l := New(0, 0)
	repairer := ports.NodeID{0xA1}
	root := ports.Hash{0x01}

	// No funding: the reserve is empty, PayBounty pays 0.
	if paid := l.PayBounty(root, repairer, 100); paid != 0 {
		t.Fatalf("PayBounty on empty reserve paid %d, want 0", paid)
	}
	if got := l.RepairsDone(repairer); got != 0 {
		t.Fatalf("an unpaid claim must not count: RepairsDone=%d, want 0", got)
	}
	if got := l.BountyEarned(repairer); got != 0 {
		t.Fatalf("an unpaid claim earns nothing: BountyEarned=%d, want 0", got)
	}
}
