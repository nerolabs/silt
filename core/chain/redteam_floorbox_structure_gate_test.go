package chain

import (
	"crypto/ed25519"
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// PERMANENT GATES for the FLOOR-BOX STRUCTURE round 1A (main-only; owner call 16).
//
// Governing documents:
//   - build brief: /Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md §7
//   - P-table delta: /Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md
//   - composition:   /Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md
//
// THE DEFECT SHAPE THESE GATES EXIST FOR. The box reproduced a node predicate's TAIL without the
// precondition a DIFFERENT validation stage had established (N1 the author screen, RT2-CARRIER-13
// the parent binding). A shared predicate SET cannot close that class; only a shared PATH can.
// These gates drive the ONE v5 accept path (validate_v5.go) through the node's own liveView.
//
// THE FIXTURE IS v5-DRIVEN, AND THAT IS ASSERTED FIRST (arm D / G-D1). BlockVersionRounds (2) is
// what the proposer mints today and Era4ActivationHeight has no cmd/silt flag, so a fixture that
// does not BUILD its v5 chain never enters the composition and every equivalence gate over it is
// vacuously green (the silt-gates-hold-the-seam scar). TestGD1_FixtureCommitsAWitnessableBlock is
// therefore the first gate in the file and every other gate here builds on the same fixture.

// structFixture is a node-accepted v5 world: four launch anchors, all bonded at genesis, objective
// mode, mature-from-genesis (MatureValidators: 0 so the maturity latch is true pre-state).
type structFixture struct {
	c    *Chain
	keys []ed25519.PrivateKey
}

func buildStructFixture(t *testing.T) structFixture {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(77000 + i))
		anchors[idOf(keys[i])] = true
	}
	c := New(Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 0,
	}, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	// One committed v5 block, so the block under test sits at height 2 (a carrier is illegal at
	// height <= 1) and the fixture's pre-state is a real post-apply state.
	f := structFixture{c: c, keys: keys}
	b1 := f.mkBlock(t, func(b *Block) { b.Entries = []ports.Entry{entry(1)} })
	if err := c.Append(b1); err != nil {
		t.Fatalf("fixture h1 must COMMIT (it is the node oracle's own path): %v", err)
	}
	return f
}

// mkBlock builds a fully certified v5 block on the fixture's head: proposer signature, committed
// roots, prepare QC and precommit certificate — a block the NODE accepts. mutate shapes the
// payload before the roots are computed.
func (f structFixture) mkBlock(t *testing.T, mutate func(*Block)) Block {
	t.Helper()
	prev, h := f.c.Head()
	b := &Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h + 40))}}
	if mutate != nil {
		mutate(b)
	}
	setD3Digests(b) // (d-3): an honest v5 proposer commits Answer/Slashes by digest before signing
	state, log, err := f.c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, f.keys[0])
	for _, k := range f.keys {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare, f.c.ChainID()))
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit, f.c.ChainID()))
	}
	return *b
}

// assertHonestTwinAccepts is the NON-VACUITY control every structure gate carries (NG-2): the
// fixture's honest block is accepted by the node's own door AND reaches Accept in the composition
// over liveView. A gate whose refusal arm is green while this fails is refusing for the wrong
// reason. It is a fixture-health check, not a contract assertion.
func assertHonestTwinAccepts(t *testing.T, c *Chain, b Block) {
	t.Helper()
	if err := c.ValidateCommit(&b); err != nil {
		t.Fatalf("NON-VACUITY BROKEN: the node refuses the honest twin (%v)", err)
	}
	if out, err := ValidateCommitV5(liveView{c}, &b); out != Accept {
		t.Fatalf("NON-VACUITY BROKEN: the composition over liveView does not Accept the honest twin (%s / %v)", out, err)
	}
}

// =============================================================================
// G-D1 / arm D — the fixture reaches the v5 path (NON-VACUITY, runs first)
// =============================================================================

