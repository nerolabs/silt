package chain

// RED-TEAM R-ANCHORED-PRE-SET — anchoredPreSet does not anchor.
// Confirmed by the Tester at origin/main @ 34591d7, 2026-09-13. The artifact is byte-identical on
// local main @ 8694e5f (`git diff --stat main origin/main -- core/chain` is empty), so the finding
// is not a function of which of the two revisions you read.
//
// Probes preserved verbatim (sha256 recorded) at
//   .claude/agent-memory/tester/evidence/2026-09-13-rt-anchoredpreset/probes-verbatim/
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
// THE UNANCHORED-READ CENSUS — re-derived here by the Tester, not taken on report
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Every digestFoldOp call site in the tree (non-test), and the anchoredPreSet read it does or does
// not cover:
//
//	class B (bondreg) — reads bondedRoot, qualifiedRoot, slashedRoot.
//	  EMITS bondedRoot IFF !idSetsEqual(preBonded, delta.postBonded);
//	  EMITS qualifiedRoot IFF !idSetsEqual(preQualified, delta.postQual);
//	  EMITS slashedRoot NEVER — there is no digestFoldOp(tagSlashedRoot, …) in that file at all.
//	  ⇒ slashedRoot is unanchored on EVERY class-B block, and it is the sole carrier of the F2 bar
//	    ("a slashed equivocator cannot re-earn standing") at stateRootBondRegWriteSet's
//	    `if _, isSlashed := preSlashed[id]` screen.
//	  ⇒ bonded/qualified are unanchored whenever membership is unchanged, AND THE FORGERY IS WHAT
//	    MAKES IT UNCHANGED: delta.post* is derived FROM the forged pre-set, so an attacker who
//	    forges the pre-set steers the very equality that decides whether it gets checked.
//
//	class P (rotate) — reads qualifiedRoot as the epoch-set FREEZE SOURCE
//	  (`post := cloneIDSet(preQualified)`), and EMITS only epochSetRoot.
//	  ⇒ qualifiedRoot is unanchored on any boundary with no B/T/S class also emitting it.
//
//	class A (atts) — reads validatorsSeenRoot; stateRootAttDigestOp returns (nil, nil) when
//	  idSetsEqual(preValidatorsSeen, postValidatorsSeen).
//	  ⇒ unanchored whenever post == pre, which injecting the attesters into the pre-set guarantees.
//
//	class S (slash) and class T (ttl) EMIT their digest ops unconditionally and are NOT part of
//	  this finding. Recording the safe classes is the point: a census that came back uniform would
//	  be a bug report about the census (scar:empty-result-reads-as-the-desired-answer).
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
// consensus-adjacent verification rule and is RESEARCH-GATED (.claude/CLAUDE.md, the research gate).
// The Tester encodes a confirmed break; it does not fix one. So these land as PINNED_DEFECT under
// D-REPAIR-CLAIM-GATES-PINNED-2026-09-12: each asserts CURRENT BROKEN BEHAVIOUR, passes today, and
// REDDENS the moment the behaviour moves — which forces the record to be updated rather than letting
// the defect close silently. t.Skip is refused by that same decision; a skip is a dark test.
//
// The idiom (PIN + TEETH + MECHANISM) is documented in full at the head of
// core/pipeline/rt_sfo_manifest_oracle_test.go. Each pin here carries:
//   1. the PIN — the symptom, via rtAnchorPin: the box returned nil for the adversary's own root;
//   2. a MECHANISM arm — via rtAnchorMechanismPin: NO fold op for the forged tag was emitted, so the
//      pre-set went unanchored. A fix that emitted the op but left the symptom, or moved the symptom
//      without removing the cause, reddens here rather than passing by coincidence;
//   3. TEETH — TestRTAnchor_PinRedensWhenTheBoxRejects, …WhenTheTagIsFolded and …WhenAnchoredPreSetAnchors,
//      which feed each predicate its POST-FIX input and assert it speaks up. A pin whose teeth are
//      untested has never been shown capable of failing (simplicity rule 7).
//
// CONTAINMENT, so nobody reads these as "main is exploitable today": (*Box).Validate still applies
// the R1.8 downgrade — Accept ⇒ IndeterminateTrustlessly / ErrRecomputeGated. The box never accepts
// on this path yet. That downgrade is the only barrier between this finding and damage, and it is
// deliberately not touched here. The finding is what happens on the day R1.8 flips.

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

