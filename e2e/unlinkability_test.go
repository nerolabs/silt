package e2e

// Publishing is unlinkable and no surveillance artifact exists, measured against an
// adversary that holds everything: every node's disk, seized after the run, every
// announcement the DHT will answer for, and the chain.
//
// Two games run on one network.
//
//   - MATCHING. Publishers publish through their own UI, interleaved. For every root
//     the adversary names the publishers its evidence points at — a publisher that
//     announced something about the root, a publisher whose disk holds the root's
//     identifiers, or a publisher identity written on the same record as them — and
//     guesses among them (uniformly among all publishers when it has nothing). Its
//     expected score is compared with chance. A control publisher runs with
//     -care-published=true, the behaviour that names the publisher, and the adversary
//     must find it: that is what shows the game can score a leak at all.
//
//   - SWEEP. Fetchers that joined after every publish fetch every object repeatedly
//     through their own UI. No record anywhere outside a fetcher's own store may carry
//     a fetcher's identity beside an object's identifiers, and no announcement may name
//     a fetcher. Inside its own store, only the debug log may name what it read. A
//     control fetcher runs with -serve-fetched=true and must be named.
//
// Every daemon runs at -debug, so the logs are the most a node writes, not the least.

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// ulNode is one daemon of the rig: its identity and where its disk is.
type ulNode struct {
	name  string
	id    string // NodeID, hex
	store string
	ui    string // base URL, empty when the node has no UI
	token string
	d     *daemon
}

// ulObject is one published object and everything that names it.
type ulObject struct {
	link      string
	handle    link.Handle
	publisher int // index into the publishers slice
	payload   []byte
	needles   [][]byte // root, manifest chunk IDs, data and parity chunk IDs: hex and raw
	announced map[string][]string
}

// reDebugRecord is a debug.log line at the debug level.
var reDebugRecord = regexp.MustCompile(`^\S+ debug `)

var reAnnounceLine = regexp.MustCompile(`^(manifest [0-9a-f]{64}|care|column -?\d+): ?(.*)$`)

