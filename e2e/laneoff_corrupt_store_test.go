package e2e

// R0.4b C3 — the LANE OFF degrade path, driven in a REAL PROCESS.
//
// WHY THIS FILE EXISTS. The F7 close (a corrupt demandkeys.cbor must stop the receipt
// lane, not the validator) shipped with a SOURCE-TEXT gate only:
// cmd/silt/rt_r04b_c3_laneoff_test.go greps daemon.go for "LANE OFF" and for the absence
// of one forbidden return. The Tester demonstrated that gate has no teeth — F7 was
// reintroduced verbatim, with the gate GREEN and `go vet` clean, by adding a DIFFERENT
// early return above the switch. That fired the third-time rule on
// [[scar-verifies-x-must-name-the-axes]]: a gate's failure text must not promise a
// runtime property it cannot measure.
//
// This is the runtime half. It writes a corrupt demand key store, starts a real
// `silt daemon` over real TCP, and asserts the four things the property actually means:
//
//  1. the PROCESS SURVIVES — it reaches peer announcement and commits blocks;
//  2. the LANE IS OFF — "delivery receipts: ACCEPTING" never appears;
//  3. the exact operator line is printed, with the store path and the restore
//     instruction (an announced log line is an observable contract, S5);
//  4. the FILE IS UNCHANGED, byte for byte — regenerating over already-committed
//     fingerprints is the unrecoverable F6 failure.
//
// Ablation that must redden it: restore the F7 death in cmd/silt/daemon.go by returning
// the store error out of runDaemon (in any spelling). The daemon then exits before the
// peer line and step 1 fails.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDaemonSurvivesACorruptDemandKeyStore(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "issuer"), 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(store, "issuer", "demandkeys.cbor")
	// Not CBOR. Load returns "corrupt demand key store", which is the hard error the
	// store is REQUIRED to return (adapters/diskissuer TestRTC3_CorruptStoreErrorsAndIsNeverRewritten).
	corrupt := []byte("this is not a CBOR demand key band\n")
	if err := os.WriteFile(keyPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	a := startDaemon(t, "corrupt-demand-keys",
		"-listen", "127.0.0.1:0", "-store", store,
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-accept-delivery-receipts", "-epoch-blocks", "8", // R2.10 / F8: a paid lane needs an epoch clock
		"-grant-capacity", "256", "-grant-per-hour", "256", // R2.12 / G-R212-1: and a configured faucet
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4803")

	// (3) the exact operator contract, both halves.
	a.waitFor(t, regexp.MustCompile(
		`delivery receipts: LANE OFF — the demand issuer key store is unreadable: .*corrupt demand key store`),
		30*time.Second)
	restore := a.waitFor(t, regexp.MustCompile(
		`delivery receipts: (\S+) was NOT modified; restore it from backup and restart to re-enable the lane\. Chain, storage and serving continue\.`),
		30*time.Second)
	if restore[1] != keyPath {
		t.Fatalf("the operator line names %q, not the store it refused to read (%q) — an operator "+
			"cannot restore a file the daemon will not name", restore[1], keyPath)
	}

	// (1) the process survives and keeps doing its real job. All three lines below are
	// printed DOWNSTREAM of the demand-lane wiring block — that is what makes them
	// evidence rather than decoration: under the F7 regression runDaemon returns before
	// any of them, so the daemon never becomes a routing peer, never arms the
	// chain-backed registry, and never bootstraps.
	a.waitFor(t, reRegistry, 30*time.Second)  // chain participation
	a.waitFor(t, rePeer, 30*time.Second)      // serving / routing
	a.waitFor(t, reBootstrap, 30*time.Second) // the swarm table
	// Still alive after it has settled, not merely alive long enough to print.
	time.Sleep(2 * time.Second)
	if err := a.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("BREAK F7 REOPENED: the daemon is not running after a corrupt demand key "+
			"store (%v). One bad byte in an OPTIONAL receipt lane's key file took down chain "+
			"participation, storage and serving.\n--- output ---\n%s", err, a.out.dump())
	}

	// (2) the lane really is off — not merely announced off.
	if m := a.out.find(regexp.MustCompile(`delivery receipts: ACCEPTING`)); m != nil {
		t.Fatalf("the daemon ARMED the receipt lane over an unreadable key store: %q", m[0])
	}

	// (4) the file is exactly as the operator left it.
	after, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("the daemon removed the corrupt store: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("the daemon REWROTE the corrupt demand key store (%d bytes -> %d bytes). "+
			"Quietly minting new keys over already-committed fingerprints is unrecoverable: "+
			"the chain is append-only, so the regenerated key can never be registered.",
			len(corrupt), len(after))
	}
	if strings.Contains(a.out.dump(), "delivery receipts: demand key rotation") {
		t.Fatal("a rotation ran on a lane that is supposed to be OFF")
	}
}
