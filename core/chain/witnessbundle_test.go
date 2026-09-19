package chain

import (
	"crypto/ed25519"
	"errors"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// THE WITNESS BUNDLE — the serving side of the state-root recompute.
//
// The point accessors answer one question the composition asked. The bundle is the other half of
// what a floor box needs: the O(payload) pre-state evidence the P13a recompute folds, which cannot
// be pulled key by key. These gates hold two properties of it.
//
// IT IS SUFFICIENT: the bundle a production provider builds over a real committed chain takes the
// honest block through the whole composition to the door's downgrade — the same far end the
// hand-built test bundle reaches. A provider that under-supplies stalls the box, so "sufficient"
// is the property worth gating; over-supply costs bytes and nothing else.
//
// IT IS NOT TRUSTED: the box derives the changed-key set itself and verifies every proof against
// prevStateRoot, so a bundle that omits, injects, or forges produces a stall. The arms below drive
// each of those three shapes and require a stall, never an acceptance.

// carrierBlock is the fixture's honest block WITH the head's attestation carrier attached — the
// shape every block above genesis has on a live chain, and the one that makes the class-A screens
// and the validatorsSeen digest part of the transition. A carrier-free block exercises neither, so
// a gate that only ever saw one would be silent about both.
func carrierBlock(t *testing.T, f structFixture) Block {
	t.Helper()
	b := f.mkBlock(t, func(b *Block) { b.LastCommit = f.c.HeadCarrier() })
	if len(b.LastCommit) == 0 {
		t.Fatal("FIXTURE: the head carries no attestation carrier, so the class-A screens are never exercised")
	}
	if err := f.c.ValidateCommit(&b); err != nil {
		t.Fatalf("ORACLE BROKEN: the node refuses its own carrier-bearing block: %v", err)
	}
	return b
}

// TestBundleTakesTheHonestBlockThroughTheDoor. Ablation: drop one entry from the ChangedLeaves
// loop in Bundle ⇒ the box stalls on a derived key with no witness ⇒ RED.
func TestBundleTakesTheHonestBlockThroughTheDoor(t *testing.T) {
	f := buildStructFixture(t)
	b := f.mkBlock(t, nil)
	if err := f.c.ValidateCommit(&b); err != nil {
		t.Fatalf("ORACLE BROKEN: the node refuses its own honest block: %v", err)
	}
	prov, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("the serving node must be able to build a provider over its own committed chain")
	}
	w, err := prov.Bundle(b)
	if err != nil {
		t.Fatalf("the provider could not build a bundle for an honest block on its own chain: %v", err)
	}
	assertBoxReachesTheDowngrade(t, boxOver(t, f, prov), b, w)
}

