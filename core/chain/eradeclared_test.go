package chain

import (
	"crypto/ed25519"
	"strings"
	"testing"
)

// THE DECLARED-ERA GATES (freeze manifest item 19, second clause: "the daemon prints its declared
// max block era at start-up"). The first clause — the era a chain OBSERVES — shipped in #808 and is
// gated in erastate_test.go. These gates cover the other number and, above all, the PAIR.
//
// WHY THE PAIR IS THE DELIVERABLE. Cloud row 13b-delivery-settlement must separate "era-4 is dark"
// (the chain has not activated; the binary is fine) from "the issuer's keys are off-commitment"
// (something else is wrong). Neither number answers that alone. A chain reporting no v5 block is a
// HEALTHY dark network under a build that declares v5 and a WRONG BUILD under one that does not,
// and the two are the same observed chain. So every gate below asserts a DIFFERENCE between two
// renders, never the mere presence of a line.
//
// declaredIsACompileTimeConstant is the structural half of GATE G-DE-2, and it asserts at COMPILE
// time, which is the only place this particular claim can be asserted without qualification: a
// const declaration accepts a constant expression and nothing else. If DeclaredMaxBlockVersion ever
// becomes a var, a field, a flag or a function call, this package stops building. A runtime
// assertion could not tell those apart from a constant that happens to hold the same value today.
const declaredIsACompileTimeConstant = DeclaredMaxBlockVersion

// TestDeclaredMaxIsTheBINARYsRealCeiling is GATE G-DE-1.
//
// The declaration is a CLAIM ABOUT THIS BUILD, and a claim is worth exactly what checks it. A
// constant that merely reads 5 would keep reading 5 after someone widened versionSupported to 6 —
// the daemon would then declare an era it is not the top of, and 13b's discrimination would invert
// silently. So the constant is checked against the two predicates that define the ceiling, both
// DRIVEN rather than read: the decode ceiling (versionSupported) and the mint ceiling (MintVersion
// on a chain whose own readiness tally activated era-4).
func TestDeclaredMaxIsTheBINARYsRealCeiling(t *testing.T) {
	// --- The DECODE ceiling. Both sides of it, because only the pair locates a ceiling: the
	// first alone is satisfied by any supported version, the second by any unsupported one.
	if !versionSupported(DeclaredMaxBlockVersion) {
		t.Fatalf("G-DE-1 RED: this build DECLARES v%d but versionSupported rejects it — the daemon "+
			"would announce an era it cannot decode", DeclaredMaxBlockVersion)
	}
	if versionSupported(DeclaredMaxBlockVersion + 1) {
		t.Fatalf("G-DE-1 RED: versionSupported accepts v%d, above the declared maximum v%d. The "+
			"declaration is STALE: someone widened the decode ceiling without moving it, so the "+
			"start-up line now understates what this binary is.",
			DeclaredMaxBlockVersion+1, DeclaredMaxBlockVersion)
	}

	// --- The MINT ceiling, driven. A fleet stamped 5 makes the chain latch era-4 by its own
	// readiness tally; past H_era4 the proposer must stamp exactly the declared maximum. The
	// activation height comes from the chain, never from this fixture.
	c, whale, minnows := eraProbeChain(t, BlockVersionWitnessable)
	if !c.era4LockedIn {
		t.Fatal("FIXTURE: era-4 did not lock in, so nothing below drives the mint ceiling")
	}
	for c.Len() <= int(c.era4Height) {
		mustAppend(t, c, mintNext4Carrier(t, c, append([]ed25519.PrivateKey{whale}, minnows...)))
	}
	_, next := c.Head()
	if got := c.MintVersion(next); got != DeclaredMaxBlockVersion {
		t.Fatalf("G-DE-1 RED: on an era-4-activated chain the proposer mints v%d, but this build "+
			"declares v%d. The two must be the same number.", got, DeclaredMaxBlockVersion)
	}
}

