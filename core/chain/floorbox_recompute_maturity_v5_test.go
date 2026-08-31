package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the trustless floor-box RECOMPUTE increment 2 (floorbox_recompute_maturity_v5.go):
// the root-only reproduction of matureNow (the maturity-latch metric via C2Metric), replicating
// increment 1's C-1 pattern AND shipping the mandatory C-6 config-from-witness ablation TEETH.
//
// The four HARD ABLATIONS (C-5, red-before-green), each injected and watched to flip the verdict,
// so a green here is not decoration:
//   - FORGED BONDED WEIGHT (C-1): a witness with the right members but a forged per-member bonded
//     weight ⇒ STALL (its inclusion proof fails against the committed root).
//   - FORGED BONDDOMAIN (C-1): a witness with a forged per-member domain ⇒ STALL.
//   - OMITTED / INJECTED MEMBER: a witness missing/padding a validatorsSeen member ⇒ MTH mismatch
//     ⇒ STALL (set-completeness).
//   - CONFIG-FROM-WITNESS (C-6, THE TEETH): a fold that read MinBond/OperatorMargin/MatureValidators
//     from the WITNESS instead of own config would make two boxes with different own config DIVERGE
//     on the same witness. The correct own-config fold is INVARIANT. This is the C-6 teeth increment
//     1 could not exercise (its predicate read no genesis knob).
//
// The recompute NEVER flips WitnessValidateV5 to Accept (the STOP boundary); it reproduces ONE
// predicate.

// maturityFixture is an objective v5 chain with a populated validatorsSeen (distinct bonds +
// distinct declared domains, each seated as an attester), plus the committed StateRoot and a Prover
// over its v5 leaf set. A floor box holds maturityFixture.root; the Prover stands in for the
// any-of-N witness provider that holds the committed set.
type maturityFixture struct {
	c       *Chain
	root    ports.Hash
	prover  *statehash.Prover
	members []ports.NodeID // the validatorsSeen ids (the non-anchor seated validators)
}

// maturityBond is one seated validator: its key, bonded weight, and declared A-axis domain.
type maturityBond struct {
	priv   ed25519.PrivateKey
	size   int64
	domain uint64
}

// buildMaturityFixture seats several distinct bonds (distinct domains) as attesters on an
// objective v5 chain, so validatorsSeen is populated and C2Metric has a non-trivial fold, then
// snapshots the committed v5 StateRoot and a Prover over its v5 leaves. Two anchors bootstrap the
// young network (they attest but are skipped by C2Metric via the own-Anchors screen). matureValidators
// and operatorMargin are the box's own config knobs the recompute reads (C-6).
func buildMaturityFixture(t *testing.T, matureValidators, operatorMargin int, bonds []maturityBond) maturityFixture {
	t.Helper()
	const minBond = int64(1) << 20
	a1, a2 := key(1), key(2)
	cfg := Config{
		Quorum: 2, MinBond: minBond, MinProposerRep: 0, MinAttesterRep: 0,
		Anchors:          map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true},
		AnchorQuorum:     1,
		MatureValidators: matureValidators,
		OperatorMargin:   operatorMargin,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	regs := []BondReg{bondReg(a1, minBond, ports.Hash{}), bondReg(a2, minBond, ports.Hash{})}
	for _, b := range bonds {
		regs = append(regs, bondRegDom(b.priv, b.size, ports.Hash{}, b.domain))
	}
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: regs}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	prev := g
	for _, b := range bonds {
		prev = appendCommit(t, c, a1, prev, a2, b.priv)
	}

	// The seated validators (non-anchor members of validatorsSeen). C2Metric folds exactly these
	// (minus slashed); the two anchors are screened by the own-Anchors gate.
	members := make([]ports.NodeID, 0, len(c.validatorsSeen))
	for id := range c.validatorsSeen {
		if cfg.Anchors[id] {
			continue
		}
		members = append(members, id)
	}
	if len(members) == 0 {
		t.Fatal("fixture precondition: validatorsSeen must have non-anchor members after the commits")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	root := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != root {
		t.Fatalf("fixture root mismatch: prover=%x chain=%x", root, sr)
	}
	return maturityFixture{c: c, root: root, prover: prover, members: members}
}

