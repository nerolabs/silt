package chain

import (
	"crypto/ed25519"
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/nerolabs/silt/ports"
)

// RED home #1 (part 2) — snapshot-boot equivalence, the differential half.
//
// Part 1 (modelcheck_state_completeness_test.go) proves the field enumeration
// cannot silently DRIFT. It does not prove the enumeration is SUFFICIENT. This
// file is the other half: a validator that boots from committed state alone —
// never having replayed the history — must reach the same validity verdicts as
// one that replayed. That is the property the whole keystone rests on:
//
//	the root [must] be identical however the state was reached — a
//	snapshot-booted node never replayed the history.
//
// The snapshot-booted replica is built by reflection over the classification,
// not from a hand-written capture list. Note that `blocks` is classified
// `input` rather than `committed`, so "copy every committed field" produces a
// replica with NO history by construction — exactly a snapshot-booted node.
// That falls out of the classification instead of being asserted separately.

// setField writes an unexported field. Reading uses the same trick in part 1;
// the alternative is a hand-written capture list, i.e. another enumeration to
// keep in sync, which is the failure this whole oracle exists to prevent.
func setField(c *Chain, name string, v any) {
	f := reflect.ValueOf(c).Elem().FieldByName(name)
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(v))
}

// snapshotCarried is everything a state snapshot must carry: set-valued state,
// the ordered log (the full entry list, per the #597 certification), and
// observables. Note this is the same set adopt() owes — a snapshot and a reorg
// swap face the identical completeness question.
func snapshotCarried() []string {
	var out []string
	for _, k := range []stateKind{committedSet, committedLog, observable} {
		out = append(out, fieldsOfKind(k)...)
	}
	return out
}

// fieldsOfKind returns the live struct's field names with the given class.
func fieldsOfKind(k stateKind) []string {
	ct := reflect.TypeOf(Chain{})
	var out []string
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		if c, ok := stateClass[name]; ok && c.kind == k {
			out = append(out, name)
		}
	}
	return out
}

// deepCopyValue returns an independent copy of a map or slice value, so a replica
// that mutates its carried state (a mutating probe's apply()) cannot write through
// a shared reference into src or into a sibling replica. Non-map/slice values (the
// scalars everMature/matureEpoch/epochStart) are returned as-is — they are value
// types, so a copy is automatic on assignment. Maps are copied one level deep,
// which is sufficient here: every committed map's VALUE is a scalar or an array
// (NodeID/Hash/Entry), none of which apply() mutates in place.
func deepCopyValue(v any) any {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		for _, k := range rv.MapKeys() {
			out.SetMapIndex(k, rv.MapIndex(k))
		}
		return out.Interface()
	case reflect.Slice:
		if rv.IsNil() {
			return v
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		reflect.Copy(out, rv)
		return out.Interface()
	default:
		return v
	}
}

// snapshotBoot builds the "never replayed" replica: injected configuration
// carried over (identical on every replica by construction), every committed
// field copied EXCEPT those named in omit, and no block history at all.
//
// Carried maps/slices are DEEP-COPIED (deepCopyValue): a snapshot replica is
// handed to mutating probes that call apply(), which writes into these maps. If
// the replica shared src's map header, that apply() would corrupt src and every
// sibling replica built from it — the leave-one-out loop ablates one field at a
// time off the SAME src, so a mutating probe on the k-th ablation would poison the
// (k+1)-th. That aliasing silently masked bondRootProven's flip (the F1 probe on
// the bondRootOwner ablation displaced src's shared bonded/owner maps before
// bondRootProven was ever ablated). Deep-copying makes each replica own its state.
func snapshotBoot(src *Chain, omit ...string) *Chain {
	skip := map[string]bool{}
	for _, o := range omit {
		skip[o] = true
	}
	dst := &Chain{}
	for _, name := range fieldsOfKind(injected) {
		setField(dst, name, fieldValue(src, name))
	}
	for _, name := range snapshotCarried() {
		if skip[name] {
			// Model the omission faithfully: a snapshot that failed to CARRY a
			// field leaves the booted node with an initialised-but-empty one,
			// not a nil one. Leaving it nil would make apply() panic, and a
			// panic masks the finding — the point is to see what the node
			// wrongly ACCEPTS, not that it crashes.
			f := reflect.ValueOf(dst).Elem().FieldByName(name)
			if f.Kind() == reflect.Map {
				setField(dst, name, reflect.MakeMap(f.Type()).Interface())
			}
			continue
		}
		setField(dst, name, deepCopyValue(fieldValue(src, name)))
	}
	return dst
}

// A probe is a validity question whose answer depends on accumulated state.
// Each names the committed field(s) it is designed to detect the loss of.
type probe struct {
	name   string
	detect []string // committed fields whose omission this probe should expose
	// mutates marks a probe that APPLIES a block. Some rules (the F1 bond-root
	// dedup) live in apply(), not in a validate predicate, so a verdict-only
	// probe is structurally blind to them. Such probes run against throwaway
	// replicas and are skipped by the replay-vs-snapshot comparison, which must
	// not mutate the replayed chain.
	mutates bool
	ask     func(c *Chain) string
}

// A worldGroup binds a set of probes to the replay-booted world they interrogate.
// Most probes run against the launch-phase richHistory world. The consensus-weight
// probes (bonded, epochSet) cannot: bonded is only read for a verdict where
// qualification consults the live bonded map (a non-epoch objective regime), while
// epochSet's frozen-set membership only governs a MATURE epoch — the two regimes are
// mutually exclusive, so each gets its own world (see the deliberation
// docs/thinking/2026-08-27-keystone-probes-bonded-epochset.md). Keeping the world
// beside the probes lets one leave-one-out loop ablate each field on the world where
// it is actually load-bearing.
type worldGroup struct {
	name   string
	build  func(t *testing.T) *Chain
	probes []probe
}

// askSafely runs a probe and converts a panic into a verdict. A replica missing
// a committed map does not politely disagree — apply() writes to a nil map and
// crashes. That is still divergence, and the loudest kind, so it is captured
// rather than allowed to take the suite down.
func askSafely(p probe, c *Chain) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = "panic"
		}
	}()
	return p.ask(c)
}

// verdict renders an error as a comparable outcome string.
func verdict(err error) string {
	if err == nil {
		return "accept"
	}
	return "reject"
}

