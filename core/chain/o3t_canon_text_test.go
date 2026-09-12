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

// o3tShippedText walks the SHIPPED PROSE of the whole repository and hands back (relative path,
// line number, line) for every line, PLUS a whitespace-flattened form of each file for the
// soft-wrap pass. `.md`, `.sh`, `.yml` and `.yaml`, because a claim is made in a README, in a
// harness banner and in a compose file's comment alike.
//
// WHY THE WHOLE TREE, not docs/. The walk under gate (d) part 6 covered `root/docs` only. The M0
// composition claim that named a bond term in fork choice lived in `README.md`, at the repo ROOT,
// and so did the claim-gating catalog line in `integration/run-all.sh`. A gate whose scope is a
// subdirectory of the thing it is about cannot fire on the loudest copy of the claim.
//
// SKIPS, and each is a frozen-history or derived surface, never a convenience:
//
//	thinking/ reviews/ buildlog/ archive/   dated records; scripts/check_cited_tests.py skips the
//	                                        same set. A record may QUOTE the retired wording.
//	buildlog.html                           the ONLY generated pages, skipped BY NAME rather than
//	changelog.html                          by directory (gen_buildlog.py, gen_changelog.py,
//	roadmap.html                            gen_roadmap.py). A hit in one of these IS a duplicate
//	                                        of a hit the walk already has in the .md source.
//
//	                                        The blanket `website/` DIRECTORY skip that used to sit
//	                                        here was wider than its own justification, which read
//	                                        "GENERATED from the .md sources". That is FALSE for
//	                                        `index.html`, `docs.html` and `node.html`: no
//	                                        generator writes them, and `git log` shows
//	                                        `ef9d041 "correct two public overclaims"` editing
//	                                        index.html by hand. Two of the three carried a retired
//	                                        claim on silt's public pages, behind a skip that said
//	                                        they could not. They are now WALKED.
//	CHANGELOG.md                            a dated record of what the claim USED to say. Same
//	                                        class as buildlog/, and it is 9k lines of it.
//	report-*.md                             cloudtest field reports; dated records of real runs.
//	.git .claude vendor node_modules        not shipped text.
//	this file                               it holds the banned strings as constants. INERT
//	                                        today — a `.go` file never survives the extension
//	                                        filter above — and kept only so that widening the
//	                                        filter to `.go` does not make the gate match itself.
func o3tShippedText(t *testing.T, root string) (lines []o3tLine, flat []o3tFlatFile, files int) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "archive", "vendor", "node_modules",
				"thinking", "reviews", "buildlog":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "CHANGELOG.md" || name == "o3t_canon_text_test.go" ||
			strings.HasPrefix(name, "report-") {
			return nil
		}
		// The three GENERATED website pages, by name. Everything else under website/ is
		// hand-maintained prose and is walked — see the skip table above.
		if name == "buildlog.html" || name == "changelog.html" || name == "roadmap.html" {
			return nil
		}
		// EXTENSIONS. `.html` was added 2026-09-12 in the same move that replaced the blanket
		// `website/` skip with a by-name skip of the three generated pages: silt's public
		// front page and docs page each carried a retired claim, and no gate could see them.
		// Admission MEASURED: the walk goes 164 -> 173 tracked files, admitting six product UI
		// pages under cmd/silt/ui/ and the three hand-maintained website pages, with ZERO new
		// false positives for any banned literal.
		//
		// `.yml`/`.yaml` was added the same day after a blind review measured the
		// hole: the repair that retired this vocabulary from integration/consensus/ and the
		// gate that was meant to police it drew their file-type scope from the SAME list, so
		// the gate was structurally incapable of finding what the repair missed —
		// `integration/consensus/docker-compose.yml:20` carried a banned literal and the
		// census called the set closed at 7. Admission was MEASURED, per this gate's own bar:
		// adding the two extensions takes the walk from 137 to 164 tracked files and the
		// `heavier-bonded` census from 7 to 8, with ZERO new false positives.
		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".sh") &&
			!strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") &&
			!strings.HasSuffix(path, ".html") {
			return nil
		}
		raw, rErr := os.ReadFile(path)
		if rErr != nil {
			return rErr
		}
		rel, _ := filepath.Rel(root, path)
		files++
		for i, line := range strings.Split(string(raw), "\n") {
			lines = append(lines, o3tLine{Path: rel, N: i + 1, Text: line})
		}
		flat = append(flat, o3tFlatten(rel, raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// ANTI-VACUITY. A walk that silently stops finding files reports a clean tree, which is the
	// failure this gate exists to prevent one layer down. TWO legs, and they are NOT equals —
	// which is itself the point, because an earlier revision of this comment presented the count
	// as an anti-vacuity floor and it is not one.
	//
	// LEG 1, THE COUNT, IS A TOTAL-COLLAPSE TRIPWIRE AND NOTHING FINER. A floor that cannot fire
	// is the defect this whole audit kept finding, so its reach is stated rather than implied.
	// Re-driven at 75c0f89 over this exact filter: 173 files, composed integration/ 101, docs/
	// 43, repo root 6, cmd/ 6, examples/ 6, .github/ 5, website/ 3, deploy/ 2, scripts/ 1. At
	// 100 it fires only if ~42 % of the walk vanishes. Drop docs/ ENTIRELY and the count is
	// 130 — GREEN, with every ratification-gated site outside the front page gone from the
	// walk. RAISING the floor does
	// not repair that: any value tight enough to notice one directory goes false-RED on ordinary
	// file churn, and a lint that cries wolf gets disabled — this gate's own admission bar. So
	// the count is kept only for what it genuinely catches (a root that does not resolve, a
	// WalkDir that returns early, an extension filter that admits nothing) and the binding work
	// moves to LEG 2.
	//
	// LEG 2, THE ANCHORS, binds, and it carries no number at all: a named file either survives
	// the walk or it does not, so there is no margin to drift and nothing to re-drive. One anchor
	// per walked AREA that holds the population this gate polices, and one per admitted
	// EXTENSION — the extension filter is exactly what was measured broken (integration/consensus/
	// docker-compose.yml carried a banned literal that no .md/.sh walk could ever reach), so it
	// gets a witness instead of a comment.
	if files < 100 {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — only %d shipped .md/.sh/.yml/.yaml/.html files "+
			"walked (173 measured on the TRACKED tree at 75c0f89 with the extensions above; 164 "+
			"without .html, 137 for .md/.sh alone, and an earlier revision of this line said 153, "+
			"which was never measured — re-drive the number when you change the filter, never carry "+
			"it forward).\n"+
			"  A WORKING COPY READS HIGHER than the tracked number if it carries untracked or "+
			"ignored .md/.sh/.yml/.yaml/.html under a walked path — re-driven 173 tracked vs 179 on "+
			"one author's disk, the six extras being integration/.run-all/report.md, "+
			"integration/cloudtest/report.html and four marketing/ notes. The TRACKED number is the "+
			"reference because CI walks a clean checkout; if you re-drive this on a laptop and get a "+
			"mismatch, that is why, and `git worktree add` reproduces the tracked figure.\n"+
			"  This floor is a TOTAL-COLLAPSE tripwire. It cannot see one directory leaving the "+
			"walk — that is what the anchors below are for.", files)
	}
	var missing []string
	for _, a := range o3tWalkAnchors {
		seen := false
		for _, l := range lines {
			if l.Path == filepath.FromSlash(a.Rel) {
				seen = true
				break
			}
		}
		if !seen {
			missing = append(missing, a.Rel+"  — "+a.Why)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — the walk reached %d file(s) but did not reach %d "+
			"ANCHOR(s):\n  %s\n\n"+
			"  Each anchor stands for an area or an extension whose silent loss this gate cannot "+
			"otherwise detect: the file count tolerates losing all of docs/ (164 -> 121, still "+
			"above the floor). If you narrowed the skip list, the extension filter or the root, "+
			"either restore the coverage or RETIRE the anchor deliberately and say why — do not "+
			"delete the line to go green.", files, len(missing), strings.Join(missing, "\n  "))
	}
	return lines, flat, files
}

