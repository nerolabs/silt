package node

// R0.4b-8 — the COMPOSED expiry boundary, the gap the per-layer triple leaves open.
//
// WHAT THE EXISTING GATES DO AND DO NOT COVER. `TestSerialGuard_EvictThenReRedeem
// MintsZero` and `_EvictionPumpIsNotSelfFinancing` run entirely at epoch 0: they are
// green because the CAP refuses, not because anything expires, so they encode "the cap
// never forgets a live serial" and never cross the boundary. The boundary is covered
// PER LAYER — credit side by `TestSerialGuard_ExpiryFreesTheCap`, demand side by
// `TestKeysetRejectsExpiredEpoch` — and every node/sim fixture runs at `EpochBlocks =
// 0`, where the consensus epoch is 0 forever and expiry never fires at all.
//
// THE COMPOSED CLAIM this file gates: on ONE ledger with ONE epoch clock, the set of
// serials the credit layer FORGETS is a subset of the set every honest redeemer
// REJECTS upstream. So "forgotten ⇒ un-redeemable" holds, and eviction cannot fund a
// second payout. That is the property the pump-closure rests on, and it is a property
// of the COMPOSITION: neither layer's own test can see it.
//
// SHAPE. Two servers on one shared ledger and one shared chain with `EpochBlocks > 0`.
// Server A issues, banks a delivery at epoch E and is paid. The chain advances past
// E + W. The same token is then presented again — and must be refused UPSTREAM, at the
// demand window, before any credit path.
//
// B-9 (2026-09-07) MOVED WHERE THE PUMP CLOSES. Deliveries are SESSIONS, and a session
// anchor verifies under the server's OWN committed key only (`verifyDeliveryAnchors`:
// `n.DemandIssuerKeyset(n.id)`, the own-key rule). So the cross-server pump — A's token
// re-presented at B, whose spent set is empty — is closed STRUCTURALLY, whatever the
// window says: B refuses an A-issued anchor fresh or expired, with the same reason. The
// composed WINDOW claim (forgotten ⇒ un-redeemable) is therefore exercised at the ISSUER
// itself: A's own expired token, whose serial the shared guard has swept, must be
// refused at A's keyset before any credit path. B here is a fully operational server —
// it holds its OWN committed issuer key and A's pinned key — so its refusal is the
// own-key rule and never darkness (blind PE on 7f2ac97 measured the earlier B arms
// satisfied by "no self keyset"; that is what this fixture closes).
//
// ABLATIONS (all re-run 2026-09-07 after the B-9 re-home, results recorded per gate):
//   (i)   the demand-layer window in `core/demand/keyset.go` (a no-op `Prune` plus an
//         unbounded `VerifyInWindow` scan) → the first gate RED on "A refused the expired
//         anchor DOWNSTREAM of the window (… token-backdated)": the shared guard's epoch
//         watermark catches the backdated spend, so the arm reads the refusal REASON to
//         tell the window from the guard;
//   (ii)  the own-key rule (`n.DemandIssuerKeyset(n.id)` → `(n.demandIssuer)`) →
//         `TestComposedSessions_ForeignIssuersFreshAnchorIsRefused` RED on "server B
//         banked a FRESH A-issued anchor". The B arms inside gates 1 and 3 stay GREEN
//         under (ii) — with A's keys resolved, B's window refuses the EXPIRED foreign
//         token too — so those arms are defence in depth; the fresh-anchor gate is the
//         one that holds the rule;
//   (iii) the epoch dropped from the demand FDH input (core/blindtoken demandMsg) → the
//         re-dating gate RED on "server A re-dated its own epoch-0 token".
//
// `TestComposedExpiryBoundary_EvictionIsClosedAtBothLayers` then drives the red-team's
// eviction pump across the boundary with the demand window BYPASSED, and shows the
// credit layer refuses anyway — defence in depth, via the R0.4b-5 epoch watermark. Its
// own teeth are in `core/credit` (`TestEpochWatermark_*`), where the watermark can be
// isolated from the caller's current epoch.

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
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

