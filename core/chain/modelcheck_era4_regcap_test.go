package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// era-4 build increment 4c — the RegCap per-block TOTAL BondReg count validity rule,
// model-check tier. RegCap is a consensus block-validity predicate, so it lives beside the
// other modelcheck_* validity oracles and is enforced through ValidateProposal (the path
// every replica runs on receipt).
//
// The certified rule (era4-regcap-recert-VERDICT / era4-regcap-VALUE-DERIVATION-VERDICT,
// both 2026-08-29): a v5 block is INVALID if len(canonicalBondRegs(b.BondRegs)) > RegCap,
// counting fresh AND renewal alike, after the same-id fold. N = RegCap = 256.
//
// These oracles prove EXACTLY the certified properties, each with its RED named:
//
//   A. > RegCap ALL-FRESH regs → REJECT.
//   B. > RegCap ALL-RENEWAL regs → REJECT (the flipped ablation: the REFUTED fresh-only
//      rule wrongly ACCEPTED an all-renewal over-cap block).
//   C. > RegCap MIXED (fresh + renewal) → REJECT.
//   D. the count is AFTER canonicalBondRegs: a same-id renew/resize pair counts as ONE.
//   E. I4 liveness: a block exactly AT the ceiling (RegCap, any mix) ACCEPTS.
//   F. v4 is UNAFFECTED: a v4 block with > RegCap regs is NOT rejected by RegCap (v5-only).
//
// The RED for A/B/C/E-ablation is: delete the `b.Version >= BlockVersionWitnessable` RegCap
// gate in validateBondRegs and the over-cap block ACCEPTS (or, for E, set RegCap = 255 and
// the at-ceiling block wrongly REJECTS). Each is demonstrated in the 4c build report.

// era4RegCapChain builds an objective launch-phase chain whose proposer is a bonded launch
// anchor (so its proposed block clears ValidateProposal's era-2 proposer checks and reaches
// the RegCap gate + the v5 root predicate). The regGate stays INACTIVE (no EpochBlocks, no
// RegGateActivationHeight), so the #506 per-identity R-interval never fires — a renewal in a
// later block is admitted, which is exactly what lets these oracles exercise the renewal
// term the total-count rule must bound. `renewIDs` are pre-registered in genesis (their
// bondRegHeight is set to 0), so re-registering them in a later block is a genuine RENEWAL
// (bondRegHeight[id] present), not a fresh reg — the distinction the fresh-only rule turned
// on and the total rule ignores.
func era4RegCapChain(t *testing.T, renewIDs []ed25519.PrivateKey) (*Chain, ed25519.PrivateKey) {
	t.Helper()
	prop := key(40301)
	att1 := key(40302)
	cfg := Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors:      map[ports.NodeID]bool{idOf(prop): true, idOf(att1): true},
		AnchorQuorum: 1,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(att1, twoMiB, ports.Hash{}),
	)
	// Pre-register the renewal ids in genesis so bondRegHeight[id]=0 → a later reg for the
	// same id is a RENEWAL, not fresh.
	for _, r := range renewIDs {
		g.BondRegs = append(g.BondRegs, bondReg(r, twoMiB, ports.Hash{}))
	}
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, prop
}

// regKeys deterministically derives n distinct validator keys from a base seed. Distinct
// seeds → distinct pubkeys → distinct ValidatorIDs AND distinct bond Roots (bondReg roots
// are HashBytes(pub)), so the per-root distinct-id dedup never fires on these.
func regKeys(base, n int) []ed25519.PrivateKey {
	out := make([]ed25519.PrivateKey, n)
	for i := 0; i < n; i++ {
		out[i] = key(int64(base + i))
	}
	return out
}

// buildV5WithRegs builds a v5 block at the chain head carrying one BondReg per key in
// `regs`, with honest v5 roots (post-apply recompute via the v5 leaf marshaller) so the
// block is otherwise VALID — only the RegCap count is under test. Roots are set before
// signing so the signature covers them.
func buildV5WithRegs(t *testing.T, c *Chain, prop ed25519.PrivateKey, regs []ed25519.PrivateKey) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{
		Version: BlockVersionWitnessable,
		Height:  next,
		Prev:    prev,
		Entries: []ports.Entry{entry(9)},
	}
	for _, r := range regs {
		b.BondRegs = append(b.BondRegs, bondReg(r, twoMiB, prev))
	}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot = &state
	b.LogRoot = &log
	Sign(b, prop)
	return b
}

