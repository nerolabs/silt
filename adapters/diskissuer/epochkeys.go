package diskissuer

// R0.4b C3 close — persistence for the PER-EPOCH demand-issuer keys.
//
// WHY A SECOND STORE. The publish-token issuer key (Store, above) is ONE key that
// must never change: peers cache its public half and the chain re-verifies committed
// publish tokens against it on every replay, so rotating it would invalidate history.
// The demand lane needs the opposite: a DIFFERENT key per consensus epoch, generated
// and committed W epochs ahead, dropped once its epoch leaves the window. Sharing one
// key across both lanes is exactly what the C3 close forbids — the publish key must
// never enter the demand keyset, or a demand blind bought on the publish lane is a
// token no bank will ever honour.
//
// THE SHAPE (Builder's call; see docs/thinking/2026-09-02-r0.4b-c3-close-design.md §3).
// ONE file holding a CBOR map epoch → PKCS#1 DER, rewritten atomically (temp +
// rename) whenever the band changes. Rejected alternatives:
//
//   - One file per epoch. Containment is marginally better (a corrupt file loses one
//     epoch, not the band) but it costs a filename codec, a directory scan, and a
//     per-file "is this corrupt or absent" decision at every load. The band is at
//     most 2W+1 ≈ 9 keys ≈ 11 KB; there is nothing to gain by paging it.
//   - Deriving key_E deterministically from one persisted seed. Tempting — a
//     one-secret store with no pruning — but REFUTED: crypto/rsa.GenerateKey is
//     explicitly not stable across Go versions, so a toolchain upgrade would silently
//     regenerate different keys for epochs whose fingerprints are ALREADY COMMITTED,
//     and the commitment is append-only, so the lane would be dead for W epochs with
//     no way to re-register. A liveness cliff bought for a few KB.
//
// A corrupt file is a hard error, never a silent regeneration — the same rule the
// publish key store keeps, and for a stronger reason here: quietly minting new keys
// over committed fingerprints is unrecoverable, while failing to start is not.

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
)

// EpochStore persists a validator's per-epoch demand-issuer keys at
// dir/demandkeys.cbor.
type EpochStore struct{ path string }

// epochKeyFile is the on-disk form: epoch → PKCS#1 DER private key.
type epochKeyFile struct {
	Keys map[uint64][]byte `cbor:"1,keyasint"`
}

// OpenEpochs prepares the per-epoch demand key store at dir/demandkeys.cbor,
// creating dir if needed.
func OpenEpochs(dir string) (*EpochStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("diskissuer: %w", err)
	}
	return &EpochStore{path: filepath.Join(dir, "demandkeys.cbor")}, nil
}

// Load returns the persisted per-epoch keys. An absent file is an empty map and no
// error (first run); a corrupt or unparsable file is a real error.
func (s *EpochStore) Load() (map[uint64]*rsa.PrivateKey, error) {
	blob, rerr := os.ReadFile(s.path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return map[uint64]*rsa.PrivateKey{}, nil
		}
		return nil, rerr
	}
	var f epochKeyFile
	if err := cbor.Unmarshal(blob, &f); err != nil {
		return nil, fmt.Errorf("diskissuer: corrupt demand key store: %w", err)
	}
	out := make(map[uint64]*rsa.PrivateKey, len(f.Keys))
	for e, der := range f.Keys {
		k, perr := x509.ParsePKCS1PrivateKey(der)
		if perr != nil {
			return nil, fmt.Errorf("diskissuer: corrupt demand key for epoch %d: %w", e, perr)
		}
		out[e] = k
	}
	return out, nil
}

// Save writes the whole band atomically with owner-only permissions.
func (s *EpochStore) Save(keys map[uint64]*rsa.PrivateKey) error {
	f := epochKeyFile{Keys: make(map[uint64][]byte, len(keys))}
	for e, k := range keys {
		f.Keys[e] = x509.MarshalPKCS1PrivateKey(k)
	}
	blob, err := cbor.Marshal(f)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".tmp-demandkeys-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(blob); err != nil {
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
	return os.Rename(tmp.Name(), s.path)
}

// EnsureBand is the one call the rotation scheduler makes each epoch. It loads the
// persisted band, GENERATES a fresh RSA key for every epoch in [genFrom, genTo] that
// has none, PRUNES every epoch outside [keepFrom, genTo], persists the result if it
// changed, and returns the band.
//
// The two lower bounds differ on purpose:
//
//   - keepFrom is cur − W: the issuer must still hold the PRIVATE key for an
//     in-window past epoch, because a requester whose consensus clock trails by an
//     epoch names that epoch and the issuer signs for it. That is what makes the
//     epoch-boundary race a served request instead of a burnt fee.
//   - genFrom is cur: a fresh node generates only forward. Generating the past band
//     too would cost W extra RSA keygens at boot for keys nothing was ever issued
//     under.
//
// genTo is cur + W: the key schedule is PRE-PUBLISHED to the end of the
// pre-publication window the chain accepts, so a token withdrawn at any epoch
// boundary finds key_E already committed. rng is injected (nil uses crypto/rand).
func (s *EpochStore) EnsureBand(rng io.Reader, keepFrom, genFrom, genTo uint64) (map[uint64]*rsa.PrivateKey, error) {
	keys, err := s.Load()
	if err != nil {
		return nil, err
	}
	changed := false
	for e := range keys {
		if e < keepFrom || e > genTo {
			delete(keys, e)
			changed = true
		}
	}
	for e := genFrom; e <= genTo; e++ {
		if keys[e] != nil {
			continue
		}
		k, gerr := generateKey(rng)
		if gerr != nil {
			return nil, gerr
		}
		keys[e] = k
		changed = true
	}
	if changed {
		if err := s.Save(keys); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// RotateWindow is one rotation step for consensus epoch cur with validity window w:
// retain [cur−w, cur], generate and pre-publish [cur, cur+w], and hand every key in
// the resulting band to install (which stages its on-chain commitment).
//
// It is idempotent — a second call for the same epoch generates nothing and installs
// the same band — so a restart, a replay, and a duplicate epoch turn all converge on
// the same schedule. w is passed rather than imported so this adapter keeps no
// dependency on core/demand; cmd/silt supplies demand.DefaultWindow and the coupling
// is pinned by TestRotateWindowUsesTheDemandWindow.
func (s *EpochStore) RotateWindow(rng io.Reader, cur, w uint64, install func(epoch uint64, priv *rsa.PrivateKey)) error {
	keepFrom := uint64(0)
	if cur > w {
		keepFrom = cur - w
	}
	keys, err := s.EnsureBand(rng, keepFrom, cur, cur+w)
	if err != nil {
		return err
	}
	for e, k := range keys {
		install(e, k)
	}
	return nil
}
