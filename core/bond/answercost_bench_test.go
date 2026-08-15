package bond

import (
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// BenchmarkAnswerSpaceTimeFieldConfig measures the absolute cost of answering ONE
// bond challenge at the FIELD config — BondVDFDelay=1000 squarings, BondLabelSamples=64
// (core/node/node.go DefaultConfig) — on this machine's single core. This is the
// per-VDF-eval cost the #424/throughput analysis needs (PE 2026-08-15 §d): under
// honest 12-validator operation each node answers ~one challenge per peer per
// audit window (~11 evals / 30s window), so per-eval-cost × evals-per-window vs the
// 30s window is the actual single-goroutine saturation condition. Sizes bracket the
// field (sybils 1 MiB, anchors 64 MiB).
func BenchmarkAnswerSpaceTimeFieldConfig(b *testing.B) {
	const fieldDelay = 1000 // BondVDFDelay default
	const fieldK = 64       // BondLabelSamples default
	p := vdf.Default()
	for _, size := range []int64{1 << 20, 64 << 20} {
		c := Seal(pk(1), size)
		b.Run(sizeLabel(size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ans, ok := c.AnswerSpaceTime(uint64(i)+1, p, fieldDelay, fieldK)
				if !ok || len(ans.VDFY) == 0 {
					b.Fatal("answer failed")
				}
			}
		})
	}
}
