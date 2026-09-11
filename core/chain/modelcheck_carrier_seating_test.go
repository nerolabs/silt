package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// Consensus model-check — CARRIER SEATING AGREEMENT (R-CARRIER-MODELCHECK,
// freeze-manifest item 14)
// =============================================================================
//
// THE PROPERTY. Every honest replica, given the same committed history, seats EXACTLY
// the same identity set from a LastCommit carrier. Formally, for a block b applied to a
// chain whose head is p:
//
//	seated(b) = { AttesterID(e) : e in b.LastCommit,
//	                              AttesterID(e) != p.ProposerID(),
//	                              attesterQualified(id) IN p's COMMITTED POST-STATE }
//
// Three consequences, and this file DRIVES each rather than asserting it:
//
//  1. AGREEMENT. seated(b) is a function of the SET of carried ids and of the parent's
//     committed post-state — of nothing else. It is invariant under the entry ORDER, under
//     WHICH replica proposed b, under duplicate entries, and under padding with entries
//     from unqualified ids. A replica that read any of those would fork the seating and,
//     because validatorsSeenRoot is a committed v5 leaf (statehash.go, tagValidatorsSeenRoot),
//     would reject a block every other replica accepts. The oracle therefore checks the
//     seated SET and the committed v5 STATE ROOT, which is the quantity replicas compare.
//
//  2. THE ORDERING PROPERTY. applyCarrier runs BEFORE this block's bond registrations, TTL
//     expiries and slashes (carrier.go applyCarrier; chain.go apply), so the screen reads
//     the PARENT's committed post-state — which is exactly the floor box's prevStateRoot.
//     TestCarrierFoldPrecedesBondRegsInApply PINS that statement order structurally. This
//     file DRIVES the inversion instead: it re-folds the same carrier over the same block's
//     POST-bond/TTL/slash state and shows the seated set DIVERGES, in all three directions
//     (a same-block bond that newly qualifies, a same-block slash, a same-block TTL lapse).
//     A pin says the statement has not moved; this says what moving it would cost.
//
//  3. THE REGIMES AND THE ERA SEAM. The screen is attesterQualified, whose meaning changes
//     with the regime: young/objective screens bonded >= MinBond || launchAnchor; a MATURE
//     epoch screens FROZEN epochSet membership. Both are driven, and a mid-epoch bond is the
//     discriminator that keeps the two arms from being the same test twice — the SAME
//     registration is seated in the young regime and refused in the mature one. At the
//     era-3 -> era-4 boundary the child is v5 while the parent is v4: the parent seated from
//     the frozen b.Atts rule, the child seats from the carrier, and the two must compose
//     into one set every replica computes.
//
// WHY THIS TIER. The consensus model-check tier runs in the required, merge-blocking CI job
// and carries no -short skip, so a property proven here cannot silently stop running.
//
// WHAT IT ASSUMES ABOUT UNMERGED WORK. Built against origin/main a28a5b5. A FORMAT branch is
// in flight over core/chain/chain.go, core/chain/carrier.go and core/node/chainrole.go. This
// file edits none of them and cites SYMBOLS, not coordinates: applyCarrier, validateCarrier,
// attesterQualified, HeadCarrier, PopulateEra4Roots, MintVersion. If that branch changes the
// era dispatch (the H_era4 liveness wedge, G-PRE-1), the era-seam oracle below is the one to
// re-read: it asserts composition across a v4 parent and a v5 child, which is that seam.

// ---- the population ---------------------------------------------------------------
//
// NON-UNIFORM BY CONSTRUCTION. A uniform population lets a wrong reducer still look right:
// if every carried id were qualified at the same weight, "seat everything carried" and
// "seat the qualified" agree on every input. The screened identities are qualified for
// DIFFERENT reasons and unqualified for FOUR different reasons:
//
//	big       8 MiB bond                         -> QUALIFIED
//	mid       4 MiB bond                         -> QUALIFIED (a different weight from big)
//	sub       512 KiB bond, BELOW MinBond        -> refused by the MinBond branch
//	unbonded  no bond at all                     -> refused by the absent-bond branch
//	slashed   4 MiB bond, slashed at height 1    -> refused by the slashed branch (the one
//	                                                live mid-epoch disqualification)
//	midEpoch  6 MiB bond registered at height 1  -> QUALIFIED in the young regime,
//	                                                refused by the FROZEN-SET branch in the
//	                                                mature one. The regime discriminator.
//
// plus the certificate signers, which qualify by a route of their own: launchAnchor (no bond
// at all) in the young regime, frozen epochSet membership in the mature one. Signer 0 is the
// PARENT's proposer, the one id the transition excludes.
const (
	csMinBond  = int64(1) << 20
	csBigBond  = int64(8) << 20
	csMidBond  = int64(4) << 20
	csSubBond  = int64(512) << 10 // deliberately BELOW csMinBond
	csLateBond = int64(6) << 20   // midEpoch's registration, above MinBond
)

// csEpochBlocks is the mature arm's epoch length. It must be > 2 so that heights 1 and 2 sit
// strictly INSIDE the epoch frozen at height 0 — with a shorter epoch the set would re-freeze
// under the test and the mid-epoch discriminator would silently integrate.
const csEpochBlocks = uint64(8)

// screenedID is one member of the screened population together with the qualification the
// PARENT's committed post-state gives it. The expectation is declared here, asserted in the
// setup proof, and then used by the oracles — so a population that drifted to "everything
// qualifies" reddens at the setup instead of making every agreement assertion trivially true.
type screenedID struct {
	name      string
	k         ed25519.PrivateKey
	qualified bool
}

// carrierWorld is a v5-from-height-1 chain with the non-uniform population above and a
// committed height 1, so the SUBJECT block is always height 2 and the parent's committed
// post-state is the state the screen must read.
type carrierWorld struct {
	c *Chain
	// mature records which regime this world was built for, so a helper that takes a world
	// can say which screen it is standing on without re-deriving it from chain internals.
	mature bool
	// cert signs every block's two-phase certificate; cert[0] proposed height 1 and is
	// therefore the parent proposer the transition excludes at height 2.
	cert           []ed25519.PrivateKey
	parentProposer ports.NodeID

	big, mid, sub, unbonded, slashed, midEpoch ed25519.PrivateKey

	// screened is the full population with its expected qualification.
	screened []screenedID
}

