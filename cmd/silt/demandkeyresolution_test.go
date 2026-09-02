package main

// `swarm receipt` refuses to withdraw a demand token unless the issuer's per-epoch
// key resolved against the committed E -> key_E binding. That refusal is correct and
// is NOT relaxed here — but the operator has to be able to read it.
//
// THE BUG THIS PINS (Tester finding, 2026-09-03). The guard was
// `if keyErr != nil || pinned == 0` over a message that formatted keyErr with %w. On
// the pinned==0, keyErr==nil branch — the branch a client hits whenever the chain
// carries no committed binding, which is every chain below the era-4/v5 flip — the
// operator got:
//
//	silt: resolve demand issuer keys from ca7e… against the committed binding (pinned 0): %!w(<nil>)
//
// A verbatim Go formatting artifact, naming no cause. Both branches must name one.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestDemandKeyResolutionErrorIsLegibleOnBothBranches(t *testing.T) {
	server := ports.NodeID(ports.HashBytes([]byte("resolution-test-server")))
	served := errors.New("node: peer has no issuer key")

	t.Run("nothing resolved, no transport error", func(t *testing.T) {
		err := demandKeyResolutionError(server, 0, nil)
		if err == nil {
			t.Fatal("pinned 0 must refuse: withdrawing against an unanchored key is exactly what " +
				"the committed binding exists to prevent")
		}
		msg := err.Error()
		if strings.Contains(msg, "%!") {
			t.Fatalf("the refusal printed a Go formatting artifact instead of a reason: %s", msg)
		}
		if !errors.Is(err, errNoCommittedDemandKeyBinding) {
			t.Fatalf("the pinned-0 branch must carry the no-committed-binding cause, got: %s", msg)
		}
		if !strings.Contains(msg, "committed") {
			t.Fatalf("the reason must name the committed binding, got: %s", msg)
		}
	})

	t.Run("a transport or serving error", func(t *testing.T) {
		err := demandKeyResolutionError(server, 0, served)
		if err == nil {
			t.Fatal("a serving error must refuse")
		}
		msg := err.Error()
		if strings.Contains(msg, "%!") {
			t.Fatalf("the refusal printed a Go formatting artifact instead of a reason: %s", msg)
		}
		if !errors.Is(err, served) {
			t.Fatalf("the served error must stay wrapped for the caller, got: %s", msg)
		}
	})

	t.Run("a resolved key is not an error", func(t *testing.T) {
		if err := demandKeyResolutionError(server, 1, nil); err != nil {
			t.Fatalf("a pinned key must not refuse: %v", err)
		}
	})
}
