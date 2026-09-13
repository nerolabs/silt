## What & why
<!-- What does this change do, and why? -->

## How I tested it
<!-- Commands run, sims exercised, manual checks. CI runs the full suite. -->
- [ ] `go test ./...` passes locally
- [ ] `go vet ./...` and `gofmt -l .` clean
- [ ] **Outcomes covered at all three tiers** for each major use case — **unit ·
      integration/sim · e2e/nat** — asserting the OUTCOME (does the thing achieve what
      the persona is promised), not the method. A tier may be skipped only with a
      reason recorded here.
  - Tier(s) skipped + why (or "none"):
- [ ] **If this fixes a bug — the cheapest-tier question.** Name the *cheapest
      deterministic tier* that could catch this bug's **class** (unit → consensus
      model-check → integration → e2e under impairment → field), and confirm that tier
      now catches it. If the tier lacks the capability, that capability gap **is** the
      real fix. A bug caught only by an expensive or non-deterministic tier that a
      cheaper one could own is a process failure even when "caught".
  - Cheapest tier for this class, and does it now catch it (or N/A):

## Consensus invariants (I1–I5) — required for any consensus-touching PR
<!-- Delete this section only if the PR touches no quorum, gate, threshold,
     fork-choice, signing, or validator-set path. -->
- [ ] States which of **I1–I5** this touches and how each is preserved, and answers the
      quorum checklist in the code comment at any quorum site. Invariants touched (or
      "none — not consensus-touching"):

## Canon
- [ ] `docs/TENETS.md` and `docs/VISION.md` still describe what the system does — or N/A

## Safety / abuse considerations
<!-- Does this touch storage, serving, the chain, or takedown? Could it let
     infrastructure learn what it carries, or weaken the takedown path?
     Say so explicitly, even if the answer is "no". -->
