package node

// R2.2 gossip half — the certified form and its two behavioural conditions.
//
// RESEARCH CERTIFICATION C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-2026-09-09: GATED,
// certifiable only as alternative A — gossip the two work fields ONLY under -privacy=off —
// under three merge conditions. M-1 and M-2 are here; M-3's route clause is in cmd/silt
// (TestR22APublishedGiniNeverReconstructsAPrivacyWithheldCounter) and its second leg is the
// self-term exclusion asserted below.
//
// WHY, in one line: the two gossiped integers are BIT-IDENTICAL to two of the three
// quantities D-UI-PRIVACY-FLAG withholds from an HTTP reader, published to a wider audience
// over a more public port at a rate the OBSERVER picks — which is the exact
// "bounded by the poll rate" non-bound D-STATUS-SNAPSHOT-INTERVAL was ratified to close.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// capturingTransport records what send actually handed the wire.
type capturingTransport struct{ last ports.Message }

func (c *capturingTransport) Send(_ ports.NodeID, m ports.Message) error   { c.last = m; return nil }
func (c *capturingTransport) SetHandler(func(ports.NodeID, ports.Message)) {}
func (c *capturingTransport) Addr() string                                 { return "capture" }
func (c *capturingTransport) Close() error                                 { return nil }

// r22PledgingNode is a node that pledges capacity (so the stamp is reached at all) and whose
// ledger holds real work.
func r22PledgingNode(t *testing.T, publish bool, served int64) (*Node, *capturingTransport) {
	t.Helper()
	var id ports.NodeID
	id[0] = 0x9C
	cfg := DefaultConfig()
	cfg.PublishWorkCounters = publish
	tr := &capturingTransport{}
	n := New(id, cfg, simclock.New(), tr, memstore.New())
	n.capRep = fixedCapacity{used: 1, total: 32 << 30}
	l := credit.New(1, 0)
	l.RecordServe(id, ports.NodeID{0x01}, ports.ChunkID{}, served)
	for i := int64(0); i < 3; i++ {
		l.PayBounty(ports.Hash{0x7}, id, 0) // no escrow: a no-op, kept so the intent is legible
	}
	n.SetLedger(l)
	if got, _ := n.selfWork(); got != served {
		t.Fatalf("precondition: selfWork served %d, want %d — the fixture must hold real work or M-1's gate is vacuous", got, served)
	}
	return n, tr
}

type fixedCapacity struct{ used, total int64 }

func (f fixedCapacity) Capacity() (int64, int64) { return f.used, f.total }

// TestR22M1WorkCountersAreGossipedOnlyWhenTheOperatorPublishesThem is the BEHAVIOURAL half
// of merge condition M-1; adapters/tcpnet's TestR22WorkGossipIsAdditiveForAnOldPeer is the
// WIRE half and states the composition from its side.
//
// THE HALF THIS GATE CLOSES, and the half its sibling closes. Here: with the withholding
// posture, send stamps EXACTLY ZERO into both fields even though the ledger holds work.
// There: adapters/tcpnet's TestR22WorkGossipIsAdditiveForAnOldPeer proves by an EXACT SCAN
// of the raw CBOR map that a zero in these two fields emits NEITHER key 29 NOR key 30 —
// deliberately not a frame-length compare, because a length compare stayed green under the
// controlled revert that deletes omitempty and had to be replaced. Composed: withholding
// posture => zero stamped => no key on the wire, and the frame is byte-identical to a
// pre-R2.2 node's.
//
// ABLATION: delete the `if n.cfg.PublishWorkCounters` guard in send and this reddens with
// the leaked byte count in the message.
func TestR22M1WorkCountersAreGossipedOnlyWhenTheOperatorPublishesThem(t *testing.T) {
	const secret = int64(987_654_321)

	withholding, wt := r22PledgingNode(t, false, secret)
	if err := withholding.send(ports.NodeID{0xFF}, ports.Message{Kind: ports.MsgFindNode}); err != nil {
		t.Fatal(err)
	}
	if wt.last.ServedBytes != 0 || wt.last.RepairsDone != 0 {
		t.Fatalf("a node on the COMPILED DEFAULT posture (-privacy=on) gossiped ServedBytes=%d RepairsDone=%d. That integer is the same one readerView nils and privacyWithheldEconomySelf drops for an unauthenticated HTTP reader — and -ui is empty by default, so this node may have no HTTP surface at all while publishing it to every DHT peer it answers",
			wt.last.ServedBytes, wt.last.RepairsDone)
	}
	// The pledge itself still rides: M-1 gates the WORK fields, not the capacity gossip
	// the tier mix and the crowd estimate are built from.
	if wt.last.CapTotal == 0 {
		t.Fatal("the capacity pledge went with the work counters; the tier mix, the bands and the crowd estimate all derive from CapTotal and are unaffected by this gate")
	}

	// The counter-arm: with -privacy=off the operator opted in and the numbers ride.
	publishing, pt := r22PledgingNode(t, true, secret)
	if err := publishing.send(ports.NodeID{0xFF}, ports.Message{Kind: ports.MsgFindNode}); err != nil {
		t.Fatal(err)
	}
	if pt.last.ServedBytes != secret {
		t.Fatalf("with -privacy=off the work counters did not ride: ServedBytes=%d, want %d. The gate above would then be vacuous — it would pass on a build that never gossips at all", pt.last.ServedBytes, secret)
	}
}