// TestStartupLinesSeparateAHealthyDarkChainFromAWrongBuild is GATE G-DE-2, the gate this half of
// item 19 exists for. It drives the four-cell matrix that 13b needs: {declared v5, declared v2} ×
// {a dark era-2 chain, an era-4 chain}.
//
// BOTH CHAINS ARE DRIVEN, not written. eraProbeChain hands the chain four signed bond registrations
// and one number — the readiness stamp — and the chain decides everything after that: whether the
// tally latches, at what height, and what version each block is minted with. The fixture never
// writes a Version, a latch or an activation height, which is the property the predecessor of this
// row lacked.
func TestStartupLinesSeparateAHealthyDarkChainFromAWrongBuild(t *testing.T) {
	// A DARK chain: a fleet stamped 3, which is what every shipped binary stamps today, driven
	// three blocks so the chain mints its own v2 history. This is the live networks' actual
	// state — an era-2 chain — and the one cloud row 13b keeps hitting.
	dark, dwhale, dminnows := eraProbeChain(t, BlockVersionRegGate)
	for i := 0; i < 3; i++ {
		mustAppend(t, dark, mintNext4Carrier(t, dark, append([]ed25519.PrivateKey{dwhale}, dminnows...)))
	}
	darkState := dark.EraState()
	if darkState.Era4.Phase != EraDark || darkState.Census.MaxVersion != BlockVersionRounds {
		t.Fatalf("FIXTURE: the dark arm is not an era-2 dark chain (phase %q, max v%d); the contrast "+
			"below would be meaningless", darkState.Era4.Phase, darkState.Census.MaxVersion)
	}

	// An ERA-4 chain: the same fixture, stamped 5, driven past its own H_era4 so the chain mints
	// real v5 blocks.
	c, whale, minnows := eraProbeChain(t, BlockVersionWitnessable)
	for c.Len() <= int(c.era4Height) {
		mustAppend(t, c, mintNext4Carrier(t, c, append([]ed25519.PrivateKey{whale}, minnows...)))
	}
	liveState := c.EraState()
	if liveState.Era4.Phase != EraActive || liveState.Census.MaxVersion != BlockVersionWitnessable {
		t.Fatalf("FIXTURE: the era-4 arm did not activate (phase %q, max v%d)",
			liveState.Era4.Phase, liveState.Census.MaxVersion)
	}

	// The four cells: {this build, an era-2 build} × {the dark chain, the era-4 chain}. The
	// era-2 build (declared max v2) is the one 13b's ambiguity is actually about — it is what a
	// pre-era-3 binary is, and what every live network is currently minting.
	darkUnderV5 := text(StartupEraLines(DeclaredMaxBlockVersion, darkState))
	darkUnderV2 := text(StartupEraLines(BlockVersionRounds, darkState))
	liveUnderV5 := text(StartupEraLines(DeclaredMaxBlockVersion, liveState))
	liveUnderV2 := text(StartupEraLines(BlockVersionRounds, liveState))
	t.Logf("dark chain, era-4 build:\n%s\n\ndark chain, era-2 build:\n%s\n\nera-4 chain, era-4 build:\n%s\n\nera-4 chain, era-2 build:\n%s",
		darkUnderV5, darkUnderV2, liveUnderV5, liveUnderV2)

	// (1) THE SAME CHAIN UNDER TWO BUILDS MUST DIFFER. If it does not, the start-up line answers
	// neither of 13b's two questions and this row has shipped its predecessor's defect again.
	if darkUnderV5 == darkUnderV2 {
		t.Fatal("G-DE-2 RED: on one dark chain, an era-4 build and an era-2 build print the SAME " +
			"start-up lines. \"era-4 is dark\" and \"the operator is running the wrong build\" are " +
			"then indistinguishable — which is precisely the sentence cloud row 13b already has.")
	}
	if liveUnderV5 == liveUnderV2 {
		t.Fatal("G-DE-2 RED: on one era-4 chain, an era-4 build and an era-2 build print the SAME " +
			"start-up lines")
	}
	// (2) THE SAME BUILD ON TWO CHAINS MUST DIFFER. The declared number alone is not an answer
	// either: a line that ignores the chain would satisfy (1) and still say nothing about 13b.
	if darkUnderV5 == liveUnderV5 {
		t.Fatal("G-DE-2 RED: one build prints the same start-up lines on a DARK chain and on an " +
			"ERA-4 chain — the observed half of the pair is not being read")
	}

	// (3) The verdicts, by name. A dark chain under the shipped build is HEALTHY and must say so;
	// an era-4 chain under an era-2 build is the WRONG BINARY and must say that.
	if got := DeclaredRelation(DeclaredMaxBlockVersion, darkState.Census); got != EraRelationAhead {
		t.Fatalf("G-DE-2 RED: declared v%d over a dark chain must be %q, got %q",
			DeclaredMaxBlockVersion, EraRelationAhead, got)
	}
	if got := DeclaredRelation(DeclaredMaxBlockVersion, liveState.Census); got != EraRelationAt {
		t.Fatalf("G-DE-2 RED: declared v%d over an era-4 chain must be %q, got %q",
			DeclaredMaxBlockVersion, EraRelationAt, got)
	}
	if got := DeclaredRelation(BlockVersionRounds, liveState.Census); got != EraRelationBehind {
		t.Fatalf("G-DE-2 RED: declared v%d over a chain carrying v%d must be %q, got %q",
			BlockVersionRounds, BlockVersionWitnessable, EraRelationBehind, got)
	}
	// The era-2 build on the era-2 chain is AT, not a defect: nothing committed there indicates
	// a higher era. Asserting it pins that "wrong build" is reserved for what is OBSERVED, and is
	// not printed over a build merely because it is old.
	if got := DeclaredRelation(BlockVersionRounds, darkState.Census); got != EraRelationAt {
		t.Fatalf("G-DE-2 RED: declared v%d over an era-2 chain must be %q, got %q",
			BlockVersionRounds, EraRelationAt, got)
	}
	for _, want := range []string{"HEALTHY", "era-4", "v5"} {
		if !strings.Contains(darkUnderV5, want) {
			t.Errorf("G-DE-2 RED: the healthy-dark verdict must contain %q; whole render:\n%s", want, darkUnderV5)
		}
	}
	for _, want := range []string{"WRONG BUILD", "CANNOT validate"} {
		if !strings.Contains(liveUnderV2, want) {
			t.Errorf("G-DE-2 RED: the behind verdict must contain %q; whole render:\n%s", want, liveUnderV2)
		}
	}
	// The dark chain under the shipped build must NOT be reported as a defect of the binary. This
	// is the false alarm the row is meant to prevent, stated as an assertion rather than as prose.
	if strings.Contains(darkUnderV5, "WRONG BUILD") {
		t.Errorf("G-DE-2 RED: a healthy dark chain was reported as a wrong build:\n%s", darkUnderV5)
	}
}

