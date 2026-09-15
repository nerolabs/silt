package chain

// A block that touches more than one id-keyed transition class folds to the root real apply()
// commits, and every committed key it changes is folded exactly ONCE.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE PROPERTY, AND WHY IT NEEDS ITS OWN GATES
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Bond registrations (B), TTL expiry (T) and slashes (S) all write the bonded and qualified
// keyspaces, and B and T both move the TTL due-buckets. Each keyspace carries a whole-set digest
// scalar at ONE fixed key, whose value is an MTH over the complete post-state member list — a
// function of every class's delta, not of one class's. A class that derived its post-set from the
// anchored pre-state alone would compute a digest for a state no block ever commits: on a block
// carrying a registration and a slash, nodeSetMTH(pre u fresh) and nodeSetMTH(pre \ culprit) are
// both wrong, and the committed value nodeSetMTH((pre u fresh) \ culprit) is neither.
//
// So the classes are composed over one running post-state, in apply's order (B -> T -> S), and each
// digest, per-member leaf and bucket leaf is emitted once from the state the last class leaves.
// These gates hold both halves: the ROOT the composition folds to, and the ONE-OP-PER-KEY shape
// that makes that root well-defined.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// WHY ONE OP PER KEY IS A PROPERTY AND NOT A TIDINESS RULE
// ────────────────────────────────────────────────────────────────────────────────────────
//
// statehash.FoldChangedPaths resolves two ops on one key by LAST WRITE — its step 5 replays deletes
// then updates, in slice order, with no dedup and no duplicate guard. Duplicate ops derived from one
// pre-state witness carry a byte-identical OldValue and Proof, so every one of them VERIFIES at step
// 1 and nothing rejects them. A duplicate digest op is therefore SILENT: the box folds to whichever
// class was appended last, and if the proposer's own fold made the same choice the terminal root
// equality passes by construction. That is a box that accepts what a full node rejects, reached with
// no forgery at all. TestADuplicateDigestOpWouldDivergeTheFold is the vacuity guard: it shows the
// invariant is load-bearing by folding the duplicate and watching the root move.
//
// A duplicate DELETE is loud rather than silent — the second delete finds no key and the fold stalls
// — which is what a floor box pointed at a live swarm actually met: every proposer renews its bond
// as it proposes, and under a short TTL a renewal lands on the very height that bond falls due, so
// the registration's bucket move and the sweep's bucket drain named one leaf twice.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE ORACLE IS REAL EXECUTION
// ────────────────────────────────────────────────────────────────────────────────────────
//
// Every gate below compares the box against apply() + StateRootForVersion(5) on a dry-run clone —
// the same oracle a full node uses — never against a hand-built model of the transition, which would
// share the box's own blind spot. The single-class controls prove the witness builders are not
// degenerate, so a compound agreement is produced by the composition and not by a fixture that
// cannot disagree with anything.

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// ══════════════════════════════════════════════════════════════════════════════
// THE PREDICATES — factored so their teeth can be asserted without a red gate
// ══════════════════════════════════════════════════════════════════════════════

// composeFoldsToHonestRoot reports the failure when a compound block does NOT reproduce the root
// real apply() commits. Empty string ⇒ the box agrees with the chain.
func composeFoldsToHonestRoot(label string, stall error, boxRoot, honest ports.Hash) string {
	if boxRoot != honest {
		return fmt.Sprintf(
			"%s: the box folded a compound block to a root real apply() does not commit.\n"+
				"  box    (what the composition folded) = %x\n"+
				"  honest (what real apply() yields)    = %x\n"+
				"  The classes are composed over one running post-state in apply's order (B -> T -> S).\n"+
				"  A divergence here means a class is reading a state another class already wrote, or\n"+
				"  writing a key a later class overwrites. Diff the two post-sets before changing an emit.",
			label, boxRoot[:], honest[:])
	}
	if stall != nil {
		return fmt.Sprintf(
			"%s: the box folded to the honest root (%x) and still refused the block: %v\n"+
				"  The fold and the terminal equality agree, so the refusal came from earlier in the\n"+
				"  composition — a scope gate, a missing witness, or a pre-state anchor.",
			label, boxRoot[:], stall)
	}
	return ""
}

