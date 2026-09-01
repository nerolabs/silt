#!/usr/bin/env python3
"""Doc-lint: catch a Status/state header that claims NOT-built while its own body
says the thing SHIPPED — the "spec-time header, never trued-up" defect.

SCAR (confirmed by the 2026-09-01 thorough docs audit):
  docs/design/h7-proof-of-repair.md carried a header that read "design, not built"
  while the SAME doc asserted "BUILT" five times and the code existed
  (core/repairproof/, wired into core/node/repairclaim.go). The header was written
  at spec-time and never trued-up when the code shipped. A reader trusting the
  header would conclude the feature does not exist. This lint fails the build any
  time a doc contradicts itself that way, preventing silent recurrence.

The defect is a SELF-CONTRADICTION inside one file. We flag a doc only when BOTH:
  1. a header/status line asserts a not-built state (see NOT_BUILT_RE), AND
  2. the same doc body asserts a built/shipped state (see BUILT_RE).

A genuinely-unbuilt spec — "design, not built" with NO built-marker — is fine and
is NOT flagged. A NEGATED built-marker ("No code has shipped", "not yet shipped",
"has not shipped") is also NOT flagged: the lint flags only self-contradiction — a
not-built Status header alongside a NON-NEGATED built-marker in the body.

Dependency-free (stdlib only). Run: python3 scripts/check_status_headers.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"

SCAR_ID = "scar:status-header-vs-body-contradiction-2026-09-01"

# Directories under docs/ that are frozen history — skip them entirely.
SKIP_DIRS = {"thinking", "buildlog", "reviews", "archive"}

# (1) The NOT-built state assertion. Case-insensitive.
NOT_BUILT_RE = re.compile(
    r"(?i)("
    r"design,\s*not\s*built"
    r"|not\s+yet\s+built"
    r"|not\s+built"
    r"|status:\s*design\b"
    r"|status:\s*planned\b"
    r"|proposed\s*\(not\s*built\)"
    r"|\bunbuilt\b"
    r")"
)

# A NOT-built assertion only counts as the defect when it sits on a genuine
# *Status header* line — a line that declares the DOC'S state, not prose that
# merely mentions an unbuilt sub-track. The h7 defect was exactly a
# "> **Status: design, not built**" line at the top of the doc. We require the
# same line to carry an explicit "Status" label. This is what excludes prose
# like "the B axis … is an unbuilt track" and a risk-register row's "NOT YET
# BUILT" — those are not the doc's own Status header.
STATUS_LABEL_RE = re.compile(r"(?i)\bstatus\b")

# (2) A strong BUILT/shipped marker in the same doc. Case-sensitive for the
# all-caps "BUILT" to avoid matching the word "built" inside ordinary prose like
# "built from"; the other markers are distinctive enough to match loosely.
BUILT_MARKERS = [
    (re.compile(r"\bBUILT\b"), "BUILT"),
    (re.compile(r"(?i)\bshipped\b"), "shipped"),
    (re.compile(r"(?i)\bgoes\s+LIVE\b"), "goes LIVE"),
    (re.compile(r"(?i)\bis\s+LIVE\b"), "is LIVE"),
    (re.compile(r"(?i)merged\s+in\s+#"), "merged in #"),
    (re.compile(r"(?i)\bwired\s+into\b"), "wired into"),
    (re.compile(r"(?i)\bin\s+production\b"), "in production"),
]

# Negation cues. If any of these tokens precedes a built-marker within a few
# tokens on the SAME line, the marker is negated ("No code has shipped", "not yet
# shipped", "has not shipped", "yet to ship", "to be built") and does NOT count as
# a built assertion. This is a general guard: it applies to every marker, not just
# "shipped", because "not yet live" / "not built" negate the others too.
NEG_CUES = {
    "not", "never", "no", "hasn't", "haven't", "isn't",
    "won't", "aren't", "wasn't", "weren't",
}
# Multi-word cues checked as a suffix of the preceding-tokens window.
NEG_PHRASES = [("yet", "to"), ("to", "be")]
# How many tokens before the marker to inspect for a negation cue.
NEG_WINDOW = 4


def _negated(line: str, marker_start: int) -> bool:
    """True if the built-marker at marker_start on `line` is negated by a cue
    within NEG_WINDOW tokens immediately preceding it on the same line."""
    prefix = line[:marker_start]
    # Tokenize the prefix into lowercase word tokens (strip punctuation).
    tokens = re.findall(r"[a-z']+", prefix.lower())
    window = tokens[-NEG_WINDOW:]
    if any(tok in NEG_CUES for tok in window):
        return True
    for a, b in NEG_PHRASES:
        for i in range(len(window) - 1):
            if window[i] == a and window[i + 1] == b:
                return True
    return False


def iter_docs():
    """Yield doc .md paths under docs/, skipping frozen-history subtrees."""
    for path in sorted(DOCS.rglob("*.md")):
        rel_parts = path.relative_to(DOCS).parts
        if rel_parts and rel_parts[0] in SKIP_DIRS:
            continue
        yield path


def scan(path: Path):
    """Return (not_built_hit, built_hit) or None if no contradiction.

    not_built_hit = (line_no, line_text); built_hit = (line_no, marker, line_text).
    """
    lines = path.read_text(errors="replace").splitlines()

    not_built_hit = None
    for i, line in enumerate(lines, 1):
        if NOT_BUILT_RE.search(line) and STATUS_LABEL_RE.search(line):
            not_built_hit = (i, line.strip())
            break
    if not_built_hit is None:
        return None

    for i, line in enumerate(lines, 1):
        for rx, label in BUILT_MARKERS:
            for m in rx.finditer(line):
                if not _negated(line, m.start()):
                    return not_built_hit, (i, label, line.strip())
    return None


def main() -> int:
    if not DOCS.exists():
        print(f"error: {DOCS.relative_to(ROOT)} not found", file=sys.stderr)
        return 1

    failures = []
    for path in iter_docs():
        hit = scan(path)
        if hit is not None:
            failures.append((path, hit))

    if failures:
        print(
            f"FAIL [{SCAR_ID}] — a doc's Status/state header claims NOT-built while\n"
            f"  the SAME doc says the feature SHIPPED. The header was likely written at\n"
            f"  spec-time and never trued-up when the code landed. True up the header.\n",
            file=sys.stderr,
        )
        for path, ((nb_line, nb_text), (b_line, marker, b_text)) in failures:
            rel = path.relative_to(ROOT)
            print(f"  {rel}", file=sys.stderr)
            print(f"    not-built header  (line {nb_line}): {nb_text}", file=sys.stderr)
            print(f"    contradicting     (line {b_line}, '{marker}'): {b_text}",
                  file=sys.stderr)
        print(
            "\nFix: update the Status/state header to match reality, or remove the\n"
            "built-marker if the feature genuinely is not built.\n",
            file=sys.stderr,
        )
        return 1

    print(
        f"OK [{SCAR_ID}] — no doc contradicts its own not-built Status header with a "
        "built/shipped marker."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
