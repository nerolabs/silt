package chain

import (
	"crypto/ed25519"
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R0.6 — the I5 cross-height Pruned slash-forgery fix. Certification:
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/
// I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
//
// Root cause (cert §3): VerifyEquivocation quantifies over a fact (the height two
// signatures were released at) that lives OUTSIDE the signed message, and reads that fact
// from two different sources: a struct field (equivocation.go:50) for the height check, and
// Hash() (equivocation.go:53) for the signature check. For a NON-pruned block those sources
// agree; a pruned block's Hash() returns the attacker-chosen b.Pruned field instead, severing
// the identity. The certified fix (F2-EVIDENCE-RECOMPUTE, cert §5.1): compute both blocks'
// hashes by re-marshalling the body, NEVER by reading Block.Pruned; a pruned block is
// therefore never admissible equivocation evidence — the accept set only narrows.
//
// Every test below drives the REAL commit path (Append) — "Append is the oracle in every
// case" (cert §10 preamble) — because VerifyEquivocation alone was the exact reason RT-SV-1
// looked survivable: it stopped short of the write path.

// ---------------------------------------------------------------------------------------
// T-1 / T-2 — the forged cross-height Pruned slash must be REFUSED end-to-end through
// Append, the culprit must remain unslashed, and must remain qualified.
// ---------------------------------------------------------------------------------------

// TestSlashProofBindsHeightToTheSignedHash_Era1 is T-1 (cert §10). RT-SV-1 verbatim, driven
// through Append. RED today: Append currently ACCEPTS the forged slash (VerifyEquivocation
// trusts Block.Pruned as if it were Hash()), so the culprit is wrongly evicted.
func TestSlashProofBindsHeightToTheSignedHash_Era1(t *testing.T) {
	w := newWorld(DefaultConfig())
	b0 := w.block(entry(1))
	w.attestAll(b0)
	if err := w.c.Append(*b0); err != nil {
		t.Fatal(err)
	}
	culprit := w.vals[0]
	culpritPub := culprit.Public().(ed25519.PublicKey)
	culpritID := idOf(culprit)

	// Two HONEST, SEQUENTIAL blocks proposed by the culprit at DIFFERENT heights — what
	// every active proposer does, never equivocation.
	h0, _ := w.c.Head()
	honestA := Block{Version: 1, Height: 1, Prev: h0, Entries: []ports.Entry{entry(2)}}
	Sign(&honestA, culprit)
	honestB := Block{Version: 1, Height: 2, Prev: honestA.Hash(), Entries: []ports.Entry{entry(3)}}
	Sign(&honestB, culprit)
	if VerifyEquivocation(&Equivocation{Culprit: culpritPub, A: honestA, B: honestB}) {
		t.Fatal("precondition broken: honest sequential proposing is not equivocation")
	}

	// FORGE: keep the genuine signatures, override Hash() with Pruned, declare a single
	// shared fictitious height. No key material required.
	fa := Block{Version: 1, Height: 4242, Pruned: honestA.Hash(),
		Proposer: append([]byte(nil), culpritPub...), ProposerSig: honestA.ProposerSig}
	fb := Block{Version: 1, Height: 4242, Pruned: honestB.Hash(),
		Proposer: append([]byte(nil), culpritPub...), ProposerSig: honestB.ProposerSig}
	forged := Equivocation{Culprit: culpritPub, A: fa, B: fb}

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)},
		Slashes: []Equivocation{forged}}
	Sign(&bs, w.prop)
	w.attestAll(&bs)

	err := w.c.Append(bs)
	if err == nil {
		t.Fatal("T-1 RED (expected): Append accepted a cross-height Pruned slash forgery — " +
			"VerifyEquivocation must recompute block hashes, never trust Block.Pruned " +
			"(I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md §5.1)")
	}
	if !errors.Is(err, ErrBadSlash) && !errors.Is(err, ErrPrunedEvidence) {
		t.Fatalf("T-1: want ErrBadSlash or ErrPrunedEvidence, got %v", err)
	}
	if w.c.slashed[culpritID] {
		t.Fatal("T-1: the honest validator must not be slashed by a proof-free cross-height forgery")
	}
	if !w.c.attesterQualified(culpritID) {
		t.Fatal("T-1: the honest validator must remain qualified after the forged slash is refused")
	}
}

