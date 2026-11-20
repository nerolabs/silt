package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// TestMeasure_PublishThroughputOnANormalWAN is a measurement, not a gate: it offers
// publishes to a bonded validator set at a fixed rate over a WAN that is neither
// clean nor adverse, and reports what the chain commits. Run it by name with -v.
//
// THE QUESTION. How many publishes per second does the trust plane commit, at what
// latency, before its backlog starts to grow? A target of tens of thousands of
// publishes a day is about 0.6/s on average; a busy hour may run ten times that.
//
// THE WAN. One-way latency drawn from 40-160 ms (inter-continental round trips of
// roughly 80-320 ms), no loss, 4 MiB/s per link. Frames up to 64 KiB queue apart
// from bulk ones, as the TCP transport carries them on a separate connection. Validators bond real plots and renew
// on the shipped TTL, so proof-carrying blocks share the chain with the publishes.
//
// THE ENTRY. Each entry carries a publish token of the real size: a 32-byte serial
// and k RSA-2048 signatures. The chain here does not require tokens, so it carries
// them without verifying them; TestMeasure_PublishEntryCost prices that CPU apart.
// Erasure coding, chunk transfer and the DHT are outside this number. It measures
// the consensus plane's capacity to order publishes, which is the part a single
// chain cannot scale out of.
//
// THE PACE. A validator proposes on its sync sweep, so by default a block follows
// at most one sweep after its work arrives. SCALE_EAGER_MS > 0 sets ProposeDelay,
// which offers the designee a proposal that many milliseconds after an entry arrives
// and after each commit.
//
// Knobs (environment): SCALE_VALIDATORS (default 4), SCALE_RATES (publishes per
// second, comma-separated, default "0.5,1,2,5,10,20"), SCALE_MINUTES of simulated
// time per rate (default 20), SCALE_CHUNKS manifest chunks per entry (default 1, as
// for any file below a few hundred MiB), SCALE_TOKEN_SIGS signatures per token
// (default 3), SCALE_EAGER_MS (default 0, off).
func TestMeasure_PublishThroughputOnANormalWAN(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only; run by name with -v")
	}
	nVal := envInt("SCALE_VALIDATORS", 4)
	minutes := envInt("SCALE_MINUTES", 20)
	shape := loadShape{
		chunks:    envInt("SCALE_CHUNKS", 1),
		tokenSigs: envIntOrZero("SCALE_TOKEN_SIGS", 3),
		eager:     ports.Duration(envIntOrZero("SCALE_EAGER_MS", 0)) * ports.Millisecond,
	}
	rates := []float64{0.5, 1, 2, 5, 10, 20}
	if s := os.Getenv("SCALE_RATES"); s != "" {
		rates = nil
		for _, f := range splitComma(s) {
			r, err := strconv.ParseFloat(f, 64)
			if err != nil {
				t.Fatalf("SCALE_RATES: %v", err)
			}
			rates = append(rates, r)
		}
	}
	t.Logf("validators=%d, %d simulated minutes per rate, %d manifest chunk(s) and a %d-signature token per entry, "+
		"eager proposal after %d ms (0 = off), WAN one-way 40-160 ms, 4 MiB/s per link",
		nVal, minutes, shape.chunks, shape.tokenSigs, shape.eager/ports.Millisecond)
	t.Logf("%8s %10s %9s %9s %9s %9s %10s %10s %10s %8s %8s", "offered/s", "committed/s", "lat p50", "lat p95", "lat max",
		"blocks", "blk p50", "blk max", "ent/blk", "round>0", "backlog")
	for _, rate := range rates {
		r := runPublishLoad(t, nVal, rate, minutes, shape)
		t.Logf("%8.1f %10.2f %8.1fs %8.1fs %8.1fs %9d %9.1fs %9.1fs %10.1f %8d %8d",
			rate, r.committedPerSec, r.latP50, r.latP95, r.latMax, r.blocks, r.blockP50, r.blockMax, r.entriesPerBlock,
			r.lateRounds, r.backlog)
	}
}

