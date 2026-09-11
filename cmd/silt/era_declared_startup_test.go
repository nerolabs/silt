package main

import (
	"crypto/ed25519"
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// THE DECLARED-ERA GATES, cmd tier (freeze manifest item 19, second clause: "the daemon prints its
// declared max block era at start-up").
//
// WHAT THIS TIER COVERS AND WHAT IT DOES NOT. The RENDER and the four-cell {build} × {chain} matrix
// are gated one tier down, on chains that produce their own v5 blocks and their own readiness latch
// (core/chain/eradeclared_test.go). No cmd-tier fixture can drive a real era-4 latch — chain.
// NewBondReg hard-codes the readiness stamp because it is a property of the binary rather than a
// caller's choice — so this tier gates the two things that only exist here: that the DAEMON's
// number comes from the build and not from anything an operator types, and that the daemon actually
// calls the renderer on its start-up path.

// startupChain builds a real chain.Chain under the given consensus config and seeds a signed
// genesis, the way the daemon's own start-up path leaves it. The config is the ablation surface:
// every value here is one an operator sets with a flag.
func startupChain(t *testing.T, cfg chain.Config) *chain.Chain {
	t.Helper()
	c := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	chain.Sign(g, ed25519.NewKeyFromSeed(make([]byte, 32)))
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c
}

// TestEraStartupLinesDeclareTheBUILDNotTheFlags is GATE G-DE-7, the flag ablation.
//
// A declared era an operator can set answers nothing: an operator diagnosing a dark network would
// be reading their own input back, and 13b's discrimination — healthy dark versus wrong build —
// would be decided by the wrong party. The daemon's entry point therefore takes a chain and nothing
// else, and this drives that: two chains under consensus configs that disagree on every flag an
// operator can reach must declare the SAME era, character for character.
//
// The structural half of the same claim is asserted at COMPILE time in core/chain (a const
// declaration takes a constant expression and nothing else), which is the only place the claim can
// be made without qualification. This half covers the wiring: a constant is worth nothing if the
// daemon reads something else.
func TestEraStartupLinesDeclareTheBUILDNotTheFlags(t *testing.T) {
	a := eraStartupLines(startupChain(t, chain.Config{Quorum: 1, MinBond: 1 << 20, EpochBlocks: 4, BondTTLBlocks: 64}))
	b := eraStartupLines(startupChain(t, chain.Config{
		Quorum: 99, MinBond: 1 << 30, EpochBlocks: 4096, BondTTLBlocks: 7,
		Era3ActivationHeight: 999999, Era4ActivationHeight: 999999,
	}))
	t.Logf("declared line under the default config:\n%s\ndeclared line under a divergent config:\n%s",
		strings.Join(a, "\n"), strings.Join(b, "\n"))

	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Fatalf("G-DE-7 RED: the start-up era render MOVED with consensus config. The declared era "+
			"would then be what an operator typed.\nDEFAULT:\n%s\nDIVERGENT:\n%s",
			strings.Join(a, "\n"), strings.Join(b, "\n"))
	}
	if strings.Contains(strings.Join(a, "\n"), "999999") {
		t.Fatal("G-DE-7 RED: a local config value reached the render verbatim")
	}

	// The number is the build's, and it is the SHIPPED one. Asserting the literal (not just
	// equality with the constant) is what makes this gate notice a stamp raise that ships
	// without moving the declaration — the two must travel together.
	if got := strings.Join(a, "\n"); !strings.Contains(got, "declares era-4 (v5)") {
		t.Fatalf("G-DE-7 RED: this build declares chain.DeclaredMaxBlockVersion = v%d, and the "+
			"start-up line must say so; got:\n%s", chain.DeclaredMaxBlockVersion, got)
	}

	// AND THE RENDER IS SENSITIVE TO THE NUMBER. Without this, the assertion above passes on a
	// render that hard-codes "era-4" and ignores its argument — which would print the same line
	// on the era-2 build 13b needs to tell apart. The contrast build is not the shipped one, so
	// it is constructed here rather than observed; what it proves is about the RENDERER, and the
	// shipped path's own number is proved above.
	if same := strings.Join(a, "\n"); same == chain.StartupEraLines(chain.BlockVersionRounds, startupChain(t, chain.Config{Quorum: 1}).EraState())[0] {
		t.Fatal("G-DE-7 RED: an era-2 build renders the same declaration line as this one")
	}
}

// TestDaemonPrintsTheEraPairAtStartUp is GATE G-DE-8 — the WIRING.
//
// A renderer nothing calls is decoration, which is the exact shape this row absorbs (its
// predecessor's code half asserted nothing because an early return always fired). So the call site
// is gated, not assumed.
//
// THIS IS A SOURCE GATE, AND THE LIMIT IS NAMED RATHER THAN GLOSSED: it reads daemon.go as TEXT and
// can therefore see only a string and an order. It proves the call is in the start-up path's source
// and sits after the two loads it describes; it does NOT prove the call is REACHED at run time — a
// new early return above it would be invisible here (instance 2 of the observable-log-contract
// scar).
//
// UNGATED: no test in this repo boots the real daemon, so NOTHING observes these lines at runtime.
// The e2e tier is where that would live, and a start-up log line did not justify a spawned-process
// fixture. What this gate does make impossible is the silent deletion of the call while every unit
// test stays green; the deletion of the STRINGS is covered by TestObservableContractStringsAreStillEmitted.
func TestDaemonPrintsTheEraPairAtStartUp(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "for _, ln := range eraStartupLines(ch) {") {
		t.Fatal("SOURCE GATE: G-DE-8 RED — the literal `for _, ln := range eraStartupLines(ch) {` is " +
			"no longer in daemon.go's text, so the daemon no longer prints the era pair at start-up. " +
			"The renderer is then decoration: freeze manifest item 19 asks the DAEMON to print its " +
			"declared max block era, and cloud row 13b reads it off the daemon's log.")
	}
	// It must run where a chain exists and is fully loaded. Ahead of the replay it reports an
	// empty chain on every restart; ahead of the genesis seed, on every fresh node.
	call := strings.Index(s, "for _, ln := range eraStartupLines(ch) {")
	for _, before := range []string{"chainstore.Recover(chainPath, ch, *acceptChainLoss)", "genesis.Build(store, &gp)"} {
		if at := strings.Index(s, before); at < 0 || at > call {
			t.Fatalf("SOURCE GATE: G-DE-8 RED — in daemon.go's TEXT the era-pair call appears BEFORE "+
				"%q, so at run time it would describe a chain the daemon has not finished loading. "+
				"This checks ORDER in the source, not execution order.", before)
		}
	}
}

// TestEraStartupLinesAreRegisteredObservables is GATE G-DE-9.
//
// The start-up markers are an operator interface: cloud row 13b reads them off the daemon's log,
// which is what "an announced log line is an OBSERVABLE CONTRACT" (S5) means. Registering them puts
// their deletion under TestObservableContractStringsAreStillEmitted, so a later change cannot make
// a build green by dropping the line that carries the answer.
func TestEraStartupLinesAreRegisteredObservables(t *testing.T) {
	for _, want := range []string{"chain: era support — this BUILD declares", "AHEAD — this chain has not activated"} {
		found := false
		for _, e := range ObservableContract {
			if e.Marker == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("G-DE-9 RED: the start-up marker %q is not in ObservableContract, so nothing "+
				"stops a later change from deleting it to make a build green", want)
		}
	}
}
