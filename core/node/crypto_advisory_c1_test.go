package node

// Crypto-specialist advisory C-1 at the NODE tier, 2026-09-03.
// Source: /Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md
//
// The unit-tier gate is core/blindtoken TestC1_*. This one drives the composition the
// advisory actually traced, which is what makes C-1 more than conformance:
//
//	1. a malicious issuer returns a garbage blind signature;
//	2. nothing at withdrawal detects it, and the fetcher is charged the fee;
//	3. the fetch happens, the fetcher signs the receipt, the server banks it;
//	4. Bank.Redeem fails at VerifyInWindow and returns credited = false;
//	5. handleDeliveryReceipt takes the else branch and NEVER CALLS THE LEDGER —
//	   so the serve's eager unwitnessed self-mint is never reversed.
//
// An issuer handing out duds drove its whole cohort onto the self-mint path at no cost
// to itself and with no detection. RFC 9474 §4.4 Finalize closes the entry at step 2.

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"

	"github.com/fxamacker/cbor/v2"
)

// TestC1_AnIssuerThatReturnsADudIsRefusedAtWithdrawal drives the real withdrawal path
// against a Byzantine issuer that serves an honestly-committed key and then signs
// NOTHING — it returns a well-formed but wrong representative under the right epoch, so
// the epoch screen (ErrDemandEpochMismatch) cannot see it. Only Finalize can.
func TestC1_AnIssuerThatReturnsADudIsRefusedAtWithdrawal(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	issuerIdent := identity.FromSeed(9911)
	fetcherIdent := identity.FromSeed(9912)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := c3Chain(t, 1, issuerIdent.Signer(),
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))

	fetcher := New(fetcherIdent.NodeID(), DefaultConfig(), sched,
		net.Endpoint(fetcherIdent.NodeID()), memstore.New())
	fetcher.SetSigner(fetcherIdent.Signer())
	fetcher.SetLedger(credit.New(50_000, 500_000))
	fetcher.EnableChain(c, fetcherIdent.Signer())

	iep := net.Endpoint(issuerIdent.NodeID())
	iep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		switch msg.Kind {
		case ports.MsgGetDemandIssuerKeys:
			blob, merr := cbor.Marshal(demandKeysetWire{Keys: []epochKeyDER{
				{Epoch: 0, DER: blindtoken.MarshalPub(&key.PublicKey)},
			}})
			if merr != nil {
				t.Error(merr)
				return
			}
			_ = iep.Send(from, ports.Message{Kind: ports.MsgDemandIssuerKeysReply, RID: msg.RID, OK: true, Data: blob})
		case ports.MsgDemandTokenRequest:
			// THE DUD: an in-range, canonically-encoded value that is not the
			// signature. It is NOT malformed and it names the RIGHT epoch, so every
			// screen except Finalize passes it through.
			dud := new(big.Int).Add(new(big.Int).SetBytes(msg.Data), big.NewInt(1))
			dud.Mod(dud, key.N)
			_ = iep.Send(from, ports.Message{Kind: ports.MsgDemandTokenReply, RID: msg.RID,
				OK: true, Data: dud.Bytes(), Height: 0})
		}
	})

	var pinned int
	fetcher.FetchDemandIssuerKeys(issuerIdent.NodeID(), func(n int, _ error) { pinned = n })
	sched.Run()
	if pinned != 1 {
		t.Fatalf("setup: the fetcher pinned %d keys, want 1", pinned)
	}

	var tok demand.Token
	var gotErr error
	var called bool
	fetcher.AcquireDemandTokenInWindow(rand.Reader, issuerIdent.NodeID(),
		func(tk demand.Token, _ uint64, err error) { tok, gotErr, called = tk, err, true })
	sched.Run()

	if !called {
		t.Fatal("the withdrawal never completed")
	}
	if gotErr == nil {
		t.Fatal("BREAK C-1: the withdrawal ACCEPTED a token whose signature does not verify " +
			"under the committed key. The fetcher pays the fee, fetches, signs a receipt, " +
			"and the server's Bank.Redeem then refuses it — so handleDeliveryReceipt never " +
			"reaches the ledger and the serve's eager self-mint is NEVER REVERSED. An " +
			"issuer that hands out duds drives its whole cohort onto the self-mint path " +
			"at no cost to itself.")
	}
	if len(tok.Serial) != 0 || len(tok.Sig) != 0 {
		t.Fatalf("a refused withdrawal must yield no token at all, got serial=%d sig=%d",
			len(tok.Serial), len(tok.Sig))
	}
	t.Logf("refused at withdrawal: %v", gotErr)
}
