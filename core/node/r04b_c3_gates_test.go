package node

// R0.4b C3 close — the node-layer gates for the red-team's confirmed breaks
// (2026-09-02). Each was a PASSING red-team probe against the pre-fix tree; each is
// inverted here into a permanent assertion, with its ablation named.
//
//   Gate A   TestStaleIssuerKeyRegDoesNotMuteTheProposer      (break 2, probes A/A2)
//   Gate H   TestPinFollowsTheChainAcrossAReorg               (break 4, probe H)
//   Gate C   TestCohortKeyIsADenialOnEveryShippedLane         (break 5, probe C)
//   Gate B   TestDemandLaneOutlivesTheWindowAndARestart       (break 3, probe B)
//
// HARNESS NOTE (the red-team's own gotcha, worth keeping): a v5 block's committed
// root covers validatorsSeen, which apply() derives from the block's Atts. A minter
// that introduces an attester not yet in validatorsSeen produces roots the proposer
// signed before gathering, so the commit mismatches. Every chain here therefore seeds
// its fixed attester into GENESIS Atts and re-uses it. (The general hazard is filed
// as R-BOX-ATTESTS and routed for its own certification; it is not R0.4b's to fix.)

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/diskissuer"
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

const c3EpochBlocks = 8

// c3Attester is a fixed non-proposer attester seeded into every fixture's genesis
// Atts (see the harness note above).
var c3Attester = identity.FromSeed(7799).Signer()

func c3Entry(tag string) ports.Entry {
	return ports.Entry{Root: ports.HashBytes([]byte(tag)),
		ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte(tag + "/m"))}, FileSize: 100}
}

// c3Chain builds a chain with a REAL epoch clock (every other node fixture runs at
// EpochBlocks = 0, where the epoch is 0 forever and none of this is reachable),
// minting v5 from era4At, in legacy mode so block production stays plain.
func c3Chain(t *testing.T, era4At uint64, genesisSigner ed25519.PrivateKey, regs ...chain.IssuerKeyReg) *chain.Chain {
	t.Helper()
	c := chain.New(chain.Config{Quorum: 1, EpochBlocks: c3EpochBlocks,
		Era3ActivationHeight: 1, Era4ActivationHeight: era4At},
		func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{c3Entry("c3-genesis")},
		IssuerKeys: regs,
	}
	chain.Sign(&g, genesisSigner)
	g.Atts = []chain.Attestation{chain.AttestAt(&g, c3Attester, 0, chain.PhasePrecommit)}
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c
}

// c3Mint appends the next block the way ANOTHER validator would: keys[0] proposes,
// every key plus the fixed attester attests both phases. This is how the chain
// advances while the node under test is NOT the proposer.
func c3Mint(t *testing.T, c *chain.Chain, keys []ed25519.PrivateKey, regs ...chain.IssuerKeyReg) {
	t.Helper()
	prev, next := c.Head()
	b := &chain.Block{Height: next, Prev: prev,
		Entries:    []ports.Entry{c3Entry(fmt.Sprintf("c3-entry-%d", next))},
		IssuerKeys: regs}
	switch mv := c.MintVersion(next); {
	case mv >= chain.BlockVersionWitnessable:
		if err := c.PopulateEra4Roots(b); err != nil {
			t.Fatalf("era-4 roots at %d: %v", next, err)
		}
	case mv >= chain.BlockVersionStateRoot:
		if err := c.PopulateEra3Roots(b); err != nil {
			t.Fatalf("era-3 roots at %d: %v", next, err)
		}
	default:
		b.Version = chain.BlockVersionRounds
	}
	chain.Sign(b, keys[0])
	for _, k := range append(keys, c3Attester) {
		b.PrepareQC = append(b.PrepareQC, chain.AttestAt(b, k, 0, chain.PhasePrepare))
	}
	for _, k := range append(keys, c3Attester) {
		b.Atts = append(b.Atts, chain.AttestAt(b, k, 0, chain.PhasePrecommit))
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("append height %d (v%d): %v", next, b.Version, err)
	}
}

