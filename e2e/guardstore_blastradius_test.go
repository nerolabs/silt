package e2e

// R0.4b C3 final round — the paid-serial guard's refuse-to-start is scoped to the
// lane that uses it. PE ruling H-3 @ 271ab81, 2026-09-03.
//
// THE FINDING. `guardstore.Open` + `LoadPaidSerials` ran at daemon boot OUTSIDE any
// flag branch, and a corrupt or unreadable `<store>/paidserials.log` is a
// refuse-to-start. So a pure storage node — one with no delivery lane, that could
// never have written the file — died on a file it does not use. That is the F7
// blast-radius lesson, which the SAME commit applied to `demandkeys.cbor` and did not
// apply to the file it adds.
//
// THE ASYMMETRY IS DELIBERATE, and both halves are asserted here:
//
//   - LANE OFF: the daemon must BOOT and do its real job. Nothing on this node can
//     read or write the guard, so opening it at all was the defect.
//   - LANE ON: the daemon must REFUSE TO START. Inside the lane the guard is
//     load-bearing — starting with an empty guard IS the eviction that re-opens the
//     F2 double-pay, so a corrupt store must never be started over.
//
// This is the RUNTIME gate. It drives real `silt daemon` processes, because the
// property is a boot outcome and a source gate cannot see one (the third-time rule on
// [[scar-verifies-x-must-name-the-axes]], fired 2026-09-03).
//
// ABLATION that must redden it: remove the `if *acceptReceipts {` wrapper from the
// guard-store block in cmd/silt/daemon.go. Arm 1 then dies with
// "delivery-credit guard store" and never reaches the peer line.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// corruptGuardStore writes a paidserials.log that is RECORD-ALIGNED but whose first
// record declares an impossible 0-byte serial. Alignment matters: `Open`'s realign
// only truncates a torn TAIL, so an unaligned file would be silently repaired and this
// test would assert nothing. A zero length byte is what `Load` calls ErrCorrupt.
func corruptGuardStore(t *testing.T, store string) string {
	t.Helper()
	path := filepath.Join(store, "paidserials.log")
	const recSize = 1 + 32 + 32 + 8 // guardstore's fixed record width
	if err := os.WriteFile(path, make([]byte, recSize), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCorruptGuardStoreStopsOnlyTheDaemonThatUsesIt(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	// --- ARM 1: NO delivery lane. The node must come all the way up.
	t.Run("lane-off-node-boots", func(t *testing.T) {
		store := t.TempDir()
		path := corruptGuardStore(t, store)

		a := startDaemon(t, "no-lane-corrupt-guard",
			"-listen", "127.0.0.1:0", "-store", store,
			"-serve-registry", "127.0.0.1:0", "-validator",
			"-objective=false", "-min-rep", "100", "-quorum", "1",
			"-bond", "8M", "-min-bond-floor", "0",
			"-capacity", "1G", "-mdns=false", "-id-seed", "4805")

		// All three lines are printed DOWNSTREAM of the guard-store block, which is
		// what makes them evidence rather than decoration: under the regression
		// runDaemon returns before any of them.
		a.waitFor(t, reRegistry, 30*time.Second)  // chain participation
		a.waitFor(t, rePeer, 30*time.Second)      // serving / routing
		a.waitFor(t, reBootstrap, 30*time.Second) // the swarm table

		if m := a.out.find(regexp.MustCompile(`delivery-credit guard store`)); m != nil {
			t.Fatalf("a node with NO delivery lane reported on the paid-serial guard: %q. "+
				"It cannot bank a receipt, so it can never read or write that file — "+
				"opening it is pure blast radius (PE H-3)", m[0])
		}
		// And it did not "repair" a file it has no business touching.
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("the lane-less daemon removed a guard store it does not use: %v", err)
		}
		if len(after) != 1+32+32+8 {
			t.Fatalf("the lane-less daemon REWROTE the guard store (%d bytes)", len(after))
		}
	})

	// --- ARM 2: the lane IS on. The refuse-to-start must survive, or the F2 close
	// becomes optional. Without this arm, arm 1 could be greened by deleting the
	// guard-store wiring entirely.
	t.Run("lane-on-node-refuses-to-start", func(t *testing.T) {
		store := t.TempDir()
		corruptGuardStore(t, store)

		b := startDaemon(t, "lane-corrupt-guard",
			"-listen", "127.0.0.1:0", "-store", store,
			"-serve-registry", "127.0.0.1:0", "-validator",
			"-accept-delivery-receipts", "-epoch-blocks", "8", // R2.10 / F8: a paid lane needs an epoch clock
			"-objective=false", "-min-rep", "100", "-quorum", "1",
			"-bond", "8M", "-min-bond-floor", "0",
			"-capacity", "1G", "-mdns=false", "-id-seed", "4806")

		b.waitFor(t, regexp.MustCompile(`delivery-credit guard store`), 30*time.Second)
		if m := b.out.find(rePeer); m != nil {
			t.Fatalf("a daemon that BANKS receipts started over a corrupt paid-serial "+
				"guard and became a peer: %q. Starting with an empty guard IS the restart "+
				"eviction that re-opens the F2 double-pay", m[0])
		}
	})
}
