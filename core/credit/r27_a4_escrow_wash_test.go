package credit

// R2.7 detector A4 — escrow laundering by self-repair, the LEDGER half (Economist
// advisory ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07 §1.2).
//
// A4-1: funded splits into a PREPAY leg and a SKIM leg, because the wash loop's
// recoverable money is the skim. A4-3: bounties released to a repairer that had already
// fetched from this node — a SHAPE, never a detection, never a disbursement input.
//
// A4-2 lives in core/node (the ledger does not know its own node id): see
// TestBountyPaidToSelfCountsTheJudgeAsHolder.
//
// ABLATIONS run RED before this shipped, recorded in the PR:
//   - reverseLane clawing back the PREPAY leg ⇒ TestEscrowFundedSplitsPrepayFromSkim red
//   - PayBounty counting every payment, not just prior fetchers ⇒
//     TestBountyToAPriorFetcherIsFlaggedNotBlocked red

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestEscrowFundedSplitsPrepayFromSkim drives a prepay, an object-aware serve and a
// PARTIAL settlement whose lane reversal claws skim back, asserting the two legs
// separately at every step. The prepay leg is never touched by a reversal.
func TestEscrowFundedSplitsPrepayFromSkim(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	const prepay = int64(100_000)
	U := int64(DeliveryIncrementBytes)
	D := int64(ServeMintBytesPerCredit)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r27-a4-1"))

	l := New(fee, grant)
	l.Register(server)
	l.Register(fetcher)

	// Step 1 — the prepay. One leg only.
	if err := l.FundEscrow(root, server, prepay); err != nil {
		t.Fatalf("FundEscrow: %v", err)
	}
	snap := l.DurabilitySnapshot(root)
	if snap.FundedPrepay != prepay || snap.FundedSkim != 0 {
		t.Fatalf("after prepay: prepay %d skim %d, want %d and 0", snap.FundedPrepay, snap.FundedSkim, prepay)
	}
	if snap.Funded != snap.FundedPrepay+snap.FundedSkim || snap.Funded != l.EscrowFunded(root) {
		t.Fatalf("after prepay: funded %d, legs %d+%d, EscrowFunded %d — the published total is not the sum of its legs",
			snap.Funded, snap.FundedPrepay, snap.FundedSkim, l.EscrowFunded(root))
	}

	// Step 2 — serve 16·Dλ on one lane. The auto-skim floors to 16/8 = 2 credits, and
	// it lands ONLY on the skim leg.
	B := 16 * D
	serveLane(l, server, fetcher, root, B, U)
	wantSkim := B * SkimNum / (SkimDen * D)
	if wantSkim != 2 {
		t.Fatalf("test setup: lane skim %d, want 2", wantSkim)
	}
	snap = l.DurabilitySnapshot(root)
	if snap.FundedPrepay != prepay || snap.FundedSkim != wantSkim {
		t.Fatalf("after serving: prepay %d skim %d, want %d and %d", snap.FundedPrepay, snap.FundedSkim, prepay, wantSkim)
	}

	// Step 3 — a PARTIAL settlement. Acknowledging one increment leaves B−U on the lane,
	// whose skim floor is 1, so reverseLane claws back exactly one credit; the
	// settlement's own skim at this size is 0. The reversal must land on the SKIM leg.
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	if _, _, why := l.SettleDelivery(server, fetcher, root, 1, budget, 0); why != ReasonPaid {
		t.Fatalf("partial settle reason %q, want %q", why, ReasonPaid)
	}
	snap = l.DurabilitySnapshot(root)
	if snap.FundedPrepay != prepay {
		t.Fatalf("after the reversal: prepay %d, want %d UNCHANGED — a reversal clawed back an operator's deposit", snap.FundedPrepay, prepay)
	}
	if snap.FundedSkim != wantSkim-1 {
		t.Fatalf("after the reversal: skim %d, want %d (exactly one credit clawed back)", snap.FundedSkim, wantSkim-1)
	}
	if snap.Funded != snap.FundedPrepay+snap.FundedSkim {
		t.Fatalf("after the reversal: funded %d != %d + %d", snap.Funded, snap.FundedPrepay, snap.FundedSkim)
	}
}

// TestBountyToAPriorFetcherIsFlaggedNotBlocked pays the same bounty to a repairer that
// has fetched from this node and to one that has not. The counter separates them; the
// disbursement is identical. A detector that changed the payment would be a mechanism.
func TestBountyToAPriorFetcherIsFlaggedNotBlocked(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	const bounty = int64(1_000)
	server, priorFetcher, stranger := id(1), id(2), id(3)
	root := ports.HashBytes([]byte("r27-a4-3"))

	l := New(fee, grant)
	l.Register(server)
	l.Register(stranger)
	if err := l.FundEscrow(root, server, 100_000); err != nil {
		t.Fatalf("FundEscrow: %v", err)
	}
	// priorFetcher earns its fetchedBytes the only way production does: by being served.
	l.RecordServe(server, priorFetcher, ports.ChunkID{1}, 4096)
	if l.FetchedBytes(priorFetcher) == 0 {
		t.Fatal("test setup: priorFetcher has no fetched bytes")
	}
	if l.FetchedBytes(stranger) != 0 {
		t.Fatal("test setup: stranger should have fetched nothing")
	}

	if p, c := l.BountyToPriorFetcher(); p != 0 || c != 0 {
		t.Fatalf("baseline: payments %d credits %d, want 0 and 0", p, c)
	}

	strangerBefore := l.Balance(stranger)
	paidStranger := l.PayBounty(root, stranger, bounty)
	p, c := l.BountyToPriorFetcher()
	if p != 0 || c != 0 {
		t.Fatalf("after paying a stranger: payments %d credits %d, want 0 and 0 — the shape fires on every payment", p, c)
	}

	fetcherBefore := l.Balance(priorFetcher)
	paidFetcher := l.PayBounty(root, priorFetcher, bounty)
	p, c = l.BountyToPriorFetcher()
	if p != 1 || c != bounty {
		t.Fatalf("after paying a prior fetcher: payments %d credits %d, want 1 and %d", p, c, bounty)
	}

	// The detector must not change disbursement: both settle identically.
	if paidStranger != bounty || paidFetcher != bounty {
		t.Fatalf("paid stranger %d, paid prior fetcher %d, want %d each — the detector moved the money", paidStranger, paidFetcher, bounty)
	}
	if got := l.Balance(stranger) - strangerBefore; got != bounty {
		t.Fatalf("stranger balance moved %d, want %d", got, bounty)
	}
	if got := l.Balance(priorFetcher) - fetcherBefore; got != bounty {
		t.Fatalf("prior-fetcher balance moved %d, want %d — flagged is not blocked", got, bounty)
	}
	if l.RepairsDone(stranger) != 1 || l.RepairsDone(priorFetcher) != 1 {
		t.Fatalf("repairsDone stranger %d prior-fetcher %d, want 1 each", l.RepairsDone(stranger), l.RepairsDone(priorFetcher))
	}
}
