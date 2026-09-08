package main

// The economy-observability surface beyond /api/economy/self (Boulder 2, R2.2 / Lane
// C3): four read-only GET routes and the bounded ring that feeds two of them.
//
// THE HONESTY RULE THIS WHOLE FILE TURNS ON (design doc §0,
// docs/thinking/2026-09-01-economy-observability-design.md). Every field carries a
// knowability tier and NO number publishes without one. The tiers, hardest first:
// local-exact (this node's own ledger/care state), committed-global (the chain every
// node holds), gossip-estimated (a local peer SAMPLE, published only WITH its sample
// size), not-knowable (never published as fact — named as absent, with the reason).
//
// SO THE THREE LEGITIMATE RENDERINGS OF A NUMBER WE DO NOT HAVE ARE: absent with a
// named reason, "sample too small", and "not yet measurable". A zero is none of them
// (Don't #4: a false absence is a silent-loss shape), and a guess is not a rendering.

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// ---- the flow ring (rows 1, 2, 4) ----------------------------------------------------

// flowSampleInterval is how often the escrow ring takes a sample, and flowRingDepth is
// how many it keeps. Presentation cadence, not a mechanism: no validity, disbursement
// or standing rule reads either.
//
// PROVENANCE (Economist advisory §2 rows 1-2): "one sample per epoch (~5.9 min
// [MEASURED]), depth 24 = ~2.4 h". 6 minutes is that measured epoch rounded up to a
// round number — sampling slightly SLOWER than an epoch is the safe direction, because
// the thing being measured (skim in, bounty out) accumulates and a coarser sample can
// only under-report the number of distinct windows, never invent one.
//
// IT COSTS NO TIMER. The ring is appended from statusSnapshot, which already recomputes
// on its own cadence, so a sample lands on the first status read after the interval
// elapses. That means the ring is only as dense as the daemon is READ — which is
// honest, and is why windowSec on the wire is the span actually spanned, never the
// nominal depth x interval.
const (
	flowSampleInterval = 6 * time.Minute
	flowRingDepth      = 24
	// flowDrainSamples is how many consecutive negative net deltas the endpoint calls
	// a drain (advisory row 2: "the drain signal is net < 0 for 3 consecutive
	// samples"). Three, so one bounty payment inside one window is not a drain.
	flowDrainSamples = 3
	// maxFlowRings bounds the per-root state this ring holds, at the same magnitude
	// and for the same reason as core/node's maxPeerInfo: an unbounded per-key cache
	// on a small box is unsafe, not merely inefficient (build-immutable #8). The cared
	// set is the OPERATOR's own list, not an attacker's, so this is a ceiling and not
	// a defence; beyond it the endpoint says objectsTruncated rather than dropping
	// rows silently.
	maxFlowRings = 4096
)

// flowTotals is the pair of cumulative escrow counters one sample holds for one scope.
// It is DERIVED from a ports.DurabilitySnapshot, never stored beside one: the ring used
// to keep both a flowTotals map and a snapshot map per sample, which stored Funded and
// Paid twice per root per sample and doubled the ring's resident cost for nothing.
type flowTotals struct {
	SkimIn    int64 `json:"skimIn"`
	BountyOut int64 `json:"bountyOut"`
	Net       int64 `json:"net"`
}

func totalsOf(s ports.DurabilitySnapshot) flowTotals {
	return flowTotals{SkimIn: s.Funded, BountyOut: s.Paid, Net: s.Funded - s.Paid}
}

func (t flowTotals) sub(o flowTotals) flowTotals {
	return flowTotals{SkimIn: t.SkimIn - o.SkimIn, BountyOut: t.BountyOut - o.BountyOut, Net: (t.SkimIn - o.SkimIn) - (t.BountyOut - o.BountyOut)}
}

// flowSample is one instant of the escrow accounting: ONE map, root -> snapshot, which
// serves both endpoints (flows differences its Funded/Paid, g differences the whole
// snapshot). The pooled figure is NOT stored: it is the sum of the per-root rows, and
// storing it separately is what let the two disagree (see windowRows).
type flowSample struct {
	at    time.Time
	obj   map[string]ports.DurabilitySnapshot
	trunc bool
}

// noteFlowSample appends one sample if the interval has elapsed. Called from
// statusSnapshot with statusMu held, off the document that was just computed — so the
// ring and the two endpoints read the SAME accounting the status document published,
// never a second read of the ledger taken at a different instant.
func (s *uiServer) noteFlowSample(now time.Time, doc *statusInfo) {
	if n := len(s.flowRing); n > 0 && now.Sub(s.flowRing[n-1].at) < flowSampleInterval {
		return
	}
	fs := flowSample{at: now, obj: map[string]ports.DurabilitySnapshot{}}
	if doc.Durability != nil {
		objs := append([]objDurability(nil), doc.Durability.Objects...)
		// Sorted so a truncation at the cap keeps the SAME roots sample to sample; an
		// arbitrary cut would make a root's series appear and vanish.
		sort.Slice(objs, func(i, j int) bool { return objs[i].Root < objs[j].Root })
		if len(objs) > maxFlowRings {
			objs, fs.trunc = objs[:maxFlowRings], true
		}
		for _, o := range objs {
			fs.obj[o.Root] = ports.DurabilitySnapshot{Balance: o.Reserve, Funded: o.Funded, Paid: o.Paid, Repairs: o.Repairs}
		}
	}
	s.flowRing = append(s.flowRing, fs)
	if len(s.flowRing) > flowRingDepth {
		s.flowRing = s.flowRing[len(s.flowRing)-flowRingDepth:]
	}
}

