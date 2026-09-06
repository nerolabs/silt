package node

// R-PARITY-AMPLIFICATION — the per-stripe DEFICIT walk in NetGet's parity fallback, held to the
// blind PE design ruling RULING-parity-fetch-per-stripe-design-2026-09-06 §7 (G-PS-1…7). The rig
// is the existing 80 KiB / 4 KiB swarm: 21 data chunks, K=10 ⇒ 3 stripes, the final stripe
// holding ONE real data shard. Withholding = deleting a shard from every node's store. The
// consumer uses NetGetRetain so the pulled parity ids are observable in its store afterwards.
// These gates do NOT claim R-PARITY-AMPLIFICATION closed — that claim is research-gated; they
// pin what the fetcher pulls. There is no uncoded (K == 0) gate: K == 0 is unreachable from any
// publish path (pipeline.Add always erasure-codes), so the dead helper's removal is safe by
// unreachability, not by suite (PE code ruling F-3).

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/ports"
)

// psWithhold deletes ids from every node's store in the rig (a withheld shard).
func psWithhold(t *testing.T, r *netgetRig, ids ...ports.ChunkID) {
	t.Helper()
	for _, nd := range r.nodes {
		for _, id := range ids {
			_ = nd.Store().Delete(bg(), id)
		}
	}
}

// psConsumer prepares r.nodes[1] as the consumer exactly as TestNetGetDropsWorkingSet does
// (pre-seeds one manifest chunk so a scratch node can start), and returns it.
func psConsumer(t *testing.T, r *netgetRig) *Node {
	t.Helper()
	consumer := r.nodes[1]
	entry, _, _ := r.reg.Lookup(bg(), r.h.Root)
	preHeld := entry.ManifestChunks[0]
	var fetched bool
	r.nodes[2].FetchChunk(preHeld, func(err error) { fetched = err == nil })
	r.sched.Run()
	if !fetched {
		t.Fatalf("rig: could not fetch the pre-seed chunk")
	}
	c, err := r.nodes[2].Store().Get(bg(), preHeld)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Store().Put(bg(), c); err != nil {
		t.Fatal(err)
	}
	r.nodes[2].dropHosted(preHeld)
	return consumer
}

// psRetain runs NetGetRetain on the consumer and asserts a bit-perfect retrieval.
func psRetain(t *testing.T, r *netgetRig, consumer *Node) {
	t.Helper()
	var out bytes.Buffer
	var getErr error
	done := false
	consumer.NetGetRetain(r.reg, r.h, &out, func(err error) { getErr, done = err, true })
	r.sched.Run()
	if !done || getErr != nil {
		t.Fatalf("NetGetRetain: done=%v err=%v", done, getErr)
	}
	if !bytes.Equal(out.Bytes(), r.data) {
		t.Fatalf("retrieval is not bit-perfect")
	}
}

// psParityHeld returns the parity ids the consumer holds after retrieval, keyed by stripe.
func psParityHeld(r *netgetRig, consumer *Node) map[int][]ports.ChunkID {
	held := map[int][]ports.ChunkID{}
	parity := r.m.ParityIDs()
	per := r.m.N - r.m.K
	for p, id := range parity {
		if ok, _ := consumer.Store().Has(bg(), id); ok {
			held[p/per] = append(held[p/per], id)
		}
	}
	return held
}

// psDataOfStripe returns stripe s's data ids.
func psDataOfStripe(r *netgetRig, s int) []ports.ChunkID {
	data := r.m.ChunkIDs()
	lo, hi := s*r.m.K, min((s+1)*r.m.K, len(data))
	return data[lo:hi]
}