// TestBundleIsNotTrusted: the three ways a provider can lie about the bundle, each required to
// produce a STALL. The honest twin runs first in each arm, so a green refusal is never a box that
// refuses everything.
func TestBundleIsNotTrusted(t *testing.T) {
	f := buildStructFixture(t)
	prov, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("provider")
	}
	honest := f.mkBlock(t, nil)
	honestW, err := prov.Bundle(honest)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	assertBoxReachesTheDowngrade(t, boxOver(t, f, prov), honest, honestW)

	// The lies are aimed at a leaf the box DERIVES from the payload — the entry's byRoot leaf —
	// because the bundle deliberately over-supplies, and a lie planted on a key the box never asks
	// about is a lie nobody reads. Targeting the derived key is what makes each arm a test of the
	// box's verification rather than of the bundle's shape.
	derived := statehash.Key(tagByRoot, honest.Entries[0].Root[:])
	at := -1
	for i := range honestW.ChangedLeaves {
		if string(honestW.ChangedLeaves[i].Key) == string(derived) {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("FIXTURE: the bundle carries no witness for the block's own entry leaf, which the box " +
			"derives from the payload — the arms below would plant their lies where nobody reads them")
	}

	for _, arm := range []struct {
		name  string
		shape string
		spoil func(StateRootWitness) StateRootWitness
	}{
		{
			"omission", "a changed leaf the payload derives is left out",
			func(w StateRootWitness) StateRootWitness {
				dup := append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...)
				w.ChangedLeaves = append(dup[:at], dup[at+1:]...)
				return w
			},
		},
		{
			"a forged pre-state value", "a changed leaf claims a committed pre-value it does not have",
			func(w StateRootWitness) StateRootWitness {
				dup := append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...)
				dup[at].OldValue = []byte("not the committed pre-state")
				w.ChangedLeaves = dup
				return w
			},
		},
		{
			"a forged proof", "a changed leaf's pre-state proof does not verify against prevStateRoot",
			func(w StateRootWitness) StateRootWitness {
				dup := append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...)
				dup[at].Proof = statehash.Witness{}
				w.ChangedLeaves = dup
				return w
			},
		},
	} {
		t.Run(arm.name, func(t *testing.T) {
			out, sErr := boxOver(t, f, prov).Validate(honest, arm.spoil(honestW))
			if out == Accept {
				t.Fatalf("ACCEPTED a block whose bundle lied (%s)", arm.shape)
			}
			if out != IndeterminateTrustlessly || errors.Is(sErr, ErrRecomputeGated) {
				t.Fatalf("a bundle that lied (%s) must STALL short of the door's downgrade; got %s / %v",
					arm.shape, out, sErr)
			}
		})
	}
}

// TestBundleOverSupplyIsIgnored pins the property that lets a provider serve without predicting
// the box's scope decisions: the box derives the changed-key set from the payload, and a witness
// for a key or a whole-set digest the derivation did not reach is IGNORED. Without it the provider
// would have to reproduce the box's class dispatch to know what to send, and a provider that
// reproduced the box's judgement is the delegation the whole seam refuses.
//
// Ablation: make the recompute consume the witness's key set instead of its own derived one ⇒ the
// injected member below changes the verdict ⇒ RED.
func TestBundleOverSupplyIsIgnored(t *testing.T) {
	f := buildStructFixture(t)
	prov, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("provider")
	}
	b := f.mkBlock(t, nil)
	w, err := prov.Bundle(b)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	assertBoxReachesTheDowngrade(t, boxOver(t, f, prov), b, w)

	// This block touches no whole-set digest, so every digest pre-image in the bundle is
	// over-supply. Inject a member into all five: the verdict must not move.
	var stranger ports.NodeID
	stranger[0] = 0x77
	dup := append([]StateRootDigestWitness(nil), w.DigestPreSets...)
	if len(dup) == 0 {
		t.Fatal("the provider serves the whole-set pre-images unconditionally; none was served")
	}
	for i := range dup {
		dup[i].PreIDs = append(append([]ports.NodeID(nil), dup[i].PreIDs...), stranger)
	}
	w.DigestPreSets = dup
	assertBoxReachesTheDowngrade(t, boxOver(t, f, prov), b, w)
}

