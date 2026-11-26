package e2e

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// TestValidatorUIPublishCommitsFromTheEphemeralClient: on an objective network, a validator's
// own UI publish is handed to the validators by the publish client's throwaway identity
// rather than by the validator (ForwardEntry; the node-level privacy assertion is
// TestForwardedEntryReachesNoValidatorUnderTheValidatorsIdentity). This drives that path over
// real TCP and requires the publish to commit and read back: the client must reach every
// validator, this one included, from the addresses it was seeded with.
func TestValidatorUIPublishCommitsFromTheEphemeralClient(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	idA := identity.FromSeed(2501).NodeID().String()
	idB := identity.FromSeed(2502).NodeID().String()
	anchors := idA + "," + idB
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-min-bond", "1M", "-mature-validators", "2", "-anchors", anchors, "-quorum", "1", "-attesters", idB,
		"-min-bond-floor", "0", "-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2501", "-ui", "127.0.0.1:0")
	addrA := a.waitFor(t, rePeer, 20*time.Second)[2]
	ui := a.waitFor(t, reUI, 20*time.Second)
	base, token := "http://"+ui[1], ui[2]
	startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-bootstrap", idA+"@"+addrA, "-validator",
		"-min-bond", "1M", "-mature-validators", "2", "-anchors", anchors, "-quorum", "1", "-attesters", idA,
		"-min-bond-floor", "0", "-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2502").waitFor(t, reBootstrap, 20*time.Second)

	payload := make([]byte, 192<<10)
	rand.New(rand.NewSource(0x2501)).Read(payload)
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for {
		status, body, err := uiPublish(base, token, "own.bin", payload)
		last = body
		if err == nil && status == 200 && strings.Contains(body, "silt:v1:") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the validator's own UI publish never committed: status=%d err=%v body=%s", status, err, last)
		}
		time.Sleep(2 * time.Second)
	}
}
