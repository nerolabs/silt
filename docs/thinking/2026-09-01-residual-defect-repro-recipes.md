# Residual field-defect repro recipes (ported from GitHub issues, 2026-09-01)

**Why this doc.** The GitHub issue tracker is being retired as a task driver; `ROADMAP.md`
is the single source of truth (`ROADMAP.md`, "Residual backlog"). A handful of open issues
carried the only copy of a defect's **reproduction recipe** — the concrete steps to make it
go RED locally. Those recipes are ported here verbatim-in-substance so they survive the
tracker's retirement. This doc is the reproduction home the Residual-backlog lines cite; it
is not a task list and does not re-litigate the fix direction (that lives with each item's
Boulder or residual line).

Every recipe below was read from the live issue via `gh issue view <n>` on 2026-09-01
(HEAD `dfeb1d5`). Issue numbers are kept as provenance anchors only.

---

## Crash-safety — torn `chain.cbor` → silent genesis fallback (#558)

**The mechanism (journal-attributed, run `a434494-deep`).** A validator was kernel-OOM-killed
(SIGKILL) mid-drive at h83. On the systemd restart the daemon logged `chain replay: chain:
bad signature: attester d4f5ec0d…` and **silently started from genesis** (working height 1).
The chain on disk was intact at the pre-crash checkpoint (h83, 87 MiB) — the SIGKILL tore the
in-progress `chain.cbor` write, replay hit the damaged region, and the daemon threw away
87 MiB of *finalized* history instead of recovering the intact prefix or failing loudly. The
sign-mark store survived (markstore is atomic, the #183 C-2 work); the chain store has no
equivalent crash-safety.

**Why it matters.** Silent discard of finalized state on a common failure (OOM / power). The
node then re-enters consensus at genesis while still holding its frozen-epoch seat.
Timing-dependent (2 OOMs in the run, 1 torn write; the first restart replayed fine).

**RED home (local, free).** SIGKILL a node mid-persist under a write-amplifying loop (or
fault-inject a truncated/garbled `chain.cbor` tail) → restart → assert the recovered head is
the longest valid prefix, never genesis, and the failure is loud. No cloud needed.

**Expected behavior (owner's call).** Persist crash-safe (tmp+rename atomic rewrite, or
append-only with valid-prefix recovery); on replay failure recover the longest valid prefix
instead of genesis; NEVER silently fall back (a replay failure that discards finalized state
should be loud, and arguably refuse to start without an explicit operator flag).

Evidence: `integration/cloudtest/{failed-nodes,flow-evidence}-a434494-deep.log`,
`report-a434494-deep.md` (12-deep-heights section).

---

## Liveness — h64 epoch-boundary wedge (#535)

**The wedge (run `45da13c-17686`, DEEP=1 SYBILS=8 MATURING=1 ECONOMY=1).** Chain committed
normally to h63 (including six commits with val-d down). **h64 — the 8th epoch boundary —
never committed across ~1 h** and three flows, surviving val-d's restart and the deep flow's
full-cohort heal. **Safety intact, pure liveness:** at run end all four validators sat on the
IDENTICAL h63 head. Ladder alive but futile: round-changes h64 r1→r5, **every one
`carries_lock=false`**, `pending=0`; `new-view proposal height=64` fires repeatedly; **no
`gather/prepare` narration for h64 at all** (h37/h52 in the same run show that narration when
gathering happens).

**The distinguishing variable.** First field run with #506 R-gate enforcement narration:
`bond-reg submit REFUSED … violates the active reg-inclusion rate bound (#506 R-rule) …
re-registering 1/6 blocks after its last reg (R=10)` at `next_height=64`, from both a sybil
and a maturer. The first boundary wedge coincides with the first visible gate enforcement —
correlation, not yet causation. The approach to the boundary also happened with a seat down.

**Gates Boulder 1 R1.8** (the #535 recovery-boundary decision — cold-auditor directive-trust
boundary — is a named precondition of the accept-flip). Evidence:
`integration/cloudtest/{report,results,console,flow-evidence,rss}-45da13c-17686.*`.

---

## Harness — Docker consensus suite stalls pre-genesis (#530)

**Observed (macOS, Docker Desktop, 2/2).** `./integration/consensus/run.sh`: P0 (negative
control) passes, four validators come up, bonds seal, partition confirmed on C/D — then **P1
never completes**: the `publish()` loop returns no link for 15+ min. During the stall,
`chain-status` on valA AND valB reports **`no chain yet (0 blocks)`** — genesis never forms.
Each daemon's log is ~18–19 lines of boot narration with zero subsequent activity. **Control:**
the identical suite from a clean `main` worktree stalls in the same state, so this is NOT a
branch regression — it reproduces on the commit the RC field run was built from. The same main
commit committed blocks fine on real VMs, so the break is Docker-suite-side (run.sh/compose
drift), host-environmental, or a Docker-only-trigger daemon regression.

**RED home / first step (build-immutable #7 — attribute, don't guess).** Re-run with
`-log debug` on the daemons (compose override) and capture ONE full `silt swarm add` client
transcript (do not discard `out` on failure in the harness). Instrument before fixing.

---

## Harness — publish clients unreachable mid-run (#574, thread 1)

**The plumbing failure (run `027c354-deep`).** Every `ft_publish` on fetch-1 and nat-1 FAILED
after 360 s with **EMPTY RESPONSE from ssh_node — node unreachable at the harness level**.
fetch-1's service was up and bootstrapped but its **RSS never exceeded 9.2 MiB** all run — it
did no real work. Downstream, three graded rows failed (`9-cross-nat` empty publish,
`durability-turnover` "not a silt:v1: link", `11-economy-repair` empty responses) — **one
plumbing failure, not three product regressions.**

**Fix direction (thread 1).** Preflight per-flow client-node reachability (ssh + API probe)
and grade GAP-with-named-cause immediately instead of burning each flow's full 360 s window;
attribute WHY ssh_node got empty responses (nodes.json map? NAT path? sshd?) before the next
run. **Thread 2 (flow-overlap disturbance):** consider a per-flow health barrier (like the
#549 Q4 barrier) before each liveness-graded flow, or sheet ordering that quarantines
seat-killing drills from drive/grade windows. Evidence: `publish-diag-027c354-deep.log`.

---

## Harness — skim-observer arming under per-node ledgers (#586)

**The gap (run `fe2376a-deep`, the gate run's one GAP).** Not a solvency break — the repair
economy closed for the third consecutive run. `11b-economy-skim` GAP: neither armed observer's
reserve grew above prepay across the repair-window reads + 90 s driven fetches (the two prior
runs skimmed +65540 relay / +32770 store-2 under the same flow). Relay journals: repeated
`repair bounty release paid nothing — escrow empty on this judge` — the relay judged verified
repairs but held no escrow for that root; the funded escrow lived on the ledger the `/api/fund`
call actually reached (per-node ledgers: judge≠funder≠paramedic by design).

**The question (topology-variant).** The skim leg's observability depends on WHICH nodes serve
the driven reads and WHICH judge holds the funded escrow — both vary run to run. Fix options:
(a) harness — arm the observers the flow can PROVE will serve (derive from `swarm holders`
output, as kill-selection already does); (b) harness — fund the root on EVERY candidate judge;
(c) product — the "escrow empty on this judge" warn may indicate a real disbursement asymmetry
worth a design look (is cross-judge settlement in scope for S7, or is per-object-funding-per-judge
the accepted model per the D-S7 solvency framing?). (a)/(b) are harness; (c) is research-routed
if pursued. Evidence: `flow-evidence-fe2376a-deep.log`.

---

## Repair — dial-storm to dead-holder provider records (#277)

**The mechanism (reproduced live, `integration/durability`, 16→12 holders).** Under heavy
permanent holder loss on a small swarm (≥~25 % gone for good), the caretaker's repair sweep —
and a fresh client's fetch — drown in a dial-storm to departed holders' stale provider records,
each dial paying a ~2 s i/o timeout. A single sweep can no longer finish and a warm fetch times
out, **even though ≥k shards of every stripe physically survive.** The caretaker log at the
wedge: sweeps completing but slowing (64→66→75 s) then stopping; **439 `dial failed` lines in
~90 s**, re-dialing the 4 permanently-dead holders serially at ~2 s each; `stripe repaired: 4`,
`repair below k: 0`. A warm `swarm get` returned 0 bytes though reachable was 84/104 (≥k per
stripe). **Bytes survive; discovery + repair degrade.**

**Root cause (candidate).** `resolveProviders` → `newWalk(MsgGetProviders,…)` walks the DHT,
dialing nodes closest to the key — which include dead holders still in the routing table. The
`deadUntil` negative cache is consulted on the fetch/repair *decision* path
(`file.go:422/451`, `repair.go:359/378`) but **not on the WALK's dials**, so the walk (and the
re-scatter `placeAt`) re-dial the dead holders every sweep.

**Directions.** Gate the DHT walk's dial targets on `deadUntil` (skip recently-failed peers
during the walk, re-admit on cooldown), as the probe path already does; and/or evict dead
holders from the routing table faster, or prune/expire stale provider records. The
un-cached-walk-dial behavior is scale-independent (amplified by the small swarm); the true
finite-but-renewable durability envelope is judged at cloud scale (`integration/cloudtest`).
Part of the pre-gate repair-sweep family (#501 unbounded sweep / #500 fetch-retained copies
never announce / #502 restart orphans the repair working set).