// TestStartupLinesDoNotMoveWithLocalConfig is GATE G-DE-3, the ABLATION behind "the declared era is
// a property of the BUILD".
//
// A declared era an operator can type answers nothing: it would report what someone believes rather
// than what the binary is, and an operator debugging a dark network would be reading their own
// input back. EraState already ignores Config (G-EP-2); this extends the same ablation over the
// DECLARED half and over the whole rendered text, so a Config read introduced anywhere in the new
// render — not only in the accessor — turns it red.
func TestStartupLinesDoNotMoveWithLocalConfig(t *testing.T) {
	c, _, _ := eraProbeChain(t, BlockVersionWitnessable)
	before := text(StartupEraLines(DeclaredMaxBlockVersion, c.EraState()))

	// Every era-related knob, moved to a value no tally could produce.
	c.cfg.Era3ActivationHeight = 999999
	c.cfg.Era4ActivationHeight = 999999
	c.cfg.EpochBlocks = 4096
	c.cfg.Quorum = 99
	after := text(StartupEraLines(DeclaredMaxBlockVersion, c.EraState()))

	if before != after {
		t.Fatalf("G-DE-3 RED: the start-up render MOVED when local config changed, on identical "+
			"committed blocks. The line would then answer \"what did this operator type\".\nBEFORE:\n%s\nAFTER:\n%s",
			before, after)
	}
	if strings.Contains(after, "999999") || strings.Contains(after, "4096") {
		t.Fatalf("G-DE-3 RED: a local config value appears verbatim in the render:\n%s", after)
	}
}

// TestStartupLinesRefuseToAssertAnUnobservedChain is GATE G-DE-4.
//
// A daemon whose store is empty has a real chain object and NO blocks — the state every fresh node
// is in for the instant before genesis is seeded. Printing "highest version v0" there would be the
// vacuity trap in its purest form: a zero that is indistinguishable from a measurement. The render
// must name the limit instead, and it must not claim any of the three comparison verdicts, because
// it has nothing to compare against.
//
// The chain here is REAL and its emptiness is produced, not written: New with no AppendGenesis.
func TestStartupLinesRefuseToAssertAnUnobservedChain(t *testing.T) {
	c := New(Config{Quorum: 1}, nil)
	st := c.EraState()
	if !st.Census.Empty() {
		t.Fatalf("FIXTURE: a chain with no genesis reports %d block(s)", st.Census.Blocks)
	}
	out := text(StartupEraLines(DeclaredMaxBlockVersion, st))
	t.Logf("empty chain:\n%s", out)

	if got := DeclaredRelation(DeclaredMaxBlockVersion, st.Census); got != EraRelationUnobserved {
		t.Fatalf("G-DE-4 RED: with no blocks the relation must be %q, got %q — the surface claimed "+
			"a comparison it had nothing to compare against", EraRelationUnobserved, got)
	}
	// The declaration itself is still printed: it is a fact about the build, true with or without
	// a chain. What must be absent is any verdict ABOUT a chain.
	if !strings.Contains(out, "era-4") {
		t.Fatalf("G-DE-4 RED: the declaration is a property of the build and must print even with "+
			"no chain:\n%s", out)
	}
	for _, forbidden := range []string{"AHEAD", "WRONG BUILD", "highest version v0", "at height 0"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("G-DE-4 RED: an unobserved chain rendered %q — a verdict with no evidence "+
				"under it:\n%s", forbidden, out)
		}
	}
}

