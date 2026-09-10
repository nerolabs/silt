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

## Checks — all wired into the `website` job in `.github/workflows/ci.yml`

| Script | Fails the build when | Scar |
| --- | --- | --- |
| `check_links.py` | a relative link or asset in `website/*.html` does not resolve | — |
| `check_claims.py` | a claim in `docs/design/claims-ledger.md` points at a test that no longer exists | — |
| `check_tenet_qualifiers.py` | the TENETS.md Sybil composition drops its design-target qualifier | `scar:sybil-design-target-overclaim-2026-09-01` |
| `check_status_headers.py` | a doc's not-built Status header contradicts a built/shipped body | `scar:status-header-vs-body-contradiction-2026-09-01` |
| `check_cited_tests.py` | a Go comment or doc cites a `TestXxx` that has no `func TestXxx(` anywhere; **or** a doc's `path.go:NNN` no longer points at the symbol cited beside it | `scar:cited-test-does-not-exist-2026-09-02`, `scar:cited-source-coordinate-decayed-2026-09-10` |
| `check_residual_register.py` | a residual name (`R-…`) appears in `ROADMAP.md` without a Residual-register row carrying a bucket, a closer and a source | `scar:residual-backlog-unbucketed-2026-09-06` |

Each check exits `0` on pass and `1` on failure, and prints its findings to
stderr. Run them all with:

```sh
for c in links claims tenet_qualifiers status_headers cited_tests source_gates residual_register; do
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