// TestRegCapAllFreshOverCapRejected — gate A. A v5 block with RegCap+1 all-FRESH regs is
// rejected with ErrRegCapExceeded. RED: remove the RegCap gate in validateBondRegs → this
// over-cap block ACCEPTS.
func TestRegCapAllFreshOverCapRejected(t *testing.T) {
	c, prop := era4RegCapChain(t, nil)
	fresh := regKeys(410000, RegCap+1) // 257 brand-new ids
	b := buildV5WithRegs(t, c, prop, fresh)

	if got := len(canonicalBondRegs(b.BondRegs)); got != RegCap+1 {
		t.Fatalf("fixture: canonical count = %d, want %d (regs must not fold)", got, RegCap+1)
	}
	if err := c.ValidateProposal(b); !errors.Is(err, ErrRegCapExceeded) {
		t.Fatalf("v5 block with %d all-fresh regs (cap %d): want ErrRegCapExceeded, got %v",
			RegCap+1, RegCap, err)
	}
}

// TestRegCapAllRenewalOverCapRejected — gate B, the FLIPPED ablation. A v5 block with
// RegCap+1 all-RENEWAL regs (every id pre-registered in genesis, so bondRegHeight[id] is
// set) is rejected. The REFUTED fresh-only rule would have EXEMPTED all of these and
// wrongly accepted the block; the certified total-count rule rejects it. RED: remove the
// RegCap gate → this over-cap all-renewal block ACCEPTS.
func TestRegCapAllRenewalOverCapRejected(t *testing.T) {
	renew := regKeys(420000, RegCap+1) // 257 ids, all pre-registered in genesis below
	c, prop := era4RegCapChain(t, renew)
	// Confirm the fixture actually made them renewals: bondRegHeight[id] set from genesis.
	for _, r := range renew {
		if _, ok := c.bondRegHeight[idOf(r)]; !ok {
			t.Fatalf("fixture: id %s has no genesis bondRegHeight — not a renewal", idOf(r))
		}
	}
	b := buildV5WithRegs(t, c, prop, renew)

	if got := len(canonicalBondRegs(b.BondRegs)); got != RegCap+1 {
		t.Fatalf("fixture: canonical count = %d, want %d", got, RegCap+1)
	}
	if err := c.ValidateProposal(b); !errors.Is(err, ErrRegCapExceeded) {
		t.Fatalf("v5 block with %d all-RENEWAL regs (cap %d): want ErrRegCapExceeded, got %v "+
			"(the fresh-only rule would have wrongly accepted this)", RegCap+1, RegCap, err)
	}
}

// TestRegCapMixedOverCapRejected — gate C. A v5 block with a fresh+renewal MIX totalling
// RegCap+1 is rejected. 130 fresh + 127 renewal = 257 total. RED: remove the RegCap gate →
// the mixed over-cap block ACCEPTS.
func TestRegCapMixedOverCapRejected(t *testing.T) {
	const nFresh, nRenew = 130, 127 // 257 total = RegCap+1
	renew := regKeys(430000, nRenew)
	c, prop := era4RegCapChain(t, renew)
	fresh := regKeys(431000, nFresh)
	mix := append(append([]ed25519.PrivateKey(nil), fresh...), renew...)

	if len(mix) != RegCap+1 {
		t.Fatalf("fixture: mix size = %d, want %d", len(mix), RegCap+1)
	}
	b := buildV5WithRegs(t, c, prop, mix)
	if got := len(canonicalBondRegs(b.BondRegs)); got != RegCap+1 {
		t.Fatalf("fixture: canonical count = %d, want %d", got, RegCap+1)
	}
	if err := c.ValidateProposal(b); !errors.Is(err, ErrRegCapExceeded) {
		t.Fatalf("v5 block with %d fresh + %d renewal = %d total (cap %d): want ErrRegCapExceeded, got %v",
			nFresh, nRenew, RegCap+1, RegCap, err)
	}
}

