package node

// R2.14 — the relay-lane prepayment anchor: the NODE-tier RED-first gates
// (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
// §2.2 (a refused open records nothing), §2.4 (doors (i) self keyset, (ii) the
// guard, (iii) domain, (v) never acct(ephID), (vi) budget = Σ face never S), §5
// (guard window == keyset window; verify cost), §8 (v1 open ⇒ errRelayNoAnchor;
// dark until era-4: no self keyset ⇒ named refusal), §9 (T-1 open half, T-2 wire,
// T-3 two-relay, T-4, T-5, T-6, T-7, T-8, T-9, T-10, T-11, T-12, T-13). Build shape:
// advisory §1.3 (RelayOpen v2 + the commitment M), §1.4 (the verify ORDER: free
// guards → k bounds → sha256(Fetcher)==from → ed25519 over M → RSA under the SELF
// keyset newest-first, stop at first failure → SpendRelayAnchors all-or-nothing →
// seen-maps → budget = Σ face → admit), §1.5 (ceiling = min(count, budget) × B),
// §1.6 (settle min(count, budget) to acct(relay); log reason=anchored), §5 steps
// 5-6 (OpenRelaySession(ephID, root, S, funding, anchors, fetcherPub, sig);
// AcquireRelayAnchors; the named S5 errors).
//
// ABLATIONS that must redden (cert §9): verify under peerDemandKeys instead of
// self (T-3 two-relay); remove the all-or-nothing check (T-10); restore
// budget := S × inc (T-4, T-9); touch acct(ephID) (T-1/T-5).
//
// Every gate here is RED on main. Most fail at anchor mint ("R2.14 not built"),
// which is "fails to reach the property"; T-1 (open half), T-11 and T-13 fail on
// the refusal identity itself.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

const (
	r214Fee   = int64(50_000)
	r214Grant = int64(500_000)
	// r214KMax is MaxAnchorsPerSession at the shipped fee (⌈262,144 / 50,000⌉ = 6);
	// a literal because relaypay.MaxAnchorsPerSession does not exist yet.
	r214KMax = 6
	// relayOpenCommitmentDomain is the domain of the ed25519 commitment M
	// (advisory §1.3): sha256(domain ‖ relayID ‖ Root ‖ uint32BE(S) ‖ uint32BE(k) ‖
	// serial_1 ‖ … ‖ serial_k). The Tester's reading of the widths (uint32BE) —
	// the cert §11 sentence does not fix them. If the Builder chooses another
	// encoding, align relayOpenCommitment below to it; the BINDING property (every
	// field is under the signature) does not change.
	relayOpenCommitmentDomain = "silt/relay/open/v1"
)

// ---- fixture -----------------------------------------------------------------

type anchorRelay struct {
	node   *Node
	ident  *identity.Identity
	key    *rsa.PrivateKey // the relay's chain-committed demand key_0
	ledger *credit.Ledger
}

func (r *anchorRelay) id() ports.NodeID { return r.ident.NodeID() }

type anchorCluster struct {
	t          *testing.T
	sched      *simclock.Scheduler
	endpoint   func(ports.NodeID) *simnet.Endpoint
	chain      *chain.Chain
	attester   *identity.Identity
	relays     []*anchorRelay
	advanceSeq int
}

// newAnchorCluster builds n relays that share ONE chain whose genesis commits
// each relay's demand key_0 (a v5 IssuerKeyReg per relay), each with its OWN
// per-node ledger (the shipped topology). epochBlocks > 0 turns the epoch clock on.
// A non-nil store is attached AND loaded on relay 0's ledger.
func newAnchorCluster(t *testing.T, seed int64, n int, epochBlocks uint64, grant int64, store ports.PaidSerialStore) *anchorCluster {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	attester := identity.FromSeed(seed + 1000)

	c := chain.New(chain.Config{Quorum: 1, EpochBlocks: epochBlocks}, func(ports.NodeID) int64 { return 1 << 30 })
	relays := make([]*anchorRelay, 0, n)
	var regs []chain.IssuerKeyReg
	for i := 0; i < n; i++ {
		key := cachedRSAKey(t, i) // distinct per relay IN a cluster; cached across clusters
		ident := identity.FromSeed(seed + int64(i))
		relays = append(relays, &anchorRelay{ident: ident, key: key})
		regs = append(regs, chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))
	}
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{{Root: ports.HashBytes([]byte("r214-genesis"))}},
		IssuerKeys: regs,
	}
	chain.Sign(&g, relays[0].ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis with the issuer-key bindings: %v", err)
	}
	for i, r := range relays {
		if got, ok := c.IssuerKeyCommitment(r.id(), 0); !ok || got != demand.KeyFingerprint(&r.key.PublicKey) {
			t.Fatalf("setup: relay %d's key_0 is not committed", i)
		}
		r.node = New(r.id(), DefaultConfig(), sched, net.Endpoint(r.id()), memstore.New())
		r.node.SetSigner(r.ident.Signer())
		r.ledger = credit.New(r214Fee, grant)
		// The relay's OWN account exists on its own ledger before any session (Builder
		// fixture correction, 2026-09-04): on a live daemon the first economy read
		// (node.go EconomySelf → Balance(n.id)) registers it long before a settle.
		// Without this the settle's acct(relay) would register it WITH the faucet
		// grant, and the ledger-total oracles below would read that grant as Δ.
		r.ledger.Register(r.id())
		if i == 0 && store != nil {
			r.ledger.SetPaidSerialStore(store)
			if err := r.ledger.LoadPaidSerials(); err != nil {
				t.Fatal(err)
			}
		}
		r.node.SetLedger(r.ledger)
		r.node.EnableChain(c, r.ident.Signer())
		r.ledger.SetEpochSource(f8EpochFunc(r.node.chainEpoch)) // R2.10 / F8: the daemon's wiring, in-process
		r.node.SetDemandIssuerKey(rand.Reader, 0, r.key)
		r.node.EnableRelayAccept()
		if ks := r.node.DemandIssuerKeyset(r.id()); ks == nil || ks.Key(0) == nil {
			t.Fatalf("setup: relay %d cannot resolve its OWN committed key_0", i)
		}
		relayKeyOf.Store(r.id(), r.key)
	}
	return &anchorCluster{t: t, sched: sched, endpoint: net.Endpoint, chain: c, attester: attester, relays: relays}
}

func (cl *anchorCluster) relay() *anchorRelay { return cl.relays[0] }

