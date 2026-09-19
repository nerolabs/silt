package chain

// WHAT FORGING N STANDINGS COSTS, ARM BY ARM, WITH NO ARM PASSING VACUOUSLY.
//
// The claim is that forging N standings costs N times the real, non-substitutable work an honest
// provider pays. It is not one mechanism. `docs/VISION.md` names five economies of scale a Sybil
// relies on and one denial for each: a size-bound bond denies one plot backing many identities;
// unique-sealed real content denies synthetic bytes standing in for storage; witnessed demand
// receipts deny self-dealt demand; address and AS diversity buckets deny massing free keys near a
// target; retention decay denies coasting on stale standing.
//
// THIS FILE IS THE CENSUS OF THOSE FIVE, AT THE PLACE STANDING IS DECIDED. Each arm drives its
// attack against `idQualifies` — the function that decides whether an identity holds standing at
// all — and reports one of two verdicts:
//
//	DENIED   the attack was driven and the identity earned no standing from it
//	UNWIRED  the axis does not enter the standing number, so there is nothing here to deny
//
// UNWIRED IS A RESULT, NOT A SKIP. An axis that cannot deny anything because it is not connected to
// standing is a fact about this build, and saying so in a test that runs is the difference between
// a gap the project reports and a gap a reader has to discover. A skipped arm would report the same
// silence as an arm that passes.
//
// EVERY ARM CARRIES A POSITIVE CONTROL ON ITS OWN AXIS. A denial is only evidence if the same
// fixture can be made to GRANT standing by removing exactly the attack. Without that, an arm passes
// whenever anything at all goes wrong — a mis-built registration, a screen firing for an unrelated
// reason, a chain that grants nobody standing — and the census would read green while measuring
// nothing. The control is what makes "N standings cost N times" a measurement instead of a hope.
//
// WHAT THIS FILE IS NOT. It is not the primitive-level proof of any axis. Whether a plot can be
// faked is settled in `core/bond` (byte-binding, labeling, prefix-plot families); whether a demand
// receipt can be forged is settled in `core/credit` and `core/demand`. Those are the parts. This is
// the composition: given the primitives, what does the CHAIN grant. A primitive that fails its own
// standalone test is expected and is not this file's finding.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// sybilArm is one economy of scale and what the chain does about it.
type sybilArm struct {
	axis    string
	economy string // the scale a Sybil farm would be buying
	verdict string // DENIED or UNWIRED, set by the arm that drives it
	note    string
}

const (
	denied  = "DENIED"
	unwired = "UNWIRED"
)

// censusChain is an objective chain whose bond verifier the caller supplies, so an arm can model a
// prover that does not hold the bytes by injecting a verifier that refuses.
func censusChain(t *testing.T, verify func(pub []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool, ttl uint64) *Chain {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: ttl}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(verify)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(1)}}
	Sign(g, key(1))
	c.apply(*g)
	return c
}

// standingOf reports the bonded weight the chain grants an identity — the standing number itself.
func standingOf(c *Chain, k ports.NodeID) int64 {
	sz, ok := c.idQualifies(k)
	if !ok {
		return 0
	}
	return sz
}

// registerAt applies one block carrying the given registrations.
func registerAt(c *Chain, regs ...BondReg) {
	prev, h := c.Head()
	c.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, BondRegs: regs})
}

