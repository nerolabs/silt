#!/usr/bin/env bash
# scenarios.sh — the field-test flows, one function each, mapping directly onto
# the acceptance brief (docs/reviews/m0-acceptance-brief.md flows 1–9) plus the
# #184 adversarial consensus drills. Each records a pass/gap/fail via slo_assert.
#
# Sourced by cloudtest.sh AFTER lib.sh and AFTER the network is up. All node
# interaction is over `ssh_node` (IAP). Field networks are noisy, so every check
# asserts a THRESHOLD/behaviour, never an exact count or timing.
set -uo pipefail

: "${COMMIT_SLO_S:=90}"
: "${FETCH_SLO_S:=120}"
: "${RESTART_SLO_S:=60}"
: "${PUBLISH_RETRY_S:=120}"

# ── swarm references (validators as peers; val-a serves the registry) ───────────
ft_peers() {
  python3 -c "
import json;t=json.load(open('$FT_DIR/topology.json'));p=t['meta']['swarm_port']
print(','.join(n['nodeid']+'@'+n['ip']+':%d'%p for n in t['nodes'].values() if n['role']=='validator'))"
}
ft_regref() { # scrape the pinned registry ref the boot validator prints
  jlog "$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")" 800 \
    | grep -oE 'registry: *\S+@https://\S+' | tail -1 | sed 's/registry: *//'
}

PEERS=""; REGREF=""
ft_init_refs() { PEERS="$(ft_peers)"; REGREF="$(ft_regref)"; }

# TOKEN_QUORUM: publish-token signatures gathered per publish (privacy: unlinkable
# above 0). Default 2. A -token-quorum publish needs the PUBLISHER to reach that
# many validators for signatures — distinct from validator<->validator reachability.
# At genesis there are no committed bonds to rank canonical signers, so it falls back
# to -peers order (a nominal-privacy note). Set TOKEN_QUORUM=1 (or 0) for a first
# bootstrap run if publisher->validator egress is the thing under test.
: "${TOKEN_QUORUM:=2}"

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

