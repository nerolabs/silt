package node

// R2.9 — the paid DELIVERY SESSION's node-tier RED-first gates. Binding spec: the
// G-R212-8 certification §3.1 (C1–C10), §6 (settle-monotone, the idle reaper, receipt
// v3), §8 (G-λ-8-1…10) —
// silt-agent-memory/researcher/reviews/research-outcome/R2.9-G-R212-8-delivery-anchor-quantization-RESEARCH-CERTIFICATION-2026-09-06.md —
// and the 2026-09-04 build-questions certification §5 (B-7, B-8, B-9, B-13).
// G-λ-8-1 / G-λ-8-2 live in cmd/silt (they need core/relaypay); G-λ-8-6 and G-λ-8-8 in
// core/credit; G-λ-8-10 is OPEN (named residual): its lift (b), returning the face at
// close, IS the G-6 owner call; its lift (a), recording the face only at the first
// verified increment, is REFUTED by G-λ-8-9 (durable-before-admission closes the restart
// double-spend) — so it is not buildable now on either ground.
//
// ABLATIONS that must redden (run and recorded in the PR): drop the `count·p ≤ budget`
// ceiling or make the counter non-monotone (G-λ-8-3); delete the handle on the first
// settle (G-λ-8-4); reap on the admit stamp instead of the last settlement (G-λ-8-5);
// add a map[ports.Hash]… field to DeliverySession (G-λ-8-7); admit before the durable
// append (G-λ-8-9); verify the anchor with VerifyAnchorInWindow (B-7); drop a field from
// receiptMsgV3 (B-8).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"reflect"
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
)

const r29Idle = 10 * ports.Second

// mintDemandTokenUnder is the issuer side of a blind DEMAND withdrawal under key for
// issue epoch e, done directly (the burn is exercised over the wire by the e2e).
func mintDemandTokenUnder(t *testing.T, key *rsa.PrivateKey, e uint64) demand.Token {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, &key.PublicKey, e, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := demand.Unblind(&key.PublicKey, e, serial, demand.SignWithdrawal(rand.Reader, key, blinded), secret)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func mintDemandTokensFor(t *testing.T, server *Node, e uint64, k int) []demand.Token {
	t.Helper()
	v, ok := relayKeyOf.Load(server.id)
	if !ok {
		t.Fatal("fixture: server has no committed key (build it with commitSelfDemandKey)")
	}
	out := make([]demand.Token, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, mintDemandTokenUnder(t, v.(*rsa.PrivateKey), e))
	}
	return out
}

// deliveryPairForTest: a SERVER holding a chain-committed demand key_0, a ledger, the
// demand bank and the session lane with a set idle window; a DURABLE fetcher node whose
// signer commits the open (sha256(pub) == its NodeID). No balance is pre-funded: the
// only money reaching the ledger is what an anchor purchase burns — here the tokens are
// minted directly and the fetcher is registered so the ledger's conservation oracle has
// a fixed account set.
func deliveryPairForTest(t *testing.T, serverLog ports.Logger) (fetcher, server *Node, ledger *credit.Ledger, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	fID := identity.FromSeed(7101)
	sID := identity.FromSeed(7102)
	fetcher = New(fID.NodeID(), DefaultConfig(), sched, net.Endpoint(fID.NodeID()), memstore.New())
	fetcher.SetSigner(fID.Signer())
	server = New(sID.NodeID(), DefaultConfig(), sched, net.Endpoint(sID.NodeID()), memstore.New())
	if serverLog != nil {
		server.SetLogger(serverLog)
	}
	ledger = credit.New(50_000, 0)
	ledger.Register(sID.NodeID())
	ledger.Register(fID.NodeID())
	server.SetLedger(ledger)
	commitSelfDemandKey(t, server, sID, cachedRSAKey(t, 0))
	server.EnableDemandBank(server.id)
	server.EnableDeliverySessions(r29Idle)
	fetcher.Bootstrap([]ports.NodeID{sID.NodeID()}, func() {})
	server.Bootstrap([]ports.NodeID{fID.NodeID()}, func() {})
	sched.Run()
	return fetcher, server, ledger, sched
}

func openOverWire(t *testing.T, fetcher, server *Node, sched *simclock.Scheduler, anchors []demand.Token) (uint64, []byte) {
	t.Helper()
	var handle uint64
	var commitment []byte
	var oerr error
	fetcher.OpenDeliverySessionRemote(server.id, anchors, func(h uint64, m []byte, e error) { handle, commitment, oerr = h, m, e })
	sched.Run()
	if oerr != nil || handle == 0 {
		t.Fatalf("open over the wire: handle %d err %v", handle, oerr)
	}
	return handle, commitment
}

