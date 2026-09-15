package chain

import "testing"

// A network that cannot seat a validator it has never seen cannot grow, and a network
// that cannot grow cannot mature. Maturity is measured over the operator- and
// domain-distinct participants the chain has actually seen attesting, so if a block is
// unable to admit a first-time attester the coefficient is frozen at whatever the launch
// set produced. The training wheels then never shed, and the launch anchors keep their
// bond-free eligibility for the life of the network — a permanent center reached through
// a format rule rather than through anyone's decision.
//
// The tension is structural, and naming it is the point of this gate. The committed state
// root covers the seating map. Below the witnessable format the seating is read from the
// block's own attestations, which are signatures over the hash that covers that root — so
// an honest proposer cannot commit a root reflecting seating its own block causes. The
// witnessable format breaks the cycle by moving the seating to a carrier of precommits
// over the PARENT, which is folded into the hash and is therefore known before signing.
func TestValidatorSetCanGrowInEveryEra(t *testing.T) {
	for _, era := range seamEras {
		t.Run(era.name, func(t *testing.T) {
			w := newSeamEra(t, 3, 4, era.era3, era.era4, 0)

			// Seat the launch set first, so the follow-up block is a clean test of
			// admitting a NEW attester rather than of admitting the very first one.
			// A format that cannot even do this has already failed the property.
			for h := 0; h < 3; h++ {
				b := w.mint(t, w.ak, w.ak, byte(0x10+h))
				if err := w.c.Append(*b); err != nil {
					if era.mintsVersion == BlockVersionStateRoot {
						t.Logf("v%d cannot seat even its own launch set (%v) — the reason the daemon refuses "+
							"to run any height on this format", b.Version, err)
						return
					}
					t.Fatalf("THE VALIDATOR SET CANNOT GROW AT v%d — the chain cannot seat even its own launch "+
						"set: the block at height %d is REFUSED: %v. This format admits no first-time attester "+
						"at all.", b.Version, b.Height, err)
				}
			}
			seenBefore := len(w.c.validatorsSeen)
			if seenBefore == 0 {
				t.Fatalf("GATE VACUOUS at v%d: the launch set was never seated, so this asserts nothing about growth",
					era.mintsVersion)
			}

			// Admit the bonded validators — none of them seen until now.
			b := w.mint(t, w.ak, w.vk, 0x20)
			if b.Version != era.mintsVersion {
				t.Fatalf("GATE VACUOUS: expected a v%d block, minted v%d — this subtest is not exercising its era",
					era.mintsVersion, b.Version)
			}
			err := w.c.Append(*b)
			if era.mintsVersion == BlockVersionStateRoot {
				// The state-root format is the one this property cannot hold on, and
				// the daemon refuses to run any height on it. Pinned rather than
				// fixed: if it ever starts growing, the refusal should be revisited
				// instead of quietly outliving its reason.
				if err == nil {
					t.Fatalf("v%d now seats a first-time attester. The daemon refuses this format precisely "+
						"because it could not, so the refusal has outlived its reason and should be revisited.",
						b.Version)
				}
				t.Logf("v%d cannot seat a first-time attester (%v) — the reason the daemon refuses to run any "+
					"height on this format", b.Version, err)
				return
			}
			if err != nil {
				t.Fatalf("THE VALIDATOR SET CANNOT GROW AT v%d — a block admitting %d first-time attesters is "+
					"REFUSED: %v. The committed state root covers the seating map, the seating is derived from "+
					"this block's own attestations, and those attestations sign over the hash that covers the "+
					"root — so no honest proposer can commit a root reflecting the seating its block causes. A "+
					"chain on this format can only ever re-seat validators it has already seen, so the maturity "+
					"coefficient is frozen, the shed never fires, and the launch anchors hold bond-free "+
					"eligibility permanently.", b.Version, len(w.vk), err)
			}
			if grew := len(w.c.validatorsSeen) - seenBefore; grew <= 0 {
				t.Fatalf("the block was accepted at v%d but seated nobody (%d seen before, %d after) — the set "+
					"is not growing, it is only reporting that it did", b.Version, seenBefore, len(w.c.validatorsSeen))
			}
			t.Logf("v%d: the set grew from %d to %d seen, coefficient %d",
				b.Version, seenBefore, len(w.c.validatorsSeen), w.c.MatureCoefficient())
		})
	}
}
