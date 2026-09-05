//go:build bbootstrap

package credit

// R2.9a — the B_bootstrap histogram's gates. Each test names the Tester gate from
// RESEARCH CERTIFICATION R2.9a-Bbootstrap-instrument-sufficiency (2026-09-04) §7 that it
// encodes, and each carries the ablation that proves it can go RED.
//
// BB-12 (core/credit imports NO randomness source at all) is NOT re-encoded here: it is
// already the whole point of internal/depcheck's TestCoreImportsNoAdaptersAndNoEffects,
// which walks every non-test file under core/ and fails on crypto/rand, math/rand and
// math/rand/v2. Duplicating it here would be a second, weaker copy of a stronger gate.

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// bbClock is BOTH time sources the instrument reads (G-BB-2 and G-BB-4), in one double,
// because the property under test is the RELATIONSHIP between them.
//
//   - `now` is the wall clock the age axis rides. A test moves it by hand, forwards and
//     backwards, because in production it is adapters/walltime and can be stepped.
//   - `monotonic()` is the independent source nothing can step. It is DERIVED as
//     `now − stepped`, so it tracks the wall clock exactly for as long as the wall clock
//     is honest, and stays put across a step.
//
// So a test moves honest elapsed time by assigning `now`, and models an NTP step by
// calling `step`, which moves only the wall reading. AfterFunc is never used by the
// instrument (it only ever calls Now), so it is a no-op.
type bbClock struct {
	now     ports.Time // the wall clock: steppable, and what every age is measured on
	stepped int64      // the accumulated wall-clock STEP; monotonic time excludes it
}

func (c *bbClock) Now() ports.Time                         { return c.now }
func (c *bbClock) AfterFunc(ports.Duration, func()) func() { return func() {} }

// monotonic is elapsed time on the source an operator, an NTP correction or a container
// cannot move: everything the wall clock has done MINUS everything it has jumped.
func (c *bbClock) monotonic() int64 { return int64(c.now) - c.stepped }

// step models an NTP step of d (forward for d > 0, backward for d < 0): the wall clock
// jumps, real elapsed time does not.
func (c *bbClock) step(d int64) {
	c.now += ports.Time(d)
	c.stepped += d
}

var _ ports.Clock = (*bbClock)(nil)
var _ ports.MonotonicNanos = (&bbClock{}).monotonic

const (
	bbMinute = int64(60 * 1e9)
	bbHour   = 60 * bbMinute
	bbDay    = 24 * bbHour
)

// reqID makes distinct requester ids from an int, so a test can build R of them.
func reqID(i int) ports.NodeID {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(i))
	return ports.HashBytes(b[:])
}

// fetched books `bytes` of fetched traffic to requester r, from a server that is not r
// (self-serving earns nothing). It goes through the real press, RecordServe.
func fetched(l *Ledger, server ports.NodeID, r ports.NodeID, bytes int64) {
	l.RecordServe(server, r, ports.Hash{}, bytes)
}

// sumCells totals every counter in the histogram.
func sumCells(h BBootstrapHistogram) int64 {
	if h.Cells == nil {
		return 0
	}
	var n int64
	for a := range h.Cells {
		for _, c := range h.Cells[a] {
			n += c
		}
	}
	return n
}

// --- BB-1: the dead-clock oracle -------------------------------------------------

// TestR29aDeadClockPublishesNoAgeCells is BB-1. A ledger with NO clock injected must
// publish an explicit clock-source field and NO age-conditioned cells — never an
// all-zero age column a reader could mistake for a genuinely young population. The
// census itself still publishes, so "the instrument is off" and "nobody fetched" stay
// different objects.
func TestR29aDeadClockPublishesNoAgeCells(t *testing.T) {
	l := New(50_000, 0)
	srv := id(1)
	for i := 0; i < 5; i++ {
		fetched(l, srv, reqID(i), 1<<20)
	}

	h := l.bBootstrapSnapshot()
	if h.ClockSource != "none" {
		t.Fatalf("clockSource = %q with no clock injected, want %q — the payload must SELF-REPORT the dead axis", h.ClockSource, "none")
	}
	if h.AgeAxisLive {
		t.Fatalf("ageAxisLive = true with no clock injected")
	}
	if h.Cells != nil {
		t.Fatalf("cells published with no clock injected: an all-zero age column is indistinguishable from a young population, which is exactly the failure BB-1 exists to stop")
	}
	if h.Requesters != 5 {
		t.Fatalf("requesters = %d, want 5 — the census must still publish so 'disabled' and 'empty' stay distinguishable", h.Requesters)
	}
	if h.Aged != 0 || h.Unstamped != 0 {
		t.Fatalf("aged = %d, unstamped = %d, want 0/0 — nothing is placed on a dead axis", h.Aged, h.Unstamped)
	}
	if h.UptimeNanos != 0 {
		t.Fatalf("uptimeNanos = %d with no clock, want 0", h.UptimeNanos)
	}

	// The precondition check must call this run VOID (BB-14's first conjunct).
	if bad := BBootstrapRunPrecondition(h, h, bbHour); len(bad) == 0 {
		t.Fatalf("BBootstrapRunPrecondition accepted a dead-clock snapshot")
	}
}

// --- BB-2: age-axis liveness, and the boundary side ------------------------------

