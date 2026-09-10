package node

import (
	"crypto/ed25519"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/vdf"
)

// THE CONFIG-IN-CONSENSUS GATE, node.Config HALF — closes R-CONFIG-GATE-NODE-SCOPE.
//
// THE MEMBERSHIP RULE (owner, 2026-09-10): every field that can change a validity verdict is
// BOUND TO THE CHAIN, or is EXPLICITLY EXCLUDED WITH A RECORDED REASON. The owner rejected both
// "five fields" and "17 fields" as the thing to ratify: the rule is ratified and the field count
// is its OUTPUT.
//
// WHY THIS FILE EXISTS. chain.Config's gate (core/chain/consensus_config_divergence_test.go)
// closes its complement over chain.Config ALONE. That is the WRONG SET, and the cost was
// measured, not hypothetical: BondLabelSamples and BondVDFDelay live in node.Config, reach a hard
// chain Reject through core/bond's verifier, and were invisible to a reflection that never looked
// at this struct. A k=32 node rejects EVERY bond registration a k=64 swarm accepts, and the flag
// help states the coordination requirement in the same breath as it invites the change.
//
// THE RULE HAS TWO HALVES HERE, and they catch different failures:
//
//	THE CLOSED COMPLEMENT (structural). Every node.Config field is reached by REFLECTION and must
//	carry a declaration. There is no third state, so a NEW field fails this gate until someone
//	decides its class. This is what would have caught BondLabelSamples the day it was added.
//
//	THE DRIVEN PROBE (runtime). Simplicity rule 7: a green gate with no demonstrated red is
//	decoration, and a field called "safe" must be a DRIVEN probe. So the fields that reach the
//	real bond verifier are perturbed against a REAL sealed plot and a REAL space-time answer, and
//	the measured divergence map is pinned. A declaration that contradicts the measurement fails.
//
// WHAT THIS GATE DELIBERATELY DOES NOT CLAIM. Most of node.Config is transport, DHT and repair
// policy that the bond verifier never reads, so no probe here can move it. Those fields report
// UNPROVEN, never "safe" — a field that does not diverge in a regime that could never have
// exercised it has been shown UNTESTED, not shown safe. Calling them safe would be the decoration
// failure in a new place.
//
// ABLATION BATTERY, run 2026-09-10 before this gate was trusted, each verified by EXIT CODE and
// each with a no-op guard (diff the patched file against its original before believing the run —
// a patch that silently fails to apply reports GREEN and is indistinguishable from a passing
// ablation):
//
//	N0 baseline ................................................................ GREEN
//	N1 add an undeclared verdict-reaching field to node.Config ................. RED   (owner's binding condition)
//	N2 leave a stale declaration for a field node.Config no longer has ......... RED
//	N3 re-declare BondLabelSamples as nodeClassLocal (the defect class itself) .. RED
//	N4 point a carriedAs at a chain.ConsensusParams field that does not exist ... RED
//	N5 delete BondLabelSamples from chain.ConsensusParams ...................... RED
//	N6 drop k from the verifier so BondLabelSamples stops diverging ............ RED
//	N7 restore everything ..................................................... GREEN

type nodeConfigClass int

const (
	// nodeClassLocal: free to differ between honest replicas. If it moves a validity verdict,
	// that is the defect this gate exists to catch.
	nodeClassLocal nodeConfigClass = iota
	// nodeClassConsensusCritical: must be swarm-uniform. Canon rule 8 governs HOW.
	nodeClassConsensusCritical
)

// nodeConfigDecl is one field's declaration. A consensus-critical row must set EXACTLY ONE of
// carriedAs / reasonNotCarried — that pair IS the owner's rule, made machine-checkable.
type nodeConfigDecl struct {
	class nodeConfigClass
	why   string
	// carriedAs names the chain.ConsensusParams field that BINDS this one to the chain. It is
	// checked by REFLECTION against the real struct, so it cannot rot into prose: renaming or
	// removing the ConsensusParams field turns this gate red.
	carriedAs string
	// reasonNotCarried is the RECORDED REASON this verdict-reaching field is not bound BY THIS
	// TABLE. Two kinds of reason live here and they are not the same claim: the field is bound
	// somewhere else (canon rule 8's first arm, or transitively through chain.Config), or it is
	// deliberately left unbound. The owner's rule permits an exclusion; it does not permit a
	// SILENT one, so this string is mandatory whenever carriedAs is empty.
	reasonNotCarried string
}

