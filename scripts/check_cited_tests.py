#!/usr/bin/env python3
"""Doc/code-lint: catch a CITED TEST THAT DOES NOT EXIST — a green check that does
not verify the property it claims.

SCAR (third-time rule fired; count 5 as of 2026-09-02):
  A production comment in core/credit/delivery.go cited
  `TestPaidSerialWindowMatchesDemandWindow` as the test pinning the paid-serial
  window to the demand window. No such test has ever existed anywhere in the tree.
  A research certification then REPEATED the claim
  (silt-reviews/research/research-outcome/R0.4b-per-epoch-key-expiry-BUILD-
  VERIFICATION-RESEARCH-CERTIFICATION-2026-09-02.md), so the phantom laundered
  from a comment into a certification. Both read as "this property is verified".
  Neither was. This lint fails the build any time a Test name is cited in a place
  that asserts verification while no `func TestX(` backs it.

  This is the same family as check_claims.py, which already enforces the linkage
  for docs/design/claims-ledger.md ONLY. That narrow scope is exactly why the
  delivery.go comment and the certification both got through. This lint widens the
  net to Go comments and string literals, CHANGELOG, ROADMAP, docs/**, and the
  external review trees.

SECOND INSTANCE (2026-09-09) — WHY STRING LITERALS ARE IN SCOPE:
  The scan covered Go COMMENTS only, and said so in its own docstring. But two
  registries in this tree make their citations in STRING LITERALS, and both are
  exactly the "this property is verified by that test" shape:
    - core/chain/floorbox_coldauditor_v5_test.go's coverage meta-test, whose every
      undriven-field excuse row names the gate that covers the field;
    - cmd/silt/observable_contract.go's Asserter column, naming the e2e test that
      asserts each announced operator string.
  A blind PE renamed a cited test everywhere EXCEPT inside the map value; this lint
  reported zero in-repo hits and EXITED 0. The row survived only incidentally,
  because the name also sat in three '//' comments. Delete a test through its doc
  comment — the ordinary way a test disappears — and the row rots in silence, which
  is the precise hole those rows were written to close.

WHAT IS CHECKED
  A name matching \\bTest[A-Z][A-Za-z0-9_]*\\b, appearing in:
    - Go COMMENTS and STRING LITERALS (not bare code) under the source roots below
    - CHANGELOG.md, ROADMAP.md, docs/**/*.md
    - optionally the external certification/ruling trees (see --external-root)
  must resolve to a `func TestX(` declaration in some *_test.go in this repo.

SCOPE NOTES (deliberate limits)
  - METASYNTACTIC PLACEHOLDERS (TestFoo, TestXxx, ...) are never citations.
  - SUBTESTS ARE OUT OF SCOPE. A `t.Run("name", ...)` subtest is not a top-level
    func, so a citation naming only a subtest resolves only via its parent Test
    func (see FAMILY below). Cite the parent, or use the allowlist. Because string
    literals are now scanned, the FIRST argument of any `.Run(` call is masked, so
    a subtest literally named TestX is not read as a citation of one.
  - FAMILY citations resolve by PREFIX. `TestOpenBreak_*Locked...`,
    `TestFoo_{A,B}Bar` and `TestFoo_A/_B` name a family, not one func; a citation
    immediately followed by * { / ... or a unicode ellipsis resolves if ANY
    defined test starts with it.
  - SOFT-WRAPPED names resolve joined. This tree wraps long identifiers across
    comment lines, with or without a trailing hyphen ("TestStateRootCoversExactly-"
    / "TheCommittedSetFields"). A name ending a line is also tried joined with the
    leading word of the next line. Joining only ever removes false positives.
  - Markdown is scanned WHOLE, including fenced code blocks: a phantom cited
    inside a fence still reads to a human as a real test.
  - FROZEN HISTORY is excluded: /archive/, docs/thinking/, docs/reviews/,
    docs/buildlog/ — the same set check_status_headers.py skips. Those are dated
    point-in-time records, and docs/thinking/ deliberately proposes test names
    before the tests exist.
  - .claude/ is excluded and this is LOAD-BEARING: it holds agent worktrees, which
    are full copies of the repo on OTHER branches. Scanning them would let a
    phantom "resolve" against a test that exists only on an unmerged branch, which
    is precisely the unsoundness this lint exists to catch.

SCOPE NOTES FOR COORDINATES (deliberate limits, each measured before it was drawn)
  - AN UNANCHORED COORDINATE IS NOT CHECKED. `chain.go:1198` with no symbol beside
    it asserts nothing a machine can test, and guessing what a reader "meant" would
    be the vacuous-gate defect in a new costume. 134 of the 187 coordinates on the
    linted surface are unanchored today. Naming the symbol is what buys coverage;
    that is the behaviour this lint is trying to create, not a hole to paper over.
  - CHANGELOG.md is exempt from the COORDINATE check (its TEST-name checking is
    unchanged). A CHANGELOG entry is a DATED point-in-time record — the same reason
    docs/buildlog/ is skipped above. A coordinate in a released entry was true when
    written, and "correcting" it would falsify the record. 16 stale coordinates sit
    there and every one of them is correct history.
  - RANGES check their FIRST number only: `chain.go:3005-3013` is checked at 3005.
  - MARKDOWN ONLY. Coordinates inside Go comments are not scanned: the anchor is a
    BACKTICKED identifier, which is a markdown convention, and no coordinate defect
    has been observed in a Go comment. Widen when one is.
  - THE EXTERNAL REVIEW TREES ARE NOT COORDINATE-CHECKED, for the CHANGELOG reason
    at full strength. A ruling or certification is a review OF A NAMED SHA; its
    coordinates were true at that SHA and are not the current tree's to correct.
    Measured before the arm was dropped: 1844 stale coordinates across 304 review
    documents, back to the #286 and #357 rounds — an advisory nobody could read,
    naming nothing anyone should change. Their TEST-name citations stay advisory
    (a test name is not SHA-relative the way a line number is).
  - A BARE SYMBOL MENTION — a backticked identifier with no file and no coordinate
    — is NOT checked. Measured: 213 distinct backticked camelCase identifiers on
    this surface resolve to no declaration in the tree, over 497 occurrences,
    dominated by names that are HISTORICALLY CORRECT in the two append-only ledgers
    (docs/decisions.md, CHANGELOG.md) precisely because the symbol was later
    deleted. Gating that class would need a ~213-line allowlist of entries with no
    defect-catching power, which is what the allowlist header below forbids.

WIDENED 2026-09-10 — SYMBOL-ANCHORED SOURCE COORDINATES
  Same defect family, second carrier. A `path.go:NNN` coordinate in prose reads as
  "open this file at this line and you will see the thing I just named". Nothing
  checked that. Three coordinates in docs/decisions.md — two of them written
  "verified" — pointed at unrelated lines, because every insertion above a symbol
  moves it and nothing tells the doc:

    `consensusSigBytes`  cited chain.go:918   actually core/chain/chain.go:1067
    `bodyHash`           cited chain.go:822   actually core/chain/chain.go:902
    `SlashesEncodedSize` cited chain.go:2217  actually core/chain/chain.go:2372

  LINE NUMBERS ROT; SYMBOLS DO NOT. A renamed or deleted symbol goes loud — the
  compiler sees it, and this lint sees it. A shifted line number goes silent. So
  the resolution key is the SYMBOL, and a coordinate is checkable ONLY when it is
  tied to one:

    A `path.go:NNN` whose NEAREST PRECEDING backticked identifier is a symbol
    DECLARED in that file must land ON that symbol — inside its declaration (doc
    comment through closing brace) or on a line where the name literally occurs
    (±2 lines, for wrapped signatures). Otherwise the coordinate is rotten.

  The "or the name occurs at that line" arm is load-bearing: prose legitimately
  cites a CALL SITE, not only a declaration ("every non-test read of the flag:
  `core/node/chainrole.go:890`). Such a citation still points at the symbol, so it
  passes. What fails is a coordinate that points at neither — which is exactly what
  a decayed line number looks like.

STRICT vs ADVISORY
  - IN-REPO citations are STRICT: a phantom fails the build (exit 1).
  - EXTERNAL-TREE citations are ADVISORY by default: those trees are outside this
    repo, are not version-locked to it, and may legitimately cite a test that is
    real but sits on a branch not yet merged. Use --strict-external to fail on
    them too (that is how the scar above is reproduced).

Dependency-free (stdlib only).
Run: python3 scripts/check_cited_tests.py [--strict-external] [--external-root PATH]
Env: SILT_CITED_TESTS_EXTERNAL_ROOTS=path1:path2  (overrides the defaults)
"""
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ALLOWLIST = ROOT / "scripts" / "cited_tests_allowlist.txt"

