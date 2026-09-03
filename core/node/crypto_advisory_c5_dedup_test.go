package node

// Crypto-specialist advisory C-5 at the NODE tier, 2026-09-03: the issuance-dedup cache
// must not be bypassable by RE-ENCODING the blinded value.
//
// THE MECHANISM. demandDedupKey / the publish-token dedup are keyed on the RAW BLINDED
// BYTES. SignBlinded used to open with `b.Mod(b, N)`, so `blinded`, `blinded + N` and
// any zero-padded spelling all produced the SAME signature under DIFFERENT cache keys.
// The dedup exists because a lost reply makes the requester re-present the same blinded
// serial, and without it the issuer charges twice (research certification 2026-08-13,
// A2). A re-encoding therefore bought a second charge for one issuance.
//
// The close is RFC 8017 §5.2.2 plus a minimal-encoding requirement, applied at the
// signer: one representative has exactly one accepted spelling, so the raw bytes ARE a
// faithful cache key. See blindtoken.canonicalRep.

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

func TestC5_ReEncodedBlindCannotBypassTheIssuanceDedup(t *testing.T) {
	const fee = 100
	nd, ledger, key := newIssuerNode(t, fee)
	pub := &key.PublicKey
	durable := identity.FromSeed(4242).NodeID()
	ledger.Register(durable)
	start := ledger.Balance(durable)

	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, _, err := blindtoken.Blind(rand.Reader, pub, serial)
	if err != nil {
		t.Fatal(err)
	}

	first := nd.answerTokenRequest(durable, ports.Message{Data: blinded})
	if !first.OK {
		t.Fatal("setup: the honest issuance must succeed")
	}
	if charged := start - ledger.Balance(durable); charged != fee {
		t.Fatalf("setup: one issuance must charge the fee once, charged %d", charged)
	}
	afterFirst := ledger.Balance(durable)

	// The bypass attempts: two spellings of the SAME representative. Each is a distinct
	// dedup cache key, so if the signer accepts it the requester is charged again for a
	// signature it already has.
	for name, reencoded := range map[string][]byte{
		"blinded + N": new(big.Int).Add(new(big.Int).SetBytes(blinded), pub.N).Bytes(),
		"zero-padded": append([]byte{0}, blinded...),
	} {
		r := nd.answerTokenRequest(durable, ports.Message{Data: reencoded})
		if r.OK {
			t.Errorf("BREAK C-5: the issuer signed the %q spelling of an ALREADY-ISSUED "+
				"blinded value. demandDedupKey is keyed on the raw bytes, so this is a "+
				"fresh cache key for an issuance already settled — a second charge for "+
				"one signature.", name)
		}
		if got := afterFirst - ledger.Balance(durable); got != 0 {
			t.Errorf("BREAK C-5: the %q re-encoding charged the requester a further %d",
				name, got)
		}
	}

	// The honest retry still works and still charges nothing extra — the dedup must not
	// have been closed by making retries fail.
	retry := nd.answerTokenRequest(durable, ports.Message{Data: blinded})
	if !retry.OK || string(retry.Data) != string(first.Data) {
		t.Fatal("an honest retry must return the IDENTICAL blind signature (the A2 dedup)")
	}
	if got := afterFirst - ledger.Balance(durable); got != 0 {
		t.Fatalf("an honest retry charged %d — the dedup is broken", got)
	}
}