func settleOverWire(t *testing.T, fetcher, server *Node, sched *simclock.Scheduler, handle uint64, m []byte, obj ports.Hash, count uint64) (int64, error) {
	t.Helper()
	var settled int64
	var serr error
	fetcher.SubmitDeliverySettle(server.id, handle, m, obj, count, func(s int64, e error) { settled, serr = s, e })
	sched.Run()
	return settled, serr
}

// TestSessionSettlesMonotonicallyAndNeverAboveBudget — G-λ-8-3. Many settlements over the
// wire: Σ settled ≤ Σ face; a lower or re-presented count pays exactly 0; a count the
// budget cannot fund is refused whole (top up first); the ledger's own conservation
// oracle moves by settled − Σ face over the cycle.
func TestSessionSettlesMonotonicallyAndNeverAboveBudget(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	s, _ := server.DeliverySessionForTest(handle)
	if s.Budget() != 50_000 {
		t.Fatalf("budget %d, want one face", s.Budget())
	}
	obj := ports.HashBytes([]byte("g8-3"))
	var sum int64
	for _, c := range []uint64{8, 24, 24, 16, 40} { // 8, +16, replay, LOWER, +16
		settled, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, c)
		if err != nil {
			t.Fatalf("count %d: %v", c, err)
		}
		sum += settled
		switch c {
		case 24:
			if settled != 0 && sum != 24 {
				t.Fatalf("replay of count 24 settled %d, want 0", settled)
			}
		case 16:
			if settled != 0 {
				t.Fatalf("a LOWER count settled %d, want 0 by arithmetic", settled)
			}
		}
	}
	if sum != 40 || s.Settled() != 40 || s.Count() != 40 {
		t.Fatalf("Σ settled %d, session (settled %d, count %d), want 40/40/40", sum, s.Settled(), s.Count())
	}
	// Above the budget: refused whole, the session unchanged.
	if _, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, 50_001); err == nil || s.Count() != 40 {
		t.Fatalf("a count above budget/p was not refused (err %v, count %d)", err, s.Count())
	}
	// Exactly the budget: settles, and the session closes as exhausted — nothing to burn.
	base := ledger.DeliverySettlementStats()
	if settled, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, 50_000); err != nil || settled != 50_000-40 {
		t.Fatalf("settling to the budget: (%d, %v), want %d", settled, err, 50_000-40)
	}
	if _, live := server.DeliverySessionForTest(handle); live {
		t.Fatal("an exhausted session stayed live")
	}
	if st := ledger.DeliverySettlementStats(); st.BurnedCredits != base.BurnedCredits || st.SettledCredits != 50_000 {
		t.Fatalf("exhaustion burned %d (want 0) / settled %d (want 50,000)", st.BurnedCredits-base.BurnedCredits, st.SettledCredits)
	}
	if ledger.Balance(server.id) != 50_000-50_000*credit.SkimNum/credit.SkimDen || ledger.EscrowBalance(obj) != 50_000*credit.SkimNum/credit.SkimDen {
		t.Fatalf("server %d / escrow %d, want Σ face − skim / skim", ledger.Balance(server.id), ledger.EscrowBalance(obj))
	}
}

// TestSessionSpansObjectsAndIsNotClosedByASettlement — G-λ-8-4 (C7). Two objects settle
// on one session; the session stays live between them and its budget carries.
func TestSessionSpansObjectsAndIsNotClosedByASettlement(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	a, b := ports.HashBytes([]byte("obj-a")), ports.HashBytes([]byte("obj-b"))
	if s, err := settleOverWire(t, fetcher, server, sched, handle, m, a, 10); err != nil || s != 10 {
		t.Fatalf("object a: (%d, %v)", s, err)
	}
	if _, live := server.DeliverySessionForTest(handle); !live {
		t.Fatal("the session was closed by its first settlement (the literal B-10 build, REFUTED)")
	}
	if s, err := settleOverWire(t, fetcher, server, sched, handle, m, b, 15); err != nil || s != 5 {
		t.Fatalf("object b: (%d, %v), want the 5-increment delta", s, err)
	}
	s, _ := server.DeliverySessionForTest(handle)
	if s.Settled() != 15 || s.Budget() != 50_000 {
		t.Fatalf("after two objects: settled %d budget %d", s.Settled(), s.Budget())
	}
	// The skim routes to the OBJECT of each receipt (C6) and floors on the SESSION's
	// cumulative settled value (G-SKIM): a = ⌊10/8⌋ − ⌊0/8⌋ = 1, b = ⌊15/8⌋ − ⌊10/8⌋ = 0 —
	// the CUMULATIVE rule; the aggregate ⌊15/8⌋ = 1 is exact. (A per-settlement floor gives
	// the same two numbers by coincidence; the third object below is what distinguishes
	// the rules — G-SKIM-6.)
	if ledger.EscrowBalance(a) != 1 || ledger.EscrowBalance(b) != 0 {
		t.Fatalf("skims per object (a %d, b %d), want (1, 0) under the cumulative rule", ledger.EscrowBalance(a), ledger.EscrowBalance(b))
	}
}

