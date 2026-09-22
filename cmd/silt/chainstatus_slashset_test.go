package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// Item 9's distinctive clause is the COMPLEMENT of "the attacker was slashed":
// across a whole run, no HONEST node appears in the slash set. A complement can
// only be asserted over a set, so the set has to be readable from committed
// state — the daemon's `chain: slashed equivocator` line says what one node
// DECIDED, not what the history COMMITTED, and the two differ exactly where the
// complement matters.
//
// This pins the reporting surface: the count, the per-identity lines, and the
// dedup rule. It is the gate the adversarial suites scrape.
func TestChainStatusReportsTheCommittedSlashSet(t *testing.T) {
	dir := t.TempDir()

	culpritKey, _, err := ed25519.GenerateKey(zeroReader{})
	if err != nil {
		t.Fatal(err)
	}
	otherKey := append([]byte(nil), culpritKey...)
	otherKey[0] ^= 0xff // a second, distinct identity

	// Two blocks the culprit double-signed. Their CONTENT is irrelevant here:
	// chain-status reports the committed set, it does not re-verify the proof
	// (VerifyEquivocation is the validity rule's job, and a block that reached
	// persisted state already passed it).
	sig := func(h uint64, tag string) chain.Block {
		return chain.Block{Version: 1, Height: h, Entries: []ports.Entry{{Root: ports.HashBytes([]byte(tag))}}}
	}
	ev := func(key []byte, h uint64) chain.Equivocation {
		return chain.Equivocation{Culprit: key, A: sig(h, "a"), B: sig(h, "b")}
	}

	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	b1 := chain.Block{Version: 1, Height: 1, Prev: g.Hash()}
	b2 := chain.Block{Version: 1, Height: 2, Prev: b1.Hash(), Slashes: []chain.Equivocation{ev(culpritKey, 1)}}
	// The SAME culprit slashed again at a later height on a second proof. The set
	// must still hold it once, at the height it was FIRST committed — otherwise a
	// complement assertion counting identities would count evidence instead.
	b3 := chain.Block{Version: 1, Height: 3, Prev: b2.Hash(), Slashes: []chain.Equivocation{ev(culpritKey, 2)}}
	// A second, distinct identity, so the test separates "one culprit" from
	// "one slash" and pins that EVERY identity is printed rather than a head.
	b4 := chain.Block{Version: 1, Height: 4, Prev: b3.Hash(), Slashes: []chain.Equivocation{ev(otherKey, 3)}}

	if err := chainstore.Save(filepath.Join(dir, "chain.cbor"), []chain.Block{g, b1, b2, b3, b4}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cmdChainStatus([]string{"-store", dir}); err != nil {
			t.Fatal(err)
		}
	})

	culpritID := ports.NodeID(sha256.Sum256(culpritKey))
	otherID := ports.NodeID(sha256.Sum256(otherKey))
	for _, want := range []string{
		"slashed:      2 identities in the committed slash set:",
		"slashed-id:   " + culpritID.String() + " (first committed at height 2)",
		"slashed-id:   " + otherID.String() + " (first committed at height 4)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("chain-status must report the committed slash set;\nwant line %q in:\n%s", want, out)
		}
	}
	// The dedup rule, asserted rather than implied: three slashes, two identities.
	if n := strings.Count(out, "slashed-id:"); n != 2 {
		t.Fatalf("the slash set is keyed by CULPRIT, not by evidence: three committed slashes over two identities must print two lines, got %d:\n%s", n, out)
	}
}

// An empty slash set is the case every honest run is in, and it must say so in
// words rather than printing nothing — a missing line and a zero are the same
// character to a scraper, and "no line" is how a complement assertion passes
// vacuously on a node whose chain never loaded.
func TestChainStatusNarratesAnEmptySlashSet(t *testing.T) {
	dir := t.TempDir()
	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	b1 := chain.Block{Version: 1, Height: 1, Prev: g.Hash()}
	if err := chainstore.Save(filepath.Join(dir, "chain.cbor"), []chain.Block{g, b1}); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := cmdChainStatus([]string{"-store", dir}); err != nil {
			t.Fatal(err)
		}
	})
	const want = "slashed:      0 identities in the committed slash set"
	if !strings.Contains(out, want) {
		t.Fatalf("an empty slash set must be narrated, not omitted;\nwant %q in:\n%s", want, out)
	}
	if strings.Contains(out, "slashed-id:") {
		t.Fatalf("an empty slash set must print no identity lines:\n%s", out)
	}
}

// zeroReader gives ed25519.GenerateKey a deterministic seed so the test's ids
// are stable across runs.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}
