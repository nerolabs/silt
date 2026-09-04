package chain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// O3 DIRECTION T — GATE (d): R-I5-TEXT-AND-CLAIMS-LEDGER
// =============================================================================
//
// Binding spec: O3-Direction-T-I5-restatement-and-divergence-RESEARCH-CERTIFICATION-2026-09-04.md
// §3 (the I5 replacement text), §6 (the closure-table rows), §7 (claims-ledger.md:47), §8.4 (pass
// conditions). Scar: scar-invariant-statement-contradicts-assert — I5's Statement named a term
// (weight) that its own Assert forbids, and the term had been 0 on every real certificate since
// 2026-08-16.
//
// What is pinned (key sentences, not the whole block — the cert's §3 text is the source; a
// re-wording that keeps these sentences is fine, a re-wording that drops one is not):
//
//	1. I5 Statement: height -> head-hash; the weight term is DELIBERATELY ABSENT; certificates are
//	   validity inputs, never transition or fork-choice inputs.
//	2. I5 Assert: the purity pin, the certificate-variant oracle, the fast/slow equivalence, the
//	   verifier-inventory pin — AND the accountable-safety half preserved verbatim.
//	3. The #357 scar line re-worded (§3.4); the R0.6 scar line UNCHANGED (§3.5).
//	4. Governs cites `heavier` (height -> head-hash).
//	5. The two closure-table rows (§6).
//	6. `weight → height → hash` appears nowhere in docs/ outside the frozen-history set.
//	7. claims-ledger.md's objective fork-choice row equals §7 exactly.
//
// RED at 59509b1 (I5 still says weight → height → hash; the ledger row still names
// the pre-T legacy-fixture witness). GREEN after the T commit's doc edits.
//
// SOURCE GATE: this test reads Markdown under docs/. It sees strings, not behaviour.
// RUNTIME GATE: TestO3T_CertificateVariantNeverRanksHeavier and TestO3T_HeavierReadsOnlyHeightAndHeadHash
// cover the property the canon text describes.

// o3tI5StatementSentences are the cert §3.2 sentences the Statement must carry.
var o3tI5StatementSentences = []string{
	"Fork-choice is a **deterministic total order** (**height → head-hash**), evaluated **only over descendants of the latest finalized block.**",
	"head selection is a pure function of the committed chain and of nothing a replica holds privately",
	"**A weight term is deliberately absent** (owner-ratified 2026-09-03, O3 Direction T).",
	"Certificates (`Atts`, `PrepareQC`, `CommitRound`) are **validity** inputs, never **transition** or **fork-choice** inputs.",
	"Two replicas holding the same committed chain and *different but valid* certificates for its blocks must compute identical accepted state and select the identical head.",
	"**And the system has accountable safety**: if two conflicting blocks ever finalize, it is always attributable to a slashable ≥ ⅓ — an **honest** validator is *never* slashed.",
}

// o3tI5AssertSentences are the cert §3.3 sentences the Assert must carry.
var o3tI5AssertSentences = []string{
	"fork-choice is a **pure function** (replay determinism — same inputs → same head on every replica)",
	"`TestModelCheck_357_ForkChoiceIsOrderIndependent`",
	"the certificate-variant oracle (two replicas holding the same committed chain and *different but valid* certificates",
	"the fast-path/slow-path equivalence (`appendExtension` and `reconstructFork → Reconcile` yield the same head on the same served window)",
	"**`heavier` reads no field outside `Height` and the head `Hash()`** (the fork-choice purity pin, AST-walked, with a teeth test)",
	"**no attestation is verified in non-test `core/chain` outside `verifyAtt` and the era-1-gated `signedBlock`** (the #558 verifier-inventory pin, with a teeth test)",
}

// o3tI5AccountableSafetyVerbatim is the accountable-safety half of the Assert at 59509b1, which
// the cert says stays verbatim (§3.3: "from 'and **no honest schedule**…' to the end").
const o3tI5AccountableSafetyVerbatim = "**no honest schedule ever produces a slash** (the accountable-safety oracle — this is the direct catch for #397). **The accountable-safety oracle must vary the DECLARED height away from the SIGNED height, set `Pruned` to {unset, the real hash, another block's hash} on each side, and cover era 1 as well as era 2** (`TestModelCheck_I5_CrossHeightPrunedExtension_{Era1,Era2}`); the write-path gates drive the real `Append` (`core/chain/r06_i5_evidence_recompute_test.go`, `core/node/r06_i5_evidence_recompute_test.go`)."

