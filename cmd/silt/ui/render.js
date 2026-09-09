// render.js — the PURE render functions behind the dashboard (index.html) and the
// observatory (observatory.html). They take decoded /api/status documents and return
// strings; they touch no DOM, so a Go test can run them under node against a fixture
// (cmd/silt/ui_privacy_test.go) and prove the pages never throw on a withheld document.
//
// WHY THIS FILE EXISTS (D-UI-PRIVACY-FLAG, blind PE ruling S1). A node running -privacy=on
// serves an UNAUTHENTICATED reader a status document with NO `stats` block and with
// `countersWithheld: true`. Before this file both pages did `s.stats.BytesServed || 0`
// inline; on such a document that expression is not a false zero, it is a TypeError that
// aborts the whole render — every card after it, the daemon table, the files table — with
// no error shown, and one such daemon in the observatory's peer list froze the page for
// every daemon in it. A withheld counter is rendered as the word "withheld" plus the
// recovery, never as a number and never as a throw.
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.siltRender = api;
})(typeof self !== "undefined" ? self : this, function () {
  function fmtB(b) {
    b = Number(b) || 0;
    return b >= 1 << 30 ? (b / (1 << 30)).toFixed(2) + " GB"
      : b >= 1 << 20 ? (b / (1 << 20)).toFixed(1) + " MB"
      : b >= 1024 ? (b / 1024).toFixed(1) + " KB" : b + " B";
  }

  // withheld reports whether a status document withholds its serve counters from us.
  // It is the MARKER that decides, not the absence of `stats`: an old daemon that
  // predates the marker still carries stats, and a new daemon that withholds carries
  // the marker — either way the accessor below never dereferences undefined.
  function withheld(s) { return !!(s && s.countersWithheld); }
  function stats(s) { return (s && s.stats) || null; }

  // WITHHELD_HINT names the recovery: the operator's own tokened tab sees everything,
  // and the node's operator can publish to all readers with -privacy=off.
  const WITHHELD_HINT = "withheld — this node runs -privacy=on; open the UI from the URL the daemon printed (it carries your token), or run it with -privacy=off";

  // statusCards renders the dashboard's served card for one status document.
  function statusCards(s) {
    if (withheld(s)) {
      return { served: "withheld", servedsub: WITHHELD_HINT, withheld: true };
    }
    const st = stats(s) || {};
    return {
      served: fmtB(st.BytesServed || 0),
      servedsub: (st.ChunksServed || 0) + " chunks · " + (st.ChunksReceived || 0) + " received",
      withheld: false,
    };
  }

  // prereleaseBanner is the human surface of the -privacy=off posture (PE ruling S8): a
  // node publishing its node-wide serve counters to any reader says so on its own pages.
  function prereleaseBanner(s) {
    if (s && s.privacy && s.privacy.mode === "off") {
      return "PRE-RELEASE: this node runs -privacy=off and publishes node-wide serve counters (bytes served, balance) to any reader that can reach it. Run without the flag to withhold them from unauthenticated readers.";
    }
    return "";
  }

  // observatoryTotals sums what the observed daemons PUBLISH and counts the ones that
  // withhold, so the bandwidth card can say "N withheld" instead of silently summing zeros.
  function observatoryTotals(live) {
    let served = 0, chunks = 0, withheldCount = 0;
    for (const r of live) {
      const s = r.status || {};
      chunks += Number(s.chunks) || 0;
      if (withheld(s)) { withheldCount++; continue; }
      served += Number((stats(s) || {}).BytesServed) || 0;
    }
    return { served, chunks, withheldCount };
  }

  // servedCell renders one daemon's served column in the observatory table.
  function servedCell(s) {
    return withheld(s) ? '<span class="dim" title="' + WITHHELD_HINT + '">withheld</span>' : fmtB((stats(s) || {}).BytesServed || 0);
  }

  // LINK_WITHHELD_HINT: the library page's recovery text. The link is a permanent
  // decryption capability, so it is served only to a tab whose token arrived in the
  // Authorization header (D-UI-PRIVACY-FLAG, the header-only predicate).
  const LINK_WITHHELD_HINT = "link withheld — this node runs -privacy=on; open the UI from the URL the daemon printed (it carries your token), or run it with -privacy=off";

  // libraryGetCell renders the action cell of one library row. Never emits a get button
  // whose data-link is undefined: on a document with linksWithheld the row has no link,
  // and the old markup sent /api/fetch?link=undefined (blind PE code ruling B2, measured).
  function libraryGetCell(row, doc) {
    if (doc && doc.linksWithheld) {
      return '<span class="dim" title="' + LINK_WITHHELD_HINT + '">link withheld</span>';
    }
    if (!row || !row.link) {
      return '<span class="dim">no link</span>';
    }
    return '<button class="get" data-link="' + row.link + '">get</button>';
  }

  // ---- the economy panels (Boulder 2, R2.2 rows 14-17) --------------------------------
  //
  // Same contract as everything above: PURE, DOM-free, and never a bare dereference of a
  // block a withheld document does not carry. They also carry the four honesty rules the
  // endpoints publish, because a rule enforced only on the wire is one render away from
  // being broken on the page:
  //   Panel 1  finite === false renders "not yet measurable" — NEVER "perpetual".
  //   Panel 2  the cost is an operator INPUT; absent, the panel shows revenue only.
  //   Panel 3  the window needs two samples; before that it is "not yet measured", not 0.
  //   Panel 4  the word is "suspected"; the panel never claims a detection. The gate on
  //            it is a literal string scan, so the copy avoids the word entirely rather
  //            than writing "not detected" and forcing the gate to parse a negation.

  function fmtDur(sec) {
    sec = Number(sec) || 0;
    const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    if (d > 0) return d + "d " + h + "h";
    if (h > 0) return h + "h " + m + "m";
    return m + "m";
  }

  // PANEL 1 — my solvency. One cared object's funded-horizon cell.
  // "not yet measurable" is the instrument's own contract (credit.Horizon returns
  // finite=false when it has observed no burn) and it is NOT an all-clear: an unmeasured
  // burn is not a proven-safe one, which is why it is neither "perpetual" nor green.
  function solvencyCell(o) {
    if (!o) return { text: "—", state: "unknown" };
    if (o.finite === false) {
      return { text: "not yet measurable", state: "unknown",
        title: "no repair has been funded yet, so there is no observed burn to project. This is not a guarantee of permanence" };
    }
    return { text: fmtDur(o.horizonSec), state: o.cliff ? "cliff" : "ok",
      title: o.cliff ? "within the re-endowment warning window — top this reserve up" : "" };
  }

  // PANEL 1/3 — the whole document may be token-gated. detailWithheld says which absence
  // this is: withheld from THIS reader, versus this node caretakes nothing.
  function economyObjects(self) {
    if (!self) return { rows: [], note: "no document" };
    if (self.detailWithheld) return { rows: [], note: WITHHELD_HINT, withheld: true };
    const rows = self.objects || [];
    return { rows, note: rows.length ? "" : "this node caretakes no objects yet" };
  }

  // PANEL 2 — am I profitable. The cost is an operator input (?cost=N), never a persisted
  // flag, so with no cost supplied the panel shows revenue and says so rather than
  // printing the balance as if it were a margin.
  function marginCard(self) {
    if (!self || self.countersWithheld || !self.revenue) {
      return { margin: "withheld", sub: WITHHELD_HINT, withheld: true };
    }
    const m = self.margin || {};
    if (!m.costGiven) {
      return { margin: fmtB(self.revenue.servedBytes) + " served", withheld: false,
        sub: "cost not supplied — add ?cost=N (credits) to see a margin. Your operating cost is off-ledger and this node never stores it" };
    }
    return { margin: (Number(m.margin) || 0) + " credits", withheld: false,
      sub: "exact GIVEN your cost number (" + (Number(m.cost) || 0) + "): revenue is local-exact, cost is yours" };
  }

  // PANEL 3 — is durability self-funding. Reads the ROLLING window (/api/economy/flows),
  // not the lifetime totals: a node that was healthy for a year and is draining today has
  // a positive lifetime net and a negative window.
  function selfFundingCard(flows) {
    if (!flows) return { net: "—", sub: "no document" };
    if (flows.detailWithheld) return { net: "withheld", sub: WITHHELD_HINT, withheld: true };
    if (flows.windowNotYetMeasured || !flows.pooled) {
      return { net: "not yet measured", withheld: false,
        sub: "a delta needs two samples, taken " + fmtDur(flows.sampleIntervalSec) + " apart. This is not a zero net" };
    }
    const p = flows.pooled;
    return { net: (Number(p.net) || 0) + " credits", withheld: false, draining: !!p.draining,
      sub: (p.draining ? "DRAINING: " + p.consecutiveNegative + " consecutive negative samples — " : "") +
        "skim in " + (Number(p.skimIn) || 0) + " vs bounty out " + (Number(p.bountyOut) || 0) + " over " + fmtDur(flows.windowSec) };
  }

  // PANEL 4 — the wash SELF-check. It exists so an HONEST operator can see their own
  // shape and show they are not the cluster. Authenticity is not-knowable (Douceur), so
  // the word is "suspected"; this is never a detection and never a slashing input.
  function washCard(self) {
    if (!self || self.countersWithheld || !self.wash) {
      return { light: "withheld", sub: WITHHELD_HINT, withheld: true };
    }
    const w = self.wash;
    const pct = (Number(w.symmetry) || 0).toFixed(2);
    if (w.suspected) {
      return { light: "shape suspected", state: "warn", withheld: false,
        sub: "serve:fetch symmetry " + pct + " with a non-positive balance is the shape a wash pair leaves. SUSPECTED — a shape, never a finding: a node cannot prove another identity is a Sybil, and nothing here feeds slashing" };
    }
    return { light: "no wash shape", state: "ok", withheld: false,
      sub: "serve:fetch symmetry " + pct + ". Shape only: authenticity is not knowable from one node" };
  }

  // The gossip-estimated panels. A gossip figure NEVER renders without its sample size,
  // and below the floor it does not render at all — "sample too small" is a legitimate
  // rendering and a guess is not.
  // It takes the whole gini BLOCK, never a bare number: the old signature was called as
  // `gossipCell(doc, doc.serveGini && doc.serveGini.value)`, and a value of 0 is falsy —
  // which is exactly the case that must not render as a measurement.
  function gossipCell(block, gini) {
    // The privacy clause, first: a published Gini plus its sample size is one equation, and
    // a reader that supplies the other terms with free identities solves it for a node-wide
    // work counter this node withholds elsewhere. So on the shipped -privacy default the
    // two Gini figures are a NAMED ABSENCE — never a zero, never a blank that reads as a
    // measurement. Everything else on those panels (the C2 block, the tier mix, the bands,
    // the target ratio) is unaffected and still renders.
    if (block && block.countersWithheld) {
      return { text: "withheld by this node's privacy setting", withheld: true,
        sub: block.note || WITHHELD_HINT };
    }
    const sample = (block && block.sample) || null;
    if (!sample) return { text: "—", sub: "no sample" };
    if (sample.tooSmall) {
      return { text: "sample too small", tooSmall: true,
        sub: sample.size + " of " + sample.minSize + " nodes — a Gini over two values is those two values' ratio, so nothing is published" };
    }
    if (!gini) {
      return { text: "not published", sub: "this figure is absent on the wire, which is not a zero" };
    }
    // known === false means the sample summed to zero: nobody reported any work. Rendering
    // that as 0.0000 would say work is perfectly evenly spread across N nodes when the truth
    // is that N nodes said nothing.
    if (gini.known === false) {
      return { text: "no work reported", unknown: true, sub: gini.reason || "the sample reported no work at all" };
    }
    return { text: Number(gini.value || 0).toFixed(4), known: true,
      sub: "gossip-estimated over " + sample.size + " nodes" + (sample.selfIncluded ? " (this node included)" : "") +
        (gini.epoch ? " · " + gini.epoch : "") };
  }

  // The PER-TIER work cells (Economist advisory §3a). Same honesty rules as gossipCell and,
  // where they overlap, THE SAME CODE: edgeShareCell delegates every not-a-number case to
  // gossipCell and only re-formats the number, so the withhold / floor / absent / unknown
  // branches cannot drift between two panels that must agree.
  //
  // WHY A SHARE NEEDS THIS AT ALL. A tier that reported NOTHING contributes 0 to the
  // numerator while other tiers keep the denominator positive, so its share computes to a
  // perfectly well-formed 0.0 — "this tier does none of the work" when the truth is "no
  // peer of this tier told me anything". On the shipped -privacy default that is every
  // tier, every time (D-WORK-VISIBILITY), so this is the NORMAL rendering in production and
  // not an edge case.
  function tierShareCell(share) {
    if (!share) return { text: "—", title: "this figure is absent on the wire, which is not a zero" };
    if (share.known !== true) return { text: "not reported", unknown: true, title: share.reason || "" };
    return { text: (Number(share.value || 0) * 100).toFixed(1) + "%", known: true, title: "" };
  }

  // The T-AR figure: the edge tier's share of the reported serve bytes. It reads as a
  // PERCENTAGE because it is a share and not a dispersion index, and it carries its own
  // coverage because edge silence depresses the edge's own share — a false alarm that names
  // itself, never a false clean bill.
  function edgeShareCell(conc) {
    const c = gossipCell(conc, conc && conc.ponyShareOfServedBytes);
    const ps = conc && conc.ponyShareOfServedBytes;
    if (!c.known || !ps) return c;
    return { text: (Number(ps.value || 0) * 100).toFixed(1) + "%", known: true,
      sub: ps.reporting + " of " + ps.population + " edge nodes reporting (coverage " +
        Number(ps.coverage || 0).toFixed(2) + ")" + (ps.epoch ? " · " + ps.epoch : "") };
  }

  return { fmtB, fmtDur, withheld, statusCards, prereleaseBanner, observatoryTotals, servedCell, libraryGetCell,
    solvencyCell, economyObjects, marginCard, selfFundingCard, washCard, gossipCell,
    tierShareCell, edgeShareCell, WITHHELD_HINT, LINK_WITHHELD_HINT };
});