func TestPublishersAndFetchersCannotBeLinkedToTheirObjects(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	rng := rand.New(rand.NewSource(11))

	// ── The network. ────────────────────────────────────────────────────────────
	vStore := t.TempDir()
	v := startDaemon(t, "V",
		"-listen", "127.0.0.1:0", "-store", vStore,
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "9801", "-debug")
	vp := v.waitFor(t, rePeer, 20*time.Second)
	regRef := v.waitFor(t, reRegistry, 20*time.Second)[1]
	boot := vp[1] + "@" + vp[2]
	all := []*ulNode{{name: "V", id: vp[1], store: vStore, d: v}}

	start := func(name, seed string, ui bool, extra ...string) *ulNode {
		store := t.TempDir()
		args := []string{"-listen", "127.0.0.1:0", "-store", store, "-bootstrap", boot,
			"-registry", regRef, "-capacity", "1G", "-mdns=false", "-id-seed", seed, "-debug"}
		if ui {
			args = append(args, "-ui", "127.0.0.1:0")
		}
		d := startDaemon(t, name, append(args, extra...)...)
		n := &ulNode{name: name, id: d.waitFor(t, rePeer, 20*time.Second)[1], store: store, d: d}
		d.waitFor(t, reBootstrap, 20*time.Second)
		if ui {
			m := d.waitFor(t, reUI, 20*time.Second)
			n.ui, n.token = "http://"+m[1], m[2]
		}
		all = append(all, n)
		return n
	}
	for i := 0; i < 4; i++ {
		start(fmt.Sprintf("S%d", i+1), fmt.Sprint(9811+i), false)
	}
	var publishers []*ulNode
	for i := 0; i < 4; i++ {
		publishers = append(publishers, start(fmt.Sprintf("P%d", i+1), fmt.Sprint(9821+i), true))
	}
	const ctrlPub = 4
	// The control cares for what it publishes, with the economy on so it also announces
	// under the object's care key, which placement never uses: an unambiguous announcement.
	publishers = append(publishers, start("Pc", "9829", true, "-care-published=true", "-economy"))

	// ── Publish: three rounds, the publishers in a fresh order each round. ─────────
	var objects []*ulObject
	for round := 0; round < 3; round++ {
		order := rng.Perm(len(publishers))
		for _, pi := range order {
			if pi == ctrlPub && round == 2 {
				continue // two control objects are enough to show the game scores a leak
			}
			p := publishers[pi]
			payload := make([]byte, 30_000+rng.Intn(670_000))
			rng.Read(payload)
			status, body, err := uiPublish(p.ui, p.token, fmt.Sprintf("o%d.bin", len(objects)), payload)
			if err != nil || status != http.StatusOK {
				t.Fatalf("%s publish: status=%d err=%v body=%s", p.name, status, err, body)
			}
			var pub struct {
				Link string `json:"link"`
			}
			_ = json.Unmarshal([]byte(body), &pub)
			h, err := link.Parse(pub.Link)
			if err != nil {
				t.Fatalf("%s publish returned no link: %s", p.name, body)
			}
			objects = append(objects, &ulObject{link: pub.Link, handle: h, publisher: pi, payload: payload})
		}
	}

	atPublish := make([]string, len(all))
	for i, n := range all {
		atPublish[i] = n.id
	}

	// ── Fetch-heavy run: fetchers that joined after every publish, so none can have
	// been a placement target, each read every object twice through its own UI. ─────
	var fetchers []*ulNode
	for i := 0; i < 3; i++ {
		fetchers = append(fetchers, start(fmt.Sprintf("F%d", i+1), fmt.Sprint(9831+i), true))
	}
	const ctrlFetch = 3
	fetchers = append(fetchers, start("Fc", "9839", true, "-serve-fetched=true"))
	client := &http.Client{Timeout: 3 * time.Minute}
	fetches := 0
	for pass := 0; pass < 2; pass++ {
		for _, fi := range rng.Perm(len(fetchers)) {
			f := fetchers[fi]
			for _, oi := range rng.Perm(len(objects)) {
				o := objects[oi]
				var got []byte
				deadline := time.Now().Add(60 * time.Second)
				for {
					resp, err := client.Get(f.ui + "/api/fetch?link=" + url.QueryEscape(o.link))
					if err == nil {
						got, _ = io.ReadAll(resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Fatalf("%s fetch of object %d never succeeded: err=%v last=%q", f.name, oi, err, lastLine(string(got)))
					}
					time.Sleep(time.Second) // the entry may still be committing
				}
				if !bytes.Equal(got, o.payload) {
					t.Fatalf("%s fetched object %d: %d bytes, not the %d published", f.name, oi, len(got), len(o.payload))
				}
				fetches++
			}
		}
	}

	// ── Observe: everything the DHT will say about each object. Caring starts once the
	// entry resolves, so the control's announcements may trail its publish: an observer
	// can look later, and so does this one, for the control's objects only. ──────────────
	time.Sleep(3 * time.Second) // let the control fetcher's announces land
	for oi, o := range objects {
		var out string
		deadline := time.Now().Add(90 * time.Second)
		for {
			out = runClient(t, "swarm", "announcers", o.link, "-peers", boot, "-registry", regRef)
			if o.publisher != ctrlPub || strings.Contains(ulLine(out, "care"), publishers[ctrlPub].id) || time.Now().After(deadline) {
				break
			}
			time.Sleep(2 * time.Second)
		}
		o.announced = map[string][]string{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			m := reAnnounceLine.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			var ids []string
			if m[2] != "" {
				ids = strings.Split(m[2], ",")
			}
			o.announced[m[1]] = ids
		}
		if len(o.announced) == 0 {
			t.Fatalf("object %d: swarm announcers printed nothing parseable:\n%s", oi, out)
		}
	}

	// ── Seize every disk. Stop every daemon first so its logs are complete. ──────────
	for _, n := range all {
		n.d.cmd.Process.Kill()
		n.d.cmd.Wait()
	}
	stores := make([]string, len(all))
	for i, n := range all {
		stores[i] = n.store
	}
	seized := seizeAll(t, stores)
	ownerOf := func(path string) *ulNode {
		for _, n := range all {
			if strings.HasPrefix(path, n.store+string(os.PathSeparator)) {
				return n
			}
		}
		t.Fatalf("seized file %s belongs to no node", path)
		return nil
	}
	ulContentNeedles(t, seized, objects)

	t.Run("matching", func(t *testing.T) {
		ulMatchingGame(t, objects, publishers, ctrlPub, seized, ownerOf, atPublish)
	})
	t.Run("sweep", func(t *testing.T) {
		ulSweep(t, objects, fetchers, ctrlFetch, seized, ownerOf, fetches)
	})
}

// ulContentNeedles fills each object's needles from the seized disks: its root, then its
// manifest chunk IDs (from the announcers view, which names them), then every data and
// parity chunk ID, read off the manifest opened with the link — so a holder's shard file
// names and any proof that mentions a shard are caught, not only lines that name the root.
func ulContentNeedles(t *testing.T, seized map[string][]byte, objects []*ulObject) {
	t.Helper()
	st := memstore.New()
	for p, b := range seized {
		if filepath.Base(filepath.Dir(filepath.Dir(p))) == "objects" {
			_ = st.Put(context.Background(), ports.NewChunk(b))
		}
	}
	for oi, o := range objects {
		entry := ports.Entry{Root: o.handle.Root}
		for label := range o.announced {
			if hexID, ok := strings.CutPrefix(label, "manifest "); ok {
				h, err := ports.ParseHash(hexID)
				if err != nil {
					t.Fatalf("object %d: manifest label %q: %v", oi, label, err)
				}
				entry.ManifestChunks = append(entry.ManifestChunks, ports.ChunkID(h))
			}
		}
		if len(entry.ManifestChunks) == 0 {
			t.Fatalf("object %d: the announcers view named no manifest chunk", oi)
		}
		m, err := pipeline.LoadFull(context.Background(), st, entry, o.handle)
		if err != nil {
			t.Fatalf("object %d: opening its manifest from the seized disks: %v", oi, err)
		}
		ids := append([]ports.ChunkID{ports.ChunkID(o.handle.Root)}, entry.ManifestChunks...)
		ids = append(ids, m.Leaves()...)
		for _, id := range ids {
			o.needles = append(o.needles, []byte(hex.EncodeToString(id[:])), append([]byte(nil), id[:]...))
		}
	}
}

// ulRecords splits a seized file into the records an adversary would read as one fact: the
// lines of a text file, or the whole of a binary one.
func ulRecords(path string, b []byte) [][]byte {
	switch filepath.Ext(path) {
	case ".log", ".json", ".txt", ".jsonl":
		return bytes.Split(b, []byte("\n"))
	}
	if filepath.Base(path) == "debug.log" {
		return bytes.Split(b, []byte("\n"))
	}
	return [][]byte{b}
}

func ulHasAny(b []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(b, n) {
			return true
		}
	}
	return false
}

