package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// The legs are consecutive intervals, so they sum to the total, and a leg that never
// ran is absent rather than reported as zero.
func TestPublishLegsAreConsecutiveAndSumToTheTotal(t *testing.T) {
	t0 := time.Unix(1000, 0)
	p := newPublishLegs(t0)
	p.at("join", t0.Add(300*time.Millisecond))
	p.at("stage", t0.Add(310*time.Millisecond))
	p.at("place", t0.Add(1510*time.Millisecond))
	want := "join 0.300s stage 0.010s place 1.200s total 1.510s"
	if got := p.String(); got != want {
		t.Fatalf("legs = %q, want %q", got, want)
	}
	if got := newPublishLegs(t0).String(); got != "total 0.000s" {
		t.Fatalf("no legs = %q, want only the zero total", got)
	}
}

// The token gather reports its three stages in the order they run, even when it fails:
// a failed gather is the one whose stage times need reading.
func TestAcquirePublishTokenMarksEachStage(t *testing.T) {
	nd, loop := testClientNode(t, 9621)
	unreachable := identity.FromSeed(9622).NodeID()
	var marks []string
	done := make(chan struct{})
	loop.Post("test", func() {
		acquirePublishToken(nd, []ports.NodeID{unreachable}, 1, func(leg string) { marks = append(marks, leg) },
			func(*ports.PublishToken, error) { close(done) })
	})
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("acquirePublishToken never called back")
	}
	if got := strings.Join(marks, ","); got != "keys,credits,sign" {
		t.Fatalf("stage marks = %s, want keys,credits,sign", got)
	}
}
