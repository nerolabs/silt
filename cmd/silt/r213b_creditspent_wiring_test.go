package main

// R2.13b — creditSpent durability (F-4), gate G-CS-5: the DAEMON wires the credit-spent
// store beside the paid-serial store — same store dir, a DISTINCT file
// (creditspent.log), attached then loaded, refuse-to-start on a load error. PE ruling
// RULING-F4-creditSpent-durability-and-F3-fee-constancy-2026-09-04.md §3 item 4;
// deliberation docs/thinking/2026-09-04-r2.13b-creditspent-durability-design.md §3.
//
// A SOURCE gate, in the rt_r04b_c3_laneoff_test.go style: the wiring sits inside
// runDaemon's flag-driven block with no callable seam. It can see literals and their
// order, nothing else, and every failure says so.

import (
	"os"
	"strings"
	"testing"
)

// RUNTIME GATE: core/node TestCreditSpentSurvivesIssuerRestart (the restart replay
// through the shipped handler, on the store seam) and core/node
// TestCreditSpentDiskStoreIsASecondFileBesidePaidSerials (a real creditspent.log,
// re-opened, distinct from paidserials.log).
// UNGATED: the daemon-tier composition — that runDaemon's LoadCreditSpent completes
// before the first credit-bearing request can arrive on a live socket — has no runtime
// observer, the same residual TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists
// carries for the paid-serial guard.
func TestDaemonWiresTheCreditSpentStoreBesideThePaidSerialStore(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	// CODE only: this gate's own subject names the file in comments elsewhere.
	body := stripLineComments(string(src))

	const (
		openCS = `guardstore.Open(filepath.Join(*storeDir, "creditspent.log"))`
		openPS = `guardstore.Open(filepath.Join(*storeDir, "paidserials.log"))`
		attach = "SetCreditSpentStore("
		load   = "LoadCreditSpent()"
	)
	iOpenPS := strings.Index(body, openPS)
	iOpenCS := strings.Index(body, openCS)
	iAttach := strings.Index(body, attach)
	iLoad := strings.Index(body, load)

	if iOpenPS < 0 {
		t.Fatal("SOURCE GATE: the literal `" + openPS + "` is gone from daemon.go, so this " +
			"gate has no anchor for 'the same store dir' — re-anchor it")
	}
	if iOpenCS < 0 {
		t.Fatal("SOURCE GATE: the literal `" + openCS + "` is absent from daemon.go. Checked: " +
			"one literal — the daemon opens NO credit-spent store, so the node's creditSpent is " +
			"process memory on the shipped binary. The consequence (an issuer restart honours " +
			"every held credit again, PE F-4) is observed by core/node " +
			"TestCreditSpentSurvivesIssuerRestart at the node tier, not here")
	}
	if c := strings.Count(body, `"creditspent.log"`); c != 1 {
		t.Fatalf("SOURCE GATE: the literal `\"creditspent.log\"` occurs %d times in daemon.go "+
			"code, want exactly 1 (one open site, one file)", c)
	}
	if iAttach < 0 || iLoad < 0 {
		t.Fatal("SOURCE GATE: `" + attach + "` or `" + load + "` is absent from daemon.go. " +
			"Checked: two literals. A store that is opened but never attached-and-loaded is " +
			"inert; the restore half of 'a restart is not an eviction' is the node-tier " +
			"LoadCreditSpent, observed by core/node TestCreditSpentSurvivesIssuerRestart")
	}
	if !(iOpenCS < iAttach && iAttach < iLoad) {
		t.Fatalf("SOURCE GATE: byte offsets in daemon.go are out of order — open %d, attach %d, "+
			"load %d; required open < attach < load. Checked: SOURCE ORDER of three literals, "+
			"which is not execution order", iOpenCS, iAttach, iLoad)
	}
	// Refuse-to-start on a load error: silently starting empty IS the eviction. The
	// window after the load call must return out of runDaemon.
	win := body[iLoad:]
	if len(win) > 400 {
		win = win[:400]
	}
	if !strings.Contains(win, "return fmt.Errorf(") {
		t.Fatal("SOURCE GATE: no `return fmt.Errorf(` within 400 bytes after `" + load + "` in " +
			"daemon.go. Checked: text proximity only. The behaviour hoped for — a corrupt or " +
			"over-cap creditspent.log stops the daemon instead of starting with an empty guard — " +
			"is UNGATED at runtime")
	}
}
