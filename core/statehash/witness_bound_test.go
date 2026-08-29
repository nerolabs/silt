package statehash

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
)

// These are the R3 DoS-bound ablation tests: the pre-verify byte caps + shape gate,
// wired so EVERY rejection resolves to the R4 accessor's NoWitness outcome, never
// ProvenAbsent. Each test was watched RED-then-GREEN (see the builder report). The
// conflation guard (TestOverBudgetAbsenceQueryNeverProvenAbsent) is the
// safety-critical one: an over-budget/malformed/wrong-shaped witness for a key a
// predicate wants ABSENT must resolve to NoWitness, never ProvenAbsent — the one
// banned move of C-7 §104.

// encodeProof marshals a proof to the wire bytes the DoS bound measures.
func encodeProof(t testing.TB, p *smt.SparseMerkleProof) []byte {
	t.Helper()
	b, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal proof: %v", err)
	}
	return b
}

// absenceProof builds an absence (non-membership) proof for rawKey against a trie
// containing otherKeys, and returns the field-tagged key plus the encoded proof.
func absenceProof(t testing.TB, rawKey []byte, otherKeys ...[]byte) (ports.Hash, []byte, []byte) {
	t.Helper()
	trie, root := buildTrie(t, otherKeys...)
	key := Key(tagTest, rawKey)
	proof, err := trie.Prove(key)
	if err != nil {
		t.Fatalf("Prove(absent): %v", err)
	}
	return root, key, encodeProof(t, proof)
}

// presenceProof builds a membership proof for rawKey (which is inserted) against a
// trie also containing otherKeys, and returns the root, tagged key, and encoded proof.
func presenceProof(t testing.TB, rawKey []byte, otherKeys ...[]byte) (ports.Hash, []byte, []byte) {
	t.Helper()
	all := append([][]byte{rawKey}, otherKeys...)
	trie, root := buildTrie(t, all...)
	key := Key(tagTest, rawKey)
	proof, err := trie.Prove(key)
	if err != nil {
		t.Fatalf("Prove(present): %v", err)
	}
	return root, key, encodeProof(t, proof)
}

