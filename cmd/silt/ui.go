package main

// The embedded web UI (M13): one Go binary, zero extra runtime. The
// daemon serves static pages (go:embed) plus a JSON API; all node-state
// reads post onto the daemon's event loop, and publish/fetch run
// through an in-process ephemeral swarm client so the daemon's storage
// pledge is never touched by UI operations.
//
// The local surface is LOCKED (Gate 1 / I1, #89): the API used to send
// CORS `*`, so any web page the operator visited could enumerate or drive
// their node. Now every request must arrive with a localhost Host (no DNS
// rebinding), any cross-origin request from a non-localhost page is
// refused outright, and every state-changing call must carry the
// per-daemon bearer token minted on first run. Read-only localhost
// ergonomics are preserved, including the observatory aggregating several
// local daemons from one browser tab (localhost origins are reflected, not
// blanket-allowed). Traces to Don't #3 (access-unsurveilled), B4 (privacy
// by construction), and S4 (no seizable single point).

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nerolabs/silt/core/dht"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/linkbook"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

//go:embed ui
var uiFiles embed.FS

type uiServer struct {
	loop          *eventloop.Loop
	nd            *node.Node
	reg           ports.Registry // may be nil (no registry configured)
	capRep        ports.CapacityReporter
	selfPeer      string // "id@addr" for the ephemeral clients
	validator     bool
	started       time.Time
	peerCount     func() int
	links         *linkbook.Book   // client mode only (nil on a plain daemon)
	carePublished bool             // daemon repairs content published through its own UI (#44)
	token         string           // per-daemon bearer token gating state-changing calls (#89)
	webOrigins    []string         // extra web origins allowed to draw content (e.g. https://app.example.com); off by default. Lets a hosted resolver surface render from this local node.
	addressCap    addressCapConfig // R4.3b: the configured observed-address cap, reported with series A/B/E
	bBootstrap    bool             // R2.9a: publish the B_bootstrap histogram on /api/status. DEFAULT FALSE (-bbootstrap)
}

// addressCapConfig is the -dht-address-cap configuration as /api/status reports it.
type addressCapConfig struct {
	Mode      string `json:"mode"` // off | shadow | on
	Width     int    `json:"width"`
	CapDirect int    `json:"capDirect"`
	CapRelay  int    `json:"capRelay"`
	Reserve   int    `json:"reserve"`
}

// addressCapInfo is the R4.3b shadow-run telemetry (cert §6.3): series A (would-
// refuse per bucket/class under the (R, cap_relay) grid, labelled with the width),
// B (relay fan-in: counts and the top relay's share — never a relay's group) and E
// (the per-bucket group-density census). Aggregates only: no group value leaves
// the process.
type addressCapInfo struct {
	addressCapConfig
	WouldRefuse []wouldRefuseRow   `json:"wouldRefuse"`
	RelayFanIn  relayFanInInfo     `json:"relayFanIn"`
	GroupCensus []dht.BucketCensus `json:"groupCensus"`
}

type wouldRefuseRow struct {
	Bucket   int    `json:"bucket"`
	Class    string `json:"class"` // direct | relayed | unverified
	Reserve  int    `json:"reserve"`
	CapRelay int    `json:"capRelay"`
	Width    int    `json:"width"`
	Count    int    `json:"count"`
}

// relayFanInInfo is series B as seen from THIS node only (the cert's series-B aggregation
// gap: the swarm-wide top-relay share is a harness-side join across nodes). LocalView is
// always true here so a reader cannot mistake it for the swarm figure.
type relayFanInInfo struct {
	LocalView bool    `json:"localView"`
	Relays    int     `json:"relays"`   // distinct relay groups with RELAYED entries
	Clients   int     `json:"clients"`  // RELAYED entries in the table
	TopShare  float64 `json:"topShare"` // the top relay's share of them (0 when none)
	PerRelay  []int   `json:"perRelay"` // clients per relay, descending, unnamed
}

func className(c ports.PeerClass) string {
	switch c {
	case ports.ClassDirect:
		return "direct"
	case ports.ClassRelayed:
		return "relayed"
	}
	return "unverified"
}

func (s *uiServer) addressCapSnapshot() addressCapInfo {
	tab := s.nd.Table()
	info := addressCapInfo{addressCapConfig: s.addressCap, WouldRefuse: []wouldRefuseRow{}, GroupCensus: []dht.BucketCensus{}}
	for k, v := range tab.ShadowRefusals() {
		info.WouldRefuse = append(info.WouldRefuse, wouldRefuseRow{Bucket: k.Bucket, Class: className(k.Class),
			Reserve: k.Reserve, CapRelay: k.CapRelay, Width: s.addressCap.Width, Count: v})
	}
	sort.Slice(info.WouldRefuse, func(i, j int) bool {
		a, b := info.WouldRefuse[i], info.WouldRefuse[j]
		if a.Bucket != b.Bucket {
			return a.Bucket < b.Bucket
		}
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		if a.Reserve != b.Reserve {
			return a.Reserve < b.Reserve
		}
		return a.CapRelay < b.CapRelay
	})
	per, top := tab.RelayFanIn()
	info.RelayFanIn = relayFanInInfo{LocalView: true, Relays: len(per), TopShare: top, PerRelay: []int{}}
	for _, n := range per {
		info.RelayFanIn.Clients += n
		info.RelayFanIn.PerRelay = append(info.RelayFanIn.PerRelay, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(info.RelayFanIn.PerRelay)))
	if rows := tab.GroupCensus(); rows != nil {
		info.GroupCensus = rows
	}
	return info
}

