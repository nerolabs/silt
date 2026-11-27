package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

// countingStore counts reads, so a test can see which chunks a range touched.
type countingStore struct {
	ports.ChunkStore
	reads map[ports.ChunkID]int
}

func (c *countingStore) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	c.reads[id]++
	return c.ChunkStore.Get(ctx, id)
}

func publishForRange(t *testing.T, data []byte, mode crypto.Mode, params erasure.Params) (*countingStore, *manifest.Manifest) {
	t.Helper()
	ctx := context.Background()
	store := &countingStore{ChunkStore: memstore.New(), reads: map[ports.ChunkID]int{}}
	reg := registry.New()
	h, err := pipeline.Add(ctx, store, reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: testChunkSize, Mode: mode, Erasure: params, Rand: rand.New(rand.NewSource(7))})
	if err != nil {
		t.Fatal(err)
	}
	m, err := pipeline.OpenManifest(ctx, store, reg, h)
	if err != nil {
		t.Fatal(err)
	}
	return store, m
}

func readRange(t *testing.T, store ports.ChunkStore, m *manifest.Manifest, off, n int64) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := pipeline.ReadRange(context.Background(), store, m, off, n, &out); err != nil {
		t.Fatalf("ReadRange(%d, %d) of %d bytes: %v", off, n, m.FileSize, err)
	}
	return out.Bytes()
}

// TestReadRangeReturnsExactlyTheBytesAsked: every range of every file shape — empty, one
// padded frame, a frame boundary exactly, many stripes with a short last stripe and filler
// frames — in both modes, matches the same slice of the original, n clamped at the end.
func TestReadRangeReturnsExactlyTheBytesAsked(t *testing.T) {
	payload := int64(testChunkSize - chunk.HeaderSize)
	sizes := []int64{0, 1, 500, payload - 1, payload, payload + 1, 3 * payload, 37_000, 41*payload + 17}
	for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
		for _, size := range sizes {
			data := make([]byte, size)
			rand.New(rand.NewSource(size)).Read(data)
			store, m := publishForRange(t, data, mode, erasure.Params{K: 4, N: 7})
			if m.FileSize != size {
				t.Fatalf("manifest says %d bytes, published %d", m.FileSize, size)
			}
			rng := rand.New(rand.NewSource(size + 99))
			cases := [][2]int64{{0, size}, {size, 0}, {0, 0}}
			if size > 0 { // past the end clamps; on an empty file any byte is unsatisfiable, as in HTTP
				cases = append(cases, [2]int64{0, size + 50})
			}
			for k := 0; k < 40 && size > 0; k++ {
				off := rng.Int63n(size)
				cases = append(cases, [2]int64{off, 1 + rng.Int63n(size-off+3*payload)})
			}
			if size > 5*payload { // straddle frame and stripe boundaries on purpose
				cases = append(cases, [2]int64{payload - 1, 2}, [2]int64{4*payload - 3, 7}, [2]int64{payload, payload})
			}
			for _, c := range cases {
				got := readRange(t, store, m, c[0], c[1])
				want := data[c[0]:min(c[0]+c[1], size)]
				if !bytes.Equal(got, want) {
					t.Fatalf("%s %d B: range %d+%d returned %d bytes that differ from the original's %d", mode, size, c[0], c[1], len(got), len(want))
				}
			}
		}
	}
}

// TestReadRangeReadsOnlyTheChunksItCovers. A ranged read is only worth having if it does not
// cost the whole file: a range inside one frame must read that one data chunk and nothing
// else, and the file's other chunks can be gone entirely.
func TestReadRangeReadsOnlyTheChunksItCovers(t *testing.T) {
	payload := int64(testChunkSize - chunk.HeaderSize)
	data := make([]byte, 40*payload)
	rand.New(rand.NewSource(3)).Read(data)
	store, m := publishForRange(t, data, crypto.Convergent, erasure.Params{K: 4, N: 7})

	store.reads = map[ports.ChunkID]int{}
	got := readRange(t, store, m, 13*payload+5, 100)
	if !bytes.Equal(got, data[13*payload+5:13*payload+105]) {
		t.Fatal("wrong bytes")
	}
	if len(store.reads) != 1 || store.reads[m.ChunkIDs()[13]] != 1 {
		t.Fatalf("a range inside frame 13 read %d chunk(s), want only chunk 13: %v", len(store.reads), store.reads)
	}

	// Every chunk outside stripe 3 (chunks 12..15) gone: the range still reads.
	ctx := context.Background()
	for i, id := range m.ChunkIDs() {
		if i/4 != 3 {
			store.Delete(ctx, id)
		}
	}
	if got := readRange(t, store, m, 12*payload, 4*payload); !bytes.Equal(got, data[12*payload:16*payload]) {
		t.Fatal("a range needed other stripes, or read the wrong bytes, after they were deleted")
	}
}

// TestReadRangeRebuildsALostChunkFromItsStripe: a covered chunk that is gone, or corrupt, is
// rebuilt from its own stripe's parity, checked against the hash the root commits, and the
// range comes back exact.
func TestReadRangeRebuildsALostChunkFromItsStripe(t *testing.T) {
	ctx := context.Background()
	payload := int64(testChunkSize - chunk.HeaderSize)
	data := make([]byte, 30*payload+9)
	rand.New(rand.NewSource(5)).Read(data)
	for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
		store, m := publishForRange(t, data, mode, erasure.Params{K: 4, N: 7})
		ids := m.ChunkIDs()
		store.Delete(ctx, ids[9])                                        // lost
		store.Put(ctx, ports.Chunk{ID: ids[10], Data: []byte("not it")}) // corrupt under its own id
		got := readRange(t, store, m, 9*payload+3, payload+10)
		if !bytes.Equal(got, data[9*payload+3:10*payload+13]) {
			t.Fatalf("%s: a range over a lost and a corrupt chunk came back wrong", mode)
		}
		// The last stripe is short (chunks 28..30); lose one there too.
		store.Delete(ctx, ids[30])
		if got := readRange(t, store, m, 29*payload, payload+9); !bytes.Equal(got, data[29*payload:]) {
			t.Fatalf("%s: the short last stripe did not rebuild", mode)
		}
		// Past tolerance in stripe 0: the range there fails loudly, never returns wrong bytes.
		for _, id := range ids[0:4] {
			store.Delete(ctx, id)
		}
		var out bytes.Buffer
		if err := pipeline.ReadRange(ctx, store, m, 0, 10, &out); err == nil {
			t.Fatalf("%s: a stripe past tolerance returned %d bytes instead of an error", mode, out.Len())
		}
	}
}

func TestReadRangePastTheEndIsNotSatisfiable(t *testing.T) {
	data := []byte("a small file")
	store, m := publishForRange(t, data, crypto.Convergent, erasure.DefaultParams)
	var out bytes.Buffer
	for _, off := range []int64{int64(len(data)), int64(len(data)) + 1} {
		err := pipeline.ReadRange(context.Background(), store, m, off, 1, &out)
		if !errors.Is(err, pipeline.ErrRangeNotSatisfiable) {
			t.Fatalf("offset %d of a %d-byte file: err=%v, want ErrRangeNotSatisfiable", off, len(data), err)
		}
	}
	if err := pipeline.ReadRange(context.Background(), store, m, -1, 1, &out); err == nil {
		t.Fatal("a negative offset was accepted")
	}
}
