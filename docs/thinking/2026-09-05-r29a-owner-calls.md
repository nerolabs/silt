# 2026-09-05 — the seven R2.9a run-precondition calls, walked through and ratified

- **Date:** 2026-09-05 · **Seat:** BUILDER (in-session, no code) · **Branch:** `docs/r29a-owner-calls-ratified`
- **Base:** `origin/main` = `028594c` (PR #740 merged)

**Context / trigger:** the owner asked to be taken through every decision open to them on the
R2.9a Rock, and for each: had it been through a seat loop, which seats, what did they
recommend, and how was it reasoned. ROADMAP item 12 listed seven. The owner then said the
first pass gave no context on three of them (the bin count, the sibling aggregates, the
library link key) and asked for the problem each sits in before deciding. This record keeps
the walk-through and the calls together.

**Evidence (per build-immutable #7 — artifacts, not vibes):**
- Seat record per call, read at the source: the Economist advisory §1.5/§3.5/§4.1/§7; the
  necessity certification §2.1/§3.3–3.4/§6; the residuals certification §1.3/§3.3; the DELTA
  certification §1.4; the Red-team F2/F4/F5/F6/F9; the Crypto-specialist §3.1/§5/§7; the PE
  four-residuals ruling §6 (full paths in ROADMAP item 12 and `D-R2.9a-RUN-CALLS`).
- `core/node/column.go:11-20` — a column's shards go to the nodes closest to `colKey(root, j)`;
  the "one host holds one shard of each stripe" anti-affinity is a property of a large enough
  network, not an enforced constraint. This is what sets `F_min = 1` on a small fleet.
- `core/erasure/erasure.go:34` `K = 10`; `core/pipeline/pipeline.go:28-31` 64 MiB production
  chunk floor; `core/manifest/manifest.go:38` 128 MiB maximum. Stripe floor 640 MiB.
- `core/credit/bbootstrap.go:92-115` — the 8 × 164 grid, 4 bins per octave, and the code's own
  disclosure that the bin count is the only privacy lever (G-BB-23).
- `cmd/silt/ui.go:304-330` — the guard checks the `Host` header and the `Origin` allow-list,
  never the remote address; GETs need no token (the #89 read-only exemption).
- `cmd/silt/ui.go:1328-1336` — `apiLibrary` emits `Link`, the full handle;
  `core/link/link.go:38-41` — the handle is retrieve-and-decrypt; `adapters/linkbook/linkbook.go:99`
  — on disk it is mode 0600.
- `cmd/silt/ui/observatory.html:36,46-47,83` — the served column and the bandwidth card are
  computed from `stats.BytesServed` deltas, cross-origin, no token.
- `docs/decisions.md` `D-TAKEDOWN` — a decision, low urgency, not a built feature.

**Which calls had a seat loop, and what it said (as reported to the owner):**

| Call | Seats | Recommendation on record |
|---|---|---|
| re-pin `grant/r` | Researcher ×2, Economist | structural floor now; Economist: err high, conditional on R2.12 first |
| population `P` | Researcher ×2, Economist | all honest fetchers (feasibility: `C = 0`; D-S7) |
| `q` | Researcher only | no value proposed; every derivation instantiated at 0.9 |
| G-BB-13′ Part A | Economist, Red-team, Crypto, PE | refuse at startup (Econ, PE); Red-team: a bind check is not access control |
| bin count | Red-team, Researcher, Crypto | Researcher: the only lever; declined to pin (Don't #3 on one side) |
| `R-BB-SIBLING-AGGREGATES` | Red-team, PE, Researcher | none; the cost was priced (observatory served column + bandwidth card) |
| `/api/library` link | none (Builder flagged, PE verified the fact) | none |

One correction made in the walk-through: the builder's memory said the seats had reached a
consensus on a remote-address policy for G-BB-12′. The source shows a divided record
(Economist/PE bind refusal; Red-team proxy finding; Crypto-specialist Tor prior art). Reported
as unconverged; the G-BB-12′ PACE must resolve it.

**Options weighed (only where the owner had more than one live option):**
- `grant/r`: (A) 28 GiB exact at `F_min = 1`; (B) 2.8 GiB at `F_min = 10`; (C) 32 GiB, (A) with
  margin. (B) assumes a column spread the code does not enforce on a small fleet. (C) chosen.
- bin count: 4 (19%, 164 bins) / 2 (41%, 82) / 1 (2×, 41). Under the run re-scope the
  precision side has no consumer; 1 chosen.
- sibling aggregates and link key: (A) leave open, rate-bounded; (B) token-gate outright,
  observatory loses its served column; (C) an operator flag, exposed in beta, withheld at
  release, `-privacy=off` explicit. The owner chose (C) for both, because the exposed case is
  the one-root edge node and flixz's catalogue nodes attribute nothing per title.

**Decision + rationale:** all seven recorded in `docs/decisions.md` as `D-R2.9a-RUN-CALLS`
(1–5) and `D-UI-PRIVACY-FLAG` (6–7). `q` is left unpinned because the re-scope removed its
consumer, not because a value was rejected.

**What would change my mind:** the G-BB-19 sentence from the Researcher failing to certify
32 GiB (the value is a security parameter and does not ship without it); a measured flixz
fleet size that makes `F_min > 1` provable, which would lower the floor; a per-title
attribution report from a one-root beta node, which reopens the beta default early.

**Status:** decisions recorded; four builds owed (bin count; startup refusal + G-BB-12′ PACE;
`-privacy` flag; re-aimed handoff) and one research deliverable (G-BB-19 sentence).
