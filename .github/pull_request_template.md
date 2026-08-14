## What & why
<!-- What does this change do, and why? Link any discussion. -->

## How I tested it
<!-- Commands run, sims exercised, manual checks. CI runs the full suite. -->
- [ ] `go test ./...` passes locally
- [ ] `go vet ./...` and `gofmt -l .` clean
- [ ] **Outcomes covered at all three tiers** for each major use case — **unit · integration/sim · e2e/nat** — asserting the OUTCOME (does the thing achieve what the persona is promised), not the method. This is immutable: a tier may be skipped only with a reason recorded below.
  - Tier(s) skipped + why (or "none"): 
- [ ] **If this fixes a bug — the cheapest-tier question** (efficiency introspection, `docs/thinking/2026-08-15-testing-tiers-introspection.md`): name the *cheapest deterministic tier* that could catch this bug's **class** (unit → consensus model-check → sim → netem → field). Confirm that tier now catches it — or, if the tier lacks the capability, that capability gap **is** the real fix (file it). A bug caught only by an expensive/non-deterministic tier that a cheaper one could own is a process failure even when "caught".
  - Cheapest tier for this class + does it now catch it (or N/A):

## Consensus invariants (I1–I5) — required for any consensus-touching PR
<!-- Binding rule, docs/design/consensus-invariants.md. Delete this section only if the PR
     does not touch a quorum, gate, threshold, fork-choice, signing, or validator-set path. -->
- [ ] States which of **I1–I5** this touches and how each is preserved (and answers the 6-question quorum checklist in the code comment at any quorum site). Invariants touched (or "none — not consensus-touching"):

## Paper trail
- [ ] Added a line to `CHANGELOG.md` `[Unreleased]` (and ran `scripts/gen_changelog.py`) — or N/A
- [ ] Updated `docs/` where relevant — or N/A

## Consensus invariants (I1–I5)
<!-- BINDING for any consensus-touching change (D-CONSENSUS, 2026-08-14):
     state which invariants in docs/design/consensus-invariants.md this PR
     touches and HOW each is preserved. Quorum/gate/threshold sites must
     answer the six-question checklist in their code comment. If the PR
     does not touch consensus, say "not consensus-touching". -->
- Invariants touched + how preserved (or "not consensus-touching"):

## Safety / abuse considerations
<!-- Does this touch storage, serving, the chain, or takedown? Could it let
     infrastructure learn what it carries, or weaken the takedown path?
     Say so explicitly, even if the answer is "no". -->
