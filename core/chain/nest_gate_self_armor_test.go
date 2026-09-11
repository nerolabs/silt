package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R-NEST-GATE — "evidence self-armor" (Tester, 2026-09-10; DRIVEN, not derived).
//
// CLAIM DRIVEN HERE: a lone equivocating proposer can make its own double-sign
// unslashable, with no bond, no coalition, and no misconfiguration, by packing its own
// blocks with junk-but-VALID equivocation proofs so a legitimate proof about those
// blocks exceeds chain.SlashesBytesCap. Routed by the blind PE ruling
// RULING-slashcap-config-route-close-CODE-2026-09-10.md and certified (Q1, "no (cap,
// body-bound) pair closes it") by
// SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md, both in
// silt-agent-memory/. That certification's §2.2 names the standing theorem T-NEST: evidence
// that embeds the object it accuses, committed inside an object of the same kind under a
// shared size bound, has NO satisfying bound — the recursion is cut only by making the
// carried quantity independent of the accused body's size (owner call A, routed to D1).
//
// MECHANISM (verified at these sites): CheckEquivocation (equivocation.go:68) checks
// only culprit key SIZE, not-pruned, equal Height, differing bodyHash, matching era, and
// a shared (round, phase) scope — never that the culprit is bonded, that the evidence
// blocks belong to this chain, or that the proof is a duplicate. Equivocation carries two
// FULL Blocks (equivocation.go:24-28); a Block carries its own Slashes field
// (chain.go:522-528), bounded only by SlashesBytesCap, and Prune() never recurses into
// Slashes (chain.go:858-873) so a committed proof stays resident forever.
//
// MEASURED (this file; core/chain, go1.26, -short skipped for the two below marked so):
//  1. One throwaway-key proof about two never-committed blocks, ACCEPTED by
//     CheckEquivocation: 687 B (TestRNestGate_JunkKeyProofIsAcceptedEvidence).
//  2. A block packed with 24,456 such proofs to 16,776,819 B of Slashes (397 B under the
//     16,777,216 B cap) is VALID under both v5ValidateSlashes and the full
//     w.c.ValidateProposal path — shipped DefaultConfig, no misconfiguration
//     (TestRNestGate_SelfArmorMeasurement, Q2).
//  3. A LEGITIMATE equivocation proof about that armored block (one real bonded key,
//     two differing bodies at its height) measures 33,555,157 B — 16,777,941 B (>2x)
//     over the cap, and v5ValidateSlashes REJECTS it with ErrSlashesBytesCapExceeded
//     (Q3). The double-signer keeps its seat.
//  4. ARMOR THRESHOLD, measured by binary search over junk-proof count (not derived):
//     the legitimate proof first exceeds the cap between 12,227 junk proofs/side
//     (8,387,725 B = 7.9992 MiB, still admissible: 247 B of cap headroom left in the
//     legitimate proof) and 12,228 junk proofs/side (8,388,411 B = 7.9998 MiB, legitimate
//     proof exceeds cap by 1,125 B). **This CORRECTS the research certification's
//     arithmetic figure of ~8.39 MiB per side** (SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-
//     RESEARCH-CERTIFICATION-2026-09-10.md §2.2, "the armor threshold is |b| >
//     (κ − c)/2 ... at κ = 16 MiB that is |b| > 8.39 MiB per side") — that seat had no
//     shell and flagged the number as not-yet-driven. The driven threshold is
//     essentially SlashesBytesCap/2 minus the small per-block header/Atts/wrapper
//     overhead (~1.5 KB observed), i.e. ~7.9998 MiB, not 8.39 MiB. 8.39 MiB overstates
//     the attacker's cost by roughly 409 KB (~5%); do not cite 8.39 MiB as measured.
//
// WHY THIS IS A REPORTING TEST, NOT A HARD GATE (and does not leave CI red): the
// certification's own Q2 finding is that no admissible (cap, body-bound) pair closes
// this — a validity-rule change here would need to be either the certified route (C)
// (put (height,round,phase) in the v5 consensus-signature preimage so evidence is O(1),
// a FORMAT change gated to D1/the freeze per .claude/CLAUDE.md's research gate) or
// something equally consensus-rule-shaped, neither of which the Tester may build. So
// TestRNestGate_SelfArmorMeasurement asserts the STRUCTURAL facts that make the break
// reachable (hard: Q1 acceptance, Q2 validity) and only REPORTS the byte comparison in
// Q3/Q4 via t.Logf — asserting "the legitimate proof must fit under cap" would redden
// CI (ci.yml's -short job, and release.yml's full `go test ./...`) on a break this file
// does not fix, which is the "do not write a gate that passes for the wrong reason"
// failure in the other direction. It IS skipped under -short (matching the repo's
// existing G-3 measurement-harness convention, core/chain/r06_slashes_cap_cost_test.go)
// because building the near-cap Slashes field takes real wall time; run it by name:
//
//	go test ./core/chain/ -run TestRNestGate_SelfArmorMeasurement -v -count=1
//
// WHAT WOULD TURN THIS INTO A REAL GATE: once route (C) or an equivalent fixed-size
// evidence format ships (owner call A, D1 manifest item per the research certification
// §9), change the t.Logf calls below for the Q3/Q4 over-cap checks to t.Fatalf — a
// legitimate proof about a maximally-packed block must then always fit under
// SlashesBytesCap, at any Slashes fill level, because evidence size no longer scales
// with the accused body. Until then this file's job is to keep the four numbers true
// and catch the day the construction itself stops working (Q1/Q2 go RED — see their own
// doc comments for what that would mean).