// composedFixture is two servers sharing ONE ledger and ONE chain whose epoch clock
// actually advances. A is the demand issuer whose tokens the tests mint; B resolves
// A's keys (the colluding-second-server shape) AND is an issuer in its own right, so a
// refusal at B is the own-key rule, never a missing keyset.
type composedFixture struct {
	a, b           *Node
	aIdent, bIdent *identity.Identity
	fetcher        *identity.Identity
	ledger         *credit.Ledger
	chain          *chain.Chain
	issuerPriv     *rsa.PrivateKey // A's
	bIssuerPriv    *rsa.PrivateKey // B's own
	object         ports.Hash
	knownIDs       []ports.NodeID
	roots          []ports.Hash
	// advanceSeq keeps every advance block's entry root distinct ACROSS calls. The
	// chain refuses a duplicate root, so a per-call counter collides the moment a
	// test advances the clock more than once.
	advanceSeq int
}

const composedFee = 50_000

// composedEpochBlocks is 2. It must be > 0 (that is the whole point — every other
// node fixture runs at 0, where the consensus epoch is 0 forever and expiry never
// fires) and > 1 so the clock still reads epoch 0 at the post-genesis head, which is
// the epoch the genesis binding commits a key for.
const composedEpochBlocks = 2

func newComposedFixture(t *testing.T) *composedFixture {
	t.Helper()
	issuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	bIssuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("B's issuer key: %v", err)
	}
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())

	aIdent := identity.FromSeed(6101)
	bIdent := identity.FromSeed(6102)
	fetcher := identity.FromSeed(6103)

	// ONE chain, shared: one epoch clock, which is the model the composed claim is
	// stated over. Legacy mode (MinBond 0) keeps block production to plain signed
	// blocks; the epoch clock is what this fixture is for.
	c := chain.New(chain.Config{Quorum: 1, EpochBlocks: composedEpochBlocks},
		func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("composed-genesis"))}},
		IssuerKeys: []chain.IssuerKeyReg{
			chain.SignIssuerKeyReg(aIdent.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)),
			chain.SignIssuerKeyReg(bIdent.Signer(), 0, demand.KeyFingerprint(&bIssuerPriv.PublicKey)),
		},
	}
	chain.Sign(&g, aIdent.Signer())
	if gerr := c.AppendGenesis(g); gerr != nil {
		t.Fatalf("genesis committing the issuer-key binding: %v", gerr)
	}

	ledger := credit.New(composedFee, composedFee*4) // headroom for several withdrawals
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetSigner(id.Signer())
		nd.SetLedger(ledger) // ONE ledger — the shared paidSerial guard set
		nd.EnableChain(c, id.Signer())
		nd.EnableDemandBank(aIdent.NodeID())         // both RESOLVE A's keys (B's pinned foreign keyset is the pump's premise)
		nd.EnableDeliverySessions(10 * ports.Second) // B-9: deliveries are sessions; each server accepts only its OWN anchors
		return nd
	}
	a, b := mk(aIdent), mk(bIdent)
	ledger.SetEpochSource(f8EpochFunc(a.chainEpoch)) // R2.10 / F8: a and b share c, so one clock
	a.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	b.SetDemandIssuerKey(rand.Reader, 0, bIssuerPriv)
	if ks := b.DemandIssuerKeyset(bIdent.NodeID()); ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: B must hold its OWN committed key_0 — otherwise its refusals are darkness, not the own-key rule")
	}

	// B pins A's key_0 through the real cross-check while epoch 0 is still in A's
	// served window. Nothing later in this test re-fetches: the point is that a key
	// legitimately held STOPS verifying once the window moves past it.
	var pinned int
	var perr error
	b.FetchDemandIssuerKeys(aIdent.NodeID(), func(n int, e error) { pinned, perr = n, e })
	sched.Run()
	if perr != nil || pinned != 1 {
		t.Fatalf("setup: B must pin A's key_0 (pinned=%d err=%v)", pinned, perr)
	}

	f := &composedFixture{
		a: a, b: b, aIdent: aIdent, bIdent: bIdent, fetcher: fetcher,
		ledger: ledger, chain: c, issuerPriv: issuerPriv, bIssuerPriv: bIssuerPriv,
		object: ports.HashBytes([]byte("composed-object-root")),
	}
	f.knownIDs = []ports.NodeID{aIdent.NodeID(), bIdent.NodeID(), fetcher.NodeID()}
	f.roots = []ports.Hash{f.object}
	for _, id := range f.knownIDs {
		ledger.Register(id)
	}
	return f
}

