package chain

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// D0 — THE COLD AUDITOR. The driven never-Accept suite.
// =============================================================================
//
// Governing: ROADMAP Lane D row D0; owner call 2 of D-TRUE-UP-CALLS-2026-09-07, ratified
// 2026-09-07 on direction (a') of
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// Part 2 (conditions H-1…H-4, advisory H-5); D-RECOMPUTE-FREEZE, which cut this row's dependency
// on the recompute spine and made it the freeze's whole floor-box gate.
// Deliberation: docs/thinking/2026-09-09-d0-cold-auditor.md
//
// WHY THIS FILE EXISTS. D0 is five subtractions and a claim: "the floor box is a cold auditor —
// it stalls, and it never Accepts." Simplicity rule 7 says a green gate with no demonstrated red is
// decoration, and a row whose deliverable is a deletion is exactly where that rule bites: nothing
// is easier to ship than a safety property nobody drove a block at. So every v5 block class is
// driven THROUGH THE DOOR here, twice — once with the honest committed root and once with a
// DIVERGENT one — and the door's verdict is asserted to be never Accept, by name.
//
// THE ATTACK EACH ARM RUNS. Build a block of the class that the NODE ITSELF accepts (the oracle
// runs first, so no arm can pass on a block that was never valid), then commit a StateRoot that is
// NOT the one that payload produces, and re-sign and re-certify it so the forgery is invisible on
// the block's face — a fully valid-looking v5 block that lies only about the post-state. The
// attacker owns the payload, the root, the proposer signature and the whole quorum. The box owns
// its parent block and its own config, and that is what it stalls on.
//
// WHAT THIS FILE IS NOT. It does not forge WITNESSES. That is the recompute's adversarial-root
// ladder (floorbox_recompute_adversarialroot_v5_test.go — driven per field across classes P/A/B/M
// with its own completeness meta-test), which D-RECOMPUTE-FREEZE froze. Re-deriving it at the door
// would buy no property and would grow the keystone the freeze exists to stop growing.
//
// NON-VACUITY (NG-2). This file is in twinGateFiles, so every Test* here calls
// assertBoxReachesTheDowngrade directly: the fixture's honest block must run the WHOLE composition
// through the box to the R1.8 downgrade. A box that stalled on everything would satisfy
// "never Accept" trivially, and that is the failure mode the twin catches.

// coldAuditorClass is one v5 block class: the payload that makes a block a member of it, the
// Block field it drives (which the coverage meta-test reflects over), and the refusal the door is
// expected to name for an honest-rooted block of the class.
//
// wantReason is a SENTINEL where the class has one and a substring where the class's refusal is
// formatted rather than wrapped. Both are asserted; neither is optional. Asserting the NAME, not
// merely "not Accept", is what keeps the arms distinct — the measurement this suite was designed
// against showed every class stalling for a DIFFERENT reason, and an arm that stopped exercising
// its own class would land on somebody else's name.
type coldAuditorClass struct {
	name       string
	blockField string // the Block field this class drives; "" = a block SHAPE, not a payload field
	mutate     func(t *testing.T, f structFixture, b *Block)
	nodeAccept bool   // the node's own oracle verdict for the honest twin of this class
	wantSentry error  // errors.Is target, or nil
	wantSubstr string // substring of the refusal text, or ""
}

