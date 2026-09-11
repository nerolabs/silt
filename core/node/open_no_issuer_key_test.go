package node

// A server that could not resolve key_E against the COMMITTED binding refuses the
// open — it does not open an unguarded session.
//
// C1 re-home (2026-09-08). The v2 twin was core/demand TestRedeemWithNoKeysetRefuses:
// "a bank with no resolved keyset accepts nothing". Bank.Redeem is retired, and the
// surviving decision point is verifyDeliveryAnchors, which is where a server decides
// whether to spend an anchor into the shared guard. The safe default is refuse, not
// accept: a redeemer with nothing consensus-attested to resolve key_E against has no
// anti-fingerprinting anchor, and the R0.4b certification is explicit that running
// without one is unsafe.
//
// NOT DARKNESS. The refusal arm and the acceptance arm are the SAME node with the SAME
// anchor; only the committed key differs. Without the control arm this would be a
// fixture that refuses everything (the vacuous-gate scar, blind PE 2026-09-07).
//
// ABLATION (run RED 2026-09-08): make verifyDeliveryAnchors fall through on a nil
// keyset → "the open was ACCEPTED with no resolved issuer key".

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
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

func TestOpenWithNoResolvedIssuerKeyRefuses(t *testing.T) {
	issuerPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverIdent, fetcherIdent := identity.FromSeed(9501), identity.FromSeed(9502)
	serverID := serverIdent.NodeID()

	// build brings a server up with the delivery lane on. pinKey decides whether the
	// issuer key is committed AND held — the ONE difference between the two arms.
	build := func(pinKey bool) *Node {
		sched := simclock.New()
		net := simnet.New(sched, 3, simnet.DefaultConfig())
		nd := New(serverID, DefaultConfig(), sched, net.Endpoint(serverID), memstore.New())
		l := credit.New(50_000, 0)
		nd.SetLedger(l)
		nd.SetSigner(serverIdent.Signer())
		regs := []chain.IssuerKeyReg{}
		if pinKey {
			regs = append(regs, chain.SignIssuerKeyReg(serverIdent.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)))
		}
		c := c3Chain(t, 1, serverIdent.Signer(), regs...)
		nd.EnableChain(c, serverIdent.Signer())
		if pinKey {
			nd.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
		}
		nd.EnableDemandBank(serverID)
		nd.EnableDeliverySessions(10 * ports.Second)
		l.Register(serverID)
		l.Register(fetcherIdent.NodeID())
		return nd
	}

	// ARM 1 — no resolved key: refuse, by NAME, and spend nothing.
	blind := build(false)
	// ARM 2's server is built FIRST so the anchor can be minted on ITS network: the two
	// arms commit different issuer-key registrations, so they mint different genesis
	// blocks and therefore different chain ids (M3). The anchor must be a real one for the
	// network that will accept it, or arm 2 would refuse it for the wrong reason.
	pinned := build(true)

	// One real blind-withdrawn anchor under the issuer key, used by BOTH arms.
	serial := make([]byte, 32)
	if _, rerr := rand.Read(serial); rerr != nil {
		t.Fatal(rerr)
	}
	cid := pinned.chainID()
	blinded, secret, werr := demand.Withdraw(rand.Reader, &issuerPriv.PublicKey, cid, 0, serial)
	if werr != nil {
		t.Fatal(werr)
	}
	token, uerr := demand.Unblind(&issuerPriv.PublicKey, cid, 0, serial, demand.SignWithdrawal(rand.Reader, issuerPriv, cid, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	if ks := blind.DemandIssuerKeyset(serverID); ks != nil && ks.Key(0) != nil {
		t.Fatal("setup: the unpinned arm resolved a key after all")
	}
	_, oerr := blind.OpenDeliverySession(fetcherIdent.NodeID(),
		demand.SignSessionOpen(fetcherIdent.Signer(), serverID, []demand.Token{token}))
	if oerr == nil {
		t.Fatal("the open was ACCEPTED with no resolved issuer key — the server spent an anchor it could not verify against the committed binding")
	}
	if !errors.Is(oerr, errDeliveryNoIssuerKey) {
		t.Fatalf("the open was refused with %q, want %q — the refusal must name the missing binding, not some later check", oerr, errDeliveryNoIssuerKey)
	}
	if d, _ := blind.ledger.(*credit.Ledger).LivePaidSerialsByLane(); d != 0 {
		t.Fatalf("%d guard entries after a refused open — a refusal must record nothing", d)
	}

	// ARM 2 — the SAME anchor at a server whose key IS committed: it opens. Without
	// this the arm above measures a fixture that refuses everything.
	if _, cerr := pinned.OpenDeliverySession(fetcherIdent.NodeID(),
		demand.SignSessionOpen(fetcherIdent.Signer(), serverID, []demand.Token{token})); cerr != nil {
		t.Fatalf("the control open failed (%v) — the refusal above measures darkness, not the rule", cerr)
	}
}