// TestGD1_FixtureCommitsAWitnessableBlock. Ablation: force b.Version = BlockVersionRounds in
// mkBlock; this gate must FAIL. Without it, a v2 fixture makes every gate in this file and in
// composition_stage_cover_v5_test.go vacuously green.
func TestGD1_FixtureCommitsAWitnessableBlock(t *testing.T) {
	f := buildStructFixture(t)
	head := f.c.Blocks(1)
	if len(head) == 0 {
		t.Fatal("fixture committed no block above genesis")
	}
	if got := head[len(head)-1].Version; got != BlockVersionWitnessable {
		t.Fatalf("FIXTURE VACUOUS: the committed block is v%d, want v%d (BlockVersionWitnessable). "+
			"A sub-v5 fixture never enters ValidateProposalV5/ValidateCommitV5, so every equivalence "+
			"gate over it passes for the wrong reason.", got, BlockVersionWitnessable)
	}
	b := f.mkBlock(t, nil)
	if b.Version != BlockVersionWitnessable {
		t.Fatalf("FIXTURE VACUOUS: mkBlock mints v%d, want v%d", b.Version, BlockVersionWitnessable)
	}
	assertHonestTwinAccepts(t, f.c, b)
}

// =============================================================================
// G-D12 — BOTH node entry points dispatch to the composition (M-2)
// =============================================================================

// TestGD12_BothNodeEntryPointsDispatchToTheComposition pins the structural claim of M-2: the
// node's v5 accept path IS the composition at BOTH doors. A node that kept its own copy in
// ValidateProposal would leave the attester signing under a rule the committer does not accept
// under — the #402 one-function-two-callers trap inside the node.
// Ablation (G-D12): revert the ValidateProposal dispatch hunk ⇒ RED.
// SOURCE GATE: reads chain.go and asserts the FIRST statement of each root is the guarded dispatch.
// RUNTIME GATE: TestGD6_LegacyRepLegOracle drives ValidateCommit (the dispatch) on one block.
func TestGD12_BothNodeEntryPointsDispatchToTheComposition(t *testing.T) {
	f := buildStructFixture(t)
	assertHonestTwinAccepts(t, f.c, f.mkBlock(t, nil))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "chain.go", nil, 0)
	if err != nil {
		t.Fatalf("SOURCE GATE: parse chain.go: %v", err)
	}
	for root, entry := range map[string]string{"ValidateProposal": "ValidateProposalV5", "ValidateCommit": "ValidateCommitV5"} {
		var body *ast.BlockStmt
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if ok && fd.Name.Name == root && fd.Recv != nil && chainReceiver(fd.Recv.List[0].Type) {
				body = fd.Body
			}
			return body == nil
		})
		if body == nil {
			t.Fatalf("SOURCE GATE: %s not found in chain.go", root)
		}
		first, ok := body.List[0].(*ast.IfStmt)
		if !ok {
			t.Fatalf("SOURCE GATE: G-D12 — %s's FIRST statement must be the v5 dispatch (BG-1: anything before it runs "+
				"on the era-1/era-2 path too, which the extraction must not touch); got %T", root, body.List[0])
		}
		var src strings.Builder
		if err := printer.Fprint(&src, fset, first); err != nil {
			t.Fatal(err)
		}
		for _, needle := range []string{"BlockVersionWitnessable", entry, "liveView"} {
			if !strings.Contains(src.String(), needle) {
				t.Fatalf("SOURCE GATE: G-D12 — %s's v5 dispatch must be guarded by BlockVersionWitnessable and call %s over "+
					"liveView; %q is missing from:\n%s", root, entry, needle, src.String())
			}
		}
	}
}

// =============================================================================
// G-5 — NoWitness is the zero of Availability; liveView never answers it; provenView with no
// source answers nothing else
// =============================================================================

