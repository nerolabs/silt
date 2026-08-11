#!/usr/bin/env bash
# silt node startup (rendered by Terraform templatefile; $${..} are BASH vars,
# $${ttl_minutes} below with no $ is the Terraform-injected value).
#
# Fully self-configuring: the node's entire `silt` argv — with every peer ID,
# anchor, and relay reference — was computed deterministically before this VM
# existed (topology.py) and handed in as the `silt-argv` metadata attribute. So
# there is nothing to discover and no reconfiguration round-trip.
set -euo pipefail
exec > >(logger -t silt-startup) 2>&1

# ── Cost backstop: hard self-destruct after the TTL, no matter what ─────────────
shutdown -h +${ttl_minutes} || true

meta() { curl -s -H "Metadata-Flavor: Google" "http://metadata.google.internal/computeMetadata/v1/instance/$1"; }

ARGV="$(meta attributes/silt-argv)"
BIN_URL="$(meta attributes/silt-binary-url)"     # gs://bucket/object
NODE="$(meta attributes/node-name)"

# ── Download the silt binary from GCS using the VM's own access token ───────────
# (avoids depending on gsutil/gcloud being preinstalled on the image)
TOKEN="$(meta service-accounts/default/token | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')"
BUCKET="$${BIN_URL#gs://}"; OBJECT="$${BUCKET#*/}"; BUCKET="$${BUCKET%%/*}"
mkdir -p /var/lib/silt
curl -sf -H "Authorization: Bearer $${TOKEN}" \
  "https://storage.googleapis.com/storage/v1/b/$${BUCKET}/o/$${OBJECT}?alt=media" \
  -o /usr/local/bin/silt
chmod +x /usr/local/bin/silt

# ── Run silt under systemd so it restarts on failure and logs to journald ───────
# (the orchestrator reads progress with `journalctl -u silt` over SSH)
cat >/etc/systemd/system/silt.service <<UNIT
[Unit]
Description=silt field-test node ($${NODE})
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/var/lib/silt
ExecStart=/usr/local/bin/silt $${ARGV}
Restart=on-failure
RestartSec=2
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload

# ── Cold-start ordering (found on the 13-node cross-region full run) ────────────
# This was the symptom of #281: silt originally did a ONE-SHOT bootstrap, so a
# joining node whose -bootstrap target was not yet listening came up with an EMPTY
# routing table and NEVER re-bootstrapped. #281 is now FIXED IN-PRODUCT —
# Node.StartBootstrapRetry (-bootstrap-retry=15s, default on) periodically re-runs
# the join while the table is empty and logs "re-bootstrapped: recovered from an
# empty routing table". This TCP-wait is therefore no longer required for
# correctness; it is kept because it models the seed-first ordering a real
# deployment uses (and mirrors the nat/consensus/upgrade harnesses).
# NOTE (ROADMAP #14): with this belt fastened, no flow exercises an empty-table
# join over the wire, so the in-product fix is not yet wire-certified. To certify
# it, set SKIP_BOOTSTRAP_WAIT=1 on a single joining validator and assert the
# "re-bootstrapped: recovered from an empty routing table" line appears.
BOOTSTRAP_REF="$(printf '%s\n' "$${ARGV}" | grep -oE -- '-bootstrap [^ ]+' | head -1 | awk '{print $2}' || true)"
if [ "$${SKIP_BOOTSTRAP_WAIT:-0}" = 1 ]; then
  # #281 wire-cert: skip the TCP-wait belt so this node joins into an empty routing
  # table and must self-heal via the in-product -bootstrap-retry loop.
  echo "silt-startup: $${NODE} SKIP_BOOTSTRAP_WAIT=1 — no TCP-wait; relying on in-product -bootstrap-retry self-heal (#281 wire-cert)"
  BOOTSTRAP_REF=""
fi
if [ -n "$${BOOTSTRAP_REF}" ]; then
  BHOSTPORT="$${BOOTSTRAP_REF#*@}"; BHOST="$${BHOSTPORT%:*}"; BPORT="$${BHOSTPORT##*:}"
  echo "silt-startup: $${NODE} waiting (≤240s) for bootstrap $${BHOST}:$${BPORT} to listen…"
  for _ in $(seq 1 120); do
    if timeout 2 bash -c ": </dev/tcp/$${BHOST}/$${BPORT}" 2>/dev/null; then
      echo "silt-startup: bootstrap $${BHOST}:$${BPORT} reachable — starting silt"; break
    fi
    sleep 2
  done
fi

systemctl enable --now silt.service
echo "silt-startup: $${NODE} up — argv: $${ARGV}"
