package chain

import (
	"context"

	"github.com/nerolabs/silt/ports"
)

// ReplicaRegistry exposes a chain replica through the ports.Registry
// seam — the promise from M1, kept: readers (pipeline.Get, NetGet)
// never learn the registry became a blockchain. Publishing through a
// replica is refused; entries enter the chain only via consensus
// (node.ProposeEntry).
//
// HonorRevocations makes this reader subscribe to the chain's takedown
// records. It defaults to false — a bare registry resolves EVERYTHING the
// chain committed, so following the chain never silently imposes someone
// else's takedowns. An operator that chooses to honor on-chain revocations
// sets it true; the effect is then "proportional to who trusts you" (TENETS
// §9), the same voluntary, per-operator stance as the local denylist — never
// a global switch that every chain-follower inherits.
//
// Curators names the curators this reader follows (curator.go): a root any of them lists is taken
// down for this reader. HonorRevocations covers only takedowns committed before curators existed,
// which name no one.
type ReplicaRegistry struct {
	C                *Chain
	HonorRevocations bool
	Curators         []ports.NodeID
}

func (r ReplicaRegistry) takenDown(root ports.Hash) bool {
	return (r.HonorRevocations && r.C.Revoked(root)) || r.C.ListedBy(root, r.Curators)
}

var _ ports.Registry = ReplicaRegistry{}

func (r ReplicaRegistry) Publish(context.Context, ports.Entry) error {
	return ErrUseConsensus
}

func (r ReplicaRegistry) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	if r.takenDown(root) {
		return ports.Entry{}, false, nil // taken down for THIS subscribing reader
	}
	e, ok := r.C.LookupRoot(root)
	return e, ok, nil
}

func (r ReplicaRegistry) All(context.Context) ([]ports.Entry, error) {
	all := r.C.AllEntries()
	if !r.HonorRevocations && len(r.Curators) == 0 {
		return all, nil
	}
	kept := all[:0]
	for _, e := range all {
		if !r.takenDown(e.Root) {
			kept = append(kept, e)
		}
	}
	return kept, nil
}
