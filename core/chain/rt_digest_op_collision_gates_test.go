package chain

// R-DIGEST-OP-COLLISION — two classes emit a fold op for the SAME digest key, and the last one wins.
//
// Routed to the Tester as DERIVED-FROM-SOURCE, NOT REPRODUCED by the Researcher
// (anchoredPreSet-not-anchored-REMEDY-RESEARCH-CERTIFICATION-2026-09-13.md §9, "I am NOT certifying
// this as a break ... Routed to the TESTER as a probe before anyone prices it").
//
// REPRODUCED by the Tester at origin/main @ 869ad9ae506f5b1b1593c8c7e023d84db301e383, 2026-09-13.
// Probes preserved verbatim (sha256 68acce8d47deb80ecd525691c112154a4eee34a0b01b30534e38ce456f37191c) at
//   .claude/agent-memory/tester/evidence/2026-09-13-rt-digest-op-collision/probes-verbatim/
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE BREAK
// ────────────────────────────────────────────────────────────────────────────────────────
//
// assembleStateRootRecomputeOps (floorbox_recompute_stateroot_v5.go) dispatches each transition
// class in turn and appends every class's digest ops into ONE slice, in the order S → B → T → A →
// M → P. Classes S, B and T each compute a whole-set digest for tagBondedRoot / tagQualifiedRoot
// from the anchored pre-set plus THEIR OWN CLASS DELTA ONLY:
//
//	S (stateRootSlashDigestOps): postBonded = pre \ culprits          — emitted unconditionally
//	B (stateRootBondRegDigestOps): postBonded = (pre ∪ regs) \ displaced — emitted iff the set changed
//	T (stateRootTTLDigestOps):   postBonded = pre \ expired           — emitted unconditionally
//
// All three land at the identical key statehash.Key(tag, nil), and all three read their OldValue
// from the SAME digest witness (digestFoldOp does `w := byTag[tag]; preDigest := nodeSetMTH(w.PreIDs)`),
// so every duplicate op carries a byte-identical OldValue and a byte-identical Proof. Step (1) of
// statehash.FoldChangedPaths therefore verifies ALL of them against prevStateRoot — none is rejected.
// Step (5) then replays them with trie.Update in slice order, with no dedup and no duplicate-key
// guard anywhere in the function. THE LAST OP ON A KEY WINS.
//
// A block carrying two of those classes folds the digest to the LAST class's value and silently
// drops the other class's membership change. MEASURED on an S+B block (D1 below):
//
//	S op NewValue                 = a7bd89ee…  == nodeSetMTH(pre \ culprit)
//	B op NewValue                 = 6bc0de14…  == nodeSetMTH(pre ∪ fresh)      ← the fold takes this
//	honest committed bondedRoot   = c92028bf…  == nodeSetMTH((pre ∪ fresh) \ culprit)
//
// CORRECTION OF RECORD vs the routed §9.1. The certification says the block "folds bondedRoot to B's
// value, silently dropping the slash's eviction" — that is right about the winner, but it understates
// the shape. The honest value equals NEITHER class's op. No class computes it, because no class ever
// sees another class's delta. This is not "the wrong one of two candidates won"; it is "the right
// answer was never a candidate". That is why the remedy direction is a single cross-class post-set
// threaded in apply() order (B → T → S) and NOT a dedup — deduping would just pick one of two values
// that are both wrong. (The remedy is research-gated and is not encoded here.)
//
// ────────────────────────────────────────────────────────────────────────────────────────
// WHY IT IS A WRONG-ACCEPT, AND WHY THE SHIPPED ABLATIONS MISS IT
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Measured against the HONEST committed root the box MISMATCHES and stalls — safe, and that stall is
// what every shipped compound-adjacent ablation would observe (ControlCompoundStallsAgainstTheHonestRoot
// below records it). But a real proposer does not commit the honest root. It commits whatever root
// ITS OWN FOLD produces from its own block, and the box's terminal equality
// (`if postRoot != committedStateRoot`) then passes by construction. MEASURED: recomputeViaHead
// returns nil against the box's own fold root, on both an S+B block (D1) and an S+T block (D2).
// That is box.Accept ∧ node.Reject — the same soundness failure as R-ANCHORED-PRE-SET, by a
// different lever, and the certified anchoring remedy does not close it: every pre-set here IS
// anchored and every duplicate op DOES verify. The defect is downstream of anchoring entirely.
//
// This is the scar the Tester already carries: scar-ablation-oracle-is-the-honest-artifact. An
// oracle built on the honest artifact models a VANDAL. The oracle below is the system's own output.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE ACCEPTED STATE IS INTERNALLY INCONSISTENT
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Class S's PER-MEMBER write-set is not affected by the collision — the per-member leaves live at
// distinct keys (bonded||id), so S's delete of bonded||culprit survives into the folded set. Only the
// whole-set digest scalar is overwritten. MEASURED in D3: the box certifies a post-state whose
// per-member leaf says the culprit is evicted while the bondedRoot digest that covers the membership
// still counts it. The two disagree inside one accepted root.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// REACHABILITY
// ────────────────────────────────────────────────────────────────────────────────────────
//
// stateRootScopeGate stalls only on b.IssuerKeys and on a TTL witness/scope disagreement; its own
// comment records that "Class B (bond regs, P1-d) / Class S (slashes, P1-b) are IN scope ... None is
// stalled here." Nothing forbids a compound block and apply() handles all of them — the controls
// below confirm the single-class blocks agree, so the fixtures are not degenerate.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// WHY THESE ARE PINS AND NOT ASSERTIONS
// ────────────────────────────────────────────────────────────────────────────────────────
//
// The remedy is research-gated (.claude/CLAUDE.md, the research gate: this is a consensus-rule
// surface). The Tester encodes a confirmed break; it does not fix one. So these land as
// PINNED_DEFECT under D-REPAIR-CLAIM-GATES-PINNED-2026-09-12: each asserts CURRENT BROKEN BEHAVIOUR,
// passes today, and REDDENS the moment the behaviour moves, which forces the record to be updated
// rather than letting the defect close silently. t.Skip is refused by that same decision.
//
// Each pin carries PIN + MECHANISM + TEETH, the idiom documented at the head of
// core/pipeline/rt_sfo_manifest_oracle_test.go:
//  1. the PIN — the symptom, via rtDigestCollisionPin: the box returned nil for the adversary's own root;
//  2. a MECHANISM arm — via rtDigestCollisionDuplicateOpPin: TWO ops with DIFFERENT NewValues still
//     land on the one digest key. A fix that moved the symptom without removing the duplicate
//     emission, or removed the duplicate without fixing the symptom, reddens here rather than
//     passing by coincidence;
//  3. TEETH — TestRTDigestCollision_PinsFireOnTheirRemediations, which feeds every predicate its
//     POST-FIX input and asserts it speaks up (simplicity rule 7).
//
// CONTAINMENT, so nobody reads this as "main is exploitable today": (*Box).Validate still applies the
// R1.8 downgrade — Accept ⇒ IndeterminateTrustlessly / ErrRecomputeGated. The box never accepts on
// this path yet. That downgrade is the only barrier between this finding and damage, and it is
// deliberately not touched here. The finding is what happens on the day R1.8 flips.

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// ══════════════════════════════════════════════════════════════════════════════
// THE PREDICATES — factored so their teeth can be asserted without merging a red test
// ══════════════════════════════════════════════════════════════════════════════