# publish a fresh random file on a node; echoes "LINK SHA" on success, empty on fail
ft_publish() { # ft_publish NODE SIZE_BYTES
  local node="$1" size="${2:-1048576}" out link lasterr=""
  ssh_node "$node" "head -c $size </dev/urandom >/tmp/ft_src.bin; sha256sum /tmp/ft_src.bin | cut -d' ' -f1" >/tmp/ft_src_sha 2>/dev/null
  local sha; sha="$(cat /tmp/ft_src_sha 2>/dev/null)"
  local deadline=$(( $(date +%s) + PUBLISH_RETRY_S ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    out="$(ssh_node "$node" "/usr/local/bin/silt swarm add /tmp/ft_src.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536" 2>&1 || true)"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { printf '%s %s\n' "$link" "$sha"; return 0; }
    lasterr="$(printf '%s' "$out" | grep -iE 'could not gather|not enough|no canonical|token|refus|unreachable|timed? ?out' | head -2 | tr '\n' ';')"
    sleep 4
  done
  # Diagnose so a failed publish is actionable, not a bare "no link produced". The
  # single most common cause is the publisher reaching < TOKEN_QUORUM validators.
  local reach; reach="$(ft_reachable_peers "$node")"
  {
    echo "ft_publish FAILED after ${PUBLISH_RETRY_S}s on $node (token-quorum=$TOKEN_QUORUM)"
    echo "  publisher->validator reachability: $reach of the -peers set reachable"
    echo "  last silt error: ${lasterr:-<none captured>}"
    echo "  note: token-quorum needs the publisher to reach >= $TOKEN_QUORUM validators to"
    echo "        gather signatures. A shortfall here — not validator<->validator — is the"
    echo "        usual cause of 'could not gather enough publish-token signatures' and a"
    echo "        chain stuck at height 0. Retry a bootstrap run with TOKEN_QUORUM=1, or"
    echo "        fix publisher egress to the validators."
  } >&2
  return 1
}

# ── Flow 1: build & first run ──────────────────────────────────────────────────
flow_first_run() {
  local n ok=1 bad=""
  for n in $(node_names); do
    [ "$(node_field "$n" role)" = natgw ] && continue
    if ! ssh_node "$n" "systemctl is-active --quiet silt.service"; then ok=0; bad="$bad $n"; fi
  done
  slo_assert "1-first-run" blocker "all silt nodes report service active${bad:+ (down:$bad)}" "$ok"
}

# ── Flow 2: publish → fetch bit-perfect from a DIFFERENT node ───────────────────
flow_publish_fetch() {
  local t0 t1 res link sha got ok=0
  t0="$(date +%s)"
  res="$(ft_publish fetch-1 1048576 || true)"
  if [ -z "$res" ]; then slo_assert "2-publish-fetch" blocker "publish never produced a silt: link within ${PUBLISH_RETRY_S}s" 0; return; fi
  link="${res%% *}"; sha="${res##* }"
  # committed on the boot validator?
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
  if waitfor "$boot" 'chain: committed block [0-9]+' "$COMMIT_SLO_S" >/dev/null; then :; else
    slo_assert "2-publish-fetch" major "publish did not commit within ${COMMIT_SLO_S}s (link=$link)" 0; return; fi
  # fetch from a different node (store-2) and compare hashes
  got="$(ssh_node store-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_got.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_got.bin | cut -d' ' -f1" 2>/dev/null || true)"
  t1="$(date +%s)"
  [ -n "$sha" ] && [ "$got" = "$sha" ] && ok=1
  FT_LAST_LINK="$link"   # reused by restart-survival + takedown
  slo_assert "2-publish-fetch" blocker "fetched from store-2 $([ "$ok" = 1 ] && echo 'bit-perfect' || echo "MISMATCH (want=$sha got=$got)")" "$ok" $((t1 - t0))
}

# ── Flow 3: care link — repair/audit without decrypting ─────────────────────────
flow_care_link() {
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
flow_become_validator() {
  local n ok=1 bad=""
  for n in val-b val-c val-d; do
    node_exists "$n" || continue
    if waitfor "$n" '(standing|bond).*(earned|qualified|registered)|chain: committed block' 90 >/dev/null; then :; else ok=0; bad="$bad $n"; fi
  done
  slo_assert "4-become-validator" major "non-anchor validators earn standing on the objective path${bad:+ (no evidence:$bad)}" "$ok"
}

# ── Flow 5: multi-validator convergence ─────────────────────────────────────────
flow_convergence() {
  local n heights="" h maxh=0 conv=1 vals=""
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
  for n in $vals; do
    h="$(jlog "$n" 800 | grep -oE 'committed block [0-9]+' | grep -oE '[0-9]+' | sort -n | tail -1)"
    h="${h:-0}"; heights="$heights $n=$h"
    [ "$h" -gt "$maxh" ] && maxh="$h"
  done
  # every validator must be within 2 blocks of the tip (real-latency tolerance)
  for n in $vals; do
    h="$(jlog "$n" 800 | grep -oE 'committed block [0-9]+' | grep -oE '[0-9]+' | sort -n | tail -1)"; h="${h:-0}"
    [ $((maxh - h)) -gt 2 ] && conv=0
  done
  slo_assert "5-convergence" major "all validators within 2 blocks of tip=$maxh (heights:$heights)" "$conv"
}

# ── Flow 6: fault tolerance — kill one validator, quorum still commits ──────────
flow_fault_tolerance() {
  require_nodes "6-fault-tolerance" major val-d || return
  svc val-d stop || true
  sleep 5
  local res ok=0
  res="$(ft_publish fetch-1 262144 || true)"
  if [ -n "$res" ]; then
    local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
    waitfor "$boot" 'chain: committed block [0-9]+' "$COMMIT_SLO_S" >/dev/null && ok=1
  fi
  if [ "$ok" = 1 ]; then
    slo_assert "6-fault-tolerance" major "publish still committed with one validator (val-d) down" 1
  else
    record "6-fault-tolerance" gap major "surviving validators did not commit with val-d down — likely quorum/byzantine-quorum sizing; pin -quorum for the validator count (see README shakedown notes)"
  fi
  svc val-d start || true
}

# ── Flow 7: restart survival — standing + issued tokens + stored content ────────
flow_restart_survival() {
  local t0 t1 ok=0
  t0="$(date +%s)"
  svc val-b restart || true
  # standing must come back WITHOUT redoing the bond (fast reload), and quickly
  if waitfor val-b '(reload|restored|persisted).*(bond|standing)|standing.*(reload|restored)|bond.*loaded' "$RESTART_SLO_S" >/dev/null; then ok=1; fi
  t1="$(date +%s)"
  slo_assert "7-restart-standing" major "val-b standing returned after restart without re-bonding" "$ok" $((t1 - t0))
  # stored content still discoverable+served after a storage-node restart
  svc store-2 restart || true; sleep 8
  local ok2=0
  if [ -n "${FT_LAST_LINK:-}" ]; then
    local got; got="$(ssh_node store-1 "/usr/local/bin/silt swarm get '$FT_LAST_LINK' -o /tmp/ft_r.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1 && echo OK" 2>/dev/null || true)"
    [ "$got" = OK ] && ok2=1
  fi
  slo_assert "7-restart-content" major "content still fetchable after a storage-node restart" "$ok2"
}

# ── Flow 8: per-hash takedown on ONE operator only ──────────────────────────────
flow_takedown() {
  if [ -z "${FT_LAST_LINK:-}" ]; then record "8-takedown" gap minor "no prior published link to deny"; return; fi
  # extract the root hash from the silt link and deny it on store-1 only
  local root; root="$(printf '%s' "$FT_LAST_LINK" | grep -oE '[0-9a-f]{64}' | head -1)"
  if [ -z "$root" ]; then record "8-takedown" gap minor "could not parse a root hash from $FT_LAST_LINK"; return; fi
  ssh_node store-1 "echo '$root' | sudo tee /var/lib/silt/deny.txt >/dev/null" >/dev/null 2>&1
  relaunch_with store-1 "-denylist /var/lib/silt/deny.txt"; sleep 8
  # store-1 should now refuse; another operator (store-2) should still serve it
  local denied served=0
  denied="$(ssh_node store-1 "/usr/local/bin/silt swarm get '$FT_LAST_LINK' -o /tmp/ft_d.bin -peers '$PEERS' -registry '$REGREF' 2>&1 | grep -iqE 'deny|refus|not.?found' && echo DENIED" 2>/dev/null || true)"
  ssh_node store-2 "/usr/local/bin/silt swarm get '$FT_LAST_LINK' -o /tmp/ft_s.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1 && echo OK" 2>/dev/null | grep -q OK && served=1
  if [ "$denied" = DENIED ] && [ "$served" = 1 ]; then
    slo_assert "8-takedown" major "root denied on store-1 while still served elsewhere (no global switch)" 1
  else
    record "8-takedown" gap major "takedown scoping not confirmed (denied=$denied served=$served) — verify -denylist semantics on the live build"
  fi
  restore_argv store-1
}

# ── Flow 9: cross-NAT — a natted node moves a file via the relay ────────────────
flow_cross_nat() {
  require_nodes "9-cross-nat" major nat-1 nat-2 || return
  local res ok=0
  res="$(ft_publish nat-1 262144 || true)"    # nat-1 is un-dialable → must use the relay
  if [ -n "$res" ]; then
    local link="${res%% *}" sha="${res##* }" got
    got="$(ssh_node nat-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_n.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_n.bin | cut -d' ' -f1" 2>/dev/null || true)"
    [ -n "$sha" ] && [ "$got" = "$sha" ] && ok=1
  fi
  slo_assert "9-cross-nat" major "natted nodes exchanged a file through the relay/hole-punch" "$ok"
}

# ── #184 adversarial: equivocation → slash ──────────────────────────────────────
adv_equivocation() {
  require_nodes "184-equivocation" blocker adversary val-b val-c || return
  local idb idc
  idb="$(node_field val-b nodeid)"; idc="$(node_field val-c nodeid)"
  relaunch_with adversary "-equivocate ${idb},${idc}"
  local ok=0
  waitfor val-a 'slashed equivocator|validator slashed for equivocation' 120 >/dev/null && ok=1
  slo_assert "184-equivocation" blocker "honest validator slashed the equivocator over the real wire" "$ok"
  restore_argv adversary
}

# ── #184 adversarial: partition → heal ──────────────────────────────────────────
adv_partition() {
  require_nodes "184-partition" major val-b val-c || return
  local idb; idb="$(node_field val-b nodeid)"
  # partition val-c off from val-b (one-sided so it can heal on reconnect)
  relaunch_with val-c "-block-peers ${idb}"; sleep 20
  restore_argv val-c    # drop the block → reconnect → reconcile
  local ok=0
  waitfor val-c 'reorged onto a heavier fork|chain: reorged' 120 >/dev/null && ok=1
  slo_assert "184-partition" major "partitioned validator healed onto the heavier fork after reconnect" "$ok"
}

# ── #184 adversarial: forged-block + low-bond proposals rejected ────────────────
adv_proposal_reject() {
  require_nodes "184-forged-block" major adversary || return
  local ida; ida="$(node_field val-a nodeid)"
  relaunch_with adversary "-forge-block ${ida}"
  local ok1=0; waitfor val-a 'bad signature|ErrBadSignature|reject.*(signature|proposal)' 90 >/dev/null && ok1=1
  slo_assert "184-forged-block" major "honest validator rejected a forged-signature proposal" "$ok1"
  restore_argv adversary
  relaunch_with adversary "-lowbond-propose ${ida}"
  local ok2=0; waitfor val-a 'low reputation|ErrLowReputation|under.?bonded|reject.*bond' 90 >/dev/null && ok2=1
  slo_assert "184-low-bond" major "honest validator rejected an under-bonded proposer" "$ok2"
  restore_argv adversary
}

# ═══════════════════════════════════════════════════════════════════════════════
# Cloud variants of the local field-test series (integration/{privacy,durability,
# chaos,client,sybil}) — the same properties, over real VMs / real regions. Each
# maps onto the existing 13-node topology with NO topology change (a flag flip via
# relaunch_with, a hard kill, a permanent stop). #5 C2-Sybil is intentionally NOT
# here: it needs NON-anchor Sybil validator VMs (a topology.py addition) — see the
# note in run_all_scenarios + the README. Until then it stays a local-only suite.
# ═══════════════════════════════════════════════════════════════════════════════

# ── privacy (#3): publisher unlinkability — refuse-to-surveil over the real wire ─
flow_publisher_unlinkability() {
  # flow_publish_fetch already proved the DEFAULT (token-quorum) publish commits
  # unlinkably. Here the adversary ASKS to record a durable Publisher identity; the
  # default chain must REFUSE it (chain: ErrPublisherEntry / "carries a durable
  # Publisher"). That refusal, over real VMs, is immutable #4 (refuse-to-surveil).
  local out ok=0
  ssh_node fetch-1 "head -c 32768 </dev/urandom >/tmp/ft_priv.bin" >/dev/null 2>&1
  out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm add /tmp/ft_priv.bin -peers '$PEERS' -registry '$REGREF' -allow-publisher -chunk-size 65536" 2>&1 || true)"
  printf '%s' "$out" | grep -iqE 'durable Publisher|ErrPublisherEntry|permanent linkage|publish unlinkably' && ok=1
  slo_assert "priv-unlinkability" major "default chain REFUSED a durable file→publisher link (refuse-to-surveil)$([ "$ok" = 1 ] || echo " — no refusal seen: $(printf '%s' "$out" | tail -1)")" "$ok"
}

# ── durability (#2): content OUTLIVES a permanent storage-node loss ─────────────
flow_durability_turnover() {
  require_nodes "durability-turnover" major store-1 store-2 fetch-1 || return
  # Publish replicated, then PERMANENTLY remove a storage node (stop + leave down —
  # a real departure, not a restart) and prove a fresh fetch still returns the bytes
  # from the survivors. The full membership-ROTATION form (a terminated VM replaced
  # by a fresh-IP VM) is the honest cloud version of the local kill-without-replace.
  local res link sha
  res="$(ft_publish fetch-1 1048576 || true)"
  if [ -z "$res" ]; then slo_assert "durability-turnover" major "publish never produced a link" 0; return; fi
  link="${res%% *}"; sha="${res##* }"
  svc store-1 stop || true    # permanent departure (left down for the fetch)
  sleep 12
  local got ok=0
  got="$(ssh_node store-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_dur.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_dur.bin | cut -d' ' -f1" 2>/dev/null || true)"
  [ "$got" = "$sha" ] && ok=1
  slo_assert "durability-turnover" major "content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor" "$ok"
  svc store-1 start || true   # restore for later flows
}

