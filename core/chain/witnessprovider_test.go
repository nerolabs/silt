package chain

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The serving half of the witness seam has to answer exactly what the box's own gates have
// been checked against. Those gates run on a prover-backed source built inside the test
// package; the production provider reads a live chain. If the two ever disagree, every gate
// that passed against the double is evidence about a source no deployment uses, and the box
// would be exercised in tests by one server and in the field by another.
//
// So the property asserted here is EQUIVALENCE, key by key and tag by tag, and then end to
// end through the door. It is what lets the existing box gates carry over to the real server.

// TestWitnessProviderAnswersWhatTheGatesWereCheckedAgainst compares the production provider
// against the prover-backed double on every accessor, over the same chain.
func TestWitnessProviderAnswersWhatTheGatesWereCheckedAgainst(t *testing.T) {
	f := buildStructFixture(t)
	double := newProverSource(t, f.c)
	prod, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("NewWitnessProvider over a committed chain must succeed")
	}

	// Every committed leaf: same value, and both must produce a witness. The proofs are
	// compared by the bytes they encode to, which is the quantity that actually crosses a
	// wire — two proofs that encode differently are two different proofs to a reader.
	leaves := f.c.stateRootLeavesV5()
	if len(leaves) == 0 {
		t.Fatal("fixture has no committed v5 leaves — the comparison would be vacuous")
	}
	for _, l := range leaves {
		gotV, gotW, gotOK := prod.Leaf(l.Key)
		wantV, wantW, wantOK := double.Leaf(l.Key)
		if gotOK != wantOK {
			t.Fatalf("Leaf(%x) ok: provider %v, double %v", l.Key, gotOK, wantOK)
		}
		if !bytes.Equal(gotV, wantV) {
			t.Fatalf("Leaf(%x) value: provider %x, double %x", l.Key, gotV, wantV)
		}
		gotB, err := gotW.MarshalBinary()
		if err != nil {
			t.Fatalf("Leaf(%x): marshal provider witness: %v", l.Key, err)
		}
		wantB, err := wantW.MarshalBinary()
		if err != nil {
			t.Fatalf("Leaf(%x): marshal double witness: %v", l.Key, err)
		}
		if !bytes.Equal(gotB, wantB) {
			t.Fatalf("Leaf(%x) proof differs: provider %d bytes, double %d bytes", l.Key, len(gotB), len(wantB))
		}
	}

	// An ABSENT key must still produce a witness — an exclusion proof. A provider that
	// returned "no witness" here would be indistinguishable from an unreachable one, and the
	// reader could not tell a proven absence from a missing answer.
	absent := []byte(tagBondedRoot + "no-such-member-key")
	if _, w, ok := prod.Leaf(absent); !ok || w.IsNil() {
		t.Fatalf("an absent key must yield an exclusion proof, not a missing witness (ok=%v nil=%v)", ok, w.IsNil())
	}

	// The five whole-set keyspaces, in the order the reader re-derives the MTH over. Order is
	// load-bearing, not cosmetic: the MTH is over the list as served, so a provider that
	// returned the same ids in a different order would produce a different root and stall an
	// honest box.
	for _, tag := range []string{tagEpochSetRoot, tagQualifiedRoot, tagBondedRoot, tagValidatorsSeenRoot, tagSlashedRoot} {
		got, gotOK := prod.Members(tag)
		want, wantOK := double.Members(tag)
		if gotOK != wantOK {
			t.Fatalf("Members(%q) ok: provider %v, double %v", tag, gotOK, wantOK)
		}
		if len(got) != len(want) {
			t.Fatalf("Members(%q) length: provider %d, double %d", tag, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("Members(%q) differs at %d: provider %x, double %x", tag, i, got[i], want[i])
			}
		}
	}

	// The ancestor window, at a depth the fixture actually has and at one past it.
	for _, k := range []int{1, 2, 64} {
		got, gotOK := prod.Ancestors(k)
		want, wantOK := double.Ancestors(k)
		if gotOK != wantOK || len(got) != len(want) {
			t.Fatalf("Ancestors(%d): provider (%v,%d), double (%v,%d)", k, gotOK, len(got), wantOK, len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("Ancestors(%d) differs at %d", k, i)
			}
		}
	}

	// The log extension, over a leaf the parent log does not already hold.
	m := f.c.revLog.Size()
	appended := []ports.Hash{{0xA1}, {0xA2}}
	gotCons, gotIncl, gotOK := prod.LogExtension(m, appended)
	wantCons, wantIncl, wantOK := double.LogExtension(m, appended)
	if gotOK != wantOK {
		t.Fatalf("LogExtension ok: provider %v, double %v", gotOK, wantOK)
	}
	if len(gotCons) != len(wantCons) || len(gotIncl) != len(wantIncl) {
		t.Fatalf("LogExtension shape: provider (%d,%d), double (%d,%d)",
			len(gotCons), len(gotIncl), len(wantCons), len(wantIncl))
	}
	for i := range gotCons {
		if gotCons[i] != wantCons[i] {
			t.Fatalf("LogExtension consistency proof differs at %d", i)
		}
	}
}

