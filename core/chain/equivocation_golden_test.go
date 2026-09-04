package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/ports"
)

// CD-2 (composed-direction cert, 2026-09-03): VerifyEquivocation's ACCEPT SET must be
// byte-identical before and after the LastCommit carrier merge — "proven by a gate,
// not by review". This golden corpus pins it: a fixed set of Equivocation fixtures,
// built deterministically (fixed key seeds, fixed entries, ed25519 is deterministic),
// serialised once to testdata/equivocation_golden.cbor with the verdict
// CheckEquivocation returned at pin time. The test DECODES the committed bytes and
// asserts every verdict; it never regenerates silently. A merge that touches
// signers, consensusSigScopes, signedBlock, bodyHash or the Attestation encoding and
// moves one verdict reddens here.
//
// Regenerate ONLY with an intentional accept-set change, then diff the verdicts:
//
//	go test ./core/chain/ -run EquivocationGolden -update-equivocation-golden
var updateEquivocationGolden = flag.Bool("update-equivocation-golden", false,
	"rewrite testdata/equivocation_golden.cbor from the deterministic builder (an ACCEPT-SET change; never routine)")

const equivocationGoldenPath = "testdata/equivocation_golden.cbor"

// equivocationGoldenCases is the pinned case count. A builder that grows or shrinks
// must update this AND the corpus in the same commit.
const equivocationGoldenCases = 26

type equivocationGoldenCase struct {
	Name    string       `cbor:"1,keyasint"`
	Proof   Equivocation `cbor:"2,keyasint"`
	Verdict string       `cbor:"3,keyasint"`
}

const (
	verdictAccept = "accept"
	verdictNotEq  = "ErrNotEquivocation"
	verdictPruned = "ErrPrunedEvidence"
)

func equivocationVerdict(err error) string {
	switch {
	case err == nil:
		return verdictAccept
	case errors.Is(err, ErrNotEquivocation):
		return verdictNotEq
	case errors.Is(err, ErrPrunedEvidence):
		return verdictPruned
	default:
		return "other:" + err.Error()
	}
}

// goldenBlock builds a block at (version, height) over prev with one entry, signed
// by proposer. Era-1 attesters sign the bare hash; era-2 attesters sign at
// (round, phase) and land in Atts or PrepareQC as directed.
type goldenAtt struct {
	k     ed25519.PrivateKey
	round uint64
	phase uint8
	inQC  bool
}

func goldenBlock(version, height uint64, prev ports.Hash, e byte, proposer ed25519.PrivateKey, atts ...goldenAtt) Block {
	b := Block{Version: version, Height: height, Prev: prev, Entries: []ports.Entry{entry(e)}}
	Sign(&b, proposer)
	for _, a := range atts {
		var att Attestation
		if a.phase == PhaseLegacy {
			att = Attest(&b, a.k)
		} else {
			att = AttestAt(&b, a.k, a.round, a.phase)
		}
		if a.inQC {
			b.PrepareQC = append(b.PrepareQC, att)
		} else {
			b.Atts = append(b.Atts, att)
		}
	}
	return b
}

