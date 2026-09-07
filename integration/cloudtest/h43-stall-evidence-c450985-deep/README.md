# Height-43 liveness stall — run `c450985-deep`, 2026-09-07 (evidence, host-side, read-only)

`6-fault-tolerance` stopped val-d at 08:45:04 UTC. Block 42 had committed at 08:44:21 (10 attestations).
Block 43 committed at **09:01:41** — 17 min 20 s later, ~16.5 min after val-d stopped and ~7 min after
it returned (08:54:22). The harness scored the flow GAP ("ladder advancing but uncommitted — OUT OF
MODEL"); prior deep runs (`fe2376a`, `8a52aba`, `585c82a`) passed it inside 650 s.

The files are `grep -E 'height=4[1-4]|committed block 4[1-4]|new-view|escape|designee'` over each
validator's `debug.log` plus its journal's commit/start/stop lines, taken at ~09:00 UTC while the
stall was live, and two rotation seats (`sybil-3`, `maturer-2`) at height 42–43.

Shape: val-a advanced h43 r1/r2/r3/r4 at 08:45:10 / 08:46:40 / 08:49:10 / 08:53:10; val-b's ladder ran
~77 s behind (r1 08:46:27, r2 08:47:57, r3 08:50:27); val-d, back at 08:54, started its own ladder at r1
08:55:26. At 08:59:26 every rotation seat recorded round-change r3 at once (they DO participate). No
`new-view proposal` gathered a commit until 09:01:41. Every seat alive, idle, no OOM (`infra-node-liveness`
PASS). Attribution is OWED to the model-check tier (`core/node/modelcheck_i2_rounds_test.go` family):
a field run confirms, never discovers — this one discovered. Register row `R-H43-ROUND-LADDER-DESYNC`.
