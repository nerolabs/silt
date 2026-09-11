package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

// era-4 (v5) — THE ONE ACCEPT COMPOSITION, with TWO entry points (M-2).
//
// ValidateProposalV5 (P1…P13) is the attester's pre-sign rule; ValidateCommitV5 is
// ValidateProposalV5 followed by C1…C5 (the two quorum stacks). The full node runs both over
// liveView (chain.go dispatches for Version >= BlockVersionWitnessable); the trustless floor box runs
// ValidateCommitV5 over a witness-backed provenView. There is ONE body per rule, so a box can no
// longer reproduce a predicate's TAIL while missing the precondition a different validation stage
// established — the defect shape defeated six times in the 2026-09 cycle (N1 the author screen,
// RT2-CARRIER-13/13b/13c the parent binding, N2/N3/N6/N7 the driver-asserted box state, N5 the
// malformed-leg divergence). And there is ONE implementation for both node entry points, so the
// attester signs under the rule the committer accepts under — the #402 one-function-two-callers law
// applied to the surface that drifted (P-table delta certification M-2).
//
// Certified (binding):
// /Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md
// /Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md
//
// THE CONTRACT IS AN IMPLICATION, NOT A BICONDITIONAL:
//
//	box.Accept  ⇒  node.Accept
//
// A box may stall where a node accepts (it holds less). It may NEVER accept where a node rejects.
// The word "exact" does not appear in this file's contracts.
//
// WHAT IT DOES NOT COVER — state it, do not let it be inferred. ValidateCommit is the accept path
// for a COMMITTED block (MsgCommitBlock → Append → ValidateCommit). It is not every disk-write
// path: appendStructural runs validateStructural, whose quorum leg is a RequiredQuorum() count over
// b.Atts with no qualification filter, anchor leg or weight leg — weaker and different, Reload
// only (#380 M-380-1 moved its floor from the bare cfg.Quorum to RequiredQuorum so a replay accepts
// what the commit path accepted); AppendGenesis is height 0, which has no parent to bind. Both are
// OUTSIDE this guarantee.
//
// BG-1 — THIS EXTRACTION IS v5-ONLY, AND IT IS ON THE LIVE PATH. The node dispatches here only
// for b.Version >= BlockVersionWitnessable; the era-1 and era-2 legs are byte-untouched. On a
// default network ValidateProposal/ValidateCommit DO enter these functions at every height above
// the genesis: -era4-activation-height defaults to 1 (cmd/silt/daemon.go, assigned into
// chain.Config and pinned by cmd/silt TestTheThreeGenesisFlagsAreDeclaredAndWired), era4Active
// takes its genesis-override branch whenever that height is non-zero, and MintVersion therefore
// returns v5 from height 1. Height 0 is v2, and nothing below the boundary reaches here.
//
// The readiness stamp IS 3 (NewBondReg stamps BlockVersionRegGate), and it decides nothing about
// which leg runs: the tally that consumes it sits inside rotateEpoch behind
// cfg.Era4ActivationHeight == 0, so a chain on the shipped default of 1 never reaches it. The
// stamp gates the LATCH route only.
//
// Identity of a replica before this round and after is provable by construction, WITH M-1…M-5
// (§6 of the delta certification): without the legacy leg (M-1) the composition refuses a v5
// block a legacy node accepts; without P8b/the P5 clause (M-5) it accepts where the node rejects.
//
// R-SHARED-RULE-BLINDSPOT — HELD-IN-TENSION, AND IT GROWS HERE. Once the node's v5 path IS this
// composition, a box-vs-node gate is structurally BLIND to a bug inside the shared body: both sides
// move together. M-2 grows it further: the attester and committer legs share one implementation,
// so a bug inside it is invisible to every box-vs-node AND attester-vs-committer comparison. The
// debt lives in the composition's own unit gates and in the STAGE-COVER gate
// (composition_stage_cover_v5_test.go), which derives the node's stage list from
// ValidateProposal/ValidateCommit and asserts this file covers every one.
//
// THE STAGES ARE MIRRORS OVER StateView, NOT CALLS INTO chain.go. The node's stage functions are
// *Chain methods reading c.<map>; this composition takes no receiver (the COMPILER, not an AST
// allowlist, is what stops it reading live box state), so it cannot call them. Each mirror is
// re-derived against the node's body line by line; nodeStages below names every node stage and
// its mirror, and the STAGE-COVER gate holds the list. The one function called DIRECTLY is
// validateCarrier, which is receiverless by design (carrier.go).

