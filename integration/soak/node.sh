#!/bin/sh
# Holder/seed entrypoint for the soak harness.
#
# Every daemon sits on ONE flat bridge network, so a wildcard bind (0.0.0.0)
# is NOT dialable on its own — peers need a concrete HOST:PORT to reach us.
# We discover our own container IP from /etc/hostname and stamp it with
# `-advertise=<ip>:<port>` so outgoing messages carry a routable endpoint.
# This mirrors examples/flow2 bring-up but for an arbitrarily large pool.
#
# ADV_PORT must match the port in the daemon's -listen flag (4001 here).
set -e
ADV_PORT="${ADV_PORT:-4001}"
IP=$(getent hosts "$(cat /etc/hostname)" | awk '{print $1}' | head -1)
[ -n "$IP" ] || { echo "[node] FATAL: could not resolve own container IP"; exit 1; }
echo "[node] self IP $IP; advertise $IP:$ADV_PORT; exec: $*"
exec "$@" -advertise="$IP:$ADV_PORT"
