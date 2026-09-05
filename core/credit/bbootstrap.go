//go:build bbootstrap

package credit

// THE INSTRUMENT COMPILES ONLY UNDER THE `bbootstrap` BUILD TAG (D-BB-BUILD-TAG,
// ratified 2026-09-05). A default `go build` produces a binary with no histogram type,
// no census reader, no age stamping and no -bbootstrap flag — bbootstrap_off.go is the
// whole of what a default build keeps (an empty struct and an empty method).
//
// WHY A TAG AND NOT A BETTER RUNTIME GATE. TENETS Part VI Don't #3 is a claim about what
// silt BUILDS — "silt builds no mechanism to observe or link who-fetches-what… The
// refusal to build surveillance is absolute" — not a claim about who can currently read
// the output. A shipped binary that contains the mechanism and merely declines to print
// it satisfies the second reading and not the first. Every containment that regulates the
// READER (a loopback bind, a status token) is a deployment posture layered on a binary
// that still holds the capability, and the prior art reaches the same place: go-ethereum
// resolved the analogous question by REMOVING the `personal` namespace from the
// network-facing surface rather than authenticating it better (ADVISORY 2026-09-05).
//
// R2.9a — the B_bootstrap instrument: a full-census 2-D COUNT HISTOGRAM over
// (identity age × log2 fetched bytes).
//
// WHY IT EXISTS. `D-R2.9-DIRECTION` sentence 4 makes one measurement a precondition
// of pinning the affordability ratio `grant/r`: on real traffic, how many bytes does a
// fresh honest identity fetch from ONE serving ledger while it is young. cloudtest
// measures its own synthetic fetch plan and therefore cannot produce it, so the series
// has to come off a deployment carrying real users.
//
// WHAT IT IS NOT. Instrumentation only. Nothing here moves credit, escrow or standing,
// and no conservation rule reads it (Invariant A: the two methods below are classified
// `neutral` in invariant_a_test.go and a test proves the snapshot writes nothing).
//
// THE SHAPE, and why it is a histogram rather than rows
// (RESEARCH CERTIFICATION R2.9a-Bbootstrap-instrument-sufficiency, 2026-09-04, §5.2).
// The first build exported per-requester rows capped at the top 4,096 BY BYTES. That
// retention rule selects on the response variable of the very regression the series
// exists to fit: it removes, from every age cell, exactly the identities below the
// (unpublished) threshold, so the YOUNG cells — the ones the fit reads — empty first and
// hardest, and the bias is not correctable after the fact. Reservoir sampling is
// unbiased but needs randomness inside core (forbidden — internal/depcheck) and
// re-introduces per-row publication. A full census into fixed bins has none of those
// properties: no cap, no truncation, no sampling, no salt, no hash, no sort, no per-row
// allocation, and a payload constant in the requester count R.
//
// COUNTS ONLY — a per-cell byte SUM is forbidden. A cell sum with count 1 is that
// identity's exact byte total in disguise; a count at bin granularity is not. The
// residual is named and disclosed rather than closed: a count-1 cell still says "one
// identity of this age bucket had bytes in this bin" (R-BB-SINGLETON-CELL, open,
// bounded). Suppressing low-count cells would destroy exactly the tail the fit reads,
// so the certification rules NO suppression.
//
// THE CLOCK IS INJECTED (G-BB-2). The age axis rides the `ports.Clock` the node already
// carries, stamped once per account at first touch (Register). The consensus epoch is
// REFUTED for this purpose: it is identically 0 on a non-validator, which is exactly the
// machine that will run this, so an epoch-aged export would publish a constant. A ledger
// with no clock injected publishes NO age-conditioned cells at all — never an all-zero
// age column that a reader could mistake for a genuinely young population.
//
// TWO CLOCKS, AND WHY (G-BB-4 / BB-13). The injected ports.Clock is a WALL clock in
// production: adapters/walltime returns time.Now().UnixNano(), which discards Go's
// monotonic reading. Uptime and every age are both differences taken from ONE reading of
// it, so a step in that clock moves both minuends and CANCELS out of any comparison
// between them. The first build asserted "largest occupied age edge <= uptime" against
// that wall uptime and called it unviolatable by construction; measured, an 8-day forward
// step put a 30-second-old identity in the ">7 days" bucket, reported 8 days of uptime,
// raised no flag, and made the run precondition accept a 60-second-old process for a
// 7-day window. That is the silent age reshaping BB-13 forbids, and it is why a SECOND,
// INDEPENDENT source is injected alongside the clock: ports.MonotonicNanos, which nothing
// can step. The divergence between them IS the step. It is published as a number
// (ClockSkewNanos) as well as a flag, so an analyst can judge it against their own W.

import (
	"fmt"
	"math/bits"

	"github.com/nerolabs/silt/ports"
)

// The axis sizes. 8 × 164 = 1,312 int64 counters ≈ 10 KiB, fixed, whatever R is.
const (
	// BBootstrapAgeBuckets is the number of identity-age buckets (the A axis).
	BBootstrapAgeBuckets = 8
	// BBootstrapByteBins is the number of fetched-byte bins (the B axis):
	// BBootstrapBinsPerOctave bins per doubling over 2^0 … 2^40.75, top bin open.
	BBootstrapByteBins = 164
	// BBootstrapBinsPerOctave is the byte-axis resolution: 4 bins per doubling
	// (quarter-log2), which resolves B to 19%. Plain log2 would resolve it to 2×,
	// comfortably inside the ~1,000× span the grant/r decision covers — but 2× is
	// NOT free at the margin, because both directions of the residual land on
	// immutables (too high a grant/r cheapens Sybil bootstrap, M0; too low raises
	// the floor of honest participation, build-immutable #4). There is no safe side
	// to round to, so the certification's ruling is to SHRINK the trade rather than
	// resolve it, and to carry the remaining interval into the ratification sentence
	// (G-BB-8). At ~10 KiB against the row export's measured 114 MiB, the finer bins
	// are free.
	BBootstrapBinsPerOctave = 4
)

