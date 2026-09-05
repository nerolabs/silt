package credit

// D-BB-BUILD-TAG (ratified 2026-09-05) — the ledger-tier gates for "a default silt build
// contains no part of the B_bootstrap instrument".
//
// THIS FILE CARRIES NO BUILD TAG ON PURPOSE. The first gate asserts a property that must
// hold in BOTH builds — untagged because the mechanism is absent, tagged because
// -bbootstrap was not passed and no clock was injected — so running it in both is the
// point, not an accident. The second and third are `!bbootstrap` only, because they
// assert the ABSENCE of methods that exist under the tag; they live in
// r29a_build_tag_absent_test.go.

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
// the field this gate has to read is firstFetchTick — the census's own. firstSeenTick
// is asserted alongside it because the whole claim is "a default build records no WHEN
// for a fetcher", and either field carrying one would falsify it. This test keeps its
// name because the property it pins is unchanged and D-BB-BUILD-TAG, the CHANGELOG and
// bbootstrap_off.go all cite it.
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
		if a.firstSeenTick != 0 {
			t.Fatalf("requester %d: firstSeenTick = %d after a SERVE, want 0. That field belongs to RecordBondChallenge and no serve-path write may reach it — a fetcher that was never bond-challenged carries no WHEN in any field. See D-BB-BUILD-TAG", i, a.firstSeenTick)
		}
	}
}

// TestR29aBondChallengeStillStampsFirstSeenTick is the OTHER HALF, and it is what keeps
// the gate above from being read as "firstSeenTick is dead".
//
// RecordBondChallenge's write PREDATES R2.9a entirely and D-BB-BUILD-TAG does not touch
// it, so a future change that removes it is removing something else's mechanism.
//
// IT IS A WALL-CLOCK STAMP. This gate used to pass tick = 77, which made the value LOOK
// like a request counter — and the doc comment here said it was one, wrongly, along with
// three other sites. The daemon's auditor passes uint64(n.clock.Now())+1 with a walltime
// clock (core/node/bondaudit.go), so the tick below is a real Unix-nanosecond reading and
// the assertion checks that magnitude survives into firstSeenTick. A reader who sees this
// gate green now re-derives the truth, not the old error.
//
// THE RESIDUAL THIS MAKES VISIBLE: on a -validator node, an identity that is both a
// bonded peer and a fetcher carries (identity, cumulative fetched bytes, first-seen
// wall-clock nanosecond) in a DEFAULT build. Filed as R-BB-BOND-STAMP-TUPLE
// (ROADMAP R2.9a), open, not closed by the build tag.
//
// RUNTIME GATE: core/node's TestR29aBondAuditStampsAWallClockNanosecondNotACounter
// measures what the auditor actually passes; this gate covers what the ledger does with
// it.
func TestR29aBondChallengeStillStampsFirstSeenTick(t *testing.T) {
	// A real walltime.Now() reading, plus the +1 the auditor adds so a first tick is
	// never mistaken for "unset".
	const tick = uint64(1_788_599_138_518_548_000) + 1
	l := New(1_000, 500_000)
	prover := ports.HashBytes([]byte("prover"))
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, tick)
	got := l.accounts[prover].firstSeenTick
	if got != tick {
		t.Fatalf("firstSeenTick = %d after a bond challenge at tick %d, want the tick verbatim — RecordBondChallenge's stamp predates R2.9a and must survive the build tag untouched", got, tick)
	}
	if got < 1_500_000_000_000_000_000 {
		t.Fatalf("firstSeenTick = %d, below Unix-nanosecond magnitude. The bond auditor's tick is a WALL CLOCK (uint64(clock.Now())+1 over adapters/walltime), not a request counter, and this gate must show that magnitude or the next reader inherits the error again (R-BB-BOND-STAMP-TUPLE)", got)
	}
	// A second challenge at a later instant must NOT move the stamp: it is a first-touch
	// value, which is what makes it a `when` rather than a liveness reading.
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, tick+uint64(3600*1e9))
	if again := l.accounts[prover].firstSeenTick; again != tick {
		t.Fatalf("firstSeenTick moved to %d on a later challenge, want %d — the stamp is FIRST touch", again, tick)
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
