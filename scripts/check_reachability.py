#!/usr/bin/env python3
"""Reachability gate: is the lane's mechanism actually IN THE SHIPPED BINARY?

  scar:mechanism-shipped-inert-2026-09-10

SCAR (count 6 in one month). Six mechanisms were present in source, described in the
record as delivered and enforcing, and had ZERO non-test callers — so the linker
dropped every one of them out of `./cmd/silt`:

  core/chain.(*Chain).CheckConsensusParams        a refuse-to-start that never ran
  core/node.(*Node).FundDeliverySessionRemote     the client half of delivery top-up
  core/node.(*Node).OpenRelaySessionRemote        \\
  core/node.(*Node).AcquireRelayAnchors            } the entire paid-relay client
  core/node.(*Node).SubmitRelayPay                /
  core/credit.(*Ledger).GuardFullRefusalsByLane   a per-lane counter nothing reads

WHY nm AND NOT grep. A grep sweep of the tree found 117 candidate sites and named its
own biggest hole: it keys on BARE IDENTIFIERS, so an inert method hides behind a live
namesake. Confirmed miss: `demand.Commit` has zero non-test callers and never appeared
in the count. Every common verb — Verify, Close, Root, Get — has that hole.

The linked binary has no such hole. Dead-code elimination is total: a symbol either
survived linking or it did not, and `go tool nm` reports the fully qualified name, so
`core/node.(*Node).Commit` and `core/demand.(*Set).Commit` are different rows. No call
graph has to be reasoned about. This is the same instrument .github/workflows/ci.yml
already points at the DEFAULT binary to prove B_bootstrap is absent (D-BB-BUILD-TAG);
this gate is the other direction — presence — and it is data-driven.

THE ONE CAVEAT, AND WHY THIS GATE REFUSES THIN SYMBOLS
  A one-line wrapper can be INLINED AWAY, so its absence from the symbol table proves
  nothing about whether the lane is reachable. adapters/relay.DialThroughPaid is exactly
  such a wrapper: its whole body is `return dialThrough(...)`. Naming a thin symbol
  would give a gate that reports a lane inert when it is live, and a maintainer who
  "fixes" that by relaxing the assertion ends up with a gate that reports nothing.

  So an entry must name a SUBSTANTIAL symbol, and THE COMPILER DECIDES THAT, not a
  proxy for it. `go build -gcflags=-m=2` prints one verdict per declaration in the
  package: `can inline <name> with cost N` or `cannot inline <name>: <reason>`. An entry
  passes only on the second. That is the inliner itself answering the exact question the
  gate depends on, so ABSENCE from the symbol table means the linker dropped the symbol.

  THE TWO PROXIES THIS REPLACED, AND THE MEASUREMENT THAT RETIRED EACH (2026-09-11)
    - "body >= 8 real lines AND contains a function literal". Driven by running the
      retired rule itself against this file's nine freeze-manifest records: it REFUSES
      SEVEN of the nine — core/chain.setD3Digests (cost 172), validateD3Digests (1010),
      (*Chain).CheckConsensusParams (278), core/genesis.Build (421),
      cmd/silt.printEraObservable (721), core/chain.StartupEraLines (906) and
      adapters/chainstore.Recover (802), each against a budget of 80, each a symbol the
      compiler will not inline at any call site. A callback closure is a property of a
      client entry point that hands work to a transport, not of a consensus validator.
      THE SAME RULE ADMITS SYMBOLS THE COMPILER WILL INLINE, and its own paradigm case
      is one: core/node.(*Node).repairStripeFetch is 88 body lines and carries a closure
      — the exact shape the rule read as substance — while the compiler says
      `can inline (*Node).repairStripeFetch with cost 77`. The body is one call handing
      a large closure to fetchStripeByColumn, and the closure is priced separately
      (repairStripeFetch.func1, cost 879). The closure taken as EVIDENCE of substance is
      what makes the parent cheap. Four more in this tree: (*Node).columnHoldersEntry
      (77), (*Node).auditLeaf (76), (*Node).EnableRelayAccept (42),
      cmd/silt.(*uiServer).guard (18).
    - the `.funcN` witness on the PRESENT path. `(*Chain).stateRootLeavesV5` declares a
      closure at core/chain/statehash.go:306, is PRESENT in the linked binary, and has
      ZERO `.funcN` rows in its symbol table — the compiler inlined the closure into its
      own body (`can inline (*Chain).stateRootLeavesV5.func1`). The old witness reported
      that live mechanism as hollowed.

  THE BOUNDARY, IN BOTH DIRECTIONS — NEITHER TEST DOMINATES THE OTHER. The inline
  verdict is NOT strictly stronger than the `.funcN` witness it replaced. The two rules
  fire on different events, so each catches a hollowing the other passes:

      hollowing shape                                     retired     inline verdict
      body collapses below cost 80                        RED         RED
      body gutted, cost stays > 80, closure folded away   RED         GREEN
      body gutted, cost stays > 80, closure survives      GREEN       GREEN
      live mechanism whose closure folds into its parent  RED (false) GREEN (correct)

    THE INPUT THIS GATE DOES NOT CATCH, NAMED AND DRIVEN: gut core/chain.v5ValidateSlashes
    — delete the SlashesBytesCap check, the M2 era floor and the culprit walk, keep one
    used closure — and the compiler still prints `cannot inline v5ValidateSlashes:
    function too complex: cost 263 exceeds budget 80`. THIS GATE REPORTS OK, EXIT 0. The
    retired `.funcN` rule goes RED on that same binary. So a green run of this gate is
    NOT evidence that manifest item 10's byte ceiling is still enforced.

    Rows 2 and 4 are the SAME compiler event — a closure cheap enough to fold into its
    parent — read once as a true positive and once as a false positive. That is why the
    retired rule is not a hollowing detector: it is a closure-inlining detector that
    correlated with hollowing by accident, and stateRootLeavesV5 is where the
    correlation broke on a live, load-bearing symbol. The trade is still right — the
    inline verdict catches the hollowing that actually happens (a body replaced by
    `return nil`, driven RED), admits seven records the retired rule refused, and
    removes a live false RED — but it is a TRADE, not a strengthening, and the next
    reader has to know which half was given up.

  DO NOT WEAKEN THIS. If a lane has no substantial entry point, that is a finding about
  the lane, not a reason to point the entry at a wrapper. Widening this rule again means
  showing a measurement, the way these two were retired.

WHAT AN ENTRY ASSERTS (scripts/reachability_lanes.txt)
  One equality, with a closed complement:

      the label says THIS SYMBOL is unreachable  ==  the symbol is absent

  The label's claim is the PAIR: the phrase "cannot be exercised" AND the symbol's own
  name, both inside the lane's posture line in docs/release-checklist.md. So:
    ABSENT + label says so, naming it   -> pass  (an honest lane).
    ABSENT + any other label            -> FAIL  (the over-claim this gate exists for).
    PRESENT + no such claim about it    -> pass.
    PRESENT + label says it is missing  -> FAIL  (the label rotted past the code).

  "cannot be exercised" and "has not been exercised" are different claims to a reader.
  The first is honest about a client half that is not in the binary at all; the second
  implies someone could run it tomorrow.

  BOTH HALVES OF THE PAIR ARE LOAD-BEARING. Requiring the phrase alone would let one
  posture line excuse a whole lane; requiring the name scopes the claim, because a lane
  routinely has a live half and a dropped half — the paid delivery lane opens and
  settles, but cannot top up. And a label that names a symbol while claiming both
  postures at once (AMBIGUOUS) states no posture, and fails either way.

EVERY ENTRY CARRIES A WRITTEN REASON, AND EVERY REASON HAS A MECHANICAL COMPANION
  scar:allowlist-rationale-is-itself-a-claim — an unreasoned row is not a row. Each
  entry must carry `claim` (the public claim in the release checklist this lane backs)
  and `substantial` (why this symbol is not a thin wrapper). Neither is decoration:
  `claim` is checked by resolving `label` in the checklist, and `substantial` is checked
  against the compiler's inline verdict. A record missing either field FAILS.

SCOPE — deliberately small. This gate covers LANES THE RELEASE CHECKLIST MAKES A PUBLIC
CLAIM ABOUT. It is not a sweep of exported symbols: the floor-box keystone is inert by
ratified owner direction (D-RECOMPUTE-FREEZE) and would drown the signal. Adding a lane
is adding a record.

REACHABILITY IS NECESSARY AND NEVER SUFFICIENT. A green run proves a symbol survived
linking. It proves nothing about whether the mechanism is correct, or whether it computes
what its record claims. DO NOT BOOK A FREEZE-MANIFEST ITEM AS BUILT ON THE STRENGTH OF A
GREEN REACHABILITY RUN.

Dependency-free (stdlib only). Needs a Go toolchain unless --binary is given.
Run: python3 scripts/check_reachability.py
"""
import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

