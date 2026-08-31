package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the trustless floor-box RECOMPUTE increment 4 (floorbox_recompute_qualifiedCount_v5.go):
// the root-only reproduction of qualifiedCount (the distinct-qualified validator COUNT N that sizes
// the count-quorum floor), replicating increments 1-3's C-1 pattern over the WHOLE bonded map and
// consuming the `slashed`-over-bonded quorum-stack whole-set read.
//
// The HARD ABLATIONS (C-5, red-before-green), each injected and watched to flip the verdict, so a
// green here is not decoration:
//   - FORGED BONDED WEIGHT (C-1): a witness with the right members but a forged per-member bonded
//     weight ⇒ STALL (its inclusion proof fails against the committed root).
//   - FORGED / DROPPED SLASHED BIT (C-1): a witness claiming a committed-slashed member is unslashed
//     (dropping the slash to inflate N), or claiming an unslashed member is slashed (injecting a
//     slash to deflate N) ⇒ STALL (its slashed proof does not verify in that direction).
//   - OMITTED / INJECTED MEMBER: a witness missing/padding a bonded member ⇒ MTH mismatch ⇒ STALL
//     (set-completeness against bondedRoot).
//   - CONFIG-FROM-WITNESS MinBond (C-6, failing-first): a count that read the MinBond eligibility
//     screen from the WITNESS instead of own config would let an attacker inflate N with cheap bonds.
//     The correct own-config count is INVARIANT; the negative control demonstrates the shift.
//
// The recompute NEVER flips WitnessValidateV5 to Accept (the STOP boundary); it reproduces ONE
// predicate.

// qualifiedCountFixture is an objective v5 chain with a populated bonded map whose members span the
// screen (some >= MinBond, some below) and the slashed set (some slashed), plus the committed
// StateRoot and a Prover over its v5 leaf set. A floor box holds root; the Prover stands in for the
// any-of-N witness provider that holds the committed set.
type qualifiedCountFixture struct {
	c      *Chain
	root   ports.Hash
	prover *statehash.Prover
	bonded []ports.NodeID // every bonded id (anchors included — qualifiedCount ranges the whole map)
}

// qcBond is one bonded member: its key, committed weight, and whether it is slashed.
type qcBond struct {
	priv    ed25519.PrivateKey
	size    int64
	slashed bool
}

// buildQualifiedCountFixture seats each bond into the committed bonded map (via a genesis bond
// registration), marks the requested members slashed (white-box, same package), then snapshots the
// committed v5 StateRoot and a Prover over its v5 leaves. Bonds BELOW MinBond are seated too — a
// bond registration commits the declared weight regardless, and qualifiedCount screens it out by the
// `>= MinBond` test, so the fixture can exercise the sub-MinBond screen path with a real committed
// leaf. No anchors: validatorSetSize falls through to qualifiedCount only outside the anchor window,
// so the fixture models the plain whole-bonded count.
func buildQualifiedCountFixture(t *testing.T, minBond int64, bonds []qcBond) qualifiedCountFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: minBond, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, b := range bonds {
		g.BondRegs = append(g.BondRegs, bondReg(b.priv, b.size, ports.Hash{}))
	}
	Sign(g, bonds[0].priv)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Mark the requested members slashed in the committed state BEFORE the snapshot, so the
	// snapshotted StateRoot carries a committed slashed INCLUSION leaf for each (statehash.go
	// stateRootLeavesV5 emits tagSlashed→Present for every id in c.slashed).
	for _, b := range bonds {
		if b.slashed {
			id := idOf(b.priv)
			if _, ok := c.bonded[id]; !ok {
				t.Fatalf("fixture precondition: slash target %x is not bonded", id[:])
			}
			c.slashed[id] = true
		}
	}

	if !c.objective() {
		t.Fatal("fixture precondition: the chain must be objective for qualifiedCount")
	}

	bondedIDs := make([]ports.NodeID, 0, len(c.bonded))
	for id := range c.bonded {
		bondedIDs = append(bondedIDs, id)
	}
	if len(bondedIDs) == 0 {
		t.Fatal("fixture precondition: bonded must be non-empty")
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
	return qualifiedCountFixture{c: c, root: root, prover: prover, bonded: bondedIDs}
}

