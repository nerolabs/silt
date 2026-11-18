// Package bond is the identity-bound storage commitment that puts a PRICE ON
// IDENTITY. To hold consensus standing (core/credit Reputation), a node must keep a
// large, per-identity plot it can be challenged on at any moment. Two identities are
// two distinct plots, each occupying its full size on disk, so N identities cost
// N × size of real storage.
//
// THE PLOT is a labeling of a depth-robust graph with n = size/LabelSize nodes. Node
// i's label is
//
//	label(i) = H(0x00 ‖ seed ‖ i ‖ label(i-1) ‖ label(p₁) ‖ label(p₂) ‖ label(p₃))
//
// where p₁..p₃ are DRSample parents (Alwen–Blocki–Harsha, "Practical Graphs for
// Optimal Side-Channel Resistant Proofs of Work", CCS'17) drawn from the public seed
//
//	seed = H("silt/bond/plot/v4" ‖ pk ‖ n) // pk = validator ed25519 key
//
// The stored plot IS the labels, 32 bytes per node, back to back. No byte of it is an
// expansion of something shorter: every 32 bytes is one hash output, so a prover
// holding less than the plot cannot answer without recomputing the missing labels
// from the seed, and the chain edge and the DRSample parents make that recompute a
// long sequential walk back through the graph. A plot whose nodes were expanded from shorter labels would be
// compressible to those labels, and the space charged would be a fraction of the
// space held; that is why a node is its label and nothing more.
//
// Folding pk into the seed binds the plot to one identity: labels sealed for one key
// fail recomputation under another. Folding n binds it to one size: a prefix of an
// n-node plot is not a valid smaller plot.
//
// THE COMMITMENT is a binary Merkle tree whose leaves are the labels, and its root is
// public. A prover keeps the tree only from the block level up, one hash per
// BlockSize block of labels, and recomputes the levels below from a block's bytes
// when a proof needs them. The tree therefore costs 1/64 of the plot in memory.
//
// THE ANSWER. The verifier derives every opened index from public data, so an answer
// carries no indices: only the opened labels in index order, one Merkle multiproof
// covering all of them (a path node shared by several opens is sent once), and the
// VDF proof. A challenge opens
//   - Samples possession labels, which catch a prover missing part of its plot;
//   - k labeling opens, each a node with its predecessor and DRSample parents. The
//     verifier recomputes the node's label from the opened parents and requires a
//     match, which catches a prover holding bytes it did not plot for (pk, n).
//     Against an ε-short prover the soundness error is ≤ (1-ε)^k;
//   - for a space-time answer, the seed label read before the VDF runs.
//
// Verification is O((Samples + 5k)·log n) hashes and one VDF check: no fetch, no VDF
// re-run.
//
// SPACE-TIME. The challenge first selects one label (seedIndex), and the VDF is
// seeded from that label (challengeSeedST), so the sequential work starts only after
// the plot is read. The VDF output then selects the possession samples and the
// labeling opens, so which labels must be resident is unknown until the work is
// done. Tuned so re-plotting takes far longer than one epoch, this makes releasing
// the plot and recomputing it on demand uneconomical.
//
// WHAT THIS DOES NOT PROVE. The tight ε→k constant through indegree-4 DRSample and
// the re-plot-≫-epoch floor are claimed, not proven here. There is no replication
// proof and no zero-knowledge: it proves "this identity holds a distinct plot of this
// size," not "this is a unique replica of user data." Elevating held network content
// to standing, so that the Sybil cost and durability funding become one mechanism, is
// the intended follow-up; the synthetic plot is the cold start.
//
// ADVERSARY-SHAPE: capability=SybilPlotSharing UNCOVERED: no fixture GRANTS AND CONTROLS FOR one operator N identities backed by LESS than N x size of real disk (dedup, compression, or a shared plot).
package bond

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/bits"
	"slices"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/vdf"
	"github.com/nerolabs/silt/ports"
)

// EncodeAnswer / DecodeAnswer serialize a challenge answer for the wire
// (MsgBondReply carries it in Message.Data). CBOR is the codec the chain
// already uses.
func EncodeAnswer(a Answer) ([]byte, error) { return cbor.Marshal(a) }

func DecodeAnswer(b []byte) (Answer, error) {
	var a Answer
	err := cbor.Unmarshal(b, &a)
	return a, err
}