// TestR29aAgeAxisLivenessAndBoundaries is BB-2. With an injected clock advanced by T, an
// account registered at t = 0 lands in the bucket containing T; and every edge is driven
// exactly, at the edge and one nanosecond below it, so the documented half-open
// [lo, hi) convention is pinned rather than assumed.
func TestR29aAgeAxisLivenessAndBoundaries(t *testing.T) {
	// Liveness first: one account, the clock advanced by a known T.
	for _, tc := range []struct {
		name string
		adv  int64
		want int
	}{
		{"exactly zero", 0, 0},
		{"one nanosecond", 1, 1},
		{"thirty seconds", 30 * 1e9, 1},
		{"five minutes", 5 * bbMinute, 2},
		{"half an hour", 30 * bbMinute, 3},
		{"three hours", 3 * bbHour, 4},
		{"twelve hours", 12 * bbHour, 5},
		{"three days", 3 * bbDay, 6},
		{"thirty days", 30 * bbDay, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clk := &bbClock{now: 1_000}
			l := New(50_000, 0)
			l.SetObservabilityClock(clk, clk.monotonic)
			fetched(l, id(1), reqID(0), 4096)
			clk.now = ports.Time(1_000 + tc.adv)

			h := l.bBootstrapSnapshot()
			if !h.AgeAxisLive || h.ClockSource != "injected" {
				t.Fatalf("age axis not live with a clock injected: live=%v source=%q", h.AgeAxisLive, h.ClockSource)
			}
			if h.Aged != 1 {
				t.Fatalf("aged = %d, want 1", h.Aged)
			}
			if got := h.Cells[tc.want]; got[bbootstrapByteBin(4096)] != 1 {
				t.Fatalf("age %d ns did not land in bucket %d (edge %d ns); cells row = %v",
					tc.adv, tc.want, h.AgeEdgeNanos[tc.want], nonZero(got))
			}
		})
	}

	// Then every edge, at the edge and one ns below: low-closed, high-open.
	for i := 1; i < BBootstrapAgeBuckets; i++ {
		edge := bbAgeEdgeNanos[i]
		if got := bbootstrapAgeBucket(edge); got != i {
			t.Fatalf("age exactly on edge %d (%d ns) landed in bucket %d, want %d — an edge is LOW-CLOSED", i, edge, got, i)
		}
		if got := bbootstrapAgeBucket(edge - 1); got != i-1 {
			t.Fatalf("age one ns below edge %d (%d ns) landed in bucket %d, want %d — an edge is HIGH-OPEN", i, edge-1, got, i-1)
		}
	}
}

func nonZero(row [BBootstrapByteBins]int64) map[int]int64 {
	out := map[int]int64{}
	for i, v := range row {
		if v != 0 {
			out[i] = v
		}
	}
	return out
}

// --- BB-3: census completeness ---------------------------------------------------

// TestR29aCensusIsCompleteAndUncapped is BB-3. Every requester with fetched bytes is
// counted exactly once: the sum over all cells equals `aged`, which equals `requesters`,
// with no cap and no truncation anywhere. R is pushed well past the 4,096 row cap the
// refuted shape carried, because that cap is precisely what this gate must never permit
// back in.
func TestR29aCensusIsCompleteAndUncapped(t *testing.T) {
	const R = 10_000 // 2.4× the retired MaxRequesterFetchRows
	clk := &bbClock{now: 1}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	srv := id(1)
	for i := 0; i < R; i++ {
		clk.now = ports.Time(1 + int64(i)*1e6) // arrivals spread over time
		fetched(l, srv, reqID(i), int64(1+i))
	}
	clk.now = ports.Time(1 + int64(R)*1e6 + bbDay)

	h := l.bBootstrapSnapshot()
	if h.Requesters != R {
		t.Fatalf("requesters = %d, want %d", h.Requesters, R)
	}
	if h.Aged != R {
		t.Fatalf("aged = %d, want %d — every requester must land in a cell", h.Aged, R)
	}
	if got := sumCells(h); got != int64(R) {
		t.Fatalf("sum over all cells = %d, want %d — a census loses nobody", got, R)
	}
	if h.Unstamped != 0 {
		t.Fatalf("unstamped = %d, want 0", h.Unstamped)
	}
}

// --- BB-4: the selection-bias oracle ---------------------------------------------

// TestR29aYoungCellsAreExactUnderASkewedPopulation is BB-4 — the refutation of top-k,
// encoded. The population is built so that the YOUNG identities fetch little and the OLD
// identities fetch a lot, which is the real shape and is exactly the shape a
// retain-top-k-by-bytes rule destroys: it removes, from every age cell, the identities
// below the threshold, so the young cells empty first and hardest. Under a full census
// the young cells are exact, whatever the old ones do.
//
// ABLATION (run 2026-09-04, restored): capping the scan at the 4,096 largest fetchers
// takes the young count from 8,000 to 0 and this test RED.
func TestR29aYoungCellsAreExactUnderASkewedPopulation(t *testing.T) {
	const young, old = 8_000, 2_000
	clk := &bbClock{now: 1}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	srv := id(1)

	// The old cohort registers first and fetches large.
	for i := 0; i < old; i++ {
		fetched(l, srv, reqID(i), 1<<30) // 1 GiB each
	}
	clk.now = ports.Time(1 + 3*bbDay)
	// The young cohort registers three days later and fetches small.
	for i := old; i < old+young; i++ {
		fetched(l, srv, reqID(i), 4096)
	}
	clk.now = ports.Time(1 + 3*bbDay + 30*1e9) // 30 s after the young cohort arrived

	h := l.bBootstrapSnapshot()
	youngBucket := bbootstrapAgeBucket(30 * 1e9) // "under a minute"
	oldBucket := bbootstrapAgeBucket(3*bbDay + 30*1e9)
	if youngBucket == oldBucket {
		t.Fatalf("the fixture does not separate the cohorts: both land in bucket %d", youngBucket)
	}
	if got := h.Cells[youngBucket][bbootstrapByteBin(4096)]; got != young {
		t.Fatalf("young cell count = %d, want %d — the young cells are where the fit reads, and a rule that selects on BYTES empties them first", got, young)
	}
	if got := h.Cells[oldBucket][bbootstrapByteBin(1<<30)]; got != old {
		t.Fatalf("old cell count = %d, want %d", got, old)
	}
	if got := sumCells(h); got != young+old {
		t.Fatalf("sum over all cells = %d, want %d", got, young+old)
	}
}

