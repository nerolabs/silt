package node

import (
	"testing"

	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/ports"
)

// Blind (integrity, S1) INVERTED as a regression.
//
// The candidate break: the auditor trusted the prover's self-reported leaf count on
// the LAST leaf. A liar holding only the first leaf of a 40-leaf shard could report
// a count of 1; the old tail-leniency branch accepted any count from 1 to the full
// one, and the challenge clamped the sample space to that single leaf, so the liar
// answered over the one part it kept and passed — collecting rent while holding
// ~1/40th of the shard. (Every stored shard is in fact full-size — chunk.Split pads
// the tail — so the leniency was unwarranted.)
//
// The fix: the auditor grades EVERY leaf against the same AUTHORITATIVE count it
// derives itself from committed geometry, never the prover's self-report. This test
// reconstructs the exact gradeAnswers predicate over the real scheme and asserts the
// shrink liar FAILS while an honest holder PASSES.
func TestTailShardShrinkIsRejected(t *testing.T) {
	const nLeaves = 40
	data := make([]byte, por.SpotLeafBytes*nLeaves)
	for i := range data {
		data[i] = byte(i*131 + 7)
	}
	shardRoot := por.ShardRoot(data, por.SpotLeafBytes)
	// The auditor's AUTHORITATIVE count for this shard, which it derives itself for
	// every leaf. The prover does not get to choose it.
	want := por.SpotLeaves(len(data), por.SpotLeafBytes)
	if want != nLeaves {
		t.Fatalf("setup: expected %d leaves, got %d", nLeaves, want)
	}
	prover := ports.HashBytes([]byte("tail-shrink-liar"))
	base := porChallengeSeed(12345)
	seed := porProverSeed(base, prover)

	// ---- Shrink liar: keep only leaf 0, report a count of 1 ----
	// It opens a challenge clamped to its self-report, the strongest form of the
	// attack: within its own one-leaf tree the answer is internally consistent.
	const reported = 1
	liarData := data[:por.SpotLeafBytes]
	liarOps, err := por.Open(liarData, por.SpotLeafBytes, seed, porSampleCount)
	if err != nil {
		t.Fatalf("liar open: %v", err)
	}

	// gradeAnswers grades over the AUTHORITATIVE want, not the self-report:
	//   passed := a.valid && blocksOK(a.blocks, want) &&
	//             por.VerifyOpenings(shardRoot, want, leafBytes, seed, count, a.opens)
	liarPassed := true && // a.valid: the Merkle proof over the chunk id still binds
		blocksOK(reported, want) &&
		por.VerifyOpenings(shardRoot, want, por.SpotLeafBytes, seed, porSampleCount, liarOps)
	if liarPassed {
		t.Fatalf("F4 regression: a liar holding 1 of %d leaves was graded PASSED — "+
			"the auditor must demand the committed leaf count, not the self-report", want)
	}
	// The count check alone must reject it: reported 1 != authoritative want.
	if blocksOK(reported, want) {
		t.Fatalf("F4 regression: blocksOK accepted reported=%d against want=%d", reported, want)
	}

	// Even a liar that LIES about its count (reports want) cannot pass: it must then
	// open the full sample space, which names leaves it dropped. Zero-filling the
	// bytes it no longer has is the best it can do.
	padded := make([]byte, len(data))
	copy(padded[:por.SpotLeafBytes], liarData) // only leaf 0's bytes are real
	padOps, err := por.Open(padded, por.SpotLeafBytes, seed, porSampleCount)
	if err != nil {
		t.Fatalf("padded liar open: %v", err)
	}
	if por.VerifyOpenings(shardRoot, want, por.SpotLeafBytes, seed, porSampleCount, padOps) {
		t.Fatal("F4 regression: a liar reporting the true count but missing the bytes verified — the scheme must reject it")
	}

	// CONTRAST: an honest holder (all bytes, reports the true count) PASSES.
	honestOps, err := por.Open(data, por.SpotLeafBytes, seed, porSampleCount)
	if err != nil {
		t.Fatalf("honest open: %v", err)
	}
	if !(blocksOK(want, want) && por.VerifyOpenings(shardRoot, want, por.SpotLeafBytes, seed, porSampleCount, honestOps)) {
		t.Fatal("an honest holder of the full shard must pass the audit")
	}
}
