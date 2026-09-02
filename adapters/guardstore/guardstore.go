// Package guardstore persists the R0.4b cross-server double-redeem guard
// (ports.PaidSerialStore) so a process restart is not an eviction of every guarded
// token.
//
// THE HAZARD IT CLOSES. The guard's certified coupling condition is "evicted ⇒
// expired ⇒ un-redeemable". Both serial guards were process memory with no persistence
// and no restore, so a restart forgot every entry — in-window or not — and the
// identical MsgDeliveryReceipt was banked and PAID a second time (red-team re-break
// F2, 2026-09-03, measured at the node tier through the real wire handler). Today that
// is masked on the shipped daemon because balances reset at the same restart; it is a
// live mint the moment the ledger gains persistence or moves to shared settlement,
// which is exactly the topology the guard exists for.
//
// THE SHAPE: an APPEND-ONLY LOG of fixed-width records, fsync'd per append, replaced
// atomically on compaction (temp → fsync file → rename → fsync dir, the shape
// adapters/markstore and adapters/diskissuer both use).
//
// Why not a whole-file rewrite per entry, like the epoch key store? The key store
// writes a ~9-key band a few times an hour; this store writes one record per PAID
// DELIVERY, up to the modelled 256 serves per block. Rewriting a 65,536-entry file per
// receipt is megabytes of I/O per delivery. An append is 73 bytes and one fsync, and
// the file is re-written only when an expiry sweep actually removes something.
//
// Why fixed-width and not CBOR? A fixed record makes a torn tail (a crash between the
// write and the fsync) a length check rather than a parse heuristic, and a partial
// record is SAFE to drop: Append had not returned, so the ledger had not paid.
package guardstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nerolabs/silt/ports"
)

// The on-disk record: serial length, the serial (right-padded), the collecting
// server's NodeID, and the token's issue epoch, big-endian — the same epoch wire form
// the FDH message and the issuerKeyCommit leaf use.
const (
	maxSerialBytes = 32 // blindtoken.SerialSize; a longer serial is not a serial
	recSize        = 1 + maxSerialBytes + 32 + 8
)

// ErrCorrupt marks a store whose contents are not a whole number of well-formed
// records. Like a corrupt sign-mark it is a refuse-to-start condition for the caller,
// never a silent fresh start: silently starting empty IS the eviction this store
// exists to prevent.
var ErrCorrupt = errors.New("guardstore: paid-serial store is corrupt")

// Disk is the append-only file store.
type Disk struct {
	path string
	f    *os.File
}

// Open prepares the store at path, creating the parent directory if needed.
func Open(path string) (*Disk, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("guardstore: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("guardstore: %w", err)
	}
	return &Disk{path: path, f: f}, nil
}

// Close releases the append handle.
func (d *Disk) Close() error { return d.f.Close() }

func encode(p ports.PaidSerial) ([recSize]byte, error) {
	var rec [recSize]byte
	if len(p.Serial) == 0 || len(p.Serial) > maxSerialBytes {
		return rec, fmt.Errorf("guardstore: serial is %d bytes, want 1..%d", len(p.Serial), maxSerialBytes)
	}
	rec[0] = byte(len(p.Serial))
	copy(rec[1:], p.Serial)
	copy(rec[1+maxSerialBytes:], p.Server[:])
	binary.BigEndian.PutUint64(rec[1+maxSerialBytes+32:], p.Epoch)
	return rec, nil
}

// Load returns every persisted entry. A trailing PARTIAL record is dropped, not an
// error: it is a crash between the write and the fsync, so Append never returned and
// the ledger never paid against it. A record with an impossible serial length is a
// real corruption.
func (d *Disk) Load() ([]ports.PaidSerial, error) {
	blob, err := os.ReadFile(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	whole := len(blob) / recSize
	out := make([]ports.PaidSerial, 0, whole)
	for i := 0; i < whole; i++ {
		rec := blob[i*recSize : (i+1)*recSize]
		n := int(rec[0])
		if n == 0 || n > maxSerialBytes {
			return nil, fmt.Errorf("%w: record %d declares a %d-byte serial", ErrCorrupt, i, n)
		}
		var p ports.PaidSerial
		p.Serial = append([]byte(nil), rec[1:1+n]...)
		copy(p.Server[:], rec[1+maxSerialBytes:1+maxSerialBytes+32])
		p.Epoch = binary.BigEndian.Uint64(rec[1+maxSerialBytes+32:])
		out = append(out, p)
	}
	return out, nil
}

// Append durably records one entry: write, then fsync, THEN return. The ledger calls
// this before it moves any credit, so the ordering is what makes a crash an under-pay
// rather than a double-pay.
func (d *Disk) Append(p ports.PaidSerial) error {
	rec, err := encode(p)
	if err != nil {
		return err
	}
	if _, err := d.f.Write(rec[:]); err != nil {
		return err
	}
	return d.f.Sync()
}

// Compact atomically replaces the store with live. Temp → fsync file → rename → fsync
// dir: the directory sync is what makes the RENAME durable, without it a power cut can
// leave the directory entry pointing at the old file while the new bytes are on disk.
func (d *Disk) Compact(live []ports.PaidSerial) error {
	dir := filepath.Dir(d.path)
	tmp, err := os.CreateTemp(dir, ".tmp-paidserials-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	buf := make([]byte, 0, len(live)*recSize)
	for _, p := range live {
		rec, err := encode(p)
		if err != nil {
			tmp.Close()
			return err
		}
		buf = append(buf, rec[:]...)
	}
	if _, err := tmp.Write(buf); err != nil {
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
	if err := os.Rename(tmp.Name(), d.path); err != nil {
		return err
	}
	if df, derr := os.Open(dir); derr == nil {
		_ = df.Sync()
		df.Close()
	}
	// Re-open the append handle onto the replaced file.
	old := d.f
	f, err := os.OpenFile(d.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	d.f = f
	return old.Close()
}

// Mem is the in-memory variant for tests and the deterministic sim (no disk in the
// sim, B1/B2). It exercises the full store seam, including sharing one store across a
// simulated restart.
type Mem struct{ entries []ports.PaidSerial }

func NewMem() *Mem { return &Mem{} }

func (m *Mem) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), m.entries...), nil
}

func (m *Mem) Append(p ports.PaidSerial) error {
	if _, err := encode(p); err != nil {
		return err
	}
	m.entries = append(m.entries, p)
	return nil
}

func (m *Mem) Compact(live []ports.PaidSerial) error {
	m.entries = append([]ports.PaidSerial(nil), live...)
	return nil
}

var (
	_ ports.PaidSerialStore = (*Disk)(nil)
	_ ports.PaidSerialStore = (*Mem)(nil)
)
