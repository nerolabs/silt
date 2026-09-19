package depcheck

// THE REPO READS WITHOUT THE RECORD THAT PRODUCED IT.
//
// Comments here describe the product: what the code does, why this approach, what an invariant
// means, and citations to work a reader can actually reach. They never describe the DECISION
// PROCESS that produced the code — no lane, slice, phase, increment, milestone or build-note
// identifiers. The same rule binds identifiers, test names and file names.
//
// WHY IT IS A GATE AND NOT A STYLE NOTE. The party that certifies this project's central claim is
// an outside reader with the repository and nothing else. A comment that only parses if you also
// read a build record is, to that reader, noise that looks like meaning — it names a thing they
// cannot fetch, so they cannot tell whether it hides a reason or hides nothing. The rule was
// already written down and the source drifted anyway, because nothing ran. This runs.
//
// WHAT IT DOES NOT CLAIM TO CATCH. The vocabulary below is the part that can be matched without
// guessing. Bare letter-number tags (`D3`, `H-4`, `F1`) are deliberately NOT here: they collide
// with real domain names in this tree, and a gate that fires on a domain term teaches people to
// work around it. Their absence is a limit of this gate, not a permission.
//
// THE ALLOWLIST IS PART OF THE GATE. A term like "increment" is process vocabulary in one file and
// the product's own word in another — a micropayment really does advance by increments. Each
// exemption is an exact phrase with the reason it is domain language. An exemption that stops
// matching is a failure too: a dead exemption means the text moved and nobody re-read the rule.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// processTerm is one banned vocabulary item: what it is, how to find it, why it cannot be read as
// a domain term, and the exact phrases where it nonetheless is one.
type processTerm struct {
	name  string
	re    *regexp.Regexp
	why   string
	allow []string // exact substrings that are legitimate product language
}

var processVocabulary = []processTerm{
	{
		name: "build lane",
		re:   regexp.MustCompile(`lane-\d+ Part`),
		why:  "a numbered build lane. A bare `lane-0` is the credit plane's own serve lane and is not matched.",
	},
	{
		name: "sub-increment",
		re:   regexp.MustCompile(`(?i)\bsub-increment\b`),
		why:  "a unit of build sequencing.",
		allow: []string{
			// A relay pump's read buffer, sized below one authorized payment increment.
			"a sub-increment read buffer",
		},
	},
	{
		name: "build increment",
		re:   regexp.MustCompile(`(?i)\bincrement \d+[a-z]?\b`),
		why:  "a numbered step of the build.",
		allow: []string{
			// A hash-chain micropayment: the payer reveals the preimage at position N.
			"increment 5 by revealing x_5",
		},
	},
	{
		name: "build slice",
		re:   regexp.MustCompile(`\b[Ss]lice \d+[a-z]?\b`),
		why:  "a numbered unit of build scope. A Go slice is never written `slice 3`.",
	},
	{
		name: "build phase",
		re:   regexp.MustCompile(`\b(?:Phase|PHASE) \d+(?:\.\d+)?\b`),
		why:  "a numbered stage of the build.",
		allow: []string{
			// A test narrating its own two acts, not a build stage.
			"Phase 1: everyone publishes once on the grant",
		},
	},
	{
		name: "recompute sub-increment tag",
		re:   regexp.MustCompile(`\bP1-[a-z]\b`),
		why:  "a build tag for one step of the floor-box recompute.",
	},
	{
		name: "build note",
		re:   regexp.MustCompile(`\bBUILD notes?\b`),
		why:  "a pointer into a build record that is not in this repository.",
	},
	{
		name: "build direction",
		re:   regexp.MustCompile(`\b(?:DIRECTION|Direction) [AB]\b`),
		why:  "a named branch of a build decision. Lower-case prose (`in the direction a validator moves`) is not matched.",
	},
	{
		name: "retest identifier",
		re:   regexp.MustCompile(`\b[Rr]etest [A-Z]\d+\b`),
		why:  "a numbered item in a retest plan.",
	},
	{
		name: "issue or change-request reference",
		re:   regexp.MustCompile(`#\d{2,}`),
		why: "a number after a hash is an issue or pull-request in someone's tracker. Direction comes " +
			"from docs/VISION.md, docs/TENETS.md and the release-candidate list, and from nothing else: a " +
			"reader who has to open a tracker to understand a comment is reading a record this project " +
			"deliberately does not keep. The canon's own numbered items — the immutables, the don'ts, the " +
			"personas — are single digits or written out, so this pattern cannot reach them.",
	},
	{
		name: "milestone",
		re:   regexp.MustCompile(`(?i)\bmilestone\b`),
		why:  "a unit of build scheduling.",
	},
}