// THE DECLARATION TABLE. Adding a node.Config field without adding a row here FAILS this gate.
var nodeConfigDecls = map[string]nodeConfigDecl{
	// ---- BOUND TO THE CHAIN: the three that reach a chain validity verdict ----
	"BondLabelSamples": {
		class: nodeClassConsensusCritical,
		why: "k, the labeling-consistency opens a bond challenge carries. core/bond's verifyLabels compares " +
			"len(a.LabelIndices) against the VERIFIER's own resolveK(k) and returns false on a mismatch, so a k=32 " +
			"node rejects every bond registration a k=64 swarm accepts — a hard Reject on the bond-reg admission " +
			"path. Two honest replicas then disagree on the same block (I1). This is the field that proved the " +
			"reflection was closed over the wrong set.",
		carriedAs: "BondLabelSamples",
	},
	"BondVDFDelay": {
		class: nodeClassConsensusCritical,
		why: "The sequential-squaring delay a bond proof must attest. VerifySpaceTime refuses outright unless " +
			"a.VDFT == delay EXACTLY, so it is the same shape as BondLabelSamples and was latent for the same reason.",
		carriedAs: "BondVDFDelay",
	},
	"MinBondBytes": {
		class: nodeClassConsensusCritical,
		why: "The anti-release floor. The daemon feeds THIS field into chain.Config.MinBondBytes, where it is a " +
			"bond-reg admission threshold — so the node-side value is the source of a chain validity term, not a " +
			"separate local knob that happens to share a name.",
		reasonNotCarried: "BOUND TRANSITIVELY, and this table must not claim it twice. cmd/silt/daemon.go assigns " +
			"this value into chain.Config.MinBondBytes, and THAT field is what ParamsFromConfig projects into " +
			"chain.ConsensusParams.MinBondBytes — so the binding is real, but the claimant is core/chain's table. " +
			"The bijection check below is what forced this row to be honest: with both tables claiming the same " +
			"committed field, one of the two knobs would have been unbound while the tables read as complete.",
	},

	// ---- CONSENSUS-CRITICAL BUT DELIBERATELY NOT CARRIED: the recorded reasons ----
	"MaxBondRegBytesPerBlock": {
		class: nodeClassConsensusCritical,
		why: "Proposer-side byte budget. Validity is unchanged by it (a block with N regs is valid), so mixed " +
			"caps across proposers are safe — but D-SLASHCAP-ROUTE found the second face: raised past a bound it " +
			"makes this validator's OWN equivocation unprovable, because the evidence pair exceeds SlashesBytesCap.",
		reasonNotCarried: "BOUND BY CANON RULE 8's FIRST ARM INSTEAD, which is the correct arm here: the invariant is a " +
			"relationship between two LOCAL flags and a consensus constant, so it IS locally checkable and a " +
			"refuse-to-start has a referent without needing committed state. node.CheckSlashEvidenceHeadroom is " +
			"that check, wired in cmd/silt/daemon.go and pinned by G-SLASHCAP-3/4. Carrying it into the genesis " +
			"would ALSO forbid honest operators from differing on a proposer-side budget that validity ignores.",
	},
	"MaxEntryBytesPerBlock": {
		class:            nodeClassConsensusCritical,
		why:              "The entry-side half of the same proposer budget, and the second input to the same headroom check.",
		reasonNotCarried: "Same arm and same check as MaxBondRegBytesPerBlock: node.CheckSlashEvidenceHeadroom.",
	},

	// ---- LOCAL: transport, discovery, repair and retention policy ----
	"PublishWorkCounters":         {class: nodeClassLocal, why: "Gossip disclosure switch (-privacy). It governs what this node PUBLISHES about itself, never what it ACCEPTS from anyone."},
	"K":                           {class: nodeClassLocal, why: "Kademlia bucket size — routing-table shape. Discovery topology reaches no validity verdict."},
	"Alpha":                       {class: nodeClassLocal, why: "Lookup parallelism; a performance knob on discovery."},
	"RequestTimeout":              {class: nodeClassLocal, why: "Transport deadline. build-immutable #3/#4: transport deadlines are DELIBERATELY decoupled from every security bound, so hardening the network never moves a validity term."},
	"RequestRetries":              {class: nodeClassLocal, why: "How many times a timed-out RPC is re-sent. Liveness under jitter, not validity."},
	"RequestBackoff":              {class: nodeClassLocal, why: "Base backoff between RPC retries; same class as RequestRetries."},
	"HolderDialTimeout":           {class: nodeClassLocal, why: "Tighter deadline on speculative holder-fetch dials; a fetch-path cost knob."},
	"RequestSizeFloorBytesPerSec": {class: nodeClassLocal, why: "Extends a transport deadline for a large payload (#286). It changes WHETHER a message arrives in time, never whether an arrived block is valid."},
	"Replication":                 {class: nodeClassLocal, why: "How many nodes receive each chunk. Placement policy; durability, not validity."},
	"RepairInterval":              {class: nodeClassLocal, why: "Caretaker sweep cadence."},
	"RepairSlack":                 {class: nodeClassLocal, why: "Missing-shard tolerance before repair fires."},
	"RepairEconomy":               {class: nodeClassLocal, why: "S7 repair-bounty PARTICIPATION switch, opt-in by design (PE ruling 2026-08-19 Q1). Credit is per-node-local accounting; the bounty AMOUNT is protocol-priced and is not this flag."},
	"RepairQuorumTau":             {class: nodeClassLocal, why: "This judge's own retrievability-confirmation count. Correctness is single-verifier-sufficient; tau gates only the retrievability leg, per judge."},
	"Domain":                      {class: nodeClassLocal, why: "The operator's failure-domain label. Per-node BY DEFINITION — a shared value would defeat the diversity it exists to create."},
	"HotThreshold":                {class: nodeClassLocal, why: "Demand-responsive dispersion trigger; a caching policy."},
	"DemandInterval":              {class: nodeClassLocal, why: "The window HotThreshold counts over."},
	"LeaseTTL":                    {class: nodeClassLocal, why: "Expiry on leased cache copies."},
	"FanoutReplicas":              {class: nodeClassLocal, why: "Extra cache copies pushed for a hot chunk."},
	"ReachabilityTimeout":         {class: nodeClassLocal, why: "Bounds a NAT dial-back check; a transport deadline."},
	"FetchAttempts":               {class: nodeClassLocal, why: "Chunk-fetch provider re-sweeps under transient failure."},
	"FetchBackoff":                {class: nodeClassLocal, why: "Base delay between fetch re-sweeps."},
	"HolderCooldown":              {class: nodeClassLocal, why: "Negative cache on holders that timed out (#226)."},
	"BondAuditInterval":           {class: nodeClassLocal, why: "How often THIS node challenges peers' bonds. It drives the LOCAL reputation view, which the objective path deliberately does not read (D2 / red-team F6)."},
	"BondMaxAge":                  {class: nodeClassLocal, why: "Local decay of un-re-proven standing in the reputation view. The ON-CHAIN lapse is chain.Config.BondTTLBlocks, which IS carried."},
	"ChainSyncInterval":           {class: nodeClassLocal, why: "Reconcile cadence. It changes WHEN this node catches up, never WHAT it accepts."},
	"BootstrapRetryInterval":      {class: nodeClassLocal, why: "Kademlia re-join cadence for an isolated node (#281)."},
	"BootstrapWellConnected":      {class: nodeClassLocal, why: "Routing-table size at which a node stops refreshing buckets."},
	"BondMaxAnswerLatency":        {class: nodeClassLocal, why: "Reply deadline on a LIVE bond challenge (C1 BREAK 1 enforcement leg). It is wall-clock and SOFT by nature, so it gates the local reputation view; it is not a chain validity term, and making it one would make validity fastest-evaluator-sensitive."},
	"RequireSignedProviders":      {class: nodeClassLocal, why: "NARROWING ONLY (M0 H5): this node accepts fewer provider records. A narrowing DHT policy cannot make a block valid that another node rejects."},
	"ProviderRecordTTL":           {class: nodeClassLocal, why: "Freshness stamp on this node's own signed provider records; narrowing, same class as RequireSignedProviders."},
	"DHTDomainCap":                {class: nodeClassLocal, why: "Per-bucket failure-domain diversity cap (M0 H5-B). Discovery topology; eclipse resistance, not validity."},
}

