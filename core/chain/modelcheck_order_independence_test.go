package chain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	mrand "math/rand"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// RED home #1 (part 3) — per-field ORDER-independence, mandated by the #597
// certification.
//
// The certification refined its own round-9 mandate after RED home #1 found
// `revLog`:
//
//	Replace "the tree is history-independent" with a per-field property: every
//	committed field must be RECONSTRUCTIBLE from the snapshot … Strengthen RED
//	home #1 to VARY APPEND ORDER, not just classify field presence …
//	classification ≠ order-checking. Order-dependence only surfaces when the
//	oracle varies order; presence-classification alone would have missed a
//	purely order-derived leaf.
//
// That is the gap this file closes. Parts 1 and 2 prove the enumeration cannot
// drift and that each enumerated field is load-bearing — but both are blind to
// a field that is *present, populated, and load-bearing* while being derived
// from the ORDER of history rather than from a set. Such a field breaks the
// SMT's history-independence premise, which is the single argument the
// certification's Q1 used to choose the SMT at all.
//
// The test is symmetric, and both directions are findings:
//
//   - a `committedSet` field that VARIES with order is misclassified — it
//     cannot live in the history-independent SMT, and a snapshot-booted node
//     would diverge from a replay-booted one. This is the #597 class.
//   - a `committedLog` field that does NOT vary with order is also
//     misclassified — it is really set-valued, so it belongs in the SMT and
//     does not need its own append-only root.

// orderIssuers holds the token-issuer RSA keys and the mint closure, created
// ONCE and shared across both orderings. Sharing matters: an RSA blind signature
// is randomized, so minting the same serial twice yields different Sig bytes.
// byRoot stores the full Entry (Token included), so re-minting per ordering would
// make byRoot[root] DIFFER between the two chains for a purely fixture reason —
// a false order-dependence. Minting each serial ONCE and committing the identical
// token in both orderings keeps byRoot genuinely order-independent.
type orderIssuers struct {
	keys   []ed25519.PrivateKey
	issuer func(ports.NodeID) *rsa.PublicKey
	mint   func(serial []byte) *ports.PublishToken
}

func newOrderIssuers(t *testing.T) *orderIssuers {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	priv := map[ports.NodeID]*rsa.PrivateKey{}
	for i := range keys {
		keys[i] = key(int64(11000 + i))
		rk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("issuer key %d: %v", i, err)
		}
		priv[idOf(keys[i])] = rk
	}
	rng := mrand.New(mrand.NewSource(1))
	oi := &orderIssuers{keys: keys}
	oi.issuer = func(n ports.NodeID) *rsa.PublicKey {
		if k, ok := priv[n]; ok {
			return &k.PublicKey
		}
		return nil
	}
	oi.mint = func(serial []byte) *ports.PublishToken {
		tok := &ports.PublishToken{Serial: serial}
		for _, v := range keys[:2] { // 2-of-4 quorum; both anchors qualify as issuers
			iss := blindtoken.NewIssuer(priv[idOf(v)])
			blinded, secret, err := blindtoken.Blind(rng, iss.Public(), serial)
			if err != nil {
				t.Fatalf("blind: %v", err)
			}
			blindSig, _ := iss.Issue(func() error { return nil }, blinded)
			tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: idOf(v),
				Sig: blindtoken.Unblind(iss.Public(), blindSig, secret)})
		}
		return tok
	}
	return oi
}

// orderWorld is roundsWorld with publish tokens required, so a committed
// token-entry drives `spent` non-empty. The 4 anchor keys are the token issuers:
// in this launch/objective world they qualify as issuers via launchAnchor, so
// publishtoken.Verify accepts a 2-of-4 blind-signed serial. The issuers are
// injected (shared across both orderings) so a serial's token bytes are identical
// in both chains.
//
// squatRoot, when non-zero, is a bond root a genesis squatter (squatKey) DECLARES
// (unproven) at genesis — the G3 precondition. A later PROVEN registration on the
// same root displaces it (chain.go:2780-2794). Threading it through orderWorld
// keeps the genesis identical across both orderings, so the squat is not itself an
// order variable — only the height-1 registration slice order is.
func orderWorld(t *testing.T, oi *orderIssuers, squatRoot ports.Hash, squatKey ed25519.PrivateKey) (*Chain, *Block) {
	t.Helper()
	anchors := map[ports.NodeID]bool{}
	for _, k := range oi.keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	c.RequireTokens(2, oi.issuer)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	if squatRoot != (ports.Hash{}) {
		// The squatter DECLARES squatRoot at genesis with no proof: bondRootOwner
		// set, bondRootProven left false — exactly the state a proven claim displaces.
		g.BondRegs = append(g.BondRegs, bondRegAt(squatKey, squatRoot, twoMiB, ports.Hash{}))
	}
	Sign(g, oi.keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, g
}

// tokenEntry returns entry(b) carrying the given quorum publish token —
// committing it drives `spent[token.Serial] = true`.
func tokenEntry(b byte, tok *ports.PublishToken) ports.Entry {
	e := entry(b)
	e.Token = tok
	return e
}

// slashProof builds a self-verifying equivocation proof against culprit: the
// culprit signs two DIFFERENT blocks at the same height (the era-1 shape
// VerifyEquivocation proves). Committing the proof drives `slashed[culprit] =
// true`. The culprit is NOT an anchor, so slashing it never disturbs the anchor
// quorum that commits the carrying block.
func slashProof(culprit ed25519.PrivateKey, prev ports.Hash, tagA, tagB byte) Equivocation {
	xa := &Block{Version: 1, Height: 9, Prev: prev, Entries: []ports.Entry{entry(tagA)}}
	Sign(xa, culprit)
	xb := &Block{Version: 1, Height: 9, Prev: prev, Entries: []ports.Entry{entry(tagB)}}
	Sign(xb, culprit)
	return Equivocation{Culprit: append([]byte(nil), culprit.Public().(ed25519.PublicKey)...), A: *xa, B: *xb}
}

// bondRegFull mints a signed, verifier-accepted registration carrying a non-zero
// Version and Domain, so a committed reg drives regVersion and bondDomain non-empty
// too (both signed — see signingBytes chain.go:465-480). Otherwise identical to
// bondRegAt.
func bondRegFull(s ed25519.PrivateKey, root ports.Hash, size int64, prev ports.Hash, version uint8, domain uint64) BondReg {
	r := BondReg{Validator: pubOf(s), Root: root, Size: size, Answer: []byte("valid"),
		Version: version, Domain: domain}
	r.Sig = ed25519.Sign(s, r.signingBytes(BondRegNonce(prev)))
	return r
}

// twoOrderings commits the same set of events in two different orders and
// returns both chains. Final set-valued state is identical by construction:
// every ordering publishes the same two roots (and both end revoked), spends
// the same two token serials, slashes the same two culprits, and commits the
// same three bond registrations (including a G3 displacement of a genesis
// squatter). Only the ORDER of the events differs.
//
// The certification (#597) mandated VARYING order, not just classifying
// presence. This fixture exercises the grow-only set families under order
// variation:
//
//   - byRoot/revoked (publish + revoke), spent (two token spends), and slashed
//     (two equivocation slashes) — swapped across HEIGHTS (these are keyed by
//     root/serial/culprit, so height is not part of their value).
//   - the bond-registration family — bonded, bondRootOwner, bondRootProven,
//     bondRegHeight, regVersion, bondDomain — exercised by flipping the SLICE
//     ORDER of BondRegs WITHIN a single height-5 block. bondRegHeight stores
//     b.Height (chain.go:2796), so the regs must land at the SAME height in both
//     orderings; only their intra-block processing order varies. That intra-block
//     order is precisely where the G3 proof-beats-declaration displacement rule
//     (chain.go:2780-2794) could be order-sensitive.
//
// SCOPE CORRECTION (2026-08-28, cert sameid-twoversion-intrablock-bondreg-contention,
// "What I corrected"): the two height-5 regs here are DISTINCT ids on DISJOINT roots
// (honestH on rootShared, validatorX on rootX). That exercises the G3 displacement
// under order variation, but it covers NOTHING for the SAME-ID two-version case — two
// regs for ONE id in one block, the seam where apply()'s last-writer-wins over
// regVersion/bondDomain was order-dependent. This fixture's regVersion/bondDomain
// green was therefore an over-claim for that seam (it proves distinct-id disjoint-root
// order-independence, not same-id). The same-id coverage is gateSwingOrderings (the
// swing trip-wire) plus TestRegVersionIntraBlockOrderIndependent (the covering probe);
// both are RED without the canonicalBondRegs fold in apply().
func twoOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()

	oi := newOrderIssuers(t)

	// Two non-anchor culprits whose slashes drive `slashed` non-empty. Distinct
	// from the 11000-range anchor keys so slashing them leaves the quorum intact.
	culpritA, culpritB := key(41), key(42)
	g0 := (&Block{Version: 1, Height: 0}).Hash() // stable prev for the proofs

	// The bond-registration cast. squatKey DECLARES rootShared at genesis (unproven);
	// honestH later PROVES rootShared, displacing the squat (G3). validatorX proves
	// its own rootX. Both proven regs land in ONE height-5 block whose slice order is
	// the variable. All three are non-anchor keys distinct from the 11000/41/42 ranges.
	squatKey, honestH, validatorX := key(51), key(52), key(53)
	rootShared := ports.HashBytes([]byte("g3-shared-plot-root"))
	rootX := ports.HashBytes([]byte("independent-plot-root-x"))

	// Mint each serial's token ONCE, up front, so the identical token bytes are
	// committed in both orderings — byRoot[root] is then order-independent for a
	// real reason, not made to differ by re-randomized blind signatures.
	tok31 := oi.mint([]byte("order-serial-31"))
	tok32 := oi.mint([]byte("order-serial-32"))

	// A spend+slash pair: a token-entry (drives spent + byRoot) plus an
	// equivocation slash (drives slashed). Committed at one height.
	type pair struct {
		entry ports.Entry
		slash Equivocation
	}
	p0 := pair{tokenEntry(31, tok31), slashProof(culpritA, g0, 101, 102)}
	p1 := pair{tokenEntry(32, tok32), slashProof(culpritB, g0, 103, 104)}

	// build commits the two pairs in the dictated order (heights 1 and 2), then
	// revokes both published roots (heights 3 and 4), then commits the bond block
	// at height 5 with BondRegs in bondOrder. Swap the pair order AND the bond slice
	// order and the final sets are identical — the property under test.
	build := func(first, second pair, hClaimFirst bool) *Chain {
		c, g := orderWorld(t, oi, rootShared, squatKey)
		keys := oi.keys

		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
			Entries: []ports.Entry{first.entry}, Slashes: []Equivocation{first.slash}}
		commitRounds(b1, keys, 0)
		if err := c.Append(*b1); err != nil {
			t.Fatalf("height 1 (spend+slash, first): %v", err)
		}

		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Entries: []ports.Entry{second.entry}, Slashes: []Equivocation{second.slash}}
		commitRounds(b2, keys, 0)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("height 2 (spend+slash, second): %v", err)
		}

		b3 := &Block{Version: BlockVersionRounds, Height: 3, Prev: b2.Hash(),
			Revocations: []ports.Hash{first.entry.Root}}
		commitRounds(b3, keys, 0)
		if err := c.Append(*b3); err != nil {
			t.Fatalf("revoke first root: %v", err)
		}
		b4 := &Block{Version: BlockVersionRounds, Height: 4, Prev: b3.Hash(),
			Revocations: []ports.Hash{second.entry.Root}}
		commitRounds(b4, keys, 0)
		if err := c.Append(*b4); err != nil {
			t.Fatalf("revoke second root: %v", err)
		}

		// Height 5: the two PROVEN bond registrations, whose intra-block slice order
		// is the variable. honestH proves rootShared (displacing the genesis squat —
		// G3); validatorX proves its own rootX. Both signed over the parent nonce, both
		// carry non-zero Version/Domain so regVersion/bondDomain populate. The regs land
		// at height 5 in BOTH orderings, so bondRegHeight is order-free by construction.
		nonce := b4.Hash()
		hClaim := bondRegFull(honestH, rootShared, twoMiB, nonce, BlockVersionRegGate, 0xA1)
		xReg := bondRegFull(validatorX, rootX, twoMiB, nonce, BlockVersionRegGate, 0xB2)
		regs := []BondReg{hClaim, xReg}
		if !hClaimFirst {
			regs = []BondReg{xReg, hClaim}
		}
		b5 := &Block{Version: BlockVersionRounds, Height: 5, Prev: b4.Hash(), BondRegs: regs}
		commitRounds(b5, keys, 0)
		if err := c.Append(*b5); err != nil {
			t.Fatalf("height 5 (bond regs, hClaimFirst=%v): %v", hClaimFirst, err)
		}
		return c
	}

	// Opposite orderings: swap the spend/slash pair order AND the bond slice order.
	return build(p0, p1, true), build(p1, p0, false)
}

