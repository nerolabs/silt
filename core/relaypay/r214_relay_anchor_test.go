package relaypay

// R2.14 — the relay-lane prepayment anchor: the WIRE-tier RED-first gates
// (Tester, 2026-09-04). Binding spec:
// silt-agent-memory/researcher/reviews/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
// §5 (k_max "must be DERIVED in code: ⌈MaxChainLength / fee⌉ with a test that the
// wire bound covers it at the shipped fee"), §8 (decode bounds, the F5 shape:
// "len(Anchors) ≤ MaxAnchorsPerSession; each Serial ≤ blindtoken.SerialSize (32);
// each Sig ≤ MaxModulusBits/8 = 1,024 B; Fetcher == 32; Sig == 64. Refuse before
// any map write or modexp"; and "v1 open at a v2 relay: len(Anchors) == 0 ⇒
// errRelayNoAnchor" — the v1 payload must still DECODE). Build shape: advisory
// §1.3 (RelayOpen v2 = Root, S, Funding, Anchors[k], Fetcher, Sig) and §5 step 4.
//
// These gates back T-7 (cheap-before-RSA): a decode refusal is the cheapest
// refusal there is, and it is where an oversized anchor list must die.

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// shippedFee is cmd/silt's default --fee (50,000 credits), the face of one anchor
// (cert §2.1: face = Fee(), an identity with the burn). Pinned as a literal so the
// derivation test is independent of core/credit.
const shippedFee = 50_000

// The Tester's reading of the v2 layout (see the stub block).
type anchorForTest struct {
	Serial []byte `cbor:"1,keyasint"`
	Sig    []byte `cbor:"2,keyasint"`
}

type relayOpenV2ForTest struct {
	Root    []byte          `cbor:"1,keyasint"`
	S       int             `cbor:"2,keyasint"`
	Funding int             `cbor:"3,keyasint"`
	Anchors []anchorForTest `cbor:"4,keyasint"`
	Fetcher []byte          `cbor:"5,keyasint"`
	Sig     []byte          `cbor:"6,keyasint"`
}

func wellFormedV2(k int) relayOpenV2ForTest {
	o := relayOpenV2ForTest{
		Root:    make([]byte, 32),
		S:       8,
		Funding: 0,
		Fetcher: make([]byte, 32),
		Sig:     make([]byte, 64),
	}
	for i := 0; i < k; i++ {
		o.Anchors = append(o.Anchors, anchorForTest{Serial: make([]byte, 32), Sig: make([]byte, 256)})
	}
	return o
}

func mustCBOR(t *testing.T, v any) []byte {
	t.Helper()
	b, err := cbor.Marshal(v)
	if err != nil {
		t.Fatalf("cbor.Marshal: %v", err)
	}
	return b
}

// TestRelayMaxAnchorsPerSessionCoversTheSessionCeiling pins the derivation of
// k_max (cert §5): at the shipped fee, MaxAnchorsPerSession anchors fund the
// longest chain a relay accepts (S_max = MaxChainLength), and one fewer does not —
// the bound is the ceiling, not a guess with slack. A lower fee raises k_max
// (granularity/liveness, never soundness).
func TestRelayMaxAnchorsPerSessionCoversTheSessionCeiling(t *testing.T) {
	const sessionCredits = MaxChainLength * RelayIncrementCredit // 50,000 since the 2026-09-06 re-price (262,144 at 4 KiB)
	if got := MaxAnchorsPerSession; got != 1 {
		t.Fatalf("MaxAnchorsPerSession = %d, want 1 = ⌈%d / %d⌉ (the derived DoS/decode bound, cert §5; one anchor funds the whole session since the re-price, T-RELAY-GRAN)", got, sessionCredits, shippedFee)
	}
	// T-RELAY-GRAN pin (G-R212-2 cert §4.1): the session ceiling is EXACTLY what a
	// full-length chain authorizes, and S_max is exactly one face — nothing a fetcher
	// pays for is unforwardable, and the derivation is face-neutral.
	if MaxSessionBytes != MaxChainLength*RelayIncrementBytes || sessionCredits != ShippedAnchorFace {
		t.Fatalf("T-RELAY-GRAN broken: MaxSessionBytes %d vs S_max·B %d; S_max·credit %d vs face %d", MaxSessionBytes, MaxChainLength*RelayIncrementBytes, sessionCredits, ShippedAnchorFace)
	}
	// F-1 (blind PE, 2026-09-06): S_max and the session ceiling now DERIVE from the face, so
	// the face's own drift detector has to be an INDEPENDENT literal — the derivation cannot
	// catch the face moving. The three certified values, pinned as numbers (cert §8).
	if ShippedAnchorFace != 50_000 || MaxChainLength != 50_000 || MaxSessionBytes != 26_214_400_000 {
		t.Fatalf("face %d / S_max %d / MaxSessionBytes %d, want 50,000 / 50,000 / 26,214,400,000 — a moved face re-derives k_max, S_max, the session ceiling, the 64 GiB pin and the guard bound together; re-certify before re-pinning", ShippedAnchorFace, MaxChainLength, MaxSessionBytes)
	}
	if RelayIncrementBytes != 524_288 {
		t.Fatalf("RelayIncrementBytes = %d, want 524,288 — the owner-ratified 2026-09-06 re-price; a different value re-opens G-R212-2 (the 64 GiB pin read on the summed prices, Don't #7 below, T-AR above)", RelayIncrementBytes)
	}
	if MaxAnchorsPerSession*shippedFee < sessionCredits {
		t.Fatalf("%d anchors × %d face = %d < S_max session value %d — the wire bound does not cover a full session at the shipped fee",
			MaxAnchorsPerSession, shippedFee, MaxAnchorsPerSession*shippedFee, sessionCredits)
	}
	if (MaxAnchorsPerSession-1)*shippedFee >= sessionCredits {
		t.Fatalf("%d anchors already cover S_max — MaxAnchorsPerSession is not the ceiling, it carries slack an attacker can fill with garbage anchors",
			MaxAnchorsPerSession-1)
	}
}