SCAR_ID = "scar:cited-test-does-not-exist-2026-09-02"
COORD_SCAR_ID = "scar:cited-source-coordinate-decayed-2026-09-10"

# Go source roots whose COMMENTS are scanned for test citations.
GO_ROOTS = ["cmd", "core", "adapters", "ports", "sim", "integration", "e2e"]

# Markdown scanned in full.
MD_FILES = ["CHANGELOG.md", "ROADMAP.md"]
MD_ROOTS = ["docs"]

# Never walk into these directory names, anywhere.
#   .claude  — agent worktrees: full copies of the repo on other branches (see above)
#   archive  — frozen history, allowed to name removed tests
SKIP_DIRS = {".git", ".claude", "node_modules", "dist", "archive", "vendor", "__pycache__"}

# Frozen-history subtrees of docs/ — NOT linted. Same set check_status_headers.py
# already skips, for the same reason. docs/thinking/ records dated deliberations
# that PROPOSE test names before the tests are written (standing rule: record each
# deliberation, dated, shipped in the same PR); docs/reviews/ and docs/buildlog/
# are point-in-time records. Flagging a proposed name as a phantom is a category
# error, and "fixing" one would rewrite a historical record.
DOC_SKIP_DIRS = {"thinking", "buildlog", "reviews", "archive"}

