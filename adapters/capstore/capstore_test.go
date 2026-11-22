package capstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/cachestore"
	"github.com/nerolabs/silt/adapters/capstore"
	"github.com/nerolabs/silt/adapters/diskstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/storetest"
	"github.com/nerolabs/silt/ports"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) ports.ChunkStore {
		s, err := capstore.Open(memstore.New(), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestBudgetEnforcedAndReclaimed(t *testing.T) {
	ctx := context.Background()
	s, err := capstore.Open(memstore.New(), 100)
	if err != nil {
		t.Fatal(err)
	}
	c1 := ports.NewChunk(make([]byte, 60))
	c2raw := make([]byte, 60)
	c2raw[0] = 1
	c2 := ports.NewChunk(c2raw)

	if err := s.Put(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if used, total := s.Capacity(); used != 60 || total != 100 {
		t.Fatalf("capacity %d/%d, want 60/100", used, total)
	}
	if err := s.Put(ctx, c2); !errors.Is(err, ports.ErrStoreFull) {
		t.Fatalf("over-budget put: want ErrStoreFull, got %v", err)
	}
	if err := s.Put(ctx, c1); err != nil { // re-put of held chunk: free
		t.Fatalf("idempotent re-put charged the budget: %v", err)
	}
	if err := s.Delete(ctx, c1.ID); err != nil {
		t.Fatal(err)
	}
	if used, _ := s.Capacity(); used != 0 {
		t.Fatalf("used %d after delete, want 0", used)
	}
	if err := s.Put(ctx, c2); err != nil {
		t.Fatalf("put after reclaim: %v", err)
	}
}

func TestReopenMeasuresExisting(t *testing.T) {
	ctx := context.Background()
	inner := memstore.New()
	c := ports.NewChunk(make([]byte, 40))
	if err := inner.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	s, err := capstore.Open(inner, 100)
	if err != nil {
		t.Fatal(err)
	}
	if used, _ := s.Capacity(); used != 40 {
		t.Fatalf("reopen measured %d used, want 40", used)
	}
}

// readCounting is a disk store that counts chunk reads.
type readCounting struct {
	*diskstore.Store
	reads int
}

func (r *readCounting) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	r.reads++
	return r.Store.Get(ctx, id)
}

// Reopening over a disk store measures what it holds by size, without reading a
// chunk: a storing node's start must not cost a read of everything it stores.
// The measure is the same one reading every chunk gives.
func TestReopenOverADiskStoreReadsNoChunk(t *testing.T) {
	ctx := context.Background()
	disk, err := diskstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var want int64
	for i := 0; i < 50; i++ {
		c := ports.NewChunk(append(make([]byte, 100+i), byte(i)))
		if err := disk.Put(ctx, c); err != nil {
			t.Fatal(err)
		}
		want += int64(len(c.Data))
	}
	counted := &readCounting{Store: disk}
	s, err := capstore.Open(counted, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if used, _ := s.Capacity(); used != want {
		t.Fatalf("measured %d bytes, the store holds %d", used, want)
	}
	if counted.reads != 0 {
		t.Fatalf("opening read %d chunks to measure them", counted.reads)
	}
}

// A read cache between the budget and the disk does not bring the reads back.
func TestReopenThroughACacheReadsNoChunk(t *testing.T) {
	ctx := context.Background()
	disk, err := diskstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := ports.NewChunk(make([]byte, 300))
	if err := disk.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	counted := &readCounting{Store: disk}
	s, err := capstore.Open(cachestore.Open(counted, 1<<20), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if used, _ := s.Capacity(); used != 300 || counted.reads != 0 {
		t.Fatalf("measured %d bytes with %d chunk reads, want 300 with none", used, counted.reads)
	}
}