// TestPSHealthyObjectFetchesNoParity (G-PS-1): no data shard missing ⇒ zero parity shards pulled
// and zero parity-column lookups. A REGRESSION PIN, not a discriminator: the old whole-column
// fallback also fetched no parity on a healthy object (PE code ruling F-2), so this gate stays
// green under that ablation and red only if a future change fetches parity unconditionally.
func TestPSHealthyObjectFetchesNoParity(t *testing.T) {
	r := newNetgetRig(t)
	consumer := psConsumer(t, r)
	psRetain(t, r, consumer)
	if consumer.Stats.ParityColumnLookups != 0 || consumer.Stats.ParityShardsPulled != 0 {
		t.Fatalf("healthy object: %d parity lookups, %d parity shards pulled, want 0/0", consumer.Stats.ParityColumnLookups, consumer.Stats.ParityShardsPulled)
	}
	if h := psParityHeld(r, consumer); len(h) != 0 {
		t.Fatalf("healthy object: parity held %v", h)
	}
}

// TestPSOneLossOneStripePullsExactlyOneParityShardOfThatStripe (G-PS-2 + G-PS-3): one data
// shard of stripe 0 withheld ⇒ bit-perfect, exactly ONE parity shard pulled and it belongs to
// stripe 0 (the id SET, not a count), and exactly ONE parity-column lookup (early exit).
func TestPSOneLossOneStripePullsExactlyOneParityShardOfThatStripe(t *testing.T) {
	r := newNetgetRig(t)
	psWithhold(t, r, psDataOfStripe(r, 0)[3])
	consumer := psConsumer(t, r)
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != 1 || len(held[0]) != 1 {
		t.Fatalf("parity held by stripe = %v, want exactly one shard of stripe 0 (the whole-column fallback would pull %d)", held, r.m.N-r.m.K)
	}
	if consumer.Stats.ParityShardsPulled != 1 || consumer.Stats.ParityColumnLookups != 1 {
		t.Fatalf("pulled %d parity shards over %d column lookups, want 1 over 1 (early exit on the first column)", consumer.Stats.ParityShardsPulled, consumer.Stats.ParityColumnLookups)
	}
}

// TestPSEveryStripeDamagedPullsOneParityShardPerStripe (G-PS-4): one data shard withheld from
// EVERY stripe ⇒ bit-perfect and exactly one parity shard per stripe — 1.0×, never the whole
// parity columns.
func TestPSEveryStripeDamagedPullsOneParityShardPerStripe(t *testing.T) {
	r := newNetgetRig(t)
	stripes := (len(r.m.ChunkIDs()) + r.m.K - 1) / r.m.K
	var withheld []ports.ChunkID
	for s := 0; s < stripes; s++ {
		withheld = append(withheld, psDataOfStripe(r, s)[0])
	}
	psWithhold(t, r, withheld...)
	consumer := psConsumer(t, r)
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != stripes {
		t.Fatalf("parity held for %d stripes, want %d", len(held), stripes)
	}
	for s, ids := range held {
		if len(ids) != 1 {
			t.Fatalf("stripe %d pulled %d parity shards, want exactly 1", s, len(ids))
		}
	}
	if consumer.Stats.ParityShardsPulled != stripes || consumer.Stats.ParityColumnLookups != 1 {
		t.Fatalf("pulled %d over %d lookups, want %d over 1", consumer.Stats.ParityShardsPulled, consumer.Stats.ParityColumnLookups, stripes)
	}
}

// TestPSShortFinalStripeDeficitIsItsRealDataCount (G-PS-5): the final stripe holds ONE real data
// shard (21 chunks, K=10). Withholding it must complete with exactly ONE parity shard — a deficit
// of realData (1), never K (10). Under-fetch is the fatal direction; over-fetch merely costs.
func TestPSShortFinalStripeDeficitIsItsRealDataCount(t *testing.T) {
	r := newNetgetRig(t)
	stripes := (len(r.m.ChunkIDs()) + r.m.K - 1) / r.m.K
	last := psDataOfStripe(r, stripes-1)
	if len(last) != 1 {
		t.Fatalf("rig premise moved: final stripe has %d real data shards, want 1", len(last))
	}
	psWithhold(t, r, last[0])
	consumer := psConsumer(t, r)
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != 1 || len(held[stripes-1]) != 1 {
		t.Fatalf("final-stripe loss: parity held %v, want exactly one shard of stripe %d", held, stripes-1)
	}
}

// TestPSMissingParityShardFallsThroughToTheNextColumn (G-PS-6): withhold one data shard of
// stripe 1 AND that stripe's first parity shard ⇒ bit-perfect via the NEXT parity column, with
// at most N−K parity-column lookups (here exactly 2).
func TestPSMissingParityShardFallsThroughToTheNextColumn(t *testing.T) {
	r := newNetgetRig(t)
	per := r.m.N - r.m.K
	firstParityOfStripe1 := r.m.ParityIDs()[1*per+0]
	psWithhold(t, r, psDataOfStripe(r, 1)[2], firstParityOfStripe1)
	consumer := psConsumer(t, r)
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != 1 || len(held[1]) != 1 || held[1][0] == firstParityOfStripe1 {
		t.Fatalf("parity held %v, want exactly one shard of stripe 1 that is NOT the withheld first parity shard", held)
	}
	if consumer.Stats.ParityColumnLookups != 2 || consumer.Stats.ParityColumnLookups > per {
		t.Fatalf("parity-column lookups = %d, want 2 (first column's shard missing, second supplies it)", consumer.Stats.ParityColumnLookups)
	}
}

// rotStore is a chunk store whose Has says a rotten id is present while Get fails on it — the
// disk store's exact shape under bit rot (Has is an os.Stat, Get re-verifies and errors). It
// wraps the rig's memstore for every other id.
type rotStore struct {
	ports.ChunkStore
	rotten map[ports.ChunkID]bool
}

func (r *rotStore) Has(ctx context.Context, id ports.ChunkID) (bool, error) {
	if r.rotten[id] {
		return true, nil
	}
	return r.ChunkStore.Has(ctx, id)
}

func (r *rotStore) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	if r.rotten[id] {
		return ports.Chunk{}, errors.New("rotStore: chunk data does not match its ID")
	}
	return r.ChunkStore.Get(ctx, id)
}

// TestPSBitRottenLocalShardCountsAsMissing (PE code ruling F-1): a data shard the consumer's
// store REPORTS as present (Has) but cannot deliver verified (Get fails, as the disk store does
// on bit rot) must count toward the stripe's deficit, so the walk fetches parity for it and the
// retrieval is bit-perfect. Under a Has-based deficit the walk fetched nothing and the pipeline
// failed on the rotten shard — the old whole-column fetch masked this by accident.
func TestPSBitRottenLocalShardCountsAsMissing(t *testing.T) {
	r := newNetgetRig(t)
	// A fresh consumer on a rotStore, bootstrapped into the rig like the others.
	id := identity.FromSeed(3999).NodeID()
	rot := &rotStore{ChunkStore: memstore.New(), rotten: map[ports.ChunkID]bool{}}
	consumer := New(id, r.nodes[0].cfg, r.sched, r.net.Endpoint(id), rot)
	consumer.Bootstrap([]ports.NodeID{r.nodes[0].ID(), r.nodes[1].ID(), r.nodes[2].ID()}, func() {})
	r.sched.Run()
	// Pre-seed one manifest chunk (as the rig's other consumers do), copied from a holder.
	entry, _, _ := r.reg.Lookup(bg(), r.h.Root)
	preHeld := entry.ManifestChunks[0]
	seeded := false
	for _, nd := range r.nodes {
		if c, err := nd.Store().Get(bg(), preHeld); err == nil {
			if err := rot.Put(bg(), c); err != nil {
				t.Fatal(err)
			}
			seeded = true
			break
		}
	}
	if !seeded {
		t.Fatalf("rig: no node holds the manifest chunk to pre-seed")
	}
	victim := psDataOfStripe(r, 0)[5]
	psWithhold(t, r, victim)  // no honest copy anywhere in the swarm...
	rot.rotten[victim] = true // ...and the consumer's own copy is rotten
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != 1 || len(held[0]) != 1 {
		t.Fatalf("a bit-rotten local shard was counted as present: parity held %v, want one shard of stripe 0", held)
	}
}

// TestPSAlreadyHeldParityIsNotCountedAsPulled (PE code ruling F-4): a parity shard the consumer
// already holds settles the deficit without a transfer and is not counted as pulled.
func TestPSAlreadyHeldParityIsNotCountedAsPulled(t *testing.T) {
	r := newNetgetRig(t)
	consumer := psConsumer(t, r)
	per := r.m.N - r.m.K
	firstParityOfStripe0 := r.m.ParityIDs()[0*per+0]
	// Pre-seed the consumer with the honest first parity shard of stripe 0, copied from whichever
	// rig node holds it (parity shards register under column keys, not their own ids, so a
	// by-id FetchChunk cannot find them).
	var seeded bool
	for _, nd := range r.nodes {
		if nd == consumer {
			continue
		}
		if c, err := nd.Store().Get(bg(), firstParityOfStripe0); err == nil {
			if err := consumer.Store().Put(bg(), c); err != nil {
				t.Fatal(err)
			}
			seeded = true
			break
		}
	}
	if !seeded {
		t.Fatalf("rig: no node holds the parity shard to pre-seed")
	}
	psWithhold(t, r, psDataOfStripe(r, 0)[1])
	psRetain(t, r, consumer)
	if consumer.Stats.ParityShardsPulled != 0 || consumer.Stats.ParityColumnLookups != 0 {
		t.Fatalf("pulled %d over %d lookups; an already-held parity shard must settle the deficit with no transfer", consumer.Stats.ParityShardsPulled, consumer.Stats.ParityColumnLookups)
	}
}

// TestPSHeldParitySettlesOneOfTwoAndTheWalkContinues (G-PS-8; PE code ruling Open-1): a stripe
// that lost TWO data shards and already holds one parity shard has its first deficit settled by
// the held shard with nothing to ask column K for — `want` is empty while `remaining > 0` — and
// the walk MUST advance to column K+1 for the second. Ablation: replacing that advance with a
// finish leaves the stripe at 9 of 16 shards and the retrieval fails while every other gate
// stays green. Asserts exactly one lookup (column K+1) and one transfer.
func TestPSHeldParitySettlesOneOfTwoAndTheWalkContinues(t *testing.T) {
	r := newNetgetRig(t)
	consumer := psConsumer(t, r)
	per := r.m.N - r.m.K
	firstParityOfStripe0 := r.m.ParityIDs()[0*per+0]
	seeded := false
	for _, nd := range r.nodes {
		if nd == consumer {
			continue
		}
		if c, err := nd.Store().Get(bg(), firstParityOfStripe0); err == nil {
			if err := consumer.Store().Put(bg(), c); err != nil {
				t.Fatal(err)
			}
			seeded = true
			break
		}
	}
	if !seeded {
		t.Fatalf("rig: no node holds the parity shard to pre-seed")
	}
	data := psDataOfStripe(r, 0)
	psWithhold(t, r, data[1], data[7]) // deficit 2 on stripe 0
	psRetain(t, r, consumer)
	held := psParityHeld(r, consumer)
	if len(held) != 1 || len(held[0]) != 2 {
		t.Fatalf("parity held %v, want exactly two shards of stripe 0 (the pre-held one and one more)", held)
	}
	if consumer.Stats.ParityColumnLookups != 1 || consumer.Stats.ParityShardsPulled != 1 {
		t.Fatalf("lookups=%d pulled=%d, want 1/1: column K is settled by the held shard with no lookup, column K+1 supplies the second", consumer.Stats.ParityColumnLookups, consumer.Stats.ParityShardsPulled)
	}
}
