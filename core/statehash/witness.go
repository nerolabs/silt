package statehash

import (
	"crypto/sha256"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
)

// Witness floor-box accessor — build increment R4-a (the three-valued spine).
//
// This is the SAFETY SPINE of the semi-stateless witness-validating floor box.
// A floor box holds the two committed roots (StateRoot / LogRoot), not the tree.
// To evaluate a validity predicate that reads a committed-set key K, it verifies
// a witness (an SMT membership or non-membership proof) for K against the
// committed StateRoot. This accessor is the ONE place that turns a supplied
// witness into a three-valued answer the predicate consumer acts on.
//
// The whole point of the type is a HARD CONSTRUCTION INVARIANT:
//
//	PROVEN_ABSENT is constructible ONLY from a verified non-membership proof
//	against the committed root.
//
// Every other path — no proof supplied, a proof that fails verification, an
// over-budget/malformed witness rejected upstream (R3), a fetch that failed
// (Delivery) — yields NO_WITNESS. A caller can NEVER read "I have no proof" as
// "the key is verifiably absent." That conflation is the single banned move in
// the C-7 certification (§104: "no witness supplied → accept"), the one
// implementation error that breaks the safe-degradation soundness proof. This
// type makes the banned move a compile-time impossibility, not a code-review
// catch: the Outcome enum has no public path to PROVEN_ABSENT except through
// Resolve verifying a non-membership proof.
//
// Certified by:
//   - C-7 (semi-stateless floor box soundness), §23/§56/§104 — safe degradation,
//     stall-not-accept, the banned move.
//   - RULING-witness-floor-box-mechanism-2026-08-29 (R4: three-valued, not
//     two-valued-plus-flag, not panic).
//
// SCOPE of this increment (R4-a): the accessor type, its construction invariant,
// and its unit ablation. It does NOT build the R3 per-block byte ceiling or the
// D-2 on-demand delivery. Those feed this accessor's NO_WITNESS arm; they are
// separate increments (RULING §"Couplings", §"Sequencing").

// Outcome is the three-valued result of resolving a committed-set key K against
// the committed StateRoot with a supplied witness. It is a closed set of exactly
// three states. It is deliberately NOT a (value, bool) pair: a bool false that
// doubles as both "absent" and "unknown" is the exact conflation this type
// exists to forbid (RULING R4, C-7 §104).
type Outcome uint8

const (
	// NoWitness is the zero value BY DESIGN. Any Result constructed without a
	// verified proof — the zero Result, a fetch failure, a rejected proof, an
	// R3 over-budget verdict — is NoWitness. The caller MUST stall (or re-fetch);
	// it must NEVER treat NoWitness as absent. Making it the zero value means the
	// safe state is the default: a forgotten or mis-constructed Result stalls,
	// never silently reads as a proven exclusion.
	NoWitness Outcome = iota

	// ProvenPresent means a valid MEMBERSHIP proof of K against the committed
	// root verified. The committed value is available via Result.Value.
	ProvenPresent

	// ProvenAbsent means a valid NON-MEMBERSHIP proof of K against the committed
	// root verified. K is absent from the ENTIRE committed keyspace (silt's
	// single root makes this a whole-set exclusion, not a per-shard one —
	// RULING R4 §"sharded-omission"). This is the ONLY outcome a caller may read
	// as "verifiably absent." It is constructible ONLY through Resolve verifying
	// a non-membership proof; there is no public constructor, factory, or literal
	// that yields it.
	ProvenAbsent
)

func (o Outcome) String() string {
	switch o {
	case ProvenPresent:
		return "PROVEN_PRESENT"
	case ProvenAbsent:
		return "PROVEN_ABSENT"
	default:
		return "NO_WITNESS"
	}
}

// Result is the outcome of resolving one key against the committed root. It is
// returned by value. The zero Result is {NoWitness, nil} — the safe default.
//
// The invariant this type guards: a Result whose Outcome() is ProvenAbsent can
// only have been produced by Resolve after smt.VerifyProof returned true for the
// non-membership claim. There is NO other code path in this package that sets
// the outcome field to ProvenAbsent, and the field is unexported, so no code
// outside this package can fabricate one. A test asserts this by construction
// (no such path exists) and by ablation (adding one goes red).
type Result struct {
	// outcome is unexported: only Resolve (and the internal constructors it
	// calls) may set it. This is the type-enforcement that makes ProvenAbsent
	// un-forgeable from outside this package.
	outcome Outcome

	// value is the committed leaf value, populated ONLY for ProvenPresent. It is
	// nil for ProvenAbsent (there is no value — the key is absent) and for
	// NoWitness (there is no proof). A caller that reads Value on a non-present
	// result gets nil, never a stale or defaulted value.
	value []byte
}

