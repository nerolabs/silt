package statehash

import (
	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
)

// Witness floor-box DoS bound — build increment R3 (the pre-verify resource gate).
//
// This is the DoS-defence half of the semi-stateless witness floor box. It sits
// UPSTREAM of the R4 accessor (Resolve, witness.go): a block arrives with a bundle
// of supplied SMT witnesses (one per committed-set key its predicates read), and
// this layer decides — BEFORE any proof is parsed or verified — which witnesses are
// admissible. Every witness this layer rejects resolves to the R4 accessor's
// NoWitness outcome, NEVER ProvenAbsent and NEVER ProvenPresent. "The witness was
// too big / malformed / wrong-shaped, so I dropped it" reads downstream as "I have
// no witness (stall)", never as "the key is absent." A rejected witness mapped to
// ProvenAbsent is the one banned move (C-7 §104): an exclusion predicate would read
// a withheld/oversized witness as a proven exclusion and accept a forgery.
//
// The three gates, in the order they run at ingest (all PRE-parse / PRE-verify):
//
//  1. Per-proof byte cap (S_proof_max = 16 KiB). Reject any single witness whose
//     ENCODED size exceeds the cap BEFORE it is unmarshaled. This must be a byte
//     cap, not a side-node COUNT cap: the pokt library bounds a proof's side-node
//     count (proofs.go:57, 256 for SHA-256) but leaves NonMembershipLeafData
//     UNBOUNDED upward (proofs.go:62-75 checks only a minimum). A count cap "ships
//     and lies" — a single legal-shaped proof with a 100 MiB leaf-data blob passes
//     the library's validateBasic and is then parsed+hashed. The byte cap closes
//     that field. (Research cert, fact 2 + Q2.)
//
//  2. Per-block byte ceiling (C_block). At ingest, before verifying ANY proof,
//     reject a witness bundle whose TOTAL encoded bytes exceed the per-block
//     ceiling. C_block is DERIVED per block, not a flat constant:
//
//     C_block = len(expected read-set) · S_proof_max
//
//     The read-set is the exact set of committed-set keys this block's transitions
//     read (the shape gate below pins the bundle to exactly that set), so the
//     honest witness bundle is exactly len(read-set) proofs, each ≤ S_proof_max.
//     C_block is therefore the tight, exact ceiling the certification's
//     "expected_witness_bytes(this_block)" form collapses to once the shape gate
//     holds (cert Q2, §104). It needs no per-block transition cap — a transition
//     cap would be a consensus-rule change (cert Q3) and is the wrong lever.
//
//  3. Shape gate. The bundle must carry a proof for EXACTLY the block's read-set —
//     no key the block does not read, no duplicate proof for a key, no padding
//     entry, and no missing proof for a read key. This is a STRUCTURAL rejection
//     (keys only), cheaper than any crypto check, and it defeats the unread-key
//     padding vector by construction (RULING §"What the menu missed", cert §54).
//
// SCOPE (R3): the pre-verify byte caps + shape gate, wired so every rejection maps
// to NoWitness. It does NOT build D-2 on-demand delivery, the A-serve slow-loris
// read deadline (a TIME attack the byte ceiling does not close — cert R-loris), or
// a fetch fan-out cap. Those are increment 3. This layer assumes the witness bundle
// is already in hand (in-block carry or a completed fetch) and gates it.
//
// Certified by:
//   - witness-floor-box-dos-bound-RESEARCH-CERTIFICATION-2026-08-29 (the byte-cap
//     mechanism, S_proof_max = 16 KiB, the C_block derivation, the shape gate; the
//     security parameters ratified by Andrew).
//   - RULING-witness-floor-box-mechanism-2026-08-29 (R3: aggregate per-block byte
//     ceiling checked at ingest BEFORE verify; the shape gate as the stronger cut;
//     every rejection → R4 NoWitness).
//   - C-7 §104 (the one banned move: no/over-budget witness → accept is forbidden).

