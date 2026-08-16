package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — tier 1, I3/B2 (in-package, mature-epoch weight quorum).
//
// I3 (`docs/design/consensus-invariants.md`): the finality quorum in a mature epoch
// is counted by WEIGHT over the frozen bonded snapshot, never by HEAD COUNT — the B2
// scar, where 8 minimum-bond sybils rode into the epoch snapshot as full members and a
// head-counted threshold handed a MinBond-per-head cohort stall/capture power that
// weight-counting denies. This oracle drives the REAL Chain over the mature-epoch
// weight rule and asserts: a coalition finalizes IFF it carries > ⅔ of the frozen
// epoch WEIGHT — so a head-count-majority-but-weight-minority sybil cohort is refused.
//
// Design + failing-first plan: docs/thinking/2026-08-15-406-model-check-approach.md.

const (
	i3Honest   = int64(20) << 20 // a real validator's weight
	i3Sybil    = int64(2) << 20  // a min-bond sybil
	i3NHonest  = 3
	i3NSybil   = 8                                     // the actual B2 number: 8 sybils give 7 non-proposer attesters = bftThreshold(11)
	i3TotalWt  = i3NHonest*i3Honest + i3NSybil*i3Sybil // 76 MiB
	i3TwoThird = 2 * i3TotalWt / 3                     // the >⅔ weight bar (≈50.7 MiB)
)

// matureWeightedEpoch drives a fresh objective chain (NO anchors) to a MATURE EPOCH
// whose frozen epochSet holds a deliberate weight imbalance: i3NHonest real validators
// (distinct domains, i3Honest each) + i3NSybil sybils (ONE shared domain, i3Sybil
// each). The honest decentralization matures it (Nakamoto 2 over both operators and
// domains); the same-domain sybils add head count but not decentralization — exactly
// the B2 snapshot. Returns the chain + the honest and sybil keys.
func matureWeightedEpoch(t *testing.T) (*Chain, []ed25519.PrivateKey, []ed25519.PrivateKey) {
	t.Helper()
	honest := make([]ed25519.PrivateKey, i3NHonest)
	for i := range honest {
		honest[i] = key(int64(7300 + i))
	}
	sybil := make([]ed25519.PrivateKey, i3NSybil)
	for i := range sybil {
		sybil[i] = key(int64(7400 + i))
	}
	const sybilDomain = uint64(0x5b11)

	cfg := Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 4, BondTTLBlocks: 0, // TTL off: genesis bonds hold
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis banks all 7 bonds directly (no anchors; honest[0] proposes/self-signs).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range honest {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, i3Honest, ports.Hash{}, uint64(i+1)))
	}
	for _, k := range sybil {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, i3Sybil, ports.Hash{}, sybilDomain))
	}
	Sign(g, honest[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Cross the EpochBlocks=4 boundary so the mature-epoch rotation freezes epochSet.
	// Pre-handoff the count quorum is bftThreshold(7)=4, so attest with everyone.
	attAll := func(b *Block, proposer ed25519.PrivateKey) {
		for _, k := range append(append([]ed25519.PrivateKey{}, honest...), sybil...) {
			if !ed25519.PublicKey(k.Public().(ed25519.PublicKey)).Equal(proposer.Public()) {
				b.Atts = append(b.Atts, Attest(b, k))
			}
		}
	}
	// ROTATE the proposer across the honest validators: C2Metric counts only
	// validatorsSeen (ATTESTERS), so a validator that only ever proposes is never
	// counted — leaving it out drops the Nakamoto coefficient (a single un-diluted
	// 20 MiB bond then exceeds ⅓ → coefficient 1 → never matures). Rotating makes
	// every honest validator attest at least once, so all three are "seen" and the
	// coefficient is 2. (This exact trap is why the setup is verified before the oracle.)
	for h := uint64(1); h <= 4; h++ {
		prev, _ := c.Head()
		proposer := honest[int(h)%i3NHonest]
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, proposer)
		attAll(b, proposer)
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit block %d (crossing to handoff): %v", h, err)
		}
	}
	return c, honest, sybil
}

