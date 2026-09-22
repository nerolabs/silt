package main

import (
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/core/node"
)

// The client's whole-operation cap must sit ABOVE every bounded step it wraps.
//
// An outer cap that lands below an inner budget does two bad things at once. It
// ends a step that is still inside spec, so a slow-but-healthy commit is
// reported as a failure — the registry's own budget names that consequence in
// the comment above it. And it replaces a specific diagnosis with a generic one:
// the registry is about to say "accepted but not committed within 6m0s — the
// consensus gather did not finish; see the validators' -log debug", and the
// caller instead hears "swarm operation timed out". A refusal that names the
// wrong cause costs more than one that says nothing.
//
// This is not hypothetical. The cap shipped as a bare 5-minute literal against a
// 6-minute registry budget, so the CLI publish path could never reach the inner
// budget at all: it was cut off 60s early, every time, on exactly the slow paths
// the inner budget exists to tolerate.
func TestTheClientsOuterCapSitsAboveEveryBudgetItWraps(t *testing.T) {
	ceiling := swarmClientOperationCeiling()
	commit := httpregistry.PublishCommitBudget()

	if ceiling <= commit {
		t.Fatalf("the client gives up after %s while the registry's accept-to-commit budget is %s:\n"+
			"a publish can never reach its own budget, so a healthy commit that is merely slow is\n"+
			"reported as a timeout and the registry's specific diagnosis is never printed", ceiling, commit)
	}

	// The headroom is not decorative: the legs BEFORE the commit wait — bootstrap,
	// issuer ranking, token gather, scatter and confirm — run inside the same cap,
	// and each is bounded by the client's own per-RPC worst case. A ceiling that
	// clears the commit budget by less than that can still end a publish whose
	// every step was in spec.
	perRPC := time.Duration(node.DeriveFetchPosture(node.SwarmClientConfig(), 0).PerRPC)
	if want := commit + preCommitLegs*perRPC; ceiling < want {
		t.Fatalf("the cap is %s: it clears the %s commit budget but leaves less than the %d pre-commit\n"+
			"legs the client runs first (%s each), so the work ahead of the commit has no room", ceiling, commit, preCommitLegs, perRPC)
	}

	// And the number REPORTED is the number ENFORCED. The posture line is what an
	// operator reads and what the field harness derives its bound from, so a
	// ceiling that drifts from the one the wait actually uses would hand both a
	// figure that grades nothing.
	if got := time.Duration(node.DeriveFetchPosture(node.SwarmClientConfig(), swarmClientCeiling()).Ceiling); got != ceiling {
		t.Fatalf("the posture reports a %s ceiling while the operation enforces %s", got, ceiling)
	}
}
