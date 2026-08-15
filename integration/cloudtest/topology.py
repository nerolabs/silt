#!/usr/bin/env python3
"""topology.py — the "brains" of the field test.

Turns a small, human-editable table of nodes (role + deterministic id-seed +
static internal IP + zone) into two artifacts:

  topology.json                        — the orchestrator + scenarios address
                                          nodes by role over SSH from this.
  terraform/topology.auto.tfvars.json  — Terraform materialises one SPOT instance
                                          per node, each with its fully-rendered
                                          `silt` argv baked into the startup script.

Why this works with zero post-apply reconfiguration: `silt id -id-seed N` is
DETERMINISTIC (cmd/silt/id.go) and the internal IPs are STATIC (we choose them
before apply), so every -bootstrap / -anchors / -attesters / relay reference is
known BEFORE any VM exists. Each node's startup script is therefore fully
self-configuring and the swarm converges without a discovery wait.

Edit NODES to change the topology; re-run to regenerate. Env knobs:
  SILT_BIN    how to invoke silt to compute NodeIDs (default: "go run ./cmd/silt"
              from the repo root; a prebuilt "silt" binary is faster)
  BOND_MODE   fast (default) | faithful   — see README (mechanism vs economics)
  SWARM_PORT / RELAY_PORT / REGISTRY_PORT
"""
import json, os, shlex, subprocess, sys

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SILT_BIN = os.environ.get("SILT_BIN", "go run ./cmd/silt")
SWARM_PORT = int(os.environ.get("SWARM_PORT", "4001"))
RELAY_PORT = int(os.environ.get("RELAY_PORT", "4002"))
REGISTRY_PORT = int(os.environ.get("REGISTRY_PORT", "8443"))
BOND_MODE = os.environ.get("BOND_MODE", "fast")
# LOG_LEVEL: daemon -log level for every node. Default "info" (the "block committed" line
# is enough to certify genesis). Set "debug" to also capture the propose→gather→attest
# path (#327) and the handshake attribution (#332: concurrent count + elapsed) — the signal
# that root-causes a #286 stall if one recurs (docs/network-durability.md §8).
LOG_LEVEL = os.environ.get("LOG_LEVEL", "info")
# LOOP_BUDGET=1 adds -loop-budget so every node emits its per-window event-loop
# goroutine-budget decomposition at INFO — names the handler that eats the single
# thread (or, via the always-on queue-wait/slow/hang lines, one that starves/hangs
# it) WITHOUT the -log debug firehose that would skew the measurement. For a
# throughput/latency diagnostic run (e.g. the MATURING single-goroutine analysis).
LOOP_BUDGET = " -loop-budget" if os.environ.get("LOOP_BUDGET", "") not in ("", "0") else ""
PUBLIC_CIDR = os.environ.get("PUBLIC_CIDR", "10.20.0.0/24")
NAT_CIDR = os.environ.get("NAT_CIDR", "10.30.0.0/24")
DEFAULT_REGION = os.environ.get("REGION", "us-central1")
# PoC (blind field-test #2): let the PRIMARY cluster's zone follow REGION so a
# capacity-short zone (e.g. us-central1-a) can be dodged without hand-editing every
# row. Defaults to us-central1-a when REGION is unset (identical to prior behavior).
# The secondary spread (us-east1-b / europe-west1-b) is intentionally left as-is so
# the run stays multi-region and within the 8-IP/region IN_USE_ADDRESSES quota.
PRIMARY_ZONE = os.environ.get("PRIMARY_ZONE", f"{DEFAULT_REGION}-a")
STORE = "/var/lib/silt"