// TestModelCheck_I3_SetupReachesMatureWeightedEpoch verifies the SETUP before any
// oracle trusts it (the anti-#303 discipline: a green oracle over a broken setup is
// worse than none). It asserts we are in a mature epoch with the exact frozen weighted
// membership the B2 oracle depends on.
func TestModelCheck_I3_SetupReachesMatureWeightedEpoch(t *testing.T) {
	c, honest, sybil := matureWeightedEpoch(t)

	if !c.matureEpoch {
		t.Fatalf("setup: must be in a MATURE epoch (matureEpoch) for the weight rule to apply; handedOff=%v everMature=%v", c.handedOff(), c.EverMature())
	}
	if got := len(c.epochSet); got != i3NHonest+i3NSybil {
		t.Fatalf("setup: frozen epochSet must hold all %d members, got %d", i3NHonest+i3NSybil, got)
	}
	var total int64
	for _, k := range honest {
		if w := c.epochSet[idOf(k)]; w != i3Honest {
			t.Fatalf("setup: honest %s frozen weight = %d, want %d", idOf(k), w, i3Honest)
		}
		total += c.epochSet[idOf(k)]
	}
	for _, k := range sybil {
		if w := c.epochSet[idOf(k)]; w != i3Sybil {
			t.Fatalf("setup: sybil %s frozen weight = %d, want %d", idOf(k), w, i3Sybil)
		}
		total += c.epochSet[idOf(k)]
	}
	if total != i3TotalWt {
		t.Fatalf("setup: total frozen weight = %d, want %d", total, i3TotalWt)
	}
	// The B2 precondition: the sybil cohort, as NON-PROPOSER attesters (one sybil
	// proposes), meets the pre-B2 head-count bar bftThreshold(memberCount) — so under
	// head-counting it WOULD finalize — while being a weight minority.
	if i3NSybil-1 < bftThreshold(i3NHonest+i3NSybil) {
		t.Fatalf("setup: the %d sybils must supply a head-count quorum as non-proposer attesters (%d ≥ bftThreshold(%d)=%d) for the B2 scenario",
			i3NSybil, i3NSybil-1, i3NHonest+i3NSybil, bftThreshold(i3NHonest+i3NSybil))
	}
	if i3NSybil*i3Sybil > i3TwoThird {
		t.Fatal("setup: the sybil cohort must be a WEIGHT minority (< ⅔) — otherwise there is no B2 gap to test")
	}
	t.Logf("setup OK: mature epoch, %d members, total %d MiB, ⅔ bar %d MiB; %d sybils = head-count quorum but weight %d MiB (minority)",
		len(c.epochSet), total>>20, i3TwoThird>>20, i3NSybil, (i3NSybil*i3Sybil)>>20)
}

