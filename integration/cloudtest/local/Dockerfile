# LOCAL=1 cloudtest node image: the smallest container that can run the silt
# daemon plus the exact commands the harness sends over ssh_node (bash, curl,
# pgrep/pkill, sha256sum, grep/sed/awk). The systemd/journald surface is provided
# by the shims in ./shims, bind-mounted at the real paths.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      bash curl ca-certificates procps coreutils grep sed gawk \
    && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /var/lib/silt /var/log /etc/systemd/system
CMD ["sleep", "infinity"]