const (
	// BlockSize is the unit a plot is stored, read and written in. A block holds
	// LabelsPerBlock consecutive labels.
	BlockSize = 4 << 10
	// LabelSize is the width of one plot node: one SHA-256 output.
	LabelSize = sha256.Size
	// LabelsPerBlock is how many labels one block holds. It is a power of two, so a
	// block's labels are exactly one complete subtree of the commitment.
	LabelsPerBlock = BlockSize / LabelSize
	// blockDepth is the height of that subtree, log2(LabelsPerBlock).
	blockDepth = 7
	// PlotSealThroughput is the measured plot-sealing rate (see BenchmarkSeal),
	// in bytes/second. It exists so the anti-release floor can be sized from a
	// real, named constant instead of a magic number: a bond must be too large to
	// re-seal inside the anti-release COMPUTE window (MinBondBytes >
	// window × PlotSealThroughput). Build-immutable #3/#4: this window is a
	// COMPUTE budget, deliberately decoupled from any transport/routing timeout —
	// raising a network timeout for durability must never move this floor.
	//
	// The plot format whose rate this is expanded each 32-byte label into a 4 KiB
	// block. The current format hashes every 32 bytes sequentially and seals at about
	// 40% of this rate (BenchmarkSeal), so a floor derived from this constant is
	// larger than the formula requires.
	PlotSealThroughput = 270 * 1000 * 1000 // ~270 MB/s
	// Samples is how many independent labels one challenge probes for POSSESSION.
	// A prover missing a fraction f of its plot slips through with probability
	// (1-f)^Samples, so 20 samples makes even a 10%-short plot fail ~88% of the
	// time and a 30%-short plot fail ~99.9%. This catches a prover MISSING labels;
	// the labeling opens (DefaultLabelSamples) catch one holding arbitrary, short or
	// reused bytes.
	Samples = 20
	// DefaultLabelSamples is the default number k of labeling-consistency opens per
	// challenge: each open recomputes one node's label from its opened parents,
	// catching a prover that committed bytes it did not correctly plot for (pk, n).
	// Against an ε-short prover the soundness error is ≤ (1-ε)^k, so k=64 catches a
	// ~30% cheat at ≈2^-33 and a ~50% cheat at 2^-64. It is a per-network EVOLVING
	// knob (node Config.BondLabelSamples); any k<=0 passed to Answer/Verify resolves
	// to this, so a zero-config caller (sim/test) still runs the full check rather
	// than silently disabling it.
	DefaultLabelSamples = 64
	// sealWindowBytes is how much of a plot SealInto keeps in memory while sealing:
	// the most recent labels, which is where most DRSample parents fall. A parent
	// older than the window is read back from the store. A parent's distance is drawn
	// log-uniformly, so a window of 2^w labels holds about w/log2(i) of label i's
	// parents: roughly 92% at a 1 GiB plot and 85% at 5 GiB for this window.
	sealWindowBytes = 256 << 20
)

// Hash-input tags. Each hash the plot is built from starts with its own tag, which
// keeps labels, interior tree nodes and parent draws in disjoint input domains.
const (
	tagLabel   = 0x00
	tagNode    = 0x01
	tagParents = 0x02
)

// resolveK maps a caller's k to the effective count, defaulting a non-positive
// value to DefaultLabelSamples so the labeling check can never be silently
// disabled by an unset config.
func resolveK(k int) int {
	if k <= 0 {
		return DefaultLabelSamples
	}
	return k
}

// NumBlocks is how many BlockSize blocks a bond of size bytes holds. It is
// derived from the PUBLIC size, so prover and verifier agree without
// exchanging it.
func NumBlocks(size int64) int {
	if size <= 0 {
		return 1
	}
	return int((size + BlockSize - 1) / BlockSize)
}

// NumLabels is how many labels, and so how many graph nodes, a bond of size bytes
// holds. Every block is full, so it is NumBlocks(size) × LabelsPerBlock.
func NumLabels(size int64) int { return NumBlocks(size) * LabelsPerBlock }

// Commitment is a sealed, identity-bound bond held on disk by its owner. Root is
// public (published once, cheap to gossip); the plot is the cost the owner carries to
// keep answering. pk and seed are the public, size-bound identity seed the plot was
// labeled from, retained so the owner can open labels and stamp its key on answers.
type Commitment struct {
	Size int64
	Root ports.Hash
	pk   []byte
	seed []byte
	// n is the plot's label count.
	n      int
	blocks [][]byte
	// src backs a plot that lives on disk rather than in memory. Exactly one of
	// blocks and src is set: blocks for a plot sealed into memory, src for one
	// streamed to a store. Answering reads a few hundred labels per challenge, so the
	// streamed plot pays that many random block reads where the resident one pays its
	// whole size in RAM.
	src      ports.PlotBlocks
	released bool
	// upper is the commitment tree from the block level up: upper[0] holds one root
	// per block, the root of its LabelsPerBlock labels, and the last level holds Root
	// alone. The levels below a block root are recomputed from the block's bytes when
	// a proof needs them, so the owner keeps two hashes per block rather than two per
	// label.
	upper [][]ports.Hash
}

// plotSeedN is the PUBLIC, identity- and size-bound plot seed for validator key pk
// and label count n: H("silt/bond/plot/v4" ‖ pk ‖ n). Folding pk binds the plot to
// one identity (a plot for pk_A fails recomputation under pk_B); folding n binds it
// to one size (a prefix of an n-plot is not a valid m-plot). It is public so the
// verifier can recompute labels without holding the plot.
func plotSeedN(pk []byte, n int) []byte {
	h := sha256.New()
	h.Write([]byte(plotDomain))
	h.Write(pk)
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], uint64(n))
	h.Write(nb[:])
	return h.Sum(nil)
}

// block returns plot block i, from memory or from the backing store, or nil if
// this commitment cannot produce it — out of range, never written, or released.
func (c *Commitment) block(i int) []byte {
	if c.released || i < 0 || i >= c.n/LabelsPerBlock {
		return nil
	}
	if c.blocks != nil {
		if i < len(c.blocks) {
			return c.blocks[i]
		}
		return nil
	}
	if c.src == nil {
		return nil
	}
	buf := make([]byte, BlockSize)
	if err := c.src.ReadBlock(i, buf); err != nil {
		return nil
	}
	return buf
}