// rtDigestCollisionPin is the SYMPTOM predicate. It is handed the stall the recompute returned when
// measured against the root the box's OWN fold produced from the compound block. Empty string ⇒ the
// defect is still live (the box certified the proposer's root). Non-empty ⇒ the behaviour moved.
func rtDigestCollisionPin(defect string, stall error, boxRoot, honest ports.Hash, consequence string) string {
	if stall == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s PIN IS RED — the box now REJECTS the compound block measured against ITS OWN fold root: %v\n"+
			"  box    (the root the box's own fold produced) = %x\n"+
			"  honest (what real apply() yields)             = %x\n"+
			"  The pinned defect was: %s\n"+
			"  READ THIS BEFORE RE-PINNING. This pin asserts CURRENT BROKEN BEHAVIOUR (PINNED_DEFECT,\n"+
			"  D-REPAIR-CLAIM-GATES-PINNED-2026-09-12). A RED here is the EXPECTED outcome of the\n"+
			"  research-gated remedy: one cross-class post-set for bonded/qualified, threaded through\n"+
			"  S, B and T in apply() order (B -> T -> S), the way reconstructPostQualifiedWithWrites\n"+
			"  already does it for the class-P freeze and was never generalised to the digest ops.\n"+
			"  If that landed: retire this pin, replace it with the straight assertion that a compound\n"+
			"  block folds to the honest root, and KEEP the teeth. If it did NOT land, something else\n"+
			"  changed the emission and the census at the head of this file must be re-derived first.",
		defect, stall, boxRoot[:], honest[:], consequence)
}