// matureOrderings brings a network to MATURITY over two opposite-order histories
// and returns both chains, so the mature-epoch family — everMature, matureEpoch,
// epochSet — is exercised under order variation rather than declared vacuous.
//
// The regime twoOrderings cannot enter (it is a launch-anchor world with
// MatureValidators=99): an ANCHORLESS objective world with epochs on and a small
// MatureValidators, so the maturity latch trips on a real bonded set and the first
// post-latch rotation freezes epochSet (#357 Conditions A+B).
//
// The order variable is a SLASH height, per the #618 lesson that a commutative
// single-actor fixture is a decoration. A victim validator bonds at genesis
// alongside the four-key governing quorum, then is slashed at height 1 in one
// ordering and height 3 in the OTHER. So the (bonded, slashed) maps are built by
// two genuinely different histories — bonded=5 vs 4 at height 1 — that must both
// freeze the SAME epochSet at the height-4 boundary (liveQualifiedSet excludes the
// slashed victim in both). The victim is NOT part of the committing quorum, so
// slashing it early never denies the block that carries its own slash.
//
// everMature latches at height 1 in both orderings (the four bonded governors give
// MatureCoefficient=2 ≥ MatureValidators=2 once they are seen); matureEpoch and the
// frozen epochSet are set at the height-4 rotation. The property under test: two
// opposite slash orders reach byte-identical everMature, matureEpoch, epochSet.
//
// SCOPE (per RULING-620): epochSet is order-INVARIANT BY CONSTRUCTION — rotateEpoch runs
// LAST in apply on the final post-block state, so the freeze reads only the converged
// bonded/slashed maps (their order-independence is #617/#618's job). This fixture CONFIRMS
// that invariance; it does not discover-or-refute a fork as #618 did. Un-stressed residual:
// the latch/handoff HEIGHT is NOT varied — all validators bond at genesis, so the latch
// trips at the SAME height in both orderings. Acceptable (one-way final-state bools cannot
// flip), but named so the residual is on the record before the era-3 freeze.
func matureOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()
	build := func(slashEarly bool) *Chain {
		// Four governing keys (the committing quorum) plus one bonded, non-quorum
		// victim. All bond at genesis so the quorum is stable across heights and the
		// epochSet freeze reads a complete bonded ledger — only the slash HEIGHT varies.
		gov := make([]ed25519.PrivateKey, 4)
		for i := range gov {
			gov[i] = key(int64(62000 + i))
		}
		victim := key(62099)
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			EpochBlocks: 4, MatureValidators: 2}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		for _, k := range append(append([]ed25519.PrivateKey{}, gov...), victim) {
			g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
		}
		Sign(g, gov[0])
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("matureOrderings genesis: %v", err)
		}

		slashHeight := uint64(3)
		if slashEarly {
			slashHeight = 1
		}
		g0 := g.Hash()
		prev := g0
		for h := uint64(1); h <= 3; h++ {
			b := &Block{Version: BlockVersionRounds, Height: h, Prev: prev,
				Entries: []ports.Entry{entry(byte(h))}}
			if h == slashHeight {
				b.Slashes = []Equivocation{slashProof(victim, g0, 201, 202)}
			}
			commitRounds(b, gov, 0)
			if err := c.Append(*b); err != nil {
				t.Fatalf("matureOrderings height %d (slashEarly=%v): %v", h, slashEarly, err)
			}
			prev = b.Hash()
		}
		// Height 4: the epoch boundary — rotateEpoch sets matureEpoch and freezes epochSet.
		b := &Block{Version: BlockVersionRounds, Height: 4, Prev: prev,
			Entries: []ports.Entry{entry(44)}}
		commitRounds(b, gov, 0)
		if err := c.Append(*b); err != nil {
			t.Fatalf("matureOrderings boundary (slashEarly=%v): %v", slashEarly, err)
		}
		return c
	}
	return build(true), build(false) // slash-early vs slash-late: opposite slash orders
}

