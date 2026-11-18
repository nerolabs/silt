package bond

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The prefix-plot Sybil: one physical plot backing several standings, each claimed
// at a smaller size under the root of a prefix of its blocks. Per-root dedup does not
// see it, because every prefix has its own root. The size folded into the seed and
// the labeling check deny it.

// A prefix of an N-block plot is not a valid m-block plot, at either layer.
func TestPrefixPlotIsNotAValidSmallerPlot(t *testing.T) {
	pkA := pk(11)
	const bigN = 64
	big := sealN(pkA, bigN) // the one physical plot the attacker holds

	// (a) Data layer: the prefix bytes of the N-plot are NOT the bytes an honest
	// m-plot has, so the prefix root differs from a real m-plot's root.
	const m = 32
	honestM := sealN(pkA, m)
	prefix := prefixOf(pkA, big, m)
	if prefix.Root == honestM.Root {
		t.Fatal("an N-plot prefix is byte-identical to a real m-plot — size not folded into the seed")
	}

	// (b) Verification layer, the decisive one. The attacker commits the N-plot's
	// first m blocks under their own root and answers a size-m challenge. Possession
	// passes, because it holds the bytes, but the labeling check recomputes labels
	// from H(pk, m) and the N-plot's labels are labeled from H(pk, N).
	const nonce = 7
	ans, ok := prefix.Answer(nonce, testK)
	if !ok {
		t.Fatal("setup: attacker holds the prefix bytes, so it can open them")
	}
	if !possessionOnly(pkA, prefix.Root, prefix.Size, nonce, ans, testK) {
		t.Fatal("setup: the prefix genuinely satisfies Merkle possession")
	}
	if Verify(pkA, prefix.Root, prefix.Size, nonce, ans, testK) {
		t.Fatal("a prefix of a larger plot passed a smaller-size challenge — one disk backs N standings")
	}
}

// A plot sealed for pk_A fails when claimed by pk_B: the verifier recomputes labels
// from the CLAIMED key, so bytes sealed for another key do not match.
func TestPlotBoundToClaimedIdentity(t *testing.T) {
	pkA, pkB := pk(21), pk(22)
	c := Seal(pkA, 1<<20)
	const nonce = 42
	ans, ok := c.Answer(nonce, testK)
	if !ok {
		t.Fatal("setup: holder answers its own plot")
	}
	if !Verify(pkA, c.Root, c.Size, nonce, ans, testK) {
		t.Fatal("setup: the plot verifies under its own key")
	}
	// The SAME bytes and root, claimed by a different identity. (bondaudit
	// additionally binds sha256(pk)==peerID, but the labeling check alone denies it.)
	if Verify(pkB, c.Root, c.Size, nonce, ans, testK) {
		t.Fatal("a plot sealed for one identity verified under another's key")
	}
}

// Committing arbitrary bytes of the right size and answering possession is not
// enough: the labeling check recomputes each challenged node's label and requires a
// match. This is what makes the bond a proof of SPACE rather than of storage.
func TestArbitraryBytesFailLabeling(t *testing.T) {
	pkA := pk(31)
	const n = 32
	blocks := make([][]byte, n)
	for i := range blocks {
		b := make([]byte, BlockSize)
		for off := 0; off < BlockSize; off += LabelSize {
			h := ports.HashBytes([]byte{byte(i), byte(off >> 8), byte(off), 0xab})
			copy(b[off:], h[:])
		}
		blocks[i] = b
	}
	size := int64(n) * BlockSize
	junk := commitBlocks(pkA, size, plotSeedN(pkA, NumLabels(size)), blocks)
	const nonce = 99
	ans, ok := junk.Answer(nonce, testK)
	if !ok {
		t.Fatal("setup: the junk plot holds its bytes, so it can open them")
	}
	if !possessionOnly(pkA, junk.Root, junk.Size, nonce, ans, testK) {
		t.Fatal("setup: junk bytes satisfy Merkle possession")
	}
	if Verify(pkA, junk.Root, junk.Size, nonce, ans, testK) {
		t.Fatal("arbitrary committed bytes passed — this is proof of storage, not proof of space")
	}
}

// Across a family of prefix roots of ONE physical plot, none forges a standing at its
// claimed smaller size, so N identities require N plots.
func TestPrefixFamilyYieldsNoExtraStandings(t *testing.T) {
	pkA := pk(41)
	big := sealN(pkA, 64)
	const nonce = 3
	forged := 0
	for _, m := range []int{4, 8, 17, 32, 50} {
		prefix := prefixOf(pkA, big, m)
		ans, ok := prefix.Answer(nonce, testK)
		if ok && Verify(pkA, prefix.Root, prefix.Size, nonce, ans, testK) {
			forged++
		}
	}
	if forged != 0 {
		t.Fatalf("%d prefix standings forged from one plot", forged)
	}
}

// Leaving k unset (0) does not disable the labeling check: it resolves to
// DefaultLabelSamples on both sides, and the prefix Sybil is still denied.
func TestUnsetKStillDeniesPrefixSybil(t *testing.T) {
	pkA := pk(51)
	prefix := prefixOf(pkA, sealN(pkA, 64), 32)
	const nonce = 5
	ans, ok := prefix.Answer(nonce, 0)
	if !ok {
		t.Fatal("setup: prefix answer builds")
	}
	if Verify(pkA, prefix.Root, prefix.Size, nonce, ans, 0) {
		t.Fatal("prefix Sybil passed with k unset — labeling check silently disabled")
	}
}

// sealN plots for pk with exactly n blocks, so tests can reason in block counts.
func sealN(pk []byte, n int) *Commitment { return Seal(pk, int64(n)*BlockSize) }

// prefixOf commits the first m blocks of big as an m-block plot claimed under pk, the
// attacker's forged smaller bond.
func prefixOf(pk []byte, big *Commitment, m int) *Commitment {
	size := int64(m) * BlockSize
	return commitBlocks(pk, size, plotSeedN(pk, NumLabels(size)), big.blocks[:m])
}

// possessionOnly runs ONLY the Merkle-possession check (the opened labels fold to
// root), with no labeling check, so a test can show an attack satisfies possession
// and is caught solely by the labeling check.
func possessionOnly(pk []byte, root ports.Hash, size int64, nonce uint64, a Answer, k int) bool {
	n := NumLabels(size)
	_, ok := checkOpened(root, n, openedIndices(plotSeedN(pk, n), root, n, nonce, resolveK(k)), a)
	return ok
}