// rtDigestCollisionDuplicateOpPin is the MECHANISM predicate: it asserts the CAUSE is still present,
// namely that MORE THAN ONE fold op lands on the one digest key with DIFFERENT NewValues. Empty
// string ⇒ cause still live.
//
// It also refuses the two vacuity shapes: a single op (the emission was unified, i.e. fixed) and
// duplicates that all carry the SAME NewValue (a duplicate that cannot diverge is not this defect).
func rtDigestCollisionDuplicateOpPin(defect, tag string, ops []statehash.FoldOp) string {
	want := statehash.Key(tag, nil)
	var vals [][]byte
	var idx []int
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) {
			vals = append(vals, ops[i].NewValue)
			idx = append(idx, i)
		}
	}
	name := rtDigestTagName(tag)
	if len(vals) < 2 {
		return fmt.Sprintf(
			"%s MECHANISM ARM IS RED — the box emitted %d fold op(s) for %q on this compound block, not 2+.\n"+
				"  The duplicate emission this pin holds is gone: the classes no longer each compute the\n"+
				"  whole-set digest from their own delta alone. That is the expected shape of the remedy.\n"+
				"  Re-derive the emission census at the head of this file (every digestFoldOp call site in\n"+
				"  assembleStateRootRecomputeOps' dispatch order) before re-pinning.",
			defect, len(vals), name)
	}
	allSame := true
	for i := 1; i < len(vals); i++ {
		if !bytes.Equal(vals[0], vals[i]) {
			allSame = false
			break
		}
	}
	if allSame {
		return fmt.Sprintf(
			"%s MECHANISM ARM IS RED — %d ops still land on %q (at slice positions %v) but they now all\n"+
				"  carry the SAME NewValue (%x), so last-write-wins can no longer change the folded root.\n"+
				"  The duplicate is inert. Either a cross-class post-set landed and every class now emits the\n"+
				"  same composed value, or the fixture stopped making the classes disagree — check which\n"+
				"  before re-pinning, because only the first is a fix.",
			defect, len(vals), name, idx, vals[0])
	}
	return ""
}

// rtDigestCollisionLastWriteWinsPin is the FOLD-PRIMITIVE predicate. Given the real compound ops, it
// asserts that statehash.FoldChangedPaths resolves the duplicate key by LAST WRITE, by folding two
// derived slices: one with every duplicate but the LAST removed, one with every duplicate but the
// FIRST removed. Empty string ⇒ last-write-wins still holds (cause live).
func rtDigestCollisionLastWriteWinsPin(defect string, prevRoot, full, keepFirst, keepLast ports.Hash) string {
	if full != keepLast {
		return fmt.Sprintf(
			"%s FOLD ARM IS RED — the full op slice no longer folds to the same root as keeping only the LAST\n"+
				"  duplicate (full=%x keepLast=%x). statehash.FoldChangedPaths no longer resolves a duplicate\n"+
				"  key by last-write-wins — it may now reject, merge, or order differently. Re-read\n"+
				"  FoldChangedPaths step (5) before re-pinning; this pin's whole premise moved.",
			defect, full[:], keepLast[:])
	}
	if keepFirst == keepLast {
		return fmt.Sprintf(
			"%s FOLD ARM IS VACUOUS — keeping the FIRST duplicate and keeping the LAST fold to the same root\n"+
				"  (%x). The two duplicate ops do not actually disagree, so last-write-wins is unobservable\n"+
				"  here and proves nothing. Fix the fixture so the classes compute different post-sets.",
			defect, keepFirst[:])
	}
	_ = prevRoot
	return ""
}

// rtDigestCollisionInconsistentStatePin is the INTERNAL-CONSISTENCY predicate: within the ops the box
// folds and certifies, the per-member leaf bonded||culprit is a DELETE while the bondedRoot digest
// the fold lands on still counts the culprit. Empty ⇒ the contradiction is still live.
func rtDigestCollisionInconsistentStatePin(defect string, perMemberIsDelete bool, digestCountsCulprit bool) string {
	if !perMemberIsDelete {
		return fmt.Sprintf(
			"%s CONSISTENCY ARM IS RED — the folded per-member leaf bonded||culprit is no longer a DELETE, so\n"+
				"  class S's per-member write-set changed. This pin compares the per-member leaf against the\n"+
				"  whole-set digest; if S stopped evicting per-member, re-derive stateRootSlashWriteSet before\n"+
				"  re-pinning.", defect)
	}
	if !digestCountsCulprit {
		return fmt.Sprintf(
			"%s CONSISTENCY ARM IS RED — the bondedRoot digest the fold lands on NO LONGER counts the slashed\n"+
				"  culprit, so the digest and the per-member leaves now agree. That is the expected outcome of\n"+
				"  the cross-class post-set remedy. Retire this pin and keep its teeth.", defect)
	}
	return ""
}

