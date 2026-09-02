package chain

import (
	"crypto/ed25519"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// RED home #3 — the era boundary Reload oracle, shipped AHEAD of era-3.
//
// The state-root certification's Q5(ii) is explicit about sequencing and about
// the shape of the mistake to avoid:
//
//	The #558 scar applies verbatim: extend the shared era-aware verification
//	paths (the `verifyAtt` pattern) — never fork a parallel era-3 path — and
//	ship the era-2→era-3 replay/Reload test AHEAD of the change … the boundary
//	chain must Reload and replay cleanly, and a torn/missing tree must trigger
//	a LOUD rebuild, never a genesis fallback.
//
// Era-3 blocks are not minted yet, so this file cannot replay a real era-3
// history. What it CAN do — and what makes it a RED home rather than a
// placeholder — is pin the two properties era-3 will need, in a form that
// extends by one block when era-3 arrives:
//
//  1. A history spanning an era boundary replays through the SHARED dispatcher.
//     `verifyAtt` (chain.go:643) is one switch on Phase; a forked parallel path
//     is precisely what breaks a mixed-era history, which is what every real
//     chain will be at an activation boundary.
//
//  2. A block from a FUTURE era — the exact situation era-3 activation creates
//     for every node that has not upgraded — must fail LOUDLY and report how
//     much was restored. #558's damage was never the rejection; it was the
//     SILENT fallback that followed it, which discarded finalized history while
//     reporting health.
//
// When era-3 lands, extend `mixedEraHistory` with an era-3 block. If someone
// forked a parallel verification path instead of extending `verifyAtt`, test 1
// goes RED at the boundary block.

// futurePhase is an attestation phase this binary does not know — the stand-in
// for era-3 arriving at an era-2 node. verifyAtt's `default: return false`
// (chain.go:652) is what must reject it.
const futurePhase uint8 = 99

// mixedEraHistory commits a chain that spans an era boundary: an era-1 genesis
// followed by an era-2 rounds block. This is the shape of every real chain at
// an activation height, and the shape era-3 will extend.
func mixedEraHistory(t *testing.T) (*Chain, []Block) {
	t.Helper()
	c, keys, g := roundsWorld(t)

	// Height 1: a genuine ERA-1 block with bare-hash (PhaseLegacy)
	// attestations. Genesis carries no attestations at all, so without this
	// block the legacy branch of verifyAtt is never exercised and the "spans
	// an era boundary" claim would be hollow — verified by ablation: deleting
	// the PhaseLegacy branch must turn this test RED.
	b1 := &Block{Version: 1, Height: 1, Prev: g.Hash(),
		Entries: []ports.Entry{entry(21)}}
	Sign(b1, keys[0])
	for _, k := range keys[1:] {
		b1.Atts = append(b1.Atts, Attest(b1, k))
	}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit era-1 block at height 1: %v", err)
	}

	// Height 2: the era-2 rounds block — the boundary itself.
	b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
		Entries: []ports.Entry{entry(22)}}
	b2.BondRegs = append(b2.BondRegs, bondReg(keys[1], twoMiB, b1.Hash()))
	commitRounds(b2, keys, 0)
	if err := c.Append(*b2); err != nil {
		t.Fatalf("commit era-2 block at height 2: %v", err)
	}

	// Byte-faithful to what chainstore persists.
	persisted, err := DecodeBlocks(EncodeBlocks(c.Blocks(0)))
	if err != nil {
		t.Fatalf("wire roundtrip: %v", err)
	}
	return c, persisted
}

// freshReplica is a same-config replica with an empty history — a restart.
func freshReplica(t *testing.T, src *Chain) *Chain {
	t.Helper()
	fc := New(src.cfg, func(ports.NodeID) int64 { return 0 })
	fc.SetBondVerifier(objectiveVerify)
	return fc
}

// TestEraBoundaryHistoryReplaysThroughSharedPath is obligation 1: a chain that
// spans an era boundary must Reload completely. It is the guard against a
// forked era-3 verification path — a fork works fine on a single-era history
// and breaks exactly here.
func TestEraBoundaryHistoryReplaysThroughSharedPath(t *testing.T) {
	src, persisted := mixedEraHistory(t)

	fresh := freshReplica(t, src)
	n, err := fresh.Reload(persisted)
	if err != nil {
		t.Fatalf("Reload of an era-spanning history failed at block %d: %v\n"+
			"Every chain is mixed-era at an activation boundary. A verification "+
			"path that handles one era but not a history containing both is the "+
			"#558 defect: extend the shared verifyAtt dispatcher (chain.go:643), "+
			"never fork a parallel path.", n, err)
	}
	if n != len(persisted) {
		t.Fatalf("Reload restored %d of %d blocks with no error — a partial "+
			"restore reported as success is the silent-truncation half of #558",
			n, len(persisted))
	}
	if _, h := fresh.Head(); h != uint64(len(persisted)) {
		t.Fatalf("restored head: next=%d, want %d", h, len(persisted))
	}
}

