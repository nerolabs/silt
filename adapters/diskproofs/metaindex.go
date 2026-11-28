package diskproofs

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/nerolabs/silt/ports"
)

// The metadata index is an append-only log beside the proof files: one fixed-size record per
// Put and a tombstone per Delete. A restart reads it in one sequential pass instead of opening
// every proof file. It is written in full once (WriteMetaIndex, after a node's first full
// reload), kept current by every Put and Delete after that, and compacted on each read. It is
// a CACHE of the proof files, never the authority: each read reconciles it against the
// directory listing, so a record lost to a crash costs one proof read, and a record whose
// file is gone is dropped.

const (
	metaLogName = "meta.log"
	opPut       = 1
	opDel       = 2
	recordSize  = 1 + 32 + 32 + 4 + 4 + 4
)

func (s *Store) metaPath() string { return filepath.Join(s.root, metaLogName) }

func encodeRecord(op byte, id ports.ChunkID, m ports.ProofMeta) []byte {
	b := make([]byte, recordSize)
	b[0] = op
	copy(b[1:33], id[:])
	copy(b[33:65], m.Root[:])
	binary.BigEndian.PutUint32(b[65:69], uint32(int32(m.Index)))
	binary.BigEndian.PutUint32(b[69:73], uint32(int32(m.Total)))
	binary.BigEndian.PutUint32(b[73:77], uint32(int32(m.Column)))
	return b
}

// appendMeta records a Put or Delete when the index exists. Called with s.mu held.
func (s *Store) appendMeta(op byte, id ports.ChunkID, m ports.ProofMeta) {
	if s.log == nil {
		return
	}
	if _, err := s.log.Write(encodeRecord(op, id, m)); err != nil {
		// A lost record is repaired by the next read's reconciliation; stop appending so the
		// log is never written out of order after a failed write.
		s.log.Close()
		s.log = nil
	}
}

// Metas returns every proof's metadata: the log, reconciled against the proof files present,
// then compacted. ErrNoMetaIndex until the index has been written once.
func (s *Store) Metas() (map[ports.ChunkID]ports.ProofMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.metaPath())
	if os.IsNotExist(err) {
		return nil, ports.ErrNoMetaIndex
	}
	if err != nil {
		return nil, err
	}
	logged := map[ports.ChunkID]ports.ProofMeta{}
	r := bufio.NewReaderSize(f, 1<<20)
	rec := make([]byte, recordSize)
	for {
		if _, err := io.ReadFull(r, rec); err != nil {
			break // EOF, or a torn final record from a crash: reconciliation covers it
		}
		var id ports.ChunkID
		copy(id[:], rec[1:33])
		switch rec[0] {
		case opPut:
			var m ports.ProofMeta
			copy(m.Root[:], rec[33:65])
			m.Index = int(int32(binary.BigEndian.Uint32(rec[65:69])))
			m.Total = int(int32(binary.BigEndian.Uint32(rec[69:73])))
			m.Column = int(int32(binary.BigEndian.Uint32(rec[73:77])))
			logged[id] = m
		case opDel:
			delete(logged, id)
		}
	}
	f.Close()

	keys, err := s.keysLocked()
	if err != nil {
		return nil, err
	}
	out := make(map[ports.ChunkID]ports.ProofMeta, len(keys))
	for _, id := range keys {
		if m, ok := logged[id]; ok {
			out[id] = m
			continue
		}
		// In the directory, not in the log (a crash between a proof write and its record):
		// read just this one.
		p, ok, gerr := s.getLocked(id)
		if gerr != nil || !ok {
			continue
		}
		out[id] = ports.ProofMeta{Root: p.Root, Index: p.Index, Total: p.Total, Column: p.Column}
	}
	if err := s.writeIndexLocked(out); err != nil {
		return nil, err
	}
	return out, nil
}

// WriteMetaIndex records a complete picture — what a node holds resident after a full reload
// — and starts keeping it current. It replaces any index already present.
func (s *Store) WriteMetaIndex(m map[ports.ChunkID]ports.ProofMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeIndexLocked(m)
}

func (s *Store) writeIndexLocked(m map[ports.ChunkID]ports.ProofMeta) error {
	tmp, err := os.CreateTemp(s.root, ".meta-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriterSize(tmp, 1<<20)
	for id, meta := range m {
		if _, err := w.Write(encodeRecord(opPut, id, meta)); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if s.log != nil {
		s.log.Close()
		s.log = nil
	}
	if err := os.Rename(tmp.Name(), s.metaPath()); err != nil {
		return err
	}
	return s.openLogLocked()
}

func (s *Store) openLogLocked() error {
	f, err := os.OpenFile(s.metaPath(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	s.log = f
	return nil
}

var _ ports.ProofMetaIndex = (*Store)(nil)
