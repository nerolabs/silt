package pipeline_test

// RED-TEAM R-SUBFRAME-SIZE-ORACLE — confirmed pass at origin/main @ 00082b8, 2026-09-11.
// Source: silt-agent-memory/red-team/reviews/RED-TEAM-R-SUBFRAME-SIZE-ORACLE-00082b8-2026-09-11.md
// PE scope ruling (RT-SFO-6): silt-agent-memory/principal-engineer/reviews/ruling-rt-sfo-6-msgchallenge-scope-2026-09-11.md
//
// Canon: a confirmed red-team break is handed to the Tester as a PERMANENT regression gate.
// This file carries RT-SFO-1 (the keyless encryption-mode oracle), RT-SFO-2 (the total
// chunk-size salt collapse) and RT-SFO-5 (Entry.FileSize is the exact byte count).
// RT-SFO-3 and RT-SFO-4 are in rt_sfo_stripe_test.go; RT-SFO-6 is in core/node.
//
// ────────────────────────────────────────────────────────────────────────────────────────
// THE IDIOM — A DEFECT PIN WITH TESTABLE TEETH, and why it is this and not a plain assert.
// ────────────────────────────────────────────────────────────────────────────────────────
//
// RT-SFO-1 and RT-SFO-5 are LIVE DEFECTS on main. Their disposition is an OPEN OWNER CALL:
// privacy is a Part-0 corner (docs/TENETS.md Part 0, M0) and the owner has not ruled. RT-SFO-5
// is additionally research-gated as a published-claim change. So a gate here cannot assert
// that the property holds — it does not — and a failing test cannot merge.
//
// Each live defect therefore gets three parts, not one:
//
//  1. A PIN. The measured current behaviour, asserted exactly, with the defect named in the
//     failure text and the open owner call cited. GREEN today. It goes RED the moment the
//     behaviour moves in ANY direction — a fix, a partial fix, or silent drift out of an
//     unrelated encoding change. The RED is the point: the defect cannot be closed
//     accidentally, and it cannot quietly stop being true.
//
//  2. TEETH. A separate test that feeds the pin's predicate the POST-FIX value and asserts
//     the predicate returns a non-empty message. This is the demonstrated RED that simplicity
//     rule 7 requires ("a green gate with no demonstrated red is decoration") WITHOUT merging
//     a red test. A pin whose teeth are untested is exactly the decoration rule 7 forbids: it
//     claims it will redden, and nothing has ever checked that it can.
//
//  3. A MECHANISM arm inside the pin, so a change that moves the NUMBER without removing the
//     CAUSE still reddens, and a change that removes the cause cannot leave the pin green by
//     coincidence. Pinning a symptom alone is how a pin decays into folklore.
//
// WHY THIS SHAPE AND NOT A BARE "pin the measured number". A bare pin records a symptom. It
// can pass for the wrong reason if the mechanism moves underneath it, and it has never been
// shown capable of failing. Parts 2 and 3 are what make it evidence instead of a comment.
//
// PRECEDENT IN THIS TREE, which is why this is the house idiom and not an invention
// (simplicity rule 1 — name the settled corner you are buying):
//   - core/chain/issuerkey_rollout_gate_test.go: gateFStampPin is a factored predicate whose
//     teeth are asserted by TestGateF_StampRaiseIsAHardFailure. That factoring exists because
//     the clause used to be a t.Logf — it claimed "this cannot happen silently" and could not
//     actually fail. Same failure mode, same remedy.
//   - core/chain/readset_v5_drift_test.go: the inert-digest-root placeholders confirm the
//     PRECONDITION that justifies the exclusion before standing down, so the placeholder is
//     honest rather than merely quiet.
//
// Every pin message below says what to re-read and what to re-confirm, in the imperative,
// because whoever reddens it will not have read this comment.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// ──────────────────────────────────────────────────────────────────────────────
// Fixtures.
//
// NON-UNIFORM by construction. A repeated-byte payload is exactly the population in which a
// wrong reducer looks right: a length measurement over bytes.Repeat cannot distinguish "we
// measured the frame" from "we measured a constant", and convergent encryption over a uniform
// payload can collapse distinct chunks into one secret. rtPayload is deterministic, so every
// number in these gates is reproducible, and content-varying, so nothing downstream can
// coincidentally dedup it.
//
// Every gate's subject is PRODUCED by pipeline.Stage. No gate in this file builds a manifest,
// a frame or a stripe by hand — "the publisher stopped producing this state" is precisely what
// a fix looks like, and a hand-built fixture would construct the very state whose production
// absence is the fix.
// ──────────────────────────────────────────────────────────────────────────────

