package chain

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// logArchive holds blocks encoded, as a chain log does, and returns decoded copies.
type logArchive struct {
	raw [][]byte
	err error
}

func (a *logArchive) Durable() uint64 { return uint64(len(a.raw)) }

func (a *logArchive) Block(h uint64) (Block, error) {
	if a.err != nil {
		return Block{}, a.err
	}
	if h >= uint64(len(a.raw)) {
		return Block{}, fmt.Errorf("no block %d", h)
	}
	b, err := Decode(a.raw[h])
	if err != nil {
		return Block{}, err
	}
	return *b, nil
}

// heavyBlock is a v5 block whose bond registration carries its heavy proof.
func heavyBlock(h uint64) Block {
	answer := []byte(fmt.Sprintf("proof-%d", h))
	d := answerDigestOf(answer)
	return Block{Version: BlockVersionWitnessable, Height: h, ProposerSig: []byte{byte(h)},
		BondRegs: []BondReg{{Validator: make([]byte, 32), Size: 1, Answer: answer, AnswerDigest: &d}}}
}

func lightBlock(h uint64) Block {
	return Block{Version: 1, Height: h, ProposerSig: []byte{byte(h), byte(h >> 8)}}
}

// archived builds a vector of blocks and an archive holding each block as it was
// appended, the way a log holds a block it wrote before the node shed its proofs.
func archived(n int, mk func(uint64) Block) (blockVec, *logArchive) {
	var v blockVec
	a := &logArchive{}
	for i := 0; i < n; i++ {
		b := mk(uint64(i))
		v.Append(b)
		a.raw = append(a.raw, Encode(&b))
	}
	return v, a
}

func hashesOf(v *blockVec) []ports.Hash {
	out := make([]ports.Hash, v.Len())
	for i := range out {
		out[i] = v.At(i).Hash()
	}
	return out
}

func always(*Block) bool { return true }

// Released blocks read back exactly as they were in memory: a block whose heavy
// proofs were shed comes back shed although the archive holds it whole, and one
// that was never shed comes back whole. Copies taken before the release are
// untouched by it.
func TestReleasedBlocksReadBackAsTheyWere(t *testing.T) {
	v, a := archived(300, heavyBlock)
	for i := 0; i < 300; i += 3 {
		v.Set(i, v.At(i).Prune())
	}
	want := hashesOf(&v)
	shed := make([]bool, 300)
	for i := range shed {
		shed[i] = v.At(i).HeavyProofsShed()
	}
	before := v

	n, err := v.release(300, a, always)
	if err != nil {
		t.Fatal(err)
	}
	if n != 256 || !v.Released(32) || !v.Released(287) || v.Released(31) || v.Released(288) {
		t.Fatalf("released %d blocks, hot from %d; want the 8 whole leaves between the first and the tail", n, v.HotFrom())
	}
	for i := 0; i < 300; i++ {
		b := v.At(i)
		if b.Hash() != want[i] || b.HeavyProofsShed() != shed[i] {
			t.Fatalf("block %d reads back with hash %s shed %v; it was %s shed %v", i, b.Hash(), b.HeavyProofsShed(), want[i], shed[i])
		}
		if !shed[i] && b.BondRegs[0].Answer == nil {
			t.Fatalf("block %d, never shed, came back without its proof", i)
		}
	}
	if before.Released(40) {
		t.Fatal("a copy taken before the release was changed by it")
	}
	if got := hashesOf(&before); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatal("a copy taken before the release reads differently")
	}
	for i, h := range want {
		if j, ok := v.IndexOf(h); !ok || j != i {
			t.Fatalf("the index finds block %d at %d, %v", i, j, ok)
		}
	}
}

// Release stops at the first block it may not release: that block's leaf, and
// every leaf after it, stays in memory.
func TestReleaseStopsAtABlockItMayNotRelease(t *testing.T) {
	v, a := archived(300, lightBlock)
	n, err := v.release(300, a, func(b *Block) bool { return b.Height != 100 })
	if err != nil {
		t.Fatal(err)
	}
	if n != 64 || v.HotFrom() != 96 {
		t.Fatalf("released %d blocks, hot from %d; want the two leaves below block 100's", n, v.HotFrom())
	}
	if _, err := v.release(300, a, func(b *Block) bool { return b.Height < 200 }); err != nil {
		t.Fatal(err)
	}
	if v.HotFrom() != 192 {
		t.Fatalf("a later release resumed to %d, want 192", v.HotFrom())
	}
}

