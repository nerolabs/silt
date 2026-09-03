package node

// R0.4b C3 re-break — node-tier regression gates. Inversions of the red-team probes
// core/node/rt_c3b_node_test.go (RT-C3B-15 … RT-C3B-17), archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.
// These run through the SHIPPED lanes — the consensus-attested pin, the real wire
// handler — not the primitives, because that is where the probes measured the breaks.

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/guardstore"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// rtByzIssuer stands up a bare endpoint that serves ONE public key for epoch 0 —
// whatever key it is handed, valid RSA or not — and a fetcher node whose chain commits
// that key's fingerprint. It returns the fetcher and the issuer's NodeID.
func rtByzIssuer(t *testing.T, served *rsa.PublicKey) (*Node, ports.NodeID, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	issuerIdent := identity.FromSeed(9101)
	fetcherIdent := identity.FromSeed(9103)

	// The commitment binds sha256(MarshalPub(served)) — the ONLY test the pin runs.
	c := c3Chain(t, 1, issuerIdent.Signer(),
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 0, demand.KeyFingerprint(served)))

	fetcher := New(fetcherIdent.NodeID(), DefaultConfig(), sched,
		net.Endpoint(fetcherIdent.NodeID()), memstore.New())
	fetcher.SetSigner(fetcherIdent.Signer())
	fetcher.SetLedger(credit.New(50_000, 500_000))
	fetcher.EnableChain(c, fetcherIdent.Signer())

	iep := net.Endpoint(issuerIdent.NodeID())
	iep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind != ports.MsgGetDemandIssuerKeys {
			return
		}
		w := demandKeysetWire{Keys: []epochKeyDER{{Epoch: 0, DER: blindtoken.MarshalPub(served)}}}
		blob, err := cbor.Marshal(w)
		if err != nil {
			t.Error(err)
			return
		}
		_ = iep.Send(from, ports.Message{Kind: ports.MsgDemandIssuerKeysReply, RID: msg.RID, OK: true, Data: blob})
	})
	return fetcher, issuerIdent.NodeID(), sched
}

