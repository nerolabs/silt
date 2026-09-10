# scripts/

Dependency-free (stdlib-only) Python. Everything here runs on a bare `python3`
with no install step, so the same command works locally and in CI.

## Generators — website pages are GENERATED, never hand-edited

| Script | Generates | Source of truth |
| --- | --- | --- |
| `gen_changelog.py` | `website/changelog.html` | `CHANGELOG.md` |
| `gen_roadmap.py` | `website/roadmap.html` | `ROADMAP.md` |
| `gen_buildlog.py` | `website/buildlog.html` | `docs/buildlog/` |
| `release_notes.py` | release notes text | `CHANGELOG.md` |

Edit the markdown, re-run the generator, commit both. CI fails the build if a
generated page is stale.

## Checks — wired into `.github/workflows/ci.yml`

All of these run in the `website` job except `check_reachability.py`, which needs a Go
toolchain and runs in its own `reachability` job.

| Script | Fails the build when | Scar |
| --- | --- | --- |
| `check_links.py` | a relative link or asset in `website/*.html` does not resolve | — |
| `check_claims.py` | a claim in `docs/design/claims-ledger.md` points at a test that no longer exists | — |
| `check_tenet_qualifiers.py` | the TENETS.md Sybil composition drops its design-target qualifier | `scar:sybil-design-target-overclaim-2026-09-01` |
| `check_status_headers.py` | a doc's not-built Status header contradicts a built/shipped body | `scar:status-header-vs-body-contradiction-2026-09-01` |
| `check_cited_tests.py` | a Go comment or doc cites a `TestXxx` that has no `func TestXxx(` anywhere; **or** a doc's `path.go:NNN` no longer points at the symbol cited beside it | `scar:cited-test-does-not-exist-2026-09-02`, `scar:cited-source-coordinate-decayed-2026-09-10` |
| `check_residual_register.py` | a residual name (`R-…`) appears in `ROADMAP.md` without a Residual-register row carrying a bucket, a closer and a source | `scar:residual-backlog-unbucketed-2026-09-06` |
| `check_source_gates.py` | a test that reads the project's own `.go` source does not say so in its failure text, or does not name its runtime cover | `scar:source-gate-promises-a-runtime-property-2026-09-03` |
| `check_reachability.py` | a lane the release checklist makes a public claim about has no client entry point in the linked `./cmd/silt` binary, and the checklist does not say the lane **cannot be exercised** | `scar:mechanism-shipped-inert-2026-09-10` |

Each check exits `0` on pass and `1` on failure, and prints its findings to
stderr. Run them all with:

```sh
for c in links claims tenet_qualifiers status_headers cited_tests source_gates residual_register reachability; do
  python3 "scripts/check_$c.py" || echo "FAILED: $c"
done
```

### `check_cited_tests.py` — the cited-test and cited-coordinate lint

Catches *a green check that does not verify the property it claims*: a comment or
doc naming a test that does not exist. It reads as "this is verified"; nothing
verifies it.

Since 2026-09-10 it catches the same defect in its second carrier: a `path.go:NNN`
coordinate that no longer points at the symbol cited beside it. Three in
`docs/decisions.md` — two written *"verified"* — pointed at unrelated lines, and 49
production Go comments were decayed by as much as 785 lines.

**Line numbers rot silently; a symbol name does not move.** So the resolution key is
the SYMBOL, and a coordinate is checkable only when it is tied to one: a coordinate
cited beside identifiers the file declares must land on ONE OF THEM, either inside a
declaration or on a line where the name occurs as a whole word. It is checked against
every identifier the sentence names, not the nearest one, because binding only the
nearest reddens correct prose of the shape *`Subject` verb `Object` (`path.go:N`)* —
and a doc lint that cries wolf is the one that gets disabled.

**It does not see a rename.** Renaming a cited symbol, moving its file or deleting it
outright each leave this check at exit 0: an anchor that resolves to no declaration is
treated as prose, so the coordinate goes silently unchecked. What it catches is a
coordinate that has drifted off a symbol that still exists.

Scope: markdown plus PRODUCTION Go comments. An unanchored coordinate is not checked —
naming the symbol is what buys coverage. `CHANGELOG.md`, the external review trees and
`docs/thinking/` (983 coordinates, the largest exclusion) are exempt because all three
are dated point-in-time records; `*_test.go` comments are out until a rot is measured
there. Coverage on the walked surface: 296 coordinates, 94 anchored, 64 allowlisted as
enumerated debt, **30 enforced**.

