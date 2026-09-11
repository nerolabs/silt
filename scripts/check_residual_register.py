#!/usr/bin/env python3
"""check_residual_register.py — every residual named in ROADMAP.md has a register row.

The scar: the residual backlog grew from 7 named residuals (2026-09-01) to 127 (2026-09-06)
with no bucket and no closer on most of them, so "held in tension" trades and "closed by
bound" numbers sat in the same list as work. The filing rule (ROADMAP.md, "Residual
filing rule") requires every `R-` name to have a row in the "Residual register" table with
a Bucket, a Closer and a Source. This lint holds that rule.

Checks (stdlib only; exit 1 on any FAIL):
  1. token scan of ROADMAP.md for residual names (see NAME_RE; a trailing ′ is the same name;
     matches inside a backtick span that contains "/" or ".md" are filenames, not names);
  2. the register table parses with the five columns in order;
  3. every scanned name has a row; a row with no prose occurrence is INFO only;
  4. Bucket is one of the four literals; Closer and Source non-empty; Source names a
     ROADMAP line anchor (L123) or a silt-agent-memory/ path;
  5. Duplicate-of is "—" or another row's Name; Names unique after stripping ′;
  6. an UNCLEAR row must carry `since:YYYY-MM-DD` in Closer.
"""
import re
import sys

ROADMAP = "ROADMAP.md"
NAME_RE = re.compile(r"(?<![A-Za-z0-9-])R-[A-Z0-9]+(?:-[A-Z0-9]+)+′?(?![a-z])")
EXTRA = {"FP-1", "FP-2", "R-membership", "R-A-membership-source"}
BUCKETS = {"ACTIONABLE", "HELD-IN-TENSION", "CLOSED-BY-BOUND", "UNCLEAR"}
SCAR = "scar:residual-backlog-unbucketed-2026-09-06"


def strip_filename_spans(text):
    # blank out backtick spans that are paths, so `…/R-BOX-ATTESTS-scoping-….md` is not a name
    def repl(m):
        body = m.group(1)
        return "`" + (" " * len(body)) + "`" if ("/" in body or ".md" in body) else m.group(0)
    return re.sub(r"`([^`\n]*)`", repl, text)


def scan_names(text):
    names = {}
    for lineno, line in enumerate(strip_filename_spans(text).splitlines(), 1):
        for m in NAME_RE.finditer(line):
            n = m.group(0)
            if not re.search(r"[A-Z]", n.split("-", 1)[1]):
                continue  # R-17 style: a digit-only tail is a finding id, not a residual
            names.setdefault(n.rstrip("′"), lineno)
        for e in EXTRA:
            if re.search(r"(?<![A-Za-z0-9-])" + re.escape(e) + r"(?![A-Za-z0-9-])", line):
                names.setdefault(e, lineno)
    return names


def parse_register(text):
    m = re.search(r"^#{2,3} Residual register.*$", text, re.M)
    if not m:
        return None, "no `## Residual register` heading"
    rest = text[m.end():]
    lines = rest.splitlines()
    rows, header_seen = [], False
    for i, line in enumerate(lines):
        if not line.startswith("|"):
            if header_seen and rows:
                break
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if not header_seen:
            want = ["Name", "Bucket", "Closer", "Source", "Duplicate-of"]
            if cells != want:
                return None, "register header is %r, want %r" % (cells, want)
            header_seen = True
            continue
        if set(line.replace("|", "").strip()) <= set("-: "):
            continue
        rows.append((cells, i))
    return rows, None


def main():
    text = open(ROADMAP, encoding="utf-8").read()
    fails = []
    rows, err = parse_register(text)
    if err:
        fails.append(err)
        rows = []
    reg = {}
    for cells, _ in rows:
        if len(cells) != 5:
            fails.append("register row has %d cells, want 5: %s" % (len(cells), " | ".join(cells)[:100]))
            continue
        name, bucket, closer, source, dup = cells
        name = name.strip("`")
        key = name.rstrip("′")
        if not (NAME_RE.fullmatch(name) or name in EXTRA):
            fails.append("register Name %r is not a residual token" % name)
        if key in reg:
            fails.append("register Name %r appears twice (′ and unprimed are one name)" % name)
        reg[key] = (bucket, closer, source, dup.strip("`"))
    for key, (bucket, closer, source, dup) in reg.items():
        if bucket not in BUCKETS:
            fails.append("%s: Bucket %r not one of %s" % (key, bucket, sorted(BUCKETS)))
        if not closer.strip() or closer.strip() == "—":
            fails.append("%s: empty Closer" % key)
        if not (re.search(r"L\d+", source) or "silt-agent-memory/" in source):
            fails.append("%s: Source %r names neither a ROADMAP line (L123) nor a silt-agent-memory/ path" % (key, source))
        if dup != "—" and dup.rstrip("′") not in reg:
            fails.append("%s: Duplicate-of %r is not a register row" % (key, dup))
        if bucket == "UNCLEAR" and not re.search(r"since:\d{4}-\d{2}-\d{2}", closer):
            fails.append("%s: an UNCLEAR row must carry since:YYYY-MM-DD in Closer" % key)
    names = scan_names(text)
    for key, lineno in sorted(names.items(), key=lambda kv: kv[1]):
        if key not in reg:
            fails.append("%s:%d: %s is named but has no Residual register row — file it with a Bucket and a Closer" % (ROADMAP, lineno, key))
    stale = sorted(k for k in reg if k not in names)
    if fails:
        print("FAIL [%s] — %d violation(s):" % (SCAR, len(fails)), file=sys.stderr)
        for f in fails:
            print("  " + f, file=sys.stderr)
        return 1
    print("OK [%s] — %d residual name(s) in ROADMAP.md, %d register row(s)%s" % (
        SCAR, len(names), len(reg), ("; INFO %d row(s) with no prose occurrence: %s" % (len(stale), ", ".join(stale)) if stale else "")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
