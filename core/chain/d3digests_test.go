package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// G-D3-1 .. G-D3-7 — the (d-3) two-level v5 block hash.
//
// G-D3-7 is the one that matters: it drives the PURCHASE. The rest guard the transitive coverage
// that makes the purchase sound — on v5 the preimage folds digests, so Answer and Slashes are
// bound by validateD3Digests and by nothing else. A weak rule here leaves them unbound on v5,
// which is strictly worse than before (d-3).
//
// ABLATION DISCIPLINE: every arm below was run against a deliberately broken validateD3Digests
// before being trusted. See TestGD3_AblationRecord for what was removed and what went red.

func v5RegBlock(t *testing.T, k ed25519.PrivateKey, withAnswer bool) *Block {
	t.Helper()
	r := bondReg(k, twoMiB, ports.Hash{})
	if !withAnswer {
		r.Answer = nil
	}
	b := &Block{Version: BlockVersionWitnessable, Height: 9, Prev: ports.HashBytes([]byte("p")),
		Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k), BondRegs: []BondReg{r}}
	setD3Digests(b)
	return b
}

// G-D3-1: a v5 registration whose AnswerDigest does not equal sha256(Answer) is REFUSED.
func TestGD3_1_WrongAnswerDigestIsRefused(t *testing.T) {
	b := v5RegBlock(t, key(9101), true)
	if err := validateD3Digests(b); err != nil {
		t.Fatalf("precondition: an honestly populated block must pass, got %v", err)
	}
	bad := ports.HashBytes([]byte("not the answer"))
	b.BondRegs[0].AnswerDigest = &bad
	if err := validateD3Digests(b); !errors.Is(err, ErrD3DigestMismatch) {
		t.Fatalf("G-D3-1: a forged AnswerDigest must be refused with ErrD3DigestMismatch, got %v", err)
	}
}

// G-D3-2: a v5 registration carrying a proof but NO digest is REFUSED. Without this arm a
// proposer could omit the digest and leave Answer bound by nothing on v5.
func TestGD3_2_MissingAnswerDigestIsRefused(t *testing.T) {
	b := v5RegBlock(t, key(9102), true)
	b.BondRegs[0].AnswerDigest = nil
	if err := validateD3Digests(b); !errors.Is(err, ErrD3DigestMissing) {
		t.Fatalf("G-D3-2: a v5 registration with no AnswerDigest must be refused, got %v", err)
	}
}

// G-D3-3/4/5: Slashes and SlashesDigest must be present together and consistent.
func TestGD3_345_SlashesDigestConsistency(t *testing.T) {
	k := key(9103)
	b := v5RegBlock(t, k, true)
	b.Slashes = []Equivocation{slashProof(key(9104), b.Prev, 0x41, 0x42)}
	setD3Digests(b)
	if err := validateD3Digests(b); err != nil {
		t.Fatalf("precondition: honestly populated slashes must pass, got %v", err)
	}

	t.Run("G-D3-3 slashes with no digest", func(t *testing.T) {
		c := *b
		c.SlashesDigest = nil
		if err := validateD3Digests(&c); !errors.Is(err, ErrD3DigestMissing) {
			t.Fatalf("slashes present with no digest must be refused, got %v", err)
		}
	})
	t.Run("G-D3-4 wrong digest", func(t *testing.T) {
		c := *b
		bad := ports.HashBytes([]byte("wrong"))
		c.SlashesDigest = &bad
		if err := validateD3Digests(&c); !errors.Is(err, ErrD3DigestMismatch) {
			t.Fatalf("a forged SlashesDigest must be refused, got %v", err)
		}
	})
	t.Run("G-D3-5 digest with no slashes", func(t *testing.T) {
		c := *b
		c.Slashes = nil
		if err := validateD3Digests(&c); !errors.Is(err, ErrD3DigestMismatch) {
			t.Fatalf("a digest of nothing must be refused, got %v", err)
		}
	})
}

// G-D3-6: a PRE-v5 block may carry NEITHER digest. This is what keeps the frozen-format story
// honest — a v2/v4 block whose bytes carry a v5-only field would hash differently from the same
// block written by a pre-(d-3) binary.
func TestGD3_6_PreV5BlockMayNotCarryDigests(t *testing.T) {
	k := key(9105)
	for _, ver := range []uint64{BlockVersionRounds, BlockVersionStateRoot} {
		b := &Block{Version: ver, Height: 4, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k),
			BondRegs: []BondReg{bondReg(k, twoMiB, ports.Hash{})}}
		if err := validateD3Digests(b); err != nil {
			t.Fatalf("v%d: a clean pre-v5 block must pass, got %v", ver, err)
		}
		d := ports.HashBytes([]byte("x"))
		b.BondRegs[0].AnswerDigest = &d
		if err := validateD3Digests(b); !errors.Is(err, ErrD3DigestPreV5) {
			t.Fatalf("v%d: a pre-v5 block carrying AnswerDigest must be refused, got %v", ver, err)
		}
		b.BondRegs[0].AnswerDigest = nil
		b.SlashesDigest = &d
		if err := validateD3Digests(b); !errors.Is(err, ErrD3DigestPreV5) {
			t.Fatalf("v%d: a pre-v5 block carrying SlashesDigest must be refused, got %v", ver, err)
		}
	}
}

