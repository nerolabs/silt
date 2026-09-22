package repairproof

import (
	"testing"

	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/ports"
)

func nodeID(b byte) ports.NodeID { return ports.HashBytes([]byte{b, 'n'}) }

// spotFixture builds a rebuilt shard and the commitment the publisher wrote for
// it — the state a judge holds (the root and the geometry) beside the state a
// repairer holds (the bytes).
func spotFixture(t *testing.T) (shardRoot ports.Hash, shard []byte, leaves int) {
	t.Helper()
	shard = make([]byte, 64<<10) // a 64 KiB shard
	for i := range shard {
		shard[i] = byte((i*7 + 5) % 251)
	}
	return por.ShardRoot(shard, por.SpotLeafBytes), shard, por.SpotLeaves(len(shard), por.SpotLeafBytes)
}

// openUnder builds the answer a prover with identity `who` would submit for the
// challenge round `base` over `data`.
func openUnder(t *testing.T, who ports.NodeID, base [32]byte, data []byte, count int) []por.Opening {
	t.Helper()
	ops, err := por.Open(data, por.SpotLeafBytes, RepairChallengeSeed(base, who), count)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return ops
}

// TestVerifyRetrievability_HonestHolderPasses: the repairer that holds the rebuilt
// shard answers its own identity-bound challenge and verifies.
func TestVerifyRetrievability_HonestHolderPasses(t *testing.T) {
	shardRoot, shard, leaves := spotFixture(t)
	repairer := nodeID(1)
	base := [32]byte{0xa1}
	const count = 4

	ops := openUnder(t, repairer, base, shard, count)
	if !VerifyRetrievability(shardRoot, leaves, por.SpotLeafBytes, repairer, base, count, ops) {
		t.Fatal("an honest holder must pass its own identity-bound challenge")
	}
}

// TestVerifyRetrievability_RelayedProofFails: the double-count defense — an attacker
// cannot claim a bounty by relaying a proof another identity built for an
// already-present replica; that proof was aggregated under the holder's seed and
// fails under the attacker's.
func TestVerifyRetrievability_RelayedProofFails(t *testing.T) {
	shardRoot, shard, leaves := spotFixture(t)
	holder, attacker := nodeID(1), nodeID(2)
	base := [32]byte{0xb2}
	const count = 4

	// The honest holder's answer, opened under the holder's seed.
	holderProof := openUnder(t, holder, base, shard, count)

	// Attacker relays it, claiming the bounty under its own identity → must fail.
	if VerifyRetrievability(shardRoot, leaves, por.SpotLeafBytes, attacker, base, count, holderProof) {
		t.Fatal("a relayed proof must fail the attacker's identity-bound challenge (double-count defense)")
	}
	// Sanity: the same relayed proof DOES verify for the holder it was built for.
	if !VerifyRetrievability(shardRoot, leaves, por.SpotLeafBytes, holder, base, count, holderProof) {
		t.Fatal("the holder's own proof should verify for the holder")
	}
}

// TestVerifyRetrievability_TamperedDataFails: a claimant that lost or altered the
// bytes cannot answer, even under its own seed.
func TestVerifyRetrievability_TamperedDataFails(t *testing.T) {
	shardRoot, shard, leaves := spotFixture(t)
	repairer := nodeID(1)
	base := [32]byte{0xc3}
	// A spot check is probabilistic — a challenge only catches tampering in a
	// SAMPLED leaf. Sample every leaf (count = leaves) so a single-byte flip in
	// leaf 0 is deterministically challenged and caught (a smaller sample would
	// catch it only with probability ≈ count/leaves, which is the honest security
	// model, not a good unit-test assertion).
	count := leaves

	tampered := append([]byte(nil), shard...)
	tampered[0] ^= 0xff // one flipped byte, in leaf 0
	ops := openUnder(t, repairer, base, tampered, count)
	if VerifyRetrievability(shardRoot, leaves, por.SpotLeafBytes, repairer, base, count, ops) {
		t.Fatal("a prover that altered a sampled shard leaf must not verify")
	}
}

// TestVerifyRetrievability_Guards: a missing commitment or a zero geometry is a
// clean false, not a panic.
func TestVerifyRetrievability_Guards(t *testing.T) {
	shardRoot, shard, leaves := spotFixture(t)
	ops := openUnder(t, nodeID(1), [32]byte{}, shard, 2)
	if VerifyRetrievability(shardRoot, 0, por.SpotLeafBytes, nodeID(1), [32]byte{}, 2, ops) {
		t.Fatal("zero leaves must not verify")
	}
	if VerifyRetrievability(shardRoot, leaves, 0, nodeID(1), [32]byte{}, 2, ops) {
		t.Fatal("a zero leaf width must not verify")
	}
	if VerifyRetrievability(ports.Hash{}, leaves, por.SpotLeafBytes, nodeID(1), [32]byte{}, 2, ops) {
		t.Fatal("an empty commitment must not verify — a judge with no root of its own denies")
	}
}

// TestDecide_TruthTable exercises the release/slash gate across the correctness ×
// retrievability × quorum space.
func TestDecide_TruthTable(t *testing.T) {
	cases := []struct {
		name          string
		correctnessOK bool
		votes         []bool
		tau           int
		want          Decision
	}{
		{"false correctness slashes regardless", false, []bool{true, true, true}, 2, Decision{Release: false, Slash: true}},
		{"false correctness, no retrievability", false, nil, 1, Decision{Release: false, Slash: true}},
		{"correct + quorum met releases", true, []bool{true, true, true}, 2, Decision{Release: true, Slash: false}},
		{"correct + quorum exactly met", true, []bool{true, false, true}, 2, Decision{Release: true, Slash: false}},
		{"correct + quorum short denies, no slash", true, []bool{true, false, false}, 2, Decision{Release: false, Slash: false}},
		{"correct + no votes denies", true, nil, 1, Decision{Release: false, Slash: false}},
		{"correct + tau=0 denies (no valid quorum)", true, []bool{true, true, true}, 0, Decision{Release: false, Slash: false}},
		{"correct + negative tau denies", true, []bool{true}, -1, Decision{Release: false, Slash: false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Decide(c.correctnessOK, c.votes, c.tau); got != c.want {
				t.Fatalf("Decide(%v, %v, %d) = %+v, want %+v", c.correctnessOK, c.votes, c.tau, got, c.want)
			}
		})
	}
}