func rtPayload(n int, seed uint64) []byte {
	out := make([]byte, n)
	var sb, bb [8]byte
	binary.BigEndian.PutUint64(sb[:], seed)
	h := sha256.New()
	for off, blk := 0, 0; off < n; off, blk = off+32, blk+1 {
		h.Reset()
		h.Write(sb[:])
		binary.BigEndian.PutUint64(bb[:], uint64(blk))
		h.Write(bb[:])
		copy(out[off:], h.Sum(nil))
	}
	return out
}

// rtDetRand is an injected, deterministic randomness source for private mode. core/pipeline
// refuses to default one ("private mode requires an injected randomness source"), which is
// what lets these gates assert exact byte counts rather than ranges.
type rtDetRand struct {
	n   uint64
	buf [32]byte
}

func (d *rtDetRand) Read(b []byte) (int, error) {
	for i := range b {
		if d.n%32 == 0 {
			d.buf = sha256.Sum256(binary.BigEndian.AppendUint64(nil, d.n))
		}
		b[i] = d.buf[d.n%32]
		d.n++
	}
	return len(b), nil
}

type rtObject struct {
	handle interface{ LayoutKey() [32]byte }
	entry  ports.Entry
	store  *memstore.Store
}

func rtStage(t *testing.T, size int, mode crypto.Mode, chunkSize, frameBytes int, ez erasure.Params) rtObject {
	t.Helper()
	st := memstore.New()
	h, entry, err := pipeline.Stage(context.Background(), st, bytes.NewReader(rtPayload(size, 1)),
		pipeline.Options{
			ChunkSize: chunkSize, Mode: mode, Erasure: ez,
			ManifestFrameBytes: frameBytes, Rand: &rtDetRand{},
		})
	if err != nil {
		t.Fatalf("stage L=%d mode=%q chunkSize=%d frameBytes=%d: %v", size, mode, chunkSize, frameBytes, err)
	}
	return rtObject{handle: h, entry: entry, store: st}
}

// rtManifestFrameBytes is the number a KEYLESS observer measures: the total bytes of the
// object's manifest chunks, as served by the unauthenticated MsgFetchChunk surface
// (core/node/node.go MsgFetchChunk). It is read back out of the store rather than computed
// from ManifestFrameSize, so the gate never takes its guard condition from its own subject.
func rtManifestFrameBytes(t *testing.T, size int, mode crypto.Mode, chunkSize, frameBytes int, ez erasure.Params) int {
	t.Helper()
	o := rtStage(t, size, mode, chunkSize, frameBytes, ez)
	total := 0
	for _, id := range o.entry.ManifestChunks {
		c, err := o.store.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("manifest chunk %s: %v", id, err)
		}
		total += len(c.Data)
	}
	return total
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-1 (HIGH, NEW) — a keyless encryption-mode oracle. CLOSED 2026-09-11.
//
// THE DEFECT. manifest.secretsPart.Mode is sealed under the CONTENT key
// (core/manifest/sealed.go). A caretaker holding only a care link is not supposed to read it;
// that is the entire purpose of the two-layer seal (core/link/link.go). But
// pipeline.ManifestFrameSize frames a single-chunk sealed manifest at len(blob)+chunk.HeaderSize,
// and the blob is canonical CBOR, so its length is a deterministic function of what is inside
// it. Convergent mode carried 34 bytes of ChunkSecrets per data chunk where private mode
// carried one 34-byte FileKey, so the frame length published — to anyone who can dial one node
// and fetch one chunk — a function of a field the seal exists to hide. By cmd/silt/main.go's
// own framing (private is the default, convergent carries a loud warning) that field is the
// publisher's own "is this secret" classification of its content.
//
// THE CLOSE. manifest.secretsPlainLen pads the secrets plaintext to a length that is a
// function of the DATA-SHARD COUNT ALONE before it is sealed — length-hiding authenticated
// encryption (Paterson–Ristenpart–Shrimpton, ASIACRYPT 2011), as deployed in TLS 1.3 record
// padding, RFC 8446 §5.4. Owner-ratified on the research certification
// "privacy-property-measured-part0-corner-entry-filesize-and-mode-oracle", 2026-09-11,
// recorded as D-MODE-ORACLE-2026-09-11. The defect PIN that shipped in #817 is retired here
// and replaced by the straight assertion; its teeth are kept and re-pointed.
// ══════════════════════════════════════════════════════════════════════════════

