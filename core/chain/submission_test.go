package chain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	mrand "math/rand"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// A submitted entry is refused exactly when a single-entry block proposing it
// would be: the same verdict, for the same reason, across every entry-level rule
// and an unqualified proposer.
func TestValidateSubmissionRefusesWhatAProposalWould(t *testing.T) {
	prop, weak := key(1), key(9)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4)}
	issuers := map[ports.NodeID]*rsa.PrivateKey{}
	reps := map[ports.NodeID]int64{idOf(prop): 1000, idOf(weak): 1}
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

	rng := mrand.New(mrand.NewSource(7))
	mint := func(signers []ed25519.PrivateKey) *ports.PublishToken {
		serial, _ := blindtoken.NewSerial(rng)
		tok := &ports.PublishToken{Serial: serial}
		for _, v := range signers {
			iss := blindtoken.NewIssuer(rand.Reader, issuers[idOf(v)])
			blinded, secret, err := blindtoken.Blind(rng, iss.Public(), serial)
			if err != nil {
				t.Fatal(err)
			}
			blindSig, _ := iss.Issue(func() error { return nil }, blinded)
			sig, err := blindtoken.Unblind(iss.Public(), serial, blindSig, secret)
			if err != nil {
				t.Fatal(err)
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
	block := func(by ed25519.PrivateKey, e ports.Entry) *Block {
		prev, height := c.Head()
		b := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{e}}
		Sign(b, by)
		return b
	}

	committed := withToken(1, mint(vals[:2]))
	b := block(prop, committed)
	b.Atts = append(b.Atts, Attest(b, vals[0]), Attest(b, vals[1]))
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	noManifest := withToken(5, mint(vals[:2]))
	noManifest.ManifestChunks = nil
	withPublisher := withToken(6, mint(vals[:2]))
	withPublisher.Publisher = idOf(prop)

	cases := []struct {
		name string
		by   ed25519.PrivateKey
		e    ports.Entry
		want error // nil: accepted; errAny: refused for a reason with no sentinel
	}{
		{"valid", prop, withToken(2, mint(vals[:2])), nil},
		{"no token", prop, entry(3), ErrTokenRequired},
		{"committed root", prop, withToken(1, mint(vals[:2])), ErrDupRoot},
		{"spent serial", prop, withToken(4, committed.Token), ErrTokenSpent},
		{"short token quorum", prop, withToken(7, mint(vals[:1])), errAny},
		{"no manifest", prop, noManifest, errAny},
		{"durable publisher", prop, withPublisher, ErrPublisherEntry},
		{"unqualified proposer", weak, withToken(8, mint(vals[:2])), ErrLowReputation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := c.ValidateProposal(block(tc.by, tc.e))
			got := c.ValidateSubmission(tc.by.Public().(ed25519.PublicKey), tc.e)
			switch {
			case tc.want == nil:
				if full != nil || got != nil {
					t.Fatalf("want accepted: proposal %v, submission %v", full, got)
				}
			case tc.want == errAny:
				if full == nil || got == nil || full.Error() != got.Error() {
					t.Fatalf("want the same refusal: proposal %v, submission %v", full, got)
				}
			default:
				if !errors.Is(full, tc.want) || !errors.Is(got, tc.want) || full.Error() != got.Error() {
					t.Fatalf("want the same %v from both: proposal %v, submission %v", tc.want, full, got)
				}
			}
		})
	}
}

var errAny = errors.New("any refusal")

// The pre-check runs once per submitted publish on the validator's single event
// loop, so it must not copy or rehash committed state: its work is the same on an
// empty chain as on one holding thousands of entries.
func TestValidateSubmissionCostIsFlatInChainSize(t *testing.T) {
	prop, att := key(1), []ed25519.PrivateKey{key(2), key(3)}
	reps := map[ports.NodeID]int64{idOf(prop): 1000, idOf(att[0]): 1000, idOf(att[1]): 1000}
	c := New(Config{MinProposerRep: 100, MinAttesterRep: 100, Quorum: 2},
		func(n ports.NodeID) int64 { return reps[n] })

	nth := func(i int) ports.Entry {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(i))
		return ports.Entry{Root: ports.HashBytes(b[:]),
			ManifestChunks: []ports.ChunkID{ports.HashBytes(append(b[:], 1))}, FileSize: 1}
	}
	probe := nth(-1)
	pub := prop.Public().(ed25519.PublicKey)
	allocs := func() float64 {
		return testing.AllocsPerRun(50, func() {
			if err := c.ValidateSubmission(pub, probe); err != nil {
				t.Fatal(err)
			}
		})
	}
	empty := allocs()

	const blocks, perBlock = 50, 40
	for h := 0; h < blocks; h++ {
		prev, height := c.Head()
		b := &Block{Version: 1, Height: height, Prev: prev}
		for i := 0; i < perBlock; i++ {
			b.Entries = append(b.Entries, nth(h*perBlock+i))
		}
		Sign(b, prop)
		b.Atts = append(b.Atts, Attest(b, att[0]), Attest(b, att[1]))
		if err := c.Append(*b); err != nil {
			t.Fatalf("append %d: %v", h, err)
		}
	}
	if full := allocs(); full != empty {
		t.Fatalf("pre-check allocates %.0f times with %d entries committed, %.0f on an empty chain",
			full, blocks*perBlock, empty)
	}
}
