package node

import (
	"context"
	"errors"

	"github.com/nerolabs/silt/ports"
)

// A fetch's WORKING SET is the pieces it pulls to assemble a file and then drops. It is not hosting:
// nothing is announced and nothing is kept. So a node whose pledge is full must still be able to
// fetch. Before this, the pull wrote each piece into the pledge-capped store, a full store refused
// it with ErrStoreFull, the refusal read as an unreachable holder, and the fetch failed as "no
// shards available at all" — on a network that held every byte (seen on the flixz node at a full
// 5 GB pledge, 2026-11-27).
//
// A piece the pledge refuses is held in memory for the life of the fetch instead, and only when the
// fetch registered it as wanted: the same pull routine serves repair, which must never be told a
// piece arrived that the node cannot actually host. The overflow is never hosted, proven or
// announced, and a retaining fetch (-serve-fetched) does not retain it — the pledge decides what a
// node hosts, and a full pledge hosts nothing more.

// wantWorking marks ids as part of an in-flight fetch's working set.
func (n *Node) wantWorking(ids []ports.ChunkID) {
	if n.workingWant == nil {
		n.workingWant = make(map[ports.ChunkID]int)
	}
	for _, id := range ids {
		n.workingWant[id]++
	}
}

// releaseWorking ends a fetch's claim on ids, dropping any of them held in the overflow.
func (n *Node) releaseWorking(ids []ports.ChunkID) {
	for _, id := range ids {
		if n.workingWant[id] <= 1 {
			delete(n.workingWant, id)
			delete(n.workOverflow, id)
		} else {
			n.workingWant[id]--
		}
	}
}

// putPulled stores a pulled piece. A piece a fetch wants and the full pledge refuses is held in
// memory for that fetch; every other refusal stands.
func (n *Node) putPulled(c ports.Chunk) error {
	err := n.store.Put(bg(), c)
	if err == nil || !errors.Is(err, ports.ErrStoreFull) || n.workingWant[c.ID] == 0 {
		return err
	}
	if n.workOverflow == nil {
		n.workOverflow = make(map[ports.ChunkID][]byte)
	}
	n.workOverflow[c.ID] = c.Data
	n.Stats.WorkingSetOverflow++
	return nil
}

// overflowed reports whether id is held only in the working-set overflow.
func (n *Node) overflowed(id ports.ChunkID) bool {
	_, ok := n.workOverflow[id]
	return ok
}

// workingStore reads a fetch's working set: the node's store, then the overflow. Writes go to the
// store. It is handed to assembly, never to anything that hosts.
type workingStore struct{ n *Node }

var _ ports.ChunkStore = workingStore{}

func (w workingStore) Put(ctx context.Context, c ports.Chunk) error { return w.n.store.Put(ctx, c) }

func (w workingStore) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	if data, ok := w.n.workOverflow[id]; ok {
		c := ports.Chunk{ID: id, Data: data}
		if !c.Verify() {
			return ports.Chunk{}, ports.ErrNotFound
		}
		return c, nil
	}
	return w.n.store.Get(ctx, id)
}

func (w workingStore) Has(ctx context.Context, id ports.ChunkID) (bool, error) {
	if w.n.overflowed(id) {
		return true, nil
	}
	return w.n.store.Has(ctx, id)
}

func (w workingStore) List(ctx context.Context) ([]ports.ChunkID, error) {
	ids, err := w.n.store.List(ctx)
	for id := range w.n.workOverflow {
		ids = append(ids, id)
	}
	return ids, err
}

func (w workingStore) Delete(ctx context.Context, id ports.ChunkID) error {
	delete(w.n.workOverflow, id)
	return w.n.store.Delete(ctx, id)
}
