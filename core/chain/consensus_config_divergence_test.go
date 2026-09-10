package chain

import (
	"crypto/ed25519"
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
)

// configDecl is one field's declaration. `why` is the justification a reviewer reads; `binding`
// is what actually enforces uniformity today (empty = NOTHING does, which is a tracked residual).
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
		binding: "", // NOTHING. R-CONSENSUS-CONFIG-UNBOUND.
		readIn:  []string{regimeObjective},
	},
	"MinBondBytes": {
		class:   classConsensusCritical,
		why:     "The anti-release floor is a second bond-reg admission threshold (validate_v5_quorum.go:225), same class and same failure as MinBond.",
		ungated: "R-CONFIG-GATE-V5-REGIME — as MinBond: validate_v5_quorum.go is not executed here.",
		binding: "", // NOTHING. R-CONSENSUS-CONFIG-UNBOUND.
		readIn:  []string{regimeObjective},
	},
	"ByzantineQuorum": {
		class:   classConsensusCritical,
		why:     "Selects WHICH quorum rule is the validity bar (derived Byzantine threshold vs the Config.Quorum floor). Two replicas disagreeing here disagree about the rule itself, not just its input.",
		binding: "",
		readIn:  []string{regimeTrustedOptOut, regimeObjective},
	},
	"Anchors": {
		class:   classConsensusCritical,
		why:     "The launch training-wheels set: a commit in the young window needs AnchorQuorum attestations from it. Named in silt's own prose as genesis config discipline (chain.go:190).",
		binding: "",
		readIn:  []string{regimeYoungAnchors},
	},
	"AnchorQuorum": {
		class:   classConsensusCritical,
		why:     "The threshold on Anchors; same rule, same failure.",
		binding: "",
		readIn:  []string{regimeYoungAnchors},
	},
	"MatureValidators": {
		class:   classConsensusCritical,
		why:     "The Nakamoto coefficient that sheds the training wheels. Divergence means replicas disagree about WHICH regime the chain is in, which is a fork of the validity rule.",
		binding: "",
		readIn:  []string{regimeYoungAnchors},
	},
	"OperatorMargin": {
		class:   classConsensusCritical,
		why:     "The M discount on the C2 concentration metric that feeds Mature(); same regime-selection failure as MatureValidators.",
		binding: "",
		readIn:  []string{regimeYoungAnchors},
	},
	"EpochBlocks": {
		class:   classConsensusCritical,
		why:     "The epoch cadence. silt's own comment says it verbatim: 'Consensus-critical: every validator in a swarm must run the same value (like MinBond/Anchors — genesis config discipline)' (chain.go:189-191).",
		binding: "",
		readIn:  []string{regimeEpochs},
	},
	"RegGateActivationHeight": {
		class:   classConsensusCritical,
		why:     "The #506 R-rule pre-latch activation override. Its own comment: 'Consensus-critical genesis config, same discipline as MinBond/Anchors' (chain.go:256-257).",
		binding: "",
		readIn:  []string{regimeEpochs},
	},
	"BondTTLBlocks": {
		class:   classConsensusCritical,
		why:     "Bond standing lapses on this cadence, so it decides WHO is qualified at a height — a committed-state question, not a local one.",
		binding: "",
		readIn:  []string{regimeEpochs},
	},
	"AllowPublisher": {
		class:   classConsensusCritical,
		why:     "Permits a durable Publisher NodeID on an entry. A replica that refuses what another accepts splits on entry validity.",
		binding: "",
		readIn:  []string{regimeLegacy, regimeTrustedOptOut, regimeObjective},
	},
	"Era3ActivationHeight": {
		class:   classConsensusCritical,
		why:     "The era-3 pre-latch activation override — it decides WHICH format/validity rules apply at a height. Its own comment calls it consensus-critical genesis config (chain.go:243-260). Divergence means two replicas validate the same block under different eras.",
		binding: "partial: New() enforces Era4ActivationHeight >= Era3ActivationHeight (chain.go:1372) — a LOCAL coherence check, which canon rule 8 says cannot enforce a distributed agreement",
		readIn:  []string{regimeEpochs},
	},
	"Era4ActivationHeight": {
		class:   classConsensusCritical,
		why:     "The era-4 (v5) pre-latch activation override, read by era4Active (chain.go:3980-3985). Same failure as Era3ActivationHeight, and it is the boundary the whole D1 freeze is about.",
		binding: "partial: the same local New() ordering check; nothing binds the VALUE across replicas",
		readIn:  []string{regimeEpochs},
	},
	"BondRegHeadWindow": {
		class:   classConsensusCritical,
		why:     "Bounds how far back a bond registration may be anchored, which is an admission rule on committed content.",
		binding: "",
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
	"Archive": {
		class:  classLocal,
		why:    "Retention policy: whether THIS node keeps full bodies. An operator's storage choice, deliberately per-node (build-immutable #8) — it must never reach a validity verdict.",
		readIn: nil,
	},
	"WSCheckpoint": {
		class:  classLocal,
		why:    "Weak-subjectivity pin. Deliberately per-operator — it is the operator's own trust anchor, and silt is weakly subjective by design (TENETS Part 0). It narrows what THIS node accepts; it must never widen it.",
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
	var violations, divergingCritical, drivenSafe, unproven, unbound []string
	for _, f := range fields {
		d := configDecls[f]
		r := results[f]
		where := make([]string, 0, len(r.divergedIn))
		for rn := range r.divergedIn {
			where = append(where, rn)
		}
		sort.Strings(where)
		switch {
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

	t.Logf("CONSENSUS-CRITICAL and measured diverging (%d): %v", len(divergingCritical), divergingCritical)
	t.Logf("LOCAL and DRIVEN-SAFE (%d): %v", len(drivenSafe), drivenSafe)
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
	t.Logf("UNBOUND consensus-critical (%d, R-CONSENSUS-CONFIG-UNBOUND open): %v", len(unbound), unbound)
}
