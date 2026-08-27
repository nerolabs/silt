package chain

import (
	"crypto/ed25519"
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

// snapshotBoot builds the "never replayed" replica: injected configuration
// carried over (identical on every replica by construction), every committed
// field copied EXCEPT those named in omit, and no block history at all.
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
		setField(dst, name, fieldValue(src, name))
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
}

// probeUncovered names committed fields for which no probe yet exists. It is a
// DECLARED, SHRINKING debt, asserted below so it cannot grow silently: a field
// nobody probes is a field whose necessity is unproven.
//
// Each entry says what a probe would have to construct — these are honest gaps,
// not fields believed irrelevant.
var probeUncovered = map[string]string{
	"spent": "needs tokenQuorum>0 and a blind-signed publish token to replay a serial",
	"bondRootProven": "the G3 displacement rule only fires when a DECLARED (genesis) owner " +
		"is displaced by a PROVEN claim; this world's owner is already proven, so the " +
		"branch is unreachable here — needs a genesis-declared bond root",
	"bondRegHeight": "the min-interval rule is gated behind regGateActive (#506); this " +
		"world has no RegGateActivationHeight, so the rule never fires — needs a " +
		"gate-active world",
	"slashed":        "needs a committed equivocation proof, then a block proposed by the slashed node",
	"regVersion":     "needs a rotateEpoch lock-in tally across a boundary (#506)",
	"bondDomain":     "read by C2Metric, which is a metric rather than a validity predicate",
	"validatorsSeen": "read by Mature/C2Metric in legacy mode only",
	"gateLockedIn":   "needs the #506 gate to lock in, then an R-rule-violating reg past H_act",
	"gateHeight":     "same as gateLockedIn",
	"everMature":     "needs the maturity latch to trip, then a launchAnchor/handoff-dependent check",
	"matureEpoch":    "needs the #357 Cond B handoff, then a regime-dependent quorum check",
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
	worlds := []worldGroup{
		{"launch", func(*testing.T) *Chain { return replayed }, probes(revokedRoot, ownedRoot, keys, prev)},
		{"mature-epoch", func(*testing.T) *Chain { return weightC }, epochSetPs},
		{"objective-bonded", func(*testing.T) *Chain { return bondedC }, bondedPs},
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
