package smtspike

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
	"github.com/pokt-network/smt/kvstore"
)

// pebbleStore is the LSM candidate the PE ruling asked to measure alongside
// bbolt (RULING keystone-node-store Q1: "measure bbolt AND one tuned LSM in one
// floor-box run … pebble with a minimal cache/memtable"). Same spike status as
// boltStore: test-only, importable by nothing, deleted once the measurement
// picks a backend.
//
// TUNED FOR THE FLOOR BOX, deliberately. Pebble's defaults size its cache and
// memtables for a server; on a 2 GB box shared with a daemon those defaults are
// the OOM shape #596 disqualified. So the cache and memtable are pinned SMALL
// here — the measurement's job is to find out whether pebble's write-throughput
// advantage survives being squeezed into the floor box's memory, which is the
// only configuration that could ship.
//
// Same batching contract as boltStore: Set() buffers into a pebble.Batch,
// Flush() commits it once per block (one sync), Get() reads the batch first so
// read-your-writes holds within a block.
type pebbleStore struct {
	db      *pebble.DB
	cache   *pebble.Cache
	batch   *pebble.Batch
	pending map[string][]byte // mirror for read-your-writes + tombstones
	count   int
}

const (
	// Minimal, on purpose — see the type comment.
	pebbleCacheBytes  = 16 << 20 // 16 MiB block cache
	pebbleMemtableB   = 4 << 20  // 4 MiB memtable
	pebbleMemtableMax = 2        // cap concurrent memtables so RAM stays bounded
)

func newPebbleStore(dir string, sync bool) (*pebbleStore, error) {
	cache := pebble.NewCache(pebbleCacheBytes)
	opts := &pebble.Options{
		Cache:                       cache,
		MemTableSize:                pebbleMemtableB,
		MemTableStopWritesThreshold: pebbleMemtableMax,
		// Keep the WAL in BOTH modes. `sync` controls whether writes are
		// fsync'd (durability); it must NOT control whether a WAL exists.
		// Disabling the WAL and not fsyncing means the memtable never reaches
		// disk before Close, so a clean reopen finds nothing — an adapter bug
		// that also made the comparison unfair (bbolt NoSync still recovers on
		// a clean reopen). NoSync here = WAL written, not fsync'd = recoverable
		// across a clean process restart, which is exactly the reopen the
		// measurement exercises, and the same semantics as bbolt NoSync.
	}
	db, err := pebble.Open(filepath.Join(dir, "pebble"), opts)
	if err != nil {
		cache.Unref()
		return nil, err
	}
	return &pebbleStore{
		db:      db,
		cache:   cache,
		batch:   db.NewBatch(),
		pending: map[string][]byte{},
	}, nil
}

func (s *pebbleStore) Get(key []byte) ([]byte, error) {
	if v, ok := s.pending[string(key)]; ok {
		if v == nil {
			return nil, kvErrNotFound
		}
		return append([]byte(nil), v...), nil
	}
	v, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, kvErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), v...)
	closer.Close()
	return out, nil
}

func (s *pebbleStore) Set(key, value []byte) error {
	if _, live := s.pebLive(key); !live {
		s.count++
	}
	s.pending[string(key)] = append([]byte(nil), value...)
	return s.batch.Set(key, value, nil)
}

func (s *pebbleStore) Delete(key []byte) error {
	if _, live := s.pebLive(key); live {
		s.count--
	}
	s.pending[string(key)] = nil
	return s.batch.Delete(key, nil)
}

func (s *pebbleStore) pebLive(key []byte) ([]byte, bool) {
	if v, ok := s.pending[string(key)]; ok {
		return v, v != nil
	}
	v, closer, err := s.db.Get(key)
	if err != nil {
		return nil, false
	}
	out := append([]byte(nil), v...)
	closer.Close()
	return out, true
}

// Flush commits the batch — one block's worth of writes — in one operation.
func (s *pebbleStore) Flush(sync bool) error {
	if s.batch.Count() == 0 {
		s.pending = map[string][]byte{}
		return nil
	}
	opts := pebble.NoSync
	if sync {
		opts = pebble.Sync
	}
	if err := s.db.Apply(s.batch, opts); err != nil {
		return err
	}
	s.batch.Close()
	s.batch = s.db.NewBatch()
	s.pending = map[string][]byte{}
	return nil
}

func (s *pebbleStore) Len() int { return s.count }

func (s *pebbleStore) ClearAll() error {
	s.batch.Close()
	s.batch = s.db.NewBatch()
	s.pending = map[string][]byte{}
	s.count = 0
	return s.db.DeleteRange([]byte{0x00}, []byte{0xff, 0xff, 0xff, 0xff}, pebble.Sync)
}

func (s *pebbleStore) Close() error {
	s.batch.Close()
	err := s.db.Close()
	s.cache.Unref()
	return err
}

// onDiskBytes sums the pebble directory — LSM stores spread across many SST
// files, so this walks the tree rather than stat-ing one file.
func (s *pebbleStore) onDiskBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(filepath.Join(dir, "pebble"), func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

var _ kvstore.MapStore = (*pebbleStore)(nil)