// rtAnchorPin is the SYMPTOM predicate for RT-ANCHOR-1..6. It is handed the stall the recompute
// returned when measured against the root the box's OWN fold produced from the FORGED witness.
// Empty string ⇒ the defect is still live (the box certified the adversary's root). Non-empty ⇒ the
// behaviour moved and the record must be re-derived.
func rtAnchorPin(defect string, stall error, forged, honest ports.Hash, consequence string) string {
	if stall == nil {
		return ""
	}
	// ports.Hash is [32]byte with a VALUE-receiver String(), and fmt routes %x through Stringer — so
	// %x on the VALUE hex-encodes the 64-char hex string AGAIN and prints 128 chars. Every root in
	// this file is sliced. Gated by TestRTAnchor_PinTextPrintsA64CharRootAndNeverTheZeroHash.
	forgedLine := fmt.Sprintf("  forged (the root the box derived from the forged witness) = %x\n", forged[:])
	if forged == (ports.Hash{}) {
		// rtAnchorForgedFold returns the ZERO Hash when op assembly or the fold REFUSED the forged
		// witness, so no root was ever derived. Printing it labels 32 zero bytes as the adversary's
		// root, which is the one line a future engineer would most reasonably trust.
		forgedLine = "  forged = UNAVAILABLE — the box refused the forged witness at op assembly or at the fold,\n" +
			"           so it never derived a root. The stall above IS the whole observation.\n"
	}
	return fmt.Sprintf(
		"%s PIN IS RED — the box now REJECTS the forged witness measured against ITS OWN fold root: %v\n"+
			"%s"+
			"  honest (what real apply() yields)                          = %x\n"+
			"  The pinned defect was: %s\n"+
			"  READ THIS BEFORE RE-PINNING. This pin asserts CURRENT BROKEN BEHAVIOUR (PINNED_DEFECT,\n"+
			"  D-REPAIR-CLAIM-GATES-PINNED-2026-09-12). A RED here is the EXPECTED outcome of the remedy:\n"+
			"  anchoredPreSet (floorbox_recompute_stateroot_slash_v5.go) doing what provenView.members\n"+
			"  (stateview_proven_v5.go) already does — Resolve the digest leaf against prevStateRoot and\n"+
			"  refuse unless nodeSetMTH(PreIDs) equals it. If that landed: retire this pin, replace it with\n"+
			"  the straight assertion that the forgery stalls, KEEP the teeth, and restore the PreIDs row of\n"+
			"  foldInputCoverageTable (floorbox_recompute_carrier_reflection_v5_test.go) from FIX-OPEN to\n"+
			"  already-anchored with the anchor the remedy actually installs. If it did NOT land, something\n"+
			"  else changed the fold-op emission and the census at the head of this file must be re-derived\n"+
			"  before anyone re-pins.\n"+
			"  BLAST RADIUS — THE REMEDY REDDENS NINE TESTS, NOT SEVEN (measured by a blind PE under a\n"+
			"  simulated remedy, 2026-09-13). Besides RT-ANCHOR-0..6, two SHIPPED ablations in this package\n"+
			"  redden, both BENIGNLY and both for the same reason: the refusal moves EARLIER, into\n"+
			"  anchoredPreSet, so the stall CLASS becomes ErrRecomputeStateRootDigest.\n"+
			"    - TestRecomputeStateRootSlashAblationForgedQualifiedScreen: its fixture drops the culprit\n"+
			"      from the qualified pre-set, so the remedy's predicate GENUINELY fires. It accepts only\n"+
			"      Fold|Mismatch. Widen the accepted class to include Digest.\n"+
			"    - TestRecomputeStateRootSlashAblationCircularAnchor: its id-lists are HONEST and only the\n"+
			"      proof anchor is circular, so the remedy's Resolve against prevStateRoot fails inside\n"+
			"      anchoredPreSet rather than in FoldChangedPaths. It asserts Fold exactly. Widen it to\n"+
			"      include Digest.\n"+
			"  Both still STALL; neither is a safety regression. Widen the accepted class — do not revert\n"+
			"  the remedy and do not delete either ablation.",
		defect, stall, forgedLine, honest[:], consequence)
}

