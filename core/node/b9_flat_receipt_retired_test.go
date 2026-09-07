package node

// B-9 GATE — the flat receipt is RETIRED, and the retirement itself has teeth.
//
// The certification's rule is "the build refuses a receipt that names no open session".
// Until this gate existed nothing asserted it: a future refactor could restore a
// banking-and-paying body in handleDeliveryReceipt and every named package stayed green
// (blind PE, 2026-09-07, measured). The ablation-first discipline (four vacuous-gate scars
// in September) says the retirement is gated on its RUNTIME effect, not on the presence
// of the constant.
//
// Non-vacuity: the receipt driven here is WELL-FORMED and OTHERWISE VALID — the v2
// primitive banks it on a scratch bank against the same committed key. So a handler that
// still banked would bank THIS receipt, and the three assertions below would go RED.
//
// ABLATION (run 2026-09-07): restore the pre-B-9 body of handleDeliveryReceipt → RED on
// "the retired lane ANSWERED OK".

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

func TestFlatReceiptIsRefusedAndMovesNothing(t *testing.T) {
	issuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	serverIdent, fetcherIdent := identity.FromSeed(9401), identity.FromSeed(9402)
	serverID, fetcherID := serverIdent.NodeID(), fetcherIdent.NodeID()

	nd := New(serverID, DefaultConfig(), sched, net.Endpoint(serverID), memstore.New())
	ledger := credit.New(50_000, 0)
	nd.SetLedger(ledger)
	nd.SetSigner(serverIdent.Signer())
	c := c3Chain(t, 1, serverIdent.Signer(),
		chain.SignIssuerKeyReg(serverIdent.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)))
	nd.EnableChain(c, serverIdent.Signer())
	nd.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	nd.EnableDemandBank(serverID)
	nd.EnableDeliverySessions(10 * ports.Second)
	ledger.Register(serverID)
	ledger.Register(fetcherID)
	ks := nd.DemandIssuerKeyset(serverID)
	if ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: the committed issuer key was not pinned")
	}

	// A real blind withdrawal under the committed key, and the v2 receipt over it.
	serial := make([]byte, 32)
	if _, err := rand.Read(serial); err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, &issuerPriv.PublicKey, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	token, uerr := demand.Unblind(&issuerPriv.PublicKey, 0, serial, demand.SignWithdrawal(rand.Reader, issuerPriv, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	obj := ports.HashBytes([]byte("b9-retired-flat-object"))
	receipt := demand.Ack(fetcherIdent.Signer(), token, obj, serverID)

	// THE PREMISE: this exact receipt is one the v2 primitive BANKS. Without this leg the
	// refusal below could be the receipt's own malformation, not the retirement.
	if credited, _, why := demand.NewBank().Redeem(ks, 0, token, receipt); !credited {
		t.Fatalf("setup: the v2 primitive refused the receipt this gate drives (%s) — the gate would be vacuous", why)
	}
	blob, err := demand.SubmittedReceipt{Token: token, Receipt: receipt}.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	sum := func() int64 { return ledger.Balance(serverID) + ledger.Balance(fetcherID) + ledger.EscrowBalance(obj) }
	before := sum()
	var acks []ports.Message
	net.Endpoint(fetcherID).SetHandler(func(_ ports.NodeID, m ports.Message) { acks = append(acks, m) })
	nd.handle(fetcherID, ports.Message{Kind: ports.MsgDeliveryReceipt, RID: 41, Data: blob})
	sched.Run()

	// 1. Refused, with the named reason — never silent, never OK.
	if len(acks) != 1 || acks[0].Kind != ports.MsgDeliveryReceiptAck || acks[0].RID != 41 {
		t.Fatalf("want exactly one MsgDeliveryReceiptAck (RID 41), got %+v", acks)
	}
	if acks[0].OK {
		t.Fatal("the retired lane ANSWERED OK: a receipt naming no open session was banked — B-9's retirement is undone")
	}
	if got := string(acks[0].Data); got != errFlatReceiptRetired.Error() {
		t.Fatalf("refusal reason %q, want the named retirement %q", got, errFlatReceiptRetired.Error())
	}
	// 2. No balance and no escrow motion.
	if got := sum(); got != before {
		t.Fatalf("the retired lane moved credit: Σ moved by %+d", got-before)
	}
	// 3. Neither demand observable moved.
	if nd.WitnessedDemand(obj) != 0 || nd.WitnessedIncrements(obj) != 0 {
		t.Fatalf("the retired lane bumped a demand observable (demand=%d increments=%d)",
			nd.WitnessedDemand(obj), nd.WitnessedIncrements(obj))
	}
	// And the token was NOT consumed by the refusal: it still opens a session, which is the
	// only lane that pays now.
	if !sessionPresent(t, nd, fetcherIdent, token, obj) {
		t.Fatal("the token did not open a session after the refused flat presentation — the refusal must consume nothing")
	}
}