// commit builds the commitment over a plot whose block roots are known.
func commit(pk []byte, size int64, seed []byte, n int, roots []ports.Hash, blocks [][]byte, src ports.PlotBlocks) *Commitment {
	upper := upperTree(roots)
	return &Commitment{Size: size, Root: upper[len(upper)-1][0], pk: append([]byte(nil), pk...),
		seed: seed, n: n, blocks: blocks, src: src, upper: upper}
}

// SealInto plots the same bond Seal does, streaming each block to dst as it fills.
// It keeps the most recent sealWindowBytes of labels in memory and reads an older
// parent back from dst. Peak memory is that window plus the block-level tree, 1/64
// of the plot, instead of the plot itself.
//
// The result is byte-identical to Seal's for the same (pk, size): the labels are a
// pure function of the public seed and the plot's own earlier labels, and this
// changes only where those labels are kept while they are produced.
func SealInto(pk []byte, size int64, dst ports.PlotBlocks) (*Commitment, error) {
	return sealInto(pk, size, dst, sealWindowBytes)
}

func sealInto(pk []byte, size int64, dst ports.PlotBlocks, window int) (*Commitment, error) {
	nb := NumBlocks(size)
	n := nb * LabelsPerBlock
	seed := plotSeedN(pk, n)
	// The window is a whole number of blocks, so a block's labels sit contiguously in
	// it and can be written out in place.
	w := min(max(window/BlockSize, 1)*LabelsPerBlock, n)
	ring := make([]byte, w*LabelSize)
	far := make([]byte, BlockSize)
	farBlock := -1
	roots := make([]ports.Hash, nb)
	var scratch blockScratch

	i := 0
	var readErr error
	get := func(j int) (l ports.Hash) {
		if j >= i-w {
			off := (j % w) * LabelSize
			copy(l[:], ring[off:off+LabelSize])
			return l
		}
		if b := j / LabelsPerBlock; b != farBlock {
			if err := dst.ReadBlock(b, far); err != nil {
				readErr = fmt.Errorf("bond: reading plot block %d back while sealing: %w", b, err)
				return l
			}
			farBlock = b
		}
		off := (j % LabelsPerBlock) * LabelSize
		copy(l[:], far[off:off+LabelSize])
		return l
	}
	for ; i < n; i++ {
		l := labelOf(seed, i, get)
		if readErr != nil {
			return nil, readErr
		}
		copy(ring[(i%w)*LabelSize:], l[:])
		if (i+1)%LabelsPerBlock == 0 {
			b := i / LabelsPerBlock
			start := ((b * LabelsPerBlock) % w) * LabelSize
			blk := ring[start : start+BlockSize]
			if err := dst.WriteBlock(b, blk); err != nil {
				return nil, fmt.Errorf("bond: writing plot block %d: %w", b, err)
			}
			roots[b] = blockRoot(blk, &scratch)
		}
	}
	return commit(pk, size, seed, n, roots, nil, dst), nil
}

// Seal plots the identity-bound bond of ~size bytes for validator key pk from the
// PUBLIC seed H(pk, n) (see the package doc: the seed binds the plot to its owner AND
// its size, and makes each identity's plot distinct and verifiable). Labels are
// generated in order because each depends on earlier ones, so this is the
// deliberately sequential "plotting" step; the owner then STORES the result to answer
// challenges cheaply. Same (pk, size) ⇒ same plot, so an owner can regenerate.
//
// Seal holds the whole plot in memory. A node with a plot store seals with SealInto.
func Seal(pk []byte, size int64) *Commitment {
	nb := NumBlocks(size)
	n := nb * LabelsPerBlock
	seed := plotSeedN(pk, n)
	plot := make([]byte, n*LabelSize)
	get := func(j int) (l ports.Hash) { copy(l[:], plot[j*LabelSize:(j+1)*LabelSize]); return l }
	blocks := make([][]byte, nb)
	roots := make([]ports.Hash, nb)
	var scratch blockScratch
	for i := 0; i < n; i++ {
		l := labelOf(seed, i, get)
		copy(plot[i*LabelSize:], l[:])
		if (i+1)%LabelsPerBlock == 0 {
			b := i / LabelsPerBlock
			blocks[b] = plot[b*BlockSize : (b+1)*BlockSize : (b+1)*BlockSize]
			roots[b] = blockRoot(blocks[b], &scratch)
		}
	}
	return commit(pk, size, seed, n, roots, blocks, nil)
}

// Blocks exposes the plot blocks so the owner can persist them
// (ports.PlotStore) and reload on restart instead of re-plotting.
func (c *Commitment) Blocks() [][]byte { return c.blocks }