// TestBundleServesAYoungChainsMaturityWitness. On a chain that has not latched maturity the box
// must reproduce the metric that decides the latch, and that metric is anchored against the
// block's OWN post-apply root — which no snapshot of the parent state can prove. The provider
// applies the candidate to a private copy of its own chain and proves against the result, so a
// floor box works at LAUNCH and not only on a network that has already matured.
//
// Ablation: drop the young-chain copy from NewWitnessProvider ⇒ the bundle is refused by name ⇒
// the young arm goes red while the latched control stays green.
func TestBundleServesAYoungChainsMaturityWitness(t *testing.T) {
	// The latched control: no set is read, so no post-apply state is needed and none is kept.
	latched := buildStructFixture(t)
	lp, ok := NewWitnessProvider(latched.c)
	if !ok {
		t.Fatal("provider")
	}
	lw, err := lp.Bundle(latched.mkBlock(t, nil))
	if err != nil {
		t.Fatalf("a latched chain must yield a bundle: %v", err)
	}
	if lw.Maturity == nil || len(lw.Maturity.SeenSet.IDs) != 0 {
		t.Fatal("a latched chain's class-M witness must carry NO set: the latch cannot flip again, so " +
			"the box reads nothing to decide it and serving one would be pure cost")
	}

	// The young chain: the set travels, and it takes the block through the door.
	young := buildUnlatchedFixture(t)
	yp, ok := NewWitnessProvider(young.c)
	if !ok {
		t.Fatal("provider")
	}
	b := carrierBlock(t, young)
	yw, err := yp.Bundle(b)
	if err != nil {
		t.Fatalf("a young chain must yield a bundle — a floor box has to work at LAUNCH, which is "+
			"exactly when the latch has not tripped: %v", err)
	}
	if yw.Maturity == nil || len(yw.Maturity.SeenSet.IDs) == 0 || len(yw.Maturity.SeenSet.Members) == 0 {
		t.Fatal("a young chain's class-M witness must carry the validatorsSeen set and one proof per member")
	}
	assertBoxReachesTheDowngrade(t, boxOver(t, young, yp), b, yw)

	// And it is not trusted either: a member dropped from the set no longer reconstructs the
	// committed digest, so the box stalls rather than folding a short set.
	short := yw
	dup := *short.Maturity
	dup.SeenSet.IDs = dup.SeenSet.IDs[1:]
	short.Maturity = &dup
	out, sErr := boxOver(t, young, yp).Validate(b, short)
	if out == Accept || errors.Is(sErr, ErrRecomputeGated) {
		t.Fatalf("a short validatorsSeen set must STALL the box short of the door; got %s / %v", out, sErr)
	}
}

// buildUnlatchedFixture is buildStructFixture with the maturity latch left FALSE in committed
// state (MatureValidators above the anchor count, so matureNow is never true) — a YOUNG network,
// which is the state every network launches in. Everything else is identical, so the only thing
// the gate above can be reading is the latch.
func buildUnlatchedFixture(t *testing.T) structFixture {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(78000 + i))
		anchors[idOf(keys[i])] = true
	}
	c := New(Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99,
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
	f := structFixture{c: c, keys: keys}
	if err := c.Append(f.mkBlock(t, func(b *Block) { b.Entries = []ports.Entry{entry(1)} })); err != nil {
		t.Fatalf("fixture h1 must commit: %v", err)
	}
	return f
}

