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
  claims to grant a capability, never that the shipped code resists one. A `fixture=`
  declaration is NOT evidence that a defence holds; it is evidence that an adversary
  with the capability was built and measured. Never cite this gate for the former.

  MEASURED on the commit that adds ratchet mode: 24 in-scope claims, ZERO undeclared,
  4 carrying `fixture=`, 19 `UNCOVERED:` and 1 `NOT-A-DEFENCE:`. All 19 are on the
  ratchet allow-list, so the DEFAULT mode exits 0 and `--strict` exits 1 on the same
  tree. These counts move with the tree. Re-run the gate rather than quoting them.

RATCHET MODE — WHAT A GREEN RUN DOES AND DOES NOT MEAN

  ▶ A GREEN RUN MEANS: no defence claim in scope is NEW to this gate, and every
    `fixture=` resolves to a test that grants the capability and controls for it.
  ▶ A GREEN RUN DOES NOT MEAN: the claims are covered. 19 of them are grandfathered
    RECORDS of missing coverage. `--strict` prints every one and exits 1, which is
    exactly what the gate did before ratchet mode existed. Nothing was deleted.

  WHY THE OLD BEHAVIOUR COULD NOT BE KEPT. `CAPABILITY-CONTROL` asks that the same
  attack FAIL without the capability. Where a defence HOLDS, it fails without every
  capability, so the control cannot discriminate and a `fixture=` is only reachable
  when the defence is BROKEN (`R-ADVERSARY-SHAPE-CONTROL-NEEDS-A-BROKEN-DEFENCE`).
  The 19 therefore cannot be driven to zero by covering them, and a gate that can
  never go green cannot be wired to CI. Ratchet mode changes the assertion from
  "nothing is uncovered" to "nothing is NEW", which is a claim the tree can satisfy.

  THE COST, STATED RATHER THAN BURIED: a grandfathered allow-list is how a backlog
  becomes permanent. Nineteen entries that "may only shrink" have no forcing function
  in this file. Growth and shrinkage cost the identical two edits; only review tells
  them apart. See the RATCHET ALLOW-LIST block below for what the keying catches.

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
  this gate until it is added — which is correct for a gate that judges the tree. The
  three PoR evidence files that were untracked when this gate was written are TRACKED
  by the commit that lands it, so the gate now resolves the fixtures they carry.