// composeOneOpPerKey reports the failure when the op set names any committed key more than once.
// Empty string ⇒ every changed key is folded exactly once.
func composeOneOpPerKey(label string, ops []statehash.FoldOp) string {
	count := map[string]int{}
	for i := range ops {
		count[string(ops[i].Key)]++
	}
	for i := range ops {
		k := string(ops[i].Key)
		if count[k] < 2 {
			continue
		}
		var values [][]byte
		for j := range ops {
			if string(ops[j].Key) == k {
				values = append(values, ops[j].NewValue)
			}
		}
		return fmt.Sprintf(
			"%s: %d fold ops land on the committed key %x, values %x.\n"+
				"  The fold resolves a duplicate key by LAST WRITE with no duplicate guard, so a second op\n"+
				"  on one key either silently decides the folded root by slice position or — for a delete —\n"+
				"  stalls on a key the first op already removed. Each class must contribute to the running\n"+
				"  post-state rather than emit its own op.",
			label, count[k], ops[i].Key, values)
	}
	return ""
}

// ══════════════════════════════════════════════════════════════════════════════
// THE COMPOUND WITNESS — honest, complete, and built the way the box derives
// ══════════════════════════════════════════════════════════════════════════════

// composeCompoundWitness builds the full honest witness for a block carrying any of E/R, Slashes and
// BondRegs, off the slashFixture chain. Every pre-set, screen, bucket and changed leaf is the TRUE
// committed pre-state — nothing here is forged.
func composeCompoundWitness(t *testing.T, f slashFixture, b Block, affectedBuckets []uint64) StateRootWitness {
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

// composeSlashPlusBondRegBlock returns the S+B compound block — one slash of the fixture culprit, one
// fresh bond registration, one E/R entry — with its honest witness.
func composeSlashPlusBondRegBlock(t *testing.T, f slashFixture) (Block, StateRootWitness) {
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
	return b, composeCompoundWitness(t, f, b, []uint64{h + f.c.cfg.BondTTLBlocks + 1})
}

// composeFoldOrFail folds ops and fails the test on a fold error, naming what a stall means here.
func composeFoldOrFail(t *testing.T, label string, prevRoot ports.Hash, ops []statehash.FoldOp) ports.Hash {
	t.Helper()
	root, err := statehash.FoldChangedPaths(prevRoot, ops)
	if err != nil {
		t.Fatalf("%s: FoldChangedPaths stalled on an HONEST compound witness: %v\n"+
			"  Every op's OldValue is the true committed pre-state, so none can fail its proof. A stall\n"+
			"  here is the composition asking the fold to replay a write the leaf set cannot take — most\n"+
			"  often a delete of a key an earlier op already removed.", label, err)
	}
	return root
}

// ══════════════════════════════════════════════════════════════════════════════
// S + B — a slash and a bond registration in one block
// ══════════════════════════════════════════════════════════════════════════════

func TestCompoundSlashPlusBondRegFoldsToTheHonestRoot(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := composeSlashPlusBondRegBlock(t, f)

	// GROUND TRUTH: the real transition, not a model of it.
	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	culprit := ports.HashBytes(pubOf(f.culprit))
	if _, stillBonded := clone.bonded[culprit]; stillBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() left the slashed culprit BONDED, so the slash and the\n" +
			"  registration do not actually disagree about the bonded set.")
	}
	if _, freshBonded := clone.bonded[ports.HashBytes(pubOf(key(91001)))]; !freshBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() did not bond the fresh registration, so class B\n" +
			"  contributes nothing for class S to compose with.")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("the box refused an honest compound S+B block: %v", err)
	}
	if msg := composeOneOpPerKey("S+B", ops); msg != "" {
		t.Fatalf("%s", msg)
	}
	boxRoot := composeFoldOrFail(t, "S+B", f.prevRoot, ops)
	stall := recomputeViaHead(f.c, f.prevRoot, honest, b, w)
	if msg := composeFoldsToHonestRoot("S+B", stall, boxRoot, honest); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// A registration and a slash of the SAME id in one block. apply screens a registration against the