// richHistory commits a history that populates the state the probes read, and
// returns the replay-booted chain plus the roots it created.
// The fourth return is the bond root keys[1] already owns (bondReg derives
// Root from the public key), which the F1 dedup probe tries to steal.
func richHistory(t *testing.T) (*Chain, []ed25519.PrivateKey, ports.Hash, ports.Hash) {
	t.Helper()
	c, keys, g := roundsWorld(t)

	// Height 1: an entry plus a bond registration — populates byRoot, bonded,
	// bondRootOwner, bondRootProven, bondRegHeight, regVersion, bondDomain.
	published := entry(9)
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Entries: []ports.Entry{published}}
	b1.BondRegs = append(b1.BondRegs, bondReg(keys[1], twoMiB, g.Hash()))
	commitRounds(b1, keys, 0)
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit height 1: %v", err)
	}

	// Height 2: revoke the published root — populates revoked and appends to
	// revLog.
	b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
		Revocations: []ports.Hash{published.Root}}
	commitRounds(b2, keys, 0)
	if err := c.Append(*b2); err != nil {
		t.Fatalf("commit height 2: %v", err)
	}

	return c, keys, published.Root, ports.HashBytes(keys[1].Public().(ed25519.PublicKey))
}

// commitRounds attaches a full era-2 two-phase certificate at the given round.
func commitRounds(b *Block, keys []ed25519.PrivateKey, round uint64) {
	Sign(b, keys[0])
	b.CommitRound = round
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, keys[0], round, PhasePrepare))
	for _, k := range keys[1:] {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, round, PhasePrepare))
	}
	b.Atts = append(b.Atts, AttestAt(b, keys[0], round, PhasePrecommit))
	for _, k := range keys[1:] {
		b.Atts = append(b.Atts, AttestAt(b, k, round, PhasePrecommit))
	}
}

func probes(revokedRoot, ownedRoot ports.Hash, keys []ed25519.PrivateKey, prev ports.Hash) []probe {
	return []probe{
		{
			name:   "dup-publish must be rejected",
			detect: []string{"byRoot"},
			ask:    func(c *Chain) string { return verdict(c.ValidateEntry(entry(9))) },
		},
		{
			name:   "revoking an unknown root must be rejected",
			detect: []string{"byRoot"},
			ask: func(c *Chain) string {
				return verdict(c.validateTakedowns(&Block{Revocations: []ports.Hash{entry(77).Root}}))
			},
		},
		{
			name:   "un-revoking a root that was revoked must be ACCEPTED",
			detect: []string{"revoked"},
			ask: func(c *Chain) string {
				return verdict(c.validateTakedowns(&Block{Unrevocations: []ports.Hash{revokedRoot}}))
			},
		},
		{
			// F1 first-owner-wins lives in apply(), not in a validate predicate:
			// a second identity claiming an already-owned bond root must NOT
			// gain standing. Without bondRootOwner the claim succeeds, which is
			// one plot backing two identities — a direct C1 no-discount break.
			name:    "a second identity cannot take an already-owned bond root",
			detect:  []string{"bondRootOwner"},
			mutates: true,
			ask: func(c *Chain) string {
				claimant := idOf(keys[2])
				b := Block{Version: BlockVersionRounds, Height: 3, Prev: prev}
				b.BondRegs = append(b.BondRegs, bondRegAt(keys[2], ownedRoot, twoMiB, prev))
				c.apply(b)
				if _, ok := c.bonded[claimant]; ok {
					return "claim-succeeded"
				}
				return "claim-blocked"
			},
		},
		{
			// bondRootProven is the G3 discriminator (chain.go:2786): once a root's
			// owner is PROVEN, a later proven claim by another identity must NOT
			// displace it (F1 holds among proven claims). richHistory's owner
			// (keys[1], height>0) IS proven, so a second PROVEN claim on ownedRoot is
			// blocked. A snapshot that lost bondRootProven sees the owner as merely
			// DECLARED, so `proven && !bondRootProven[root]` fires the displacement —
			// the true owner is wrongly stripped and the challenger earns the root.
			// That is a C1 no-discount break the field exists to prevent.
			name:    "a proven bond-root owner cannot be displaced by a later proven claim",
			detect:  []string{"bondRootProven"},
			mutates: true,
			ask: func(c *Chain) string {
				challenger := idOf(keys[2])
				b := Block{Version: BlockVersionRounds, Height: 3, Prev: prev}
				b.BondRegs = append(b.BondRegs, bondRegAt(keys[2], ownedRoot, twoMiB, prev))
				c.apply(b)
				if _, ok := c.bonded[challenger]; ok {
					return "displaced-the-proven-owner"
				}
				return "proven-owner-held"
			},
		},
	}
}

// weightWorld is a MATURE-EPOCH world (no anchors, epochs on, MatureValidators
// unset so Mature() holds and everMature latches at genesis → rotateEpoch freezes
// epochSet immediately). Four equal 2 MiB bonds are frozen into epochSet. It returns
// the replay-booted chain plus a block the FULL frozen set ACCEPTS (proposer keys[0]
// + two attesters, well clear of any count or weight floor).
//
// The flip on omission is carried by FROZEN-SET MEMBERSHIP, not the ⅔-weight
// predicate. Omitting epochSet restores an EMPTY frozen set, so the attesters fail
// effectiveEpochSet membership in attesterQualifiedAt, seen collapses to 0, and the
// COUNT floor rejects: ErrNoQuorum. The weight predicate requireEpochWeightQuorum
// never fires — with epochSet empty its `total <= 0` branch short-circuits to nil
// (chain.go:2452), so if membership were not the discriminator the block would not
// flip at all. This probe proves epochSet MEMBERSHIP is load-bearing; the per-member
// WEIGHT bytes are a separate claim, owed as its own probe (issue #603, the era-3
// format-freeze gate).
func weightWorld(t *testing.T) (*Chain, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = key(int64(31000 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("weightWorld genesis: %v", err)
	}

	// A commit at height 1 the FULL frozen set accepts: proposer keys[0] + two
	// attesters, well clear of both the count floor and the ⅔-weight predicate.
	// Omitting epochSet empties frozen membership → attesters disqualified →
	// ErrNoQuorum (count floor), not the weight predicate. See the doc comment.
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(30)}}
	Sign(b, keys[0])
	b.Atts = []Attestation{Attest(b, keys[1]), Attest(b, keys[2])}
	return c, b
}

