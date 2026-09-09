package credit

import "testing"

// G-BT-2 (BOULDER2 residual-closures certification 2026-09-07 §2.6): the repair price
// divides into credits ONCE, at the end — ⌊c·k·shardBytes·(lost+1)/(U/p)⌋ — instead of
// flooring the base first and multiplying a whole number after. These gates RUN the
// claim; they do not describe it.
//
// Ablation that must go RED: make RepairBounty return
// repairBountyCredits(k, shardBytes, 1) * int64(RarestShardMultiplier(k, n, reachable)).

// TestRepairBountyDividesAfterTheMultiplier is the named discriminator. At the
// certification's worst un-warned geometry (-chunk-size 52412 ⇒ a 52,428-byte shard,
// exact price 1.99996 credits) a stripe three shards down pays 7, not 4: flooring first
// threw away 3 of the 7 credits the repairer earned.
func TestRepairBountyDividesAfterTheMultiplier(t *testing.T) {
	const k, n, reachable = 10, 16, 13 // 3 lost ⇒ a 4× multiplier
	const shardBytes = 52_412 + 16
	mult := RarestShardMultiplier(k, n, reachable)
	if mult != 4 {
		t.Fatalf("fixture: multiplier %d, want 4", mult)
	}
	floorFirst := RepairBountyBase(k, shardBytes) * int64(mult)
	if floorFirst != 4 {
		t.Fatalf("fixture: the pre-G-BT-2 price was %d, want 4 (a base of 1 times a 4× multiplier)", floorFirst)
	}
	if got := RepairBounty(k, n, reachable, shardBytes); got != 7 {
		t.Fatalf("RepairBounty at the 52,428-byte shard, 3 lost = %d, want 7 — the division is not last", got)
	}
}

// TestRepairBountyIsDominantAndNeverOverPays sweeps the whole shipped multiplier range
// and a band of geometries either side of one credit of fetch. Two-sided: the single
// division is never WORSE than flooring first (that is why G-BT-2 is free), and never
// pays more than the exact price (that is why it is safe — PayBounty caps at the escrow
// balance regardless, but conservation must not lean on the cap).
func TestRepairBountyIsDominantAndNeverOverPays(t *testing.T) {
	const k, n = 10, 16
	for _, shardBytes := range []int64{1, 16, 20_000, 26_215, 52_428, 65_552, 262_160, 1 << 20, 1<<27 + 16} {
		for reachable := n + 2; reachable >= 0; reachable-- {
			mult := int64(RarestShardMultiplier(k, n, reachable))
			got := RepairBounty(k, n, reachable, shardBytes)
			if floorFirst := RepairBountyBase(k, shardBytes) * mult; got < floorFirst {
				t.Fatalf("shard %d, reachable %d: RepairBounty %d < the floor-first price %d — G-BT-2 must be dominant", shardBytes, reachable, got, floorFirst)
			}
			if exactNum := k * shardBytes * mult * RepairBountyCoeffNum; got*DeliveryBytesPerCredit*RepairBountyCoeffDen > exactNum {
				t.Fatalf("shard %d, reachable %d: RepairBounty %d exceeds the exact price %d/%d — an OVER-pay", shardBytes, reachable, got, exactNum, int64(DeliveryBytesPerCredit)*RepairBountyCoeffDen)
			}
		}
	}
}

// TestZeroSignalReadsTheUnmultipliedBase: the multiplier can lift a geometry whose base
// is ZERO to a positive price, so the G-λ-8 zero-signal would go VACUOUS if it read the
// paid price instead of the base. A 20,000-byte shard at k = 10 is 200,000 B — below one
// credit of fetch — yet a stripe at the k-floor pays 5 credits for it.
func TestZeroSignalReadsTheUnmultipliedBase(t *testing.T) {
	const k, n, shardBytes = 10, 16, 20_000
	if got := RepairBountyBase(k, shardBytes); got != 0 {
		t.Fatalf("fixture: base %d, want 0 (k·shardBytes = %d < U/p = %d)", got, k*shardBytes, int64(DeliveryBytesPerCredit))
	}
	if got := RepairBounty(k, n, k, shardBytes); got != 5 {
		t.Fatalf("the price at the k-floor = %d, want 5 — the case that makes reading the base load-bearing", got)
	}
}

