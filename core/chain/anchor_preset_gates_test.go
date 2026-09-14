package chain

// ADVERSARY — anchoredPreSet does not anchor. The artifact is byte-identical on both
// trees, so the finding is not a function of which of the two
// revisions you read.
//
// Probes preserved verbatim (sha256 recorded) at
//
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE BREAK
// ────────────────────────────────────────────────────────────────────────────────────────
//
// anchoredPreSet (floorbox_recompute_stateroot_slash_v5.go) checks two things: that a witness for
// the tag exists, and that w.Proof is non-nil. It never computes nodeSetMTH(w.PreIDs), never calls
// statehash.Resolve, and takes no prevStateRoot — it has no material with which to anchor anything.
// Anchoring is a SIDE EFFECT of a later digestFoldOp for the same tag, and statehash.FoldChangedPaths
// verifies only the ops it is actually handed. So a tag read through anchoredPreSet whose fold op is
// never emitted has its PreIDs and Proof consumed as attacker-chosen data.
//
// Contrast provenView.members (stateview_proven_v5.go), which does the anchoring UNCONDITIONALLY:
// it Resolves the digest leaf against the root and refuses unless bytes.Equal(nodeSetMTH(ids),
// rootValue). Two readers of the same whole-set digests, one anchored and one not.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE UNANCHORED-READ CENSUS — re-derived here by the research, not taken on report
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Every digestFoldOp call site in the tree (non-test), and the anchoredPreSet read it does or does
// not cover:
//
//	class B (bondreg) — reads bondedRoot, qualifiedRoot, slashedRoot.
//	 EMITS bondedRoot IFF !idSetsEqual(preBonded, delta.postBonded);
//	 EMITS qualifiedRoot IFF !idSetsEqual(preQualified, delta.postQual);
//	 EMITS slashedRoot NEVER — there is no digestFoldOp(tagSlashedRoot, …) in that file at all.
//	 ⇒ slashedRoot is unanchored on EVERY class-B block, and it is the sole carrier of the F2 bar
//	 ("a slashed equivocator cannot re-earn standing") at stateRootBondRegWriteSet's
//	 `if _, isSlashed:= preSlashed[id]` screen.
//	 ⇒ bonded/qualified are unanchored whenever membership is unchanged, AND THE FORGERY IS WHAT
//	 MAKES IT UNCHANGED: delta.post* is derived FROM the forged pre-set, so an attacker who
//	 forges the pre-set steers the very equality that decides whether it gets checked.
//
//	class P (rotate) — reads qualifiedRoot as the epoch-set FREEZE SOURCE
//	 (`post:= cloneIDSet(preQualified)`), and EMITS only epochSetRoot.
//	 ⇒ qualifiedRoot is unanchored on any boundary with no B/T/S class also emitting it.
//
//	class A (atts) — reads validatorsSeenRoot; stateRootAttDigestOp returns (nil, nil) when
//	 idSetsEqual(preValidatorsSeen, postValidatorsSeen).
//	 ⇒ unanchored whenever post == pre, which injecting the attesters into the pre-set guarantees.
//
//	class S (slash) and class T (ttl) EMIT their digest ops unconditionally and are NOT part of
//	 this finding. Recording the safe classes is the point: a census that came back uniform would
//	 be a bug report about the census.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// WHY THE SHIPPED ABLATIONS DID NOT SEE IT — THE ORACLE IS THE WHOLE POINT
// ────────────────────────────────────────────────────────────────────────────────────────
//
// The two shipped tests in this package that forge PreIDs —
// TestRecomputeStateRootRotateAblationShortQualifiedSet and
// TestRecomputeStateRootSlashAblationForgedQualifiedScreen — both oracle on
// `f.applyAndCommittedRoot(t, b)`, the HONEST committed root. Against that oracle the forgery
// stalls at ErrRecomputeStateRootMismatch, and the stall is read as "the pre-set is anchored". It is
// not: the box simply folded to a DIFFERENT root than the honest one it was asked to match.
// forgeDivergentRoot in the cold-auditor tier has the same shape — it XORs one byte of the honest
// root and keeps the honest witness. That is the VANDAL model.
//
// A real proposer is not a vandal. It writes into b.StateRoot whatever root ITS OWN FOLD PRODUCES
// FROM ITS OWN FORGED WITNESS, and the box's terminal equality check then passes by construction.
// Every gate below therefore measures the recompute against rtAnchorForgedFold's output — the root
// the box itself derives from the forged witness — never against the honest root. A4b is the same
// forgery oracled the shipped way, kept as the CONTROL that makes the difference visible.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// WHY THESE ARE PINS AND NOT ASSERTIONS
// ────────────────────────────────────────────────────────────────────────────────────────
//
// The remedy — do inside anchoredPreSet what provenView.members already does — changes a
// consensus-adjacent verification rule, so it is not made here. These land as PINNED_DEFECT
// instead: each asserts CURRENT BROKEN BEHAVIOUR, passes today, and REDDENS the moment the behaviour
// moves — which forces the record to be updated rather than letting the defect close silently.
// t.Skip is refused by that same decision; a skip is a dark test.
//
// The idiom (PIN + TEETH + MECHANISM) is documented in full at the head of
// core/pipeline/subframe_manifest_oracle_test.go. Each pin here carries:
// 1. the PIN — the symptom, via rtAnchorPin: the box returned nil for the adversary's own root;
// 2. a MECHANISM arm — via rtAnchorMechanismPin: NO fold op for the forged tag was emitted, so the
// pre-set went unanchored. A fix that emitted the op but left the symptom, or moved the symptom
// without removing the cause, reddens here rather than passing by coincidence;
// 3. TEETH — TestPinRedensWhenTheBoxRejects, …WhenTheTagIsFolded and …WhenAnchoredPreSetAnchors,
// which feed each predicate its POST-FIX input and assert it speaks up. A pin whose teeth are
// untested has never been shown capable of failing.
//
// CONTAINMENT, so nobody reads these as "main is exploitable today": (*Box).Validate still applies
// the downgrade — Accept ⇒ IndeterminateTrustlessly / ErrRecomputeGated. The box never accepts on
// this path yet. That downgrade is the only barrier between this finding and damage, and it is
// deliberately not touched here. The finding is what happens on the day the gate flips.

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// ══════════════════════════════════════════════════════════════════════════════
// THE PREDICATES — factored so their teeth can be asserted without merging a red test
// ══════════════════════════════════════════════════════════════════════════════

