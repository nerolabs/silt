// Package chainstore persists a chain replica to disk so a restarted validator
// daemon rejoins with its history. On load it re-verifies every block's
// cryptographic integrity — hashes and signatures — because trusting your own
// disk is still trusting; but it does NOT re-gate our own committed history on
// the live reputation view (which is empty at boot), or a restart would be
// stranded at genesis (F1). Reputation is re-earned live via bond audits;
// catch-up on blocks missed while down is a separate, peer-facing path
// (Node.SyncChain).
//
// The file is an append-only log: a magic header, then one frame per block,
// each a 4-byte big-endian payload length, the block's canonical CBOR, and a
// CRC-32C of the payload. A commit appends its block's frame and fsyncs, so
// persisting a block costs its own size, not the chain's. Rewriting the whole
// chain on every commit cost each block time and disk bandwidth in proportion to
// the chain's whole history, on the validator's event loop. The whole file is
// rewritten, atomically, only when the chain no longer extends what the file
// holds (a reorg), and when appends have doubled it since the last rewrite, so
// heavy bond proofs the replica has since shed (PruneBelowHorizon) leave the disk
// too, at an amortized cost per block that stays constant.
//
// A file written before the log format is a single CBOR array of blocks. Load
// still reads it, and the first save rewrites it as a log.
package chainstore

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// logMagic opens every log-format file. A legacy file opens with a CBOR array
// header (major type 4), so the two can never be mistaken for each other.
var logMagic = []byte("SILTCLG1")

// maxFrame bounds one frame's declared payload length. A length beyond it can
// only be a corrupted header, so it refuses the file rather than reading the
// rest of the file as a torn final frame.
const maxFrame = 256 << 20

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// ErrCorruptFrame reports a complete frame whose payload fails its checksum or
// whose declared length is impossible: the disk returned different bytes than
// were written.
var ErrCorruptFrame = errors.New("chainstore: corrupt frame")

// Load reads blocks from path; a missing file is an empty history.
//
// A log whose final frame is incomplete holds an append that a crash
// interrupted before its fsync returned; the commit it carried was never
// durable, so Load returns the blocks before it and no error, exactly as a crash
// before the old whole-file rename left the previous file. A complete frame that
// fails its checksum or does not decode is an error, returned with the blocks
// before it.
func Load(path string) ([]chain.Block, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(raw, logMagic) {
		return chain.DecodeBlocks(raw)
	}
	var blocks []chain.Block
	for off := len(logMagic); off < len(raw); {
		if len(raw)-off < 4 {
			break // torn length header
		}
		n := int(binary.BigEndian.Uint32(raw[off:]))
		if n > maxFrame {
			return blocks, fmt.Errorf("%w: frame %d declares %d bytes", ErrCorruptFrame, len(blocks), n)
		}
		end := off + 4 + n + 4
		if end > len(raw) {
			break // torn final frame
		}
		payload := raw[off+4 : off+4+n]
		if crc32.Checksum(payload, crcTable) != binary.BigEndian.Uint32(raw[off+4+n:]) {
			return blocks, fmt.Errorf("%w: frame %d fails its checksum", ErrCorruptFrame, len(blocks))
		}
		b, err := chain.Decode(payload)
		if err != nil {
			return blocks, fmt.Errorf("frame %d: %w", len(blocks), err)
		}
		blocks = append(blocks, *b)
		off = end
	}
	return blocks, nil
}

// appendFrames appends one frame per block to buf.
func appendFrames(buf []byte, blocks []chain.Block) []byte {
	for i := range blocks {
		payload := chain.Encode(&blocks[i])
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(payload)))
		buf = append(buf, payload...)
		buf = binary.BigEndian.AppendUint32(buf, crc32.Checksum(payload, crcTable))
	}
	return buf
}

