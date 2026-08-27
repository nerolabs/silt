package chain

import (
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// RED home #1 (part 3) — per-field ORDER-independence, mandated by the #597
// certification.
//
// The certification refined its own round-9 mandate after RED home #1 found
// `revLog`:
//
//	Replace "the tree is history-independent" with a per-field property: every
//	committed field must be RECONSTRUCTIBLE from the snapshot … Strengthen RED
//	home #1 to VARY APPEND ORDER, not just classify field presence …
//	classification ≠ order-checking. Order-dependence only surfaces when the
//	oracle varies order; presence-classification alone would have missed a
//	purely order-derived leaf.
//
// That is the gap this file closes. Parts 1 and 2 prove the enumeration cannot
// drift and that each enumerated field is load-bearing — but both are blind to
// a field that is *present, populated, and load-bearing* while being derived
// from the ORDER of history rather than from a set. Such a field breaks the
// SMT's history-independence premise, which is the single argument the
// certification's Q1 used to choose the SMT at all.
//
// The test is symmetric, and both directions are findings:
//
//   - a `committedSet` field that VARIES with order is misclassified — it
//     cannot live in the history-independent SMT, and a snapshot-booted node
//     would diverge from a replay-booted one. This is the #597 class.
//   - a `committedLog` field that does NOT vary with order is also
//     misclassified — it is really set-valued, so it belongs in the SMT and
//     does not need its own append-only root.

// twoOrderings commits the same set of events in two different orders and
// returns both chains. Final set-valued state is identical by construction:
// the same two roots are published, and both end up revoked. Only the ORDER of
// the revocation events differs.
func twoOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()

	build := func(revokeFirst, revokeSecond ports.Entry) *Chain {
		c, keys, g := roundsWorld(t)

		// Height 1: publish both roots (same set in both orderings).
		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
			Entries: []ports.Entry{revokeFirst, revokeSecond}}
		commitRounds(b1, keys, 0)
		if err := c.Append(*b1); err != nil {
			t.Fatalf("publish both roots: %v", err)
		}

		// Heights 2 and 3: revoke them, in the order this ordering dictates.
		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Revocations: []ports.Hash{revokeFirst.Root}}
		commitRounds(b2, keys, 0)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("revoke first: %v", err)
		}

		b3 := &Block{Version: BlockVersionRounds, Height: 3, Prev: b2.Hash(),
			Revocations: []ports.Hash{revokeSecond.Root}}
		commitRounds(b3, keys, 0)
		if err := c.Append(*b3); err != nil {
			t.Fatalf("revoke second: %v", err)
		}
		return c
	}

	x, y := entry(31), entry(32)
	return build(x, y), build(y, x) // same events, opposite order
}

// TestCommittedSetFieldsAreOrderIndependent is the load-bearing half. Every
// field classified `committedSet` goes under the history-independent SMT, so
// two histories reaching the same final set MUST agree on it exactly.
func TestCommittedSetFieldsAreOrderIndependent(t *testing.T) {
	a, b := twoOrderings(t)

	fields := fieldsOfKind(committedSet)
	if len(fields) == 0 {
		t.Fatal("no committedSet fields — the classification or reflection is broken")
	}

	var orderDependent []string
	for _, name := range fields {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			orderDependent = append(orderDependent, name)
		}
	}
	if len(orderDependent) > 0 {
		t.Fatalf("%d field(s) classified `committedSet` DIFFER between two histories "+
			"that reach the same final state: %v\n\n"+
			"These cannot live in the history-independent SMT. The certification's "+
			"Q1 chose the SMT on exactly one argument — that the root is identical "+
			"however the state was reached, because a snapshot-booted node never "+
			"replayed the history. An order-derived value under that root breaks the "+
			"argument, and a snapshot-booted validator diverges from a replay-booted "+
			"one. Either the field is really an ordered log (reclassify as "+
			"`committedLog`, give it its own append-only root — the #597 resolution), "+
			"or the state it accumulates must be made order-free.",
			len(orderDependent), orderDependent)
	}
	t.Logf("all %d committedSet fields identical across opposite event orderings", len(fields))
}

// TestCommittedLogFieldsAreGenuinelyOrderDependent is the other direction, and
// it is a real assertion rather than a formality: a "log" that turns out to be
// order-INDEPENDENT does not need its own append-only root, and carrying one
// costs a separate header field and a full entry list in every snapshot for
// nothing. Per the consensus-correctness discipline, an oracle that observes
// something it cannot explain flags rather than assumes-benign.
func TestCommittedLogFieldsAreGenuinelyOrderDependent(t *testing.T) {
	a, b := twoOrderings(t)

	fields := fieldsOfKind(committedLog)
	if len(fields) == 0 {
		t.Skip("no committedLog fields classified")
	}

	for _, name := range fields {
		if reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Errorf("field %q is classified `committedLog` but is IDENTICAL across "+
				"two opposite event orderings.\nIf it does not depend on order it is "+
				"set-valued, so it belongs in the SMT — a separate append-only root "+
				"and a full entry list in every snapshot would be paid for nothing. "+
				"Reclassify it, or explain what order-dependence this history fails "+
				"to exercise.", name)
		}
	}
}

// TestRevLogRootIsOrderDependent is the concrete #597 statement, asserted on
// the published API rather than on the field: the transparency-log ROOT — the
// value era-3 will commit in its own header field — differs between the two
// orderings, while the `revoked` status set does not.
//
// That single pair of facts is the whole certified resolution: the same events
// in different orders yield one identical set and two different log roots, so
// the two must live under two different roots.
func TestRevLogRootIsOrderDependent(t *testing.T) {
	a, b := twoOrderings(t)

	if !reflect.DeepEqual(a.revoked, b.revoked) {
		t.Fatalf("the revoked STATUS set differs across orderings (%v vs %v) — the "+
			"premise of this test is broken", a.revoked, b.revoked)
	}
	if a.RevocationLogSize() != b.RevocationLogSize() {
		t.Fatalf("log sizes differ (%d vs %d); the two histories should log the same "+
			"number of events", a.RevocationLogSize(), b.RevocationLogSize())
	}

	ra, rb := a.RevocationLogRoot(), b.RevocationLogRoot()
	if ra == rb {
		t.Fatal("the revocation-log roots are EQUAL across two opposite orderings.\n" +
			"If the log root is order-independent, #597's resolution is wrong and " +
			"revLog could simply be an SMT leaf. Re-derive before changing the " +
			"classification — translog.Root() is the RFC-6962 MTH over an ordered " +
			"slice (translog.go:54/:106), so this should not happen.")
	}
	t.Logf("same revoked set (%d roots), different log roots: %x… vs %x… — "+
		"two kinds of committed data, two roots", len(a.revoked), ra[:6], rb[:6])
}
