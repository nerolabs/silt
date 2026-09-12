// Package pipeline is the M1 roundtrip: the full path from a byte stream
// to a published Merkle root and back.
//
//	Add: split → encrypt per chunk → hash → store → manifest → store
//	     manifest as chunks → publish root
//	Get: lookup root → fetch+verify manifest chunks → parse → verify the
//	     chunk list against the root → fetch+verify data chunks → decrypt
//	     → join → size check
//
// Everything speaks through ports; the pipeline neither knows nor cares
// whether the store is a map, a directory, or (later) a swarm of peers.
package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/ports"
)

// DefaultChunkSize is the publish default: 256 KiB (D-R2.9-NODE-HALF-CALLS call 4,
// amended 4′, ratified 2026-09-07; Economist advisory
// silt-agent-memory/economist/reviews/ADVISORY-default-chunk-size-256KiB-2026-09-06.md). One chunk is
// one delivery credit (credit.DeliveryIncrementBytes, pinned in cmd/silt), a full-frame
// shard of it pays a repair-bounty base of exactly 1, and 256 KiB is the largest power of two at which a PoR audit
// still samples every block (p = 1.0; at 64 MiB p = 0.0076). The old comment's "64 MiB
// production minimum" was unenforced folklore: it would cut edge participation per 1 GiB
// object from 6,557 holders to 29. The manifest layer bounds the maximum at
// manifest.MaxChunkSize.
//
// The bounty ground was "a k = 10 stripe pays a base of exactly 10 (the certified D-S7
// threshold of 36 stripe-retrievals per repair)" until 2026-09-12, when F1
// (D-BOUNTY-PRICE-F1-2026-09-12) re-based the price on the ONE shard the bounty's payee
// moves. The ground SURVIVES and tightens: 262,144 B is now the exact zero-bounty cliff,
// to within crypto.Overhead's 16 bytes, and the D-S7 self-funding threshold is 3.60
// stripe-retrievals per shard-repair at m̄ = 3. The other three grounds are untouched.
// RAISING this constant to widen that 16-byte margin is REFUSED: it moves the height-0
// block hash (below). cmd/silt checkBountyDisclosureHeadroom refuses to start below it.
//
// It is a MAXIMUM frame size, not a fixed one. A frame that shares an erasure stripe with
// another frame is padded to it, because shards within a stripe must be equal-length; a
// single-frame object (splitFile) and a single-frame manifest (ManifestFrameSize) are
// framed at their true length, and the length actually used travels in
// manifest.ChunkSize. So a small MANIFEST and a small FILE both escape the padding, and a
// multi-frame file's tail still pays it.
//
// Changing this changes the root every NEW publish of the same bytes produces (convergent
// dedup does not span the boundary); core/genesis keeps its own 64 KiB chunk, and both of
// its frame derivations follow this file, so the height-0 block hash — which covers the
// entry's manifest chunk IDs — MOVED with 4′ (the manifest frame) and again with
// R-SHORT-FINAL-STRIPE (the data frame: the 2,042-byte manifesto is a single-frame
// object). Both moves are owner-accepted on the same ground — no live network exists —
// and core/genesis TestGenesisBlockHashIsPinned holds the literal, so it moves only by an
// explicit, recorded decision.
const DefaultChunkSize = 256 << 10

type Options struct {
	ChunkSize int
	Mode      crypto.Mode
	// Erasure is the (k, n) code; zero value means DefaultParams.
	Erasure erasure.Params
	// Rand supplies key material for private mode. Injected, never
	// defaulted inside core: determinism under test is non-negotiable.
	Rand io.Reader
	// Publisher is recorded in the registry entry; required when the
	// registry is credit-gated, ignored otherwise.
	Publisher ports.NodeID
	// Token, when set, is a quorum-issued publish credential that authorizes
	// the entry WITHOUT a durable Publisher identity (T3, #14/F1). Acquire it
	// (node.AcquireToken) before calling Add and pass it here; the entry then
	// carries the token instead of a publisher.
	Token *ports.PublishToken
	// ManifestFrameBytes fixes the frame size the sealed manifest is split at. 0 (the
	// default) DERIVES it — ManifestFrameSize(len(blob), ChunkSize): one true-length frame
	// when the manifest fits in one chunk. A non-zero value pins it: the pre-4′ padded
	// framing is ManifestFrameBytes == ChunkSize (the dup-publish gate reproduces it that
	// way). The frame reaches the genesis block — entry.ManifestChunks is inside what
	// chain.Block.Hash covers — so changing the derivation moves height-0 identity (blind
	// PE on 4′, 2026-09-07; the owner accepted that move, core/genesis holds the literal).
	ManifestFrameBytes int
}