// advanceEpochs appends attested blocks until relay 0's chainEpoch has moved by n
// (the composedFixture idiom).
func (cl *anchorCluster) advanceEpochs(n int) {
	cl.t.Helper()
	r := cl.relay()
	want := r.node.chainEpoch() + uint64(n)
	for i := 0; r.node.chainEpoch() < want; i++ {
		if i > n*64 {
			cl.t.Fatalf("the epoch clock did not reach %d after %d blocks", want, i)
		}
		prev, next := cl.chain.Head()
		b := &chain.Block{Version: 1, Height: next, Prev: prev,
			Entries: []ports.Entry{mkEntry(fmt.Sprintf("r214-advance-%d", cl.advanceSeq))}} // mkEntry: equivocation_test.go (manifest-bearing)
		cl.advanceSeq++
		chain.Sign(b, r.ident.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, cl.attester.Signer())}
		if err := cl.chain.Append(*b); err != nil {
			cl.t.Fatalf("advance block %d: %v", i, err)
		}
	}
}

// mintAnchor is the issuer side of a blind withdrawal under the relay's key_0 for
// issue epoch e, done directly (the burn is exercised separately through the wire
// in TestRelayAnchorsAreBoughtOnTheRelaysOwnLedger).
func (r *anchorRelay) mintAnchor(t *testing.T, e uint64) relaypay.Anchor {
	t.Helper()
	return mintAnchorUnder(t, r.key, e)
}

func (r *anchorRelay) mintAnchors(t *testing.T, e uint64, k int) []relaypay.Anchor {
	t.Helper()
	out := make([]relaypay.Anchor, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, r.mintAnchor(t, e))
	}
	return out
}

// garbageAnchors are well-sized anchors that verify under no key.
func garbageAnchors(k int) []relaypay.Anchor {
	out := make([]relaypay.Anchor, 0, k)
	for i := 0; i < k; i++ {
		s := make([]byte, blindtoken.SerialSize)
		rand.Read(s)
		g := make([]byte, 256)
		rand.Read(g)
		out = append(out, relaypay.Anchor{Serial: s, Sig: g})
	}
	return out
}

// ephemeral is a fresh session identity: ephID = sha256(pub), per the transport.
type ephemeral struct {
	id   ports.NodeID
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func newEphemeral(seed int64) ephemeral {
	ident := identity.FromSeed(seed)
	priv := ident.Signer()
	return ephemeral{id: ident.NodeID(), priv: priv, pub: priv.Public().(ed25519.PublicKey)}
}

// relayOpenCommitmentIndependent is M (advisory §1.3), recomputed independently here
// (the production form is relayrole.go relayOpenCommitment; T-11's honest-control
// subtest is what pins the two to one encoding).
func relayOpenCommitmentIndependent(relayID ports.NodeID, root []byte, S int, serials [][]byte) []byte {
	h := sha256.New()
	h.Write([]byte(relayOpenCommitmentDomain))
	h.Write(relayID[:])
	h.Write(root)
	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], uint32(S))
	h.Write(b4[:])
	binary.BigEndian.PutUint32(b4[:], uint32(len(serials)))
	h.Write(b4[:])
	for _, s := range serials {
		h.Write(s)
	}
	return h.Sum(nil)
}

func serialsOf(anchors []relaypay.Anchor) [][]byte {
	out := make([][]byte, 0, len(anchors))
	for _, a := range anchors {
		out = append(out, a.Serial)
	}
	return out
}

func freshChain(t *testing.T, tag string, S int) *relaypay.Chain {
	t.Helper()
	tip := sha256.Sum256([]byte(tag))
	c, err := relaypay.BuildChain(tip[:], S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	return c
}

// open signs M for relay r and presents the session.
func (r *anchorRelay) open(e ephemeral, root []byte, S int, anchors []relaypay.Anchor) (*RelaySession, error) {
	sig := ed25519.Sign(e.priv, relayOpenCommitmentIndependent(r.id(), root, S, serialsOf(anchors)))
	return r.node.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, anchors, e.pub, sig)
}

// ledgerTotal is Σ balances over every registered account (escrow is untouched by
// the relay lane), the node-tier form of credit's sumConserved.
func ledgerTotal(l *credit.Ledger) int64 {
	var total int64
	for _, b := range l.Balances() {
		total += b
	}
	return total
}

func minI64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// ---- T-1 (open half) -----------------------------------------------------------

// TestRelayOpenRefusesUnanchoredSession is the node half of T-1 (cert §8: "v1 open
// at a v2 relay: len(Anchors) == 0 ⇒ errRelayNoAnchor"): an open with zero anchors
// and an otherwise valid commitment is refused with the named reason, no session is
// created, and the ledger total does not move. The ledger half (GREEN by the
// interim) is core/credit TestRelaySettlementRefusesUnanchoredSession.
func TestRelayOpenRefusesUnanchoredSession(t *testing.T) {
	cl := newAnchorCluster(t, 8100, 1, 0, r214Grant, nil)
	r := cl.relay()
	before := ledgerTotal(r.ledger)
	c := freshChain(t, "t1-unanchored", 8)
	sess, err := r.open(newEphemeral(8190), c.Root(), 8, nil)
	if !errors.Is(err, errRelayNoAnchor) {
		t.Fatalf("an open with NO anchors returned (sess=%v, err=%v), want errRelayNoAnchor — an unanchored session was admitted", sess != nil, err)
	}
	if got := r.node.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("%d live sessions after a refused unanchored open, want 0", got)
	}
	if got := ledgerTotal(r.ledger); got != before {
		t.Fatalf("ledger total moved %d → %d on a refused open", before, got)
	}
}

// ---- T-2 (wire) + the anchored log line ----------------------------------------

