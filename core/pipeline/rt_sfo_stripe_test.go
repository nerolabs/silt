package pipeline_test

// RED-TEAM R-SUBFRAME-SIZE-ORACLE — RT-SFO-3 (the single-shard reconstruction half) and
// RT-SFO-4 (the parity-distinctness defect). Confirmed at origin/main @ 00082b8, 2026-09-11.
// Source: silt-agent-memory/red-team/reviews/RED-TEAM-R-SUBFRAME-SIZE-ORACLE-00082b8-2026-09-11.md
//
// The idiom used for the live defect below (a PIN with TESTABLE TEETH plus a MECHANISM arm) is
// documented in full at the head of rt_sfo_manifest_oracle_test.go. RT-SFO-4 is RED on main
// today and its disposition is not settled — the red-team routed it to the Economist and the
// Researcher and explicitly declined to price it — so it is pinned, not asserted.
//
// The RT-SFO-3 column-key half lives in core/node (it needs colKey / placementKey).

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// rtSubFrameLayout publishes a sub-frame object through the REAL publisher and returns its
// opened Layout plus the store. Every stripe gate below measures THIS, not a hand-built
// stripe: "pipeline.Stage stopped emitting a one-data-shard stripe" is one of the shapes a fix
// could take, and a fixture that called erasure.EncodeStripe with one shard directly would
// construct that state itself and hide the fix. (The erasure-level arm exists too, in
// TestRT_SFO_4_PinRedensWhenTheSixthParityBecomesDistinct, but only to exercise the predicate.)
func rtSubFrameLayout(t *testing.T, size int, mode crypto.Mode) (*manifest.Layout, rtObject) {
	t.Helper()
	o := rtStage(t, size, mode, pipeline.DefaultChunkSize, 0, erasure.Params{})
	blob, err := pipeline.LoadBlob(context.Background(), o.store, o.entry)
	if err != nil {
		t.Fatalf("LoadBlob: %v", err)
	}
	l, err := manifest.OpenLayout(blob, o.handle.LayoutKey())
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	if len(l.Chunks) != 1 {
		t.Fatalf("fixture precondition failed: a %d-byte object produced %d data chunks, want 1. These gates "+
			"measure the ONE-data-shard stripe; if the publisher no longer produces one at this size, the "+
			"sub-frame boundary moved (pipeline.DataFrameSize) and RT-SFO-3/RT-SFO-4 need re-deriving, not "+
			"re-pinning.", size, len(l.Chunks))
	}
	return l, o
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-3 (MED, NEW) — every one of a sub-frame object's seven shards alone reconstructs it.
//
// A sub-frame object is ONE data shard in a k=10 stripe: pipeline.Stage calls
// erasure.EncodeStripe(p, ctChunks[lo:hi]) with hi-lo == 1, and the nine absent data shards
// are implicit zeros (erasure.ReconstructStripe fills them). So the redundancy that exists to
// buy durability also means each of the seven stored shards is individually sufficient to
// recover the ciphertext — one holder, not k, is the retrieval unit.
//
// This is a genuine property of the shipped publisher and it is asserted, not pinned: it holds
// today and a change to it is a change to the durability model, which is the Economist's.
// ══════════════════════════════════════════════════════════════════════════════

func TestRT_SFO_3_EverySubFrameShardAloneReconstructs(t *testing.T) {
	l, o := rtSubFrameLayout(t, 4096, crypto.Convergent)
	p := erasure.DefaultParams
	if l.K != p.K || l.N != p.N {
		t.Fatalf("fixture precondition: layout committed k=%d n=%d, want the shipped default k=%d n=%d", l.K, l.N, p.K, p.N)
	}

	// Pull the real stored bytes for every leaf: one data shard at column 0, six parity at
	// columns 10..15 (core/node/column.go columnAt: data leaf i -> i%k; parity leaf q -> k+q%(n-k)).
	leaves := l.Leaves()
	if len(leaves) != 1+p.ParityShards() {
		t.Fatalf("fixture precondition: %d leaves, want %d (1 data + %d parity)", len(leaves), 1+p.ParityShards(), p.ParityShards())
	}
	stored := make([][]byte, 0, len(leaves))
	for _, id := range leaves {
		c, err := o.store.Get(context.Background(), ports.ChunkID(id))
		if err != nil {
			t.Fatalf("shard %s: %v", id, err)
		}
		stored = append(stored, c.Data)
	}
	wantData := append([]byte(nil), stored[0]...)

	// Column of each stored leaf, derived the way the node derives it, so the slots line up
	// with what a real holder would hold.
	cols := []int{0}
	for q := 0; q < p.ParityShards(); q++ {
		cols = append(cols, p.K+q%p.ParityShards())
	}

	recovered := 0
	for i, col := range cols {
		shards := make([][]byte, p.N)
		shards[col] = append([]byte(nil), stored[i]...) // hold exactly ONE shard
		if err := erasure.ReconstructStripe(p, shards, 1); err != nil {
			t.Fatalf("RT-SFO-3: holding only the shard at column %d, reconstruction failed: %v. The red-team "+
				"measured 7 of 7 stored shards individually reconstructing a sub-frame object. If this is now an "+
				"error the stripe geometry changed; re-derive RT-SFO-3 and RT-SFO-4 together.", col, err)
		}
		if !bytes.Equal(shards[0], wantData) {
			t.Fatalf("RT-SFO-3: holding only column %d, reconstruction produced the wrong data shard", col)
		}
		recovered++
	}
	if recovered != len(cols) {
		t.Fatalf("RT-SFO-3: %d of %d shards individually reconstructed, want all", recovered, len(cols))
	}

	// THE BOUND, so the finding is not over-read. A stripe with TWO real data shards needs two
	// shards, not one. The single-shard sufficiency is a property of the one-data-shard stripe
	// specifically, which is why RT-SFO-4 sits next to it.
	shards := make([][]byte, p.N)
	shards[0] = bytes.Repeat([]byte{1}, 64)
	if err := erasure.ReconstructStripe(p, shards, 2); err == nil {
		t.Fatal("RT-SFO-3 bound arm: one shard reconstructed a TWO-data-shard stripe. Single-shard sufficiency " +
			"is supposed to be specific to realData=1; if it is now general, the erasure layer changed and the " +
			"durability accounting in credit.RarestShardMultiplier is wrong by more than RT-SFO-4 says.")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-4 (LOW-MED, NEW) — a sub-frame object's "six parity shards" are five.
//
// With one real data shard, two parity rows of the Reed-Solomon matrix share their column-0
// coefficient, so the shards at columns 12 and 14 are BYTE-IDENTICAL. Consequences the
// red-team named: manifest.Leaves() contains a repeated hash; two distinct DHT column keys
// point at the same bytes; a node assigned both stores one copy under content addressing; and
// durability accounting (credit.RarestShardMultiplier, the audit's per-leaf sweep) counts 7
// independent shards where 6 exist.
//
// NOT scoped to sub-frame objects: ANY object whose tail stripe holds exactly one chunk hits
// it — FileSize in (10m·(cs−8), 10m·(cs−8) + (cs−8)].
//
// THIS IS A LIVE DEFECT AND IT IS PINNED, NOT ASSERTED. The red-team declined to price it
// ("the Economist's and the Researcher's, not mine") and no disposition has been ruled. The
// pin is GREEN today because it asserts the defect is still present; see the idiom note in
// rt_sfo_manifest_oracle_test.go.
// ══════════════════════════════════════════════════════════════════════════════

// rtSFO4Pin returns "" while the defect is present (fewer distinct parity shards than
// declared) and the instruction once they are all distinct. Teeth:
// TestRT_SFO_4_PinRedensWhenTheSixthParityBecomesDistinct.
func rtSFO4Pin(declared, distinct int, dupes []string) string {
	if distinct == declared {
		return fmt.Sprintf(
			"RT-SFO-4 PIN IS RED — A ONE-DATA-SHARD STRIPE NOW HAS %d DISTINCT PARITY SHARDS, WHICH IS THE FIX.\n"+
				"  At 00082b8 it had %d of %d: columns 12 and 14 were byte-identical (RT-SFO-4), so manifest.Leaves()\n"+
				"  carried a repeated hash, two DHT column keys pointed at the same bytes, and durability accounting\n"+
				"  counted 7 independent shards where 6 existed.\n"+
				"  DO THIS, in the same commit that reddened it:\n"+
				"    1. Confirm the Researcher certified the change. The erasure geometry feeds credit.RarestShardMultiplier\n"+
				"       and the D-S7 durability economy — a durability knob that is also a security parameter is the\n"+
				"       exact shape docs/build-process.md warns about, twice.\n"+
				"    2. Confirm the Economist re-priced. The repair bounty and the audit's per-leaf sweep both assumed\n"+
				"       7 independent shards; making that true changes stored bytes per object.\n"+
				"    3. Confirm this did NOT arrive as a side effect of changing erasure.DefaultParams. Check that\n"+
				"       TestRT_SFO_3_EverySubFrameShardAloneReconstructs still reflects the intended geometry.\n"+
				"    4. Replace this pin with the positive assertion.",
			distinct, declared-1, declared)
	}
	if distinct != declared-1 {
		return fmt.Sprintf(
			"RT-SFO-4 PIN IS RED — THE DEFECT MOVED: %d distinct parity shards of %d declared, pinned at %d.\n"+
				"  Duplicates found: %v. At 00082b8 there was exactly ONE collision (columns 12 and 14).\n"+
				"  MORE collisions is a WORSENING — durability over-counts by more than one shard. FEWER is a partial\n"+
				"  fix. Either way, establish which before touching this number, and re-run the durability accounting\n"+
				"  in credit.RarestShardMultiplier against the real distinct count.",
			distinct, declared, declared-1, dupes)
	}
	if len(dupes) != 1 || dupes[0] != "col12==col14" {
		return fmt.Sprintf(
			"RT-SFO-4 PIN IS RED — the collision is still one, but it MOVED: %v, pinned at [col12==col14].\n"+
				"  The identity of the colliding columns is what decides which DHT keys alias and which holder ends up\n"+
				"  storing one copy for two assignments (core/node/column.go colKey). A moved collision means the\n"+
				"  Reed-Solomon matrix or columnAt changed; re-derive the placement consequence before re-pinning.",
			dupes)
	}
	return ""
}

// rtParityDupes reports the distinct count and the colliding column pairs for a stripe with
// realData real shards, derived from the shard BYTES rather than from any declared count.
func rtParityDupes(t *testing.T, p erasure.Params, parity [][]byte) (int, []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, s := range parity {
		seen[string(s)] = true
	}
	var dupes []string
	for i := 0; i < len(parity); i++ {
		for j := i + 1; j < len(parity); j++ {
			if bytes.Equal(parity[i], parity[j]) {
				dupes = append(dupes, fmt.Sprintf("col%d==col%d", p.K+i%p.ParityShards(), p.K+j%p.ParityShards()))
			}
		}
	}
	return len(seen), dupes
}

func TestRT_SFO_4_SingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT(t *testing.T) {
	p := erasure.DefaultParams

	// (a) THE DEFECT, measured on the REAL publisher's output. The colliding shards are the
	// same CONTENT, so under content addressing they are the same ChunkID, so the duplicate is
	// directly visible as a repeated hash in the committed Layout — which is where it does its
	// damage (manifest.Leaves feeds the audit sweep and the placement).
	l, o := rtSubFrameLayout(t, 4096, crypto.Convergent)
	parity := make([][]byte, 0, len(l.Parity))
	for _, raw := range l.ParityIDs() {
		c, err := o.store.Get(context.Background(), ports.ChunkID(raw))
		if err != nil {
			t.Fatalf("parity shard %s: %v", raw, err)
		}
		parity = append(parity, c.Data)
	}
	if len(parity) != p.ParityShards() {
		t.Fatalf("fixture precondition: the layout declares %d parity shards, want %d", len(parity), p.ParityShards())
	}
	distinct, dupes := rtParityDupes(t, p, parity)
	if msg := rtSFO4Pin(p.ParityShards(), distinct, dupes); msg != "" {
		t.Fatal(msg)
	}

	// (b) THE CONSEQUENCE, asserted where it actually bites: a REPEATED HASH in the committed
	// leaf set. Pinning only the byte-equality would miss a change that kept the bytes equal
	// but stopped them sharing a leaf (or vice versa). This arm is what ties the erasure defect
	// to the durability accounting that reads manifest.Leaves().
	leaves := l.Leaves()
	counts := map[ports.Hash]int{}
	for _, h := range leaves {
		counts[h]++
	}
	repeated := 0
	for _, c := range counts {
		if c > 1 {
			repeated++
		}
	}
	if repeated != 1 {
		t.Fatalf("RT-SFO-4 PIN IS RED (consequence arm) — the committed leaf set has %d repeated hashes, pinned "+
			"at 1. manifest.Leaves() returns %d leaves of which %d are distinct. The audit's per-leaf sweep and "+
			"credit.RarestShardMultiplier both treat leaves as independent shards; that over-count is the finding. "+
			"If this changed, re-derive the durability accounting before re-pinning.",
			repeated, len(leaves), len(counts))
	}

	// (c) THE CONTROL — and it is a control, not a pin. Stripes with 2..k real data shards have
	// all six parity distinct, so the defect is specific to realData==1. Without this arm the
	// pin would also be green if EVERY stripe collapsed, which is a far worse defect wearing
	// the same number. A non-uniform data population per stripe, so a wrong reducer (say, one
	// that compares lengths instead of bytes) cannot look right.
	for realData := 2; realData <= p.K; realData++ {
		data := make([][]byte, realData)
		for i := range data {
			data[i] = rtPayload(64, uint64(i)+100)
		}
		par, err := erasure.EncodeStripe(p, data)
		if err != nil {
			t.Fatalf("EncodeStripe realData=%d: %v", realData, err)
		}
		d, dd := rtParityDupes(t, p, par)
		if d != p.ParityShards() {
			t.Fatalf("RT-SFO-4 control arm: a stripe with %d real data shards has %d distinct parity of %d (%v). "+
				"The defect was measured SPECIFIC to realData==1; if it now reaches stripes with more real data, it "+
				"is materially larger than LOW-MED and the durability over-count applies to ordinary objects, not "+
				"just sub-frame ones. Escalate before re-pinning.", realData, d, p.ParityShards(), dd)
		}
	}
}

// TestRT_SFO_4_PinRedensWhenTheSixthParityBecomesDistinct is the TEETH. It exercises
// rtSFO4Pin directly with the post-fix, worsened, and moved-collision inputs.
func TestRT_SFO_4_PinRedensWhenTheSixthParityBecomesDistinct(t *testing.T) {
	if msg := rtSFO4Pin(6, 6, nil); msg == "" {
		t.Fatal("rtSFO4Pin stayed silent when all six parity shards became distinct — the pin cannot detect the " +
			"fix it exists to detect (simplicity rule 7)")
	}
	if msg := rtSFO4Pin(6, 4, []string{"col12==col14", "col11==col13"}); msg == "" {
		t.Fatal("rtSFO4Pin stayed silent on a WORSENING (two collisions) — durability would over-count by two " +
			"shards unnoticed")
	}
	if msg := rtSFO4Pin(6, 5, []string{"col11==col13"}); msg == "" {
		t.Fatal("rtSFO4Pin stayed silent when the collision moved to a different column pair — the DHT keys that " +
			"alias would change without anyone re-reading the placement consequence")
	}
	if msg := rtSFO4Pin(6, 5, []string{"col12==col14"}); msg != "" {
		t.Fatalf("rtSFO4Pin fired on the pinned state it is supposed to accept: %s", msg)
	}
}
