package node

// Blocker 4 of the blind PE ruling on B-9 (2026-09-07): on the session lane the
// paid-serial guard can fill with LIVE entries ONLY at open and fund (spendDeliveryAnchors
// is called from nowhere else; Ledger.SettleDelivery has no guard-full path). So the
// operator's signal for "serve rate above the bound the cap was derived against" must be
// emitted THERE, and the settle-path WARN must fire only for refusals that passed the
// owner, commitment and signature checks — a pre-auth refusal is one unauthenticated
// message per WARN line with no rate limit on logf.
//
// The guard-full condition is induced with a ledger double that refuses every spend with
// ReasonGuardFull: filling a real guard takes 65,536 live entries and would make this a
// minute-long test for a one-line contract. The double changes nothing else — it embeds
// the real ledger.

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

// guardFullLedger is the real ledger with the anchor spend forced to ReasonGuardFull
// while refuse is set, counting the refusals the way the real guard does.
type guardFullLedger struct {
	*credit.Ledger
	refuse   bool
	refusals int64
}

func (g *guardFullLedger) SpendDeliveryAnchors(server ports.NodeID, anchors []ports.RelayAnchor) (int64, string) {
	if g.refuse {
		g.refusals++
		return 0, credit.ReasonGuardFull
	}
	return g.Ledger.SpendDeliveryAnchors(server, anchors)
}

func (g *guardFullLedger) GuardFullRefusals() int64 { return g.refusals + g.Ledger.GuardFullRefusals() }

type guardFullRig struct {
	nd      *Node
	sched   *simclock.Scheduler
	ledger  *guardFullLedger
	lg      *levelCaptureLog
	server  *identity.Identity
	fetcher *identity.Identity
	key     *rsa.PrivateKey
	obj     ports.Hash
}

func newGuardFullRig(t *testing.T) *guardFullRig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	server, fetcher := identity.FromSeed(9411), identity.FromSeed(9412)
	nd := New(server.NodeID(), DefaultConfig(), sched, net.Endpoint(server.NodeID()), memstore.New())
	ledger := &guardFullLedger{Ledger: credit.New(50_000, 0)}
	nd.SetLedger(ledger)
	nd.SetSigner(server.Signer())
	c := c3Chain(t, 1, server.Signer(), chain.SignIssuerKeyReg(server.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))
	nd.EnableChain(c, server.Signer())
	nd.SetDemandIssuerKey(rand.Reader, 0, key)
	nd.EnableDemandBank(server.NodeID())
	nd.EnableDeliverySessions(10 * ports.Second)
	ledger.Register(server.NodeID())
	ledger.Register(fetcher.NodeID())
	if ks := nd.DemandIssuerKeyset(server.NodeID()); ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: the committed issuer key was not pinned")
	}
	lg := &levelCaptureLog{}
	nd.SetLogger(lg)
	return &guardFullRig{nd: nd, sched: sched, ledger: ledger, lg: lg, server: server, fetcher: fetcher, key: key,
		obj: ports.HashBytes([]byte("guardfull-object"))}
}

