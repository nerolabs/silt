package main

// R0.4b C3 final round — the S5 announced-marker gate for `swarm receipt`.
//
// THE FINDING (PE ruling §2 @ 271ab81, G-8 convergence §5, both 2026-09-03).
// `TestDeliveryReceiptRefusedWhenLaneOff` asserts a CLIENT-LEGIBILITY contract: a
// daemon started without -accept-delivery-receipts must refuse, and the client must
// say "NOT banked". The daemon refused correctly; the client did not say it. The
// R0.4b withdrawal lane added a key-resolution step ABOVE the submit, so a lane-off
// peer — which serves no issuer key at all — returns at that step, and the announced
// marker, which lives only below the submit, became unreachable on the one path a
// user actually hits. That is the S5 class (the `freeload: ON` rename scar): an
// announced string is an observable contract, and moving the control flow around it
// breaks the contract just as surely as renaming it.
//
// This is the FAST gate — a pure-function assertion on the refusal builder, catchable
// in milliseconds. The e2e test drives the same property through two real processes.
//
// ABLATION: delete the `errors.Is(keyErr, node.ErrNoIssuerKey)` arm from
// demandKeyResolutionError (so the lane-off case falls through to the generic
// transport-error arm) and this test goes RED on the marker, exactly as e2e does.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestLaneOffRefusalCarriesTheAnnouncedNotBankedMarker(t *testing.T) {
	server := ports.HashBytes([]byte("lane-off-server"))

	// (1) THE LANE-OFF CASE. A peer that runs no demand issuer answers the key
	// request with ErrNoIssuerKey, and the operator must get the announced marker.
	laneOff := demandKeyResolutionError(server, 0, node.ErrNoIssuerKey)
	if laneOff == nil {
		t.Fatal("a peer serving no issuer key must be a refusal, not a nil error")
	}
	if !strings.Contains(laneOff.Error(), notBankedMarker) {
		t.Fatalf("the lane-off refusal does not carry the announced marker %q — e2e "+
			"TestDeliveryReceiptRefusedWhenLaneOff asserts it, and an operator greps "+
			"one phrase, not a taxonomy of causes. Got: %s", notBankedMarker, laneOff)
	}
	// The cause must still unwrap: a wrapper that swallows the sentinel makes the
	// two refusals indistinguishable to any caller that is not a human.
	if !errors.Is(laneOff, node.ErrNoIssuerKey) {
		t.Fatalf("the lane-off refusal does not unwrap to node.ErrNoIssuerKey: %s", laneOff)
	}
	// It must NOT claim a spend. Nothing was withdrawn on this path, so telling the
	// fetcher its token is gone would be a false statement about its money.
	if strings.Contains(laneOff.Error(), "the token is spent regardless") {
		t.Fatalf("the lane-off refusal claims the token is spent, but the refusal happens "+
			"BEFORE any withdrawal — no fee was paid: %s", laneOff)
	}

	// (2) THE ERA-4 CASE stays a DIFFERENT sentence. The issuer served keys; none
	// resolved against a committed E->key binding. That is not "the server is not
	// running the lane", and conflating the two sends an operator to the wrong fix.
	era4 := demandKeyResolutionError(server, 0, nil)
	if era4 == nil {
		t.Fatal("pinned==0 with no transport error must still be a refusal")
	}
	if !errors.Is(era4, errNoCommittedDemandKeyBinding) {
		t.Fatalf("the pinned==0 refusal must name the missing committed binding: %s", era4)
	}
	if strings.Contains(era4.Error(), "serves no demand issuer key") {
		t.Fatalf("the era-4 refusal was rendered as the lane-off refusal: %s", era4)
	}

	// (3) The success arm stays silent.
	if err := demandKeyResolutionError(server, 3, nil); err != nil {
		t.Fatalf("three pinned keys is not a refusal, got %v", err)
	}
}
