package bond

import (
	"testing"

	"github.com/nerolabs/silt/core/vdf"
)

// BenchmarkVerifySpaceTimeFieldConfig measures the absolute cost of VERIFYING one
// bond answer at the field config (BondVDFDelay=1000, BondLabelSamples=64) — the
// per-message CPU an attacker can force through MsgSubmitBondReg with a self-signed
// garbage reg (the signature is theirs, so it passes; the space-time verify is where
// the node burns). This is the number the Phase 1.2 submit gate is sized against,
// and one input to the E5 drain-rate model (a flood's drain rate ≈ frame size /
// (this + decode)). Sizes bracket the field (sybils 1 MiB, anchors 64 MiB).
//
// The garbage-answer case is ALSO measured (VerifyGarbage) because the DoS cost is
// the verify-until-reject path, not the happy path: a forged answer should die at
// the earliest structural check, and the gap between the two numbers is the
// amplification a cheap-first validation order already denies.
func BenchmarkVerifySpaceTimeFieldConfig(b *testing.B) {
	const fieldDelay = 1000
	const fieldK = 64
	p := vdf.Default()
	for _, size := range []int64{1 << 20, 64 << 20} {
		c := Seal(pk(1), size)
		ans, ok := c.AnswerSpaceTime(7, p, fieldDelay, fieldK)
		if !ok {
			b.Fatal("answer failed")
		}
		b.Run("VerifyValid-"+sizeLabel(size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if !VerifySpaceTime(pk(1), c.Root, size, 7, ans, p, fieldDelay, fieldK) {
					b.Fatal("valid answer refused")
				}
			}
		})
		// Garbage: right shape, wrong bytes — the flood case. Corrupt the VDF
		// output so the reject happens mid-pipeline (after seed checks).
		bad := ans
		bad.VDFY = append([]byte(nil), ans.VDFY...)
		bad.VDFY[0] ^= 1
		b.Run("VerifyGarbage-"+sizeLabel(size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if VerifySpaceTime(pk(1), c.Root, size, 7, bad, p, fieldDelay, fieldK) {
					b.Fatal("garbage answer accepted")
				}
			}
		})
	}
}
