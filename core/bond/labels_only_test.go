package bond

import (
	"crypto/sha256"
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// A plot whose blocks are expansions of 32-byte labels is compressible to the labels:
// a prover that keeps one label per block holds 1/128 of the claimed plot and can
// regenerate every block on demand. Here every 32 bytes of the plot is a label in its
// own right, so a prover holding one 32-byte value per block and expanding it
// rebuilds nothing of the committed plot.
func TestLabelsOnlyProverIsDenied(t *testing.T) {
	pkA := pk(11)
	const size = 1 << 20
	honest := Seal(pkA, size)
	nb := NumBlocks(size)

	expand := func(label []byte) []byte {
		block := make([]byte, BlockSize)
		cur := label
		for off := 0; off < BlockSize; off += len(cur) {
			copy(block[off:], cur)
			next := sha256.Sum256(cur)
			cur = next[:]
		}
		return block
	}
	blocks := make([][]byte, nb)
	for i := range blocks {
		blocks[i] = expand(honest.blocks[i][:LabelSize])
	}
	rebuilt, err := Reconstruct(pkA, size, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Root == honest.Root {
		t.Fatalf("a prover holding %d B, 1/%d of the %d B plot, rebuilt the committed root",
			nb*LabelSize, LabelsPerBlock, size)
	}

	p := vdf.Default()
	const nonce = 77
	if ans, ok := rebuilt.AnswerSpaceTime(nonce, p, stDelay, testK); ok &&
		VerifySpaceTime(pkA, honest.Root, size, nonce, ans, p, stDelay, testK) {
		t.Fatal("an answer built from expanded labels verified against the committed root")
	}
}

// The general form of that attack: keep every 128th label, the same 1/128 of the plot,
// and recompute whatever a challenge opens. Each missing label needs its predecessor
// and its parents, recursively, until the walk reaches stored labels. The chain edge
// makes that walk at least as long as the gap to the previous stored label, and the
// DRSample parents fan it out across the whole graph, so answering one challenge
// recomputes most of the plot. Holding less than the plot buys no shortcut over
// re-sealing it.
func TestSparseLabelProverRecomputesMostOfThePlot(t *testing.T) {
	const size = 1 << 20
	n := NumLabels(size)
	seed := plotSeedN(pk(12), n)
	stored := func(j int) bool { return j%LabelsPerBlock == 0 }

	honest := Seal(pk(12), size)
	opens := openedIndices(seed, honest.Root, n, 99, DefaultLabelSamples)

	// recomputed[j] marks a label the prover has to derive to answer. The walk is
	// iterative because the chain edge alone can make it n deep.
	recomputed := make([]bool, n)
	var stack []int
	need := func(j int) {
		if !stored(j) && !recomputed[j] {
			recomputed[j] = true
			stack = append(stack, j)
		}
	}
	for _, v := range opens {
		need(v)
	}
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if j == 0 {
			continue
		}
		need(j - 1)
		for _, p := range parentIndices(seed, j) {
			need(p)
		}
	}
	count := 0
	for _, r := range recomputed {
		if r {
			count++
		}
	}
	t.Logf("holding 1/%d of a %d-label plot, one challenge recomputes %d labels (%.0f%% of the plot)",
		LabelsPerBlock, n, count, 100*float64(count)/float64(n))
	if count < n/2 {
		t.Fatalf("a prover holding 1/%d of the plot answers one challenge by recomputing only %d of %d labels — "+
			"the plot is compressible", LabelsPerBlock, count, n)
	}
}
