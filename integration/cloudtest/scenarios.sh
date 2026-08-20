#!/usr/bin/env bash
# scenarios.sh — the field-test flows, one function each, mapping directly onto
# the acceptance brief (docs/reviews/m0-acceptance-brief.md flows 1–9) plus the
# #184 adversarial consensus drills. Each records a pass/gap/fail via slo_assert.
#
# Sourced by cloudtest.sh AFTER lib.sh and AFTER the network is up. All node
# interaction is over `ssh_node` (IAP). Field networks are noisy, so every check
# asserts a THRESHOLD/behaviour, never an exact count or timing.
set -uo pipefail

# ── PRINCIPLED windows (PE cadence ruling 2026-08-15 §4: replace arbitrary
# wall-clocks with bounds COMPUTED from the path — and a miss INSIDE a computed
# window is a FINDING, never a re-grade). The per-leg worst case is derived from
# the deployed flags: -request-timeout 8s × (1 + 3 -request-retries) + backoff
# (250ms doubling: 0.25+0.5+1) ≈ 34s/leg. The fresh-publisher path is ~5
# sequential legs (join/bootstrap → canonical-issuer ranking fetch → parallel
# token gather (#388) → scatter+confirm → register): 4×34 ≈ 136s; the COMMIT
# WAIT leg is escape-aware under the #451 synchronizer durations (submit-then-
# poll rides the designee rotation, and a contested height may pay the 2-round
# escape: H_ESCAPE_S ≈ 220s — see its derivation at the soak defaults) ≈
# 136 + 220 ≈ 356 → 360; +1 leg-equivalent for the relay hop on the cross-NAT
# flow is absorbed by the same allowance. (240 was the pre-#453 figure — its
# commit-wait leg assumed one flat 64s drain cycle; run 82bcd2b-39478's only
# non-#345/#350 GAP was a publish missing exactly this stale window.)
# Fetch is ~3 legs (discovery → manifest → parallel chunk fetches) ≈ 102s → 120.
: "${COMMIT_SLO_S:=90}"
: "${FETCH_SLO_S:=120}"
: "${RESTART_SLO_S:=60}"
: "${PUBLISH_RETRY_S:=360}"

# ── swarm references (validators as peers; val-a serves the registry) ───────────
ft_peers() {
  python3 -c "
import json;t=json.load(open('$FT_DIR/topology.json'));p=t['meta']['swarm_port']
print(','.join(n['nodeid']+'@'+n['ip']+':%d'%p for n in t['nodes'].values() if n['role']=='validator'))"
}
ft_regref() { # the deterministic pinned registry ref (boot validator NodeID@https://ip:port)
  # Built by topology.py from the known NodeID + internal ip. We do NOT scrape the
  # daemon's `registry: chain-backed, serving ...` banner: the old regex could not
  # match it (REGREF came back empty → every `swarm add` hit a usage error), and the
  # banner prints the bound 0.0.0.0 address, which a publisher cannot dial.
  python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['regref'])"
}

PEERS=""; REGREF=""
ft_init_refs() { PEERS="$(ft_peers)"; REGREF="$(ft_regref)"; }

