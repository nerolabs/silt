#!/usr/bin/env bash
# silt node user-data (AWS; rendered by Terraform templatefile — $${..} shown here are
# BASH vars; the Terraform-injected values are node_name/role/argv/s3_uri/ttl_minutes/
# registry_port, referenced WITHOUT the extra $). Runs as root via cloud-init on boot.
#
# Fully self-configuring: the node's entire `silt` argv — every peer/anchor/relay
# reference — was computed deterministically before this instance existed (topology.py)
# and is baked in as ${argv}. Nothing to discover, no reconfiguration round-trip.
set -euo pipefail
exec > >(logger -t silt-startup) 2>&1

# ── Cost backstop: hard self-destruct after the TTL, no matter what ─────────────
# The instance is launched with instance_initiated_shutdown_behavior=terminate, so
# this shutdown TERMINATES it even if the orchestrator dies.
shutdown -h +${ttl_minutes} || true

NODE="${node_name}"
ROLE="${role}"
ARGV='${argv}'

# ── Download the silt binary from S3 using the instance profile (aws-cli preinstalled
#    on Amazon Linux 2023; the role grants s3:GetObject on the run's bucket) ─────────
mkdir -p /var/lib/silt /etc/silt
for _ in $(seq 1 30); do
  if aws s3 cp "${s3_uri}" /usr/local/bin/silt --quiet; then break; fi
  echo "silt-startup: $${NODE} waiting for S3 binary…"; sleep 5
done
chmod +x /usr/local/bin/silt

# Persist the argv so lib.sh restore_argv (after an adversarial relaunch) can reset it
# — the AWS analog of the GCP metadata attribute the GCP harness reads.
printf '%s' "$${ARGV}" > /etc/silt/argv

# ── systemd unit (Restart=on-failure; logs to journald, read by the orchestrator) ──
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

# ── Cold-start ordering ─────────────────────────────────────────────────────────
# #281 is FIXED IN-PRODUCT (Node.StartBootstrapRetry, -bootstrap-retry=15s default), so
# this TCP-wait is NOT required for correctness — it models the seed-first ordering a
# real deployment uses. Set SKIP_BOOTSTRAP_WAIT=1 (via a future flow) to exercise the
# empty-routing-table self-heal over the wire (parity with the GCP harness hook).
BOOTSTRAP_REF="$(printf '%s\n' "$${ARGV}" | grep -oE -- '-bootstrap [^ ]+' | head -1 | awk '{print $2}' || true)"
if [ "$${SKIP_BOOTSTRAP_WAIT:-0}" = 1 ]; then
  echo "silt-startup: $${NODE} SKIP_BOOTSTRAP_WAIT=1 — relying on in-product -bootstrap-retry self-heal (#281)"
  BOOTSTRAP_REF=""
fi
if [ -n "$${BOOTSTRAP_REF}" ]; then
  BHOSTPORT="$${BOOTSTRAP_REF#*@}"; BHOST="$${BHOSTPORT%:*}"; BPORT="$${BHOSTPORT##*:}"
  echo "silt-startup: $${NODE} waiting (≤240s) for bootstrap $${BHOST}:$${BPORT} to listen…"
  for _ in $(seq 1 120); do
    if timeout 2 bash -c ": </dev/tcp/$${BHOST}/$${BPORT}" 2>/dev/null; then
      echo "silt-startup: bootstrap reachable — starting silt"; break
    fi
    sleep 2
  done
fi

systemctl enable --now silt.service
echo "silt-startup: $${NODE} up — argv: $${ARGV}"