// TestRelayAnchorsAreBoughtOnTheRelaysOwnLedger is T-2 through the WIRE: the
// fetcher's DURABLE identity D buys k anchors from the relay (AcquireRelayAnchors →
// answerDemandTokenRequest → tokenChargeFor → ChargePublish on the RELAY's ledger,
// cert §2.1 — the paying ledger is the settling ledger, INV-RELAY-CONS (iii)); a
// fresh EPHEMERAL E opens over the wire, pays c, and the relay settles. Then:
// D's balance fell by exactly k·fee; settled == min(c, k·fee); Δ Σ_L == settled −
// k·fee ≤ 0; E is never registered; and the S5 line carries reason=anchored
// (the recorded goalpost move from the interim's no-anchor; cert §1.6).
func TestRelayAnchorsAreBoughtOnTheRelaysOwnLedger(t *testing.T) {
	const k = 2
	const S = 6 // c = S × inc = 6 ≪ k·fee: partial consumption, Δ < 0
	log := &capturingLogger{}
	cl := newAnchorCluster(t, 8200, 1, 0, r214Grant, nil)
	r := cl.relay()
	r.node.SetLogger(log)

	dIdent := identity.FromSeed(8290) // the fetcher's DURABLE identity
	d := New(dIdent.NodeID(), DefaultConfig(), cl.sched, cl.endpoint(dIdent.NodeID()), memstore.New())
	d.SetSigner(dIdent.Signer())
	d.EnableChain(cl.chain, dIdent.Signer()) // the fetcher pins the relay's key against the chain
	e := newEphemeral(8291)
	eNode := New(e.id, DefaultConfig(), cl.sched, cl.endpoint(e.id), memstore.New())
	eNode.SetSigner(e.priv)
	d.Bootstrap([]ports.NodeID{r.id()}, func() {})
	eNode.Bootstrap([]ports.NodeID{r.id()}, func() {})
	r.node.Bootstrap([]ports.NodeID{d.id, e.id}, func() {})
	cl.sched.Run()

	var pinned int
	var perr error
	d.FetchDemandIssuerKeys(r.id(), func(n int, err error) { pinned, perr = n, err })
	cl.sched.Run()
	if perr != nil || pinned != 1 {
		t.Fatalf("setup: D must pin the relay's committed key_0 (pinned=%d err=%v)", pinned, perr)
	}

	r.ledger.Register(d.id)
	before := ledgerTotal(r.ledger)
	dBefore := r.ledger.Balance(d.id)

	var anchors []relaypay.Anchor
	var aerr error
	d.AcquireRelayAnchors(rand.Reader, r.id(), k, func(a []relaypay.Anchor, err error) { anchors, aerr = a, err })
	cl.sched.Run()
	if aerr != nil || len(anchors) != k {
		t.Fatalf("AcquireRelayAnchors(k=%d) = (%d anchors, %v)", k, len(anchors), aerr)
	}
	if got := r.ledger.Balance(d.id); got != dBefore-k*r214Fee {
		t.Fatalf("D's balance on the RELAY's ledger is %d after buying %d anchors, want %d − %d·fee = %d — the burn did not land on the paying ledger", got, k, dBefore, k, dBefore-k*r214Fee)
	}

	c := freshChain(t, "t2-wire", S)
	var handle uint64
	var oerr error
	eNode.OpenRelaySessionRemote(r.id(), c.Root(), S, FundingEphemeralBlind, anchors, func(h uint64, err error) { handle, oerr = h, err })
	cl.sched.Run()
	if oerr != nil || handle == 0 {
		t.Fatalf("open over the wire: handle=%d err=%v", handle, oerr)
	}
	for i := 1; i <= S; i++ {
		var perr error
		eNode.SubmitRelayPay(r.id(), handle, c.Preimage(i), i, func(_ int, err error) { perr = err })
		cl.sched.Run()
		if perr != nil {
			t.Fatalf("pay %d: %v", i, perr)
		}
	}

	paid := r.node.SettleRelaySession(handle)
	want := minI64(int64(S)*relaypay.RelayIncrementCredit, k*r214Fee)
	if paid != want {
		t.Fatalf("settled %d, want min(c = %d, k·fee = %d) = %d", paid, S*relaypay.RelayIncrementCredit, k*r214Fee, want)
	}
	after := ledgerTotal(r.ledger)
	if after-before != paid-k*r214Fee || after > before {
		t.Fatalf("Δ Σ_L = %d, want settled − k·fee = %d (≤ 0)", after-before, paid-k*r214Fee)
	}
	for _, bal := range r.ledger.Balances() {
		if bal < 0 {
			t.Fatalf("an account went negative (%d)", bal)
		}
	}
	// The ephemeral must never be registered: Balances() lists every registered
	// account; the count must equal what was registered before the session.
	if got, wantN := len(r.ledger.Balances()), 2; got != wantN {
		t.Fatalf("%d registered accounts after settle, want %d (relay, D) — the ephemeral was registered", got, wantN)
	}

	var rec *logRecord
	for i := range log.records {
		if log.records[i].event == "relay session settled" {
			rec = &log.records[i]
		}
	}
	if rec == nil {
		t.Fatal("no 'relay session settled' log record")
	}
	reason := ""
	for i := 0; i+1 < len(rec.kv); i += 2 {
		if key, _ := rec.kv[i].(string); key == "reason" {
			reason, _ = rec.kv[i+1].(string)
		}
	}
	if reason != "anchored" {
		t.Fatalf("settlement log reason = %q, want %q (S5: the recorded goalpost move from no-anchor; register it in cmd/silt/observable_contract.go)", reason, "anchored")
	}
}

// ---- T-3 ---------------------------------------------------------------------

// TestRelayCredentialIsSpentOncePerLedger is T-3 at the node tier: a second open
// on the same anchor (fresh eph + fresh root) is refused as SPENT; and the
// two-relay variant (G-A5): relay B, which holds A's key_0 PINNED AS A PEER key,
// still refuses A's anchor — verification runs under the SELF keyset only. The
// ablation "verify under peerDemandKeys[any]" reddens exactly here.
func TestRelayCredentialIsSpentOncePerLedger(t *testing.T) {
	cl := newAnchorCluster(t, 8300, 2, 0, r214Grant, nil)
	a, b := cl.relays[0], cl.relays[1]

	anchor := a.mintAnchor(t, 0)
	c1 := freshChain(t, "t3-first", 8)
	if _, err := a.open(newEphemeral(8390), c1.Root(), 8, []relaypay.Anchor{anchor}); err != nil {
		t.Fatalf("first open with a fresh anchor: %v", err)
	}
	c2 := freshChain(t, "t3-second", 8)
	sess, err := a.open(newEphemeral(8391), c2.Root(), 8, []relaypay.Anchor{anchor})
	if !errors.Is(err, errRelayAnchorSpent) {
		t.Fatalf("a SECOND session on the same anchor (fresh eph, fresh root) returned (sess=%v, err=%v), want errRelayAnchorSpent — one fee, two sessions", sess != nil, err)
	}

	// Two-relay variant: B pins A's committed key as a PEER key (the delivery-lane
	// fetcher path does this), then is offered A's anchor.
	if !b.node.pinDemandIssuerKey(a.id(), 0, &a.key.PublicKey) {
		t.Fatal("setup: B could not pin A's committed key_0 as a peer key")
	}
	fresh := a.mintAnchor(t, 0)
	c3 := freshChain(t, "t3-relay-b", 8)
	bBefore := ledgerTotal(b.ledger)
	sess, err = b.open(newEphemeral(8392), c3.Root(), 8, []relaypay.Anchor{fresh})
	if !errors.Is(err, errRelayAnchorInvalid) {
		t.Fatalf("relay B ADMITTED an anchor issued by relay A (sess=%v, err=%v), want errRelayAnchorInvalid — the burn landed on A's ledger, the settle would land on B's (cert §2.4 door (i), G-A5)", sess != nil, err)
	}
	if got := ledgerTotal(b.ledger); got != bBefore {
		t.Fatalf("B's ledger moved %d → %d on a refused foreign anchor", bBefore, got)
	}
}

