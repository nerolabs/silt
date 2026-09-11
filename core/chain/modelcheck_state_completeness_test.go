package chain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// RED home #1 (part 1) — state-field completeness, proven mechanically rather
// than by inspection.
//
// The state-root keystone certification makes one obligation load-bearing:
//
//	The completeness of the 16-field enumeration … must be proven by the
//	snapshot-boot-equivalence oracle, NOT by inspection (inspection already
//	missed fields — the PE's list was a subset). … Treat any field discovered
//	later by that oracle as a soundness bug, not an optimization.
//
// The tempting oracle — capture the enumerated fields, restore them, compare
// them — is inspection wearing a test costume: it can only ever test the list
// it was handed, so field #17 lands green and silent. That is the #558
// silent-divergence class exactly.
//
// So this file does not compare a list. It cross-binds THREE independent
// enumerations with reflection over the real struct, and fails if any two
// disagree:
//
//  1. `stateClass` below — every field of Chain, classified.
//  2. `populateCommitted` — assigns a distinctive value to each committed field.
//  3. `adopt` (PRODUCT CODE, chain.go) — the reorg path's state swap.
//
// A field added to Chain fails (1) until classified; once classified committed
// it fails (2) until populated; then it fails (3) until `adopt` copies it. You
// cannot add accumulated state and stay green by accident, which is the whole
// point.
//
// WHY THIS ALREADY MATTERS: `adopt` is a hand-maintained copy list on the reorg
// path that copies 19 fields, while the certification's enumeration names 16.
// They disagree about `revLog` and `epochStart`. See the findings on
// TestStateFieldsAreClassified.

// stateKind is why a Chain field does or does not belong in committed state.
type stateKind int

const (
	// committedSet: set-valued validity state, under the history-INDEPENDENT
	// state SMT. Must be reconstructible from a set-valued snapshot, so its
	// value may NEVER depend on the ORDER of history — asserted by
	// TestCommittedSetFieldsAreOrderIndependent.
	committedSet stateKind = iota
	// committedLog: an ordered, append-only log. Gets its OWN append-only
	// (RFC-6962) root, NEVER a leaf in the SMT — folding an order-derived
	// value into the state root is the category error #597 identified.
	committedLog
	// observable: derived from block history and swapped on reorg, but read by
	// NO block-validity predicate, so it sits under no committed root. Losing
	// it misreports health, never validity.
	observable
	// input: the block history itself — the source the committed state is
	// derived FROM, not derived state.
	input
	// injected: supplied at construction or by a setter, never derived from
	// history. That is the WHOLE claim, and it is the only one this class is
	// entitled to make.
	//
	// WHAT THIS CLASS USED TO SAY, AND WHY IT NO LONGER SAYS IT. The premise was
	// "identical on every replica by configuration". Re-derived 2026-09-11: that
	// holds for NONE of the five members unconditionally, and is materially false
	// for three of them. A class doc is read as a guarantee, so a premise that is
	// false for most of its members is worse than no premise.
	//
	//   - cfg     MIXED. 16 of the 21 chain.Config fields are genesis-covered via
	//             ConsensusParams, so those ARE uniform (a divergent node computes a
	//             different genesis hash and cannot join). FIVE are deliberately
	//             per-node — the ratified exclusions in configMemberships
	//             (consensus_config_divergence_test.go). WSCheckpoint MUST differ
	//             per node or it is not an independent anchor at all.
	//   - rep     FALSE by design, and this row's own reason already said so: the
	//             local reputation view is local and divergent.
	//   - issuerKey  FALSE. It is wired to (*node.Node).IssuerKeyOf, which returns
	//             n.peerIssuerKeys[v] — a PEER-POPULATED cache — and ValidateEntry
	//             reads it through publishtoken.Verify on the block-validity path.
	//             NOT FIXED HERE and NOT this file's to fix; recorded so the class
	//             doc stops asserting the opposite of what the wiring does.
	//   - tokenQuorum  UNBOUND. Nothing in silt makes it uniform; see its row.
	//   - verifyBond   Uniform only in the sense that matters: its two divergent
	//             INPUTS (BondLabelSamples, BondVDFDelay) are genesis-covered in
	//             ConsensusParams, and core/node's own divergence gate claims them.
	//
	// So: injected means injected. Any uniformity claim belongs on the ROW, with
	// the mechanism that delivers it named.
	injected
	// transient: scoped to a single operation and never accumulated.
	transient
)

