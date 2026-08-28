package chain

import (
	"crypto/ed25519"
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/nerolabs/silt/ports"
)

// era-3 build step 2b — the v4 validity predicate model-check tier. These oracles prove
// the four properties the 2a ruling's "What 2b MUST carry" requires, and NO more:
//
//  1. a CORRECT v4 block (roots = post-apply recompute) is ACCEPTED;
//  2. a v4 block with a WRONG StateRoot or LogRoot is REJECTED (named errors);
//  3. a v4 block with a NIL StateRoot or LogRoot is REJECTED explicitly (ErrEra3RootMissing);
//  4. a v2/v3 block is UNAFFECTED — the predicate does not fire (era-gating).
//
// Plus the dry-run clone drift guard (postApplyRoots must not silently forget a
// committed field). Each test names the RED that keeps its green honest. STOP boundary
// (2b): the predicate ADDS a v4-gated rejection only; nothing mints v4 (2c) here — the
// v4 blocks are built by the test, never by a propose path. Deliberation:
// docs/thinking/2026-08-29-era3-step2b-validity-predicate.md.

// era3ValidityChain builds an objective launch-phase chain whose proposer (prop) is a
// bonded launch anchor, so a block it proposes passes ValidateProposal's era-2 checks
// and reaches the era-3 predicate. It commits one real block first so the committed
// state is non-trivial (byRoot/spent/bonded/revLog all populated), then returns the
// chain positioned at its head, plus the proposer key.
func era3ValidityChain(t *testing.T) (*Chain, ed25519.PrivateKey) {
	t.Helper()
	prop := key(30201)
	att1, att2, att3 := key(30202), key(30203), key(30204)
	cfg := Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: map[ports.NodeID]bool{
			idOf(prop): true, idOf(att1): true, idOf(att2): true, idOf(att3): true,
		},
		AnchorQuorum: 1,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(att1, twoMiB, ports.Hash{}),
		bondReg(att2, twoMiB, ports.Hash{}),
		bondReg(att3, twoMiB, ports.Hash{}),
	)
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	// Commit one real block so committed state is non-trivial (and includes a
	// revocation, so LogRoot is non-empty and the dry-run apply exercises revLog).
	prev, next := c.Head()
	b := &Block{Version: 1, Height: next, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b, prop)
	b.Atts = append(b.Atts, Attest(b, att1), Attest(b, att2), Attest(b, att3))
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit block 1: %v", err)
	}
	// A second block that REVOKES the first block's root, so revLog is non-empty at head.
	prev, next = c.Head()
	rev := &Block{Version: 1, Height: next, Prev: prev, Revocations: []ports.Hash{entry(1).Root}}
	Sign(rev, prop)
	rev.Atts = append(rev.Atts, Attest(rev, att1), Attest(rev, att2), Attest(rev, att3))
	if err := c.Append(*rev); err != nil {
		t.Fatalf("commit revocation block: %v", err)
	}
	return c, prop
}

// buildHonestV4 builds a v4 block at the chain head carrying an entry, signed by prop,
// with roots set to the HONEST post-apply recompute (what a correct proposer computes).
// Roots are set BEFORE signing so the signature covers them.
func buildHonestV4(t *testing.T, c *Chain, prop ed25519.PrivateKey) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{
		Version: BlockVersionStateRoot,
		Height:  next,
		Prev:    prev,
		Entries: []ports.Entry{entry(9)},
	}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot = &state
	b.LogRoot = &log
	Sign(b, prop)
	return b
}

