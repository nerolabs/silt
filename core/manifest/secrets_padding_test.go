package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/nerolabs/silt/core/crypto"
)

// These gates hold the close of RT-SFO-1, the keyless encryption-mode oracle
// (docs/threat-catalog.md F8; research certification
// "privacy-property-measured-part0-corner-entry-filesize-and-mode-oracle", 2026-09-11).
// The end-to-end property — a published object's manifest frames identically in both modes —
// is asserted in core/pipeline (TestRT_SFO_1_ModeIsNotRecoverableFromManifestFrameLength).
// What lives here is the two things that gate cannot see: that secretsPlainLen's ARITHMETIC
// really bounds every encoding, and that the padded plaintext still round-trips.

func padTestHash(i int) []byte {
	h := sha256.Sum256(binary.BigEndian.AppendUint64([]byte("silt/pad-test"), uint64(i)))
	return h[:]
}

func padTestManifest(t *testing.T, mode crypto.Mode, nChunks int, fileSize int64) *Manifest {
	t.Helper()
	m := &Manifest{Version: Version, Mode: string(mode), ChunkSize: 1 << 18, FileSize: fileSize}
	for i := 0; i < nChunks; i++ {
		m.Chunks = append(m.Chunks, padTestHash(i))
	}
	switch mode {
	case crypto.Convergent:
		for i := 0; i < nChunks; i++ {
			m.ChunkSecrets = append(m.ChunkSecrets, padTestHash(1000+i))
		}
	case crypto.Private:
		m.FileKey = padTestHash(9999)
	}
	return m
}

// TestSecretsPlainLenBoundsEveryEncoding DRIVES the arithmetic in secretsPlainLen instead of
// trusting it. A padding target is a derived figure, and a derived figure is a hypothesis
// until something marshals the real thing against it (scar: silt-derive-then-drive). If the
// target were ever SMALLER than a real encoding, Seal would refuse to publish; if it were
// mode-DEPENDENT, the oracle would be back with every existing gate still green.
//
// The sweep crosses every canonical-CBOR width boundary that the target's arithmetic claims
// to have accounted for: the array header at 23/24 and 255/256 elements, and the FileSize
// integer at 23/24, 255/256, 65535/65536 and 2^32.
func TestSecretsPlainLenBoundsEveryEncoding(t *testing.T) {
	chunkCounts := []int{0, 1, 2, 23, 24, 25, 255, 256, 257, 1000}
	fileSizes := []int64{0, 1, 23, 24, 255, 256, 65535, 65536, 1 << 32, 1<<63 - 1}
	for _, n := range chunkCounts {
		want := secretsPlainLen(n)
		for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
			for _, fs := range fileSizes {
				m := padTestManifest(t, mode, n, fs)
				b, err := encMode.Marshal(secretsPart{
					Mode: m.Mode, FileSize: m.FileSize,
					ChunkSecrets: m.ChunkSecrets, FileKey: m.FileKey,
				})
				if err != nil {
					t.Fatalf("marshal (%s, n=%d, size=%d): %v", mode, n, fs, err)
				}
				if len(b) > want {
					t.Fatalf("secretsPlainLen(%d)=%d does NOT bound the %s encoding at FileSize=%d, which is "+
						"%d bytes. Seal would refuse this manifest outright. Re-derive the arithmetic in "+
						"secretsPlainLen against canonical CBOR widths; a target that a real plaintext can "+
						"exceed is not a padding scheme.", n, want, mode, fs, len(b))
				}
			}
		}
	}
}

// TestSecretsPadIsNeverNegative is the arm that would redden under the ablation named in the
// fix's commit message: derive the target from a SECRET (the mode, the key branch, the
// FileSize) instead of the public chunk count and one of the two branches goes negative here,
// in one package, without publishing anything. It sweeps every chunk count from 0 to 300, so
// it also crosses the array-header boundary at 24 and 256 that the arithmetic claims to
// handle, and it is exhaustive where TestSecretsPlainLenBoundsEveryEncoding samples.
func TestSecretsPadIsNeverNegative(t *testing.T) {
	for n := 0; n <= 300; n++ {
		want := secretsPlainLen(n)
		for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
			for _, fs := range []int64{0, 1, 1 << 20, 1<<63 - 1} {
				m := padTestManifest(t, mode, n, fs)
				b, err := encMode.Marshal(secretsPart{
					Mode: m.Mode, FileSize: m.FileSize,
					ChunkSecrets: m.ChunkSecrets, FileKey: m.FileKey,
				})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				if want-len(b) < 0 {
					t.Fatalf("secretsPlainLen(%d)=%d needs %d pad bytes for the %s encoding at FileSize=%d "+
						"(%d bytes) — a negative pad. Seal refuses this manifest, so publishing in this mode "+
						"at this shape is BROKEN, not merely unpadded.", n, want, want-len(b), mode, fs, len(b))
				}
			}
		}
	}
}

