# silt cloud field test — runbook

Drives the full field test on real GCP machines across three regions: build →
apply → run scenarios → report → destroy. Every `apply` brings up billable VMs, so
the spend discipline below is part of the harness, not advice around it.

Work from this directory.

## Ground rules

1. **Confirm before every `apply`.** Never run `./cloudtest.sh up` or `all` without
   an explicit go from whoever owns the project. Each brings up real VMs.
2. **Always tear down, and verify it.** The default lifecycle destroys on exit; you
   still check. After any run
   `gcloud compute instances list --filter labels.cloudtest:*` must be empty. If it
   is not, `./cloudtest.sh nuke`.
3. **Cheap first.** Validate with no spend, then a 4-node SMOKE run (pennies), then
   the full 13-node run only once SMOKE is green.
4. **Iterate without re-paying.** Bring the network up once with `KEEP_UP=1`, then
   re-run scenarios for free with `./cloudtest.sh run` while you fix log patterns or
   quorum sizing. Tear down when done.
5. **Nothing fails silently.** Every unmet SLO lands in `results.jsonl` and the
   report as `gap` or `fail`. A `gap` means "could not confirm", not "broken" —
   investigate it, never paper over it.

## Prerequisites (once, no spend)

- `gcloud auth login`, against a billing-enabled project.
- APIs enabled:
  `gcloud services enable compute.googleapis.com iap.googleapis.com storage.googleapis.com`
  (add `cloudbilling.googleapis.com` only for the budget alarm).
- The account holds `roles/compute.admin`, `roles/iap.tunnelResourceAccessor` and
  `roles/storage.admin` (project Owner covers all three).
- Local tools: `terraform`, `gcloud`, `go`, `python3`, `curl`.
- `cp config.env.example config.env`, then set `PROJECT_ID`. Leave the rest at
  defaults for a first run.

## Phase 1 — validate with no spend

Creates nothing in the cloud:

```bash
cd integration/cloudtest

# a) the deterministic topology generator (builds a throwaway local silt binary)
( cd ../.. && go build -o integration/cloudtest/.silt-local ./cmd/silt )
SILT_BIN="$PWD/.silt-local" SMOKE=1 python3 topology.py    # prints "4 nodes, 2 validators"
python3 -c "import json; t=json.load(open('topology.json')); print(t['nodes']['val-a']['argv'])"

# b) terraform validates the config (no apply)
terraform -chdir=terraform init -input=false
SILT_BIN="$PWD/.silt-local" SMOKE=1 python3 topology.py     # regenerate tfvars for validate
terraform -chdir=terraform validate
```

If `terraform validate` errors, fix the HCL before spending anything — usually a
provider field renamed across `google` provider majors, or `google_billing_budget`
needing its vars unset (it is guarded by `count`; `BUDGET_AMOUNT_USD=0` disables
it). If `topology.py` errors, the local silt build failed; fix that first.

## Phase 2 — the SMOKE run (~4 nodes, a few cents)

Validates the whole cloud path — apply, binary pull, systemd boot, IAP SSH,
publish → commit → fetch — at minimum cost. The NAT, adversary and fourth-validator
scenarios skip cleanly; they are not in the smoke topology.

```bash
SMOKE=1 KEEP_UP=1 ./cloudtest.sh up     # watch for: all nodes ready
./cloudtest.sh run                      # scenarios + report.md / report.html
```

For every `gap` or `fail`, use the debugging playbook, fix `scenarios.sh` (usually a
log pattern) or `topology.py` (quorum), then `./cloudtest.sh run` again. Changing
scenarios needs no re-apply; changing `topology.py` does — `./cloudtest.sh down`,
then `SMOKE=1 KEEP_UP=1 ./cloudtest.sh up`.

When SMOKE is green, tear down and confirm:

```bash
./cloudtest.sh down
gcloud compute instances list --project "$PROJECT_ID" --filter "labels.cloudtest:*"   # must be EMPTY
```