// TestOverProofCapRejectedPreParse is ablation 1: a single witness whose ENCODED
// size exceeds SProofMax is rejected BEFORE it is parsed/verified, and resolves to
// NoWitness. The RED premise: without the per-proof byte cap, the oversized blob
// would be unmarshaled and verified. We assert the cap is pre-parse by making the
// oversized Encoded bytes UNPARSEABLE garbage — if the gate parsed them it would
// hit an Unmarshal error path (still NoWitness, but the point is the bytes are
// never touched by Unmarshal because the size check fires first). The safety
// assertion is the outcome: NoWitness, never ProvenAbsent.
//
// This test is a GENUINE gate-1 discriminator: it goes RED when ONLY the per-proof
// cap (gate 1) is disabled. A single-key read-set would NOT — C_block = 1·SProofMax,
// so an over-SProofMax blob also breaches the per-block ceiling and gate 2 catches
// it, the test stays green, and its name lies. To pin the failure to gate 1, the
// read-set has TWO keys (C_block = 2·SProofMax) with a small honest proof for the
// second, so the over-cap blob + the honest proof stay UNDER the block ceiling.
// Only gate 1 can catch the oversized blob (mirrors TestOverProofCapIsPreParseNotVerify).
func TestOverProofCapRejectedPreParse(t *testing.T) {
	// Two absent keys against the same committed set → C_block = 2·SProofMax.
	trie, root := buildTrie(t, []byte("k1"), []byte("k2"))
	oversizedKey := Key(tagTest, []byte("oversized-key"))
	honestKey := Key(tagTest, []byte("honest-key"))

	honestProof, err := trie.Prove(honestKey)
	if err != nil {
		t.Fatalf("Prove(honest): %v", err)
	}
	honestEnc := encodeProof(t, honestProof)

	// An oversized blob of non-proof bytes (SProofMax+1). If the gate ever tried to
	// Unmarshal this, it would fail to decode — but the byte cap must fire FIRST, so
	// Unmarshal is never reached. Either way the outcome must be NoWitness.
	oversized := bytes.Repeat([]byte{0xAB}, SProofMax+1)

	readSet := []ReadEntry{{Key: oversizedKey, Kind: QueryAbsent}, {Key: honestKey, Kind: QueryAbsent}}

	// Premise: the over-cap blob breaches gate 1, but the bundle TOTAL stays under
	// C_block = 2·SProofMax, so gate 2 (the ceiling) CANNOT catch it — only gate 1 can.
	if len(oversized) <= SProofMax {
		t.Fatalf("premise: oversized blob %d must exceed the per-proof cap %d", len(oversized), SProofMax)
	}
	if len(oversized)+len(honestEnc) > CBlock(readSet) {
		t.Fatalf("premise: bundle %d must stay under C_block %d so ONLY the per-proof cap catches it",
			len(oversized)+len(honestEnc), CBlock(readSet))
	}
	bundle := []RawWitness{
		{Key: oversizedKey, Encoded: oversized},
		{Key: honestKey, Encoded: honestEnc},
	}

	got := IngestBlockWitnesses(root, readSet, bundle)

	if !got.Rejected {
		t.Fatal("an over-S_proof_max witness must reject the bundle pre-parse")
	}
	r := got.Results[string(oversizedKey)]
	if r.IsProvenAbsent() {
		t.Fatal("BANNED MOVE (C-7 §104): an over-cap witness resolved to PROVEN_ABSENT")
	}
	if !r.MustStall() {
		t.Fatalf("an over-cap witness must resolve to NO_WITNESS, got %s", r.Outcome())
	}
	// The reason names the per-proof cap, confirming gate 1 fired (not gate 2/3).
	if !strings.Contains(got.RejectReason, "S_proof_max") {
		t.Fatalf("expected the per-proof cap (gate 1) to fire; got reason %q", got.RejectReason)
	}
}

// TestOverProofCapIsPreParseNotVerify proves the per-proof cap does work the
// per-block ceiling cannot: a single bloated proof that is WITHIN the per-block
// ceiling. The read-set has TWO keys (C_block = 2·SProofMax), so a bundle of one
// honest small proof + one bloated ~1.5·SProofMax absence proof has total < C_block
// — gate 2 (the ceiling) does NOT catch the bloat. ONLY the per-proof byte cap
// (gate 1) stops it. The bloat is a padded-but-structurally-valid absence proof
// (NonMembershipLeafData is unbounded upward — cert fact 2), so if it reached
// VerifyProof it would verify as ProvenAbsent. The cap must stop it as NoWitness
// BEFORE parse. RED premise: with the per-proof cap disabled, the bloated proof
// falls through to verify and its ABSENCE key resolves to PROVEN_ABSENT.
func TestOverProofCapIsPreParseNotVerify(t *testing.T) {
	// Two absent keys against the same committed set → C_block = 2·SProofMax.
	trie, root := buildTrie(t, []byte("k1"), []byte("k2"))
	bloatKey := Key(tagTest, []byte("bloat-key"))
	honestKey := Key(tagTest, []byte("honest-key"))

	bloatProof, err := trie.Prove(bloatKey)
	if err != nil {
		t.Fatalf("Prove(bloat): %v", err)
	}
	honestProof, err := trie.Prove(honestKey)
	if err != nil {
		t.Fatalf("Prove(honest): %v", err)
	}

	// Bloat one proof to ~1.5·SProofMax: over the per-proof cap, but the bundle
	// total (~1.5·SProofMax + a tiny honest proof) stays under C_block = 2·SProofMax,
	// so the per-block ceiling cannot catch it.
	bloated := &smt.SparseMerkleProof{
		SideNodes:             bloatProof.SideNodes,
		NonMembershipLeafData: bytes.Repeat([]byte{0x00}, SProofMax*3/2),
		SiblingData:           bloatProof.SiblingData,
	}
	bloatEnc := encodeProof(t, bloated)
	honestEnc := encodeProof(t, honestProof)

	readSet := []ReadEntry{{Key: bloatKey, Kind: QueryAbsent}, {Key: honestKey, Kind: QueryAbsent}}
	if bloatEnc == nil || len(bloatEnc) <= SProofMax {
		t.Fatalf("premise: bloated proof %d must exceed cap %d", len(bloatEnc), SProofMax)
	}
	if len(bloatEnc)+len(honestEnc) > CBlock(readSet) {
		t.Fatalf("premise: bundle %d must stay under C_block %d so ONLY the per-proof cap catches it",
			len(bloatEnc)+len(honestEnc), CBlock(readSet))
	}
	bundle := []RawWitness{{Key: bloatKey, Encoded: bloatEnc}, {Key: honestKey, Encoded: honestEnc}}

	got := IngestBlockWitnesses(root, readSet, bundle)

	r := got.Results[string(bloatKey)]
	if r.IsProvenAbsent() {
		t.Fatal("BANNED MOVE: a bloated (within-ceiling) absence witness verified to PROVEN_ABSENT — " +
			"the per-proof byte cap did not fire pre-verify (a count cap or ceiling-only bound admits " +
			"the unbounded leaf-data blob)")
	}
	if !r.MustStall() {
		t.Fatalf("bloated over-cap absence witness must be NO_WITNESS, got %s", r.Outcome())
	}
	if !got.Rejected {
		t.Fatal("a bundle with an over-per-proof-cap witness must be rejected")
	}
}