// matureFields are the committedSet fields matureOrderings covers — the mature-epoch
// family. They are EMPTY in the launch-anchor twoOrderings world, so their
// order-independence is proven on this mature world instead. Kept beside the fixture
// so the guard in TestCommittedSetFieldsAreOrderIndependent knows which fields to
// verify on the mature world rather than declare vacuous.
var matureFields = []string{"everMature", "matureEpoch", "epochSet"}

// gateSwingOrderings drives the #506 lock-in tally (rotateEpoch:2922) where a
// SAME-ID TWO-VERSION validator is the exact >⅔ swing, and returns both chains, so
// the #506-gate family — gateLockedIn, gateHeight — AND the same-id regVersion/
// bondDomain seam are exercised under intra-block order variation rather than
// declared vacuous. This is the fixture the cert
// sameid-twoversion-intrablock-bondreg-contention (2026-08-28, residual R2) gates on.
//
// The regime twoOrderings/matureOrderings cannot reach: an anchorless objective world
// with epochs on (EpochBlocks=2, MatureValidators=2) whose FROZEN mature set is
// {r1, r2, x} at equal weight w. Two ready validators r1, r2 carry regVersion=3 from
// genesis; the swing validator x submits TWO regs for its OWN id in the height-1
// block — version 2 (size w) and version 3 (size 2w) — whose intra-block SLICE ORDER
// is the variable.
//
// The tally at the height-2 boundary: total = 3w over {r1, r2, x}. ready counts
// members with regVersion ≥ BlockVersionRegGate.
//   - If x commits version 3: ready = 3w, 3·3w=9w > 2·3w=6w → gateLockedIn, gateHeight set.
//   - If x commits version 2: ready = 2w, 3·2w=6w > 6w is FALSE → NOT locked in.
//
// So x is the EXACT swing: before the apply()-canonicalization fix, the slice order
// decided x's committed version, hence whether the gate locked — an order-dependent
// gateLockedIn/gateHeight (a committedSet fork). After the fix, canonicalBondRegs
// picks the largest-Size reg (x's version-3 2w reg) in BOTH orders, so the gate locks
// identically. The boundary block commits under LAUNCH rules (the freeze happens IN
// rotateEpoch, after the block validated), so the count-floor quorum carries it — the
// ⅔-weight rule does not bite until the next epoch.
func gateSwingOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()
	const w = int64(2) << 20 // per-validator weight (bond size), ≥ MinBond

	build := func(v3First bool) *Chain {
		r1, r2, x := key(70001), key(70002), key(70003)
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 2}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		// Genesis bonds all three at weight w. r1, r2 register READY (version 3) so
		// they are the stable ⅔-of-ready majority-minus-the-swing. x bonds at genesis
		// too (version 3 baseline) so it is in the frozen qualified set regardless of
		// the height-1 slice order; its height-1 regs then CONTEND its committed version.
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = []BondReg{
			bondRegFull(r1, ports.HashBytes(pubOf(r1)), w, ports.Hash{}, BlockVersionRegGate, 0),
			bondRegFull(r2, ports.HashBytes(pubOf(r2)), w, ports.Hash{}, BlockVersionRegGate, 0),
			bondReg(x, w, ports.Hash{}),
		}
		Sign(g, r1)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("gateSwingOrderings genesis: %v", err)
		}

		// Height 1 (pre-latch/pre-freeze): x submits TWO regs for its OWN id on its own
		// root. Both SIZE w (so x's tally WEIGHT is w in every ordering, pre- and
		// post-fix — the swing is over x's committed VERSION, not its weight), distinct
		// in Version AND Domain:
		//   loV:  size w, version 2,                    domain 0x33
		//   hiV:  size w, version BlockVersionRegGate(3), domain 0x44
		// The total order's primary key (Size) TIES, so the canonical winner is decided
		// by the next key, Version: hiV (version 3) wins in BOTH orders post-fix. Pre-fix,
		// LAST-in-slice wins, so v3First flips x's committed version — the swing.
		rootX := ports.HashBytes(pubOf(x))
		prev := g.Hash()
		loV := bondRegFull(x, rootX, w, prev, 2, 0x33)
		hiV := bondRegFull(x, rootX, w, prev, BlockVersionRegGate, 0x44)
		regs := []BondReg{loV, hiV} // v3 last
		if v3First {
			regs = []BondReg{hiV, loV} // v3 first
		}
		// The proposer ROTATES across the two blocks (r1 proposes h1, r2 proposes h2)
		// so validatorsSeen accumulates all three non-proposer attesters {r1, r2, x}
		// over the chain — the maturity coefficient needs ≥ 2 distinct SEEN bonds, and
		// the per-block proposer is excluded from validatorsSeen (apply()). Without the
		// rotation only two are ever seen and the network never matures, so the tally
		// (post-latch only) never runs. The tally SET is still exactly the three bonded
		// validators (liveQualifiedSet reads bonded, not validatorsSeen).
		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		commitRounds(b1, []ed25519.PrivateKey{r1, r2, x}, 0) // r1 proposes
		if err := c.Append(*b1); err != nil {
			t.Fatalf("gateSwingOrderings height 1 (v3First=%v): %v", v3First, err)
		}

		// Height 2: the epoch boundary. r2 proposes so r1 joins validatorsSeen; the
		// maturity latch trips in this apply (three distinct seen bonds ≥
		// MatureValidators=2), then rotateEpoch freezes {r1, r2, x} and runs the #506
		// tally over their committed regVersion in the SAME commit (the boundary block
		// that also trips maturity hands off in one commit, chain.go).
		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Entries: []ports.Entry{entry(2)}}
		commitRounds(b2, []ed25519.PrivateKey{r2, r1, x}, 0) // r2 proposes (rotated)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("gateSwingOrderings boundary (v3First=%v): %v", v3First, err)
		}
		return c
	}
	return build(true), build(false) // v3-first vs v3-last: opposite intra-block orders
}

// gateFields are the committedSet fields gateSwingOrderings covers — the #506-gate
// lock-in family. They are EMPTY in both twoOrderings (no post-latch tally) and
// matureOrderings (no regVersion signalling), so their order-independence is proven
// on this swing world instead. Kept beside the fixture so the oracle guard knows to
// verify them on the swing world rather than declare vacuous.
var gateFields = []string{"gateLockedIn", "gateHeight"}

