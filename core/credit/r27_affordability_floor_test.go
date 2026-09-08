package credit

// R2.7 §1.3 — the affordability floor (Economist advisory
// ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07 §1.3).
//
// Every other counter in this package measures a flow that HAPPENED. These two measure
// the flow that was REFUSED for want of credit — build-immutable #4's exact failure
// mode, and the failure R2.7 is most likely to miss, because an economy in which nobody
// can afford to transact reads solvent.
//
// FLOOR DETECTOR ONLY: non-zero proves honest demand is being refused somewhere and can
// abort a canary; zero certifies NOTHING — an adversary inflates the number at will by
// presenting underfunded identities.
//
// ABLATION run RED before this shipped (recorded in the PR): counting the distinct
// refusers on every refusal rather than on the account's first one turns retries into
// identities and reddens the assertion below.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestSpendRefusalsCountDistinctIdentitiesNotRetries pins the two counters apart:
// retries move the total only, a second identity moves both, and an identity that later
// succeeds decrements neither. It also pins the two spend gates symmetric.
func TestSpendRefusalsCountDistinctIdentitiesNotRetries(t *testing.T) {
	const fee = int64(50_000)
	l := New(fee, 0) // no grant: a fresh identity cannot afford the fee
	root := ports.HashBytes([]byte("r27-floor"))
	poor, alsoPoor, rich := id(1), id(2), id(3)

	if fs := l.FaucetStats(); fs.SpendRefusedInsufficientCredit != 0 || fs.SpendRefusersDistinct != 0 {
		t.Fatalf("baseline: refused %d distinct %d, want 0 and 0", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// One identity, five refusals: five on the total, ONE on the distinct count.
	for i := 0; i < 5; i++ {
		if err := l.ChargePublish(poor); err != ports.ErrInsufficientCredit {
			t.Fatalf("retry %d: ChargePublish = %v, want %v", i, err, ports.ErrInsufficientCredit)
		}
	}
	fs := l.FaucetStats()
	if fs.SpendRefusedInsufficientCredit != 5 || fs.SpendRefusersDistinct != 1 {
		t.Fatalf("after five retries by ONE identity: refused %d distinct %d, want 5 and 1 — retries are being counted as identities", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// A second identity moves both.
	if err := l.ChargePublish(alsoPoor); err != ports.ErrInsufficientCredit {
		t.Fatalf("second identity: ChargePublish = %v, want %v", err, ports.ErrInsufficientCredit)
	}
	fs = l.FaucetStats()
	if fs.SpendRefusedInsufficientCredit != 6 || fs.SpendRefusersDistinct != 2 {
		t.Fatalf("after a second identity: refused %d distinct %d, want 6 and 2", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// THE OTHER SPEND GATE. FundEscrow refuses for want of credit too, and the two must
	// be symmetric or the floor reads low on any node that funds escrows.
	if err := l.FundEscrow(root, poor, 10); err != ports.ErrInsufficientCredit {
		t.Fatalf("FundEscrow: %v, want %v", err, ports.ErrInsufficientCredit)
	}
	fs = l.FaucetStats()
	if fs.SpendRefusedInsufficientCredit != 7 || fs.SpendRefusersDistinct != 2 {
		t.Fatalf("after a FundEscrow refusal by an already-counted identity: refused %d distinct %d, want 7 and 2 — the two spend gates are not symmetric", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}
	if err := l.FundEscrow(root, id(4), 10); err != ports.ErrInsufficientCredit {
		t.Fatalf("FundEscrow by a fresh identity: %v, want %v", err, ports.ErrInsufficientCredit)
	}
	if fs = l.FaucetStats(); fs.SpendRefusedInsufficientCredit != 8 || fs.SpendRefusersDistinct != 3 {
		t.Fatalf("after a fresh identity is refused at FundEscrow: refused %d distinct %d, want 8 and 3", fs.SpendRefusedInsufficientCredit, fs.SpendRefusersDistinct)
	}

	// A refusal followed by a SUCCESS decrements neither: the floor is a lifetime record
	// of demand that was priced out, not a gauge of who is currently poor.
	if err := l.ChargePublish(rich); err != ports.ErrInsufficientCredit {
		t.Fatalf("a third identity's first call must refuse on a zero-grant ledger: %v", err)
	}
	before := l.FaucetStats()
	l.acct(rich).balance += fee
	if err := l.ChargePublish(rich); err != nil {
		t.Fatalf("funded identity: ChargePublish = %v, want nil", err)
	}
	after := l.FaucetStats()
	if after.SpendRefusedInsufficientCredit != before.SpendRefusedInsufficientCredit || after.SpendRefusersDistinct != before.SpendRefusersDistinct {
		t.Fatalf("a later success moved the floor from (%d, %d) to (%d, %d) — it must never decrement",
			before.SpendRefusedInsufficientCredit, before.SpendRefusersDistinct, after.SpendRefusedInsufficientCredit, after.SpendRefusersDistinct)
	}
}