// TestG5_LiveViewNeverAnswersNoWitness. IndeterminateTrustlessly is unreachable under liveView
// because a full node holds the whole state — a DIAGNOSTIC, never a premise: the dispatch maps it
// to a refusal anyway, so an adapter bug costs a refusal and never an acceptance. This gate is what
// tells us the adapter is not silently stalling. Ablation (G-5): return NoWitness from one liveView
// accessor ⇒ RED.
func TestG5_LiveViewNeverAnswersNoWitness(t *testing.T) {
	if Availability(0) != NoWitness {
		t.Fatal("G-5: NoWitness must be the ZERO of Availability — a forgotten field must read as a stall, never as absent")
	}
	f := buildStructFixture(t)
	b := f.mkBlock(t, nil)
	assertHonestTwinAccepts(t, f.c, b)
	v := liveView{f.c}
	if !v.WitnessBudget().Unlimited() {
		t.Fatal("G-5: liveView must return UnlimitedBudget() — a ceiling on the node is a new validity rule")
	}
	if h := v.Head(); h.Empty || h.StateRoot == nil || h.LogRoot == nil || h.NextHeight != b.Height || h.Hash != b.Prev {
		t.Fatalf("G-5: liveView.Head() must reproduce Chain.Head() with the parent's roots present: %+v", h)
	}

	id := b.ProposerID()
	type check struct {
		name string
		av   Availability
	}
	var checks []check
	rec := func(n string, av Availability) { checks = append(checks, check{n, av}) }
	_, av := v.EpochSet()
	rec("EpochSet", av)
	_, av = v.Qualified()
	rec("Qualified", av)
	_, av = v.Bonded()
	rec("Bonded", av)
	_, av = v.ValidatorsSeen()
	rec("ValidatorsSeen", av)
	_, av = v.SlashedSet()
	rec("SlashedSet", av)
	_, av = v.Slashed(id)
	rec("Slashed", av)
	_, av = v.BondedOf(id)
	rec("BondedOf", av)
	_, av = v.ByRoot(b.Entries[0].Root)
	rec("ByRoot", av)
	_, av = v.Spent([]byte("any"))
	rec("Spent", av)
	_, av = v.Revoked(b.Entries[0].Root)
	rec("Revoked", av)
	_, av = v.BondRootOwner(ports.Hash{})
	rec("BondRootOwner", av)
	_, av = v.BondRegHeight(id)
	rec("BondRegHeight", av)
	_, av = v.BondDomain(id)
	rec("BondDomain", av)
	_, av = v.Ancestors(8)
	rec("Ancestors", av)
	_, av = v.Rep(id)
	rec("Rep", av)
	for _, tag := range []string{tagEverMature, tagMatureEpoch, tagGateLockedIn, tagGateHeight,
		tagEra3LockedIn, tagEra3Height, tagEra4LockedIn, tagEra4Height, tagEpochStart} {
		_, av := v.Scalar(tag)
		rec("Scalar("+strings.TrimSuffix(tag, "\x00")+")", av)
	}
	for _, ck := range checks {
		if ck.av == NoWitness {
			t.Errorf("G-5: liveView.%s answered NO WITNESS. A full node holds the whole state; this is an adapter bug. "+
				"It costs a REFUSAL (never an acceptance) because the dispatch maps Indeterminate to an error — but it is still a bug.", ck.name)
		}
	}
	// End to end: the node's own accept path must not stall on its own block, at either door.
	if out, err := ValidateProposalV5(v, &b); out != Accept {
		t.Fatalf("liveView must reach Accept in ValidateProposalV5 on a node-accepted block; got %s / %v", out, err)
	}
	if out, err := ValidateCommitV5(v, &b); out != Accept {
		t.Fatalf("liveView must reach Accept in ValidateCommitV5 on a node-accepted block; got %s / %v", out, err)
	}

	// The proven twin: with NO witness source every class-2 read and Rep answer NoWitness — the
	// safe zero — and never "absent".
	pv := f.provenViewOver(t, nil)
	_, av = pv.Rep(id)
	if av != NoWitness {
		t.Fatalf("G-5: provenView.Rep must answer NoWitness (legacy rep is not a committed leaf); got %s", av)
	}
	for name, get := range map[string]func() Availability{
		"Slashed":        func() Availability { _, av := pv.Slashed(id); return av },
		"BondedOf":       func() Availability { _, av := pv.BondedOf(id); return av },
		"ByRoot":         func() Availability { _, av := pv.ByRoot(b.Entries[0].Root); return av },
		"EpochSet":       func() Availability { _, av := pv.EpochSet(); return av },
		"Qualified":      func() Availability { _, av := pv.Qualified(); return av },
		"Bonded":         func() Availability { _, av := pv.Bonded(); return av },
		"ValidatorsSeen": func() Availability { _, av := pv.ValidatorsSeen(); return av },
		"Scalar":         func() Availability { _, av := pv.Scalar(tagEverMature); return av },
		"Ancestors":      func() Availability { _, av := pv.Ancestors(8); return av },
	} {
		if av := get(); av != NoWitness {
			t.Errorf("G-5: provenView.%s with no source answered %s, want NO_WITNESS — a sourceless box must stall, never see absence", name, av)
		}
	}
}

