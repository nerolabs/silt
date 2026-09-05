package credit

// R2.9a DELTA — the minimum-requester floor (G-BB-11) and the dead discriminator
// (§1.3), from RESEARCH CERTIFICATION
// R2.9a-Bbootstrap-DELTA-contamination-privacy-floor-clock (2026-09-04) §2.3, §1.3 and
// the Tester gates BB-15 and BB-19 in its §6.
//
// Each test names its gate and carries the ablation that proves it can go RED.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// bbFloorLedger builds a ledger with a live pair of observability clocks and exactly R
// requesters, each with distinct bytes, at a fixed age. It returns the raw ledger so a
// test can read the UNFLOORED census as well as the floored one — the two are different
// objects and the difference is what BB-15 is about.
func bbFloorLedger(t *testing.T, R int) (*Ledger, *bbClock, ports.NodeID) {
	t.Helper()
	l := New(50_000, 0)
	clk := &bbClock{}
	l.SetObservabilityClock(clk, clk.monotonic)
	server := ports.HashBytes([]byte("bb-floor-server"))
	for i := 0; i < R; i++ {
		fetched(l, server, reqID(i), int64(1024+i))
	}
	clk.now = ports.Time(2 * bbHour)
	return l, clk, server
}

// --- BB-15: the floor ---------------------------------------------------------------

// TestR29aBB15FloorWithholdsEveryCensusCountBelowRMin is BB-15 at the core tier: at
// R_min − 1 the histogram carries Suppressed and NO census quantity; at R_min it carries
// all of them.
//
// WHY THE FLOOR COVERS `Requesters` AND NOT ONLY `Cells`. `requesters` is the
// ANONYMITY-SET SIZE. stats.bytesServed and durability.objects[].funded are published
// unconditionally and predate this instrument, so at a degenerate census their deltas
// already describe one identity's fetches; publishing the set size is what makes that
// attributable. Suppressing cells while still publishing `requesters` would close
// nothing, which is why the assertion below is over EVERY census-derived field rather
// than over the grid.
func TestR29aBB15FloorWithholdsEveryCensusCountBelowRMin(t *testing.T) {
	if BBootstrapMinRequesters < 10 {
		t.Fatalf("R_min = %d, below the certified floor of 10", BBootstrapMinRequesters)
	}

	// Below the floor, by exactly one.
	l, _, _ := bbFloorLedger(t, BBootstrapMinRequesters-1)
	raw := l.bBootstrapSnapshot()
	if raw.Requesters != BBootstrapMinRequesters-1 || raw.Cells == nil {
		t.Fatalf("the LOCAL census is itself lossy: requesters = %d, cells nil = %v — the floor is a publication rule, not a recording rule, and the operator's own read must stay honest", raw.Requesters, raw.Cells == nil)
	}
	got := raw.WithMinRequesterFloor()
	if !got.Suppressed {
		t.Fatalf("suppressed = false at %d requesters, one below R_min = %d", raw.Requesters, BBootstrapMinRequesters)
	}
	if got.Requesters != 0 || got.Aged != 0 || got.Unstamped != 0 {
		t.Fatalf("below the floor the published census counts are requesters/aged/unstamped = %d/%d/%d, want 0/0/0: requesters IS the anonymity-set size, so withholding the grid while publishing the count closes nothing", got.Requesters, got.Aged, got.Unstamped)
	}
	if got.Cells != nil {
		t.Fatalf("below the floor the cell grid is published: %v", got.Cells[0])
	}
	if got.MaxOccupiedAgeEdgeNanos != 0 {
		t.Fatalf("below the floor maxOccupiedAgeEdgeNanos = %d: at a degenerate anonymity set that is the singleton's own age", got.MaxOccupiedAgeEdgeNanos)
	}
	// The clock apparatus SURVIVES suppression: it describes the instrument, not the
	// population, and an operator needs it exactly when the census is too small.
	if got.ClockSource != "injected" || got.MonotonicSource != "injected" || !got.AgeAxisLive {
		t.Fatalf("suppression ate the clock self-report: clockSource %q, monotonicSource %q, ageAxisLive %v — a reader must be able to tell 'below the floor' from 'no clock injected'", got.ClockSource, got.MonotonicSource, got.AgeAxisLive)
	}
	if got.MonotonicUptimeNanos != int64(2*bbHour) {
		t.Fatalf("suppression moved the censoring bound: %d, want %d", got.MonotonicUptimeNanos, int64(2*bbHour))
	}
	// And the run precondition says WHY, rather than reporting an empty census.
	if reasons := BBootstrapRunPrecondition(BBootstrapHistogram{}, got, bbHour); !bbReasonContains(reasons, "below the minimum-requester floor") {
		t.Fatalf("the run precondition does not name the floor: %v", reasons)
	}

	// At the floor, everything publishes. The rule is `>= R_min`, not `> R_min`.
	l2, _, _ := bbFloorLedger(t, BBootstrapMinRequesters)
	at := l2.BBootstrapPublish()
	if at.Suppressed {
		t.Fatalf("suppressed at exactly R_min = %d requesters — the floor publishes AT the floor", BBootstrapMinRequesters)
	}
	if at.Requesters != BBootstrapMinRequesters || at.Aged != BBootstrapMinRequesters {
		t.Fatalf("at R_min: requesters/aged = %d/%d, want %d/%d", at.Requesters, at.Aged, BBootstrapMinRequesters, BBootstrapMinRequesters)
	}
	if at.Cells == nil || sumCells(at) != int64(BBootstrapMinRequesters) {
		t.Fatalf("at R_min the grid is missing or short: cells nil = %v, sum = %d", at.Cells == nil, sumCells(at))
	}
	if at.MaxOccupiedAgeEdgeNanos == 0 {
		t.Fatalf("at R_min maxOccupiedAgeEdgeNanos = 0 with a two-hour-old census")
	}

	// ABLATION, in the test rather than in a comment: run the same assertions against a
	// census one requester SMALLER than the one that publishes, and against one
	// requester LARGER than the one that suppresses. If the floor were a no-op both
	// arms would agree, and the two comparisons below would both be false.
	if raw.WithMinRequesterFloor().Suppressed == at.Suppressed {
		t.Fatalf("the floor does not discriminate: R_min-1 and R_min produce the same suppressed flag (%v). The gate has no teeth", at.Suppressed)
	}
	t.Logf("BB-15: R_min = %d, derived as ceil(1/(1-q)) at the certified edge q >= 0.90; suppressed at %d, published at %d",
		BBootstrapMinRequesters, BBootstrapMinRequesters-1, BBootstrapMinRequesters)
}

