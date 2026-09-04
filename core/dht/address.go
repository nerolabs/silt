package dht

// R4.3b — the H5-B eclipse cap keyed on the OBSERVED contacted-at address (the
// geth / Bitcoin Core form). The table never sees an IP: a ports.PeerClassifier
// hands it an opaque, per-process-salted (class, group) for a peer it has actually
// conversed with. The class lives WITH the table entry (cert C-4), captured at
// admission and deleted on Remove — never a soft cache that a distinct-sender
// flood can evict.
//
// The rule (research-certified 2026-09-04, silt-reviews/research/research-outcome/
// R4.3b-relayed-class-and-observed-address-keying-RESEARCH-CERTIFICATION-2026-09-04.md):
//
//	per-group entries per bucket  ≤ capDirect (DIRECT) | capRelay (RELAYED)
//	                                 counted over ONE group namespace across classes (C-1)
//	non-DIRECT entries per bucket ≤ K − reserve                             (C-2)
//	one group across the table    ≤ tableGroupCap (geth's 10-per-/24)
//	DIRECT is never downgraded by a later relayed conversation              (C-3)
//	adversary floor per bucket    = ⌈reserve / capDirect⌉ paid /24s + (K − reserve) free slots
//
// UNVERIFIED (reply-learned, never contacted) entries are charged to the
// INTRODUCER's group and re-checked at their first classification — a narrowing
// only (the admission rule applied late); class LOSS is inert. Seeds and
// -persistent-peers (ObserveStatic) are exempt from every cap: operator-typed,
// count-bounded, unforgeable.
//
// Modes: off (no rule), shadow (the rule is evaluated and every would-be refusal
// is COUNTED per (bucket, class, R, capRelay) over the series-A grid, and NO
// admission changes — shadow admissions equal off admissions exactly), on (the
// rule refuses). The reserve is a SECURITY PARAMETER (Evolving tier): R ≥ K/2 is
// enforced here by clamping; the daemon refuses the flag outright.

import "github.com/nerolabs/silt/ports"

// AddressMode selects what the observed-address cap DOES.
type AddressMode uint8

const (
	AddressCapOff    AddressMode = iota // no rule; classes are still stored for telemetry
	AddressCapShadow                    // evaluate + count every would-be refusal; never refuse
	AddressCapOn                        // refuse
)

func (m AddressMode) String() string {
	switch m {
	case AddressCapShadow:
		return "shadow"
	case AddressCapOn:
		return "on"
	}
	return "off"
}

// ShadowKey is one cell of the would-refuse counter (series A): the bucket, the
// entry's class, and the (reserve, capRelay) hypothesis it was evaluated under.
// The address WIDTH (/24, /32) is a transport setting; the daemon labels series A
// with it.
type ShadowKey struct {
	Bucket   int
	Class    ports.PeerClass
	Reserve  int
	CapRelay int
}

// shadowGrid is the series-A hypothesis grid the reserve must be set from
// (cert §6.3): R ∈ {4, 6, 8} × capRelay ∈ {2, 4}. The configured pair is always
// evaluated too.
var shadowGrid = [][2]int{{4, 2}, {4, 4}, {6, 2}, {6, 4}, {8, 2}, {8, 4}}

// BucketCensus is one row of series E: the per-bucket group-density census. It
// carries counts only — never a group value.
type BucketCensus struct {
	Bucket      int
	Entries     int
	Groups      int // distinct non-zero groups
	GroupsAtCap int // groups holding ≥ min(capDirect, capRelay) entries: at least one class is refused there
	NonDirect   int // relayed + unverified + unclassified (the reserve's load)
}

// entryMeta is the class stored WITH a table entry (C-4).
type entryMeta struct {
	class  ports.PeerClass
	group  uint64
	known  bool // false = unclassified (no completed conversation, no introducer group)
	static bool // seed / -persistent-peers: exempt from every cap
}

func (m entryMeta) direct() bool { return m.known && m.class == ports.ClassDirect }

// contacted reports whether the entry's key was PAID for: a completed
// conversation classified it. Unverified and unclassified entries are not.
func (m entryMeta) contacted() bool { return m.known && m.class != ports.ClassUnverified }

// addressRule is the configured observed-address cap.
type addressRule struct {
	classifier    ports.PeerClassifier
	capDirect     int
	capRelay      int
	reserve       int
	mode          AddressMode
	tableGroupCap int
	shadow        map[ShadowKey]int
}

// SetAddressDiversity turns on the observed-address cap. capDirect / capRelay ≤ 0
// disable that per-group cap; reserve is clamped into [⌈K/2⌉, K] (a value below K/2
// lets the adversary own a bucket majority for free through honest relays — cert
// §4 — so it is never applied; the daemon refuses such a flag before it gets here).
// Resets the shadow counters.
func (t *Table) SetAddressDiversity(cl ports.PeerClassifier, capDirect, capRelay, reserve int, mode AddressMode) {
	if min := (t.k + 1) / 2; reserve < min {
		reserve = min
	}
	if reserve > t.k {
		reserve = t.k
	}
	t.addr.classifier = cl
	t.addr.capDirect = capDirect
	t.addr.capRelay = capRelay
	t.addr.reserve = reserve
	t.addr.mode = mode
	t.addr.shadow = make(map[ShadowKey]int)
}