// bbAgeEdgeNanos are the LOWER edges of the age buckets, in nanoseconds. Bucket i
// covers [bbAgeEdgeNanos[i], bbAgeEdgeNanos[i+1]) — half-open, low-closed — and the
// last bucket is open-topped. Bucket 0 is age EXACTLY zero (a same-tick fetch), which
// a sim clock produces routinely and a wall clock essentially never does; keeping it
// separate is what stops "brand new" and "arrived in the last minute" from fusing.
//
// G-BB-1 (the owner pins W and q before the RUN) MAY REQUIRE ADDING AN EDGE. The
// reading rule is "the cell containing W", which is exact only when W lands ON an edge;
// off an edge it is bracketed by the bucket. These edges are chosen to bracket the
// plausible candidates (a viewing session, an hour, a day, a week) and NO reading rule
// is hard-coded anywhere in this file or on the wire — the analyst picks the bucket.
// Adding an edge is a one-line change here plus the bucket count above.
var bbAgeEdgeNanos = [BBootstrapAgeBuckets]int64{
	0,                   // exactly age 0
	1,                   // (0, 1 minute)
	60 * 1e9,            // [1 minute, 10 minutes)
	600 * 1e9,           // [10 minutes, 1 hour)
	3600 * 1e9,          // [1 hour, 6 hours)
	6 * 3600 * 1e9,      // [6 hours, 24 hours)
	24 * 3600 * 1e9,     // [24 hours, 7 days)
	7 * 24 * 3600 * 1e9, // [7 days, ∞)
}

// The quarter-octave thresholds, as a mantissa normalised into [2^62, 2^63). Integer
// comparison only: no math package, no floating point, exact and deterministic.
// bbQuarter{1,2,3} = round(2^(k/4) · 2^62) for k = 1, 2, 3.
const (
	bbQuarter1 = uint64(5484249825272419328)
	bbQuarter2 = uint64(6521908912666391552)
	bbQuarter3 = uint64(7755900482342532096)
)

// bbClockSkewToleranceNanos is how far the wall clock may diverge from the monotone
// source before the artifact declares the run suspect. It is ONE MINUTE, and the number
// is DERIVED rather than picked: one minute is the width of the narrowest positive-width
// age bucket (bucket 1 spans (0, 1 minute) — bbAgeEdgeNanos below). Below one bucket
// width a divergence cannot displace an identity by a whole bucket; at or above it, it
// can, and the bottom of the age axis is exactly where the fit reads a FRESH identity.
//
// It is deliberately on the strict side. A long run that accumulates a minute of ordinary
// NTP slew is flagged even though a minute is negligible against a W of days, and that
// costs a re-run. The other error — a step reshaping the young cells with nobody
// noticing — costs a wrong grant/r, and grant/r lands on M0 (too high cheapens Sybil
// bootstrap). The asymmetry decides the direction. ClockSkewNanos carries the raw number
// either way, so an operator who disagrees with this threshold can read past it.
const bbClockSkewToleranceNanos = int64(60 * 1e9)

