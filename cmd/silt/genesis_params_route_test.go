package main

import (
	"os"
	"strings"
	"testing"
)

// THE WIRING PIN, SOURCE HALF — owner call F.
//
// WHY A SOURCE GATE IS THE RIGHT INSTRUMENT HERE, narrowly. The defect being closed is not a wrong
// behaviour; it is an ABSENT CALL. chain.ConsensusParams shipped with 17 fields, five green gates,
// and no production writer: genesis was minted as a Block literal with no Params, cbor omitempty
// dropped the key, and CheckConsensusParams had ZERO non-test callers. No runtime assertion on the
// predicate can see that, because the predicate was always correct — it was simply never reached.
// What the runtime gates CAN see is covered next door: core/genesis G-CFGBIND-6 drives
// genesis.Build itself, and core/chain G-CFGBIND-1..5 drive the predicates.
//
// These gates observe strings and their ORDER in cmd/silt/daemon.go, and they claim nothing else.
// That is the convention scripts/check_source_gates.py enforces and the idiom
// TestG_SLASHCAP_4_TheRefusalIsWiredAtStartup_Source already uses in this package.
//
// THE RUNTIME COVER IS IN e2e, AND IT IS NOT OPTIONAL. A blind review measured the shape a
// source gate cannot see: the check lifted into a helper defined later in daemon.go and called
// BEFORE chainstore.Recover left every gate in this file GREEN while the mechanism was dead — the
// binary served under a divergent -bond-label-k with zero refusal lines. e2e/consensus_config_bind_test.go
// drives both halves in real processes and is what actually holds the seam; the order assertion
// below was re-anchored on that finding.
//
// UNGATED: R-CONSENSUS-CONFIG-UNBOUND — WHAT A GREEN IN THIS FILE DOES NOT MEAN. This hole is
// DISCLOSED, not closed, and the disclosure is deliberate. A second blind review measured the
// strongest surviving shape: the check written as a closure defined BETWEEN the two landmarks and
// NEVER INVOKED (`_ = checkParams`). Both gates in this file PASS over it, the mechanism is
// entirely dead, and only the e2e reddens. No stronger lexical gate closes that — a call-string
// check locates a STRING, and an uninvoked closure supplies the string by construction. What closes
// it is the runtime cover in the `Go — multi-process e2e (real TCP)` job, which runs the e2e
// package BY PACKAGE and without -short (every other go test in ci.yml passes -short, and both
// e2e tests skip under it).
//
// THAT COVER WAS NOT MERGE-BLOCKING WHEN THIS GATE WAS WRITTEN, and the fix is a repo-wide CI
// policy call the owner holds. He made it on 2026-09-11: the e2e job is added to ruleset 19729396's
// required status checks (five -> six) immediately AFTER the merge that lands this file —
// deliberately after, because a job must not be made required while the change it was added to
// protect is still in flight. Evidence he ruled on: three commits (18e267a, c22fa2c, 55900ac) were
// merge-eligible with that job as the sole red, and over the last 40 completed ci.yml runs on main
// the e2e job's mean is 448 s against the already required race job's 525 s (40/40 green), so it
// costs no wall-clock.
//
// SO READ THIS DISCLOSURE IN TWO LEGS. Leg one — "the runtime cover is not merge-blocking" —
// RETIRES the moment the ruleset read-back shows six contexts. Leg two does not retire and is the
// reason the marker stays: a green in THIS file means "the strings are present and in this order",
// never "the mechanism is live". Locally, run `go test ./e2e -run ConsensusConfig`.
//
// FOR A NEW FIELD ON A CONSENSUS TYPE, A READER IS NOT ENOUGH — the pin must require a non-test
// WRITER. G-CFGBIND-7 is that requirement: it fails if genesis.Build stops being handed real
// params on the daemon path, which is precisely the state the tree was in before this change.

// ungatedDisclosure rides EVERY failure message in this file, so the admission above is where a
// reader MEETS the gate and not only where a reader browses it. That is the repo's convention
// (ROADMAP.md, the R-CONFIG-GATE-V5-REGIME row): the marker in the DECLARATION and in the FAILURE
// TEXT, because the failure text is the sentence someone reads at 2 a.m. after a red.
//
// AND THE MARKER ITSELF IS NOT MACHINE-ENFORCED, measured: scripts/check_source_gates.py accepts
// `RUNTIME GATE:` OR `UNGATED:` (COVER_RE), and these gates carry both, so deleting every
// `UNGATED:` here leaves the lint at EXIT=0. What the lint DOES enforce, now that the daemon.go
// read is inlined into both test bodies rather than hidden behind a helper, is that both gates are
// in its scope at all (its count went 34 -> 36) and that every failure message opens with
// `SOURCE GATE:` (stripping one prefix reddens it, measured). Treat the disclosure as prose a human
// must keep true, not as a gate.
const ungatedDisclosure = " · UNGATED: R-CONSENSUS-CONFIG-UNBOUND — this is a STRING check on " +
	"daemon.go, and it is GREEN over a check that is defined between the landmarks and never " +
	"invoked (measured), so a green here is not evidence the mechanism is live. The instrument that " +
	"binds is e2e/consensus_config_bind_test.go (G-CFGBIND-10/11), in the `Go — multi-process e2e " +
	"(real TCP)` job — it skips under -short, so no other job runs it. Run " +
	"`go test ./e2e -run ConsensusConfig`."

