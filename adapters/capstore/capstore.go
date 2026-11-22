// Package capstore bounds any ChunkStore to a fixed byte budget — the
// storage a daemon pledges to the network ("-capacity 2G"). Puts that
// would exceed the pledge are refused with ErrStoreFull; the network's
// spill-over placement then simply tries the next node. Deletes reclaim
// budget. Opening over an existing store (a restarted daemon's disk)
// re-measures what's already there.
package capstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/nerolabs/silt/ports"
)

type Store struct {
	mu    sync.Mutex
	inner ports.ChunkStore
	total int64
	used  int64
	sizes map[ports.ChunkID]int64
}

var (
	_ ports.ChunkStore       = (*Store)(nil)
	_ ports.CapacityReporter = (*Store)(nil)
)

// sizer is a chunk store that can report every chunk's size without reading it.
type sizer interface {
	Sizes(ctx context.Context) (map[ports.ChunkID]int64, error)
}

// Open wraps inner with a budget of total bytes, measuring existing
// contents first. A store that can report sizes without reading its chunks is
// measured that way: reading every stored chunk at start cost a storing node
// time in proportion to everything it holds, 46 minutes for 1.5 GB on a small
// cloud disk, before it could serve or rejoin consensus. Measured by size, a
// chunk that would fail its hash still counts until it is removed, as it still
// occupies the disk.
func Open(inner ports.ChunkStore, total int64) (*Store, error) {
	s := &Store{inner: inner, total: total, sizes: make(map[ports.ChunkID]int64)}
	if sz, ok := inner.(sizer); ok {
		sizes, err := sz.Sizes(context.Background())
		if err != nil {
			return nil, fmt.Errorf("capstore: measuring existing store: %w", err)
		}
		for id, n := range sizes {
			s.sizes[id] = n
			s.used += n
		}
		return s, nil
	}
	ids, err := inner.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("capstore: measuring existing store: %w", err)
	}
	for _, id := range ids {
		c, err := inner.Get(context.Background(), id)
		if err != nil {
			continue // unreadable chunk doesn't count against the pledge
		}
		s.sizes[id] = int64(len(c.Data))
		s.used += int64(len(c.Data))
	}
	return s, nil
}

func (s *Store) Capacity() (used, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used, s.total
}

func (s *Store) Put(ctx context.Context, c ports.Chunk) error {
	s.mu.Lock()
	if _, have := s.sizes[c.ID]; !have && s.used+int64(len(c.Data)) > s.total {
		s.mu.Unlock()
		return fmt.Errorf("capstore put %s (%d bytes, %d/%d used): %w",
			c.ID, len(c.Data), s.used, s.total, ports.ErrStoreFull)
	}
	s.mu.Unlock()
	if err := s.inner.Put(ctx, c); err != nil {
		return err
	}
	s.mu.Lock()
	if _, have := s.sizes[c.ID]; !have {
		s.sizes[c.ID] = int64(len(c.Data))
		s.used += int64(len(c.Data))
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) Delete(ctx context.Context, id ports.ChunkID) error {
	if err := s.inner.Delete(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	if sz, have := s.sizes[id]; have {
		s.used -= sz
		delete(s.sizes, id)
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	return s.inner.Get(ctx, id)
}

func (s *Store) Has(ctx context.Context, id ports.ChunkID) (bool, error) {
	return s.inner.Has(ctx, id)
}

func (s *Store) List(ctx context.Context) ([]ports.ChunkID, error) {
	return s.inner.List(ctx)
}
