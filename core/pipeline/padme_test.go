package pipeline_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// TestPaddedLengthIsPadme pins the Padmé rule against values worked by hand from the
// definition (E = ⌊log2 L⌋, S = ⌊log2 E⌋ + 1, clear the low E − S bits upwards) and
// checks its three properties over a sweep: it never shrinks, it is idempotent (a class
// representative is its own class), and its overhead stays under 12 %.
func TestPaddedLengthIsPadme(t *testing.T) {
	for _, c := range []struct{ in, want int64 }{
		{0, 0}, {1, 1}, {2, 2}, {3, 3}, {9, 10}, {17, 18}, {111, 112}, {119, 120},
		{1000, 1024}, {1008, 1024}, {1032, 1088}, {3968, 3968}, {3969, 4096},
		{262144, 262144}, {262145, 270336}, {600000, 606208}, {1 << 30, 1 << 30},
		{1<<30 + 1, 1<<30 + 1<<25},
	} {
		if got := pipeline.PaddedLength(c.in); got != c.want {
			t.Fatalf("PaddedLength(%d) = %d, want %d", c.in, got, c.want)
		}
	}
	classes := map[int64]bool{}
	for n := int64(1); n < 1<<20; n += 1 + n/257 {
		p := pipeline.PaddedLength(n)
		if p < n {
			t.Fatalf("PaddedLength(%d) = %d shrinks", n, p)
		}
		if pipeline.PaddedLength(p) != p {
			t.Fatalf("PaddedLength(%d) = %d is not its own class", n, p)
		}
		if float64(p-n) > 0.12*float64(n) {
			t.Fatalf("PaddedLength(%d) = %d: overhead %.1f %% exceeds Padmé's bound", n, p, 100*float64(p-n)/float64(n))
		}
		classes[p] = true
	}
	// Padmé leaks O(log log n) bits: below 2^20 there are a few hundred classes, not a
	// million lengths. The exact count is derivable; the bound is what matters.
	if len(classes) > 600 {
		t.Fatalf("%d distinct padded lengths below 2^20 — the rule is not coarsening", len(classes))
	}
}

// TestFillerFramesHideTheContentFrameCount drives the multi-frame half of the padding. Two
// different objects in one size class store the SAME number of data frames even though
// their content fills a different number; the filler frames round-trip to nothing; and in
// convergent mode the fillers are deterministic in the content (dedup holds) yet shared by
// no other object (an all-zero filler would encrypt to one chunk everywhere and give away
// the content frame count of every object that carried it).
func TestFillerFramesHideTheContentFrameCount(t *testing.T) {
	ctx := context.Background()
	const cs = 4096
	// At 4,088 payload bytes a frame, 131,073 B fills 33 content frames, but its Padmé
	// class (135,168 B) needs 34: one filler. 135,000 B is in the same class and fills 34
	// content frames: no filler. Both must store 34.
	stage := func(size int, seed uint64, mode crypto.Mode) (pipeline.Options, []byte) {
		return pipeline.Options{ChunkSize: cs, Mode: mode, Rand: rand.Reader}, rtPayload(size, seed)
	}
	for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
		counts := map[int]int{}
		for _, size := range []int{131073, 135000} {
			opts, payload := stage(size, uint64(size), mode)
			store := memstore.New()
			h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(payload), opts)
			if err != nil {
				t.Fatalf("%s %d: stage: %v", mode, size, err)
			}
			m, err := pipeline.LoadFull(ctx, store, entry, h)
			if err != nil {
				t.Fatalf("%s %d: load: %v", mode, size, err)
			}
			counts[size] = len(m.Chunks)
			var out bytes.Buffer
			if err := pipeline.Get(ctx, store, memRegistry{entry: entry}, h, &out); err != nil || !bytes.Equal(out.Bytes(), payload) {
				t.Fatalf("%s %d: round trip: err=%v, got %d of %d bytes", mode, size, err, out.Len(), len(payload))
			}
		}
		if counts[131073] != 34 || counts[135000] != 34 {
			t.Fatalf("%s: frame counts %v, want 34 for both members of the 135,168 class", mode, counts)
		}
	}

	// Convergent: same content, same root (dedup survives the fillers).
	_, a := stage(131073, 1, crypto.Convergent)
	ra, ia := stageConv(t, a, cs)
	rb, _ := stageConv(t, a, cs)
	if ra != rb {
		t.Fatal("the same content staged twice yields two roots: the filler is not deterministic in the content")
	}
	// Convergent: two different objects with a filler each share no data chunk at all.
	_, b := stage(131073, 2, crypto.Convergent)
	_, ib := stageConv(t, b, cs)
	for id := range ia {
		if ib[id] {
			t.Fatalf("two different objects share data chunk %x — a filler (or a frame) is not unique to its content", id[:8])
		}
	}
}

// stageConv stages payload in convergent mode and returns its root and the set of its
// data chunk IDs.
func stageConv(t *testing.T, payload []byte, cs int) (ports.Hash, map[ports.ChunkID]bool) {
	t.Helper()
	ctx := context.Background()
	store := memstore.New()
	h, entry, err := pipeline.Stage(ctx, store, bytes.NewReader(payload),
		pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	m, err := pipeline.LoadFull(ctx, store, entry, h)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ids := map[ports.ChunkID]bool{}
	for _, id := range m.ChunkIDs() {
		ids[id] = true
	}
	return h.Root, ids
}
