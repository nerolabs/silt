# LOCAL=1 cloudtest node image: the smallest container that can run the silt
# daemon plus the exact commands the harness sends over ssh_node (bash, curl,
# pgrep/pkill, sha256sum, grep/sed/awk). The systemd/journald surface is provided
# by the shims in ./shims, bind-mounted at the real paths.
FROM debian:bookworm-slim
# iproute2 carries `ip` and `tc`: the field sheet shapes swarm-bound egress with
# tc netem to drive the chain under latency, jitter, loss and reordering, and a
# LOCAL run that could not apply the same shaping would leave that mechanism
# untried until it was costing money.
RUN apt-get update && apt-get install -y --no-install-recommends \
      bash curl ca-certificates procps coreutils grep sed gawk iproute2 \
    && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /var/lib/silt /var/log /etc/systemd/system
CMD ["sleep", "infinity"]