// TestModelCheck_I3_MatureWeightQuorum is the I3/B2 oracle: over EVERY coalition of the
// frozen epoch set, a block finalizes IFF its support carries a >⅔ WEIGHT super-majority
// — never a head-count one. So the 8-sybil cohort, though a head-count quorum
// (7 non-proposer attesters = bftThreshold(11)), cannot finalize (weight 16 << ⅔·76).
// The equivalence `passes ⟺ (≥1 attester ∧ 3·weight > 2·total)` is the exact rule
// ValidateCommit enforces (requireEpochWeightQuorum).
//
// FAILING-FIRST (verified by a controlled revert): with requireEpochWeightQuorum
// temporarily counting HEADS (the pre-B2 rule), a head-count-supermajority-but-weight-
// minority coalition finalizes and the oracle catches it — RED. Under the shipped weight
// rule (#389) it is refused — GREEN.
func TestModelCheck_I3_MatureWeightQuorum(t *testing.T) {
	c, honest, sybil := matureWeightedEpoch(t)
	prev, _ := c.Head() // height-4 head; candidates are at height 5 (still the mature epoch)

	// EXHAUSTIVE over the finality-relevant space, by equivalence class. All honest
	// weigh i3Honest and all sybils i3Sybil, and every member is epochSet-qualified, so
	// the finality verdict is a pure function of (proposer type, #honest attesters,
	// #sybil attesters) — NOT of identities. Enumerating one representative per class is
	// therefore exhaustive over everything that can change the outcome, in ~72 cases
	// instead of 2^10·11 (which is the same coverage but re-verifies interchangeable sigs).
	build := func(proposer ed25519.PrivateKey, honAtt, sybAtt []ed25519.PrivateKey) (*Block, int64, int) {
		b := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(5)}}
		Sign(b, proposer)
		weight := c.epochSet[idOf(proposer)]
		for _, k := range append(append([]ed25519.PrivateKey{}, honAtt...), sybAtt...) {
			b.Atts = append(b.Atts, Attest(b, k))
			weight += c.epochSet[idOf(k)]
		}
		return b, weight, len(honAtt) + len(sybAtt)
	}
	checked := 0
	// proposerKind: honest[0] (leaves honest[1..2] + all 8 sybils as attesters) and
	// sybil[0] (leaves all 3 honest + sybil[1..7]).
	for _, pk := range []struct {
		proposer ed25519.PrivateKey
		honPool  []ed25519.PrivateKey
		sybPool  []ed25519.PrivateKey
	}{
		{honest[0], honest[1:], sybil},
		{sybil[0], honest, sybil[1:]},
	} {
		for h := 0; h <= len(pk.honPool); h++ {
			for s := 0; s <= len(pk.sybPool); s++ {
				b, weight, atts := build(pk.proposer, pk.honPool[:h], pk.sybPool[:s])
				passes := c.ValidateCommit(b) == nil
				// The rule: a >⅔ WEIGHT super-majority (3·support > 2·total), AND at least
				// one non-proposer attester (RequiredQuorum floor Quorum=1 in a mature epoch).
				want := atts >= 1 && 3*weight > 2*i3TotalWt
				if passes != want {
					t.Fatalf("I3 VIOLATION — proposer-weight %d + %dh/%ds attesters (support %d MiB of %d, ⅔ bar %d): passed=%v want %v "+
						"(finality must track WEIGHT, not head count)",
						c.epochSet[idOf(pk.proposer)]>>20, h, s, weight>>20, i3TotalWt>>20, i3TwoThird>>20, passes, want)
				}
				checked++
			}
		}
	}

	// B2 spotlight: the sybil cohort alone is a HEAD-COUNT quorum but must NOT finalize.
	syb := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(6)}}
	Sign(syb, sybil[0])
	for _, k := range sybil[1:] {
		syb.Atts = append(syb.Atts, Attest(syb, k))
	}
	if err := c.ValidateCommit(syb); err == nil {
		t.Fatalf("B2: the %d-sybil cohort is a head-count quorum (bftThreshold(%d)=%d) but a weight minority — it must NOT finalize",
			i3NSybil, i3NHonest+i3NSybil, bftThreshold(i3NHonest+i3NSybil))
	}
	// Positive control: the 3 honest carry >⅔ weight and DO finalize.
	hon := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(7)}}
	Sign(hon, honest[0])
	hon.Atts = []Attestation{Attest(hon, honest[1]), Attest(hon, honest[2])}
	if err := c.ValidateCommit(hon); err != nil {
		t.Fatalf("positive control: the 3 honest (weight %d MiB > ⅔ bar %d) must finalize: %v", (i3NHonest*i3Honest)>>20, i3TwoThird>>20, err)
	}
	t.Logf("I3 oracle GREEN: %d coalitions checked, finality tracks weight not head count; sybil head-count quorum refused, honest weight-majority admitted", checked)
}
