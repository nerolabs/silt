package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// TestFloorBoxAcceptsTheNetworksNextBlockOverTCP: a floor box — no replica, no registry, no
// state tree — anchored on an operator checkpoint judges the block the network commits above it
// against witnesses fetched over TCP from the validators, and reports ACCEPTED: the verdict a
// tree-holding node reaches for the same block. A box started without a checkpoint refuses to
// start, because an accepting box never anchors on a provider's reported head.
func TestFloorBoxAcceptsTheNetworksNextBlockOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	const nVal = 4
	ids := make([]string, nVal)
	ports := make([]string, nVal)
	stores := make([]string, nVal)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9700 + i)).NodeID().String()
		ports[i] = reservePort(t)
		stores[i] = t.TempDir()
	}
	regPort := reservePort(t)
	consensus := []string{"-objective", "-min-bond", "1M", "-min-bond-floor", "0",
		"-mature-validators", "99", "-anchors", strings.Join(ids, ","), "-quorum", "2"}
	var v0 *daemon
	var issuers []string
	for i := 0; i < nVal; i++ {
		var att, persistent []string
		for j := 0; j < nVal; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+ports[j])
			}
		}
		args := append([]string{"-id-seed", fmt.Sprint(9700 + i), "-listen", ports[i], "-advertise", ports[i],
			"-store", stores[i], "-validator", "-bond", "8M",
			"-attesters", strings.Join(att, ","), "-persistent-peers", strings.Join(persistent, ","),
			"-capacity", "1G", "-mdns=false"}, consensus...)
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+ports[0])
		}
		d := startDaemon(t, fmt.Sprintf("V%d", i), args...)
		d.waitFor(t, rePeer, 20*time.Second)
		if i == 0 {
			v0 = d
		}
		issuers = append(issuers, ids[i]+"@"+ports[i])
	}
	regRef := v0.waitFor(t, reRegistry, 30*time.Second)[1]
	publish := func(name string) {
		src := filepath.Join(t.TempDir(), name)
		os.WriteFile(src, []byte(name), 0o644)
		for deadline := time.Now().Add(90 * time.Second); ; {
			if _, err := runClientAllowErr(t, "swarm", "add", src, "-peers", strings.Join(issuers, ","),
				"-registry", regRef, "-token-quorum", "2"); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("publish %s never committed", name)
			}
			time.Sleep(2 * time.Second)
		}
	}
	publish("warm") // the anchors have committed their bonds and the chain is v5

	// The CHECKPOINT: the head of a validator's own store, as an operator would take it.
	status := runClient(t, "chain-status", "-store", stores[1])
	hm := regexp.MustCompile(`head height[:\s]+(\d+)`).FindStringSubmatch(status)
	xm := regexp.MustCompile(`head hash[:\s]+([0-9a-f]{64})`).FindStringSubmatch(status)
	if hm == nil || xm == nil {
		t.Fatalf("chain-status gave no head height and hash:\n%s", status)
	}
	checkpoint := hm[1] + ":" + xm[1]

	boxArgs := func(seed string, extra ...string) []string {
		return append(append([]string{"-id-seed", seed, "-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-floor-box", "-witness-from", strings.Join(ids, ","), "-floor-box-interval", "2s",
			"-attesters", strings.Join(ids, ","), "-bootstrap", ids[0] + "@" + ports[0],
			"-capacity", "1G", "-mdns=false"}, consensus...), extra...)
	}

	// The anchor rule: no checkpoint, no box.
	unanchored := startDaemon(t, "box-unanchored", boxArgs("9790")...)
	unanchored.waitFor(t, regexp.MustCompile(`-floor-box needs -ws-checkpoint`), 20*time.Second)

	box := startDaemon(t, "box", boxArgs("9791", "-ws-checkpoint", checkpoint)...)
	box.waitFor(t, regexp.MustCompile(`anchored on the operator's checkpoint `+hm[1]+`:`), 30*time.Second)
	publish("above-the-checkpoint") // the block the box judges
	var above int
	fmt.Sscanf(hm[1], "%d", &above)
	above++
	box.waitFor(t, regexp.MustCompile(fmt.Sprintf(`floor-box: verdict=ACCEPTED height=%d `, above)), 120*time.Second)
	if m := box.out.find(regexp.MustCompile(`floor-box: verdict=(REFUSED|STALL) height=` + fmt.Sprint(above) + ` .*`)); m != nil {
		t.Fatalf("the box also reported %q for the height it accepted", m[0])
	}
}