// TestEra3ValidV4BlockAccepted — a v4 block whose roots equal the post-apply recompute
// passes ValidateProposal.
//
// The block carries an entry, so its POST-apply committed state (entry in byRoot)
// differs from the chain's PRE-apply state — the test asserts that gap first. The
// honest roots are computed INDEPENDENTLY of the predicate's postApplyRoots (a fresh
// clone-apply-read here), so a predicate that recomputed PRE-apply would produce a
// DIFFERENT root than the committed one and reject the block. RED (demonstrated): drop
// scratch.apply(b) in postApplyRoots — the predicate then compares the committed
// (post-apply) root against a pre-apply recompute and this ACCEPT fails.
func TestEra3ValidV4BlockAccepted(t *testing.T) {
	c, prop := era3ValidityChain(t)

	prev, next := c.Head()
	b := &Block{Version: BlockVersionStateRoot, Height: next, Prev: prev, Entries: []ports.Entry{entry(9)}}

	// Independent honest recompute: clone the live chain, apply b, read the roots. This
	// does NOT call postApplyRoots, so it is a genuine second computation of the same
	// property the predicate checks.
	scratch := c.cloneForDryRun()
	scratch.apply(*b)
	honestState, err := scratch.StateRoot()
	if err != nil {
		t.Fatalf("independent StateRoot: %v", err)
	}
	honestLog := scratch.LogRoot()

	// The post-apply state MUST differ from the pre-apply state — b publishes an entry,
	// so its root joins byRoot. This is what makes the timing observable: a pre-apply
	// recompute would yield the chain's CURRENT StateRoot, not honestState.
	preState, err := c.StateRoot()
	if err != nil {
		t.Fatalf("pre-apply StateRoot: %v", err)
	}
	if honestState == preState {
		t.Fatal("post-apply state equals pre-apply state — the block's entry did not " +
			"change committed state, so this test cannot distinguish pre- from post-apply timing")
	}

	b.StateRoot = &honestState
	b.LogRoot = &honestLog
	Sign(b, prop)

	if err := c.ValidateProposal(b); err != nil {
		t.Fatalf("honest v4 block (roots = post-apply recompute) rejected, want accept: %v", err)
	}
}

// TestEra3WrongStateRootRejected — a v4 block with a perturbed StateRoot is rejected
// with ErrEra3StateRootMismatch. RED: remove the StateRoot comparison in
// validateEra3Roots and the wrong-root block is accepted.
func TestEra3WrongStateRootRejected(t *testing.T) {
	c, prop := era3ValidityChain(t)
	b := buildHonestV4(t, c, prop)
	// Perturb the committed StateRoot to a wrong value and re-sign (so the block is
	// otherwise valid — the predicate, not the signature, must catch it).
	wrong := *b.StateRoot
	wrong[0] ^= 0xFF
	b.StateRoot = &wrong
	b.hashMemoSet = false
	Sign(b, prop)
	err := c.ValidateProposal(b)
	if !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("wrong StateRoot: got %v, want ErrEra3StateRootMismatch", err)
	}
}

// TestEra3WrongLogRootRejected — a v4 block with a perturbed LogRoot is rejected with
// ErrEra3LogRootMismatch. RED: remove the LogRoot comparison and it is accepted.
func TestEra3WrongLogRootRejected(t *testing.T) {
	c, prop := era3ValidityChain(t)
	b := buildHonestV4(t, c, prop)
	wrong := *b.LogRoot
	wrong[0] ^= 0xFF
	b.LogRoot = &wrong
	b.hashMemoSet = false
	Sign(b, prop)
	err := c.ValidateProposal(b)
	if !errors.Is(err, ErrEra3LogRootMismatch) {
		t.Fatalf("wrong LogRoot: got %v, want ErrEra3LogRootMismatch", err)
	}
}

// TestEra3NilStateRootRejected — a v4 block with a nil StateRoot is rejected explicitly
// with ErrEra3RootMissing (the 2a-omitempty carry-forward, ruling MUST 1). RED: rely on
// the equality check alone — the nil would still reject, but not with the named
// missing-root error, and not before the recompute; this test pins the explicit reject.
func TestEra3NilStateRootRejected(t *testing.T) {
	c, prop := era3ValidityChain(t)
	b := buildHonestV4(t, c, prop)
	b.StateRoot = nil
	b.hashMemoSet = false
	Sign(b, prop)
	err := c.ValidateProposal(b)
	if !errors.Is(err, ErrEra3RootMissing) {
		t.Fatalf("nil StateRoot: got %v, want ErrEra3RootMissing", err)
	}
}

// TestEra3NilLogRootRejected — a v4 block with a nil LogRoot is rejected explicitly.
func TestEra3NilLogRootRejected(t *testing.T) {
	c, prop := era3ValidityChain(t)
	b := buildHonestV4(t, c, prop)
	b.LogRoot = nil
	b.hashMemoSet = false
	Sign(b, prop)
	err := c.ValidateProposal(b)
	if !errors.Is(err, ErrEra3RootMissing) {
		t.Fatalf("nil LogRoot: got %v, want ErrEra3RootMissing", err)
	}
}

