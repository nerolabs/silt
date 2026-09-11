package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// THE CONFIG-IN-CONSENSUS GATE — canon rule 8 (docs/build-process.md, owner ruling 2026-09-10):
// a consensus quantity must be a function of the CHAIN, never of local config.
//
// THE INVARIANT: two honest replicas whose local Config differs in exactly ONE field must reach
// the SAME validity verdict on the SAME block. A field that can move the verdict is a
// consensus quantity wearing a flag's clothes.
//
// WHY A RUNTIME GATE AND NOT A SOURCE LINT. The property is a runtime verdict; only executing
// ValidateCommit against two differently-configured replicas can observe it. A source-text gate
// would promise a property it cannot see — the scar scripts/check_source_gates.py exists to stop.
// Deliberation: docs/thinking/2026-09-10-config-in-consensus-lint-design.md.
//
// THE THREE INSTANCES THIS GENERALISES (it checks the PRINCIPLE, not these three):
//   - #380      RequiredQuorum() read cfg.Quorum on the objective path      → fixed, and is this
//               gate's ABLATION: restore it and Quorum diverges again (2 → 3 fields).
//   - SlashesBytesCap  an invariant derived from two proposer-side flag defaults → D-SLASHCAP-ROUTE.
//   - MinBond   a validity threshold that is a bare flag                     → R-CONSENSUS-CONFIG-UNBOUND.
//
// THE CLOSED COMPLEMENT. Every chain.Config field is reached by REFLECTION and must carry a
// declaration below. There is no third state and no field can be silently absent, so a NEW
// Config field fails this gate until someone decides which class it is. That is the point:
// silt's prose already names a "consensus-critical genesis config" class (chain.go:190, :253)
// but nothing enumerated it, so membership was a human remembering to write the sentence.
//
// THE ABLATION BATTERY, RUN 2026-09-10 BEFORE THIS GATE WAS TRUSTED (docs/build-process.md; the
// four vacuous-gate scars of sessions 19-20). Verified by EXIT CODE, not by reading output — the
// first run of this battery reported four false GREENs because it executed from the wrong
// directory and the package path never resolved.
//
//	A0 baseline .................................................. GREEN
//	A1 restore the #380 defect (RequiredQuorum returns cfg.Quorum) . RED
//	A2 add an undeclared chain.Config field ....................... RED
//	A3 re-declare MinBond as classLocal (the defect class itself) .. RED
//	A4 leave a stale declaration for a removed field ............... RED
//	A5 restore everything ......................................... GREEN
//	A6 DELETE a field's perturbation entirely ..................... RED  (added after review)
//	A7 weaken the era probe to a dead value (H=2 at height 1) ..... RED  (added after review)
//	A8 restore everything ......................................... GREEN
//	A9 strip an UNGATED marker off a v5-citing declaration ........ RED  (owner merge condition)
//	A10 restore everything ........................................ GREEN
//
// A6 and A7 exist because a BLIND review broke the first version of this gate with exactly those
// two moves, and the first fix for A6 was itself insufficient — it stayed GREEN until the
// probeless assertion landed. Both are now permanent rows.
//
// A1 is the sharpest: with the fix in place Config.Quorum diverges ONLY in [legacy,
// trusted-optout] — the two trusted-deployment legs where the local count floor IS the intended
// rule. With the defect restored it diverges in all five regimes including objective, and the
// sanctionedIn pin fails.
//
// ⚠ THE SCOPE OF THAT CLAIM, CORRECTED BY BLIND REVIEW (F-1). This fixture validates a
// Version: 1 block (redteam_consensus_test.go:61), and ValidateCommit dispatches to the v5
// composition only at Version >= 5 (chain.go:3020). Measured statement coverage under THIS test
// alone: validate_v5_predicates.go 0/178, validate_v5_quorum.go 0/229, validate_v5.go 0/53,
// stateview_live_v5.go 0/64. So A1 pins the ERA-1 twin, RequiredQuorum. Restoring the same defect
// in v5RequiredQuorum (validate_v5_quorum.go:350) ALONE leaves this gate GREEN.
//
// The v5 twin is not unguarded — TestM1A3_V4V5ParityOracle covers it — but this gate does not
// cover it, and the declaration table below justifies its three most important rows by citing
// validate_v5_* files that never execute here. Those citations are the CORRECT justification for
// the classification; they are NOT evidence produced by this test. A v5 regime that REPLACES the
// era-1 one is the named residual: R-CONFIG-GATE-V5-REGIME.
//
// WHAT THIS GATE DELIBERATELY DOES NOT CLAIM (simplicity rule 7). A field that does not diverge
// in a regime that could never have exercised it has NOT been shown safe — it has been shown
// UNTESTED. So a LOCAL declaration reports DRIVEN-SAFE only when a regime that actually reads it
// ran; otherwise UNPROVEN. Calling all the quiet fields "safe" would be the decoration failure in
// a new place.

// configClass is the closed complement: every Config field is exactly one of these.
type configClass int

const (
	// classLocal: free to differ between honest replicas. If it moves a validity verdict, that
	// is the defect this gate exists to catch.
	classLocal configClass = iota
	// classConsensusCritical: must be swarm-uniform. Canon rule 8 governs HOW it is bound —
	// refuse-to-start when the invariant is locally checkable, committed/genesis-covered state
	// when it needs distributed agreement.
	classConsensusCritical
	// classNetworkIdentity: an IDENTITY PROPERTY OF THE NETWORK ITSELF. It changes NO validity
	// verdict, and it IS genesis-covered.
	//
	// THE SECOND CATEGORY, AND WHY IT NEEDED ONE (owner ruling, 2026-09-11). NetworkName is the
	// first ConsensusParams member that reaches no verdict — and "it reaches no verdict" is this
	// table's own recorded reason for EXCLUDING Archive. Shipping it as classLocal would have
	// said a committed field is free to differ; shipping it as classConsensusCritical would have
	// claimed a verdict it cannot move. Either lie makes the doctrine unfalsifiable, and the next
	// non-consensus field is then admitted by precedent rather than by argument.
	//
	// THE OWNER'S BINDING CONDITION: this category has a CLOSED COMPLEMENT TOO. "Otherwise 'it's
	// an identity field' becomes the escape hatch that admits anything, and we've traded a
	// testable doctrine for a rhetorical one." Both arms are machine-checked below, from
	// opposite sides:
	//
	//   - CHANGES NO VERDICT is MEASURED, never asserted — the field must carry a perturbation
	//     that actually ran (the probeless check), and it must diverge in ZERO regimes. Diverge
	//     anywhere and it is classConsensusCritical; declaring it here is RED.
	//   - IS GENESIS-COVERED is resolved BY REFLECTION against the real chain.ConsensusParams via
	//     configMemberships.carriedAs. Not carried and it is an exclusion; declaring it here is
	//     RED.
	//
	// So this category admits exactly the fields that ride in the genesis hash and move no
	// verdict. Such a field has precisely ONE observable effect: it partitions networks and names
	// them. That is what an identity property of the network IS, and nothing else fits through.
	classNetworkIdentity
)

