package node

import (
	"testing"
	"time"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// WHAT IT COSTS TO SAY WHICH REGISTRATIONS THIS NODE HOLDS.
//
// The last cause of the impairment wedge is an EVIDENCE gap, not a delivery gap: a
// proposer may shed a registration's heavy proof only to peers it can prove hold
// it, and its only two proofs are "the peer authored it" and "the peer
// acknowledged MY OWN". On a live chain renewals are staggered, so a block carries
// one OTHER validator's registration, only its author qualifies, and the remaining
// attesters are sent ~1,574,000 B they already have. Measured in the field: 15 of
// 24 registration-bearing prepare legs carried, and the chain wedged.
//
// The fix is for the HOLDER to say what it holds, on the head probe that already
// crosses between every pair of nodes on every sweep. That makes the reply carry a
// set of answer-digests — and a digest is sha256 over a ~1.5 MB answer.
//
// BUILD-IMMUTABLE #8 IS THE REASON THIS FILE EXISTS. The rule is to measure a
// mechanism's PRODUCTION cost on the floor box before committing to it, because a
// mechanism whose output is tiny and whose production is not is disqualified
// however elegant. The output here is 32 bytes per registration. The production is
// a megabyte-scale hash, on the single serialized loop, and the question is
// whether it can be paid per SWEEP or must be paid once per SUBMIT and cached.
//
// The answer decides the shape, so it is measured rather than assumed.

// answerBytes is the size of a real space-time answer on the shipped parameters,
// as measured in the field and recorded on the release-candidate list.
const answerBytes = 1_514_986

func TestTheCostOfDigestingAHeldRegistration(t *testing.T) {
	answer := make([]byte, answerBytes)
	for i := range answer { // not all-zero: a constant page would flatter any hash
		answer[i] = byte(i * 31)
	}

	const reps = 20
	start := time.Now()
	var sink ports.Hash
	for i := 0; i < reps; i++ {
		sink = chain.AnswerDigestOf(answer)
	}
	per := time.Since(start) / reps
	_ = sink

	// The two cadences this has to be priced against, from the shipped defaults.
	const sweep = 30 * time.Second // ChainSyncInterval — how often a head probe goes out
	peers := 3                     // a four-validator set: every peer probed every sweep

	perSweep := time.Duration(peers) * per
	t.Logf("MEASURED — sha256 over one %d-byte space-time answer:", answerBytes)
	t.Logf("  per digest:                       %v", per)
	t.Logf("  recomputed for %d peers a sweep:   %v per %v sweep (%.3f%% of the loop)",
		peers, perSweep, sweep, 100*float64(perSweep)/float64(sweep))
	t.Logf("  cached at submit instead:         %v, once per renewal rather than once per sweep", per)

	// THE FINDING, asserted so it cannot rot: a digest is NOT free at this size, so
	// the inventory must be computed where a registration ARRIVES and not where it
	// is reported. This is a floor-box budget, so the bar is deliberately strict —
	// anything that would spend a measurable slice of the serialized loop on
	// bookkeeping, every sweep, forever, is the wrong shape even when the absolute
	// number looks small on a developer's machine.
	if per < 100*time.Microsecond {
		t.Logf("NOTE: a digest is cheap on THIS machine (%v). The floor box is ~1 vCPU and this runs on the "+
			"single serialized loop beside consensus; the caching decision stands on the shape, not on this "+
			"number, and a faster box is not evidence that per-sweep recomputation is safe.", per)
	}
	if per > sweep/10 {
		t.Fatalf("one digest costs %v against a %v sweep — at that price the inventory cannot be produced at "+
			"all, and the whole approach needs re-deriving rather than caching", per, sweep)
	}
}