// forgedPreSetMustStall is the symptom predicate: handed the stall the recompute returned
// when measured against the root the box's OWN fold produced from the FORGED witness, it
// reports a failure only if the box ACCEPTED the forgery. Empty string means the attack was
// refused.
//
// Measuring against the box's own fold root rather than the honest one is the whole point.
// An oracle that compares the box's output to what real apply() yields would redden for any
// divergence, including a box that simply computed something else; comparing against the
// root the adversary's own witness produces asks the only question that matters — did the
// box certify the adversary's state as its own?
func forgedPreSetMustStall(attack string, stall error, forged, honest ports.Hash, consequence string) string {
	if stall != nil {
		return ""
	}
	return fmt.Sprintf(
		"%s — THE BOX ACCEPTED A FORGED PRE-STATE SET. It derived %x from the forged witness and\n"+
			"  raised no objection, against the honest %x that real apply() yields.\n"+
			"  The attack: %s\n"+
			"  A whole-set digest commits membership, so a witnessed pre-set must be proven against\n"+
			"  prevStateRoot before it is read — not left to a later fold op that is only emitted when\n"+
			"  the set changes, because the post-set is derived FROM the pre-set and the forgery is\n"+
			"  therefore what decides whether it is ever checked.",
		attack, forged[:], honest[:], consequence)
}

// anchoringMustNotDependOnTheFoldOp asserts the structural reason the remedy holds: the
// forged tag is refused even though the box emits NO fold op for it.
//
// That is what separates anchoring at the read from anchoring as a side effect. A fold op
// verifies its own OldValue, so a tag whose op is emitted was always checked; the tags that
// were not — slashedRoot on a bond registration, and bonded/qualified/validatorsSeen
// whenever the forgery makes pre equal post — were read as attacker-chosen data. If an op
// IS emitted for this tag, the test is no longer observing the unanchored path and proves
// less than it appears to.
func anchoringMustNotDependOnTheFoldOp(attack, tag string, ops []statehash.FoldOp) string {
	want := statehash.Key(tag, nil)
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) {
			return fmt.Sprintf(
				"%s IS VACUOUS — the box emitted a fold op for %q (op[%d], NewValue=%x), so this tag is\n"+
					"  anchored by the fold and the refusal above does not demonstrate anchoring AT THE READ.\n"+
					"  Re-derive which tags are read without an op before trusting this test.",
				attack, rtAnchorTagName(tag), i, ops[i].NewValue)
		}
	}
	return ""
}

// forgedPreSetMustBeRejected is the unit predicate: anchoredPreSet must refuse an id-list
// whose nodeSetMTH is not the committed digest. Empty string means it refused.
func forgedPreSetMustBeRejected(err error, gotMTH, committed []byte) string {
	if bytes.Equal(gotMTH, committed) {
		return fmt.Sprintf(
			"IS VACUOUS — the forged id-list happens to hash to the committed digest\n"+
				"  (nodeSetMTH=%x == committed=%x), so refusing it proves nothing. Change the junk id.",
			gotMTH, committed)
	}
	if err == nil {
		return fmt.Sprintf(
			"anchoredPreSet ACCEPTED a forged pre-set: its id-list hashes to %x, the committed digest\n"+
				"  is %x. The set a whole-set digest commits is the chain's, not the producer's.",
			gotMTH, committed)
	}
	return ""
}

