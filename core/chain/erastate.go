package chain

// The ERA OBSERVABLE (freeze manifest item 19, R-CLOUD-ERA-PROBE).
//
// WHY IT EXISTS. Three investigations stalled on the same absence. A Tester could not confirm
// whether the live test networks carry any v4 block, because no shipped command prints a block's
// Version. The live maximum len(b.Atts) is named as unmeasured on two certifications and no
// command produces it. And the cloud sheet's 13b-delivery-settlement row SKIPs with one sentence
// that covers two different worlds — "era-4 is dark" and "the issuer's keys are off-commitment" —
// because nothing distinguishes them.
//
// WHAT IT READS, AND WHAT IT REFUSES TO READ. Nothing here touches Config. Not one field. The era
// activation state has two routes — the genesis override (Era3ActivationHeight /
// Era4ActivationHeight) and the readiness-tally latch — and reading the override off the local
// Config would be the #380 class: a consensus quantity answered from what an operator typed rather
// than from what the chain says. The override IS committed now, at cbor key 20 fields 13-14 (see
// ConsensusParams), but CheckConsensusParams returns nil on a paramless genesis, so the paramless
// path survives and on it a Config read has no referent at all. Not reading it costs nothing:
// ACTIVE is defined below in a way that holds under EITHER route.
//
// WHY "ACTIVE" IS DEFINED FROM THE BLOCKS. Era activation is a MINT boundary — at and above H_era4
// a block MUST be v5, which MintVersion and the era-4 validity rule jointly enforce. So "a v5 block
// is committed" and "era-4 has activated" are the same fact, and the second can be read off the
// first without asking any configuration anything. That is route-independent by construction: it
// holds whether the boundary came from the tally or from a genesis override.
//
// WHAT THE BLOCKS CANNOT SHOW, AND WHY THE LATCH IS STILL HERE. Between the tally locking in and
// the first block of the new era there is one epoch of notice. In that window no v5 block exists,
// so a block-only view renders it identically to "dark" — and that window is the single most useful
// state for an operator watching a stamp raise land. EraPhase separates them.
//
// THE ANTI-VACUITY DISCIPLINE, AND WHY THIS FILE STATES IT. This row absorbs
// R-CARRIER-ROLLOUT-SIGNAL, "whose code half was vacuous" (ROADMAP). The vacuity mechanism there
// was a gate taking its guard condition from its own subject: TestG5_StampFiveImpliesTheCarrierIsHashCovered
// returns early while the stamp is 3, so it asserts nothing and prints ok exactly as
// a real pass would. Transposed into an observable, the same shape is a field that is always zero
// or always absent — indistinguishable from a field correctly reporting an empty state. Three
// devices keep it out of this one:
//
//  1. EraPhase is a closed string enum with NO zero value. "" is not a phase, so a field that was
//     never populated cannot masquerade as EraDark.
//  2. Optional heights are *uint64. Height 0 is a legal height — a genesis block can itself be v5 —
//     so 0 cannot mean "none". nil means none, and omitempty makes absent-versus-present-zero a
//     structural difference rather than a convention two fields must maintain in step.
//  3. A zero count is narrated by its renderer, never printed bare.
//
// Every phase is driven by a real chain in TestEraStateDrivesEveryPhaseFromCommittedState, and the
// no-Config claim is ablated in TestEraStateIgnoresDivergentLocalConfig.

import "fmt"

// EraPhase names the one state an era is in on a given chain. It is a CLOSED enum, and it is a
// STRING with no zero value on purpose: see the anti-vacuity note above.
type EraPhase string

const (
	// EraDark: the readiness tally has not locked in and no block of this era is committed.
	EraDark EraPhase = "dark"
	// EraPending: the tally HAS locked in and named an activation height, but no block of this
	// era is committed yet — the one epoch of notice. Unobservable from blocks alone, which is
	// why the offline CLI reports it as "not observable here" rather than as EraDark.
	EraPending EraPhase = "pending"
	// EraActive: a block of this era is committed. Route-independent — true under the tally and
	// under a genesis override alike.
	EraActive EraPhase = "active"
)

