package diskproofs

import (
	"errors"
	"os"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func proofFor(i int) (ports.ChunkID, ports.StorageProof) {
	id := ports.ChunkID(ports.HashBytes([]byte{byte(i), byte(i >> 8), 7}))
	return id, ports.StorageProof{Root: ports.HashBytes([]byte{byte(i), 1}), Index: i, Total: 1000, Column: i % 16,
		Path: []ports.Hash{ports.HashBytes([]byte{byte(i)})}}
}

// TestTheMetaIndexStaysTheProofFilesTruth: no index until one is written; after that it
// follows every Put and Delete across a reopen; and a read reconciles it with the files —
// a proof whose record was lost to a crash is found, a record whose file is gone is
// dropped, and a torn final record is ignored.
func TestTheMetaIndexStaysTheProofFilesTruth(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		id, p := proofFor(i)
		s.Put(id, p)
	}
	if _, err := s.Metas(); !errors.Is(err, ports.ErrNoMetaIndex) {
		t.Fatalf("before any index was written: err=%v, want ErrNoMetaIndex", err)
	}
	full := map[ports.ChunkID]ports.ProofMeta{}
	for i := 0; i < 50; i++ {
		id, p := proofFor(i)
		full[id] = ports.ProofMeta{Root: p.Root, Index: p.Index, Total: p.Total, Column: p.Column}
	}
	if err := s.WriteMetaIndex(full); err != nil {
		t.Fatal(err)
	}
	// Kept current: 10 more, 5 deleted, then a reopen.
	for i := 50; i < 60; i++ {
		id, p := proofFor(i)
		s.Put(id, p)
	}
	for i := 0; i < 5; i++ {
		id, _ := proofFor(i)
		s.Delete(id)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s2.Metas()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 55 {
		t.Fatalf("index holds %d proofs, want 55", len(m))
	}
	id7, p7 := proofFor(7)
	if got := m[id7]; got.Root != p7.Root || got.Index != 7 || got.Column != 7 {
		t.Fatalf("proof 7's metadata came back wrong: %+v", got)
	}
	if _, ok := m[func() ports.ChunkID { id, _ := proofFor(2); return id }()]; ok {
		t.Fatal("a deleted proof is still in the index")
	}

	// Crash shapes: a proof file with no record, a record with no file, a torn tail.
	lost, pl := proofFor(500)
	s2.log.Close()
	s2.log = nil // writes now skip the log, as after a crash between the file and its record
	s2.Put(lost, pl)
	ghost, _ := proofFor(9)
	os.Remove(s2.path(ghost)) // the file vanished; its record is still in the log
	f, _ := os.OpenFile(s2.metaPath(), os.O_WRONLY|os.O_APPEND, 0o644)
	f.Write([]byte{opPut, 1, 2, 3}) // a torn final record
	f.Close()
	m, err = s2.Metas()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m[lost]; !ok {
		t.Fatal("a proof whose record was lost is missing from the reconciled index")
	}
	if _, ok := m[ghost]; ok {
		t.Fatal("a record whose proof file is gone survived reconciliation")
	}
	if len(m) != 55 {
		t.Fatalf("reconciled index holds %d proofs, want 55 (55 - 1 vanished + 1 recovered)", len(m))
	}
	// The read compacted and reopened the log: the next Put is recorded again.
	extra, pe := proofFor(600)
	s2.Put(extra, pe)
	s3, _ := Open(dir)
	if m, _ := s3.Metas(); len(m) != 56 {
		t.Fatalf("after compaction the log stopped recording: %d proofs, want 56", len(m))
	}
}
