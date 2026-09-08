package registry_test

// R2.7 §1.3 — the THIRD spend gate's refusal decision is counted (blind PE ruling
// RULING-c4-r27-blocking-telemetry-93deb56-2026-09-08 B2; Economist as-built §4(d)).
//
// The ledger names three spend gates. ChargePublish and FundEscrow count at their own
// refusal branch. CanPublish is a PREDICATE and must not count — the sim calls it for
// display (sim/economy.go), and a counter that moves when a dashboard reads it is worse
// than a missing one. Its one refusal DECISION lives here, in Gated.Publish.
//
// Why it is worth a gate even though NewGated has one non-test caller (sim/economy.go):
// ROADMAP C6 makes ANY rise in spendRefusersDistinct a HARD canary abort, and R2.7's
// adversarial workload runs in the sim tier, where sim/economy.go grades
// FreeloadersRejected off exactly these refusals. A floor reading zero beside a non-zero
// rejection count is an abort input that silently under-reads.
//
// TWO ABLATIONS run RED before this shipped (recorded in the PR): dropping the
// NoteSpendRefused call from gated.Publish, and moving the count into CanPublish (which
// then fires on the pure predicate read this test makes).

import (
	"context"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

func TestGatedPublishRefusalMovesTheAffordabilityFloor(t *testing.T) {
	ctx := context.Background()
	ledger := credit.New(100, 0) // no grant: a fresh publisher cannot afford the fee
	g := registry.NewGated(ledger)
	poor, alsoPoor := ports.HashBytes([]byte("r27-poor")), ports.HashBytes([]byte("r27-also-poor"))

	if fs := ledger.FaucetStats(); fs.SpendRefusedInsufficientCredit != 0 || fs.SpendRefusersDistinct != 0 {
		t.Fatalf("baseline: refused %d distinct %d, want 0 and 0", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// An anonymous entry is refused BEFORE the credit gate. It is not an affordability
	// refusal and must not move the floor.
	if err := g.Publish(ctx, entry(1, 100)); !errors.Is(err, ports.ErrPublisherRequired) {
		t.Fatalf("anonymous publish: %v, want ErrPublisherRequired", err)
	}
	if fs := ledger.FaucetStats(); fs.SpendRefusedInsufficientCredit != 0 {
		t.Fatalf("a missing-publisher refusal moved the affordability floor to %d — the floor must count only refusals for want of CREDIT", fs.SpendRefusedInsufficientCredit)
	}

	// The credit refusal, twice by one identity then once by a second: retries move the
	// total only, a new identity moves both — the same semantics as the other two gates.
	for i := 0; i < 2; i++ {
		if err := g.Publish(ctx, gatedEntry(byte(2+i), poor)); !errors.Is(err, ports.ErrInsufficientCredit) {
			t.Fatalf("retry %d: %v, want ErrInsufficientCredit", i, err)
		}
	}
	if err := g.Publish(ctx, gatedEntry(4, alsoPoor)); !errors.Is(err, ports.ErrInsufficientCredit) {
		t.Fatalf("second identity: %v, want ErrInsufficientCredit", err)
	}
	fs := ledger.FaucetStats()
	if fs.SpendRefusedInsufficientCredit != 3 || fs.SpendRefusersDistinct != 2 {
		t.Fatalf("after three Gated.Publish credit refusals by two identities: refused %d distinct %d, want 3 and 2 — the CanPublish gate's refusal decision is miscounted (0 = uncounted, so the floor under-reads beneath a HARD canary abort; more than 3 = counted twice, which trips that abort on honest traffic) (ROADMAP C6)",
			fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// CanPublish IS A PREDICATE. Reading it must move nothing — sim/economy.go calls it
	// for display, and a floor that stepped on a dashboard render would be worse than a
	// missing one.
	before := ledger.FaucetStats()
	for i := 0; i < 3; i++ {
		if ledger.CanPublish(poor) {
			t.Fatal("fixture: poor must not be able to publish")
		}
	}
	after := ledger.FaucetStats()
	if after.SpendRefusedInsufficientCredit != before.SpendRefusedInsufficientCredit || after.SpendRefusersDistinct != before.SpendRefusersDistinct {
		t.Fatalf("three CanPublish READS moved the floor from (%d, %d) to (%d, %d) — the counter is inside the predicate, so a dashboard render inflates the canary's abort input",
			before.SpendRefusedInsufficientCredit, before.SpendRefusersDistinct, after.SpendRefusedInsufficientCredit, after.SpendRefusersDistinct)
	}
}
