package node

// R2.2 / Lane C3 — the PER-TIER work totals at their source (Economist
// ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09 §3a).
//
// The cmd/silt gates drive these fields end to end through the real routes. This file
// pins the four rules the ACCUMULATION has to obey, at the tier where a mistake in them
// would live, because each is one line in the add closure and three of the four are
// invisible from the wire once the renderer has folded them into a ratio.

import "testing"

// TestR22PerTierTotalsFollowTheExclusionRuleAndNameTheirAbsences.
//
// FOUR RULES, one fixture, each asserted separately:
//
//  1. REPORTING PEERS ONLY. A peer excluded from the work series by the certified M-2 rule
//     is out of every per-tier numerator AND out of the pledged denominator. If its
//     CapTotal leaked into PledgedBytesByTier the null would be computed over a different
//     population from the observation, which is not a null.
//  2. A TIER WITH NO REPORTER IS ABSENT from all four maps. Not a zero entry: "no horse in
//     my sample reported" and "the horses in my sample did nothing" are different facts and
//     both render as 0.
//  3. A TIER WHOSE REPORTERS SUMMED TO ZERO IS PRESENT, with a zero. That zero is a
//     measurement, and ReportersByTier is the only field that tells it from rule 2.
//  4. RepairsByTier IS KEYED ON RepairCapable AND NOTHING ELSE. A reporting pony has no
//     entry however many repairs it claims, because it is outside the repair series'
//     denominator.
//
// ABLATIONS, each with the point it was MEASURED to redden at rather than the point it was
// expected to (a blind PE re-ran the first one and found the description named the wrong
// line):
//   - move the THREE per-tier lines above the `if srv <= 0 && rep <= 0 { return }` and rule 1
//     reddens at the FIRST of its three assertions, `ReportersByTier[pony] = 3, want 2` --
//     not on the pledged total, which is simply never reached.
//   - move ONLY the PledgedBytesByTier line and it reddens on the pledged total:
//     `PledgedBytesByTier[pony] = 12884901888, want 8589934592`.
//   - delete the RepairCapable guard around the RepairsByTier line and rule 4 reddens.
func TestR22PerTierTotalsFollowTheExclusionRuleAndNameTheirAbsences(t *testing.T) {
	n := r22Node(t)
	const gib = int64(1) << 30
	// Two REPORTING ponies (one of which serves nothing but claims repairs — a reporting
	// peer with a measured zero in the serve field), one SILENT pony, one reporting
	// archival node, and NO horse at all.
	gossip(n, 1, 4*gib, 10*gib, 0)  // pony, reports
	gossip(n, 2, 4*gib, 0, 5)       // pony, reports repairs only -> measured 0 served
	gossip(n, 3, 4*gib, 0, 0)       // pony, SILENT -> excluded from every series
	gossip(n, 4, 4<<40, 90*gib, 20) // archival, reports

	es := n.EconomySample()

	if es.Size != 4 || es.Mix[TierPony] != 3 || es.Mix[TierArchival] != 1 {
		t.Fatalf("the fixture did not land: size %d mix %v", es.Size, es.Mix)
	}

	// RULE 1 — the silent pony is nowhere. Its 4 GiB pledge must not appear.
	if got, want := es.ReportersByTier[TierPony], 2; got != want {
		t.Fatalf("ReportersByTier[pony] = %d, want %d: the silent pony is excluded from the work series by M-2 and must be excluded from its per-tier coverage too", got, want)
	}
	if got, want := es.PledgedBytesByTier[TierPony], 8*gib; got != want {
		t.Fatalf("PledgedBytesByTier[pony] = %d, want %d (TWO reporting ponies at 4 GiB). A silent peer's pledge in this denominator computes the holdings expectation over a different population from the repair observation it is compared against — the two stop being a null and an observation of the same thing.", got, want)
	}
	if got, want := es.ServeBytesByTier[TierPony], 10*gib; got != want {
		t.Fatalf("ServeBytesByTier[pony] = %d, want %d", got, want)
	}

	// RULE 2 — no horse reported because no horse exists. Every map must OMIT the key.
	for name, present := range map[string]bool{
		"ServeBytesByTier":   mapHasInt64(es.ServeBytesByTier, TierHorse),
		"RepairsByTier":      mapHasInt64(es.RepairsByTier, TierHorse),
		"PledgedBytesByTier": mapHasInt64(es.PledgedBytesByTier, TierHorse),
		"ReportersByTier":    mapHasInt(es.ReportersByTier, TierHorse),
	} {
		if present {
			t.Fatalf("%s carries a horse entry on a sample with no horse in it. A 0 here is a false absence: the consumer publishes it as a measured share of the work and an operator reads 'the horses do none of it'.", name)
		}
	}

	// RULE 3 — the pony tier DID report and its repair contribution is not applicable,
	// while its SERVE total is a real measured value. The distinguishing case is a capable
	// tier: give the archival node a reporting peer with zero repairs and the entry must
	// still exist. Driven here on the serve field of the reporting-repairs-only pony, whose
	// contribution to ServeBytesByTier is a measured 0 folded into the tier's 10 GiB.
	if _, ok := es.ServeBytesByTier[TierPony]; !ok {
		t.Fatalf("the pony tier reported and has no ServeBytesByTier entry")
	}
	// RULE 4 — RepairsByTier is keyed on RepairCapable and nothing else. The reporting pony
	// claimed 5 repairs.
	if mapHasInt64(es.RepairsByTier, TierPony) {
		t.Fatalf("RepairsByTier carries a pony entry (%d). A pony's repairs are outside the repair series' denominator (RepairCapable, D-TIERING coupling (b)), so a share published from this key would be a ratio over a population the tier is not in.", es.RepairsByTier[TierPony])
	}
	if got, want := es.RepairsByTier[TierArchival], int64(20); got != want {
		t.Fatalf("RepairsByTier[archival] = %d, want %d", got, want)
	}
	if es.RepairWorkTotal != 20 {
		t.Fatalf("RepairWorkTotal = %d, want 20: the pony's 5 claimed repairs must not be in the capable series' total either", es.RepairWorkTotal)
	}

	// CapableSize is the repair series' POPULATION and it must count every classifiable
	// capable peer, reporting or not. It is derived through RepairCapable over Mix, so it
	// cannot name a different set from the series.
	if es.CapableSize != 1 {
		t.Fatalf("CapableSize = %d, want 1 (the archival node)", es.CapableSize)
	}
	var viaMix int
	for tier, c := range es.Mix {
		if RepairCapable(tier) {
			viaMix += c
		}
	}
	if es.CapableSize != viaMix {
		t.Fatalf("CapableSize %d disagrees with the mix's capable sum %d", es.CapableSize, viaMix)
	}

	// The sum of the per-tier serve totals must BE ServeWorkTotal — the shares the renderer
	// publishes are over that denominator, so if the parts do not sum to the whole the
	// shares do not sum to 1 and no consumer can tell which is wrong.
	var serveSum, pledgeSum int64
	for _, v := range es.ServeBytesByTier {
		serveSum += v
	}
	for _, v := range es.PledgedBytesByTier {
		pledgeSum += v
	}
	if serveSum != es.ServeWorkTotal {
		t.Fatalf("the per-tier serve totals sum to %d but ServeWorkTotal is %d — the published shares would not sum to 1", serveSum, es.ServeWorkTotal)
	}
	if pledgeSum != 8*gib+(4<<40) {
		t.Fatalf("the per-tier pledged totals sum to %d, want %d (the three REPORTING peers)", pledgeSum, 8*gib+(4<<40))
	}

	// And a capable tier that reports and repairs NOTHING is present with a zero, which is
	// the measured-zero half of rule 3 and the case a key-presence test would otherwise
	// never reach.
	n2 := r22Node(t)
	gossip(n2, 1, 64*gib, 10*gib, 0) // horse, serves, has never repaired
	gossip(n2, 2, 4*gib, 10*gib, 0)  // pony, serves
	es2 := n2.EconomySample()
	v, ok := es2.RepairsByTier[TierHorse]
	if !ok || v != 0 {
		t.Fatalf("RepairsByTier[horse] = %d present=%v, want 0 PRESENT. A capable tier that reported and has done no repair yet is a MEASURED zero; dropping the key would make it indistinguishable from a tier that said nothing.", v, ok)
	}
	if es2.ReportersByTier[TierHorse] != 1 {
		t.Fatalf("ReportersByTier[horse] = %d, want 1 — the pair (total, reporters) is what tells a measured zero from an absence", es2.ReportersByTier[TierHorse])
	}
}

func mapHasInt64(m map[string]int64, k string) bool { _, ok := m[k]; return ok }
func mapHasInt(m map[string]int, k string) bool     { _, ok := m[k]; return ok }
