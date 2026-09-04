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

import (
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
	// truncation — this is a census, and Aged + Unstamped == Requesters always.
	Requesters int
	// Aged is how many of those requesters were placed into a cell. Sum(Cells) == Aged.
	Aged int
	// Unstamped is how many carried no first-touch stamp (registered before the clock
	// was injected). They are counted here rather than dumped into age bucket 0, which
	// would make them look brand new — a silent age reshaping.
	Unstamped int

	// UptimeNanos is the ledger's own elapsed time since the clock was injected. It is
	// the CENSORING BOUND: no identity can carry an age greater than it, so no window
	// W larger than the longest clean uptime is measurable at all, ever (accounts are
	// in-memory and a restart destroys every one — R-BB-CENSORED-WINDOW). Carrying it
	// in the payload puts that bound in the artifact instead of in a separate scrape.
	UptimeNanos int64

	// MaxOccupiedAgeEdgeNanos is the lower edge of the highest age bucket that has any
	// count. Zero when nothing is occupied.
	MaxOccupiedAgeEdgeNanos int64
	// ClockStepBack is set when the injected clock read EARLIER than a stamp (or than
	// the ledger start). The age is clamped to zero rather than underflowed, and this
	// flag says so — a wall clock discards Go's monotonic reading (adapters/walltime),
	// so an NTP step is a real hazard and a boot-time step lands at the start of the
	// observation window (R-BB-WALLCLOCK-STEP).
	ClockStepBack bool
	// AgeExceedsUptime is the G-BB-4 assertion, evaluated at snapshot time:
	// MaxOccupiedAgeEdgeNanos must be <= UptimeNanos. It cannot be violated by
	// construction (a stamp is never earlier than the ledger start), so a true here
	// means a foreign tick source reached firstSeenTick, or the clock stepped. Either
	// way the run is suspect and the artifact says so.
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
	if cur.UptimeNanos < windowNanos {
		// The age axis is right-censored at uptime, permanently (R-BB-CENSORED-WINDOW).
		bad = append(bad, "uptime below W: the window asked for is longer than this process has been alive, so its cell cannot be read")
	}
	if cur.Requesters == 0 {
		bad = append(bad, "no requesters: the census is empty")
	}
	if cur.ClockStepBack {
		bad = append(bad, "clock stepped backwards: ages were clamped and the run is suspect")
	}
	if cur.AgeExceedsUptime {
		bad = append(bad, "an occupied age bucket exceeds uptime: a foreign tick source or a clock step reached the stamps")
	}
	if cur.Unstamped > 0 {
		bad = append(bad, "unstamped requesters present: accounts predate the clock injection and carry no age")
	}
	if cur.Cells != nil {
		degenerate := true
		for b := 1; b < BBootstrapAgeBuckets && degenerate; b++ {
			for _, n := range cur.Cells[b] {
				if n > 0 {
					degenerate = false
					break
				}
			}
		}
		if degenerate && cur.Aged > 0 {
			bad = append(bad, "degenerate age axis: every counted identity sits in age bucket 0")
		}
	}
	return bad
}

// SetObservabilityClock injects the ONE clock the B_bootstrap age axis reads (R2.9a,
// G-BB-2). It follows SetEpochSource exactly: call it once, at construction, before any
// account exists; the daemon wires the same ports.Clock it hands every node, and a sim
// passes adapters/simclock and stays deterministic. core may not touch the wall clock
// (internal/depcheck), which is why this is a setter and not a package-level default.
//
// It moves nothing. Its only effect is that Register begins stamping a first-touch tick
// — a field no standing calculation reads (see the T-axis note on account.firstSeenTick)
// and that reaches the wire only as a coarse bucket.
//
// A ledger with no clock is the safe state: the snapshot reports ClockSource "none" and
// publishes no cells.
func (l *Ledger) SetObservabilityClock(c ports.Clock) {
	l.obsClock = c
	if c != nil {
		now := c.Now()
		if now < 0 {
			now = 0
		}
		l.obsStartNanos = int64(now)
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
		ClockSource:   "none",
		AgeEdgeNanos:  bbAgeEdgeNanos,
		BinsPerOctave: BBootstrapBinsPerOctave,
		ByteBins:      BBootstrapByteBins,
		ByteBinRule:   BBootstrapByteBinRule,
	}
	if l.obsClock != nil {
		out.ClockSource = "injected"
		out.AgeAxisLive = true
		out.Cells = new([BBootstrapAgeBuckets][BBootstrapByteBins]int64)
	}

	now := l.obsNowNanos()
	if l.obsClock != nil {
		if up := now - l.obsStartNanos; up >= 0 {
			out.UptimeNanos = up
		} else {
			out.ClockStepBack = true // the clock read earlier than the ledger's own start
		}
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
		// G-BB-4: the censoring assertion. An age can only exceed uptime if a stamp
		// came from a clock other than the injected one, or the clock stepped.
		out.AgeExceedsUptime = out.MaxOccupiedAgeEdgeNanos > out.UptimeNanos
	}
	return out
}
