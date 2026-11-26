package chain

import (
	"crypto/ed25519"
	"github.com/nerolabs/silt/core/advisory"
	"testing"

	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestFloorBoxAndFullNodeReturnTheSameVerdicts is the DIFFERENTIAL the floor box's flip waits on:
// the one accept composition (ValidateCommitV5) driven over the full node's liveView and, through
// the box's own door, over its provenView with honest witnesses, across a corpus of blocks — and
// the two verdict sets must be identical. "Same security as a tree-holding node" is then a
// measured equivalence over this corpus, not an assertion.
//
// The box's verdict is taken as its door returns it. With an honest prover-backed source serving
// every read, a stall means the box could not see what the node saw, which fails the differential.
//
// The corpus is honest blocks of each payload class the fixture can commit (entries, revocations,
// un-revocations, bond registrations, false-repair slashes, issuer-key registrations), and blocks
// broken in exactly one rule. Every case must yield Accept or Reject on the full node (a liveView
// never stalls), and the box must match it. The box reads through
// the production WitnessProvider, the serving path a real box uses.
func TestFloorBoxAndFullNodeReturnTheSameVerdicts(t *testing.T) {
	type tcase struct {
		name  string
		build func(f structFixture) Block
		want  FloorBoxOutcome // the full node's verdict, asserted so the corpus is known to span both
	}
	reseal := func(f structFixture, b *Block) {
		b.ProposerSig, b.PrepareQC, b.Atts = nil, nil, nil
		b.hashMemoSet = false
		Sign(b, f.keys[0])
		for _, k := range f.keys {
			b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare, f.c.ChainID()))
			b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit, f.c.ChainID()))
		}
	}
	committed := func(f structFixture) ports.Hash { return f.c.Blocks(1)[0].Entries[0].Root }
	fp := ports.HashBytes([]byte("differential issuer key"))

	// A false-repair slash proof against a validator: the claim names leaf 3 of a toy object
	// as a shard it does not commit (or, for the forged accusation, as the one it does).
	falseRepair := func(f structFixture, culprit ed25519.PrivateKey, truthful bool) Equivocation {
		leaves := []ports.Hash{ports.HashBytes([]byte("a")), ports.HashBytes([]byte("b")), ports.HashBytes([]byte("c")), ports.HashBytes([]byte("d"))}
		pr, _ := manifest.Prove(leaves, 3)
		c := repairproof.RepairClaim{Root: manifest.MerkleRoot(leaves), LeafIndex: 3, ShardID: ports.ChunkID(ports.HashBytes([]byte("never committed")))}
		if truthful {
			c.ShardID = ports.ChunkID(leaves[3])
		}
		signed, err := repairproof.SignClaim(culprit, f.c.ChainID(), c)
		if err != nil {
			t.Fatal(err)
		}
		return Equivocation{Culprit: signed.Pub, FalseRepair: &FalseRepairProof{Claim: signed, Leaf: leaves[3], Total: pr.Total, Path: pr.Path}}
	}

	corpus := []tcase{
		{"honest bond registration", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.BondRegs = []BondReg{bondReg(key(99300), twoMiB, b.Prev)}
			})
		}, Accept},
		{"honest false-repair slash", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) { b.Slashes = []Equivocation{falseRepair(f, f.keys[3], false)} })
		}, Accept},
		{"forged false-repair slash (a true claim)", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) { b.Slashes = []Equivocation{falseRepair(f, f.keys[3], true)} })
		}, Reject},
		{"honest entry block", func(f structFixture) Block { return f.mkBlock(t, nil) }, Accept},
		{"honest revocation block", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) { b.Entries = nil; b.Revocations = []ports.Hash{committed(f)} })
		}, Accept},
		{"honest revocation and un-revocation", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{committed(f)} })
		}, Accept},
		{"honest issuer-key-only block", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.Entries = nil
				b.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(f.keys[1], 0, fp)}
			})
		}, Accept},
		{"unbonded issuer key", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.Entries = nil
				b.IssuerKeys = []IssuerKeyReg{SignIssuerKeyReg(key(99101), 0, fp)}
			})
		}, Reject},
		{"honest version-floor advisory", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.Entries = nil
				b.Advisory = signedAdvisory(t, f.c.ChainID(), advisory.Advisory{Seq: 1, Floor: 2, Tier: advisory.TierEnforce, GraceBlocks: 8}, 0, 1)
			})
		}, Accept},
		{"advisory below the maintainers' threshold", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.Advisory = signedAdvisory(t, f.c.ChainID(), advisory.Advisory{Seq: 1, Floor: 2, Tier: advisory.TierEnforce}, 2)
			})
		}, Reject},
		{"advisory signed for another network", func(f structFixture) Block {
			return f.mkBlock(t, func(b *Block) {
				b.Advisory = signedAdvisory(t, ports.HashBytes([]byte("elsewhere")), advisory.Advisory{Seq: 1, Floor: 2, Tier: advisory.TierNotice}, 0, 1)
			})
		}, Reject},
		{"empty block", func(f structFixture) Block { return f.mkBlock(t, func(b *Block) { b.Entries = nil }) }, Reject},
		{"forged proposer signature", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.ProposerSig = append([]byte(nil), b.ProposerSig...)
			b.ProposerSig[0] ^= 1
			return b
		}, Reject},
		{"wrong parent", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Prev = ports.HashBytes([]byte("not the parent"))
			reseal(f, &b)
			return b
		}, Reject},
		{"wrong height", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Height++
			reseal(f, &b)
			return b
		}, Reject},
		{"forged state root", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			bad := ports.HashBytes([]byte("forged state"))
			b.StateRoot = &bad
			reseal(f, &b)
			return b
		}, Reject},
		{"forged log root", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			bad := ports.HashBytes([]byte("forged log"))
			b.LogRoot = &bad
			reseal(f, &b)
			return b
		}, Reject},
		{"below-quorum certificate", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Atts = b.Atts[:1]
			return b
		}, Reject},
		{"certificate signed by strangers", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Atts = nil
			for i := int64(0); i < 4; i++ {
				b.Atts = append(b.Atts, AttestAt(&b, key(99200+i), 0, PhasePrecommit, f.c.ChainID()))
			}
			return b
		}, Reject},
		{"pre-v5 version", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Version = BlockVersionRounds
			reseal(f, &b)
			return b
		}, Reject},
		{"entry mutated after signing", func(f structFixture) Block {
			b := f.mkBlock(t, nil)
			b.Entries = append([]ports.Entry(nil), b.Entries...)
			b.Entries[0].FileSize++
			b.hashMemoSet = false
			return b
		}, Reject},
	}

	sawAccept, sawReject := false, false
	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			f := buildStructFixture(t)
			// The box reads through the PRODUCTION witness provider — the serving path a real
			// box uses — for both its point reads and the block's state-root witness.
			prov, ok := NewWitnessProvider(f.c)
			if !ok {
				t.Fatal("the fixture node must be able to serve witnesses over its own chain")
			}
			box := boxOver(t, f, prov)
			b := tc.build(f)

			node, nerr := ValidateCommitV5(liveView{f.c}, &b)
			if node != tc.want {
				t.Fatalf("corpus: the full node's verdict is %s (%v), the case expects %s — the corpus no longer spans what it claims", node, nerr, tc.want)
			}
			w, werr := prov.Bundle(b)
			if werr != nil {
				// A block the provider cannot apply gets no bundle; the box must then refuse or
				// stall on its own reads, and the differential requires it to refuse.
				w = StateRootWitness{}
			}
			bo, berr := box.Validate(b, w)
			if bo != node {
				t.Fatalf("DIVERGENCE: the full node says %s (%v), the floor box says %s (%v)", node, nerr, bo, berr)
			}
			if node == Accept {
				sawAccept = true
			} else {
				sawReject = true
			}
		})
	}
	if !sawAccept || !sawReject {
		t.Fatalf("the corpus must span both verdicts: accept=%v reject=%v", sawAccept, sawReject)
	}
}
