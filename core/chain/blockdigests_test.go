package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The two-level v5 block hash.
//
// A pruned v5 block is self-covering: that is the property these gates exist for. The rest guard
// the transitive coverage that makes it sound — on v5 the preimage folds digests, so Answer and
// Slashes are bound by validateBlockDigests and by nothing else. A weak rule here leaves them
// unbound on v5, which is strictly worse than the pre-v5 hash.

func v5RegBlock(t *testing.T, k ed25519.PrivateKey, withAnswer bool) *Block {
	t.Helper()
	r := bondReg(k, twoMiB, ports.Hash{})
	if !withAnswer {
		r.Answer = nil
	}
	b := &Block{Version: BlockVersionWitnessable, Height: 9, Prev: ports.HashBytes([]byte("p")),
		Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k), BondRegs: []BondReg{r}}
	setBlockDigests(b)
	return b
}

// A v5 registration whose AnswerDigest does not equal sha256(Answer) is REFUSED.
func TestWrongAnswerDigestIsRefused(t *testing.T) {
	b := v5RegBlock(t, key(9101), true)
	if err := validateBlockDigests(b); err != nil {
		t.Fatalf("precondition: an honestly populated block must pass, got %v", err)
	}
	bad := ports.HashBytes([]byte("not the answer"))
	b.BondRegs[0].AnswerDigest = &bad
	if err := validateBlockDigests(b); !errors.Is(err, ErrBlockDigestMismatch) {
		t.Fatalf("a forged AnswerDigest must be refused with ErrBlockDigestMismatch, got %v", err)
	}
}

// A v5 registration carrying a proof but NO digest is REFUSED. Without this arm a
// proposer could omit the digest and leave Answer bound by nothing on v5.
func TestMissingAnswerDigestIsRefused(t *testing.T) {
	b := v5RegBlock(t, key(9102), true)
	b.BondRegs[0].AnswerDigest = nil
	if err := validateBlockDigests(b); !errors.Is(err, ErrBlockDigestMissing) {
		t.Fatalf("a v5 registration with no AnswerDigest must be refused, got %v", err)
	}
}

// Slashes and SlashesDigest must be present together and consistent.
func TestSlashesDigestConsistency(t *testing.T) {
	k := key(9103)
	b := v5RegBlock(t, k, true)
	b.Slashes = []Equivocation{slashProof(key(9104), b.Prev, 0x41, 0x42)}
	setBlockDigests(b)
	if err := validateBlockDigests(b); err != nil {
		t.Fatalf("precondition: honestly populated slashes must pass, got %v", err)
	}

	t.Run("slashes with no digest", func(t *testing.T) {
		c := *b
		c.SlashesDigest = nil
		if err := validateBlockDigests(&c); !errors.Is(err, ErrBlockDigestMissing) {
			t.Fatalf("slashes present with no digest must be refused, got %v", err)
		}
	})
	t.Run("wrong digest", func(t *testing.T) {
		c := *b
		bad := ports.HashBytes([]byte("wrong"))
		c.SlashesDigest = &bad
		if err := validateBlockDigests(&c); !errors.Is(err, ErrBlockDigestMismatch) {
			t.Fatalf("a forged SlashesDigest must be refused, got %v", err)
		}
	})
	t.Run("digest with no slashes", func(t *testing.T) {
		c := *b
		c.Slashes = nil
		if err := validateBlockDigests(&c); !errors.Is(err, ErrBlockDigestMismatch) {
			t.Fatalf("a digest of nothing must be refused, got %v", err)
		}
	})
}

// A PRE-v5 block may carry NEITHER digest. This is what keeps the frozen-format story honest —
// a v2/v4 block whose bytes carry a v5-only field would hash differently from the same block
// written by a pre- binary.
func TestPreV5BlockMayNotCarryDigests(t *testing.T) {
	k := key(9105)
	for _, ver := range []uint64{BlockVersionRounds, BlockVersionStateRoot} {
		b := &Block{Version: ver, Height: 4, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k),
			BondRegs: []BondReg{bondReg(k, twoMiB, ports.Hash{})}}
		if err := validateBlockDigests(b); err != nil {
			t.Fatalf("v%d: a clean pre-v5 block must pass, got %v", ver, err)
		}
		d := ports.HashBytes([]byte("x"))
		b.BondRegs[0].AnswerDigest = &d
		if err := validateBlockDigests(b); !errors.Is(err, ErrBlockDigestPreV5) {
			t.Fatalf("v%d: a pre-v5 block carrying AnswerDigest must be refused, got %v", ver, err)
		}
		b.BondRegs[0].AnswerDigest = nil
		b.SlashesDigest = &d
		if err := validateBlockDigests(b); !errors.Is(err, ErrBlockDigestPreV5) {
			t.Fatalf("v%d: a pre-v5 block carrying SlashesDigest must be refused, got %v", ver, err)
		}
	}
}

// THE PURCHASE, driven. A pruned v5 block reproduces its OWN hash from what it retains, carries
// no `Pruned` token, and its retained body is still hash-covered.
//
// The defect this closes, from Block.Hash's own comment: on pre-v5, Hash short-circuits to the
// declared b.Pruned, so once pruned NONE of LastCommit/StateRoot/Entries/Revocations/Slashes is
// covered — "the attack is not forging Pruned, it is KEEPING Pruned and the real signatures while
// mutating the body". The v2 arm below DEMONSTRATES that defect rather than describing it.
func TestPrunedV5BlockIsSelfCovering(t *testing.T) {
	k := key(9106)
	b := v5RegBlock(t, k, true)
	b.Revocations = []ports.Hash{ports.HashBytes([]byte("rev"))}
	setBlockDigests(b)
	Sign(b, k)
	full := b.Hash()

	p := b.Prune()
	if p.Pruned != (ports.Hash{}) {
		t.Fatalf("a pruned v5 block must carry NO Pruned token — it is retired for v5, got %x", p.Pruned[:8])
	}
	if p.IsPruned() {
		t.Fatal("a pruned v5 block must not report IsPruned — its identity is recomputed, not declared")
	}
	if p.BondRegs[0].Answer != nil {
		t.Fatal("the heavy Answer must still be dropped")
	}
	if p.BondRegs[0].AnswerDigest == nil {
		t.Fatal("AnswerDigest must SURVIVE the prune — it is what reproduces the hash")
	}
	if got := p.Hash(); got != full {
		t.Fatalf("a PRUNED v5 block must recompute its own hash.\n got  %x\n want %x\n"+
			"This is the entire purchase: the retained body is self-covering, so `Pruned` — a "+
			"DECLARED identity nothing recomputes — is retired.", got, full)
	}

	// The second half of self-covering: the retained body is still BOUND. Mutating it must move
	// the hash, which is exactly what a pruned pre-v5 block does NOT do.
	m := p
	m.Revocations = []ports.Hash{ports.HashBytes([]byte("rewritten"))}
	m.hashMemoSet = false // the struct copy carried the cached hash; recompute or this proves nothing
	if m.Hash() == full {
		t.Fatal("mutating a pruned v5 block's retained Revocations did NOT move its hash — " +
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
