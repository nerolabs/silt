package e2e

// #558 / Lane B8 — the REFUSE-TO-START rule, driven in a REAL PROCESS (the
// laneoff_corrupt_store pattern: a daemon-level property needs a daemon-level
// gate; the historical bug lived in cmd/silt/daemon.go, not in the library).
//
//  1. a chain.cbor the binary cannot replay ⇒ the daemon EXITS 3 before it
//     becomes a peer, the refusal names the file and the flag, and the file
//     is byte-for-byte untouched;
//  2. with -accept-chain-loss the daemon STARTS (registry, peer, bootstrap
//     lines), the accepted-loss line names the preserved copy, and that copy
//     holds the original bytes while chain.cbor is freshly written.
//
// Ablation that must redden (1): make chainstore.Recover accept unconditionally
// (the daemon then starts from genesis, the pre-B8 shape).

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func chainRefuseArgs(store string, seed string, extra ...string) []string {
	args := []string{
		"-listen", "127.0.0.1:0", "-store", store,
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", seed,
	}
	return append(args, extra...)
}

func TestDaemonRefusesToStartOnAnUnreplayableChain(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	chainPath := filepath.Join(store, "chain.cbor")
	garbage := []byte("this is not a CBOR chain\n")
	if err := os.WriteFile(chainPath, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	a := startDaemon(t, "refuse-558", chainRefuseArgs(store, "4804")...)
	line := a.waitFor(t, regexp.MustCompile(`chain replay: REFUSING TO START — .*`), 30*time.Second)
	if !strings.Contains(line[0], chainPath) || !strings.Contains(line[0], "-accept-chain-loss") {
		t.Fatalf("the refusal must name the file and the flag: %q", line[0])
	}
	err := a.cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("#558: the daemon must EXIT 3 on an unreplayable chain.cbor, got err=%v\n--- output ---\n%s", err, a.out.dump())
	}
	if m := a.out.find(rePeer); m != nil {
		t.Fatalf("the daemon became a peer over a chain it could not replay: %q", m[0])
	}
	after, rerr := os.ReadFile(chainPath)
	if rerr != nil || string(after) != string(garbage) {
		t.Fatalf("a refusal must leave chain.cbor untouched: err=%v", rerr)
	}
}

func TestDaemonAcceptedChainLossPreservesTheOriginal(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	chainPath := filepath.Join(store, "chain.cbor")
	garbage := []byte("this is not a CBOR chain\n")
	if err := os.WriteFile(chainPath, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	a := startDaemon(t, "accept-558", chainRefuseArgs(store, "4805", "-accept-chain-loss")...)
	line := a.waitFor(t, regexp.MustCompile(`chain replay: FAILED at block 0: .*-accept-chain-loss set: the original (\S+) is PRESERVED untouched as (\S+);`), 30*time.Second)
	if line[1] != chainPath || !strings.HasPrefix(line[2], chainPath+".rejected-") {
		t.Fatalf("the accepted-loss line must name the original and its preserved copy: %q", line[0])
	}
	a.waitFor(t, reRegistry, 30*time.Second)
	a.waitFor(t, rePeer, 30*time.Second)
	kept, err := os.ReadFile(line[2])
	if err != nil || string(kept) != string(garbage) {
		t.Fatalf("the preserved original must be byte-identical: err=%v", err)
	}
	// The daemon has since written a FRESH chain.cbor (genesis) — the original was moved, not overwritten.
	fresh, err := os.ReadFile(chainPath)
	if err == nil && string(fresh) == string(garbage) {
		t.Fatal("chain.cbor still holds the rejected bytes — the daemon neither preserved-by-move nor wrote a fresh store")
	}
}
