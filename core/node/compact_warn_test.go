package node

// R2.13 / R-COMPACT-ORPHAN owed-after (Lane C10): the BENIGN compaction-failure class
// had no daemon WARN line — CompactFailures / LastCompactError were counters read by
// nothing outside tests (PE ruling RULING-R2.13-compact-orphan-11396f1-2026-09-03.md).
// These gates pin the node-side surface: the "paid-serial guard compaction failed"
// marker is emitted ONCE PER NEW FAILURE on the paths that run the guard's expiry sweep
// (the periodic SweepDeliverySessions and the anchor-spending handlers), never once per
// call, and the same numbers ride DeliverySettlementStats for /api/status.
//
// The failure is induced with a PaidSerialStore double whose Compact returns an error
// (the R2.13 handle clause allows exactly this: the store stays appendable, the log a
// superset of the live set). Entries are PRELOADED at two epochs so two sweeps each
// remove something and each compaction fails — no key schedule beyond epoch 0 is
// needed. Ablation record (2026-09-08): with the n.logf in logCompactFailures removed
// the first gate fails at "no ... line after the first failed compaction"; restored, GREEN.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

const compactFailMarker = "paid-serial guard compaction failed"

// failingCompactStore is an in-memory PaidSerialStore whose Compact fails while
// failCompact is set. Append stays durable-and-reachable, as the port's handle
// clause requires of a store that refused a compaction.
type failingCompactStore struct {
	entries     []ports.PaidSerial
	failCompact bool
	compacts    int
}

func (m *failingCompactStore) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), m.entries...), nil
}
func (m *failingCompactStore) Append(p ports.PaidSerial) error {
	m.entries = append(m.entries, p)
	return nil
}
func (m *failingCompactStore) Compact(live []ports.PaidSerial) error {
	m.compacts++
	if m.failCompact {
		return errors.New("compact: rename of the temp file failed (simulated)")
	}
	m.entries = append([]ports.PaidSerial(nil), live...)
	return nil
}

func serialFor(b byte) []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = b
	}
	return s
}

// newCompactRig is the guard-full rig with the node's ledger REPLACED by a real ledger
// that carries the failing store (attached, then loaded — the daemon's order) and an
// injected epoch source the test moves by hand. Entries at epoch 0 and epoch 5 are
// preloaded so the sweeps at epoch 5 and epoch 10 each remove one and compact.
func newCompactRig(t *testing.T) (*guardFullRig, *credit.Ledger, *failingCompactStore, *uint64) {
	t.Helper()
	r := newGuardFullRig(t)
	store := &failingCompactStore{failCompact: true, entries: []ports.PaidSerial{
		{Serial: serialFor(0xa1), Server: r.server.NodeID(), Epoch: 0},
		{Serial: serialFor(0xb2), Server: r.server.NodeID(), Epoch: credit.PaidSerialWindow + 1},
	}}
	epoch := new(uint64)
	ledger := credit.New(50_000, 0)
	ledger.SetPaidSerialStore(store)
	if err := ledger.LoadPaidSerials(); err != nil {
		t.Fatalf("setup: load the preloaded guard: %v", err)
	}
	ledger.SetEpochSource(f8EpochFunc(func() uint64 { return *epoch }))
	ledger.Register(r.server.NodeID())
	r.nd.SetLedger(ledger)
	return r, ledger, store, epoch
}

func (c *levelCaptureLog) count(event string) int {
	n := 0
	for _, e := range c.events {
		if e == event {
			n++
		}
	}
	return n
}