// rtSFO1Sizes carries both measurements: `framed`, the post-fix length both modes now share,
// and `wasConv`/`wasPriv`, byte-for-byte what the red-team pass recorded at 00082b8. The
// pre-fix pair is kept because a gate that no longer remembers the defect cannot say how far
// it moved, and because TestRT_SFO_1_GateRedensWhenTheModesSeparate feeds it back in.
var rtSFO1Sizes = []struct {
	L                int
	framed           int
	wasConv, wasPriv int
}{
	{1, 353, 345, 341},
	{100, 354, 347, 343},
	{1024, 355, 349, 345},
	{65536, 357, 353, 349},
	{262136, 357, 353, 349},  // the last single-frame object at the 256 KiB default
	{262137, 425, 421, 383},  // the first two-frame object: the delta used to widen per data chunk
	{600000, 493, 489, 417},  //
	{2097152, 902, 898, 621}, // 9 chunks: 277 bytes of separation, before. The pad costs 281.
}

// rtSFO1ModeHidden is the gate predicate. It returns "" while the two modes are
// INDISTINGUISHABLE by manifest frame length — the property the fix establishes — and a
// non-empty instruction the moment they separate again. Factored out so its teeth are
// themselves testable (TestRT_SFO_1_GateRedensWhenTheModesSeparate), the gateFStampPin
// pattern inherited from the retired pin.
func rtSFO1ModeHidden(L, conv, priv int) string {
	if conv == priv {
		return ""
	}
	return fmt.Sprintf(
		"RT-SFO-1 IS BACK — THE SEALED-MANIFEST FRAME LENGTH PUBLISHES THE ENCRYPTION MODE AGAIN.\n"+
			"  At FileSize=%d the manifest frames at convergent=%d private=%d (delta %d). EQUAL is the\n"+
			"  property. Unequal is a keyless encryption-mode oracle: a peer with no keys, no care link, no\n"+
			"  bond and no token reads Entry.ManifestChunks off the unauthenticated MsgGetChain\n"+
			"  (core/node/chainrole.go), fetches that chunk over the unauthenticated MsgFetchChunk\n"+
			"  (core/node/node.go), and recovers the publisher's own secret/not-secret classification of the\n"+
			"  root — docs/threat-catalog.md F8.\n"+
			"  IT WAS 345 vs 341 AT FileSize=1 ON 00082b8. That is the state this gate exists to prevent.\n"+
			"  DO THIS: the close is manifest.secretsPlainLen, which pads secretsPart to a length that is a\n"+
			"  function of the DATA-SHARD COUNT ALONE. Find what made that length mode-dependent again — a\n"+
			"  new secretsPart field outside the pad, a wider mode name, or a target that reads m.Mode,\n"+
			"  m.FileKey or m.ChunkSecrets. Do NOT close it by re-padding the manifest FRAME instead: once a\n"+
			"  convergent manifest outgrows one frame, len(Entry.ManifestChunks) separates the modes\n"+
			"  on-chain with no fetch at all. The inner box is the fix.",
		L, conv, priv, conv-priv)
}