// PRE-state slashed set, so the bond is recorded and then stripped again inside one apply: the
// bonded and qualified leaves end exactly where they started, and the committed leaf set never
// records them moving. A composition that emitted the registration's write and the slash's delete
// separately would ask the fold to remove a leaf the pre-state never held, and the block would stall
// — safe, and unable to judge a block the chain accepted.
func TestCompoundBondRegSlashedInTheSameBlockLeavesNoNetWrite(t *testing.T) {
	f := buildSlashFixture(t)
	prev, h := f.c.Head()
	fresh := key(91002)
	fid := ports.HashBytes(pubOf(fresh))
	b := Block{
		Version:  BlockVersionWitnessable,
		Height:   h,
		Prev:     prev,
		Entries:  []ports.Entry{entry(40)},
		BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)},
		Slashes:  []Equivocation{slashProof(fresh, prev, 0x41, 0x42)},
	}
	w := composeCompoundWitness(t, f, b, []uint64{h + f.c.cfg.BondTTLBlocks + 1})

	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, bonded := clone.bonded[fid]; bonded {
		t.Fatalf("FIXTURE IS VACUOUS — apply() left the id bonded, so the registration was not stripped\n" +
			"  and there is no write that nets back to the pre-state.")
	}
	if !clone.slashed[fid] {
		t.Fatalf("FIXTURE IS VACUOUS — apply() did not slash the id, so only class B ran")
	}
	if _, wroteRegHeight := clone.bondRegHeight[fid]; !wroteRegHeight {
		t.Fatalf("FIXTURE IS VACUOUS — apply() recorded no bondRegHeight for the id, so the registration\n" +
			"  was screened out rather than applied and then stripped")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("the box refused an honest register-and-slash block: %v", err)
	}
	if msg := composeOneOpPerKey("B+S same id", ops); msg != "" {
		t.Fatalf("%s", msg)
	}
	for _, tag := range []string{tagBonded, tagQualified} {
		k := statehash.Key(tag, fid[:])
		for i := range ops {
			if bytes.Equal(ops[i].Key, k) {
				t.Fatalf("the box folded an op for %s||id (NewValue %x), but the committed leaf never moved:\n"+
					"  the registration wrote it and the slash removed it inside one apply. Folding it asks\n"+
					"  the trie to delete a leaf the pre-state does not hold.", composeTagName(tag), ops[i].NewValue)
			}
		}
	}
	boxRoot := composeFoldOrFail(t, "B+S same id", f.prevRoot, ops)
	stall := recomputeViaHead(f.c, f.prevRoot, honest, b, w)
	if msg := composeFoldsToHonestRoot("B+S same id", stall, boxRoot, honest); msg != "" {
		t.Fatalf("%s", msg)
	}
}

func composeTagName(tag string) string { return string(bytes.TrimRight([]byte(tag), "\x00")) }

// ══════════════════════════════════════════════════════════════════════════════
// S + T — a slash and a firing TTL sweep in one block
// ══════════════════════════════════════════════════════════════════════════════

func TestCompoundSlashPlusTTLSweepFoldsToTheHonestRoot(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	prev, _ := f.c.Head()
	b.Slashes = []Equivocation{slashProof(f.proposer, prev, 0x41, 0x42)}

	expired := f.expiredMembers()
	if len(expired) == 0 {
		t.Fatalf("FIXTURE IS VACUOUS — no expired members in dueBucket[%d], so class T does not dispatch", f.sweepH)
	}
	w := f.ttlSweepWitness(t, b, expired)
	for _, wr := range stateRootSlashWriteSet(b, idSet(f.preIDsBonded()), idSet(f.preIDsQualified())) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, stillBonded := clone.bonded[ports.HashBytes(pubOf(f.proposer))]; stillBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() left the slashed culprit BONDED on the S+T block")
	}
	if _, stillBonded := clone.bonded[expired[0]]; stillBonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() did not expire the swept member, so class T\n" +
			"  contributes nothing for class S to compose with.")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("the box refused an honest compound S+T block: %v", err)
	}
	if msg := composeOneOpPerKey("S+T", ops); msg != "" {
		t.Fatalf("%s", msg)
	}
	boxRoot := composeFoldOrFail(t, "S+T", f.prevRoot, ops)
	stall := recomputeViaHead(f.c, f.prevRoot, honest, b, w)
	if msg := composeFoldsToHonestRoot("S+T", stall, boxRoot, honest); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// B + T on ONE bucket — the block a live swarm produces constantly
