package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// RT-CARRIER-1 / RT-CARRIER-12 — the box-vs-node split gates
// =============================================================================
//
// Red-team: RED-TEAM-lastcommit-carrier-3bd13e2-2026-09-03.md (probes RTCarrier1, 1b, 1c, 1d, 12).
// PE ruling: RULING-floorbox-predicate-rederivation-structure-2026-09-03.md §4 (the oracle rules),
// §6(a) (the fix shape), §7 (the merge conditions).
//
// THE DEFECT, at 3bd13e2. The trustless floor box reproduced applyCarrier's TRANSITION — class A
// derives its write-set straight off b.LastCommit[i].AttesterID() — but never validateCarrier's
// VALIDITY rule, which was wired only onto the full node's two disk-write paths. So an attacker
// minted a v5 block whose carrier named the PUBLIC keys of real qualified validators with zero-byte
// signatures, computed StateRoot with the real apply() (applyCarrier does not verify either, by
// design, because validateCarrier already did), and published. Every full node REJECTED. The box
// AGREED with the attacker's root. Cost: no key material; bound: the transport frame.
// RT-CARRIER-12 is the escalation — the same forged carrier flips the one-way everMature latch,
// i.e. it forges the MEASURED decentralisation quantity the maturity shed gates on.
//
// THE FIX these gates pin: assembleStateRootRecomputeOps calls the SAME validateCarrier the node
// calls, before any class dispatches. One function, three callers.
//
// THE ORACLE RULES THESE GATES OBEY (PE ruling §4):
//
//	O-1  The oracle is the full node's ACCEPT PATH — (*Chain).ValidateCommit over a real driven
//	     block on a real chain — never the single predicate the box is being compared against.
//	     None of these gates calls validateCarrier. A gate whose `want` came from validateCarrier
//	     would be a unit test of that function, and it would have stayed green through exactly the
//	     defect it is named for.
//	O-2  The asserted claim is the IMPLICATION `box agrees ⇒ node accepts`, NEVER the biconditional.
//	     The box is permitted to stall where the node accepts. assertBoxImpliesNode encodes only
//	     that direction. (The equality framing is what talked the previous round out of a
//	     safe-direction screen; see the ruling §4 O-2.)
//	O-3  Each mutant's node verdict is RE-COMPUTED, never carried forward from the honest base and
//	     never inferred from the mutation's intent.
//	O-5  The witness bundle is honest and PROVER-BUILT from the node's real committed state — never
//	     from the box's own recompute. The forgery is wholly inside hash-covered BLOCK content, so a
//	     sweep that only forges witnesses would be blind to it.
//
// NON-VACUITY. "The box stalls on everything" satisfies the implication trivially, so every gate
// runs an HONEST twin of its own fixture through the same machinery and requires the box to agree
// there. That twin is a fixture-health check, not a contract: it proves the gate can reach the
// box's agree outcome. It is NOT the converse of O-2.
//
// TIERS. Every gate runs under eachTier — the warm box (the chain that applied the history) and the
// COLD box (R-COLD-BOX-HARNESS: New(cfg)+SetBondVerifier, never applied a block). A gate that
// passed only warm would be passing on live state.

// rtGateWorld is a ValidateCommit-DRIVEN era-4 world: four anchors that actually commit two-phase
// v5 blocks, plus bonded non-anchor "victim" operators who are qualified but have never attested,
// so seating them is observable. The victims are what a forged carrier steals.
type rtGateWorld struct {
	c        *Chain
	cfg      Config
	keys     []ed25519.PrivateKey // the anchors; keys[0] proposes every height
	victims  []ed25519.PrivateKey // bonded, qualified, never seen
	prevRoot ports.Hash
	prover   *statehash.Prover
}

