package credit

// D-BB-BUILD-TAG (ratified 2026-09-05) — the ledger-tier gates for "a default silt build
// contains no part of the B_bootstrap instrument".
//
// THIS FILE CARRIES NO BUILD TAG ON PURPOSE. The first gate asserts a property that must
// hold in BOTH builds — untagged because the mechanism is absent, tagged because
// -bbootstrap was not passed and no clock was injected — so running it in both is the
// point, not an accident. The second and third are `!bbootstrap` only, because they
// assert the ABSENCE of methods that exist under the tag; they live in
// r29a_build_tag_absent_test.go. The two bond-path gates (the G-BB-28 inversion and the
// retention-unit pin) are untagged for the same reason as the first.

import (
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestR29aDefaultBuildStampsNoFirstTouchOnRegister is the regression gate for the
// finding that made this decision: before it, cmd/silt/daemon.go injected the
// observability clock UNCONDITIONALLY, so every default-flags silt node recorded
// (identity, cumulative bytes, first-seen wall-clock nanosecond) for every requester,
// and no flag disabled it. R2.9a's genuine addition to the RECORD was the `when`.
//
// A ledger with no observability clock stamps nothing. In a default build that is
// structural — stampFirstFetch is an empty function (bbootstrap_off.go) and there is no
// setter to call. In a tagged build it is the -bbootstrap default: the daemon calls
// bbootstrapInject(ledger, clk, false), which injects nothing.
//
// THE FIXTURE DRIVES RecordServe, AND THAT IS THE STAMPING PATH. The R2.9a stamp moved
// off Register onto recordFetched, the one write path for fetchedBytes (G-BB-24), so
// the field this gate reads is firstFetchTick — the census's own, and since G-BB-28 the
// ONLY first-touch field on an account. This test keeps its name because the property
// it pins is unchanged and D-BB-BUILD-TAG, the CHANGELOG and bbootstrap_off.go all cite
// it.
//
// THE ARRIVAL ORDER AND GAP ARE WHAT THIS DENIES. The red-team probe recovered them from
// two accounts' stamps alone. With no stamp there is no order and no gap.
func TestR29aDefaultBuildStampsNoFirstTouchOnRegister(t *testing.T) {
	l := New(1_000, 500_000)
	server := ports.HashBytes([]byte("server"))
	for i := 0; i < 3; i++ {
		requester := ports.HashBytes([]byte{byte(i), 0x29})
		l.RecordServe(server, requester, ports.Hash{}, 4096)
		a := l.accounts[requester]
		if a == nil {
			t.Fatalf("requester %d: no account after RecordServe", i)
		}
		if a.fetchedBytes != 4096 {
			t.Fatalf("requester %d: fetchedBytes = %d, want 4096 — the fixture must actually create a requester or the assertion below is vacuous", i, a.fetchedBytes)
		}
		if a.firstFetchTick != 0 {
			t.Fatalf("requester %d: firstFetchTick = %d with NO observability clock injected, want 0. A default silt build must record no first-fetch time for a fetcher at all — the tuple is (identity, bytes), never (identity, bytes, WHEN). See D-BB-BUILD-TAG", i, a.firstFetchTick)
		}
	}
}

// bondTick is a real walltime.Now() reading plus the +1 the auditor adds
// (core/node/bondaudit.go; the +1 once kept the deleted first-seen stamp's unset
// guard off zero and now serves no reader — see the comment there). The magnitude is
// the point: the ledger has to be shown handling a Unix-nanosecond, not a small
// counter.
const bondTick = uint64(1_788_599_138_518_548_000) + 1

// TestR29aBondChallengeStampsNoFirstTouch is the INVERSION of the gate that used to
// stand here, TestR29aBondChallengeStillStampsFirstSeenTick, and the other half of the
// gate above: a default build records no `when` for a fetcher, and the BOND path
// records no `when` either.
//
// WHAT WAS WRONG BEFORE. RecordBondChallenge wrote account.firstSeenTick — the
// wall-clock nanosecond of the first challenge an identity answered — and the old gate
// here pinned that write as "something else's mechanism" that removing would break.
// There was no other mechanism. Nothing read the field in any build configuration:
// DecayStale reads lastBondTick, Reputation reads neither, the census reads
// firstFetchTick. A retained `when` that no decided function needs is SURPLUS under
// T-DONT3 prong (a) (D-DONT3-READING, docs/decisions.md), so the write and the field
// are deleted (G-BB-28) and residual R-BB-BOND-STAMP-TUPLE is CLOSED. Certification:
// silt-reviews/research/research-outcome/R2.9a-DONT3-READING-AND-BOND-STAMP-TUPLE-RESEARCH-CERTIFICATION-2026-09-05.md §2.
//
// WHAT A BOND CHALLENGE WRITES INSTEAD is exactly one tick, lastBondTick, and that one
// is RETENTION, not a first-touch stamp: it moves on every passing challenge, so it is a
// liveness reading rather than a `when`. TestR29aRetentionReadsLastBondTickInNanoseconds
// pins its unit.
//
// RUNTIME GATE: core/node's TestR29aBondAuditStampsAWallClockNanosecondNotACounter
// measures what the auditor actually passes; this gate covers what the ledger does with
// it.
func TestR29aBondChallengeStampsNoFirstTouch(t *testing.T) {
	l := New(1_000, 500_000)
	prover := ports.HashBytes([]byte("prover"))
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, bondTick)
	a := l.accounts[prover]
	if a == nil {
		t.Fatal("no account after RecordBondChallenge — the fixture is not reaching the ledger and every assertion below is vacuous")
	}
	if a.bondedBytes != 1<<20 {
		t.Fatalf("bondedBytes = %d after a passing challenge, want %d — the challenge did not land", a.bondedBytes, 1<<20)
	}
	if a.firstFetchTick != 0 {
		t.Fatalf("firstFetchTick = %d after a bond challenge, want 0. A bond challenge is not a fetch and must not place an identity on the census age axis (G-BB-24)", a.firstFetchTick)
	}
	// THE STRUCTURAL HALF. The field itself is gone, and this reads the TYPE rather
	// than the source: the set of tick-typed fields on account is CLOSED, exactly
	// {firstFetchTick, lastBondTick}, so a tick added under ANY name fails here with
	// that name in the message. It is a whitelist on the field's type, not a match on
	// its name. The earlier form of this check matched names containing "firstseen";
	// the blind review re-added the stamp as `bondSeenTick` with the identical
	// unset-guarded write and every gate stayed green (RULING-R2.9a-four-residuals,
	// Blocker 1). Under this form that ablation is RED, as is `bondSeenAt ports.Time`.
	//
	// WHAT COUNTS AS A TICK TYPE, and the honest limit. Every uint64 on account is a
	// clock reading and every byte or count is int64/int, so on this struct uint64 IS
	// the tick discriminator; ports.Time and ports.Duration are added because they are
	// the clock types a later hand would reach for. A `when` smuggled in as a bare
	// int64 under a byte-count-shaped name is not caught by this gate.
	assertAccountTickSetIsClosed(t)
	if a.lastBondTick != bondTick {
		t.Fatalf("lastBondTick = %d after a passing challenge at %d, want the tick verbatim — retention is the ONE thing a bond challenge stamps, and it must survive the deletion untouched", a.lastBondTick, bondTick)
	}
}

