package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// recomputeViaHead drives the REAL recompute entry with the parent proposer the box door would
// supply: the head's proposer id of the chain standing in for the box's own parent record
// (HeadRef.ProposerID, class 3). Every direct test driver of the recompute goes through here, so
// no test can hand the recompute a witness-chosen parent proposer — there is no such input.
func recomputeViaHead(c *Chain, prevStateRoot, committedStateRoot ports.Hash, b Block, w StateRootWitness) error {
	return recomputeViaHeadOn(c, c.ChainID(), prevStateRoot, committedStateRoot, b, w)
}

// recomputeViaHeadOn is recomputeViaHead with the box's NETWORK IDENTITY supplied explicitly, for
// the drivers whose *Chain is COLD — it holds no blocks, so c.ChainID() is the zero hash and every
// era-4 carrier entry would fail to verify. That is the deployment target, and it is why the
// recompute takes the chain id as a threaded parameter (BoxConfig.ChainID → HeadRef.ChainID → here)
// rather than reading it off the chain: the fold-file pin denies `blocks` by name for exactly this
// reason.
func recomputeViaHeadOn(c *Chain, chainID ports.Hash, prevStateRoot, committedStateRoot ports.Hash, b Block, w StateRootWitness) error {
	parentProposer, _ := c.headProposerID()
	return c.recomputeStateRootEntriesRevocations(prevStateRoot, committedStateRoot, b, w, parentProposer, chainID)
}

// assembleOpsViaHead is recomputeViaHead for the op-assembly half (the leaf-diff guard's driver).
func assembleOpsViaHead(c *Chain, prevStateRoot, committedStateRoot ports.Hash, b Block, w StateRootWitness) ([]statehash.FoldOp, error) {
	parentProposer, _ := c.headProposerID()
	return c.assembleStateRootRecomputeOps(prevStateRoot, committedStateRoot, b, w, parentProposer, c.ChainID())
}

// headProposerOrZero is the box-owned parent-proposer stand-in for a direct class-A driver.
func headProposerOrZero(c *Chain) ports.NodeID {
	id, _ := c.headProposerID()
	return id
}

// =============================================================================
// THE BOX DOOR — gates G-3 (RT2-CARRIER-13 through the door), G-7 (the witness leg of BG-3),
// and the door's own honest twin
// =============================================================================
//
// Governing: PE build brief §7 steps 6 and 8, P-table delta certification G-D10 + §6 (BG-2 split,
// BG-3 with M-4). The box under test is NewBox over the struct fixture's h1 block as parent, with a
// prover-backed WitnessSource over the fixture chain's own committed leaves — everything served is
// genuine, so the gates exercise the composition and the view accessors, not a mock.

// proverSource is a WitnessSource over a chain's committed v5 leaf set: point leaves proven by a
// real statehash prover, whole sets as the chain's own id-lists, the ancestor window from the
// chain's head. Honest by construction; a gate that needs a lie forges the BLOCK.
type proverSource struct {
	prover  *statehash.Prover
	leaves  map[string][]byte
	members map[string][]ports.NodeID
	chain   []ports.Hash
}

func newProverSource(t *testing.T, c *Chain) *proverSource {
	t.Helper()
	leafSet := c.stateRootLeavesV5()
	pr, err := statehash.NewProver(leafSet)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	src := &proverSource{prover: pr, leaves: make(map[string][]byte, len(leafSet)), members: map[string][]ports.NodeID{}}
	for _, l := range leafSet {
		src.leaves[string(l.Key)] = l.Value
	}
	ids := func(m map[ports.NodeID]int64) []ports.NodeID {
		out := make([]ports.NodeID, 0, len(m))
		for id := range m {
			out = append(out, id)
		}
		return sortIDs(out)
	}
	flags := func(m map[ports.NodeID]bool) []ports.NodeID {
		out := make([]ports.NodeID, 0, len(m))
		for id, v := range m {
			if v {
				out = append(out, id)
			}
		}
		return sortIDs(out)
	}
	src.members[tagEpochSetRoot] = ids(c.epochSet)
	src.members[tagQualifiedRoot] = ids(c.qualified)
	src.members[tagBondedRoot] = ids(c.bonded)
	src.members[tagValidatorsSeenRoot] = flags(c.validatorsSeen)
	src.members[tagSlashedRoot] = flags(c.slashed)
	cur, _ := c.Head()
	for i := 0; i < 64; i++ {
		src.chain = append(src.chain, cur)
		blk, ok := c.blockByHash(cur)
		if !ok || blk.Height == 0 {
			break
		}
		cur = blk.Prev
	}
	return src
}