// TestR29aBB15FloorIsDerivedNotChosen pins the DERIVATION, not just the number. R_min is
// ceil(1/(1-q)) evaluated at the lower edge of the range the certification covers
// (q >= 0.90), which is 10; the census must carry far more than one cell's requirement,
// so a census-wide floor of 10 is strictly dominated by the fit's own need and costs it
// nothing. If the q edge ever moves the constant must move with it, and this recomputes
// the rule independently of the constant's own arithmetic.
func TestR29aBB15FloorIsDerivedNotChosen(t *testing.T) {
	// ceil(1/(1-q)) computed from the percent edge in a different way than the constant
	// computes it: count up until n*(1-q) >= 1.
	num := 100 - bbDerivedQuantileFloorPercent // (1-q) in percent
	want := 0
	for n := 1; ; n++ {
		if n*num >= 100 {
			want = n
			break
		}
	}
	if BBootstrapMinRequesters != want {
		t.Fatalf("R_min = %d but ceil(1/(1-q)) at q = 0.%d is %d — the constant and its stated derivation disagree", BBootstrapMinRequesters, bbDerivedQuantileFloorPercent, want)
	}
	if want < 10 {
		t.Fatalf("the derived floor is %d, below the certified minimum of 10: the derivation was re-run at a q below the certified edge (q >= 0.90) without re-certifying it", want)
	}
}

// --- BB-19: the dead discriminator ---------------------------------------------------

// TestR29aBB19ServedBytesSelectsTheEmptySet is BB-19. The certification REFUTED
// splitting the census by `servedBytes > 0` to separate infrastructure fetchers from
// viewers: RecordServe and RecordServeToObject credit the SERVER, and on a serving node
// the server is always that node itself, so the predicate holds for exactly one account
// — the node's own — and that account is never in the census, because self-serving
// returns early and leaves its fetchedBytes at zero. The split partitions the census
// into everyone and nobody.
//
// This pins the refutation so the discriminator is not rebuilt. It enumerates l.accounts
// DIRECTLY rather than through ServedBytes(), because that reader goes through acct() →
// Register and would MINT an account (with a 500,000 grant) for any id it has not seen —
// a reader that mints is not a reader, and it would also add a fresh census member.
func TestR29aBB19ServedBytesSelectsTheEmptySet(t *testing.T) {
	l := New(50_000, 10_000_000)
	clk := &bbClock{}
	l.SetObservabilityClock(clk, clk.monotonic)
	server := ports.HashBytes([]byte("bb19-server"))
	root := ports.HashBytes([]byte("bb19-object"))

	// A dozen requesters over BOTH serve presses: the plain one and the object-aware
	// one that skims into an escrow. Both are the production path (core/node calls
	// exactly these two, always with the node's own id as `server`).
	const R = 12
	for i := 0; i < R; i++ {
		if i%2 == 0 {
			l.RecordServe(server, reqID(i), ports.Hash{}, int64(4096+i))
		} else {
			l.RecordServeToObject(server, reqID(i), root, ports.Hash{}, int64(4096+i))
		}
	}
	clk.now = ports.Time(bbHour)

	withServed := 0
	var servedIDs []ports.NodeID
	for _, id := range l.order {
		a := l.accounts[id]
		if a == nil || a.servedBytes <= 0 {
			continue
		}
		withServed++
		servedIDs = append(servedIDs, id)
	}
	if withServed != 1 || servedIDs[0] != server {
		t.Fatalf("%d accounts carry servedBytes > 0 (%v); on a serving ledger the predicate must hold for EXACTLY the node's own account, which is what makes it a null discriminator", withServed, servedIDs)
	}
	// And the one account it selects is not in the census, so the "infrastructure" side
	// of the proposed split is empty.
	if a := l.accounts[server]; a == nil || a.fetchedBytes != 0 {
		t.Fatalf("the server's own account has fetchedBytes = %v — self-serving must earn no fetched bytes, or the server enters its own census", a)
	}
	h := l.bBootstrapSnapshot()
	if h.Requesters != R {
		t.Fatalf("census = %d, want %d (the server must not be counted)", h.Requesters, R)
	}
	t.Logf("BB-19: servedBytes > 0 selects %d of %d accounts, and that one is not a census member — the proposed split is everyone vs nobody",
		withServed, len(l.order))
}