func (r *guardFullRig) token(t *testing.T) demand.Token {
	t.Helper()
	serial := make([]byte, 32)
	if _, err := rand.Read(serial); err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, &r.key.PublicKey, r.nd.chainID(), 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, uerr := demand.Unblind(&r.key.PublicKey, r.nd.chainID(), 0, serial, demand.SignWithdrawal(rand.Reader, r.key, r.nd.chainID(), blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	return tok
}

func wireBlob(t *testing.T, m interface{ Marshal() ([]byte, error) }) []byte {
	t.Helper()
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// warnLines counts the WARN-level lines the capture holds.
func (c *levelCaptureLog) warnLines() (n int, events []string) {
	for i, lvl := range c.levels {
		if lvl == ports.LogWarn {
			n++
			events = append(events, c.events[i])
		}
	}
	return n, events
}

const guardFullMarker = "delivery anchor refused: guard full"

func TestGuardFullOpenLogsTheWarnMarker(t *testing.T) {
	r := newGuardFullRig(t)
	fid := r.fetcher.NodeID()

	// OPEN at a guard full of live entries.
	r.ledger.refuse = true
	r.nd.handle(fid, ports.Message{Kind: ports.MsgDeliveryOpen, Ephemeral: true,
		Data: wireBlob(t, demand.SignSessionOpen(r.fetcher.Signer(), r.server.NodeID(), []demand.Token{r.token(t)}))})
	kv, lvl, ok := r.lg.last(guardFullMarker)
	if !ok {
		t.Fatalf("no %q line on a guard-full OPEN refusal: the paid-serial guard filling with live entries is "+
			"silent to the operator\nlines seen: %v", guardFullMarker, r.lg.events)
	}
	if lvl != ports.LogWarn {
		t.Fatalf("%q at level %v, want WARN", guardFullMarker, lvl)
	}
	if got := kv["serial_guard_refusals"]; got != int64(1) {
		t.Fatalf("%q: serial_guard_refusals=%v, want the ledger's counter 1", guardFullMarker, got)
	}
	if got := kv["step"]; got != "open" {
		t.Fatalf("%q: step=%v, want open", guardFullMarker, got)
	}

	// FUND at a guard full of live entries: a live session first, then the guard fills.
	r.ledger.refuse = false
	sess, oerr := r.nd.OpenDeliverySession(fid, demand.SignSessionOpen(r.fetcher.Signer(), r.server.NodeID(), []demand.Token{r.token(t)}))
	if oerr != nil {
		t.Fatalf("setup: open with the guard free: %v", oerr)
	}
	r.ledger.refuse = true
	r.nd.handle(fid, ports.Message{Kind: ports.MsgDeliveryFund, Ephemeral: true,
		Data: wireBlob(t, demand.SignSessionFund(r.fetcher.Signer(), r.server.NodeID(), sess.handle, []demand.Token{r.token(t)}))})
	kv, _, ok = r.lg.last(guardFullMarker)
	if !ok || kv["step"] != "fund" || kv["serial_guard_refusals"] != int64(2) {
		t.Fatalf("guard-full FUND refusal: marker present=%v step=%v serial_guard_refusals=%v, want fund / 2", ok, kv["step"], kv["serial_guard_refusals"])
	}
	// The status surface carries the same number (the marker alone is not a surface).
	if got := r.nd.DeliverySettlementStats().GuardFullRefusals; got != r.ledger.Ledger.DeliverySettlementStats().GuardFullRefusals {
		t.Fatalf("DeliverySettlementStats.GuardFullRefusals = %d, not the ledger's", got)
	}
}

// TestPreAuthSettleRefusalIsNotAWarn: a settle that fails BEFORE the owner/commitment/
// signature checks — reachable by any peer — must not produce the WARN marker, and an
// ordinary admission refusal at open must not either.
func TestPreAuthSettleRefusalIsNotAWarn(t *testing.T) {
	r := newGuardFullRig(t)
	stranger := identity.FromSeed(9413)
	fid := r.fetcher.NodeID()

	// (a) Unknown handle from an unauthenticated peer.
	r.nd.handle(stranger.NodeID(), ports.Message{Kind: ports.MsgDeliverySettle, Ephemeral: true,
		Data: wireBlob(t, demand.AckSession(stranger.Signer(), 777, []byte("no such commitment"), r.obj, r.server.NodeID(), 1))})
	// (b) Another fetcher's live session (not the owner).
	sess, oerr := r.nd.OpenDeliverySession(fid, demand.SignSessionOpen(r.fetcher.Signer(), r.server.NodeID(), []demand.Token{r.token(t)}))
	if oerr != nil {
		t.Fatalf("setup: open: %v", oerr)
	}
	r.nd.handle(stranger.NodeID(), ports.Message{Kind: ports.MsgDeliverySettle, Ephemeral: true,
		Data: wireBlob(t, demand.AckSession(stranger.Signer(), sess.handle, sess.commitment, r.obj, r.server.NodeID(), 1))})
	// (c) A garbage anchor at open (not guard-full): an admission refusal, Debug only.
	r.nd.handle(stranger.NodeID(), ports.Message{Kind: ports.MsgDeliveryOpen, Ephemeral: true,
		Data: wireBlob(t, demand.SignSessionOpen(stranger.Signer(), r.server.NodeID(), []demand.Token{{Serial: make([]byte, 32), Sig: make([]byte, 256)}}))})

	if n, events := r.lg.warnLines(); n != 0 {
		t.Fatalf("%d WARN line(s) from unauthenticated refusals (%v): one attacker message = one WARN line, unbounded", n, events)
	}
	if _, _, ok := r.lg.last("delivery settle refused"); !ok {
		t.Fatal("the pre-auth settle refusal left no Debug trace at all")
	}
	if _, _, ok := r.lg.last("delivery admission refused"); !ok {
		t.Fatal("the admission refusal left no Debug trace at all")
	}
	// The premise: the SAME session, post-auth, does produce the announced marker.
	r.nd.handle(fid, ports.Message{Kind: ports.MsgDeliverySettle, Ephemeral: true,
		Data: wireBlob(t, demand.AckSession(r.fetcher.Signer(), sess.handle, sess.commitment, r.obj, r.server.NodeID(), 1_000_000))})
	if _, lvl, ok := r.lg.last("delivery receipt paid NO credit"); !ok || lvl != ports.LogWarn {
		t.Fatalf("the post-auth refusal (count above budget) did not produce the WARN marker (present=%v level=%v)", ok, lvl)
	}
}
