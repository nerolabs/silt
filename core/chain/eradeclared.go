package chain

// THE DECLARED ERA (freeze manifest item 19, second clause: "the daemon prints its declared max
// block era at start-up"). erastate.go answers what the CHAIN says; this file answers what the
// BINARY is, and renders the two together.
//
// WHY BOTH NUMBERS, AND WHY ONE OF THEM ANSWERS NOTHING. Cloud row 13b-delivery-settlement SKIPs
// with a sentence that covers two different worlds — "era-4 is dark" and "the issuer's keys are
// off-commitment" — and #808 gave the first half of the discrimination: what era the chain has
// reached. That half alone is still ambiguous. A chain carrying no v5 block is a HEALTHY DARK
// NETWORK under a build that declares v5, and the WRONG BUILD under one that declares v2. Same
// chain, same observation, opposite verdicts. The declared number is the other half, and the pair
// is the deliverable.
//
// WHY IT IS A CONSTANT AND NOT A SETTING. A declared era an operator can type answers nothing: it
// reports a belief, and an operator debugging a dark network would be reading their own input back.
// DeclaredMaxBlockVersion is a compile-time constant, asserted as one at compile time by
// declaredIsACompileTimeConstant in eradeclared_test.go — a const declaration takes a constant
// expression and nothing else, so the package stops building if this ever becomes a var, a field or
// a flag. And the claim it makes about the build is checked against the build's two real ceilings
// (versionSupported and MintVersion) rather than trusted, in TestDeclaredMaxIsTheBINARYsRealCeiling.
//
// THE ANTI-VACUITY DISCIPLINE IS THE SAME ONE erastate.go STATES, for the same reason: this row
// absorbs R-CARRIER-ROLLOUT-SIGNAL, whose code half was vacuous because a gate took its guard
// condition from its own subject and returned before asserting anything. Transposed into an
// observable the shape is a render that is always the same render. Hence: EraRelation is a closed
// string enum with NO zero value; every member is driven from a real chain and asserted pairwise
// distinct (TestEveryEraRelationIsDrivenAndDistinct); and a chain with no blocks is NOT rendered as
// "highest version v0" but as a named refusal to assert.

import "fmt"

// DeclaredMaxBlockVersion is the highest block version THIS BUILD mints and validates — the
// binary's half of the era pair. It moves in the release that widens versionSupported and flips
// MintVersion, never separately: TestDeclaredMaxIsTheBINARYsRealCeiling drives both ceilings and
// fails if this constant lags either one.
const DeclaredMaxBlockVersion = BlockVersionWitnessable

// EraRelation is how a build's declared maximum stands to the versions a chain actually carries.
// It is a CLOSED enum, and a STRING with no zero value on purpose: "" is not a relation, so a
// never-populated field cannot masquerade as a real verdict.
type EraRelation string

const (
	// EraRelationAhead: the build declares a higher version than any block on the chain. The
	// healthy-dark case — every live silt network is here today.
	EraRelationAhead EraRelation = "ahead"
	// EraRelationAt: the build declares exactly the highest version the chain carries.
	EraRelationAt EraRelation = "at"
	// EraRelationBehind: the chain carries a version above the build's declared maximum. The
	// wrong-build case.
	EraRelationBehind EraRelation = "behind"
	// EraRelationUnobserved: no block is loaded. Distinct from every other member because
	// nothing was measured — not because a measurement came back zero.
	EraRelationUnobserved EraRelation = "unobserved"
)

// EraRelations is the closed set. A completeness gate enumerates it rather than hand-listing the
// members, so a member added without a driven arm fails that gate instead of passing quietly.
var EraRelations = []EraRelation{EraRelationAhead, EraRelationAt, EraRelationBehind, EraRelationUnobserved}

// DeclaredRelation compares a declared maximum block version against the versions a chain actually
// carries. It reads the CENSUS — committed bytes — and no configuration, so it answers "what does
// this chain hold", never "what did this operator type".
//
// The comparison is against MaxVersion rather than HeadVersion deliberately: the question is
// whether this build can validate EVERY block on this chain, and a chain can carry a version above
// its own head if history was reorganised below an activation boundary.
func DeclaredRelation(declaredMax uint64, census VersionCensus) EraRelation {
	switch {
	case census.Empty():
		return EraRelationUnobserved
	case census.MaxVersion > declaredMax:
		return EraRelationBehind
	case census.MaxVersion == declaredMax:
		return EraRelationAt
	default:
		return EraRelationAhead
	}
}

// EraNameOf maps a block version to its rule-era number.
//
// THE MAPPING IS A TABLE, NOT ARITHMETIC, and the difference is load-bearing: era-1 mints v1, era-2
// mints v2, era-3 mints v4 and era-4 mints v5. v3 (BlockVersionRegGate) is skipped — it is the #506
// readiness STAMP and no block is ever minted with it, which is why the sequence jumps. Deriving
// the era with version-1 happens to be right above v3 and wrong below it, so a chain of v2 blocks
// would be announced as "era-1" and every operator reading the line would be off by one against the
// CHANGELOG, which calls those chains era-2.
//
// ok is false for a version that names no era, so a caller prints the version alone rather than
// inventing a name.
func EraNameOf(version uint64) (uint64, bool) {
	switch version {
	case 1:
		return 1, true
	case BlockVersionRounds: // 2
		return 2, true
	case BlockVersionStateRoot: // 4
		return 3, true
	case BlockVersionWitnessable: // 5
		return 4, true
	default: // v3 is a readiness stamp, never minted; anything higher has no era yet.
		return 0, false
	}
}

