package chain

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// tagRevLogSize — freeze-manifest item 1, the SAFETY leaf. The driven gates.
// =============================================================================
//
// Certification: ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md section 4.1,
// ratified 2026-09-07 (freeze manifest section 8, owner call 9).
// Deliberation: docs/thinking/2026-09-11-tagrevlogsize-freeze-manifest-item-1.md
//
// WHAT THE LEAF IS FOR. The floor box verifies a revocation-bearing block's LogRoot
// verify-not-recompute: a consistency proof from the parent's log root at size m to the block's
// LogRoot at size m+k. `m` is a field of no block and a value of no committed root, and it is not
// recoverable from committed state (apply() DELETES from `revoked` on an un-revocation, so
// |revoked| != len(revLog)). A witness-supplied m is a WRONG-ACCEPT, not a stall:
// TestGD9_WitnessSuppliedLogSizeIsUnsound_Control (redteam_revlog_size_control_v5_test.go) drives
// the degenerate m = 1 right-spine forgery and asserts it PASSES translog.VerifyConsistency. This
// leaf is what makes m non-forgeable.
//
// THE THREE CERTIFIED BUILD CONDITIONS, one gate each:
//   C-a ALWAYS-EMIT     -> TestRevLogSizeLeafIsAlwaysEmitted (an EMPTY log commits EncodeUint64(0);
//                          no absent-vs-empty shortcut).
//   C-b POST-APPLY      -> TestRevLogSizeIsThePostApplyLogSize (driven on a real chain: the value
//                          Resolved from the PARENT's committed StateRoot is exactly the m the
//                          honest consistency proof for the child's LogRoot verifies at; an
//                          off-by-one in either direction fails the proof).
//   C-c THE ABLATION    -> the box-side lift, which is where a forged or omitted witness can be
//                          driven through provenView. NOT in this commit: provenView still refuses
//                          every revocation-bearing block by name (ErrRevLogSizeUnauthenticated),
//                          so there is no accept path a forged m could reach. What IS driven here
//                          is the recompute half -- dropping the counter derivation makes the fold
//                          commit size 0 and land on ErrRecomputeStateRootMismatch, a stall.
//
// AND THE FROZEN-FORMAT PAIR, which must be read together:
//   TestRevLogSizeLeafDoesNotMoveTheEra3Root -- a v4 root is invariant to the log size (the leaf is
//     v5-only, so no live-history block hash can move).
//   TestRevLogSizeLeafBindsTheV5Root -- the NON-VACUITY twin: over the SAME pair the v5 root must
//     DIFFER. Without it the first gate is satisfied by a leaf that was never added at all, which
//     is precisely the state of the tree before this change.

// revLogSizeLeafValue returns the value stateRootLeavesV5 emits at the tagRevLogSize scalar key,
// and whether it emitted one at all. It matches by KEY, not by position.
func revLogSizeLeafValue(c *Chain) ([]byte, bool) {
	want := statehash.Key(tagRevLogSize, nil)
	for _, lf := range c.stateRootLeavesV5() {
		if bytes.Equal(lf.Key, want) {
			return lf.Value, true
		}
	}
	return nil, false
}

// revLogSizePair builds two chains that are identical in EVERY committed field except the size of
// the revocation log: `lo` carries populateCommitted's one-entry log, `hi` carries that same entry
// plus one more. Nothing else varies, so a root difference between them is attributable to the log
// size and to nothing else.
func revLogSizePair(t *testing.T) (lo, hi *Chain) {
	t.Helper()
	lo, hi = &Chain{}, &Chain{}
	populateCommitted(lo)
	populateCommitted(hi)
	if lo.revLog.Size() != hi.revLog.Size() {
		t.Fatalf("fixture: populateCommitted is not deterministic (%d != %d)", lo.revLog.Size(), hi.revLog.Size())
	}
	hi.revLog.Append(RevocationLeaf(UnrevOp, ports.Hash{0xBE, 0xEF}, 9))
	if hi.revLog.Size() != lo.revLog.Size()+1 {
		t.Fatalf("fixture: want hi = lo + 1 log entries, got %d vs %d", hi.revLog.Size(), lo.revLog.Size())
	}
	return lo, hi
}

