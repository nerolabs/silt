package chain

// R0.4b — the write path of the consensus-attested E -> key_E binding.
//
// Every test here drives a rule whose absence re-opens a concrete channel:
// self-verification (forge someone else's binding), the era gate (write committed
// state a v4 root does not cover), no-backdating (choose key_E after seeing who
// redeems under it), first-write-wins (re-point a committed key), and the retention
// bound (unbounded committed state).

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func issuerKeyTestKey(seed int64) ed25519.PrivateKey { return key(seed) }

// TestIssuerKeyRegIsSelfVerifying: the record verifies standalone, and any tamper —
// to the epoch, the fingerprint, or the identity — breaks it. Without this a
// proposer could mint a binding for a validator it does not control, which IS the
// targeted-key attack, just spelled differently.
func TestIssuerKeyRegIsSelfVerifying(t *testing.T) {
	k := issuerKeyTestKey(91001)
	fp := ports.Hash{0x11}
	r := SignIssuerKeyReg(k, 7, fp)

	if !VerifyIssuerKeyReg(r) {
		t.Fatal("an honestly signed registration did not verify")
	}
	if got := r.IssuerID(); got != ports.HashBytes(k.Public().(ed25519.PublicKey)) {
		t.Fatalf("IssuerID %x is not sha256(Pub) — the identity is asserted, not derived", got)
	}

	tampered := r
	tampered.Epoch = 8
	if VerifyIssuerKeyReg(tampered) {
		t.Fatal("a registration with a rewritten EPOCH verified — the epoch is outside the signature")
	}
	tampered = r
	tampered.Fingerprint = ports.Hash{0x22}
	if VerifyIssuerKeyReg(tampered) {
		t.Fatal("a registration with a rewritten FINGERPRINT verified — the key is outside the signature")
	}
	tampered = r
	tampered.Pub = ed25519.PublicKey(issuerKeyTestKey(91002).Public().(ed25519.PublicKey))
	if VerifyIssuerKeyReg(tampered) {
		t.Fatal("a registration re-pointed at ANOTHER identity verified — a validator's binding is forgeable")
	}
	if VerifyIssuerKeyReg(IssuerKeyReg{}) {
		t.Fatal("an empty registration verified")
	}
}

// issuerKeyChain builds an epoch-enabled chain with no bond requirement (so the
// objective bonded gate is inert) at the given epoch length.
func issuerKeyChain(epochBlocks uint64) *Chain {
	cfg := Config{Quorum: 1, EpochBlocks: epochBlocks}
	return New(cfg, func(ports.NodeID) int64 { return 1 << 30 })
}

// TestIssuerKeyRejectedBelowEra4 is the FREEZE guard: the era-3 leaf set does not
// commit this keyspace, so a v4 block carrying a registration would write committed
// state its own committed root does not cover — a silent divergence. It must be
// rejected outright.
func TestIssuerKeyRejectedBelowEra4(t *testing.T) {
	c := issuerKeyChain(8)
	r := SignIssuerKeyReg(issuerKeyTestKey(91010), 0, ports.Hash{0x33})

	for _, v := range []uint64{BlockVersionRounds, BlockVersionStateRoot} {
		b := &Block{Version: v, Height: 1, IssuerKeys: []IssuerKeyReg{r}}
		if err := c.validateIssuerKeys(b); err == nil {
			t.Fatalf("a v%d block carrying an issuer-key registration was accepted — "+
				"the era-3 byte-identical freeze is broken", v)
		}
	}
	b := &Block{Version: BlockVersionWitnessable, Height: 1, IssuerKeys: []IssuerKeyReg{r}}
	if err := c.validateIssuerKeys(b); err != nil {
		t.Fatalf("a v5 block carrying a valid registration was rejected: %v", err)
	}
}

