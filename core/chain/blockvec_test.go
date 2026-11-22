package chain

import (
	"math/rand"
	"testing"
)

func bvBlock(h uint64, tag byte) Block {
	return Block{Height: h, Entries: nil, ProposerSig: []byte{tag}}
}

// A blockVec behaves as a slice under appends and Sets across every trie depth the
// test reaches, and every earlier copy is unchanged by what later copies did.
func TestBlockVecMatchesASliceAndKeepsEveryCopy(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	var v blockVec
	var ref []Block
	type snapshot struct {
		v   blockVec
		ref []Block
	}
	var copies []snapshot
	const n = 40_000 // past 32·32·32, so the root grows to three levels
	for i := 0; i < n; i++ {
		v.Append(bvBlock(uint64(i), 0))
		ref = append(ref, bvBlock(uint64(i), 0))
		if i > 0 && rng.Intn(5) == 0 {
			j := rng.Intn(len(ref))
			tag := byte(rng.Intn(255) + 1)
			v.Set(j, bvBlock(uint64(j), tag))
			ref[j] = bvBlock(uint64(j), tag)
		}
		if i%3331 == 0 {
			copies = append(copies, snapshot{v, append([]Block(nil), ref...)})
		}
	}
	check := func(name string, v blockVec, ref []Block) {
		t.Helper()
		if v.Len() != len(ref) {
			t.Fatalf("%s: length %d, want %d", name, v.Len(), len(ref))
		}
		for i := range ref {
			got := v.At(i)
			if got.Height != ref[i].Height || got.ProposerSig[0] != ref[i].ProposerSig[0] {
				t.Fatalf("%s: block %d differs", name, i)
			}
		}
		if len(ref) > 0 && v.Last().Height != ref[len(ref)-1].Height {
			t.Fatalf("%s: Last is not the final block", name)
		}
		s := v.Slice(len(ref) / 3)
		for i := range s {
			if s[i].Height != ref[len(ref)/3+i].Height {
				t.Fatalf("%s: Slice differs at %d", name, i)
			}
		}
	}
	check("final", v, ref)
	for i, c := range copies {
		check("copy", c.v, c.ref)
		_ = i
	}

	// A copy that appends and Sets leaves the original exactly as it was.
	orig := v
	before := orig.Slice(0)
	fork := v
	fork.Append(bvBlock(n, 9))
	fork.Set(0, bvBlock(0, 7))
	fork.Set(n-1, bvBlock(n-1, 7))
	after := orig.Slice(0)
	for i := range before {
		if after[i].ProposerSig[0] != before[i].ProposerSig[0] {
			t.Fatalf("a copy's writes reached the original at block %d", i)
		}
	}
	if orig.Len() != n || fork.Len() != n+1 {
		t.Fatalf("lengths: original %d (want %d), copy %d (want %d)", orig.Len(), n, fork.Len(), n+1)
	}
}

// The trie's shape depends only on the length: the same blocks reached through
// different histories of Sets build equal vectors.
func TestBlockVecShapeIsHistoryIndependent(t *testing.T) {
	var a, b blockVec
	for i := 0; i < 5000; i++ {
		a.Append(bvBlock(uint64(i), 1))
		b.Append(bvBlock(uint64(i), 2))
	}
	for i := 0; i < 5000; i++ {
		b.Set(i, bvBlock(uint64(i), 1))
	}
	if a.Len() != b.Len() || a.shift != b.shift {
		t.Fatalf("equal contents, different shape: len %d/%d shift %d/%d", a.Len(), b.Len(), a.shift, b.shift)
	}
	for i := 0; i < a.Len(); i++ {
		if a.At(i).ProposerSig[0] != b.At(i).ProposerSig[0] {
			t.Fatalf("block %d differs", i)
		}
	}
}