// stateClass classifies EVERY field of Chain. The reason string is not a
// comment — for anything not `committed` it is the claim being made, and the
// consensus-correctness discipline's rule 6 applies: every field you drop is a
// claim you can prove you don't need it.
//
// THE BACKTICK CONVENTION, and it is MACHINE-CHECKED. A reason string names code
// in `backticks`, and TestStateClassReasonSymbolsResolve resolves every backticked
// token against the real declarations of package chain. Prose stays free; only
// backticks make a claim about a symbol.
//
// WHY: on 2026-09-11 this table's tokenQuorum row read "set by SetTokenQuorum" and
// SetTokenQuorum existed NOWHERE in the repo — the setter is RequireTokens. Nothing
// read the sentence, so it stayed green through every audit. That is the same class
// scripts/check_cited_tests.py exists for, one layer down: a citation that names a
// symbol decays exactly like a citation that names a test.
//
// DO NOT BACKTICK a file path, a package-qualified name, or a cross-package symbol
// — the gate resolves against package chain only, and says so when it fails.
var stateClass = map[string]struct {
	kind   stateKind
	reason string
}{
	// ---- committed: the certification's enumerated 16 ----
	"byRoot":         {committedSet, "cert field 1 — `ValidateEntry` dup-reject, `validateTakedowns` existence"},
	"spent":          {committedSet, "cert field 2 — `ValidateEntry` replay-reject (#183 F-1 order)"},
	"revoked":        {committedSet, "cert field 3 — `validateTakedowns` unrevocation target"},
	"slashed":        {committedSet, "cert field 4 — qualification, quorum N, C2, de-mature super-quorum"},
	"bonded":         {committedSet, "cert field 5 — qualification, `RoundCatchupMet` (the fork-choice weight read is retired, O3 Direction T)"},
	"epochSet":       {committedSet, "cert field 6 — frozen-set membership, `validatorSetSize`, weight quorum (#357 Cond A)"},
	"bondRootOwner":  {committedSet, "cert field 7 — `apply` first-owner-wins dedup (F1)"},
	"bondRootProven": {committedSet, "cert field 8 — `apply` displacement rule (G3)"},
	"bondRegHeight":  {committedSet, "cert field 9 — bond TTL clock, #506 R-rule distance"},
	"regVersion":     {committedSet, "cert field 10 — #506 `rotateEpoch` lock-in tally"},
	"bondDomain":     {committedSet, "cert field 11 — `C2Metric` A-axis"},
	"validatorsSeen": {committedSet, "cert field 12 — `Mature`/`C2Metric` (legacy mode)"},
	"gateLockedIn":   {committedSet, "cert field 13a — #506 activation latch"},
	"gateHeight":     {committedSet, "cert field 13b — #506 enforcement boundary H_act"},
	"everMature":     {committedSet, "cert field 14 — one-way maturity latch (F-1)"},
	"matureEpoch":    {committedSet, "cert field 15 — handoff flag (#357 Cond B)"},
	"era3LockedIn":   {committedSet, "step 2c — era-3 (v4) activation latch (mirrors gateLockedIn, regVersion>=4)"},
	"era3Height":     {committedSet, "step 2c — era-3 (v4) enforcement boundary H_era3 (mirrors gateHeight)"},
	"era4LockedIn": {committedSet, "step 4d — era-4 (v5) activation latch (mirrors `era3LockedIn`, " +
		"`regVersion`>=5). v5-ONLY scalar leaf (`tagEra4LockedIn`): committing it in the era-3 " +
		"leaf set would break the byte-identical freeze. Before activation it is zero on v4 " +
		"blocks; `era4Active` first fires at a v5 height, where it IS committed."},
	"era4Height": {committedSet, "step 4d — era-4 (v5) enforcement boundary H_era4 (mirrors " +
		"`era3Height`). v5-ONLY scalar leaf (`tagEra4Height`)."},

	// ---- committed: era-4 (v5) maintenance spine (step 4b). Committed under the
	// state root as v5-ONLY leaves — on a v4 block the marshaller emits no leaf for
	// these, so the era-3 root stays byte-identical. They are set-valued derived
	// state, so they classify committedSet: adopt swaps them, the dry-run clone copies
	// them, and the v5 marshaller commits them. ----
	"qualified": {committedSet, "era-4 4b (E-2) — live materialization of `liveQualifiedSet`; " +
		"the boundary-computation accelerator `rotateEpoch` copies into the frozen `epochSet`. " +
		"v5-only leaf (`tagQualified`)."},
	"dueBucket": {committedSet, "era-4 4b (T-3) — due-height index; one bucket per occupied " +
		"expiry height, committed as an MTH over the canonical id list. v5-only leaf (`tagDueBucket`)."},
	"issuerKeyCommit": {committedSet, "R0.4b — the consensus-attested per-epoch demand-issuer " +
		"key binding (epoch -> issuer -> key fingerprint). Committed so every honest node " +
		"agrees on key_E and an off-commitment (targeted, per-cohort) key is rejectable by " +
		"construction — the anti-fingerprinting binding the R0.4b certification makes " +
		"MANDATORY (Verdict 2; the lighter pinned-keyset is REFUTED as sufficient). It is " +
		"set-valued derived state: `apply` writes it, `adopt` swaps it, the dry-run clone copies " +
		"it, and the v5 marshaller commits it. INERT to consensus — no validity predicate, " +
		"quorum, fork-choice rule, or floor-box recompute reads it; the only reader is the " +
		"demand-lane redeemer, out of band. v5-only leaf (`tagIssuerKey`)."},

	// RE-DERIVED 2026-09-11. The load-bearing clause — "no quorum/validity predicate reads
	// it" — HOLDS at source. The parenthetical that followed it, "its only reader is
	// Regime()", did NOT: the O-1 promotion this same sentence describes ADDED readers
	// (statehash.go's leaf emitter, readset_v5.go's accumulator, liveView's scalar
	// accessor, the rotate fold). They are the commitment, not a predicate, so the claim
	// survives — but a reader checking the parenthetical would have found it false and had
	// no way to tell which half was load-bearing.
	"epochStart": {committedSet, "era-4 4b (O-1) — PROMOTED from observable to committed. " +
		"CERTIFIED narrowly (RECERT2): no quorum/validity predicate reads it (its only " +
		"NON-COMMITMENT reader is `Regime`; the commitment path reads it too, which is what " +
		"the promotion means), so committing it changes no quorum decision; it removes one " +
		"uncommitted observable and doubles as the E-2 epoch pointer. v5-only scalar leaf " +
		"(`tagEpochStart`)."},

	// ---- committed: NOT in the certification's enumeration (findings) ----
	// RE-DERIVED 2026-09-11. The line citation "translog.go:54/:106" was stale: :54 is
	// blank and :106 is a section header. The symbols are translog.Log.Root and
	// translog.MTH. They are CROSS-PACKAGE, so they stay out of backticks — the resolver
	// covers package chain only, and a backtick it cannot resolve is a false red.
	"revLog": {committedLog, "CERTIFIED #597: an ordered CT-style transparency log, " +
		"NOT set-valued state. Its root is the RFC-6962 MTH over an ORDERED slice " +
		"(translog.Log.Root -> translog.MTH), so it is history-DEPENDENT by design. It gets its own " +
		"append-only root and must NEVER become a leaf in the history-independent SMT " +
		"— that category error would make the state root order-dependent. The snapshot " +
		"carries the full entry list so a snapshot-booted node can extend the log and " +
		"still serve H9 inclusion/consistency proofs."},

	// ---- not committed ----
	"blocks": {input, "the committed history itself; committed state is derived FROM this"},

	// RE-DERIVED 2026-09-11. "identical on every replica by config" was the class doc's
	// blanket premise and it is false here: 16 of the 21 chain.Config fields are
	// genesis-covered, FIVE are per-node by ratified design, and one of the five MUST
	// differ per node. The uniformity claim now names its mechanism and its hole.
	"cfg": {injected, "genesis/operator configuration. UNIFORM only for the 16 fields " +
		"carried in ConsensusParams (genesis-hash-covered: a divergent node computes a " +
		"different genesis and cannot join). The five ratified exclusions are per-node by " +
		"design — see configMemberships in consensus_config_divergence_test.go. It reaches " +
		"validity verdicts and is NOT derived from history, which is why it is here."},
	"rep": {injected, "local reputation view — the certification excludes it explicitly: " +
		"local and divergent by design"},

	// RE-DERIVED 2026-09-11. This row read "set by SetTokenQuorum". SetTokenQuorum does
	// not exist and never did — grep the tree; the ONE hit is the sentence itself. The
	// setter is `RequireTokens`. The backtick gate below exists because of this row.
	"tokenQuorum": {injected, "set by `RequireTokens`, not derived from history. NOT " +
		"genesis-covered and NOT a chain.Config field, so the config-in-consensus gate's " +
		"reflection never sees it: nothing in silt makes it swarm-uniform. `ValidateEntry` " +
		"reads it, so this row records the shape rather than claiming uniformity it cannot " +
		"deliver."},

	// RE-DERIVED 2026-09-11. "injected callback" is true and is the whole of what this row
	// may claim. The class doc used to add "identical on every replica by configuration",
	// which is FALSE here and not marginally: the wiring is (*node.Node).IssuerKeyOf,
	// returning n.peerIssuerKeys[v]. NOT THIS FILE'S TO FIX — recorded, not repaired.
	"issuerKey": {injected, "injected callback, set by `RequireTokens` alongside " +
		"`tokenQuorum`. Wired in cmd/silt to (*node.Node).IssuerKeyOf, which answers from a " +
		"PEER-POPULATED cache (node.peerIssuerKeys), and `ValidateEntry` reads it on the " +
		"block-validity path. So it is injected AND divergent; this row states that rather " +
		"than inheriting a uniformity premise the wiring contradicts."},
	"verifyBond": {injected, "injected bond verifier, set by `SetBondVerifier`. Uniform in " +
		"the way that decides verdicts: its two divergent inputs (BondLabelSamples, " +
		"BondVDFDelay) are carried in ConsensusParams and claimed by core/node's " +
		"divergence gate."},

	// RE-DERIVED 2026-09-11. Both line citations were stale — chain.go:3120 is revlog leaf
	// bytes and retention.go:136 is a doc comment. Symbols instead, so a move cannot
	// silently invalidate them.
	"trustFloorOverride": {transient, "set only on the `Reconcile` scratch chain and read " +
		"within that replay by `trustFloor`; never accumulated across blocks, and " +
		"deliberately NOT copied by `adopt`"},
}

