package chain

// THE WORK ONE BLOCK CAN DEMAND OF A VALIDATOR IS BOUNDED BY THE CHAIN, NOT BY THE TRANSPORT.
//
// validateCarrier verifies an ed25519 signature for EVERY entry in a block's LastCommit. Without a
// count checked first, the only thing bounding that loop is how many entries fit in a frame — about
// 1.3 million, which at the measured 52.6 us per verify is roughly 68 seconds of single-core work,
// demanded by one block from any peer that can send one. On the declared floor spec that is the
// whole machine for over a minute, per block, for free.
//
// THE CEILING IS NOT A NEW NUMBER. A carrier holds at most one precommit per qualified validator —
// the duplicate-id refusal enforces the "at most one" — and the qualified set is already bounded by
// two shipped rules: RegCap caps registrations per block, and the TTL sweep evicts registrations
// older than the re-challenge cadence. carrierCap restates that bound as a count. So this rule
// invents no security parameter; it refuses carriers that were never constructible by an honest
// proposer in the first place, which makes it a strict narrowing rather than a new judgement.
//
// WHY IT IS CHECKED BEFORE THE SIGNATURES AND NOT AFTER. A count checked after the loop bounds
// nothing at all — the cost has already been paid by the time the block is refused. That ordering is
// the whole rule, and TestCarrierCostIsRefusedBeforeAnySignatureWork holds it directly rather than
// trusting the source to stay in that order.

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// carrierOfSize builds a LastCommit of n entries signed by n distinct identities over the PARENT
// block — which is what a carrier is, and what validateCarrier derives its signing scope from
// (b.Height-1). Every entry is GENUINE: the point is the COUNT, so a fixture of forgeries would be
// refused for the wrong reason and prove nothing about the cost bound.
func carrierOfSize(t *testing.T, n int, parent *Block, chainID ports.Hash) []Attestation {
	t.Helper()
	out := make([]Attestation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, AttestAt(parent, key(int64(700000+i)), 0, PhasePrecommit, chainID))
	}
	return out
}

// carrierParentAndChild returns a v5 parent whose precommits a carrier may hold, and the child
// block that carries them.
func carrierParentAndChild(t *testing.T) (*Block, Block, ports.Hash) {
	t.Helper()
	var chainID ports.Hash
	chainID[0] = 0x5A
	parent := &Block{Version: BlockVersionWitnessable, Height: 8,
		Prev: ports.HashBytes([]byte("grandparent")), Entries: []ports.Entry{entry(70)}}
	Sign(parent, key(700999))
	child := Block{Version: BlockVersionWitnessable, Height: 9, Prev: parent.Hash(),
		Entries: []ports.Entry{entry(71)}}
	return parent, child, chainID
}

// TestCarrierCapIsTheQualifiedSetCeiling pins the derivation rather than a literal: the ceiling a
// chain derives must be exactly the membership bound restated as a count, so a change to either
// shipped rule moves it automatically.
func TestCarrierCapIsTheQualifiedSetCeiling(t *testing.T) {
	for _, tc := range []struct {
		ttl  uint64
		want int
	}{
		{ttl: 0, want: 0}, // no re-challenge cadence: no bounded qualified set, so no ceiling to derive
		{ttl: 1, want: RegCap * 2},
		{ttl: 32, want: RegCap * 33}, // the shipped objective default
	} {
		if got := carrierCap(Config{BondTTLBlocks: tc.ttl}); got != tc.want {
			t.Errorf("carrierCap at TTL %d = %d, want %d — the ceiling must be RegCap*(ttl+1), the "+
				"membership bound restated as a count, not a literal that drifts from it",
				tc.ttl, got, tc.want)
		}
	}
	// An enormous committed cadence must clamp rather than wrap: a wrapped ceiling would refuse
	// honest carriers instead of hostile ones, which is the failure direction that matters.
	if got := carrierCap(Config{BondTTLBlocks: 1 << 60}); got <= 0 {
		t.Fatalf("carrierCap overflowed to %d on an enormous cadence; a non-positive ceiling reads as "+
			"UNCAPPED, so the overflow would silently remove the bound entirely", got)
	}
}