// TestOverBlockCeilingRejectedPreVerify is ablation 2: a bundle whose TOTAL encoded
// bytes exceed C_block is rejected pre-verify, and NO proof is verified. It exercises
// the proof-COUNT blowup vector: a single-key read-set has C_block = 1·SProofMax, but
// an adversary supplies a bundle of TWO proofs each just under SProofMax (so each
// passes the per-proof cap, gate 1) whose sum exceeds C_block. The ceiling (gate 2)
// fires before the shape gate (gate 3) even runs — the ordering that stops
// proof-count blowup pre-verify, exactly the vector the per-proof cap alone leaves
// open (RULING §"Why the per-proof cap alone fails", vector 1). RED premise: without
// the per-block ceiling, the ingest walks into shape/verify carrying both blobs.
func TestOverBlockCeilingRejectedPreVerify(t *testing.T) {
	// A one-key read-set: C_block = 1 · SProofMax.
	root, key, _ := absenceProof(t, []byte("read-key"), []byte("k1"), []byte("k2"))
	readSet := []ReadEntry{{Key: key, Kind: QueryAbsent}}

	// Two proofs, each padded to just under SProofMax, so each passes gate 1 but the
	// SUM exceeds C_block = SProofMax. Both are offered for the read key (a
	// count-blowup bundle); the ceiling must fire before the shape gate.
	trie, _ := buildTrie(t, []byte("k1"), []byte("k2"))
	p, err := trie.Prove(key)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	padNear := func() []byte {
		base := encodeProof(t, p)
		grow := (SProofMax - 512) - len(base) // land ~512 B under the per-proof cap
		if grow < 0 {
			grow = 0
		}
		padded := &smt.SparseMerkleProof{
			SideNodes:             p.SideNodes,
			NonMembershipLeafData: bytes.Repeat([]byte{0x00}, len(p.NonMembershipLeafData)+grow),
			SiblingData:           p.SiblingData,
		}
		out := encodeProof(t, padded)
		if len(out) > SProofMax {
			t.Fatalf("pad overshoot: %d > cap %d", len(out), SProofMax)
		}
		return out
	}
	enc1, enc2 := padNear(), padNear()

	if len(enc1)+len(enc2) <= CBlock(readSet) {
		t.Fatalf("test premise broken: sum %d did not exceed C_block %d", len(enc1)+len(enc2), CBlock(readSet))
	}
	bundle := []RawWitness{{Key: key, Encoded: enc1}, {Key: key, Encoded: enc2}}

	got := IngestBlockWitnesses(root, readSet, bundle)

	if !got.Rejected {
		t.Fatal("an over-C_block bundle must be rejected pre-verify")
	}
	r := got.Results[string(key)]
	if r.IsProvenAbsent() {
		t.Fatal("BANNED MOVE: an over-ceiling bundle resolved a key to PROVEN_ABSENT")
	}
	if !r.MustStall() {
		t.Fatalf("over-ceiling bundle key must be NO_WITNESS, got %s", r.Outcome())
	}
	// Confirm the ceiling (gate 2) fired BEFORE the shape gate (gate 3). The
	// count-blowup bundle (two proofs for one read key) is ALSO a shape violation
	// (a duplicate), so the reason distinguishes which gate caught it. Gate 2 must
	// win: it is the cheaper running-sum early-out, run before the shape gate builds
	// its maps. This is the ordering that stops proof-count/byte blowup at the
	// cheapest possible point (RULING §"Where it is enforced").
	if !strings.Contains(got.RejectReason, "C_block") {
		t.Fatalf("expected the per-block ceiling (gate 2) to fire before the shape gate; "+
			"got reason %q", got.RejectReason)
	}
}