// coldAuditorClasses is the driven class table. Every Hash-committed payload field of a v5 Block
// that a proposer can populate appears here or in coldAuditorUndriven, with a reason.
func coldAuditorClasses() []coldAuditorClass {
	return []coldAuditorClass{
		{
			name: "entries", blockField: "Entries", nodeAccept: true,
			mutate:     func(t *testing.T, f structFixture, b *Block) { b.Entries = []ports.Entry{entry(0x51), entry(0x52)} },
			wantSentry: ErrRecomputeGated,
		},
		{
			name: "revocations", blockField: "Revocations", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				b.Revocations = []ports.Hash{entry(1).Root} // the root the fixture committed at h1
			},
			wantSubstr: "tagRevLogSize",
		},
		{
			name: "unrevocations", blockField: "Unrevocations", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				b.Unrevocations = []ports.Hash{entry(1).Root} // revoked by the fixture's own h2 block
			},
			wantSubstr: "tagRevLogSize",
		},
		{
			name: "bondregs", blockField: "BondRegs", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				b.BondRegs = []BondReg{bondReg(f.keys[0], 4<<20, b.Prev)}
			},
			wantSubstr: "bondedRoot",
		},
		{
			name: "slashes", blockField: "Slashes", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				b.Slashes = []Equivocation{slashProof(f.keys[3], b.Prev, 0x41, 0x42)}
			},
			wantSubstr: "slashedRoot",
		},
		{
			name: "issuerkeys", blockField: "IssuerKeys", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				b.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(f.keys[0], 0, ports.Hash{0x77})}
			},
			wantSentry: ErrRecomputeStateRootScopeStall,
			wantSubstr: "issuerKeyCommit",
		},
		{
			name: "carrier", blockField: "LastCommit", nodeAccept: true,
			mutate: func(t *testing.T, f structFixture, b *Block) {
				// The carrier republishes the PARENT's precommits, and the parent is the block the
				// box holds — not the fixture's h1. The oracle caught the h1 version of this arm.
				b.LastCommit = f.c.Blocks(b.Height - 1)[0].Atts
			},
			wantSubstr: "validatorsSeenRoot",
		},
	}
}

// coldAuditorUndriven is every remaining exported Block field with the written reason the class
// table does not drive it. The coverage meta-test requires the union to be EXACTLY Block's
// exported fields, so a new payload field reddens rather than silently escaping the suite.
var coldAuditorUndriven = map[string]string{
	"Height":      "position, not a payload class; P1 binds it to the box's OWN head before any other read, and TestG3_ParentBindingPrecedesTheCarrierLeg drives that",
	"Prev":        "position, not a payload class; bound to the box's own head at P1 (same gate)",
	"Proposer":    "identity, not a payload class; NewBox refuses a parent whose proposer signature does not verify, and the proposer screens run in the composition",
	"ProposerSig": "the signature over the class payloads above; every arm here re-signs after forging, so it is exercised by all of them",
	"Version":     "the class SELECTOR, not a class: TestWitnessValidateV5_SubV5BlockRejected drives every sub-v5 version to Reject",
	"CommitRound": "excluded from Hash — a certificate slot a replica may hold differently, not committed state",
	"PrepareQC":   "excluded from Hash; the quorum stacks C1..C5 read it, and every arm here carries a real one",
	"Atts":        "excluded from Hash; same as PrepareQC, and the carrier arm drives the parent's copy as committed state",
	"Pruned":      "a block SHAPE, driven by TestColdAuditor_RefusesPrunedBlocks at BOTH box entries and at NewBox",
	"StateRoot":   "the quantity every arm FORGES; it is the divergent root, not a class of its own",
	"LogRoot":     "forged alongside StateRoot by the same helper on every arm",
}

// coldAuditorFixture is the struct fixture with one extra committed block, so the pre-state holds a
// revoked root (the un-revocation class needs one) and the box's parent is a real post-apply state.
// The box holds the fixture head as its parent; the block under test sits one above it.
func coldAuditorFixture(t *testing.T) (structFixture, *proverSource, *Box) {
	t.Helper()
	f := buildStructFixture(t)
	rev := f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{entry(1).Root} })
	if err := f.c.Append(rev); err != nil {
		t.Fatalf("fixture: the revocation block must COMMIT (the un-revocation class needs a revoked root): %v", err)
	}
	src := newProverSource(t, f.c)
	parent := f.c.Blocks(rev.Height)[0]
	box, err := NewBox(f.c, parent, BoxConfig{BudgetBytes: 1 << 22}, src)
	if err != nil {
		t.Fatalf("NewBox: %v", err)
	}
	return f, src, box
}

