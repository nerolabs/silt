// Package diskplot persists a node's identity-bound storage-bond plot
// (core/bond) to disk, so a restarting daemon RELOADS its multi-block plot
// and re-verifies it against the committed root instead of re-plotting from
// scratch. Plotting is deliberately expensive — that expense is the
// Sybil cost — so paying it again on every restart would be both wasteful and,
// for a large pledge, a long stall before the validator can prove standing.
//
// One file per identity. The blocks are stored raw (they are already fixed-
// width, high-entropy bytes; a codec would only add overhead) behind a small
// header carrying the block geometry and the committed root, and written
// atomically (temp file + rename) so a crash mid-write never yields a
// half-plot that reads back as valid.
package diskplot

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nerolabs/silt/ports"
)

// magic/version guard the on-disk format; a mismatch is treated as "no plot"
// so an older or foreign file triggers a clean re-plot rather than a misread.
const (
	magic = 0x53504c54 // "SPLT"
	// version 3: the M0 Sybil fix G2 reseeds the plot from a PUBLIC, identity-
	// and size-bound seed H(pk, n) folded into every label, so a v2 plot's
	// blocks are labeled from the old private secret and would fail the new
	// labeling- consistency check. Bumping the format version makes an older
	// file read as "no plot" → a clean re-plot, so a restart never reloads an
	// insecure plot. (One- time fleet re-plot cost on upgrade.)
	version    = 3
	headerSize = 4 + 4 + 4 + 4 + 32 // magic, version, blockSize, nBlocks, root
)

// blockSize is the plot block width this store writes. It matches core/bond's
// BlockSize; the header carries it so a reader never assumes.
const blockSize = 4 << 10

type Store struct {
	root string
	// pending holds plots opened for streaming but not yet committed, so a seal
	// interrupted before CommitBlocks leaves the identity's previous plot intact.
	pending map[ports.NodeID]*plot
}

var _ ports.PlotBlockStore = (*Store)(nil)

var _ ports.PlotStore = (*Store)(nil)

// Open roots a plot store at dir, creating it if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("diskplot: %w", err)
	}
	return &Store{root: dir}, nil
}

func (s *Store) path(id ports.NodeID) string {
	return filepath.Join(s.root, hex.EncodeToString(id[:])+".plot")
}