var (
	// ErrViewNoWitness marks an IndeterminateTrustlessly whose cause is a read the view could not
	// see. It NAMES the read, because a stall whose reason is "something was missing" is how the
	// next round talks itself into a default. It is never returned by liveView (G-5 pins that as a
	// diagnostic); if it ever is, the cost is a refusal, never an acceptance.
	ErrViewNoWitness = errors.New("chain: floor-box state view has no witness for a required committed read — stall")

	// ErrWitnessBudgetUnset marks the M-4 STALL: the view's Budget is the zero value. Zero is
	// neither "unlimited" nor "nothing"; it is "unset", and an unset ceiling admits nothing.
	ErrWitnessBudgetUnset = errors.New("chain: floor-box witness budget is unset (the zero Budget stalls; a box derives its ceiling from its own config, a node returns UnlimitedBudget()) — stall")

	// ErrWitnessBudgetExceeded marks an IndeterminateTrustlessly from the BG-3 box-owned byte
	// budget: the block's frame is larger than this view will pay for. It is checked BEFORE any
	// per-entry crypto and before any witness is looked at, so an out-of-budget block costs one
	// encode. Stall-adding only — it can never turn a node-Reject into a box-Accept. See Budget for
	// the measured 2.67 GiB exposure it bounds.
	ErrWitnessBudgetExceeded = errors.New("chain: floor-box witness budget exceeded — stall (R-CARRIER-BYTES: the validity-rule ceiling is not built yet)")

	// ErrAboveCurrentEraVersion is the L1 partition's upper edge: a block from an era this binary
	// does not know. Decode already refuses it on the wire (versionSupported); the composition
	// keeps the partition exact so an in-process caller cannot route a v6 block through v5 rules.
	ErrAboveCurrentEraVersion = errors.New("chain: block version is above the current era (v5) — this binary cannot validate it")
)

// stall builds the named-read stall reason. Naming the read is the #572 attribution lesson: a
// refusal that does not name its actual branch sends a debug down a false trail.
func stall(read string) error { return fmt.Errorf("%w: %s", ErrViewNoWitness, read) }

// nodeStage is one row of nodeStages: a node validity stage and where the composition carries it.
type nodeStage struct {
	// ID is the certification's label (P1…P13b, C1…C5, Q1…Q4).
	ID string
	// Node is the node function that IS the stage, or "" for a predicate written inline in
	// ValidateProposal / requireQuorumStack (arm B of the STAGE-COVER gate holds those).
	Node string
	// Mirror is the composition function that carries the stage, or "" when the composition writes
	// it inline in the same place the node does. Equal to Node when the node function is called
	// DIRECTLY (validateCarrier, receiverless by design).
	Mirror string
	// Nested are the error-returning callees the node stage reaches (the transitive closure the
	// STAGE-COVER gate derives), with their mirrors.
	Nested []nodeStage
	// Substituted names the reason a stage is NOT mirrored but replaced by a StateView method.
	// Exactly ONE row may carry it (validateEra3Roots → CommittedRoots). A second substitution is a
	// research-gated event, not a table edit.
	Substituted string
}