// era3SwingOrderings is gateSwingOrderings at the era-3 (v4) readiness level (build step
// 2c). The era-3 lock-in tally is the #506 tally reused with the bar at
// BlockVersionStateRoot (≥4), so its order-independence is proven the SAME way: a same-id
// TWO-VERSION swing validator whose committed version is the exact >⅔ ready-weight margin.
// A dedicated fixture (rather than lifting gateSwingOrderings' ready signal to v4) keeps
// the certified #506 fixture — which asserts a version-3 winner
// (TestGateLockInSwingIsOrderIndependent) — untouched.
//
// r1, r2 register READY at v4; x swings between v2 (loV) and v4 (hiV). The tally at the
// height-2 boundary over {r1, r2, x} at weight w each:
//   - x commits v4: ready = 3w, 3·3w > 2·3w → era3LockedIn set, era3Height = 2 + 2 = 4.
//   - x commits v2: ready = 2w, 3·2w > 6w is FALSE → NOT locked in.
//
// The same-id fold (canonicalBondRegs) must pick the SAME winner (largest-Size, then
// Version → v4) in both slice orders, so era3LockedIn/era3Height are order-independent.
func era3SwingOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()
	const w = int64(2) << 20

	build := func(v4First bool) *Chain {
		r1, r2, x := key(70101), key(70102), key(70103)
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 2}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = []BondReg{
			bondRegFull(r1, ports.HashBytes(pubOf(r1)), w, ports.Hash{}, BlockVersionStateRoot, 0),
			bondRegFull(r2, ports.HashBytes(pubOf(r2)), w, ports.Hash{}, BlockVersionStateRoot, 0),
			bondReg(x, w, ports.Hash{}),
		}
		Sign(g, r1)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("era3SwingOrderings genesis: %v", err)
		}

		// x submits TWO regs for its OWN id, same Size w (weight fixed), distinct Version
		// and Domain: loV (v2, 0x33) and hiV (v4, 0x44). Size ties → Version breaks the
		// tie → hiV (v4) is the canonical winner in BOTH orders post-fix.
		rootX := ports.HashBytes(pubOf(x))
		prev := g.Hash()
		loV := bondRegFull(x, rootX, w, prev, 2, 0x33)
		hiV := bondRegFull(x, rootX, w, prev, BlockVersionStateRoot, 0x44)
		regs := []BondReg{loV, hiV} // v4 last
		if v4First {
			regs = []BondReg{hiV, loV} // v4 first
		}
		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		commitRounds(b1, []ed25519.PrivateKey{r1, r2, x}, 0) // r1 proposes
		if err := c.Append(*b1); err != nil {
			t.Fatalf("era3SwingOrderings height 1 (v4First=%v): %v", v4First, err)
		}
		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Entries: []ports.Entry{entry(2)}}
		commitRounds(b2, []ed25519.PrivateKey{r2, r1, x}, 0) // r2 proposes (rotated)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("era3SwingOrderings boundary (v4First=%v): %v", v4First, err)
		}
		return c
	}
	return build(true), build(false) // v4-first vs v4-last: opposite intra-block orders
}

// era3Fields are the committedSet fields era3SwingOrderings covers — the era-3 (v4)
// activation family. EMPTY in twoOrderings/matureOrderings for the same reasons as
// gateFields, so their order-independence is proven on the era-3 swing world.
var era3Fields = []string{"era3LockedIn", "era3Height"}

// era4SwingOrderings is era3SwingOrderings at the era-4 (v5) readiness level (build step
// 4d), the exact mirror one era up. The era-4 lock-in tally is the same tally with the bar
// at BlockVersionWitnessable (>= 5), so its order-independence is proven the SAME way: a
// same-id TWO-VERSION swing validator whose committed version is the exact >⅔ ready-weight
// margin. A dedicated fixture keeps the era-3 fixture untouched.
//
// r1, r2 register READY at v5; x swings between v2 (loV) and v5 (hiV). The tally at the
// height-2 boundary over {r1, r2, x} at weight w each:
//   - x commits v5: ready = 3w, 3·3w > 2·3w → era4LockedIn set, era4Height = 2 + 2 = 4
//     (and era3LockedIn set too — a v5 signaller is v4-ready).
//   - x commits v2: era-4 ready = 2w, 3·2w > 6w is FALSE → era-4 NOT locked in.
//
// The same-id fold (canonicalBondRegs) must pick the SAME winner (largest-Size, then
// Version → v5) in both slice orders, so era4LockedIn/era4Height are order-independent.
func era4SwingOrderings(t *testing.T) (*Chain, *Chain) {
	t.Helper()
	const w = int64(2) << 20

	build := func(v5First bool) *Chain {
		r1, r2, x := key(70201), key(70202), key(70203)
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 2}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = []BondReg{
			bondRegFull(r1, ports.HashBytes(pubOf(r1)), w, ports.Hash{}, BlockVersionWitnessable, 0),
			bondRegFull(r2, ports.HashBytes(pubOf(r2)), w, ports.Hash{}, BlockVersionWitnessable, 0),
			bondReg(x, w, ports.Hash{}),
		}
		Sign(g, r1)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("era4SwingOrderings genesis: %v", err)
		}

		// x submits TWO regs for its OWN id, same Size w (weight fixed), distinct Version
		// and Domain: loV (v2, 0x33) and hiV (v5, 0x55). Size ties → Version breaks the tie
		// → hiV (v5) is the canonical winner in BOTH orders.
		rootX := ports.HashBytes(pubOf(x))
		prev := g.Hash()
		loV := bondRegFull(x, rootX, w, prev, 2, 0x33)
		hiV := bondRegFull(x, rootX, w, prev, BlockVersionWitnessable, 0x55)
		regs := []BondReg{loV, hiV} // v5 last
		if v5First {
			regs = []BondReg{hiV, loV} // v5 first
		}
		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		commitRounds(b1, []ed25519.PrivateKey{r1, r2, x}, 0) // r1 proposes
		if err := c.Append(*b1); err != nil {
			t.Fatalf("era4SwingOrderings height 1 (v5First=%v): %v", v5First, err)
		}
		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Entries: []ports.Entry{entry(2)}}
		commitRounds(b2, []ed25519.PrivateKey{r2, r1, x}, 0) // r2 proposes (rotated)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("era4SwingOrderings boundary (v5First=%v): %v", v5First, err)
		}
		return c
	}
	return build(true), build(false) // v5-first vs v5-last: opposite intra-block orders
}

// era4Fields are the committedSet fields era4SwingOrderings covers — the era-4 (v5)
// activation family. EMPTY in twoOrderings/matureOrderings for the same reasons as
// gateFields/era3Fields, so their order-independence is proven on the era-4 swing world.
var era4Fields = []string{"era4LockedIn", "era4Height"}

// orderVacuous names committedSet fields that NEITHER twoOrderings NOR matureOrderings
// populates — they belong to a regime those worlds do not enter. Each is compared as
// DeepEqual(∅, ∅), so its order-independence is NOT proven by this oracle; the entry
// records what a covering fixture would have to construct. This is a DECLARED, SHRINKING
// debt (mirroring probeUncovered in the snapshot oracle): the guard below fails on any
// NEW empty-vs-empty committedSet field not listed here, so the vacuous-green hole can
// never silently reopen. `spent`/`slashed` (#617) and the bond-registration family
// (#618) were moved out by twoOrderings; the mature-epoch family (everMature, matureEpoch,
// epochSet) is now moved out by matureOrderings.
var orderVacuous = map[string]string{
	// Bond-registration state (bonded, bondRootOwner, bondRootProven, bondRegHeight,
	// regVersion, bondDomain) is COVERED by twoOrderings' height-5 bond block whose
	// BondReg slice order flips (incl. a G3 displacement). The mature-epoch family
	// (everMature, matureEpoch, epochSet) is COVERED by matureOrderings' opposite slash
	// orders. The #506-gate family (gateLockedIn, gateHeight) is COVERED by
	// gateSwingOrderings, whose same-id two-version swing validator flips the lock-in
	// tally across intra-block orders (moved out 2026-08-28, cert
	// sameid-twoversion-intrablock-bondreg-contention). All are enforced non-empty below.
	// validatorsSeen is NOT listed: the attesting anchors qualify, so apply()
	// populates it — its order-independence is genuinely exercised here.
	//
	// era-4 (v5) maintenance spine. `qualified` is NOT listed: it is
	// filter(bonded, slashed, MinBond), so the same worlds that populate bonded
	// populate it, and its order-independence is genuinely exercised across the
	// opposite orderings. The remaining two need a regime these worlds do not enter:
	"dueBucket": "era-4 T-3 due-height index — populated only with BondTTLBlocks>0 " +
		"(TTL enabled), which neither twoOrderings nor matureOrderings sets. Its " +
		"order-independence is instead proven directly by TestStateRootV5IsOrderIndependent " +
		"(the canonical-MTH bucket over a random-order id set) and by the byte-identical " +
		"post-apply replay (TestV5PostApplyRootByteIdenticalAcrossOrderings). A covering " +
		"fixture would enable TTL and (re)register the same ids in two intra-block orders.",
	"issuerKeyCommit": "R0.4b — the per-epoch demand-issuer key binding. Populated only by a " +
		"block carrying IssuerKeys, which is v5-ONLY; neither twoOrderings nor matureOrderings " +
		"mints a v5 block, so both orderings leave it empty. Its order-independence is not in " +
		"doubt for a structural reason worth stating: apply writes it FIRST-WRITE-WINS keyed on " +
		"(epoch, issuer) and never overwrites, so two orderings of the same registrations differ " +
		"only in WHICH duplicate is skipped — and a duplicate that differs in fingerprint is " +
		"skipped either way, so the surviving map is identical. That is exercised directly by " +
		"TestIssuerKeyFirstWriteWinsIsOrderIndependent (issuerkey_test.go). A covering fixture " +
		"here would have to mint a v5 block and flip its IssuerKeys slice order.",

	"epochStart": "era-4 O-1 — the boundary height scalar. It is a pure function of height " +
		"(h of the last rotation), not of event ORDER within a block, so two orderings of the " +
		"same history reach the identical epochStart trivially; matureOrderings leaves it at a " +
		"value the guard reads as zero. Its commitment is exercised by the v5 marshaller tests " +
		"(TestStateRootV5EmitsALeafForEveryV5Field, TestStateRootV5IsOrderIndependent).",
}