// rtAnchorMechanismPin is the MECHANISM predicate: it asserts the CAUSE is still present, namely
// that the box emitted NO fold op for the forged tag, which is exactly why that tag's PreIDs was
// never anchored. Empty string ⇒ cause still live.
func rtAnchorMechanismPin(defect, tag string, ops []statehash.FoldOp) string {
	want := statehash.Key(tag, nil)
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) {
			return fmt.Sprintf(
				"%s MECHANISM ARM IS RED — the box DID emit a fold op for %q (op[%d], NewValue=%x), so that\n"+
					"  tag's PreIDs is now anchored by FoldChangedPaths after all. The cause this pin holds is\n"+
					"  gone or moved. Re-derive the unanchored-read census at the head of this file (every\n"+
					"  digestFoldOp call site against every anchoredPreSet read) before re-pinning — a symptom\n"+
					"  that still reproduces with a different cause is a different finding.",
				defect, rtAnchorTagName(tag), i, ops[i].NewValue)
		}
	}
	return ""
}

// rtAnchorA0Pin is the unit-level predicate for RT-ANCHOR-0: anchoredPreSet accepts an id-list whose
// nodeSetMTH is not the committed digest. Empty string ⇒ still unanchored.
func rtAnchorA0Pin(err error, gotMTH, committed []byte) string {
	if err != nil {
		return fmt.Sprintf(
			"RT-ANCHOR-0 PIN IS RED — anchoredPreSet now REJECTS a forged pre-set: %v\n"+
				"  This is the expected outcome of the research-gated remedy (make anchoredPreSet do what\n"+
				"  provenView.members does). Retire this pin, keep its teeth, and restore the PreIDs row of\n"+
				"  foldInputCoverageTable — this finding is why that row is classified FIX-OPEN today.\n"+
				"  The remedy's full BLAST RADIUS is nine tests, not seven; rtAnchorPin's FIX CASE above\n"+
				"  names the two shipped ablations that also redden and the class widening they need.", err)
	}
	if bytes.Equal(gotMTH, committed) {
		return fmt.Sprintf(
			"RT-ANCHOR-0 IS VACUOUS — the forged id-list happens to hash to the committed digest\n"+
				"  (nodeSetMTH=%x == committed=%x), so accepting it proves nothing. Change the junk id.",
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
// live defect (scar:noop-ablation-anchor-not-unique).
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
// RT-ANCHOR-0 — anchoredPreSet accepts a pre-set whose MTH is NOT the committed digest
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A0_AnchoredPreSetDoesNotAnchor_PINNED_DEFECT(t *testing.T) {
	f := buildBondFixture(t)
	junk := []ports.NodeID{ports.HashBytes([]byte("not-a-member-at-all"))}
	w := f.digestWitness(t, tagSlashedRoot, junk) // HONEST proof, FORGED PreIDs
	byTag := map[string]*StateRootDigestWitness{tagSlashedRoot: &w}

	set, err := anchoredPreSet(byTag, tagSlashedRoot)
	committed := f.preValue(statehash.Key(tagSlashedRoot, nil))
	if msg := rtAnchorA0Pin(err, nodeSetMTH(junk), committed); msg != "" {
		t.Fatal(msg)
	}
	// MECHANISM: the returned set is the witness id-list verbatim — no filtering, no anchoring. A
	// fix that anchored but returned the same cardinality by coincidence still reddens above; this
	// arm pins that the function is a pure transcription of attacker input.
	if len(set) != len(junk) {
		t.Fatalf("RT-ANCHOR-0 MECHANISM ARM IS RED — anchoredPreSet returned %d ids for a %d-id witness, so it "+
			"is no longer a verbatim transcription of PreIDs. Re-read the function before re-pinning.", len(set), len(junk))
	}
	if _, ok := set[junk[0]]; !ok {
		t.Fatalf("RT-ANCHOR-0 MECHANISM ARM IS RED — the forged id is no longer present in the returned set; " +
			"anchoredPreSet has started filtering. Re-derive the finding.")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-ANCHOR-1 — class B, slashedRoot INJECTION erases a bond registration
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A1_ForgedSlashedPreSetErasesABondReg_PINNED_DEFECT(t *testing.T) {
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
	if msg := rtAnchorPin("RT-ANCHOR-1", stall, forged, honest,
		fmt.Sprintf("injecting registrant %x into the never-folded slashedRoot pre-set makes the box drop its whole "+
			"bond registration and certify a root in which the registration never happened", freshID[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-1", tagSlashedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
	// MECHANISM, second arm: the erasure is total — the box emits a root identical to prevStateRoot,
	// i.e. a block carrying a BondReg payload certifies as a complete no-op. Pinning only "forged !=
	// honest" would stay green if the forgery merely perturbed the root.
	if forged != f.prevRoot {
		t.Fatalf("RT-ANCHOR-1 MECHANISM ARM IS RED — the forged root %x is no longer identical to prevStateRoot %x. "+
			"The pinned consequence is TOTAL erasure: a BondReg-carrying block folding to zero net state change. "+
			"A partial erasure is a different finding; re-derive stateRootBondRegWriteSet's screen before re-pinning.",
			forged[:], f.prevRoot[:])
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-ANCHOR-2 — class B, all three pre-sets forged at once
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A2_AllThreeClassBPreSetsUnverified_PINNED_DEFECT(t *testing.T) {
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
	if msg := rtAnchorPin("RT-ANCHOR-2", stall, forged, honest,
		"all THREE class-B pre-sets (slashed, bonded, qualified) replaced with garbage and the box still "+
			"certified — bonded and qualified escape their emission guard because the forgery itself is what "+
			"makes idSetsEqual(pre, post) true"); msg != "" {
		t.Fatal(msg)
	}
	// MECHANISM: none of the three tags was folded. This is the arm that carries the emission-guard
	// half of the finding — bonded/qualified are guarded by !idSetsEqual, and a forged pre-set steers
	// that equality.
	for _, tag := range []string{tagSlashedRoot, tagBondedRoot, tagQualifiedRoot} {
		if msg := rtAnchorMechanismPin("RT-ANCHOR-2", tag, ops); msg != "" {
			t.Fatal(msg)
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-ANCHOR-3 — class B, qualifiedRoot OMISSION keeps an under-bonded validator qualified
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A3_ForgedQualifiedPreSetKeepsUnderBondedStanding_PINNED_DEFECT(t *testing.T) {
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
	if msg := rtAnchorPin("RT-ANCHOR-3", stall, forged, honest,
		fmt.Sprintf("dropping %x from the un-anchored pre-qualified set makes the box certify a root in which an "+
			"UNDER-BONDED validator keeps its qualified||id leaf and qualifiedRoot never moves", pid[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-3", tagQualifiedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-ANCHOR-4 — class P, the boundary FREEZE SOURCE evicts an honest qualified validator
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A4_ForgedFreezeSourceEvictsAQualifiedValidator_PINNED_DEFECT(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	honest := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	victim := rtAnchorPickVictim(t, f)
	if _, in := f.c.epochSet[victim]; !in {
		t.Fatalf("VACUOUS FIXTURE: the victim is not in the prior epochSet, so there is nothing to evict it from")
	}

	// (1) THE FORGERY: drop the victim from the UN-ANCHORED pre-qualified id-list, which class P
	//     reads as the freeze source (`post := cloneIDSet(preQualified)`).
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
	if msg := rtAnchorPin("RT-ANCHOR-4", stall, forged, honest,
		fmt.Sprintf("the boundary freeze copies the UN-ANCHORED pre-qualified set, so dropping honest qualified "+
			"validator %x from it evicts that validator from the frozen epochSet for the whole next epoch", victim[:8])); msg != "" {
		t.Fatal(msg)
	}
	// MECHANISM: class P emits epochSetRoot and NOT qualifiedRoot. Both arms matter — the absence is
	// the cause, and the presence of epochSetRoot proves the op list was really assembled (an empty
	// op list would make the absence arm pass for the wrong reason).
	if msg := rtAnchorMechanismPin("RT-ANCHOR-4", tagQualifiedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
	if rtAnchorMechanismPin("RT-ANCHOR-4", tagEpochSetRoot, ops) == "" {
		t.Fatalf("RT-ANCHOR-4 MECHANISM ARM IS RED — class P emitted NO epochSetRoot fold op (%d ops total). The "+
			"qualifiedRoot-absence arm above would then be passing for the wrong reason. Re-derive rotateOps.", len(ops))
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

// TestRTAnchor_A4b_SameForgeryAgainstHonestRootStalls is the NON-VACUITY CONTROL, and it is a
// control, not a pin. It runs the RT-ANCHOR-4 forgery against the HONEST committed root — the oracle
// every shipped PreIDs ablation uses — and asserts it stalls. It must stay GREEN before AND after
// the remedy lands.
//
// WHAT IT PROVES, and this is its whole value: the A0..A6 pins are not simply rejecting everything.
// The same box, the same forgery, a different oracle, a different outcome — so the pins' greens are
// about the ORACLE, not about a broken fixture.
//
// WHAT IT DOES NOT PROVE — MEASURED, not argued. An earlier draft of this docstring claimed "a remedy
// that OVER-REJECTS shows up here". That is FALSE, and it was driven: under a maximally
// over-rejecting box — anchoredPreSet refusing EVERY pre-set, honest or forged, with
// ErrRecomputeStateRootDigest — all seven pins went RED and this control stayed GREEN (blind PE,
// 2026-09-13). Two structural reasons, either one sufficient. rtAnchorIsRecomputeStall ACCEPTS
// ErrRecomputeStateRootDigest, which is precisely the class the natural remedy refuses with. And
// this test runs only a FORGED witness, so it has no honest-path arm: "stall harder" is
// indistinguishable from "stall correctly".
//
// WHAT DOES DETECT OVER-REJECTION: the package's honest-path recomputes, the …AgreesWithApply family
// — TestRecomputeStateRootSlashAgreesWithApply, TestRecomputeStateRootRotateAgreesWithApply,
// TestRecomputeStateRootAttAgreesWithApply and their siblings. Those feed an HONEST witness and
// require the box to agree with apply(), so a box that refuses everything fails them all. Those, not
// this, are the arm a remedy must keep green.
//
// What this control does bound is narrower and still worth having: the stall against the honest root
// stays inside {digest, fold, mismatch}, so a NEW failure shape — a panic turned into an error, a
// nil-deref — surfaces here rather than reading as "it stalled, good".
func TestRTAnchor_A4b_SameForgeryAgainstHonestRootStalls(t *testing.T) {
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
			"certifies an honest root from a forged witness, which is strictly worse than the pinned finding and is "+
			"NOT what RT-ANCHOR-4 pins. Escalate before touching any pin above.", honest[:])
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
// RT-ANCHOR-5 — class A, injecting the attesters suppresses the validatorsSeenRoot fold
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A5_ForgedSeenPreSetSuppressesTheDigest_PINNED_DEFECT(t *testing.T) {
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
	if msg := rtAnchorPin("RT-ANCHOR-5", stall, forged, honest,
		"injecting the named attesters into the un-anchored pre-seen set makes post == pre, suppresses the "+
			"validatorsSeenRoot fold op entirely, and certifies a root whose measured-decentralisation digest "+
			"is stale"); msg != "" {
		t.Fatal(msg)
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-5", tagValidatorsSeenRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-ANCHOR-6 — class B, an EMPTY slashedRoot pre-set re-seats a slashed equivocator (F2)
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_A6_EmptySlashedPreSetReSeatsAnEquivocator_PINNED_DEFECT(t *testing.T) {
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

	// GROUND TRUTH: real apply() must DROP the registration. That is F2.
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
	if msg := rtAnchorPin("RT-ANCHOR-6", stall, forged, honest,
		fmt.Sprintf("an EMPTY slashedRoot pre-set defeats the F2 bar outright: slashed equivocator %x is certified "+
			"BONDED and QUALIFIED again, because preSlashed is the only thing enforcing F2 in the box and it is "+
			"never folded on a class-B block", sqid[:8])); msg != "" {
		t.Fatal(msg)
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-6", tagSlashedRoot, ops); msg != "" {
		t.Fatal(msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// THE TEETH — each predicate is fed its POST-FIX input and must speak up.
// Without these the pins above claim they will redden and nothing has ever checked that they can
// (simplicity rule 7: a green gate with no demonstrated red is decoration).
// ══════════════════════════════════════════════════════════════════════════════

func TestRTAnchor_PinRedensWhenTheBoxRejects(t *testing.T) {
	a, bb := ports.HashBytes([]byte("forged")), ports.HashBytes([]byte("honest"))
	if msg := rtAnchorPin("RT-ANCHOR-X", ErrRecomputeStateRootDigest, a, bb, "the consequence"); msg == "" {
		t.Fatal("rtAnchorPin stayed silent on a digest stall — the pins cannot detect the very fix they exist to " +
			"detect, and the defect could be closed with nobody updating the record")
	}
	if msg := rtAnchorPin("RT-ANCHOR-X", ErrRecomputeStateRootMismatch, a, bb, "the consequence"); msg == "" {
		t.Fatal("rtAnchorPin stayed silent on a terminal-mismatch stall")
	}
	if msg := rtAnchorPin("RT-ANCHOR-X", fmt.Errorf("some unrelated refusal"), a, bb, "c"); msg == "" {
		t.Fatal("rtAnchorPin stayed silent on an UNRELATED refusal — a box that started refusing for a new reason " +
			"would read as 'still broken' and the pin would be folklore")
	}
	if msg := rtAnchorPin("RT-ANCHOR-X", nil, a, bb, "c"); msg != "" {
		t.Fatalf("rtAnchorPin fired on the pinned state it is supposed to accept: %s", msg)
	}
}

// TestRTAnchor_PinTextPrintsA64CharRootAndNeverTheZeroHash gates the pin's own FAILURE TEXT, which
// is the only thing a future engineer reads on the day a pin fires. Two defects, both observed live
// in this file's output before this gate existed (blind PE, 2026-09-13):
//
//  1. ports.Hash is [32]byte with a VALUE-receiver String(), and fmt applies Stringer for %x. So %x
//     on the VALUE hex-encodes the 64-char hex string a SECOND time and prints 128 chars. The run
//     that found it printed a "root" of 3030303030… — the hex of the ASCII text "000…". go vet does
//     not flag this: %x is a legal verb for a Stringer. This test is the only thing standing between
//     that slip and the next one.
//  2. rtAnchorForgedFold returns the ZERO Hash when op assembly or the fold REFUSED, and the pin
//     labelled those 32 zero bytes "the root the box derived from the forged witness".
func TestRTAnchor_PinTextPrintsA64CharRootAndNeverTheZeroHash(t *testing.T) {
	forged, honest := ports.HashBytes([]byte("forged")), ports.HashBytes([]byte("honest"))
	msg := rtAnchorPin("RT-ANCHOR-X", ErrRecomputeStateRootDigest, forged, honest, "the consequence")

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

	// The refusal path: rtAnchorForgedFold derived no root, so none may be labelled as one.
	zeroMsg := rtAnchorPin("RT-ANCHOR-X", ErrRecomputeStateRootDigest, ports.Hash{}, honest, "c")
	if strings.Contains(zeroMsg, "the root the box derived from the forged witness") {
		t.Fatalf("the pin labelled the ZERO hash as the adversary's derived root. rtAnchorForgedFold returns "+
			"ports.Hash{} when op assembly or the fold REFUSED — there is no root to print, and a reader who "+
			"trusts that line chases a root that never existed.\n  msg=%s", zeroMsg)
	}
	if !strings.Contains(zeroMsg, "UNAVAILABLE") {
		t.Fatalf("the refusal path must SAY the box refused before deriving a root, not silently omit it.\n  msg=%s", zeroMsg)
	}
	if strings.Contains(zeroMsg, strings.Repeat("00", 32)) {
		t.Fatalf("the pin text still contains 32 zero bytes rendered as a root.\n  msg=%s", zeroMsg)
	}
}

func TestRTAnchor_MechanismArmRedensWhenTheTagIsFolded(t *testing.T) {
	folded := []statehash.FoldOp{
		{Key: statehash.Key(tagEpochSetRoot, nil), NewValue: []byte("e")},
		{Key: statehash.Key(tagSlashedRoot, nil), NewValue: []byte("s")},
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-X", tagSlashedRoot, folded); msg == "" {
		t.Fatal("rtAnchorMechanismPin stayed silent when the tag WAS folded — the mechanism arm cannot detect the " +
			"remedy, so a fix that anchored the pre-set would leave the arm green")
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-X", tagQualifiedRoot, folded); msg != "" {
		t.Fatalf("rtAnchorMechanismPin fired for a tag that is NOT in the op list: %s", msg)
	}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-X", tagSlashedRoot, nil); msg != "" {
		t.Fatalf("rtAnchorMechanismPin fired on an empty op list: %s", msg)
	}
	// Prefix-safety: the tags are NUL-terminated precisely so one tag's key is not a prefix of
	// another's. A comparison that lost that would make every arm above answer about the wrong tag.
	memberID := ports.HashBytes([]byte("m"))
	perMember := []statehash.FoldOp{{Key: statehash.Key(tagEpochSet, memberID[:]), NewValue: []byte("v")}}
	if msg := rtAnchorMechanismPin("RT-ANCHOR-X", tagEpochSetRoot, perMember); msg != "" {
		t.Fatalf("rtAnchorMechanismPin confused a per-member leaf key for a whole-set digest key: %s", msg)
	}
}

func TestRTAnchor_A0PinRedensWhenAnchoredPreSetAnchors(t *testing.T) {
	mth := nodeSetMTH([]ports.NodeID{ports.HashBytes([]byte("a"))})
	other := nodeSetMTH(nil)
	if msg := rtAnchorA0Pin(ErrRecomputeStateRootDigest, mth, other); msg == "" {
		t.Fatal("rtAnchorA0Pin stayed silent when anchoredPreSet started REJECTING — that refusal is the remedy, " +
			"and the pin exists to force the record to be updated when it lands")
	}
	if msg := rtAnchorA0Pin(nil, mth, mth); msg == "" {
		t.Fatal("rtAnchorA0Pin stayed silent when the forged id-list hashed to the COMMITTED digest — that is a " +
			"vacuous gate, and it is the shape this file most needs to refuse")
	}
	if msg := rtAnchorA0Pin(nil, mth, other); msg != "" {
		t.Fatalf("rtAnchorA0Pin fired on the pinned state it is supposed to accept: %s", msg)
	}
}