// buildEquivocationGoldenCorpus is the deterministic builder. Verdicts are NOT
// hard-coded here: the builder records what CheckEquivocation says NOW, and the pin is
// the committed file. The names document the intended verdict so a diff is readable.
func buildEquivocationGoldenCorpus() []equivocationGoldenCase {
	prop, propB, culprit, other := key(21000), key(21001), key(21002), key(21003)
	cul := pubOf(culprit)
	prev := ports.HashBytes([]byte("golden-genesis"))
	leg := func(k ed25519.PrivateKey) goldenAtt { return goldenAtt{k: k} }
	e2 := func(k ed25519.PrivateKey, r uint64, ph uint8, qc bool) goldenAtt {
		return goldenAtt{k: k, round: r, phase: ph, inQC: qc}
	}
	var cases []equivocationGoldenCase
	add := func(name string, e Equivocation) {
		cases = append(cases, equivocationGoldenCase{Name: name, Proof: e, Verdict: equivocationVerdict(CheckEquivocation(&e))})
	}

	// ---- era 1 ----
	a1 := goldenBlock(1, 1, prev, 1, prop, leg(culprit), leg(other))
	b1 := goldenBlock(1, 1, prev, 2, propB, leg(culprit))
	add("era1-attester-in-both-ACCEPT", Equivocation{Culprit: cul, A: a1, B: b1})
	pa := goldenBlock(1, 1, prev, 1, culprit, leg(other))
	pb := goldenBlock(1, 1, prev, 2, culprit, leg(other))
	add("era1-proposer-of-both-ACCEPT", Equivocation{Culprit: cul, A: pa, B: pb})
	add("era1-proposer-A-attester-B-ACCEPT", Equivocation{Culprit: cul, A: pa, B: b1})
	c2 := goldenBlock(1, 2, a1.Hash(), 3, prop, leg(culprit))
	add("era1-sequential-heights-REFUSE", Equivocation{Culprit: cul, A: a1, B: c2})
	add("era1-same-block-twice-REFUSE", Equivocation{Culprit: cul, A: a1, B: a1})
	add("era1-signed-neither-REFUSE", Equivocation{Culprit: pubOf(key(21004)), A: a1, B: b1})
	add("era1-bad-culprit-key-length-REFUSE", Equivocation{Culprit: cul[:31], A: a1, B: b1})
	forged := b1
	forged.Atts = []Attestation{{PubKey: cul, Sig: make([]byte, 64)}}
	add("era1-forged-signature-in-B-REFUSE", Equivocation{Culprit: cul, A: a1, B: forged})
	onlyA := goldenBlock(1, 1, prev, 2, propB, leg(other))
	add("era1-signed-A-only-REFUSE", Equivocation{Culprit: cul, A: a1, B: onlyA})

	// ---- era 2 (rounds) ----
	a2 := goldenBlock(2, 1, prev, 1, prop, e2(culprit, 1, PhasePrecommit, false))
	b2 := goldenBlock(2, 1, prev, 2, propB, e2(culprit, 1, PhasePrecommit, false))
	add("era2-precommit-same-round-ACCEPT", Equivocation{Culprit: cul, A: a2, B: b2})
	qa := goldenBlock(2, 1, prev, 1, prop, e2(culprit, 1, PhasePrepare, true))
	qb := goldenBlock(2, 1, prev, 2, propB, e2(culprit, 1, PhasePrepare, true))
	add("era2-prepareqc-only-signer-ACCEPT", Equivocation{Culprit: cul, A: qa, B: qb})
	xa := goldenBlock(2, 1, prev, 1, prop, e2(culprit, 1, PhasePrepare, false))
	add("era2-prepare-in-Atts-vs-PrepareQC-same-slot-ACCEPT", Equivocation{Culprit: cul, A: xa, B: qb})
	r2 := goldenBlock(2, 1, prev, 2, propB, e2(culprit, 2, PhasePrecommit, false))
	add("era2-cross-round-lock-change-REFUSE", Equivocation{Culprit: cul, A: a2, B: r2})
	add("era2-cross-phase-same-round-REFUSE", Equivocation{Culprit: cul, A: a2, B: qb})
	p2a := goldenBlock(2, 1, prev, 1, culprit, e2(other, 1, PhasePrecommit, false))
	p2b := goldenBlock(2, 1, prev, 2, culprit, e2(other, 1, PhasePrecommit, false))
	add("era2-proposer-sig-only-authorship-REFUSE", Equivocation{Culprit: cul, A: p2a, B: p2b})
	la := goldenBlock(2, 1, prev, 1, prop, leg(culprit))
	lb := goldenBlock(2, 1, prev, 2, propB, leg(culprit))
	add("era2-legacy-shaped-att-inside-v2-REFUSE", Equivocation{Culprit: cul, A: la, B: lb})
	add("era2-bad-culprit-key-length-REFUSE", Equivocation{Culprit: cul[:31], A: a2, B: b2})
	add("era2-same-block-twice-REFUSE", Equivocation{Culprit: cul, A: a2, B: a2})
	v4 := goldenBlock(4, 1, prev, 1, prop, e2(culprit, 3, PhasePrecommit, false))
	v5 := goldenBlock(5, 1, prev, 2, propB, e2(culprit, 3, PhasePrecommit, false))
	add("v4-vs-v5-both-rounds-era-same-slot-ACCEPT", Equivocation{Culprit: cul, A: v4, B: v5})

	// ---- mixed eras ----
	add("mixed-era1-era2-REFUSE", Equivocation{Culprit: cul, A: a1, B: b2})

	// ---- pruned evidence (R0.6) ----
	add("pruned-A-REFUSE", Equivocation{Culprit: cul, A: a1.Prune(), B: b1})
	add("pruned-B-REFUSE", Equivocation{Culprit: cul, A: a1, B: b1.Prune()})
	add("pruned-both-REFUSE", Equivocation{Culprit: cul, A: a1.Prune(), B: b1.Prune()})

	// ---- the I5 cross-height forgery ----
	// Two GENUINE signatures at heights 1 and 2; the accuser relabels the h2 block to
	// h1 and carries its real pre-relabel hash in Pruned. With Pruned read this was a
	// double-sign; with the body recomputed it is pruned evidence, refused.
	honest2 := goldenBlock(1, 2, a1.Hash(), 3, prop, leg(culprit))
	forgeP := honest2
	forgeP.Pruned = honest2.bodyHash()
	forgeP.Height = 1
	add("i5-cross-height-forgery-via-Pruned-REFUSE", Equivocation{Culprit: cul, A: a1, B: forgeP})
	forgeN := honest2
	forgeN.Height = 1
	add("i5-cross-height-relabel-without-Pruned-REFUSE", Equivocation{Culprit: cul, A: a1, B: forgeN})
	h2a := goldenBlock(2, 2, a2.Hash(), 3, prop, e2(culprit, 1, PhasePrecommit, false))
	forgeE2 := h2a
	forgeE2.Pruned = h2a.bodyHash()
	forgeE2.Height = 1
	add("i5-cross-height-forgery-era2-REFUSE", Equivocation{Culprit: cul, A: a2, B: forgeE2})

	return cases
}

