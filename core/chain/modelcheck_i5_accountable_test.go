package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — tier 1, I5 ACCOUNTABLE SAFETY, EXHAUSTIVE.
//
// The red-team #183 verdict's coverage caveat C-1: I5's "an honest node is
// NEVER slashed" (and its dual, "a genuine double-sign IS caught") was covered
// by SCENARIO tests (equivocation_proposer_test.go, modelcheck_i5_357_test.go),
// not by an exhaustive adversarial-schedule sweep. This promotes the I5
// accountable-safety oracle into the enumerated tier: it drives the REAL
// VerifyEquivocation predicate over the FULL space of signature schedules one
// key can produce across two same-height blocks, and asserts the exact
// characterization on every one — the honest-never-slashed direction and the
// completeness direction at once.
//
// THE INVARIANT (equivocation.go): two era-2 blocks A, B at one height are a
// provable double-sign IFF the culprit released a verifying consensus signature
// at the SAME (round, phase) slot in BOTH, over DIFFERENT block hashes. Every
// other schedule is honest and must NOT be flagged:
//   - disjoint slots (signed prepare-r0 in A, precommit-r0 or prepare-r1 in B):
//     the lock-change-under-POL liveness escape (#432/#397 I5 requirement);
//   - the SAME block hash in both (idempotent re-sign / re-broadcast);
//   - a bare-hash ProposerSig, which is authorship, not a consensus vote.
//
// FAILING-FIRST (verified by controlled revert): dropping the `sa == sb`
// same-slot guard in VerifyEquivocation (flag on ANY shared key across
// different hashes) makes the honest cross-round schedules flag — RED, the
// #397 honest-self-slash. With the shipped slot guard — GREEN.

// era2SigSlot is one (round, phase) at which a culprit released a consensus
// signature. phase ∈ {PhasePrepare, PhasePrecommit}; round ∈ {0, 1}.
type era2SigSlot struct {
	round uint64
	phase uint8
}

// allEra2Slots is the enumerated slot alphabet: {r0,r1} × {prepare,precommit}.
func allEra2Slots() []era2SigSlot {
	var out []era2SigSlot
	for _, r := range []uint64{0, 1} {
		for _, p := range []uint8{PhasePrepare, PhasePrecommit} {
			out = append(out, era2SigSlot{round: r, phase: p})
		}
	}
	return out
}

// blockWithCulpritSlots builds an era-2 block over `seed` content, carrying the
// culprit's genuine consensus signature at each slot in `slots` (split across
// PrepareQC / Atts by phase, exactly where collectQuorumSigs / consensusSigScopes
// read them). The returned block's hash is fixed by `seed`, so two blocks with
// the same seed share a hash (the idempotent case) and different seeds conflict.
func blockWithCulpritSlots(culprit ed25519.PrivateKey, seed byte, slots []era2SigSlot) Block {
	b := Block{Version: BlockVersionRounds, Height: 1, Entries: []ports.Entry{entry(seed)}}
	for _, s := range slots {
		att := AttestAt(&b, culprit, s.round, s.phase)
		if s.phase == PhasePrepare {
			b.PrepareQC = append(b.PrepareQC, att)
		} else {
			b.Atts = append(b.Atts, att)
		}
	}
	return b
}

func slotsIntersect(a, b []era2SigSlot) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// TestModelCheck_I5_AccountableSafety_Exhaustive enumerates EVERY pair of
// culprit slot-sets (2^4 × 2^4) across two same-height era-2 blocks, in both
// the same-hash and different-hash cases, and asserts VerifyEquivocation
// returns exactly (differentHash ∧ slotsIntersect) — no honest schedule is ever
// convicted, and no genuine same-slot double-sign is ever missed.
func TestModelCheck_I5_AccountableSafety_Exhaustive(t *testing.T) {
	culprit := key(9700)
	pub := append([]byte(nil), culprit.Public().(ed25519.PublicKey)...)
	slots := allEra2Slots()
	subsets := era2SlotSubsets(slots) // 2^4 = 16 subsets

	checked, convictions := 0, 0
	for _, sa := range subsets {
		for _, sb := range subsets {
			// diffHash=false → B reuses A's seed (identical hash); true → distinct.
			for _, diffHash := range []bool{false, true} {
				a := blockWithCulpritSlots(culprit, 1, sa)
				seedB := byte(1)
				if diffHash {
					seedB = 2
				}
				bb := blockWithCulpritSlots(culprit, seedB, sb)

				want := diffHash && slotsIntersect(sa, sb)
				e := Equivocation{Culprit: pub, A: a, B: bb}
				got := VerifyEquivocation(&e)
				if got != want {
					t.Fatalf("I5 accountable-safety VIOLATION: slots A=%v B=%v diffHash=%v → VerifyEquivocation=%v, want %v (a false slash convicts an honest cross-round/re-sign schedule; a false negative lets a same-slot double-sign escape)",
						sa, sb, diffHash, got, want)
				}
				if got {
					convictions++
				}
				checked++
			}
		}
	}
	// Both directions must be exercised: some schedules convict (completeness)
	// and most do not (the honest space is large). A test that never convicts
	// would pass a broken predicate that flags nothing.
	if convictions == 0 {
		t.Fatal("oracle never convicted — the completeness direction is untested")
	}
	if checked != len(subsets)*len(subsets)*2 {
		t.Fatalf("enumeration miscount: checked %d, want %d", checked, len(subsets)*len(subsets)*2)
	}
}