// --- BB-6: cost on the loop ------------------------------------------------------

// TestR29aSnapshotCostDoesNotGrowInR is BB-6. The number to beat is the PE's measured
// 124 ms / 114 MiB at R = 500,000 for the row export (RULING-R2.9a-bbootstrap-export-
// b142a65-2026-09-04 §5.1), on the event loop, per unauthenticated GET.
//
// WHAT IS ASSERTED, and what is deliberately not. The assertion is ALLOCATION, not wall
// time: allocations are deterministic and hardware-independent, while a wall-clock
// ceiling calibrated on a dev box has already reddened this repo's CI once on slower
// hardware (the R0.4b C3 ValidatePub budget). So the gate is "one allocation, of a fixed
// size, whatever R is" — which is the property that made the row export unsafe — and the
// elapsed time is MEASURED and logged rather than turned into a flaky ceiling.
func TestR29aSnapshotCostDoesNotGrowInR(t *testing.T) {
	measure := func(R int) (allocs float64, bytesPerRun uint64) {
		clk := &bbClock{now: 1}
		l := New(50_000, 0)
		l.SetObservabilityClock(clk, clk.monotonic)
		srv := id(1)
		for i := 0; i < R; i++ {
			fetched(l, srv, reqID(i), int64(1+i))
		}
		clk.now = ports.Time(1 + bbDay)
		var sink BBootstrapHistogram
		allocs = testing.AllocsPerRun(3, func() { sink = l.bBootstrapSnapshot() })
		if sink.Requesters != R {
			t.Fatalf("fixture: requesters = %d, want %d", sink.Requesters, R)
		}
		// One fixed allocation: the cell array. 8 × 164 × 8 = 10,496 bytes.
		return allocs, uint64(BBootstrapAgeBuckets * BBootstrapByteBins * 8)
	}

	small, cellBytes := measure(1_000)
	large, _ := measure(100_000)
	if small != large {
		t.Fatalf("allocations per snapshot grew with R: %.0f at R=1,000 vs %.0f at R=100,000 — the payload and the work must both be constant in R", small, large)
	}
	if small != 1 {
		t.Fatalf("allocations per snapshot = %.0f, want exactly 1 (the fixed cell array) — a per-requester allocation is what made the row export cost 114 MiB at R=500,000", small)
	}
	if cellBytes > 16<<10 {
		t.Fatalf("the fixed cell array is %d bytes, above the 16 KiB ceiling this instrument is allowed on the floor box", cellBytes)
	}
	t.Logf("BB-6: %.0f alloc/snapshot at R=1,000 and R=100,000; fixed cell array %d bytes (the row export measured 114 MiB at R=500,000)", small, cellBytes)
}

// --- BB-9: the censoring invariant, and G-BB-4 ------------------------------------

// TestR29aOccupiedAgeBucketsNeverExceedUptime is BB-9 / G-BB-4: the highest OCCUPIED age
// bucket's edge must not exceed the censoring bound.
//
// THE GATE'S TEETH COME FROM THE BOUND BEING INDEPENDENT of the ages. Against the wall
// clock this assertion was decorative — uptime and every age are two differences of the
// same reading, so a step moved both and cancelled, and the only way to violate it was to
// write the private stamp field by hand. Against the MONOTONE bound it fires on the
// production path: arm 2 below steps nothing but the wall clock, through the real API,
// and the assertion goes true. The foreign-tick arm is kept because it is a different
// hazard (a stamp in the wrong UNIT, not a clock that moved) and it still has teeth.
func TestR29aOccupiedAgeBucketsNeverExceedUptime(t *testing.T) {
	// The ledger is injected at a wall-clock-magnitude instant, which is what a real
	// daemon does, and runs for three hours.
	const boot = 10 * bbDay
	clk := &bbClock{now: ports.Time(boot)}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	fetched(l, id(1), reqID(0), 1<<20)
	clk.now = ports.Time(boot + 3*bbHour)

	h := l.bBootstrapSnapshot()
	if h.UptimeNanos != 3*bbHour {
		t.Fatalf("uptimeNanos = %d, want %d", h.UptimeNanos, 3*bbHour)
	}
	if h.MaxOccupiedAgeEdgeNanos > h.UptimeNanos {
		t.Fatalf("largest occupied age edge %d > uptime %d", h.MaxOccupiedAgeEdgeNanos, h.UptimeNanos)
	}
	if h.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime set on an honest run")
	}

	// TEETH 1, ON THE PRODUCTION PATH: an 8-day forward NTP step. Nothing here reaches
	// into the ledger — the wall clock jumps and that is all. The 3-hour-old identity is
	// now reported as 8 days old and lands in the top bucket, while the process has
	// genuinely been alive for 3 hours, so an occupied bucket exceeds the bound.
	clk.step(8 * bbDay)
	stepped := l.bBootstrapSnapshot()
	if !stepped.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime NOT set after an 8-day forward wall-clock step: max occupied edge %d, wall uptime %d, monotone uptime %d — the censoring assertion is comparing the wall clock against itself", stepped.MaxOccupiedAgeEdgeNanos, stepped.UptimeNanos, stepped.MonotonicUptimeNanos)
	}
	if stepped.MonotonicUptimeNanos != 3*bbHour {
		t.Fatalf("monotonicUptimeNanos = %d after a wall-clock step, want %d — the second source must not move with the wall clock or it is not a second source", stepped.MonotonicUptimeNanos, 3*bbHour)
	}
	if stepped.CensoringBoundNanos() != 3*bbHour {
		t.Fatalf("censoring bound = %d, want %d (the monotone uptime)", stepped.CensoringBoundNanos(), 3*bbHour)
	}
	clk.step(-8 * bbDay) // put the clock back; the arms below are about a foreign stamp

	// TEETH 2: plant a stamp from a FOREIGN tick source in a different unit — the real
	// hazard, since RecordBondChallenge also writes firstSeenTick, from the bond
	// auditor's own clock. A tick of 1 against a wall-clock ledger makes the identity
	// look ten days old on a three-hour-old process. The assertion must fire.
	l.accounts[reqID(0)].firstSeenTick = 1
	h = l.bBootstrapSnapshot()
	if h.ClockSuspect {
		t.Fatalf("clockSuspect set with the wall clock restored — a foreign stamp is not a clock step and the two must stay distinguishable (skew %d)", h.ClockSkewNanos)
	}
	if !h.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime NOT set after a stamp older than the ledger start: max occupied edge %d, uptime %d — the censoring assertion is vacuous", h.MaxOccupiedAgeEdgeNanos, h.UptimeNanos)
	}
	if bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, h, bbHour); len(bad) == 0 {
		t.Fatalf("BBootstrapRunPrecondition accepted a snapshot whose ages exceed uptime")
	}
}

