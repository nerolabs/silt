package main

// R2.7 §1.3 — the affordability floor reaches the operator. The counters live on the
// ledger (core/credit) and this is the wire half: they ride the faucetInfo block on
// GET /api/status beside grantsDenied, and they ship WHETHER OR NOT a faucet bucket is
// configured, because a spend gate refuses for want of CREDIT and not for want of a
// token. Dropping them on the unconfigured branch would be a silent loss (Don't #4) on
// exactly the default posture — this fixture has no faucet.
//
// ABLATION run RED before this shipped (recorded in the PR): dropping the two counters
// from the unconfigured-faucet branch of the status assembly reddens this test.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/ports"
)

func TestAffordabilityFloorReachesTheStatusSurface(t *testing.T) {
	s, led := statusServer(t)
	at := time.Unix(1_700_000_000, 0).UTC()

	read := func() faucetInfo {
		t.Helper()
		s.invalidateStatus()
		var fi faucetInfo
		raw := statusKey(t, statusAt(t, s, at, true), "faucet")
		if len(raw) == 0 {
			t.Fatal("no faucet block on /api/status")
		}
		if err := json.Unmarshal(raw, &fi); err != nil {
			t.Fatalf("decode faucet: %v (%s)", err, raw)
		}
		return fi
	}

	base := read()
	if base.Configured {
		t.Fatal("fixture: this ledger has no faucet, so the block must report configured=false — the point of the gate is the UNCONFIGURED branch")
	}
	if base.SpendRefusedInsufficientCredit != 0 || base.SpendRefusersDistinct != 0 {
		t.Fatalf("baseline: refused %d distinct %d, want 0 and 0", base.SpendRefusedInsufficientCredit, base.SpendRefusersDistinct)
	}
	// The honest limit ships WITH the zero, or a reader quotes the zero as a clean bill.
	if !strings.Contains(base.SpendRefusalNote, "zero certifies NOTHING") {
		t.Fatalf("spendRefusalNote = %q, want the FLOOR-detector caveat — a zero that ships without it reads as 'the lane is affordable'", base.SpendRefusalNote)
	}

	// Drive real refusals through the FundEscrow spend gate: two identities, one of them
	// twice. The fixture's fee is 0, so FundEscrow is the gate that can refuse here.
	root := ports.HashBytes([]byte("r27-floor-surface"))
	poor := ports.HashBytes([]byte("poor"))
	alsoPoor := ports.HashBytes([]byte("also-poor"))
	huge := int64(1) << 40
	for _, who := range []ports.NodeID{poor, poor, alsoPoor} {
		if err := led.FundEscrow(root, who, huge); err != ports.ErrInsufficientCredit {
			t.Fatalf("FundEscrow(%d) = %v, want %v — the fixture is not driving a refusal", huge, err, ports.ErrInsufficientCredit)
		}
	}

	got := read()
	if got.SpendRefusedInsufficientCredit != 3 || got.SpendRefusersDistinct != 2 {
		t.Fatalf("after three refusals by two identities: refused %d distinct %d, want 3 and 2 — the floor does not reach the operator on a node with no faucet",
			got.SpendRefusedInsufficientCredit, got.SpendRefusersDistinct)
	}
}
