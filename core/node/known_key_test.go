package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

type writerSet map[ports.NodeID]bool

func (w writerSet) IsWriter(id ports.NodeID) bool { return w[id] }

// TestANonWritersProviderRecordIsKeptOnlyForKnownContent: under -writers, a joiner's "I hold
// X" is kept only when X is content this node or a writer already provides, so the operator's
// nodes never index content the operator did not publish; a viewer re-serving the operator's
// own file is kept.
func TestANonWritersProviderRecordIsKeptOnlyForKnownContent(t *testing.T) {
	r := newNetgetRig(t)
	nd := r.nodes[5]
	ae, joiner := r.nodes[0].ID(), identity.FromSeed(4801).NodeID()
	nd.SetWriterOracle(writerSet{ae: true})
	known, unknown := ports.HashBytes([]byte("published by the operator")), ports.HashBytes([]byte("someone else's"))
	nd.provs.Add(ports.ProviderRecord{Key: known, ID: ae})

	if !nd.knownKeyFor(joiner, known) {
		t.Fatal("a joiner's record for the operator's own content was refused")
	}
	if nd.knownKeyFor(joiner, unknown) {
		t.Fatal("a joiner's record for content no writer provides was kept")
	}
	if !nd.knownKeyFor(ae, unknown) {
		t.Fatal("a writer's record was refused")
	}
	nd.SetWriterOracle(nil)
	if !nd.knownKeyFor(joiner, unknown) {
		t.Fatal("without -writers every record must be kept")
	}
}
