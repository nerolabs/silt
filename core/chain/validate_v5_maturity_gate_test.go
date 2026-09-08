package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// M-1A-1 (research certification FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e §0): v5MatureNow
// must call the ONE receiverless nakamotoCoefficient, not carry a third body of the coefficient
// arithmetic. This gate is the maturity mirror's direct parity oracle: over a table of committed
// worlds that exercise every branch of the fold — the degenerate total, equal bonds, a whale, a
// shared domain, the operator margin, and the three exclusions (anchor / slashed / below MinBond) —
// v5MatureNow over liveView must answer exactly what the node's matureNow answers.
//
// The worlds are written straight into the chain's committed maps (validatorsSeen, bonded,
// bondDomain, slashed): the node's matureNow reads those maps and nothing else, and the mirror
// reads the same maps through liveView, so the comparison is between the two BODIES, which is the
// property M-1A-1 protects. The accept-path driver for the same mirror is the v4/v5 parity oracle
// (parity_oracle_v4v5_test.go, the de-mature regime).
//
// Ablation (M-1A-1): reintroduce a divergent inline coefficient in v5MatureNow (e.g. `cum >=
// threshold`) ⇒ the "three equal bonds, MatureValidators 2" row reddens (node 2, mirror 1).
func TestM1A1_V5MatureNowEqualsNodeMatureNow(t *testing.T) {
	f := buildStructFixture(t)
	assertHonestTwinAccepts(t, f.c, f.mkBlock(t, nil))
	c := f.c
	const mib = int64(1) << 20
	anchor := idOf(f.keys[0])
	ids := make([]ports.NodeID, 8)
	for i := range ids {
		ids[i] = idOf(key(int64(77100 + i)))
	}

	type member struct {
		id      ports.NodeID
		bond    int64
		domain  uint64
		slashed bool
	}
	type world struct {
		name    string
		members []member
		mv      int
		margin  int
		want    bool
	}
	worlds := []world{
		{"no qualifying weight, MatureValidators 0", nil, 0, 0, true},
		{"no qualifying weight, MatureValidators 1", nil, 1, 0, false},
		{"three equal bonds, MatureValidators 2", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, false}}, 2, 0, true},
		{"three equal bonds, MatureValidators 3", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, false}}, 3, 0, false},
		{"a whale concentrates the weight", []member{{ids[0], 32 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, false}, {ids[3], 2 * mib, 0, false}}, 2, 0, false},
		{"four bonds in ONE declared domain (A axis)", []member{{ids[0], 2 * mib, 7, false}, {ids[1], 2 * mib, 7, false}, {ids[2], 2 * mib, 7, false}, {ids[3], 2 * mib, 7, false}}, 2, 0, false},
		{"four bonds in one domain clears MatureValidators 1", []member{{ids[0], 2 * mib, 7, false}, {ids[1], 2 * mib, 7, false}, {ids[2], 2 * mib, 7, false}, {ids[3], 2 * mib, 7, false}}, 1, 0, true},
		{"six equal bonds under OperatorMargin 2", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, false}, {ids[3], 2 * mib, 0, false}, {ids[4], 2 * mib, 0, false}, {ids[5], 2 * mib, 0, false}}, 2, 2, false},
		{"six equal bonds under OperatorMargin 1", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, false}, {ids[3], 2 * mib, 0, false}, {ids[4], 2 * mib, 0, false}, {ids[5], 2 * mib, 0, false}}, 2, 1, true},
		{"a slashed member does not count", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], 2 * mib, 0, true}}, 2, 0, false},
		{"a below-MinBond member does not count", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {ids[2], mib / 2, 0, false}}, 2, 0, false},
		{"an anchor does not count", []member{{ids[0], 2 * mib, 0, false}, {ids[1], 2 * mib, 0, false}, {anchor, 2 * mib, 0, false}}, 2, 0, false},
	}
	for _, w := range worlds {
		// Reset the maturity inputs to the fixture's committed state, then write the world.
		c.validatorsSeen = map[ports.NodeID]bool{}
		for _, id := range ids {
			delete(c.bonded, id)
			delete(c.bondDomain, id)
			delete(c.slashed, id)
		}
		for _, m := range w.members {
			c.validatorsSeen[m.id] = true
			c.bonded[m.id] = m.bond
			if m.domain != 0 {
				c.bondDomain[m.id] = m.domain
			}
			if m.slashed {
				c.slashed[m.id] = true
			}
		}
		c.cfg.MatureValidators, c.cfg.OperatorMargin = w.mv, w.margin

		node := c.matureNow()
		mirror, out, err := v5MatureNow(liveView{c})
		if out != Accept || err != nil {
			t.Fatalf("M-1A-1 (%s): v5MatureNow over liveView must not stall; got %s / %v", w.name, out, err)
		}
		if node != w.want {
			t.Fatalf("M-1A-1 (%s): the world is mis-specified — the node's matureNow answers %v, the table expects %v", w.name, node, w.want)
		}
		if mirror != node {
			t.Fatalf("M-1A-1 VIOLATED (%s): the node's matureNow answers %v, the composition's v5MatureNow answers %v — "+
				"the maturity coefficient has a second body; v5MatureNow must call nakamotoCoefficient", w.name, node, mirror)
		}
	}
}
