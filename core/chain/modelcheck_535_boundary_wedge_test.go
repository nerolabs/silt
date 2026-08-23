package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the #535 EPOCH-BOUNDARY LIVENESS CLIFF (field DEEP
// run 45da13c-17686, 2026-08-23). This is the deterministic face of the field
// wedge: h64 (the 8th epoch boundary) never committed for ~1h with all four
// validators on the IDENTICAL h63 head (safety intact, pure liveness), the
// round ladder cycling r1–r5 with no prepare-QC ever forming.
//
// THE MECHANISM (confirmed by code, not hypothesis):
//   - The mature-epoch finality quorum is >⅔ of the FROZEN epoch weight
//     (requireEpochWeightQuorum: `total` sums c.epochSet, the snapshot frozen
//     at the epoch's opening boundary — chain.go ~:2156).
//   - A member whose bond lapses OR who goes offline mid-epoch KEEPS its frozen
//     weight in `total` for the whole epoch (attesterQualified is frozen
//     membership — chain.go :936-946; the design comment there names the exact
//     risk: "a protocol-forced mid-epoch disqualification would shrink the
//     attester supply below the frozen N and could stall the chain before it
//     ever reaches the boundary that rotates it out").
//   - So if members holding > ⅓ of the FROZEN weight are unavailable to sign,
//     NO block can gather the > ⅔ super-quorum — including the epoch-boundary
//     block whose commit is the ONLY thing that rotates the snapshot to a
//     lighter set. The epoch cannot progress and cannot self-heal: a permanent
//     liveness wedge until enough frozen weight returns. The field's
//     `cost-to-corrupt 108 MiB of 324 MiB bonded across 9` is this exactly —
//     324 MiB live against a frozen total that counted the lapsed maturers.
//
// This oracle is a CHARACTERIZATION, not a born-RED-fix-me: > ⅓ offline denying
// a commit IS the BFT liveness bound and is correct for a LIVE set. The finding
// is that the denominator is the FROZEN set, so the effective bound is measured
// against stale membership — a live-1/3 dip can wedge when the frozen set
// over-counts departed members, and the rotation that would fix it is gated
// behind the very quorum the departure denies. Whether to change that (exclude
// provably-lapsed members from the frozen denominator; a boundary-local quorum
// re-basing; an R-gate exemption for a returning frozen member) is a
// consensus-rule call — research-gated (build-process rule 5), consult filed at
// silt-reviews/research/535-epoch-boundary-liveness-cliff-CONSULT.md. This test
// pins the exact arithmetic so the ruling has a deterministic reference.
func TestModelCheck_535_FrozenWeightBoundaryWedge(t *testing.T) {
	const anchorBond = int64(64) << 20
	const maturerBond = int64(64) << 20
	const sybilBond = int64(1) << 20

	ak := make([]ed25519.PrivateKey, 4) // anchors 64M
	mk := make([]ed25519.PrivateKey, 4) // maturers 64M
	yk := make([]ed25519.PrivateKey, 4) // sybils 1M
	for i := range ak {
		ak[i] = key(int64(53500 + i))
		mk[i] = key(int64(53600 + i))
		yk[i] = key(int64(53700 + i))
	}
	anchorID := func(i int) ports.NodeID { return idOf(ak[i]) }
	maturerID := func(i int) ports.NodeID { return idOf(mk[i]) }

	cfg := Config{Quorum: 2, MinBond: sybilBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis seats all 12 (MatureValidators=0 hands off at genesis; the epoch
	// snapshot freezes all 12 — Condition A unfiltered admission, the field
	// topology: 4 anchors + 4 maturers at 64M, 4 sybils at 1M).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	dom := uint64(1)
	for _, k := range ak {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, anchorBond, ports.Hash{}, dom))
		dom++
	}
	for _, k := range mk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, maturerBond, ports.Hash{}, dom))
		dom++
	}
	for _, k := range yk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, sybilBond, ports.Hash{}, 0))
	}
	Sign(g, ak[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if !c.EverMature() || !c.handedOff() {
		t.Fatalf("setup: must hand off at genesis (everMature=%v handedOff=%v)", c.EverMature(), c.handedOff())
	}
	if n := c.validatorSetSize(); n != 12 {
		t.Fatalf("setup: frozen epoch must seat all 12, got %d", n)
	}

	// Frozen weight: 8×64 + 4×1 = 516 MiB. > ⅓ = 172 MiB; the 3 maturers hold
	// 192 MiB. Take them OFFLINE (they never sign). Live-and-signing weight =
	// 516 − 192 = 324 MiB, below the 344 MiB (>⅔) bar — the field's 324 exactly.
	total := int64(0)
	for _, w := range c.epochSet {
		total += w
	}
	if total != (8*anchorBond + 4*sybilBond) {
		t.Fatalf("setup: frozen total = %d, want %d", total, 8*anchorBond+4*sybilBond)
	}

	// THE WEDGE: anchor-0 proposes; the FULL live-and-willing set attests —
	// anchors 1-3, one maturer that stayed up, all 4 sybils. That is 9 of 12
	// members, every one online and honest. Their frozen weight:
	//   proposer a0 (64) + a1,a2,a3 (192) + m0 (64) + 4 sybils (4) = 324 MiB.
	// 3×324 = 972 ≤ 2×516 = 1032 → refused for lack of the frozen super-quorum.
	prev, _ := c.Head()
	b := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b, ak[0])
	b.Atts = append(b.Atts,
		Attest(b, ak[1]), Attest(b, ak[2]), Attest(b, ak[3]), // 192
		Attest(b, mk[0]), // one maturer up: 64
	)
	for _, k := range yk { // 4 sybils: 4
		b.Atts = append(b.Atts, Attest(b, k))
	}
	err := c.Append(*b)
	if err == nil {
		t.Fatal("#535 NOT REPRODUCED: a 9-of-12 live coalition committed — the frozen-weight cliff did not bite (check the weight arithmetic)")
	}
	if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("#535: the refusal must be the frozen-weight rule, got: %v", err)
	}

	// THE NO-SELF-HEAL PROPERTY: every height in the epoch — including the
	// boundary block whose commit would rotate the snapshot to a set WITHOUT the
	// 3 offline maturers — is subject to the same > ⅔-frozen bar. So the epoch
	// cannot rotate out of the wedge; the live majority is stuck until the
	// offline weight returns. SupportMeetsQuorum agrees (the client-visible
	// predicate must not disagree with ValidateCommit — the #402 lesson).
	if c.SupportMeetsQuorum(anchorID(0), []ports.NodeID{
		anchorID(1), anchorID(2), anchorID(3), maturerID(0),
		idOf(yk[0]), idOf(yk[1]), idOf(yk[2]), idOf(yk[3]),
	}, 1) {
		t.Fatal("#535: SupportMeetsQuorum says the 9-of-12 live coalition is sufficient, but ValidateCommit refused it — the two must agree")
	}

	// THE CONTROL (this is a cliff, not a total break): if just ONE more
	// maturer is up (m1, +64 → 388 MiB live), the > ⅔ bar (344) is cleared and
	// the height commits. So the wedge is precisely "> ⅓ of the FROZEN weight
	// offline", and the fix space is about the DENOMINATOR (should a departed
	// member's frozen weight keep counting?), not the quorum rule itself.
	b2 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(b2, ak[0])
	b2.Atts = append(b2.Atts,
		Attest(b2, ak[1]), Attest(b2, ak[2]), Attest(b2, ak[3]),
		Attest(b2, mk[0]), Attest(b2, mk[1]), // TWO maturers up now: 128
	)
	for _, k := range yk {
		b2.Atts = append(b2.Atts, Attest(b2, k))
	}
	if err := c.Append(*b2); err != nil {
		t.Fatalf("#535 control: with >⅔ frozen weight live (388 of 516 MiB) the height MUST commit, got: %v", err)
	}
}