// bondedWorld is an ANCHORLESS objective world with epochs disabled. everMature
// latches (MatureValidators unset), so there is no launch-anchor crutch and
// qualification consults the bonded map DIRECTLY (attesterQualifiedAt /
// proposerQualifiedAt fall through to `bonded[id] >= MinBond || launchAnchor(id)`,
// and launchAnchor is false). It returns the replay-booted chain plus a block whose
// quorum is carried by bonded non-proposers. Omitting bonded disqualifies the
// proposer (and drops the attesters from seen), so the same block is REJECTED — the
// bonded flip. This changes which identities are ADMITTED as qualified, not how any
// weight/count is summed (the #402 seam is untouched).
func bondedWorld(t *testing.T) (*Chain, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = key(int64(32000 + i))
	}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("bondedWorld genesis: %v", err)
	}

	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(9)}}
	Sign(b, keys[0])
	b.Atts = []Attestation{Attest(b, keys[1]), Attest(b, keys[2])}
	return c, b
}

// spentWorld is a token-required launch/objective world with ONE serial already
// spent (a committed token entry). It returns the replay-booted chain plus the
// serial that is now spent. The probe re-submits that serial on a FRESH root:
// ValidateEntry rejects it (ErrTokenSpent, chain.go:2229). A snapshot that lost
// `spent` no longer sees the serial as used, re-verifies the still-valid token,
// and ACCEPTS the replay — the double-spend the spent set exists to prevent.
func spentWorld(t *testing.T) (*Chain, []byte, func([]byte) *ports.PublishToken) {
	t.Helper()
	oi := newOrderIssuers(t)
	c, g := orderWorld(t, oi, ports.Hash{}, nil)

	serial := []byte("leaveoneout-spent-serial")
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Entries: []ports.Entry{tokenEntry(7, oi.mint(serial))}}
	commitRounds(b1, oi.keys, 0)
	if err := c.Append(*b1); err != nil {
		t.Fatalf("spentWorld commit: %v", err)
	}
	return c, serial, oi.mint
}

// slashedWorld is an anchor launch world with ONE anchor slashed by a committed
// equivocation proof. It returns the replay-booted chain plus the slashed anchor
// ID. The probe asks attesterQualified(culprit): a slashed identity is refused
// (chain.go:1026) BEFORE the launchAnchor fallthrough. A snapshot that lost
// `slashed` re-admits the anchor via launchAnchor — the flip depends on `slashed`
// alone, not bonded (the anchor never carried a bond). The culprit is the FIFTH
// key, not one of the four whose quorum commits the slash block, so slashing it
// never disturbs that quorum.
func slashedWorld(t *testing.T) (*Chain, ports.NodeID) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(34000 + i))
		anchors[idOf(keys[i])] = true
	}
	culprit := key(34099) // a fifth anchor, not needed for the carrying quorum
	anchors[idOf(culprit)] = true
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("slashedWorld genesis: %v", err)
	}

	// The culprit provably double-signs; height 1 carries the slash. Committed by
	// the four-anchor quorum, which the culprit is not part of.
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Slashes: []Equivocation{slashProof(culprit, g.Hash(), 101, 102)}}
	commitRounds(b1, keys, 0)
	if err := c.Append(*b1); err != nil {
		t.Fatalf("slashedWorld slash: %v", err)
	}
	return c, idOf(culprit)
}

// setValuedProbes are the leave-one-out probes for `spent` and `slashed`, each
// bound to the world that populates it. block/serial are prebuilt by the world;
// the field's omission (via snapshotBoot) is the only variable.
func spentProbe(serial []byte, mint func([]byte) *ports.PublishToken) probe {
	return probe{
		name:   "re-spending a committed serial on a fresh root must be rejected; a snapshot that lost spent accepts the double-spend",
		detect: []string{"spent"},
		ask: func(c *Chain) string {
			e := entry(99) // a fresh, never-published root
			e.Token = mint(serial)
			return verdict(c.ValidateEntry(e))
		},
	}
}

func slashedProbe(culprit ports.NodeID) probe {
	return probe{
		name:   "a slashed anchor must be disqualified; a snapshot that lost slashed re-admits it via launchAnchor",
		detect: []string{"slashed"},
		ask: func(c *Chain) string {
			if c.attesterQualified(culprit) {
				return "qualified"
			}
			return "disqualified"
		},
	}
}

// quorumVerdict runs the real qualification + quorum path for a block WITHOUT the
// head-extension check: collectQuorumSigs admits attesters via attesterQualifiedAt
// (which reads bonded in the objective regime and epochSet in a mature epoch), then
// requireQuorumStack applies the count floor and the mature-epoch weight rule. A
// snapshot-booted node has no block history, so the full ValidateCommit path fails
// its Prev==head check for any real block — the same reason the set-valued probes
// call ValidateEntry/validateTakedowns directly rather than ValidateCommit. This
// isolates exactly the predicate the ablated field feeds.
func quorumVerdict(c *Chain, b *Block) string {
	seen, err := c.collectQuorumSigs(b, b.Atts, PhaseLegacy, 0)
	if err != nil {
		return "reject"
	}
	return verdict(c.requireQuorumStack(b, seen))
}

// weightProbes are the two consensus-weight probes, each bound to the world where
// its field flips a verdict. block is prebuilt by the world; the probe asks the
// qualification+quorum verdict, so the field's omission (via snapshotBoot) is the
// ONLY variable.
func weightProbes(epochSetBlock, bondedBlock *Block) ([]probe, []probe) {
	epochSetProbe := probe{
		name:   "a mature-epoch commit by frozen members must be accepted; a snapshot that lost epochSet empties frozen membership and rejects it (ErrNoQuorum)",
		detect: []string{"epochSet"},
		ask:    func(c *Chain) string { return quorumVerdict(c, epochSetBlock) },
	}
	bondedProbe := probe{
		name:   "an objective commit by bonded validators must be accepted; a snapshot that lost bonded rejects it",
		detect: []string{"bonded"},
		ask:    func(c *Chain) string { return quorumVerdict(c, bondedBlock) },
	}
	return []probe{epochSetProbe}, []probe{bondedProbe}
}

// ---------------------------------------------------------------------------
// The latch / gate / domain tranche (2026-08-28). Six committed fields whose
// leave-one-out flip lives outside the qualification+quorum count path the
// bonded/epochSet probes drive. Each world bakes the exact regime state a
// snapshot would carry (via setField on the built chain, so snapshotBoot
// deep-copies it into the replica) and each probe drives the ONE predicate the
// ablated field feeds. See docs/thinking/2026-08-28-keystone-leaveoneout-latch-
// gate-domain.md for the per-field mechanism and the STOP-boundary analysis.
// ---------------------------------------------------------------------------

