package chain

import (
	"crypto/ed25519"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// eraProbeChain builds the chain the era-observable gates drive: one whale and three minnows,
// bonded on-chain, an epoch cadence of 4, and NO activation override. Every bond registration
// carries the readiness stamp given, which is the ONE input that decides whether the readiness
// tally locks an era in.
//
// THE FIXTURE DOES NOT SUPPLY THE ANSWER. It supplies four signed bond registrations and lets the
// chain decide; era4LockedIn, era4Height, every block's Version and every entry of every Atts
// carrier are produced by rotateEpoch, MintVersion and Attest, not written by this function. That
// distinction is the whole point of this file — see the anti-vacuity note on EraPhase.
//
// The stamp of 5 cannot come from NewBondReg, which hard-codes BlockVersionRegGate because the
// readiness signal is a property of the binary rather than a caller's choice. That is why these
// gates live in core/chain and not in cmd/silt: no cmd-tier fixture can drive a real era-4 latch.
//
// THE WEIGHT SPLIT IS DELIBERATE. The whale holds 32 MiB against three 2 MiB minnows, so the whale
// ALONE clears the >⅔ frozen-weight bar (32 of 38 MiB). With four equal bonds no coalition below
// three could commit, and the attestation-carrier widths this file needs — 1 through 4 — would be
// unreachable, leaving a near-uniform distribution under which a wrong reducer still looks right.
func eraProbeChain(t *testing.T, stamp uint8) (*Chain, ed25519.PrivateKey, []ed25519.PrivateKey) {
	t.Helper()
	whale := key(77001)
	minnows := []ed25519.PrivateKey{key(77002), key(77003), key(77004)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondRegV(whale, 32<<20, ports.Hash{}, stamp))
	for _, m := range minnows {
		g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, stamp))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, whale, minnows
}

// sameEra reports whether two era statuses are equal BY VALUE. EraStatus carries *uint64 heights,
// so Go's == compares ADDRESSES: two calls to EraState on an unchanged chain return statuses that
// are `!=` every time. Writing the ablation below with == would have made it fire on every run —
// red for a reason that has nothing to do with the property. It did, on the first run, which is
// how this helper came to exist.
func sameEra(a, b EraStatus) bool {
	eq := func(x, y *uint64) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return *x == *y
	}
	return a.Version == b.Version && a.Phase == b.Phase && a.LockedIn == b.LockedIn &&
		eq(a.ActivationHeight, b.ActivationHeight) && eq(a.FirstHeight, b.FirstHeight)
}

// showEra renders an era status with its heights DEREFERENCED. %+v on an EraStatus prints pointer
// addresses, which turns the ablation's failure message into two lines of hex that say nothing
// about which field moved.
func showEra(s EraStatus) string {
	h := func(p *uint64) string {
		if p == nil {
			return "absent"
		}
		return strconv.FormatUint(*p, 10)
	}
	return fmt.Sprintf("v%d phase=%s lockedIn=%v activationHeight=%s firstHeight=%s",
		s.Version, s.Phase, s.LockedIn, h(s.ActivationHeight), h(s.FirstHeight))
}