func (s *proverSource) Leaf(key []byte) ([]byte, statehash.Witness, bool) {
	w, err := s.prover.Prove(key)
	if err != nil {
		return nil, statehash.Witness{}, false
	}
	return s.leaves[string(key)], w, true
}

func (s *proverSource) Members(digestTag string) ([]ports.NodeID, bool) {
	ids, ok := s.members[digestTag]
	if !ok {
		return nil, true // an empty keyspace has an empty complete list
	}
	return ids, true
}

func (s *proverSource) Ancestors(k int) ([]ports.Hash, bool) {
	if len(s.chain) == 0 {
		return nil, false
	}
	if k > len(s.chain) {
		k = len(s.chain)
	}
	return s.chain[:k], true
}

// structWitnessFor is the honest, prover-built StateRootWitness for an ENTRIES-ONLY v5 block on the
// struct fixture (no TTL, no carrier, mature-from-genesis): the E changed-leaf proofs and the
// class-M latch scalars.
func structWitnessFor(t *testing.T, f structFixture, src *proverSource, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness
	preValue := func(k []byte) []byte { return src.leaves[string(k)] }
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		wit, err := src.prover.Prove(wr.key)
		if err != nil {
			t.Fatalf("Prove: %v", err)
		}
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{Key: wr.key, OldValue: preValue(wr.key), Proof: wit})
	}
	w.Maturity = latchedMaturityWitness(t, src.prover, preValue)
	return w
}

// boxOver builds the box the door gates run: the struct fixture's committed h1 block as parent, a
// generous byte budget, the prover-backed source.
func boxOver(t *testing.T, f structFixture, src WitnessSource) *Box {
	t.Helper()
	parent := f.c.Blocks(1)[0]
	box, err := NewBox(f.c, parent, BoxConfig{BudgetBytes: 1 << 22, ChainID: f.c.ChainID()}, src)
	if err != nil {
		t.Fatalf("NewBox: %v", err)
	}
	if got, want := box.Head(), (liveView{f.c}).Head(); got.Hash != want.Hash || got.NextHeight != want.NextHeight ||
		got.ProposerID != want.ProposerID || *got.StateRoot != *want.StateRoot || *got.LogRoot != *want.LogRoot || got.Empty {
		t.Fatalf("the box's head record derived from the parent block must equal the node's own: box %+v node %+v", got, want)
	}
	return box
}

// assertBoxReachesTheDowngrade is the door's HONEST TWIN (NG-2): the honest block runs the whole
// composition through the box, over genuine witnesses, up to the R1.8 downgrade. It proves the door
// can reach the far end — a door that stalled on everything would pass every refusal gate below
// for the wrong reason.
func assertBoxReachesTheDowngrade(t *testing.T, box *Box, b Block, w StateRootWitness) {
	t.Helper()
	out, err := box.Validate(b, w)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrRecomputeGated) {
		t.Fatalf("NON-VACUITY BROKEN: the honest block must run the composition through the door to the R1.8 "+
			"downgrade (IndeterminateTrustlessly / ErrRecomputeGated); got %s / %v", out, err)
	}
}

// TestBoxDoor_HonestBlockReachesTheDowngrade drives the whole v5 path through the box: P1 over the
// derived head, every class-2 read Resolved through the prover source, P13a through the recompute
// with the box-owned parent proposer, the two quorum stacks, then the one-line downgrade.
func TestBoxDoor_HonestBlockReachesTheDowngrade(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	box := boxOver(t, f, src)
	b := f.mkBlock(t, nil)
	if err := f.c.ValidateCommit(&b); err != nil {
		t.Fatalf("oracle: %v", err)
	}
	assertBoxReachesTheDowngrade(t, box, b, structWitnessFor(t, f, src, b))
	// The downgrade is the ONLY thing between the door and Accept: with no source the same block
	// stalls at its first committed read, by name — never-Accept is earned twice.
	blind := boxOver(t, f, nil)
	if out, err := blind.Validate(b, structWitnessFor(t, f, src, b)); out != IndeterminateTrustlessly || !errors.Is(err, ErrViewNoWitness) {
		t.Fatalf("a sourceless box must stall at its first class-2 read; got %s / %v", out, err)
	}
}