// G-CFGBIND-7 — THE GENESIS MINT WRITES REAL PARAMS, on the production path.
//
// RUNTIME GATE: e2e TestGenesisHashMovesWithTheConsensusConfig — two daemons differing only in
// -bond-label-k must mint DIFFERENT genesis blocks, and the same config the same one. The
// paramless mint compiles and prints the same line; what it cannot do is move the hash.
//
// UNGATED: R-CONSENSUS-CONFIG-UNBOUND — that runtime gate lives in the e2e job, and this source
// gate cannot see whether the mechanism is live. See the two-leg disclosure at the top of this file.
func TestG_CFGBIND_7_TheGenesisWiringIsOnTheProductionPath_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read daemon.go as text, so the wiring is unverified: %v", err)
	}
	s := string(src)

	// The WRITER. `nil` here is the pre-bind path and is exactly the defect: it compiles, it is
	// silent, and it mints a network that commits no configuration.
	const mint = "genesis.Build(store, &gp)"
	mintAt := strings.Index(s, mint)
	if mintAt < 0 {
		if strings.Contains(s, "genesis.Build(store, nil)") {
			t.Fatal("SOURCE GATE: WIRING REGRESSED — daemon.go mints genesis with `genesis.Build(store, nil)`. A network " +
				"launched by this binary commits NO consensus config at height 0, so -min-bond, -quorum, " +
				"-anchors, -epoch-blocks and -bond-label-k are local knobs again and two honest " +
				"operators who differ on any of them reach different validity verdicts on the same block (I1). " +
				"This is the exact shape the schema shipped in and sat inert." + ungatedDisclosure)
		}
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go. Either the mint moved (re-home this "+
			"gate) or the production genesis no longer carries params at all.%s", mint, ungatedDisclosure)
	}

	// The params must be projected off the CHAIN, not rebuilt from the Config literal. Both arms
	// — the mint and the refuse-to-start check — must read ONE source, or the node that founded
	// the network refuses its own genesis at the next restart.
	const project = "ch.ConsensusParams(cfg.BondLabelSamples, cfg.BondVDFDelay)"
	projAt := strings.Index(s, project)
	if projAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go. The committed params must be projected "+
			"off the chain's own config, so that the arm that WRITES genesis and the arm that CHECKS it read the "+
			"same source; a second projection built from the Config literal can drift from it silently.%s",
			project, ungatedDisclosure)
	}
	if projAt > mintAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params projection follows the genesis mint in daemon.go, " +
			"so the mint cannot be using it." + ungatedDisclosure)
	}
}

