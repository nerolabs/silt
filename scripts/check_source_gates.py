#!/usr/bin/env python3
"""Test-lint: a SOURCE-TEXT gate must say so, and must name its runtime cover.

SCAR (count=3, third-time rule fired 2026-09-03 — scar-verifies-x-must-name-the-axes):
  cmd/silt/rt_r04b_c3_laneoff_test.go TestDaemonDegradesTheDemandLaneInsteadOfDying
  greps daemon.go for the string "LANE OFF" and for the ABSENCE of one exact return
  statement. Its t.Fatal text promises a RUNTIME property — "a corrupt demand key file
  must stop the RECEIPT LANE, not chain participation, storage and serving". The Tester
  reintroduced that exact regression with a DIFFERENT early return: the gate stayed
  GREEN and `go vet` stayed clean. A green source gate had been reading as evidence of a
  behaviour it cannot observe. Two earlier occurrences of the same shape (a gate that
  does not measure the axis its own text names) are recorded in the scar.

THE RULE this lint enforces, on any test that reads a non-testdata `.go` file:

  1. Every failure message string in that test begins with the marker `SOURCE GATE:`.
     The message must describe WHAT WAS CHECKED (a string, an order, a count), so a
     reader of the failure knows the gate is structural.
  2. The test carries an annotation naming its runtime cover, in the doc comment or the
     body: either `RUNTIME GATE: <name>` (the test that observes the behaviour) or
     `UNGATED: <residual>` (an explicit statement that the behaviour is unobserved).

internal/depcheck is the precedent: it is honest about being structural. The cmd/silt
gates borrowed the style without the disclaimer. This lint closes that gap mechanically.

Dependency-free (stdlib only). Run: python3 scripts/check_source_gates.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

SCAR_ID = "scar:source-gate-promises-a-runtime-property-2026-09-03"

MARKER = "SOURCE GATE:"
COVER_RE = re.compile(r"RUNTIME GATE:\s*\S|UNGATED:\s*\S")

# Directories that are frozen history or vendored — never linted.
SKIP_PARTS = {"archive", "vendor", ".git", "testdata"}

# A read of a literal `.go` path that is not under testdata/. This is the signature of a
# source-text gate: the test is asserting on the project's own source.
SOURCE_READ_RE = re.compile(r'os\.ReadFile\(\s*"((?:[^"\\]|\\.)*\.go)"')

# A failure call with a leading string literal. `t.Fatal(err)` has no literal and is an
# infrastructure error, not a property claim, so it is not linted.
FAIL_RE = re.compile(r'\bt\.(?:Fatal|Fatalf|Error|Errorf)\(\s*"')

FUNC_RE = re.compile(r"^func\s+(Test\w+)\s*\(", re.MULTILINE)


def read_string_literal(text, i):
    """text[i] is the opening quote of a Go interpreted string literal. Return its
    decoded-enough content (escapes left as written) and the index past the close."""
    assert text[i] == '"'
    j = i + 1
    out = []
    while j < len(text):
        c = text[j]
        if c == "\\":
            out.append(text[j:j + 2])
            j += 2
            continue
        if c == '"':
            return "".join(out), j + 1
        out.append(c)
        j += 1
    return "".join(out), len(text)


def code_mask(src):
    """Return a same-length string with comments, string literals and rune literals
    blanked to spaces (newlines preserved). Brace counting on the mask cannot be thrown
    by an apostrophe in a comment — which is exactly how the first draft of this lint
    silently skipped core/statehash/witness_test.go."""
    out = []
    i = 0
    n = len(src)
    while i < n:
        c = src[i]
        if c == "/" and i + 1 < n and src[i + 1] == "/":
            j = src.find("\n", i)
            j = n if j < 0 else j
            out.append(" " * (j - i))
            i = j
            continue
        if c == "/" and i + 1 < n and src[i + 1] == "*":
            j = src.find("*/", i + 2)
            j = n if j < 0 else j + 2
            out.append("".join(" " if ch != "\n" else "\n" for ch in src[i:j]))
            i = j
            continue
        if c in ('"', "'", "`"):
            j = i + 1
            while j < n:
                if src[j] == "\\" and c != "`":
                    j += 2
                    continue
                if src[j] == c:
                    j += 1
                    break
                if src[j] == "\n" and c != "`":
                    break  # unterminated; stop at the newline
                j += 1
            out.append("".join(" " if ch != "\n" else "\n" for ch in src[i:j]))
            i = j
            continue
        out.append(c)
        i += 1
    mask = "".join(out)
    assert len(mask) == len(src), (len(mask), len(src))
    return mask


def func_spans(src):
    """Yield (name, start, end) for each top-level Test function, by brace counting on
    the comment/literal-blanked mask."""
    mask = code_mask(src)
    for m in FUNC_RE.finditer(src):
        if mask[m.start():m.end()].strip() == "":
            continue  # the match is inside a comment or a literal
        brace = mask.find("{", m.end())
        if brace < 0:
            continue
        depth = 0
        for k in range(brace, len(mask)):
            if mask[k] == "{":
                depth += 1
            elif mask[k] == "}":
                depth -= 1
                if depth == 0:
                    yield m.group(1), m.start(), k + 1
                    break


def preceding_comment(src, start):
    """The contiguous // comment block immediately above `start`."""
    lines = src[:start].split("\n")
    out = []
    for ln in reversed(lines[:-1] if lines and lines[-1] == "" else lines):
        s = ln.strip()
        if s.startswith("//"):
            out.append(s)
            continue
        if s == "":
            # A blank line ends the doc comment.
            break
        break
    return "\n".join(reversed(out))


