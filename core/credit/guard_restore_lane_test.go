package credit

// R-GUARD-RESTORE-LANE-UNKNOWN — the guard's per-lane live counts must survive a
// restart. PE ruling
// /Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R2.9-deposit-at-anchor-expiry-4d4a90c-2026-09-07.md
// §4 measured it: "lanes before restart (delivery, relay) = (2, 1); after restore =
// (3, 0)" — LoadPaidSerials rebuilt every entry as laneDelivery because the durable
// record carried no lane, so RestoredGuardEntries and LivePaidSerialsByLane both
// conflated the two populations. The lane is observability only (no accounting rule
// reads it), which is exactly why nothing else went red.
//
// ABLATION (run RED once, 2026-09-08): drop the Relay term from the restore in
// LoadPaidSerials (`fresh[...] = paidSerialEntry{server: e.Server, epoch: e.Epoch}`)
// and this gate fails with "after restart (3, 0), want (2, 1)".

import "testing"

// TestRestoredGuardEntriesKeepTheirLane is the mixed-population restart. Two delivery
// anchors and one relay anchor go into one shared guard; a fresh ledger loaded from
// the same store must report the SAME split, not the whole population as delivery.
func TestRestoredGuardEntriesKeepTheirLane(t *testing.T) {
	store := &memStore{}
	server := id(1)

	l1 := New(r214Fee, r214Grant)
	l1.SetPaidSerialStore(store)
	if err := l1.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if face, reason := l1.SpendDeliveryAnchors(server, anchorsAt(0, 0, 2)); face != 2*r214Fee {
		t.Fatalf("delivery open: (face %d, reason %q), want (%d, \"\")", face, reason, 2*r214Fee)
	}
	if face, reason := l1.SpendRelayAnchors(anchorsAt(0, 2, 1)); face != r214Fee {
		t.Fatalf("relay open: (face %d, reason %q), want (%d, \"\")", face, reason, r214Fee)
	}
	if d, r := l1.LivePaidSerialsByLane(); d != 2 || r != 1 {
		t.Fatalf("before restart (%d, %d), want (2, 1)", d, r)
	}

	// The durable record must carry the lane, or the restore below has nothing to
	// read: assert the store's own view, not just the ledger's.
	var relayRecords int
	for _, e := range store.entries {
		if e.Relay {
			relayRecords++
		}
	}
	if relayRecords != 1 {
		t.Fatalf("the durable store holds %d relay-marked records of %d, want 1 — the lane is not persisted, so no restore can recover it",
			relayRecords, len(store.entries))
	}

	// Restart: a fresh ledger, the same store.
	l2 := New(r214Fee, r214Grant)
	l2.SetPaidSerialStore(store)
	if err := l2.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if d, r := l2.LivePaidSerialsByLane(); d != 2 || r != 1 {
		t.Fatalf("after restart (%d, %d), want (2, 1) — the restore conflates the two populations (R-GUARD-RESTORE-LANE-UNKNOWN)", d, r)
	}

	// RestoredGuardEntries is the delivery-lane statistic an operator reads at boot
	// (DeliverySettlementStats). It counts entries whose session state did not
	// survive, so a relay anchor — which never had a session or a deposit — must not
	// inflate it.
	if got := l2.DeliverySettlementStats().RestoredGuardEntries; got != 2 {
		t.Fatalf("RestoredGuardEntries = %d, want 2 (the two delivery anchors; the relay anchor lost no deposit)", got)
	}
}