func c3Advance(t *testing.T, c *chain.Chain, keys []ed25519.PrivateKey, epochs int) {
	t.Helper()
	for i := 0; i < epochs*c3EpochBlocks; i++ {
		c3Mint(t, c, keys)
	}
}

func c3Node(t *testing.T, seed int64) (*Node, ed25519.PrivateKey) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(seed)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	return nd, ident.Signer()
}

// c3ProposeErr drives the node's real propose path and returns the synchronous error.
// The local pre-check fires before any gather, so a pre-check failure is synchronous
// and needs no peers.
func c3ProposeErr(nd *Node) error {
	prev, height := nd.chain.Head()
	var got error
	fired := false
	nd.proposeBlock(&chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev,
		Entries: []ports.Entry{c3Entry(fmt.Sprintf("own-%d", height))}},
		nil, nil, 1, func(err error) { got, fired = err, true })
	if !fired {
		return errors.New("proposeBlock did not fire done synchronously (it went to gather)")
	}
	return got
}

// ---------------------------------------------------------------------------
// GATE A — a stale registration must not mute the proposer.
// ---------------------------------------------------------------------------

// TestStaleIssuerKeyRegDoesNotMuteTheProposer (break 2, probe A). A node that stages
// key_E and then does NOT propose inside epoch E — a non-anchor in the launch window,
// a late joiner, a restart late in an epoch — used to carry that registration in
// pendingIssuerKeys FOREVER. validateIssuerKeys rejects a backdated epoch, correctly,
// so every later proposal failed the node's OWN local pre-check with
// ErrIssuerKeyEpoch: a permanent, restart-only proposer mute triggered by ordinary
// operation.
//
// ABLATION: remove the `r.Epoch < blockEpoch` drop at the pendingIssuerKeys fold
// (core/node/chainrole.go) → RED on the epoch-1 proposal.
func TestStaleIssuerKeyRegDoesNotMuteTheProposer(t *testing.T) {
	nd, signer := c3Node(t, 7401)
	others := []ed25519.PrivateKey{identity.FromSeed(7402).Signer()}
	c := c3Chain(t, 1, others[0]) // v5 from height 1
	nd.EnableChain(c, signer)

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	nd.SetDemandIssuerKey(nd.DemandEpoch(), rsaKey)
	if nd.DemandEpoch() != 0 || len(nd.pendingIssuerKeys) != 1 {
		t.Fatalf("setup: bootEpoch=%d pending=%d", nd.DemandEpoch(), len(nd.pendingIssuerKeys))
	}

	// The chain advances a whole epoch without this node proposing.
	c3Advance(t, c, others, 1)
	if got := nd.DemandEpoch(); got != 1 {
		t.Fatalf("chain epoch %d, want 1", got)
	}

	if err := c3ProposeErr(nd); err != nil && strings.Contains(err.Error(), "local pre-check") {
		t.Fatalf("epoch 1: the node cannot propose because a stale key registration is "+
			"still queued — one missed epoch mutes the proposer forever: %v", err)
	}
	if len(nd.pendingIssuerKeys) != 0 {
		t.Fatalf("the stale registration is still queued (%d); it must be DROPPED so the "+
			"rotation schedule can re-stage a registrable epoch", len(nd.pendingIssuerKeys))
	}
	// Ten more epochs, and the public publish path too.
	c3Advance(t, c, others, 10)
	var pubErr error
	nd.ProposeEntry(c3Entry("publish-attempt"), nil, nil, 1, func(e error) { pubErr = e })
	if pubErr != nil && errors.Is(pubErr, chain.ErrIssuerKeyEpoch) {
		t.Fatalf("epoch 11 ProposeEntry still muted: %v", pubErr)
	}
}