// EraPhases is the closed set, in increasing order of progress. It exists so a completeness gate
// can enumerate the phases rather than hand-listing them.
var EraPhases = []EraPhase{EraDark, EraPending, EraActive}

// EraStatus is one era's state on one chain.
type EraStatus struct {
	// Version is the block version this era mints (4 for era-3, 5 for era-4).
	Version uint64 `json:"version"`
	// Phase is one of EraPhases. Never the empty string on a value this package produces.
	Phase EraPhase `json:"phase"`
	// LockedIn is the readiness-tally latch, derived from committed history.
	LockedIn bool `json:"lockedIn"`
	// ActivationHeight is the tally's H_era. nil — key absent — when the tally has not locked
	// in. NOT 0, which is a legal height.
	ActivationHeight *uint64 `json:"activationHeight,omitempty"`
	// FirstHeight is the height of the first committed block at this version. nil — key absent —
	// when no such block exists. NOT 0, which is a legal height: a genesis block can be v5.
	FirstHeight *uint64 `json:"firstHeight,omitempty"`
}

// VersionCensus is the block-version distribution of a committed chain, plus the live attestation-
// carrier width. Every field is derived from the blocks themselves; no configuration is consulted.
type VersionCensus struct {
	Blocks      int    `json:"blocks"`
	HeadHeight  uint64 `json:"headHeight"`
	HeadVersion uint64 `json:"headVersion"`
	// MaxVersion is the highest block version present anywhere on the chain, and
	// MaxVersionFirstHeight is the height at which it first appears. On a mid-upgrade chain
	// MaxVersion exceeds HeadVersion only if history was reorganised below the boundary, so a
	// difference between the two is itself a signal.
	MaxVersion            uint64 `json:"maxVersion"`
	MaxVersionFirstHeight uint64 `json:"maxVersionFirstHeight"`
	// Counts is the per-version block count and FirstHeights the per-version first appearance.
	// Both are keyed by version. JSON renders the keys as strings.
	Counts       map[uint64]int    `json:"counts"`
	FirstHeights map[uint64]uint64 `json:"firstHeights"`
	// MaxAtts is max_h len(blocks[h].Atts), the live maximum attestation-carrier width, and
	// MaxAttsHeight the FIRST height attaining it. This is the figure two certification items
	// name as unmeasured; no shipped command produced it before this one.
	//
	// AttsMeasured distinguishes "measured, and the answer is zero" from "never computed". A
	// bare 0 here would be the vacuity trap: a genesis-only chain legitimately carries no
	// attestation, and that is an ANSWER, not an absence.
	MaxAtts       int    `json:"maxAtts"`
	MaxAttsHeight uint64 `json:"maxAttsHeight"`
	AttsMeasured  bool   `json:"attsMeasured"`
}

// Empty reports whether the census describes a chain with no blocks at all. This is the one true
// absence on this surface, and it is reported as one rather than as a page of zeros.
func (v VersionCensus) Empty() bool { return v.Blocks == 0 }

// EraState is the whole era observable for one chain: the block census, which is pure committed
// bytes, and the two era statuses, which add the readiness-tally latch.
type EraState struct {
	Census VersionCensus `json:"census"`
	Era3   EraStatus     `json:"era3"`
	Era4   EraStatus     `json:"era4"`
}