// TestRegCapCountedAfterCanonicalFold — gate D. The count is len(canonicalBondRegs(...)),
// NOT len(b.BondRegs). A block that lists RegCap distinct ids PLUS one same-id renew/resize
// duplicate (so len(b.BondRegs) == RegCap+1 but the canonical fold yields RegCap) must
// ACCEPT — the duplicate folds to one. RED: count len(b.BondRegs) instead of the canonical
// fold → this block wrongly REJECTS.
func TestRegCapCountedAfterCanonicalFold(t *testing.T) {
	c, prop := era4RegCapChain(t, nil)
	ids := regKeys(440000, RegCap) // RegCap distinct ids
	prev, next := c.Head()
	b := &Block{
		Version: BlockVersionWitnessable,
		Height:  next,
		Prev:    prev,
		Entries: []ports.Entry{entry(9)},
	}
	for _, r := range ids {
		b.BondRegs = append(b.BondRegs, bondReg(r, twoMiB, prev))
	}
	// One same-id DUPLICATE of ids[0], a resize-up (larger size): a legitimate same-id
	// multi-reg that canonicalBondRegs folds to one (largest size wins). This makes the raw
	// list RegCap+1 but the canonical count RegCap.
	dup := bondReg(ids[0], twoMiB*2, prev)
	b.BondRegs = append(b.BondRegs, dup)

	if raw := len(b.BondRegs); raw != RegCap+1 {
		t.Fatalf("fixture: raw count = %d, want %d", raw, RegCap+1)
	}
	if canon := len(canonicalBondRegs(b.BondRegs)); canon != RegCap {
		t.Fatalf("fixture: canonical count = %d, want %d (the same-id pair must fold)", canon, RegCap)
	}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, prop)

	if err := c.ValidateProposal(b); err != nil {
		t.Fatalf("v5 block with %d raw regs folding to %d canonical (cap %d): want ACCEPT, got %v "+
			"(RegCap must count AFTER canonicalBondRegs)", RegCap+1, RegCap, RegCap, err)
	}
}

// TestRegCapAtCeilingAccepted — gate E, the I4 liveness edge. A v5 block with EXACTLY RegCap
// regs (a fresh+renewal mix) ACCEPTS — the predicate must not reject an honest at-ceiling
// block. RED (gate ablation): set RegCap = 255 and this exactly-256 block wrongly REJECTS.
func TestRegCapAtCeilingAccepted(t *testing.T) {
	const nRenew = 100
	renew := regKeys(450000, nRenew)
	c, prop := era4RegCapChain(t, renew)
	fresh := regKeys(451000, RegCap-nRenew) // 156 fresh + 100 renewal = 256 total
	mix := append(append([]ed25519.PrivateKey(nil), fresh...), renew...)

	if len(mix) != RegCap {
		t.Fatalf("fixture: at-ceiling size = %d, want %d", len(mix), RegCap)
	}
	b := buildV5WithRegs(t, c, prop, mix)
	if got := len(canonicalBondRegs(b.BondRegs)); got != RegCap {
		t.Fatalf("fixture: canonical count = %d, want %d", got, RegCap)
	}
	if err := c.ValidateProposal(b); err != nil {
		t.Fatalf("v5 block AT the ceiling (%d regs, %d fresh + %d renewal): want ACCEPT, got %v",
			RegCap, RegCap-nRenew, nRenew, err)
	}
}

// TestRegCapDoesNotAffectV4 — gate F. RegCap is v5-ONLY. A v4 block carrying > RegCap regs
// is NOT rejected by the RegCap rule (it must fail, if at all, for era-3 reasons, never
// ErrRegCapExceeded). This proves the v5 gate keeps era-3 (v4) validity byte- and
// behaviour-identical — the frozen era-3 format (#632) is untouched. RED: drop the
// `b.Version >= BlockVersionWitnessable` gate → the v4 block is wrongly rejected with
// ErrRegCapExceeded.
func TestRegCapDoesNotAffectV4(t *testing.T) {
	c, prop := era4RegCapChain(t, nil)
	fresh := regKeys(460000, RegCap+1) // 257 regs, more than the cap
	prev, next := c.Head()
	b := &Block{
		Version: BlockVersionStateRoot, // v4 — era-3, no RegCap
		Height:  next,
		Prev:    prev,
		Entries: []ports.Entry{entry(9)},
	}
	for _, r := range fresh {
		b.BondRegs = append(b.BondRegs, bondReg(r, twoMiB, prev))
	}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, prop)

	// The v4 block may or may not pass full validation for other reasons, but it must NEVER
	// fail with ErrRegCapExceeded — RegCap does not apply to v4.
	if err := c.ValidateProposal(b); errors.Is(err, ErrRegCapExceeded) {
		t.Fatalf("v4 block with %d regs was rejected by RegCap — RegCap must be v5-only, got %v",
			RegCap+1, err)
	}
}