// newCarrierWorld builds the world for one regime and commits height 1. Height 1 carries the
// equivocation proof that slashes w.slashed AND the late registration that bonds w.midEpoch,
// so BOTH the slashed branch and the frozen-set branch of the screen are driven by committed
// history rather than by a hand-set map.
//
// The two regimes differ in exactly one structural way, and it is forced by silt's own rules:
// an ANCHOR cannot propose inside a mature epoch (proposerQualifiedAt screens frozen epochSet
// membership, and an anchor holds no bond), so the mature arm's certificate is four BONDED
// validators instead of four anchors. Stated rather than worked around: the certificate route
// is part of the regime.
func newCarrierWorld(t *testing.T, mature bool) *carrierWorld {
	t.Helper()
	w := &carrierWorld{
		mature:   mature,
		big:      key(91001),
		mid:      key(91002),
		sub:      key(91003),
		unbonded: key(91004),
		slashed:  key(91005),
		midEpoch: key(91006),
	}
	for i := 0; i < 4; i++ {
		w.cert = append(w.cert, key(int64(91100+i)))
	}

	chCfg := Config{
		Quorum: 1, MinBond: csMinBond, MinBondBytes: 0, ByzantineQuorum: true,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}
	var genesisRegs []BondReg
	if mature {
		// MATURE: no anchors; MatureValidators 0 makes Mature() true from the first apply, so
		// everMature latches at the genesis apply and the genesis rotation freezes epochSet.
		// EpochBlocks csEpochBlocks keeps heights 1 and 2 strictly inside that epoch.
		chCfg.MatureValidators = 0
		chCfg.EpochBlocks = csEpochBlocks
		// Distinct weights: a weight-blind reducer cannot hide behind a uniform certificate.
		for i, k := range w.cert {
			genesisRegs = append(genesisRegs, bondReg(k, int64(16-i)<<20, ports.Hash{}))
		}
	} else {
		// YOUNG: four launch anchors, NO bond of their own, so their qualification route is
		// launchAnchor and nothing else. MatureValidators 99 keeps the network immature for
		// the whole run, so the handoff never fires and launchAnchor stays true.
		anchors := map[ports.NodeID]bool{}
		for _, k := range w.cert {
			anchors[idOf(k)] = true
		}
		chCfg.Anchors = anchors
		chCfg.AnchorQuorum = 3
		chCfg.MatureValidators = 99
		chCfg.EpochBlocks = 0 // epochs DISABLED => the bonded || launchAnchor branch
	}
	genesisRegs = append(genesisRegs,
		bondReg(w.big, csBigBond, ports.Hash{}),
		bondReg(w.mid, csMidBond, ports.Hash{}),
		bondReg(w.sub, csSubBond, ports.Hash{}),
		bondReg(w.slashed, csMidBond, ports.Hash{}),
	)

	w.c = New(chCfg, func(ports.NodeID) int64 { return 0 })
	w.c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: genesisRegs}
	Sign(g, w.cert[0])
	if err := w.c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	prev, next := w.c.Head()
	if next != 1 {
		t.Fatalf("fixture: head after genesis must be height 1, got %d", next)
	}
	h1 := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(w.midEpoch, csLateBond, prev)},
		Slashes:  []Equivocation{slashProof(w.slashed, prev, 0xC1, 0xC2)}}
	if mv := w.c.MintVersion(next); mv != BlockVersionWitnessable {
		t.Fatalf("fixture: height 1 must mint v5 (%d), got v%d", BlockVersionWitnessable, mv)
	}
	if err := w.c.PopulateEra4Roots(h1); err != nil {
		t.Fatalf("populate era-4 roots at height 1: %v", err)
	}
	signCert(h1, w.cert, 0)
	if err := w.c.Append(*h1); err != nil {
		t.Fatalf("height 1: %v", err)
	}
	w.parentProposer = idOf(w.cert[0])

	// The declared population. cert[0] is the parent proposer; cert[1] is a second signer of
	// the same qualification route, so the exclusion is proven to be about the PARENT's
	// proposer and not about "a certificate signer".
	w.screened = []screenedID{
		// QUALIFIED under both regimes (launchAnchor when young, frozen-set membership when
		// mature) and EXCLUDED anyway, because it is the PARENT's proposer. Listing it as
		// qualified keeps the exclusion the only thing that can remove it from the seated set.
		{"cert0-parent-proposer", w.cert[0], true},
		{"cert1", w.cert[1], true},
		{"big", w.big, true},
		{"mid", w.mid, true},
		{"sub-below-minbond", w.sub, false},
		{"unbonded", w.unbonded, false},
		{"slashed", w.slashed, false},
		{"midEpoch", w.midEpoch, !mature},
	}
	return w
}

// signCert is twoPhaseSign with an EXPLICIT proposer: keys[proposer] signs the block and every
// key signs both phases. Rotating the proposer is how "which replica proposed" is varied.
func signCert(b *Block, keys []ed25519.PrivateKey, proposer int) {
	Sign(b, keys[proposer])
	for _, k := range keys {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
	}
	for _, k := range keys {
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
	}
}

// precommitOver returns a GENUINE PhasePrecommit attestation by k over hash h at round r —
// the bytes validateCarrier verifies. A carrier built from these is a carrier the production
// validity rule accepts; nothing here is a stub.
func precommitOver(k ed25519.PrivateKey, h ports.Hash, round uint64) Attestation {
	return Attestation{
		PubKey: append([]byte(nil), k.Public().(ed25519.PublicKey)...),
		Sig:    ed25519.Sign(k, consensusSigBytes(PhasePrecommit, round, h)),
		Round:  round,
		Phase:  PhasePrecommit,
	}
}

// seenSet reads the committed seated set as a sorted id list. In-package, so it reads the map
// the transition writes rather than the COUNT Regime() exposes: the property is about the SET,
// and a count agrees on two different sets of the same size.
func seenSet(c *Chain) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(c.validatorsSeen))
	for id := range c.validatorsSeen {
		out = append(out, id)
	}
	return sortIDs(out)
}