// rtGateWorldWith builds the world and advances it to height 2 with HONEST carrier blocks, so the
// attack block sits at height >= 2 (the height-1 carrier rule is out of the way for gates 1/1b/1c)
// and every block on the chain got there through ValidateCommit.
func rtGateWorldWith(t *testing.T, matureValidators, nVictims int, seed int64) *rtGateWorld {
	t.Helper()
	keys := []ed25519.PrivateKey{key(seed), key(seed + 1), key(seed + 2), key(seed + 3)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 40,
		MatureValidators: matureValidators, Era3ActivationHeight: 1, Era4ActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Height 1: the victims bond. Distinct domains so each counts separately in MatureCoefficient
	// (a same-domain set collapses to one and RT-CARRIER-12 would be unable to cross the bar).
	var victims []ed25519.PrivateKey
	var regs []BondReg
	prev, _ := c.Head()
	for i := 0; i < nVictims; i++ {
		vk := key(seed + 100 + int64(i))
		victims = append(victims, vk)
		regs = append(regs, bondRegFull(vk, ports.HashBytes(pubOf(vk)), twoMiB, prev, 5, uint64(1000+i)))
	}
	w := &rtGateWorld{c: c, cfg: cfg, keys: keys, victims: victims}
	// Heights 1 and 2 commit with an EMPTY carrier. Under-carrying is valid and downward-only
	// (O1: the producer rule is honest-maximal but unenforceable), and it keeps the victims UNSEEN
	// so the forged-carrier arm has something to steal. EVERY key signs the certificate — including
	// the victims — because once the chain is latched, validatorSetSize is the live qualified count,
	// so bonding victims RAISES RequiredQuorum above what four anchors alone can meet.
	mustAppend(t, c, w.mintEmptyCarrier(t, regs...)) // height 1: the victims bond
	mustAppend(t, c, w.mintEmptyCarrier(t))          // height 2

	w.reanchor(t)
	for _, vk := range victims {
		id := idOf(vk)
		if !c.attesterQualified(id) {
			t.Fatalf("fixture: victim %x is not qualified — the seating attack would write nothing", id[:6])
		}
		if c.validatorsSeen[id] {
			t.Fatalf("fixture: victim %x is already seen — the gate would be vacuous", id[:6])
		}
	}
	return w
}

// signers is every key that signs a certificate in this world: the anchors (which satisfy the
// launch-phase anchor gate) plus the bonded victims (which keep the post-handoff Byzantine quorum
// reachable). Signing a certificate seats nobody on a v5 block — only the carrier does — so this
// does not disturb what the gates measure.
func (w *rtGateWorld) signers() []ed25519.PrivateKey {
	return append(append([]ed25519.PrivateKey{}, w.keys...), w.victims...)
}

// mintEmptyCarrier builds the next block with an EXPLICITLY EMPTY carrier.
func (w *rtGateWorld) mintEmptyCarrier(t *testing.T, regs ...BondReg) *Block {
	t.Helper()
	prev, h := w.c.Head()
	b := &Block{Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}, BondRegs: regs}
	if err := w.c.PopulateEra4Roots(b); err != nil {
		t.Fatalf("PopulateEra4Roots at height %d: %v", h, err)
	}
	twoPhaseSign(b, w.signers())
	return b
}

func (w *rtGateWorld) reanchor(t *testing.T) {
	t.Helper()
	p, err := statehash.NewProver(w.c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	r, err := w.c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if r != p.Root() {
		t.Fatalf("fixture pre-root mismatch: chain %x prover %x", r[:8], p.Root())
	}
	w.prover, w.prevRoot = p, r
}

// blockWithCarrier mints the next block carrying exactly `carrier`, populates the era-4 roots with
// the REAL apply() (so the committed root is the one the carrier's own transition produces — the
// attacker computes its root honestly; only the carrier is forged), and signs a full two-phase
// anchor certificate so nothing BUT the carrier rule can be what the node objects to.
func (w *rtGateWorld) blockWithCarrier(t *testing.T, carrier []Attestation) Block {
	t.Helper()
	prev, h := w.c.Head()
	b := &Block{Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}, LastCommit: carrier}
	if err := w.c.PopulateEra4Roots(b); err != nil {
		t.Fatalf("PopulateEra4Roots at height %d: %v", h, err)
	}
	twoPhaseSign(b, w.signers())
	return *b
}

