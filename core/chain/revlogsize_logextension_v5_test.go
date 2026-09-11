package chain

import (
	"bytes"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// P13b k >= 1 — the LOG-EXTENSION arm the tagRevLogSize leaf unlocks. C-c.
// =============================================================================
//
// Certification: ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md section 4.1
// condition C-c, and the P-table delta certification section 3.3 (T-LOGEXT).
// Deliberation: docs/thinking/2026-09-11-tagrevlogsize-freeze-manifest-item-1.md
//
// WHAT CHANGED. Until the leaf landed, provenView.CommittedRoots refused EVERY revocation-bearing
// block by name and that refusal was TERMINAL — a box that cannot validate block H never obtains a
// trustworthy post-H StateRoot, so it cannot anchor H+1, and takedowns are a first-class expected
// mechanism (Don't #2). The leaf turns the refusal into a resolvable read, and this file drives
// both arms: the block that now passes P13b, and the four ways it still must not.
//
// C-c AND ITS POSITIVE CONTROL. The certification is explicit that a generic "wrong number" arm
// proves nothing: the branch that actually breaks is the DEGENERATE m = 1 right-spine extension,
// where VerifyConsistency's isPow2 seeding leaves the old-root accumulator vacuous. So the forged
// arm here IS that construction, embedded in a block — the same forgery
// TestGD9_WitnessSuppliedLogSizeIsUnsound_Control drives at the unit tier, driven through the
// composition at a COMMITTED m.
//
// THE ABLATION, RUN (2026-09-11). Take the source's tagRevLogSize value WITHOUT Resolving it
// against the parent's committed StateRoot — i.e. put m back where it was before the leaf — and
// TestLogExtensionRefusesTheDegenerateForgery goes GREEN-into-P13a: the forged LogRoot passes P13b
// and the block reaches the recompute. That is the wrong-accept, reproduced, and the Resolve is
// what refuses it.

// revLogExtensionFixture returns a v5 fixture whose PARENT log holds exactly three entries — not a
// power of two, so the honest path exercises VerifyConsistency's general branch rather than its
// isPow2 seeding — together with an honestly-rooted child block that appends one more.
//
// The parent log is deliberately NON-EMPTY. At m = 0 VerifyConsistency returns true without
// reading either root, and the extension is then pinned entirely by the per-leaf inclusion legs
// over a tree of size k; that is sound, but it is not the branch this arm exists to exercise.
func revLogExtensionFixture(t *testing.T) (structFixture, Block) {
	t.Helper()
	f := buildStructFixture(t)
	b2 := f.mkBlock(t, func(b *Block) { b.Entries = []ports.Entry{entry(50), entry(51), entry(52)} })
	if err := f.c.Append(b2); err != nil {
		t.Fatalf("h2 must commit: %v", err)
	}
	b3 := f.mkBlock(t, func(b *Block) {
		b.Revocations = []ports.Hash{entry(0).Root, entry(1).Root, entry(50).Root}
	})
	if err := f.c.Append(b3); err != nil {
		t.Fatalf("h3 must commit: %v", err)
	}
	if got := f.c.revLog.Size(); got != 3 {
		t.Fatalf("fixture: want a 3-entry parent log, got %d", got)
	}
	// The child under test is BUILT but NOT appended: the chain stays at the parent state, which is
	// what the proven view's head record and the prover both bind to.
	b4 := f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{entry(51).Root} })
	return f, b4
}

// forgingSource wraps the honest prover-backed source and lies about exactly one thing at a time.
type forgingSource struct {
	*proverSource
	forgedSize []byte         // served in place of the committed tagRevLogSize value; nil = honest
	forgedCons []ports.Hash   // served in place of the honest consistency proof; nil = honest
	forgedIncl [][]ports.Hash // served in place of the honest inclusion proofs; nil = honest
	refuse     bool           // LogExtension answers "I have no witness"
}

