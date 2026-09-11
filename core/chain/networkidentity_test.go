package chain

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// G-NAME-1 — THE NAME IS NEVER DISPLAYED WITHOUT THE TAG.
// =============================================================================
//
// The certification made this a GATE rather than a policy sentence (G-2c) for a reason silt has
// now watched decay four times in one month: a rule that lives only in prose is not a rule. The
// composition it guards is real — a committed NAME makes the silent-singleton failure WORSE,
// because a network of one founded by a typo'd flag now reports a confident operator-chosen name.
// The tag travelling with it is the only thing that makes that legible to its operator.
//
// EACH CONDITION IS DRIVEN ONE AT A TIME, and every arm carries its own ablation in-process.

// G-NAME-1a — over the whole (name, tag) space, including the two shapes that break naive
// renderers: the EMPTY name, and a name that is itself hash-shaped.
func TestGNAME1_TheNameNeverRendersWithoutTheTag(t *testing.T) {
	ids := []ports.Hash{
		ports.HashBytes([]byte("network X genesis")),
		ports.HashBytes([]byte("network Y genesis")),
		{}, // the zero hash: a chain with no genesis has no identity, and must not pretend to
	}
	names := []string{
		"",                  // unnamed — the zero must be NARRATED, never blank
		"silt mainnet",      // the ordinary case
		"   ",               // whitespace: visually empty but not the empty string
		ids[0].String(),     // a name that IMPERSONATES a tag
		"silt [deadbeef01]", // a name carrying tag-shaped punctuation
	}
	for _, id := range ids {
		for _, name := range names {
			got := NetworkIdentityOf(name, id)
			// THE RULE. The FULL hash is what a reader can act on, so it is what is asserted —
			// asserting the short tag alone would pass on a renderer that printed only a prefix.
			if !strings.Contains(got, id.String()) {
				t.Fatalf("G-NAME-1 VIOLATED: NetworkIdentityOf(%q, %s) = %q — it does NOT carry the genesis "+
					"hash. The hash is the IDENTITY and the name is a LABEL; a name shown without its tag "+
					"invites an operator to trust a string two networks can both choose", name, id, got)
			}
			// AND THE NAME MUST SURVIVE, or the rule would be satisfied by printing the tag alone
			// and the owner's requirement (report BOTH) would be half-met by a gate that says so.
			want := name
			if strings.TrimSpace(name) == "" && name == "" {
				want = unnamedNetwork
			}
			if !strings.Contains(got, want) {
				t.Fatalf("G-NAME-1 VACUOUS THE OTHER WAY: NetworkIdentityOf(%q, %s) = %q dropped the NAME. "+
					"The requirement is BOTH identifiers, so a tag-only rendering fails it too", name, id, got)
			}
		}
	}

	// ABLATION, in-process: the NAME-ONLY rendering this gate exists to forbid, run through the
	// very same predicate the arm above uses. If that predicate cannot fail on it, it is
	// decoration and every green above is worthless.
	id := ids[0]
	ablated := 0
	for _, name := range names {
		nameOnly := name
		if nameOnly == "" {
			nameOnly = unnamedNetwork
		}
		if strings.Contains(nameOnly, id.String()) {
			// The tag-impersonating name genuinely contains the hash, so it cannot demonstrate an
			// absence. It is EXCLUDED and counted, never quietly skipped — see the floor below.
			continue
		}
		if strings.Contains(nameOnly, id.String()) {
			t.Fatalf("ABLATION DID NOT APPLY: the name-only rendering of %q still contains the tag", name)
		}
		ablated++
	}
	// A FLOOR ON THE ABLATION ITSELF. The `continue` above could, with a different fixture, skip
	// every case and leave this "ablation" having demonstrated nothing while reporting green —
	// which is exactly the vacuous-ablation shape. Demand that most of the table actually ran.
	if ablated < len(names)-1 {
		t.Fatalf("THE ABLATION IS VACUOUS: only %d of %d names were actually ablated; the rest were "+
			"excluded as tag-impersonating. An ablation that skips its own cases proves nothing", ablated, len(names))
	}
	t.Logf("G-NAME-1 ablation RED as required on %d/%d names: a name-only rendering carries no genesis "+
		"hash, so the assertion above discriminates", ablated, len(names))
}

// G-NAME-1b — the ZERO is narrated. An empty name must not render as a blank, because a blank
// where a name belongs is indistinguishable from a rendering bug (the anti-vacuity bar: absent and
// zero must be structurally distinguishable).
func TestGNAME1_TheEmptyNameIsNarratedNotBlank(t *testing.T) {
	id := ports.HashBytes([]byte("some genesis"))
	got := NetworkIdentityOf("", id)
	if !strings.Contains(got, unnamedNetwork) {
		t.Fatalf("the empty name must render as %q, got %q — a blank is a measurement-shaped non-measurement",
			unnamedNetwork, got)
	}
	// NON-VACUITY: a REAL name must not render as the unnamed marker, or the assertion above is
	// satisfied by a renderer that says "(unnamed)" for everything.
	if named := NetworkIdentityOf("silt mainnet", id); strings.Contains(named, unnamedNetwork) {
		t.Fatalf("a NAMED network rendered the unnamed marker: %q", named)
	}
}