// THE MINIMUM-REQUESTER FLOOR (G-BB-11), and why it is a WHOLE-BLOCK rule rather than
// the per-cell suppression this instrument REFUSES.
//
// Two different objects, and a reader must not fuse them:
//
//   - A LOW-COUNT CELL is never suppressed. Suppression there eats exactly the tail the
//     quantile fit reads, and the bias is not correctable after the fact. That ruling
//     stands (see the COUNTS ONLY note above).
//   - THE WHOLE BLOCK is suppressed when the census is not a POPULATION. Below the floor
//     there is no tail to eat, because there is nothing to fit.
//
// WHAT THE FLOOR ACTUALLY CLOSES, stated exactly, because the obvious reading is wrong.
// The leak is not that a cell count identifies someone. `stats.bytesServed`
// (cmd/silt/ui.go) and `durability.objects[].funded` (core/credit/escrow.go) are
// published UNCONDITIONALLY and PREDATE this instrument: at one requester, their deltas
// already say "the single requester fetched X bytes of object Y". What this instrument
// adds is `requesters` — THE ANONYMITY-SET SIZE — and publishing that is what converts a
// pre-existing aggregate counter into an attributable observation about one identity.
// So suppressing cells while still publishing `requesters` would close NOTHING, and the
// floor covers every census count, not just the grid.
//
// AND WHAT IT IS NOT — a correction, because the first version of this comment implied
// the opposite and the implication is false (RE-CERTIFICATION 2026-09-05 §2.2, §2.5).
// THE FLOOR BOUNDS THE PUBLISHED CENSUS **COUNT**. IT DOES NOT BOUND THE ANONYMITY
// **SET**. The census population is the set of identities that fetched, and an identity
// is a keypair (adapters/identity/identity.go:64-67 — "generating a keypair is
// joining"); the serve path has no admission control beyond freeload and chunkDenied,
// and Register mints an account for any unseen id. So an observer that can FETCH lifts
// the floor for R_min − 1 keypairs and one chunk each — nine identities and about
// 576 KiB — and because fetchedBytes never decreases and accounts are never deleted,
// that purchase is ONE-TIME and PERMANENT for the process's lifetime. A minimum-count
// threshold is disclosure control only over a population the adversary cannot write to,
// and this population is free by design: the same freeness B_bootstrap exists to price.
//
// The floor is therefore a FIT PRECONDITION (below R_min no q-quantile at q >= 0.9 is
// estimable at all — see the derivation below) and a defence against a reader that
// CANNOT fetch: a monitoring network filtered from the swarm port, a passive scraper
// that runs no silt node, an accidental exposure during a genuinely idle period. It is
// NOT a privacy mitigation against a capable adversary, and the Don't #3 question is not
// answered by it. Residuals: R-BB-CENSUS-SYBIL-PAD and R-BB-ANONYMITY-SET-SIZE, both
// open, neither closable at this instrument.
//
// WHAT IT DOES NOT CLOSE. A polled series of cell deltas yields single-identity bin
// trajectories at ANY R: one identity crossing a bin edge between two polls shows up as
// -1 at one bin and +1 at another. The floor closes ATTRIBUTION (whose trajectory it is)
// only against a reader that cannot pad the census; against a fetching reader it closes
// neither attribution nor extraction. That residual is R-BB-DELTA-TRAJECTORY, open. It
// is NOT claimed closed here.
//
// THE OBSERVATION RATE IS BOUNDED BY THE PUBLISHER, NOT BY THE READER (G-BB-26). An
// earlier version of this paragraph said the residual was "bounded by the poll rate".
// That was wrong as written and is corrected here: the poll rate is the READER's own
// choice, and there is no rate limiter anywhere on the UI server. The bound is real only
// because the UI server serves ONE snapshot recomputed at most once per a fixed
// interval T and the cached copy in between (cmd/silt/ui.go, statusSnapshotInterval), so
// an observer gets at most floor(uptime/T) distinct blocks however fast it asks and
// every crossing inside one interval is unresolvable. THE BOUND COVERS EXACTLY THE TWO
// ENDPOINTS SERVED OFF THAT SNAPSHOT, GET /api/status (this block, durability.balance,
// stats.BytesServed) and GET /api/economy/self (revenue.*, the escrow sums). As first
// written this paragraph said "/api/status now serves a snapshot", which was true of
// the block and false of the sibling aggregates it names: /api/economy/self recomputed
// per request, and a blind PE review extracted the escrow step from it at 330 ms. It
// now reads the same snapshot. T is a SECURITY PARAMETER: it is derived, published on
// the wire beside the axis constants so an analyst can price this residual, and
// ratified by the owner (D-STATUS-SNAPSHOT-INTERVAL, with its appended correction).
//
// THE RULE THE FLOOR ENFORCES IS A PROPERTY, NOT A FIELD LIST (G-BB-11′). Partition
// every published field of the block:
//
//   - INSTRUMENT: the value is a function of the injected clock sources, their injection
//     instants and the compiled axis constants ALONE.
//   - CENSUS: the value depends on the contents of l.accounts or l.order.
//
// Below R_min the published block MUST be a function of the instrument fields alone,
// with exactly ONE named exemption: Suppressed itself, whose information content is
// precisely "the census is below R_min" — a published upper bound of R_min − 1 on the
// anonymity set (R-BB-SUPPRESSED-IS-A-DISCLOSURE). It cannot be withheld without fusing
// "below the floor" with "no clock injected", which is the absent-vs-empty distinction
// this file enforces everywhere else, so it is disclosed in the owner's brief rather
// than hidden.
//
// WHY A PROPERTY. A list failed three times in one pull request: the certification's
// three fields missed two, the build's five missed two more (AgeExceedsUptime, and one
// of ClockStepBack's two arms), and a source gate reading two literal paths claimed a
// whole-tree property a reviewer ablated past. The form was the defect. So
// WithMinRequesterFloor CONSTRUCTS the suppressed block out of the instrument class
// rather than CLEARING a list of census fields: a field nobody has foreseen takes its
// zero value, which is a compile-time constant and therefore trivially a function of the
// instrument alone. The default flips from PUBLISHED to WITHHELD. BB-20
// (cmd/silt/r29a_bb20_equivalence_test.go) runs the property at the wire.
const (
	// bbDerivedQuantileFloorPercent is NOT q. q is the owner's and is UNPINNED
	// (G-BB-1). This is the LOWER EDGE OF THE RANGE OVER WHICH THE DERIVATION BELOW IS
	// CERTIFIED: q >= 0.90. At any q at or above this edge the floor is strictly
	// dominated by the fit's own sample requirement and therefore costs the fit nothing.
	// AT q BELOW THIS EDGE THE DERIVATION MUST BE RE-RUN — that is why the rule is
	// encoded here and not just its answer.
	bbDerivedQuantileFloorPercent = 90

	// bbQuantileObservationFloor is the RULE, evaluated: estimating a q-quantile needs at
	// least ceil(1/(1-q)) observations IN THE READ CELL. Written as an integer ceiling
	// over percent so it re-derives if the edge above moves:
	//
	//	ceil(100 / (100 - q%))   =   (100 + (100-q%) - 1) / (100 - q%)
	//
	// At q = 0.90 that is (100 + 10 - 1) / 10 = 10.
	bbQuantileObservationFloor = (100 + (100 - bbDerivedQuantileFloorPercent) - 1) /
		(100 - bbDerivedQuantileFloorPercent)

	// BBootstrapMinRequesters is R_min, the CENSUS-WIDE floor. The derivation above is a
	// requirement on ONE CELL of the eight-bucket age axis, so the census must carry far
	// more than this; a census-wide floor of the same number is therefore strictly
	// weaker than what the fit already needs, and costs it nothing.
	BBootstrapMinRequesters = bbQuantileObservationFloor
)

// A compile-time guard, not a comment: the certified floor is R_min >= 10 whatever the
// derivation above yields. If someone lowers bbDerivedQuantileFloorPercent far enough to
// drive the derived floor under 10, this conversion of a negative constant to uint fails
// to build, which is the only way to make "do not pin R_min below the certified floor"
// structural rather than a promise.
const _ = uint(BBootstrapMinRequesters - 10)