// TestCommittedSetFieldsAreOrderIndependent is the load-bearing half. Every
// field classified `committedSet` goes under the history-independent SMT, so
// two histories reaching the same final set MUST agree on it exactly.
func TestCommittedSetFieldsAreOrderIndependent(t *testing.T) {
	a, b := twoOrderings(t)
	ma, mb := matureOrderings(t)
	ga, gb := gateSwingOrderings(t)
	e3a, e3b := era3SwingOrderings(t)
	e4a, e4b := era4SwingOrderings(t)

	fields := fieldsOfKind(committedSet)
	if len(fields) == 0 {
		t.Fatal("no committedSet fields — the classification or reflection is broken")
	}

	// A field is covered on whichever world POPULATES it: the launch twoOrderings
	// world for the launch/objective families, the mature matureOrderings world for
	// the mature-epoch family, the gateSwingOrderings world for the #506-gate family,
	// the era3SwingOrderings world for the era-3 (v4) activation family.
	// Pairing a field with its populating world is the same union-of-worlds pattern the
	// snapshot oracle uses (worldGroup). matureFields/gateFields/era3Fields declare which
	// fields switch to which world; every other field uses the launch world.
	mature := map[string]bool{}
	for _, f := range matureFields {
		mature[f] = true
	}
	gate := map[string]bool{}
	for _, f := range gateFields {
		gate[f] = true
	}
	era3 := map[string]bool{}
	for _, f := range era3Fields {
		era3[f] = true
	}
	era4 := map[string]bool{}
	for _, f := range era4Fields {
		era4[f] = true
	}
	worldOf := func(name string) (*Chain, *Chain) {
		if mature[name] {
			return ma, mb
		}
		if gate[name] {
			return ga, gb
		}
		if era3[name] {
			return e3a, e3b
		}
		if era4[name] {
			return e4a, e4b
		}
		return a, b
	}

	// The durable fix (PE ruling "Coupling", 2026-08-28): a field that is EMPTY
	// in both orderings is compared as DeepEqual(∅, ∅) — a vacuous green that
	// asserts nothing. Every committedSet field the oracle claims to prove
	// order-independent must be NON-EMPTY in at least one ordering of its populating
	// world, OR be explicitly declared in orderVacuous with the reason no fixture
	// populates it. Otherwise "all N identical" reads as coverage while some
	// fraction of it is empty-vs-empty — exactly the shape that let `spent` and
	// `slashed` show green over an unexercised map.
	var undeclaredVacuous []string
	populated := map[string]bool{}
	for _, name := range fields {
		wa, wb := worldOf(name)
		if isZero(fieldValue(wa, name)) && isZero(fieldValue(wb, name)) {
			if _, declared := orderVacuous[name]; !declared {
				undeclaredVacuous = append(undeclaredVacuous, name)
			}
			continue
		}
		populated[name] = true
		// A field cannot be both populated and declared un-populatable.
		if _, declared := orderVacuous[name]; declared {
			t.Errorf("%q is populated by its fixture yet still listed in orderVacuous "+
				"— remove the stale entry; its order-independence is now genuinely exercised.", name)
		}
	}
	if len(undeclaredVacuous) > 0 {
		t.Fatalf("%d committedSet field(s) are EMPTY in both orderings and NOT declared "+
			"in orderVacuous, so the order-independence comparison over them is "+
			"DeepEqual(∅, ∅) — vacuous, asserting nothing: %v\n\n"+
			"A field under the history-independent SMT must have its order-independence "+
			"EXERCISED, not merely restated over an empty map. Extend a fixture to "+
			"populate the field with a real event order (see the spent/slashed spends "+
			"and slashes, or matureOrderings' opposite slash orders), or record in "+
			"orderVacuous what a fixture would have to construct. Do not let the count "+
			"of 'identical' fields include ones no history touched.", len(undeclaredVacuous), undeclaredVacuous)
	}
	// Proof the coverage is real: the fields this work exists to cover must be
	// non-empty in their fixture, not merely absent from orderVacuous. spent/slashed
	// come from the spend+slash pairs; the bond-registration family from the height-5
	// bond block with its slice-order variation (incl. the G3 displacement); the
	// mature-epoch family from matureOrderings' opposite slash orders.
	mustCover := append([]string{"spent", "slashed",
		"bonded", "bondRootOwner", "bondRootProven", "bondRegHeight", "regVersion", "bondDomain"},
		matureFields...)
	mustCover = append(mustCover, gateFields...)
	for _, name := range mustCover {
		if !populated[name] {
			wa, wb := worldOf(name)
			t.Fatalf("%q must be NON-EMPTY in its fixture — the whole point is to exercise "+
				"its order-independence over a real event order, not DeepEqual(∅, ∅). "+
				"a-empty=%v b-empty=%v", name, isZero(fieldValue(wa, name)), isZero(fieldValue(wb, name)))
		}
	}

	var orderDependent []string
	for _, name := range fields {
		wa, wb := worldOf(name)
		if !reflect.DeepEqual(fieldValue(wa, name), fieldValue(wb, name)) {
			orderDependent = append(orderDependent, name)
		}
	}
	if len(orderDependent) > 0 {
		t.Fatalf("%d field(s) classified `committedSet` DIFFER between two histories "+
			"that reach the same final state: %v\n\n"+
			"These cannot live in the history-independent SMT. The certification's "+
			"Q1 chose the SMT on exactly one argument — that the root is identical "+
			"however the state was reached, because a snapshot-booted node never "+
			"replayed the history. An order-derived value under that root breaks the "+
			"argument, and a snapshot-booted validator diverges from a replay-booted "+
			"one. Either the field is really an ordered log (reclassify as "+
			"`committedLog`, give it its own append-only root — the #597 resolution), "+
			"or the state it accumulates must be made order-free.",
			len(orderDependent), orderDependent)
	}
	t.Logf("all %d committedSet fields identical across opposite event orderings "+
		"(mature-epoch family on matureOrderings, the rest on twoOrderings)", len(fields))
}

// TestStateRootIsOrderIndependentAcrossHistories lifts the per-field
// order-independence assertion to the ROOT — closing the gap between "the 18 fields
// are equal" and "the computed StateRoot is equal." Two histories that reach the same
// final committedSet must produce byte-identical StateRoots, because the root is a
// pure function of that set. This is the era-3 root-equality half the format freeze
// rests on: a value-encoding defect that made two orderings produce the same fields
// but a different root would be caught HERE, not in the field.
//
// The two-root separation is asserted in the same breath: the twoOrderings pair
// differs in revLog (order-dependent — TestRevLogRootIsOrderDependent), so its
// LogRoots DIFFER while its StateRoots must MATCH. Equal state root, different log
// root: two kinds of committed data, two roots (#597), proven on the computed roots.
func TestStateRootIsOrderIndependentAcrossHistories(t *testing.T) {
	stateRoot := func(c *Chain) ports.Hash {
		r, err := c.StateRoot()
		if err != nil {
			t.Fatalf("StateRoot: %v", err)
		}
		return r
	}

	// Each fixture pair reaches the same final committedSet by opposite orderings.
	// The StateRoot must be identical for every pair.
	a, b := twoOrderings(t)
	if ra, rb := stateRoot(a), stateRoot(b); ra != rb {
		t.Fatalf("twoOrderings: StateRoot DIFFERS across opposite orderings (%x != %x) — "+
			"the 18 fields are equal (TestCommittedSetFieldsAreOrderIndependent) but the "+
			"computed root is not, so a value-encoding defect made the root order-dependent. "+
			"This is a consensus finding to route, not a test to relax.", ra, rb)
	}
	// The two-root separation, on the computed roots: same StateRoot, DIFFERENT LogRoot.
	if la, lb := a.LogRoot(), b.LogRoot(); la == lb {
		t.Fatalf("twoOrderings: LogRoots are EQUAL across opposite orderings (%x) — the "+
			"premise that revLog is order-dependent is broken, so the two-root split is "+
			"untested here", la)
	}

	ma, mb := matureOrderings(t)
	if ra, rb := stateRoot(ma), stateRoot(mb); ra != rb {
		t.Fatalf("matureOrderings: StateRoot DIFFERS across opposite slash orderings "+
			"(%x != %x) — the mature-epoch family (everMature/matureEpoch/epochSet) made "+
			"the root order-dependent. Consensus finding to route.", ra, rb)
	}

	ga, gb := gateSwingOrderings(t)
	if ra, rb := stateRoot(ga), stateRoot(gb); ra != rb {
		t.Fatalf("gateSwingOrderings: StateRoot DIFFERS across opposite intra-block "+
			"orderings (%x != %x) — the #506-gate family (gateLockedIn/gateHeight) or the "+
			"same-id regVersion/bondDomain seam made the root order-dependent. This is the "+
			"certified fork surfacing at the root level. Route, do not relax.", ra, rb)
	}

	t.Logf("StateRoot byte-identical across opposite orderings on all three fixture " +
		"worlds; LogRoot differs on twoOrderings (two roots, two kinds of data)")
}

