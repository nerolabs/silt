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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
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
// blocks committed since its last Sync. It is not safe for concurrent use; the
// daemon calls it from its event loop.
type Writer struct {
	path string
	f    *os.File // open for appending after the first rewrite
	// held is how many blocks the file holds, and head the hash of the last of
	// them. held is -1 until the first Sync, which rewrites the file whatever it
	// held: that migrates a legacy file and drops a torn final frame.
	held int
	head ports.Hash
	// size is the file's length; compactAt is twice its length after the last
	// rewrite, past which the next Sync rewrites instead of appending.
	size, compactAt int64
}

// NewWriter returns a Writer for path. It touches nothing until the first Sync.
func NewWriter(path string) *Writer { return &Writer{path: path, held: -1} }

// Sync makes the file hold c's committed chain. When c still extends what the
// file holds it appends only the new blocks, then fsyncs; otherwise, or once
// appends have doubled the file, it rewrites the whole file atomically.
func (w *Writer) Sync(c *chain.Chain) error {
	if w.held > 0 && w.held <= c.Len() && w.size < w.compactAt {
		tail := c.Blocks(uint64(w.held - 1))
		if len(tail) > 0 && tail[0].Hash() == w.head {
			return w.append(tail[1:])
		}
	}
	return w.rewrite(c.Blocks(0))
}

func (w *Writer) append(blocks []chain.Block) error {
	if len(blocks) == 0 {
		return nil
	}
	data := appendFrames(nil, blocks)
	if _, err := w.f.Write(data); err != nil {
		return w.reset(err)
	}
	if err := w.f.Sync(); err != nil {
		return w.reset(err)
	}
	w.held += len(blocks)
	w.head = blocks[len(blocks)-1].Hash()
	w.size += int64(len(data))
	return nil
}

func (w *Writer) rewrite(blocks []chain.Block) error {
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	size, err := save(w.path, blocks)
	if err != nil {
		return w.reset(err)
	}
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return w.reset(err)
	}
	w.f, w.held, w.size, w.compactAt = f, len(blocks), size, 2*size
	w.head = ports.Hash{}
	if len(blocks) > 0 {
		w.head = blocks[len(blocks)-1].Hash()
	}
	return nil
}

// reset forgets what the file holds after a failed write, which may have left a
// partial frame, so the next Sync rewrites it whole.
func (w *Writer) reset(err error) error {
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	w.held = -1
	return err
}

// Close releases the file. The Writer may still be used; its next Sync rewrites.
func (w *Writer) Close() error { return w.reset(nil) }

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
	n, err := Replay(path, c)
	if err == nil {
		return n, nil, nil
	}
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

// Replay loads path into c and reloads our own persisted history via
// chain.Reload — re-verifying each block's structure and signatures, but not
// the live reputation gate (see the package doc and chain.appendStructural).
// Returns how many blocks were restored. A file that fails to load after a valid
// prefix restores that prefix and returns the load error.
func Replay(path string, c *chain.Chain) (int, error) {
	blocks, err := Load(path)
	if len(blocks) == 0 {
		return 0, err
	}
	n, rerr := c.Reload(blocks)
	if rerr != nil {
		return n, rerr
	}
	return n, err
}