// regVerdict validates a bond-registration block against a snapshot-booted node
// via validateBondRegs — the #506 R-rule path (chain.go:1497). A history-less
// replica CAN drive it: recentBondRegNonces returns BondRegNonce(prev) as its
// first window nonce regardless of block history (chain.go:1385-1386 appends
// before the blockByHash break), so a reg signed against prev validates. This is
// the ValidateCommit-free entry point the gate fields feed, mirroring how the
// set-valued probes call ValidateEntry directly.
func regVerdict(c *Chain, b *Block) string { return verdict(c.validateBondRegs(b)) }

// deMatureWorld (everMature). Epochs DISABLED, no anchors, so handedOff == the raw
// everMature latch and requireEpochWeightQuorum never fires (it needs
// epochsEnabled) — the de-maturation super-quorum is the ONLY regime gate. The
// ramp latches everMature while decentralized (6 equal bonds, MatureValidators=2);
// the built chain's LIVE bonded is then set to a whale-dominated split so
// matureNow() is false. The probed block is a minnow coalition that clears the
// count floor (bftThreshold(6)=4 attesters) but holds far below ⅔ of live bonded
// weight. Full snapshot → requireDeMatureSuperQuorum rejects (ErrDeMatureQuorum);
// an everMature-dropped snapshot skips the bar (chain.go:2471) and accepts. The
// flip changes whether the de-mature bar APPLIES, never how weight is summed.
func deMatureWorld(t *testing.T) (*Chain, *Block) {
	t.Helper()
	whale := key(40000)
	minnows := make([]ed25519.PrivateKey, 5)
	for i := range minnows {
		minnows[i] = key(int64(40001 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	all := append([]ed25519.PrivateKey{whale}, minnows...)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range all {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("deMatureWorld genesis: %v", err)
	}
	// A rotating-proposer ramp so all six enter validatorsSeen (the coefficient
	// reads validatorsSeen) → the maturity latch trips → everMature is committed.
	prev := g.Hash()
	n := len(all)
	for h := uint64(1); h <= 3; h++ {
		order := make([]ed25519.PrivateKey, n)
		for i := 0; i < n; i++ {
			order[i] = all[(int(h)+i)%n]
		}
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, order[0])
		for _, a := range order[1:] {
			b.Atts = append(b.Atts, Attest(b, a))
		}
		if err := c.Append(*b); err != nil {
			t.Fatalf("deMatureWorld block %d: %v", h, err)
		}
		prev = b.Hash()
	}
	if !c.everMature {
		t.Fatalf("deMatureWorld: everMature did not latch (coeff=%d)", c.MatureCoefficient())
	}

	// Realize the de-maturation regime: live decentralization has since dropped —
	// the whale concentrated real bond and minnows shrank. matureNow() now false.
	liveBonded := map[ports.NodeID]int64{idOf(whale): 100 << 20}
	for _, m := range minnows {
		liveBonded[idOf(m)] = 1 << 20
	}
	setField(c, "bonded", liveBonded)

	// The probed block: a minnow coalition (proposer + 4 minnow attesters, clearing
	// bftThreshold(6)=4) whose weight is a sliver of the whale-dominated total.
	b := &Block{Version: 1, Height: 4, Prev: prev, Entries: []ports.Entry{entry(44)}}
	Sign(b, minnows[0])
	b.Atts = []Attestation{Attest(b, minnows[1]), Attest(b, minnows[2]),
		Attest(b, minnows[3]), Attest(b, minnows[4])}
	return c, b
}

func everMatureProbe(b *Block) probe {
	return probe{
		name: "a de-matured network must refuse a sub-⅔ real-bond coalition; a snapshot that " +
			"lost everMature skips the de-mature bar (ErrDeMatureQuorum) and accepts it",
		detect: []string{"everMature"},
		ask:    func(c *Chain) string { return quorumVerdict(c, b) },
	}
}

// matureEpochWorld (matureEpoch). Epochs ON; genesis freezes four members into
// epochSet with UNEQUAL weight (two silent whales). The built chain's LIVE bonded
// is then narrowed to just the two coalition members, so in a non-mature regime
// the count floor is bftThreshold(2)=1 (satisfiable by the single attester), while
// the frozen epochSet still weights the silent whales. The probed block is
// proposer+one attester: below ⅔ of frozen epoch weight but clearing the count
// floor. Full snapshot (matureEpoch true) → requireEpochWeightQuorum rejects
// (ErrNoQuorumWeight, chain.go:2457). A matureEpoch-dropped snapshot skips the
// weight quorum AND qualification leaves the frozen-set branch → the count floor
// (now bftThreshold(2)=1) is cleared → accept. The flip changes whether the
// mature-epoch weight rule APPLIES, never how the ⅔ is summed.
func matureEpochWorld(t *testing.T) (*Chain, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = key(int64(46000 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	// Frozen weights: proposer keys[0]=1 MiB, attester keys[1]=1 MiB, silent whales
	// keys[2]/keys[3]=10 MiB each. All ≥ MinBond so all four freeze at genesis.
	weights := []int64{1 << 20, 1 << 20, 10 << 20, 10 << 20}
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, weights[i], ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("matureEpochWorld genesis: %v", err)
	}
	if !c.matureEpoch {
		t.Fatalf("matureEpochWorld: matureEpoch did not set (epochSet=%d)", len(c.epochSet))
	}
	// Live bonded (governs the non-mature count floor + qualification once matureEpoch
	// is dropped): only the two coalition members remain, so bftThreshold(2)=1.
	setField(c, "bonded", map[ports.NodeID]int64{
		idOf(keys[0]): 1 << 20, idOf(keys[1]): 1 << 20,
	})

	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(30)}}
	Sign(b, keys[0])
	b.Atts = []Attestation{Attest(b, keys[1])}
	return c, b
}

func matureEpochProbe(b *Block) probe {
	return probe{
		name: "a mature-epoch commit below ⅔ frozen weight must be refused; a snapshot that lost " +
			"matureEpoch skips the frozen-weight quorum (ErrNoQuorumWeight) and accepts on the count floor",
		detect: []string{"matureEpoch"},
		ask:    func(c *Chain) string { return quorumVerdict(c, b) },
	}
}

// gateWorld (gateLockedIn, gateHeight). Latches maturity and LOCKS the #506 gate at
// a boundary: three ready members (regVersion ≥ BlockVersionRegGate) freeze into
// epochSet, the rotateEpoch tally clears the ⅔-ready super-quorum, so gateLockedIn
// is set and gateHeight = boundary + EpochBlocks. Member x carries a recent
// bondRegHeight (its height-1 re-reg). The two probes validate a within-R re-reg
// for x on this world.
func gateWorld(t *testing.T) *Chain {
	t.Helper()
	const w = int64(2) << 20
	r1, r2, x := key(70001), key(70002), key(70003)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 2, BondTTLBlocks: 40}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondRegFull(r1, ports.HashBytes(pubOf(r1)), w, ports.Hash{}, BlockVersionRegGate, 0),
		bondRegFull(r2, ports.HashBytes(pubOf(r2)), w, ports.Hash{}, BlockVersionRegGate, 0),
		bondRegFull(x, ports.HashBytes(pubOf(x)), w, ports.Hash{}, BlockVersionRegGate, 0),
	}
	Sign(g, r1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("gateWorld genesis: %v", err)
	}
	// Height 1: x re-registers its OWN root (bondRegHeight[x]=1). Proposer r1.
	rootX := ports.HashBytes(pubOf(x))
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Entries:  []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondRegFull(x, rootX, w, g.Hash(), BlockVersionRegGate, 0)}}
	commitRounds(b1, []ed25519.PrivateKey{r1, r2, x}, 0)
	if err := c.Append(*b1); err != nil {
		t.Fatalf("gateWorld height 1: %v", err)
	}
	// Height 2: the epoch boundary. r2 proposes so r1 joins validatorsSeen → three
	// distinct seen bonds → maturity latches, rotateEpoch freezes {r1,r2,x} and the
	// #506 tally locks the gate (all ready). gateHeight = 2 + EpochBlocks = 4.
	b2 := &Block{Version: BlockVersionRounds, Height: 2, Prev: b1.Hash(),
		Entries: []ports.Entry{entry(2)}}
	commitRounds(b2, []ed25519.PrivateKey{r2, r1, x}, 0)
	if err := c.Append(*b2); err != nil {
		t.Fatalf("gateWorld boundary: %v", err)
	}
	if !c.gateLockedIn {
		t.Fatalf("gateWorld: gate did not lock (gateHeight=%d)", c.gateHeight)
	}
	return c
}

