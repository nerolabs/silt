package credit

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
	AgeAxisLive bool

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
	// ClockStepBack is set when the injected clock read EARLIER than a stamp (or than
	// the ledger start), so a subtraction would have gone negative. The age is clamped
	// to zero rather than underflowed, and this flag says so.
	//
	// IT IS NOT THE CLOCK-STEP DETECTOR, and must not be read as one: it fires only when
	// a step is large enough to cross zero. A backward step SMALLER than the accounts'
	// ages — measured, 2 h 50 m against 3-hour-old identities — reshapes every bucket
	// and never trips it. ClockSuspect is the detector (R-BB-WALLCLOCK-STEP).
	ClockStepBack bool
	// AgeExceedsUptime is the G-BB-4 censoring assertion, evaluated at snapshot time:
	// MaxOccupiedAgeEdgeNanos must be <= CensoringBoundNanos(). With a monotone source
	// injected the two sides come from INDEPENDENT clocks, so this fires on the
	// production path — a forward wall-clock step ages identities past a bound that did
	// not move. With NO monotone source it degenerates into a comparison of the wall
	// clock against itself, invariant under every step, and can then only catch a stamp
	// written from a foreign tick source. That degeneracy is why
	// BBootstrapRunPrecondition refuses a run with no monotone source at all.
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
	if cur.Requesters == 0 {
		bad = append(bad, "no requesters: the census is empty")
	}
	if cur.ClockStepBack {
		bad = append(bad, "clock stepped backwards past zero: ages were clamped and the run is suspect")
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

// SetObservabilityClock injects the TWO time sources the B_bootstrap instrument reads
// (R2.9a, G-BB-2 and G-BB-4). It follows SetEpochSource: call it once, at construction,
// before any account exists; the daemon wires the same ports.Clock it hands every node,
// and a sim passes adapters/simclock and stays deterministic. core may not touch the wall
// clock (internal/depcheck), which is why this is a setter and not a package default.
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
// It moves nothing. Its only effect is that Register begins stamping a first-touch tick
// — a field no standing calculation reads (see the T-axis note on account.firstSeenTick)
// and that reaches the wire only as a coarse bucket.
//
// Either argument may be nil, and nil is the safe state, not a silent downgrade: with no
// clock the snapshot reports ClockSource "none" and publishes no cells; with no monotone
// source it reports MonotonicSource "none" and BBootstrapRunPrecondition REFUSES the run
// because the censoring assertion is then a self-comparison.
func (l *Ledger) SetObservabilityClock(c ports.Clock, mono ports.MonotonicNanos) {
	l.obsClock = c
	if c != nil {
		now := c.Now()
		if now < 0 {
			now = 0
		}
		l.obsStartNanos = int64(now)
	}
	l.obsMono = mono
	if mono != nil {
		l.obsMonoStartNano = mono()
	}
}

// obsNowNanos is the clamped read of the injected clock. Negative simulated time is not
// a thing any shipped adapter produces, but a uint64 tick derived from a negative int64
// would wrap into a nonsense age, so it is clamped at the one place it enters.
func (l *Ledger) obsNowNanos() int64 {
	if l.obsClock == nil {
		return 0
	}
	now := l.obsClock.Now()
	if now < 0 {
		return 0
	}
	return int64(now)
}

// stampFirstTouch is called from Register — the ONE place an account is created, which
// is what makes "written once at first touch" structural rather than a guarded
// assignment at N call sites. The +1 follows the bond auditor's convention
// (core/node/bondaudit.go): tick 0 means UNSET, so the first tick is never 0.
func (l *Ledger) stampFirstTouch(a *account) {
	if l.obsClock == nil {
		return
	}
	a.firstSeenTick = uint64(l.obsNowNanos()) + 1
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
// Loop-owned, like every other ledger read: call it on the node's event loop.
func (l *Ledger) BBootstrapSnapshot() BBootstrapHistogram {
	out := BBootstrapHistogram{
		ClockSource:     "none",
		MonotonicSource: "none",
		AgeEdgeNanos:    bbAgeEdgeNanos,
		BinsPerOctave:   BBootstrapBinsPerOctave,
		ByteBins:        BBootstrapByteBins,
		ByteBinRule:     BBootstrapByteBinRule,
	}
	if l.obsClock != nil {
		out.ClockSource = "injected"
		out.AgeAxisLive = true
		out.Cells = new([BBootstrapAgeBuckets][BBootstrapByteBins]int64)
	}

	now := l.obsNowNanos()
	wallElapsed := int64(0)
	if l.obsClock != nil {
		wallElapsed = now - l.obsStartNanos
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
	if l.obsMono != nil {
		out.MonotonicSource = "injected"
		if up := l.obsMono() - l.obsMonoStartNano; up > 0 {
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
		if a.firstSeenTick == 0 {
			out.Unstamped++
			continue
		}
		age := now - (int64(a.firstSeenTick) - 1)
		if age < 0 {
			age = 0
			out.ClockStepBack = true // clamped, never underflowed — and never silently
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
