package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// commitV4 appends a v4 block carrying the honest post-apply roots, attested by the
// era3ValidityChain quorum.
func commitV4(t *testing.T, c *Chain, prop ed25519.PrivateKey, b *Block) {
	t.Helper()
	prev, next := c.Head()
	b.Version, b.Height, b.Prev = BlockVersionStateRoot, next, prev
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, prop)
	for _, k := range []ed25519.PrivateKey{prop, key(30202), key(30203), key(30204)} {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare, c.ChainID()))
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit, c.ChainID()))
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit v4 block %d: %v", b.Height, err)
	}
}

// An honoured removal's receipt verifies against nothing but the anchoring header,
// proves the log only grew since the operator's previous receipt, and fails on every
// field a forger would change.
func TestRevocationReceiptProvesInclusionAndConsistency(t *testing.T) {
	c, prop := era3ValidityChain(t) // height 2 revokes entry(1) in a v1 block: no LogRoot

	if _, err := c.RevocationReceiptFor(Listing{Root: entry(1).Root}); !errors.Is(err, ErrNoLogRoot) {
		t.Fatalf("a revocation no block's LogRoot commits yet has no receipt, got %v", err)
	}
	if _, err := c.RevocationReceiptFor(Listing{Root: entry(9).Root}); !errors.Is(err, ErrNotRevoked) {
		t.Fatalf("an unrevoked root has no receipt, got %v", err)
	}

	commitV4(t, c, prop, &Block{Entries: []ports.Entry{entry(9)}}) // height 3
	first, err := c.RevocationReceiptFor(Listing{Root: entry(1).Root})
	if err != nil {
		t.Fatal(err)
	}
	if first.Height != 2 || first.At != 3 || first.Index != 0 || first.Size != 1 {
		t.Fatalf("the v1 revocation must anchor at the first block that commits a LogRoot: %+v", first)
	}
	if err := VerifyRevocationReceipt(first, c.BlockAt(3)); err != nil {
		t.Fatalf("an honest receipt failed: %v", err)
	}

	commitV4(t, c, prop, &Block{Revocations: []ports.Hash{entry(9).Root}}) // height 4
	second, err := c.RevocationReceiptFor(Listing{Root: entry(9).Root})
	if err != nil {
		t.Fatal(err)
	}
	if second.Height != 4 || second.At != 4 || second.Index != 1 || second.Size != 2 {
		t.Fatalf("a v4 revocation anchors at its own block: %+v", second)
	}
	if err := c.ExtendFrom(&second, first.Size); err != nil {
		t.Fatal(err)
	}
	if second.PrevLogRoot != first.LogRoot {
		t.Fatalf("the checkpoint must be the earlier receipt's log root: %s vs %s", second.PrevLogRoot, first.LogRoot)
	}
	if err := VerifyRevocationReceipt(second, c.BlockAt(4)); err != nil {
		t.Fatalf("an honest extended receipt failed: %v", err)
	}
	back, err := DecodeReceipt(EncodeReceipt(second))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRevocationReceipt(back, c.BlockAt(4)); err != nil {
		t.Fatalf("the wire form does not round-trip: %v", err)
	}

	forge := []struct {
		name string
		edit func(r *RevocationReceipt)
		at   uint64
	}{
		{"another root", func(r *RevocationReceipt) { r.Root = entry(0).Root }, 4},
		{"another revoking height", func(r *RevocationReceipt) { r.Height = 3 }, 4},
		{"another leaf", func(r *RevocationReceipt) { r.Index = 0 }, 4},
		{"a shorter log", func(r *RevocationReceipt) { r.Size = 1 }, 4},
		{"another log root", func(r *RevocationReceipt) { r.LogRoot = first.LogRoot }, 4},
		{"another block", func(r *RevocationReceipt) {}, 3},
		{"a rewritten checkpoint", func(r *RevocationReceipt) { r.PrevLogRoot = ports.HashBytes([]byte("rewritten past")) }, 4},
		{"a checkpoint past the log", func(r *RevocationReceipt) { r.PrevSize = 3 }, 4},
		{"a dropped proof step", func(r *RevocationReceipt) { r.Inclusion = r.Inclusion[:len(r.Inclusion)-1] }, 4},
	}
	for _, f := range forge {
		r := second
		r.Inclusion = append([]ports.Hash(nil), second.Inclusion...)
		f.edit(&r)
		if err := VerifyRevocationReceipt(r, c.BlockAt(f.at)); err == nil {
			t.Errorf("a receipt forged with %s verified", f.name)
		}
	}
}

// An auditor holding only a replica's blocks derives the same receipt the chain
// does, and a replica whose blocks were edited after commit cannot prove its record.
func TestReceiptFromBlocksMatchesTheChainAndCatchesAnEditedReplica(t *testing.T) {
	c, prop := era3ValidityChain(t)
	commitV4(t, c, prop, &Block{Entries: []ports.Entry{entry(9)}})
	commitV4(t, c, prop, &Block{Revocations: []ports.Hash{entry(9).Root}})

	want, err := c.RevocationReceiptFor(Listing{Root: entry(9).Root})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ExtendFrom(&want, 1); err != nil {
		t.Fatal(err)
	}
	blocks := c.Blocks(0)
	got, err := ReceiptFromBlocks(blocks, Listing{Root: entry(9).Root}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(EncodeReceipt(got)) != string(EncodeReceipt(want)) {
		t.Fatalf("the auditor's receipt differs from the chain's:\n got %+v\nwant %+v", got, want)
	}
	if _, err := ReceiptFromBlocks(blocks, Listing{Root: entry(0).Root}, 0); !errors.Is(err, ErrNotRevoked) {
		t.Fatalf("an unrevoked root has no receipt, got %v", err)
	}

	// Edit the replica after the fact: drop the first takedown from its block.
	edited := append([]Block(nil), blocks...)
	edited[2].Revocations = nil
	if _, err := ReceiptFromBlocks(edited, Listing{Root: entry(9).Root}, 0); err == nil {
		t.Fatal("a replica with a takedown erased from its history still proved its record")
	}
}
