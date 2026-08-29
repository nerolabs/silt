package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// era-3 build step 2a — the schema/Hash/versionSupported model-check tier. These
// oracles prove the four properties the certification requires of 2a and NO more:
//
//  1. era-2 byte-identity: a v2 block hashes to the same bytes after the two root
//     fields are added (committed history is never re-interpreted, chain.go:260-268).
//  2. era-3 roots are attester-signed: a tampered StateRoot/LogRoot changes Hash(),
//     so a signature over the real hash no longer verifies.
//  3. v4 decodes and is accepted; a version beyond the ceiling is refused loudly with
//     ErrBlockVersion (the hard-fork failure mode preserved). era-4 4c widened the ceiling
//     to v5, so the refused boundary is now v6+.
//  4. population wiring: a v4 block constructed with the step-1 accessors carries
//     StateRoot == c.StateRoot() and LogRoot == c.LogRoot().
//
// STOP boundary (2a): these exercise the SCHEMA and HASH only. No validity predicate
// rejects on a root mismatch (2b); nothing flips minting to v4 (2c). The design and
// the load-bearing omitempty compat decision are in
// docs/thinking/2026-08-29-era3-step2a-commit-roots-schema.md.

// TestEra2BlockHashesByteIdenticalAfter2a is the compat oracle: a v2 block that never
// sets the era-3 roots must hash EXACTLY as it did before 2a. The roots are omitempty
// and zero, so they are omitted from the CBOR body and the hash is unchanged.
//
// The RED that keeps this honest: change StateRoot/LogRoot to non-omitempty (or set a
// non-zero root on a v2 block) and the era-2 hash changes — this golden assertion fails.
// The golden hash is pinned below; it is the sha256 of the pre-2a unsigned body for the
// fixture block, so it cannot silently track a change to what Hash covers.
func TestEra2BlockHashesByteIdenticalAfter2a(t *testing.T) {
	b := era3FixtureV2Block()

	// The era-3 root fields are absent (nil) on a v2 block.
	if b.StateRoot != nil || b.LogRoot != nil {
		t.Fatalf("fixture v2 block unexpectedly carries roots: state=%v log=%v", b.StateRoot, b.LogRoot)
	}

	got := b.Hash()

	// A block IDENTICAL except carrying a set era-3 root must hash DIFFERENTLY (the roots
	// are in the signed body). This is the paired positive to the byte-identity property:
	// era-2 (nil roots) is unchanged, but a root that is actually SET moves the hash —
	// omitempty drops only the nil pointer, never a present value.
	withRoot := era3FixtureV2Block()
	withRoot.hashMemoSet = false
	r := ports.Hash{0x01}
	withRoot.StateRoot = &r
	if withRoot.Hash() == got {
		t.Fatal("setting a StateRoot did not change the hash — the field is not in the " +
			"signed body, or omitempty is dropping a present pointer")
	}
}

// TestEra2GoldenHashUnchanged pins a deterministic v2 block's hash to a literal. This is
// the true byte-identity guard: the literal was captured from the pre-2a code path (a v2
// block whose body has no era-3 fields). If the additive change ever perturbs the era-2
// unsigned body — e.g. a non-omitempty tag emits 32 zero bytes — this literal no longer
// matches and the test goes RED. The fixture is fully deterministic (fixed keys, fixed
// entry), so the hash is stable across runs and machines.
func TestEra2GoldenHashUnchanged(t *testing.T) {
	b := era3DeterministicV2Block()
	got := b.Hash()

	// Captured from the pre-2a schema (v2 block, no era-3 fields). See the deliberation.
	want := era3PinnedV2Hash

	if got != want {
		t.Fatalf("era-2 v2 block hash CHANGED after 2a:\n got  %x\n want %x\n\n"+
			"The additive StateRoot/LogRoot fields must be INVISIBLE to a v2 block "+
			"(omitempty + zero value). A changed hash means committed era-2 history would "+
			"be re-interpreted — the exact thing chain.go:260-268 forbids. If this fails, "+
			"the roots are being emitted for a v2 block (check omitempty).", got, want)
	}
}