func encodeEquivocationGolden(cases []equivocationGoldenCase) []byte {
	raw, err := encMode.Marshal(cases)
	if err != nil {
		panic(err)
	}
	return raw
}

func readEquivocationGolden(t *testing.T) []equivocationGoldenCase {
	t.Helper()
	raw, err := os.ReadFile(equivocationGoldenPath)
	if err != nil {
		t.Fatalf("golden corpus missing: %v (generate once with -update-equivocation-golden and COMMIT it)", err)
	}
	var cases []equivocationGoldenCase
	if err := cbor.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("golden corpus does not decode: %v", err)
	}
	return cases
}

// TestEquivocationGoldenCorpusVerdicts: every committed fixture's verdict is what
// CheckEquivocation returns today. This is the CD-2 accept-set pin.
func TestEquivocationGoldenCorpusVerdicts(t *testing.T) {
	if *updateEquivocationGolden {
		built := buildEquivocationGoldenCorpus()
		if err := os.MkdirAll(filepath.Dir(equivocationGoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(equivocationGoldenPath, encodeEquivocationGolden(built), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("REGENERATED %s with %d cases — this run is not a verdict; review the diff, commit, and rerun without the flag", equivocationGoldenPath, len(built))
	}
	cases := readEquivocationGolden(t)
	if len(cases) != equivocationGoldenCases {
		t.Fatalf("golden corpus has %d cases, pinned %d — the corpus and the pin must change together", len(cases), equivocationGoldenCases)
	}
	accepts := 0
	for i := range cases {
		c := &cases[i]
		got := equivocationVerdict(CheckEquivocation(&c.Proof))
		if got != c.Verdict {
			t.Errorf("ACCEPT SET MOVED: case %q pinned %s, CheckEquivocation now returns %s", c.Name, c.Verdict, got)
		}
		if c.Verdict == verdictAccept {
			accepts++
		}
		// The name documents the INTENDED verdict; a builder that mislabels a case
		// is caught here, so the corpus cannot pin a verdict nobody meant.
		if strings.HasSuffix(c.Name, "-ACCEPT") != (c.Verdict == verdictAccept) {
			t.Errorf("case %q is named for one verdict and pinned with another (%s)", c.Name, c.Verdict)
		}
		if VerifyEquivocation(&c.Proof) != (c.Verdict == verdictAccept) {
			t.Errorf("case %q: VerifyEquivocation disagrees with CheckEquivocation==nil", c.Name)
		}
	}
	if accepts < 6 {
		t.Errorf("golden corpus has only %d accept cases — an accept-set pin with no accepts is vacuous", accepts)
	}
}

// TestEquivocationGoldenCorpusBytesArePinned: the deterministic builder still
// reproduces the committed bytes. Distinct from the verdict pin: this reddens when
// the ENCODING or the SIGNING of a fixture changed (a Block/Attestation cbor layout
// change, a non-omitempty new field, a key-derivation change), even if every verdict
// still agrees. A carrier field with omitempty on an absent slot must NOT move it.
func TestEquivocationGoldenCorpusBytesArePinned(t *testing.T) {
	if *updateEquivocationGolden {
		t.Skip("regeneration run")
	}
	raw, err := os.ReadFile(equivocationGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	built := encodeEquivocationGolden(buildEquivocationGoldenCorpus())
	if !bytes.Equal(raw, built) {
		t.Fatalf("golden corpus bytes differ from the deterministic builder (file %d B, built %d B) — the fixture encoding or signing changed; if intentional, regenerate with -update-equivocation-golden and review the verdict diff", len(raw), len(built))
	}
}

// TestEquivocationGoldenCorpusHasTeeth: the accept cases depend on the hash path.
// Perturb one entry byte in block A of every accept case (the signatures were over the
// old body hash) and the verdict must flip to refuse. If it does not, the corpus is
// not pinning the signature-over-body-hash property the carrier merge threatens.
func TestEquivocationGoldenCorpusHasTeeth(t *testing.T) {
	cases := readEquivocationGolden(t)
	flipped := 0
	for i := range cases {
		c := cases[i]
		if c.Verdict != verdictAccept {
			continue
		}
		c.Proof.A.Entries[0].FileSize++
		if got := equivocationVerdict(CheckEquivocation(&c.Proof)); got == verdictAccept {
			t.Errorf("TEETH FAILED: case %q still ACCEPTS after its body was perturbed — the verdict does not depend on the body hash", c.Name)
		}
		flipped++
	}
	if flipped == 0 {
		t.Fatal("TEETH FAILED: no accept case to perturb")
	}
}