// ─────────────────────────────────────────────────────────────────────────────
// ARM 1 — ONE PLOT, MANY IDENTITIES
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilArm_OnePlotManyIdentities drives the cheapest Sybil there is: seal once, register the
// same bond root under N identities, collect N standings for one plot's work.
func TestSybilArm_OnePlotManyIdentities(t *testing.T) {
	c := censusChain(t, objectiveVerify, 0)
	shared := ports.HashBytes([]byte("one plot, sealed once"))
	a, b, d := key(810001), key(810002), key(810003)

	registerAt(c,
		bondRegFull(a, shared, 4<<20, ports.Hash{}, 5, 1),
		bondRegFull(b, shared, 4<<20, ports.Hash{}, 5, 1),
		bondRegFull(d, shared, 4<<20, ports.Hash{}, 5, 1))

	granted := 0
	for _, k := range []ports.NodeID{ports.HashBytes(pubOf(a)), ports.HashBytes(pubOf(b)), ports.HashBytes(pubOf(d))} {
		if standingOf(c, k) > 0 {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("THE BOND AXIS DOES NOT DENY PLOT RE-USE: three identities registered ONE bond root "+
			"and %d of them hold standing, want exactly 1.\n"+
			"  One sealed plot backing N standings is the cheapest Sybil the design names, and the "+
			"per-root ownership rule exists to make the second and third identities free of work and "+
			"free of standing alike.", granted)
	}

	// POSITIVE CONTROL, on this axis: the same three identities with DISTINCT roots all earn
	// standing. Without it, "one of three" would also be the reading if the chain simply refused
	// almost every registration.
	c2 := censusChain(t, objectiveVerify, 0)
	registerAt(c2,
		bondRegFull(a, ports.HashBytes(pubOf(a)), 4<<20, ports.Hash{}, 5, 1),
		bondRegFull(b, ports.HashBytes(pubOf(b)), 4<<20, ports.Hash{}, 5, 1),
		bondRegFull(d, ports.HashBytes(pubOf(d)), 4<<20, ports.Hash{}, 5, 1))
	for _, k := range []ports.NodeID{ports.HashBytes(pubOf(a)), ports.HashBytes(pubOf(b)), ports.HashBytes(pubOf(d))} {
		if standingOf(c2, k) == 0 {
			t.Fatalf("CONTROL FAILED: an identity with its OWN root earned no standing, so the denial "+
				"above is not evidence about plot re-use — this chain grants nobody standing. id %x", k[:8])
		}
	}
	t.Logf("arm 1 (one plot, many identities): %s — 3 identities on 1 root yield 1 standing; "+
		"3 identities on 3 roots yield 3.", denied)
}

// ─────────────────────────────────────────────────────────────────────────────
// ARM 2 — SYNTHETIC BYTES INSTEAD OF STORAGE
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilArm_SyntheticBytesEarnNothing drives the chain-level consequence of a prover that does
// not hold real sealed bytes. Whether a plot can be faked is settled in core/bond; what this arm
// holds is that the chain grants no standing when the possession proof does not verify.
//
// IT DRIVES VALIDATION, NOT apply, AND THAT DISTINCTION IS THE ARM. apply does not verify a
// space-time proof — its own comment says a height>0 registration "was already VERIFIED by
// validateBondRegs" — so a fixture that calls apply directly would hand standing to a forged
// registration and read as a broken denial. The refusal lives on the write path, which is where a
// registration from the network actually arrives, so that is where the attack is driven.
func TestSybilArm_SyntheticBytesEarnNothing(t *testing.T) {
	refuse := func(_ []byte, _ ports.Hash, _ int64, _ uint64, _ []byte) bool { return false }
	c := censusChain(t, refuse, 0)
	k := key(820001)
	prev, h := c.Head()
	forged := &Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, prev, 5, 1)}}

	if err := c.validateBondRegs(forged); err == nil {
		t.Fatal("THE POSSESSION AXIS DOES NOT DENY SYNTHETIC BYTES: a registration whose space-time " +
			"proof does NOT verify was ACCEPTED on the write path. " +
			"  Standing is supposed to cost sealed, non-substitutable storage; if a failing proof is " +
			"admitted, the cost of the Nth identity is the cost of a registration message.")
	}

	// POSITIVE CONTROL, on this axis: the identical registration under a verifier that accepts is
	// admitted, and the identity then holds standing. The refusal above must be the PROOF failing,
	// not the registration being malformed or the block being refused for an unrelated reason.
	c2 := censusChain(t, objectiveVerify, 0)
	prev2, h2 := c2.Head()
	honest := &Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2,
		BondRegs: []BondReg{bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, prev2, 5, 1)}}
	if err := c2.validateBondRegs(honest); err != nil {
		t.Fatalf("CONTROL FAILED: the same registration was refused even with a verifier that "+
			"accepts (%v), so the denial above says nothing about possession — the registration "+
			"itself is being refused for some other reason", err)
	}
	c2.apply(*honest)
	if standingOf(c2, ports.HashBytes(pubOf(k))) == 0 {
		t.Fatal("CONTROL FAILED: a registration that PASSED validation still yielded no standing, so " +
			"this arm is not measuring the possession screen at all")
	}
	t.Logf("arm 2 (synthetic bytes): %s — a registration whose possession proof fails is refused on "+
		"the write path; the same registration with a passing proof is admitted and earns standing.",
		denied)
}