// eraTag renders a version as "era-4 (v5)", or as "v3" when the version names no era.
func eraTag(version uint64) string {
	if era, ok := EraNameOf(version); ok {
		return fmt.Sprintf("era-%d (v%d)", era, version)
	}
	return fmt.Sprintf("v%d", version)
}

// StartupEraLines renders the era pair a daemon prints once at start-up: what this build declares,
// what the chain it just loaded reports, and the verdict that relates them. The caller supplies the
// declared maximum so the render is testable against a build other than this one — the contrast
// that makes the shipped render mean something — while the daemon always passes the constant.
//
// Every line carries the daemon's "chain: " prefix, so the text under gate is the text an operator
// gets. The two era lines come from EraStatus.EraLine with tallyVisible=true: a caller holding a
// Chain CAN see the readiness tally, unlike the offline chain-status path.
func StartupEraLines(declaredMax uint64, st EraState) []string {
	lines := []string{fmt.Sprintf(
		"chain: era support — this BUILD declares %s: the highest block version it mints and validates. "+
			"That is a COMPILE-TIME property of the binary (chain.DeclaredMaxBlockVersion); no flag, "+
			"config or genesis field moves it.", eraTag(declaredMax))}

	// NOTHING OBSERVED. A daemon whose store is empty holds a real chain with no blocks — the
	// state every fresh node is in until genesis is seeded. Rendering "highest version v0" here
	// would be the vacuity this row exists to close: a zero indistinguishable from a measurement.
	// The declaration above still stands, because it is a fact about the build; the verdict does
	// not, because there is nothing to compare against.
	if st.Census.Empty() {
		return append(lines, "chain:   no block is loaded, so NOTHING about this chain's era is observed. "+
			"The line above is a fact about this binary alone and is not evidence about any chain.")
	}

	lines = append(lines,
		"chain:   "+st.Era3.EraLine(true),
		"chain:   "+st.Era4.EraLine(true),
		fmt.Sprintf("chain:   observed on this chain: %d block(s), head v%d at height %d, highest version v%d first at height %d",
			st.Census.Blocks, st.Census.HeadVersion, st.Census.HeadHeight,
			st.Census.MaxVersion, st.Census.MaxVersionFirstHeight))

	declared, observed := eraTag(declaredMax), eraTag(st.Census.MaxVersion)
	rel := DeclaredRelation(declaredMax, st.Census)
	switch rel {
	case EraRelationAhead:
		lines = append(lines, fmt.Sprintf(
			"chain:   DECLARED %s vs OBSERVED highest %s: AHEAD — this chain has not activated the era "+
				"this build supports. That is a HEALTHY dark network, not a wrong binary: this build "+
				"validates every block here and mints v%d once the chain activates it. The two era lines "+
				"above say which era is dark and which has locked in.",
			declared, observed, declaredMax))
	case EraRelationAt:
		lines = append(lines, fmt.Sprintf(
			"chain:   DECLARED %s vs OBSERVED highest %s: AT — this build declares exactly the highest "+
				"version this chain carries, so it validates every block on it.", declared, observed))
	case EraRelationBehind:
		lines = append(lines, fmt.Sprintf(
			"chain:   DECLARED %s vs OBSERVED highest %s: BEHIND — this chain carries blocks this build "+
				"CANNOT validate (v%d is above its declared maximum v%d). This is the WRONG BUILD for this "+
				"chain; upgrade the binary. A chain FILE carrying an unsupported version is refused at "+
				"replay, so a daemon reaching this line obtained those blocks some other way.",
			declared, observed, st.Census.MaxVersion, declaredMax))
	}

	// THE STRANDING WARNING. A build can be at or ahead of every COMMITTED block and still be
	// about to be left behind: the readiness tally may have locked an era in whose first block
	// this binary will reject. That is observable — the latch is chain state — and it is the one
	// case where the operator can still act before the stall rather than after it.
	//
	// NOT printed under BEHIND, where it would repeat the verdict directly above it in weaker
	// words: a build that already holds blocks it cannot validate is past the window this warning
	// exists to open, and a line that fires on every unhealthy state is how an operator learns to
	// skip the block.
	if rel == EraRelationBehind {
		return lines
	}
	for _, s := range []EraStatus{st.Era3, st.Era4} {
		if s.LockedIn && s.Version > declaredMax {
			lines = append(lines, fmt.Sprintf(
				"chain:   ⚠ the chain's readiness tally has LOCKED IN %s, ABOVE this build's declared "+
					"maximum v%d: at the activation height above, this build rejects the first v%d block "+
					"and STALLS. Upgrade before that height.",
				eraTag(s.Version), declaredMax, s.Version))
		}
	}
	return lines
}
