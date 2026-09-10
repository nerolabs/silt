#!/bin/sh
# Runs INSIDE the container. Impairs the loopback the e2e daemons talk over
# (they listen/dial on 127.0.0.1, so `tc netem` on `lo` degrades the real
# consensus traffic), then runs the adversarial e2e drivers under that
# impairment. The attacks are DRIVEN and must be DENIED — exactly the thing the
# flaky live cloud kept failing to even set up.
#
# HONEST-GREEN DISCIPLINE (build-immutable #4). Three distinct things can go
# wrong here and they are NOT the same finding. Each gets its own exit code so
# the caller can never render one as another:
#
#   4  SETUP    — the drills did not BUILD. Zero properties were exercised.
#   3  HARNESS  — an impairment was REQUESTED but tc could not apply it (missing
#                 --cap-add NET_ADMIN). Running anyway would be a false CLEAN cert.
#   5  UNEARNED — the drills built and ran, but fewer than all of them produced a
#                 result (a regex matching nothing, a t.Skip). `go test` exits 0
#                 on "no tests to run"; that is a green with zero execution.
#   0           — every NAMED drill ran and held.
#   other       — a real property verdict from `go test`.
#
# Code 4 is the one that cost the most: for thirteen consecutive nights this
# suite printed a durability finding while the e2e package had failed to compile
# and not one drill had run. See run.sh for the mechanism.
set -e

cd /silt

# ── Phase 1: BUILD ───────────────────────────────────────────────────────────
# Compile the drill binary before touching the network. A failure here is a
# SETUP failure — it says nothing whatsoever about any consensus property and
# must never be rendered as one. Building first also keeps module downloads off
# the impaired loopback and fails fast on the cheap, common error.
echo "== build the e2e drill binary (setup — not yet a verdict) =="
if ! go test -c -o /dev/null ./e2e; then
  echo ""
  echo "setup: the e2e drills FAILED TO BUILD — zero drills ran, so NO property was exercised."
  echo "  This is a SETUP failure, not a durability finding. Fix the build; do not read a"
  echo "  consensus verdict into it."
  exit 4
fi

# ── Phase 2: IMPAIR ──────────────────────────────────────────────────────────
if [ -n "${NETEM:-}" ]; then
  if tc qdisc add dev lo root netem ${NETEM} 2>/dev/null; then
    echo "netem: impaired lo with [${NETEM}]"
  else
    echo "netem: FAILED to apply on lo (need --cap-add NET_ADMIN) — refusing to run a false CLEAN cert"
    exit 3
  fi
else
  echo "netem: CLEAN control (no impairment)"
fi

# ── Phase 3: DRIVE ───────────────────────────────────────────────────────────
echo "== e2e adversarial-consensus drills (run=${TESTS}) =="
# No -short: the drills spawn real daemons over real TCP. -count=1 forbids a
# cached pass. Output is tee'd so the run streams live AND can be counted.
{ go test ./e2e -run "${TESTS}" -count=1 -v -timeout "${TIMEOUT:-900s}"; echo $? >/tmp/netem.code; } 2>&1 | tee /tmp/netem.out
code=$(cat /tmp/netem.code)

# `go test` only ever exits 0/1/2. Remap defensively so a coincidental collision
# can never masquerade as one of this script's harness codes — one number meaning
# two different things is the defect this whole file exists to prevent.
case "$code" in 3 | 4 | 5) code=1 ;; esac

# ── Phase 4: was the verdict EARNED? ─────────────────────────────────────────
# Top-level results have no leading whitespace; subtests are indented.
ran=$(grep -c '^--- \(PASS\|FAIL\):' /tmp/netem.out || true)
skipped=$(grep -c '^--- SKIP:' /tmp/netem.out || true)
# Only countable when TESTS is a plain alternation of literal names (every SUITE
# preset is). A hand-passed regex with metacharacters degrades to "not counted"
# rather than to a wrong count.
want=0
case "$TESTS" in
*[!A-Za-z0-9_\|]*) : ;;
*) want=$(printf '%s' "$TESTS" | tr '|' '\n' | grep -c .) ;;
esac
echo ""
echo "drills: ${ran} of ${want} named drills produced a result (${skipped} skipped)"

if [ "$code" = 0 ] && [ "$want" -gt 0 ] && { [ "$ran" -lt "$want" ] || [ "$skipped" -gt 0 ]; }; then
  echo "REFUSING to certify: go test exited 0 but only ${ran}/${want} drills ran (${skipped} skipped)."
  echo "  A green with zero — or partial — execution is not a property verdict."
  exit 5
fi
exit "$code"
