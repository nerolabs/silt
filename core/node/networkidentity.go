package node

import (
	"errors"

	"github.com/nerolabs/silt/ports"
)

// THE REQUESTER-SIDE NETWORK IDENTITY — how a CHAINLESS client says which network it is on.
//
// silt holds the network identity as TWO quantities with the same 32 bytes and opposite failure
// directions. They must never be collapsed into one accessor.
//
//   - (*Node).chainID is the VERIFIER's own value, read from its own committed chain and from
//     nothing else. A wrong value there is a WRONG-ACCEPT — it widens what this node admits — so
//     it is never caller-supplied, and it stays the ZERO hash when this node holds no chain so a
//     chainless node convicts nobody on era-4 evidence and mints no era-4 signature a peer would
//     take. NOTHING HERE CHANGES THAT. A declared identity does not reach chainID, and
//     TestDeclaringANetworkIdentityDoesNotMoveTheVerifierSideChainID pins it.
//   - RequesterChainID is the REQUESTER's value, blinded into a blind-token message. A wrong
//     value there can only DENY this requester its own token: every verifier checks under ITS OWN
//     chain id, so a message blinded under any other value verifies nowhere the requester wants.
//     Deny-only is why an operator may declare it.
//
// The binding quantity is the genesis block's hash — (*Chain).ChainID, which is written once
// (AppendGenesis refuses a non-empty chain) and which Reconcile refuses to move (ErrForeignGenesis).
// So it is time-invariant for the life of a network, and a client carries 32 bytes instead of a
// chain. Certified 2026-09-12, ledger D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12.
//
// This is the same shape as ResolvedDemandIssuerKey — a seam built where the value enters, ahead
// of the lane that consumes it. The consumer is the prepaid-credit lane (AcquireCredits), which
// binds the chain id under M3; until that lands, RequesterChainID's production readers are the
// CLI's own round-trip and this package's tests.
var (
	// ErrNetworkIdentityDerived refuses a declaration on a node that holds a chain. Such a node
	// DERIVES its identity from genesis; letting an operator flag override the derived value would
	// turn a wrong flag into a wrong-accept on the verifier side, which is the one direction the
	// split above exists to prevent.
	ErrNetworkIdentityDerived = errors.New("node: this node holds a chain, so its network identity is DERIVED from genesis and cannot be declared")
	// ErrNetworkIdentityZero refuses the zero hash. The zero hash is already this node's "I do not
	// know which network I am on"; accepting it as a declaration would report success and change
	// nothing, which is exactly the silent shape D-TD-3 is about.
	ErrNetworkIdentityZero = errors.New("node: the zero hash is not a network identity — it is this node's \"I do not know which network I am on\"")
)

// SetNetworkIdentity declares which network this chainless node is on, for the blind-token lanes
// that bind it. Refuses a node that holds a chain (ErrNetworkIdentityDerived) and refuses the zero
// hash (ErrNetworkIdentityZero).
func (n *Node) SetNetworkIdentity(id ports.Hash) error {
	if n.chain != nil {
		return ErrNetworkIdentityDerived
	}
	if id == (ports.Hash{}) {
		return ErrNetworkIdentityZero
	}
	n.declaredChainID = id
	return nil
}

// RequesterChainID is the network identity this node BLINDS UNDER: the chain's own genesis hash
// when this node holds a chain, otherwise whatever SetNetworkIdentity declared, otherwise the zero
// hash. A chain always outranks a declaration — a chain-holder never reads an operator's guess.
//
// THE ZERO IT RETURNS IS AMBIGUOUS ON ITS OWN, AND HasNetworkIdentity IS HOW A CONSUMER RESOLVES
// IT. Ask HasNetworkIdentity first; blinding under an undeclared zero is the silent shape
// SetNetworkIdentity refuses one function above.
func (n *Node) RequesterChainID() ports.Hash {
	if n.chain != nil {
		return n.chain.ChainID()
	}
	return n.declaredChainID
}

// HasNetworkIdentity reports whether this node has a requester-side network identity AT ALL —
// derived from a chain, or declared by an operator. It exists because RequesterChainID alone
// cannot meet the bar this repo states one package over, on `unnamedNetwork` in
// core/chain/networkidentity.go:
//
//	the anti-vacuity bar — absent and zero must be structurally distinguishable
//
// RequesterChainID answers the ZERO HASH for an undeclared node, and the zero hash is the exact
// value SetNetworkIdentity REFUSES as a declaration ("it is this node's 'I do not know which
// network I am on'"). Without a second observable a consumer cannot tell the refusal's own
// sentinel from a real identity, so the setter's refusal would be bypassed by the default path and
// the client would blind under 32 zero bytes and learn nothing when it failed.
//
// The consumer is the prepaid-credit lane under M3 (#828), and what this buys it is the ability to
// REFUSE an undeclared client with an actionable message instead of proceeding under zero. That is
// the second half of the 2026-09-11 design option this seam otherwise defers — an explicit
// -chain-id flag, REFUSING when unset — and the refusal belongs on the consumer, so the API has to
// make it expressible.
func (n *Node) HasNetworkIdentity() bool {
	return n.RequesterChainID() != (ports.Hash{})
}
