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

// orderVacuous names committedSet fields that twoOrderings genuinely cannot
// populate — they belong to a regime this launch/objective, never-mature,
// gate-inactive world does not enter. Each is compared as DeepEqual(∅, ∅) here,
// so its order-independence is NOT proven by this oracle; the entry records what
// a covering fixture would have to construct. This is a DECLARED, SHRINKING debt
// (mirroring probeUncovered in the snapshot oracle): the guard below fails on any
// NEW empty-vs-empty committedSet field not listed here, so the vacuous-green
// hole can never silently reopen. `spent` and `slashed` were moved OUT of this
// bucket by populating them with a real spend/slash order.
var orderVacuous = map[string]string{
	// Bond-registration state (bonded, bondRootOwner, bondRootProven, bondRegHeight,
	// regVersion, bondDomain) is now COVERED: twoOrderings commits a height-5 bond
	// block whose BondReg slice order flips between the two orderings, including a G3
	// proof-beats-declaration displacement of a genesis squatter. The fields are
	// non-empty in both orderings, so the guard below enforces they stay covered.
	// Mature-epoch state: this world sets MatureValidators=99 and never matures,
	// so epochStart/rotateEpoch never freezes an epoch set.
	"epochSet":    "needs a mature epoch (rotateEpoch freeze); this world never matures",
	"matureEpoch": "needs the #357 Condition-B handoff; this world never matures",
	"everMature":  "needs the maturity latch to trip; MatureValidators=99 keeps it launch-phase",
	// #506 registration-gate state: no RegGateActivationHeight configured here.
	"gateLockedIn": "needs the #506 gate to lock in (no RegGateActivationHeight in this world)",
	"gateHeight":   "same as gateLockedIn",
	// validatorsSeen is NOT listed: the attesting anchors qualify, so apply()
	// populates it — its order-independence is genuinely exercised here.
}

// TestCommittedSetFieldsAreOrderIndependent is the load-bearing half. Every
// field classified `committedSet` goes under the history-independent SMT, so
// two histories reaching the same final set MUST agree on it exactly.
func TestCommittedSetFieldsAreOrderIndependent(t *testing.T) {
	a, b := twoOrderings(t)

	fields := fieldsOfKind(committedSet)
	if len(fields) == 0 {
		t.Fatal("no committedSet fields — the classification or reflection is broken")
	}

	// The durable fix (PE ruling "Coupling", 2026-08-28): a field that is EMPTY
	// in both orderings is compared as DeepEqual(∅, ∅) — a vacuous green that
	// asserts nothing. Every committedSet field the oracle claims to prove
	// order-independent must be NON-EMPTY in at least one ordering, OR be
	// explicitly declared in orderVacuous with the reason twoOrderings cannot
	// populate it. Otherwise "all N identical" reads as coverage while some
	// fraction of it is empty-vs-empty — exactly the shape that let `spent` and
	// `slashed` show green over an unexercised map.
	var undeclaredVacuous []string
	populated := map[string]bool{}
	for _, name := range fields {
		if isZero(fieldValue(a, name)) && isZero(fieldValue(b, name)) {
			if _, declared := orderVacuous[name]; !declared {
				undeclaredVacuous = append(undeclaredVacuous, name)
			}
			continue
		}
		populated[name] = true
		// A field cannot be both populated here and declared un-populatable.
		if _, declared := orderVacuous[name]; declared {
			t.Errorf("%q is populated by twoOrderings yet still listed in orderVacuous "+
				"— remove the stale entry; its order-independence is now genuinely exercised.", name)
		}
	}
	if len(undeclaredVacuous) > 0 {
		t.Fatalf("%d committedSet field(s) are EMPTY in both orderings and NOT declared "+
			"in orderVacuous, so the order-independence comparison over them is "+
			"DeepEqual(∅, ∅) — vacuous, asserting nothing: %v\n\n"+
			"A field under the history-independent SMT must have its order-independence "+
			"EXERCISED, not merely restated over an empty map. Extend twoOrderings to "+
			"populate the field with a real event order (see the spent/slashed spends "+
			"and slashes), or record in orderVacuous what a fixture would have to "+
			"construct. Do not let the count of 'identical' fields include ones this "+
			"history never touched.", len(undeclaredVacuous), undeclaredVacuous)
	}
	// Proof the coverage is real: the fields this work exists to cover must be
	// non-empty in the fixture, not merely absent from orderVacuous. spent/slashed
	// come from the spend+slash pairs; the bond-registration family comes from the
	// height-5 bond block with its slice-order variation (incl. the G3 displacement).
	for _, name := range []string{"spent", "slashed",
		"bonded", "bondRootOwner", "bondRootProven", "bondRegHeight", "regVersion", "bondDomain"} {
		if !populated[name] {
			t.Fatalf("%q must be NON-EMPTY in twoOrderings — the whole point of this "+
				"fixture is to exercise its order-independence over a real event order, "+
				"not DeepEqual(∅, ∅). len(a)=%d len(b)=%d", name,
				reflect.ValueOf(fieldValue(a, name)).Len(), reflect.ValueOf(fieldValue(b, name)).Len())
		}
	}

	var orderDependent []string
	for _, name := range fields {
		if !reflect.DeepEqual(fieldValue(a, name), fieldValue(b, name)) {
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
	t.Logf("all %d committedSet fields identical across opposite event orderings", len(fields))
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
// trip-wire for the bond-registration family. It is NOT enough that the
// committedSet fields happen to match across the two orderings — the match must
// be over a G3 displacement that ACTUALLY FIRED, else the coverage is vacuous.
//
// This asserts the end state directly: in BOTH orderings the genesis squatter is
// removed from bonded and is no longer the owner of the shared root, honestH is
// the PROVEN owner, and validatorX is bonded on its own root. If the two
// orderings had reached DIFFERENT bond-root states, that would be a real consensus
// finding (an order-sensitive displacement validity rule under a history-
// independent root) — STOP-and-escalate, no rule change. They do not: the G3 rule
// is order-INDEPENDENT here.
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