// sum is Σbalances + Σescrow over every account and escrow root this fixture touches.
// A conserved step leaves it unchanged; an unfunded mint raises it.
func (f *composedFixture) sum() int64 {
	total := int64(0)
	for _, id := range f.knownIDs {
		total += f.ledger.Balance(id)
	}
	for _, r := range f.roots {
		total += f.ledger.EscrowBalance(r)
	}
	// B-9: the session's unsettled remainder is a pending DEPOSIT (released at anchor
	// expiry), so it is part of the conserved total.
	total += f.ledger.DeliverySettlementStats().PendingRefundCredits
	return total
}

// advanceEpochs appends the blocks needed to move the shared epoch clock forward by
// n epochs, and checks it actually moved.
func (f *composedFixture) advanceEpochs(t *testing.T, n int) {
	t.Helper()
	want := f.a.chainEpoch() + uint64(n)
	for i := 0; f.a.chainEpoch() < want; i++ {
		if i > n*composedEpochBlocks+2 {
			t.Fatalf("the epoch clock did not reach %d after %d blocks", want, i)
		}
		prev, next := f.chain.Head()
		b := &chain.Block{Version: 1, Height: next, Prev: prev,
			Entries: []ports.Entry{mkEntry(fmt.Sprintf("composed-advance-%d", f.advanceSeq))}}
		f.advanceSeq++
		chain.Sign(b, f.aIdent.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, f.bIdent.Signer())}
		if err := f.chain.Append(*b); err != nil {
			t.Fatalf("advance block %d: %v", i, err)
		}
	}
}

// mintToken performs a real blind withdrawal against key_0, binding issue epoch 0
// into the signed message, and returns the token. The fetcher pays the fee, which is
// the credit the redeem later conserves.
func (f *composedFixture) mintToken(t *testing.T) demand.Token {
	t.Helper()
	return f.mintTokenAt(t, 0, f.issuerPriv)
}

// mintTokenAt withdraws under priv for ISSUE EPOCH epoch — the (b1) shape: the epoch
// is inside the blind-signed message, so the token verifies under the pair
// (priv, epoch) and under no other epoch, whatever else priv is bound to.
func (f *composedFixture) mintTokenAt(t *testing.T, epoch uint64, priv *rsa.PrivateKey) demand.Token {
	t.Helper()
	serial := make([]byte, 32)
	if _, err := rand.Read(serial); err != nil {
		t.Fatalf("serial: %v", err)
	}
	pub := &priv.PublicKey
	blinded, secret, err := demand.Withdraw(rand.Reader, pub, epoch, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if err := f.ledger.ChargePublish(f.fetcher.NodeID()); err != nil {
		t.Fatalf("the fetcher must pay the withdrawal fee: %v", err)
	}
	tok, uerr := demand.Unblind(pub, epoch, serial, demand.SignWithdrawal(rand.Reader, priv, blinded), secret)
	if uerr != nil {
		t.Fatalf("unblind: %v", uerr)
	}
	return tok
}

// present presents token at `server` as a SESSION anchor (B-9: the flat receipt is
// retired) and reports whether the server banked the delivery.
func (f *composedFixture) present(t *testing.T, server *Node, token demand.Token) bool {
	t.Helper()
	before := server.WitnessedIncrements(f.object)
	ok := sessionPresent(t, server, f.fetcher, token, f.object)
	return ok && server.WitnessedIncrements(f.object) > before
}

// openRefusal opens a session at server with token and returns the open's refusal (nil
// if it was admitted; the session is then closed so the fetcher may present again). The
// tests read WHICH rule refused: the own-key rule at B is errDeliveryAnchorInvalid, and
// so is the window at A — the arms are told apart by the SERVER, and the header explains
// why that is the property.
func (f *composedFixture) openRefusal(t *testing.T, server *Node, token demand.Token) error {
	t.Helper()
	s, err := server.OpenDeliverySession(f.fetcher.NodeID(), demand.SignSessionOpen(f.fetcher.Signer(), server.id, []demand.Token{token}))
	if err == nil {
		server.closeDeliverySession(s.handle, "test")
	}
	return err
}

// TestComposedSessions_ForeignIssuersFreshAnchorIsRefused is the gate on the own-key rule
// itself — the seam that, under sessions, closes the cross-server pump (blind PE fix 6,
// 2026-09-07: changing `n.DemandIssuerKeyset(n.id)` to `(n.demandIssuer)` left every
// package green). B has A's key_0 PINNED (its resolved foreign issuer) and its own key; a
// FRESH, in-window A-issued anchor must be refused at B naming the own-key rule, move
// nothing, and then be banked at A — so the refusal is the rule and not the token.
//
// ABLATION (ii) → RED on "server B banked a FRESH A-issued anchor".
func TestComposedSessions_ForeignIssuersFreshAnchorIsRefused(t *testing.T) {
	f := newComposedFixture(t)
	if ks := f.b.DemandIssuerKeyset(f.aIdent.NodeID()); ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: B must hold A's pinned key_0 — the pump's premise is a redeemer that CAN verify the foreign token")
	}
	token := f.mintToken(t)
	base := f.sum()
	err := f.openRefusal(t, f.b, token)
	if err == nil || f.present(t, f.b, token) {
		t.Fatalf("server B banked a FRESH A-issued anchor (open err=%v): the own-key rule is gone and the cross-server pump is open", err)
	}
	if !errors.Is(err, errDeliveryAnchorInvalid) {
		t.Fatalf("B refused with %q, want the own-key rule %q (a missing keyset is darkness, not the rule)", err, errDeliveryAnchorInvalid)
	}
	if got := f.sum(); got != base {
		t.Fatalf("a refused foreign anchor must move nothing: Σ moved by %+d", got-base)
	}
	if f.b.WitnessedIncrements(f.object) != 0 {
		t.Fatal("B's demand observable moved on a refused anchor")
	}
	// The premise: the same token is good — its issuer banks it.
	if !f.present(t, f.a, token) {
		t.Fatal("server A must bank its own fresh token")
	}
}

