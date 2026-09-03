package node

// CRYPTO ADVISORY R1 (2026-09-03) — the C-3 hardness checks reached an
// UNAUTHENTICATED HOT PATH.
//
// The mechanism, verbatim from the finding and re-derived here:
//
//	handleDeliveryReceipt (demandrole.go) calls DemandIssuerKeyset as its FIRST real
//	action on any inbound MsgDeliveryReceipt — before UnmarshalSubmittedReceipt, before
//	the `sub.Receipt.Server != n.id` screen, with no authentication and no rate limit.
//	DemandIssuerKeyset re-pins every held epoch on every read (`for e, iss := range
//	n.demandIssuers { pinDemandIssuerKey(...) }`), and Keyset.Put ran the full
//	ValidatePub — hardness included, ~3.3 ms — unconditionally. Re-Put is unavoidable
//	because Prune drops every FUTURE epoch on every read, so the held map cannot itself
//	serve as the memo.
//
//	Result: one one-byte message from any peer bought 5-9 x 3.3 ms of RSA work on the
//	single-threaded node loop. That is the exact CPU amplifier issuer.go's own comment
//	says the shape/hardness split exists to prevent, re-entered through another door.
//	TestC3_HardnessRunsAtAdmissionNotOnEveryModexp cannot see it: it times the two
//	functions in isolation and never walks the node path.
//
// The gate below COUNTS hardness executions along the real node path rather than
// timing them, because "how often does this run" is the property, and a count is the
// only thing that cannot be greened by a faster machine.

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"
	"time"

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

// c3HeldBandNode builds a self-issuing node holding a band of `epochs` distinct RSA
// keys, every one committed at genesis, with the demand bank armed — the shipped
// daemon's shape (cmd/silt/daemon.go: EnableDemandBank(nd.ID()), so the self-pin
// branch is the LIVE branch). It returns the node and a peer NodeID to send from.
//
// The band is 5 deep, not the daemon's 9: issuerKeyPrePublish = 4 caps a genesis
// block's registrations at epochs [0, 4]. The amplification is linear in the band, so
// 5 proves the shape; the shipped daemon's is 9/5 worse.
func c3HeldBandNode(t testing.TB, epochs int) (*Node, ports.NodeID) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	ident := identity.FromSeed(9401)

	keys := make([]*rsa.PrivateKey, epochs)
	regs := make([]chain.IssuerKeyReg, epochs)
	for e := 0; e < epochs; e++ {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		keys[e] = k
		regs[e] = chain.SignIssuerKeyReg(ident.Signer(), uint64(e), demand.KeyFingerprint(&k.PublicKey))
	}

	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{{Root: ports.HashBytes([]byte("c3-hotpath-genesis"))}},
		IssuerKeys: regs,
	}
	chain.Sign(&g, ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis committing %d issuer-key bindings: %v", epochs, err)
	}

	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	nd.SetLedger(credit.New(50_000, 500_000))
	nd.EnableChain(c, ident.Signer())
	for e := 0; e < epochs; e++ {
		nd.SetDemandIssuerKey(rand.Reader, uint64(e), keys[e])
	}
	nd.EnableDemandBank(ident.NodeID())

	// Deliberately NOT calling DemandIssuerKeyset here: the band's first admission is
	// what the gate measures, and a warm-up in the fixture would hide it.
	return nd, identity.FromSeed(9402).NodeID()
}