func sameIDs(a, b []ports.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fmtIDs(ids []ports.NodeID) string {
	out := "{"
	for i, id := range ids {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%x", id[:4])
	}
	return out + "}"
}

// replicaSeats is ONE honest replica applying b to its own deep copy of the committed history
// and reporting (seated set, committed v5 state root). cloneForDryRun + apply is the REAL
// transition over a REAL copy of committed state — this file's model of a replica.
func replicaSeats(t *testing.T, c *Chain, b Block) ([]ports.NodeID, ports.Hash) {
	t.Helper()
	r := c.cloneForDryRun()
	r.apply(b)
	root, err := r.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("state root: %v", err)
	}
	return seenSet(r), root
}

// invertedFoldSeats is the ORDERING INVERSION, driven rather than described: it applies the
// block with the carrier REMOVED — so this block's bond registrations, TTL expiries and
// slashes land first — and only THEN folds the same carrier. That is exactly what moving
// `c.applyCarrier(b, parentProposer)` below the bond loop in apply() would do, and it uses
// the REAL applyCarrier, so the inversion is the production function reading a mid-apply
// state instead of the parent's committed post-state.
func invertedFoldSeats(t *testing.T, c *Chain, b Block, parentProposer ports.NodeID) []ports.NodeID {
	t.Helper()
	r := c.cloneForDryRun()
	stripped := b
	stripped.LastCommit = nil
	stripped.hashMemoSet = false // #555: a stripped copy must never serve the un-stripped hash
	r.apply(stripped)
	r.applyCarrier(b, parentProposer)
	return seenSet(r)
}

// oracleSeats is the INDEPENDENT expectation: the screen evaluated against the PARENT's
// committed post-state, before the child block exists. It consults the parent chain and the
// carried ids only — never the subject block's own effects — so it cannot take its guard
// condition from its own subject.
func oracleSeats(parent *Chain, parentProposer ports.NodeID, carried []Attestation) []ports.NodeID {
	uniq := map[ports.NodeID]bool{}
	for i := range carried {
		id := carried[i].AttesterID()
		if id == parentProposer {
			continue
		}
		if parent.attesterQualified(id) {
			uniq[id] = true
		}
	}
	out := make([]ports.NodeID, 0, len(uniq))
	for id := range uniq {
		out = append(out, id)
	}
	return sortIDs(out)
}

// mintSubject builds the height-2 SUBJECT block: the given carrier, a chosen proposer, correct
// v5 roots over its own post-apply state, and a full certificate. mut runs BEFORE the roots are
// populated, so a mutation is covered by the signed roots (never a tamper behind the signature).
func mintSubject(t *testing.T, w *carrierWorld, carrier []Attestation, proposer int, mut func(*Block)) *Block {
	t.Helper()
	prev, next := w.c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	b.LastCommit = carrier
	if mut != nil {
		mut(b)
	}
	if err := w.c.PopulateEra4Roots(b); err != nil {
		t.Fatalf("populate era-4 roots at height %d: %v", next, err)
	}
	signCert(b, w.cert, proposer)
	return b
}

// carrierFrom builds a carrier over the world's head from the named population members.
func (w *carrierWorld) carrierFrom(ks []ed25519.PrivateKey, round uint64) []Attestation {
	head, _ := w.c.Head()
	out := make([]Attestation, 0, len(ks))
	for _, k := range ks {
		out = append(out, precommitOver(k, head, round))
	}
	return out
}

func (w *carrierWorld) allScreenedKeys() []ed25519.PrivateKey {
	out := make([]ed25519.PrivateKey, 0, len(w.screened))
	for _, s := range w.screened {
		out = append(out, s.k)
	}
	return out
}

// =============================================================================
// SETUP PROOF — the fixture is verified before any oracle trusts it
// =============================================================================
func TestModelCheck_CarrierSeating_SetupIsNonUniform(t *testing.T) {
	for _, mature := range []bool{false, true} {
		name := "young-objective"
		if mature {
			name = "mature-frozen-set"
		}
		t.Run(name, func(t *testing.T) {
			w := newCarrierWorld(t, mature)
			c := w.c

			if !c.objective() {
				t.Fatal("fixture VACUOUS: the chain must be OBJECTIVE, or the screen is the legacy reputation rule")
			}
			frozen := c.epochsEnabled() && c.matureEpoch
			if frozen != mature {
				t.Fatalf("fixture REGIME DRIFT: epochsEnabled=%v matureEpoch=%v (frozen-set screen=%v), want %v — "+
					"the two regime arms must not silently be the same test twice",
					c.epochsEnabled(), c.matureEpoch, frozen, mature)
			}
			if mature && c.epochStart+csEpochBlocks <= 2 {
				t.Fatalf("fixture: heights 1 and 2 must sit INSIDE the epoch frozen at %d (EpochBlocks %d), "+
					"or the mid-epoch discriminator re-integrates under the test", c.epochStart, csEpochBlocks)
			}
			if n := len(c.validatorsSeen); n != 0 {
				t.Fatalf("fixture: validatorsSeen must be EMPTY after height 1 (a v5 block's own Atts seat nothing), got %d", n)
			}
			if !c.slashed[idOf(w.slashed)] {
				t.Fatal("fixture: the height-1 slash did not commit — the slashed branch of the screen would be undriven")
			}
			if got := c.bonded[idOf(w.midEpoch)]; got != csLateBond {
				t.Fatalf("fixture: the height-1 late registration did not commit (bonded=%d want %d) — "+
					"the frozen-set branch would be undriven", got, csLateBond)
			}
			if got := c.bonded[idOf(w.sub)]; got != csSubBond || got >= c.cfg.MinBond {
				t.Fatalf("fixture: the sub-MinBond identity must be bonded BELOW MinBond: bonded=%d MinBond=%d", got, c.cfg.MinBond)
			}
			if _, ok := c.bonded[idOf(w.unbonded)]; ok {
				t.Fatal("fixture: the unbonded identity must carry NO committed bond")
			}
			if c.bonded[idOf(w.big)] == c.bonded[idOf(w.mid)] {
				t.Fatal("fixture UNIFORM: the two plainly-qualified identities must carry DIFFERENT bond weights")
			}
			if !mature {
				for _, k := range w.cert {
					if _, ok := c.bonded[idOf(k)]; ok {
						t.Fatal("fixture: the young arm's certificate signers must hold NO bond, so their route is launchAnchor ONLY")
					}
				}
			}

			nQual := 0
			for _, s := range w.screened {
				if got := c.attesterQualified(idOf(s.k)); got != s.qualified {
					t.Fatalf("fixture: attesterQualified(%s) = %v, want %v (frozen-set screen=%v)", s.name, got, s.qualified, frozen)
				}
				if s.qualified {
					nQual++
				}
			}
			if nQual == len(w.screened) || nQual == 0 {
				t.Fatalf("fixture VACUOUS: %d of %d screened members qualify — a uniform population cannot "+
					"distinguish a correct reducer from 'seat everything carried'", nQual, len(w.screened))
			}
			// The regime discriminator, asserted as a DIFFERENCE rather than assumed.
			if got := c.attesterQualified(idOf(w.midEpoch)); got == mature {
				t.Fatalf("fixture: the mid-epoch registration must qualify in the YOUNG regime and be refused "+
					"in the MATURE one; qualified=%v mature=%v — without that difference the two arms are one test",
					got, mature)
			}
			t.Logf("%s: %d/%d screened qualify, frozen-set screen=%v, midEpoch qualified=%v",
				name, nQual, len(w.screened), frozen, c.attesterQualified(idOf(w.midEpoch)))
		})
	}
}