// G-D3-7 — THE PURCHASE, driven. A pruned v5 block reproduces its OWN hash from what it retains,
// carries no `Pruned` token, and its retained body is still hash-covered.
//
// The defect this closes, from Block.Hash's own comment: on pre-v5, Hash() short-circuits to the
// declared b.Pruned, so once pruned NONE of LastCommit/StateRoot/Entries/Revocations/Slashes is
// covered — "the attack is not forging Pruned, it is KEEPING Pruned and the real signatures while
// mutating the body". The v2 arm below DEMONSTRATES that defect rather than describing it.
func TestGD3_7_PrunedV5BlockIsSelfCovering(t *testing.T) {
	k := key(9106)
	b := v5RegBlock(t, k, true)
	b.Revocations = []ports.Hash{ports.HashBytes([]byte("rev"))}
	setD3Digests(b)
	Sign(b, k)
	full := b.Hash()

	p := b.Prune()
	if p.Pruned != (ports.Hash{}) {
		t.Fatalf("G-D3-7: a pruned v5 block must carry NO Pruned token — it is retired for v5, got %x", p.Pruned[:8])
	}
	if p.IsPruned() {
		t.Fatal("G-D3-7: a pruned v5 block must not report IsPruned — its identity is recomputed, not declared")
	}
	if p.BondRegs[0].Answer != nil {
		t.Fatal("G-D3-7: the heavy Answer must still be dropped")
	}
	if p.BondRegs[0].AnswerDigest == nil {
		t.Fatal("G-D3-7: AnswerDigest must SURVIVE the prune — it is what reproduces the hash")
	}
	if got := p.Hash(); got != full {
		t.Fatalf("G-D3-7: a PRUNED v5 block must recompute its own hash.\n  got  %x\n  want %x\n"+
			"This is the entire (d-3) purchase: the retained body is self-covering, so `Pruned` — a "+
			"DECLARED identity nothing recomputes — is retired.", got, full)
	}

	// The second half of self-covering: the retained body is still BOUND. Mutating it must move
	// the hash, which is exactly what a pruned pre-v5 block does NOT do.
	m := p
	m.Revocations = []ports.Hash{ports.HashBytes([]byte("rewritten"))}
	m.hashMemoSet = false // the struct copy carried the cached hash; recompute or this proves nothing
	if m.Hash() == full {
		t.Fatal("G-D3-7: mutating a pruned v5 block's retained Revocations did NOT move its hash — " +
			"the retained body is not covered, which is the defect this change exists to close")
	}

	// And the contrast, DRIVEN not asserted: the same rewrite on a pruned v2 block is invisible.
	v2 := &Block{Version: BlockVersionRounds, Height: 9, Prev: ports.HashBytes([]byte("p")),
		Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k),
		BondRegs:    []BondReg{bondReg(k, twoMiB, ports.Hash{})},
		Revocations: []ports.Hash{ports.HashBytes([]byte("rev"))}}
	Sign(v2, k)
	pv2 := v2.Prune()
	if pv2.Pruned == (ports.Hash{}) {
		t.Fatal("precondition: a pruned v2 block must carry the declared Pruned token")
	}
	before := pv2.Hash()
	pv2.Revocations = []ports.Hash{ports.HashBytes([]byte("rewritten"))}
	pv2.hashMemoSet = false
	if pv2.Hash() != before {
		t.Fatal("precondition: on v2 a pruned block's retained body is NOT hash-covered; if this " +
			"now moves, the pre-v5 short-circuit changed and this contrast must be re-derived")
	}
}

// TestGD3_AblationRecord documents the ablations run before these gates were trusted. It asserts
// nothing at runtime; it exists so the evidence is not lost when the session's shell history is.
//
// SOURCE GATE: a written record, not a check. RUNTIME GATE: the arms above.
func TestGD3_AblationRecord(t *testing.T) {
	t.Log("ablations run 2026-09-10, each verified by EXIT CODE, arm restored after, no residue:")
	t.Log("  A1 disable the AnswerDigest mismatch arm            -> G-D3-1   RED")
	t.Log("  A2 disable the missing-AnswerDigest arm             -> G-D3-2   RED")
	t.Log("  A3 disable the digest-with-no-slashes arm           -> G-D3-3/5 RED")
	t.Log("  A4 disable the SlashesDigest mismatch arm           -> G-D3-4   RED")
	t.Log("  A5 delete the pre-v5 SlashesDigest refusal          -> G-D3-6   RED")
	t.Log("  A6 set out.Pruned unconditionally (pre-(d-3) Prune) -> G-D3-7   RED")
	t.Log("")
	t.Log("A5 IS RECORDED THE WAY IT IS ON PURPOSE. Its first run reported GREEN, which would have")
	t.Log("read as 'this gate is blind'. It was neither: the patch anchor matched TWO sites")
	t.Log("(validateD3Digests and setD3Digests share the version guard), so the edit never applied")
	t.Log("and the ablation never ran. A false GREEN from a patch that silently no-ops looks exactly")
	t.Log("like a passing ablation. Re-run against a unique anchor, it is RED. Verify that an")
	t.Log("ablation CHANGED THE SOURCE before believing its result.")
}