// nodeStages is the node's validity stage list for a v5 block, in the node's order — P-table
// delta certification §1 (24 rows). The STAGE-COVER gate DERIVES the call-shaped rows from
// ValidateProposal / ValidateCommit / requireQuorumStack by AST walk and asserts this table equals
// the derivation, element for element, including order; then asserts every row is carried by the
// composition. Do not hand-edit this table without the gate: it drifted once in four days.
//
// TWO ACCELERATOR SUBSTITUTIONS THIS TABLE DOES NOT SHOW (PE ruling F-5, 2026-09-08). The one
// SUBSTITUTED row is validateEra3Roots. Two node helpers that return no error — and so are not
// rows — are also not mirrored but replaced by the era-4 committed accelerator: qualifiedCount()
// (chain.go, a live filter over bonded/slashed/MinBond) is read as len(Qualified()) in
// v5ValidatorSetSize, and liveQualifiedSet() is read as Qualified() in v5EffectiveEpochSet's
// recovery arm. Equal iff the era-4 maintenance invariant qualified == filter(bonded, slashed,
// MinBond) holds at the five apply() sites; the guard is TestQualifiedMaintenanceDriftGuard
// (modelcheck_era4_maintenance_test.go), which must not be weakened — for a v5 block
// len(qualified) is now an input to RequiredQuorum, a safety quantity
// (R-QUALIFIED-ACCELERATOR-SAFETY). The v4/v5 parity oracle drives both reads against the node's
// live filters (the recovery-boundary and de-mature regimes).
var nodeStages = []nodeStage{
	// ---- ValidateProposal (chain.go) ----
	{ID: "P1", Node: "", Mirror: ""},                           // parent binding: (b.Height, b.Prev) vs v.Head()
	{ID: "P2", Node: "", Mirror: ""},                           // proposer key size
	{ID: "P3", Node: "", Mirror: ""},                           // proposer signature over b.Hash()
	{ID: "P4", Node: "", Mirror: "v5RequireProposerQualified"}, // proposerQualifiedAt (+ the #572 branches, P4a)
	{ID: "P5", Node: "", Mirror: ""},                           // non-empty block, SIX clauses (M-5)
	{ID: "P6", Node: "validateTakedowns", Mirror: "v5ValidateTakedowns"},
	{ID: "P7", Node: "validateBondRegs", Mirror: "v5ValidateBondRegs", Nested: []nodeStage{
		{Node: "validateBondRegWindow", Mirror: "v5ValidateBondRegWindow", Nested: []nodeStage{
			{Node: "validateBondReg", Mirror: "v5ValidateBondReg"},
		}},
	}},
	{ID: "P8", Node: "validateSlashes", Mirror: "v5ValidateSlashes"},
	{ID: "P8b", Node: "validateIssuerKeys", Mirror: "v5ValidateIssuerKeys"}, // v5-ONLY (M-5)
	{ID: "P9", Node: "ValidateEntry", Mirror: "v5ValidateEntries"},          // the entry loop + per-entry rule
	{ID: "P10", Node: "validateEra3Version", Mirror: "v5ValidateEraVersions"},
	{ID: "P11", Node: "validateEra4Version", Mirror: "v5ValidateEraVersions"},
	{ID: "P12", Node: "validateCarrier", Mirror: "validateCarrier"}, // called DIRECTLY: receiverless
	{ID: "P13a", Node: "validateEra3Roots", Substituted: "StateView.CommittedRoots — the ONE substituted step (StateRoot equality)", Nested: []nodeStage{
		{Node: "postApplyRoots"},
	}},
	{ID: "P13b", Node: "validateEra3Roots", Substituted: "StateView.CommittedRoots — the ONE substituted step (LogRoot equality; k = 0 closes, k ≥ 1 stalls on the proven view)", Nested: []nodeStage{
		{Node: "postApplyRoots"},
	}},
	// ---- ValidateCommit (chain.go), after ValidateProposal ----
	{ID: "C1", Node: "requireProposerPrepare", Mirror: "v5RequireProposerPrepare"},
	{ID: "C2", Node: "collectQuorumSigs", Mirror: "v5CollectQuorumSigs"},
	{ID: "C3", Node: "requireQuorumStack", Mirror: "v5RequireQuorumStack", Nested: []nodeStage{
		{Node: "requireEpochWeightQuorum", Mirror: "v5RequireEpochWeightQuorum"},
		{Node: "requireDeMatureSuperQuorum", Mirror: "v5RequireDeMatureSuperQuorum"},
	}},
	{ID: "C4", Node: "collectQuorumSigs", Mirror: "v5CollectQuorumSigs"},
	{ID: "C5", Node: "requireQuorumStack", Mirror: "v5RequireQuorumStack", Nested: []nodeStage{
		{Node: "requireEpochWeightQuorum", Mirror: "v5RequireEpochWeightQuorum"},
		{Node: "requireDeMatureSuperQuorum", Mirror: "v5RequireDeMatureSuperQuorum"},
	}},
	// ---- requireQuorumStack (chain.go), the four phase-independent requirements ----
	{ID: "Q1", Node: "", Mirror: "v5RequiredQuorum"},        // len(seen) < RequiredQuorum()
	{ID: "Q2", Node: "", Mirror: "v5RequiredLaunchAnchors"}, // launch-anchor majority
	{ID: "Q3", Node: "requireEpochWeightQuorum", Mirror: "v5RequireEpochWeightQuorum"},
	{ID: "Q4", Node: "requireDeMatureSuperQuorum", Mirror: "v5RequireDeMatureSuperQuorum"},
}

