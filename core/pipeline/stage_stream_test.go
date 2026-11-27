package pipeline_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// TestStageOutputIsUnchangedByStreaming. Stage now reads one frame and codes one stripe at a
// time. The bytes it produces must not move: a changed root breaks every existing link and
// convergent dedup, a changed shard set breaks placement and audit. testdata holds the
// outputs of the pre-streaming implementation for 52 shapes (empty, single padded frame,
// every frame boundary, short last stripe, filler frames) in both modes.
func TestStageOutputIsUnchangedByStreaming(t *testing.T) {
	f, err := os.Open("testdata/stage_golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cases := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		var mode string
		var cs, size int
		var want string
		if _, err := fmt.Sscanf(line, "%s %d %d %s", &mode, &cs, &size, &want); err != nil {
			t.Fatalf("golden line %q: %v", line, err)
		}
		data := make([]byte, size)
		rand.New(rand.NewSource(int64(size))).Read(data)
		st := memstore.New()
		h, e, err := pipeline.Stage(context.Background(), st, bytes.NewReader(data),
			pipeline.Options{ChunkSize: cs, Mode: crypto.Mode(mode), Erasure: erasure.Params{K: 4, N: 7}, Rand: rand.New(rand.NewSource(9))})
		if err != nil {
			t.Fatal(err)
		}
		ids, _ := st.List(context.Background())
		keys := make([]string, 0, len(ids))
		for _, id := range ids {
			keys = append(keys, id.String())
		}
		sort.Strings(keys)
		got := fmt.Sprintf("GOLDEN %s %d %d root=%s key=%x fs=%d manifest=%v stored=%x", mode, cs, size, h.Root, h.Key, e.FileSize, e.ManifestChunks, keys)
		if fmt.Sprintf("%x", sha256.Sum256([]byte(got))) != want {
			t.Errorf("%s cs=%d size=%d: Stage produced different bytes than before", mode, cs, size)
		}
		cases++
	}
	if cases != 52 {
		t.Fatalf("checked %d golden cases, want 52", cases)
	}
}

// readCounter counts how much of the source Stage has consumed.
type readCounter struct {
	r io.Reader
	n int64
}

func (c *readCounter) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.n += int64(k)
	return k, err
}

// firstParity records how much input had been read when the first parity shard was stored.
type firstParity struct {
	ports.ChunkStore
	src  *readCounter
	data map[ports.ChunkID]bool
	at   int64
}

func (f *firstParity) Put(ctx context.Context, c ports.Chunk) error {
	if f.at < 0 && !f.data[c.ID] {
		f.at = f.src.n
	}
	return f.ChunkStore.Put(ctx, c)
}

// TestStageCodesAStripeBeforeReadingTheNext: the first stripe's parity is stored after Stage
// has read about one stripe of input — not after the whole file — which is what lets a
// 2.4 GB film publish on a 2 GB box.
func TestStageCodesAStripeBeforeReadingTheNext(t *testing.T) {
	const cs, k = 4096, 4
	payload := int64(cs - chunk.HeaderSize)
	data := make([]byte, 200*payload)
	rand.New(rand.NewSource(1)).Read(data)

	// The data chunk ids of the first stripe, from a reference staging, tell data puts apart
	// from parity puts.
	ref := memstore.New()
	_, e, err := pipeline.Stage(context.Background(), ref, bytes.NewReader(data), pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent, Erasure: erasure.Params{K: k, N: 7}})
	if err != nil {
		t.Fatal(err)
	}
	_ = e
	all, _ := ref.List(context.Background())
	src := &readCounter{r: bytes.NewReader(data)}
	st := &firstParity{ChunkStore: memstore.New(), src: src, data: map[ports.ChunkID]bool{}, at: -1}
	// Mark every chunk a convergent split of the data produces as data, by content.
	frames, _ := chunk.Split(bytes.NewReader(data), cs)
	for _, fr := range frames {
		ct, _, _ := crypto.ConvergentEncrypt(fr)
		st.data[ports.NewChunk(ct).ID] = true
	}
	if len(all) == 0 || len(st.data) == 0 {
		t.Fatal("CONTROL: nothing staged")
	}
	if _, _, err := pipeline.Stage(context.Background(), st, src, pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent, Erasure: erasure.Params{K: k, N: 7}}); err != nil {
		t.Fatal(err)
	}
	if st.at < 0 {
		t.Fatal("no parity shard was stored")
	}
	if limit := int64(k+2) * payload; st.at > limit {
		t.Fatalf("the first parity shard was stored after %d bytes of input had been read (the file is %d); a streaming Stage stores it within about one stripe (%d)", st.at, len(data), limit)
	}
}

// heapProbe discards what it is given and, every so often, measures the live heap.
type heapProbe struct {
	puts int
	base uint64
	max  uint64
}

func (h *heapProbe) Put(_ context.Context, c ports.Chunk) error {
	if h.puts++; h.puts%64 == 0 {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.HeapAlloc > h.base && ms.HeapAlloc-h.base > h.max {
			h.max = ms.HeapAlloc - h.base
		}
	}
	return nil
}
func (h *heapProbe) Get(context.Context, ports.ChunkID) (ports.Chunk, error) {
	return ports.Chunk{}, ports.ErrNotFound
}
func (h *heapProbe) Has(context.Context, ports.ChunkID) (bool, error) { return false, nil }
func (h *heapProbe) Delete(context.Context, ports.ChunkID) error      { return nil }
func (h *heapProbe) List(context.Context) ([]ports.ChunkID, error)    { return nil, nil }

// zeroReader is an endless source that allocates nothing.
type zeroReader struct{ left int64 }

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.left <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > z.left {
		n = z.left
	}
	clear(p[:n])
	z.left -= n
	return int(n), nil
}

// TestStageHoldsOneStripeNotTheFile measures the live heap while Stage publishes a 48 MB file
// in both modes. A streaming Stage holds a stripe and the manifest's ids; the manifest once
// kept c.ID[:], which pinned every chunk's ciphertext (617 MB live staging 512 MB).
func TestStageHoldsOneStripeNotTheFile(t *testing.T) {
	if testing.Short() {
		t.Skip("measures the heap over a 48 MB publish")
	}
	const size = 48 << 20
	for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		probe := &heapProbe{base: ms.HeapAlloc}
		if _, _, err := pipeline.Stage(context.Background(), probe, &zeroReader{left: size},
			pipeline.Options{ChunkSize: 64 << 10, Mode: mode, Rand: rand.New(rand.NewSource(1))}); err != nil {
			t.Fatal(err)
		}
		if probe.puts < 700 {
			t.Fatalf("CONTROL: only %d chunks staged", probe.puts)
		}
		if limit := uint64(12 << 20); probe.max > limit {
			t.Fatalf("%s: Stage held %d MB live publishing a %d MB file; streaming holds about one stripe (limit %d MB)", mode, probe.max>>20, size>>20, limit>>20)
		}
		t.Logf("%s: max live heap while staging %d MB: %.1f MB", mode, size>>20, float64(probe.max)/(1<<20))
	}
}
