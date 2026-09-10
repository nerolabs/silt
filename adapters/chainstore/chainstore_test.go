package chainstore

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/ports"
)

func testKey(seed int64) ed25519.PrivateKey {
	var b [ed25519.SeedSize]byte
	rand.New(rand.NewSource(seed)).Read(b[:])
	return ed25519.NewKeyFromSeed(b[:])
}

func idOf(priv ed25519.PrivateKey) ports.NodeID {
	return sha256.Sum256(priv.Public().(ed25519.PublicKey))
}

// committedChain builds a real [genesis, block1] history using a reputation
// view in which the proposer and attesters are fully qualified — i.e. the
// state a validator holds at the moment it commits and persists a block.
func committedChain(t *testing.T, rep func(ports.NodeID) int64) *chain.Chain {
	t.Helper()
	cfg := chain.DefaultConfig() // MinProposerRep/MinAttesterRep = 100, Quorum = 3
	c := chain.New(cfg, rep)

	gb, _, _, err := genesis.Build(memstore.New(), nil)
	if err != nil {
		t.Fatalf("genesis build: %v", err)
	}
	if err := c.AppendGenesis(gb); err != nil {
		t.Fatalf("append genesis: %v", err)
	}

	prop := testKey(1)
	prev, height := c.Head()
	b := &chain.Block{
		Version: 1, Height: height, Prev: prev,
		Entries: []ports.Entry{{
			Root:           ports.HashBytes([]byte{7}),
			ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte{7, 7})},
			FileSize:       700,
		}},
	}
	chain.Sign(b, prop)
	for i := int64(2); i <= 5; i++ { // four distinct attesters clear a quorum of 3
		b.Atts = append(b.Atts, chain.Attest(b, testKey(i)))
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit block 1: %v", err)
	}
	return c
}

// TestReplayReloadsOwnChainWithEmptyLedger is the F1 regression test: a
// restarted validator reloads its OWN persisted chain before any bond audit
// has re-established reputation, so its reputation view is empty. Reload must
// still rejoin at the persisted height (the "rejoin at its height, not from
// genesis" promise) — reputation is a live, time-varying view, NOT a
// corruption check, so re-gating our own committed history on it strands the
// node at genesis. Structural integrity (hashes + signatures) is still
// verified; only the reputation gate is skipped for our own disk.
func TestReplayReloadsOwnChainWithEmptyLedger(t *testing.T) {
	// Commit-time reputation: everyone the block needs is qualified.
	fullRep := map[ports.NodeID]int64{idOf(genesis.Key()): 0} // genesis earns none, by design
	for i := int64(1); i <= 5; i++ {
		fullRep[idOf(testKey(i))] = 1000
	}
	c := committedChain(t, func(n ports.NodeID) int64 { return fullRep[n] })
	if c.Len() != 2 {
		t.Fatalf("setup: want 2 blocks, got %d", c.Len())
	}

	path := filepath.Join(t.TempDir(), "chain.cbor")
	if err := Save(path, c.Blocks(0)); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Restart: a brand-new replica whose reputation view is EMPTY (bond audits
	// have not run yet). This is the exact daemon boot state.
	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	n, err := Replay(path, fresh)
	if err != nil {
		t.Fatalf("replay of own chain with empty ledger failed: %v", err)
	}
	if n != 2 || fresh.Len() != 2 {
		t.Fatalf("want 2 blocks reloaded, got n=%d len=%d", n, fresh.Len())
	}
	wantHash, wantHeight := c.Head()
	gotHash, gotHeight := fresh.Head()
	if wantHash != gotHash || wantHeight != gotHeight {
		t.Fatalf("reloaded head diverged: want %x@%d, got %x@%d", wantHash, wantHeight, gotHash, gotHeight)
	}
}

// TestReplayStillDetectsCorruption proves the reload does NOT blindly trust the
// disk: a tampered block (any content change breaks the proposer signature over
// the block hash) must still be rejected, even with the reputation gate skipped.
// This is B7's real intent — catch bit-rot/truncation/tampering, not re-litigate
// a policy decision the quorum already made.
func TestReplayStillDetectsCorruption(t *testing.T) {
	fullRep := map[ports.NodeID]int64{}
	for i := int64(1); i <= 5; i++ {
		fullRep[idOf(testKey(i))] = 1000
	}
	c := committedChain(t, func(n ports.NodeID) int64 { return fullRep[n] })
	blocks := c.Blocks(0)

	// Corrupt block 1's payload without re-signing: the proposer signature no
	// longer covers these bytes.
	blocks[1].Entries[0].FileSize = 999999

	path := filepath.Join(t.TempDir(), "chain.cbor")
	if err := Save(path, blocks); err != nil {
		t.Fatalf("save: %v", err)
	}
	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	if _, err := Replay(path, fresh); err == nil {
		t.Fatal("replay accepted a tampered block; corruption must be rejected")
	} else if !errors.Is(err, chain.ErrBadSignature) {
		t.Fatalf("want ErrBadSignature on tamper, got %v", err)
	}
}

// TestSaveLeavesNoTempAndDecodes pins the atomic-replace SHAPE (#558 / B8): a
// Save leaves exactly chain.cbor (no temp file) and it decodes. It is a shape
// pin, NOT a gate on the fsync — the pre-PR tmp+rename Save satisfies it too
// (PE ruling, B8); durability under a real power loss has no runtime oracle.
func TestSaveLeavesNoTempAndDecodes(t *testing.T) {
	fullRep := map[ports.NodeID]int64{}
	for i := int64(1); i <= 5; i++ {
		fullRep[idOf(testKey(i))] = 1000
	}
	c := committedChain(t, func(n ports.NodeID) int64 { return fullRep[n] })
	dir := t.TempDir()
	path := filepath.Join(dir, "chain.cbor")
	if err := Save(path, c.Blocks(0)); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "chain.cbor" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("Save must leave exactly chain.cbor, got %v", names)
	}
	got, err := Load(path)
	if err != nil || len(got) != 2 {
		t.Fatalf("load after save: n=%d err=%v", len(got), err)
	}
}