// Add ingests r and returns the file's silt link: the Merkle root
// (its public name) plus the link key (the private capability). The link
// key is the hash of the plaintext manifest, so convergent content
// yields the same link every time — dedup extends all the way up to the
// handle you share.
//
// Add stages the content and publishes the registry entry in one shot —
// the right path for callers that don't distribute separately (local add,
// genesis, sim). A networked publish that scatters to peers should instead
// Stage, distribute, and publish only once distribution is confirmed, so a
// failed scatter never leaves a dangling registry entry (#65) — see Stage.
func Add(ctx context.Context, store ports.ChunkStore, reg ports.Registry, r io.Reader, opts Options) (link.Handle, error) {
	h, entry, err := Stage(ctx, store, r, opts)
	if err != nil {
		return link.Handle{}, err
	}
	if err := reg.Publish(ctx, entry); err != nil {
		return link.Handle{}, fmt.Errorf("add: publish: %w", err)
	}
	return h, nil
}

// Stage ingests r and stores every chunk plus the sealed manifest, then
// returns the file's handle and the registry entry that names it — WITHOUT
// publishing. The caller publishes (reg.Publish) only after confirming the
// content is distributed, so a loud placement failure never leaves a
// dangling registry entry pointing at content that isn't actually placed
// (register-after-distribute, #65 / tenet S5). The returned entry carries
// the manifest-chunk pointers, so the caller can LoadFull and Distribute
// straight from it without a registry round-trip.
func Stage(ctx context.Context, store ports.ChunkStore, r io.Reader, opts Options) (link.Handle, ports.Entry, error) {
	if opts.ChunkSize == 0 {
		opts.ChunkSize = DefaultChunkSize
	}
	if opts.Erasure == (erasure.Params{}) {
		opts.Erasure = erasure.DefaultParams
	}
	if err := opts.Erasure.Validate(); err != nil {
		return link.Handle{}, ports.Entry{}, fmt.Errorf("add: %w", err)
	}
	frames, err := splitFile(r, opts.ChunkSize)
	if err != nil {
		return link.Handle{}, ports.Entry{}, fmt.Errorf("add: %w", err)
	}
	// The frame size the file was ACTUALLY split at, read off the artifact rather than
	// re-decided: splitFile emits equal-length frames, so frame 0's length is the
	// geometry. It equals opts.ChunkSize except for a single-frame object, which
	// splitFile frames at its true length (R-SHORT-FINAL-STRIPE). Every later consumer
	// of the geometry — the erasure stripe, the PoR auditor's expected block count
	// (core/node/por.go), the repair judge's — reads this field, so the short frame
	// stays a fully committed, auditor-fixed geometry rather than a special case.
	frameBytes := opts.ChunkSize
	if len(frames) > 0 {
		frameBytes = len(frames[0])
	}

	m := &manifest.Manifest{
		Version:   manifest.Version,
		Mode:      string(opts.Mode),
		ChunkSize: int64(frameBytes),
		FileSize:  fileSizeOf(frames),
		K:         opts.Erasure.K,
		N:         opts.Erasure.N,
	}

	var fileKey [crypto.KeySize]byte
	if opts.Mode == crypto.Private {
		if opts.Rand == nil {
			return link.Handle{}, ports.Entry{}, fmt.Errorf("add: private mode requires an injected randomness source")
		}
		fileKey, err = crypto.NewFileKey(opts.Rand)
		if err != nil {
			return link.Handle{}, ports.Entry{}, err
		}
		m.FileKey = fileKey[:]
	}

	ctChunks := make([][]byte, 0, len(frames))
	for i, frame := range frames {
		var ct []byte
		switch opts.Mode {
		case crypto.Convergent:
			var secret [crypto.SecretSize]byte
			ct, secret, err = crypto.ConvergentEncrypt(frame)
			if err != nil {
				return link.Handle{}, ports.Entry{}, fmt.Errorf("add: chunk %d: %w", i, err)
			}
			m.ChunkSecrets = append(m.ChunkSecrets, secret[:])
		case crypto.Private:
			ct, err = crypto.PrivateEncrypt(fileKey, uint64(i), frame)
			if err != nil {
				return link.Handle{}, ports.Entry{}, fmt.Errorf("add: chunk %d: %w", i, err)
			}
		default:
			return link.Handle{}, ports.Entry{}, fmt.Errorf("add: unknown mode %q", opts.Mode)
		}
		c := ports.NewChunk(ct)
		if err := store.Put(ctx, c); err != nil {
			return link.Handle{}, ports.Entry{}, fmt.Errorf("add: storing chunk %d: %w", i, err)
		}
		m.Chunks = append(m.Chunks, c.ID[:])
		ctChunks = append(ctChunks, ct)
	}

	// Erasure-code the ciphertext stream: each stripe of k chunks gains
	// n-k parity shards, stored like any other chunk.
	p := opts.Erasure
	for j := 0; j < p.Stripes(len(ctChunks)); j++ {
		lo := j * p.K
		hi := min(lo+p.K, len(ctChunks))
		parity, err := erasure.EncodeStripe(p, ctChunks[lo:hi])
		if err != nil {
			return link.Handle{}, ports.Entry{}, fmt.Errorf("add: encoding stripe %d: %w", j, err)
		}
		for q, shard := range parity {
			c := ports.NewChunk(shard)
			if err := store.Put(ctx, c); err != nil {
				return link.Handle{}, ports.Entry{}, fmt.Errorf("add: storing parity %d of stripe %d: %w", q, j, err)
			}
			m.Parity = append(m.Parity, c.ID[:])
		}
	}

	root := m.Root()

	// The manifest is sealed before it touches storage: the link key is
	// the hash of the plaintext manifest (deterministic, content-bound),
	// and the stored blob is ciphertext twice over — layout under the
	// layout key, decryption material boxed under the content key.
	// Infrastructure hosts noise describing noise.
	mbytes, err := m.Marshal()
	if err != nil {
		return link.Handle{}, ports.Entry{}, fmt.Errorf("add: %w", err)
	}
	h := link.Handle{Root: root, Key: ports.HashBytes(mbytes)}
	blob, err := manifest.Seal(m, h.LayoutKey(), h.ContentKey())
	if err != nil {
		return link.Handle{}, ports.Entry{}, fmt.Errorf("add: sealing manifest: %w", err)
	}
	frame := opts.ManifestFrameBytes
	if frame <= 0 {
		frame = ManifestFrameSize(len(blob), opts.ChunkSize)
	}
	mframes, err := chunk.Split(bytes.NewReader(blob), frame)
	if err != nil {
		return link.Handle{}, ports.Entry{}, fmt.Errorf("add: chunking manifest: %w", err)
	}
	var manifestIDs []ports.ChunkID
	for i, f := range mframes {
		c := ports.NewChunk(f)
		if err := store.Put(ctx, c); err != nil {
			return link.Handle{}, ports.Entry{}, fmt.Errorf("add: storing manifest chunk %d: %w", i, err)
		}
		manifestIDs = append(manifestIDs, c.ID)
	}

	entry := ports.Entry{
		Root:           root,
		ManifestChunks: manifestIDs,
		FileSize:       m.FileSize,
		Publisher:      opts.Publisher,
		Token:          opts.Token,
	}
	// Deliberately NOT published here: the caller registers it after
	// distribution is confirmed (Add does so immediately; a networked
	// publish waits for a successful scatter) — see #65.
	return h, entry, nil
}