// gateProbes builds the within-R re-registration blocks for x at heights straddling
// gateHeight, and the two probes that drive them. bondRegHeight[x]=1 and R≈10, so a
// reg at any height within 10 of block 1 is "too soon" ONLY where the gate is active.
func gateProbes(c *Chain) (probe, probe) {
	x := key(70003)
	rootX := ports.HashBytes(pubOf(x))
	gh := c.gateHeight
	regAt := func(h uint64) *Block {
		return &Block{Version: BlockVersionRounds, Height: h, Prev: ports.Hash{},
			BondRegs: []BondReg{bondRegFull(x, rootX, twoMiB, ports.Hash{}, BlockVersionRegGate, 0)}}
	}
	// gateLockedIn: a within-R re-reg PAST gateHeight. Full → ErrRegGate (gate active);
	// gateLockedIn-dropped (→ false) → regGateActive false → the R-rule never fires → accept.
	past := regAt(gh + 1)
	lockedInProbe := probe{
		name: "a within-R re-registration past H_act must be rejected; a snapshot that lost " +
			"gateLockedIn treats the gate as never armed (ErrRegGate → accept)",
		detect: []string{"gateLockedIn"},
		ask:    func(c *Chain) string { return regVerdict(c, past) },
	}
	// gateHeight: a within-R re-reg BELOW gateHeight (pre-gate, so accepted). Full →
	// accept; gateHeight-dropped (→ 0) makes regGateActive = gateLockedIn && h>0 fire
	// for this height → the R-rule rejects. The flip runs the OPPOSITE way.
	pre := regAt(gh - 1)
	heightProbe := probe{
		name: "a within-R re-registration BELOW H_act must be accepted; a snapshot that lost " +
			"gateHeight collapses H_act to 0, activating the gate early (accept → ErrRegGate)",
		detect: []string{"gateHeight"},
		ask:    func(c *Chain) string { return regVerdict(c, pre) },
	}
	return lockedInProbe, heightProbe
}

// regVersionWorld (regVersion). regVersion is read at exactly ONE verdict-relevant
// site: rotateEpoch's #506 lock-in tally (chain.go:3007). It feeds no Validate path,
// so its leave-one-out flip must let rotateEpoch RUN on the snapshot replica. The
// world stops ONE block short of the boundary: everMature not yet latched, gate not
// yet locked, three ready members carrying regVersion. The probe applies the
// boundary block (a mutating probe), which trips the latch and runs the tally.
func regVersionWorld(t *testing.T) *Chain {
	t.Helper()
	const w = int64(2) << 20
	r1, r2, x := key(71001), key(71002), key(71003)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 2, BondTTLBlocks: 40}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondRegFull(r1, ports.HashBytes(pubOf(r1)), w, ports.Hash{}, BlockVersionRegGate, 0),
		bondRegFull(r2, ports.HashBytes(pubOf(r2)), w, ports.Hash{}, BlockVersionRegGate, 0),
		bondRegFull(x, ports.HashBytes(pubOf(x)), w, ports.Hash{}, BlockVersionRegGate, 0),
	}
	Sign(g, r1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("regVersionWorld genesis: %v", err)
	}
	// Height 1 only (r1 proposes → seen={r2,x}); the boundary is deferred to the probe.
	rootX := ports.HashBytes(pubOf(x))
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(),
		Entries:  []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondRegFull(x, rootX, w, g.Hash(), BlockVersionRegGate, 0)}}
	commitRounds(b1, []ed25519.PrivateKey{r1, r2, x}, 0)
	if err := c.Append(*b1); err != nil {
		t.Fatalf("regVersionWorld height 1: %v", err)
	}
	if c.gateLockedIn || c.everMature {
		t.Fatalf("regVersionWorld: expected pre-latch pre-lock snapshot, got everMature=%v gateLockedIn=%v",
			c.everMature, c.gateLockedIn)
	}
	return c
}