// TestSlashProofBindsHeightToTheSignedHash_Era2 is T-2 (cert §10) — RT-SV-1b, extended to
// drive Append (the shipped probe stopped at VerifyEquivocation). RED today for the same
// reason as T-1.
func TestSlashProofBindsHeightToTheSignedHash_Era2(t *testing.T) {
	w := newWorld(DefaultConfig())
	b0 := w.block(entry(1))
	w.attestAll(b0)
	if err := w.c.Append(*b0); err != nil {
		t.Fatal(err)
	}
	culprit := w.vals[1]
	culpritPub := culprit.Public().(ed25519.PublicKey)
	culpritID := idOf(culprit)

	h0, _ := w.c.Head()
	realA := Block{Version: BlockVersionRounds, Height: 1, Prev: h0, Entries: []ports.Entry{entry(2)}}
	Sign(&realA, w.prop)
	realB := Block{Version: BlockVersionRounds, Height: 2, Prev: realA.Hash(), Entries: []ports.Entry{entry(3)}}
	Sign(&realB, w.prop)
	// Honest era-2 precommits at the SAME round but DIFFERENT heights: the textbook honest
	// schedule, never slashable.
	attA := AttestAt(&realA, culprit, 0, PhasePrecommit)
	attB := AttestAt(&realB, culprit, 0, PhasePrecommit)
	realA.Atts = append(realA.Atts, attA)
	realB.Atts = append(realB.Atts, attB)
	if VerifyEquivocation(&Equivocation{Culprit: culpritPub, A: realA, B: realB}) {
		t.Fatal("precondition broken: same-round precommits at DIFFERENT heights are honest")
	}

	fa := Block{Version: BlockVersionRounds, Height: 777, Pruned: realA.Hash(), Atts: []Attestation{attA}}
	fb := Block{Version: BlockVersionRounds, Height: 777, Pruned: realB.Hash(), Atts: []Attestation{attB}}
	forged := Equivocation{Culprit: culpritPub, A: fa, B: fb}

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)},
		Slashes: []Equivocation{forged}}
	Sign(&bs, w.prop)
	w.attestAll(&bs)

	err := w.c.Append(bs)
	if err == nil {
		t.Fatal("T-2 RED (expected): Append accepted an era-2 cross-height Pruned slash forgery")
	}
	if !errors.Is(err, ErrBadSlash) && !errors.Is(err, ErrPrunedEvidence) {
		t.Fatalf("T-2: want ErrBadSlash or ErrPrunedEvidence, got %v", err)
	}
	if w.c.slashed[culpritID] {
		t.Fatal("T-2: the honest validator must not be slashed by a proof-free cross-height forgery")
	}
	if !w.c.attesterQualified(culpritID) {
		t.Fatal("T-2: the honest validator must remain qualified after the forged slash is refused")
	}
}

// TestGenuineDoubleSignStillConvictsThroughAppend is T-3 (cert §10): guards against "fix by
// refusing everything." GREEN today; must stay green after the fix — a real, non-pruned,
// same-height double-sign still commits the slash through Append.
func TestGenuineDoubleSignStillConvictsThroughAppend(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	a, b := w.conflicting(g, w.prop, w.vals[3], []ed25519.PrivateKey{w.vals[0]}, []ed25519.PrivateKey{w.vals[0]})
	culpritID := idOf(w.vals[0])

	e := Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *b}
	if !VerifyEquivocation(&e) {
		t.Fatal("precondition: the full double-sign must be genuinely provable")
	}

	// Commit a's fork as the canonical history so the follow-on block has a real parent.
	w.attestAll(a)
	if err := w.c.Append(*a); err != nil {
		t.Fatalf("committing the canonical fork: %v", err)
	}

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)},
		Slashes: []Equivocation{e}}
	Sign(&bs, w.prop)
	w.attestAll(&bs)
	if err := w.c.Append(bs); err != nil {
		t.Fatalf("T-3: a genuine full-body same-height double-sign must still commit the slash, got err=%v", err)
	}
	if !w.c.slashed[culpritID] {
		t.Fatal("T-3: culprit must be slashed after a genuine, non-pruned double-sign is committed")
	}
	if w.c.attesterQualified(culpritID) {
		t.Fatal("T-3: the culprit must lose qualification after being slashed")
	}
}

// ---------------------------------------------------------------------------------------
// T-6 — the (d-2) per-block encoded-byte ceiling on Slashes (cert §7).
// ---------------------------------------------------------------------------------------