// RegisterAfterDistribute publishes a staged entry to reg only when the
// scatter it names actually succeeded. It is the single gate every networked
// publish (swarm add, the daemon UI) runs from its Distribute callback,
// passing the placement count and error Distribute reported:
//
//	nd.Distribute(entry, m, false, porKey, func(placed int, derr error) {
//	    n, err := pipeline.RegisterAfterDistribute(ctx, reg, entry, placed, derr)
//	    ...
//	})
//
// On a failed scatter (derr != nil) the registry is left untouched and the
// scatter error is returned, so a loud placement failure never leaves a
// dangling entry that names content the swarm can't actually serve
// (register-after-distribute, #65 / tenet S5). Only on a confirmed scatter is
// the entry published — and any publish error is surfaced too, never
// swallowed. Extracting the gate here means both call sites share one tested
// decision instead of duplicating "publish iff derr == nil" by hand.
func RegisterAfterDistribute(ctx context.Context, reg ports.Registry, entry ports.Entry, placed int, derr error) (int, error) {
	if derr != nil {
		return placed, derr // failed scatter → registry untouched
	}
	return placed, reg.Publish(ctx, entry) // confirmed scatter → register now
}

// Get retrieves the file named by root and writes it to w. Every chunk
// is hash-verified on receipt, and the manifest's chunk list is verified
// against the root before any data is fetched — the registry entry
// itself is treated as untrusted routing metadata.
func Get(ctx context.Context, store ports.ChunkStore, reg ports.Registry, h link.Handle, w io.Writer) error {
	entry, ok, err := reg.Lookup(ctx, h.Root)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if !ok {
		return fmt.Errorf("get %s: %w", h.Root, ports.ErrNoSuchEntry)
	}

	m, err := LoadFull(ctx, store, entry, h)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	// The critical check: the manifest we fetched must actually be the
	// one the root names. A registry (or store) pointing us at a
	// different manifest is caught right here.
	if m.Root() != h.Root {
		return fmt.Errorf("get: manifest root %s does not match requested root %s", m.Root(), h.Root)
	}

	mode, err := crypto.ParseMode(m.Mode)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	var fileKey [crypto.KeySize]byte
	if mode == crypto.Private {
		copy(fileKey[:], m.FileKey)
	}

	ctChunks, err := fetchDataChunks(ctx, store, m)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	frames := make([][]byte, 0, len(ctChunks))
	for i, ct := range ctChunks {
		var pt []byte
		switch mode {
		case crypto.Convergent:
			var secret [crypto.SecretSize]byte
			copy(secret[:], m.ChunkSecrets[i])
			pt, err = crypto.ConvergentDecrypt(ct, secret)
		case crypto.Private:
			pt, err = crypto.PrivateDecrypt(fileKey, uint64(i), ct)
		}
		if err != nil {
			return fmt.Errorf("get: chunk %d: %w", i, err)
		}
		frames = append(frames, pt)
	}
	counter := &countingWriter{w: w}
	if err := chunk.Join(counter, frames); err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if counter.n != m.FileSize {
		return fmt.Errorf("get: reassembled %d bytes, manifest says %d", counter.n, m.FileSize)
	}
	return nil
}

// DataFrameSize is the frame size an object of objectBytes is split at, and it is THE
// rule: its own length plus the frame header when the whole object fits in one frame,
// else the chunk size. The boundary is therefore objectBytes <= chunkSize − HeaderSize − 1
// (262,135 B at the 256 KiB default); an object of exactly chunkSize − HeaderSize FILLS
// the first frame and is unchanged. Same shape as ManifestFrameSize, and deliberately so.
//
// splitFile implements this over a STREAM, which cannot know the length ahead, so the two
// must be driven against each other rather than trusted to agree —
// TestDataFrameSizeIsWhatStageCommits does that over the boundary. cmd/silt reads it to
// price a publish before staging it (the repair bounty is computed on the shard that gets
// stored, not on the chunk size that was asked for).
func DataFrameSize(objectBytes, chunkSize int) int {
	if objectBytes <= 0 {
		// An empty object has no frames at all (chunk.Split returns none), so it stores
		// no chunk and no shard and Stage keeps the asked-for geometry. Returning
		// HeaderSize here described a 8-byte frame that is never written, and cmd/silt
		// priced a repair of it — `silt add` on an empty file warned about a 24-byte
		// shard that does not exist (blind PE, 2026-09-09).
		return chunkSize
	}
	if fs := objectBytes + chunk.HeaderSize; fs < chunkSize {
		return fs
	}
	return chunkSize
}

// splitFile frames the file's own bytes. It is chunk.Split with ONE exception: an object
// whose entire content fits in a single frame is framed at its TRUE length instead of
// being padded up to chunkSize (R-SHORT-FINAL-STRIPE, Economist advisory 2026-09-06 §4
// rider 2). Such a frame is ALONE in its erasure stripe, so nothing forces it to full
// length — equal-length shards are a within-stripe constraint — and the six parity shards
// are computed at that same true length. A 1 KB object at the 256 KiB default stored
// 7 × 262,160 = 1,835,120 B of mostly zeros; it now stores 7 × 1,048 B.
//
// Every OTHER object is byte-identical to before: two or more frames share a stripe with
// each other, so the tail stays padded and chunk.Split does the whole job. The frame size
// travels in manifest.ChunkSize, so the audit, the repair judge and the reader all read
// the geometry that was used rather than the one that was asked for. It can legitimately
// be as small as chunk.MinChunkSize (a 1-byte object commits ChunkSize = 9), so a
// consumer that divides by the field or assumes a floor must say so.
//
// This changes the chunk IDs — and therefore the root — of every object at or below
// chunkSize − 9. See DefaultChunkSize on what that costs and what it does not.
func splitFile(r io.Reader, chunkSize int) ([][]byte, error) {
	if chunkSize < chunk.MinChunkSize {
		return chunk.Split(r, chunkSize) // one place reports a bad chunk size
	}
	head := make([]byte, chunkSize-chunk.HeaderSize)
	n, err := io.ReadFull(r, head)
	switch {
	case err == io.EOF:
		return nil, nil // empty input yields zero frames, exactly as chunk.Split does
	case err == io.ErrUnexpectedEOF:
		// The whole object arrived inside one frame, so its length is known now.
		return chunk.Split(bytes.NewReader(head[:n]), DataFrameSize(n, chunkSize))
	case err != nil:
		return nil, err
	}
	// The object filled the first frame, so it is not a single short frame: hand the
	// bytes back to chunk.Split unchanged, prefix and all.
	return chunk.Split(io.MultiReader(bytes.NewReader(head), r), chunkSize)
}

// ManifestFrameSize is the frame size a sealed manifest blob of blobLen bytes is split
// at: the blob's own length plus the frame header when it fits in one chunk, else the
// data chunk size. Manifests carry no parity, so padding them to the data chunk size
// bought nothing — on the first production store 87.6 % of objects were 1.4 KB manifests
// padded to 65,536 B (R-MANIFEST-PADDING, Economist advisory 2026-09-06); at a 256 KiB
// default that store would have grown 3.9× for no content. chunk.Join accepts frames of
// any size ≥ chunk.MinChunkSize, so a true-length single frame round-trips unchanged.
func ManifestFrameSize(blobLen, chunkSize int) int {
	if fs := blobLen + chunk.HeaderSize; fs < chunkSize {
		return fs
	}
	return chunkSize
}

// LoadBlob fetches, verifies, and joins the sealed manifest blob
// referenced by a registry entry — still ciphertext.
func LoadBlob(ctx context.Context, store ports.ChunkStore, entry ports.Entry) ([]byte, error) {
	var mbuf bytes.Buffer
	mframes := make([][]byte, 0, len(entry.ManifestChunks))
	for _, id := range entry.ManifestChunks {
		c, err := store.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("manifest chunk: %w", err)
		}
		if !c.Verify() {
			return nil, fmt.Errorf("manifest chunk %s: %w", id, ports.ErrCorrupt)
		}
		mframes = append(mframes, c.Data)
	}
	if err := chunk.Join(&mbuf, mframes); err != nil {
		return nil, fmt.Errorf("reassembling manifest: %w", err)
	}
	return mbuf.Bytes(), nil
}

// LoadFull opens the manifest with the full link: layout + secrets.
func LoadFull(ctx context.Context, store ports.ChunkStore, entry ports.Entry, h link.Handle) (*manifest.Manifest, error) {
	blob, err := LoadBlob(ctx, store, entry)
	if err != nil {
		return nil, err
	}
	return manifest.OpenFull(blob, h.LayoutKey(), h.ContentKey())
}

// LoadLayout opens only the outer layer with a care link: structure
// without secrets — the caretaker's whole world.
func LoadLayout(ctx context.Context, store ports.ChunkStore, entry ports.Entry, ch link.CareHandle) (*manifest.Layout, error) {
	blob, err := LoadBlob(ctx, store, entry)
	if err != nil {
		return nil, err
	}
	l, err := manifest.OpenLayout(blob, ch.LayoutKey)
	if err != nil {
		return nil, err
	}
	if l.Root() != ch.Root {
		return nil, fmt.Errorf("layout root %s does not match care link root %s", l.Root(), ch.Root)
	}
	return l, nil
}

// fetchDataChunks returns every ciphertext data chunk of the file, in
// order, reconstructing missing or corrupt ones from parity when the
// manifest is erasure-coded. A shard that fails its hash check is
// treated exactly like a lost shard — to the decoder, corruption IS
// loss. Every reconstructed chunk is re-verified against the manifest's
// (root-committed) hash before use.
func fetchDataChunks(ctx context.Context, store ports.ChunkStore, m *manifest.Manifest) ([][]byte, error) {
	dataIDs := m.ChunkIDs()
	if m.K == 0 { // uncoded manifest: every chunk must be present
		out := make([][]byte, len(dataIDs))
		for i, id := range dataIDs {
			c, err := store.Get(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("chunk %d: %w (no erasure coding to recover with)", i, err)
			}
			out[i] = c.Data
		}
		return out, nil
	}

	p := erasure.Params{K: m.K, N: m.N}
	parityIDs := m.ParityIDs()
	out := make([][]byte, len(dataIDs))
	for j := 0; j < p.Stripes(len(dataIDs)); j++ {
		lo := j * p.K
		hi := min(lo+p.K, len(dataIDs))
		realData := hi - lo

		shards := make([][]byte, p.N)
		missing := 0
		for i := 0; i < realData; i++ {
			c, err := store.Get(ctx, dataIDs[lo+i])
			if err != nil {
				missing++ // lost or corrupt — parity's problem now
				continue
			}
			shards[i] = c.Data
		}
		if missing > 0 {
			for q := 0; q < p.ParityShards(); q++ {
				c, err := store.Get(ctx, parityIDs[j*p.ParityShards()+q])
				if err != nil {
					continue
				}
				shards[p.K+q] = c.Data
			}
			if err := erasure.ReconstructStripe(p, shards, realData); err != nil {
				return nil, fmt.Errorf("stripe %d: %d data shard(s) lost and %w", j, missing, err)
			}
			// Reconstruction is only trusted if the recovered bytes hash
			// to the IDs the Merkle root committed to.
			for i := 0; i < realData; i++ {
				if ports.HashBytes(shards[i]) != dataIDs[lo+i] {
					return nil, fmt.Errorf("stripe %d: reconstructed chunk %d does not match its manifest hash", j, lo+i)
				}
			}
		}
		copy(out[lo:hi], shards[:realData])
	}
	return out, nil
}

func fileSizeOf(frames [][]byte) int64 {
	var total int64
	for _, f := range frames {
		total += int64(frameLen(f))
	}
	return total
}

func frameLen(f []byte) int {
	n := 0
	for i := 0; i < chunk.HeaderSize; i++ {
		n = n<<8 | int(f[i])
	}
	return n
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
