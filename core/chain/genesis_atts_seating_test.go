package chain

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Genesis attestation seating rule — gates G1–G6, G7b, G10 (RED-first on main).
//
// Spec: genesis-atts-seating-rule-RESEARCH-CERTIFICATION-2026-09-04.md §4.1 (rule text)
// and §8 (the gate table). Owner-ratified 2026-09-04 ("I ratify 1").
//
// THE RULE. In AppendGenesis, after the proposer-signature check and before c.apply(b):
// keep exactly the entries a of b.Atts with verifyAtt(a, b.Hash()), in a fresh slice;
// assign it to b.Atts; never return an error on this account. The committed
// blocks[0].Atts is therefore the verified subset. apply's seating predicate
// (id != ProposerID() && attesterQualified(id)) is unchanged, so the seated set is
// verified ∧ qualified ∧ non-proposer — the same predicate the committed lane uses.
// LastCommit on genesis stays REFUSED (hash-covered; TestGenesisLastCommitIsRefused = G9).
//
// ABLATION TEETH (§8): revert the filter (seat from raw Atts) ⇒ G1–G6 RED; replace it
// with `b.Atts = nil` ⇒ G8 (the four core/node A11 fixtures) and G2 RED; replace it with
// a refusal ⇒ G1/G3/G4 RED.
//
// "Stub" = one attestation with a QUALIFIED id (a rep-qualified key in legacy mode; an
// anchor key in objective mode) and a 64-zero-byte signature, appended AFTER Sign — the
// in-transit mutation every relaying peer can perform, because Atts is outside the
// Hash() preimage (hash_literal_pin_test.go). Every fixture asserts that premise.
//
// G7 (TestProductionGenesisCarriesNoAtts) lives in core/genesis (import direction).
// G8 is the four core/node fixtures, unedited. G10 is re-targeted: Weight() was deleted
// with O3 Direction T (PR #722), so the fork-choice pin is "same Hash(), same heavier
// outcome against a common competitor".

// gasStub is a signature nobody made: priv's REAL public key with 64 zero bytes.
func gasStub(priv ed25519.PrivateKey) Attestation {
	return Attestation{PubKey: pubOf(priv), Sig: make([]byte, 64)}
}

// gasForeignAtt is a REAL signature by priv over a DIFFERENT block's hash (a replayed
// attestation) — present, well-formed, and false under verifyAtt for g.
func gasForeignAtt(priv ed25519.PrivateKey) Attestation {
	other := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(200)}}
	return Attest(other, priv)
}

// gasLegacyGenesis builds (does not append) a proposer-signed era-1 genesis on w.
func gasLegacyGenesis(w *world) *Block {
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, w.prop)
	return g
}

// gasWithAtts returns a copy of g carrying atts appended AFTER signing, asserting the
// premise that the hash (and so the proposer signature) is unchanged.
func gasWithAtts(t *testing.T, g *Block, atts ...Attestation) Block {
	t.Helper()
	pre := g.Hash()
	v := *g
	v.Atts = append(append([]Attestation(nil), g.Atts...), atts...)
	v.hashMemoSet = false
	if v.Hash() != pre {
		t.Fatal("premise: Atts must be outside the Hash() preimage for this attack to exist — if that changed, re-derive these gates from hash_literal_pin_test.go")
	}
	return v
}

// gasSeated returns the seated set as sorted hex ids (white-box read of validatorsSeen).
func gasSeated(c *Chain) []string {
	out := make([]string, 0, len(c.validatorsSeen))
	for id := range c.validatorsSeen {
		out = append(out, fmt.Sprintf("%x", id))
	}
	sort.Strings(out)
	return out
}

func gasIDs(keys ...ed25519.PrivateKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%x", idOf(k)))
	}
	sort.Strings(out)
	return out
}