// TestSkimAggregatesExactlyAcrossObjectsInOneSession — G-SKIM-6. Three objects at deltas
// 10, 5, 5 (cumulative 10, 15, 20): the per-object split is (1, 0, 1) and the aggregate is
// ⌊20/8⌋ = 2. Ablation: a per-settlement floor ⇒ (1, 0, 0), aggregate 1; routing the
// boundary credit to the FIRST object moves the split.
func TestSkimAggregatesExactlyAcrossObjectsInOneSession(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	a, b, c := ports.HashBytes([]byte("skim-a")), ports.HashBytes([]byte("skim-b")), ports.HashBytes([]byte("skim-c"))
	for _, step := range []struct {
		obj   ports.Hash
		count uint64
	}{{a, 10}, {b, 15}, {c, 20}} {
		if _, err := settleOverWire(t, fetcher, server, sched, handle, m, step.obj, step.count); err != nil {
			t.Fatal(err)
		}
	}
	ea, eb, ec := ledger.EscrowBalance(a), ledger.EscrowBalance(b), ledger.EscrowBalance(c)
	if ea != 1 || eb != 0 || ec != 1 || ea+eb+ec != 20*credit.SkimNum/credit.SkimDen {
		t.Fatalf("per-object skims (%d, %d, %d), aggregate %d — want (1, 0, 1) and ⌊20/8⌋ = 2: the skim floors on the session's cumulative value, attributed to the object whose settlement crosses the boundary", ea, eb, ec, ea+eb+ec)
	}
}

// TestSessionSkimIsPartitionIndependent — G-SKIM-3, the attack named. Two sessions at two
// servers settle the same 40 increments: one in 5 deltas of 8, one in 40 deltas of 1. The
// two escrow totals are EQUAL and both ⌊40/8⌋ = 5. Ablation: a per-settlement floor ⇒ 5 vs 0.
func TestSessionSkimIsPartitionIndependent(t *testing.T) {
	run := func(deltas []uint64) int64 {
		fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
		handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
		obj := ports.HashBytes([]byte("partition"))
		var cum uint64
		for _, d := range deltas {
			cum += d
			if _, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, cum); err != nil {
				t.Fatal(err)
			}
		}
		return ledger.EscrowBalance(obj)
	}
	eights := make([]uint64, 5)
	for i := range eights {
		eights[i] = 8
	}
	ones := make([]uint64, 40)
	for i := range ones {
		ones[i] = 1
	}
	if a, b := run(eights), run(ones); a != b || a != 5 {
		t.Fatalf("escrow after 40 increments: in eights %d, in ones %d — want both ⌊40/8⌋ = 5 (a payer-chosen partition must not move the skim)", a, b)
	}
}

