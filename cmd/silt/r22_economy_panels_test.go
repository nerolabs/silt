package main

// R2.2 rows 14-17 — the four panels, and the honesty rules they must render.
//
// A rule enforced only on the wire is one render away from being broken on the page, and
// this repo has already paid for that: before render.js, a withheld document was not a
// false zero on the dashboard, it was a TypeError that aborted the whole page silently
// (D-UI-PRIVACY-FLAG, blind PE ruling S1). So the panels are PURE functions run here
// under node against fixtures, exactly as the status cards are.
//
// THE FOUR RULES UNDER TEST, each of which is a specific instruction from the design doc
// or the Economist advisory:
//   Panel 1  finite == false renders "not yet measurable" and NEVER "perpetual".
//   Panel 2  the operating cost is an operator INPUT; with none supplied no margin is
//            asserted, and nothing here is persisted.
//   Panel 3  before two samples there is no window: "not yet measured", never a 0 net.
//   Panel 4  the word is "suspected". Never "detected".
// Plus row 13: a gossip-estimated figure never renders without its sample size.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestR22EconomyPanelsRenderTheHonestyRules(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required to run the panel gate (cmd/silt/ui/render.js): %v", err)
	}
	script := `
const r = require(require("path").resolve(process.argv[1]));
const out = {};
// Panel 1: the two horizon states, plus a withheld document.
out.horizonUnmeasured = r.solvencyCell({ finite: false, horizonSec: 0, cliff: false });
out.horizonCliff = r.solvencyCell({ finite: true, horizonSec: 3600 * 30, cliff: true });
out.horizonOk = r.solvencyCell({ finite: true, horizonSec: 86400 * 400, cliff: false });
out.objectsWithheld = r.economyObjects({ detailWithheld: true });
out.objectsEmpty = r.economyObjects({ detailWithheld: false, objects: [] });
// Panel 2.
out.marginNoCost = r.marginCard({ revenue: { servedBytes: 1048576 }, margin: { costGiven: false, margin: 7, cost: 0 } });
out.marginCost = r.marginCard({ revenue: { servedBytes: 1 }, margin: { costGiven: true, margin: -40, cost: 100 } });
out.marginWithheld = r.marginCard({ countersWithheld: true });
// Panel 3.
out.fundingNoWindow = r.selfFundingCard({ windowNotYetMeasured: true, sampleIntervalSec: 360 });
out.fundingDraining = r.selfFundingCard({ windowSec: 1080, sampleIntervalSec: 360, pooled: { skimIn: 0, bountyOut: 300, net: -300, draining: true, consecutiveNegative: 3 } });
out.fundingWithheld = r.selfFundingCard({ detailWithheld: true });
// Panel 4.
out.washSuspected = r.washCard({ wash: { symmetry: 0.98, balanceNonPositive: true, suspected: true } });
out.washClear = r.washCard({ wash: { symmetry: 0.1, suspected: false } });
out.washWithheld = r.washCard({ countersWithheld: true });
// Row 13.
const bigSample = { sample: { size: 9, minSize: 3, tooSmall: false, selfIncluded: true } };
out.gossipSmall = r.gossipCell({ sample: { size: 2, minSize: 3, tooSmall: true } }, undefined);
out.gossipAbsent = r.gossipCell(bigSample, undefined);
out.gossipValue = r.gossipCell(bigSample, { known: true, value: 0.1234, sampleSize: 9, epoch: "since each node's process started, NOT lifetime" });
// B2 (blind PE): a sample that summed to zero. The wire sends known:false and NO value, so
// the cell is driven exactly as the endpoint would send it — and, separately, with an
// explicit 0 present, because a consumer must branch on known and never on the number
// (a value of 0 is falsy, which is how the old bare-number signature shipped this).
out.gossipNoWork = r.gossipCell(bigSample, { known: false, sampleSize: 9, reason: "no work reported by this sample" });
out.gossipZeroValue = r.gossipCell(bigSample, { known: false, value: 0, sampleSize: 9, reason: "no work reported by this sample" });
out.gossipMeasuredZero = r.gossipCell(bigSample, { known: true, value: 0, sampleSize: 9 });
// B3 / the owner's ruling: on the shipped -privacy default the two Gini figures are a
// NAMED absence. The block still carries its note, and the cell must not fall through to
// "no sample", which would read as a node that knows nobody.
out.gossipPrivacy = r.gossipCell({ countersWithheld: true, note: "withheld by this node's privacy setting (-privacy=on, the default)" }, undefined);
// The PER-TIER work cells (Economist advisory §3a). A tier that reported nothing must NOT
// render 0 %: its share computes to a well-formed 0.0 because the exclusion rule keeps it
// out of the numerator while other tiers hold the denominator up, and 0 % on the panel says
// the tier does none of the work.
out.tierAbsent = r.tierShareCell(null);
out.tierUnknown = r.tierShareCell({ known: false, reason: "no peer of this tier reported any work" });
out.tierMeasuredZero = r.tierShareCell({ known: true, value: 0 });
out.tierValue = r.tierShareCell({ known: true, value: 0.1998 });
out.edgeWithheld = r.edgeShareCell({ countersWithheld: true, note: "withheld by this node's privacy setting (-privacy=on, the default)" });
out.edgeAbsent = r.edgeShareCell(bigSample);
out.edgeUnknown = r.edgeShareCell(Object.assign({}, bigSample, { ponyShareOfServedBytes: { known: false, reporting: 0, population: 100, coverage: 0, reason: "no peer of this tier reported any work" } }));
out.edgeValue = r.edgeShareCell(Object.assign({}, bigSample, { ponyShareOfServedBytes: { known: true, value: 0.1998, reporting: 1000, population: 1000, coverage: 1, epoch: "since each node's process started, NOT lifetime" } }));
// And the shapes a real withheld/absent document actually has: nothing may throw.
out.nulls = [r.solvencyCell(null).text, r.marginCard(null).margin, r.selfFundingCard(null).net,
             r.washCard(null).light, r.gossipCell(null, null).text, r.economyObjects(null).rows.length,
             r.tierShareCell(null).text, r.edgeShareCell(null).text];
console.log(JSON.stringify(out));`
	cmd := exec.Command(node, "-e", script, filepath.Join("ui", "render.js"))
	cmd.Dir = "."
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render.js THREW on the economy fixtures (the abort-the-page shape): %v\n%s", err, raw)
	}
	type cell struct {
		Text, Sub, State, Title, Margin, Net, Light  string
		Withheld, TooSmall, Draining, Unknown, Known bool
	}
	var out struct {
		HorizonUnmeasured, HorizonCliff, HorizonOk        cell
		ObjectsWithheld, ObjectsEmpty                     struct{ Note string }
		MarginNoCost, MarginCost, MarginWithheld          cell
		FundingNoWindow, FundingDraining, FundingWithheld cell
		WashSuspected, WashClear, WashWithheld            cell
		GossipSmall, GossipAbsent, GossipValue            cell
		GossipNoWork, GossipZeroValue, GossipMeasuredZero cell
		GossipPrivacy                                     cell
		TierAbsent, TierUnknown                           cell
		TierMeasuredZero, TierValue                       cell
		EdgeWithheld, EdgeAbsent                          cell
		EdgeUnknown, EdgeValue                            cell
		Nulls                                             []json.RawMessage
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}

	// Panel 1. The instrument's contract: no observed burn is NOT a proven-safe one.
	if out.HorizonUnmeasured.Text != "not yet measurable" {
		t.Fatalf("an unmeasurable horizon renders %q, want \"not yet measurable\"", out.HorizonUnmeasured.Text)
	}
	if strings.Contains(strings.ToLower(out.HorizonUnmeasured.Text+out.HorizonUnmeasured.Sub+out.HorizonUnmeasured.Title), "perpetual") {
		t.Fatalf("the unmeasurable-horizon cell says \"perpetual\" somewhere: %+v. silt funds a FINITE renewable horizon (D-S7); calling an unmeasured burn perpetual is the claim the instrument exists to refuse", out.HorizonUnmeasured)
	}
	if out.HorizonUnmeasured.State == "ok" {
		t.Fatalf("the unmeasurable horizon renders as OK — an unmeasured burn is not a proven-safe one, so it must not be green")
	}
	if out.HorizonCliff.State != "cliff" || out.HorizonOk.State != "ok" {
		t.Fatalf("cliff/ok states = %q / %q — the gate above is vacuous unless a real horizon still renders", out.HorizonCliff.State, out.HorizonOk.State)
	}
	if !strings.Contains(out.ObjectsWithheld.Note, "-privacy") || strings.Contains(out.ObjectsEmpty.Note, "-privacy") {
		t.Fatalf("withheld/empty object lists are not distinguished: %q vs %q. `objects: []` means this node caretakes nothing; a withheld document means you did not authenticate",
			out.ObjectsWithheld.Note, out.ObjectsEmpty.Note)
	}

	// Panel 2. The cost is the operator's, and this node never keeps it.
	if strings.Contains(out.MarginNoCost.Margin, "credits") || !strings.Contains(out.MarginNoCost.Sub, "cost not supplied") {
		t.Fatalf("with no cost supplied the panel asserts a margin: %+v. Revenue is local-exact; the cost is off-ledger, so a margin without it is a number with no meaning", out.MarginNoCost)
	}
	if !strings.Contains(out.MarginCost.Sub, "GIVEN your cost") {
		t.Fatalf("with a cost supplied the panel does not qualify it: %+v", out.MarginCost)
	}
	if !out.MarginWithheld.Withheld || out.MarginWithheld.Margin != "withheld" {
		t.Fatalf("withheld margin = %+v, want the word, never a number", out.MarginWithheld)
	}

	// Panel 3. No window is not a zero net.
	if out.FundingNoWindow.Net != "not yet measured" {
		t.Fatalf("a one-sample window renders %q. A 0 net on a draining node is the silent-loss shape (Don't #4)", out.FundingNoWindow.Net)
	}
	if !out.FundingDraining.Draining || !strings.Contains(out.FundingDraining.Sub, "DRAINING") {
		t.Fatalf("a three-sample drain does not render as draining: %+v", out.FundingDraining)
	}
	if !out.FundingWithheld.Withheld {
		t.Fatalf("withheld flows = %+v", out.FundingWithheld)
	}

	// Panel 4. Suspected, never detected — a node cannot prove another is a Sybil.
	all := out.WashSuspected.Light + out.WashSuspected.Sub + out.WashClear.Light + out.WashClear.Sub
	if !strings.Contains(strings.ToLower(out.WashSuspected.Light+out.WashSuspected.Sub), "suspected") {
		t.Fatalf("the wash panel does not say \"suspected\": %+v", out.WashSuspected)
	}
	if strings.Contains(strings.ToLower(all), "detected") {
		t.Fatalf("the wash panel says \"detected\": %q. Authenticity is not-knowable (Douceur); this is a SHAPE self-check and never a detection or a slashing input", all)
	}
	if !out.WashWithheld.Withheld {
		t.Fatalf("withheld wash = %+v", out.WashWithheld)
	}

	// Row 13. A gossip figure never renders without its sample size.
	if !out.GossipSmall.TooSmall || !strings.Contains(out.GossipSmall.Text, "sample too small") || !strings.Contains(out.GossipSmall.Sub, "2 of 3") {
		t.Fatalf("below the floor the cell = %+v; it must say the sample is too small AND how small", out.GossipSmall)
	}
	if !strings.Contains(out.GossipAbsent.Text, "not published") {
		t.Fatalf("an absent value above the floor = %+v; absent is not zero", out.GossipAbsent)
	}
	if out.GossipValue.Text != "0.1234" || !strings.Contains(out.GossipValue.Sub, "over 9 nodes") {
		t.Fatalf("a published gossip figure = %+v; it must carry its sample size in the same cell", out.GossipValue)
	}
	// B4: and its EPOCH. The counters reset at every restart (D-FP2-SCOPE), so a
	// concentration figure over them partly measures uptime. That caveat is part of what
	// the number means, so it travels with it to the operator, not only on the wire.
	if !strings.Contains(out.GossipValue.Sub, "NOT lifetime") {
		t.Fatalf("the published gossip cell does not carry the epoch: %q. These counters reset at every restart, so a node up for a week and one up for an hour differ by uptime as much as by behaviour", out.GossipValue.Sub)
	}
	// B2. An unknown Gini must never render as a number, in either wire form.
	for name, got := range map[string]cell{"no value sent": out.GossipNoWork, "an explicit 0 sent": out.GossipZeroValue} {
		if got.Text != "no work reported" || !got.Unknown {
			t.Fatalf("a sample that reported no work (%s) renders %q. credit.Gini returns 0 for BOTH a measured equality and a zero sum; rendering the second as 0.0000 tells the operator that work is perfectly evenly spread across 9 nodes when 9 nodes said nothing", name, got.Text)
		}
	}
	// ...and a REAL measured 0 (nine nodes that all did identical work) still renders as a
	// number, or the branch above is just a blanket refusal to publish zeros.
	if out.GossipMeasuredZero.Text != "0.0000" || !out.GossipMeasuredZero.Known {
		t.Fatalf("a MEASURED equality renders %q; known:true with value 0 is a real result and must still publish", out.GossipMeasuredZero.Text)
	}

	if !out.GossipPrivacy.Withheld || !strings.Contains(out.GossipPrivacy.Text, "withheld") {
		t.Fatalf("a privacy-withheld gossip block renders %+v. It must say so plainly — never a zero, and never a blank that reads as a measurement or as a node with no peers", out.GossipPrivacy)
	}
	if !strings.Contains(strings.ToLower(out.GossipPrivacy.Sub), "privacy") {
		t.Fatalf("the withheld cell does not name the setting responsible: %q", out.GossipPrivacy.Sub)
	}

	// THE PER-TIER CELLS (Economist advisory §3a). The rule is the SAME one as the Gini's
	// and it has a second edge here: a tier's share of the work is 0.0 both when the tier
	// did nothing and when the tier SAID nothing, and on the shipped -privacy default it is
	// always the second.
	if out.TierAbsent.Text != "—" {
		t.Fatalf("an absent per-tier share renders %q; a missing block is not a zero", out.TierAbsent.Text)
	}
	if out.TierUnknown.Text != "not reported" || !out.TierUnknown.Unknown {
		t.Fatalf("a tier with no reporting peer renders %q. 0 %% would say the tier does none of the work; the truth is that no peer of that tier said anything, and under the certified exclusion rule a silent peer is out of the numerator AND the denominator", out.TierUnknown.Text)
	}
	if out.TierUnknown.Title == "" {
		t.Fatalf("the not-reported cell carries no reason in its title: %+v. A named absence is named", out.TierUnknown)
	}
	if out.TierMeasuredZero.Text != "0.0%" || !out.TierMeasuredZero.Known {
		t.Fatalf("a MEASURED zero share renders %q; a tier that reported and did none of this work is a real result and must publish, or the branch above is a blanket refusal to show zeros", out.TierMeasuredZero.Text)
	}
	if out.TierValue.Text != "20.0%" {
		t.Fatalf("a measured 0.1998 share renders %q, want \"20.0%%\"", out.TierValue.Text)
	}
	// The edge card delegates every not-a-number case to gossipCell, so these four arms are
	// the proof that the delegation is live rather than a comment.
	if !out.EdgeWithheld.Withheld || !strings.Contains(out.EdgeWithheld.Text, "withheld") {
		t.Fatalf("the edge-share card on a privacy-withheld document = %+v", out.EdgeWithheld)
	}
	if !strings.Contains(out.EdgeAbsent.Text, "not published") {
		t.Fatalf("an absent ponyShareOfServedBytes renders %q; absent is not zero", out.EdgeAbsent.Text)
	}
	if !out.EdgeUnknown.Unknown || out.EdgeUnknown.Text == "0.0%" {
		t.Fatalf("an UNKNOWN edge share renders %+v. This is the reading on every node running the shipped -privacy default (D-WORK-VISIBILITY), so rendering it as 0 %% would tell every operator the edge tier does none of the work", out.EdgeUnknown)
	}
	if out.EdgeValue.Text != "20.0%" || !out.EdgeValue.Known {
		t.Fatalf("a measured edge share renders %+v, want \"20.0%%\"", out.EdgeValue)
	}
	if !strings.Contains(out.EdgeValue.Sub, "1000 of 1000") || !strings.Contains(out.EdgeValue.Sub, "coverage") {
		t.Fatalf("the edge-share card does not carry its own coverage: %q. Edge silence depresses the edge's own share, so the coverage is how a reader tells a real minority from a quiet one", out.EdgeValue.Sub)
	}
	if !strings.Contains(out.EdgeValue.Sub, "NOT lifetime") {
		t.Fatalf("the edge-share card lost the epoch: %q", out.EdgeValue.Sub)
	}

	// SOURCE GATE: the page must go through render.js, like every other page.
	html, err := os.ReadFile(filepath.Join("ui", "economy.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<script src="render.js"></script>`, "siltRender.solvencyCell(", "siltRender.washCard(", "siltRender.gossipCell(", "siltRender.selfFundingCard(", "siltRender.marginCard(",
		"siltRender.edgeShareCell(", "siltRender.tierShareCell("} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("SOURCE GATE: economy.html does not use %s — a panel built inline is a panel the honesty gate above cannot see", want)
		}
	}
	if strings.Contains(string(html), ".revenue.balance") || strings.Contains(string(html), ".wash.suspected") {
		t.Fatalf("SOURCE GATE: economy.html dereferences a withheld-able block inline; on a -privacy=on document those blocks are ABSENT and the deref aborts the render")
	}
}