// forgeDivergentRoot is the attack: replace the block's committed StateRoot with one it does NOT
// produce, then re-sign and re-certify so the lie is invisible on the block's face. The forged
// block carries a genuine proposer signature over the forged root and a genuine quorum's prepare
// and precommit certificates over it — the attacker controls every one of those, and the point of
// the state-root commitment is that controlling them is not enough.
func forgeDivergentRoot(t *testing.T, f structFixture, honest Block) Block {
	t.Helper()
	if honest.StateRoot == nil {
		t.Fatal("fixture: an honest v5 block must commit a StateRoot")
	}
	b := honest
	forged := *honest.StateRoot
	forged[0] ^= 0xFF
	if forged == *honest.StateRoot {
		t.Fatal("the forged root must differ from the honest one")
	}
	b.StateRoot = &forged
	b.Atts, b.PrepareQC, b.ProposerSig = nil, nil, nil
	b.hashMemoSet = false
	Sign(&b, f.keys[0])
	for _, k := range f.keys {
		b.PrepareQC = append(b.PrepareQC, AttestAt(&b, k, 0, PhasePrepare))
		b.Atts = append(b.Atts, AttestAt(&b, k, 0, PhasePrecommit))
	}
	if h, hh := b.Hash(), honest.Hash(); h == hh {
		t.Fatal("the forged block must not hash identically to the honest one")
	}
	return b
}

// TestColdAuditor_NeverAcceptsAnyV5BlockClass is D0's suite. For every v5 block class: the node's
// own oracle first, then the honest-rooted block through the door (verdict never Accept, refusal
// named), then the same block with a DIVERGENT committed root, re-signed and re-certified (verdict
// never Accept).
//
// ABLATION (the whole table): delete the R1.8 downgrade in (*Box).Validate — `if out == Accept {
// return IndeterminateTrustlessly, ErrRecomputeGated }` — and the entries arm returns ACCEPT ⇒ RED.
// Per-class ablations, each of which reddens exactly one arm by changing the name it lands on:
// remove the `len(b.IssuerKeys) > 0` clause from stateRootScopeGate (issuerkeys); remove the digest
// pre-set requirement from the class-S/B/A digest reconstruction (bondregs, slashes, carrier);
// remove the tagRevLogSize stall (revocations, unrevocations).
func TestColdAuditor_NeverAcceptsAnyV5BlockClass(t *testing.T) {
	// NG-2: the door can reach the far end. Everything below is a refusal, and a door that stalls
	// on everything would pass every one of them for the wrong reason.
	tf := buildStructFixture(t)
	tsrc := newProverSource(t, tf.c)
	honestTwin := tf.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, tf, tsrc), honestTwin, structWitnessFor(t, tf, tsrc, honestTwin))

	for _, cls := range coldAuditorClasses() {
		t.Run(cls.name, func(t *testing.T) {
			f, src, box := coldAuditorFixture(t)
			honest := f.mkBlock(t, func(b *Block) { cls.mutate(t, f, b) })

			// The ORACLE runs first: this is a real v5 block of this class, not junk that would be
			// refused for a reason having nothing to do with the class.
			if err := f.c.ValidateCommit(&honest); (err == nil) != cls.nodeAccept {
				t.Fatalf("ORACLE: the node's verdict on the honest %s block is %v, want accept=%v — "+
					"the arm is not driving the class it claims to", cls.name, err, cls.nodeAccept)
			}

			w := structWitnessFor(t, f, src, honest)
			out, err := box.Validate(honest, w)
			assertNeverAccept(t, cls.name+"/honest-root", out, err)
			assertRefusalNamed(t, cls, out, err)

			// The attack: a divergent committed root, re-signed and re-certified.
			forged := forgeDivergentRoot(t, f, honest)
			fout, ferr := box.Validate(forged, structWitnessFor(t, f, src, forged))
			assertNeverAccept(t, cls.name+"/divergent-root", fout, ferr)
		})
	}
}