// TestFutureEraBlockFailsLoudlyAndNeverFallsBackToGenesis is obligation 2, and
// the one that carries the #558 lesson forward.
//
// At era-3 activation, every un-upgraded node meets a block it cannot verify.
// That rejection is CORRECT. What must never happen is the #558 sequence:
// replay fails → the node silently starts from genesis → finalized history is
// discarded while the node reports itself healthy, stranding it once peers
// prune below the horizon.
//
// The contract asserted here: Reload reports an error AND the honest count of
// what it restored, so no caller can mistake a truncated replay for a complete
// one.
func TestFutureEraBlockFailsLoudlyAndNeverFallsBackToGenesis(t *testing.T) {
	src, persisted := mixedEraHistory(t)

	// Forge the future-era block: re-sign height 1's attestations at a phase
	// this binary does not know. Everything else is untouched, so the ONLY
	// reason to reject it is the unknown era.
	future := append([]Block(nil), persisted...)
	fb := future[2]
	fb.Atts = nil
	for _, a := range persisted[2].Atts {
		fb.Atts = append(fb.Atts, Attestation{
			PubKey: a.PubKey, Sig: a.Sig, Round: a.Round, Phase: futurePhase,
		})
	}
	future[2] = fb
	future, err := DecodeBlocks(EncodeBlocks(future))
	if err != nil {
		t.Fatalf("future-era roundtrip: %v", err)
	}

	fresh := freshReplica(t, src)
	n, err := fresh.Reload(future)

	if err == nil {
		t.Fatalf("Reload ACCEPTED a block whose attestations carry an unknown "+
			"era phase (%d), restoring %d blocks.\nAn un-upgraded node must not "+
			"verify a future era's consensus signatures — verifyAtt's default "+
			"branch (chain.go:652) is what refuses them.", futurePhase, n)
	}

	// Loud AND honest: the caller must be able to see it got a partial prefix.
	if n != 2 {
		t.Errorf("Reload reported %d blocks restored before the future-era "+
			"block; want 2 (genesis + the era-1 block). A caller cannot "+
			"distinguish a truncated replay from a complete one if this count "+
			"is wrong.", n)
	}

	// The #558 assertion proper: the node is NOT silently sitting at genesis
	// believing itself healthy. The error is the signal the caller must act on.
	if _, h := fresh.Head(); h != 2 {
		t.Errorf("after a failed Reload the replica's head is %d; want 2 — the "+
			"honest restored prefix, with the error reported alongside it", h)
	}
	t.Logf("future-era block rejected loudly at index %d: %v", n, err)
}

// TestFutureEraRejectionIsAboutTheEraNotTheBytes is the positive control. The
// test above is only meaningful if the SAME block, differing only in its
// attestation phase, replays fine — otherwise it could be passing because the
// block was malformed for some unrelated reason.
func TestFutureEraRejectionIsAboutTheEraNotTheBytes(t *testing.T) {
	src, persisted := mixedEraHistory(t)

	fresh := freshReplica(t, src)
	n, err := fresh.Reload(persisted)
	if err != nil || n != len(persisted) {
		t.Fatalf("the unmodified history must replay cleanly (restored %d of %d, err=%v) — "+
			"without this control, the future-era test could pass for the wrong reason",
			n, len(persisted), err)
	}

	// And the rejection must name a signature/attestation problem rather than,
	// say, a height or ancestry complaint that would mean the forge was
	// malformed in some other way.
	future := append([]Block(nil), persisted...)
	fb := future[2]
	fb.Atts = nil
	for _, a := range persisted[2].Atts {
		fb.Atts = append(fb.Atts, Attestation{
			PubKey: a.PubKey, Sig: a.Sig, Round: a.Round, Phase: futurePhase,
		})
	}
	future[2] = fb
	future, _ = DecodeBlocks(EncodeBlocks(future))

	fresh2 := freshReplica(t, src)
	_, err = fresh2.Reload(future)
	if err == nil {
		t.Fatal("future-era block was accepted")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "signature") && !strings.Contains(msg, "quorum") &&
		!strings.Contains(msg, "attest") {
		t.Errorf("rejection reason %q does not name a signature/attestation/quorum "+
			"failure — the forged block may be malformed for an unrelated reason, "+
			"which would make the loud-failure test pass vacuously", err)
	}
}