// TestStateFieldsAreClassified is the guard that catches the field nobody
// enumerated — the only failure mode a list-comparison oracle structurally
// cannot catch.
//
// FINDINGS ALREADY PRODUCED BY THIS GUARD (recorded here because the
// certification says to treat them as soundness questions, not optimizations):
//
//   - `revLog` and `epochStart` are accumulated by apply()/rotateEpoch() and
//     swapped by adopt(), but are absent from the certification's 16-field
//     enumeration. Product code and the certification disagree about what
//     "committed state" means, and the disagreement predates this test.
//
//   - `revLog` is HISTORY-DEPENDENT: a CT-style append-only log whose root is a
//     function of append ORDER, not of a key→value set. The certification's Q1
//     chose the SMT precisely because "the root is identical however the state
//     was reached." A snapshot-booted validator that never replayed cannot
//     rebuild revLog from set-valued state, so the keystone must either carry
//     the whole log in the snapshot or commit only its root and accept that a
//     snapshot-booted node cannot serve inclusion/consistency proofs. That is a
//     design question the certification does not answer.
func TestStateFieldsAreClassified(t *testing.T) {
	ct := reflect.TypeOf(Chain{})

	var unclassified []string
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		if _, ok := stateClass[name]; !ok {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		t.Fatalf("Chain has %d unclassified field(s): %v\n\n"+
			"A new field on Chain is a SOUNDNESS QUESTION, not a formality: if it "+
			"accumulates across blocks it must be classified `committed`, which "+
			"obliges adopt() to swap it, any state snapshot to carry it, and the "+
			"era-3 state root to commit it. If it does not accumulate, classify it "+
			"with the claim you are making about why. Do not delete this test to "+
			"go green.", len(unclassified), unclassified)
	}

	// The reverse direction: a classification entry for a field that no longer
	// exists is stale and would silently weaken the guards below.
	live := map[string]bool{}
	for i := 0; i < ct.NumField(); i++ {
		live[ct.Field(i).Name] = true
	}
	for name := range stateClass {
		if !live[name] {
			t.Errorf("stateClass names %q, which is not a field of Chain — stale entry", name)
		}
	}
}