# ── The node table ─────────────────────────────────────────────────────────────
# role: validator | storage | registry | relay | fetcher | natgw | natted | adversary | sybil | maturer
# IPs are STATIC and must sit in the subnet the role belongs to (public vs nat).
# Zones are spread across regions on purpose, to exercise REAL inter-node latency.
NODES = [
    # name        role         seed  ip            zone
    ("val-a",     "validator", 6001, "10.20.0.11", PRIMARY_ZONE),
    ("val-b",     "validator", 6002, "10.20.0.12", "us-east1-b"),
    ("val-c",     "validator", 6003, "10.20.0.13", "europe-west1-b"),
    ("val-d",     "validator", 6004, "10.20.0.14", PRIMARY_ZONE),
    ("store-1",   "storage",   6101, "10.20.0.21", PRIMARY_ZONE),
    ("store-2",   "storage",   6102, "10.20.0.22", "us-east1-b"),
    ("registry",  "registry",  6201, "10.20.0.31", PRIMARY_ZONE),
    ("relay",     "relay",     6301, "10.20.0.41", PRIMARY_ZONE),
    ("fetch-1",   "fetcher",   6401, "10.20.0.51", PRIMARY_ZONE),
    ("adversary", "adversary", 6901, "10.20.0.91", PRIMARY_ZONE),
    ("natgw",     "natgw",        0, "10.30.0.2",  PRIMARY_ZONE),
    ("nat-1",     "natted",    6501, "10.30.0.11", PRIMARY_ZONE),
    ("nat-2",     "natted",    6502, "10.30.0.12", PRIMARY_ZONE),
]

# SMOKE=1 trims the topology to the cheapest set that still exercises the whole
# terraform → provision → SSH → publish → commit → fetch plumbing (~4 nodes, a
# few cents). Use it for the FIRST real run to shake out infra before paying for
# the full 13-node topology. Scenarios that need absent nodes (NAT, adversary,
# 4th validator) skip cleanly rather than fail.
if os.environ.get("SMOKE") == "1":
    _keep = {"val-a", "val-b", "store-1", "fetch-1"}
    NODES = [n for n in NODES if n[0] in _keep]

# C2 Sybil-capture cohort (opt-in; adds cost, runs on SPOT). A set of bonded but
# NON-ANCHOR validators, all committing ONE shared -domain (modeling one operator's
# satellite keys), that attempt to advance the chain WITHOUT the launch anchors.
# The C2 concentration metric discounts a single-domain cohort, so they cannot
# reach the bond-DISTINCT maturity threshold to shed the anchors — the capture is
# refused (ErrAnchorRequired), and ≥8 equal single-domain bonds trip the bond-
# atomization note. This is the property the LOCAL sybil suite can only scope down
# to the standing gate; the cloud is where the PURE anchor gate is certified.
# Off by default (0 extra VMs). `SYBILS=8 ./cloudtest.sh` opts in. Never in SMOKE.
SYBILS = 0 if os.environ.get("SMOKE") == "1" else int(os.environ.get("SYBILS", "0"))