// gasAnchorCfg is the objective-mode launch config: four anchors, MinBond set, Byzantine
// quorum, no epochs, optional genesis-declared era-3 boundary. With four anchors and no
// bonds a commit needs the proposer + 2 anchor attesters (n−f = 3 = strict anchor majority).
func gasAnchorCfg(keys []ed25519.PrivateKey, era3At uint64, matureValidators int) Config {
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	return Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		AnchorQuorum: 3, BondTTLBlocks: 40, MatureValidators: matureValidators,
		Era3ActivationHeight: era3At}
}

func gasAnchorKeys() []ed25519.PrivateKey {
	return []ed25519.PrivateKey{key(53101), key(53102), key(53103), key(53104)}
}

// gasAnchorChain builds an objective replica (bond verifier wired) with NOTHING appended.
func gasAnchorChain(cfg Config) *Chain {
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c
}

// gasAnchorGenesis is the PRODUCTION-SHAPED genesis (Entries only, no BondRegs, no Atts),
// proposed by keys[0]. Anchors qualify at height 0 through launchAnchor alone (§2 A8).
func gasAnchorGenesis(keys []ed25519.PrivateKey) *Block {
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, keys[0])
	return g
}

func gasRoots(t *testing.T, c *Chain) (ports.Hash, ports.Hash) {
	t.Helper()
	r4, err := c.StateRootForVersion(BlockVersionStateRoot)
	if err != nil {
		t.Fatalf("StateRootForVersion(4): %v", err)
	}
	r5, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion(5): %v", err)
	}
	return r4, r5
}

// gasAssertGenesisStripped: the committed genesis carries no stub and the stub's id was
// never seated.
func gasAssertStubGone(t *testing.T, c *Chain, stubKey ed25519.PrivateKey, path string) {
	t.Helper()
	if c.Len() == 0 {
		t.Fatalf("%s: no genesis committed", path)
	}
	for _, a := range c.Blocks(0)[0].Atts {
		if a.AttesterID() == idOf(stubKey) {
			t.Errorf("%s: committed blocks[0].Atts still carries the stub (a signature nobody made) — the unverified entry was NOT STRIPPED before apply (rule §4.1: keep only verifyAtt(a, Hash()))", path)
		}
	}
	if c.validatorsSeen[idOf(stubKey)] {
		t.Errorf("%s: validatorsSeen was PRE-SEATED from an unsigned stub attestation — the maturity metric counted a signature nobody made", path)
	}
}

// ---------------------------------------------------------------------------------------
// G1 — TestGenesisStubAttsAreStrippedNotFatal: genesis + stub ⇒ AppendGenesis returns
// nil; Regime().ValidatorsSeen == 0; Blocks(0)[0].Atts is empty. Two arms: legacy
// (rep-qualified stub key) and objective (anchor stub key on a production-shaped genesis).
// ---------------------------------------------------------------------------------------

func TestGenesisStubAttsAreStrippedNotFatal(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		w := newWorld(DefaultConfig())
		stubKey := key(31000)
		w.reps[idOf(stubKey)] = 1000 // qualified by reputation, exactly like w.vals
		g := gasWithAtts(t, gasLegacyGenesis(w), gasStub(stubKey))
		if err := w.c.AppendGenesis(g); err != nil {
			t.Fatalf("AppendGenesis REFUSED a genesis carrying one unsigned stub Att: %v — an unsigned field must never be a refusal lever (strip, never refuse)", err)
		}
		if n := w.c.Regime().ValidatorsSeen; n != 0 {
			t.Errorf("Regime().ValidatorsSeen = %d, want 0 — seated from a signature nobody made", n)
		}
		if n := len(w.c.Blocks(0)[0].Atts); n != 0 {
			t.Errorf("committed genesis carries %d Atts, want 0 — the unverified stub was NOT STRIPPED", n)
		}
		gasAssertStubGone(t, w.c, stubKey, "AppendGenesis/legacy")
	})
	t.Run("objective-anchor", func(t *testing.T) {
		keys := gasAnchorKeys()
		c := gasAnchorChain(gasAnchorCfg(keys, 0, 99))
		stubKey := keys[3] // an anchor: the ONLY key a relayer can seat on a production genesis (§3)
		g := gasWithAtts(t, gasAnchorGenesis(keys), gasStub(stubKey))
		if err := c.AppendGenesis(g); err != nil {
			t.Fatalf("AppendGenesis REFUSED a production-shaped genesis carrying one unsigned anchor stub: %v", err)
		}
		if n := c.Regime().ValidatorsSeen; n != 0 {
			t.Errorf("Regime().ValidatorsSeen = %d, want 0 — an anchor was seated from a signature nobody made", n)
		}
		if n := len(c.Blocks(0)[0].Atts); n != 0 {
			t.Errorf("committed genesis carries %d Atts, want 0 — the unverified stub was NOT STRIPPED", n)
		}
		gasAssertStubGone(t, c, stubKey, "AppendGenesis/objective")
	})
}

