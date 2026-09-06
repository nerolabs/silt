package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/relaypay"
)

// TestDaemonFeeIsTheRelayAnchorFace pins the ONE fee constant (Tester finding, R2.14
// verification): relaypay.MaxAnchorsPerSession derives from ShippedAnchorFace, and the
// daemon's ledger fee must be that same constant or a re-pricing (R2.9) silently drifts
// k_max from the real face. SOURCE GATE: reads daemon.go as text and asserts the ledger
// is constructed from relaypay.ShippedAnchorFace, never a literal. RUNTIME GATE:
// TestRelayMaxAnchorsPerSessionCoversTheSessionCeiling (core/relaypay) covers the k_max
// derivation from the constant.
func TestDaemonFeeIsTheRelayAnchorFace(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "credit.New(relaypay.ShippedAnchorFace,") {
		t.Fatal("SOURCE GATE: daemon.go must construct the ledger with credit.New(relaypay.ShippedAnchorFace, ...) — a second fee literal lets k_max drift from the real face")
	}
	if relaypay.ShippedAnchorFace <= 0 || relaypay.MaxAnchorsPerSession != (relaypay.MaxChainLength*relaypay.RelayIncrementCredit+relaypay.ShippedAnchorFace-1)/relaypay.ShippedAnchorFace {
		t.Fatalf("SOURCE GATE: k_max %d is not derived from the face %d", relaypay.MaxAnchorsPerSession, relaypay.ShippedAnchorFace)
	}
	// Since the 2026-09-06 re-price S_max itself derives from the face, so the line above
	// is an identity for ANY face (blind PE F-1). The face is held by an independent
	// literal: the shipped --fee the flag help and the 64 GiB grant/r pin were derived on.
	if relaypay.ShippedAnchorFace != 50_000 {
		t.Fatalf("SOURCE GATE: the shipped face is %d, not 50,000 — the publish fee moved; the grant/r pin, the guard bound and the relay ceilings were all derived on 50,000 and must be re-certified", relaypay.ShippedAnchorFace)
	}
}
