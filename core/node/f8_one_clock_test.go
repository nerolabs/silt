package node

// R2.10 / F8 — G-F8-5, ONE CLOCK (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §3.2 (the source is the node's chainEpoch() and nothing else; finalized-head
// REFUTED twice), §3.4 (R2.14's anchors on the same source), §6 G-F8-5.
//
// The property: the ledger's epoch and the keyset's prune epoch are the SAME value
// at every height, including boundary blocks. Two arms on the R2.14 cluster shape
// (EpochBlocks = 2, one relay, the relay's key_e committed at genesis for
// e = 0..issuerKeyPrePublish so an anchor can be minted AT the keyset's epoch on
// every boundary):
//
//   1. TestF8_LedgerEpochIsTheNodesChainEpochAtEveryBlock — with the ledger's source
//      wired to chainEpoch(), ledger.Epoch() == n.chainEpoch() after every appended
//      block, and an anchor minted at the keyset's epoch is admitted at every
//      boundary (never ReasonAnchorFuture). RED on main: Epoch() is a stub that
//      reads no source.
//   2. TestF8_LedgerFollowsItsSourceNotTheCaller — the DUAL, and the gate that refutes
//      the finalized-head source through the ledger itself: with the source wired to
//      the finalized-head epoch, the keyset sits at 1 on the first boundary while the
//      source reads 0, so an anchor verified in-window at the keyset MUST be refused
//      ReasonAnchorFuture. That refusal is (a) the proof the ledger reads its SOURCE
//      and not the caller's epoch (R-F8-SOURCE), and (b) exactly why finalized-head
//      is inadmissible in production (an honest-anchor denial once per epoch). RED
//      on main: SpendRelayAnchors still reads the caller's `cur`, so the anchor is
//      admitted.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// f8EpochFunc adapts a func to ports.EpochSource (the test-side wiring; the daemon's
// production wiring is G-F8-6 in cmd/silt).
type f8EpochFunc func() uint64

func (f f8EpochFunc) Epoch() uint64 { return f() }

// f8PrePublish mirrors chain.issuerKeyPrePublish (unexported there; pinned to
// demand.DefaultWindow by core/chain TestIssuerKeyPrePublishMatchesDemandWindow).
const f8PrePublish = int(demand.DefaultWindow)

type f8Relay struct {
	node   *Node
	ident  *identity.Identity
	keys   map[uint64]*rsa.PrivateKey // key_e, committed at genesis, e = 0..f8PrePublish
	ledger *credit.Ledger
}

type f8Cluster struct {
	t        *testing.T
	chain    *chain.Chain
	attester *identity.Identity
	relay    *f8Relay
	seq      int
}

// newF8Cluster is newAnchorCluster with ONE relay whose key_e is committed for every
// epoch in the genesis pre-publish window, so the keyset can verify an anchor AT its
// current epoch on every boundary the test walks.
func newF8Cluster(t *testing.T, seed int64, epochBlocks uint64) *f8Cluster {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	attester := identity.FromSeed(seed + 1000)
	ident := identity.FromSeed(seed)

	c := chain.New(chain.Config{Quorum: 1, EpochBlocks: epochBlocks}, func(ports.NodeID) int64 { return 1 << 30 })
	keys := map[uint64]*rsa.PrivateKey{}
	var regs []chain.IssuerKeyReg
	for e := 0; e <= f8PrePublish; e++ {
		k := cachedRSAKey(t, 40+e) // 40+: clear of the r214 fixtures' 0..n
		keys[uint64(e)] = k
		regs = append(regs, chain.SignIssuerKeyReg(ident.Signer(), uint64(e), demand.KeyFingerprint(&k.PublicKey)))
	}
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{{Root: ports.HashBytes([]byte("f8-genesis"))}},
		IssuerKeys: regs,
	}
	chain.Sign(&g, ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis with key_0..key_%d committed: %v", f8PrePublish, err)
	}
	for e, k := range keys {
		if got, ok := c.IssuerKeyCommitment(ident.NodeID(), e); !ok || got != demand.KeyFingerprint(&k.PublicKey) {
			t.Fatalf("setup: key_%d is not committed", e)
		}
	}
	r := &f8Relay{ident: ident, keys: keys}
	r.node = New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	r.node.SetSigner(ident.Signer())
	r.ledger = credit.New(r214Fee, r214Grant)
	r.ledger.Register(ident.NodeID())
	r.node.SetLedger(r.ledger)
	r.node.EnableChain(c, ident.Signer())
	for e := 0; e <= f8PrePublish; e++ {
		r.node.SetDemandIssuerKey(rand.Reader, uint64(e), keys[uint64(e)])
	}
	r.node.EnableRelayAccept()
	if ks := r.node.DemandIssuerKeyset(ident.NodeID()); ks == nil || ks.Key(0) == nil {
		t.Fatal("setup: the relay cannot resolve its OWN committed key_0")
	}
	return &f8Cluster{t: t, chain: c, attester: attester, relay: r}
}

