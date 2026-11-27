package e2e

import (
	"regexp"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// TestAnAnchorLaunchDerivesItsMaturityCoefficient. Launching a network takes an anchor set; it used
// to also take -mature-validators, and a validator given the anchors alone refused to start, naming
// a flag the operator then had to guess a value for. The coefficient is now derived like the other
// launch parameters: the node starts, says what it chose, and names the flag that overrides it.
func TestAnAnchorLaunchDerivesItsMaturityCoefficient(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := identity.FromSeed(9811).NodeID().String()
	b := identity.FromSeed(9812).NodeID().String()
	d := startDaemon(t, "anchor-launch",
		"-id-seed", "9811", "-listen", "127.0.0.1:0", "-store", t.TempDir(), "-mdns=false",
		"-validator", "-anchors", a+","+b, "-attesters", b, "-bond", "8M", "-min-bond-floor", "0", "-capacity", "1G")
	line := d.waitFor(t, regexp.MustCompile(`consensus: mature-validators defaulted to (\d+) .*Override with -mature-validators`), 60*time.Second)
	if line[1] != "4" {
		t.Fatalf("the derived coefficient is %s, want 4 (the smallest operator set that tolerates one fault)", line[1])
	}
	d.waitFor(t, regexp.MustCompile(`training wheels: 2 anchor\(s\), strict majority 2 required .* shed at 4 independent validators`), 30*time.Second)
	d.waitFor(t, rePeer, 60*time.Second) // it started, rather than refusing for want of the flag
}