// flowWindow returns a copy of the ring, taking the snapshot first so the ring is
// current. Copied under the lock: a handler must not read the slice a later snapshot
// may re-slice.
func (s *uiServer) flowWindow(now time.Time) ([]flowSample, *statusInfo, time.Time) {
	doc, takenAt := s.statusSnapshot(now)
	s.statusMu.Lock()
	ring := append([]flowSample(nil), s.flowRing...)
	s.statusMu.Unlock()
	return ring, doc, takenAt
}

// ---- GET /api/economy/flows (rows 1-2) -----------------------------------------------

type economyFlows struct {
	Tier              string `json:"tier"` // local-exact: my own escrows
	SampleIntervalSec int64  `json:"sampleIntervalSec"`
	RingDepth         int    `json:"ringDepth"`
	DrainSamples      int    `json:"drainSamples"`
	// Samples is how many are IN the ring and WindowSec the span they actually cover.
	// Absent (with WindowNotYetMeasured) rather than zero when fewer than two exist: a
	// delta needs two samples, and "net 0" over a window that does not exist yet is a
	// false all-clear.
	Samples              int              `json:"samples,omitempty"`
	WindowSec            int64            `json:"windowSec,omitempty"`
	WindowNotYetMeasured bool             `json:"windowNotYetMeasured,omitempty"`
	Pooled               *economyFlowRow  `json:"pooled,omitempty"`
	Objects              []economyFlowRow `json:"objects,omitempty"`
	ObjectsTruncated     bool             `json:"objectsTruncated,omitempty"`
	DetailWithheld       bool             `json:"detailWithheld"`
	Note                 string           `json:"note"`

	SnapshotTakenAtUnix int64 `json:"snapshotTakenAtUnix"`
	SnapshotAgeSec      int64 `json:"snapshotAgeSec"`
	SnapshotIntervalSec int64 `json:"snapshotIntervalSec"`
}

type economyFlowRow struct {
	Root string `json:"root,omitempty"` // absent on the pooled row
	flowTotals
	// ConsecutiveNegative is the trailing run of sample-to-sample deltas with net < 0,
	// and Draining is that run reaching drainSamples. The DIRECTION signal, separate
	// from the window total: an object can be net-positive over the window and still be
	// draining right now.
	ConsecutiveNegative int  `json:"consecutiveNegative"`
	Draining            bool `json:"draining"`
}

func drainRun(steps []int64) int {
	run := 0
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i] >= 0 {
			break
		}
		run++
	}
	return run
}

