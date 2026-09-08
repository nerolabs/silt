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
// Non-vacuity: the payload driven here is the EXACT wire bundle a pre-B-9 fetcher sent —
// a real blind-withdrawn token under this server's committed key, and a receipt signed
// over the retired v2 message, CBOR-encoded with the retired field names (v2Bundle
// below, byte-identical to demand.SubmittedReceipt before C1 deleted it). So a handler
// that still parsed and banked would bank THIS bundle, and the assertions below would go
// RED.
//
// C1 (2026-09-08) removed the primitive itself, so there is no longer a type in
// production that can decode this payload. The wire shape is kept HERE, in the gate,
// because the retirement's claim is about what arrives on the wire from an old peer —
// not about what this build can construct.
//
// ABLATIONS (each run RED once): restore the pre-B-9 body of handleDeliveryReceipt →
// "the retired lane ANSWERED OK" (2026-09-07); reply OK with an empty body → the same
// line (2026-09-08).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"

	"github.com/fxamacker/cbor/v2"

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
	receipt := v2Ack(fetcherIdent.Signer(), token, obj, serverID)

	// THE PREMISE: the anchor inside this bundle is one this server's OWN committed
	// keyset verifies, and the receipt signature is a real one over the retired v2
	// message. Without this leg the refusal below could be the payload's own
	// malformation rather than the retirement.
	if e, ok := ks.VerifyInWindow(0, token); !ok || e != 0 {
		t.Fatalf("setup: the anchor inside the bundle does not verify under the committed key (epoch %d, ok %v) — the gate would be vacuous", e, ok)
	}
	if !ed25519.Verify(receipt.Fetcher, receipt.msg(), receipt.Sig) {
		t.Fatal("setup: the v2 receipt signature does not verify — the gate would be vacuous")
	}
	blob, err := cbor.Marshal(v2Bundle{Token: token, Receipt: receipt})
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
	// 3. The demand observable did not move.
	if nd.WitnessedIncrements(obj) != 0 {
		t.Fatalf("the retired lane bumped the demand observable (increments=%d)", nd.WitnessedIncrements(obj))
	}
	// And the token was NOT consumed by the refusal: it still opens a session, which is the
	// only lane that pays now.
	if !sessionPresent(t, nd, fetcherIdent, token, obj) {
		t.Fatal("the token did not open a session after the refused flat presentation — the refusal must consume nothing")
	}
}

// ---------------------------------------------------------------------------
// The RETIRED v2 wire shape, kept here and nowhere else.
//
// These three declarations are byte-for-byte what core/demand exported until C1
// (2026-09-08): the same CBOR field names (no keyasint tags, so the map keys are the
// Go field names) and the same signed message under the "silt/demand/receipt/v2"
// domain. They exist so this gate can drive what an OLD PEER actually sends, which is
// the only input the retirement makes a claim about. Nothing in production reads them.
// ---------------------------------------------------------------------------

type v2Receipt struct {
	Serial  []byte
	Object  ports.Hash
	Server  ports.NodeID
	Fetcher ed25519.PublicKey
	Sig     []byte
}

type v2Bundle struct {
	Token   demand.Token
	Receipt v2Receipt
}

func (r v2Receipt) msg() []byte {
	h := sha256.New()
	h.Write([]byte("silt/demand/receipt/v2"))
	h.Write(r.Serial)
	h.Write(r.Object[:])
	h.Write(r.Server[:])
	h.Write(r.Fetcher)
	return h.Sum(nil)
}

func v2Ack(fetcher ed25519.PrivateKey, token demand.Token, object ports.Hash, server ports.NodeID) v2Receipt {
	r := v2Receipt{
		Serial:  append([]byte(nil), token.Serial...),
		Object:  object,
		Server:  server,
		Fetcher: append(ed25519.PublicKey(nil), fetcher.Public().(ed25519.PublicKey)...),
	}
	r.Sig = ed25519.Sign(fetcher, r.msg())
	return r
}