// witnessFor builds a well-formed QualifiedCountWitness proving the COMPLETE bonded set against the
// committed root: the bondedRoot digest leaf + one QualifiedMemberWitness (bonded weight inclusion
// proof + slashed present/absent proof) per bonded id.
func (f qualifiedCountFixture) witnessFor(t *testing.T) QualifiedCountWitness {
	t.Helper()
	rootKey := statehash.Key(tagBondedRoot, nil)
	rootVal := nodeSetMTHFromInt64(f.c.bonded)
	rootProof, err := f.prover.Prove(rootKey)
	if err != nil {
		t.Fatalf("Prove(bondedRoot): %v", err)
	}
	ids := make([]ports.NodeID, 0, len(f.c.bonded))
	members := make(map[ports.NodeID]QualifiedMemberWitness, len(f.c.bonded))
	for id := range f.c.bonded {
		ids = append(ids, id)
		members[id] = f.memberWitness(t, id)
	}
	return QualifiedCountWitness{IDs: ids, BondedRootWitness: rootProof, BondedRootValue: rootVal, Members: members}
}

// memberWitness builds one bonded member's bonded-weight + slashed witness.
func (f qualifiedCountFixture) memberWitness(t *testing.T, id ports.NodeID) QualifiedMemberWitness {
	t.Helper()
	mw := QualifiedMemberWitness{}
	bp, err := f.prover.Prove(statehash.Key(tagBonded, id[:]))
	if err != nil {
		t.Fatalf("Prove(bonded[%x]): %v", id[:], err)
	}
	mw.Bonded = f.c.bonded[id]
	mw.BondedProof = bp
	sp, err := f.prover.Prove(statehash.Key(tagSlashed, id[:]))
	if err != nil {
		t.Fatalf("Prove(slashed[%x]): %v", id[:], err)
	}
	mw.Slashed = f.c.slashed[id]
	mw.SlashedProof = sp
	return mw
}

// cloneQualifiedWitness deep-copies a QualifiedCountWitness's Members map + IDs slice so an ablation
// can mutate one entry without disturbing the shared baseline witness.
func cloneQualifiedWitness(w QualifiedCountWitness) QualifiedCountWitness {
	out := w
	out.IDs = append([]ports.NodeID(nil), w.IDs...)
	out.Members = make(map[ports.NodeID]QualifiedMemberWitness, len(w.Members))
	for id, mw := range w.Members {
		out.Members[id] = mw
	}
	return out
}

// mixedQCBonds is a bonded set spread across the screen: three members >= MinBond and unslashed
// (counted), one >= MinBond but SLASHED (screened by the slashed bit), one BELOW MinBond (screened
// by the eligibility test). qualifiedCount over this set is 3.
func mixedQCBonds() []qcBond {
	const minBond = int64(1) << 20
	return []qcBond{
		{key(70), 4 << 20, false},     // counted
		{key(71), 3 << 20, false},     // counted
		{key(72), 2 << 20, false},     // counted
		{key(73), 5 << 20, true},      // >= MinBond but slashed → NOT counted
		{key(74), minBond / 2, false}, // below MinBond → NOT counted
	}
}

// TestRecomputeQualifiedCount_MatchesFullNode is the equivalence anchor: over the SAME committed
// state, the trustless recompute's N equals the full node's qualifiedCount() — with the count
// spanning the >= MinBond screen AND the slashed screen.
func TestRecomputeQualifiedCount_MatchesFullNode(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)

	got, reason := f.c.RecomputeQualifiedCount(f.root, w)
	if reason != nil {
		t.Fatalf("recompute stalled unexpectedly: %v", reason)
	}
	want := f.c.qualifiedCount()
	if got != want {
		t.Fatalf("recompute N %d != full node qualifiedCount() %d", got, want)
	}
	if got != 3 {
		t.Fatalf("fixture invariant: expected N=3 (3 counted, 1 slashed, 1 sub-MinBond); got %d", got)
	}
}