// TestStaleIssuerKeyRegPreFlipBoot (break 2, probe A2) is the pre-era-4 variant: the
// node boots BELOW the v5 flip, so its registration is staged but never folded
// (MintVersion < 5). By the time the flip arrives the reg is stale, and it used to
// ride the first v5 proposal and mute it.
func TestStaleIssuerKeyRegPreFlipBoot(t *testing.T) {
	nd, signer := c3Node(t, 7411)
	others := []ed25519.PrivateKey{identity.FromSeed(7412).Signer()}
	const flipAt = 24 // epoch 3
	c := c3Chain(t, flipAt, others[0])
	nd.EnableChain(c, signer)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	nd.SetDemandIssuerKey(nd.DemandEpoch(), rsaKey) // epoch 0, pre-flip

	for {
		_, next := c.Head()
		if next >= flipAt {
			break
		}
		c3Mint(t, c, others)
	}
	if mv := c.MintVersion(flipAt); mv != chain.BlockVersionWitnessable {
		t.Fatalf("mint version at %d = %d, want v5", flipAt, mv)
	}
	if err := c3ProposeErr(nd); err != nil && errors.Is(err, chain.ErrIssuerKeyEpoch) {
		t.Fatalf("the first v5 proposal after a pre-flip boot is muted by the stale "+
			"boot-epoch registration: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GATE H — the pin follows the chain.
// ---------------------------------------------------------------------------

// TestPinFollowsTheChainAcrossAReorg (break 4, probe H). The pin is a CACHE of the
// chain's committed E ↦ key_E binding, not an independent record. It used to be
// append-only in its own right: after a reorg onto a fork committing a DIFFERENT
// key_0, the redeemer kept verifying against the ABANDONED fork's key and refused the
// canonical one for W+1 epochs — and pinDemandIssuerKey reported the re-pin as a
// success while changing nothing.
//
// ABLATION: remove the read-time re-validation in DemandIssuerKeyset (the ks.Retain
// call), or restore the `if ks.Key(epoch) != nil { return true }` early return in
// pinDemandIssuerKey → RED.
func TestPinFollowsTheChainAcrossAReorg(t *testing.T) {
	nd, signer := c3Node(t, 7441)
	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	forkA := c3Chain(t, 1, signer, chain.SignIssuerKeyReg(signer, 0, demand.KeyFingerprint(&keyA.PublicKey)))
	nd.EnableChain(forkA, signer)
	nd.EnableDemandBank(nd.ID())
	if !nd.pinDemandIssuerKey(nd.ID(), 0, &keyA.PublicKey) {
		t.Fatal("setup: key_A was not pinned on fork A")
	}

	// Reorg: the node adopts fork B, which binds key_B for the same epoch.
	forkB := c3Chain(t, 1, signer, chain.SignIssuerKeyReg(signer, 0, demand.KeyFingerprint(&keyB.PublicKey)))
	nd.EnableChain(forkB, signer)

	tokA, tokB := blindTokenUnder(t, keyA), blindTokenUnder(t, keyB)
	ks := nd.DemandIssuerKeyset(nd.ID())
	if _, ok := ks.VerifyInWindow(0, tokA); ok {
		t.Fatal("after the reorg the redeemer still accepts the ABANDONED fork's key_A — " +
			"the pin outlived the commitment it was a cache of")
	}
	if !nd.pinDemandIssuerKey(nd.ID(), 0, &keyB.PublicKey) {
		t.Fatal("the canonical key_B was refused")
	}
	if _, ok := nd.DemandIssuerKeyset(nd.ID()).VerifyInWindow(0, tokB); !ok {
		t.Fatal("pinDemandIssuerKey reported success for the canonical key_B but the " +
			"keyset does not verify its tokens — a re-pin that changes nothing")
	}
	// And an off-commitment key is still refused: following the chain is not laxity.
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if nd.pinDemandIssuerKey(nd.ID(), 0, &other.PublicKey) {
		t.Fatal("an off-commitment key was pinned")
	}
}

// ---------------------------------------------------------------------------
// GATE C — a cohort key is a DENIAL on every shipped lane.
// ---------------------------------------------------------------------------

// TestCohortKeyIsADenialOnEveryShippedLane (break 5, probe C). R0.4b made the bank
// accept W+1 distinct committed keys at once. The shipped withdrawal paths fetched
// the issuer key UNPINNED, so a Byzantine issuer could serve cohort A key_E and
// cohort B key_{E+1} — both legitimately committed, both in window — and "which key
// verified you" became a tag on an ACCEPTED token. Pre-R0.4b the bank held ONE key,
// so a cohort key was a denial.
//
// Under (b1) the closure is structural: the requester names its epoch and blinds it
// in, so an issuer that answers under any other epoch's key produces a reply the
// withdrawal REFUSES. This gate drives the two shipped shapes — the `swarm receipt`
// shape (AcquireDemandTokenInWindow) and the D3 shape (AcquireDemandTokenWithCredit,
// key resolved by the durable parent) — against exactly that issuer.
//
// ABLATION: drop the resp.Height != epoch refusal in withdrawDemandToken, or blind
// against IssuerKeyOf instead of the resolved keyset → RED.
func TestCohortKeyIsADenialOnEveryShippedLane(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	issuerIdent := identity.FromSeed(7431)
	fetcherIdent := identity.FromSeed(7433)

	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// BOTH keys legitimately committed: key_A for epoch 0, key_B pre-published for
	// epoch 1. Nothing here is off-commitment — that is the point. Pre-R0.4b the bank
	// held ONE key, so a cohort key was a denial; R0.4b made W+1 keys acceptable at
	// once, which is what turned it into a tag.
	c := c3Chain(t, 1, issuerIdent.Signer(),
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 0, demand.KeyFingerprint(&keyA.PublicKey)),
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 1, demand.KeyFingerprint(&keyB.PublicKey)))

	fetcher := New(fetcherIdent.NodeID(), DefaultConfig(), sched,
		net.Endpoint(fetcherIdent.NodeID()), memstore.New())
	fetcher.SetSigner(fetcherIdent.Signer())
	fetcher.SetLedger(credit.New(50_000, 500_000))
	fetcher.EnableChain(c, fetcherIdent.Signer())

	// The BYZANTINE ISSUER is a bare endpoint, not a Node: it serves the honest
	// committed key window (so the fetcher's pin succeeds — nothing here is
	// off-commitment) but answers EVERY withdrawal under the other committed key,
	// naming that key's epoch. That is the cohort-tagging move.
	iep := net.Endpoint(issuerIdent.NodeID())
	iep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		switch msg.Kind {
		case ports.MsgGetDemandIssuerKeys:
			w := demandKeysetWire{Keys: []epochKeyDER{
				{Epoch: 0, DER: blindtoken.MarshalPub(&keyA.PublicKey)},
			}}
			blob, merr := cbor.Marshal(w)
			if merr != nil {
				t.Error(merr)
				return
			}
			_ = iep.Send(from, ports.Message{Kind: ports.MsgDemandIssuerKeysReply, RID: msg.RID, OK: true, Data: blob})
		case ports.MsgDemandTokenRequest:
			_ = iep.Send(from, ports.Message{Kind: ports.MsgDemandTokenReply, RID: msg.RID, OK: true,
				Data: demand.SignWithdrawal(keyB, msg.Data), Height: 1})
		}
	})

	// The fetcher resolves the issuer's key window against the committed binding.
	var pinned int
	fetcher.FetchDemandIssuerKeys(issuerIdent.NodeID(), func(n int, _ error) { pinned = n })
	sched.Run()
	if pinned != 1 {
		t.Fatalf("setup: the fetcher pinned %d keys, want 1", pinned)
	}
	pub, epoch, ok := fetcher.ResolvedDemandIssuerKey(issuerIdent.NodeID())
	if !ok || demand.KeyFingerprint(pub) != demand.KeyFingerprint(&keyA.PublicKey) || epoch != 0 {
		t.Fatalf("setup: resolved epoch %d ok=%v — want key_A at epoch 0", epoch, ok)
	}

	// Shape 1 — the `swarm receipt` lane.
	var tok demand.Token
	var gotErr error
	fetcher.AcquireDemandTokenInWindow(rand.Reader, issuerIdent.NodeID(), func(tk demand.Token, _ uint64, err error) {
		tok, gotErr = tk, err
	})
	sched.Run()
	if gotErr == nil {
		t.Fatal("the swarm-receipt lane ACCEPTED a token signed under a different committed " +
			"key than the one it withdrew against — 'which key verified you' is then a cohort " +
			"tag on an accepted token, not a denial")
	}
	if !errors.Is(gotErr, ErrDemandEpochMismatch) {
		t.Fatalf("swarm-receipt lane refused for %v, want ErrDemandEpochMismatch", gotErr)
	}
	if len(tok.Serial) != 0 {
		t.Fatal("a refused withdrawal must yield no token at all")
	}

	// Shape 2 — the D3 lane, key AND epoch resolved by the durable parent.
	gotErr = nil
	fetcher.AcquireDemandTokenWithCredit(rand.Reader, issuerIdent.NodeID(), pub, epoch,
		ports.PublishCredit{}, func(tk demand.Token, err error) { tok, gotErr = tk, err })
	sched.Run()
	if gotErr == nil {
		t.Fatal("the D3 lane ACCEPTED a cohort-tagged token")
	}
	if !errors.Is(gotErr, ErrDemandEpochMismatch) {
		t.Fatalf("D3 lane refused for %v, want ErrDemandEpochMismatch", gotErr)
	}

	// And even if a fetcher kept a signature made under the OTHER key, no bank would
	// credit it at the epoch the withdrawal named: (b1) binds the epoch into the
	// signed message, so the pair (key_A, 0) is the only one that verifies.
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, err := demand.Withdraw(rand.Reader, pub, epoch, serial)
	if err != nil {
		t.Fatal(err)
	}
	crossTok := demand.Unblind(pub, serial, demand.SignWithdrawal(keyB, blinded), secret)
	ks := fetcher.DemandIssuerKeyset(issuerIdent.NodeID())
	if _, ok := ks.VerifyInWindow(0, crossTok); ok {
		t.Fatal("a signature made under key_B verified in a keyset holding key_A")
	}
}