const (
	// SProofMax is the per-proof envelope byte cap: the maximum ENCODED size of a
	// single supplied witness, enforced BEFORE the proof is unmarshaled or verified.
	// 16 KiB, a RATIFIED security parameter (research cert Q2; Andrew ratified).
	//
	// Derivation (cert Q2): an honest proof's bytes are side-nodes (≤ 256 × 32 =
	// 8,192 B, library-capped) + NonMembershipLeafData (honest ≤ ~65 B, capped at a
	// generous 256 B) + SiblingData (≤ ~256 B) + gob framing (≤ ~256 B) ≈ 9 KiB,
	// rounded to 16 KiB for headroom. The cap MUST be bytes, not a side-node count:
	// the library leaves NonMembershipLeafData unbounded upward, so a count cap does
	// not bound a single proof's bytes (cert fact 2). This closes that field.
	SProofMax = 16 * 1024
)

// QueryKind names what a read-set key asks its witness to prove: presence of a
// value, or absence. It is derived from the predicate that reads the key (a
// double-spend check asks spent[serial] ABSENT; a takedown asks byRoot[root]
// PRESENT). The floor box knows the kind from the block's transitions, so the
// shape gate is keyed on (key, kind): a witness for a read key must prove the kind
// the predicate expects.
type QueryKind uint8

const (
	// QueryAbsent asks the witness to prove the key is absent from the committed
	// keyspace (a non-membership proof). Resolve is called with an empty value.
	QueryAbsent QueryKind = iota

	// QueryPresent asks the witness to prove the key is present with an expected
	// value (a membership proof). Resolve is called with that value.
	QueryPresent
)

// ReadEntry is one key the block's predicates read: the field-tagged committed-set
// key (built with Key(tag, rawKey)), the kind of query (presence/absence), and —
// for a presence query — the expected committed value the membership proof must
// match. The floor box computes the block's read-set as a slice of these from the
// block's transitions (the predicate table); this package gates a witness bundle
// against that read-set.
type ReadEntry struct {
	// Key is the field-tagged leaf key, e.g. Key(tagSpent, serial). It is what both
	// the shape gate matches on and what Resolve verifies against the root.
	Key []byte

	// Kind is presence or absence — the claim the predicate needs proven.
	Kind QueryKind

	// Value is the expected committed value for a QueryPresent read (len > 0). It is
	// ignored for QueryAbsent. Resolve keys on len(value) == 0 to select
	// membership vs non-membership, matching the library's empty-value convention
	// (witness.go Resolve doc), so a QueryPresent entry MUST carry a non-empty value.
	Value []byte
}

// RawWitness is one supplied, UNPARSED witness: the field-tagged key it is offered
// for, and the ENCODED (gob-marshaled) proof bytes. It is untrusted input arriving
// from an any-of-N provider or in-block side data. The DoS bound gates it by its
// encoded byte length BEFORE it is unmarshaled — that is the whole point of R3: the
// adversary must not be able to force a parse/verify of an oversized blob. A
// RawWitness is turned into a *smt.SparseMerkleProof only AFTER it passes the byte
// caps and the shape gate.
type RawWitness struct {
	// Key is the field-tagged key this witness is offered for. The shape gate
	// matches it against the block's read-set; a key not in the read-set is padding
	// and rejects the whole bundle.
	Key []byte

	// Encoded is the gob-marshaled SparseMerkleProof bytes (proof.Marshal()). Its
	// length is what the byte caps measure — the pre-parse cap is len(Encoded), so
	// an oversized blob is dropped before Unmarshal ever allocates its fields.
	Encoded []byte
}