// TestEraStateDrivesEveryPhaseFromCommittedState is GATE G-EP-1 and GATE G-EP-4 together
// (cold chain). It is the answer to the defect this row absorbs.
//
// R-CARRIER-ROLLOUT-SIGNAL's code half was vacuous because its gate took its guard condition from
// its own subject and returned before asserting anything. The transposition of that defect into an
// observable is a phase that is only ever one value, so that "the era is dark" and "the field was
// never populated" render identically. This gate closes it by DRIVING all three phases on one real
// chain and asserting they are pairwise distinct — no phase is read off a constant, and none is
// asserted from a fixture-written field.
//
// The three phases are driven on two chains, split by the ONE input that decides the tally:
//
//	dark    — a fleet stamped 3, which is what every SHIPPED binary stamps today (NewBondReg
//	          hard-codes BlockVersionRegGate). This is the live networks' actual state, and the
//	          state cloud row 13b has been unable to name.
//	pending — a fleet stamped 5: the tally locks in and names H_era4 while no v5 block exists yet
//	          (the one epoch of notice, which a block-only view cannot see at all)
//	active  — once the first v5 block commits
func TestEraStateDrivesEveryPhaseFromCommittedState(t *testing.T) {
	// --- DARK, driven on the SHIPPED stamp. ---
	dark, _, _ := eraProbeChain(t, BlockVersionRegGate)
	st := dark.EraState()
	if st.Era4.Phase != EraDark {
		t.Fatalf("G-EP-1 RED: a fleet stamped %d must report era-4 %q, got %q",
			BlockVersionRegGate, EraDark, st.Era4.Phase)
	}
	if st.Era4.LockedIn || st.Era4.ActivationHeight != nil || st.Era4.FirstHeight != nil {
		t.Fatalf("G-EP-1 RED: dark must carry NO heights (lockedIn %v, activation %v, first %v). "+
			"A present zero here is unreadable against a real height 0.",
			st.Era4.LockedIn, st.Era4.ActivationHeight, st.Era4.FirstHeight)
	}

	// --- PENDING. A fleet stamped 5: the tally locks in and names H_era4 at the genesis
	// rotation, before any v5 block can exist.
	c, whale, minnows := eraProbeChain(t, BlockVersionWitnessable)
	st = c.EraState()
	if !st.Era4.LockedIn {
		t.Fatal("FIXTURE: the readiness tally did not lock era-4 in — the rest of this gate would be vacuous")
	}
	if st.Era4.Phase != EraPending {
		t.Fatalf("G-EP-1 RED: locked in with no v5 block committed must report %q, got %q. "+
			"Rendering this window as %q is the defect: it is the state an operator watching a "+
			"stamp raise land most needs to see.", EraPending, st.Era4.Phase, EraDark)
	}
	if st.Era4.ActivationHeight == nil {
		t.Fatal("G-EP-1 RED: pending must name its activation height")
	}
	if got := *st.Era4.ActivationHeight; got != c.era4Height {
		t.Fatalf("G-EP-1 RED: reported activation height %d != the chain's H_era4 %d", got, c.era4Height)
	}
	if st.Era4.FirstHeight != nil {
		t.Fatalf("G-EP-1 RED: no v5 block is committed, so firstHeight must be ABSENT, got %d", *st.Era4.FirstHeight)
	}
	pendingAt := *st.Era4.ActivationHeight

	// --- ACTIVE. Commit past H_era4; the chain mints v5 and the phase advances. ---
	for c.Len() <= int(c.era4Height) {
		mustAppend(t, c, mintNext4Carrier(t, c, append([]ed25519.PrivateKey{whale}, minnows...)))
	}
	st = c.EraState()
	if st.Era4.Phase != EraActive {
		t.Fatalf("G-EP-1 RED: a committed v5 block must report %q, got %q", EraActive, st.Era4.Phase)
	}
	if st.Era4.FirstHeight == nil {
		t.Fatal("G-EP-1 RED: active must name the first v5 height")
	}
	if got := *st.Era4.FirstHeight; got != pendingAt {
		t.Fatalf("G-EP-1 RED: the first v5 block landed at height %d but the tally named %d. "+
			"Activation is a MINT boundary, so these are the same fact reached two ways; a "+
			"difference means one of the two readings is wrong.", got, pendingAt)
	}
	if st.Census.HeadVersion != BlockVersionWitnessable {
		t.Fatalf("G-EP-1 RED: head version must be v5, got v%d", st.Census.HeadVersion)
	}

	// --- G-EP-4: the three phases this chain produced are pairwise distinct and non-empty, and
	// the closed set has no member that no driven arm reached. A phase set where two members
	// render alike would make the observable answer nothing.
	seen := map[EraPhase]bool{EraDark: true, EraPending: true, EraActive: true}
	if len(seen) != len(EraPhases) {
		t.Fatalf("G-EP-4 RED: %d phases driven, %d in the closed set EraPhases — an undriven phase "+
			"is a row that has never been shown to render differently from its neighbours",
			len(seen), len(EraPhases))
	}
	for _, p := range EraPhases {
		if p == "" {
			t.Fatal("G-EP-4 RED: a phase is the empty string, which is the Go zero value — " +
				"a never-populated field would be indistinguishable from it")
		}
		if !seen[p] {
			t.Fatalf("G-EP-4 RED: phase %q is in the closed set but no arm of this gate drove it", p)
		}
	}
}