func rtDigestTagName(tag string) string { return string(bytes.TrimRight([]byte(tag), "\x00")) }

// ══════════════════════════════════════════════════════════════════════════════
// THE COMPOUND WITNESS — honest, complete, and built the way the box derives
// ══════════════════════════════════════════════════════════════════════════════

// rtDigestCompoundWitness builds the full honest witness for a block carrying any of E/R, Slashes and
// BondRegs, off the slashFixture chain. Every pre-set, screen, bucket and changed leaf is the TRUE
// committed pre-state — nothing here is forged. The break needs no forgery: an honest witness over a
// compound block is enough.
func rtDigestCompoundWitness(t *testing.T, f slashFixture, b Block, affectedBuckets []uint64) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	preBonded := idSet(f.preIDsBonded())
	preQualified := idSet(f.preIDsQualified())
	preSlashed := idSet(f.preIDsSlashed())

	for _, wr := range stateRootSlashWriteSet(b, preBonded, preQualified) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
	}

	screens := map[ports.Hash]StateRootBondRegScreen{}
	for _, r := range b.BondRegs {
		owner, claimed := f.c.bondRootOwner[r.Root]
		sc := StateRootBondRegScreen{
			Root:        r.Root,
			PriorOwner:  owner,
			Claimed:     claimed,
			PriorProven: f.c.bondRootProven[r.Root],
			OwnerProof:  mustProve(f.prover, statehash.Key(tagBondRootOwner, r.Root[:])),
			ProvenProof: mustProve(f.prover, statehash.Key(tagBondRootProven, r.Root[:])),
		}
		w.BondRegScreens = append(w.BondRegScreens, sc)
		screens[r.Root] = sc
	}

	for _, d := range affectedBuckets {
		var hk [8]byte
		putUint64BE(hk[:], d)
		k := statehash.Key(tagDueBucket, hk[:])
		pre := []ports.NodeID{}
		for id := range f.c.dueBucket[d] {
			pre = append(pre, id)
		}
		pre = sortIDs(pre)
		if len(pre) == 0 {
			wit, err := f.prover.Prove(k)
			if err != nil {
				t.Fatalf("Prove(bucket %d): %v", d, err)
			}
			w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{DueHeight: d, PreMembers: nil, Proof: wit})
			continue
		}
		wit, sibs, err := f.prover.ProveWithSiblings(k)
		if err != nil {
			t.Fatalf("ProveWithSiblings(bucket %d): %v", d, err)
		}
		w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{DueHeight: d, PreMembers: pre, Proof: wit, DeleteSiblings: sibs})
	}

	if len(b.BondRegs) > 0 {
		preBRH := map[ports.NodeID]uint64{}
		for id, h := range f.c.bondRegHeight {
			preBRH[id] = h
		}
		delta, err := f.c.stateRootBondRegWriteSet(f.prevRoot, b, preBonded, preQualified, preSlashed, screens, preBRH)
		if err != nil {
			t.Fatalf("stateRootBondRegWriteSet: %v", err)
		}
		for _, wr := range delta.writes {
			w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
		}
	}

	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, err := f.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(dueBucket scope): %v", err)
		}
		w.DueBucketProof = dp
	}
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	return w
}

// rtDigestSBBlock returns the S+B compound block (one slash of the fixture culprit, one fresh bond
// registration, one E/R entry) plus its honest witness and the due-bucket the registration lands in.
func rtDigestSBBlock(t *testing.T, f slashFixture) (Block, StateRootWitness) {
	t.Helper()
	prev, h := f.c.Head()
	fresh := key(91001)
	b := Block{
		Version:  BlockVersionWitnessable,
		Height:   h,
		Prev:     prev,
		Entries:  []ports.Entry{entry(40)},
		Slashes:  []Equivocation{slashProof(f.culprit, prev, 0x41, 0x42)},
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)},
	}
	return b, rtDigestCompoundWitness(t, f, b, []uint64{h + f.c.cfg.BondTTLBlocks + 1})
}

