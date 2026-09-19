# Runtime image for the retention-decay harness. It carries the prebuilt `silt`
# binary; ONE image plays every role in the topology (each container is just a
# `silt daemon` with a different seed).
#
# The binary is built on the host and COPYed in (not compiled in-container): it
# keeps this image tiny, avoids a ~1 GB Go-build memory spike, and lets CI reuse
# the compile it already did. run.sh does the host build.
#
# procps is for inspecting a live daemon when a leg fails.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates procps \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
RUN chmod +x /usr/local/bin/silt
ENTRYPOINT []
CMD ["silt", "help"]
