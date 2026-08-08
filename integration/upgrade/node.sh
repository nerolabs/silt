#!/bin/sh
# Version-selecting daemon entrypoint for the rolling-upgrade harness.
#
# The whole test is "swap the binary on a persisted store and prove it reloads".
# The two binaries do NOT accept the same flags, though — the OLD release
# (silt-v1 = v0.1.1) predates -advertise / -mdns / -log / -bond, so it can't be
# driven with HEAD's flag set. Rather than maintain two compose files, this
# entrypoint reads a few env vars and assembles the correct daemon invocation
# for whichever SILT_VERSION the container was (re)created with. Recreating a
# container with a different SILT_VERSION, reattached to the SAME /data volume,
# IS the upgrade under test.
#
# Env (set per-service in docker-compose.yml, SILT_VERSION overridden at swap):
#   SILT_VERSION  v1 | v2                (which baked-in binary to exec)
#   NODE_IP       this container's static IP (how peers dial us back)
#   PORT          swarm listen port (default 4001)
#   ROLE          holder | seed
#   BOOTSTRAP     ID@HOST:PORT of the seed (empty for the seed itself)
#   REG_SERVE     addr to serve the registry on (seed only, e.g. 0.0.0.0:4003)
#   CAPACITY      storage pledge (e.g. 5G, or 1 for a store-nothing vantage)
#   VALIDATOR     1 to run -validator (seed only)
#   EXTRA         extra raw flags (version-safe caller-supplied)
set -e

: "${SILT_VERSION:?set SILT_VERSION=v1|v2}"
: "${NODE_IP:?set NODE_IP (the container static IP)}"
PORT="${PORT:-4001}"
BIN="/usr/local/bin/silt-${SILT_VERSION}"
[ -x "$BIN" ] || { echo "[node] no such binary: $BIN"; exit 1; }

set -- "$BIN" daemon -store=/data -capacity="${CAPACITY:-5G}"

# Listen/advertise: v0.1.1 has no -advertise and stamps its -listen address on
# outgoing messages, so it must listen on the concrete container IP. HEAD binds
# the wildcard and advertises the routable IP explicitly.
if [ "$SILT_VERSION" = "v1" ]; then
  set -- "$@" -listen="${NODE_IP}:${PORT}"
else
  set -- "$@" -listen="0.0.0.0:${PORT}" -advertise="${NODE_IP}:${PORT}" -mdns=false -log=info
fi

[ -n "$BOOTSTRAP" ]  && set -- "$@" -bootstrap="$BOOTSTRAP"
[ -n "$REG_SERVE" ]  && set -- "$@" -serve-registry="$REG_SERVE"
# Validator with a chain-backed registry. -quorum=0 lets this single seed
# self-commit (trusted one-box swarm), so a publish commits a real block to the
# chain without a second attester — which is exactly what we want: a real,
# persisted chain to reload across the upgrade. -min-rep=0 = trusted (self-sign).
[ "${VALIDATOR:-0}" = 1 ] && set -- "$@" -validator -quorum=0 -min-rep=0
[ -n "$EXTRA" ]      && set -- "$@" $EXTRA

echo "[node] SILT_VERSION=$SILT_VERSION ROLE=${ROLE:-?} IP=$NODE_IP exec: $*"
exec "$@"