// BBootstrapHistogram is the whole published object. It carries no per-identity datum
// at all — no id, no salted label, no exact age, no row. Fixed-size by construction:
// every field is a scalar or a fixed-length array, so the payload is constant in R.
type BBootstrapHistogram struct {
	// ClockSource is the age axis's self-report (H-1): "injected" when a ports.Clock
	// was wired, "none" when it was not. A reader must never have to infer it.
	ClockSource string
	// AgeAxisLive is false exactly when ClockSource == "none". When it is false Cells
	// is nil — the instrument REFUSES to publish age-conditioned cells rather than
	// publish an all-zero age column indistinguishable from a young population
	// (G-BB-2). "Disabled" and "empty" are different objects on the wire.
	//
	// Cells is ALSO nil when Suppressed is true, with the age axis live. The two cases
	// are told apart by AgeAxisLive and Suppressed, which is why both are published.
	AgeAxisLive bool

	// Suppressed is the minimum-requester floor (G-BB-11′), applied. TRUE means the
	// census held fewer than BBootstrapMinRequesters requesters and the block is a
	// function of the INSTRUMENT fields alone: the clock self-reports, the uptimes, the
	// skew and the axis description, plus this bit. Every CENSUS-class field is
	// withheld — not by a list, but because WithMinRequesterFloor builds the suppressed
	// block out of the instrument class and everything else takes its zero value.
	//
	// This bit is the ONE named exemption to that rule, and it is itself a disclosure:
	// its information content is exactly "the census is below R_min", a published upper
	// bound of R_min − 1 on the anonymity set (R-BB-SUPPRESSED-IS-A-DISCLOSURE). It
	// cannot be withheld, because a reader that sees no cells must be able to tell
	// "below the floor" from "no clock injected".
	//
	// It is published rather than inferred. A reader that sees no cells must be able to
	// tell "below the floor" from "no clock injected" without guessing, and a reader
	// that sees zero counts must not read them as a measured zero.
	Suppressed bool

	// Requesters is the TRUE total: every account with fetchedBytes > 0. No cap, no
	// truncation — this is a census. Aged + Unstamped == Requesters holds WHENEVER THE
	// AGE AXIS IS LIVE, and only then: with no clock injected a requester is counted
	// here and placed in neither counter, so the honest reading of a dead-clock payload
	// is Requesters > 0 with Aged == Unstamped == 0 (BB-1 pins exactly 5 / 0 / 0).
	Requesters int
	// Aged is how many of those requesters were placed into a cell. Sum(Cells) == Aged.
	Aged int
	// Unstamped is how many carried no first-touch stamp (registered before the clock
	// was injected). They are counted here rather than dumped into age bucket 0, which
	// would make them look brand new — a silent age reshaping.
	Unstamped int

	// UptimeNanos is the ledger's elapsed time on the INJECTED ports.Clock since that
	// clock was injected. In production that is a wall clock, so this number moves with
	// an NTP step and is NOT on its own a bound on anything — read CensoringBoundNanos
	// instead. It is published because the analyst needs both sides of the skew.
	UptimeNanos int64

	// MonotonicSource is the second time source's self-report: "injected" when a
	// ports.MonotonicNanos was wired, "none" when it was not. "None" and "zero elapsed"
	// have to be different objects for the same reason "off" and "idle" do.
	MonotonicSource string
	// MonotonicUptimeNanos is elapsed time on the source NOTHING CAN STEP. It is the
	// real CENSORING BOUND: no identity can have been known to this ledger for longer
	// than the process has been running, whatever the wall clock says, so no window W
	// larger than the longest clean monotone uptime is measurable at all, ever (accounts
	// are in-memory and a restart destroys every one — R-BB-CENSORED-WINDOW). Zero when
	// no monotone source is injected.
	MonotonicUptimeNanos int64
	// ClockSkewNanos is the wall clock's elapsed time minus MonotonicUptimeNanos: how far
	// the wall clock has diverged from real elapsed time since injection. It is taken
	// from the RAW wall delta, before the clamp UptimeNanos applies, so a step past the
	// ledger start still reports its full size instead of reporting zero. On a clean run
	// it is zero to within the two reads. POSITIVE means the wall clock jumped FORWARD
	// (ages inflated, identities pushed up the age axis); NEGATIVE means it jumped BACK
	// (ages deflated, identities pulled down). It is also zero when no monotone source is
	// injected, which is why the run precondition refuses that configuration outright
	// rather than reading the zero as a clean run.
	ClockSkewNanos int64
	// ClockSuspect is |ClockSkewNanos| >= bbClockSkewToleranceNanos: the wall clock has
	// diverged far enough to have moved an identity a whole age bucket. This is the flag
	// that catches a step in EITHER direction, including the ones ClockStepBack cannot
	// see because no subtraction crossed zero.
	ClockSuspect bool

	// MaxOccupiedAgeEdgeNanos is the lower edge of the highest age bucket that has any
	// count. Zero when nothing is occupied.
	MaxOccupiedAgeEdgeNanos int64
	// ClockStepBack is set when the injected clock read EARLIER than THE LEDGER'S OWN
	// START, so the wall delta would have gone negative. It is INSTRUMENT class: it
	// compares two clock readings and touches no account, so it fires on an empty
	// ledger and survives suppression.
	//
	// IT WAS SPLIT (RE-CERTIFICATION 2026-09-05 §5.1). The reviewed build fused this
	// with the per-account clamp below under one name, and that second arm is CENSUS
	// class — it can only fire if an account exists — so the fused flag was a census
	// bit published below the floor. The split keeps the operator's whole signal and
	// puts each arm in its own class, which is what the property (G-BB-11′) asks for.
	//
	// IT IS NOT THE CLOCK-STEP DETECTOR, and must not be read as one: it fires only when
	// a step is large enough to cross zero. A backward step SMALLER than the accounts'
	// ages — measured, 2 h 50 m against 3-hour-old identities — reshapes every bucket
	// and never trips it. ClockSuspect is the detector (R-BB-WALLCLOCK-STEP).
	ClockStepBack bool
	// AgeClampedToZero is ClockStepBack's other arm: the injected clock read EARLIER
	// than some account's first-touch stamp, so that account's age was clamped to zero
	// rather than underflowed. CENSUS class — it is a statement about the accounts, and
	// at a degenerate anonymity set it says "the one requester is stamped in the
	// future" — so it is WITHHELD below the floor. The operator keeps ClockSuspect and
	// the raw signed ClockSkewNanos, which report the same corruption and are
	// census-free.
	AgeClampedToZero bool
	// AgeExceedsUptime is the G-BB-4 censoring assertion, evaluated at snapshot time:
	// MaxOccupiedAgeEdgeNanos must be <= CensoringBoundNanos(). With a monotone source
	// injected the two sides come from INDEPENDENT clocks, so this fires on the
	// production path — a forward wall-clock step ages identities past a bound that did
	// not move. With NO monotone source it degenerates into a comparison of the wall
	// clock against itself, invariant under every step, and can then only catch a stamp
	// written from a foreign tick source. That degeneracy is why
	// BBootstrapRunPrecondition refuses a run with no monotone source at all.
	//
	// CENSUS class, and this is the field the reviewed build got wrong: it is a
	// THRESHOLD ON MaxOccupiedAgeEdgeNanos, the very field the floor withholds, so at a
	// census of one it published a lower bound on that one identity's age. It is
	// WITHHELD below the floor. Nothing is lost: ClockSuspect and ClockSkewNanos report
	// the same corruption without reading an account.
	AgeExceedsUptime bool

	// The axes, published so the artifact is self-describing for a third party.
	AgeEdgeNanos  [BBootstrapAgeBuckets]int64 // lower edges; bucket i = [i, i+1), last open
	BinsPerOctave int                         // 4
	ByteBins      int                         // 164
	// ByteBinRule states the byte axis exactly, as a closed form rather than a rounded
	// array: floor(2^(k/4)) is not strictly increasing for k < 16 (integers below 16
	// bytes cannot resolve a quarter octave), so a printed edge array would be a lie at
	// the bottom where the closed form is exact everywhere.
	ByteBinRule string

	// Cells[ageBucket][byteBin] is a COUNT of identities. nil when AgeAxisLive is false.
	Cells *[BBootstrapAgeBuckets][BBootstrapByteBins]int64
}