SCAR_ID = "scar:mechanism-shipped-inert-2026-09-10"

LANES_FILE = ROOT / "scripts" / "reachability_lanes.txt"
CHECKLIST = ROOT / "docs" / "release-checklist.md"
MAIN_PKG = "./cmd/silt"

REQUIRED_FIELDS = ("lane", "symbol", "label", "claim", "substantial")
PROSE_FIELDS = ("claim", "substantial")

# A written reason is a sentence, not a shrug. Anything shorter than this, or in the
# null set, is an unreasoned row.
MIN_REASON_CHARS = 40
NULL_REASONS = {"n/a", "na", "none", "tbd", "todo", "-", "see above", "obvious", "?"}

# Ask the compiler to print an inline verdict for every declaration it compiles. A lane
# symbol must draw `cannot inline`: only then does its ABSENCE from the symbol table mean
# the linker dropped it rather than that a caller swallowed it.
GCFLAGS_INLINE = "-gcflags=-m=2"

# The phrase that licenses an absent symbol. Reserved for "the client half is not in
# the shipped binary".
CANNOT_PHRASE = "cannot be exercised"
# Phrases that claim the opposite — the lane is shippable but unrun. A label carrying
# both is ambiguous and fails.
NOT_YET_PHRASES = (
    "has not been exercised",
    "have not been exercised",
    "never exercised",
    "never been exercised",
    "has never run",
    "never run in the field",
    "not yet exercised",
)