// TestSlashesBytesCapEnforced is T-6's over-ceiling half. Duplicating a SINGLE genuine
// double-sign proof until the canonically-encoded Slashes field exceeds the (stub) byte
// ceiling must be REJECTED — VerifyEquivocation places no dedup or count bound on Slashes
// today, so nothing stops full evidence bodies (each pinning two full block bodies,
// including BondReg.Answer where present) from accumulating without limit in a slot
// Prune() never reaches (cert §6.2). RED today: no byte ceiling exists, so this ACCEPTS.
func TestSlashesBytesCapEnforced(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	e := fatGenuineProof(t, w, g)
	slashes, lastLen := proofsAroundCap(t, e, true)

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)}, Slashes: slashes}
	Sign(&bs, w.prop)
	w.attestAll(&bs)

	err := w.c.ValidateProposal(&bs)
	if err == nil {
		t.Fatalf("T-6 RED (expected): a block whose canonically-encoded Slashes field is %d "+
			"bytes (cap %d, %d duplicated genuine proofs) was ACCEPTED — no per-block byte "+
			"ceiling exists on Slashes today (cert §6.2/§7)", lastLen, SlashesBytesCap, len(slashes))
	}
	if !errors.Is(err, ErrSlashesBytesCapExceeded) {
		t.Fatalf("T-6: want the byte-ceiling error, got %v", err)
	}
}

// fatGenuineProof is a genuine (full/full, same-height) double-sign by w.vals[0] whose two
// blocks each carry ~1 MiB of entry payload, so a handful of proofs reach SlashesBytesCap
// and the T-6 fixtures stay fast (a header-only proof is ~1 KB; ~18k of them would make
// the at-ceiling validation the slow part of the tier).
func fatGenuineProof(t *testing.T, w *world, g *Block) Equivocation {
	t.Helper()
	fat := func(seed byte) ports.Entry {
		e := entry(seed)
		e.ManifestChunks = make([]ports.ChunkID, 32<<10) // 32k × 32 B = 1 MiB
		for i := range e.ManifestChunks {
			e.ManifestChunks[i] = ports.HashBytes([]byte{seed, byte(i), byte(i >> 8)})
		}
		return e
	}
	a := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{fat(1)}}
	Sign(a, w.prop)
	a.Atts = append(a.Atts, Attest(a, w.vals[0]))
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{fat(2)}}
	Sign(b, w.vals[3])
	b.Atts = append(b.Atts, Attest(b, w.vals[0]))
	e := Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *b}
	if !VerifyEquivocation(&e) {
		t.Fatal("precondition: the double-sign must be genuinely provable")
	}
	return e
}

// proofsAroundCap duplicates e until the canonically-encoded Slashes field is just OVER
// SlashesBytesCap (over=true) or the largest list at-or-under it (over=false). Returns
// the list and its encoded size, measured by the same function validateSlashes uses.
func proofsAroundCap(t *testing.T, e Equivocation, over bool) ([]Equivocation, int) {
	t.Helper()
	var slashes []Equivocation
	for {
		next := append(slashes, e)
		n := SlashesEncodedSize(next)
		if n > SlashesBytesCap {
			if over {
				return next, n
			}
			if len(slashes) == 0 {
				t.Fatal("fixture: a single genuine proof already exceeds SlashesBytesCap — shrink the fat entry")
			}
			return slashes, SlashesEncodedSize(slashes)
		}
		slashes = next
		if len(slashes) > 1000 {
			t.Fatal("fixture runaway: could not reach SlashesBytesCap")
		}
	}
}

// TestSlashesAtCeilingAccepted is T-6's I4 liveness edge, mirroring TestRegCapAtCeilingAccepted
// (modelcheck_era4_regcap_test.go): a block AT (not over) the byte ceiling must still ACCEPT.
// Expected GREEN both before and after the fix — the cap must never reject an honest
// at-ceiling slash.
func TestSlashesAtCeilingAccepted(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	e := fatGenuineProof(t, w, g)
	slashes, _ := proofsAroundCap(t, e, false)

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)}, Slashes: slashes}
	Sign(&bs, w.prop)
	w.attestAll(&bs)

	if err := w.c.ValidateProposal(&bs); err != nil {
		t.Fatalf("T-6: an at-or-under-ceiling block with %d genuine proofs was rejected, got %v", len(slashes), err)
	}
}

// ---------------------------------------------------------------------------------------
// G-2 — the honest-path-unaffected claim: FindEquivocations never pairs a pruned block.
// ---------------------------------------------------------------------------------------