def main():
    failures = []
    checked = 0
    for path in sorted(ROOT.rglob("*_test.go")):
        rel = path.relative_to(ROOT)
        if SKIP_PARTS & set(rel.parts):
            continue
        src = path.read_text(encoding="utf-8", errors="replace")
        if not SOURCE_READ_RE.search(src):
            continue
        for name, start, end in func_spans(src):
            body = src[start:end]
            reads = SOURCE_READ_RE.findall(body)
            if not reads:
                continue
            checked += 1
            context = preceding_comment(src, start) + "\n" + body
            line0 = src[:start].count("\n") + 1
            if not COVER_RE.search(context):
                failures.append((
                    rel, line0, name, reads,
                    "no `RUNTIME GATE: <name>` and no `UNGATED: <residual>` annotation. "
                    "A source gate must name the test that observes the behaviour, or "
                    "state in the open that nothing does.",
                ))
            bad = []
            for fm in FAIL_RE.finditer(body):
                q = body.index('"', fm.start())
                lit, _ = read_string_literal(body, q)
                if not lit.startswith(MARKER):
                    bad.append((body[:q].count("\n") + line0, lit[:90]))
            for ln, lit in bad:
                failures.append((
                    rel, ln, name, reads,
                    f'failure message does not begin with "{MARKER}": "{lit}…"',
                ))

    if failures:
        print(
            f"FAIL [{SCAR_ID}] — a source-text gate reads as a runtime gate.\n\n"
            "  A test that asserts on the project's own .go source can only see STRINGS\n"
            "  and ORDER. Its failure text must say so, and it must name the runtime\n"
            "  gate that covers the behaviour (or declare the behaviour UNGATED).\n",
            file=sys.stderr,
        )
        for rel, ln, name, reads, why in failures:
            print(f"  {rel}:{ln}  {name}  (reads {', '.join(reads)})", file=sys.stderr)
            print(f"    {why}", file=sys.stderr)
        print(
            f'\nFix: prefix every failure message with "{MARKER} " and describe what was\n'
            "checked (a string, an order, a count); then add a `RUNTIME GATE: <test>` or\n"
            "`UNGATED: <residual>` line to the doc comment.\n",
            file=sys.stderr,
        )
        return 1

    print(
        f"OK [{SCAR_ID}] — {checked} source-text gate(s); each is labelled "
        f'"{MARKER}" and names its runtime cover.'
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
