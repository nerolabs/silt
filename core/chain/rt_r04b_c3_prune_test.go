package chain

// R0.4b C3 re-break — BREAK 1 (F1) regression gates, chain tier.
//
// Source: the red-team probes core/chain/rt_c3_prune_test.go +
// rt_c3_prune_accept_test.go, archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.
// The probes asserted the BREAK; these assert the CLOSE, on the identical fixture, so the
// same scenario that measured the split now measures its absence.
//
// THE MECHANISM (root cause, not the symptom). applyIssuerKeys called
// pruneIssuerKeyCommit(b.Height) unconditionally, so a block carrying ZERO IssuerKeys still
// DELETED committed issuerKeyCommit leaves at an epoch turn. The floor box's scope gate
// stalls only on len(b.IssuerKeys) > 0 and its O(payload) fold has no op for the prune, so
// those committed writes were neither folded nor named. Measured consequence, both
// directions (rt_r04b_c3_split_test.go): the box AGREED with a forged root the full node
// rejects, and read an HONEST zero-registration block as a forged root.
//
// THE CLOSE. The prune runs only inside the registration-carrying branch of applyIssuerKeys.
// "len(b.IssuerKeys) == 0 ⇒ no issuerKeyCommit write" is now a property of apply() rather
// than an assumption of the box. The keyspace stays bounded because every ADD is in-band by
// validity, so pruning at each add re-establishes the 2W+2-bucket bound on every block that
// can grow it. See core/chain/issuerkey.go and
// docs/thinking/2026-09-02-r0.4b-c3-close-design.md ("re-break round").

import (
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// buildPruneFixture: a v5 objective chain, EpochBlocks=2, advanced so the NEXT block is the
// h=10 boundary (epoch 5). withReg seeds a genesis demand-issuer key registration for epoch 0,
// which the OLD height-driven prune dropped at exactly that boundary (cur=5 > prePublish=4,
// 0+4 < 5). It is the red-team fixture, verbatim.
func buildPruneFixture(t *testing.T, withReg bool) rotateFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(55001)
	v2 := key(55002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(v2, ports.HashBytes(pubOf(v2)), 4<<20, ports.Hash{}, 5, 2),
	)
	if withReg {
		// A perfectly ordinary, VALID registration: own epoch (genesis = epoch 0), self-signed.
		g.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(prop, 0, ports.Hash{0x77})}
	}
	Sign(g, prop)
	c.apply(*g)

	for {
		_, h := c.Head()
		if h == 10 {
			break
		}
		prev, hh := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: hh, Prev: prev,
			Entries: []ports.Entry{entry(byte(50 + hh))}}
		Sign(&b, prop)
		c.apply(b)
	}
	if !c.matureEpoch {
		t.Fatalf("fixture: expected matureEpoch=true")
	}
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("fixture pre-root mismatch")
	}
	return rotateFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop}
}

func issuerKeyLeafKeys(c *Chain) []string {
	var out []string
	for _, lf := range c.stateRootLeavesV5() {
		if tagOfKey(string(lf.Key)) == "issuerKeyCommit" {
			out = append(out, string(lf.Key))
		}
	}
	return out
}

// TestRTC3_ZeroRegistrationBlockWritesNoIssuerKeyCommitLeaf is the direct inversion of
// red-team BREAK-A. The fixture block is the epoch-turn height whose prune deleted the
// committed epoch-0 leaf; it carries no registrations, so it must now change NOTHING in the
// issuerKeyCommit keyspace — and the scope gate's silence must therefore be correct rather
// than merely permissive.
func TestRTC3_ZeroRegistrationBlockWritesNoIssuerKeyCommitLeaf(t *testing.T) {
	f := buildPruneFixture(t, true)
	if got := len(issuerKeyLeafKeys(f.c)); got != 1 {
		t.Fatalf("pre-state should hold exactly 1 issuerKeyCommit leaf, got %d", got)
	}
	b := f.boundaryBlock(nil)
	if len(b.IssuerKeys) != 0 {
		t.Fatalf("probe bug: block must carry no IssuerKeys")
	}

	post := f.c.cloneForDryRun()
	post.apply(b)
	if got := len(issuerKeyLeafKeys(post)); got != 1 {
		t.Fatalf("BREAK-A REOPENED: a block carrying 0 IssuerKeys changed the issuerKeyCommit "+
			"leaf set (1 -> %d). The prune is height-driven again; the box's scope gate "+
			"(len(b.IssuerKeys) > 0) cannot see it and the fold cannot reproduce it.", got)
	}

	for k := range committedLeafDiff(f.c, post) {
		if tagOfKey(k) == "issuerKeyCommit" {
			t.Fatalf("BREAK-A REOPENED: the committed-leaf diff names issuerKeyCommit key %x "+
				"for a block with no registrations", k)
		}
	}

	// And the gate stays silent — which is now the CORRECT answer, not a miss.
	w := f.witnessForBoundary(t, b)
	if err := f.c.stateRootScopeGate(f.prevRoot, b, w); err != nil {
		t.Fatalf("the scope gate stalled on an in-scope block: %v", err)
	}
}