// assertNeverAccept is D0's property, stated once.
func assertNeverAccept(t *testing.T, label string, out FloorBoxOutcome, err error) {
	t.Helper()
	if out == Accept {
		t.Fatalf("SAFETY VIOLATION (%s): the cold auditor returned ACCEPT (reason %v). The box never "+
			"Accepts until R1.8 flips it, which is a consensus-rule change (I1), owner-ratified and "+
			"research-gated — and this row is not it.", label, err)
	}
	if err == nil {
		t.Fatalf("%s: a non-Accept verdict (%s) must carry a LOUD reason; got nil. A silent stall is "+
			"the failure mode the cold auditor exists to rule out.", label, out)
	}
}

// assertRefusalNamed keeps the arms distinct: each class must land on its OWN refusal.
func assertRefusalNamed(t *testing.T, cls coldAuditorClass, out FloorBoxOutcome, err error) {
	t.Helper()
	if cls.wantSentry != nil && !errors.Is(err, cls.wantSentry) {
		t.Fatalf("%s: refusal should wrap %v; got %s / %v. If this arm has landed on another class's "+
			"name it is no longer driving its own.", cls.name, cls.wantSentry, out, err)
	}
	if cls.wantSubstr != "" && !strings.Contains(err.Error(), cls.wantSubstr) {
		t.Fatalf("%s: refusal should name %q; got %s / %v", cls.name, cls.wantSubstr, out, err)
	}
}

// TestColdAuditor_ClassCoverageIsComplete is the completeness meta-test. It reflects over Block's
// exported fields and requires each to be either driven by a class in coldAuditorClasses or listed
// in coldAuditorUndriven with a reason. An un-driven "safe" row is what simplicity rule 7 forbids,
// and a hand-written list of classes is how a suite quietly stops covering the type it is about.
//
// ABLATION: add an exported field to Block and drive nothing ⇒ RED naming the field; delete a class
// from coldAuditorClasses without listing its field ⇒ RED the same way.
// SOURCE GATE: none — this reads the Block TYPE by reflection, so it sees field names and nothing
// else (not their semantics, not whether an arm's mutate actually populates them). RUNTIME GATE:
// TestColdAuditor_NeverAcceptsAnyV5BlockClass, whose per-arm ORACLE call proves each mutate
// produces a real block of its class, and whose named-refusal assertion proves the arms are
// distinct.
func TestColdAuditor_ClassCoverageIsComplete(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	b := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), b, structWitnessFor(t, f, src, b))

	driven := map[string]string{}
	for _, cls := range coldAuditorClasses() {
		if cls.blockField == "" {
			continue
		}
		if prev, dup := driven[cls.blockField]; dup {
			t.Fatalf("class table: %s and %s both claim Block.%s; one field, one class",
				prev, cls.name, cls.blockField)
		}
		driven[cls.blockField] = cls.name
	}

	var missing, stale []string
	fields := map[string]bool{}
	bt := reflect.TypeOf(Block{})
	for i := 0; i < bt.NumField(); i++ {
		fd := bt.Field(i)
		if fd.PkgPath != "" {
			continue // unexported: not a proposer-populated field
		}
		fields[fd.Name] = true
		if driven[fd.Name] == "" && coldAuditorUndriven[fd.Name] == "" {
			missing = append(missing, fd.Name)
		}
	}
	for name := range coldAuditorUndriven {
		if !fields[name] {
			stale = append(stale, name)
		}
	}
	for name := range driven {
		if !fields[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Fatalf("COVERAGE: Block field(s) %v are neither driven by a class in coldAuditorClasses nor "+
			"listed in coldAuditorUndriven with a reason. A new v5 block field is a new block class: "+
			"drive it through the door, or write down why it is not one.", missing)
	}
	if len(stale) > 0 {
		t.Fatalf("COVERAGE: %v are named by the class table or the undriven list but are not Block "+
			"fields any more — the suite is asserting against a type that has moved", stale)
	}
}