// SetTableGroupCap bounds one group's entries across the WHOLE table (geth's
// 10-per-/24). n ≤ 0 disables.
func (t *Table) SetTableGroupCap(n int) { t.addr.tableGroupCap = n }

// AddressMode reports the configured mode.
func (t *Table) AddressMode() AddressMode { return t.addr.mode }

// AddressReserve reports the applied (clamped) reserve.
func (t *Table) AddressReserve() int { return t.addr.reserve }

// ObserveIntroduced admits a reply-learned id that was never contacted: class
// UNVERIFIED, charged to the INTRODUCER's group. If the classifier already knows
// id (a conversation completed earlier) it is admitted under its own class.
func (t *Table) ObserveIntroduced(id, introducer ports.NodeID) {
	t.observe(id, &introducer, false)
}

// ObserveStatic admits a configured seed / -persistent-peers entry, exempt from
// every cap (operator-typed, count-bounded, unforgeable).
func (t *Table) ObserveStatic(id ports.NodeID) { t.observe(id, nil, true) }

// EntryClass reports the class and group stored WITH the table entry (C-4);
// known=false if id is not tabled or is unclassified.
func (t *Table) EntryClass(id ports.NodeID) (class ports.PeerClass, group uint64, known bool) {
	m, ok := t.meta[id]
	if !ok || !m.known {
		return 0, 0, false
	}
	return m.class, m.group, true
}

// ShadowRefusals is series A: every would-be refusal recorded in shadow mode.
func (t *Table) ShadowRefusals() map[ShadowKey]int {
	out := make(map[ShadowKey]int, len(t.addr.shadow))
	for k, v := range t.addr.shadow {
		out[k] = v
	}
	return out
}

// RelayFanIn is series B (the per-node half): RELAYED entries per relay group
// and the top relay's share of them (0 when there are none).
func (t *Table) RelayFanIn() (clientsPerRelay map[uint64]int, topShare float64) {
	clientsPerRelay = make(map[uint64]int)
	total, top := 0, 0
	for _, m := range t.meta {
		if m.known && m.class == ports.ClassRelayed {
			clientsPerRelay[m.group]++
			total++
			if clientsPerRelay[m.group] > top {
				top = clientsPerRelay[m.group]
			}
		}
	}
	if total > 0 {
		topShare = float64(top) / float64(total)
	}
	return clientsPerRelay, topShare
}

// GroupCensus is series E: one row per non-empty bucket.
func (t *Table) GroupCensus() []BucketCensus {
	atCap := t.addr.capDirect
	if t.addr.capRelay > 0 && (atCap <= 0 || t.addr.capRelay < atCap) {
		atCap = t.addr.capRelay
	}
	var out []BucketCensus
	for i, b := range t.buckets {
		if len(b) == 0 {
			continue
		}
		row := BucketCensus{Bucket: i, Entries: len(b)}
		groups := make(map[uint64]int)
		for _, id := range b {
			m := t.meta[id]
			if m.known && m.group != 0 {
				groups[m.group]++
			}
			if !m.direct() {
				row.NonDirect++
			}
		}
		row.Groups = len(groups)
		for _, n := range groups {
			if atCap > 0 && n >= atCap {
				row.GroupsAtCap++
			}
		}
		out = append(out, row)
	}
	return out
}

// candidate computes the (class, group) id would be admitted under right now:
// the classifier's answer if it has one; else the introducer's group as
// UNVERIFIED; else unclassified.
func (t *Table) candidate(id ports.NodeID, introducer *ports.NodeID) entryMeta {
	if t.addr.classifier != nil {
		if c, g, known := t.addr.classifier.ClassOf(id); known {
			return entryMeta{class: c, group: g, known: true}
		}
	}
	if introducer != nil {
		if g, ok := t.groupOf(*introducer); ok {
			return entryMeta{class: ports.ClassUnverified, group: g, known: true}
		}
	}
	return entryMeta{}
}

// groupOf resolves the introducer's group: the classifier (the live
// conversation it answered on) first, the stored entry second.
func (t *Table) groupOf(id ports.NodeID) (uint64, bool) {
	if t.addr.classifier != nil {
		if _, g, known := t.addr.classifier.ClassOf(id); known {
			return g, true
		}
	}
	if m, ok := t.meta[id]; ok && m.known {
		return m.group, true
	}
	return 0, false
}

// groupCap is the per-bucket cap for an entry of class m under a capRelay
// hypothesis: capDirect for DIRECT, capRelay for RELAYED, and the tighter of the
// two for UNVERIFIED (an unpaid entry gets no more than the cheapest paid class).
// ≤ 0 = uncapped.
func (t *Table) groupCap(m entryMeta, capRelay int) int {
	switch m.class {
	case ports.ClassDirect:
		return t.addr.capDirect
	case ports.ClassRelayed:
		return capRelay
	}
	c := t.addr.capDirect
	if capRelay > 0 && (c <= 0 || capRelay < c) {
		c = capRelay
	}
	return c
}