// o3tWalkAnchors are the files whose PRESENCE in o3tShippedText's walk is asserted. See LEG 2 in
// o3tShippedText: this is the anti-vacuity leg that actually binds, because it is structural — a
// named path is in the walk or it is not — where the file COUNT tolerates a 39 % loss.
//
// EACH ENTRY IS EVIDENCE-DRIVEN, not a sample. The set covers every walked AREA that holds a site
// of the vocabulary this gate polices, plus one witness per admitted EXTENSION.
//
// ABLATED, one leg at a time, 2026-09-12 — and the SECOND column is the point, because it is the
// discrimination the file count alone could never make. Each ablation removes the file from the
// WALK (not the anchor from this list, which would prove nothing) and the gate is re-run:
//
//	ablation                                  files walked   count floor   anchor leg
//	drop docs/ from the skip switch                    130   GREEN         RED  docs/threat-model.md
//	drop website/ from the skip switch                 170   GREEN         RED  website/index.html
//	drop the .html extension                           164   GREEN         RED  website/index.html
//	drop the .yml/.yaml extension                      146   GREEN         RED  …/docker-compose.yml
//	skip every file named README.md                    146   GREEN         RED  README.md
//	skip every file named run-all.sh                   172   GREEN         RED  integration/run-all.sh
//	drop integration/ from the skip switch              72   RED           (not reached)
//
// Read the first SIX rows as the finding: in every one of them the COUNT FLOOR IS GREEN and the
// gate is red only because an anchor is missing. Rows 2-4 are this change's own subject matter —
// the website/ skip and the .html and .yml extensions — and the floor notices none of them; row 4
// is the exact 2026-09-12 regression that started this work, a .yml file carrying a banned literal
// the walk could not reach. The last row is honest about its own limit: losing all of integration/
// is a collapse big enough that the tripwire fires first and returns before the anchor loop runs,
// so that run does not demonstrate its anchor; row 6 does, in isolation.
var o3tWalkAnchors = []struct{ Rel, Why string }{
	{"README.md",
		"the repo ROOT, and the front-door site: the sweep this gate replaced walked docs/ only, " +
			"so the claim sat in README.md for months with a gate in the tree that looked like it covered it"},
	{"integration/run-all.sh",
		"integration/ (101 of the 173 walked files at 75c0f89) AND the .sh extension"},
	{"docs/threat-model.md",
		"docs/ (43 of 173). Measured: dropping docs/ leaves the count at 130, above the floor, " +
			"and neither original anchor was under docs/ — so the entire ratification-gated repair " +
			"set outside the front page could leave the walk with every leg still green"},
	{"integration/consensus/docker-compose.yml",
		"the .yml extension, which is not decoration: this exact file carried a banned literal at " +
			"75c0f89 that the .md/.sh-only walk was structurally incapable of reaching"},
	{"website/index.html",
		"the .html extension AND the website/ directory, both admitted by this change. Until " +
			"2026-09-12 website/ was skipped wholesale on a reason that was FALSE for three of " +
			"its six pages, and this file carried the retired claim on silt's public front page " +
			"while every gate in the tree stayed green. If the blanket skip ever comes back, this " +
			"anchor is what says so"},
}

