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
#   NETEM="" ./run.sh                          # clean-network control (must PASS)
#   TESTS='TestEquivocatorSlashedOverTCP' ./run.sh   # one drill
#
# The scheduled arms (.github/workflows/nightly-netem.yml) — build-immutable #5
# names jitter, latency, packet loss AND reordering, so each gets an arm:
#   NETEM="delay 80ms 20ms distribution normal"           ./run.sh   # jitter
#   NETEM="delay 120ms 40ms distribution normal loss 2%"  ./run.sh   # loss
#   NETEM="delay 20ms reorder 25% 50%"                    ./run.sh   # reorder
# `reorder` REQUIRES a delay — tc rejects it outright otherwise ("reordering not
# possible without specifying some delay", exit 1), which this harness renders as
# a HARNESS ERROR, never as a clean pass. A malformed reorder arm cannot go green.
#
# EXIT CODES (the full contract is in netem-run.sh's header): 0 pass · 3 harness
# (impairment unappliable) · 4 setup (the drills never built) · 5 unearned (not
# every named drill produced a result) · anything else, a real property verdict.
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
: "${TIMEOUT:=900s}"

# SUITE presets (override with an explicit TESTS='<regex>' for one property):
#   adversarial (default) — the M0 consensus DENIAL drills (equivocation-slash,
#                           partition-heal, forged/low-bond reject).
#   substrate             — the P0 LIVENESS/durability substrate over the wire:
#                           objective quorum commit, bond-earned-standing commit,
#                           and publish→fetch bit-perfect — the "does the network
#                           stay live and serve" half the adversarial drills ride on.
#   all                   — both, the full P0 netem gate in one run.
# NOTE: the cold-start re-mesh test (TestBootstrapRetryRecoversColdStartRace) is
# deliberately EXCLUDED from the netem suite — it runs with -request-timeout 500ms
# -request-retries 0 (a clean-localhost timing test of the self-heal LOGIC, "not
# about RPC retry" per its own comment), so a dropped packet under netem flakes it.
# The bootstrap-retry FIX is certified in the clean e2e suite (bootstrap_test.go);
# netem-hardening that specific race is tracked, not faked green here.
ADVERSARIAL='TestEquivocatorSlashedOverTCP|TestPartitionHealsToHeavierForkOverTCP|TestForgedBlockRejectedOverTCP|TestLowBondProposerRejectedOverTCP'
SUBSTRATE='TestObjectiveConsensusCommitsOverTCP|TestBondEarnedStandingCommitsOverTCP|TestPublishCommitFetchOverTCP'
# `noun` is the verdict line's subject, set explicitly per SUITE. It used to be
# derived by chopping `kind` at its first space, which mangled SUITE=all into
# "every FULL property held" — a verdict line that cannot name what it certified
# is one edit away from a verdict line that names the wrong thing.
case "${SUITE:-adversarial}" in
  adversarial) : "${TESTS:=$ADVERSARIAL}"; kind="adversarial-consensus drills"; noun="adversarial-consensus"; verb="DENIED its attack" ;;
  substrate)   : "${TESTS:=$SUBSTRATE}";   kind="P0 substrate liveness";        noun="P0-substrate-liveness"; verb="held" ;;
  all)         : "${TESTS:=$ADVERSARIAL|$SUBSTRATE}"; kind="full P0 netem gate (substrate + adversarial)"; noun="P0 netem gate"; verb="held/denied" ;;
  *) echo "unknown SUITE='$SUITE' (use adversarial | substrate | all, or set TESTS=<regex>)"; exit 1 ;;
esac
MODCACHE="$(go env GOMODCACHE 2>/dev/null || true)"

echo "== build image (golang + iproute2) =="
docker build -q -t silt-adversarial . >/dev/null || { echo "FAIL: image build"; exit 1; }

echo "== certify ${kind} under netem [${NETEM:-CLEAN}] =="
echo "   tests: ${TESTS}"
# The host module cache is mounted WRITABLE, and that is load-bearing. Mounting it
# `:ro` shadowed the container's own writable /go/pkg/mod, so any module the host
# cache happened to be MISSING became unresolvable: `go: writing go.mod cache:
# mkdir /go/pkg/mod/cache/...: read-only file system`, 72 of them, then
# `FAIL github.com/nerolabs/silt/e2e [setup failed]` with ZERO tests run.
# The perverse part is why nobody caught it: with NO host cache the mount is
# skipped entirely and the container downloads freely (green), and with a COMPLETE
# host cache nothing needs writing (green). It bites only on a PARTIAL cache — the
# steady state of a CI cache keyed on a go.sum that has stopped moving. That is
# nightly-netem's whole history: 19 red of 21 lifetime runs, and the two greens
# were the two days a go.sum change rotated the cache key.
# Writable means the container can FILL the gaps. It also warms the host cache,
# which is the behaviour a local dev loop wants anyway.
mc_mount=()
[ -n "$MODCACHE" ] && [ -d "$MODCACHE" ] && mc_mount=(-v "$MODCACHE":/go/pkg/mod)

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
# A verdict this harness did not EARN must not be printed. netem-run.sh separates
# the ways a run can end (see its header for the code contract); this renders each
# as itself. The old code had a single else-branch, so ANY non-zero exit printed
# "a property did NOT hold" — and for thirteen consecutive nights that sentence
# described a package that had failed to COMPILE with zero drills run. Nobody
# triaged it, because it read as a durability finding rather than a broken build.
case "$code" in
  0)
    echo "RESULT: PASS ✅  every ${noun} property ${verb} under [${NETEM:-CLEAN}] — certified deterministically, off-cloud."
    ;;
  4)
    echo "RESULT: SETUP FAILED ⛔  the ${noun} drills never BUILT under [${NETEM:-CLEAN}] — ZERO drills ran."
    echo "  This is NOT a property verdict: no ${noun} property was exercised, so none passed and"
    echo "  none failed. Do not read a durability finding into it. Fix the build and re-run."
    ;;
  3)
    echo "RESULT: HARNESS ERROR ⛔  the requested impairment [${NETEM}] could not be applied (needs --cap-add NET_ADMIN)."
    echo "  This is NOT a property verdict — running on an unimpaired loopback would have"
    echo "  produced a CLEAN pass wearing an adverse-network label."
    ;;
  5)
    echo "RESULT: UNEARNED ⛔  the drills built and ran, but not every NAMED drill produced a result under [${NETEM:-CLEAN}]."
    echo "  This is NOT a property verdict. A green over partial execution certifies nothing;"
    echo "  check the 'drills: N of M' line above and the -run regex."
    ;;
  125 | 126 | 127)
    echo "RESULT: HARNESS ERROR ⛔  docker could not run the container (exit $code) — no drill was reached."
    echo "  This is NOT a property verdict."
    ;;
  *)
    echo "RESULT: FAIL ❌  a ${noun} property did NOT hold under [${NETEM:-CLEAN}] (go test exit $code)."
    echo "  Per the rescue guardrail, a property you cannot drive+verify is a RED, never a passing GAP —"
    echo "  this is a REAL finding. Reproduce and fix it here; do NOT route around it on the cloud."
    ;;
esac
exit $code
