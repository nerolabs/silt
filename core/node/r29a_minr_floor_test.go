package node

// R2.9a DELTA — the node tier of the minimum-requester floor (G-BB-11 / BB-15), the
// census-is-a-superset pin (BB-18) and the dead-discriminator pin (BB-19), from
// RESEARCH CERTIFICATION
// R2.9a-Bbootstrap-DELTA-contamination-privacy-floor-clock (2026-09-04) §2.3, §1.1,
// §1.3 and the Tester gates in its §6.

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// --- BB-15: the floor, at the seam ---------------------------------------------------

// TestR29aBB15NodeSeamAppliesTheFloor is BB-15 at the node tier. Node.BBootstrap is the
// ONE route the histogram takes out of the ledger to any consumer, so the floor has to
// bite there: at R_min − 1 the object the node hands up carries no census quantity at
// all, and at R_min it carries all of them.
func TestR29aBB15NodeSeamAppliesTheFloor(t *testing.T) {
	seam := func(R int) credit.BBootstrapHistogram {
		nd, ledger, sched := r29aNode(t)
		server := ports.HashBytes([]byte("bb15-server"))
		for i := 0; i < R; i++ {
			ledger.RecordServe(server, r29aFetcher(byte(40+i)), ports.HashBytes([]byte("c")), int64(1024+i))
		}
		r29aAdvance(t, sched, 2*3600*ports.Second)
		h, ok := nd.BBootstrap()
		if !ok {
			t.Fatalf("BBootstrap reported no export at R = %d", R)
		}
		return h
	}

	below := seam(credit.BBootstrapMinRequesters - 1)
	if !below.Suppressed {
		t.Fatalf("the node seam published a census of %d, one below R_min = %d, without suppressing it", credit.BBootstrapMinRequesters-1, credit.BBootstrapMinRequesters)
	}
	if below.Requesters != 0 || below.Aged != 0 || below.Unstamped != 0 || below.Cells != nil || below.MaxOccupiedAgeEdgeNanos != 0 {
		t.Fatalf("a census quantity survived the seam below the floor: requesters %d, aged %d, unstamped %d, cells nil = %v, maxEdge %d",
			below.Requesters, below.Aged, below.Unstamped, below.Cells == nil, below.MaxOccupiedAgeEdgeNanos)
	}
	if below.ClockSource != "injected" || below.MonotonicUptimeNanos == 0 {
		t.Fatalf("suppression ate the clock apparatus: %+v", below)
	}

	at := seam(credit.BBootstrapMinRequesters)
	if at.Suppressed || at.Requesters != credit.BBootstrapMinRequesters || at.Cells == nil {
		t.Fatalf("the node seam suppressed at exactly R_min = %d: suppressed %v, requesters %d, cells nil = %v",
			credit.BBootstrapMinRequesters, at.Suppressed, at.Requesters, at.Cells == nil)
	}
}

// TestR29aBB15TheFloorIsAppliedAtTheOnlySeam is the SOURCE GATE behind BB-15's
// structural claim. The floor is worth its comment only if there is no second route from
// the ledger to a consumer: core/node/bbootstrap.go must apply WithMinRequesterFloor,
// and nothing outside core/credit may call BBootstrapSnapshot directly, or a future
// publisher would bypass the floor without touching the line that enforces it.
//
// RUNTIME GATE: TestR29aBB15NodeSeamAppliesTheFloor observes the behaviour at this seam,
// and cmd/silt's TestR29aBB15WirePayloadWithholdsTheCensusBelowTheFloor observes it on
// the wire. This gate only closes the "is there another door" question, which no runtime
// test can answer.
func TestR29aBB15TheFloorIsAppliedAtTheOnlySeam(t *testing.T) {
	src, err := os.ReadFile("bbootstrap.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "BBootstrapSnapshot().WithMinRequesterFloor()") {
		t.Fatalf("SOURCE GATE: core/node/bbootstrap.go does not contain the literal BBootstrapSnapshot().WithMinRequesterFloor() — the minimum-requester floor (G-BB-11) is applied at this seam and nowhere else, so without this call every consumer of the histogram receives the raw census")
	}
	for _, f := range []string{"../../cmd/silt/ui.go", "../../cmd/silt/daemon.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "BBootstrapSnapshot(") {
			t.Fatalf("SOURCE GATE: %s calls BBootstrapSnapshot( directly. That reader returns the UNFLOORED census; the only sanctioned route is Node.BBootstrap, which applies the minimum-requester floor", f)
		}
	}
}

