package por

import (
	"testing"
)

// M2 of the PoR key-distribution certification (§8): the shipped §3.2 scheme's
// prover/verifier/tagger cost at the deployment regime — s = 128 sectors per
// block, n = 67 blocks (a 256 KiB frame + 16 B GCM tag), sample = 67 (p = 1.0).
//
// This measures the BASELINE only. The §3.3 pairing spike the certification
// wants compared against does not exist and is not built here.

const benchShardBytes = (256 << 10) + 16

func benchSetup(b *testing.B) (*Key, []byte, []byte, [][]byte, Challenge) {
	b.Helper()
	k, err := DeriveKey([]byte("m2-bench-seed"), DefaultParams)
	if err != nil {
		b.Fatalf("derive: %v", err)
	}
	unitID := make([]byte, 32)
	for i := range unitID {
		unitID[i] = byte(i * 7)
	}
	data := make([]byte, benchShardBytes)
	for i := range data {
		data[i] = byte(i*167 + 29)
	}
	tags := k.Tags(unitID, data)
	if len(tags) != 67 {
		b.Fatalf("regime: expected 67 blocks, got %d", len(tags))
	}
	c := Challenge{Blocks: len(tags), Count: len(tags)}
	copy(c.Seed[:], []byte("m2-bench-challenge-seed---------"))
	return k, unitID, data, tags, c
}

func BenchmarkPorTags(b *testing.B) {
	k, unitID, data, _, _ := benchSetup(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = k.Tags(unitID, data)
	}
}

func BenchmarkPorProve(b *testing.B) {
	_, _, data, tags, c := benchSetup(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Prove(DefaultParams, data, tags, c); err != nil {
			b.Fatalf("prove: %v", err)
		}
	}
}

func BenchmarkPorVerify(b *testing.B) {
	k, unitID, data, tags, c := benchSetup(b)
	p, err := Prove(DefaultParams, data, tags, c)
	if err != nil {
		b.Fatalf("prove: %v", err)
	}
	if !k.Verify(unitID, c, p) {
		b.Fatal("setup: honest proof did not verify")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !k.Verify(unitID, c, p) {
			b.Fatal("verify failed")
		}
	}
}
