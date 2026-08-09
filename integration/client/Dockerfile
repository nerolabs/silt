# Runtime image for the client / web-UI path field test. Carries the prebuilt
# silt binary + curl (to drive the daemon's HTTP UI exactly as a browser would).
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl coreutils \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
COPY role.sh /usr/local/bin/role.sh
RUN chmod +x /usr/local/bin/silt /usr/local/bin/role.sh
ENTRYPOINT []
CMD ["silt", "help"]
