package e2e

// The cloud harness's EXACT posture for the R2.9 delivery lane, proven on a laptop before
// a fleet is paid for (blind PE on the cloud flow, 2026-09-07, item 5: every other proof of
// "delivery receipts: ACCEPTING" used -objective=false with an explicit -epoch-blocks; the
// derived-epoch OBJECTIVE path, the 90s window and the CLI were unproven). This boots ONE
// objective validator with the flags integration/cloudtest/topology.py puts on the boot
// validator — no explicit -epoch-blocks, so the epoch clock the paid-serial guard needs
// must be DERIVED — and drives the real `silt swarm receipt` CLI against it. On a chain
// where era-4 is dark (this one, and every real network until the R3.4 stamp raise) the
// client must be refused at the withdrawal naming the committed E->key binding, must NOT
// be told the lane is off, and the server's debug.log must carry no banked line. The flag
// literals below are the harness's; a change on either side must move both.

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPaidDeliveryLaneArmsInTheHarnessPosture(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "harness-posture",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-objective",
		"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
		"-mature-validators", "1", "-quorum", "1",
		// The cold-start scaffolding an objective validator refuses to start without. TWO
		// anchors, only one of them live here — which is the harness's actual shape, not a
		// concession to the MinObjectiveAnchors floor: topology.py declares EVERY validator
		// as an anchor (`anchors = ",".join(... for v in validators)`), and the boot
		// validator starts before any peer exists. Declaring itself as the sole anchor was
		// the simplification, and it is the posture the daemon now refuses, because at one
		// anchor bftThreshold is 0, the #402 majority is self-satisfied by the proposer and
		// finality engages at 0 — the node would commit alone with zero attestations.
		// Nothing here needs a commit: the assertions are start-up lines plus a withdrawal
		// refused for want of a committed E->key binding, which an un-committed chain gives
		// all the more surely. IDs are silt id -id-seed 4811 (this node) and 4812.
		"-anchors", "023bfdb715f3452ede2812ed3124a58230a5a3659314650c1ef94512805ffcd6,"+
			"399301aaac39f431ed526fb4f8d643ced1c3317072580741e8a8afd228481662",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4811",
		// topology.py, boot validator — verbatim:
		"-accept-delivery-receipts", "-delivery-idle-window", "24m", "-grant-capacity", "64", "-grant-per-hour", "64")
	a.waitFor(t, regexp.MustCompile(`epoch-blocks defaulted to [1-9]`), 20*time.Second)
	a.waitFor(t, regexp.MustCompile(`delivery receipts: ACCEPTING`), 20*time.Second)
	a.waitFor(t, regexp.MustCompile(`delivery settlement: p=`), 20*time.Second)
	logPath := a.waitFor(t, regexp.MustCompile(`log: info and above → (\S+)`), 20*time.Second)[1]
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]

	root := strings.Repeat("ab", 32)
	out, err := runClientAllowErr(t, "swarm", "receipt", root, "-peers", bootstrapA, "-increments", "4")
	if err == nil {
		t.Fatalf("the client BANKED a receipt on a chain with no committed E->key binding: %s", out)
	}
	if !strings.Contains(out, "committed E->key binding") {
		t.Fatalf("the refusal must name the committed binding (the harness classifies on it): %s", out)
	}
	if strings.Contains(out, "serves no demand issuer key") {
		t.Fatalf("the lane-on refusal is conflated with the lane-off sentence: %s", out)
	}
	if m := findInLog(t, logPath, regexp.MustCompile(`delivery receipt banked`)); m != nil {
		t.Fatalf("the server banked a receipt the client was refused for: %q", m[0])
	}
}