// TestRevLogSizeLeafIsAlwaysEmitted is condition C-a. The leaf is emitted on EVERY v5 root,
// including when the log is EMPTY, where it commits EncodeUint64(0). An absent-vs-empty shortcut
// would leave a box that reads absence unable to tell "empty log" from "no witness" -- the R4
// accessor defect the five digest roots already pay C-4 to avoid.
//
// The empty arm is built by RESETTING populateCommitted's log, so the gate does not depend on the
// shared fixture happening to leave it empty.
func TestRevLogSizeLeafIsAlwaysEmitted(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{{"empty log", 0}, {"one entry", 1}, {"seventeen entries", 17}} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Chain{}
			populateCommitted(c)
			c.revLog = translog.New()
			for i := 0; i < tc.size; i++ {
				c.revLog.Append(RevocationLeaf(RevOp, ports.Hash{byte(i)}, uint64(i)))
			}
			v, ok := revLogSizeLeafValue(c)
			if !ok {
				t.Fatalf("C-a VIOLATED: stateRootLeavesV5 emitted NO leaf at %q for a log of size %d. "+
					"The leaf is unconditional -- an empty log commits EncodeUint64(0), never an absent leaf.",
					tagRevLogSize, tc.size)
			}
			if want := statehash.EncodeUint64(uint64(tc.size)); !bytes.Equal(v, want) {
				t.Fatalf("C-a VIOLATED: tagRevLogSize committed %x, want %x (log size %d)", v, want, tc.size)
			}
		})
	}
}

// TestRevLogSizeLeafBindsTheV5Root is the NON-VACUITY twin of the frozen-format gate below, and it
// is the gate that fails on the tree as it stood before this leaf landed: two states differing ONLY
// in the size of the revocation log committed the SAME v5 state root, so nothing in committed state
// pinned m.
func TestRevLogSizeLeafBindsTheV5Root(t *testing.T) {
	lo, hi := revLogSizePair(t)
	loRoot, err := lo.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatal(err)
	}
	hiRoot, err := hi.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatal(err)
	}
	if loRoot == hiRoot {
		t.Fatalf("the v5 state root does NOT bind the revocation-log size: two states differing only "+
			"in len(revLog) (%d vs %d) commit the same root %x. m is then witness-supplied, and a "+
			"witness-supplied m is a WRONG-ACCEPT (TestGD9_WitnessSuppliedLogSizeIsUnsound_Control).",
			lo.revLog.Size(), hi.revLog.Size(), loRoot)
	}
}

// TestRevLogSizeLeafDoesNotMoveTheEra3Root is the frozen-format gate: the leaf is v5-only, so an
// era-3 (v4) root must be INVARIANT to the revocation-log size, on both the era-3 entry point
// (StateRoot) and the era-gated one (StateRootForVersion below BlockVersionWitnessable).
//
// This is the whole no-live-history-moves argument at the root level. The block level is closed by
// construction: this change adds no Block field and no cbor key, so Hash() is byte-identical for
// every block of every version. Ablation (demonstrated): move the add(tagRevLogSize, ...) call from
// stateRootLeavesV5 into stateRootLeaves and this gate goes red on all four versions.
func TestRevLogSizeLeafDoesNotMoveTheEra3Root(t *testing.T) {
	lo, hi := revLogSizePair(t)
	loEra3, err := lo.StateRoot()
	if err != nil {
		t.Fatal(err)
	}
	hiEra3, err := hi.StateRoot()
	if err != nil {
		t.Fatal(err)
	}
	if loEra3 != hiEra3 {
		t.Fatalf("FROZEN FORMAT BROKEN: the era-3 root moved with the revocation-log size (%x != %x). "+
			"tagRevLogSize leaked into stateRootLeaves; every deployed v2/v4 node diverges.", loEra3, hiEra3)
	}
	for _, v := range []uint64{1, BlockVersionRounds, BlockVersionRegGate, BlockVersionStateRoot} {
		a, err := lo.StateRootForVersion(v)
		if err != nil {
			t.Fatal(err)
		}
		b, err := hi.StateRootForVersion(v)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("FROZEN FORMAT BROKEN: StateRootForVersion(%d) moved with the revocation-log size (%x != %x)", v, a, b)
		}
		if a != loEra3 {
			t.Fatalf("StateRootForVersion(%d) = %x but StateRoot() = %x -- the era gate no longer routes "+
				"pre-v5 versions through the era-3 leaf set", v, a, loEra3)
		}
	}
}

