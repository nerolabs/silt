# Runtime image for the FLAKY-NETWORK durability harness. Same host-built silt
# binary as the other suites, plus `iproute2` for `tc netem` (kernel network
# impairment: latency, jitter, packet loss). The netem-entry.sh wrapper applies
# the impairment from $NETEM before exec'ing the real command, so every container
# on this network runs silt through a degraded link — the "real world" a P2P node
# actually faces. Needs cap_add: NET_ADMIN (set in docker-compose) for tc.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates procps iputils-ping iproute2 \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
COPY netem-entry.sh /usr/local/bin/netem-entry.sh
RUN chmod +x /usr/local/bin/silt /usr/local/bin/netem-entry.sh
ENTRYPOINT ["/usr/local/bin/netem-entry.sh"]
CMD ["silt", "help"]