// TestFindEquivocationsNeverPairsAPrunedBlock is G-2 (cert §11). FindEquivocations is what
// slashEquivocators (core/node/chainrole.go) feeds every peer-served fork through on
// detection — this drives the same two-slice shape it receives. Post-fix this is a direct
// corollary of T-4/F2-EVIDENCE-RECOMPUTE (pruned evidence never verifies): pinned here at
// the FindEquivocations layer specifically because ITS candidate-selection comparison
// (equivocation.go:130, ab.Hash()==bb.Hash()) must use the SAME recomputing hash function as
// VerifyEquivocation (G-6) — if it did not, a stale Pruned digest could still pass candidate
// SELECTION even though VerifyEquivocation would (correctly) refuse it. RED today: Prune()
// preserves the Atts/ProposerSig, and Hash() trusts the stored Pruned value, so the pruned
// side's signature verifies unchanged and FindEquivocations returns a live conviction.
func TestFindEquivocationsNeverPairsAPrunedBlock(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	a, b := w.conflicting(g, w.prop, w.vals[3], []ed25519.PrivateKey{w.vals[0]}, []ed25519.PrivateKey{w.vals[0]})

	// The peer's served history carries the SAME conflicting block, payload-pruned — a
	// legitimate late-reveal shape (or an attacker manufacturing the same effect).
	pb := b.Prune()

	found := FindEquivocations([]Block{*g, *a}, []Block{*g, pb})
	for _, e := range found {
		if e.A.IsPruned() || e.B.IsPruned() {
			t.Fatalf("G-2 VIOLATION: FindEquivocations produced an Equivocation with a PRUNED "+
				"side (A.Pruned-set=%v B.Pruned-set=%v) — pruned evidence must never be usable, "+
				"on the detection path exactly as on the direct VerifyEquivocation path (T-4)",
				e.A.IsPruned(), e.B.IsPruned())
		}
	}
	if len(found) != 0 {
		t.Fatalf("G-2: expected NO equivocations from a pair where one side is pruned, got %d", len(found))
	}
}

// ---------------------------------------------------------------------------------------
// G-4 — the memo bypass: the recompute must not be satisfiable by a stale hashMemo.
// ---------------------------------------------------------------------------------------

// TestVerifyEquivocationNotSatisfiableByStaleMemo is G-4 (cert §5.1 point 3, §11). Same
// underlying defect as RT-SV-1/T-1 — a fact (Height) decoupled from what Hash() actually
// covers — reached via the hashMemo cache (F5/RT-SV-2's mechanism) instead of Pruned: a
// Block value can be mutated in place AFTER Sign() without invalidating hashMemo, so Hash()
// keeps returning a digest over content the struct no longer holds. RED today: VerifyEquivocation
// reads e.A.Hash()/e.B.Hash() directly, so it inherits this staleness exactly like Append/
// blockByHash do (RT-SV-2).
func TestVerifyEquivocationNotSatisfiableByStaleMemo(t *testing.T) {
	w := newWorld(DefaultConfig())
	b0 := w.block(entry(1))
	w.attestAll(b0)
	if err := w.c.Append(*b0); err != nil {
		t.Fatal(err)
	}
	culprit := w.vals[2]
	culpritPub := culprit.Public().(ed25519.PublicKey)
	culpritID := idOf(culprit)

	h0, _ := w.c.Head()
	honestA := Block{Version: 1, Height: 1, Prev: h0, Entries: []ports.Entry{entry(2)}}
	Sign(&honestA, culprit) // memo set over Height=1 content

	honestB := Block{Version: 1, Height: 2, Prev: honestA.Hash(), Entries: []ports.Entry{entry(3)}}
	Sign(&honestB, culprit) // an honest, later, sequential block — never equivocation with honestA

	if VerifyEquivocation(&Equivocation{Culprit: culpritPub, A: honestA, B: honestB}) {
		t.Fatal("precondition broken: two honest sequential blocks must not verify as equivocation")
	}

	// Mutate a hash-covered field DIRECTLY (no re-Sign, no memo invalidation) so the struct
	// now DECLARES Height=2 (matching honestB) while its Hash() keeps returning the STALE
	// memo computed over the ORIGINAL Height=1 content.
	stale := honestA.Hash() // captures the memo
	forgedA := honestA
	forgedA.Height = honestB.Height
	if forgedA.Hash() != stale {
		t.Fatal("G-4 precondition broken: mutating Height did not preserve the stale memo — " +
			"fixture assumption about hashMemo invalid")
	}

	forged := Equivocation{Culprit: culpritPub, A: forgedA, B: honestB}
	if VerifyEquivocation(&forged) {
		t.Fatal("G-4 VIOLATION (RED expected): VerifyEquivocation accepted evidence whose " +
			"declared Height (the struct field) does not match what its OWN memoized Hash() " +
			"covers — the recompute must bypass hashMemo (cert §5.1 point 3), never trust a " +
			"cached digest over content that has since been mutated")
	}

	// Drive it through the real commit path too (Append is the oracle).
	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)},
		Slashes: []Equivocation{forged}}
	Sign(&bs, w.prop)
	w.attestAll(&bs)
	err := w.c.Append(bs)
	if err == nil {
		t.Fatal("G-4: Append accepted a stale-memo forged slash")
	}
	if w.c.slashed[culpritID] {
		t.Fatal("G-4: culprit must not be slashed via a stale-memo forgery")
	}
}