// TestComposedExpiryBoundary_EvictedSerialIsRefusedUpstream is the R0.4b-8 gate.
func TestComposedExpiryBoundary_EvictedSerialIsRefusedUpstream(t *testing.T) {
	f := newComposedFixture(t)
	if f.a.chainEpoch() != 0 {
		t.Fatalf("setup: the epoch clock must start at 0, got %d", f.a.chainEpoch())
	}

	base := f.sum()
	token := f.mintToken(t)
	if !f.present(t, f.a, token) {
		t.Fatal("server A must bank the receipt at the issuing epoch")
	}
	// Conserved: the fetcher's fee moved to A's balance plus the object's escrow.
	if got := f.sum(); got != base {
		t.Fatalf("the first redeem must be conserved: Σ moved by %+d", got-base)
	}
	paid := f.sum()

	// Cross the boundary: at current = W+1 the issuing epoch 0 has left the window.
	issued := uint64(0)
	f.advanceEpochs(t, int(demand.DefaultWindow)+1)
	if cur := f.a.chainEpoch(); cur-issued <= uint64(demand.DefaultWindow) {
		t.Fatalf("setup: the clock must be past the window, at %d with W=%d", cur, demand.DefaultWindow)
	}

	// (a) The credit layer has FORGOTTEN the serial: a sweep at this clock evicts it.
	// Proven by the ledger's own boundary, exercised through the public API in the
	// ablation test below (a forgotten serial is one that would pay again).

	// (b) Every honest redeemer REJECTS it upstream. At B — which holds A's pinned key_0,
	// its own key, and an EMPTY spent set, the cross-server pump's whole premise — the
	// refusal is the own-key rule (B-9), before the window is consulted.
	if berr := f.openRefusal(t, f.b, token); berr == nil || !errors.Is(berr, errDeliveryAnchorInvalid) || f.present(t, f.b, token) {
		t.Fatalf("the pump: server B banked (or refused for the wrong reason: %v) a token whose issuing epoch has left the window", berr)
	}
	// THE WINDOW ARM, at the ISSUER itself: A's keyset has pruned key_0, so the expired
	// anchor fails at open — the demand window, upstream of any credit path — while the
	// shared guard has already swept its serial (that is "forgotten ⇒ un-redeemable").
	// The REASON is read: the shared guard's epoch watermark (R0.4b-5) would refuse the
	// backdated spend downstream even with the window gone, so "not banked" alone cannot
	// tell the window from the guard. ABLATION (i) → RED on the reason line.
	aerr := f.openRefusal(t, f.a, token)
	if aerr == nil || f.present(t, f.a, token) {
		t.Fatal("server A banked its own token after the issuing epoch left the window")
	}
	if !errors.Is(aerr, errDeliveryAnchorInvalid) {
		t.Fatalf("A refused the expired anchor DOWNSTREAM of the window (%v): the keyset did not refuse it — "+
			"the composed claim rests on the window firing first, before any credit path", aerr)
	}
	if ks := f.a.DemandIssuerKeyset(f.aIdent.NodeID()); ks == nil || ks.Key(0) != nil {
		t.Fatal("the expired key must be pruned from A's OWN keyset at the current epoch")
	}
	if got := f.sum(); got != paid {
		t.Fatalf("a refused re-presentation must move nothing: Σ moved by %+d", got-paid)
	}
	if ks := f.b.DemandIssuerKeyset(f.aIdent.NodeID()); ks.Key(0) != nil {
		t.Fatal("the expired key must be pruned from the keyset at the current epoch")
	}
}