// ReleaseBlocks drops the resident plot bytes, keeping only the commitment (Root,
// Size) and the block-level tree — the state of a prover that pledged the space,
// advertised the bond, then freed the bytes to save disk, intending to recompute on
// demand. AnswerSpaceTime needs the seed label, the sampled labels and the labeling
// opens' parents, which it no longer holds, and recomputing them is the depth-robust
// walk the labeling exists to force. So a released commitment fails a live audit,
// the wire-level consequence the sim asserts.
func (c *Commitment) ReleaseBlocks() { c.blocks, c.src, c.released = nil, nil, true }

// ReconstructFrom rebuilds a Commitment over a plot held in a store rather than in
// memory, RE-DERIVING the tree and Merkle root from the stored bytes rather than
// trusting a persisted root. It reads each block once to hash it and keeps none of
// them, so reloading a plot on restart costs the block-level tree, 1/64 of the plot,
// instead of the plot's own size.
//
// The caller should check the returned Root against the root it persisted, which
// catches silent on-disk corruption.
func ReconstructFrom(pk []byte, size int64, src ports.PlotBlocks) (*Commitment, error) {
	nb := NumBlocks(size)
	n := nb * LabelsPerBlock
	roots := make([]ports.Hash, nb)
	buf := make([]byte, BlockSize)
	var scratch blockScratch
	for b := range roots {
		if err := src.ReadBlock(b, buf); err != nil {
			return nil, fmt.Errorf("bond: reading stored plot block %d: %w", b, err)
		}
		roots[b] = blockRoot(buf, &scratch)
	}
	return commit(pk, size, plotSeedN(pk, n), n, roots, nil, src), nil
}

// Reconstruct rebuilds a Commitment from persisted plot blocks for validator key pk,
// RE-DERIVING the tree and Merkle root from the bytes rather than trusting a stored
// root. It errors if the block count doesn't match size or any block is the wrong
// length — a corrupt or stale plot the caller should discard and re-plot. The caller
// should additionally check the returned Root equals the root it persisted, catching
// silent on-disk corruption. The plot store's format version makes an older plot
// read as absent, so a reload never builds a commitment over another format's bytes.
func Reconstruct(pk []byte, size int64, blocks [][]byte) (*Commitment, error) {
	nb := NumBlocks(size)
	if len(blocks) != nb {
		return nil, fmt.Errorf("bond: plot has %d blocks, want %d for size %d", len(blocks), nb, size)
	}
	for i, b := range blocks {
		if len(b) != BlockSize {
			return nil, fmt.Errorf("bond: plot block %d is %d bytes, want %d", i, len(b), BlockSize)
		}
	}
	n := nb * LabelsPerBlock
	return commitBlocks(pk, size, plotSeedN(pk, n), blocks), nil
}

// commitBlocks builds the commitment over resident blocks labeled from seed.
func commitBlocks(pk []byte, size int64, seed []byte, blocks [][]byte) *Commitment {
	roots := make([]ports.Hash, len(blocks))
	var scratch blockScratch
	for b, blk := range blocks {
		roots[b] = blockRoot(blk, &scratch)
	}
	return commit(pk, size, seed, len(blocks)*LabelsPerBlock, roots, blocks, nil)
}

// Answer is a prover's response to a challenge. The verifier derives every opened
// index itself from public data (the root, the size, the nonce and, for space-time,
// the VDF output), so the answer carries only the opened labels and what it takes to
// check them.
type Answer struct {
	// PK is the prover's ed25519 public key. The plot seed is H(PK, n), so the
	// verifier needs PK to recompute labels; the caller BINDS it (the live audit
	// checks sha256(PK)==challenged NodeID, the chain uses BondReg.Validator as the
	// authoritative key). It is already public (NodeID = sha256(PK), gossiped bond
	// roots are already NodeID-linked), so carrying it leaks nothing new.
	PK []byte `cbor:"1,keyasint,omitempty"`
	// Labels are the opened labels, LabelSize bytes each, in ascending index order
	// of the opened set (openedIndices).
	Labels []byte `cbor:"2,keyasint,omitempty"`
	// Siblings is the multiproof for Labels: the tree nodes the verifier cannot
	// compute from the opened labels, LabelSize bytes each, in the order foldProof
	// consumes them.
	Siblings []byte `cbor:"3,keyasint,omitempty"`
	// VDF proof (core/vdf) for a space-time answer: it attests that the prover did
	// VDFT sequential squarings over the challenge, and the opened labels are derived
	// from VDFY — so the prover cannot know which labels to hold until the sequential
	// work is done. Empty ⇒ space-only.
	//
	// ADVERSARY-SHAPE: capability=VDFOutputPrediction UNCOVERED: no fixture GRANTS AND CONTROLS FOR a prover that learns the probed indices before the sequential work completes.
	VDFY  []byte `cbor:"4,keyasint,omitempty"`
	VDFPi []byte `cbor:"5,keyasint,omitempty"`
	VDFT  uint64 `cbor:"6,keyasint,omitempty"`
}

// Answer builds the space-only response for nonce from the held plot, including k
// labeling opens. It returns false if the owner no longer holds an opened label
// (i.e. it cannot prove the bond it committed to).
func (c *Commitment) Answer(nonce uint64, k int) (Answer, bool) {
	labels, sib, ok := c.open(openedIndices(c.seed, c.Root, c.n, nonce, resolveK(k)))
	if !ok {
		return Answer{}, false
	}
	return Answer{PK: c.pk, Labels: labels, Siblings: sib}, true
}

