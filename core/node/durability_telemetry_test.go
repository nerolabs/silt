package node

// Phase 2, Slice 2 — durability telemetry. The S7 repair economy runs half-open on
// a live daemon today: the serve auto-skim fills each object's escrow, but the
// funded reserve, lifetime skim/pay, and whether bounties actually DISBURSE were
// invisible (credit.G/Horizon computed only for a local repair decision, never
// surfaced). These accessors make it observable — the prerequisite for watching g
// once the economy is switched on. Deliberation:
// docs/thinking/2026-08-19-phase2-economy-on-deliberation.md.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

func telemetryNode(t *testing.T) *Node {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9700)
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.SetLedger(credit.New(0, 100_000)) // fee 0; 100k starter grant per account
	return nd
}

// TestDurabilityTelemetrySurfacesTheHalfOpenEconomy: with RepairBountyBase=0
// (default) the economy is half-open — escrows accept funding but bounties do not
// pay. The telemetry must report that honestly (bountyOn=false) AND surface the
// reserve a funded/skimmed object holds, so an operator can see credits accruing
// with nowhere to go. RED before the accessors existed (no way to observe it).
func TestDurabilityTelemetrySurfacesTheHalfOpenEconomy(t *testing.T) {
	nd := telemetryNode(t)

	if nd.RepairBountyEnabled() {
		t.Fatal("default node must report the repair economy OFF (RepairBountyBase=0)")
	}
	if b := nd.CreditBalance(); b <= 0 {
		t.Fatalf("node should carry its starter grant balance, got %d", b)
	}

	// Care an object and fund its durability reserve from the node's own balance
	// (the publisher-endowment path FundDurability wires).
	root := ports.Hash{0xD0, 0x0B}
	nd.care = append(nd.care, link.CareHandle{Root: root})
	const endow = 4_000
	balBefore := nd.CreditBalance()
	if err := nd.FundDurability(root, endow); err != nil {
		t.Fatalf("FundDurability: %v", err)
	}
	if got := nd.CreditBalance(); got != balBefore-endow {
		t.Fatalf("funding must debit the node balance: %d → %d (want -%d)", balBefore, got, endow)
	}

	cared := nd.CaredDurability()
	if len(cared) != 1 || cared[0].Root != root {
		t.Fatalf("CaredDurability should surface the one cared root, got %+v", cared)
	}
	if cared[0].Snapshot.Balance != endow || cared[0].Snapshot.Funded != endow {
		t.Fatalf("reserve should show the endowment: reserve=%d funded=%d, want %d/%d",
			cared[0].Snapshot.Balance, cared[0].Snapshot.Funded, endow, endow)
	}
	if cared[0].Snapshot.Paid != 0 || cared[0].Snapshot.Repairs != 0 {
		t.Fatalf("nothing paid yet (bounties off): paid=%d repairs=%d", cared[0].Snapshot.Paid, cared[0].Snapshot.Repairs)
	}

	// Enabling the base flips the observable state to ON — the keystone Slice 1
	// will set this from a flag; the telemetry must reflect it.
	nd.cfg.RepairBountyBase = 1_000
	if !nd.RepairBountyEnabled() {
		t.Fatal("RepairBountyBase>0 must report the economy ON")
	}
}

// TestCaredDurabilityDedupesRoots: a node may hold several care handles for the
// same root (re-cared across restarts); the telemetry must not double-count it.
func TestCaredDurabilityDedupesRoots(t *testing.T) {
	nd := telemetryNode(t)
	root := ports.Hash{0xAB}
	nd.care = append(nd.care, link.CareHandle{Root: root}, link.CareHandle{Root: root})
	if got := len(nd.CaredDurability()); got != 1 {
		t.Fatalf("duplicate care handles for one root must collapse to one row, got %d", got)
	}
}

// TestDurabilityTelemetryNoLedgerIsSafe: the accessors are zero-valued and never
// panic when no ledger is wired (a plain storage node with the economy off).
func TestDurabilityTelemetryNoLedgerIsSafe(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9701)
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	// no SetLedger
	if nd.CreditBalance() != 0 || nd.CaredDurability() != nil {
		t.Fatal("no-ledger node must report zero balance / nil durability, not panic")
	}
}