type loadShape struct {
	chunks, tokenSigs int
	eager             ports.Duration
}

type loadResult struct {
	committedPerSec float64
	latP50, latP95  float64
	latMax          float64
	blocks          int
	blockP50        float64
	blockMax        float64
	lateRounds      int // blocks that committed only after a round change
	entriesPerBlock float64
	backlog         int
}

func runPublishLoad(t *testing.T, nVal int, rate float64, minutes int, shape loadShape) loadResult {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.Config{
		LatencyMin:      40 * ports.Millisecond,
		LatencyMax:      160 * ports.Millisecond,
		RateBytesPerSec: 4 << 20,
		CtrlLaneBytes:   64 << 10,
	})
	type kindLoad struct {
		msgs, bytes int64
		maxWait     ports.Duration
	}
	var measuring bool
	load := map[ports.MsgKind]*kindLoad{}
	net.Observe = func(_, _ ports.NodeID, m ports.Message, wait ports.Duration) {
		if !measuring {
			return
		}
		k := load[m.Kind]
		if k == nil {
			k = &kindLoad{}
			load[m.Kind] = k
		}
		k.msgs++
		k.bytes += int64(len(m.Data))
		if wait > k.maxWait {
			k.maxWait = wait
		}
	}
	ids := make([]*identity.Identity, nVal)
	anchors := map[ports.NodeID]bool{}
	all := make([]ports.NodeID, nVal)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9700 + i))
		all[i] = ids[i].NodeID()
		anchors[all[i]] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("scale-genesis")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{
		// Byzantine quorum sizing, as the daemon defaults it on the objective path: it
		// is what makes commits final, and a final head is what lets a sync sweep ask
		// a peer only for the blocks above its own head.
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 2,
		MatureValidators: 99, BondTTLBlocks: shippedBondTTL,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}
	nodes := make([]*Node, nVal)
	for i, id := range ids {
		ncfg := DefaultConfig()
		ncfg.ProposeDelay = shape.eager
		nd := New(id.NodeID(), ncfg, sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		nodes[i] = nd
	}
	// The network's commit time for a height is the first moment any validator
	// committed it, by its own proposal or on receiving the committed block.
	firstCommit := map[uint64]ports.Time{}
	if path := os.Getenv("SCALE_TRACE"); path != "" {
		f, err := os.Create(fmt.Sprintf("%s-%g", path, rate))
		if err != nil {
			t.Fatalf("SCALE_TRACE: %v", err)
		}
		defer f.Close()
		for i, nd := range nodes {
			nd.SetLogger(&traceLog{w: f, now: sched.Now, who: fmt.Sprintf("v%d", i)})
		}
	}
	for _, nd := range nodes {
		nd.OnCommit(func(b chain.Block) {
			if _, seen := firstCommit[b.Height]; !seen {
				firstCommit[b.Height] = sched.Now()
			}
		})
	}
	for i := 1; i < nVal; i++ {
		nodes[i].Bootstrap([]ports.NodeID{all[0]}, func() {})
	}
	sched.Run()
	for i := range nodes {
		seed := make([]ports.NodeID, 0, nVal-1)
		for j := range nodes {
			if j != i {
				seed = append(seed, all[j])
			}
		}
		nodes[i].StartChainSync(seed, nil)
	}
	// Warm up: let the founding registrations commit before the load starts, so the
	// measurement is steady state rather than genesis drain.
	sched.RunUntil(sched.Now().Add(5 * (60 * ports.Second)))

	start := sched.Now()
	measuring = true
	end := start.Add(ports.Duration(minutes) * (60 * ports.Second))
	interval := ports.Duration(float64(ports.Second) / rate)
	submitted := map[ports.Hash]ports.Time{}
	seq := 0
	var tick func()
	tick = func() {
		if sched.Now() >= end {
			return
		}
		seq++
		e := mkEntry(fmt.Sprintf("scale-%g-%d", rate, seq))
		for c := 1; c < shape.chunks; c++ {
			e.ManifestChunks = append(e.ManifestChunks, ports.HashBytes([]byte(fmt.Sprintf("%d/%d", seq, c))))
		}
		e.FileSize = 64 << 10
		if shape.tokenSigs > 0 {
			serial := ports.HashBytes([]byte(fmt.Sprintf("serial-%g-%d", rate, seq)))
			tok := &ports.PublishToken{Serial: serial[:]}
			for k := 0; k < shape.tokenSigs; k++ {
				sig := make([]byte, 256)
				copy(sig, serial[:])
				sig[0] = 0x80 | byte(k) // a full-width RSA-2048 signature has its top bit set
				tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: all[k%nVal], Sig: sig})
			}
			e.Token = tok
		}
		submitted[e.Root] = sched.Now()
		nodes[seq%nVal].SubmitEntry(e, all) // publishers spread across the registries
		sched.AfterFunc(interval, tick)
	}
	sched.AfterFunc(0, tick)
	sched.RunUntil(end)
	measuring = false
	if os.Getenv("SCALE_LINKS") != "" {
		kinds := make([]ports.MsgKind, 0, len(load))
		for k := range load {
			kinds = append(kinds, k)
		}
		sort.Slice(kinds, func(i, j int) bool { return load[kinds[i]].bytes > load[kinds[j]].bytes })
		for _, k := range kinds {
			l := load[k]
			t.Logf("  %-22v %8d msgs %10.1f KiB/s across all links, worst wait %v ms", k, l.msgs,
				float64(l.bytes)/1024/(float64(minutes)*60), l.maxWait/ports.Millisecond)
		}
	}

	// Read the committed history once: when each publish committed, and the block cadence.
	type commit struct {
		at     ports.Time
		height uint64
	}
	committedAt := map[ports.Hash]commit{}
	var blockTimes []ports.Time
	lateRounds := 0
	perBlock := map[uint64]int{}
	for _, b := range nodes[0].Chain().Blocks(1) {
		ts, ok := firstCommit[b.Height]
		if !ok || ts < start || ts > end {
			continue
		}
		blockTimes = append(blockTimes, ts)
		if b.CommitRound > 0 {
			lateRounds++
		}
		for _, e := range b.Entries {
			if _, ok := submitted[e.Root]; ok {
				committedAt[e.Root] = commit{at: ts, height: b.Height}
				perBlock[b.Height]++
			}
		}
	}
	var lats []float64
	for root, c := range committedAt {
		lats = append(lats, float64(c.at-submitted[root])/float64(ports.Second))
	}
	sort.Float64s(lats)
	var gaps []float64
	for i := 1; i < len(blockTimes); i++ {
		gaps = append(gaps, float64(blockTimes[i]-blockTimes[i-1])/float64(ports.Second))
	}
	sort.Float64s(gaps)
	r := loadResult{
		committedPerSec: float64(len(committedAt)) / (float64(minutes) * 60),
		latP50:          pct(lats, 0.50),
		latP95:          pct(lats, 0.95),
		latMax:          pct(lats, 1.0),
		blocks:          len(blockTimes),
		blockP50:        pct(gaps, 0.50),
		blockMax:        pct(gaps, 1.0),
		lateRounds:      lateRounds,
		backlog:         len(submitted) - len(committedAt),
	}
	if len(perBlock) > 0 {
		r.entriesPerBlock = float64(len(committedAt)) / float64(len(perBlock))
	}
	return r
}

