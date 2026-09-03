package node

// RED-TEAM: classes the task named that I could NOT break. Each RUNS the attack and
// records the measurement that refuted it.

import (
	"crypto/ed25519"
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
)

// RT-C3B-20 (REFUTED). Two epoch clocks run in this system:
//   - the chain's prune clock, blockEpoch(b.Height) — retains [cur_c-4, cur_c+4];
//   - the node's keyset clock, chainEpoch() = (last.Height+1)/EpochBlocks — window
//     [cur_n-W, cur_n].
//
// DemandIssuerKeyset drops any held key whose commitment the chain no longer has. If
// cur_n ever exceeded cur_c by more than the band's slack, an in-window honest token
// would stop verifying EARLY. This walks every height across ten epoch boundaries and
// measures the two clocks and the resulting band containment.
func TestRTC3B_ChainPruneBandNeverUndercutsTheKeysetWindow(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	ident := identity.FromSeed(9301)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := c3Chain(t, 1, ident.Signer(),
		chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&key.PublicKey)))
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	nd.SetLedger(credit.New(50_000, 500_000))
	nd.EnableChain(c, ident.Signer())

	minted := []ed25519.PrivateKey{ident.Signer()}
	maxSkew := uint64(0)
	for i := 0; i < 10*c3EpochBlocks; i++ {
		c3Mint(t, c, minted)
		_, next := c.Head()
		curChain := c.BlockEpoch(next - 1) // the epoch the LAST APPLIED block pruned against
		curNode := nd.chainEpoch()         // the epoch the keyset window is measured at
		if curNode < curChain {
			t.Fatalf("height %d: the node clock (%d) fell BEHIND the prune clock (%d)", next-1, curNode, curChain)
		}
		if s := curNode - curChain; s > maxSkew {
			maxSkew = s
		}
		// Band containment: the chain retains e >= curChain-4; the keyset wants
		// e >= curNode-W. Containment requires curNode-W >= curChain-4.
		var keysetFloor, chainFloor uint64
		if curNode > demand.DefaultWindow {
			keysetFloor = curNode - demand.DefaultWindow
		}
		if curChain > 4 {
			chainFloor = curChain - 4
		}
		if keysetFloor < chainFloor {
			t.Fatalf("BREAK at height %d: keyset wants epoch %d but the chain has pruned "+
				"below %d — an in-window token stops verifying early", next-1, keysetFloor, chainFloor)
		}
	}
	t.Logf("RT-C3B-20 REFUTED over 10 epoch boundaries: max(node clock - prune clock) = %d, "+
		"and the chain's retained floor never rose above the keyset's wanted floor. "+
		"Head() returning last.Height+1 makes the node clock lead by at most 1 epoch, and "+
		"the chain's band is W deep on the same side, so containment holds with slack.", maxSkew)
}

// RT-C3B-21 (REFUTED). Shipped-lane Height != E. The withdrawal names E in
// Message.Height and refuses resp.Height != E. The attack is an issuer that ECHOES the
// requested E (passing that check) while signing under a DIFFERENT epoch's key that it
// ALSO holds legitimately — the shape a shared/persisted RSA key across epochs allows.
func TestRTC3B_EchoedEpochWithAnotherEpochsKeyIsStillADenial(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	issuerIdent := identity.FromSeed(9401)
	fetcherIdent := identity.FromSeed(9403)

	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := c3Chain(t, 1, issuerIdent.Signer(),
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 0, demand.KeyFingerprint(&keyA.PublicKey)),
		// The SAME RSA key committed for epoch 1 too — an ordinary restart produces this
		// and no validity rule forbids it (issuerkey.go, "WHAT APPEND-ONLY DOES NOT BUY").
		chain.SignIssuerKeyReg(issuerIdent.Signer(), 1, demand.KeyFingerprint(&keyA.PublicKey)))

	fetcher := New(fetcherIdent.NodeID(), DefaultConfig(), sched,
		net.Endpoint(fetcherIdent.NodeID()), memstore.New())
	fetcher.SetSigner(fetcherIdent.Signer())
	fetcher.SetLedger(credit.New(50_000, 500_000))
	fetcher.EnableChain(c, fetcherIdent.Signer())

	ks := demand.NewKeyset(demand.DefaultWindow)
	ks.Put(0, &keyA.PublicKey)
	ks.Put(1, &keyA.PublicKey)

	// A token honestly withdrawn for epoch 0 under the shared key.
	serial, _ := blindtokenSerial(t)
	blinded, secret, err := demand.Withdraw(rand.Reader, &keyA.PublicKey, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, uerr := demand.Unblind(&keyA.PublicKey, 0, serial, demand.SignWithdrawal(rand.Reader, keyA, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}

	// The re-dating attempt: can the epoch-0 token be read as an epoch-1 token, so a
	// guard entry swept at epoch 0's expiry leaves a still-verifying token?
	if e, ok := ks.VerifyInWindow(1, tok); !ok || e != 0 {
		t.Fatalf("setup: the epoch-0 token must verify AT EPOCH 0 (got ok=%v e=%d)", ok, e)
	}
	t.Logf("RT-C3B-21 REFUTED: under ONE key committed for epochs 0 and 1, an epoch-0 token " +
		"still reads as epoch 0 — the (b1) FDH epoch binding makes issuedEpoch a pure " +
		"function of the token. The pre-fix re-dating is closed at this shape.")
}

func blindtokenSerial(t *testing.T) ([]byte, error) {
	t.Helper()
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		t.Fatal(err)
	}
	return s, nil
}