// ---------------------------------------------------------------------------------------
// G2 — seated count equals verified count. Genesis with k=2 verified qualified
// non-proposer atts (one era-1 bare-hash, one era-2 round-0 precommit: accepted under
// whichever era each declares) + m=2 stubs (a zero signature; a real signature over a
// foreign hash) + one verified-but-unqualified att + one verified proposer self-att.
// Seated set == exactly the k ids; Blocks(0)[0].Atts has exactly k+2 entries (the
// verified subset — unqualified and self are KEPT but not seated, matching :2927-2928).
// ---------------------------------------------------------------------------------------

func TestGenesisSeatedCountEqualsVerifiedCount(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := gasLegacyGenesis(w)
	k1, k2 := w.vals[0], w.vals[1] // verified, qualified (rep 1000), non-proposer
	stub1, stub2 := key(31000), key(31001)
	w.reps[idOf(stub1)], w.reps[idOf(stub2)] = 1000, 1000 // qualified: a stub of an unqualified key proves nothing
	unq := key(31002)                                     // verified-but-unqualified: rep 0 < MinAttesterRep
	w.reps[idOf(unq)] = 0

	verifiedK1 := Attest(g, k1)                      // era-1 form
	verifiedK2 := AttestAt(g, k2, 0, PhasePrecommit) // era-2 form (the A11 fixture convention)
	verifiedUnq := Attest(g, unq)
	verifiedSelf := Attest(g, w.prop)
	v := gasWithAtts(t, g, gasStub(stub1), verifiedK1, gasForeignAtt(stub2), verifiedUnq, verifiedK2, verifiedSelf)
	for i, a := range []Attestation{verifiedK1, verifiedK2, verifiedUnq, verifiedSelf} {
		if !verifyAtt(a, g.Hash()) {
			t.Fatalf("premise: verified att %d does not verify", i)
		}
	}
	for i, a := range []Attestation{gasStub(stub1), gasForeignAtt(stub2)} {
		if verifyAtt(a, g.Hash()) {
			t.Fatalf("premise: stub %d verifies", i)
		}
	}

	if err := w.c.AppendGenesis(v); err != nil {
		t.Fatalf("AppendGenesis refused: %v", err)
	}
	// Seated set: exactly the verified ∧ qualified ∧ non-proposer ids.
	want := gasIDs(k1, k2)
	if got := gasSeated(w.c); !reflect.DeepEqual(got, want) {
		t.Errorf("seated set != verified∧qualified∧non-proposer set\n got  %v\n want %v\n(the stubs %v must not seat; the unqualified %v and the proposer %v never seat)",
			got, want, gasIDs(stub1, stub2), gasIDs(unq), gasIDs(w.prop))
	}
	if n := w.c.Regime().ValidatorsSeen; n != 2 {
		t.Errorf("Regime().ValidatorsSeen = %d, want 2 (k)", n)
	}
	// Committed Atts: exactly the verified subset (k + unqualified + self = 4), in a fresh slice.
	committed := w.c.Blocks(0)[0].Atts
	if len(committed) != 4 {
		t.Errorf("committed blocks[0].Atts has %d entries, want 4 (the verified subset: k=2 + unqualified + proposer self-att); a strip-ALL rule gives 0, the unfiltered rule gives 6", len(committed))
	}
	for i, a := range committed {
		if !verifyAtt(a, g.Hash()) {
			t.Errorf("committed blocks[0].Atts[%d] does NOT verify over the genesis hash — an unverified entry survived into the committed genesis", i)
		}
	}
	wantKept := gasIDs(k1, k2, unq, w.prop)
	gotKept := make([]string, 0, len(committed))
	for _, a := range committed {
		gotKept = append(gotKept, fmt.Sprintf("%x", a.AttesterID()))
	}
	sort.Strings(gotKept)
	if !reflect.DeepEqual(gotKept, wantKept) && len(committed) == 4 {
		t.Errorf("committed Atts ids\n got  %v\n want %v", gotKept, wantKept)
	}
}