// className renders a class for a failure message. A numeric class in a diagnosis makes the
// reader go look the constant up, and the constants are iota — renumbering them would silently
// re-label every message.
func className(c configClass) string {
	switch c {
	case classLocal:
		return "classLocal"
	case classConsensusCritical:
		return "classConsensusCritical"
	case classNetworkIdentity:
		return "classNetworkIdentity"
	}
	return fmt.Sprintf("configClass(%d) — UNDECLARED, add it to className", int(c))
}

// configDecl is one field's declaration. `why` is the justification a reviewer reads; `binding`
// is what actually enforces uniformity today (empty = NOTHING does, which is a tracked residual).
//
// ⚠ THE RECORD CONTRADICTION THIS FIELD CARRIED, CORRECTED 2026-09-11. Every
// consensus-critical row below carried `binding: ""` — "NOTHING BINDS IT" — while
// configMemberships, IN THIS SAME FILE, declared the same fields `carriedAs` a real
// chain.ConsensusParams member. Both cannot be true. The membership table was right: owner call
// F landed the production bind (genesis.Build requires the params, the genesis hash covers them,
// CheckConsensusParams is wired as a refuse-to-start after replay). So the gate was printing
// "UNBOUND consensus-critical (3): [Anchors MinBond MinBondBytes]" about three fields that have
// been genesis-covered since that merge.
//
// The two fields answer DIFFERENT questions and both are kept, because collapsing them would
// lose the one that matters: `carriedAs` is STRUCTURAL (is it in the genesis hash — resolved by
// reflection), `binding` is the MECHANISM (what that coverage actually enforces, and what it
// still does not). Every binding string below now names its hole as well as its cover.
type configDecl struct {
	class   configClass
	why     string
	binding string // consensus-critical only: what binds it, or "" if nothing does yet
	// readIn names the regimes that actually exercise this field. A LOCAL field is only
	// DRIVEN-SAFE if one of these regimes ran; otherwise it reports UNPROVEN.
	readIn []string
	// ungated marks a declaration whose justification cites code this gate does NOT execute.
	// The convention is scripts/check_source_gates.py's own: a structural claim must name its
	// runtime cover, or state plainly that it has none. This gate validates a Version: 1 block,
	// so every citation of a validate_v5_* file is justification, NOT evidence this test produced
	// — and the marker must travel with the DECLARATION and into the FAILURE TEXT, not sit only
	// in the file header where a reader of a failure never sees it. Owner condition on the merge
	// of this gate, 2026-09-10.
	ungated string

	// sanctionedIn names the regimes where divergence is CORRECT and intended. Config.Quorum
	// IS the count floor on the legacy and trusted-opt-out legs — that is the design. It is
	// NOT a validity term on the objective path, and divergence there is the #380 defect.
	// A consensus-critical field that diverges OUTSIDE this set is a RED assertion, which is
	// what pins the #380 fix: the ablation (return c.cfg.Quorum unconditionally from
	// RequiredQuorum) makes Quorum diverge in all five regimes and this gate FAILS.
	sanctionedIn []string
}

// genesisBind is the mechanism that binds the whole carried family, written once because it IS
// one mechanism — sixteen copies of the same sentence is sixteen places for it to decay. A row
// that has MORE to say appends; a row whose bind is weaker says so instead of using this.
//
// The hole is named because it is real and open: a genesis minted before owner call F carries
// nil Params, and CheckConsensusParams returns nil on it by design. That surviving paramless
// path is what keeps R-CONSENSUS-CONFIG-UNBOUND on the register.
const genesisBind = "genesis-covered in ConsensusParams (a divergent node computes a different genesis " +
	"and cannot join — ErrForeignGenesis) + CheckConsensusParams refuse-to-start on restart " +
	"(D-CFGBIND-MEMBERSHIP-RULE-2026-09-10). HOLE: a pre-bind genesis carries nil Params — " +
	"R-CONSENSUS-CONFIG-UNBOUND stays open for it"

