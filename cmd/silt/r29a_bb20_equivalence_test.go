//go:build bbootstrap

package main

// R2.9a RE-CERT — BB-20, the G-BB-11′ EQUIVALENCE GATE, asserted AT THE WIRE.
//
// From RESEARCH CERTIFICATION
// R2.9a-minR-floor-RECERT-sybil-pad-and-estimand-steerability (2026-09-05) §5.3, §5.4.
//
// THE PROPERTY, and why it replaced a field list. G-BB-11 named three fields; the build
// widened it to five; a two-file source gate claimed the rest. All three enumerations
// missed a field, in one pull request — `ageExceedsUptime` (census-derived, published
// below the floor) and one of `clockStepBack`'s two arms. A list cannot cover fields
// nobody foresaw, so the rule is now a PROPERTY over a partition of the published block:
//
//   - INSTRUMENT field: its value is a function of the injected clock sources, their
//     injection instants and the compiled axis constants ALONE.
//   - CENSUS field: its value depends on the contents of l.accounts or l.order.
//
// Below R_min the published block MUST be a function of the instrument fields alone,
// with exactly ONE named exemption — `suppressed` itself, whose information content is
// precisely "the census is below R_min". That is a published upper bound of R_min − 1 on
// the anonymity set (R-BB-SUPPRESSED-IS-A-DISCLOSURE): a disclosure, carried in the
// owner's brief rather than hidden.
//
// Stated so a test can run it: FOR ANY TWO LEDGER STATES BELOW THE FLOOR SHARING THE
// SAME CLOCK STATE, THE PUBLISHED BLOCK IS BYTE-IDENTICAL.
//
// It is asserted on the JSON `/api/status` actually emits, not at an internal seam,
// because the seam is where the last two enumerations were checked and where the misses
// got through. A future census-derived field reaching the wire by ANY route reddens this
// gate without anyone having named the field.
//
// FIXTURE RULES (the reviewer's F-3, which is correct). Every arm of a group replays the
// SAME clock script, so clock state is equal BY CONSTRUCTION — unlike BB-16's
// byte-identity arm, which holds only because its fixture pins clk.now and must not be
// cited as a production property. And r29aServer sets uiServer.peerCount, a nil func in
// the economyServer fixture that segfaults /api/status without it.