// TestModelCheck_I5_ForkChoiceDeterminism_AllPermutations promotes the #357
// order-independence scenario (modelcheck_i5_357_test.go, three hand-picked
// orders) to the EXHAUSTIVE tier: reconciling the SAME set of competing forks
// in EVERY permutation must land on the identical head — fork-choice is a pure
// function of the message multiset, never a function of arrival order (the
// #357 hash-luck-tiebreak scar). k=4 forks → 4! = 24 orderings.
//
// FAILING-FIRST (verified by controlled revert): the #357 fix is what makes
// this hold — modelcheck_i5_357_test.go records that forcing finalityQuorumActive
// false reopens the order-dependent reorg. This oracle widens the witness from
// 3 orders to all 24, so an order-dependence that only surfaces on an untested
// permutation cannot hide.
func TestModelCheck_I5_ForkChoiceDeterminism_AllPermutations(t *testing.T) {
	// The fork set (built fresh per run so no shared mutation): a much-taller
	// fork, a bare genesis, and two mid-height conflicts — the same shapes the
	// #357 scenario used, now swept over every order.
	buildForks := func(g *Block, ak []ed25519.PrivateKey) [][]Block {
		return [][]Block{
			anchorFork(g, ak, 5, 130),
			{*g},
			anchorFork(g, ak, 2, 110),
			anchorFork(g, ak, 3, 120),
		}
	}
	run := func(order []int) (ports.Hash, uint64) {
		c, ak, g := ramp357(t)
		forks := buildForks(g, ak)
		for _, i := range order {
			c.Reconcile(forks[i])
		}
		return c.Head()
	}

	perms := permutations([]int{0, 1, 2, 3})
	if len(perms) != 24 {
		t.Fatalf("expected 24 permutations of 4 forks, got %d", len(perms))
	}
	wantHash, wantHeight := run(perms[0])
	for _, order := range perms {
		h, ht := run(order)
		if h != wantHash || ht != wantHeight {
			t.Fatalf("#357 I5 VIOLATION — fork-choice is order-dependent: order %v → %x@%d, but order %v → %x@%d (fork-choice must be a pure function of the fork set)",
				perms[0], wantHash, wantHeight, order, h, ht)
		}
	}
}

// permutations returns every ordering of xs (Heap's algorithm, n! results).
func permutations(xs []int) [][]int {
	var out [][]int
	var gen func(k int, a []int)
	gen = func(k int, a []int) {
		if k == 1 {
			cp := make([]int, len(a))
			copy(cp, a)
			out = append(out, cp)
			return
		}
		for i := 0; i < k; i++ {
			gen(k-1, a)
			if k%2 == 0 {
				a[i], a[k-1] = a[k-1], a[i]
			} else {
				a[0], a[k-1] = a[k-1], a[0]
			}
		}
	}
	gen(len(xs), append([]int(nil), xs...))
	return out
}

// era2SlotSubsets returns every subset of the slot alphabet (2^n).
func era2SlotSubsets(slots []era2SigSlot) [][]era2SigSlot {
	n := len(slots)
	out := make([][]era2SigSlot, 0, 1<<n)
	for mask := 0; mask < (1 << n); mask++ {
		var s []era2SigSlot
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				s = append(s, slots[i])
			}
		}
		out = append(out, s)
	}
	return out
}

// TestModelCheck_I5_ProposerSigIsNotAVote_Exhaustive pins the bare-hash
// ProposerSig exemption across the same slot space: a validator that AUTHORED
// two different blocks at one height (a bare ProposerSig on each, no consensus
// attestation) is NOT convicted in era 2 — authorship is not a vote (the #432
// re-propose-fresh-after-a-lock-free-view-change liveness rule). The moment it
// adds a real same-slot consensus signature to both, it IS convicted.
func TestModelCheck_I5_ProposerSigIsNotAVote_Exhaustive(t *testing.T) {
	author := key(9701)
	pub := append([]byte(nil), author.Public().(ed25519.PublicKey)...)

	// Two different era-2 blocks, each merely AUTHORED (ProposerSig only).
	mkAuthored := func(seed byte) Block {
		b := Block{Version: BlockVersionRounds, Height: 1, Entries: []ports.Entry{entry(seed)}}
		Sign(&b, author) // bare-hash ProposerSig — authorship
		return b
	}
	a, bb := mkAuthored(1), mkAuthored(2)
	if VerifyEquivocation(&Equivocation{Culprit: pub, A: a, B: bb}) {
		t.Fatal("I5 VIOLATION: authoring two different blocks at one height was convicted — a bare-hash ProposerSig is authorship, not a consensus vote (#432)")
	}
	// Now the author ALSO consensus-signs both at the same (r0, prepare) slot →
	// that is the double-vote, and it must convict.
	a.PrepareQC = append(a.PrepareQC, AttestAt(&a, author, 0, PhasePrepare))
	bb.PrepareQC = append(bb.PrepareQC, AttestAt(&bb, author, 0, PhasePrepare))
	if !VerifyEquivocation(&Equivocation{Culprit: pub, A: a, B: bb}) {
		t.Fatal("I5 VIOLATION: a real same-slot consensus double-sign by the author was NOT convicted")
	}
}
