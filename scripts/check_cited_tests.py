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
  net to Go comments, CHANGELOG, ROADMAP, docs/**, and the external review trees.

WHAT IS CHECKED
  A name matching \\bTest[A-Z][A-Za-z0-9_]*\\b, appearing in:
    - Go COMMENTS (not code, not string literals) under the source roots below
    - CHANGELOG.md, ROADMAP.md, docs/**/*.md
    - optionally the external certification/ruling trees (see --external-root)
  must resolve to a `func TestX(` declaration in some *_test.go in this repo.

SCOPE NOTES (deliberate limits)
  - METASYNTACTIC PLACEHOLDERS (TestFoo, TestXxx, ...) are never citations.
  - SUBTESTS ARE OUT OF SCOPE. A `t.Run("name", ...)` subtest is not a top-level
    func, so a comment citing only a subtest name resolves only via its parent
    Test func (see FAMILY below). Cite the parent, or use the allowlist.
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


def go_comment_mask(src: str) -> str:
    """Return `src` with every NON-comment character replaced by a space and all
    newlines preserved, so regex offsets still map to real line numbers.

    A real (small) Go scanner is needed here rather than a naive '//' split:
    without tracking string state, a test name inside a string literal — e.g.
    t.Run("TestFoo") or a fixture path — would be mis-read as a citation, and a
    '//' inside a string would swallow the rest of the line.
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
            out.append(" " * (j - i))
            i = j
        elif c == "`":
            j = src.find("`", i + 1)
            j = n if j == -1 else j + 1
            # Raw strings may span lines; keep the newlines for line accuracy.
            out.append("".join("\n" if ch == "\n" else " " for ch in src[i:j]))
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


def load_allowlist() -> set:
    if not ALLOWLIST.exists():
        return set()
    names = set()
    for line in ALLOWLIST.read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if line:
            names.add(line)
    return names


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
    """Citations appearing in a Go COMMENT (never in code or a string literal)."""
    return scan_lines(go_comment_mask(read_text(path)).splitlines())


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


def main() -> int:
    argv = sys.argv[1:]
    strict_external = "--strict-external" in argv

    defined = defined_tests()
    if not defined:
        print("error: no `func TestX(` declarations found — refusing to run", file=sys.stderr)
        return 1
    allowed = load_allowlist()
    known = defined | allowed | PLACEHOLDER_NAMES

    def phantoms(records):
        out, seen = [], set()
        for path, line_no, name, cands, fam in records:
            key = (path, line_no, name)
            if key in seen or resolves(cands, fam, known):
                continue
            seen.add(key)
            out.append(key)
        return out

    in_repo = phantoms(collect_in_repo())
    ext = phantoms(collect_external(external_roots(argv)))

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

    n_allow = len(allowed)
    print(
        f"OK [{SCAR_ID}] — every cited test name resolves to a real `func TestX(` "
        f"({len(defined)} tests defined, {n_allow} allowlisted)."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