// IngestResult is the outcome of gating a whole block's witness bundle against its
// read-set. It maps each read-set key to an R4 Result: either a verified
// ProvenPresent/ProvenAbsent (the witness passed every gate AND verified against
// the root), or NoWitness (the witness was rejected by a byte cap or the shape
// gate, was missing, or failed to parse/verify). A caller evaluating a predicate
// that reads key K looks up Results[string(K)] and acts on its three-valued
// outcome — a MustStall result means stall/re-fetch, never treat as absent.
//
// CRITICAL WIRING: there is NO entry in Results whose outcome is ProvenAbsent that
// did not come from a witness passing every byte cap, the shape gate, AND a
// verified non-membership proof. Every rejection path — over per-proof cap, over
// per-block ceiling, shape mismatch, unparseable, failed verify — writes NoWitness.
// This is enforced structurally: the only call that can produce ProvenAbsent is
// Resolve (witness.go), reached only in the admit branch below.
type IngestResult struct {
	// Results maps string(read-set key) → the R4 Result for that key. Every key in
	// the block's read-set has an entry; a key whose witness was rejected maps to a
	// NoWitness Result (the safe stall state).
	Results map[string]Result

	// Rejected is true if the whole bundle was rejected pre-verify (over C_block, a
	// shape violation, or any per-proof cap breach). When true, NO proof in the
	// bundle was parsed or verified, and EVERY read-set key maps to NoWitness. The
	// flag lets a caller distinguish "the bundle was structurally bad, stall on all
	// keys" from "some individual witnesses failed to verify." Either way the safe
	// action is identical: stall any key that is not ProvenPresent/ProvenAbsent.
	Rejected bool

	// RejectReason is a short human-readable reason when Rejected is true, for logs
	// and tests. It is NOT consulted for control flow — the safety property is that
	// a rejected bundle maps every key to NoWitness regardless of the reason.
	RejectReason string
}

// allNoWitness builds an IngestResult where every read-set key maps to NoWitness —
// the safe result for a bundle rejected pre-verify. It is the ONLY result a
// bundle-level rejection produces, and it constructs zero-value (NoWitness) Results
// exclusively: there is no path here to ProvenAbsent. This is the structural
// guarantee that an over-budget / malformed / wrong-shaped bundle can never read as
// a proven exclusion.
func allNoWitness(readSet []ReadEntry, reason string) IngestResult {
	res := make(map[string]Result, len(readSet))
	for _, re := range readSet {
		res[string(re.Key)] = Result{outcome: NoWitness} // zero value; the safe state
	}
	return IngestResult{Results: res, Rejected: true, RejectReason: reason}
}

// CBlock is the derived per-block witness byte ceiling for a block whose predicates
// read the given read-set: C_block = len(readSet) · SProofMax. It is exact and
// tight because the shape gate pins the honest bundle to exactly one proof per
// read-set key, each ≤ SProofMax. A bundle whose total encoded bytes exceed CBlock
// is rejected pre-verify. Exposed so a caller (and the tests) can compute the same
// ceiling the ingest gate enforces.
//
// This is NOT a flat constant: it scales with the block's actual read-set, so a
// small block gets a tight ceiling and a large block gets a proportionally larger
// one — no size lottery that would starve a legal high-demand block (cert Q2, the
// #441 starvation shape one layer down). It requires no per-block transition cap
// (cert Q3): the block's own read-set bounds its honest witness.
func CBlock(readSet []ReadEntry) int {
	return len(readSet) * SProofMax
}