// CensoringBoundNanos is the largest age this artifact can honestly carry: elapsed time
// on the source that cannot be stepped, when one is injected, and otherwise the wall
// clock's own elapsed time. It is a method rather than a field so the published field set
// stays the audited one (core/node/bbootstrap_test.go pins it by reflection).
//
// The fallback is stated rather than hidden: with no monotone source this returns a
// quantity derived from the same clock the ages are, which is exactly the self-comparison
// F-1 named, so the run precondition refuses that configuration.
func (h BBootstrapHistogram) CensoringBoundNanos() int64 {
	if h.MonotonicSource == "injected" {
		return h.MonotonicUptimeNanos
	}
	return h.UptimeNanos
}

// WithMinRequesterFloor applies the minimum-requester floor (G-BB-11′) and returns the
// PUBLISHABLE histogram. At or above BBootstrapMinRequesters it returns the receiver
// unchanged. Below it, it returns a block that is A FUNCTION OF THE INSTRUMENT FIELDS
// ALONE plus the Suppressed bit.
//
// IT CONSTRUCTS RATHER THAN CLEARS, AND THAT IS THE WHOLE POINT. The reviewed build
// zeroed a list of five census fields and left everything else standing; two
// census-derived fields were not on the list and rode out below the floor. Three
// independent enumerations in one pull request each missed a field, so the FORM was
// refuted: a list cannot cover a field nobody has foreseen. Here the suppressed block is
// built from the instrument class and every other field — including one added tomorrow
// by someone who never read this comment — takes its ZERO VALUE, which is a compile-time
// constant and therefore trivially a function of the instrument alone. The default is
// WITHHELD, and a new field is published below the floor only by a deliberate edit to
// this literal.
//
// The cost of that default is stated rather than hidden: a new INSTRUMENT field is also
// withheld until someone adds it here. That is a loss of operator signal, never a loss
// of privacy, and it is the right way round.
//
// WHY IT IS A SEPARATE METHOD RATHER THAN PART OF THE SNAPSHOT. The floor is about
// PUBLICATION, not about recording. The serving operator's own process already holds
// fetchedBytes keyed by NodeID and every ChunkID it answered, so a node-local read gives
// the operator nothing it does not already have; the leak exists only against a reader
// who is not the operator. bBootstrapSnapshot therefore stays the honest local census,
// and it is UNEXPORTED: BBootstrapPublish is the only way the histogram leaves this
// package, and it floors. That is the type system enforcing "no unfloored census leaves
// core/credit" — a compiler property, not a lint (see BBootstrapPublish for the exact
// scope of that claim and its residual).
//
// It is a value method on a value receiver: it mutates nothing and allocates nothing on
// the pass-through path.
func (h BBootstrapHistogram) WithMinRequesterFloor() BBootstrapHistogram {
	if h.Requesters >= BBootstrapMinRequesters {
		return h
	}
	// THE INSTRUMENT CLASS, enumerated ONCE, as a construction. Every field here is a
	// function of the injected clock sources, their injection instants and the compiled
	// axis constants alone. Nothing here reads l.accounts or l.order, directly or
	// through a threshold. Adding a field to this literal is a privacy decision and must
	// be argued as one; NOT adding it is the safe default.
	return BBootstrapHistogram{
		// The one named exemption: its content is exactly "the census is below R_min".
		Suppressed: true,

		ClockSource:          h.ClockSource,
		AgeAxisLive:          h.AgeAxisLive,
		UptimeNanos:          h.UptimeNanos,
		MonotonicSource:      h.MonotonicSource,
		MonotonicUptimeNanos: h.MonotonicUptimeNanos,
		ClockSkewNanos:       h.ClockSkewNanos,
		ClockSuspect:         h.ClockSuspect,
		// The ledger-start arm only. The per-account clamp is AgeClampedToZero, which
		// is census class and is NOT copied.
		ClockStepBack: h.ClockStepBack,

		AgeEdgeNanos:  h.AgeEdgeNanos,
		BinsPerOctave: h.BinsPerOctave,
		ByteBins:      h.ByteBins,
		ByteBinRule:   h.ByteBinRule,
	}
}

// BBootstrapPublish is THE ONLY ROUTE the B_bootstrap histogram takes out of this
// package, and it applies the minimum-requester floor (G-BB-11′) on the way.
//
// M-2, CLOSED BY THE TYPE SYSTEM RATHER THAN BY A NAME GATE. The reviewed build kept the
// raw snapshot exported and pinned "nothing outside core/credit calls it" with a test
// that read two hard-coded file paths. A reviewer ablated past it in five lines by adding
// a second unfloored export to a third file: build clean, gate green. Walking the whole
// tree instead of two literals would not have closed it either, because the consuming
// seam is DUCK-TYPED — core/node asserts an anonymous interface on the METHOD NAME — so a
// name-based gate is blind to a second exported reader added INSIDE core/credit. The
// close is therefore the compiler: bBootstrapSnapshot is unexported, so no package
// outside this one can obtain the raw census histogram at all, whatever it names its
// method.
//
// THE EXACT SCOPE OF THAT CLAIM, because an overstated gate is the defect this replaces:
//
//   - CLOSED by the compiler: no *unfloored BBootstrapHistogram* can leave core/credit.
//     A duck-typed impostor can satisfy core/node's interface, but it can only supply its
//     own data; it cannot reach this ledger's raw census.
//   - CLOSED by a source gate over this WHOLE PACKAGE, not by a literal path list:
//     TestR29aBBootstrapHasOneExportedRoute parses every non-test file in core/credit and
//     requires that the only exported functions returning a BBootstrapHistogram are this
//     one and WithMinRequesterFloor (which can only ever floor). That gate checks exactly
//     the package it names.
//   - NOT CLOSED, and stated rather than implied: this is a rule about the HISTOGRAM
//     OBJECT, not about census data in general. FetchedBytes and ServedBytes are exported
//     per-identity readers that predate this instrument and are unaffected. A future
//     exported method returning census-derived SCALARS of some other type would not be
//     caught by the type system, and would be caught by BB-20 only once it reached
//     /api/status. Residual: R-BB-EXPORT-SCALAR-BYPASS, open, bounded by review.
//
// Loop-owned, like every other ledger read.
func (l *Ledger) BBootstrapPublish() BBootstrapHistogram {
	return l.bBootstrapSnapshot().WithMinRequesterFloor()
}