import (
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// bb20Tick is one instant in a clock script. Exactly one of setNow / step is used:
// setNow places the wall clock at an absolute instant, step moves the wall clock while
// leaving the monotone source where it was, which is an NTP step.
type bb20Tick struct {
	setNow int64
	step   int64
}

// bb20Fetch injects `count` DISTINCT requesters, each with `bytes` fetched, at the
// instant `at` (an index into the group's clock script).
type bb20Fetch struct {
	at    int
	count int
	bytes int64
}

// bb20Census is one ledger state: a name and the fetches that build it. Every census in
// a group replays the same clock script; only the fetches differ.
type bb20Census struct {
	name    string
	fetches []bb20Fetch
}

// bb20Group is one clock script plus the censuses to compare under it.
type bb20Group struct {
	name    string
	clock   []bb20Tick
	because string // the field this group exists to discriminate
	census  []bb20Census
}

// bb20Render replays a group's clock script against a fresh server, injecting the arm's
// fetches at the matching instants, and returns the raw `bBootstrap` JSON off the wire.
func bb20Render(t *testing.T, g bb20Group, c bb20Census) (string, map[string]any) {
	t.Helper()
	s, led, clk := r29aServer(t, true)
	for i, tick := range g.clock {
		if tick.step != 0 {
			clk.step(tick.step)
		} else {
			clk.now = ports.Time(tick.setNow)
		}
		for _, f := range c.fetches {
			if f.at != i {
				continue
			}
			for k := 0; k < f.count; k++ {
				r29aFetch(led, 1000*(i+1)+k, f.bytes)
			}
		}
	}
	block, present, raw := r29aBlock(t, s)
	if !present {
		t.Fatalf("BB-20 fixture %s/%s: the bBootstrap block is absent from /api/status", g.name, c.name)
	}
	if block["clockSource"] != "injected" || block["monotonicSource"] != "injected" {
		t.Fatalf("BB-20 fixture %s/%s did not inject both clock sources (%v / %v): the comparison would be between two dead instruments",
			g.name, c.name, block["clockSource"], block["monotonicSource"])
	}
	return raw, block
}

// bb20Groups are the clock scripts and the below-floor censuses compared under each.
// Shared with the positive control below, which drives the SAME scripts above the floor.
func bb20Groups() []bb20Group {
	const hour = bbStatusHour
	const day = 24 * hour
	return []bb20Group{
		{
			name: "ages-and-bytes",
			// Four instants on a clean wall clock, ending at 8 hours.
			clock:   []bb20Tick{{setNow: hour}, {setNow: 3 * hour}, {setNow: 6*hour + 1}, {setNow: 8 * hour}},
			because: "cells, aged, requesters, unstamped and maxOccupiedAgeEdgeNanos",
			census: []bb20Census{
				{name: "empty"},
				{name: "one-young", fetches: []bb20Fetch{{at: 3, count: 1, bytes: 250}}},
				{name: "one-old", fetches: []bb20Fetch{{at: 0, count: 1, bytes: 1 << 30}}},
				{name: "nine-mixed", fetches: []bb20Fetch{
					{at: 0, count: 3, bytes: 4096},
					{at: 1, count: 3, bytes: 1 << 20},
					{at: 2, count: 3, bytes: 1 << 34},
				}},
			},
		},
		{
			name: "forward-step",
			// Register at zero, then an 8-day FORWARD wall step: ages inflate past a
			// monotone bound that did not move, which is what AgeExceedsUptime reads.
			clock:   []bb20Tick{{setNow: 0}, {step: 8 * day}},
			because: "ageExceedsUptime — a THRESHOLD on the very field the floor withholds",
			census: []bb20Census{
				{name: "empty"},
				{name: "one", fetches: []bb20Fetch{{at: 0, count: 1, bytes: 4096}}},
				{name: "nine", fetches: []bb20Fetch{{at: 0, count: 9, bytes: 4096}}},
			},
		},
		{
			name: "backward-step-past-a-stamp",
			// Register at 5 hours, then step the wall clock BACK four hours. The wall
			// delta from the ledger's own start stays positive, so the census-free arm
			// of clockStepBack cannot fire; but every stamp now reads as being in the
			// future and its age is clamped, which is the CENSUS arm.
			clock:   []bb20Tick{{setNow: 5 * hour}, {step: -4 * hour}},
			because: "clockStepBack's census arm (an account's age clamped to zero)",
			census: []bb20Census{
				{name: "empty"},
				{name: "one", fetches: []bb20Fetch{{at: 0, count: 1, bytes: 8192}}},
				{name: "nine", fetches: []bb20Fetch{{at: 0, count: 9, bytes: 8192}}},
			},
		},
	}
}

// TestR29aBB20BelowFloorBlockIsAFunctionOfTheClockAlone is BB-20.
//
// RUNTIME GATE: this test IS the runtime cover for G-BB-11′. It observes published
// bytes and reads no source text.
func TestR29aBB20BelowFloorBlockIsAFunctionOfTheClockAlone(t *testing.T) {
	for _, g := range bb20Groups() {
		g := g
		t.Run(g.name, func(t *testing.T) {
			if len(g.census) < 2 {
				t.Fatalf("group %s has %d census arms: an equivalence gate needs at least two", g.name, len(g.census))
			}
			base, baseBlock := bb20Render(t, g, g.census[0])
			if baseBlock["suppressed"] != true {
				t.Fatalf("group %s arm %q is NOT below the floor: the gate compares below-floor states only, so this arm proves nothing. Raw: %s", g.name, g.census[0].name, base)
			}
			for _, c := range g.census[1:] {
				got, block := bb20Render(t, g, c)
				if block["suppressed"] != true {
					t.Fatalf("group %s arm %q is NOT below the floor (R_min = %d): raw %s", g.name, c.name, credit.BBootstrapMinRequesters, got)
				}
				if got == base {
					continue
				}
				t.Fatalf("BB-20 (G-BB-11′) FAILED in group %q, which discriminates %s.\n"+
					"Two ledger states BELOW the floor with IDENTICAL clock state published DIFFERENT bytes,\n"+
					"so some published field is a function of the census — which is exactly what the floor\n"+
					"withholds. The rule is a PROPERTY, not a field list: below R_min the block must be a\n"+
					"function of the INSTRUMENT fields alone (the injected clock sources, their injection\n"+
					"instants and the compiled axis constants), with one named exemption, `suppressed`.\n"+
					"Withhold the offending field below the floor, or split it into a census arm and an\n"+
					"instrument arm and withhold only the census arm.\n"+
					"  census %q: %s\n  census %q: %s",
					g.name, g.because, g.census[0].name, base, c.name, got)
			}
		})
	}
}

// TestR29aBB20HasTeeth is the POSITIVE CONTROL, and it is the reason a green BB-20 means
// anything. A gate that asserts "these two blocks are equal" is vacuous if the fixture
// could never make them differ.
//
// It replays each group's clock script with censuses AT and ABOVE the floor, where the
// same census differences are published, and requires the blocks to DIFFER. If they did
// not, the arms compared by BB-20 were identical before the floor ever ran.
//
// RUNTIME GATE: TestR29aBB20BelowFloorBlockIsAFunctionOfTheClockAlone.
func TestR29aBB20HasTeeth(t *testing.T) {
	const R = credit.BBootstrapMinRequesters
	for _, g := range bb20Groups() {
		g := g
		t.Run(g.name, func(t *testing.T) {
			// Two censuses that are both ABOVE the floor and differ in exactly the way
			// the below-floor arms of this group differ: how many identities fetched,
			// how big, and at which instant.
			lo := bb20Census{name: "at-floor", fetches: []bb20Fetch{{at: 0, count: R, bytes: 4096}}}
			hi := bb20Census{name: "above-floor-different-shape", fetches: []bb20Fetch{
				{at: 0, count: R, bytes: 4096},
				{at: len(g.clock) - 1, count: 3, bytes: 1 << 34},
			}}
			a, blockA := bb20Render(t, g, lo)
			b, _ := bb20Render(t, g, hi)
			if blockA["suppressed"] != false {
				t.Fatalf("the control arm is suppressed at R = %d: it must be ABOVE the floor or it proves nothing. Raw: %s", R, a)
			}
			if a == b {
				t.Fatalf("POSITIVE CONTROL FAILED for group %q: two DIFFERENT censuses published byte-identical blocks ABOVE the floor, so BB-20's byte-identity below the floor is a property of the fixture and not of the floor.\n  %s\n  %s", g.name, a, b)
			}
		})
	}
}