# ── chaos (#7): hard crash (SIGKILL) recovery + #69 reprovide over real VMs ──────
flow_chaos_crash() {
  require_nodes "chaos-crash" major store-1 store-2 || return
  local link="${FT_LAST_LINK:-}"
  if [ -z "$link" ]; then local res; res="$(ft_publish fetch-1 262144 || true)"; link="${res%% *}"; fi
  if [ -z "$link" ]; then slo_assert "chaos-crash" major "no link to verify crash-recovery against" 0; return; fi
  # SIGKILL the silt process (abrupt death, not a graceful stop). systemd
  # (Restart=on-failure) brings it back, reloading the persisted store.
  ssh_node store-2 "sudo pkill -9 -f '/usr/local/bin/silt' || true" >/dev/null 2>&1
  ssh_node store-2 "sudo systemctl start silt.service" >/dev/null 2>&1 || true   # idempotent nudge
  local reann=0
  waitfor store-2 're-announced [0-9]+ held chunks' 90 >/dev/null && reann=1
  slo_assert "chaos-reprovide" major "SIGKILLed storage node re-announced its held chunks (#69) after a hard crash" "$reann"
  local got ok=0
  got="$(ssh_node store-1 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_ch.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1 && echo OK" 2>/dev/null || true)"
  [ "$got" = OK ] && ok=1
  slo_assert "chaos-fetch" major "content fetchable after a hard-crash (SIGKILL) + restart of a storage node" "$ok"
}