// Save writes the full chain as a log DURABLY and atomically: the bytes go to a
// temp file in the same directory, are fsync'd, the temp file is closed, renamed
// over path, and the directory is fsync'd so the rename itself is durable — the
// markstore pattern. A reader sees the old chain or the new one, never a mixture.
func Save(path string, blocks []chain.Block) error {
	_, err := save(path, blocks)
	return err
}

func save(path string, blocks []chain.Block) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".chain-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	fail := func(err error) (int64, error) {
		tmp.Close()
		os.Remove(tmpName)
		return 0, err
	}
	data := appendFrames(append([]byte(nil), logMagic...), blocks)
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		df.Close()
	}
	return int64(len(data)), nil
}

// Writer keeps a chain replica's file in step with the replica, appending the
// blocks committed since its last Sync, and reads blocks back from it: it is the
// chain's archive (chain.BlockArchive), which lets the chain release old blocks
// from memory. It is not safe for concurrent use; the daemon calls it from its
// event loop.
type Writer struct {
	path string
	// f is the file, open for reading and writing, and offs the offset of each
	// readable frame in it, one per block from genesis. Both stay valid for
	// reading after a failed write; only a rewrite replaces them.
	f    *os.File
	offs []int64
	// held is how many blocks the file holds for appending to, and head the hash
	// of the last of them. held is -1 when the next Sync must rewrite the file
	// whatever it holds: before the first Sync or Recover, after a failed write,
	// and after a legacy file is read.
	held int
	head ports.Hash
	// size is the length of the file's valid frames; compactAt is twice its
	// length after the last rewrite, past which the next Sync rewrites instead of
	// appending.
	size, compactAt int64
	// heavy lists, in height order, the frames the file holds that carry heavy
	// bond proofs, with the bytes of proof each carries.
	heavy []heavyFrame
}

// heavyFrame is a frame written with heavy bond proofs: its height and the bytes
// of proof a prune would take out of it.
type heavyFrame struct {
	height uint64
	bytes  int64
}

// proofBytes is how many bytes of heavy bond proof b carries.
func proofBytes(b *chain.Block) int64 {
	var n int64
	for i := range b.BondRegs {
		n += int64(len(b.BondRegs[i].Answer))
	}
	return n
}

// noteHeavy records b's frame if it carries heavy proofs.
func (w *Writer) noteHeavy(b *chain.Block) {
	if b.CarriesHeavyProofs() {
		w.heavy = append(w.heavy, heavyFrame{height: b.Height, bytes: proofBytes(b)})
	}
}

// shedOutweighs reports whether the proofs c has shed that the file still carries
// are a quarter of the file or more. Past that point a rewrite takes them out of
// the file; below it a file that is mostly live history is not rewritten for each
// proof the horizon passes, so the cost of rewriting stays proportional to what it
// reclaims. A prune sheds every heavy block below its floor, so the shed frames are
// a prefix of heavy.
func (w *Writer) shedOutweighs(c *chain.Chain) bool {
	var shed int64
	for _, f := range w.heavy {
		if c.CarriesHeavyProofsAt(f.height) {
			break
		}
		shed += f.bytes
	}
	return 4*shed >= w.size
}

// NewWriter returns a Writer for path. It touches nothing until the first Sync
// or Recover.
func NewWriter(path string) *Writer { return &Writer{path: path, held: -1} }

// releaseEvery is how many replayed blocks pass between the replay's chances to
// shed heavy proofs and release old blocks, so a restart never holds the whole
// history in memory at once.
const releaseEvery = 256

// Durable is how many blocks, from genesis, the file holds as the chain last
// synced them; 0 when the next Sync will rewrite it.
func (w *Writer) Durable() uint64 {
	if w.held < 0 {
		return 0
	}
	return uint64(w.held)
}