// TestRecomputeQualifiedCount_ForgedBondedWeightRejects is HARD ABLATION 1 (C-1): a witness with the
// RIGHT members but a FORGED per-member bonded weight makes the recompute STALL — the forged weight's
// inclusion proof does not verify against the committed root.
//
// RED-BEFORE-GREEN: the un-forged witness reaches a verdict; forging one member's weight stalls it.
func TestRecomputeQualifiedCount_ForgedBondedWeightRejects(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeQualifiedCount(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: forge the sub-MinBond member's claimed weight UP past MinBond (which would
	// wrongly count it, N=4) while KEEPING its original inclusion proof (built for the TRUE weight).
	// Resolve against the forged EncodeInt64(weight) fails ⇒ the member's bonded is unproven ⇒ stall.
	victim := idOf(key(74))
	forged := cloneQualifiedWitness(w)
	mw := forged.Members[victim]
	mw.Bonded = 8 << 20 // forged up past MinBond
	forged.Members[victim] = mw

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("C-1 VIOLATION: a forged per-member bonded weight was accepted (N=%d) — the count is forgeable", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedMemberStateUnproven) {
		t.Fatalf("forged bonded should stall on ErrRecomputeQualifiedMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_DroppedSlashRejects is HARD ABLATION 2a (C-1): a witness claiming a
// COMMITTED-SLASHED member is UNSLASHED — the inflation attack (it would wrongly count the slashed
// member, N=4). The claimed-unslashed member needs a NON-INCLUSION proof, but slashed[id] IS
// committed present, so a non-inclusion proof cannot exist / verify ⇒ STALL.
func TestRecomputeQualifiedCount_DroppedSlashRejects(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeQualifiedCount(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: the slashed member (key(73)) is committed slashed=present. Claim it
	// unslashed while KEEPING its (inclusion) proof. Resolve as a non-inclusion proof of a
	// committed-present leaf fails ⇒ stall. A withholding prover cannot drop a slash to inflate N.
	victim := idOf(key(73))
	forged := cloneQualifiedWitness(w)
	mw := forged.Members[victim]
	mw.Slashed = false // claim unslashed (the inflation attack)
	forged.Members[victim] = mw

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("C-1 VIOLATION: a dropped slash (claimed-unslashed committed-slashed member) was accepted (N=%d) — N is inflatable", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedMemberStateUnproven) {
		t.Fatalf("dropped slash should stall on ErrRecomputeQualifiedMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_InjectedSlashRejects is HARD ABLATION 2b (C-1): a witness claiming an
// UNSLASHED member is SLASHED — the deflation attack (it would wrongly screen a counted member,
// N=2). The claimed-slashed member needs an INCLUSION proof of Present, but slashed[id] is committed
// ABSENT, so an inclusion proof cannot exist / verify ⇒ STALL.
func TestRecomputeQualifiedCount_InjectedSlashRejects(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)

	// THE INJECTED DEFECT: a counted member (key(70)) is committed unslashed. Claim it slashed while
	// KEEPING its (non-inclusion) proof. Resolve as an inclusion proof of Present fails ⇒ stall.
	victim := idOf(key(70))
	forged := cloneQualifiedWitness(w)
	mw := forged.Members[victim]
	mw.Slashed = true // claim slashed (the deflation attack)
	forged.Members[victim] = mw

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("C-1 VIOLATION: an injected slash (claimed-slashed unslashed member) was accepted (N=%d) — N is deflatable", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedMemberStateUnproven) {
		t.Fatalf("injected slash should stall on ErrRecomputeQualifiedMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_OmittedMemberRejects is HARD ABLATION 3: a witness MISSING a bonded
// member reconstructs a DIFFERENT MTH than the committed bondedRoot ⇒ STALL. A withholding prover
// cannot shrink the counted set, because the digest binds the complete id-set.
func TestRecomputeQualifiedCount_OmittedMemberRejects(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)
	if _, reason := f.c.RecomputeQualifiedCount(f.root, w); reason != nil {
		t.Fatalf("baseline should reach a verdict with no stall; reason=%v", reason)
	}

	// THE INJECTED DEFECT: drop one member from the witnessed id-list (and its witness). The
	// reconstructed nodeSetMTH over the short list differs from the committed bondedRoot.
	dropped := idOf(key(72))
	forged := cloneQualifiedWitness(w)
	shortIDs := make([]ports.NodeID, 0, len(forged.IDs)-1)
	for _, id := range forged.IDs {
		if id != dropped {
			shortIDs = append(shortIDs, id)
		}
	}
	forged.IDs = shortIDs
	delete(forged.Members, dropped)

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("SET-COMPLETENESS VIOLATION: a witness missing a bonded member was accepted (N=%d)", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedBondedSetIncomplete) {
		t.Fatalf("omitted member should stall on ErrRecomputeQualifiedBondedSetIncomplete; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_InjectedMemberRejects is the dual: INJECTING an extra id into the
// witnessed list also breaks set-completeness (a different MTH), so a prover cannot pad the set to
// inflate N either.
func TestRecomputeQualifiedCount_InjectedMemberRejects(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)

	forged := cloneQualifiedWitness(w)
	extra := idOf(key(99997)) // not a bonded member
	forged.IDs = append(forged.IDs, extra)
	forged.Members[extra] = forged.Members[idOf(key(70))] // bogus witness; completeness fails first

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("SET-COMPLETENESS VIOLATION: a witness with an injected extra member was accepted (N=%d)", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedBondedSetIncomplete) {
		t.Fatalf("injected member should stall on ErrRecomputeQualifiedBondedSetIncomplete; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_MinBondFromConfig is the C-6 ABLATION (failing-first): N must depend
// ONLY on the committed member weights/slashed bits (own StateRoot) and the box's OWN MinBond —
// NEVER on a MinBond carried in the witness. The test proves it by showing that a MinBond-from-witness
// count (the negative control) yields a DIFFERENT N than the real own-config count: an attacker who
// could carry a lax MinBond in the witness would inflate N by admitting the sub-MinBond member. The
// real count is INVARIANT to any witness-carried MinBond.
//
// RED-BEFORE-GREEN (evidence, reported in the PR): the negative control
// (recomputeQualifiedCountMinBondFromWitness) with a lax MinBond of 1 counts the sub-MinBond member
// too (N=4); the production own-config fold does not (N=3). That divergence is the C-6 teeth — a
// MinBond-from-witness regression would inflate the production N to match the lax witness.
func TestRecomputeQualifiedCount_MinBondFromConfig(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)

	// PRODUCTION (own MinBond): N=3 (the sub-MinBond member key(74) is screened out).
	got, reason := f.c.RecomputeQualifiedCount(f.root, w)
	if reason != nil {
		t.Fatalf("recompute stalled unexpectedly: %v", reason)
	}
	if got != 3 {
		t.Fatalf("fixture invariant: production own-config N must be 3; got %d", got)
	}

	// NEGATIVE CONTROL (the RED): a MinBond-from-witness count reads a LAX MinBond of 1 (a stand-in
	// for a witness-carried screen), so the sub-MinBond member key(74) (weight minBond/2 >= 1) is
	// wrongly counted ⇒ N=4. The production own-config fold does not; the divergence is the C-6 teeth.
	nInj := recomputeQualifiedCountMinBondFromWitness(f.c, f.root, w, 1)
	if nInj != 4 {
		t.Fatalf("negative-control precondition: a lax witnessed MinBond=1 must count the sub-MinBond member (N=4); got %d", nInj)
	}
	if got == nInj {
		t.Fatal("C-6 VIOLATION: the production count and the MinBond-from-witness count AGREED — the production fold read MinBond from the witness, not own config")
	}
}

// recomputeQualifiedCountMinBondFromWitness is the NEGATIVE-CONTROL injected variant for the C-6
// ablation: it reproduces RecomputeQualifiedCount's set-completeness + per-member verification
// EXACTLY, but reads the MinBond eligibility screen from a CALLER-supplied parameter (a stand-in for
// a witness-carried screen) instead of own config. It exists ONLY in the test to demonstrate that a
// MinBond-from-witness count inflates N past the own-config count. TEST-ONLY; it touches no
// production path.
func recomputeQualifiedCountMinBondFromWitness(c *Chain, root ports.Hash, w QualifiedCountWitness, minBond int64) int {
	rootKey := statehash.Key(tagBondedRoot, nil)
	if !statehash.Resolve(root, rootKey, w.BondedRootValue, w.BondedRootWitness).IsProvenPresent() {
		return -1
	}
	if string(nodeSetMTH(w.IDs)) != string(w.BondedRootValue) {
		return -1
	}
	n := 0
	for _, id := range w.IDs {
		mw := w.Members[id]
		if !statehash.Resolve(root, statehash.Key(tagBonded, id[:]), statehash.EncodeInt64(mw.Bonded), mw.BondedProof).IsProvenPresent() {
			return -1
		}
		var slashedVal []byte
		if mw.Slashed {
			slashedVal = statehash.Present
		}
		sr := statehash.Resolve(root, statehash.Key(tagSlashed, id[:]), slashedVal, mw.SlashedProof)
		if mw.Slashed && !sr.IsProvenPresent() {
			return -1
		}
		if !mw.Slashed && !sr.IsProvenAbsent() {
			return -1
		}
		if mw.Bonded >= minBond && !mw.Slashed { // THE INJECTED DEFECT: caller-supplied minBond, not own config.
			n++
		}
	}
	return n
}

// TestRecomputeQualifiedCount_UnprovenBondedRootStalls proves the box stalls (never counts) when the
// committed bondedRoot leaf cannot be proven present — e.g. a corrupted claimed value. The
// corrupted value makes the reconstructed MTH differ (set-incomplete) or the presence proof fail
// (unproven); either is a valid stall.
func TestRecomputeQualifiedCount_UnprovenBondedRootStalls(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)

	forged := cloneQualifiedWitness(w)
	forged.BondedRootValue = append([]byte(nil), w.BondedRootValue...)
	forged.BondedRootValue[0] ^= 0xff

	got, reason := f.c.RecomputeQualifiedCount(f.root, forged)
	if reason == nil {
		t.Fatalf("a corrupted bondedRoot value must not reach a count (N=%d)", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedBondedRootUnproven) && !errors.Is(reason, ErrRecomputeQualifiedBondedSetIncomplete) {
		t.Fatalf("corrupted bondedRoot should stall on a bonded-root error; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_MissingMemberWitnessStalls proves the box stalls when a
// completeness-verified bonded member has no witness at all (the map entry is absent). The set
// reconstructs (digest matches) but a member's state cannot be verified ⇒ stall, never count a
// partial set.
func TestRecomputeQualifiedCount_MissingMemberWitnessStalls(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	w := f.witnessFor(t)
	delete(w.Members, idOf(key(70))) // keep IDs complete (digest matches) but drop a witness

	got, reason := f.c.RecomputeQualifiedCount(f.root, w)
	if reason == nil {
		t.Fatalf("a member with no witness must stall the count (N=%d), not be counted/skipped", got)
	}
	if !errors.Is(reason, ErrRecomputeQualifiedMemberStateUnproven) {
		t.Fatalf("missing member witness should stall on ErrRecomputeQualifiedMemberStateUnproven; got %v", reason)
	}
}

// TestRecomputeQualifiedCount_NeverFlipsWitnessValidateAccept pins the STOP boundary: this increment
// reproduces ONE predicate; it must NOT have flipped WitnessValidateV5 to Accept.
func TestRecomputeQualifiedCount_NeverFlipsWitnessValidateAccept(t *testing.T) {
	const minBond = int64(1) << 20
	f := buildQualifiedCountFixture(t, minBond, mixedQCBonds())
	got, _ := f.c.WitnessValidateV5(v5Block(3), f.root, RecoveryDirective{})
	if got == Accept {
		t.Fatal("STOP boundary violated: WitnessValidateV5 returned ACCEPT — the accept flip (#657) must wait until ALL predicates are reproduced")
	}
}
