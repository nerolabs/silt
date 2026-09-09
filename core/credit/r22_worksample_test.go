package credit

// R2.2 (Lane C3) — the gossip stamp's read must not WRITE.
//
// MECHANISM. Node.send stamps every outbound message with this node's two work
// counters (core/node, rows 8-9). The obvious readers, ServedBytes and RepairsDone, go
// through acct(), and acct() calls Register, and Register CREATES an account: on an
// unconfigured ledger it hands out the starter grant, and on an R2.12 faucet-configured
// one it sets grantPending and increments grantsPending. So reading a counter to fill in
// a FindNode's gossip fields would mint an account and move faucet accounting as a side
// effect of sending a packet. WorkSample is the non-registering read that closes it.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestR22WorkSampleDoesNotRegisterAnAccount is the gate. ABLATION (run before shipping):
// change WorkSample's body to `a := l.acct(n); return a.servedBytes, a.repairsDone, true`
// and both arms below go RED — the census grows to 1 and, on the faucet arm,
// grantsPending goes to 1.
func TestR22WorkSampleDoesNotRegisterAnAccount(t *testing.T) {
	stranger := ports.NodeID{0x22, 0x01}

	t.Run("unconfigured", func(t *testing.T) {
		l := New(0, 5_000)
		served, repairs, ok := l.WorkSample(stranger)
		// THE CENSUS FIRST, deliberately: it is the load-bearing assertion, and if the
		// tuple check ran first the ablation would redden on the cheap symptom and
		// leave this line unproved.
		if n := len(l.Balances()); n != 0 {
			t.Fatalf("the account census grew to %d after a READ. WorkSample registered an account: on the gossip path that mints one per outbound message to a never-seen peer", n)
		}
		if ok || served != 0 || repairs != 0 {
			t.Fatalf("WorkSample on an unknown node = (%d, %d, %v), want (0, 0, false): an unknown node is not a node that has done zero work, and only the caller can tell the two apart", served, repairs, ok)
		}
	})

	t.Run("faucet-configured", func(t *testing.T) {
		l := New(0, 5_000)
		mono := ports.MonotonicNanos(func() int64 { return 0 })
		l.SetFaucet(4, 1, int64(1e9), 0, mono)
		_, _, ok := l.WorkSample(stranger)
		if p := l.FaucetStats().GrantsPending; p != 0 {
			t.Fatalf("faucet grantsPending = %d after a READ, want 0. Register sets grantPending and increments this counter, so a registering read moves the ECONOMY from the gossip stamp", p)
		}
		if ok {
			t.Fatalf("WorkSample reported an account for a node the ledger has never seen")
		}
	})

	t.Run("reports the real counters once the node exists", func(t *testing.T) {
		l := New(0, 5_000)
		server, fetcher := ports.NodeID{0xAA}, ports.NodeID{0xBB}
		l.RecordServe(server, fetcher, ports.ChunkID{0x1}, 4096)
		served, repairs, ok := l.WorkSample(server)
		if !ok || served != 4096 || repairs != 0 {
			t.Fatalf("WorkSample = (%d, %d, %v), want (4096, 0, true) — the gate above is vacuous unless this reads the same counters ServedBytes/RepairsDone do", served, repairs, ok)
		}
		if got := l.ServedBytes(server); got != served {
			t.Fatalf("WorkSample served %d but ServedBytes says %d — the two must read one counter, or the gossiped figure and the panel figure drift", served, got)
		}
	})
}