// TestR22M3SelfsWorkTermsStayOutOfTheSampleOnTheWithholdingPosture is M-3's second leg. The
// route clause in cmd/silt is what the certification requires; this makes the aggregate
// itself carry nothing to republish, so a future open route cannot reintroduce the break.
func TestR22M3SelfsWorkTermsStayOutOfTheSampleOnTheWithholdingPosture(t *testing.T) {
	const secret = int64(987_654_321)
	n, _ := r22PledgingNode(t, false, secret)
	for i := 0; i < 4; i++ {
		gossip(n, i, 32<<30, 1_000_000, 2)
	}
	s := n.EconomySample()
	if s.SelfIncluded != true {
		t.Fatalf("self left the sample entirely (Size %d). Self still pledges capacity, so it belongs in Size and Mix — only its WORK terms are withheld", s.Size)
	}
	if s.ServeWorkTotal != 4_000_000 || s.ServeSampleSize != 4 {
		t.Fatalf("serve series = %d nodes totalling %d, want 4 and 4,000,000 (the four reporting peers, self excluded). Self's %d withheld bytes are in the aggregate", s.ServeSampleSize, s.ServeWorkTotal, secret)
	}
}

// TestR22RepairsOnlySybilsSteerTheServeGiniUpwards encodes the blind PE's re-ruling
// measurement (register row R-C3-SERVEGINI-STEERABLE) as a KNOWN, DISCLOSED behaviour rather
// than leaving it to be rediscovered as a surprise.
//
// The pair discriminator admits a peer that reports ONLY repairs, and that peer lands a ZERO
// in the serve series. So free identities move the serve figure UP, not only down — which
// refuted the sentence the scope string used to carry ("excluding UNDERSTATES inequality,
// which is the safe direction"). That sentence is now corrected on the wire, and this test is
// what keeps the correction honest: if the number ever stops being steerable, the caveat is
// overclaiming and should be re-read; if it stays steerable, the caveat must stay.
//
// IT IS NOT A REGRESSION and the fix is not to change the discriminator. Before M-2 the same
// twenty sybils reported 0,0 and achieved the same reading. A sybil does not need the
// repairs-only trick at all — it can declare any positive servedBytes. The lever is that
// every term is self-reported, which is why both figures are labelled Sybil-settable in
// either direction and may never become an input to anything (research certification §3).
// Per-series membership would close this particular lever and would cost the repair alarm
// outright — see EconomySample's doc comment for that trade.
func TestR22RepairsOnlySybilsSteerTheServeGiniUpwards(t *testing.T) {
	honest := func() *Node {
		n := r22Node(t)
		for i := 0; i < 3; i++ {
			gossip(n, i, 32<<30, 1_000_000, 1)
		}
		return n
	}
	before := honest().EconomySample()
	if before.ServeGini != 0 || before.ServeSampleSize != 3 {
		t.Fatalf("baseline serveGini %.4f over %d — want 0 over 3 (three identical reporters)", before.ServeGini, before.ServeSampleSize)
	}

	steered := honest()
	const sybils = 20
	for i := 0; i < sybils; i++ {
		gossip(steered, 1000+i, 32<<30, 0, 1) // repairs only: REPORTING, zero in the serve series
	}
	after := steered.EconomySample()
	if after.ServeSampleSize != 3+sybils {
		t.Fatalf("the repairs-only peers did not enter the serve series (%d); this test measures what happens when they do", after.ServeSampleSize)
	}
	if after.ServeGini <= before.ServeGini {
		t.Fatalf("serveGini did not move (%.4f -> %.4f). The scope string on the wire claims free identities can drive this number UP as well as down; if that is no longer true the caveat is overclaiming and must be re-read", before.ServeGini, after.ServeGini)
	}
	t.Logf("R-C3-SERVEGINI-STEERABLE: %d repairs-only sybils moved serveGini %.4f -> %.4f over a perfectly even network. Disclosed on the wire, never an input to anything.",
		sybils, before.ServeGini, after.ServeGini)
}