// AnswerSpaceTime is the proof-of-space-TIME response. It binds the VDF to a plot
// READ done before the VDF runs: it first reads the label at seedIndex(root, nonce),
// seeds the VDF from it, runs `delay` sequential squarings, then derives the
// possession samples AND the k labeling opens from the VDF output. The seed requires
// a plot label, and re-deriving that label on demand is the depth-robust recompute,
// so a prover that released the plot cannot cheaply produce the seed, and releasing
// the plot forfeits the answer. delay == 0 falls back to a space-only answer.
//
// ADVERSARY-SHAPE: capability=OnDemandPlotRecompute UNCOVERED: no fixture GRANTS AND CONTROLS FOR a zero-resident prover a cheap recompute of an arbitrary plot block.
func (c *Commitment) AnswerSpaceTime(nonce uint64, p vdf.Params, delay uint64, k int) (Answer, bool) {
	if delay == 0 {
		return c.Answer(nonce, k)
	}
	si := seedIndex(c.Root, c.n, nonce)
	seedLabel, ok := c.label(si)
	if !ok {
		return Answer{}, false
	}
	proof, err := vdf.Eval(p, challengeSeedST(c.Root, nonce, seedLabel[:]), delay)
	if err != nil {
		return Answer{}, false
	}
	idx := openedIndices(c.seed, c.Root, c.n, vdfDerivedNonce(proof.Y), resolveK(k), si)
	labels, sib, ok := c.open(idx)
	if !ok {
		return Answer{}, false
	}
	return Answer{PK: c.pk, Labels: labels, Siblings: sib, VDFY: proof.Y, VDFPi: proof.Pi, VDFT: proof.T}, true
}

// label reads plot label j.
func (c *Commitment) label(j int) (ports.Hash, bool) {
	var h ports.Hash
	blk := c.block(j / LabelsPerBlock)
	if blk == nil {
		return h, false
	}
	off := (j % LabelsPerBlock) * LabelSize
	copy(h[:], blk[off:off+LabelSize])
	return h, true
}

// open reads the labels at idx and builds their multiproof. It returns false if the
// owner no longer holds a block it needs, or if the blocks it holds no longer fold to
// the committed root (on-disk corruption), so it never sends an answer that would
// fail.
func (c *Commitment) open(idx []int) (labels, siblings []byte, ok bool) {
	trees := map[int]*blockLevels{}
	node := func(level, x int) (ports.Hash, bool) {
		if level >= blockDepth {
			lv := c.upper[level-blockDepth]
			if x >= len(lv) {
				return ports.Hash{}, false
			}
			return lv[x], true
		}
		shift := blockDepth - level
		b := x >> shift
		t, ok := trees[b]
		if !ok {
			blk := c.block(b)
			if blk == nil {
				return ports.Hash{}, false
			}
			t = blockTree(blk)
			trees[b] = t
		}
		return t[level][x-b<<shift], true
	}
	leaves := make([]ports.Hash, len(idx))
	for j, x := range idx {
		h, ok := node(0, x)
		if !ok {
			return nil, nil, false
		}
		leaves[j] = h
	}
	root, ok := foldProof(c.n, idx, leaves, func(level, x int) (ports.Hash, bool) {
		h, ok := node(level, x)
		if ok {
			siblings = append(siblings, h[:]...)
		}
		return h, ok
	})
	if !ok || root != c.Root {
		return nil, nil, false
	}
	labels = make([]byte, 0, len(idx)*LabelSize)
	for _, h := range leaves {
		labels = append(labels, h[:]...)
	}
	return labels, siblings, true
}

// Verify checks a space-only answer against ONLY the committed root — no ground
// truth, no held blocks on the verifier's side. Passing requires the opened labels to
// fold to the root AND the k labeling opens to recompute correctly, which requires a
// plot correctly labeled for (pk, size). pk is the authoritative key the caller
// trusts (see Answer.PK); labels are recomputed from H(pk, n).
func Verify(pk []byte, root ports.Hash, size int64, nonce uint64, a Answer, k int) bool {
	n := NumLabels(size)
	seed := plotSeedN(pk, n)
	kk := resolveK(k)
	idx := openedIndices(seed, root, n, nonce, kk)
	labels, ok := checkOpened(root, n, idx, a)
	return ok && checkLabels(seed, root, n, nonce, kk, idx, labels)
}

