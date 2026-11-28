package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestANodeNeverAnswersForWhatItIsFetching: a piece held only in a fetch's working set is not
// advertised — the node is reading it, and naming itself would link it to the content — while
// the same piece hosted (outside any working set) is answered for.
func TestANodeNeverAnswersForWhatItIsFetching(t *testing.T) {
	r := newNetgetRig(t)
	nd := r.nodes[7]
	c := ports.NewChunk([]byte("a piece this node is fetching"))
	key := ports.Hash(c.ID)
	nd.Store().Put(bg(), c)

	nd.wantWorking([]ports.ChunkID{c.ID})
	for _, rec := range nd.withSelfIfHeld(key, nil) {
		if rec.ID == nd.ID() {
			t.Fatal("a node advertised itself for a piece in its fetch's working set")
		}
	}
	nd.releaseWorking([]ports.ChunkID{c.ID})
	found := false
	for _, rec := range nd.withSelfIfHeld(key, nil) {
		found = found || rec.ID == nd.ID()
	}
	if !found {
		t.Fatal("CONTROL: the same piece, hosted and out of any working set, was not answered for")
	}
}
