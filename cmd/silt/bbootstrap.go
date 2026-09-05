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
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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
	// BBootstrapWithheld is G-BB-12′'s marker: the instrument is ON and this reader is
	// NOT the operator, so the block is not served to it. It is a SIBLING key, never a
	// field inside the block — a zero-valued bBootstrapInfo would emit sixteen keys of
	// false facts, including ageAxisLive:false + cells:null, which is byte-identical to
	// the documented "no clock injected" state (blind PE ruling
	// RULING-R2.9a-G-BB-12-design-2026-09-05 S1). Three wire states, each a distinct
	// key set: neither key (default build, or tagged with the flag off); this key alone
	// (on; not the operator); bBootstrap alone (on; the operator).
	BBootstrapWithheld bool `json:"bBootstrapWithheld,omitempty"`
}

// withholdBBootstrap is the R2.9a clause of uiServer.readerView, the (b) half of G-BB-12′:
// the B_bootstrap block is served only to a reader that presented the API token in the
// Authorization header. Anyone else — a reverse proxy or port-forward arriving from
// loopback (Red-team F5), a co-tenant process that cannot read the 0600 token file, a
// cross-origin page (the observatory, or any http://localhost:* origin the guard
// reflects), a ?token= query that could have come from a log — gets the withheld marker
// and no block. Together with bbootstrapRefuseRoutableBind (which keeps the token from
// ever needing to leave the host) this is the code establishing that the reader can
// read <store>/ui-token, mode 0600 — the one checkable correlate of "is the operator".
//
// x is the COPY's extras (readerView copies the cached document first). This function
// ASSIGNS nil into the copy and never mutates *x.BBootstrap: the pointer is shared with
// the cache, and clearing through it would withhold the block from the operator's next
// read too (S2). With the instrument off x.BBootstrap is already nil and neither key is
// emitted — the marker must not appear on a node that has nothing to withhold.
func withholdBBootstrap(x *statusExtras, operator bool) {
	if x.BBootstrap == nil || operator {
		return
	}
	x.BBootstrap = nil
	x.BBootstrapWithheld = true
}

// registerBBootstrapFlag declares -bbootstrap on fs and returns a reader for it. It
// returns a closure rather than writing a package-level variable so that two daemons in
// one process (the tests do this) do not share one flag value.
func registerBBootstrapFlag(fs *flag.FlagSet) func() bool {
	on := fs.Bool("bbootstrap", false, "R2.9a: RECORD AND PUBLISH the B_bootstrap histogram on GET /api/status (ROADMAP R2.9a). A full-census 2-D COUNT histogram over (identity age × log2 fetched bytes) — 8 age buckets × 41 log2 byte bins (one per doubling, 1 byte … 1 TiB), 328 counters, about 1.3 KiB on the wire, constant in the requester count. Counts ONLY: no requester id, no salted label, no per-identity row, no exact age, and no per-cell byte SUM (a cell sum with count 1 is that identity's exact byte total in disguise). The age axis is the boot-relative elapsed tick from the node's injected clock, right-censored at the ledger's uptime — a restart destroys every account, so no window longer than the longest clean uptime is measurable at all. It is the instrument D-R2.9-DIRECTION sentence 4 makes a precondition of pinning grant/r, and it is INSTRUMENTATION ONLY: no conservation rule, no standing calculation and no economic rule reads it. This FLAG IS ONLY PRESENT IN A BINARY BUILT WITH -tags bbootstrap (D-BB-BUILD-TAG); a default silt binary rejects it, because the mechanism is not compiled in. DEFAULT OFF, and off means NOT RECORDED, not merely unpublished: no observability clock is injected, so no account carries a first-touch time. Turning it on takes a restart and then a wait for the population to re-stamp. THE READER MUST BE THE OPERATOR (G-BB-12′): the daemon refuses to start unless -ui is a loopback bind and <store>/ui-token is owner-only, and the block is served only to a request carrying that token in the Authorization header — read it with: curl -H \"Authorization: Bearer $(cat <store>/ui-token)\" http://127.0.0.1:<port>/api/status. Any other reader sees bBootstrapWithheld:true and no block")
	return func() bool { return *on }
}

