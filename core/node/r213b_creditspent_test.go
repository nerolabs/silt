package node

// R2.13b — creditSpent durability (F-4). RED-first gates G-CS-1 … G-CS-4.
//
// Binding spec: PE ruling
// silt-reviews/principle-engineer/RULING-F4-creditSpent-durability-and-F3-fee-constancy-2026-09-04.md
// §2.1 (the reproduction), §3 (the fix shape). Deliberation:
// docs/thinking/2026-09-04-r2.13b-creditspent-durability-design.md §4.
//
// THE MECHANISM. creditSpent (node.go) is process memory. A publish credit's validity
// is the persisted publish key's lifetime (no epoch in silt/blindcredit/fdh/v1), so an
// issuer restart forgets every spent credit and honours each one again — a second
// demand token with a distinct serial per credit per restart. Measured by the PE:
// one 50,000 credit → two tokens → payouts 43,750 + 43,750 against one burn.
//
// THE FIX SHAPE these gates pin: a SECOND guardstore.Disk at creditspent.log behind
// the unchanged ports.PaidSerialStore (Serial = credit serial, Epoch = 0), attached
// and loaded the way the paid-serial guard is (SetPaidSerialStore / LoadPaidSerials),
// Append BEFORE the in-memory mark, refuse on a store error, cap with refuse-not-evict.
// NOT a namespace in paidserials.log: the ledger's sweep compacts that file with its
// own live set and would evict every credit record (G-CS-4 pins the two files apart).
//
// Fixture: the PE's overlay probe, verbatim in shape — boot the issuer the way the
// daemon boots across a restart (same identity seed, same persisted publish key,
// same committed demand key for epoch 0, a FRESH per-process ledger).

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

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

// csBootIssuer builds a node the way the daemon does across a restart: SAME identity
// (seed), SAME persisted publish issuer key (diskissuer), SAME committed demand key
// for epoch 0, a FRESH per-process ledger. If store is non-nil it is attached and
// loaded BEFORE the node serves anything — the daemon's own attach-then-load order
// for the paid-serial guard (cmd/silt/daemon.go, SetPaidSerialStore/LoadPaidSerials).
func csBootIssuer(t *testing.T, seed int64, publishKey, committed *rsa.PrivateKey, store ports.PaidSerialStore) (*Node, *credit.Ledger) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(seed)
	id := ident.NodeID()
	nd := New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())
	nd.SetSigner(ident.Signer())
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("r213b-genesis-entry"))}},
		IssuerKeys: []chain.IssuerKeyReg{
			chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&committed.PublicKey)),
		},
	}
	chain.Sign(&g, ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	nd.EnableChain(c, ident.Signer())
	nd.EnableTokenIssuer(rand.Reader, publishKey)
	nd.SetDemandIssuerKey(rand.Reader, 0, committed)
	if store != nil {
		nd.SetCreditSpentStore(store)
		if err := nd.LoadCreditSpent(); err != nil {
			t.Fatalf("LoadCreditSpent at boot: %v", err)
		}
	}
	l := credit.New(50_000, 500_000)
	nd.SetLedger(l)
	return nd, l
}

// csMintCredit: a DURABLE requester buys ONE publish credit through the shipped
// handler (a fee-charged token request blinded in the CREDIT domain). Returns the
// credit and the fee burned on this ledger.
func csMintCredit(t *testing.T, iss *Node, ledger *credit.Ledger, publishPub *rsa.PublicKey, durable ports.NodeID) (ports.PublishCredit, int64) {
	t.Helper()
	before := ledger.Balance(durable)
	cserial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := blindtoken.BlindCredit(rand.Reader, publishPub, cserial)
	if err != nil {
		t.Fatal(err)
	}
	rep := iss.answerTokenRequest(durable, ports.Message{Kind: ports.MsgTokenRequest, Data: blinded})
	if !rep.OK {
		t.Fatal("credit mint refused")
	}
	sig, err := blindtoken.UnblindCredit(publishPub, cserial, rep.Data, secret)
	if err != nil {
		t.Fatal(err)
	}
	return ports.PublishCredit{Serial: cserial, Sig: sig}, before - ledger.Balance(durable)
}

// csWithdraw spends cr for a demand token under the committed epoch-0 key through the
// shipped wire handler. ok=false is a refusal (OK=false, no token).
func csWithdraw(t *testing.T, iss *Node, from ports.NodeID, committedPub *rsa.PublicKey, cr *ports.PublishCredit) (demand.Token, bool) {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, committedPub, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	reply := iss.answerDemandTokenRequest(from, ports.Message{Kind: ports.MsgDemandTokenRequest, Data: blinded, Height: 0, Credit: cr})
	if !reply.OK {
		return demand.Token{}, false
	}
	tok, err := demand.Unblind(committedPub, 0, serial, reply.Data, secret)
	if err != nil {
		t.Fatalf("unblind: %v", err)
	}
	return tok, true
}