// TestEraStateIgnoresDivergentLocalConfig is GATE G-EP-2 (cold chain), the ABLATION that gives the
// "committed state, not local config" claim teeth.
//
// The era activation state has two routes: the genesis override (Era3ActivationHeight /
// Era4ActivationHeight) and the readiness-tally latch. Reading the override off the local Config
// would be the #380 class — a consensus quantity answered from what an operator typed. EraState
// reads Config NOWHERE, so this ablation is total rather than targeted: mutate every era-related
// Config field on an ALREADY-LATCHED chain, with the committed blocks untouched, and the reported
// era state must not move by one field.
//
// The mutation is applied WITHOUT re-applying any block on purpose. Re-applying would change the
// latch legitimately (rotateEpoch skips the tally when an override is set), which would conflate
// "the accessor reads config" with "the config changed the rules". This isolates the accessor,
// which is the line being shipped.
func TestEraStateIgnoresDivergentLocalConfig(t *testing.T) {
	c, _, _ := eraProbeChain(t, BlockVersionWitnessable)
	before := c.EraState()
	if !before.Era3.LockedIn || !before.Era4.LockedIn {
		t.Fatal("FIXTURE: both tallies must have locked in, or this ablation compares two darks")
	}

	// The divergent local reading. 999999 is nowhere near either tally height, so an accessor
	// that consulted Config could not accidentally agree.
	c.cfg.Era3ActivationHeight = 999999
	c.cfg.Era4ActivationHeight = 999999
	c.cfg.EpochBlocks = 4096
	after := c.EraState()

	if !sameEra(before.Era3, after.Era3) || !sameEra(before.Era4, after.Era4) {
		t.Fatalf("G-EP-2 RED: the reported era state MOVED when local config changed, on identical "+
			"committed blocks. That is the #380 class — the surface would answer \"what did this "+
			"operator type\", not \"what does the chain say\".\n  era3 %s\n    -> %s\n  era4 %s\n    -> %s",
			showEra(before.Era3), showEra(after.Era3), showEra(before.Era4), showEra(after.Era4))
	}
	if *after.Era4.ActivationHeight == 999999 {
		t.Fatal("G-EP-2 RED: the reported H_era4 is the LOCAL config value verbatim")
	}
}