// Outcome returns which of the three states this Result is. Callers MUST switch
// on all three; the compiler will not force it, but a caller that treats a
// non-ProvenAbsent result as absent is the banned move. Prefer the intent
// helpers (IsProvenAbsent / IsProvenPresent / MustStall) at call sites so the
// NO_WITNESS branch cannot be silently dropped.
func (r Result) Outcome() Outcome { return r.outcome }

// Value returns the committed value for a ProvenPresent result. For ProvenAbsent
// and NoWitness it returns nil — there is no committed value in either case.
func (r Result) Value() []byte { return r.value }

// IsProvenAbsent reports whether the key is VERIFIABLY absent from the committed
// keyspace. This is the ONLY predicate a caller may use to conclude exclusion
// (e.g. spent[Serial] absent → the publish is not a double-spend). It is true
// ONLY when a non-membership proof verified against the committed root.
func (r Result) IsProvenAbsent() bool { return r.outcome == ProvenAbsent }

// IsProvenPresent reports whether the key is VERIFIABLY present, with its value
// available via Value().
func (r Result) IsProvenPresent() bool { return r.outcome == ProvenPresent }

// MustStall reports whether the floor box has NO usable answer for this key and
// must stall (or re-fetch the witness from another provider — the D-2 liveness
// path). It is true exactly for NoWitness. A caller evaluating a predicate that
// reads K MUST check MustStall and refuse to decide the predicate when it is
// true, never falling through to a false/absent reading. This is the direct
// encoding of the C-7 stall-not-accept invariant.
func (r Result) MustStall() bool { return r.outcome == NoWitness }

// Witness is a supplied SMT proof for one key, awaiting verification against the
// committed root. It wraps the pokt-network/smt proof. A Witness is untrusted
// input (it arrives from an any-of-N provider or in-block side data); it becomes
// meaningful ONLY after Resolve verifies it against the committed root. A nil or
// absent Witness resolves to NoWitness.
type Witness struct {
	proof *smt.SparseMerkleProof
}

// NewWitness wraps a supplied proof. The proof is untrusted; NewWitness performs
// NO verification (that is Resolve's job against a specific root). A nil proof is
// permitted and resolves to NoWitness — an absent witness is the expected
// missing-proof case, not an error.
func NewWitness(proof *smt.SparseMerkleProof) Witness { return Witness{proof: proof} }

// verifySpec is the TrieSpec the accessor verifies proofs under. It MUST match
// the spec statehash.Root builds its trie with (smt.NewSparseMerkleTrie(store,
// sha256.New()) — a non-sum SHA-256 trie), or a valid proof against the committed
// root would fail to verify here and every key would degrade to NoWitness. SHA-256
// is the pinned consensus/security parameter (statehash.go value-encoding cert
// Q6 flag 1); this reuses it, it does not choose a new one.
func verifySpec() *smt.TrieSpec {
	spec := smt.NewTrieSpec(sha256.New(), false)
	return &spec
}

// Resolve is the ONE function that produces a ProvenPresent or ProvenAbsent
// Result. Given the committed root, a field-tagged key (built with Key(tag,
// rawKey)), the expected value for a membership claim, and a supplied witness,
// it verifies the witness against the root and returns the three-valued Result.
//
// The membership/non-membership distinction is the value the caller asks the
// witness to prove:
//   - To test PRESENCE, pass the expected committed value. A verified proof of
//     (key → value) yields ProvenPresent(value).
//   - To test ABSENCE, pass a nil value. A verified non-membership proof yields
//     ProvenAbsent.
//
// The wiring that makes the banned move unrepresentable:
//   - no witness (nil) → NoWitness. Never ProvenAbsent.
//   - proof fails to verify (wrong root, tampered, wrong key) → NoWitness. Never
//     ProvenAbsent.
//   - VerifyProof returns an error → NoWitness. Never ProvenAbsent.
//
// ProvenAbsent is reachable ONLY through the single branch below where
// value == nil AND VerifyProof returned (true, nil). There is no other assignment
// of ProvenAbsent in this package.
func Resolve(root ports.Hash, key []byte, value []byte, w Witness) Result {
	if w.proof == nil {
		// No witness supplied. Expected missing-proof case → stall, never absent.
		return Result{outcome: NoWitness}
	}

	ok, err := smt.VerifyProof(w.proof, root[:], key, value, verifySpec())
	if err != nil || !ok {
		// The supplied proof failed to verify against the committed root: wrong
		// root, tampered proof, wrong key, or a library error. This is an ABSENT
		// witness, not an exclusion. → NoWitness, never ProvenAbsent.
		return Result{outcome: NoWitness}
	}

	// The proof verified. The claim it proved is determined by `value`:
	//   value == nil  → a non-membership claim verified → ProvenAbsent.
	//   value != nil  → a membership claim for that value verified → ProvenPresent.
	// This is the ONLY construction site for ProvenPresent and ProvenAbsent.
	if value == nil {
		return Result{outcome: ProvenAbsent}
	}
	return Result{outcome: ProvenPresent, value: value}
}