// historyDerived returns every field derived from block history — set-valued
// state, the ordered log, AND observables. All three must survive a reorg, so
// all three are what adopt() owes. Only the first two go under a committed
// root, and they go under DIFFERENT roots (#597).
func historyDerived(t *testing.T) []string {
	t.Helper()
	ct := reflect.TypeOf(Chain{})
	var out []string
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		c, ok := stateClass[name]
		if ok && (c.kind == committedSet || c.kind == committedLog || c.kind == observable) {
			out = append(out, name)
		}
	}
	return out
}

// fieldValue reads an unexported field. reflect refuses .Interface() on
// unexported fields, and the alternative — hand-written per-field comparisons —
// would be a fourth enumeration to keep in sync, which is the exact failure
// this file exists to prevent. Confined to this test.
func fieldValue(c *Chain, name string) any {
	f := reflect.ValueOf(c).Elem().FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface()
}

// isZero reports whether a committed field is still at its zero/empty value.
func isZero(v any) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		return rv.Len() == 0
	case reflect.Ptr, reflect.Func, reflect.Interface:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}

// populateCommitted assigns a distinctive non-zero value to every committed
// field. It is an explicit enumeration on purpose — and
// TestAdoptCopiesEveryCommittedField proves it complete against the struct, so
// it cannot drift.
func populateCommitted(c *Chain) {
	id := ports.NodeID{9}
	root := ports.Hash{7}

	c.byRoot = map[ports.Hash]ports.Entry{root: {Root: root}}
	c.spent = map[string]bool{"serial-9": true}
	c.revoked = map[ports.Hash]bool{root: true}
	c.slashed = map[ports.NodeID]bool{id: true}
	c.bonded = map[ports.NodeID]int64{id: 1 << 21}
	c.epochSet = map[ports.NodeID]int64{id: 1 << 21}
	c.bondRootOwner = map[ports.Hash]ports.NodeID{root: id}
	c.bondRootProven = map[ports.Hash]bool{root: true}
	c.bondRegHeight = map[ports.NodeID]uint64{id: 11}
	c.regVersion = map[ports.NodeID]uint8{id: 3}
	c.bondDomain = map[ports.NodeID]uint64{id: 42}
	c.validatorsSeen = map[ports.NodeID]bool{id: true}
	c.gateLockedIn = true
	c.gateHeight = 13
	c.era3LockedIn = true
	c.era3Height = 21
	c.era4LockedIn = true
	c.era4Height = 29
	c.everMature = true
	c.matureEpoch = true
	c.epochStart = 17
	// era-4 (v5) maintenance spine.
	c.qualified = map[ports.NodeID]int64{id: 1 << 21}
	c.dueBucket = map[uint64]map[ports.NodeID]struct{}{43: {id: struct{}{}}}
	// R0.4b per-epoch demand-issuer key binding: epoch -> issuer -> fingerprint.
	c.issuerKeyCommit = map[uint64]map[ports.NodeID]ports.Hash{2: {id: ports.Hash{0xAB}}}

	rl := translog.New()
	rl.Append(RevocationLeaf(RevOp, root, 5))
	c.revLog = rl
}