// --- BB-18: the census is a SUPERSET -------------------------------------------------

// bb18Swarm is three real nodes on one simnet: a serving node with a ledger and two
// peers that will fetch from it over the wire, one on the ordinary reader path and one
// on the repair path.
type bb18Swarm struct {
	sched          *simclock.Scheduler
	srv            *Node
	viewer, repair *Node
	ledger         *credit.Ledger
}

func newBB18Swarm(t *testing.T) *bb18Swarm {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 2918, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	ledger.SetObservabilityClock(sched, func() int64 { return int64(sched.Now()) })

	mk := func(seed int64) *Node {
		id := identity.FromSeed(seed).NodeID()
		return New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())
	}
	srv, viewer, repair := mk(29181), mk(29182), mk(29183)
	srv.SetLedger(ledger) // ONLY the serving node keeps the census under test
	viewer.Bootstrap([]ports.NodeID{srv.ID()}, func() {})
	repair.Bootstrap([]ports.NodeID{srv.ID(), viewer.ID()}, func() {})
	sched.Run()
	return &bb18Swarm{sched: sched, srv: srv, viewer: viewer, repair: repair, ledger: ledger}
}

// TestR29aBB18TheCensusIsASupersetOfAnyPopulation is BB-18. A peer fetching over the
// REPAIR path lands in the serving node's census indistinguishably from a viewer.
//
// THIS GATE ASSERTS THE CONTAMINATION EXISTS. It is not a defect to fix: the census
// enumerates every authenticated wire peer whose MsgFetchChunk this node answered, and
// whether repairing/judging peers belong in the estimand's population is an OPEN OWNER
// DECISION (G-BB-9) that trades M0 against D-S7. The pin exists so a later reader cannot
// mistake the census for the population, and so nobody re-proposes separating them at
// the instrument — the certification refuted every ledger-visible discriminator (BB-19).
//
// The two arms are genuinely different client entry points: the viewer resolves a plain
// chunk by its own id and calls FetchChunk; the repairer resolves a coded shard by its
// COLUMN key through fetchStripeByColumn, which is the function repair.go and
// repairclaim.go both enter on. They converge on one MsgFetchChunk at the server, which
// is exactly why the server cannot tell them apart.
func TestR29aBB18TheCensusIsASupersetOfAnyPopulation(t *testing.T) {
	s := newBB18Swarm(t)

	// Two chunks of the SAME length, so the two fetches land in the same byte bin and
	// any difference in the published cell would have to come from the path, not the
	// size. One is a plain chunk; one is a coded shard, resident under a column key.
	plain := ports.NewChunk([]byte("a plain object chunk, fetched by a viewer......."))
	shard := ports.NewChunk([]byte("a coded shard, fetched by a repairing caretaker!"))
	if len(plain.Data) != len(shard.Data) {
		t.Fatalf("fixture: the two chunks must be the same length (%d vs %d) or the byte bin, not the path, explains any difference", len(plain.Data), len(shard.Data))
	}
	var root ports.Hash
	root[0], root[1] = 0x29, 0x18
	const col = 3
	s.srv.Store().Put(bg(), plain)
	s.srv.Store().Put(bg(), shard)
	s.srv.proofMeta[shard.ID] = proofMeta{Root: root, Index: 0, Total: 1, Column: col}
	done := false
	s.srv.AnnounceHeld(func(int) { done = true })
	s.sched.Run()
	if !done {
		t.Fatal("AnnounceHeld did not finish")
	}

	// The viewer path.
	var viewerErr error
	viewerDone := false
	s.viewer.FetchChunk(plain.ID, func(err error) { viewerErr, viewerDone = err, true })
	s.sched.Run()
	if !viewerDone || viewerErr != nil {
		t.Fatalf("the viewer fetch did not complete: done %v, err %v", viewerDone, viewerErr)
	}

	// The repair path, entered where repair.go:704 and repairclaim.go:246 enter it.
	repairDone := false
	var unfetched []ports.ChunkID
	s.repair.fetchStripeByColumn(root, []shardRef{{id: shard.ID, stripe: 0, pos: col}},
		func(un []ports.ChunkID, _ map[uint64]int) { unfetched, repairDone = un, true })
	s.sched.Run()
	if !repairDone || len(unfetched) != 0 {
		t.Fatalf("the repair-path fetch did not complete: done %v, unfetched %v", repairDone, unfetched)
	}

	// Both are in the census, and the census cannot tell them apart: same age bucket,
	// same byte bin, so they are ONE CELL WITH COUNT 2. There is no published quantity
	// that distinguishes the repairer from the viewer.
	h := s.ledger.BBootstrapSnapshot() // the RAW census: two requesters is below R_min
	if h.Requesters != 2 || h.Aged != 2 {
		t.Fatalf("census requesters/aged = %d/%d, want 2/2 — a repair-path fetch must be counted exactly like a viewer's", h.Requesters, h.Aged)
	}
	occupied := map[[2]int]int64{}
	for a := range h.Cells {
		for b, n := range h.Cells[a] {
			if n > 0 {
				occupied[[2]int{a, b}] = n
			}
		}
	}
	if len(occupied) != 1 {
		t.Fatalf("the repairer and the viewer landed in %d different cells (%v): the instrument is separating them, which the certification says it cannot do — if this is intentional it is a new published attribute and a privacy question", len(occupied), occupied)
	}
	for cell, n := range occupied {
		if n != 2 {
			t.Fatalf("cell %v carries %d, want 2", cell, n)
		}
	}
	// And the bytes really moved on both paths, so the gate is not passing on two
	// no-ops that both booked nothing.
	if _, err := s.viewer.Store().Get(bg(), plain.ID); err != nil {
		t.Fatalf("the viewer did not actually receive the chunk: %v", err)
	}
	if _, err := s.repair.Store().Get(bg(), shard.ID); err != nil {
		t.Fatalf("the repairer did not actually receive the shard: %v", err)
	}
	t.Logf("BB-18: viewer and repairer both land in cell %v with count 2 — the census is a SUPERSET of any population P, and P is unpinned (G-BB-9)", occupied)
}

