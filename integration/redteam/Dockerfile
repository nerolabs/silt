# Runtime image for the Byzantine red-team harness (#184 validator
# accountability). It carries the prebuilt `silt` binary on a slim base — one
# image plays every role in the topology: honest validators and the three
# adversaries (equivocator, forged-block proposer, low-bond proposer).
#
# The binary is built on the host and COPYed in (not compiled in-container): it
# keeps this image tiny, avoids a ~1 GB Go-build memory spike inside Docker (so
# this runs on small hosts too), and lets CI reuse the compile it already did.
# run.sh does the host build; see it for the exact `go build` invocation.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates procps \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
RUN chmod +x /usr/local/bin/silt
ENTRYPOINT []
CMD ["silt", "help"]
