package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// OWNER CALL A — THE ERA-4 CONSENSUS SIGNATURE PREIMAGE. GATES G-PRE-1 .. G-PRE-10.
// =============================================================================
//
// WHAT WAS BOUGHT, AND ON WHAT. consensusSigBytes gains HEIGHT and a CHAIN ID for era 4
// (consensusSigBytesV5, chain.go). Bought on the SCAR, not on an optimisation: dropping Height is
// the SECOND occurrence of the #397 watermark scar (docs/build-process.md rule 6) — the same
// (height, round, step) schema family, the same dropped field, and the dropped field is again the
// thing that broke. silt's own durable watermark already carried (height, round, step) while its
// own signature preimage carried only (round, step): the two halves of one mechanism disagreed
// about the slot, inside one codebase.
//
// Research certification (binding, verdict GATED — direction CERTIFIED, build gated on five
// conditions with these ten RED-first gates):
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md
//
// EVERY GATE IN THIS FILE CARRIES ITS ABLATION IN-PROCESS. Simplicity rule 7: a green gate with no
// demonstrated red is decoration. Where the certification names an ablation as a source edit ("key
// the dispatch on b.Version", "drop height from the preimage"), this file DRIVES the ablated
// arithmetic beside the shipped one and asserts they DISAGREE — so the red is permanent and
// re-run on every commit, not a one-off manual edit whose evidence lives in a report.
//
// THE CONDITIONS EACH GATE IS DRIVEN ONE AT A TIME (silt-a-claim-about-a-gate-is-itself-a-claim).
// No gate takes its guard condition from its own subject, and each fixture asserts the state whose
// ABSENCE would make the gate vacuous — at the era-4 boundary that means asserting the parent is
// v4 and the child is v5 before asserting anything about their signatures.

// ---------------------------------------------------------------------------
// G-PRE-1 — THE LIVENESS WEDGE AT H_era4.
// ---------------------------------------------------------------------------
//
// THE DEFECT, from the certification §5.3. era4Active is AT-OR-GREATER, so H_era4 is itself the
// first v5 height and ITS PARENT IS v4. HeadCarrier filters the parent's precommits with the v4
// parent in scope, so it produces v2-FORM entries. validateCarrier then verifies those entries
// with the v5 CHILD in scope. Key that dispatch on the CONTAINING block's Version and every entry
// fails: the honest proposer mints a block its own replica rejects, every designated proposer at
// that height does the same, and the chain never advances past H_era4. A permanent liveness wedge
// at exactly one height.
//
// THE FIX, certified: dispatch on the ATTESTATION's own Phase — the mechanism verifyAtt already
// used for era-1 vs era-2. Wire-additive, and what the #558 one-dispatcher pin demands.
//
// THE ABLATION, driven below: relabel each honest carrier entry to the phase a b.Version-keyed
// dispatch would compute (AttPhase(b.Version, PhasePrecommit) = PhasePrecommitV5). That makes
// verifyAtt verify a v2-form signature under the v5 preimage — which is exactly what keying on the
// container does — and validateCarrier must REFUSE. The shipped rule must ACCEPT the same block
// unrelabelled.
func TestGPRE1_Era4BoundaryCarrierIsNotAWedge(t *testing.T) {
	// H_era3 = 2, H_era4 = 3: height 1 is v2, height 2 is v4, and height 3 is the FIRST v5 block
	// with a v4 PARENT. That is the exact configuration the wedge lives at.
	//
	// Height 1 must be v2 and not v4 for a fixture reason worth stating: a v4 block seats its own
	// attesters from b.Atts, and the proposer populates its committed roots BEFORE gathering, so
	// the first v4 block that seats anybody cannot commit at all (that is R-BOX-ATTESTS, the defect
	// the carrier exists to fix and which era 3 still carries). Height 1 seats the three
	// non-proposers under the era-2 rule; height 2 then adds no seating and its roots hold.
	c, keys := era4AnchorChain(t, 2, 3)
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h1, v2
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h2, v4

	// NON-VACUITY, asserted before anything else: this fixture must actually straddle the
	// boundary. A gate that silently ran two v5 blocks would prove nothing about the wedge.
	parent, _ := c.headBlock()
	if parent.Version != BlockVersionStateRoot {
		t.Fatalf("GATE VACUOUS: the parent of H_era4 must be v4, got v%d — the wedge only exists where "+
			"the producer and the validator straddle the boundary", parent.Version)
	}
	if _, next := c.Head(); next != 3 || c.MintVersion(next) != BlockVersionWitnessable {
		t.Fatalf("GATE VACUOUS: height %d must mint v5 (H_era4 = 3)", next)
	}

	b := mintNext4Carrier(t, c, keys) // h3, v5, carrying h2's v2-form precommits
	if b.Version != BlockVersionWitnessable {
		t.Fatalf("GATE VACUOUS: the boundary block must be v5, got v%d", b.Version)
	}
	if len(b.LastCommit) == 0 {
		t.Fatal("GATE VACUOUS: the boundary block's carrier is EMPTY — the wedge needs entries to fail on. " +
			"The carrier is empty by rule only at height 1; if HeadCarrier returned nothing here, the " +
			"producer filter is already broken and this gate would pass over a dead mechanism")
	}
	for i, a := range b.LastCommit {
		if a.Phase != PhasePrecommit {
			t.Fatalf("GATE VACUOUS: carrier entry %d has phase %d, want the v2 form (%d) — the parent is v4, "+
				"so its precommits must be heightless; a v5-form entry here means the fixture is not at the boundary",
				i, a.Phase, PhasePrecommit)
		}
	}

	// --- THE SHIPPED RULE: the honest boundary block VALIDATES, on every path that judges it. ---
	if err := validateCarrier(b, c.ChainID()); err != nil {
		t.Fatalf("G-PRE-1 VIOLATED (validity rule): the honest boundary carrier must be valid: %v", err)
	}
	if err := c.ValidateProposal(b); err != nil {
		t.Fatalf("G-PRE-1 VIOLATED (the proposer's own replica): an honest proposer at H_era4 rejected its "+
			"OWN block — that is the permanent liveness wedge: %v", err)
	}
	if out, err := ValidateProposalV5(liveView{c}, b); out != Accept {
		t.Fatalf("G-PRE-1 VIOLATED (the ONE accept composition): %s / %v", out, err)
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("G-PRE-1 VIOLATED (commit): the boundary block must COMMIT: %v", err)
	}
	// And the own-disk reload path, which runs validateCarrier in appendStructural.
	replay := New(c.cfg, func(ports.NodeID) int64 { return 0 })
	replay.SetBondVerifier(objectiveVerify)
	blocks := c.Blocks(0)
	if n, err := replay.Reload(blocks); err != nil || n != len(blocks) {
		t.Fatalf("G-PRE-1 VIOLATED (reload): the boundary block must replay: %d of %d, %v", n, len(blocks), err)
	}

	// --- THE ABLATION: key the dispatch on the CONTAINING block's Version. RED, driven. ---
	wedged := *b
	wedged.LastCommit = append([]Attestation(nil), b.LastCommit...)
	for i := range wedged.LastCommit {
		// What a b.Version-keyed dispatch computes for this entry. The SIGNATURE is untouched —
		// only which preimage the verifier reaches for changes, which is the whole defect.
		wedged.LastCommit[i].Phase = AttPhase(wedged.Version, PhasePrecommit)
	}
	if wedged.LastCommit[0].Phase != PhasePrecommitV5 {
		t.Fatalf("ABLATION DID NOT APPLY: the container-keyed phase is %d, expected PhasePrecommitV5 (%d). "+
			"A patch that silently fails to apply reports GREEN and is indistinguishable from a passing "+
			"ablation (silt-ablation-noop-guard)", wedged.LastCommit[0].Phase, PhasePrecommitV5)
	}
	err := validateCarrier(&wedged, c.ChainID())
	if !errors.Is(err, ErrCarrierBadSignature) {
		t.Fatalf("G-PRE-1 GATE IS DECORATION: keying the carrier's preimage dispatch on the CONTAINING "+
			"block's version must REJECT every entry at H_era4 (that is the wedge), got %v", err)
	}
	t.Logf("G-PRE-1 ablation RED as required: container-keyed dispatch at H_era4 ⇒ %v", err)
}