// --- BB-10: the restart is visible ------------------------------------------------

// TestR29aRestartIsVisibleNotSilent is BB-10. Accounts are in-memory and nothing evicts
// them, so a restart destroys the whole census — permanently, not pending FP-2. That
// makes uptime the hard right-censoring bound on the age axis, and this pins that the
// reset is OBSERVABLE: a fresh ledger reports zero requesters, zero uptime and an empty
// grid, so a reader can never mistake a post-restart snapshot for a quiet network.
func TestR29aRestartIsVisibleNotSilent(t *testing.T) {
	clk := &bbClock{now: 1}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	for i := 0; i < 100; i++ {
		fetched(l, id(1), reqID(i), 1<<20)
	}
	clk.now = ports.Time(1 + 2*bbDay)
	before := l.bBootstrapSnapshot()
	if before.Requesters != 100 || before.UptimeNanos != 2*bbDay {
		t.Fatalf("fixture: requesters = %d uptime = %d", before.Requesters, before.UptimeNanos)
	}

	// "Restart": the process is gone, so the ledger is gone. Only the wall clock
	// carries over.
	restarted := New(50_000, 0)
	restarted.SetObservabilityClock(clk, clk.monotonic)
	after := restarted.bBootstrapSnapshot()
	if after.Requesters != 0 || after.Aged != 0 || sumCells(after) != 0 {
		t.Fatalf("after restart: requesters = %d aged = %d cells = %d, want 0/0/0", after.Requesters, after.Aged, sumCells(after))
	}
	if after.UptimeNanos != 0 {
		t.Fatalf("after restart: uptimeNanos = %d, want 0 — the censoring bound must reset with the census", after.UptimeNanos)
	}
	if bad := BBootstrapRunPrecondition(before, after, bbHour); len(bad) == 0 {
		t.Fatalf("BBootstrapRunPrecondition accepted a post-restart snapshot as a valid one-hour window")
	}
}

// --- BB-11: Invariant A, the write-on-read defect stays closed --------------------

// TestR29aBBootstrapSnapshotWritesNothing is BB-11. The refuted shape's sibling defect
// was a reader that MINTED: FetchedBytes goes through acct() → Register and therefore
// creates an account and hands out a 500,000 grant for any id it is passed. The snapshot
// must iterate l.order and index l.accounts directly. This deep-compares the entire
// account map, the order slice and the balances across a snapshot.
func TestR29aBBootstrapSnapshotWritesNothing(t *testing.T) {
	clk := &bbClock{now: 500}
	l := New(50_000, 500_000)
	l.SetObservabilityClock(clk, clk.monotonic)
	for i := 0; i < 50; i++ {
		fetched(l, id(1), reqID(i), int64(1<<uint(i%20)))
	}
	clk.now = ports.Time(500 + bbDay)

	snapshotOf := func() (map[ports.NodeID]account, []ports.NodeID) {
		accts := make(map[ports.NodeID]account, len(l.accounts))
		for k, v := range l.accounts {
			accts[k] = *v
		}
		return accts, append([]ports.NodeID(nil), l.order...)
	}
	wantAccts, wantOrder := snapshotOf()

	_ = l.bBootstrapSnapshot()
	_ = l.bBootstrapSnapshot() // twice: a lazy one-shot write would show on the first

	gotAccts, gotOrder := snapshotOf()
	if !reflect.DeepEqual(wantAccts, gotAccts) {
		t.Fatalf("BBootstrapSnapshot mutated the account map: %d accounts before, %d after (a reader that mints is not a reader)", len(wantAccts), len(gotAccts))
	}
	if !reflect.DeepEqual(wantOrder, gotOrder) {
		t.Fatalf("BBootstrapSnapshot mutated l.order: %d entries before, %d after", len(wantOrder), len(gotOrder))
	}
}

// --- BB-13: clock-step robustness -------------------------------------------------

