#!/usr/bin/env python3
"""THE ADVERSARY-SHAPE GATE — roadmap row F1, `D-STRUCTURAL-GATES-2026-09-12`.

  ⚠ NAME: this gate is called THE ADVERSARY-SHAPE GATE. Some records call it "G-2".
    Do not use that spelling anywhere. `R-membership` has its own G-1/G-2/G-3 family
    and `R-membership`'s G-1 is a CLOSED, DIFFERENT object; a document was nearly
    corrupted by the collision. Write the name out in full, always.

THE RULE THIS GATE ENCODES

  For every defence silt claims, a fixture must exist in which the adversary HAS the
  capability the defence assumes it lacks.

WHAT IT COVERS, AND WHAT IT DOES NOT — read this before citing it

  ▶ Do not describe this gate as covering "the claim". IT COVERS A VOCABULARY.

  It finds the sentences in the scoped source that SAY an adversary cannot do
  something, and it holds each of those sentences to a declaration. A defence that is
  real but never written down in this vocabulary is INVISIBLE to it. A defence written
  as "only a care-link holder can derive it" is caught (`only ... can` is in the
  vocabulary); one written as "the key stays with the auditor" is not.

  It CANNOT decide that a fixture's adversary genuinely holds a capability. That is not
  statically decidable, and pretending otherwise is how a gate becomes decoration. What
  it enforces instead is a PROXY with two legs (see CHECK 3): the fixture must GRANT the
  capability under a marker, and it must carry a NEGATIVE CONTROL showing the same
  attack fails without it. A fixture that lies in both markers defeats this gate. The
  control leg is what makes lying expensive: a control that does not actually remove the
  capability produces a passing control and the fixture's own author sees it.

  It is a SOURCE GATE. It observes text, never behaviour: it can say that no fixture
  claims to grant a capability, never that the shipped code resists one. As of
  2026-09-12 NOTHING in the tree is declared -- all 24 in-scope claims report as
  UNDECLARED -- so this gate currently has no runtime cover to name, and must not be
  cited as evidence that any defence holds.

WHY THE COMPLEMENT IS CLOSED ON THE SIDE THAT MATTERS

  The claim inventory is DERIVED from the tracked source on every run, never hand-listed
  (scar-inventory-gate-is-a-hand-list: a "covers EVERY X" gate built from a hand list
  silently stops covering X the moment X grows). Add a defence to a scoped file and
  write it in the vocabulary, and it enters the inventory and FAILS CLOSED until it is
  declared. That is the half a gate can own. The other half — writing the claim in a
  vocabulary nobody checks — it cannot.

  The walk is `git ls-files` on this checkout, NOT a filesystem walk. That is one
  mechanism answering two recorded failures at once: a filesystem walk descends into
  every nested agent worktree under this root and judges another branch's files by
  this branch's rules (scar:lint-walks-into-another-checkout-2026-09-11, measured at
  610 false findings from one lint), and a filesystem walk also reads GITIGNORED
  artifacts (scar-source-gate-walks-gitignored-artifacts, 2026-09-12: a month-old
  gitignored artifact made local `main` RED on a SHA whose CI was 14/14 green).
  `git ls-files` reports neither. It does mean an UNSTAGED new file is invisible to
  this gate until it is added — which is correct for a gate that judges the tree,
  and is also why this gate cannot see the three untracked PoR evidence files.

THE DECLARATION FORMAT

  In the same comment block as the claim:

    ADVERSARY-SHAPE: capability=<Name> fixture=<TestName>
    ADVERSARY-SHAPE: capability=<Name> UNCOVERED: <reason, with a residual or run id>
    ADVERSARY-SHAPE: NOT-A-DEFENCE: <reason>

  The third form is the honest way to spend the vocabulary's false positives. Measured
  at b870ade: 24 matches in scope, of which about 19 are genuine defence claims. The
  alternative -- tightening the regex once per false positive -- is the
  "pattern that must be re-escaped per instance" shape row F1 exists to avoid, and it
  quietly narrows coverage every time it is used. A NOT-A-DEFENCE line is one reviewable
  line in a diff and it narrows nothing.

  In the named fixture:

    ADVERSARY-HOLDS: <Name>        -- this fixture's adversary is GRANTED <Name>
    CAPABILITY-CONTROL: <Name>     -- the same attack WITHOUT <Name> is asserted to fail

Dependency-free (stdlib only). Run: python3 scripts/check_adversary_shape.py
"""
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

GATE_NAME = "the adversary-shape gate"
ROADMAP_ROW = "ROADMAP.md row F1"
DECISION = "D-STRUCTURAL-GATES-2026-09-12"