// commitV4OnDisk extends the era3ValidityChain to commit ONE real v4 (era-3) block
// carrying correct post-apply roots, so the chain's persisted history contains an era-3
// block a fresh replica must Reload. It returns the full persisted history (genesis
// through the v4 block, wire-roundtripped) and the proposer key that signed the v4 block.
//
// The v4 block is committed through Append (the commit path, which enforces the roots at
// commit time), so a correct root is on disk. The tamper tests below then corrupt that
// on-disk block and re-sign it — the exact re-signed-wrong-root-v4 attack the Reload path
// must catch.
func commitV4OnDisk(t *testing.T) (persisted []Block, prop ed25519.PrivateKey) {
	t.Helper()
	c, prop := era3ValidityChain(t)
	att1, att2, att3 := key(30202), key(30203), key(30204)
	keys := []ed25519.PrivateKey{prop, att1, att2, att3}

	// Build a v4 block carrying the honest post-apply roots, then attach a full era-2
	// two-phase certificate (v4 is >= BlockVersionRounds, so ValidateCommit requires the
	// proposer-prepare + prepare-QC + precommit stack). Roots are set BEFORE commitRounds,
	// which signs, so the signature covers them.
	prev, next := c.Head()
	b := &Block{Version: BlockVersionStateRoot, Height: next, Prev: prev, Entries: []ports.Entry{entry(9)}}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot = &state
	b.LogRoot = &log
	commitRounds(b, keys, 0)
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit honest v4 block: %v", err)
	}

	// Byte-faithful to what chainstore persists.
	persisted, err = DecodeBlocks(EncodeBlocks(c.Blocks(0)))
	if err != nil {
		t.Fatalf("wire roundtrip: %v", err)
	}
	// Sanity: the last block is a v4 block, so the history genuinely exercises era-3.
	if last := persisted[len(persisted)-1]; last.Version != BlockVersionStateRoot {
		t.Fatalf("last persisted block is v%d, want v%d — the fixture is not era-3",
			last.Version, BlockVersionStateRoot)
	}
	return persisted, prop
}

// TestReloadRejectsResignedWrongStateRootV4 is the A-bare regression. It is the hole the
// blind PE ruled: appendStructural (the own-disk Reload path) verifies the proposer and
// attester SIGNATURES — which cover the block Hash, roots included — but a signature over
// a root proves only that the signer committed to THAT byte string, never that the root
// equals the post-apply state. A v4 block with a WRONG StateRoot that is re-signed with
// the proposer key passes every signature check in validateStructural. Integrity ≠
// root-correctness.
//
// Before the fix this test is RED because Reload ACCEPTS the tampered block. The RED must
// be that acceptance, and the GREEN must reject with ErrEra3StateRootMismatch — the
// SAME named error the commit path raises — NOT a signature error (that would mean the
// tamper broke integrity, not that the root check fired) and NOT a nil-map panic. The
// ablation subtest below pins the cause.
func TestReloadRejectsResignedWrongStateRootV4(t *testing.T) {
	persisted, prop := commitV4OnDisk(t)

	// Tamper: corrupt the v4 block's StateRoot and RE-SIGN with the proposer key, so the
	// block is otherwise byte-valid. Only the root check can catch it.
	tampered := resignWithWrongStateRoot(t, persisted, prop)

	fresh := New(reloadCfg(), func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	n, err := fresh.Reload(tampered)

	if !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("Reload of a re-signed wrong-StateRoot v4 block: got err=%v (restored %d), "+
			"want ErrEra3StateRootMismatch.\nappendStructural verifies signatures but a "+
			"signature over a wrong root is still a valid signature — the own-disk path "+
			"must re-validate the root against post-apply state (A-bare), exactly as the "+
			"commit path does.", err, n)
	}
	// Honest partial-restore count: genesis + block1 + revocation block were restored
	// before the tampered v4 block (index 3) was rejected.
	if n != len(persisted)-1 {
		t.Errorf("Reload restored %d blocks before rejecting the tampered v4 block; want %d "+
			"(everything up to but not including the tampered block)", n, len(persisted)-1)
	}
	// #558 half: the replica sits at the honest prefix head, not silently at genesis.
	if _, h := fresh.Head(); h != uint64(len(persisted)-1) {
		t.Errorf("after the rejected Reload the replica head is %d; want %d (the honest "+
			"restored prefix)", h, len(persisted)-1)
	}
}

