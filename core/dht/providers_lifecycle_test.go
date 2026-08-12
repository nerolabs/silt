package dht

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// P0-1 (#277 dead-peer envelope): the provider store must age out a departed
// holder's record — filter it on read (Live), reclaim it (Evict), and prune a
// confirmed-dead holder from keys where a live alternative exists (RemoveIfNotSole)
// WITHOUT orphaning a sole holder's content. These are the failing-first regressions
// for the substrate dial-storm: before the fix the store returned every record
// forever regardless of freshness, so a departed holder was re-dialed once per
// deadUntil cooldown indefinitely.

func rec(id byte, key ports.Hash, expiry int64) ports.ProviderRecord {
	var n ports.NodeID
	n[0] = id
	return ports.ProviderRecord{Key: key, ID: n, Expiry: expiry}
}

func hasID(recs []ports.ProviderRecord, id byte) bool {
	var want ports.NodeID
	want[0] = id
	for _, r := range recs {
		if r.ID == want {
			return true
		}
	}
	return false
}

func TestLiveFiltersExpiredRecords(t *testing.T) {
	p := NewProviders()
	key := ports.Hash{0xAB}
	now := int64(1000)
	p.Add(rec(1, key, now+500)) // fresh lease
	p.Add(rec(2, key, now-1))   // lapsed lease (departed holder)
	p.Add(rec(3, key, 0))       // never-expires (unsigned/legacy)

	live := p.Live(key, now)
	if hasID(live, 2) {
		t.Fatalf("Live must drop the lapsed record: got %d records including the corpse", len(live))
	}
	if !hasID(live, 1) || !hasID(live, 3) {
		t.Fatalf("Live must keep fresh and never-expiring records; got %d", len(live))
	}
	// Get stays the raw, unfiltered accessor — all three still present.
	if len(p.Get(key)) != 3 {
		t.Fatalf("Get must remain unfiltered (raw store): got %d, want 3", len(p.Get(key)))
	}
}

func TestEvictDropsExpiredAndReclaimsEmptyKeys(t *testing.T) {
	p := NewProviders()
	k1, k2 := ports.Hash{0x01}, ports.Hash{0x02}
	now := int64(1000)
	p.Add(rec(1, k1, now+500)) // fresh
	p.Add(rec(2, k1, now-1))   // lapsed
	p.Add(rec(3, k2, now-1))   // lapsed — k2's only record

	if n := p.Evict(now); n != 2 {
		t.Fatalf("Evict should remove 2 lapsed records, removed %d", n)
	}
	if hasID(p.Get(k1), 2) {
		t.Fatal("k1's lapsed record survived Evict")
	}
	if !hasID(p.Get(k1), 1) {
		t.Fatal("k1's fresh record must survive Evict")
	}
	if len(p.Get(k2)) != 0 {
		t.Fatal("k2 had only a lapsed record — it must be fully reclaimed")
	}
	if p.Len() != 1 {
		t.Fatalf("only k1 should remain a live key, got Len=%d", p.Len())
	}
}

func TestRemoveIfNotSolePrunesDeadButKeepsSoleHolder(t *testing.T) {
	p := NewProviders()
	replicated := ports.Hash{0x01} // dead holder #9 has a live sibling #1 here
	soleKey := ports.Hash{0x02}    // dead holder #9 is the ONLY provider here
	p.Add(rec(1, replicated, 0))
	p.Add(rec(9, replicated, 0))
	p.Add(rec(9, soleKey, 0))

	var dead ports.NodeID
	dead[0] = 9
	removed := p.RemoveIfNotSole(dead)
	if removed != 1 {
		t.Fatalf("expected to prune the dead holder from the 1 replicated key, removed %d", removed)
	}
	if hasID(p.Get(replicated), 9) {
		t.Fatal("a confirmed-dead holder must be pruned from a replicated key (live sibling exists)")
	}
	if !hasID(p.Get(replicated), 1) {
		t.Fatal("the live sibling must remain")
	}
	// #69 availability: the SOLE holder's record is kept so its content stays
	// discoverable (re-probeable) rather than orphaned.
	if !hasID(p.Get(soleKey), 9) {
		t.Fatal("a SOLE dead holder must be KEPT — orphaning it makes its content undiscoverable (#69)")
	}
}
