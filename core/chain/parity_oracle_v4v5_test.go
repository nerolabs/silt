package chain

import (
	"crypto/ed25519"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// SCOPE, stated plainly (PE re-ruling 2026-09-08): the v4 twin is minted in TWO passes, because
// era-3 `apply` seats validatorsSeen from the block's non-hash-covered Atts while the attestations
// sign the hash that covers the root (R-BOX-ATTESTS / O1–O2, retired unrun on main). That procedure
// is one a real proposer cannot execute, so this oracle proves parity of the era-3 validity BODIES
// between the node and the mirrors — not that a v4 block is mintable on the real path.
//
// M-1A-3 — THE v4/v5 PARITY ORACLE (research certification
// FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e §7.3; PE ruling §1.2, the seven regimes)
// =============================================================================
//
// WHY THIS IS THE ONLY DIFFERENTIAL LEFT. M-2 made the node's v5 accept path BE the composition:
// for a v5 block, ValidateCommit is ValidateCommitV5(liveView{c}, b). "Composition vs node" is
// therefore one function compared with itself (the G-D6 vacuity, M-1A-4). What still exists is
// the node's OWN era-3 body — proposerQualifiedAt, attesterQualifiedAt, validateBondRegs,
// requireQuorumStack, matureNow, … — which a v4 block runs verbatim. So: the SAME committed state,
// the SAME block content, minted once at BlockVersionStateRoot (the node's bodies) and once at
// BlockVersionWitnessable (the mirrors), must draw the SAME verdict and, where a sentinel exists,
// the SAME sentinel and the SAME rendering.
//
// THE EXCLUSIONS, named so the gate is honest. v4 and v5 differ BY RULE in four places and the
// oracle's pairs carry none of them: the v5-only RegCap clause (a pair carries ≤ 2 BondRegs, far
// under RegCap), the v5-only IssuerKeys era gate (no IssuerKeys), the carrier (no LastCommit —
// a v4 carrier is ErrCarrierNotWitnessable), and the leaf set each root commits (each twin's roots
// are computed for its own version by postApplyRoots, so P13 is a same-predicate comparison whose
// RENDERED roots differ — the one case where text parity is not asserted). A fifth is driven as
// an EXPECTED divergence: at/past Era4ActivationHeight the v4 twin is refused by name.
//
// WHAT IT COVERS. Every regime the round's other fixtures never reach (certification §7.2): the
// mature epoch (Q3 and the frozen arms of P4 / the attester filter), the launch window (Q2 and the
// anchor-only proposer arm), the reg gate (P7's active arm, including the #535 restore exemption),
// the #535 recovery boundary (v5EffectiveEpochSet's recovery arm), de-maturation (v5MatureNow and
// Q4), the pruned leg of P7, the legacy leg, and the era-3 / era-4 version rules. Each regime
// asserts one accept and at least one refusal per mirrored stage, and records — right after the
// by-name assertion that proves it ran — which of the uncovered mirrors it drove. The
// closing assertion holds the record to the certification's list.
//
// Ablation (M-1A-3): delete one mirrored clause — e.g. the `seenReg[id]` twice-in-one-block check
// in v5ValidateBondRegs — and the v4 and v5 verdicts diverge in the reg-gate regime ⇒ RED naming
// the regime and the case.

// uncoveredMirrors are the eight mirrors the certification found with ZERO driven coverage before
// this oracle (§7.2), plus v5RequiredQuorum's mature leg (#380 direction (1), regime (b)). The oracle must drive every one; a renamed mirror reddens the closing check.
var uncoveredMirrors = []string{
	"v5RequireEpochWeightQuorum",   // Q3
	"v5RequireDeMatureSuperQuorum", // Q4
	"v5MatureNow",                  // the objective maturity metric
	"v5EffectiveEpochSet",          // the #535 recovery arm
	"v5RestoresHeldStanding",       // the #535 restore exemption
	"v5RegGateActive",              // the active arm
	"v5ValidateBondRegs",           // P7's pruned arm
	"v5RequireProposerQualified",   // the mature-epoch (frozen-set) arm, and v5AttesterQualifiedAt's
	"v5RequiredQuorum",             // Q1's regime (b) — the mature-epoch floor 0 (#380 direction (1))
}

// parityWorld is one committed chain plus the signers that can mint on it.
type parityWorld struct {
	t       *testing.T
	regime  string
	c       *Chain
	anchors []ed25519.PrivateKey
	drove   map[string][]string
}

func (w *parityWorld) driven(mirror, evidence string) {
	w.drove[mirror] = append(w.drove[mirror], w.regime+": "+evidence)
}

// mint builds a block at the given version on the world's head: proposer signature, the roots
// computed FOR THAT VERSION, the proposer's own prepare, and prepare + precommit signatures from
// every attester. mutate shapes the payload before the roots are computed. carry harvests the
// head block's precommits as the LastCommit carrier (v5 only; used for committed history, never
// for a pair).
func (w *parityWorld) mint(version uint64, proposer ed25519.PrivateKey, attesters []ed25519.PrivateKey, carry bool, mutate func(*Block)) Block {
	w.t.Helper()
	prev, h := w.c.Head()
	b := &Block{Version: version, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h + 40))}}
	if carry && h >= 2 {
		for _, a := range w.c.blocks[len(w.c.blocks)-1].Atts {
			if a.Phase == PhasePrecommit {
				b.LastCommit = append(b.LastCommit, a)
			}
		}
	}
	if mutate != nil {
		mutate(b)
	}
	// TWO PASSES for the roots. A v4 block's post-apply state includes the seating write from
	// b.Atts (validatorsSeen — the era-3 transition input that is NOT hash-covered, the
	// R-BOX-ATTESTS defect), so its roots depend on WHO attests, while the attestations sign the
	// hash that covers the roots. Seating depends on the attester IDS alone, so: attach the
	// attester set provisionally, compute the roots, sign, re-issue the attestations over the
	// final hash. A v5 block's own Atts write nothing (the carrier seats), so for it the second
	// pass changes no root — which is exactly the property era-4 was built for.
	attach := func() {
		b.PrepareQC, b.Atts = nil, nil
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, proposer, 0, PhasePrepare))
		for _, k := range attesters {
			b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
			b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
		}
	}
	Sign(b, proposer) // provisional: sets b.Proposer for the seating exclusion in apply
	attach()
	state, log, err := w.c.postApplyRoots(*b)
	if err != nil {
		w.t.Fatalf("%s: postApplyRoots: %v", w.regime, err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, proposer)
	attach()
	return *b
}