It widens `check_claims.py`, which enforces the same linkage for
`docs/design/claims-ledger.md` only. That narrow scope is why the instance that
fired the third-time rule got through: a production comment cited
`TestPaidSerialWindowMatchesDemandWindow`, and a research certification then
repeated the claim. No such test has ever existed.

- **In-repo citations are STRICT** — a phantom fails the build.
- **External review trees are ADVISORY** — they live outside the repo, are not
  version-locked to it, and may cite a real test on an unmerged branch. Pass
  `--strict-external` to fail on them too. They are absent in CI, so CI stays
  hermetic.

```sh
python3 scripts/check_cited_tests.py                     # repo only (what CI runs)
python3 scripts/check_cited_tests.py --strict-external    # + fail on the review trees
python3 scripts/check_cited_tests.py --external-root DIR  # point at another tree
SILT_CITED_TESTS_EXTERNAL_ROOTS=a:b python3 scripts/check_cited_tests.py
```

Known-unbacked citations live in `cited_tests_allowlist.txt`. That file is a
ledger, not an exemption list: every entry says whether it is frozen HISTORY or an
OWED test, and an OWED entry is a debt whose repayment is deleting the line.

### `check_reachability.py` — is the mechanism in the shipped binary?

Catches *a mechanism recorded as delivered that no operator can run*: present in
source, described as enforcing, and with zero non-test callers, so the linker drops
it out of `./cmd/silt`. Six shipped that way in one month, including a refuse-to-start
on consensus-config divergence and the entire paid-relay client.

**No Go test can see this.** A test that can call the symbol is itself the caller that
keeps it alive. A grep sweep cannot see it either: it keys on bare identifiers, so an
inert method hides behind a live namesake — the 117-site sweep that preceded this lint
missed `demand.Commit` outright, and every common verb (`Verify`, `Close`, `Root`,
`Get`) has the same hole. `go tool nm` on the linked binary has neither hole: the name
is fully qualified, and dead-code elimination already decided the question.

The gate asserts one equality per entry, with a closed complement:

> the checklist label says THIS SYMBOL is unreachable  ==  the symbol is absent

The label's claim is a PAIR — the phrase **"cannot be exercised"** and the symbol's own
name, in the lane's posture line in `docs/release-checklist.md`. Both halves are needed.
The phrase alone would let one line excuse a whole lane; the name scopes the claim,
because a lane routinely has a live half and a dropped half (the paid delivery lane
opens and settles but cannot top up). "Cannot be exercised" and "has not been exercised"
are different claims to a reader, and only one of them is true of a lane whose client
entry point is not linked.

**Never point an entry at a one-line wrapper.** A thin function is inlined into its
caller and vanishes from the symbol table in a build where the lane is perfectly live —
`adapters/relay.DialThroughPaid` is exactly that. Entries must name a SUBSTANTIAL
symbol, and the lint measures the declaration to enforce it: at least 8 body lines AND a
function literal, which surfaces as `<symbol>.funcN` and is required as a second witness
when the symbol is present. The lint REFUSES a thin entry rather than passing it, and
lowering that threshold is how this gate stops meaning anything.

Lanes live in `reachability_lanes.txt`, one record per client entry symbol, each
carrying a written `claim` and a written `substantial`. An unreasoned row fails: an
allowlist rationale is itself a claim, and it decays exactly like a cited test name. So
each written reason has a mechanical companion — `label` must resolve in the checklist,
and the measured body must agree with `substantial`.

Scope is deliberately bounded to lanes the release checklist makes a public claim
about. It is not a sweep of exported symbols; the floor-box keystone is inert by
ratified owner direction (`D-RECOMPUTE-FREEZE`) and would drown the signal.

```sh
python3 scripts/check_reachability.py                    # builds ./cmd/silt, then checks
python3 scripts/check_reachability.py --binary PATH      # reuse a build
python3 scripts/check_reachability.py --checklist PATH   # read the labels elsewhere
python3 scripts/check_reachability.py --lanes PATH       # a different lane table
```

On the shared dev box the build runs under `taskpolicy -c background nice -n 19`
automatically; CI has no `taskpolicy` and builds at full speed.
