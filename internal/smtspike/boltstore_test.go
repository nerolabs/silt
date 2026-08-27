package smtspike

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/pokt-network/smt/kvstore"
	bolt "go.etcd.io/bbolt"
)

// boltStore is a disk-backed kvstore.MapStore for the floor-box measurement of
// the SMT node store (PE ruling keystone-node-store, Q1). It is a SPIKE, not
// the product store: the product NodeStore lands with the keystone build behind
// ports.NodeStore, once this measurement picks the backend.
//
// THE LOAD-BEARING DESIGN POINT — write batching. The SMT calls Set() once per
// dirty node during Commit(); a naive adapter that opened one bbolt write
// transaction per Set would fsync per node, turning a 100-changed-key block into
// hundreds of fsyncs and making the measurement meaningless. Instead Set()
// buffers into a pending map and Flush() commits the whole block in ONE bbolt
// transaction (one fsync). That mirrors how the real integration must work: one
// store flush per committed block. Get() checks the pending buffer first so a
// node written earlier in the same block is visible before the flush.
type boltStore struct {
	db      *bolt.DB
	pending map[string][]byte // nil value = tombstone
	count   int               // live key count, maintained across flushes
}

var bucket = []byte("smt")

var kvErrNotFound = errors.New("smtspike: key not found")

func newBoltStore(dir string, noSync bool) (*boltStore, error) {
	path := filepath.Join(dir, "smt.db")
	db, err := bolt.Open(path, 0600, &bolt.Options{
		// NoSync trades crash-durability for speed. The real store fsyncs once
		// per block (markstore-class); the spike measures BOTH so the fsync cost
		// is visible rather than hidden.
		NoSync: noSync,
	})
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists(bucket)
		return e
	}); err != nil {
		db.Close()
		return nil, err
	}
	return &boltStore{db: db, pending: map[string][]byte{}}, nil
}

func (s *boltStore) Get(key []byte) ([]byte, error) {
	if v, ok := s.pending[string(key)]; ok {
		if v == nil {
			return nil, kvErrNotFound
		}
		return append([]byte(nil), v...), nil
	}
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucket).Get(key)
		if v == nil {
			return kvErrNotFound
		}
		out = append([]byte(nil), v...)
		return nil
	})
	return out, err
}

func (s *boltStore) Set(key, value []byte) error {
	k := string(key)
	if _, existed := s.liveLookup(key); !existed {
		s.count++
	}
	s.pending[k] = append([]byte(nil), value...)
	return nil
}

func (s *boltStore) Delete(key []byte) error {
	if _, existed := s.liveLookup(key); existed {
		s.count--
	}
	s.pending[string(key)] = nil // tombstone
	return nil
}

// liveLookup reports whether key is currently live (pending non-tombstone, or
// on disk and not pending-tombstoned).
func (s *boltStore) liveLookup(key []byte) ([]byte, bool) {
	if v, ok := s.pending[string(key)]; ok {
		return v, v != nil
	}
	v, err := s.getDisk(key)
	return v, err == nil
}

func (s *boltStore) getDisk(key []byte) ([]byte, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucket).Get(key)
		if v == nil {
			return kvErrNotFound
		}
		out = append([]byte(nil), v...)
		return nil
	})
	return out, err
}

// Flush commits all buffered writes in one transaction — one block's worth.
func (s *boltStore) Flush() error {
	if len(s.pending) == 0 {
		return nil
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for k, v := range s.pending {
			if v == nil {
				if e := b.Delete([]byte(k)); e != nil {
					return e
				}
				continue
			}
			if e := b.Put([]byte(k), v); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.pending = map[string][]byte{}
	return nil
}

func (s *boltStore) Len() int { return s.count }

func (s *boltStore) ClearAll() error {
	s.pending = map[string][]byte{}
	s.count = 0
	return s.db.Update(func(tx *bolt.Tx) error {
		if e := tx.DeleteBucket(bucket); e != nil {
			return e
		}
		_, e := tx.CreateBucket(bucket)
		return e
	})
}

func (s *boltStore) Close() error { return s.db.Close() }

// onDiskBytes returns the size of the backing file — the on-disk cost the
// measurement reports.
func (s *boltStore) onDiskBytes() (int64, error) {
	fi, err := os.Stat(s.db.Path())
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

var _ kvstore.MapStore = (*boltStore)(nil)