// TestReloadWrongStateRootIsCaughtByTheRootCheckNotTheSignature is the ablation that
// keeps the green above honest (the session-7 leave-one-out lesson: a green check with
// no demonstrated correct-cause red is decoration). It proves TWO things:
//
//  1. The tampered block's PROPOSER SIGNATURE is VALID — validateStructural accepts it,
//     so the block is NOT rejected for a signature/ancestry/quorum reason. The only
//     remaining reason to reject it is the root check.
//  2. The SAME tampered history, Reloaded WITHOUT the root check, would be ACCEPTED —
//     demonstrated by the positive control (an honest, untampered v4 history Reloads
//     cleanly), so the rejection above is caused by the wrong root, not by the block
//     being malformed for an unrelated reason.
func TestReloadWrongStateRootIsCaughtByTheRootCheckNotTheSignature(t *testing.T) {
	persisted, prop := commitV4OnDisk(t)

	// Positive control: the untampered v4 history Reloads cleanly. Without this, the
	// rejection test could pass because the fixture itself is broken.
	ctrl := New(reloadCfg(), func(ports.NodeID) int64 { return 0 })
	ctrl.SetBondVerifier(objectiveVerify)
	if n, err := ctrl.Reload(persisted); err != nil || n != len(persisted) {
		t.Fatalf("the untampered v4 history must Reload cleanly (restored %d of %d, err=%v) — "+
			"without this control the rejection test could pass for the wrong reason",
			n, len(persisted), err)
	}

	tampered := resignWithWrongStateRoot(t, persisted, prop)

	// Assertion 1: validateStructural (signatures + ancestry + quorum) ACCEPTS the
	// tampered block. Replay the honest prefix into a fresh chain, then run
	// validateStructural on the tampered block directly — it must pass, proving the
	// re-sign made the wrong-root block signature-valid.
	fresh := New(reloadCfg(), func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	prefix := tampered[:len(tampered)-1]
	if n, err := fresh.Reload(prefix); err != nil || n != len(prefix) {
		t.Fatalf("honest prefix must Reload cleanly (restored %d of %d, err=%v)", n, len(prefix), err)
	}
	tb := tampered[len(tampered)-1]
	if err := fresh.validateStructural(&tb); err != nil {
		t.Fatalf("validateStructural REJECTED the tampered block (%v) — the re-sign did not "+
			"make it signature-valid, so the root-check regression is testing the wrong "+
			"thing (it must be the ROOT check, not the signature, that rejects)", err)
	}

	// Assertion 2: appendStructural (which now carries the post-apply root check) REJECTS
	// it, and specifically with the root-mismatch error — not a signature error, not a
	// panic. This is the demonstrated correct cause.
	err := fresh.appendStructural(tb)
	if !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("appendStructural on the signature-valid, wrong-root block: got %v, want "+
			"ErrEra3StateRootMismatch — the reject must be caused by the ROOT check", err)
	}
}

// TestReloadRejectsResignedWrongLogRootV4 is the LogRoot sibling: the same re-signed
// tamper on the LogRoot must be rejected with ErrEra3LogRootMismatch.
func TestReloadRejectsResignedWrongLogRootV4(t *testing.T) {
	persisted, prop := commitV4OnDisk(t)

	tampered := append([]Block(nil), persisted...)
	last := len(tampered) - 1
	b := tampered[last]
	wrong := *b.LogRoot
	wrong[0] ^= 0xFF
	b.LogRoot = &wrong
	b.hashMemoSet = false
	Sign(&b, prop)
	// Re-attest: the re-sign changed the block hash, so the persisted attestations no
	// longer verify. Re-attest with the same anchors so only the root check can reject.
	b.Atts = nil
	for _, k := range []ed25519.PrivateKey{key(30202), key(30203), key(30204)} {
		b.Atts = append(b.Atts, Attest(&b, k))
	}
	tampered[last] = b
	tampered, err := DecodeBlocks(EncodeBlocks(tampered))
	if err != nil {
		t.Fatalf("tamper roundtrip: %v", err)
	}

	fresh := New(reloadCfg(), func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	if _, err := fresh.Reload(tampered); !errors.Is(err, ErrEra3LogRootMismatch) {
		t.Fatalf("Reload of a re-signed wrong-LogRoot v4 block: got %v, want ErrEra3LogRootMismatch", err)
	}
}

// resignWithWrongStateRoot corrupts the persisted v4 block's StateRoot, re-signs with the
// proposer key, and re-attests with the launch anchors — so the returned history is
// byte-valid at every signature check and differs from the honest history ONLY in the
// committed StateRoot value. The re-sign is the attack: a naive integrity check ("the
// signature covers the root, so the root is trusted") accepts it.
func resignWithWrongStateRoot(t *testing.T, persisted []Block, prop ed25519.PrivateKey) []Block {
	t.Helper()
	out := append([]Block(nil), persisted...)
	last := len(out) - 1
	b := out[last]
	if b.Version != BlockVersionStateRoot {
		t.Fatalf("resignWithWrongStateRoot: last block is v%d, not v4", b.Version)
	}
	wrong := *b.StateRoot
	wrong[0] ^= 0xFF
	b.StateRoot = &wrong
	b.hashMemoSet = false
	Sign(&b, prop)
	b.Atts = nil
	for _, k := range []ed25519.PrivateKey{key(30202), key(30203), key(30204)} {
		b.Atts = append(b.Atts, Attest(&b, k))
	}
	out[last] = b
	out, err := DecodeBlocks(EncodeBlocks(out))
	if err != nil {
		t.Fatalf("tamper roundtrip: %v", err)
	}
	return out
}

// reloadCfg is the config era3ValidityChain builds, reconstructed so a fresh replica can
// Reload its history. It must MATCH era3ValidityChain's config exactly, or the anchor set
// / quorum differs and the replay fails for an unrelated reason.
func reloadCfg() Config {
	prop, att1, att2, att3 := key(30201), key(30202), key(30203), key(30204)
	return Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: map[ports.NodeID]bool{
			idOf(prop): true, idOf(att1): true, idOf(att2): true, idOf(att3): true,
		},
		AnchorQuorum: 1,
	}
}