// witnessFor builds a well-formed SeenSetWitness proving the complete validatorsSeen set against
// the committed root: the validatorsSeenRoot digest leaf + one MemberStateWitness (slashed / bonded
// / bondDomain proofs) per SEATED validatorsSeen id. It witnesses the FULL validatorsSeen set
// (including anchors, since C2Metric's fold ranges the whole set and screens anchors internally by
// own config).
func (f maturityFixture) witnessFor(t *testing.T) SeenSetWitness {
	t.Helper()
	rootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	rootVal := nodeSetMTHFromBool(f.c.validatorsSeen)
	rootProof, err := f.prover.Prove(rootKey)
	if err != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", err)
	}

	ids := make([]ports.NodeID, 0, len(f.c.validatorsSeen))
	members := make(map[ports.NodeID]MemberStateWitness, len(f.c.validatorsSeen))
	for id := range f.c.validatorsSeen {
		ids = append(ids, id)
		members[id] = f.memberWitness(t, id)
	}
	return SeenSetWitness{
		IDs:             ids,
		SeenRootWitness: rootProof,
		SeenRootValue:   rootVal,
		Members:         members,
	}
}

// memberWitness builds one member's committed-state witness: its slashed / bonded / bondDomain
// (non-)inclusion proofs and claimed values, all against the committed root.
func (f maturityFixture) memberWitness(t *testing.T, id ports.NodeID) MemberStateWitness {
	t.Helper()
	mw := MemberStateWitness{}

	slashedKey := statehash.Key(tagSlashed, id[:])
	sp, err := f.prover.Prove(slashedKey)
	if err != nil {
		t.Fatalf("Prove(slashed[%x]): %v", id[:], err)
	}
	mw.Slashed = f.c.slashed[id]
	mw.SlashedProof = sp

	bondedKey := statehash.Key(tagBonded, id[:])
	bp, err := f.prover.Prove(bondedKey)
	if err != nil {
		t.Fatalf("Prove(bonded[%x]): %v", id[:], err)
	}
	mw.Bonded = f.c.bonded[id]
	mw.BondedProof = bp

	domainKey := statehash.Key(tagBondDomain, id[:])
	dp, err := f.prover.Prove(domainKey)
	if err != nil {
		t.Fatalf("Prove(bondDomain[%x]): %v", id[:], err)
	}
	d, present := f.c.bondDomain[id]
	mw.Domain = d
	mw.DomainPresent = present
	mw.DomainProof = dp
	return mw
}

// diverseBonds is a fixture set of five distinct bonds with FIVE distinct declared domains, so the
// domain-distinct coefficient equals the bond-distinct coefficient (no aggregation). Weights are
// spread so the Nakamoto fold is non-trivial.
func diverseBonds() []maturityBond {
	return []maturityBond{
		{key(30), 8 << 20, 0xA1},
		{key(31), 6 << 20, 0xB2},
		{key(32), 5 << 20, 0xC3},
		{key(33), 4 << 20, 0xD4},
		{key(34), 4 << 20, 0xE5},
	}
}