// rtForgedEntry is a carrier entry CLAIMING a precommit by victimPub over the parent, carrying a
// zero signature. It costs the attacker nothing: the public key is public.
func rtForgedEntry(victimPub []byte, round uint64) Attestation {
	return Attestation{PubKey: append([]byte(nil), victimPub...), Sig: make([]byte, ed25519.SignatureSize),
		Round: round, Phase: PhasePrecommit}
}

// genuineCarry is the honest twin of rtForgedEntry: a REAL PhasePrecommit over the parent block.
func (w *rtGateWorld) genuineCarry(t *testing.T, k ed25519.PrivateKey) Attestation {
	t.Helper()
	head, ok := w.c.headBlock()
	if !ok {
		t.Fatal("no head block")
	}
	return AttestAt(&head, k, 0, PhasePrecommit)
}

func (w *rtGateWorld) preValue(k []byte) []byte {
	for _, lf := range w.c.stateRootLeavesV5() {
		if string(lf.Key) == string(k) {
			return lf.Value
		}
	}
	return nil
}

func (w *rtGateWorld) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
	t.Helper()
	old := w.preValue(wr.key)
	if wr.newValue == nil {
		wit, sibs, err := w.prover.ProveWithSiblings(wr.key)
		if err != nil {
			t.Fatalf("ProveWithSiblings: %v", err)
		}
		return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit, DeleteSiblings: sibs}
	}
	wit, err := w.prover.Prove(wr.key)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit}
}

func (w *rtGateWorld) preSeenIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(w.c.validatorsSeen))
	for id := range w.c.validatorsSeen {
		out = append(out, id)
	}
	return sortIDs(out)
}

func (w *rtGateWorld) screen(id ports.NodeID) StateRootAttScreen {
	sz, bp := w.c.bonded[id]
	esVal, inES := w.c.epochSet[id]
	sc := StateRootAttScreen{Attester: id, Slashed: w.c.slashed[id], InEpochSet: inES,
		BondedSize: sz, BondedPresent: bp}
	sc.SlashedProof = mustProve(w.prover, statehash.Key(tagSlashed, id[:]))
	sc.EpochSetProof = mustProve(w.prover, statehash.Key(tagEpochSet, id[:]))
	if inES {
		sc.EpochSetValue = statehash.EncodeInt64(esVal)
	}
	sc.BondedProof = mustProve(w.prover, statehash.Key(tagBonded, id[:]))
	return sc
}

// witnessFor builds the HONEST, prover-built bundle a witness server would serve for b (O-5). It
// derives the class-A write-set the same way the box does, so the bundle is complete for the block
// as written — forged carrier included. That is the point: the attack needs no witness forgery.
func (w *rtGateWorld) witnessFor(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var wit StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		wit.ChangedLeaves = append(wit.ChangedLeaves, w.leafWitness(t, wr))
	}
	preSeen := idSet(w.preSeenIDs())
	screens := map[ports.NodeID]StateRootAttScreen{}
	wit.ParentProposer, wit.ParentProposerSig = w.c.CarrierParentProposerWitness()
	parentProposer, _ := w.c.headProposerID()
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if id == parentProposer {
			continue
		}
		if _, done := screens[id]; done {
			continue
		}
		screens[id] = w.screen(id)
		wit.AttScreens = append(wit.AttScreens, w.screen(id))
	}
	aWrites, _, err := w.c.stateRootAttWriteSet(w.prevRoot, b, preSeen, screens,
		livePreForProbe(w.c), wit.ParentProposer, wit.ParentProposerSig)
	if err != nil {
		// The box's own derivation refuses this block outright (e.g. a forged parent-proposer
		// anchor). No class-A write-set exists, so there is nothing to witness; the box will stall.
		aWrites = nil
	}
	for _, wr := range aWrites {
		wit.ChangedLeaves = append(wit.ChangedLeaves, w.leafWitness(t, wr))
	}
	wit.DigestPreSets = []StateRootDigestWitness{w.digest(t, tagValidatorsSeenRoot, w.preSeenIDs())}
	if w.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, err := w.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		wit.DueBucketProof = dp
	}
	wit.Maturity = latchedMaturityWitness(t, w.prover, w.preValue)
	if !w.c.everMature {
		// Unlatched: the box must reconstruct matureNow over the POST-apply committed state to decide
		// the crossing, so the honest prover serves the post-state SeenSet (RT-CARRIER-12's arm).
		wit.Maturity.SeenSet = w.seenWitnessPost(t, w.applied(b))
	}
	return wit
}

