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

// TestDataFrameSizeIsWhatStageCommits drives the two halves of ONE rule against each
// other: DataFrameSize decides from a LENGTH, splitFile decides from a STREAM that cannot
// know the length ahead, and the committed manifest.ChunkSize must be what the first says
// for every object. It also fixes the re-addressing boundary, which the first published
// statement of this change got wrong by one byte (blind PE B-1, 2026-09-09): an object of
// exactly chunkSize − HeaderSize FILLS the first frame and is unchanged; the largest
// object that re-addresses is chunkSize − HeaderSize − 1.
func TestDataFrameSizeIsWhatStageCommits(t *testing.T) {
	ctx := context.Background()
	const cs = 4096
	// LITERAL expectations, not DataFrameSize applied to itself: the table is the
	// independent statement of the rule and both the helper and Stage are checked
	// against it. cs − HeaderSize = 4088 is the largest object that fills one frame.
	for _, c := range []struct{ size, wantFrame int }{
		{0, cs},      // no frames at all: no chunk, no shard, the asked-for geometry
		{1, 9},       // the minimum frame
		{9, 17},      //
		{1000, 1008}, //
		{4086, 4094}, //
		{4087, 4095}, // the LARGEST object that re-addresses: cs − HeaderSize − 1
		{4088, cs},   // fills the first frame exactly — unchanged
		{4089, cs},   // two frames
		{cs, cs},     //
		{2 * cs, cs},
		{3*cs + 17, cs},
	} {
		store := memstore.New()
		h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(bytes.Repeat([]byte{0x3c}, c.size)),
			pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent})
		if err != nil {
			t.Fatalf("size %d: stage: %v", c.size, err)
		}
		m, err := pipeline.LoadFull(ctx, store, entry, h)
		if err != nil {
			t.Fatalf("size %d: load: %v", c.size, err)
		}
		if m.ChunkSize != int64(c.wantFrame) {
			t.Fatalf("size %d: Stage committed ChunkSize %d, want %d", c.size, m.ChunkSize, c.wantFrame)
		}
		// No excused row: size 0 is checked here too. Carving it out is exactly how the
		// helper came to disagree with Stage at 0 while this gate stayed green.
		if got := pipeline.DataFrameSize(c.size, cs); got != c.wantFrame {
			t.Fatalf("size %d: DataFrameSize says %d, Stage commits %d — the length rule and the stream rule disagree", c.size, got, c.wantFrame)
		}
	}
	if pipeline.DataFrameSize(1, cs) != chunk.MinChunkSize {
		t.Fatalf("a 1-byte object does not frame at the minimum %d — manifest.ChunkSize can legitimately be that small", chunk.MinChunkSize)
	}
}

// TestConvergentDedupNowSpansTheChunkSize records a PRIVACY consequence of the short final
// stripe that no source named until the blind PE found it (N-1, 2026-09-09), and records
// it as a measurement so it cannot be argued away later. Padding used to make a sub-frame
// object's ciphertext — and so its chunk ID, root and link key — depend on the publisher's
// -chunk-size. At true length it does not: the same bytes published at 64 KiB, 256 KiB and
// 1 MiB now yield ONE root.
//
// Dedup improves. So does the confirmation attack that cmd/silt already warns about for
// convergent mode: chunk size was an accidental salt and is no longer one, so a guesser
// needs the plaintext alone rather than the plaintext AND the geometry. silt claims no
// size hiding (docs/threat-catalog.md F3) and this is NOT a mitigation — inventing a salt
// here would be exactly the unreviewed novelty B8 forbids. A red-team pass is owed.
func TestConvergentDedupNowSpansTheChunkSize(t *testing.T) {
	ctx := context.Background()
	roots := map[string]int{}
	for _, cs := range []int{64 << 10, 256 << 10, 1 << 20} {
		store := memstore.New()
		h, _, err := pipeline.Stage(ctx, store, bytes.NewReader(bytes.Repeat([]byte{0x7e}, 4096)),
			pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent})
		if err != nil {
			t.Fatalf("chunk size %d: %v", cs, err)
		}
		roots[string(h.Root[:])]++
	}
	if len(roots) != 1 {
		t.Fatalf("the same 4,096-byte payload produced %d distinct roots across three chunk sizes; the short final stripe means it must produce ONE — if this is RED the salt is back and the F3 note needs re-reading", len(roots))
	}
	// A multi-frame object still depends on the geometry, which bounds the finding.
	multi := map[string]int{}
	for _, cs := range []int{4096, 8192} {
		store := memstore.New()
		h, _, err := pipeline.Stage(ctx, store, bytes.NewReader(bytes.Repeat([]byte{0x7e}, 100_000)),
			pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent})
		if err != nil {
			t.Fatalf("chunk size %d: %v", cs, err)
		}
		multi[string(h.Root[:])]++
	}
	if len(multi) != 2 {
		t.Fatalf("a 100,000-byte multi-frame object collapsed to %d root(s) across two chunk sizes, want 2 — the finding is scoped to SUB-FRAME objects", len(multi))
	}
}
