#!/usr/bin/env python3
"""topology.py (AWS) — the "brains" of the AWS field test.

Mirror of integration/cloudtest/topology.py for AWS. Turns the SAME node table into:

  topology.json                        — the orchestrator + the SHARED scenarios.sh
                                          (../cloudtest/scenarios.sh) address nodes by
                                          role from this. Identical shape to the GCP one.
  terraform/topology.auto.tfvars.json  — Terraform materialises one EC2 instance per
                                          node, each with its fully-rendered `silt` argv
                                          baked into the user-data startup script.

The argv/meta logic is IDENTICAL to the GCP topology.py — the `silt` command line is
substrate-agnostic. Only the placement differs: AWS has no global VPC, so v1 runs in
ONE region across 3 AZs (pre-assigned private IPs route natively within the VPC), which
keeps the GCP design's "zero post-apply reconfiguration" intact. See README for why
single-region multi-AZ (and multi-region AWS as a future enhancement).

Env knobs: SILT_BIN, BOND_MODE, SWARM_PORT/RELAY_PORT/REGISTRY_PORT, AWS_REGION, SMOKE, SYBILS.
"""
import json, os, shlex, subprocess, sys

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SILT_BIN = os.environ.get("SILT_BIN", "go run ./cmd/silt")
SWARM_PORT = int(os.environ.get("SWARM_PORT", "4001"))
RELAY_PORT = int(os.environ.get("RELAY_PORT", "4002"))
REGISTRY_PORT = int(os.environ.get("REGISTRY_PORT", "8443"))
BOND_MODE = os.environ.get("BOND_MODE", "fast")
AWS_REGION = os.environ.get("AWS_REGION", "us-west-2")
STORE = "/var/lib/silt"

VPC_CIDR = os.environ.get("VPC_CIDR", "10.20.0.0/16")
# One /24 public subnet per AZ (az slot 0/1/2 → 10.20.1/2/3.0/24) + one private NAT
# subnet (10.20.9.0/24, in az slot 0). The host octet of each node's IP (which encodes
# role: .11-.14 validators, .21-.22 storage, …) is preserved into its AZ's subnet, so
# addressing stays legible and deterministic — same trick as the GCP per-region remap,
# but per-AZ within a single regional VPC.
PUBLIC_SUBNET = {0: "10.20.1.0/24", 1: "10.20.2.0/24", 2: "10.20.3.0/24"}
NAT_SUBNET = "10.20.9.0/24"

# ── The node table ─────────────────────────────────────────────────────────────
# Identical roles/seeds/host-octets to the GCP topology; `az` is the AZ SLOT (0/1/2),
# mirroring the GCP grouping (primary cluster in slot 0, the us-east1 pair in slot 1,
# the europe single in slot 2) so the fault-domain spread is the same shape.
#   name        role         seed  host_octet  az_slot
NODES = [
    ("val-a",     "validator", 6001, 11, 0),
    ("val-b",     "validator", 6002, 12, 1),
    ("val-c",     "validator", 6003, 13, 2),
    ("val-d",     "validator", 6004, 14, 0),
    ("store-1",   "storage",   6101, 21, 0),
    ("store-2",   "storage",   6102, 22, 1),
    ("registry",  "registry",  6201, 31, 0),
    ("relay",     "relay",     6301, 41, 0),
    ("fetch-1",   "fetcher",   6401, 51, 0),
    ("adversary", "adversary", 6901, 91, 0),
    ("natgw",     "natgw",        0,  2, 0),   # AWS uses a managed NAT gateway; this row is inert (see below)
    ("nat-1",     "natted",    6501, 11, 0),
    ("nat-2",     "natted",    6502, 12, 0),
]

# SMOKE=1 trims to the cheapest set that still exercises the whole
# terraform → provision → SSM → publish → commit → fetch plumbing (~4 nodes).
if os.environ.get("SMOKE") == "1":
    _keep = {"val-a", "val-b", "store-1", "fetch-1"}
    NODES = [n for n in NODES if n[0] in _keep]