// provenViewOver builds a provenView over the fixture chain's OWN head record (the box derives its
// head from a pinned block in part 2; here the node's record stands in) with the given source and a
// positive frame budget. No P13a predicate is wired: the substituted step's StateRoot leg stalls.
func (f structFixture) provenViewOver(t *testing.T, src WitnessSource) provenView {
	t.Helper()
	bud, err := ByteBudget(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	return provenView{
		params:     liveView{f.c}.Params(),
		objective:  true,
		verifyBond: objectiveVerify,
		budget:     bud,
		head:       liveView{f.c}.Head(),
		src:        src,
	}
}

// =============================================================================
// G-D10 — the zero Budget STALLS; only UnlimitedBudget() is unlimited (M-4)
// =============================================================================

// TestGD10_ZeroBudgetStallsAndOnlyLiveViewIsUnlimited. Ablation (G-D10): make v5CheckBudget treat
// the zero Budget as unlimited ⇒ the zero-budget view proceeds past step 0b ⇒ RED.
func TestGD10_ZeroBudgetStallsAndOnlyLiveViewIsUnlimited(t *testing.T) {
	if !(Budget{}).IsZero() || (Budget{}).Unlimited() {
		t.Fatal("G-D10: the zero Budget must be IsZero and NOT Unlimited")
	}
	if _, err := ByteBudget(0); !errors.Is(err, ErrBudgetNotPositive) {
		t.Fatalf("G-D10: ByteBudget(0) must REFUSE; got %v", err)
	}
	if _, err := ByteBudget(-1); !errors.Is(err, ErrBudgetNotPositive) {
		t.Fatalf("G-D10: ByteBudget(-1) must REFUSE; got %v", err)
	}
	f := buildStructFixture(t)
	b := f.mkBlock(t, nil)
	assertHonestTwinAccepts(t, f.c, b)
	// A view constructed with the zero Budget stalls at step 0b, BY NAME, before any other read.
	pv := f.provenViewOver(t, nil)
	pv.budget = Budget{}
	out, err := ValidateCommitV5(pv, &b)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrWitnessBudgetUnset) {
		t.Fatalf("G-D10 VIOLATED: the zero Budget must STALL at step 0b with ErrWitnessBudgetUnset; got %s / %v", out, err)
	}
	// A positive frame budget below the block's frame stalls on the budget, before any signature
	// work: the discriminator is a GARBAGE proposer signature that would otherwise fail P3.
	garbage := b
	garbage.hashMemoSet = false
	garbage.ProposerSig = make([]byte, ed25519.SignatureSize)
	tight, _ := ByteBudget(1)
	pv.budget = tight
	out, err = ValidateCommitV5(pv, &garbage)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrWitnessBudgetExceeded) {
		t.Fatalf("G-D10: an over-budget frame must stall on the BUDGET before any crypto; got %s / %v", out, err)
	}
	// Within budget the same block fails on its garbage signature — proving the budget check is
	// what fired above and that it sits ahead of the crypto.
	pv.budget, _ = ByteBudget(len(Encode(&garbage)) + 1)
	out, err = ValidateCommitV5(pv, &garbage)
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("G-D10 ablation arm: within budget the garbage signature must fail P3; got %s / %v", out, err)
	}
	if !(liveView{f.c}).WitnessBudget().Unlimited() {
		t.Fatal("G-D10: liveView must be the one view that returns UnlimitedBudget()")
	}
}