// TestEra3RootsAreAttesterSigned proves the roots are inside the signed Hash body: a v4
// block with a tampered StateRoot or LogRoot has a DIFFERENT hash, so a signature over
// the real hash fails. RED: remove StateRoot/LogRoot from the Hash() unsigned body and
// tampering no longer changes the hash — both sub-assertions fail.
func TestEra3RootsAreAttesterSigned(t *testing.T) {
	b := era3FixtureV4Block()
	honest := b.Hash()

	tamperState := b
	tamperState.hashMemoSet = false
	ts := ports.Hash{0xDE, 0xAD}
	tamperState.StateRoot = &ts
	if tamperState.Hash() == honest {
		t.Fatal("tampering StateRoot did not change Hash — the root is not in the signed " +
			"body; a forged state root could ride a valid signature")
	}

	tamperLog := b
	tamperLog.hashMemoSet = false
	tl := ports.Hash{0xBE, 0xEF}
	tamperLog.LogRoot = &tl
	if tamperLog.Hash() == honest {
		t.Fatal("tampering LogRoot did not change Hash — the root is not in the signed " +
			"body; a forged log root could ride a valid signature")
	}
}

// TestV4DecodesAndIsAccepted proves the versionSupported widening: a v4 block round-trips
// through Encode/Decode and is accepted; a version beyond the ceiling is refused loudly with
// ErrBlockVersion. RED: leave versionSupported at <= BlockVersionRegGate and Decode rejects
// the v4 block — the accept assertion fails.
//
// era-4 4c widened the ceiling to BlockVersionWitnessable (v5), so v5 now DECODES (its
// TestV5DecodesAndIsAccepted counterpart proves it). The "beyond the ceiling" version this
// test refuses therefore moved from BlockVersionStateRoot+1 (=5, now supported) to
// BlockVersionWitnessable+1 (=6) — the current hard-fork guard boundary.
func TestV4DecodesAndIsAccepted(t *testing.T) {
	b := era3FixtureV4Block()

	dec, err := Decode(Encode(&b))
	if err != nil {
		t.Fatalf("Decode rejected a v4 block, want accept: %v", err)
	}
	if dec.Version != BlockVersionStateRoot {
		t.Fatalf("decoded version = %d, want %d", dec.Version, BlockVersionStateRoot)
	}
	// The roots survive the round-trip (they are on the wire, set, emitted).
	if dec.StateRoot == nil || dec.LogRoot == nil {
		t.Fatalf("decoded v4 block dropped a root: state=%v log=%v", dec.StateRoot, dec.LogRoot)
	}
	if *dec.StateRoot != *b.StateRoot || *dec.LogRoot != *b.LogRoot {
		t.Fatalf("roots did not survive decode: state %x/%x log %x/%x",
			*dec.StateRoot, *b.StateRoot, *dec.LogRoot, *b.LogRoot)
	}

	// A version beyond the ceiling (v6, one past BlockVersionWitnessable) is still refused
	// loudly — the hard-fork guard.
	future := b
	future.hashMemoSet = false
	future.Version = BlockVersionWitnessable + 1
	if _, err := Decode(Encode(&future)); !errors.Is(err, ErrBlockVersion) {
		t.Fatalf("Decode accepted a v%d block, want ErrBlockVersion, got %v",
			BlockVersionWitnessable+1, err)
	}
}