// BBootstrapByteBinRule is the byte axis, stated exactly. Published verbatim.
const BBootstrapByteBinRule = "bin k covers [2^(k/4), 2^((k+1)/4)) bytes; k = floor(4*log2(bytes)); bin 163 is open-topped"

// BBootstrapRunPrecondition is the handoff gate (BB-14), executable BEFORE the
// deployment window instead of discovered after it: it takes two snapshots taken at
// different instants and returns the reasons the run would be VOID. An empty slice
// means the run is valid for that window.
//
// windowNanos is W, THE ESTIMAND'S OBSERVATION WINDOW, and it is a required argument
// with no default on purpose. W is UNPINNED (G-BB-1: the owner pins W and q before the
// run, advised by the Economist) and a pure fetcher has no income on the serving ledger,
// so "before it has income" does not define a window at all (R-FETCHER-INCOME). Pinning
// a W here would invent the estimand. No reading rule is hard-coded either: the analyst
// reads the cell containing W, and this function only says whether such a cell can be
// read honestly.
func BBootstrapRunPrecondition(prev, cur BBootstrapHistogram, windowNanos int64) []string {
	var bad []string
	if !cur.AgeAxisLive || cur.ClockSource != "injected" {
		bad = append(bad, "clock not injected: the age axis is dead and no cell is age-conditioned")
	}
	if cur.UptimeNanos <= prev.UptimeNanos {
		// A live-but-FROZEN clock is as fatal as an absent one (H-1).
		bad = append(bad, "clock not advancing: uptime did not move between the two snapshots")
	}
	if cur.MonotonicSource != "injected" {
		// Without a second, independent source, uptime is a wall-clock quantity
		// cross-checked against itself: a step moves every age and the bound it is
		// compared against by the SAME amount and cancels. Every clock arm below is
		// then decorative, so this one refuses the whole configuration (F-1).
		bad = append(bad, "no monotone source: uptime is a wall-clock quantity cross-checked against itself, so a clock step would reshape every age invisibly")
	}
	if cur.CensoringBoundNanos() < windowNanos {
		// The age axis is right-censored at the MONOTONE uptime, permanently
		// (R-BB-CENSORED-WINDOW). Reading the wall clock here is what let a 60-second-old
		// process be accepted for a 7-day window after an 8-day forward step.
		bad = append(bad, "uptime below W: the window asked for is longer than this process has actually been alive on a clock nothing can step, so its cell cannot be read")
	}
	if cur.Suppressed {
		// The minimum-requester floor (G-BB-11) withheld every census count, so there
		// is nothing to read. This is not a defect: below R_min there are too few
		// observations to estimate a q-quantile at any q >= 0.9 anyway, so a run that
		// reports this was void on the fit's own terms before it was void on privacy's.
		bad = append(bad, fmt.Sprintf("census below the minimum-requester floor of %d: every census count was withheld (G-BB-11) and no quantile is estimable from this few observations", BBootstrapMinRequesters))
	} else if cur.Requesters == 0 {
		bad = append(bad, "no requesters: the census is empty")
	}
	if cur.ClockStepBack {
		bad = append(bad, "clock stepped backwards past the ledger start: the wall delta went negative and the run is suspect")
	}
	if cur.AgeClampedToZero {
		bad = append(bad, "clock read earlier than a first-touch stamp: at least one age was clamped to zero and the run is suspect")
	}
	if cur.ClockSuspect {
		bad = append(bad, fmt.Sprintf("wall clock diverged from the monotone source by %d ns (tolerance %d): ages were reshaped by at least one bucket, %s", cur.ClockSkewNanos, bbClockSkewToleranceNanos, bbSkewDirection(cur.ClockSkewNanos)))
	}
	if cur.AgeExceedsUptime {
		bad = append(bad, "an occupied age bucket exceeds uptime: a foreign tick source or a clock step reached the stamps")
	}
	if cur.Unstamped > 0 {
		bad = append(bad, "unstamped requesters present: accounts predate the clock injection and carry no age")
	}
	if cur.Cells != nil && cur.Aged > 0 {
		// DEGENERACY IS "ONE BUCKET", not "bucket 0". The certified check was written
		// against the epoch clock, where every age really was 0 and bucket 0 was the
		// whole population. Under an injected wall clock bucket 0 is an age of EXACTLY
		// 0 ns, which that clock essentially never produces, so a bucket-0 test cannot
		// fire on the machine that will run this and the one case it would catch (a
		// frozen clock) is already caught above. The live degeneracy is a census with no
		// VARIATION on the age axis — everything piled in the top bucket after a long
		// uptime, or everything in bucket 1 on a short one — because the estimand is a
		// quantile CONDITIONED on age and one occupied bucket conditions on nothing.
		occupied := 0
		for b := 0; b < BBootstrapAgeBuckets; b++ {
			for _, n := range cur.Cells[b] {
				if n > 0 {
					occupied++
					break
				}
			}
		}
		if occupied < 2 {
			bad = append(bad, "degenerate age axis: every counted identity sits in ONE age bucket, so there is no age variation to condition the quantile on")
		}
	}
	return bad
}

// bbSkewDirection says which way the wall clock jumped, because the two directions are
// different failures: forward inflates every age and pushes identities UP the axis (and
// trips the censoring assertion, since the bound did not move), backward deflates them
// and pulls identities DOWN (and trips nothing else, which is why this flag exists).
func bbSkewDirection(skew int64) string {
	if skew > 0 {
		return "the wall clock jumped FORWARD, so ages are inflated and identities were pushed up the age axis"
	}
	return "the wall clock jumped BACK, so ages are deflated and identities were pulled down the age axis"
}

