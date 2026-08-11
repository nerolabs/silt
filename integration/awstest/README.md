# silt AWS field test (`awstest`) — the fallback substrate

A second **real-hardware** substrate for the silt field test, mirroring
`integration/cloudtest/` (GCP) on **AWS**. Its job is to run the **same** 13-node
field test when GCP is capacity- or quota-blocked — and, later, to be one half of a
**two-cloud** run (GCP ↔ AWS) that exercises real inter-provider WAN latency.

> **Status: FIRST CUT — dry-validated only, not yet run on real AWS.** The topology
> generator and all shell are syntax-checked and `terraform validate`-clean, but the
> Terraform/IAM/SSM path has **not** been shaken out against a live AWS account (there
> are no AWS credentials wired yet). Treat the first real run as a shakedown, exactly
> as `cloudtest/` started. The GCP harness is the certified one today.

## The one important property: it is the SAME test

The field-test **flows are substrate-agnostic** — `scenarios.sh` only ever calls
`ssh_node` / `jlog` / `dlog` / `slo_assert` / `record`. So this harness **reuses
`../cloudtest/scenarios.sh` and `../cloudtest/gen_report.sh` verbatim** (sourced, not
forked) and swaps only the substrate layer. If a flow drifts, it drifts for both
clouds at once — there is no second copy of the properties to rot.

What differs from GCP, and nothing else:

| Concern | GCP (`cloudtest`) | AWS (`awstest`) |
|---|---|---|
| Compute | `google_compute_instance` | `aws_instance` (EC2) |
| Keyless remote exec | `gcloud ssh --tunnel-through-iap` | `aws ssm start-session` (SSM) |
| Binary delivery | GCS bucket + VM access token | S3 bucket + instance-profile |
| Node metadata / argv | GCP instance metadata | EC2 **user-data** + IMDS |
| Network | one **global** VPC (cross-region internal IPs just work) | one **regional** VPC, multi-AZ (see below) |
| Cheap/ephemeral | SPOT + `max_run_duration`+DELETE | Spot + `shutdown -h +TTL` (terminate-on-shutdown) |
| Teardown by label | `labels.cloudtest=<run>` | tag `silt:awstest=<run>` |

## Why single-region multi-AZ (v1)

GCP's elegance is **zero post-apply reconfiguration**: every node's `silt` argv — with
peer/anchor/relay references — is baked from **static internal IPs chosen before
apply**, because a global VPC makes cross-region internal IPs routable. AWS has **no
global VPC**: each region is isolated, so multi-*region* would need VPC peering (or a
two-phase render over public IPs), and the argv could not be baked pre-apply.

So v1 keeps the GCP design intact by staying in **one region across 3 AZs** with
**pre-assigned private IPs** — the argv bakes cleanly, the run is fault-domain-spread,
and it is a faithful fallback for "GCP is blocked, run the full field test on AWS."
It does **not** reproduce cross-*region* (~60–150 ms) latency — that is (a) already
GCP-certified and (b) the job of the **two-cloud** test. Multi-region AWS (peering or
public-IP two-phase) is a tracked enhancement.

## Layout

```
awstest/
  README.md              ← you are here
  awstest.sh             ← orchestrator (mirrors cloudtest.sh); SOURCES ../cloudtest/scenarios.sh
  lib.sh                 ← AWS substrate: ssh_node (SSM), jlog/dlog, restore_argv (IMDS) + reused record/slo_assert
  topology.py            ← same node table + argv logic as GCP; AZ spread + pre-assigned private IPs
  config.env.example     ← AWS_REGION / PROFILE / cost guards
  provision/
    silt-startup.sh      ← EC2 user-data: pull binary from S3, write systemd unit, cold-start bootstrap-wait
  terraform/
    versions.tf variables.tf main.tf outputs.tf
```

`scenarios.sh` and `gen_report.sh` are **NOT** copied here — `awstest.sh` sources them
from `../cloudtest/`.

## Run it (once AWS creds are wired)

```sh
cd integration/awstest
./awstest.sh setup           # interactive: AWS profile/region → config.env
SMOKE=1 ./awstest.sh         # cheap ~4-node shakeout first (pennies)
./awstest.sh                 # full 13-node → report → DESTROY
./awstest.sh nuke            # last-resort teardown by tag, if state is ever lost
# after any run: aws ec2 describe-instances --filters Name=tag:silt:awstest,Values='*' must be empty
```

Prereqs: `aws` CLI (v2, with the **Session Manager plugin**), `terraform`, `go`,
`python3`, and an AWS profile with EC2/VPC/IAM/SSM/S3 permissions + a billing alarm.

## Cost safety (same discipline as GCP)

- `trap teardown EXIT` armed **before** apply — destroys on any exit.
- Every instance runs `shutdown -h +TTL_MINUTES` in user-data and is launched with
  `instance_initiated_shutdown_behavior = terminate`, so the TTL **terminates** it
  even if the orchestrator dies.
- Spot by default for non-core; on-demand core (validators/registry/store) via
  `ALL_ON_DEMAND=1` for a cert run (mirrors the GCP lesson: Spot is fatal to a cert).
- `nuke` deletes **every** resource tagged `silt:awstest=<run>` (instances, EIPs, the
  VPC/subnets/SGs/route-tables/IGW/NAT, the S3 bucket) and asserts a clean sweep.

## What the first real run must shake out
1. SSM reachability to every node (agent up, instance-profile has `AmazonSSMManagedInstanceCore`).
2. The binary pull from S3 via the instance profile.
3. The cold-start bootstrap ordering (same `SKIP_BOOTSTRAP_WAIT` hook as GCP).
4. Security-group rules for the swarm/relay/registry ports across AZs, and NAT egress for the natted nodes.
5. Teardown to **zero** residual (the `nuke` full-sweep assert).
