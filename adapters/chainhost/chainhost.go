// Package chainhost fronts a validator's chain replica as a
// ports.Registry for HTTP clients — the piece that lets `silt swarm
// add/get` keep working unchanged after the registry becomes a chain.
// Publish triggers a consensus round on the daemon's event loop and
// blocks the (goroutine-per-request) HTTP handler until commit or
// timeout; reads serve straight from the local replica.
package chainhost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

type Host struct {
	Loop      *eventloop.Loop
	Node      *node.Node
	Attesters []ports.NodeID
	Broadcast []ports.NodeID
	Quorum    int
	Timeout   time.Duration

	// outcomes records the TERMINAL failure of an in-flight async gather (root -> error),
	// so PublishAsync's client can stop polling the instant the gather resolves either way —
	// committed (the entry is on the chain) or failed (no quorum this round) — instead of
	// waiting out the whole accept→commit budget. A successful commit is read from the chain,
	// not stored here; only failures are recorded, and each fresh PublishAsync for a root
	// clears the prior attempt's failure (a convergent retry re-proposes the same root).
	outcomes sync.Map // ports.Hash -> error
}

var _ ports.Registry = (*Host)(nil)

func (h *Host) onLoop(fn func(done func())) error {
	ch := make(chan struct{})
	h.Loop.Post(func() { fn(func() { close(ch) }) })
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	select {
	case <-ch:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("chainhost: consensus timed out")
	}
}

func (h *Host) Publish(_ context.Context, e ports.Entry) error {
	var result error
	err := h.onLoop(func(done func()) {
		// Idempotent republish of an identical entry is a free no-op.
		if existing, ok := h.Node.Chain().LookupRoot(e.Root); ok {
			if existing.Root == e.Root && existing.FileSize == e.FileSize {
				done()
				return
			}
			result = ports.ErrDupPublish
			done()
			return
		}
		h.Node.ProposeEntry(e, h.Attesters, h.Broadcast, h.Quorum, func(err error) {
			result = err
			done()
		})
	})
	if err != nil {
		return err
	}
	return result
}

// PublishAsync is the async publish path (#286 Layer 1). It runs the SYNCHRONOUS local
// validation on the loop — returning the refusals a client must learn immediately (no
// publish token when required, a durable Publisher identity the refuse-to-surveil chain
// rejects, a double-spent token, a duplicate root) — then kicks off the commit gather
// FIRE-AND-FORGET and returns. The HTTP handler therefore replies 202 at once instead of
// blocking the whole quorum gather under a flat 10s-client / 30s-server deadline that
// guillotined the ~1.5 MB genesis round; the client polls Lookup until the entry commits.
// Only the slow gather is async — every refusal is still synchronous. nil = accepted.
func (h *Host) PublishAsync(_ context.Context, e ports.Entry) error {
	var vErr error
	err := h.onLoop(func(done func()) {
		if existing, ok := h.Node.Chain().LookupRoot(e.Root); ok {
			if existing.Root == e.Root && existing.FileSize == e.FileSize {
				done() // idempotent republish of an identical entry — already committed
				return
			}
			vErr = ports.ErrDupPublish
			done()
			return
		}
		if vErr = h.Node.ValidateEntryProposal(e); vErr != nil {
			done() // a synchronous refusal — the client gets the reason
			return
		}
		// Accepted: run the commit gather in the BACKGROUND (does not block the handler).
		// Clear any prior terminal failure for this root (a convergent retry re-proposes the
		// same root) and record the gather's terminal failure when it resolves, so a client
		// polling PublishStatus fast-fails on no-quorum instead of waiting out the budget.
		// Success is read from the chain by Lookup/PublishStatus; the gather path also logs
		// the outcome (chainrole.go: "block committed" / "gather: NO QUORUM").
		h.outcomes.Delete(e.Root)
		h.Node.ProposeEntry(e, h.Attesters, h.Broadcast, h.Quorum, func(err error) {
			if err != nil {
				h.outcomes.Store(e.Root, err)
			}
		})
		done()
	})
	if err != nil {
		return err
	}
	return vErr
}

// PublishStatus reports the terminal state of an async publish so the client can stop
// polling the moment the gather resolves — committed (the entry is on the chain) or failed
// (the gather could not reach quorum this round) — rather than waiting out the whole budget.
// A gather still in progress returns (false, nil): pending, keep polling. This is what keeps
// a publish-retry-until-standing caller (bond earned-standing, revocation) able to retry
// promptly while a genuinely slow ~1.5 MB genesis gather (#286) is still allowed to finish.
func (h *Host) PublishStatus(_ context.Context, root ports.Hash) (committed bool, failErr error) {
	if err := h.onLoop(func(done func()) {
		_, committed = h.Node.Chain().LookupRoot(root)
		done()
	}); err != nil {
		return false, nil // loop hiccup — treat as pending; the client polls again
	}
	if committed {
		return true, nil
	}
	if v, ok := h.outcomes.Load(root); ok {
		return false, v.(error)
	}
	return false, nil
}

func (h *Host) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	var e ports.Entry
	var ok bool
	err := h.onLoop(func(done func()) {
		e, ok = h.Node.Chain().LookupRoot(root)
		done()
	})
	return e, ok, err
}

func (h *Host) All(context.Context) ([]ports.Entry, error) {
	var out []ports.Entry
	err := h.onLoop(func(done func()) {
		out = h.Node.Chain().AllEntries()
		done()
	})
	return out, err
}

// Blocks snapshots the replica for persistence.
func (h *Host) Blocks() []chain.Block {
	var out []chain.Block
	h.onLoop(func(done func()) {
		out = h.Node.Chain().Blocks(0)
		done()
	})
	return out
}
