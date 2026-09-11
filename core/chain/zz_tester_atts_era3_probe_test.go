package chain

// TESTER PROBE — R-ATTS-ERA3-ROOT-COUPLING. Diagnosis only, NOT a shipped gate.
// Drives the claim: "on a v4 block the proposer commits StateRoot with Atts empty; on
// reload the root is recomputed over the STORED Atts; so any Atts entry that first-seats
// a qualified id changes the recomputed root and the node refuses to start."

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// ProbeA — does the divergence fire at COMMIT, before anything reaches disk?
// era-3 activation at height 1: the FIRST non-genesis block is v4 and validatorsSeen is
// still EMPTY, so its Atts first-seat three qualified ids.
func TestProbeA_AttsFirstSeatOnFirstV4Block_CommitPath(t *testing.T) {
	c, keys := era3AnchorChain(t, 1)
	t.Logf("PRE: validatorsSeen=%d qualified(k1)=%v objective=%v epochsEnabled=%v matureEpoch=%v",
		len(c.validatorsSeen), c.attesterQualified(idOf(keys[1])), c.objective(), c.epochsEnabled(), c.matureEpoch)

	prev, next := c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if mv := c.MintVersion(next); mv != BlockVersionStateRoot {
		t.Fatalf("fixture: MintVersion(%d)=%d, want v4(%d)", next, mv, BlockVersionStateRoot)
	}
	// Proposer order, verbatim from core/node/chainrole.go: roots FIRST, gather AFTER.
	if err := c.PopulateEra3Roots(b); err != nil {
		t.Fatalf("PopulateEra3Roots: %v", err)
	}
	committed := *b.StateRoot
	if len(b.Atts) != 0 {
		t.Fatalf("fixture: Atts must be empty at root population, got %d", len(b.Atts))
	}
	twoPhaseSign(b, keys) // the gather: 4 precommits, 3 of them non-proposer + qualified
	t.Logf("BLOCK: v%d height=%d Atts=%d committedStateRoot=%x", b.Version, b.Height, len(b.Atts), committed[:8])

	// What the reload/commit recompute would produce over the STORED Atts:
	rState, _, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	t.Logf("RECOMPUTE over stored Atts: %x  (committed %x)  equal=%v", rState[:8], committed[:8], rState == committed)

	err = c.Append(*b)
	t.Logf("Append verdict: err=%v  ErrEra3StateRootMismatch=%v", err, errors.Is(err, ErrEra3StateRootMismatch))
	if err == nil {
		t.Logf("COMMIT ACCEPTED. post-apply validatorsSeen=%d", len(c.validatorsSeen))
	}
}

// ProbeB — the reload half. Build a chain that legitimately commits v4 blocks, then
// RELOAD a fresh replica from the persisted blocks. No tampering, no attacker.
func TestProbeB_ReloadOwnDiskHistory(t *testing.T) {
	c, keys := era3AnchorChain(t, 3)
	for i := 0; i < 5; i++ {
		b := mintNext(t, c, keys)
		if err := c.Append(*b); err != nil {
			t.Fatalf("append height %d v%d: %v", b.Height, b.Version, err)
		}
		t.Logf("committed height=%d v%d Atts=%d validatorsSeen=%d", b.Height, b.Version, len(b.Atts), len(c.validatorsSeen))
	}
	disk := c.Blocks(0)
	t.Logf("disk: %d blocks; last block Atts=%d", len(disk), len(disk[len(disk)-1].Atts))

	fresh, _ := freshReplicaKeys(t, keys, 3)
	n, err := fresh.Reload(disk)
	t.Logf("Reload verdict: restored=%d/%d err=%v", n, len(disk), err)
	if err != nil {
		t.Logf("REFUSED TO START at block index %d: %v (StateRootMismatch=%v)", n, err, errors.Is(err, ErrEra3StateRootMismatch))
	}
}

// ProbeC — the ONE case the residual actually needs: a v4 block whose stored Atts
// first-seat a qualified id, arriving on a replica that never ran the commit check on
// THOSE bytes. Constructed by appending a genuine, verifying precommit from a qualified
// validator that the proposer's gather did not include (Atts are outside Hash(), so this
// leaves every signature valid).
func TestProbeC_LateQualifiedPrecommitAppendedToStoredBlock(t *testing.T) {
	// 5 validators; only 4 sign the certificate. The 5th is bonded + qualified and its
	// precommit is appended to the STORED copy afterwards.
	keys := []ed25519.PrivateKey{key(77001), key(77002), key(77003), key(77004), key(77005)}
	c := era3ChainWithKeys(t, keys, 3)

	for i := 0; i < 5; i++ {
		prev, next := c.Head()
		b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
		if c.MintVersion(next) >= BlockVersionStateRoot {
			if err := c.PopulateEra3Roots(b); err != nil {
				t.Fatalf("roots h%d: %v", next, err)
			}
		} else {
			b.Version = BlockVersionRounds
		}
		twoPhaseSign(b, keys[:4]) // the 5th never signs
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d v%d: %v", b.Height, b.Version, err)
		}
		t.Logf("committed h=%d v%d Atts=%d validatorsSeen=%d seen(k5)=%v",
			b.Height, b.Version, len(b.Atts), len(c.validatorsSeen), c.validatorsSeen[idOf(keys[4])])
	}
	disk := append([]Block(nil), c.Blocks(0)...)

	// Append k5's genuine precommit to the LAST v4 block as stored. Signature verifies
	// over the same hash; the block hash does not move (Atts are outside the preimage).
	last := len(disk) - 1
	hBefore := disk[last].Hash()
	disk[last].Atts = append(append([]Attestation(nil), disk[last].Atts...),
		AttestAt(&disk[last], keys[4], 0, PhasePrecommit))
	hAfter := disk[last].Hash()
	t.Logf("tampered stored block h=%d v%d: Atts %d -> %d; hash moved=%v",
		disk[last].Height, disk[last].Version, len(disk[last].Atts)-1, len(disk[last].Atts), hBefore != hAfter)

	fresh, _ := freshReplicaKeys(t, keys, 3)
	n, err := fresh.Reload(disk)
	t.Logf("Reload verdict: restored=%d/%d err=%v StateRootMismatch=%v", n, len(disk), err, errors.Is(err, ErrEra3StateRootMismatch))
}

// --- fixtures (local to this probe) ---

func era3ChainWithKeys(t *testing.T, keys []ed25519.PrivateKey, activation uint64) *Chain {
	t.Helper()
	c, _ := freshReplicaKeys(t, keys, activation)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c
}

func freshReplicaKeys(t *testing.T, keys []ed25519.PrivateKey, activation uint64) (*Chain, []ed25519.PrivateKey) {
	t.Helper()
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 40, Era3ActivationHeight: activation}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c, keys
}