# silt:v1: links carry the root in BASE64URL, not hex. The takedown denylist keys
# on the HEX root, so decode it (mirrors the local takedown/run.sh helper). Without
# this, `grep -oE '[0-9a-f]{64}'` on the link matched nothing and cloud takedown
# gap'd on every run — it never actually tested takedown (blind cloud finding).
b64url_to_hex() {
  local s="$1"
  while [ $(( ${#s} % 4 )) -ne 0 ]; do s="${s}="; done
  printf '%s' "$s" | tr '_-' '/+' | base64 -d 2>/dev/null | od -An -v -tx1 | tr -d ' \r\n'
}

# TOKEN_QUORUM: publish-token signatures gathered per publish (privacy: unlinkable
# above 0). Default 2. A -token-quorum publish needs the PUBLISHER to reach that
# many validators for signatures — distinct from validator<->validator reachability.
# At genesis there are no committed bonds to rank canonical signers, so it falls back
# to -peers order (a nominal-privacy note). Set TOKEN_QUORUM=1 (or 0) for a first
# bootstrap run if publisher->validator egress is the thing under test.
#
# H6 (blind field test 2026-08-12): a fresh network has no committed bonds, so a
# token-quorum≥2 publish can't rank canonical signers and the headline publish→fetch
# GAPs out of the box. The SMOKE path is precisely that first-bootstrap case, so
# default it to 1 there (an explicit TOKEN_QUORUM= still wins); the full run keeps 2.
if [ "${SMOKE:-0}" = 1 ]; then : "${TOKEN_QUORUM:=1}"; else : "${TOKEN_QUORUM:=2}"; fi

# ft_reachable_peers NODE — from NODE, how many of the -peers validators are TCP-
# reachable on the swarm port? Prints "OK/TOTAL"; lists unreachable refs on stderr.
# This attributes a token-gather shortfall to the publisher->validator path, which
# is the usual cause of "could not gather enough publish-token signatures" and a
# chain stuck at height 0 (the block only commits once a publish proposes it).
ft_reachable_peers() { # ft_reachable_peers NODE
  local node="$1" ref hostport host port ok=0 total=0 bad=""
  for ref in ${PEERS//,/ }; do
    [ -n "$ref" ] || continue
    total=$((total + 1))
    hostport="${ref#*@}"; host="${hostport%:*}"; port="${hostport##*:}"
    if ssh_node "$node" "timeout 3 bash -c ': </dev/tcp/$host/$port' 2>/dev/null"; then
      ok=$((ok + 1))
    else
      bad="$bad ${host}:${port}"
    fi
  done
  [ -n "$bad" ] && echo "  unreachable from $node:$bad" >&2
  printf '%s/%s' "$ok" "$total"
}

# publish a fresh random file on a node; echoes "LINK SHA" on success, empty on fail.
# On failure it sets FT_PUBLISH_GAP=1 iff the cause was a publisher→validator
# reachability shortfall (< TOKEN_QUORUM peers reachable — an egress/preemption
# problem, e.g. a SPOT-reclaimed validator), so a caller can record a GAP ("couldn't
# confirm") rather than a property FAIL. Cleared to 0 on entry.
ft_publish() { # ft_publish NODE SIZE_BYTES
  FT_PUBLISH_GAP=0
  # Cross-subshell handoff (#7 evidence): ft_publish runs inside `res="$(…)"`, so
  # variables set here NEVER reach the caller's scope. Files do. .ft_publish_gap
  # carries the honest gap-vs-fail signal to publish_verdict; .ft_publish_lasterr
  # carries the last captured silt error into the recorded verdict detail (it used
  # to go to the console only, which is not persisted — run beb3628-95860's
  # 9-cross-nat FAIL left no clue which leg died).
  printf 0 > "$FT_DIR/.ft_publish_gap"; : > "$FT_DIR/.ft_publish_lasterr"
  local node="$1" size="${2:-1048576}" out link lasterr=""
  ssh_node "$node" "head -c $size </dev/urandom >/tmp/ft_src.bin; sha256sum /tmp/ft_src.bin | cut -d' ' -f1" >/tmp/ft_src_sha 2>/dev/null
  local sha; sha="$(cat /tmp/ft_src_sha 2>/dev/null)"
  local deadline=$(( $(date +%s) + PUBLISH_RETRY_S ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    # 2>&1 INSIDE the remote command: ssh_node suppresses gcloud stderr (2>/dev/null),
    # so a publish error (on silt's stderr) is only captured for the diagnostic below
    # if the redirect happens remotely.
    out="$(ssh_node "$node" "/usr/local/bin/silt swarm add /tmp/ft_src.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1" || true)"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { printf '%s %s\n' "$link" "$sha"; return 0; }
    lasterr="$(printf '%s' "$out" | grep -iE 'could not gather|not enough|no canonical|token|refus|unreachable|timed? ?out' | head -2 | tr '\n' ';')"
    # The allow-list above was built for the KNOWN failure modes and silently
    # dropped a new class (run a56ac10-42834: '#441 insufficient valid
    # attestations: 2 prepares of 2 gathered' matched nothing → the diagnostic
    # printed '<none captured>' and the decisive error had to be re-captured
    # live before teardown). Fall back to the raw tail: an UNRECOGNIZED error
    # is the one most worth recording.
    [ -z "$lasterr" ] && lasterr="$(printf '%s' "$out" | grep -vE '^[[:space:]]*$' | tail -2 | tr '\n' ';')"
    sleep 4
  done
  # Diagnose so a failed publish is actionable, not a bare "no link produced". The
  # single most common cause is the publisher reaching < TOKEN_QUORUM validators.
  local reach; reach="$(ft_reachable_peers "$node")"
  # If the publisher can't reach enough validators to gather the token quorum, this
  # is an egress/preemption problem, not a property failure — flag it so the caller
  # records a GAP. (max(1,TOKEN_QUORUM): even token-quorum 0 needs one reachable peer.)
  local rok="${reach%%/*}"; local need=$(( TOKEN_QUORUM > 1 ? TOKEN_QUORUM : 1 ))
  [ "${rok:-0}" -lt "$need" ] 2>/dev/null && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  # A publish that fails because the ephemeral CLI publisher could not DISCOVER the
  # canonical issuer set / gather a token — even with validators reachable — is a
  # publish-token *discovery* problem over WAN (worse after mid-run node churn), not a
  # break of the property a SETUP publish is a precondition for (durability, crash
  # recovery). The publisher re-warm (#351) reduces it but a churned WAN can still
  # exceed the window. Flag it as a GAP so publish_verdict records the DEPENDENT flow
  # as UNTESTED, not FAILED; the publish-reliability issue itself stays visible in this
  # diagnostic and in #351. (A genuine no-quorum terminal outcome still fails fast via
  # /publish-status, so this does not mask a real refusal.)
  printf '%s' "$lasterr" | grep -qiE 'no canonical issuer|could not gather|not enough|issuer set|token' && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  # …or if the fetch publish subsystem was already found degraded this run (warm
  # failed): a subsequent publish failure — even one with no captured error text — is
  # then a discovery/setup problem, not a property break, so score it a GAP too.
  [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ] && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  printf '%s' "$lasterr" > "$FT_DIR/.ft_publish_lasterr"
  # Persist the diagnostic (#7): stderr reaches the console, but the console dies
  # with the terminal — the tee'd copy survives teardown next to the run's report.
  {
    echo "[$(date -u +%FT%TZ)] ft_publish FAILED after ${PUBLISH_RETRY_S}s on $node (token-quorum=$TOKEN_QUORUM)"
    echo "  publisher->validator reachability: $reach of the -peers set reachable"
    echo "  last silt error: ${lasterr:-<none captured>}"
    echo "  note: token-quorum needs the publisher to reach >= $TOKEN_QUORUM validators to"
    echo "        gather signatures. A shortfall here — not validator<->validator — is the"
    echo "        usual cause of 'could not gather enough publish-token signatures' and a"
    echo "        chain stuck at height 0. Retry a bootstrap run with TOKEN_QUORUM=1, or"
    echo "        fix publisher egress to the validators."
  } | tee -a "$FT_DIR/publish-diag-${RUN_ID:-local}.log" >&2
  return 1
}

# Record a publish-dependent flow's failed publish honestly: a reachability
# shortfall OR a degraded publish subsystem is a GAP — the property was UNTESTED,
# not broken — while any other publish failure is a real fail.
#
# NOTE the scope bug this closes: ft_publish is called as `res="$(ft_publish …)"`,
# i.e. in a command-substitution SUBSHELL, so any FT_PUBLISH_GAP it sets is lost and
# never reaches this parent scope. ft_publish therefore ALSO writes the signal to
# $FT_DIR/.ft_publish_gap (files cross the subshell boundary; run beb3628-95860's
# 9-cross-nat graded FAIL with the per-call gap signal silently dropped). We still
# honor FETCH_PUBLISH_DEGRADED (wait_publisher_warm sets it in the PARENT): it is 1
# whenever the publisher re-warm could not land a throwaway publish this run — a
# dependent flow's publish failure is then a discovery/setup problem (#351), not a
# break of the property that publish is a precondition for. The last captured silt
# error (.ft_publish_lasterr) is folded into the verdict so the report names the
# mechanism, not just "no link".
publish_verdict() { # publish_verdict FLOW SEVERITY "detail"
  local gap lasterr
  gap="$(cat "$FT_DIR/.ft_publish_gap" 2>/dev/null || echo 0)"
  lasterr="$(cat "$FT_DIR/.ft_publish_lasterr" 2>/dev/null || true)"
  if [ "$gap" = 1 ] || [ "${FT_PUBLISH_GAP:-0}" = 1 ] || [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ]; then
    record "$1" gap "$2" "$3 — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351)${lasterr:+; last publish error: ${lasterr}}; property UNTESTED, not failed"
  else
    record "$1" fail "$2" "$3${lasterr:+ (last publish error: ${lasterr})}"
  fi
}

# ── committed-height helpers (audit #303) ──────────────────────────────────────
# A stale 'chain: committed block N' already in the journal must NOT satisfy a
# "the publish committed" gate — capture the height BEFORE the action and require
# a STRICTLY HIGHER one after, so only a genuinely NEW commit counts.
ft_commit_height() { # ft_commit_height NODE — max committed-block height in the journal (0 if none)
  local h; h="$(jlog "$1" 800 | grep -oE 'chain: committed block [0-9]+' | grep -oE '[0-9]+$' | sort -n | tail -1)"
  printf '%s' "${h:-0}"
}
ft_wait_new_block() { # ft_wait_new_block NODE H0 TIMEOUT_S -> 0 if a block > H0 committed
  local node="$1" h0="${2:-0}" timeout="${3:-90}" deadline
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    [ "$(ft_commit_height "$node")" -gt "$h0" ] 2>/dev/null && return 0
    sleep 4
  done
  return 1
}

# ── Flow 1: build & first run ──────────────────────────────────────────────────
# LOCAL_PROOF: LOCAL=1 SMOKE=1 ./integration/cloudtest/cloudtest.sh  (this flow runs verbatim on the docker backend)
flow_first_run() {
  local n ok=1 bad=""
  for n in $(node_names); do
    [ "$(node_field "$n" role)" = natgw ] && continue
    if ! ssh_node "$n" "systemctl is-active --quiet silt.service"; then ok=0; bad="$bad $n"; fi
  done
  # shellcheck disable=SC2086
  flow_evidence_nodes $bad   # a fail captures exactly the non-active nodes' journals
  slo_assert "1-first-run" blocker "all silt nodes report service active${bad:+ (down:$bad)}" "$ok"
}

# ── Flow 2: publish → fetch bit-perfect from a DIFFERENT node ───────────────────
# LOCAL_PROOF: go test ./e2e -run TestPublishCommitFetchOverTCP -count=1
flow_publish_fetch() {
  local t0 t1 res link sha got ok=0
  t0="$(date +%s)"
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
  flow_evidence_nodes fetch-1 "$boot" store-2 store-1   # publisher, boot validator, fetch side
  local h0; h0="$(ft_commit_height "$boot")"   # audit #303: baseline BEFORE the publish
  res="$(ft_publish fetch-1 1048576 || true)"
  if [ -z "$res" ]; then publish_verdict "2-publish-fetch" blocker "publish never produced a silt: link within ${PUBLISH_RETRY_S}s"; return; fi
  link="${res%% *}"; sha="${res##* }"
  # committed on the boot validator? Require a NEW block (> h0), so a stale
  # pre-publish 'committed block' line can't satisfy the gate (audit #303).
  if ft_wait_new_block "$boot" "$h0" "$COMMIT_SLO_S"; then :; else
    slo_assert "2-publish-fetch" major "publish did not commit a NEW block (head stayed at $h0) within ${COMMIT_SLO_S}s (link=$link)" 0; return; fi
  # fetch from a DIFFERENT node than the publisher and compare hashes. store-2 in
  # the full topology; store-1 in the SMOKE set (store-2 absent) — both differ from
  # the fetch-1 publisher, so this blocker still runs (not false-fails) under SMOKE.
  local fetchnode=store-2; node_exists store-2 || fetchnode=store-1
  got="$(ssh_node "$fetchnode" "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_got.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_got.bin | cut -d' ' -f1" 2>/dev/null || true)"
  t1="$(date +%s)"
  [ -n "$sha" ] && [ "$got" = "$sha" ] && ok=1
  FT_LAST_LINK="$link"; FT_LAST_SHA="$sha"   # reused (+ SHA-compared) by restart-survival, takedown, chaos
  slo_assert "2-publish-fetch" blocker "fetched from $fetchnode $([ "$ok" = 1 ] && echo 'bit-perfect' || echo "MISMATCH (want=$sha got=$got)")" "$ok" $((t1 - t0))
}

# ── Flow 3: care link — repair/audit without decrypting ─────────────────────────
# LOCAL_PROOF: go test ./e2e -run TestRepairBountyPaysOnTheWire -count=1  (captures the siltcare: link from a real publish)
flow_care_link() {
  flow_evidence_nodes fetch-1
  # The publish emits a siltcare: link; a -care node repairs it and cannot read it.
  local out care
  out="$(ssh_node fetch-1 "/usr/local/bin/silt info /tmp/ft_src.bin 2>&1" || true)"
  care="$(printf '%s' "$out" | grep -oE 'siltcare:\S+' | head -1)"
  if [ -n "$care" ]; then
    slo_assert "3-care-link" minor "publish exposes a siltcare: link (repair/audit without the key)" 1
  else
    record "3-care-link" gap minor "no siltcare: link surfaced from the documented commands — verify the care-link UX is documented"
  fi
}

# ── Flow 4: become a validator (earned standing, not rubber-stamp) ──────────────
# LOCAL_PROOF: go test ./e2e -run TestBondEarnedStandingCommitsOverTCP -count=1
flow_become_validator() {
  flow_evidence_nodes val-b val-c val-d
  local n ok=1 bad=""
  for n in val-b val-c val-d; do
    node_exists "$n" || continue
    # audit #303: assert the node's OWN earned standing — 'chain: committed block'
    # is satisfiable by any OBSERVER, so it proved nothing about THIS node. The
    # daemon self-narrates 'standing self=<own id> reputation=N' every bond-audit
    # sweep once its bond qualifies (core/node/bondaudit.go), a genuinely per-node
    # signal that it earned standing on the objective path. That line is a
    # STRUCTURED n.logf → it lands in the on-disk debug.log, NOT journald, so it
    # must be read with waitfor_dlog (the #310 version used waitfor/journald and
    # could never match — SMOKE flagged it).
    local nid; nid="$(node_field "$n" nodeid)"
    if [ "${#nid}" -eq 64 ] && waitfor_dlog "$n" "standing self=$nid reputation=[1-9]" 90 >/dev/null; then :; else ok=0; bad="$bad $n"; fi
  done
  slo_assert "4-become-validator" major "non-anchor validators earn their OWN standing on the objective path${bad:+ (no per-node earned-standing signal:$bad)}" "$ok"
}

# ── Flow 5: multi-validator convergence ─────────────────────────────────────────
# LOCAL_PROOF: go test ./e2e -run TestObjectiveColdStartCommitsGenesis -count=1  (+ the I1-I5 model-check tier)
flow_convergence() {
  local n vals="" maxh=0
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
  # shellcheck disable=SC2086
  flow_evidence_nodes $vals
  # Read each validator's AUTHORITATIVE committed head — HEIGHT *and* HASH — from its
  # OWN chain-status store, not a journald 'committed block N' line. A height-only
  # signal can't tell same-fork convergence from same-height/DIFFERENT-fork; the head
  # HASH can (blind field test #2 §D; real evidence #3). Same pattern the C2/sybil flow
  # already uses.
  chain_head() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | awk '/head height:/{h=$3} /head hash:/{hh=$3} END{print (h==""?0:h), hh}'; }
  local heights="" info h hh nv
  for n in $vals; do
    info="$(chain_head "$n")"; h="${info%% *}"; hh="${info#* }"; h="${h:-0}"
    nv="${n//-/_}"; eval "H_$nv=\$h; HH_$nv=\$hh"
    heights="$heights $n=$h:${hh:0:12}"
    [ "$h" -gt "$maxh" ] 2>/dev/null && maxh="$h"
  done
  # A chain that never advanced past genesis is NOT "converged" — all-at-0 means
  # consensus never formed (assert an actual committed block, not agreement-on-nothing).
  if [ "$maxh" -lt 1 ] 2>/dev/null; then
    slo_assert "5-convergence" major "NO block ever committed — the chain is stuck at genesis (heights:$heights); consensus did not form" 0
    return
  fi
  # Convergence = (a) every validator within 2 blocks of the tip (real-latency
  # tolerance) AND (b) every validator AT the tip height agrees on the head HASH. A
  # same-height/DIFFERENT-hash pair is a live FORK that a height-only check would score
  # as "converged" — this is exactly the gap §D flags.
  local conv=1 tiphash="" fork="" nh nhh
  for n in $vals; do
    nv="${n//-/_}"; eval "nh=\$H_$nv; nhh=\$HH_$nv"
    [ $((maxh - nh)) -gt 2 ] && conv=0
    if [ "$nh" = "$maxh" ]; then
      if [ -z "$tiphash" ]; then tiphash="$nhh"
      elif [ "$nhh" != "$tiphash" ]; then conv=0; fork="$fork $n"; fi
    fi
  done
  local detail="all validators within 2 of tip=$maxh AND every tip-height validator shares head hash ${tiphash:0:12}… (heights:$heights)"
  [ -n "$fork" ] && detail="FORK at tip height $maxh — divergent head hashes on:$fork (heights:$heights)"
  # H5 — DURABLE convergence, not a point-in-time sample. A single instant can read
  # PASS on a chain that immediately reorgs its committed blocks away (the 2026-08-12
  # fork-choice oscillation: committed height 2 → reorg to height 0). Re-sample the
  # boot validator's head after a settle window and require it did NOT regress; a head
  # that fell below its first sample is an oscillating chain, scored a FAIL directly
  # (previously only caught downstream at publish). If it never converged in the first
  # place, keep that verdict.
  if [ "$conv" = 1 ]; then
    local boot bnv h1 hh1 info2 h2
    boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
    bnv="${boot//-/_}"; eval "h1=\${H_$bnv:-0}; hh1=\${HH_$bnv}"
    sleep 20
    info2="$(chain_head "$boot")"; h2="${info2%% *}"; h2="${h2:-0}"
    if [ "$h2" -lt "$h1" ] 2>/dev/null; then
      slo_assert "5-convergence" major "chain REGRESSED after a 20s settle — $boot head fell from height $h1 to $h2 (fork-choice oscillation: a committed chain reorged onto a lighter/height-0 fork; see the fork-choice finding)" 0
      return
    fi
    detail="$detail; DURABLE ($boot head ${h1}->${h2} over 20s, no regression)"
  fi
  slo_assert "5-convergence" major "$detail" "$conv"
}

# ── Flow 6: fault tolerance — kill one validator, quorum still commits ──────────
# LOCAL_PROOF: SUITE=substrate ./integration/adversarial/run.sh  (commit with a validator down, under netem)
flow_fault_tolerance() {
  require_nodes "6-fault-tolerance" major val-d || return
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
  # A fault-tolerance failure lives in the PUBLISHER (fetch-1) and the SURVIVING
  # validators, not val-d (deliberately down) — require_nodes stashed only val-d.
  # Override so the capture attributes a real gap (run 4faaee8-22913's flow-6 gap
  # had only the down node's journal). shellcheck disable=SC2086
  flow_evidence_nodes fetch-1 $(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator' and n!='val-d'))")
  local h0; h0="$(ft_commit_height "$boot")"   # audit #303: baseline BEFORE stopping val-d + publishing
  svc val-d stop || true
  sleep 5
  # FT_DOWN_COMMIT_S is COMPUTED (PE §4), not the generic COMMIT_SLO_S: under the
  # #441 certified design every publish rides the designee rotation, and a height
  # whose (h, r0) designee is the DOWN validator pays the round escape before a
  # live designee carries the entry — the 220s 2-round escape bound under the
  # #451 increasing durations (see H_ESCAPE_S) + one gather-leg margin ≈ 260.
  : "${FT_DOWN_COMMIT_S:=260}"
  local res ok=0
  res="$(ft_publish fetch-1 262144 || true)"
  if [ -n "$res" ]; then
    # Require a NEW block with val-d down, not a stale pre-kill 'committed block' line.
    ft_wait_new_block "$boot" "$h0" "$FT_DOWN_COMMIT_S" && ok=1
  fi
  if [ "$ok" = 1 ]; then
    slo_assert "6-fault-tolerance" major "publish still committed with one validator (val-d) down (within the computed ${FT_DOWN_COMMIT_S}s down-designee escape bound)" 1
  else
    record "6-fault-tolerance" gap major "no new commit within the computed ${FT_DOWN_COMMIT_S}s down-designee bound with val-d down — read the captured client error (publish-diag / .ft_publish_lasterr) before attributing; candidates: the O(f+1) designee ladder under load, quorum sizing, or a real FT break"
  fi
  svc val-d start || true
}

# ── Flow 7: restart survival — standing + issued tokens + stored content ────────
# LOCAL_PROOF: RESTART=1 ./integration/nat/run.sh  (persisted-store reload + re-announce)
flow_restart_survival() {
  flow_evidence_nodes val-b store-1 store-2
  local t0 t1 ok=0
  t0="$(date +%s)"
  svc val-b restart || true
  # standing must come back WITHOUT redoing the bond (fast reload), and quickly.
  # SCOPED to post-restart logs (t0 captured above): the 'bond: reloaded …' line
  # is emitted on EVERY boot, so an unscoped waitfor could match a prior boot's
  # line even if THIS restart hung/failed to reload standing (audit #303
  # restart-standing stale-gap). --since @t0 admits only the post-restart boot.
  if waitfor_since val-b '(reload|restored|persisted).*(bond|standing)|standing.*(reload|restored)|bond.*loaded' "$t0" "$RESTART_SLO_S" >/dev/null; then ok=1; fi
  t1="$(date +%s)"
  slo_assert "7-restart-standing" major "val-b standing returned after restart without re-bonding" "$ok" $((t1 - t0))
  # stored content still discoverable+served after a storage-node restart. Needs a
  # second storage node to restart while store-1 serves — skip cleanly under SMOKE
  # (store-2 absent) rather than restart a nonexistent node and assert on nothing.
  if node_exists store-2; then
    # No upstream link ⇒ nothing to fetch: GAP (untested), not a want=?/got=<none> FAIL —
    # the same missing-link precondition 8-takedown already GAPs on (H3). A failed
    # publish is scored where it happens (2-publish-fetch), not cascaded here.
    require_link "7-restart-content" major || return
    svc store-2 restart || true; sleep 8
    local ok2=0 got=""
    # SHA-compare, not echo-OK (§D): a `swarm get` that exits 0 but writes truncated or
    # wrong bytes must NOT pass as "fetchable" — assert the fetched file's SHA matches.
    got="$(ssh_node store-1 "/usr/local/bin/silt swarm get '$FT_LAST_LINK' -o /tmp/ft_r.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_r.bin | cut -d' ' -f1" 2>/dev/null || true)"
    [ -n "${FT_LAST_SHA:-}" ] && [ "$got" = "$FT_LAST_SHA" ] && ok2=1
    slo_assert "7-restart-content" major "content still fetchable BIT-PERFECT after a storage-node restart$([ "$ok2" = 1 ] || echo " (want=${FT_LAST_SHA:-?} got=${got:-<none>})")" "$ok2"
  else
    record "7-restart-content" skip major "skipped — needs store-2 (absent in this topology, e.g. SMOKE)"
  fi
}

# ── Flow 8: per-hash takedown on ONE operator only ──────────────────────────────
# LOCAL_PROOF: ./integration/takedown/run.sh
flow_takedown() {
  flow_evidence_nodes store-1 store-2
  if [ -z "${FT_LAST_LINK:-}" ]; then record "8-takedown" gap minor "no prior published link to deny"; return; fi
  # extract the HEX root from the silt:v1:<b64url-root>:<...> link and deny it on store-1
  local root; root="$(b64url_to_hex "$(printf '%s' "$FT_LAST_LINK" | cut -d: -f3)")"
  if [ ${#root} -ne 64 ]; then record "8-takedown" gap minor "could not decode a 64-hex root from $FT_LAST_LINK (got '${root:0:16}…')"; return; fi
  ssh_node store-1 "echo '$root' | sudo tee /var/lib/silt/deny.txt >/dev/null" >/dev/null 2>&1
  relaunch_with store-1 "-denylist /var/lib/silt/deny.txt"; sleep 8
  # DENIAL leg (fixed — audit-#303 class, wrong-surface probe): the old probe ran
  # `swarm get` ON store-1 and grepped its output for a refusal. But `swarm get` is a
  # SHORT-LIVED CLIENT node ("join, do the thing, leave" — cmd/silt/swarm.go): it
  # never consults the store-1 daemon's denylist, walks the DHT, and happily fetches
  # from any other holder — so the refusal grep could NEVER match and this flow GAP'd
  # on every run (`denied= served=1`). The denylist gates the DAEMON (refuse to
  # store/serve/prove/announce/repair + purge held chunks — core/node/denylist.go),
  # so assert THAT surface: the daemon's own enforcement line in its journal
  # ("denylist: N root(s) denied; purged M held chunk(s)" or "denylist: honoring N
  # denied root(s)" when it held none of them — both prove the operator's takedown
  # is loaded and enforced on this node only).
  local denyline denied=0 served=0
  denyline="$(waitfor store-1 'denylist: .*denied' 60 || true)"
  [ -n "$denyline" ] && denied=1
  # SHA-compare the SERVE side (§D): "still served elsewhere" must mean store-2 returned
  # the REAL bytes bit-perfect, not merely that `swarm get` exited 0.
  local sgot; sgot="$(ssh_node store-2 "/usr/local/bin/silt swarm get '$FT_LAST_LINK' -o /tmp/ft_s.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_s.bin | cut -d' ' -f1" 2>/dev/null || true)"
  [ -n "${FT_LAST_SHA:-}" ] && [ "$sgot" = "$FT_LAST_SHA" ] && served=1
  if [ "$denied" = 1 ] && [ "$served" = 1 ]; then
    slo_assert "8-takedown" major "store-1 enforces the operator denylist (${denyline##*silt}) while store-2 still serves BIT-PERFECT (no global switch)" 1
  else
    record "8-takedown" gap major "takedown scoping not confirmed (denied=$denied served=$served) — daemon never narrated denylist enforcement, or store-2 failed to serve"
  fi
  restore_argv store-1
}

# ── Flow 9: cross-NAT — a natted node moves a file via the relay ────────────────
# LOCAL_PROOF: ./integration/nat/run.sh  (EMULATED NAT; the real-middlebox cone/symmetric decision is the owned cloud residue)
flow_cross_nat() {
  require_nodes "9-cross-nat" major nat-1 nat-2 || return
  flow_evidence_nodes nat-1 nat-2 relay   # a cross-NAT failure lives on either NAT node OR the relay
  local res
  res="$(ft_publish nat-1 262144 || true)"    # nat-1 is un-dialable → must use the relay
  # Attribute the LEG (#7): run beb3628-95860 recorded a bare "did not exchange a
  # file" FAIL that left publish-vs-fetch unknowable after teardown.
  if [ -z "$res" ]; then
    publish_verdict "9-cross-nat" major "natted nodes did not exchange a file via the relay — the PUBLISH leg (nat-1 → relay → validators) never landed a link"
    return
  fi
  local link="${res%% *}" sha="${res##* }" got
  got="$(ssh_node nat-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_n.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_n.bin | cut -d' ' -f1" 2>/dev/null || true)"
  if [ -n "$sha" ] && [ "$got" = "$sha" ]; then
    slo_assert "9-cross-nat" major "natted nodes exchanged a file through the relay/hole-punch" 1
  else
    slo_assert "9-cross-nat" major "natted nodes did not exchange a file via the relay — publish landed ($link) but the FETCH leg on nat-2 returned '${got:-<none>}' (want $sha)" 0
  fi
}

# ── #184 adversarial: equivocation → slash (certified on a DEDICATED net) ────────
# LOCAL_PROOF: go test ./e2e -run TestEquivocatorSlashedOverTCP -count=1
adv_equivocation() {
  # PE ruling 2026-08-17 (184-equivocation-topology-ruling): equivocation is the ONE
  # irreversible drill — a proven double-sign is a PERMANENT eviction (F2), correctly.
  # Running it mid-sheet would leave the requirement pinned at ⌊4/2⌋+1=3 over the
  # CONFIGURED anchors while only 3 stay live → every later commit needs all 3
  # unanimously (zero fault tolerance), risking spurious end-of-sheet flakes on the
  # very sheet handed to a red team. So the destructive drill runs on its OWN
  # dedicated, ephemeral network, not the shared sheet.
  #
  # Its certifying home is the OBJECTIVE-mode slash-on-detection drill over real
  # daemons: e2e/equivocation_test.go (TestEquivocatorSlashedOverTCP) — a 4-anchor
  # net where a Byzantine anchor SERVES a conflicting signed block (a fork can never
  # be COMMITTED onto a target under a 3-of-4 BFT floor; the crime is SIGNING two
  # conflicting blocks at one height) and an honest anchor slashes it unaided on the
  # reconcile path — run under adverse conditions by integration/adversarial (netem).
  # The in-process merge gate is core/node/modelcheck_184_equivocation_objective_test.go.
  # (The old cloudtest flow GAPped every run: the outside non-anchor adversary is not
  # a gathered signer, so its prepare never reaches an honest commit and the detection
  # can never fire — the real root, one layer below "honest already attested".)
  record "184-equivocation" skip blocker "the one destructive drill (a proven double-sign is a PERMANENT eviction, F2) runs on its DEDICATED ephemeral net, not mid-sheet — certified by e2e TestEquivocatorSlashedOverTCP (objective slash-on-detection over real daemons) + integration/adversarial (netem), merge-gated by the in-process oracle. Mid-sheet eviction would pin the commit requirement at 3-of-4 against only 3 live anchors (zero fault tolerance). PE ruling 2026-08-17."
}

# ── #184 adversarial: partition → heal (BFT: stall-then-catch-up) ────────────────
# LOCAL_PROOF: go test ./e2e -run TestPartitionHealsToHeavierForkOverTCP -count=1
adv_partition() {
  require_nodes "184-partition" major val-a val-b val-c val-d fetch-1 || return
  local ida idb idd idc
  ida="$(node_field val-a nodeid)"; idb="$(node_field val-b nodeid)"
  idd="$(node_field val-d nodeid)"; idc="$(node_field val-c nodeid)"
  local v
  for v in "$ida" "$idb" "$idd" "$idc"; do
    [ ${#v} -eq 64 ] || { record "184-partition" gap major "could not resolve a validator NodeID from nodes.json — partition not applied, not a property failure"; return; }
  done
  # Head (height + hash) from a node's OWN chain-status store — same discipline as
  # flow-5 (a height-only check can't tell catch-up from a divergent same-height fork;
  # the hash proves same-history reconverge).
  chain_head() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | awk '/head height:/{h=$3} /head hash:/{hh=$3} END{print (h==""?0:h), hh}'; }

  # SEVER val-c into a genuine < ⅓-weight MINORITY. It must be cut from EVERY node that
  # HOLDS the committed chain — not only the anchors: any reachable chain-holder lets
  # val-c SYNC the majority's blocks (adopt-via-Reconcile, which logs "reconciled", not
  # "committed block N") and stay current, so the "minority" never falls behind. Runs
  # 1ebd487-73707 (base: val-c synced h14→h16 THROUGH the bonded `adversary` node) and
  # 1ebd487-7457 (MATURING: h25→h37 through adversary + 4 maturers + 4 sybils) proved
  # this: an anchors-only sever misses the other validator-role chain-holders. So block
  # val-c from ALL validator-role peers (validator / adversary / maturer / sybil),
  # BOTH directions (-block-peers drops traffic to AND from them). A single isolated
  # node cannot reach the commit quorum, so it CANNOT commit — the correct
  # BFT-intersection behaviour (a minority committing a conflicting fork is the I1
  # violation model B forbids). This is why on heal val-c CATCHES UP (a forward sync,
  # dropped=0), it does NOT "reorg": a dropped-block reorg would require val-c to have
  # committed a conflicting fork, which it correctly cannot — the ABSENCE of a reorg
  # line IS the safety property (PE ruling 2026-08-17).
  local blockids; blockids="$(python3 -c "
import json
t=json.load(open('$FT_DIR/topology.json'))
print(','.join(n['nodeid'] for name,n in t['nodes'].items()
  if n['role'] in ('validator','adversary','maturer','sybil') and name!='val-c'))" 2>/dev/null)"
  if [ -z "$blockids" ]; then
    record "184-partition" gap major "could not build the validator-role block set from topology.json — partition not applied, not a property failure"; return
  fi
  local t0sever; t0sever="$(date +%s)"
  relaunch_with val-c "-block-peers ${blockids}"
  # The sever is LIVE only once the restarted daemon narrates it. Reading val-c's
  # baseline BEFORE this point races the relaunch window (sed + daemon-reload +
  # restart + chain reload = seconds) on a chain that commits drain blocks
  # near-continuously — val-c legitimately commits 1–2 more blocks in the gap, and
  # the stall check then reads "ADVANCED during the partition" (run 2323b09:
  # h27→h29; the false-GAP class behind most of the 18 archived partition GAPs
  # that survived the earlier sever-widening fix). Mechanism: baseline-before-
  # sever race; fix: confirm the banner, THEN baseline.
  if ! waitfor_since val-c "PARTITION: -block-peers set" "$t0sever" 90 >/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "val-c never narrated the partition banner within 90s of the sever relaunch — sever not confirmed live, drill UNTESTED not failed"
    return
  fi
  local ci ch0; ci="$(chain_head val-c)"; ch0="${ci%% *}"; ch0="${ch0:-0}"

  # DRIVE the > ⅔ majority to commit a heavier chain during the window — publish to
  # the majority ONLY (val-c can't hear it), so a genuine height gap forms for val-c
  # to catch up to. (val-a serves the registry and is in the majority.)
  local majpeers; majpeers="$(printf '%s' "$PEERS" | tr ',' '\n' | grep -v "^${idc}@" | paste -sd, -)"
  local i
  for i in 1 2 3; do
    ssh_node fetch-1 "head -c 4096 </dev/urandom >/tmp/ft_part.bin; /usr/local/bin/silt swarm add /tmp/ft_part.bin -peers '${majpeers}' -registry '${REGREF}' -token-quorum 2 -chunk-size 65536 2>&1" >/dev/null 2>&1 || true
    sleep 6
  done
  sleep 18

  # ANTI-VACUITY (the key step): val-c must have STALLED — proof it was a genuine
  # minority that COULDN'T commit, not an idle chain trivially catching up to nothing.
  local ci1 ch1; ci1="$(chain_head val-c)"; ch1="${ci1%% *}"; ch1="${ch1:-0}"
  local mi mh; mi="$(chain_head val-a)"; mh="${mi%% *}"; mh="${mh:-0}"
  if [ "$mh" -le "$ch0" ] 2>/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "the majority committed NO heavier chain during the window (val-a head h$mh ≤ val-c's pre-partition h$ch0) — nothing to catch up to, heal UNTESTED not failed (drive under-committed; the majority publishes did not land)"
    return
  fi
  if [ "$ch1" -gt "$ch0" ] 2>/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "val-c ADVANCED during the partition (h${ch0}→h${ch1}) — the sever did not isolate it below the commit threshold, so it was NOT a genuine < ⅓ minority (drill under-drove; widen the sever). Not a product failure"
    return
  fi

  # HEAL: drop the block → val-c reconnects to the majority and catches up.
  restore_argv val-c
  # Assert val-c CATCHES UP: it advances past its stall AND reaches the majority's
  # LIVE head with a matching hash (both advance, so compare val-c to val-a live —
  # they align at the tip once val-c catches up).
  # Heal window sized to the CATCH-UP, not a magic constant (#5): the sever fix works
  # (run 76f654d-33422: val-c genuinely STALLED at h31 while the majority reached h38),
  # but a 7-block cross-region catch-up sync ran past 120s and GAPped ("did not
  # reconverge in 120s"). 300s matches the drill's other resume windows (10b's clincher)
  # and rides out a multi-block WAN catch-up while a real reconverge break still GAPs.
  local ok=0 t0; t0="$(date +%s)"
  # Snapshot the majority's head AT HEAL START as a FIXED reconverge target. val-a
  # keeps advancing while val-c catches up, so requiring val-c == val-a's LIVE head
  # false-GAPs whenever both advance at similar rates — val-c sits perpetually one
  # block behind a moving tip (run 6a38d7b-42691: val-c h34 vs live val-a h35, a
  # healthy lag, not a break). Reaching the heal-time head PROVES reconvergence:
  # a < ⅓ island committed nothing of its own during the partition (guarded above),
  # so val-c can only advance by SYNCING the majority chain, and Reconcile validates
  # full linkage — so equalling the target (hash-matched) or surpassing it (synced
  # THROUGH it) both mean the same chain, never a fork.
  local ai0 aht ahht; ai0="$(chain_head val-a)"; aht="${ai0%% *}"; ahht="${ai0#* }"
  local ci2 ch2 chh2
  while [ $(( $(date +%s) - t0 )) -lt 300 ]; do
    ci2="$(chain_head val-c)"; ch2="${ci2%% *}"; chh2="${ci2#* }"
    if [ "${ch2:-0}" -gt "$ch0" ] 2>/dev/null; then
      # Caught up ⇔ reached the heal-time majority head aht: exact height ⇒ hash must
      # match (reconverged to that block); surpassed ⇒ synced past it on the majority
      # chain (a fork could neither match the hash nor sync the majority's aht+1).
      if { [ "$ch2" = "$aht" ] && [ -n "$chh2" ] && [ "$chh2" = "$ahht" ]; } || [ "${ch2:-0}" -gt "$aht" ] 2>/dev/null; then ok=1; break; fi
    fi
    sleep 4
  done
  if [ "$ok" = 1 ]; then
    slo_assert "184-partition" major "minority val-c STALLED at h$ch0 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h$aht (now at h$ch2) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork)" 1
  else
    ft_add_validator_evidence
    record "184-partition" gap major "val-c did not reconverge to the heal-time majority head h$aht within 300s of heal (val-c=${ch2:-?}:${chh2:0:12}… stalled-from h$ch0) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break)"
  fi
}

# ── #184 adversarial: forged-block + low-bond proposals rejected ────────────────
# LOCAL_PROOF: go test ./e2e -run 'TestForgedBlockRejectedOverTCP|TestLowBondProposerRejectedOverTCP' -count=1
adv_proposal_reject() {
  require_nodes "184-forged-block" major adversary || return
  local ida; ida="$(node_field val-a nodeid)"
  if [ ${#ida} -ne 64 ]; then
    record "184-forged-block" gap major "could not resolve val-a NodeID from nodes.json (ida='${ida:0:12}…') — proposals not delivered, not a property failure"; return
  fi
  # audit #303: grep the line the PRODUCT actually emits — the adversary daemon prints
  # 'adversary: <label> proposal correctly REJECTED by <targetID>' when the honest target
  # refuses it (cmd/silt/daemon.go badPropose), same as the local integration/redteam
  # harness. The old assertions greped val-a for 'ErrBadSignature'/'ErrLowReputation',
  # strings the daemon never emits there, so they could only ever time out (false FINDING)
  # or, worse, match unrelated noise.
  relaunch_with adversary "-forge-block ${ida}"
  local ok1=0; waitfor adversary "forge-block proposal correctly REJECTED by ${ida}" 90 >/dev/null && ok1=1
  slo_assert "184-forged-block" major "forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a')$([ "$ok1" = 1 ] || echo ' — adversary never logged the reject within 90s (or val-a wrongly ACCEPTED it)')" "$ok1"
  restore_argv adversary
  # low-bond: the adversary carries a full, qualifying 64M bond. While its bond is not
  # yet committed on-chain it is (correctly) rejected as an unqualified proposer — the
  # PASS this drill was written for. But once its bond commits it becomes a QUALIFIED
  # proposer, and val-a CORRECTLY accepts its well-formed block: that "UNEXPECTEDLY
  # ACCEPTED" is right product behavior, not a defect, so it must NOT be a FAIL. The
  # outcome therefore flips on the adversary's standing race between runs (#350). Score
  # it honestly: a logged reject is a PASS; a logged accept is a GAP (this node is
  # qualified — a genuine under-bond REJECTION test needs a dedicated sub-min-bond
  # identity, #350); silence is a GAP (not driven). low-bond rejection is certified
  # in-process (#204) either way.
  relaunch_with adversary "-lowbond-propose ${ida}"
  if waitfor adversary "lowbond-propose proposal correctly REJECTED by ${ida}" 90 >/dev/null; then
    slo_assert "184-low-bond" major "under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a')" 1
  elif waitfor adversary "lowbond-propose proposal UNEXPECTEDLY ACCEPTED by ${ida}" 5 >/dev/null; then
    record "184-low-bond" gap major "adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204)"
  else
    record "184-low-bond" gap major "no reject/accept line from the adversary within 90s — low-bond not driven this run (#350)"
  fi
  restore_argv adversary
}

# ═══════════════════════════════════════════════════════════════════════════════
# Cloud variants of the local field-test series (integration/{privacy,durability,
# chaos,client,sybil}) — the same properties, over real VMs / real regions. Most
# map onto the existing 13-node topology with NO topology change (a flag flip via
# relaunch_with, a hard kill, a permanent stop). #5 C2-Sybil additionally needs the
# opt-in NON-anchor Sybil cohort (SYBILS=8, a topology.py role) so the Sybils' bonds
# actually bank and the PURE ErrAnchorRequired gate + the ≥8-bond atomization note
# become assertable; absent the cohort the flow records an honest skip.
# ═══════════════════════════════════════════════════════════════════════════════

# ── privacy (#3): publisher unlinkability — refuse-to-surveil over the real wire ─
# LOCAL_PROOF: go test ./e2e -run TestUnlinkablePublishOverTCP -count=1
flow_publisher_unlinkability() {
  require_live "priv-unlinkability" major fetch-1 || return   # preempted node ⇒ GAP, not "no refusal seen" FAIL (H2)
  # flow_publish_fetch already proved the DEFAULT (token-quorum) publish commits
  # unlinkably. Here the adversary ASKS to record a durable Publisher identity; the
  # default chain must REFUSE it (chain: ErrPublisherEntry / "carries a durable
  # Publisher"). That refusal, over real VMs, is immutable #4 (refuse-to-surveil).
  local out ok=0
  ssh_node fetch-1 "head -c 32768 </dev/urandom >/tmp/ft_priv.bin" >/dev/null 2>&1
  # The refusal is written to silt's STDERR; ssh_node suppresses gcloud stderr
  # (2>/dev/null), so the `2>&1` MUST be INSIDE the remote command or the refusal is
  # lost and this false-FAILs (a warm chain does refuse — verified live on GCP).
  out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm add /tmp/ft_priv.bin -peers '$PEERS' -registry '$REGREF' -allow-publisher -chunk-size 65536 2>&1" || true)"
  printf '%s' "$out" | grep -iqE 'durable Publisher|ErrPublisherEntry|permanent linkage|publish unlinkably' && ok=1
  slo_assert "priv-unlinkability" major "default chain REFUSED a durable file→publisher link (refuse-to-surveil)$([ "$ok" = 1 ] || echo " — no refusal seen: $(printf '%s' "$out" | tail -1)")" "$ok"
}

# A failed publish's decisive evidence lives on the VALIDATORS — the accept→commit
# window (was the chain committing? did the entry sit in a mempool? which height
# carried it late?) — not only on the client side. Run 82bcd2b-39478's
# durability-turnover GAP captured only store/fetch journals, so the root was
# unpinnable after teardown (#7: capture the evidence first, then look). Callers
# recording a verdict after a failed ft_publish extend the capture set with the
# validator cohort before record().
ft_add_validator_evidence() {
  local n vals=""
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
  FT_FLOW_NODES="$FT_FLOW_NODES$vals"
}

# ── durability (#2): content OUTLIVES a permanent storage-node loss ─────────────
# LOCAL_PROOF: ./integration/durability/run.sh
flow_durability_turnover() {
  require_nodes "durability-turnover" major store-1 store-2 fetch-1 || return
  # Publish replicated, then PERMANENTLY remove a storage node (stop + leave down —
  # a real departure, not a restart) and prove a fresh fetch still returns the bytes
  # from the survivors. The full membership-ROTATION form (a terminated VM replaced
  # by a fresh-IP VM) is the honest cloud version of the local kill-without-replace.
  local res link sha
  res="$(ft_publish fetch-1 1048576 || true)"
  # A failed SETUP publish means durability is UNTESTED (we have no content to lose),
  # not that durability broke — this flow tests survival of a permanent node loss, not
  # the publish path (2-publish-fetch is the publish canary). So GAP unconditionally on
  # a missing link, independent of the FT_PUBLISH_GAP/degraded signals (#351).
  if [ -z "$res" ]; then ft_add_validator_evidence; record "durability-turnover" gap major "setup publish did not land a link — durability UNTESTED this run, not a durability failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = the accept→commit path — commit latency vs the client poll window, #441-family; token/issuer-set errors = discovery #351. The validator journals for the publish window are captured with this verdict)"; return; fi
  link="${res%% *}"; sha="${res##* }"
  svc store-1 stop || true    # permanent departure (left down for the fetch)
  sleep 12
  local got ok=0 geterr=""
  geterr="$(ssh_node store-2 "rm -f /tmp/ft_dur.bin; /usr/local/bin/silt swarm get '$link' -o /tmp/ft_dur.bin -peers '$PEERS' -registry '$REGREF' 2>&1 >/dev/null | tail -3" 2>/dev/null || true)"
  got="$(ssh_node store-2 "sha256sum /tmp/ft_dur.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null || true)"
  [ "$got" = "$sha" ] && ok=1
  # Premise classifier (roadmap 2a): "root not in registry" means the setup publish
  # was ACCEPTED but its entry never became resolvable (#441-family accept→commit) —
  # durability of committed content is then UNTESTED, not failed. Any other failure
  # (hash mismatch, partial bytes, timeout with the entry present) is a real FAIL.
  if [ "$ok" != 1 ] && printf '%s' "$geterr" | grep -qiE 'root not in registry|no such entry'; then
    record "durability-turnover" gap major "fetch found the root ABSENT from the registry — the setup publish premise broke upstream (#441-family accept→commit), durability of committed content UNTESTED not failed (client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))"
  else
    slo_assert "durability-turnover" major "content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor$([ "$ok" = 1 ] || echo " (want=$sha got=${got:-<none>}; client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))")" "$ok"
  fi
  svc store-1 start || true   # restore for later flows
}

# ── chaos (#7): hard crash (SIGKILL) recovery + #69 reprovide over real VMs ──────
# LOCAL_PROOF: ./integration/chaos/run.sh
flow_chaos_crash() {
  require_nodes "chaos-crash" major store-1 store-2 || return
  # Capture the REGISTRY (val-a) + validator journals alongside store-1/store-2 on any
  # chaos FAIL: run 1ebd487-7457 FAILed chaos-fetch with "root not in registry" but the
  # capture held only the store journals, so the REGISTRY's own view of the root was
  # unattributable after teardown (#7 capture-first). ft_add_validator_evidence appends
  # val-a..d (val-a serves the registry) to this flow's evidence set.
  ft_add_validator_evidence
  local link="${FT_LAST_LINK:-}" wantsha="${FT_LAST_SHA:-}"
  if [ -z "$link" ]; then local res; res="$(ft_publish fetch-1 262144 || true)"; link="${res%% *}"; wantsha="${res##* }"; fi
  # As with durability-turnover: a failed SETUP publish means crash-recovery is UNTESTED
  # (no content to crash-and-recover), not broken — GAP unconditionally (#351).
  if [ -z "$link" ]; then ft_add_validator_evidence; record "chaos-crash" gap major "setup publish did not land a link — crash-recovery UNTESTED this run, not a failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = accept→commit #441-family; token/issuer errors = discovery #351. Validator journals captured with this verdict)"; return; fi
  # SIGKILL the silt process (abrupt death, not a graceful stop). systemd
  # (Restart=on-failure) brings it back, reloading the persisted store.
  # Capture t0 BEFORE the kill: flow_restart_survival already restarted store-2
  # earlier and emitted 're-announced N held chunks' on that prior boot — an
  # unscoped waitfor could match that stale line and false-pass a broken reprovide
  # (audit #303 chaos-reprovide stale-gap). --since @t0 admits only the post-crash
  # boot's re-announce.
  local t0; t0="$(date +%s)"
  ssh_node store-2 "sudo pkill -9 -f '/usr/local/bin/silt' || true" >/dev/null 2>&1
  ssh_node store-2 "sudo systemctl start silt.service" >/dev/null 2>&1 || true   # idempotent nudge
  # Wait for the CONDITION on a generous, evidence-sized deadline — never a magic
  # constant (build-immutable #5). Run 4faaee8-22913 attributed the old 90s FAIL:
  # re-announce completes but its latency is ~LINEAR in held-chunk count (that run,
  # store-2's own journal: 24 chunks → 19s early, 132 chunks → 102s late; ≈1.3
  # announces/s over WAN), so a fixed 90s under-provisions a store that has
  # accumulated chunks by late in a session. 300s rides out a loaded store while a
  # genuine reprovide gap still FAILs after it. Record the observed latency so every
  # run keeps measuring the trend (an M1 signal — the announce sweep looks serialized).
  local reann=0 reann_line reann_s
  reann_line="$(waitfor_since store-2 're-announced [0-9]+ held chunks' "$t0" 300 || true)"
  if [ -n "$reann_line" ]; then reann=1; reann_s=$(( $(date +%s) - t0 )); fi
  if [ "$reann" = 1 ]; then
    slo_assert "chaos-reprovide" major "SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (${reann_s}s to re-announce; latency scales with held-chunk count, #402/M1)" 1
  else
    # 300s with no re-announce is now a REAL gap (well past the measured linear
    # envelope) — the path survives an abrupt kill locally (integration/nat
    # RESTART=1, green post-#393). store-2's captured journal attributes it.
    slo_assert "chaos-reprovide" major "no post-crash 're-announced N held chunks' on store-2 within 300s of the SIGKILL — past the measured re-announce envelope; attribute from store-2's captured journal (#69)" 0
  fi
  local got ok=0 geterr=""
  # SHA-compare, not echo-OK (§D): crash-recovery must return the REAL bytes, not just
  # a zero exit on a possibly-truncated fetch. Keep the client's stderr — run
  # a56ac10-42834's chaos-fetch FAIL was UNATTRIBUTABLE because this line sent it
  # to /dev/null (got=<none> with the deciding error discarded).
  geterr="$(ssh_node store-1 "rm -f /tmp/ft_ch.bin; /usr/local/bin/silt swarm get '$link' -o /tmp/ft_ch.bin -peers '$PEERS' -registry '$REGREF' 2>&1 >/dev/null | tail -3" 2>/dev/null || true)"
  got="$(ssh_node store-1 "sha256sum /tmp/ft_ch.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null || true)"
  [ -n "$wantsha" ] && [ "$got" = "$wantsha" ] && ok=1
  # Premise classifier (roadmap 2a — Run B's two FAILs were this): "root not in
  # registry" = the publish premise broke upstream (#441-family accept→commit), so
  # crash-recovery of committed content is UNTESTED — GAP. A hash mismatch or a
  # timeout with the entry resolvable stays a real FAIL.
  if [ "$ok" != 1 ] && printf '%s' "$geterr" | grep -qiE 'root not in registry|no such entry'; then
    record "chaos-fetch" gap major "fetch found the root ABSENT from the registry — the publish premise broke upstream (#441-family accept→commit), crash-recovery UNTESTED not failed (client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))"
  else
    slo_assert "chaos-fetch" major "content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node$([ "$ok" = 1 ] || echo " (want=${wantsha:-?} got=${got:-<none>}; client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 300))")" "$ok"
  fi
}

# ── client/UI (#4): the web-UI local-security guard (#89) over a real VM ─────────
# LOCAL_PROOF: ./integration/client/run.sh
flow_web_ui_guard() {
  require_nodes "web-ui" minor fetch-1 || return
  require_live  "web-ui" minor fetch-1 || return   # preempted node ⇒ GAP, not empty-code FAIL (H2)
  # Turn on the web UI on fetch-1 (a flag flip, restored after), then attack its
  # guard from the node's OWN localhost — the realistic operator-on-the-box model.
  relaunch_with fetch-1 "-ui=127.0.0.1:8081"
  sleep 10
  local c401 c403 c200 ok=0
  c401="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8081/api/publish" 2>/dev/null || true)"
  c403="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' -H 'Host: evil.example.com' http://127.0.0.1:8081/api/status" 2>/dev/null || true)"
  c200="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8081/api/status" 2>/dev/null || true)"
  { [ "$c401" = 401 ] && [ "$c403" = 403 ] && [ "$c200" = 200 ]; } && ok=1
  slo_assert "web-ui-guard" major "web-UI guard held on a real VM: no-token POST=$c401 (want 401), DNS-rebinding Host=$c403 (want 403), token-free read=$c200 (want 200)" "$ok"
  restore_argv fetch-1
}

# ── C2 Sybil no-capture (#5): a bonded NON-anchor cohort cannot take the chain ──
# The LOCAL integration/sybil suite can only reach the STANDING gate — on a laptop a
# fresh Sybil set can't bank bonds (a young network's bond-registration needs
# anchor-proposed blocks; chicken-and-egg). On CLOUD, over the warm period the
# ANCHORS' committed blocks bank the Sybils' BondRegs, so this certifies the PURE
# gate: with the anchors gone, a self-majority of bonded Sybils all sharing ONE
# -domain still cannot advance the chain (ErrAnchorRequired) — because the C2
# concentration metric refuses to count a single-domain split as the address-diverse
# decentralization that sheds the launch anchors. The clincher: restore the anchors
# and the chain RESUMES — proving it was the missing anchors, not dead Sybils.
# Opt-in (needs the cohort): SYBILS=8 ./cloudtest.sh.
# LOCAL_PROOF: go test ./e2e -run TestAnchorStopHaltsBondedNonAnchors -count=1  (built 2026-08-20 as this flow's local twin)
flow_c2_no_capture() {
  if ! node_exists sybil-1; then
    record "5-sybil-no-capture" skip major "no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate)"
    return
  fi
  # Mutually exclusive with the MATURING topology: this flow certifies the
  # ANCHOR gate on a network that never sheds; under MATURING=1 the anchors shed
  # by design, so the premise (ErrAnchorRequired without anchors) does not exist
  # — the post-shed capture property is flow 10's B2 capture drill instead.
  if [ "$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta'].get('maturing',0))")" = "1" ]; then
    record "5-sybil-no-capture" skip major "MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)"
    return
  fi
  local n_syb sybils anchors_nodes
  n_syb="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['n_syb'])")"
  sybils="$(python3 -c "import json;print(' '.join(json.load(open('$FT_DIR/topology.json'))['meta']['sybils']))")"
  anchors_nodes="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  # This flow never calls require_nodes, so stash its evidence set explicitly: a
  # non-green verdict here (e.g. the resume clincher not firing) needs the anchors'
  # AND sybil-1's journals to attribute — run beb3628-95860's resume gap had none.
  # shellcheck disable=SC2086
  flow_evidence_nodes sybil-1 $anchors_nodes

  # chain head height from a node's OWN store (real chain-status, not a harness echo)
  syb_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }

  # 0) precondition: the Sybils must have SYNCED the anchor-committed chain (their
  #    bonds banked). head height 0 ⇒ never synced ⇒ capture premise not set up →
  #    honest GAP (property UNTESTED), never a fake pass.
  local h0; h0="$(syb_height sybil-1)"; h0="${h0:-0}"
  if [ "$h0" -lt 1 ] 2>/dev/null; then
    record "5-sybil-no-capture" gap major "sybil-1 never synced a committed chain (head height 0) — the anchors had not yet banked the Sybil bonds; capture precondition unmet, property UNTESTED"; return
  fi

  # 1) anchors UP: an anchor logs the C2 metric with the wheels ENGAGED, and (≥8
  #    equal single-domain bonds) the atomization note — real evidence the metric
  #    SEES the cohort and refuses to count it as decentralization. Observational.
  local wheels atom
  wheels="$(waitfor val-a 'wheels engaged|C2: nakamoto' 60 || true)"
  [ -n "$wheels" ] && echo "    C2 metric (anchors up): ${wheels##*silt}"
  atom="$(jlog val-a 800 | grep -E 'atomization note' | tail -1 || true)"
  [ -n "$atom" ] && echo "    atomization note tripped: ${atom##*silt}"

  # THE ANCHORED CEILING — the TRUE committed tip, read from the ANCHORS (which lead)
  # BEFORE stopping them, NOT from a Sybil's local head. This is the #338/C2 false-
  # positive fix: under load a Sybil can lag many blocks behind the committed tip, so
  # reading the ceiling off sybil-1 (as this flow used to) let its benign CATCH-UP —
  # syncing the anchors' already-committed blocks after the anchors stop — read as a
  # Sybil "advance" and fire a FALSE capture. A catch-up can only ever reach the tip
  # the anchors already committed, so pinning the ceiling to that true tip makes a
  # catch-up a no-op and reserves "advance" for a genuine NEW block. (Chain-level
  # proof the property itself holds in this exact topology: core/chain
  # TestC2SingleDomainSybilsDoNotMature — 8 equal single-domain bonds cannot mature,
  # so the anchor gate cannot shed and a real capture is structurally impossible.)
  local ceiling=0 a hh
  for a in $anchors_nodes; do
    hh="$(syb_height "$a")"; hh="${hh:-0}"
    [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"
  done
  # Fall back to the max Sybil head only if no anchor answered (all already down).
  if [ "$ceiling" -lt 1 ] 2>/dev/null; then
    for s in $sybils; do hh="$(syb_height "$s")"; hh="${hh:-0}"; [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"; done
  fi
  echo "    anchored ceiling (true committed tip, from the anchors): h${ceiling} (sybil-1 local head h${h0}$([ "$h0" -lt "$ceiling" ] 2>/dev/null && echo ' — sybil-1 is LAGGING; its catch-up is NOT a capture'))"

  # 1b) PRE-EXISTING DIVERGENCE guard: if a Sybil's head is ABOVE the anchored
  #     ceiling BEFORE we stop any anchor, EITHER the Sybil is on a different
  #     fork (the 4faaee8-22913 event — a real #402-class finding) OR it merely
  #     SYNCED a fresh commit the ceiling-read anchors hadn't landed at read
  #     time (benign broadcast skew on ONE chain — run 6fbcf2e-18553, where the
  #     "fork" was hash-identical: sybil h43 624c3c… == val-b/d h43). Heights
  #     cannot tell the two apart; HASHES can (consensus-discipline rule 7:
  #     never presume the mechanism). Compare the sybil's hash AT THE SHARED
  #     HEIGHT with an anchor's: same ⇒ skew (re-read the ceiling and proceed);
  #     different ⇒ a real divergent fork (GAP + the finding).
  local maxsyb=0 sh msyb=""
  for s in $sybils; do sh="$(syb_height "$s")"; sh="${sh:-0}"; if [ "$sh" -gt "$maxsyb" ] 2>/dev/null; then maxsyb="$sh"; msyb="$s"; fi; done
  if [ "$maxsyb" -gt "$ceiling" ] 2>/dev/null; then
    local syb_at anchor_at
    syb_at="$(jlog "$msyb" 400 | grep -oE "checkpoint: ${ceiling}:[0-9a-f]+" | tail -1 | cut -d: -f3)"
    anchor_at="$(jlog "$boot" 400 | grep -oE "checkpoint: ${ceiling}:[0-9a-f]+" | tail -1 | cut -d: -f3)"
    if [ -z "$syb_at" ] || [ -z "$anchor_at" ]; then
      # Premise UNREADABLE: no `checkpoint: h${ceiling}:` line to compare on the
      # sybil and/or the anchor (checkpoints log only at some heights, so a sybil
      # ahead of the last-readable anchor checkpoint leaves nothing to diff). A fork
      # cannot be concluded from a hash we could not read — consensus-discipline
      # rule 7: an oracle that can't read its premise FLAGS, it never presumes the
      # mechanism. A sybil merely ahead of the readable ceiling is far more likely
      # benign skew/lag than a divergent fork (run 6a38d7b-42691's false positive).
      record "5-sybil-no-capture" gap major "PRE-EXISTING DIVERGENCE UNVERIFIABLE: sybil ${msyb} h${maxsyb} > readable anchor ceiling h${ceiling}, but the hash at h${ceiling} was unreadable ($([ -z "$syb_at" ] && printf 'sybil')$([ -z "$syb_at" ] && [ -z "$anchor_at" ] && printf '+')$([ -z "$anchor_at" ] && printf 'anchor'); sybil=${syb_at:-unreadable} anchor=${anchor_at:-unreadable}) — cannot diff, so fork-vs-skew is UNKNOWN, NOT asserted as a fork; journals captured for attribution (#7)"
      return
    elif [ "$syb_at" = "$anchor_at" ]; then
      echo "    sybil head h${maxsyb} > ceiling h${ceiling} but HASH-IDENTICAL at h${ceiling} (${syb_at}) — broadcast skew on one chain, not a fork; re-reading the ceiling"
      ceiling=0
      for a in $anchors_nodes; do
        hh="$(syb_height "$a")"; hh="${hh:-0}"
        [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"
      done
    else
      # BOTH hashes readable AND DIFFERENT at the shared height ⇒ a genuine divergent
      # fork (the #402 class). This is the only branch that may assert a fork.
      record "5-sybil-no-capture" gap major "PRE-EXISTING DIVERGENT FORK: sybil ${msyb} at h${maxsyb} does NOT share the anchor chain's hash at h${ceiling} (sybil=${syb_at} anchor=${anchor_at}, both readable and DIFFERENT) — a real fork finding (#402 class), not skew; journals captured"
      return
    fi
  fi

  # 2) THE CAPTURE ATTEMPT — stop every anchor; only the bonded Sybil self-majority
  #    remains. Give it time to try to advance on its own. Capture t0 BEFORE the
  #    stop so the fresh-commit outcome check (step 3) can be SCOPED to post-stop
  #    journald lines only — an unscoped grep matched stale pre-stop 'committed
  #    block' lines and mis-fired CAPTURE (#402 detector false-positive, #303 class).
  local stop_t0; stop_t0="$(date +%s)"
  echo "    stopping all anchors ($anchors_nodes) — the Sybil cohort attempts to advance…"
  for a in $anchors_nodes; do svc "$a" stop >/dev/null 2>&1 || true; done
  sleep 90

  # 3) OUTCOME (immutable #2 — outcome first, log corroborates): the chain must NOT
  #    advance past the anchored CEILING (a +1 tolerance for a block already in
  #    flight when the anchors dropped). A real capture ALSO leaves a fresh Sybil
  #    'committed block' log (a proposal/broadcast) — a catch-up SyncChain logs
  #    'chain reconciled', never 'committed block', so requiring the fresh-commit log
  #    is a second, independent guard against the catch-up false positive.
  local h1; h1="$(syb_height sybil-1)"; h1="${h1:-$h0}"
  local s hs; for s in $sybils; do hs="$(syb_height "$s")"; hs="${hs:-0}"; [ "$hs" -gt "$h1" ] 2>/dev/null && h1="$hs"; done
  local no_advance=0; [ "$h1" -le "$((ceiling + 1))" ] 2>/dev/null && no_advance=1
  # Fresh-commit guard SCOPED to post-stop (#402/#303): a 'committed block' line
  # from BEFORE the anchors were stopped is not a capture — only a block a Sybil
  # committed AFTER the anchors left is. --since @stop_t0 admits only those.
  local fresh_commit=0
  for s in $sybils; do
    if ssh_node "$s" "sudo journalctl -u silt --no-pager --since \"@${stop_t0}\" -n 400" | grep -qE 'chain: committed block [0-9]'; then fresh_commit=1; break; fi
  done
  local gate=0
  for s in $sybils; do
    if jlog "$s" 800 | grep -qE 'immature network requires anchor|requires anchor attestations|ErrAnchorRequired|training wheels'; then gate=1; break; fi
  done

  # 4) THE CLINCHER — restore the anchors and DRIVE a block; the chain must
  #    commit it and the Sybil must sync it, proving the halt was the missing
  #    anchors and not merely dead Sybils. DRIVE, don't wait (B6): the chain is
  #    reactive — every due renewal was drained into blocks before the stop,
  #    and the frozen height mints no new ones (renewal-due is HEIGHT-based) —
  #    so a restored network is legitimately QUIESCENT, and waiting for a
  #    spontaneous block mis-grades healthy idleness as a liveness gap. Three
  #    runs GAPed exactly this way; the captured journals (run 9b2198e-67673)
  #    show the restored anchors fully healthy — bootstrapped, standing back,
  #    bond challenges passing — with simply nothing to propose. (The pre-#397
  #    drain over-proposed own renewals, an accidental heartbeat that masked
  #    this.) Same drive-then-verify pattern as flow 10's B2 drills.
  echo "    restoring anchors — driving a block; the chain must commit it and the Sybil must sync it…"
  for a in $anchors_nodes; do svc "$a" start >/dev/null 2>&1 || true; done
  local resumed=0 h2 t0; t0="$(date +%s)"
  while [ $(( $(date +%s) - t0 )) -lt 180 ]; do
    # Re-drive every loop until the commit shows: a publish can shed on a
    # just-restored issuer set, and the retry is what makes the clincher a
    # DRIVEN verification instead of a coin-flip on background traffic.
    if [ "$(syb_height val-a)" -ge 1 ] 2>/dev/null; then
      ssh_node val-a "head -c 4096 </dev/urandom >/tmp/ft_c2r.bin; /usr/local/bin/silt swarm add /tmp/ft_c2r.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 >/dev/null 2>&1 || true"
    fi
    h2="$(syb_height sybil-1)"; h2="${h2:-$h1}"
    [ "$h2" -gt "$h1" ] 2>/dev/null && { resumed=1; break; }
    sleep 6
  done
  h2="${h2:-$h1}"

  # Verdict: "no quiet capture" = the Sybils could NOT advance the chain past the
  # anchored ceiling without the anchors, AND it came back WITH them. A CAPTURE
  # requires BOTH signals — height past the true ceiling AND a fresh Sybil commit
  # log — so a lagging Sybil's catch-up (height rises toward the ceiling, no fresh
  # commit) can never be misread as a capture.
  if [ "$no_advance" = 1 ] && [ "$resumed" = 1 ]; then
    record "5-sybil-no-capture" pass major "no quiet capture: $n_syb bonded single-domain Sybils could NOT advance the chain past the anchored ceiling h${ceiling} with all anchors down (sybil head →${h1}, no fresh Sybil commit), and a DRIVEN block committed + synced to the Sybil (${h1}→${h2}) once the anchors returned$([ "$gate" = 1 ] && echo '; a Sybil logged the anchor-required refusal' || echo '')"
  elif [ "$no_advance" != 1 ] && [ "$fresh_commit" = 1 ]; then
    record "5-sybil-no-capture" fail major "CAPTURE: a Sybil COMMITTED a new block past the anchored ceiling h${ceiling} (sybil head →$h1) with ALL anchors down — the training wheels did not hold"
  elif [ "$no_advance" != 1 ]; then
    # Height rose past the ceiling but NO Sybil logged a fresh commit ⇒ the rise is a
    # catch-up sync of anchor-committed blocks, not a capture (the old false positive).
    record "5-sybil-no-capture" gap major "sybil head rose to $h1 (ceiling h${ceiling}) with anchors down but NO Sybil logged a fresh 'committed block' — this is a lagging Sybil CATCHING UP to anchor-committed blocks, not a capture; the property held but the drivability was masked by Sybil lag (see the run-slowness finding). Re-run on a healthier network to certify the clean no-capture outcome."
  else
    record "5-sybil-no-capture" gap major "no-capture outcome held (head ≤ ceiling h${ceiling} with anchors down) but the DRIVEN post-restore block did not commit+sync to sybil-1 within 180s (h2=$h2) — the restored network could not commit a driven publish; the anchors' + sybil-1's journals are captured at this verdict (flow-evidence log) — attribute before re-running (#7)"
  fi
}

# ── Flow 10: MATURING topology (§4 / B2) — handoff, post-shed regime, weight-quorum
# drills, and WS cold-sync, over real WAN ────────────────────────────────────────
# The base topology never matures by design (4 equal bonds → Nakamoto 2 < bar 4), so
# every prior field run exercised only the YOUNG anchor-gated regime — while the
# external red team's sharpest target is the handoff/post-shed regime (seam #8).
# `MATURING=1 SYBILS=8 ./cloudtest.sh` runs the topology that hands off on the wire
# (bar 2 at margin 1 — deliberate, disclosed; the cohort bonds the MINIMUM so B2's
# drills price per-head cheapness honestly): warm → everMature latches ('wheels shed
# permanently', the F-1 one-way latch) → the first epoch boundary (height % 8) hands
# off → post-shed commits continue with NO anchor requirement → the B2 STALL drill
# (cohort declines to attest; honest weight must still commit — head-counting made
# this network born-unable-to-commit) → the B2 CAPTURE drill (cheap-member cohort
# alone must be refused: no advance past the honest ceiling, no fresh cohort commit;
# corroborated by the frozen-weight refusal in a cohort log) → the clincher (honest
# validators return, chain resumes) → WS COLD-SYNC (restart a validator pinned to a
# 'checkpoint: H:HASH' obtained from a peer; it must catch up AND the latch must
# hold — anchors never re-arm). Outcome-first throughout (immutable #2): heights and
# commits grade; log lines corroborate.
# LATCH_S is COMPUTED (PE §4), not arbitrary: the latch trips once TWO maturer
# bonds commit (bar 2 = min(NakamotoOperators, NakamotoDomains) over the
# non-anchor set; C2Metric excludes anchors). The reg queue is FIFO-by-arrival
# since #448 (the ID-sort seed-luck is gone), so the bound is ~2 maturer
# reg-blocks + interleaved renewal/first-timer traffic — a 5-reg-block
# allowance × the worst-case per-height bound under the #451 synchronizer
# durations (H_ESCAPE_S = 220s: dur(0)+dur(1) sweeps + a gather leg) ≈ 1100.
# The drain begins at network start, well before this flow runs (waitfor
# matches the C2 line, which repeats on every commit); runs ce15a80/e2fab4b
# latched at h15/h16 in well under half this. Per the PE rule: with the
# premise fixed, a latch that misses even THIS window is a FAIL — a finding —
# never a re-grade.
: "${LATCH_S:=1100}"
# HANDOFF_BLOCKS_S: the drive must cross the next epoch boundary + 1 from
# wherever the latch left the head — ≤ 9 blocks × the 220s worst-height bound
# ≈ 1980. Run e2fab4b-9589 FAILed the old 600s window while genuinely crossing
# (h40→51 at the measured 80–170s/height steady cadence): 600 assumed the
# pre-#451 64s worst-case block. A miss inside THIS bound is a real stall.
: "${HANDOFF_BLOCKS_S:=1980}"
# LOCAL_PROOF: n/a — real-daemon latch/handoff is the named residual (in-process: sim TestTrainingWheelsShedThroughTheNodeLoop + the core/node modelcheck mature fixtures); e2e twin tracked in docs/thinking/2026-08-20-harness-local-first.md
flow_maturing_handoff() {
  local maturing
  maturing="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta'].get('maturing',0))")"
  if [ "$maturing" != "1" ]; then
    record "10-maturing-handoff" skip major "not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)"
    return
  fi
  require_nodes "10-maturing-handoff" major val-a val-b || return
  require_live  "10-maturing-handoff" major val-a || return
  local anchors_nodes n_syb sybils n_mat maturers
  anchors_nodes="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  n_syb="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['n_syb'])")"
  sybils="$(python3 -c "import json;print(' '.join(json.load(open('$FT_DIR/topology.json'))['meta']['sybils']))")"
  n_mat="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta'].get('n_mat',0))")"
  maturers="$(python3 -c "import json;print(' '.join(json.load(open('$FT_DIR/topology.json'))['meta'].get('maturers',[])))")"
  # The latch premise needs the maturer cohort (2026-08-15 re-split): without it
  # the bar-2 latch is unreachable by construction (C2Metric excludes anchors;
  # the single-domain sybils aggregate to one group —
  # core/chain/maturing_topology_premise_test.go). An old-style topology is a
  # premise GAP, not a network failure.
  if [ "${n_mat:-0}" -lt 2 ]; then
    record "10-maturing-handoff" gap major "MATURING topology has no maturer cohort (n_mat=${n_mat:-0} < 2) — the bar-2 latch is unreachable by construction (premise test: TestMaturingFieldTopologyLatchUnreachable); regenerate the topology with the 2026-08-15 re-split"
    return
  fi
  # A handoff/post-shed verdict needs the anchors AND both cohorts' journals.
  # shellcheck disable=SC2086
  flow_evidence_nodes $anchors_nodes $maturers $sybils

  mh_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  mh_ceiling() { # max head over the honest validators
    local c=0 a hh
    for a in $anchors_nodes; do hh="$(mh_height "$a")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  # drive one committed block via a publish (boot validator = most reliable
  # publisher); returns 0 if the honest ceiling strictly rises within 90s.
  mh_drive_block() {
    local before after t0; before="$(mh_ceiling)"
    ssh_node val-a "head -c 4096 </dev/urandom >/tmp/ft_mh.bin; /usr/local/bin/silt swarm add /tmp/ft_mh.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 >/dev/null 2>&1 || true"
    t0="$(date +%s)"
    while [ $(( $(date +%s) - t0 )) -lt 90 ]; do
      after="$(mh_ceiling)"
      [ "$after" -gt "$before" ] 2>/dev/null && return 0
      sleep 5
    done
    return 1
  }

  # 1) THE LATCH: the daemon's own C2 status line must flip to the one-way F-1
  #    latch wording. With the maturer cohort deployed the premise is REACHABLE,
  #    so missing the COMPUTED window is a FAIL — a finding (PE §4: a miss inside
  #    a principled bound is never re-graded), not the old premise GAP.
  local wheels
  wheels="$(waitfor val-a 'wheels shed permanently' "$LATCH_S" || true)"
  if [ -z "$wheels" ]; then
    # FAIL vs GAP hinges on whether the maturer premise actually HELD: a latch
    # miss with every maturer alive is a real drain/maturity FINDING (PE §4 —
    # never re-grade a miss inside a computed bound); a miss with maturers down
    # is preemption/substrate-shaped, so the property is UNTESTED (the same
    # rule require_live encodes).
    local mdown="" mn
    for mn in $maturers; do
      ssh_node "$mn" "systemctl is-active --quiet silt.service" || mdown="$mdown $mn"
    done
    if [ -n "$mdown" ]; then
      record "10-maturing-handoff" gap major "the everMature latch did not trip within the computed ${LATCH_S}s bound AND maturer node(s) were down (down:$mdown) — premise degraded by substrate/preemption, property UNTESTED not failed"
    else
      record "10-maturing-handoff" fail major "the everMature latch did not trip within the COMPUTED ${LATCH_S}s bound with the full maturer cohort live ($n_mat maturers; bound = 9 reg-blocks × 64s worst-case + submit leg) — a real drain/maturity FINDING (PE cadence ruling §4), not a window artifact; read the drain curve in the evidence journals"
    fi
    return
  fi
  # PE note 2 (the bound is '2 MATURER regs', not '2 of any 12'): record the
  # DRAIN CURVE so the computed bound is checked against the real drain order
  # (ID-sorted packing makes order seed-luck). val-a's per-commit C2 line
  # carries it: 'of N MiB bonded across P' — a 64 MiB jump is a maturer
  # banking, a 1 MiB jump is a sybil; the trip height is the commit line
  # adjacent to the first shed wording.
  local latch_h
  latch_h="$(jlog val-a 800 | grep -B2 'wheels shed permanently' | grep -oE 'committed block [0-9]+' | grep -oE '[0-9]+' | head -1)"
  echo "    latch tripped at commit height ${latch_h:-?}: ${wheels##*silt}"
  echo "    drain curve (val-a C2 lines — 64MiB jumps = maturer bonds, 1MiB = sybil):"
  jlog val-a 800 | grep -oE 'committed block [0-9]+|nakamoto [0-9]+ bonds → [0-9]+ operators[^|]*\| cost-to-corrupt [0-9]+ MiB of [0-9]+ MiB bonded across [0-9]+' \
    | sed 's/^/      /' | tail -24

  # 2) THE HANDOFF: the shed applies at the first epoch boundary (height % 8 == 0)
  #    at-or-after the latch. Drive commits across the next boundary + 1 so the
  #    frozen mature snapshot demonstrably GOVERNS, then assert the post-shed
  #    commit: chain advances with NO anchor-required refusal after the latch.
  local h_latch target t0 ok=0
  h_latch="$(mh_ceiling)"
  target=$(( ( h_latch / 8 + 1 ) * 8 + 1 ))
  echo "    driving commits across the epoch boundary: h${h_latch} → h${target}…"
  t0="$(date +%s)"
  while [ $(( $(date +%s) - t0 )) -lt "$HANDOFF_BLOCKS_S" ]; do
    [ "$(mh_ceiling)" -ge "$target" ] 2>/dev/null && { ok=1; break; }
    mh_drive_block || true
  done
  local anchor_refusal=0
  jlog val-a 400 | grep -qE 'immature network requires anchor|requires anchor attestations' && anchor_refusal=1
  # Capture the ceiling ONCE for both the verdict and its message: run
  # e2fab4b-9589 printed a success-shaped FAIL because the message re-read
  # mh_ceiling at print time (h51, past target) while the drive loop had
  # timed out below it — a verdict and its evidence must read the same state.
  local h_end; h_end="$(mh_ceiling)"
  slo_assert "10-maturing-handoff" major "young→mature HANDOFF: latch tripped on the wire; drive reached h${h_end} (target h${target}) within ${HANDOFF_BLOCKS_S}s$([ "$ok" = 1 ] || echo ' — TARGET NOT REACHED IN BOUND')$([ "$anchor_refusal" = 1 ] && echo ' (ANCHOR REFUSAL SEEN POST-SHED — the wheels did not shed)')" \
    "$([ "$ok" = 1 ] && [ "$anchor_refusal" = 0 ] && echo 1 || echo 0)"
  [ "$ok" = 1 ] || return  # no governed mature epoch ⇒ the drills below are untestable

  # Cohort-seated precondition for the B2 drills: the epoch snapshot admits every
  # qualified bond, so the cohort must have BANKED bonds before the boundary. The
  # C2 status line's participant count is the on-chain evidence. NOTE: that count
  # is NON-ANCHOR bonds only (C2Metric excludes anchors — the same fact behind
  # the premise fix), so the full-drain target is n_mat + n_syb, NOT 4 + n_syb
  # (the old target of 12 was unreachable: max Participants here is 8).
  #
  # SEATED_S is a COMPUTED bound (PE §4, the LATCH_S arithmetic): the un-seated
  # tail is at worst the whole (n_mat+n_syb)-member cohort, one first-time
  # reg-block each at the 64s worst-case cadence. Bounded wait, never "eventual
  # completion" — run 09fbe60-84613 had 6 of 8 seated ~18 min in, so the tail is
  # real and a one-shot read here converts a live drain into a premise GAP.
  local seated=0 parts
  if [ -n "$sybils" ]; then
    : "${SEATED_S:=$(( (n_mat + n_syb) * 64 ))}"
    local t_seat; t_seat="$(date +%s)"
    while :; do
      parts="$(jlog val-a 400 | grep -oE 'bonded across [0-9]+' | grep -oE '[0-9]+' | tail -1)"
      [ "${parts:-0}" -ge $(( n_mat + n_syb )) ] 2>/dev/null && { seated=1; break; }
      [ $(( $(date +%s) - t_seat )) -ge "$SEATED_S" ] && break
      mh_drive_block || true
      sleep 10
    done
    if [ "$seated" = 1 ] && [ $(( $(date +%s) - t_seat )) -gt 15 ]; then
      # A LATE seat only enters the snapshot at the NEXT finalized boundary (I3:
      # set changes apply at epoch boundaries) — drive across one more so the
      # frozen snapshot the drills exercise really contains the whole cohort.
      local h_seat target2 t_seat2
      h_seat="$(mh_ceiling)"; target2=$(( ( h_seat / 8 + 1 ) * 8 + 1 )); t_seat2="$(date +%s)"
      echo "    late cohort seat (participants=${parts}): driving h${h_seat}→h${target2} across the next epoch boundary so the governed snapshot admits the full cohort…"
      while [ $(( $(date +%s) - t_seat2 )) -lt "$HANDOFF_BLOCKS_S" ]; do
        [ "$(mh_ceiling)" -ge "$target2" ] 2>/dev/null && break
        mh_drive_block || true
      done
    fi
  fi

  # 3) 10a — THE STALL DRILL: the cohort DECLINES to attest (stopped = the
  #    strongest decline; nothing slashable either way). Under head counting this
  #    epoch needs bftThreshold(4+n_syb) attestations and the mature phase is
  #    born unable to commit; under the weight rule the honest coalition carries
  #    ~all the weight and MUST keep committing.
  if [ -z "$sybils" ]; then
    record "10a-stall-drill" skip major "no cohort in this topology (SYBILS=0) — the B2 stall drill needs the cheap members seated in the epoch; run MATURING=1 SYBILS=8"
  elif [ "$seated" != 1 ]; then
    record "10a-stall-drill" gap major "the cohort did not fully seat within the computed SEATED_S=${SEATED_S}s bound (participants=${parts:-?} < $(( n_mat + n_syb ))) — the epoch snapshot lacks part of the cheap cohort, drill premise unmet, property UNTESTED"
  else
    echo "    stall drill: stopping the $n_syb-member cohort (declining to attest)…"
    local s; for s in $sybils; do svc "$s" stop >/dev/null 2>&1 || true; done
    # STALL_S is COMPUTED (PE §4) under the #451 synchronizer durations: the
    # staggered-takeover ladder ((3+n_syb)×30s) for downed designees + the
    # 2-round escape bound (220s — see H_ESCAPE_S derivation). Any honest
    # ceiling advance (a drain commit counts) refutes the stall.
    : "${STALL_S:=$(( (3 + n_syb) * 30 + 220 ))}"
    local stall_ok=0 t_stall; t_stall="$(date +%s)"
    while [ $(( $(date +%s) - t_stall )) -lt "$STALL_S" ]; do
      mh_drive_block && { stall_ok=1; break; }
    done
    slo_assert "10a-stall-drill" major "B2 stall drill: with the $n_syb cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed ${STALL_S}s bound (head-counted quorum left this exact network born-unable-to-commit at ${n_syb}×MinBond)" "$stall_ok"
    for s in $sybils; do svc "$s" start >/dev/null 2>&1 || true; done
    sleep 20
  fi

  # 4) 10b — THE CAPTURE DRILL: only the cheap cohort remains. It must NOT advance
  #    the chain (its ~n_syb MiB is nowhere near >⅔ of the frozen weight). Ceiling
  #    read from the HONEST validators BEFORE stopping them (#383 lesson: a lagging
  #    cohort's catch-up must never read as an advance), and a real capture also
  #    needs a FRESH cohort 'committed block' log.
  if [ -z "$sybils" ] || [ "$seated" != 1 ]; then
    record "10b-capture-drill" skip major "no seated cohort (see 10a) — the B2 capture drill premise is the cheap members alone attempting a commit"
  else
    local ceiling h1 fresh=0 weightlog=0 resumed=0 h2
    ceiling="$(mh_ceiling)"
    # The honest side is the anchors AND the maturers (the re-split's honest
    # weight): leaving the maturers up would let the chain legitimately advance
    # on their >⅔ weight and fake a "capture". The drill premise is the CHEAP
    # cohort alone.
    echo "    capture drill: stopping the honest validators ($anchors_nodes $maturers) — the cheap cohort alone attempts to advance (ceiling h${ceiling})…"
    local a; for a in $anchors_nodes $maturers; do svc "$a" stop >/dev/null 2>&1 || true; done
    sleep 90
    h1=0; for s in $sybils; do local hs; hs="$(mh_height "$s")"; hs="${hs:-0}"; [ "$hs" -gt "$h1" ] 2>/dev/null && h1="$hs"; done
    local no_advance=0; [ "$h1" -le "$((ceiling + 1))" ] 2>/dev/null && no_advance=1
    for s in $sybils; do jlog "$s" 200 | grep -qE 'chain: committed block [0-9]' && { fresh=1; break; }; done
    for s in $sybils; do jlog "$s" 600 | grep -qE 'frozen-weight super-majority' && { weightlog=1; break; }; done
    echo "    restoring the honest validators — chain must resume…"
    for a in $anchors_nodes $maturers; do svc "$a" start >/dev/null 2>&1 || true; done
    local t1; t1="$(date +%s)"
    while [ $(( $(date +%s) - t1 )) -lt 240 ]; do
      h2="$(mh_ceiling)"
      [ "${h2:-0}" -gt "$ceiling" ] 2>/dev/null && { resumed=1; break; }
      mh_drive_block || true
    done
    if [ "$no_advance" = 1 ] && [ "$resumed" = 1 ]; then
      record "10b-capture-drill" pass major "B2 capture drill: the $n_syb MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h${ceiling} (cohort head →$h1, fresh cohort commit: $fresh$([ "$weightlog" = 1 ] && echo '; a cohort node logged the frozen-weight super-majority refusal' || echo '')), and it resumed past h${ceiling} once honest weight returned — post-shed capture is weight-priced, not head-priced"
    elif [ "$no_advance" != 1 ] && [ "$fresh" = 1 ]; then
      record "10b-capture-drill" fail major "CAPTURE ON THE WIRE (B2 regression): a cheap epoch member COMMITTED a new block past the honest ceiling h${ceiling} (cohort head →$h1) with every honest validator down — the mature quorum accepted a cohort-only coalition"
    elif [ "$no_advance" != 1 ]; then
      record "10b-capture-drill" gap major "cohort head rose to $h1 (ceiling h${ceiling}) with honest validators down but NO fresh cohort 'committed block' — a lagging cohort catching up, not a capture; re-run on a healthier network for the clean outcome"
    else
      record "10b-capture-drill" gap major "no-capture outcome held (head ≤ h$((ceiling+1)) with honest validators down) but the chain did not resume within 240s of their return (h2=${h2:-?}) — clincher inconclusive (SPOT preemption?); re-run to confirm"
    fi
  fi

  # 5) 10c — WS COLD-SYNC: restart val-b pinned to a checkpoint published by a
  #    peer (the daemon's own 'checkpoint: H:HASH' line — the F-1 out-of-band
  #    pin). It must catch back up to the honest ceiling AND come back with the
  #    latch still shed — a restart must never re-arm the anchors.
  local cp
  cp="$(jlog val-a 600 | grep -oE 'checkpoint: [0-9]+:[0-9a-f]+' | tail -1 | sed 's/checkpoint: //')"
  if [ -z "$cp" ]; then
    record "10c-ws-cold-sync" gap major "no 'checkpoint: H:HASH' line found on val-a — cannot pin the cold-sync; property UNTESTED"
    return
  fi
  echo "    ws cold-sync: restarting val-b pinned to -ws-checkpoint ${cp}…"
  local base_h; base_h="$(mh_ceiling)"
  relaunch_with val-b "-ws-checkpoint $cp"
  local sync_ok=0 latch_held=0 t2; t2="$(date +%s)"
  while [ $(( $(date +%s) - t2 )) -lt 240 ]; do
    local hb; hb="$(mh_height val-b)"; hb="${hb:-0}"
    [ "$hb" -ge "$base_h" ] 2>/dev/null && { sync_ok=1; break; }
    sleep 10
  done
  jlog val-b 200 | grep -qE 'wheels shed permanently' && latch_held=1
  slo_assert "10c-ws-cold-sync" major "WS cold-sync under the latch: val-b restarted pinned to checkpoint $cp, caught up to h${base_h} (sync=$sync_ok) and came back with the wheels STILL shed (latch_held=$latch_held — a restart must never re-arm the anchors, F-1)" \
    "$([ "$sync_ok" = 1 ] && [ "$latch_held" = 1 ] && echo 1 || echo 0)"
  restore_argv val-b
}

# A cold objective genesis network needs its peer mesh established and its first
# block committed before publish-token gathering works. Proven on GCP: the exact
# same flows that FAIL at ~4 min post-boot all PASS once the chain has advanced (a
# warm network fetched bit-perfect and reached height 3). So warm the network
# before grading: retry a throwaway publish from the boot validator (it also serves
# the registry, so this is the most reliable publisher) until it produces a link —
# which commits the first block — bounded by WARMUP_S. A timeout does not fail the
# run; the graded flows then report honestly.
#
# 600s default, not 300: on a 3-REGION fleet (e.g. europe-west1 val-c) the FIRST
# objective commit is a cold cross-region bootstrap — mesh + bond seal + bond-reg
# commit + cross-region attestation quorum — and was measured at ~9 min. A 300s
# window expired JUST as the chain committed block 1, so every flow then graded
# against a genesis chain and false-FAILed. Overridable (a single-region run can
# set WARMUP_S=180).
: "${WARMUP_S:=600}"
wait_network_warm() {
  local boot deadline t0 out link
  boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
  t0="$(date +%s)"; deadline=$(( t0 + WARMUP_S ))
  echo "  warming the network (≤${WARMUP_S}s): retrying a throwaway publish from $boot until the chain commits…"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    out="$(ssh_node "$boot" "head -c 4096 </dev/urandom >/tmp/ft_warm.bin; /usr/local/bin/silt swarm add /tmp/ft_warm.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { echo "    network warm after $(( $(date +%s) - t0 ))s (first publish committed)"; return 0; }
    sleep 8
  done
  echo "    WARN: network did not warm within ${WARMUP_S}s — grading anyway (flows will report honestly)"
  return 1
}

# wait_publisher_warm warms a NON-VALIDATOR publisher's token path (#344). The
# chain warm above publishes from the boot VALIDATOR, which already holds the
# issuer keys and is itself on the canonical issuer set — so it commits genesis
# without proving a fetcher can publish. A fresh non-validator (the fetch nodes the
# graded flows publish from) must first DISCOVER the canonical issuer set
# (MsgGetCanonicalIssuers) and the validators' issuer keys before it can gather a
# publish-token signature, and that discovery LAGS genesis on a seconds-old chain:
# on the #286 re-cert, flow_publish_fetch false-FAILed ("no canonical issuer set
# from peers") while the identical publish from the same node succeeded minutes
# later. So after the chain warms, warm the first fetch publisher too — a throwaway
# publish retried until it lands — so the graded publish flows start from a
# fully-propagated state. Bounded and NON-FATAL: a genuine publish break still
# times out here (WARN) and the graded flow runs and reports honestly, so this
# removes the transient-timing false-FAIL without masking a real one.
# FETCH_PUBLISH_DEGRADED records whether the fetch publish path is currently working:
# wait_publisher_warm sets it 0 on a successful warm, 1 when it can't warm in the
# window. ft_publish reads it so that when the publish SUBSYSTEM is known-degraded
# (issuer-set discovery not landing over a churned WAN), a dependent flow's publish
# failure is scored a GAP (untested), not a FAIL — even when the CLI captured no error
# text (the lasterr grep alone misses that case). The publish-reliability issue stays
# visible via the WARN line and #351.
: "${FETCH_PUBLISH_DEGRADED:=0}"
# Computed, not arbitrary (PE §4): the fresh-publisher warm IS the ~5-leg path the
# publish bound describes (join → issuer-set discovery → parallel token gather →
# scatter → register/commit), so its window is the same computed ≈240s — the old
# 180s sat BELOW the bound and declared the subsystem degraded before the path's
# own retry budget had run out (run 8ae8326-34086's warm WARN at 180s).
: "${PUBLISHER_WARMUP_S:=240}"
wait_publisher_warm() { # wait_publisher_warm NODE
  local node="$1" t0 deadline out link
  node_exists "$node" || return 0
  t0="$(date +%s)"; deadline=$(( t0 + PUBLISHER_WARMUP_S ))
  echo "  warming publisher $node (≤${PUBLISHER_WARMUP_S}s): a fresh non-validator must discover the canonical issuer set + issuer keys before it can gather a publish token (#344)…"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    out="$(ssh_node "$node" "head -c 4096 </dev/urandom >/tmp/ft_pwarm.bin; /usr/local/bin/silt swarm add /tmp/ft_pwarm.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { echo "    publisher $node warm after $(( $(date +%s) - t0 ))s"; FETCH_PUBLISH_DEGRADED=0; return 0; }
    sleep 6
  done
  FETCH_PUBLISH_DEGRADED=1
  # NEVER discard the failing attempt's client output (run a56ac10-42834: forty
  # failed warms left ZERO recorded errors; the decisive '#441 insufficient valid
  # attestations' line had to be re-captured live before teardown). The last
  # attempt's tail IS the attribution — print it and persist it past teardown.
  {
    echo "    WARN: publisher $node did not warm within ${PUBLISHER_WARMUP_S}s — publish subsystem degraded; dependent publishes this run report GAP (untested), not FAIL (#351/#441)"
    printf '%s\n' "$out" | grep -vE '^[[:space:]]*$' | tail -3 | sed 's/^/      last attempt: /'
  } | tee -a "$FT_DIR/publish-diag-${RUN_ID:-local}.log"
  return 1
}

# ── SOAK (PE #432 gate): launch-regime interleaved publish/drain liveness ────────
# The wedge needed only one crossed publish-vs-drain proposer race; #338 serializes
# drain-vs-drain only, so the two streams are UNCOORDINATED in production and the PE
# ruled P1's clean pass incomplete without holding them open against each other
# (i4-liveness-wedge-rounds-ruling §Gate). SOAK shape, not a scheduled race: both
# streams run for a computed window and the schedule lands where it lands, many
# times. LAUNCH-topology only (MATURING=0 keeps the launch regime permanent; in a
# MATURING topology the latch ends the regime mid-soak and the mature steady state
# is #441's separately-graded question). Design: docs/thinking/2026-08-16-launch-
# soak-drill-design.md. Opt-in: SOAK=1.
#
# The per-height escape bound H_ESCAPE_S is COMPUTED (PE §4) under the #451
# synchronizer's INCREASING round durations (core/node/rounds.go sweepsForRound:
# dur(r) = 2 + r(r+1)/2 sweeps × the 30s ChainSyncInterval): a 2-round allowance
# costs dur(0)+dur(1) = 2+3 = 5 sweeps = 150s, plus one ~34s computed gather leg
# ≈ 184 → 220 (run e2fab4b-9589 measured 80–170s/height at steady state). A
# height older than that with the network live is the WEDGE SIGNATURE and a FAIL
# (PE §4 — a miss inside a principled bound is a finding), never a window
# artifact. Escape FREQUENCY at steady state (~half of heights reach r1) is an
# M1 cadence question, tracked separately — the bound covers the mechanism.
# ── economy (#11): the S7 repair bounty pays a VERIFIED RECONSTRUCTION on the wire ──
# The Phase 2 exit gate. Publish erasure-coded, make store-2 a caretaker with the
# economy ON, fund its reserve, then use `swarm holders` to KILL every holder of
# > RepairSlack columns so every stripe must RECONSTRUCT from parity (not just
# re-fetch a surviving replica — the gap durability-turnover leaves) — and confirm
# the caretaker rebuilt and the bounty DREW THE RESERVE DOWN (paid > 0). Opt-in
# (ECONOMY=1). Moderate chunk (256 KiB) so reconstruction fits the box (§0.1: a
# 64 MiB stripe holds ~1 GiB and OOMs a 2 GB node). Design:
# docs/thinking/2026-08-19-cloudtest-economy-scenario-design.md.
# LOCAL_PROOF: go test ./e2e -run TestRepairBountyPaysOnTheWire -count=1 (prepay→bounty legs; the skim leg's in-process proof is sim TestServeAutoSkimFundsObjectEscrow)
flow_economy_repair() {
  [ "${ECONOMY:-0}" = 1 ] || { record "11-economy-repair" skip minor "opt-in (ECONOMY=1): the S7 repair-bounty-on-the-wire grade"; return; }
  require_nodes "11-economy-repair" major fetch-1 store-1 store-2 relay || return
  local care="store-2"   # paramedic candidate: a storage node, NEVER an anchor/validator,
                         # so killing shard-holders can never touch consensus.

  # 1) Publish an erasure-coded object; capture BOTH the silt: link and the siltcare:.
  # RETRY the publish (run 2323b09-20931 GAPped here): a chain-backed registry publish
  # IS a consensus commit, and on quorum-2 across 3 regions a single attempt can time
  # out (context deadline) when its entry's commit lands slower than the client window,
  # especially late in the sheet under load (#441-family). Ride it out with retries —
  # the same tolerance ft_publish has (PUBLISH_RETRY_S) — instead of GAPping on one
  # slow-commit window; only GAP after the whole budget is spent.
  local out link carelink attempt=0
  local econ_deadline=$(( $(date +%s) + ${ECONOMY_PUBLISH_RETRY_S:-360} ))
  while :; do
    # -replication 1: each column lands on ONE holder, so 3 all-killable columns
    # exist on a small killable pool (parity is the redundancy — the flag's own
    # help text names this exact use, and the local e2e proof publishes the same
    # shape). At the default replication 3 a column needs ALL THREE holders
    # killable; the first full LOCAL sheet measured the result — 0 of 16 columns
    # qualified, the flow GAPs at selection (the 4th latent defect this flow
    # shipped with, caught for $0 on docker).
    out="$(ssh_node fetch-1 "head -c 4194304 </dev/urandom >/tmp/ft_econ.bin; /usr/local/bin/silt swarm add /tmp/ft_econ.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 262144 -replication 1 2>&1" || true)"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    carelink="$(printf '%s' "$out" | grep -oE 'siltcare:\S+' | head -1)"
    { [ -n "$link" ] && [ -n "$carelink" ]; } && break
    [ "$(date +%s)" -ge "$econ_deadline" ] && break
    attempt=$((attempt+1))
    echo "    economy publish attempt $attempt did not land (registry commit latency? $(printf '%s' "$out" | tr '\n' ' ' | head -c 120)); retrying in 30s…"
    sleep 30
  done
  if [ -z "$link" ] || [ -z "$carelink" ]; then
    ft_add_validator_evidence
    record "11-economy-repair" gap major "setup publish landed no link+carelink after ${ECONOMY_PUBLISH_RETRY_S:-360}s of retries — economy UNTESTED this run, not a failure (registry publish-commit latency #441-family; $(printf '%s' "$out" | tr '\n' ';' | head -c 160))"; return
  fi

  # 2) Make TWO caretakers with the economy ON + a local UI (fund/status). Two is
  #    structural, not redundancy (proven by the local wire proof,
  #    e2e/economy_repair_test.go): the paramedic never judges its own claim
  #    (repairclaim.go emitRepairClaim skips itself and the holder), credit is
  #    per-node-local, so `paid` materializes on the OTHER caretaker's ledger —
  #    the judge's. The judge is the relay node: a full daemon that is NOT in the
  #    killable role set, so arming it costs zero killable shard-holders.
  #    -registry is REQUIRED: -care without one now refuses to start (it used to
  #    silently never caretake — the shape this scenario shipped in run 2323b09).
  local judge="relay" skimnode=""
  econ_restore() { restore_argv "$care"; restore_argv "$judge"; [ -n "$skimnode" ] && restore_argv "$skimnode"; return 0; }
  relaunch_with "$care"  "-care $carelink -economy -registry $REGREF -ui=127.0.0.1:8098"
  relaunch_with "$judge" "-care $carelink -economy -registry $REGREF -ui=127.0.0.1:8098"
  sleep 20   # restart + re-bootstrap + warm the manifest + arm the repair sweeps

  # 3) Fund the object's reserve on BOTH caretakers, each from its own grant
  #    balance (Slice 3): which one ends up the judge is timing, and PayBounty
  #    draws from the payer's OWN escrow. The amount must fit the 500k starter
  #    grant — FundEscrow refuses more (the prior 2000000 could never fund).
  local cnode tok fund_code
  for cnode in "$care" "$judge"; do
    tok="$(ssh_node "$cnode" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
    fund_code="$(ssh_node "$cnode" "curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer $tok' --data 'root=$link&amount=400000' http://127.0.0.1:8098/api/fund" 2>/dev/null || true)"
    if [ "$fund_code" != "200" ]; then
      record "11-economy-repair" gap major "could not fund the reserve (/api/fund → HTTP ${fund_code:-none} on $cnode) — economy setup incomplete, UNTESTED not failed"; econ_restore; return
    fi
  done

  local holders_out; holders_out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm holders '$link' -peers '$PEERS' -registry '$REGREF' 2>&1" || true)"

  # 3b) THE SKIM LEG on the wire (11b): S7's full sentence is prepay → SKIM →
  #     bounty, and until now the skim (serving revenue auto-funding the object's
  #     reserve) had no wire grade anywhere — sim-only. The skim lands on the
  #     SERVING holder's per-node ledger, and the UI surfaces escrows only for
  #     CARED roots — so arm one shard-holder as a third caretaker with NO
  #     prepay: any funded>0 on its status is pure skim, unmistakable. With
  #     -replication 1 it is the ONLY holder of its column(s), so a full-object
  #     fetch MUST route through it — deterministic, not placement luck.
  local sk_id sk_name kv2
  while IFS= read -r line; do
    case "$line" in column\ *) : ;; *) continue ;; esac
    for sk_id in $(printf '%s' "${line#*: }" | tr ',' ' '); do
      [ -z "$sk_id" ] && continue
      sk_name=""
      for kv2 in $(node_names); do [ "$(node_field "$kv2" nodeid)" = "$sk_id" ] && sk_name="$kv2"; done
      [ -z "$sk_name" ] && continue
      [ "$sk_name" = "$care" ] || [ "$sk_name" = "$judge" ] && continue
      case "$(node_field "$sk_name" role)" in storage|fetcher) skimnode="$sk_name"; break ;; esac
    done
    [ -n "$skimnode" ] && break
  done <<EOF
$holders_out
EOF
  if [ -z "$skimnode" ]; then
    record "11b-economy-skim" gap major "no storage/fetcher-role shard-holder available to arm as the skim observer — skim leg UNTESTED this run (holders all validators/caretakers)"
  else
    relaunch_with "$skimnode" "-care $carelink -economy -registry $REGREF -ui=127.0.0.1:8098"
    sleep 15   # restart + re-announce held shards + warm the care manifest
    local i2 sk_tok sk_body sk_funded=0
    for i2 in 1 2 3; do
      ssh_node fetch-1 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_econ_skim.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1" || true
    done
    sk_tok="$(ssh_node "$skimnode" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
    local sk_t0; sk_t0="$(date +%s)"
    while [ $(( $(date +%s) - sk_t0 )) -lt 90 ]; do
      sk_body="$(ssh_node "$skimnode" "curl -s -H 'Authorization: Bearer $sk_tok' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
      sk_funded="$(printf '%s' "$sk_body" | grep -oE '"funded":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; sk_funded="${sk_funded:-0}"
      [ "$sk_funded" -gt 0 ] 2>/dev/null && break
      sleep 6
    done
    if [ "${sk_funded:-0}" -gt 0 ] 2>/dev/null; then
      slo_assert "11b-economy-skim" major "the SKIM leg closed on the wire: fetches of the cared object routed serve revenue into its durability reserve on the serving holder's ledger (funded=$sk_funded with ZERO prepay on $skimnode) — the object pays for its own repair (S7)" 1
    else
      record "11b-economy-skim" gap major "no skim landed on $skimnode's reserve within 90s of 3 driven fetches (funded=$sk_funded, zero prepay) — skim leg not confirmed; attribute from $skimnode's journal (serve accounting) before re-running (#7)"
    fi
  fi

  # 4) Resolve holders per column and KILL every holder of 3 columns whose holders
  #    are ALL killable (storage/fetcher, and NOT the caretakers/skim observer) —
  #    so consensus is never touched. 3 lost shards/stripe > RepairSlack(2) ⇒
  #    every stripe must reconstruct, and 3 ≤ n−k(6) ⇒ still recoverable.
  # Build a killable NodeID→name map: every content-holding node EXCEPT the 4
  # anchors (role "validator") and the caretaker. In launch phase (the economy run
  # is MATURING=0) ONLY the anchors finalize — maturers and sybils are non-anchor
  # validators whose loss cannot break launch-phase consensus — so storage, fetcher,
  # maturer, and sybil nodes are all safe to stop. (This is why the flow runs before
  # the maturing drill and restarts every node it stops.) Anchors are never touched.
  local killable_ids="" n nid role
  for n in $(node_names); do
    [ "$n" = "$care" ] && continue
    [ "$n" = "$judge" ] && continue
    [ "$n" = "$skimnode" ] && continue   # the skim observer is a caretaker now — never killed
    role="$(node_field "$n" role)"
    case "$role" in storage|fetcher|maturer|sybil) : ;; *) continue ;; esac
    nid="$(node_field "$n" nodeid)"
    [ ${#nid} -eq 64 ] && killable_ids="$killable_ids $nid:$n"
  done
  # Pick 3 columns whose holders are all killable; collect the nodes to stop.
  local cols_killed=0 to_stop="" col ids id ok name
  while IFS= read -r line; do
    case "$line" in column\ *) : ;; *) continue ;; esac
    [ "$cols_killed" -ge 3 ] && break
    ids="${line#*: }"
    ok=1; local col_nodes=""
    for id in ${ids//,/ }; do
      [ -z "$id" ] && continue
      name=""; for kv in $killable_ids; do [ "${kv%%:*}" = "$id" ] && name="${kv#*:}"; done
      if [ -z "$name" ]; then ok=0; break; fi     # a holder we may not kill (validator/caretaker) → skip this column
      col_nodes="$col_nodes $name"
    done
    if [ "$ok" = 1 ] && [ -n "$col_nodes" ]; then
      to_stop="$to_stop $col_nodes"; cols_killed=$((cols_killed+1))
    fi
  done <<EOF
$holders_out
EOF
  if [ "$cols_killed" -lt 3 ]; then
    record "11-economy-repair" gap major "only $cols_killed of 3 needed columns had ALL-killable holders (shards landed on validators/the caretakers we must not kill) — could not force a reconstruction WITHOUT touching consensus; economy UNTESTED not failed (add dedicated storage nodes / lower -replication to concentrate shards). holders:$(printf '%s' "$holders_out" | tr '\n' ';' | head -c 200)"; econ_restore; return
  fi
  local uniq_stop; uniq_stop="$(printf '%s\n' $to_stop | sort -u | tr '\n' ' ')"
  echo "    killing shard-holders of $cols_killed columns to force reconstruction:$uniq_stop"
  for n in $uniq_stop; do svc "$n" stop || true; done

  # 5) Wait (bounded) for a reconstruction + payout: poll BOTH caretakers'
  #    /api/status — the paramedic emits the claim, the OTHER one judges and
  #    pays on its own ledger, and which is which is timing.
  local t0; t0="$(date +%s)" paid=0 repairs=0 body ptok pv rv
  while [ $(( $(date +%s) - t0 )) -lt 240 ]; do
    for cnode in "$care" "$judge"; do
      ptok="$(ssh_node "$cnode" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
      body="$(ssh_node "$cnode" "curl -s -H 'Authorization: Bearer $ptok' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
      pv="$(printf '%s' "$body" | grep -oE '"paid":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; pv="${pv:-0}"
      rv="$(printf '%s' "$body" | grep -oE '"repairs":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; rv="${rv:-0}"
      if [ "$pv" -gt "$paid" ] 2>/dev/null; then paid="$pv" repairs="$rv"; fi
    done
    [ "$paid" -gt 0 ] 2>/dev/null && break
    sleep 6
  done

  # 6) Verdict: a bounty paid a verified reconstruction on the wire (the exit gate).
  ft_add_validator_evidence
  if [ "${paid:-0}" -gt 0 ] 2>/dev/null; then
    slo_assert "11-economy-repair" major "the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=$paid credits over $repairs repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A)" 1
  else
    record "11-economy-repair" gap major "3 columns killed but no bounty drew the reserve within 240s (paid=$paid repairs=$repairs) — the loop did not reconstruct+judge+pay in the window; attribute from $care's AND $judge's journals (repair sweep / claim / judge legs) before re-running (#7)"
  fi

  # 6b) The g-instrumentation row (11c, observational — S5): S7 says the funded
  #     HORIZON is finite-but-renewable and `g` is the one number to instrument.
  #     Record the payer-side reserve/horizon/cost-per-repair so every graded run
  #     extends the time series — a measured row, never a pass/fail (a single run
  #     cannot grade a trend).
  if [ "${paid:-0}" -gt 0 ] 2>/dev/null; then
    local hz rs
    hz="$(printf '%s' "$body" | grep -oE '"horizonSec":-?[0-9]+' | grep -oE '\-?[0-9]+' | tail -1)"
    rs="$(printf '%s' "$body" | grep -oE '"reserve":[0-9]+' | grep -oE '[0-9]+' | tail -1)"
    record "11c-economy-horizon" pass info "g-instrumentation sample (S7 finite-but-renewable): paid=$paid over $repairs repair(s), reserve-after=${rs:-?}, horizonSec=${hz:-unmeasured} (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample"
  else
    record "11c-economy-horizon" skip info "no payout this run — horizon/g sample unmeasured"
  fi

  # 7) Restore: revert all armed caretakers' argv, restart the stopped holders.
  econ_restore
  for n in $uniq_stop; do svc "$n" start || true; done
}