// TestCommittedLogFieldsAreGenuinelyOrderDependent is the other direction, and
// it is a real assertion rather than a formality: a "log" that turns out to be
// order-INDEPENDENT does not need its own append-only root, and carrying one
// costs a separate header field and a full entry list in every snapshot for
// nothing. Per the consensus-correctness discipline, an oracle that observes
// something it cannot explain flags rather than assumes-benign.
func TestCommittedLogFieldsAreGenuinelyOrderDependent(t *testing.T) {
	a, b := twoOrderings(t)

	fields := fieldsOfKind(committedLog)
	if len(fields) == 0 {
		t.Skip("no committedLog fields classified")
	}

	for _, name := range fields {
		if reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Errorf("field %q is classified `committedLog` but is IDENTICAL across "+
				"two opposite event orderings.\nIf it does not depend on order it is "+
				"set-valued, so it belongs in the SMT — a separate append-only root "+
				"and a full entry list in every snapshot would be paid for nothing. "+
				"Reclassify it, or explain what order-dependence this history fails "+
				"to exercise.", name)
		}
	}
}

// TestBondRegG3DisplacementIsOrderIndependent is the consensus-correctness
// trip-wire for the bond-registration family under the DISJOINT-ROOT case. It is
// NOT enough that the committedSet fields happen to match across the two
// orderings — the match must be over a G3 displacement that ACTUALLY FIRED, else
// the coverage is vacuous.
//
// This asserts the end state directly: in BOTH orderings the genesis squatter is
// removed from bonded and is no longer the owner of the shared root, honestH is
// the PROVEN owner, and validatorX is bonded on its own DISJOINT root. If the two
// orderings had reached DIFFERENT bond-root states, that would be a real consensus
// finding (an order-sensitive displacement validity rule under a history-
// independent root) — STOP-and-escalate, no rule change. They do not: G3/bond-root
// ownership is order-INDEPENDENT for this disjoint-root construction.
//
// SCOPE (certified 2026-08-28, same-root-intrablock-bondreg-contention): this
// covers ONE proven claimant per root (a squat displaced by a single proof, plus
// an independent claim on a DISJOINT root). It does NOT cover two DISTINCT-ID
// proven claims on the SAME root in one block — that case IS order-dependent in
// apply() and is handled at the validity layer, which now REJECTS such a block
// (ErrSharedRootInBlock). See redteam_verify_sameroot-intrablock_test.go. So
// G3/bond-root ownership is order-independent for every ADMISSIBLE block because
// the same-root distinct-ID collision is no longer admitted.
func TestBondRegG3DisplacementIsOrderIndependent(t *testing.T) {
	squatKey, honestH, validatorX := key(51), key(52), key(53)
	rootShared := ports.HashBytes([]byte("g3-shared-plot-root"))
	rootX := ports.HashBytes([]byte("independent-plot-root-x"))
	sq, h, x := idOf(squatKey), idOf(honestH), idOf(validatorX)

	a, b := twoOrderings(t)

	for _, tc := range []struct {
		name string
		c    *Chain
	}{{"hClaimFirst", a}, {"xRegFirst", b}} {
		c := tc.c
		// The displacement fired: the squatter has no standing and is not the owner.
		if _, ok := c.bonded[sq]; ok {
			t.Fatalf("[%s] G3 did NOT fire: squatter still bonded (%d) — the coverage is "+
				"vacuous, the displacement branch was never taken", tc.name, c.bonded[sq])
		}
		if owner := c.bondRootOwner[rootShared]; owner != h {
			t.Fatalf("[%s] shared-root owner is %x, want honestH %x — G3 displacement did not "+
				"transfer ownership", tc.name, owner[:6], h[:6])
		}
		if !c.bondRootProven[rootShared] {
			t.Fatalf("[%s] shared root not marked proven after the displacing claim", tc.name)
		}
		if c.bonded[h] == 0 {
			t.Fatalf("[%s] honestH earned no standing after displacing the squat", tc.name)
		}
		if c.bonded[x] == 0 || c.bondRootOwner[rootX] != x {
			t.Fatalf("[%s] validatorX not bonded on its own root (bonded=%d owner=%x)",
				tc.name, c.bonded[x], c.bondRootOwner[rootX])
		}
	}

	// The two orderings reach byte-identical bond-registration state.
	for _, name := range []string{"bonded", "bondRootOwner", "bondRootProven",
		"bondRegHeight", "regVersion", "bondDomain"} {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("bond field %q DIFFERS across the two BondReg orderings — the G3 "+
				"displacement is ORDER-DEPENDENT. This is a consensus finding to route, "+
				"NOT a test to relax:\n  hClaimFirst: %v\n  xRegFirst:   %v",
				name, fieldValue(a, name), fieldValue(b, name))
		}
	}
	t.Logf("G3 displacement fired in both orderings and reached byte-identical bond state "+
		"— proof-beats-declaration is order-INDEPENDENT (squatter %x displaced by %x)", sq[:6], h[:6])
}

// TestMatureEpochFamilyIsOrderIndependent is the consensus-correctness trip-wire
// for the mature-epoch family (everMature, matureEpoch, epochSet). Byte-identity
// across the two orderings is necessary but not sufficient: the match must be over
// a maturity latch and an epoch freeze that ACTUALLY FIRED, and over two GENUINELY
// different histories, else the coverage is vacuous (the #618 lesson).
//
// It asserts directly: in BOTH orderings the network matured (everMature), handed
// off (matureEpoch), and froze the SAME four-key governing set into epochSet — the
// slashed victim excluded. The victim is slashed at height 1 in one ordering and
// height 3 in the other, so the (bonded, slashed) maps are built by two different
// histories that must converge. If the two orderings had frozen DIFFERENT epoch
// sets — or one matured and the other did not at the same final height — that would
// be a REAL consensus finding (an order-sensitive maturity/rotation rule under a
// history-independent root), STOP-and-escalate, NO rule change. They do not.
func TestMatureEpochFamilyIsOrderIndependent(t *testing.T) {
	victim := idOf(key(62099))
	gov := make([]ports.NodeID, 4)
	for i := range gov {
		gov[i] = idOf(key(int64(62000 + i)))
	}

	a, b := matureOrderings(t)

	for _, tc := range []struct {
		name string
		c    *Chain
	}{{"slash-early", a}, {"slash-late", b}} {
		c := tc.c
		if !c.everMature {
			t.Fatalf("[%s] everMature did NOT latch — the maturity path never fired, coverage is vacuous", tc.name)
		}
		if !c.matureEpoch {
			t.Fatalf("[%s] matureEpoch did NOT set — the #357 Cond-B handoff never fired, coverage is vacuous", tc.name)
		}
		// The freeze captured exactly the four governors; the slashed victim is excluded.
		if len(c.epochSet) != 4 {
			t.Fatalf("[%s] epochSet froze %d members, want 4 (the governors, victim excluded) "+
				"— the freeze did not capture the expected set", tc.name, len(c.epochSet))
		}
		if _, ok := c.epochSet[victim]; ok {
			t.Fatalf("[%s] slashed victim %x is in the frozen epochSet — liveQualifiedSet should exclude it", tc.name, victim[:6])
		}
		for _, id := range gov {
			if _, ok := c.epochSet[id]; !ok {
				t.Fatalf("[%s] governor %x missing from frozen epochSet", tc.name, id[:6])
			}
		}
	}

	// The two orderings reach byte-identical mature-epoch state.
	for _, name := range matureFields {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("mature-epoch field %q DIFFERS across the two slash orderings — maturity/"+
				"rotation is ORDER-DEPENDENT. This is a consensus finding to route, NOT a test "+
				"to relax:\n  slash-early: %v\n  slash-late:  %v",
				name, fieldValue(a, name), fieldValue(b, name))
		}
	}
	t.Logf("network matured and froze an identical %d-member epochSet across two opposite "+
		"slash orderings — epochSet is order-INVARIANT BY CONSTRUCTION (rotateEpoch is last "+
		"in apply, a deterministic read of bonded/slashed; #617/#618 cover those). This "+
		"CONFIRMS invariance; it does not discover-or-refute a fork as #618 did. Residual: "+
		"latch/handoff HEIGHT not varied (all bond at genesis), one-way bools that cannot flip",
		len(a.epochSet))
}