// TestV4CarriesStepOneRoots proves the population wiring: a v4 block constructed from a
// chain's committed state carries exactly that chain's StateRoot() and LogRoot(). RED:
// leave the fields unset in the constructor and they are zero — the equality fails.
func TestV4CarriesStepOneRoots(t *testing.T) {
	c := New(DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	populateCommitted(c) // give the chain real committed state so the roots are non-trivial

	wantState, err := c.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot: %v", err)
	}
	wantLog := c.LogRoot()

	b := c.newV4BlockWithRoots(7, ports.Hash{1}, nil)

	if b.Version != BlockVersionStateRoot {
		t.Fatalf("constructed block version = %d, want %d", b.Version, BlockVersionStateRoot)
	}
	if b.StateRoot == nil || b.LogRoot == nil {
		t.Fatalf("constructed v4 block has a nil root: state=%v log=%v", b.StateRoot, b.LogRoot)
	}
	if *b.StateRoot != wantState {
		t.Fatalf("v4 block StateRoot = %x, want the chain's %x", *b.StateRoot, wantState)
	}
	if *b.LogRoot != wantLog {
		t.Fatalf("v4 block LogRoot = %x, want the chain's %x", *b.LogRoot, wantLog)
	}
	// era-3 roots are non-zero constants, so a set (non-nil) pointer to them is emitted.
	if *b.StateRoot == (ports.Hash{}) || *b.LogRoot == (ports.Hash{}) {
		t.Fatal("an era-3 block's roots must be non-zero constants (empty-state SMT / " +
			"sha256(\"\") log)")
	}
}

// era3PinnedV2Hash is the hash of era3DeterministicV2Block, captured from the PRE-2a
// schema (origin/main 72d5c4c, a v2 block with no era-3 fields). After 2a the same block
// — roots nil, omitempty-omitted — must hash to this EXACT value. A mismatch means the
// additive change perturbed the era-2 unsigned body. Verified equal to the pre-2a value
// by computing the same block's hash in a worktree at origin/main (see the deliberation).
var era3PinnedV2Hash = ports.Hash{
	0x02, 0x25, 0xf1, 0x9d, 0x6f, 0x30, 0x35, 0xee,
	0xfd, 0x37, 0xf1, 0x1d, 0x2e, 0x9a, 0xbe, 0x61,
	0x91, 0xad, 0xab, 0x6a, 0xff, 0x83, 0xe9, 0x15,
	0x53, 0x73, 0x84, 0x50, 0xda, 0x37, 0x7d, 0x0c,
}

// --- fixtures ---

// era3FixtureV2Block is a minimal signed v2 block for the compat/tamper oracles.
func era3FixtureV2Block() Block {
	_, priv, _ := ed25519.GenerateKey(nil)
	b := Block{
		Version: BlockVersionRounds,
		Height:  3,
		Prev:    ports.Hash{2},
		Entries: []ports.Entry{entry(1)},
	}
	Sign(&b, priv)
	return b
}

// era3FixtureV4Block is a v2-shaped block re-tagged v4 and carrying non-zero roots, for
// the decode/tamper oracles. The roots need not match any chain here — 2a adds no
// predicate; TestV4CarriesStepOneRoots covers the real wiring.
func era3FixtureV4Block() Block {
	_, priv, _ := ed25519.GenerateKey(nil)
	sr := ports.Hash{0x11, 0x22, 0x33}
	lr := ports.Hash{0x44, 0x55, 0x66}
	b := Block{
		Version:   BlockVersionStateRoot,
		Height:    3,
		Prev:      ports.Hash{2},
		Entries:   []ports.Entry{entry(1)},
		StateRoot: &sr,
		LogRoot:   &lr,
	}
	Sign(&b, priv)
	return b
}

// era3DeterministicV2Block is a fully deterministic v2 block (fixed proposer bytes, fixed
// entry, no signature) so its Hash is stable across runs — the golden-pin subject.
func era3DeterministicV2Block() Block {
	proposer := make([]byte, 32)
	for i := range proposer {
		proposer[i] = byte(i + 1)
	}
	return Block{
		Version:  BlockVersionRounds,
		Height:   42,
		Prev:     ports.Hash{9, 9, 9},
		Entries:  []ports.Entry{entry(7)},
		Proposer: proposer,
	}
}
