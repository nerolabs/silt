#!/usr/bin/env python3
"""topology.py (two-cloud) — SCAFFOLD.

Assigns the 13-node topology across GCP + AWS and emits:
  topology.json            — combined map; each node carries a `cloud` (gcp|aws) field
                             the unified lib.sh router dispatches on, plus its role/seed/
                             nodeid and a `public_ip` (a PLACEHOLDER until phase-0 reserves
                             real static IPs — see README §3).
  gcp.nodes.json           — the GCP subset, shaped for cloudtest/terraform's var.nodes.
  aws.nodes.json           — the AWS subset, shaped for awstest/terraform's var.nodes.

What is REAL here: the split, the deterministic NodeIDs, and the combined shape the
router + scenarios consume. What is STUBBED (the phase-0 TODO): the cross-provider
public IPs the argv must reference, because peers on the other cloud reach a node only
by its public IP. The argv itself is therefore emitted by a SECOND pass (rebind) once
the reserved public IPs are known — see twocloud.sh. This file locks the split + shape.

Env: SILT_BIN, BOND_MODE, SPLIT (comma-separated `name=gcp|aws` overrides), SMOKE.
"""
import json, os, shlex, subprocess, sys

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SILT_BIN = os.environ.get("SILT_BIN", "go run ./cmd/silt")

# ── The node table + DEFAULT cloud split (README §4): validators on BOTH clouds so
#    quorum crosses the provider boundary; boot/registry + relay on GCP (public); the
#    natted pair on AWS (reaching the GCP relay over the internet — real cross-provider
#    NAT traversal). Override any assignment with SPLIT="nat-1=gcp,val-c=gcp".
#   name        role         seed   default_cloud
NODES = [
    ("val-a",     "validator", 6001, "gcp"),   # boot + serves the registry
    ("val-b",     "validator", 6002, "gcp"),
    ("val-c",     "validator", 6003, "aws"),
    ("val-d",     "validator", 6004, "aws"),
    ("store-1",   "storage",   6101, "gcp"),
    ("store-2",   "storage",   6102, "aws"),
    ("relay",     "relay",     6301, "gcp"),   # public — natted nodes on either cloud reach it
    ("fetch-1",   "fetcher",   6401, "gcp"),
    ("adversary", "adversary", 6901, "aws"),
    ("nat-1",     "natted",    6501, "aws"),
    ("nat-2",     "natted",    6502, "aws"),
]

if os.environ.get("SMOKE") == "1":
    _keep = {"val-a", "val-c", "store-1", "fetch-1"}   # spans both clouds, minimal
    NODES = [n for n in NODES if n[0] in _keep]

# SPLIT overrides: "val-c=gcp,nat-1=gcp"
_overrides = {}
for kv in filter(None, os.environ.get("SPLIT", "").split(",")):
    k, _, v = kv.partition("=")
    if k and v in ("gcp", "aws"):
        _overrides[k.strip()] = v.strip()


def node_id(seed):
    out = subprocess.run(shlex.split(SILT_BIN) + ["id", "-id-seed", str(seed)],
                         cwd=REPO_ROOT, capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"topology: `silt id -id-seed {seed}` failed:\n{out.stderr}")
    return out.stdout.strip().splitlines()[0]


def main():
    nodes = {}
    for (name, role, seed, cloud) in NODES:
        cloud = _overrides.get(name, cloud)
        nodes[name] = {
            "role": role, "seed": seed, "cloud": cloud,
            "nodeid": node_id(seed),
            # PLACEHOLDER — phase 0 reserves a static public IP per dialable node and the
            # rebind pass (twocloud.sh) replaces this before baking the argv. Natted nodes
            # never get a public IP (they reach the swarm through the relay).
            "public_ip": "" if role == "natted" else f"PUBIP_{name}",
        }

    validators = [n for n, v in nodes.items() if v["role"] == "validator"]
    here = os.path.dirname(os.path.abspath(__file__))
    combined = {
        "meta": {
            "n_val": len(validators),
            "quorum": max(1, len(validators) - 2),
            "boot": validators[0] if validators else None,
            "clouds": sorted({v["cloud"] for v in nodes.values()}),
            "note": "SCAFFOLD — public_ip fields are placeholders; argv is rebound post phase-0 IP reservation.",
        },
        "nodes": nodes,
    }
    with open(os.path.join(here, "topology.json"), "w") as f:
        json.dump(combined, f, indent=2)

    # Per-cloud subsets, so each cloud's existing Terraform (cloudtest/awstest) provisions
    # only its share (both already take var.nodes as a map). Shapes are filled by the
    # rebind pass once IPs + argv exist; here we emit the membership + roles.
    for cloud in ("gcp", "aws"):
        sub = {n: {"role": v["role"], "seed": v["seed"], "nodeid": v["nodeid"],
                   "public_ip": v["public_ip"]}
               for n, v in nodes.items() if v["cloud"] == cloud}
        with open(os.path.join(here, f"{cloud}.nodes.json"), "w") as f:
            json.dump(sub, f, indent=2)

    g = sum(1 for v in nodes.values() if v["cloud"] == "gcp")
    a = sum(1 for v in nodes.values() if v["cloud"] == "aws")
    sys.stderr.write(
        f"topology(two-cloud): {len(nodes)} nodes — {g} on GCP, {a} on AWS; "
        f"{len(validators)} validators (quorum {combined['meta']['quorum']}); "
        f"wrote topology.json + gcp.nodes.json + aws.nodes.json\n")
    sys.stderr.write("  SCAFFOLD: public_ip fields are placeholders — wire phase-0 reservation + argv rebind (README §3/§6).\n")


if __name__ == "__main__":
    main()