// TestNewBox_RefusesWhatItCannotOwn: the four construction refusals, each by name.
func TestNewBox_RefusesWhatItCannotOwn(t *testing.T) {
	f := buildStructFixture(t)
	parent := f.c.Blocks(1)[0]
	src := newProverSource(t, f.c)
	honest := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), honest, structWitnessFor(t, f, src, honest))
	// G-7 / G-D10 at the box: an unset budget is refused at construction; ∞ is not expressible.
	if _, err := NewBox(f.c, parent, BoxConfig{}, nil); !errors.Is(err, ErrBudgetNotPositive) {
		t.Fatalf("G-7: NewBox with an unset BudgetBytes must REFUSE (ErrBudgetNotPositive); got %v", err)
	}
	if _, err := NewBox(f.c, parent, BoxConfig{BudgetBytes: -1}, nil); !errors.Is(err, ErrBudgetNotPositive) {
		t.Fatalf("G-7: NewBox with a negative BudgetBytes must REFUSE; got %v", err)
	}
	// The S2 mode fence at the door: a legacy chain is not a box.
	lf := buildLegacyFixture(t)
	if _, err := NewBox(lf.c, lf.c.Blocks(0)[0], BoxConfig{BudgetBytes: 1 << 20}, nil); !errors.Is(err, ErrBoxLegacyMode) {
		t.Fatalf("NewBox over a legacy chain must REFUSE (ErrBoxLegacyMode); got %v", err)
	}
	// A parent whose heavy proofs are shed cannot anchor the box. The parent must actually HAVE a
	// proof to shed: (d-3) retires `Pruned` for v5, so Prune() on an entry-only v5 block is a
	// legitimate no-op and this arm would assert against an unpruned parent.
	shedParent := parent
	d := answerDigestOf([]byte("valid"))
	shedParent.BondRegs = []BondReg{{Validator: pubOf(f.keys[0]), Root: ports.Hash{0x11}, Size: twoMiB, AnswerDigest: &d}}
	if !shedParent.HeavyProofsShed() {
		t.Fatal("fixture: the parent under test must actually have shed a proof, or this arm is vacuous")
	}
	if _, err := NewBox(f.c, shedParent, BoxConfig{BudgetBytes: 1 << 20}, nil); !errors.Is(err, ErrBoxParentPruned) {
		t.Fatalf("NewBox over a pruned parent must REFUSE (ErrBoxParentPruned); got %v", err)
	}
	// A parent that does not verify over its own hash anchors nothing.
	bad := parent
	bad.hashMemoSet = false
	bad.Entries = []ports.Entry{entry(200)}
	if _, err := NewBox(f.c, bad, BoxConfig{BudgetBytes: 1 << 20}, nil); !errors.Is(err, ErrBoxParentUnsigned) {
		t.Fatalf("NewBox over a mutated parent must REFUSE (ErrBoxParentUnsigned); got %v", err)
	}
}

