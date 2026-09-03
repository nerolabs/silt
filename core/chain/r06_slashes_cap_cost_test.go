package chain

import (
	"crypto/ed25519"
	"runtime"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

// TestSlashesBytesCapWorstCaseCost is the R0.6 G-3 MEASUREMENT harness, not a gate: it
// builds the worst ADMISSIBLE Slashes field — genuine proofs whose evidence blocks each
// carry a full-size (~1.5 MB) BondReg.Answer, the payload pruning exists to shed — packed
// up to SlashesBytesCap, and reports (1) the encoded bytes, (2) the heap delta of holding
// the decoded field resident, and (3) the validateSlashes wall time. The owner ratifies
// the cap VALUE on immutable-#8 grounds from these numbers (certification
// I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03 §7,
// G-3). Skipped under -short; run it by name:
//
//	go test ./core/chain/ -run TestSlashesBytesCapWorstCaseCost -v -count=1
func TestSlashesBytesCapWorstCaseCost(t *testing.T) {
	if testing.Short() {
		t.Skip("G-3 measurement harness; run by name")
	}
	w := newWorld(DefaultConfig())
	g := w.genesis()

	// A reg-laden evidence block: one BondReg carrying a synthetic Answer of the shipped
	// proof size. The bytes are opaque to Hash()/CheckEquivocation (only the signature
	// over the body matters for evidence), so a synthetic payload measures the same cost
	// as a real proof without the VDF.
	const answerBytes = 1536 << 10 // ~1.5 MB, the shipped space-time proof size
	regLaden := func(seed byte, proposer ed25519.PrivateKey) *Block {
		b := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(seed)}}
		b.BondRegs = []BondReg{{Validator: pubOf(w.vals[1]), Answer: make([]byte, answerBytes)}}
		for i := range b.BondRegs[0].Answer {
			b.BondRegs[0].Answer[i] = byte(i*7 + int(seed))
		}
		Sign(b, proposer)
		b.Atts = append(b.Atts, Attest(b, w.vals[0]))
		return b
	}
	a, b := regLaden(1, w.prop), regLaden(2, w.vals[3])
	e := Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *b}
	if err := CheckEquivocation(&e); err != nil {
		t.Fatalf("precondition: the reg-laden double-sign must be provable, got %v", err)
	}
	one := SlashesEncodedSize([]Equivocation{e})
	slashes, size := proofsAroundCap(t, e, false)

	// Resident cost: decode the field from its wire bytes and hold it.
	raw, err := encMode.Marshal(slashes)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var decoded []Equivocation
	if err := cbor.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	resident := int64(after.HeapAlloc) - int64(before.HeapAlloc)

	prev, height := w.c.Head()
	bs := Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(9)}, Slashes: decoded}
	Sign(&bs, w.prop)
	w.attestAll(&bs)
	start := time.Now()
	if err := w.c.validateSlashes(&bs); err != nil {
		t.Fatalf("an at-cap field of genuine reg-laden proofs must validate, got %v", err)
	}
	validate := time.Since(start)

	t.Logf("G-3 measurement: SlashesBytesCap=%d B (%.1f MiB); one reg-laden proof=%d B (%.2f MiB); "+
		"at-cap field=%d proofs, %d B on the wire; resident after decode=%.1f MiB; validateSlashes=%s",
		SlashesBytesCap, float64(SlashesBytesCap)/(1<<20), one, float64(one)/(1<<20),
		len(slashes), size, float64(resident)/(1<<20), validate)
	runtime.KeepAlive(decoded)
}
