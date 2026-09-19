package main

import (
	"time"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// The whole-operation cap on one client publish or retrieval. Past it the client
// gives up on itself and reports a timeout rather than waiting on a swarm that
// is not going to answer.
//
// It is DERIVED, and the derivation is the point. An outer cap is a backstop
// against a hang; it must never be the thing that ends a step which is still
// inside its own budget, because then two things go wrong at once: a healthy but
// slow commit is reported as a failure, and the specific diagnosis the inner
// step was about to produce is replaced by the outer cap's generic one. The
// registry's own budget states the rule — "a client window below the chain's
// in-spec height cost manufactures failure verdicts for healthy commits" — and a
// typed outer cap is exactly how a build ends up violating it.
//
// The longest step a publish wraps is the registry's accept-to-commit budget,
// itself derived from the synchronizer's per-height escape bound. Ahead of that
// step the client does its own sequential work — bootstrap into the swarm, fetch
// the canonical issuer ranking, gather the publish token, scatter and confirm —
// and each of those legs is bounded by the client's own per-RPC worst case, the
// same figure it reports in its fetch posture. So:
//
//	ceiling = registry accept-to-commit budget + preCommitLegs x per-RPC worst case
//
// Nothing in it is a literal except the leg count, which is named below.
const preCommitLegs = 4 // bootstrap, issuer ranking, token gather, scatter+confirm

func swarmClientOperationCeiling() time.Duration {
	perRPC := time.Duration(node.DeriveFetchPosture(node.SwarmClientConfig(), 0).PerRPC)
	return httpregistry.PublishCommitBudget() + preCommitLegs*perRPC
}

// swarmClientCeiling is the same value in the units core speaks.
func swarmClientCeiling() ports.Duration {
	return ports.Duration(swarmClientOperationCeiling())
}
