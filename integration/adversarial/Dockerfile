# Deterministic adversarial-consensus certification image: the Go toolchain to
# build+run the e2e drivers, plus iproute2 so the harness can impair the
# container's loopback with `tc netem`. The silt source is MOUNTED at run time
# (read-only) rather than copied in, so the cert always runs against the working
# tree and there is no stale prebuilt binary (cf. the deleted flakynet/silt).
FROM golang:1.26-bookworm
RUN apt-get -qq update \
 && apt-get -qq install -y --no-install-recommends iproute2 \
 && rm -rf /var/lib/apt/lists/*
# No COPY: /silt is a read-only bind mount; GOCACHE stays ephemeral in the layer.
WORKDIR /silt