// --- BB-19: the dead discriminator, on the real serve path ---------------------------

// TestR29aBB19NoLedgerVisibleDiscriminatorOnTheServePath is BB-19 at the node tier. The
// certification refuted splitting the census by `servedBytes > 0` on the artifact: the
// serve path books the bytes to the SERVING NODE, always as `n.id`, so on a serving
// ledger every remote account carries servedBytes == 0 and the only account that does
// not is the node's own — which is never in the census.
//
// It drives the real MsgFetchChunk handler rather than the ledger primitive, because the
// fact that could regress is the ARGUMENT ORDER at the two call sites in node.go, not
// the arithmetic in core/credit.
func TestR29aBB19NoLedgerVisibleDiscriminatorOnTheServePath(t *testing.T) {
	s := newBB18Swarm(t)
	chunk := ports.NewChunk([]byte("bytes that two different peers will both fetch"))
	s.srv.Store().Put(bg(), chunk)
	done := false
	s.srv.AnnounceHeld(func(int) { done = true })
	s.sched.Run()
	if !done {
		t.Fatal("AnnounceHeld did not finish")
	}
	for _, peer := range []*Node{s.viewer, s.repair} {
		got := false
		peer.FetchChunk(chunk.ID, func(error) { got = true })
		s.sched.Run()
		if !got {
			t.Fatalf("peer %s did not complete its fetch", peer.ID())
		}
	}

	// Every REMOTE account: zero. Reading through ServedBytes is safe here only because
	// each id already has an account (the serve created it); on an unseen id that reader
	// goes through acct() → Register and MINTS one.
	for _, peer := range []*Node{s.viewer, s.repair} {
		if got := s.ledger.ServedBytes(peer.ID()); got != 0 {
			t.Fatalf("remote account %s carries servedBytes = %d. The certification refuted `servedBytes > 0` as a way to separate repair traffic from viewer traffic BECAUSE it selects the empty set; if it is now non-zero for a remote account, the serve path's server argument changed and the refutation needs re-certifying", peer.ID(), got)
		}
	}
	if own := s.ledger.ServedBytes(s.srv.ID()); own <= 0 {
		t.Fatalf("the serving node's own servedBytes = %d, want > 0 — the fixture served nothing and this gate is vacuous", own)
	}
	// The one account the predicate selects is not a census member.
	h := s.ledger.BBootstrapSnapshot()
	if h.Requesters != 2 {
		t.Fatalf("census = %d, want 2 (the two fetching peers, and NOT the server)", h.Requesters)
	}
	t.Logf("BB-19 (serve path): servedBytes is 0 for both remote accounts and %d for the node's own, which is not in the census of %d",
		s.ledger.ServedBytes(s.srv.ID()), h.Requesters)
}