// assertAccountTickSetIsClosed is the closed-set gate TestR29aBondChallengeStampsNoFirstTouch
// relies on: every tick-typed field on account (uint64, ports.Time, ports.Duration) must be
// one of firstFetchTick and lastBondTick, and both must be present so the whitelist
// cannot go vacuous by a rename.
func assertAccountTickSetIsClosed(t *testing.T) {
	t.Helper()
	wantTicks := map[string]bool{"firstFetchTick": true, "lastBondTick": true}
	tickTypes := map[reflect.Type]bool{
		reflect.TypeOf(ports.Time(0)):     true,
		reflect.TypeOf(ports.Duration(0)): true,
	}
	at := reflect.TypeOf(account{})
	seen := map[string]bool{}
	for i := 0; i < at.NumField(); i++ {
		f := at.Field(i)
		if f.Type.Kind() != reflect.Uint64 && !tickTypes[f.Type] {
			continue
		}
		if !wantTicks[f.Name] {
			t.Fatalf("account has a tick-typed field %q (%s) outside the closed set {firstFetchTick, lastBondTick}. The bond-path first-touch stamp was deleted under G-BB-28 (T-DONT3 prong (a): a `when` no decided function reads is SURPLUS); a bond challenge writes lastBondTick and nothing else, and a new `when` under any name needs its reader named and the set here re-derived, not a quiet field", f.Name, f.Type)
		}
		seen[f.Name] = true
	}
	for name := range wantTicks {
		if !seen[name] {
			t.Fatalf("account has no tick-typed field %q — the closed set this gate asserts no longer matches the struct, so the whitelist is vacuous; re-derive it before trusting this gate", name)
		}
	}
}