// TestRepairBountyTruncationIsExactIntegerArithmetic pins the two integers the publish
// warning speaks (G-BT-1): they are computed, never typed, and the money path uses no
// floating point. The floored 1e-5 price is never over-stated; the loss is rounded to
// the nearest tenth of a percent.
func TestRepairBountyTruncationIsExactIntegerArithmetic(t *testing.T) {
	for _, c := range []struct {
		k                     int
		shardBytes            int64
		wantExactE5, wantLoss int64
	}{
		{10, 52_412 + 16, 199_996, 500},  // the certification's 50 % silent case
		{10, 65_536 + 16, 250_061, 200},  // the former 64 KiB default: 2 of an exact 2.5006
		{10, 262_144 + 16, 1_000_061, 0}, // the shipped default: 10 of an exact 10.00061
		{10, 20_000, 76_293, 1_000},      // a zero base loses the WHOLE price
		{10, 26_199 + 16, 100_002, 0},    // one byte's worth above the zero threshold
		{0, 262_160, 0, 0},               // degenerate k
	} {
		gotE5, gotLoss := RepairBountyTruncation(c.k, c.shardBytes)
		if gotE5 != c.wantExactE5 || gotLoss != c.wantLoss {
			t.Fatalf("RepairBountyTruncation(%d, %d) = (%d, %d), want (%d, %d)", c.k, c.shardBytes, gotE5, gotLoss, c.wantExactE5, c.wantLoss)
		}
	}
}

// TestSubFrameObjectDurabilityIsPrepayOnly RUNS the consequence the C5 deliberation first
// stated the wrong way round (blind PE B-2, 2026-09-09). The claim was that a sub-frame
// object, whose repair bounty base is now ZERO, is "kept alive by the serve economy
// instead". Measured, it is not: the auto-skim accumulates on a per-(server, requester,
// root) LANE, so a small object served once each to many DIFFERENT fetchers never reaches
// one credit on any lane. Its durability is prepay-only.
//
// The band is the point, not the endpoints: it takes 3,002 serves of the same object TO
// THE SAME FETCHER to fund the first escrow credit at a 1,048-byte shard, against 12 at
// the padded 262,160-byte shard.
func TestSubFrameObjectDurabilityIsPrepayOnly(t *testing.T) {
	const (
		paddedShard = 262_160 // a full 256 KiB frame + the GCM tag, as it was
		shortShard  = 1_048   // a 1 KB object's own bytes + header + tag, as it is
		serves      = 5_000
		fetchers    = 250
	)
	spread := func(shardBytes int64) int64 {
		l := New(50_000, 1_000_000)
		server, payee := id(251), id(252) // fetchers are bytes 0..249, so no id collides
		root := objRoot("sub-frame")
		for i := 0; i < serves; i++ {
			l.RecordServeToObject(server, id(byte(i%fetchers)), root, payee, shardBytes)
		}
		return l.EscrowBalance(root)
	}
	sameLane := func(shardBytes int64) int {
		l := New(50_000, 1_000_000)
		server, requester, payee := id(251), id(0), id(252)
		root := objRoot("one-lane")
		for i := 1; ; i++ {
			l.RecordServeToObject(server, requester, root, payee, shardBytes)
			if l.EscrowBalance(root) > 0 {
				return i
			}
			if i > 10_000 {
				t.Fatalf("no escrow credit after %d serves of %d B", i, shardBytes)
			}
		}
	}
	if got := spread(paddedShard); got != 250 {
		t.Fatalf("padded shard: %d serves over %d fetchers skimmed %d credits, want 250", serves, fetchers, got)
	}
	if got := spread(shortShard); got != 0 {
		t.Fatalf("short shard: %d serves over %d fetchers skimmed %d credits, want 0 — a sub-frame object earns NO skim in the many-fetchers pattern, so its durability is prepay-only", serves, fetchers, got)
	}
	if got := sameLane(paddedShard); got != 12 {
		t.Fatalf("padded shard: first escrow credit after %d same-lane serves, want 12", got)
	}
	if got := sameLane(shortShard); got != 3_002 {
		t.Fatalf("short shard: first escrow credit after %d same-lane serves, want 3,002", got)
	}
}
