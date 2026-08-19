package chainhost

// The Care/NetGet loop-reentry deadlock (2026-08-19, the flixz concurrent-publish
// 502): core maintenance paths (Care, NetGet, repairRoot, Audit, claim-verify)
// call ports.Registry.Lookup synchronously. When such a path runs ON the node's
// event loop (the UI's auto-caretake, apiFetch, the repair/audit sweeps) and the
// registry is chainhost, Lookup marshals BACK onto the same loop and blocks in a
// select waiting for a task the wedged loop can never run — a reentrant
// post-and-wait self-deadlock that stalls the node's single thread for the whole
// chainhost Timeout, starving every queued message behind it (the three observed
// 502 faces). The fix: a node that carries a chain replica answers these lookups
// from its own committed chain (the very read chainhost would have performed),
// so no loop-context lookup ever round-trips through the adapter.
// Mechanism record: docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// validatorOnLoop builds the composition the daemon runs: a validator node
// (chain replica enabled) whose registry is chainhost over the SAME loop.
func validatorOnLoop(t *testing.T) (*node.Node, *Host, *eventloop.Loop) {
	t.Helper()
	ident, err := identity.Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	tr, err := tcpnet.New(loop, ident, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	nd := node.New(ident.NodeID(), node.DefaultConfig(), walltime.New(loop), tr, memstore.New())
	ch := chain.New(chain.Config{Quorum: 0}, func(ports.NodeID) int64 { return 0 })
	nd.EnableChain(ch, ident.Signer())
	// A short Timeout keeps the RED (deadlocked) run fast: pre-fix, the
	// loop-context Lookup wedges the loop for exactly this long.
	host := &Host{Loop: loop, Node: nd, Timeout: 500 * time.Millisecond}
	return nd, host, loop
}

// TestCareOnLoopDoesNotDeadlock reproduces ui.go's auto-caretake seam
// (s.onLoop(func(){ nd.Care(s.reg, ch) })): Care invoked ON the loop with a
// chainhost registry must complete without wedging the loop for the chainhost
// timeout. RED before the chain-first lookup (elapsed ≈ Timeout), GREEN after
// (elapsed ≈ 0).
func TestCareOnLoopDoesNotDeadlock(t *testing.T) {
	nd, host, loop := validatorOnLoop(t)
	care := link.CareHandle{Root: ports.Hash{0xC4, 0x2E}}

	start := time.Now()
	done := make(chan struct{})
	loop.Post("ui", func() {
		nd.Care(host, care)
		close(done)
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Care on the loop never completed — loop wedged")
	}
	if elapsed := time.Since(start); elapsed >= host.Timeout {
		t.Fatalf("Care on the loop stalled %v (>= chainhost Timeout %v): the reentrant Lookup deadlock", elapsed, host.Timeout)
	}
}

// TestNetGetOnLoopDoesNotDeadlock reproduces ui.go's apiFetch seam
// (loop.Post("api-fetch", func(){ nd.NetGet(s.reg, …) })): the registry lookup
// at the head of NetGet must not wedge the loop. The root is unknown, so NetGet
// reports not-found — but promptly, not after a deadlocked chainhost Timeout.
func TestNetGetOnLoopDoesNotDeadlock(t *testing.T) {
	nd, host, loop := validatorOnLoop(t)
	h := link.Handle{Root: ports.Hash{0xFE, 0x7C}}

	start := time.Now()
	done := make(chan struct{})
	loop.Post("api-fetch", func() {
		nd.NetGet(host, h, discardWriter{}, func(error) { close(done) })
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("NetGet on the loop never completed — loop wedged")
	}
	if elapsed := time.Since(start); elapsed >= host.Timeout {
		t.Fatalf("NetGet on the loop stalled %v (>= chainhost Timeout %v): the reentrant Lookup deadlock", elapsed, host.Timeout)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
