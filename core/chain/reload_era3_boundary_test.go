package chain

import (
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