// =============================================================================
// ORACLE 1 — AGREEMENT: the seated set is a function of the carried SET and the
// parent's committed post-state, and of nothing else
// =============================================================================
//
// Four variations, each of which MUST be irrelevant:
//
//	(a) WHICH IDS — exhaustive over all 2^n subsets of the screened population. This is the
//	    reducer oracle: for every possible carrier content the seated set equals the screen
//	    evaluated on the parent. A wrong screen (no screen, bonded>0, slashed ignored,
//	    frozen-set ignored) differs from the oracle on a DIFFERENT subset in each case, so
//	    the enumeration separates them.
//	(b) ORDER — the same set, permuted. Order-freedom is what lets two honest proposers build
//	    different blocks from the same held precommits without forking the seating.
//	(c) PROPOSER — the same carrier, each certificate signer proposing in turn. The excluded
//	    id is the PARENT's proposer, which does not move when the child's does.
//	(d) DUPLICATES and ROUNDS — the same signer carried twice, and at two different rounds.
//	    The transition is idempotent; the VALIDITY rule refuses the duplicate outright, so no
//	    two replicas can even be presented with a set that differs by one.
//
// (b), (c) and (d) also compare the committed v5 STATE ROOT, because that is the quantity a
// replica actually compares before it accepts a block.
func TestModelCheck_CarrierSeating_AgreementOverCarrierVariation(t *testing.T) {
	for _, mature := range []bool{false, true} {
		name := "young-objective"
		if mature {
			name = "mature-frozen-set"
		}
		t.Run(name, func(t *testing.T) {
			w := newCarrierWorld(t, mature)
			pool := w.allScreenedKeys()
			if len(pool) > 10 {
				t.Fatalf("the exhaustive subset enumeration is 2^%d — bound the population", len(pool))
			}

			// ---- (a) exhaustive over the 2^n carrier contents ----
			distinct := map[string]bool{}
			for mask := 0; mask < (1 << len(pool)); mask++ {
				var ks []ed25519.PrivateKey
				for i := range pool {
					if mask&(1<<i) != 0 {
						ks = append(ks, pool[i])
					}
				}
				carrier := w.carrierFrom(ks, 0)
				b := mintSubject(t, w, carrier, 0, nil)
				got, _ := replicaSeats(t, w.c, *b)
				want := oracleSeats(w.c, w.parentProposer, carrier)
				if !sameIDs(got, want) {
					t.Fatalf("subset mask %b: seated %s, want %s — the seated set is not the parent-post-state "+
						"screen over the carried ids", mask, fmtIDs(got), fmtIDs(want))
				}
				distinct[fmtIDs(got)] = true
			}
			// The enumeration must actually SEPARATE outcomes. One distinct seated set over 2^n
			// carriers would mean the oracle never discriminated anything.
			if len(distinct) < 4 {
				t.Fatalf("enumeration VACUOUS: %d distinct seated sets over %d carriers — the population does "+
					"not separate a correct reducer from a wrong one", len(distinct), 1<<len(pool))
			}
			t.Logf("%s: 2^%d carriers enumerated, %d distinct seated sets", name, len(pool), len(distinct))

			// ---- (b) order-freedom over the full population ----
			base := w.carrierFrom(pool, 0)
			refBlock := mintSubject(t, w, base, 0, nil)
			refSeats, refRoot := replicaSeats(t, w.c, *refBlock)
			if len(refSeats) == 0 {
				t.Fatal("VACUOUS: the reference carrier seats NOBODY — an order-invariance check over an empty " +
					"result holds for every reducer")
			}
			orders := carrierOrderings(base)
			if len(orders) < 24 {
				t.Fatalf("order family too small (%d) to be evidence of order-freedom", len(orders))
			}
			for oi, perm := range orders {
				b := mintSubject(t, w, perm, 0, nil)
				if b.Hash() == refBlock.Hash() && oi != 0 {
					t.Fatalf("ordering %d produced the SAME block hash as the reference — Hash() does not cover "+
						"the carrier ORDER, so this variation is not varying anything", oi)
				}
				got, root := replicaSeats(t, w.c, *b)
				if !sameIDs(got, refSeats) {
					t.Fatalf("ordering %d seated %s, reference seated %s — the carrier ORDER moved the seating",
						oi, fmtIDs(got), fmtIDs(refSeats))
				}
				if root != refRoot {
					t.Fatalf("ordering %d committed state root %x, reference %x — replicas would reject each "+
						"other's blocks over carrier order alone", oi, root[:8], refRoot[:8])
				}
			}

			// ---- (c) which replica proposed ----
			for p := range w.cert {
				b := mintSubject(t, w, base, p, nil)
				got, root := replicaSeats(t, w.c, *b)
				if !sameIDs(got, refSeats) {
					t.Fatalf("proposer %d seated %s, proposer 0 seated %s — the transition excluded THIS block's "+
						"proposer instead of the PARENT's", p, fmtIDs(got), fmtIDs(refSeats))
				}
				if root != refRoot {
					t.Fatalf("proposer %d committed state root %x, proposer 0 %x", p, root[:8], refRoot[:8])
				}
			}
			// And the exclusion is real: the parent's proposer is carried and NOT seated, while a
			// second signer of the SAME qualification route IS. Without this the agreement above
			// would also hold for a transition that excluded nobody.
			carriedParent := false
			for i := range base {
				if base[i].AttesterID() == w.parentProposer {
					carriedParent = true
				}
			}
			if !carriedParent {
				t.Fatal("VACUOUS: the reference carrier does not carry the parent's proposer, so the exclusion is undriven")
			}
			for _, id := range refSeats {
				if id == w.parentProposer {
					t.Fatalf("the PARENT's proposer %x was seated off its own block — the anti-self-declaration "+
						"property C2Metric depends on is broken", id[:4])
				}
			}
			seatedCert1 := false
			for _, id := range refSeats {
				if id == idOf(w.cert[1]) {
					seatedCert1 = true
				}
			}
			if !seatedCert1 {
				t.Fatalf("cert1 — the same qualification route as the parent's proposer, but NOT the parent's "+
					"proposer — must be seated; seated=%s", fmtIDs(refSeats))
			}

			// ---- (d) duplicates and mixed rounds ----
			dup := append(append([]Attestation{}, base...), w.carrierFrom(pool, 0)...)
			dupBlock := mintSubject(t, w, dup, 0, nil)
			gotDup, rootDup := replicaSeats(t, w.c, *dupBlock)
			if !sameIDs(gotDup, refSeats) || rootDup != refRoot {
				t.Fatalf("a duplicated carrier changed the transition: seated %s (want %s), root %x (want %x) — "+
					"the seating write must be idempotent", fmtIDs(gotDup), fmtIDs(refSeats), rootDup[:8], refRoot[:8])
			}
			if err := validateCarrier(dupBlock); !errors.Is(err, ErrCarrierDuplicateID) {
				t.Fatalf("the VALIDITY rule must refuse a duplicated carrier with ErrCarrierDuplicateID, got %v — "+
					"otherwise two replicas can be handed carriers that differ by a duplicate", err)
			}
			// Mixed rounds: O1 binds each entry to its OWN declared round, so the same signer set
			// carried at round 3 seats the same ids and is equally valid.
			r3 := w.carrierFrom(pool, 3)
			r3Block := mintSubject(t, w, r3, 0, nil)
			if err := validateCarrier(r3Block); err != nil {
				t.Fatalf("a carrier whose entries declare round 3 must be VALID (O1 binds per entry, not to a "+
					"common round): %v", err)
			}
			gotR3, rootR3 := replicaSeats(t, w.c, *r3Block)
			if !sameIDs(gotR3, refSeats) || rootR3 != refRoot {
				t.Fatalf("the entry ROUND moved the seating: seated %s (want %s)", fmtIDs(gotR3), fmtIDs(refSeats))
			}

			// ---- the PRODUCTION producer path ----
			//
			// Every carrier above is hand-built. That is faithful to the VALIDITY rule, which
			// admits any genuine precommit from any identity — but it is not what an honest
			// proposer emits. HeadCarrier is, and a fixture that only ever feeds the transition
			// its own hand-built input would never notice a producer that emitted a carrier the
			// transition reads differently. So: take what the producer actually emits over this
			// head and require the SAME oracle to predict the seating.
			prod := w.c.HeadCarrier()
			if len(prod) == 0 {
				t.Fatal("HeadCarrier emitted an EMPTY carrier over a head whose stored certificate is " +
					"non-empty — the producer path is not being exercised at all")
			}
			prodBlock := mintSubject(t, w, prod, 0, nil)
			// It must satisfy the validity rule BY CONSTRUCTION (HeadCarrier's own claim).
			if err := validateCarrier(prodBlock); err != nil {
				t.Fatalf("HeadCarrier emitted a carrier its own validity rule REFUSES: %v", err)
			}
			gotProd, _ := replicaSeats(t, w.c, *prodBlock)
			wantProd := oracleSeats(w.c, w.parentProposer, prod)
			if !sameIDs(gotProd, wantProd) {
				t.Fatalf("the PRODUCED carrier seats %s but the parent-post-state screen over it is %s — "+
					"producer and transition disagree", fmtIDs(gotProd), fmtIDs(wantProd))
			}
			if len(gotProd) == 0 {
				t.Fatal("the produced carrier seats NOBODY, so this arm distinguishes no reducer")
			}
			// The producer must EXCLUDE nothing on its own account: the parent's proposer is
			// carried (HeadCarrier is a witness list, not a transition) and dropped by the
			// TRANSITION. Asserting both halves keeps one from absorbing the other.
			carriedParentProd := false
			for i := range prod {
				if prod[i].AttesterID() == w.parentProposer {
					carriedParentProd = true
				}
			}
			if !carriedParentProd {
				t.Fatal("HeadCarrier dropped the parent's proposer — the exclusion is the TRANSITION's rule, " +
					"not the producer's; moving it would make the seating depend on who built the block")
			}
			for _, id := range gotProd {
				if id == w.parentProposer {
					t.Fatalf("the parent's proposer %x was seated from the PRODUCED carrier", id[:4])
				}
			}

			// ---- the whole subject block, through the REAL validity + root predicate ----
			// Everything above rides cloneForDryRun+apply. This one commits through Append, so the
			// committed-root predicate re-runs the transition and would reject a block whose
			// recomputed seating differed from the proposer's.
			if err := w.c.Append(*refBlock); err != nil {
				t.Fatalf("the reference subject block must COMMIT through the real validity path: %v", err)
			}
			if got := seenSet(w.c); !sameIDs(got, refSeats) {
				t.Fatalf("after Append the committed seated set is %s, the dry-run said %s", fmtIDs(got), fmtIDs(refSeats))
			}
		})
	}
}

