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
// FOR A NEW FIELD ON A CONSENSUS TYPE, A READER IS NOT ENOUGH — the pin must require a non-test
// WRITER. G-CFGBIND-7 is that requirement: it fails if genesis.Build stops being handed real
// params on the daemon path, which is precisely the state the tree was in before this change.

// G-CFGBIND-7 — THE GENESIS MINT WRITES REAL PARAMS, on the production path.
//
// RUNTIME GATE: e2e TestGenesisHashMovesWithTheConsensusConfig — two daemons differing only in
// -bond-label-k must mint DIFFERENT genesis blocks, and the same config the same one. The
// paramless mint compiles and prints the same line; what it cannot do is move the hash.
func TestG_CFGBIND_7_TheGenesisWiringIsOnTheProductionPath_Source(t *testing.T) {
	s := daemonSource(t)

	// The WRITER. `nil` here is the pre-bind path and is exactly the defect: it compiles, it is
	// silent, and it mints a network that commits no configuration.
	const mint = "genesis.Build(store, &gp)"
	mintAt := strings.Index(s, mint)
	if mintAt < 0 {
		if strings.Contains(s, "genesis.Build(store, nil)") {
			t.Fatal("WIRING REGRESSED: daemon.go mints genesis with `genesis.Build(store, nil)`. A network " +
				"launched by this binary commits NO consensus config at height 0, so -min-bond, -quorum, " +
				"-anchors, -epoch-blocks and -bond-label-k are local knobs again and two honest " +
				"operators who differ on any of them reach different validity verdicts on the same block (I1). " +
				"This is the exact shape the schema shipped in and sat inert.")
		}
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go. Either the mint moved (re-home this "+
			"gate) or the production genesis no longer carries params at all.", mint)
	}

	// The params must be projected off the CHAIN, not rebuilt from the Config literal. Both arms
	// — the mint and the refuse-to-start check — must read ONE source, or the node that founded
	// the network refuses its own genesis at the next restart.
	const project = "ch.ConsensusParams(cfg.BondLabelSamples, cfg.BondVDFDelay)"
	projAt := strings.Index(s, project)
	if projAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go. The committed params must be projected "+
			"off the chain's own config, so that the arm that WRITES genesis and the arm that CHECKS it read the "+
			"same source; a second projection built from the Config literal can drift from it silently.", project)
	}
	if projAt > mintAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params projection follows the genesis mint in daemon.go, " +
			"so the mint cannot be using it.")
	}
}

// G-CFGBIND-8 — THE REFUSE-TO-START ARM HAS ITS PRODUCTION CALLER, between the replay and the
// point this node joins consensus, and it REFUSES rather than warns.
//
// RUNTIME GATE: e2e TestDaemonRefusesToStartOnADivergentConsensusConfig — a persisted genesis
// committing k=64, a daemon started with -bond-label-k 32, a non-zero exit and no peer line.
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
// check reads; the upper bound is the point this node joins consensus with that chain, after which
// refusing is too late. It is still a lexical proxy — a helper defined BETWEEN the two landmarks
// would satisfy it — which is why the e2e above, not this gate, is the instrument of record.
func TestG_CFGBIND_8_TheParamsRefusalIsWiredAtStartup_Source(t *testing.T) {
	s := daemonSource(t)

	const call = "ch.CheckConsensusParams(cfg.BondLabelSamples, cfg.BondVDFDelay)"
	callAt := strings.Index(s, call)
	if callAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so the refuse-to-start arm has NO "+
			"production caller — the state this gate was written to close, in which the predicate existed, was "+
			"tested, and was never reached. An operator can edit a consensus flag, restart on a chain the node "+
			"has already joined, and apply different rules to that history with nothing objecting.", call)
	}

	// ORDER, LOWER BOUND: after the replay that populates the chain from disk.
	const recover_ = "chainstore.Recover("
	recoverAt := strings.Index(s, recover_)
	if recoverAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so this gate cannot locate the "+
			"replay it anchors the check against — re-home it, or the chain is no longer loaded from disk at "+
			"startup at all.", recover_)
	}
	if callAt < recoverAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params check precedes chainstore.Recover in daemon.go. " +
			"CheckConsensusParams reads blocks[0] and returns nil on an EMPTY chain, and the chain is empty " +
			"until the replay fills it — so the check would pass unconditionally on every start while the " +
			"divergent config it exists to catch is loaded a moment later. MEASURED: in that position the " +
			"binary serves under a -bond-label-k the chain's genesis contradicts, with zero refusal lines.")
	}
	// ORDER, UPPER BOUND: before this node joins consensus with that chain. Refusing after
	// EnableChain is refusing after the damage. This bound is what catches the check being lifted
	// into a helper defined further down the file (its text then lands after every landmark here).
	const enable = "nd.EnableChain(ch, ident.Signer())"
	enableAt := strings.Index(s, enable)
	if enableAt < 0 {
		t.Fatalf("SOURCE GATE: the string %q is absent from daemon.go, so this gate cannot locate the point "+
			"the node joins consensus — re-home it.", enable)
	}
	if callAt > enableAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params check follows nd.EnableChain in daemon.go, so " +
			"this node has already joined consensus with the chain before anything compares its config " +
			"against what that chain commits. If the check was moved into a helper, the CALL SITE is what " +
			"must sit between chainstore.Recover and EnableChain — and e2e " +
			"TestDaemonRefusesToStartOnADivergentConsensusConfig is the gate that decides whether it does.")
	}

	// It must REFUSE, not warn. A warning lets the node start and reach the divergent verdicts.
	tail := s[callAt:min(callAt+320, len(s))]
	if !strings.Contains(tail, "return fmt.Errorf") {
		t.Fatalf("SOURCE GATE: the 320 bytes of text after the params check contain no \"return fmt.Errorf\" — "+
			"a warning lets the node start and apply the divergent rules anyway, which is the whole failure this "+
			"arm exists to prevent; got: %q", tail)
	}
}

func daemonSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read daemon.go as text, so the wiring is unverified: %v", err)
	}
	return string(src)
}