// THE DECLARATION TABLE. Adding a chain.Config field without adding a row here FAILS this gate.
var configDecls = map[string]configDecl{
	"Quorum": {
		class:   classConsensusCritical,
		why:     "#380: on the objective path this is NOT a validity term — RequiredQuorum defers to the chain-derived bftThreshold(N). It survives as the proposer-side GATHER target only. On the legacy/opt-out leg it IS the count floor, and that leg is a trusted deployment.",
		binding: "chain-derived: RequiredQuorum()/v5RequiredQuorum() ignore it on the objective path (D-CONSENSUS-ARMING (20), G-380-B)",
		ungated: "R-CONFIG-GATE-V5-REGIME — v5RequiredQuorum is NOT executed here; the era-1 twin RequiredQuorum is. Restoring the #380 defect in the v5 twin alone leaves this gate green. Runtime cover for the v5 twin: TestM1A3_V4V5ParityOracle.",
		readIn:  []string{regimeLegacy, regimeTrustedOptOut, regimeObjective},
		// The #380 PIN. Legacy and trusted-opt-out are trusted deployments where the local
		// count floor is the intended rule; the objective path must be chain-derived.
		sanctionedIn: []string{regimeLegacy, regimeTrustedOptOut},
	},
	"MinBond": {
		class:   classConsensusCritical,
		why:     "Gates Reject in three v5 validity paths: proposer qualification (validate_v5_predicates.go:246-249), bond-reg admission (validate_v5_quorum.go:222) and the qualification filter (:522). Divergent values mean two honest replicas disagree on the same block — I1.",
		ungated: "R-CONFIG-GATE-V5-REGIME — those three v5 sites are NOT executed here (measured: validate_v5_predicates.go 0/178, validate_v5_quorum.go 0/229). The divergence this gate measures is on the era-1 path; the v5 citation is the classification's justification, not this test's evidence.",
		// CORRECTED 2026-09-11: this read `binding: ""` — "NOTHING BINDS IT" — while
		// configMemberships declared carriedAs "MinBond" in the same file. The bind landed with
		// owner call F; the row had not moved.
		binding: genesisBind,
		readIn:  []string{regimeObjective},
	},
	"MinBondBytes": {
		class:   classConsensusCritical,
		why:     "The anti-release floor is a second bond-reg admission threshold (validate_v5_quorum.go:225), same class and same failure as MinBond.",
		ungated: "R-CONFIG-GATE-V5-REGIME — as MinBond: validate_v5_quorum.go is not executed here.",
		binding: genesisBind, // CORRECTED 2026-09-11, as MinBond: carriedAs "MinBondBytes".
		readIn:  []string{regimeObjective},
	},
	"ByzantineQuorum": {
		class:   classConsensusCritical,
		why:     "Selects WHICH quorum rule is the validity bar (derived Byzantine threshold vs the Config.Quorum floor). Two replicas disagreeing here disagree about the rule itself, not just its input.",
		binding: genesisBind,
		readIn:  []string{regimeTrustedOptOut, regimeObjective},
	},
	"Anchors": {
		class: classConsensusCritical,
		why:   "The launch training-wheels set: a commit in the young window needs AnchorQuorum attestations from it. Named in silt's own prose as genesis config discipline (chain.go:190).",
		// CORRECTED 2026-09-11: carriedAs "Anchors" (as the canonical SORTED slice — see
		// SortedAnchors, which exists because CBOR map ordering is not a hash guarantee).
		binding: genesisBind,
		readIn:  []string{regimeYoungAnchors},
	},
	"AnchorQuorum": {
		class:   classConsensusCritical,
		why:     "The threshold on Anchors; same rule, same failure.",
		binding: genesisBind,
		readIn:  []string{regimeYoungAnchors},
	},
	"MatureValidators": {
		class:   classConsensusCritical,
		why:     "The Nakamoto coefficient that sheds the training wheels. Divergence means replicas disagree about WHICH regime the chain is in, which is a fork of the validity rule.",
		binding: genesisBind,
		readIn:  []string{regimeYoungAnchors},
	},
	"OperatorMargin": {
		class:   classConsensusCritical,
		why:     "The M discount on the C2 concentration metric that feeds Mature(); same regime-selection failure as MatureValidators.",
		binding: genesisBind,
		readIn:  []string{regimeYoungAnchors},
	},
	"EpochBlocks": {
		class:   classConsensusCritical,
		why:     "The epoch cadence. silt's own comment says it verbatim: 'Consensus-critical: every validator in a swarm must run the same value (like MinBond/Anchors — genesis config discipline)' (chain.go:189-191).",
		binding: genesisBind,
		readIn:  []string{regimeEpochs},
	},
	"RegGateActivationHeight": {
		class:   classConsensusCritical,
		why:     "The #506 R-rule pre-latch activation override. Its own comment: 'Consensus-critical genesis config, same discipline as MinBond/Anchors' (chain.go:256-257).",
		binding: genesisBind,
		readIn:  []string{regimeEpochs},
	},
	"BondTTLBlocks": {
		class:   classConsensusCritical,
		why:     "Bond standing lapses on this cadence, so it decides WHO is qualified at a height — a committed-state question, not a local one.",
		binding: genesisBind,
		readIn:  []string{regimeEpochs},
	},
	"AllowPublisher": {
		class:   classConsensusCritical,
		why:     "Permits a durable Publisher NodeID on an entry. A replica that refuses what another accepts splits on entry validity.",
		binding: genesisBind,
		readIn:  []string{regimeLegacy, regimeTrustedOptOut, regimeObjective},
	},
	"Era3ActivationHeight": {
		class: classConsensusCritical,
		why:   "The era-3 pre-latch activation override — it decides WHICH format/validity rules apply at a height. Its own comment calls it consensus-critical genesis config (chain.go:243-260). Divergence means two replicas validate the same block under different eras.",
		// CORRECTED 2026-09-11. The old string named ONLY the local New() ordering check and
		// concluded "nothing binds the VALUE across replicas" — true when written, false since
		// owner call F, and it read as the strongest claim available while carriedAs said
		// otherwise two hundred lines down.
		binding: genesisBind + ". PLUS a local New() check that Era4ActivationHeight >= " +
			"Era3ActivationHeight — canon rule 8: that binds operators, never PEERS",
		readIn: []string{regimeEpochs},
	},
	"Era4ActivationHeight": {
		class:   classConsensusCritical,
		why:     "The era-4 (v5) pre-latch activation override, read by era4Active (chain.go:3980-3985). Same failure as Era3ActivationHeight, and it is the boundary the whole D1 freeze is about.",
		binding: genesisBind + ". PLUS the same local New() ordering check (CORRECTED 2026-09-11, as Era3ActivationHeight)",
		readIn:  []string{regimeEpochs},
	},
	"BondRegHeadWindow": {
		class:   classConsensusCritical,
		why:     "Bounds how far back a bond registration may be anchored, which is an admission rule on committed content.",
		binding: genesisBind,
		readIn:  []string{regimeObjective},
	},
	"MinProposerRep": {
		class: classConsensusCritical,
		// Blind PE F-3 re-classified this, and the gate then PROVED the re-classification: with a
		// probe that actually crosses the >= boundary (1_000_001, not 1_000_000) it diverges. It
		// is NOT a safe local knob — in legacy mode it decides proposer qualification, so two
		// replicas with different values reach different verdicts. That divergence is the known
		// local-audit subjectivity objective mode exists to remove (red-team F6).
		why:          "Legacy proposer qualification against the LOCAL reputation view. Divergence here is the subjectivity objective mode removes, not a safe local knob.",
		binding:      "chain-derived in objective mode: MinBond > 0 replaces the reputation gate with committed bond (D2 / red-team F6). Nothing binds it on the legacy leg, which is a trusted deployment.",
		readIn:       []string{regimeLegacy},
		sanctionedIn: []string{regimeLegacy},
	},
	"MinAttesterRep": {
		class:        classConsensusCritical,
		why:          "Legacy attester qualification; same reasoning and same sanctioned leg as MinProposerRep.",
		binding:      "chain-derived in objective mode (see MinProposerRep).",
		readIn:       []string{regimeLegacy},
		sanctionedIn: []string{regimeLegacy},
	},
	"NetworkName": {
		class: classNetworkIdentity,
		// The contrast with Archive one row below is the whole point of the second category, so
		// it is stated here rather than left for a reader to infer: BOTH reach no verdict. Archive
		// is EXCLUDED for that reason — binding it would forbid the retention heterogeneity the
		// durability design depends on. NetworkName is CARRIED for a different reason: it is what
		// the network is called, and a name a node reads off its own flags reports a belief.
		why: "The network's canonical text name. It reaches NO validity verdict — nothing in the validation path reads it, and this gate MEASURES that (zero divergence in all five regimes). It is genesis-covered so a node reports the network it is actually serving instead of reading its own -network-name back (the eradeclared.go vacuity). The hash is the identity; the name is a label, and (*Chain).NetworkIdentity never renders one without the other.",
		// Every regime is listed because no regime can exercise it: the point of naming them all
		// is that a ZERO divergence here is a measurement across the whole driven surface, not a
		// quiet UNPROVEN in a regime that could not have read the field anyway.
		readIn: []string{regimeLegacy, regimeTrustedOptOut, regimeObjective, regimeEpochs, regimeYoungAnchors},
	},
	"Archive": {
		class:  classLocal,
		why:    "Retention policy: whether THIS node keeps full bodies. An operator's storage choice, deliberately per-node (build-immutable #8) — it must never reach a validity verdict.",
		readIn: nil,
	},
	"WSCheckpoint": {
		class: classLocal,
		// ⚠ THE "NARROWING-ONLY" CLAUSE WAS MEASURED FALSE, 2026-09-11. This row read "It narrows
		// what THIS node accepts; it must never widen it." Driven against the real predicate:
		// with WSCheckpoint UNSET, trustFloor() is 0 and validateBondRegs REFUSES a
		// heavy-proofs-shed block at height 50 (ErrPrunedAboveHorizon); with WSCheckpoint at
		// height 100, trustFloor() is 100 and the SAME block is ACCEPTED. It widens, and the
		// widening is the mechanism, not a bug: trustFloor() is max(RetentionHorizon(),
		// WSCheckpoint.Height), and trusting pruned history below your own anchor is what the
		// anchor is FOR. Cover: TestWSCheckpointWidensThePrunedTrustFloor.
		//
		// THE EXCLUSION STILL HOLDS, on its OTHER arm. It is exactly because setting the pin
		// WIDENS what this node will trust unverified that the pin must be the operator's OWN —
		// a checkpoint every operator took from the same place is not an independent anchor, it
		// is one party's say-so wearing five signatures. That argument never needed
		// "narrowing-only", which is why removing the false clause costs the exclusion nothing.
		why: "Weak-subjectivity pin. Deliberately per-operator — it is the operator's own trust anchor, and silt is weakly subjective by design (TENETS Part 0). Setting it WIDENS what this node trusts without re-verification (it raises trustFloor(), the pruned-tolerance anchor), which is precisely why it must be independently chosen per operator.",
		// readIn STAYS nil, deliberately. None of the five regimes drives a heavy-proofs-shed
		// block, so none of them reads this field, and the gate correctly reports it UNPROVEN.
		// Naming a regime here to move it out of UNPROVEN would be the dead-probe failure blind
		// PE F-3 caught twice: a DRIVEN-SAFE that no driving produced. The widening above is
		// measured by a separate driven test rather than faked here.
		readIn: nil,
	},
	"LivenessRecoveryHeight": {
		class: classConsensusCritical,
		// Blind PE F-5: chain.go:236-237 calls this "consensus-coordination config" in as many
		// words. Declaring it classLocal re-adopted inside a declaration table exactly the
		// reading a prior PE ruling rejected.
		why:     "The #535 recovery directive re-bases one epoch boundary against the LIVE qualified set. Every honest operator must set the SAME height or replicas validate that boundary differently.",
		binding: "partial and LOCAL ONLY: cmd/silt refuses a non-boundary height and announces loudly when armed. Canon rule 8: a local assertion cannot enforce a distributed agreement, so nothing binds the VALUE across replicas.",
		readIn:  nil,
	},
}