// TestR29aBackwardClockStepClampsAndSaysSo is BB-13 / G-BB-4. adapters/walltime returns
// time.Now().UnixNano(), which DISCARDS Go's monotonic reading, so an NTP step is a real
// hazard and a boot-time step lands at the start of the observation window. A backwards
// step must not underflow, must not wrap a bucket, and must not silently reshape ages —
// it is absorbed by clamping at zero AND reported.
func TestR29aBackwardClockStepClampsAndSaysSo(t *testing.T) {
	// ARM 1 — the step lands AFTER the ledger start but BEFORE the stamp, so uptime
	// stays positive and only the AGE goes negative. This is the arm that isolates the
	// age clamp: without it the age is silently reshaped into bucket 0 with nothing
	// said, which is exactly what "no silent age reshaping" forbids.
	{
		const boot = 10 * bbDay
		clk := &bbClock{now: ports.Time(boot)}
		l := New(50_000, 0)
		l.SetObservabilityClock(clk, clk.monotonic)
		clk.now = ports.Time(boot + 2*bbHour)
		fetched(l, id(1), reqID(0), 1<<20) // stamped two hours into the run
		clk.step(-bbHour)                  // …then the WALL clock steps back one hour

		h := l.bBootstrapSnapshot()
		if h.UptimeNanos != bbHour {
			t.Fatalf("arm 1: uptimeNanos = %d, want %d — uptime is still positive here, so only the AGE can report the step", h.UptimeNanos, bbHour)
		}
		// THE SPLIT (RE-CERT 2026-09-05 §5.1), pinned by its discriminator. Only the
		// AGE went negative, so this is the CENSUS arm: AgeClampedToZero fires and the
		// instrument arm ClockStepBack — which compares the clock against the ledger's
		// own start and reads no account — must stay DOWN. If they moved together the
		// two arms would still be fused and a census bit would ride out below the floor.
		if !h.AgeClampedToZero {
			t.Fatalf("arm 1: ageClampedToZero not reported when only the AGE went negative — the age was silently reshaped into bucket 0")
		}
		if h.ClockStepBack {
			t.Fatalf("arm 1: the INSTRUMENT arm clockStepBack fired on a per-account clamp. The two arms are fused again, so a census-derived bit survives the minimum-requester floor (G-BB-11′)")
		}
		if h.Aged != 1 || h.Cells[0][bbootstrapByteBin(1<<20)] != 1 {
			t.Fatalf("arm 1: the stepped identity did not clamp into age bucket 0: aged = %d, row 0 = %v", h.Aged, nonZero(h.Cells[0]))
		}
	}

	// ARM 2 — the step lands before the ledger start itself, so uptime would go
	// negative too. It is clamped at zero, never underflowed.
	clk := &bbClock{now: ports.Time(10 * bbDay)}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	fetched(l, id(1), reqID(0), 1<<20)
	clk.step(-bbDay)

	h := l.bBootstrapSnapshot()
	if !h.ClockStepBack {
		t.Fatalf("clockStepBack not reported after a backwards step past the LEDGER START — a silent reshaping is exactly what BB-13 forbids")
	}
	if !h.AgeClampedToZero {
		t.Fatalf("ageClampedToZero not reported after a backwards step that also passed the stamp — the census arm must still fire when both do")
	}
	if h.Aged != 1 {
		t.Fatalf("aged = %d, want 1 — a stepped clock must not lose the identity", h.Aged)
	}
	if h.UptimeNanos != 0 {
		t.Fatalf("uptimeNanos = %d, want 0 (clamped) — a negative uptime is an underflow waiting to happen", h.UptimeNanos)
	}
	if got := h.Cells[0][bbootstrapByteBin(1<<20)]; got != 1 {
		t.Fatalf("the stepped identity did not clamp into age bucket 0: cells row 0 = %v", nonZero(h.Cells[0]))
	}
	// Every bucket index is in range and no count is negative — no wrap anywhere.
	for a := range h.Cells {
		for b, c := range h.Cells[a] {
			if c < 0 {
				t.Fatalf("negative count at cell (%d,%d) = %d", a, b, c)
			}
		}
	}
	if bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, h, bbHour); len(bad) == 0 {
		t.Fatalf("BBootstrapRunPrecondition accepted a snapshot taken across a backwards clock step")
	}
}