// ---- T-4 ---------------------------------------------------------------------

// TestRelaySettlementIgnoresForwardedBytesIsBoundedByAnchor is T-4 at the node
// tier: Count() = S (the WHOLE chain revealed), forwarded = 0 (nothing was ever
// pumped), one anchor ⇒ settled ≤ face; the ledger total moves at settle by
// exactly settled. S is chosen ABOVE face so the S × inc budget the fix deletes
// would over-pay; the ablation "restore budget := S × inc" reddens here.
func TestRelaySettlementIgnoresForwardedBytesIsBoundedByAnchor(t *testing.T) {
	const S = int(r214Fee) + 4_096 // 54,096 increments > one face
	cl := newAnchorCluster(t, 8400, 1, 0, r214Grant, nil)
	r := cl.relay()
	anchor := r.mintAnchor(t, 0)
	c := freshChain(t, "t4-forwarded-zero", S)
	sess, err := r.open(newEphemeral(8490), c.Root(), S, []relaypay.Anchor{anchor})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	r.node.relaySessionSeq++
	handle := r.node.relaySessionSeq
	r.node.relaySessions[handle] = sess
	// Reveal the whole chain at once (bounded to S hashes by the #644 clamp). The
	// fix may refuse a claim past the budget (advisory §1.5, "either form"); the
	// settlement bound must hold either way.
	_ = sess.PayTo(c.Preimage(S), S)
	if sess.Count() == 0 {
		t.Fatal("setup: nothing was authorized")
	}
	afterOpen := ledgerTotal(r.ledger)
	relayBefore := r.ledger.Balance(r.id())

	paid := r.node.SettleRelaySession(handle) // forwarded bytes: 0 — never pumped
	if paid > r214Fee {
		t.Fatalf("settled %d > face %d with count %d and ZERO bytes forwarded — settlement is bounded by S, not by the anchor", paid, r214Fee, sess.Count())
	}
	if paid != minI64(int64(sess.Count())*relaypay.RelayIncrementCredit, r214Fee) {
		t.Fatalf("settled %d, want min(count = %d, face = %d)", paid, sess.Count(), r214Fee)
	}
	if got := ledgerTotal(r.ledger) - afterOpen; got != paid {
		t.Fatalf("the settle moved the total by %d, want exactly settled = %d", got, paid)
	}
	if got := r.ledger.Balance(r.id()); got != relayBefore+paid {
		t.Fatalf("relay balance %d, want %d + %d", got, relayBefore, paid)
	}
}

// ---- T-5 ---------------------------------------------------------------------

// TestRelaySettlementNeverLeavesAnAccountNegative is T-5 at the node tier: after
// an over-long chain settles against a k=6 budget, no account on L is below 0,
// the buyer's post-burn balance is untouched by the settle, and the ephemeral is
// never registered. A zero-balance buyer cannot acquire (the burn refuses; the
// relay issues nothing): Balances stay ≥ 0.
func TestRelaySettlementNeverLeavesAnAccountNegative(t *testing.T) {
	t.Run("over_long_chain_against_k6", func(t *testing.T) {
		const k = 6
		const S = 320_000 / 4 // 80,000 increments... keep S ≤ S_max
		cl := newAnchorCluster(t, 8500, 1, 0, r214Grant, nil)
		r := cl.relay()
		buyer := identity.FromSeed(8590).NodeID()
		r.ledger.Register(buyer)
		for i := 0; i < k; i++ {
			if err := r.ledger.ChargePublish(buyer); err != nil {
				t.Fatalf("burn %d: %v", i, err)
			}
		}
		buyerAfterBurn := r.ledger.Balance(buyer)
		anchors := r.mintAnchors(t, 0, k)
		c := freshChain(t, "t5-overlong", S)
		sess, err := r.open(newEphemeral(8591), c.Root(), S, anchors)
		if err != nil {
			t.Fatalf("open k=6: %v", err)
		}
		r.node.relaySessionSeq++
		handle := r.node.relaySessionSeq
		r.node.relaySessions[handle] = sess
		_ = sess.PayTo(c.Preimage(S), S)
		paid := r.node.SettleRelaySession(handle)
		if paid <= 0 || paid > k*r214Fee {
			t.Fatalf("settled %d, want 0 < settled ≤ k·fee = %d", paid, k*r214Fee)
		}
		for _, bal := range r.ledger.Balances() {
			if bal < 0 {
				t.Fatalf("an account is negative after settle: %d", bal)
			}
		}
		if got := r.ledger.Balance(buyer); got != buyerAfterBurn {
			t.Fatalf("the settle moved the buyer %d → %d — settlement debits nobody", buyerAfterBurn, got)
		}
		if got := len(r.ledger.Balances()); got != 2 {
			t.Fatalf("%d registered accounts, want 2 (relay, buyer) — the ephemeral was registered", got)
		}
	})

	t.Run("zero_balance_buyer_acquires_nothing", func(t *testing.T) {
		cl := newAnchorCluster(t, 8510, 1, 0, 0, nil) // grant 0
		r := cl.relay()
		dIdent := identity.FromSeed(8592)
		d := New(dIdent.NodeID(), DefaultConfig(), cl.sched, cl.endpoint(dIdent.NodeID()), memstore.New())
		d.SetSigner(dIdent.Signer())
		d.EnableChain(cl.chain, dIdent.Signer())
		d.Bootstrap([]ports.NodeID{r.id()}, func() {})
		r.node.Bootstrap([]ports.NodeID{d.id}, func() {})
		cl.sched.Run()
		d.FetchDemandIssuerKeys(r.id(), func(int, error) {})
		cl.sched.Run()
		var anchors []relaypay.Anchor
		var aerr error
		d.AcquireRelayAnchors(rand.Reader, r.id(), 1, func(a []relaypay.Anchor, err error) { anchors, aerr = a, err })
		cl.sched.Run()
		if aerr == nil && len(anchors) > 0 {
			t.Fatalf("a ZERO-balance buyer acquired %d anchor(s) — the issuance burn is not refusable", len(anchors))
		}
		for _, bal := range r.ledger.Balances() {
			if bal < 0 {
				t.Fatalf("an account went negative on a refused acquire: %d", bal)
			}
		}
	})
}

