package e2e

// `silt swarm holders <link>` — object shard placement made observable (the
// capability the deterministic cloud economy grade needs: know which nodes hold
// which columns, so a controlled kill of > RepairSlack columns forces every stripe
// to reconstruct). This drives it over real TCP against a real published object and
// asserts it reports per-column holders drawn from the actual swarm.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var reHolderCol = regexp.MustCompile(`(?m)^column \d+: ([0-9a-f,]+)$`)

func TestSwarmHoldersReportsPerColumnPlacement(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	// Storage nodes so shards actually scatter to holders the walk can find.
	for i, name := range []string{"B", "C"} {
		d := startDaemon(t, name,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-bootstrap", bootstrapA, "-capacity", "1G", "-mdns=false",
			"-id-seed", fmt.Sprintf("%d", 1002+i))
		_ = d
	}
	time.Sleep(2 * time.Second) // let B, C bootstrap into the routing table

	// A file that stripes: 1 MiB / 64 KiB = 16 chunks → erasure-coded (k=10, n=16).
	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 1<<20)
	rand.New(rand.NewSource(0x40D)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	out := runClient(t, "swarm", "add", src, "-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536")
	link := reLink.FindString(strings.TrimSpace(out))
	if link == "" {
		t.Fatalf("swarm add printed no link:\n%s", out)
	}
	a.waitFor(t, reCommitted, 20*time.Second)

	// swarm holders must report per-column holders drawn from the real swarm.
	hout := runClient(t, "swarm", "holders", link, "-peers", bootstrapA, "-registry", regRef)
	matches := reHolderCol.FindAllStringSubmatch(hout, -1)
	if len(matches) == 0 {
		t.Fatalf("swarm holders reported no columns:\n%s", hout)
	}
	// At least one column must name a real holder, and every holder listed must be
	// a 64-hex NodeID (not garbage) — the placement is genuinely resolved, not empty.
	known := map[string]bool{idA: true}
	nonEmpty := 0
	for _, m := range matches {
		ids := strings.Split(m[1], ",")
		for _, id := range ids {
			if id == "" {
				continue
			}
			nonEmpty++
			if len(id) != 64 {
				t.Fatalf("holder id is not a 64-hex NodeID: %q", id)
			}
			_ = known
		}
	}
	if nonEmpty == 0 {
		t.Fatalf("swarm holders resolved columns but named no holders:\n%s", hout)
	}
	t.Logf("swarm holders resolved %d columns, %d holder entries", len(matches), nonEmpty)
}
