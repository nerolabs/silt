#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Deterministic adversarial-consensus certification.
#
# The trust plane's marquee denials — equivocation → slash, partition → heal to
# the heavier fork, forged/under-bonded proposal → reject — must be proven under
# ADVERSE network conditions, not only on a clean localhost. The 2026-08 rescue
# audit found these being "certified" on a flaky live GCP wire that kept failing
# to even DRIVE the attack, then re-grading the miss as a passing GAP. That is
# backwards: an attack you cannot schedule is not a test.
#
# So we certify them where they CAN be scheduled: the e2e drivers (real daemons,
# real TCP, deterministic by construction — fork-choice is summed attester weight)
# run inside a container whose loopback is impaired with `tc netem`. The daemons
# talk over 127.0.0.1, so netem on `lo` degrades the actual consensus traffic with
# the latency/jitter/loss a real WAN has — every run, on a laptop, in minutes.
#
# GRADING (the rescue guardrail): a drill that does not DENY its attack under the
# impairment is a RED (this script exits non-zero), NEVER a passing GAP. The cloud
# is then reserved for what only a real WAN proves — liveness + timing at scale —
# not for discovering causes or forcing attacks.
#
# Usage:
#   ./run.sh                                   # default impairment (cross-region-ish)
#   NETEM="delay 120ms 40ms distribution normal loss 2%" ./run.sh
#   NETEM="" ./run.sh                          # clean-network control (must PASS)
#   TESTS='TestEquivocatorSlashedOverTCP' ./run.sh   # one drill
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

# Default: ~cross-region internet latency, delay-only so the OUTCOME is
# deterministic every run (TCP is reliable; delay changes timing, not the verdict).
# Loss is an opt-in stressor — realistic, but a heavy value can push a drill past
# its internal timeout, which would (correctly) grade RED.
# Note: `=` not `:=` — an explicit NETEM="" must STAY empty (the clean-network
# control), while an UNSET NETEM gets the default impairment.
: "${NETEM=delay 80ms 20ms distribution normal}"
: "${TESTS:=TestEquivocatorSlashedOverTCP|TestPartitionHealsToHeavierForkOverTCP|TestForgedBlockRejectedOverTCP|TestLowBondProposerRejectedOverTCP}"
: "${TIMEOUT:=900s}"
MODCACHE="$(go env GOMODCACHE 2>/dev/null || true)"

echo "== build image (golang + iproute2) =="
docker build -q -t silt-adversarial . >/dev/null || { echo "FAIL: image build"; exit 1; }

echo "== certify adversarial-consensus drills under netem [${NETEM:-CLEAN}] =="
echo "   drills: ${TESTS}"
mc_mount=()
[ -n "$MODCACHE" ] && [ -d "$MODCACHE" ] && mc_mount=(-v "$MODCACHE":/go/pkg/mod:ro)

set +e
docker run --rm --cap-add NET_ADMIN \
  -v "$ROOT":/silt:ro "${mc_mount[@]}" \
  -e NETEM="$NETEM" -e TESTS="$TESTS" -e TIMEOUT="$TIMEOUT" \
  -e GOFLAGS=-buildvcs=false \
  -e GOCACHE=/tmp/go-build \
  silt-adversarial sh /silt/integration/adversarial/netem-run.sh
code=$?
set -e

echo ""
if [ "$code" = 0 ]; then
  echo "RESULT: PASS ✅  every adversarial-consensus drill DENIED its attack under [${NETEM:-CLEAN}] — certified deterministically, off-cloud."
else
  echo "RESULT: FAIL ❌  a drill did NOT deny its attack under [${NETEM:-CLEAN}] (go test exit $code)."
  echo "  Per the rescue guardrail, an attack you cannot drive+deny is a RED, never a passing GAP —"
  echo "  this is a REAL finding. Reproduce and fix it here; do NOT route around it on the cloud."
fi
exit $code
