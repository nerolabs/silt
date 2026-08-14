package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// Objective consensus bootstraps from a FULLY-SIMULTANEOUS cold start (positive
// e2e). Four objective validators all attesting each other, -quorum 2,
// Byzantine-quorum default ON, -mature-validators 4 — deterministic ids on RESERVED
// fixed ports, so every argv (-anchors/-attesters/-advertise/-bootstrap) is known
// before any process starts (a true simultaneous cold start, no start-ordering).
// The anchors (training wheels) co-sign the genesis block, so it commits within
// seconds — even at the GCP `-bond-audit=30s` (REPRO_BOND_AUDIT).
//
// This was written to reproduce #286 (on GCP this same shape did NOT commit — chain
// stuck at height 0). It does NOT reproduce locally: the consensus code + exact GCP
// params commit fine over real TCP in <2s. So #286 is GCP-ENVIRONMENT-specific (the
// one GCP run that warmed was all-SPOT; the failing runs used on-demand core), NOT a
// consensus-code bug. Kept as the positive regression guard — if this ever fails,
// the objective cold-start bootstrap has genuinely regressed.
func TestObjectiveColdStartCommitsGenesis(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	const N = 4
	ids := make([]string, N)
	port := make([]string, N)
	for i := 0; i < N; i++ {
		ids[i] = identity.FromSeed(int64(7100 + i)).NodeID().String()
		port[i] = reservePort(t)
	}
	anchors := strings.Join(ids, ",")
	regPort := reservePort(t)

	// bond-audit interval + publish window are env-tunable so we can probe the GCP
	// value (30s) vs a fast local default without editing.
	bondAudit := os.Getenv("REPRO_BOND_AUDIT")
	if bondAudit == "" {
		bondAudit = "2s"
	}
	windowS := 90
	if w := os.Getenv("REPRO_WINDOW_S"); w != "" {
		fmt.Sscanf(w, "%d", &windowS)
	}

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
			"-id-seed", fmt.Sprintf("%d", 7100+i),
			"-listen", port[i], "-advertise", port[i],
			"-store", t.TempDir(),
			"-validator", "-objective",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
			"-mature-validators", fmt.Sprintf("%d", N),
			"-anchors", anchors,
			"-attesters", strings.Join(att, ","),
			// Configure the validator set as a static, never-evicted persistent-peer tier
			// (network-durability §8): consensus must not depend on discovery for the
			// addresses it needs to gather quorum. Required now that #402 raised the launch
			// bar to a strict anchor majority (3 of 4) — a half-converged discovery mesh
			// reaches 2 but not 3 under load, so a pure-`-bootstrap` cold start flakes. This
			// is the shipped #286-Layer-2 fix, so the regression guard should exercise it.
			"-persistent-peers", strings.Join(persistent, ","),
			"-quorum", "2",
			"-bond-audit", bondAudit,
			"-capacity", "1G", "-mdns=false",
		}
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+port[0])
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

	src := filepath.Join(t.TempDir(), "p.bin")
	if err := os.WriteFile(src, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	// Retry a tokened publish (quorum 2, exactly the GCP warmup) while standing
	// accrues. Success = a real committed block on the proposer.
	committed := false
	deadline := time.Now().Add(time.Duration(windowS) * time.Second)
	for time.Now().Before(deadline) {
		out, _ := runClientAllowErr(t, "swarm", "add", src,
			"-peers", peers, "-registry", regRef, "-token-quorum", "2",
			"-chunk-size", "65536", "-mode", "convergent")
		if strings.Contains(out, "silt:v1:") && daemons[0].out.find(reCommitted) != nil {
			committed = true
			break
		}
		time.Sleep(3 * time.Second)
	}

	if !committed {
		for _, d := range daemons {
			t.Logf("--- %s tail ---\n%s", d.name, d.out.dump())
		}
		t.Fatalf("objective 4-validator simultaneous cold start (quorum 2, mature 4, Byzantine on, bond-audit %s) did NOT commit a block within %ds — the anchor training-wheels bootstrap has regressed (cf. #286)", bondAudit, windowS)
	}
}