// TestBundleWireCarriesTheWholeBundle: the encoding is a transport, not a second definition of
// what a witness is. A bundle that crosses and comes back is field-for-field the one that went in,
// and it takes the same block to the same far end of the composition.
//
// WHAT THIS GATE COVERS: the classes a young chain's ordinary block carries — the changed leaves,
// the whole-set digest pre-images, the class-A screens and the class-M set. The bond-registration,
// TTL-sweep and epoch-rotation classes need a chain running a TTL and an epoch cadence, and they
// are carried end to end over the real transport by the node tier's live-network gate, where a
// field this encoding dropped would stall the box on the block that needed it.
//
// Ablation: drop one field from stateRootWitnessWire ⇒ the round trip diverges ⇒ RED.
func TestBundleWireCarriesTheWholeBundle(t *testing.T) {
	f := buildUnlatchedFixture(t)
	prov, ok := NewWitnessProvider(f.c)
	if !ok {
		t.Fatal("provider")
	}
	b := carrierBlock(t, f)
	w, err := prov.Bundle(b)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if len(w.AttScreens) == 0 || w.Maturity == nil || len(w.Maturity.SeenSet.IDs) == 0 {
		t.Fatal("GATE VACUOUS: the bundle carries no class-A screens or no class-M set, so the round " +
			"trip below says nothing about whether the wire carries them")
	}
	raw, err := EncodeStateRootWitness(w)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := DecodeStateRootWitness(raw, 16<<20, 1<<20)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Field-for-field, with the proofs compared by the bytes they marshal to — a Witness wraps a
	// library proof behind an unexported field, so the bytes are what "the same proof" means here.
	sent, got := proofShapes(t, w), proofShapes(t, back)
	if len(sent) != len(got) {
		t.Fatalf("the wire carried %d of the bundle's %d proofs — a class is being dropped", len(got), len(sent))
	}
	for i := range sent {
		if !reflect.DeepEqual(sent[i], got[i]) {
			t.Fatalf("the wire reshaped proof %d of %d: %d bytes sent, %d bytes back", i, len(sent), len(sent[i]), len(got[i]))
		}
	}
	if len(back.ChangedLeaves) != len(w.ChangedLeaves) || len(back.DigestPreSets) != len(w.DigestPreSets) ||
		len(back.AttScreens) != len(w.AttScreens) || back.Maturity == nil ||
		len(back.Maturity.SeenSet.IDs) != len(w.Maturity.SeenSet.IDs) ||
		len(back.Maturity.SeenSet.Members) != len(w.Maturity.SeenSet.Members) {
		t.Fatalf("the wire dropped a class: leaves %d/%d digests %d/%d screens %d/%d maturity=%v members %d/%d",
			len(back.ChangedLeaves), len(w.ChangedLeaves), len(back.DigestPreSets), len(w.DigestPreSets),
			len(back.AttScreens), len(w.AttScreens), back.Maturity != nil,
			len(back.Maturity.SeenSet.Members), len(w.Maturity.SeenSet.Members))
	}
	// The property that matters: the DECODED bundle judges the block exactly as the sent one does.
	assertBoxReachesTheDowngrade(t, boxOver(t, f, prov), b, back)

	// A bundle whose own fields disagree is refused rather than served short: a member set that
	// names an id it carries no witness for would stall the box for a reason it would attribute to
	// the block.
	inconsistent := w
	dup := *w.Maturity
	dup.SeenSet.Members = map[ports.NodeID]MemberStateWitness{}
	inconsistent.Maturity = &dup
	if _, err := EncodeStateRootWitness(inconsistent); !errors.Is(err, ErrBundleWireUncarriedClass) {
		t.Fatalf("the encoder must REFUSE a bundle that names a member it carries no witness for; got %v", err)
	}
	if _, err := DecodeStateRootWitness(raw, len(raw)-1, 1<<20); !errors.Is(err, ErrBundleWireTooLarge) {
		t.Fatalf("the decoder must refuse an over-ceiling bundle BEFORE it allocates; got %v", err)
	}
}

// proofShapes renders every proof in a bundle as its marshalled bytes, in order, so two bundles
// can be compared for proof identity without reaching inside statehash.Witness.
func proofShapes(t *testing.T, w StateRootWitness) [][]byte {
	t.Helper()
	var out [][]byte
	add := func(b []byte, err error) {
		if err != nil {
			t.Fatalf("marshal a bundle proof: %v", err)
		}
		out = append(out, b)
	}
	add(w.DueBucketProof.MarshalBinary())
	for _, cl := range w.ChangedLeaves {
		add(cl.Proof.MarshalBinary())
	}
	for _, d := range w.DigestPreSets {
		add(d.Proof.MarshalBinary())
	}
	for _, sc := range w.AttScreens {
		add(sc.SlashedProof.MarshalBinary())
		add(sc.EpochSetProof.MarshalBinary())
		add(sc.BondedProof.MarshalBinary())
	}
	if w.Maturity != nil {
		add(w.Maturity.EverMature.Proof.MarshalBinary())
		add(w.Maturity.MatureEpoch.Proof.MarshalBinary())
		add(w.Maturity.SeenSet.SeenRootWitness.MarshalBinary())
		for _, id := range w.Maturity.SeenSet.IDs {
			m := w.Maturity.SeenSet.Members[id]
			add(m.BondedProof.MarshalBinary())
			add(m.DomainProof.MarshalBinary())
			add(m.SlashedProof.MarshalBinary())
		}
	}
	return out
}