// TestIssuerKeyRejectsBackdating is the equivocation guard. Committing key_E AFTER
// epoch E has run is exactly "choose the key once I know who is watching": the
// issuer would learn which cohort redeemed and then bind a key that partitions them.
// Forward pre-publication (a key SCHEDULE) is allowed and bounded.
func TestIssuerKeyRejectsBackdating(t *testing.T) {
	c := issuerKeyChain(8)
	k := issuerKeyTestKey(91020)
	const height = 40 // epoch 5 at EpochBlocks=8

	cases := []struct {
		epoch uint64
		ok    bool
		why   string
	}{
		{4, false, "one epoch in the PAST — backdating is the equivocation move"},
		{0, false, "the genesis epoch, long past"},
		{5, true, "the block's own epoch"},
		{5 + issuerKeyPrePublish, true, "the far edge of the pre-publication window"},
		{5 + issuerKeyPrePublish + 1, false, "beyond the pre-publication window — unbounded look-ahead"},
	}
	for _, tc := range cases {
		b := &Block{Version: BlockVersionWitnessable, Height: height,
			IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, tc.epoch, ports.Hash{0x44})}}
		err := c.validateIssuerKeys(b)
		if tc.ok && err != nil {
			t.Fatalf("epoch %d (%s) was rejected: %v", tc.epoch, tc.why, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("epoch %d (%s) was ACCEPTED", tc.epoch, tc.why)
		}
	}
}

// TestIssuerKeyIsAppendOnly is THE security property. Once (epoch, issuer) is
// committed it can never be re-pointed — not by the same issuer, not in the same
// block, not in a later one. An issuer able to rewrite key_E after redemptions began
// has the whole targeted-key channel back.
func TestIssuerKeyIsAppendOnly(t *testing.T) {
	c := issuerKeyChain(8)
	k := issuerKeyTestKey(91030)
	id := ports.HashBytes(k.Public().(ed25519.PublicKey))
	first := ports.Hash{0x55}
	second := ports.Hash{0x66}

	c.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: 0,
		IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, 0, first)}})
	got, ok := c.IssuerKeyCommitment(id, 0)
	if !ok || got != first {
		t.Fatalf("first commitment: got (%x, %v), want (%x, true)", got, ok, first)
	}

	// A later block re-points the same (epoch, issuer).
	c.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: 1,
		IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, 0, second)}})
	if got, _ = c.IssuerKeyCommitment(id, 0); got != first {
		t.Fatalf("a committed key_E was RE-POINTED to %x — append-only is broken, and with it "+
			"the anti-fingerprinting binding", got)
	}

	// Two conflicting registrations inside ONE block: the first wins, deterministically.
	c2 := issuerKeyChain(8)
	c2.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: 0, IssuerKeys: []IssuerKeyReg{
		SignIssuerKeyReg(k, 0, first),
		SignIssuerKeyReg(k, 0, second),
	}})
	if got, _ = c2.IssuerKeyCommitment(id, 0); got != first {
		t.Fatalf("intra-block conflict resolved to %x, want the FIRST (%x)", got, first)
	}
}

// TestIssuerKeyFirstWriteWinsIsOrderIndependent: the surviving map is identical
// whichever order the registrations are applied in, so long as the SET of
// (epoch, issuer) pairs is the same. This is the claim orderVacuous cites for this
// field — two orderings differ only in which DUPLICATE is skipped, and a duplicate
// is skipped either way.
func TestIssuerKeyFirstWriteWinsIsOrderIndependent(t *testing.T) {
	kA, kB := issuerKeyTestKey(91040), issuerKeyTestKey(91041)
	idA := ports.HashBytes(kA.Public().(ed25519.PublicKey))
	idB := ports.HashBytes(kB.Public().(ed25519.PublicKey))

	regs := []IssuerKeyReg{
		SignIssuerKeyReg(kA, 0, ports.Hash{0x01}),
		SignIssuerKeyReg(kB, 0, ports.Hash{0x02}),
		SignIssuerKeyReg(kA, 1, ports.Hash{0x03}),
		SignIssuerKeyReg(kB, 1, ports.Hash{0x04}),
	}
	reversed := make([]IssuerKeyReg, len(regs))
	for i := range regs {
		reversed[len(regs)-1-i] = regs[i]
	}

	fwd, rev := issuerKeyChain(8), issuerKeyChain(8)
	fwd.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: 0, IssuerKeys: regs})
	rev.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: 0, IssuerKeys: reversed})

	for _, id := range []ports.NodeID{idA, idB} {
		for _, e := range []uint64{0, 1} {
			a, aok := fwd.IssuerKeyCommitment(id, e)
			b, bok := rev.IssuerKeyCommitment(id, e)
			if a != b || aok != bok {
				t.Fatalf("(%x, epoch %d): forward=(%x,%v) reversed=(%x,%v) — "+
					"the committed binding depends on apply ORDER, so the state root is not history-independent",
					id[:4], e, a, aok, b, bok)
			}
		}
	}
}

