package bond

import (
	"slices"
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// A plot answers only while it holds its labels. A prover that keeps the commitment
// and the block-level tree but frees the plot, intending to recompute on demand,
// holds no label to open and no seed label to start the VDF from.
func TestReleasedPlotCannotAnswer(t *testing.T) {
	pkA := pk(9)
	c := Seal(pkA, 1<<20)
	if _, ok := c.AnswerSpaceTime(5, vdf.Default(), stDelay, testK); !ok {
		t.Fatal("setup: the held plot must answer")
	}
	c.ReleaseBlocks()
	if _, ok := c.AnswerSpaceTime(5, vdf.Default(), stDelay, testK); ok {
		t.Fatal("a released plot still answered a space-time challenge")
	}
	if _, ok := c.Answer(5, testK); ok {
		t.Fatal("a released plot still answered a space-only challenge")
	}
}

// The VDF is seeded from a plot label read BEFORE the VDF (seedIndex), and that label
// is opened under the same multiproof as the rest of the answer. An answer whose seed
// label is not the committed one fails, so a VDF run from public data alone, or from
// another plot's label, buys nothing.
func TestVDFBoundToPlotRead(t *testing.T) {
	pkA := pk(4)
	c := Seal(pkA, 1<<20)
	p := vdf.Default()
	const nonce = 123

	honest, ok := c.AnswerSpaceTime(nonce, p, stDelay, testK)
	if !ok || !VerifySpaceTime(pkA, c.Root, c.Size, nonce, honest, p, stDelay, testK) {
		t.Fatal("setup: honest space-time answer must verify")
	}
	si := seedIndex(c.Root, c.n, nonce)
	idx := openedIndices(c.seed, c.Root, c.n, vdfDerivedNonce(honest.VDFY), resolveK(testK), si)
	at := slices.Index(idx, si)
	if at < 0 {
		t.Fatal("the space-time opens do not include the seed label")
	}
	want, _ := c.label(si)
	if string(honest.Labels[at*LabelSize:(at+1)*LabelSize]) != string(want[:]) {
		t.Fatal("the answer's seed label is not the plot's label at seedIndex")
	}

	// (a) Tamper the seed label: it no longer folds to the committed root.
	tampered := honest
	tampered.Labels = append([]byte(nil), honest.Labels...)
	tampered.Labels[at*LabelSize] ^= 0xff
	if VerifySpaceTime(pkA, c.Root, c.Size, nonce, tampered, p, stDelay, testK) {
		t.Fatal("a tampered VDF seed label passed")
	}

	// (b) Leave the seed label out, as a prover that never read the plot would.
	missing := honest
	missing.Labels = append(append([]byte(nil), honest.Labels[:at*LabelSize]...), honest.Labels[(at+1)*LabelSize:]...)
	if VerifySpaceTime(pkA, c.Root, c.Size, nonce, missing, p, stDelay, testK) {
		t.Fatal("an answer with no seed label passed")
	}

	// (c) Seed the VDF from ANOTHER plot's label at the same position and rerun it:
	// the VDF verifies against that label, but the label does not fold to this root.
	other := Seal(pk(5), 1<<20)
	foreign, _ := other.label(si)
	proof, err := vdf.Eval(p, challengeSeedST(c.Root, nonce, foreign[:]), stDelay)
	if err != nil {
		t.Fatal(err)
	}
	crossed := Answer{PK: pkA, VDFY: proof.Y, VDFPi: proof.Pi, VDFT: proof.T}
	cidx := openedIndices(c.seed, c.Root, c.n, vdfDerivedNonce(proof.Y), resolveK(testK), si)
	labels, sib, ok := c.open(cidx)
	if !ok {
		t.Fatal("setup: open the honest labels for the crossed answer")
	}
	cat := slices.Index(cidx, si)
	copy(labels[cat*LabelSize:], foreign[:])
	crossed.Labels, crossed.Siblings = labels, sib
	if VerifySpaceTime(pkA, c.Root, c.Size, nonce, crossed, p, stDelay, testK) {
		t.Fatal("a VDF seeded from another plot's label passed")
	}
}

// Distinct identities produce distinct plots, so cost lives in the proof and the
// root-owner dedup is only a same-root tiebreak. This records the coupling so a change
// that re-collapses plots is caught here too.
func TestDistinctIdentitiesDistinctPlots(t *testing.T) {
	a := Seal(pk(1), 1<<20)
	b := Seal(pk(2), 1<<20)
	if a.Root == b.Root {
		t.Fatal("two identities share a plot root — cost would not scale with identities")
	}
}