func ulIDNeedles(id string) [][]byte {
	raw, _ := hex.DecodeString(id)
	return [][]byte{[]byte(id), raw}
}

// ulXorRank is how many of the ids are strictly closer to key than id is, by XOR distance.
func ulXorRank(key []byte, id string, ids []string) int {
	d := func(x string) []byte {
		b, _ := hex.DecodeString(x)
		out := make([]byte, len(key))
		for i := range out {
			if i < len(b) {
				out[i] = b[i] ^ key[i]
			}
		}
		return out
	}
	mine := d(id)
	r := 0
	for _, o := range ids {
		if o != id && bytes.Compare(d(o), mine) < 0 {
			r++
		}
	}
	return r
}

// ulMatchingGame scores the adversary's guess of each object's publisher against chance,
// one strategy at a time, so a strategy with real signal cannot hide inside the noise of
// another. Each strategy names, per object, the publishers its evidence points at; the
// adversary guesses uniformly among them, or among all publishers when there are none.
//
//   - announce: from the root alone (the chain), the providers of the object's manifest
//     chunks and care key that placement did not choose. Placement puts a manifest chunk on
//     the nodes nearest its ID, so of the m providers a key has, any whose XOR rank among
//     the nodes that existed at publish time is m or worse announced for another reason.
//   - own-disk: the publishers whose own seized disk holds any of the object's identifiers.
//   - named: the publishers whose identity is written on the same record as one of them.
func ulMatchingGame(t *testing.T, objects []*ulObject, publishers []*ulNode, ctrl int,
	seized map[string][]byte, ownerOf func(string) *ulNode, atPublish []string) {
	k := float64(len(publishers))
	pubIndex := map[string]int{}
	for i, p := range publishers {
		pubIndex[p.id] = i
	}
	paths := make([]string, 0, len(seized))
	for p := range seized {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	strategies := []string{"announce", "own-disk", "named"}
	cands := make([]map[string]map[int]string, len(objects)) // strategy → publisher → one piece of evidence
	for oi, o := range objects {
		c := map[string]map[int]string{}
		for _, s := range strategies {
			c[s] = map[int]string{}
		}
		for label, ids := range o.announced {
			var key []byte
			switch {
			case strings.HasPrefix(label, "manifest "):
				key, _ = hex.DecodeString(strings.TrimPrefix(label, "manifest "))
			case label == "care":
				key = nil // no placement: every care announcer chose to announce
			default:
				continue // column keys need the layout key; the chain does not give them
			}
			for _, id := range ids {
				pi, ok := pubIndex[id]
				if !ok {
					continue
				}
				if key == nil || ulXorRank(key, id, atPublish) >= len(ids) {
					c["announce"][pi] = label
				}
			}
		}
		for _, p := range paths {
			owner := ownerOf(p)
			for _, rec := range ulRecords(p, seized[p]) {
				if !ulHasAny(rec, o.needles) {
					continue
				}
				if pi, ok := pubIndex[owner.id]; ok {
					c["own-disk"][pi] = filepath.Base(p)
				}
				for pi, pub := range publishers {
					if ulHasAny(rec, ulIDNeedles(pub.id)) {
						c["named"][pi] = owner.name + ":" + strings.TrimPrefix(p, owner.store) + ": " + ulClip(rec)
					}
				}
			}
		}
		cands[oi] = c
	}
	pCorrect := func(o *ulObject, cand map[int]string) float64 {
		if len(cand) == 0 {
			return 1 / k
		}
		if _, ok := cand[o.publisher]; ok {
			return 1 / float64(len(cand))
		}
		return 0
	}
	names := func(cand map[int]string) string {
		var out []string
		for pi := range cand {
			out = append(out, publishers[pi].name)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	for _, s := range strategies {
		var score, chance float64
		var n int
		var rows []string
		for oi, o := range objects {
			p := pCorrect(o, cands[oi][s])
			rows = append(rows, fmt.Sprintf("object %2d by %-2s: [%s] %.2f", oi, publishers[o.publisher].name, names(cands[oi][s]), p))
			if ev, ok := cands[oi][s][o.publisher]; ok && s != "own-disk" {
				rows = append(rows, "      evidence for the true publisher: "+ev)
			}
			if o.publisher == ctrl {
				// The control announces itself: the announce strategy must single it out,
				// which is what shows this game can score a leak at all.
				if s == "announce" && p < 0.5 {
					t.Errorf("CONTROL: object %d was published by %s with -care-published=true, which announces "+
						"the publisher, and the announce strategy did not single it out (P(correct)=%.2f) — the game "+
						"cannot score the leak it exists to score", oi, publishers[ctrl].name, p)
				}
				continue
			}
			score += p
			chance += 1 / k
			n++
		}
		// Under no leak the evidence is independent of who published, so each object's
		// P(correct) has mean 1/k; the sum of n of them, each in [0,1], has standard
		// deviation at most sqrt(n·(1/k)(1−1/k)). Three of them is the bar.
		sd := math.Sqrt(float64(n) * (1 / k) * (1 - 1/k))
		t.Logf("strategy %-8s: score %.2f, chance %.2f, bar %.2f over %d objects (control's excluded)\n  %s",
			s, score, chance, chance+3*sd, n, strings.Join(rows, "\n  "))
		if score > chance+3*sd {
			t.Errorf("strategy %s matches publishers to objects at %.2f, better than chance %.2f by more than three standard deviations", s, score, chance)
		}
	}
}

// ulSweep looks for a (fetcher, content) pair in every record outside the fetcher's own
// store and in every announcement.
func ulSweep(t *testing.T, objects []*ulObject, fetchers []*ulNode, ctrl int,
	seized map[string][]byte, ownerOf func(string) *ulNode, fetches int) {
	var pairs []string
	ctrlNamed := 0
	for oi, o := range objects {
		for label, ids := range o.announced {
			for _, id := range ids {
				for fi, f := range fetchers {
					if id != f.id {
						continue
					}
					if fi == ctrl {
						ctrlNamed++
						continue
					}
					pairs = append(pairs, fmt.Sprintf("object %d: %s is announced under %q", oi, f.name, label))
				}
			}
		}
	}
	paths := make([]string, 0, len(seized))
	for p := range seized {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	own := map[string]int{}            // fetcher → records in its own store naming an object
	ownSample := map[string][]string{} // a few of them, to say what they are
	for _, p := range paths {
		owner := ownerOf(p)
		for _, rec := range ulRecords(p, seized[p]) {
			var hit []int
			for oi, o := range objects {
				if ulHasAny(rec, o.needles) {
					hit = append(hit, oi)
				}
			}
			if len(hit) == 0 {
				continue
			}
			for fi, f := range fetchers {
				if owner.id == f.id {
					// A fetcher's own store is its own business, with one exception: on the
					// default it must keep no record of what it read except in the debug
					// firehose, which the operator turns on to diagnose. A validator writes
					// INFO to disk by default, so an INFO line naming a root is a reading
					// history kept without being asked for.
					own[f.name]++
					if len(ownSample[f.name]) < 4 {
						ownSample[f.name] = append(ownSample[f.name], strings.TrimPrefix(p, owner.store)+": "+ulClip(rec))
					}
					if fi != ctrl && !reDebugRecord.Match(rec) {
						pairs = append(pairs, fmt.Sprintf("%s keeps a record of reading object(s) %v outside the debug log level: %s: %q",
							f.name, hit, strings.TrimPrefix(p, owner.store), ulClip(rec)))
					}
					continue
				}
				if fi == ctrl || !ulHasAny(rec, ulIDNeedles(f.id)) {
					continue
				}
				pairs = append(pairs, fmt.Sprintf("%s:%s names %s beside object(s) %v: %q",
					owner.name, strings.TrimPrefix(p, owner.store), f.name, hit, ulClip(rec)))
			}
		}
	}
	t.Logf("sweep after %d fetches: %d seized files; records naming an object in each fetcher's OWN store: %v; "+
		"the control fetcher is announced %d times", fetches, len(seized), own, ctrlNamed)
	for name, recs := range ownSample {
		t.Logf("  %s own store, e.g.:\n    %s", name, strings.Join(recs, "\n    "))
	}
	if ctrlNamed == 0 {
		t.Errorf("CONTROL: %s ran -serve-fetched=true, which announces what it fetched, and no announcement named it — "+
			"the sweep cannot see the pair it exists to find", fetchers[ctrl].name)
	}
	for _, p := range pairs {
		t.Errorf("(fetcher, content) pair: %s", p)
	}
}

// ulLine is the value of the announcers line labelled label, or "".
func ulLine(out, label string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), label+":"); ok {
			return v
		}
	}
	return ""
}

func ulClip(b []byte) string {
	s := strings.ToValidUTF8(string(b), "?")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}