// TestSessionReaperKeysOnIdleNotOnAdmit — G-λ-8-5 (C9, cert §6.2). A session settling
// every tick survives well past the idle window measured from its open; a silent one
// is reaped at the window, its remainder accounted ONCE (B-13: forfeits ≤ its unsettled
// budget), by the node's own clock.
func TestSessionReaperKeysOnIdleNotOnAdmit(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	live, _ := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	silentFetcher := identity.FromSeed(7103)
	// A second, SILENT session from another fetcher, opened directly.
	tok := mintDemandTokensFor(t, server, 0, 1)
	silent, err := server.OpenDeliverySession(silentFetcher.NodeID(), demand.SignSessionOpen(silentFetcher.Signer(), server.id, tok))
	if err != nil {
		t.Fatal(err)
	}
	obj := ports.HashBytes([]byte("g8-5"))
	_, m := server.DeliverySessionForTest(live)
	_ = m
	liveSess, _ := server.DeliverySessionForTest(live)
	commitment := liveSess.commitment
	var count uint64
	// Settle every idle/4 for 4 windows: the live session must survive 4 × idle.
	for i := 0; i < 16; i++ {
		sched.RunUntil(sched.Now().Add(r29Idle / 4))
		count++
		if _, err := settleOverWire(t, fetcher, server, sched, live, commitment, obj, count); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	if _, ok := server.DeliverySessionForTest(live); !ok {
		t.Fatal("a session settling every tick was reaped — the reaper keys on the admit stamp, not on idleness")
	}
	if _, ok := server.DeliverySessionForTest(silent.handle); ok {
		t.Fatal("a session silent for 4 idle windows was not reaped")
	}
	st := ledger.DeliverySettlementStats()
	if st.SessionsClosed != 1 || st.PendingRefundCredits != 50_000 || st.BurnedCredits != 0 {
		t.Fatalf("the silent session's remainder: closed %d, pending %d, burned %d — want 1, exactly its unsettled face booked as a deposit (B-13: the fetcher forfeits nothing but latency), 0 burned", st.SessionsClosed, st.PendingRefundCredits, st.BurnedCredits)
	}
	// The stamp is COARSE (cert §7): never finer than idle/4.
	if g := ports.Time(r29Idle / deliveryStampDivisor); liveSess.lastSettle%g != 0 {
		t.Fatalf("idle stamp %d is finer than the reaper's granularity %d", liveSess.lastSettle, g)
	}
}

// TestSessionCarriesNoPerObjectMap — G-λ-8-7 (cert §7, the Don't #3 refutation). The
// session struct exposes no object-keyed collection, and a settlement for object O
// leaves no O-keyed residue on the node once the lane is superseded.
func TestSessionCarriesNoPerObjectMap(t *testing.T) {
	typ := reflect.TypeOf(DeliverySession{})
	hashT := reflect.TypeOf(ports.Hash{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Map:
			if f.Type.Key() == hashT {
				t.Fatalf("DeliverySession.%s is keyed by object — a per-session per-object map is INSIDE Don't #3 (cert §7)", f.Name)
			}
			t.Fatalf("DeliverySession.%s is a %s — the session holds no collection", f.Name, f.Type)
		case reflect.Slice:
			if f.Type.Elem().Kind() != reflect.Uint8 {
				t.Fatalf("DeliverySession.%s is a %s — the session holds scalars, fixed arrays and byte strings only", f.Name, f.Type)
			}
		}
	}
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	obj := ports.HashBytes([]byte("g8-7"))
	server.ledger.RecordServeToObject(server.id, fetcher.id, obj, ports.ChunkID{1}, 4<<20)
	if _, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, 16); err != nil { // 16 × 256 KiB = the 4 MiB served: full ack
		t.Fatal(err)
	}
	if _, live := ledger.ProvisionalLaneForTest(server.id, fetcher.id, obj); live {
		t.Fatal("a fully acknowledged lane survived — the object-level record must die with the settlement that needed it")
	}
}

// failingAppendStore is a durable guard store whose Append fails after n successes.
type failingAppendStore struct {
	ok  int
	got []ports.PaidSerial
}

func (f *failingAppendStore) Load() ([]ports.PaidSerial, error) { return nil, nil }
func (f *failingAppendStore) Append(p ports.PaidSerial) error {
	if len(f.got) >= f.ok {
		return errors.New("disk full")
	}
	f.got = append(f.got, p)
	return nil
}
func (f *failingAppendStore) Compact([]ports.PaidSerial) error { return nil }