// ---- T-6 ---------------------------------------------------------------------

// TestRelayAnchorDomainIsNotADemandToken is T-6 (cert §9, §6.3): under the SAME
// key_E (the relay's key_0), the same epoch and the same serial, (a) a DEMAND
// token presented as an anchor is refused by OpenRelaySession as invalid, with no
// session and no ledger movement; (b) an ANCHOR presented to demand.Bank.Redeem
// under the relay's own keyset is not credited. One fee, one lane.
func TestRelayAnchorDomainIsNotADemandToken(t *testing.T) {
	cl := newAnchorCluster(t, 8600, 1, 0, r214Grant, nil)
	r := cl.relay()

	// (a) a real demand token under key_0, offered as an anchor.
	tok := blindTokenUnderAt(t, r.key, 0)
	before := ledgerTotal(r.ledger)
	c := freshChain(t, "t6-demand-as-anchor", 8)
	sess, err := r.open(newEphemeral(8690), c.Root(), 8, []relaypay.Anchor{{Serial: tok.Serial, Sig: tok.Sig}})
	if !errors.Is(err, errRelayAnchorInvalid) {
		t.Fatalf("a DEMAND token opened a paid relay session (sess=%v, err=%v), want errRelayAnchorInvalid — one 50,000 fee would pay the delivery server AND the relay", sess != nil, err)
	}
	if got := r.node.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("%d live sessions after the refused cross-lane open", got)
	}
	if got := ledgerTotal(r.ledger); got != before {
		t.Fatalf("ledger moved %d → %d on a refused cross-lane open", before, got)
	}

	// (b) a real anchor under key_0, offered to the delivery bank.
	anchor := r.mintAnchor(t, 0)
	keys := r.node.DemandIssuerKeyset(r.id())
	fetcher := identity.FromSeed(8691)
	receipt := demand.Ack(fetcher.Signer(), demand.Token{Serial: anchor.Serial, Sig: anchor.Sig}, ports.HashBytes([]byte("t6-object")), r.id())
	credited, _, reason := demand.NewBank().Redeem(keys, 0, demand.Token{Serial: anchor.Serial, Sig: anchor.Sig}, receipt)
	if credited {
		t.Fatal("a relay ANCHOR was credited by demand.Bank.Redeem as a delivery token — one fee paid two lanes")
	}
	if reason != "token expired or not issued" {
		t.Fatalf("Bank.Redeem refused the anchor for %q, want the verify failure \"token expired or not issued\" — the refusal must come from the domain, not a later check", reason)
	}
}

// ---- T-7 ---------------------------------------------------------------------

// TestRelayOpenRefusesCheaplyBeforeRSA is T-7 (cert §9, §5 verify cost; advisory
// §1.4 order and §4.9): the RSA verify counter (the ValidatePubHardnessRuns idiom)
// shows a guard-(ii)-refused open runs 0 modexps; a k > k_max open runs 0; a
// k = 6 GARBAGE open runs ≥ 1 and ≤ W+1 (newest-first, stop at the first failure —
// the counter must move, or it is not wired); an honest k = 6 open runs ≥ 6 and
// ≤ 6·(W+1).
func TestRelayOpenRefusesCheaplyBeforeRSA(t *testing.T) {
	const wPlus1 = int(demand.DefaultWindow) + 1
	cl := newAnchorCluster(t, 8700, 1, 0, r214Grant, nil)
	r := cl.relay()
	runs := func(f func()) int {
		before := blindtoken.RelayAnchorVerifyRuns()
		f()
		return int(blindtoken.RelayAnchorVerifyRuns() - before)
	}

	// Honest k = 6.
	honest := r.mintAnchors(t, 0, r214KMax)
	c1 := freshChain(t, "t7-honest", 8)
	e1 := newEphemeral(8790)
	var err error
	n := runs(func() { _, err = r.open(e1, c1.Root(), 8, honest) })
	if err != nil {
		t.Fatalf("honest k=6 open: %v", err)
	}
	if n < r214KMax || n > r214KMax*wPlus1 {
		t.Fatalf("honest k=6 open ran %d RSA verifies, want %d ≤ n ≤ %d (one per anchor, newest-first)", n, r214KMax, r214KMax*wPlus1)
	}

	// Guard (ii): the SAME ephemeral again, fresh root, fresh anchors: 0 modexps.
	fresh := r.mintAnchors(t, 0, 1)
	c2 := freshChain(t, "t7-reuse", 8)
	n = runs(func() { _, err = r.open(e1, c2.Root(), 8, fresh) })
	if !errors.Is(err, errRelayEphemeralReuse) {
		t.Fatalf("ephemeral reuse: err=%v, want errRelayEphemeralReuse", err)
	}
	if n != 0 {
		t.Fatalf("a guard-(ii)-refused open ran %d RSA verifies, want 0 — RSA before the free guards is the open-path CPU DoS (RT-RELAY-3)", n)
	}

	// k > k_max: refused before any modexp.
	c3 := freshChain(t, "t7-kmax", 8)
	n = runs(func() { _, err = r.open(newEphemeral(8791), c3.Root(), 8, garbageAnchors(r214KMax+1)) })
	if err == nil {
		t.Fatalf("k = %d > MaxAnchorsPerSession was admitted", r214KMax+1)
	}
	if n != 0 {
		t.Fatalf("a k > k_max open ran %d RSA verifies, want 0", n)
	}

	// k = 6 garbage: stop at the first bad anchor, ≤ W+1 modexps, ≥ 1.
	c4 := freshChain(t, "t7-garbage", 8)
	n = runs(func() { _, err = r.open(newEphemeral(8792), c4.Root(), 8, garbageAnchors(r214KMax)) })
	if !errors.Is(err, errRelayAnchorInvalid) {
		t.Fatalf("garbage k=6 open: err=%v, want errRelayAnchorInvalid", err)
	}
	if n < 1 {
		t.Fatal("a garbage open ran 0 RSA verifies — the verify counter is not wired to VerifyRelayAnchor (the T-7 hook is missing)")
	}
	if n > wPlus1 {
		t.Fatalf("a garbage k=6 open ran %d RSA verifies, want ≤ W+1 = %d (stop at the first failure)", n, wPlus1)
	}
}

// ---- T-8 ---------------------------------------------------------------------

