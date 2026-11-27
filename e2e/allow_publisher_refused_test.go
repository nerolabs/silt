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

// TestAnExplicitPublisherLinkIsRefusedNotSilentlyDropped. A publisher who passes -allow-publisher
// asks to record their node as the file's publisher. A default (untrusted) chain refuses that link
// — refuse-to-surveil — and the publisher must see the refusal. Since publish tokens became the
// default the client had been acquiring one anyway, which silently dropped the request and
// published unlinkably: safe, but it hid the chain's rule from the person who asked to break it,
// and the field sheet's priv-unlinkability row went red on it.
func TestAnExplicitPublisherLinkIsRefusedNotSilentlyDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// The objective, token-requiring network a newcomer runs (examples/try-a-local-network.sh):
	// three launch anchors, so the token quorum derives to 2 and the client acquires a token by
	// default — the condition under which the request used to be dropped.
	ids := []string{identity.FromSeed(9851).NodeID().String(), identity.FromSeed(9852).NodeID().String(), identity.FromSeed(9853).NodeID().String()}
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
		return append([]string{"-id-seed", fmt.Sprint(9851 + i), "-listen", "127.0.0.1:0", "-store", t.TempDir(), "-mdns=false",
			"-validator", "-anchors", anchors, "-attesters", others(i), "-mature-validators", "3", "-quorum", "2",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M", "-capacity", "1G"}, extra...)
	}
	a := startDaemon(t, "pub-A", val(0, "-serve-registry", "127.0.0.1:0")...)
	peer := a.waitFor(t, rePeer, 30*time.Second)
	bootA := peer[1] + "@" + peer[2]
	reg := a.waitFor(t, reRegistry, 30*time.Second)[1]
	for i := 1; i < 3; i++ {
		d := startDaemon(t, fmt.Sprintf("pub-%c", 'A'+i), val(i, "-bootstrap", bootA)...)
		d.waitFor(t, rePeer, 30*time.Second)
	}

	src := filepath.Join(t.TempDir(), "f.bin")
	os.WriteFile(src, []byte(strings.Repeat("silt", 4096)), 0o644)
	// The control: the same publish WITHOUT the flag commits (once the chain is warm).
	deadline := time.Now().Add(120 * time.Second)
	for {
		out, err := runClientAllowErr(t, "swarm", "add", src, "-peers", bootA, "-registry", reg, "-mode", "convergent") // a retry re-proposes the same block
		if err == nil && regexp.MustCompile(`silt:v1:`).MatchString(out) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("CONTROL: a default publish never committed: %v\n%s", err, out)
		}
		time.Sleep(2 * time.Second)
	}
	other := filepath.Join(t.TempDir(), "g.bin")
	os.WriteFile(other, []byte(strings.Repeat("link", 4096)), 0o644)
	out, err := runClientAllowErr(t, "swarm", "add", other, "-peers", bootA, "-registry", reg, "-mode", "convergent", "-allow-publisher")
	if err == nil {
		t.Fatalf("a publish asking to record a durable publisher committed on a default chain:\n%s", out)
	}
	if !regexp.MustCompile(`(?i)durable Publisher|publisher entry|permanent linkage|publish unlinkably`).MatchString(out) {
		t.Fatalf("the publish failed, but not with the refuse-to-surveil reason:\n%s", out)
	}
}