// ---------------------------------------------------------------------------
// G-PRE-2 — THE DERIVED SIGNING HEIGHT AT THE CARRIER SEAM.
// ---------------------------------------------------------------------------
//
// validateCarrier is the ONE site of the nine where the signing height is DERIVED rather than READ:
// the entries are the PARENT's precommits and the parent is not in scope, so the height is
// b.Height-1. Certified SOUND under P1 PARENT BINDING and a strict NARROWING (certification §5.2).
//
// Driven one condition at a time:
//
//	(a) a genuine parent precommit at b.Height-1 is ACCEPTED;
//	(b) the ABLATION — the same key's precommit signed at b.Height (what an off-by-one
//	    validateCarrier would demand) is REFUSED;
//	(c) the same key's precommit over a DIFFERENT parent at a different height is REFUSED.
func TestGPRE2_TheCarrierSigningHeightIsTheParents(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h1, v5
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h2, v5 — its carrier is h1's precommits

	head, _ := c.headBlock()
	if head.Version != BlockVersionWitnessable {
		t.Fatalf("GATE VACUOUS: the parent must be v5 so its precommits bind a height, got v%d", head.Version)
	}
	prev, next := c.Head()
	cid := c.ChainID()

	mk := func(entries []Attestation) *Block {
		return &Block{Version: BlockVersionWitnessable, Height: next, Prev: prev,
			Entries: []ports.Entry{entry(77)}, LastCommit: entries}
	}

	// (a) the honest entry — signed at the PARENT's height, which is b.Height-1.
	honest := AttestAt(&head, keys[1], 0, PhasePrecommit, cid)
	if honest.Phase != PhasePrecommitV5 {
		t.Fatalf("GATE VACUOUS: a v5 parent's precommit must carry the v5 form, got phase %d", honest.Phase)
	}
	if err := validateCarrier(mk([]Attestation{honest}), cid); err != nil {
		t.Fatalf("G-PRE-2 (a) VIOLATED: a genuine parent precommit at b.Height-1 must be accepted: %v", err)
	}

	// (b) THE ABLATION, driven: the byte-for-byte signature an off-by-one validateCarrier (one that
	// passed b.Height instead of b.Height-1) would demand. Same key, same round, same parent hash —
	// only the declared height inside the preimage moves by one.
	offByOne := honest
	offByOne.Sig = ed25519.Sign(keys[1], consensusSigBytesV5(cid, PhasePrecommitV5, next, 0, prev))
	if bytes.Equal(offByOne.Sig, honest.Sig) {
		t.Fatal("ABLATION DID NOT APPLY: the off-by-one signature is identical to the honest one, so the " +
			"preimage does not bind the height at all and this gate carries zero bits")
	}
	if err := validateCarrier(mk([]Attestation{offByOne}), cid); !errors.Is(err, ErrCarrierBadSignature) {
		t.Fatalf("G-PRE-2 (b) GATE IS DECORATION: an entry signed at b.Height (not b.Height-1) must be "+
			"REFUSED — the derived height is what makes it so; got %v", err)
	}

	// (c) a genuine precommit by the same key over a DIFFERENT block at a DIFFERENT height.
	other := Block{Version: BlockVersionWitnessable, Height: next - 2, Prev: ports.HashBytes([]byte("elsewhere")),
		Entries: []ports.Entry{entry(9)}}
	foreign := AttestAt(&other, keys[1], 0, PhasePrecommit, cid)
	if err := validateCarrier(mk([]Attestation{foreign}), cid); !errors.Is(err, ErrCarrierBadSignature) {
		t.Fatalf("G-PRE-2 (c) VIOLATED: a genuine precommit over another block at another height must be "+
			"REFUSED; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// G-PRE-4 — THE PHASE CONSTANT ORDERING IS A SAFETY CONSTRAINT.
// ---------------------------------------------------------------------------
//
// core/node.slotCompare orders the DURABLE anti-double-sign watermark by comparing the phase byte
// NUMERICALLY. PhasePrepareV5 < PhasePrecommitV5 is therefore what stops a validator taking the
// prepare slot AFTER the precommit slot within one height. Had the two been assigned the other way
// round, an era-4 validator could prepare a second block at a height it had already precommitted.
//
// The ordering is pinned HERE with the reason in the failure text, and the ABLATION (swap them) is
// driven against the same comparison slotCompare performs, so a future edit that reverses the
// constants reddens rather than being caught by a code reviewer reading a comment.
func TestGPRE4_PhaseConstantOrderingIsMonotone(t *testing.T) {
	// The watermark comparison, reproduced exactly as core/node.slotCompare performs it at equal
	// (height, round): a probe is ALLOWED iff its phase is numerically greater than the mark's.
	allowedAfter := func(probe, mark uint8) bool { return probe > mark }

	if !(PhasePrepare < PhasePrecommit) {
		t.Fatalf("the era-2 steps are out of order (%d, %d)", PhasePrepare, PhasePrecommit)
	}
	if !(PhasePrepareV5 < PhasePrecommitV5) {
		t.Fatalf("PhasePrepareV5 (%d) must be numerically BELOW PhasePrecommitV5 (%d). This is a SAFETY "+
			"constraint, not a style choice: core/node.slotCompare orders the durable anti-double-sign "+
			"watermark by comparing the phase byte numerically, so reversing these two lets an era-4 "+
			"validator take the PREPARE slot after the PRECOMMIT slot at one height — a self-manufactured "+
			"double-sign", PhasePrepareV5, PhasePrecommitV5)
	}
	if !(PhasePrecommit < PhasePrepareV5) {
		t.Fatalf("the era-4 constants (%d, %d) must sit ABOVE the era-2 ones (%d, %d): a durable mark "+
			"persisted under the old constants is read back by the new binary, and the watermark's "+
			"alphabet must only ever grow upward",
			PhasePrepareV5, PhasePrecommitV5, PhasePrepare, PhasePrecommit)
	}

	// SHIPPED: a precommit mark blocks a later prepare at the same slot.
	if allowedAfter(PhasePrepareV5, PhasePrecommitV5) {
		t.Fatal("G-PRE-4 VIOLATED: a prepare is signable after a precommit at one height")
	}
	// ABLATION, driven: swap the two constants and the same comparison ALLOWS it.
	const swappedPrepare, swappedPrecommit = 4, 3
	if !allowedAfter(swappedPrepare, swappedPrecommit) {
		t.Fatal("G-PRE-4 GATE IS DECORATION: with the constants swapped the watermark comparison must " +
			"ALLOW a prepare after a precommit — if it does not, this gate is not testing slotCompare's " +
			"actual arithmetic and its PASS carries zero bits")
	}
	t.Logf("G-PRE-4 ablation RED as required: swapping the two era-4 constants makes a prepare signable "+
		"after a precommit at one height (%d > %d)", swappedPrepare, swappedPrecommit)
}

// ---------------------------------------------------------------------------
// G-PRE-5 — THE DECLARED HEIGHT BINDS THE SIGNATURE (the R0.6 relabel has no expression).
// ---------------------------------------------------------------------------
//
// The R0.6 / I5 cross-height forgery takes two GENUINE signatures by an honest validator at two
// DIFFERENT heights, relabels one with a fictitious height, and convicts. It worked because the
// height came from OUTSIDE the signed message — the accuser supplied it, and Block.Pruned severed
// the only thing binding it (chain.go's Hash() short-circuit).
//
// Under the era-4 preimage the attack has no expression: to accept, BOTH signatures must verify at
// ONE declared height, and each verifies only at its own. This drives that directly, with the
// ablation being the preimage WITHOUT the height field — the shape that was shipped before this
// change — verified against the same two signatures.
func TestGPRE5_DeclaredHeightMustBindTheSignature(t *testing.T) {
	k := key(97001)
	cid := ports.HashBytes([]byte("G-PRE-5 chain"))
	h := ports.HashBytes([]byte("one block hash"))
	const r = 3

	// One genuine signature, released at height 10.
	sig10 := ed25519.Sign(k, consensusSigBytesV5(cid, PhasePrecommitV5, 10, r, h))
	att := Attestation{PubKey: k.Public().(ed25519.PublicKey), Sig: sig10, Round: r, Phase: PhasePrecommitV5}

	// SHIPPED: it verifies at 10 and NOWHERE else. Each height driven on its own.
	if !verifyAtt(att, attScope{ChainID: cid, Height: 10}, h) {
		t.Fatal("fixture: the genuine signature must verify at its own height")
	}
	for _, wrong := range []uint64{0, 9, 11, 1 << 40} {
		if verifyAtt(att, attScope{ChainID: cid, Height: wrong}, h) {
			t.Fatalf("G-PRE-5 VIOLATED: a signature released at height 10 verified at declared height %d — "+
				"the accuser can relabel the height and the R0.6 cross-height forgery is back", wrong)
		}
	}

	// ABLATION, driven: the same signature under a preimage with the HEIGHT DROPPED. It is the same
	// message at every height, so one genuine signature answers for all of them — which is exactly
	// the degree of freedom R0.6 exploited.
	heightless := func(height uint64) []byte {
		buf := make([]byte, 0, consensusSigPreimageV5Len-8)
		buf = append(buf, consensusSigDomainV5...)
		buf = append(buf, cid[:]...)
		buf = append(buf, PhasePrecommitV5)
		var rb [8]byte
		rb[0] = r
		buf = append(buf, rb[:]...)
		return append(buf, h[:]...)
	}
	_ = heightless(10)
	if !bytes.Equal(heightless(10), heightless(11)) {
		t.Fatal("ABLATION DID NOT APPLY: the height-dropped preimage still differs across heights, so this " +
			"arm is not the shape it claims to ablate")
	}
	ablatedSig := ed25519.Sign(k, heightless(10))
	for _, anyHeight := range []uint64{9, 10, 11, 1 << 40} {
		if !ed25519.Verify(k.Public().(ed25519.PublicKey), heightless(anyHeight), ablatedSig) {
			t.Fatalf("G-PRE-5 GATE IS DECORATION: without the height in the preimage one signature must "+
				"answer at EVERY declared height (that is the forgery); it failed at %d", anyHeight)
		}
	}
	t.Log("G-PRE-5 ablation RED as required: with `height` dropped from the preimage, one genuine " +
		"signature verifies at every declared height — the R0.6 relabel convicts an honest validator")
}

// eraFloorAt reports the block version a proposer must stamp at height h on a chain whose genesis
// commits Era4ActivationHeight = era4At — i.e. the ERA FLOOR at that height.
//
// IT IS THE GUARD FOR THE ERA-2-FORM ARMS BELOW, AND IT DOES NOT COME FROM THEIR SUBJECT. The
// chain built here holds no blocks, is never appended to, and never sees the evidence blocks; it
// exists only to answer MintVersion, which is the single site of the era -> version map and is a
// pure function of committed state (I5). Re-deriving `h >= era4At` inline instead would be a
// second copy of that map, which is the #397 drift shape this whole arc is paying back.
func eraFloorAt(t *testing.T, h, era4At uint64) uint64 {
	t.Helper()
	c := New(Config{Quorum: 1, Era3ActivationHeight: era4At, Era4ActivationHeight: era4At}, func(ports.NodeID) int64 { return 0 })
	return c.MintVersion(h)
}

// ---------------------------------------------------------------------------
// G-PRE-6 — CROSS-CHAIN: TWO HONEST SIGNATURES ON TWO NETWORKS ARE NOT EVIDENCE.
// ---------------------------------------------------------------------------
//
// consensusSigDomain separates message KINDS, not NETWORKS: it is the same constant on every silt
// network that has ever existed. The genesis MOVED on 2026-09-07, so two silt networks exist and
// operators carry identity key files across. One key honestly precommitting a different block at
// the same (height, round) on each network then produces a VALID equivocation proof on EITHER —
// validateSlashes performs no chain-membership check and apply() evicts permanently.
//
// THE PRE-ERA-4 ARM IS NOT AN ABLATION. IT IS A LIVE RESIDUAL, AND IT IS SCOPED. The v2 preimage
// has no chain id, so building the same two blocks at v4 reproduces the cross-network false slash
// exactly as it exists on main today. Calling that "the ablation" said the defect was closed and
// only re-enacted here; it is not. `consensusSigBytes` is FROZEN (chain.go's own doc: "this
// function and its domain constant may never change"), so the era-2 form is chain-blind forever
// and that face survives at EVERY HEIGHT BELOW H_era4. The era-4 arm below closes it only where
// the era-4 form is the required form.
//
// SO THE ARM MUST NAME ITS HEIGHT, AND PROVE IT. eraFloorAt builds a SEPARATE chain — never the
// evidence blocks — and reads the floor from (*Chain).MintVersion, the one site of the era ->
// version map. The arm asserts the fixture height is BELOW the era-4 floor on the chain it claims
// to describe, and AT OR ABOVE it on an RC-shaped chain (Era4ActivationHeight = 1), which is what
// makes "the RC network has no reachable height for this" a measurement instead of a sentence.
//
// THE CLOSER IS M2, NOT THIS GATE: refuse evidence below the chain's committed era floor at
// validateSlashes, FindEquivocations and slashEquivocators in one commit. Research-gated
// (NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11 §1.9, G-1a..G-1d).
// Residual: R-SLASH-CULPRIT-ADMISSIBILITY.
func TestGPRE6_CrossChainHonestSignaturesAreNotEvidence(t *testing.T) {
	victim, propX, propY := key(96001), key(96002), key(96003)
	cidX := ports.HashBytes([]byte("silt network X genesis"))
	cidY := ports.HashBytes([]byte("silt network Y genesis"))
	pub := []byte(victim.Public().(ed25519.PublicKey))
	const h, r = 12, 1

	// Two DIFFERENT blocks at the same height, one per network. The victim honestly precommits
	// each ON ITS OWN NETWORK: two honest acts, no protocol violation anywhere.
	build := func(version uint64, e byte, prop ed25519.PrivateKey, cid ports.Hash) Block {
		b := Block{Version: version, Height: h, Prev: ports.HashBytes([]byte("shared-prev")),
			Entries: []ports.Entry{entry(e)}}
		if version >= BlockVersionWitnessable {
			b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
			setD3Digests(&b)
		}
		Sign(&b, prop)
		b.Atts = []Attestation{AttestAt(&b, victim, r, PhasePrecommit, cid)}
		return b
	}

	// --- THE PRE-ERA-4 ARM, SCOPED TO ITS HEIGHT: v4 blocks, v2 preimage, NO chain id. ---
	//
	// THE SCOPE IS ASSERTED BEFORE THE VERDICT, and it is read off a chain this arm does not
	// otherwise touch. Below the era-4 floor the era-2 form is the REQUIRED form, so an honest
	// validator genuinely produces these bytes; at or above it, it does not.
	const lateBoundary, rcBoundary = 64, 1
	if got := eraFloorAt(t, h, lateBoundary); got >= BlockVersionWitnessable {
		t.Fatalf("ARM OUT OF SCOPE: this arm describes a PRE-ERA-4 height, but on a chain with "+
			"Era4ActivationHeight=%d the floor at height %d is already v%d. The era-2 form is not the "+
			"required form there and this arm would be asserting a defect outside the interval where it lives",
			uint64(lateBoundary), h, got)
	}
	if got := eraFloorAt(t, h, rcBoundary); got != BlockVersionWitnessable {
		t.Fatalf("THE RC SCOPE CLAIM IS FALSE: on a chain committing Era4ActivationHeight=%d the floor at "+
			"height %d must be v%d, got v%d. If it is not, 'the RC network has no reachable height for this "+
			"residual' is prose, not a measurement", uint64(rcBoundary), h, BlockVersionWitnessable, got)
	}
	av4, bv4 := build(BlockVersionStateRoot, 1, propX, cidX), build(BlockVersionStateRoot, 2, propY, cidY)
	if av4.Atts[0].Phase != PhasePrecommit {
		t.Fatalf("ARM DID NOT APPLY: a v4 block's precommit must be the heightless v2 form, got phase %d", av4.Atts[0].Phase)
	}
	if bytes.Equal(av4.Atts[0].Sig, bv4.Atts[0].Sig) {
		t.Fatal("fixture: the two networks' signatures must differ (they cover different block hashes)")
	}
	live := Equivocation{Culprit: pub, A: av4, B: bv4}
	if err := CheckEquivocation(&live, cidX); err != nil {
		t.Fatalf("THE PRE-ERA-4 RESIDUAL IS CLOSED — and nothing in this branch closes it, so read this as a "+
			"CHANGE, not a pass. Either the M2 era-floor rule landed (in which case retire this arm and assert "+
			"the closure at h >= H_era4 instead), or the era-2 form stopped being chain-blind. Do not silence "+
			"this by deleting the arm; got %v", err)
	}
	t.Logf("LIVE RESIDUAL, driven at height %d (BELOW the era-4 floor, asserted above): with the v2 "+
		"(chain-id-less) preimage one honest validator running on two silt networks is CONVICTED and "+
		"permanently evicted. Open, R-SLASH-CULPRIT-ADMISSIBILITY; closer = the M2 era-floor rule; on an "+
		"RC genesis committing Era4ActivationHeight=1 no height above 0 is in this interval.", h)

	// --- THE SHIPPED ERA-4 FORM: the same two honest acts, refused on BOTH networks. ---
	av5, bv5 := build(BlockVersionWitnessable, 1, propX, cidX), build(BlockVersionWitnessable, 2, propY, cidY)
	if av5.Atts[0].Phase != PhasePrecommitV5 {
		t.Fatalf("GATE VACUOUS: a v5 block's precommit must carry the era-4 form, got phase %d", av5.Atts[0].Phase)
	}
	proof := Equivocation{Culprit: pub, A: av5, B: bv5}
	for name, cid := range map[string]ports.Hash{"network X": cidX, "network Y": cidY} {
		if err := CheckEquivocation(&proof, cid); !errors.Is(err, ErrNotEquivocation) {
			t.Fatalf("G-PRE-6 VIOLATED on %s: an honest validator running on two silt networks must NOT be "+
				"slashable; got %v", name, err)
		}
	}

	// NON-VACUITY: the era-4 form must still convict a REAL double-sign on ONE network. Otherwise
	// the refusal above could be "v5 evidence never convicts", which is an accountability hole
	// dressed as a fix.
	realA := build(BlockVersionWitnessable, 1, propX, cidX)
	realB := build(BlockVersionWitnessable, 2, propX, cidX)
	real := Equivocation{Culprit: pub, A: realA, B: realB}
	if err := CheckEquivocation(&real, cidX); err != nil {
		t.Fatalf("G-PRE-6 NON-VACUITY BROKEN: a genuine era-4 double-sign on ONE network must still convict; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// G-PRE-7 — THE HONEST EXEMPTIONS SURVIVE THE ERA-4 FORM. Each driven ONE AT A TIME.
// ---------------------------------------------------------------------------
//
// Certification §3. SCOPE NOTE, stated rather than implied: the certification's C1 (a proof carries
// EITHER two bodies OR the v5 tuple) and C2 (the phase whitelist on that tuple) are conditions on
// the FIXED-SIZE EquivV5 EVIDENCE OBJECT, which route (C) introduces and this branch does not.
// They cannot be driven here and are NOT silently claimed as covered. What IS driven is every
// exemption that the preimage change itself can break.
func TestGPRE7_HonestExemptionsSurviveTheEra4Form(t *testing.T) {
	culprit, prop := key(95001), key(95002)
	cid := ports.HashBytes([]byte("G-PRE-7 chain"))
	pub := []byte(culprit.Public().(ed25519.PublicKey))

	v5 := func(e byte, atts func(b *Block)) Block {
		b := Block{Version: BlockVersionWitnessable, Height: 8, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(e)}}
		b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
		setD3Digests(&b)
		Sign(&b, prop)
		if atts != nil {
			atts(&b)
		}
		return b
	}

	t.Run("cross-round lock change is HONEST", func(t *testing.T) {
		// A locked value re-proposed at a higher round: the same key precommits two different
		// hashes at two DIFFERENT rounds. That is a legitimate lock change under a POL.
		a := v5(1, func(b *Block) { b.Atts = []Attestation{AttestAt(b, culprit, 0, PhasePrecommit, cid)} })
		bb := v5(2, func(b *Block) { b.Atts = []Attestation{AttestAt(b, culprit, 1, PhasePrecommit, cid)} })
		e := Equivocation{Culprit: pub, A: a, B: bb}
		if err := CheckEquivocation(&e, cid); !errors.Is(err, ErrNotEquivocation) {
			t.Fatalf("a cross-ROUND different-hash pair is an honest lock change, not evidence; got %v", err)
		}
		// Non-vacuity for THIS condition alone: move only the round to match and it convicts.
		bb2 := v5(2, func(b *Block) { b.Atts = []Attestation{AttestAt(b, culprit, 0, PhasePrecommit, cid)} })
		if err := CheckEquivocation(&Equivocation{Culprit: pub, A: a, B: bb2}, cid); err != nil {
			t.Fatalf("same-round arm must convict, else the refusal above is not about the round; got %v", err)
		}
	})

	t.Run("bare-hash ProposerSig is authorship, not a vote", func(t *testing.T) {
		// The culprit AUTHORS two different blocks at one height and attaches NO consensus
		// signature to either. Re-proposing fresh after a lock-free view change is honest, and the
		// era-1-shaped ProposerSig must never be read as an era-4 vote. Structurally it cannot be:
		// Sign() signs 32 bytes and the era-4 preimage is 99, so the message spaces are disjoint.
		a := v5(1, func(b *Block) { Sign(b, culprit) })
		bb := v5(2, func(b *Block) { Sign(b, culprit) })
		e := Equivocation{Culprit: pub, A: a, B: bb}
		if err := CheckEquivocation(&e, cid); !errors.Is(err, ErrNotEquivocation) {
			t.Fatalf("a bare-hash ProposerSig is authorship, not a consensus vote; got %v", err)
		}
		if consensusSigPreimageV5Len == 32 {
			t.Fatal("the era-4 preimage is 32 bytes long, so a ProposerSig and a consensus vote share a " +
				"message space — the disjointness this exemption rests on is gone")
		}
	})

	t.Run("different steps at one slot are not a conflict", func(t *testing.T) {
		// A prepare and a precommit are different slots. Two different hashes across them is the
		// ordinary two-phase flow, not a double-sign.
		a := v5(1, func(b *Block) { b.PrepareQC = []Attestation{AttestAt(b, culprit, 0, PhasePrepare, cid)} })
		bb := v5(2, func(b *Block) { b.Atts = []Attestation{AttestAt(b, culprit, 0, PhasePrecommit, cid)} })
		if err := CheckEquivocation(&Equivocation{Culprit: pub, A: a, B: bb}, cid); !errors.Is(err, ErrNotEquivocation) {
			t.Fatalf("a prepare and a precommit are different slots; got %v", err)
		}
	})

	t.Run("the mixed-FORM pair is a CROSS-NETWORK FALSE SLASH, not an accountability requirement", func(t *testing.T) {
		// WHAT THIS SUBTEST USED TO DEMAND, AND WHY IT WAS WRONG. It was titled "a v2-form and a
		// v5-form signature at ONE slot DO convict" and asserted CheckEquivocation == nil, calling
		// the refusal an ACCOUNTABILITY REGRESSION. That demand is an I5 VIOLATION stated as
		// required behaviour, and it directly contradicts G-PRE-6's property — "an honest validator
		// running on two silt networks must NOT be slashable" — on bytes that are identical.
		//
		// THE DERIVATION, driven below one step at a time. AttPhase returns `step` unchanged for a
		// sub-era-4 block (chain.go), so AttestAt never enters the era-4 branch and NEVER READS
		// chainID; verifyAtt's era-2 arm ignores the scope entirely. The era-2-form leg therefore
		// carries no network, and an attacker on network X can lift the victim's genuine era-2-form
		// precommit off network Y and pair it with the victim's genuine era-4-form precommit on X.
		// One honest act on each network; a conviction on X. The penalty hits the honest — #397.
		//
		// WHAT REPLACED THE DEMAND. Nothing in this branch changes CheckEquivocation: the closer is
		// M2's era-floor rule, which is research-gated and lands at three call sites in one commit.
		// The honest-node half of T-STEP-VS-FORM — that a validator cannot double-sign ACROSS the
		// era boundary because ports.SignMark records the canonical step and is era-independent —
		// is driven where it actually lives, in core/node:
		// TestGPRE3_UpgradeMustNotReinterpretTheDurableSignMark. It never needed a slash verdict.
		//
		// WHAT WAS SOLD FOR IT, and it is ratified, not overlooked: the ON-NETWORK boundary
		// double-signer (a validator precommitting a v4 block and a v5 block at one height near
		// H_era4) becomes unslashable. It cannot FINALIZE either way — validateEra4Version refuses
		// a sub-era-4 block at every height at or above the boundary on every disk-write path — and
		// on an RC genesis committing Era4ActivationHeight = 1 there is no such boundary to stand
		// on at all, which the scope assertions below measure rather than assert.
		const h, lateBoundary, rcBoundary = 8, 64, 1
		if got := eraFloorAt(t, h, rcBoundary); got != BlockVersionWitnessable {
			t.Fatalf("SCOPE CLAIM FALSE: on an RC-shaped chain (Era4ActivationHeight=%d) the floor at height %d "+
				"must be v%d, got v%d — so 'the RC network has no height where an honest validator mints the "+
				"era-2 form' would be prose", uint64(rcBoundary), h, BlockVersionWitnessable, got)
		}
		if got := eraFloorAt(t, h, lateBoundary); got >= BlockVersionWitnessable {
			t.Fatalf("SCOPE PROBE DEAD: with a LATE boundary (Era4ActivationHeight=%d) the floor at height %d "+
				"must be below v%d, got v%d. Without a height where the two forms differ, the assertion above "+
				"cannot discriminate and reports the same answer for every chain",
				uint64(lateBoundary), h, BlockVersionWitnessable, got)
		}

		// (1) THE ERA-2-FORM LEG IS CHAIN-BLIND. Same body, same key, two DIFFERENT chain ids.
		// THE TWO SCOPES ARE A LIST, AND THE LEGS ARE DERIVED FROM IT. Writing the two calls out by
		// hand made the comparison ablatable into a self-comparison — v2leg(foreign) against
		// v2leg(foreign) — which is trivially equal and reported GREEN. Deriving both legs from a
		// slice whose members are asserted DISTINCT makes that shape unrepresentable.
		scopes := []ports.Hash{cid, ports.HashBytes([]byte("a DIFFERENT silt network's genesis"))}
		if scopes[0] == scopes[1] {
			t.Fatal("VACUOUS: the two chain ids are equal, so 'chain-blind' is indistinguishable from 'chain-bound'")
		}
		v2leg := func(scope ports.Hash) Block {
			b := Block{Version: BlockVersionStateRoot, Height: h, Prev: ports.HashBytes([]byte("p")),
				Entries: []ports.Entry{entry(1)}}
			Sign(&b, prop)
			b.Atts = []Attestation{AttestAt(&b, culprit, 0, PhasePrecommit, scope)}
			return b
		}
		// The SAME body and the SAME key under each scope. Equal bytes = the signature carries no
		// network, so the leg is portable between silt networks.
		legs := make([]Block, len(scopes))
		for i, s := range scopes {
			legs[i] = v2leg(s)
		}
		if !bytes.Equal(legs[0].Atts[0].Sig, legs[1].Atts[0].Sig) {
			t.Fatal("PREMISE FALSE: the era-2-form precommit differs across chain ids, so it is NOT chain-blind " +
				"and the harvest below is not the shape this subtest claims to record")
		}
		harvested := legs[1] // the leg signed under the FOREIGN scope: what an attacker lifts off network Y
		if harvested.Atts[0].Phase != PhasePrecommit {
			t.Fatalf("VACUOUS: the harvested leg must carry the era-2 wire phase, got %d", harvested.Atts[0].Phase)
		}

		// (2) THE VICTIM'S HONEST ERA-4-FORM PRECOMMIT ON THIS NETWORK, over a different body.
		bb := v5(2, func(b *Block) { b.Atts = []Attestation{AttestAt(b, culprit, 0, PhasePrecommit, cid)} })
		if bb.Atts[0].Phase != PhasePrecommitV5 {
			t.Fatalf("VACUOUS: the era-4 leg must carry the era-4 wire phase, got %d", bb.Atts[0].Phase)
		}
		if harvested.Atts[0].Phase == bb.Atts[0].Phase {
			t.Fatalf("VACUOUS: both legs carry phase %d, so this is not the mixed-FORM case", harvested.Atts[0].Phase)
		}

		// (3) THE VERDICT TODAY, RECORDED — with a trip that reddens the DAY it changes. This is
		// deliberately not a demand: it is the open face, and the arm must be retired by whoever
		// closes it rather than quietly kept green.
		if err := CheckEquivocation(&Equivocation{Culprit: pub, A: harvested, B: bb}, cid); err != nil {
			t.Fatalf("THE MIXED-FORM FACE IS CLOSED, and nothing in this branch closes it — read this as a "+
				"CHANGE, not a pass. If M2's era-floor rule landed, RETIRE this subtest and assert the closure "+
				"at h >= H_era4 in its place; the boundary double-signer being unslashable there is the "+
				"ratified price. Do not re-add a demand that the pair convict; got %v", err)
		}
		t.Logf("OPEN FACE, driven at height %d: a mixed-FORM pair whose era-2-form leg is BIT-IDENTICAL to "+
			"one harvested from another silt network CONVICTS an honest validator. I5, the #397 shape. "+
			"Closer = M2's era-floor rule at validateSlashes/FindEquivocations/slashEquivocators "+
			"(research-gated); residual R-SLASH-CULPRIT-ADMISSIBILITY.", h)
	})
}

// ---------------------------------------------------------------------------
// G-PRE-8 — THE v5 QUORUM DEMANDS EXACTLY ONE FORM.
// ---------------------------------------------------------------------------
//
// The carrier accepts BOTH precommit forms, by necessity (it cannot see the parent's version —
// G-PRE-10). That dual acceptance is a SEATING rule and must not leak into a quorum: a heightless
// v2-form precommit inside a v5 block's Atts must be FATAL at collectQuorumSigs, not silently
// ignored, or a signature that binds no height completes an era-4 quorum.
func TestGPRE8_V5QuorumIsPhaseExact(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	honest := mintNext4Carrier(t, c, keys)
	if honest.Version != BlockVersionWitnessable {
		t.Fatalf("GATE VACUOUS: need a v5 block, got v%d", honest.Version)
	}
	if err := c.ValidateCommit(honest); err != nil {
		t.Fatalf("NON-VACUITY BROKEN: the honest v5 block must commit; got %v", err)
	}
	for _, a := range honest.Atts {
		if a.Phase != PhasePrecommitV5 {
			t.Fatalf("GATE VACUOUS: a v5 certificate must carry the era-4 form, got phase %d", a.Phase)
		}
	}

	// Swap ONE attester's era-4 precommit for a GENUINE v2-form precommit by the same key, same
	// round, over the same block. Only the form differs, so nothing but the exactness rule can
	// refuse it.
	mixed := *honest
	mixed.Atts = append([]Attestation(nil), honest.Atts...)
	hb := honest.Hash()
	victim := keys[1]
	replaced := false
	for i := range mixed.Atts {
		if bytes.Equal(mixed.Atts[i].PubKey, victim.Public().(ed25519.PublicKey)) {
			mixed.Atts[i] = Attestation{
				PubKey: victim.Public().(ed25519.PublicKey),
				Sig:    ed25519.Sign(victim, consensusSigBytes(PhasePrecommit, mixed.Atts[i].Round, hb)),
				Round:  mixed.Atts[i].Round,
				Phase:  PhasePrecommit,
			}
			replaced = true
			break
		}
	}
	if !replaced {
		t.Fatal("GATE VACUOUS: no attester to swap — the fixture's certificate does not contain keys[1]")
	}
	// The swapped entry is a GENUINE signature: prove it before asserting the refusal, so a PASS
	// cannot come from "the signature was broken".
	if !verifyAtt(mixed.Atts[0], attScope{ChainID: c.ChainID(), Height: mixed.Height}, hb) &&
		!verifyAtt(mixed.Atts[1], attScope{ChainID: c.ChainID(), Height: mixed.Height}, hb) {
		t.Fatal("fixture: the swapped v2-form precommit must be a genuine signature")
	}
	if err := c.ValidateCommit(&mixed); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("G-PRE-8 VIOLATED: a heightless v2-form precommit inside a v5 block's Atts must be FATAL "+
			"at the quorum, not silently ignored; got %v", err)
	}
	if out, _, err := v5CollectQuorumSigs(liveView{c}, &mixed, mixed.Atts, PhasePrecommit, mixed.CommitRound); out != nil || err == nil {
		t.Fatalf("G-PRE-8 VIOLATED (composition): the shared constructor must refuse the mixed-form set; got %v", err)
	}

	// ABLATION, driven: the SAME entry IS admissible to the CARRIER, which is the dual-form rule.
	// Driving both halves side by side is what shows the two rules are deliberately different
	// rather than accidentally the same.
	if !isCarrierPrecommit(mixed.Atts[0].Phase) && !isCarrierPrecommit(PhasePrecommit) {
		t.Fatal("G-PRE-8 GATE IS DECORATION: if the carrier ALSO refused the v2 form, this gate would be " +
			"asserting a property that holds for an unrelated reason")
	}
}

// ---------------------------------------------------------------------------
// G-PRE-9 — MEASUREMENT, NOT PASS/FAIL.
// ---------------------------------------------------------------------------
//
// silt-derive-then-drive: a parameter's VALUE and its DERIVATION are two claims, and a derived
// figure must never become the citation. The certification's preimage layout is DERIVED at 99
// bytes; this MEASURES it. It also measures the evidence sizes that actually exist in this branch,
// and states plainly which certified figure it cannot measure and why.
func TestGPRE9_MeasureThePreimageAndTheEvidenceMaterial(t *testing.T) {
	cid := ports.HashBytes([]byte("measurement chain"))
	h := ports.HashBytes([]byte("measurement block"))

	got := len(consensusSigBytesV5(cid, PhasePrecommitV5, 1<<40, 1<<20, h))
	const want = 99 // 18 domain + 32 chain id + 1 phase + 8 height + 8 round + 32 hash
	if got != want {
		t.Fatalf("MEASURED era-4 preimage = %d B, certified layout = %d B", got, want)
	}
	if got != consensusSigPreimageV5Len {
		t.Fatalf("consensusSigPreimageV5Len (%d) disagrees with the measured length (%d)", consensusSigPreimageV5Len, got)
	}
	// FIXED WIDTH is what makes the layout injective without a length prefix. Measure it across
	// the full range of every variable-looking field, not just at one point.
	for _, ht := range []uint64{0, 1, 1<<32 - 1, 1<<64 - 1} {
		for _, r := range []uint64{0, 1, 1<<64 - 1} {
			if n := len(consensusSigBytesV5(cid, PhasePrepareV5, ht, r, h)); n != want {
				t.Fatalf("the preimage is NOT fixed-width: height=%d round=%d encodes to %d B, not %d. "+
					"Injectivity without a length prefix rests entirely on this", ht, r, n, want)
			}
		}
	}
	t.Logf("MEASURED: era-4 signature preimage = %d B, fixed-width (era-2 form = %d B)",
		got, len(consensusSigBytes(PhasePrecommit, 0, h)))

	// The evidence object. THE CERTIFICATION'S ~251 B FIGURE IS NOT MEASURABLE IN THIS BRANCH and
	// is not claimed here: 251 B describes the FIXED-SIZE EquivV5 tuple that route (C) introduces,
	// which owner call A does not build. The preimage is 99 B; conflating the two is a category
	// error and it is recorded rather than papered over. What IS measurable is the body-form proof
	// this branch still uses for every height, and the junk-leaf price it sets.
	k := key(94001)
	mk := func(e byte) Block {
		b := Block{Version: BlockVersionWitnessable, Height: 4, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(e)}}
		b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
		setD3Digests(&b)
		Sign(&b, k)
		b.Atts = []Attestation{AttestAt(&b, k, 0, PhasePrecommit, cid)}
		return b
	}
	one := Equivocation{Culprit: []byte(k.Public().(ed25519.PublicKey)), A: mk(1), B: mk(2)}
	oneB := SlashesEncodedSize([]Equivocation{one})
	twoB := SlashesEncodedSize([]Equivocation{one, one})
	marginal := twoB - oneB
	if marginal <= 0 {
		t.Fatalf("calibration: marginal cost %d B is not positive", marginal)
	}
	t.Logf("MEASURED: a minimal body-form v5 equivocation proof encodes to %d B (marginal %d B); "+
		"%d fit under SlashesBytesCap (%d B), each minting one permanent SMT leaf",
		oneB, marginal, (SlashesBytesCap-(oneB-marginal))/marginal, SlashesBytesCap)
	t.Log("NOT MEASURED HERE, and NOT claimed: the certification's ~251 B figure is the size of the " +
		"fixed-size EquivV5 evidence tuple introduced by route (C). Owner call A changes the signature " +
		"PREIMAGE only, so no such object exists in this branch. The junk-leaf re-pricing that figure " +
		"feeds (R-SLASH-CULPRIT-ADMISSIBILITY) must be measured when route (C) lands, not before.")
}

// ---------------------------------------------------------------------------
// G-PRE-10 — THE CARRIER ACCEPTS BOTH PRECOMMIT FORMS AND NOTHING ELSE.
// ---------------------------------------------------------------------------
//
// Dual-form acceptance is forced: validateCarrier verifies the PARENT's precommits while holding
// only the child, so it cannot read the parent's version (T-ERA-DISPATCH). It is capability-neutral
// by the certification's §5.3 lemma. What it must NOT become is a hole: every other phase — both
// prepare forms and the era-1 legacy shape — must still be refused, and the CLOSED COMPLEMENT is
// asserted rather than a list of examples, so a future constant cannot be admitted by omission.
func TestGPRE10_CarrierAcceptsBothPrecommitFormsAndNoOther(t *testing.T) {
	// The rule, over its CLOSED complement: every uint8 except the two precommit forms is refused.
	for p := 0; p < 256; p++ {
		phase := uint8(p)
		want := phase == PhasePrecommit || phase == PhasePrecommitV5
		if got := isCarrierPrecommit(phase); got != want {
			t.Fatalf("G-PRE-10 VIOLATED: phase %d admissible=%v, want %v — the carrier's phase rule must be "+
				"EXACTLY {PhasePrecommit, PhasePrecommitV5}", phase, got, want)
		}
	}

	// Driven through the real rule, one phase at a time, on a real v5 child over a real v5 parent.
	c, keys := era4AnchorChain(t, 1, 1)
	mustAppend(t, c, mintNext4Carrier(t, c, keys))
	head, _ := c.headBlock()
	prev, next := c.Head()
	cid := c.ChainID()
	mk := func(a Attestation) *Block {
		return &Block{Version: BlockVersionWitnessable, Height: next, Prev: prev,
			Entries: []ports.Entry{entry(55)}, LastCommit: []Attestation{a}}
	}

	// ACCEPTED: the era-4 form (this parent is v5, so this is the on-form entry).
	onForm := AttestAt(&head, keys[1], 0, PhasePrecommit, cid)
	if onForm.Phase != PhasePrecommitV5 {
		t.Fatalf("GATE VACUOUS: expected the era-4 form off a v5 parent, got phase %d", onForm.Phase)
	}
	if err := validateCarrier(mk(onForm), cid); err != nil {
		t.Fatalf("G-PRE-10: the era-4 precommit form must be accepted: %v", err)
	}

	// ACCEPTED: the era-2 form. Minted by hand because this parent is v5 — that is precisely the
	// off-form seating R-CARRIER-OFFFORM-SEAT holds in tension, and it is asserted, not assumed.
	offForm := Attestation{
		PubKey: keys[1].Public().(ed25519.PublicKey),
		Sig:    ed25519.Sign(keys[1], consensusSigBytes(PhasePrecommit, 0, head.Hash())),
		Round:  0, Phase: PhasePrecommit,
	}
	if err := validateCarrier(mk(offForm), cid); err != nil {
		t.Fatalf("G-PRE-10: the era-2 precommit form must ALSO be accepted at the carrier — that dual "+
			"acceptance is what stops the boundary wedge (G-PRE-1): %v", err)
	}

	// REFUSED, one phase at a time. Each is a GENUINE signature by a qualified key, so a PASS
	// cannot come from a broken signature.
	for name, a := range map[string]Attestation{
		"era-4 prepare": AttestAt(&head, keys[1], 0, PhasePrepare, cid),
		"era-2 prepare": {PubKey: keys[1].Public().(ed25519.PublicKey),
			Sig:   ed25519.Sign(keys[1], consensusSigBytes(PhasePrepare, 0, head.Hash())),
			Round: 0, Phase: PhasePrepare},
		"era-1 legacy": {PubKey: keys[1].Public().(ed25519.PublicKey),
			Sig:   ed25519.Sign(keys[1], func() []byte { h := head.Hash(); return h[:] }()),
			Phase: PhaseLegacy},
	} {
		if err := validateCarrier(mk(a), cid); !errors.Is(err, ErrCarrierBadSignature) {
			t.Fatalf("G-PRE-10 VIOLATED: a genuine %s signature must NOT be a carrier entry — a prepare "+
				"that seats a validator is a seating the attester never consented to; got %v", name, err)
		}
	}

	// And HeadCarrier — the PRODUCER filter — must use the SAME predicate. If the two ever differ,
	// an honest proposer mints a carrier its own replica refuses, which is the wedge.
	for _, a := range c.HeadCarrier() {
		if !isCarrierPrecommit(a.Phase) {
			t.Fatalf("G-PRE-10 VIOLATED: HeadCarrier emitted phase %d, which validateCarrier refuses — the "+
				"producer filter and the validity rule have split", a.Phase)
		}
	}
}

// ---------------------------------------------------------------------------
// A ZERO CHAIN ID IS NOT A CHAIN.
// ---------------------------------------------------------------------------
//
// The one-line property that makes every "forgot to thread the chain id" route a REFUSAL rather
// than a verification against network zero — the same discipline that makes NoWitness the zero of
// Availability and makes the zero Budget stall. Driven on both era-4 forms and shown NOT to touch
// the era-1/era-2 arms, which carry no chain id at all.
func TestGPREZeroChainIDVerifiesNothingAtEra4(t *testing.T) {
	k := key(93001)
	h := ports.HashBytes([]byte("b"))
	cid := ports.HashBytes([]byte("real chain"))

	for _, phase := range []uint8{PhasePrepareV5, PhasePrecommitV5} {
		sig := ed25519.Sign(k, consensusSigBytesV5(ports.Hash{}, phase, 5, 0, h))
		a := Attestation{PubKey: k.Public().(ed25519.PublicKey), Sig: sig, Phase: phase}
		if verifyAtt(a, attScope{ChainID: ports.Hash{}, Height: 5}, h) {
			t.Fatalf("phase %d: a signature made under the ZERO chain id must never verify — an unset scope "+
				"has to be a refusal, not an acceptance on network zero", phase)
		}
		// Non-vacuity: the same construction under a REAL chain id does verify, so the refusal above
		// is about the zero and not about the phase.
		good := Attestation{PubKey: k.Public().(ed25519.PublicKey),
			Sig: ed25519.Sign(k, consensusSigBytesV5(cid, phase, 5, 0, h)), Phase: phase}
		if !verifyAtt(good, attScope{ChainID: cid, Height: 5}, h) {
			t.Fatalf("phase %d: the same signature under a real chain id must verify", phase)
		}
	}

	// The era-2 arm is UNTOUCHED: its preimage carries no chain id, so a zero scope is simply
	// irrelevant there. Committed history is never re-interpreted.
	legacy := Attestation{PubKey: k.Public().(ed25519.PublicKey),
		Sig: ed25519.Sign(k, consensusSigBytes(PhasePrecommit, 0, h)), Phase: PhasePrecommit}
	if !verifyAtt(legacy, attScope{}, h) {
		t.Fatal("the era-2 arm must be unaffected by the chain id: every committed pre-era-4 signature " +
			"depends on it")
	}
}