func (w *rtGateWorld) digest(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	wit, err := w.prover.Prove(statehash.Key(tag, nil))
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

func (w *rtGateWorld) applied(b Block) *Chain {
	clone := w.c.cloneForDryRun()
	clone.apply(b)
	return clone
}

// seenWitnessPost is the honest post-apply SeenSet witness RecomputeMatureNow verifies against the
// block's committed root.
func (w *rtGateWorld) seenWitnessPost(t *testing.T, applied *Chain) SeenSetWitness {
	t.Helper()
	post, err := statehash.NewProver(applied.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver(post): %v", err)
	}
	rootProof, err := post.Prove(statehash.Key(tagValidatorsSeenRoot, nil))
	if err != nil {
		t.Fatalf("Prove(validatorsSeenRoot post): %v", err)
	}
	ids := make([]ports.NodeID, 0, len(applied.validatorsSeen))
	members := make(map[ports.NodeID]MemberStateWitness, len(applied.validatorsSeen))
	for id := range applied.validatorsSeen {
		ids = append(ids, id)
		d, present := applied.bondDomain[id]
		members[id] = MemberStateWitness{
			Slashed: applied.slashed[id], SlashedProof: mustProve(post, statehash.Key(tagSlashed, id[:])),
			Bonded: applied.bonded[id], BondedProof: mustProve(post, statehash.Key(tagBonded, id[:])),
			Domain: d, DomainPresent: present, DomainProof: mustProve(post, statehash.Key(tagBondDomain, id[:])),
		}
	}
	return SeenSetWitness{IDs: ids, SeenRootWitness: rootProof,
		SeenRootValue: nodeSetMTHFromBool(applied.validatorsSeen), Members: members}
}

// eachTier runs fn against the WARM box (the chain that applied the history) and the COLD box
// (never applied a block). R-COLD-BOX-HARNESS: a gate that passes only warm was passing on live
// state, which is exactly the class of defect the fold-file pin exists for.
func (w *rtGateWorld) eachTier(t *testing.T, b Block, wit StateRootWitness, fn func(t *testing.T, tier string, boxErr error)) {
	t.Helper()
	committed := *b.StateRoot
	warm := w.c.RecomputeStateRootEntriesRevocations(w.prevRoot, committed, b, wit)
	t.Run("warm", func(t *testing.T) { fn(t, "warm", warm) })
	cold := coldRecompute(t, w.cfg, w.prevRoot, committed, b, wit)
	t.Run("cold", func(t *testing.T) { fn(t, "cold", cold) })
}

// assertBoxImpliesNode is THE assertion (O-1 + O-2): the box may stall where the node accepts, but
// it must never agree where the node rejects. The node verdict is recomputed here, per mutant (O-3).
func (w *rtGateWorld) assertBoxImpliesNode(t *testing.T, what string, b Block) {
	t.Helper()
	nodeErr := w.c.ValidateCommit(&b) // THE ORACLE: the full node's accept path, not a predicate
	wit := w.witnessFor(t, b)
	w.eachTier(t, b, wit, func(t *testing.T, tier string, boxErr error) {
		if boxErr == nil && nodeErr != nil {
			t.Fatalf("BOX/NODE SPLIT (%s, %s tier): the floor box AGREES with a committed root the full "+
				"node REJECTS.\n  node ValidateCommit -> %v\n  box  recompute      -> nil (agrees with root %x)\n"+
				"  The class-A write-set was derived from a carrier the node refuses. The box must run the "+
				"SHARED validateCarrier (assembleStateRootRecomputeOps) before any class dispatches — one "+
				"function, three callers, not a box-side counterpart.", what, tier, nodeErr, (*b.StateRoot)[:8])
		}
	})
}

// assertHonestTwinAgrees is the NON-VACUITY control, not a contract assertion: it proves the
// fixture can reach the box's agree outcome, so a box that stalled on everything could not pass
// these gates by default. It is NOT the converse of O-2.
func (w *rtGateWorld) assertHonestTwinAgrees(t *testing.T, b Block) {
	t.Helper()
	if err := w.c.ValidateCommit(&b); err != nil {
		t.Fatalf("NON-VACUITY BROKEN: the honest twin block is rejected by the full node (%v) — the "+
			"gate's forged arm would then be comparing against a fixture that cannot commit at all", err)
	}
	wit := w.witnessFor(t, b)
	w.eachTier(t, b, wit, func(t *testing.T, tier string, boxErr error) {
		if boxErr != nil {
			t.Fatalf("NON-VACUITY BROKEN (%s tier): the box stalls on the HONEST twin (%v). A box that "+
				"stalls on everything satisfies box-agrees=>node-accepts trivially; this gate would then "+
				"prove nothing.", tier, boxErr)
		}
	})
}

// -----------------------------------------------------------------------------
// GATE RT-CARRIER-1 — a forged-signature carrier
// -----------------------------------------------------------------------------

func TestRTGateCarrier1_BoxNeverAgreesWithACarrierTheNodeRejects(t *testing.T) {
	w := rtGateWorldWith(t, 0, 1, 61000)
	victim := idOf(w.victims[0])

	// NON-VACUITY: the same block shape with a GENUINE precommit commits, and the box agrees.
	honest := w.blockWithCarrier(t, []Attestation{w.genuineCarry(t, w.victims[0])})
	if !w.applied(honest).validatorsSeen[victim] {
		t.Fatalf("fixture: the honest carry does not seat the victim — the forged arm proves nothing")
	}
	w.assertHonestTwinAgrees(t, honest)

	// THE ATTACK: the same id, a zero signature. No key material.
	forged := w.blockWithCarrier(t, []Attestation{rtForgedEntry(pubOf(w.victims[0]), 0)})
	if !w.applied(forged).validatorsSeen[victim] {
		t.Fatalf("fixture: the forged carrier does not seat the victim — nothing to attack")
	}
	w.assertBoxImpliesNode(t, "zero-signature carrier entry", forged)
}

// GATE RT-CARRIER-1b — the bound is the frame, not one seat: one block steals every qualified id.
func TestRTGateCarrier1b_BoxNeverAgreesAtScale(t *testing.T) {
	w := rtGateWorldWith(t, 0, 4, 62000)
	var carrier []Attestation
	for _, vk := range w.victims {
		carrier = append(carrier, rtForgedEntry(pubOf(vk), 0))
	}
	forged := w.blockWithCarrier(t, carrier)
	applied := w.applied(forged)
	for _, vk := range w.victims {
		if !applied.validatorsSeen[idOf(vk)] {
			t.Fatalf("fixture: victim %x not seated at scale", idOf(vk))
		}
	}
	w.assertBoxImpliesNode(t, "4 zero-signature carrier entries in one block", forged)
}

// GATE RT-CARRIER-1c — the other validity clauses are equally unreproduced: wrong phase, a
// precommit over a FOREIGN block hash, duplicate ids. All three carry GENUINE signatures, so this
// arm cannot pass by a signature check alone.
func TestRTGateCarrier1c_BoxNeverAgreesOnPhaseForeignHashOrDuplicateID(t *testing.T) {
	t.Run("wrong-phase", func(t *testing.T) {
		w := rtGateWorldWith(t, 0, 1, 63000)
		head, _ := w.c.headBlock()
		b := w.blockWithCarrier(t, []Attestation{AttestAt(&head, w.victims[0], 0, PhasePrepare)})
		w.assertBoxImpliesNode(t, "genuine PhasePrepare signature in the carrier", b)
	})
	t.Run("foreign-hash", func(t *testing.T) {
		w := rtGateWorldWith(t, 0, 1, 64000)
		other := Block{Version: BlockVersionWitnessable, Height: 999, Entries: []ports.Entry{entry(99)}}
		b := w.blockWithCarrier(t, []Attestation{AttestAt(&other, w.victims[0], 0, PhasePrecommit)})
		w.assertBoxImpliesNode(t, "genuine precommit over a FOREIGN block hash", b)
	})
	t.Run("duplicate-ids", func(t *testing.T) {
		w := rtGateWorldWith(t, 0, 1, 65000)
		a := w.genuineCarry(t, w.victims[0])
		b := w.blockWithCarrier(t, []Attestation{a, a})
		w.assertBoxImpliesNode(t, "duplicate carrier ids", b)
	})
}

// GATE RT-CARRIER-1d — the height-1 rule. Height 1's carrier is EMPTY BY RULE (the genesis is
// DECLARED, not agreed, so its certificate must never pre-seat validatorsSeen). The entry carries a
// GENUINE signature; only the height makes it invalid.
func TestRTGateCarrier1d_BoxNeverAgreesOnAHeightOneCarrier(t *testing.T) {
	keys := []ed25519.PrivateKey{key(66000), key(66001), key(66002), key(66003)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 40,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	w := &rtGateWorld{c: c, cfg: cfg, keys: keys}
	w.reanchor(t)

	_, h := c.Head()
	if h != 1 {
		t.Fatalf("gate setup: next height must be 1, got %d", h)
	}
	b := w.blockWithCarrier(t, []Attestation{w.genuineCarry(t, keys[1])})
	if !w.applied(b).validatorsSeen[idOf(keys[1])] {
		t.Fatalf("fixture: the height-1 carrier does not seat anyone — nothing to attack")
	}
	w.assertBoxImpliesNode(t, "height-1 carrier (empty by rule)", b)
}

// -----------------------------------------------------------------------------
// GATE RT-CARRIER-12 — the escalation: the forged carrier flips the maturity latch
// -----------------------------------------------------------------------------
//
// validatorsSeen is the sole input to C2Metric -> MatureCoefficient -> matureNow -> the ONE-WAY
// everMature latch, and to the box's own RecomputeMatureNow, which class M consumes. So the
// RT-CARRIER-1 hole does not merely seat ids: it forges the MEASURED decentralisation quantity
// TENETS Part 0's maturity shed and C1's arrival count both read.
//
// The witness here is the HONEST crossing witness a real witness server would serve — pre-state
// handoff scalars against prevStateRoot, the post-apply SeenSet against the block's committed root.
// No witness forgery is involved (O-5). The forgery is wholly inside hash-covered block content.
func TestRTGateCarrier12_ForgedCarrierCannotFlipTheMaturityLatchPastTheBox(t *testing.T) {
	w := rtGateWorldWith(t, 3, 6, 67000)
	if w.c.everMature {
		t.Fatal("gate setup: the fixture must start UNLATCHED, or there is no crossing to steal")
	}
	if got := w.c.MatureCoefficient(); got != 0 {
		t.Fatalf("gate setup: MatureCoefficient must start at 0 (anchors are skipped by C2Metric), got %d", got)
	}

	var carrier []Attestation
	for _, vk := range w.victims {
		carrier = append(carrier, rtForgedEntry(pubOf(vk), 0))
	}
	forged := w.blockWithCarrier(t, carrier)

	applied := w.applied(forged)
	if !applied.everMature {
		t.Fatalf("gate setup: the forged carrier does not flip everMature (coefficient %d, want >= %d) — "+
			"the escalation this gate names is not reproduced by the fixture",
			applied.MatureCoefficient(), w.cfg.MatureValidators)
	}
	w.assertBoxImpliesNode(t, "forged carrier that flips the everMature latch", forged)
}
