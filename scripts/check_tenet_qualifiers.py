#!/usr/bin/env python3
"""Scar-lint: guard the Sybil-composition design-target framing in docs/TENETS.md Part 0.

SCAR (confirmed 2026-09-01, session-18 TENETS rewrite):
  A prose "tightening" pass turned the Sybil-composition *design target* in Part 0 into
  an achieved present-tense property — "forging standing becomes indistinguishable from
  honest provision" — which reads as Sybil-PROOF. The blind red-team caught it. The
  honest form is the design-target qualifier: "the composition is *designed so that* …
  forging standing *would become* indistinguishable from honest provision." This lint
  fails the build any time that qualifier is dropped, preventing silent recurrence.

The forbidden pattern (present-tense over-claim, no qualifier):
  "standing becomes indistinguishable" — or any close variant — appearing without the
  "designed so that … would become" guard that scopes it to a design target.

The required pattern (design-target framing, currently on main at bb52d26):
  "designed so that" ... "would become" ... "indistinguishable"

Dependency-free (stdlib only). Run: python3 scripts/check_tenet_qualifiers.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TENETS = ROOT / "docs" / "TENETS.md"

# The over-claim: present-tense "becomes indistinguishable" adjacent to "standing"
# (with no qualifier). We look for this pattern within a 300-char window so it catches
# variants like "forging standing becomes indistinguishable" and
# "standing becomes indistinguishable from honest provision".
FORBIDDEN_RE = re.compile(
    r"(?i)standing\s+becomes\s+indistinguishable",
)

# The required qualifier that scopes the claim to a design target.
# Both halves must appear in the same paragraph (within 500 chars).
REQUIRED_DESIGNED = re.compile(r"(?i)designed\s+so\s+that")
REQUIRED_WOULD    = re.compile(r"(?i)would\s+become")

SCAR_ID = "scar:sybil-design-target-overclaim-2026-09-01"


def paragraphs(text: str):
    """Yield (start_line, paragraph_text) for each blank-line-separated block."""
    lines = text.splitlines(keepends=True)
    buf = []
    start = 1
    for i, line in enumerate(lines, 1):
        if line.strip() == "" and buf:
            yield start, "".join(buf)
            buf = []
            start = i + 1
        else:
            buf.append(line)
    if buf:
        yield start, "".join(buf)


def main() -> int:
    if not TENETS.exists():
        print(f"error: {TENETS.relative_to(ROOT)} not found", file=sys.stderr)
        return 1

    text = TENETS.read_text()

    # Pass 1 — detect the forbidden pattern in any paragraph.
    forbidden_hits = []
    for start_line, para in paragraphs(text):
        for m in FORBIDDEN_RE.finditer(para):
            # Count line number within the paragraph.
            line_no = start_line + para[: m.start()].count("\n")
            forbidden_hits.append((line_no, para.strip()[:120]))

    if forbidden_hits:
        print(
            f"FAIL [{SCAR_ID}] — docs/TENETS.md contains the over-claim:\n"
            f"  Present-tense 'standing becomes indistinguishable' asserts Sybil-proof,\n"
            f"  not a design target. The required form is:\n"
            f"    'the composition is *designed so that* … forging standing *would become*\n"
            f"     indistinguishable from honest provision'\n",
            file=sys.stderr,
        )
        for line_no, snippet in forbidden_hits:
            print(f"  Line ~{line_no}: …{snippet}…", file=sys.stderr)
        print(
            "\nFix: restore the design-target qualifier "
            "('designed so that … would become indistinguishable').\n"
            "See commit bb52d26 for the correct framing.",
            file=sys.stderr,
        )
        return 1

    # Pass 2 — verify the required qualifier is present.
    # The two halves must appear in the same paragraph so a stray "designed so that"
    # elsewhere doesn't mask a missing "would become" near the Sybil claim.
    qualifier_found = False
    for _start_line, para in paragraphs(text):
        if (
            "indistinguishable" in para.lower()
            and REQUIRED_DESIGNED.search(para)
            and REQUIRED_WOULD.search(para)
        ):
            qualifier_found = True
            break

    if not qualifier_found:
        print(
            f"FAIL [{SCAR_ID}] — docs/TENETS.md: the 'indistinguishable' Sybil claim\n"
            f"  exists but neither 'designed so that' nor 'would become' appears in the\n"
            f"  same paragraph — the design-target qualifier is missing.\n"
            f"  Required form: 'the composition is *designed so that* … forging standing\n"
            f"  *would become* indistinguishable from honest provision'\n"
            f"  See commit bb52d26 for the correct framing.",
            file=sys.stderr,
        )
        return 1

    print(
        f"OK [{SCAR_ID}] — docs/TENETS.md Sybil-composition claim carries "
        "the required design-target qualifier."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