// junkEquivocation builds a self-verifying (CheckEquivocation-accepted) era-1 double-sign
// proof for a throwaway ed25519 key that this chain has never seen, over two blocks that
// were never proposed, never gossiped, and never committed. seed selects both the key and
// the one differing Entry byte between the two conflicting bodies.
func junkEquivocation(g *Block, seed int64) Equivocation {
	culprit := key(seed + 90000) // offset clear of newWorld's seeds 1..5 and other fixtures' seed ranges
	a := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(byte(seed))}}
	Sign(a, culprit)
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(byte(seed + 1))}}
	Sign(b, culprit)
	return Equivocation{Culprit: pubOf(culprit), A: *a, B: *b}
}

// TestRNestGate_JunkKeyProofIsAcceptedEvidence pins the ROOT CAUSE, not the break itself:
// CheckEquivocation validates a double-sign proof for a throwaway key this chain never
// bonded, never registered, and never saw propose or attest anything real — the culprit
// need not be a validator at all (equivocation.go:68, "verified at source" per the
// research certification §6.1 T1). This is what makes the self-armor construction FREE
// to build (no bond, no coalition). It is cheap and always runs (not -short-gated).
//
// If this ever goes RED, CheckEquivocation gained a bonded/known-validator check on the
// culprit. That would close the T1 (lone-proposer, zero-cost) variant of this residual —
// good news — but would NOT close R-NEST-GATE itself: T0 (armoring with two GENUINE
// committed proofs from an honest chain's own pendingSlashes backlog, certification
// §6.1) needs no throwaway key at all. Re-derive this file's measurements either way;
// do not read a RED here as the residual closed.
func TestRNestGate_JunkKeyProofIsAcceptedEvidence(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()

	e := junkEquivocation(g, 1)
	if err := CheckEquivocation(&e); err != nil {
		t.Fatalf("ROOT CAUSE GONE: CheckEquivocation now REFUSES a throwaway-key proof about "+
			"two never-committed blocks (%v). The T1 self-armor variant (no bond, no coalition) "+
			"is closed; R-NEST-GATE's T0 variant (two genuine committed proofs) is not — see this "+
			"test's doc comment. Update SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-"+
			"2026-09-10.md §6.1 and re-run TestRNestGate_SelfArmorMeasurement.", err)
	}
	if n := SlashesEncodedSize([]Equivocation{e}); n <= 0 {
		t.Fatalf("GATE VACUOUS: accepted junk proof encodes to %d B", n)
	}
}