// #286, hypothesis 2: the full cloudtest topology has an EXTRA objective validator
// beyond the 4 anchors — the `adversary` node (honest by default): `-validator
// -objective`, referencing the SAME `-anchors` set and attesting all 4 core
// validators, but NOT itself an anchor and NOT in the 4 core validators' attester
// lists. SMOKE (no adversary) warmed; the full topology (with it) mostly did not.
// Does a 5th bonded non-anchor validator joining the cold start prevent the 4
// anchors from committing genesis? This reproduces that shape locally.
func TestObjectiveColdStartWithSatelliteValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	const CORE = 4
	ids := make([]string, CORE+1) // +1 satellite (the adversary-style node)
	port := make([]string, CORE+1)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(7200 + i)).NodeID().String()
		port[i] = reservePort(t)
	}
	anchors := strings.Join(ids[:CORE], ",") // ONLY the 4 core validators are anchors
	regPort := reservePort(t)

	launch := func(i int, extra ...string) *daemon {
		att := make([]string, 0, CORE)
		for j := 0; j < CORE; j++ { // core validators attest the OTHER core validators
			if j != i {
				att = append(att, ids[j])
			}
		}
		if i >= CORE { // the satellite attests all 4 core validators (none is "self")
			att = append(att[:0], ids[:CORE]...)
		}
		// Configure the WHOLE validator set as a static, never-evicted persistent-peer
		// tier (network-durability §8 / the cloudtest precedent, #286 Layer 2): consensus
		// must NOT depend on discovery for the addresses it needs to gather the anchor
		// quorum. Without it the mesh converges only partially under CPU load, and the
		// #402 strict anchor majority (3 of 4) then can't be reached — a CONFOUND (mesh
		// convergence) masquerading as the variable this test isolates (does the SATELLITE
		// block commit?). Pre-#402 the lower 2-quorum was reachable on a half-formed mesh,
		// so the confound stayed hidden; the higher bar surfaced it. Configuring the set is
		// the settled answer, and it makes the test assert its actual question.
		persistent := make([]string, 0, CORE)
		for j := range ids {
			if j != i {
				persistent = append(persistent, ids[j]+"@"+port[j])
			}
		}
		args := []string{
			"-id-seed", fmt.Sprintf("%d", 7200+i),
			"-listen", port[i], "-advertise", port[i], "-store", t.TempDir(),
			"-validator", "-objective",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
			"-mature-validators", fmt.Sprintf("%d", CORE),
			"-anchors", anchors, "-attesters", strings.Join(att, ","),
			"-persistent-peers", strings.Join(persistent, ","),
			"-quorum", "2", "-bond-audit", "2s", "-capacity", "1G", "-mdns=false",
		}
		args = append(args, extra...)
		return startDaemon(t, fmt.Sprintf("v%d", i), args...)
	}

	all := make([]*daemon, CORE+1)
	all[0] = launch(0, "-serve-registry", regPort)
	for i := 1; i <= CORE; i++ { // core joiners + the satellite (i==CORE) all bootstrap to v0
		all[i] = launch(i, "-bootstrap", ids[0]+"@"+port[0])
	}

	regRef := all[0].waitFor(t, reRegistry, 30*time.Second)[1]
	peers := ""
	for i := 0; i < CORE; i++ {
		if i > 0 {
			peers += ","
		}
		peers += ids[i] + "@" + port[i]
	}
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
		if strings.Contains(out, "silt:v1:") && all[0].out.find(reCommitted) != nil {
			committed = true
			break
		}
		time.Sleep(3 * time.Second)
	}
	if !committed {
		for _, d := range all {
			t.Logf("--- %s tail ---\n%s", d.name, d.out.dump())
		}
		t.Fatal("#286 REPRODUCED: a 5th bonded non-anchor (adversary-style) validator prevented the 4 anchors from committing genesis")
	}
}
