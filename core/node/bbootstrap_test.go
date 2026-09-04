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

// R2.9a — the node-tier B_bootstrap snapshot: (age, bytes) pairs only.

func r29aNode(t *testing.T) (*Node, *credit.Ledger) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(29001)
	ledger := credit.New(50_000, 0)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetLedger(ledger)
	return nd, ledger
}

func r29aFetcher(b byte) ports.NodeID { return ports.HashBytes([]byte{b, 0x29}) }

// TestR29a_NodeSeriesIsAgeAndBytesOnly: the node snapshot reports the ledger epoch,
// the requester total, and one (ageEpochs, fetchedBytes) row per requester — sorted
// by age then bytes — and it carries NO identity at all.
func TestR29a_NodeSeriesIsAgeAndBytesOnly(t *testing.T) {
	nd, ledger := r29aNode(t)
	server := ports.HashBytes([]byte("server"))

	// old identity: first touch at epoch 0, 1,000 bytes.
	ledger.RecordServe(server, r29aFetcher(1), ports.HashBytes([]byte("c1")), 1_000)
	// Move the ledger clock (R2.10: an injected source, not a call argument), then a
	// young identity fetches less.
	ledger.SetEpochSource(f8EpochFunc(func() uint64 { return 6 }))
	ledger.RedeemDeliveryCredit(server, r29aFetcher(9), ports.HashBytes([]byte("tick")), []byte("tick-6"), 6)
	ledger.RecordServe(server, r29aFetcher(2), ports.HashBytes([]byte("c2")), 250)

	got := nd.BBootstrap()
	if got.Epoch != 6 {
		t.Fatalf("Epoch = %d, want 6", got.Epoch)
	}
	if got.Requesters != 2 || got.Truncated {
		t.Fatalf("Requesters = %d, Truncated = %v, want 2, false", got.Requesters, got.Truncated)
	}
	if len(got.Series) != 2 {
		t.Fatalf("Series = %+v, want 2 rows", got.Series)
	}
	// Sorted by age ascending: the YOUNG identity (age 0) comes first.
	if got.Series[0].AgeEpochs != 0 || got.Series[0].FetchedBytes != 250 {
		t.Fatalf("row 0 = %+v, want {AgeEpochs:0 FetchedBytes:250}", got.Series[0])
	}
	if got.Series[1].AgeEpochs != 6 || got.Series[1].FetchedBytes != 1_000 {
		t.Fatalf("row 1 = %+v, want {AgeEpochs:6 FetchedBytes:1000}", got.Series[1])
	}

	// Privacy floor: the node row is (age, bytes). Nothing else.
	rt := reflect.TypeOf(BBootstrapRow{})
	if rt.NumField() != 2 {
		t.Fatalf("BBootstrapRow has %d fields — the HTTP series is (ageEpochs, fetchedBytes) ONLY", rt.NumField())
	}
	for i := 0; i < rt.NumField(); i++ {
		if f := rt.Field(i); f.Type.Kind() != reflect.Uint64 && f.Type.Kind() != reflect.Int64 {
			t.Fatalf("BBootstrapRow.%s is %s — only the two numbers may ride the series", f.Name, f.Type)
		}
	}
}

// TestR29a_NoLedgerYieldsAnEmptySeries: a node with no ledger reports an empty
// series rather than panicking (the same shape EconomySelf uses).
func TestR29a_NoLedgerYieldsAnEmptySeries(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(29002)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	got := nd.BBootstrap()
	if got.Requesters != 0 || len(got.Series) != 0 || got.Truncated {
		t.Fatalf("no-ledger snapshot = %+v, want empty", got)
	}
}

// TestR29a_EconomySelfFieldsAreUnchanged: R2.9a adds a series, it does not touch the
// existing SELF panel. Pin EconomySelf's exported field set so a later edit to the
// economy surface cannot silently drop or rename one.
func TestR29a_EconomySelfFieldsAreUnchanged(t *testing.T) {
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
	// And it still reports this node's own numbers.
	nd, ledger := r29aNode(t)
	ledger.RecordServe(nd.ID(), ports.HashBytes([]byte("other")), ports.HashBytes([]byte("c")), 77)
	if es := nd.EconomySelf(); es.ServedBytes != 77 {
		t.Fatalf("EconomySelf.ServedBytes = %d, want 77", es.ServedBytes)
	}
}