// TestRelayAnchorGuardSurvivesRestart is T-8 at the node tier (red-team F2): spend
// an anchor on relay R (ledger attached to a durable store); build R again from
// the same store (a restart: same identity, fresh Node, fresh Ledger, loaded);
// re-presenting the anchor is refused as SPENT. An attached-but-unloaded store
// refuses opens outright.
func TestRelayAnchorGuardSurvivesRestart(t *testing.T) {
	store := &memPaidSerialStore{}
	cl := newAnchorCluster(t, 8800, 1, 0, r214Grant, store)
	r := cl.relay()
	anchor := r.mintAnchor(t, 0)
	c1 := freshChain(t, "t8-before-restart", 8)
	if _, err := r.open(newEphemeral(8890), c1.Root(), 8, []relaypay.Anchor{anchor}); err != nil {
		t.Fatalf("open before restart: %v", err)
	}
	if len(store.entries) != 1 || !bytes.Equal(store.entries[0].Serial, anchor.Serial) {
		t.Fatalf("durable store holds %d entries after the open, want exactly the spent anchor", len(store.entries))
	}

	// Restart: the SAME chain and identity, a fresh node and ledger loaded from the store.
	restarted := credit.New(r214Fee, r214Grant)
	restarted.Register(r.id())
	restarted.SetPaidSerialStore(store)
	if err := restarted.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	r2 := &anchorRelay{ident: r.ident, key: r.key, ledger: restarted}
	r2.node = New(r2.id(), DefaultConfig(), cl.sched, cl.endpoint(r2.id()), memstore.New())
	r2.node.SetSigner(r2.ident.Signer())
	r2.node.SetLedger(restarted)
	r2.node.EnableChain(cl.chain, r2.ident.Signer())
	r2.node.SetDemandIssuerKey(rand.Reader, 0, r2.key)
	r2.node.EnableRelayAccept()
	c2 := freshChain(t, "t8-after-restart", 8)
	sess, err := r2.open(newEphemeral(8891), c2.Root(), 8, []relaypay.Anchor{anchor})
	if !errors.Is(err, errRelayAnchorSpent) {
		t.Fatalf("after a restart the same anchor opened a session (sess=%v, err=%v), want errRelayAnchorSpent — restart is an eviction (F2)", sess != nil, err)
	}

	// Attached but not loaded: refuse every open.
	unloaded := credit.New(r214Fee, r214Grant)
	unloaded.SetPaidSerialStore(store)
	r3 := &anchorRelay{ident: r.ident, key: r.key, ledger: unloaded}
	r3.node = New(r3.id(), DefaultConfig(), cl.sched, cl.endpoint(r3.id()), memstore.New())
	r3.node.SetSigner(r3.ident.Signer())
	r3.node.SetLedger(unloaded)
	r3.node.EnableChain(cl.chain, r3.ident.Signer())
	r3.node.SetDemandIssuerKey(rand.Reader, 0, r3.key)
	r3.node.EnableRelayAccept()
	freshAnchor := r.mintAnchor(t, 0)
	c3 := freshChain(t, "t8-unloaded", 8)
	if sess, err := r3.open(newEphemeral(8892), c3.Root(), 8, []relaypay.Anchor{freshAnchor}); err == nil || sess != nil {
		t.Fatal("an open on an attached-but-UNLOADED guard was admitted — the ledger cannot know what it already accepted")
	}
	if got := r3.node.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("%d live sessions on the unloaded relay", got)
	}
}

// memPaidSerialStore is the node package's in-memory PaidSerialStore fake (the
// credit package's memStore is not visible here).
type memPaidSerialStore struct{ entries []ports.PaidSerial }

func (m *memPaidSerialStore) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), m.entries...), nil
}
func (m *memPaidSerialStore) Append(p ports.PaidSerial) error {
	m.entries = append(m.entries, p)
	return nil
}
func (m *memPaidSerialStore) Compact(live []ports.PaidSerial) error {
	m.entries = append([]ports.PaidSerial(nil), live...)
	return nil
}

// ---- T-9 ---------------------------------------------------------------------

// TestRelayCeilingNeverExceedsBudget is T-9 (cert §9; advisory §1.5): with k = 1
// (budget = face = 50,000 increments) and a chain longer than the budget, paying
// to count > budget leaves AuthorizedBytes() ≤ budget × RelayIncrementBytes —
// the pump never forwards past the funded budget. Below the budget the ceiling
// tracks the count (liveness).
func TestRelayCeilingNeverExceedsBudget(t *testing.T) {
	const budget = r214Fee // one anchor, in increments (RelayIncrementCredit = 1)
	const S = int(budget) + 2
	cl := newAnchorCluster(t, 8900, 1, 0, r214Grant, nil)
	r := cl.relay()
	anchor := r.mintAnchor(t, 0)
	c := freshChain(t, "t9-ceiling", S)
	sess, err := r.open(newEphemeral(8990), c.Root(), S, []relaypay.Anchor{anchor})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := sess.PayTo(c.Preimage(100), 100); err != nil {
		t.Fatalf("pay to 100 (below budget): %v", err)
	}
	if got := sess.AuthorizedBytes(); got != 100*relaypay.RelayIncrementBytes {
		t.Fatalf("ceiling %d after 100 paid increments below budget, want %d", got, 100*relaypay.RelayIncrementBytes)
	}
	// Past the budget: the fix may refuse the claim or clamp the ceiling ("either
	// form"); the ceiling bound holds either way.
	_ = sess.PayTo(c.Preimage(S), S)
	if got, max := sess.AuthorizedBytes(), budget*relaypay.RelayIncrementBytes; got > max {
		t.Fatalf("AuthorizedBytes() = %d after paying to count %d > budget %d, want ≤ budget × B = %d — the relay forwards bytes it will never be paid for, and the funding cap is invisible on the wire", got, S, budget, max)
	}
	if got, want := sess.AuthorizedBytes(), minI64(int64(sess.Count()), budget)*relaypay.RelayIncrementBytes; got != want {
		t.Fatalf("AuthorizedBytes() = %d, want min(count = %d, budget = %d) × B = %d", got, sess.Count(), budget, want)
	}
}

// ---- T-10 --------------------------------------------------------------------

