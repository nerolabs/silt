package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestJudgePaysTheUndividedRepairPrice is G-BT-2 at the seam where the runtime value is
// READ: the judge settles a real verdict and the repairer's balance carries the price
// divided ONCE, at the end. The credit-tier gates prove the arithmetic; this one proves
// the judge calls it, on the shard length it measured from a survivor.
//
// The geometry is the worst un-warned case — -chunk-size 524264 ⇒ a 524,280-byte shard,
// an exact 1.99996 credits — on a stripe three shards down. The repairer is owed 7; the
// pre-G-BT-2 build paid 4. (The chunk size moved 52412 → 524264 with F1,
// D-BOUNTY-PRICE-F1-2026-09-12: the price is one SHARD of witnessed fetch, not k of them,
// so the shard carrying an exact 1.99996 credits is 10× larger. Every credit figure below
// is unchanged — G-BT-2 is a property of the division ORDER, not of the price basis.)
//
// Ablation that must go RED: settle with
// credit.RepairBountyBase(p.K, shardBytes) * int64(credit.RarestShardMultiplier(p.K, p.N, reachable)).
func TestJudgePaysTheUndividedRepairPrice(t *testing.T) {
	nd, l := mkJudge(t, 21)
	holder := identity.FromSeed(30).NodeID()
	claimant := identity.FromSeed(31).NodeID()
	funder := identity.FromSeed(32).NodeID()
	l.Register(funder)

	var root ports.Hash
	root[0] = 0xB2
	if err := l.FundEscrow(root, funder, 500_000); err != nil {
		t.Fatalf("fund escrow: %v", err)
	}

	const shardBytes = 524_264 + 16 // one whole ciphertext chunk at -chunk-size 524264
	const reachable = 13            // 3 of 16 lost ⇒ a 4× multiplier
	p := erasure.Params{K: 10, N: 16}
	floorFirst := credit.RepairBountyBase(p.K, shardBytes) * int64(credit.RarestShardMultiplier(p.K, p.N, reachable))
	if floorFirst != 4 {
		t.Fatalf("fixture: the pre-G-BT-2 price was %d, want 4", floorFirst)
	}

	before := l.Balance(holder)
	claim := repairproof.RepairClaim{Root: root, Stripe: 0, ShardPos: 3, Holder: holder}
	nd.settleRepairVerdict(claimant, claim, p, shardBytes, reachable, repairproof.Decision{Release: true})

	paid := l.Balance(holder) - before
	if paid != 7 {
		t.Fatalf("the judge paid %d credits, want 7 (⌊1.99996 × 4⌋); the floor-first price is %d — the division is not last at the call site", paid, floorFirst)
	}
	if nd.Stats.BountyBaseZero != 0 {
		t.Fatalf("the zero-signal fired on a base of %d", credit.RepairBountyBase(p.K, shardBytes))
	}
	if nd.Stats.BountiesReleased != 1 {
		t.Fatalf("BountiesReleased = %d, want 1", nd.Stats.BountiesReleased)
	}
}
