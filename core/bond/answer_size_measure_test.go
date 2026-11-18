package bond

import (
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// TestMeasure_AnswerSizeBreakdown is the measurement, not an assert: it decomposes
// the encoded space-time answer into opened labels and multiproof siblings, so the
// proof's size on the wire is read from evidence. Run with -v to read it.
func TestMeasure_AnswerSizeBreakdown(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only")
	}
	for _, size := range []int64{8 << 20, 64 << 20} {
		c := Seal(pk(1), size)
		a, ok := c.AnswerSpaceTime(12345, vdf.Default(), 1000, DefaultLabelSamples) // the field BondVDFDelay
		if !ok {
			t.Fatal("answer failed")
		}
		enc, err := EncodeAnswer(a)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("bond=%dMiB labels=%d: encoded=%d B | opened labels=%d (%d B) | siblings=%d (%d B) | VDF=%d B",
			size>>20, NumLabels(size), len(enc),
			len(a.Labels)/LabelSize, len(a.Labels), len(a.Siblings)/LabelSize, len(a.Siblings),
			len(a.VDFY)+len(a.VDFPi))
	}
}