// goSources returns every .go file in the tree, test files included — the rule binds test names
// as much as source. archive/ is frozen history and is skipped by every gate in this tree.
func goSources(t *testing.T) []string {
	t.Helper()
	root := repoRootForProcessGate(t)
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "archive", "vendor", "node_modules", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		// This file names the banned vocabulary in order to ban it, and its teeth carry a
		// violating sample for every term. Reading itself would make the gate permanently red on
		// its own definition, so it is the one exemption that is structural rather than editorial.
		if strings.HasSuffix(path, ".go") && filepath.Base(path) != "no_process_identifiers_test.go" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if len(out) < 200 {
		t.Fatalf("GATE VACUOUS: the walk reached %d .go files, which cannot be this repository. "+
			"A gate that reads nothing passes for the wrong reason.", len(out))
	}
	return out
}

func repoRootForProcessGate(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	dir := wd
	for range 8 {
		if _, sErr := os.Stat(filepath.Join(dir, "go.mod")); sErr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("repo root not found above %s", wd)
	return ""
}

// TestNoProcessIdentifiersInSource is the rule. Every hit is reported with its file, line and the
// matched text, so clearing them is a reading task rather than a hunt.
func TestNoProcessIdentifiersInSource(t *testing.T) {
	files := goSources(t)
	usedAllow := map[string]bool{}

	var hits []string
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel := relForProcessGate(t, path)
		for i, line := range strings.Split(string(raw), "\n") {
			for _, term := range processVocabulary {
				m := term.re.FindString(line)
				if m == "" {
					continue
				}
				if exempt, phrase := allowedPhrase(line, term.allow); exempt {
					usedAllow[term.name+"|"+phrase] = true
					continue
				}
				hits = append(hits, rel+":"+itoa(i+1)+"  "+term.name+"  "+strings.TrimSpace(line))
			}
		}
	}

	// A dead exemption is a failure of its own: the text it excused has moved, and nobody
	// re-read the rule against what replaced it.
	for _, term := range processVocabulary {
		for _, phrase := range term.allow {
			if !usedAllow[term.name+"|"+phrase] {
				t.Errorf("DEAD EXEMPTION: %q was allowed for %q and no longer matches anything.\n"+
					"  The text it excused has changed. Re-read the new text against the rule and either\n"+
					"  drop the exemption or restate it — do not leave a standing permission for prose that\n"+
					"  is no longer there.", phrase, term.name)
			}
		}
	}

	if len(hits) == 0 {
		return
	}
	shown := hits
	if len(shown) > 40 {
		shown = shown[:40]
	}
	t.Fatalf("SOURCE GATE: %d process identifier(s) in Go source, want 0:\n  %s\n%s\n\n"+
		"  Comments describe the PRODUCT — what the code does, why this approach, what an invariant\n"+
		"  means — or cite work a reader can reach. They do not name the build's own lanes, slices,\n"+
		"  phases, increments or notes. To the outside reader who certifies this project, such a name\n"+
		"  points at something they cannot fetch: it looks like a reason and is not one.\n"+
		"  If a comment only makes sense to someone who read a record this repository does not have,\n"+
		"  delete the comment. If it carries a real reason, keep the reason and drop the identifier.\n"+
		"  A term that is genuinely product language belongs in that term's allow list, as an exact\n"+
		"  phrase with the reason — never as a widened pattern.",
		len(hits), strings.Join(shown, "\n  "),
		map[bool]string{true: "  … and " + itoa(len(hits)-len(shown)) + " more", false: ""}[len(hits) > len(shown)])
}

