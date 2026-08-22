package node

import (
	"context"
	"testing"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

// #473 — the remaining face of the concurrent-publish 502 class: on a CHAINLESS
// node, the loop-driven sweeps (Care, repairRoot, netGet, Audit, repair-claim
// judging, ColumnHolders) called ports.Registry.Lookup INLINE on the event
// loop; against a network registry that is a blocking HTTP round-trip holding
// the node's single thread for up to the HTTP timeout, per call, per sweep.
// The fix: lookupEntryAsync uses the registry's ports.AsyncRegistry capability
// when it has one (the round-trip runs on the adapter's goroutine, the
// continuation marshals back through the loop), and even the sync fallback
// defers completion (the #467 contract: done never runs on the caller's stack).

// heldRegistry implements ports.Registry + ports.AsyncRegistry and HOLDS every
// async lookup until the test releases it — modeling an HTTP round-trip in
// flight. syncCalls counts blocking Lookup calls made against it (the defect:
// any such call on the loop is the RTT stall).
type heldRegistry struct {
	syncCalls  int
	asyncCalls int
	pending    []func(ports.Entry, bool, error)
}

func (r *heldRegistry) Publish(context.Context, ports.Entry) error { return nil }
func (r *heldRegistry) All(context.Context) ([]ports.Entry, error) { return nil, nil }
func (r *heldRegistry) Lookup(context.Context, ports.Hash) (ports.Entry, bool, error) {
	r.syncCalls++
	return ports.Entry{}, false, nil
}
func (r *heldRegistry) LookupAsync(_ context.Context, _ ports.Hash, done func(ports.Entry, bool, error)) {
	r.asyncCalls++
	r.pending = append(r.pending, done)
}

// release answers every held lookup with "no such entry".
func (r *heldRegistry) release() {
	for _, done := range r.pending {
		done(ports.Entry{}, false, nil)
	}
	r.pending = nil
}

// TestChainlessSweepUsesAsyncRegistryAndKeepsLoopLive473: an Audit on a
// chainless node against an async-capable registry must (a) take the async
// path — zero blocking Lookup calls, (b) leave the LOOP LIVE while the
// round-trip is in flight (a scheduled timer fires before the lookup answers),
// and (c) complete only after the registry answers.
func TestChainlessSweepUsesAsyncRegistryAndKeepsLoopLive473(t *testing.T) {
	n, sched := aloneNode(t, 0)
	reg := &heldRegistry{}
	var root ports.Hash
	root[0] = 0x73

	done := false
	n.Audit(reg, link.CareHandle{Root: root}, func(AuditReport) { done = true })

	ticked := false
	n.clock.AfterFunc(0, func() { ticked = true })
	sched.Run()

	if reg.syncCalls != 0 {
		t.Fatalf("#473: a chainless sweep made %d BLOCKING Lookup call(s) against an async-capable "+
			"registry — on httpregistry that is an HTTP round-trip holding the event loop", reg.syncCalls)
	}
	if reg.asyncCalls != 1 {
		t.Fatalf("#473: expected exactly one async lookup, got %d", reg.asyncCalls)
	}
	if !ticked {
		t.Fatal("#473: the loop did not stay live while the registry round-trip was in flight")
	}
	if done {
		t.Fatal("#473: the sweep completed before the registry answered")
	}

	reg.release()
	sched.Run()
	if !done {
		t.Fatal("#473: the sweep did not complete after the registry answered")
	}
}

// syncOnlyRegistry has no async capability — the in-memory registry shape.
type syncOnlyRegistry struct{ calls int }

func (r *syncOnlyRegistry) Publish(context.Context, ports.Entry) error { return nil }
func (r *syncOnlyRegistry) All(context.Context) ([]ports.Entry, error) { return nil, nil }
func (r *syncOnlyRegistry) Lookup(context.Context, ports.Hash) (ports.Entry, bool, error) {
	r.calls++
	return ports.Entry{}, false, nil
}

// TestSyncRegistryFallbackDefersCompletion473: with a sync-only registry the
// fallback still must not run the continuation on the caller's stack (the #467
// contract) — completion arrives only when the loop drains.
func TestSyncRegistryFallbackDefersCompletion473(t *testing.T) {
	n, sched := aloneNode(t, 0)
	reg := &syncOnlyRegistry{}
	var root ports.Hash
	root[0] = 0x74

	done := false
	n.lookupEntryAsync(reg, root, func(ports.Entry, bool, error) { done = true })
	if done {
		t.Fatal("#473/#467: the sync-fallback lookup completed INLINE on the caller's stack")
	}
	sched.Run()
	if !done {
		t.Fatal("lookup did not complete after draining the loop")
	}
	if reg.calls != 1 {
		t.Fatalf("expected one sync fallback Lookup, got %d", reg.calls)
	}
}