# External certification / ruling trees. Read-only, outside the repo. Skipped
# silently when absent so CI stays hermetic.
DEFAULT_EXTERNAL_ROOTS = [
    "/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome",
    "/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer",
]

TEST_NAME_RE = re.compile(r"\bTest[A-Z][A-Za-z0-9_]*\b")
TEST_FUNC_RE = re.compile(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(", re.MULTILINE)

# --- symbol-anchored source coordinates -----------------------------------
# Markdown that is NOT coordinate-checked because it is dated history (see the
# scope notes). Its TEST-name checking is unchanged.
COORD_EXEMPT_MD = {"CHANGELOG.md"}

# `core/chain/chain.go:918`, chain.go:2217, `…/foo_test.go:79`. The optional
# leading backtick is consumed so an offset comparison against a preceding
# identifier stays honest.
COORD_RE = re.compile(r"`?((?:[A-Za-z0-9_./-]+/)?[A-Za-z0-9_.-]+\.go):(\d+)")

# The ANCHOR: a backticked identifier, optionally written call-style. Markdown
# in this tree spells every code reference this way.
TICKED_IDENT_RE = re.compile(r"`([A-Za-z_][A-Za-z0-9_]*)(?:\(\))?`")

# How far back from a coordinate an anchor may sit. 140 chars covers this tree's
# widest observed "`Sym` (`path.go:N`)" phrasing including a wrapped clause.
ANCHOR_WINDOW = 140

# Tolerance around a cited line when looking for the bare name. A signature
# wrapped over two lines puts the name up to two lines off the cited one.
NEAR_LINES = 2

# Top-level Go declaration forms. A struct FIELD and a const/var BLOCK MEMBER are
# included on purpose: docs cite `Slashes` and `bondRegHeight` as often as they
# cite a func, and the "or the name occurs at that line" arm keeps their many use
# sites from reading as failures.
DECL_RES = (
    re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*[\(\[]"),
    re.compile(r"^type\s+([A-Za-z_][A-Za-z0-9_]*)\b"),
    re.compile(r"^(?:const|var)\s+([A-Za-z_][A-Za-z0-9_]*)\b"),
    re.compile(r"^\t([A-Za-z_][A-Za-z0-9_]*)(?:,\s*[A-Za-z_][A-Za-z0-9_]*)*\s*(?:[A-Za-z_\[\*].*)?="),
    re.compile(r"^\t([A-Za-z_][A-Za-z0-9_]*)\s+[\*\[\]A-Za-z_]"),
)
BRACED_DECL = (0, 1)  # indices into DECL_RES whose bodies run to a column-0 close

# Metasyntactic placeholders. Prose that EXPLAINS test naming (this lint's own
# CHANGELOG entry, a design doc describing a convention) writes these to stand for
# "any test", never to cite one. Kept deliberately tiny: only names that could not
# plausibly be a real silt test.
PLACEHOLDER_NAMES = {
    "TestXxx", "TestXXX", "TestX", "TestY", "TestZ",
    "TestFoo", "TestBar", "TestBaz", "TestName", "TestSomething",
}

# A citation immediately followed by one of these names a FAMILY, not one func:
#   TestOpenBreak_*LockedInOldValuePredicate   TestFoo_{A,B}Bar   TestFoo_A/_B
FAMILY_MARKERS = ("*", "{", "/", "\u2026", "...")

# What may sit between the start of a wrapped continuation line and the rest of
# the identifier: Go comment markers, markdown list/quote lead-ins, backticks.
CONT_LEAD_RE = re.compile(r"^[\s/*>#`\-]*")


RUN_CALL_RE = re.compile(r"\.Run\(\s*$")


def go_citation_mask(src: str) -> str:
    """Return `src` with everything that is not a CITATION SITE replaced by spaces,
    newlines preserved, so regex offsets still map to real line numbers.

    A citation site is a Go COMMENT or the body of a STRING LITERAL. Both are read by a
    human as "this property is verified by that test"; neither is executed, so neither
    can go stale loudly on its own.

    STRING LITERALS WERE ADDED 2026-09-09, and the reason is a measured hole rather than
    a tidy-up. The floor box's coverage meta-test excuses each undriven Block field with a
    prose row naming the gate that covers it, and every row is a string literal in a map.
    A blind PE renamed a cited test everywhere except inside the map value: this lint
    reported zero in-repo hits and EXITED 0. The row was protected only incidentally,
    because the same name also appeared in three `//` comments — so deleting a test
    through its doc comment, which is how a test ordinarily disappears, would have left
    the row citing a phantom in silence. The same shape holds cmd/silt/observable_contract.go's
    Asserter column, which names the e2e test asserting each announced marker.
    Measured before widening: 62 citations in *_test.go literals across 23 files and 29 in
    observable_contract.go, with ZERO phantoms — the widening is exact today, and it is what
    makes those two registries rot loudly from here.

    A real (small) Go scanner is needed rather than a naive '//' split: a '//' inside a
    string would otherwise swallow the rest of the line, and the first argument of
    `t.Run(` must stay masked because SUBTEST NAMES ARE OUT OF SCOPE (see the scope notes)
    — a subtest is not a `func TestX(`, so citing one is not a resolvable claim.
    """
    out = []
    i, n = 0, len(src)
    while i < n:
        c = src[i]
        nxt = src[i + 1] if i + 1 < n else ""
        if c == "/" and nxt == "/":
            j = src.find("\n", i)
            j = n if j == -1 else j
            out.append(src[i:j])
            i = j
        elif c == "/" and nxt == "*":
            j = src.find("*/", i + 2)
            j = n if j == -1 else j + 2
            # Keep the comment body verbatim; newlines inside it are preserved.
            out.append(src[i:j])
            i = j
        elif c == '"':
            j = i + 1
            while j < n and src[j] != '"':
                if src[j] == "\\":
                    j += 1
                if src[j : j + 1] == "\n":
                    break
                j += 1
            j = min(j + 1, n)
            # A t.Run("...") subtest name is not a citation; everything else is.
            keep = not RUN_CALL_RE.search(src[max(0, i - 64) : i])
            out.append(src[i:j] if keep else " " * (j - i))
            i = j
        elif c == "`":
            j = src.find("`", i + 1)
            j = n if j == -1 else j + 1
            # Raw strings may span lines; keep the newlines for line accuracy.
            keep = not RUN_CALL_RE.search(src[max(0, i - 64) : i])
            out.append(src[i:j] if keep else "".join("\n" if ch == "\n" else " " for ch in src[i:j]))
            i = j
        elif c == "'":
            j = i + 1
            while j < n and src[j] != "'":
                if src[j] == "\\":
                    j += 1
                if src[j : j + 1] == "\n":
                    break
                j += 1
            j = min(j + 1, n)
            out.append(" " * (j - i))
            i = j
        else:
            out.append("\n" if c == "\n" else " ")
            i += 1
    return "".join(out)


def line_of(text: str, offset: int) -> int:
    return text.count("\n", 0, offset) + 1


def iter_files(base: Path, suffix: str):
    """Walk `base` yielding files ending in `suffix`, honouring SKIP_DIRS."""
    for dirpath, dirs, files in os.walk(base):
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        for f in sorted(files):
            if f.endswith(suffix):
                yield Path(dirpath, f)


def defined_tests() -> set:
    """Every `func TestXxx(` declared in a *_test.go in THIS repo."""
    found = set()
    for path in iter_files(ROOT, "_test.go"):
        try:
            src = path.read_text(errors="ignore")
        except OSError:
            continue
        found.update(TEST_FUNC_RE.findall(src))
    return found


class SymbolIndex:
    """Every top-level Go declaration in this repo, keyed (relpath, name) -> spans.

    A SPAN runs from the first line of the declaration's doc comment through the
    line that closes its body. Citing a symbol's doc comment is citing the symbol,
    so the comment is inside the span deliberately: without it a citation of
    `cloneForDryRun` at era3validity.go:173 reads as rotten when it points two
    lines above the `func`, at the comment that explains it.
    """

    def __init__(self):
        self.spans = {}   # (relpath, name) -> [(start, end)]
        self.lines = {}   # relpath -> [str]
        self.by_base = {} # basename -> {relpath}

    def build(self):
        for rel in GO_ROOTS:
            base = ROOT / rel
            if not base.exists():
                continue
            for path in iter_files(base, ".go"):
                self._index(path)
        return self

    def _index(self, path: Path):
        disp = str(path.relative_to(ROOT))
        lines = read_text(path).splitlines()
        self.lines[disp] = lines
        self.by_base.setdefault(os.path.basename(disp), set()).add(disp)
        for i, ln in enumerate(lines, 1):
            name, which = None, None
            for k, rx in enumerate(DECL_RES):
                m = rx.match(ln)
                if m:
                    name, which = m.group(1), k
                    break
            if not name:
                continue
            end = i
            if which in BRACED_DECL and ln.rstrip().endswith(("{", "(")):
                for j in range(i, len(lines)):
                    if lines[j].startswith(("}", ")")):
                        end = j + 1
                        break
            start = i
            k = i - 2
            while k >= 0 and lines[k].lstrip().startswith("//"):
                start = k + 1
                k -= 1
            self.spans.setdefault((disp, name), []).append((start, end))

    def resolve_path(self, cited: str):
        """Map a cited path to a repo-relative one, or None if it is not ours.

        A bare basename resolves only when it is UNAMBIGUOUS in the tree. Two
        files named chain.go would make the check guess, and a guessing gate is
        worse than no gate.
        """
        cited = cited.lstrip("./")
        if cited in self.lines:
            return cited
        hits = self.by_base.get(os.path.basename(cited), set())
        return next(iter(hits)) if len(hits) == 1 else None

    def declared_in(self, rel: str, name: str) -> bool:
        return (rel, name) in self.spans

    def decl_line(self, rel: str, name: str) -> int:
        return self.spans[(rel, name)][0][0]

    def points_at(self, rel: str, name: str, n: int) -> bool:
        """True if line `n` of `rel` is ON the symbol `name`."""
        if any(start <= n <= end for start, end in self.spans[(rel, name)]):
            return True
        lines = self.lines[rel]
        lo, hi = max(1, n - NEAR_LINES), min(len(lines), n + NEAR_LINES)
        return any(name in lines[k - 1] for k in range(lo, hi + 1))


def coord_citations_in_md(path: Path, index: SymbolIndex):
    """Yield (line_no, symbol, cited_coord, line_number) for ANCHORED coordinates.

    The anchor is the nearest backticked identifier within ANCHOR_WINDOW chars
    before the coordinate that is actually DECLARED in the cited file. Requiring
    the declaration is what keeps the pairing honest: an identifier that the file
    does not declare is prose, not a claim about that file.
    """
    for i, line in enumerate(read_text(path).splitlines(), 1):
        for m in COORD_RE.finditer(line):
            rel = index.resolve_path(m.group(1))
            if rel is None:
                continue  # not a file this repo owns; nothing to check against
            window = line[max(0, m.start() - ANCHOR_WINDOW):m.start()]
            for cand in reversed(TICKED_IDENT_RE.findall(window)):
                if index.declared_in(rel, cand):
                    yield i, cand, m.group(1), int(m.group(2)), rel
                    break


def collect_coords_in_repo(index: SymbolIndex):
    for rel in MD_FILES:
        if rel in COORD_EXEMPT_MD:
            continue
        path = ROOT / rel
        if path.exists():
            for rec in coord_citations_in_md(path, index):
                yield (rel,) + rec

    for rel in MD_ROOTS:
        base = ROOT / rel
        if not base.exists():
            continue
        for path in iter_files(base, ".md"):
            parts = path.relative_to(base).parts
            if parts and parts[0] in DOC_SKIP_DIRS:
                continue
            for rec in coord_citations_in_md(path, index):
                yield (str(path.relative_to(ROOT)),) + rec


def load_allowlist():
    """Return (test_names, coord_keys) from the one allowlist file.

    Two entry shapes, told apart by arity — one file, because two files invite a
    second discipline and the H/O rule in its header is what makes this work:
      TestFoo                                   a test-name citation
      docs/x.md  symbolName  path/to/f.go:918   a source-coordinate citation
    A coordinate entry carries NO doc line number on purpose: keyed that way it
    survives an unrelated edit above it, and only goes stale when the citation it
    excuses is actually repaired — which is the moment it should be deleted.
    """
    if not ALLOWLIST.exists():
        return set(), set()
    names, coords = set(), set()
    for line in ALLOWLIST.read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) == 1:
            names.add(parts[0])
        elif len(parts) == 3:
            coords.add(tuple(parts))
        else:
            print(f"warning: unparsable allowlist entry: {line!r}", file=sys.stderr)
    return names, coords


def scan_lines(lines):
    """Yield (line_no, name, candidates, is_family) for each test-name citation.

    `lines` is comment-masked for Go and raw for markdown, so every offset maps to
    a real line. `candidates` holds the alternative spellings a citation may
    legitimately resolve under: the bare name, plus its soft-wrap join when the
    name ends the line.
    """
    for i, line in enumerate(lines):
        for m in TEST_NAME_RE.finditer(line):
            name = m.group(0)
            rest = line[m.end():]
            cands = [name]
            if rest.strip() in ("", "-") and i + 1 < len(lines):
                tail = CONT_LEAD_RE.sub("", lines[i + 1])
                word = re.match(r"[A-Za-z0-9_]+", tail)
                if word:
                    cands.append(name + word.group(0))
            yield i + 1, name, cands, rest.startswith(FAMILY_MARKERS)


def read_text(path: Path) -> str:
    try:
        return path.read_text(errors="ignore")
    except OSError:
        return ""


def citations_in_go(path: Path):
    """Citations appearing in a Go COMMENT or a STRING LITERAL (never in bare code)."""
    return scan_lines(go_citation_mask(read_text(path)).splitlines())


def citations_in_md(path: Path):
    """Citations appearing anywhere in a markdown file, code fences included."""
    return scan_lines(read_text(path).splitlines())


def resolves(candidates, is_family: bool, known: set) -> bool:
    """True if any candidate spelling names a real (or allowlisted) test."""
    for cand in candidates:
        if cand in known:
            return True
        # A placeholder carrying a family suffix — "TestFoo_{A,B}" matches
        # "TestFoo_" — is still a placeholder, not a citation.
        if cand.rstrip("_") in PLACEHOLDER_NAMES:
            return True
        if is_family and any(k.startswith(cand) for k in known):
            return True
    return False


def collect_in_repo():
    """Yield (display_path, line_no, name, candidates, is_family) in-repo."""
    for rel in GO_ROOTS:
        base = ROOT / rel
        if not base.exists():
            continue
        for path in iter_files(base, ".go"):
            disp = str(path.relative_to(ROOT))
            for line_no, name, cands, fam in citations_in_go(path):
                yield disp, line_no, name, cands, fam

    for rel in MD_FILES:
        path = ROOT / rel
        if path.exists():
            for line_no, name, cands, fam in citations_in_md(path):
                yield rel, line_no, name, cands, fam

    for rel in MD_ROOTS:
        base = ROOT / rel
        if not base.exists():
            continue
        for path in iter_files(base, ".md"):
            parts = path.relative_to(base).parts
            if parts and parts[0] in DOC_SKIP_DIRS:
                continue
            disp = str(path.relative_to(ROOT))
            for line_no, name, cands, fam in citations_in_md(path):
                yield disp, line_no, name, cands, fam


def collect_external(roots):
    """Yield the same shape for the read-only review trees outside the repo."""
    for root in roots:
        base = Path(root)
        if not base.is_dir():
            continue  # hermetic: an absent tree is simply not checked
        for path in iter_files(base, ".md"):
            for line_no, name, cands, fam in citations_in_md(path):
                yield str(path), line_no, name, cands, fam


def external_roots(argv) -> list:
    roots = []
    for i, a in enumerate(argv):
        if a == "--external-root" and i + 1 < len(argv):
            roots.append(argv[i + 1])
        elif a.startswith("--external-root="):
            roots.append(a.split("=", 1)[1])
    if roots:
        return roots
    env = os.environ.get("SILT_CITED_TESTS_EXTERNAL_ROOTS")
    if env:
        return [p for p in env.split(":") if p]
    return DEFAULT_EXTERNAL_ROOTS


def report(title: str, phantoms: list, stream) -> None:
    print(title, file=stream)
    for path, line_no, name in phantoms:
        print(f"  {path}:{line_no}  {name}  (no such test)", file=stream)


def report_coords(title: str, rotten: list, stream) -> None:
    print(title, file=stream)
    for doc, line_no, sym, cited, n, rel, decl in rotten:
        print(
            f"  {doc}:{line_no}  `{sym}` cited at {cited}:{n}"
            f"  ->  {rel}:{n} is not on `{sym}` (declared {rel}:{decl})",
            file=stream,
        )


def main() -> int:
    argv = sys.argv[1:]
    strict_external = "--strict-external" in argv

    defined = defined_tests()
    if not defined:
        print("error: no `func TestX(` declarations found — refusing to run", file=sys.stderr)
        return 1
    allowed, allowed_coords = load_allowlist()
    known = defined | allowed | PLACEHOLDER_NAMES

    index = SymbolIndex().build()
    if not index.spans:
        print("error: no Go declarations indexed — refusing to run", file=sys.stderr)
        return 1

    def phantoms(records):
        out, seen = [], set()
        for path, line_no, name, cands, fam in records:
            key = (path, line_no, name)
            if key in seen or resolves(cands, fam, known):
                continue
            seen.add(key)
            out.append(key)
        return out

    def rotten(records):
        out, seen = [], set()
        for doc, line_no, sym, cited, n, rel in records:
            if (doc, sym, f"{cited}:{n}") in allowed_coords:
                continue
            if index.points_at(rel, sym, n):
                continue
            key = (doc, line_no, sym, cited, n)
            if key in seen:
                continue
            seen.add(key)
            out.append((doc, line_no, sym, cited, n, rel, index.decl_line(rel, sym)))
        return out

    roots = external_roots(argv)
    in_repo = phantoms(collect_in_repo())
    ext = phantoms(collect_external(roots))
    coords_in_repo = rotten(collect_coords_in_repo(index))

    failed = False

    if in_repo:
        failed = True
        report(
            f"FAIL [{SCAR_ID}] — these citations name a test that does not exist.\n"
            f"  A comment or doc claims a property is verified by a test that has no\n"
            f"  `func TestX(` anywhere in the tree. The check is green because it is\n"
            f"  absent, not because the property holds.\n",
            in_repo,
            sys.stderr,
        )
        print(
            "\nFix: write the test, correct the name, or — if the citation is\n"
            "intentionally historical — add the name to scripts/cited_tests_allowlist.txt\n"
            "with a comment saying why.\n",
            file=sys.stderr,
        )

    if coords_in_repo:
        failed = True
        report_coords(
            f"FAIL [{COORD_SCAR_ID}] — these source coordinates no longer point at the\n"
            f"  symbol they are cited beside. A reader who opens the file at that line\n"
            f"  sees unrelated code, and several of these are written \"verified\".\n"
            f"  Line numbers rot on every insertion above them; symbol names do not.\n",
            coords_in_repo,
            sys.stderr,
        )
        print(
            "\nFix: re-read the source and correct the line number, or drop the number\n"
            "and cite the symbol alone. If the citation is deliberately historical, add\n"
            "`<doc-path> <symbol> <cited-coord>` to scripts/cited_tests_allowlist.txt\n"
            "with a comment saying which case it is.\n",
            file=sys.stderr,
        )

    if ext:
        stream = sys.stderr if strict_external else sys.stdout
        label = "FAIL" if strict_external else "ADVISORY"
        report(
            f"\n{label} [{SCAR_ID}] — external review trees cite tests absent from this repo.\n"
            f"  These files live outside the repo and are not version-locked to it, so a\n"
            f"  name here may be real but sitting on an unmerged branch. Verify before\n"
            f"  relying on any of them as evidence that a property is checked.\n",
            ext,
            stream,
        )
        if strict_external:
            failed = True

    if failed:
        return 1

    print(
        f"OK [{SCAR_ID}] — every cited test name resolves to a real `func TestX(` "
        f"({len(defined)} tests defined, {len(allowed)} allowlisted)."
    )
    print(
        f"OK [{COORD_SCAR_ID}] — every symbol-anchored source coordinate points at its "
        f"symbol ({len(index.spans)} declarations indexed, {len(allowed_coords)} allowlisted)."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
