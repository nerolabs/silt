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

// TestEquivocatorSlashedOverTCP is the #184 accountability gate over the REAL WIRE,
// OBJECTIVE mode (the deployed regime) — a DEDICATED, minimal, ephemeral 4-anchor
// network whose only job is this one drill (PE ruling 2026-08-17: the equivocation
// drill is the one irreversible drill — a proven double-sign is a permanent
// eviction, F2 — so it runs on its own throwaway net, never mid-sheet where the
// eviction would leave a zero-fault-tolerance tail).
//
// Under a BFT commit floor (3-of-4) a fork can NEVER be committed onto a target, so
// the legacy commit-based placement can't drive here. The faithful route is
// slash-on-DETECTION: the crime is SIGNING two conflicting blocks at one height,
// not committing two forks. The Byzantine anchor participates honestly (its prepare
// lands on-chain), then SERVES a conflicting signed block at that height
// (-equivocate's objective path → PlaceConflictingSigned); an honest anchor that
// syncs it fetches the fork, and chain.FindEquivocations catches the same-slot
// cross-fork prepare pair and slashes — unaided, on the product's own reconcile
// path, without ever adopting the invalid (quorum-short) loser.
//
// Division of labour (the drill narrates it, per the PE's transparency condition):
// the ADVERSARY earns standing and authors both conflicting prepares (the
// un-bypassable self-incrimination); the PRODUCT detects and slashes. The harness
// only builds the range. The in-process merge gate is
// core/node/modelcheck_184_equivocation_objective_test.go.
func TestEquivocatorSlashedOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	const N = 4 // 4 anchors: a real 3-of-4 objective consensus set, one of them Byzantine
	ids := make([]string, N)
	port := make([]string, N)
	for i := 0; i < N; i++ {
		ids[i] = identity.FromSeed(int64(7300 + i)).NodeID().String()
		port[i] = reservePort(t)
	}
	anchors := strings.Join(ids, ",")
	regPort := reservePort(t)
	const byz = N - 1 // val3 is the Byzantine equivocator; val0 serves the registry + detects

	daemons := make([]*daemon, N)
	for i := 0; i < N; i++ {
		att := make([]string, 0, N-1)
		persistent := make([]string, 0, N-1)
		for j := 0; j < N; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+port[j])
			}
		}
		args := []string{
			"-id-seed", fmt.Sprintf("%d", 7300+i),
			"-listen", port[i], "-advertise", port[i],
			"-store", t.TempDir(),
			"-validator", "-objective",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
			"-mature-validators", fmt.Sprintf("%d", N),
			"-anchors", anchors,
			"-attesters", strings.Join(att, ","),
			"-persistent-peers", strings.Join(persistent, ","),
			"-quorum", "2", "-bond-audit", "2s",
			"-capacity", "1G", "-mdns=false",
		}
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+port[0])
		}
		if i == byz {
			// The objective -equivocate path: the value is just the trigger (the fork
			// is served on GetChain to whoever syncs, so peer IDs are irrelevant here).
			args = append(args, "-equivocate", "objective")
		}
		daemons[i] = startDaemon(t, fmt.Sprintf("val%d", i), args...)
	}

	regRef := daemons[0].waitFor(t, reRegistry, 30*time.Second)[1]
	peers := ""
	for i := 0; i < N; i++ {
		if i > 0 {
			peers += ","
		}
		peers += ids[i] + "@" + port[i]
	}

	// Drive commits so the Byzantine anchor's prepare lands on-chain (it attests the
	// honest blocks): retry a tokened publish until a block commits. The Byzantine
	// node then serves its conflicting fork at a height it prepared.
	src := filepath.Join(t.TempDir(), "p.bin")
	if err := os.WriteFile(src, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	committed := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := runClientAllowErr(t, "swarm", "add", src,
			"-peers", peers, "-registry", regRef, "-token-quorum", "2",
			"-chunk-size", "65536", "-mode", "convergent")
		if strings.Contains(out, "silt:v1:") && daemons[0].out.find(reCommitted) != nil {
			committed = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !committed {
		t.Fatal("no block committed — the objective net never warmed, so the equivocator has no on-chain prepare to fork")
	}

	// The adversary's OWN malicious act: it announces the double-sign once it has a
	// committed prepare to fork (PlaceConflictingSigned served the conflicting block).
	daemons[byz].waitFor(t, regexp.MustCompile(`adversary: equivocation complete \(double-signed height \d+\)`), 90*time.Second)

	// The PRODUCT's defence, unaided: an honest anchor that syncs the Byzantine node
	// catches the cross-fork double-sign and slashes it — the real reconcile-path log
	// line (#7: the daemon's own output, never a harness-echoed string).
	daemons[0].waitFor(t, regexp.MustCompile(`chain: slashed equivocator `+ids[byz]), 90*time.Second)
}
