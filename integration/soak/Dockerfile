# Runtime image for the soak/scale harness. Carries the prebuilt `silt`
# binary and a self-advertising entrypoint, so ONE image plays every role in
# a flat-network pool: seed/registry, holder, and ephemeral publish/fetch
# client. procps is included so `docker stats` and in-container process checks
# have the usual tools.
#
# The binary is built on the HOST and COPYed in (not compiled in-container):
# it keeps the image tiny and avoids a ~1 GB Go-build memory spike inside
# Docker. run.sh does the host build; see it for the exact `go build`.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends procps ca-certificates coreutils \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
COPY node.sh /usr/local/bin/node.sh
RUN chmod +x /usr/local/bin/silt /usr/local/bin/node.sh
ENTRYPOINT []
CMD ["silt", "help"]