// ---------------------------------------------------------------------------------------
// G-6 — one hash function: candidate selection and verification must call the SAME
// hash-producing method.
// ---------------------------------------------------------------------------------------

// TestOneHashFunctionForEquivocationCandidatesAndVerification is G-6 (cert §5.1 point 1,
// "ONE FUNCTION, THREE CALLERS", §11). FindEquivocations selects CANDIDATES by comparing
// block hashes and VerifyEquivocation VERIFIES using block hashes; if selection and
// verification ever call DIFFERENT hash-producing methods they can disagree about which
// pairs are even candidates. This is a SOURCE-LEVEL pin (a self-referential scan of
// equivocation.go, not a fixed name), because the defect it guards — two call sites
// silently drifting onto different functions — is invisible to any runtime input today's
// (or the fixed) Hash() can construct.
//
// SOURCE GATE: this reads equivocation.go as TEXT and can only see method NAMES and their
// COUNT per function; it cannot observe what those functions compute.
// RUNTIME GATE: TestFindEquivocationsNeverPairsAPrunedBlock (candidate selection through
// the real detection path) and TestVerifyEquivocationNotSatisfiableByStaleMemo (the
// verification side) observe the behaviour this pin protects.
func TestOneHashFunctionForEquivocationCandidatesAndVerification(t *testing.T) {
	src, err := os.ReadFile("equivocation.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	verifyBody := funcBody(t, text, "func CheckEquivocation") // VerifyEquivocation is a one-line wrapper over it
	findBody := funcBody(t, text, "func FindEquivocations")

	// Any call of the shape `<ident>.<Method>()` where Method contains "Hash" is treated as
	// a hash-producing call.
	hashCallRe := regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\(`)
	verifyMethods := methodNames(hashCallRe, verifyBody)
	findMethods := methodNames(hashCallRe, findBody)

	if len(verifyMethods) == 0 {
		t.Fatal("SOURCE GATE: fixture: CheckEquivocation calls no *Hash* method — G-6 cannot see the call it must pin; update the fixture if the function was refactored")
	}
	if len(findMethods) == 0 {
		t.Fatal("SOURCE GATE: fixture: FindEquivocations calls no *Hash* method — G-6 cannot see the call it must pin; update the fixture if the function was refactored")
	}
	if len(verifyMethods) != 1 || len(findMethods) != 1 {
		t.Fatalf("SOURCE GATE: G-6: expected exactly ONE hash-producing method name per function, "+
			"CheckEquivocation calls %v, FindEquivocations calls %v — a second distinct hash "+
			"entry point on this path is exactly what G-6 forbids", verifyMethods, findMethods)
	}
	if verifyMethods[0] != findMethods[0] {
		t.Fatalf("SOURCE GATE: G-6 VIOLATION: CheckEquivocation's verification and FindEquivocations' "+
			"candidate selection call DIFFERENT hash functions (%q vs %q) — they must call the "+
			"SAME function or they can disagree about which pairs are even candidates",
			verifyMethods[0], findMethods[0])
	}
}

// funcBody returns the source text from `marker` (a "func Name" prefix) up to (but not
// including) the next top-level "\nfunc " declaration, or EOF. A source-level text scan,
// not a Go parse — sufficient for a self-referential "do these two call the same method"
// pin.
func funcBody(t *testing.T, src, marker string) string {
	t.Helper()
	i := strings.Index(src, marker)
	if i < 0 {
		t.Fatalf("could not find %q in equivocation.go — G-6 fixture is stale, update the marker", marker)
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "\nfunc "); j >= 0 {
		return rest[:j+1]
	}
	return rest
}

// methodNames returns the distinct method-call names in body matched by re, RESTRICTED to
// names that look hash-producing ("Hash" case-insensitively) — a hand-rolled Go-source
// method-call regex cannot itself embed a case-insensitive "contains" test without eating
// the very characters it needs to match twice, so the filter is a separate step.
func methodNames(re *regexp.Regexp, body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		if !strings.Contains(strings.ToLower(m[1]), "hash") {
			continue
		}
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}