// TestOpenIsAllOrNothingAndDurableBeforeAdmission — G-λ-8-9 (C2; T-3/T-10 per open and
// per top-up). A refused open records nothing and admits nothing; a store that cannot
// append refuses the open with no session; a re-presented anchor is refused at open;
// one live session per fetcher (C1); the cap refuses and never evicts (C10).
func TestOpenIsAllOrNothingAndDurableBeforeAdmission(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	store := &failingAppendStore{ok: 0}
	ledger.SetPaidSerialStore(store)
	if err := ledger.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	tok := mintDemandTokensFor(t, server, 0, 1)
	var oerr error
	fetcher.OpenDeliverySessionRemote(server.id, tok, func(_ uint64, _ []byte, e error) { oerr = e })
	sched.Run()
	if oerr == nil || server.LiveDeliverySessions() != 0 {
		t.Fatalf("an open whose durable append failed was admitted (err %v, live %d)", oerr, server.LiveDeliverySessions())
	}
	// Now the store works: the SAME anchor opens (nothing was recorded by the refusal).
	store.ok = 8
	handle, _ := openOverWire(t, fetcher, server, sched, tok)
	if len(store.got) != 1 {
		t.Fatalf("durable store holds %d entries after one admitted open, want 1", len(store.got))
	}
	// Re-presenting the spent anchor — as a top-up — is refused and records nothing.
	var ferr error
	fetcher.FundDeliverySessionRemote(server.id, handle, tok, func(_ int64, e error) { ferr = e })
	sched.Run()
	if ferr == nil || len(store.got) != 1 {
		t.Fatalf("a re-presented anchor funded the session (err %v, store %d)", ferr, len(store.got))
	}
	// A fresh anchor funds it: budget rises by exactly one face.
	var budget int64
	fetcher.FundDeliverySessionRemote(server.id, handle, mintDemandTokensFor(t, server, 0, 1), func(b int64, e error) { budget, ferr = b, e })
	sched.Run()
	if ferr != nil || budget != 100_000 {
		t.Fatalf("top-up: budget %d err %v, want 100,000", budget, ferr)
	}
	// One live session per fetcher: a second open from the same fetcher is refused.
	fetcher.OpenDeliverySessionRemote(server.id, mintDemandTokensFor(t, server, 0, 1), func(_ uint64, _ []byte, e error) { oerr = e })
	sched.Run()
	if oerr == nil || server.LiveDeliverySessions() != 1 {
		t.Fatalf("a second open from the same fetcher was admitted (err %v)", oerr)
	}
}

// TestDeliveryAnchorIsNotARelayAnchor — B-7 (the T-6 twin). A relay-domain credential
// under the SAME key and epoch fails the delivery open with the domain refusal and moves
// nothing; the demand-domain token is what opens.
func TestDeliveryAnchorIsNotARelayAnchor(t *testing.T) {
	_, server, ledger, _ := deliveryPairForTest(t, nil)
	fID := identity.FromSeed(7104)
	relayCred := mintAnchorsFor(t, server, 0, 1)[0] // relay domain, same key_0
	asToken := []demand.Token{{Serial: relayCred.Serial, Sig: relayCred.Sig}}
	before := len(ledger.LivePaidSerialsRaw())
	_, err := server.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), server.id, asToken))
	if !errors.Is(err, errDeliveryAnchorInvalid) {
		t.Fatalf("a relay-domain credential opened a delivery session or failed elsewhere: %v", err)
	}
	if len(ledger.LivePaidSerialsRaw()) != before || server.LiveDeliverySessions() != 0 {
		t.Fatal("the refused open moved the guard or admitted a session")
	}
	if _, err := server.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), server.id, mintDemandTokensFor(t, server, 0, 1))); err != nil {
		t.Fatalf("the demand-domain token did not open: %v", err)
	}
}

// TestReceiptBindsSessionAnchorsAndCount — B-8 (receipt v3, cert §6.3). Tampering the
// count, the commitment, the object, the server, the fetcher, or the handle breaks the
// binding; a receipt for a session opened by another identity is refused; a receipt
// naming no live session is refused (B-9's anchored half).
func TestReceiptBindsSessionAnchorsAndCount(t *testing.T) {
	fetcher, server, _, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	fID := identity.FromSeed(7101) // the fixture fetcher's identity
	obj := ports.HashBytes([]byte("b-8"))
	good := demand.AckSession(fID.Signer(), handle, m, obj, server.id, 3)
	if s, err := server.SettleDeliveryReceipt(fetcher.id, good); err != nil || s != 3 {
		t.Fatalf("the honest receipt: (%d, %v)", s, err)
	}
	tamper := func(name string, mut func(r *demand.SessionReceipt), from ports.NodeID) {
		r := good
		r.Count = 5 // would advance if accepted
		r = demand.AckSession(fID.Signer(), r.Handle, r.Commitment, r.Object, r.Server, r.Count)
		mut(&r)
		if s, err := server.SettleDeliveryReceipt(from, r); err == nil {
			t.Fatalf("%s: a tampered receipt settled %d", name, s)
		}
	}
	other := ports.HashBytes([]byte("other"))
	tamper("count", func(r *demand.SessionReceipt) { r.Count = 7 }, fetcher.id)
	tamper("commitment", func(r *demand.SessionReceipt) { r.Commitment[0] ^= 1 }, fetcher.id)
	tamper("object", func(r *demand.SessionReceipt) { r.Object = other }, fetcher.id)
	tamper("server", func(r *demand.SessionReceipt) { r.Server = other }, fetcher.id)
	tamper("fetcher key", func(r *demand.SessionReceipt) { r.Fetcher[0] ^= 1 }, fetcher.id)
	tamper("handle", func(r *demand.SessionReceipt) { r.Handle = 99 }, fetcher.id)
	tamper("another identity presents it", func(*demand.SessionReceipt) {}, identity.FromSeed(7105).NodeID())
	// A receipt for a session that does not exist (never opened): refused, not banked.
	stray := demand.AckSession(fID.Signer(), 4242, m, obj, server.id, 1)
	if _, err := server.SettleDeliveryReceipt(fetcher.id, stray); !errors.Is(err, errDeliveryNoSession) {
		t.Fatalf("a receipt naming no live session: %v", err)
	}
	if server.WitnessedIncrements(obj) != 3 {
		t.Fatalf("witnessed increments %d, want the 3 the honest receipt settled (the tampered ones must move nothing)", server.WitnessedIncrements(obj))
	}
}