// ---------------------------------------------------------------------------------------
// G3 — TestReloadSurvivesAGenesisWithAStubAtt: the own-disk path. Persist
// [genesis+stub, b1, b2] (EncodeBlocks, the bytes chainstore writes), Reload into a
// fresh chain: returns n+1, nil; stub not seated; reloaded blocks[0].Atts has the stub
// removed; Regime() equals a CLEAN reload's. Legacy arm and objective (anchor) arm.
// ---------------------------------------------------------------------------------------

func TestReloadSurvivesAGenesisWithAStubAtt(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		w := newWorld(DefaultConfig())
		w.genesis()
		for i := byte(1); i <= 2; i++ {
			b := w.block(entry(i))
			w.attestAll(b)
			if err := w.c.Append(*b); err != nil {
				t.Fatal(err)
			}
		}
		clean := w.c.Blocks(0)
		stubKey := key(31000)
		w.reps[idOf(stubKey)] = 1000
		stubbed := append([]Block(nil), clean...)
		stubbed[0] = gasWithAtts(t, &clean[0], gasStub(stubKey))
		rep := func(n ports.NodeID) int64 { return w.reps[n] }
		gasReloadPair(t, New(DefaultConfig(), rep), New(DefaultConfig(), rep), clean, stubbed, stubKey)
	})
	t.Run("objective-anchor", func(t *testing.T) {
		keys := gasAnchorKeys()
		cfg := gasAnchorCfg(keys, 0, 99)
		src := gasAnchorChain(cfg)
		if err := src.AppendGenesis(*gasAnchorGenesis(keys)); err != nil {
			t.Fatal(err)
		}
		mustAppend(t, src, mintNext(t, src, keys[:3])) // v2; keys[3] is the dead-at-launch anchor
		mustAppend(t, src, mintNext(t, src, keys[:3]))
		clean := src.Blocks(0)
		stubKey := keys[3]
		stubbed := append([]Block(nil), clean...)
		stubbed[0] = gasWithAtts(t, &clean[0], gasStub(stubKey))
		gasReloadPair(t, gasAnchorChain(cfg), gasAnchorChain(cfg), clean, stubbed, stubKey)
	})
}

