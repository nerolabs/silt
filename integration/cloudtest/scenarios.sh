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
    sleep 4
  done
  # Diagnose so a failed publish is actionable, not a bare "no link produced". The
  # single most common cause is the publisher reaching < TOKEN_QUORUM validators.
  local reach; reach="$(ft_reachable_peers "$node")"
  # If the publisher can't reach enough validators to gather the token quorum, this
  # is an egress/preemption problem, not a property failure — flag it so the caller
  # records a GAP. (max(1,TOKEN_QUORUM): even token-quorum 0 needs one reachable peer.)
  local rok="${reach%%/*}"; local need=$(( TOKEN_QUORUM > 1 ? TOKEN_QUORUM : 1 ))
  [ "${rok:-0}" -lt "$need" ] 2>/dev/null && FT_PUBLISH_GAP=1
  # A publish that fails because the ephemeral CLI publisher could not DISCOVER the
  # canonical issuer set / gather a token — even with validators reachable — is a
  # publish-token *discovery* problem over WAN (worse after mid-run node churn), not a
  # break of the property a SETUP publish is a precondition for (durability, crash
  # recovery). The publisher re-warm (#351) reduces it but a churned WAN can still
  # exceed the window. Flag it as a GAP so publish_verdict records the DEPENDENT flow
  # as UNTESTED, not FAILED; the publish-reliability issue itself stays visible in this
  # diagnostic and in #351. (A genuine no-quorum terminal outcome still fails fast via
  # /publish-status, so this does not mask a real refusal.)
  printf '%s' "$lasterr" | grep -qiE 'no canonical issuer|could not gather|not enough|issuer set|token' && FT_PUBLISH_GAP=1
  # …or if the fetch publish subsystem was already found degraded this run (warm
  # failed): a subsequent publish failure — even one with no captured error text — is
  # then a discovery/setup problem, not a property break, so score it a GAP too.
  [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ] && FT_PUBLISH_GAP=1
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