# --------------------------------------------------------------------------- source

def code_mask(src):
    """Same-length copy of `src` with comments, strings and rune literals blanked to
    spaces (newlines kept). Brace and paren counting on the mask cannot be thrown by a
    brace inside a string or an apostrophe in a comment."""
    out = []
    i, n = 0, len(src)
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
            out.append("".join(ch if ch == "\n" else " " for ch in src[i:j]))
            i = j
            continue
        if c in ('"', "`", "'"):
            q = c
            j = i + 1
            while j < n:
                if q != "`" and src[j] == "\\":
                    j += 2
                    continue
                if src[j] == q:
                    j += 1
                    break
                j += 1
            out.append("".join(ch if ch == "\n" else " " for ch in src[i:j]))
            i = j
            continue
        out.append(c)
        i += 1
    return "".join(out)


def module_path():
    for line in (ROOT / "go.mod").read_text().splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    raise SystemExit("go.mod has no module line")


def split_symbol(sym):
    """`github.com/m/core/node.(*Node).SubmitRelayPay` -> (pkg, recv, func).
    `github.com/m/core/x.Foo` -> (pkg, None, 'Foo'). Returns None if unparseable."""
    i = sym.find(".(")
    if i >= 0:
        pkg = sym[:i]
        rest = sym[i + 1:]                      # "(*Node).SubmitRelayPay"
        m = re.match(r"\((\*?)(\w+)\)\.(\w+)$", rest)
        if not m:
            return None
        return pkg, m.group(2), m.group(3)
    i = sym.rfind(".")
    if i < 0:
        return None
    return sym[:i], None, sym[i + 1:]


def fname_of(sym):
    parsed = split_symbol(sym)
    return parsed[2] if parsed else sym


def table_name(sym, mod):
    """The name `go tool nm` prints for `sym`. A record spells the IMPORT PATH, because
    that is what locates the declaration in source; the linker spells the PACKAGE NAME,
    and for `./cmd/silt` that is `main`. Without this, a record naming a cmd/silt symbol
    reads as ABSENT from a binary that plainly contains it."""
    main_path = mod + MAIN_PKG.lstrip(".")
    if sym.startswith(main_path + "."):
        return "main." + sym[len(main_path) + 1:]
    return sym