# MATURING topology (re-split 2026-08-15 per the PE concurrence,
# silt-reviews/principle-engineer/maturing-topology-resplit-concurrence-PE-2026-08-15.md):
# the base topology NEVER matures BY DESIGN, and neither did the original
# MATURING=1 parameterization — the latch was UNREACHABLE BY CONSTRUCTION:
# C2Metric EXCLUDES anchors (chain.go — counting the scaffolding's own bonds to
# shed the scaffolding would be circular, immutable #3), all 4 validators here
# ARE the anchors, and the only non-anchor cohort (8 single-domain Sybils)
# aggregates to NakamotoDomains=1 < bar 2 — the certified C2 discount doing its
# job. Pinned by core/chain/maturing_topology_premise_test.go; deliberation in
# docs/thinking/2026-08-15-maturing-latch-premise-unreachable.md.
#
# `MATURING=1 SYBILS=8 ./cloudtest.sh` therefore re-splits the 8 cohort slots:
# 4 HONEST MATURERS (non-anchor validators, full 64M bond, NO -domain — unset
# domains count as independent address-diversity groups) + 4 single-domain
# MinBond Sybils. The maturers are what the maturity metric is designed to see
# (the I3 oracle's certified shape, modelcheck_i3_test.go matureWeightedEpoch,
# now on the wire): at full drain the non-anchor set is 4×64M distinct +
# 4×1M one-domain → NakamotoOperators=2, NakamotoDomains=2 → min=2 ≥ bar 2
# (margin 1, uniform across every consensus role) → the everMature latch trips,
# the anchors shed at the next epoch boundary, and the post-shed regime — the
# external red team's sharpest target (seam #8) — is field-exercised: post-shed
# commits, the B2 stall drill (cheap cohort declines to attest; honest weight
# must still commit), the B2 capture drill (the cheap cohort ALONE — honest
# maturers stopped too — must be refused for lack of frozen-weight
# super-majority), and the weak-subjectivity cold-sync.
#
# WHAT THIS FIELD-PROVES, AND WHAT IT DOES NOT (PE note 1 — disclose, don't
# inflate): it field-confirms the handoff MECHANISM — the real shed on the
# wire, post-shed commits via the bonded set, the weight-quorum SHAPE, WS
# cold-sync. It does NOT field-test the ⅓-weight STALL BOUNDARY: the 4×1M
# Sybils are ~1.5% of bonded weight, trivially outweighed, so the stall/capture
# drills confirm shape, not boundary. The boundary stays the deterministic
# tier's job (the I3 oracle) + the red team. MATURING=0 topologies (the P1
# launch gate, 5-sybil-no-capture at 8 sybils) are UNTOUCHED by this re-split
# (PE note 3). Mutually exclusive with flow 5 (the anchor-gate capture premise
# assumes a network that never sheds).
MATURING = 0 if os.environ.get("SMOKE") == "1" else int(os.environ.get("MATURING", "0"))
# Place the Sybil cohort in the SECONDARY regions' IP HEADROOM, never the primary.
# The base 13-node topology already fills the primary region to its 8-IP
# IN_USE_ADDRESSES quota, so any Sybil there overflows it (needs a manual quota bump).
# The other regions have room — europe-west1 (~5 free) then us-east1 (~3 free) — which
# exactly absorbs SYBILS=8 within every region's default 8-IP quota, so the full C2
# anchor-gate run works without a quota increase. Region placement does not affect the
# C2 property (bond-distinctness + the anchor gate), only where the VMs live. Override
# with SYB_ZONES="zoneA,zoneB,…" if your quotas differ.
_syb_zones = os.environ.get("SYB_ZONES")
if _syb_zones:
    _syb_slots = [z.strip() for z in _syb_zones.split(",") if z.strip()]
else:
    _syb_slots = ["europe-west1-b"] * 5 + ["us-east1-b"] * 3  # fill europe's headroom first, then us-east1
# MATURING re-split: the first 4 cohort slots become HONEST MATURERS (the
# non-anchor distinct-operator cohort the maturity metric gates on — see the
# MATURING comment above); the rest stay single-domain Sybils. MATURING=0 keeps
# all slots as Sybils, byte-identical to the P1 launch topology (PE note 3).
_N_MATURERS = 4 if MATURING else 0
for _i in range(SYBILS):
    _z = _syb_slots[_i] if _i < len(_syb_slots) else _syb_slots[_i % len(_syb_slots)]
    if _i < _N_MATURERS:
        NODES.append((f"maturer-{_i+1}", "maturer", 6601 + _i, f"10.20.0.{61+_i}", _z))
    else:
        NODES.append((f"sybil-{_i+1-_N_MATURERS}", "sybil", 6601 + _i, f"10.20.0.{61+_i}", _z))


def node_id(seed):
    """Deterministic NodeID for an id-seed, via `silt id`."""
    out = subprocess.run(shlex.split(SILT_BIN) + ["id", "-id-seed", str(seed)],
                         cwd=REPO_ROOT, capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"topology: `silt id -id-seed {seed}` failed:\n{out.stderr}")
    return out.stdout.strip().splitlines()[0]


