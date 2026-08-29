---
name: pr619-named-premise-verification-2026-08-28
description: PR #619 verification 2026-08-28: diff-scope PASS (test/doc-only), both tripwires RED confirmed by injection, empty-BondRegs premise verified by code path, full suite GREEN
metadata:
  type: project
---

# PR #619 ground-truth verification — 2026-08-28

Branch: `name-genesis-sameroot-premise-rg`, base `9e6cf0f`, head `9932be0`.

## Diff scope — PASS (test/doc-only)

`git diff 9e6cf0f...HEAD --stat` changed 6 files, 372 insertions, 0 deletions:

- `CHANGELOG.md` — docs
- `core/chain/chain.go` — ONLY a 13-line comment block in `AppendGenesis` (the NAMED PREMISE anchor); zero logic change
- `core/chain/redteam_verify_genesis-sameroot-premise_test.go` — new test file (Tripwire 1)
- `core/genesis/genesis_sameroot_premise_test.go` — new test file (Tripwire 2)
- `docs/thinking/2026-08-28-genesis-sameroot-residual.md` — deliberation doc
- `website/changelog.html` — generated

`AppendGenesis` apply-path logic: unchanged. `validateBondRegs` logic: unchanged. `genesis.go` build logic: unchanged. No validity rule or consensus rule changed.

## Tripwire 1 — TestGenesisSameRootApplyIsOrderDependent

**As-shipped (GREEN):** PASS — logged "premise pinned: genesis apply() owner is slice-order-dependent (A-first→1a3675ec81f5, B-first→035cc4a43629) — safe only by genesis byte-identity"

**Injection (canonical sort into AppendGenesis before apply):** sorted BondRegs by validator key bytes, making apply() see same canonical order regardless of input → ownerA == ownerB → test fired RED with exact message: "PREMISE CHANGED: genesis apply() is now ORDER-INDEPENDENT for two distinct-ID unproven same-root regs (owner=035cc4a43629 in both orderings)."

Teeth confirmed. Restore verified (test GREEN again after restore).

Note: a rejection-based injection (returning an error from AppendGenesis on same-root collision) also makes the test go RED, but at an earlier assertion (`build()` fatals before the ownerA==ownerB comparison). Both confirm the test has teeth; the canonical-sort path hits the premise-changed message precisely.

## Tripwire 2 — TestProductionGenesisCarriesNoBondRegs

**As-shipped (GREEN):** PASS — logged "premise pinned: production genesis is byte-identical (...) and carries 0 BondRegs"

**Injection (add BondReg to genesis.Build at line 79):** injected `chain.BondReg{Validator: []byte(Key().Public().(ed25519.PublicKey)), Root: entry.Root, Size: 1<<20}` into the Block construction → test fired RED with exact message: "PREMISE CHANGED: the production genesis now carries 1 BondReg(s). The un-guarded genesis apply() order-dependence [...] is now REACHABLE in production..."

Teeth confirmed. Restore verified.

## Empty-BondRegs premise — code path confirmed

`core/genesis/genesis.go:79` (post-restore, no BondReg references in file):
```
b := chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{entry}}
```
`BondRegs` field is absent → zero-valued nil slice → `len(gb1.BondRegs) == 0`. The comment in `AppendGenesis` cites this line correctly.

## Final suite

`go test ./core/... ./sim/...` — all 22 packages PASS. Working tree clean (only `.claude/` untracked, which is harness).

## Verdict

Diff-scope: YES, test/doc-only. Both tripwires have real teeth (RED confirmed by injection). Premise fact confirmed by code path. Full suite clean. No validity or consensus rule changed.