// TestAdoptCopiesEveryCommittedField is the live-bug guard. `adopt` is product
// code on the REORG path: it replaces this replica's state with a reconciled
// fork's. A committed field it forgets survives the reorg with the losing
// fork's value — state from a chain this node no longer follows — which is a
// silent divergence between replicas, the #558 class.
//
// Reflection makes the guard total: it asserts over whatever the struct says is
// committed, not over a list written next to it.
func TestAdoptCopiesEveryCommittedField(t *testing.T) {
	fields := historyDerived(t)
	if len(fields) == 0 {
		t.Fatal("no history-derived fields found — the classification or reflection is broken")
	}

	// The winning fork, fully populated.
	winner := &Chain{}
	populateCommitted(winner)

	// First prove populateCommitted is complete: every field the struct says is
	// committed must actually have been set. This is what stops the enumeration
	// inside populateCommitted from drifting away from the classification.
	var unpopulated []string
	for _, name := range fields {
		if isZero(fieldValue(winner, name)) {
			unpopulated = append(unpopulated, name)
		}
	}
	if len(unpopulated) > 0 {
		t.Fatalf("populateCommitted left %d committed field(s) at zero: %v — "+
			"add them there, so the adopt guard below actually exercises them",
			len(unpopulated), unpopulated)
	}

	// The losing replica, empty: any field adopt fails to copy stays zero and
	// is therefore detectable.
	loser := &Chain{}
	loser.adopt(winner)

	var missed []string
	for _, name := range fields {
		got, want := fieldValue(loser, name), fieldValue(winner, name)
		if !reflect.DeepEqual(got, want) {
			missed = append(missed, name)
		}
	}
	if len(missed) > 0 {
		t.Fatalf("adopt() did not transfer %d committed field(s): %v\n\n"+
			"On a reorg this replica keeps the LOSING fork's value for those "+
			"fields while following the winning fork's blocks — a silent "+
			"replica divergence (the #558 class). Either copy them in adopt() "+
			"or reclassify them with the claim that they are "+
			"not derived from history.", len(missed), missed)
	}
}