// Save writes the plot atomically: a temp file fully flushed, then renamed
// over the destination, so a reader never observes a partial plot.
func (s *Store) Save(id ports.NodeID, root ports.Hash, blocks [][]byte) error {
	if len(blocks) == 0 {
		return fmt.Errorf("diskplot: refusing to save an empty plot")
	}
	bs := len(blocks[0])
	dst := s.path(id)
	tmp, err := os.CreateTemp(s.root, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	var hdr [headerSize]byte
	binary.BigEndian.PutUint32(hdr[0:], magic)
	binary.BigEndian.PutUint32(hdr[4:], version)
	binary.BigEndian.PutUint32(hdr[8:], uint32(bs))
	binary.BigEndian.PutUint32(hdr[12:], uint32(len(blocks)))
	copy(hdr[16:], root[:])
	if _, err := tmp.Write(hdr[:]); err != nil {
		tmp.Close()
		return err
	}
	for i, b := range blocks {
		if len(b) != bs {
			tmp.Close()
			return fmt.Errorf("diskplot: block %d is %d bytes, want a uniform %d", i, len(b), bs)
		}
		if _, err := tmp.Write(b); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// Load reads a persisted plot. ok is false (with nil error) when no plot
// exists or the file is unreadable/foreign — the caller then re-plots. A file
// whose length disagrees with its header is corrupt and reported as such.
func (s *Store) Load(id ports.NodeID) (root ports.Hash, blocks [][]byte, ok bool, err error) {
	raw, rerr := os.ReadFile(s.path(id))
	if rerr != nil {
		return root, nil, false, nil // absent (or unreadable): re-plot, not fatal
	}
	if len(raw) < headerSize {
		return root, nil, false, fmt.Errorf("diskplot: plot file shorter than its header")
	}
	if binary.BigEndian.Uint32(raw[0:]) != magic || binary.BigEndian.Uint32(raw[4:]) != version {
		return root, nil, false, nil // foreign/older format: re-plot cleanly
	}
	bs := int(binary.BigEndian.Uint32(raw[8:]))
	n := int(binary.BigEndian.Uint32(raw[12:]))
	copy(root[:], raw[16:headerSize])
	if bs <= 0 || n <= 0 {
		return ports.Hash{}, nil, false, fmt.Errorf("diskplot: bad geometry (blockSize=%d nBlocks=%d)", bs, n)
	}
	body := raw[headerSize:]
	if len(body) != bs*n {
		return ports.Hash{}, nil, false, fmt.Errorf("diskplot: body is %d bytes, header implies %d", len(body), bs*n)
	}
	blocks = make([][]byte, n)
	for i := 0; i < n; i++ {
		blocks[i] = append([]byte(nil), body[i*bs:(i+1)*bs]...)
	}
	return root, blocks, true, nil
}

// blockAt is where block i's bytes begin in a plot file of the given block size.
func blockAt(blockSize, i int) int64 { return int64(headerSize) + int64(i)*int64(blockSize) }

// plot is random access to one plot file's blocks. It is the streaming face of
// the same on-disk layout Save writes — a fixed header followed by uniform
// blocks — so a plot sealed block by block and one written all at once are the
// same bytes, and either can be read back by either path.
type plot struct {
	f         *os.File
	blockSize int
	n         int
}

func (p *plot) ReadBlock(i int, into []byte) error {
	if i < 0 || i >= p.n {
		return fmt.Errorf("diskplot: block %d out of range (%d blocks)", i, p.n)
	}
	if len(into) != p.blockSize {
		return fmt.Errorf("diskplot: read buffer is %d bytes, plot blocks are %d", len(into), p.blockSize)
	}
	_, err := p.f.ReadAt(into, blockAt(p.blockSize, i))
	return err
}

func (p *plot) WriteBlock(i int, b []byte) error {
	if i < 0 || i >= p.n {
		return fmt.Errorf("diskplot: block %d out of range (%d blocks)", i, p.n)
	}
	if len(b) != p.blockSize {
		return fmt.Errorf("diskplot: block %d is %d bytes, want a uniform %d", i, len(b), p.blockSize)
	}
	_, err := p.f.WriteAt(b, blockAt(p.blockSize, i))
	return err
}

// OpenBlocks prepares an n-block plot for id and returns random access to it.
//
// The bytes go to a temp file that only becomes the identity's plot at
// CommitBlocks, so a crash part-way through a seal leaves the previous plot
// intact rather than a half-plot that reads back as valid. The committed root is
// not known until every block is labeled, so the header is stamped with a zero
// root here and filled in at commit.
func (s *Store) OpenBlocks(id ports.NodeID, n int) (ports.PlotBlocks, error) {
	if n <= 0 {
		return nil, fmt.Errorf("diskplot: refusing to open a plot of %d blocks", n)
	}
	// Discard any earlier open for this identity. A seal that failed part-way — or a
	// process that died between opening and committing — leaves a temp file and a
	// half-written plot behind, and the next attempt is the moment to be rid of them
	// rather than accumulating one per try.
	if prev, ok := s.pending[id]; ok {
		name := prev.f.Name()
		prev.f.Close()
		os.Remove(name)
		delete(s.pending, id)
	}
	tmp, err := os.CreateTemp(s.root, ".tmp-*")
	if err != nil {
		return nil, err
	}
	var hdr [headerSize]byte
	binary.BigEndian.PutUint32(hdr[0:], magic)
	binary.BigEndian.PutUint32(hdr[4:], version)
	binary.BigEndian.PutUint32(hdr[8:], uint32(blockSize))
	binary.BigEndian.PutUint32(hdr[12:], uint32(n))
	if _, err := tmp.WriteAt(hdr[:], 0); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	if s.pending == nil {
		s.pending = map[ports.NodeID]*plot{}
	}
	p := &plot{f: tmp, blockSize: blockSize, n: n}
	s.pending[id] = p
	return p, nil
}

// CommitBlocks stamps the committed root into the header of the plot opened for
// id and moves it into place atomically.
//
// The caller is still holding this plot as the live source for its commitment — a
// bond exists to answer challenges, and it answers them by reading blocks. So the
// write handle is replaced with a read handle on the committed file rather than
// simply closed: closing it would leave the running node holding a bond whose
// blocks it can no longer read, which looks exactly like a validator that never
// earned standing.
func (s *Store) CommitBlocks(id ports.NodeID, root ports.Hash) error {
	p, ok := s.pending[id]
	if !ok {
		return fmt.Errorf("diskplot: no plot is open for this identity")
	}
	delete(s.pending, id)
	tmpName := p.f.Name()
	if _, err := p.f.WriteAt(root[:], 16); err != nil {
		p.f.Close()
		os.Remove(tmpName)
		return err
	}
	if err := p.f.Sync(); err != nil {
		p.f.Close()
		os.Remove(tmpName)
		return err
	}
	if err := p.f.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	dst := s.path(id)
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	rf, err := os.Open(dst)
	if err != nil {
		return fmt.Errorf("diskplot: committed plot cannot be reopened for reading: %w", err)
	}
	p.f = rf // the commitment holding this plot keeps reading, now from the committed file
	return nil
}

// LoadBlocks reopens a persisted plot for random access instead of reading it
// whole, so a restart re-verifies and answers from disk rather than paying the
// plot's size in memory. ok is false when no readable plot of this format exists.
func (s *Store) LoadBlocks(id ports.NodeID) (root ports.Hash, blocks ports.PlotBlocks, n int, ok bool, err error) {
	f, ferr := os.Open(s.path(id))
	if ferr != nil {
		return root, nil, 0, false, nil // absent: re-plot, not fatal
	}
	var hdr [headerSize]byte
	if _, rerr := f.ReadAt(hdr[:], 0); rerr != nil {
		f.Close()
		return root, nil, 0, false, nil
	}
	if binary.BigEndian.Uint32(hdr[0:]) != magic || binary.BigEndian.Uint32(hdr[4:]) != version {
		f.Close()
		return root, nil, 0, false, nil // foreign/older format: re-plot cleanly
	}
	bs := int(binary.BigEndian.Uint32(hdr[8:]))
	count := int(binary.BigEndian.Uint32(hdr[12:]))
	copy(root[:], hdr[16:headerSize])
	if bs <= 0 || count <= 0 {
		f.Close()
		return ports.Hash{}, nil, 0, false, fmt.Errorf("diskplot: bad geometry (blockSize=%d nBlocks=%d)", bs, count)
	}
	info, serr := f.Stat()
	if serr != nil {
		f.Close()
		return ports.Hash{}, nil, 0, false, serr
	}
	if want := blockAt(bs, count); info.Size() != want {
		f.Close()
		return ports.Hash{}, nil, 0, false, fmt.Errorf("diskplot: plot file is %d bytes, header implies %d", info.Size(), want)
	}
	return root, &plot{f: f, blockSize: bs, n: count}, count, true, nil
}