// TestR22GossipSampleIsExactlyGiniOverSampledValues is what lets cmd/silt's reconstruction
// gate build its sample BY HAND and still be a gate about the real thing: it pins that
// EconomySample publishes exactly credit.Gini over the values it sampled, with no smoothing,
// rounding or reweighting in between. If a later edit puts anything between the sampled
// values and the published figure, the hand-built fixture over there stops corresponding to
// what a node produces and its solve would be testing arithmetic nobody ships — so this is
// the joint, and it reddens rather than letting that drift go unnoticed.
func TestR22GossipSampleIsExactlyGiniOverSampledValues(t *testing.T) {
	n := r22Node(t)
	vals := []int64{1_000_000, 1_000_000, 1_000_000, 1_000_000, 987_654_321}
	for i, v := range vals {
		gossip(n, i, 32<<30, v, 0)
	}
	s := n.EconomySample()
	if s.ServeSampleSize != len(vals) {
		t.Fatalf("serve series = %d, want %d", s.ServeSampleSize, len(vals))
	}
	if s.ServeGini != credit.Gini(vals) {
		t.Fatalf("EconomySample published serveGini %.15f but credit.Gini over the same values is %.15f. Something now sits between the sampled values and the published figure; cmd/silt's reconstruction gate builds its fixture from credit.Gini directly and would stop corresponding to a real node",
			s.ServeGini, credit.Gini(vals))
	}
}

// TestR22M2ANonReportingPeerIsExcludedNotCountedAsZero.
//
// THE MECHANISM. Both wire fields are omitempty, so a peer WITHHOLDING its counters and a
// peer that is genuinely idle are THE SAME BYTES — there is nothing at the receiver to tell
// them apart. Under M-1 the withholding posture is the default, so counting the absence as a
// zero fills the series with zeros and drives the serve Gini toward 1.0: a FALSE READING OF
// TOTAL CAPTURE on the shipped default, on a network where nothing is captured at all.
//
// This is verbatim the hazard the wire gate's own comment names — "a dropped field decodes
// as 0 and 0 is a legal value here … the Ginis would report perfect equality over a captured
// network, and nothing would error" — arriving from the other direction.
//
// THE CERTIFICATION'S ABLATION, run: flipping a peer from reporting to withholding must NOT
// move the Gini toward 1.
func TestR22M2ANonReportingPeerIsExcludedNotCountedAsZero(t *testing.T) {
	// Six peers reporting identical work: a perfectly even, fully reported network.
	reporting := r22Node(t)
	for i := 0; i < 6; i++ {
		gossip(reporting, i, 32<<30, 1_000_000, 5)
	}
	base := reporting.EconomySample()
	if base.ServeSampleSize != 6 || base.ServeGini != 0 {
		t.Fatalf("baseline: %d reporting nodes, serveGini %.4f — want 6 and 0 (identical work IS perfect equality)", base.ServeSampleSize, base.ServeGini)
	}

	// Now five of the six adopt the withholding default. On the wire that is indistinguishable
	// from five idle nodes, and the naive reading is a network where one node does everything.
	mixed := r22Node(t)
	gossip(mixed, 0, 32<<30, 1_000_000, 5)
	for i := 1; i < 6; i++ {
		gossip(mixed, i, 32<<30, 0, 0) // withholding: omitempty means these keys never arrived
	}
	got := mixed.EconomySample()
	if got.Size != 6 {
		t.Fatalf("Size = %d, want 6: a withholding peer still pledged capacity and still belongs to the crowd's SHAPE", got.Size)
	}
	if got.ServeGini > base.ServeGini {
		t.Fatalf("five of six peers switching to the WITHHOLDING DEFAULT moved serveGini from %.4f to %.4f. Nothing about the work distribution changed — they went quiet — and the panel now reads as total capture on the shipped posture (research certification M-2)",
			base.ServeGini, got.ServeGini)
	}
	if got.ServeSampleSize != 1 {
		t.Fatalf("serve series size = %d, want 1: the series is over the REPORTING subset and the size published beside the figure must be that subset's, or the sibling sizes the wrong population", got.ServeSampleSize)
	}
	if got.RepairSampleSize != 1 {
		t.Fatalf("repair series size = %d, want 1 — the same rule on the repair leg", got.RepairSampleSize)
	}

	// AND THE ALARM SURVIVES, which is the half a blanket "drop every zero" would have
	// destroyed. The discriminator is the PAIR: a peer that SERVES but has never repaired
	// emits key 29 and not key 30, so it is a reporting peer and its repair zero is a real
	// measured zero. Repair concentrated on one of six capable nodes must still redden.
	captured := r22Node(t)
	gossip(captured, 0, 32<<30, 1_000_000, 60)
	for i := 1; i < 6; i++ {
		gossip(captured, i, 32<<30, 1_000_000, 0) // serving, never repairing — REPORTING
	}
	cs := captured.EconomySample()
	if cs.RepairSampleSize != 6 {
		t.Fatalf("repair series = %d nodes, want 6. A node that reported serve work is a reporting node and its repair ZERO is a measurement; dropping it would collapse total capture into a one-element series and silence the alarm", cs.RepairSampleSize)
	}
	if cs.RepairGini <= 0.40 {
		t.Fatalf("all repair on ONE of six reporting capable nodes gives repairGini %.4f — M-2's exclusion has eaten the concentration signal it was supposed to leave alone", cs.RepairGini)
	}
	// And with one reporter the series is below minGossipSample, so the consumer publishes
	// nothing at all. "Sample too small" is one of the three legitimate renderings; a
	// Gini of 1.0 over five silent nodes is not.
	if got.ServeSampleSize >= 3 {
		t.Fatalf("a one-reporter series (%d) is at or above the consumer's floor; it must fall below it and render as a named absence", got.ServeSampleSize)
	}
}