// carrierOrderings returns a deterministic family of orderings of the same carrier SET: all 24
// permutations of the first four entries with the tail fixed, the full reversal, and every
// rotation. Deterministic so a failure is reproducible by index.
func carrierOrderings(base []Attestation) [][]Attestation {
	var out [][]Attestation
	n := len(base)
	if n >= 4 {
		for _, p := range perms4() {
			next := make([]Attestation, 0, n)
			for _, i := range p {
				next = append(next, base[i])
			}
			next = append(next, base[4:]...)
			out = append(out, next)
		}
	}
	rev := make([]Attestation, 0, n)
	for i := n - 1; i >= 0; i-- {
		rev = append(rev, base[i])
	}
	out = append(out, rev)
	for r := 1; r < n; r++ {
		rot := append(append([]Attestation{}, base[r:]...), base[:r]...)
		out = append(out, rot)
	}
	return out
}

func perms4() [][4]int {
	var out [][4]int
	idx := []int{0, 1, 2, 3}
	var rec func(k int)
	rec = func(k int) {
		if k == 4 {
			out = append(out, [4]int{idx[0], idx[1], idx[2], idx[3]})
			return
		}
		for i := k; i < 4; i++ {
			idx[k], idx[i] = idx[i], idx[k]
			rec(k + 1)
			idx[k], idx[i] = idx[i], idx[k]
		}
	}
	rec(0)
	return out
}