// TestCensusMeasuresMaxAttsOnANonUniformChain is GATE G-EP-3 (cold chain). max_h len(blocks[h].Atts)
// is the live attestation-carrier width, a figure two certification items name as unmeasured and
// which no shipped command produced before this row.
//
// THE DISTRIBUTION IS NON-UNIFORM AND THE MAXIMUM IS IN THE INTERIOR, deliberately. On a uniform
// chain every wrong reducer looks right: "the head's width", "the first block's width" and "the
// maximum" all agree, so the gate would pass while measuring the wrong thing. Here the widths run
// 0, 1, 3, 4, 2 — the maximum is neither first nor last — and the attesting coalitions are driven
// through Attest on real blocks rather than written into a fixture.
//
// THE FLEET IS STAMPED 3, which is what every shipped binary stamps, so no era boundary lands
// inside the measured window and the figure is taken on a chain shaped like the live networks
// rather than on a hypothetical era-4 one.
func TestCensusMeasuresMaxAttsOnANonUniformChain(t *testing.T) {
	c, whale, minnows := eraProbeChain(t, BlockVersionRegGate)
	all := append([]ed25519.PrivateKey{whale}, minnows...)

	// Widths per height, by how many of the four bonded keys attest. Height 0 (genesis) carries
	// none. The whale alone clears the >⅔ frozen-weight bar, so every coalition below commits.
	widths := []int{1, 3, 4, 2}
	for _, n := range widths {
		commit(t, c, whale, all[:n])
	}

	got := c.EraState().Census
	if got.MaxAtts != 4 {
		t.Fatalf("G-EP-3 RED: max atts is %d, want 4. The widths committed were %v across heights "+
			"1..%d; a reducer that took the head (%d), the first (%d) or a count of blocks would "+
			"differ from the maximum, which is why this chain is non-uniform.",
			got.MaxAtts, widths, len(widths), widths[len(widths)-1], widths[0])
	}
	if got.MaxAttsHeight != 3 {
		t.Fatalf("G-EP-3 RED: the widest block is height 3, reported %d", got.MaxAttsHeight)
	}
	if !got.AttsMeasured {
		t.Fatal("G-EP-3 RED: attsMeasured must be true on a walked chain — it is what separates " +
			"a measured zero from a figure that was never computed")
	}
	t.Logf("MEASURED live carrier width on a driven four-validator chain: max_h len(blocks[h].Atts) "+
		"= %d at height %d, across %d blocks (widths per height 1..%d: %v)",
		got.MaxAtts, got.MaxAttsHeight, got.Blocks, len(widths), widths)

	// The measured ZERO is a distinct answer, not an absence. A genesis-only chain legitimately
	// carries no attestation, and that must not render as "never computed".
	empty := CensusOf(c.Blocks(0)[:1])
	if empty.MaxAtts != 0 || !empty.AttsMeasured {
		t.Fatalf("G-EP-3 RED: a genesis-only chain must report a MEASURED zero (maxAtts %d, measured %v)",
			empty.MaxAtts, empty.AttsMeasured)
	}
	if none := CensusOf(nil); none.AttsMeasured || !none.Empty() {
		t.Fatalf("G-EP-3 RED: no blocks at all must report NOT measured and Empty (measured %v, empty %v). "+
			"This is the absent-versus-zero distinction the row exists to make.", none.AttsMeasured, none.Empty())
	}
}

// TestEraLineDistinguishesAnUnobservableTallyFromADarkOne is GATE G-EP-5's core-tier half.
//
// The offline `silt chain-status` path holds a []Block and no Chain, so the readiness-tally latch
// is genuinely out of its reach. Printing "lockedIn: false" there would be a lie dressed as a zero
// — exactly the shape of the defect this row absorbs. EraLine(false) must therefore render
// differently from EraLine(true), and must say what it cannot see rather than asserting a value.
func TestEraLineDistinguishesAnUnobservableTallyFromADarkOne(t *testing.T) {
	dark := EraStatus{Version: BlockVersionWitnessable, Phase: EraDark}
	withTally, withoutTally := dark.EraLine(true), dark.EraLine(false)
	if withTally == withoutTally {
		t.Fatalf("G-EP-5 RED: a caller that CAN see the tally and one that CANNOT render the same "+
			"line %q. The second would be asserting a fact it has no access to.", withTally)
	}
	if !strings.Contains(withTally, "DARK") {
		t.Fatalf("G-EP-5 RED: a tally-visible dark era must say DARK, got %q", withTally)
	}
	if strings.Contains(withoutTally, "DARK") {
		t.Fatalf("G-EP-5 RED: an offline caller must NOT claim DARK — it cannot see the tally. Got %q", withoutTally)
	}
	if !strings.Contains(withoutTally, "/api/status") {
		t.Fatalf("G-EP-5 RED: the offline line must name where the tally IS observable, got %q", withoutTally)
	}
	// Every line, in every phase, on both callers, is non-empty and names its era. An empty
	// render is the vacuity: it would look the same whether the state was computed or forgotten.
	h := uint64(0)
	for _, s := range []EraStatus{
		dark,
		{Version: BlockVersionWitnessable, Phase: EraPending, LockedIn: true, ActivationHeight: &h},
		{Version: BlockVersionWitnessable, Phase: EraActive, FirstHeight: &h},
	} {
		for _, visible := range []bool{true, false} {
			line := s.EraLine(visible)
			if line == "" || !strings.Contains(line, "era-4 (v5)") {
				t.Fatalf("G-EP-5 RED: phase %q visible=%v rendered %q", s.Phase, visible, line)
			}
		}
	}
}