// composedMaxPaidSerial mirrors the unexported credit.maxPaidSerial floor. It is
// duplicated rather than exported because the test only needs "enough to fill the
// guard set"; the fill loop asserts the eviction actually happened, so a drift in the
// real constant surfaces as a loud failure, not a silent pass.
const composedMaxPaidSerial = 65_536

// TestComposedExpiryBoundary_EvictionIsClosedAtBothLayers drives the red-team's
// eviction pump across the epoch boundary at the NODE layer, with the demand-layer
// window bypassed (the credit layer called directly with the true issuing epoch,
// which is what "the redeemer did not enforce the window" reduces to), and requires
// the second payout to be refused anyway.
//
// TWO ORDERING FACTS make this test what it is, both verified against the code:
//
//  1. `RedeemDeliveryCredit` tests membership in `paidSerial` BEFORE calling
//     `reservePaidSerial`, which is what sweeps. So an expired serial is never
//     forgotten on its own next redeem; some other redeem must sweep first.
//  2. `reservePaidSerial` returns early while the set is under the cap and sweeps
//     ONLY when it is full. So expiry evicts nothing until the guard set fills.
//
// Together: the credit layer forgets a serial only under cap pressure, which is
// exactly the red-team's eviction-pump setup, and this test reproduces it one epoch
// window later.
//
// WHAT MAKES IT NON-VACUOUS. Two steps assert the machinery actually engaged before
// the final refusal is read: the cap must REFUSE a fresh live serial while everything
// is in-window, and the post-boundary redeem must PAY, which is only possible if the
// sweep ran and dropped the expired generation. If either drifts, this fails loudly
// instead of passing for the wrong reason.
func TestComposedExpiryBoundary_EvictionIsClosedAtBothLayers(t *testing.T) {
	f := newComposedFixture(t)
	token := f.mintToken(t)
	if !f.present(t, f.a, token) {
		t.Fatal("server A must bank the receipt at the issuing epoch")
	}
	paid := f.sum()

	// CONTROL, in-window: the credit-layer serial guard alone refuses a second payout,
	// with no help from the window. This shows what follows measures EXPIRY, not
	// merely the absence of a guard.
	cur := f.a.chainEpoch()
	if got := nodePaidOnLane(f.ledger, f.b.id, f.fetcher.NodeID(), f.object,
		token.Serial, 0); got != 0 {
		t.Fatalf("in-window: the serial guard must refuse a second payout, paid %d", got)
	}
	if got := f.sum(); got != paid {
		t.Fatalf("a refused in-window redeem must move nothing: Σ moved by %+d", got-paid)
	}

	// Set up the eviction. The fill uses a server, fetcher and escrow root OUTSIDE the
	// tracked set, so it does not disturb the conservation measurement.
	//
	//  (i)   fill the guard set to the cap at the ISSUING epoch, so every entry —
	//        including the target — expires together;
	//  (ii)  advance past the window;
	//  (iii) one redeem at the new epoch finds the set full, sweeps, and drops the
	//        whole expired generation, the target with it.
	filler := identity.FromSeed(6104).NodeID()
	fillFetcher := identity.FromSeed(6105).NodeID()
	fillRoot := ports.HashBytes([]byte("composed-fill-root"))
	serial := make([]byte, 32)
	fill := func(i int, epoch uint64) int64 {
		serial[0], serial[1], serial[2] = byte(i), byte(i>>8), byte(i>>16)
		return nodePaidOnLane(f.ledger, filler, fillFetcher, fillRoot,
			append([]byte(nil), serial...), epoch)

	}
	for i := 0; i < composedMaxPaidSerial-1; i++ {
		fill(i, cur)
	}
	// The set is now full of LIVE serials: even a fresh one is refused rather than
	// evicting a still-redeemable entry. That is the guard's own no-FIFO property,
	// re-confirmed here at the node layer.
	if got := fill(-1, cur); got != 0 {
		t.Fatalf("at a cap full of live serials a fresh redeem paid %d, want 0", got)
	}

	f.advanceEpochs(t, int(demand.DefaultWindow)+1)
	cur = f.a.chainEpoch()
	if got := fill(-2, cur); got <= 0 {
		t.Fatalf("after the window advanced the sweep must free the cap, paid %d — "+
			"without eviction here the rest of this test proves nothing", got)
	}
	swept := f.sum()

	// THE GATE: on the live ledger the evicted serial is refused anyway. The epoch
	// watermark (R0.4b-5) has moved past issuedEpoch + W, so a backdated redeem cannot
	// collect a second payout even with the demand window bypassed.
	if got := nodePaidOnLane(f.ledger, f.b.id, f.fetcher.NodeID(), f.object,
		token.Serial, 0); got != 0 {
		t.Fatalf("the eviction pump re-opened: an evicted, expired serial paid %d", got)
	}
	if delta := f.sum() - swept; delta != 0 {
		t.Fatalf("a refused backdated redeem must move nothing: Σ moved by %+d", delta)
	}
}

