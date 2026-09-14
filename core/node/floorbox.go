package node

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// THE FLOOR BOX'S ENTRY POINT — a validator that holds no tree, judging one block by proof.
//
// WHAT IT DOES, stated narrowly because the narrowness is the honest part. It takes a block from
// a peer, judges it against the committed state root of a parent the OPERATOR pinned, and reports
// the verdict. It does not follow the chain, and it cannot: (*chain.Box).Validate maps Accept to a
// downgrade, so a box never adopts the block it just judged and never advances its own head. That
// downgrade is one line and removing it is a consensus-rule change; until it is taken, "audit the
// block above the pin" is the whole of what a floor box can be driven to do, and this function is
// exactly that and nothing more.
//
// WHY THE PARENT IS A PIN AND NOT A FETCH. Everything else the box reads is checked against a root
// it already holds, so it may take it from a stranger. The parent is the one thing that is not:
// it IS the root everything else is checked against. A box that accepted whatever parent a peer
// served would be auditing the peer's history rather than the network's. So the parent arrives as
// the operator's weak-subjectivity pin — the same trust class the daemon's -ws-checkpoint already
// carries — and a served block that does not hash to it is refused.

var (
	// ErrFloorBoxPinNotServed means no source served a block at the pinned height that hashes to
	// the pinned hash. It is NOT a verdict about any block: the box has not begun.
	ErrFloorBoxPinNotServed = errors.New("node: floor box — no source served the pinned parent block")
	// ErrFloorBoxNothingAbovePin means the sources are at or below the pin, so there is nothing to
	// audit yet. A benign not-yet, named so an operator can tell it from a refusal.
	ErrFloorBoxNothingAbovePin = errors.New("node: floor box — nothing has been committed above the pin yet")
)

// FloorBoxPin is the operator's trust anchor: the block the box judges from, and the network it
// judges under. The chain id is the genesis hash, which a box holding no blocks cannot derive.
type FloorBoxPin struct {
	Height  uint64
	Hash    ports.Hash
	ChainID ports.Hash
}

// FloorBoxVerdict is one audit's result. Outcome is the box's, verbatim; Err names the stall when
// there is one. Height is the block judged.
type FloorBoxVerdict struct {
	Height  uint64
	Outcome chain.FloorBoxOutcome
	Err     error
}

// WitnessHead asks the providers which committed head they will answer witness requests from, and
// reports where that head sits. It is the box's first act on a cold start, and it does two things
// at once: it tells the box what to pin on, and — because the answering node builds the snapshot
// to answer — it leaves that node holding the state the box is about to ask about.
//
// The request names NO head, which is the one case a server answers from wherever it is. Every
// later request of a validation names the head this one returned, so no two answers in one
// judgement can come from two states.
func (n *Node) WitnessHead(providers []ports.NodeID, done func(head ports.Hash, height uint64, err error)) {
	if len(providers) == 0 {
		done(ports.Hash{}, 0, fmt.Errorf("%w: no witness providers configured", ErrWitnessUnreachable))
		return
	}
	raw, err := cbor.Marshal(witnessReq{Call: witnessCallAncestors, K: 1})
	if err != nil {
		done(ports.Hash{}, 0, fmt.Errorf("node: encode head probe: %w", err))
		return
	}
	var walk func(i int)
	walk = func(i int) {
		if i >= len(providers) {
			done(ports.Hash{}, 0, fmt.Errorf("%w: %d provider(s) tried, none reported a head", ErrWitnessUnreachable, len(providers)))
			return
		}
		n.request(providers[i], ports.Message{Kind: ports.MsgGetWitness, Data: raw},
			func(resp ports.Message, rErr error) {
				if rErr != nil || !resp.OK {
					walk(i + 1)
					return
				}
				r, dErr := decodeWitnessResp(resp.Data, ports.Hash{})
				if dErr != nil || !r.OK || r.Head == (ports.Hash{}) || r.NextHeight == 0 {
					walk(i + 1)
					return
				}
				done(r.Head, r.NextHeight-1, nil)
			})
	}
	walk(0)
}

// AuditAbovePin fetches the block above the pin from `sources`, builds a box over `cold` — a
// config-bearing chain that holds no blocks — anchored on the pinned parent, and judges it against
// witnesses pulled from `providers`. done is called exactly once.
//
// sources and providers are separate lists and may overlap or not. Neither is trusted: a source
// that serves the wrong parent is refused by the pin, and a provider that lies is caught by the
// roots the box already holds.
func (n *Node) AuditAbovePin(cold *chain.Chain, pin FloorBoxPin, budgetBytes int,
	sources, providers []ports.NodeID, done func(FloorBoxVerdict)) {
	if cold == nil {
		done(FloorBoxVerdict{Height: pin.Height + 1, Outcome: chain.IndeterminateTrustlessly,
			Err: errors.New("node: floor box — no config-bearing chain")})
		return
	}
	if len(sources) == 0 {
		done(FloorBoxVerdict{Height: pin.Height + 1, Outcome: chain.IndeterminateTrustlessly,
			Err: fmt.Errorf("%w: no sources configured", ErrFloorBoxPinNotServed)})
		return
	}
	n.fetchAbovePin(sources, 0, pin, func(parent, child *chain.Block, err error) {
		if err != nil {
			done(FloorBoxVerdict{Height: pin.Height + 1, Outcome: chain.IndeterminateTrustlessly, Err: err})
			return
		}
		mkBox := func(src chain.WitnessSource) (*chain.Box, error) {
			return chain.NewBox(cold, *parent, chain.BoxConfig{BudgetBytes: budgetBytes, ChainID: pin.ChainID}, src)
		}
		n.ValidateWithWitnesses(mkBox, *child, providers, pin.Hash,
			func(out chain.FloorBoxOutcome, vErr error) {
				done(FloorBoxVerdict{Height: child.Height, Outcome: out, Err: vErr})
			})
	})
}

// fetchAbovePin walks the source list for a window at the pinned height, returning the pinned
// parent and the block above it. A source that cannot be reached, cannot be decoded, or serves a
// block at the pinned height that does not hash to the pin is stepped past — the pin is what makes
// "the wrong history" a detectable answer rather than a silent substitution.
func (n *Node) fetchAbovePin(sources []ports.NodeID, i int, pin FloorBoxPin,
	done func(parent, child *chain.Block, err error)) {
	if i >= len(sources) {
		done(nil, nil, fmt.Errorf("%w: %d source(s) tried at height %d", ErrFloorBoxPinNotServed, len(sources), pin.Height))
		return
	}
	next := func() { n.fetchAbovePin(sources, i+1, pin, done) }
	n.request(sources[i], ports.Message{Kind: ports.MsgGetChain, Height: pin.Height},
		func(resp ports.Message, err error) {
			if err != nil || !resp.OK {
				next()
				return
			}
			window, dErr := chain.DecodeBlocks(resp.Data)
			if dErr != nil {
				next()
				return
			}
			var parent, child *chain.Block
			for j := range window {
				b := window[j]
				switch {
				case b.Height == pin.Height && b.Hash() == pin.Hash:
					parent = &b
				case b.Height == pin.Height+1 && b.Prev == pin.Hash:
					child = &b
				}
			}
			if parent == nil {
				next() // this source is on another history, or has pruned past the pin
				return
			}
			if child == nil {
				// The pin is served and nothing stands above it. That is a state of the network,
				// not a property of this source, so it ends the walk rather than continuing it.
				done(nil, nil, fmt.Errorf("%w: the pin at height %d is the head", ErrFloorBoxNothingAbovePin, pin.Height))
				return
			}
			done(parent, child, nil)
		})
}
