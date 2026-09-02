package main

// R0.4b C3 re-break — F7 regression gate (blast radius). Inversion of the red-team
// probe adapters/diskissuer/rt_c3b_store_test.go RT-C3B-13, at the tier the finding is
// actually about: the DAEMON, not the store.
//
// The finding: `Load` treats a corrupt file as a hard error (correct — see
// TestRTC3_CorruptStoreErrorsAndIsNeverRewritten) and runDaemon returned it straight
// out, so ONE bad byte in demandkeys.cbor stopped chain participation, storage and
// serving. The demand receipt lane is OPTIONAL; a validator must not die for it.
//
// This is a SOURCE gate, in the depcheck style, because the branch it protects sits
// deep inside runDaemon's flag-driven wiring and has no callable seam. It asserts the
// two facts that make the degrade real: the demand-lane block reads the store before
// arming the lane, and the failure path PRINTS a lane-off line rather than returning an
// error out of runDaemon.

import (
	"os"
	"strings"
	"testing"
)

func TestDaemonDegradesTheDemandLaneInsteadOfDying(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// The demand-lane wiring block, delimited by the store open and the ACCEPTING line.
	start := strings.Index(body, `des, derr := diskissuer.OpenEpochs(`)
	if start < 0 {
		t.Fatal("the demand issuer key store is no longer opened in daemon.go — re-anchor this gate")
	}
	end := strings.Index(body[start:], "delivery receipts: ACCEPTING")
	if end < 0 {
		t.Fatal("the delivery-receipts ACCEPTING line is gone — re-anchor this gate")
	}
	block := body[start : start+end]

	if !strings.Contains(block, "des.Load()") {
		t.Fatal("the demand lane no longer READS the key store before arming itself, so a " +
			"corrupt store is discovered somewhere else — probably on a path that returns " +
			"out of runDaemon and takes the whole validator down with the receipt lane")
	}
	if !strings.Contains(block, "LANE OFF") {
		t.Fatal("the demand-lane wiring has no LANE OFF degrade path. A corrupt demand key " +
			"file must stop the RECEIPT LANE, not chain participation, storage and serving " +
			"(red-team re-break F7).")
	}
	// The specific regression: returning the store error out of runDaemon.
	for _, forbidden := range []string{
		`return fmt.Errorf("demand issuer keys: %w", kerr)`,
	} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("daemon.go still returns a demand-lane failure out of runDaemon (%q): "+
				"one bad byte in demandkeys.cbor takes the whole validator down", forbidden)
		}
	}
}

// TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists is the F2 wiring gate: the
// durable guard must be attached AND loaded before anything can accept a receipt.
// Ordering is the property, so it is asserted on the source order.
func TestDaemonLoadsThePaidSerialGuardBeforeTheNodeExists(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	iAttach := strings.Index(body, "SetPaidSerialStore(")
	iLoad := strings.Index(body, "LoadPaidSerials()")
	iBank := strings.Index(body, "nd.EnableDemandBank(")
	switch {
	case iAttach < 0 || iLoad < 0:
		t.Fatal("the daemon no longer attaches and loads the durable paid-serial guard. A " +
			"restart is then an eviction of every guarded token and the same wire receipt " +
			"pays twice (red-team re-break F2).")
	case iBank < 0:
		t.Fatal("EnableDemandBank is gone from daemon.go — re-anchor this gate")
	case !(iAttach < iLoad && iLoad < iBank):
		t.Fatalf("the guard must be attached (%d) and LOADED (%d) before the demand bank is "+
			"armed (%d): a redeem before the load completes is refused, but only if the load "+
			"actually happens first", iAttach, iLoad, iBank)
	}
}