// CensusOf reduces committed blocks to their version distribution and carrier width. It is a PURE
// function of the blocks — the offline `silt chain-status` path and the live daemon status path
// call the same one, so the two surfaces cannot drift into disagreeing about the same chain.
func CensusOf(blocks []Block) VersionCensus {
	out := VersionCensus{Blocks: len(blocks), Counts: map[uint64]int{}, FirstHeights: map[uint64]uint64{}}
	if len(blocks) == 0 {
		return out
	}
	head := blocks[len(blocks)-1]
	out.HeadHeight, out.HeadVersion = head.Height, head.Version
	// AttsMeasured is set here, not inside the loop and not on the first non-empty carrier: the
	// claim is "this chain was walked", which is true from the first block onward regardless of
	// how many attestations any block carries.
	out.AttsMeasured = true
	for i := range blocks {
		b := &blocks[i]
		out.Counts[b.Version]++
		if _, seen := out.FirstHeights[b.Version]; !seen {
			out.FirstHeights[b.Version] = b.Height
		}
		if b.Version > out.MaxVersion {
			out.MaxVersion = b.Version
		}
		// Strictly greater: MaxAttsHeight is the FIRST height attaining the maximum, so a
		// later block of equal width does not move it.
		if n := len(b.Atts); n > out.MaxAtts {
			out.MaxAtts, out.MaxAttsHeight = n, b.Height
		}
	}
	out.MaxVersionFirstHeight = out.FirstHeights[out.MaxVersion]
	return out
}

// EraState reports this replica's era observable. The census half is committed bytes; the latch
// half (LockedIn, ActivationHeight) is the readiness tally, which is itself derived from committed
// history in rotateEpoch. Config is not read.
func (c *Chain) EraState() EraState {
	census := CensusOf(c.blocks)
	return EraState{
		Census: census,
		Era3:   eraStatus(BlockVersionStateRoot, c.era3LockedIn, c.era3Height, census),
		Era4:   eraStatus(BlockVersionWitnessable, c.era4LockedIn, c.era4Height, census),
	}
}

// eraStatus composes one era's status. ACTIVE dominates PENDING: once a block of the era is
// committed the era has activated, whatever route got it there, so the tally's opinion no longer
// decides the phase.
func eraStatus(version uint64, lockedIn bool, activationHeight uint64, census VersionCensus) EraStatus {
	s := EraStatus{Version: version, Phase: EraDark, LockedIn: lockedIn}
	if lockedIn {
		h := activationHeight
		s.ActivationHeight, s.Phase = &h, EraPending
	}
	if h, ok := census.FirstHeights[version]; ok {
		hh := h
		s.FirstHeight, s.Phase = &hh, EraActive
	}
	return s
}

// EraLine renders one era status as a single operator-facing line. Both the offline CLI and any
// log line share it, so the two cannot describe the same state differently.
//
// tallyVisible is false for a caller that holds only blocks — the offline `silt chain-status` path,
// which loads chain.cbor into a []Block and has no Chain. Such a caller CANNOT see the tally, and
// this says so instead of printing "lockedIn: false". That difference is the whole point of the
// row: a false that means "not observable here" is the vacuity the predecessor shipped.
func (s EraStatus) EraLine(tallyVisible bool) string {
	era := s.Version - 1 // era-3 mints v4, era-4 mints v5
	switch {
	case s.Phase == EraActive:
		return fmt.Sprintf("era-%d (v%d): ACTIVE — first v%d block at height %d",
			era, s.Version, s.Version, *s.FirstHeight)
	case s.Phase == EraPending:
		return fmt.Sprintf("era-%d (v%d): PENDING — the readiness tally locked in; activates at height %d, no v%d block committed yet",
			era, s.Version, *s.ActivationHeight, s.Version)
	case tallyVisible:
		return fmt.Sprintf("era-%d (v%d): DARK — the readiness tally has not locked in and no v%d block is committed",
			era, s.Version, s.Version)
	default:
		return fmt.Sprintf("era-%d (v%d): NOT ON THIS CHAIN — no v%d block is committed. "+
			"Offline this cannot separate \"the tally has not locked in\" from \"locked in, activating at a later height\": "+
			"the tally is chain state, not a block field. A running daemon reports it at GET /api/status .chain.era",
			era, s.Version, s.Version)
	}
}