func pct(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	i := int(p * float64(len(xs)-1))
	return xs[i]
}

func envIntOrZero(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v >= 0 {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	return append(out, cur)
}

// TestMeasure_PublishEntryCost is a measurement, not a gate: it reports what one real
// publish costs the trust plane, so the throughput measurement above can be read in
// real entries rather than synthetic ones. Run it by name with -v.
//
// SIZE. The entry comes from the real staging pipeline for a few file sizes, and the
// token is built with real RSA-2048 blind signatures from k issuers. Two sizes are
// reported: what the block's entry budget CHARGES (the entry encoded alone, inside an
// otherwise empty block) and what the entry actually ADDS to a block of its peers.
//
// TIME. Issuing one signature is one RSA-2048 private-key operation per validator per
// token; verifying a token is k public-key operations, run by every validator on every
// entry it validates (at submit, at fold and at commit).
func TestMeasure_PublishEntryCost(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only; run by name with -v")
	}
	ctx := context.Background()
	for _, size := range []int{1 << 10, 1 << 20, 16 << 20} {
		store := memstore.New()
		data := make([]byte, size)
		_, _ = rand.Read(data)
		_, e, err := pipeline.Stage(ctx, store, bytes.NewReader(data), pipeline.Options{
			ChunkSize: pipeline.DefaultChunkSize, Mode: crypto.Private, Rand: rand.Reader,
		})
		if err != nil {
			t.Fatalf("stage %d: %v", size, err)
		}
		blob, err := pipeline.LoadBlob(ctx, store, e)
		if err != nil {
			t.Fatalf("manifest %d: %v", size, err)
		}
		t.Logf("file %8d B: %d manifest chunk(s), sealed manifest %d B, entry without token: charged %d B, adds %d B",
			size, len(e.ManifestChunks), len(blob), len(entryEncode(e)), marginalEntryBytes(e))
	}

	const maxK = 7
	keys := make([]*rsa.PrivateKey, maxK)
	for i := range keys {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = k
	}
	serial, _ := blindtoken.NewSerial(rand.Reader)
	tok := ports.PublishToken{Serial: serial}
	var signTotal time.Duration
	for i, k := range keys {
		blinded, secret, err := blindtoken.Blind(rand.Reader, &k.PublicKey, serial)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		bsig, err := blindtoken.SignBlinded(rand.Reader, k, blinded)
		signTotal += time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		sig, err := blindtoken.Unblind(&k.PublicKey, serial, bsig, secret)
		if err != nil {
			t.Fatal(err)
		}
		tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: ports.HashBytes([]byte{byte(i)}), Sig: sig})
	}
	base := mkEntry("entry-cost")
	for _, k := range []int{2, 3, 5, 7} {
		e := base
		tk := ports.PublishToken{Serial: tok.Serial, Sigs: tok.Sigs[:k]}
		e.Token = &tk
		t.Logf("token k=%d: entry charged %d B, adds %d B; a 64 KiB entry budget holds %d", k,
			len(entryEncode(e)), marginalEntryBytes(e), (64<<10)/len(entryEncode(e)))
	}

	const verifies = 200
	start := time.Now()
	for i := 0; i < verifies; i++ {
		if !blindtoken.Verify(&keys[0].PublicKey, tok.Serial, tok.Sigs[0].Sig) {
			t.Fatal("a real signature did not verify")
		}
	}
	verifyEach := time.Since(start) / verifies
	t.Logf("issue: %v per blind signature (RSA-2048 private-key operation, mean of %d)", signTotal/maxK, maxK)
	t.Logf("verify: %v per signature; a k=3 token costs %v per validation", verifyEach, 3*verifyEach)
}

// marginalEntryBytes is what one more entry of e's shape adds to a block that already
// holds 63 of its peers.
func marginalEntryBytes(e ports.Entry) int {
	block := func(n int) int {
		b := chain.Block{Version: chain.BlockVersion}
		for i := 0; i < n; i++ {
			x := e
			x.Root = ports.HashBytes([]byte(fmt.Sprintf("peer-%d", i)))
			b.Entries = append(b.Entries, x)
		}
		return len(chain.Encode(&b))
	}
	return block(64) - block(63)
}

// traceLog writes every node event at Info and above, stamped with simulated time,
// so a slow height can be read back event by event.
type traceLog struct {
	w   io.Writer
	now func() ports.Time
	who string
}

func (l *traceLog) Enabled(lvl ports.LogLevel) bool { return lvl >= ports.LogInfo }

func (l *traceLog) Log(_ ports.LogLevel, event string, kv ...any) {
	fmt.Fprintf(l.w, "%9.3f %s %s %v\n", float64(l.now())/float64(ports.Second), l.who, event, kv)
}