type o3tLine struct {
	Path string
	N    int
	Text string
}

// o3tFlatFile is one file with every run of whitespace — SPACES, TABS AND NEWLINES — collapsed to
// a single space, plus a map from each byte of that flattened text back to its source line.
//
// WHY: the per-line scan is defeated by a SOFT LINE WRAP, and not hypothetically. A blind review
// executed it: the retired claim restored verbatim with a newline between `heavier-standing` and
// `chain` renders identically in Markdown and left every gate in this file GREEN. README and the
// harness docs are soft-wrapped prose throughout, and the PR that retired this vocabulary re-wrapped
// that exact paragraph, so the evasion needs no intent at all — a normal re-flow disarms the gate.
// Flattening closes it for every wrap depth, not just a two-line one. MEASURED on the tracked tree
// at 75c0f89: the flattened scan finds the same 4 and 8 hits as the per-line scan and ZERO extra, so
// the closure costs no false positives.
type o3tFlatFile struct {
	Path string
	Text string // whitespace-collapsed
	Line []int  // Line[i] is the source line number of Text[i]
}

func o3tFlatten(rel string, raw []byte) o3tFlatFile {
	f := o3tFlatFile{Path: rel}
	var b strings.Builder
	line, inSpace := 1, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		at := line
		if c == '\n' {
			line++
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			// A separator is emitted once per run, and only after real text, so the
			// flattened form never opens with a space and never doubles one.
			if !inSpace && b.Len() > 0 {
				b.WriteByte(' ')
				f.Line = append(f.Line, at)
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteByte(c)
		f.Line = append(f.Line, at)
	}
	f.Text = b.String()
	return f
}

// o3tRetiredForkChoiceVocabulary is the CLOSED set of phrases that assert a bond or weight term in
// fork choice. Fork choice ranks on Height then head hash and reads nothing else (`heavier`, pinned
// by TestO3T_HeavierReadsOnlyHeightAndHeadHash); with the finality gate on, Reconcile admits only
// forks containing the committed head, so a sub-quorum partition commits nothing and CATCHES UP
// rather than reorging. Both phrases below assert the opposite.
//
// EACH ENTRY IS MEASURED, and that is the admission bar — a lint that cries wolf gets disabled, so
// the false-positive rate is a correctness property of this gate. Re-driven 2026-09-12 over the
// tracked tree at 75c0f89, with this walk's extensions:
//
//	"heavier-bonded"  8 hits, 8 of 8 ARE the retired claim: integration/consensus/run.sh x5,
//	                  integration/consensus/README.md x2, and integration/consensus/docker-compose.yml
//	                  x1. Closed. It was booked as "7 hits, 7 of 7 … Closed" before a blind review
//	                  measured it: the eighth was real, unrepaired, and sat outside the extension
//	                  filter. A census is a CLAIM — re-drive it, never carry it forward.
//
//	"heavier-standing chain"  4 hits, 4 of 4 ARE the retired claim (README.md,
//	                          docs/threat-catalog.md, docs/risk-register.md,
//	                          docs/math/08-quorum-chains.md). Closed. THIS ENTRY IS
//	                          RATIFICATION-GATED: its replacement is a research-certified
//	                          PUBLISHED CLAIM (see TestO3T_CertifiedForkChoiceSentenceIsPresent
//	                          below), so this line and that test land together or not at all.
//	                          The companion "heavier-bonded" entry is harness text and needed
//	                          no ratification; it landed separately, ahead of this.
//
//	"on-chain-bond fork-choice"  13 hits AT BASE (75c0f89) IN THIS WALK, 13 of 13 ARE the retired
//	                          claim. Found by CENSUS while severing this work — NOT by the gate,
//	                          because it is a different phrase from the two above. It locates the
//	                          objectivity in the RANKING; the bond decides who may participate
//	                          and what counts toward quorum, never how two candidate heads are
//	                          ordered. Ratification-gated, so this entry lands here.
//
//	                          NAME THE TREE. This entry previously read "7 hits when this was
//	                          measured … One hit was harness text (run.sh:5) … the remaining six
//	                          are docs/threat-model.md x4 and silt's public index.html and
//	                          docs.html". Every part of that was wrong, and re-driving it is the
//	                          only reason we know:
//	                            - 7 is the count on the HARNESS-HALF tree, not at base. At base
//	                              in this walk it is 13; in the harness half's narrower walk
//	                              (no website/, no .html) it is 11. A census with no tree named
//	                              is not re-drivable, so all three are given.
//	                            - the 1 + 6 split is wrong for EVERY tree. SIX hits at base were
//	                              harness text, not one: integration/README.md:164,
//	                              integration/run-all.sh:30, integration/consensus/README.md:7
//	                              and :92, integration/consensus/docker-compose.yml:16, and
//	                              integration/consensus/run.sh:5.
//	                            - it OMITTED README.md:43 — the front-door site, the loudest
//	                              one, and the reason TestO3T_CertifiedForkChoiceSentenceIsPresent
//	                              exists. 1 + 6 = 7 balanced only because the harness count was
//	                              collapsed to one and the README was dropped.
//	                          The seven live at the harness half's head, all repaired HERE:
//	                          README.md:43, website/index.html:173, website/docs.html:34, and
//	                          docs/threat-model.md x4. Re-driven at this head: 0 remain.
//
// DELIBERATELY NOT BANNED, and this is the gate's honest coverage limit. BOTH figures below were
// RE-DRIVEN 2026-09-12 over THIS walk (173 files) at 75c0f89, after a blind review measured that
// the two censuses this gate REASONED about were wrong while the two it re-drove were exact.
// Both counts are also driven over the harness half's narrower 164-file walk and are IDENTICAL
// there, so neither figure depends on which of the two scopes you stand in:
//
//	"heavier chain"  5 hits, only 2 of which are the claim (integration/README.md:164 and
//	                 integration/run-all.sh:30 — both repaired by text in the harness half). The
//	                 other
//	                 3 describe a genuinely TALLER chain in harness mechanics
//	                 (integration/cloudtest/scenarios.sh:820,837 "DRIVE the majority to commit a
//	                 heavier chain" and a height comparison; integration/consensus/run.sh:225
//	                 "valA held its OWN heavier chain byte-for-byte") — true statements. Banning
//	                 it would false-flag 3 of 5. The line above previously read "7 hits … the
//	                 other 5 … false-flag 5 of 7"; no scope reproduces 7 (.md/.sh: 5; +.yml: 5;
//	                 whole tree: 22). The CONCLUSION survives the correction — the majority of
//	                 hits are true statements — but the arithmetic that justified it did not.
//	"heavier fork"   24 hits on 23 distinct lines (one line carries it twice). NOT "mostly legacy-
//	                 posture tests": this walk admits NO .go file at all, so none of the hits is a
//	                 test. They are harness prose, compose files and shell —
//	                 integration/consensus/ x14, integration/redteam/ x5, integration/adversarial/
//	                 x2, integration/cloudtest/ x1, docs/ x2. The line above previously read "17
//	                 hits, mostly legacy-posture tests and the daemon's own narration history": 17
//	                 was correct for the RETIRED .md/.sh 137-file scope and went stale when the
//	                 filter widened, and the characterisation was drawn from files this walk has
//	                 never been able to see. THE 24 ARE NOT ALL TRUE STATEMENTS and this entry
//	                 does not claim they are: integration/adversarial/README.md:9 says "on heal,
//	                 the lighter side reorgs onto the heavier fork", which is the retired claim.
//	                 Classifying all 24 was not done here and the phrase stays unbanned on the
//	                 strength of the true uses above it, not on a measured false-positive rate.
//	                 Named so the next reader inherits the open edge rather than a count.
//
// So integration/run-all.sh's catalog line and integration/README.md's suite row were repaired by
// TEXT and are NOT covered by this gate. A future re-introduction in those exact words would not
// fire. Said plainly rather than left for a reader to discover.
//
// ============================================================================================
// WHAT THIS GATE CANNOT DO. Two evasions were EXECUTED against it by a blind reviewer, both
// leaving every canon-text test in this file green. One is closed; the other is not closable by a lexical rule
// and is recorded here so the next reader inherits the limit instead of an impression of
// completeness.
//
//	CLOSED — SOFT LINE WRAP. The retired claim restored verbatim with a newline between
//	  `heavier-standing` and `chain` renders identically in Markdown and defeated a per-line
//	  match. Closed by the flattened pass (o3tFlatten): every whitespace run, newlines included,
//	  collapses to one space before the match, so no wrap depth evades it. Driven RED in
//	  TestO3T_SoftWrapDoesNotDisarmTheCanonTextGates.
//
//	NOT CLOSED — PARAPHRASE. "the chain carrying the most bonded standing" states the retired
//	  claim and this gate is silent, because the gate matches LITERALS and a paraphrase is not one.
//	  Widening to a semantic rule is not available to a text lint, and widening the literal set is
//	  what the "heavier chain" measurement above already rejects: it false-flags 5 of 7. So this
//	  gate stops a REGRESSION to the exact retired wording and stops a re-wrap of it. It does NOT
//	  stop a rewrite of the claim in new words, and nothing in this tree does. The cover for that
//	  is review, plus the runtime gates named on each test — not this file. Do not describe this
//	  gate as covering "the claim"; it covers a vocabulary.
//
//	ALSO NOT COVERED, by construction: any file outside the extension filter (.go, .py, .tf,
//	  .json, Makefile, Dockerfile, extensionless) and any directory in the skip list above.
//	  website/ was in that sentence until 2026-09-12; its three hand-maintained pages are now
//	  WALKED, and the two that carried the claim (website/index.html, website/docs.html) are
//	  repaired here. Re-driven at this head: 0 hits of any banned literal tree-wide.
//
//	  THE .go POPULATION IS REAL, IS ONLY PARTLY REPAIRED, AND IS NOT GATED AT ALL. An earlier
//	  revision of this line said it "is repaired by text in this same change", which asserted
//	  more than the tree supports; a blind review named live sites inside it. The population is
//	  the research certification's own residual R-6. Re-driven at this head:
//
//	    REPAIRED by text here — core/chain/redteam_consensus_test.go:11,132;
//	      core/chain/redteam_f7_test.go (two comment sites); sim/objective_consensus_test.go
//	      (one comment site and one failure string); core/node/partition.go (SetBlockedPeers'
//	      doc comment, PRODUCTION source); sim/reorg_test.go (the header, the fixture's stated
//	      operating theory, two heal comments and two failure strings).
//	    NOT REPAIRED, deliberately, each for its own reason —
//	      cmd/silt/daemon.go:1299, the operator narration "chain: reorged onto a heavier fork".
//	        integration/consensus/run.sh greps for that exact string, so editing it would fail a
//	        Docker-gated harness OPEN that nobody has re-run at this commit. It is also still
//	        REACHABLE: the finality gate makes dropped > 0 impossible only while
//	        finalityQuorumActive(), and a legacy-posture chain has no gate. Residual.
//	      cmd/silt/daemon.go:154, the -equivocate flag help, which scopes itself to "LEGACY
//	        mode" and is therefore accurate as written.
//	      sim/reorg_test.go's test NAME, TestPartitionHealsToHeavierFork. Its comments are
//	        corrected; renaming it is a cited-test surface and is left as a residual rather than
//	        bundled into a text repair.
//
//	  Nothing above is gated by anything. A .go regression of this vocabulary goes unnoticed.
//
// ============================================================================================
var o3tRetiredForkChoiceVocabulary = []string{
	"heavier-bonded",
	"heavier-standing chain",
	"on-chain-bond fork-choice",
}

// TestO3T_NoRetiredForkChoiceClaimInShippedText bans the retired bond/weight fork-choice vocabulary
// across the WHOLE shipped tree.
//
// WHY IT EXISTS. `o3tLedgerRow47Old` below bans the claims-ledger's table row, but that check opens
// ONE file (docs/design/claims-ledger.md) and the banned string is a full table row — it could never
// match README.md's prose form of the same claim. The claim therefore sat in the repo's front door
// for months with a gate in the tree that looked like it covered it. This is that gate.
//
// Certified replacement wording: research certification
// README-BOND-FORKCHOICE-literal-claim-and-equivalence-RESEARCH-CERTIFICATION-2026-09-12 §4.4/§7.
// It is a PUBLISHED CLAIM. Do not reword it to make this test pass — that re-opens the certification.
//
// SOURCE GATE: this reads text. RUNTIME GATE: TestO3T_HeavierReadsOnlyHeightAndHeadHash pins that
// `heavier` reads only Height and the head Hash(); e2e/partition_test.go drives the catch-up-not-reorg
// behaviour the replacement sentence describes.
func TestO3T_NoRetiredForkChoiceClaimInShippedText(t *testing.T) {
	_, flat, files := o3tShippedText(t, o3tRepoRoot(t))
	hits := o3tBannedHits(flat, o3tRetiredForkChoiceVocabulary)
	if len(hits) > 0 {
		t.Fatalf("SOURCE GATE: %d line(s) across %d shipped file(s) assert a bond or weight term in fork choice, want 0:\n  %s\n\n"+
			"  Fork choice reads Height then head hash and NOTHING else, and with the finality gate on a sub-quorum\n"+
			"  partition commits nothing, stalls, and CATCHES UP — it does not reorg onto anything.\n"+
			"  REMEDY: use the certified replacement sentence from README-BOND-FORKCHOICE-literal-claim-and-\n"+
			"  equivalence-RESEARCH-CERTIFICATION-2026-09-12 §7, VERBATIM where the site states the composition\n"+
			"  claim; where the site states a narrower property, make that property true in its own terms.\n"+
			"  It is a PUBLISHED CLAIM: rewording it silently re-opens the certification.",
			len(hits), files, strings.Join(hits, "\n  "))
	}
}

// o3tBannedHits reports every banned literal in the FLATTENED text of each file, as
// `path:line  (literal)` where line is the source line the match STARTS on. Flattened, so a soft
// line wrap cannot hide a phrase — see o3tFlatFile.
func o3tBannedHits(flat []o3tFlatFile, banned []string) []string {
	var hits []string
	for _, f := range flat {
		for _, b := range banned {
			for from := 0; ; {
				k := strings.Index(f.Text[from:], b)
				if k < 0 {
					break
				}
				at := from + k
				hits = append(hits, f.Path+":"+itoa(f.Line[at])+"  ("+b+")")
				from = at + 1
			}
		}
	}
	return hits
}

// TestO3T_NoWeightHeightHashOrderInDocs is gate (d) part 6: the retired ranking order
// `weight → height → hash` appears nowhere in shipped text. WIDENED 2026-09-12 from `root/docs`
// to the whole tree (o3tShippedText) — the old scope could not see README.md — and matched over
// the flattened text, so a wrap between `weight →` and `height` does not hide it either.
func TestO3T_NoWeightHeightHashOrderInDocs(t *testing.T) {
	_, flat, files := o3tShippedText(t, o3tRepoRoot(t))
	hits := o3tBannedHits(flat, []string{"weight → height → hash"})
	if len(hits) > 0 {
		t.Fatalf("SOURCE GATE: `weight → height → hash` count over %d shipped file(s) is %d, want 0:\n  %s",
			files, len(hits), strings.Join(hits, "\n  "))
	}
}

// TestO3T_SoftWrapDoesNotDisarmTheCanonTextGates drives the CLOSED evasion, because a coverage
// claim that is not driven is the thing this whole gate exists to distrust.
//
// WHICH LITERALS THE CLOSURE ACTUALLY PROTECTS. A soft wrap breaks at a SPACE, so it can only
// split a MULTI-WORD literal. `weight → height → hash` — live in
// TestO3T_NoWeightHeightHashOrderInDocs — is one, and so is `heavier-standing chain`, the
// published-claim literal held for ratification. `heavier-bonded` is a single token and cannot be
// wrap-split at all; it is driven here as the CONTROL that says so, rather than left to imply a
// protection it does not need. Flattening is therefore load-bearing TODAY, not in anticipation.
func TestO3T_SoftWrapDoesNotDisarmTheCanonTextGates(t *testing.T) {
	const multi = "weight → height → hash"
	const single = "heavier-bonded"
	for _, tc := range []struct {
		name, banned, body string
		want               bool
	}{
		{"multi-word, same line", multi, "ranked by weight → height → hash today.\n", true},
		{"multi-word, SOFT WRAPPED", multi, "ranked by weight →\nheight → hash today.\n", true},
		{"multi-word, wrapped twice", multi, "ranked by weight\n→ height\n→ hash today.\n", true},
		{"multi-word, wrapped with list indentation", multi, "- ranked by weight → height\n      → hash today.\n", true},
		{"single token, same line", single, "converges to the heavier-bonded chain.\n", true},
		{"single token, wrap before it", single, "converges to the\nheavier-bonded chain.\n", true},
		{"CONTROL: the words apart, not the phrase", multi, "a weight.\n\nA height.\n\nA hash.\n", false},
		{"CONTROL: a hyphen split is NOT a soft wrap", single, "converges to the heavier-\nbonded chain.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := o3tFlatten("synthetic.md", []byte(tc.body))
			got := len(o3tBannedHits([]o3tFlatFile{f}, []string{tc.banned})) > 0
			if got != tc.want {
				t.Fatalf("SOURCE GATE: banned literal %q found=%v, want %v, in:\n%q\nflattened to:\n%q\n\n"+
					"  A per-line match is disarmed by a soft wrap and a blind review EXECUTED that evasion "+
					"against this gate — the retired claim restored verbatim with a newline in the middle, "+
					"every canon-text gate in this file green. The flattened pass is what closes it. If "+
					"this arm is red the "+
					"closure is gone and a normal re-flow of a paragraph silently re-opens the hole.",
					tc.banned, got, tc.want, tc.body, f.Text)
			}
		})
	}
	// The line map must point at the line the phrase STARTS on, or the failure text sends a reader
	// to the wrong place — which is how a true finding gets dismissed as noise. The phrase below
	// starts on line 3 and finishes on line 4.
	f := o3tFlatten("synthetic.md", []byte("one\ntwo\nranked by weight →\nheight → hash today\n"))
	hits := o3tBannedHits([]o3tFlatFile{f}, []string{multi})
	if len(hits) != 1 || !strings.Contains(hits[0], "synthetic.md:3") {
		t.Fatalf("SOURCE GATE: a wrapped hit reported %v, want exactly one hit at synthetic.md:3 — the "+
			"line the phrase STARTS on, not the line it ends on", hits)
	}
}