## Phase 3 — the full run (13 nodes, 3 regions)

Only after SMOKE is green, and only on an explicit go:

```bash
./cloudtest.sh                                       # build → apply → run → report → DESTROY
KEEP_UP=1 ./cloudtest.sh up && ./cloudtest.sh run    # or iterate; then ./cloudtest.sh down
```

The full run exercises multi-validator convergence, f=1 fault tolerance, restart
survival, per-hash takedown, cross-NAT via the relay, and the adversarial drills
(equivocation → slash, partition → heal, forged and low-bond proposals → reject).

## The two tuning points to expect

1. **Quorum versus Byzantine-quorum sizing** (`6-fault-tolerance`).
   `-byzantine-quorum` defaults ON for objective validators and can raise the
   effective commit threshold above the `-quorum` floor. The scenario records the
   *observed* behaviour as a `gap` rather than a false pass. If it gaps: read val-a's
   journal around the publish while val-d is down, see which threshold it actually
   needed, and pin `quorum` in `topology.py` (the `quorum = max(1, n_val - 2)` line)
   — or add a validator. Then re-apply.
2. **Log patterns** (`waitfor` in `scenarios.sh`). Each check greps the daemon's
   `-log info` output for a phrase. If the live build phrases it differently, the
   check gaps. SSH to the node, read the real line, update the pattern, run again.
   The patterns come from the e2e tests (`e2e/*.go`) — `chain: committed block N`,
   `slashed equivocator`, `adopted a competing fork`.

Neither needs re-architecting anything. They are phrasing and number tuning.

## Debugging playbook

Every node is reachable over IAP without an external IP:

```bash
# instance names/zones for this run:
terraform -chdir=terraform output -json nodes | python3 -m json.tool

# SSH to a node:
gcloud compute ssh silt-ft-val-a-<run> --zone us-central1-a --tunnel-through-iap

# on the node:
sudo systemctl status silt.service
sudo journalctl -u silt.service --no-pager -n 200      # the daemon's own log
sudo journalctl -t silt-startup --no-pager             # startup script: binary pull, unit write
cat /etc/systemd/system/silt.service                   # the exact argv it is running
```

| symptom | likely cause | fix |
|---|---|---|
| node never `active`, `silt-startup` shows curl 403 | VM cannot read the GCS bucket | confirm `storage.objectViewer` is bound (Terraform does this) and the service-account scope; re-apply |
| binary pull OK but silt exits | bad argv or flag mismatch against the live build | read `journalctl -u silt`; compare the unit's argv against `silt daemon -h`; fix `topology.py` |
| SSH hangs or permission denied | IAP not enabled, or role missing | enable `iap.googleapis.com`; grant `roles/iap.tunnelResourceAccessor`; the firewall rule allows 35.235.240.0/20 |
| `apply` fails on SPOT capacity or quota | region or zone out of preemptible capacity | change a zone in `topology.py`, or `MACHINE_TYPE`; re-apply |
| publish never returns a `silt:` link | validators have not earned standing yet, or the token issuer is cold | it retries for `PUBLISH_RETRY_S`; raise it, or check validator journals for standing |
| cross-NAT fails | natgw route/firewall, or the relay is unreachable | check the natgw instance's `journalctl -t natgw-startup`; confirm the relay node is up |

## A run is done when

- The SMOKE run is green: publish → commit → fetch, bit-perfect, over real machines.
- Every flow in the full run is `pass` or an understood `gap`, and `report.html` is
  generated.
- Teardown is confirmed empty for this run's label.
- Any `topology.py` or `scenarios.sh` fixes are on a branch and in a pull request.
  `main` is ruleset-protected; never push to it directly.

Run artifacts (`report.md`, `report.html`, `results.jsonl`, console and evidence
logs) are written here and are gitignored. They are evidence for the run that
produced them, not repository content — keep what you need outside the tree.
