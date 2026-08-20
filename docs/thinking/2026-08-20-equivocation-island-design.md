# 2026-08-20 — The equivocation island: run the destructive drill on EVERY sheet, contained

**Owner directive (Andrew, 2026-08-20, during the confirming ECONOMY run):** "run
it [184-equivocation] in a contained environment on every cloud test run — no
blast radius if totally contained, but skipping it is a blind spot."

**Status of the prior ruling:** the 2026-08-17 PE ruling
(`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/184-equivocation-topology-ruling-PE-2026-08-17.md`)
said (D): the destructive equivocation drill runs on its OWN ephemeral net, not
mid-sheet, because a mid-sheet slash pins the launch quorum at `⌊4/2⌋+1=3` over
only 3 live anchors — a zero-fault-tolerance tail that flakes the rest of the
sheet. The ruling's objection was the **fault-tolerance blast radius**, never the
drill itself; it explicitly wanted the drill *drivable and always fired* ("the
range is shootable"). Andrew's directive resolves the open piece: make (D) a
STANDING part of every run, not an on-demand aside. This is consistent with the
ruling — a fully-contained island has no tail to flake.

## The containment, three ways (why "totally contained" holds)

1. **Consensus containment (the one that matters).** The island is its OWN
   consensus universe: 4 island anchors, its own genesis, `-anchors` and
   `-persistent-peers` naming ONLY each other. No main-swarm validator ever names
   an island node, and vice versa. So an island slash consumes the ISLAND's fault
   tolerance — which no main-sheet flow grades. The PE's zero-FT-tail objection is
   structurally impossible: the tail is on a chain nothing else depends on.
2. **Network containment.** Island nodes sit on their own subnet with a firewall
   rule allowing island↔island only (plus IAP ssh). Even the accidental cross-dial
   can't happen.
3. **Quota/cost containment (the REAL constraint — [[silt-time-is-the-cost]]).**
   Island nodes get NO external IP. The base topology already fills the primary
   region to its 8-IP `IN_USE_ADDRESSES` quota; 4 public island IPs would force a
   manual quota bump (a run-blocking step). Instead the island subnet egresses via
   **Cloud NAT** (managed google_compute_router + router_nat — no instance, ~free,
   no external IPs) so nodes pull the GCS binary and boot, staying dark inbound.
   Zero quota, near-zero wall-clock (same parallel apply).

## Why not just keep it on its own separate `apply`?

Considered: a second `terraform apply` for a throwaway island net, run before/after
the main sheet. Rejected: it doubles provision+teardown wall-clock (the thing we're
cutting), and a separate apply is a separate failure surface to babysit. Folding the
island into the main apply as an isolated subnet gets containment without a second
lifecycle — it comes up and dies with the sheet, one apply, one destroy.

## The drill (`flow_equivocation_island`, replaces the SKIP row)

1. Island comes up as 4 objective anchors on their own genesis; wait for a
   committed block (their chain is live and independent).
2. `relaunch_with island-a "-equivocate <island-b-id>"` — the adversary
   `PlaceConflictingSigned`s two conflicting signed blocks at one height.
3. An honest island anchor's `FindEquivocations` catches it on the honest sync
   path, unaided, and slashes. Assert the real `slashed equivocator` /
   `validator slashed for equivocation` line (#7 — the product's own words).
4. PASS = the slash fired on the wire; FAIL = no slash within the window; GAP =
   the island never reached a baseline commit (premise unmet, classifier-style).
5. No restore needed — the island is torn down with the run.

## LOCAL-first (this session's whole discipline)

On the LOCAL=1 backend the island is 4 more docker containers on the bridge with
their own anchor set — no external IP concept, no GCS, no Cloud NAT. So the DRILL
LOGIC (baseline → equivocate → slash-fires assertion) is verified for $0 first;
the terraform (Cloud NAT + island subnet + no-external-IP instances) is a smaller
addition whose only proof is a clean `terraform plan`, since real GCP NAT is the
one piece LOCAL can't exercise (and doesn't need to — it's infra plumbing, not
drill logic). LOCAL_PROOF annotation:
`LOCAL=1 ./cloudtest.sh (flow runs verbatim; the island is 4 containers)`.

## Optional follow-on (the PE's (C), still open)

The ruling flagged a *stronger* bonus property — liveness-AFTER-eviction on a
MATURING sheet (a 12-member epoch has the weight headroom to survive a slash, so
the chain keeps committing after eviction). Gated on two verifications the ruling
listed (post-latch maturer-gather; residual >⅔-weight quorum). NOT built here —
this doc builds (D)-always-on; (C) stays a named future consult.