// ValidateProposalV5 is the ONE proposal-accept composition for a v5 block: P1…P13, preceded by
// the two box-owned screens (0: the L1 version partition; 0b: the BG-3 frame budget).
//
// It takes NO *Chain receiver. The order is the NODE's path (ValidateProposal, chain.go), not a
// function list. P4 (proposer qualification) runs BEFORE the quorum stacks in ValidateCommitV5:
// the author screen the box missed is not inside requireEpochWeightQuorum — that function's tail
// is `support := set[proposer]` with no screen, and that is CORRECT, because P4 already refused the
// block. A box that reproduces the weight tally and not P4 is exactly N1.
//
//  0. L1 version partition        box-owned scope; node: no-op            O(1)
//     0b. BG-3 frame byte budget      v.WitnessBudget(); node: unlimited      one encode
//  1. P1  parent binding          v.Head() vs (b.Prev, b.Height)          O(1)
//  2. P2, P3                      proposer key size, proposer signature   1 verify
//  3. P4 (+P4a verbatim)          proposer qualification, both modes      1-2 reads
//  4. P5                          non-empty block, six clauses            O(1)
//     P6                          takedowns                               O(payload)
//  5. P7, P8, P8b                 bond regs, slashes, ISSUER KEYS         O(payload)+verifyBond
//  4. P9                          the entry loop                          O(payload)
//     P10, P11                    the era version-boundary rules          O(1)
//  6. P12 validateCarrier(b)      the SHARED carrier validity rule        |LastCommit| verifies
//  8. P13a ∧ P13b                 THE ONE SUBSTITUTED STEP (both roots)   O(payload)+O(registry)
//
// There is NO S2 mode fence here (M-1): the node's liveView takes the legacy branch through
// v.Rep(); a fence on Objective() would stall a legacy node, the refuted direction. The fence is
// box-entry code, where provenView.Rep = NoWitness already stalls a legacy box.
func ValidateProposalV5(v StateView, b *Block) (FloorBoxOutcome, error) {
	// ---- 0. L1 VERSION PARTITION. ----
	if out, err := v5VersionPartition(b); out != Accept {
		return out, err
	}
	// ---- 0b. BG-3 FRAME BYTE BUDGET — before any per-entry crypto, before any witness. ----
	if err := v5CheckBudget(v.WitnessBudget(), b); err != nil {
		return IndeterminateTrustlessly, err
	}

	// ---- 1. P1 PARENT BINDING. The read of the view's OWN position, not a predicate. ----
	// This is the missing precondition behind RT2-CARRIER-13/13b/13c: validateCarrier verifies
	// signatures over b.Prev, and without this line b.Prev is whatever the block's author chose.
	head := v.Head()
	if b.Height != head.NextHeight || b.Prev != head.Hash {
		return Reject, fmt.Errorf("%w: got height %d prev %s, want height %d prev %s",
			ErrWrongParent, b.Height, b.Prev, head.NextHeight, head.Hash)
	}

	// ---- 1b. (d-3) DIGEST CONSISTENCY. Block-local, no state read, no witness. ----
	// The MIRROR of validateD3Digests on the node path (G-D13: a node body the composition
	// mirrors changed, so the mirror is re-derived here in the SAME commit — updating the pin
	// alone would split the v5 accept path from the v2/v4 path, which is R-PTABLE-DRIFT).
	//
	// It runs BEFORE the proposer-signature check below on purpose. From era-4 the preimage folds
	// AnswerDigest / SlashesDigest in place of the payloads, so a signature verifies over a block
	// whose heavy content is committed only THROUGH those digests. Checking them first means a
	// block whose digest lies is refused on its own terms rather than passing a signature check
	// that says nothing about the payload.
	//
	// Reject, not Indeterminate: the inputs are entirely block-local, so a floor box can decide it
	// without witnessing anything.
	if err := validateD3Digests(b); err != nil {
		return Reject, err
	}

	// ---- 1c. GENESIS-CONFIG PLACEMENT. Block-local, no state read. ----
	// The mirror of validateParamsPlacement on the node path (G-D13). Only the genesis block may
	// commit consensus params: they bind because the GENESIS hash covers them, and Reconcile's
	// foreign-genesis check reads blocks[0] alone. Params on any later block would be hash-covered
	// by a hash no joining node compares — committed-looking, binding nothing.
	if err := validateParamsPlacement(b); err != nil {
		return Reject, err
	}

	// ---- 2. P2/P3 proposer key size and proposer signature over b.Hash(). ----
	if len(b.Proposer) != ed25519.PublicKeySize {
		return Reject, ErrBadSignature
	}
	bh := b.Hash()
	if !ed25519.Verify(ed25519.PublicKey(b.Proposer), bh[:], b.ProposerSig) {
		return Reject, fmt.Errorf("%w: proposer", ErrBadSignature)
	}

	// ---- 3. P4 PROPOSER QUALIFICATION (+ P4a, the #572 attribution branches, verbatim). ----
	if out, err := v5RequireProposerQualified(v, b); out != Accept {
		return out, err
	}

	// ---- 4. P5 non-empty block — SIX clauses (M-5: IssuerKeys is the sixth). ----
	if len(b.Entries) == 0 && len(b.Revocations) == 0 && len(b.Unrevocations) == 0 &&
		len(b.BondRegs) == 0 && len(b.Slashes) == 0 && len(b.IssuerKeys) == 0 {
		return Reject, errors.New("chain: empty block")
	}

	// ---- 4. P6 takedowns. ----
	if out, err := v5ValidateTakedowns(v, b); out != Accept {
		return out, err
	}

	// ---- 5. P7 bond registrations, P8 slashes, P8b issuer keys — the node's order. ----
	if out, err := v5ValidateBondRegs(v, b); out != Accept {
		return out, err
	}
	if out, err := v5ValidateSlashes(v, b); out != Accept {
		return out, err
	}
	if out, err := v5ValidateIssuerKeys(v, b); out != Accept {
		return out, err
	}

	// ---- 4. P9 the entry loop: intra-block dup root / dup serial, then the per-entry rule. ----
	if out, err := v5ValidateEntries(v, b); out != Accept {
		return out, err
	}

	// ---- 4. P10/P11 the era version-boundary rules, BEFORE the roots predicate deliberately, so
	// the failure names the version, not a missing root. ----
	if out, err := v5ValidateEraVersions(v, b); out != Accept {
		return out, err
	}

	// ---- 6. P12 the SHARED carrier validity rule. One function, four callers. ----
	// head was read at step 1 and P1 has already bound (b.Prev, b.Height) to it, which is the
	// precondition the carrier's DERIVED signing height (b.Height-1) rides on. The chain id is
	// the view's own (class 3, BG-2) — never b's author's.
	if err := validateCarrier(b, head.ChainID); err != nil {
		return Reject, err
	}

	// ---- 8. P13a ∧ P13b — THE ONE SUBSTITUTED STEP, last in the proposal as in the node. ----
	if out, err := v.CommittedRoots(b); out != Accept {
		return out, err
	}
	return Accept, nil
}