// flowWindowRows turns a ring into the pooled row and the per-object rows. PURE, and
// split out for the reason economyConcentrationDoc is: the rule it enforces is a decision
// about a VALUE, and a value decision should be testable without a node behind it — the
// node has no uncare API, so the departure direction can only be driven here.
//
// THE RULE, WHICH IS ONE RULE AND NOT TWO (blind PE ruling B1, measured). A root's window
// starts where the root FIRST APPEARS in the ring, because differencing against a zero it
// never held would report its whole pre-window lifetime inflow as one window's. The
// per-object path always did this. The pooled path did not: it differenced two whole-sample
// totals, so a root cared for mid-window added its entire lifetime `funded` to the pooled
// delta while its own row correctly reported 0. Measured on the real fixture before the fix:
//
//	"pooled":{"skimIn":5000,"bountyOut":0,"net":5000}
//	"objects":[{"root":"0100…","net":0},{"root":"0200…","net":0}]
//
// That is not cosmetic. The pooled STEPS drive Draining, Panel 3's headline alarm: caring
// for one new object injects a large positive step and MASKS a real drain for
// flowDrainSamples samples, and dropping care on one injects a large negative step and
// LATCHES a false drain.
//
// The fix is to stop having two rules. Pooled is now the SUM OF THE PER-OBJECT ROWS, both
// for the window total and step by step, so the two can no longer disagree by construction
// — a pooled figure that is not the sum of the rows printed beneath it is a lie whatever
// the arithmetic behind it. A step contributes only where the root is present at BOTH ends
// of that step, which is the same first-appearance rule applied per interval and is what
// makes a departure contribute nothing rather than a phantom negative.
func flowWindowRows(ring []flowSample) (economyFlowRow, []economyFlowRow, bool) {
	last := ring[len(ring)-1]
	roots := make([]string, 0, len(last.obj))
	for root := range last.obj {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	pooledSteps := make([]int64, len(ring)-1)
	objects := make([]economyFlowRow, 0, len(roots))
	var pooled economyFlowRow
	for _, root := range roots {
		base, steps := flowTotals{}, make([]int64, 0, len(ring)-1)
		var prev *flowTotals
		for k, fs := range ring {
			sn, ok := fs.obj[root]
			if !ok {
				continue
			}
			t := totalsOf(sn)
			if prev == nil {
				base = t
			} else {
				steps = append(steps, t.sub(*prev).Net)
			}
			// The pooled step for interval k-1 -> k takes this root's term only when
			// the root is in BOTH samples. An entry contributes nothing to the step it
			// appears in; a departure contributes nothing to the step it vanishes in.
			if k > 0 {
				if p, ok := ring[k-1].obj[root]; ok {
					pooledSteps[k-1] += t.sub(totalsOf(p)).Net
				}
			}
			cp := t
			prev = &cp
		}
		row := economyFlowRow{Root: root, flowTotals: totalsOf(last.obj[root]).sub(base), ConsecutiveNegative: drainRun(steps)}
		row.Draining = row.ConsecutiveNegative >= flowDrainSamples
		objects = append(objects, row)
		pooled.SkimIn += row.SkimIn
		pooled.BountyOut += row.BountyOut
	}
	pooled.Net = pooled.SkimIn - pooled.BountyOut
	pooled.ConsecutiveNegative = drainRun(pooledSteps)
	pooled.Draining = pooled.ConsecutiveNegative >= flowDrainSamples
	return pooled, objects, last.trunc
}

func (s *uiServer) apiEconomyFlows(w http.ResponseWriter, r *http.Request) {
	now := s.nowWall()
	ring, doc, takenAt := s.flowWindow(now)
	out := economyFlows{
		Tier:                "local-exact",
		SampleIntervalSec:   int64(flowSampleInterval / time.Second),
		RingDepth:           flowRingDepth,
		DrainSamples:        flowDrainSamples,
		Note:                "skim-in vs bounty-out over a rolling window of my OWN cared objects (local-exact). A persistently negative net is the drain signal; one negative sample is not",
		SnapshotTakenAtUnix: doc.SnapshotTakenAtUnix,
		SnapshotAgeSec:      int64(now.Sub(takenAt).Seconds()),
		SnapshotIntervalSec: doc.SnapshotIntervalSec,
	}
	if len(ring) < 2 {
		out.WindowNotYetMeasured = true
		out.Note = "window not yet measured: a delta needs two samples and the ring holds " + itoa(len(ring)) + ". This is NOT a zero net"
	} else {
		pooled, objects, trunc := flowWindowRows(ring)
		out.Samples = len(ring)
		out.WindowSec = int64(ring[len(ring)-1].at.Sub(ring[0].at).Seconds())
		out.ObjectsTruncated = trunc
		out.Pooled = &pooled
		out.Objects = objects
	}
	writeJSON(w, withheldEconomyFlows(out, s.readerAuthFor(r)))
}

// withheldEconomyFlows is the flows document an UNAUTHENTICATED reader gets. It is an
// ALLOW-LIST, the same shape as withheldEconomySelf, and it withholds EVERYTHING
// derived from the escrows — pooled included, not just the per-object array.
//
// WHY POOLED GOES TOO. On a node caretaking ONE object the pooled window delta IS that
// object's delta, and objects[].skimIn is the delta of the counter /api/status already
// gates (durability.objects[].funded, red-team F2) while /api/roots supplies the name
// half of the join. Publishing the one-term sum of a withheld array is what shipped the
// selfFunding figures open in the first place. There is no aggregate here that is not
// also, on some real node, a single object's figure.
func withheldEconomyFlows(full economyFlows, auth readerAuth) economyFlows {
	if auth.token {
		return full
	}
	return economyFlows{
		Tier:                full.Tier,
		SampleIntervalSec:   full.SampleIntervalSec,
		RingDepth:           full.RingDepth,
		DrainSamples:        full.DrainSamples,
		DetailWithheld:      true,
		Note:                "withheld: token-gated. Every figure on this route is an escrow delta of a root /api/roots names (red-team F2) — the pooled sum too, because on a one-object node it IS that object's figure",
		SnapshotTakenAtUnix: full.SnapshotTakenAtUnix,
		SnapshotAgeSec:      full.SnapshotAgeSec,
		SnapshotIntervalSec: full.SnapshotIntervalSec,
	}
}

// ---- GET /api/economy/g (rows 4-5) ---------------------------------------------------

type economyG struct {
	Tier                 string        `json:"tier"` // local-exact per MY objects
	Threshold            string        `json:"threshold"`
	WindowSec            int64         `json:"windowSec,omitempty"`
	Objects              []economyGRow `json:"objects,omitempty"`
	NetworkNotKnowable   string        `json:"networkNotKnowable,omitempty"`
	WindowNotYetMeasured bool          `json:"windowNotYetMeasured,omitempty"`
	DetailWithheld       bool          `json:"detailWithheld"`

	SnapshotTakenAtUnix int64 `json:"snapshotTakenAtUnix"`
	SnapshotAgeSec      int64 `json:"snapshotAgeSec"`
	SnapshotIntervalSec int64 `json:"snapshotIntervalSec"`
}

type economyGRow struct {
	Root string `json:"root"`
	// Known separates "g is 0 because cost did not move" from "g could not be
	// computed". credit.G returns 0 for BOTH (instruments.go: dt <= 0, or either
	// snapshot funded no repairs, or the earlier cost was 0), and the advisory is
	// explicit that the second must render as UNKNOWN, never as flat.
	Known         bool    `json:"known"`
	G             float64 `json:"g,omitempty"`
	Perpetual     bool    `json:"perpetualEarnable,omitempty"` // g > 0, only when Known
	CostPerRepair int64   `json:"costPerRepair"`
	Repairs       int64   `json:"repairs"`
	Reason        string  `json:"reason,omitempty"` // why Known is false
}

// networkGNotKnowable is the honest answer to advisory row 5 (network aggregate g), and
// it is a STOP, not an omission.
//
// g is the annualized trend of cost-per-repair: it needs each node's PAID CREDITS and
// its REPAIR COUNT (credit.CostPerRepair = Paid/Repairs). Rows 8-9 gossip served bytes
// and repairs done. Repairs done is the denominator; the numerator — credits paid out
// of an escrow, or credits earned as a repairer — is gossiped by nothing, and row 10 is
// explicit that a THIRD gossip field must not be added. So the network aggregate is not
// computable from the surface this track is allowed to build, and the choice is between
// publishing a number derived from an input nobody sent and publishing the absence.
// Row 13's rule already names the right answer: "field absent" is a legitimate
// rendering; a guess is not.
const networkGNotKnowable = "not knowable from the two gossiped work fields: g needs credits-paid-per-repair and gossip carries repairs-done (the denominator) with no numerator. Adding a third gossip field is out of scope by advisory row 10, so this is absent rather than estimated"

func (s *uiServer) apiEconomyG(w http.ResponseWriter, r *http.Request) {
	now := s.nowWall()
	ring, doc, takenAt := s.flowWindow(now)
	out := economyG{
		Tier:                "local-exact",
		Threshold:           "g > 0: the credit cost of a repair is DECLINING and perpetual durability is earnable. g <= 0: the plateau/inflation regime — re-endow (D-S7). g is MEASURED, never assumed",
		NetworkNotKnowable:  networkGNotKnowable,
		SnapshotTakenAtUnix: doc.SnapshotTakenAtUnix,
		SnapshotAgeSec:      int64(now.Sub(takenAt).Seconds()),
		SnapshotIntervalSec: doc.SnapshotIntervalSec,
	}
	if len(ring) < 2 {
		out.WindowNotYetMeasured = true
	} else {
		first, last := ring[0], ring[len(ring)-1]
		out.WindowSec = int64(last.at.Sub(first.at).Seconds())
		roots := make([]string, 0, len(last.obj))
		for root := range last.obj {
			roots = append(roots, root)
		}
		sort.Strings(roots)
		for _, root := range roots {
			newSnap := last.obj[root]
			row := economyGRow{Root: root, CostPerRepair: credit.CostPerRepair(newSnap), Repairs: newSnap.Repairs}
			oldSnap, ok := first.obj[root]
			dt := ports.Duration(last.at.Sub(first.at))
			switch {
			case !ok:
				row.Reason = "no sample at the start of the window: this object entered the ring mid-window"
			case dt <= 0:
				row.Reason = "the window has no duration yet"
			case credit.CostPerRepair(oldSnap) <= 0:
				row.Reason = "no repair had been funded at the start of the window, so there is no earlier cost to trend from"
			case newSnap.Repairs <= 0:
				row.Reason = "no repair funded in this object's history, so there is no realised cost"
			default:
				row.Known = true
				row.G = credit.G(oldSnap, newSnap, dt)
				row.Perpetual = row.G > 0
			}
			out.Objects = append(out.Objects, row)
		}
	}
	writeJSON(w, withheldEconomyG(out, s.readerAuthFor(r)))
}

// withheldEconomyG: same ALLOW-LIST rule as flows, same reason. Every row names a root
// and carries that root's realised repair cost.
func withheldEconomyG(full economyG, auth readerAuth) economyG {
	if auth.token {
		return full
	}
	return economyG{
		Tier:                full.Tier,
		Threshold:           full.Threshold,
		NetworkNotKnowable:  full.NetworkNotKnowable,
		DetailWithheld:      true,
		SnapshotTakenAtUnix: full.SnapshotTakenAtUnix,
		SnapshotAgeSec:      full.SnapshotAgeSec,
		SnapshotIntervalSec: full.SnapshotIntervalSec,
	}
}

// ---- the gossip-estimated pair (rows 6-7, 10-13) -------------------------------------

// minGossipSample is the smallest peer sample any gossip-estimated figure on this
// surface renders over. Below it every endpoint says sampleTooSmall and publishes no
// value at all.
//
// DERIVED, not chosen for taste, and it is a PRIVACY floor as much as an honesty one:
//   - at n = 1 a Gini is 0 by construction, so the "number" carries no information at
//     all and would read as perfect equality;
//   - at n = 2 a Gini INVERTS: G = |a-b| / (2(a+b)), so publishing it beside the sample
//     size republishes the ratio of two named peers' work counters. An aggregate that
//     resolves to one other node's value is not an aggregate.
//   - 3 is the smallest sample where neither holds.
//
// The same floor gates the tier mix, where a 1-peer histogram IS that peer's band.
const minGossipSample = 3

// gossipSample is the required sibling of every gossip-estimated number on this surface
// (row 13). A field must not render without it.
type gossipSample struct {
	Size         int    `json:"size"`
	MinSize      int    `json:"minSize"`
	TooSmall     bool   `json:"tooSmall"`
	SelfIncluded bool   `json:"selfIncluded"`
	Note         string `json:"note"`
}

func newGossipSample(size int, selfIncluded bool) gossipSample {
	gs := gossipSample{Size: size, MinSize: minGossipSample, TooSmall: size < minGossipSample, SelfIncluded: selfIncluded}
	if gs.TooSmall {
		gs.Note = "sample too small: fewer than " + itoa(minGossipSample) + " nodes have gossiped a capacity pledge, and a Gini over two values is the ratio of those two values. No estimate is published"
	} else {
		gs.Note = "gossip-estimated over this node's local peer sample; self-reported figures, advisory only, never a consensus or standing input"
	}
	return gs
}

// ---- the privacy clause on the two gossip-estimated routes ---------------------------
//
// THE BREAK THIS CLOSES (blind PE ruling B3 and red-team REDTEAM-c3-gossip-disclosure-
// f03ab50-2026-09-09 F1, both measured, independently). The first cut of these two routes
// was OPEN, on the argument — written into r29a_status_surface_test.go and the build doc —
// that self's own bytes being one term inside the sample made the aggregate safe. That
// argument is FALSE and both seats produced a working solve.
//
// A published Gini plus its sample size is ONE EQUATION. minGossipSample bounds the sample
// SIZE; it does not bound the number of terms the READER does not already know. Identity
// is free (M0), so an adversary furnishes n-1 of the n terms with sybils gossiping chosen
// ServedBytes and a classifiable CapTotal, and solves for the one term left. Measured
// end-to-end on the real Node.EconomySample() and the real credit.Gini: 4 sybils declaring
// 1,000,000 recovered a planted secret of 7,777,777 EXACTLY.
//
// The recovered quantity is the node-wide serve counter that -privacy — the compiled
// default, privacyDefaultWithheld — nils for precisely this reader in readerView, and drops
// with the whole economyRevenue block in privacyWithheldEconomySelf. So an open route was
// handing an unauthenticated reader the number the privacy default exists to withhold.
//
// THREE AMPLIFICATIONS that decide the shape of the fix:
//  1. THE RECOVERED TERM NEED NOT BE SELF. Any node with an open route is an ORACLE for its
//     PEERS' withheld counters: a web client that never peers with V reads V's counter
//     through A. So "drop self from the sample" is not a fix — it moves the target.
//  2. The counters are MONOTONE CUMULATIVE, so re-solving over time yields a per-node
//     serve/repair RATE. An activity timeline, not a snapshot.
//  3. RepairGini fingerprints the few load-bearing repairers, which is eclipse-targeting
//     material.
//
// THE SHAPE, and why it is the minimal one. These two documents now honour the SAME privacy
// clause as the rest of the read surface — `auth.privacy && !auth.token`, byte for byte the
// predicate readerView uses for the node-wide counters — and the marker is the SAME
// countersWithheld, because this is the same covered set one derivation removed. Tighter
// (withhold unconditionally) would withhold a derivation of numbers a -privacy=off node
// publishes raw two routes over, which closes nothing and invites someone to open the wrong
// one later. Looser (withhold only the Ginis) leaves the mix, which publishes the sample
// SIZE — half of the equation. Narrower still (drop self) is refuted by amplification 1.
//
// OWNER RULING 2026-09-09: the privacy default wins — empty panels on the shipped default
// are preferred over the disclosure. The cost, stated: a cross-origin observatory can no
// longer read these two panels from a -privacy=on node. The operator's own dashboard is
// unaffected, because cmd/silt/ui/app.js attaches the bearer token to every same-origin
// /api/ call.
//
// WHAT STAYS OPEN, and why each is not a withhold in name only:
//   - the published BANDS and the target ratio: constants, no measurement;
//   - estimatedNodes: the SAME number /api/status already publishes in `network`, which the
//     privacy clause does not touch. Pinned by a gate so the claim cannot rot;
//   - C2: chain-derived and committed-global — every node holds that chain — and named an
//     honest negative by the red-team pass. It is not a gossip figure at all.
func gossipWithheld(auth readerAuth) bool { return auth.privacy && !auth.token }

const gossipWithholdNote = "withheld by this node's privacy setting (-privacy=on, the default): a published Gini plus its sample size is one equation, and a reader that supplies the other terms with free identities solves it for a node-wide work counter this node withholds elsewhere. Present the API token for your own node's view, or run it with -privacy=off"

// ---- GET /api/economy/concentration (rows 6, 7, 12) ----------------------------------

type economyConcentration struct {
	Tier string `json:"tier"`
	// Sample and the two Ginis are the GOSSIP-ESTIMATED half and they move together:
	// present or absent as one set, named by CountersWithheld. Sample is a POINTER so its
	// absence is an absence — a zeroed block would publish size 0 and read as "this node
	// knows no peers", which is a different fact from "you may not see this".
	Sample     *gossipSample `json:"sample,omitempty"`
	ServeGini  *giniValue    `json:"serveGini,omitempty"`
	RepairGini *giniValue    `json:"repairGini,omitempty"`
	// CountersWithheld is the SAME marker readerView uses for the node-wide serve
	// counters, because this is the same covered set one derivation removed.
	CountersWithheld bool `json:"countersWithheld,omitempty"`
	// C2 is the committed-global standing concentration, on a different tier from the
	// two Ginis beside it and labelled so. Absent (not zeroed) with no chain.
	C2       *c2Info `json:"c2,omitempty"`
	C2Absent string  `json:"c2Absent,omitempty"`
	Note     string  `json:"note"`

	SnapshotTakenAtUnix int64 `json:"snapshotTakenAtUnix"`
	SnapshotAgeSec      int64 `json:"snapshotAgeSec"`
	SnapshotIntervalSec int64 `json:"snapshotIntervalSec"`
}

// giniValue carries Known for the same reason economyGRow does: the underlying function
// returns 0 for two different facts. credit.Gini is 0 when every sampled value is
// identical AND when they sum to zero, and the second is not a measurement — it is a
// sample in which nobody reported any work. Rendering it as 0.0000 tells a reader that
// work is perfectly evenly spread across N nodes when the truth is that N nodes said
// nothing. Known false ships the reason and NO value (Value is omitempty and left at 0,
// which is why Known is what a consumer must branch on, never the number).
type giniValue struct {
	Known      bool    `json:"known"`
	Value      float64 `json:"value,omitempty"`
	SampleSize int     `json:"sampleSize"`
	Tier       string  `json:"tier"`
	Scope      string  `json:"scope"`
	Reason     string  `json:"reason,omitempty"`
}

// The two scope strings. They are on the wire because each series is over a DIFFERENT and
// non-obvious population, and a concentration figure whose population a reader has to guess
// is unreadable — the network-wide repair Gini is ~0.99 on a healthy network, so getting the
// population wrong inverts the reading.
const (
	serveGiniScope  = "the nodes that REPORTED serve work, of every tier — every tier serves. A node that reported nothing is excluded rather than counted as a zero: both wire fields are omitempty, so a peer withholding its counters and an idle peer are identical bytes, and counting the absence as a zero would drag this toward 1.0 and read as total capture. Excluding UNDERSTATES inequality, which is the safe direction for a capture alarm. Self-reported and Sybil-settable; never an input to anything"
	repairGiniScope = "the REPAIR-CAPABLE nodes (horse + archival by capacity band) that REPORTED repair work. A network-wide repair Gini is ~0.99 by construction under D-TIERING, where transient ponies do no durability work, so it carries no signal. Same reporting-subset rule as the serve series"
)

// noWorkReported is the rendering of a Gini whose sample summed to zero.
const noWorkReported = "no work reported by this sample: every sampled node reported zero. That is not an even distribution — it is an absent measurement, and the two are different facts"

func giniOver(value float64, total int64, size int, tier, scope string) *giniValue {
	gv := &giniValue{SampleSize: size, Tier: tier, Scope: scope}
	if total <= 0 {
		gv.Reason = noWorkReported
		return gv
	}
	gv.Known, gv.Value = true, value
	return gv
}

type c2Info struct {
	Tier              string  `json:"tier"`
	NakamotoBonds     int     `json:"nakamotoBonds"`
	NakamotoOperators int     `json:"nakamotoOperators"`
	NakamotoDomains   int     `json:"nakamotoDomains"`
	Participants      int     `json:"participants"`
	DistinctDomains   int     `json:"distinctDomains"`
	TopShare          float64 `json:"topShare"`
	HHI               float64 `json:"hhi"`
	Gini              float64 `json:"gini"`
	// WeightUniformity is the companion the three weight signals are BLIND to: an
	// equal-bond split reads as maximally decentralized on HHI/Gini/TopShare and only
	// this one carries the tell. Carried because publishing three of four would show a
	// splitter as healthy.
	WeightUniformity float64 `json:"weightUniformity"`
	Note             string  `json:"note"`
}

func (s *uiServer) apiEconomyConcentration(w http.ResponseWriter, r *http.Request) {
	now := s.nowWall()
	doc, takenAt := s.statusSnapshot(now)
	var sample node.EconomySample
	var c2 *chain.C2
	s.onLoop(func() {
		sample = s.nd.EconomySample()
		if ch := s.nd.Chain(); ch != nil {
			m := ch.C2Metric()
			c2 = &m
		}
	})
	out := economyConcentrationDoc(sample, c2, s.readerAuthFor(r))
	out.SnapshotTakenAtUnix = doc.SnapshotTakenAtUnix
	out.SnapshotAgeSec = int64(now.Sub(takenAt).Seconds())
	out.SnapshotIntervalSec = doc.SnapshotIntervalSec
	writeJSON(w, &out)
}

// economyConcentrationDoc is the PURE rendering of one sample, split from the handler
// for the reason render.js exists: the honesty rules this endpoint has to obey — no
// estimate below the sample floor, the repair Gini scoped to the capable subset, C2
// absent rather than zeroed — are decisions about a VALUE, and a decision about a value
// should be testable without a node, a chain and an event loop behind it.
func economyConcentrationDoc(sample node.EconomySample, c2 *chain.C2, auth readerAuth) economyConcentration {
	out := economyConcentration{
		Tier: "gossip-estimated + committed-global",
		Note: "serve-work and repair-work are SEPARATE series on purpose: a balance Gini conflates them, and repair concentrating on the persistent tier while serving federates is exactly the drift a single number hides",
	}
	if c2 == nil {
		out.C2Absent = "no chain on this node: C2 is committed-global and needs the chain every validator holds"
	} else {
		out.C2 = &c2Info{Tier: "committed-global", NakamotoBonds: c2.NakamotoBonds, NakamotoOperators: c2.NakamotoOperators,
			NakamotoDomains: c2.NakamotoDomains, Participants: c2.Participants, DistinctDomains: c2.DistinctDomains,
			TopShare: c2.TopShare, HHI: c2.HHI, Gini: c2.Gini, WeightUniformity: c2.WeightUniformity,
			Note: "exact as of my head — every node holds this chain. Standing concentration, a different quantity from the two work Ginis beside it"}
	}
	if gossipWithheld(auth) {
		out.CountersWithheld = true
		out.Note = gossipWithholdNote
		return out
	}
	gs := newGossipSample(sample.Size, sample.SelfIncluded)
	out.Sample = &gs
	if !out.Sample.TooSmall {
		// Each series carries its OWN size against its OWN floor. The serve series is the
		// subset that REPORTED work, which is smaller than the sample and routinely below
		// the floor when the sample is not — and publishing the sample's size beside the
		// series' Gini would be the wrong sibling on the wrong number.
		if sample.ServeSampleSize >= minGossipSample {
			out.ServeGini = giniOver(sample.ServeGini, sample.ServeWorkTotal, sample.ServeSampleSize, "gossip-estimated",
				serveGiniScope)
		}
		// The repair series carries its OWN floor, against its OWN subset size. The
		// capable subset is a fraction of the sample, so a sample that clears the floor
		// routinely holds a capable subset that does not — and publishing the sample's
		// size beside the subset's Gini would be the wrong sibling on the wrong number.
		if sample.RepairSampleSize >= minGossipSample {
			out.RepairGini = giniOver(sample.RepairGini, sample.RepairWorkTotal, sample.RepairSampleSize, "gossip-estimated",
				repairGiniScope)
		}
	}
	return out
}

// ---- GET /api/economy/network (rows 10, 11, 13) --------------------------------------

type economyNetwork struct {
	Tier   string        `json:"tier"`
	Sample *gossipSample `json:"sample,omitempty"`
	// EstimatedNodes is the DHT crowd estimate. It is published even below the sample
	// floor because it is the SAME number /api/status already carries in network — but
	// it is stamped with its own tier so nobody reads it as a census.
	EstimatedNodes float64      `json:"estimatedNodes"`
	Bands          []tierBand   `json:"bands"`
	Mix            []tierMixRow `json:"mix,omitempty"`
	TargetRatio    []tierMixRow `json:"targetRatio"`
	// ObservedRatio normalises the mix to the target's archival = 1. Absent, with a
	// reason, when the sample holds no archival node: a ratio with a zero denominator
	// is not a large number, it is an unknown one.
	ObservedRatio       []tierMixRow `json:"observedRatio,omitempty"`
	ObservedRatioAbsent string       `json:"observedRatioAbsent,omitempty"`
	// CountersWithheld: see economyConcentration. The mix is withheld with the Ginis
	// because it publishes the sample SIZE, which is half of the equation the Ginis are
	// the other half of.
	CountersWithheld bool   `json:"countersWithheld,omitempty"`
	Note             string `json:"note"`

	SnapshotTakenAtUnix int64 `json:"snapshotTakenAtUnix"`
	SnapshotAgeSec      int64 `json:"snapshotAgeSec"`
	SnapshotIntervalSec int64 `json:"snapshotIntervalSec"`
}

type tierBand struct {
	Class    string `json:"class"`
	MinBytes int64  `json:"minBytes"`
	MaxBytes int64  `json:"maxBytes,omitempty"` // absent on the open-ended top band
	Source   string `json:"source"`
}

type tierMixRow struct {
	Class   string  `json:"class"`
	Sampled int     `json:"sampled,omitempty"`
	Share   float64 `json:"share,omitempty"`
	Ratio   float64 `json:"ratio,omitempty"`
}

// publishedTierBands is the wire form of core/node's capacity bands, published so an
// operator can see WHERE the classification cut and check it against the table it came
// from rather than trusting a label.
func publishedTierBands() []tierBand {
	const src = "Economist tier table, silt-reviews/economist/2026-09-01-tiered-edge-economy-sustainability-audit.md: horse = \"16+ GB disk\", archival = \"TBs\""
	return []tierBand{
		{Class: node.TierPony, MinBytes: 1, MaxBytes: node.TierPonyMaxBytes - 1, Source: src},
		{Class: node.TierHorse, MinBytes: node.TierPonyMaxBytes, MaxBytes: node.TierHorseMaxBytes - 1, Source: src},
		{Class: node.TierArchival, MinBytes: node.TierHorseMaxBytes, Source: src},
	}
}

// targetTierRatio is the ratified vision ratio (decisions.md, D-TIERING ratification
// 2026-08-31: "ponies : horses : archival ~ 10000 : 100 : 1"). Published beside the
// observed mix so the panel compares against the goal, not against a builder's memory.
func targetTierRatio() []tierMixRow {
	return []tierMixRow{{Class: node.TierPony, Ratio: 10000}, {Class: node.TierHorse, Ratio: 100}, {Class: node.TierArchival, Ratio: 1}}
}

func (s *uiServer) apiEconomyNetwork(w http.ResponseWriter, r *http.Request) {
	now := s.nowWall()
	doc, takenAt := s.statusSnapshot(now)
	var sample node.EconomySample
	s.onLoop(func() { sample = s.nd.EconomySample() })
	out := economyNetworkDoc(sample, s.readerAuthFor(r))
	out.SnapshotTakenAtUnix = doc.SnapshotTakenAtUnix
	out.SnapshotAgeSec = int64(now.Sub(takenAt).Seconds())
	out.SnapshotIntervalSec = doc.SnapshotIntervalSec
	writeJSON(w, &out)
}

// economyNetworkDoc is the PURE rendering of one sample — same split, same reason, as
// economyConcentrationDoc.
func economyNetworkDoc(sample node.EconomySample, auth readerAuth) economyNetwork {
	out := economyNetwork{
		Tier:           "gossip-estimated",
		EstimatedNodes: sample.EstimatedNodes,
		Bands:          publishedTierBands(),
		TargetRatio:    targetTierRatio(),
		Note:           "the tier class is DERIVED from the capacity pledge each peer already gossips, against the published bands below — it is not a third gossip field and not a self-declared label. Every figure is self-reported and advisory",
	}
	if gossipWithheld(auth) {
		out.CountersWithheld = true
		out.Note = gossipWithholdNote
		return out
	}
	gs := newGossipSample(sample.Size, sample.SelfIncluded)
	out.Sample = &gs
	if out.Sample.TooSmall {
		return out
	}
	order := []string{node.TierPony, node.TierHorse, node.TierArchival}
	for _, class := range order {
		n := sample.Mix[class]
		if n == 0 {
			continue // absent, never a zero row: "none in my sample" is not "none exist"
		}
		out.Mix = append(out.Mix, tierMixRow{Class: class, Sampled: n, Share: float64(n) / float64(sample.Size)})
	}
	if arch := sample.Mix[node.TierArchival]; arch == 0 {
		out.ObservedRatioAbsent = "no archival node in this sample, so the ratio has no denominator. That is an UNKNOWN ratio, not an infinite one"
	} else {
		for _, class := range order {
			out.ObservedRatio = append(out.ObservedRatio, tierMixRow{Class: class, Ratio: float64(sample.Mix[class]) / float64(arch)})
		}
	}
	return out
}

// itoa keeps the notes above free of a fmt import for one integer.
func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }
