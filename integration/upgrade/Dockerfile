# Runtime image for the rolling-upgrade / on-disk-format-stability harness
# (field test #11). Unlike the other harnesses, this image carries BOTH
# prebuilt binaries — an OLD release (silt-v1) and HEAD (silt-v2) — so a single
# image, reattached to the SAME named volume, can play a node on either version.
# The upgrade is a container RECREATE with a different SILT_VERSION env, pointed
# at the unchanged /data volume: exactly a real binary swap on a persisted store.
#
# Both binaries are built on the host and COPYed in (not compiled in-container):
# it keeps this image tiny, avoids a ~1 GB Go-build memory spike (so it runs on
# small hosts), and lets the driver reuse the compiles it already did. run.sh
# does the host builds; see it for the exact `go build` invocations and which
# commit each binary is cut from.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates procps iputils-ping \
 && rm -rf /var/lib/apt/lists/*
COPY silt-v1 /usr/local/bin/silt-v1
COPY silt-v2 /usr/local/bin/silt-v2
COPY node.sh /usr/local/bin/node.sh
RUN chmod +x /usr/local/bin/silt-v1 /usr/local/bin/silt-v2 /usr/local/bin/node.sh
ENTRYPOINT []
CMD ["node.sh"]