// TestRNestGate_SelfArmorMeasurement drives Q2–Q4 of the self-armor claim. See the
// package-level doc comment above for the full reproduction, the four measured numbers,
// and why the over-cap comparisons are reported (t.Logf) rather than asserted.
func TestRNestGate_SelfArmorMeasurement(t *testing.T) {
	if testing.Short() {
		t.Skip("R-NEST-GATE measurement harness (OPEN break, owner: Tester, closer: route (C) " +
			"fixed-size evidence at D1); run by name: go test ./core/chain/ -run TestRNestGate_SelfArmorMeasurement -v -count=1")
	}
	w := newWorld(DefaultConfig())
	g := w.genesis()

	// --- Q2: pack a block's Slashes to just under SlashesBytesCap with junk-key proofs. ---
	// Calibrated build (not an O(n) append-and-remeasure loop, which is O(n^2) in the encoder
	// at this proof count — ~24,456 elements): measure the marginal per-element cost from two
	// small marshals, compute the target count directly, then nudge by at most a few marshals.
	one := junkEquivocation(g, 1)
	oneSize := SlashesEncodedSize([]Equivocation{one})
	twoSize := SlashesEncodedSize([]Equivocation{one, one})
	marginal := twoSize - oneSize
	if marginal <= 0 {
		t.Fatalf("calibration: two-element marshal (%d B) not larger than one-element (%d B)", twoSize, oneSize)
	}
	overhead := oneSize - marginal
	target := (SlashesBytesCap - overhead) / marginal
	slashes := make([]Equivocation, target)
	for i := range slashes {
		slashes[i] = one
	}
	for SlashesEncodedSize(slashes) > SlashesBytesCap {
		slashes = slashes[:len(slashes)-1]
	}
	for SlashesEncodedSize(append(slashes, one)) <= SlashesBytesCap {
		slashes = append(slashes, one)
	}
	fillSize := SlashesEncodedSize(slashes)
	t.Logf("Q2: %d junk-key proofs packed into Slashes, encoded %d B (cap %d B, %d B headroom)",
		len(slashes), fillSize, SlashesBytesCap, SlashesBytesCap-fillSize)

	prev, height := w.c.Head()
	armored := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(200)}, Slashes: slashes}
	Sign(armored, w.prop)
	w.attestAll(armored)

	if outcome, err := v5ValidateSlashes(armored); outcome != Accept || err != nil {
		t.Fatalf("Q2 REGRESSED: the armored block (Slashes at %d B, %d proofs, shipped DefaultConfig, "+
			"no misconfiguration) is no longer valid under v5ValidateSlashes: outcome=%v err=%v. Either "+
			"the construction is stale (re-derive) or a validity rule changed — check whether it is the "+
			"certified route (C) fixed-size-evidence close (good) or an ad hoc body bound (the research "+
			"certification's §3 REFUTED-in-the-unsafe-direction finding: a body bound HALVES the "+
			"attacker's price, it does not raise it).", fillSize, len(slashes), outcome, err)
	}
	if err := w.c.ValidateProposal(armored); err != nil {
		t.Fatalf("Q2 REGRESSED: the armored block is valid under v5ValidateSlashes but the full "+
			"ValidateProposal path now refuses it: %v", err)
	}
	t.Logf("Q2: armored block VALID under v5ValidateSlashes and w.c.ValidateProposal (shipped DefaultConfig)")

	// --- Q3: a LEGITIMATE proof about the armored block (one real bonded key, two differing
	// bodies at its height, both carrying the armor). ---
	realKey := w.vals[0]
	legitA := *armored
	legitA.Entries = []ports.Entry{entry(201)}
	Sign(&legitA, realKey)
	legitB := *armored
	legitB.Entries = []ports.Entry{entry(202)}
	Sign(&legitB, realKey)
	legit := Equivocation{Culprit: pubOf(realKey), A: legitA, B: legitB}
	if err := CheckEquivocation(&legit); err != nil {
		t.Fatalf("Q3: a genuine double-sign by a real bonded key over two differing bodies at one "+
			"height is not recognized as equivocation: %v", err)
	}
	legitSize := SlashesEncodedSize([]Equivocation{legit})
	over := legitSize - SlashesBytesCap
	t.Logf("Q3: legitimate proof about the armored block = %d B; cap = %d B; over by %d B (%.2f MiB)",
		legitSize, SlashesBytesCap, over, float64(over)/(1<<20))
	if over > 0 {
		t.Logf("Q3 OPEN BREAK: the legitimate proof exceeds SlashesBytesCap and is REJECTED by the " +
			"shipped validity path (verified below) — the equivocator keeps its seat. This is reported, " +
			"not asserted; see the package doc comment for why.")
	} else {
		t.Logf("Q3: legitimate proof now FITS under cap at this fill level — the construction as built " +
			"no longer reproduces the break at Q2's fill level; re-run at other fill levels before " +
			"concluding the residual is closed.")
	}
	slashBlock := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(250)}, Slashes: []Equivocation{legit}}
	Sign(slashBlock, w.prop)
	w.attestAll(slashBlock)
	outcome, verr := v5ValidateSlashes(slashBlock)
	t.Logf("Q3: v5ValidateSlashes(block carrying the legitimate proof) outcome=%v err=%v", outcome, verr)
	if over > 0 && (outcome == Accept || !errors.Is(verr, ErrSlashesBytesCapExceeded)) {
		t.Fatalf("Q3 INCONSISTENT: the legitimate proof measures %d B over cap by %d B, but "+
			"v5ValidateSlashes did not reject it with ErrSlashesBytesCapExceeded (outcome=%v err=%v) — "+
			"the byte accounting above disagrees with the shipped validity path; investigate before "+
			"trusting either number.", legitSize, over, outcome, verr)
	}

	// --- Q4: the ARMOR THRESHOLD, by binary search (not arithmetic) over the number of
	// junk-key proofs packed per side. ---
	base := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(90)}}
	w.attestAll(base)
	legitSizeForCount := func(k int) (int, []Equivocation) {
		s := make([]Equivocation, k)
		for i := range s {
			s[i] = one
		}
		a2 := *base
		a2.Slashes = s
		a2.Entries = []ports.Entry{entry(91)}
		Sign(&a2, w.prop)
		b2 := *base
		b2.Slashes = s
		b2.Entries = []ports.Entry{entry(92)}
		Sign(&b2, w.prop)
		return SlashesEncodedSize([]Equivocation{{Culprit: pubOf(w.prop), A: a2, B: b2}}), s
	}
	lo, hi := 0, len(slashes)
	if hiSize, _ := legitSizeForCount(hi); hiSize <= SlashesBytesCap {
		t.Fatalf("Q4 precondition: hi=%d junk proofs/side (Q2's own fill count) must already push "+
			"the legitimate proof over cap (got %d B, cap %d B) — Q2 and Q4 disagree", hi, hiSize, SlashesBytesCap)
	}
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if midSize, _ := legitSizeForCount(mid); midSize > SlashesBytesCap {
			hi = mid
		} else {
			lo = mid
		}
	}
	loLegit, loSlashes := legitSizeForCount(lo)
	hiLegit, hiSlashes := legitSizeForCount(hi)
	loSide, hiSide := SlashesEncodedSize(loSlashes), SlashesEncodedSize(hiSlashes)
	t.Logf("Q4 ARMOR THRESHOLD: %d junk proofs/side (%d B = %.4f MiB per side) keeps the legitimate "+
		"proof admissible (%d B, %d B of cap headroom); %d junk proofs/side (%d B = %.4f MiB per side) "+
		"pushes it over cap (%d B, %d B over). Compare to the research certification's arithmetic "+
		"figure of ~8.39 MiB per side (SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-"+
		"2026-09-10.md §2.2) — the measured threshold here is ~%.4f MiB, not 8.39 MiB.",
		lo, loSide, float64(loSide)/(1<<20), loLegit, SlashesBytesCap-loLegit,
		hi, hiSide, float64(hiSide)/(1<<20), hiLegit, hiLegit-SlashesBytesCap,
		float64(hiSide)/(1<<20))
}