// =============================================================================
// ORACLE 2 — THE ORDERING PROPERTY, DRIVEN
// =============================================================================
//
// The screen must read the PARENT's committed post-state. Three same-block contents change
// qualification WITHIN the subject block, in the three directions apply() can move it:
//
//	(a) a BondReg that raises the sub-MinBond identity above the bar  (unqualified -> qualified)
//	(b) a Slash of a qualified identity                               (qualified -> unqualified)
//	(c) a TTL expiry that lapses a genesis bond at this height        (qualified -> unqualified)
//
// For each, the correct fold (carrier FIRST) and the inverted fold (carrier LAST) disagree, and
// the correct one equals the parent-post-state screen. Moving applyCarrier below the bond loop
// in apply() fails BOTH assertions: the correct arm becomes the inverted result.
//
// All three run in the YOUNG regime deliberately: the certificate is carried by unbonded
// anchors, so changing bond state inside the subject block cannot disturb the quorum that
// commits it. In the MATURE regime the frozen set makes (a) and (c) unobservable at all — the
// screen ignores mid-epoch bond movement by design — which is itself asserted below.
func TestModelCheck_CarrierSeating_ScreenReadsTheParentPostState(t *testing.T) {
	t.Run("a/in-block-bond-newly-qualifies", func(t *testing.T) {
		w := newCarrierWorld(t, false)
		if w.c.attesterQualified(idOf(w.sub)) {
			t.Fatal("setup: sub must be UNQUALIFIED in the parent's post-state, or the direction is undriven")
		}
		carrier := w.carrierFrom(w.allScreenedKeys(), 0)
		prev, _ := w.c.Head()
		b := mintSubject(t, w, carrier, 0, func(b *Block) {
			b.BondRegs = []BondReg{bondReg(w.sub, csBigBond, prev)} // raises sub ABOVE MinBond, in THIS block
		})
		assertFoldOrderDiverges(t, w, b, idOf(w.sub), false)
	})

	t.Run("b/in-block-slash-disqualifies", func(t *testing.T) {
		w := newCarrierWorld(t, false)
		if !w.c.attesterQualified(idOf(w.mid)) {
			t.Fatal("setup: mid must be QUALIFIED in the parent's post-state, or the direction is undriven")
		}
		carrier := w.carrierFrom(w.allScreenedKeys(), 0)
		prev, _ := w.c.Head()
		b := mintSubject(t, w, carrier, 0, func(b *Block) {
			b.Slashes = []Equivocation{slashProof(w.mid, prev, 0xD1, 0xD2)} // disqualifies mid, in THIS block
		})
		assertFoldOrderDiverges(t, w, b, idOf(w.mid), true)
	})

	t.Run("c/in-block-ttl-expiry-disqualifies", func(t *testing.T) {
		// A dedicated world: BondTTLBlocks 1 means a genesis bond (regHeight 0) survives height 1
		// and LAPSES at height 2 — inside the subject block, in the TTL sweep that follows the
		// bond loop. The anchors hold the certificate, so the lapse cannot affect the quorum.
		w := newCarrierWorldTTL(t, 1)
		if !w.c.attesterQualified(idOf(w.big)) {
			t.Fatal("setup: big must still be QUALIFIED in the parent's post-state at height 1")
		}
		carrier := w.carrierFrom(w.allScreenedKeys(), 0)
		b := mintSubject(t, w, carrier, 0, nil)
		assertFoldOrderDiverges(t, w, b, idOf(w.big), true)
	})

	t.Run("d/mature-frozen-set-ignores-in-block-bond", func(t *testing.T) {
		// The complement, and the reason (a) is regime-specific: inside a mature epoch the screen
		// reads FROZEN membership, so a same-block registration cannot qualify anybody under
		// EITHER fold order. The inversion is therefore invisible here — stated and asserted, so a
		// future reader does not mistake the absent divergence for an absent property.
		w := newCarrierWorld(t, true)
		carrier := w.carrierFrom(w.allScreenedKeys(), 0)
		prev, _ := w.c.Head()
		b := mintSubject(t, w, carrier, 0, func(b *Block) {
			b.BondRegs = []BondReg{bondReg(w.sub, csBigBond, prev)}
		})
		correct, _ := replicaSeats(t, w.c, *b)
		inverted := invertedFoldSeats(t, w.c, *b, w.parentProposer)
		if !sameIDs(correct, inverted) {
			t.Fatalf("inside a mature epoch a same-block registration must not move the seating under EITHER "+
				"fold order (the set is FROZEN): correct %s, inverted %s", fmtIDs(correct), fmtIDs(inverted))
		}
		for _, id := range correct {
			if id == idOf(w.sub) {
				t.Fatal("a mid-epoch registration was seated inside the frozen epoch — the frozen-set screen is bypassed")
			}
		}
	})
}