// TestReloadRejectsV2AtEra3Boundary is the step-2c defense-in-depth symmetry regression.
// It is the version-boundary sibling of TestReloadRejectsResignedWrongStateRootV4: the 2b
// root check was duplicated onto the own-disk Reload path (appendStructural), but the 2c
// VERSION-boundary rule (ErrEra3VersionRequired — a v2 block at/above H_era3 is invalid)
// lived ONLY on the commit path (ValidateProposal/ValidateCommit). appendStructural did not
// run it, so a v2 block at/above H_era3 fed to Reload would be persisted unvalidated. This
// is not exploitable today (a valid quorum-signed v2 block cannot commit at/above H_era3),
// but a future unguarded disk-write path (fast-sync/import) is where the asymmetry could
// turn into a hole. The blind PE ruled the asymmetry (RULING-era3-step2c...-2026-08-29).
//
// The forged block is SIGNATURE-VALID (a full two-phase certificate), so the ONLY reason to
// reject it is the version rule. Before the fix appendStructural ACCEPTS it (RED); after,
// it rejects with ErrEra3VersionRequired (GREEN) — NOT a signature error and NOT a panic.
// The ablation subtest below pins that cause.
func TestReloadRejectsV2AtEra3Boundary(t *testing.T) {
	// Genesis-declared boundary at height 3: heights 1..2 are era-2, height >= 3 is era-3.
	c, keys := era3AnchorChain(t, 3)
	mustAppend(t, c, mintNext(t, c, keys)) // height 1, v2
	mustAppend(t, c, mintNext(t, c, keys)) // height 2, v2

	// Forge a signature-valid v2 block AT the boundary (height 3). The commit path would
	// reject it (ValidateProposal → ErrEra3VersionRequired), so it can never reach disk
	// through Append — we build it by hand and feed it to appendStructural directly, which
	// is exactly the own-disk path being hardened.
	prev, next := c.Head()
	if next != 3 {
		t.Fatalf("fixture: expected head 3 (H_era3), got %d", next)
	}
	v2AtBoundary := &Block{Version: BlockVersionRounds, Height: next, Prev: prev,
		Entries: []ports.Entry{entry(byte(next))}}
	twoPhaseSign(v2AtBoundary, keys)

	// Assertion 1 (the RED cause pin): validateStructural ACCEPTS the block — its
	// signatures/ancestry/quorum are valid, so the ONLY remaining reason to reject it is
	// the version rule, not a signature failure.
	if err := c.validateStructural(v2AtBoundary); err != nil {
		t.Fatalf("validateStructural REJECTED the forged v2 boundary block (%v) — the forge is "+
			"malformed for an unrelated reason, so the version-rule regression would test the "+
			"wrong thing (it must be the VERSION check, not a signature check, that rejects)", err)
	}

	// Assertion 2 (the regression): appendStructural (the own-disk Reload path) REJECTS it
	// with ErrEra3VersionRequired. RED before the fix (accepted), GREEN after, correct cause.
	err := c.appendStructural(*v2AtBoundary)
	if !errors.Is(err, ErrEra3VersionRequired) {
		t.Fatalf("appendStructural on a signature-valid v2 block at H_era3: got %v, want "+
			"ErrEra3VersionRequired.\nThe own-disk Reload path must run the version-boundary "+
			"rule, exactly as the commit path does — the 2b root check symmetry extended to the "+
			"2c version rule.", err)
	}
	// The rejection left NO block applied: head unchanged at 3 (longest-valid-prefix contract).
	if _, h := c.Head(); h != 3 {
		t.Fatalf("after the rejected appendStructural the head is %d; want 3 — a rejected block "+
			"must not be left applied (the version check runs BEFORE apply)", h)
	}
}