// THE MEMBERSHIP RULE, chain.Config half (owner, 2026-09-10): every field that can change a
// validity verdict is BOUND TO THE CHAIN, or is EXPLICITLY EXCLUDED WITH A RECORDED REASON.
//
// The owner rejected both "five fields" and "17 fields" as the thing to ratify. The RULE is
// ratified and the field count is its OUTPUT — so this table names, per field, the
// ConsensusParams field that binds it, or the reason it is not bound. G-CFGBIND-1 resolves every
// carriedAs against the REAL struct by reflection, which is what stops it decaying into prose: a
// sentence describing what a gate checks decays exactly like a cited test name.
//
// It is a SEPARATE map from configDecls on purpose. configDecls answers "what class is this
// field"; this answers "what makes it uniform". Keeping them apart kept a 200-line declaration
// table from being rewritten wholesale, and the gate asserts both are complete over chain.Config,
// so the two cannot drift apart.
//
// THE COUNT FALLS OUT: 16 carried here + 2 carried by core/node's table (the residue named below)
// = the 18 fields of chain.ConsensusParams. Nobody has to remember 18 — it moved from 17 to 18 on
// 2026-09-11 when NetworkName landed, and the only reason that number appears here at all is to
// show it is an OUTPUT. The gate below asserts the bijection by reflection; nothing keys on the
// arithmetic, so this sentence is a reader's aid and never the check.
type configMembership struct {
	// carriedAs names the chain.ConsensusParams field that binds this one, resolved by reflection.
	carriedAs string
	// reasonNotCarried is the recorded reason it is not. These five ARE the owner's ratified
	// exclusions, and each is a DIFFERENT reason — that is why they are five strings and not one
	// "excluded" boolean.
	reasonNotCarried string
}

var configMemberships = map[string]configMembership{
	"Quorum":                  {carriedAs: "Quorum"},
	"ByzantineQuorum":         {carriedAs: "ByzantineQuorum"},
	"Anchors":                 {carriedAs: "Anchors"},
	"AnchorQuorum":            {carriedAs: "AnchorQuorum"},
	"MatureValidators":        {carriedAs: "MatureValidators"},
	"OperatorMargin":          {carriedAs: "OperatorMargin"},
	"MinBond":                 {carriedAs: "MinBond"},
	"MinBondBytes":            {carriedAs: "MinBondBytes"},
	"BondTTLBlocks":           {carriedAs: "BondTTLBlocks"},
	"BondRegHeadWindow":       {carriedAs: "BondRegHeadWindow"},
	"EpochBlocks":             {carriedAs: "EpochBlocks"},
	"RegGateActivationHeight": {carriedAs: "RegGateActivationHeight"},
	"Era3ActivationHeight":    {carriedAs: "Era3ActivationHeight"},
	"Era4ActivationHeight":    {carriedAs: "Era4ActivationHeight"},
	"AllowPublisher":          {carriedAs: "AllowPublisher"},

	// ---- CATEGORY (b): AN IDENTITY PROPERTY OF THE NETWORK. Carried, and it moves no verdict. ----
	"NetworkName": {carriedAs: "NetworkName"},

	// ---- THE FIVE RATIFIED EXCLUSIONS. Read them as five distinct arguments, not one policy. ----
	"Archive": {reasonNotCarried: "RETENTION ONLY — it reaches no verdict. Whether THIS node keeps full bodies is " +
		"an operator's storage choice and is deliberately per-node (build-immutable #8). Binding it would forbid " +
		"the heterogeneity the durability design depends on."},
	// ⚠ CORRECTED 2026-09-11: the "narrows and can never widen it, so divergence is safe" clause
	// was MEASURED FALSE — it widens (see the configDecls row). The exclusion stands on the arm
	// that was always doing the work, and that arm is now the whole reason.
	"WSCheckpoint": {reasonNotCarried: "SHARING IT WOULD DESTROY WEAK SUBJECTIVITY. It is the operator's OWN trust " +
		"anchor, and silt is weakly subjective by design (TENETS Part 0). It MUST vary per node, because a " +
		"checkpoint every operator got from the same place is not an independent anchor at all — and since setting " +
		"it WIDENS what this node trusts unverified (it raises the pruned-tolerance trust floor), a shared pin would " +
		"hand one party the power to widen every node at once. Divergence here is the design, not a tolerated cost."},
	"MinProposerRep": {reasonNotCarried: "BINDING IS INEFFECTIVE. The divergent term is the local reputation VIEW, " +
		"not the threshold: two nodes agreeing on the number still disagree on the verdict because they are " +
		"comparing it against different inputs. Committing it would look like a fix and change nothing. The real " +
		"close is objective mode, where MinBond > 0 replaces the reputation gate with committed bond (D2 / F6)."},
	"MinAttesterRep": {reasonNotCarried: "Same argument as MinProposerRep, same leg: the input is the local view."},
	"LivenessRecoveryHeight": {reasonNotCarried: "STRUCTURALLY UNBINDABLE. It is set AFTER launch, on a chain that " +
		"by construction cannot commit it — genesis is already written by the time an operator knows the outage " +
		"happened. This is not a decision deferred; it is a shape the mechanism cannot hold " +
		"(R-LIVENESS-RECOVERY-UNBOUND, open and honestly so)."},
}

// theTwoNodeSideParams is the residue: the chain.ConsensusParams fields this table does NOT claim,
// because they live in core/node.Config. core/node's gate
// (TestNodeConsensusVerdictIsNotAFunctionOfLocalConfig) claims exactly these, and asserts the same
// list from its side. Naming the residue in both places is what makes the two tables a checked
// bijection onto ConsensusParams without either package importing the other's test fixtures.
var theTwoNodeSideParams = []string{"BondLabelSamples", "BondVDFDelay"}