// TestR29aWallClockStepIsDetectedNotAbsorbed is BB-13's substance and the regression for
// the defect the blind review measured in the reviewed build: BOTH published uptimes came
// off the same wall clock, so a step moved every age AND the bound they were checked
// against by the same amount and cancelled. Measured then: an 8-day forward step put a
// 30-second-old identity in the ">7 days" bucket on a 60-second-old process, raised no
// flag, and let the run precondition accept a 7-day window. A 2 h 50 m backward step
// reshaped every bucket and raised no flag either, because ClockStepBack only fires when
// a subtraction crosses zero and 2 h 50 m is smaller than the ages it moved.
//
// The three arms below are the three states the artifact must be able to tell apart: a
// clean run, a forward step and a backward step. They are driven ONLY through the wall
// clock — nothing writes a private field, and nothing reaches past the public API.
func TestR29aWallClockStepIsDetectedNotAbsorbed(t *testing.T) {
	// The PE's fixture, exactly: a 30-second-old identity on a 60-second-old process.
	const boot = 10 * bbDay
	const week = 7 * bbDay
	build := func() (*Ledger, *bbClock) {
		clk := &bbClock{now: ports.Time(boot)}
		l := New(50_000, 0)
		l.SetObservabilityClock(clk, clk.monotonic)
		// TWO identities, in two different age buckets. One is the PE's 30-second-old
		// fixture; the other arrives at the injection instant and is 60 s old at the
		// snapshot. Two occupied buckets are what keep the CLEAN arm out of the
		// degeneracy refusal, so the arm tests the clock and nothing else.
		fetched(l, id(1), reqID(1), 1<<10)  // age 60 s at the snapshot → bucket 2
		clk.now = ports.Time(boot + 30*1e9) // 30 s in, the second identity arrives
		fetched(l, id(1), reqID(0), 1<<20)  // age 30 s at the snapshot → bucket 1
		clk.now = ports.Time(boot + 60*1e9) // …and the snapshot is taken at 60 s
		return l, clk
	}

	// ARM 1 — CLEAN. The two sources agree, every flag is down, and a W the process has
	// actually covered is accepted.
	clean, _ := build()
	h := clean.bBootstrapSnapshot()
	if h.MonotonicSource != "injected" {
		t.Fatalf("monotonicSource = %q, want \"injected\" — with no second source the cross-check is a self-comparison", h.MonotonicSource)
	}
	if h.ClockSkewNanos != 0 || h.ClockSuspect || h.ClockStepBack || h.AgeExceedsUptime {
		t.Fatalf("clean run raised a flag: skew %d, suspect %v, stepBack %v, ageExceedsUptime %v", h.ClockSkewNanos, h.ClockSuspect, h.ClockStepBack, h.AgeExceedsUptime)
	}
	if h.UptimeNanos != 60*1e9 || h.MonotonicUptimeNanos != 60*1e9 {
		t.Fatalf("clean uptimes = wall %d / monotone %d, want both 60 s", h.UptimeNanos, h.MonotonicUptimeNanos)
	}
	if bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, h, 30*1e9); len(bad) != 0 {
		t.Fatalf("a clean 60-second run was refused for a 30-second W: %v", bad)
	}

	// ARM 2 — an 8-DAY FORWARD STEP. The 30-second-old identity is reported as 8 days
	// old and lands in the top bucket; real elapsed time is still 60 seconds.
	fwdL, fwdClk := build()
	fwdClk.step(8 * bbDay)
	fwd := fwdL.bBootstrapSnapshot()
	if fwd.Cells[BBootstrapAgeBuckets-1][bbootstrapByteBin(1<<20)] != 1 {
		t.Fatalf("the forward step did not reshape the age axis, so this arm is not testing what it claims: row 7 = %v", nonZero(fwd.Cells[BBootstrapAgeBuckets-1]))
	}
	if !fwd.ClockSuspect {
		t.Fatalf("clockSuspect NOT set after an 8-day forward step: wall uptime %d, monotone uptime %d, skew %d — the step was absorbed silently, which is exactly what BB-13 forbids", fwd.UptimeNanos, fwd.MonotonicUptimeNanos, fwd.ClockSkewNanos)
	}
	if fwd.ClockSkewNanos != 8*bbDay {
		t.Fatalf("clockSkewNanos = %d after an 8-day forward step, want %d — the artifact must carry the SIZE of the step, not just a bit", fwd.ClockSkewNanos, 8*bbDay)
	}
	if !fwd.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime NOT set after an 8-day forward step: max occupied edge %d against a %d-ns monotone bound", fwd.MaxOccupiedAgeEdgeNanos, fwd.MonotonicUptimeNanos)
	}
	if fwd.ClockStepBack {
		t.Fatalf("clockStepBack set on a FORWARD step — the two directions must stay distinguishable")
	}
	// And the run is refused for a 7-day window, on the ground that matters: the process
	// has been alive for 60 seconds however many days the wall clock claims.
	bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, fwd, week)
	if !bbReasonContains(bad, "uptime below W") {
		t.Fatalf("BBootstrapRunPrecondition did not refuse a 7-day W on a 60-second-old process after an 8-day forward step; reasons: %v", bad)
	}
	if !bbReasonContains(bad, "diverged from the monotone source") {
		t.Fatalf("BBootstrapRunPrecondition did not report the divergence; reasons: %v", bad)
	}

	// ARM 3 — a 2 h 50 m BACKWARD STEP, smaller than the ages it moves, so no subtraction
	// crosses zero and ClockStepBack stays down. This is the arm the reviewed build had
	// no detector for at all.
	backL, backClk := build()
	backClk.now = ports.Time(boot + 3*bbHour) // let the identity reach three hours old
	before := backL.bBootstrapSnapshot()
	backClk.step(-(2*bbHour + 50*bbMinute))
	back := backL.bBootstrapSnapshot()
	if before.MaxOccupiedAgeEdgeNanos == back.MaxOccupiedAgeEdgeNanos {
		t.Fatalf("the backward step did not reshape the age axis, so this arm is not testing what it claims: occupied edge %d before and after", back.MaxOccupiedAgeEdgeNanos)
	}
	if !back.ClockSuspect {
		t.Fatalf("clockSuspect NOT set after a 2 h 50 m backward step: wall uptime %d, monotone uptime %d, skew %d — a step smaller than the ages it moves is exactly the silent reshaping BB-13 forbids", back.UptimeNanos, back.MonotonicUptimeNanos, back.ClockSkewNanos)
	}
	if back.ClockSkewNanos != -(2*bbHour + 50*bbMinute) {
		t.Fatalf("clockSkewNanos = %d after a 2 h 50 m backward step, want %d", back.ClockSkewNanos, -(2*bbHour + 50*bbMinute))
	}
	if back.ClockStepBack {
		t.Fatalf("clockStepBack set by a backward step that crossed no zero — it would then be reporting the wrong thing, and the two flags must stay distinguishable")
	}
	if back.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime set by a BACKWARD step — a backward step deflates ages, so the censoring bound is not the arm that catches it; the two failures must stay distinguishable")
	}
	if bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, back, bbHour); !bbReasonContains(bad, "diverged from the monotone source") {
		t.Fatalf("BBootstrapRunPrecondition did not refuse a run taken across a 2 h 50 m backward step; reasons: %v", bad)
	}

	// The three states are pairwise DISTINGUISHABLE on the wire, which is the property a
	// reader of the artifact actually needs: "something moved" is not a finding.
	sig := func(x BBootstrapHistogram) [4]int64 {
		b := func(v bool) int64 {
			if v {
				return 1
			}
			return 0
		}
		return [4]int64{b(x.ClockSuspect), b(x.AgeExceedsUptime), b(x.ClockStepBack), x.ClockSkewNanos}
	}
	if sig(h) == sig(fwd) || sig(h) == sig(back) || sig(fwd) == sig(back) {
		t.Fatalf("clean %v, forward %v and backward %v are not pairwise distinguishable", sig(h), sig(fwd), sig(back))
	}
}