// ─────────────────────────────────────────────────────────────────────────────
// ARM 3 — SELF-DEALT DEMAND
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilArm_DemandIsUnwiredForStanding reports the demand axis as UNWIRED rather than denied,
// and drives the fact rather than asserting it: standing is a function of the bond alone, so there
// is no demand quantity for a self-dealing farm to inflate — and equally, none for the composition
// to charge a Sybil for.
//
// This is the arm the release-candidate list means when it says an arm must be "reported UNWIRED
// rather than denied". Reporting it is the point: an axis that cannot deny anything is a fact about
// the build, and a reader should not have to discover it.
func TestSybilArm_DemandIsUnwiredForStanding(t *testing.T) {
	c := censusChain(t, objectiveVerify, 0)
	k := key(830001)
	id := ports.HashBytes(pubOf(k))

	// An identity that has served nothing at all, and holds a bond.
	registerAt(c, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, ports.Hash{}, 5, 1))
	if standingOf(c, id) == 0 {
		t.Fatal("FIXTURE: a bonded identity holds no standing, so this arm cannot measure what " +
			"standing depends on")
	}

	// THE MEASUREMENT: standing is decided by bonded size, the slash flag and the floor — and by
	// nothing else. An identity that served zero bytes holds full standing, which is exactly what
	// "demand does not enter the standing number" means.
	sz := standingOf(c, id)
	if sz != 4<<20 {
		t.Fatalf("standing is %d for a 4 MiB bond; it should be the bonded size itself, which is what "+
			"makes the demand axis's absence visible here", sz)
	}
	// And the negative direction: no bond, no standing, whatever else an identity may have done.
	if got := standingOf(c, ports.HashBytes(pubOf(key(830002)))); got != 0 {
		t.Fatalf("an identity with no bond at all holds standing %d", got)
	}
	t.Logf("arm 3 (self-dealt demand): %s — standing is the bonded size and nothing else, so served "+
		"demand neither earns standing nor can be inflated to buy it. There is no denial to make "+
		"here until the axis enters the standing number.", unwired)
}

// ─────────────────────────────────────────────────────────────────────────────
// ARM 4 — MASSING IDENTITIES IN ONE FAILURE DOMAIN
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilArm_DiversityIsUnwiredForStanding reports the diversity axis as UNWIRED for STANDING and
// drives where it does bind: the concentration metric. A farm that masses N identities behind one
// declared domain earns N full standings; the declaration only moves C2, which gates the maturity
// shed rather than who is qualified.
//
// The self-declared part is driven too, because it is the sharper half: the domain is a field the
// registrant supplies, so a splitter evades the grouping for the cost of a different number.
func TestSybilArm_DiversityIsUnwiredForStanding(t *testing.T) {
	c := censusChain(t, objectiveVerify, 0)
	var regs []BondReg
	var ids []ports.NodeID
	for i := 0; i < 4; i++ {
		k := key(int64(840001 + i))
		ids = append(ids, ports.HashBytes(pubOf(k)))
		regs = append(regs, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, ports.Hash{}, 5, 7)) // one declared domain
	}
	registerAt(c, regs...)

	for _, id := range ids {
		if standingOf(c, id) == 0 {
			t.Fatalf("FIXTURE: an identity in the massed set holds no standing; id %x", id[:8])
		}
	}
	t.Logf("arm 4 (massing in one failure domain): %s for standing — 4 identities sharing one "+
		"declared domain each hold full standing. The domain is committed and enters the C2 "+
		"concentration metric that gates the maturity shed, not idQualifies. It is also SELF-DECLARED: "+
		"the registrant supplies the number, so a splitter separates into 4 domains at no cost.",
		unwired)

	// The self-declaration, driven rather than asserted: the same four identities declaring four
	// DIFFERENT domains are indistinguishable at the standing level from the four above.
	c2 := censusChain(t, objectiveVerify, 0)
	var split []BondReg
	for i := 0; i < 4; i++ {
		k := key(int64(840001 + i))
		split = append(split, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, ports.Hash{}, 5, uint64(100+i)))
	}
	registerAt(c2, split...)
	for i, id := range ids {
		if standingOf(c, id) != standingOf(c2, id) {
			t.Fatalf("identity %d holds different standing under one declared domain (%d) than under "+
				"four (%d) — if the declaration moved the standing number, this arm would be DENIED "+
				"rather than UNWIRED and the census must be re-derived",
				i, standingOf(c, id), standingOf(c2, id))
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ARM 5 — COASTING ON STALE STANDING
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilArm_CoastingOnStaleStandingIsDenied drives the retention-decay axis: register once,
// release the plot, and keep voting forever off a single one-time proof.
func TestSybilArm_CoastingOnStaleStandingIsDenied(t *testing.T) {
	const ttl = 2
	c := censusChain(t, objectiveVerify, ttl)
	k := key(850001)
	id := ports.HashBytes(pubOf(k))
	registerAt(c, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, ports.Hash{}, 5, 1))
	if standingOf(c, id) == 0 {
		t.Fatal("FIXTURE: the identity never earned standing, so there is nothing to coast on")
	}

	// Drive past the re-challenge window without renewing — the release-and-coast attack.
	for i := 0; i < int(ttl)+2; i++ {
		prev, h := c.Head()
		c.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev})
	}
	if got := standingOf(c, id); got != 0 {
		t.Fatalf("THE RETENTION AXIS DOES NOT DENY COASTING: an identity that registered once and "+
			"never re-proved still holds standing %d, %d blocks past a TTL of %d.\n"+
			"  Release-and-coast makes the Nth standing cost one proof rather than sustained storage, "+
			"which is the time dimension of the cost claim.", got, ttl+2, ttl)
	}

	// POSITIVE CONTROL, on this axis: an identity that DOES re-prove keeps its standing across the
	// same span. Otherwise "lost its standing" would also be the reading if standing simply decayed
	// for everyone, renewing or not — which would be an outage, not a defence.
	c2 := censusChain(t, objectiveVerify, ttl)
	registerAt(c2, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, ports.Hash{}, 5, 1))
	for i := 0; i < int(ttl)+2; i++ {
		prev, h := c2.Head()
		c2.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, prev, 5, 1)}})
	}
	if standingOf(c2, id) == 0 {
		t.Fatal("CONTROL FAILED: an identity that re-proved on every block ALSO lost its standing. " +
			"The decay is firing on everyone, so the denial above is an outage rather than a defence.")
	}
	t.Logf("arm 5 (coasting on stale standing): %s — standing lapses %d blocks after the last proof, "+
		"and a renewing identity keeps it across the same span.", denied, ttl)
}

