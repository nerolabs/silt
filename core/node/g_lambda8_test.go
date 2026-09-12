package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestGLambda8ZeroBountyBaseIsNamedNotSilent is G-λ-8 (G-R212-7): a released repair whose
// geometry pays a ZERO base — k·shardBytes below one credit of fetch — is NAMED, counted
// in Stats.BountyBaseZero and journalled, whatever it ends up paying. The daemon has no
// chunk geometry at start-up to refuse on, so the loud settlement plus the publish-time
// warning (cmd/silt warnBountyChunk) are the built form of the certified refusal.
// Ablation: drop the base == 0 branch ⇒ RED (counter stays 0, silence).
//
// The signal reads the UNMULTIPLIED base, and G-BT-2 is why that matters: since the price
// divides once at the end, a rare stripe's multiplier can lift a zero-base geometry to a
// non-zero payment. Both arms are driven below — a geometry that pays nothing at ANY
// multiplier and one the multiplier lifts to 1 — and the counter fires on both. Reading
// the paid price instead would make the whole signal vacuous on exactly the stripes that
// matter most.
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
	// A REAL published geometry, not a synthetic one: a 1 KB object is a single frame, so
	// R-SHORT-FINAL-STRIPE makes its shard 1,024 + 8 + 16 = 1,048 bytes, far below the one
	// credit of fetch F1 prices a repair at. Even at the k-floor multiplier it pays nothing.
	const shardBytes = 1_024 + 8 + crypto.Overhead
	if base := credit.RepairBountyBase(p.K, shardBytes); base != 0 {
		t.Fatalf("setup: base %d, want 0 for this geometry", base)
	}
	claim := repairproof.RepairClaim{Root: root, Stripe: 0, ShardPos: 11, Holder: holder}
	nd.settleRepairVerdict(claimant, claim, p, shardBytes, p.K, repairproof.Decision{Release: true})
	if got := l.Balance(holder) - before; got != 0 {
		t.Fatalf("a zero-base release at the k-floor multiplier paid %d", got)
	}
	if nd.Stats.BountyBaseZero != 1 {
		t.Fatalf("BountyBaseZero = %d, want 1 — a bounty silently OFF reads as a lost claim", nd.Stats.BountyBaseZero)
	}

	// The other arm: a zero base the rarest-shard multiplier LIFTS above zero (G-BT-2).
	// The payment is real, and the signal must still fire — that is what keeps an
	// operator from reading "it paid something" as "the geometry is fine".
	ndLift, lLift := mkJudge(t, 5)
	lLift.Register(funder)
	if err := lLift.FundEscrow(root, funder, 500_000); err != nil {
		t.Fatal(err)
	}
	const liftShard = 65_536 // a base of 0 (0.25 credits), but 7× is 1.75 credits
	if base := credit.RepairBountyBase(p.K, liftShard); base != 0 {
		t.Fatalf("setup: base %d, want 0", base)
	}
	beforeLift := lLift.Balance(holder)
	ndLift.settleRepairVerdict(claimant, claim, p, liftShard, 8, repairproof.Decision{Release: true})
	if got := lLift.Balance(holder) - beforeLift; got != 1 {
		t.Fatalf("the lifted price paid %d, want 1 (⌊0.25 × 7⌋)", got)
	}
	if ndLift.Stats.BountyBaseZero != 1 {
		t.Fatalf("BountyBaseZero = %d on the LIFTED arm, want 1 — the signal must read the unmultiplied base", ndLift.Stats.BountyBaseZero)
	}
	// Positive control: one credit of fetch per SHARD pays, and is not counted (F1).
	nd2, l2 := mkJudge(t, 4)
	l2.Register(funder)
	if err := l2.FundEscrow(root, funder, 500_000); err != nil {
		t.Fatal(err)
	}
	nd2.settleRepairVerdict(claimant, claim, p, 262_144, 8, repairproof.Decision{Release: true})
	if l2.Balance(holder) <= 0 || nd2.Stats.BountyBaseZero != 0 {
		t.Fatalf("positive control: paid %d, BountyBaseZero %d", l2.Balance(holder), nd2.Stats.BountyBaseZero)
	}
}
