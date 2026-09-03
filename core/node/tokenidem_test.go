package node

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// Token issuance must be IDEMPOTENT under transport retries (research
// certification 2026-08-13, A2): a lost REPLY makes the requester re-present
// the SAME blinded serial (requestAttempt re-sends msg.Data verbatim), and the
// issuer must answer with the SAME signature while charging the fee ONCE.
// Signing is deterministic RSA-FDH, so the dedup changes no cryptography —
// only the charge/spend accounting.

// Legacy fee path: a re-presented blinded serial returns the identical
// signature and the durable identity is charged exactly once.
func TestTokenIssueRetryIdempotent_LegacyFee(t *testing.T) {
	const fee = 100
	nd, ledger, key := newIssuerNode(t, fee)
	pub := &key.PublicKey
	durable := identity.FromSeed(2).NodeID()
	ledger.Register(durable)
	start := ledger.Balance(durable)

	ts, _ := blindtoken.NewSerial(rand.Reader)
	tblind, tsecret, err := blindtoken.Blind(rand.Reader, pub, ts)
	if err != nil {
		t.Fatal(err)
	}
	first := nd.answerTokenRequest(durable, ports.Message{Data: tblind})
	if !first.OK {
		t.Fatal("first issuance should succeed")
	}
	retry := nd.answerTokenRequest(durable, ports.Message{Data: tblind})
	if !retry.OK {
		t.Fatal("a retry re-presenting the same blinded serial must succeed")
	}
	if !bytes.Equal(first.Data, retry.Data) {
		t.Fatal("a retry must return the IDENTICAL blind signature (deterministic RSA-FDH)")
	}
	if charged := start - ledger.Balance(durable); charged != fee {
		t.Fatalf("a retried issuance must charge the fee ONCE: charged %d, want %d", charged, fee)
	}
	if sig := mustUnblindToken(t, pub, ts, retry.Data, tsecret); !blindtoken.Verify(pub, ts, sig) {
		t.Fatal("the deduped signature must still verify")
	}
}

// Prepaid-credit path (the flagship privacy flow): before the dedup, a retry
// re-presenting the same blinded serial with the same credit was REFUSED as a
// credit double-spend — a lost reply then failed the whole gather. The retry
// must succeed with the same signature and the credit spent once; a genuinely
// NEW blinded serial on that spent credit must still be refused.
func TestTokenIssueRetryIdempotent_CreditPath(t *testing.T) {
	const fee = 100
	nd, ledger, key := newIssuerNode(t, fee)
	pub := &key.PublicKey
	durable := identity.FromSeed(2).NodeID()
	ledger.Register(durable)

	cs, _ := blindtoken.NewSerial(rand.Reader)
	cblind, csecret, err := blindtoken.BlindCredit(rand.Reader, pub, cs)
	if err != nil {
		t.Fatal(err)
	}
	mint := nd.answerTokenRequest(durable, ports.Message{Data: cblind})
	if !mint.OK {
		t.Fatal("credit mint should succeed")
	}
	credit := ports.PublishCredit{Serial: cs, Sig: mustUnblindCredit(t, pub, cs, mint.Data, csecret)}

	ts, _ := blindtoken.NewSerial(rand.Reader)
	tblind, _, err := blindtoken.Blind(rand.Reader, pub, ts)
	if err != nil {
		t.Fatal(err)
	}
	first := nd.answerTokenRequest(durable, ports.Message{Data: tblind, Credit: &credit})
	if !first.OK {
		t.Fatal("credit-backed issuance should succeed")
	}
	retry := nd.answerTokenRequest(durable, ports.Message{Data: tblind, Credit: &credit})
	if !retry.OK {
		t.Fatal("retry regression (A2): re-presenting the same blinded serial with the same credit must be idempotent, not a double-spend refusal")
	}
	if !bytes.Equal(first.Data, retry.Data) {
		t.Fatal("the credit-path retry must return the identical blind signature")
	}

	// A DIFFERENT blinded serial on the (now spent) credit is a real double-spend.
	ts2, _ := blindtoken.NewSerial(rand.Reader)
	tblind2, _, _ := blindtoken.Blind(rand.Reader, pub, ts2)
	if r := nd.answerTokenRequest(durable, ports.Message{Data: tblind2, Credit: &credit}); r.OK {
		t.Fatal("a fresh blinded serial on a spent credit must still be refused")
	}
}

// The dedup window is bounded: once the transport retry window (tokenDedupTTL)
// has passed, a re-presented blinded serial is a NEW issuance again (charged).
// This keeps the cache from becoming an unbounded free-signature oracle.
func TestTokenIssueDedupExpires(t *testing.T) {
	const fee = 100
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(fee, 50_000)
	id := identity.FromSeed(1)
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.SetLedger(ledger)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	nd.EnableTokenIssuer(rand.Reader, key)
	pub := &key.PublicKey
	durable := identity.FromSeed(2).NodeID()
	ledger.Register(durable)
	start := ledger.Balance(durable)

	ts, _ := blindtoken.NewSerial(rand.Reader)
	tblind, _, err := blindtoken.Blind(rand.Reader, pub, ts)
	if err != nil {
		t.Fatal(err)
	}
	if r := nd.answerTokenRequest(durable, ports.Message{Data: tblind}); !r.OK {
		t.Fatal("first issuance should succeed")
	}
	// Advance the virtual clock past the TTL.
	sched.AfterFunc(nd.tokenDedupTTL()+1, func() {})
	sched.Run()
	if r := nd.answerTokenRequest(durable, ports.Message{Data: tblind}); !r.OK {
		t.Fatal("a post-TTL re-present is a normal (charged) issuance and should succeed")
	}
	if charged := start - ledger.Balance(durable); charged != 2*fee {
		t.Fatalf("post-TTL re-present should charge again: charged %d, want %d", charged, 2*fee)
	}
}