// TestEveryRepoDocumentCitedInSourceExists is the other half of the same rule, and the sharper
// half: a comment that cites a document this repository does not have is a dead pointer by
// construction, whatever vocabulary it uses.
func TestEveryRepoDocumentCitedInSourceExists(t *testing.T) {
	root := repoRootForProcessGate(t)
	cite := regexp.MustCompile(`\b(docs/[A-Za-z0-9_.-]+\.md)\b`)

	seen := map[string][]string{}
	for _, path := range goSources(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel := relForProcessGate(t, path)
		for i, line := range strings.Split(string(raw), "\n") {
			for _, m := range cite.FindAllStringSubmatch(line, -1) {
				seen[m[1]] = append(seen[m[1]], rel+":"+itoa(i+1))
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("GATE VACUOUS: no in-repo document citation found in any Go file. This tree cites " +
			"docs/TENETS.md and docs/VISION.md, so a zero here means the pattern stopped matching.")
	}

	var dangling []string
	for doc, sites := range seen {
		if _, err := os.Stat(filepath.Join(root, doc)); err == nil {
			continue
		}
		dangling = append(dangling, doc+"  cited at "+strings.Join(sites, ", "))
	}
	if len(dangling) > 0 {
		t.Fatalf("SOURCE GATE: %d cited document(s) do not exist in this repository:\n  %s\n\n"+
			"  Two documents govern this project and there are no others. A comment pointing at a third\n"+
			"  asks its reader to fetch something that is not there, which is exactly the shape this rule\n"+
			"  exists to remove. Keep whatever reason the comment carried and drop the pointer.",
			len(dangling), strings.Join(dangling, "\n  "))
	}
}

// TestNoProcessIdentifiersInFileNames binds the rule to file names, which the source gate above
// cannot see: a file called `slice3_recompute.go` names the build in a place no comment sweep
// reaches, and it is the first thing a reader sees.
func TestNoProcessIdentifiersInFileNames(t *testing.T) {
	var hits []string
	for _, path := range goSources(t) {
		base := filepath.Base(path)
		spaced := strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(base)
		for _, term := range processVocabulary {
			if term.re.MatchString(spaced) {
				hits = append(hits, relForProcessGate(t, path)+"  ("+term.name+")")
			}
		}
	}
	if len(hits) > 0 {
		t.Fatalf("SOURCE GATE: %d file name(s) carry a process identifier, want 0:\n  %s\n\n"+
			"  Rename them for what the code does.", len(hits), strings.Join(hits, "\n  "))
	}
}

// TestProcessIdentifierGateFiresOnItsOwnVocabulary is the teeth. Every banned term is fed a line
// that violates it and must match; every allowed phrase is fed back and must be excused. A gate
// nobody has watched fire is a gate nobody knows the shape of.
func TestProcessIdentifierGateFiresOnItsOwnVocabulary(t *testing.T) {
	samples := map[string]string{
		"build lane":                        "// era-4 recompute — lane-1 Part B core.",
		"sub-increment":                     "// the first sub-increment of the recompute.",
		"build increment":                   "// era-4 build increment 4c — the per-block rule.",
		"build slice":                       "// bounding resident payload (slice 2).",
		"build phase":                       "// the durability telemetry (Phase 2). Pure observability.",
		"recompute sub-increment tag":       "// class S (P1-b) reconstructs the digests.",
		"build note":                        "// the class-P Weight anchor, BUILD note D4.",
		"build direction":                   "// DIRECTION B: record the just-written regVersion.",
		"retest identifier":                 "// below the objective anti-release floor (retest G4).",
		"issue or change-request reference": "// a restart reuses it (#93). Say so.",
		"milestone":                         "// deferred to the next milestone.",
	}
	for _, term := range processVocabulary {
		line, ok := samples[term.name]
		if !ok {
			t.Fatalf("TEETH INCOMPLETE: %q has no sample line. Every banned term carries one, so a "+
				"pattern that silently stopped matching is caught here and not in review.", term.name)
		}
		if !term.re.MatchString(line) {
			t.Errorf("TEETH FAILED: %q did not match its own violating sample %q", term.name, line)
		}
		if exempt, _ := allowedPhrase(line, term.allow); exempt {
			t.Errorf("TEETH FAILED: %q excused its own violating sample %q", term.name, line)
		}
	}

	// The allow list must excuse what it claims to, or it is decoration.
	for _, term := range processVocabulary {
		for _, phrase := range term.allow {
			if !term.re.MatchString(phrase) {
				t.Errorf("TEETH FAILED: %q allows %q, but the pattern does not match it — the exemption "+
					"is inert and hides nothing.", term.name, phrase)
			}
			if exempt, _ := allowedPhrase("// "+phrase, term.allow); !exempt {
				t.Errorf("TEETH FAILED: %q does not excuse its own allowed phrase %q", term.name, phrase)
			}
		}
	}

	// And a clean line must pass every term, or the gate fires on ordinary product prose.
	clean := "// a validator re-proves its bond on a short clock or its standing lapses."
	for _, term := range processVocabulary {
		if term.re.MatchString(clean) {
			t.Errorf("TEETH FAILED: %q fired on ordinary product prose %q", term.name, clean)
		}
	}
}

// allowedPhrase reports whether a line carries one of a term's exempt phrases, and which.
func allowedPhrase(line string, allow []string) (bool, string) {
	for _, phrase := range allow {
		if strings.Contains(line, phrase) {
			return true, phrase
		}
	}
	return false, ""
}

func relForProcessGate(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel(repoRootForProcessGate(t), path)
	if err != nil {
		return path
	}
	return rel
}
