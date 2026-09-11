// Sealed manifests (M11): what gets stored in the swarm is ciphertext
// twice over.
//
//	blob = Seal_layoutKey( layout ‖ Seal_contentKey(secrets) )
//
// The outer layer hides the stripe structure from infrastructure; the
// inner box hides the decryption material from caretakers. Opening with
// only the layout key yields a Layout — everything repair and audit
// need, nothing a reader wants. Opening with both keys yields the full
// Manifest. Keys are supplied by the caller (core/link derives them
// from the silt link); this package just does the boxing.
//
// # The inner box hides the material; secretsPlainLen hides its length
//
// A box hides what is inside it, never how big the inside is. The secrets
// plaintext is 34 bytes per data chunk in convergent mode against a flat
// 34 bytes in private mode, and SealBox expands by a constant tag, so the
// sealed blob's length used to disclose the MODE to anyone who could
// measure it. Since the manifest frame is no longer padded
// (pipeline.ManifestFrameSize frames at true length), that was any peer
// that could fetch the manifest chunk — no keys, no care link, no bond,
// no token: 341 bytes private against 345 convergent at FileSize=1,
// widening 34 bytes per data chunk. The publisher's own secret/not-secret
// classification of every root was public (docs/threat-catalog.md F8).
//
// secretsPlainLen closes it: the secrets plaintext is zero-padded to a
// length that is a function of the DATA-SHARD COUNT and nothing else,
// before it is sealed. That is length-hiding authenticated encryption
// (Paterson–Ristenpart–Shrimpton, ASIACRYPT 2011), in the shape TLS 1.3
// deploys it — pad the record plaintext, authenticate the padding, strip
// it on open (RFC 8446 §5.4).
//
// Two things this deliberately is NOT. It is not a move or a deletion of
// the Mode field: Validate already makes ChunkSecrets ⟺ convergent and
// FileKey ⟺ private, so Mode is a redundant discriminator and removing it
// takes three of the four delta bytes at one chunk and none of the 34 per
// chunk that dominate at scale. And it is not padding of the manifest
// FRAME: that lifts the oracle only while both modes fit one frame, after
// which len(Entry.ManifestChunks) separates them on-chain with no fetch
// at all. The inner box is where the length is, so the inner box is the
// fix.
package manifest

import (
	"fmt"

	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/ports"
)

const (
	layoutDomain  = "silt/manifest/v1/layout"
	secretsDomain = "silt/manifest/v1/secrets"
)

// Layout is the repair-grade view of a file: which chunks, which
// stripes, what code — and no way to read any of it.
type Layout struct {
	Version   int      `cbor:"1,keyasint"`
	ChunkSize int64    `cbor:"2,keyasint"`
	K         int      `cbor:"3,keyasint,omitempty"`
	N         int      `cbor:"4,keyasint,omitempty"`
	Chunks    [][]byte `cbor:"5,keyasint"`
	Parity    [][]byte `cbor:"6,keyasint,omitempty"`
	// Box is the sealed secrets (opaque without the content key).
	Box []byte `cbor:"7,keyasint"`
}

type secretsPart struct {
	Mode         string   `cbor:"1,keyasint"`
	FileSize     int64    `cbor:"2,keyasint"`
	ChunkSecrets [][]byte `cbor:"3,keyasint,omitempty"`
	FileKey      []byte   `cbor:"4,keyasint,omitempty"`
}

func (l *Layout) ChunkIDs() []ports.ChunkID  { return toHashes(l.Chunks) }
func (l *Layout) ParityIDs() []ports.ChunkID { return toHashes(l.Parity) }
func (l *Layout) Leaves() []ports.Hash       { return append(l.ChunkIDs(), l.ParityIDs()...) }
func (l *Layout) Root() ports.Hash           { return MerkleRoot(l.Leaves()) }

// secretsPlainLen is the padded length of the secrets plaintext for a
// manifest with nChunks data chunks. It is the whole of the mode-oracle
// fix, so read the two properties it has to have:
//
//   - It is a function of nChunks ALONE. nChunks is the data-shard count,
//     which Layout publishes by construction (one 34-byte chunk ID each,
//     outside the inner box), so the target discloses nothing the
//     caretaker's view did not already carry. This signature is the
//     structure, not a convention: secretsPlainLen cannot be made to read
//     Mode, FileKey or ChunkSecrets without changing it, so a future edit
//     that would reintroduce the oracle cannot be a quiet one.
//   - It is an UPPER BOUND on the canonical encoding of any secretsPart
//     at that nChunks, in either mode, at any FileSize. Seal refuses to
//     ship a plaintext that exceeds it rather than truncating or growing,
//     because a target that is silently too small is the oracle back.
//
// The arithmetic is canonical-CBOR widths, and it is DRIVEN rather than
// trusted: TestSecretsPlainLenBoundsEveryEncoding marshals real
// secretsParts across the shape space and asserts each one fits with the
// slack this function predicts.
func secretsPlainLen(nChunks int) int {
	const (
		mapHeader  = 1      // map(n) header, n <= 23 keys
		keyByte    = 1      // an unsigned map key <= 23
		textHeader = 1      // text string header, len <= 23
		maxUint    = 1 + 8  // 0x1b + eight bytes: int64 at its widest
		hashField  = 2 + 32 // bstr(32): a 2-byte header plus the digest
	)
	widestMode := len(crypto.Convergent)
	if n := len(crypto.Private); n > widestMode {
		widestMode = n
	}
	// Keys 1 and 2 (Mode, FileSize) are present in both shapes. FileSize is
	// carried at its WIDEST encoding rather than its actual one, so the
	// target does not vary with it either — Entry.FileSize already publishes
	// the exact length on-chain, but a target that reads a sealed field is a
	// seam, and the whole of this fix is not having one.
	common := mapHeader + keyByte + textHeader + widestMode + keyByte + maxUint
	// Key 3, the convergent branch: one 32-byte secret per data chunk,
	// dropped entirely by omitempty when there are no chunks.
	convergent := 0
	if nChunks > 0 {
		convergent = keyByte + arrayHeaderLen(nChunks) + nChunks*hashField
	}
	// Key 4, the private branch: one flat 32-byte file key.
	private := keyByte + hashField
	if convergent > private {
		return common + convergent
	}
	return common + private
}

