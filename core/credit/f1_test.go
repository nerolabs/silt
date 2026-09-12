package credit

import "testing"

// F1 — the coherence floor, and the gate that holds its SHAPE.
// D-BOUNTY-PRICE-F1-2026-09-12, on
// silt-agent-memory/researcher/reviews/research-outcome/R-HOLDER-PARTICIPATION-CONSTRAINT-structural-floor-RESEARCH-CERTIFICATION-2026-09-12.md
// and its predecessor escrow-price-repair-cost-model-RESEARCH-CERTIFICATION-2026-09-12.md.
//
//	F1:       bounty(one shard-repair, mult = 1) ≥ shardBytes/(U/p)   ⇔  c·k ≥ 1
//	ceiling:  the payee is not paid more than the bytes it moved       ⇔  c·k ≤ 1
//	⇒ c·k = 1 exactly.
//
// The number is not the fragile part — the SHAPE is. c·k = 1 holds only while `k` is
// absent from the price, and `k` (erasure.DefaultParams) is Evolving-tier. These gates
// RUN that; they do not describe it.

// TestF1PriceIsOneShardOfWitnessedFetch drives floor and ceiling together over the whole
// shipped multiplier range and a band of geometries either side of one credit of fetch.
//
// Ablation that must go RED: put k back in the product in repairBountyCredits.
func TestF1PriceIsOneShardOfWitnessedFetch(t *testing.T) {
	// The point where floor and ceiling touch: a shard of EXACTLY one credit of fetch
	// pays exactly one credit. Below it the floor cannot be met at all (the zero class,
	// R-BOUNTY-ZERO-BELOW-262KB); above it the price tracks the bytes.
	if got := RepairBountyBase(10, DeliveryBytesPerCredit); got != 1 {
		t.Fatalf("a shard of exactly U/p pays %d, want 1 — F1's floor and the over-pay ceiling coincide there", got)
	}
	for _, shardBytes := range []int64{1, 16, 65_552, 262_143, DeliveryBytesPerCredit, 262_160, 524_280, 1 << 20, 1<<27 + 16} {
		for _, mult := range []int{1, 2, 4, 7} {
			got := repairBountyCredits(10, shardBytes, mult)
			// CEILING: never more credits than the witnessed price of the bytes moved.
			if got*DeliveryBytesPerCredit > shardBytes*int64(mult) {
				t.Fatalf("shard %d × %d: paid %d credits, worth %d B — an OVER-pay against the payee's own act",
					shardBytes, mult, got, shardBytes*int64(mult))
			}
			// FLOOR: the whole witnessed price, less at most the one truncation G-BT-2
			// permits (a single floor, at the end).
			if (got+1)*DeliveryBytesPerCredit <= shardBytes*int64(mult) {
				t.Fatalf("shard %d × %d: paid %d credits, but %d B of witnessed price was moved — more than one credit short",
					shardBytes, mult, got, shardBytes*int64(mult))
			}
		}
	}
}

// TestF1PriceCarriesNoK is the shape gate and the W4 refutation, driven.
//
// W4 (REFUTED by the certification): "encode c = 1/k as RepairBountyCoeffNum/Den = 1/10."
// It gives the right number at k = 10 and silently over-pays the moment k re-tunes, which
// it may — k is Evolving-tier. The counterfactual is computed here, not asserted.
//
// One supporting reason the record carried is WRONG and is corrected here rather than
// repeated: "1/10 adds a second integer floor before the last division." For positive
// integers ⌊⌊x/a⌋/b⌋ = ⌊x/(a·b)⌋ always, so nested integer division loses nothing and that
// is NOT why 1/10 is refused. The k-coupling is, on its own. Third arm below drives it.
func TestF1PriceCarriesNoK(t *testing.T) {
	const shardBytes = 262_160
	// 1. The shipped price does not move with k. k is still a parameter (it names a
	//    degenerate geometry and the judge's call sites hold it), so this is the gate
	//    that keeps it out of the arithmetic.
	want := RepairBountyBase(10, shardBytes)
	for k := 1; k <= 64; k++ {
		if got := RepairBountyBase(k, shardBytes); got != want {
			t.Fatalf("base at k=%d is %d, want %d — the price moved with k; F1 is c·k = 1 and k is Evolving-tier", k, got, want)
		}
	}
	// 2. The refused encoding, computed: c = 1/10 with k back in the product.
	coupled := func(k int, s int64, mult int) int64 {
		return int64(k) * s * int64(mult) * 1 / 10 / DeliveryBytesPerCredit
	}
	if got := coupled(10, shardBytes, 1); got != want {
		t.Fatalf("fixture: the 1/10 encoding pays %d at k=10, want the same %d — it is wrong only OFF the current k", got, want)
	}
	// At a 262,160 B shard the base truncates to 1 either way, so drive the over-pay at a
	// geometry where it is visible: 12/10 of the price, in credits.
	const big = 10 * 262_160
	if a, b := coupled(12, big, 1), repairBountyCredits(12, big, 1); a != 12 || b != 10 {
		t.Fatalf("k=12: the 1/10 encoding pays %d and F1 pays %d, want 12 and 10 — a 20%% silent over-pay is the reason W4 is refused", a, b)
	}
	// 3. The reason that does NOT hold: a second integer floor is lossless.
	for _, x := range []int64{1, 9, 25, 39, 262_143, 262_145, 2_621_600, 1 << 40} {
		if nested, single := x/10/DeliveryBytesPerCredit, x/(10*DeliveryBytesPerCredit); nested != single {
			t.Fatalf("x=%d: ⌊⌊x/10⌋/U⌋ = %d but ⌊x/(10·U)⌋ = %d — the nested-floor identity is what this arm asserts", x, nested, single)
		}
	}
}