func gasReloadPair(t *testing.T, cleanChain, stubChain *Chain, clean, stubbed []Block, stubKey ed25519.PrivateKey) {
	t.Helper()
	persisted, err := DecodeBlocks(EncodeBlocks(stubbed))
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted[0].Atts) != len(clean[0].Atts)+1 {
		t.Fatal("premise: the stub must survive the persisted encoding (it is on the wire and on disk)")
	}
	if n, err := cleanChain.Reload(clean); err != nil || n != len(clean) {
		t.Fatalf("clean Reload: n=%d err=%v", n, err)
	}
	n, err := stubChain.Reload(persisted)
	if err != nil {
		t.Fatalf("Reload WEDGED on a persisted genesis carrying one unsigned stub Att (applied %d of %d): %v — a node whose disk genesis ever acquired a stub never restarts", n, len(persisted), err)
	}
	if n != len(persisted) {
		t.Fatalf("Reload applied %d blocks, want %d", n, len(persisted))
	}
	gasAssertStubGone(t, stubChain, stubKey, "Reload")
	if got, want := len(stubChain.Blocks(0)[0].Atts), len(clean[0].Atts); got != want {
		t.Errorf("reloaded blocks[0].Atts has %d entries, want %d (the stub removed on the way in)", got, want)
	}
	if got, want := stubChain.Regime(), cleanChain.Regime(); got != want {
		t.Errorf("Regime() after reloading the stubbed disk != clean reload\n got  %+v\n want %+v", got, want)
	}
}

// ---------------------------------------------------------------------------------------
// G4 — TestGenesisStubAttDoesNotSurviveForkAdopt: the relay path. Reconcile re-runs
// tmp.AppendGenesis(fork[0]) on the PEER's bytes and adopt copies validatorsSeen from tmp.
// Legacy arm: a taller fork with a stubbed fork[0] is adopted, stub not seated, adopted
// blocks[0].Atts clean. Era-3 arm: with v4 fork blocks, the stubbed fork adopts iff the
// clean fork adopts, and the post-adopt StateRootForVersion(4) equals the clean adopt's
// (a pre-seated dead-at-launch anchor is a validatorsSeen leaf, so the victim's recompute
// of block 1's root diverges — the §3 era-3 price, which the rule closes).
// ---------------------------------------------------------------------------------------