// --- THE REASON-SYMBOL GATE ---------------------------------------------------------
//
// THE FAILURE CLASS: prose that describes code, drifts, and stays green because nothing
// reads it. On 2026-09-11 stateClass["tokenQuorum"] read "set by SetTokenQuorum". That
// symbol existed nowhere in the repo — the setter is RequireTokens — and the sentence had
// survived every audit of this file, because a reason string is a Go string literal and
// the compiler has no opinion about it.
//
// THE EXISTING MECHANISM, AND WHY IT DOES NOT REACH HERE. scripts/check_cited_tests.py
// already enforces this shape for one citation kind: a doc that names a test must name a
// test that exists. It works on prose FILES. These reason strings live inside a Go map in
// a _test.go file, so no doc lint sees them, and a Go lint has nothing to complain about.
// This is the same rule one layer down.
//
// WHY RESOLUTION AND NOT GREP. A grep for "SetTokenQuorum" across the tree matches a
// comment, a doc, a changelog entry, or the reason string itself — and the self-match is
// the killer: every stale symbol grep-resolves to the sentence that invented it. This
// parses the package's own non-test sources and collects what they DECLARE. A name that
// only ever appears inside a comment is not declared and does not resolve.
//
// THE SCOPE, STATED HONESTLY (simplicity rule 7, and the "gate nobody can keep green gets
// disabled" problem):
//
//   IN : backticked bare Go identifiers in stateClass reason strings, resolved against
//        the declarations of package chain.
//   OUT: prose (never checked), cross-package and package-qualified names (the resolver
//        cannot see them, so backticking one is a FALSE RED — the gate says so by name),
//        and every other table in the repo. It is deliberately NOT cross-bound to
//        configDecls: that table's reason strings are not yet fully re-derived, and a
//        merge-blocking gate over un-re-derived prose launders a stale claim into a
//        requirement. That cross-bind is the next leg, not this one.
//
// A NARROW GATE THAT HOLDS BEATS A BROAD ONE THAT GETS DISABLED. Adding a symbol to a
// reason string is opt-in: write it in backticks and it is checked forever; write it in
// prose and it is not. The convention is stated on stateClass itself.
//
// THE ABLATION BATTERY, RUN 2026-09-11 BEFORE THIS GATE WAS TRUSTED. Verified by EXIT
// CODE, and every patched file was `diff`ed against its pristine copy before either run
// was believed — a patch that silently fails to apply reports GREEN and is
// indistinguishable from a passing check (the scar behind this step).
//
//	A0 baseline ..................................................... GREEN  exit 0
//	A1 restore the real defect (`RequireTokens` -> `SetTokenQuorum`) . RED    exit 1
//	A2 revert ....................................................... GREEN  exit 0
//	A3 backtick a package-qualified name (`translog.MTH`) ........... RED    exit 1
//	A4 revert ....................................................... GREEN  exit 0
//
// The two RED arms are the two ways this gate can be wrong, and they fail DIFFERENTLY:
// A1 is a claim about a symbol that does not exist, A3 is a claim the resolver cannot
// see. Collapsing them into one message would tell a reader to fix the wrong thing.
//
// The in-test self-check is the third leg: the resolver must answer NO to
// `SetTokenQuorum` and YES to `RequireTokens` before it judges anything. A resolver that
// says yes to everything passes every reason string, which is the vacuous-gate shape the
// sessions-19/20 scars are about.