func csKeys(t *testing.T) (publishKey, committed *rsa.PrivateKey) {
	t.Helper()
	var err error
	if publishKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if committed, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	return
}

func csStoreHas(t *testing.T, s ports.PaidSerialStore, serial []byte) (int, bool) {
	t.Helper()
	got, err := s.Load()
	if err != nil {
		t.Fatalf("store Load: %v", err)
	}
	for _, e := range got {
		if string(e.Serial) == string(serial) {
			return len(got), true
		}
	}
	return len(got), false
}

const (
	csIssuerSeed  = 9001
	csDurableSeed = 9002
	csEphSeed     = 9003
)

// ---------------------------------------------------------------------------
// G-CS-1 — the PE's reproduction as a gate. One credit, one token, ACROSS a restart.
// ---------------------------------------------------------------------------
func TestCreditSpentSurvivesIssuerRestart(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := guardstore.NewMem() // shared across both boots: the "disk"
	durable := identity.FromSeed(csDurableSeed).NodeID()
	eph := identity.FromSeed(csEphSeed).NodeID()

	iss, ledger := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	cr, burned := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	if burned != 50_000 {
		t.Fatalf("credit mint: durable identity burned %d, want exactly one fee 50000", burned)
	}
	tok1, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr)
	if !ok {
		t.Fatal("first credit-paid withdrawal refused")
	}
	if got := ledger.Balance(eph); got != 500_000 {
		t.Fatalf("ephemeral was charged on the pre-restart ledger: balance %d, want the 500000 grant untouched", got)
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("in-process double-spend was ACCEPTED (the in-memory guard itself is broken)")
	}
	if n, has := csStoreHas(t, store, cr.Serial); !has {
		t.Errorf("G-CS-1: after the credit was spent the credit-spent store holds %d entries and NOT the spent serial — "+
			"the spend was never appended (fix: Append before the in-memory mark in tokenChargeFor's closure)", n)
	}

	// RESTART: same identity, same publish key, same committed demand key, same store.
	iss2, ledger2 := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	if !iss2.creditSpent[string(cr.Serial)] {
		t.Errorf("G-CS-1: after LoadCreditSpent the restarted issuer's creditSpent does not hold the spent serial "+
			"(store has %d entries)", func() int { n, _ := csStoreHas(t, store, cr.Serial); return n }())
	}
	tok2, ok := csWithdraw(t, iss2, eph, &committed.PublicKey, &cr)
	if ok {
		t.Fatalf("G-CS-1 RED (PE F-4 §2.1): the restarted issuer honoured the SAME credit again — a second demand "+
			"token with a distinct serial (tok1=%x… tok2=%x…). One 50000 burn now funds two conserved payouts "+
			"(43750 + 43750). creditSpent must be restored from the credit-spent store at boot",
			tok1.Serial[:4], tok2.Serial[:4])
	}

	// Only one token exists; the ledgers burned once and never charged the ephemeral.
	if got := ledger.Balance(durable); got != 500_000-50_000 {
		t.Errorf("pre-restart ledger: durable balance %d, want 450000 (exactly one fee burned)", got)
	}
	if got := ledger2.Balance(eph); got != 500_000 && got != 0 {
		t.Errorf("post-restart ledger: ephemeral balance %d — a refused withdrawal must not charge anyone", got)
	}

	// Anti-over-correction: a FRESH credit still buys a token on the restarted issuer,
	// and its spend lands in the store beside the first.
	cr2, _ := csMintCredit(t, iss2, ledger2, &publishKey.PublicKey, durable)
	if _, ok := csWithdraw(t, iss2, eph, &committed.PublicKey, &cr2); !ok {
		t.Fatal("over-correction: the restarted issuer refused a fresh, unspent credit")
	}
	if n, has := csStoreHas(t, store, cr2.Serial); !has || n != 2 {
		t.Errorf("after two spends the store holds %d entries (has second serial: %v), want 2", n, has)
	}
}

// csFlakyStore is a Mem store whose Append can be made to fail on demand.
type csFlakyStore struct {
	*guardstore.Mem
	fail error
}

func (s *csFlakyStore) Append(p ports.PaidSerial) error {
	if s.fail != nil {
		return s.fail
	}
	return s.Mem.Append(p)
}