// ValidateCommitV5 is ValidateProposalV5 followed by C1…C5: the proposer's own prepare, then the
// two quorum stacks (prepare-QC, precommit certificate), each held to the full quorum stack — the
// POL threshold IS the commit threshold (#432 certification §4).
//
// C2/C4 BUILD the signer set; C3/C5 CONSUME it. v5CollectQuorumSigs skips the author BEFORE its
// round-exactness check and drops unqualified ids AFTER the signature check. Both placements are
// load-bearing (F1, G-I, N5), so the composition calls the shared constructor and never takes
// `seen` as a parameter.
//
// It is the ONLY entry the floor box calls (the box validates COMMITTED blocks — build-plan
// certification §1.5).
func ValidateCommitV5(v StateView, b *Block) (FloorBoxOutcome, error) {
	if out, err := ValidateProposalV5(v, b); out != Accept {
		return out, err
	}
	// ---- 7. C1..C5 the two quorum stacks. ----
	if out, err := v5RequireProposerPrepare(v, b); out != Accept {
		return out, err
	}
	seenPrep, out, err := v5CollectQuorumSigs(v, b, b.PrepareQC, PhasePrepare, b.CommitRound)
	if out != Accept {
		return out, fmt.Errorf("prepare-QC: %w", err)
	}
	if out, err := v5RequireQuorumStack(v, b, seenPrep); out != Accept {
		return out, fmt.Errorf("prepare-QC: %w", err)
	}
	seen, out, err := v5CollectQuorumSigs(v, b, b.Atts, PhasePrecommit, b.CommitRound)
	if out != Accept {
		return out, err
	}
	if out, err := v5RequireQuorumStack(v, b, seen); out != Accept {
		return out, err
	}
	return Accept, nil
}

// v5VersionPartition is composition step 0 — the L1 version partition, exact equality with the
// current era, both sides Reject. Below: the sub-v5 era-3 read-set is out of this mode's scope.
// Above: an era this binary cannot validate.
func v5VersionPartition(b *Block) (FloorBoxOutcome, error) {
	if b.Version < BlockVersionWitnessable {
		return Reject, ErrNotWitnessableVersion
	}
	if b.Version > BlockVersionWitnessable {
		return Reject, ErrAboveCurrentEraVersion
	}
	return Accept, nil
}

// v5CheckBudget is BG-3 at step 0b. The zero Budget STALLS (M-4); an unlimited budget costs
// nothing; a frame budget is measured against the block's canonical encoded size — the FRAME, the
// only witnessable-surface quantity that exists before any witness is looked at. Byte-denominated
// because distinct carrier ids are free (carrier.go), so a count bounds nothing.
func v5CheckBudget(bud Budget, b *Block) error {
	if bud.Unlimited() {
		return nil // no encode on the node
	}
	return bud.Check(len(Encode(b)), "frame")
}