// bbootstrapState is the instrument's whole ledger-side state, held in one struct
// because a build tag cannot split a struct's fields across files. Ledger carries it as
// `bb`. In a default build the untagged declaration in bbootstrap_off.go is
// `struct{}` — zero bytes, no clock, no origin, nothing to inject into.
//
// EVERY FIELD HERE IS NIL/ZERO UNTIL -bbootstrap IS PASSED. Injection is gated on the
// flag (cmd/silt/bbootstrap.go), which is the second half of D-BB-BUILD-TAG.
type bbootstrapState struct {
	// obsClock is the injected ports.Clock the age axis reads. It is an OBSERVABILITY
	// clock and nothing else reads it: no accounting rule, no screen, no standing
	// calculation. Nil is the safe state — no first-touch stamp is written and the
	// snapshot publishes no age-conditioned cells. obsStartNanos is the clock's reading
	// when it was injected; the snapshot publishes the difference as UptimeNanos. That
	// is a WALL-clock quantity and is not on its own a bound on anything — the censoring
	// bound is obsMono's elapsed time, below.
	obsClock      ports.Clock
	obsStartNanos int64

	// obsMono is the SECOND, independent time source the same setter injects, and it is
	// the whole of the clock-step defence. obsClock is a wall clock (adapters/walltime
	// returns time.Now().UnixNano(), which discards Go's monotonic reading), so uptime
	// and every age are two differences of ONE stepped reading and a step cancels out of
	// any comparison between them. obsMono is stepped by nothing, so the divergence
	// between the two IS the step, published as ClockSkewNanos. Both origins are stamped
	// inside one call so the offset between them is structural rather than a
	// call-ordering hope. Nil is the safe state: the snapshot reports MonotonicSource
	// "none" and BBootstrapRunPrecondition REFUSES the run.
	obsMono          ports.MonotonicNanos
	obsMonoStartNano int64
}

// SetObservabilityClock injects the TWO time sources the B_bootstrap instrument reads
// (R2.9a, G-BB-2 and G-BB-4). It follows SetEpochSource: call it once, at construction,
// before any account exists; the daemon wires the same ports.Clock it hands every node,
// and a sim passes adapters/simclock and stays deterministic. core may not touch the wall
// clock (internal/depcheck), which is why this is a setter and not a package default.
//
// THE DAEMON CALLS IT ONLY WHEN -bbootstrap IS SET (D-BB-BUILD-TAG, 2026-09-05), and this
// method is only compiled at all under the `bbootstrap` build tag. Not calling it is what
// makes "a default node records no first-touch time" true rather than merely unpublished.
//
//   - c is the AGE clock. Every age and the published UptimeNanos come off it. In
//     production it is a wall clock and it can be stepped.
//   - mono is an INDEPENDENT elapsed-nanosecond source that cannot be stepped
//     (ports.MonotonicNanos; in cmd/silt a closure over time.Since, which uses Go's
//     monotonic reading). Nothing is measured on it. Its only job is to make a step in c
//     VISIBLE, as a divergence between two quantities that should track.
//
// ONE CALL, TWO ORIGINS, ON PURPOSE. Both start instants are stamped here, in the same
// call, so the offset between them is fixed at zero by construction. Two setters would
// make "injected at the same instant" a call-ordering hope, and any gap between them
// would read forever after as skew — which is the same class of defect (a check that
// depends on an unenforced ordering) this method exists to close.
//
// It moves nothing. Its only effect is that recordFetched begins stamping a first-FETCH
// tick — a field no standing calculation reads (see the T-axis note on
// account.firstFetchTick) and that reaches the wire only as a coarse bucket.
//
// Either argument may be nil, and nil is the safe state, not a silent downgrade: with no
// clock the snapshot reports ClockSource "none" and publishes no cells; with no monotone
// source it reports MonotonicSource "none" and BBootstrapRunPrecondition REFUSES the run
// because the censoring assertion is then a self-comparison.
func (l *Ledger) SetObservabilityClock(c ports.Clock, mono ports.MonotonicNanos) {
	l.bb.obsClock = c
	if c != nil {
		now := c.Now()
		if now < 0 {
			now = 0
		}
		l.bb.obsStartNanos = int64(now)
	}
	l.bb.obsMono = mono
	if mono != nil {
		l.bb.obsMonoStartNano = mono()
	}
}

// obsNowNanos is the clamped read of the injected clock. Negative simulated time is not
// a thing any shipped adapter produces, but a uint64 tick derived from a negative int64
// would wrap into a nonsense age, so it is clamped at the one place it enters.
func (l *Ledger) obsNowNanos() int64 {
	if l.bb.obsClock == nil {
		return 0
	}
	now := l.bb.obsClock.Now()
	if now < 0 {
		return 0
	}
	return int64(now)
}

// stampFirstFetch writes the first-fetch tick once. It is called from recordFetched
// (credit.go), the ONE write path for account.fetchedBytes, which is what makes "an
// account is stamped if and only if it is in the census" structural rather than a
// guarded assignment at N call sites. The "already set" guard is what keeps the window
// open at the FIRST fetch rather than the latest; Register's old
// call-once-at-construction placement gave that for free and this does not. The +1
// follows the bond auditor's convention (core/node/bondaudit.go): tick 0 means UNSET,
// so the first tick is never 0.
//
// THE STAMP MOVED OFF Register (G-BB-24, R-BB-STAMP-BY-ANY-PATH). Register is reached
// through acct() by every ledger path, including bond audit (core/node/bondaudit.go),
// PoR grading (core/node/por.go), bounty payment (escrow.go PayBounty) and the
// false-repair slash — so the age axis recorded first ledger touch by ANY path and
// over-stated the age of every identity that is also a DHT participant, without bound
// above by the ledger's uptime. The axis is specified as time since first FETCH (see
// the header), and recordFetched is where a fetch is recorded.
//
// IT NEVER TOUCHES account.firstSeenTick. That field belongs to RecordBondChallenge and
// records a DIFFERENT EVENT — the first bond challenge this identity answered. Both are
// wall-clock nanoseconds off the same daemon clock (corrected 2026-09-05; the earlier
// claim that the auditor's tick was a request counter was wrong), so the split is not
// about units: one shared field guarded on "unset" would keep the CHALLENGE instant for
// a peer the auditor reached first, and publish that as its fetch age.
//
// THIS FUNCTION IS THE INSTRUMENT'S ONLY WRITE, and it is the one call a build tag
// cannot remove from an untagged function. bbootstrap_off.go declares an empty twin, so
// a default build stamps nothing anywhere.
func (l *Ledger) stampFirstFetch(a *account) {
	if l.bb.obsClock == nil || a.firstFetchTick != 0 {
		return
	}
	a.firstFetchTick = uint64(l.obsNowNanos()) + 1
}