// rtDigestFoldOrStall folds ops and fails the test on a fold error — a fold that stalls is a
// different finding, not this one.
func rtDigestFoldOrStall(t *testing.T, prevRoot ports.Hash, ops []statehash.FoldOp) ports.Hash {
	t.Helper()
	root, err := statehash.FoldChangedPaths(prevRoot, ops)
	if err != nil {
		t.Fatalf("FoldChangedPaths stalled on an HONEST compound witness: %v\n"+
			"  Every duplicate digest op carries the same OldValue and the same Proof, so all of them\n"+
			"  must verify at step (1). A stall here means the fold changed and this pin's premise moved.", err)
	}
	return root
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-DIGEST-0 — the emission census, at the unit level
// ══════════════════════════════════════════════════════════════════════════════

// TestRTDigestCollision_D0_TwoClassesEmitTheSameDigestKey_PINNED_DEFECT pins the CAUSE on its own:
// on one compound S+B block the box emits TWO ops for tagBondedRoot and TWO for tagQualifiedRoot, at
// the identical key, with different NewValues and a byte-identical OldValue and Proof.
func TestRTDigestCollision_D0_TwoClassesEmitTheSameDigestKey_PINNED_DEFECT(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := rtDigestSBBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assembleStateRootRecomputeOps stalled on an honest compound block: %v", err)
	}

	for _, tag := range []string{tagBondedRoot, tagQualifiedRoot} {
		if msg := rtDigestCollisionDuplicateOpPin("RT-DIGEST-0", tag, ops); msg != "" {
			t.Fatalf("%s", msg)
		}
		// The duplicates must share OldValue and Proof, else they would not BOTH verify and the
		// break would be a stall instead of a wrong-accept.
		want := statehash.Key(tag, nil)
		var first *statehash.FoldOp
		for i := range ops {
			if !bytes.Equal(ops[i].Key, want) {
				continue
			}
			if first == nil {
				first = &ops[i]
				continue
			}
			if !bytes.Equal(first.OldValue, ops[i].OldValue) {
				t.Fatalf("RT-DIGEST-0 PIN IS RED — the duplicate ops for %q no longer share an OldValue\n"+
					"  (%x vs %x). They would no longer both verify against prevStateRoot, so the defect\n"+
					"  would surface as a STALL rather than a wrong-accept. Re-derive digestFoldOp before re-pinning.",
					rtDigestTagName(tag), first.OldValue, ops[i].OldValue)
			}
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-DIGEST-1 — S+B: the box certifies its own fold root and the slash eviction is dropped
// ══════════════════════════════════════════════════════════════════════════════

func TestRTDigestCollision_D1_CompoundSlashPlusBondRegDropsTheEviction_PINNED_DEFECT(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := rtDigestSBBlock(t, f)

	// GROUND TRUTH: the real transition, not a model of it.
	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	culprit := ports.HashBytes(pubOf(f.culprit))
	if _, stillBonded := clone.bonded[culprit]; stillBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() left the slashed culprit BONDED, so there is no\n" +
			"  eviction for the collision to drop. Re-check buildSlashFixture and the slash payload.")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assembleStateRootRecomputeOps stalled on an honest compound block: %v", err)
	}

	// THE ORACLE IS THE BOX'S OWN OUTPUT, never the honest committed root. A real proposer commits
	// what its own fold produced; measuring against the honest root models a vandal and stalls.
	boxRoot := rtDigestFoldOrStall(t, f.prevRoot, ops)
	if boxRoot == honest {
		t.Fatalf("RT-DIGEST-1 IS VACUOUS — the box's own fold agrees with real apply() (%x), so there is\n"+
			"  no divergence to exploit. The collision must change the folded root for this pin to mean\n"+
			"  anything; check that class B still emits its bondedRoot op on this block.", boxRoot[:])
	}

	// MECHANISM arm: the duplicate emission is why.
	if msg := rtDigestCollisionDuplicateOpPin("RT-DIGEST-1", tagBondedRoot, ops); msg != "" {
		t.Fatalf("%s", msg)
	}

	// PIN: the box certifies the proposer's root.
	stall := recomputeViaHead(f.c, f.prevRoot, boxRoot, b, w)
	if msg := rtDigestCollisionPin("RT-DIGEST-1", stall, boxRoot, honest,
		"a block carrying BOTH Slashes and BondRegs folds bondedRoot/qualifiedRoot to class B's value, "+
			"dropping the slash's eviction from the whole-set digest, and the box accepts the resulting "+
			"root because the proposer committed it"); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-DIGEST-2 — S+T: the SAME defect with a different winner, so it is an ORDERING defect
// ══════════════════════════════════════════════════════════════════════════════

// The append order is S → B → T. On an S+T block class T is last, so T's value wins and the SLASH is
// the change that gets dropped. Recording a second class pair with a different winner is the point:
// it shows the defect is the shared key plus the append order, not a quirk of class B.
func TestRTDigestCollision_D2_CompoundSlashPlusTTLSweepDropsTheEviction_PINNED_DEFECT(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	prev, _ := f.c.Head()
	b.Slashes = []Equivocation{slashProof(f.proposer, prev, 0x41, 0x42)}

	expired := f.expiredMembers()
	if len(expired) == 0 {
		t.Fatalf("FIXTURE IS VACUOUS — no expired members in dueBucket[%d], so class T does not dispatch", f.sweepH)
	}
	w := f.ttlSweepWitness(t, b, expired)
	preBonded := idSet(f.preIDsBonded())
	preQualified := idSet(f.preIDsQualified())
	for _, wr := range stateRootSlashWriteSet(b, preBonded, preQualified) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	culprit := ports.HashBytes(pubOf(f.proposer))
	if _, stillBonded := clone.bonded[culprit]; stillBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() left the slashed culprit BONDED on the S+T block")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assembleStateRootRecomputeOps stalled on an honest S+T block: %v", err)
	}
	boxRoot := rtDigestFoldOrStall(t, f.prevRoot, ops)
	if boxRoot == honest {
		t.Fatalf("RT-DIGEST-2 IS VACUOUS — the box's own fold agrees with real apply() (%x)", boxRoot[:])
	}
	if msg := rtDigestCollisionDuplicateOpPin("RT-DIGEST-2", tagBondedRoot, ops); msg != "" {
		t.Fatalf("%s", msg)
	}
	stall := recomputeViaHead(f.c, f.prevRoot, boxRoot, b, w)
	if msg := rtDigestCollisionPin("RT-DIGEST-2", stall, boxRoot, honest,
		"a block carrying BOTH Slashes and a firing TTL sweep folds bondedRoot to class T's value "+
			"(T is appended after S), dropping the slash's eviction, and the box accepts the resulting root"); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-DIGEST-3 — the accepted state contradicts itself
// ══════════════════════════════════════════════════════════════════════════════

func TestRTDigestCollision_D3_FoldedDigestContradictsItsOwnPerMemberLeaf_PINNED_DEFECT(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := rtDigestSBBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	culprit := ports.HashBytes(pubOf(f.culprit))

	// The per-member arm: bonded||culprit is present in the folded op set as a DELETE.
	perMemberKey := statehash.Key(tagBonded, culprit[:])
	perMemberIsDelete := false
	found := false
	for i := range ops {
		if bytes.Equal(ops[i].Key, perMemberKey) {
			found = true
			perMemberIsDelete = ops[i].NewValue == nil
		}
	}
	if !found {
		t.Fatalf("RT-DIGEST-3 PIN IS RED — no folded op for the per-member leaf bonded||culprit at all.\n" +
			"  Class S's per-member write-set changed shape; re-derive stateRootSlashWriteSet before re-pinning.")
	}

	// The digest arm: the value the fold LANDS on still counts the culprit. The fold takes the last
	// duplicate, so recompute what that value's set is by comparing against nodeSetMTH of the two
	// candidate sets.
	want := statehash.Key(tagBondedRoot, nil)
	var landed []byte
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) {
			landed = ops[i].NewValue
		}
	}
	pre := idSet(f.preIDsBonded())
	withCulprit := cloneIDSet(pre)
	withCulprit[ports.HashBytes(pubOf(key(91001)))] = struct{}{}
	digestCountsCulprit := bytes.Equal(landed, rtDigestMTH(withCulprit))

	if msg := rtDigestCollisionInconsistentStatePin("RT-DIGEST-3", perMemberIsDelete, digestCountsCulprit); msg != "" {
		t.Fatalf("%s", msg)
	}
}