func TestGenesisStubAttDoesNotSurviveForkAdopt(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		w := newWorld(DefaultConfig())
		g := w.genesis()
		if err := w.c.Append(*w.forkBlock(g.Hash(), entry(1), 3)); err != nil {
			t.Fatal(err)
		}
		stubKey := key(31000) // signs nothing else in this test; w.vals legitimately attest the fork
		w.reps[idOf(stubKey)] = 1000
		relayed := gasWithAtts(t, g, gasStub(stubKey))
		f1 := w.forkBlock(g.Hash(), entry(2), 3)
		f2 := w.blockAt(f1.Hash(), 2, entry(3), 3) // taller ⇒ wins under height → head-hash
		adopted, err := w.c.Reconcile([]Block{relayed, *f1, *f2})
		if err != nil {
			t.Fatalf("Reconcile REFUSED a taller valid fork because its relayed genesis carries one unsigned stub Att: %v — fork-adopt is deniable by any serving peer at zero cost", err)
		}
		if !adopted {
			t.Fatal("a strictly taller fork must be adopted")
		}
		gasAssertStubGone(t, w.c, stubKey, "Reconcile/legacy")
		if n := len(w.c.Blocks(0)[0].Atts); n != 0 {
			t.Errorf("adopted blocks[0].Atts has %d entries, want 0", n)
		}
		if _, ok := w.c.LookupRoot(entry(3).Root); !ok {
			t.Error("the adopted fork's entry must be present")
		}
	})
	t.Run("era3-v4-fork", func(t *testing.T) {
		keys := gasAnchorKeys()
		cfg := gasAnchorCfg(keys, 3, 99) // heights 1-2 mint v2 (seating the live anchors), height 3 mints v4 with committed roots
		src := gasAnchorChain(cfg)
		g := gasAnchorGenesis(keys)
		if err := src.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		// Proposer keys[0] + anchors keys[1],keys[2]. Two v2 blocks first: a v4 block seats its
		// NEW attesters after its roots are computed (the A11 harness hazard), so the live
		// anchors must already be seated before the first v4 mint (era3AnchorChain shape).
		mustAppend(t, src, mintNext(t, src, keys[:3]))
		mustAppend(t, src, mintNext(t, src, keys[:3]))
		mustAppend(t, src, mintNext(t, src, keys[:3]))
		fork := src.Blocks(0)
		if fork[2].Version != BlockVersionRounds || fork[3].Version != BlockVersionStateRoot {
			t.Fatalf("premise: fork must be [g, v2, v2, v4] (got v%d at 2, v%d at 3)", fork[2].Version, fork[3].Version)
		}
		stubKey := keys[3] // the dead-at-launch anchor: attests NOTHING, so a seat is the stub's doing
		stubbedFork := append([]Block(nil), fork...)
		stubbedFork[0] = gasWithAtts(t, &fork[0], gasStub(stubKey))

		victimClean, victimStub := gasAnchorChain(cfg), gasAnchorChain(cfg)
		for _, v := range []*Chain{victimClean, victimStub} {
			if err := v.AppendGenesis(*g); err != nil {
				t.Fatal(err)
			}
		}
		adoptedClean, errClean := victimClean.Reconcile(fork)
		if errClean != nil || !adoptedClean {
			t.Fatalf("premise: the CLEAN v4 fork must adopt (adopted=%v err=%v)", adoptedClean, errClean)
		}
		adoptedStub, errStub := victimStub.Reconcile(stubbedFork)
		if errStub != nil {
			t.Fatalf("the stubbed v4 fork was REFUSED while the clean one adopted: %v — one unsigned genesis Att pre-seated a validatorsSeen leaf, so the victim's recompute of the first v4 block's committed root diverged (the §3 era-3 price; a relayer denies every v4 fork at zero cost)", errStub)
		}
		if adoptedStub != adoptedClean {
			t.Fatalf("stubbed fork adopted=%v, clean fork adopted=%v — must be equal", adoptedStub, adoptedClean)
		}
		gasAssertStubGone(t, victimStub, stubKey, "Reconcile/era3")
		c4, _ := gasRoots(t, victimClean)
		s4, _ := gasRoots(t, victimStub)
		if c4 != s4 {
			t.Errorf("post-adopt StateRootForVersion(4) differs: clean %x stubbed %x", c4[:8], s4[:8])
		}
		if got, want := victimStub.Regime(), victimClean.Regime(); got != want {
			t.Errorf("post-adopt Regime() differs\n got  %+v\n want %+v", got, want)
		}
	})
}

// ---------------------------------------------------------------------------------------
// G5 — the latch does not move on a stub. Legacy config (MinBond 0, everyone rep-qualified,
// MatureValidators 1, no anchors): genesis + stub ⇒ EverMature() == false; positive
// control: genesis + one VERIFIED non-anchor att ⇒ EverMature() == true. Objective arm:
// genesis + stub(anchor key) ⇒ C2Metric() and EverMature() unchanged from the clean genesis.
// ---------------------------------------------------------------------------------------

