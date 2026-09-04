package node

import (
	"reflect"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"

	"github.com/nerolabs/silt/ports"
)

// R2.9a — the node-tier read of the B_bootstrap histogram.

func r29aNode(t *testing.T) (*Node, *credit.Ledger, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(29001)
	ledger := credit.New(50_000, 0)
	// G-BB-2: the age axis rides an INJECTED ports.Clock, so a sim passes its own
	// scheduler and the whole instrument stays deterministic and replayable by seed.
	ledger.SetObservabilityClock(sched)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetLedger(ledger)
	return nd, ledger, sched
}

func r29aFetcher(b byte) ports.NodeID { return ports.HashBytes([]byte{b, 0x29}) }

// advance moves simulated time by d without needing an event to fire: the instrument
// only ever calls Now, so a scheduled no-op callback is the cheapest way to step the
// scheduler's clock deterministically.
func r29aAdvance(t *testing.T, sched *simclock.Scheduler, d ports.Duration) {
	t.Helper()
	sched.AfterFunc(d, func() {})
	if !sched.Step() {
		t.Fatalf("simclock did not advance by %d", d)
	}
}

// TestR29aNodeSnapshotIsTheHistogramWithNoIdentity is the node tier's gate. The node
// used to narrow the ledger snapshot here, dropping a salted requester label before
// publication; under the histogram there is nothing left to narrow, and this pins that:
// the object the node hands up carries counts and axis metadata and NOTHING that names,
// labels or times an individual.
func TestR29aNodeSnapshotIsTheHistogramWithNoIdentity(t *testing.T) {
	nd, ledger, sched := r29aNode(t)
	server := ports.HashBytes([]byte("server"))

	// An old identity fetches 1,000 bytes, then two hours pass, then a young one
	// fetches 250.
	ledger.RecordServe(server, r29aFetcher(1), ports.HashBytes([]byte("c1")), 1_000)
	r29aAdvance(t, sched, 2*3600*ports.Second)
	ledger.RecordServe(server, r29aFetcher(2), ports.HashBytes([]byte("c2")), 250)

	h, ok := nd.BBootstrap()
	if !ok {
		t.Fatalf("BBootstrap reported no export on a node with a real ledger")
	}
	if h.ClockSource != "injected" || !h.AgeAxisLive {
		t.Fatalf("clock source = %q, ageAxisLive = %v — the sim scheduler IS the injected clock", h.ClockSource, h.AgeAxisLive)
	}
	if h.Requesters != 2 || h.Aged != 2 || h.Unstamped != 0 {
		t.Fatalf("requesters/aged/unstamped = %d/%d/%d, want 2/2/0", h.Requesters, h.Aged, h.Unstamped)
	}
	if h.UptimeNanos != int64(2*3600*ports.Second) {
		t.Fatalf("uptimeNanos = %d, want %d", h.UptimeNanos, int64(2*3600*ports.Second))
	}
	if h.Cells == nil {
		t.Fatalf("cells nil with a live clock")
	}
	// The old identity is two hours old; the young one is exactly zero.
	var total int64
	for a := range h.Cells {
		for _, c := range h.Cells[a] {
			total += c
		}
	}
	if total != 2 {
		t.Fatalf("cells total %d, want 2", total)
	}
	// The young one: age exactly 0 (bucket 0), 250 bytes → bin floor(4·log2(250)) = 31.
	if got := h.Cells[0][31]; got != 1 {
		t.Fatalf("the age-0 identity (250 bytes → bin 31) count = %d, want 1; row 0 = %v", got, h.Cells[0])
	}
	// The old one: two hours → bucket 4 ([1h, 6h)), 1,000 bytes → bin 39.
	if got := h.Cells[4][39]; got != 1 {
		t.Fatalf("the two-hour-old identity (1,000 bytes → bin 39) count = %d in bucket 4, want 1; row 4 = %v", got, h.Cells[4])
	}
	if h.MaxOccupiedAgeEdgeNanos > h.UptimeNanos || h.AgeExceedsUptime {
		t.Fatalf("censoring invariant violated: max occupied edge %d, uptime %d", h.MaxOccupiedAgeEdgeNanos, h.UptimeNanos)
	}

	// PRIVACY FLOOR, structural: every exported field of the object the node hands up
	// is a number, a bool, a fixed axis array or the two documented strings. There is
	// no slice of rows and no identity-shaped field, so there is nothing a per-requester
	// datum could ride out on.
	rt := reflect.TypeOf(credit.BBootstrapHistogram{})
	allowed := map[string]bool{
		"ClockSource": true, "AgeAxisLive": true, "Requesters": true, "Aged": true,
		"Unstamped": true, "UptimeNanos": true, "MaxOccupiedAgeEdgeNanos": true,
		"ClockStepBack": true, "AgeExceedsUptime": true, "AgeEdgeNanos": true,
		"BinsPerOctave": true, "ByteBins": true, "ByteBinRule": true, "Cells": true,
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !allowed[f.Name] {
			t.Fatalf("credit.BBootstrapHistogram has an unaudited field %q (%s): every field the instrument publishes must be an aggregate, and a new one is a privacy question, not a formatting one", f.Name, f.Type)
		}
	}
	if rt.NumField() != len(allowed) {
		t.Fatalf("credit.BBootstrapHistogram has %d fields, the audited set has %d — a field was removed without updating this gate", rt.NumField(), len(allowed))
	}
}

// TestR29aNoLedgerYieldsNoExport: a node with no ledger reports "no export" rather than
// an empty-looking histogram a reader could mistake for a quiet network (the same shape
// EconomySelf uses).
func TestR29aNoLedgerYieldsNoExport(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(29002)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	if h, ok := nd.BBootstrap(); ok {
		t.Fatalf("BBootstrap reported an export on a ledger-less node: %+v", h)
	}
}

// TestR29aEconomySelfFieldsAreUnchanged: R2.9a adds an instrument, it does not touch the
// existing SELF panel. Pin EconomySelf's exported field set so a later edit to the
// economy surface cannot silently drop or rename one.
func TestR29aEconomySelfFieldsAreUnchanged(t *testing.T) {
	want := []string{"Balance", "ServedBytes", "FetchedBytes", "RepairsDone", "BountyEarned"}
	rt := reflect.TypeOf(EconomySelf{})
	if rt.NumField() != len(want) {
		t.Fatalf("EconomySelf has %d fields, want %d (%v)", rt.NumField(), len(want), want)
	}
	for i, name := range want {
		if got := rt.Field(i).Name; got != name {
			t.Fatalf("EconomySelf field %d = %q, want %q", i, got, name)
		}
	}
	nd, ledger, _ := r29aNode(t)
	ledger.RecordServe(nd.ID(), ports.HashBytes([]byte("other")), ports.HashBytes([]byte("c")), 77)
	if es := nd.EconomySelf(); es.ServedBytes != 77 {
		t.Fatalf("EconomySelf.ServedBytes = %d, want 77", es.ServedBytes)
	}
}