// o3tCertifiedForkChoiceSentence is the M0 composition claim's fork-choice clause, VERBATIM from
// research certification README-BOND-FORKCHOICE-literal-claim-and-equivalence-RESEARCH-
// CERTIFICATION-2026-09-12 §7 (the certification carries it at :214).
//
// DO NOT RE-WORD IT to make a test pass, to fix a typo, or to fit a line. It is a PUBLISHED CLAIM;
// changing the words re-opens the certification, and this constant exists precisely so that a
// silent re-wording fails a build instead of shipping.
const o3tCertifiedForkChoiceSentence = "objective, bond-weighted commit admission — a block " +
	"commits only on an intersecting super-quorum of a validator set the chain itself sizes " +
	"(a strict anchor majority at launch, >⅔ of the epoch's frozen on-chain bond once standing " +
	"is earned), so a sub-quorum partition commits nothing, stalls, and catches up to the " +
	"majority's history on heal rather than reorging onto it"

// TestO3T_CertifiedForkChoiceSentenceIsPresent is the half the ban set does not have.
//
// WHY. A ban asserts only a NEGATIVE: the retired wording is gone. Delete the replacement sentence
// outright and every banning gate in this file stays green, because nothing in the tree required it
// to be there. The claims-ledger gate next door asserts BOTH directions — old row absent AND new
// row present verbatim — and this is the same discipline applied to the front door.
//
// Matched over the FLATTENED text, so the sentence may be soft-wrapped in README.md however the
// prose needs it; what is pinned is the words and their order, not the line breaks.
func TestO3T_CertifiedForkChoiceSentenceIsPresent(t *testing.T) {
	root := o3tRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	f := o3tFlatten("README.md", raw)
	want := strings.Join(strings.Fields(o3tCertifiedForkChoiceSentence), " ")
	if !strings.Contains(f.Text, want) {
		t.Fatalf("SOURCE GATE: README.md does not carry the certified fork-choice sentence verbatim:\n\n  %s\n\n"+
			"  It is a PUBLISHED CLAIM from research certification README-BOND-FORKCHOICE-literal-claim-\n"+
			"  and-equivalence-RESEARCH-CERTIFICATION-2026-09-12 §7. Restore it word for word. Do NOT edit\n"+
			"  o3tCertifiedForkChoiceSentence to match the README — that inverts the gate and re-opens the\n"+
			"  certification silently, which is the exact failure this test exists to make impossible.\n"+
			"  Line breaks are free: the match runs over the whitespace-flattened file.", want)
	}
}

