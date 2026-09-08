package main

// R2.7 / A2 — receiptCoverage 0 is ambiguous three ways, and the third is the DEFAULT
// posture (Economist as-built ADVISORY-c4-r27-telemetry-as-built-93deb56-2026-09-08 §4(a)).
//
// A node that does not run -accept-delivery-receipts never calls SettleDelivery, so it
// prints receiptCoverage 0 beside a large serveBytesObjectAware while behaving perfectly.
// ROADMAP C6 makes coverage < 0.75 a machine-read canary abort, and the RC ships
// default-OFF, so without a lane-state field EVERY honest RC-default node trips it. The
// shape is the one economySelfFunding.bountyOn already uses for -economy.
//
// ABLATION run RED before this shipped (recorded in the PR): pinning LaneOn true (the
// "there is only one posture" assumption) reddens the lane-off arm.

import (
	"encoding/json"
	"testing"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

func TestCoverageZeroIsDistinguishableFromTheLaneBeingOff(t *testing.T) {
	s, led := statusServer(t)
	root := ports.Hash{0xC0, 0x11}
	fetcher := ports.NodeID{0xF1}
	at := s.started

	readServeMint := func() struct {
		LaneOn          bool    `json:"laneOn"`
		ObjectAware     int64   `json:"serveBytesObjectAware"`
		Witnessed       int64   `json:"serveBytesWitnessed"`
		ReceiptCoverage float64 `json:"receiptCoverage"`
	} {
		t.Helper()
		var sm struct {
			LaneOn          bool    `json:"laneOn"`
			ObjectAware     int64   `json:"serveBytesObjectAware"`
			Witnessed       int64   `json:"serveBytesWitnessed"`
			ReceiptCoverage float64 `json:"receiptCoverage"`
		}
		s.invalidateStatus()
		raw := statusKey(t, statusAt(t, s, at, true), "serveMint")
		if len(raw) == 0 {
			t.Fatal("no serveMint block on the TOKENED /api/status")
		}
		if err := json.Unmarshal(raw, &sm); err != nil {
			t.Fatalf("decode serveMint: %v (%s)", err, raw)
		}
		return sm
	}

	// ARM 1 — the DEFAULT posture. Real object-aware serving, no accepted receipts,
	// coverage 0. This is an honest node and the abort must not fire on it.
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		led.RecordServeToObject(s.nd.ID(), fetcher, root, ports.ChunkID{0x1}, 8*econMintUnit)
	})
	off := readServeMint()
	if off.ObjectAware == 0 {
		t.Fatal("fixture is vacuous: nothing witnessable was served, so this arm would pass on the zero-denominator case instead")
	}
	if off.Witnessed != 0 || off.ReceiptCoverage != 0 {
		t.Fatalf("lane-off arm: witnessed %d coverage %v, want 0 and 0 — the fixture is settling receipts", off.Witnessed, off.ReceiptCoverage)
	}
	if off.LaneOn {
		t.Fatal("laneOn is true on a node that never called EnableDeliverySessions — a machine-read coverage abort cannot tell an honest default node from a suppressing one")
	}

	// ARM 2 — the lane ON, still nothing witnessed. Same coverage, DIFFERENT document:
	// this is the node on which a persistent zero really is worth investigating.
	s.onLoop(func() { s.nd.EnableDeliverySessions(ports.Duration(60e9)) })
	on := readServeMint()
	if !on.LaneOn {
		t.Fatal("laneOn is false after EnableDeliverySessions — the field does not track the lane, so it disambiguates nothing")
	}
	if on.ReceiptCoverage != off.ReceiptCoverage || on.ObjectAware != off.ObjectAware {
		t.Fatalf("the two arms differ in coverage (%v vs %v) or denominator (%d vs %d); they must differ ONLY in laneOn, or this gate is not testing the ambiguity",
			on.ReceiptCoverage, off.ReceiptCoverage, on.ObjectAware, off.ObjectAware)
	}
}