// The driven regimes. A LOCAL field is DRIVEN-SAFE only if a regime naming it actually ran.
const (
	// regimeLegacy is the TRUE legacy path: MinBond == 0, so objective() is false and
	// qualification is reputation-gated. NOTE the trap this gate fell into first: a config with
	// MinBond > 0 and a wired verifier is OBJECTIVE even with ByzantineQuorum off — that is the
	// trusted opt-out leg (c), not legacy — so the reputation fields were reporting DRIVEN-SAFE
	// from a regime that never read them.
	regimeLegacy        = "legacy"         // MinBond == 0: reputation-gated qualification
	regimeTrustedOptOut = "trusted-optout" // objective, ByzantineQuorum off (leg (c))
	regimeObjective     = "objective"      // objective, ByzantineQuorum on
	regimeYoungAnchors  = "young-anchors"  // launch window: anchors + maturity threshold
	regimeEpochs        = "epochs"         // epoch cadence enabled
)

// perturbations returns a mutated copy of cfg for one field, or ok=false when this gate does not
// yet know how to move that field. An unknown perturbation is NOT a pass — it keeps the field
// UNPROVEN.
func perturbConfig(cfg Config, field string) (Config, bool) {
	switch field {
	case "MinProposerRep":
		// Must EXCEED the fixture's reputation (1_000_000), not equal it: the gate reads
		// `rep >= MinProposerRep`, so landing ON the boundary cannot reject and the probe is
		// dead. Blind PE F-3 — both DRIVEN-SAFE rows were green for exactly this reason.
		cfg.MinProposerRep = 1_000_001
	case "MinAttesterRep":
		cfg.MinAttesterRep = 1_000_001
	case "Quorum":
		cfg.Quorum++
	case "ByzantineQuorum":
		cfg.ByzantineQuorum = !cfg.ByzantineQuorum
	case "Anchors":
		cfg.Anchors = map[ports.NodeID]bool{ports.HashBytes([]byte("not-a-real-anchor")): true}
	case "AnchorQuorum":
		cfg.AnchorQuorum += 2
	case "MatureValidators":
		cfg.MatureValidators += 9
	case "OperatorMargin":
		cfg.OperatorMargin += 4
	case "AllowPublisher":
		cfg.AllowPublisher = !cfg.AllowPublisher
	case "MinBond":
		cfg.MinBond = cfg.MinBond*2 + 1
	case "MinBondBytes":
		// Must CROSS the fixture's bond size (twoMiB) or the perturbation proves nothing —
		// a sub-threshold nudge is an undriven probe wearing a driven probe's clothes.
		cfg.MinBondBytes = twoMiB * 2
	case "BondTTLBlocks":
		cfg.BondTTLBlocks = 1
	case "BondRegHeadWindow":
		cfg.BondRegHeadWindow = 1
	case "Archive":
		cfg.Archive = !cfg.Archive
	case "EpochBlocks":
		cfg.EpochBlocks = 8
	case "RegGateActivationHeight":
		cfg.RegGateActivationHeight = 1
	case "Era3ActivationHeight":
		// The fixture block is height 1, and activation is `h >= H`. H = 2 is dead by one
		// height; H = 1 actually moves the era. Blind PE F-4.
		cfg.Era3ActivationHeight = 1
	case "Era4ActivationHeight":
		cfg.Era4ActivationHeight = 1
	case "NetworkName":
		// The perturbation must be a name the baseline does NOT carry. The baseline regimes leave
		// it empty, so any non-empty string crosses. A probe that left it empty would be the dead
		// probe blind PE F-3 caught twice — it would report zero divergence without ever having
		// moved the field, and this category's whole complement rests on that zero being MEASURED.
		cfg.NetworkName = "a DIFFERENT network's name"
	case "WSCheckpoint":
		cfg.WSCheckpoint = WSCheckpoint{Height: 1, Hash: ports.HashBytes([]byte("elsewhere"))}
	case "LivenessRecoveryHeight":
		cfg.LivenessRecoveryHeight = 8
	default:
		return cfg, false
	}
	return cfg, true
}