// =============================================================================
// G-D5 — an IssuerKeys-only v5 block: accepted; a bad issuer key: refused by name (M-5)
// =============================================================================

// TestGD5_IssuerKeysOnlyBlockParity. The two halves of the PE's finding (a), driven:
//   - a block whose ONLY payload is a valid issuer-key registration is NOT "empty" (the sixth P5
//     clause). Ablation: drop len(b.IssuerKeys)==0 from the composition's P5 ⇒ RED.
//   - a block carrying an UNBONDED issuer's registration is refused by P8b, by name. Ablation:
//     drop the v5ValidateIssuerKeys call ⇒ RED.
//
// Both are asserted at BOTH node doors and on the composition directly.
func TestGD5_IssuerKeysOnlyBlockParity(t *testing.T) {
	f := buildStructFixture(t)
	fp := ports.HashBytes([]byte("issuer key fingerprint"))
	good := f.mkBlock(t, func(b *Block) {
		b.Entries = nil
		b.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(f.keys[1], 0, fp)} // keys[1] is bonded at genesis
	})
	if len(good.Entries) != 0 || len(good.IssuerKeys) != 1 {
		t.Fatal("fixture: the block must carry ONLY an issuer-key registration")
	}
	assertHonestTwinAccepts(t, f.c, f.mkBlock(t, nil))
	if err := f.c.ValidateProposal(&good); err != nil {
		t.Fatalf("G-D5 VIOLATED (P5): ValidateProposal refused an IssuerKeys-only v5 block: %v", err)
	}
	if err := f.c.ValidateCommit(&good); err != nil {
		t.Fatalf("G-D5 VIOLATED (P5): ValidateCommit refused an IssuerKeys-only v5 block: %v", err)
	}
	if out, err := ValidateCommitV5(liveView{f.c}, &good); out != Accept {
		t.Fatalf("G-D5 VIOLATED (P5): the composition refused an IssuerKeys-only v5 block: %s / %v", out, err)
	}

	unbonded := key(99001)
	bad := f.mkBlock(t, func(b *Block) {
		b.Entries = nil
		b.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(unbonded, 0, fp)}
	})
	for name, err := range map[string]error{
		"ValidateProposal": f.c.ValidateProposal(&bad),
		"ValidateCommit":   f.c.ValidateCommit(&bad),
	} {
		if !errors.Is(err, ErrIssuerKeyUnbonded) {
			t.Fatalf("G-D5 VIOLATED (P8b): %s must refuse an UNBONDED issuer's registration by name (ErrIssuerKeyUnbonded); got %v", name, err)
		}
	}
	if out, err := ValidateProposalV5(liveView{f.c}, &bad); out != Reject || !errors.Is(err, ErrIssuerKeyUnbonded) {
		t.Fatalf("G-D5 VIOLATED (P8b): the composition must refuse an unbonded issuer by name; got %s / %v", out, err)
	}
}

// =============================================================================
// G-D6 — LEGACY-MODE PARITY (M-1)
// =============================================================================

// legacyFixture is a chain in the LEGACY (non-objective) regime — MinBond == 0, qualification by the
// local reputation view — which is operator-reachable in production (cmd/silt/daemon.go
// useObjective := *objective && *minRep > 0). The composition dispatches on VERSION, not on mode,
// so it must take this branch faithfully.
type legacyFixture struct {
	c    *Chain
	reps map[ports.NodeID]int64
	prop ed25519.PrivateKey
	vals []ed25519.PrivateKey
}

func buildLegacyFixture(t *testing.T) legacyFixture {
	t.Helper()
	lf := legacyFixture{reps: map[ports.NodeID]int64{}, prop: key(88001)}
	for i := int64(2); i <= 4; i++ {
		lf.vals = append(lf.vals, key(88000+i))
	}
	lf.c = New(Config{MinProposerRep: 100, MinAttesterRep: 100, Quorum: 2}, func(id ports.NodeID) int64 { return lf.reps[id] })
	lf.reps[idOf(lf.prop)] = 1000
	for _, v := range lf.vals {
		lf.reps[idOf(v)] = 1000
	}
	if lf.c.objective() {
		t.Fatal("fixture: the legacy chain must NOT be objective (MinBond == 0)")
	}
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, lf.prop)
	if err := lf.c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return lf
}