// TestO3T_TheTextGatesActuallyUseTheFlattenedPass closes the gap the arm above cannot.
//
// WHY BOTH ARMS EXIST. TestO3T_SoftWrapDoesNotDisarmTheCanonTextGates drives o3tFlatten and
// o3tBannedHits DIRECTLY, so it proves the helper is correct — and it stays GREEN if a future edit
// reverts either gate body to a per-line `strings.Contains(l.Text, …)` loop and simply stops
// CALLING the helper. Measured: reverting TestO3T_NoWeightHeightHashOrderInDocs to the per-line
// form leaves that arm green while a soft-wrapped banned phrase planted in a real walked file goes
// undetected. A helper nothing calls is not a closure. This arm asserts the WIRING.
func TestO3T_TheTextGatesActuallyUseTheFlattenedPass(t *testing.T) {
	root := o3tRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "core", "chain", "o3t_canon_text_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, fn := range []string{
		"TestO3T_NoRetiredForkChoiceClaimInShippedText",
		"TestO3T_NoWeightHeightHashOrderInDocs",
	} {
		head := "func " + fn + "(t *testing.T) {"
		i := strings.Index(src, head)
		if i < 0 {
			t.Fatalf("SOURCE GATE: %s is gone from this file — the text gate it names no longer exists", fn)
		}
		body := src[i:]
		if j := strings.Index(body, "\n}\n"); j >= 0 {
			body = body[:j]
		}
		if !strings.Contains(body, "o3tBannedHits(") {
			t.Errorf("SOURCE GATE: %s does not obtain its hits through o3tBannedHits — the FLATTENED "+
				"pass. A per-line match is disarmed by an ordinary soft line wrap (a blind reviewer "+
				"executed exactly that and every canon-text gate in this file stayed green), so a gate that stops "+
				"calling the flattened path has silently given the evasion back. Restore the call; do "+
				"not re-implement the loop here.", fn)
		}
		if strings.Contains(body, "l.Text") {
			t.Errorf("SOURCE GATE: %s iterates lines and matches on l.Text. That is the per-line form "+
				"the soft-wrap evasion defeats. Match over the flattened text via o3tBannedHits.", fn)
		}
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