// TestEveryEraRelationIsDrivenAndDistinct is GATE G-DE-5 (simplicity rule 7: a row called "safe" in
// any coverage table must be a DRIVEN probe, not an assumption).
//
// The closed set EraRelations is enumerated here rather than hand-listed, so adding a member
// without driving it fails this gate instead of passing silently. Each member must be REACHED by a
// census, must be a non-empty string (the enum has no zero value, by construction), and must render
// a verdict distinct from every other member's — two relations that print alike are one relation
// wearing two names.
func TestEveryEraRelationIsDrivenAndDistinct(t *testing.T) {
	dark, _, _ := eraProbeChain(t, BlockVersionRegGate)
	c, whale, minnows := eraProbeChain(t, BlockVersionWitnessable)
	for c.Len() <= int(c.era4Height) {
		mustAppend(t, c, mintNext4Carrier(t, c, append([]ed25519.PrivateKey{whale}, minnows...)))
	}
	empty := New(Config{Quorum: 1}, nil)

	drove := map[EraRelation]string{}
	for _, arm := range []struct {
		declared uint64
		state    EraState
	}{
		{DeclaredMaxBlockVersion, dark.EraState()},
		{DeclaredMaxBlockVersion, c.EraState()},
		{BlockVersionStateRoot, c.EraState()},
		{DeclaredMaxBlockVersion, empty.EraState()},
	} {
		rel := DeclaredRelation(arm.declared, arm.state.Census)
		drove[rel] = text(StartupEraLines(arm.declared, arm.state))
	}

	for _, r := range EraRelations {
		if r == "" {
			t.Fatal("G-DE-5 RED: a relation is the empty string — the Go zero value, which a " +
				"never-populated field would be indistinguishable from")
		}
		if _, ok := drove[r]; !ok {
			t.Fatalf("G-DE-5 RED: relation %q is in the closed set EraRelations but no arm of this "+
				"gate produced it from a real chain. An un-driven state is decoration.", r)
		}
	}
	if len(drove) != len(EraRelations) {
		t.Fatalf("G-DE-5 RED: %d relations driven, %d in the closed set", len(drove), len(EraRelations))
	}
	for a, ta := range drove {
		for b, tb := range drove {
			if a != b && ta == tb {
				t.Fatalf("G-DE-5 RED: relations %q and %q render IDENTICALLY:\n%s", a, b, ta)
			}
		}
	}
}

// text joins rendered lines for comparison and for the failure messages. Comparing the joined text
// rather than the slice is deliberate: what an operator gets is the text, so the gates assert over
// the artifact that actually ships.
func text(lines []string) string { return strings.Join(lines, "\n") }

// TestStartupLinesWarnTheBuildTheTallyIsAboutToStrand is GATE G-DE-6.
//
// A build can be at or ahead of every COMMITTED block and still be about to be left behind: the
// readiness tally locks an era in one epoch BEFORE its first block, and that window is the only
// moment an operator can upgrade without a stall. The state is observable — the latch is chain
// state, not config — so a render that stayed silent here would be withholding the one thing the
// operator could still act on.
//
// The chain is driven to PENDING and left there: stamped 5, no block past H_era4, so no v5 block
// exists and the census alone cannot tell this apart from dark.
func TestStartupLinesWarnTheBuildTheTallyIsAboutToStrand(t *testing.T) {
	c, _, _ := eraProbeChain(t, BlockVersionWitnessable)
	st := c.EraState()
	if st.Era4.Phase != EraPending {
		t.Fatalf("FIXTURE: the chain must be PENDING for this gate to mean anything, got %q", st.Era4.Phase)
	}
	if st.Census.MaxVersion > BlockVersionRounds {
		t.Fatalf("FIXTURE: a v%d block is already committed — the warning would then be redundant "+
			"with the BEHIND verdict", st.Census.MaxVersion)
	}

	stranded := text(StartupEraLines(BlockVersionRounds, st))
	shipped := text(StartupEraLines(DeclaredMaxBlockVersion, st))
	t.Logf("pending chain, era-2 build:\n%s\n\npending chain, era-4 build:\n%s", stranded, shipped)

	if !strings.Contains(stranded, "STALLS") {
		t.Fatalf("G-DE-6 RED: the tally has locked in v%d and this build declares v%d, so it will "+
			"reject the first block of the new era — and the start-up render said nothing:\n%s",
			BlockVersionWitnessable, BlockVersionRounds, stranded)
	}
	// The shipped build is NOT stranded by the same latch, and must not be warned about it. A
	// warning that fires on every build is noise, and noise is how an operator learns to skip
	// the line the row exists to make them read.
	if strings.Contains(shipped, "STALLS") {
		t.Fatalf("G-DE-6 RED: the shipped build declares v%d and the tally locked in v%d — it is not "+
			"stranded, but the render warned anyway:\n%s",
			DeclaredMaxBlockVersion, BlockVersionWitnessable, shipped)
	}
}