// bbReasonContains reports whether any refusal reason mentions sub. The precondition
// returns REASONS, not a bool, so a gate can assert the run was refused for the right
// ground rather than for any ground at all.
func bbReasonContains(reasons []string, sub string) bool {
	for _, r := range reasons {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

// --- BB-14: the handoff precondition, one command --------------------------------

// TestR29aRunPreconditionAcceptsOnlyAValidRun is BB-14, made executable BEFORE the
// deployment window instead of discovered after it. W is a REQUIRED ARGUMENT and this
// test passes several: no value of W is pinned anywhere, because G-BB-1 makes pinning W
// the owner's call and a pure fetcher has no income on the serving ledger, so "before it
// has income" does not define a window at all (R-FETCHER-INCOME).
func TestR29aRunPreconditionAcceptsOnlyAValidRun(t *testing.T) {
	build := func(uptime int64, arrivals []int64) BBootstrapHistogram {
		clk := &bbClock{now: 1}
		l := New(50_000, 0)
		l.SetObservabilityClock(clk, clk.monotonic)
		for i, at := range arrivals {
			clk.now = ports.Time(1 + at)
			fetched(l, id(1), reqID(i), int64(4096*(i+1)))
		}
		clk.now = ports.Time(1 + uptime)
		return l.bBootstrapSnapshot()
	}

	prev := build(bbHour, []int64{0, bbMinute})
	cur := build(6*bbHour, []int64{0, bbMinute, 3 * bbHour})
	if bad := BBootstrapRunPrecondition(prev, cur, bbHour); len(bad) != 0 {
		t.Fatalf("a valid one-hour run was refused: %v", bad)
	}

	// W longer than uptime: the window is unreadable, permanently (the age axis is
	// right-censored at uptime).
	if bad := BBootstrapRunPrecondition(prev, cur, 30*bbDay); len(bad) == 0 {
		t.Fatalf("a W of 30 days was accepted against 6 hours of uptime")
	}
	// A frozen clock is as fatal as an absent one.
	if bad := BBootstrapRunPrecondition(cur, cur, bbHour); len(bad) == 0 {
		t.Fatalf("a FROZEN clock was accepted: uptime did not advance between the snapshots")
	}
	// An empty census.
	if bad := BBootstrapRunPrecondition(prev, build(6*bbHour, nil), bbHour); len(bad) == 0 {
		t.Fatalf("an empty census was accepted")
	}
	// Everything in age bucket 0 — a degenerate axis that would fit nothing.
	if bad := BBootstrapRunPrecondition(prev, build(0, []int64{0, 0, 0}), 0); len(bad) == 0 {
		t.Fatalf("a degenerate age axis (every identity in bucket 0) was accepted")
	}

	// DEGENERACY, THE LIVE CASE. Bucket 0 is an age of EXACTLY 0 ns, which a wall clock
	// essentially never produces, so a bucket-0-only rule cannot fire on the machine that
	// will run this and that arm was decorative. The live degeneracy is "every counted
	// identity in ONE bucket": three identities that arrive together and are all three
	// hours old at the snapshot pile into bucket 4, bucket 0 is EMPTY, and the age axis
	// carries no variation for a conditional quantile to read.
	oneBucket := build(3*bbHour, []int64{0, 0, 0})
	if sumCells(oneBucket) != 3 || len(nonZero(oneBucket.Cells[0])) != 0 {
		t.Fatalf("the one-bucket fixture is not what this arm claims: aged %d, bucket 0 row = %v", oneBucket.Aged, nonZero(oneBucket.Cells[0]))
	}
	if got := BBootstrapRunPrecondition(prev, oneBucket, bbHour); !bbReasonContains(got, "ONE age bucket") {
		t.Fatalf("a degenerate age axis with every identity in bucket 4 (NOT bucket 0) was accepted: %v", got)
	}

	// NO MONOTONE SOURCE. Without a second, independent clock the censoring assertion and
	// the skew check are comparisons of the wall clock against itself, so every clock arm
	// above is decorative and the configuration is refused outright.
	noMono := func() BBootstrapHistogram {
		clk := &bbClock{now: 1}
		l := New(50_000, 0)
		l.SetObservabilityClock(clk, nil)
		fetched(l, id(1), reqID(0), 4096)
		clk.now = ports.Time(1 + bbMinute)
		fetched(l, id(1), reqID(1), 8192)
		clk.now = ports.Time(1 + 6*bbHour)
		return l.bBootstrapSnapshot()
	}()
	if noMono.MonotonicSource != "none" || noMono.CensoringBoundNanos() != noMono.UptimeNanos {
		t.Fatalf("the no-monotone fixture is not what this arm claims: source %q, bound %d, wall uptime %d", noMono.MonotonicSource, noMono.CensoringBoundNanos(), noMono.UptimeNanos)
	}
	if got := BBootstrapRunPrecondition(prev, noMono, bbHour); !bbReasonContains(got, "no monotone source") {
		t.Fatalf("a run with no monotone source was accepted: %v", got)
	}
}

// --- the byte axis ----------------------------------------------------------------

// TestR29aByteBinMatchesTheClosedForm pins the integer bin function against the float
// reference it implements, k = floor(4·log2(b)). The instrument's bins are computed with
// bits.Len64 and three integer comparisons — no math package, because core stays
// deterministic — so the float reference lives here in the test, where it is allowed.
func TestR29aByteBinMatchesTheClosedForm(t *testing.T) {
	if got := bbootstrapByteBin(0); got != -1 {
		t.Fatalf("bin(0) = %d, want -1 — an account with no fetched bytes is not a requester", got)
	}
	if got := bbootstrapByteBin(-5); got != -1 {
		t.Fatalf("bin(-5) = %d, want -1", got)
	}
	// EXHAUSTIVE over the first 100,000 byte counts. 2^(k/4) is irrational unless k is a
	// multiple of 4, so floor(4·log2(b)) is never ambiguous at a float boundary, and an
	// exhaustive sweep beats trying to reconstruct the edges in floating point (which is
	// wrong at the bottom anyway: quarter-octave edges below 4 bytes are not resolvable
	// by an integer, so ceil(2^(k/4)) is not a faithful edge there).
	for b := int64(1); b <= 100_000; b++ {
		want := int(math.Floor(4 * math.Log2(float64(b))))
		if got := bbootstrapByteBin(b); got != want {
			t.Fatalf("bin(%d) = %d, want floor(4*log2(%d)) = %d", b, got, b, want)
		}
	}
	// And the exact, integer-only boundaries: a power of two opens bin 4e, and one byte
	// below it closes bin 4e−1.
	for e := 4; e <= 40; e++ {
		b := int64(1) << uint(e)
		if got := bbootstrapByteBin(b); got != 4*e {
			t.Fatalf("bin(2^%d) = %d, want %d — a power of two must OPEN its octave's first bin", e, got, 4*e)
		}
		if got := bbootstrapByteBin(b - 1); got != 4*e-1 {
			t.Fatalf("bin(2^%d − 1) = %d, want %d — one byte below must CLOSE the previous bin", e, got, 4*e-1)
		}
	}
	// The top bin is OPEN, not a silent drop: a fetch far above the nominal top lands
	// in bin 163 and is counted.
	if got := bbootstrapByteBin(int64(1) << 62); got != BBootstrapByteBins-1 {
		t.Fatalf("bin(2^62) = %d, want %d — the top bin is open-topped and must SATURATE, never drop a requester", got, BBootstrapByteBins-1)
	}
	// And the counter layout is exactly the certified size.
	if n := BBootstrapAgeBuckets * BBootstrapByteBins; n != 1312 {
		t.Fatalf("counter count = %d, want 1,312 (8 age buckets × 164 quarter-log2 byte bins)", n)
	}
}

// TestR29aUnstampedRequestersAreCountedNotAged pins the one honest gap: an account that
// registered BEFORE the clock was injected carries no first-touch stamp. It is reported
// as `unstamped` rather than dumped into age bucket 0, which would make an old identity
// look brand new — the silent age reshaping G-BB-4 forbids.
func TestR29aUnstampedRequestersAreCountedNotAged(t *testing.T) {
	l := New(50_000, 0)
	srv := id(1)
	fetched(l, srv, reqID(0), 1<<20) // registered with NO clock: unstamped
	clk := &bbClock{now: ports.Time(bbDay)}
	l.SetObservabilityClock(clk, clk.monotonic)
	fetched(l, srv, reqID(1), 1<<20) // registered after injection: stamped
	clk.now = ports.Time(bbDay + bbHour)

	h := l.bBootstrapSnapshot()
	if h.Requesters != 2 {
		t.Fatalf("requesters = %d, want 2", h.Requesters)
	}
	if h.Unstamped != 1 {
		t.Fatalf("unstamped = %d, want 1 — an account older than the clock must not be aged", h.Unstamped)
	}
	if h.Aged != 1 || sumCells(h) != 1 {
		t.Fatalf("aged = %d, cells sum = %d, want 1/1", h.Aged, sumCells(h))
	}
	if h.Aged+h.Unstamped != h.Requesters {
		t.Fatalf("aged (%d) + unstamped (%d) != requesters (%d) — the census must balance", h.Aged, h.Unstamped, h.Requesters)
	}
	if bad := BBootstrapRunPrecondition(BBootstrapHistogram{}, h, bbHour); len(bad) == 0 {
		t.Fatalf("BBootstrapRunPrecondition accepted a snapshot with unstamped requesters")
	}
}

// TestR29aFirstTouchIsStampedOnceAtRegister pins the stamp's placement. The estimand's
// window opens at FIRST TOUCH on this ledger (the instant the faucet grant is minted),
// so the stamp belongs in Register — the one place an account is constructed — and must
// not move on later traffic.
func TestR29aFirstTouchIsStampedOnceAtRegister(t *testing.T) {
	clk := &bbClock{now: 1_000}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)
	r := reqID(0)
	fetched(l, id(1), r, 1<<10)
	first := l.accounts[r].firstSeenTick
	if first != 1_001 {
		t.Fatalf("firstSeenTick = %d, want 1,001 (clock 1,000, +1 so tick 0 stays UNSET)", first)
	}
	clk.now = ports.Time(5 * bbDay)
	fetched(l, id(1), r, 1<<10)
	fetched(l, id(2), r, 1<<10)
	if got := l.accounts[r].firstSeenTick; got != first {
		t.Fatalf("firstSeenTick moved on later traffic: %d → %d — the window opens at FIRST touch, not the latest", first, got)
	}
	// And with no clock the stamp stays unset, so the shipped behaviour is unchanged.
	bare := New(50_000, 0)
	fetched(bare, id(1), r, 1<<10)
	if got := bare.accounts[r].firstSeenTick; got != 0 {
		t.Fatalf("firstSeenTick = %d with no clock injected, want 0 — nil must stay the safe, behaviour-identical state", got)
	}
}
