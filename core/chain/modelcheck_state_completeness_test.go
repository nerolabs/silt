package chain

import (
	"reflect"
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
	// injected: supplied at construction or by a setter; identical on every
	// replica by configuration, never derived from history.
	injected
	// transient: scoped to a single operation and never accumulated.
	transient
)

// stateClass classifies EVERY field of Chain. The reason string is not a
// comment — for anything not `committed` it is the claim being made, and the
// consensus-correctness discipline's rule 6 applies: every field you drop is a
// claim you can prove you don't need it.
var stateClass = map[string]struct {
	kind   stateKind
	reason string
}{
	// ---- committed: the certification's enumerated 16 ----
	"byRoot":         {committedSet, "cert field 1 — ValidateEntry dup-reject, validateTakedowns existence"},
	"spent":          {committedSet, "cert field 2 — ValidateEntry replay-reject (#183 F-1 order)"},
	"revoked":        {committedSet, "cert field 3 — validateTakedowns unrevocation target"},
	"slashed":        {committedSet, "cert field 4 — qualification, quorum N, C2, de-mature super-quorum"},
	"bonded":         {committedSet, "cert field 5 — qualification, blockWeight fork-choice, RoundCatchupMet"},
	"epochSet":       {committedSet, "cert field 6 — frozen-set membership, validatorSetSize, weight quorum (#357 Cond A)"},
	"bondRootOwner":  {committedSet, "cert field 7 — apply first-owner-wins dedup (F1)"},
	"bondRootProven": {committedSet, "cert field 8 — apply displacement rule (G3)"},
	"bondRegHeight":  {committedSet, "cert field 9 — bond TTL clock, #506 R-rule distance"},
	"regVersion":     {committedSet, "cert field 10 — #506 rotateEpoch lock-in tally"},
	"bondDomain":     {committedSet, "cert field 11 — C2Metric A-axis"},
	"validatorsSeen": {committedSet, "cert field 12 — Mature/C2Metric (legacy mode)"},
	"gateLockedIn":   {committedSet, "cert field 13a — #506 activation latch"},
	"gateHeight":     {committedSet, "cert field 13b — #506 enforcement boundary H_act"},
	"everMature":     {committedSet, "cert field 14 — one-way maturity latch (F-1)"},
	"matureEpoch":    {committedSet, "cert field 15 — handoff flag (#357 Cond B)"},
	"era3LockedIn":   {committedSet, "step 2c — era-3 (v4) activation latch (mirrors gateLockedIn, regVersion>=4)"},
	"era3Height":     {committedSet, "step 2c — era-3 (v4) enforcement boundary H_era3 (mirrors gateHeight)"},

	// ---- committed: NOT in the certification's enumeration (findings) ----
	"revLog": {committedLog, "CERTIFIED #597: an ordered CT-style transparency log, " +
		"NOT set-valued state. Its root is the RFC-6962 MTH over an ORDERED slice " +
		"(translog.go:54/:106), so it is history-DEPENDENT by design. It gets its own " +
		"append-only root and must NEVER become a leaf in the history-independent SMT " +
		"— that category error would make the state root order-dependent. The snapshot " +
		"carries the full entry list so a snapshot-booted node can extend the log and " +
		"still serve H9 inclusion/consistency proofs."},
	// ---- observable: history-derived, under no committed root ----
	"epochStart": {observable, "CERTIFIED #597 Q4.3: rotateEpoch writes it " +
		"(chain.go:2844) and adopt() swaps it, but its ONLY reader is Regime() — the " +
		"permanent save/restore health instrumentation, not a block-validity predicate. " +
		"So it must survive a reorg, but goes under no committed root: losing it " +
		"misreports restore health, never validity."},

	// ---- not committed ----
	"blocks": {input, "the committed history itself; committed state is derived FROM this"},
	"cfg":    {injected, "genesis/operator configuration, identical on every replica by config"},
	"rep": {injected, "local reputation view — the certification excludes it explicitly: " +
		"local and divergent by design"},
	"tokenQuorum": {injected, "set by SetTokenQuorum, not derived from history"},
	"issuerKey":   {injected, "injected callback"},
	"verifyBond":  {injected, "injected bond verifier"},
	"trustFloorOverride": {transient, "set only on the Reconcile scratch chain " +
		"(chain.go:3120) and read within that replay (retention.go:136); never " +
		"accumulated across blocks, and deliberately NOT copied by adopt"},
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
	c.everMature = true
	c.matureEpoch = true
	c.epochStart = 17

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
			"(chain.go:3172) or reclassify them with the claim that they are "+
			"not derived from history.", len(missed), missed)
	}
}
