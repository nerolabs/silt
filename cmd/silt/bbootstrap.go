//go:build bbootstrap

package main

// R2.9a — the B_bootstrap instrument's WHOLE cmd/silt surface: the -bbootstrap flag, the
// clock injection it gates, the /api/status renderer, and the one extra status key.
//
// THIS FILE COMPILES ONLY UNDER THE `bbootstrap` BUILD TAG (D-BB-BUILD-TAG, ratified
// 2026-09-05; docs/decisions.md). A default `go build` gets bbootstrap_off.go instead:
// no flag, no injection, no renderer, no histogram type. `silt daemon -bbootstrap` on a
// default binary fails at flag parse with "flag provided but not defined", which is the
// intended answer — the mechanism is not there to enable.
//
// TWO GATES, NOT ONE, AND THE SECOND IS THE CHANGE. The tag decides whether the
// mechanism EXISTS. Inside a tagged build the flag decides whether it RECORDS: the
// injection below is conditional, so a tagged binary run without -bbootstrap stamps no
// account and its ledger holds exactly what it held before R2.9a — (identity,
// cumulative bytes), with no `when`.

import (
	"flag"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// statusExtras is the optional tail of GET /api/status. Under this tag it carries the
// one R2.9a key; untagged (bbootstrap_off.go) it is empty and contributes nothing.
type statusExtras struct {
	// R2.9a: ABSENT unless -bbootstrap is set. Absent and empty are different objects:
	// a reader that sees no key knows the instrument is off, and a reader that sees the
	// key with zero requesters knows the instrument is on and the ledger is idle.
	BBootstrap *bBootstrapInfo `json:"bBootstrap,omitempty"`
}

// registerBBootstrapFlag declares -bbootstrap on fs and returns a reader for it. It
// returns a closure rather than writing a package-level variable so that two daemons in
// one process (the tests do this) do not share one flag value.
func registerBBootstrapFlag(fs *flag.FlagSet) func() bool {
	on := fs.Bool("bbootstrap", false, "R2.9a: RECORD AND PUBLISH the B_bootstrap histogram on GET /api/status (ROADMAP R2.9a). A full-census 2-D COUNT histogram over (identity age × log2 fetched bytes) — 8 age buckets × 164 quarter-log2 byte bins, ~10 KiB, constant in the requester count. Counts ONLY: no requester id, no salted label, no per-identity row, no exact age, and no per-cell byte SUM (a cell sum with count 1 is that identity's exact byte total in disguise). The age axis is the boot-relative elapsed tick from the node's injected clock, right-censored at the ledger's uptime — a restart destroys every account, so no window longer than the longest clean uptime is measurable at all. It is the instrument D-R2.9-DIRECTION sentence 4 makes a precondition of pinning grant/r, and it is INSTRUMENTATION ONLY: no conservation rule, no standing calculation and no economic rule reads it. This FLAG IS ONLY PRESENT IN A BINARY BUILT WITH -tags bbootstrap (D-BB-BUILD-TAG); a default silt binary rejects it, because the mechanism is not compiled in. DEFAULT OFF, and off means NOT RECORDED, not merely unpublished: no observability clock is injected, so no account carries a first-touch time. Turning it on takes a restart and then a wait for the population to re-stamp. /api/status needs no token, so anything published there is world-readable wherever -ui is bound off loopback")
	return func() bool { return *on }
}

// bbootstrapInject wires the TWO observability time sources into the ledger — AND ONLY
// WHEN THE OPERATOR ASKED FOR THE INSTRUMENT.
//
// THE TRADE THIS REVERSES, STATED WHERE THE OLD COMMENT CLAIMED THE OPPOSITE. Until
// 2026-09-05 this call was UNCONDITIONAL, and daemon.go said so deliberately: injecting
// always meant an operator who flips -bbootstrap on at the next restart gets an
// already-stamped population from that boot's first touch, rather than a ledger full of
// unstamped accounts. That property is GONE, on purpose. A tagged operator must now
// restart WITH the flag on and then WAIT for the population to re-stamp, which costs a
// window of measurable uptime (G-BB-15 wants monotone uptime >= 2x the read bucket's
// upper edge, so the wait is the run's own precondition and not an extra cost on top).
//
// It is the intended trade because the instrument is run ONCE, for ONE series, on ONE
// deployment. Paying a restart-plus-wait once buys the property that every silt node
// that was not asked for the instrument records no first-seen time for any fetcher at
// all — which is the claim Part VI Don't #3 is about, and which an unconditional
// injection makes false on every node in the network.
//
// TWO SOURCES, ONE CALL (G-BB-4). clk is a wall clock — adapters/walltime returns
// time.Now().UnixNano(), which discards Go's monotonic reading — so uptime and every age
// come off one steppable reading and a step cancels out of the comparison between them.
// obsStart is a time.Time, which DOES carry the monotonic reading, and time.Since reads
// it, so the closure below is elapsed time nothing can step: not an NTP correction, not
// an operator, not a container clock. Nothing is measured on it; it exists so the wall
// clock's divergence from it is visible in the artifact.
//
// Call it before the ledger is reachable — before nd.SetLedger — so that no account can
// be created unstamped ahead of the injection. daemon.go's call site is order-anchored
// on that by TestR29aDaemonInjectsBothClocksBeforeTheLedgerIsReachable.
func bbootstrapInject(l *credit.Ledger, clk ports.Clock, on bool) {
	if !on {
		// The safe state, and it is a REAL state, not a degraded one: with no clock
		// injected Register stamps nothing, bBootstrapSnapshot reports ClockSource
		// "none" with AgeAxisLive false and nil cells, and BBootstrapRunPrecondition
		// refuses the run outright. "Off" and "idle" stay different objects.
		return
	}
	obsStart := time.Now()
	l.SetObservabilityClock(clk, func() int64 { return int64(time.Since(obsStart)) })
}

// bbootstrapWireUI installs the /api/status renderer, and only when the flag is set. An
// unasked-for instrument is not merely un-rendered: uiServer.statusExtra stays nil, so
// there is no path from the handler to the ledger at all.
func bbootstrapWireUI(s *uiServer, on bool) {
	if !on {
		return
	}
	s.statusExtra = func(x *statusExtras) { x.BBootstrap = s.bBootstrapSnapshot() }
}

// bBootstrapInfo is the published B_bootstrap histogram (R2.9a): a full-census 2-D
// COUNT histogram over (identity age × log2 fetched bytes), the instrument
// D-R2.9-DIRECTION sentence 4 requires before the affordability ratio grant/r can be
// pinned. cloudtest measures its own synthetic fetch plan, so the numbers have to come
// off a deployment with real users.
//
// WHAT IT DELIBERATELY IS NOT (immutable #4, refuse-to-surveil). Counts, and nothing
// else. No requester id — not even a salted label — no object root, no per-identity row,
// no exact age, and no per-cell byte SUM (a cell sum with count 1 is that identity's
// exact byte total in disguise). An analyst can read Q_q(bytes | age bucket) from it and
// can learn nothing about who fetched what.
//
// DEFAULT OFF (-bbootstrap). GET /api/status needs no token, so anything published here
// is world-readable wherever -ui is bound off loopback; reversing a default is cheap now
// and expensive after adoption, and the measurement needs exactly one deployment.
type bBootstrapInfo struct {
	ClockSource string `json:"clockSource"` // "injected" | "none" — the age axis self-report (H-1)
	AgeAxisLive bool   `json:"ageAxisLive"` // false ⇒ cells is null; NEVER an all-zero age column

	// THE MINIMUM-REQUESTER FLOOR (G-BB-11). suppressed is true when the census held
	// fewer than credit.BBootstrapMinRequesters requesters, and then every census count
	// below is ABSENT FROM THE JSON — not published as zero. A published zero would be a
	// false total, and a reader that sums the block would read it as a measured one; a
	// missing key cannot be misread. The pointers exist for exactly that: omitempty on a
	// non-nil pointer still emits a legitimate 0, so "on and idle" and "below the floor"
	// stay different objects, which is the same distinction the absent-vs-empty rule for
	// the whole block draws.
	//
	// WHY THE FLOOR EXISTS, in one line, because the obvious reading is wrong: requesters
	// is THE ANONYMITY-SET SIZE, and publishing it is what makes the UNCONDITIONALLY
	// published stats.bytesServed and durability.objects[].funded deltas attributable to
	// one identity. Suppressing cells while still publishing requesters would close
	// nothing.
	//
	// WHAT THE FLOOR IS NOT, corrected here because the first version of this comment
	// implied the opposite (RE-CERTIFICATION 2026-09-05 §2.5). IT BOUNDS THE PUBLISHED
	// CENSUS **COUNT**, NOT THE ANONYMITY **SET**. The census population is the set of
	// identities that fetched, an identity is just a keypair, and the serve path has no
	// admission control — so an observer that can FETCH lifts the floor for nine keypairs
	// and one chunk each, permanently for the process's lifetime. The floor is a FIT
	// PRECONDITION and a defence against a reader that CANNOT fetch. It is not a privacy
	// mitigation against a capable adversary, and the Don't #3 question is not answered by
	// it (R-BB-CENSUS-SYBIL-PAD, R-BB-ANONYMITY-SET-SIZE, both open). Nor does it close
	// the delta trajectory of a POLLED series (R-BB-DELTA-TRAJECTORY, open, bounded by
	// poll rate). And `suppressed: true` is itself a disclosure: a published upper bound
	// of R_min − 1 on the anonymity set (R-BB-SUPPRESSED-IS-A-DISCLOSURE).
	//
	// THE RULE BEHIND WHICH KEYS ARE ABSENT IS A PROPERTY, NOT A LIST (G-BB-11′): below
	// the floor this block is a function of the INSTRUMENT fields alone — the injected
	// clock sources, their injection instants and the compiled axis constants — plus this
	// one bit. Every key below that is a pointer is CENSUS class. BB-20 asserts the
	// property on these exact bytes.
	Suppressed bool `json:"suppressed"`

	Requesters *int `json:"requesters,omitempty"` // the TRUE total: every account with fetched bytes > 0. ABSENT below the floor
	Aged       *int `json:"aged,omitempty"`       // how many landed in a cell; equals the sum of all cells. ABSENT below the floor
	Unstamped  *int `json:"unstamped,omitempty"`  // counted, never dumped into age bucket 0. ABSENT below the floor

	UptimeNanos             int64  `json:"uptimeNanos"`                       // elapsed on the WALL clock; moves with an NTP step, so not a bound on its own
	MaxOccupiedAgeEdgeNanos *int64 `json:"maxOccupiedAgeEdgeNanos,omitempty"` // lower edge of the highest occupied bucket. ABSENT below the floor: at a degenerate anonymity set it is a per-identity age
	ClockStepBack           bool   `json:"clockStepBack"`                     // the wall clock read earlier than the LEDGER START. Instrument class — it touches no account — so it survives suppression. NOT the step detector; see clockSuspect
	AgeClampedToZero        *bool  `json:"ageClampedToZero,omitempty"`        // the clock read earlier than an account's own stamp, so that age was clamped. CENSUS class: ABSENT below the floor
	AgeExceedsUptime        *bool  `json:"ageExceedsUptime,omitempty"`        // the G-BB-4 censoring assertion failed — the run is suspect. CENSUS class (a threshold on maxOccupiedAgeEdgeNanos, which the floor withholds): ABSENT below the floor

	// The clock cross-check (G-BB-4 / BB-13). uptimeNanos and every age come off ONE
	// wall clock, so a step moves both and cancels; monotonicUptimeNanos comes off a
	// source nothing can step, and the difference between them IS the step. It is
	// published as a signed number as well as a flag, because the two directions are
	// different failures and an analyst judges the magnitude against their own W.
	MonotonicSource      string `json:"monotonicSource"`      // "injected" | "none" — the cross-check's self-report
	MonotonicUptimeNanos int64  `json:"monotonicUptimeNanos"` // the REAL censoring bound: no age can exceed it
	ClockSkewNanos       int64  `json:"clockSkewNanos"`       // wall − monotone; positive = the wall clock jumped forward
	ClockSuspect         bool   `json:"clockSuspect"`         // the divergence moved identities at least a whole age bucket

	AgeEdgeNanos  []int64 `json:"ageEdgeNanos"`  // lower edges; bucket i = [i, i+1), last open
	AgeBuckets    int     `json:"ageBuckets"`    //
	BinsPerOctave int     `json:"binsPerOctave"` // 4 — quarter-log2 byte bins
	ByteBins      int     `json:"byteBins"`      // 164
	ByteBinRule   string  `json:"byteBinRule"`   // the byte axis stated exactly, as a closed form

	Cells [][]int64 `json:"cells"` // [ageBucket][byteBin] counts; null when the age axis is not live
}

// bBootstrapSnapshot renders the histogram for the wire, or nil when no ledger
// implements the export (the block is then ABSENT from /api/status, not
// present-and-empty).
//
// THE -bbootstrap CHECK IS NO LONGER HERE. It moved to the wiring: bbootstrapWireUI
// installs this renderer as uiServer.statusExtra only when the flag is set, so an
// unasked-for instrument is not merely un-rendered, it is unreachable — and, one layer
// down, unrecorded, because the same flag gates the clock injection (D-BB-BUILD-TAG).
func (s *uiServer) bBootstrapSnapshot() *bBootstrapInfo {
	h, ok := s.nd.BBootstrap()
	if !ok {
		return nil
	}
	out := &bBootstrapInfo{
		ClockSource:          h.ClockSource,
		AgeAxisLive:          h.AgeAxisLive,
		Suppressed:           h.Suppressed,
		UptimeNanos:          h.UptimeNanos,
		ClockStepBack:        h.ClockStepBack,
		MonotonicSource:      h.MonotonicSource,
		MonotonicUptimeNanos: h.MonotonicUptimeNanos,
		ClockSkewNanos:       h.ClockSkewNanos,
		ClockSuspect:         h.ClockSuspect,
		AgeEdgeNanos:         h.AgeEdgeNanos[:],
		AgeBuckets:           credit.BBootstrapAgeBuckets,
		BinsPerOctave:        h.BinsPerOctave,
		ByteBins:             h.ByteBins,
		ByteBinRule:          h.ByteBinRule,
	}
	if !h.Suppressed {
		// Above the floor the census counts are published, INCLUDING legitimate zeros:
		// a node with the instrument on and no traffic reports requesters 0, which a
		// reader must be able to tell from a withheld count.
		out.Requesters = &h.Requesters
		out.Aged = &h.Aged
		out.Unstamped = &h.Unstamped
		out.MaxOccupiedAgeEdgeNanos = &h.MaxOccupiedAgeEdgeNanos
		// The two CENSUS-class corruption flags. They ride with the counts, not with
		// the clock apparatus: ageExceedsUptime is a threshold on the withheld
		// maxOccupiedAgeEdgeNanos, and ageClampedToZero can only fire if an account
		// exists. An operator loses nothing below the floor — clockSuspect and the raw
		// signed clockSkewNanos report the same corruption and read no account.
		out.AgeClampedToZero = &h.AgeClampedToZero
		out.AgeExceedsUptime = &h.AgeExceedsUptime
	}
	if h.Cells != nil {
		out.Cells = make([][]int64, credit.BBootstrapAgeBuckets)
		for i := range h.Cells {
			out.Cells[i] = h.Cells[i][:]
		}
	}
	return out
}