// TestRT_SFO_1_ModeIsNotRecoverableFromManifestFrameLength is the gate. It replaces the
// PINNED_DEFECT that shipped in #817.
//
// SEEN RED FIRST on the pre-fix tree at f826c72, which is the certification's own §9(a)
// condition and simplicity rule 7 ("a green gate with no demonstrated red is decoration"):
//
//	RT-SFO-1 IS BACK — ... At FileSize=1 the manifest frames at convergent=345 private=341 (delta 4).
func TestRT_SFO_1_ModeIsNotRecoverableFromManifestFrameLength(t *testing.T) {
	cs := pipeline.DefaultChunkSize

	// (a) THE PROPERTY, at the eight sizes the red-team measured the oracle at.
	for _, tc := range rtSFO1Sizes {
		conv := rtManifestFrameBytes(t, tc.L, crypto.Convergent, cs, 0, erasure.Params{})
		priv := rtManifestFrameBytes(t, tc.L, crypto.Private, cs, 0, erasure.Params{})
		if msg := rtSFO1ModeHidden(tc.L, conv, priv); msg != "" {
			t.Fatal(msg)
		}
		if conv != tc.framed {
			t.Fatalf("RT-SFO-1 DRIFT — a %d-byte object now frames at %d bytes in both modes; this gate pins %d.\n"+
				"  The two modes still agree, so this is NOT the oracle returning — it is the seal re-encoding.\n"+
				"  Find what moved: manifest.Layout, manifest.secretsPart, manifest.secretsPlainLen,\n"+
				"  crypto.SealBox's overhead, or pipeline.ManifestFrameSize. Confirm the change was intended,\n"+
				"  confirm it moves core/genesis TestGenesisBlockHashIsPinned deliberately, and update these\n"+
				"  numbers here. A frame length that drifts silently is how a padding scheme rots.",
				tc.L, conv, tc.framed)
		}
	}

	// (b) THE PROPERTY, densely. Eight points can be equalised by coincidence; 429 cannot —
	// and 429 is the population the red-team classified 429/429 correctly on, so this arm is
	// the direct refutation of that measurement.
	for L := 1; L <= 3000; L += 7 {
		conv := rtManifestFrameBytes(t, L, crypto.Convergent, cs, 0, erasure.Params{})
		priv := rtManifestFrameBytes(t, L, crypto.Private, cs, 0, erasure.Params{})
		if msg := rtSFO1ModeHidden(L, conv, priv); msg != "" {
			t.Fatal(msg)
		}
	}

	// (c) THE MECHANISM, asserted against the store, because equality at the FRAME can be
	// bought the wrong way. The certification is explicit that re-padding the manifest frame
	// lifts this only PARTIALLY: once a convergent manifest outgrows one frame,
	// len(Entry.ManifestChunks) separates the modes on-chain with no fetch at all. So this arm
	// asserts (i) the SEALED BLOB itself is mode-independent — the inner box is what was
	// fixed, which is also what closes it for the care-link holder — and (ii) the frame is
	// still the blob's true length plus a header, i.e. nothing above leans on frame padding.
	blob := func(mode crypto.Mode, L int) []byte {
		o := rtStage(t, L, mode, cs, 0, erasure.Params{})
		b, err := pipeline.LoadBlob(context.Background(), o.store, o.entry)
		if err != nil {
			t.Fatalf("LoadBlob (%s, L=%d): %v", mode, L, err)
		}
		return b
	}
	for _, L := range []int{1, 1024, 262136, 262137, 600000, 2097152} {
		bc, bp := blob(crypto.Convergent, L), blob(crypto.Private, L)
		if len(bc) != len(bp) {
			t.Fatalf("RT-SFO-1 IS BACK AT THE ROOT — the SEALED BLOB for a %d-byte object is %d bytes convergent "+
				"and %d bytes private. The frames may still agree, but the BLOB length is the deeper oracle: the "+
				"care-link holder measures it directly (manifest.OpenLayout gives it Layout.Box), and once the "+
				"manifest spans more than one frame the chunk COUNT publishes it on-chain. Fix "+
				"manifest.secretsPlainLen, not pipeline.ManifestFrameSize.", L, len(bc), len(bp))
		}
	}
	if got, want := rtManifestFrameBytes(t, 1024, crypto.Convergent, cs, 0, erasure.Params{}),
		len(blob(crypto.Convergent, 1024))+chunk.HeaderSize; got != want {
		t.Fatalf("RT-SFO-1 GATE IS MEASURING THE WRONG THING — the manifest frame is %d bytes where the sealed "+
			"blob plus a %d-byte header is %d. pipeline.ManifestFrameSize has stopped framing at true length, so "+
			"arms (a) and (b) may be green because the FRAME is padded rather than because the BOX is. That is "+
			"the partial fix the certification refuses; re-read its §9 before trusting this gate.",
			got, chunk.HeaderSize, want)
	}
}

// TestRT_SFO_1_ModeIsNotRecoverableAtAnyErasureGeometry is the geometry arm, inverted by the
// fix. It used to pin that the convergent and private length SETS stay DISJOINT over a
// seven-point (k,n) grid — the property that made the oracle survive an attacker who did not
// know the publisher's -erasure setting. The pad target is the data-shard count, which is the
// same in both modes at a given (L, k, n), so the modes now collide at every point of that
// grid. Assert the collision pointwise, which is strictly stronger than set equality: two
// length SETS can coincide while no individual geometry matches.
func TestRT_SFO_1_ModeIsNotRecoverableAtAnyErasureGeometry(t *testing.T) {
	cs := pipeline.DefaultChunkSize
	grid := []erasure.Params{{K: 2, N: 4}, {K: 3, N: 5}, {K: 4, N: 8}, {K: 6, N: 10}, {K: 10, N: 16}, {K: 12, N: 20}, {K: 16, N: 24}}
	for _, L := range []int{1024, 40000} {
		for _, g := range grid {
			conv := rtManifestFrameBytes(t, L, crypto.Convergent, cs, 0, g)
			priv := rtManifestFrameBytes(t, L, crypto.Private, cs, 0, g)
			if msg := rtSFO1ModeHidden(L, conv, priv); msg != "" {
				t.Fatalf("at k=%d,n=%d: %s", g.K, g.N, msg)
			}
		}
	}
}