// ---------------------------------------------------------------------------
// GATE G/I — the composed boundary under a RE-REGISTERED key (red-team break 1).
// ---------------------------------------------------------------------------

// commitIssuerKeyAt commits a registration binding priv's fingerprint to epoch for
// server A's identity, on a v5 block. This is what a RESTART does: the persisted key
// is re-installed for the new boot epoch, the reg lands, and the chain now binds ONE
// fingerprint to TWO epochs. Nothing forbids it — validateIssuerKeys never compares a
// fingerprint against another epoch's commitment, and the §2.3 refutation showed it
// cannot be made to (the committed band is pruned, so distinctness is unenforceable
// beyond 2W+1 without unbounded state).
func (f *composedFixture) commitIssuerKeyAt(t *testing.T, epoch uint64, priv *rsa.PrivateKey) {
	t.Helper()
	// The restart INSTALLS the persisted key for the new boot epoch — which is what
	// stages the registration AND what makes A serve key_epoch to redeemers. Doing
	// only the on-chain half would leave B unable to pin it, and this gate would then
	// pass for the wrong reason (the ordinary window, not the epoch binding).
	f.a.SetDemandIssuerKey(rand.Reader, epoch, priv)
	var regs []chain.IssuerKeyReg
	for _, r := range f.a.pendingIssuerKeys {
		if r.Epoch == epoch {
			regs = append(regs, r)
		}
	}
	if len(regs) == 0 {
		t.Fatalf("the restart staged no registration for epoch %d", epoch)
	}
	prev, next := f.chain.Head()
	b := &chain.Block{Height: next, Prev: prev,
		Entries:    []ports.Entry{mkEntry(fmt.Sprintf("composed-reg-%d", epoch))},
		IssuerKeys: regs}
	if err := f.chain.PopulateEra4Roots(b); err != nil {
		t.Fatalf("era-4 roots for the key registration at epoch %d: %v", epoch, err)
	}
	chain.Sign(b, f.aIdent.Signer())
	// B attests, as it has on every advance block, so it is ALREADY in validatorsSeen
	// — which matters because that set is inside the v5 root the proposer populated
	// before gathering. An attester whose first attestation this were would move the
	// committed root out from under the signature (the R-BOX-ATTESTS harness hazard).
	b.PrepareQC = []chain.Attestation{
		chain.AttestAt(b, f.aIdent.Signer(), 0, chain.PhasePrepare), // the proposer's authorship vote (#432/I5)
		chain.AttestAt(b, f.bIdent.Signer(), 0, chain.PhasePrepare),
	}
	b.Atts = []chain.Attestation{chain.AttestAt(b, f.bIdent.Signer(), 0, chain.PhasePrecommit)}
	if err := f.chain.Append(*b); err != nil {
		t.Fatalf("commit key_%d: %v", epoch, err)
	}
	if _, ok := f.chain.IssuerKeyCommitment(f.aIdent.NodeID(), epoch); !ok {
		t.Fatalf("key_%d was not committed", epoch)
	}
}

