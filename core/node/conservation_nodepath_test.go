package node

// R0.5 node-path integration conservation gate (Tester seat, Boulder-0, 2026-09-01).
//
// Proves the A4 conservation fix is wired on the REAL NODE PATH, not just the
// bare credit.Ledger. Exercises:
//   - node.go:1576: RecordServeToObject called from the MsgFetchChunk handler
//     when n.ledger != nil and n.proofMeta[chunkID].Root != zero hash
//   - demandrole.go:201: RedeemDeliveryCredit called from handleDeliveryReceipt
//     when a valid, bank-accepted delivery receipt arrives
//
// Scenario:
//   1. Serve lane-0 VIA THE NODE HANDLER (proves node.go:1576 fires).
//   2. Flood maxProvisional-1 additional lanes DIRECTLY ON THE LEDGER (setup
//      only — does not re-test the node-handler wiring, keeps conservation simple).
//   3. Submit a delivery receipt VIA THE NODE HANDLER (proves demandrole.go:201 fires).
//   4. Assert conservation end-to-end.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

// TestR05NodePathConservation is the R0.5 gate: the A4 fix must be wired on the
// real node path. It fails if:
//   - node.go:1576 does not call RecordServeToObject (the lane-0 serve assertion)
//   - demandrole.go:201 does not call RedeemDeliveryCredit with conservation
//   - the eviction claw-back (reverseProvisional at eviction) is absent
func TestR05NodePathConservation(t *testing.T) {
	const fee = 50_000
	const bytes0 = 1024 // lane-0 serve size

	// nodFloodSize is the flood size. Must be >= maxProvisional (8192) to trigger
	// eviction of lane 0. We use 8192 directly to avoid coupling to the unexported
	// maxProvisional constant.
	const nodFloodSize = 8192

	// Build an RSA issuer key for the demand bank.
	issuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	issuerPub := &issuerPriv.PublicKey

	// Build the server node. grant=0 so auto-registration gives no credits.
	sched := simclock.New()
	simNet := simnet.New(sched, 2, simnet.DefaultConfig())
	serverIdent := identity.FromSeed(8001)
	serverID := serverIdent.NodeID()
	serverStore := memstore.New()
	nd := New(serverID, DefaultConfig(), sched, simNet.Endpoint(serverID), serverStore)

	ledger := credit.New(fee, 0 /*grant=0: no auto-credits on registration*/)
	nd.SetLedger(ledger)
	nd.SetSigner(serverIdent.Signer())

	// R0.4b: the bank verifies against a per-epoch keyset whose key_E was resolved
	// against the CONSENSUS-ATTESTED binding, so this fixture must commit the binding
	// before the receipt can be banked at all. The node issues to itself (the
	// bilateral shape the daemon runs), so the issuer identity is serverID. Epochs
	// are off, so the consensus epoch is 0 throughout.
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("r05-genesis-entry"))}},
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
		t.Fatal("setup: the committed issuer key was not pinned - the bank would reject every receipt")
	}

	fetcherIdent := identity.FromSeed(8002)
	fetcherID := fetcherIdent.NodeID()

	// Give the fetcher exactly fee credits by making it "earn" via a RecordServe
	// from the server (not RecordServeToObject — that would create a provisional
	// entry we'd need to account for).
	// We use a fake non-object serve (RecordServe) to credit the fetcher.
	// Wait — RecordServe credits the SERVER, not the fetcher. The fetcher earns
	// nothing from being served.
	//
	// Alternative: ChargePublish requires a balance >= fee. We must fund the
	// fetcher. The only way without unexported fields is to make the fetcher serve
	// the server first. But that inverts the roles.
	//
	// Cleanest: use credit.New(fee, fee) so every registered account starts with
	// exactly fee. Then initial = 0 (no accounts registered yet), and the first
	// Register adds fee to the total. We pre-register only the two accounts we
	// need (server and fetcher). All flood accounts also auto-register with fee
	// credits, but we do the flood on the ledger DIRECTLY with distinct flood nodes
	// that we also pre-register to control exactly the initial sum.
	//
	// ACTUAL CLEANEST: do the flood on the ledger but give each flood node a
	// controlled credit amount. Since grant=0, all auto-registered flood nodes
	// get 0 credits. No balances change for flood nodes — only the server's
	// balance and escrow change. So conservation is:
	//   Σ(balances) = server.balance + fetcher.balance + all flood nodes (0 each)
	//   Σ(escrow)   = escrow[objRoot] + Σ escrow[floodRoot[i]]
	//
	// We inject the fetcher's fee by doing a serve FROM some "bank" node TO the
	// fetcher... but again, RecordServe credits the SERVER (the first argument).
	//
	// OK: use credit.New(fee, fee) so every new account starts at fee. The fetcher
	// starts at fee (enough for one ChargePublish). The server starts at fee. All
	// flood nodes start at fee. Track: initial = 2*fee (server + fetcher, pre-registered).
	// Flood creates 8192 new accounts each with fee credits = 8192*fee added to initial.
	// But those flood nodes also earn serve credits — server balance increases per flood.
	// This means initial grows by 8192*fee when the flood nodes auto-register. That's
	// fine if we account for it.
	//
	// SIMPLEST CORRECT APPROACH: use grant=fee. Pre-register server+fetcher only.
	// Do the flood on the ledger directly (not via node handler). Flood nodes are
	// not pre-registered, so they auto-register with fee credits on first acct() call.
	// That means each flood node contributes fee to the sum. We add them to the
	// "all known IDs" list.
	//
	// Let's track this cleanly.

	// Rebuild with grant=fee so every new account starts with one fee's worth.
	ledger = credit.New(fee, fee)
	nd.SetLedger(ledger)
	nd.EnableDemandBank(serverID)

	// Pre-register server and fetcher (grant each fee credits).
	ledger.Register(serverID)
	ledger.Register(fetcherID)

	// Track all NodeIDs and escrow roots to compute the sum later.
	// allIDs grows as flood nodes auto-register (via ledger.acct).
	knownIDs := []ports.NodeID{serverID, fetcherID}
	escrowRoots := []ports.Hash{}

	// sumLedger computes Σbalances + Σescrow over all known accounts and roots.
	// This is conservative: if any new account was auto-registered but not in
	// knownIDs, the sum would be wrong. We therefore add flood IDs to knownIDs.
	sumLedger := func() int64 {
		total := int64(0)
		for _, id := range knownIDs {
			total += ledger.Balance(id)
		}
		for _, root := range escrowRoots {
			total += ledger.EscrowBalance(root)
		}
		return total
	}

	// Pre-register all flood nodes so we can include them in sumLedger.
	// Do this before any serves so their grant is counted in initial.
	floodFetchers := make([]ports.NodeID, nodFloodSize)
	floodRoots := make([]ports.Hash, nodFloodSize)
	for i := 0; i < nodFloodSize; i++ {
		floodFetchers[i] = ports.NodeID(ports.HashBytes([]byte(fmt.Sprintf("r05-flood-req-%d", i))))
		floodRoots[i] = ports.HashBytes([]byte(fmt.Sprintf("r05-flood-obj-%d", i)))
		ledger.Register(floodFetchers[i])
		knownIDs = append(knownIDs, floodFetchers[i])
		escrowRoots = append(escrowRoots, floodRoots[i])
	}

	// Add the lane-0 escrow root to the tracked list.
	objRoot := ports.HashBytes([]byte("r0.5-integration-object-root"))
	escrowRoots = append(escrowRoots, objRoot)

	// Record the initial sum: (2 + nodFloodSize) accounts each with fee credits.
	initial := sumLedger()
	if want := int64(2+nodFloodSize) * int64(fee); initial != want {
		t.Fatalf("setup: initial sum %d, want %d = %d accounts * fee", initial, want, 2+nodFloodSize)
	}

	// ── Step 1: serve lane 0 via the MsgFetchChunk handler (node.go:1576). ──
	// This proves the RecordServeToObject wiring is present.
	chunkData := make([]byte, bytes0)
	for i := range chunkData {
		chunkData[i] = 0xB7
	}
	chunkID := ports.HashBytes(chunkData)
	if err := serverStore.Put(context.Background(), ports.Chunk{ID: chunkID, Data: chunkData}); err != nil {
		t.Fatalf("store lane-0 chunk: %v", err)
	}
	// Set proofMeta so the object-aware branch fires (not the plain RecordServe branch).
	nd.proofMeta[chunkID] = proofMeta{Root: objRoot}

	// Fire the handler: RecordServeToObject(serverID, fetcherID, objRoot, chunkID, bytes0)
	// at node.go:1576.
	nd.handle(fetcherID, ports.Message{Kind: ports.MsgFetchChunk, ChunkID: chunkID, Ephemeral: true})

	// Verify the serve self-mint landed (proves node.go:1576 ran RecordServeToObject).
	skim0 := int64(bytes0) * credit.SkimNum / credit.SkimDen
	net0 := int64(bytes0) - skim0
	wantServerAfterLane0 := int64(fee) + net0 // server started at fee (grant)
	if got := ledger.Balance(serverID); got != wantServerAfterLane0 {
		t.Fatalf("after lane-0 serve via node handler: server balance %d, want %d\n"+
			"RecordServeToObject may not have fired at node.go:1576 (check that proofMeta is set and root is non-zero)",
			got, wantServerAfterLane0)
	}
	if got := ledger.EscrowBalance(objRoot); got != skim0 {
		t.Fatalf("after lane-0 serve: escrow[objRoot]=%d, want skim0=%d", got, skim0)
	}

	// ── Step 2: flood nodFloodSize lanes directly on the ledger. ──
	// This exercises the eviction logic to push lane 0 out of the provisional map.
	// Using the ledger directly (not the node handler) keeps the flood fast and
	// avoids complex simnet reply plumbing for 8192 distinct fetchers. The WIRING
	// under test is the lane-0 serve (step 1) and the redeem (step 3).
	const floodBytes = 8
	floodChunk := ports.HashBytes([]byte("r05-flood-chunk-base"))
	for i := 0; i < nodFloodSize; i++ {
		ledger.RecordServeToObject(serverID, floodFetchers[i], floodRoots[i], floodChunk, floodBytes)
	}

	// ── Step 3: ChargePublish — fetcher pays the withdrawal fee. ──
	if err := ledger.ChargePublish(fetcherID); err != nil {
		t.Fatalf("ChargePublish: %v", err)
	}

	// ── Step 4: issue a valid demand token and submit receipt via the node handler. ──
	// This proves RedeemDeliveryCredit fires at demandrole.go:201.
	serial := make([]byte, 32)
	if _, err := rand.Read(serial); err != nil {
		t.Fatalf("rand.Read serial: %v", err)
	}
	blinded, secret, bErr := demand.Withdraw(rand.Reader, issuerPub, 0, serial)
	if bErr != nil {
		t.Fatalf("demand.Withdraw: %v", bErr)
	}
	blindSig := demand.SignWithdrawal(issuerPriv, blinded)
	token := demand.Unblind(issuerPub, serial, blindSig, secret)

	// Ack signs over (serial, objRoot, serverID) with the fetcher's private key.
	receipt := demand.Ack(fetcherIdent.Signer(), token, objRoot, serverID)
	submitted := demand.SubmittedReceipt{Token: token, Receipt: receipt}
	blob, mErr := submitted.Marshal()
	if mErr != nil {
		t.Fatalf("SubmittedReceipt.Marshal: %v", mErr)
	}

	// Verify the demand bank will accept this receipt before submitting.
	// (If the bank rejects, RedeemDeliveryCredit is never called — a different failure.)
	preRedeemTotal := sumLedger()

	// Submit via the node handler (demandrole.go:175 → RedeemDeliveryCredit at :201).
	nd.handle(fetcherID, ports.Message{Kind: ports.MsgDeliveryReceipt, Data: blob, Ephemeral: true})

	// ── Step 5: conservation assertion. ──
	// Under the A4 fix (eviction reverses the lane-0 self-mint):
	//   initial                          = (2+nodFloodSize)*fee
	//   + bytes0                         (lane-0 self-mint at serve via node handler)
	//   - bytes0                         (eviction reversal of lane-0 self-mint)
	//   + nodFloodSize*floodBytes        (flood self-mints, all legitimately unwitnessed)
	//   - fee                            (ChargePublish debit from fetcher)
	//   + fee                            (conserved fee credited at redeem)
	//   = initial + nodFloodSize*floodBytes
	//
	// Under the bug (no eviction reversal), bytes0 is NOT subtracted:
	//   gotTotal = initial + bytes0 + nodFloodSize*floodBytes
	//   delta    = +bytes0 = +1024 — the leaked mint.
	wantTotal := initial + int64(nodFloodSize)*floodBytes
	gotTotal := sumLedger()
	if gotTotal != wantTotal {
		delta := gotTotal - wantTotal
		t.Errorf("R0.5 node-path conservation VIOLATED:\n"+
			"  Σbalances+Σescrow = %d\n"+
			"  want              = %d\n"+
			"  delta             = %+d\n"+
			"  preRedeemTotal=%d initial=%d fee=%d bytes0=%d skim0=%d\n"+
			"  If delta == +%d: evicted lane's self-mint NOT reversed — A4 claw-back missing.\n"+
			"  If delta is 0 but preRedeemTotal == wantTotal+fee: redeem did not fire (bank rejected receipt).",
			gotTotal, wantTotal, delta,
			preRedeemTotal, initial, int64(fee), int64(bytes0), skim0,
			int64(bytes0))
	}

	// Additional guard: if the bank rejected the receipt, conservation can still look
	// correct (ChargePublish debit not recovered), but witnessed demand stays at 0.
	// Verify the bank actually accepted the receipt (demand > 0).
	if got := nd.WitnessedDemand(objRoot); got == 0 {
		t.Errorf("demand bank did not bank the receipt (WitnessedDemand=0) — " +
			"RedeemDeliveryCredit at demandrole.go:201 may not have been called. " +
			"Check that the receipt's Fetcher key hashes correctly to fetcherID.")
	}
}