# ---------------------------------------------------------------------------
# SCOPE. The storage/proof surface, which is where the measured failure lives.
# Stated as directories and files so a new file in a scoped package is IN scope
# automatically; a new PACKAGE is not, and that limit is real.
# ---------------------------------------------------------------------------
SCOPE = [
    "core/por",
    "core/repairproof",
    "core/bond",
    "core/node/por.go",
    "core/node/repairclaim.go",
    "core/node/proofbacking.go",
    "core/node/bondaudit.go",
    "adapters/diskproofs",
    "adapters/memproofs",
    "adapters/proofcache",
]

# ---------------------------------------------------------------------------
# THE VOCABULARY. Two CLOSED token sets. A sentence is an incapability claim iff
# it carries a token from each. Closed sets, not open regex: a reader can hold
# the whole predicate in their head, and widening it is a visible diff.
# ---------------------------------------------------------------------------
# THE VOCABULARY. The predicate is ADJACENCY, not co-occurrence. An earlier
# draft asked only that a negation token and an adversary token appear in the
# same sentence; it returned 46 findings of which about half were prose like
# "lock-free because it is only ever touched from the node's single loop".
# A lint that reports noise is a lint nobody runs, and an unrun lint is not a
# gate (repo_walk.py's own docstring makes the same point about 610 false
# findings). So the claim must be SUBJECT-then-NEGATION within a short window,
# which is where a defence claim actually puts them.
#
# Two closed forms, and only two:
#   A.  <adversary-subject> ... cannot/can't/never <verb>     ("a prover cannot forge")
#   B.  only <...> can <verb>                                  ("only a care-link holder can derive it")
#
# Form B is in because an exclusive-capability sentence is an incapability
# claim about everyone else, and core/node/por.go's porKeyDomain — the site of
# the measured failure — is written in exactly that form.
SUBJECT = (
    r"(?:adversar\w*|attacker|prover|liar|claimant|forger|sybil|storage node|"
    r"data-less \w+|repairer|holder|caretaker|publisher|peer|party|node)"
)
NEGATION = r"(?:cannot|can't|can not|could not|never|is unable to|are unable to)"
# Window: the negation must follow the subject within 80 characters. Wider and
# it re-admits the co-occurrence noise; narrower and it drops
# "a prover that released the space cannot produce the seed".
FORM_A = re.compile(rf"\b{SUBJECT}\b.{{0,80}}?\b{NEGATION}\b", re.I | re.S)
FORM_B = re.compile(r"\bonly\b[^.;]{0,70}?\bcan\b", re.I | re.S)


def is_claim(sentence):
    return bool(FORM_A.search(sentence) or FORM_B.search(sentence))


# Three declaration forms, and exactly three. The third exists because the
# vocabulary has a measured ~79% precision on the real tree (19 genuine claims
# of 24 matches at b870ade), and the honest way to spend the other 21% is an
# explicit, diff-visible denial — NOT a per-instance tightening of the regex,
# which is the "pattern that must be re-escaped per instance" shape row F1
# tells this gate to avoid.
DECL_RE = re.compile(
    r"ADVERSARY-SHAPE:\s*(?:"
    r"capability=(?P<cap>[A-Za-z0-9_.\-]+)\s+"
    r"(?:fixture=(?P<fix>Test[A-Za-z0-9_]+)|UNCOVERED:\s*(?P<unc>\S.*?))"
    r"|NOT-A-DEFENCE:\s*(?P<nad>\S.*?)"
    r")\s*$", re.M)