// VerifySpaceTime checks a proof-of-space-time answer: the opened labels must fold
// to the committed root, the k labeling opens must recompute correctly for (pk,
// size), and the VDF must attest the required delay over a challenge seeded from the
// committed seed label. The seed label is opened under the same multiproof, so a
// prover that released the plot cannot present a seed label that folds to the
// committed root. It stays O(log n) on the verifier: the whole point of the VDF is
// that checking it is fast even though producing it was slow.
//
// ADVERSARY-SHAPE: capability=SeedBlockInclusionProof UNCOVERED: no fixture GRANTS AND CONTROLS FOR a prover that presents a seed block it does not store with a proof this verifier accepts.
func VerifySpaceTime(pk []byte, root ports.Hash, size int64, nonce uint64, a Answer, p vdf.Params, delay uint64, k int) bool {
	if delay == 0 {
		return Verify(pk, root, size, nonce, a, k)
	}
	if a.VDFT != delay {
		return false // must attest exactly the required amount of work
	}
	n := NumLabels(size)
	seed := plotSeedN(pk, n)
	kk := resolveK(k)
	si := seedIndex(root, n, nonce)
	eff := vdfDerivedNonce(a.VDFY)
	idx := openedIndices(seed, root, n, eff, kk, si)
	labels, ok := checkOpened(root, n, idx, a)
	if !ok || !checkLabels(seed, root, n, eff, kk, idx, labels) {
		return false
	}
	j, _ := slices.BinarySearch(idx, si)
	return vdf.Verify(p, challengeSeedST(root, nonce, labels[j][:]), vdf.Proof{Y: a.VDFY, Pi: a.VDFPi, T: a.VDFT})
}

// checkOpened verifies that a.Labels are the committed labels at idx: exactly one
// label per index, and a multiproof that folds them to root using every sibling it
// carries and no more.
func checkOpened(root ports.Hash, n int, idx []int, a Answer) ([]ports.Hash, bool) {
	if len(a.Labels) != len(idx)*LabelSize || len(a.Siblings)%LabelSize != 0 {
		return nil, false
	}
	leaves := make([]ports.Hash, len(idx))
	for j := range leaves {
		copy(leaves[j][:], a.Labels[j*LabelSize:])
	}
	rest := a.Siblings
	got, ok := foldProof(n, idx, leaves, func(_, _ int) (ports.Hash, bool) {
		var h ports.Hash
		if len(rest) < LabelSize {
			return h, false
		}
		copy(h[:], rest)
		rest = rest[LabelSize:]
		return h, true
	})
	return leaves, ok && len(rest) == 0 && got == root
}

// checkLabels is the labeling-consistency check. For each of the k challenged nodes
// it RECOMPUTES the node's label from the opened predecessor and DRSample parents
// under the public seed H(pk, n) and requires it to equal the opened node. An
// attacker committing arbitrary, reused or wrong-size bytes cannot satisfy this for a
// node it did not correctly plot for (pk, n), so a challenged node it faked fails
// deterministically, and k independent challenges catch an ε-short prover with
// probability ≥ 1-(1-ε)^k. Because the seed is public, the
// verifier does all of this WITHOUT holding the plot. idx must already be verified
// (checkOpened) and hold every index the opens need, which openedIndices guarantees.
//
// ADVERSARY-SHAPE: capability=ForeignPlotLabels UNCOVERED: no fixture GRANTS AND CONTROLS FOR an attacker labels correctly plotted for a DIFFERENT (pk, n) to replay against this one.
func checkLabels(seed []byte, root ports.Hash, n int, effNonce uint64, k int, idx []int, labels []ports.Hash) bool {
	get := func(j int) ports.Hash {
		at, _ := slices.BinarySearch(idx, j)
		return labels[at]
	}
	for _, v := range labelIndices(root, n, effNonce, k) {
		at, _ := slices.BinarySearch(idx, v)
		if labelOf(seed, v, get) != labels[at] {
			return false
		}
	}
	return true
}

// openedIndices lists every label an answer opens for effNonce, ascending and
// without repeats: the possession samples, each labeling open's node, predecessor and
// parents, and extra (the space-time seed label). Prover and verifier both derive it,
// so an answer needs to carry none of it.
func openedIndices(seed []byte, root ports.Hash, n int, effNonce uint64, k int, extra ...int) []int {
	set := append([]int(nil), extra...)
	set = append(set, challengeIndices(root, n, effNonce)...)
	for _, v := range labelIndices(root, n, effNonce, k) {
		set = append(set, v)
		if v > 0 {
			ps := parentIndices(seed, v)
			set = append(set, v-1)
			set = append(set, ps[:]...)
		}
	}
	slices.Sort(set)
	return slices.Compact(set)
}

// seedIndex picks the single plot label that seeds the space-time VDF, from the
// PUBLIC (root, n, nonce). Holding this label is required to start the VDF (see
// challengeSeedST), which is what binds the "time" half to held space: a prover that
// released the plot cannot produce the seed label without recomputing it through the
// graph.
// Domain-separated from challengeIndices so the seed label and the sampled labels
// are chosen independently.
//
// ADVERSARY-SHAPE: capability=SeedBlockWithoutPlot UNCOVERED: no fixture GRANTS AND CONTROLS FOR a released-space prover the seed block plus a valid inclusion proof.
func seedIndex(root ports.Hash, n int, nonce uint64) int {
	h := sha256.New()
	h.Write([]byte("silt/bond/st/v2/seed"))
	h.Write(root[:])
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], nonce)
	h.Write(nb[:])
	sum := h.Sum(nil)
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(n))
}