// TestRecomputeMatureNow_MatchesFullNode is the equivalence anchor: over the SAME committed state,
// the trustless recompute's verdict equals the full node's matureNow verdict — for BOTH a mature
// config (low bar) and an immature config (high bar). This proves the recompute reproduces the
// predicate, not merely that it does not crash.
func TestRecomputeMatureNow_MatchesFullNode(t *testing.T) {
	t.Run("mature: low MatureValidators bar is cleared (matches full node)", func(t *testing.T) {
		f := buildMaturityFixture(t, 2, 1, diverseBonds()) // 5 diverse bonds, coefficient >= 2
		w := f.witnessFor(t)
		got, reason := f.c.RecomputeMatureNow(f.root, w)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		if got != f.c.matureNow() {
			t.Fatalf("recompute verdict %v != full node matureNow() %v", got, f.c.matureNow())
		}
		if !got {
			t.Fatal("5 address-diverse bonds must clear a MatureValidators=2 bar")
		}
	})

	t.Run("immature: high MatureValidators bar is missed (matches full node)", func(t *testing.T) {
		f := buildMaturityFixture(t, 6, 1, diverseBonds()) // bar 6 > 5 members, never mature
		w := f.witnessFor(t)
		got, reason := f.c.RecomputeMatureNow(f.root, w)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		if got != f.c.matureNow() {
			t.Fatalf("recompute verdict %v != full node matureNow() %v", got, f.c.matureNow())
		}
		if got {
			t.Fatal("a MatureValidators=6 bar over 5 members must NOT be met")
		}
	})
}