// ---------------------------------------------------------------------------
// RT-C3B-15 / 16 CLOSED, on the shipped lane. The consensus-attested pin used to
// accept a modulus of ZERO (it attests 32 bytes, not a key) and the shipped withdrawal
// lane then PANICKED inside big.Int.Mod — a bonded Byzantine issuer crashing every
// fetcher that transacted with it. N = 1 pinned just as cleanly and verified an
// arbitrary (serial, sig) pair.
//
// The pin must now REFUSE the key outright, and the withdrawal lane must return a
// legible error instead of dying.
// ---------------------------------------------------------------------------
func TestRTC3_DegenerateCommittedKeyIsRefusedByThePinAndTheLane(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  *rsa.PublicKey
	}{
		{"zero-modulus", &rsa.PublicKey{N: big.NewInt(0), E: 65537}},
		{"unit-modulus", &rsa.PublicKey{N: big.NewInt(1), E: 65537}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fetcher, issuer, sched := rtByzIssuer(t, tc.key)

			var pinned int
			fetcher.FetchDemandIssuerKeys(issuer, func(n int, _ error) { pinned = n })
			sched.Run()
			if pinned != 0 {
				t.Fatalf("BREAK RT-C3B-15/16 REOPENED: the pin HELD a degenerate key "+
					"(pinned=%d). The consensus anchor constrains WHICH BYTES an issuer "+
					"serves, never whether those bytes are an unforgeable signature scheme.",
					pinned)
			}
			if ks := fetcher.DemandIssuerKeyset(issuer); ks != nil && ks.Key(0) != nil {
				t.Fatalf("the keyset holds a degenerate key")
			}

			// The shipped withdrawal lane: a legible refusal, NEVER a panic. This is the
			// exact call chain cmd/silt/swarm.go drives.
			var gotErr error
			var called bool
			fetcher.AcquireDemandTokenInWindow(rand.Reader, issuer, func(_ demand.Token, _ uint64, err error) {
				called, gotErr = true, err
			})
			sched.Run()
			if !called {
				t.Fatalf("the withdrawal lane never completed")
			}
			if gotErr == nil {
				t.Fatalf("the withdrawal lane SUCCEEDED against a degenerate committed key")
			}
			t.Logf("%s: pin refused, lane refused legibly with %v", tc.name, gotErr)
		})
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-17 CLOSED, through the real wire handler. The probe measured "the identical
// MsgDeliveryReceipt paid 43750, was refused in-process, then paid 43750 AGAIN after a
// restart" — a restart is an eviction of every serial, in-window or not, which is the
// one eviction mode the design forbids.
//
// The restart is modelled exactly as the daemon performs it (a fresh demand.Bank and a
// fresh credit.Ledger), except the ledger now shares the DURABLE guard store the
// daemon opens at boot.
// ---------------------------------------------------------------------------
func TestRTC3_RestartDoesNotRePayTheSameWireReceipt(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	srvIdent := identity.FromSeed(9201)
	fetcherIdent := identity.FromSeed(9203)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := c3Chain(t, 1, srvIdent.Signer(),
		chain.SignIssuerKeyReg(srvIdent.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))

	store, err := guardstore.Open(t.TempDir() + "/paidserials.log")
	if err != nil {
		t.Fatal(err)
	}
	boot := func() *Node {
		nd := New(srvIdent.NodeID(), DefaultConfig(), sched, net.Endpoint(srvIdent.NodeID()), memstore.New())
		nd.SetSigner(srvIdent.Signer())
		ledger := credit.New(50_000, 500_000)
		ledger.SetPaidSerialStore(store)
		if lerr := ledger.LoadPaidSerials(); lerr != nil {
			t.Fatal(lerr)
		}
		nd.SetLedger(ledger)
		nd.EnableChain(c, srvIdent.Signer())
		nd.SetDemandIssuerKey(0, key)
		nd.EnableDemandBank(nd.ID()) // issuer == server, the shipped wiring
		return nd
	}

	srv := boot()

	// One real blind withdrawal, one fee, one receipt.
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, err := demand.Withdraw(rand.Reader, &key.PublicKey, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok := demand.Unblind(&key.PublicKey, serial, demand.SignWithdrawal(key, blinded), secret)
	obj := ports.HashBytes([]byte("rt-c3b-object"))
	r := demand.Ack(fetcherIdent.Signer(), tok, obj, srv.ID())
	blob, err := demand.SubmittedReceipt{Token: tok, Receipt: r}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	wire := ports.Message{Kind: ports.MsgDeliveryReceipt, Data: blob}

	before := srv.ledger.Balance(srv.ID())
	srv.handleDeliveryReceipt(fetcherIdent.NodeID(), wire)
	firstPay := srv.ledger.Balance(srv.ID()) - before
	if firstPay <= 0 {
		t.Fatalf("setup: the first submission must pay, moved %+d", firstPay)
	}
	mid := srv.ledger.Balance(srv.ID())
	srv.handleDeliveryReceipt(fetcherIdent.NodeID(), wire)
	if srv.ledger.Balance(srv.ID()) != mid {
		t.Fatalf("setup: the in-process guard must refuse the replay")
	}

	// RESTART.
	srv2 := boot()
	before2 := srv2.ledger.Balance(srv2.ID())
	srv2.handleDeliveryReceipt(fetcherIdent.NodeID(), wire)
	if secondPay := srv2.ledger.Balance(srv2.ID()) - before2; secondPay != 0 {
		t.Fatalf("BREAK RT-C3B-17 REOPENED: the identical MsgDeliveryReceipt paid %d, then "+
			"paid %d AGAIN after a restart. A restart is an eviction of EVERY guarded token, "+
			"in-window or not — the one eviction mode the R0.4b coupling condition forbids.",
			firstPay, secondPay)
	}
}

// ---------------------------------------------------------------------------
// G-4 gate (iii), on the shipped wire lane (research certification
// R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03, item 4).
//
// The G-4 fix makes every guard refusal reverse the serve's eager self-mint. The
// boundary of that rule is WITNESSED: an unwitnessed or malformed receipt must NOT
// reverse anything — the self-mint stays until a valid receipt arrives, which is the
// legitimate unwitnessed bilateral fallback (RecordServeToObject's 1 credit/byte).
//
// The property is structural: demandBank.Redeem returns credited=false, so
// handleDeliveryReceipt never calls the ledger at all. This gate holds it in place,
// because "reverse on every refusal" is one careless hoist away from "reverse on
// anything that arrives on the wire" — which would let an unauthenticated peer erase
// a server's earnings for free.
// ---------------------------------------------------------------------------
func TestG4_UnwitnessedReceiptLeavesTheSelfMintAlone(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	srvIdent := identity.FromSeed(9301)
	fetcherIdent := identity.FromSeed(9303)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := c3Chain(t, 1, srvIdent.Signer(),
		chain.SignIssuerKeyReg(srvIdent.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))

	srv := New(srvIdent.NodeID(), DefaultConfig(), sched, net.Endpoint(srvIdent.NodeID()), memstore.New())
	srv.SetSigner(srvIdent.Signer())
	ledger := credit.New(50_000, 0)
	srv.SetLedger(ledger)
	srv.EnableChain(c, srvIdent.Signer())
	srv.SetDemandIssuerKey(0, key)
	srv.EnableDemandBank(srv.ID())

	obj := ports.HashBytes([]byte("g4-unwitnessed-object"))
	const served = int64(64 << 20)
	ledger.Register(srv.ID())
	ledger.Register(fetcherIdent.NodeID())
	before := ledger.Balance(srv.ID())
	ledger.RecordServeToObject(srv.ID(), fetcherIdent.NodeID(), obj, ports.ChunkID(obj), served)
	selfMint := ledger.Balance(srv.ID()) - before
	if selfMint <= 0 {
		t.Fatalf("setup: the serve produced no self-mint (%+d)", selfMint)
	}

	// A receipt whose blind signature is garbage: correctly shaped, correctly
	// addressed to this server, and NOT witnessed by the issuer.
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	forged := demand.Token{Serial: serial, Sig: make([]byte, 256)}
	r := demand.Ack(fetcherIdent.Signer(), forged, obj, srv.ID())
	blob, err := demand.SubmittedReceipt{Token: forged, Receipt: r}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		srv.handleDeliveryReceipt(fetcherIdent.NodeID(), ports.Message{Kind: ports.MsgDeliveryReceipt, Data: blob})
	}
	if got := ledger.Balance(srv.ID()) - before; got != selfMint {
		t.Fatalf("an UNWITNESSED receipt moved the server's balance to %+d off pre-serve, "+
			"want the intact self-mint %+d. The supersede reverses only for a receipt the "+
			"bank verified; an unauthenticated peer must not be able to erase a serve.",
			got, selfMint)
	}

	// The premise leg: this fixture CAN reverse. Without it the assertion above would
	// pass on a lane that never reaches the ledger for any reason at all.
	blinded, secret, err := demand.Withdraw(rand.Reader, &key.PublicKey, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok := demand.Unblind(&key.PublicKey, serial, demand.SignWithdrawal(key, blinded), secret)
	real := demand.Ack(fetcherIdent.Signer(), tok, obj, srv.ID())
	realBlob, err := demand.SubmittedReceipt{Token: tok, Receipt: real}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	srv.handleDeliveryReceipt(fetcherIdent.NodeID(), ports.Message{Kind: ports.MsgDeliveryReceipt, Data: realBlob})
	if got := ledger.Balance(srv.ID()) - before; got == selfMint {
		t.Fatalf("the WITNESSED receipt did not supersede the self-mint (still %+d) — "+
			"the negative leg above proves nothing on a fixture that never reaches the ledger",
			got)
	}
}