def main():
    nodes = {name: {"role": role, "seed": seed, "ip": ip, "zone": zone}
             for (name, role, seed, ip, zone) in NODES}
    # PIN_ZONE forces every node into one zone (single-region mode). Use it to
    # validate the cloud path without the multi-region subnet requirement — the
    # single default-region subnet then covers every node. Default (unset) keeps
    # the real multi-region spread. (SMOKE=1 PIN_ZONE=us-central1-a → cheap,
    # single-region shakeout.)
    pin = os.environ.get("PIN_ZONE")
    if pin:
        for n in nodes.values():
            n["zone"] = pin

    # ── Per-region subnets ──────────────────────────────────────────────────────
    # GCP subnets are REGIONAL, so a node in us-east1 cannot attach to a
    # us-central1 subnet. Give each region its own /24 and remap every non-NAT
    # node's static IP into its region's network — the host octet (which encodes
    # role: .11-.14 validators, .21 storage, …) is preserved, so addressing stays
    # legible and deterministic. Cross-region internal IPs remain reachable over
    # the (global) VPC. The single NAT subnet stays in the default region.
    #   default region → 10.20.0.0/24 ; others → 10.21/.22/… (.30 is the NAT subnet)
    def region_of(zone):
        return zone.rsplit("-", 1)[0]
    pub_regions = sorted({region_of(n["zone"]) for n in nodes.values() if n["role"] != "natted"})
    octet, nxt = {DEFAULT_REGION: 20}, 21
    for r in pub_regions:
        if r == DEFAULT_REGION:
            continue
        if nxt == 30:            # reserved for the NAT subnet
            nxt += 1
        octet[r], nxt = nxt, nxt + 1
    region_cidrs = {r: f"10.{octet[r]}.0.0/24" for r in pub_regions}
    for n in nodes.values():
        if n["role"] == "natted":
            n["region"] = DEFAULT_REGION           # single NAT subnet, default region
            continue
        r = region_of(n["zone"])
        host = n["ip"].split(".")[-1]
        n["ip"] = f"10.{octet[r]}.0.{host}"        # e.g. natgw 10.30.0.2 → 10.20.0.2 (public)
        n["region"] = r

    for name, n in nodes.items():
        n["nodeid"] = "" if n["role"] == "natgw" else node_id(n["seed"])

    validators = [name for name, n in nodes.items() if n["role"] == "validator"]
    n_val = len(validators)
    anchors = ",".join(nodes[v]["nodeid"] for v in validators)
    # The C2 Sybil cohort is a SEPARATE set — deliberately NOT in `validators`, so
    # it never pollutes the honest quorum/anchor/maturity math. It references the
    # honest anchor set (which it does not control) and co-signs only itself.
    sybils = [name for name, n in nodes.items() if n["role"] == "sybil"]
    n_syb = len(sybils)
    maturers = [name for name, n in nodes.items() if n["role"] == "maturer"]
    n_mat = len(maturers)
    # syb_quorum retained for the meta/report only. NOTE (#338 cloud GAP root cause):
    # the Sybil role must run the SAME -quorum FLOOR as the rest of the swarm, NOT a
    # self-majority. -quorum is a hard floor on ValidateCommit (max(Quorum,
    # bftThreshold)); a Sybil set to n_syb//2+1 (=5 at SYBILS=8) then re-validates the
    # anchors' honestly-committed blocks (2 attestations) under ITS floor of 5, rejects
    # the whole chain in Reconcile, and stays stranded at genesis — exactly the field
    # GAP "sybil-1 never synced a committed chain (head 0)". In OBJECTIVE mode the
    # "self-majority capture" is sized by bftThreshold over the Sybils' committed bond,
    # not a config knob, so a uniform floor both lets the Sybils SYNC and makes their
    # capture attempt reach the real ANCHOR gate (ErrAnchorRequired) rather than dying
    # on a quorum count. See #380 (the objective-mode quorum-floor footgun).
    syb_quorum = (n_syb // 2 + 1) if n_syb else 0   # reporting only; the Sybil daemon uses the uniform `quorum`
    # Size quorum for f=1 crash tolerance: after one validator is down, the
    # proposer + `quorum` attesters must still be reachable. quorum = n_val - 2
    # (min 1) means losing any one validator still leaves proposer + quorum.
    # (Byzantine-quorum defaults ON for objective validators and may raise the
    # effective bar; the fault-tolerance scenario records the observed threshold
    # so the first real run tells you the exact number to pin — see README.)
    quorum = max(1, n_val - 2)
    boot = validators[0]                              # first validator = bootstrap anchor
    bootstrap = f'{nodes[boot]["nodeid"]}@{nodes[boot]["ip"]}:{SWARM_PORT}'
    relay = next((name for name, n in nodes.items() if n["role"] == "relay"), None)
    relay_ref = f'{nodes[relay]["nodeid"]}@{nodes[relay]["ip"]}:{RELAY_PORT}' if relay else ""
    # Deterministic, dialable pinned registry ref. The boot validator serves the
    # registry on 0.0.0.0:REGISTRY_PORT; a publisher must dial its INTERNAL ip
    # (not 0.0.0.0, which the daemon prints as its bound address). We construct it
    # here from the known NodeID + ip rather than scraping the daemon's log line,
    # so REGREF is never empty (the old log-scrape regex could not match the real
    # `registry: chain-backed, serving <id>@https://...` banner) and never 0.0.0.0.
    regref = f'{nodes[boot]["nodeid"]}@https://{nodes[boot]["ip"]}:{REGISTRY_PORT}'

    # Bond economics: FAST proves the mechanism cheaply; FAITHFUL is economically real.
    if BOND_MODE == "faithful":
        bond, min_bond, min_floor = "2G", "1G", "1G"
    else:
        bond, min_bond, min_floor = "64M", "1M", "0"

    # Maturity parameterization (uniform across EVERY consensus role — a mismatch
    # would perturb objective quorum math). Base: bar = n_val (never reached by 4
    # equal bonds — the young-regime topology). MATURING: bar = 2 at margin 1 (the
    # coefficient 4 distinct-operator equal bonds actually reach), so the latch
    # trips on the wire; the cohort bonds the MINIMUM so B2's drills price
    # per-head cheapness honestly.
    mature_bar = 2 if MATURING else n_val
    margin_flag = " -operator-margin 1" if MATURING else ""
    syb_bond = min_bond if MATURING else bond

    # -request-timeout 8s: a belt for the multi-region cert run (#286). The product now
    # extends a request's transport deadline in proportion to the OUTBOUND payload (a
    # validator's one-time ~1.5 MB bond-registration/genesis block gets WAN headroom
    # automatically), but a generous base leaves margin on a truly bad transcontinental
    # path. Uniform across all roles so a config mismatch can't perturb objective quorum
    # math on a fresh network. holder-fetch keeps its own tighter deadline (#277).
    common = f"-listen 0.0.0.0:{SWARM_PORT} -store {STORE} -mdns=false -log {LOG_LEVEL} -request-timeout 8s{LOOP_BUDGET}"

    def argv(name):
        n = nodes[name]
        role, ip = n["role"], n["ip"]
        if role == "validator":
            attesters = ",".join(nodes[v]["nodeid"] for v in validators if v != name)
            # #286 Layer 2 (docs/network-durability.md §8): at genesis there is no chain,
            # so a proposer cannot DISCOVER its attesters' addresses — and hub-and-spoke
            # bootstrap (one seed) never converges a full mesh because silt's routing table
            # holds bare NodeIDs. Configure every validator with the WHOLE validator set as
            # a static, never-evicted persistent-peer tier (Tendermint persistent_peers), so
            # proposer-initiated quorum can form on a fresh multi-region net.
            persistent = ",".join(f'{nodes[v]["nodeid"]}@{nodes[v]["ip"]}:{SWARM_PORT}'
                                  for v in validators if v != name)
            a = (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -validator -objective "
                 f"-min-bond {min_bond} -min-bond-floor {min_floor} -mature-validators {mature_bar}{margin_flag} "
                 f"-anchors {anchors} -attesters {attesters} -quorum {quorum} "
                 f"-persistent-peers {persistent} "
                 f"-bond {bond} -bond-audit 30s -capacity 5G")
            if name == boot:
                a += f" -serve-registry 0.0.0.0:{REGISTRY_PORT}"   # boot validator issues tokens
            else:
                a += f" -bootstrap {bootstrap}"
            return a
        if role == "storage":
            return f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} -capacity 5G"
        if role == "registry":
            return f"daemon -id-seed {n['seed']} -store {STORE} -log {LOG_LEVEL} -registry-only -serve-registry 0.0.0.0:{REGISTRY_PORT}"
        if role == "relay":
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-relay 0.0.0.0:{RELAY_PORT} -capacity 5G")
        if role == "fetcher":
            return f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} -capacity 2G"
        if role == "natted":
            # un-dialable (no -advertise): must reach the swarm THROUGH the relay
            return f"daemon -id-seed {n['seed']} {common} -bootstrap {bootstrap} -relay-via {relay_ref} -capacity 2G"
        if role == "adversary":
            # honest by default; #184 scenarios relaunch it with -equivocate/-forge-block/etc.
            # Same maturity/attester baseline as the honest validators so its only
            # difference is the injected red-team flag — otherwise the mismatched
            # config can perturb objective quorum math on a fresh network.
            adv_attesters = ",".join(nodes[v]["nodeid"] for v in validators)
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-validator -objective -min-bond {min_bond} -min-bond-floor {min_floor} "
                    f"-mature-validators {mature_bar}{margin_flag} -anchors {anchors} -attesters {adv_attesters} "
                    f"-quorum {quorum} -bond {bond} -bond-audit 30s -capacity 2G")
        if role == "maturer":
            # HONEST MATURER (the 2026-08-15 re-split): a non-anchor validator with
            # a FULL bond and NO -domain (unset = an independent address-diversity
            # group, chain.go "unset → independent"). This is the cohort the maturity
            # metric is designed to see — C2Metric excludes anchors, so only these
            # bonds can trip the bar-2 latch (min(NakamotoOperators, NakamotoDomains)
            # ≥ 2 needs two 64M distinct-domain non-anchor bonds committed). No core
            # change is needed for their attestations to count: drain proposers
            # gather from syncTargets() ∩ AttesterEligible (chainrole.go), so once a
            # maturer's bond commits, the anchors solicit it and it lands in
            # validatorsSeen. -attesters/-persistent-peers over the REAL validator
            # set: the static tier is the chain-sync + reg-submit path
            # (configure-not-discover, network-durability §8), same as the sybils.
            mat_attesters = ",".join(nodes[v]["nodeid"] for v in validators)
            mat_persistent = ",".join(f'{nodes[v]["nodeid"]}@{nodes[v]["ip"]}:{SWARM_PORT}'
                                      for v in validators)
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-validator -objective -min-bond {min_bond} -min-bond-floor {min_floor} "
                    f"-mature-validators {mature_bar}{margin_flag} -anchors {anchors} -attesters {mat_attesters} "
                    f"-persistent-peers {mat_persistent} "
                    f"-quorum {quorum} -bond {bond} -bond-audit 30s -capacity 2G")
        if role == "sybil":
            # C2 quiet-capture attempt: bonded, NON-anchor, all sharing ONE -domain
            # (one operator's satellite keys). References the REAL anchor set it does
            # NOT control and co-signs only other sybils. The C2 concentration metric
            # discounts a single-domain cohort, so it cannot reach the bond-distinct
            # maturity threshold to shed the launch anchors: without anchor
            # attestations the chain refuses to advance (ErrAnchorRequired). ≥8 equal
            # single-domain bonds also trip the atomization note.
            #
            # -persistent-peers over the REAL validator set (#338): a sybil's
            # -attesters are only other sybils — none of whom hold the committed
            # chain — so without a configured, dialable path to the validators the
            # cohort can neither sync the chain nor submit its bond registrations,
            # and the capture drill's precondition (banked committed Sybil standing)
            # is never met (the SYBILS=8 field GAP: "sybil-1 never synced a
            # committed chain (head 0)"). The static tier is a chain-sync target
            # (configure-not-discover, network-durability §8), so the sybils bank
            # standing through the anchors' drain blocks and the drill becomes
            # DRIVABLE. A real Sybil operator would configure exactly this — the
            # validator addresses are public; nothing here helps the attack.
            syb_attesters = ",".join(nodes[s]["nodeid"] for s in sybils if s != name)
            att = f" -attesters {syb_attesters}" if syb_attesters else ""
            syb_persistent = ",".join(f'{nodes[v]["nodeid"]}@{nodes[v]["ip"]}:{SWARM_PORT}'
                                      for v in validators)
            return (f"daemon -id-seed {n['seed']} {common} -advertise {ip}:{SWARM_PORT} -bootstrap {bootstrap} "
                    f"-validator -objective -min-bond {min_bond} -min-bond-floor {min_floor} "
                    f"-mature-validators {mature_bar}{margin_flag} -anchors {anchors}{att} "
                    f"-persistent-peers {syb_persistent} "
                    f"-quorum {quorum} -bond {syb_bond} -bond-audit 30s -capacity 2G -domain sybilnet")
        if role == "natgw":
            return "NATGW"   # not a silt node — runs integration/nat/natgw.sh instead
        sys.exit(f"topology: unknown role {role} for {name}")

    for name, n in nodes.items():
        n["argv"] = argv(name)

    meta = {
        "swarm_port": SWARM_PORT, "relay_port": RELAY_PORT, "registry_port": REGISTRY_PORT,
        "bond_mode": BOND_MODE, "n_val": n_val, "quorum": quorum, "bootstrap": bootstrap,
        "sybils": sybils, "n_syb": n_syb, "syb_quorum": syb_quorum, "syb_domain": "sybilnet",
        "maturers": maturers, "n_mat": n_mat,
        "maturing": MATURING, "mature_bar": mature_bar,
        "relay_ref": relay_ref, "regref": regref, "anchors": anchors, "boot": boot,
        "public_cidr": PUBLIC_CIDR, "nat_cidr": NAT_CIDR,
    }
    here = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(here, "topology.json"), "w") as f:
        json.dump({"meta": meta, "nodes": nodes}, f, indent=2)

    os.makedirs(os.path.join(here, "terraform"), exist_ok=True)
    tfvars = {
        "nodes": {name: {"role": n["role"], "ip": n["ip"], "zone": n["zone"],
                         "region": n["region"], "argv": n["argv"]}
                  for name, n in nodes.items()},
        "region_cidrs": region_cidrs,
        "public_cidr": PUBLIC_CIDR, "nat_cidr": NAT_CIDR,
        "swarm_port": SWARM_PORT, "relay_port": RELAY_PORT, "registry_port": REGISTRY_PORT,
    }
    with open(os.path.join(here, "terraform", "topology.auto.tfvars.json"), "w") as f:
        json.dump(tfvars, f, indent=2)

    sys.stderr.write(
        f"topology: wrote topology.json + terraform/topology.auto.tfvars.json "
        f"({len(nodes)} nodes, {n_val} validators, quorum {quorum}, bond={BOND_MODE})\n")


if __name__ == "__main__":
    main()
