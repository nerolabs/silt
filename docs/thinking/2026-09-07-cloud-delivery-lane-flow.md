# 2026-09-07 — A cloud flow for the R2.9 delivery lane, honest about the dark era

**Trigger:** the `c450985-deep` billable run went out with the first R2.9/B-9 build and the
cloud sheet had NO delivery-session flow (`topology.py` armed no `-accept-delivery-receipts`).
Owner: "create the necessary cloud tests before the next billable run."

**The constraint that shapes the design.** The positive settlement — a receipt BANKED on the
wire — needs the fetcher to withdraw a token against the server's chain-committed key_E, and
that binding needs an era-4/v5 chain. Era-4 is dark on every real network until the R3.4 stamp
raise, and the owner ratified NO activation override (ROADMAP `R-E2E-ERA4-FIXTURE`). So a cloud
flow cannot bank a receipt today, and a flow that pretended to would be exactly the fake green
`integration/FIELD-TEST-STATUS.md` forbids.

**Options.** (A) Wait for the stamp raise, add the flow then — leaves the lane's field contract
(daemon start preconditions, the announced markers, the client's two distinct refusals)
unexercised on the fleet for weeks. (B) One era-aware flow, two rows: the contract row grades what
is live today and the settlement row is a stated GAP that flips to a real pass at the stamp raise
with no harness change. (C) Two flows, one added later — duplicates the publish/fetch scaffolding
and invites the later one to be forgotten.

**Decision: (B).** `flow_delivery_lane` arms the lane on the boot validator (the issuer), fetches
with it as the only peer, presents `swarm receipt` against it and against a lane-off store, and
classifies the client outcome: banked → require the server's `delivery receipt banked` and the
idle `delivery session closed` (90s window; the close line audited for identity); refused naming
the committed binding → require the refusal NOT be conflated with the lane-off sentence, the
server banked nothing, and the lane-off control carried the NOT-banked marker; anything else →
fail. LOCAL proof: the three e2e tests, plus a `LOCAL=1` docker drive of the flow before it is
trusted on a fleet. The topology change is confined to the boot validator; the faucet becomes
rate-limited there (256 grants / 256 per hour), which the economy flows never draw on.

**What this does NOT claim.** Row 13b is a gap until the stamp raise. The billable run that
follows the true-up session grades row 13 for real and shows row 13b as the owed seam.

## Blind PE fold-in (2026-09-07)

**Ruling:** `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-cloudtest-delivery-lane-flow-34114c4-2026-09-07.md`
— **BLOCK**, eleven items, the two decisive ones measured: (1) the boot banner was read through the
harness's 800-line journal window on the fleet's noisiest node (~27 journald lines per block, 79 % TLS
handshake noise), so a mid-sheet position would have missed a line printed once at boot and scored FAIL;
(2) every server-side marker (`delivery receipt banked`, `delivery session closed`) is an `n.logf` line that
lives in `$STORE/debug.log` and never reaches journald — the "server banked nothing" conjunct was vacuous
and the live arm could never have passed. The PE also booted the exact argv (it starts; the seven start
gates clear) and drove the CLI against an armed and an unarmed daemon (both classification greps hold on
the wire) — the part that had to be right was right; the READING of the result was wrong.

**Folded:** the banner is read over the whole unit journal and the arming over the unit's `ExecStart`;
every server-side read is `debug.log` after a per-flow line baseline, and an unreadable/empty `debug.log`
GAPs the row (no darkness pass); `require_live` + a retried client call so substrate noise GAPs rather than
FAILs; row 13b is a SKIP behind the era probe (the PE's recommendation on the one call that was mine — a
permanent GAP would have made the green RC gate unreachable until R3.4), with the skip text naming what is
untested; the faucet bucket is 64 (5× below the daemon's derived cap; 256 sat at 78 %); the "only peer"
overclaim is rewritten (holders serve via the DHT — the receipt's server is the fetcher's own claim, which
is the R2.9 shape); 13b is recorded on every exit; the fetch's error text reaches the console; the README
row moved out of the "no topology change" table. **New local proof:** `TestPaidDeliveryLaneArmsInTheHarness
Posture` boots one objective validator with the harness's verbatim flags (epoch clock DERIVED, no explicit
`-epoch-blocks`), waits for the banner and the affordability line, drives the real CLI, and requires the
committed-binding refusal with nothing banked in `debug.log` — the posture no other test covered.
