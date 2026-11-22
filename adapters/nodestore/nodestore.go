// Package nodestore keeps a state tree's nodes on disk, so a validator's memory
// does not grow with every file ever published.
//
// The chain keeps its v5 state tree between blocks so a block's root costs one
// path per changed leaf. Held in memory, the tree's nodes cost about a kilobyte
// per published file, which a small validator runs out of within months at
// realistic load. A Store holds them in a bbolt file instead: the operating
// system's page cache keeps the hot part resident and gives the rest back under
// memory pressure, and the process holds only a bounded write buffer.
//
// The file is a cache, never a record. The tree is a function of the committed
// state, which the node rebuilds from its own chain log at every start, so a
// Store begins empty, belongs to one tree, and is deleted when the tree is
// closed or at the next start. Nothing is fsynced: a crash loses only a file
// that start discards. That is what makes the write path cheap enough to run
// on every block.
package nodestore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/pokt-network/smt/kvstore"
	bolt "go.etcd.io/bbolt"
)

// filePrefix names every store file, so a start can find what a crashed
// process left behind.
const filePrefix = "nodes-"

// DefaultBufferBytes is how much a Store buffers before it writes to disk. The
// SMT writes each path's new nodes and deletes its old ones on every commit, so
// the upper levels are replaced many times between flushes; a buffer that
// absorbs those churns turns most of them into nothing at all on disk.
const DefaultBufferBytes = 32 << 20

// flushChunk bounds how many keys one write transaction carries. Node keys are
// digests, so a flush's keys land on pages spread across the whole file, and
// bbolt holds every page a transaction dirties in memory until it commits: one
// transaction for the whole buffer would hold close to the whole file. A chunk
// this size dirties at most this many pages, whatever the file has grown to.
const flushChunk = 4096

var bucket = []byte("n")

// ErrNotFound is returned by Get for a key the store does not hold.
var ErrNotFound = errors.New("nodestore: key not found")

// ErrClosed is returned by every operation on a closed Store.
var ErrClosed = errors.New("nodestore: closed")

// Store is a kvstore.MapStore over a private bbolt file with a write-back
// buffer. Writes land in the buffer; reads see the buffer first and then the
// file. When the buffer passes its size it is written to the file in key
// order. A Store is not safe for concurrent use, like the tree that owns it.
//
// Like the SMT's own in-memory store, a Store keeps the slice Set is given and
// hands out its own slice from Get, so neither may be modified afterwards. The
// tree never modifies a node once encoded; copying each one in and out would
// cost a large share of the tree's time for nothing.
type Store struct {
	db   *bolt.DB
	path string
	file *fileHandle

	// pending holds writes not yet in the file: a value, or nil for a key the
	// file may hold and must lose.
	pending      map[string][]byte
	pendingBytes int
	limit        int

	// recent and older hold nodes recently read from the file. A block's root
	// is computed on a view of the tree more than once before the block is
	// applied (when it is proposed, checked, and committed), and each pass
	// walks the same paths; after the first, those reads come from here. When
	// recent fills it becomes older and the previous older is dropped, so a
	// node read in either of the last two generations stays in memory.
	recent, older map[string][]byte
	recentBytes   int
	cacheLimit    int

	closed bool
}

// fileHandle is what releasing a store's file needs. It is separate from the
// Store so a cleanup can release the file of a Store dropped without Close,
// and a leaked tree cannot leave a file behind for the life of the process.
type fileHandle struct {
	once sync.Once
	db   *bolt.DB
	path string
	read *bolt.Tx // a read transaction over the file as of the last flush
	err  error
}

func (h *fileHandle) release() error {
	h.once.Do(func() {
		if h.read != nil {
			h.read.Rollback() // Close waits for open transactions
			h.read = nil
		}
		err := h.db.Close()
		if rerr := os.Remove(h.path); rerr != nil && !os.IsNotExist(rerr) && err == nil {
			err = rerr
		}
		h.err = err
	})
	return h.err
}

// Option adjusts a Store.
type Option func(*Store)

// DefaultCacheBytes is how many bytes of recently read nodes a Store keeps in
// each of its two cache generations.
const DefaultCacheBytes = 8 << 20

// WithCacheBytes sets how many bytes of recently read nodes a Store keeps in
// each of its two cache generations; 0 keeps none.
func WithCacheBytes(n int) Option { return func(s *Store) { s.cacheLimit = n } }

// WithBufferBytes sets how many bytes of writes a Store buffers before it
// writes them to its file.
func WithBufferBytes(n int) Option { return func(s *Store) { s.limit = n } }

// Open creates an empty Store in a new file under dir. The file is the Store's
// alone, and Close removes it.
func Open(dir string, opts ...Option) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, filePrefix+"*.db")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	db, err := bolt.Open(path, 0o600, &bolt.Options{
		NoSync:         true,
		NoGrowSync:     true,
		NoFreelistSync: true,
		FreelistType:   bolt.FreelistMapType,
	})
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucket(bucket)
		return e
	}); err != nil {
		db.Close()
		os.Remove(path)
		return nil, err
	}
	s := &Store{db: db, path: path, file: &fileHandle{db: db, path: path},
		pending: map[string][]byte{}, limit: DefaultBufferBytes,
		recent: map[string][]byte{}, cacheLimit: DefaultCacheBytes}
	for _, o := range opts {
		o(s)
	}
	if err := s.beginRead(); err != nil {
		db.Close()
		os.Remove(path)
		return nil, err
	}
	runtime.AddCleanup(s, func(h *fileHandle) { h.release() }, s.file)
	return s, nil
}