// TestShapeViolationsRejected is ablation 3: a bundle that does not carry a proof
// for EXACTLY the read-set — an extra unread key, a duplicate, or a missing read
// key — is rejected, and every read-set key resolves to NoWitness. RED premise:
// without the shape gate, an unread-key padding proof is admitted (and its verify
// cycles wasted), or a missing key is silently absent.
func TestShapeViolationsRejected(t *testing.T) {
	// A single-key read-set: one absent key.
	root, key, enc := absenceProof(t, []byte("read-key"), []byte("k1"), []byte("k2"))
	readSet := []ReadEntry{{Key: key, Kind: QueryAbsent}}

	// A valid witness for an UNREAD key (against the same root), used as padding.
	unreadKey := Key(tagTest, []byte("unread-key"))
	_, _, unreadEnc := absenceProof(t, []byte("unread-key"), []byte("k1"), []byte("k2"))

	cases := []struct {
		name   string
		bundle []RawWitness
	}{
		{
			name: "extra unread key (padding)",
			bundle: []RawWitness{
				{Key: key, Encoded: enc},
				{Key: unreadKey, Encoded: unreadEnc},
			},
		},
		{
			name: "duplicate witness for the read key",
			bundle: []RawWitness{
				{Key: key, Encoded: enc},
				{Key: key, Encoded: enc},
			},
		},
		{
			name:   "missing witness for the read key",
			bundle: []RawWitness{}, // empty bundle, read key has no proof
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IngestBlockWitnesses(root, readSet, tc.bundle)
			if !got.Rejected {
				t.Fatalf("%s: shape violation must reject the bundle", tc.name)
			}
			r := got.Results[string(key)]
			if r.IsProvenAbsent() {
				t.Fatalf("%s: BANNED MOVE — a shape-violating bundle resolved to PROVEN_ABSENT", tc.name)
			}
			if !r.MustStall() {
				t.Fatalf("%s: shape violation must be NO_WITNESS, got %s", tc.name, r.Outcome())
			}
		})
	}
}

