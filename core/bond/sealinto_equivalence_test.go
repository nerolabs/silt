package bond

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// sliceBlocks is a BlockStore over a plain slice — the store a test uses to
// exercise the streaming path without a filesystem.
type sliceBlocks struct{ b [][]byte }

func newSliceBlocks(n int) *sliceBlocks { return &sliceBlocks{b: make([][]byte, n)} }

func (s *sliceBlocks) ReadBlock(i int, into []byte) error {
	if i < 0 || i >= len(s.b) || s.b[i] == nil {
		return fmt.Errorf("no block %d", i)
	}
	copy(into, s.b[i])
	return nil
}

func (s *sliceBlocks) WriteBlock(i int, b []byte) error {
	if i < 0 || i >= len(s.b) {
		return fmt.Errorf("block %d out of range", i)
	}
	s.b[i] = append([]byte(nil), b...)
	return nil
}

// TestSealIntoProducesTheSamePlot pins the property the whole streaming path rests
// on: where a plot's bytes are kept while they are produced changes nothing about
// what is produced. A label is a pure function of the public seed and the plot's
// own earlier bytes, so streaming to a store and holding everything resident must
// agree block for block, on the committed root, and on answers a verifier accepts.
//
// If they ever diverge, an operator who seals to disk commits to a different bond
// than one who seals in memory, and the same identity would fail its own audit.
func TestSealIntoProducesTheSamePlot(t *testing.T) {
	for _, size := range []int64{1 << 20, 4 << 20} {
		t.Run(sizeLabel(size), func(t *testing.T) {
			key := pk(9)
			resident := Seal(key, size)

			store := newSliceBlocks(NumBlocks(size))
			streamed, err := SealInto(key, size, store)
			if err != nil {
				t.Fatalf("SealInto: %v", err)
			}

			if streamed.Root != resident.Root {
				t.Fatalf("THE STREAMED PLOT IS A DIFFERENT BOND — root %x streamed vs %x resident. An operator "+
					"who seals to disk would commit to a bond it cannot prove and fail its own audit.",
					streamed.Root[:8], resident.Root[:8])
			}
			for i := 0; i < NumBlocks(size); i++ {
				if !bytes.Equal(streamed.block(i), resident.block(i)) {
					t.Fatalf("plot block %d differs between the streamed and resident seals", i)
				}
			}

			// The streamed commitment must answer a live challenge, which is the
			// only thing a plot is for.
			const nonce = uint64(4242)
			a, ok := streamed.AnswerSpaceTime(nonce, vdf.Default(), stDelay, testK)
			if !ok {
				t.Fatal("the streamed plot could not answer a challenge — a plot that cannot answer proves nothing")
			}
			if !VerifySpaceTime(key, streamed.Root, size, nonce, a, vdf.Default(), stDelay, testK) {
				t.Fatal("an answer from the streamed plot does not verify against its own committed root")
			}

			// Vacuity guard: the same challenge from a RELEASED plot must fail, so
			// the check above is testing possession rather than passing on structure.
			streamed.ReleaseBlocks()
			if _, ok := streamed.AnswerSpaceTime(nonce, vdf.Default(), stDelay, testK); ok {
				t.Fatal("GATE VACUOUS: a released plot still answered — this test is not observing possession")
			}
		})
	}
}