// TestReloadV2BoundaryRuleDoesNotOverReject is the over-rejection control for the version
// rule on the Reload path: a legit v2 block BELOW H_era3, and a correct v4 block AT/ABOVE
// H_era3, must both still Reload/appendStructural cleanly. Without this, the rule above
// could pass by rejecting everything.
func TestReloadV2BoundaryRuleDoesNotOverReject(t *testing.T) {
	c, keys := era3AnchorChain(t, 3)

	// A v2 block below H_era3 (height 1) appendStructurals cleanly.
	b1 := mintNext(t, c, keys)
	if b1.Version != BlockVersionRounds {
		t.Fatalf("height 1 must mint v2, got v%d", b1.Version)
	}
	if err := c.appendStructural(*b1); err != nil {
		t.Fatalf("a v2 block BELOW H_era3 must appendStructural cleanly, got %v — the version "+
			"rule over-rejected below the boundary", err)
	}
	// Height 2, still below the boundary.
	b2 := mintNext(t, c, keys)
	if err := c.appendStructural(*b2); err != nil {
		t.Fatalf("a v2 block at height 2 (below H_era3) must appendStructural cleanly, got %v", err)
	}

	// A correct v4 block AT H_era3 (height 3) appendStructurals cleanly.
	b3 := mintNext(t, c, keys)
	if b3.Version != BlockVersionStateRoot {
		t.Fatalf("at H_era3 mintNext must produce v4, got v%d", b3.Version)
	}
	if err := c.appendStructural(*b3); err != nil {
		t.Fatalf("a correct v4 block AT H_era3 must appendStructural cleanly, got %v — the "+
			"version rule wrongly rejected a valid v4 block at the boundary", err)
	}
	if _, h := c.Head(); h != 4 {
		t.Fatalf("after appending the v4 boundary block the head is %d; want 4", h)
	}
}

// TestEveryDiskWritePathRunsTheEra3RootCheck is the write-set enumeration guard. It is
// STRUCTURAL, not a hand-list: it discovers every Chain method that writes a block to the
// live committed history by scanning for calls to c.apply(b), and asserts that each such
// method runs BOTH era-3 consensus-entry rules on the block it applied — the ROOT check
// (validateEra3Roots) AND the VERSION-boundary rule (validateEra3Version). A FUTURE
// unguarded disk-write path (fast-sync, import) fails this guard rather than silently
// re-opening the A-bare hole OR the 2c version-boundary asymmetry.
//
// Why BOTH, not just the root check (the 2c hardening): a v2 block carries NO roots, so the
// root check is era-gated OFF for it (validateEra3Roots returns nil for sub-v4). A future
// disk-write path that ran ONLY the root check would satisfy the old guard while persisting
// a v2 block at/above H_era3 — the exact defense-in-depth asymmetry the blind PE ruled
// (RULING-era3-step2c...-2026-08-29). Requiring the version rule too makes "every disk-write
// path enforces the era-3 rules" UNIFORM across root AND version.
//
// Enforcement mechanism (structural, rot-proof): each era-3 rule is centralized in ONE named
// validator — validateEra3Roots and validateEra3Version — both checked BEFORE apply so a
// rejection never leaves a bad block applied. Both disk-write families route through both:
// the commit path via ValidateProposal (which calls both), and the reload path
// (appendStructural) calls both directly. The guard reads chain.go's source and asserts,
// for each of the two rules independently:
//
//   - every method's body CALLS that rule's validator (directly, or via a validator that
//     does — ValidateCommit/ValidateProposal for the commit family), OR is on the explicit
//     genesis allowlist (AppendGenesis: a v1 genesis is declared-not-agreed, below any era-3
//     boundary and carrying no committed root by construction).
//
// A new `func (c *Chain) FastSync(b Block) { ...; c.apply(b); ... }` that forgets EITHER
// rule trips this guard: it calls c.apply but does not call that rule's validator and is not
// allowlisted. That is the future hole this closes.
//
// Coverage is decided by callsFn, which matches a real CALL — the validator name followed
// by `(` in the method body AFTER comments are stripped — not a bare symbol mention. The
// earlier strings.Contains(body, name) form was defeatable two ways: a method with
// `// validateEra3Roots intentionally skipped` plus a bare `c.apply(b)` scored "guarded"
// while running no check (comment text matched), and any non-call mention of the symbol
// matched too. TestGuardMatchesCallsNotCommentText is the ablation for that defeat.
func TestEveryDiskWritePathRunsTheEra3RootCheck(t *testing.T) {
	src := readChainSource(t)
	methods := methodsCallingApply(t, src)
	if len(methods) == 0 {
		t.Fatal("found NO methods calling c.apply — the scanner is broken (it must find at " +
			"least Append/appendStructural/AppendGenesis), so this guard would pass vacuously")
	}

	// The two named era-3 consensus-entry validators. A disk-write method is guarded for a
	// rule if its body names that rule's validator, OR names a validator known to run it
	// (the commit family funnels through ValidateProposal, which calls BOTH).
	//
	//   - validateEra3Roots:   the committed-root check (2b).
	//   - validateEra3Version: the version-boundary rule (2c) — the addition this guard now
	//                          requires, so a path enforcing only the root check REDs.
	//   - validateEra4Version: the era-4 (v5) version-boundary rule (4d) — enforced on the
	//                          SAME paths as the era-3 version rule, so it is pinned on every
	//                          disk-write path too. A path enforcing only the era-3 rules REDs.
	//                          (The v5 committed-root check reuses validateEra3Roots, which
	//                          recomputes via StateRootForVersion(b.Version) — already covered.)
	//   - validateCarrier:     the era-4 (v5) LastCommit carrier validity rule (R-BOX-ATTESTS
	//                          O1). The carrier is a TRANSITION input (it writes validatorsSeen),
	//                          so a disk-write path that skips it can persist a block whose
	//                          seating rule was never checked. Enforced on the SAME paths.
	era3Rules := []string{"validateEra3Roots", "validateEra3Version", "validateEra4Version", "validateCarrier"}
	// Validators that themselves run BOTH era-3 rules (transitive coverage). ValidateProposal
	// calls both; ValidateCommit calls ValidateProposal — so a method calling either is
	// covered for both rules. Kept as an explicit, auditable transitive set rather than a
	// full call-graph walk; each entry is verified below to actually reach both rules.
	transitiveGuards := []string{"ValidateProposal", "ValidateCommit"}
	// genesisAllowlist: methods that legitimately persist a block WITHOUT the era-3 rules.
	// A v1 genesis is declared-not-agreed (Bitcoin-shape), sits below any era-3 boundary and
	// carries no StateRoot/LogRoot by construction, so neither era-3 predicate applies. This
	// is the ONLY allowed exemption; adding to it is a reviewed decision, not a silent default.
	genesisAllowlist := map[string]bool{"AppendGenesis": true}

	// callsRule reports whether a method body runs the given rule directly, or via a
	// transitive guard that reaches it.
	callsRule := func(body, rule string) bool {
		if callsFn(body, rule) {
			return true
		}
		for _, g := range transitiveGuards {
			if callsFn(body, g) {
				return true
			}
		}
		return false
	}

	// Verify each transitive guard genuinely reaches BOTH era-3 rules, so the allowance is
	// not a fiction that lets a real hole through. ValidateCommit reaches them via
	// ValidateProposal, so a call to another transitive guard counts as reaching.
	for _, g := range transitiveGuards {
		body := methodBody(t, src, g)
		for _, rule := range era3Rules {
			reaches := callsFn(body, rule)
			for _, other := range transitiveGuards {
				if other != g && callsFn(body, other) {
					reaches = true
				}
			}
			if !reaches {
				t.Fatalf("transitive guard %q no longer reaches era-3 rule %q — the guard's "+
					"coverage assumption rotted; a disk-write path relying on it is now "+
					"unguarded for that rule", g, rule)
			}
		}
	}

	// The core assertion: every disk-write path runs BOTH rules.
	for _, m := range methods {
		if genesisAllowlist[m] {
			continue
		}
		body := methodBody(t, src, m)
		for _, rule := range era3Rules {
			if !callsRule(body, rule) {
				t.Errorf("method %q writes a block to disk (calls c.apply) but does NOT run "+
					"era-3 rule %q (neither directly nor via %v) and is not on the genesis "+
					"allowlist. A disk-write path that skips an era-3 rule re-opens the hole it "+
					"closes: skipping validateEra3Roots persists a re-signed wrong-root v4 block "+
					"(A-bare); skipping validateEra3Version persists a v2 block at/above H_era3 "+
					"(the 2c version-boundary asymmetry). Route the block through %q BEFORE apply, "+
					"so a rejection leaves no bad block applied.", m, rule, transitiveGuards, rule)
			}
		}
	}

	// Belt-and-suspenders: the two paths we KNOW must be in the set are, so a scanner that
	// silently matched nothing cannot pass.
	haveAppend, haveStructural := false, false
	for _, m := range methods {
		switch m {
		case "Append":
			haveAppend = true
		case "appendStructural":
			haveStructural = true
		}
	}
	if !haveAppend || !haveStructural {
		t.Errorf("the apply-scanner missed a known disk-write path (Append=%v, "+
			"appendStructural=%v) — the structural guard is not covering the real set",
			haveAppend, haveStructural)
	}
}