// bbootstrapByteBin returns the quarter-log2 bin of b, or -1 for b <= 0 (an account with
// no fetched bytes is not a requester and is not counted). Integer-only: bits.Len64 gives
// floor(log2 b), and the quarter is decided by three comparisons against a normalised
// mantissa. The top bin is open, so a fetch above 2^40.75 bytes saturates into bin 163
// rather than being dropped — documented on the wire by ByteBinRule.
func bbootstrapByteBin(b int64) int {
	if b <= 0 {
		return -1
	}
	e := bits.Len64(uint64(b)) - 1 // 0 … 62 for a positive int64
	m := uint64(b) << uint(62-e)   // normalised into [2^62, 2^63)
	q := 0
	switch {
	case m >= bbQuarter3:
		q = 3
	case m >= bbQuarter2:
		q = 2
	case m >= bbQuarter1:
		q = 1
	}
	k := BBootstrapBinsPerOctave*e + q
	if k >= BBootstrapByteBins {
		k = BBootstrapByteBins - 1
	}
	return k
}

// bbootstrapAgeBucket returns the bucket of a non-negative age in nanoseconds. Half-open,
// low-closed: an age exactly on an edge lands in the HIGHER bucket.
func bbootstrapAgeBucket(ageNanos int64) int {
	for i := BBootstrapAgeBuckets - 1; i > 0; i-- {
		if ageNanos >= bbAgeEdgeNanos[i] {
			return i
		}
	}
	return 0
}

// BBootstrapSnapshot builds the histogram. ONE O(R) pass over the append-only order
// slice, no sort, no hash, no allocation per requester — the only allocation is the one
// fixed 10 KiB cell array, and only when the age axis is live.
//
// IT WRITES NOTHING (Invariant A / BB-11). It indexes l.accounts directly and must never
// call FetchedBytes or any other l.acct() reader: acct() goes through Register and
// therefore CREATES an account and hands out a 500,000 grant for any id it is passed. A
// reader that mints is not a reader. TestR29aBBootstrapSnapshotWritesNothing pins it.
//
// IT IS UNEXPORTED. This is the RAW census, and the minimum-requester floor is the rule
// that the raw census does not leave this package. BBootstrapPublish is the exported
// route and it floors; making that structural is what the type system is for.
//
// Loop-owned, like every other ledger read: call it on the node's event loop.
func (l *Ledger) bBootstrapSnapshot() BBootstrapHistogram {
	out := BBootstrapHistogram{
		ClockSource:     "none",
		MonotonicSource: "none",
		AgeEdgeNanos:    bbAgeEdgeNanos,
		BinsPerOctave:   BBootstrapBinsPerOctave,
		ByteBins:        BBootstrapByteBins,
		ByteBinRule:     BBootstrapByteBinRule,
	}
	if l.bb.obsClock != nil {
		out.ClockSource = "injected"
		out.AgeAxisLive = true
		out.Cells = new([BBootstrapAgeBuckets][BBootstrapByteBins]int64)
	}

	now := l.obsNowNanos()
	wallElapsed := int64(0)
	if l.bb.obsClock != nil {
		wallElapsed = now - l.bb.obsStartNanos
		if wallElapsed >= 0 {
			out.UptimeNanos = wallElapsed
		} else {
			out.ClockStepBack = true // the clock read earlier than the ledger's own start
		}
	}
	// The cross-check (G-BB-4). Both uptimes are measured from the SAME injection
	// instant, so on a clean run they agree to within the two reads and the skew is
	// nanoseconds. A step in the wall clock moves one and not the other, and the
	// difference is the step — in a signed quantity, so the two directions stay
	// distinguishable.
	if l.bb.obsMono != nil {
		out.MonotonicSource = "injected"
		if up := l.bb.obsMono() - l.bb.obsMonoStartNano; up > 0 {
			out.MonotonicUptimeNanos = up
		}
		// From the RAW wall delta, not from the clamped UptimeNanos: a step big enough
		// to drive the wall delta negative is clamped to zero for publication, and
		// taking the skew from the clamped value would hide the largest steps there are.
		out.ClockSkewNanos = wallElapsed - out.MonotonicUptimeNanos
		out.ClockSuspect = out.ClockSkewNanos >= bbClockSkewToleranceNanos ||
			out.ClockSkewNanos <= -bbClockSkewToleranceNanos
	}

	maxOccupied := -1
	for _, nodeID := range l.order {
		a := l.accounts[nodeID]
		if a == nil || a.fetchedBytes <= 0 {
			continue
		}
		out.Requesters++
		if out.Cells == nil {
			continue // no clock: counted in the census, never placed on the age axis
		}
		if a.firstFetchTick == 0 {
			out.Unstamped++
			continue
		}
		age := now - (int64(a.firstFetchTick) - 1)
		if age < 0 {
			age = 0
			// CENSUS class: this arm can only fire if an account exists, so it is its
			// own flag and is withheld below the floor. The ledger-start arm above is
			// instrument class and keeps the ClockStepBack name.
			out.AgeClampedToZero = true // clamped, never underflowed — and never silently
		}
		bucket := bbootstrapAgeBucket(age)
		bin := bbootstrapByteBin(a.fetchedBytes)
		out.Cells[bucket][bin]++
		out.Aged++
		if bucket > maxOccupied {
			maxOccupied = bucket
		}
	}
	if maxOccupied >= 0 {
		out.MaxOccupiedAgeEdgeNanos = bbAgeEdgeNanos[maxOccupied]
		// G-BB-4: the censoring assertion, against the bound the wall clock cannot
		// move. An occupied bucket above it means either a stamp from a foreign tick
		// source or a FORWARD wall-clock step, which ages identities past a process
		// that has not been alive that long.
		out.AgeExceedsUptime = out.MaxOccupiedAgeEdgeNanos > out.CensoringBoundNanos()
	}
	return out
}