func TestGenesisLatchDoesNotMoveOnAStubAtt(t *testing.T) {
	legacyCfg := Config{MinProposerRep: 100, MinAttesterRep: 100, Quorum: 3, MatureValidators: 1}
	t.Run("legacy-stub", func(t *testing.T) {
		w := newWorld(legacyCfg)
		stubKey := key(31000)
		w.reps[idOf(stubKey)] = 1000
		if err := w.c.AppendGenesis(gasWithAtts(t, gasLegacyGenesis(w), gasStub(stubKey))); err != nil {
			t.Fatal(err)
		}
		if w.c.EverMature() {
			t.Error("EverMature() == true after a genesis carrying ONE unsigned stub — the one-way maturity latch (F-1) tripped on a signature nobody made")
		}
		if w.c.Mature() {
			t.Error("Mature() == true on a stub-only genesis")
		}
	})
	t.Run("legacy-verified-control", func(t *testing.T) {
		w := newWorld(legacyCfg)
		if err := w.c.AppendGenesis(gasWithAtts(t, gasLegacyGenesis(w), Attest(gasLegacyGenesis(w), w.vals[0]))); err != nil {
			t.Fatal(err)
		}
		if !w.c.EverMature() {
			t.Error("positive control: a VERIFIED non-anchor genesis attestation must seat and trip the latch at MatureValidators=1 — a strip-ALL rule (refuted §5.1) reddens this")
		}
	})
	t.Run("objective-anchor", func(t *testing.T) {
		keys := gasAnchorKeys()
		cfg := gasAnchorCfg(keys, 0, 2)
		clean, stub := gasAnchorChain(cfg), gasAnchorChain(cfg)
		g := gasAnchorGenesis(keys)
		if err := clean.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		if err := stub.AppendGenesis(gasWithAtts(t, g, gasStub(keys[3]))); err != nil {
			t.Fatal(err)
		}
		if got, want := stub.C2Metric(), clean.C2Metric(); !reflect.DeepEqual(got, want) {
			t.Errorf("C2Metric() moved on an anchor stub: got %+v want %+v", got, want)
		}
		if got, want := stub.EverMature(), clean.EverMature(); got != want {
			t.Errorf("EverMature() moved on an anchor stub: got %v want %v", got, want)
		}
	})
}

// ---------------------------------------------------------------------------------------
// G6 — served-variant determinism at height 0 (the owed height-0 twin of the served-
// variant gate; scar-uncovered-slot-decides-a-verdict): two replicas fed the clean and the
// stubbed copy of ONE genesis hold identical Regime(), StateRootForVersion(4) and (5).
// Weight() no longer exists (O3 Direction T); the ranking half is G10.
// ---------------------------------------------------------------------------------------

func TestGenesisServedVariantDeterminismAtHeightZero(t *testing.T) {
	check := func(t *testing.T, clean, stub *Chain) {
		t.Helper()
		if clean.Blocks(0)[0].Hash() != stub.Blocks(0)[0].Hash() {
			t.Fatal("premise: the two replicas must hold the same genesis hash")
		}
		if got, want := stub.Regime(), clean.Regime(); got != want {
			t.Errorf("Regime() differs between two replicas fed byte-different copies of ONE genesis\n stubbed %+v\n clean   %+v", got, want)
		}
		c4, c5 := gasRoots(t, clean)
		s4, s5 := gasRoots(t, stub)
		if c4 != s4 {
			t.Errorf("StateRootForVersion(4) differs: clean %x stubbed %x — an unsigned field decided a committed-root leaf", c4[:8], s4[:8])
		}
		if c5 != s5 {
			t.Errorf("StateRootForVersion(5) differs: clean %x stubbed %x — an unsigned field decided a committed-root leaf", c5[:8], s5[:8])
		}
	}
	t.Run("legacy", func(t *testing.T) {
		wc, ws := newWorld(DefaultConfig()), newWorld(DefaultConfig())
		stubKey := key(31000)
		wc.reps[idOf(stubKey)], ws.reps[idOf(stubKey)] = 1000, 1000
		g := gasLegacyGenesis(wc) // both worlds share prop=key(1), so the genesis is the same bytes
		if err := wc.c.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		if err := ws.c.AppendGenesis(gasWithAtts(t, g, gasStub(stubKey))); err != nil {
			t.Fatal(err)
		}
		check(t, wc.c, ws.c)
	})
	t.Run("objective-anchor", func(t *testing.T) {
		keys := gasAnchorKeys()
		cfg := gasAnchorCfg(keys, 0, 99)
		clean, stub := gasAnchorChain(cfg), gasAnchorChain(cfg)
		g := gasAnchorGenesis(keys)
		if err := clean.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		if err := stub.AppendGenesis(gasWithAtts(t, g, gasStub(keys[3]))); err != nil {
			t.Fatal(err)
		}
		check(t, clean, stub)
	})
}

