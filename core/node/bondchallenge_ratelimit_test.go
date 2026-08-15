package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
)

// #424 (red-team seam #7): answering a bond challenge forces a fresh sequential
// VDF-eval on the node's single goroutine, so an unbounded challenger is a remote
// CPU-DoS. allowBondChallenge is the cheap gate in front of the eval: it admits an
// honest cadence with headroom but caps a flood, PER CHALLENGER so a flooder can't
// starve honest challengers of their own budget.
func TestBondChallengeRateLimitPerChallenger(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(1)
	cfg := DefaultConfig() // BondAuditInterval = 60s
	nd := New(ident.NodeID(), cfg, sched, net.Endpoint(ident.NodeID()), memstore.New())

	x := identity.FromSeed(100).NodeID()
	y := identity.FromSeed(200).NodeID()

	// The burst budget of challenges from X is admitted (honest cadence + retries
	// have wide headroom under it)...
	for i := 0; i < bondChallengeBurst; i++ {
		if !nd.allowBondChallenge(x) {
			t.Fatalf("challenge %d/%d from X refused within its burst budget", i+1, bondChallengeBurst)
		}
	}
	// ...the next one, in the same window, is refused — BEFORE any VDF-eval runs.
	if nd.allowBondChallenge(x) {
		t.Fatal("a challenge past the per-window burst must be refused (the DoS gate)")
	}

	// Control 1 — the gate is per-challenger, not global: a different challenger Y
	// still has its full budget while X is capped, so a flood from X cannot deny
	// honest Y's audit. (Proves the refusal above isn't just "refuse everything".)
	if !nd.allowBondChallenge(y) {
		t.Fatal("a distinct challenger must not be charged for X's spent budget")
	}

	// Control 2 — the budget resets after the audit window elapses, so an honest
	// once-per-window challenger is never blocked. (Proves it isn't permanent.)
	sched.AfterFunc(cfg.BondAuditInterval, func() {})
	sched.Run()
	if !nd.allowBondChallenge(x) {
		t.Fatal("X's budget must reset after one BondAuditInterval window")
	}
}