// arrayHeaderLen is the canonical CBOR array header width for n elements.
func arrayHeaderLen(n int) int {
	switch {
	case n < 24:
		return 1
	case n < 1<<8:
		return 2
	case n < 1<<16:
		return 3
	default:
		return 5
	}
}

// Seal encrypts m into the storable blob under the two derived keys.
func Seal(m *Manifest, layoutKey, contentKey [32]byte) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	sb, err := encMode.Marshal(secretsPart{
		Mode: m.Mode, FileSize: m.FileSize,
		ChunkSecrets: m.ChunkSecrets, FileKey: m.FileKey,
	})
	if err != nil {
		return nil, err
	}
	// Pad to the mode-independent length before sealing. The zeros go
	// inside the AEAD, so they are authenticated, not malleable.
	want := secretsPlainLen(len(m.Chunks))
	if len(sb) > want {
		return nil, fmt.Errorf("manifest: secrets encode to %d bytes over a %d-byte pad target for %d chunks: "+
			"secretsPlainLen no longer bounds secretsPart, and padding to a length the plaintext can exceed "+
			"republishes the encryption mode (threat-catalog F8)", len(sb), want, len(m.Chunks))
	}
	sb = append(sb, make([]byte, want-len(sb))...)
	box, err := crypto.SealBox(contentKey, secretsDomain, sb)
	if err != nil {
		return nil, err
	}
	eb, err := encMode.Marshal(Layout{
		Version: m.Version, ChunkSize: m.ChunkSize,
		K: m.K, N: m.N, Chunks: m.Chunks, Parity: m.Parity,
		Box: box,
	})
	if err != nil {
		return nil, err
	}
	return crypto.SealBox(layoutKey, layoutDomain, eb)
}

// OpenLayout decrypts only the outer layer: the caretaker's view.
func OpenLayout(blob []byte, layoutKey [32]byte) (*Layout, error) {
	eb, err := crypto.OpenBox(layoutKey, layoutDomain, blob)
	if err != nil {
		return nil, fmt.Errorf("manifest: layout: %w", err)
	}
	var l Layout
	if err := cborUnmarshal(eb, &l); err != nil {
		return nil, fmt.Errorf("manifest: layout: %w", err)
	}
	if l.Version != Version {
		return nil, fmt.Errorf("manifest: unsupported version %d", l.Version)
	}
	// OpenLayout returns before the full Validate (that needs the content
	// key), so it enforces the declared-number bounds itself: the decoder
	// already caps array element counts, this rejects an oversize declared
	// chunk size and is belt-and-suspenders on the counts (#88, B7, #14).
	if l.ChunkSize <= 0 || l.ChunkSize > MaxChunkSize {
		return nil, fmt.Errorf("manifest: layout chunk size %d out of range", l.ChunkSize)
	}
	if len(l.Chunks) > MaxChunks || len(l.Parity) > MaxChunks {
		return nil, fmt.Errorf("manifest: layout declares more than %d shards", MaxChunks)
	}
	return &l, nil
}

// OpenFull decrypts both layers: the reader's view.
func OpenFull(blob []byte, layoutKey, contentKey [32]byte) (*Manifest, error) {
	l, err := OpenLayout(blob, layoutKey)
	if err != nil {
		return nil, err
	}
	sb, err := crypto.OpenBox(contentKey, secretsDomain, l.Box)
	if err != nil {
		return nil, fmt.Errorf("manifest: secrets: %w", err)
	}
	var s secretsPart
	// UnmarshalFirst, not Unmarshal: the plaintext carries secretsPlainLen's
	// zero padding after the CBOR. The padding is refused unless it is
	// zero-filled, which keeps the plaintext canonical — one manifest has
	// exactly one sealed encoding — and refuses a blob that smuggles bytes
	// past the decoder inside the box.
	rest, err := decMode.UnmarshalFirst(sb, &s)
	if err != nil {
		return nil, fmt.Errorf("manifest: secrets: %w", err)
	}
	for _, b := range rest {
		if b != 0 {
			return nil, fmt.Errorf("manifest: secrets: %d trailing bytes are not zero padding", len(rest))
		}
	}
	m := &Manifest{
		Version: l.Version, ChunkSize: l.ChunkSize, K: l.K, N: l.N,
		Chunks: l.Chunks, Parity: l.Parity,
		Mode: s.Mode, FileSize: s.FileSize,
		ChunkSecrets: s.ChunkSecrets, FileKey: s.FileKey,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}
