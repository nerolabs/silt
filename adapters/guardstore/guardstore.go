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
// adapters/markstore and adapters/diskissuer both use), with the new append handle
// opened on the temp file BEFORE the rename so a failed open can never orphan the
// handle (R2.13, see Compact).
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

// ErrStoreBroken marks a store whose append handle can no longer be trusted to reach
// the live path. It is STICKY until the process restarts: every later Append and
// Compact fails with it, so the ledger refuses the payout (ReasonGuardStore) instead
// of paying against a guard entry a restart would never see. Loud beats silent here:
// the silent form of this state is R-COMPACT-ORPHAN, an over-pay.
var ErrStoreBroken = errors.New("guardstore: paid-serial store is broken (append handle no longer trusted; restart to recover)")

// openAppend is the OS hook used to open the append handle, both in Open and on the
// temp file inside Compact. Indirected ONLY so a test can force that open to fail
// without faking the filesystem — the R-COMPACT-ORPHAN defect (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1, gate G-CO-1).
// Production behaviour is unchanged: this is os.OpenFile and nothing else calls it.
var openAppend = os.OpenFile

// Disk is the append-only file store.
type Disk struct {
	path string
	f    *os.File
	// broken is the sticky backstop (ruling §1: "keep as backstop, not as the fix").
	// Set only by Compact, only for a failure AFTER the rename — see the comment
	// there for what can still fail — and checked first by Append and Compact.
	broken error
}

// Open prepares the store at path, creating the parent directory if needed, and
// REALIGNS a torn tail before handing back an append handle.
func Open(path string) (*Disk, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("guardstore: %w", err)
	}
	if err := realign(path); err != nil {
		return nil, fmt.Errorf("guardstore: %w", err)
	}
	f, err := openAppend(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("guardstore: %w", err)
	}
	return &Disk{path: path, f: f}, nil
}

// realign truncates the file back to its last COMPLETE record boundary and fsyncs,
// before any append handle exists (research certification 2026-09-03, gate G-3).
//
// WHY IT IS NOT OPTIONAL. Load drops a trailing partial record, which is sound only
// while the partial record STAYS the tail. O_APPEND sets the write offset to the
// file's size, so without this the next Append lands at an unaligned offset and every
// record boundary after it is shifted by the size of the orphan fragment. The next
// Load then splices the fragment onto the head of the real record: if the spliced
// length byte is impossible the store is a refuse-to-start outage, and if it is
// plausible (the fragment's first byte is a serial or NodeID byte, so roughly 1-in-8)
// the load succeeds SILENTLY with wrong contents and an ACKNOWLEDGED paid serial —
// one the ledger already paid against — is no longer guarded. That is the F2
// double-pay, reached through the adapter written to close it.
//
// Dropping the fragment is safe by this store's own argument: a partial record means
// Append had not returned, so the ledger had not moved any credit against it. Truncate
// then fsync, so the shorter size is durable BEFORE any append can be written past it.
func realign(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // a fresh store is aligned by construction
		}
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	rem := st.Size() % recSize
	if rem == 0 {
		return nil
	}
	if err := f.Truncate(st.Size() - rem); err != nil {
		return err
	}
	return f.Sync()
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
	if d.broken != nil {
		return d.broken
	}
	rec, err := encode(p)
	if err != nil {
		return err
	}
	if _, err := d.f.Write(rec[:]); err != nil {
		return err
	}
	return d.f.Sync()
}

// Compact atomically replaces the store with live. Temp → fsync file → open the new
// append handle ON THE TEMP FILE → rename → fsync dir → swap handles.
//
// The directory sync is what makes the RENAME durable; without it a power cut can
// leave the directory entry pointing at the old file while the new bytes are on disk.
//
// WHY THE NEW HANDLE IS OPENED BEFORE THE RENAME (R2.13, R-COMPACT-ORPHAN; PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §1). The previous
// shape renamed first and re-opened the append handle on d.path afterwards. If that
// re-open failed, Compact returned the error but d.f still pointed at the inode the
// rename had just unlinked; write AND fsync through that handle succeed (POSIX keeps an
// open unlinked inode alive), so every later Append returned nil for a record no Load
// could ever see — an over-pay, once per epoch since compaction moved onto the band
// advance. rename(2) moves the directory entry, not the inode: a handle opened on the
// temp file stays valid across the rename and then refers to the live path. So the
// only fallible open now happens BEFORE any state changes, and a failure there leaves
// the store exactly as it was (d.f valid, the log a superset of live — benign, it only
// ever over-refuses). After the rename nothing fallible remains between it and the
// handle swap.
//
// THE BACKSTOP. Anything that still fails after the rename marks the store broken
// (ErrStoreBroken, sticky until restart) rather than returning an error the caller
// might treat as benign. Today the only call after the rename that can fail is closing
// the retired handle; that is not, in itself, a broken store, and marking it broken is
// the deliberate loud-over-silent choice: a Close error after a successful write+fsync
// is not a state this code understands, and the cost of the loud reading is an
// under-pay until restart, never an over-pay.
func (d *Disk) Compact(live []ports.PaidSerial) error {
	if d.broken != nil {
		return d.broken
	}
	dir := filepath.Dir(d.path)
	tmp, err := os.CreateTemp(dir, ".tmp-paidserials-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has moved it
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
	// The new append handle, opened on the temp inode BEFORE the rename. A failure
	// here changes nothing: d.f is still the live handle and the temp is removed.
	next, err := openAppend(tmp.Name(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), d.path); err != nil {
		next.Close()
		return err
	}
	// From here the directory entry is the new file and next already refers to it.
	if df, derr := os.Open(dir); derr == nil {
		_ = df.Sync()
		df.Close()
	}
	old := d.f
	d.f = next
	if err := old.Close(); err != nil {
		d.broken = fmt.Errorf("%w: closing the retired append handle after compaction: %v", ErrStoreBroken, err)
		return d.broken
	}
	return nil
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