// rtAnchorTagName strips the NUL terminator the tag constants carry, for readable failure text.
func rtAnchorTagName(tag string) string { return string(bytes.TrimRight([]byte(tag), "\x00")) }

// rtAnchorIsRecomputeStall reports whether err is one of the box's OWN recompute refusals, rather
// than some new failure SHAPE. The A4b control asserts this, not merely non-nil: a panic turned into
// an error, or a nil-deref, would satisfy "it stalled" while meaning something quite different.
//
// It does NOT separate a correct refusal from an over-rejecting one: a box that refuses every pre-set
// with ErrRecomputeStateRootDigest passes this predicate, measured. See the A4b docstring.
func rtAnchorIsRecomputeStall(err error) bool {
	return errors.Is(err, ErrRecomputeStateRootDigest) ||
		errors.Is(err, ErrRecomputeStateRootFold) ||
		errors.Is(err, ErrRecomputeStateRootMismatch)
}

// rtAnchorForgedFold returns the post-root the BOX ITSELF computes for (b, w) — precisely the value
// a malicious proposer writes into b.StateRoot so the box's terminal equality check passes — plus
// the op list it assembled. THIS IS THE ORACLE. Measuring against the honest committed root instead
// is what hid this finding for the whole life of the shipped ablations.
//
// An assembly error is returned rather than fataled: after the remedy lands, refusing at assembly is
// one of the shapes the fix can take, and it must reach the pin as a RED rather than as a bare
// t.Fatal with no instruction attached.
//
// IT ASSEMBLES TWICE, ON PURPOSE. assembleStateRootRecomputeOps takes committedStateRoot, and ONE
// class reads it: maturityLatchOps → recomputeMatureNow. The malicious proposer's committed root is
// not known until its own fold produces it, so pass 1 assembles under prevRoot only to LEARN that
// root, and pass 2 re-assembles under it — which is the committedStateRoot recomputeViaHead is then
// handed. Assembling once under prevRoot leaves the op list produced under a different committed
// root than the verification pass consumes; today the two agree (the pins are green, and a
// divergence would surface as a terminal mismatch), but the fixtures are one everMature flip away
// from a red that has nothing to do with anchoring and pin text that sends the reader hunting for a
// remedy that never landed. A divergence is fataled HERE, named as a fixture fact.
func rtAnchorForgedFold(t *testing.T, c *Chain, prevRoot ports.Hash, b Block, w StateRootWitness) (ports.Hash, []statehash.FoldOp, error) {
	t.Helper()
	ops, err := assembleOpsViaHead(c, prevRoot, prevRoot, b, w)
	if err != nil {
		return ports.Hash{}, nil, fmt.Errorf("op assembly refused the forged witness: %w", err)
	}
	r, err := statehash.FoldChangedPaths(prevRoot, ops)
	if err != nil {
		return ports.Hash{}, ops, fmt.Errorf("the fold refused the forged witness: %w", err)
	}
	ops2, err := assembleOpsViaHead(c, prevRoot, r, b, w)
	if err != nil {
		return ports.Hash{}, ops, fmt.Errorf("op assembly refused the forged witness under its own fold root: %w", err)
	}
	r2, err := statehash.FoldChangedPaths(prevRoot, ops2)
	if err != nil {
		return ports.Hash{}, ops2, fmt.Errorf("the fold refused the forged witness under its own fold root: %w", err)
	}
	if r2 != r {
		t.Fatalf("FIXTURE DIVERGENCE, NOT AN ANCHORING CHANGE — assembling the forged op list under prevRoot "+
			"yields root %x, and re-assembling it under THAT root yields %x. Some class now reads\n"+
			"  committedStateRoot in a way that changes the op list (maturityLatchOps is the only such reader\n"+
			"  today, and it returns early when the everMature latch is already set). Do NOT read this as the\n"+
			"  anchoring remedy landing: fix the fixture (or iterate to a fixpoint) first, then re-read the pins.",
			r[:], r2[:])
	}
	return r2, ops2, nil
}