// TestWitnessProviderDrivesTheDoorLikeTheDouble is the end-to-end half: an honest block must
// reach the SAME door outcome through the production provider as through the double. The
// accessor comparison above could pass while some ordering or copy defect still changed what
// the composition concluded, so the door is asserted directly rather than inferred.
func TestWitnessProviderDrivesTheDoorLikeTheDouble(t *testing.T) {
	f := buildStructFixture(t)
	double := newProverSource(t, f.c)
	prod, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("NewWitnessProvider over a committed chain must succeed")
	}
	b := f.mkBlock(t, nil)
	if err := f.c.ValidateCommit(&b); err != nil {
		t.Fatalf("oracle: the block under test must be valid to the node: %v", err)
	}
	w := structWitnessFor(t, f, double, b)

	// Through the double: the honest block runs the whole composition and lands on the
	// documented downgrade. This is the vacuity guard for the arm below — if the double
	// stopped reaching the downgrade, comparing against it would prove nothing.
	assertBoxReachesTheDowngrade(t, boxOver(t, f, double), b, w)

	// Through the production provider: the same outcome, by the same name.
	outProd, errProd := boxOver(t, f, prod).Validate(b, w)
	outDbl, errDbl := boxOver(t, f, double).Validate(b, w)
	if outProd != outDbl {
		t.Fatalf("the production provider drives the door to a DIFFERENT outcome than the double: provider %s, double %s", outProd, outDbl)
	}
	if (errProd == nil) != (errDbl == nil) || (errProd != nil && errProd.Error() != errDbl.Error()) {
		t.Fatalf("the production provider drives the door to a different reason: provider %v, double %v", errProd, errDbl)
	}
}

// TestWitnessProviderHeadPinsTheStateItServes: a provider is a snapshot. It must report the
// head it was built over, because a box that asked about one head and was answered from
// another would read half its leaves from each and stall on the seam with nothing to name.
func TestWitnessProviderHeadPinsTheStateItServes(t *testing.T) {
	f := buildStructFixture(t)
	p, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("NewWitnessProvider must succeed")
	}
	wantHash, wantNext := f.c.Head()
	gotHash, gotNext := p.Head()
	if gotHash != wantHash || gotNext != wantNext {
		t.Fatalf("provider head %x/%d does not pin the chain head %x/%d", gotHash[:8], gotNext, wantHash[:8], wantNext)
	}

	// The cache hands back the SAME provider while the head is unchanged — rebuilding the
	// leaf set and the prover per request is what would make serving witnesses expensive
	// enough for a stranger to weaponise.
	var pc ProviderCache
	first, ok := pc.For(f.c, ports.Hash{})
	if !ok {
		t.Fatal("ProviderCache.For must succeed over a committed chain")
	}
	again, ok := pc.For(f.c, ports.Hash{})
	if !ok || first != again {
		t.Fatal("ProviderCache must reuse its provider while the head is unchanged")
	}

	// And a caller that NAMES the head it wants gets that one. This is what lets a validation
	// that anchored at one head finish there while the serving chain commits under it: the box
	// names the head on every request, and one snapshot back is retained for exactly that.
	named, ok := pc.For(f.c, wantHash)
	if !ok || named != first {
		t.Fatal("ProviderCache must serve the head a caller names when it still holds it")
	}
	if err := f.c.Append(f.mkBlock(t, nil)); err != nil {
		t.Fatalf("the fixture chain must commit one more block: %v", err)
	}
	moved, ok := pc.For(f.c, ports.Hash{})
	if !ok || moved == first {
		t.Fatal("ProviderCache must rebuild once the chain head has moved")
	}
	back, ok := pc.For(f.c, wantHash)
	if !ok || back != first {
		t.Fatal("ProviderCache must still serve the PREVIOUS head: a box that anchored there is still " +
			"asking about it, and refusing would make it re-anchor and race the chain forever")
	}
	stale, _ := pc.For(f.c, ports.HashBytes([]byte("a head this node never held")))
	if stale != nil && stale.head == ports.HashBytes([]byte("a head this node never held")) {
		t.Fatal("ProviderCache invented a provider for a head it never held")
	}
}