def find_declaration(pkg_dir, recv, fname):
    """Locate `func (r *Recv) fname(` or `func fname(` in a non-test .go file under
    pkg_dir. Returns (relpath, line_no) or None. A record that no longer resolves to a
    declaration is a rotted claim, and the location is what every failure text cites."""
    if recv:
        sig = re.compile(r"^func\s+\(\s*\w+\s+\*?" + re.escape(recv) + r"\s*\)\s*"
                         + re.escape(fname) + r"\s*\(", re.MULTILINE)
    else:
        sig = re.compile(r"^func\s+" + re.escape(fname) + r"\s*\(", re.MULTILINE)
    if not pkg_dir.is_dir():
        return None
    for path in sorted(pkg_dir.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        src = path.read_text(encoding="utf-8", errors="replace")
        mask = code_mask(src)
        m = sig.search(mask)
        if not m:
            continue
        # Walk to the body's opening brace: the first `{` seen at paren depth 0 after
        # the signature. A wrapped signature (params over several lines) is handled.
        depth = 0
        i = m.end() - 1
        while i < len(mask):
            ch = mask[i]
            if ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
            elif ch == "{" and depth == 0:
                break
            i += 1
        else:
            return None
        rel = path.relative_to(ROOT).as_posix()
        return rel, src[:m.start()].count("\n") + 1
    return None


# ----------------------------------------------------------------------- the inliner

def go_cmd(args):
    """`go <args>`, throttled on the shared dev box. On macOS `nice` alone does not
    throttle; the background QoS class does. CI has no taskpolicy and wants full speed."""
    env = dict(os.environ)
    cmd = ["go"] + args
    if shutil.which("taskpolicy"):
        cmd = ["taskpolicy", "-c", "background", "nice", "-n", "19"] + cmd
        env.setdefault("GOFLAGS", "-p=1")
        env.setdefault("GOMAXPROCS", "2")
    return cmd, env


def inline_verdicts(pkg):
    """Compile `pkg` with the inliner reporting, and return its verdict per declaration:
    {printed name: True if the compiler CAN inline it}. The printed name is what the
    compiler writes — `(*Chain).CheckConsensusParams`, `Build`, `f.func1` — so a method
    and a package-level function of the same name stay distinct, and so does a closure.

    Only `pkg` is compiled under these flags; its dependencies come from the build cache
    at the flags the ./cmd/silt build already used."""
    cmd, env = go_cmd(["build", GCFLAGS_INLINE, "-o", os.devnull, pkg])
    r = subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True, text=True)
    if r.returncode != 0:
        return None, (f"`{' '.join(cmd)}` failed — the gate cannot measure whether this "
                      f"package's symbols are inlinable:\n{r.stderr.strip()}")
    verdicts = {}
    for line in r.stderr.splitlines():
        m = re.search(r"\b(can|cannot) inline (\S+?)(?: with cost \d+|:) ", line)
        if m:
            verdicts.setdefault(m.group(2), m.group(1) == "can")
    return verdicts, None


# --------------------------------------------------------------------------- binary

def build_binary(out_path):
    """Build ./cmd/silt."""
    cmd, env = go_cmd(["build", "-o", str(out_path), MAIN_PKG])
    r = subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True, text=True)
    if r.returncode != 0:
        print(f"FAIL [{SCAR_ID}] — `{' '.join(cmd)}` failed:\n{r.stderr}", file=sys.stderr)
        return False
    return True


def symbol_table(binary):
    r = subprocess.run(["go", "tool", "nm", str(binary)], cwd=ROOT,
                       capture_output=True, text=True)
    if r.returncode != 0:
        print(f"FAIL [{SCAR_ID}] — `go tool nm` failed:\n{r.stderr}", file=sys.stderr)
        return None
    names = set()
    for line in r.stdout.splitlines():
        parts = line.split()
        if parts:
            names.add(parts[-1])
    return names


# ------------------------------------------------------------------------ checklist

def label_block(text, anchor):
    """The markdown block (bullet or paragraph) containing `anchor`. Returns
    (block, error). A block ends at a blank line or at the next bullet at the same or
    shallower indent."""
    hits = text.count(anchor)
    if hits == 0:
        return None, "label anchor not found in the checklist"
    if hits > 1:
        return None, f"label anchor is ambiguous — {hits} occurrences in the checklist"
    lines = text.splitlines()
    idx = next((i for i, ln in enumerate(lines) if anchor in ln), None)
    if idx is None:
        # The anchor spans a soft-wrapped line break; fall back to a paragraph window.
        p = text.index(anchor)
        start = text.rfind("\n\n", 0, p) + 2
        end = text.find("\n\n", p)
        return text[start:end if end > 0 else len(text)], None

    def indent_of(s):
        return len(s) - len(s.lstrip(" "))

    def is_bullet(s):
        return re.match(r"^\s*(?:[-*+]|\d+\.)\s", s) is not None

    start = idx
    while start > 0 and not is_bullet(lines[start]) and lines[start].strip():
        start -= 1
    base = indent_of(lines[start]) if is_bullet(lines[start]) else 0
    end = idx + 1
    while end < len(lines):
        ln = lines[end]
        if not ln.strip():
            break
        if is_bullet(ln) and indent_of(ln) <= base:
            break
        end += 1
    return "\n".join(lines[start:end]), None