// G-NAME-1c — the name is read off the GENESIS, not off local config. This is the anti-vacuity
// argument the whole field rests on: an operator reading their own -network-name back learns
// nothing about the network they are on.
func TestGNAME1_TheNameIsReadFromTheChainNotTheConfig(t *testing.T) {
	const committed, local = "the network this chain actually is", "what this operator typed"
	if committed == local {
		t.Fatal("VACUOUS: the two names are equal, so 'read from the chain' is indistinguishable from " +
			"'read from the config'")
	}
	// The chain COMMITS one name; its live Config carries a DIFFERENT one. Only a read off
	// blocks[0].Params can tell them apart.
	c := New(Config{Quorum: 1, NetworkName: local}, func(ports.NodeID) int64 { return 0 })
	p := ParamsFromConfig(Config{Quorum: 1, NetworkName: committed}, 64, 100)
	g := Block{Version: BlockVersionRounds, Height: 0, Entries: []ports.Entry{entry(1)}, Params: &p}
	Sign(&g, key(88001))
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	got := c.NetworkIdentity()
	if !strings.Contains(got, committed) {
		t.Fatalf("NetworkIdentity() = %q — it does not report the COMMITTED name %q. A name read from "+
			"anywhere but the chain reports a belief, which is the eradeclared.go vacuity", got, committed)
	}
	if strings.Contains(got, local) {
		t.Fatalf("NetworkIdentity() = %q — it leaked the LOCAL config name %q. That is the operator "+
			"reading their own input back", got, local)
	}
	if !strings.Contains(got, c.ChainID().String()) {
		t.Fatalf("NetworkIdentity() = %q carries no genesis hash", got)
	}
}

// G-NAME-1d — an EMPTY chain reports NO identity rather than a zero-hash one. A chain with no
// genesis genuinely has no network; rendering the zero hash would be a measurement-shaped
// non-measurement, the same failure StartupEraLines refuses for an unloaded chain.
func TestGNAME1_AnEmptyChainHasNoIdentity(t *testing.T) {
	c := New(Config{Quorum: 1}, func(ports.NodeID) int64 { return 0 })
	if c.Len() != 0 {
		t.Fatalf("VACUOUS: the fixture chain is not empty (%d blocks)", c.Len())
	}
	got := c.NetworkIdentity()
	if strings.Contains(got, (ports.Hash{}).String()) {
		t.Fatalf("an empty chain rendered the ZERO hash as a tag: %q. A zero chain id is not a chain "+
			"(verifyAtt's discipline), and a node with no genesis has no network to name", got)
	}
	if !strings.Contains(got, "NO network identity") {
		t.Fatalf("an empty chain must SAY it has no identity, got %q", got)
	}
	// And the start-up lines must not append the name/tag advice to a node that has neither.
	if lines := NetworkIdentityLines(c); len(lines) != 1 {
		t.Fatalf("an empty chain must report ONE line, got %d: %v", len(lines), lines)
	}
	// NON-VACUITY: a chain WITH a genesis reports the full pair.
	p := ParamsFromConfig(Config{Quorum: 1, NetworkName: "named"}, 64, 100)
	g := Block{Version: BlockVersionRounds, Height: 0, Entries: []ports.Entry{entry(1)}, Params: &p}
	Sign(&g, key(88002))
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if lines := NetworkIdentityLines(c); len(lines) != 2 {
		t.Fatalf("a seeded chain must report TWO lines (identity + the hash-is-the-identity rule), got %d: %v",
			len(lines), lines)
	}
}

// G-NAME-2 — THE NAME MOVES THE GENESIS HASH, so two networks differing only by name are
// different networks. Without this the label would be decoration: nodes would agree on the genesis
// and merely disagree about what to call it.
func TestGNAME2_TheNameMovesTheGenesisHash(t *testing.T) {
	mk := func(name string) ports.Hash {
		p := ParamsFromConfig(Config{Quorum: 3, MinBond: 1 << 20, NetworkName: name}, 64, 100)
		b := Block{Version: BlockVersionRounds, Height: 0, Entries: []ports.Entry{entry(1)}, Params: &p}
		return b.Hash()
	}
	a, b := mk("silt mainnet"), mk("silt testnet")
	if a == b {
		t.Fatal("G-NAME-2 VIOLATED: two networks differing ONLY by name computed the SAME genesis hash, so " +
			"the name is hash-covered decoration and ErrForeignGenesis would let them join each other")
	}
	// NON-VACUITY: the SAME name must reproduce the SAME hash, or the assertion above is satisfied
	// by a hash that is simply unstable.
	if mk("silt mainnet") != a {
		t.Fatal("the genesis hash is not a function of the params — the assertion above proves nothing")
	}
}