// appendBlock appends ONE attested block (the advanceEpochs idiom, one step).
func (cl *f8Cluster) appendBlock() {
	cl.t.Helper()
	prev, next := cl.chain.Head()
	b := &chain.Block{Version: 1, Height: next, Prev: prev,
		Entries: []ports.Entry{mkEntry(fmt.Sprintf("f8-block-%d", cl.seq))}}
	cl.seq++
	chain.Sign(b, cl.relay.ident.Signer())
	b.Atts = []chain.Attestation{chain.Attest(b, cl.attester.Signer())}
	if err := cl.chain.Append(*b); err != nil {
		cl.t.Fatalf("append block %d: %v", next, err)
	}
}

// open signs M for the relay and presents the session (anchorRelay.open's twin).
func (r *f8Relay) open(t *testing.T, seed int64, tag string, anchors []relaypay.Anchor) (*RelaySession, error) {
	t.Helper()
	e := newEphemeral(seed)
	root := freshChain(t, tag, 8).Root()
	sig := signRelayOpen(e, r.ident.NodeID(), root, 8, anchors)
	return r.node.OpenRelaySession(e.id, root, 8, FundingEphemeralBlind, anchors, e.pub, sig)
}

func signRelayOpen(e ephemeral, relayID ports.NodeID, root []byte, S int, anchors []relaypay.Anchor) []byte {
	return ed25519.Sign(e.priv, relayOpenCommitmentIndependent(relayID, root, S, serialsOf(anchors)))
}