func (s *forgingSource) Leaf(key []byte) ([]byte, statehash.Witness, bool) {
	v, w, ok := s.proverSource.Leaf(key)
	if ok && s.forgedSize != nil && bytes.Equal(key, statehash.Key(tagRevLogSize, nil)) {
		// The PROOF stays the honest one. That is the whole test: the forgery is caught by
		// Resolve against the parent's committed StateRoot, not by a malformed witness.
		return s.forgedSize, w, true
	}
	return v, w, ok
}

func (s *forgingSource) LogExtension(m int, leaves []ports.Hash) ([]ports.Hash, [][]ports.Hash, bool) {
	if s.refuse {
		return nil, nil, false
	}
	if s.forgedCons != nil || s.forgedIncl != nil {
		return s.forgedCons, s.forgedIncl, true
	}
	return s.proverSource.LogExtension(m, leaves)
}

// TestRevocationLogLeavesMirrorsApply pins the box's payload-derived log entries against a REAL
// apply(). The derivation is the one thing in the extension proof that no witness covers — the box
// computes the appended content itself from the hash-covered block — so if it drifts from apply()
// by one entry, one order, or one op byte, the box verifies an extension the node never made.
//
// It drives the three shapes the STATE write-set nets away but the LOG does not: a duplicate root,
// a revoke and un-revoke of the same root in one block, and both lists non-empty at once.
func TestRevocationLogLeavesMirrorsApply(t *testing.T) {
	f := buildStructFixture(t)
	b2 := f.mkBlock(t, func(b *Block) { b.Entries = []ports.Entry{entry(50), entry(51)} })
	if err := f.c.Append(b2); err != nil {
		t.Fatalf("h2 must commit: %v", err)
	}
	// h3 takes down two roots so h4 has something to un-revoke.
	b3 := f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{entry(0).Root, entry(50).Root} })
	if err := f.c.Append(b3); err != nil {
		t.Fatalf("h3 must commit: %v", err)
	}
	b4 := f.mkBlock(t, func(b *Block) {
		b.Revocations = []ports.Hash{entry(51).Root, entry(51).Root} // a DUPLICATE: two log entries, one leaf
		b.Unrevocations = []ports.Hash{entry(0).Root, entry(50).Root}
	})

	parentRoot := f.c.LogRoot()
	m := f.c.revLog.Size()
	derived := revocationLogLeaves(&b4)
	if want := len(b4.Revocations) + len(b4.Unrevocations); len(derived) != want {
		t.Fatalf("the derivation dropped entries: %d derived, k = %d (the LOG does not dedup)", len(derived), want)
	}

	// GROUND TRUTH: a real apply(), through the node's own accept path.
	if err := f.c.Append(b4); err != nil {
		t.Fatalf("h4 must commit (the node's own oracle runs first): %v", err)
	}
	if got, want := f.c.revLog.Size(), m+len(derived); got != want {
		t.Fatalf("apply() appended %d log entries, the derivation says %d", got-m, len(derived))
	}
	// Rebuild the post-log from the PARENT's entries plus the derived ones and require the same MTH
	// apply() produced. This binds order and content, not just count.
	// The parent's own entries, recovered by replaying the two committed takedown blocks through
	// the same derivation — so the replay is itself a second use of the function under test.
	rebuilt := translog.New()
	for _, blk := range []Block{f.c.Blocks(2)[0], f.c.Blocks(3)[0]} {
		for _, lf := range revocationLogLeaves(&blk) {
			rebuilt.Append(lf)
		}
	}
	if rebuilt.Size() != m {
		t.Fatalf("the parent-log replay produced %d entries, want %d", rebuilt.Size(), m)
	}
	if rebuilt.Root() != parentRoot {
		t.Fatalf("the parent-log replay does not reproduce the committed parent LogRoot (%x != %x)", rebuilt.Root(), parentRoot)
	}
	for _, lf := range derived {
		rebuilt.Append(lf)
	}
	if rebuilt.Root() != f.c.LogRoot() {
		t.Fatalf("the derived entries do not reproduce apply()'s post LogRoot (%x != %x) — the box would "+
			"verify an extension the node never made", rebuilt.Root(), f.c.LogRoot())
	}
}