// TestRecomputeMatureNow_ForgedBondedWeightRejects is HARD ABLATION 1 (C-1): a witness with the
// RIGHT members but a FORGED per-member bonded weight makes the recompute STALL — the forged
// weight's inclusion proof does not verify against the committed root.
//
// RED-BEFORE-GREEN: the un-forged witness (TestRecomputeMatureNow_MatchesFullNode) reaches a
// verdict; forging one member's bonded weight flips it to a stall.
func TestRecomputeMatureNow_ForgedBondedWeightRejects(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeMatureNow(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: forge one member's claimed bonded weight while KEEPING its original
	// inclusion proof. The proof was built for the TRUE weight, so Resolve against the forged
	// EncodeInt64(weight) fails ⇒ the member's bonded is unproven ⇒ stall.
	victim := f.members[0]
	forged := cloneWitness(w)
	mw := forged.Members[victim]
	mw.Bonded += 100 << 20
	forged.Members[victim] = mw

	got, reason := f.c.RecomputeMatureNow(f.root, forged)
	if got {
		t.Fatal("C-1 VIOLATION: a forged per-member bonded weight was accepted — the coefficient is forgeable")
	}
	if !errors.Is(reason, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("forged bonded should stall on ErrRecomputeMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeMatureNow_ForgedDomainRejects is HARD ABLATION 2 (C-1): a witness with a FORGED
// per-member bondDomain makes the recompute STALL — the forged domain's inclusion proof does not
// verify against the committed root. The domain drives the address-diverse coefficient
// (NakamotoDomains), so a forged domain could otherwise fake decentralization.
func TestRecomputeMatureNow_ForgedDomainRejects(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeMatureNow(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: forge one member's claimed domain while KEEPING its original proof.
	victim := f.members[0]
	forged := cloneWitness(w)
	mw := forged.Members[victim]
	mw.Domain += 0xFFFF // a domain value the committed root does not commit for this member
	forged.Members[victim] = mw

	got, reason := f.c.RecomputeMatureNow(f.root, forged)
	if got {
		t.Fatal("C-1 VIOLATION: a forged per-member bondDomain was accepted — the address-diverse coefficient is forgeable")
	}
	if !errors.Is(reason, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("forged domain should stall on ErrRecomputeMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeMatureNow_OmittedMemberRejects is HARD ABLATION 3: a witness MISSING a seated member
// reconstructs a DIFFERENT MTH than the committed validatorsSeenRoot ⇒ STALL. A withholding prover
// cannot shrink the folded set to fake (or deny) maturity, because the digest binds the complete
// id-set.
func TestRecomputeMatureNow_OmittedMemberRejects(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeMatureNow(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: drop one member from the witnessed id-list (and its member witness). The
	// reconstructed nodeSetMTH over the short list differs from the committed validatorsSeenRoot.
	dropped := f.members[0]
	forged := cloneWitness(w)
	shortIDs := make([]ports.NodeID, 0, len(forged.IDs)-1)
	for _, id := range forged.IDs {
		if id != dropped {
			shortIDs = append(shortIDs, id)
		}
	}
	forged.IDs = shortIDs
	delete(forged.Members, dropped)

	got, reason := f.c.RecomputeMatureNow(f.root, forged)
	if got {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness missing a seated member was accepted")
	}
	if !errors.Is(reason, ErrRecomputeSeenSetIncomplete) {
		t.Fatalf("omitted member should stall on ErrRecomputeSeenSetIncomplete; got %v", reason)
	}
}

// TestRecomputeMatureNow_InjectedMemberRejects is the dual: INJECTING an extra id into the witnessed
// list also breaks set-completeness (a different MTH), so a prover cannot pad the set either.
func TestRecomputeMatureNow_InjectedMemberRejects(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)

	forged := cloneWitness(w)
	extra := idOf(key(99999)) // not a seated member
	forged.IDs = append(forged.IDs, extra)
	// Give the extra a bogus member witness so the code reaches the completeness check first.
	forged.Members[extra] = forged.Members[f.members[0]]

	got, reason := f.c.RecomputeMatureNow(f.root, forged)
	if got {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness with an injected extra member was accepted")
	}
	if !errors.Is(reason, ErrRecomputeSeenSetIncomplete) {
		t.Fatalf("injected member should stall on ErrRecomputeSeenSetIncomplete; got %v", reason)
	}
}

// TestRecomputeMatureNow_ConfigFromOwnConfig is the MANDATORY C-6 ABLATION TEETH — the reason this
// predicate is next. The recompute's verdict must depend ONLY on the committed member state (own
// StateRoot) and the box's OWN genesis config — NEVER on a config value carried in the witness. The
// test proves it by running the SAME witness against boxes with WIDELY DIFFERENT own config and
// asserting the verdict is INVARIANT.
//
// TEETH (why the config values are chosen at the maturity knee): the committed set is 5 diverse
// bonds whose weights (8,6,5,4,4; total 28M) give a Nakamoto coefficient of 2 (the two heaviest,
// 8+6=14M, exceed total/3=9.4M). So MatureCoefficient = 2. A box with MatureValidators=2 is mature;
// a box with MatureValidators=3 is immature. A recompute that (wrongly) read MatureValidators from
// the WITNESS would make these two boxes AGREE (both reading the same witnessed threshold) — masking
// the bug. A correct C-6 fold reads OWN MatureValidators, so they DIVERGE: one mature, one not. The
// negative control (recomputeMatureNowConfigFromWitness, the injected config-from-witness variant)
// makes the boxes AGREE on a witnessed threshold — the RED this ablation reddens; the real
// own-config fold is the GREEN.
func TestRecomputeMatureNow_ConfigFromOwnConfig(t *testing.T) {
	// Both boxes hold the SAME committed state (5 diverse bonds, coefficient 2). Build the witness
	// once from a reference box, then verify it against boxes with DIFFERENT own MatureValidators.
	ref := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := ref.witnessFor(t)

	// matureBox: own MatureValidators=2 (== coefficient) ⇒ mature. immatureBox: own
	// MatureValidators=3 (> coefficient) ⇒ immature. Same committed root, same witness.
	matureBox := ref.c
	immatureBox := buildBoxWithConfig(t, 3, 1)

	metMature, rMature := matureBox.RecomputeMatureNow(ref.root, w)
	metImmature, rImmature := immatureBox.RecomputeMatureNow(ref.root, w)
	if rMature != nil || rImmature != nil {
		t.Fatalf("neither box should stall; rMature=%v rImmature=%v", rMature, rImmature)
	}

	// C-6: the OWN-config fold makes the boxes DIVERGE on the same witness (own MatureValidators
	// governs). If they AGREED, the fold read the threshold from the witness — the C-6 violation.
	if metMature == metImmature {
		t.Fatalf("C-6 VIOLATION: boxes with different own MatureValidators (2 vs 3) AGREED on the same "+
			"witness (both %v) — the fold read the threshold from the witness, not own config", metMature)
	}
	if !metMature {
		t.Fatal("fixture invariant: the MatureValidators=2 box should be mature (coefficient=2)")
	}
	if metImmature {
		t.Fatal("fixture invariant: the MatureValidators=3 box should be immature (coefficient=2 < 3)")
	}

	// NEGATIVE CONTROL (the RED): a config-from-witness fold reads the threshold from the witness, so
	// both boxes read the SAME witnessed threshold and AGREE — masking the divergence own config
	// produces. This is exactly the ablation that must go red if config-from-witness is injected.
	wThreshold := 3 // the witness carries a threshold of 3 (an attacker's shifted value)
	metMatureInj := recomputeMatureNowConfigFromWitness(matureBox, ref.root, w, wThreshold, 1, 1<<20)
	metImmatureInj := recomputeMatureNowConfigFromWitness(immatureBox, ref.root, w, wThreshold, 1, 1<<20)
	if metMatureInj != metImmatureInj {
		t.Fatal("negative-control precondition: the config-from-witness variant should make the boxes " +
			"AGREE (both read the witnessed threshold) — if they diverge, the negative control is not " +
			"actually reading config from the witness")
	}
	if metMatureInj {
		t.Fatal("negative-control precondition: a witnessed threshold of 6 over coefficient 5 should be immature")
	}
	// The teeth: own-config DIVERGES (metMature != metImmature, asserted above) while
	// config-from-witness AGREES (metMatureInj == metImmatureInj, asserted here) — proving the real
	// fold reads OWN config, and a config-from-witness regression would be caught by the divergence
	// flipping to agreement.
}

// buildBoxWithConfig makes an objective v5 box with the given own MatureValidators/OperatorMargin
// and no committed state of its own — it only holds a root a witness is verified against. It uses
// the SAME anchors (key(1)/key(2)) and MinBond as buildMaturityFixture, so the ONLY config knob that
// differs from the fixture box is MatureValidators — isolating the C-6 teeth to the threshold (a box
// without the fixture's anchors would fold the anchor members and skew the coefficient, confounding
// the teeth). Used to verify one witness against boxes with a different own threshold.
func buildBoxWithConfig(t *testing.T, matureValidators, operatorMargin int) *Chain {
	t.Helper()
	c := New(Config{
		Quorum: 2, MinBond: 1 << 20, MatureValidators: matureValidators, OperatorMargin: operatorMargin,
		Anchors: map[ports.NodeID]bool{idOf(key(1)): true, idOf(key(2)): true},
	}, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c
}

// recomputeMatureNowConfigFromWitness is the NEGATIVE-CONTROL injected variant for the C-6 ablation:
// it reproduces RecomputeMatureNow's set-completeness + per-member verification EXACTLY, but reads
// the maturity threshold (and margin / MinBond screen) from the WITNESS-carried parameters instead
// of own config. It exists ONLY in the test to demonstrate that a config-from-witness fold makes
// boxes with different own config AGREE (the C-6 bug), which the real own-config fold does not.
// TEST-ONLY; it touches no production path.
func recomputeMatureNowConfigFromWitness(c *Chain, root ports.Hash, w SeenSetWitness, wMatureValidators, wOperatorMargin int, wMinBond int64) bool {
	// Set-completeness (same as production).
	rootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	if !statehash.Resolve(root, rootKey, w.SeenRootValue, w.SeenRootWitness).IsProvenPresent() {
		return false
	}
	if string(nodeSetMTH(w.IDs)) != string(w.SeenRootValue) {
		return false
	}
	sizes := make([]int64, 0, len(w.IDs))
	domainWeight := make(map[uint64]int64)
	var zeroDomainWeights []int64
	var total int64
	for _, id := range w.IDs {
		mw := w.Members[id]
		if c.cfg.Anchors[id] || mw.Slashed {
			continue
		}
		// THE INJECTED DEFECT: the eligibility screen reads the WITNESS-carried MinBond, not own cfg.
		if mw.Bonded < wMinBond {
			continue
		}
		sizes = append(sizes, mw.Bonded)
		total += mw.Bonded
		if mw.DomainPresent && mw.Domain != 0 {
			domainWeight[mw.Domain] += mw.Bonded
		} else {
			zeroDomainWeights = append(zeroDomainWeights, mw.Bonded)
		}
	}
	if total == 0 {
		return 0 >= wMatureValidators
	}
	nakamotoBonds := nakamotoCoefficient(sizes, total)
	margin := wOperatorMargin // THE INJECTED DEFECT: witness-carried margin, not own cfg.
	if margin < 1 {
		margin = 1
	}
	nakamotoOperators := nakamotoBonds / margin
	groups := make([]int64, 0, len(domainWeight)+len(zeroDomainWeights))
	for _, weight := range domainWeight {
		groups = append(groups, weight)
	}
	groups = append(groups, zeroDomainWeights...)
	nakamotoDomains := nakamotoCoefficient(groups, total)
	coeff := nakamotoOperators
	if nakamotoDomains < coeff {
		coeff = nakamotoDomains
	}
	return coeff >= wMatureValidators // THE INJECTED DEFECT: witness-carried threshold.
}

// cloneWitness deep-copies a SeenSetWitness's Members map so an ablation can mutate one entry
// without disturbing the shared baseline witness.
func cloneWitness(w SeenSetWitness) SeenSetWitness {
	out := w
	out.IDs = append([]ports.NodeID(nil), w.IDs...)
	out.Members = make(map[ports.NodeID]MemberStateWitness, len(w.Members))
	for id, mw := range w.Members {
		out.Members[id] = mw
	}
	return out
}

// TestRecomputeMatureNow_UnprovenRootStalls proves the box stalls (never folds) when the committed
// validatorsSeenRoot leaf cannot be proven present — e.g. a witness verified against the WRONG root.
func TestRecomputeMatureNow_UnprovenRootStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	var wrongRoot ports.Hash
	wrongRoot[0] = 0xff
	got, reason := f.c.RecomputeMatureNow(wrongRoot, w)
	if got {
		t.Fatal("a witness verified against the wrong root must not reach a mature verdict")
	}
	if !errors.Is(reason, ErrRecomputeSeenRootUnproven) {
		t.Fatalf("wrong-root should stall on ErrRecomputeSeenRootUnproven; got %v", reason)
	}
}

// TestRecomputeMatureNow_MissingMemberWitnessStalls proves the box stalls when a completeness-verified
// member has no state witness at all (the map entry is absent). The set reconstructs (digest matches)
// but a member's committed state cannot be verified ⇒ stall, never fold a partial set.
func TestRecomputeMatureNow_MissingMemberWitnessStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	delete(w.Members, f.members[0]) // keep IDs complete (digest matches) but drop a member witness

	got, reason := f.c.RecomputeMatureNow(f.root, w)
	if got {
		t.Fatal("a member with no state witness must stall the fold, not be folded as skipped")
	}
	if !errors.Is(reason, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("missing member witness should stall on ErrRecomputeMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeMatureNow_NeverFlipsWitnessValidateAccept pins the STOP boundary: this increment
// reproduces ONE predicate; it must NOT have flipped WitnessValidateV5 to Accept.
func TestRecomputeMatureNow_NeverFlipsWitnessValidateAccept(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	got, _ := f.c.WitnessValidateV5(v5Block(3), f.root, RecoveryDirective{})
	if got == Accept {
		t.Fatal("STOP boundary violated: WitnessValidateV5 returned ACCEPT — the accept flip (#657) must wait until ALL predicates are reproduced")
	}
}
