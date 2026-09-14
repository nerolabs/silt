package chain

import (
	"crypto/ed25519"
	"testing"
)

// Every rule that decides whether one history replaces another compares blocks by HASH.
// The finality gate refuses a fork unless the fork's block at our head's index hashes
// equal to ours; fork-choice orders candidates by height and head hash; a checkpoint pins
// a height to a hash. So a field the hash does not cover is a field those rules cannot
// see, and two histories that differ only in such a field are, to every one of them, the
// same history.
//
// That is safe for a field nothing reads. It is not safe for a TRANSITION INPUT — a field
// the apply path feeds into committed state. The seating map is exactly that: it decides
// which operators the chain has seen attesting, which sets the maturity coefficient, which
// trips the one-way shed that takes the launch anchors' bond-free eligibility away. If the
// seating is read from a field outside the hash, an adversary can hand us our own history
// back, block for block, hashing identically at every index, carrying a different seating —
// and the gates that exist to refuse a rewritten history have nothing to compare.
//
// A block cannot cover its own attestations: they are signatures OVER its hash. The
// resolution is to take the transition input from a field that can be covered — the
// witnessable format's carrier of precommits over the PARENT, which is folded into the
// hash and known before signing.

// stripSeating returns b with the non-anchor signers removed from whichever field this
// era reads the seating from, and the memoized hash cleared so the digest is recomputed
// from the body the block now holds. Below the witnessable format the seating comes from
// the block's own attestations; at that format it comes from the carrier.
func stripSeating(b Block, anchors map[string]bool) (Block, int) {
	keep := func(in []Attestation) ([]Attestation, int) {
		var out []Attestation
		for _, a := range in {
			if anchors[string(a.PubKey)] {
				out = append(out, a)
			}
		}
		return out, len(in) - len(out)
	}
	removed := 0
	if b.Version >= BlockVersionWitnessable {
		var n int
		b.LastCommit, n = keep(b.LastCommit)
		removed += n
	} else {
		var n int
		b.Atts, n = keep(b.Atts)
		removed += n
		b.PrepareQC, _ = keep(b.PrepareQC)
	}
	b.hashMemoSet = false
	return b, removed
}

// TestSeatingIsHashCovered asserts that the seating a block performs cannot be changed
// without changing the block's hash — so a rewritten history is visible to every rule
// that compares histories by hash.
func TestSeatingIsHashCovered(t *testing.T) {
	for _, era := range seamEras {
		t.Run(era.name, func(t *testing.T) {
			w := newSeamEra(t, 3, 4, era.era3, era.era4, 0)
			anchors := map[string]bool{}
			for _, k := range w.ak {
				anchors[string(k.Public().(ed25519.PublicKey))] = true
			}

			// Commit a block that seats the bonded validators — the transition input
			// under test. A format that cannot do this at all is covered by its own
			// gate; here it means there is nothing to strip.
			var seated *Block
			for h := 0; h < 4 && seated == nil; h++ {
				b := w.mint(t, w.ak, w.vk, byte(0x30+h))
				before := len(w.c.validatorsSeen)
				if err := w.c.Append(*b); err != nil {
					if era.mintsVersion != BlockVersionWitnessable {
						t.Logf("v%d admits no first-time attester (%v), so its seating cannot be exercised — "+
							"one of the two reasons the daemon refuses to run any height on this format",
							b.Version, err)
						return
					}
					t.Fatalf("the witnessable format failed to seat a validator (%v) — this gate cannot run", err)
				}
				if len(w.c.validatorsSeen) > before {
					seated = b
				}
			}
			if seated == nil {
				t.Fatal("GATE VACUOUS: no committed block seated anyone, so no transition input was exercised")
			}

			stripped, removed := stripSeating(*seated, anchors)
			if removed == 0 {
				t.Fatalf("GATE VACUOUS: stripping removed nothing from the block's seating field — the variant "+
					"is the original and this asserts nothing")
			}

			notCovered := stripped.Hash() == seated.Hash()
			if era.mintsVersion != BlockVersionWitnessable {
				// The pre-witnessable formats are the ones this property fails on, and
				// the daemon refuses to run any height on them for exactly this reason.
				// Pinned rather than fixed: a block cannot cover its own attestations,
				// because they are signatures over its hash.
				if !notCovered {
					t.Fatalf("v%d now covers its seating in the block hash. The daemon refuses this format "+
						"partly because it did not, so the refusal should be revisited rather than left to "+
						"outlive its reason.", seated.Version)
				}
				t.Logf("v%d: removing %d seating signatures leaves the hash unchanged — the reason the daemon "+
					"refuses to run any height on this format", seated.Version, removed)
				return
			}
			if notCovered {
				t.Fatalf("THE SEATING IS NOT HASH-COVERED AT v%d — the same block with %d seating signatures "+
					"removed hashes IDENTICALLY (%x). The removed signers are exactly the ones the apply path "+
					"seats, so a history carrying this variant commits a different seating map, a different "+
					"maturity coefficient, and a different verdict on whether the launch anchors have shed — "+
					"while matching our chain hash for hash at every index. The finality gate, fork-choice and "+
					"the weak-subjectivity checkpoint all compare histories by hash, so none of them can tell "+
					"the two apart. A transition input must come from a field the hash covers; attestations "+
					"cannot be, because they sign over that hash.",
					seated.Version, removed, func() []byte { h := seated.Hash(); return h[:8] }())
			}
			t.Logf("v%d: removing %d seating signatures changes the block hash — a rewritten seating is visible "+
				"to every rule that compares by hash", seated.Version, removed)
		})
	}
}
