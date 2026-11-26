// Package advisory is the version-floor advisory: the network's maintainers, by a threshold of
// their keys, ask its validators to run at least a given release.
//
// It never updates anything. A node reads it, warns its operator, and — only at the enforce tier,
// only after a grace period counted in blocks from the height the advisory was committed — patched
// validators stop counting consensus messages from nodes advertising an older release. No single
// key can declare one: an advisory carrying fewer than the threshold of distinct maintainer
// signatures is not an advisory and changes nothing. A later advisory, by sequence number,
// replaces the one before it, which is how one is recalled.
package advisory

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

// Tier is how hard an advisory presses.
type Tier uint8

const (
	// TierRecall withdraws the floor: an advisory at this tier, with a higher sequence than the
	// one in force, leaves no floor at all.
	TierRecall Tier = 0
	// TierNotice and TierUrgent warn the operator of a node below the floor, on its log, its
	// status page and its UI. They change no behaviour.
	TierNotice Tier = 1
	TierUrgent Tier = 2
	// TierEnforce is the gate of last resort: once the grace period has passed, patched
	// validators refuse consensus messages from nodes advertising a release below the floor.
	// Storage, serving and fetching are untouched.
	TierEnforce Tier = 3
)

// MaxNoteBytes bounds the operator-facing note an advisory carries.
const MaxNoteBytes = 280

// MaxGraceBlocks bounds the grace period, so an advisory cannot defer its effect indefinitely.
const MaxGraceBlocks = 1 << 20

// Advisory is the signed content.
type Advisory struct {
	// Seq orders advisories: one with a higher sequence replaces the one in force.
	Seq uint64 `cbor:"1,keyasint"`
	// Floor is the lowest release (core/release.Number) validators are asked to run.
	Floor uint32 `cbor:"2,keyasint"`
	Tier  Tier   `cbor:"3,keyasint"`
	// GraceBlocks is how many blocks after the advisory is committed the enforce tier waits.
	GraceBlocks uint64 `cbor:"4,keyasint"`
	// Note tells an operator what the release fixes and where to get it.
	Note string `cbor:"5,keyasint,omitempty"`
}

// Signature is one maintainer's signature over the advisory.
type Signature struct {
	Pub []byte `cbor:"1,keyasint"` // ed25519 public key
	Sig []byte `cbor:"2,keyasint"`
}

// Signed is an advisory with its maintainers' signatures.
type Signed struct {
	Advisory   Advisory    `cbor:"1,keyasint"`
	Signatures []Signature `cbor:"2,keyasint"`
}

const domain = "silt/version-floor/v1"

var (
	// ErrMalformed is an advisory whose fields are out of bounds.
	ErrMalformed = errors.New("advisory: malformed")
	// ErrBelowThreshold is an advisory with fewer valid signatures from distinct maintainer keys
	// than the threshold. It is not an advisory at all.
	ErrBelowThreshold = errors.New("advisory: fewer valid maintainer signatures than the threshold")
)

// SigBytes is the message a maintainer signs: the domain, the network's chain id (an advisory
// for one network is no advisory on another), and the canonical advisory.
func SigBytes(chainID ports.Hash, a Advisory) ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	raw, err := enc.Marshal(a)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(domain)+len(chainID)+len(raw))
	out = append(out, domain...)
	out = append(out, chainID[:]...)
	return append(out, raw...), nil
}

// Sign adds one maintainer's signature to s.
func Sign(s Signed, priv ed25519.PrivateKey, chainID ports.Hash) (Signed, error) {
	msg, err := SigBytes(chainID, s.Advisory)
	if err != nil {
		return Signed{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	out := s
	out.Signatures = append(append([]Signature(nil), s.Signatures...), Signature{Pub: append([]byte(nil), pub...), Sig: ed25519.Sign(priv, msg)})
	return out, nil
}

// Validate checks the advisory's own bounds.
func (a Advisory) Validate() error {
	switch {
	case a.Seq == 0:
		return fmt.Errorf("%w: sequence 0", ErrMalformed)
	case a.Tier > TierEnforce:
		return fmt.Errorf("%w: tier %d", ErrMalformed, a.Tier)
	case a.Tier == TierRecall && a.Floor != 0:
		return fmt.Errorf("%w: a recall names no floor", ErrMalformed)
	case a.Tier != TierRecall && a.Floor == 0:
		return fmt.Errorf("%w: a floor of 0 asks nothing", ErrMalformed)
	case a.GraceBlocks > MaxGraceBlocks:
		return fmt.Errorf("%w: grace %d blocks exceeds %d", ErrMalformed, a.GraceBlocks, MaxGraceBlocks)
	case len(a.Note) > MaxNoteBytes:
		return fmt.Errorf("%w: note of %d bytes exceeds %d", ErrMalformed, len(a.Note), MaxNoteBytes)
	}
	return nil
}

// Verify checks s under chainID against the maintainer key set and threshold. It counts each
// maintainer key once however many times it signs; a signature from a key outside the set, or one
// that does not verify, counts for nothing. A threshold below 2 accepts nothing: no single key may
// declare a floor.
func Verify(s Signed, chainID ports.Hash, maintainers [][]byte, threshold int) error {
	if err := s.Advisory.Validate(); err != nil {
		return err
	}
	if threshold < 2 || threshold > len(maintainers) {
		return fmt.Errorf("%w: threshold %d over %d maintainer key(s) accepts nothing", ErrBelowThreshold, threshold, len(maintainers))
	}
	msg, err := SigBytes(chainID, s.Advisory)
	if err != nil {
		return err
	}
	valid := map[string]bool{}
	for _, sig := range s.Signatures {
		if len(sig.Pub) != ed25519.PublicKeySize || len(sig.Sig) != ed25519.SignatureSize || valid[string(sig.Pub)] {
			continue
		}
		if !isMaintainer(maintainers, sig.Pub) {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(sig.Pub), msg, sig.Sig) {
			valid[string(sig.Pub)] = true
		}
	}
	if len(valid) < threshold {
		return fmt.Errorf("%w: %d of %d", ErrBelowThreshold, len(valid), threshold)
	}
	return nil
}

func isMaintainer(maintainers [][]byte, pub []byte) bool {
	for _, m := range maintainers {
		if bytes.Equal(m, pub) {
			return true
		}
	}
	return false
}

// SortKeys returns the maintainer keys in canonical (ascending byte) order, the form genesis
// commits them in.
func SortKeys(keys [][]byte) [][]byte {
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = append([]byte(nil), k...)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i], out[j]) < 0 })
	return out
}

// Marshal is the canonical CBOR of s.
func (s Signed) Marshal() ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	return enc.Marshal(s)
}

// Unmarshal parses a signed advisory. Nothing in it is trusted until Verify passes.
func Unmarshal(b []byte) (Signed, error) {
	var s Signed
	if err := cbor.Unmarshal(b, &s); err != nil {
		return Signed{}, err
	}
	return s, nil
}

// Enforcing reports whether an enforce-tier advisory committed at height committedAt is in force
// at height h: its grace has passed. The clock is the chain's height, never a wall clock.
func Enforcing(a Advisory, committedAt, h uint64) bool {
	return a.Tier == TierEnforce && a.Floor > 0 && h >= committedAt+a.GraceBlocks
}