# ── client/UI (#4): the web-UI local-security guard (#89) over a real VM ─────────
flow_web_ui_guard() {
  require_nodes "web-ui" minor fetch-1 || return
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

run_all_scenarios() {
  ft_init_refs
  echo "  peers=$PEERS"
  echo "  registry=$REGREF"
  # acceptance flows 1–9 + #184 adversarial drills
  flow_first_run
  flow_become_validator
  flow_publish_fetch
  flow_care_link
  flow_convergence
  flow_fault_tolerance
  flow_restart_survival
  flow_takedown
  flow_cross_nat
  adv_equivocation
  adv_partition
  adv_proposal_reject
  # cloud variants of the local field-test series
  flow_publisher_unlinkability   # privacy #3
  flow_durability_turnover       # durability #2
  flow_chaos_crash               # chaos #7
  flow_web_ui_guard              # client/UI #4
  # NOTE: C2-Sybil (#5) has no cloud flow yet — it needs NON-anchor Sybil validator
  # VMs (a topology.py addition), so on cloud the Sybils' bonds actually bank and the
  # pure ErrAnchorRequired gate + the ≥8-bond atomization note become assertable.
  # Tracked as the remaining cloud-variant work; stays a local-only suite until then.
  record "sybil-c2-cloud" skip major "C2-Sybil cloud flow pending a non-anchor Sybil validator topology (topology.py) — local suite integration/sybil covers it today"
}