func regVersionProbe() probe {
	r1, r2, x := key(71001), key(71002), key(71003)
	rootX := ports.HashBytes(pubOf(x))
	// The boundary block at height 2: r2 proposes, r1+x attest, so apply() sees the
	// third distinct attester (r1) → the maturity latch trips and rotateEpoch(2) runs
	// the #506 tally over the frozen set's regVersion. gateHeight would be 2+2=4.
	boundary := func() Block {
		b := &Block{Version: BlockVersionRounds, Height: 2, Prev: ports.Hash{},
			Entries: []ports.Entry{entry(2)}}
		Sign(b, r2)
		b.Atts = []Attestation{Attest(b, r1), Attest(b, x)}
		return *b
	}
	// A within-R re-reg for x at height 5 (past the would-be gateHeight 4; 5-1=4 < R).
	reg := &Block{Version: BlockVersionRounds, Height: 5, Prev: ports.Hash{},
		BondRegs: []BondReg{bondRegFull(x, rootX, twoMiB, ports.Hash{}, BlockVersionRegGate, 0)}}
	return probe{
		name: "regVersion carries the #506 readiness super-quorum: applying the boundary must " +
			"lock the gate so a within-R reg is rejected; a snapshot that lost regVersion tallies " +
			"zero ready weight, never locks, and accepts the reg (ErrRegGate → accept)",
		detect:  []string{"regVersion"},
		mutates: true, // it apply()s the boundary block; runs against throwaway replicas
		ask: func(c *Chain) string {
			c.apply(boundary())
			return regVerdict(c, reg)
		},
	}
}

// domainWorld (bondDomain). bondDomain is NOT metric-only: it feeds matureNow()
// through the A-axis Nakamoto coefficient (C2Metric → MatureCoefficient), and
// matureNow() gates the maturity LATCH (chain.go:2893) and thereby the launch-anchor
// shed. The world latches nothing yet (everMature false), holds four anchors plus six
// equal real bonds, and its built bondDomain merges every real bond into ONE declared
// domain — so with domains carried, matureNow() is FALSE (one address-diverse group),
// but with bondDomain DROPPED the bonds count as independent groups and matureNow()
// rises TRUE. The probe applies a block (a mutating probe): full stays immature so the
// anchors keep eligibility and an anchor-only commit is ACCEPTED; a bondDomain-dropped
// replica matures, latches everMature, sheds the anchors, and REJECTS the same commit.
func domainWorld(t *testing.T) (*Chain, *Block, Block) {
	t.Helper()
	anchorKeys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range anchorKeys {
		anchorKeys[i] = key(int64(44000 + i))
		anchors[idOf(anchorKeys[i])] = true
	}
	realKeys := make([]ed25519.PrivateKey, 6)
	for i := range realKeys {
		realKeys[i] = key(int64(44100 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, anchorKeys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("domainWorld genesis: %v", err)
	}
	// Bake the regime a snapshot would carry: six equal real bonds, all SEEN, all in
	// one shared declared domain, everMature not yet latched.
	bonded := map[ports.NodeID]int64{}
	domain := map[ports.NodeID]uint64{}
	seen := map[ports.NodeID]bool{}
	for _, k := range realKeys {
		bonded[idOf(k)] = 2 << 20
		domain[idOf(k)] = 0x99 // one shared domain → coefficient capped at one group
		seen[idOf(k)] = true
	}
	setField(c, "bonded", bonded)
	setField(c, "bondDomain", domain)
	setField(c, "validatorsSeen", seen)

	// The anchor-only probe block: proposer + two anchor attesters (clears the
	// count floor while the anchors are eligible), zero real bond behind it.
	pb := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(9)}}
	Sign(pb, anchorKeys[0])
	pb.Atts = []Attestation{Attest(pb, anchorKeys[1]), Attest(pb, anchorKeys[2])}

	// The apply-block that re-evaluates Mature() (a trivial committed block).
	trigger := Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(5)}}
	return c, pb, trigger
}

func domainProbe(anchorBlock *Block, trigger Block) probe {
	return probe{
		name: "declared bondDomain is load-bearing via the maturity latch: with domains merged the " +
			"network stays immature and an anchor-only commit is accepted; a snapshot that lost " +
			"bondDomain counts the bonds as independent, matures, sheds the anchors, and rejects it",
		detect:  []string{"bondDomain"},
		mutates: true, // it apply()s the trigger block; runs against throwaway replicas
		ask: func(c *Chain) string {
			c.apply(trigger)
			return quorumVerdict(c, anchorBlock)
		},
	}
}

// weightBytesWorld is the era-3 freeze gate (#603): a mature epoch whose FROZEN
// per-member weights are UNEQUAL, and a block whose support coalition clears the
// COUNT floor but whose verdict is carried by the ⅔-WEIGHT predicate. It is the
// discriminator the membership probes cannot reach.
//
// Four members freeze into epochSet at genesis; the bond size IS the frozen weight
// (liveQualifiedSet → rotateEpoch). Weights are UNEQUAL by design: proposer keys[0]
// and one attester keys[1] hold 5 MiB each; the two silent members keys[2]/keys[3]
// hold 1 MiB each. total = 12 MiB, support = proposer+attester = 10 MiB, so
// 3·10 > 2·12 — the ⅔-weight predicate PASSES on the true weights (ACCEPT). Quorum=1,
// seen={keys[1]} clears the count floor honestly (RequiredQuorum returns Quorum in a
// mature epoch, chain.go:1204 — the weight rule carries the Byzantine bar), so the
// verdict is carried by requireEpochWeightQuorum, not the count floor.
//
// The ablation is NOT map-omission (that empties membership → the count-floor flip the
// membership probes already own). It FLATTENS the weight bytes to a constant, membership
// intact: support/total collapses to |coalition|/|members| = 2/4 = ½ for ANY constant, so
// 3·support ≤ 2·total → ErrNoQuorumWeight. A validator that lost the true per-member
// weights and knew only membership cannot reproduce the ⅔ verdict — the weight bytes
// proven load-bearing. See docs/thinking/2026-08-27-keystone-weight-discriminator-probe.md.
func weightBytesWorld(t *testing.T) (*Chain, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = key(int64(33000 + i))
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Unequal frozen weights: the coalition (proposer keys[0] + attester keys[1]) holds
	// 5+5 MiB; the silent members keys[2]/keys[3] hold 1 MiB each. All ≥ MinBond so all
	// freeze into epochSet. The bond amount becomes the frozen weight.
	weights := []int64{5 << 20, 5 << 20, 1 << 20, 1 << 20}
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, weights[i], ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("weightBytesWorld genesis: %v", err)
	}

	// The probed block: proposer keys[0] (5 MiB), a single attester keys[1] (5 MiB).
	// support = 10 MiB of 12 MiB total → 3·10=30 > 2·12=24 → weight predicate ACCEPTS.
	// The silent whales keys[2]/keys[3] do NOT attest — the coalition carries ⅔ only
	// because its TRUE weight is concentrated, not by head count.
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(33)}}
	Sign(b, keys[0])
	b.Atts = []Attestation{Attest(b, keys[1])}
	return c, b
}