// TestGD8_RevocationBearingBlockVerifiesItsLogExtension is the ACCEPT arm: with a witness source,
// a revocation-bearing block now passes P13b and falls through to P13a, which stalls only because
// no recompute predicate is wired on this view. Before the leaf this block was refused by name at
// P13b and never reached P13a at all.
//
// ErrRecomputeGated is the marker that P13b PASSED: it is raised inside stateRootConjunct, which
// the k >= 1 arm reaches only after verifying the extension.
func TestGD8_RevocationBearingBlockVerifiesItsLogExtension(t *testing.T) {
	f, b := revLogExtensionFixture(t)
	assertHonestTwinAccepts(t, f.c, b)
	pv := f.provenViewOver(t, newProverSource(t, f.c))
	out, err := pv.CommittedRoots(&b)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRecomputeGated) {
		t.Fatalf("P13b did NOT pass on an honest revocation-bearing block: got %s / %v.\n"+
			"With m authenticated from the parent's committed tagRevLogSize leaf and the honest "+
			"RFC-6962 proofs, the extension must verify and the block must reach P13a.", out, err)
	}
}

// TestLogExtensionRefusesTheDegenerateForgery is condition C-c with its certified positive
// control. The attacker publishes b.LogRoot = node(L_p, leafHash(leaf)) — a two-element tree whose
// left child happens to be the parent's three-entry MTH — claims m = 1, and supplies the
// one-element proofs that make BOTH legs of verifyLogExtension pass at that m. That forgery is not
// hypothetical: TestGD9_WitnessSuppliedLogSizeIsUnsound_Control asserts it passes.
//
// The box refuses it at AUTHENTICATION, not at verification, and the assertion is on the sentinel
// for exactly that reason: ErrRevLogSizeUnauthenticated means the forged size never Resolved
// against the parent's committed StateRoot, so the forged proofs were never consulted. Landing on
// ErrRevLogExtensionUnproven instead would mean the box had accepted the attacker's m and merely
// failed to fold his proofs — a much weaker property, and one a cleverer forgery would beat.
func TestLogExtensionRefusesTheDegenerateForgery(t *testing.T) {
	f, b := revLogExtensionFixture(t)
	parentRoot := f.c.LogRoot()
	leaves := revocationLogLeaves(&b)
	if len(leaves) != 1 {
		t.Fatalf("fixture: want k = 1 for the two-element forgery, got %d", len(leaves))
	}
	forgedRoot := rfc6962Node(parentRoot, rfc6962Leaf(leaves[0]))
	if forgedRoot == *b.LogRoot {
		t.Fatal("fixture VACUOUS: the forged LogRoot equals the honest one")
	}

	// PRE-CONDITION: the forgery really is one — it passes the shipped verifier at the claimed m.
	forgedCons := []ports.Hash{rfc6962Leaf(leaves[0])}
	forgedIncl := [][]ports.Hash{{parentRoot}}
	if !verifyLogExtension(parentRoot, 1, forgedRoot, leaves, forgedCons, forgedIncl) {
		t.Fatal("CONTROL BROKEN: the m = 1 right-spine forgery must pass verifyLogExtension at the " +
			"CLAIMED m, or this gate proves nothing about the authentication step")
	}

	forged := b
	forged.LogRoot = &forgedRoot
	src := &forgingSource{
		proverSource: newProverSource(t, f.c),
		forgedSize:   statehash.EncodeUint64(1),
		forgedCons:   forgedCons,
		forgedIncl:   forgedIncl,
	}
	pv := f.provenViewOver(t, src)
	out, err := pv.CommittedRoots(&forged)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogSizeUnauthenticated) {
		t.Fatalf("WRONG-ACCEPT: the degenerate m = 1 forgery must be refused at AUTHENTICATION with "+
			"ErrRevLogSizeUnauthenticated; got %s / %v", out, err)
	}

	// NON-VACUITY: the SAME source, lying about nothing, passes P13b on the honest block. Without
	// this the arm above could be refusing for any unrelated reason.
	honestSrc := &forgingSource{proverSource: newProverSource(t, f.c)}
	if out, err := f.provenViewOver(t, honestSrc).CommittedRoots(&b); out != IndeterminateTrustlessly || !errors.Is(err, ErrRecomputeGated) {
		t.Fatalf("NON-VACUITY BROKEN: the same wrapper serving the honest size must pass P13b; got %s / %v", out, err)
	}
}