// reasonBacktick captures a `token` inside a reason string.
var reasonBacktick = regexp.MustCompile("`([^`]*)`")

// bareGoIdent is a single unqualified Go identifier — the only thing this gate resolves.
var bareGoIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// packageChainDeclarations parses every NON-TEST .go file of this package and returns the
// set of identifiers it declares: funcs and methods, types and their fields, consts and
// package-level vars. It is deliberately declaration-only — an identifier that appears
// solely in a comment or a string is absent, which is the whole difference from a grep.
//
// The parse runs against ".", which under `go test` is the package directory.
func packageChainDeclarations(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package chain: %v", err)
	}
	pkg, ok := pkgs["chain"]
	if !ok {
		t.Fatalf("package %q not found in the parse of %q — got %v; the resolver has nothing "+
			"to resolve against and would pass vacuously", "chain", ".", keysOf(pkgs))
	}
	decls := map[string]bool{}
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				decls[d.Name.Name] = true
			case *ast.TypeSpec:
				decls[d.Name.Name] = true
			case *ast.ValueSpec:
				for _, id := range d.Names {
					decls[id.Name] = true
				}
			case *ast.Field:
				for _, id := range d.Names {
					decls[id.Name] = true
				}
			}
			return true
		})
	}
	if len(decls) < 100 {
		t.Fatalf("package chain yielded only %d declarations — the parse is broken or ran in the "+
			"wrong directory, and a small declaration set makes this gate fail NOISILY rather "+
			"than pass vacuously", len(decls))
	}
	return decls
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestStateClassReasonSymbolsResolve is the gate. See the block comment above.
func TestStateClassReasonSymbolsResolve(t *testing.T) {
	decls := packageChainDeclarations(t)

	// Self-check: the resolver must actually be able to say NO. A resolver that returns
	// true for everything passes every reason string, which is the vacuous-gate shape.
	if decls["SetTokenQuorum"] {
		t.Fatal("the resolver claims `SetTokenQuorum` is declared in package chain. It is not — " +
			"that name is the defect this gate was built for. The declaration set is wrong.")
	}
	if !decls["RequireTokens"] {
		t.Fatal("the resolver cannot find `RequireTokens`, which IS declared in package chain " +
			"(chain.go). The parse is not reaching the package's real sources.")
	}

	var unresolved, notBare []string
	for field, c := range stateClass {
		for _, m := range reasonBacktick.FindAllStringSubmatch(c.reason, -1) {
			tok := m[1]
			if !bareGoIdent.MatchString(tok) {
				notBare = append(notBare, fmt.Sprintf("%s: `%s`", field, tok))
				continue
			}
			if !decls[tok] {
				unresolved = append(unresolved, fmt.Sprintf("%s: `%s`", field, tok))
			}
		}
	}
	sort.Strings(unresolved)
	sort.Strings(notBare)

	if len(notBare) > 0 {
		t.Errorf("%d backticked token(s) in stateClass reason strings are not bare Go identifiers: %v\n\n"+
			"This gate resolves against the declarations of package chain ONLY, so a file path, a\n"+
			"package-qualified name (translog.MTH) or a method expression cannot be resolved and\n"+
			"backticking one would be a FALSE RED. Write it in prose instead — prose is never\n"+
			"checked, and that is deliberate: the backtick IS the claim.", len(notBare), notBare)
	}
	if len(unresolved) > 0 {
		t.Errorf("%d backticked symbol(s) in stateClass reason strings do NOT resolve to a declaration "+
			"in package chain: %v\n\n"+
			"A reason string is the claim being made about a field, and a claim that names a symbol\n"+
			"is a claim that the symbol exists. This table shipped \"set by SetTokenQuorum\" for months\n"+
			"against a symbol that never existed; the setter is RequireTokens.\n\n"+
			"Fix the NAME, not this gate. If the symbol was renamed, use the new name. If it was\n"+
			"removed, the claim it supported is gone too — re-derive the reason rather than deleting\n"+
			"the backticks to go green.", len(unresolved), unresolved)
	}
}