// ---------------------------------------------------------------------------
// GATE B — the lane outlives its window, and a restart.
// ---------------------------------------------------------------------------

// TestDemandLaneOutlivesTheWindowAndARestart (break 3, probe B). The daemon installed
// the persisted PUBLISH key as key_{boot} once, with no rotation: from boot+1 the
// demand lane refused to issue, and from boot+W+1 the bank rejected everything the
// lane had signed while the withdrawal path still charged the fee.
//
// This drives the daemon's own rotation step (installDemandKeys is shared with
// cmd/silt/daemon.go, so a drift there is caught here) across more than W+1 epochs
// AND across a RESTART — a fresh node reading the same key store — asserting that a
// token can still be withdrawn and redeemed at every step.
//
// ABLATION: skip the per-epoch installDemandKeys call (install once at boot, the old
// behaviour) → RED at epoch 1 on "the demand lane refused to issue".
func TestDemandLaneOutlivesTheWindowAndARestart(t *testing.T) {
	dir := t.TempDir()
	es, err := diskissuer.OpenEpochs(dir)
	if err != nil {
		t.Fatal(err)
	}
	signer := identity.FromSeed(7421).Signer()
	others := []ed25519.PrivateKey{identity.FromSeed(7422).Signer()}
	c := c3Chain(t, 1, others[0])

	// boot brings a node up against the shared chain and runs one rotation step, the
	// same two calls cmd/silt/daemon.go makes at boot.
	boot := func() *Node {
		nd, _ := c3Node(t, 7421)
		nd.SetLedger(credit.New(50_000, 5_000_000))
		nd.EnableChain(c, signer)
		if err := es.RotateWindow(rand.Reader, nd.DemandEpoch(), demand.DefaultWindow, nd.SetDemandIssuerKey); err != nil {
			t.Fatalf("rotation at boot: %v", err)
		}
		nd.EnableDemandBank(nd.ID())
		return nd
	}
	nd := boot()

	// commitStaged folds the node's staged registrations onto the chain the way any
	// proposer would. The registrations must LAND for the lane to work at all: an
	// unanchored key is refused by design.
	commitStaged := func(nd *Node) {
		_, next := c.Head()
		blockEpoch := c.BlockEpoch(next)
		var regs []chain.IssuerKeyReg
		for _, r := range nd.pendingIssuerKeys {
			// Mirror the proposer fold: already-committed and STALE regs are dropped.
			if r.Epoch < blockEpoch {
				continue
			}
			if _, done := c.IssuerKeyCommitment(r.IssuerID(), r.Epoch); !done {
				regs = append(regs, r)
			}
		}
		if len(regs) > 0 {
			c3Mint(t, c, others, regs...)
		}
	}
	commitStaged(nd)

	fetcherIdent := identity.FromSeed(7424)
	obj := ports.HashBytes([]byte("c3-object"))

	// cycle: withdraw at the current epoch on the demand lane, ack, redeem. Returns
	// the credit paid.
	cycle := func(nd *Node, label string) int64 {
		cur := nd.DemandEpoch()
		iss := nd.demandIssuers[cur]
		if iss == nil {
			t.Fatalf("%s: the demand lane holds no key for epoch %d — without a per-epoch "+
				"schedule the lane dies once the boot band runs out (immediately at boot+1 "+
				"with a single installed key, at boot+W+1 with a pre-published band)", label, cur)
		}
		serial, _ := blindtoken.NewSerial(rand.Reader)
		ks := nd.DemandIssuerKeyset(nd.ID())
		if ks == nil || ks.Key(cur) == nil {
			t.Fatalf("%s: no COMMITTED key for epoch %d — the schedule did not pre-publish", label, cur)
		}
		pub := ks.Key(cur)
		blinded, secret, err := demand.Withdraw(rand.Reader, pub, cur, serial)
		if err != nil {
			t.Fatal(err)
		}
		reply := nd.answerDemandTokenRequest(fetcherIdent.NodeID(),
			ports.Message{Kind: ports.MsgDemandTokenRequest, Data: blinded, Height: cur})
		if !reply.OK {
			t.Fatalf("%s: the demand lane refused to issue at epoch %d", label, cur)
		}
		if reply.Height != cur {
			t.Fatalf("%s: the issuer signed for epoch %d, not the %d asked for", label, reply.Height, cur)
		}
		tok := demand.Unblind(pub, serial, reply.Data, secret)
		rcpt := demand.Ack(fetcherIdent.Signer(), tok, obj, nd.ID())
		credited, ep, why := nd.demandBank.Redeem(ks, cur, tok, rcpt)
		if !credited {
			t.Fatalf("%s: the bank refused a token it had just issued at epoch %d: %s", label, cur, why)
		}
		return nd.ledger.RedeemDeliveryCredit(nd.ID(), ports.HashBytes(rcpt.Fetcher), obj, rcpt.Serial, ep, cur)
	}
	if paid := cycle(nd, "epoch 0"); paid == 0 {
		t.Fatal("epoch 0: nothing paid")
	}

	// Walk well past the old boot+W+1 death point, rotating each epoch as the daemon
	// does on the commit stream.
	for e := 1; e <= int(demand.DefaultWindow)+3; e++ {
		c3Advance(t, c, others, 1)
		if err := es.RotateWindow(rand.Reader, nd.DemandEpoch(), demand.DefaultWindow, nd.SetDemandIssuerKey); err != nil {
			t.Fatalf("rotation at epoch %d: %v", e, err)
		}
		commitStaged(nd)
		if paid := cycle(nd, fmt.Sprintf("epoch %d", e)); paid == 0 {
			t.Fatalf("epoch %d: the lane issued but nothing was paid", e)
		}
	}

	// RESTART: a fresh process, the same key store, the same chain. The keys for the
	// epochs whose fingerprints are already committed must come back off disk — a
	// regenerated band would be un-committable (the binding is append-only and
	// backdating is rejected), so the lane would be dead for W epochs.
	restarted := boot()
	commitStaged(restarted)
	if paid := cycle(restarted, "after restart"); paid == 0 {
		t.Fatal("after a restart the lane issued but nothing was paid")
	}
	// And the publish key is NEVER in the demand keyset (the C3 structural rule).
	pubKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	restarted.EnableTokenIssuer(pubKey)
	ks := restarted.DemandIssuerKeyset(restarted.ID())
	for e := uint64(0); e <= restarted.DemandEpoch(); e++ {
		if k := ks.Key(e); k != nil && demand.KeyFingerprint(k) == demand.KeyFingerprint(&pubKey.PublicKey) {
			t.Fatalf("the publish key entered the demand keyset at epoch %d", e)
		}
	}
}