// mkBlock builds a certified v5 block on the legacy chain's head.
func (lf legacyFixture) mkBlock(t *testing.T) Block {
	t.Helper()
	prev, h := lf.c.Head()
	b := &Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h + 60))}}
	state, log, err := lf.c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, lf.prop)
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, lf.prop, 0, PhasePrepare, lf.c.ChainID())) // C1: the author's own prepare
	for _, v := range lf.vals {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, v, 0, PhasePrepare, lf.c.ChainID()))
		b.Atts = append(b.Atts, AttestAt(b, v, 0, PhasePrecommit, lf.c.ChainID()))
	}
	return *b
}

// TestGD6_LegacyRepLegOracle pins M-1: the composition's LEGACY leg reads the local reputation
// view through StateView.Rep, and liveView answers it PRESENT — so a MinBond == 0 node accepts a
// certified v5 block, refuses a low-reputation proposer by name (ErrLowReputation) with the node's
// own rendering ("has 10, needs 100"), and drops a low-reputation attester from the quorum.
//
// This is an ORACLE with absolute assertions, not a differential. Under M-2 the node's
// ValidateCommit IS ValidateCommitV5(liveView{c}, b) for a v5 block, so comparing the two would
// compare one function with itself (the certification's own G-D6 spec was self-defeating; M-1A-4
// struck the clause). The v4/v5 differential for the legacy leg lives in the parity oracle
// (parity_oracle_v4v5_test.go, the legacy regime).
//
// Ablation (G-D6): make liveView.Rep answer NoWitness ⇒ the accept case stalls ⇒ RED (never a
// skip: the fixture asserts it is legacy).
func TestGD6_LegacyRepLegOracle(t *testing.T) {
	lf := buildLegacyFixture(t)
	b := lf.mkBlock(t)
	assertHonestTwinAccepts(t, lf.c, b)

	if err := lf.c.ValidateCommit(&b); err != nil {
		t.Fatalf("G-D6 VIOLATED (accept): a legacy node must accept a certified v5 block — the legacy leg's Rep read must be PRESENT on liveView; got %v", err)
	}
	if err := lf.c.Append(b); err != nil {
		t.Fatalf("G-D6: the legacy chain must COMMIT the v5 block: %v", err)
	}

	// The MinProposerRep refusal: drop the proposer's reputation below the bar.
	b2 := lf.mkBlock(t)
	lf.reps[idOf(lf.prop)] = 10
	err := lf.c.ValidateCommit(&b2)
	if !errors.Is(err, ErrLowReputation) {
		t.Fatalf("G-D6 VIOLATED (refusal): the legacy leg must refuse on MinProposerRep by name (ErrLowReputation); got %v", err)
	}
	want := "proposer " + idOf(lf.prop).String() + " has 10, needs 100"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("G-D6 VIOLATED (attribution): the legacy refusal must render the node's way (%q); got %v", want, err)
	}
	// And the attester leg: an attester below MinAttesterRep is dropped from the quorum, so with
	// Quorum: 2 and one of three attesters demoted the block still commits; with two demoted it
	// does not — through the composition, on the legacy chain.
	lf.reps[idOf(lf.prop)] = 1000
	lf.reps[idOf(lf.vals[0])] = 10
	b3 := lf.mkBlock(t)
	if err := lf.c.ValidateCommit(&b3); err != nil {
		t.Fatalf("G-D6 (attester leg): one demoted attester of three must still meet Quorum: 2; got %v", err)
	}
	lf.reps[idOf(lf.vals[1])] = 10
	if err := lf.c.ValidateCommit(&b3); !errors.Is(err, ErrNoQuorum) {
		t.Fatalf("G-D6 (attester leg): two demoted attesters of three must miss Quorum: 2 (ErrNoQuorum); got %v", err)
	}
}

// =============================================================================
// G-D7 / G-D8 — the P13b LogRoot conjunct on BOTH views
// =============================================================================