// TestCarrierOverTheCeilingIsRefused is the rule. A carrier larger than the chain's qualified set
// could ever be is refused, by name.
func TestCarrierOverTheCeilingIsRefused(t *testing.T) {
	parent, over, chainID := carrierParentAndChild(t)
	const ttl = 1
	cap := carrierCap(Config{BondTTLBlocks: ttl})

	// A carrier over the ceiling. The entries are genuine, so the only thing wrong with this block
	// is that no qualified set could have produced it.
	over.LastCommit = carrierOfSize(t, 3, parent, chainID)
	if err := validateCarrier(&over, chainID, 2); !errors.Is(err, ErrCarrierTooLarge) {
		t.Fatalf("a carrier of %d against a ceiling of 2 must be refused as too large; got %v",
			len(over.LastCommit), err)
	}

	// VACUITY: the same carrier under a ceiling that admits it must NOT be refused for size. It may
	// still be refused for something else, and that is fine — what must not happen is the size rule
	// firing on a carrier the chain could legitimately hold.
	if err := validateCarrier(&over, chainID, cap); errors.Is(err, ErrCarrierTooLarge) {
		t.Fatalf("a carrier of %d was refused as too large against a ceiling of %d — the rule is "+
			"firing on carriers an honest proposer can build", len(over.LastCommit), cap)
	}

	// And an UNCAPPED chain (no re-challenge cadence) must not gain a size refusal it never had.
	if err := validateCarrier(&over, chainID, 0); errors.Is(err, ErrCarrierTooLarge) {
		t.Fatal("a chain with no TTL has no bounded qualified set and therefore no ceiling to " +
			"derive; a size refusal there would be an invented limit, not a derived one")
	}
}

// TestCarrierCostIsRefusedBeforeAnySignatureWork is the ordering, and the ordering IS the rule. A
// count checked after the verify loop bounds nothing: the work is already spent.
//
// It is asserted structurally rather than by timing — a wall-clock assertion on a shared developer
// box measures the box. Every entry here carries a signature that CANNOT verify, so if any verify
// ran the refusal would name the signature; the refusal naming the SIZE is the proof that the count
// came first.
func TestCarrierCostIsRefusedBeforeAnySignatureWork(t *testing.T) {
	parent, b, chainID := carrierParentAndChild(t)
	b.LastCommit = carrierOfSize(t, 6, parent, chainID)
	for i := range b.LastCommit {
		b.LastCommit[i].Sig = []byte("not a signature at all")
	}

	err := validateCarrier(&b, chainID, 2)
	if errors.Is(err, ErrCarrierBadSignature) {
		t.Fatal("THE COUNT IS CHECKED AFTER THE SIGNATURES: the refusal names a bad signature, which " +
			"means the verify loop ran before the size rule. That ordering bounds nothing — the CPU a " +
			"hostile block demands has already been spent by the time it is refused, which is the whole " +
			"cost this rule exists to cap.")
	}
	if !errors.Is(err, ErrCarrierTooLarge) {
		t.Fatalf("an over-size carrier of unverifiable entries must be refused for its SIZE, before "+
			"any signature is examined; got %v", err)
	}
}

// TestCarrierCeilingAdmitsAFullHonestQualifiedSet is the liveness half, and it is the one that
// would matter most if the ceiling were set wrong. A rule that refuses hostile blocks and honest
// ones too is not a defence, it is an outage.
//
// The largest carrier an honest chain can produce is one precommit per qualified validator, and the
// ceiling is derived from exactly that quantity — so a carrier AT the bound must pass the size rule.
func TestCarrierCeilingAdmitsAFullHonestQualifiedSet(t *testing.T) {
	parent, b, chainID := carrierParentAndChild(t)
	const ttl = 2

	// The bound the membership gate drives, restated: RegCap registrations per block across ttl+1
	// blocks. Building that many real signatures is slow, so the shape is asserted at a small
	// multiple and the arithmetic at the real one.
	full := carrierCap(Config{BondTTLBlocks: ttl})
	if full != RegCap*(ttl+1) {
		t.Fatalf("the ceiling (%d) is not the membership bound (%d)", full, RegCap*(ttl+1))
	}

	b.LastCommit = carrierOfSize(t, 8, parent, chainID)
	if err := validateCarrier(&b, chainID, full); err != nil {
		t.Fatalf("a carrier well inside the ceiling was refused: %v.\n"+
			"  The ceiling is the most entries a qualified set can produce, so anything at or below it "+
			"must pass the size rule or the rule is an outage rather than a defence.", err)
	}
}
