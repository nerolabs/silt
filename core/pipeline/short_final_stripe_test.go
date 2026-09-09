package pipeline_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// storedBytes is what the object actually costs on disk: every data and parity shard.
func storedBytes(t *testing.T, store ports.ChunkStore, ids []ports.ChunkID) (total int, sizes map[int]int) {
	t.Helper()
	sizes = map[int]int{}
	for _, id := range ids {
		c, err := store.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("chunk %s: %v", id, err)
		}
		total += len(c.Data)
		sizes[len(c.Data)]++
	}
	return total, sizes
}

// TestSingleFrameObjectIsFramedAtItsTrueLength is R-SHORT-FINAL-STRIPE. A single-frame
// object is alone in its erasure stripe, so nothing forces it to full length: it and its
// six parity shards are computed at the frame's true length, and the geometry travels in
// manifest.ChunkSize. At the 256 KiB default a 1 KB object stored 7 × 262,160 =
// 1,835,120 B of mostly zeros (the erasure floor the Economist priced at +300 %); it now
// stores 7 × 1,048 = 7,336 B.
//
// Ablation that must go RED: split the file at opts.ChunkSize again (chunk.Split in
// Stage), or keep manifest.ChunkSize = opts.ChunkSize.
func TestSingleFrameObjectIsFramedAtItsTrueLength(t *testing.T) {
	ctx := context.Background()
	store := memstore.New()
	data := bytes.Repeat([]byte{0x5c}, 1024)
	h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(data), pipeline.Options{
		ChunkSize: pipeline.DefaultChunkSize, Mode: crypto.Private, Rand: rand.Reader,
	})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	m, err := pipeline.LoadFull(ctx, store, entry, h)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	frame := len(data) + chunk.HeaderSize
	if len(m.Chunks) != 1 {
		t.Fatalf("a 1 KB object made %d data chunks, want 1", len(m.Chunks))
	}
	if m.ChunkSize != int64(frame) {
		t.Fatalf("manifest.ChunkSize = %d, want the TRUE frame length %d — the geometry the auditor reads must be the one that was used", m.ChunkSize, frame)
	}
	total, sizes := storedBytes(t, store, append(m.ChunkIDs(), m.ParityIDs()...))
	shard := frame + crypto.Overhead
	if len(sizes) != 1 || sizes[shard] != erasure.DefaultParams.N-erasure.DefaultParams.K+1 {
		t.Fatalf("shard sizes %v, want %d shards of exactly %d B", sizes, erasure.DefaultParams.N-erasure.DefaultParams.K+1, shard)
	}
	if want := 7 * shard; total != want {
		t.Fatalf("stored %d B, want %d", total, want)
	}
	// The floor this removes: the padded geometry was 7 × (DefaultChunkSize + overhead).
	if padded := 7 * (pipeline.DefaultChunkSize + crypto.Overhead); total*100 > padded {
		t.Fatalf("stored %d B is not below 1 %% of the padded %d B — the erasure floor is still there", total, padded)
	}
	// It still round-trips.
	var out bytes.Buffer
	reg := memRegistry{entry: entry}
	if err := pipeline.Get(ctx, store, reg, h, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("round-trip returned %d bytes, want %d", out.Len(), len(data))
	}
}

// TestMultiFrameObjectsKeepThePaddedTail is the control arm and the blast-radius bound:
// the moment an object has two frames they SHARE a stripe, equal-length shards are forced
// again, and every byte — chunk IDs, root, stored size — is what it was before
// R-SHORT-FINAL-STRIPE. Only single-frame objects re-address.
func TestMultiFrameObjectsKeepThePaddedTail(t *testing.T) {
	ctx := context.Background()
	const cs = 4096
	for _, size := range []int{cs - chunk.HeaderSize, cs - chunk.HeaderSize + 1, 3*(cs-chunk.HeaderSize) + 17} {
		store := memstore.New()
		data := bytes.Repeat([]byte{0xA5}, size)
		h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(data), pipeline.Options{
			ChunkSize: cs, Mode: crypto.Convergent,
		})
		if err != nil {
			t.Fatalf("size %d: stage: %v", size, err)
		}
		m, err := pipeline.LoadFull(ctx, store, entry, h)
		if err != nil {
			t.Fatalf("size %d: load: %v", size, err)
		}
		if m.ChunkSize != cs {
			t.Fatalf("size %d: manifest.ChunkSize = %d, want the full %d — only a SINGLE frame goes short", size, m.ChunkSize, cs)
		}
		_, sizes := storedBytes(t, store, append(m.ChunkIDs(), m.ParityIDs()...))
		if len(sizes) != 1 || sizes[cs+crypto.Overhead] == 0 {
			t.Fatalf("size %d: shard sizes %v, want every shard at %d B", size, sizes, cs+crypto.Overhead)
		}
		_ = h
	}
}

// memRegistry answers exactly one entry — enough for pipeline.Get.
type memRegistry struct{ entry ports.Entry }

func (r memRegistry) Publish(context.Context, ports.Entry) error { return nil }
func (r memRegistry) All(context.Context) ([]ports.Entry, error) {
	return []ports.Entry{r.entry}, nil
}
func (r memRegistry) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	if root == r.entry.Root {
		return r.entry, true, nil
	}
	return ports.Entry{}, false, nil
}