// ══════════════════════════════════════════════════════════════════════════════

// A validator renews its bond at the very height that bond falls due. The registration vacates
// dueBucket[h] and the sweep drains it: one leaf, two classes, and the second class must not name it
// again. This is the ordinary block on a swarm where every proposer renews as it proposes — and the
// renewing validator must NOT expire, because apply resets bondRegHeight before the sweep reads it.
func TestCompoundRenewalOnTheSweptBucketFoldsToTheHonestRoot(t *testing.T) {
	f := buildCoExpiryFixture(t)
	b, w := f.renewalOnSweepBlock(t)

	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	if _, bonded := clone.bonded[ports.HashBytes(pubOf(f.renewer))]; !bonded {
		t.Fatalf("FIXTURE IS VACUOUS — real apply() expired the id that renewed at its own due height.\n" +
			"  The registration loop resets bondRegHeight before the sweep reads it, so this id keeps its\n" +
			"  standing; if it does not, the oracle has moved and the gate below means nothing.")
	}
	if _, bonded := clone.bonded[ports.HashBytes(pubOf(f.expirer))]; bonded {
		t.Fatalf("FIXTURE IS VACUOUS — the co-member of the bucket did not expire, so the block carries\n" +
			"  no sweep for the registration to collide with.")
	}
	if n := len(clone.dueBucket[f.sweepH]); n != 0 {
		t.Fatalf("FIXTURE IS VACUOUS — dueBucket[%d] holds %d member(s) after apply, so the bucket leaf is\n"+
			"  not written from both sides", f.sweepH, n)
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("the box refused an honest compound B+T block: %v", err)
	}
	if msg := composeOneOpPerKey("B+T", ops); msg != "" {
		t.Fatalf("%s", msg)
	}
	boxRoot := composeFoldOrFail(t, "B+T", f.prevRoot, ops)
	stall := recomputeViaHead(f.c, f.prevRoot, honest, b, w)
	if msg := composeFoldsToHonestRoot("B+T", stall, boxRoot, honest); msg != "" {
		t.Fatalf("%s", msg)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// The digest agrees with the per-member leaves it covers
// ══════════════════════════════════════════════════════════════════════════════

// A whole-set digest and the per-member leaves of the same keyspace live at different keys, so a
// composition that took the digest from one class's delta could fold a root whose digest counts a
// member its own per-member leaf says was evicted — a state that contradicts itself inside one root.
// This asserts the two agree: the bondedRoot the box folds is the MTH over exactly the post-state
// apply() leaves behind.
func TestTheFoldedDigestAgreesWithItsOwnPerMemberLeaves(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := composeSlashPlusBondRegBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	culprit := ports.HashBytes(pubOf(f.culprit))
	if _, counted := clone.bonded[culprit]; counted {
		t.Fatalf("FIXTURE IS VACUOUS — apply() still counts the culprit in bonded, so the digest and the\n" +
			"  per-member leaf cannot disagree about it")
	}

	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	perMemberKey := statehash.Key(tagBonded, culprit[:])
	found := false
	for i := range ops {
		if !bytes.Equal(ops[i].Key, perMemberKey) {
			continue
		}
		found = true
		if ops[i].NewValue != nil {
			t.Fatalf("the per-member leaf bonded||culprit is not a DELETE (%x); real apply() evicts a "+
				"slashed culprit from bonded", ops[i].NewValue)
		}
	}
	if !found {
		t.Fatalf("no folded op for the per-member leaf bonded||culprit at all — the slash's eviction is missing")
	}

	digestKey := statehash.Key(tagBondedRoot, nil)
	var landed []byte
	for i := range ops {
		if bytes.Equal(ops[i].Key, digestKey) {
			landed = ops[i].NewValue
		}
	}
	if want := nodeSetMTHFromInt64(clone.bonded); !bytes.Equal(landed, want) {
		t.Fatalf("the folded bondedRoot is not the MTH over the post-state bonded set:\n"+
			"  folded = %x\n  apply  = %x\n"+
			"  The digest covers the same membership the per-member leaves above record; a disagreement\n"+
			"  means one class's delta reached the digest and another's did not.", landed, want)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// The one-op-per-key invariant is load-bearing
// ══════════════════════════════════════════════════════════════════════════════

// TestADuplicateDigestOpWouldDivergeTheFold is the vacuity guard for composeOneOpPerKey. It takes the
// box's real op set, appends a SECOND bondedRoot op carrying the value a class deriving from the
// pre-state alone would compute, and folds both slices. The root moves — so the invariant the gates
// above assert is what keeps the folded root well-defined, and not a tidiness rule.
func TestADuplicateDigestOpWouldDivergeTheFold(t *testing.T) {
	f := buildSlashFixture(t)
	b, w := composeSlashPlusBondRegBlock(t, f)
	honest := f.applyAndCommittedRoot(t, b)
	ops, err := assembleOpsViaHead(f.c, f.prevRoot, honest, b, w)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	digestKey := statehash.Key(tagBondedRoot, nil)
	var composed *statehash.FoldOp
	for i := range ops {
		if bytes.Equal(ops[i].Key, digestKey) {
			composed = &ops[i]
		}
	}
	if composed == nil {
		t.Fatalf("the compound block folded no bondedRoot op, so there is no duplicate to construct")
	}

	// What class B alone would have computed: the pre-state bonded set plus the fresh registration,
	// with the slash's eviction never applied.
	singleClass := cloneIDSet(idSet(f.preIDsBonded()))
	singleClass[ports.HashBytes(pubOf(key(91001)))] = struct{}{}
	dup := *composed
	dup.NewValue = composeMTH(singleClass)
	if bytes.Equal(dup.NewValue, composed.NewValue) {
		t.Fatalf("VACUOUS — a single-class bondedRoot value equals the composed one (%x), so a duplicate\n"+
			"  could not change the folded root here and this guard proves nothing.", dup.NewValue)
	}

	one := composeFoldOrFail(t, "composed", f.prevRoot, ops)
	two := composeFoldOrFail(t, "duplicated", f.prevRoot, append(append([]statehash.FoldOp(nil), ops...), dup))
	if one != honest {
		t.Fatalf("CONTROL BROKEN — the composed op set does not fold to the honest root (%x vs %x)", one[:], honest[:])
	}
	if two == one {
		t.Fatalf("the fold produced the same root with and without a duplicate bondedRoot op (%x).\n"+
			"  FoldChangedPaths is documented to resolve a duplicate key by last write; if it now merges\n"+
			"  or rejects instead, composeOneOpPerKey is no longer holding a live hazard and the reason it\n"+
			"  exists must be re-derived.", one[:])
	}
}

func composeMTH(m map[ports.NodeID]struct{}) []byte {
	ids := make([]ports.NodeID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return nodeSetMTH(ids)
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTROLS — the fixtures are not degenerate
// ══════════════════════════════════════════════════════════════════════════════

// TestControlSingleClassBlocksAgree is the non-vacuity control: the SAME witness builder, on a
// slash-ONLY block and a bondreg-ONLY block, agrees with real apply. The compound agreement above is
// produced by the composition, not by a fixture that cannot disagree with anything.
func TestControlSingleClassBlocksAgree(t *testing.T) {
	t.Run("slash-only", func(t *testing.T) {
		f := buildSlashFixture(t)
		prev, h := f.c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			Entries: []ports.Entry{entry(40)},
			Slashes: []Equivocation{slashProof(f.culprit, prev, 0x41, 0x42)}}
		w := composeCompoundWitness(t, f, b, nil)
		honest := f.applyAndCommittedRoot(t, b)
		if err := recomputeViaHead(f.c, f.prevRoot, honest, b, w); err != nil {
			t.Fatalf("CONTROL FAILED — a slash-ONLY block must agree with real apply(), got %v.\n"+
				"  Without this the compound agreement could be an artifact of the witness builder.", err)
		}
	})
	t.Run("bondreg-only", func(t *testing.T) {
		f := buildSlashFixture(t)
		prev, h := f.c.Head()
		fresh := key(91001)
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			Entries:  []ports.Entry{entry(40)},
			BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
		w := composeCompoundWitness(t, f, b, []uint64{h + f.c.cfg.BondTTLBlocks + 1})
		honest := f.applyAndCommittedRoot(t, b)
		if err := recomputeViaHead(f.c, f.prevRoot, honest, b, w); err != nil {
			t.Fatalf("CONTROL FAILED — a bondreg-ONLY block must agree with real apply(), got %v", err)
		}
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// TEETH — every predicate is fed the broken input and must speak up
// ══════════════════════════════════════════════════════════════════════════════

func TestComposeGatesFireOnTheirBreaks(t *testing.T) {
	var a, bh ports.Hash
	a[0], bh[0] = 1, 2

	t.Run("root-gate-fires-on-a-diverged-fold", func(t *testing.T) {
		if msg := composeFoldsToHonestRoot("X", nil, a, bh); msg == "" {
			t.Fatalf("TEETH FAILED: composeFoldsToHonestRoot stayed silent when the box root differed from apply's")
		}
		if msg := composeFoldsToHonestRoot("X", errors.New("stalled"), a, a); msg == "" {
			t.Fatalf("TEETH FAILED: composeFoldsToHonestRoot stayed silent on a refusal at the honest root")
		}
		if msg := composeFoldsToHonestRoot("X", nil, a, a); msg != "" {
			t.Fatalf("TEETH FAILED: composeFoldsToHonestRoot spoke on the fixed input: %s", msg)
		}
	})

	t.Run("one-op-gate-fires-on-a-duplicate", func(t *testing.T) {
		k := statehash.Key(tagBondedRoot, nil)
		other := statehash.Key(tagSlashedRoot, nil)
		diverging := []statehash.FoldOp{
			{Key: k, NewValue: []byte{1}}, {Key: other, NewValue: []byte{9}}, {Key: k, NewValue: []byte{2}},
		}
		if msg := composeOneOpPerKey("X", diverging); msg == "" {
			t.Fatalf("TEETH FAILED: composeOneOpPerKey stayed silent on two ops with different values at one key")
		}
		// An INERT duplicate is still a duplicate: it makes the folded root depend on slice position
		// the moment the two derivations stop agreeing, so the gate refuses it too.
		inert := []statehash.FoldOp{{Key: k, NewValue: []byte{1}}, {Key: k, NewValue: []byte{1}}}
		if msg := composeOneOpPerKey("X", inert); msg == "" {
			t.Fatalf("TEETH FAILED: composeOneOpPerKey stayed silent on two ops with the SAME value at one key")
		}
		duplicateDelete := []statehash.FoldOp{{Key: k}, {Key: k}}
		if msg := composeOneOpPerKey("X", duplicateDelete); msg == "" {
			t.Fatalf("TEETH FAILED: composeOneOpPerKey stayed silent on a duplicated DELETE")
		}
		clean := []statehash.FoldOp{{Key: k, NewValue: []byte{1}}, {Key: other, NewValue: []byte{9}}}
		if msg := composeOneOpPerKey("X", clean); msg != "" {
			t.Fatalf("TEETH FAILED: composeOneOpPerKey spoke on the fixed input: %s", msg)
		}
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// THE CO-EXPIRY FIXTURE — one due-bucket, one renewal and one expiry
// ══════════════════════════════════════════════════════════════════════════════

// coExpiryFixture seats TWO validators whose bonds come due at the same height, and renews only one
// of them on that height. It is the ttlFixture's chain with that second member added, so every
// witness helper is shared and the two fixtures cannot drift apart.
type coExpiryFixture struct {
	ttlFixture
	renewer ed25519.PrivateKey
}

// buildCoExpiryFixture seats a proposer, an expirer and a renewer at genesis (ttl=4 ⇒ all due at
// height 5), advances to h=4 renewing only the proposer, and captures the pre-state there. At h=5
// the renewer re-registers and the expirer does not: one due-bucket, written by the registration and
// by the sweep.
func buildCoExpiryFixture(t *testing.T) coExpiryFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop, expirer, renewer := key(72001), key(72002), key(72003)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30), entry(31)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(expirer, ports.HashBytes(pubOf(expirer)), 4<<20, ports.Hash{}, 5, 2),
		bondRegFull(renewer, ports.HashBytes(pubOf(renewer)), 4<<20, ports.Hash{}, 5, 3),
	)
	Sign(g, prop)
	c.apply(*g)

	for h := uint64(1); h <= 4; h++ {
		prev, _ := c.Head()
		c.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 5, 1)}})
	}
	const sweepH = 5
	due := c.dueBucket[sweepH]
	for _, want := range []ed25519.PrivateKey{expirer, renewer} {
		if _, in := due[ports.HashBytes(pubOf(want))]; !in {
			t.Fatalf("FIXTURE: dueBucket[%d] does not hold both members that come due there (%d present)",
				sweepH, len(due))
		}
	}

	prover, err := statehash.NewProver(c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prover.Root() {
		t.Fatalf("FIXTURE pre-root mismatch: prover=%x chain=%x", prover.Root(), sr)
	}
	return coExpiryFixture{
		ttlFixture: ttlFixture{c: c, prevRoot: prover.Root(), prover: prover,
			proposer: prop, expirer: expirer, sweepH: sweepH},
		renewer: renewer,
	}
}

// renewalOnSweepBlock builds the block at the sweep height that re-registers the renewer, together
// with its honest witness: the due-bucket both classes write, the bucket the renewal moves into, the
// registration's ownership screen, and a proof for every leaf the composition derives.
func (f coExpiryFixture) renewalOnSweepBlock(t *testing.T) (Block, StateRootWitness) {
	t.Helper()
	prev, _ := f.c.Head()
	rid := ports.HashBytes(pubOf(f.renewer))
	b := Block{
		Version:  BlockVersionWitnessable,
		Height:   f.sweepH,
		Prev:     prev,
		Entries:  []ports.Entry{entry(40)},
		BondRegs: []BondReg{bondRegFull(f.renewer, ports.HashBytes(pubOf(f.renewer)), 4<<20, prev, 5, 3)},
	}

	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
	}

	// The bucket that comes due, proven once and carried by both the sweep witness and the
	// registration's bucket list — two readings of one leaf against one root.
	sweptKey := statehash.Key(tagDueBucket, uint64Key(f.sweepH))
	sweptProof, sweptSibs, err := f.prover.ProveWithSiblings(sweptKey)
	if err != nil {
		t.Fatalf("ProveWithSiblings(dueBucket[%d]): %v", f.sweepH, err)
	}
	dueMembers := f.expiredMembers()
	w.DueBucketProof = sweptProof
	w.TTLSweep = &StateRootTTLWitness{
		Height: f.sweepH, Members: dueMembers, BucketProof: sweptProof, BucketDeleteSiblings: sweptSibs,
	}
	w.BondRegBuckets = []StateRootBucketWitness{
		{DueHeight: f.sweepH, PreMembers: dueMembers, Proof: sweptProof, DeleteSiblings: sweptSibs},
	}

	// The bucket the renewal moves INTO, absent pre-state.
	newDue := b.Height + f.c.cfg.BondTTLBlocks + 1
	newProof, err := f.prover.Prove(statehash.Key(tagDueBucket, uint64Key(newDue)))
	if err != nil {
		t.Fatalf("Prove(dueBucket[%d]): %v", newDue, err)
	}
	w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{DueHeight: newDue, Proof: newProof})

	root := ports.HashBytes(pubOf(f.renewer))
	owner, claimed := f.c.bondRootOwner[root]
	w.BondRegScreens = []StateRootBondRegScreen{{
		Root: root, PriorOwner: owner, Claimed: claimed, PriorProven: f.c.bondRootProven[root],
		OwnerProof:  mustProve(f.prover, statehash.Key(tagBondRootOwner, root[:])),
		ProvenProof: mustProve(f.prover, statehash.Key(tagBondRootProven, root[:])),
	}}

	// The renewer's committed bondRegHeight is what tells the registration WHICH bucket to vacate,
	// so it has to be in the bundle before the composition can derive the rest.
	w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, stateRootWrite{
		key: statehash.Key(tagBondRegHeight, rid[:]), newValue: statehash.EncodeUint64(b.Height),
	}))

	idSets, err := f.c.composeIDSetTransition(f.prevRoot, b, w, false)
	if err != nil {
		t.Fatalf("composeIDSetTransition (witness build): %v", err)
	}
	for _, wr := range idSets.netWrites() {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	return b, w
}

// uint64Key encodes a due-height as the big-endian raw key a dueBucket leaf is tagged with.
func uint64Key(h uint64) []byte {
	var hk [8]byte
	putUint64BE(hk[:], h)
	return hk[:]
}
