package node

// R2.7 detector A4-2 — a judge that pays a repair bounty to ITSELF (Economist advisory
// ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07 §1.2).
//
// It lives on the node, not the ledger: the Ledger does not know its own node id, and
// coupling the ledger to identity for a pure observability read would drag the
// Invariant-A classification surface with it. The judge already holds both values.
//
// REACHABLE, not a vacuous gate: claim.Holder is a field of an inbound,
// attacker-declared MsgRepairClaim and nothing refuses a claim naming the judge itself
// as the holder. THRESHOLD: an honest judge never pays itself, so ANY non-zero value is
// the A4 self-dealing shape (canary abort C-7, HARD).
//
// ABLATION run RED before this shipped (recorded in the PR): counting every release
// rather than only `claim.Holder == n.id` reddens the third-party arm below.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

func TestBountyPaidToSelfCountsTheJudgeAsHolder(t *testing.T) {
	p := erasure.Params{K: 6, N: 10}
	const shardBytes = 1 << 20
	wantBounty := credit.BountyFor(credit.RepairBountyBase(p.K, shardBytes), p.K, p.N, 8)
	if wantBounty <= 0 {
		t.Fatal("test setup: bounty should be positive")
	}

	// release settles one verdict on a fresh judge, with holder chosen by the caller,
	// and returns the judge so the arms can be compared.
	release := func(seed int64, holderOf func(self ports.NodeID) ports.NodeID) *Node {
		nd, l := mkJudge(t, seed)
		var root ports.Hash
		root[0] = 0xA4
		funder := identity.FromSeed(seed + 100).NodeID()
		l.Register(funder)
		if err := l.FundEscrow(root, funder, 500_000); err != nil {
			t.Fatalf("fund escrow: %v", err)
		}
		claimant := identity.FromSeed(seed + 200).NodeID()
		claim := repairproof.RepairClaim{Root: root, Stripe: 0, ShardPos: 7, Holder: holderOf(nd.id)}
		nd.settleRepairVerdict(claimant, claim, p, shardBytes, 8, repairproof.Decision{Release: true})
		if nd.Stats.BountiesReleased != 1 {
			t.Fatalf("seed %d: BountiesReleased = %d, want 1 — the fixture did not actually pay", seed, nd.Stats.BountiesReleased)
		}
		return nd
	}

	// The A4 shape: an inbound claim naming the judge itself as the holder. Nothing
	// refuses it, the bounty pays, and the counter fires.
	self := release(31, func(selfID ports.NodeID) ports.NodeID { return selfID })
	if self.Stats.BountyPaidToSelf != 1 {
		t.Fatalf("BountyPaidToSelf = %d, want 1 — a judge paid itself and nothing saw it", self.Stats.BountyPaidToSelf)
	}
	if self.Stats.BountyCreditsPaidToSelf != wantBounty {
		t.Fatalf("BountyCreditsPaidToSelf = %d, want %d", self.Stats.BountyCreditsPaidToSelf, wantBounty)
	}

	// The ablation arm: the same claim with a THIRD-PARTY holder leaves it at zero.
	other := release(32, func(ports.NodeID) ports.NodeID { return identity.FromSeed(999).NodeID() })
	if other.Stats.BountyPaidToSelf != 0 || other.Stats.BountyCreditsPaidToSelf != 0 {
		t.Fatalf("third-party holder: BountyPaidToSelf = %d credits = %d, want 0 and 0 — the counter fires on every release, so it is not an A4 detector",
			other.Stats.BountyPaidToSelf, other.Stats.BountyCreditsPaidToSelf)
	}
}