// TestComposedBoundary_SameFingerprintAtTwoEpochsDoesNotRedateTokens is GATE G/I —
// the composed boundary gate (R0.4b-8) driven through the RE-REGISTRATION the
// red-team found, at the node layer, on one shared ledger and one epoch clock.
//
// THE ATTACK. Withdraw at epoch 0. Pay on server A. Restart: the SAME persisted key
// is committed again, for epoch 3. Advance past epoch 0 + W, so the credit guard has
// swept its epoch-0 entry. Re-present the epoch-0 token to server B, whose spent set
// is empty. Before (b1) the token verified — re-dated to epoch 3, the newest epoch
// that key was held for — and B collected a second full payout: +fee per re-presented
// serial with nothing charged behind it.
//
// THE GATE. The token must be refused at the demand window, the credit layer must
// never be reached, and Σ(balances+escrow) must be exactly unchanged. Then the §2.3
// refutation is driven too: a FRESH same-fingerprint registration at 2W+1, after the
// epoch-0 commitment has been pruned out of the band, must not revive it either.
//
// UNDER SESSIONS (B-9) the re-dating pump is a SAME-SERVER pump: A itself holds key_3 and
// has pruned key_0, and A's spent set (the shared guard) has swept the epoch-0 serial. So
// the arm that measures the epoch binding is A re-presented with its own epoch-0 token;
// the B arm measures the own-key rule (B refuses the foreign anchor whatever its epoch).
//
// ABLATION (iii): drop the epoch from the demand FDH input (core/blindtoken demandMsg) →
// RED on "server A re-dated its own epoch-0 token", with Σ up by fee−skim.
func TestComposedBoundary_SameFingerprintAtTwoEpochsDoesNotRedateTokens(t *testing.T) {
	f := newComposedFixture(t)
	token := f.mintToken(t) // withdrawn for issue epoch 0, under key_0
	if !f.present(t, f.a, token) {
		t.Fatal("server A must bank the receipt at the issuing epoch")
	}
	paid := f.sum()

	// The restart: the SAME key, re-registered for a later epoch. One fingerprint,
	// two committed epochs.
	f.advanceEpochs(t, 3)
	f.commitIssuerKeyAt(t, f.a.chainEpoch(), f.issuerPriv)
	fp0, ok0 := f.chain.IssuerKeyCommitment(f.aIdent.NodeID(), 0)
	fp3, ok3 := f.chain.IssuerKeyCommitment(f.aIdent.NodeID(), f.a.chainEpoch())
	if !ok0 || !ok3 || fp0 != fp3 {
		t.Fatalf("setup: the chain must bind ONE fingerprint to TWO epochs (ok %v/%v, equal %v)",
			ok0, ok3, fp0 == fp3)
	}
	// B re-resolves the issuer's window, so it legitimately holds the re-registered
	// key for the newer epoch too — the adversary's best case.
	var pinned int
	f.b.FetchDemandIssuerKeys(f.aIdent.NodeID(), func(n int, _ error) { pinned = n })
	f.a.clock.(interface{ Run() }).Run()
	if pinned == 0 {
		t.Fatal("setup: B pinned no key after the re-registration")
	}

	// Cross the boundary for the ORIGINAL issuing epoch — and NOT past the
	// re-registered one. At epoch W+1 = 5 the epoch-0 key has left the window while
	// key_3 is still held, which is precisely the re-dating window: without the epoch
	// in the signed message the epoch-0 token verifies under key_3 and is banked.
	f.advanceEpochs(t, int(demand.DefaultWindow)+1-3)
	if cur := f.b.chainEpoch(); cur != demand.DefaultWindow+1 {
		t.Fatalf("setup: expected epoch %d, at %d", demand.DefaultWindow+1, cur)
	}
	if ks := f.b.DemandIssuerKeyset(f.aIdent.NodeID()); ks == nil || ks.Key(3) == nil || ks.Key(0) != nil {
		t.Fatal("setup: B must hold the RE-REGISTERED key_3 and no longer key_0 — that is " +
			"the exact configuration in which a token can be re-dated")
	}
	if ks := f.a.DemandIssuerKeyset(f.aIdent.NodeID()); ks == nil || ks.Key(3) == nil || ks.Key(0) != nil {
		t.Fatal("setup: A must hold its OWN re-registered key_3 and no longer key_0 — the " +
			"same-server re-dating configuration")
	}
	aerr := f.openRefusal(t, f.a, token)
	if aerr == nil || f.present(t, f.a, token) {
		t.Fatal("THE PUMP: server A re-dated its own epoch-0 token at epoch " +
			fmt.Sprint(f.a.chainEpoch()) + ". The same key is committed for a later epoch, so " +
			"without the issue epoch inside the signed message the token is re-dated to that " +
			"epoch while the credit guard has already swept its entry — a second full payout " +
			"off one withdrawal fee.")
	}
	if !errors.Is(aerr, errDeliveryAnchorInvalid) {
		t.Fatalf("A refused the epoch-0 token downstream of the keyset (%v): the epoch binding did not refuse it", aerr)
	}
	if berr := f.openRefusal(t, f.b, token); berr == nil || !errors.Is(berr, errDeliveryAnchorInvalid) || f.present(t, f.b, token) {
		t.Fatalf("server B banked (or refused for the wrong reason: %v) an epoch-0 A-issued token — the own-key rule is gone", berr)
	}
	if got := f.sum(); got != paid {
		t.Fatalf("a refused re-presentation must move nothing: Σ moved by %+d", got-paid)
	}

	// The §2.3 refutation, driven: at 2W+1 the epoch-0 commitment has been pruned out
	// of the committed band, so a FRESH registration of the same fingerprint is
	// admissible again. That is why a distinctness validity rule could not have closed
	// this — and it must still not revive the token.
	f.advanceEpochs(t, int(demand.DefaultWindow))
	cur := f.a.chainEpoch()
	// The prune is PAYLOAD-DRIVEN (red-team re-break F1, 2026-09-03): a block carrying no
	// registrations writes nothing in the issuerKeyCommit keyspace, not even a delete, so
	// the epoch-0 commitment leaves the band on the next REGISTRATION-carrying block
	// rather than on the next epoch turn. That is the whole point of the change — a
	// height-driven delete is a committed write the floor box's O(payload) fold cannot
	// reproduce and its scope gate cannot see. The §2.3 refutation is unaffected: the
	// same fresh registration that prunes epoch 0 is the one that would revive the token.
	f.commitIssuerKeyAt(t, cur, f.issuerPriv)
	if _, still := f.chain.IssuerKeyCommitment(f.aIdent.NodeID(), 0); still {
		t.Fatal("setup: the epoch-0 commitment should have been pruned out of the band by " +
			"the registration-carrying block just committed")
	}
	f.b.FetchDemandIssuerKeys(f.aIdent.NodeID(), func(int, error) {})
	f.a.clock.(interface{ Run() }).Run()
	if f.present(t, f.a, token) {
		t.Fatalf("at epoch %d a fresh same-fingerprint registration revived an epoch-0 token at its "+
			"own issuer — the pump's period merely lengthened to 2W+1", cur)
	}
	if f.present(t, f.b, token) {
		t.Fatalf("at epoch %d server B banked a foreign epoch-0 token — the own-key rule is gone", cur)
	}
	if got := f.sum(); got != paid {
		t.Fatalf("Σ moved by %+d at the 2W+1 replay", got-paid)
	}
}

// nodePaidOnLane is the node-tier twin of core/credit's settleOnLane: the SESSION-LANE
// walk that replaced the retired flat leg (credit.RedeemDeliveryCredit, deleted in C1
// 2026-09-08). Open a one-anchor delivery session on (issuedEpoch, serial) at server,
// settle its whole budget for fetcher on root, and return the credits PAID.
//
// A refusal at the OPEN pays nothing and records nothing — which is the property these
// boundary tests measure, so the two-call shape is the mechanism and not a wrapper.
func nodePaidOnLane(l *credit.Ledger, server, fetcher ports.NodeID, root ports.Hash, serial []byte, issuedEpoch uint64) int64 {
	face, _ := l.SpendDeliveryAnchors(server, []ports.RelayAnchor{{Epoch: issuedEpoch, Serial: serial}})
	if face == 0 {
		return 0
	}
	_, paid, _ := l.SettleDelivery(server, fetcher, root, face/credit.DeliveryIncrementCredit, face, 0)
	return paid
}
