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

// TestDaemonArmsTheRotatorOnlyAfterABootInstall is the H-2 structure gate (PE ruling
// @ 271ab81, 2026-09-03). `rotateDemandKeys` used to be assigned ABOVE the boot
// install, so the boot-rotation-failure branch printed "LANE OFF" and `break`ed out
// with the scheduler still armed: the OnCommit hook kept rotating, and the daemon went
// on staging IssuerKeyReg commitments into consensus and charging withdrawal fees for
// tokens a nil demandBank would deny forever.
//
// The fix is the ORDER — the single assignment now sits BELOW the single failure exit,
// so no branch can arm a lane it has just declared off. Order is exactly what a source
// gate can see, so this gate is faithful to its own limits.
//
// UNGATED: R-LANEOFF-ROTATION-RUNTIME — that no rotation goroutine actually RUNS after
// a failed boot install has no runtime observer. Reaching that branch in a real daemon
// needs an unwritable issuer directory whose publish-token key already exists (verified
// by hand, 2026-09-03: the daemon prints "LANE OFF — the boot key rotation failed" and
// keeps serving), and OBSERVING the difference additionally needs the chain to cross an
// epoch boundary, which on a lone validator needs a driven publish. Filed rather than
// approximated: a gate that waits for a line that cannot appear proves nothing.
func TestDaemonArmsTheRotatorOnlyAfterABootInstall(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	// Count on CODE only. This file's own subject matter names the forbidden
	// assignment inside a comment in daemon.go, and a naive count reads that as a
	// second assignment — the comment-masking trap scripts/check_source_gates.py
	// documents.
	body := stripLineComments(string(src))
	iInstall := strings.Index(body, "if kerr := installDemandKeys(")
	iArm := strings.Index(body, "rotateDemandKeys = func(")
	iCount := strings.Count(body, "rotateDemandKeys = ")
	switch {
	case iInstall < 0 || iArm < 0:
		t.Fatal("SOURCE GATE: `if kerr := installDemandKeys(` or `rotateDemandKeys = func(` " +
			"is absent from daemon.go. Checked: two literals — re-anchor this gate")
	case iArm < iInstall:
		t.Fatalf("SOURCE GATE: daemon.go assigns rotateDemandKeys at byte %d, ABOVE the boot "+
			"install at byte %d. Checked: SOURCE ORDER of two literals, which is not "+
			"execution order. The consequence — a daemon that announces LANE OFF and then "+
			"keeps rotating, staging on-chain key commitments and charging withdrawal fees "+
			"for a bank that will deny every receipt (PE H-2) — is UNGATED at runtime "+
			"(R-LANEOFF-ROTATION-RUNTIME)", iArm, iInstall)
	case iCount != 1:
		t.Fatalf("SOURCE GATE: daemon.go has %d assignments to rotateDemandKeys, want exactly "+
			"1. Checked: a literal count. More than one assignment means the ordering above "+
			"no longer decides whether the scheduler is armed, and this gate stops being a "+
			"faithful check of the H-2 property", iCount)
	}
}

// stripLineComments blanks `//` line comments, preserving byte offsets and line
// structure so a source gate's offsets stay comparable with the raw file. It is
// deliberately naive about string literals containing "//": no gate in this file
// anchors on one, and a smarter mask is a parser, not a helper.
func stripLineComments(src string) string {
	out := []byte(src)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == '/' && out[i+1] == '/' {
			for j := i; j < len(out) && out[j] != '\n'; j++ {
				out[j] = ' '
			}
		}
	}
	return string(out)
}