// pair mints the SAME content at v4 and at v5.
func (w *parityWorld) pair(proposer ed25519.PrivateKey, attesters []ed25519.PrivateKey, mutate func(*Block)) (v4, v5 Block) {
	w.t.Helper()
	return w.mint(BlockVersionStateRoot, proposer, attesters, false, mutate),
		w.mint(BlockVersionWitnessable, proposer, attesters, false, mutate)
}

// commit appends a v5 block (with the carrier) through the node's own door.
func (w *parityWorld) commit(proposer ed25519.PrivateKey, attesters []ed25519.PrivateKey, mutate func(*Block)) {
	w.t.Helper()
	b := w.mint(BlockVersionWitnessable, proposer, attesters, true, mutate)
	if err := w.c.Append(b); err != nil {
		w.t.Fatalf("%s: history block at height %d must COMMIT through the node's door: %v", w.regime, b.Height, err)
	}
}

// parityCase is one (v4, v5) pair and the verdict both must draw.
type parityCase struct {
	name string
	v4   Block
	v5   Block
	want error // nil: both accept; otherwise both refuse with errors.Is(err, want)
	// rootsRendered marks a refusal whose text renders the (version-specific) roots, so text
	// parity is not asserted — only the sentinel. The ONLY permitted text departure.
	rootsRendered bool
}

// assertParity is THE oracle: same verdict, same sentinel, same rendering.
func (w *parityWorld) assertParity(pc parityCase) {
	w.t.Helper()
	e4 := w.c.ValidateCommit(&pc.v4)
	e5 := w.c.ValidateCommit(&pc.v5)
	if pc.v4.Version != BlockVersionStateRoot || pc.v5.Version != BlockVersionWitnessable {
		w.t.Fatalf("PARITY ORACLE MIS-BUILT (%s / %s): the pair is v%d / v%d", w.regime, pc.name, pc.v4.Version, pc.v5.Version)
	}
	if pc.want == nil {
		if e4 != nil || e5 != nil {
			w.t.Fatalf("PARITY VIOLATED (%s / %s): both twins must be ACCEPTED\n  v4 (the node's bodies): %v\n  v5 (the mirrors):        %v",
				w.regime, pc.name, e4, e5)
		}
		return
	}
	if !errors.Is(e4, pc.want) || !errors.Is(e5, pc.want) {
		w.t.Fatalf("PARITY VIOLATED (%s / %s): both twins must be REFUSED with %v\n  v4 (the node's bodies): %v\n  v5 (the mirrors):        %v",
			w.regime, pc.name, pc.want, e4, e5)
	}
	if !pc.rootsRendered && e4.Error() != e5.Error() {
		w.t.Fatalf("PARITY VIOLATED (%s / %s): same sentinel, DIFFERENT rendering (the #572 attribution contract)\n  v4: %v\n  v5: %v",
			w.regime, pc.name, e4, e5)
	}
}