// ---------------------------------------------------------------------------
// G-CS-2 — a store whose Append fails refuses the withdrawal with the named reason and
// does NOT mark the credit spent in memory, so a later successful append spends it once.
// ---------------------------------------------------------------------------
func TestCreditSpentStoreFailureRefusesTheWithdrawal(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := &csFlakyStore{Mem: guardstore.NewMem()}
	durable := identity.FromSeed(csDurableSeed).NodeID()
	eph := identity.FromSeed(csEphSeed).NodeID()

	iss, ledger := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	cr, _ := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	key := string(cr.Serial)

	injected := errors.New("injected: credit-spent store append failed")
	store.fail = injected

	// The named reason, at the seam the handlers share: the settlement closure.
	charge, err := iss.tokenChargeFor(eph, &cr)
	if err != nil {
		t.Fatalf("a valid unspent credit was refused before settlement: %v", err)
	}
	cerr := charge()
	if cerr == nil {
		t.Fatalf("G-CS-2 RED: the settlement closure returned nil while the credit-spent store's Append fails — " +
			"a token would be signed for a spend no restart can see (the F-4 hole, re-opened per write failure)")
	}
	if !errors.Is(cerr, errCreditStore) && !errors.Is(cerr, injected) {
		t.Fatalf("G-CS-2: the refusal is not the named reason: got %v, want errors.Is errCreditStore (or the injected error wrapped)", cerr)
	}
	if iss.creditSpent[key] {
		t.Fatalf("G-CS-2: the credit was marked spent in memory although the durable append failed — " +
			"the requester can never retry, and the fee is lost (must append BEFORE marking)")
	}

	// Through the shipped wire handler: no token while the store fails.
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("G-CS-2 RED: the wire handler issued a demand token although the credit-spent store's Append fails")
	}
	if n, _ := csStoreHas(t, store, cr.Serial); n != 0 {
		t.Fatalf("store holds %d entries after a failed append, want 0", n)
	}

	// The store heals: the same credit spends exactly once.
	store.fail = nil
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); !ok {
		t.Fatal("after the store healed the still-unspent credit was refused")
	}
	if n, has := csStoreHas(t, store, cr.Serial); !has || n != 1 {
		t.Fatalf("after the healed spend the store holds %d entries (has serial: %v), want exactly 1", n, has)
	}
	if !iss.creditSpent[key] {
		t.Fatal("the healed spend did not mark the credit spent in memory")
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("the healed credit was spent TWICE")
	}
}

// csPad appends n synthetic credit-spent records (distinct 32-byte serials, epoch 0)
// straight into the store, bypassing RSA — the shape a long-lived issuer's file has.
func csPad(t *testing.T, s ports.PaidSerialStore, n int) [][]byte {
	t.Helper()
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		serial := make([]byte, 32)
		copy(serial, "r213b-pad")
		binary.BigEndian.PutUint64(serial[24:], uint64(i))
		if err := s.Append(ports.PaidSerial{Serial: serial, Epoch: 0}); err != nil {
			t.Fatalf("pad append %d: %v", i, err)
		}
		out = append(out, serial)
	}
	return out
}

// ---------------------------------------------------------------------------
// G-CS-3 — at the cap a NEW credit is refused; no recorded credit is evicted; after a
// restart every recorded credit is still refused.
//
// The store is pre-filled to cap-1 (a long-lived issuer's file), so ONE real spend
// lands exactly at the cap and the NEXT credit is the cap refusal — the gate's own
// axis, not G-CS-1's append.
// ---------------------------------------------------------------------------
func TestCreditSpentCapRefusesNeverEvicts(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := guardstore.NewMem()
	durable := identity.FromSeed(csDurableSeed).NodeID()
	eph := identity.FromSeed(csEphSeed).NodeID()
	pad := csPad(t, store, maxCreditSpent-1)

	iss, ledger := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	if got := len(iss.creditSpent); got != maxCreditSpent-1 {
		t.Errorf("G-CS-3: after load creditSpent holds %d, want %d (the store's contents)", got, maxCreditSpent-1)
	}
	crA, _ := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	crB, _ := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &crA); !ok {
		t.Fatal("the spend that fills the last slot was refused (cap is off by one, or the guard refuses below the cap)")
	}
	if n, has := csStoreHas(t, store, crA.Serial); !has || n != maxCreditSpent {
		t.Errorf("G-CS-3: after the last-slot spend the store holds %d records (crA present: %v), want exactly the cap %d", n, has, maxCreditSpent)
	}

	// A NEW, valid, unspent credit AT the cap: REFUSED with the named reason.
	charge, err := iss.tokenChargeFor(eph, &crB)
	if err == nil {
		err = charge()
	}
	if err == nil {
		t.Fatalf("G-CS-3 RED: at the cap (%d recorded credits) a NEW credit was honoured — the guard has no cap, "+
			"or it evicted a record to make room (the eviction the design refuses)", maxCreditSpent)
	}
	if !errors.Is(err, errCreditGuardFull) {
		t.Errorf("G-CS-3: the cap refusal is not the named reason: got %v, want errors.Is errCreditGuardFull", err)
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &crB); ok {
		t.Fatal("G-CS-3 RED: the wire handler issued a token for a new credit at the cap")
	}
	if iss.creditSpent[string(crB.Serial)] {
		t.Error("G-CS-3: the refused credit was marked spent")
	}

	// No eviction: the store still holds every record; crA and the samples are still held.
	if n, has := csStoreHas(t, store, crA.Serial); n != maxCreditSpent || !has {
		t.Fatalf("G-CS-3: store holds %d records (crA present: %v) after the refusal, want %d and present — a record was EVICTED", n, has, maxCreditSpent)
	}
	for i, s := range []int{0, len(pad) / 2, len(pad) - 1} {
		if !iss.creditSpent[string(pad[s])] {
			t.Errorf("G-CS-3: padded record %d (sample %d) is no longer held in memory — evicted", s, i)
		}
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &crA); ok {
		t.Fatal("G-CS-3 RED: a RECORDED credit was honoured at the cap — the record was evicted")
	}

	// Restart at the cap: loads (only ABOVE the cap is the refuse-to-start); every
	// recorded credit still refused; the new one still refused; nothing evicted.
	iss2, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	if got := len(iss2.creditSpent); got != maxCreditSpent {
		t.Errorf("G-CS-3: after a restart at the cap creditSpent holds %d, want %d", got, maxCreditSpent)
	}
	if _, ok := csWithdraw(t, iss2, eph, &committed.PublicKey, &crA); ok {
		t.Fatal("G-CS-3 RED: after a restart the recorded credit crA was honoured")
	}
	if _, ok := csWithdraw(t, iss2, eph, &committed.PublicKey, &crB); ok {
		t.Fatal("G-CS-3 RED: after a restart the cap no longer refuses the new credit crB")
	}
	if n, _ := csStoreHas(t, store, crA.Serial); n != maxCreditSpent {
		t.Fatalf("G-CS-3: store holds %d after the restart, want %d unchanged", n, maxCreditSpent)
	}
}

