// Package markstore persists a validator's monotonic last-signed watermark
// (ports.SignMark) — the crash-safe half of the never-sign-twice rule (#397).
// The mark is fsync'd to disk BEFORE the matching signature is released to the
// wire (the Tendermint priv_validator_state pattern, B8): a crash between the
// two leaves an unused mark, which is safe; the reverse order would let a
// restarted validator re-sign a height it already signed and be permanently
// slashed for an honest double-sign.
package markstore

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nerolabs/silt/ports"
)

// Disk stores the mark in a single JSON file, replaced atomically
// (write temp → fsync file → rename → fsync dir).
type Disk struct{ path string }

func New(path string) *Disk { return &Disk{path: path} }

// Round/Phase are additive (#432 rounds): a mark persisted before rounds loads
// as {round:0, phase:0} — the legacy era-1 mark — exactly the schema-migration
// rule the certification pins.
type diskMark struct {
	Height uint64 `json:"height"`
	Round  uint64 `json:"round,omitempty"`
	Phase  uint8  `json:"phase,omitempty"`
	Hash   string `json:"hash"`
}

func (d *Disk) Load() (ports.SignMark, bool, error) {
	raw, err := os.ReadFile(d.path)
	if os.IsNotExist(err) {
		return ports.SignMark{}, false, nil
	}
	if err != nil {
		return ports.SignMark{}, false, err
	}
	// A corrupt mark is a refuse-to-start condition for the caller, never a
	// silent fresh start: losing the mark is exactly the crash window this
	// store exists to close.
	var m diskMark
	if err := json.Unmarshal(raw, &m); err != nil {
		return ports.SignMark{}, false, fmt.Errorf("sign-mark %s is corrupt: %w", d.path, err)
	}
	hb, err := hex.DecodeString(m.Hash)
	if err != nil || len(hb) != len(ports.Hash{}) {
		return ports.SignMark{}, false, fmt.Errorf("sign-mark %s is corrupt: bad hash %q", d.path, m.Hash)
	}
	var h ports.Hash
	copy(h[:], hb)
	return ports.SignMark{Height: m.Height, Round: m.Round, Phase: m.Phase, Hash: h}, true, nil
}

func (d *Disk) Save(m ports.SignMark) error {
	raw, err := json.Marshal(diskMark{Height: m.Height, Round: m.Round, Phase: m.Phase, Hash: hex.EncodeToString(m.Hash[:])})
	if err != nil {
		return err
	}
	dir := filepath.Dir(d.path)
	tmp, err := os.CreateTemp(dir, ".signmark-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, d.path); err != nil {
		os.Remove(tmpName)
		return err
	}
	// fsync the directory so the rename itself is durable.
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		df.Close()
	}
	return nil
}

// Mem is the in-memory variant for tests and the deterministic sim (no disk in
// the sim, B1/B2); it still exercises the full store seam, including sharing
// one store across a simulated restart.
type Mem struct {
	mark ports.SignMark
	set  bool
}

func NewMem() *Mem { return &Mem{} }

func (m *Mem) Load() (ports.SignMark, bool, error) { return m.mark, m.set, nil }
func (m *Mem) Save(mark ports.SignMark) error      { m.mark, m.set = mark, true; return nil }
