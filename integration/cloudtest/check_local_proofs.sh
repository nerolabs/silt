#!/usr/bin/env bash
# check_local_proofs.sh — every graded cloud flow must declare its LOCAL proof.
#
# The per-flow extension of the #490 per-run gate: a `flow_*`/`adv_*` function in
# scenarios.sh carries, on the comment line directly above its definition,
#   # LOCAL_PROOF: <command that proves the same property on a laptop>
# or an explicit
#   # LOCAL_PROOF: n/a — <why only a real WAN can test this>
# A new flow without one is refused at CI: the parity between the cloud sheet and
# the local tiers is a standing invariant, not a report (build-immutable #7;
# docs/thinking/2026-08-20-harness-local-first.md). The `n/a` set IS the owned
# cloud-only residue: `grep 'LOCAL_PROOF: n/a' scenarios.sh` lists it.
set -euo pipefail
cd "$(dirname "$0")"

fail=0
prev=""
while IFS= read -r line; do
  case "$line" in
    flow_*"() {"|adv_*"() {")
      fn="${line%%(*}"
      case "$prev" in
        "# LOCAL_PROOF: "?*) : ;;
        *) echo "✗ $fn has no '# LOCAL_PROOF:' line directly above its definition"; fail=1 ;;
      esac ;;
  esac
  prev="$line"
done < scenarios.sh

if [ "$fail" = 1 ]; then
  echo "every graded cloud flow needs a local proof (or an explicit 'n/a — <WAN-only reason>')"
  exit 1
fi
echo "✓ every flow_*/adv_* in scenarios.sh declares a LOCAL_PROOF (n/a residue: $(grep -c '# LOCAL_PROOF: n/a' scenarios.sh) flow(s))"