# C2 Sybil-capture cohort (opt-in) — identical semantics to the GCP harness.
SYBILS = 0 if os.environ.get("SMOKE") == "1" else int(os.environ.get("SYBILS", "0"))
for _i in range(SYBILS):
    NODES.append((f"sybil-{_i+1}", "sybil", 6601 + _i, 61 + _i, _i % 3))


def node_id(seed):
    """Deterministic NodeID for an id-seed, via `silt id` (cmd/silt/id.go)."""
    out = subprocess.run(shlex.split(SILT_BIN) + ["id", "-id-seed", str(seed)],
                         cwd=REPO_ROOT, capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"topology: `silt id -id-seed {seed}` failed:\n{out.stderr}")
    return out.stdout.strip().splitlines()[0]


def main():
    az_letters = ["a", "b", "c"]
    nodes = {}
    for (name, role, seed, host, az_slot) in NODES:
        az = f"{AWS_REGION}{az_letters[az_slot]}"
        if role == "natted":
            ip = f"10.20.9.{host}"          # private NAT subnet
            subnet_cidr = NAT_SUBNET
            az_slot = 0                      # single NAT subnet lives in slot 0
            az = f"{AWS_REGION}{az_letters[0]}"
        else:
            ip = f"10.20.{az_slot + 1}.{host}"   # slot 0→10.20.1, 1→10.20.2, 2→10.20.3
            subnet_cidr = PUBLIC_SUBNET[az_slot]
        nodes[name] = {"role": role, "seed": seed, "ip": ip, "az": az,
                       "az_slot": az_slot, "subnet_cidr": subnet_cidr,
                       # `region` kept for parity with the GCP topology.json shape.
                       "region": AWS_REGION}

    for name, n in nodes.items():
        n["nodeid"] = "" if n["role"] == "natgw" else node_id(n["seed"])

    validators = [name for name, n in nodes.items() if n["role"] == "validator"]
    n_val = len(validators)
    anchors = ",".join(nodes[v]["nodeid"] for v in validators)
    sybils = [name for name, n in nodes.items() if n["role"] == "sybil"]
    n_syb = len(sybils)
    syb_quorum = (n_syb // 2 + 1) if n_syb else 0
    # quorum = n_val - 2 (min 1): losing any one validator still leaves proposer + quorum.
    quorum = max(1, n_val - 2)
    boot = validators[0]
    bootstrap = f'{nodes[boot]["nodeid"]}@{nodes[boot]["ip"]}:{SWARM_PORT}'
    relay = next((name for name, n in nodes.items() if n["role"] == "relay"), None)
    relay_ref = f'{nodes[relay]["nodeid"]}@{nodes[relay]["ip"]}:{RELAY_PORT}' if relay else ""
    regref = f'{nodes[boot]["nodeid"]}@https://{nodes[boot]["ip"]}:{REGISTRY_PORT}'

    if BOND_MODE == "faithful":
        bond, min_bond, min_floor = "2G", "1G", "1G"
    else:
        bond, min_bond, min_floor = "64M", "1M", "0"

    # -request-timeout 8s belt for the multi-node run (#286; the product size-scales the
    # deadline for the one-time ~1.5MB bond-registration block, this leaves extra margin).
    common = f"-listen 0.0.0.0:{SWARM_PORT} -store {STORE} -mdns=false -log info -request-timeout 8s"

    # ── argv(): IDENTICAL to the GCP topology.py (the silt command line is the same on
    #    any substrate). Kept in lock-step deliberately — if it diverges, the two clouds
    #    stop testing the same thing.
    def argv(name):
        n = nodes[name]
        role, ip = n["role"], n["ip"]
        if role == "validator":
            attesters = ",".join(nodes[v]["nodeid"] for v in validators if v != name)
            a = (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -validator -objective "
                 f"-min-bond {min_bond} -min-bond-floor {min_floor} -mature-validators {n_val} "
                 f"-anchors {anchors} -attesters {attesters} -quorum {quorum} "
                 f"-bond {bond} -bond-audit 30s -capacity 5G")
            a += f" -serve-registry 0.0.0.0:{REGISTRY_PORT}" if name == boot else f" -bootstrap {bootstrap}"
            return a
        if role == "storage":
            return f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} -capacity 5G"
        if role == "registry":
            return f"daemon -id-seed {n['seed']} -store {STORE} -log info -registry-only -serve-registry 0.0.0.0:{REGISTRY_PORT}"
        if role == "relay":
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-relay 0.0.0.0:{RELAY_PORT} -capacity 5G")
        if role == "fetcher":
            return f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} -capacity 2G"
        if role == "natted":
            return f"daemon -id-seed {n['seed']} {common} -bootstrap {bootstrap} -relay-via {relay_ref} -capacity 2G"
        if role == "adversary":
            adv_attesters = ",".join(nodes[v]["nodeid"] for v in validators)
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-validator -objective -min-bond {min_bond} -min-bond-floor {min_floor} "
                    f"-mature-validators {n_val} -anchors {anchors} -attesters {adv_attesters} "
                    f"-quorum {quorum} -bond {bond} -bond-audit 30s -capacity 2G")
        if role == "sybil":
            syb_attesters = ",".join(nodes[s]["nodeid"] for s in sybils if s != name)
            att = f" -attesters {syb_attesters}" if syb_attesters else ""
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-validator -objective -min-bond {min_bond} -min-bond-floor {min_floor} "
                    f"-mature-validators {n_val} -anchors {anchors}{att} "
                    f"-quorum {syb_quorum} -bond {bond} -bond-audit 30s -capacity 2G -domain sybilnet")
        if role == "natgw":
            return "NATGW"   # AWS uses a managed NAT gateway (terraform), not a silt node
        sys.exit(f"topology: unknown role {role} for {name}")

    for name, n in nodes.items():
        n["argv"] = argv(name)

    meta = {
        "swarm_port": SWARM_PORT, "relay_port": RELAY_PORT, "registry_port": REGISTRY_PORT,
        "bond_mode": BOND_MODE, "n_val": n_val, "quorum": quorum, "bootstrap": bootstrap,
        "sybils": sybils, "n_syb": n_syb, "syb_quorum": syb_quorum, "syb_domain": "sybilnet",
        "relay_ref": relay_ref, "regref": regref, "anchors": anchors, "boot": boot,
        "region": AWS_REGION, "vpc_cidr": VPC_CIDR,
    }
    here = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(here, "topology.json"), "w") as f:
        json.dump({"meta": meta, "nodes": nodes}, f, indent=2)

    os.makedirs(os.path.join(here, "terraform"), exist_ok=True)
    # AWS Terraform materialises one aws_instance per non-natgw node. The natgw row is a
    # managed NAT gateway (declared directly in main.tf), so it is excluded here.
    tfnodes = {name: {"role": n["role"], "ip": n["ip"], "az": n["az"], "az_slot": n["az_slot"],
                      "subnet_cidr": n["subnet_cidr"], "region": n["region"], "argv": n["argv"]}
               for name, n in nodes.items() if n["role"] != "natgw"}
    tfvars = {
        "region": AWS_REGION,
        "vpc_cidr": VPC_CIDR,
        "public_subnets": PUBLIC_SUBNET,
        "nat_subnet": NAT_SUBNET,
        "nodes": tfnodes,
        "swarm_port": SWARM_PORT, "relay_port": RELAY_PORT, "registry_port": REGISTRY_PORT,
    }
    with open(os.path.join(here, "terraform", "topology.auto.tfvars.json"), "w") as f:
        json.dump(tfvars, f, indent=2)

    sys.stderr.write(
        f"topology(aws): wrote topology.json + terraform/topology.auto.tfvars.json "
        f"({len(nodes)} nodes, {n_val} validators, quorum {quorum}, region {AWS_REGION}, bond={BOND_MODE})\n")


if __name__ == "__main__":
    main()