// theTwoNodeSideParams is the residue chain.Config's gate NAMES and this gate CLAIMS. The two
// tables together must claim every chain.ConsensusParams field exactly once; each gate checks its
// own half and names the other's, which keeps the bijection machine-checked without either
// package importing the other's test fixtures.
var theTwoNodeSideParams = []string{"BondLabelSamples", "BondVDFDelay"}

// G-CFGBIND-9 — THE MEMBERSHIP RULE OVER node.Config: bound to the chain, or excluded with a
// recorded reason. Closes R-CONFIG-GATE-NODE-SCOPE.
func TestNodeConsensusVerdictIsNotAFunctionOfLocalConfig(t *testing.T) {
	// ---- the closed complement: every node.Config field must be declared ----
	ct := reflect.TypeOf(Config{})
	fields := make([]string, 0, ct.NumField())
	var undeclared []string
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		fields = append(fields, name)
		if _, ok := nodeConfigDecls[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	if len(undeclared) > 0 {
		t.Fatalf("node.Config has %d UNDECLARED field(s): %v\n"+
			"THE MEMBERSHIP RULE (owner, 2026-09-10): every field that can change a validity verdict is BOUND TO\n"+
			"THE CHAIN, or is EXPLICITLY EXCLUDED WITH A RECORDED REASON. There is no third state and no field may\n"+
			"be silently absent.\n"+
			"This struct is NOT off the consensus path: BondLabelSamples and BondVDFDelay reach a hard chain\n"+
			"Reject through core/bond's verifier, and they were missed precisely because the only reflective gate\n"+
			"was closed over chain.Config. Decide the class and write the justification; do not delete this check.",
			len(undeclared), undeclared)
	}
	// A stale declaration is as bad as a missing one — it asserts a destination that does not exist.
	for name := range nodeConfigDecls {
		if _, ok := ct.FieldByName(name); !ok {
			t.Fatalf("nodeConfigDecls declares %q, which is no longer a node.Config field — remove the row", name)
		}
	}

	// ---- the rule itself: consensus-critical => carried XOR excluded, and carriedAs must RESOLVE ----
	params := reflect.TypeOf(chain.ConsensusParams{})
	claimed := map[string]string{} // ConsensusParams field -> node.Config field claiming it
	for _, f := range fields {
		d := nodeConfigDecls[f]
		if d.class != nodeClassConsensusCritical {
			if d.carriedAs != "" || d.reasonNotCarried != "" {
				t.Fatalf("%s is declared LOCAL but carries a binding/exclusion (%q/%q). A local field needs neither; "+
					"if it needs one, it is not local.", f, d.carriedAs, d.reasonNotCarried)
			}
			continue
		}
		switch {
		case d.carriedAs != "" && d.reasonNotCarried != "":
			t.Fatalf("%s declares BOTH carriedAs=%q and reasonNotCarried — a field is bound to the chain or it is excluded, "+
				"never both, or the reader cannot tell which claim is load-bearing", f, d.carriedAs)
		case d.carriedAs == "" && d.reasonNotCarried == "":
			t.Fatalf("%s is CONSENSUS-CRITICAL but names neither a chain.ConsensusParams field that binds it nor a "+
				"recorded reason for excluding it. That is the membership rule, verbatim: bound to the chain, or "+
				"excluded with a recorded reason.\nwhy: %s", f, d.why)
		case d.carriedAs != "":
			// THE STRUCTURAL BIND. A pin on prose is not a pin: this resolves the named field
			// against the REAL struct, so removing or renaming it turns this gate red instead of
			// leaving a sentence that quietly stops being true.
			if _, ok := params.FieldByName(d.carriedAs); !ok {
				t.Fatalf("%s declares carriedAs=%q, but chain.ConsensusParams has NO such field. Either the bind was "+
					"never written, or the field was renamed/removed and this node-side knob is unbound again — which "+
					"is the state BondLabelSamples was in through three audits.", f, d.carriedAs)
			}
			if prev, dup := claimed[d.carriedAs]; dup {
				t.Fatalf("chain.ConsensusParams.%s is claimed by BOTH %s and %s. One committed field cannot bind two "+
					"local knobs — one of them is unbound and the table hides which.", d.carriedAs, prev, f)
			}
			claimed[d.carriedAs] = f
		}
	}

	// THE RESIDUE, both directions. chain.Config's gate names these two as the fields it does NOT
	// claim; this gate must claim exactly them. Together the two tables are a bijection onto
	// chain.ConsensusParams, which is what makes "17 fields" the RULE'S OUTPUT rather than an
	// approved list someone has to remember.
	var got []string
	for p := range claimed {
		got = append(got, p)
	}
	sort.Strings(got)
	want := append([]string(nil), theTwoNodeSideParams...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the node-side half of chain.ConsensusParams changed.\n  claimed here: %v\n  named residue: %v\n"+
			"core/chain's gate closes its complement over chain.Config and names exactly this residue as the part it\n"+
			"does not claim. If a ConsensusParams field is claimed by neither table it is committed but unowned; if\n"+
			"it is claimed by both, the membership is double-counted. Update BOTH gates in one change.", got, want)
	}

	// ---- THE DRIVEN HALF: perturb against the REAL bond verifier ----
	// A real sealed plot and a real space-time answer, so an ACCEPT is a genuine accept. A probe
	// that could never accept at baseline cannot detect divergence, because every perturbation
	// rejects too — the vacuous shape this repo has been bitten by four times.
	const (
		plotSize  = 1 << 20 // 256 blocks of 4 KiB: enough for real label opens, cheap to seal
		baseK     = 16      // below DefaultLabelSamples so the perturbation can move DOWN and UP
		baseDelay = 64      // small but non-zero: delay 0 takes the space-only path and cannot probe BondVDFDelay
		nonce     = 7
	)
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := bond.Seal(pub, plotSize)
	ans, ok := c.AnswerSpaceTime(nonce, vdf.Default(), baseDelay, baseK)
	if !ok {
		t.Fatal("could not produce a space-time answer for the fixture plot — the probe cannot accept at baseline, " +
			"so every perturbation would reject too and this gate would be vacuous")
	}
	answer, err := bond.EncodeAnswer(ans)
	if err != nil {
		t.Fatal(err)
	}

	verdict := func(cfg Config) string {
		// The REAL production closure the daemon wires onto the replica, not a re-implementation.
		if SpaceTimeBondVerifier(cfg.BondVDFDelay, cfg.BondLabelSamples)(pub, c.Root, plotSize, nonce, answer) {
			return "ACCEPT"
		}
		return "REJECT"
	}

	base := Config{BondLabelSamples: baseK, BondVDFDelay: baseDelay}
	if ref := verdict(base); ref != "ACCEPT" {
		t.Fatalf("the baseline bond verdict is %s, not ACCEPT. A regime whose baseline REJECTS cannot observe "+
			"divergence — every perturbation rejects as well and the gate reports a clean bill over nothing.", ref)
	}

	probed := map[string]bool{}
	diverged := map[string]bool{}
	for _, f := range fields {
		mutated, ok := perturbNodeConfig(base, f)
		if !ok {
			continue
		}
		probed[f] = true
		if verdict(mutated) != "ACCEPT" {
			diverged[f] = true
		}
	}

	// ---- the verdict, and the three states simplicity rule 7 demands ----
	var violations, drivenSafe, unproven []string
	for _, f := range fields {
		d := nodeConfigDecls[f]
		switch {
		case d.class == nodeClassLocal && diverged[f]:
			violations = append(violations, fmt.Sprintf(
				"%s is declared LOCAL but CHANGES the bond verifier's verdict — %s", f, d.why))
		case d.class == nodeClassConsensusCritical && diverged[f] && d.carriedAs == "":
			violations = append(violations, fmt.Sprintf(
				"%s measurably changes the verdict and is EXCLUDED rather than bound. An exclusion is a claim that "+
					"the field reaches no verdict this gate can see; the measurement contradicts it. Reason on file: %s",
				f, d.reasonNotCarried))
		case d.class == nodeClassLocal && probed[f]:
			drivenSafe = append(drivenSafe, f)
		case d.class == nodeClassLocal:
			unproven = append(unproven, f)
		}
	}
	sort.Strings(violations)
	sort.Strings(unproven)
	if len(violations) > 0 {
		t.Fatalf("the membership rule is VIOLATED — %d finding(s):\n  %v\n"+
			"A consensus quantity must be a function of the CHAIN (canon rule 8). Bind it to chain.ConsensusParams, "+
			"or re-declare it with the reason it is excluded.", len(violations), violations)
	}
	t.Logf("LOCAL and DRIVEN-SAFE (%d): %v", len(drivenSafe), drivenSafe)
	// UNPROVEN is REPORTED, never asserted safe (simplicity rule 7). Most of node.Config is
	// transport/DHT/repair policy the bond verifier never reads, so no probe here can move it.
	// "Did not diverge in a regime that could not have exercised it" is UNTESTED, not safe.
	t.Logf("UNPROVEN — the bond-verifier probe cannot exercise these; NOT a safety claim (%d): %v", len(unproven), unproven)

	// THE DIVERGENCE-MAP PIN. It pins the MEASUREMENT, not prose, so nothing about it can be
	// satisfied by editing a comment. A field that newly moves the verdict, or one that stops
	// moving it because a probe went dead, fails here and forces a decision.
	var divergedList []string
	for f := range diverged {
		divergedList = append(divergedList, f)
	}
	sort.Strings(divergedList)
	wantDiverged := []string{"BondLabelSamples", "BondVDFDelay"}
	if !reflect.DeepEqual(divergedList, wantDiverged) {
		t.Fatalf("the NODE CONFIG DIVERGENCE MAP changed.\n  got:  %v\n  want: %v\n"+
			"A newly-diverging field is a new instance of the class this gate was built for: bind it to the CHAIN and "+
			"route it as a consensus-rule change. A field that STOPPED diverging means either its binding landed or "+
			"its probe went dead — and a dead probe is the more likely of the two.", divergedList, wantDiverged)
	}
}

// perturbNodeConfig moves ONE field, or reports ok=false when this gate has no probe for it. An
// unknown perturbation is NOT a pass — it leaves the field UNPROVEN, and the gate says so.
//
// Only the fields the bond verifier actually reads have probes, and each one must CROSS the
// boundary it is read against: verifyLabels compares against resolveK(k) and VerifySpaceTime
// demands a.VDFT == delay exactly, so a nudge that lands on the same effective value is a dead
// probe wearing a live one's clothes.
func perturbNodeConfig(cfg Config, field string) (Config, bool) {
	switch field {
	case "BondLabelSamples":
		cfg.BondLabelSamples *= 2 // the k=32-vs-k=64 shape, at fixture scale
	case "BondVDFDelay":
		cfg.BondVDFDelay++ // VerifySpaceTime requires EXACT equality, so one is enough
	default:
		return cfg, false
	}
	return cfg, true
}
