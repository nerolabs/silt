# Runtime image for the floor-spec harness. It carries the prebuilt `silt`
# binary; ONE image plays every role in the topology (each container is just a
# `silt daemon` with different flags and different cgroup limits).
#
# The binary is built on the host and COPYed in (not compiled in-container): it
# keeps this image tiny, avoids a ~1 GB Go-build memory spike, and lets CI reuse
# the compile it already did. run.sh does the host build.
#
# coreutils is named explicitly because run.sh reads the floor box's own limits
# from inside it: `nproc` for the effective core count and `du -sb` for the
# on-disk store size. procps is for the same reason a sibling harness carries
# it — inspecting a live daemon when a leg fails.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates procps coreutils \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
RUN chmod +x /usr/local/bin/silt
ENTRYPOINT []
CMD ["silt", "help"]
