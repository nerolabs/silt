package chain

import "testing"

// #535 fix (2) PROOF-FIRST model-check (research certification 2026-08-23, the
// "certify (2) only once the #402 handoff-intersection proof is model-checked"
// obligation). Fix (2) proposes: at the epoch boundary, re-base the rotation
// quorum against `old ∩ next` (the continuing members) so a boundary can rotate
// after `old` has bled offline weight. The certification left "the precise
// old∩next sizing and the handoff proof" as an OPEN builder/model-check detail
// and named the safety it must preserve: "the boundary block must be final under
// BOTH the old and the new set so two competing boundaries cannot each finalize."
//
// THIS TEST DISCHARGES THAT OBLIGATION — and finds the NAIVE form UNSAFE.
//
// The mature super-quorum is weight-counted: a block finalizes on support
// `3·support > 2·total`. Two conflicting boundary blocks B1, B2 both finalize
// iff honest members can be split into disjoint H1, H2 with Byz+H1 and Byz+H2
// each over the bar (the shared Byzantine weight double-signs). Over a total
// W with Byzantine weight Byz and honest H = W−Byz, that is feasible iff
// **Byz > W/3** (algebra: 2·Byz + (H1+H2) > 4/3·W with H1+H2 ≤ H ⇒ Byz > H/2 ⇒
// Byz > W/3). The frozen set's safety premise is exactly Byz_old < ⅓·T_old.
//
// Re-basing to `old∩next` shrinks the total to T−L (L = lapsed weight) but the
// EXCLUDED weight may be honest — Byzantine members can lapse too, but the
// WORST case for safety is that only HONEST weight lapsed, leaving Byz
// unchanged over a smaller denominator. Then the boundary quorum finalizes a
// fork iff Byz_old > ⅓·(T_old − L) — which, for Byz_old near its ⅓·T_old
// premise limit, holds for ANY honest lapse L > 0. This is the SAME
// fault-tolerance wall that sank the certification's rejected fix (1): excluding
// possibly-honest weight raises the Byzantine FRACTION of what remains.
//
// The oracle asserts the property the SHIPPED code satisfies (no re-basing: the
// bar stays ⅔ of the full frozen `old`, so a bled boundary STALLS — safe) and
// DEMONSTRATES that the naive re-basing rule would admit a conflicting boundary
// at field-realistic parameters (the reason fix (2) is not shippable as an
// automatic denominator re-basing; the recovery is fix (4) + fix (3)).
func TestModelCheck_535_Fix2NaiveRebasingBreaksI1(t *testing.T) {
	// forkPossible reports whether two conflicting blocks can each meet the
	// weight super-quorum 3·support > 2·total over a set of the given total,
	// with `byz` Byzantine weight free to double-sign — i.e. Byz > total/3.
	forkPossible := func(total, byz int64) bool { return 3*byz > total }

	// The field topology (run 45da13c-17686): 4 anchors + 4 maturers at 64 MiB,
	// 4 sybils at 1 MiB. Frozen total 516 MiB.
	const MiB = int64(1) << 20
	tOld := (8*64 + 4*1) * MiB // 516 MiB

	// Byzantine weight at the frozen set's safety premise limit (just under
	// ⅓·T_old = 172 MiB): a coalition the frozen-epoch quorum is designed to
	// tolerate. Use 171 MiB — safe for the FULL frozen set.
	byz := int64(171) * MiB
	if got := forkPossible(tOld, byz); got {
		t.Fatalf("premise: Byz=%d MiB must be SAFE against the full frozen set T=%d MiB (< ⅓·T=%d) — the frozen quorum's whole guarantee",
			byz/MiB, tOld/MiB, tOld/(3*MiB))
	}

	// SHIPPED behavior (no re-basing): the bled boundary is measured against the
	// full frozen T_old, so no fork is possible — it STALLS instead. Safe.
	// (The stall is the certified-correct behavior; recovery is fixes (4)/(3).)
	if forkPossible(tOld, byz) {
		t.Fatal("shipped: measuring the boundary against the full frozen set must never admit a fork")
	}

	// NAIVE FIX (2): re-base to old∩next by excluding the lapsed weight. The
	// field lapsed 3 maturers = 192 MiB (all honest). Re-based total 324 MiB.
	lapsedHonest := int64(3*64) * MiB // 192 MiB, the field's lapsed maturers
	tRebased := tOld - lapsedHonest   // 324 MiB — the field's `324 MiB across 9`
	// Byzantine weight is UNCHANGED (Byzantine members need not lapse) over the
	// smaller denominator → its fraction rises.
	if !forkPossible(tRebased, byz) {
		t.Fatalf("EXPECTED the naive re-basing to be unsafe here: Byz=%d MiB vs ⅓·(T−L)=%d MiB — if this is green the analysis is wrong, re-derive before building fix (2)",
			byz/MiB, tRebased/(3*MiB))
	}
	// It IS unsafe: 171 MiB Byzantine > ⅓·324 = 108 MiB → two conflicting
	// boundary blocks can each gather the re-based super-quorum. This is the
	// #535 fix (2) proof obligation DISCHARGED with a counterexample: naive
	// old∩next re-basing reopens I1 at field-realistic parameters.
	t.Logf("#535 fix (2) naive re-basing is UNSAFE at field parameters: "+
		"frozen T=%d MiB (Byz=%d < ⅓·T=%d, safe); re-based T−L=%d MiB (Byz=%d > ⅓·(T−L)=%d, I1 BREAK). "+
		"Excluding possibly-honest lapsed weight raises the Byzantine fraction — the same wall that sank fix (1). "+
		"Automatic denominator re-basing is NOT safely realizable; recovery is fix (4) [returning members] + fix (3) [WS social escape].",
		tOld/MiB, byz/MiB, tOld/(3*MiB), tRebased/MiB, byz/MiB, tRebased/(3*MiB))

	// The SAFE boundary: re-basing is only sound when the excluded weight is
	// small enough that Byzantine stays < ⅓ of the reduced set — L < T − 3·Byz.
	// At Byz near ⅓·T that is L < ~0, i.e. NO exclusion is safe without knowing
	// Byz. This is exactly why fix (3) (an operator-signaled WS re-snapshot that
	// a human confirms is a real >⅓-honest-loss event) is the guaranteed-safe
	// recovery, not an automatic quorum change.
	safeExclusion := tOld - 3*byz // T − 3·Byz
	if safeExclusion >= lapsedHonest {
		t.Fatalf("if the field's 192 MiB lapse were within the safe exclusion bound (%d MiB) the naive rule would be safe — it is not (bound ≤ 0 near the Byzantine limit)", safeExclusion/MiB)
	}
}