// TestColdAuditor_StallsUnconditionallyAtARecoveryBoundary is D0's first deliverable at THE DOOR.
// At an ambiguous #535 recovery boundary the box stalls loudly, and there is nothing a caller can
// supply to change that: BoxConfig carries no directive, the box holds no flag, and Validate takes
// no per-block authorization. The three paths (a') removed are gone from the type system, so what
// is left to drive is that the stall fires at the boundary and does not fire away from it.
//
// The stall is TERMINAL, not per-block (certification §2.1): the box needs a verified parent state
// root for H+1 and its only source is H's committed StateRoot, the quantity it just declined to
// reproduce. Recovery is the operator's -ws-checkpoint re-anchor at H+1, and an unreachable pin is
// a critical and irrecoverable failure (§2.3 clause 3).
//
// ABLATION: restore any escape past recoveryBoundaryDecision — e.g. `return true, nil` — ⇒ the
// boundary block reaches the recompute and this arm reports another reason ⇒ RED.
func TestColdAuditor_StallsUnconditionallyAtARecoveryBoundary(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	b := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), b, structWitnessFor(t, f, src, b))

	// Point the chain's own recovery config at the height under test. LivenessRecoveryHeight and
	// EpochBlocks are PUBLIC consensus config the box reads as class 3 — the ambiguity is never
	// whether recovery is configured, it is whether the box may trust the re-base.
	f.c.cfg.EpochBlocks = b.Height
	f.c.cfg.LivenessRecoveryHeight = b.Height
	if !f.c.isAmbiguousRecoveryBoundary(b.Height) {
		t.Fatalf("fixture: height %d must be an ambiguous recovery boundary for this arm to mean anything", b.Height)
	}
	box := boxOver(t, f, src)
	w := structWitnessFor(t, f, src, b)

	out, err := box.Validate(b, w)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRecoveryBoundaryStall) {
		t.Fatalf("at an ambiguous recovery boundary the door must stall (ErrRecoveryBoundaryStall); got %s / %v", out, err)
	}
	// A divergent root does not change it either: the box never got far enough to look.
	if out, err := box.Validate(forgeDivergentRoot(t, f, b), w); out != IndeterminateTrustlessly ||
		!errors.Is(err, ErrRecoveryBoundaryStall) {
		t.Fatalf("the boundary stall precedes any root check; got %s / %v", out, err)
	}
	// ABLATION CONTROL: move the boundary and the same block proceeds.
	f.c.cfg.LivenessRecoveryHeight = b.Height + f.c.cfg.EpochBlocks
	if out, err := boxOver(t, f, src).Validate(b, w); errors.Is(err, ErrRecoveryBoundaryStall) {
		t.Fatalf("away from the configured boundary the door must NOT stall on it; got %s / %v", out, err)
	}
}

// TestColdAuditor_RefusesPrunedBlocks is H-3, at every box surface. A pruned block's Hash()
// short-circuits to a stored token bound to NO struct field — StateRoot included (§2.4) — so a pin
// on it binds nothing and a floor over it would make the reader skip proof verification entirely
// (§2.5, chain.go's pruned leg). The box refuses rather than taking a trust floor from its caller.
//
// ABLATION: delete the b.IsPruned() stall from (*Box).Validate ⇒ the door arm lands on another
// reason ⇒ RED; delete it from WitnessValidateV5 ⇒ the scaffold arm reports ErrRecomputeGated ⇒ RED
// (that arm is in floorbox_v5_test.go); delete the parent.IsPruned() refusal from NewBox ⇒ the
// construction arm ⇒ RED.
func TestColdAuditor_RefusesPrunedBlocks(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	b := f.mkBlock(t, nil)
	w := structWitnessFor(t, f, src, b)
	box := boxOver(t, f, src)
	assertBoxReachesTheDowngrade(t, box, b, w)

	out, err := box.Validate(b.Prune(), w)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrPrunedBlockUnreproducible) {
		t.Fatalf("the door must refuse a pruned block (ErrPrunedBlockUnreproducible); got %s / %v", out, err)
	}
	// The pruned re-anchor case (§2.4): a pruned PARENT cannot anchor a head record either.
	if _, err := NewBox(f.c, f.c.Blocks(1)[0].Prune(), BoxConfig{BudgetBytes: 1 << 22}, src); !errors.Is(err, ErrBoxParentPruned) {
		t.Fatalf("NewBox must refuse a pruned parent (ErrBoxParentPruned); got %v", err)
	}
	// The composition's own pruned leg, reached directly over the box's view, stalls rather than
	// answering from a floor the box does not have (H-4's belt).
	pruned := b.Prune()
	if out, err := ValidateCommitV5(box.view(), &pruned); out == Accept {
		t.Fatalf("the composition must not Accept a pruned block over a proven view; got %s / %v", out, err)
	}
}

