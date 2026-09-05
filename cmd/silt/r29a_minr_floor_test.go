//go:build bbootstrap

package main

// R2.9a DELTA — the WIRE tier of the minimum-requester floor (G-BB-11 / BB-15) and the
// polling oracle (BB-16), from RESEARCH CERTIFICATION
// R2.9a-Bbootstrap-DELTA-contamination-privacy-floor-clock (2026-09-04) §2.1 to §2.4.

import (
	"encoding/json"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// --- BB-15: the floor, on the wire ---------------------------------------------------

// TestR29aBB15WirePayloadWithholdsTheCensusBelowTheFloor is BB-15 on the artifact a
// third party actually reads. At R_min − 1 the block carries `suppressed: true` and the
// keys `cells`, `aged` and `requesters` do not appear; at R_min all three are back.
//
// The census counts are ABSENT rather than zero on purpose. A published `requesters: 0`
// under a census of nine is a FALSE total, and a reader that sums the block reads it as
// a measured one; a missing key cannot be misread. `cells` is null, which carries no
// count either — it is the same "no grid" object a dead clock publishes, and
// `suppressed` plus `ageAxisLive` tell the two apart.
func TestR29aBB15WirePayloadWithholdsTheCensusBelowTheFloor(t *testing.T) {
	block := func(R int) map[string]any {
		s, led, clk := r29aServer(t, true)
		for i := 0; i < R; i++ {
			r29aFetch(led, i, int64(4096+i))
		}
		clk.now = ports.Time(2 * bbStatusHour)
		b, present, raw := r29aBlock(t, s)
		if !present {
			t.Fatalf("bBootstrap absent at R = %d: %s", R, raw)
		}
		return b
	}

	below := block(credit.BBootstrapMinRequesters - 1)
	if below["suppressed"] != true {
		t.Fatalf("suppressed = %v at %d requesters, one below R_min = %d", below["suppressed"], credit.BBootstrapMinRequesters-1, credit.BBootstrapMinRequesters)
	}
	for _, k := range []string{"cells", "aged", "requesters", "unstamped", "maxOccupiedAgeEdgeNanos"} {
		if v, present := below[k]; present && v != nil {
			t.Fatalf("the key %q is on the wire below the minimum-requester floor, with value %v. `requesters` IS the anonymity-set size: publishing it is what makes the unconditionally-published stats.bytesServed and durability.objects[].funded deltas attributable to one identity, so withholding the grid while publishing the count would close nothing", k, v)
		}
	}
	// The clock apparatus survives: an operator must be able to tell "below the floor"
	// from "no clock injected", and needs the corruption flags either way.
	if below["clockSource"] != "injected" || below["monotonicSource"] != "injected" {
		t.Fatalf("suppression ate the clock self-report: %v / %v", below["clockSource"], below["monotonicSource"])
	}
	if below["ageAxisLive"] != true {
		t.Fatalf("ageAxisLive = %v under suppression: a suppressed block and a dead-clock block must stay distinguishable", below["ageAxisLive"])
	}

	at := block(credit.BBootstrapMinRequesters)
	if at["suppressed"] != false {
		t.Fatalf("suppressed = %v at exactly R_min = %d — the rule is >= R_min, not > R_min", at["suppressed"], credit.BBootstrapMinRequesters)
	}
	if at["requesters"] != float64(credit.BBootstrapMinRequesters) || at["aged"] != float64(credit.BBootstrapMinRequesters) {
		t.Fatalf("at R_min the wire carries requesters %v / aged %v, want %d / %d", at["requesters"], at["aged"], credit.BBootstrapMinRequesters, credit.BBootstrapMinRequesters)
	}
	if at["cells"] == nil {
		t.Fatalf("at R_min the cell grid is null")
	}
	// A legitimate zero still publishes: "the instrument is on and nobody fetched" and
	// "the census is below the floor" are different objects.
	s, _, _ := r29aServer(t, true)
	idle, present, raw := r29aBlock(t, s)
	if !present {
		t.Fatalf("bBootstrap absent on an idle node")
	}
	if idle["suppressed"] != true {
		t.Fatalf("an EMPTY census is below the floor too: suppressed = %v; %s", idle["suppressed"], raw)
	}
}

// --- BB-16: the polling oracle -------------------------------------------------------

// bbCellGrid is one snapshot's cell grid, decoded from the wire block. A nil grid (the
// block is suppressed, or the clock is dead) is an empty map: nothing to difference.
func bbCellGrid(block map[string]any) map[[2]int]int64 {
	out := map[[2]int]int64{}
	rows, ok := block["cells"].([]any)
	if !ok {
		return out
	}
	for a, row := range rows {
		for b, n := range row.([]any) {
			if v := int64(n.(float64)); v != 0 {
				out[[2]int{a, b}] = v
			}
		}
	}
	return out
}

// bbTrajectorySteps is THE ORACLE. Given a sequence of cell grids from consecutive
// polls, it returns every single-identity bin transition the sequence discloses: a delta
// of exactly one −1 at one cell and exactly one +1 at another, with nothing else moving,
// is one identity crossing a bin edge between two polls.
//
// This is the PE's measured refutation, mechanised. Against a polled series the
// counts-only argument fails at ANY R — R governs ATTRIBUTION, not extraction — so the
// oracle is deliberately blind to R and reads only the deltas.
func bbTrajectorySteps(seq []map[[2]int]int64) [][2][2]int {
	var steps [][2][2]int
	for i := 1; i < len(seq); i++ {
		delta := map[[2]int]int64{}
		for c, n := range seq[i] {
			delta[c] += n
		}
		for c, n := range seq[i-1] {
			delta[c] -= n
		}
		var down, up [][2]int
		moved := 0
		for c, d := range delta {
			switch {
			case d == 0:
			case d == -1:
				down = append(down, c)
				moved++
			case d == 1:
				up = append(up, c)
				moved++
			default:
				moved += 2 // an ambiguous move: not a clean single-identity step
			}
		}
		if moved == 2 && len(down) == 1 && len(up) == 1 {
			steps = append(steps, [2][2]int{down[0], up[0]})
		}
	}
	return steps
}

// TestR29aBB16PolledSeriesRevealsNoSingleIdentityTrajectory is BB-16: the refutation,
// encoded as a permanent gate.
//
// THE MECHANISM IT GUARDS. The prior certification claimed "deltas are cell counts;
// nothing to match". Measured, that is false: six polls of ONE requester produced the bin
// sequence 80 → 84 → 86 → 88 → 89 → 90, a per-identity byte trajectory read straight off
// a counts-only histogram. The floor does not make that pattern unextractable — nothing
// in this instrument can, and R-BB-DELTA-TRAJECTORY stays OPEN and bounded by poll rate
// — but at a degenerate anonymity set it withholds the grid entirely, so the singleton's
// trajectory is not published at all.
//
// THE POSITIVE CONTROL IS IN THE TEST, AND IT MOVED — it is now stronger and it is on
// the wire. The raw census no longer leaves core/credit (M-2), so the control can no
// longer be "the operator's unfloored local read". Instead the SAME fixture is run with
// the census PADDED over the floor: nine extra identities, one fetch each, which is
// exactly what an observer that can fetch buys for nine keypairs and about 576 KiB
// (R-BB-CENSUS-SYBIL-PAD). With the floor lifted, the one moving identity's trajectory
// is readable straight off the PUBLISHED series. That is the reviewer's measured
// refutation, encoded: it proves the oracle can fire on this exact wire path, and it
// records in a permanent gate that the floor is bought out by a fetching adversary.
func TestR29aBB16PolledSeriesRevealsNoSingleIdentityTrajectory(t *testing.T) {
	s, led, clk := r29aServer(t, true)
	clk.now = ports.Time(2 * bbStatusHour) // a fixed instant: only the BYTES move

	// ONE requester, six fetches, a poll between each. The cumulative totals walk the
	// quarter-log2 axis one or two bins at a time, which is the regime the measurement
	// was taken in.
	const requester = 7
	steps := []int64{1 << 20, 1 << 20, 1 << 20, 1 << 20, 2 << 20, 2 << 20}

	poll := func(s *uiServer, led *credit.Ledger, into *[]map[[2]int]int64, raws *[]string) {
		t.Helper()
		for _, add := range steps {
			r29aFetch(led, requester, add)
			block, present, raw := r29aBlock(t, s)
			if !present {
				t.Fatalf("bBootstrap absent")
			}
			*into = append(*into, bbCellGrid(block))
			*raws = append(*raws, raw)
		}
	}

	var wire []map[[2]int]int64
	var rawBlocks []string
	poll(s, led, &wire, &rawBlocks)

	// THE CONTROL, on the wire: the SAME six fetches by the SAME single identity, on a
	// node whose census has been PADDED over the floor. Nine keypairs and one fetch
	// each is the whole price (R-BB-CENSUS-SYBIL-PAD), so the pad is a fixture in eight
	// lines and an attack in seconds. The pads sit still, so every delta in the series
	// belongs to the one moving identity and the oracle must find its trajectory.
	padS, padLed, padClk := r29aServer(t, true)
	padClk.now = ports.Time(2 * bbStatusHour)
	for i := 0; i < credit.BBootstrapMinRequesters-1; i++ {
		r29aFetch(padLed, 500+i, int64(1)<<uint(i+3))
	}
	var padded []map[[2]int]int64
	var paddedRaw []string
	poll(padS, padLed, &padded, &paddedRaw)

	control := bbTrajectorySteps(padded)
	if len(control) < 4 {
		t.Fatalf("the trajectory oracle found only %d single-identity steps in the PADDED, PUBLISHED series. It must find the trajectory there, or its silence on the unpadded wire series means nothing. Padded blocks: %v", len(control), paddedRaw)
	}

	// THE GATE: the published sequence discloses no step at all.
	if got := bbTrajectorySteps(wire); len(got) != 0 {
		t.Fatalf("the polled wire sequence discloses %d single-identity bin transitions %v: one requester's byte trajectory is readable off the published cell deltas. The minimum-requester floor (G-BB-11) must withhold the grid at a degenerate anonymity set", len(got), got)
	}
	// Stronger, and simpler to reason about: at one requester the published block does
	// not move AT ALL across six polls, so there is no delta of any kind to difference.
	for i := 1; i < len(rawBlocks); i++ {
		if rawBlocks[i] != rawBlocks[0] {
			t.Fatalf("poll %d differs from poll 0 while a single requester fetched:\n  %s\n  %s", i, rawBlocks[0], rawBlocks[i])
		}
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(rawBlocks[0]), &probe); err != nil {
		t.Fatal(err)
	}
	if probe["suppressed"] != true {
		t.Fatalf("the block is not suppressed at one requester: %s", rawBlocks[0])
	}
	t.Logf("BB-16: at one requester the published series discloses 0 single-identity bin transitions and is byte-identical across all six polls; with the census padded over the floor by %d identities the SAME published path discloses %d — the floor is bought out by an observer that can fetch (R-BB-CENSUS-SYBIL-PAD)",
		credit.BBootstrapMinRequesters-1, len(control))
}
