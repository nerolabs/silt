package node

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// chainlessClient builds the shape cmd/silt's `swarm add` builds: a node with no chain at all.
func chainlessClient(t *testing.T, seed int64) *Node {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	id := identity.FromSeed(seed)
	return New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
}

// chainHolder builds a node that HOLDS a chain, so its network identity is derived from genesis.
// It returns the node and the genesis hash the chain derives.
func chainHolder(t *testing.T, seed int64) (*Node, ports.Hash) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	id := identity.FromSeed(seed)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis")}}
	chain.Sign(g, id.Signer())
	ch := chain.New(chain.Config{Quorum: 1, Anchors: map[ports.NodeID]bool{id.NodeID(): true}},
		func(ports.NodeID) int64 { return 0 })
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.EnableChain(ch, id.Signer())
	return nd, ch.ChainID()
}

// TestChainlessClientCarriesADeclaredNetworkIdentity is the whole point of the seam: a client that
// holds no chain can still say which network it is on, in 32 bytes, with no enable/reconcile/store.
// Before SetNetworkIdentity existed there was no route at all — RequesterChainID's only possible
// answer on such a node was the zero hash.
func TestChainlessClientCarriesADeclaredNetworkIdentity(t *testing.T) {
	nd := chainlessClient(t, 9101)
	if got := nd.RequesterChainID(); got != (ports.Hash{}) {
		t.Fatalf("an undeclared chainless client must read the ZERO hash, got %s", got)
	}
	want := ports.HashBytes([]byte("some network's genesis"))
	if err := nd.SetNetworkIdentity(want); err != nil {
		t.Fatalf("SetNetworkIdentity on a chainless node: %v", err)
	}
	if got := nd.RequesterChainID(); got != want {
		t.Fatalf("RequesterChainID = %s, want the declared %s", got, want)
	}
}

// TestDeclaringANetworkIdentityDoesNotMoveTheVerifierSideChainID is the SAFETY gate, and it is the
// reason the two accessors are separate functions. chainID is the value this node JUDGES under; a
// caller-supplied value there is a wrong-accept (cross-network replay) and it is what keeps a
// chainless node from minting an era-4 signature a peer would take. A declaration must reach the
// requester side and nothing else.
//
// ABLATION (drives this RED): make chainID fall back to n.declaredChainID when n.chain == nil.
func TestDeclaringANetworkIdentityDoesNotMoveTheVerifierSideChainID(t *testing.T) {
	nd := chainlessClient(t, 9102)
	declared := ports.HashBytes([]byte("a network this client was told about"))
	if err := nd.SetNetworkIdentity(declared); err != nil {
		t.Fatal(err)
	}
	if got := nd.chainID(); got != (ports.Hash{}) {
		t.Fatalf("chainID moved to %s after a DECLARATION — the verifier-side identity must stay the zero hash on a chainless node", got)
	}
	// Anti-vacuity: the declaration did land, one accessor over. Without this the test would pass
	// on a build where SetNetworkIdentity silently stored nothing.
	if got := nd.RequesterChainID(); got != declared {
		t.Fatalf("the declaration did not land at all: RequesterChainID = %s, want %s", got, declared)
	}
}

// TestSetNetworkIdentityRefusesAChainHolder: a node with a chain DERIVES its identity from genesis.
// Letting a flag override it would put a caller-supplied value on the wrong-accept side.
func TestSetNetworkIdentityRefusesAChainHolder(t *testing.T) {
	nd, genesis := chainHolder(t, 9103)
	other := ports.HashBytes([]byte("a different network"))
	err := nd.SetNetworkIdentity(other)
	if !errors.Is(err, ErrNetworkIdentityDerived) {
		t.Fatalf("SetNetworkIdentity on a chain-holder = %v, want ErrNetworkIdentityDerived", err)
	}
	if got := nd.RequesterChainID(); got != genesis {
		t.Fatalf("a chain-holder must read its DERIVED identity %s, got %s", genesis, got)
	}
}

// TestSetNetworkIdentityRefusesTheZeroHash: the zero hash already means "I do not know which
// network I am on". Accepting it would report success and change nothing — the silent shape.
func TestSetNetworkIdentityRefusesTheZeroHash(t *testing.T) {
	nd := chainlessClient(t, 9104)
	if err := nd.SetNetworkIdentity(ports.Hash{}); !errors.Is(err, ErrNetworkIdentityZero) {
		t.Fatalf("SetNetworkIdentity(zero) = %v, want ErrNetworkIdentityZero", err)
	}
}

// TestAChainOutranksADeclaration pins the precedence: a node that acquires a chain reads the
// chain's own genesis hash, never an earlier declaration. Consumer 3 of the certification (a
// JOINING daemon) depends on this — its refusal window is transient precisely because the chain
// takes over the moment it exists.
func TestAChainOutranksADeclaration(t *testing.T) {
	nd, genesis := chainHolder(t, 9105)
	nd.declaredChainID = ports.HashBytes([]byte("a stale declaration"))
	if got := nd.RequesterChainID(); got != genesis {
		t.Fatalf("RequesterChainID = %s, want the chain's own %s — a chain-holder must never read a declaration", got, genesis)
	}
}