func rtDigestMTH(m map[ports.NodeID]struct{}) []byte {
	ids := make([]ports.NodeID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return nodeSetMTH(ids)
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-DIGEST-4 — the fold primitive resolves the duplicate by LAST WRITE
// ══════════════════════════════════════════════════════════════════════════════

func TestRTDigestCollision_D4_FoldChangedPathsResolvesDuplicatesByLastWrite_PINNED_DEFECT(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := rtDigestSBBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if msg := rtDigestCollisionDuplicateOpPin("RT-DIGEST-4", tagBondedRoot, ops); msg != "" {
		t.Fatalf("%s", msg)
	}

	full := rtDigestFoldOrStall(t, f.prevRoot, ops)
	keepFirst := rtDigestFoldOrStall(t, f.prevRoot, rtDigestKeepDuplicate(ops, tagBondedRoot, true))
	keepLast := rtDigestFoldOrStall(t, f.prevRoot, rtDigestKeepDuplicate(ops, tagBondedRoot, false))

	if msg := rtDigestCollisionLastWriteWinsPin("RT-DIGEST-4", f.prevRoot, full, keepFirst, keepLast); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// rtDigestKeepDuplicate returns a copy of ops keeping only the FIRST (first=true) or the LAST
// (first=false) op on Key(tag, nil), preserving the order of everything else.
func rtDigestKeepDuplicate(ops []statehash.FoldOp, tag string, first bool) []statehash.FoldOp {
	want := statehash.Key(tag, nil)
	var hits []int
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) {
			hits = append(hits, i)
		}
	}
	keep := hits[len(hits)-1]
	if first {
		keep = hits[0]
	}
	out := make([]statehash.FoldOp, 0, len(ops))
	for i := range ops {
		if bytes.Equal(ops[i].Key, want) && i != keep {
			continue
		}
		out = append(out, ops[i])
	}
	return out
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTROLS — the fixtures are not degenerate, and the honest-root oracle is why this was missed
// ══════════════════════════════════════════════════════════════════════════════

// TestRTDigestCollision_ControlSingleClassBlocksAgree is the non-vacuity control: the SAME witness
// builder, on a slash-ONLY block and a bondreg-ONLY block, agrees with real apply(). The divergence
// in D1 is produced by the compounding, not by a broken fixture or a malformed witness.
func TestRTDigestCollision_ControlSingleClassBlocksAgree(t *testing.T) {
	t.Run("slash-only", func(t *testing.T) {
		f := buildSlashFixture(t)
		prev, h := f.c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			Entries: []ports.Entry{entry(40)},
			Slashes: []Equivocation{slashProof(f.culprit, prev, 0x41, 0x42)}}
		w := rtDigestCompoundWitness(t, f, b, nil)
		honest := f.applyAndCommittedRoot(t, b)
		if err := recomputeViaHead(f.c, f.prevRoot, honest, b, w); err != nil {
			t.Fatalf("CONTROL FAILED — a slash-ONLY block must agree with real apply(), got %v.\n"+
				"  Without this the compound divergence could be an artifact of the witness builder.", err)
		}
	})
	t.Run("bondreg-only", func(t *testing.T) {
		f := buildSlashFixture(t)
		prev, h := f.c.Head()
		fresh := key(91001)
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			Entries:  []ports.Entry{entry(40)},
			BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
		w := rtDigestCompoundWitness(t, f, b, []uint64{h + f.c.cfg.BondTTLBlocks + 1})
		honest := f.applyAndCommittedRoot(t, b)
		if err := recomputeViaHead(f.c, f.prevRoot, honest, b, w); err != nil {
			t.Fatalf("CONTROL FAILED — a bondreg-ONLY block must agree with real apply(), got %v", err)
		}
	})
}

// TestRTDigestCollision_ControlCompoundStallsAgainstTheHonestRoot records WHY the shipped ablations in
// this package never saw this: measured against the HONEST committed root the compound block STALLS,
// and a stall reads as "the box is sound". It is not — the box simply folded to a different root than
// the one it was handed. Only the box's OWN output is a sound oracle here
// (scar-ablation-oracle-is-the-honest-artifact).
func TestRTDigestCollision_ControlCompoundStallsAgainstTheHonestRoot(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := rtDigestSBBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	if err := recomputeViaHead(f.c, f.prevRoot, honest, b, w); err == nil {
		t.Fatalf("CONTROL IS RED — the compound block now AGREES with the honest root. If that is because\n" +
			"  the cross-class post-set landed, this whole file must be retired, not just this control.")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// TEETH — every predicate is fed its POST-FIX input and must speak up
// ══════════════════════════════════════════════════════════════════════════════

func TestRTDigestCollision_PinsFireOnTheirRemediations(t *testing.T) {
	var a, bh ports.Hash
	a[0], bh[0] = 1, 2

	t.Run("symptom-pin-fires-when-the-box-rejects", func(t *testing.T) {
		if msg := rtDigestCollisionPin("X", fmt.Errorf("stalled"), a, bh, "c"); msg == "" {
			t.Fatalf("TEETH FAILED: rtDigestCollisionPin stayed silent on a non-nil stall")
		}
		if msg := rtDigestCollisionPin("X", nil, a, bh, "c"); msg != "" {
			t.Fatalf("TEETH FAILED: rtDigestCollisionPin spoke on the live-defect input: %s", msg)
		}
	})

	t.Run("mechanism-pin-fires-on-a-unified-emission", func(t *testing.T) {
		k := statehash.Key(tagBondedRoot, nil)
		one := []statehash.FoldOp{{Key: k, NewValue: []byte{1}}}
		if msg := rtDigestCollisionDuplicateOpPin("X", tagBondedRoot, one); msg == "" {
			t.Fatalf("TEETH FAILED: the mechanism pin stayed silent on a SINGLE op (the fixed shape)")
		}
		none := []statehash.FoldOp{{Key: statehash.Key(tagSlashedRoot, nil), NewValue: []byte{1}}}
		if msg := rtDigestCollisionDuplicateOpPin("X", tagBondedRoot, none); msg == "" {
			t.Fatalf("TEETH FAILED: the mechanism pin stayed silent on ZERO ops")
		}
		same := []statehash.FoldOp{{Key: k, NewValue: []byte{1}}, {Key: k, NewValue: []byte{1}}}
		if msg := rtDigestCollisionDuplicateOpPin("X", tagBondedRoot, same); msg == "" {
			t.Fatalf("TEETH FAILED: the mechanism pin stayed silent on two IDENTICAL NewValues (inert duplicate)")
		}
		live := []statehash.FoldOp{{Key: k, NewValue: []byte{1}}, {Key: k, NewValue: []byte{2}}}
		if msg := rtDigestCollisionDuplicateOpPin("X", tagBondedRoot, live); msg != "" {
			t.Fatalf("TEETH FAILED: the mechanism pin spoke on the live-defect input: %s", msg)
		}
	})

	t.Run("fold-pin-fires-when-last-write-no-longer-wins", func(t *testing.T) {
		var c ports.Hash
		c[0] = 3
		if msg := rtDigestCollisionLastWriteWinsPin("X", a, a, bh, c); msg == "" {
			t.Fatalf("TEETH FAILED: the fold pin stayed silent when full != keepLast")
		}
		if msg := rtDigestCollisionLastWriteWinsPin("X", a, bh, bh, bh); msg == "" {
			t.Fatalf("TEETH FAILED: the fold pin stayed silent when keepFirst == keepLast (vacuous)")
		}
		if msg := rtDigestCollisionLastWriteWinsPin("X", a, c, bh, c); msg != "" {
			t.Fatalf("TEETH FAILED: the fold pin spoke on the live-defect input: %s", msg)
		}
	})

	t.Run("consistency-pin-fires-when-the-digest-agrees", func(t *testing.T) {
		if msg := rtDigestCollisionInconsistentStatePin("X", true, false); msg == "" {
			t.Fatalf("TEETH FAILED: the consistency pin stayed silent when the digest stopped counting the culprit")
		}
		if msg := rtDigestCollisionInconsistentStatePin("X", false, true); msg == "" {
			t.Fatalf("TEETH FAILED: the consistency pin stayed silent when the per-member leaf stopped being a DELETE")
		}
		if msg := rtDigestCollisionInconsistentStatePin("X", true, true); msg != "" {
			t.Fatalf("TEETH FAILED: the consistency pin spoke on the live-defect input: %s", msg)
		}
	})

	t.Run("keep-duplicate-helper-actually-drops-one", func(t *testing.T) {
		k := statehash.Key(tagBondedRoot, nil)
		other := statehash.Key(tagSlashedRoot, nil)
		in := []statehash.FoldOp{
			{Key: k, NewValue: []byte{1}},
			{Key: other, NewValue: []byte{9}},
			{Key: k, NewValue: []byte{2}},
		}
		gotFirst := rtDigestKeepDuplicate(in, tagBondedRoot, true)
		gotLast := rtDigestKeepDuplicate(in, tagBondedRoot, false)
		if len(gotFirst) != 2 || len(gotLast) != 2 {
			t.Fatalf("TEETH FAILED: rtDigestKeepDuplicate did not drop exactly one op (first=%d last=%d)",
				len(gotFirst), len(gotLast))
		}
		if !bytes.Equal(gotFirst[0].NewValue, []byte{1}) || !bytes.Equal(gotLast[1].NewValue, []byte{2}) {
			t.Fatalf("TEETH FAILED: rtDigestKeepDuplicate kept the wrong op (first=%x last=%x)",
				gotFirst[0].NewValue, gotLast[1].NewValue)
		}
	})
}