// TestRelayOpenRefusalRecordsNoAnchor is T-10 (cert §2.2, §9): k = 2 where anchor
// 2 is already spent ⇒ the open is refused AND anchor 1 is still spendable in a
// later open; the refused open left no guard entry (anchor 1 opens), no durable
// append (the store holds exactly the anchors of ADMITTED sessions), and no
// seen-map entry (the refused ephemeral and root are admissible later). The
// ablation "remove the all-or-nothing check" reddens here.
func TestRelayOpenRefusalRecordsNoAnchor(t *testing.T) {
	store := &memPaidSerialStore{}
	cl := newAnchorCluster(t, 9000, 1, 0, r214Grant, store)
	r := cl.relay()
	a1, a2 := r.mintAnchor(t, 0), r.mintAnchor(t, 0)

	c1 := freshChain(t, "t10-spend-a2", 8)
	if _, err := r.open(newEphemeral(9090), c1.Root(), 8, []relaypay.Anchor{a2}); err != nil {
		t.Fatalf("setup: spending a2 alone: %v", err)
	}
	entriesAfterFirst := len(store.entries)
	// OpenRelaySession returns the session; only handleRelayOpen inserts it into the
	// table (Builder correction, 2026-09-04: the original "want 1" assumed an insert).
	liveAfterFirst := r.node.relayLiveSessionCountForTest()

	e2 := newEphemeral(9091)
	c2 := freshChain(t, "t10-refused", 8)
	sess, err := r.open(e2, c2.Root(), 8, []relaypay.Anchor{a1, a2})
	if !errors.Is(err, errRelayAnchorSpent) {
		t.Fatalf("an open carrying a spent anchor returned (sess=%v, err=%v), want errRelayAnchorSpent", sess != nil, err)
	}
	if got := len(store.entries); got != entriesAfterFirst {
		t.Fatalf("the REFUSED open appended to the durable store (%d → %d entries) — anchor 1 is burned with no session", entriesAfterFirst, got)
	}
	if got := r.node.relayLiveSessionCountForTest(); got != liveAfterFirst {
		t.Fatalf("%d live sessions after the refused open, want %d (unchanged)", got, liveAfterFirst)
	}

	// Anchor 1 is still spendable in a LATER open.
	c3 := freshChain(t, "t10-a1-later", 8)
	if _, err := r.open(newEphemeral(9092), c3.Root(), 8, []relaypay.Anchor{a1}); err != nil {
		t.Fatalf("anchor 1 is no longer spendable after the refused open: %v — the fetcher lost anchor 1 because anchor 2 was spent", err)
	}
	// The refused ephemeral and root left no seen-map entry.
	c4 := freshChain(t, "t10-eph2-later", 8)
	if _, err := r.open(e2, c4.Root(), 8, []relaypay.Anchor{r.mintAnchor(t, 0)}); err != nil {
		t.Fatalf("the ephemeral of a REFUSED open cannot open later: %v — the refusal recorded a seen-map entry", err)
	}
	if _, err := r.open(newEphemeral(9093), c2.Root(), 8, []relaypay.Anchor{r.mintAnchor(t, 0)}); err != nil {
		t.Fatalf("the root of a REFUSED open cannot be used later: %v — the refusal recorded a seen-map entry", err)
	}
}

// ---- T-11 --------------------------------------------------------------------

// TestRelayOpenCommitmentBindsRelayRootAndSerials is T-11 (cert §9; advisory §1.3):
// M binds relayID, Root, S, k and every serial. Tampering any of them (the
// relayID case is a replay of A's commitment at relay B) is refused on the
// ed25519 check BEFORE any RSA verify (counter delta 0); sha256(Fetcher) != from
// is refused likewise. The anchors are garbage on purpose: the property under
// test is the commitment, and it must be decided before the anchors are touched.
func TestRelayOpenCommitmentBindsRelayRootAndSerials(t *testing.T) {
	cl := newAnchorCluster(t, 9100, 2, 0, r214Grant, nil)
	a, b := cl.relays[0], cl.relays[1]
	const S = 8
	anchors := garbageAnchors(2)
	root := freshChain(t, "t11-root", S).Root()
	root2 := freshChain(t, "t11-root-2", S).Root()

	type tamper struct {
		name    string
		present func(e ephemeral) (*RelaySession, error)
		want    error
	}
	signFor := func(e ephemeral, relayID ports.NodeID, r []byte, s int, an []relaypay.Anchor) []byte {
		return ed25519.Sign(e.priv, relayOpenCommitmentIndependent(relayID, r, s, serialsOf(an)))
	}
	cases := []tamper{
		{"relayID: A's commitment replayed at relay B", func(e ephemeral) (*RelaySession, error) {
			sig := signFor(e, a.id(), root, S, anchors)
			return b.node.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, anchors, e.pub, sig)
		}, errRelayOpenSigInvalid},
		{"Root", func(e ephemeral) (*RelaySession, error) {
			sig := signFor(e, a.id(), root, S, anchors)
			return a.node.OpenRelaySession(e.id, root2, S, FundingEphemeralBlind, anchors, e.pub, sig)
		}, errRelayOpenSigInvalid},
		{"S", func(e ephemeral) (*RelaySession, error) {
			sig := signFor(e, a.id(), root, S, anchors)
			return a.node.OpenRelaySession(e.id, root, S+1, FundingEphemeralBlind, anchors, e.pub, sig)
		}, errRelayOpenSigInvalid},
		{"k: signed over 2 serials, presented 1", func(e ephemeral) (*RelaySession, error) {
			sig := signFor(e, a.id(), root, S, anchors)
			return a.node.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, anchors[:1], e.pub, sig)
		}, errRelayOpenSigInvalid},
		{"serial: one byte flipped in a presented anchor", func(e ephemeral) (*RelaySession, error) {
			sig := signFor(e, a.id(), root, S, anchors)
			flipped := []relaypay.Anchor{{Serial: append([]byte(nil), anchors[0].Serial...), Sig: anchors[0].Sig}, anchors[1]}
			flipped[0].Serial[0] ^= 0x01
			return a.node.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, flipped, e.pub, sig)
		}, errRelayOpenSigInvalid},
		{"sha256(Fetcher) != from", func(e ephemeral) (*RelaySession, error) {
			other := newEphemeral(9199)
			sig := signFor(other, a.id(), root, S, anchors)
			return a.node.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, anchors, other.pub, sig)
		}, errRelayFetcherMismatch},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEphemeral(9190 + int64(i))
			before := blindtoken.RelayAnchorVerifyRuns()
			sess, err := tc.present(e)
			if !errors.Is(err, tc.want) {
				t.Fatalf("tampered %s: (sess=%v, err=%v), want %v — the commitment does not bind this field", tc.name, sess != nil, err, tc.want)
			}
			if n := blindtoken.RelayAnchorVerifyRuns() - before; n != 0 {
				t.Fatalf("tampered %s: %d RSA verifies ran before the ed25519 refusal, want 0", tc.name, n)
			}
		})
	}
	t.Run("honest commitment is not refused on the signature", func(t *testing.T) {
		e := newEphemeral(9198)
		_, err := a.open(e, root, S, anchors)
		if errors.Is(err, errRelayOpenSigInvalid) || errors.Is(err, errRelayFetcherMismatch) {
			t.Fatalf("an HONEST commitment was refused on the signature: %v — the encoding of M in relayOpenCommitment and the node disagree (align the widths, see relayOpenCommitmentDomain)", err)
		}
	})
}