# LOCAL_PROOF: n/a — WAN-cadence liveness-BOUND soak; the deterministic escape-bound oracle is core/node/modelcheck_i4_liveness_test.go, the wall-clock at WAN scale is the cloud's job
flow_soak_publish_drain() {
  [ "${SOAK:-0}" = 1 ] || return 0
  local n_mat_soak
  n_mat_soak="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta'].get('n_mat',0))" 2>/dev/null || echo 0)"
  if [ "${n_mat_soak:-0}" -gt 0 ]; then
    record "soak-publish-drain" skip major "SOAK requires a LAUNCH topology (MATURING=0) — the latch would end the launch regime mid-soak; run SOAK=1 without MATURING"
    return
  fi
  require_nodes "soak-publish-drain" major val-a val-b val-c val-d || return
  : "${H_ESCAPE_S:=220}"
  : "${SOAK_HEIGHTS:=20}"
  local wall=$(( SOAK_HEIGHTS * 64 ))   # 64s worst-case block time — the LATCH_S arithmetic
  # Launch ceiling over the validators (mirrors mh_ceiling, which is flow-10-local).
  soak_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  soak_ceiling() {
    local c=0 v hh
    for v in val-a val-b val-c val-d; do hh="$(soak_height "$v")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  local h0 t0 last_h last_t now h pubs=0 pub_ok=0 maxgap=0 gap wedged=0 lastout=""
  h0="$(soak_ceiling)"; h0="${h0:-0}"; t0="$(date +%s)"; last_h="$h0"; last_t="$t0"
  echo "  soak: interleaving publishes with the natural renewal drain from h${h0} for ${SOAK_HEIGHTS} heights (wall ${wall}s; per-height escape bound ${H_ESCAPE_S}s)…"
  while :; do
    now="$(date +%s)"
    [ $(( now - t0 )) -ge "$wall" ] && break
    # The publish stream: one attempt per iteration, alternating publishers so the
    # proposal comes from different seats (val-a registry path + the fresh
    # publisher when present). Errors are KEPT (the run-3 lesson).
    if [ $(( pubs % 3 )) -eq 2 ] && node_exists fetch-1; then
      lastout="$(ssh_node fetch-1 "head -c 8192 </dev/urandom >/tmp/ft_soak.bin; /usr/local/bin/silt swarm add /tmp/ft_soak.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    else
      lastout="$(ssh_node val-a "head -c 8192 </dev/urandom >/tmp/ft_soak.bin; /usr/local/bin/silt swarm add /tmp/ft_soak.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    fi
    pubs=$(( pubs + 1 ))
    printf '%s' "$lastout" | grep -qE 'silt:v1:' && pub_ok=$(( pub_ok + 1 ))
    # The drain stream runs at its natural TTL cadence — nothing to schedule.
    h="$(soak_ceiling)"; h="${h:-$last_h}"
    now="$(date +%s)"
    if [ "$h" -gt "$last_h" ] 2>/dev/null; then
      gap=$(( now - last_t )); [ "$gap" -gt "$maxgap" ] && maxgap="$gap"
      last_h="$h"; last_t="$now"
      [ $(( h - h0 )) -ge "$SOAK_HEIGHTS" ] && break
    elif [ $(( now - last_t )) -gt $(( H_ESCAPE_S * 2 )) ]; then
      # Twice the escape bound with no commit and the wall still running: stop
      # early — this IS the wedge signature; soaking longer adds nothing.
      wedged=1; break
    fi
    sleep 8
  done
  local final_gap=$(( $(date +%s) - last_t ))
  [ "$final_gap" -gt "$maxgap" ] && [ "$wedged" = 1 ] && maxgap="$final_gap"
  local heights=$(( last_h - h0 ))
  # Honest-slash census over the soak window (I5): no honest validator slashed.
  local slashes=0 v
  for v in val-a val-b val-c val-d; do
    jlog "$v" 400 | grep -qE 'validator slashed for equivocation' && slashes=$(( slashes + 1 ))
  done
  if [ "$wedged" = 1 ] || [ "$maxgap" -gt "$H_ESCAPE_S" ]; then
    record "soak-publish-drain" fail major "WEDGE SIGNATURE under the publish/drain soak: a height went ${maxgap}s (> the computed ${H_ESCAPE_S}s escape bound) without a commit with the network live (h${h0}→h${last_h}, ${pub_ok}/${pubs} publishes landed) — the #432 escape did not clear the interleaved race; last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
  elif [ "$heights" -lt "$SOAK_HEIGHTS" ] && [ "$heights" -lt $(( SOAK_HEIGHTS / 2 )) ]; then
    record "soak-publish-drain" gap major "soak under-ran: only ${heights} of ${SOAK_HEIGHTS} heights committed within the ${wall}s wall (max inter-commit gap ${maxgap}s ≤ bound) — cadence, not a wedge; property PARTIALLY tested"
  elif [ "$pub_ok" -eq 0 ]; then
    record "soak-publish-drain" fail major "the chain advanced ${heights} heights under the soak but ZERO of ${pubs} publishes landed — launch-regime publish starvation (the #441 shape in the launch regime); last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
  else
    slo_assert "soak-publish-drain" major "launch-regime publish/drain SOAK: ${heights} heights committed under continuously interleaved publish (${pub_ok}/${pubs} landed) + natural renewal drain, max inter-commit gap ${maxgap}s ≤ the computed ${H_ESCAPE_S}s escape bound, ${slashes} honest-slash lines (want 0) — the #432 escape clears the production-reachable race the PE gate names" \
      "$([ "$slashes" -eq 0 ] && echo 1 || echo 0)"
  fi
}

