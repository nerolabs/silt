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
- `cmd/silt/ui.go:290-294` — the listener is plain `net.Listen("tcp", addr)`, and `ui.serve`
  starts `go http.Serve` BEFORE it returns the bound address. So the startup check must read
  the OPERATOR'S FLAG STRING, before `ui.serve` is called; a check on the returned address
  would refuse a daemon that is already listening. *(Corrected per the blind PE ruling S7;
  the first draft of this line said the bound address was the input, and the certification's
  §3.3(a) carries the same error — flag it when that document is next touched.)*
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
- `loadOrCreateUIToken` — the token lives at `<store>/ui-token`, WRITTEN mode 0600; the store
  directory itself is 0755 (`daemon.go` `MkdirAll(*storeDir, 0o755)`), so it is the FILE mode,
  not the directory, that carries the operator predicate — and `loadOrCreateUIToken` reads an
  existing file without checking its mode, which is why a startup mode check is part of the
  build (PE ruling S5). *(Corrected per S7: the first draft said "0600 store directory".)*
- `cmd/silt/ui/app.js:26-28` — the dashboard sends the token in the `Authorization` header;
  the `?token=` query form serves form POSTs and download links, never `/api/status`. So a
  header-only predicate for the block costs the operator nothing. *(Corrected per S7: the
  first draft said the dashboard depends on the query form.)*

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

**Residuals after (E), named (S8 adds the last two):**
- `R-BB-WITHHELD-IS-A-DISCLOSURE` — the marker publishes "the instrument is on" to an on-host
  untokened reader. Beside `R-BB-SUPPRESSED-IS-A-DISCLOSURE`. Revisit trigger: if the loopback
  refusal is ever relaxed, G-BB-13′ Part B reopens on the marker, and that is the owner's.
- The write credential enters the operator's routine measurement loop. Bounded by the
  header-only predicate (S3) and by documenting the `$(cat <store>/ui-token)` idiom in the flag
  help rather than the startup log line.
- An operator who deliberately configures a proxy to inject the token has published the block
  themselves; silt refused the routable bind and the operator went around it. Disclosed, not
  closed — no mechanism distinguishes that from the operator's own `curl`.
- F9 stands: the token is printed to stdout and rides the URL query for form POSTs and download
  links (the dashboard itself sends the header). A read-scoped token is the follow-on.
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

**The observatory under (E), so the next builder does not re-derive it.** The observatory
(`cmd/silt/ui/observatory.html`) is a cross-origin localhost reader that deliberately sends no
token (`app.js`), so it reads `bBootstrapWithheld: true`. That is correct — it never rendered
the block — and it does NOT pre-empt `D-UI-PRIVACY-FLAG`'s open question of whether the
observatory sends the token or requires `-privacy=off` on its targets.

**Blind PE ruling on this record:** PROCEED-WITH-CHANGES
(`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R2.9a-G-BB-12-design-2026-09-05.md`).
Option (E) upheld; eight changes, all built in the same PR: S1 the marker is a sibling key,
never a zero-valued block (sixteen false facts measured); S2 the withhold is a tag-split pair
applied to the COPY; S3 header-only token for the block; S4 the `D-BB-BUILD-TAG` scoping
correction; S5 the token-file mode refusal; S6 the shared fixture reads as the operator and
no content gate grew a "withheld is also acceptable" branch; S7 the three false claims above;
S8 the two residuals. Plus the coupling it named: ONE composition point, `uiServer.readerView`,
where `D-UI-PRIVACY-FLAG`'s clauses will land.

**Blind PE CODE ruling** (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R2.9a-G-BB-12-code-32adf76-2026-09-05.md`,
MERGE-AFTER one fix; all four wire claims measured on a live tagged daemon; six ablations RED).
Folded in: **Finding 1 (HIGH)** — a second untokened test helper made a REQUIRED anchor's
positive control pass on the marker (`"bBootstrap"` is a substring of `"bBootstrapWithheld"`);
the helper reads as the operator and the assertion names the quoted, colon-terminated block
key. **Finding 2 (MED, the PE's own miss in S5)** — the token-file check tested mode, not
ownership; a pre-planted 0600 token owned by another user was adopted and served the block.
An ownership clause (file uid = daemon euid) is added behind a pure predicate with a
fake-owner test, since an unprivileged test cannot chown. Findings 5, 6, 9 (a wrong
declaration count, a half-false comment, a self-contradiction here) corrected.
**Named, not built:** `R-BB-TOKEN-MODE-STARTUP-ONLY` (Finding 7 — a `chmod 0644` after start
reopens the file with no refusal; the check is startup-only by S5's own scope);
`readerView` composes `/api/status` only, while `/api/economy/self` withholds outside it
(Finding 4 — bites when `-privacy` lands; that build brings both documents under one
composition point); `loadOrCreateUIToken` rewrites an empty existing file without changing its
mode (Finding 8, untagged path, out of scope); a directory named `ui-token` fails safely but
confusingly (Finding 10). The loopback predicate is a string check on the flag's name, not a
resolution (Finding 3) — `localhost` is trusted by name, as the request guard already does.

**Status:** built; both PE rulings folded in; merged when CI is green.