// TestRecoverRefusesATornTail is the #558 gate (Lane B8, scope call S3): a
// chain.cbor whose tail is torn (the file truncated mid-array, the shape a
// power loss or OOM-kill leaves) must REFUSE to start — Recover returns a
// LossError with no prefix — unless the operator accepts the loss explicitly.
// Before this rule the daemon printed the failure and started from genesis,
// silently discarding finalized history (run a434494-deep, h83, 87 MiB).
func TestRecoverRefusesATornTail(t *testing.T) {
	fullRep := map[ports.NodeID]int64{}
	for i := int64(1); i <= 5; i++ {
		fullRep[idOf(testKey(i))] = 1000
	}
	c := committedChain(t, func(n ports.NodeID) int64 { return fullRep[n] })
	path := filepath.Join(t.TempDir(), "chain.cbor")
	if err := Save(path, c.Blocks(0)); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Tear the tail: keep the first two thirds of the bytes.
	if err := os.WriteFile(path, raw[:len(raw)*2/3], 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	n, loss, refused := Recover(path, fresh, false)
	if refused == nil {
		t.Fatalf("#558: a torn chain.cbor must REFUSE to start; Recover returned restored=%d loss=%v refused=nil", n, loss)
	}
	var le *LossError
	if !errors.As(refused, &le) || le.Restored != 0 {
		t.Fatalf("want a LossError with no valid prefix, got %v", refused)
	}
	if _, h := fresh.Head(); h != 0 {
		t.Fatalf("a refused replay must leave the chain empty (next height 0), got next=%d", h)
	}

	// The operator's explicit acceptance is the ONLY way past: the loss is still
	// reported, the refusal is lifted, and the node starts from the (empty) prefix.
	fresh2 := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	n2, loss2, refused2 := Recover(path, fresh2, true)
	if refused2 != nil || loss2 == nil || n2 != 0 {
		t.Fatalf("with acceptLoss the refusal lifts and the loss stays reported: n=%d loss=%v refused=%v", n2, loss2, refused2)
	}
}

// TestRecoverRefusesACorruptSuffixKeepsPrefixOnlyWhenAccepted: a block that
// fails structural verification mid-file is the other loss shape — the valid
// prefix exists, the suffix is lost. Refuse unless accepted; with acceptance
// the prefix is what the node restarts on (the pre-existing longest-valid-
// prefix behaviour, now behind the operator's word).
func TestRecoverRefusesACorruptSuffixKeepsPrefixOnlyWhenAccepted(t *testing.T) {
	fullRep := map[ports.NodeID]int64{}
	for i := int64(1); i <= 5; i++ {
		fullRep[idOf(testKey(i))] = 1000
	}
	c := committedChain(t, func(n ports.NodeID) int64 { return fullRep[n] })
	blocks := c.Blocks(0)
	blocks[1].Entries[0].FileSize = 999999 // breaks the proposer signature
	path := filepath.Join(t.TempDir(), "chain.cbor")
	if err := Save(path, blocks); err != nil {
		t.Fatalf("save: %v", err)
	}
	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	n, _, refused := Recover(path, fresh, false)
	if refused == nil || n != 1 {
		t.Fatalf("a corrupt suffix must refuse (prefix 1): n=%d refused=%v", n, refused)
	}
	fresh2 := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	n2, loss2, refused2 := Recover(path, fresh2, true)
	if refused2 != nil || n2 != 1 || loss2 == nil || !errors.Is(loss2, chain.ErrBadSignature) {
		t.Fatalf("accepted loss keeps the 1-block prefix and names the cause: n=%d loss=%v refused=%v", n2, loss2, refused2)
	}
}

// TestRecoverAcceptedLossPreservesTheOriginal is the PE ruling's blocker 1 (B8):
// -accept-chain-loss must never destroy the file it rejects — a rejected
// chain.cbor may be byte-perfect and merely unreadable by THIS binary. With
// acceptance, the original is moved untouched to chain.cbor.rejected-<unix>,
// so the daemon's next Save cannot overwrite it; the LossError names it.
func TestRecoverAcceptedLossPreservesTheOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chain.cbor")
	garbage := []byte("not a chain")
	if err := os.WriteFile(path, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	_, loss, refused := Recover(path, fresh, true)
	if refused != nil || loss == nil || loss.Preserved == "" {
		t.Fatalf("accepted loss must lift the refusal and name the preserved file: loss=%v refused=%v", loss, refused)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the rejected chain.cbor must have been MOVED out of the daemon's save path, stat err=%v", err)
	}
	kept, err := os.ReadFile(loss.Preserved)
	if err != nil || string(kept) != string(garbage) {
		t.Fatalf("the preserved original must be byte-identical: err=%v got %q", err, kept)
	}
	// And a refusal leaves the file exactly where it was.
	if err := os.WriteFile(path, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, refused := Recover(path, chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 }), false); refused == nil {
		t.Fatal("refusal expected")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(garbage) {
		t.Fatalf("a refusal must leave chain.cbor untouched: err=%v", err)
	}
}