// TestOverBudgetAbsenceQueryNeverProvenAbsent is THE conflation guard — the
// safety-critical ablation. A predicate wants key K ABSENT (a double-spend check:
// spent[serial] absent). An adversary supplies an over-budget / malformed /
// shape-violating witness for K. That rejected witness MUST resolve to NoWitness,
// NEVER ProvenAbsent. If the rejection were wired to the absent arm, the predicate
// would read a withheld/oversized witness as a proven exclusion and accept a
// forgery — C-7 §104, the one banned move. This test goes RED the instant any
// rejection path maps to ProvenAbsent.
//
// It exercises all three rejection vectors for an ABSENCE query key and asserts the
// outcome is NoWitness in every one.
func TestOverBudgetAbsenceQueryNeverProvenAbsent(t *testing.T) {
	// The predicate's key K is an absence query. A HONEST bundle for it would verify
	// to ProvenAbsent — that is the whole danger: the difference between the honest
	// verified answer (ProvenAbsent) and every rejected answer (NoWitness) must be
	// preserved. We first prove the honest path DOES yield ProvenAbsent, so the test
	// is not vacuous, then prove every rejection yields NoWitness instead.
	root, key, honestEnc := absenceProof(t, []byte("serial-K"), []byte("k1"), []byte("k2"))
	readSet := []ReadEntry{{Key: key, Kind: QueryAbsent}}

	t.Run("honest witness DOES verify to PROVEN_ABSENT (non-vacuous)", func(t *testing.T) {
		got := IngestBlockWitnesses(root, readSet, []RawWitness{{Key: key, Encoded: honestEnc}})
		if got.Rejected {
			t.Fatalf("honest absence witness was rejected: %s", got.RejectReason)
		}
		if !got.Results[string(key)].IsProvenAbsent() {
			t.Fatalf("honest absence witness must verify to PROVEN_ABSENT, got %s",
				got.Results[string(key)].Outcome())
		}
	})

	// Now every rejection vector for the SAME absence-query key must be NoWitness.
	oversized := bytes.Repeat([]byte{0xCC}, SProofMax+1)
	unreadKey := Key(tagTest, []byte("unread"))
	_, _, unreadEnc := absenceProof(t, []byte("unread"), []byte("k1"), []byte("k2"))

	vectors := []struct {
		name   string
		bundle []RawWitness
	}{
		{"over per-proof cap", []RawWitness{{Key: key, Encoded: oversized}}},
		{"missing witness", []RawWitness{}},
		{"shape padding (extra unread key)", []RawWitness{
			{Key: key, Encoded: honestEnc}, {Key: unreadKey, Encoded: unreadEnc},
		}},
		{"malformed encoding for the read key", []RawWitness{
			{Key: key, Encoded: []byte("not-a-gob-proof")},
		}},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			got := IngestBlockWitnesses(root, readSet, v.bundle)
			r := got.Results[string(key)]
			if r.IsProvenAbsent() {
				t.Fatalf("CONFLATION / BANNED MOVE (C-7 §104): a %s witness for an ABSENCE-query "+
					"key resolved to PROVEN_ABSENT — a rejected witness read as a proven exclusion",
					v.name)
			}
			if !r.MustStall() {
				t.Fatalf("%s: an absence-query key with a rejected witness must be NO_WITNESS, got %s",
					v.name, r.Outcome())
			}
		})
	}
}

// TestHonestBundleVerifies is the non-vacuity guard for the whole gate: an honest,
// exactly-shaped, within-budget bundle of a presence query and an absence query
// against the same root verifies BOTH to their proven outcomes. A gate that
// rejected everything (trivially safe but useless) fails here.
func TestHonestBundleVerifies(t *testing.T) {
	// Committed set: present-key is in, absent-key is not.
	trie, root := buildTrie(t, []byte("present-key"), []byte("filler"))
	presentKey := Key(tagTest, []byte("present-key"))
	absentKey := Key(tagTest, []byte("absent-key"))

	pProof, err := trie.Prove(presentKey)
	if err != nil {
		t.Fatalf("Prove(present): %v", err)
	}
	aProof, err := trie.Prove(absentKey)
	if err != nil {
		t.Fatalf("Prove(absent): %v", err)
	}

	readSet := []ReadEntry{
		{Key: presentKey, Kind: QueryPresent, Value: Present},
		{Key: absentKey, Kind: QueryAbsent},
	}
	bundle := []RawWitness{
		{Key: presentKey, Encoded: encodeProof(t, pProof)},
		{Key: absentKey, Encoded: encodeProof(t, aProof)},
	}

	got := IngestBlockWitnesses(root, readSet, bundle)
	if got.Rejected {
		t.Fatalf("honest in-budget exactly-shaped bundle was rejected: %s", got.RejectReason)
	}
	if !got.Results[string(presentKey)].IsProvenPresent() {
		t.Fatalf("present key must verify to PROVEN_PRESENT, got %s",
			got.Results[string(presentKey)].Outcome())
	}
	if !got.Results[string(absentKey)].IsProvenAbsent() {
		t.Fatalf("absent key must verify to PROVEN_ABSENT, got %s",
			got.Results[string(absentKey)].Outcome())
	}
}