// flattenWeights returns a copy of epochSet with identical membership but every weight
// set to the same constant — the "blinded weight bytes" ablation. This is the defect a
// snapshot-booted node exhibits if it carried WHICH members are frozen but not HOW MUCH
// each is bonded: it can reproduce membership but not the ⅔-weight verdict.
func flattenWeights(src map[ports.NodeID]int64, k int64) map[ports.NodeID]int64 {
	out := make(map[ports.NodeID]int64, len(src))
	for id := range src {
		out[id] = k
	}
	return out
}

// TestEpochWeightBytesAreLoadBearing is the era-3 freeze gate (#603): the committed
// per-member WEIGHT bytes of epochSet — not merely its membership — must flip a finality
// verdict. The membership probes (#604) prove omission empties frozen membership and
// rejects via the COUNT floor (ErrNoQuorum); they would still pass if epochSet stored
// membership with all weights set to a constant (blind PE ruling, "Coupling", L104). This
// probe closes that: with membership held fixed and the weights flattened to a constant,
// the verdict must flip via the WEIGHT predicate (ErrNoQuorumWeight). A validator that
// committed the true weights accepts; one that lost them (flattened) rejects a block the
// network finalized — the weight bytes are load-bearing in the committed root.
func TestEpochWeightBytesAreLoadBearing(t *testing.T) {
	c, b := weightBytesWorld(t)

	// Full: the true unequal weights. The coalition holds >⅔ by real weight → ACCEPT.
	full := snapshotBoot(c)
	if got := quorumVerdict(full, b); got != "accept" {
		t.Fatalf("full (true weights): want accept, got %s — the coalition should clear "+
			"the ⅔-weight predicate on its real weight", got)
	}

	// Ablated: membership intact, weights flattened to a constant (MinBond). support/total
	// collapses to 2/4 = ½ < ⅔ → the WEIGHT predicate rejects. The count floor is
	// untouched (same membership → same seen), so this is unambiguously the weight rule.
	ablated := snapshotBoot(c)
	setField(ablated, "epochSet", flattenWeights(c.epochSet, c.cfg.MinBond))

	// Assert the RED reason is the weight predicate specifically, not the count floor or a
	// panic. quorumVerdict collapses errors to "reject"; call the predicate path directly
	// so the discriminator is named.
	seen, err := ablated.collectQuorumSigs(b, b.Atts, PhaseLegacy, 0)
	if err != nil {
		t.Fatalf("ablated collectQuorumSigs errored (%v) — membership must survive the "+
			"weight-flatten ablation; only the weight bytes change", err)
	}
	stackErr := ablated.requireQuorumStack(b, seen)
	if !errors.Is(stackErr, ErrNoQuorumWeight) {
		t.Fatalf("ablated: want ErrNoQuorumWeight (the weight predicate is the discriminator), "+
			"got %v (len(seen)=%d). If this is ErrNoQuorum the flip is membership/count, not "+
			"the weight bytes — the gap this probe exists to close.", stackErr, len(seen))
	}
	t.Logf("[weight-bytes] epochSet weights: full=accept ablated=reject via %v "+
		"(seen=%d clears the count floor; the ⅔-weight predicate is the discriminator)",
		ErrNoQuorumWeight, len(seen))
}

// TestSnapshotBootMatchesReplayBoot is the equivalence assertion: with the FULL
// committed set restored, a never-replayed replica must answer every probe
// exactly as the replayed one does.
func TestSnapshotBootMatchesReplayBoot(t *testing.T) {
	replayed, keys, revokedRoot, ownedRoot := richHistory(t)
	_, head := replayed.Head()
	prev := replayed.Blocks(0)[head-1].Hash()

	all := probes(revokedRoot, ownedRoot, keys, prev)

	// The consensus-weight probes each run against their own mature/objective world.
	weightC, epochSetBlock := weightWorld(t)
	bondedC, bondedBlock := bondedWorld(t)
	epochSetPs, bondedPs := weightProbes(epochSetBlock, bondedBlock)

	// The set-valued spent/slashed probes each run against their own world.
	spentC, spentSerial, spentMint := spentWorld(t)
	slashedC, slashedCulprit := slashedWorld(t)

	// The latch/gate/domain tranche. The mutating probes (regVersion/bondDomain apply
	// a block) are skipped by check() — leave-one-out covers them; the read-only
	// everMature/matureEpoch/gate probes must answer identically on either boot.
	deMatureC, deMatureBlock := deMatureWorld(t)
	matureEpochC, matureEpochBlock := matureEpochWorld(t)
	gateC := gateWorld(t)
	lockedInProbe, heightProbe := gateProbes(gateC)

	check := func(replayed *Chain, ps []probe) {
		snap := snapshotBoot(replayed)
		for _, p := range ps {
			if p.mutates {
				// Would mutate the replayed chain; leave-one-out covers these
				// against throwaway replicas instead.
				continue
			}
			want := askSafely(p, replayed)
			got := askSafely(p, snap)
			if want != got {
				t.Errorf("DIVERGENCE on %q: replay-booted says %q, snapshot-booted says %q\n"+
					"A snapshot-booted validator reaches a different verdict than a "+
					"replayed one — the committed set is INSUFFICIENT, which is the "+
					"unsoundness the state root exists to prevent.", p.name, want, got)
			}
		}
	}
	check(replayed, all)
	check(weightC, epochSetPs)
	check(bondedC, bondedPs)
	check(spentC, []probe{spentProbe(spentSerial, spentMint)})
	check(slashedC, []probe{slashedProbe(slashedCulprit)})
	check(deMatureC, []probe{everMatureProbe(deMatureBlock)})
	check(matureEpochC, []probe{matureEpochProbe(matureEpochBlock)})
	check(gateC, []probe{lockedInProbe, heightProbe})
}