HOLDS_RE = re.compile(r"ADVERSARY-HOLDS:\s*([A-Za-z0-9_.\-]+)")
CONTROL_RE = re.compile(r"CAPABILITY-CONTROL:\s*([A-Za-z0-9_.\-]+)")
FUNC_RE = re.compile(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(", re.M)


def tracked_files(root=None):
    """Files git TRACKS in this checkout. Never the filesystem: a gitignored
    artifact is not source, and a gate that reads one reports a failure whose
    fix is `rm`."""
    out = subprocess.run(["git", "-C", str(root or ROOT), "ls-files", "-z"],
                         capture_output=True, text=True, check=True).stdout
    return [p for p in out.split("\0") if p]


def in_scope(rel):
    return any(rel == s or rel.startswith(s.rstrip("/") + "/") for s in SCOPE)


def comment_blocks(text):
    """Yield (first_line_no, block_text) for each run of consecutive `//` lines.
    A declaration binds to the BLOCK, so it may sit on its own line beside the
    sentence it answers."""
    lines = text.splitlines()
    i, n = 0, len(lines)
    while i < n:
        if lines[i].lstrip().startswith("//"):
            j = i
            while j < n and lines[j].lstrip().startswith("//"):
                j += 1
            yield i + 1, "\n".join(lines[i:j])
            i = j
        else:
            i += 1


def sentences(block):
    stripped = " ".join(re.sub(r"^\s*//\s?", "", ln) for ln in block.splitlines())
    return [s.strip() for s in re.split(r"(?<=[.;])\s+", stripped) if s.strip()]


def main(root=None, quiet=False):
    root = Path(root) if root else ROOT
    files = tracked_files(root)
    # Every test function in the tracked tree, name -> file. Built once; a
    # citation resolves against it (scar-cited-gate-does-not-exist, count=5).
    tests = {}
    for rel in files:
        if not rel.endswith("_test.go"):
            continue
        p = root / rel
        try:
            body = p.read_text(errors="replace")
        except OSError:
            continue
        for m in FUNC_RE.finditer(body):
            tests[m.group(1)] = (rel, body)

    claims, problems = [], []
    for rel in sorted(files):
        if not rel.endswith(".go") or rel.endswith("_test.go") or not in_scope(rel):
            continue
        text = (root / rel).read_text(errors="replace")
        for lineno, block in comment_blocks(text):
            hit = [s for s in sentences(block) if is_claim(s)]
            if not hit:
                continue
            decl = DECL_RE.search(block)
            claims.append((rel, lineno, hit[0], decl))
            if decl is None:
                problems.append(
                    f"{rel}:{lineno}: UNDECLARED DEFENCE CLAIM. The comment asserts an "
                    f"adversary incapability:\n      \"{hit[0][:150]}\"\n"
                    f"    Every such claim needs a fixture in which the adversary HAS the "
                    f"capability it assumes is absent. Add to this comment block:\n"
                    f"      // ADVERSARY-SHAPE: capability=<Name> fixture=<TestName>\n"
                    f"    or, if no such fixture exists, record that honestly:\n"
                    f"      // ADVERSARY-SHAPE: capability=<Name> UNCOVERED: <reason>")
                continue
            if decl.group("nad") is not None:
                continue  # declared not a defence claim; the denial is in the diff
            cap = decl.group("cap")
            if decl.group("unc") is not None:
                problems.append(
                    f"{rel}:{lineno}: DECLARED UNCOVERED — capability={cap}. "
                    f"No fixture grants the adversary this capability.\n"
                    f"    reason: {decl.group('unc')[:200]}\n"
                    f"    This is a RECORD, not an exemption. {GATE_NAME} stays RED until a "
                    f"capability-holding fixture exists ({ROADMAP_ROW}, {DECISION}).")
                continue
            fix = decl.group("fix")
            if fix not in tests:
                problems.append(
                    f"{rel}:{lineno}: CITED FIXTURE DOES NOT EXIST — {fix} "
                    f"(capability={cap}). No `func {fix}(` in any tracked *_test.go.")
                continue
            frel, fbody = tests[fix]
            holds = set(HOLDS_RE.findall(fbody))
            ctrl = set(CONTROL_RE.findall(fbody))
            if cap not in holds:
                problems.append(
                    f"{rel}:{lineno}: FIXTURE DOES NOT GRANT THE CAPABILITY — {fix} "
                    f"({frel}) carries no `ADVERSARY-HOLDS: {cap}`.\n"
                    f"    A fixture whose adversary holds LESS than a legitimate participant "
                    f"cannot witness this defence; that is the exact shape {ROADMAP_ROW} exists "
                    f"to catch. Declared holds: {sorted(holds) or 'none'}")
                continue
            if cap not in ctrl:
                problems.append(
                    f"{rel}:{lineno}: FIXTURE HAS NO CAPABILITY CONTROL — {fix} "
                    f"({frel}) carries no `CAPABILITY-CONTROL: {cap}`.\n"
                    f"    Without a control asserting the SAME attack fails WITHOUT {cap}, the "
                    f"fixture cannot show the capability is load-bearing, and a bystander passes "
                    f"as a witness (scar-gate-passes-on-a-bystander, count=3, third-time fired).")

    if quiet:
        return 1 if problems else 0
    print(f"{GATE_NAME}: scanned {len(files)} tracked files, "
          f"{sum(1 for r in files if in_scope(r) and r.endswith('.go') and not r.endswith('_test.go'))} "
          f"in scope; {len(claims)} defence claims found; {len(tests)} test functions resolvable.")
    if not problems:
        print(f"{GATE_NAME}: OK — every defence claim in scope names a capability-holding fixture.")
        return 0
    print(f"\n{GATE_NAME}: {len(problems)} problem(s).\n")
    for p in problems:
        print("  - " + p + "\n")
    print(f"  Authority: {ROADMAP_ROW}, docs/decisions.md {DECISION}.")
    return 1




# ---------------------------------------------------------------------------
# SELF-TEST. The real tree can only ever exhibit the FAILING directions — every
# defence claim in scope is uncovered today, which is the finding. A gate whose
# passing direction has never been observed is a gate that might pass on
# nothing, so the passing direction is MANUFACTURED here, the way
# repo_walk.py --self-test manufactures the nested checkout CI can never show.
#
# Each case gets its OWN temp repo and its own exit code. Sharing one tree would
# let a single failing case mask every other verdict, which is the same
# two-failures-one-label shape that has already cost this project a session.
#
# THE TWO TEETH LEGS MUST BE INDEPENDENTLY OBSERVABLE. Measured 2026-09-12: an
# earlier self-test had eight cases and could not see the grant leg at all --
# disabling it left every case green, because the bystander case then failed on
# the CONTROL leg instead and reported the same verdict for a different reason.
# "Fixture controls but does not grant" isolates it. Two legs that can only ever
# fail together are one leg wearing two names.
#
# Case 5 is the one that matters most: its fixture is a faithful reduction of a
# REAL tracked test, core/por TestForgeryWithoutKeyFails, whose adversary holds
# strictly LESS than an honest holder (it corrupts data it already has and
# reuses an honest sigma; the name itself says "WithoutKey"). The gate must
# refuse it as a witness. A gate that accepts a bystander is the failure shape
# recorded three times over in this project's scar ledger.
# ---------------------------------------------------------------------------
CASES = [
    ("undeclared claim", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\npackage por\n"}),
    ("declared UNCOVERED", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey UNCOVERED: no fixture grants it (run 2026-09-12)\npackage por\n"}),
    ("cited fixture does not exist", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestNoSuchFixture\npackage por\n"}),
    ("bystander fixture (real shape: TestForgeryWithoutKeyFails)", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestForgeryWithoutKeyFails\npackage por\n",
        "core/por/x_test.go": "package por\n"
                              "// Adversary corrupts the data it holds, then reuses the honest sigma.\n"
                              "func TestForgeryWithoutKeyFails(t *testing.T) { _ = 0 }\n"}),
    ("fixture controls but does NOT grant (isolates the grant leg)", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestControlsButHoldsNothing\npackage por\n",
        "core/por/x_test.go": "package por\n// CAPABILITY-CONTROL: LayoutKey -- same attack without it must fail\n"
                              "func TestControlsButHoldsNothing(t *testing.T) { _ = 0 }\n"}),
    ("fixture holds the capability but has no control", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestKeyHoldingProver\npackage por\n",
        "core/por/x_test.go": "package por\n// ADVERSARY-HOLDS: LayoutKey\n"
                              "func TestKeyHoldingProver(t *testing.T) { _ = 0 }\n"}),
    ("fixture holds the capability AND controls for it", 0, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestKeyHoldingProver\npackage por\n",
        "core/por/x_test.go": "package por\n// ADVERSARY-HOLDS: LayoutKey\n"
                              "// CAPABILITY-CONTROL: LayoutKey -- same attack without it must fail\n"
                              "func TestKeyHoldingProver(t *testing.T) { _ = 0 }\n"}),
    ("declared NOT-A-DEFENCE", 0, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: NOT-A-DEFENCE: this sentence is a note about padding, not a claim\npackage por\n"}),
    ("no claim vocabulary at all (over-match control)", 0, {
        "core/por/x.go": "// Blocks reports how many blocks a unit of n bytes splits into.\npackage por\n"}),
    ("claim OUTSIDE the declared scope is not read", 0, {
        "core/credit/x.go": "// a prover that dropped the bytes cannot make the answer verify.\npackage credit\n"}),
]


def self_test():
    import tempfile
    bad = 0
    for name, want, files in CASES:
        with tempfile.TemporaryDirectory() as td:
            td = Path(td)
            subprocess.run(["git", "-C", str(td), "init", "-q"], check=True)
            for rel, body in files.items():
                (td / rel).parent.mkdir(parents=True, exist_ok=True)
                (td / rel).write_text(body)
            subprocess.run(["git", "-C", str(td), "add", "-A"], check=True,
                           capture_output=True)
            got = main(root=td, quiet=True)
            ok = got == want
            bad += 0 if ok else 1
            print(f"  [{'ok' if ok else 'FAIL'}] want exit {want}, got {got} -- {name}")
    print(f"{GATE_NAME} self-test: {len(CASES) - bad}/{len(CASES)} cases correct")
    return 1 if bad else 0


if __name__ == "__main__":
    if "--self-test" in sys.argv:
        sys.exit(self_test())
    sys.exit(main())