// G-CFGBIND-8 — THE REFUSE-TO-START ARM HAS ITS PRODUCTION CALLER, between the replay and the
// point this node arms its consensus role, and it REFUSES rather than warns.
//
// RUNTIME GATE: e2e TestDaemonRefusesToStartOnADivergentConsensusConfig — a persisted genesis
// committing k=64, a daemon started with -bond-label-k 32, a non-zero exit and no peer line.
//
// UNGATED: R-CONSENSUS-CONFIG-UNBOUND — this gate is green over an uninvoked closure, and only the
// e2e decides whether the check runs. See the two-leg disclosure at the top of this file.
//
// WHAT THIS CATCHES THAT JOINING CANNOT (and therefore why the call must exist at all): an
// operator who edits a consensus flag and restarts on a chain this node has ALREADY joined. The
// genesis on disk is unchanged, so no fork boundary is crossed, no hash mismatch exists to detect,
// and the node simply begins applying different rules to a history it already holds.
//
// THE ORDER IS LOAD-BEARING, AND THE BOUNDARY IS THE REPLAY — NOT THE GENESIS SEED. The check is
// meaningful only against a chain LOADED FROM DISK. On a fresh node the committed params and the
// local ones are both ParamsFromConfig over the same cfg in the same process, so wherever it sits
// relative to the mint it is a TAUTOLOGY; an earlier draft of this gate pinned mint -> check and
// published that ordering as the reason, which is false and was measured false (a tree with the
// check moved ahead of the mint refuses identically). What kills the mechanism is placing the
// check ahead of chainstore.Recover: the chain is then empty, CheckConsensusParams returns nil on
// blocks[0], and the daemon serves under a config its own chain contradicts. That was measured
// too, with the previous form of this gate GREEN over it.
//
// So the assertion is a SANDWICH: Recover < check < EnableChain. The lower bound is the state the
// check reads. THE UPPER BOUND IS NOT A LAST-SAFE-MOMENT, and an earlier draft of this comment
// wrongly said refusing after it was too late: measured, with the check moved after EnableChain the
// daemon still exits 1 and never prints a peer line, and the TCP listener has been accepting since
// tcpnet.New (cmd/silt/daemon.go:266) in either position. What EnableChain actually is, is the
// point this node ARMS its consensus role — two field assignments (core/node/chainrole.go:22-25),
// with every chain-role handler nil-guarded on n.chain — which makes it a cheap lexical anchor for
// "still inside startup". Its measured WORK is the other half of the sandwich: it is what catches
// the check lifted into a helper defined further down the file, whose text lands after every
// landmark here. Both bounds are lexical; see the UNGATED disclosure at the top of this file for
// the shape neither of them can see.
func TestG_CFGBIND_8_TheParamsRefusalIsWiredAtStartup_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read daemon.go as text, so the wiring is unverified: %v", err)
	}
	s := string(src)

	const call = "ch.CheckConsensusParams(cfg.BondLabelSamples, cfg.BondVDFDelay)"
	callAt := strings.Index(s, call)
	if callAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so the refuse-to-start arm has NO "+
			"production caller — the state this gate was written to close, in which the predicate existed, was "+
			"tested, and was never reached. An operator can edit a consensus flag, restart on a chain the node "+
			"has already joined, and apply different rules to that history with nothing objecting.%s",
			call, ungatedDisclosure)
	}

	// ORDER, LOWER BOUND: after the replay that populates the chain from disk.
	const recover_ = "chainstore.Recover("
	recoverAt := strings.Index(s, recover_)
	if recoverAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so this gate cannot locate the "+
			"replay it anchors the check against — re-home it, or the chain is no longer loaded from disk at "+
			"startup at all.%s", recover_, ungatedDisclosure)
	}
	if callAt < recoverAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params check precedes chainstore.Recover in daemon.go. " +
			"CheckConsensusParams reads blocks[0] and returns nil on an EMPTY chain, and the chain is empty " +
			"until the replay fills it — so the check would pass unconditionally on every start while the " +
			"divergent config it exists to catch is loaded a moment later. MEASURED: in that position the " +
			"binary serves under a -bond-label-k the chain's genesis contradicts, with zero refusal lines." +
			ungatedDisclosure)
	}
	// ORDER, UPPER BOUND: before this node arms its consensus role. Its measured work is catching
	// the check lifted into a helper defined further down the file (its text then lands after every
	// landmark here). It is an anchor for "inside startup", not a last-safe-moment — see the
	// SANDWICH paragraph above.
	const enable = "nd.EnableChain(ch, ident.Signer())"
	enableAt := strings.Index(s, enable)
	if enableAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so this gate cannot locate the point "+
			"the node arms its consensus role — re-home it.%s", enable, ungatedDisclosure)
	}
	if callAt > enableAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params check follows nd.EnableChain in daemon.go, so " +
			"this node arms its consensus role with the chain before anything compares its config against " +
			"what that chain commits. What this gate LOCATES is the predicate INVOCATION STRING " +
			"`ch.CheckConsensusParams(cfg.BondLabelSamples, cfg.BondVDFDelay)` and its offset, and nothing " +
			"else: a closure that contains that string between the two landmarks and is never invoked is " +
			"GREEN here (measured). e2e TestDaemonRefusesToStartOnADivergentConsensusConfig is what decides " +
			"whether the check actually runs." + ungatedDisclosure)
	}

	// It must REFUSE, not warn. A warning lets the node start and reach the divergent verdicts.
	tail := s[callAt:min(callAt+320, len(s))]
	if !strings.Contains(tail, "return fmt.Errorf") {
		t.Fatalf("SOURCE GATE: the 320 bytes of text after the params check contain no \"return fmt.Errorf\" — "+
			"a warning lets the node start and apply the divergent rules anyway, which is the whole failure this "+
			"arm exists to prevent; got: %q%s", tail, ungatedDisclosure)
	}
}