// TestRTC3_BoundaryRecomputeAgreesWithAndWithoutAPriorRegistration is the inversion of
// BREAK-B. One ordinary registration five epochs earlier flipped the SAME boundary block from
// "box agrees" to "recomputed root != committed root" (read: forged root) on an honest block.
// The control and the attack must now give the same verdict: agree.
func TestRTC3_BoundaryRecomputeAgreesWithAndWithoutAPriorRegistration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		witReg bool
	}{
		{"no-prior-registration", false},
		{"prior-registration-committed", true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := buildPruneFixture(t, tc.witReg)
			b := f.boundaryBlock(nil)
			committed := f.applyAndCommittedRoot(t, b)
			w := f.witnessForBoundary(t, b)
			if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
				t.Fatalf("BREAK-B REOPENED: the boundary recompute of an honest block with 0 "+
					"IssuerKeys did not agree: %v", err)
			}
		})
	}
}

// TestRTC3_LeafDiffMinusFoldIsEmptyAtThePruneBoundary is the inversion of BREAK-C: the
// completeness property the leaf-diff guard asserts, run on the shape its exemption argument
// was wrong about. Every committed leaf a real apply() changes must be a leaf the fold
// reproduces.
func TestRTC3_LeafDiffMinusFoldIsEmptyAtThePruneBoundary(t *testing.T) {
	f := buildPruneFixture(t, true)
	b := f.boundaryBlock(nil)
	post := f.c.cloneForDryRun()
	post.apply(b)
	diff := committedLeafDiff(f.c, post)

	ops, err := f.c.assembleStateRootRecomputeOps(f.prevRoot, f.applyAndCommittedRoot(t, b), b, f.witnessForBoundary(t, b))
	if err != nil {
		t.Fatalf("op assembly stalled on an in-scope block: %v", err)
	}
	folded := foldedChangeKeys(ops)
	var missing []string
	for k := range diff {
		if _, ok := folded[k]; !ok {
			missing = append(missing, tagOfKey(k))
		}
	}
	if len(missing) != 0 {
		t.Fatalf("BREAK-C REOPENED: diff-minus-fold = %v — committed writes the O(payload) "+
			"recompute does not reproduce and the scope gate does not name", missing)
	}
}

// TestRTC3_PruneStillBoundsTheKeyspace earns the other half of the close. Making the prune
// payload-driven must not cost build-immutable #8: a registration-carrying block still
// narrows the committed keyspace to the [cur-W, cur+prePublish] band, so the bucket count
// stays bounded no matter how long the chain runs.
func TestRTC3_PruneStillBoundsTheKeyspace(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 4096}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(55011)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	g.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(prop, 0, ports.Hash{0x01})}
	Sign(g, prop)
	c.apply(*g)

	// Register a fresh key for the block's own epoch every few blocks, for 100 blocks.
	maxBuckets := 0
	for i := 0; i < 100; i++ {
		prev, h := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			Entries: []ports.Entry{entry(byte(i))}}
		if h%3 == 0 {
			b.IssuerKeys = []IssuerKeyReg{
				SignIssuerKeyReg(prop, c.blockEpoch(h), ports.Hash{byte(i), 0x02}),
			}
		}
		Sign(&b, prop)
		c.apply(b)
		if n := len(c.issuerKeyCommit); n > maxBuckets {
			maxBuckets = n
		}
	}
	// The band is [cur-W, cur+prePublish] with W == prePublish == issuerKeyPrePublish, so at
	// most 2W+1 buckets can survive a prune, plus the bucket the current block just added.
	if limit := int(2*issuerKeyPrePublish + 2); maxBuckets > limit {
		t.Fatalf("issuerKeyCommit grew to %d epoch buckets over 100 blocks (bound %d) — "+
			"the payload-driven prune no longer bounds the keyspace (build-immutable #8)",
			maxBuckets, limit)
	}
	t.Logf("max issuerKeyCommit epoch buckets over 100 blocks: %d (bound %d)",
		maxBuckets, 2*issuerKeyPrePublish+2)
}
