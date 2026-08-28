package e2e

// TestAnchorStopHaltsBondedNonAnchors is the LOCAL twin of the cloud
// 5-sybil-no-capture flow (the parity gap the 2026-08-20 harness analysis named):
// in the launch phase, finality requires a strict anchor majority (#402), so a
// bonded NON-anchor cohort must not be able to advance the chain while every
// anchor is down — and the chain must resume the moment the anchors return (the
// cloud flow's "clincher"). The cloud run still owns the full C2 shape (8 sybil
// bonds banking over a warm WAN); this drill pins the anchor GATE itself on real
// daemons over real TCP, deterministically, for $0.
//
// Same drive as the cloud flow: baseline commit → stop all anchors → watch for
// quiet advance (none allowed) → restart anchors → drive a publish → committed
// and synced. Nothing is driven DURING the stop window (the registry lives on an
// anchor, exactly as on GCP) — the assertion there is the absence of fresh
// commits on the survivors, the same evidence the cloud flow greps journals for.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// resumeObserveSweeps is the resume-observation window in ChainSyncInterval
// units. Derived (#583, docs/thinking/2026-08-28-583-anchorstop-resume-window.md):
// after the anchors return and the driven publish commits, a bonded NON-anchor
// that missed the live commit round heals on its next chainSyncTick, which
// reschedules every ChainSyncInterval (core/node/chainrole.go:1527) at an
// ARBITRARY phase. A window equal to ONE interval catches ZERO sweeps in the
// worst phase — the identical zero-overlap-margin defect #549-Q3 fixed. Two
// sweeps guarantee one fires regardless of phase; the third is the measured
// ~30 s CI/local load stretch (#583: CI 76 s − local 46 s ≈ one interval). So
// 3 × 30 s = 90 s.
const resumeObserveSweeps = 3

// chainSyncInterval MIRRORS core/node/node.go ChainSyncInterval (30 s). The e2e
// binary is a separate process, so this is a mirror, not a read: if the daemon's
// ChainSyncInterval changes, re-sync this constant (the #549-Q3 residual).
const chainSyncInterval = 30 * time.Second

// resumeObserveWindow is the derived poll budget for the resume clincher.
const resumeObserveWindow = resumeObserveSweeps * chainSyncInterval

// The gate against a fourth silent #583 reroll: a window below two sweeps
// re-opens the zero-phase-margin flake. Lowering resumeObserveSweeps < 2 fails
// the whole e2e package at init, so the window can never silently regress.
func init() {
	if resumeObserveSweeps < 2 {
		panic("resumeObserveSweeps < 2 re-opens #583: a resume-observation window " +
			"≤ 1 ChainSyncInterval has zero phase margin for the non-anchor catch-up sweep")
	}
}

// committedCount counts how many "chain: committed block N" lines a daemon has
// printed — the quiet-advance detector (any fresh commit bumps it).
func committedCount(d *daemon) int {
	n := 0
	for _, ln := range strings.Split(d.out.dump(), "\n") {
		if reCommitted.MatchString(ln) {
			n++
		}
	}
	return n
}