// RemoveStale deletes the store files under dir. A Store's file outlives its
// process whenever the process ends without closing it, as a daemon stopped by
// a signal does, and is never read again; a node calls this at start, before
// it opens any Store there.
func RemoveStale(dir string) error {
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasPrefix(e.Name(), filePrefix) || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Path is the store's file.
func (s *Store) Path() string { return s.path }

func (s *Store) beginRead() error {
	tx, err := s.db.Begin(false)
	if err != nil {
		return err
	}
	s.file.read = tx
	return nil
}

func (s *Store) endRead() error {
	tx := s.file.read
	s.file.read = nil
	return tx.Rollback()
}

// onDisk returns the file's value for key, copied out of the mapping, which a
// flush may move.
func (s *Store) onDisk(key []byte) ([]byte, bool) {
	v := s.file.read.Bucket(bucket).Get(key)
	if v == nil {
		return nil, false
	}
	return bytes.Clone(v), true
}

// Get returns the value stored for key.
func (s *Store) Get(key []byte) ([]byte, error) {
	if s.closed {
		return nil, ErrClosed
	}
	if v, ok := s.pending[string(key)]; ok {
		if v == nil {
			return nil, ErrNotFound
		}
		return v, nil
	}
	if v, ok := s.recent[string(key)]; ok {
		return v, nil
	}
	if v, ok := s.older[string(key)]; ok {
		s.remember(string(key), v)
		return v, nil
	}
	if v, ok := s.onDisk(key); ok {
		s.remember(string(key), v)
		return v, nil
	}
	return nil, ErrNotFound
}

// remember keeps v, read from the file, in the cache's current generation.
func (s *Store) remember(k string, v []byte) {
	if s.cacheLimit <= 0 {
		return
	}
	if s.recentBytes >= s.cacheLimit {
		s.older, s.recent, s.recentBytes = s.recent, map[string][]byte{}, 0
	}
	s.recent[k] = v
	s.recentBytes += len(k) + len(v)
}

// forget drops k from the cache, which a write makes stale.
func (s *Store) forget(k string) {
	delete(s.recent, k)
	delete(s.older, k)
}

// Set stores value for key.
func (s *Store) Set(key, value []byte) error {
	if s.closed {
		return ErrClosed
	}
	k := string(key)
	s.forget(k)
	if old, buffered := s.pending[k]; buffered {
		s.pendingBytes -= len(k) + len(old)
	}
	s.pending[k] = value
	s.pendingBytes += len(k) + len(value)
	return s.maybeFlush()
}

// Delete removes key.
func (s *Store) Delete(key []byte) error {
	if s.closed {
		return ErrClosed
	}
	k := string(key)
	s.forget(k)
	old, buffered := s.pending[k]
	if buffered && old == nil {
		return nil // already deleted
	}
	if buffered {
		s.pendingBytes -= len(k) + len(old)
		if _, ok := s.onDisk(key); !ok {
			// Written and removed between flushes: the file never sees it.
			// This is where most of the SMT's churn in its upper levels ends.
			delete(s.pending, k)
			return nil
		}
	}
	// The file may hold key. Deleting a key it does not hold is harmless, and
	// cheaper than looking first.
	s.pending[k] = nil
	s.pendingBytes += len(k)
	return s.maybeFlush()
}

// Len is the number of keys held. It flushes the buffer and walks the file, so
// it costs time in proportion to the store's size; the tree never calls it.
func (s *Store) Len() int {
	if s.closed {
		return 0
	}
	if err := s.Flush(); err != nil {
		return 0
	}
	return s.file.read.Bucket(bucket).Stats().KeyN
}

// ClearAll removes every key.
func (s *Store) ClearAll() error {
	if s.closed {
		return ErrClosed
	}
	if err := s.endRead(); err != nil {
		return err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(bucket)
		return err
	}); err != nil {
		return err
	}
	s.pending, s.pendingBytes = map[string][]byte{}, 0
	s.recent, s.older, s.recentBytes = map[string][]byte{}, nil, 0
	return s.beginRead()
}

func (s *Store) maybeFlush() error {
	if s.pendingBytes < s.limit {
		return nil
	}
	return s.Flush()
}

// Flush writes the buffer to the file, in key order and in transactions of at
// most flushChunk keys. The file is a cache, so a flush need not be atomic: one
// that fails part way leaves the buffer whole, and writing it again is harmless.
func (s *Store) Flush() error {
	if s.closed {
		return ErrClosed
	}
	if len(s.pending) == 0 {
		return nil
	}
	keys := make([]string, 0, len(s.pending))
	for k := range s.pending {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	// bbolt grows its mapping inside a write transaction, which waits for every
	// open read transaction; the buffer is the only reader, so close it first.
	if err := s.endRead(); err != nil {
		return err
	}
	var err error
	for len(keys) > 0 && err == nil {
		chunk := keys[:min(flushChunk, len(keys))]
		keys = keys[len(chunk):]
		err = s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucket)
			for _, k := range chunk {
				var err error
				if v := s.pending[k]; v == nil {
					err = b.Delete([]byte(k))
				} else {
					err = b.Put([]byte(k), v)
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	if rerr := s.beginRead(); rerr != nil && err == nil {
		err = rerr
	}
	if err != nil {
		return fmt.Errorf("nodestore: flush: %w", err)
	}
	s.pending, s.pendingBytes = map[string][]byte{}, 0
	return nil
}

// Close releases the store and deletes its file.
func (s *Store) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.pending, s.recent, s.older = nil, nil, nil
	return s.file.release()
}

var _ kvstore.MapStore = (*Store)(nil)
