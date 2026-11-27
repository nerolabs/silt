package e2e

import (
	"regexp"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// TestMatureBondedZeroStartsAValidator. -mature-bonded's help says 0 means no floor, and the
// maturing field topology passes exactly that; the size parser refused "0", so every such node
// crash-looped and a whole cloud sheet aborted before grading a row. The documented opt-out must
// start a validator.
func TestMatureBondedZeroStartsAValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := identity.FromSeed(9871).NodeID().String()
	b := identity.FromSeed(9872).NodeID().String()
	d := startDaemon(t, "mature-bonded-zero",
		"-id-seed", "9871", "-listen", "127.0.0.1:0", "-store", t.TempDir(), "-mdns=false",
		"-validator", "-anchors", a+","+b, "-attesters", b, "-mature-validators", "3",
		"-mature-bonded", "0", "-mature-window", "0",
		"-bond", "8M", "-min-bond-floor", "0", "-capacity", "1G")
	d.waitFor(t, rePeer, 60*time.Second)
	if m := d.out.find(regexp.MustCompile(`-mature-bonded .*bad size`)); m != nil {
		t.Fatalf("the documented -mature-bonded 0 was refused: %s", m[0])
	}
}
