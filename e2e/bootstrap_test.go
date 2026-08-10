package e2e

import (
	"net"
	"regexp"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
)

// reservePort binds a throwaway listener on an ephemeral port, then frees it, so a
// daemon can be told to dial (and later another to bind) that exact address. There
// is a small window between free and reuse — acceptable for a local test.
func reservePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

var reReBootstrap = regexp.MustCompile(`re-bootstrapped: recovered from an empty routing table \((\d+) table entries\)`)

// #281 over real TCP: silt's Kademlia join is ONE-SHOT, so a node that starts
// before its bootstrap target is listening lands with an EMPTY routing table and,
// without a retry, stays isolated forever. Here B joins through A's address while A
// is DOWN (comes up with 0 table entries), then A starts and B re-bootstraps on its
// own — no restart, no manual intervention.
func TestBootstrapRetryRecoversColdStartRace(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// A's id is deterministic (id-seed) and its port is reserved, so B can name
	// A@addr before A exists.
	idA := identity.FromSeed(2810).NodeID().String()
	addrA := reservePort(t)
	bootstrapA := idA + "@" + addrA

	// B joins through A — but A is not up yet. Fast retry so the test is quick, and
	// fast RPC failure (-request-timeout small, -request-retries 0) so B notices the
	// dead bootstrap target quickly rather than riding out the adverse-network
	// retry-backoff (this test is about the re-bootstrap self-heal, not RPC retry).
	b := startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-bootstrap-retry", "1s",
		"-request-timeout", "500ms", "-request-retries", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2811")
	if m := b.waitFor(t, reBootstrap, 20*time.Second); m[1] != "0" {
		t.Fatalf("precondition: B should come up isolated (0 table entries), got %s", m[1])
	}

	// A's listener comes up on the reserved address.
	a := startDaemon(t, "A",
		"-listen", addrA, "-store", t.TempDir(),
		"-capacity", "1G", "-mdns=false", "-id-seed", "2810")
	a.waitFor(t, rePeer, 20*time.Second)

	// Without the retry B would be stranded forever. With it, it recovers on its own.
	b.waitFor(t, reReBootstrap, 30*time.Second)
}