// TestGateLockInSwingIsOrderIndependent is the consensus-correctness trip-wire for
// the #506-gate family (gateLockedIn, gateHeight) AND the certified real coverage of
// the same-id regVersion/bondDomain seam. Byte-identity across the two orderings is
// necessary but not sufficient: the match must be over a lock-in tally that ACTUALLY
// FIRED with the two-version validator as the EXACT >⅔ swing, else the coverage is
// vacuous (the #618 / session-7 lesson). This is the fixture the cert
// sameid-twoversion-intrablock-bondreg-contention (2026-08-28, residual R2) gates on.
//
// It asserts directly: in BOTH intra-block orderings the gate LOCKED IN, at the SAME
// gateHeight, and the swing validator x committed the SAME regVersion (BlockVersionRegGate,
// the canonical largest-Size-then-Version winner) and bondDomain. Before the apply()
// canonicalization fix, flipping the height-1 slice order flipped x's committed version
// (v3 vs v2), and x is the marginal ⅔ ready-weight, so gateLockedIn/gateHeight forked
// across the two orderings — the exact propagation the cert traces (a committedSet field
// feeding a consensus-decision field). If the two orderings had reached DIFFERENT gate
// state, that would be the live fork; they do not, because canonicalBondRegs makes x's
// committed version a pure function of block content.
func TestGateLockInSwingIsOrderIndependent(t *testing.T) {
	x := idOf(key(70003)) // the two-version swing validator

	a, b := gateSwingOrderings(t)

	for _, tc := range []struct {
		name string
		c    *Chain
	}{{"v3-first", a}, {"v3-last", b}} {
		c := tc.c
		if !c.matureEpoch {
			t.Fatalf("[%s] matureEpoch did NOT set — the #506 tally runs only post-latch, so "+
				"coverage is vacuous; the fixture never reached the boundary rotation", tc.name)
		}
		if !c.gateLockedIn {
			t.Fatalf("[%s] gateLockedIn is FALSE — the #506 lock-in tally did not fire (the swing "+
				"validator's version did not clear the >⅔ ready-weight bar). The coverage is "+
				"vacuous: without a lock-in there is no gateHeight to compare. Re-derive the swing "+
				"weights (x must be the marginal ready vote).", tc.name)
		}
		if c.gateHeight == 0 {
			t.Fatalf("[%s] gateHeight is 0 after a lock-in — inconsistent state", tc.name)
		}
		// The swing validator committed the canonical (largest-Size, then Version) winner:
		// version 3, the ready signal. If it committed 2, the gate would NOT have locked.
		if c.regVersion[x] != BlockVersionRegGate {
			t.Fatalf("[%s] swing validator committed regVersion=%d, want %d (the canonical "+
				"version-3 winner) — the same-id fold picked the wrong reg", tc.name,
				c.regVersion[x], BlockVersionRegGate)
		}
		if c.bondDomain[x] != 0x44 {
			t.Fatalf("[%s] swing validator committed bondDomain=%#x, want 0x44 (the version-3 "+
				"winner's domain) — the fold must take ALL fields of the ONE winning reg",
				tc.name, c.bondDomain[x])
		}
	}

	// The two orderings reach byte-identical gate state AND same-id bond state.
	for _, name := range []string{"gateLockedIn", "gateHeight", "regVersion", "bondDomain"} {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("field %q DIFFERS across the two intra-block orderings — the #506 gate "+
				"inherited the same-id version split. This is the certified fork; the "+
				"canonicalization has regressed:\n  v3-first: %v\n  v3-last:  %v",
				name, fieldValue(a, name), fieldValue(b, name))
		}
	}
	t.Logf("the #506 gate locked in identically across two opposite intra-block orderings "+
		"(gateLockedIn=%v gateHeight=%d) with the same-id two-version validator as the ⅔ swing "+
		"— gateLockedIn/gateHeight no longer inherit the version split", a.gateLockedIn, a.gateHeight)
}

// TestEra3LockInSwingIsOrderIndependent is TestGateLockInSwingIsOrderIndependent at the
// era-3 (v4) readiness level (build step 2c). The same-id two-version swing validator's
// committed version decides whether era-3 locks; the two opposite intra-block orderings
// must reach byte-identical era3LockedIn/era3Height (and the same-id fold must pick the
// v4 winner in both). If the fold regressed, the two orderings would fork the era-3
// activation state — a committedSet fork under the history-independent SMT.
func TestEra3LockInSwingIsOrderIndependent(t *testing.T) {
	x := idOf(key(70103)) // the two-version swing validator

	a, b := era3SwingOrderings(t)

	for _, tc := range []struct {
		name string
		c    *Chain
	}{{"v4-first", a}, {"v4-last", b}} {
		c := tc.c
		if !c.matureEpoch {
			t.Fatalf("[%s] matureEpoch did NOT set — the era-3 tally runs only post-latch, so "+
				"coverage is vacuous; the fixture never reached the boundary rotation", tc.name)
		}
		if !c.era3LockedIn {
			t.Fatalf("[%s] era3LockedIn is FALSE — the era-3 lock-in tally did not fire (the swing "+
				"validator's version did not clear the >⅔ ready-weight bar). Re-derive the swing "+
				"weights (x must be the marginal ready vote).", tc.name)
		}
		if c.era3Height == 0 {
			t.Fatalf("[%s] era3Height is 0 after a lock-in — inconsistent state", tc.name)
		}
		// The swing validator committed the canonical (largest-Size, then Version) winner:
		// version 4, the era-3 ready signal. If it committed 2, era-3 would NOT have locked.
		if c.regVersion[x] != BlockVersionStateRoot {
			t.Fatalf("[%s] swing validator committed regVersion=%d, want %d (the canonical "+
				"version-4 winner) — the same-id fold picked the wrong reg", tc.name,
				c.regVersion[x], BlockVersionStateRoot)
		}
	}

	// The two orderings reach byte-identical era-3 activation state.
	for _, name := range []string{"era3LockedIn", "era3Height"} {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("field %q DIFFERS across the two intra-block orderings — era-3 activation "+
				"inherited the same-id version split:\n  v4-first: %v\n  v4-last:  %v",
				name, fieldValue(a, name), fieldValue(b, name))
		}
	}
	t.Logf("era-3 locked in identically across two opposite intra-block orderings "+
		"(era3LockedIn=%v era3Height=%d) with the same-id two-version validator as the ⅔ swing",
		a.era3LockedIn, a.era3Height)
}

