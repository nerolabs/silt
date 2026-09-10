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
// This gate observes strings and their ORDER in cmd/silt/daemon.go, and it claims nothing else.
// That is the convention scripts/check_source_gates.py enforces and the idiom
// TestG_SLASHCAP_4_TheRefusalIsWiredAtStartup_Source already uses in this package.
//
// FOR A NEW FIELD ON A CONSENSUS TYPE, A READER IS NOT ENOUGH — the pin must require a non-test
// WRITER. G-CFGBIND-7 is that requirement: it fails if genesis.Build stops being handed real
// params on the daemon path, which is precisely the state the tree was in before this change.

// G-CFGBIND-7 — THE GENESIS MINT WRITES REAL PARAMS, on the production path.
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
				"-anchors, -epoch-blocks, -bond-label-k and -bond-vdf are local knobs again and two honest " +
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

// G-CFGBIND-8 — THE REFUSE-TO-START ARM HAS ITS PRODUCTION CALLER, in the right place, and it
// REFUSES rather than warns.
//
// WHAT THIS CATCHES THAT JOINING CANNOT (and therefore why the call must exist at all): an
// operator who edits a consensus flag and restarts on a chain this node has ALREADY joined. The
// genesis on disk is unchanged, so no fork boundary is crossed, no hash mismatch exists to detect,
// and the node simply begins applying different rules to a history it already holds.
//
// THE ORDER IS LOAD-BEARING, and getting it wrong is the vacuous-gate shape: CheckConsensusParams
// reads c.blocks[0] and returns nil on an EMPTY chain. Placed before the replay and the genesis
// seed it would be green on every start while checking nothing at all.
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

	// ORDER: after the genesis seed (which is itself after the replay). Anchored on the mint,
	// which G-CFGBIND-7 has already proven present.
	mintAt := strings.Index(s, "genesis.Build(store, &gp)")
	if mintAt < 0 {
		t.Fatal("SOURCE GATE: this gate checks an ORDER against the genesis mint, which is no longer in " +
			"daemon.go — re-home it (G-CFGBIND-7 reports the same absence).")
	}
	if callAt < mintAt {
		t.Fatal("SOURCE GATE: by string OFFSET the params check precedes the genesis seed in daemon.go. " +
			"CheckConsensusParams returns nil on an EMPTY chain (it reads blocks[0]), so on a fresh node it " +
			"would pass unconditionally while checking nothing — a green gate over an empty chain.")
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