// TestConsensusVerdictIsNotAFunctionOfLocalConfig is the gate. See the file header.
func TestConsensusVerdictIsNotAFunctionOfLocalConfig(t *testing.T) {
	// ---- the closed complement: every Config field must be declared ----
	var undeclared []string
	ct := reflect.TypeOf(Config{})
	fields := make([]string, 0, ct.NumField())
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		fields = append(fields, name)
		if _, ok := configDecls[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	if len(undeclared) > 0 {
		t.Fatalf("chain.Config has %d UNDECLARED field(s): %v\n"+
			"Canon rule 8 (docs/build-process.md): a consensus quantity must be a function of the CHAIN.\n"+
			"Every Config field must be declared classLocal or classConsensusCritical in configDecls,\n"+
			"because the class silt names in prose ('consensus-critical genesis config', chain.go:190)\n"+
			"was never enumerated — which is how MinBond stayed a bare flag through three audits.\n"+
			"Decide the class and write the justification; do not delete this check.", len(undeclared), undeclared)
	}
	// A stale declaration is as bad as a missing one.
	for name := range configDecls {
		if _, ok := ct.FieldByName(name); !ok {
			t.Fatalf("configDecls declares %q, which is no longer a chain.Config field — remove the row", name)
		}
	}

	// ---- THE MEMBERSHIP RULE: bound to the chain, or excluded with a recorded reason ----
	// This is the owner's rule made machine-checkable. It runs over EVERY chain.Config field, not
	// only the consensus-critical ones, because all five ratified exclusions were adjudicated
	// individually and two of them (Archive, WSCheckpoint) are classLocal. A local classification
	// is not by itself a recorded reason for leaving a field out of the committed set.
	params := reflect.TypeOf(ConsensusParams{})
	claimed := map[string]string{} // ConsensusParams field -> the Config field claiming it
	var unruled []string
	for _, f := range fields {
		m, ok := configMemberships[f]
		if !ok {
			unruled = append(unruled, f)
			continue
		}
		switch {
		case m.carriedAs != "" && m.reasonNotCarried != "":
			t.Fatalf("%s is declared BOTH carried (as %q) and not-carried. A field is bound to the chain or it is "+
				"excluded, never both, or a reader cannot tell which claim is load-bearing.", f, m.carriedAs)
		case m.carriedAs == "" && m.reasonNotCarried == "":
			unruled = append(unruled, f)
		case m.carriedAs != "":
			// THE STRUCTURAL BIND. A pin on prose is not a pin (blind PE F-4 broke the earlier
			// binding pin for exactly this reason: it keyed on free text). Resolve the named field
			// against the REAL struct, so removing or renaming it turns this gate red.
			if _, ok := params.FieldByName(m.carriedAs); !ok {
				t.Fatalf("%s declares carriedAs=%q, but chain.ConsensusParams has NO such field. Either the bind was "+
					"never written, or the field was renamed/removed and this knob is silently unbound again.", f, m.carriedAs)
			}
			if prev, dup := claimed[m.carriedAs]; dup {
				t.Fatalf("chain.ConsensusParams.%s is claimed by BOTH %s and %s. One committed field cannot bind two "+
					"local knobs — one of them is unbound and the table hides which.", m.carriedAs, prev, f)
			}
			claimed[m.carriedAs] = f
		}
		// ---- CATEGORY (b)'s CLOSED COMPLEMENT, arm 2 of 2: IT MUST BE GENESIS-COVERED. ----
		// The other arm (it must move no verdict) is measured in the runtime half below. This one
		// is structural and resolves against the REAL ConsensusParams through carriedAs, which the
		// case above already checked exists. A field that is not carried is an EXCLUSION with a
		// recorded reason — category (c) — and calling it the network's identity while leaving it
		// out of the genesis hash is the contradiction this arm refuses: a network cannot be
		// identified by something its genesis does not commit.
		if configDecls[f].class == classNetworkIdentity && m.carriedAs == "" {
			t.Fatalf("%s is declared %s but is NOT carried in chain.ConsensusParams (reasonNotCarried: %q).\n"+
				"An IDENTITY PROPERTY OF THE NETWORK must be GENESIS-COVERED — that is half of the category's\n"+
				"closed complement (owner ruling, 2026-09-11). A field the genesis does not commit cannot identify\n"+
				"the network: two nodes differing on it compute the SAME genesis hash and join each other happily,\n"+
				"which is the opposite of what a name is for. Carry it, or re-declare it %s / %s with its reason.",
				f, className(classNetworkIdentity), m.reasonNotCarried, className(classLocal), className(classConsensusCritical))
		}
	}
	sort.Strings(unruled)
	if len(unruled) > 0 {
		t.Fatalf("%d chain.Config field(s) have NO membership ruling: %v\n"+
			"THE MEMBERSHIP RULE (owner, 2026-09-10): every field that can change a validity verdict is BOUND TO THE\n"+
			"CHAIN, or is EXPLICITLY EXCLUDED WITH A RECORDED REASON. The owner rejected both \"five fields\" and\n"+
			"\"17 fields\" as the thing to ratify — the RULE is what is ratified, and the count is its OUTPUT.\n"+
			"Add a configMemberships row naming the ConsensusParams field that binds it, or the reason it is not bound.",
			len(unruled), unruled)
	}
	// And no ruling may survive its field.
	for name := range configMemberships {
		if _, ok := ct.FieldByName(name); !ok {
			t.Fatalf("configMemberships rules on %q, which is no longer a chain.Config field — remove the row", name)
		}
	}

	// THE BIJECTION. Every ConsensusParams field must be claimed exactly once, here or by
	// core/node's table. A field claimed by neither is COMMITTED BUT UNOWNED: it rides in the
	// genesis hash while no local knob is known to feed it, which is how a committed value and the
	// config it is supposed to bind quietly stop being the same thing.
	var unclaimed []string
	nodeSide := map[string]bool{}
	for _, p := range theTwoNodeSideParams {
		nodeSide[p] = true
	}
	for i := 0; i < params.NumField(); i++ {
		name := params.Field(i).Name
		if claimed[name] == "" && !nodeSide[name] {
			unclaimed = append(unclaimed, name)
		}
	}
	sort.Strings(unclaimed)
	if len(unclaimed) > 0 {
		t.Fatalf("%d chain.ConsensusParams field(s) are claimed by NO membership table: %v\n"+
			"Every committed field must trace to the local knob it binds — here for chain.Config, or in core/node's\n"+
			"gate for the node-side residue %v. A committed-but-unowned field is hash-covered decoration: it moves\n"+
			"the genesis hash while binding nothing an operator can actually set.",
			len(unclaimed), unclaimed, theTwoNodeSideParams)
	}
	// The residue must also not be claimed HERE, or it is double-counted across the two tables.
	for _, p := range theTwoNodeSideParams {
		if by := claimed[p]; by != "" {
			t.Fatalf("chain.ConsensusParams.%s is named as the node-side residue but is ALSO claimed here by %s. "+
				"core/node's gate claims it too, so the membership is double-counted and one of the two knobs is "+
				"unbound while both tables read as complete.", p, by)
		}
	}

	// ---- the runtime half: perturb one field, re-run the REAL validity path ----
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4), key(5)}

	// rep is the LOCAL reputation view. It is deliberately generous so the legacy regime ACCEPTS
	// at baseline: a regime whose baseline is REJECT cannot detect divergence, because every
	// perturbation rejects too (this gate's young-anchors regime failed exactly that way first).
	rep := func(ports.NodeID) int64 { return 1_000_000 }
	mk := func(cfg Config) *Chain {
		c := New(cfg, rep)
		if cfg.MinBond > 0 {
			c.SetBondVerifier(objectiveVerify)
		}
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = append(g.BondRegs, bondReg(prop, twoMiB, ports.Hash{}))
		for _, v := range vals {
			g.BondRegs = append(g.BondRegs, bondReg(v, twoMiB, ports.Hash{}))
		}
		Sign(g, prop)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		return c
	}

	// The anchor set must include the PROPOSER: the young-network rule is anchor-only proposal
	// (#402), so a non-anchor proposer makes the baseline REJECT and the whole regime vacuous.
	anchorSet := map[ports.NodeID]bool{
		ports.HashBytes(prop.Public().(ed25519.PublicKey)): true,
	}
	for _, v := range vals {
		anchorSet[ports.HashBytes(v.Public().(ed25519.PublicKey))] = true
	}

	regimes := []struct {
		name string
		cfg  Config
	}{
		{regimeLegacy, Config{Quorum: 3}}, // MinBond 0 => objective() false => reputation-gated
		{regimeTrustedOptOut, Config{Quorum: 3, MinBond: 1 << 20}},
		{regimeObjective, Config{Quorum: 3, MinBond: 1 << 20, ByzantineQuorum: true}},
		{regimeEpochs, Config{Quorum: 3, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4, BondTTLBlocks: 64}},
		{regimeYoungAnchors, Config{Quorum: 3, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchorSet, AnchorQuorum: 2, MatureValidators: 3, OperatorMargin: 1}},
	}

	// The block under test is fixed across every replica: only the CONFIG moves.
	gen := mk(regimes[1].cfg)
	head, _ := gen.Head()
	blk := attestedFork(prop, vals, head, entry(1), 3)

	verdict := func(cfg Config) string {
		if err := mk(cfg).ValidateCommit(blk); err != nil {
			return "REJECT: " + err.Error()
		}
		return "ACCEPT"
	}

	ranRegimes := map[string]bool{}
	for _, rg := range regimes {
		ranRegimes[rg.name] = true
	}

	// Divergence is tracked PER REGIME. Lumping regimes together loses the fact that matters:
	// Config.Quorum diverging on the LEGACY leg is the sanctioned trusted-deployment behaviour,
	// while diverging on the OBJECTIVE leg is the #380 defect. Same field, opposite verdicts.
	type result struct {
		divergedIn map[string]bool
		// probedIn records regimes where a perturbation was ACTUALLY APPLIED to this field.
		// Blind PE F-2: the earlier version set `driven` from the regime LIST, so it reduced to
		// len(readIn) > 0 — deleting a field's perturbation entirely still reported DRIVEN-SAFE.
		// "The regime ran" is not "the field was exercised."
		probedIn map[string]bool
	}
	results := map[string]*result{}
	for _, f := range fields {
		results[f] = &result{divergedIn: map[string]bool{}, probedIn: map[string]bool{}}
	}

	for _, rg := range regimes {
		ref := verdict(rg.cfg)
		t.Logf("regime %-14s baseline = %s", rg.name, ref)
		for _, f := range fields {
			mutated, ok := perturbConfig(rg.cfg, f)
			if !ok {
				continue
			}
			results[f].probedIn[rg.name] = true
			if verdict(mutated) != ref {
				results[f].divergedIn[rg.name] = true
			}
		}
	}
	// A field is DRIVEN only where a probe actually ran AND the regime is one that reads it.
	driven := map[string]bool{}
	for _, f := range fields {
		for _, r := range configDecls[f].readIn {
			if ranRegimes[r] && results[f].probedIn[r] {
				driven[f] = true
			}
		}
	}

	// ---- the verdict, and the three states rule 7 demands ----
	var violations, divergingCritical, drivenSafe, unproven, unbound, netIdentity []string
	for _, f := range fields {
		d := configDecls[f]
		r := results[f]
		where := make([]string, 0, len(r.divergedIn))
		for rn := range r.divergedIn {
			where = append(where, rn)
		}
		sort.Strings(where)
		switch {
		// ---- CATEGORY (b)'s CLOSED COMPLEMENT, arm 1 of 2: IT MUST MOVE NO VERDICT, MEASURED. ----
		// This is the arm that stops "it's an identity field" from admitting anything. A field
		// that diverges anywhere is category (a) and must be re-declared with its binding; the
		// measurement decides, not the declaration. (Arm 2 — it must be genesis-covered — runs in
		// the membership block above, where carriedAs is resolved against the real struct.)
		case d.class == classNetworkIdentity && len(where) > 0:
			violations = append(violations, fmt.Sprintf(
				"%s is declared %s — an IDENTITY PROPERTY OF THE NETWORK, which the owner's 2026-09-11 ruling "+
					"admits ONLY for a field that changes no validity verdict. It CHANGES THE VERDICT in regime(s) "+
					"%v. That makes it category (a): bind it to the chain and re-declare it %s with the binding "+
					"that makes it swarm-uniform. The second category has a closed complement precisely so this "+
					"cannot be argued past — %s",
				f, className(d.class), where, className(classConsensusCritical), d.why))
		case d.class == classNetworkIdentity:
			// A probe that never ran cannot support "measured zero". The global probeless check
			// below catches a missing perturbation; this catches the subtler case where the field
			// declares regimes that did not run, so nothing exercised it here either.
			if !driven[f] {
				violations = append(violations, fmt.Sprintf(
					"%s is declared %s, but NO driven regime exercised it, so its zero divergence is UNTESTED "+
						"rather than measured (simplicity rule 7). This category's complement rests on the zero "+
						"being a measurement. Name a regime in readIn that actually runs, and give it a "+
						"perturbation that crosses.", f, className(d.class)))
				break
			}
			netIdentity = append(netIdentity, f)
		case d.class == classLocal && len(where) > 0:
			violations = append(violations, fmt.Sprintf(
				"%s is declared LOCAL but CHANGES the validity verdict in regime(s) %v — %s", f, where, d.why))
		case d.class == classConsensusCritical && len(where) > 0:
			bind := d.binding
			if bind == "" {
				bind = "NOTHING BINDS IT — R-CONSENSUS-CONFIG-UNBOUND"
				unbound = append(unbound, f)
			}
			// A field whose binding is supposed to cover a regime must NOT diverge there.
			sanctioned := map[string]bool{}
			for _, r := range d.sanctionedIn {
				sanctioned[r] = true
			}
			if len(d.sanctionedIn) > 0 {
				for _, rn := range where {
					if !sanctioned[rn] {
						violations = append(violations, fmt.Sprintf(
							"%s diverges in regime %q, which its binding is supposed to cover. Binding: %s",
							f, rn, d.binding))
					}
				}
			}
			line := fmt.Sprintf("%s in %v [%s]", f, where, bind)
			if d.ungated != "" {
				line += " (UNGATED: " + d.ungated + ")"
			}
			divergingCritical = append(divergingCritical, line)
		case d.class == classLocal && driven[f]:
			drivenSafe = append(drivenSafe, f)
		default:
			unproven = append(unproven, f)
		}
	}
	sort.Strings(violations)
	sort.Strings(divergingCritical)
	sort.Strings(drivenSafe)
	sort.Strings(unproven)

	sort.Strings(netIdentity)
	t.Logf("CONSENSUS-CRITICAL and measured diverging (%d): %v", len(divergingCritical), divergingCritical)
	t.Logf("LOCAL and DRIVEN-SAFE (%d): %v", len(drivenSafe), drivenSafe)
	// Category (b), reported with BOTH arms named so the line is not just a list: each of these
	// was measured to move no verdict AND resolved against the real ConsensusParams.
	t.Logf("NETWORK IDENTITY — genesis-covered AND measured to move no verdict (%d): %v", len(netIdentity), netIdentity)
	// UNPROVEN is reported, never asserted safe (simplicity rule 7): a field that does not diverge
	// in a regime that could not have exercised it is UNTESTED, not safe.
	t.Logf("UNPROVEN — no driven regime exercises these yet, NOT a safety claim (%d): %v", len(unproven), unproven)

	if len(violations) > 0 {
		t.Fatalf("canon rule 8 VIOLATED — %d finding(s):\n  %v\n"+
			"A consensus quantity must be a function of the CHAIN. Either bind it to the chain, or\n"+
			"re-declare the field with the binding that actually makes it swarm-uniform.",
			len(violations), violations)
	}

	// EVERY v5 CITATION MUST NAME ITS COVER. scripts/check_source_gates.py's rule, applied to this
	// gate's own declaration table: a justification that cites code this test does not execute is
	// a structural claim, and it must say so or name the test that does observe it. Without this,
	// a future reader sees `validate_v5_quorum.go:222` in a passing gate's table and reasonably
	// concludes the gate exercised it.
	for _, f := range fields {
		d := configDecls[f]
		if !strings.Contains(d.why+d.binding, "validate_v5_") && !strings.Contains(d.binding, "v5RequiredQuorum") {
			continue
		}
		if d.ungated == "" {
			t.Fatalf("%s cites a validate_v5_* site in its justification but carries no `ungated` marker.\n"+
				"This gate validates a Version: 1 block and does not execute the v5 composition, so such a\n"+
				"citation is JUSTIFICATION, not evidence this test produced. Name the residual and the\n"+
				"runtime cover (scripts/check_source_gates.py's convention), or drop the citation.", f)
		}
	}

	// EVERY FIELD MUST HAVE A PROBE. A field perturbConfig cannot move is a field this gate
	// CANNOT SPEAK ABOUT — and the earlier version said so only by silently reporting UNPROVEN
	// forever. Deleting a probe for a field that happens not to diverge was invisible (the F-2
	// ablation, A6, stayed GREEN even after the fix). Make it loud instead: a missing probe is a
	// deliberate declaration, not an oversight.
	var probeless []string
	for _, f := range fields {
		if len(results[f].probedIn) == 0 {
			probeless = append(probeless, f)
		}
	}
	sort.Strings(probeless)
	if len(probeless) > 0 {
		t.Fatalf("%d chain.Config field(s) have NO perturbation in perturbConfig: %v\n"+
			"This gate cannot observe them at all, so it must not imply anything about them.\n"+
			"Add a probe that CROSSES the threshold the field is read against — a nudge that\n"+
			"lands on a >= boundary is a dead probe wearing a live one's clothes (blind PE F-3).",
			len(probeless), probeless)
	}

	// THE DIVERGENCE-MAP PIN. Blind PE F-4 broke the earlier pin: it keyed on `binding == ""`,
	// free text, so a field carrying non-empty prose that ITSELF admitted nothing binds it slipped
	// through — and both era-activation fields did exactly that while diverging in all five
	// regimes. A pin on prose is not a pin.
	//
	// This pins the MEASUREMENT instead: the full field -> regimes map of observed divergence.
	// Any change — a new field diverging, an existing one diverging somewhere new, or a binding
	// landing and removing one — fails here and forces a decision. Nothing about it can be
	// satisfied by editing a comment.
	//
	// WHAT TURNS THE UNBOUND ROWS INTO A HARD FAILURE: when R-CONSENSUS-CONFIG-UNBOUND closes
	// (the genesis-config family bound to committed or genesis-covered state, canon rule 8),
	// those rows leave this map and the residual closes with them.
	got := map[string][]string{}
	for _, f := range fields {
		if len(results[f].divergedIn) == 0 {
			continue
		}
		where := make([]string, 0, len(results[f].divergedIn))
		for rn := range results[f].divergedIn {
			where = append(where, rn)
		}
		sort.Strings(where)
		got[f] = where
	}
	wantDivergence := map[string][]string{
		// Sanctioned: the legacy leg is a trusted deployment whose local audit view IS the rule.
		"MinProposerRep": {regimeLegacy},
		"MinAttesterRep": {regimeLegacy},
		"Quorum":         {regimeLegacy, regimeTrustedOptOut},
		// UNBOUND — R-CONSENSUS-CONFIG-UNBOUND. Nothing makes these swarm-uniform.
		"Anchors":              {regimeYoungAnchors},
		"MinBond":              {regimeEpochs, regimeObjective, regimeTrustedOptOut},
		"MinBondBytes":         {regimeEpochs, regimeObjective, regimeTrustedOptOut},
		"Era3ActivationHeight": {regimeEpochs, regimeLegacy, regimeObjective, regimeTrustedOptOut, regimeYoungAnchors},
		"Era4ActivationHeight": {regimeEpochs, regimeLegacy, regimeObjective, regimeTrustedOptOut, regimeYoungAnchors},
	}
	if !reflect.DeepEqual(got, wantDivergence) {
		t.Fatalf("the CONFIG DIVERGENCE MAP changed.\n  got:  %v\n  want: %v\n"+
			"A field that newly moves a validity verdict is a new instance of the #380 class:\n"+
			"bind it to the CHAIN (canon rule 8, docs/build-process.md) and route it as a\n"+
			"consensus-rule change. A field that STOPPED diverging means its binding landed —\n"+
			"update this map, and when the unbound rows are gone close R-CONSENSUS-CONFIG-UNBOUND.",
			got, wantDivergence)
	}
	// CORRECTED 2026-09-11. This line printed "UNBOUND consensus-critical (3): [Anchors MinBond
	// MinBondBytes]" for three fields that owner call F had already genesis-covered — the gate's
	// loudest output was its stalest claim, because `binding` is prose and nothing re-derived it.
	//
	// The MECHANISM is unchanged and still asserts nothing: a consensus-critical field that
	// diverges with an EMPTY binding lands here, which is what makes a NEW unbound field visible
	// the moment it appears. The set is now empty, so the line reports that instead of a fiction.
	// R-CONSENSUS-CONFIG-UNBOUND stays OPEN regardless — for the surviving paramless genesis, a
	// hole no `binding` string on any row can close.
	if len(unbound) == 0 {
		t.Logf("UNBOUND consensus-critical (0) — every diverging consensus-critical field names a " +
			"binding. R-CONSENSUS-CONFIG-UNBOUND stays OPEN for the paramless-genesis hole.")
	} else {
		t.Logf("UNBOUND consensus-critical (%d, R-CONSENSUS-CONFIG-UNBOUND open): %v", len(unbound), unbound)
	}
}