def classify(block):
    """CANNOT | NOT_YET | AMBIGUOUS. Closed complement: the cannot-phrase decides, and
    a block that claims both ways decides nothing."""
    low = " ".join(block.lower().split())
    can = CANNOT_PHRASE in low
    not_yet = any(p in low for p in NOT_YET_PHRASES)
    if can and not_yet:
        return "AMBIGUOUS"
    return "CANNOT" if can else "NOT_YET"


# ---------------------------------------------------------------------------- lanes

def parse_lanes(path):
    """Blank-line-separated records of `key: value`. Returns (records, errors)."""
    records, errors = [], []
    rec, first_line = {}, 0

    def flush():
        nonlocal rec, first_line
        if rec:
            records.append((first_line, rec))
        rec, first_line = {}, 0

    for n, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if raw.lstrip().startswith("#"):
            continue
        line = raw.rstrip()
        if not line.strip():
            flush()
            continue
        if ":" not in line:
            errors.append(f"{path.name}:{n}: not a `key: value` line: {raw.strip()!r}")
            continue
        key, val = line.split(":", 1)
        key = key.strip().lower()
        if not rec:
            first_line = n
        rec[key] = val.strip()
    flush()
    return records, errors


# --------------------------------------------------------------------------- report

def report(errors, checked):
    print(f"FAIL [{SCAR_ID}] — a lane's mechanism is not where the record says it is.\n\n"
          "  A mechanism with no non-test caller is DROPPED BY THE LINKER. It is not in\n"
          "  the shipped binary, so no operator can exercise it, however complete the\n"
          "  source looks. Six shipped inert this month.\n", file=sys.stderr)
    for e in errors:
        print(f"  {e}", file=sys.stderr)
    print(f"\n{len(errors)} problem(s); {checked} lane entr(ies) reached the binary check.",
          file=sys.stderr)


def check_structure(records, lanes_path):
    """Fields, written reasons, uniqueness. Runs before anything expensive is built."""
    errors, seen = [], set()
    for line_no, rec in records:
        where = f"{lanes_path.name}:{line_no}"
        missing = [f for f in REQUIRED_FIELDS if not rec.get(f)]
        if missing:
            errors.append(f"{where}: record is missing required field(s): "
                          f"{', '.join(missing)}  [{rec.get('lane', '?')} / "
                          f"{rec.get('symbol', '?')}]")
            continue
        extra = set(rec) - set(REQUIRED_FIELDS)
        if extra:
            errors.append(f"{where}: unknown field(s): {', '.join(sorted(extra))}")
        for f in PROSE_FIELDS:
            v = rec[f].strip()
            if v.lower().rstrip(".") in NULL_REASONS or len(v) < MIN_REASON_CHARS:
                errors.append(
                    f"{where}: `{f}` is not a written reason ({v!r}). An unreasoned row "
                    f"FAILS — an allowlist rationale is itself a claim.")
        key = (rec["lane"], rec["symbol"])
        if key in seen:
            errors.append(f"{where}: duplicate entry for {key[0]} / {key[1]}")
        seen.add(key)
    return errors