// TestRelayOpenDecodeBoundsRefuseOversizedAnchors is the cert §8 decode-bound
// gate (the F5 amplifier shape, demand.go:441-459): every oversized field is a
// DECODE error, before any map write or modexp. A v1 payload (no v2 fields) must
// still decode — skew fails safe at the node (len(Anchors)==0 ⇒ refused), not at
// the codec. RED on main: UnmarshalRelayOpen has no bounds and ignores the v2 keys.
func TestRelayOpenDecodeBoundsRefuseOversizedAnchors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*relayOpenV2ForTest)
	}{
		{"k = MaxAnchorsPerSession+1 exceeds the bound", func(o *relayOpenV2ForTest) {
			for len(o.Anchors) < MaxAnchorsPerSession+1 {
				o.Anchors = append(o.Anchors, anchorForTest{Serial: make([]byte, 32), Sig: make([]byte, 256)})
			}
		}},
		{"serial 33 B > blindtoken.SerialSize", func(o *relayOpenV2ForTest) { o.Anchors[0].Serial = make([]byte, 33) }},
		{"anchor sig 1025 B > MaxModulusBits/8", func(o *relayOpenV2ForTest) { o.Anchors[0].Sig = make([]byte, 1025) }},
		{"Fetcher 31 B != 32", func(o *relayOpenV2ForTest) { o.Fetcher = make([]byte, 31) }},
		{"Fetcher 33 B != 32", func(o *relayOpenV2ForTest) { o.Fetcher = make([]byte, 33) }},
		{"Sig 63 B != 64", func(o *relayOpenV2ForTest) { o.Sig = make([]byte, 63) }},
		{"Sig 65 B != 64", func(o *relayOpenV2ForTest) { o.Sig = make([]byte, 65) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := wellFormedV2(MaxAnchorsPerSession)
			tc.mut(&o)
			if _, err := UnmarshalRelayOpen(mustCBOR(t, o)); err == nil {
				t.Fatalf("UnmarshalRelayOpen ACCEPTED a RelayOpen with %s — the decode bound is missing; an attacker-sized field reaches the map/modexp path", tc.name)
			}
		})
	}

	t.Run("well-formed v2 decodes", func(t *testing.T) {
		if _, err := UnmarshalRelayOpen(mustCBOR(t, wellFormedV2(MaxAnchorsPerSession))); err != nil {
			t.Fatalf("a well-formed v2 RelayOpen (k = k_max, 32-B serials, 256-B sigs, 32-B Fetcher, 64-B Sig) failed to decode: %v", err)
		}
	})
	t.Run("v1 payload still decodes (skew fails safe at the node, not the codec)", func(t *testing.T) {
		v1 := RelayOpen{Root: make([]byte, 32), S: 8, Funding: 0}
		blob, err := v1.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := UnmarshalRelayOpen(blob); err != nil {
			t.Fatalf("a v1 RelayOpen (no Anchors/Fetcher/Sig) must decode and be refused at OpenRelaySession with a named reason, not fail in the codec: %v", err)
		}
	})
}
