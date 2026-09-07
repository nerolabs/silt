package sim

// B-9 (2026-09-07): the sim's demand-lane tests drive the SESSION lane (open with the token
// as the anchor, settle cumulative counts) instead of the retired flat receipt. The
// properties they pin are unchanged; only the driver is.

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// openSession opens a paid delivery session at server from fetcher's durable identity
// with tok as the anchor, over the wire. It returns the handle and the open commitment,
// or the server's named refusal.
func openSession(t *testing.T, cl *Cluster, fetcher, server *node.Node, tok demand.Token) (uint64, []byte, error) {
	t.Helper()
	var handle uint64
	var m []byte
	var oerr error
	fetcher.OpenDeliverySessionRemote(server.ID(), []demand.Token{tok}, func(h uint64, c []byte, err error) { handle, m, oerr = h, c, err })
	cl.Sched.Run()
	return handle, m, oerr
}

// settleSession submits a cumulative-count receipt for object on the session and returns
// the gross credits the server settled (0 for a non-advancing count) or its refusal.
func settleSession(t *testing.T, cl *Cluster, fetcher, server *node.Node, handle uint64, m []byte, object ports.Hash, count uint64) (int64, error) {
	t.Helper()
	var settled int64
	var serr error
	fetcher.SubmitDeliverySettle(server.ID(), handle, m, object, count, func(s int64, err error) { settled, serr = s, err })
	cl.Sched.Run()
	return settled, serr
}

// bondingGenesis builds a minimal objective chain whose genesis DECLARES a bond for
// each key in `bonded` (its NodeID = sha256(pubkey) enters the committed bond ledger
// at minBond). Genesis state is trusted — AppendGenesis applies bonds without
// re-verifying the space-time proof — so this seeds exactly the committed bond ledger
// the P3b gate (chain.IsBonded) consults, without standing up consensus rounds. Each
// bond gets a distinct Root so the per-root dedup (F1) counts them as distinct
// identities.
func bondingGenesis(t *testing.T, proposer ed25519.PrivateKey, issuerReg chain.IssuerKeyReg,
	bonded ...ed25519.PublicKey) *chain.Chain {
	t.Helper()
	const minBond = int64(1) << 20
	propPub := proposer.Public().(ed25519.PublicKey)
	cfg := chain.Config{
		Quorum:           1,
		MinBond:          minBond,
		Anchors:          map[ports.NodeID]bool{ports.HashBytes(propPub): true},
		AnchorQuorum:     1,
		MatureValidators: 99, // never matures here; irrelevant to IsBonded
	}
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })

	reg := func(pub ed25519.PublicKey) chain.BondReg {
		return chain.BondReg{
			Validator: append([]byte(nil), pub...),
			Root:      ports.HashBytes(pub), // distinct per identity (dodges F1 dedup)
			Size:      minBond,
		}
	}
	regs := []chain.BondReg{reg(propPub)}
	for _, pub := range bonded {
		regs = append(regs, reg(pub))
	}
	// R0.4b: v5 genesis so it can commit the demand-issuer key binding (the era-3 leaf
	// set does not carry that keyspace, so a pre-v5 block carrying one is rejected).
	g := &chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{synthEntry("bonded-demand genesis")},
		BondRegs:   regs,
		IssuerKeys: []chain.IssuerKeyReg{issuerReg},
	}
	chain.Sign(g, proposer)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("bonding genesis: %v", err)
	}
	return ch
}