// TestIssuerKeyCommitIsBounded: the committed keyspace does not grow forever. As the
// head advances, epochs that have left the retention band are pruned, so the map is
// bounded by (band width) x (issuers) — build-immutable #8 applied to committed
// state, which is the version that matters most because it is never forgotten
// otherwise.
func TestIssuerKeyCommitIsBounded(t *testing.T) {
	const epochBlocks = 8
	c := issuerKeyChain(epochBlocks)
	k := issuerKeyTestKey(91050)
	id := ports.HashBytes(k.Public().(ed25519.PublicKey))

	// Register the current epoch's key at every epoch boundary for 50 epochs.
	for e := uint64(0); e < 50; e++ {
		h := e * epochBlocks
		c.applyIssuerKeys(Block{Version: BlockVersionWitnessable, Height: h,
			IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, e, ports.Hash{byte(e)})}})
	}
	// Band is [cur-W, cur+W]: at most 2W+1 epochs survive.
	if bound := int(2*issuerKeyPrePublish + 1); len(c.issuerKeyCommit) > bound {
		t.Fatalf("issuerKeyCommit holds %d epochs after 50 rotations, bound is %d — "+
			"committed state grows without limit", len(c.issuerKeyCommit), bound)
	}
	// The CURRENT epoch's binding must survive the pruning, or redemption breaks.
	if _, ok := c.IssuerKeyCommitment(id, 49); !ok {
		t.Fatal("pruning dropped the CURRENT epoch's binding — every live token becomes unresolvable")
	}
	// A long-expired epoch must be gone.
	if _, ok := c.IssuerKeyCommitment(id, 0); ok {
		t.Fatal("an epoch far outside the retention band survived pruning")
	}
}

// TestIssuerKeyDoesNotLeakIntoTheEra3Root is the freeze guard at the ROOT level: with
// the binding fully populated, the era-3 (v4) root must be byte-identical to the root
// over a chain with the binding empty. If it leaked, every deployed v4 node would
// diverge.
func TestIssuerKeyDoesNotLeakIntoTheEra3Root(t *testing.T) {
	empty := &Chain{}
	populateCommitted(empty)
	empty.issuerKeyCommit = nil

	full := &Chain{}
	populateCommitted(full) // populates issuerKeyCommit

	emptyRoot, err := empty.StateRoot()
	if err != nil {
		t.Fatalf("era-3 root (binding empty): %v", err)
	}
	fullRoot, err := full.StateRoot()
	if err != nil {
		t.Fatalf("era-3 root (binding populated): %v", err)
	}
	if emptyRoot != fullRoot {
		t.Fatalf("the era-3 (v4) root MOVED when issuerKeyCommit was populated (%x vs %x) — "+
			"a v5-only keyspace leaked into the frozen era-3 leaf set", emptyRoot, fullRoot)
	}

	// And it MUST move the v5 root, or the commitment commits nothing.
	emptyV5, err := empty.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 root (binding empty): %v", err)
	}
	fullV5, err := full.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 root (binding populated): %v", err)
	}
	if emptyV5 == fullV5 {
		t.Fatal("the v5 root did NOT move when issuerKeyCommit was populated — " +
			"the binding is not actually committed, so no node can detect an off-commitment key")
	}
}

// TestIssuerKeyPrePublishMatchesDemandWindow pins the coupling the design doc names:
// the on-chain pre-publication window and the demand-lane validity window W are the
// same number. If they drift apart, either a key is committed too late for a token
// that still verifies (an honest unpaid delivery) or a key is committed for an epoch
// no token can reach.
//
// core/chain deliberately carries NO dependency on core/demand, so the constant is
// duplicated and pinned here by VALUE rather than by import.
func TestIssuerKeyPrePublishMatchesDemandWindow(t *testing.T) {
	const demandDefaultWindow = 4 // core/demand.DefaultWindow
	if issuerKeyPrePublish != demandDefaultWindow {
		t.Fatalf("issuerKeyPrePublish=%d but demand.DefaultWindow=%d — the on-chain key "+
			"schedule and the token validity window have drifted apart",
			issuerKeyPrePublish, demandDefaultWindow)
	}
}
