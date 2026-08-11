# silt two-cloud field test (`twocloud`) — the real-world WAN test

Run the **same** 13-node field test with the topology **split across GCP and AWS**, so
silt's consensus, storage, and NAT traversal cross a **real inter-provider internet
boundary** — genuinely independent networks, ASNs, and routing, with real
transcontinental latency, jitter, and loss. This is the closest thing to production
conditions the harness can produce, and the ultimate exercise of build-immutable #5
("build for the adverse internet"). It is also the reason the AWS variant
(`integration/awstest/`) exists.

> **Status: SCAFFOLDING / DESIGN — not runnable yet.** It composes the GCP
> (`cloudtest`) and AWS (`awstest`) harnesses, so it is gated on **both** substrates
> certifying independently first, **and** on the one genuinely-new piece — cross-provider
> **public-IP** addressing (see §3) — being wired. This dir captures the architecture,
> the unified per-node exec router (the novel, done part), a split-topology generator,
> and the orchestrator skeleton with the IP-reservation phase marked TODO. It is a
> foundation to build on, not a green run.

## 1. Why this is different from either single cloud

`cloudtest` (GCP, one global VPC) and `awstest` (AWS, one regional VPC) each run the
whole topology **inside one provider's private network**, where cross-node IPs are
routable internally and can be baked static before apply. Two-cloud has **no shared
private network**: a GCP node dialing an AWS node must use that node's **public IP**
over the open internet. That single fact drives the whole design.

The payoff: it exercises what neither single cloud can — real BGP-path latency and loss
*between providers*, MTU/PMTUD differences, distinct NAT/egress behaviour on each side,
and consensus that must commit across the boundary. If #286 (the 1.5 MB genesis proof
vs a flat transport deadline) is the kind of thing that only shows up over a real WAN,
two-cloud is where the *next* one shows up first.

## 2. It is STILL the same test (the reuse contract holds)

- **`scenarios.sh` + `gen_report.sh` are reused verbatim** from `../cloudtest/` — again.
  The flows only ever call `ssh_node`/`jlog`/`dlog`/`slo_assert`/`record`.
- The **novel piece** is a unified `lib.sh` whose `ssh_node` reads each node's `cloud`
  field (`gcp` | `aws`) from `nodes.json` and **routes** to the right substrate — GCP
  IAP SSH or AWS SSM — transparently. `scenarios.sh` never knows a flow's two endpoints
  live on different clouds. This is the done, verifiable-by-inspection core (`lib.sh`).
- Each cloud's **Terraform is reused as-is**: `cloudtest/terraform` and
  `awstest/terraform` already take `var.nodes` as a *map*, so each cloud provisions only
  the **subset** assigned to it. `twocloud` runs both stacks with disjoint node maps.

## 3. The one hard new problem: cross-provider public-IP addressing

The `silt` argv (`-advertise`, `-bootstrap`, `-anchors`, `-attesters`, `-relay-via`,
the registry ref) must reference every dialable node by its **public** IP, because
peers on the other cloud can only reach it that way. Public IPs aren't known until
instances exist — the same chicken-and-egg multi-region AWS hit. Two options:

- **(A) Pre-reserved static IPs — the clean approach (design target).** A **phase 0**
  reserves the public addresses on both clouds *before* the instances — GCP
  `google_compute_address` (static external IPs), AWS `aws_eip` — reads them back, and
  only *then* does `topology.py` bake the argv against those known public IPs, and
  phase 1 applies the instances attaching the reserved IPs. Preserves the harness's
  zero-post-apply-reconfiguration property end-to-end.
- **(B) Two-phase render — the fallback.** Apply instances with ephemeral public IPs →
  read them back → re-render the argv → push it to each node (SSM / GCP metadata) and
  restart `silt`. Simpler Terraform, but a reconfiguration round-trip and a moment where
  the mesh isn't yet formed.

Natted nodes stay private on their own cloud (their cloud's NAT + the relay); the
**relay must have a public IP** so natted nodes on *either* cloud reach the swarm
through it.

## 4. The node split (default)

Put validators on **both** clouds so quorum genuinely crosses the provider boundary —
the whole point. A sensible default 13-node split:

| cloud | nodes |
|---|---|
| **GCP** | val-a (boot/registry), val-b, store-1, relay, fetch-1 |
| **AWS** | val-c, val-d, store-2, adversary, natgw, nat-1, nat-2 |

Boot/registry on GCP (val-a), the relay on GCP (public), the two natted nodes on AWS
(reaching the GCP relay over the internet — a real cross-provider NAT-traversal test).
`quorum = n_val - 2 = 2`, so a commit needs a proposer + 2 attesters that will usually
span both clouds. The split is a knob (`SPLIT=…`), not a constant.

## 5. Layout (planned)

```
twocloud/
  README.md              ← you are here
  twocloud.sh            ← orchestrator SKELETON: reserve IPs → topology → apply BOTH → run shared scenarios → destroy BOTH
  lib.sh                 ← the unified router: ssh_node/jlog/dlog dispatch by node["cloud"] (DONE)
  topology.py            ← split assignment + public-IP argv (reuses the GCP/AWS argv logic)
  config.env.example     ← both clouds' knobs (GCP project + AWS region + the split)
```

`scenarios.sh` and `gen_report.sh` are **not** copied — `twocloud.sh` sources them from
`../cloudtest/`. The per-cloud Terraform/provisioning are **not** copied — it invokes
`../cloudtest/terraform` and `../awstest/terraform` with node subsets.

## 6. What's DONE vs TODO in this scaffold

- **DONE (verifiable by inspection):** `lib.sh` — the per-node exec router; `topology.py`
  — the split assignment + a combined `nodes.json` shape carrying `cloud`; the
  `twocloud.sh` phase structure and the shared-scenario wiring.
- **TODO (needs both substrates + real creds):** the phase-0 public-IP **reservation**
  in each cloud's Terraform (small additive change to `cloudtest/terraform` +
  `awstest/terraform`), the argv rebind against reserved public IPs, and the first real
  cross-provider run to shake out SG/firewall rules for public swarm/relay/registry
  ports on both sides and the cross-provider NAT path.

## 7. Prereqs (when it's wired)
Everything `cloudtest` needs (gcloud, a GCP project) **and** everything `awstest` needs
(aws CLI + Session Manager plugin, an AWS profile) — plus a billing alarm on *both*.
Teardown destroys *both* stacks on exit; `nuke` cleans each cloud by its own label/tag.
