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
// structural — stampFirstTouch is an empty function (bbootstrap_off.go) and there is no
// setter to call. In a tagged build it is the -bbootstrap default: the daemon calls
// bbootstrapInject(ledger, clk, false), which injects nothing.
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
		if a.firstSeenTick != 0 {
			t.Fatalf("requester %d: firstSeenTick = %d with NO observability clock injected, want 0. A default silt build must record no first-seen time for a fetcher at all — the tuple is (identity, bytes), never (identity, bytes, WHEN). See D-BB-BUILD-TAG", i, a.firstSeenTick)
		}
	}
}

// TestR29aBondChallengeStillStampsFirstSeenTick is the OTHER HALF, and it is what keeps
// the gate above from being read as "firstSeenTick is dead".
//
// RecordBondChallenge's write PREDATES R2.9a entirely. It is stamped from the bond
// auditor's own request counter (core/node/bondaudit.go), not from a wall clock, and it
// fires only for a validator answering a storage-bond challenge — never for a fetcher.
// D-BB-BUILD-TAG does not touch it, and a future change that removes it is removing
// something else's mechanism, not this one's.
func TestR29aBondChallengeStillStampsFirstSeenTick(t *testing.T) {
	l := New(1_000, 500_000)
	prover := ports.HashBytes([]byte("prover"))
	l.RecordBondChallenge(prover, ports.Hash{1}, 1<<20, true, 77)
	if got := l.accounts[prover].firstSeenTick; got != 77 {
		t.Fatalf("firstSeenTick = %d after a bond challenge at tick 77, want 77 — RecordBondChallenge's stamp predates R2.9a and must survive the build tag untouched", got)
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