// TestPresenceQueryEmptyValueNeverProvenAbsent is the Kind/Value-disagreement
// ablation (safety, blind PE review). A QueryPresent read carrying a nil/empty
// Value is a MALFORMED read-set entry: Resolve keys on len(value) == 0 to select
// the non-membership branch, so an empty value would route a presence query to
// ProvenAbsent — Kind says present, Value wins as absent. Kind is authoritative,
// so IngestBlockWitnesses must reject it to NoWitness BEFORE Resolve. This is the
// same class as the R4 empty-value finding.
//
// RED premise: without the Kind/Value gate, a QueryPresent{Value: nil} entry
// against a valid ABSENCE proof falls through to Resolve, which sees len(value)==0
// and returns ProvenAbsent. GREEN after the gate: the entry resolves to NoWitness.
func TestPresenceQueryEmptyValueNeverProvenAbsent(t *testing.T) {
	// A valid non-membership (absence) proof for the key. This is the dangerous
	// witness: if the presence query's empty value reaches Resolve, this proof
	// verifies and yields ProvenAbsent.
	root, key, absenceEnc := absenceProof(t, []byte("serial-K"), []byte("k1"), []byte("k2"))

	for _, tc := range []struct {
		name  string
		value []byte
	}{
		{"nil value", nil},
		{"empty non-nil value", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A presence query with NO value — the malformed entry. The shape gate is
			// satisfied (exactly one witness for the one read key), so the entry reaches
			// the per-key resolve loop where the Kind/Value gate must catch it.
			readSet := []ReadEntry{{Key: key, Kind: QueryPresent, Value: tc.value}}
			bundle := []RawWitness{{Key: key, Encoded: absenceEnc}}

			got := IngestBlockWitnesses(root, readSet, bundle)
			r := got.Results[string(key)]
			if r.IsProvenAbsent() {
				t.Fatalf("Kind/Value DISAGREEMENT: a QueryPresent entry with %s against a valid "+
					"absence proof resolved to PROVEN_ABSENT — Value won over an authoritative Kind "+
					"(same class as the R4 empty-value finding)", tc.name)
			}
			if !r.MustStall() {
				t.Fatalf("%s: a QueryPresent entry with no value must resolve to NO_WITNESS, got %s",
					tc.name, r.Outcome())
			}
		})
	}
}

// TestCBlockDerivation pins the exact C_block formula the gate enforces:
// C_block = len(readSet) · SProofMax, scaling with the read-set, not a flat
// constant. This is the ratified derivation (cert Q2). If someone changes it to a
// flat constant or a different multiple, this goes RED.
func TestCBlockDerivation(t *testing.T) {
	for _, n := range []int{0, 1, 2, 4, 1024} {
		rs := make([]ReadEntry, n)
		for i := range rs {
			rs[i] = ReadEntry{Key: Key(tagTest, []byte{byte(i), byte(i >> 8)}), Kind: QueryAbsent}
		}
		if got, want := CBlock(rs), n*SProofMax; got != want {
			t.Fatalf("CBlock(%d keys) = %d, want %d (= n · SProofMax)", n, got, want)
		}
	}
	if SProofMax != 16*1024 {
		t.Fatalf("SProofMax = %d, want 16 KiB (ratified security parameter)", SProofMax)
	}
}
