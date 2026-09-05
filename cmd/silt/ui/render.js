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

  return { fmtB, withheld, statusCards, prereleaseBanner, observatoryTotals, servedCell, libraryGetCell, WITHHELD_HINT, LINK_WITHHELD_HINT };
});
