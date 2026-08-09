# Runtime image for the retrieval-at-scale harness. One image plays every role
# in a flat-network pool (seed/registry, holder, ephemeral publish/fetch client);
# the silt binary is built on the HOST and COPYed in (keeps the image tiny and
# avoids a Go-build memory spike in Docker — run.sh does the host build).
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates coreutils \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
COPY role.sh /usr/local/bin/role.sh
RUN chmod +x /usr/local/bin/silt /usr/local/bin/role.sh
ENTRYPOINT []
CMD ["silt", "help"]
