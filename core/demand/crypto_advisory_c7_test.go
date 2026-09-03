package demand

// Crypto-specialist advisory C-7 at the BANK, 2026-09-03. Same finding as
// core/credit's twin: the spent set was swept only at the 65,536 cap, so on any node
// below it — every node most of the time — an expired token's guard entry was retained
// indefinitely, past the W-epoch window that is its whole justification.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
)

func TestC7_SpentSetIsRetiredOnTheBandAdvanceNotOnlyAtTheCap(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := newScene(t, "obj-c7")
	ks := NewKeyset(DefaultWindow)
	for e := uint64(0); e <= DefaultWindow+2; e++ {
		ks.Put(e, &key.PublicKey)
	}
	bank := NewBank()

	// A handful of spends at epoch 0 — nowhere near maxSpentTokens.
	const n = 8
	for i := 0; i < n; i++ {
		serial, _ := blindtoken.NewSerial(rand.Reader)
		blinded, secret, werr := Withdraw(rand.Reader, &key.PublicKey, 0, serial)
		if werr != nil {
			t.Fatal(werr)
		}
		tok, uerr := Unblind(&key.PublicKey, 0, serial, SignWithdrawal(rand.Reader, key, blinded), secret)
		if uerr != nil {
			t.Fatal(uerr)
		}
		r := Ack(s.fetcher, tok, s.object, s.server)
		if ok, _, why := bank.Redeem(ks, 0, tok, r); !ok {
			t.Fatalf("setup: spend %d refused: %s", i, why)
		}
	}
	if got := len(bank.spent); got != n {
		t.Fatalf("setup: the spent set holds %d, want %d", got, n)
	}

	// One redeem attempt at an epoch past 0 + W. It does not matter whether it credits;
	// the ONLY thing under test is that the band advance retires the epoch-0 entries.
	serial, _ := blindtoken.NewSerial(rand.Reader)
	future := DefaultWindow + 1
	blinded, secret, _ := Withdraw(rand.Reader, &key.PublicKey, future, serial)
	tok, uerr := Unblind(&key.PublicKey, future, serial, SignWithdrawal(rand.Reader, key, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	bank.Redeem(ks, future, tok, Ack(s.fetcher, tok, s.object, s.server))

	for k, e := range bank.spent {
		if e == 0 {
			t.Fatalf("BREAK C-7: an epoch-0 spent entry (%x…) survived the advance to epoch "+
				"%d with the set at %d/%d — far below the cap, so nothing will ever sweep "+
				"it. The entry's justification is a %d-epoch window; it must not outlive it.",
				k[:8], future, len(bank.spent), maxSpentTokens, DefaultWindow)
		}
	}
	t.Logf("after the advance: %d live spent entries", len(bank.spent))
}