// keysFrom mints n deterministic keys from a seed base.
func keysFrom(base int64, n int) []ed25519.PrivateKey {
	ks := make([]ed25519.PrivateKey, n)
	for i := range ks {
		ks[i] = key(base + int64(i))
	}
	return ks
}

func anchorsOf(ks []ed25519.PrivateKey) map[ports.NodeID]bool {
	m := map[ports.NodeID]bool{}
	for _, k := range ks {
		m[idOf(k)] = true
	}
	return m
}

// newParityWorld builds an objective world: four anchors bonded twoMiB at genesis plus any extra
// genesis registrations, under cfg (Anchors/AnchorQuorum are set here). The genesis is v1 and
// signed by anchor 0.
func newParityWorld(t *testing.T, regime string, seed int64, cfg Config, extraGenesis []BondReg, drove map[string][]string) *parityWorld {
	t.Helper()
	anchors := keysFrom(seed, 4)
	cfg.Anchors, cfg.AnchorQuorum = anchorsOf(anchors), 1
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range anchors {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	g.BondRegs = append(g.BondRegs, extraGenesis...)
	Sign(g, anchors[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("%s: genesis: %v", regime, err)
	}
	return &parityWorld{t: t, regime: regime, c: c, anchors: anchors, drove: drove}
}

// TestM1A3_V4V5ParityOracle drives every regime. One test, so the closing mirror-coverage
// assertion sees every subtest's record; each regime also carries the honest twin (NG-2).
func TestM1A3_V4V5ParityOracle(t *testing.T) {
	drove := map[string][]string{}

	// ---------------------------------------------------------------------------------------
	// Regime 1 — objective, epochs off, handed off at genesis (the round's own fixture regime),
	// widened to a mutant table and to the PRUNED leg of P7.
	// ---------------------------------------------------------------------------------------
	t.Run("objective-noepochs", func(t *testing.T) {
		w := newParityWorld(t, "objective-noepochs", 91000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0}, nil, drove)
		a := w.anchors
		w.commit(a[0], a[1:], nil)
		honest4, honest5 := w.pair(a[0], a[1:], nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "honest", v4: honest4, v5: honest5})

		// P7's PRUNED leg, BOTH arms. A pruned twin keeps its payload (P5 passes) and reaches P7.
		// At/above the reader's OWN trust floor it is refused — the space-time proof cannot be
		// re-verified. Strictly below the floor the Answer-less registrations are trusted (finality
		// made them irreversible) and an Answer smuggled back is malformed. The floor is the
		// reader's: a Reconcile replay threads the RECEIVER's anchor through trustFloorOverride,
		// which is the only way a block at the head can sit below it; the box's provenView never
		// answers a floor at all (its door stalls on a pruned block before the composition).
		fresh := key(91100)
		reg4, reg5 := w.pair(a[0], a[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(fresh, twoMiB, b.Prev)} })
		if floor := w.c.trustFloor(); reg4.Height < floor {
			t.Fatalf("world: the candidate (h%d) must sit at/above the reader's floor (%d) for the refusal arm", reg4.Height, floor)
		}
		w.assertParity(parityCase{name: "pruned block at/above the trust floor (P7 pruned arm)", v4: reg4.Prune(), v5: reg5.Prune(), want: ErrPrunedAboveHorizon})
		w.driven("v5ValidateBondRegs", "P7 pruned arm — ErrPrunedAboveHorizon at/above the reader's floor")
		floor := reg4.Height + 3
		w.c.trustFloorOverride = &floor
		w.assertParity(parityCase{name: "pruned block below the reader's anchor: Answer-less regs trusted (P7 pruned arm)", v4: reg4.Prune(), v5: reg5.Prune()})
		smuggled4, smuggled5 := reg4.Prune(), reg5.Prune()
		smuggled4.BondRegs[0].Answer, smuggled5.BondRegs[0].Answer = []byte("valid"), []byte("valid")
		w.assertParity(parityCase{name: "pruned block below the anchor with an Answer smuggled back (P7 pruned arm)", v4: smuggled4, v5: smuggled5, want: ErrMalformedPruned})
		w.driven("v5ValidateBondRegs", "P7 pruned arm below the floor — Answer-less regs ACCEPTED, a smuggled Answer is ErrMalformedPruned")
		w.c.trustFloorOverride = nil

		v4, v5 := w.pair(a[0], a[1:], func(b *Block) { b.Revocations = []ports.Hash{ports.HashBytes([]byte("never-published"))} })
		w.assertParity(parityCase{name: "revoke unknown root (P6)", v4: v4, v5: v5, want: ErrRevokeUnknownRoot})
		v4, v5 = w.pair(a[0], a[1:], nil)
		v4.PrepareQC, v5.PrepareQC = v4.PrepareQC[1:], v5.PrepareQC[1:]
		w.assertParity(parityCase{name: "no proposer prepare (C1)", v4: v4, v5: v5, want: ErrProposerPrepare})
		v4, v5 = w.pair(a[0], nil, nil)
		w.assertParity(parityCase{name: "no attestations (Q1)", v4: v4, v5: v5, want: ErrNoQuorum})
		v4, v5 = w.pair(a[0], a[1:], nil)
		v4.Prev, v5.Prev = ports.HashBytes([]byte("elsewhere")), ports.HashBytes([]byte("elsewhere"))
		Sign(&v4, a[0])
		Sign(&v5, a[0])
		w.assertParity(parityCase{name: "wrong parent (P1)", v4: v4, v5: v5, want: ErrWrongParent})
		v4, v5 = w.pair(a[0], a[1:], nil)
		forged := ports.HashBytes([]byte("forged"))
		v4.StateRoot, v5.StateRoot = &forged, &forged
		Sign(&v4, a[0])
		Sign(&v5, a[0])
		w.assertParity(parityCase{name: "forged StateRoot (P13a)", v4: v4, v5: v5, want: ErrEra3StateRootMismatch, rootsRendered: true})
	})

	// ---------------------------------------------------------------------------------------
	// Regime 2 — the LAUNCH WINDOW: MatureValidators unmet, anchors not handed off. P4's
	// anchor-only arm and Q2's strict anchor majority.
	// ---------------------------------------------------------------------------------------
	t.Run("launch-window", func(t *testing.T) {
		vals := keysFrom(92100, 2)
		extra := []BondReg{bondReg(vals[0], twoMiB, ports.Hash{}), bondReg(vals[1], twoMiB, ports.Hash{})}
		w := newParityWorld(t, "launch-window", 92000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 2}, extra, drove)
		a := w.anchors
		if w.c.handedOff() {
			t.Fatal("world: the launch window must NOT have handed off")
		}
		w.commit(a[0], append(a[1:], vals...), nil)
		honest4, honest5 := w.pair(a[0], append(a[1:], vals...), nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "honest (anchor proposes, anchor majority)", v4: honest4, v5: honest5})
		v4, v5 := w.pair(vals[0], a, nil)
		w.assertParity(parityCase{name: "bonded non-anchor proposes (P4 anchor-only arm)", v4: v4, v5: v5, want: ErrLowReputation})
		// Q1 (bftThreshold(4) = 2) is met by 3 qualified attesters; Q2 wants 3 anchor support and
		// gets proposer + 1.
		v4, v5 = w.pair(a[0], []ed25519.PrivateKey{a[1], vals[0], vals[1]}, nil)
		w.assertParity(parityCase{name: "anchor support short (Q2)", v4: v4, v5: v5, want: ErrAnchorRequired})
	})

	// ---------------------------------------------------------------------------------------
	// Regime 3 — the MATURE EPOCH: EpochBlocks > 0, matureEpoch from the genesis rotation, the
	// four anchors frozen. Q3 weight quorum; the frozen arms of P4 and the attester filter.
	// ---------------------------------------------------------------------------------------
	t.Run("mature-epoch", func(t *testing.T) {
		w := newParityWorld(t, "mature-epoch", 93000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 2}, nil, drove)
		a := w.anchors
		if !w.c.matureEpoch || len(w.c.epochSet) != 4 {
			t.Fatalf("world: the genesis rotation must freeze the four anchors; matureEpoch=%v |epochSet|=%d", w.c.matureEpoch, len(w.c.epochSet))
		}
		newcomer := key(93100)
		w.commit(a[0], a[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(newcomer, twoMiB, b.Prev)} })
		if _, frozen := w.c.epochSet[idOf(newcomer)]; frozen || w.c.bonded[idOf(newcomer)] != twoMiB {
			t.Fatal("world: the newcomer must be bonded mid-epoch and NOT in the frozen set")
		}
		honest4, honest5 := w.pair(a[0], a[1:], nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "honest (full frozen weight)", v4: honest4, v5: honest5})
		v4, v5 := w.pair(a[0], a[1:2], nil)
		w.assertParity(parityCase{name: "coalition holds half the frozen weight (Q3)", v4: v4, v5: v5, want: ErrNoQuorumWeight})
		w.driven("v5RequireEpochWeightQuorum", "ErrNoQuorumWeight at 4 of 8 MiB, node rendering")
		v4, v5 = w.pair(newcomer, a[1:], nil)
		w.assertParity(parityCase{name: "mid-epoch newcomer proposes (P4 frozen arm)", v4: v4, v5: v5, want: ErrLowReputation})
		w.driven("v5RequireProposerQualified", "the frozen-set refusal by name, '… not in the frozen epoch set governing height …'")
		// The attester frozen arm drops the newcomer, leaving the proposer alone. Q1's floor is 0
		// in a mature epoch (#380 regime (b)), so the refusal is Q3's, by name — a mirror that
		// still floored at Params.Quorum would say ErrNoQuorum here and break parity.
		v4, v5 = w.pair(a[0], []ed25519.PrivateKey{newcomer}, nil)
		w.assertParity(parityCase{name: "only the newcomer attests (attester frozen arm drops it → Q1 floor 0 → Q3)", v4: v4, v5: v5, want: ErrNoQuorumWeight})
		w.driven("v5RequiredQuorum", "regime (b): the proposer alone passes Q1 (floor 0) and is refused by Q3, ErrNoQuorumWeight, node rendering")
	})

	// ---------------------------------------------------------------------------------------
	// Regime 3b — the MATURE EPOCH with a WHALE: one frozen member holds > 2/3 of the frozen
	// weight, so the node ACCEPTS a commit with ZERO non-proposer attestations (#380 regime (b),
	// gate arm 8c(i)). This is the only world where Q1's floor 0 is observable as an accept.
	// ---------------------------------------------------------------------------------------
	t.Run("mature-epoch-whale", func(t *testing.T) {
		whale := key(93500)
		extra := []BondReg{bondReg(whale, 20<<20, ports.Hash{})} // 20 of 28 MiB frozen: > 2/3 alone
		w := newParityWorld(t, "mature-epoch-whale", 93600, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 2}, extra, drove)
		a := w.anchors
		if !w.c.matureEpoch || len(w.c.epochSet) != 5 {
			t.Fatalf("world: the genesis rotation must freeze the four anchors and the whale; matureEpoch=%v |epochSet|=%d", w.c.matureEpoch, len(w.c.epochSet))
		}
		if rq := w.c.RequiredQuorum(); rq != 0 || w.c.cfg.Quorum <= 0 {
			t.Fatalf("world: RequiredQuorum() must be 0 in the mature epoch with cfg.Quorum (%d) above it; got %d", w.c.cfg.Quorum, rq)
		}
		honest4, honest5 := w.pair(whale, nil, nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "the whale alone, ZERO attestations (Q1 floor 0, Q3 by weight)", v4: honest4, v5: honest5})
		w.driven("v5RequiredQuorum", "regime (b): a zero-attestation > 2/3-weight commit ACCEPTED by both twins")
		v4, v5 := w.pair(a[0], nil, nil)
		w.assertParity(parityCase{name: "an anchor alone, ZERO attestations (Q1 floor 0 clears, Q3 refuses by name)", v4: v4, v5: v5, want: ErrNoQuorumWeight})
		w.driven("v5RequiredQuorum", "regime (b): the count floor never fires; the weight rule refuses, ErrNoQuorumWeight, node rendering")
		v4, v5 = w.pair(a[0], a[1:], nil)
		w.assertParity(parityCase{name: "the four anchors without the whale (8 of 28 MiB, Q3)", v4: v4, v5: v5, want: ErrNoQuorumWeight})
	})

	// ---------------------------------------------------------------------------------------
	// Regime 4 — the REG GATE ACTIVE, in a mature epoch with a TTL (R = 10): P7's active arm —
	// first registration, twice-in-one-block, re-registration inside R, and the #535 restore
	// exemption for a LAPSED frozen member re-proving its own root.
	// ---------------------------------------------------------------------------------------
	t.Run("reggate-active", func(t *testing.T) {
		w := newParityWorld(t, "reggate-active", 94000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0,
			EpochBlocks: 2, BondTTLBlocks: 32, RegGateActivationHeight: 1}, nil, drove)
		a := w.anchors
		w.commit(a[0], a[1:], nil)
		if !w.c.regGateActive(2) || w.c.regMinInterval() != 10 {
			t.Fatalf("world: the gate must be active at h2 with R = 10; active=%v R=%d", w.c.regGateActive(2), w.c.regMinInterval())
		}
		fresh := key(94100)
		honest4, honest5 := w.pair(a[0], a[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(fresh, twoMiB, b.Prev)} })
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "a fresh identity's first registration past the gate", v4: honest4, v5: honest5})
		w.driven("v5RegGateActive", "active arm taken (gate at h2 > 1), first reg admitted")
		v4, v5 := w.pair(a[0], a[1:], func(b *Block) {
			r := bondReg(fresh, twoMiB, b.Prev)
			b.BondRegs = []BondReg{r, r}
		})
		w.assertParity(parityCase{name: "the same identity registered twice in one block (P7 gate arm)", v4: v4, v5: v5, want: ErrRegGate})
		w.driven("v5RegGateActive", "ErrRegGate '… registered twice in one block'")
		member := a[1]
		v4, v5 = w.pair(a[0], a[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(member, twoMiB, b.Prev)} })
		w.assertParity(parityCase{name: "a bonded frozen member re-registers inside R (no restore: it holds live standing)", v4: v4, v5: v5, want: ErrRegGate})
		w.driven("v5RestoresHeldStanding", "false for a member with LIVE standing → ErrRegGate '… re-registered 2 blocks after its last reg (R=10)'")
		// THE LAPSE (the shipped #535 fix-(4) test's surgery): the member's standing drops below
		// MinBond while it stays frozen-epoch-seated and still owns its root.
		delete(w.c.bonded, idOf(member))
		v4, v5 = w.pair(a[0], a[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(member, twoMiB, b.Prev)} })
		w.assertParity(parityCase{name: "a LAPSED frozen member re-proves its own root inside R (the #535 restore exemption)", v4: v4, v5: v5})
		w.driven("v5RestoresHeldStanding", "true for a lapsed frozen member re-proving its owned root → ACCEPT inside R")
	})

	// ---------------------------------------------------------------------------------------
	// Regime 5 — the #535 RECOVERY BOUNDARY: LivenessRecoveryHeight names the next boundary, so
	// the governing set at that height is the LIVE qualified set, not the frozen one. Driven
	// against a twin world with the directive OFF, so the arm is the measured discriminator.
	// ---------------------------------------------------------------------------------------
	t.Run("recovery-boundary", func(t *testing.T) {
		build := func(recovery uint64, tag string) (*parityWorld, ed25519.PrivateKey) {
			w := newParityWorld(t, "recovery-boundary/"+tag, 95000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0,
				EpochBlocks: 2, LivenessRecoveryHeight: recovery}, nil, drove)
			fresh := key(95100)
			w.commit(w.anchors[0], w.anchors[1:], func(b *Block) { b.BondRegs = []BondReg{bondReg(fresh, 16<<20, b.Prev)} })
			if _, frozen := w.c.epochSet[idOf(fresh)]; frozen || w.c.bonded[idOf(fresh)] != 16<<20 {
				t.Fatal("world: the fresh joiner must be bonded and NOT frozen")
			}
			return w, fresh
		}
		on, fresh := build(2, "on")
		off, _ := build(0, "off")
		a := on.anchors
		// At the recovery boundary the live set is 24 MiB and the anchors hold 8, so the honest
		// coalition NEEDS the joiner: the re-base is load-bearing for the accept itself.
		honest4, honest5 := on.pair(a[0], append(append([]ed25519.PrivateKey(nil), a[1:]...), fresh), nil)
		assertHonestTwinAccepts(t, on.c, honest5)
		on.assertParity(parityCase{name: "honest at the recovery boundary (anchors + the joiner)", v4: honest4, v5: honest5})
		v4, v5 := off.pair(off.anchors[0], off.anchors[1:], nil)
		off.assertParity(parityCase{name: "honest with the directive OFF (the frozen anchors alone)", v4: v4, v5: v5})
		// The fresh joiner is LOAD-BEARING: under the live set it qualifies and carries 16 of 24
		// MiB; under the frozen set it is dropped and nobody is left.
		v4, v5 = on.pair(a[0], []ed25519.PrivateKey{fresh}, nil)
		on.assertParity(parityCase{name: "the non-frozen joiner's attestation carries the weight quorum (recovery arm)", v4: v4, v5: v5})
		on.driven("v5EffectiveEpochSet", "recovery arm: a non-frozen live-qualified attester counts at LivenessRecoveryHeight")
		on.driven("v5RequireEpochWeightQuorum", "weight summed over the LIVE set at the recovery boundary (accept)")
		// With the directive OFF the joiner is dropped and the proposer stands alone; Q1's floor is
		// 0 in a mature epoch (#380 regime (b)), so the refusal is Q3's (2 of 8 MiB), by name.
		v4, v5 = off.pair(off.anchors[0], []ed25519.PrivateKey{fresh}, nil)
		off.assertParity(parityCase{name: "the same block with the directive OFF: the joiner is dropped (Q1 floor 0 → Q3)", v4: v4, v5: v5, want: ErrNoQuorumWeight})
		v4, v5 = on.pair(fresh, a[1:], nil)
		on.assertParity(parityCase{name: "the non-frozen joiner PROPOSES at the recovery boundary (P4 recovery re-base)", v4: v4, v5: v5})
		v4, v5 = off.pair(fresh, off.anchors[1:], nil)
		off.assertParity(parityCase{name: "the joiner proposes with the directive OFF (P4 frozen arm)", v4: v4, v5: v5, want: ErrLowReputation})
	})

	// ---------------------------------------------------------------------------------------
	// Regime 6 — DE-MATURATION: the network latches mature on three equal bonds, then a whale is
	// seated and the coefficient collapses. v5MatureNow both ways; Q4's real-bond super-quorum.
	// Seating goes through the v5 carrier (the committed history is v5), never through Atts.
	// ---------------------------------------------------------------------------------------
	t.Run("demature", func(t *testing.T) {
		vals := keysFrom(96100, 3)
		whale := key(96200)
		extra := []BondReg{bondReg(whale, 32<<20, ports.Hash{})}
		for _, v := range vals {
			extra = append(extra, bondReg(v, twoMiB, ports.Hash{}))
		}
		w := newParityWorld(t, "demature", 96000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 2}, extra, drove)
		a := w.anchors
		// h1: attested by the anchors and the three validators (NOT the whale); h2 carries them.
		w.commit(a[0], append(a[1:], vals...), nil)
		w.commit(a[0], append(append(a[1:], vals...), whale), nil)
		if !w.c.everMature || !w.c.matureNow() || w.c.validatorsSeen[idOf(whale)] {
			t.Fatalf("world: after h2 the network must be LATCHED mature on the three validators with the whale unseated; everMature=%v matureNow=%v", w.c.everMature, w.c.matureNow())
		}
		all := append(append(a[1:], vals...), whale)
		honest4, honest5 := w.pair(a[0], all, nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "mature and decentralized (Q4 skipped: matureNow true)", v4: honest4, v5: honest5})
		w.driven("v5MatureNow", "true on the accept path (three equal bonds ≥ MatureValidators 2); Q4 not entered")
		// h3 carries h2's precommits, which include the whale: seated. Coefficient 1 < 2.
		w.commit(a[0], all, nil)
		if !w.c.validatorsSeen[idOf(whale)] || w.c.matureNow() || !w.c.everMature {
			t.Fatalf("world: after h3 the whale must be seated and the network DE-MATURED; seated=%v matureNow=%v", w.c.validatorsSeen[idOf(whale)], w.c.matureNow())
		}
		v4, v5 := w.pair(a[0], all, nil)
		assertHonestTwinAccepts(t, w.c, v5)
		w.assertParity(parityCase{name: "de-matured, the whale in the coalition (Q4 met)", v4: v4, v5: v5})
		v4, v5 = w.pair(a[0], append(a[1:], vals...), nil)
		w.assertParity(parityCase{name: "de-matured, the whale absent (Q4: 14 of 46 MiB)", v4: v4, v5: v5, want: ErrDeMatureQuorum})
		w.driven("v5MatureNow", "false once the whale is seated (coefficient 1 < 2) → Q4 entered")
		w.driven("v5RequireDeMatureSuperQuorum", "ErrDeMatureQuorum '… 14 MiB of 46 MiB bonded (need ≥31 MiB)', node rendering; and the accept with the whale")
	})

	// ---------------------------------------------------------------------------------------
	// Regime 7 — LEGACY (MinBond == 0, the local reputation view): the G-D6 differential.
	// ---------------------------------------------------------------------------------------
	t.Run("legacy", func(t *testing.T) {
		lf := buildLegacyFixture(t)
		w := &parityWorld{t: t, regime: "legacy", c: lf.c, drove: drove}
		honest4, honest5 := w.pair(lf.prop, lf.vals, nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "honest (rep ≥ MinProposerRep / MinAttesterRep)", v4: honest4, v5: honest5})
		lf.reps[idOf(lf.prop)] = 10
		v4, v5 := w.pair(lf.prop, lf.vals, nil)
		w.assertParity(parityCase{name: "proposer below MinProposerRep", v4: v4, v5: v5, want: ErrLowReputation})
		lf.reps[idOf(lf.prop)] = 1000
		lf.reps[idOf(lf.vals[0])], lf.reps[idOf(lf.vals[1])] = 10, 10
		v4, v5 = w.pair(lf.prop, lf.vals, nil)
		w.assertParity(parityCase{name: "two of three attesters below MinAttesterRep (Q1 at Quorum 2)", v4: v4, v5: v5, want: ErrNoQuorum})
	})

	// ---------------------------------------------------------------------------------------
	// Regime 8 — the ERA version rules by genesis override. era-3 active: both twins satisfy
	// P10. era-4 active: the v4 twin is refused BY RULE — the expected divergence, by name.
	// ---------------------------------------------------------------------------------------
	t.Run("era3-active", func(t *testing.T) {
		w := newParityWorld(t, "era3-active", 97000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, Era3ActivationHeight: 1}, nil, drove)
		a := w.anchors
		w.commit(a[0], a[1:], nil)
		honest4, honest5 := w.pair(a[0], a[1:], nil)
		assertHonestTwinAccepts(t, w.c, honest5)
		w.assertParity(parityCase{name: "honest at/past the era-3 boundary (P10)", v4: honest4, v5: honest5})
	})
	t.Run("era4-active", func(t *testing.T) {
		w := newParityWorld(t, "era4-active", 98000, Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0,
			Era3ActivationHeight: 1, Era4ActivationHeight: 2}, nil, drove)
		a := w.anchors
		w.commit(a[0], a[1:], nil)
		v4, v5 := w.pair(a[0], a[1:], nil)
		assertHonestTwinAccepts(t, w.c, v5)
		if err := w.c.ValidateCommit(&v5); err != nil {
			t.Fatalf("era4-active: the v5 twin must be accepted at H_era4: %v", err)
		}
		if err := w.c.ValidateCommit(&v4); !errors.Is(err, ErrEra4VersionRequired) {
			t.Fatalf("era4-active: the v4 twin must be refused BY RULE at H_era4 (ErrEra4VersionRequired, P11); got %v", err)
		}
	})

	// ---------------------------------------------------------------------------------------
	// The closing check: every mirror the certification listed as undriven was driven, by name,
	// and every name is a real composition function (a renamed mirror reddens here).
	// SOURCE GATE: the name check. RUNTIME GATE: the regimes above.
	// ---------------------------------------------------------------------------------------
	comp := parseIndex(t, compositionFiles)
	var missing []string
	for _, m := range uncoveredMirrors {
		if comp.decls[m] == nil {
			t.Fatalf("SOURCE GATE: M-1A-3 — uncoveredMirrors names %s, which is not a composition function in %v", m, compositionFiles)
		}
		if len(drove[m]) == 0 {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("M-1A-3 INCOMPLETE: the parity oracle did not drive %v (certification §7.2 lists eight mirrors with zero driven coverage; #380 added v5RequiredQuorum)", missing)
	}
	var record []string
	for m, ev := range drove {
		record = append(record, m+": "+strings.Join(ev, " | "))
	}
	sort.Strings(record)
	t.Logf("M-1A-3 mirrors driven:\n  %s", strings.Join(record, "\n  "))
}