// rtAnchorSetPreIDs overwrites one tag's pre-set id-list in a witness. It fatals when the tag is
// absent: a forgery that silently forged nothing would report GREEN and be indistinguishable from a
// live defect.
func rtAnchorSetPreIDs(t *testing.T, w *StateRootWitness, tag string, ids []ports.NodeID) {
	t.Helper()
	found := 0
	for i := range w.DigestPreSets {
		if w.DigestPreSets[i].Tag == tag {
			w.DigestPreSets[i].PreIDs = ids
			found++
		}
	}
	if found != 1 {
		t.Fatalf("fixture: expected exactly ONE %s pre-set in the witness, found %d — the forgery did not "+
			"land where it was aimed and a green result would be meaningless", rtAnchorTagName(tag), found)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// anchoredPreSet accepts a pre-set whose MTH is NOT the committed digest
// ══════════════════════════════════════════════════════════════════════════════

func TestAForgedPreSetIsRejectedAtTheRead(t *testing.T) {
	f := buildBondFixture(t)
	junk := []ports.NodeID{ports.HashBytes([]byte("not-a-member-at-all"))}
	w := f.digestWitness(t, tagSlashedRoot, junk) // HONEST proof, FORGED PreIDs
	byTag := map[string]*StateRootDigestWitness{tagSlashedRoot: &w}

	set, err := anchoredPreSet(byTag, tagSlashedRoot, f.prevRoot)
	committed := f.preValue(statehash.Key(tagSlashedRoot, nil))
	if msg := forgedPreSetMustBeRejected(err, nodeSetMTH(junk), committed); msg != "" {
		t.Fatal(msg)
	}
	// A refusal must hand back nothing. Returning the forged set alongside an error invites a
	// caller that checks the value and not the error, which is how an unanchored read reaches
	// the state transition in the first place.
	if len(set) != 0 {
		t.Fatalf("anchoredPreSet REFUSED the forged pre-set but still returned %d ids. A refused read "+
			"must yield no set, or a caller that ignores the error consumes attacker-chosen membership "+
			"anyway.", len(set))
	}
	if _, ok := set[junk[0]]; ok {
		t.Fatal("the forged id is present in the set returned alongside a refusal")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// class B, slashedRoot INJECTION erases a bond registration
// ══════════════════════════════════════════════════════════════════════════════

func TestAForgedSlashedPreSetCannotEraseABondReg(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	fresh := key(81009)
	freshID := ports.HashBytes(pubOf(fresh))
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
	newDue := h + f.c.cfg.BondTTLBlocks + 1

	honest := f.applyAndCommittedRoot(t, b) // what a FULL NODE computes
	if honest == f.prevRoot {
		t.Fatalf("VACUOUS FIXTURE: real apply() did not move the state root, so 'the registration vanished' " +
			"would be indistinguishable from 'there was nothing to erase'")
	}

	w := f.bondWitness(t, b, []uint64{newDue})
	rtAnchorSetPreIDs(t, &w, tagSlashedRoot, []ports.NodeID{freshID}) // THE FORGERY: claim the registrant is slashed

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: the forged root equals the honest root %x — the forgery changed nothing", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		fmt.Sprintf("injecting registrant %x into the never-folded slashedRoot pre-set makes the box drop its whole "+
			"bond registration and certify a root in which the registration never happened", freshID[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagSlashedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
	// A refusal must produce no root at all. The attack this closes made the box fold a
	// BondReg-carrying block to a root identical to prevStateRoot — the registration erased,
	// certified as a complete no-op — so a box that refuses but still emits SOME root has only
	// moved the forgery rather than stopped it.
	if forged != (ports.Hash{}) {
		t.Fatalf("the box refused the forged witness but still derived root %x. A refusal must yield no "+
			"root: the attack was a BondReg-carrying block folding to zero net state change, and any "+
			"emitted root leaves that reachable.", forged[:])
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// class B, all three pre-sets forged at once
// ══════════════════════════════════════════════════════════════════════════════

func TestEveryBondRegPreSetIsVerified(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	fresh := key(81009)
	freshID := ports.HashBytes(pubOf(fresh))
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	honest := f.applyAndCommittedRoot(t, b)

	w := f.bondWitness(t, b, []uint64{newDue})
	rtAnchorSetPreIDs(t, &w, tagSlashedRoot, []ports.NodeID{freshID})
	rtAnchorSetPreIDs(t, &w, tagBondedRoot, []ports.NodeID{ports.HashBytes([]byte("garbage-bonded"))})
	rtAnchorSetPreIDs(t, &w, tagQualifiedRoot, nil)

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: forged root == honest root %x", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		"all THREE class-B pre-sets (slashed, bonded, qualified) replaced with garbage and the box still "+
			"admitted — bonded and qualified escape their emission guard because the forgery itself is what "+
			"makes idSetsEqual(pre, post) true"); msg != "" {
		t.Fatal(msg)
	}
	// MECHANISM: none of the three tags was folded. This is the arm that carries the emission-guard
	// half of the finding — bonded/qualified are guarded by !idSetsEqual, and a forged pre-set steers
	// that equality.
	for _, tag := range []string{tagSlashedRoot, tagBondedRoot, tagQualifiedRoot} {
		if msg := anchoringMustNotDependOnTheFoldOp("", tag, ops); msg != "" {
			t.Fatal(msg)
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// class B, qualifiedRoot OMISSION keeps an under-bonded validator qualified
// ══════════════════════════════════════════════════════════════════════════════

func TestAForgedQualifiedPreSetCannotKeepUnderBondedStanding(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	pid := ports.HashBytes(pubOf(f.proposer))
	if _, ok := f.c.qualified[pid]; !ok {
		t.Fatalf("VACUOUS FIXTURE: the proposer is not qualified pre-state, so there is no standing to keep")
	}
	oldDue, ok := f.preDueHeight(pid)
	if !ok {
		t.Fatalf("VACUOUS FIXTURE: the proposer is not registered pre-state")
	}
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	// Resize BELOW MinBond (still >= MinBondBytes, which is 0 in this fixture).
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(f.proposer, ports.HashBytes(pubOf(f.proposer)), 1, prev, 6, 1)}}

	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, still := clone.qualified[pid]; still {
		t.Fatalf("VACUOUS FIXTURE: real apply() did NOT de-qualify the under-bonded proposer, so 'it kept " +
			"standing' is what an honest node does too and the gate asserts nothing")
	}
	honest := f.applyAndCommittedRoot(t, b)

	w := f.bondWitness(t, b, uniqueU64(oldDue, newDue))
	// THE FORGERY: drop the proposer from the un-anchored pre-qualified set, so the box's derived
	// delta has nothing to de-qualify.
	var kept []ports.NodeID
	for _, id := range f.preIDsQualified() {
		if id != pid {
			kept = append(kept, id)
		}
	}
	rtAnchorSetPreIDs(t, &w, tagQualifiedRoot, kept)

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: forged root == honest root %x", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		fmt.Sprintf("dropping %x from the un-anchored pre-qualified set makes the box certify a root in which an "+
			"UNDER-BONDED validator keeps its qualified||id leaf and qualifiedRoot never moves", pid[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagQualifiedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// class P, the boundary FREEZE SOURCE evicts an honest qualified validator
// ══════════════════════════════════════════════════════════════════════════════

func TestAForgedFreezeSourceCannotEvictAQualifiedValidator(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	honest := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	victim := rtAnchorPickVictim(t, f)
	if _, in := f.c.epochSet[victim]; !in {
		t.Fatalf("VACUOUS FIXTURE: the victim is not in the prior epochSet, so there is nothing to evict it from")
	}

	// (1) THE FORGERY: drop the victim from the UN-ANCHORED pre-qualified id-list, which class P
	// reads as the freeze source (`post:= cloneIDSet(preQualified)`).
	for i := range w.DigestPreSets {
		if w.DigestPreSets[i].Tag == tagQualifiedRoot {
			w.DigestPreSets[i].PreIDs = dropID(w.DigestPreSets[i].PreIDs, victim)
		}
	}
	// (2) Match the freeze witness to the forged source so the member cross-check passes.
	var kept []StateRootRotateMember
	for _, m := range w.Rotate.Members {
		if m.ID != victim {
			kept = append(kept, m)
		}
	}
	w.Rotate.Members = kept
	// (3) Supply the victim's own HONEST epochSet DELETE proof, so the box evicts it outright.
	esKey := statehash.Key(tagEpochSet, victim[:])
	wit, sibs, err := f.prover.ProveWithSiblings(esKey)
	if err != nil {
		t.Fatalf("fixture: ProveWithSiblings(victim epochSet): %v", err)
	}
	w.Rotate.PriorEpochSet = append(w.Rotate.PriorEpochSet, StateRootRotateMember{
		ID: victim, EpochSetOldValue: f.preValue(esKey), EpochSetProof: wit, EpochSetDeleteSiblings: sibs,
	})

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: forged root == honest root %x", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		fmt.Sprintf("the boundary freeze copies the UN-ANCHORED pre-qualified set, so dropping honest qualified "+
			"validator %x from it evicts that validator from the frozen epochSet for the whole next epoch", victim[:8])); msg != "" {
		t.Fatal(msg)
	}
	// The refusal happens before any op is assembled, so the boundary emits nothing at all.
	// That is the point: the freeze source is proven when it is READ, so a forged one never
	// reaches the op list that would otherwise have been its only check — and on this class
	// qualifiedRoot never appears in that list anyway.
	if len(ops) != 0 {
		t.Fatalf("the boundary refused the forged freeze source but still assembled %d fold op(s). A "+
			"read refused at the anchor must produce no ops, or the forged set has already been used "+
			"to derive them.", len(ops))
	}
}

// rtAnchorPickVictim returns a qualified pre-state id that is NOT the proposer.
func rtAnchorPickVictim(t *testing.T, f rotateFixture) ports.NodeID {
	t.Helper()
	pid := ports.HashBytes(pubOf(f.proposer))
	for _, id := range f.preQualifiedIDs() {
		if id != pid {
			return id
		}
	}
	t.Fatalf("VACUOUS FIXTURE: no non-proposer qualified victim exists")
	return ports.NodeID{}
}

// TestSameForgeryAgainstHonestRootStalls is the NON-VACUITY CONTROL, and it is a control, not a pin.
// It runs the forgery against the HONEST committed root — the oracle every shipped PreIDs ablation
// uses — and asserts it stalls. It must stay GREEN before AND after the remedy lands.
//
// WHAT IT PROVES, and this is its whole value: the A0.A6 pins are not simply rejecting everything.
// The same box, the same forgery, a different oracle, a different outcome — so the pins' greens are
// about the ORACLE, not about a broken fixture.
//
// WHAT IT DOES NOT PROVE — MEASURED, not argued. An earlier draft of this docstring claimed "a remedy
// that OVER-REJECTS shows up here". That is FALSE, and it was driven: under a maximally
// over-rejecting box — anchoredPreSet refusing EVERY pre-set, honest or forged, with
// ErrRecomputeStateRootDigest — all seven pins went RED and this control stayed GREEN. Two structural
// reasons, either one sufficient. rtAnchorIsRecomputeStall ACCEPTS ErrRecomputeStateRootDigest, which
// is precisely the class the natural remedy refuses with. And this test runs only a FORGED witness,
// so it has no honest-path arm: "stall harder" is indistinguishable from "stall correctly".
//
// WHAT DOES DETECT OVER-REJECTION: the package's honest-path recomputes, the …AgreesWithApply family
// — TestRecomputeStateRootSlashAgreesWithApply, TestRecomputeStateRootRotateAgreesWithApply,
// TestRecomputeStateRootAttAgreesWithApply and their siblings. Those feed an HONEST witness and
// require the box to agree with apply, so a box that refuses everything fails them all. Those, not
// this, are the arm a remedy must keep green.
//
// What this control does bound is narrower and still worth having: the stall against the honest root
// stays inside {digest, fold, mismatch}, so a NEW failure shape — a panic turned into an error, a
// nil-deref — surfaces here rather than reading as "it stalled, good".
func TestSameForgeryAgainstHonestRootStalls(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	honest := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)
	victim := rtAnchorPickVictim(t, f)

	for i := range w.DigestPreSets {
		if w.DigestPreSets[i].Tag == tagQualifiedRoot {
			w.DigestPreSets[i].PreIDs = dropID(w.DigestPreSets[i].PreIDs, victim)
		}
	}
	var kept []StateRootRotateMember
	for _, m := range w.Rotate.Members {
		if m.ID != victim {
			kept = append(kept, m)
		}
	}
	w.Rotate.Members = kept

	err := recomputeViaHead(f.c, f.prevRoot, honest, b, w)
	if err == nil {
		t.Fatalf("CONTROL FAILED — the same forgery measured against the HONEST root %x did NOT stall. The box now "+
			"accepts an honest root from a forged witness, which is strictly worse than the pinned finding and is "+
			"NOT what pins. Escalate before touching any pin above.", honest[:])
	}
	if !rtAnchorIsRecomputeStall(err) {
		t.Fatalf("CONTROL FAILED — the stall against the honest root is not a digest/fold/mismatch refusal: %v. A "+
			"remedy that rejects with a NEW SHAPE (a panic turned into an error, a nil-deref) would read as "+
			"'fixed' everywhere else in this file. NOTE, measured: a blanket refusal carrying "+
			"ErrRecomputeStateRootDigest does NOT trip this arm — see this test's docstring for what does.", err)
	}
	t.Logf("CONTROL ok — against the HONEST root the identical forgery stalls: %v", err)
}

// ══════════════════════════════════════════════════════════════════════════════
// class A, injecting the attesters suppresses the validatorsSeenRoot fold
// ══════════════════════════════════════════════════════════════════════════════

func TestAForgedSeenPreSetCannotSuppressTheDigest(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	honest := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	if len(b.LastCommit) == 0 {
		t.Fatalf("VACUOUS FIXTURE: the block names no attesters, so there is nothing to inject")
	}

	// THE FORGERY: claim everyone the carrier names was ALREADY seen, which makes post == pre and
	// drives stateRootAttDigestOp down its `return nil, nil` arm.
	inject := append([]ports.NodeID(nil), f.preIDsValidatorsSeen()...)
	for i := range b.LastCommit {
		inject = append(inject, b.LastCommit[i].AttesterID())
	}
	rtAnchorSetPreIDs(t, &w, tagValidatorsSeenRoot, sortIDs(inject))

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: forged root == honest root %x — the digest did not move honestly either", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		"injecting the named attesters into the un-anchored pre-seen set makes post == pre, suppresses the "+
			"validatorsSeenRoot fold op entirely, and accepts a root whose measured-decentralisation digest "+
			"is stale"); msg != "" {
		t.Fatal(msg)
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagValidatorsSeenRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// class B, an EMPTY slashedRoot pre-set re-seats a slashed equivocator (F2)
// ══════════════════════════════════════════════════════════════════════════════

func TestAnEmptySlashedPreSetCannotReSeatAnEquivocator(t *testing.T) {
	f := buildBondFixture(t)

	// Slash the squatter on-chain, then re-derive the pre-state prover/root at the new head.
	prev, h := f.c.Head()
	sqid := ports.HashBytes(pubOf(f.squatter))
	sb := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		Slashes: []Equivocation{slashProof(f.squatter, prev, 0x41, 0x42)}}
	Sign(&sb, f.proposer)
	f.c.apply(sb)
	if !f.c.slashed[sqid] {
		t.Fatalf("VACUOUS FIXTURE: the squatter is not slashed after the slash block, so F2 is not under test")
	}
	pr, err := statehash.NewProver(f.c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("fixture: NewProver: %v", err)
	}
	f.prover = pr
	f.prevRoot = pr.Root()
	sr, err := f.c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil || sr != f.prevRoot {
		t.Fatalf("fixture: pre-root mismatch after the slash: %v %x %x", err, sr[:], f.prevRoot[:])
	}

	// The slashed equivocator re-registers its OWN root.
	prev, h = f.c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(f.squatter, ports.HashBytes(pubOf(f.squatter)), 8<<20, prev, 5, 7)}}

	// GROUND TRUTH: real apply must DROP the registration. That is
	// F2.
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, back := clone.bonded[sqid]; back {
		t.Fatalf("VACUOUS FIXTURE: real apply() RE-BONDED a slashed id. F2 is broken in apply() itself, which is a " +
			"far more serious finding than the one pinned here. Escalate; do not re-pin.")
	}
	honest := f.applyAndCommittedRoot(t, b)

	oldDue, _ := f.preDueHeight(sqid)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, uniqueU64(oldDue, newDue))

	rtAnchorSetPreIDs(t, &w, tagSlashedRoot, nil) // THE FORGERY: claim nobody is slashed

	// Supply the changed-leaf witnesses the FORGED derivation needs.
	screens := map[ports.Hash]StateRootBondRegScreen{}
	for _, r := range b.BondRegs {
		screens[r.Root] = f.bondScreen(r.Root)
	}
	preBRH := map[ports.NodeID]uint64{}
	for id, hh := range f.c.bondRegHeight {
		preBRH[id] = hh
	}
	delta, dErr := f.c.stateRootBondRegWriteSet(f.prevRoot, b,
		idSet(f.preIDsBonded()), idSet(f.preIDsQualified()), map[ports.NodeID]struct{}{}, screens, preBRH)
	if dErr != nil {
		t.Fatalf("fixture: forged write-set: %v", dErr)
	}
	if len(delta.writes) == 0 {
		t.Fatalf("VACUOUS: the forged derivation produced no writes")
	}
	for _, wr := range delta.writes {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	if _, requal := delta.postQual[sqid]; !requal {
		t.Fatalf("VACUOUS: the forged derivation did not re-qualify the slashed id, so the F2 violation is not " +
			"actually constructed and a green result would assert nothing")
	}

	forged, ops, foldErr := rtAnchorForgedFold(t, f.c, f.prevRoot, b, w)
	stall := foldErr
	if stall == nil {
		if forged == honest {
			t.Fatalf("VACUOUS: forged root == honest root %x", honest[:])
		}
		stall = recomputeViaHead(f.c, f.prevRoot, forged, b, w)
	}
	if msg := forgedPreSetMustStall("", stall, forged, honest,
		fmt.Sprintf("an EMPTY slashedRoot pre-set defeats the F2 bar outright: slashed equivocator %x is admitted "+
			"BONDED and QUALIFIED again, because preSlashed is the only thing enforcing F2 in the box and it is "+
			"never folded on a class-B block", sqid[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagSlashedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// THE TEETH — each predicate is fed its POST-FIX input and must speak up.
// Without these the pins above claim they will redden and nothing has ever checked that they can
// (a green gate with no demonstrated red is decoration).
// ══════════════════════════════════════════════════════════════════════════════

func TestTheStallAssertionSpeaksWhenTheBoxAccepts(t *testing.T) {
	a, bb := ports.HashBytes([]byte("forged")), ports.HashBytes([]byte("honest"))
	if msg := forgedPreSetMustStall("attack", nil, a, bb, "the consequence"); msg == "" {
		t.Fatal("the stall assertion stayed SILENT when the box raised no objection to a forged pre-set — it " +
			"cannot detect the very break it exists to detect, and the attack could reopen with every gate green")
	}
	// Any refusal is a pass: the assertion asks whether the box certified the adversary's state,
	// not which refusal it chose. A stall class that changes because a check moved earlier is not
	// a regression, and an assertion that demanded one exact class would redden on every such move.
	for _, stall := range []error{ErrRecomputeStateRootDigest, ErrRecomputeStateRootMismatch, fmt.Errorf("some other refusal")} {
		if msg := forgedPreSetMustStall("attack", stall, a, bb, "c"); msg != "" {
			t.Fatalf("the stall assertion fired on a refusal (%v), which is the outcome it exists to accept: %s", stall, msg)
		}
	}
}

// TestTheFailureTextPrintsA64CharRoot gates the assertion's own FAILURE TEXT, which is the only
// thing a future engineer reads on the day it fires. Two defects, both observed live in this
// file's output before this gate existed:
//
// 1. ports.Hash is [32]byte with a VALUE-receiver String, and fmt applies Stringer for %x. So %x
// On the VALUE hex-encodes the 64-char hex string a SECOND time and prints 128 chars. The run
// that found it printed a "root" of 3030303030… — the hex of the ASCII text "000…". go vet does
// not flag this: %x is a legal verb for a Stringer. This test is the only thing standing between
// that slip and the next one.
// 2. The roots are only meaningful on the accept path — a refused witness derives none — so the
// text must render the ones it does print correctly.
func TestTheFailureTextPrintsA64CharRoot(t *testing.T) {
	forged, honest := ports.HashBytes([]byte("forged")), ports.HashBytes([]byte("honest"))
	msg := forgedPreSetMustStall("attack", nil, forged, honest, "the consequence")

	for _, tc := range []struct {
		name string
		h    ports.Hash
	}{{"forged", forged}, {"honest", honest}} {
		sliced := fmt.Sprintf("%x", tc.h[:])
		if len(sliced) != 64 {
			t.Fatalf("TEETH SETUP: a sliced ports.Hash must render 64 hex chars, got %d", len(sliced))
		}
		doubled := fmt.Sprintf("%x", tc.h) // the Stringer route: 128 chars
		if len(doubled) != 128 {
			t.Fatalf("TEETH SETUP: %%x on a ports.Hash VALUE should double-encode to 128 chars, got %d — "+
				"ports.Hash lost its value-receiver String() and this whole gate is now about nothing", len(doubled))
		}
		if strings.Contains(msg, doubled) {
			t.Fatalf("the %s root is DOUBLE-ENCODED in the pin text (128 hex chars, via the value-receiver "+
				"String()). Print h[:], not h.\n  msg=%s", tc.name, msg)
		}
		if !strings.Contains(msg, sliced) {
			t.Fatalf("the %s root does not appear in the pin text as 64 hex chars.\n  want=%s\n  msg=%s",
				tc.name, sliced, msg)
		}
	}

	// A refusal prints nothing at all: there is no root to name, and a reader who trusts a
	// rendered zero hash chases one that never existed.
	if quiet := forgedPreSetMustStall("attack", ErrRecomputeStateRootDigest, ports.Hash{}, honest, "c"); quiet != "" {
		t.Fatalf("a refusal produced failure text: %s", quiet)
	}
}

func TestTheAnchoringAssertionSpeaksWhenTheTagIsFolded(t *testing.T) {
	folded := []statehash.FoldOp{
		{Key: statehash.Key(tagEpochSetRoot, nil), NewValue: []byte("e")},
		{Key: statehash.Key(tagSlashedRoot, nil), NewValue: []byte("s")},
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagSlashedRoot, folded); msg == "" {
		t.Fatal("the anchoring assertion stayed silent when the tag WAS folded — the mechanism arm cannot detect the " +
			"remedy, so a fix that anchored the pre-set would leave the arm green")
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagQualifiedRoot, folded); msg != "" {
		t.Fatalf("rtAnchorMechanismPin fired for a tag that is NOT in the op list: %s", msg)
	}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagSlashedRoot, nil); msg != "" {
		t.Fatalf("rtAnchorMechanismPin fired on an empty op list: %s", msg)
	}
	// Prefix-safety: the tags are NUL-terminated precisely so one tag's key is not a prefix of
	// another's. A comparison that lost that would make every arm above answer about the wrong tag.
	memberID := ports.HashBytes([]byte("m"))
	perMember := []statehash.FoldOp{{Key: statehash.Key(tagEpochSet, memberID[:]), NewValue: []byte("v")}}
	if msg := anchoringMustNotDependOnTheFoldOp("", tagEpochSetRoot, perMember); msg != "" {
		t.Fatalf("the anchoring assertion confused a per-member leaf key for a whole-set digest key: %s", msg)
	}
}

func TestTheRejectAssertionSpeaksWhenAnchoredPreSetAccepts(t *testing.T) {
	mth := nodeSetMTH([]ports.NodeID{ports.HashBytes([]byte("a"))})
	other := nodeSetMTH(nil)
	if msg := forgedPreSetMustBeRejected(nil, mth, other); msg == "" {
		t.Fatal("the reject assertion stayed SILENT when anchoredPreSet ACCEPTED an id-list that does not hash " +
			"to the committed digest — the one thing it exists to catch")
	}
	if msg := forgedPreSetMustBeRejected(ErrRecomputeStateRootDigest, mth, other); msg != "" {
		t.Fatalf("the reject assertion fired on a refusal, which is the outcome it exists to accept: %s", msg)
	}
	if msg := forgedPreSetMustBeRejected(ErrRecomputeStateRootDigest, mth, mth); msg == "" {
		t.Fatal("the reject assertion stayed silent when the forged id-list hashed to the COMMITTED digest — a " +
			"refusal there proves nothing, and a vacuous gate is the shape this file most needs to refuse")
	}
}
