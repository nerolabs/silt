package node

import (
	"strconv"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// TestPendingEntriesBounded is the boundedness-audit A2 regression: the entry
// mempool dedups by ROOT, so a party submitting distinct roots faster than the
// designee drains them would grow it without bound (a resident-memory DoS). It
// must stay ≤ maxMempool no matter how many distinct entries are queued.
func TestPendingEntriesBounded(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9201)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-mempool")}}
	chain.Sign(g, id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, MatureValidators: 99}
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd.EnableChain(ch, id.Signer())

	for i := 0; i < maxMempool*2; i++ {
		nd.queuePendingEntry(mkEntry("e-" + strconv.Itoa(i)))
	}
	if got := len(nd.pendingEntries); got > maxMempool {
		t.Fatalf("pendingEntries=%d exceeds cap %d — the entry mempool is not bounded", got, maxMempool)
	}
	if len(nd.pendingEntries) == 0 {
		t.Fatal("pendingEntries is empty — nothing queued at all")
	}
}

// TestPendingBondRegsBounded covers the defense-in-depth cap on the bond-reg
// pool: it is normally validator-bounded (one slot per ValidatorID), but a
// forged/unbonded-ID path must not grow it past maxMempool either.
func TestPendingBondRegsBounded(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9301)
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())

	var root ports.Hash
	for i := 0; i < maxMempool+256; i++ {
		signer := identity.FromSeed(int64(30000 + i)).Signer() // distinct ValidatorID each
		root[0], root[1] = byte(i), byte(i>>8)
		nd.queuePendingBondReg(chain.NewBondReg(signer, root, 1<<20, []byte("valid"), ports.Hash{}, 1))
	}
	if got := len(nd.pendingBondRegs); got > maxMempool {
		t.Fatalf("pendingBondRegs=%d exceeds cap %d", got, maxMempool)
	}
}
