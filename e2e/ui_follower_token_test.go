package e2e

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// TestAFollowerNodesUIPublishCarriesAToken. A non-validator -economy node follows the chain as a
// non-voting replica, and only a validator sets the token requirement on its chain, so the
// follower's replica reports quorum 0. The web UI trusted that 0 and published with no token,
// which the validators refused ("entry has no publish token (required)") — found by the cloud
// sheet's 11-unlinkability row whenever ECONOMY=1 made store-1 a follower.
var regexpFollowing = regexp.MustCompile(`chain: FOLLOWING`)

func TestAFollowerNodesUIPublishCarriesAToken(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	ids := []string{identity.FromSeed(9881).NodeID().String(), identity.FromSeed(9882).NodeID().String(), identity.FromSeed(9883).NodeID().String()}
	anchors := strings.Join(ids, ",")
	others := func(i int) string {
		var o []string
		for j, id := range ids {
			if j != i {
				o = append(o, id)
			}
		}
		return strings.Join(o, ",")
	}
	val := func(i int, extra ...string) []string {
		return append([]string{"-id-seed", fmt.Sprint(9881 + i), "-listen", "127.0.0.1:0", "-store", t.TempDir(), "-mdns=false",
			"-validator", "-anchors", anchors, "-attesters", others(i), "-mature-validators", "3", "-quorum", "2",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M", "-capacity", "1G"}, extra...)
	}
	a := startDaemon(t, "fol-A", val(0, "-serve-registry", "127.0.0.1:0")...)
	peer := a.waitFor(t, rePeer, 30*time.Second)
	bootA := peer[1] + "@" + peer[2]
	reg := a.waitFor(t, reRegistry, 30*time.Second)[1]
	for i := 1; i < 3; i++ {
		d := startDaemon(t, fmt.Sprintf("fol-%c", 'A'+i), val(i, "-bootstrap", bootA)...)
		d.waitFor(t, rePeer, 30*time.Second)
	}
	f := startDaemon(t, "follower",
		"-id-seed", "9889", "-listen", "127.0.0.1:0", "-store", t.TempDir(), "-mdns=false",
		"-anchors", anchors, "-economy", "-bootstrap", bootA, "-registry", reg,
		"-capacity", "1G", "-ui", "127.0.0.1:0")
	f.waitFor(t, regexpFollowing, 60*time.Second)
	ui := f.waitFor(t, reUI, 20*time.Second)
	base, token := "http://"+ui[1], ui[2]

	payload := []byte(strings.Repeat("follower", 4096))
	var last string
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		status, body, err := uiPublish(base, token, "f.bin", payload)
		if err == nil && status == http.StatusOK && strings.Contains(body, "silt:v1:") {
			return
		}
		last = fmt.Sprintf("status=%d err=%v body=%s", status, err, body)
		if strings.Contains(body, "no publish token") {
			t.Fatalf("the follower's UI published with no token: %s", last)
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("the follower's UI publish never committed: %s\n--- follower ---\n%s", last, f.out.dump())
}