// TestG3_ParentBindingPrecedesTheCarrierLeg is the RT2-CARRIER-13 gate through the door (step 6).
//
// The box's old recompute entry took (b, parentStateRoot, witness) and had no position of its
// own, so the carrier was verified over whatever b.Prev the author chose (RT2-CARRIER-13/13b/13c),
// and the parent proposer came from the witness. Now the door runs P1 over the box's OWN head
// before any carrier crypto. Two arms, both refused ON THE PARENT BINDING, BY NAME:
//   - a stale-but-valid replay: the committed h1 block, genuine signatures, genuine roots — the
//     block the header sweep cannot catch (its signature verifies; only the reader's position
//     refuses it);
//   - a forged carrier on a FOREIGN parent: zero-signature entries over a parent the box does not
//     hold. The discriminator is the refusal's NAME: ErrWrongParent (P1) and not ErrCarrier*
//     (P12). Ablation (G-3): derive the view's head from the block's own b.Prev/b.Height (the old
//     driver-asserted parent) ⇒ P1 passes ⇒ the verdict names the carrier ⇒ RED.
func TestG3_ParentBindingPrecedesTheCarrierLeg(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	box := boxOver(t, f, src)
	honest := f.mkBlock(t, nil)
	assertBoxReachesTheDowngrade(t, box, honest, structWitnessFor(t, f, src, honest))

	// Arm 1: the stale valid replay.
	stale := f.c.Blocks(1)[0]
	sh := stale.Hash()
	if !ed25519.Verify(stale.Proposer, sh[:], stale.ProposerSig) {
		t.Fatal("fixture VACUOUS: the replayed block must carry a GENUINE proposer signature")
	}
	if err := f.c.ValidateCommit(&stale); !errors.Is(err, ErrWrongParent) {
		t.Fatalf("ORACLE: the node refuses a stale-but-valid block on the PARENT BINDING; got %v", err)
	}
	out, err := box.Validate(stale, StateRootWitness{})
	if out != Reject || !errors.Is(err, ErrWrongParent) {
		t.Fatalf("RT2-CARRIER-13: the box must refuse a stale-but-valid replay on the PARENT BINDING, by name; got %s / %v", out, err)
	}

	// Arm 2: a forged carrier over a FOREIGN parent. The foreign parent is a self-consistent block
	// the box does not hold; the carrier entries are zero-signature claims over its hash.
	foreign := Block{Version: BlockVersionWitnessable, Height: 1, Prev: f.c.Blocks(0)[0].Hash(), Entries: []ports.Entry{entry(150)}}
	Sign(&foreign, f.keys[0])
	fh := foreign.Hash()
	forged := f.mkBlock(t, nil)
	forged.Prev = fh
	forged.LastCommit = []Attestation{{PubKey: pubOf(f.keys[1]), Sig: make([]byte, ed25519.SignatureSize), Round: 0, Phase: PhasePrecommit}}
	forged.hashMemoSet = false
	Sign(&forged, f.keys[0])
	if err := validateCarrier(&forged, ports.Hash{}); err == nil {
		t.Fatal("fixture VACUOUS: the forged carrier must be one validateCarrier refuses")
	}
	if err := f.c.ValidateCommit(&forged); !errors.Is(err, ErrWrongParent) {
		t.Fatalf("ORACLE: the node refuses the foreign-parent block on the PARENT BINDING; got %v", err)
	}
	out, err = box.Validate(forged, StateRootWitness{})
	if out != Reject || !errors.Is(err, ErrWrongParent) {
		t.Fatalf("RT2-CARRIER-13: the box must refuse a forged carrier over a FOREIGN parent on the PARENT BINDING (P1), "+
			"BEFORE the carrier leg (P12) is reached; got %s / %v", out, err)
	}
	if errors.Is(err, ErrCarrierBadSignature) {
		t.Fatalf("RT2-CARRIER-13: the refusal named the CARRIER — the carrier leg ran before the parent binding: %v", err)
	}
}

// TestG7_BoxBudgetCoversTheWitness is the witness leg of BG-3 (step 8; G-D10 covers the frame
// leg). The door charges FRAME + WITNESS bytes against the box's one config-derived ceiling, before
// any crypto: the discriminator is a GARBAGE proposer signature that would otherwise fail P3.
// Ablation (G-7): charge the frame alone at the door ⇒ the over-budget witness proceeds to P3 ⇒ RED.
func TestG7_BoxBudgetCoversTheWitness(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	parent := f.c.Blocks(1)[0]
	b := f.mkBlock(t, nil)
	w := structWitnessFor(t, f, src, b)
	wb := witnessBytes(w)
	if wb == 0 {
		t.Fatal("fixture VACUOUS: the honest witness measures 0 bytes")
	}
	frame := len(Encode(&b))
	assertBoxReachesTheDowngrade(t, boxOver(t, f, src), b, w)
	garbage := b
	garbage.hashMemoSet = false
	garbage.ProposerSig = make([]byte, ed25519.SignatureSize)

	// A ceiling that admits the frame but NOT frame + witness: the door stalls on the budget, by
	// name, before the garbage signature is looked at.
	tight, err := NewBox(f.c, parent, BoxConfig{BudgetBytes: frame + wb/2, ChainID: f.c.ChainID()}, src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tight.Validate(garbage, w)
	if out != IndeterminateTrustlessly || !errors.Is(err, ErrWitnessBudgetExceeded) {
		t.Fatalf("G-7 VIOLATED: frame + witness above the box's ceiling must STALL on the budget before any crypto; got %s / %v", out, err)
	}
	// Within the ceiling the same block fails on its garbage signature — the budget is what fired.
	loose, err := NewBox(f.c, parent, BoxConfig{BudgetBytes: frame + wb + 1, ChainID: f.c.ChainID()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := loose.Validate(garbage, w); out != Reject || !errors.Is(err, ErrBadSignature) {
		t.Fatalf("G-7 control: within budget the garbage signature must fail P3; got %s / %v", out, err)
	}
	// witnessBytes measures the quantity BG-3 bounds: adding one screen with real proofs grows it.
	grown := w
	k1 := idOf(f.keys[1])
	grown.AttScreens = append(grown.AttScreens, StateRootAttScreen{Attester: k1,
		SlashedProof: mustProve(src.prover, statehash.Key(tagSlashed, k1[:]))})
	if witnessBytes(grown) <= wb {
		t.Fatalf("witnessBytes did not grow with an added screen (%d -> %d)", wb, witnessBytes(grown))
	}
}