def check_substantial(records, lanes_path, mod):
    """Refuse any entry pointing at a symbol the compiler is willing to inline. Returns
    (declarations, errors)."""
    decls, errors, verdict_cache = {}, [], {}
    for line_no, rec in records:
        where = f"{lanes_path.name}:{line_no}"
        sym = rec["symbol"]
        parsed = split_symbol(sym)
        if not parsed:
            errors.append(f"{where}: cannot parse symbol {sym!r} — expected a fully "
                          f"qualified `<import path>.Func` or "
                          f"`<import path>.(*Type).Method`")
            continue
        pkg, recv, fname = parsed
        if not (pkg == mod or pkg.startswith(mod + "/")):
            errors.append(f"{where}: {sym} is outside module {mod} — this gate covers "
                          f"silt's own lanes")
            continue
        pkg_dir = ROOT / pkg[len(mod):].lstrip("/")
        found = find_declaration(pkg_dir, recv, fname)
        if not found:
            errors.append(f"{where}: no declaration of {sym} in "
                          f"{pkg_dir.relative_to(ROOT)}/ — a lane entry that no longer "
                          f"resolves to source is a rotted claim")
            continue
        rel, dline = found

        # THE INLINER'S OWN VERDICT. One compile per distinct lane package, cached
        # across the records that share it.
        if pkg not in verdict_cache:
            verdict_cache[pkg], build_err = inline_verdicts(pkg)
            if build_err:
                errors.append(f"{where}: {build_err}")
        verdicts = verdict_cache[pkg]
        if verdicts is None:
            continue
        printed = f"(*{recv}).{fname}" if recv else fname
        can = verdicts.get(printed)
        if can is None:
            can = verdicts.get(f"{recv}.{fname}") if recv else None
        if can is None:
            errors.append(
                f"{where}: the compiler printed NO inline verdict for {printed} while "
                f"building {pkg} ({rel}:{dline} declares it).\n"
                f"    This gate's whole assertion rests on knowing that an ABSENT symbol "
                f"was dropped by the linker rather than swallowed by a caller, and "
                f"without a verdict it does not know. Check the symbol spelling.")
            continue
        if can:
            errors.append(
                f"{where}: REFUSING this entry — the compiler CAN INLINE {printed} "
                f"({rel}:{dline}).\n"
                f"    An inlinable symbol can vanish from the table of a binary where the "
                f"lane is perfectly live, so its absence proves nothing and the gate would "
                f"report a live lane as inert.\n"
                f"    THE REMEDY IS TO REPOINT, NOT TO DELETE. This fires on a refactor "
                f"that made {printed} cheaper without taking the lane out of the binary "
                f"— folding two calls into one is enough. In that case name the "
                f"NON-INLINABLE function this one calls (the mechanism itself, not its "
                f"wrapper) and keep the record: the reachability signal is identical "
                f"whenever the named callee has no other production caller, and the "
                f"margin stops being an arithmetic accident. `go build -gcflags=-m=2 "
                f"{pkg}` prints every candidate's cost.\n"
                f"    Deleting the record, or relaxing this rule, is how the gate stops "
                f"meaning anything. If the lane truly has NO substantial entry point, "
                f"that is a finding about the lane — file it, do not silence it.")
            continue
        decls[(rec["lane"], sym)] = (rel, dline)
    return decls, errors