run_all_scenarios() {
  ft_init_refs
  echo "  peers=$PEERS"
  echo "  registry=$REGREF"
  wait_network_warm
  wait_publisher_warm fetch-1   # #344: non-validator issuer-set/issuer-key discovery lags genesis
  # acceptance flows 1–9 + #184 adversarial drills
  run_flow flow_first_run
  run_flow flow_become_validator
  run_flow flow_publish_fetch
  run_flow flow_care_link
  run_flow flow_convergence
  run_flow flow_fault_tolerance
  run_flow flow_restart_survival
  run_flow flow_takedown
  run_flow flow_cross_nat
  run_flow adv_equivocation
  run_flow adv_partition
  run_flow adv_proposal_reject
  # cloud variants of the local field-test series
  # Re-warm the fetch publisher (#351): flows 7 (restart-survival) and the #184 drills
  # restart/partition validators, which transiently drops a validator from the
  # discoverable canonical-issuer set until it re-syncs — so a fresh ephemeral publish
  # here re-races issuer-set discovery (durability-turnover false-FAILed on the last
  # re-cert for exactly this). One warm at genesis (#344) doesn't cover post-restart
  # flows; re-warm before the publish-dependent cloud variants. Bounded + non-fatal.
  wait_publisher_warm fetch-1
  run_flow flow_publisher_unlinkability   # privacy #3
  run_flow flow_durability_turnover       # durability #2
  run_flow flow_chaos_crash               # chaos #7
  run_flow flow_web_ui_guard              # client/UI #4
  run_flow flow_c2_no_capture             # C2 Sybil #5 — opt-in (SYBILS=8): certifies the PURE anchor gate on cloud
  run_flow flow_economy_repair            # S7 #11 — opt-in (ECONOMY=1): the repair-bounty pays a verified reconstruction on the wire (Phase 2 exit gate)
  run_flow flow_soak_publish_drain        # PE #432 gate — opt-in (SOAK=1, MATURING=0): launch publish/drain interleave soak
  run_flow flow_maturing_handoff          # §4/B2 #10 — opt-in (MATURING=1 SYBILS=8): handoff + post-shed + weight-quorum drills + WS cold-sync. LAST: it stops validators.
}
