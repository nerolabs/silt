package chain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	mrand "math/rand"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// #183 red-team F-1 (the reorder half): ValidateEntry must reject a REPLAYED
// (already-spent) token on the cheap serial map-lookup BEFORE it runs the RSA
// signature verify. Committed tokens are public on the append-only chain, so an
// attacker pairs a harvested valid token with a novel Root and floods; if the
// spent-check sits AFTER publishtoken.Verify, every genuine signature is
// verified to completion (N RSA modexps on the single consensus loop) before
// the replay is caught — the CPU amplifier F-1 names.
//
// Deterministic RED/GREEN by ERROR KIND (a proxy for "Verify was not reached"):
// with a spent serial AND tampered signatures, the pre-fix order returns the
// signature-verify error; the fixed order returns ErrTokenSpent, which is only
// reachable if the spent-check ran first — i.e. the N modexps were skipped.
func TestValidateEntry_183_SpentTokenRejectedBeforeVerify(t *testing.T) {
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4)}
	issuers := map[ports.NodeID]*rsa.PrivateKey{}
	reps := map[ports.NodeID]int64{idOf(prop): 1000}
	for _, v := range vals {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		issuers[idOf(v)] = k
		reps[idOf(v)] = 1000
	}
	c := New(Config{MinProposerRep: 100, MinAttesterRep: 100, Quorum: 2},
		func(n ports.NodeID) int64 { return reps[n] })
	c.RequireTokens(2, func(n ports.NodeID) *rsa.PublicKey {
		if k, ok := issuers[n]; ok {
			return &k.PublicKey
		}
		return nil
	})

	rng := mrand.New(mrand.NewSource(1))
	mint := func(serial []byte, signers []ed25519.PrivateKey) *ports.PublishToken {
		tok := &ports.PublishToken{Serial: serial}
		for _, v := range signers {
			iss := blindtoken.NewIssuer(rand.Reader, issuers[idOf(v)])
			blinded, secret, err := blindtoken.Blind(rng, iss.Public(), serial)
			if err != nil {
				t.Fatal(err)
			}
			blindSig, _ := iss.Issue(func() error { return nil }, blinded)
			sig, uerr := blindtoken.Unblind(iss.Public(), serial, blindSig, secret)
			if uerr != nil {
				t.Fatalf("unblind: %v", uerr)
			}
			tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: idOf(v), Sig: sig})
		}
		return tok
	}
	withToken := func(b byte, tok *ports.PublishToken) ports.Entry {
		e := entry(b)
		e.Token = tok
		return e
	}
	commit := func(e ports.Entry) error {
		prev, height := c.Head()
		blk := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{e}}
		Sign(blk, prop)
		blk.Atts = append(blk.Atts, Attest(blk, vals[0]), Attest(blk, vals[1]))
		return c.Append(*blk)
	}

	serial, _ := blindtoken.NewSerial(rng)
	// Commit a valid token so its serial is now SPENT chain-wide (the attacker's
	// harvested-from-the-chain token).
	if err := commit(withToken(1, mint(serial, vals[:2]))); err != nil {
		t.Fatalf("setup: a valid quorum-token entry should commit; got %v", err)
	}

	// The replay: the SAME (now-spent) serial, novel Root, and TAMPERED
	// signatures. If the spent-check runs first, this fails ErrTokenSpent and
	// the RSA verify never runs. If verify runs first (the pre-fix bug), it
	// fails on the bad signatures — a different error, and the N modexps burned.
	replay := withToken(2, mint(serial, vals[:2]))
	replay.Token.Sigs[0].Sig = append([]byte(nil), replay.Token.Sigs[0].Sig...)
	replay.Token.Sigs[0].Sig[0] ^= 0xff // corrupt the first signature
	err := c.ValidateEntry(replay)
	if !errors.Is(err, ErrTokenSpent) {
		t.Fatalf("#183 F-1: a spent token with tampered sigs must be rejected by the cheap spent-check BEFORE the RSA verify (want ErrTokenSpent, proving verify was skipped); got %v", err)
	}

	// Sanity: an UNSPENT token with the same tampering DOES reach verify and
	// fails there — so the spent-check is an early-out, not a blanket bypass.
	serial2, _ := blindtoken.NewSerial(rng)
	fresh := withToken(3, mint(serial2, vals[:2]))
	fresh.Token.Sigs[0].Sig = append([]byte(nil), fresh.Token.Sigs[0].Sig...)
	fresh.Token.Sigs[0].Sig[0] ^= 0xff
	if err := c.ValidateEntry(fresh); err == nil || errors.Is(err, ErrTokenSpent) {
		t.Fatalf("#183 F-1: a FRESH tampered token must still fail at verify (not ErrTokenSpent, not nil); got %v", err)
	}
}
