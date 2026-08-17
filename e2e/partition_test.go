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

// TestPartitionHealsToHeavierForkOverTCP is the #184 partition→heal case over the
// REAL WIRE under the ratified BFT model (era-2 rounds+locking): a validator SEVERED
// into a sub-quorum MINORITY cannot commit, stalls behind the > ⅔ majority that keeps
// committing, and on heal CATCHES UP to the majority's heavier chain — consensus
// reconverges on one history over real TCP.
//
// This REPLACES the pre-BFT "each side commits its own fork then reorgs" test (PE
// ruling 2026-08-17 / the note the old skip left): under a 3-of-4 commit floor a
// minority committing a conflicting fork is an I1 violation, so there is no droppable
// reorg — the minority stalls and does a forward catch-up (dropped=0). The ABSENCE of
// a reorg line is the safety property, not a gap; the success signal is the stalled
// minority reaching the majority head (height + hash).
//
// Topology (objective, 4 anchors, quorum 2 → ByzantineQuorum raises to 3-of-4):
//   - Majority = {A, B, D}: A serves a registry; publishes drive it to commit a
//     heavier chain while C is away.
//   - Minority = {C}: severed from A, B, D (-block-peers, both directions). A 1-of-4
//     island cannot reach the strict anchor majority (3), so it STALLS.
//   - Heal: restart C without the block; it reconnects and catches up to the
//     majority's head. Assert its head advances to match A's (height + hash).
func TestPartitionHealsToHeavierForkOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	const N = 4
	const minority = 2 // val2 is severed into the minority; A(0) serves the registry
	ids := make([]string, N)
	port := make([]string, N)
	for i := 0; i < N; i++ {
		ids[i] = identity.FromSeed(int64(7200 + i)).NodeID().String()
		port[i] = reservePort(t)
	}
	anchors := strings.Join(ids, ",")
	regPort := reservePort(t)
	storeC := t.TempDir() // the minority's persistent store — reused across its heal-restart

	commonArgs := func(i int, store string) []string {
		att := make([]string, 0, N-1)
		persistent := make([]string, 0, N-1)
		for j := 0; j < N; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+port[j])
			}
		}
		args := []string{
			"-id-seed", fmt.Sprintf("%d", 7200+i),
			"-listen", port[i], "-advertise", port[i],
			"-store", store,
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
		return args
	}

	daemons := make([]*daemon, N)
	for i := 0; i < N; i++ {
		store := t.TempDir()
		if i == minority {
			store = storeC
		}
		args := commonArgs(i, store)
		if i == minority {
			// SEVER the minority from the whole majority, both directions — a 1-of-4
			// island that cannot reach the strict anchor majority (3), so it stalls.
			others := []string{ids[0], ids[1], ids[3]}
			args = append(args, "-block-peers", strings.Join(others, ","))
		}
		daemons[i] = startDaemon(t, fmt.Sprintf("val%d", i), args...)
	}

	regRef := daemons[0].waitFor(t, reRegistry, 30*time.Second)[1]
	majPeers := ids[0] + "@" + port[0] + "," + ids[1] + "@" + port[1] + "," + ids[3] + "@" + port[3]

	// Drive the majority to commit a heavier chain while the minority is severed.
	src := filepath.Join(t.TempDir(), "p.bin")
	if err := os.WriteFile(src, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	reCommitN := regexp.MustCompile(`chain: committed block (\d+)`)
	committed := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := runClientAllowErr(t, "swarm", "add", src,
			"-peers", majPeers, "-registry", regRef, "-token-quorum", "2",
			"-chunk-size", "65536", "-mode", "convergent")
		if strings.Contains(out, "silt:v1:") && daemons[0].out.find(reCommitN) != nil {
			committed = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !committed {
		t.Fatal("the majority never committed a heavier chain — nothing for the minority to catch up to")
	}
	// The minority must NOT have committed (it is a sub-quorum island): its store shows
	// no committed block beyond genesis. (Anti-vacuity: proof it genuinely stalled.)
	if daemons[minority].out.find(reCommitN) != nil {
		t.Fatal("the severed minority committed a block — the sever did not isolate it below the 3-of-4 threshold (I1 would be violated)")
	}
	t.Logf("majority committed a heavier chain; the severed minority stalled at genesis")

	// HEAL: restart the minority WITHOUT the partition; it reconnects and catches up.
	daemons[minority].cmd.Process.Kill()
	daemons[minority].cmd.Wait()
	healed := startDaemon(t, "val2(healed)", commonArgs(minority, storeC)...)
	healed.waitFor(t, reBootstrap, 20*time.Second)

	// The minority catches up to the majority's heavier chain — a forward sync, NOT a
	// reorg (a BFT minority never committed a conflicting fork to drop). Assert it
	// commits/adopts a block above genesis after healing.
	healed.waitFor(t, reCommitN, 60*time.Second)
	t.Logf("the healed minority caught up to the majority's heavier chain over TCP — BFT partition→heal reconverged (catch-up, not reorg)")
}