// TestF1SolvencyBandIsExact pins the D-S7 self-funding threshold as the integer relation it
// is, because it is a PUBLISHED number and the record carries it wrong.
//
// income per stripe-retrieval = k·shardBytes/(SkimDen·Dλ); outflow per shard-repair =
// shardBytes/(U/p). **shardBytes cancels**, so S/R ≥ m̄ · SkimDen·Dλ/(k·U/p) — under F1
// exactly 1.2·m̄, i.e. 3.60 at m̄ = 3, and exactly 12·m̄ = 36.00 on the pre-F1 k·shardBytes
// basis. The certifications report 1.20007 and 12.0007; that 0.006 % is an artifact of
// pricing income at a 262,144 B shard and outflow at a 262,160 B one, and it cannot be
// real — the ratio has no shardBytes in it.
//
// Stated as integers so nothing here is a float comparison: 10·SkimDen·Dλ = 12·k·(U/p).
func TestF1SolvencyBandIsExact(t *testing.T) {
	const k = 10
	if got, want := int64(10)*SkimDen*ServeMintBytesPerCredit, int64(12)*k*DeliveryBytesPerCredit; got != want {
		t.Fatalf("the D-S7 threshold is not exactly 1.2·m̄ under F1: 10·SkimDen·Dλ = %d, 12·k·(U/p) = %d", got, want)
	}
	// And it is dimensionless: the same relation holds at any shard size, because the ratio
	// carries none. Driven rather than asserted — this is the property that clears
	// build-immutable #3's steerable-estimand rule (the publisher's choices cancel).
	// Cross-multiplied, so no division rounds: outflow/income = (s · SkimDen·Dλ)/(U/p · k · s),
	// and 10× that must equal 12 for every s.
	for _, shardBytes := range []int64{1_048, 65_552, 262_160, 6_710_887} {
		lhs := 10 * shardBytes * SkimDen * ServeMintBytesPerCredit // 10 · outflowNum · incomeDen
		rhs := 12 * DeliveryBytesPerCredit * int64(k) * shardBytes // 12 · outflowDen · incomeNum
		if lhs != rhs {
			t.Fatalf("shard %d: 10·(S/R per m̄) is %d/%d, want 12 — the threshold moved with shardBytes", shardBytes, lhs, rhs/12)
		}
	}
}

// TestF1ZeroClassIsTheAcceptedCost RUNS the cost the owner accepted, so it cannot rot into
// a sentence: the class of objects whose base repair bounty is ZERO widens 10.008×, from
// ≤ 26,190 B to ≤ 262,119 B of object. The boundary is stated in OBJECT bytes because that
// is what a publisher chooses; a single-frame object is stored at its true length
// (R-SHORT-FINAL-STRIPE), so its shard is its own bytes + an 8-byte frame header + the
// 16-byte tag. R-BOUNTY-ZERO-BELOW-262KB.
func TestF1ZeroClassIsTheAcceptedCost(t *testing.T) {
	const frameHeader, overhead = 8, 16
	shard := func(object int64) int64 { return object + frameHeader + overhead }

	if got := RepairBountyBase(10, shard(262_119)); got != 0 {
		t.Fatalf("a 262,119 B object pays a base of %d, want 0 — it is the LAST object in the zero class", got)
	}
	if got := RepairBountyBase(10, shard(262_120)); got != 1 {
		t.Fatalf("a 262,120 B object pays a base of %d, want 1 — it is the FIRST funded object", got)
	}
	// The pre-F1 boundary, for the same arithmetic: 10 × shard ≥ U/p held from 26,191 B.
	if got := int64(10) * shard(26_191) / DeliveryBytesPerCredit; got != 1 {
		t.Fatalf("the pre-F1 boundary re-derived: a 26,191 B object paid %d, want 1", got)
	}
	if got := int64(10) * shard(26_190) / DeliveryBytesPerCredit; got != 0 {
		t.Fatalf("the pre-F1 boundary re-derived: a 26,190 B object paid %d, want 0", got)
	}
	// 262,120 / 26,191 = 10.008× — stated as integers so the ratio is computed, not typed.
	if 262_120*1_000/26_191 != 10_008 {
		t.Fatalf("the zero class widened %d.%03d×, not 10.008×", 262_120*1_000/26_191/1_000, 262_120*1_000/26_191%1_000)
	}
}