// callsFn reports whether methodBody contains a real CALL to fn — the name immediately
// followed by `(` — after line and block comments are stripped. This is the robust form of
// the coverage predicate. The naive strings.Contains(body, fn) it replaces matched two
// non-calls: comment text (`// validateEra3Roots skipped`) and any bare symbol mention,
// either of which would score an unguarded method as guarded. Stripping comments closes the
// comment hole; requiring the trailing `(` closes the bare-mention hole. The trailing `(`
// tolerates whitespace so `validateEra3Roots (` still counts.
func callsFn(methodBody, fn string) bool {
	code := stripComments(methodBody)
	for i := 0; ; {
		j := strings.Index(code[i:], fn)
		if j < 0 {
			return false
		}
		after := code[i+j+len(fn):]
		k := 0
		for k < len(after) && (after[k] == ' ' || after[k] == '\t' || after[k] == '\n' || after[k] == '\r') {
			k++
		}
		if k < len(after) && after[k] == '(' {
			return true
		}
		i += j + len(fn)
	}
}

// stripComments removes // line comments and /* */ block comments from Go source. It does
// not track string/rune literals — the method bodies scanned here contain no string literal
// holding a `//` or `/*`, so the simple scan is sufficient and stays legible. If that ever
// changes, prefer go/scanner over extending this.
func stripComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// TestGuardMatchesCallsNotCommentText is the ablation for the coverage predicate itself: it
// injects the exact defeat a symbol-name grep permits and shows callsFn REDs on it. A
// comment-only mention of the validator (with a bare c.apply and no real call) is genuinely
// unguarded and must NOT count as covered; a real call must; no mention at all must not.
// Before the callsFn change the comment-only case passed (green decoration); after, it is
// caught.
func TestGuardMatchesCallsNotCommentText(t *testing.T) {
	const fn = "validateEra3Roots"

	// commentOnly is the Tester's defeat verbatim: the validator name appears ONLY in a
	// comment, the block is applied bare. A strings.Contains(body, fn) grep scores this
	// "guarded"; it is not.
	commentOnly := "{\n\t// validateEra3Roots intentionally skipped here\n\tc.apply(b)\n}"
	if callsFn(commentOnly, fn) {
		t.Errorf("callsFn scored a comment-only mention as a call — the guard is still "+
			"defeatable by %q plus a bare c.apply(b), which re-opens the A-bare hole a "+
			"future disk-write path could slip through", "// "+fn)
	}

	// blockCommentOnly is the same defeat via a /* */ comment, to prove stripComments covers
	// both comment forms.
	blockCommentOnly := "{\n\t/* validateEra3Roots handled elsewhere */\n\tc.apply(b)\n}"
	if callsFn(blockCommentOnly, fn) {
		t.Errorf("callsFn scored a block-comment mention as a call — stripComments must " +
			"remove /* */ comments too")
	}

	// realCall is a genuinely guarded path: a real validateEra3Roots(...) call. It must count.
	realCall := "{\n\tif err := c.validateEra3Roots(&b); err != nil {\n\t\treturn err\n\t}\n\tc.apply(b)\n}"
	if !callsFn(realCall, fn) {
		t.Errorf("callsFn missed a REAL %s(...) call — the fix broke the guard's true-positive "+
			"path; genuinely guarded methods would now be flagged as holes", fn)
	}

	// spacedCall exercises the whitespace tolerance between name and `(`.
	spacedCall := "{\n\tc.validateEra3Roots (&b)\n\tc.apply(b)\n}"
	if !callsFn(spacedCall, fn) {
		t.Errorf("callsFn missed a call with whitespace before `(` — %q should still count", fn+" (")
	}

	// noMention is a genuinely unguarded path: no reference to the validator at all. It must
	// not count (unchanged from the naive predicate, but pinned so the fix cannot flip it).
	noMention := "{\n\tc.apply(b)\n}"
	if callsFn(noMention, fn) {
		t.Errorf("callsFn scored a body with no mention of %s as guarded — impossible unless "+
			"the matcher is broken", fn)
	}
}

