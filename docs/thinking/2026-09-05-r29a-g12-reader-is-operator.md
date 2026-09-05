# 2026-09-05 — G-BB-12′: how the code establishes that the histogram's reader is the operator

- **Date:** 2026-09-05 · **Seat:** BUILDER · **Branch:** `builder/r2.9a-g12-reader-is-operator`
- **Base:** `origin/main` = `9bfad86` (PR #742 merged)
- **Status of the two questions this touches:** G-BB-13′ Part A is RATIFIED (owner, 2026-09-05:
  *"refuse at startup"*; `docs/decisions.md` `D-R2.9a-RUN-CALLS` item 4). G-BB-12′ — the
  MECHANISM by which the code establishes the reader is the operator — is a builder gate the
  seats did not converge on. This record is the deliberation the ratification asked for, and it
  goes to a blind PE review BEFORE the mechanism half is coded.

**Context / trigger.** The RE-CERT (`R2.9a-minR-floor-RECERT-…-2026-09-05.md` §3.3) upgraded
G-BB-12 from a deployment note to a code requirement, G-BB-12′: *"The `bBootstrap` block may
be published only when the code has established that the reader is the operator,"* and named
two sufficient forms — (a) refuse `-bbootstrap` at startup unless `-ui` is bound to loopback,
(b) require the API token for the block on any bind — while declining to pin the mechanism.
The Red-team then showed (a) is not access control.

**Evidence (per build-immutable #7 — artifacts, not vibes):**
- `cmd/silt/ui.go:304-313` — `guard` checks the client-controlled `Host` header; `:395-411`
  `isLocalHost` never reads the connection's remote address; `:343` the token is required on
  MUTATING methods only. `GET /api/status` is untokened.
- `cmd/silt/ui.go:290-293` — the listener is plain `net.Listen("tcp", addr)`; the bound
  address is returned to `daemon.go:1285`, so a startup check has its input before serving.
- `cmd/silt/daemon.go:431-433` — the precedent shape: `-dht-address-reserve` is REFUSED at
  startup with a message naming the constraint.
- Red-team F5 (`TestRT_COMPOSE4_ReverseProxyHostPassesTheLoopbackGuard`): nginx's default
  `proxy_set_header Host $proxy_host` forwards a loopback `Host`; a daemon bound to loopback
  behind a proxy on `0.0.0.0:443` serves the full block. Same class: `ssh -L`,
  `kubectl port-forward`, `socat`, iptables REDIRECT — all arrive FROM loopback.
- Red-team F6: `-allow-web-origin` reflects an allow-listed origin with no token. **Verified
  on the tree: that flag exists only on the `client` subcommand (`cmd/silt/client.go:63,193`),
  the daemon registers no such flag, and `client.go` calls neither `registerBBootstrapFlag`
  nor `bbootstrapWireUI`.** On a daemon F6 is unreachable today; it stays that way only if a
  gate says so.
- Red-team F9: the token is one unscoped secret that also authorises publish/fund/library
  mutations, is printed to stdout at start (`daemon.go:1291`), and rides the URL query.
  Sharing it with a REMOTE analyst is a privilege escalation.
- Crypto-specialist §3.1: Tor's `MetricsPortPolicy` is a per-connection remote-address
  policy, reject-all by default — and Tor's own man page names its residual: *"allowing
  localhost, every user on the server will be able to access it."* §5 table: geth keeps
  `admin`/`debug` IPC-only (filesystem-permissioned); Bitcoin Core requires a cookie
  credential EVEN on loopback.
- Economist §1.2–1.5: no measurand needs a non-operator reader; the operator scrapes locally
  (`curl localhost`, `docker exec`, `ssh -L`); the G-BB-18 pad screen is a POLLED series and
  is safe only when the poller is the operator.
- `cmd/silt/ui.go:550-558` — the F2 pattern already in the tree: ONE cached document, the
  withheld view applied to a copy at serve time, keyed on `validToken(r)`. Absent-vs-empty is
  handled by a `detailWithheld` marker, never by a missing key.
- `loadOrCreateUIToken` — the token lives at `<store>/ui-token`, mode 0600, owner-readable.

**The question, stated exactly.** "The reader is the operator" is a claim about a PERSON's
relationship to the node, and no network-position check can establish it: a loopback bind
cannot tell the operator's `curl` from a proxy the operator (or a co-tenant) installed, and a
remote-address policy on a loopback bind sees only loopback. The only things a node can check
that correlate with "is the operator" are (i) possession of something in the operator's 0600
store directory, or (ii) filesystem permission itself.

**Options weighed:**
- **(A) Startup bind refusal alone** — cert form (a); Economist and PE favour it. Closes the
  remote scraper (F11) and makes the token never need to travel. Leaves: a local proxy or
  port-forward (F5), any process or user on the host, any `http://localhost:*` page (the guard
  REFLECTS every localhost origin so the observatory works). Cheapest. Owner-ratified as the
  Part A answer, so it ships regardless of what else does.
- **(B) Token on the block alone** — cert form (b). Closes F5 (a proxy does not inject the
  token unless the operator configures it to), F6, the localhost-origin page, and the
  co-tenant who cannot read the 0600 token file. Leaves: the token TRAVELS if the analyst is
  remote (F9), and it is the write credential. Alone it invites a remote-scrape posture.
- **(C) Per-connection remote-address policy (Tor `MetricsPortPolicy`)** — the
  Crypto-specialist's prior-art family. On a loopback bind it adds nothing over (A): every
  connection is loopback. Its value is on a ROUTABLE bind, which Part A just refused. Its
  residual is Tor's own: every local user. Rejected for this run; the shape to reach for if
  a fleet-scrape posture is ever ratified.
- **(D) No HTTP publication: write the block to `<store>/bbootstrap.json` (0600) on the
  snapshot interval** — Tor writes its stats to `DataDirectory/stats/`; geth's sensitive
  namespaces are IPC-only. Strongest: the reader IS whoever can read the operator's directory,
  by the OS. Cost: the whole tagged `/api/status` surface (renderer, BB-5, BB-20, BB-21 and
  their fixtures, 16 call sites) moves to a file writer, for a one-shot instrument the owner
  has just re-scoped. Rejected on cost for this run; recorded as the shape with the best
  prior-art fidelity.
- **(E) (A) + (B) composed — RECOMMENDED.** The refusal removes the network exposure and the
  reason for the token to travel; the token then establishes the one checkable correlate of
  "operator": the reader can read `<store>/ui-token`, mode 0600. The two forms cover each
  other's named gap — (A)'s proxy/co-tenant/origin gap is closed by the token, (B)'s
  travelling-token gap is closed by the refusal. Both are forms the certification named as
  sufficient; composing them is not a new mechanism. The operator's scrape is one header:
  `curl -H "Authorization: Bearer $(cat d1/ui-token)" http://127.0.0.1:8081/api/status`.
  **Absent-vs-empty holds:** an untokened reader on a tagged, flag-on node gets
  `"bBootstrap": {"withheld": true}` — three states stay distinguishable on the wire:
  no key (instrument off or default build), `withheld` (on; you are not the operator), the
  block (on; you are). Same ONE-cache/two-views shape as F2, so the cache stays uncounted by
  the reader's authentication.

**Decision + rationale:** (E). It is the only option under which every reader the Red-team
named — remote, proxied, cross-origin, co-tenant — is refused by something the node can
actually check, without adding a second secret (a read-scoped token would be a third thing to
rotate and is the follow-on F9 asks for, not a precondition). (A) alone was ratified for Part
A and ships in either case; the PE rules on whether (B) rides with it.

**Residuals after (E), named:**
- An operator who deliberately configures a proxy to inject the token has published the block
  themselves; silt refused the routable bind and the operator went around it. Disclosed, not
  closed — no mechanism distinguishes that from the operator's own `curl`.
- F9 stands: the token is printed to stdout and rides the URL query (pre-existing operator UX
  the dashboard depends on). A read-scoped token is the follow-on.
- A co-tenant running AS the operator's OS user is the operator for every purpose silt can see.

**What would change my mind:** the PE ruling that (B) must not ride (E) — then (A) ships alone
with the proxy/co-tenant gap disclosed in the flag text; or a ruling that only (D) satisfies
"the code establishes the reader is the operator", which re-scopes this to a file writer.

**Gates planned (tagged, `cmd/silt`):** the startup refusal on routable, wildcard, empty-host
and LAN binds and its acceptance of loopback literals and `localhost`; a source gate that
`daemon.go` runs the check BEFORE `ui.serve`; the three wire states (absent / withheld /
block); the F5 shape (upstream loopback `Host`, no token) reads `withheld`; a localhost-origin
cross-origin GET reads `withheld`; the F6 source gate (`client.go` wires no instrument). All
added to the tagged CI anchor list in the same PR (the four-residuals ruling's unanchored-gate
residual, materialised once already on #742).

**Status:** proposed — to blind PE review before the (B) half is coded; the (A) half is
owner-ratified and is built in parallel.