def check_binary(records, lanes_path, decls, syms, checklist, checklist_name, mod):
    """The assertion itself, one lane entry at a time."""
    errors = []
    present = excused = 0
    for line_no, rec in records:
        where = f"{lanes_path.name}:{line_no}"
        lane, sym = rec["lane"], rec["symbol"]
        tname = table_name(sym, mod)
        rel, dline = decls[(lane, sym)]
        block, label_err = label_block(checklist, rec["label"])

        # EVERY record's label must resolve, present or absent. `claim` is prose; the
        # anchor resolving in the checklist is the mechanical half of it. An anchor that
        # no longer points at anything is a rationale that has rotted — the exact thing
        # a written reason is supposed to prevent.
        if label_err:
            errors.append(
                f"{where}: {lane}: {label_err} — {rec['label']!r}.\n"
                f"    This record's `claim` says the lane backs a public claim in "
                f"{checklist_name}; the anchor is the only mechanical hold on that. "
                f"Re-point `label` at the lane's posture line, or remove the record.\n"
                f"    (symbol {sym} is "
                f"{'PRESENT in' if tname in syms else 'ABSENT from'} the binary.)")
            continue

        # THE ASSERTION, as one equality with a closed complement:
        #
        #     the label says THIS SYMBOL is unreachable  ==  the symbol is absent
        #
        # The label's claim is the PAIR (the phrase "cannot be exercised", the symbol's
        # name). Both halves are needed. Without the phrase it is a not-yet claim; without
        # the name it is a claim about some other part of the lane, and a lane commonly
        # has a live half and a dropped half — the paid delivery lane opens and settles
        # but cannot top up. Scoping the label's claim to the symbol it names is what
        # lets one posture line carry both facts without either going unchecked.
        verdict = classify(block)
        names_symbol = fname_of(sym) in block
        says_unreachable = verdict == "CANNOT" and names_symbol
        ambiguous_about_symbol = verdict == "AMBIGUOUS" and names_symbol

        if tname in syms:
            if says_unreachable or ambiguous_about_symbol:
                errors.append(
                    f"{where}: {lane}: {sym} IS in the shipped binary, but its label in "
                    f"{checklist_name} names it and says \"{CANNOT_PHRASE}\".\n"
                    f"    The label has rotted past the code. That phrase is reserved for a "
                    f"client entry point that is NOT in the binary; this one is linked. "
                    f"Re-label to whatever is now true "
                    f"(\"built, sim-proven, never exercised on a real network\" if the lane "
                    f"has not run in the field).\n"
                    f"    anchor: {rec['label']!r}")
                continue
            present += 1
            continue

        # ABSENT.
        dropped = (f"{sym} is ABSENT from the shipped binary ({rel}:{dline} declares it; "
                   f"the linker dropped it — no non-test caller)")
        if says_unreachable:
            excused += 1
            continue
        if verdict == "NOT_YET":
            why = (f'the label says the lane has NOT YET been exercised, not that it '
                   f'"{CANNOT_PHRASE}"')
        elif verdict == "AMBIGUOUS":
            why = (f'the label claims BOTH "{CANNOT_PHRASE}" and not-yet-exercised, so it '
                   f'states no posture at all')
        else:
            why = (f'the label says "{CANNOT_PHRASE}" but does not name '
                   f'`{fname_of(sym)}` — a label about a lane must say WHICH client entry '
                   f'point is missing, or the claim cannot be re-checked')
        errors.append(
            f"{where}: {lane}: {dropped}, and {why}.\n"
            f"    A lane whose client entry point is not even linked cannot be run by "
            f"anyone. Labelling it \"never exercised\" implies it could be run tomorrow; "
            f"that is the over-claim this gate exists to refuse.\n"
            f"    Either wire the mechanism to a real caller, or re-label the lane in "
            f"{checklist_name} to say it {CANNOT_PHRASE} and name `{fname_of(sym)}`.\n"
            f"    label read (anchor {rec['label']!r}):\n"
            + "\n".join("      | " + l for l in block.splitlines()))
    return present, excused, errors


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--lanes", default=str(LANES_FILE))
    ap.add_argument("--checklist", default=str(CHECKLIST))
    ap.add_argument("--binary", default=None,
                    help="use this prebuilt ./cmd/silt instead of building one")
    args = ap.parse_args()

    lanes_path, checklist_path = Path(args.lanes), Path(args.checklist)
    records, errors = parse_lanes(lanes_path)
    if not records and not errors:
        errors.append(f"{lanes_path.name}: no lane records — an empty gate is not a gate")
    errors += check_structure(records, lanes_path)
    if errors:
        report(errors, 0)
        return 1

    decls, errors = check_substantial(records, lanes_path, module_path())
    if errors:
        report(errors, 0)
        return 1

    tmpdir = None
    if args.binary:
        binary = Path(args.binary)
        if not binary.exists():
            print(f"FAIL [{SCAR_ID}] — --binary {binary} does not exist", file=sys.stderr)
            return 1
    else:
        tmpdir = tempfile.mkdtemp(prefix="silt-reach-")
        binary = Path(tmpdir) / "silt"
        if not build_binary(binary):
            shutil.rmtree(tmpdir, ignore_errors=True)
            return 1
    syms = symbol_table(binary)
    if tmpdir:
        shutil.rmtree(tmpdir, ignore_errors=True)
    if syms is None:
        return 1

    checklist = checklist_path.read_text(encoding="utf-8")
    present, excused, errors = check_binary(records, lanes_path, decls, syms,
                                            checklist, checklist_path.name, module_path())
    if errors:
        report(errors, len(records))
        return 1

    print(f"OK [{SCAR_ID}] — {len(records)} lane entr(ies) checked against the linked "
          f"{MAIN_PKG} binary, each a symbol the compiler refuses to inline: "
          f"{present} present, "
          f'{excused} absent and labelled "{CANNOT_PHRASE}" in {checklist_path.name}.\n'
          f"     Reachability is NECESSARY and never SUFFICIENT: this proves the symbols "
          f"survived linking, not that any mechanism is correct.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