// TestReloadRejectsResignedWrongStateRootV5 is the predicate-first ROOT half (gate G, root
// side): the v5 committed-root validity predicate fires on the OWN-DISK Reload path, not
// only the commit path. It is the v5 sibling of TestReloadRejectsResignedWrongStateRootV4.
// The widen to versionSupported <= 5 (4c) must NOT accept a v5 block whose committed root is
// wrong: a re-signed wrong-StateRoot v5 block passes every signature check (integrity ≠
// root-correctness) but the own-disk Reload path (appendStructural → validateEra3Roots,
// which recomputes via StateRootForVersion(5)) must reject it with ErrEra3StateRootMismatch.
//
// This proves the v5 root predicate is live and on every disk-write path the instant the
// ceiling widens — the machinery 4b wired makes it automatic; 4c's version widen must ride
// it, never outrun it. RED: revert versionSupported to <= 4 and Reload rejects the v5 block
// at DECODE with ErrBlockVersion (not the root check) — proving the widen is what lets v5
// reach the root predicate at all. GREEN: the widen is in place AND the root predicate fires.
func TestReloadRejectsResignedWrongStateRootV5(t *testing.T) {
	c, prop := era3ValidityChain(t)
	att1, att2, att3 := key(30202), key(30203), key(30204)
	keys := []ed25519.PrivateKey{prop, att1, att2, att3}

	// Commit an HONEST v5 block on disk (roots = post-apply v5 recompute), through the
	// commit path (which now accepts v5). This is the block the tamper then corrupts.
	prev, next := c.Head()
	b := &Block{Version: BlockVersionWitnessable, Height: next, Prev: prev, Entries: []ports.Entry{entry(9)}}
	state, log, err := c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	commitRounds(b, keys, 0)
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit honest v5 block: %v", err)
	}

	persisted, err := DecodeBlocks(EncodeBlocks(c.Blocks(0)))
	if err != nil {
		t.Fatalf("wire roundtrip: %v", err)
	}
	if last := persisted[len(persisted)-1]; last.Version != BlockVersionWitnessable {
		t.Fatalf("last persisted block is v%d, want v%d — fixture is not era-4", last.Version, BlockVersionWitnessable)
	}

	// Tamper: corrupt the v5 block's StateRoot and RE-SIGN with the proposer key + attesters,
	// so the block is otherwise byte-valid. Only the root check can catch it.
	tampered := append([]Block(nil), persisted...)
	li := len(tampered) - 1
	tb := tampered[li]
	wrong := *tb.StateRoot
	wrong[0] ^= 0xFF
	tb.StateRoot = &wrong
	tb.hashMemoSet = false
	Sign(&tb, prop)
	tb.Atts = nil
	for _, k := range []ed25519.PrivateKey{att1, att2, att3} {
		tb.Atts = append(tb.Atts, Attest(&tb, k))
	}
	tampered[li] = tb
	tampered, err = DecodeBlocks(EncodeBlocks(tampered))
	if err != nil {
		t.Fatalf("tamper roundtrip: %v", err)
	}

	fresh := New(reloadCfg(), func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	n, err := fresh.Reload(tampered)
	if !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("Reload of a re-signed wrong-StateRoot v5 block: got err=%v (restored %d), "+
			"want ErrEra3StateRootMismatch — the v5 root predicate must fire on the own-disk "+
			"Reload path, exactly as it does for v4", err, n)
	}
	if n != len(persisted)-1 {
		t.Errorf("Reload restored %d blocks before rejecting the tampered v5 block; want %d",
			n, len(persisted)-1)
	}
}