// ---- the witnessed-demand observable under sessions (G-DEM-2, G-DEM-3, G-DEM-4; the
// certification R2.9-witnessed-demand-observable-under-sessions-2026-09-06 §7).

// shortSettleLedger settles FEWER increments than asked — the seam under test in
// G-DEM-2 without the node's own ceiling check being the thing measured.
type shortSettleLedger struct {
	*credit.Ledger
	cap int64
}

func (l *shortSettleLedger) SettleDelivery(server, fetcher ports.NodeID, root ports.Hash, count, budget, prior int64) (settled, paid int64, reason string) {
	if count > l.cap {
		count = l.cap
	}
	return l.Ledger.SettleDelivery(server, fetcher, root, count, budget, prior)
}

// TestWitnessedDemandReadsTheLedgersSettledNotTheReceiptCount — G-DEM-2. Ablation: bump
// by the delta (or r.Count − s.count) instead of the ledger's settled.
func TestWitnessedDemandReadsTheLedgersSettledNotTheReceiptCount(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	server.SetLedger(&shortSettleLedger{Ledger: ledger, cap: 4})
	obj := ports.HashBytes([]byte("g-dem-2"))
	if settled, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, 10); err != nil || settled != 4 {
		t.Fatalf("settle: (%d, %v), want the ledger's 4", settled, err)
	}
	if server.WitnessedIncrements(obj) != 4 {
		t.Fatalf("witnessed %d, want the LEDGER's settled 4, not the receipt's delta 10", server.WitnessedIncrements(obj))
	}
}

// TestWitnessedDemandNeverExceedsCreditsSettled — G-DEM-3 (P-SESSION as a property over a
// randomized sequence, refusals included). Ablation: move the Witness call above the
// ReasonPaid check so a refused settlement counts.
func TestWitnessedDemandNeverExceedsCreditsSettled(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	objs := []ports.Hash{ports.HashBytes([]byte("a")), ports.HashBytes([]byte("b")), ports.HashBytes([]byte("c"))}
	var count uint64
	seed := uint64(0x9e3779b97f4a7c15)
	for i := 0; i < 200; i++ {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		obj := objs[seed%3]
		var next uint64
		switch (seed >> 8) % 4 {
		case 0:
			next = count + 1 + (seed>>16)%7 // advance
		case 1:
			next = count // replay
		case 2:
			if count > 0 {
				next = count - 1 // lower
			}
		case 3:
			next = 60_000 // above the budget: refused
		}
		s, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, next)
		if err == nil && s > 0 && next > count {
			count = next
		}
		if count >= 50_000 {
			break
		}
	}
	var sum int64
	for _, o := range objs {
		sum += server.WitnessedIncrements(o)
	}
	st := ledger.DeliverySettlementStats()
	if sum*credit.DeliveryIncrementCredit > st.SettledCredits || st.SettledCredits > 50_000 {
		t.Fatalf("Σ demand·p = %d > Σ settled %d (P-SESSION broken), or settled above one face", sum, st.SettledCredits)
	}
	if sum != st.SettledCredits/credit.DeliveryIncrementCredit {
		t.Fatalf("Σ demand %d != settled increments %d — refused or replayed settlements counted", sum, st.SettledCredits)
	}
}