// ---- T-12 --------------------------------------------------------------------

// TestRelayAnchorGuardWindowMatchesKeysetWindow is T-12 at the node tier (the
// ledger twin is in core/credit): with the epoch clock on, an anchor spent at
// epoch 0 is still refused as SPENT at epoch W (the guard remembers exactly as
// long as the keyset verifies), a fresh epoch-0 anchor still opens at epoch W (key_0
// is in the window), and at epoch W+1 a fresh epoch-0 anchor is refused as INVALID
// (the self keyset pruned key_0 — evicted ⇒ expired). SpendRelayAnchors is driven
// with the same n.chainEpoch() the keyset is pruned with; the boundary is observed
// on both sides at once.
func TestRelayAnchorGuardWindowMatchesKeysetWindow(t *testing.T) {
	const W = int(demand.DefaultWindow)
	cl := newAnchorCluster(t, 9200, 1, 2, r214Grant, nil) // EpochBlocks = 2
	r := cl.relay()
	if got := r.node.chainEpoch(); got != 0 {
		t.Fatalf("setup: epoch %d, want 0", got)
	}
	a0 := r.mintAnchor(t, 0)
	c1 := freshChain(t, "t12-epoch0", 8)
	if _, err := r.open(newEphemeral(9290), c1.Root(), 8, []relaypay.Anchor{a0}); err != nil {
		t.Fatalf("open at epoch 0: %v", err)
	}

	cl.advanceEpochs(W)
	if got := r.node.chainEpoch(); got != uint64(W) {
		t.Fatalf("epoch %d after advancing, want %d", got, W)
	}
	c2 := freshChain(t, "t12-epochW-spent", 8)
	if _, err := r.open(newEphemeral(9291), c2.Root(), 8, []relaypay.Anchor{a0}); !errors.Is(err, errRelayAnchorSpent) {
		t.Fatalf("at epoch W = %d the epoch-0 anchor spent at epoch 0 returned err=%v, want errRelayAnchorSpent — the guard forgot while the keyset still verifies (the eviction pump)", W, err)
	}
	b0 := r.mintAnchor(t, 0)
	c3 := freshChain(t, "t12-epochW-fresh", 8)
	if _, err := r.open(newEphemeral(9292), c3.Root(), 8, []relaypay.Anchor{b0}); err != nil {
		t.Fatalf("at epoch W = %d a fresh epoch-0 anchor was refused: %v — key_0 must still be in the window", W, err)
	}

	cl.advanceEpochs(1)
	c0 := r.mintAnchor(t, 0)
	c4 := freshChain(t, "t12-epochW1", 8)
	sess, err := r.open(newEphemeral(9293), c4.Root(), 8, []relaypay.Anchor{c0})
	if !errors.Is(err, errRelayAnchorInvalid) {
		t.Fatalf("at epoch W+1 = %d a fresh epoch-0 anchor returned (sess=%v, err=%v), want errRelayAnchorInvalid — the self keyset did not prune key_0 (an anchor outlives its guard entry)", W+1, sess != nil, err)
	}
}

// ---- T-13 --------------------------------------------------------------------

// TestRelayOpenRefusesWithoutSelfKeyset is T-13 (cert §8, §9; advisory finding E):
// no chain / a chain with no committed key_E for this relay / a committed key but
// keys never scheduled (relay-accept on without the demand-key schedule) ⇒ every
// open is refused with the NAMED reason (the dark-lane direction), no session, no
// ledger movement. The commitment is honest and the anchors are garbage: the
// refusal must name the missing keyset, not the anchors.
func TestRelayOpenRefusesWithoutSelfKeyset(t *testing.T) {
	present := func(t *testing.T, n *Node, relayID ports.NodeID, l *credit.Ledger) {
		t.Helper()
		before := ledgerTotal(l)
		e := newEphemeral(9390)
		root := freshChain(t, "t13", 8).Root()
		anchors := garbageAnchors(1)
		sig := ed25519.Sign(e.priv, relayOpenCommitmentIndependent(relayID, root, 8, serialsOf(anchors)))
		sess, err := n.OpenRelaySession(e.id, root, 8, FundingEphemeralBlind, anchors, e.pub, sig)
		if !errors.Is(err, errRelayNoIssuerKey) {
			t.Fatalf("open with no self keyset returned (sess=%v, err=%v), want errRelayNoIssuerKey — the lane must be dark with a NAMED reason", sess != nil, err)
		}
		if got := n.relayLiveSessionCountForTest(); got != 0 {
			t.Fatalf("%d live sessions", got)
		}
		if got := ledgerTotal(l); got != before {
			t.Fatalf("ledger moved %d → %d", before, got)
		}
	}

	t.Run("no chain", func(t *testing.T) {
		n := newRelayTestNode(9301)
		l := credit.New(r214Fee, r214Grant)
		n.SetLedger(l)
		n.EnableRelayAccept()
		present(t, n, n.id, l)
	})

	t.Run("chain without a committed key for this relay", func(t *testing.T) {
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.DefaultConfig())
		ident := identity.FromSeed(9302)
		c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
		g := chain.Block{Version: chain.BlockVersionWitnessable, Height: 0,
			Entries: []ports.Entry{{Root: ports.HashBytes([]byte("t13-no-reg"))}}}
		chain.Sign(&g, ident.Signer())
		if err := c.AppendGenesis(g); err != nil {
			t.Fatal(err)
		}
		n := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
		n.SetSigner(ident.Signer())
		l := credit.New(r214Fee, r214Grant)
		n.SetLedger(l)
		n.EnableChain(c, ident.Signer())
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		n.SetDemandIssuerKey(rand.Reader, 0, key) // uncommitted: the self pin refuses it
		n.EnableRelayAccept()
		if ks := n.DemandIssuerKeyset(n.id); ks != nil && ks.Key(0) != nil {
			t.Fatal("setup: an uncommitted self key was pinned")
		}
		present(t, n, n.id, l)
	})

	t.Run("committed key but keys never scheduled (finding E)", func(t *testing.T) {
		f := newIssuerKeyFixture(t, 9303) // chain commits key_0; SetDemandIssuerKey is NEVER called
		l := credit.New(r214Fee, r214Grant)
		f.nd.SetLedger(l)
		f.nd.EnableRelayAccept()
		present(t, f.nd, f.issuer, l)
	})
}
