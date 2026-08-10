#!/bin/sh
# Apply kernel network impairment to this container's primary interface, then run
# the real command. $NETEM is a raw `tc qdisc ... netem` argument string, e.g.
#   NETEM="delay 80ms 20ms distribution normal loss 3%"
# Empty $NETEM = no impairment (a clean-network control run). We find the default-
# route interface rather than hard-coding eth0 (Docker names vary), and never fail
# the container if tc is unavailable — a missing NET_ADMIN degrades to a clean run,
# which the harness detects and reports rather than silently passing.
set -e
if [ -n "${NETEM}" ]; then
  IFACE="$(ip route show default 2>/dev/null | awk '/default/{print $5; exit}')"
  IFACE="${IFACE:-eth0}"
  if tc qdisc add dev "${IFACE}" root netem ${NETEM} 2>/dev/null; then
    echo "netem: impaired ${IFACE} with [${NETEM}]"
  else
    echo "netem: FAILED to apply on ${IFACE} (need cap_add NET_ADMIN) — running CLEAN"
  fi
fi
exec "$@"