// TestSealedSecretsLengthIsModeIndependent is the property at the layer it is created, one
// step below the pipeline gate: two manifests that differ ONLY in mode seal to blobs of the
// same length. A caretaker holding the layout key measures Layout.Box directly, so this is
// the arm that closes the oracle for the care-link holder — the vantage the manifest-frame
// remedy never reached.
func TestSealedSecretsLengthIsModeIndependent(t *testing.T) {
	var lk, ck [32]byte
	lk[0], ck[0] = 1, 2
	for _, n := range []int{1, 2, 9, 40, 300} {
		conv, err := Seal(padTestManifest(t, crypto.Convergent, n, int64(n)*1000), lk, ck)
		if err != nil {
			t.Fatalf("seal convergent n=%d: %v", n, err)
		}
		priv, err := Seal(padTestManifest(t, crypto.Private, n, int64(n)*1000), lk, ck)
		if err != nil {
			t.Fatalf("seal private n=%d: %v", n, err)
		}
		if len(conv) != len(priv) {
			t.Fatalf("RT-SFO-1 IS BACK AT THE SEAL — a %d-chunk manifest seals to %d bytes convergent and %d "+
				"bytes private. The sealed blob's length is the encryption mode (threat-catalog F8); it is read "+
				"by a keyless stranger through the manifest frame AND by any care-link holder through "+
				"Layout.Box. Fix secretsPlainLen.", n, len(conv), len(priv))
		}
	}
}

// TestPaddedSecretsRoundTrip is the does-it-still-work half. Padding that cannot be opened is
// a different defect, not a fix.
func TestPaddedSecretsRoundTrip(t *testing.T) {
	var lk, ck [32]byte
	lk[0], ck[0] = 3, 4
	for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
		for _, n := range []int{1, 2, 40} {
			m := padTestManifest(t, mode, n, int64(n)*4096)
			blob, err := Seal(m, lk, ck)
			if err != nil {
				t.Fatalf("seal: %v", err)
			}
			got, err := OpenFull(blob, lk, ck)
			if err != nil {
				t.Fatalf("open full (%s, n=%d): %v", mode, n, err)
			}
			if got.Mode != m.Mode || got.FileSize != m.FileSize || len(got.Chunks) != n {
				t.Fatalf("round trip lost fields: %+v", got)
			}
			if mode == crypto.Convergent && len(got.ChunkSecrets) != n {
				t.Fatalf("round trip lost %d chunk secrets", n-len(got.ChunkSecrets))
			}
			if mode == crypto.Private && !bytes.Equal(got.FileKey, m.FileKey) {
				t.Fatal("round trip lost the file key")
			}
			// The caretaker's view opens with the layout key alone and must still work.
			if _, err := OpenLayout(blob, lk); err != nil {
				t.Fatalf("open layout: %v", err)
			}
		}
	}
}

// TestPaddedSecretsRefuseNonZeroTrailer keeps the plaintext canonical. Without this, the
// padding is a free channel INSIDE the box: a publisher could write arbitrary bytes after the
// CBOR and any holder of the content key would read them, while Marshal(Unmarshal(b)) stops
// being the identity the package doc promises.
func TestPaddedSecretsRefuseNonZeroTrailer(t *testing.T) {
	var lk, ck [32]byte
	lk[0], ck[0] = 5, 6
	m := padTestManifest(t, crypto.Private, 1, 10)

	// Build the blob the way Seal does, then forge one byte of the trailer. Hand-building is
	// the point here: this is the only shape Seal itself cannot produce, so a fixture that
	// went through Seal would test nothing.
	sb, err := encMode.Marshal(secretsPart{Mode: m.Mode, FileSize: m.FileSize, FileKey: m.FileKey})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pad := make([]byte, secretsPlainLen(len(m.Chunks))-len(sb))
	if len(pad) == 0 {
		t.Fatal("no padding at one chunk, so this gate cannot exercise the trailer check at all — " +
			"re-derive secretsPlainLen before trusting its green result (the silt-ablation-noop-guard scar)")
	}
	forge := func(trailer []byte) []byte {
		box, err := crypto.SealBox(ck, secretsDomain, append(append([]byte{}, sb...), trailer...))
		if err != nil {
			t.Fatalf("seal box: %v", err)
		}
		eb, err := encMode.Marshal(Layout{
			Version: m.Version, ChunkSize: m.ChunkSize, Chunks: m.Chunks, Box: box,
		})
		if err != nil {
			t.Fatalf("marshal layout: %v", err)
		}
		blob, err := crypto.SealBox(lk, layoutDomain, eb)
		if err != nil {
			t.Fatalf("seal layout: %v", err)
		}
		return blob
	}

	// The control: an honestly zero-filled trailer opens. Without this arm a rejection below
	// could be caused by the hand-built fixture rather than by the trailer.
	if _, err := OpenFull(forge(pad), lk, ck); err != nil {
		t.Fatalf("the hand-built control blob does not open, so the rejection arm below proves nothing: %v", err)
	}

	smuggled := append([]byte{}, pad...)
	smuggled[len(smuggled)-1] = 0xff
	if _, err := OpenFull(forge(smuggled), lk, ck); err == nil {
		t.Fatal("OpenFull accepted a secrets plaintext whose padding is not zero-filled. The pad is then a " +
			"free channel INSIDE the box — a publisher writes arbitrary bytes after the CBOR and every holder " +
			"of the content key reads them — and one manifest stops having exactly one sealed encoding, which " +
			"is the canonicality the package doc promises.")
	}
}