// probeUncovered names committed fields for which no probe yet exists. It is a
// DECLARED, SHRINKING debt, asserted below so it cannot grow silently: a field
// nobody probes is a field whose necessity is unproven.
//
// Each entry says what a probe would have to construct — these are honest gaps,
// not fields believed irrelevant.
var probeUncovered = map[string]string{
	"bondRegHeight": "the min-interval rule is gated behind regGateActive (#506); this " +
		"world has no RegGateActivationHeight, so the rule never fires — needs a " +
		"gate-active world",
	"validatorsSeen": "read by Mature/C2Metric in legacy mode only",
	// everMature/matureEpoch/regVersion/gateLockedIn/gateHeight/bondDomain were moved
	// to real leave-one-out probes on 2026-08-28 (the de-mature/mature-epoch-flag/
	// gate-lock/gate-tally/domain-latch worlds). See docs/thinking/
	// 2026-08-28-keystone-leaveoneout-latch-gate-domain.md. Notably bondDomain is NOT
	// metric-only: it feeds matureNow() → the maturity latch → the launch-anchor shed,
	// a real verdict (domainWorld/domainProbe), overturning the prior "metric" excuse.
}

// TestLeaveOneOutProvesEachFieldLoadBearing is the sharp half. For every
// committed field with a probe, omitting it from the snapshot MUST change a
// verdict. A field that can be dropped with no observable effect is a finding
// either way — either it does not belong in the committed set (bloat on a
// forever-growing term), or the probe is not adversarial enough. Per the
// consensus-correctness discipline, an oracle that sees something it cannot
// explain FLAGS; it never assumes-benign.
func TestLeaveOneOutProvesEachFieldLoadBearing(t *testing.T) {
	replayed, keys, revokedRoot, ownedRoot := richHistory(t)
	_, head := replayed.Head()
	prev := replayed.Blocks(0)[head-1].Hash()

	// Three worlds: the launch richHistory world for the set-valued probes, plus a
	// mature-epoch world (epochSet) and an anchorless objective world (bonded) for
	// the consensus-weight fields, which are load-bearing only in those regimes.
	weightC, epochSetBlock := weightWorld(t)
	bondedC, bondedBlock := bondedWorld(t)
	epochSetPs, bondedPs := weightProbes(epochSetBlock, bondedBlock)
	spentC, spentSerial, spentMint := spentWorld(t)
	slashedC, slashedCulprit := slashedWorld(t)

	// The latch/gate/domain tranche (2026-08-28): each field's leave-one-out flip
	// lives outside the count path, so each gets the world where it is load-bearing.
	deMatureC, deMatureBlock := deMatureWorld(t)
	matureEpochC, matureEpochBlock := matureEpochWorld(t)
	gateC := gateWorld(t)
	lockedInProbe, heightProbe := gateProbes(gateC)
	regVersionC := regVersionWorld(t)
	domainC, domainAnchorBlock, domainTrigger := domainWorld(t)

	worlds := []worldGroup{
		{"launch", func(*testing.T) *Chain { return replayed }, probes(revokedRoot, ownedRoot, keys, prev)},
		{"mature-epoch", func(*testing.T) *Chain { return weightC }, epochSetPs},
		{"objective-bonded", func(*testing.T) *Chain { return bondedC }, bondedPs},
		{"token-spent", func(*testing.T) *Chain { return spentC }, []probe{spentProbe(spentSerial, spentMint)}},
		{"slashed-anchor", func(*testing.T) *Chain { return slashedC }, []probe{slashedProbe(slashedCulprit)}},
		{"de-mature", func(*testing.T) *Chain { return deMatureC }, []probe{everMatureProbe(deMatureBlock)}},
		{"mature-epoch-flag", func(*testing.T) *Chain { return matureEpochC }, []probe{matureEpochProbe(matureEpochBlock)}},
		{"gate-lock", func(*testing.T) *Chain { return gateC }, []probe{lockedInProbe, heightProbe}},
		{"gate-tally", func(*testing.T) *Chain { return regVersionC }, []probe{regVersionProbe()}},
		{"domain-latch", func(*testing.T) *Chain { return domainC }, []probe{domainProbe(domainAnchorBlock, domainTrigger)}},
	}

	// Guard the declared debt against the live struct first: a newly added
	// committed field must be probed (in SOME world) or explicitly recorded as
	// unprobed.
	covered := map[string]bool{}
	for _, w := range worlds {
		for _, p := range w.probes {
			for _, f := range p.detect {
				covered[f] = true
			}
		}
	}
	for _, name := range fieldsOfKind(committedSet) {
		if covered[name] {
			if _, dup := probeUncovered[name]; dup {
				t.Errorf("%q is both probed and listed in probeUncovered — remove the stale entry", name)
			}
			continue
		}
		if _, ok := probeUncovered[name]; !ok {
			t.Errorf("committed field %q has no probe and is not declared in probeUncovered.\n"+
				"Its necessity is unproven: add a probe that exposes its loss, or "+
				"record what a probe would have to construct. Do not leave it silent.", name)
		}
	}

	// The ablation itself: for each world, drop one committed field that world's
	// probes cover and expect a changed verdict. A field is proven load-bearing
	// once ANY world's probe flips on its omission.
	flipped := map[string]bool{}
	for _, w := range worlds {
		wcov := map[string]bool{}
		for _, p := range w.probes {
			for _, f := range p.detect {
				wcov[f] = true
			}
		}
		src := w.build(t)
		for _, name := range fieldsOfKind(committedSet) {
			if !wcov[name] {
				continue
			}
			ablated := snapshotBoot(src, name)

			// Baseline is a FULL snapshot replica, not the replayed chain: apply-time
			// probes mutate, so both sides must be throwaways for a fair comparison.
			full := snapshotBoot(src)
			for _, p := range w.probes {
				fv, av := askSafely(p, full), askSafely(p, ablated)
				if fv != av {
					// Logged so the pass is evidence, not an assertion: CI shows
					// which world+probe caught which field, and a field that starts
					// "diverging" only via an unrelated panic is visible here.
					t.Logf("[%s] omitting %-16s → probe %q: full=%s ablated=%s", w.name, name, p.name, fv, av)
					flipped[name] = true
					break
				}
				// Rebuild after a mutating probe so later probes see clean state.
				if p.mutates {
					full = snapshotBoot(src)
					ablated = snapshotBoot(src, name)
				}
			}
		}
	}
	for name := range covered {
		if !flipped[name] {
			t.Errorf("omitting committed field %q changed NO verdict in any world.\n"+
				"Either the field is not actually load-bearing (so committing it "+
				"is bloat on the snapshot — revisit the Q2 enumeration and the Q3 "+
				"growth analysis), or the probes are not adversarial enough. This "+
				"is a finding to route, not a test to relax.", name)
		}
	}
}