// TestLogExtensionRefusesTheZeroSizeClaim is the OTHER degeneracy the certification names:
// VerifyConsistency at m == 0 returns true without reading either root, so an attacker who could
// make the box believe the parent log is empty gets the consistency leg for free.
//
// MEASURED, AND IT SHARPENS THE CLAIM (2026-09-11 ablation): m = 0 alone is NOT a break. With the
// Resolve removed, the m = 1 forgery passes P13b outright but the m = 0 claim still fails — at
// n = k the per-leaf inclusion legs pin the whole tree to MTH(leaves), and the block's real LogRoot
// is not that. So the two legs are not belt-and-braces: the consistency leg binds the PREFIX and
// the inclusion legs bind the CONTENT, and m = 0 merely discards the first. The arm is kept
// because m = 0 is what a box would compute if it read an ABSENT leaf as "empty log" — which is
// why C-a requires the leaf on every v5 root and why logExtends treats ProvenAbsent as a stall.
func TestLogExtensionRefusesTheZeroSizeClaim(t *testing.T) {
	f, b := revLogExtensionFixture(t)
	src := &forgingSource{proverSource: newProverSource(t, f.c), forgedSize: statehash.EncodeUint64(0)}
	out, err := f.provenViewOver(t, src).CommittedRoots(&b)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogSizeUnauthenticated) {
		t.Fatalf("a claimed parent log size of 0 must be refused at authentication; got %s / %v", out, err)
	}
	// And a mis-encoded value is refused by the same sentinel rather than coerced to a number.
	short := &forgingSource{proverSource: newProverSource(t, f.c), forgedSize: []byte{0x03}}
	if out, err := f.provenViewOver(t, short).CommittedRoots(&b); out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogSizeUnauthenticated) {
		t.Fatalf("a 1-byte committed size must be refused, never coerced; got %s / %v", out, err)
	}
}

// TestLogExtensionStallsOnAForgedLogRootAndAMissingProof covers the arms where m IS authenticated:
// the block's LogRoot is wrong, or the source will not serve proofs. Both are STALLS, not Rejects
// — the verification has one error channel and two causes (a bad witness, a wrong root) and the
// box cannot tell them apart, so it never renders a witness gap as a disproof.
func TestLogExtensionStallsOnAForgedLogRootAndAMissingProof(t *testing.T) {
	f, b := revLogExtensionFixture(t)

	// (1) a forged LogRoot with an entirely honest source.
	forged := b
	bad := *b.LogRoot
	bad[0] ^= 0xFF
	forged.LogRoot = &bad
	out, err := f.provenViewOver(t, newProverSource(t, f.c)).CommittedRoots(&forged)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogExtensionUnproven) {
		t.Fatalf("a forged LogRoot on a revocation-bearing block must STALL with "+
			"ErrRevLogExtensionUnproven (never Reject, never Accept); got %s / %v", out, err)
	}

	// (2) the source has no extension proof.
	refusing := &forgingSource{proverSource: newProverSource(t, f.c), refuse: true}
	if out, err := f.provenViewOver(t, refusing).CommittedRoots(&b); out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogExtensionUnproven) {
		t.Fatalf("a source with no extension proof must STALL with ErrRevLogExtensionUnproven; got %s / %v", out, err)
	}

	// (3) the right number of proofs, all garbage.
	junk := &forgingSource{
		proverSource: newProverSource(t, f.c),
		forgedCons:   []ports.Hash{{0x01}},
		forgedIncl:   [][]ports.Hash{{{0x02}}},
	}
	if out, err := f.provenViewOver(t, junk).CommittedRoots(&b); out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogExtensionUnproven) {
		t.Fatalf("garbage proofs must STALL with ErrRevLogExtensionUnproven; got %s / %v", out, err)
	}
}