// TestF8_LedgerEpochIsTheNodesChainEpochAtEveryBlock is arm 1.
func TestF8_LedgerEpochIsTheNodesChainEpochAtEveryBlock(t *testing.T) {
	const EB = 2
	cl := newF8Cluster(t, 9400, EB)
	r := cl.relay
	r.ledger.SetEpochSource(f8EpochFunc(r.node.chainEpoch))

	if got, want := r.ledger.Epoch(), r.node.chainEpoch(); got != want {
		t.Fatalf("at genesis ledger.Epoch() = %d, chainEpoch() = %d — the ledger is not reading the node's clock", got, want)
	}
	prevE := r.node.chainEpoch()
	boundaries := 0
	// 2·f8PrePublish blocks walk epochs 1..f8PrePublish; every odd block is a boundary
	// (chainEpoch = (last.Height + 1) / EB).
	for i := 0; i < EB*f8PrePublish; i++ {
		cl.appendBlock()
		_, height := cl.chain.Head()
		e := r.node.chainEpoch()
		if got := r.ledger.Epoch(); got != e {
			t.Fatalf("after block %d (head %d): ledger.Epoch() = %d, chainEpoch() = %d — two clocks; "+
				"the keyset prunes at %d and the guard expires at %d (R-F8-SOURCE: ONE clock)",
				i+1, height, got, e, e, got)
		}
		if e == prevE {
			continue
		}
		prevE = e
		boundaries++
		// A boundary. An anchor minted under key_e for THIS epoch verifies in-window at
		// the keyset (VerifyAnchorInWindow starts at cur = e) and must be admitted: the
		// ledger's clock is the same e, so it is not future-dated.
		a := mintAnchorUnder(t, r.keys[e], e)
		sess, err := r.open(t, 9400+int64(i), fmt.Sprintf("f8-boundary-%d", e), []relaypay.Anchor{a})
		if err != nil {
			if strings.Contains(err.Error(), credit.ReasonAnchorFuture) {
				t.Fatalf("boundary block %d (epoch %d): an anchor minted AT the keyset's epoch was refused %q — "+
					"the ledger's clock is behind the keyset's (the finalized-head break, cert §3.2): %v",
					height-1, e, credit.ReasonAnchorFuture, err)
			}
			t.Fatalf("boundary block %d (epoch %d): open with an epoch-%d anchor failed: %v", height-1, e, e, err)
		}
		if sess == nil {
			t.Fatalf("boundary block %d: nil session with nil error", height-1)
		}
	}
	if boundaries != f8PrePublish {
		t.Fatalf("walked %d boundaries, want %d — the fixture did not exercise every epoch", boundaries, f8PrePublish)
	}
}

// TestF8_LedgerFollowsItsSourceNotTheCaller is arm 2, the dual (see the file note).
func TestF8_LedgerFollowsItsSourceNotTheCaller(t *testing.T) {
	const EB = 2
	cl := newF8Cluster(t, 9450, EB)
	r := cl.relay
	// The REFUTED source: the finalized-head epoch (cert §3.2). Read exactly as the
	// chain exposes it: (0, false) without BFT finality; last.Height / EB with it.
	finalizedHead := f8EpochFunc(func() uint64 {
		h, ok := cl.chain.FinalizedHeight()
		if !ok {
			return 0
		}
		return h / EB
	})
	r.ledger.SetEpochSource(finalizedHead)

	cl.appendBlock() // head → 2, the first boundary: keyset epoch 1
	if got := r.node.chainEpoch(); got != 1 {
		t.Fatalf("precondition: after one block chainEpoch() = %d, want 1 (EpochBlocks = %d)", got, EB)
	}
	if src := finalizedHead.Epoch(); src == 1 {
		t.Fatalf("precondition: the finalized-head epoch equals the keyset's at the first boundary (%d) — "+
			"the cert §3.2 refutation's premise does not hold on this fixture; this subtest cannot discriminate", src)
	}
	before := ledgerTotal(r.ledger)
	a1 := mintAnchorUnder(t, r.keys[1], 1)
	sess, err := r.open(t, 9451, "f8-finalized-head", []relaypay.Anchor{a1})
	if err == nil || sess != nil {
		t.Fatalf("with the ledger's source at %d and the keyset at 1, an epoch-1 anchor was ADMITTED (sess=%v, err=%v). "+
			"The ledger is not reading its injected source: SpendRelayAnchors still takes the caller's epoch "+
			"(R-F8-SOURCE). Want the refusal %q — which is also the honest-anchor denial that makes the "+
			"finalized-head source inadmissible (cert §3.2)", finalizedHead.Epoch(), sess != nil, err, credit.ReasonAnchorFuture)
	}
	if !errors.Is(err, errRelayGuardRefused) || !strings.Contains(err.Error(), credit.ReasonAnchorFuture) {
		t.Fatalf("refused for the wrong reason: %v, want errRelayGuardRefused naming %q", err, credit.ReasonAnchorFuture)
	}
	if after := ledgerTotal(r.ledger); after != before {
		t.Fatalf("a refused open moved the ledger total by %+d", after-before)
	}
}