// TestColdAuditor_NoTrustFloorOnTheContractSurface is H-4 as the certification restates it after
// REFUTING its own first draft: trustFloor does not appear in the box's composition signature at
// all, and a test asserts the composition takes no *Chain receiver and no floor argument.
//
// The decisive artifact is chain.go's pruned leg: a pruned block strictly BELOW the floor skips the
// space-time re-verify, so a caller who supplies a RAISED floor makes the reader skip proof
// verification for everything under it — accept forged bonded standing. A floor is not a benign
// contract parameter; it is a wrong-accept vector, which is why it is not parameterized but
// REMOVED, and why StateView answers the pruned-tolerance QUESTION instead of exposing the scalar.
//
// ABLATION: re-add `TrustFloor() uint64` (or any method or parameter whose name carries "floor")
// to StateView or to the composition ⇒ RED naming it.
// SOURCE GATE: none — this reads the StateView TYPE and the ValidateCommitV5 FUNC VALUE by
// reflection, so it sees method names, parameter types and arity, and nothing about behaviour.
// RUNTIME GATE: TestColdAuditor_RefusesPrunedBlocks, which drives an actual pruned block through
// the composition over the box's view and watches it not Accept, and the node-side pruned tests
// (prune_test.go), which keep liveView's answer equal to the node's own rule.
func TestColdAuditor_NoTrustFloorOnTheContractSurface(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	b := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), b, structWitnessFor(t, f, src, b))

	// (1) The composition takes no *Chain receiver and no floor argument. ValidateCommitV5 is a
	// package function, so "no receiver" is structural; the arguments are checked by type.
	ft := reflect.TypeOf(ValidateCommitV5)
	if ft.NumIn() != 2 {
		t.Fatalf("H-4: ValidateCommitV5 must take exactly (StateView, *Block); got %d parameters", ft.NumIn())
	}
	chainPtr := reflect.TypeOf((*Chain)(nil))
	for i := 0; i < ft.NumIn(); i++ {
		in := ft.In(i)
		if in == chainPtr {
			t.Fatalf("H-4: ValidateCommitV5 parameter %d is *Chain — the composition must not take the "+
				"node it is supposed to be independent of", i)
		}
		if in.Kind() == reflect.Uint64 {
			t.Fatalf("H-4: ValidateCommitV5 parameter %d is a bare uint64 — a caller-supplied height or "+
				"floor is exactly the wrong-accept vector §2.5 refutes", i)
		}
	}

	// (2) No method on the box's contract surface exposes a floor VALUE. A method whose name
	// carries "floor" and whose only result is a scalar is the shape that was removed.
	svt := reflect.TypeOf((*StateView)(nil)).Elem()
	for i := 0; i < svt.NumMethod(); i++ {
		m := svt.Method(i)
		if !strings.Contains(strings.ToLower(m.Name), "floor") {
			continue
		}
		if m.Type.NumOut() == 1 && m.Type.Out(0).Kind() == reflect.Uint64 {
			t.Fatalf("H-4: StateView.%s returns a bare floor value. A raised floor makes the reader "+
				"SKIP space-time re-verification for every block under it (chain.go's pruned leg), so "+
				"the floor must not be expressible on this interface — ask the question, not the scalar.", m.Name)
		}
	}

	// (3) The box's view answers the pruned-tolerance question with NoWitness — it has no floor and
	// cannot be given one — while the node's view answers its own rule.
	if ok, av := (boxOver(t, f, src)).view().PrunedTolerated(1); ok || av != NoWitness {
		t.Fatalf("H-4: provenView must answer (false, NoWitness); got (%v, %s)", ok, av)
	}
	floor := f.c.trustFloor()
	if ok, av := (liveView{f.c}).PrunedTolerated(floor); ok || av != Present {
		t.Fatalf("liveView must NOT tolerate a pruned block AT its own floor; got (%v, %s)", ok, av)
	}
	if floor > 0 {
		if ok, av := (liveView{f.c}).PrunedTolerated(floor - 1); !ok || av != Present {
			t.Fatalf("liveView must tolerate a pruned block strictly BELOW its own floor; got (%v, %s)", ok, av)
		}
	}
}