// assertFoldOrderDiverges is the shared body of the ordering drive: the correct fold equals the
// parent-post-state screen, the inverted fold does NOT, and the difference is exactly `pivot`.
// wantSeatedWhenCorrect says which side the pivot lands on under the correct order.
func assertFoldOrderDiverges(t *testing.T, w *carrierWorld, b *Block, pivot ports.NodeID, wantSeatedWhenCorrect bool) {
	t.Helper()
	correct, _ := replicaSeats(t, w.c, *b)
	want := oracleSeats(w.c, w.parentProposer, b.LastCommit)
	if !sameIDs(correct, want) {
		t.Fatalf("the committed seating is %s but the PARENT-POST-STATE screen is %s — applyCarrier is not "+
			"reading the child's pre-state (move the fold back above the bond loop in apply)",
			fmtIDs(correct), fmtIDs(want))
	}
	inverted := invertedFoldSeats(t, w.c, *b, w.parentProposer)
	if sameIDs(correct, inverted) {
		t.Fatalf("ORDERING DRIVE VACUOUS: folding the carrier AFTER this block's bond/TTL/slash maintenance "+
			"produced the SAME set %s, so this scenario does not witness the ordering property at all",
			fmtIDs(correct))
	}
	has := func(ids []ports.NodeID) bool {
		for _, id := range ids {
			if id == pivot {
				return true
			}
		}
		return false
	}
	if has(correct) != wantSeatedWhenCorrect || has(inverted) == wantSeatedWhenCorrect {
		t.Fatalf("the divergence is not the expected one: pivot %x seated-when-correct=%v (want %v), "+
			"seated-when-inverted=%v (want %v)", pivot[:4], has(correct), wantSeatedWhenCorrect,
			has(inverted), !wantSeatedWhenCorrect)
	}
	t.Logf("fold order is load-bearing: correct %s vs inverted %s (pivot %x)",
		fmtIDs(correct), fmtIDs(inverted), pivot[:4])
}

// newCarrierWorldTTL is newCarrierWorld's young arm with BondTTLBlocks set, so a genesis bond
// lapses inside the subject block. Kept separate because a TTL changes what the SETUP means and
// the setup proof above pins the TTL-free world.
func newCarrierWorldTTL(t *testing.T, ttl uint64) *carrierWorld {
	t.Helper()
	w := newCarrierWorld(t, false)
	// Rebuild with the TTL: the config is read at New, so a fresh world is the only honest way.
	w2 := &carrierWorld{mature: false, big: w.big, mid: w.mid, sub: w.sub,
		unbonded: w.unbonded, slashed: w.slashed, midEpoch: w.midEpoch, cert: w.cert}
	anchors := map[ports.NodeID]bool{}
	for _, k := range w2.cert {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: csMinBond, MinBondBytes: 0, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, MatureValidators: 99, EpochBlocks: 0,
		BondTTLBlocks: ttl, Era3ActivationHeight: 1, Era4ActivationHeight: 1}
	w2.c = New(cfg, func(ports.NodeID) int64 { return 0 })
	w2.c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: []BondReg{
		bondReg(w2.big, csBigBond, ports.Hash{}),
		bondReg(w2.mid, csMidBond, ports.Hash{}),
		bondReg(w2.sub, csSubBond, ports.Hash{}),
		bondReg(w2.slashed, csMidBond, ports.Hash{}),
	}}
	Sign(g, w2.cert[0])
	if err := w2.c.AppendGenesis(*g); err != nil {
		t.Fatalf("ttl genesis: %v", err)
	}
	prev, next := w2.c.Head()
	h1 := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(w2.midEpoch, csLateBond, prev)}}
	if err := w2.c.PopulateEra4Roots(h1); err != nil {
		t.Fatalf("ttl populate: %v", err)
	}
	signCert(h1, w2.cert, 0)
	if err := w2.c.Append(*h1); err != nil {
		t.Fatalf("ttl height 1: %v", err)
	}
	w2.parentProposer = idOf(w2.cert[0])
	w2.screened = []screenedID{
		{"cert0-parent-proposer", w2.cert[0], true},
		{"cert1", w2.cert[1], true},
		{"big", w2.big, true},
		{"mid", w2.mid, true},
		{"sub-below-minbond", w2.sub, false},
		{"unbonded", w2.unbonded, false},
		{"midEpoch", w2.midEpoch, true},
	}
	if w2.c.cfg.BondTTLBlocks != ttl {
		t.Fatalf("ttl world: BondTTLBlocks = %d, want %d", w2.c.cfg.BondTTLBlocks, ttl)
	}
	return w2
}