// o3tR06ScarLineVerbatim is the R0.6 scar bullet at 59509b1 (consensus-invariants.md:167). T
// touches no byte of it (cert §3.5).
const o3tR06ScarLineVerbatim = "- **R0.6 (2026-09-03) — the cross-height `Pruned` slash forgery.** The accountability predicate **quantified over a fact (`Height`) outside the signed message**: `VerifyEquivocation` read the height from a struct field but the signed digest from `Hash()`, which returns the accuser-supplied `Pruned` for the two evidence blocks. Two genuine signatures at two different heights, re-labelled with one height, convicted an honest validator through `Append` — in era 1 and era 2 — and a Byzantine *peer* sufficed, because an honest node queued the forgery on detection. The exhaustive I5 oracle never saw it: it fuzzed one height, never set `Pruned`, and was era-2-only. Fix: evidence hashes are always recomputed from the body and a pruned evidence block is refused (`D-F2-EVIDENCE-RECOMPUTE`)."

// o3t357ScarRewordKey is the load-bearing clause of the cert §3.4 re-worded #357 line.
const o3t357ScarRewordKey = "went inert with the era-2 signature change (#432, 2026-08-16) and is retired (O3 Direction T)"

// o3tClosureRows are the two cert §6 closure-table rows.
var o3tClosureRows = []string{
	"| R-BOX-ATTESTS uncovered-certificate transition | I5 (transition read a non-hash-covered field) + I4 (operation-liveness: intermittent stall) |",
	"| dead fork-choice weight (#558, third site) | I5 (Statement named a term the Assert forbids; the term was inert on every real certificate) |",
}

// o3tLedgerRow47 is the cert §7 replacement row for docs/design/claims-ledger.md:47.
const o3tLedgerRow47 = "| **Objective fork-choice converges** a partition onto one history: a sub-quorum minority cannot commit (intersecting quorum, I1), stalls, and catches up to the majority's head — selected by height → head-hash among descendants of the finalized head | `TestObjectiveConsensusCommitsOverTCP`, `TestRedteamF6_ObjectiveForkChoiceConvergesByCatchUp`, `TestModelCheck_357_NoReorgOfFinalizedLaunchBlock` |"

// o3tLedgerRow47Old is the row at 59509b1 that must be GONE (it names a mechanism false since
// 2026-08-16 and a legacy-config unit fixture as an "objective" witness).
const o3tLedgerRow47Old = "| **Objective fork-choice heals** a partition to the heavier-standing chain |"

func o3tRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, sErr := os.Stat(filepath.Join(root, "go.mod")); sErr != nil {
		t.Fatalf("SOURCE GATE: repo root not found at %s: %v", root, sErr)
	}
	return root
}

// o3tI5Block returns the I5 section of consensus-invariants.md: from the `## I5` heading to the
// next `---` rule.
func o3tI5Block(t *testing.T, doc string) string {
	t.Helper()
	i := strings.Index(doc, "\n## I5 ")
	if i < 0 {
		t.Fatal("SOURCE GATE: no `## I5 ` heading in docs/design/consensus-invariants.md")
	}
	rest := doc[i+1:]
	j := strings.Index(rest, "\n---")
	if j < 0 {
		t.Fatal("SOURCE GATE: I5 block is not terminated by a `---` rule")
	}
	return rest[:j]
}

