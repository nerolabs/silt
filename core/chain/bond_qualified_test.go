package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestBondQualifiedNeedsACommittedBondAtTheFloor: only a committed bond at or above
// the bond floor, held by an identity not slashed, qualifies. A network with no floor
// qualifies nobody.
func TestBondQualifiedNeedsACommittedBondAtTheFloor(t *testing.T) {
	a, b, c := ports.NodeID{1}, ports.NodeID{2}, ports.NodeID{3}
	ch := New(Config{MinBond: 1 << 20}, func(ports.NodeID) int64 { return 0 })
	ch.bonded[a] = 1 << 20
	ch.bonded[b] = 1<<20 - 1
	ch.bonded[c] = 4 << 20
	ch.slashed[c] = true
	if !ch.BondQualified(a) {
		t.Fatal("a bond at the floor must qualify")
	}
	if ch.BondQualified(b) {
		t.Fatal("a bond below the floor must not qualify")
	}
	if ch.BondQualified(c) {
		t.Fatal("a slashed identity must not qualify, whatever its bond")
	}
	if ch.BondQualified(ports.NodeID{4}) {
		t.Fatal("an identity with no bond must not qualify")
	}
	free := New(Config{}, func(ports.NodeID) int64 { return 0 })
	free.bonded[a] = 1 << 30
	if free.BondQualified(a) {
		t.Fatal("with no bond floor, nothing qualifies: there is no bond to lose")
	}
}