// TestWSCheckpointWidensThePrunedTrustFloor is the driven cover for the corrected WSCheckpoint
// reason string, and it exists because the claim it replaces was UNVERIFIABLE prose sitting in a
// passing gate.
//
// THE CLAIM THAT WAS FALSE: "It narrows what THIS node accepts; it must never widen it." Setting
// the pin raises trustFloor() — max(RetentionHorizon(), WSCheckpoint.Height) — and validateBondRegs
// TRUSTS a heavy-proofs-shed block strictly below that floor. So the same block flips REJECT ->
// ACCEPT when the operator sets a checkpoint above it. That is the mechanism working as designed;
// the record describing it was the defect.
//
// WHY THIS IS A SEPARATE TEST AND NOT A regimes ROW. None of the five config-divergence regimes
// drives a proofs-shed block, so the field is honestly UNPROVEN there and its readIn stays nil.
// Adding a regime to make the row look driven is the dead-probe failure blind PE F-3 caught twice.
// This measures the one thing the reason string asserts, directly.
//
// ABLATIONS, 2026-09-11, by EXIT CODE, each diffed against the pristine file first:
//
//	W0 baseline ......................................... GREEN  exit 0
//	W1 pin the checkpoint BELOW the block (100 -> 40) ... RED    exit 1  (floor assertion)
//	W2 revert ........................................... GREEN  exit 0
//	W3 move the block ABOVE the floor (50 -> 150) ....... RED    exit 1  (the WIDENING assertion)
//	W4 revert ........................................... GREEN  exit 0
//
// W3 is the one that matters: it is the only ablation that can distinguish "the checkpoint
// widened the floor" from "the block happened to validate". W1 alone would pass a test that
// never exercised the widening at all.
func TestWSCheckpointWidensThePrunedTrustFloor(t *testing.T) {
	mk := func(cp WSCheckpoint) *Chain {
		c := New(Config{Quorum: 1, MinBond: 1 << 20, WSCheckpoint: cp}, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		return c
	}
	const h = 50
	b := prunedRegBlockAt(h)

	unset := mk(WSCheckpoint{})
	if got := unset.trustFloor(); got != 0 {
		t.Fatalf("with no checkpoint and no finality the trust floor must be 0, got %d — the probe "+
			"is not in the regime it claims to measure", got)
	}
	if err := unset.validateBondRegs(&b); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("baseline must REJECT: an unset checkpoint leaves floor 0, so a proofs-shed block "+
			"at height %d is at/above it. got %v", h, err)
	}

	set := mk(WSCheckpoint{Height: 100, Hash: ports.HashBytes([]byte("this operator's own anchor"))})
	if got := set.trustFloor(); got != 100 {
		t.Fatalf("the checkpoint must raise the trust floor to 100, got %d", got)
	}
	if err := set.validateBondRegs(&b); err != nil {
		t.Fatalf("SETTING the checkpoint must WIDEN: the same block is now strictly below the floor "+
			"and its space-time re-verify is skipped. got %v\n\n"+
			"If this now REJECTS, the widening is gone and the WSCheckpoint reason string in "+
			"configDecls/configMemberships is wrong AGAIN, in the other direction.", err)
	}
}
