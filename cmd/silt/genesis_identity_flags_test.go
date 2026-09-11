package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// M1 — THE THREE GENESIS FLAGS, AND THE NETWORK-IDENTITY LINE.
// =============================================================================
//
// Era3ActivationHeight and Era4ActivationHeight have been committed at ConsensusParams cbor keys
// 13/14 and have ridden the refuse-to-start arm since the genesis-config bind landed. They had NO
// FLAG: `grep Era[34]ActivationHeight cmd/` returned nothing outside a test, so every node the
// daemon started ran the post-latch readiness tally whether or not that was what the operator
// wanted, and CheckConsensusParams' own diagnosis named `era4-activation-height` — telling an
// operator to change something that did not exist. That is the same defect the `bond-vdf-delay`
// row documents in Diff's own comment, and these gates close it for the era pair.
//
// WHY THE DEFAULT IS 1/1, and it is not convenience. The era-4 attestation form binds the chain
// id; the era-2 form does not and is FROZEN, so at every height below H_era4 a consensus signature
// carries no network and is portable between silt networks. Committing Era4ActivationHeight = 1
// leaves NO height above the genesis in that interval. The flag and the evidence rule that reads
// the floor are one decision, not two.

// G-NET-1 — THE WIRING, as a SOURCE gate, with the limit stated rather than glossed.
//
// It reads daemon.go as TEXT, so it sees a string and an order. It proves the renderer is CALLED
// in the start-up path's source and that the call sits after both loads it describes; it does NOT
// prove the call is REACHED at run time — a new early return above it would be invisible here.
// This is the identical shape and the identical limit as G-DE-8 for the era pair.
//
// UNGATED: no test in this repo boots the real daemon, so nothing observes these lines at runtime.
func TestDaemonPrintsTheNetworkIdentityAtStartUp(t *testing.T) {
	s := daemonSource(t)
	const call = "for _, ln := range networkIdentityLines(ch) {"
	if !strings.Contains(s, call) {
		t.Fatalf("SOURCE GATE: G-NET-1 RED — the literal %q is no longer in daemon.go's text, so the daemon "+
			"no longer reports WHICH NETWORK it is on. The renderer is then decoration, and the owner's "+
			"requirement (report the network by both a name and a cryptographic identifier) is unmet by a "+
			"build whose unit tests are all green.", call)
	}
	at := strings.Index(s, call)
	for _, before := range []string{"chainstore.Recover(chainPath, ch, *acceptChainLoss)", "genesis.Build(store, &gp)"} {
		if i := strings.Index(s, before); i < 0 || i > at {
			t.Fatalf("SOURCE GATE: G-NET-1 RED — in daemon.go's TEXT the network-identity call appears BEFORE "+
				"%q, so at run time it would describe a chain the daemon has not finished loading: an empty "+
				"chain on every restart, or an unseeded one on every fresh node. This checks ORDER in the "+
				"source, not execution order.", before)
		}
	}
}

// G-NET-2 — THE THREE GENESIS FLAGS EXIST, CARRY THE RATIFIED DEFAULTS, AND REACH chain.Config.
//
// A flag declared and never assigned is the sharper failure of the two: it reports as supported in
// -help, an operator sets it, and the genesis commits something else. Both halves are checked.
func TestTheThreeGenesisFlagsAreDeclaredAndWired(t *testing.T) {
	s := daemonSource(t)
	for _, f := range []struct{ decl, assign, why string }{
		{`fs.Uint64("era3-activation-height", 1,`, "Era3ActivationHeight: *era3Activation,",
			"the era-3 boundary is committed at ConsensusParams cbor key 13"},
		{`fs.Uint64("era4-activation-height", 1,`, "Era4ActivationHeight: *era4Activation,",
			"the era-4 boundary is committed at cbor key 14, and its default 1 is what empties the " +
				"sub-era-4 interval where an attestation carries no chain id"},
		{`fs.String("network-name", "",`, "NetworkName:          *networkName,",
			"the network's committed name, cbor key 18"},
	} {
		if !strings.Contains(s, f.decl) {
			t.Fatalf("G-NET-2 RED: daemon.go no longer declares the flag %q — %s. A committed genesis field "+
				"with no flag cannot be set by the operator who founds the network, and CheckConsensusParams' "+
				"diagnosis would name a flag that does not exist.", f.decl, f.why)
		}
		if !strings.Contains(s, f.assign) {
			t.Fatalf("G-NET-2 RED: daemon.go declares a flag but never assigns %q into the chain.Config literal "+
				"— %s. A flag that reaches no config is WORSE than a missing one: -help reports it as "+
				"supported, the operator sets it, and the genesis commits something else.", f.assign, f.why)
		}
		if d := strings.Index(s, f.decl); d > strings.Index(s, f.assign) {
			t.Fatalf("G-NET-2 RED: %q is declared AFTER it is assigned in daemon.go's text", f.decl)
		}
	}
}

// G-NET-3 — THE RC POSTURE, DRIVEN: Era4ActivationHeight = 1 empties the sub-era-4 interval above
// the genesis, and the height-0 residual is named rather than hidden.
//
// This is the measurement the flag default rests on. It does NOT assert that the daemon passes 1 —
// G-NET-2 does that on the source — it asserts what committing 1 MEANS, which is the half a source
// gate cannot see.
func TestEra4ActivationOneEmptiesTheSubEra4Interval(t *testing.T) {
	c := chain.New(chain.Config{Quorum: 1, Era3ActivationHeight: 1, Era4ActivationHeight: 1},
		func(ports.NodeID) int64 { return 0 })
	for _, h := range []uint64{1, 2, 3, 8, 1 << 20} {
		if got := c.MintVersion(h); got != chain.BlockVersionWitnessable {
			t.Fatalf("G-NET-3 RED: with Era4ActivationHeight=1, height %d mints v%d, not era 4 (v%d). The "+
				"sub-era-4 interval is then NOT empty, and a consensus attestation at that height carries no "+
				"chain id — portable to any other silt network", h, got, chain.BlockVersionWitnessable)
		}
	}
	// THE HEIGHT-0 RESIDUAL, ASSERTED RATHER THAN GLOSSED. era4Active is `h >= H`, so height 0 is
	// below the boundary even at H = 1 and still mints the era-2 form. It is unreachable on the
	// daemon path — genesis is minted by AppendGenesis, which runs no consensus round, and the
	// daemon seeds it BEFORE nd.EnableChain, so no proposer ever sees an empty chain. Bounded, not
	// closed, and it is JOIN mode (a later move) that would make it live again.
	if got := c.MintVersion(0); got == chain.BlockVersionWitnessable {
		t.Fatalf("the height-0 residual has CHANGED: MintVersion(0) now returns era 4 (v%d). That is a "+
			"different chain from the one the era-floor reasoning was written against — re-derive it rather "+
			"than deleting this assertion", got)
	}
	// NON-VACUITY: a LATE boundary must leave those heights below the floor, or the loop above
	// could not discriminate and would pass for any configuration.
	late := chain.New(chain.Config{Quorum: 1, Era3ActivationHeight: 64, Era4ActivationHeight: 64},
		func(ports.NodeID) int64 { return 0 })
	if got := late.MintVersion(8); got >= chain.BlockVersionWitnessable {
		t.Fatalf("NON-VACUITY BROKEN: with Era4ActivationHeight=64, height 8 mints v%d — the probe cannot "+
			"tell an empty interval from a non-empty one", got)
	}
}

func daemonSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}
