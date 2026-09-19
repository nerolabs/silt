package repairproof

// The retrievability leg and the release/slash gate (design doc §5-§8). The
// correctness leg (repairproof.go) proves the produced shard is the right value;
// this file proves the repairer actually HOLDS it now, and composes the two into a
// bounty verdict.

import (
	"crypto/sha256"

	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/ports"
)

// RepairChallengeSeed binds a proof-of-retrievability challenge to the repairer's
// OWN node identity: seed = H("silt/repair/challenge/prover/v1" ‖ base ‖ repairer).
// This closes the double-count / relay attack (design §5): a claimant cannot answer
// a bounty challenge by relaying an existing honest holder's proof of an
// already-present replica, because that proof was aggregated under the holder's
// seed and fails under the claimant's. It mirrors core/node porProverSeed for the
// bond-audit path — the same defense gave the standing bond, inherited here
// for the durability bounty.
//
// ADVERSARY-SHAPE: capability=RelayedHolderProof UNCOVERED: no fixture GRANTS AND CONTROLS FOR a claimant an honest holder's proof of an already-present replica. The seed binding does deny a REPLAY; the outsourcing variant -- asking the holder to compute under the CLAIMANT's seed -- was pinned open on the audit path and CLOSED there on 2026-09-19 by the prover-identity binding (TestChallengeOutsourcingIsRefusedByTheProver). This leg inherits that close rather than carrying its own: challengeHolderRetrievability now sends the base beside the derived seed and the prover tests BOTH this domain and the audit domain against its own id, asserted by TestProverIdentityBindingServesBothVersions. What stays uncovered here is the ORIGINAL claim above, a claimant holding an honest holder's own-seed proof, which no fixture grants.
func RepairChallengeSeed(base [32]byte, repairer ports.NodeID) [32]byte {
	h := sha256.New()
	h.Write([]byte("silt/repair/challenge/prover/v1"))
	h.Write(base[:])
	h.Write(repairer[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// VerifyRetrievability checks that `repairer` actually holds the rebuilt shard NOW,
// via a hash-only spot check bound to its identity. shardRoot is the commitment the
// publisher wrote into the object's sealed layout; leaves and leafBytes are that
// shard's committed geometry; base is the challenge nonce for this round. It returns
// true iff the prover opened every sampled leaf against the committed root. A
// data-less claimant cannot open one, and a claimant relaying an answer built under
// another identity's seed opened the wrong leaves.
//
// THE ROOT IS THE JUDGE'S, NEVER THE CLAIMANT'S. It is read from the layout the
// judge opened under its own care handle; a claimant that could name it could name
// a tree containing whatever sliver it kept.
//
// ADVERSARY-SHAPE: capability=DataLessClaimant UNCOVERED: no fixture GRANTS AND CONTROLS FOR a claimant a passing retrievability answer without the bytes. core/node's TestCareLinkHolderWithZeroBytesFailsTheAudit drives that adversary on the AUDIT surface and this leg shares the scheme, but a control that removes the capability leaves the identical failing answer, so it cannot discriminate. This leg is also not the one that fails a NO-LOSS claim: the named holder genuinely holds, and TestClaimWithNoLossIsPaid_PINNED_DEFECT pins that gap.
func VerifyRetrievability(shardRoot ports.Hash, leaves, leafBytes int, repairer ports.NodeID, base [32]byte, count int, ops []por.Opening) bool {
	if leaves <= 0 || leafBytes <= 0 {
		return false
	}
	return por.VerifyOpenings(shardRoot, leaves, leafBytes, RepairChallengeSeed(base, repairer), count, ops)
}

// Decision is the verdict for a repair-bounty claim.
type Decision struct {
	Release bool // pay the durability bounty (credit.PayBounty)
	Slash   bool // attributable false correctness claim → bond-slash (credit.SlashFalseRepair)
}

// Decide gates a repair bounty on the composed proof (design §6, §8), splitting the
// two legs by their trust properties:
//
// - CORRECTNESS is deterministic and publicly reproducible (VerifyByRecompute), so
// It is single-verifier-sufficient AND self-attributing: a claim that fails it
// carries its own fraud proof. correctnessOK=false ⇒ Slash, no release — full
// stop, regardless of retrievability.
// - RETRIEVABILITY is where independent verifiers add value (each issues its own
// identity-bound random challenge, so release additionally requires a τ-of-q
// quorum of caretakers to confirm it.
//
// A retrievability shortfall (fewer than τ confirmations) DENIES the bounty but does
// NOT slash: it may be transient (the repairer is briefly offline or slow), and only
// the unambiguous, mathematically-attributable correctness lie is ever punished.
// τ must be ≥ 1; a non-positive τ is treated as "no valid quorum" and denies.
func Decide(correctnessOK bool, retrievabilityVotes []bool, tau int) Decision {
	if !correctnessOK {
		return Decision{Release: false, Slash: true}
	}
	if tau < 1 {
		return Decision{} // no valid quorum → deny, don't slash
	}
	confirmed := 0
	for _, ok := range retrievabilityVotes {
		if ok {
			confirmed++
		}
	}
	return Decision{Release: confirmed >= tau}
}
