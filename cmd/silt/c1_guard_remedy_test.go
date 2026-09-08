package main

// C1 / B1 (blind PE ruling RULING-c1-demand-v2-and-flat-leg-retirement-a290bff-2026-09-08.md):
// adapters/guardstore is opened on TWO files with DIFFERENT safety properties, and the
// format bump's refusal reaches an operator through BOTH.
//
//   - paidserials.log — through the RC the credit ledger is ephemeral (D-FP2-SCOPE), so
//     the guarded payouts' credits reset at the same restart and clearing the file costs
//     nothing.
//   - creditspent.log — the publish issuer key PERSISTS, so every credit it signed stays
//     spendable. Clearing that guard on its own re-opens every held credit for a second
//     spend: the F-4 pump, measured at core/node/r213b_creditspent_test.go (one 50,000
//     credit redeemed 43,750 + 43,750 against one burn). The ratified rule is to rotate
//     the publish key AND clear creditspent.log TOGETHER (R-CREDITSPENT-UNBOUNDED, owner
//     call 6, D-TRUE-UP-CALLS-2026-09-07; docs/design/m0.md, docs/decisions.md).
//
// So the adapter's sentinel states the CONDITION and names no remedy, and the DAEMON
// attaches the remedy for the file it opened. These gates pin that pairing: the text
// itself (runtime), and the file-to-remedy wiring at the two open sites (source).

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/guardstore"
)

// TestGuardStoreRemedyTextIsSafePerStore is the TEXT gate: what an operator reads on a
// refused boot. The creditspent remedy must carry the ratified rotate-and-clear pairing
// and must never carry the paidserials "costs nothing" licence, which on that file IS
// the F-4 pump.
func TestGuardStoreRemedyTextIsSafePerStore(t *testing.T) {
	cs := fmt.Sprintf("%v", fmt.Errorf("publish-credit guard store: %w\n%s",
		fmt.Errorf("%w: /store/creditspent.log", guardstore.ErrLegacyFormat), remedyCreditSpent))
	ps := fmt.Sprintf("%v", fmt.Errorf("delivery-credit guard store: %w\n%s",
		fmt.Errorf("%w: /store/paidserials.log", guardstore.ErrLegacyFormat), remedyPaidSerials))

	// The adapter sentinel is store-NEUTRAL: it reaches both files, so it may not name
	// one of them.
	if strings.Contains(guardstore.ErrLegacyFormat.Error(), "paid-serial") ||
		strings.Contains(guardstore.ErrLegacyFormat.Error(), "paidserials") {
		t.Errorf("guardstore.ErrLegacyFormat names the paid-serial store, but the same sentinel "+
			"refuses creditspent.log: %q", guardstore.ErrLegacyFormat.Error())
	}
	if !errors.Is(fmt.Errorf("%w: x", guardstore.ErrLegacyFormat), guardstore.ErrLegacyFormat) {
		t.Fatal("ErrLegacyFormat no longer wraps")
	}

	// creditspent.log: the ratified procedure, and nothing that reads as "just delete it".
	for _, want := range []string{"creditspent.log", "rotate", "publish key", "together"} {
		if !strings.Contains(strings.ToLower(cs), strings.ToLower(want)) {
			t.Errorf("the creditspent.log refusal does not contain %q — an operator reading it has "+
				"no ratified procedure and the obvious guess (clear the file) is the F-4 double-spend "+
				"pump.\nGot:\n%s", want, cs)
		}
	}
	for _, banned := range []string{"costs nothing", "removing it", "remove the file"} {
		if strings.Contains(strings.ToLower(cs), banned) {
			t.Errorf("the creditspent.log refusal contains %q. That licence is true only for "+
				"paidserials.log; on creditspent.log clearing the guard alone re-opens every held "+
				"credit for a second spend.\nGot:\n%s", banned, cs)
		}
	}
	if strings.Contains(cs, "paidserials.log") {
		t.Errorf("the creditspent.log refusal names paidserials.log — the two remedies have "+
			"drifted onto one file.\nGot:\n%s", cs)
	}

	// paidserials.log: the remedy that IS "remove it", with the reason it is safe here.
	for _, want := range []string{"paidserials.log", "remove", "D-FP2-SCOPE"} {
		if !strings.Contains(ps, want) {
			t.Errorf("the paidserials.log refusal does not contain %q — the operator is refused "+
				"with no remedy.\nGot:\n%s", want, ps)
		}
	}
	if strings.Contains(ps, "creditspent.log") {
		t.Errorf("the paidserials.log refusal names creditspent.log — the two remedies have "+
			"drifted onto one file.\nGot:\n%s", ps)
	}
}

// TestDaemonPairsEachGuardStoreWithItsOwnRemedy is the SOURCE gate, in the
// r213b_creditspent_wiring_test.go style: the daemon has no callable seam around the two
// open sites, so the wiring is checked as literals and their order. It sees which remedy
// constant is cited in which file's block and nothing else.
//
// RUNTIME GATE: TestGuardStoreRemedyTextIsSafePerStore (the CONTENT of the two remedy
// constants, asserted on the exact wrapped string an operator reads).
// UNGATED: the daemon-tier composition — that a real refused boot on a pre-bump store dir
// prints the remedy for the file it opened — has no runtime observer, the same residual
// TestDaemonWiresTheCreditSpentStoreBesideThePaidSerialStore already carries for the
// wiring it checks.
func TestDaemonPairsEachGuardStoreWithItsOwnRemedy(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripLineComments(string(src))

	const (
		openPS = `guardstore.Open(filepath.Join(*storeDir, "paidserials.log"))`
		openCS = `guardstore.Open(filepath.Join(*storeDir, "creditspent.log"))`
	)
	iPS, iCS := strings.Index(body, openPS), strings.Index(body, openCS)
	if iPS < 0 || iCS < 0 {
		t.Fatal("SOURCE GATE: one of the two guardstore.Open literals is gone from daemon.go, so " +
			"this gate has no anchor for 'which remedy goes with which file' — re-anchor it")
	}
	// The wraps for one store live in the block that opens it. 1200 bytes covers the
	// open-error wrap and the load-error wrap for either site; both were measured to fit.
	win := func(i int) string {
		w := body[i:]
		if len(w) > 1200 {
			w = w[:1200]
		}
		return w
	}
	psWin, csWin := win(iPS), win(iCS)

	if n := strings.Count(csWin, "remedyCreditSpent"); n < 2 {
		t.Errorf("SOURCE GATE: `remedyCreditSpent` appears %d times in the 1200 bytes after the "+
			"creditspent.log open, want >= 2 (the open-error wrap and the load-error wrap). A "+
			"refusal with no remedy is the defect: the obvious operator guess for that file is "+
			"the F-4 double-spend pump", n)
	}
	if strings.Contains(csWin, "remedyPaidSerials") {
		t.Error("SOURCE GATE: the creditspent.log block cites `remedyPaidSerials`. That remedy says " +
			"clearing the file costs nothing, which on creditspent.log re-opens every held credit " +
			"for a second spend")
	}
	if n := strings.Count(psWin, "remedyPaidSerials"); n < 2 {
		t.Errorf("SOURCE GATE: `remedyPaidSerials` appears %d times in the 1200 bytes after the "+
			"paidserials.log open, want >= 2 (the open-error wrap and the LoadPaidSerials wrap)", n)
	}
	if strings.Contains(psWin, "remedyCreditSpent") {
		t.Error("SOURCE GATE: the paidserials.log block cites `remedyCreditSpent` — the two remedies " +
			"have drifted onto one file")
	}
}