THE DECLARATION FORMAT

  In the same comment block as the claim:

    ADVERSARY-SHAPE: capability=<Name> fixture=<TestName>
    ADVERSARY-SHAPE: capability=<Name> UNCOVERED: <reason, with a residual or run id>
    ADVERSARY-SHAPE: NOT-A-DEFENCE: <reason>

  The third form is the honest way to spend the vocabulary's false positives. An
  earlier draft of this docstring estimated "about 19 of 24" matches were genuine. The
  declaration pass that shipped with it classified 23 of the 24 as genuine and exactly
  1 as NOT-A-DEFENCE, so that estimate was wrong and is withdrawn: the rate is whatever
  the declarations say, and each one is reviewable line by line in a diff.
  The alternative -- tightening the regex once per false positive -- is the
  "pattern that must be re-escaped per instance" shape row F1 exists to avoid, and it
  quietly narrows coverage every time it is used. A NOT-A-DEFENCE line is one reviewable
  line in a diff and it narrows nothing.

  ONE KNOWN LIMIT, because a declaration can hide it.

  A comment block gets ONE declaration and the gate reports only the FIRST
  matching sentence in it. Four blocks carry more than one claim sentence, so the
  second and third are recorded under a capability name that may not describe them.
  Read the WHOLE block before trusting a declaration on a long one, and where the
  names diverge, say so inside the block (core/por/por.go's package header does).
  This limit also bounds the ratchet key: the key digests the FIRST matching
  sentence, so rewording one of its neighbours changes no key.

  A SECOND LIMIT WAS HERE AND IS NOW FIXED, recorded because the fix is what the
  binding-B self-test case defends. The grant and control legs used to bind to the
  FILE, not to the named function: main() stored each test name against its WHOLE
  file body. MEASURED 2026-09-12: re-pointing a `capability=LayoutKey` declaration
  at TestRT_POR_2_ChallengeProxyPassesAudit_PINNED_DEFECT — which does not grant
  LayoutKey — left the gate at 19 problems, NOT CAUGHT, because both RT-POR
  fixtures share core/node/rt_por_m1_gates_test.go. `marker_scopes` now binds each
  marker to ONE test. RE-MEASURED on the same patched tree: the pre-fix gate
  reports 19 and the post-fix gate reports 20, naming the mis-pointed fixture.
  What survives of the old warning: a fixture that lies in BOTH markers still
  defeats this gate, and `fixture=` is still a claim about a test, not a proof.

  In the named fixture:

    ADVERSARY-HOLDS: <Name>        -- this fixture's adversary is GRANTED <Name>
    CAPABILITY-CONTROL: <Name>     -- the same attack WITHOUT <Name> is asserted to fail

Dependency-free (stdlib only).
  python3 scripts/check_adversary_shape.py              ratchet mode (the default)
  python3 scripts/check_adversary_shape.py --strict     the full record; exits 1 today
  python3 scripts/check_adversary_shape.py --self-test  the manufactured cases
"""
import hashlib
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
# vocabulary has false positives. MEASURED ON THE TREE THIS COMMIT SHIPS: 24
# matches, of which the declaration pass classified 23 as genuine and exactly 1
# as NOT-A-DEFENCE. An earlier draft of this comment said "~79% precision (19
# genuine of 24)"; that estimate was wrong and is withdrawn — the docstring says
# so and this line used to contradict it. The rate is whatever the declarations
# say. The honest way to spend a false positive is an explicit, diff-visible
# denial — NOT a per-instance tightening of the regex, which is the "pattern
# that must be re-escaped per instance" shape row F1 tells this gate to avoid.
DECL_RE = re.compile(
    r"ADVERSARY-SHAPE:\s*(?:"
    r"capability=(?P<cap>[A-Za-z0-9_.\-]+)\s+"
    r"(?:fixture=(?P<fix>Test[A-Za-z0-9_]+)|UNCOVERED:\s*(?P<unc>\S.*?))"
    r"|NOT-A-DEFENCE:\s*(?P<nad>\S.*?)"
    r")\s*$", re.M)
HOLDS_RE = re.compile(r"ADVERSARY-HOLDS:\s*([A-Za-z0-9_.\-]+)")
CONTROL_RE = re.compile(r"CAPABILITY-CONTROL:\s*([A-Za-z0-9_.\-]+)")
FUNC_RE = re.compile(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(", re.M)
# Every TOP-LEVEL declaration, not just the test ones. It is what ends a span.
TOPFUNC_RE = re.compile(r"^func\b", re.M)


def marker_scopes(body):
    """name -> (holds, ctrl) for every Test function in one file, each marker
    bound to ONE test instead of to the whole file.

    THE BINDING RULE, and it is two clauses because the tree uses two placements:

      1. A marker INSIDE a top-level function's body binds to that function, and
         only if that function is a test. A marker inside a HELPER's body binds
         to no test and is dropped.
      2. A marker anywhere else — a comment block between declarations, which is
         where both RT-POR fixtures put theirs, above the test's own helpers —
         binds FORWARD to the next test DECLARED after it.

    A marker after the last test declaration in a file, and not inside a test
    body, binds to nothing.

    A body ends at the first line beginning with `}` at column 0. That is a
    gofmt property, not a parse; every tracked Go file in this repo is gofmt'd
    and CI enforces it. An un-gofmt'd file would over-extend a span, which
    over-credits rather than under-credits, so the failure direction is the
    loud one: the wrong test gets the marker and its own declaration goes RED.
    """
    lines = body.splitlines(keepends=True)
    offs, acc = [], 0
    for ln in lines:
        offs.append(acc)
        acc += len(ln)
    offs.append(acc)

    decls = []  # (test-name or None, start offset, end-of-body offset)
    for i, ln in enumerate(lines):
        if not ln.startswith("func "):
            continue
        m = FUNC_RE.match(ln)
        j = i + 1
        while j < len(lines) and not lines[j].startswith("}"):
            j += 1
        decls.append((m.group(1) if m else None, offs[i],
                      offs[min(j + 1, len(lines))]))

    named = [d for d in decls if d[0]]
    out = {d[0]: (set(), set()) for d in named}
    for rx, idx in ((HOLDS_RE, 0), (CONTROL_RE, 1)):
        for mk in rx.finditer(body):
            o = mk.start()
            inside = next((d for d in decls if d[1] <= o < d[2]), None)
            if inside is not None:
                owner = inside[0]
            else:
                nxt = next((d for d in named if d[1] > o), None)
                owner = nxt[0] if nxt else None
            if owner:
                out[owner][idx].add(mk.group(1))
    return out


def claim_key(rel, cap, sentence):
    """The RATCHET KEY. Path, capability, and a digest of the claim sentence.

    Line numbers are deliberately NOT in it: they move on every unrelated edit
    above the block, and a ratchet that fires on an innocuous diff is a ratchet
    somebody turns off. The digest is what makes a REWORD visible — see the
    RATCHET section of this file's docstring for exactly what it does and does
    not catch."""
    return (rel, cap, hashlib.sha256(
        " ".join(sentence.split()).encode()).hexdigest()[:12])


# ---------------------------------------------------------------------------
# THE RATCHET ALLOW-LIST. Ratified by the owner 2026-09-12
# (`D-ADVERSARY-SHAPE-RATCHET-2026-09-12`).
#
# WHY IT EXISTS. A `fixture=` is only reachable when the defence is BROKEN — the
# CAPABILITY-CONTROL leg asks that the same attack FAIL without the capability,
# and where the defence holds it fails for every capability, so the control
# cannot discriminate (`R-ADVERSARY-SHAPE-CONTROL-NEEDS-A-BROKEN-DEFENCE`).
# The 19 `UNCOVERED:` records therefore CANNOT be driven to zero by covering
# them, and a gate that can never go green cannot be wired to CI. Ratchet mode
# changes what the gate asserts: not "there is no uncovered claim" but "there is
# no claim this gate has not SEEN BEFORE".
#
# ⚠ READ THE 19 AS A RECORD, NEVER AS A COUNTDOWN. `R-ADVERSARY-SHAPE-CONTROL-
# NEEDS-A-BROKEN-DEFENCE` says so in terms, and at least three of them
# (`ForeignSeedProof`, `ClaimantChosenSurvivorSet`, `UntrustedClaimFields`) have
# a fixture that GRANTS the capability while the defence HOLDS — they are stuck
# for a structural reason and not for any gap in the tree.
#
# THE KEY IS (path, capability, digest-of-the-claim-sentence), and the digest is
# the part that matters. STATED PLAINLY, because a grandfathered list is itself a
# decaying claim:
#
#   IT CATCHES  — any reword of the FIRST matching claim sentence in the block,
#                 any change of capability name, and any move to another file.
#                 Each of those drops the key, so the claim is reported FRESH
#                 (red) and the orphaned entry is reported STALE (red). Both
#                 sides are loud; neither can shrink coverage silently.
#   IT MISSES   — a reword of any OTHER claim sentence in the same comment block.
#                 The gate reports only the FIRST match per block (known limit
#                 ONE in the docstring), and four blocks carry more than one
#                 claim sentence, so the digest covers the first of those and
#                 not its neighbours. It also misses a change to the `UNCOVERED:`
#                 reason text, and a move of the block within the same file:
#                 line numbers are deliberately not in the key.
#
# HOW IT MAY MOVE. Removing an entry costs two edits in this file (the row and
# RATCHET_COUNT) and the gate checks they agree. Adding one costs exactly the
# same two edits — the ratchet does not make growth impossible, it makes growth
# a reviewable diff that says out loud what it is doing. There is NO automatic
# forcing function pushing this list down; the direction "may only shrink" is
# carried by the decision entry and by review, not by the machine.
# ---------------------------------------------------------------------------
RATCHET = (
    ('core/bond/bond.go', 'SybilPlotSharing', '23a5decdc9ea'),
    ('core/bond/bond.go', 'VDFOutputPrediction', '1badfd3b7eed'),
    ('core/bond/bond.go', 'SeedBlockWithoutPlot', 'f689249c9dcd'),
    ('core/bond/bond.go', 'OnDemandPlotRecompute', '4526713b8ced'),
    ('core/bond/bond.go', 'SeedBlockInclusionProof', 'f8b6c80eb0df'),
    ('core/bond/bond.go', 'ForeignPlotLabels', 'e50876438657'),
    ('core/bond/bond.go', 'CrossEpochProofReplay', 'fe5526229678'),
    ('core/node/bondaudit.go', 'PeerAcceptedSelfAssertedBond', 'f95c92fcdd45'),
    ('core/node/por.go', 'TagsWithoutBytes', 'e281e2c299e7'),
    ('core/node/por.go', 'UnderReportedBlockCount', 'd167da6e5b2a'),
    ('core/node/por.go', 'ForeignSeedProof', '10e0a2bd116d'),
    ('core/node/repairclaim.go', 'UntrustedClaimFields', '1e72f903ca2a'),
    ('core/node/repairclaim.go', 'JudgeWithoutCareHandle', '8759c679dacc'),
    ('core/node/repairclaim.go', 'CaretakerDiscoveryWithoutCareKey', '6798ee022955'),
    ('core/por/por.go', 'CrossChunkTagSubstitution', '14b5ce355e3d'),
    ('core/por/por.go', 'TagsAfterByteLoss', 'f861bd27c61f'),
    ('core/repairproof/claim.go', 'ClaimantChosenSurvivorSet', '426fa3a62ed1'),
    ('core/repairproof/gate.go', 'RelayedHolderProof', '624c8811e63a'),
    ('core/repairproof/gate.go', 'DataLessClaimant', '65c855794004'),
)
# Redundant on purpose: see "HOW IT MAY MOVE" above.
RATCHET_COUNT = 19


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


def main(root=None, quiet=False, strict=False, allow=None):
    root = Path(root) if root else ROOT
    allow = RATCHET if allow is None else tuple(allow)
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
        for name, (holds, ctrl) in marker_scopes(body).items():
            tests[name] = (rel, holds, ctrl)

    claims, problems, uncovered = [], [], []
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
                uncovered.append((
                    claim_key(rel, cap, hit[0]),
                    f"{rel}:{lineno}: DECLARED UNCOVERED — capability={cap}. "
                    f"No fixture grants the adversary this capability.\n"
                    f"    reason: {decl.group('unc')[:200]}\n"
                    f"    This is a RECORD, not an exemption "
                    f"({ROADMAP_ROW}, {DECISION})."))
                continue
            fix = decl.group("fix")
            if fix not in tests:
                problems.append(
                    f"{rel}:{lineno}: CITED FIXTURE DOES NOT EXIST — {fix} "
                    f"(capability={cap}). No `func {fix}(` in any tracked *_test.go.")
                continue
            frel, holds, ctrl = tests[fix]
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

    # ── THE RATCHET ────────────────────────────────────────────────────────
    # Three buckets, and only the middle one is grandfathered.
    held = [m for k, m in uncovered if k in allow]
    fresh = [m for k, m in uncovered if k not in allow]
    seen = {k for k, _ in uncovered}
    stale = [e for e in allow if e not in seen]

    ratchet_problems = list(problems)
    for m in fresh:
        ratchet_problems.append(
            m + f"\n    ★ NOT ON THE RATCHET ALLOW-LIST. This claim is NEW to "
                f"{GATE_NAME} — either newly written, or an allow-listed claim whose "
                f"SENTENCE, CAPABILITY NAME or FILE changed, which drops its key.\n"
                f"    The allow-list grandfathers a fixed set of {RATCHET_COUNT} "
                f"pre-existing uncovered claims and NOTHING else. To land this you must "
                f"edit RATCHET and RATCHET_COUNT in {Path(__file__).name} together, in a "
                f"diff a reviewer reads. That edit GROWS the backlog and the ratified "
                f"direction is that it may only shrink.")
    for e in stale:
        ratchet_problems.append(
            f"STALE RATCHET ENTRY — {e[0]} capability={e[1]} digest={e[2]} matches "
            f"nothing in the tree.\n"
            f"    This is the DECAY the ratchet exists to make loud. Either the claim was "
            f"covered or deleted (good: remove this entry and decrement RATCHET_COUNT), or "
            f"it was REWORDED and is now reported above as a fresh claim (then move the "
            f"entry, do not add one). An entry that quietly matched nothing would shrink "
            f"this gate's coverage with no diff; that is why it fails instead.")
    if allow is RATCHET and len(RATCHET) != RATCHET_COUNT:
        ratchet_problems.append(
            f"RATCHET_COUNT DISAGREES WITH RATCHET — declared {RATCHET_COUNT}, "
            f"list holds {len(RATCHET)}.\n"
            f"    The two are deliberately redundant: changing the backlog size costs TWO "
            f"edits in one file, so neither growth nor shrinkage can happen by accident.")

    failing = (problems + [m for _, m in uncovered]) if strict else ratchet_problems

    if quiet:
        return 1 if failing else 0
    print(f"{GATE_NAME}: scanned {len(files)} tracked files, "
          f"{sum(1 for r in files if in_scope(r) and r.endswith('.go') and not r.endswith('_test.go'))} "
          f"in scope; {len(claims)} defence claims found; {len(tests)} test functions resolvable.")
    print(f"{GATE_NAME}: mode={'STRICT' if strict else 'RATCHET'}; "
          f"{len(uncovered)} declared-uncovered ({len(held)} grandfathered, "
          f"{len(fresh)} NOT on the allow-list), {len(stale)} stale allow-list "
          f"entr{'y' if len(stale) == 1 else 'ies'}, {len(problems)} hard problem(s).")
    if not failing:
        print(f"{GATE_NAME}: OK — no NEW undeclared or uncovered defence claim, and every "
              f"`fixture=` resolves to a test that grants the capability and controls for it.")
        print(f"  This is NOT 'the claim is covered'. {len(held)} claims are grandfathered "
              f"RECORDS of missing coverage; read them with --strict.")
        return 0
    print(f"\n{GATE_NAME}: {len(failing)} problem(s).\n")
    for p in failing:
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
# The claim sentence every manufactured case uses, spelled once so the ratchet
# cases key on the SAME text the gate will extract from them.
SELFTEST_CLAIM = "a prover that dropped the bytes cannot make the answer verify."

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

    # ── THE MARKER-BINDING CASES ────────────────────────────────────────────
    # The gate used to read `holds`/`ctrl` from the WHOLE FILE, so a `fixture=`
    # resolved to the right FILE and never to the right FUNCTION. MEASURED
    # 2026-09-12 on the real tree: re-pointing a `capability=LayoutKey`
    # declaration at TestRT_POR_2 — which does not grant LayoutKey — left the
    # gate at 19 problems, NOT CAUGHT. Case B is that miss, reduced.
    ("binding A: a section-header marker binds FORWARD across a helper", 0, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestKeyHoldingProver\npackage por\n",
        "core/por/x_test.go": "package por\n\n"
                              "// ADVERSARY-HOLDS: LayoutKey\n"
                              "// CAPABILITY-CONTROL: LayoutKey -- same attack without it must fail\n"
                              "// ----------------------------------------------------------------\n\n"
                              "// pin returns \"\" while the defect stands.\n"
                              "func pin(x int) string {\n\treturn \"\"\n}\n\n"
                              "func TestKeyHoldingProver(t *testing.T) {\n\t_ = 0\n}\n"}),
    ("binding B: markers belong to ANOTHER test in the same file", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey fixture=TestSomeOtherTest\npackage por\n",
        "core/por/x_test.go": "package por\n\n"
                              "// ADVERSARY-HOLDS: LayoutKey\n"
                              "// CAPABILITY-CONTROL: LayoutKey -- same attack without it must fail\n"
                              "func TestKeyHoldingProver(t *testing.T) {\n\t_ = 0\n}\n\n"
                              "func TestSomeOtherTest(t *testing.T) {\n\t_ = 0\n}\n"}),

    # ── THE RATCHET CASES ───────────────────────────────────────────────────
    # These carry a 4th element: the allow-list this case runs against. Every
    # case above runs against an EMPTY allow-list, which is why they are
    # unaffected by ratchet mode — a case with no grandfathered entry behaves
    # exactly as it did before.
    #
    # There is deliberately NO case for "an undeclared claim is grandfathered".
    # It cannot be constructed: a key needs a capability NAME, an undeclared
    # claim has none, and only a DECLARED UNCOVERED claim is ever offered to the
    # allow-list. The hard classes — undeclared, phantom fixture, missing grant,
    # missing control — are unconditionally RED and no entry can reach them.
    ("ratchet: a grandfathered UNCOVERED claim passes", 0, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey UNCOVERED: no fixture grants it (run 2026-09-12)\npackage por\n"},
     (claim_key("core/por/x.go", "LayoutKey", SELFTEST_CLAIM),)),
    ("ratchet: the SAME claim REWORDED is red twice (fresh + stale)", 1, {
        "core/por/x.go": "// a prover that released the space cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey UNCOVERED: no fixture grants it (run 2026-09-12)\npackage por\n"},
     (claim_key("core/por/x.go", "LayoutKey", SELFTEST_CLAIM),)),
    ("ratchet: a NEW undeclared claim beside a grandfathered one", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey UNCOVERED: no fixture grants it (run 2026-09-12)\npackage por\n",
        "core/por/y.go": "// an attacker cannot substitute a tag from another chunk.\npackage por\n"},
     (claim_key("core/por/x.go", "LayoutKey", SELFTEST_CLAIM),)),
    ("ratchet: an allow-list entry matching NOTHING is STALE", 1, {
        "core/por/x.go": "// Blocks reports how many blocks a unit of n bytes splits into.\npackage por\n"},
     (claim_key("core/por/x.go", "LayoutKey", SELFTEST_CLAIM),)),
    ("ratchet: --strict still reports the grandfathered claim", 1, {
        "core/por/x.go": "// a prover that dropped the bytes cannot make the answer verify.\n"
                         "// ADVERSARY-SHAPE: capability=LayoutKey UNCOVERED: no fixture grants it (run 2026-09-12)\npackage por\n"},
     (claim_key("core/por/x.go", "LayoutKey", SELFTEST_CLAIM),), {"strict": True}),
]


def self_test():
    import tempfile
    bad = 0
    for case in CASES:
        name, want, files = case[0], case[1], case[2]
        allow = case[3] if len(case) > 3 else ()
        kwargs = case[4] if len(case) > 4 else {}
        with tempfile.TemporaryDirectory() as td:
            td = Path(td)
            subprocess.run(["git", "-C", str(td), "init", "-q"], check=True)
            for rel, body in files.items():
                (td / rel).parent.mkdir(parents=True, exist_ok=True)
                (td / rel).write_text(body)
            subprocess.run(["git", "-C", str(td), "add", "-A"], check=True,
                           capture_output=True)
            got = main(root=td, quiet=True, allow=allow, **kwargs)
            ok = got == want
            bad += 0 if ok else 1
            print(f"  [{'ok' if ok else 'FAIL'}] want exit {want}, got {got} -- {name}")
    print(f"{GATE_NAME} self-test: {len(CASES) - bad}/{len(CASES)} cases correct")
    return 1 if bad else 0


if __name__ == "__main__":
    if "--self-test" in sys.argv:
        sys.exit(self_test())
    sys.exit(main(strict="--strict" in sys.argv))
