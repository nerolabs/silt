package main

// R0.4b C3 re-break — F7 regression gate (blast radius). Inversion of the red-team
// probe adapters/diskissuer/rt_c3b_store_test.go RT-C3B-13, at the tier the finding is
// actually about: the DAEMON, not the store.
//
// The finding: `Load` treats a corrupt file as a hard error (correct — see
// TestRTC3_CorruptStoreErrorsAndIsNeverRewritten) and runDaemon returned it straight
// out, so ONE bad byte in demandkeys.cbor stopped chain participation, storage and
// serving. The demand receipt lane is OPTIONAL; a validator must not die for it.
//
// These are SOURCE gates, in the depcheck style, because the branch they protect sits
// deep inside runDaemon's flag-driven wiring and has no callable seam. A source gate can
// see STRINGS and ORDER and nothing else, so every failure message below is prefixed
// SOURCE GATE: and states what was checked, never the behaviour hoped to follow.
//
// Why that discipline: the Tester reintroduced the exact F7 regression with every string
// below intact — a DIFFERENT early return above the switch — and this file stayed GREEN
// with `go vet` clean (scar-verifies-x-must-name-the-axes, count=3, third-time rule
// fired 2026-09-03). The behaviour now has its own runtime gate, named per test, and
// scripts/check_source_gates.py enforces both halves repo-wide.

import (
	"os"
	"strings"
	"testing"
)

// RUNTIME GATE: e2e.TestDaemonSurvivesACorruptDemandKeyStore drives a real `silt daemon`
// over a corrupt demandkeys.cbor and observes the four things this gate can only imply:
// the process survives, the lane never arms, the operator line is exact, and the file is
// byte-unchanged. THIS test only pins the source shape so the wiring is not silently
// re-plumbed between e2e runs.
func TestDaemonDegradesTheDemandLaneInsteadOfDying(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// The demand-lane wiring block, delimited by the store open and the ACCEPTING line.
	start := strings.Index(body, `des, derr := diskissuer.OpenEpochs(`)
	if start < 0 {
		t.Fatal("SOURCE GATE: the literal `des, derr := diskissuer.OpenEpochs(` is no longer " +
			"present in daemon.go, so this gate has nothing to anchor on — re-anchor it")
	}
	end := strings.Index(body[start:], "delivery receipts: ACCEPTING")
	if end < 0 {
		t.Fatal("SOURCE GATE: the literal `delivery receipts: ACCEPTING` is gone from " +
			"daemon.go, so the wiring block has no end delimiter — re-anchor this gate")
	}
	block := body[start : start+end]

	if !strings.Contains(block, "des.Load()") {
		t.Fatal("SOURCE GATE: the substring `des.Load()` is absent from the demand-lane " +
			"wiring block. Checked: text only. The behaviour at stake — a corrupt store " +
			"discovered on a path that returns out of runDaemon — is observed by " +
			"e2e.TestDaemonSurvivesACorruptDemandKeyStore, not here")
	}
	if !strings.Contains(block, "LANE OFF") {
		t.Fatal("SOURCE GATE: the substring `LANE OFF` is absent from the demand-lane " +
			"wiring block. Checked: text only. Whether a corrupt demand key file actually " +
			"stops the RECEIPT LANE rather than chain participation, storage and serving " +
			"(red-team re-break F7) is observed by " +
			"e2e.TestDaemonSurvivesACorruptDemandKeyStore")
	}
	// The specific regression: returning the store error out of runDaemon.
	for _, forbidden := range []string{
		`return fmt.Errorf("demand issuer keys: %w", kerr)`,
	} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("SOURCE GATE: the exact forbidden line %q is back in the demand-lane "+
				"wiring block. Checked: one literal. This gate cannot see any OTHER "+
				"spelling of the same return — e2e.TestDaemonSurvivesACorruptDemandKeyStore "+
				"is what catches those", forbidden)
		}
	}
}

// TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists is the F2 wiring gate: the
// durable guard must be attached AND loaded before anything can accept a receipt.
// Ordering is the property, so it is asserted on the source order.
//
// RUNTIME GATE: core/node TestRTC3_RestartDoesNotRePayTheSameWireReceipt observes the
// PROPERTY this ordering exists to protect — a restart must not let the same wire
// receipt pay twice — against a real ledger and a real guard store across two boots.
// UNGATED: the daemon-tier composition (that runDaemon's own load completes before the
// first receipt can arrive on a live socket) has no runtime observer. Closing it needs a
// receipt delivered into the boot window, which the e2e harness cannot currently time.
func TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	iAttach := strings.Index(body, "SetPaidSerialStore(")
	iLoad := strings.Index(body, "LoadPaidSerials()")
	iBank := strings.Index(body, "nd.EnableDemandBank(")
	switch {
	case iAttach < 0 || iLoad < 0:
		t.Fatal("SOURCE GATE: `SetPaidSerialStore(` or `LoadPaidSerials()` is absent from " +
			"daemon.go. Checked: two literals. The consequence — a restart evicting every " +
			"guarded token so the same wire receipt pays twice (red-team re-break F2) — is " +
			"observed by core/node TestRTC3_RestartDoesNotRePayTheSameWireReceipt")
	case iBank < 0:
		t.Fatal("SOURCE GATE: the literal `nd.EnableDemandBank(` is gone from daemon.go, so " +
			"this gate has no third anchor — re-anchor it")
	case !(iAttach < iLoad && iLoad < iBank):
		t.Fatalf("SOURCE GATE: byte offsets in daemon.go are out of order — attach %d, load "+
			"%d, arm %d; required attach < load < arm. Checked: SOURCE ORDER of three "+
			"literals, which is not execution order. Whether the load actually completes "+
			"before a receipt can arrive is UNGATED at this tier", iAttach, iLoad, iBank)
	}
}