// TestC3_InboundReceiptsCostOHardnessChecksNotOPerMessage is the R1 gate.
//
// RED before the admission memo in demand.Keyset.Put: 50 messages x 5 held epochs =
// 250 hardness runs. GREEN after: the band is admitted ONCE and every subsequent
// message costs zero.
//
// Ablation (run 2026-09-03): delete the `k.admitted` lookup in Keyset.Put so it calls
// blindtoken.ValidatePub unconditionally -> RED at "message 2 ... 5 hardness runs".
func TestC3_InboundReceiptsCostOHardnessChecksNotOPerMessage(t *testing.T) {
	const band = 5
	const messages = 50

	nd, peer := c3HeldBandNode(t, band)

	// Warm: the band's first admission. This is the O(distinct keys) cost the design
	// allows — hardness AT ADMISSION.
	before := blindtoken.ValidatePubHardnessRuns()
	nd.handleDeliveryReceipt(peer, ports.Message{Kind: ports.MsgDeliveryReceipt, Data: []byte{0x00}})
	admission := blindtoken.ValidatePubHardnessRuns() - before
	// NON-VACUITY, and it is the load-bearing half of this gate: the first message must
	// pay EXACTLY one hardness run per band key. If it paid zero the fixture would not
	// be driving the door at all and "0 per message" below would prove nothing.
	if admission != band {
		t.Fatalf("the band's first admission ran hardness %d times, want exactly %d "+
			"(one per committed key). Either the fixture is not reaching Keyset.Put, or "+
			"the memo is skipping an admission it must pay for.", admission, band)
	}
	if ks := nd.DemandIssuerKeyset(nd.id); ks == nil || ks.Key(0) == nil {
		t.Fatal("the committed issuer key was not pinned — the fixture would be measuring " +
			"a keyset that never admits anything")
	}

	// The gate: every message after the band is admitted costs ZERO hardness.
	base := blindtoken.ValidatePubHardnessRuns()
	start := time.Now()
	for i := 0; i < messages; i++ {
		nd.handleDeliveryReceipt(peer, ports.Message{Kind: ports.MsgDeliveryReceipt, Data: []byte{0x00}})
		if got := blindtoken.ValidatePubHardnessRuns() - base; got != 0 {
			t.Fatalf("message %d: %d hardness runs on an inbound MsgDeliveryReceipt. "+
				"ValidatePub's hardness half (~3.3 ms) must run at ADMISSION only. An "+
				"unauthenticated peer that can drive it per-message owns the node loop: "+
				"at a %d-epoch band that is %d x 3.3 ms per one-byte frame (crypto "+
				"advisory R1, 2026-09-03).", i+1, got, band, band)
		}
	}
	elapsed := time.Since(start)
	perMsg := elapsed / messages

	// The budget. A hardness run is ~3.3 ms, so ANY per-message hardness blows this by
	// more than an order of magnitude; the check is a second, independent statement of
	// the same property that also catches a cost regression the counter cannot see.
	//
	// NOT UNDER -race. The COUNT above is the property and runs under both builds. The
	// wall-clock half is a measurement, and the race detector's instrumentation inflates
	// it ~10x: 4.8 µs/message uninstrumented, 45 µs under -race on the same box, 108 µs
	// on the CI runner's -race job, all on the same code (PR #711). Under -race the
	// number measures the detector, not the path —
	// the same reason `TestC3_ValidatePubCostBudget` sits behind `//go:build !race`. The
	// cost is still LOGGED under both builds so a human reads it.
	const budget = 100 * time.Microsecond
	if perMsg > budget && !raceEnabled {
		t.Fatalf("inbound MsgDeliveryReceipt cost %v/message over %d messages (budget %v). "+
			"The C-3 design puts hardness at admission and SHAPE ONLY on this path.",
			perMsg, messages, budget)
	}
	t.Logf("R1 CLOSED: %d-epoch band. Hardness runs = %d at the band's first admission, "+
		"then 0 across %d further inbound receipts. Cost %v/message (budget %v, "+
		"asserted only without -race; race=%v).",
		band, admission, messages, perMsg, budget, raceEnabled)
}

// TestC3_ADifferentCommittedKeyStillPaysFullAdmission is the memo's teeth-check: the
// memo must be an identity cache, not a bypass. A key the keyset has never admitted
// runs the full hardness half however many other keys are memoised.
func TestC3_ADifferentCommittedKeyStillPaysFullAdmission(t *testing.T) {
	ks := demand.NewKeyset(demand.DefaultWindow)
	a, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	before := blindtoken.ValidatePubHardnessRuns()
	ks.Put(0, &a.PublicKey)
	if n := blindtoken.ValidatePubHardnessRuns() - before; n != 1 {
		t.Fatalf("first Put of key A ran hardness %d times, want 1", n)
	}
	before = blindtoken.ValidatePubHardnessRuns()
	for i := 0; i < 10; i++ {
		ks.Put(0, &a.PublicKey)
	}
	if n := blindtoken.ValidatePubHardnessRuns() - before; n != 0 {
		t.Fatalf("re-Put of the IDENTICAL key A ran hardness %d times, want 0", n)
	}
	before = blindtoken.ValidatePubHardnessRuns()
	ks.Put(1, &b.PublicKey)
	if n := blindtoken.ValidatePubHardnessRuns() - before; n != 1 {
		t.Fatalf("first Put of a DIFFERENT key B ran hardness %d times, want 1 — the memo "+
			"is an identity cache keyed on the committed fingerprint, never a bypass", n)
	}

	// And the memo must not admit a degenerate key by memo-hit on a sibling: shape runs
	// on every Put. N = 1 is the F4 universal forgery.
	ks.Put(2, &rsa.PublicKey{N: big.NewInt(1), E: 65537})
	if ks.Key(2) != nil {
		t.Fatal("a degenerate N = 1 key entered the keyset — the memo must skip HARDNESS " +
			"only; ValidateShape still runs on every Put (F4)")
	}
	t.Log("memo teeth: identical key -> 0 hardness runs; a new key -> 1; a degenerate key " +
		"is still refused by shape on every Put")
}

// BenchmarkC3InboundDeliveryReceipt measures the per-message cost of the
// unauthenticated inbound path over a held band — the number the advisory priced at
// 28.6 ms before the memo.
func BenchmarkC3InboundDeliveryReceipt(b *testing.B) {
	nd, peer := c3HeldBandNode(b, 5)
	msg := ports.Message{Kind: ports.MsgDeliveryReceipt, Data: []byte{0x00}}
	nd.handleDeliveryReceipt(peer, msg) // admit the band
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nd.handleDeliveryReceipt(peer, msg)
	}
}
