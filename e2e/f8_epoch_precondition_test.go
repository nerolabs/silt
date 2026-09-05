package e2e

// R2.10 / F8 — G-F8-3, the RUNTIME half (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §5 R-F8-DISABLED, §6 G-F8-3: "With effective EpochBlocks == 0, the daemon refuses
// to start with either paid-lane flag, naming both the flag and -epoch-blocks."
//
// Positive arms: the legacy receipt-banker fixture (e2e_test.go
// TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding's argument list — an
// -objective=false posture, so effectiveEpochBlocks returns the explicit 0) minus
// any -epoch-blocks, once per paid-lane flag, must print the refusal, EXIT non-zero,
// and never become a peer. Controls: the same fixture with -epoch-blocks 8 reaches
// the lane's ACCEPTING line.
//
// -short SKIPS this test (it spawns OS processes); the -short run reports no verdict
// on it (scar-short-suite-skips-e2e). The brick ablation lives at the unit tier:
// core/credit TestSerialGuard_SetIsBounded's ReasonGuardFull assertion.
//
// RED on main: the daemon starts and announces ACCEPTING with EpochBlocks == 0.

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var reRefuseLine = regexp.MustCompile(`.*refusing to start.*`)

func TestF8_PaidLanesRefuseToStartWithoutAnEpochClock(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	base := []string{
		"-listen", "127.0.0.1:0",
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false",
		// R2.12 (G-R212-1): a priced lane refuses to start with the faucet unconfigured, so
		// every paid-lane launch carries a rate; the F8 refusal under test is the epoch one.
		"-grant-capacity", "256", "-grant-per-hour", "256",
	}
	lanes := []struct {
		name, flag, seed, controlSeed string
		accepting                     *regexp.Regexp
	}{
		{"delivery-receipts", "-accept-delivery-receipts", "4810", "4811", regexp.MustCompile(`delivery receipts: ACCEPTING`)},
		{"relay-payments", "-accept-relay-payments", "4812", "4813", regexp.MustCompile(`relay payments: ACCEPTING`)},
	}
	for _, lane := range lanes {
		lane := lane
		t.Run(lane.name+"/refused-with-effective-epoch-blocks-0", func(t *testing.T) {
			args := append(append([]string{}, base...), "-store", t.TempDir(), lane.flag, "-id-seed", lane.seed)
			d := startDaemon(t, "f8-"+lane.name+"-no-epochs", args...)
			line := d.waitFor(t, reRefuseLine, 20*time.Second)[0]
			for _, want := range []string{"-accept-delivery-receipts", "-accept-relay-payments", "-epoch-blocks"} {
				if !strings.Contains(line, want) {
					t.Errorf("the refusal does not name %q (R-F8-DISABLED: name BOTH lane flags and -epoch-blocks):\n\t%s", want, line)
				}
			}
			// The refusal must be an EXIT, not a warning: wait for the process, bounded.
			done := make(chan error, 1)
			go func() { done <- d.cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("the daemon exited 0 after printing a refusal; a refusal is a non-zero exit\n--- output ---\n%s", d.out.dump())
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("the daemon printed the refusal but did not exit within 20s\n--- output ---\n%s", d.out.dump())
			}
			if m := d.out.find(rePeer); m != nil {
				t.Fatalf("a daemon refused for a missing epoch clock became a peer first: %q", m[0])
			}
			if m := d.out.find(lane.accepting); m != nil {
				t.Fatalf("a daemon refused for a missing epoch clock announced the lane first: %q", m[0])
			}
		})
		t.Run(lane.name+"/control-epoch-blocks-8-reaches-accepting", func(t *testing.T) {
			args := append(append([]string{}, base...), "-store", t.TempDir(), lane.flag, "-epoch-blocks", "8", "-id-seed", lane.controlSeed)
			d := startDaemon(t, "f8-"+lane.name+"-epochs-8", args...)
			d.waitFor(t, lane.accepting, 20*time.Second)
			if m := d.out.find(reRefuseLine); m != nil {
				t.Fatalf("the control (-epoch-blocks 8) printed a refusal: %q", m[0])
			}
		})
	}
}