// TestRevLogSizeIsThePostApplyLogSize is condition C-b, DRIVEN rather than asserted.
//
// It builds a real v5 chain, commits a block that appends THREE revocation-log entries, then commits
// a child that appends ONE more. It then does exactly what the floor box does: Resolve the
// tagRevLogSize leaf against the PARENT's committed StateRoot (a real statehash proof, not a slice
// lookup) to obtain m, and check the honest RFC-6962 consistency proof from the parent's LogRoot at
// size m to the child's committed LogRoot at size m+k.
//
// That is what makes the position claim testable instead of tautological: if the leaf committed the
// PRE-apply size the parent would resolve to 0 and the proof would not fold; if it committed
// m_true+1 it would not fold either. The parent log size is 3 -- deliberately NOT a power of two, so
// the honest path exercises VerifyConsistency's general branch rather than its isPow2 seeding.
func TestRevLogSizeIsThePostApplyLogSize(t *testing.T) {
	f := buildStructFixture(t)

	// h2: three more committed entries, so h3 has roots to take down.
	b2 := f.mkBlock(t, func(b *Block) {
		b.Entries = []ports.Entry{entry(50), entry(51), entry(52)}
	})
	if err := f.c.Append(b2); err != nil {
		t.Fatalf("h2 must commit: %v", err)
	}
	// h3: THREE revocations -> the parent log of the block under test has size 3.
	b3 := f.mkBlock(t, func(b *Block) {
		b.Revocations = []ports.Hash{entry(0).Root, entry(1).Root, entry(50).Root}
	})
	if err := f.c.Append(b3); err != nil {
		t.Fatalf("h3 must commit: %v", err)
	}
	if b3.StateRoot == nil || b3.LogRoot == nil {
		t.Fatal("fixture: the parent block carries no committed roots")
	}
	if got := f.c.revLog.Size(); got != 3 {
		t.Fatalf("fixture: want a 3-entry parent log (not a power of two), got %d", got)
	}

	// AUTHENTICATE m the way the box does: a statehash proof Resolved against the PARENT's
	// committed StateRoot. A value read out of a leaf slice would prove nothing about the root.
	// The chain is AT the parent state here -- b4 has not been appended yet.
	pr, err := statehash.NewProver(f.c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	key := statehash.Key(tagRevLogSize, nil)
	wit, err := pr.Prove(key)
	if err != nil {
		t.Fatalf("Prove(tagRevLogSize): %v", err)
	}
	raw, _ := revLogSizeLeafValue(f.c)
	res := statehash.Resolve(*b3.StateRoot, key, raw, wit)
	if !res.IsProvenPresent() {
		t.Fatalf("C-b VIOLATED: tagRevLogSize does not Resolve as PRESENT against the parent's committed "+
			"StateRoot %x -- the box has no authenticated m", *b3.StateRoot)
	}
	m, ok := decodeUint64Leaf(res.Value())
	if !ok {
		t.Fatalf("C-b VIOLATED: the committed tagRevLogSize value %x is not an 8-byte uint64", res.Value())
	}
	if m != 3 {
		t.Fatalf("C-b VIOLATED: the parent commits log size %d, want 3 (POST-apply: h3 appended three "+
			"revocation-log entries). A pre-apply value would be 0 and mis-size every proof.", m)
	}

	// h4, the block under test: ONE more revocation-log entry (k = 1).
	b4 := f.mkBlock(t, func(b *Block) {
		b.Revocations = []ports.Hash{entry(51).Root}
	})
	if err := f.c.Append(b4); err != nil {
		t.Fatalf("h4 must commit: %v", err)
	}
	k := len(b4.Revocations) + len(b4.Unrevocations)

	// THE DRIVEN HALF: the honest consistency proof from (parent.LogRoot, m) to (b4.LogRoot, m+k)
	// must verify at exactly this m, and at no neighbouring one.
	cons, err := f.c.revLog.ConsistencyProof(int(m), int(m)+k)
	if err != nil {
		t.Fatalf("ConsistencyProof(%d, %d): %v", m, int(m)+k, err)
	}
	if !translog.VerifyConsistency(*b3.LogRoot, int(m), *b4.LogRoot, int(m)+k, cons) {
		t.Fatalf("C-b VIOLATED: the honest log extension does NOT verify at the committed m = %d "+
			"(parent %x -> child %x, k = %d)", m, *b3.LogRoot, *b4.LogRoot, k)
	}
	for _, bad := range []int{int(m) - 1, int(m) + 1} {
		badCons, cErr := f.c.revLog.ConsistencyProof(bad, bad+k)
		if cErr != nil {
			continue
		}
		if translog.VerifyConsistency(*b3.LogRoot, bad, *b4.LogRoot, bad+k, badCons) {
			t.Fatalf("the gate is VACUOUS: the extension also verifies at m = %d, so it does not "+
				"discriminate the committed size", bad)
		}
	}
}
