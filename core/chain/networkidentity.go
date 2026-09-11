package chain

import (
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// THE NETWORK'S IDENTITY, AND THE ONE RULE THAT GOVERNS HOW IT IS SHOWN.
//
// A node must report its network by BOTH a cryptographic identifier and a canonical text name
// (owner requirement, 2026-09-11). The two are not interchangeable and the asymmetry is the whole
// design:
//
//	THE HASH IS THE IDENTITY. THE NAME IS A LABEL.
//
// The genesis hash is self-authenticating and unforgeable: a node that disagrees about it cannot
// join at all (Reconcile refuses the fork with ErrForeignGenesis). The name is operator-chosen
// text. COLLISIONS ARE NOT PREVENTABLE AND ARE NOT MEANT TO BE — two networks may pick "silt
// mainnet"; they cannot pick the same genesis hash.
//
// SO THE NAME IS NEVER DISPLAYED WITHOUT THE TAG, and that is enforced structurally rather than by
// policy. NetworkIdentity is the ONLY way the name leaves the chain: there is deliberately no
// (*Chain).NetworkName accessor and no getter that returns the bare string. A render site cannot
// print a name without its tag without reaching past this function into ConsensusParams, which is
// a visible act rather than an omission.
//
// This is not cosmetic. A typo'd flag on a fresh store founds a network of ONE that reports
// healthy (cmd/silt/daemon.go's own comment: relative to the mint, CheckConsensusParams is a
// tautology). Adding a committed NAME makes that singleton worse, because it now reports a
// confident operator-chosen name — unless the tag travels with it, which is the only thing that
// makes the singleton legible to its operator. The fix for the singleton itself is JOIN/START
// mode, which is a later move; the display rule is what keeps this move from making it harder to
// see in the meantime.

// networkTagWidth is how many leading hex characters of the genesis hash the SHORT tag shows.
//
// It is a DISPLAY width and nothing else. Nothing compares tags, nothing routes on them, and no
// security property rests on the prefix being collision-free — the full hash is always printed
// beside it by NetworkIdentityLines. 12 hex characters is the git-abbreviation convention: enough
// for a human to tell two networks apart at a glance in a log line.
const networkTagWidth = 12

// unnamedNetwork is how an EMPTY NetworkName renders. The zero is narrated, never blank: a blank
// where a name belongs is indistinguishable from a rendering bug, which is the vacuity every
// observable in this repo is held to (the anti-vacuity bar — absent and zero must be
// structurally distinguishable).
const unnamedNetwork = "(unnamed)"

// NetworkIdentity renders this chain's identity as ONE string carrying BOTH halves. It is the
// only accessor for the committed name.
//
// The empty chain case is a real state, not an error: a daemon whose store is empty holds a chain
// with no genesis, so it has no identity to report at all. It says so rather than rendering a zero
// hash, which would be a measurement-shaped non-measurement.
func (c *Chain) NetworkIdentity() string {
	if len(c.blocks) == 0 {
		return "no genesis is loaded, so this node has NO network identity yet — neither a name nor a tag"
	}
	return NetworkIdentityOf(c.committedNetworkName(), c.ChainID())
}

// committedNetworkName reads the name off the GENESIS, never off c.cfg. Reading the local config
// would report the operator's own input back, which is the vacuity a committed field exists to
// close. A genesis that predates the bind carries no params and is therefore unnamed.
func (c *Chain) committedNetworkName() string {
	if len(c.blocks) == 0 || c.blocks[0].Params == nil {
		return ""
	}
	return c.blocks[0].Params.NetworkName
}

// NetworkIdentityOf is the rendering rule itself, separated from the chain so it can be driven
// directly over the whole (name, tag) space including the empty name.
//
// The full hash is present in every rendering. The short tag is a convenience placed FIRST because
// that is what a human scans; the full hash follows so no reader has to trust the prefix.
func NetworkIdentityOf(name string, id ports.Hash) string {
	shown := name
	if shown == "" {
		shown = unnamedNetwork
	}
	full := id.String()
	tag := full
	if len(tag) > networkTagWidth {
		tag = tag[:networkTagWidth]
	}
	return fmt.Sprintf("%s [%s] (genesis %s)", shown, tag, full)
}

// NetworkIdentityLines is the daemon's start-up report of WHICH NETWORK this node is on. It is the
// name/tag twin of StartupEraLines and is printed from the same place, after the replay and after
// any genesis seed, so both halves describe the chain this daemon will actually serve.
func NetworkIdentityLines(c *Chain) []string {
	lines := []string{"network: " + c.NetworkIdentity()}
	if len(c.blocks) == 0 {
		return lines
	}
	return append(lines, "network:   the genesis hash is the IDENTITY and the name is a LABEL: two networks may "+
		"choose the same name, and cannot choose the same genesis. Match the hash out-of-band before trusting "+
		"the name.")
}