// TestRT_SFO_1_GateRedensWhenTheModesSeparate is the TEETH, carried over from the retired
// pin's teeth and re-pointed at the new predicate. It publishes nothing: it feeds
// rtSFO1ModeHidden the PRE-FIX MEASUREMENTS and asserts the gate speaks. This is the permanent
// record of the RED, and it is the reason the gate above is evidence rather than decoration —
// nothing else ever checks that it CAN fail.
func TestRT_SFO_1_GateRedensWhenTheModesSeparate(t *testing.T) {
	for _, tc := range rtSFO1Sizes {
		if msg := rtSFO1ModeHidden(tc.L, tc.wasConv, tc.wasPriv); msg == "" {
			t.Fatalf("rtSFO1ModeHidden stayed silent on the ORIGINAL RT-SFO-1 measurement at FileSize=%d "+
				"(convergent=%d, private=%d, as recorded at 00082b8) — the gate cannot detect the defect it "+
				"exists to prevent, so it is decoration (simplicity rule 7)", tc.L, tc.wasConv, tc.wasPriv)
		}
	}
	// A ONE-BYTE separation. A classifier needs one bit, so a gate with any slack does not
	// hold the property; this is the arm that refuses a "close enough" reformulation.
	if msg := rtSFO1ModeHidden(1, 346, 345); msg == "" {
		t.Fatal("rtSFO1ModeHidden stayed silent on a ONE-BYTE separation — one byte is more than the one bit " +
			"the mode oracle needs, so a gate that tolerates it does not hold the property")
	}
	// The post-fix state: must stay silent, or the gate is a permanent false alarm.
	if msg := rtSFO1ModeHidden(1, rtSFO1Sizes[0].framed, rtSFO1Sizes[0].framed); msg != "" {
		t.Fatalf("rtSFO1ModeHidden fired on the state it exists to accept: %s", msg)
	}
}

// TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes is the framing ablation, kept from
// #817 and still load-bearing for a reason that OUTLIVED the defect. It pins that padded
// manifest framing ALSO collapses the modes — which is exactly why it must stay visible next
// to the real fix. Someone reading the gate above could conclude that re-padding the frame is
// an equivalent remedy. It is not: it lifts the oracle only while both modes fit one frame,
// because len(Entry.ManifestChunks) then separates them on-chain with no fetch at all, and it
// reinstates R-MANIFEST-PADDING's 3.9x store growth. This test records the weaker remedy as
// weaker, in the same file as the stronger one.
func TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes(t *testing.T) {
	cs := pipeline.DefaultChunkSize
	for _, L := range []int{1, 1024, 262135} {
		conv := rtManifestFrameBytes(t, L, crypto.Convergent, cs, cs, erasure.Params{})
		priv := rtManifestFrameBytes(t, L, crypto.Private, cs, cs, erasure.Params{})
		if conv != priv {
			t.Fatalf("ABLATION FAILED TO ABLATE — under padded framing (ManifestFrameBytes=%d) a %d-byte object "+
				"still frames convergent=%d private=%d. Re-read RT-SFO-1's fix analysis before proceeding.",
				cs, L, conv, priv)
		}
		if conv != cs {
			t.Fatalf("ABLATION FAILED TO ABLATE — padded framing produced %d bytes, not the chunk size %d. "+
				"Options.ManifestFrameBytes no longer pins the frame, so this ablation is not exercising the pre-4' "+
				"behaviour it claims to and its green result means nothing (the silt-ablation-noop-guard scar).",
				conv, cs)
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-2 (MED-HIGH, NEW) — the chunk-size salt collapse is TOTAL, not three-point.
//
// pipeline.DataFrameSize returns objectBytes+chunk.HeaderSize whenever that is strictly less
// than chunkSize, so a sub-frame object's framed bytes — and therefore its ciphertext, chunk
// IDs and root — do not mention chunkSize AT ANY VALUE above L+7.
//
// WHY THIS GATE EXISTS RATHER THAN THE ONE ALREADY IN THE TREE. The existing gate,
// TestConvergentDedupNowSpansTheChunkSize (core/pipeline/short_final_stripe_test.go), samples
// THREE chunk sizes. A three-point sample cannot distinguish "three plausible values
// collapsed" from "the entire legal range collapsed", and the threat-catalog text it backs
// asserts the former. The difference is load-bearing for the FIX decision: "re-salt with a
// random chunk size" is a real mitigation under the first reading and a MEASURED NO-OP under
// the second. This gate asserts the RANGE PROPERTY and its exact COMPLEMENT, so the shape can
// never be re-derived as "three" by a future reader.
//
// This gate does not replace the three-point one and does not touch it; it is the range
// version of the same claim.
// ══════════════════════════════════════════════════════════════════════════════

func rtRootAt(t *testing.T, payload []byte, chunkSize int) ports.Hash {
	t.Helper()
	st := memstore.New()
	h, _, err := pipeline.Stage(context.Background(), st, bytes.NewReader(payload),
		pipeline.Options{ChunkSize: chunkSize, Mode: crypto.Convergent})
	if err != nil {
		t.Fatalf("stage at chunkSize=%d: %v", chunkSize, err)
	}
	return h.Root
}

func TestRT_SFO_2_SubFrameRootIsChunkSizeIndependentAcrossTheLegalRange(t *testing.T) {
	const L = 111
	payload := rtPayload(L, 3)

	// (a) THE COLLAPSE, over the whole legal range by log-spaced probe. chunk.MinChunkSize is
	// the floor and manifest.MaxChunkSize the ceiling, both read from source rather than
	// written as literals, so a change to either widens this sweep automatically.
	predicted := rtRootAt(t, payload, manifest.MaxChunkSize)
	probes := []int{L + 8, L + 9, 128, 256, 1024, 4096, 64 << 10, 256 << 10, 1 << 20, 4 << 20, 64 << 20, manifest.MaxChunkSize}
	for _, cs := range probes {
		if got := rtRootAt(t, payload, cs); got != predicted {
			t.Fatalf("RT-SFO-2 PIN IS RED — a %d-byte object published at chunkSize=%d now yields root %s, "+
				"not %s. The salt is BACK: the chunk size is once again an input to a sub-frame object's root.\n"+
				"  This was measured TOTAL at 00082b8 — 3,978 of 4,088 chunk sizes in [9,4096] collapse to one root,\n"+
				"  and the only salting values are chunkSize <= L+7 (arm (b)).\n"+
				"  DO THIS: confirm the re-salt is DELIBERATE. Inventing a salt here without a certification is the\n"+
				"  unreviewed novelty B8 forbids, and docs/threat-catalog.md F3's text depends on which reading is\n"+
				"  true. Route to the Researcher before recording it as a mitigation.",
				L, cs, got, predicted)
		}
	}

	// (b) THE COMPLEMENT, exhaustively. The surviving salt is exactly [chunk.MinChunkSize, L+7]
	// and nothing else. Asserting the complement is what converts "we sampled and saw a
	// collapse" into a closed statement about the range — and a closed complement is a rule a
	// future reader cannot re-derive as "three points". Exhaustive over [9, 4096]: 4,088
	// stages, measured at 0.68 s.
	var salted, collapsed []int
	for cs := chunk.MinChunkSize; cs <= 4096; cs++ {
		if rtRootAt(t, payload, cs) == predicted {
			collapsed = append(collapsed, cs)
		} else {
			salted = append(salted, cs)
		}
	}
	if len(salted) == 0 {
		t.Fatal("RT-SFO-2: no chunk size in [9,4096] salts at all — the complement arm is vacuous and cannot " +
			"detect a change in the boundary. Re-derive the boundary from pipeline.DataFrameSize before trusting arm (a).")
	}
	wantBoundary := L + chunk.HeaderSize - 1 // 118: the largest chunkSize that still salts
	if salted[0] != chunk.MinChunkSize || salted[len(salted)-1] != wantBoundary || len(salted) != wantBoundary-chunk.MinChunkSize+1 {
		t.Fatalf("RT-SFO-2 BOUNDARY MOVED — the salting chunk sizes for a %d-byte object are %d values spanning "+
			"[%d,%d]; the pinned complement is the contiguous range [%d,%d] (%d values), i.e. exactly "+
			"chunkSize <= L+chunk.HeaderSize-1, which is where pipeline.DataFrameSize stops returning L+%d. "+
			"A moved boundary changes how many publishers accidentally salt; establish why and update this.",
			L, len(salted), salted[0], salted[len(salted)-1], chunk.MinChunkSize, wantBoundary,
			wantBoundary-chunk.MinChunkSize+1, chunk.HeaderSize)
	}
	if len(collapsed) != 4096-wantBoundary {
		t.Fatalf("RT-SFO-2: %d of the chunk sizes in [%d,4096] collapse to the attacker-predicted root; the "+
			"complement of the salting range is %d values. The two arms disagree, so one of them is measuring "+
			"something other than the root.", len(collapsed), chunk.MinChunkSize, 4096-wantBoundary)
	}

	// (c) THE SHAPE, stated as a fraction of the legal range so it cannot be re-read as
	// "three values". NOTE FOR THE RECORD — the red-team report says 134,217,609 of
	// 134,217,720 collapse; the measured count is 134,217,610. The legal range is
	// [9, 134217728] = 134,217,720 values, of which exactly [9,118] = 110 salt, leaving
	// 134,217,610. The report is off by one. This gate asserts the DERIVED count, so it
	// cannot inherit that error.
	legal := manifest.MaxChunkSize - chunk.MinChunkSize + 1
	saltingTotal := wantBoundary - chunk.MinChunkSize + 1
	if collapsedTotal := legal - saltingTotal; collapsedTotal != 134_217_610 {
		t.Fatalf("RT-SFO-2: the legal chunk-size range is [%d,%d] (%d values) of which %d salt, so %d collapse — "+
			"this gate pins 134,217,610 (99.99992%%). If chunk.MinChunkSize or manifest.MaxChunkSize moved, the "+
			"privacy statement in docs/threat-catalog.md F3 moves with it; update both together.",
			chunk.MinChunkSize, manifest.MaxChunkSize, legal, saltingTotal, collapsedTotal)
	}

	// (d) THE ESCAPE COSTS WHAT THE CLI TELLS PUBLISHERS TO AVOID. cmd/silt/numeraire.go's
	// bountyPriceWarning tells a publisher to RAISE -chunk-size for a non-zero repair bounty,
	// i.e. to move INTO the collapsed regime. So the only surviving salt is the setting the
	// economics forbid. Assert the two really do point opposite ways, at the boundary, by
	// measurement rather than by citing the warning's text — a claim about a warning is itself
	// a claim, and a warning's wording decays.
	if rtRootAt(t, payload, wantBoundary) == predicted {
		t.Fatalf("RT-SFO-2 arm (d): chunkSize=%d (the largest salting value) produced the attacker-predicted "+
			"root, so the salting range and the collapse range overlap. Arms (a) and (b) cannot both be right.",
			wantBoundary)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-5 (HIGH, PRE-EXISTING) — ports.Entry.FileSize is the exact byte count.
//
// It is hash-covered (inside chain.Block.Entries), replicated to every node, permanent, and
// never validated by consensus. So the exact plaintext length of every object is global,
// keyless and unverified — for private-mode objects exactly as for convergent ones.
//
// WHAT IS PINNED HERE AND WHAT IS DELIBERATELY NOT. This gate pins the MEASUREMENT: the
// publisher writes the exact count. It does NOT assert anything about what silt CLAIMS.
// The red-team measured two published claims false — docs/threat-catalog.md F3(a)'s "before,
// padding blurred it to 'somewhere in one chunk'" and docs/math/02-convergent-encryption.md's
// "no confirmation surface" for private mode. Correcting what silt claims about privacy is a
// published-claim change: research-gated, owner-ratified. A test asserting doc text would be
// this seat deciding a question that is not its to decide, and would ALSO decay the moment the
// doc was reworded for any other reason (the silt-a-claim-about-a-gate-is-itself-a-claim scar).
// The measurement is the durable half. The doc is the owner's.
//
// This is also the finding that makes "re-pad the sub-frame" a no-op, so it must stay visible
// next to TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes.
// ══════════════════════════════════════════════════════════════════════════════

// rtSFO5Pin returns "" while Entry.FileSize is still the exact count, and the instruction once
// it is blinded. Teeth: TestRT_SFO_5_PinRedensWhenFileSizeIsBlinded.
func rtSFO5Pin(wantExact int64, got int64, mode crypto.Mode) string {
	if got == wantExact {
		return ""
	}
	return fmt.Sprintf(
		"RT-SFO-5 PIN IS RED — ports.Entry.FileSize IS NO LONGER THE EXACT BYTE COUNT.\n"+
			"  A %d-byte object published in %q mode now records FileSize=%d.\n"+
			"  At 00082b8 this field was the exact, hash-covered, consensus-unvalidated plaintext length of every\n"+
			"  object on the chain (RT-SFO-5). Blinding or bucketing it is the ONLY remedy that closes the length\n"+
			"  oracle — re-padding the sub-frame does not (RT-SFO-5 §'Consequence for the fix').\n"+
			"  DO THIS, in the same commit:\n"+
			"    1. Confirm the Researcher CERTIFIED the change. Entry sits at cbor key 3 inside chain.Block and is\n"+
			"       inside the signing preimage, so this is a FORMAT change and an era question, not a local edit.\n"+
			"    2. Confirm the owner ratified. It was an OPEN OWNER CALL when this pin was written.\n"+
			"    3. Re-read docs/threat-catalog.md F3(a) and docs/math/02-convergent-encryption.md's 'no confirmation\n"+
			"       surface' clause. BOTH were measured FALSE at 00082b8 BECAUSE of this field. A fix here is the\n"+
			"       first thing that could make them true, and correcting them without the fix is NOT a mitigation.\n"+
			"    4. Replace this pin with the positive assertion and state the blinding rule it enforces.",
		wantExact, mode, got)
}

func TestRT_SFO_5_EntryFileSizeIsTheExactByteCount_PINNED_DEFECT(t *testing.T) {
	cs := pipeline.DefaultChunkSize
	// Non-uniform sizes, including both modes. Private mode is the one that matters: the
	// published claim says private mode has "no confirmation surface", and the exact length is
	// published for private objects exactly as for convergent ones.
	for _, L := range []int{1, 999, 262135, 262137, 600000} {
		for _, mode := range []crypto.Mode{crypto.Convergent, crypto.Private} {
			o := rtStage(t, L, mode, cs, 0, erasure.Params{})
			if msg := rtSFO5Pin(int64(L), o.entry.FileSize, mode); msg != "" {
				t.Fatal(msg)
			}
		}
	}

	// THE MECHANISM. The number alone could be right for the wrong reason — e.g. if Entry
	// stopped carrying the field and something else filled it in. Assert the field is
	// populated by the publisher and survives the boundary where the framing changes, and that
	// the sealed manifest's own copy agrees with it. The manifest copy is INSIDE the content
	// box and is the one pipeline.Get checks against; the Entry copy is the public one and is
	// checked by nothing. That asymmetry is the finding.
	o := rtStage(t, 262135, crypto.Private, cs, 0, erasure.Params{})
	blob, err := pipeline.LoadBlob(context.Background(), o.store, o.entry)
	if err != nil {
		t.Fatalf("LoadBlob: %v", err)
	}
	if _, err := manifest.OpenLayout(blob, o.handle.LayoutKey()); err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	if o.entry.FileSize != 262135 {
		t.Fatalf("RT-SFO-5 mechanism arm: Entry.FileSize=%d for a 262,135-byte object", o.entry.FileSize)
	}
}

// TestRT_SFO_5_PinRedensWhenFileSizeIsBlinded is the TEETH.
func TestRT_SFO_5_PinRedensWhenFileSizeIsBlinded(t *testing.T) {
	// Bucketed to a coarse ladder — one of the two remedies the red-team names. Must redden.
	if msg := rtSFO5Pin(999, 1024, crypto.Private); msg == "" {
		t.Fatal("rtSFO5Pin stayed silent when a 999-byte object reported FileSize=1024 — the pin cannot detect " +
			"the bucketing remedy it exists to detect (simplicity rule 7)")
	}
	// Dropped entirely — the other remedy ("nothing in consensus reads it"). Must redden.
	if msg := rtSFO5Pin(999, 0, crypto.Private); msg == "" {
		t.Fatal("rtSFO5Pin stayed silent when FileSize was dropped to 0 — a removal of the field would land unnoticed")
	}
	// The pinned state must stay silent.
	if msg := rtSFO5Pin(999, 999, crypto.Private); msg != "" {
		t.Fatalf("rtSFO5Pin fired on the pinned state it is supposed to accept: %s", msg)
	}
}