// bbootstrapRefuseRoutableBind is G-BB-13′ Part A, owner-ratified 2026-09-05 ("refuse at
// startup"; docs/decisions.md D-R2.9a-RUN-CALLS item 4), and the (a) half of G-BB-12′: a
// tagged daemon REFUSES TO START when -bbootstrap is set and -ui names anything but a
// loopback address. The predicate is isLocalHost — the same one the request guard uses
// for the Host header — applied here to the OPERATOR'S OWN flag string before anything is
// bound, so "127.0.0.1:8081", "localhost:8081", "[::1]:8081" and any 127/8 literal pass,
// and "0.0.0.0:8081", "[::]:8081", ":8081" (all interfaces) and any LAN or public
// literal or hostname are refused with both flag names in the message.
//
// REFUSE THE COMBINATION AT STARTUP, NEVER OMIT THE BLOCK AT REQUEST TIME. A block that
// silently went missing on a routable bind would read as an idle instrument, which is
// the absent-vs-empty ambiguity this file forbids everywhere else (the certification's
// own words for form (a)). The shape is the -dht-address-reserve precedent in daemon.go:
// a security-relevant flag combination the code will not run, stated as a constraint.
//
// WHAT THIS DOES AND DOES NOT ESTABLISH, so a bind check is not read as more than it is.
// It removes the block from every reader that is not on this host, and it removes the
// reason for the API token to ever leave the host (the operator scrapes locally). It does
// NOT establish that a loopback connection IS the operator: a reverse proxy, an SSH
// forward, a port-forward and a co-tenant process all arrive from loopback (Red-team F5),
// and Tor's man page names the same residual for its own MetricsPortPolicy ("allowing
// localhost, every user on the server will be able to access it"). The reader-is-the-
// operator half of G-BB-12′ is the token requirement on the block, decided separately
// (docs/thinking/2026-09-05-r29a-g12-reader-is-operator.md).
func bbootstrapRefuseRoutableBind(uiAddr string, on bool) error {
	if !on || uiAddr == "" || isLocalHost(uiAddr) {
		return nil
	}
	return fmt.Errorf("-bbootstrap with -ui %q refused: the B_bootstrap histogram is published on GET /api/status and may only be served on a loopback bind (127.0.0.1, ::1, localhost); bind -ui to loopback and read it locally (curl, ssh -L, docker exec) — G-BB-13′ Part A, owner-ratified 2026-09-05", uiAddr)
}

// bbootstrapRefuseInsecureTokenFile is the second startup refusal G-BB-12′ needs, and it
// exists because the (b) half makes a file mode load-bearing. withholdBBootstrap serves
// the block to whoever presents <store>/ui-token, and the reason that establishes "the
// reader is the operator" is that the file is owner-only (0600, written so by
// loadOrCreateUIToken). loadOrCreateUIToken READS an existing file without checking its
// mode, so a 0644 token — a restored backup, a `cp` without -p, a volume copy — would
// silently hand every local user the operator predicate with every gate green (blind PE
// ruling RULING-R2.9a-G-BB-12-design-2026-09-05 S5). With -bbootstrap set, a token file
// readable by group or other refuses the start and names the fix. A missing file is fine:
// it is about to be created 0600. Tagged build only; a default daemon is untouched.
func bbootstrapRefuseInsecureTokenFile(storeDir string, on bool) error {
	if !on {
		return nil
	}
	path := filepath.Join(storeDir, "ui-token")
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("-bbootstrap: cannot stat %s: %w", path, err)
	}
	if why := bbootstrapTokenFileIssue(fi, os.Geteuid()); why != "" {
		return fmt.Errorf("-bbootstrap refused: %s %s; the B_bootstrap block is served to whoever presents this token, so it must be the daemon user's and owner-only — run `chown $(id -u) %s && chmod 0600 %s` (G-BB-12′)", path, why, path, path)
	}
	return nil
}

// bbootstrapTokenFileIssue is the pure predicate behind bbootstrapRefuseInsecureTokenFile:
// "" when the token file is the caller's and owner-only, otherwise the one-line reason.
// Two clauses, because "the reader knows a secret in a 0600 file" is only "the reader is
// the operator" when nobody else could have WRITTEN that file: (1) the mode grants no
// group or other bit; (2) the file is owned by the daemon's effective user — a token
// pre-planted by another user in a shared or bind-mounted store, 0600 to THEM, would
// otherwise be adopted by loadOrCreateUIToken and hand that user the operator predicate
// (blind PE code ruling RULING-R2.9a-G-BB-12-code-32adf76 Finding 2, measured). On a
// platform whose FileInfo carries no owner the second clause is skipped, stated here so
// the skip is a known gap and not a silent one.
func bbootstrapTokenFileIssue(fi os.FileInfo, euid int) string {
	if mode := fi.Mode().Perm(); mode&0o077 != 0 {
		return fmt.Sprintf("is mode %04o, readable beyond its owner", mode)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && uint64(st.Uid) != uint64(euid) {
		return fmt.Sprintf("is owned by uid %d, not the daemon's uid %d", st.Uid, euid)
	}
	return ""
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
// off a deployment with real users. WHERE grant/r LANDS, so the reader does not inherit
// the earlier error (G-BB-22): too low is build-immutable #4 (the honest-participation
// floor); too high is Don't #7 / T-AR / build-immutable #8 (unpaid real work at the edge
// tier, a free-resource surface). Never M0's Sybil corner — the grant mints balance and
// standing is bond-only (Invariant A).
//
// WHAT IT DELIBERATELY IS NOT (immutable #4, refuse-to-surveil). Counts, and nothing
// else. No requester id — not even a salted label — no object root, no per-identity row,
// no exact age, and no per-cell byte SUM (a cell sum with count 1 is that identity's
// exact byte total in disguise). An analyst can read Q_q(bytes | age bucket) from it and
// can learn nothing about who fetched what.
//
// DEFAULT OFF (-bbootstrap), and served ONLY TO THE OPERATOR (G-BB-12′): a loopback
// bind is refused at startup otherwise, and the block itself is withheld from any request
// that does not carry the owner-only API token in the Authorization header
// (withholdBBootstrap). Reversing a default is cheap now and expensive after adoption,
// and the measurement needs exactly one deployment.
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
	BinsPerOctave int     `json:"binsPerOctave"` // 1 — one log2 byte bin per doubling (G-BB-23, ratified 2026-09-05)
	ByteBins      int     `json:"byteBins"`      // 41
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
