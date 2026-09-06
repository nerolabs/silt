package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestGLambda8ZeroBountyBaseIsNamedNotSilent is G-λ-8 (G-R212-7): a released repair
// whose geometry pays a ZERO base (k·shardBytes below one credit of fetch — the 64 KiB
// sim chunk, or the 4 KiB shards of the old fixtures) pays nothing AND is named: the
// judge counts it in Stats.BountyBaseZero and journals it. The daemon has no chunk
// geometry at start-up to refuse on, so the loud settlement plus the publish-time
// warning (cmd/silt warnBountyChunk) are the built form of the certified refusal.
// Ablation: drop the base == 0 branch ⇒ RED (counter stays 0, silence).
func TestGLambda8ZeroBountyBaseIsNamedNotSilent(t *testing.T) {
	nd, l := mkJudge(t, 3)
	holder := identity.FromSeed(30).NodeID()
	claimant := identity.FromSeed(31).NodeID()
	var root ports.Hash
	root[0] = 0xD0
	funder := identity.FromSeed(32).NodeID()
	l.Register(funder)
	if err := l.FundEscrow(root, funder, 500_000); err != nil {
		t.Fatal(err)
	}
	before := l.Balance(holder)
	p := erasure.Params{K: 10, N: 16}
	const shardBytes = 6_553 // the shipped 64 KiB chunk / 10: k·shardBytes = 65,530 ≪ 262,144
	if base := credit.RepairBountyBase(p.K, shardBytes); base != 0 {
		t.Fatalf("setup: base %d, want 0 for this geometry", base)
	}
	claim := repairproof.RepairClaim{Root: root, Stripe: 0, ShardPos: 11, Holder: holder}
	nd.settleRepairVerdict(claimant, claim, p, shardBytes, 8, repairproof.Decision{Release: true})
	if got := l.Balance(holder) - before; got != 0 {
		t.Fatalf("a zero-base release paid %d", got)
	}
	if nd.Stats.BountyBaseZero != 1 {
		t.Fatalf("BountyBaseZero = %d, want 1 — a bounty silently OFF reads as a lost claim", nd.Stats.BountyBaseZero)
	}
	// Positive control: one credit of fetch per k·shardBytes pays, and is not counted.
	nd2, l2 := mkJudge(t, 4)
	l2.Register(funder)
	if err := l2.FundEscrow(root, funder, 500_000); err != nil {
		t.Fatal(err)
	}
	nd2.settleRepairVerdict(claimant, claim, p, 26_215, 8, repairproof.Decision{Release: true})
	if l2.Balance(holder) <= 0 || nd2.Stats.BountyBaseZero != 0 {
		t.Fatalf("positive control: paid %d, BountyBaseZero %d", l2.Balance(holder), nd2.Stats.BountyBaseZero)
	}
}
