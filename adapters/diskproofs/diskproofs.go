// Package diskproofs persists a ports.StorageProof per hosted chunk as a
// small CBOR sidecar, mirroring the diskstore's object layout:
//
//	<root>/<first two hex chars>/<full hex chunk id>
//
// It is the durable half of the #69 fix: after a restart the node reloads
// these proofs so it can re-announce each coded shard under the right column
// key (and still answer storage-audit challenges). Writes are atomic
// (temp file + rename), like the object store.
package diskproofs

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

type Store struct{ root string }

var _ ports.ProofStore = (*Store)(nil)

// Open roots the proof tree at dir (created if absent).
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("diskproofs: %w", err)
	}
	return &Store{root: dir}, nil
}

// diskProof is the on-disk shape: hashes as byte strings (compact), fields
// keyed by int so the encoding is stable and small.
type diskProof struct {
	Root    []byte   `cbor:"1,keyasint"`
	Index   int      `cbor:"2,keyasint"`
	Total   int      `cbor:"3,keyasint"`
	Path    [][]byte `cbor:"4,keyasint"`
	Column  int      `cbor:"5,keyasint"`
	PorTags [][]byte `cbor:"6,keyasint,omitempty"`
}

func (s *Store) path(id ports.ChunkID) string {
	h := hex.EncodeToString(id[:])
	return filepath.Join(s.root, h[:2], h)
}

func (s *Store) Put(id ports.ChunkID, p ports.StorageProof) error {
	dp := diskProof{
		Root:   append([]byte(nil), p.Root[:]...),
		Index:  p.Index,
		Total:  p.Total,
		Column: p.Column,
	}
	for _, h := range p.Path {
		dp.Path = append(dp.Path, append([]byte(nil), h[:]...))
	}
	// PoR authenticators must survive a restart: without them a re-announced
	// shard can't answer an audit and its honest host is wrongly slashed (#69
	// / Gate 4a). One 32-byte tag per por-block.
	for _, tag := range p.PorTags {
		dp.PorTags = append(dp.PorTags, append([]byte(nil), tag...))
	}
	b, err := cbor.Marshal(dp)
	if err != nil {
		return fmt.Errorf("diskproofs marshal %s: %w", id, err)
	}
	dst := s.path(id)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func (s *Store) Delete(id ports.ChunkID) error {
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// decode turns a sidecar's CBOR bytes into a StorageProof. ok is false for an
// unreadable/corrupt/short encoding — the caller skips it (a chunk with an
// undecodable proof just re-announces under its bare id, better than refusing).
func decode(b []byte) (ports.StorageProof, bool) {
	var dp diskProof
	if cbor.Unmarshal(b, &dp) != nil || len(dp.Root) != 32 {
		return ports.StorageProof{}, false
	}
	p := ports.StorageProof{Index: dp.Index, Total: dp.Total, Column: dp.Column}
	copy(p.Root[:], dp.Root)
	for _, hb := range dp.Path {
		if len(hb) != 32 {
			continue
		}
		var h ports.Hash
		copy(h[:], hb)
		p.Path = append(p.Path, h)
	}
	for _, tag := range dp.PorTags {
		p.PorTags = append(p.PorTags, append([]byte(nil), tag...))
	}
	return p, true
}

// idFromName recovers a ChunkID from a sidecar filename, rejecting stray temp
// files / non-proof entries (the 64-hex-char naming is the filter).
func idFromName(name string) (ports.ChunkID, bool) {
	if len(name) != 64 {
		return ports.ChunkID{}, false
	}
	raw, herr := hex.DecodeString(name)
	if herr != nil || len(raw) != 32 {
		return ports.ChunkID{}, false
	}
	var id ports.ChunkID
	copy(id[:], raw)
	return id, true
}

// Get reads one proof sidecar. ok is false if the id has no proof stored (or its
// sidecar is unreadable/corrupt — treated as absent, same as Load skips it). One
// file read: this is the per-id page the bounded proof cache does on a miss.
func (s *Store) Get(id ports.ChunkID) (ports.StorageProof, bool, error) {
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return ports.StorageProof{}, false, nil
		}
		return ports.StorageProof{}, false, err
	}
	p, ok := decode(b)
	return p, ok, nil
}

// Keys walks the two-level tree and returns every stored chunk id, so a restart
// can repopulate resident proof metadata one Get at a time (bounded RAM) instead
// of loading the whole store into a map.
func (s *Store) Keys() ([]ports.ChunkID, error) {
	var out []ports.ChunkID
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if id, ok := idFromName(d.Name()); ok {
			out = append(out, id)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("diskproofs keys: %w", err)
	}
	return out, nil
}

// Load reads every sidecar back into a map. A single unreadable/corrupt proof
// is skipped, not fatal — the chunk just re-announces under its bare id until
// re-hosted, which is strictly better than refusing to start.
func (s *Store) Load() (map[ports.ChunkID]ports.StorageProof, error) {
	out := make(map[ports.ChunkID]ports.StorageProof)
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		id, ok := idFromName(d.Name())
		if !ok {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if p, ok := decode(b); ok {
			out[id] = p
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("diskproofs load: %w", err)
	}
	return out, nil
}