// TestEra4LockInSwingIsOrderIndependent is TestEra3LockInSwingIsOrderIndependent at the
// era-4 (v5) readiness level (build step 4d). The same-id two-version swing validator's
// committed version decides whether era-4 locks; the two opposite intra-block orderings
// must reach byte-identical era4LockedIn/era4Height (and the same-id fold must pick the v5
// winner in both). If the fold regressed, the two orderings would fork the era-4 activation
// state — a committedSet fork under the history-independent SMT.
func TestEra4LockInSwingIsOrderIndependent(t *testing.T) {
	x := idOf(key(70203)) // the two-version swing validator

	a, b := era4SwingOrderings(t)

	for _, tc := range []struct {
		name string
		c    *Chain
	}{{"v5-first", a}, {"v5-last", b}} {
		c := tc.c
		if !c.matureEpoch {
			t.Fatalf("[%s] matureEpoch did NOT set — the era-4 tally runs only post-latch, so "+
				"coverage is vacuous; the fixture never reached the boundary rotation", tc.name)
		}
		if !c.era4LockedIn {
			t.Fatalf("[%s] era4LockedIn is FALSE — the era-4 lock-in tally did not fire (the swing "+
				"validator's version did not clear the >⅔ ready-weight bar). Re-derive the swing "+
				"weights (x must be the marginal ready vote).", tc.name)
		}
		if c.era4Height == 0 {
			t.Fatalf("[%s] era4Height is 0 after a lock-in — inconsistent state", tc.name)
		}
		// era-4 activation implies era-3 activation (a v5 signaller is v4-ready) — the
		// layering invariant, asserted on the swing world.
		if !c.era3LockedIn {
			t.Fatalf("[%s] era4LockedIn set but era3LockedIn is FALSE — era-4 must layer on era-3 "+
				"(v5 ⊇ v4); a v5 signaller is necessarily v4-ready", tc.name)
		}
		// The swing validator committed the canonical (largest-Size, then Version) winner:
		// version 5, the era-4 ready signal. If it committed 2, era-4 would NOT have locked.
		if c.regVersion[x] != BlockVersionWitnessable {
			t.Fatalf("[%s] swing validator committed regVersion=%d, want %d (the canonical "+
				"version-5 winner) — the same-id fold picked the wrong reg", tc.name,
				c.regVersion[x], BlockVersionWitnessable)
		}
	}

	// The two orderings reach byte-identical era-4 activation state.
	for _, name := range []string{"era4LockedIn", "era4Height"} {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
			t.Fatalf("field %q DIFFERS across the two intra-block orderings — era-4 activation "+
				"inherited the same-id version split:\n  v5-first: %v\n  v5-last:  %v",
				name, fieldValue(a, name), fieldValue(b, name))
		}
	}
	t.Logf("era-4 locked in identically across two opposite intra-block orderings "+
		"(era4LockedIn=%v era4Height=%d) with the same-id two-version validator as the ⅔ swing",
		a.era4LockedIn, a.era4Height)
}

// TestRevLogRootIsOrderDependent is the concrete #597 statement, asserted on
// the published API rather than on the field: the transparency-log ROOT — the
// value era-3 will commit in its own header field — differs between the two
// orderings, while the `revoked` status set does not.
//
// That single pair of facts is the whole certified resolution: the same events
// in different orders yield one identical set and two different log roots, so
// the two must live under two different roots.
func TestRevLogRootIsOrderDependent(t *testing.T) {
	a, b := twoOrderings(t)

	if !reflect.DeepEqual(a.revoked, b.revoked) {
		t.Fatalf("the revoked STATUS set differs across orderings (%v vs %v) — the "+
			"premise of this test is broken", a.revoked, b.revoked)
	}
	if a.RevocationLogSize() != b.RevocationLogSize() {
		t.Fatalf("log sizes differ (%d vs %d); the two histories should log the same "+
			"number of events", a.RevocationLogSize(), b.RevocationLogSize())
	}

	ra, rb := a.RevocationLogRoot(), b.RevocationLogRoot()
	if ra == rb {
		t.Fatal("the revocation-log roots are EQUAL across two opposite orderings.\n" +
			"If the log root is order-independent, #597's resolution is wrong and " +
			"revLog could simply be an SMT leaf. Re-derive before changing the " +
			"classification — translog.Root() is the RFC-6962 MTH over an ordered " +
			"slice (translog.go:54/:106), so this should not happen.")
	}
	t.Logf("same revoked set (%d roots), different log roots: %x… vs %x… — "+
		"two kinds of committed data, two roots", len(a.revoked), ra[:6], rb[:6])
}

// TestBondedOrderFreeUnderSlashInteraction traces the residual the PE ruling
// flagged: apply() pairs slashed[culprit]=true with delete(c.bonded, culprit)
// (chain.go:2819-2820). `slashed` is grow-only, but `bonded` is MUTATED in the
// same step, so a mid-block slash could in principle make the final bonded set
// order-sensitive. The twoOrderings fixture slashes NON-bonded culprits, so its
// delete is a no-op and does not exercise this interaction. This test does: it
// bonds two validators, then slashes both in two OPPOSITE orders, and asserts
// the final `bonded` (and `slashed`) sets are byte-identical.
//
// Mechanism: bonded is a map keyed by NodeID; delete removes a key. Deleting C1
// then C2 leaves the same map as deleting C2 then C1 — deletion of distinct keys
// commutes. This test is the execution-grade evidence for that algebra, which is
// what the keystone discipline demands over "clean by inspection."
func TestBondedOrderFreeUnderSlashInteraction(t *testing.T) {
	// An anchor world carries quorum via the four anchors. Two EXTRA validators
	// C1, C2 bond via genesis and are the slash culprits — their bonded standing
	// is what the paired delete removes.
	build := func(slashC1First bool) (*Chain, ports.NodeID, ports.NodeID) {
		keys := make([]ed25519.PrivateKey, 4)
		anchors := map[ports.NodeID]bool{}
		for i := range keys {
			keys[i] = key(int64(35000 + i))
			anchors[idOf(keys[i])] = true
		}
		c1, c2 := key(35101), key(35102) // bonded, non-anchor culprits
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = append(g.BondRegs,
			bondReg(c1, twoMiB, ports.Hash{}), bondReg(c2, twoMiB, ports.Hash{}))
		Sign(g, keys[0])
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		if c.bonded[idOf(c1)] == 0 || c.bonded[idOf(c2)] == 0 {
			t.Fatalf("setup: both culprits must be bonded (c1=%d c2=%d)", c.bonded[idOf(c1)], c.bonded[idOf(c2)])
		}

		first, second := c1, c2
		if !slashC1First {
			first, second = c2, c1
		}
		g0 := g.Hash()
		b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g0,
			Slashes: []Equivocation{slashProof(first, g0, 101, 102)}}
		commitRounds(b1, keys, 0)
		if err := c.Append(*b1); err != nil {
			t.Fatalf("slash first: %v", err)
		}
		b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
			Slashes: []Equivocation{slashProof(second, g0, 103, 104)}}
		commitRounds(b2, keys, 0)
		if err := c.Append(*b2); err != nil {
			t.Fatalf("slash second: %v", err)
		}
		return c, idOf(c1), idOf(c2)
	}

	a, c1, c2 := build(true) // slash C1 then C2
	b, _, _ := build(false)  // slash C2 then C1

	// The interaction is genuinely exercised: both culprits' bonded standing was
	// removed by the paired delete, so bonded lost two keys in opposite orders.
	if _, ok := a.bonded[c1]; ok {
		t.Fatal("premise broken: c1 must be removed from bonded by its slash")
	}
	if _, ok := a.bonded[c2]; ok {
		t.Fatal("premise broken: c2 must be removed from bonded by its slash")
	}
	if !a.slashed[c1] || !a.slashed[c2] {
		t.Fatal("premise broken: both culprits must be slashed")
	}

	if !reflect.DeepEqual(a.bonded, b.bonded) {
		t.Fatalf("bonded DIFFERS across two slash orderings (%v vs %v) — the paired "+
			"delete(c.bonded, culprit) in apply() is order-sensitive, which would make "+
			"bonded an order-dependent SMT leaf. This is a soundness finding to route, "+
			"not a test to relax.", a.bonded, b.bonded)
	}
	if !reflect.DeepEqual(a.slashed, b.slashed) {
		t.Fatalf("slashed differs across orderings (%v vs %v)", a.slashed, b.slashed)
	}
	t.Logf("bonded and slashed byte-identical across two opposite slash orderings "+
		"(bonded=%d entries, slashed=%d) — the paired delete commutes", len(a.bonded), len(a.slashed))
}
