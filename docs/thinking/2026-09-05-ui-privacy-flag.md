# 2026-09-05 — `-privacy`: the node-wide counters and the library link key behind one operator flag

- **Date:** 2026-09-05 · **Seat:** BUILDER · **Branch:** `builder/r2.9a-privacy-flag`
- **Base:** `origin/main` = `965a594` (PR #743 merged)
- **Ratification this executes:** `docs/decisions.md` `D-UI-PRIVACY-FLAG` (owner, 2026-09-05).
  Decided there: ONE operator flag `-privacy` on the UI server; it governs whether an UNTOKENED
  reader gets the node-wide `stats.bytesServed`, `durability.balance` and `revenue.*` (on
  `GET /api/status` and `GET /api/economy/self`) and the `link` field of `GET /api/library`; a
  token-bearing reader always gets them; default EXPOSED during the flixz beta with a
  pre-release label on the wire; default WITHHELD at release; `-privacy=off` is the explicit
  opt-out. Left to this deliberation and a blind PE review: the default-flip mechanism, the
  wire form of the label, whether the observatory sends the token or needs `-privacy=off`, and
  the F9 point (the token is an unscoped write credential — a read-scoped token is a follow-on,
  not a precondition).

**Context / trigger.** Red-team F2: on a one-root node every node-wide counter is that root's
counter, so `stats.bytesServed` polled untokened is a named title's watch counter
(`R-BB-SIBLING-AGGREGATES`). And a `silt:v1:` link is retrieve-AND-decrypt forever
(`core/link/link.go:38-41`), served untokened by `apiLibrary` under the #89 read-only
exemption that was reasoned for counters, not capabilities.

**Evidence (per build-immutable #7):**
- `cmd/silt/ui.go` `readerView` (PR #743) — THE composition point for `GET /api/status`; the PE
  code ruling (`RULING-R2.9a-G-BB-12-code-32adf76-2026-09-05.md` Finding 4) requires
  `/api/economy/self` to join it when this build lands: today `apiEconomySelf` withholds via
  `withheldEconomySelf(out)` outside it (`ui.go:929`).
- `cmd/silt/ui.go` `statusInfo` — `Stats node.Stats` is a VALUE field (`json:"stats"`), so it
  cannot be omitted; `Durability *durabilityInfo` carries `Balance` and the F2 `DetailWithheld`
  marker; `economySelf.Revenue`/`Margin` are values.
- `cmd/silt/ui/index.html:67` — the dashboard's "served" card reads `stats.BytesServed`;
  `cmd/silt/ui/app.js:20-35` — the dashboard attaches `Authorization: Bearer` to every
  same-origin `/api/` call, so the operator's own dashboard is a TOKENED reader and loses
  nothing under any default.
- `cmd/silt/ui/observatory.html:83,109` — the observatory reads `r.status.stats.BytesServed`
  cross-origin with NO token (`app.js:5-6`, "cross-origin calls never receive this daemon's
  token"). It renders `fmtB(x || 0)`, so a withheld counter would display as **0 bytes served —
  a false zero**, the silent-loss shape Don't #4 forbids. The observatory must learn the marker.
- `cmd/silt/ui/library.html:59,68` — the library page uses `f.link` to open a file (`get`). The
  page is same-origin and tokened, so it keeps working under any default.
- `cmd/silt/client.go:186-195` — the `client` subcommand builds the same `uiServer` (with
  `links`, so `/api/library` is ITS surface as much as the daemon's); it has `-allow-web-origin`,
  so a hosted resolver on an allow-listed origin reads `/api/library` cross-origin and
  UNTOKENED today — the F6 shape applied to the link key. The flag must exist on BOTH
  subcommands.
- There is no version variable in `cmd/silt` (`main.go` declares none; `build.sh` passes only
  `-ldflags "-s -w"`), so no code can ask "am I a release build" today. `docs/release-checklist.md`
  §"Before the tag (maintainer)" is the one place a release step is enumerated.
- `D-DONT3-READING` T-DONT3 prong (b) REACH: a record "leaves the node by publication". These
  counters are aggregates, not who-fetched-what records; the owner took the one-root exposure
  knowingly for the beta (`D-UI-PRIVACY-FLAG`, "an owner call on a Don't #3 question").

**Options weighed — the default-flip mechanism (the open design question):**
- **(A) A build tag `release` that flips the default** — the `D-BB-BUILD-TAG` shape. Cost: a
  release binary built WITHOUT the tag silently ships the beta default into production, and
  nothing in `build.sh` passes tags; the failure is invisible. A build tag is the right tool
  when the MECHANISM must be absent; here the mechanism is present in both builds and only a
  default moves. Rejected: it converts a checklist item into a silent-wrong-default risk.
- **(B) An `-ldflags -X` variable set at release** — same silent failure (forget the flag, ship
  the beta default), plus a second build convention nobody runs locally. Rejected.
- **(C) ONE named constant, `privacyDefaultWithheld`, in ONE file, flipped by hand on the
  release checklist, with the wire carrying which default is in force** — the default is
  visible on every response (`privacy.mode`, `privacy.prerelease`), so a forgotten flip is
  VISIBLE in production rather than silent, which is the property the owner asked for
  ("labelled pre-release information"). A source gate pins the constant's single site and that
  `docs/release-checklist.md` names the flip. **Chosen.** Weakness, stated: it is a human step.
  Its mitigation is that the wire tells on it.

**The withhold shape (marker discipline, same as F2 and G-BB-12′):** a withheld field is
ABSENT with a sibling marker, never a zero.
- `GET /api/status`: `stats` becomes a pointer, omitted when withheld, with `statsWithheld: true`;
  `durability.balance` omitted with `balanceWithheld: true` (the block keeps `bountyOn`,
  `detailWithheld`); a new top-level `privacy` object on every response: `{"mode": "on"|"off",
  "default": "on"|"off", "prerelease": bool}` so a reader can tell "withheld by policy" from
  "withheld because you are not the operator" from "exposed because this is a beta".
- `GET /api/economy/self`: `revenue` and `margin` omitted with `revenueWithheld: true`; `wash`
  stays (a shape self-check with no counter — verify: it carries `symmetry`, a RATIO of the two
  withheld byte counts, so it leaks their quotient; withhold it too). `selfFunding` and
  `objects` already follow the F2 token.
- `GET /api/library`: each row omits `link` with `linkWithheld: true`; `root`, `label`, sizes
  stay (they name what you hold, not the key to open it).
- All three under ONE composition: `readerView` gains the privacy clause for `/api/status`;
  `apiEconomySelf` routes through a sibling `economyView(full, auth)` in the same place with
  the same `readerAuth`; `apiLibrary` through `libraryView(rows, auth)`. One `readerAuth` per
  request carries `token`, `tokenHeader` and `privacy` (the process flag), so every clause reads
  the same three facts.
- Which token unlocks it: `readerAuth.token` (any route), matching the F2 detail it sits
  beside — these are counters, not the census, and the dashboard's own form/download paths use
  the query form. The header-only predicate stays reserved for the histogram.

**The observatory.** It must not paint a false zero. `observatory.html` renders the served
column and the bandwidth card as "withheld" when `statsWithheld` is set, and the summary omits
those daemons from the bandwidth sum with a count of how many were withheld. Whether it should
instead SEND a token is the open question `D-UI-PRIVACY-FLAG` named; this build does not decide
it (a cross-origin token would have to be one token per target daemon, typed in by the operator
— a UX change, not a flag). Under the beta default it reads everything as before.

**Decision + rationale:** (C) for the flip; the marker shape above; one composition point over
three documents. It is the only option under which a wrong default is visible on the wire
rather than silent, and it is the smallest change that puts every withhold on this surface
behind one `readerAuth`.

**Residuals, named:**
- `R-PRIVACY-DEFAULT-IS-A-CHECKLIST-STEP` — the release flip is a human step; the wire label is
  the detector, the checklist the instruction, a source gate the pin on the single site.
- The `-privacy` process flag is per-daemon: a fleet of one-root ponies each defaulting to
  exposed during the beta is the exposure the owner accepted.
- The token that unlocks these counters is the write credential (F9); a read-scoped token
  remains the follow-on.

**What would change my mind:** the PE ruling that the default must be a build-time property
(then (A) with a `build.sh` that always passes the tag and a CI check that a tagged and untagged
binary differ in the default), or that the observatory must send a token now.

**Gates planned (untagged — this is default-build code):** the three documents' marker shapes
under `privacy=on` untokened (absent + marker, never a zero); tokened reads unchanged; the
default constant's single site and the checklist line (source gate, annotated); `economySelf`
and `library` route through the composition functions (source gate); the observatory HTML
contains the `statsWithheld` branch (static gate); the flag exists on both subcommands (source
gate); the `wash.symmetry` quotient is withheld with its operands.

**Blind PE ruling on this record** (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-UI-PRIVACY-FLAG-design-2026-09-05.md`,
PROCEED-WITH-CHANGES). Three of my premises were FALSE on the tree and are corrected here:
- **S1 — the observatory did not paint a false zero; it THREW.** The `try` wrapped only the
  fetches; `r.status.stats.BytesServed` on an omitted block is a TypeError that aborts the
  render for every daemon in the list, silently. Same shape on the dashboard behind a
  comment-only `catch`. Fix: the render logic moved to `cmd/silt/ui/render.js` (pure
  functions), both pages route through it, and the gate is BEHAVIOURAL — a Go test runs the
  functions under node against a fixture with a withheld daemon.
- **S2 — the operator's dashboard is NOT reliably tokened.** `app.js` keeps the token in
  `sessionStorage` (per tab); a fresh tab is untokened. The untokened operator is a first-class
  reader: the served card says "withheld" and names the recovery. Residual
  `R-PRIVACY-OPERATOR-TAB-TOKEN` named.
- **S4 — (B) was rejected on a fail-open spelling and (E) was never weighed.** Built as **(E): no
  flip.** The compiled default is WITHHELD in every build; the flixz beta nodes run
  `-privacy=off` and are labelled. This honours the owner's guarantee sentence literally and
  makes a wrong shipped default impossible rather than visible; `release.yml` additionally
  asserts the default on the built linux artifact. **It changes a ratified default** ("default
  ON through the BETA") — flagged for the owner in `D-UI-PRIVACY-FLAG`'s appended note; the
  one-line site is `privacyDefaultWithheld` in `cmd/silt/ui.go`.
- S3: the self-document privacy view is a second ALLOW-LIST composed after the F2 one, never a
  set of nils. S5: `-privacy` is a string flag accepting `on|off`, anything else refuses at
  startup (a bool rejects `-privacy=off` at parse). S6: the WHOLE `stats` block is withheld, on
  purpose and over the decision's letter — `ChunksServed × chunk size` reconstructs
  `BytesServed`. S7: `Balance`, `Revenue`, `Margin`, `Wash` are pointers; ONE marker per
  document, `countersWithheld`, with its covered set enumerated on the field. S8: the
  pre-release label has a human surface — a banner on both pages when `privacy.mode` is
  `off`. S9: the link clause is live on `silt client` only (`links` is nil on a plain daemon);
  an OLD observatory page pointed at a NEW `privacy=on` daemon still throws — CHANGELOG
  compatibility note. S10: the library types are hoisted and the response is typed.
- Predicate split (Q4): counters take any-route `token`; the link takes header-only
  `tokenHeader`. Rule recorded in the composition-point doc comment.

**Blind PE CODE ruling** (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-UI-PRIVACY-FLAG-code-0c9e373-2026-09-05.md`,
MERGE-AFTER; seven of eight controlled reverts RED; the wire claims measured on a live daemon
and a live client). Folded in: **B1** the e2e F2 arm was RED at the first push (`go test -short
./...` skips e2e — the whole e2e suite now runs locally before a push; the arm runs its daemon
`-privacy=off` and a new live-daemon gate covers the default). **B2** `library.html` had no
reader for `linksWithheld`: an untokened tab rendered `data-link="undefined"` and the get
button sent `/api/fetch?link=undefined` — the action cell is rendered by `render.js`, the
page shows the recovery, and the node gate covers it (my S2 miss, and the PE's: "both pages"
should have been "every page that reads a withheld field"). **B3** `parsePrivacyFlag` sat
inside the `-ui` block, so `silt daemon -privacy=bogus` with no UI booted; moved before it,
source gate widened. **M1** the pointer-sharing hazard is gated (an untokened read followed by
the operator's read in one interval). **M2** three texts still called
`R-BB-SIBLING-AGGREGATES` open. **M3** the privacy gates are name-anchored in the untagged CI
loop. **M4** `R-PRIVACY-OPERATOR-TAB-TOKEN` is on the ROADMAP. **Contingent, the PE's verdict
and mine:** whether flixz reads `/api/library` cross-origin from a hosted resolver, which the
default would break — asked in the handoff.

**Status:** built; both PE rulings folded in; merged when CI is green.
