package node

// The "delivery receipt paid NO credit" WARN line is an ANNOUNCED OBSERVABLE
// CONTRACT (S5), and until now nothing asserted it (Tester finding, 2026-09-03: a
// repo-wide grep for the event string and for serial_guard_refusals in *_test.go
// returned nothing). This is the SECOND instance of the observable-log-contract scar
// — the first cost an e2e failure when `freeload: ON` was reworded — so the line is
// gated here at the unit tier: the exact event string, the level, and both fields.
//
// What the line is for: a receipt the bank ACCEPTED can still settle nothing (the
// paid-serial guard full of live serials, a serial already paid on this ledger, a
// zero fee). Before it existed, all of those surfaced only under the success line
// `delivery receipt banked … credit=0`, which reads as success. Renaming the event,
// dropping either field, or folding it back into the banked line all break the
// operator's only signal — so all three go RED here.
//
// The reason value under test is ReasonNoFee because it is the one non-paying path a
// unit test can reach in milliseconds; the operator-actionable ReasonGuardFull value
// is gated at the ledger tier (core/credit TestCapFullRefusalIsObservable) and
// composed at core/node TestComposedExpiryBoundary_EvictionIsClosedAtBothLayers.
// What is under test HERE is the LINE, not which reason produced it.

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

// levelCaptureLog records the LEVEL as well as the event and fields — the package's
// shared captureLog drops it, and the level is part of the contract under test here.
type levelCaptureLog struct {
	events []string
	levels []ports.LogLevel
	kvs    [][]any
}

func (c *levelCaptureLog) Enabled(ports.LogLevel) bool { return true }

func (c *levelCaptureLog) Log(lvl ports.LogLevel, event string, kv ...any) {
	c.events = append(c.events, event)
	c.levels = append(c.levels, lvl)
	c.kvs = append(c.kvs, append([]any(nil), kv...))
}

// last returns the fields of the most recent occurrence of event, and its level.
func (c *levelCaptureLog) last(event string) (map[string]any, ports.LogLevel, bool) {
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i] != event {
			continue
		}
		m := map[string]any{}
		for j := 0; j+1 < len(c.kvs[i]); j += 2 {
			k, _ := c.kvs[i][j].(string)
			m[k] = c.kvs[i][j+1]
		}
		return m, c.levels[i], true
	}
	return nil, 0, false
}

func TestBankedButUnpaidReceiptLogsTheWarnLine(t *testing.T) {
	issuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	issuerPub := &issuerPriv.PublicKey

	sched := simclock.New()
	simNet := simnet.New(sched, 2, simnet.DefaultConfig())
	serverIdent := identity.FromSeed(9101)
	serverID := serverIdent.NodeID()
	nd := New(serverID, DefaultConfig(), sched, simNet.Endpoint(serverID), memstore.New())

	// fee = 0 is the cheap non-paying path: the receipt banks, the redeem reaches the
	// settlement block and returns (0, ReasonNoFee).
	ledger := credit.New(0, 0)
	nd.SetLedger(ledger)
	nd.SetSigner(serverIdent.Signer())

	// R0.4b: the bank verifies against a keyset whose key_E resolved against the
	// CONSENSUS-ATTESTED binding, so the fixture commits it at genesis (epochs off, so
	// the consensus epoch is 0 throughout). Same shape as TestR05NodePathConservation.
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("warnline-genesis-entry"))}},
		IssuerKeys: []chain.IssuerKeyReg{
			chain.SignIssuerKeyReg(serverIdent.Signer(), 0, demand.KeyFingerprint(issuerPub)),
		},
	}
	chain.Sign(&g, serverIdent.Signer())
	if gerr := c.AppendGenesis(g); gerr != nil {
		t.Fatalf("genesis committing the issuer-key binding: %v", gerr)
	}
	nd.EnableChain(c, serverIdent.Signer())
	nd.SetDemandIssuerKey(0, issuerPriv)
	nd.EnableDemandBank(serverID)
	if ks := nd.DemandIssuerKeyset(serverID); ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: the committed issuer key was not pinned — the bank would reject every receipt")
	}

	lg := &levelCaptureLog{}
	nd.SetLogger(lg)

	fetcherIdent := identity.FromSeed(9102)
	serial := make([]byte, 32)
	if _, rerr := rand.Read(serial); rerr != nil {
		t.Fatal(rerr)
	}
	blinded, secret, bErr := demand.Withdraw(rand.Reader, issuerPub, 0, serial)
	if bErr != nil {
		t.Fatalf("demand.Withdraw: %v", bErr)
	}
	token := demand.Unblind(issuerPub, serial, demand.SignWithdrawal(issuerPriv, blinded), secret)
	objRoot := ports.HashBytes([]byte("warnline-object-root"))
	submitted := demand.SubmittedReceipt{
		Token:   token,
		Receipt: demand.Ack(fetcherIdent.Signer(), token, objRoot, serverID),
	}
	blob, mErr := submitted.Marshal()
	if mErr != nil {
		t.Fatalf("SubmittedReceipt.Marshal: %v", mErr)
	}

	nd.handle(fetcherIdent.NodeID(), ports.Message{Kind: ports.MsgDeliveryReceipt, Data: blob, Ephemeral: true})

	// The receipt must actually have BANKED — otherwise this test would pass
	// vacuously against a node that rejected it outright.
	if nd.WitnessedDemand(objRoot) == 0 {
		t.Fatal("setup: the bank rejected the receipt, so the unpaid-settlement path was never reached")
	}
	if paid := ledger.Balance(serverID); paid != 0 {
		t.Fatalf("setup: the redeem paid %d — this fixture must reach the NON-paying path", paid)
	}

	// THE CONTRACT. Event string verbatim, level WARN, both fields present.
	const event = "delivery receipt paid NO credit"
	kv, lvl, ok := lg.last(event)
	if !ok {
		t.Fatalf("no %q line: a banked receipt that settled NOTHING must say so, or the only "+
			"operator signal is the `delivery receipt banked … credit=0` success line "+
			"(observable-log-contract scar, instance 2)\nlines seen: %v", event, lg.events)
	}
	if got, present := kv["reason"]; !present || got != credit.ReasonNoFee {
		t.Fatalf("%q: reason=%v (present=%v), want %q — the typed reason is what makes the "+
			"refusal diagnosable", event, got, ok, credit.ReasonNoFee)
	}
	if got, present := kv["serial_guard_refusals"]; !present || got != ledger.GuardFullRefusals() {
		t.Fatalf("%q: serial_guard_refusals=%v (present=%v), want the ledger's counter %d — it is "+
			"how an operator sees a serve rate above the bound the guard cap was derived against",
			event, got, present, ledger.GuardFullRefusals())
	}
	if lvl != ports.LogWarn {
		t.Fatalf("%q logged at level %v, want WARN: at INFO it is lost in the banked-receipt "+
			"stream, and at DEBUG an operator never sees it", event, lvl)
	}
	// The success line stays exactly as it is (S5) — this line is ADDITIONAL, never a
	// replacement, and must not reuse the word "banked".
	if _, _, banked := lg.last("delivery receipt banked"); !banked {
		t.Fatal("the `delivery receipt banked` line disappeared — it is an announced observable " +
			"other tiers watch; the WARN line is additional, not a replacement")
	}
}