// ---------------------------------------------------------------------------------------
// G7b — the archival half of G7: every committed archival fixture's block 0 carries no
// Atts and no PrepareQC (the pin that makes "no era gate" sound, §4.4). Directory
// listing, not a hand list (scar-inventory-gate-is-a-hand-list); a new era's fixture is
// covered the day it lands.
// ---------------------------------------------------------------------------------------

func TestArchivalFixturesGenesisCarriesNoAtts(t *testing.T) {
	ents, err := os.ReadDir(archivalDir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cbor") {
			continue
		}
		n++
		raw, err := os.ReadFile(filepath.Join(archivalDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		blocks, err := DecodeBlocks(raw)
		if err != nil {
			t.Fatalf("%s: decode: %v", e.Name(), err)
		}
		if len(blocks) == 0 || blocks[0].Height != 0 {
			t.Fatalf("%s: no genesis at index 0", e.Name())
		}
		if len(blocks[0].Atts) != 0 || len(blocks[0].PrepareQC) != 0 {
			t.Errorf("%s: archival genesis carries %d Atts / %d PrepareQC — the seating rule is no longer an identity transform on this archive; re-derive the no-era-gate claim (§4.4)", e.Name(), len(blocks[0].Atts), len(blocks[0].PrepareQC))
		}
	}
	if n < 4 {
		t.Fatalf("only %d archival fixtures found in %s (want the four committed eras at least)", n, archivalDir)
	}
}

// ---------------------------------------------------------------------------------------
// G10 — a stub adds no fork-choice ranking. Weight() was deleted (O3 Direction T, PR
// #722), so the pin is re-targeted: the stubbed and the clean genesis have the same
// Hash() and produce the same heavier outcome against a common competitor in all three
// head relations (taller, shorter, equal-height hash tiebreak). This overlaps the O3-T
// AST purity pin (TestO3T_HeavierReadsOnlyHeightAndHeadHash) but is the runtime pin at
// height 0 specifically; it is GREEN on main by construction under T.
// ---------------------------------------------------------------------------------------

func TestGenesisStubAttAddsNoForkChoiceRanking(t *testing.T) {
	wc, ws := newWorld(DefaultConfig()), newWorld(DefaultConfig())
	stubKey := key(31000)
	wc.reps[idOf(stubKey)], ws.reps[idOf(stubKey)] = 1000, 1000
	g := gasLegacyGenesis(wc)
	if err := wc.c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	if err := ws.c.AppendGenesis(gasWithAtts(t, g, gasStub(stubKey))); err != nil {
		t.Fatal(err)
	}
	if wc.c.Blocks(0)[0].Hash() != ws.c.Blocks(0)[0].Hash() {
		t.Fatal("stubbed and clean genesis hash differently — Atts entered the preimage")
	}
	// Competitors: taller (genesis + one block); equal height with a different genesis body.
	taller := newWorld(DefaultConfig())
	taller.genesis()
	if err := taller.c.Append(*taller.forkBlock(g.Hash(), entry(1), 3)); err != nil {
		t.Fatal(err)
	}
	other := newWorld(DefaultConfig())
	og := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(9)}}
	Sign(og, other.prop)
	if err := other.c.AppendGenesis(*og); err != nil {
		t.Fatal(err)
	}
	for _, comp := range []struct {
		name string
		c    *Chain
	}{{"taller", taller.c}, {"equal-height-other-genesis", other.c}} {
		if a, b := heavier(comp.c, wc.c), heavier(comp.c, ws.c); a != b {
			t.Errorf("%s: heavier(competitor, clean)=%v but heavier(competitor, stubbed)=%v — a stub att changed fork-choice", comp.name, a, b)
		}
		if a, b := heavier(wc.c, comp.c), heavier(ws.c, comp.c); a != b {
			t.Errorf("%s: heavier(clean, competitor)=%v but heavier(stubbed, competitor)=%v — a stub att changed fork-choice", comp.name, a, b)
		}
	}
}