// TestColdAuditor_TheKnobIsGoneFromTheTypeSystem pins the two deletions owner call 2 names, so a
// re-introduction is a reviewed event rather than a quiet one. RecoveryDirective.Heights and
// LiveFollower were knobs the recompute cannot honour: a box that can be TOLD to proceed past an
// ambiguous boundary is not a cold auditor, and a box that carries the flag is one flip from not
// being one.
//
// ABLATION: re-add a Recovery field to BoxConfig, or a bool field to Box, or a third parameter to
// WitnessValidateV5 ⇒ RED.
// SOURCE GATE: none — reflection over the BoxConfig and Box TYPES and the WitnessValidateV5 METHOD
// VALUE, so it sees field names, types and arity only. RUNTIME GATE:
// TestColdAuditor_StallsUnconditionallyAtARecoveryBoundary (the door) and
// TestWitnessValidateV5_RecoveryBoundaryStallsUnconditionally (the scaffold), which drive the
// boundary and watch the stall fire with no input able to lift it.
func TestColdAuditor_TheKnobIsGoneFromTheTypeSystem(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	b := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), b, structWitnessFor(t, f, src, b))

	// BoxConfig carries exactly the byte ceiling. Any other field is a knob.
	ct := reflect.TypeOf(BoxConfig{})
	if ct.NumField() != 1 || ct.Field(0).Name != "BudgetBytes" {
		var names []string
		for i := 0; i < ct.NumField(); i++ {
			names = append(names, ct.Field(i).Name)
		}
		t.Fatalf("BoxConfig must carry only BudgetBytes; got %v. Owner call 2 deleted the recovery "+
			"directive: the #535 stall is unconditional, so there is nothing left to configure.", names)
	}
	// The box holds no boolean it could be flipped on. LiveFollower was one.
	bt := reflect.TypeOf(Box{})
	for i := 0; i < bt.NumField(); i++ {
		if bt.Field(i).Type.Kind() == reflect.Bool {
			t.Fatalf("Box carries a bool field %q. LiveFollower was a bool, and a cold auditor with a "+
				"mode flag is one flip from not being one.", bt.Field(i).Name)
		}
	}
	// WitnessValidateV5 takes (Block, [32]byte) — no directive.
	mt := reflect.TypeOf((*Chain).WitnessValidateV5)
	if mt.NumIn() != 3 { // receiver + 2
		t.Fatalf("WitnessValidateV5 must take (b Block, parentStateRoot [32]byte); got %d parameters "+
			"including the receiver — the RecoveryDirective parameter is deleted", mt.NumIn())
	}
	if mt.In(1) != reflect.TypeOf(Block{}) || mt.In(2) != reflect.TypeOf([32]byte{}) {
		t.Fatalf("WitnessValidateV5's parameter types drifted: (%s, %s)", mt.In(1), mt.In(2))
	}
}