// ---------------------------------------------------------------------------
// G-CS-4 — the second file. The node's credit-spent store is a guardstore.Disk at
// creditspent.log, DISTINCT from paidserials.log: a Compact of the paid-serial store
// (what the ledger's expiry sweep does, with its own live set) is inert on the credit
// records, and a real re-open of creditspent.log restores them.
//
// The R2.13 handle clause on this file is the ADAPTER's gate, path-agnostic on the
// same Disk type reused unchanged: adapters/guardstore
// TestG_CO1_PostRenameOpenFailureOrphansTheAppendHandle (plus the R213_ backstop
// tests). This gate pins the WIRING: two files, two stores, one adapter.
// ---------------------------------------------------------------------------
func TestCreditSpentDiskStoreIsASecondFileBesidePaidSerials(t *testing.T) {
	publishKey, committed := csKeys(t)
	dir := t.TempDir()
	csPath := filepath.Join(dir, "creditspent.log")
	psPath := filepath.Join(dir, "paidserials.log")
	durable := identity.FromSeed(csDurableSeed).NodeID()
	eph := identity.FromSeed(csEphSeed).NodeID()

	cs, err := guardstore.Open(csPath)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := guardstore.Open(psPath)
	if err != nil {
		t.Fatal(err)
	}

	iss, ledger := csBootIssuer(t, csIssuerSeed, publishKey, committed, cs)
	ledger.SetPaidSerialStore(ps)
	if err := ledger.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	cr, _ := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); !ok {
		t.Fatal("first spend refused")
	}
	if n, has := csStoreHas(t, cs, cr.Serial); !has || n != 1 {
		t.Errorf("G-CS-4: creditspent.log holds %d records (spent serial present: %v), want exactly 1 — the spend was not written to the second file", n, has)
	}
	if n, has := csStoreHas(t, ps, cr.Serial); has || n != 0 {
		t.Errorf("G-CS-4: paidserials.log holds %d records (credit serial present: %v), want 0 — the credit record went into the PAID-SERIAL file, which the sweep compacts with the ledger's live set", n, has)
	}

	// The ledger's sweep on the OTHER file: inert here.
	_, hadBefore := csStoreHas(t, cs, cr.Serial)
	if err := ps.Compact(nil); err != nil {
		t.Fatal(err)
	}
	if n, has := csStoreHas(t, cs, cr.Serial); hadBefore && !has {
		t.Errorf("G-CS-4: compacting paidserials.log removed the credit record from creditspent.log (%d records left)", n)
	}

	// Restart against the REAL file: close, re-open at the path, load, refuse.
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(csPath); err != nil || fi.Size() == 0 {
		t.Errorf("G-CS-4: creditspent.log is absent or empty on disk after a spend (err=%v)", err)
	}
	cs2, err := guardstore.Open(csPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs2.Close()
	iss2, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, cs2)
	if _, ok := csWithdraw(t, iss2, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("G-CS-4 RED: after a re-open of creditspent.log the restarted issuer honoured the spent credit")
	}
}