// TestBenignCompactionFailureLogsTheWarnMarkerOncePerFailure: the periodic sweep path
// (SweepDeliverySessions → ReleaseDueRefunds → the ledger's epoch-band advance → the
// expiry sweep → Compact). One failed compaction ⇒ exactly one WARN carrying the
// ledger's count and the error; a repeat sweep with no new failure ⇒ no new line; a
// second failed compaction ⇒ a second line with the count at 2. The status surface
// carries the same numbers.
func TestBenignCompactionFailureLogsTheWarnMarkerOncePerFailure(t *testing.T) {
	r, ledger, store, epoch := newCompactRig(t)

	// Epoch 5: the epoch-0 entry leaves the window, the sweep compacts, the store refuses.
	*epoch = credit.PaidSerialWindow + 1
	r.nd.SweepDeliverySessions()
	if store.compacts != 1 || ledger.CompactFailures() != 1 {
		t.Fatalf("setup: compacts=%d CompactFailures=%d after the first expiring sweep, want 1/1", store.compacts, ledger.CompactFailures())
	}
	kv, lvl, ok := r.lg.last(compactFailMarker)
	if !ok {
		t.Fatalf("no %q line after the first failed compaction: the benign compaction-failure class is silent to the operator (R2.13 owed-after)\nlines seen: %v", compactFailMarker, r.lg.events)
	}
	if lvl != ports.LogWarn {
		t.Fatalf("%q at level %v, want WARN", compactFailMarker, lvl)
	}
	if got := kv["compact_failures"]; got != int64(1) {
		t.Fatalf("%q: compact_failures=%v, want the ledger's counter 1", compactFailMarker, got)
	}
	if e, _ := kv["error"].(string); !strings.Contains(e, "simulated") {
		t.Fatalf("%q: error=%q, want the store's error text", compactFailMarker, e)
	}
	if n := r.lg.count(compactFailMarker); n != 1 {
		t.Fatalf("%d %q lines after one failure, want exactly 1", n, compactFailMarker)
	}

	// Same epoch, another sweep: nothing new expired, nothing compacted, no new line.
	r.nd.SweepDeliverySessions()
	r.nd.SweepDeliverySessions()
	if store.compacts != 1 {
		t.Fatalf("setup: a same-epoch sweep compacted again (%d)", store.compacts)
	}
	if n := r.lg.count(compactFailMarker); n != 1 {
		t.Fatalf("%d %q lines with no new failure, want still 1 — the WARN must key on the count, not fire per sweep", n, compactFailMarker)
	}

	// Epoch 10: the epoch-5 entry leaves the window; a second failed compaction, a second line.
	*epoch = 2*credit.PaidSerialWindow + 2
	r.nd.SweepDeliverySessions()
	if store.compacts != 2 || ledger.CompactFailures() != 2 {
		t.Fatalf("setup: compacts=%d CompactFailures=%d after the second expiring sweep, want 2/2", store.compacts, ledger.CompactFailures())
	}
	kv, _, _ = r.lg.last(compactFailMarker)
	if n := r.lg.count(compactFailMarker); n != 2 || kv["compact_failures"] != int64(2) {
		t.Fatalf("after the second failure: %d lines, last compact_failures=%v; want 2 lines and 2", n, kv["compact_failures"])
	}

	// The status surface carries the same numbers (the marker alone is not a surface).
	ds := r.nd.DeliverySettlementStats()
	if ds.CompactFailures != 2 || !strings.Contains(ds.LastCompactError, "simulated") {
		t.Fatalf("DeliverySettlementStats = {CompactFailures:%d LastCompactError:%q}, want 2 and the store's error", ds.CompactFailures, ds.LastCompactError)
	}

	// Once the store compacts again, no further line: the marker is per failure, not per sweep.
	store.failCompact = false
	r.nd.SweepDeliverySessions()
	if n := r.lg.count(compactFailMarker); n != 2 {
		t.Fatalf("%d lines after a healthy sweep, want still 2", n)
	}
}

// TestCompactionFailureOnTheAnchorSpendPathIsLogged: the anchor spend inside a wire
// open runs the same epoch-band advance, so a compaction that fails THERE is logged by
// the handler — even though this particular open is then refused (its epoch-0 anchor is
// backdated at the ledger's new watermark). A second refused open logs nothing new.
func TestCompactionFailureOnTheAnchorSpendPathIsLogged(t *testing.T) {
	r, ledger, store, epoch := newCompactRig(t)
	fid := r.fetcher.NodeID()
	*epoch = credit.PaidSerialWindow + 1

	r.nd.handle(fid, ports.Message{Kind: ports.MsgDeliveryOpen, Ephemeral: true,
		Data: wireBlob(t, demand.SignSessionOpen(r.fetcher.Signer(), r.server.NodeID(), []demand.Token{r.token(t)}))})
	if store.compacts != 1 || ledger.CompactFailures() != 1 {
		t.Fatalf("setup: the open's anchor spend did not run the failing sweep (compacts=%d failures=%d)", store.compacts, ledger.CompactFailures())
	}
	if r.nd.LiveDeliverySessions() != 0 {
		t.Fatal("setup: the backdated open was admitted")
	}
	kv, lvl, ok := r.lg.last(compactFailMarker)
	if !ok || lvl != ports.LogWarn || kv["compact_failures"] != int64(1) {
		t.Fatalf("handler path: marker present=%v level=%v compact_failures=%v, want WARN with 1\nlines seen: %v", ok, lvl, kv["compact_failures"], r.lg.events)
	}
	r.nd.handle(fid, ports.Message{Kind: ports.MsgDeliveryOpen, Ephemeral: true,
		Data: wireBlob(t, demand.SignSessionOpen(r.fetcher.Signer(), r.server.NodeID(), []demand.Token{r.token(t)}))})
	if n := r.lg.count(compactFailMarker); n != 1 {
		t.Fatalf("%d lines after a second refused open with no new failure, want 1", n)
	}
}