// TestR29aRetentionReadsLastBondTickInNanoseconds is G-BB-28's third item: the
// ablation that proves the deletion above was surgical. lastBondTick must still advance
// on a passing challenge, and DecayStale must still zero a lapsed bond at BondMaxAge —
// and it must do so in NANOSECONDS, because that is the unit the daemon feeds it.
//
// WHY THE UNIT IS PINNED AND NOT JUST THE ORDERING. core/node/bondaudit.go calls
// DecayStale(now, uint64(n.cfg.BondMaxAge)) with BondMaxAge = 300 * ports.Second
// (core/node/node.go DefaultConfig) and a `now` off the same walltime clock the tick
// came from. The second arm shows what a counter would do: sweeps numbered 1, 2, 3, …
// never accumulate 300 s of "age", so DecayStale never fires and retention is silently
// disabled — no test goes red, standing simply stops lapsing. A later hand that
// re-denominates lastBondTick "to tidy up" fails here.
func TestR29aRetentionReadsLastBondTickInNanoseconds(t *testing.T) {
	const maxAge = uint64(300 * ports.Second) // core/node/node.go DefaultConfig().BondMaxAge, in nanoseconds
	if maxAge != 300_000_000_000 {
		t.Fatalf("300 * ports.Second = %d, want 300e9 — ports.Duration is no longer nanoseconds and every retention comparison in this package changed meaning", maxAge)
	}
	prover := ports.HashBytes([]byte("prover"))

	// ARM 1 — nanoseconds, the production unit. Two passing challenges advance the
	// tick; a `now` one nanosecond past BondMaxAge from the LAST proof retires the
	// standing, and a `now` exactly at BondMaxAge does not.
	l := New(1_000, 500_000)
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, bondTick)
	later := bondTick + uint64(3600*ports.Second)
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, later)
	if got := l.accounts[prover].lastBondTick; got != later {
		t.Fatalf("lastBondTick = %d after a second passing challenge at %d, want it ADVANCED — retention is a last-proof reading, not a first-touch stamp", got, later)
	}
	l.DecayStale(later+maxAge, maxAge)
	if l.accounts[prover].bondedBytes == 0 {
		t.Fatalf("a bond proven exactly BondMaxAge ago was retired; DecayStale must retire strictly older proofs only")
	}
	l.DecayStale(later+maxAge+1, maxAge)
	if got := l.accounts[prover].bondedBytes; got != 0 {
		t.Fatalf("bondedBytes = %d one nanosecond past BondMaxAge, want 0 — a bond that stops being re-proven must lapse, and this is the retention surface the G-BB-28 deletion had to leave intact", got)
	}

	// ARM 2 — the failure a counter would produce. A ledger fed sweep numbers instead
	// of nanoseconds never sees 300e9 elapse, so standing never lapses. The assertion
	// is on the MECHANISM: this is why lastBondTick must not be re-denominated.
	c := New(1_000, 500_000)
	c.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, 1)
	c.DecayStale(1_000_000, maxAge) // a million sweeps later, as a counter would count them
	if c.accounts[prover].bondedBytes == 0 {
		t.Fatalf("a counter-valued tick lapsed under a nanosecond BondMaxAge — the arithmetic changed; re-derive the unit before trusting arm 1")
	}
}

// TestR29aLedgerStateIsEmptyInADefaultBuild pins the SIZE claim: untagged, the
// instrument's whole ledger-side state is a zero-byte struct, so the field on Ledger
// costs nothing and cannot hold a clock. Tagged, it holds the two injected sources and
// is non-empty — which is why this asserts a direction rather than a number.
func TestR29aLedgerStateIsEmptyInADefaultBuild(t *testing.T) {
	size := reflect.TypeOf(bbootstrapState{}).Size()
	tagged := reflect.TypeOf(&Ledger{}).Elem().NumField() // referenced so the type stays live
	_ = tagged
	if hasBBootstrap && size == 0 {
		t.Fatalf("bbootstrapState is zero-sized under the `bbootstrap` build tag — the two injected time sources are missing, so the age axis cannot work")
	}
	if !hasBBootstrap && size != 0 {
		t.Fatalf("bbootstrapState occupies %d bytes in a DEFAULT build, want 0. A default silt binary must carry no observability clock, no monotone source and no injection origin (D-BB-BUILD-TAG)", size)
	}
}
