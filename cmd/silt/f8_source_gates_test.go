package main

// R2.10 / F8 — the two cmd/silt SOURCE gates (Tester, 2026-09-04), in the
// rt_r04b_c3_laneoff_test.go style: cmdDaemon's wiring sits inside the flag-driven
// block with no callable seam, so these see literals and their order, nothing else,
// and every failure says so. Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §5 R-F8-DISABLED, §6 G-F8-3 and G-F8-6.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestF8_DaemonRefusesPaidLanesWithoutAnEpochClock_Source is the text half of G-F8-3.
//
// RUNTIME GATE: e2e TestF8_PaidLanesRefuseToStartWithoutAnEpochClock (an OS-process
// daemon with each paid-lane flag and effective EpochBlocks == 0 exits naming both
// flags and -epoch-blocks; the -epoch-blocks 8 control reaches ACCEPTING). The
// brick itself is core/credit TestSerialGuard_SetIsBounded's ReasonGuardFull
// assertion.
func TestF8_DaemonRefusesPaidLanesWithoutAnEpochClock_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripLineComments(string(src))
	const (
		receipts = "-accept-delivery-receipts"
		relay    = "-accept-relay-payments"
		epochs   = "-epoch-blocks"
		marker   = "refusing to start"
	)
	found := -1
	for at := 0; ; {
		i := strings.Index(body[at:], marker)
		if i < 0 {
			break
		}
		i += at
		lo := strings.LastIndex(body[:i], "\n") + 1
		hi := strings.Index(body[i:], "\n")
		if hi < 0 {
			hi = len(body)
		} else {
			hi += i
		}
		line := body[lo:hi]
		if strings.Contains(line, receipts) && strings.Contains(line, relay) && strings.Contains(line, epochs) {
			found = i
			break
		}
		at = i + len(marker)
	}
	if found < 0 {
		t.Fatalf("SOURCE GATE: no `%s` line in daemon.go names %s, %s AND %s together. Checked: "+
			"three literals on one line, per occurrence of the marker. R-F8-DISABLED: with effective "+
			"EpochBlocks == 0 nothing expires and the paid lanes brick at the guard cap; the daemon "+
			"must refuse both flags with a message naming all three. The runtime consequence is "+
			"observed by e2e TestF8_PaidLanesRefuseToStartWithoutAnEpochClock", marker, receipts, relay, epochs)
	}
	// The refusal must be conditioned on the EFFECTIVE epoch cadence, not the raw
	// flag: a defaulted objective validator gets DerivedEpochBlocks and must not be
	// refused. Text proximity only.
	lo := found - 1200
	if lo < 0 {
		lo = 0
	}
	if win := body[lo:found]; !strings.Contains(win, "effEpoch == 0") {
		t.Fatalf("SOURCE GATE: the paid-lane refusal at byte %d is not preceded (within 1200 bytes) by "+
			"`effEpoch == 0`. Checked: text proximity of one literal, which is not control flow. The "+
			"refusal must key on effectiveEpochBlocks' result (an explicit 0 or a non-objective posture "+
			"with -epoch-blocks unset), never on *epochBlocks alone", found)
	}
	if !strings.Contains(body[found:], "return fmt.Errorf(") && !strings.Contains(body[lo:found], "return fmt.Errorf(") {
		t.Fatal("SOURCE GATE: no `return fmt.Errorf(` near the paid-lane refusal in daemon.go. Checked: " +
			"text proximity only. A printed warning that does not return out of cmdDaemon is not a refusal")
	}
}

// TestF8_DaemonWiresTheLedgerEpochSourceToTheNode is the text half of G-F8-6.
//
// RUNTIME GATE: TestF8_LedgerEpochFollowsTheChainThroughTheDaemonSeam (this package;
// the seam on a real chain + node). UNGATED: that cmdDaemon's call to the seam
// executes before the first receipt or relay open can arrive on a live socket — the
// same daemon-tier composition residual TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists
// and TestDaemonWiresTheCreditSpentStoreBesideThePaidSerialStore carry.
func TestF8_DaemonWiresTheLedgerEpochSourceToTheNode(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripLineComments(string(src))
	call := regexp.MustCompile(`wireLedgerEpochSource\(\s*ledger\s*,\s*nd\s*\)`)
	hits := call.FindAllStringIndex(body, -1)
	if len(hits) != 1 {
		t.Fatalf("SOURCE GATE: daemon.go calls `wireLedgerEpochSource(ledger, nd)` %d times, want exactly 1. "+
			"Checked: a regex count. Zero means the daemon's ledger reads NO source — every guarded redeem "+
			"and anchor spend runs at epoch 0 and the paid lanes brick at the guard cap (R-F8-SOURCE, "+
			"R-F8-DISABLED); more than one means the ordering below no longer decides which source wins. "+
			"If the Builder renamed the seam, re-anchor this literal and the runtime test together", len(hits))
	}
	iCall := hits[0][0]
	iNode := strings.Index(body, "nd := node.New(")
	iLedger := strings.Index(body, "ledger := credit.New(")
	if iNode < 0 || iLedger < 0 {
		t.Fatal("SOURCE GATE: `nd := node.New(` or `ledger := credit.New(` is absent from daemon.go — re-anchor this gate")
	}
	if !(iNode < iCall && iLedger < iCall) {
		t.Fatalf("SOURCE GATE: byte offsets in daemon.go are out of order — node %d, ledger %d, wiring call %d; "+
			"required node < call and ledger < call. Checked: SOURCE ORDER of three literals, which is not "+
			"execution order", iNode, iLedger, iCall)
	}
}