// TestGD7_ForgedLogRootIsRefusedOnBothViews. A revocation-free v5 block with a mutated b.LogRoot is
// REFUSED by the composition on liveView (the node's recompute) AND by the proven view's k = 0
// equality against the head's LogRoot. Ablation (G-D7): delete the k == 0 equality in
// provenView.CommittedRoots ⇒ the mutant passes P13b ⇒ RED.
func TestGD7_ForgedLogRootIsRefusedOnBothViews(t *testing.T) {
	f := buildStructFixture(t)
	honest := f.mkBlock(t, nil)
	assertHonestTwinAccepts(t, f.c, honest)
	forged := f.mkBlock(t, nil)
	bad := ports.HashBytes([]byte("a forged revocation-log root"))
	forged.LogRoot = &bad
	forged.hashMemoSet = false
	Sign(&forged, f.keys[0])
	forged.PrepareQC, forged.Atts = nil, nil
	for _, k := range f.keys {
		forged.PrepareQC = append(forged.PrepareQC, AttestAt(&forged, k, 0, PhasePrepare, f.c.ChainID()))
		forged.Atts = append(forged.Atts, AttestAt(&forged, k, 0, PhasePrecommit, f.c.ChainID()))
	}
	if len(forged.Revocations)+len(forged.Unrevocations) != 0 {
		t.Fatal("fixture: the forged block must be revocation-free (k = 0)")
	}
	// liveView: the node's own recompute names the LogRoot.
	if err := f.c.ValidateCommit(&forged); !errors.Is(err, ErrEra3LogRootMismatch) {
		t.Fatalf("G-D7 VIOLATED (liveView): a forged LogRoot must be refused by name; got %v", err)
	}
	// provenView: the k = 0 equality against the head's LogRoot, with NO witness at all.
	pv := f.provenViewOver(t, nil)
	if out, err := pv.CommittedRoots(&forged); out != Reject || !errors.Is(err, ErrEra3LogRootMismatch) {
		t.Fatalf("G-D7 VIOLATED (provenView): the k = 0 LogRoot equality must REJECT a forged LogRoot; got %s / %v", out, err)
	}
	// The honest twin: the same view passes P13b on the honest block and reaches P13a, which
	// stalls (no recompute wired here) — a stall, never a Reject.
	if out, err := pv.CommittedRoots(&honest); out != IndeterminateTrustlessly || !errors.Is(err, ErrRecomputeGated) {
		t.Fatalf("G-D7 twin: the honest block must pass P13b and stall at the unwired P13a; got %s / %v", out, err)
	}
}

// TestGD8_RevocationBearingBlockStallsWithoutAWitness. A revocation-bearing v5 block with NO
// witness source still returns IndeterminateTrustlessly by name: with no source there is no
// tagRevLogSize leaf to Resolve, so the parent log size is unauthenticated and the box refuses.
// This is the arm that used to be the WHOLE story (k >= 1 was a terminal stall until the leaf
// landed); it is kept because it is still the correct verdict when the witness is absent, and it
// is the negative control for the arm below.
//
// Ablation (G-D8): delete the k > 0 branch in CommittedRoots => the block falls to the k = 0
// equality and is REJECTED (its LogRoot moved) => RED. The refusal must never render as a
// disproof, and never as an accept.
func TestGD8_RevocationBearingBlockStallsWithoutAWitness(t *testing.T) {
	f := buildStructFixture(t)
	committed := f.c.Blocks(1)[0].Entries[0].Root // entry(1), committed at h1
	b := f.mkBlock(t, func(b *Block) {
		b.Entries = nil
		b.Revocations = []ports.Hash{committed}
	})
	assertHonestTwinAccepts(t, f.c, b) // the node and the composition accept the revocation block
	pv := f.provenViewOver(t, nil)
	out, err := pv.CommittedRoots(&b)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRevLogSizeUnauthenticated) {
		t.Fatalf("G-D8 VIOLATED: a revocation-bearing block with no witness source must STALL on the proven view with ErrRevLogSizeUnauthenticated; got %s / %v", out, err)
	}
}