// TestR22TheWorkStampHasExactlyOneProductionWriter closes register row
// R-C3-WIRE-SINGLE-WRITER.
//
// The M-1 gate is a COMPOSITION across two packages: adapters/tcpnet proves a Message with
// both work fields at zero emits neither CBOR key, and TestR22M1... above proves send stamps
// zero on the withholding posture. The joint that makes the composition sound is that NOTHING
// ELSE writes the field — a second writer, or a toWire that derived the value from somewhere
// other than the Message, would break the composition silently, with both halves still green.
// Nothing asserted that, so this does.
//
// RUNTIME GATE: TestR22M1WorkCountersAreGossipedOnlyWhenTheOperatorPublishesThem observes the
// behaviour (the stamped value under each posture); this gate only pins that there is one
// place where the behaviour can be changed.
func TestR22TheWorkStampHasExactlyOneProductionWriter(t *testing.T) {
	root := filepath.Join("..", "..")
	assign := regexp.MustCompile(`(?m)^[^/]*\b\w+\.(ServedBytes|RepairsDone)\s*(,[^=]*)?=[^=]`)
	writers := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "archive", "vendor", "testdata", "website", "docs":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if assign.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				writers[rel] = append(writers[rel], strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// THE AUDITED SET, enumerated as exact source lines rather than as counts per file.
	// Counting per file would let one assignment be swapped for another silently, and the
	// receiver's NAME is not what makes an assignment safe — what it is assigning to is.
	// Each line below is here because someone read it against the posture guard.
	audited := map[string]string{
		// The stamp itself, and the only place the wire value is produced. Guarded by
		// Config.PublishWorkCounters (M-1).
		"msg.ServedBytes, msg.RepairsDone = n.selfWork()": "core/node/node.go",
		// The CBOR boundary, both directions. Pure copies; they add no source of value.
		"w.ServedBytes, w.RepairsDone = m.ServedBytes, m.RepairsDone": "adapters/tcpnet/wire.go",
		"m.ServedBytes, m.RepairsDone = w.ServedBytes, w.RepairsDone": "adapters/tcpnet/wire.go",
		// EconomySelf's LOCAL-EXACT read: a different object entirely — this node's own
		// ledger counters for its own /api/economy/self document, never the wire. Listed
		// so the gate's set is the whole truth rather than a filtered view.
		"es.ServedBytes = r.ServedBytes(n.id)": "core/node/node.go",
		"es.RepairsDone = r.RepairsDone(n.id)": "core/node/node.go",
	}
	for file, lines := range writers {
		for _, line := range lines {
			want, ok := audited[line]
			if !ok {
				t.Fatalf("SOURCE GATE: %s assigns ServedBytes/RepairsDone on a line that is not in the audited set:\n\t%s\nThe M-1 privacy composition (core/node stamps zero on the withholding posture; adapters/tcpnet emits no CBOR key for a zero) rests on there being ONE place the wire value is produced — a second producer breaks it with BOTH halves still green. Read the new line against Config.PublishWorkCounters, then add it here", file, line)
			}
			if want != file {
				t.Fatalf("SOURCE GATE: the audited line\n\t%s\nmoved from %s to %s. Re-read it against the posture guard in its new home", line, want, file)
			}
		}
	}
	for line, file := range audited {
		found := false
		for _, got := range writers[file] {
			if got == line {
				found = true
			}
		}
		if !found {
			t.Fatalf("SOURCE GATE: the audited assignment\n\t%s\nis gone from %s. Either the stamp or a wire mapping was removed or rewritten, so this gate is no longer watching what its name claims", line, file)
		}
	}
}