// challengeSeedST binds the VDF challenge to this bond, the nonce, AND the seed
// label — so the VDF cannot even be started without reading the plot. A proof for
// one bond or epoch cannot be replayed for another, and a zero-resident prover cannot
// produce the seed label without recomputing it through the graph.
//
// ADVERSARY-SHAPE: capability=CrossEpochProofReplay UNCOVERED: no fixture GRANTS AND CONTROLS FOR a prover that holds a valid answer for one bond/epoch and replays it against another.
func challengeSeedST(root ports.Hash, nonce uint64, seedLabel []byte) []byte {
	h := sha256.New()
	h.Write([]byte("silt/bond/st/v2/vdfseed"))
	h.Write(root[:])
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], nonce)
	h.Write(nb[:])
	h.Write(seedLabel)
	return h.Sum(nil)
}

// vdfDerivedNonce turns the VDF output into the label-sampling nonce, so the labels
// a prover must hold are unknowable until the sequential work is done.
func vdfDerivedNonce(y []byte) uint64 {
	h := sha256.Sum256(append([]byte("silt/bond/st/v1"), y...))
	return binary.BigEndian.Uint64(h[:8])
}

// challengeIndices derives which labels a nonce probes for POSSESSION, from the
// PUBLIC (root, n, nonce) so prover and verifier compute them identically.
func challengeIndices(root ports.Hash, n int, nonce uint64) []int {
	idx := make([]int, Samples)
	buf := make([]byte, len(root)+16)
	copy(buf, root[:])
	for j := 0; j < Samples; j++ {
		binary.BigEndian.PutUint64(buf[len(root):], nonce)
		binary.BigEndian.PutUint64(buf[len(root)+8:], uint64(j))
		h := ports.HashBytes(buf)
		idx[j] = int(binary.BigEndian.Uint64(h[:8]) % uint64(n))
	}
	return idx
}

// labelIndices derives the k nodes for the labeling-consistency check, from the
// PUBLIC (root, n, nonce) — domain-separated from challengeIndices so the labeling
// nodes and the possession samples are chosen independently. Index j depends only on
// (root, nonce, j), so the first k of any longer draw is a prefix: a network can
// raise k without invalidating shorter checks.
func labelIndices(root ports.Hash, n int, nonce uint64, k int) []int {
	k = resolveK(k)
	idx := make([]int, k)
	for j := 0; j < k; j++ {
		h := sha256.New()
		h.Write([]byte("silt/bond/label/v3"))
		h.Write(root[:])
		var b [16]byte
		binary.BigEndian.PutUint64(b[:8], nonce)
		binary.BigEndian.PutUint64(b[8:], uint64(j))
		h.Write(b[:])
		sum := h.Sum(nil)
		idx[j] = int(binary.BigEndian.Uint64(sum[:8]) % uint64(n))
	}
	return idx
}

const (
	// plotDomain names the plot format and namespaces the seed H(plotDomain ‖ pk ‖
	// n). v4 makes each graph node its 32-byte label. A plot of an earlier format
	// re-plots rather than reloading; the disk format version guards this.
	plotDomain = "silt/bond/plot/v4"
	// plotParents is how many DRSample long-range parents each node depends on, on
	// top of its immediate predecessor. DRSample with a chain already yields a
	// depth-robust graph at indegree 2; extra parents strengthen the pebbling bound
	// at a small plotting-time cost.
	plotParents = 3
	// labelInput is the size of a label's hash input: the tag, the seed, the index,
	// the predecessor and the parents.
	labelInput = 1 + LabelSize + 8 + (1+plotParents)*LabelSize
)

// labelOf computes label i from the seed and the labels it depends on, which get
// supplies: its predecessor and its DRSample parents. Node 0 has neither,
// and hashes an all-zero predecessor. labelOf is the one definition of a label:
// sealing calls it with the plot's own earlier labels, and verification calls it
// with opened, Merkle-proven ones, so a match proves the committed label is the
// plot's label for i.
func labelOf(seed []byte, i int, get func(j int) ports.Hash) ports.Hash {
	var buf [labelInput]byte
	buf[0] = tagLabel
	copy(buf[1:1+LabelSize], seed)
	binary.BigEndian.PutUint64(buf[1+LabelSize:], uint64(i))
	const pred = 1 + LabelSize + 8
	if i == 0 {
		return sha256.Sum256(buf[:pred+LabelSize])
	}
	l := get(i - 1)
	copy(buf[pred:], l[:])
	for m, p := range parentIndices(seed, i) {
		l = get(p)
		copy(buf[pred+(1+m)*LabelSize:], l[:])
	}
	return sha256.Sum256(buf[:])
}

// parentIndices derives node i's plotParents long-range parents, all in [0, i),
// using DRSample (Alwen–Blocki–Harsha, "Practical Graphs for Optimal Side-Channel
// Resistant Proofs of Work", CCS'17): each parent's distance from i is drawn
// log-uniformly — pick a bucket g in [1, ⌊log2 i⌋], then a distance uniformly in
// (2^(g-1), 2^g] — so short and long edges are both well represented, which is what
// makes the graph depth-robust. Deterministic from (seed, i), and the seed folds in
// n, so the draw is size-bound: a prefix of an n-plot is not a valid smaller plot.
// Repeats are harmless. i must be at least 1.
func parentIndices(seed []byte, i int) [plotParents]int {
	var buf [1 + LabelSize + 8]byte
	buf[0] = tagParents
	copy(buf[1:], seed)
	binary.BigEndian.PutUint64(buf[1+LabelSize:], uint64(i))
	sum := sha256.Sum256(buf[:])
	var out [plotParents]int
	for j := range out {
		out[j] = drSampleParent(i, binary.BigEndian.Uint64(sum[8*j:]))
	}
	return out
}

