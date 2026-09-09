package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
)

// TestAuditorAndHonestProverAgreeOnEveryShardLength is the coupling R-SHORT-FINAL-STRIPE
// puts under load, RUN rather than described. The auditor fixes the PoR sample space
// itself — want = Blocks(m.ChunkSize + ctOverhead), demanded EXACTLY of every leaf
// (blocksOK, red-team F4) — while an honest prover reports len(tags), which is
// Blocks(the shard it actually holds). Those two must be the same number for every
// object, including one framed at its true length.
//
// If the short frame did not travel in manifest.ChunkSize, this is what would break: the
// auditor would demand 65 blocks of a 1,048-byte shard, blocksOK would refuse every
// honest holder, and every sub-chunk object would read as lost. That is a silent audit
// failure, not a compile error, which is why it is gated here.
//
// Ablation that must go RED: set manifest.ChunkSize back to opts.ChunkSize in Stage.
func TestAuditorAndHonestProverAgreeOnEveryShardLength(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name      string
		size      int
		chunkSize int
		wantShort bool
	}{
		{"single frame, framed short", 1024, pipeline.DefaultChunkSize, true},
		{"two frames, tail padded", 4096, 2048, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			store := memstore.New()
			h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(bytes.Repeat([]byte{0x11}, c.size)),
				pipeline.Options{ChunkSize: c.chunkSize, Mode: crypto.Private, Rand: rand.Reader})
			if err != nil {
				t.Fatalf("stage: %v", err)
			}
			m, err := pipeline.LoadFull(ctx, store, entry, h)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			// The auditor's number, computed exactly as auditEntry computes it.
			want := por.DefaultParams.Blocks(int(m.ChunkSize) + ctOverhead)
			if want < 1 {
				t.Fatalf("the auditor would demand %d blocks", want)
			}
			porKey := DerivePorKey(h.LayoutKey())
			for i, id := range append(m.ChunkIDs(), m.ParityIDs()...) {
				ck, err := store.Get(ctx, id)
				if err != nil {
					t.Fatalf("leaf %d: %v", i, err)
				}
				if got := len(ck.Data); got != int(m.ChunkSize)+ctOverhead {
					t.Fatalf("leaf %d is %d B, but the auditor sizes it at %d — the committed geometry is not the stored one", i, got, int(m.ChunkSize)+ctOverhead)
				}
				leaf := id
				honest := len(porKey.Tags(leaf[:], ck.Data))
				if !blocksOK(honest, want) {
					t.Fatalf("leaf %d: an HONEST prover reports %d blocks and the auditor demands %d — every holder of this object would fail its audit", i, honest, want)
				}
			}
			// Both regimes are genuinely driven: one case commits a short frame, the
			// other the full chunk size. (What the short frame must BE is
			// TestSingleFrameObjectIsFramedAtItsTrueLength's claim, in core/pipeline.)
			if short := m.ChunkSize < int64(c.chunkSize); short != c.wantShort {
				t.Fatalf("manifest.ChunkSize = %d against a %d-byte chunk size; short=%v, want %v", m.ChunkSize, c.chunkSize, short, c.wantShort)
			}
		})
	}
}