// Block reads the block at height back from the file.
func (w *Writer) Block(height uint64) (chain.Block, error) {
	if w.f == nil || height >= uint64(len(w.offs)) {
		return chain.Block{}, fmt.Errorf("chainstore: block %d is not in %s", height, w.path)
	}
	off := w.offs[height]
	var hdr [4]byte
	if _, err := w.f.ReadAt(hdr[:], off); err != nil {
		return chain.Block{}, err
	}
	n := int(binary.BigEndian.Uint32(hdr[:]))
	if n > maxFrame {
		return chain.Block{}, fmt.Errorf("%w: block %d declares %d bytes", ErrCorruptFrame, height, n)
	}
	buf := make([]byte, n+4)
	if _, err := w.f.ReadAt(buf, off+4); err != nil {
		return chain.Block{}, err
	}
	if crc32.Checksum(buf[:n], crcTable) != binary.BigEndian.Uint32(buf[n:]) {
		return chain.Block{}, fmt.Errorf("%w: block %d fails its checksum", ErrCorruptFrame, height)
	}
	b, err := chain.Decode(buf[:n])
	if err != nil {
		return chain.Block{}, fmt.Errorf("block %d: %w", height, err)
	}
	return *b, nil
}

// Sync makes the file hold c's committed chain. When c still extends what the
// file holds it appends only the new blocks, then fsyncs; otherwise, once appends
// have doubled the file, or once the proofs c has shed are a quarter of it, it
// rewrites the whole file atomically.
func (w *Writer) Sync(c *chain.Chain) error {
	if w.held > 0 && w.held <= c.Len() && w.size < w.compactAt && !w.shedOutweighs(c) {
		if last := c.BlockAt(uint64(w.held - 1)); last.Hash() == w.head {
			return w.append(c.Blocks(uint64(w.held)))
		}
	}
	return w.rewrite(c)
}

func (w *Writer) append(blocks []chain.Block) error {
	if len(blocks) == 0 {
		return nil
	}
	offs := make([]int64, 0, len(blocks))
	data := make([]byte, 0, 4096)
	for i := range blocks {
		offs = append(offs, w.size+int64(len(data)))
		data = appendFrames(data, blocks[i:i+1])
	}
	if _, err := w.f.WriteAt(data, w.size); err != nil {
		return w.reset(err)
	}
	if err := w.f.Sync(); err != nil {
		return w.reset(err)
	}
	w.offs = append(w.offs[:w.held], offs...)
	for i := range blocks {
		w.noteHeavy(&blocks[i])
	}
	w.held += len(blocks)
	w.head = blocks[len(blocks)-1].Hash()
	w.size += int64(len(data))
	return nil
}

// rewrite writes c's whole chain to a new file and swaps it in, a block at a time
// so no more than one block is in memory for it. Blocks c has released are read
// from the old file, which stays open until the new one has replaced it.
func (w *Writer) rewrite(c *chain.Chain) error {
	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".chain-*")
	if err != nil {
		return w.reset(err)
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return w.reset(err)
	}
	bw := bufio.NewWriterSize(tmp, 1<<20)
	if _, err := bw.Write(logMagic); err != nil {
		return fail(err)
	}
	size := int64(len(logMagic))
	n := c.Len()
	offs := make([]int64, 0, n)
	var heavy []heavyFrame
	var frame []byte
	var head ports.Hash
	for h := 0; h < n; h++ {
		b := c.BlockAt(uint64(h))
		frame = appendFrames(frame[:0], []chain.Block{b})
		if _, err := bw.Write(frame); err != nil {
			return fail(err)
		}
		if b.CarriesHeavyProofs() {
			heavy = append(heavy, heavyFrame{height: b.Height, bytes: proofBytes(&b)})
		}
		offs = append(offs, size)
		size += int64(len(frame))
		head = b.Hash()
	}
	if err := bw.Flush(); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmpName, w.path); err != nil {
		return fail(err)
	}
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		df.Close()
	}
	// tmp is the file now at path, still open for writing; reopen it for both.
	tmp.Close()
	f, err := os.OpenFile(w.path, os.O_RDWR, 0)
	if err != nil {
		return w.reset(err)
	}
	if w.f != nil {
		w.f.Close()
	}
	w.f, w.offs, w.held, w.head, w.size, w.compactAt = f, offs, n, head, size, 2*size
	w.heavy = heavy
	return nil
}