// drSampleParent returns an earlier node for node i using the DRSample distance
// distribution. r's residue picks the bucket (a power-of-two distance band) and its
// quotient picks the distance within it. The result is always in [0, i).
func drSampleParent(i int, r uint64) int {
	// bucket g ∈ [1, ⌊log2 i⌋]; distance ∈ (2^(g-1), 2^g], clamped to ≤ i.
	maxg := uint64(max(bits.Len(uint(i))-1, 1))
	g := 1 + int(r%maxg)
	r /= maxg
	lo := 1 << (g - 1)
	hi := min(1<<g, i)
	span := uint64(max(hi-lo, 1))
	dist := min(lo+int(r%span)+1, i)
	return i - dist
}

// blockLevels is the subtree over one block's labels: level 0 is the labels and
// level blockDepth is the block's root.
type blockLevels [blockDepth + 1][]ports.Hash

// blockTree computes every level of one block's subtree, for building proofs.
func blockTree(block []byte) *blockLevels {
	var t blockLevels
	t[0] = make([]ports.Hash, LabelsPerBlock)
	for j := range t[0] {
		copy(t[0][j][:], block[j*LabelSize:])
	}
	for d := 1; d <= blockDepth; d++ {
		prev := t[d-1]
		cur := make([]ports.Hash, len(prev)/2)
		for j := range cur {
			cur[j] = nodeHash(prev[2*j][:], prev[2*j+1][:])
		}
		t[d] = cur
	}
	return &t
}

// blockScratch is blockRoot's working space, reused across blocks so hashing a whole
// plot allocates nothing per block.
type blockScratch [LabelsPerBlock / 2]ports.Hash

// blockRoot is the root of one block's labels: the same value as blockTree's top
// level, computed without keeping the levels below it.
func blockRoot(block []byte, s *blockScratch) ports.Hash {
	for j := range s {
		s[j] = nodeHash(block[2*j*LabelSize:(2*j+1)*LabelSize], block[(2*j+1)*LabelSize:(2*j+2)*LabelSize])
	}
	for w := len(s) / 2; w >= 1; w /= 2 {
		for j := 0; j < w; j++ {
			s[j] = nodeHash(s[2*j][:], s[2*j+1][:])
		}
	}
	return s[0]
}

// upperTree builds the commitment tree from the block roots up. The last node of an
// odd-width level moves up unchanged, which is the rule foldProof follows.
func upperTree(roots []ports.Hash) [][]ports.Hash {
	levels := [][]ports.Hash{roots}
	for cur := roots; len(cur) > 1; {
		next := make([]ports.Hash, (len(cur)+1)/2)
		for j := range next {
			if 2*j+1 < len(cur) {
				next[j] = nodeHash(cur[2*j][:], cur[2*j+1][:])
			} else {
				next[j] = cur[2*j]
			}
		}
		levels = append(levels, next)
		cur = next
	}
	return levels
}

// nodeHash is an interior node of the commitment tree.
func nodeHash(l, r []byte) ports.Hash {
	var buf [1 + 2*LabelSize]byte
	buf[0] = tagNode
	copy(buf[1:], l)
	copy(buf[1+LabelSize:], r)
	return sha256.Sum256(buf[:])
}

// foldProof walks the commitment tree over n leaves from the opened leaves at idx
// (ascending, no repeats) up to the root, and returns the root. At each level a node
// whose sibling is also known pairs with it, a node whose sibling is not known takes
// it from sibling(level, index), and the last node of an odd-width level moves up
// unchanged. Prover and verifier both fold through here, so the order a prover emits
// siblings in is by construction the order a verifier consumes them.
func foldProof(n int, idx []int, leaves []ports.Hash, sibling func(level, x int) (ports.Hash, bool)) (ports.Hash, bool) {
	if len(idx) == 0 || len(idx) != len(leaves) {
		return ports.Hash{}, false
	}
	for j, x := range idx {
		if x < 0 || x >= n || (j > 0 && x <= idx[j-1]) {
			return ports.Hash{}, false
		}
	}
	xs := slices.Clone(idx)
	hs := slices.Clone(leaves)
	for level, width := 0, n; width > 1; level, width = level+1, (width+1)/2 {
		out := 0
		for j := 0; j < len(xs); j++ {
			x, h := xs[j], hs[j]
			var parent ports.Hash
			switch {
			case x%2 == 1:
				l, ok := sibling(level, x-1)
				if !ok {
					return ports.Hash{}, false
				}
				parent = nodeHash(l[:], h[:])
			case x+1 == width:
				parent = h
			case j+1 < len(xs) && xs[j+1] == x+1:
				parent = nodeHash(h[:], hs[j+1][:])
				j++
			default:
				r, ok := sibling(level, x+1)
				if !ok {
					return ports.Hash{}, false
				}
				parent = nodeHash(h[:], r[:])
			}
			xs[out], hs[out] = x/2, parent
			out++
		}
		xs, hs = xs[:out], hs[:out]
	}
	return hs[0], true
}