// TestSettlementIsNotGatedOnTheDemandObservable — G-DEM-4. P3b ON, the same bonded
// fetcher settles twice on one object: the second settlement still PAYS. Ablation: carry
// the v2 `if credited { … }` shape onto the v3 path.
func TestSettlementIsNotGatedOnTheDemandObservable(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	fPub := []byte(identity.FromSeed(7101).Signer().Public().(ed25519.PublicKey))
	server.demandBank.RequireBondedFetcher(func(pub []byte) (string, bool) {
		if string(pub) == string(fPub) {
			return "slot-f", true
		}
		return "", false
	})
	handle, m := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	obj := ports.HashBytes([]byte("g-dem-4"))
	before := ledger.Balance(server.id)
	for i, c := range []uint64{8, 16} {
		if s, err := settleOverWire(t, fetcher, server, sched, handle, m, obj, c); err != nil || s != 8 {
			t.Fatalf("settlement %d: (%d, %v) — the settlement was gated on the demand observable", i, s, err)
		}
	}
	if ledger.Balance(server.id) != before+16-2 {
		t.Fatalf("server balance rose by %d, want 14 (16 increments − skim)", ledger.Balance(server.id)-before)
	}
	if server.demandBank.DistinctBondedFetchers(obj) != 1 || server.WitnessedIncrements(obj) != 16 {
		t.Fatalf("distinct %d / increments %d, want 1 / 16", server.demandBank.DistinctBondedFetchers(obj), server.WitnessedIncrements(obj))
	}
}

// ---- the deposit's release epoch is the anchor's REAL epoch (blind PE item 1, 2026-09-07):
// the whole M2 / T-DEPOSIT argument rests on the runtime value the node hands the ledger
// at close. Ablation: `maxAnchorEpoch: maxEpoch(spend)` → 0 at open, or deleting the
// fund-time raise ⇒ RED.

// epochRecordingLedger records the maxAnchorEpoch the node hands CloseDeliverySession.
type epochRecordingLedger struct {
	*credit.Ledger
	closes []uint64
}

func (l *epochRecordingLedger) CloseDeliverySession(f ports.NodeID, remaining int64, maxAnchorEpoch uint64) int64 {
	l.closes = append(l.closes, maxAnchorEpoch)
	return l.Ledger.CloseDeliverySession(f, remaining, maxAnchorEpoch)
}

// liveEpochServer builds a server whose chain is at epoch `epoch` (c3 fixture: real
// EpochBlocks, blocks minted by another validator) with committed demand keys for epoch 0
// and for `epoch`, so an anchor minted under key_epoch verifies at that epoch.
func liveEpochServer(t *testing.T, epoch int) (*Node, *rsa.PrivateKey, *credit.Ledger, *settableEpoch) {
	t.Helper()
	nd, signer := c3Node(t, 7301)
	key0, keyE := cachedRSAKey(t, 3), cachedRSAKey(t, 4)
	c := c3Chain(t, 1, signer,
		chain.SignIssuerKeyReg(signer, 0, demand.KeyFingerprint(&key0.PublicKey)),
		chain.SignIssuerKeyReg(signer, uint64(epoch), demand.KeyFingerprint(&keyE.PublicKey)))
	ledger := credit.New(50_000, 0)
	src := &settableEpoch{}
	ledger.SetEpochSource(src)
	nd.SetLedger(ledger)
	nd.EnableChain(c, signer)
	nd.SetDemandIssuerKey(rand.Reader, 0, key0)
	c3Advance(t, c, []ed25519.PrivateKey{signer}, epoch)
	nd.SetDemandIssuerKey(rand.Reader, uint64(epoch), keyE)
	if got := nd.chainEpoch(); got != uint64(epoch) {
		t.Fatalf("fixture: chain epoch %d, want %d", got, epoch)
	}
	if ks := nd.DemandIssuerKeyset(nd.id); ks == nil || ks.Key(uint64(epoch)) == nil {
		t.Fatalf("fixture: no committed key_%d — the genesis registration for a later epoch was not accepted", epoch)
	}
	src.e = uint64(epoch)
	nd.EnableDemandBank(nd.id)
	nd.EnableDeliverySessions(r29Idle)
	return nd, keyE, ledger, src
}