// TestO3T_CanonI5TextMatchesCertification is gate (d), parts 1-5.
func TestO3T_CanonI5TextMatchesCertification(t *testing.T) {
	root := o3tRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "docs", "design", "consensus-invariants.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	i5 := o3tI5Block(t, doc)

	var missing []string
	for _, s := range o3tI5StatementSentences {
		if !strings.Contains(i5, s) {
			missing = append(missing, "Statement: "+s)
		}
	}
	for _, s := range o3tI5AssertSentences {
		if !strings.Contains(i5, s) {
			missing = append(missing, "Assert: "+s)
		}
	}
	if !strings.Contains(i5, o3tI5AccountableSafetyVerbatim) {
		missing = append(missing, "Assert (accountable-safety half, must be VERBATIM): "+o3tI5AccountableSafetyVerbatim[:80]+"…")
	}
	if !strings.Contains(i5, o3tR06ScarLineVerbatim) {
		missing = append(missing, "Scar R0.6 (must be UNCHANGED, cert §3.5): "+o3tR06ScarLineVerbatim[:80]+"…")
	}
	if !strings.Contains(i5, o3t357ScarRewordKey) {
		missing = append(missing, "Scar #357 re-word (cert §3.4): "+o3t357ScarRewordKey)
	}
	if !strings.Contains(i5, "`heavier` (height → head-hash)") {
		missing = append(missing, "Governs: `heavier` (height → head-hash)")
	}
	if strings.Contains(i5, "weight → height → hash") {
		missing = append(missing, "Statement still names the retired order `weight → height → hash`")
	}
	for _, row := range o3tClosureRows {
		if !strings.Contains(doc, row) {
			missing = append(missing, "closure table row (cert §6): "+row)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("SOURCE GATE: docs/design/consensus-invariants.md I5 block does not carry the O3 Direction T canon text "+
			"(%d item(s) missing or wrong):\n  %s\n\n  Source of truth: the research cert §3.2-§3.6 and §6.", len(missing), strings.Join(missing, "\n  "))
	}
}

// TestO3T_NoWeightHeightHashOrderInDocs is gate (d) part 6: `grep -c 'weight → height → hash'`
// over docs/ is 0. Scope: docs/ minus the frozen-history set scripts/check_cited_tests.py also
// skips (docs/thinking/, docs/reviews/, docs/buildlog/, archive/) — dated records may quote the
// retired order (2026-09-04-o3-direction-t-design.md quotes the grep itself).
func TestO3T_NoWeightHeightHashOrderInDocs(t *testing.T) {
	root := o3tRepoRoot(t)
	var hits []string
	n := 0
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "thinking", "reviews", "buildlog", "archive":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		raw, rErr := os.ReadFile(path)
		if rErr != nil {
			return rErr
		}
		n++
		for ln, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "weight → height → hash") {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, rel+":"+itoa(ln+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 20 {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — only %d .md files walked under docs/ (41 at 59509b1 outside the frozen set)", n)
	}
	if len(hits) > 0 {
		t.Fatalf("SOURCE GATE: `weight → height → hash` count over docs/ (minus frozen history) is %d, want 0:\n  %s",
			len(hits), strings.Join(hits, "\n  "))
	}
}

// TestO3T_ClaimsLedgerForkChoiceRowMatchesCertification is gate (d) part 7.
func TestO3T_ClaimsLedgerForkChoiceRowMatchesCertification(t *testing.T) {
	root := o3tRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "docs", "design", "claims-ledger.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	if strings.Contains(doc, o3tLedgerRow47Old) {
		t.Fatalf("SOURCE GATE: docs/design/claims-ledger.md still carries the pre-T row %q — it names a mechanism "+
			"false since 2026-08-16 and a legacy-config unit fixture as an objective witness (cert §7).", o3tLedgerRow47Old)
	}
	if !strings.Contains(doc, o3tLedgerRow47) {
		t.Fatalf("SOURCE GATE: docs/design/claims-ledger.md does not carry the cert §7 row verbatim:\n  %s", o3tLedgerRow47)
	}
	// The three witnesses in the row must exist in this tree (scripts/check_claims.py enforces the
	// same in CI; asserting here keeps the gate self-contained).
	for _, name := range []string{"TestObjectiveConsensusCommitsOverTCP", "TestRedteamF6_ObjectiveForkChoiceConvergesByCatchUp", "TestModelCheck_357_NoReorgOfFinalizedLaunchBlock"} {
		if !o3tTestFuncExists(t, root, name) {
			t.Fatalf("SOURCE GATE: ledger witness %s does not exist as a `func %s(` in any *_test.go", name, name)
		}
	}
}

func o3tTestFuncExists(t *testing.T, root, name string) bool {
	t.Helper()
	found := false
	needle := "func " + name + "("
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "archive", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, rErr := os.ReadFile(path)
		if rErr == nil && strings.Contains(string(raw), needle) {
			found = true
		}
		return nil
	})
	return found
}
