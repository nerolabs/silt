package bond

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/core/manifest"
)

// TestMeasure_AnswerSizeBreakdown299 is the #299 measurement, not an assert:
// it decomposes the encoded answer into its byte terms so the compression
// tiers are chosen on evidence (ROADMAP Phase 3). Run with -v to read it.
//
// Terms:
//   - possession blocks (challengeIndices samples)
//   - label-open blocks, raw vs deduped-by-leaf-index (the #299 "shared
//     DRSample parents sent duplicated" question)
//   - Merkle proof bytes, raw vs the distinct-hash union (the multiproof
//     compression floor: shared path nodes collapse)
func TestMeasure_AnswerSizeBreakdown299(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only")
	}
	for _, size := range []int64{8 << 20, 64 << 20} {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		c := Seal(pub, size)
		a, ok := c.Answer(12345, DefaultLabelSamples)
		if !ok {
			t.Fatal("answer failed")
		}
		enc, err := EncodeAnswer(a)
		if err != nil {
			t.Fatal(err)
		}

		n := int(size) / BlockSize
		possBytes := 0
		for _, b := range a.Blocks {
			possBytes += len(b)
		}

		seen := map[int]bool{}
		rawLabelBytes, dupLabelBytes := 0, 0
		addLeaf := func(idx int, b []byte) {
			rawLabelBytes += len(b)
			if seen[idx] {
				dupLabelBytes += len(b)
			} else {
				seen[idx] = true
			}
		}
		for j, v := range a.LabelIndices {
			lo := a.LabelBundles[j]
			addLeaf(v, lo.Node)
			if v > 0 {
				addLeaf(v-1, lo.Pred)
				for pi, p := range parentIndices(c.seed, v, n) {
					addLeaf(p, lo.Parents[pi])
				}
			}
		}
		possOverlap := 0
		for _, i := range a.Indices {
			if seen[i] {
				possOverlap += BlockSize
			}
		}

		rawProofBytes := 0
		unionHashes := map[[32]byte]bool{}
		addProof := func(pr manifest.Proof) {
			for _, h := range pr.Path {
				rawProofBytes += len(h)
				var k [32]byte
				copy(k[:], h[:])
				unionHashes[k] = true
			}
		}
		for _, p := range a.Proofs {
			addProof(p)
		}
		for _, lo := range a.LabelBundles {
			addProof(lo.NodeProof)
			if lo.Pred != nil {
				addProof(lo.PredProof)
			}
			for _, pp := range lo.ParentProofs {
				addProof(pp)
			}
		}
		unionProofBytes := len(unionHashes) * 32

		t.Logf("bond=%dMiB n=%d: encoded=%dKiB | possession=%dKiB (%d blocks) | label raw=%dKiB dup-by-index=%dKiB (%.1f%%) uniq-leaves=%d | poss∩label=%dKiB | proofs raw=%dKiB union-floor=%dKiB (%.1f%% saved)",
			size>>20, n, len(enc)>>10, possBytes>>10, len(a.Blocks),
			rawLabelBytes>>10, dupLabelBytes>>10, 100*float64(dupLabelBytes)/float64(rawLabelBytes),
			len(seen), possOverlap>>10,
			rawProofBytes>>10, unionProofBytes>>10,
			100*(1-float64(unionProofBytes)/float64(rawProofBytes)))
	}
}