// reset forgets what the file holds for appending after a failed write, which
// may have left a partial frame, so the next Sync rewrites it whole. The file
// stays open for reading the frames before it.
func (w *Writer) reset(err error) error {
	w.held = -1
	return err
}

// Close releases the file. The Writer may still be used; its next Sync rewrites.
// A chain that released blocks to it can no longer read them, so a daemon closes
// it only on its way out.
func (w *Writer) Close() error {
	w.held = -1
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f, w.offs = nil, nil
	return err
}

// Replay loads the file into c, block by block, re-verifying each block's
// structure and signatures through chain.Reload but not the live reputation gate
// (see the package doc and chain.appendStructural). As it goes it lets c shed
// heavy proofs and release old blocks to this Writer, exactly as a running node
// does after each commit, so a restart holds no more history in memory than the
// node did before it stopped. It returns how many blocks were restored. A file
// that fails after a valid prefix restores that prefix and returns the error.
//
// Afterwards the Writer holds the file as replayed: the next Sync appends to it.
// A torn final frame, an append a crash interrupted before its fsync returned,
// is cut off, as the commit it carried was never durable.
func (w *Writer) Replay(c *chain.Chain) (int, error) {
	f, err := os.OpenFile(w.path, os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	magic, err := r.Peek(len(logMagic))
	if err != nil || !bytes.Equal(magic, logMagic) {
		// A legacy file, or one too short to be anything: read it whole as
		// before. The first Sync rewrites it as a log.
		f.Close()
		blocks, err := Load(w.path)
		if len(blocks) == 0 {
			return 0, err
		}
		n, rerr := c.Reload(blocks)
		if rerr != nil {
			return n, rerr
		}
		return n, err
	}
	r.Discard(len(logMagic))
	w.f, w.offs, w.held, w.heavy = f, nil, -1, nil
	off := int64(len(logMagic))
	var hdr [4]byte
	var payload []byte
	var last chain.Block
	finish := func() {
		w.held, w.size, w.compactAt = len(w.offs), off, 2*off
		w.head = ports.Hash{}
		if len(w.offs) > 0 {
			w.head = last.Hash()
		}
	}
	// failed keeps the valid prefix readable but never appends after a bad
	// frame: the next Sync rewrites the file.
	failed := func(err error) (int, error) {
		finish()
		w.held = -1
		return len(w.offs), err
	}
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			break // the end, or a torn length header
		}
		n := int(binary.BigEndian.Uint32(hdr[:]))
		if n > maxFrame {
			return failed(fmt.Errorf("%w: frame %d declares %d bytes", ErrCorruptFrame, len(w.offs), n))
		}
		if cap(payload) < n+4 {
			payload = make([]byte, n+4)
		}
		payload = payload[:n+4]
		if _, err := io.ReadFull(r, payload); err != nil {
			break // torn final frame
		}
		if crc32.Checksum(payload[:n], crcTable) != binary.BigEndian.Uint32(payload[n:]) {
			return failed(fmt.Errorf("%w: frame %d fails its checksum", ErrCorruptFrame, len(w.offs)))
		}
		b, err := chain.Decode(payload[:n])
		if err != nil {
			return failed(fmt.Errorf("frame %d: %w", len(w.offs), err))
		}
		if _, err := c.Reload([]chain.Block{*b}); err != nil {
			return failed(err)
		}
		w.offs = append(w.offs, off)
		w.noteHeavy(b)
		off += int64(4 + n + 4)
		last = *b
		if len(w.offs)%releaseEvery == 0 {
			w.held = len(w.offs)
			c.PruneBelowHorizon()
			if _, err := c.ReleaseBlocks(w); err != nil {
				return failed(err)
			}
		}
	}
	finish()
	if fi, err := f.Stat(); err == nil && fi.Size() > off {
		if err := f.Truncate(off); err != nil {
			w.held = -1 // the next Sync rewrites instead
		}
	}
	return len(w.offs), nil
}