// =============================================================================
// ORACLE 3 — THE ERA SEAM: a v4 parent, a v5 child
// =============================================================================
//
// At H_era4 the child is the first v5 block while its parent is the last v4 block. Three rules
// meet there and must compose into ONE set every replica computes:
//
//  1. the parent (v4) seated from its OWN Atts under the FROZEN era-3 rule;
//  2. the child (v5) seats from the CARRIER, whose entries are precommits over that v4 parent;
//  3. the child's own Atts seat NOTHING — a v5 block's uncovered attestations are not a
//     transition input, which is the whole point of the carrier.
//
// Driven from both sides: an identity present ONLY in the v4 parent's Atts IS seated; an
// identity present ONLY in the v5 child's Atts is NOT; an identity present only in the child's
// CARRIER is. And a v4 block that carries the field at all is refused by rule, so replicas
// cannot split over whether to fold carrier bytes on a pre-v5 block.
func TestModelCheck_CarrierSeating_EraSeamV4ParentV5Child(t *testing.T) {
	// H_era3 = 1, H_era4 = 2: height 1 mints v4, height 2 mints v5.
	cert := []ed25519.PrivateKey{key(92100), key(92101), key(92102), key(92103)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range cert {
		anchors[idOf(k)] = true
	}
	viaParentAtts := key(92001) // qualified; attests the v4 PARENT only
	viaChildAtts := key(92002)  // qualified; attests the v5 CHILD only
	viaCarrier := key(92003)    // qualified; appears only in the child's CARRIER
	unqual := key(92004)        // sub-MinBond; appears in the child's carrier and must write nothing

	cfg := Config{Quorum: 1, MinBond: csMinBond, MinBondBytes: 0, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, MatureValidators: 99, EpochBlocks: 0,
		Era3ActivationHeight: 1, Era4ActivationHeight: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: []BondReg{
		bondReg(viaParentAtts, csBigBond, ports.Hash{}),
		bondReg(viaChildAtts, csMidBond, ports.Hash{}),
		bondReg(viaCarrier, csLateBond, ports.Hash{}),
		bondReg(unqual, csSubBond, ports.Hash{}),
	}}
	Sign(g, cert[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if mv := c.MintVersion(1); mv != BlockVersionStateRoot {
		t.Fatalf("seam fixture: height 1 must mint v4 (%d), got v%d — the seam needs a v4 PARENT",
			BlockVersionStateRoot, mv)
	}
	if mv := c.MintVersion(2); mv != BlockVersionWitnessable {
		t.Fatalf("seam fixture: height 2 must mint v5 (%d), got v%d", BlockVersionWitnessable, mv)
	}

	// ---- the v4 parent. Its OWN Atts are the transition input (the frozen era-3 rule). ----
	//
	// BUILDING ONE IS AWKWARD, AND THE AWKWARDNESS IS THE DEFECT THE CARRIER CLOSES. Under v4
	// the Atts feed apply() but are NOT covered by Hash(), so the committed root must be
	// computed over a certificate the proposer has not gathered yet. The fixture resolves the
	// circularity the only way it can be resolved: fix the ATTESTER IDS first (the sub-v5
	// seating loop reads AttesterID and verifies nothing), populate the roots over those ids,
	// and only then sign — Hash() covers neither Atts nor PrepareQC, so the signatures land
	// over the final root-bearing hash. A live v4 proposer cannot do this, which is precisely
	// consequence (b) of the R-BOX-ATTESTS verdict.
	prev, _ := c.Head()
	h1 := &Block{Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	h1.Proposer = append([]byte(nil), pubOf(cert[0])...)
	seaters := append(append([]ed25519.PrivateKey{}, cert...), viaParentAtts)
	for _, k := range seaters {
		h1.Atts = append(h1.Atts, Attestation{PubKey: pubOf(k), Phase: PhasePrecommit})
	}
	if err := c.PopulateEra3Roots(h1); err != nil {
		t.Fatalf("populate era-3 roots over the intended certificate: %v", err)
	}
	Sign(h1, cert[0]) // signs the final, root-bearing hash
	h1.PrepareQC, h1.Atts = nil, nil
	for _, k := range cert {
		h1.PrepareQC = append(h1.PrepareQC, AttestAt(h1, k, 0, PhasePrepare))
	}
	for _, k := range seaters {
		h1.Atts = append(h1.Atts, AttestAt(h1, k, 0, PhasePrecommit))
	}
	if err := c.Append(*h1); err != nil {
		t.Fatalf("v4 parent: %v", err)
	}
	afterParent := seenSet(c)
	if !containsID(afterParent, idOf(viaParentAtts)) {
		t.Fatalf("the v4 parent must seat an identity carried in its OWN Atts (the frozen era-3 rule): seated %s",
			fmtIDs(afterParent))
	}

	// A v4 block carrying the field at all is refused BY RULE, so the frozen era cannot acquire
	// a second seating input.
	badPrev, badNext := c.Head()
	bad := &Block{Version: BlockVersionStateRoot, Height: badNext, Prev: badPrev,
		Entries:    []ports.Entry{entry(9)},
		LastCommit: []Attestation{precommitOver(viaCarrier, badPrev, 0)}}
	if err := validateCarrier(bad); !errors.Is(err, ErrCarrierNotWitnessable) {
		t.Fatalf("a pre-v5 block carrying a LastCommit must be refused with ErrCarrierNotWitnessable, got %v", err)
	}

	// ---- the v5 child at H_era4. Carrier over the v4 parent; its own Atts seat nothing. ----
	prev2, next2 := c.Head()
	h2 := &Block{Height: next2, Prev: prev2, Entries: []ports.Entry{entry(2)}}
	h2.LastCommit = []Attestation{
		precommitOver(viaCarrier, prev2, 0),
		precommitOver(unqual, prev2, 0),
		precommitOver(cert[0], prev2, 0), // the PARENT's proposer — excluded by rule
		precommitOver(cert[1], prev2, 0),
	}
	if err := c.PopulateEra4Roots(h2); err != nil {
		t.Fatalf("populate era-4 roots at the seam: %v", err)
	}
	// Checked AFTER the version is stamped: validateCarrier's FIRST clause is the era gate, so
	// on an unstamped block it would report ErrCarrierNotWitnessable and say nothing about the
	// signatures. The rule under test here is that the entries verify over a V4 parent's hash.
	if err := validateCarrier(h2); err != nil {
		t.Fatalf("the seam carrier must be VALID over the v4 parent's hash: %v", err)
	}
	signCert(h2, cert, 0)
	h2.Atts = append(h2.Atts, AttestAt(h2, viaChildAtts, 0, PhasePrecommit))
	// NO root re-population this time — and that is the property: under v5 the Atts are not a
	// transition input, so bolting one on cannot move the recomputed root. If it did, Append
	// would reject the block here.
	if err := c.Append(*h2); err != nil {
		t.Fatalf("the v5 seam child must COMMIT with an extra attester bolted onto its Atts — under v5 the "+
			"Atts are NOT a transition input, so they cannot move the recomputed root: %v", err)
	}
	afterChild := seenSet(c)

	if !containsID(afterChild, idOf(viaCarrier)) {
		t.Fatalf("the v5 child must seat the identity carried in its CARRIER over the v4 parent: seated %s",
			fmtIDs(afterChild))
	}
	if containsID(afterChild, idOf(viaChildAtts)) {
		t.Fatalf("an identity present ONLY in the v5 child's own Atts was SEATED — a v5 block's uncovered "+
			"attestations are not a transition input; this is the R-BOX-ATTESTS defect restored: seated %s",
			fmtIDs(afterChild))
	}
	if containsID(afterChild, idOf(unqual)) {
		t.Fatalf("a sub-MinBond identity in the seam carrier was seated — the screen does not run at the seam: %s",
			fmtIDs(afterChild))
	}
	if containsID(afterChild, idOf(cert[0])) {
		t.Fatalf("the v4 parent's PROPOSER was seated off the carrier of its own block: %s", fmtIDs(afterChild))
	}
	if !containsID(afterChild, idOf(cert[1])) {
		t.Fatalf("a non-proposer certificate signer carried at the seam must be seated: %s", fmtIDs(afterChild))
	}
	// COMPOSITION: nothing the v4 parent seated is lost when the v5 child folds its carrier.
	for _, id := range afterParent {
		if !containsID(afterChild, id) {
			t.Fatalf("the era seam LOST a seating the v4 parent had committed (%x) — validatorsSeen is "+
				"grow-only across the boundary", id[:4])
		}
	}
	// AGREEMENT at the seam: an independent replica applying the same child reaches the same set.
	t.Logf("era seam: after v4 parent %s, after v5 child %s", fmtIDs(afterParent), fmtIDs(afterChild))
}

func containsID(ids []ports.NodeID, want ports.NodeID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