// readChainSource returns the source of core/chain/chain.go, located relative to this
// test file so it does not depend on the working directory.
func readChainSource(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate the source directory")
	}
	dir := thisFile[:strings.LastIndex(thisFile, "/")]
	b, err := os.ReadFile(dir + "/chain.go")
	if err != nil {
		t.Fatalf("read chain.go: %v", err)
	}
	return string(b)
}

// methodsCallingApply scans the source for methods with a `func (c *Chain) Name(` receiver
// whose body contains a call to `c.apply(`. Returns the method names.
func methodsCallingApply(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, name := range chainMethodNames(src) {
		if strings.Contains(methodBody(t, src, name), "c.apply(") {
			out = append(out, name)
		}
	}
	return out
}

// chainMethodNames returns every `func (c *Chain) Name(` method name in src.
func chainMethodNames(src string) []string {
	const marker = "func (c *Chain) "
	var names []string
	seen := map[string]bool{}
	for i := 0; ; {
		j := strings.Index(src[i:], marker)
		if j < 0 {
			break
		}
		start := i + j + len(marker)
		paren := strings.IndexByte(src[start:], '(')
		if paren < 0 {
			break
		}
		name := strings.TrimSpace(src[start : start+paren])
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		i = start + paren
	}
	return names
}

// methodBody returns the source text of `func (c *Chain) name(...) { ... }` by brace
// matching from the method's opening brace. Used to scope the c.apply / validator scans to
// one method.
func methodBody(t *testing.T, src, name string) string {
	t.Helper()
	marker := "func (c *Chain) " + name + "("
	idx := strings.Index(src, marker)
	if idx < 0 {
		t.Fatalf("method %q not found in chain.go", name)
	}
	// Find the first '{' after the signature.
	open := strings.IndexByte(src[idx:], '{')
	if open < 0 {
		t.Fatalf("method %q: no opening brace", name)
	}
	open += idx
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open : i+1]
			}
		}
	}
	t.Fatalf("method %q: unbalanced braces", name)
	return ""
}
