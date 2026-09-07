package pipeline_test

// Decision 4′ (2026-09-07): the manifest is framed at TRUE length — a sealed manifest
// smaller than a chunk is one frame of exactly len(blob) + chunk.HeaderSize bytes, with
// zero padding, and round-trips through LoadBlob unchanged; a manifest larger than a chunk
// keeps the data chunk size. Ablation: split the manifest at opts.ChunkSize again ⇒ RED
// (the frame is 262,144 bytes for a 1.4 KB manifest).

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/pipeline"
)

func TestManifestIsFramedAtTrueLength(t *testing.T) {
	store := memstore.New()
	data := make([]byte, 3<<10) // a 3 KiB file: one padded data chunk, one tiny manifest
	rand.Read(data)
	_, entry, err := pipeline.Stage(context.Background(), store, bytes.NewReader(data), pipeline.Options{ChunkSize: pipeline.DefaultChunkSize, Mode: crypto.Convergent, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.ManifestChunks) != 1 {
		t.Fatalf("a small manifest split into %d frames, want 1", len(entry.ManifestChunks))
	}
	c, err := store.Get(context.Background(), entry.ManifestChunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Data) >= pipeline.DefaultChunkSize {
		t.Fatalf("the manifest frame is %d bytes — padded to the data chunk size (the R-MANIFEST-PADDING waste)", len(c.Data))
	}
	blob, err := pipeline.LoadBlob(context.Background(), store, entry)
	if err != nil {
		t.Fatalf("a true-length manifest frame did not round-trip: %v", err)
	}
	if len(c.Data) != len(blob)+chunk.HeaderSize {
		t.Fatalf("frame %d bytes for a %d-byte blob: padding %d, want exactly the header", len(c.Data), len(blob), len(c.Data)-len(blob)-chunk.HeaderSize)
	}
	if pipeline.ManifestFrameSize(1_400, pipeline.DefaultChunkSize) != 1_408 || pipeline.ManifestFrameSize(1<<20, pipeline.DefaultChunkSize) != pipeline.DefaultChunkSize {
		t.Fatal("ManifestFrameSize: a 1.4 KB blob frames at 1,408 B; a blob above the chunk size frames at the chunk size")
	}
}

// TestDefaultChunkSizeIs256KiB pins the ratified default (D-R2.9-NODE-HALF-CALLS 4′). The
// alignment with the delivery increment is pinned in cmd/silt, the package that imports both.
func TestDefaultChunkSizeIs256KiB(t *testing.T) {
	if pipeline.DefaultChunkSize != 262_144 {
		t.Fatalf("DefaultChunkSize %d, want 262,144 — a moved default changes every new publish's root; re-ratify", pipeline.DefaultChunkSize)
	}
}