func (s *uiServer) onLoop(fn func()) {
	ch := make(chan struct{})
	s.loop.Post("ui", func() { fn(); close(ch) })
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
	}
}

func (s *uiServer) serve(addr string) (string, error) {
	mux := http.NewServeMux()
	static, _ := fs.Sub(uiFiles, "ui")
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /api/status", s.apiStatus)
	mux.HandleFunc("GET /api/economy/self", s.apiEconomySelf)
	mux.HandleFunc("GET /api/roots", s.apiRoots)
	mux.HandleFunc("GET /api/registry", s.apiRegistry)
	mux.HandleFunc("GET /api/chain", s.apiChain)
	mux.HandleFunc("POST /api/publish", s.apiPublish)
	mux.HandleFunc("POST /api/fund", s.apiFund)
	mux.HandleFunc("GET /api/fetch", s.apiFetch)
	mux.HandleFunc("GET /api/library", s.apiLibrary)
	mux.HandleFunc("POST /api/library/add", s.apiLibraryAdd)
	mux.HandleFunc("POST /api/library/remove", s.apiLibraryRemove)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	go http.Serve(ln, s.guard(mux))
	return ln.Addr().String(), nil
}

// guard locks the local surface (#89). Order matters: reject a non-local
// Host (DNS-rebinding) and a cross-origin drive-by before doing any work,
// answer CORS preflight, then require the bearer token on anything that
// changes state. Static file requests (no /api/ prefix) still pass the
// Host and Origin checks — a rebinding page must not read them either — but
// never need the token.
func (s *uiServer) guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Host allow-list: the browser echoes the host it was pointed at,
		// so a DNS-rebinding page (evil.com → 127.0.0.1) arrives with a
		// non-local Host and is refused here.
		if !isLocalHost(r.Host) {
			httpError(w, http.StatusForbidden, fmt.Errorf("non-local Host %q refused", r.Host))
			return
		}
		// 2. Origin allow-list, replacing CORS `*`. A same-origin GET sends
		// no Origin (skip). A cross-origin page (drive-by or observatory)
		// sends one: reflect localhost origins (so the observatory keeps
		// working) plus any operator-allow-listed web origin (-allow-web-origin,
		// e.g. https://app.example.com so a hosted resolver can render from this node);
		// refuse everything else.
		if origin := r.Header.Get("Origin"); origin != "" {
			local := isLocalOrigin(origin)
			if !local && !s.webOriginAllowed(origin) {
				httpError(w, http.StatusForbidden, fmt.Errorf("cross-origin request from %q refused", origin))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			// Private Network Access: a hosted HTTPS page reaching this localhost
			// node needs this on the preflight (Chrome). Only for the explicitly
			// allow-listed web origins — never for the default local surface.
			if !local {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
		}
		// 3. CORS preflight: answer after the checks, before the token gate.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// 4. Token gate on state-changing methods only — reads keep their
		// no-token localhost ergonomics.
		if isMutating(r.Method) && !s.validToken(r) {
			httpError(w, http.StatusUnauthorized, fmt.Errorf("missing or invalid API token"))
			return
		}
		h.ServeHTTP(w, r)
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// validToken accepts the bearer token from the Authorization header or,
// for form-driven POSTs and download links that can't set headers, a
// `token` field/query param. Compared in constant time.
func (s *uiServer) validToken(r *http.Request) bool {
	if s.token == "" {
		return false // never allow mutation on a daemon with no token
	}
	got := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		got = strings.TrimPrefix(h, "Bearer ")
	} else if t := r.URL.Query().Get("token"); t != "" {
		got = t
	} else {
		got = r.FormValue("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

// isLocalHost reports whether the request's Host authority names the local
// machine (any port). Anything else — a rebinding page's real hostname —
// is refused.
func isLocalHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host // no port
	}
	switch h {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	// Any loopback literal (127.0.0.0/8, ::1) counts.
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// isLocalOrigin reports whether an Origin header names a localhost page —
// the daemon's own UI or a sibling local daemon's observatory.
func isLocalOrigin(origin string) bool {
	rest, ok := strings.CutPrefix(origin, "http://")
	if !ok {
		rest, ok = strings.CutPrefix(origin, "https://")
	}
	if !ok {
		return false
	}
	return isLocalHost(rest)
}

// webOriginAllowed reports whether origin is in the operator's explicit
// -allow-web-origin list — opt-in, exact-match, never a wildcard. This is
// what lets a hosted resolver surface (e.g. https://app.example.com) draw content
// from this local node, while keeping the default surface localhost-only.
func (s *uiServer) webOriginAllowed(origin string) bool {
	for _, o := range s.webOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

// parseWebOrigins splits a comma-separated -allow-web-origin value, trimming
// blanks. Empty input yields no allowed web origins (the secure default).
func parseWebOrigins(csv string) []string {
	var out []string
	for _, o := range strings.Split(csv, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// loadOrCreateUIToken returns the daemon's persistent UI bearer token,
// minting one on first run. Persisted 0600 in the store dir so it survives
// restarts and the same operator's browser keeps working.
func loadOrCreateUIToken(storeDir string) (string, error) {
	path := filepath.Join(storeDir, "ui-token")
	if b, err := os.ReadFile(path); err == nil {
		if tok := strings.TrimSpace(string(b)); tok != "" {
			return tok, nil
		}
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw[:])
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func (s *uiServer) apiStatus(w http.ResponseWriter, _ *http.Request) {
	type chainInfo struct {
		Height  int `json:"height"`
		Entries int `json:"entries"`
	}
	var out struct {
		ID           string           `json:"id"`
		Peer         string           `json:"peer"`
		UptimeSec    int64            `json:"uptimeSec"`
		CapUsed      int64            `json:"capUsed"`
		CapTotal     int64            `json:"capTotal"`
		Chunks       int              `json:"chunks"`
		Peers        int              `json:"peers"`
		Stats        node.Stats       `json:"stats"`
		Network      node.NetEstimate `json:"network"`
		Validator    bool             `json:"validator"`
		Reachability string           `json:"reachability"`
		Chain        *chainInfo       `json:"chain,omitempty"`
		Durability   *durabilityInfo  `json:"durability,omitempty"`
		AddressCap   addressCapInfo   `json:"addressCap"` // R4.3b series A/B/E (shadow-run telemetry)
		// R2.9a: ABSENT unless -bbootstrap is set. Absent and empty are different
		// objects: a reader that sees no key knows the instrument is off, and a reader
		// that sees the key with zero requesters knows the instrument is on and the
		// ledger is idle.
		BBootstrap *bBootstrapInfo `json:"bBootstrap,omitempty"`
	}
	out.ID = s.nd.ID().String()
	out.Peer = s.selfPeer
	uptime := time.Since(s.started)
	out.UptimeSec = int64(uptime.Seconds())
	out.Validator = s.validator
	out.Peers = s.peerCount()
	s.onLoop(func() {
		out.Reachability = s.nd.Reachability().String()
		if s.capRep != nil {
			out.CapUsed, out.CapTotal = s.capRep.Capacity()
		}
		if ids, err := s.nd.Store().List(context.Background()); err == nil {
			out.Chunks = len(ids)
		}
		out.Stats = s.nd.Stats
		out.Network = s.nd.EstimateNetwork()
		if ch := s.nd.Chain(); ch != nil {
			out.Chain = &chainInfo{Height: ch.Len(), Entries: len(ch.AllEntries())}
		}
		out.Durability = s.durabilitySnapshot(uptime)
		out.AddressCap = s.addressCapSnapshot()
		out.BBootstrap = s.bBootstrapSnapshot()
	})
	writeJSON(w, out)
}

// bBootstrapInfo is the published B_bootstrap histogram (R2.9a): a full-census 2-D
// COUNT histogram over (identity age × log2 fetched bytes), the instrument
// D-R2.9-DIRECTION sentence 4 requires before the affordability ratio grant/r can be
// pinned. cloudtest measures its own synthetic fetch plan, so the numbers have to come
// off a deployment with real users.
//
// WHAT IT DELIBERATELY IS NOT (immutable #4, refuse-to-surveil). Counts, and nothing
// else. No requester id — not even a salted label — no object root, no per-identity row,
// no exact age, and no per-cell byte SUM (a cell sum with count 1 is that identity's
// exact byte total in disguise). An analyst can read Q_q(bytes | age bucket) from it and
// can learn nothing about who fetched what.
//
// DEFAULT OFF (-bbootstrap). GET /api/status needs no token, so anything published here
// is world-readable wherever -ui is bound off loopback; reversing a default is cheap now
// and expensive after adoption, and the measurement needs exactly one deployment.
type bBootstrapInfo struct {
	ClockSource string `json:"clockSource"` // "injected" | "none" — the age axis self-report (H-1)
	AgeAxisLive bool   `json:"ageAxisLive"` // false ⇒ cells is null; NEVER an all-zero age column

	Requesters int `json:"requesters"` // the TRUE total: every account with fetched bytes > 0
	Aged       int `json:"aged"`       // how many landed in a cell; equals the sum of all cells
	Unstamped  int `json:"unstamped"`  // counted, never dumped into age bucket 0

	UptimeNanos             int64 `json:"uptimeNanos"`             // elapsed on the WALL clock; moves with an NTP step, so not a bound on its own
	MaxOccupiedAgeEdgeNanos int64 `json:"maxOccupiedAgeEdgeNanos"` // lower edge of the highest occupied bucket
	ClockStepBack           bool  `json:"clockStepBack"`           // a subtraction crossed zero; ages clamped at 0. NOT the step detector — see clockSuspect
	AgeExceedsUptime        bool  `json:"ageExceedsUptime"`        // the G-BB-4 censoring assertion failed — the run is suspect

	// The clock cross-check (G-BB-4 / BB-13). uptimeNanos and every age come off ONE
	// wall clock, so a step moves both and cancels; monotonicUptimeNanos comes off a
	// source nothing can step, and the difference between them IS the step. It is
	// published as a signed number as well as a flag, because the two directions are
	// different failures and an analyst judges the magnitude against their own W.
	MonotonicSource      string `json:"monotonicSource"`      // "injected" | "none" — the cross-check's self-report
	MonotonicUptimeNanos int64  `json:"monotonicUptimeNanos"` // the REAL censoring bound: no age can exceed it
	ClockSkewNanos       int64  `json:"clockSkewNanos"`       // wall − monotone; positive = the wall clock jumped forward
	ClockSuspect         bool   `json:"clockSuspect"`         // the divergence moved identities at least a whole age bucket

	AgeEdgeNanos  []int64 `json:"ageEdgeNanos"`  // lower edges; bucket i = [i, i+1), last open
	AgeBuckets    int     `json:"ageBuckets"`    //
	BinsPerOctave int     `json:"binsPerOctave"` // 4 — quarter-log2 byte bins
	ByteBins      int     `json:"byteBins"`      // 164
	ByteBinRule   string  `json:"byteBinRule"`   // the byte axis stated exactly, as a closed form

	Cells [][]int64 `json:"cells"` // [ageBucket][byteBin] counts; null when the age axis is not live
}

// bBootstrapSnapshot renders the histogram for the wire, or nil when -bbootstrap is
// unset (the block is then ABSENT from /api/status, not present-and-empty) or when no
// ledger implements the export.
func (s *uiServer) bBootstrapSnapshot() *bBootstrapInfo {
	if !s.bBootstrap {
		return nil
	}
	h, ok := s.nd.BBootstrap()
	if !ok {
		return nil
	}
	out := &bBootstrapInfo{
		ClockSource:             h.ClockSource,
		AgeAxisLive:             h.AgeAxisLive,
		Requesters:              h.Requesters,
		Aged:                    h.Aged,
		Unstamped:               h.Unstamped,
		UptimeNanos:             h.UptimeNanos,
		MaxOccupiedAgeEdgeNanos: h.MaxOccupiedAgeEdgeNanos,
		ClockStepBack:           h.ClockStepBack,
		AgeExceedsUptime:        h.AgeExceedsUptime,
		MonotonicSource:         h.MonotonicSource,
		MonotonicUptimeNanos:    h.MonotonicUptimeNanos,
		ClockSkewNanos:          h.ClockSkewNanos,
		ClockSuspect:            h.ClockSuspect,
		AgeEdgeNanos:            h.AgeEdgeNanos[:],
		AgeBuckets:              credit.BBootstrapAgeBuckets,
		BinsPerOctave:           h.BinsPerOctave,
		ByteBins:                h.ByteBins,
		ByteBinRule:             h.ByteBinRule,
	}
	if h.Cells != nil {
		out.Cells = make([][]int64, credit.BBootstrapAgeBuckets)
		for i := range h.Cells {
			out.Cells[i] = h.Cells[i][:]
		}
	}
	return out
}

// durabilityInfo makes the built-but-previously-invisible S7 repair economy
// observable (Phase 2): the node's credit balance (what serving earned) and, per
// object it caretakes, the funded reserve, lifetime skim/pay, and the projected
// funded horizon. `bountyOn` reports whether repair bounties actually PAY on this
// node (the -economy switch) — an economy whose escrows fill but never disburse
// reads very differently from one that is live. Standing is never in this block
// (Invariant A: credits fund durability, never consensus weight).
type durabilityInfo struct {
	BountyOn bool            `json:"bountyOn"`
	Balance  int64           `json:"balance"`
	Objects  []objDurability `json:"objects"`
}

type objDurability struct {
	Root       string `json:"root"`
	Reserve    int64  `json:"reserve"`
	Funded     int64  `json:"funded"`
	Paid       int64  `json:"paid"`
	Repairs    int64  `json:"repairs"`
	HorizonSec int64  `json:"horizonSec"` // -1 = not yet measurable (no burn observed); >=0 = projected
	// Finite is credit.Horizon's own second return: true only when a real burn has
	// been observed (Paid>0) so the horizon is measurable. false renders as "horizon
	// not yet measurable", NEVER as "perpetual" (instruments.go:47-49) — never fake
	// precision. Cliff is the solvency early-warning (Boulder 2 R2.1 Panel 1): the
	// reserve has a measurable, finite horizon within the warning window, i.e. the
	// object's funded durability runs out soon at the observed burn rate.
	Finite bool `json:"finite"`
	Cliff  bool `json:"cliff"`
}

// durabilitySnapshot builds the durability block from the node's cared objects.
// Loop-only (CaredDurability reads n.care); called inside apiStatus's onLoop.
// The funded horizon is projected over `uptime` — the node's observation window
// for the burn rate — matching credit.Horizon's semantics.
func (s *uiServer) durabilitySnapshot(uptime time.Duration) *durabilityInfo {
	cared := s.nd.CaredDurability()
	di := &durabilityInfo{
		BountyOn: s.nd.RepairBountyEnabled(),
		Balance:  s.nd.CreditBalance(),
		Objects:  make([]objDurability, 0, len(cared)),
	}
	for _, rd := range cared {
		hs := int64(-1)
		finite := false
		cliff := false
		if h, ok := credit.Horizon(rd.Snapshot, ports.Duration(uptime)); ok {
			finite = true
			hs = int64(h / ports.Duration(time.Second))
			// Cliff: a measurable horizon at or below the warning window is the
			// funding-cliff early-warning (Panel 1). A depleted reserve (horizon 0)
			// is the sharpest cliff. Not-yet-measurable (finite==false) is never a
			// cliff — an unmeasured burn is not a proven-safe one, so it is surfaced
			// as "not yet measurable", not as green.
			cliff = h <= horizonWarningWindow
		}
		di.Objects = append(di.Objects, objDurability{
			Root:       rd.Root.String(),
			Reserve:    rd.Snapshot.Balance,
			Funded:     rd.Snapshot.Funded,
			Paid:       rd.Snapshot.Paid,
			Repairs:    rd.Snapshot.Repairs,
			HorizonSec: hs,
			Finite:     finite,
			Cliff:      cliff,
		})
	}
	return di
}

// horizonWarningWindow is how close to the funded horizon running out an object's
// reserve must be before Panel 1 flags a cliff. It is a presentation threshold
// (evolving-tier), not a mechanism: it changes no validity, disbursement, or
// standing rule — it only decides when the dashboard turns a bar red. 30 days is a
// conservative re-endowment lead time for an operator watching cold data. Local to
// the read path; no network number is invented here.
const horizonWarningWindow = ports.Duration(30 * 24 * time.Hour)

// apiEconomySelf serves the economy-observability SELF panels (Boulder 2, R2.1
// slice 6a): the four LOCAL-EXACT panels an operator reads to see their own
// economy's health from ONE node, with no aggregator and no network estimation.
// Every field is read from this node's own Ledger/care state, so every number is
// local-exact. It ships cert-free and economy-OFF: with the repair economy off the
// escrows still fill (the auto-skim), so the accounting is real; only the bounty
// disbursement is dormant, which `bountyOn` reports. Read-only GET; reading moves
// nothing (the #89 gate lets read-only localhost through without a token).
//
// Knowability tier is stamped per block: everything here is "local-exact" except
// the wash AUTHENTICITY, which is "not-knowable" (Douceur — a node cannot prove
// another identity is a Sybil), so the wash block is a SHAPE self-check labeled
// "suspected", never "detected", and is never a slashing input.
func (s *uiServer) apiEconomySelf(w http.ResponseWriter, r *http.Request) {
	// Panel 2 margin: revenue is local-exact; operator cost is off-ledger and
	// private, so it is an OPTIONAL query parameter (?cost=N credits/observation
	// window), never a persisted flag. Absent, the margin is "cost not supplied"
	// and only revenue renders — the panel never asserts a margin it cannot ground.
	costGiven := false
	var cost int64
	if c := r.URL.Query().Get("cost"); c != "" {
		if v, err := strconv.ParseInt(c, 10, 64); err == nil {
			cost = v
			costGiven = true
		}
	}

	var self node.EconomySelf
	var di *durabilityInfo
	var uptime time.Duration
	s.onLoop(func() {
		uptime = time.Since(s.started)
		self = s.nd.EconomySelf()
		di = s.durabilitySnapshot(uptime)
	})

	// Panel 3 (is durability self-funding): skim-in vs bounty-out. funded is the
	// lifetime skim-in (prepay + auto-skim); paid is the lifetime bounty-out. A
	// persistent paid>funded is the drain signal. Pooled + per-object.
	var poolFunded, poolPaid int64
	objects := make([]economyObject, 0, len(di.Objects))
	for _, o := range di.Objects {
		poolFunded += o.Funded
		poolPaid += o.Paid
		objects = append(objects, economyObject{
			Root:       o.Root,
			Reserve:    o.Reserve,
			SkimIn:     o.Funded,
			BountyOut:  o.Paid,
			Net:        o.Funded - o.Paid,
			Repairs:    o.Repairs,
			HorizonSec: o.HorizonSec,
			Finite:     o.Finite,
			Cliff:      o.Cliff,
		})
	}

	// Panel 4 (wash self-check): the SHAPE only. Serve/fetch byte symmetry near 1.0
	// plus net-negative churn is the wash signature — but authenticity is
	// not-knowable, so this is a self-check that lets an HONEST operator SEE their
	// own shape and show they are not the cluster. Never authenticity, never a
	// slash input. Symmetry = min/max so it is in [0,1] and undefined (0) when the
	// node has neither served nor fetched.
	symmetry := 0.0
	if self.ServedBytes > 0 || self.FetchedBytes > 0 {
		lo, hi := self.ServedBytes, self.FetchedBytes
		if lo > hi {
			lo, hi = hi, lo
		}
		if hi > 0 {
			symmetry = float64(lo) / float64(hi)
		}
	}
	// "suspected" only: symmetric byte flow (near 1:1 serve:fetch) AND a
	// non-positive balance (churn that nets to nothing or debt) is the shape a wash
	// pair leaves. An honest hot server has high symmetry but a POSITIVE balance;
	// an honest leech has low symmetry. The conjunction is what narrows it.
	washSuspected := symmetry >= washSymmetryThreshold && self.Balance <= 0

	out := economySelf{
		Tier: "local-exact",
		Revenue: economyRevenue{
			Balance:      self.Balance,
			ServedBytes:  self.ServedBytes,
			FetchedBytes: self.FetchedBytes,
			RepairsDone:  self.RepairsDone,
			BountyEarned: self.BountyEarned,
			// Serve revenue is what the balance holds net of repair revenue; it is a
			// derived split for the panel, not a second accumulator. It can be
			// negative if the node has spent (funded escrows, publish fees) more than
			// it earned serving — that is real and honestly shown.
			ServeRevenue: self.Balance - self.BountyEarned,
		},
		Margin: economyMargin{
			CostGiven: costGiven,
			Cost:      cost,
			Margin:    self.Balance - cost,
			Note:      "revenue is local-exact; cost is operator-supplied (?cost=N), so margin is exact GIVEN your cost number",
		},
		SelfFunding: economySelfFunding{
			SkimIn:    poolFunded,
			BountyOut: poolPaid,
			Net:       poolFunded - poolPaid,
			BountyOn:  di.BountyOn,
		},
		Wash: economyWash{
			Symmetry:             symmetry,
			BalanceNonPositive:   self.Balance <= 0,
			Suspected:            washSuspected,
			AuthenticityKnowable: false,
			Note:                 "SHAPE self-check only. Authenticity is not-knowable (Douceur); this is 'suspected', never 'detected', and never a slashing input",
		},
		Objects: objects,
	}
	writeJSON(w, out)
}

// washSymmetryThreshold is how close serve:fetch byte flow must be to 1:1 before
// the wash self-check calls the shape "suspected". Presentation threshold
// (evolving-tier), not a mechanism: it gates a dashboard label, never a slash. A
// wash pair ping-pongs bytes, so its serve and fetch totals track each other;
// 0.9 catches near-symmetric flow while leaving an honest hot server (serves far
// more than it fetches) below it.
const washSymmetryThreshold = 0.9

// economySelf is the SELF-panel response (GET /api/economy/self). One flat object
// carrying the four local-exact panels: Revenue+Margin (Panel 2 am-I-profitable),
// SelfFunding (Panel 3 is-durability-self-funding), Wash (Panel 4 wash self-check),
// and Objects (Panel 1 my-solvency, per cared object with its cliff flag).
type economySelf struct {
	Tier        string             `json:"tier"` // "local-exact" — every field here is read from this node's own ledger
	Revenue     economyRevenue     `json:"revenue"`
	Margin      economyMargin      `json:"margin"`
	SelfFunding economySelfFunding `json:"selfFunding"`
	Wash        economyWash        `json:"wash"`
	Objects     []economyObject    `json:"objects"`
}

type economyRevenue struct {
	Balance      int64 `json:"balance"`
	ServedBytes  int64 `json:"servedBytes"`
	FetchedBytes int64 `json:"fetchedBytes"`
	RepairsDone  int64 `json:"repairsDone"`
	BountyEarned int64 `json:"bountyEarned"`
	ServeRevenue int64 `json:"serveRevenue"` // balance − bountyEarned (derived split)
}

type economyMargin struct {
	CostGiven bool   `json:"costGiven"` // false: cost not supplied, margin is balance-only
	Cost      int64  `json:"cost"`
	Margin    int64  `json:"margin"` // balance − cost (exact given the operator's cost)
	Note      string `json:"note"`
}

type economySelfFunding struct {
	SkimIn    int64 `json:"skimIn"`    // pooled lifetime funded (prepay + auto-skim)
	BountyOut int64 `json:"bountyOut"` // pooled lifetime paid (repair bounties)
	Net       int64 `json:"net"`       // skimIn − bountyOut; persistently negative = drain
	BountyOn  bool  `json:"bountyOn"`  // does repair actually disburse on this node (-economy)
}

type economyWash struct {
	Symmetry             float64 `json:"symmetry"`             // min(serve,fetch)/max(serve,fetch), in [0,1]
	BalanceNonPositive   bool    `json:"balanceNonPositive"`   // net-negative/zero churn
	Suspected            bool    `json:"suspected"`            // shape matches a wash pair (NOT a detection)
	AuthenticityKnowable bool    `json:"authenticityKnowable"` // always false: Douceur
	Note                 string  `json:"note"`
}

type economyObject struct {
	Root       string `json:"root"`
	Reserve    int64  `json:"reserve"`
	SkimIn     int64  `json:"skimIn"`
	BountyOut  int64  `json:"bountyOut"`
	Net        int64  `json:"net"`
	Repairs    int64  `json:"repairs"`
	HorizonSec int64  `json:"horizonSec"`
	Finite     bool   `json:"finite"`
	Cliff      bool   `json:"cliff"`
}

const (
	defaultRootsPage = 50
	maxRootsPage     = 500
)

type rootRow struct {
	Root   string `json:"root"`
	Shards int    `json:"shards"`
}

func (s *uiServer) apiRoots(w http.ResponseWriter, r *http.Request) {
	var rows []rootRow
	s.onLoop(func() {
		for root, count := range s.nd.HeldRoots() {
			rows = append(rows, rootRow{Root: root.String(), Shards: count})
		}
	})
	page, total := paginateRoots(rows,
		atoiOr(r.URL.Query().Get("limit"), defaultRootsPage),
		atoiOr(r.URL.Query().Get("offset"), 0))
	writeJSON(w, map[string]any{"total": total, "rows": page})
}

// paginateRoots sorts rows by shard count (desc — most-hosted first), then
// root for a stable order, and returns the [offset, offset+limit) window
// plus the total. A limit <= 0 uses the default page and is capped at
// maxRootsPage; offset is clamped, so out-of-range paging yields an empty
// page rather than a panic.
func paginateRoots(rows []rootRow, limit, offset int) (page []rootRow, total int) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Shards != rows[j].Shards {
			return rows[i].Shards > rows[j].Shards
		}
		return rows[i].Root < rows[j].Root
	})
	total = len(rows)
	switch {
	case limit <= 0:
		limit = defaultRootsPage
	case limit > maxRootsPage:
		limit = maxRootsPage
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return rows[offset:end], total
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func (s *uiServer) apiRegistry(w http.ResponseWriter, r *http.Request) {
	if s.reg == nil {
		writeJSON(w, []any{})
		return
	}
	entries, err := s.reg.All(r.Context())
	if err != nil {
		// A remote public registry no longer serves a bulk /all dump (F-3), so a UI
		// pointed at one degrades to an empty list rather than erroring. An operator's
		// OWN registry (-serve-registry) is read in-process and lists fully.
		writeJSON(w, []any{})
		return
	}
	type row struct {
		Root           string `json:"root"`
		FileSize       int64  `json:"fileSize"`
		ManifestChunks int    `json:"manifestChunks"`
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{Root: e.Root.String(), FileSize: e.FileSize, ManifestChunks: len(e.ManifestChunks)})
	}
	writeJSON(w, rows)
}

func (s *uiServer) apiChain(w http.ResponseWriter, _ *http.Request) {
	type row struct {
		Height   uint64 `json:"height"`
		Hash     string `json:"hash"`
		Entries  int    `json:"entries"`
		Proposer string `json:"proposer"`
		Atts     int    `json:"atts"`
	}
	rows := []row{}
	s.onLoop(func() {
		ch := s.nd.Chain()
		if ch == nil {
			return
		}
		for _, b := range ch.Blocks(0) {
			rows = append(rows, row{
				Height: b.Height, Hash: b.Hash().String(),
				Entries: len(b.Entries), Proposer: b.ProposerID().String(), Atts: len(b.Atts),
			})
		}
	})
	writeJSON(w, rows)
}

// apiPublish: multipart upload → chunk, encrypt, scatter, register —
// through an ephemeral client, exactly like `silt swarm add`.
func (s *uiServer) apiPublish(w http.ResponseWriter, r *http.Request) {
	if s.reg == nil {
		httpError(w, 400, fmt.Errorf("this daemon has no registry configured"))
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		httpError(w, 400, err)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpError(w, 400, err)
		return
	}
	defer f.Close()
	mode := crypto.Private // H6: private by default — convergent must be opted into explicitly
	if r.FormValue("mode") == "convergent" {
		mode = crypto.Convergent
	}

	e, run, err := joinSwarm(s.selfPeer, 0)
	if err != nil {
		httpError(w, 502, err)
		return
	}
	defer e.close()

	var h link.Handle
	var placed int
	var opErr error
	rerr := run(func(done func()) {
		var aerr error
		var entry ports.Entry
		// No Publisher: a UI publish is the default user path and must be
		// unlinkable (M0/#97). A durable Publisher→root record is permanent
		// on the chain; the UI never opts into it.
		//
		// Stage (not Add): store the content, register only after a confirmed
		// scatter, so a placement failure never leaves a dangling entry (#65).
		h, entry, aerr = pipeline.Stage(context.Background(), e.nd.Store(), f, pipeline.Options{
			Mode: mode, Rand: rand.Reader,
		})
		if aerr != nil {
			opErr = aerr
			done()
			return
		}
		m, merr := pipeline.LoadFull(context.Background(), e.nd.Store(), entry, h)
		if merr != nil {
			opErr = merr
			done()
			return
		}
		e.nd.DistributeFrom(e.nd.Store(), entry, m, node.DerivePorKey(h.LayoutKey()), func(p int, derr error) {
			// Publish only on a confirmed scatter; a failed one leaves the
			// registry untouched so no dangling entry survives (#65).
			placed, opErr = pipeline.RegisterAfterDistribute(context.Background(), s.reg, entry, p, derr)
			done()
		})
	})
	if rerr != nil {
		httpError(w, 504, rerr)
		return
	}
	if opErr != nil {
		httpError(w, 502, opErr)
		return
	}
	// Auto-caretake our own content: without a caretaker, a published
	// file's redundancy only decays as nodes churn. The publishing daemon
	// is the natural first caretaker, so it starts repairing this root (its
	// manifest now counts toward this node's pledge). Opt out with
	// -care-published=false.
	cared := false
	if s.carePublished && s.reg != nil {
		ch := h.Care()
		s.onLoop(func() { s.nd.Care(s.reg, ch) })
		cared = true
	}
	writeJSON(w, map[string]any{
		"name":      hdr.Filename,
		"link":      h.String(),
		"careLink":  h.Care().String(),
		"placed":    placed,
		"caretaker": cared,
	})
}

// apiFund prepays an object's durability reserve from THIS node's own credit
// balance (Phase 2, Slice 3 — the publisher/operator endowment path over
// FundDurability). A publisher endows a repair budget so their content outlives
// churn before it is popular enough to self-fund via the serve auto-skim; the
// credits come from what this daemon EARNED by serving, and standing is untouched
// (Invariant A). Token-gated (mutating). Body: root (a silt:/siltcare: link or a
// bare root hash) + amount (credits, > 0).
func (s *uiServer) apiFund(w http.ResponseWriter, r *http.Request) {
	root, err := parseRootArg(r.FormValue("root"))
	if err != nil {
		httpError(w, 400, err)
		return
	}
	amount, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("amount")), 10, 64)
	if err != nil || amount <= 0 {
		httpError(w, 400, fmt.Errorf("amount must be a positive integer (credits), got %q", r.FormValue("amount")))
		return
	}
	var fundErr error
	var reserve, balance int64
	s.onLoop(func() {
		fundErr = s.nd.FundDurability(root, amount)
		if fundErr == nil {
			reserve = s.nd.DurabilityReserve(root)
			balance = s.nd.CreditBalance()
		}
	})
	if fundErr != nil {
		// Insufficient balance is a client-correctable condition, not a server
		// fault — 402 (the daemon has nothing wrong; the caller lacks credit).
		if errors.Is(fundErr, ports.ErrInsufficientCredit) {
			httpError(w, 402, fundErr)
			return
		}
		httpError(w, 400, fundErr)
		return
	}
	writeJSON(w, map[string]any{
		"root":    root.String(),
		"funded":  amount,
		"reserve": reserve, // the object's escrow after this endowment
		"balance": balance, // this node's remaining credit balance
	})
}

// parseRootArg accepts either a full silt:/siltcare: link (returning its root) or
// a bare 32-byte root hash, so a publisher can fund straight from the link they hold.
func parseRootArg(s string) (ports.Hash, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ports.Hash{}, fmt.Errorf("root required (a silt: link or a root hash)")
	}
	if h, err := link.Parse(s); err == nil {
		return h.Root, nil
	}
	if c, err := link.ParseAnyCare(s); err == nil {
		return c.Root, nil
	}
	if h, err := ports.ParseHash(s); err == nil {
		return h, nil
	}
	return ports.Hash{}, fmt.Errorf("not a silt: link or a 32-byte root hash: %q", s)
}

// apiFetch: ?link=silt:v1:… → the decrypted file, verified end to end.
func (s *uiServer) apiFetch(w http.ResponseWriter, r *http.Request) {
	if s.reg == nil {
		httpError(w, 400, fmt.Errorf("this daemon has no registry configured"))
		return
	}
	h, err := link.Parse(r.URL.Query().Get("link"))
	if err != nil {
		httpError(w, 400, err)
		return
	}

	// Local-store fast path: if this node already holds the content,
	// reconstruct it straight from the local store — no ephemeral swarm node,
	// no network round-trip. A node should never cross the swarm to read bytes
	// it already has, and it *can't* if provider resolution is degraded
	// (dead-peer pollution, lookup timeouts) — which is exactly when a node that
	// holds the data must still be able to serve it. This lets a daemon serve
	// its own published content, and lets a client re-serve films it has already
	// pulled (the consumer==provider path). pipeline.Get reads and verifies
	// against the root; on a complete store it never writes, and diskstore.Get is
	// a plain os.ReadFile, so this is safe to run off the event loop. If any
	// chunk is missing locally, pipeline.Get errors and we fall through to the
	// swarm fetch below unchanged.
	{
		var local bytes.Buffer
		if err := pipeline.Get(context.Background(), s.nd.Store(), s.reg, h, &local); err == nil {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition",
				fmt.Sprintf("attachment; filename=%q", h.Root.String()[:16]+".bin"))
			io.Copy(w, &local)
			return
		}
	}

	// Swarm fetch on the MAIN node — not a throwaway ephemeral node. This is
	// the consumer==provider path: NetGetRetain pulls the missing shards into
	// THIS node's own store (bounded by its -capacity pledge), so content you
	// draw is content you now HOLD and can serve back. The old code fetched
	// through a per-request ephemeral node (memstore, SetEphemeral) that kept
	// nothing — making the client a pure leech (chunks-held stays 0, nothing to
	// share back) and leaking a 127.0.0.1 address-book entry on every fetch.
	// Fetching on s.nd means the next read of the same link hits the local-store
	// fast path above — and RETAIN wires the rest of the promise (#500): the
	// pulled shards get real storage proofs minted from the link's layout key,
	// register under their placement keys, and ANNOUNCE, so the node is a
	// discoverable, audit-answerable provider of what it consumed rather than a
	// silent hoarder (plain NetGet now drops its working set after assembly).
	var buf bytes.Buffer
	var opErr error
	fetchDone := make(chan struct{})
	s.loop.Post("api-fetch", func() {
		s.nd.NetGetRetain(s.reg, h, &buf, func(err error) { opErr = err; close(fetchDone) })
	})
	select {
	case <-fetchDone:
	case <-time.After(5 * time.Minute):
		httpError(w, 504, fmt.Errorf("swarm fetch timed out"))
		return
	}
	if opErr != nil {
		httpError(w, 502, opErr)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", h.Root.String()[:16]+".bin"))
	io.Copy(w, &buf)
}

// apiLibrary lists the user's saved links (files they hold keys for),
// each annotated with what the network/chain currently knows about that
// root — so the library distinguishes "yours and available" from "yours
// but the network seems to have lost it".
func (s *uiServer) apiLibrary(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Root     string `json:"root"`
		Link     string `json:"link"`
		Label    string `json:"label"`
		Added    int64  `json:"added"`
		OnChain  bool   `json:"onChain"`
		FileSize int64  `json:"fileSize"`
	}
	var rows []row
	items := []linkbook.Item{}
	if s.links != nil {
		items = s.links.Items()
	}
	// Build a root→entry index from the registry once.
	index := map[string]ports.Entry{}
	networkTotal := 0
	if s.reg != nil {
		if entries, err := s.reg.All(r.Context()); err == nil {
			networkTotal = len(entries)
			for _, e := range entries {
				index[e.Root.String()] = e
			}
		}
	}
	held := map[string]bool{}
	for _, it := range items {
		h, err := link.Parse(it.Link)
		if err != nil {
			continue
		}
		rootHex := h.Root.String()
		held[rootHex] = true
		e, on := index[rootHex]
		rows = append(rows, row{
			Root: rootHex, Link: it.Link, Label: it.Label, Added: it.Added,
			OnChain: on, FileSize: e.FileSize,
		})
	}
	// How many identifiers the network hosts that the user has NO key for.
	opaque := 0
	for root := range index {
		if !held[root] {
			opaque++
		}
	}
	writeJSON(w, map[string]any{
		"library":      rows,
		"networkFiles": networkTotal,
		"opaqueToYou":  opaque,
	})
}

func (s *uiServer) apiLibraryAdd(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		httpError(w, 400, fmt.Errorf("no link book (client mode only)"))
		return
	}
	root, err := s.links.Add(r.FormValue("link"), r.FormValue("label"))
	if err != nil {
		httpError(w, 400, err)
		return
	}
	writeJSON(w, map[string]string{"root": root})
}

func (s *uiServer) apiLibraryRemove(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		httpError(w, 400, fmt.Errorf("no link book"))
		return
	}
	if err := s.links.Remove(r.FormValue("root")); err != nil {
		httpError(w, 400, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