// veto reports whether the address rule refuses admitting m into bucket b under
// the (reserve, capRelay) hypothesis, excluding entry `self` from every tally
// (the re-check evaluates an entry against the rest of its bucket).
func (t *Table) veto(b []ports.NodeID, self ports.NodeID, m entryMeta, reserve, capRelay int) bool {
	inGroup, nonDirect := 0, 0
	for _, e := range b {
		if e == self {
			continue
		}
		em := t.meta[e]
		if m.known && m.group != 0 && em.known && em.group == m.group {
			inGroup++ // C-1: one namespace, every class counts
		}
		if !em.direct() {
			nonDirect++
		}
	}
	if m.known && m.group != 0 {
		if cap := t.groupCap(m, capRelay); cap > 0 && inGroup >= cap {
			return true
		}
		if t.addr.tableGroupCap > 0 && t.tableGroupCount(m.group, self) >= t.addr.tableGroupCap {
			return true
		}
	}
	if !m.direct() && nonDirect >= t.k-reserve {
		return true // C-2: the reserve bounds ALL non-DIRECT entries
	}
	return false
}

func (t *Table) tableGroupCount(group uint64, self ports.NodeID) int {
	n := 0
	for id, m := range t.meta {
		if id != self && m.known && m.group == group {
			n++
		}
	}
	return n
}

// decide evaluates the configured rule for m into bucket b (excluding self) and
// reports whether `on` refuses. In shadow it records one would-refuse cell per
// hypothesis in the grid ∪ the configured pair and reports false (the veto
// never applies). Off: false, nothing recorded.
func (t *Table) decide(bucket int, b []ports.NodeID, self ports.NodeID, m entryMeta) bool {
	switch t.addr.mode {
	case AddressCapOn:
		return t.veto(b, self, m, t.addr.reserve, t.addr.capRelay)
	case AddressCapShadow:
		seen := map[[2]int]bool{}
		for _, h := range append(append([][2]int{}, shadowGrid...), [2]int{t.addr.reserve, t.addr.capRelay}) {
			if seen[h] {
				continue
			}
			seen[h] = true
			if t.veto(b, self, m, h[0], h[1]) {
				t.addr.shadow[ShadowKey{Bucket: bucket, Class: m.class, Reserve: h[0], CapRelay: h[1]}]++
			}
		}
	}
	return false
}

// observe is the one admission path (Observe / ObserveIntroduced / ObserveStatic).
func (t *Table) observe(id ports.NodeID, introducer *ports.NodeID, static bool) {
	i := BucketIndex(t.self, id)
	if i < 0 {
		return // never table yourself
	}
	b := t.buckets[i]
	m := t.candidate(id, introducer)
	for j, existing := range b {
		if existing != id {
			continue
		}
		t.buckets[i] = append(append(b[:j:j], b[j+1:]...), id) // move to back
		t.refresh(i, id, m, static)
		return
	}
	if len(b) >= t.k || t.domainSaturated(b, id) {
		return
	}
	m.static = static
	if !static && t.decide(i, b, id, m) {
		return
	}
	t.buckets[i] = append(b, id)
	t.meta[id] = m
}

// refresh updates a tabled entry's stored class on a repeat observation.
//   - class loss (m unknown): inert (C-4).
//   - DIRECT → non-DIRECT: inert (C-3).
//   - a re-introduction of an uncontacted entry: keeps its first charge.
//   - first classification of an uncontacted entry: the re-check — the admission
//     rule applied late against the rest of the bucket; `on` removes a refused
//     entry, shadow counts it, off just re-keys. Never fires for a static entry.
//   - re-key of a contacted entry (DIRECT at a new /24, a punch upgrade): applied
//     only if the new key is admissible under `on`; otherwise the entry keeps the
//     key it paid for (so two /24s cannot fill a bucket by shuffling). Off and
//     shadow always re-key, matching each other exactly.
func (t *Table) refresh(bucket int, id ports.NodeID, m entryMeta, static bool) {
	old := t.meta[id]
	if static {
		old.static = true
	}
	switch {
	case !m.known:
		t.meta[id] = old
		return
	case old.direct() && !m.direct():
		t.meta[id] = old
		return
	case !m.contacted():
		// A re-introduction (another replier named it): the entry keeps its first
		// charge until a conversation classifies it — one charge per entry, in
		// every mode, so off / shadow / on hold identical stored classes.
		t.meta[id] = old
		return
	case !old.contacted():
		m.static = old.static
		if !old.static && t.decide(bucket, t.buckets[bucket], id, m) {
			t.Remove(id)
			return
		}
		t.meta[id] = m
		return
	}
	m.static = old.static
	if m.group != old.group && t.addr.mode == AddressCapOn && !old.static &&
		t.veto(t.buckets[bucket], id, m, t.addr.reserve, t.addr.capRelay) {
		t.meta[id] = old
		return
	}
	t.meta[id] = m
}