func TestDepositReleaseEpochIsTheAnchorsRealEpoch(t *testing.T) {
	const E = 3
	nd, keyE, ledger, src := liveEpochServer(t, E)
	rec := &epochRecordingLedger{Ledger: ledger}
	nd.SetLedger(rec)
	fID := identity.FromSeed(7302)
	ledger.Register(fID.NodeID())
	ledger.Register(nd.id)
	// OPEN with an anchor issued at epoch 0 (still inside the window at epoch E = 3), then
	// FUND with an anchor issued at epoch E: the session's release epoch must follow the
	// NEWEST anchor (the raise at fund), so the value handed at close is E, not 0.
	key0 := cachedRSAKey(t, 3)
	s, err := nd.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, key0, 0)}))
	if err != nil {
		t.Fatalf("open at epoch %d with an epoch-0 anchor: %v", E, err)
	}
	if s.maxAnchorEpoch != 0 {
		t.Fatalf("session maxAnchorEpoch %d after an epoch-0 open, want 0", s.maxAnchorEpoch)
	}
	if _, err := nd.FundDeliverySession(fID.NodeID(), demand.SignSessionFund(fID.Signer(), nd.id, s.handle, []demand.Token{mintDemandTokenUnder(t, keyE, E)})); err != nil {
		t.Fatalf("fund: %v", err)
	}
	if s.maxAnchorEpoch != E {
		t.Fatalf("session maxAnchorEpoch %d after a fund at epoch %d, want the raise to %d", s.maxAnchorEpoch, E, E)
	}
	nd.closeDeliverySession(s.handle, "test")
	// A second session OPENED with an epoch-E anchor and never funded: the open path alone
	// must hand E (a constant 0 at open is indistinguishable on the epoch-0 arm above).
	gID := identity.FromSeed(7303)
	ledger.Register(gID.NodeID())
	s2, err := nd.OpenDeliverySession(gID.NodeID(), demand.SignSessionOpen(gID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, keyE, E)}))
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	nd.closeDeliverySession(s2.handle, "test")
	if len(rec.closes) != 2 || rec.closes[0] != E || rec.closes[1] != E {
		t.Fatalf("the node handed the ledger maxAnchorEpoch %v at the two closes, want [%d %d] — a constant here re-creates release-at-close on any chain past epoch W+1", rec.closes, E, E)
	}
	// The observable: the deposit (two whole faces) is locked until E + W + 1 on the
	// ledger's own clock, then returns whole.
	for e := uint64(E); e <= E+uint64(credit.PaidSerialWindow); e++ {
		src.e = e
		ledger.ReleaseDueRefunds()
		if ledger.Balance(fID.NodeID()) != 0 {
			t.Fatalf("epoch %d: the deposit released before the anchor left the window (balance %d)", e, ledger.Balance(fID.NodeID()))
		}
	}
	src.e = E + uint64(credit.PaidSerialWindow) + 1
	ledger.ReleaseDueRefunds()
	if ledger.Balance(fID.NodeID()) != 100_000 {
		t.Fatalf("at E+W+1 the fetcher holds %d, want both faces (100,000) back", ledger.Balance(fID.NodeID()))
	}
}

// TestSilentServerSweepReleasesDueDeposits — the refundReleaser wire in SweepDeliverySessions
// (blind PE ablation 6): on a server with NO other guarded ledger activity, the periodic sweep
// alone returns a due deposit. Ablation: delete the ReleaseDueRefunds call from the sweep.
func TestSilentServerSweepReleasesDueDeposits(t *testing.T) {
	fetcher, server, ledger, sched := deliveryPairForTest(t, nil)
	src := &settableEpoch{}
	ledger.SetEpochSource(src)
	handle, _ := openOverWire(t, fetcher, server, sched, mintDemandTokensFor(t, server, 0, 1))
	server.closeDeliverySession(handle, "test") // a deposit of one face, anchor epoch 0
	before := ledger.Balance(fetcher.id)
	src.e = uint64(credit.PaidSerialWindow) + 1 // the anchor has left the window; nothing else touches the ledger
	server.SweepDeliverySessions()
	if ledger.Balance(fetcher.id) != before+50_000 {
		t.Fatalf("a silent server's sweep did not release the due deposit (balance %d → %d)", before, ledger.Balance(fetcher.id))
	}
}

// settableEpoch is a ledger EpochSource a test steps by hand (the ledger's own clock; the
// node's chain epoch is a separate clock in these fixtures — R2.10 / F8 wires them to one
// source in production).
type settableEpoch struct{ e uint64 }

func (s *settableEpoch) Epoch() uint64 { return s.e }
