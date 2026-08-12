#!/bin/sh
# Runs INSIDE the container. Impairs the loopback the e2e daemons talk over
# (they listen/dial on 127.0.0.1, so `tc netem` on `lo` degrades the real
# consensus traffic), then runs the adversarial e2e drivers under that
# impairment. The attacks are DRIVEN and must be DENIED — exactly the thing the
# flaky live cloud kept failing to even set up.
#
# Honest-green discipline (build-immutable #4): if an impairment was REQUESTED
# but tc could not apply it (missing --cap-add NET_ADMIN), we EXIT NON-ZERO
# rather than run a false CLEAN pass that would look like an adverse-network cert.
set -e

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

cd /silt
echo "== e2e adversarial-consensus drills (run=${TESTS}) =="
# No -short: the drills spawn real daemons over real TCP. -count=1 forbids a
# cached pass. go test's exit code IS the verdict: 0 = every attack denied.
exec go test ./e2e -run "${TESTS}" -count=1 -v -timeout "${TIMEOUT:-900s}"