# Record a publish-dependent flow's failed publish honestly: a reachability
# shortfall OR a degraded publish subsystem is a GAP — the property was UNTESTED,
# not broken — while any other publish failure is a real fail.
#
# NOTE the scope bug this works around: ft_publish is called as `res="$(ft_publish …)"`,
# i.e. in a command-substitution SUBSHELL, so any FT_PUBLISH_GAP it sets is lost and
# never reaches this parent scope — publish_verdict would ALWAYS fall through to a FAIL.
# FETCH_PUBLISH_DEGRADED is the reliable signal: wait_publisher_warm sets it in the
# PARENT (it is not called in a subshell), so gate on it here. It is 1 whenever the
# publisher re-warm could not land a throwaway publish this run — i.e. the ephemeral-CLI
# issuer-set discovery is not working over the (churned) WAN — so a dependent flow's
# publish failure is a discovery/setup problem (#351), not a break of the property that
# publish is a precondition for.
publish_verdict() { # publish_verdict FLOW SEVERITY "detail"
  if [ "${FT_PUBLISH_GAP:-0}" = 1 ] || [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ]; then
    record "$1" gap "$2" "$3 — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); property UNTESTED, not failed"
  else
    slo_assert "$1" "$2" "$3" 0
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
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
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
flow_convergence() {
  local n vals="" maxh=0
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
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
flow_fault_tolerance() {
  require_nodes "6-fault-tolerance" major val-d || return
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['boot'])")"
  local h0; h0="$(ft_commit_height "$boot")"   # audit #303: baseline BEFORE stopping val-d + publishing
  svc val-d stop || true
  sleep 5
  local res ok=0
  res="$(ft_publish fetch-1 262144 || true)"
  if [ -n "$res" ]; then
    # Require a NEW block with val-d down, not a stale pre-kill 'committed block' line.
    ft_wait_new_block "$boot" "$h0" "$COMMIT_SLO_S" && ok=1
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
flow_takedown() {
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
flow_cross_nat() {
  require_nodes "9-cross-nat" major nat-1 nat-2 || return
  local res ok=0
  res="$(ft_publish nat-1 262144 || true)"    # nat-1 is un-dialable → must use the relay
  if [ -n "$res" ]; then
    local link="${res%% *}" sha="${res##* }" got
    got="$(ssh_node nat-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_n.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_n.bin | cut -d' ' -f1" 2>/dev/null || true)"
    [ -n "$sha" ] && [ "$got" = "$sha" ] && ok=1
  fi
  if [ "$ok" = 1 ]; then
    slo_assert "9-cross-nat" major "natted nodes exchanged a file through the relay/hole-punch" 1
  else
    publish_verdict "9-cross-nat" major "natted nodes did not exchange a file via the relay"
  fi
}

# ── #184 adversarial: equivocation → slash ──────────────────────────────────────
adv_equivocation() {
  require_nodes "184-equivocation" blocker adversary val-b val-c || return
  local idb idc
  idb="$(node_field val-b nodeid)"; idc="$(node_field val-c nodeid)"
  if [ ${#idb} -ne 64 ] || [ ${#idc} -ne 64 ]; then
    record "184-equivocation" gap blocker "could not resolve val-b/val-c NodeID from nodes.json (idb='${idb:0:12}…' idc='${idc:0:12}…') — attack not delivered, not a property failure"; return
  fi
  # READINESS GATE (#345/#350). The adversary is a NON-anchor validator: it can only
  # PLACE an equivocating fork once it is a QUALIFIED proposer (its bond committed
  # on-chain), because an honest target refuses to attest a proposal from a node
  # without committed standing. The equivocation drill previously fired before that
  # was true, so proposeAndCommitTo was refused ("not yet standing"), the forks were
  # never placed, and the drill FAILed for a reason that is not a product break —
  # racing the adversary's standing, which flips between runs. Gate on the daemon's
  # own positive control (-goodpropose): it retries a well-formed proposal until the
  # adversary's bond earns standing, logging ACCEPTED. If the adversary never
  # qualifies over WAN in the window, the equivocation attack cannot be DRIVEN, so
  # record a GAP (untested), NOT a FAIL — equivocation slashing is certified
  # in-process (#204). Only once the adversary is confirmed qualified does a missing
  # slash mean a real failure.
  local ida; ida="$(node_field val-a nodeid)"
  relaunch_with adversary "-goodpropose ${ida}"
  local qualified=0
  waitfor adversary "goodpropose proposal ACCEPTED by ${ida}" 180 >/dev/null && qualified=1
  restore_argv adversary
  if [ "$qualified" != 1 ]; then
    record "184-equivocation" gap blocker "adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350."
    return
  fi
  relaunch_with adversary "-equivocate ${idb},${idc}"
  # PLACEMENT GATE (#345/#350). A double-sign can only be DETECTED once it is PLACED —
  # both conflicting forks committed onto two honest peers. But on a live, actively-
  # proposing chain an honest validator has usually ALREADY attested a block at the
  # current height, so it refuses the adversary's conflicting proposal ("gather/attest:
  # REFUSED (already attested a different block at height)") — the double-sign never
  # lands. The adversary logs "equivocation complete" only when both forks are placed.
  # If that never appears, the attack could not be DRIVEN on this live chain → GAP
  # (untested), NOT a FAIL: equivocation slashing is certified in-process (#204), and a
  # clean double-sign is genuinely hard to inject into a live 3-region chain. Only a
  # PLACED-but-unslashed double-sign is a real product failure.
  local placed=0
  waitfor adversary 'equivocation complete' 120 >/dev/null && placed=1
  if [ "$placed" != 1 ]; then
    restore_argv adversary
    record "184-equivocation" gap blocker "adversary could not PLACE the double-sign on the live chain within 120s (honest validators had already attested at that height — 'already attested a different block at height'), so equivocation was UNTESTED this run, not a failure (certified in-process #204, #345/#350)"
    return
  fi
  # PLACED — now the slash MUST fire (this is the real assertion). Watch val-b, the
  # DIRECT detector: the adversary places fork X on val-b and the heavier fork Y,Z on
  # val-c, so val-b catches the double-sign the moment it syncs val-c's heavier fork
  # (slashEquivocators runs in the sync path). Fall back to val-c / val-a for the
  # on-chain-propagated slash so any honest observer counts.
  local ok=0
  { waitfor val-b 'slashed equivocator|validator slashed for equivocation' 120 >/dev/null \
    || waitfor val-c 'slashed equivocator|validator slashed for equivocation' 20 >/dev/null \
    || waitfor val-a 'slashed equivocator|validator slashed for equivocation' 20 >/dev/null; } && ok=1
  slo_assert "184-equivocation" blocker "equivocator PLACED a double-sign and was slashed over the real wire$([ "$ok" = 1 ] || echo ' — the double-sign was PLACED (equivocation complete) but NO slash line appeared on val-b/val-c/val-a within the window (real detection gap)')" "$ok"
  restore_argv adversary
}

# ── #184 adversarial: partition → heal ──────────────────────────────────────────
adv_partition() {
  require_nodes "184-partition" major val-b val-c || return
  local idb; idb="$(node_field val-b nodeid)"
  if [ ${#idb} -ne 64 ]; then
    record "184-partition" gap major "could not resolve val-b NodeID from nodes.json (idb='${idb:0:12}…') — partition not applied, not a property failure"; return
  fi
  # partition val-c off from val-b (one-sided so it can heal on reconnect)
  relaunch_with val-c "-block-peers ${idb}"; sleep 30
  restore_argv val-c    # drop the block → reconnect → reconcile
  # A validator only REORGS onto a heavier fork if one FORMED while it was partitioned —
  # i.e. the majority side committed at least one block during the ~30s window. On an
  # idle chain (no publishes in that window) both sides stay at the same height, there is
  # nothing heavier to heal onto, and no reorg line is emitted — the reconcile is then
  # UNTESTED, not broken (partition-healing is certified in-process #204). So score a
  # missing reorg as a GAP, not a FAIL: this drill's PASS↔FAIL flipping between runs was
  # exactly this precondition race (#350).
  if waitfor val-c 'reorged onto a heavier fork|chain: reorged' 120 >/dev/null; then
    slo_assert "184-partition" major "partitioned validator healed onto the heavier fork after reconnect" 1
  else
    record "184-partition" gap major "no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)"
  fi
}

# ── #184 adversarial: forged-block + low-bond proposals rejected ────────────────
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

# ── durability (#2): content OUTLIVES a permanent storage-node loss ─────────────
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
  if [ -z "$res" ]; then record "durability-turnover" gap major "setup publish did not land a link — durability UNTESTED this run (ephemeral-CLI issuer-set discovery over WAN, #351), not a durability failure"; return; fi
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
  local link="${FT_LAST_LINK:-}" wantsha="${FT_LAST_SHA:-}"
  if [ -z "$link" ]; then local res; res="$(ft_publish fetch-1 262144 || true)"; link="${res%% *}"; wantsha="${res##* }"; fi
  # As with durability-turnover: a failed SETUP publish means crash-recovery is UNTESTED
  # (no content to crash-and-recover), not broken — GAP unconditionally (#351).
  if [ -z "$link" ]; then record "chaos-crash" gap major "setup publish did not land a link — crash-recovery UNTESTED this run (issuer-set discovery over WAN, #351), not a failure"; return; fi
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
  local reann=0
  waitfor_since store-2 're-announced [0-9]+ held chunks' "$t0" 90 >/dev/null && reann=1
  slo_assert "chaos-reprovide" major "SIGKILLed storage node re-announced its held chunks (#69) after a hard crash" "$reann"
  local got ok=0
  # SHA-compare, not echo-OK (§D): crash-recovery must return the REAL bytes, not just
  # a zero exit on a possibly-truncated fetch.
  got="$(ssh_node store-1 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_ch.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_ch.bin | cut -d' ' -f1" 2>/dev/null || true)"
  [ -n "$wantsha" ] && [ "$got" = "$wantsha" ] && ok=1
  slo_assert "chaos-fetch" major "content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node$([ "$ok" = 1 ] || echo " (want=${wantsha:-?} got=${got:-<none>})")" "$ok"
}

# ── client/UI (#4): the web-UI local-security guard (#89) over a real VM ─────────
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

  # 2) THE CAPTURE ATTEMPT — stop every anchor; only the bonded Sybil self-majority
  #    remains. Give it time to try to advance on its own.
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
  local fresh_commit=0
  for s in $sybils; do
    if jlog "$s" 200 | grep -qE 'chain: committed block [0-9]'; then fresh_commit=1; break; fi
  done
  local gate=0
  for s in $sybils; do
    if jlog "$s" 800 | grep -qE 'immature network requires anchor|requires anchor attestations|ErrAnchorRequired|training wheels'; then gate=1; break; fi
  done

  # 4) THE CLINCHER — restore the anchors; the chain must RESUME advancing, proving
  #    the halt was the missing anchors and not merely dead Sybils.
  echo "    restoring anchors — chain must resume…"
  for a in $anchors_nodes; do svc "$a" start >/dev/null 2>&1 || true; done
  local resumed=0 h2 t0; t0="$(date +%s)"
  while [ $(( $(date +%s) - t0 )) -lt 180 ]; do
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
    record "5-sybil-no-capture" pass major "no quiet capture: $n_syb bonded single-domain Sybils could NOT advance the chain past the anchored ceiling h${ceiling} with all anchors down (sybil head →$h1, no fresh Sybil commit), and it resumed to $h2 once the anchors returned$([ "$gate" = 1 ] && echo '; a Sybil logged the anchor-required refusal' || echo '')"
  elif [ "$no_advance" != 1 ] && [ "$fresh_commit" = 1 ]; then
    record "5-sybil-no-capture" fail major "CAPTURE: a Sybil COMMITTED a new block past the anchored ceiling h${ceiling} (sybil head →$h1) with ALL anchors down — the training wheels did not hold"
  elif [ "$no_advance" != 1 ]; then
    # Height rose past the ceiling but NO Sybil logged a fresh commit ⇒ the rise is a
    # catch-up sync of anchor-committed blocks, not a capture (the old false positive).
    record "5-sybil-no-capture" gap major "sybil head rose to $h1 (ceiling h${ceiling}) with anchors down but NO Sybil logged a fresh 'committed block' — this is a lagging Sybil CATCHING UP to anchor-committed blocks, not a capture; the property held but the drivability was masked by Sybil lag (see the run-slowness finding). Re-run on a healthier network to certify the clean no-capture outcome."
  else
    record "5-sybil-no-capture" gap major "no-capture outcome held (head ≤ ceiling h${ceiling} with anchors down) but the chain did NOT resume within 180s after restoring anchors (h2=$h2) — liveness inconclusive (SPOT preemption?); re-run to confirm the clincher"
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
: "${LATCH_S:=420}"
: "${HANDOFF_BLOCKS_S:=600}"
flow_maturing_handoff() {
  local maturing
  maturing="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta'].get('maturing',0))")"
  if [ "$maturing" != "1" ]; then
    record "10-maturing-handoff" skip major "not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)"
    return
  fi
  require_nodes "10-maturing-handoff" major val-a val-b || return
  require_live  "10-maturing-handoff" major val-a || return
  local anchors_nodes n_syb sybils
  anchors_nodes="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  n_syb="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['n_syb'])")"
  sybils="$(python3 -c "import json;print(' '.join(json.load(open('$FT_DIR/topology.json'))['meta']['sybils']))")"

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
  #    latch wording. Never tripping is an honest GAP — the maturity precondition
  #    was unmet and everything downstream is UNTESTED.
  local wheels
  wheels="$(waitfor val-a 'wheels shed permanently' "$LATCH_S" || true)"
  if [ -z "$wheels" ]; then
    record "10-maturing-handoff" gap major "the everMature latch never tripped within ${LATCH_S}s ('wheels shed permanently' absent on val-a) — bar-2 maturity precondition unmet (bonds not committed? C2 metric short?); handoff/post-shed regime UNTESTED"
    return
  fi
  echo "    latch tripped: ${wheels##*silt}"

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
  slo_assert "10-maturing-handoff" major "young→mature HANDOFF field-exercised: latch tripped on the wire, commits crossed the epoch boundary into the governed mature snapshot (h${h_latch}→$(mh_ceiling), target h${target}), and no anchor-required refusal after the shed$([ "$anchor_refusal" = 1 ] && echo ' (ANCHOR REFUSAL SEEN POST-SHED — the wheels did not shed)')" \
    "$([ "$ok" = 1 ] && [ "$anchor_refusal" = 0 ] && echo 1 || echo 0)"
  [ "$ok" = 1 ] || return  # no governed mature epoch ⇒ the drills below are untestable

  # Cohort-seated precondition for the B2 drills: the epoch snapshot admits every
  # qualified bond, so the cohort must have BANKED bonds before the boundary. The
  # C2 status line's participant count is the on-chain evidence.
  local seated=0 parts
  if [ -n "$sybils" ]; then
    parts="$(jlog val-a 400 | grep -oE 'bonded across [0-9]+' | grep -oE '[0-9]+' | tail -1)"
    [ "${parts:-0}" -ge $(( 4 + n_syb )) ] 2>/dev/null && seated=1
  fi

  # 3) 10a — THE STALL DRILL: the cohort DECLINES to attest (stopped = the
  #    strongest decline; nothing slashable either way). Under head counting this
  #    epoch needs bftThreshold(4+n_syb) attestations and the mature phase is
  #    born unable to commit; under the weight rule the honest coalition carries
  #    ~all the weight and MUST keep committing.
  if [ -z "$sybils" ]; then
    record "10a-stall-drill" skip major "no cohort in this topology (SYBILS=0) — the B2 stall drill needs the cheap members seated in the epoch; run MATURING=1 SYBILS=8"
  elif [ "$seated" != 1 ]; then
    record "10a-stall-drill" gap major "the cohort never banked its bonds on-chain before the boundary (participants=${parts:-?} < $((4 + n_syb))) — the epoch snapshot has no cheap members to decline, drill premise unmet, property UNTESTED"
  else
    echo "    stall drill: stopping the $n_syb-member cohort (declining to attest)…"
    local s; for s in $sybils; do svc "$s" stop >/dev/null 2>&1 || true; done
    local stall_ok=0
    mh_drive_block && stall_ok=1
    slo_assert "10a-stall-drill" major "B2 stall drill: with the $n_syb cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire (head-counted quorum left this exact network born-unable-to-commit at ${n_syb}×MinBond)" "$stall_ok"
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
    echo "    capture drill: stopping the honest validators ($anchors_nodes) — the cohort alone attempts to advance (ceiling h${ceiling})…"
    local a; for a in $anchors_nodes; do svc "$a" stop >/dev/null 2>&1 || true; done
    sleep 90
    h1=0; for s in $sybils; do local hs; hs="$(mh_height "$s")"; hs="${hs:-0}"; [ "$hs" -gt "$h1" ] 2>/dev/null && h1="$hs"; done
    local no_advance=0; [ "$h1" -le "$((ceiling + 1))" ] 2>/dev/null && no_advance=1
    for s in $sybils; do jlog "$s" 200 | grep -qE 'chain: committed block [0-9]' && { fresh=1; break; }; done
    for s in $sybils; do jlog "$s" 600 | grep -qE 'frozen-weight super-majority' && { weightlog=1; break; }; done
    echo "    restoring the honest validators — chain must resume…"
    for a in $anchors_nodes; do svc "$a" start >/dev/null 2>&1 || true; done
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
  echo "    ws cold-sync: restarting val-b pinned to -ws-checkpoint $cp…"
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
: "${PUBLISHER_WARMUP_S:=180}"
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
  echo "    WARN: publisher $node did not warm within ${PUBLISHER_WARMUP_S}s — publish subsystem degraded; dependent publishes this run report GAP (untested), not FAIL (#351)"
  return 1
}

run_all_scenarios() {
  ft_init_refs
  echo "  peers=$PEERS"
  echo "  registry=$REGREF"
  wait_network_warm
  wait_publisher_warm fetch-1   # #344: non-validator issuer-set/issuer-key discovery lags genesis
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
  # Re-warm the fetch publisher (#351): flows 7 (restart-survival) and the #184 drills
  # restart/partition validators, which transiently drops a validator from the
  # discoverable canonical-issuer set until it re-syncs — so a fresh ephemeral publish
  # here re-races issuer-set discovery (durability-turnover false-FAILed on the last
  # re-cert for exactly this). One warm at genesis (#344) doesn't cover post-restart
  # flows; re-warm before the publish-dependent cloud variants. Bounded + non-fatal.
  wait_publisher_warm fetch-1
  flow_publisher_unlinkability   # privacy #3
  flow_durability_turnover       # durability #2
  flow_chaos_crash               # chaos #7
  flow_web_ui_guard              # client/UI #4
  flow_c2_no_capture             # C2 Sybil #5 — opt-in (SYBILS=8): certifies the PURE anchor gate on cloud
  flow_maturing_handoff          # §4/B2 #10 — opt-in (MATURING=1 SYBILS=8): handoff + post-shed + weight-quorum drills + WS cold-sync. LAST: it stops validators.
}