// LossError reports that replaying path would DISCARD finalized history: the
// file is torn (undecodable — no valid prefix survives, Restored = 0) or a
// block inside it fails structural verification (the valid prefix of
// Restored blocks is kept, the suffix is lost). It is returned by Recover
// when the operator has not accepted the loss.
type LossError struct {
	Restored  int    // blocks of valid prefix restored into the chain
	Cause     error  // the replay failure
	Preserved string // with an accepted loss: where the untouched original file was moved
}

func (e *LossError) Error() string {
	return fmt.Sprintf("chain replay would discard finalized history (%d-block valid prefix kept): %v", e.Restored, e.Cause)
}

// rejectedName is the name an accepted-loss boot preserves the original
// chain.cbor under: never overwritten, never deleted by the daemon.
func rejectedName(path string, now int64) string {
	return fmt.Sprintf("%s.rejected-%d", path, now)
}

func (e *LossError) Unwrap() error { return e.Cause }

// Recover is the daemon's boot-time replay with the refuse-to-start rule
// (this lane, scope call S3): a replay that would discard finalized history —
// a corrupt frame, an undecodable legacy file, or a block that fails structural
// verification — is a LossError and the caller must NOT start, unless acceptLoss
// is set (the
// operator's explicit acknowledgement that the suffix will be re-synced from
// peers, which is impossible below the swarm's prune horizon without a fresh
// -ws-checkpoint). With acceptLoss the valid prefix is kept, the loss is
// reported through the returned LossError with a nil refusal, and — because
// the daemon's first save would otherwise OVERWRITE the file — the original
// chain.cbor is first MOVED, untouched, to chain.cbor.rejected-<unix>: a
// rejected file may be byte-perfect and merely unreadable by THIS binary (a
// version-unsupported block after a downgrade; the no-verifier guard), so the
// operator's acceptance must never destroy it. A missing file is an empty
// history, never a loss.
func Recover(path string, c *chain.Chain, acceptLoss bool) (restored int, loss *LossError, refused error) {
	return NewWriter(path).Recover(c, acceptLoss)
}

// Recover is the package's Recover through w, which then holds the file as
// replayed, or, after an accepted loss, rewrites a new one at its next Sync.
func (w *Writer) Recover(c *chain.Chain, acceptLoss bool) (restored int, loss *LossError, refused error) {
	path := w.path
	n, err := w.Replay(c)
	if err == nil {
		return n, nil, nil
	}
	w.held = -1
	loss = &LossError{Restored: n, Cause: err}
	if !acceptLoss {
		return n, loss, loss
	}
	kept := rejectedName(path, time.Now().Unix())
	for i := 1; ; i++ {
		// Never clobber an earlier preserved copy (second-granular names).
		if _, serr := os.Stat(kept); os.IsNotExist(serr) {
			break
		}
		kept = fmt.Sprintf("%s.%d", rejectedName(path, time.Now().Unix()), i)
	}
	if rerr := os.Rename(path, kept); rerr != nil {
		// Cannot preserve the original ⇒ cannot safely accept the loss.
		loss.Cause = fmt.Errorf("%w; and the original could not be preserved as %s: %v", err, kept, rerr)
		return n, loss, loss
	}
	loss.Preserved = kept
	return n, loss, nil
}

// Replay loads path into c through a new Writer (see Writer.Replay) and returns
// how many blocks were restored.
func Replay(path string, c *chain.Chain) (int, error) {
	return NewWriter(path).Replay(c)
}
