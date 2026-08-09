# Runtime image for the durability field test. Carries the prebuilt silt binary
# + the self-advertising role.sh entrypoint (built on the host by run.sh).
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates coreutils \
 && rm -rf /var/lib/apt/lists/*
COPY silt /usr/local/bin/silt
COPY role.sh /usr/local/bin/role.sh
RUN chmod +x /usr/local/bin/silt /usr/local/bin/role.sh
ENTRYPOINT []
CMD ["silt", "help"]