// An archive that does not hold the chain's blocks is found at release, while the
// blocks are still in memory, and nothing it disagrees on is released.
func TestReleaseRefusesAnArchiveThatDisagrees(t *testing.T) {
	v, a := archived(300, lightBlock)
	wrong := lightBlock(50)
	wrong.ProposerSig = []byte("not the committed block")
	a.raw[50] = Encode(&wrong)
	if _, err := v.release(300, a, always); err == nil {
		t.Fatal("an archive holding a different block 50 was accepted")
	}
	if v.Released(50) || v.HotFrom() != 0 && v.HotFrom() > 32 {
		t.Fatalf("released up to %d past the disagreement", v.HotFrom())
	}
	// An archive holding only part of the chain lets only that part go.
	v, a = archived(300, lightBlock)
	short := &logArchive{raw: a.raw[:100]}
	if n, err := v.release(300, short, always); err != nil || v.HotFrom() != 96 || n != 64 {
		t.Fatalf("an archive holding 100 blocks let %d go (hot from %d), %v", n, v.HotFrom(), err)
	}
}

// A released block the archive can no longer return, or returns changed, stops
// the node: no answer is better than another block.
func TestAReleasedBlockTheArchiveLosesIsFatal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*logArchive)
	}{
		{"unreadable", func(a *logArchive) { a.err = errors.New("disk gone") }},
		{"changed", func(a *logArchive) {
			b := lightBlock(40)
			b.ProposerSig = []byte("changed")
			a.raw[40] = Encode(&b)
		}},
	} {
		v, a := archived(300, lightBlock)
		if _, err := v.release(300, a, always); err != nil {
			t.Fatal(err)
		}
		tc.break_(a)
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: reading released block 40 did not stop the node", tc.name)
				}
			}()
			v.At(40)
		}()
	}
}

// A released block is final: replacing it is refused.
func TestAReleasedBlockCannotBeReplaced(t *testing.T) {
	v, a := archived(100, lightBlock)
	if _, err := v.release(100, a, always); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a released block was replaced")
		}
	}()
	v.Set(40, lightBlock(40))
}

// The census and the hash index the vector keeps are what a walk of its blocks
// gives, after appends and replacements, without reading a block.
func TestCensusAndIndexFollowTheBlocks(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	var v blockVec
	mk := func(h uint64, tag byte) Block {
		b := Block{Version: uint64(1 + rng.Intn(int(BlockVersionWitnessable))), Height: h, ProposerSig: []byte{tag}}
		for k := rng.Intn(6); k > 0; k-- {
			b.Atts = append(b.Atts, Attestation{PubKey: []byte{byte(k)}})
		}
		return b
	}
	for i := 0; i < 2000; i++ {
		v.Append(mk(uint64(i), 0))
		if i > 0 && rng.Intn(4) == 0 {
			j := rng.Intn(i)
			old := v.At(j)
			repl := *old
			repl.ProposerSig, repl.hashMemoSet = []byte{byte(1 + rng.Intn(250))}, false
			oldHash := old.Hash()
			v.Set(j, repl)
			if got, _ := v.IndexOf(oldHash); got == j && oldHash != repl.Hash() {
				t.Fatalf("the index still finds the replaced block %d by its old hash", j)
			}
		}
	}
	if got, want := v.Census(), CensusOf(v.Slice(0)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("census %+v, a walk gives %+v", got, want)
	}
	for i := 0; i < v.Len(); i++ {
		if j, ok := v.IndexOf(v.At(i).Hash()); !ok || j != i {
			t.Fatalf("the index finds block %d at %d, %v", i, j, ok)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a replacement that changes a block's version was accepted")
		}
	}()
	b := *v.At(5)
	b.Version++
	v.Set(5, b)
}