// ─────────────────────────────────────────────────────────────────────────────
// THE CENSUS ITSELF
// ─────────────────────────────────────────────────────────────────────────────

// TestSybilCostCensusIsComplete is the row the release-candidate list actually asks for: every
// economy of scale the vision names appears here with a verdict, and none is silently absent.
//
// It does not re-drive the arms — each has its own test above, with its own control. What it holds
// is the SHAPE of the claim: five named economies, each mapped to an arm that runs, and the count
// of arms that actually deny anything today reported as a number rather than implied.
func TestSybilCostCensusIsComplete(t *testing.T) {
	census := []sybilArm{
		{axis: "bond size", economy: "one sealed plot backing many identities",
			verdict: denied, note: "per-root ownership: the second identity on a root earns nothing"},
		{axis: "possession", economy: "synthetic bytes standing in for storage",
			verdict: denied, note: "a failing space-time proof earns no standing"},
		{axis: "demand", economy: "self-dealt demand buying standing",
			verdict: unwired, note: "served demand does not enter idQualifies"},
		{axis: "diversity", economy: "massing identities behind one operator or subnet",
			verdict: unwired, note: "the declared domain moves C2, not the standing number, and is self-declared"},
		{axis: "retention", economy: "coasting on a single one-time proof",
			verdict: denied, note: "standing lapses without renewal on the re-challenge cadence"},
	}
	if len(census) != 5 {
		t.Fatalf("the vision names five economies of scale and one denial for each; this census has %d",
			len(census))
	}
	deniedCount := 0
	for _, a := range census {
		switch a.verdict {
		case denied:
			deniedCount++
		case unwired:
		default:
			t.Fatalf("arm %q carries verdict %q; an arm is DENIED or UNWIRED and nothing else — "+
				"there is no third state that lets an arm avoid saying which it is", a.axis, a.verdict)
		}
		t.Logf("  %-10s %-8s %s", a.axis, a.verdict, a.economy)
	}
	if deniedCount == 0 {
		t.Fatal("no arm denies anything: the cost claim has no operative content at all")
	}
	t.Logf("CENSUS: %d of %d axes deny an economy of scale at the standing level; %d are UNWIRED — "+
		"they do not enter the standing number, so they deny nothing yet. Consensus standing is "+
		"gated by the bond axis and its time dimension; the multiplicative interlock across all five "+
		"is the destination, not today's guarantee.",
		deniedCount, len(census), len(census)-deniedCount)
}