// TestEra3PredicateDoesNotFireForV2 — the era-gating. A valid v2 block still passes
// ValidateProposal unchanged, AND a v2 block carrying a set-but-arbitrary StateRoot
// pointer is STILL accepted (era-2 does not read roots). RED: drop the
// `Version < BlockVersionStateRoot` gate in validateEra3Roots and the second assertion
// fails — the predicate would recompute-and-compare a v2 block's arbitrary root and
// reject it, changing an era-2 verdict.
func TestEra3PredicateDoesNotFireForV2(t *testing.T) {
	c, prop := era3ValidityChain(t)

	// A normal v2 block (no roots) validates exactly as before.
	prev, next := c.Head()
	v2 := &Block{Version: BlockVersionRounds, Height: next, Prev: prev, Entries: []ports.Entry{entry(9)}}
	Sign(v2, prop)
	if err := c.ValidateProposal(v2); err != nil {
		t.Fatalf("valid v2 block rejected by the era-3 predicate — the version gate is wrong: %v", err)
	}

	// A v2 block carrying an arbitrary (wrong-for-any-state) StateRoot pointer must
	// STILL be accepted: era-2 does not read roots. If the predicate fired regardless
	// of version, this arbitrary root would fail the recompute and be rejected.
	arb := ports.Hash{0xAB, 0xCD}
	v2root := &Block{Version: BlockVersionRounds, Height: next, Prev: prev, Entries: []ports.Entry{entry(9)}, StateRoot: &arb}
	Sign(v2root, prop)
	if err := c.ValidateProposal(v2root); err != nil {
		t.Fatalf("v2 block with a set StateRoot rejected — the predicate fired for a sub-v4 block: %v", err)
	}
}

// TestEra3PredicateRidesThroughValidateCommit — the commit path carries the check via
// ValidateProposal (there is one root-check site). A commit of a wrong-StateRoot v4
// block is rejected by ValidateCommit too.
func TestEra3PredicateRidesThroughValidateCommit(t *testing.T) {
	c, prop := era3ValidityChain(t)
	att1, att2, att3 := key(30202), key(30203), key(30204)
	b := buildHonestV4(t, c, prop)
	wrong := *b.StateRoot
	wrong[0] ^= 0xFF
	b.StateRoot = &wrong
	b.hashMemoSet = false
	Sign(b, prop)
	b.Atts = append(b.Atts, Attest(b, att1), Attest(b, att2), Attest(b, att3))
	if err := c.ValidateCommit(b); !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("ValidateCommit did not carry the root check: got %v, want ErrEra3StateRootMismatch", err)
	}
}

// TestDryRunCloneCopiesEveryAppliedField is the drift guard for postApplyRoots'
// cloneForDryRun. A committed/log/observable field the clone forgets would make the
// dry-run apply diverge and the recompute silently wrong — the #558 class. Reflection
// makes the guard total: it asserts that cloneForDryRun copies every field the
// classification calls history-derived (committedSet | committedLog | observable) to a
// value equal to the source, and that the clone is a DISTINCT object per reference field
// (so apply() on the clone cannot mutate the live chain).
//
// RED: drop any map copy in cloneForDryRun (e.g. `s.bonded = c.bonded` by reference, or
// omit a field entirely) and either the equality or the distinctness assertion fails.
func TestDryRunCloneCopiesEveryAppliedField(t *testing.T) {
	src := &Chain{}
	populateCommitted(src)
	// populateCommitted leaves cfg/rep unset; give the clone a config so apply()'s
	// callees behave, though this test only inspects the copied history-derived fields.
	src.cfg = DefaultConfig()

	clone := src.cloneForDryRun()

	for _, name := range historyDerived(t) {
		sv := dryRunFieldValue(src, name)
		cv := dryRunFieldValue(clone, name)

		// Value equality: the clone must start from the same committed state.
		if !reflect.DeepEqual(sv, cv) {
			t.Errorf("field %q: clone value %v != source %v — cloneForDryRun did not copy it", name, cv, sv)
			continue
		}
		// Distinctness for reference types: mutating the clone must not touch the source.
		// A non-empty map/slice with a shared backing array is the bug — apply() on the
		// clone would mutate live state. (A nil/empty ref has no backing to share.)
		rv := reflect.ValueOf(sv)
		switch rv.Kind() {
		case reflect.Map, reflect.Slice, reflect.Ptr:
			if rv.IsNil() {
				continue
			}
			if (rv.Kind() != reflect.Ptr) && rv.Len() == 0 {
				continue // empty map/slice: no shared backing to worry about
			}
			if reflect.ValueOf(cv).Pointer() == rv.Pointer() {
				t.Errorf("field %q: clone shares the SAME underlying %s as the source — "+
					"apply() on the clone would mutate live state (the bug this dry run avoids)",
					name, rv.Kind())
			}
		}
	}
}

// dryRunFieldValue reads an unexported field (the same reflect trick the completeness
// guard uses). Confined to this test.
func dryRunFieldValue(c *Chain, name string) any {
	f := reflect.ValueOf(c).Elem().FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface()
}
