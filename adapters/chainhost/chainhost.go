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
	h.Loop.Post("commit", func() { fn(func() { close(ch) }) })
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

func (h *Host) Publish(ctx context.Context, e ports.Entry) error {
	// #441 (certified 2026-08-16): publish is SUBMIT-then-poll-for-finality, never
	// propose-then-gather. The old ProposeEntry client raced the drain designee for
	// the same (h, r0) prepare slots and could win no round of any height (zero
	// entry-blocks post-latch on run a56ac10-42834); as mempool content the entry
	// rides whichever designee block commits. B7/S3 hold exactly as before: this
	// returns nil only once the entry is READ BACK from the committed chain.
	if err := h.PublishAsync(ctx, e); err != nil {
		return err
	}
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		committed, ferr := h.PublishStatus(ctx, e.Root)
		if committed {
			return nil
		}
		if ferr != nil {
			return ferr
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("chainhost: entry %s not committed within %s (submitted to the mempool; still pending)", e.Root, timeout)
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
		h.outcomes.Delete(e.Root)
		if h.Node.Chain().Objective() {
			// Accepted: SUBMIT to the entry mempool (#441 — the certified fix).
			// The entry enters this validator's own pending queue and is
			// broadcast to the other eligible proposers; whichever designee
			// commits the next block folds it in (FIFO, separate entry byte
			// budget), and the escape rounds carry it too. No per-attempt
			// terminal failure exists in the submit world — inclusion is read
			// from the chain by Lookup/PublishStatus, and a client re-submission
			// (a fresh PublishAsync) is a mempool-dedup no-op that re-broadcasts
			// to any proposer that lost it.
			h.Node.SubmitEntry(e, h.Broadcast)
			done()
			return
		}
		// LEGACY (subjective, -objective=false) keeps the direct propose path:
		// there is no designee sweep and no round machinery to drive the
		// mempool there — and no drain contention either, so the #441
		// starvation cannot arise. The certified submit path is
		// objective-mode-scoped by its own premise.
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
