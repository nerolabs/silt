package e2e

// The daemon must refuse a configuration that would run any height on a block
// format older than the witnessable one.
//
// Those formats commit the seating map in the state root while reading the seating
// from a block's own attestations — signatures over the hash that covers that root.
// Two consequences follow, and both are fatal to a launched network. No block can
// seat a validator the chain has not already seen, so the validator set cannot grow,
// the maturity coefficient stays where the launch set left it, and the anchors never
// shed their bond-free eligibility. And a history rewritten to carry a different
// seating hashes identically to the real one, which the finality gate, fork-choice
// and the weak-subjectivity checkpoint all compare by hash and therefore cannot
// tell apart.
//
// Activation at height 1 is the shipped default, so this refusal only fires for an
// operator who opened the window deliberately. There is no configuration in which
// doing so is correct, which is why it is refused at start-up rather than warned
// about.

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func eraWindowArgs(store, seed string, extra ...string) []string {
	args := []string{
		"-listen", "127.0.0.1:0", "-store", store,
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-bond", "8M", "-min-bond-floor", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", seed,
	}
	return append(args, extra...)
}

func TestDaemonRefusesAPreWitnessableEraWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	for _, tc := range []struct{ name, flag string }{
		{"era-4 activation deferred", "-era4-activation-height=8"},
		{"era-3 activation deferred", "-era3-activation-height=8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := startDaemon(t, "erawindow", eraWindowArgs(t.TempDir(), "4831", tc.flag)...)
			line := a.waitFor(t, regexp.MustCompile(`(?i)(era[34]-activation-height).*`), 30*time.Second)
			if !strings.Contains(line[0], "activation-height") {
				t.Fatalf("the refusal must name the flag it is refusing: %q", line[0])
			}
			err := a.cmd.Wait()
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatalf("the daemon must EXIT rather than run on a pre-witnessable era window; got err=%v\n--- output ---\n%s",
					err, a.out.dump())
			}
			if m := a.out.find(rePeer); m != nil {
				t.Fatalf("the daemon became a peer on a configuration that opens a pre-witnessable window: %q", m[0])
			}
		})
	}
}

// TestDaemonStartsOnTheShippedEraDefault is the positive control: the same daemon,
// with no era flag at all, must reach serving. Without it the refusal above could
// pass on a daemon that refuses every configuration put in front of it.
func TestDaemonStartsOnTheShippedEraDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "eradefault", eraWindowArgs(t.TempDir(), "4832")...)
	a.waitFor(t, reRegistry, 30*time.Second)
	a.waitFor(t, rePeer, 30*time.Second)
}