// IngestBlockWitnesses is the R3 gate: given the committed StateRoot, the block's
// read-set (the exact committed-set keys its predicates read, with their query
// kinds), and the supplied witness bundle, it applies the three pre-verify gates
// and returns the R4 Result for every read-set key. Every rejection resolves to
// NoWitness; only a witness that passes all gates AND verifies against the root
// yields ProvenPresent/ProvenAbsent.
//
// Order is load-bearing (all gates run BEFORE any VerifyProof):
//
//  1. Per-proof byte cap: any RawWitness with len(Encoded) > SProofMax rejects the
//     WHOLE bundle pre-parse. (A single oversized blob is an attack on the box; the
//     block's witness is not trustworthy, so the box stalls on all its keys.)
//  2. Per-block byte ceiling: total encoded bytes > CBlock(readSet) rejects the
//     whole bundle pre-verify.
//  3. Shape gate: the bundle's key SET must equal the read-set's key set exactly —
//     no extra key, no duplicate, no missing key. Any mismatch rejects the whole
//     bundle pre-verify.
//
// Only when all three pass does it unmarshal each proof and call Resolve. A proof
// that fails to unmarshal or fails to verify maps that ONE key to NoWitness (the
// bundle is not bundle-rejected in that case — an individual verify failure is a
// per-key stall, not evidence the whole bundle is adversarial). ProvenAbsent /
// ProvenPresent are reachable ONLY through Resolve in the admit branch.
func IngestBlockWitnesses(root ports.Hash, readSet []ReadEntry, bundle []RawWitness) IngestResult {
	// Gate 1 — per-proof byte cap, PRE-parse. Checked first and across the whole
	// bundle: if any single witness is oversized, the bundle is adversarial and the
	// box stalls on every key. No Unmarshal has run at this point.
	total := 0
	for _, rw := range bundle {
		if len(rw.Encoded) > SProofMax {
			return allNoWitness(readSet, "per-proof byte cap exceeded (S_proof_max)")
		}
		total += len(rw.Encoded)
	}

	// Gate 2 — per-block byte ceiling, PRE-verify. Derived from the read-set, exact
	// under the shape gate. Still no Unmarshal / VerifyProof has run.
	if ceiling := CBlock(readSet); total > ceiling {
		return allNoWitness(readSet, "per-block byte ceiling exceeded (C_block)")
	}

	// Gate 3 — shape gate, PRE-verify. The bundle's key set must equal the read-set
	// key set EXACTLY: one witness per read key, no unread key, no duplicate. A
	// structural (keys-only) check, still pre-parse.
	if reason := checkShape(readSet, bundle); reason != "" {
		return allNoWitness(readSet, reason)
	}

	// All bundle gates passed. Only now do we parse and verify. Index the bundle by
	// key (shape gate proved it is a bijection with the read-set).
	byKey := make(map[string]RawWitness, len(bundle))
	for _, rw := range bundle {
		byKey[string(rw.Key)] = rw
	}

	results := make(map[string]Result, len(readSet))
	for _, re := range readSet {
		rw := byKey[string(re.Key)] // present by the shape gate

		var proof smt.SparseMerkleProof
		if err := proof.Unmarshal(rw.Encoded); err != nil {
			// Unparseable proof for this key → NoWitness (stall), never ProvenAbsent.
			results[string(re.Key)] = Result{outcome: NoWitness}
			continue
		}

		// The query value: empty for an absence query (Resolve keys on len==0 to
		// select non-membership), the expected value for a presence query.
		var value []byte
		if re.Kind == QueryPresent {
			value = re.Value
		}

		// Resolve is the ONE producer of ProvenPresent/ProvenAbsent. A failed verify
		// inside Resolve returns NoWitness. This is the admit branch — the only path
		// to a proven outcome, reached only after all three pre-verify gates passed.
		results[string(re.Key)] = Resolve(root, re.Key, value, NewWitness(&proof))
	}

	return IngestResult{Results: results, Rejected: false}
}

// checkShape enforces the shape gate: the bundle's key set must equal the
// read-set's key set exactly. It returns "" on an exact match, or a short reason on
// any violation (a duplicate key in the bundle, a key not in the read-set, or a
// read-set key with no witness). Keys-only; no proof is parsed. This is the
// structural rejection that defeats unread-key padding by construction.
func checkShape(readSet []ReadEntry, bundle []RawWitness) string {
	want := make(map[string]struct{}, len(readSet))
	for _, re := range readSet {
		want[string(re.Key)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(bundle))
	for _, rw := range bundle {
		k := string(rw.Key)
		if _, dup := seen[k]; dup {
			return "shape gate: duplicate witness for a key"
		}
		seen[k] = struct{}{}
		if _, ok := want[k]; !ok {
			return "shape gate: witness for a key the block does not read (padding)"
		}
	}
	// Every read-set key must have a witness. len equality + subset (checked above)
	// implies set equality, but check explicitly for a precise reason.
	if len(seen) != len(want) {
		return "shape gate: missing witness for a read-set key"
	}
	return ""
}