func TestAnchorStopHaltsBondedNonAnchors(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// 3 anchors + 2 bonded non-anchors. Three anchors, not two: launch finality
	// needs a strict anchor majority AND `-quorum 2` non-proposer attestations
	// drawn from the anchor set (#402 size-set == fill-set), so A=2 leaves only
	// one counting attester and the BASELINE can never commit (the first cut of
	// this test proved that at 90s of silence).
	const N = 5
	const nAnchors = 3
	ids := make([]string, N)
	ports := make([]string, N)
	stores := make([]string, N)
	for i := 0; i < N; i++ {
		ids[i] = identity.FromSeed(int64(7300 + i)).NodeID().String()
		ports[i] = reservePort(t)
		stores[i] = t.TempDir()
	}
	anchors := strings.Join(ids[:nAnchors], ",")
	regPort := reservePort(t)

	argsFor := func(i int) []string {
		att := make([]string, 0, N-1)
		persistent := make([]string, 0, N-1)
		for j := 0; j < N; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+ports[j])
			}
		}
		args := []string{
			"-id-seed", fmt.Sprintf("%d", 7300+i),
			"-listen", ports[i], "-advertise", ports[i],
			"-store", stores[i],
			"-validator", "-objective",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
			// A bar the 2 non-anchors can never reach, so the launch phase (and
			// its anchor gate) holds for the whole test — this drill is about the
			// gate, not the shed.
			"-mature-validators", fmt.Sprintf("%d", N),
			"-anchors", anchors,
			"-attesters", strings.Join(att, ","),
			"-persistent-peers", strings.Join(persistent, ","),
			"-quorum", "2",
			"-bond-audit", "1s",
			"-capacity", "1G", "-mdns=false",
		}
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+ports[0])
		}
		return args
	}

	daemons := make([]*daemon, N)
	for i := 0; i < N; i++ {
		daemons[i] = startDaemon(t, fmt.Sprintf("val%d", i), argsFor(i)...)
	}
	regRef := daemons[0].waitFor(t, reRegistry, 30*time.Second)[1]
	peers := ""
	for i := 0; i < N; i++ {
		if i > 0 {
			peers += ","
		}
		peers += ids[i] + "@" + ports[i]
	}

	src := filepath.Join(t.TempDir(), "p.bin")
	if err := os.WriteFile(src, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	publishCommits := func(budget time.Duration) bool {
		deadline := time.Now().Add(budget)
		for time.Now().Before(deadline) {
			out, _ := runClientAllowErr(t, "swarm", "add", src,
				"-peers", peers, "-registry", regRef, "-token-quorum", "2",
				"-chunk-size", "65536", "-mode", "convergent")
			if strings.Contains(out, "silt:v1:") {
				return true
			}
			time.Sleep(3 * time.Second)
		}
		return false
	}

	// Baseline: the full network commits (the anchor gate is satisfiable).
	if !publishCommits(120 * time.Second) {
		t.Fatalf("baseline publish never committed on the full network:\n%s", daemons[0].out.dump())
	}

	// STOP every anchor. From here the strict anchor majority (2 of 3) is
	// unreachable, so nothing may finalize — however bonded the survivors are.
	naBefore := make([]int, N)
	for i := nAnchors; i < N; i++ {
		naBefore[i] = committedCount(daemons[i])
	}
	for i := 0; i < nAnchors; i++ {
		daemons[i].cmd.Process.Kill()
	}
	time.Sleep(15 * time.Second) // several proposal/audit rounds of opportunity

	for i := nAnchors; i < N; i++ {
		if c := committedCount(daemons[i]); c > naBefore[i] {
			t.Fatalf("bonded non-anchor val%d committed fresh blocks with ALL anchors down (%d→%d) — the launch anchor gate did not hold",
				i, naBefore[i], c)
		}
	}

	// CLINCHER: anchors return (same identity, port, store — a restart, not a new
	// node) and the chain resumes: a driven publish commits and the non-anchors
	// see it. Same-history resume, not a fresh chain: the anchors reload their
	// persisted stores.
	for i := 0; i < nAnchors; i++ {
		daemons[i] = startDaemon(t, fmt.Sprintf("val%dr", i), argsFor(i)...)
	}
	daemons[0].waitFor(t, reRegistry, 30*time.Second)

	if !publishCommits(120 * time.Second) {
		t.Fatalf("chain did not resume after the anchors returned:\n--- val0r ---\n%s\n--- val%d ---\n%s",
			daemons[0].out.dump(), nAnchors, daemons[nAnchors].out.dump())
	}
	// A survivor observes the resumed chain (fresh commit lines appear). The
	// window is DERIVED from the non-anchor catch-up cadence (#583), not a
	// fixed guess: a non-anchor that missed the live round heals on its next
	// chainSyncTick (every ChainSyncInterval, arbitrary phase), so the poll must
	// span >= 2 sweeps to guarantee one fires under any phase, plus the measured
	// CI load stretch. This POLL fails fast on a genuine halt (the liveness
	// regression this test exists to catch) and does not expire on normal gossip
	// latency under load.
	deadline := time.Now().Add(resumeObserveWindow)
	for time.Now().Before(deadline) {
		if committedCount(daemons[nAnchors]) > naBefore[nAnchors] {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("val%d never observed a committed block within %s after the anchors resumed (derived catch-up window, #583):\n%s",
		nAnchors, resumeObserveWindow, daemons[nAnchors].out.dump())
}
