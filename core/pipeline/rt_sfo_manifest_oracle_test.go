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
// RT-SFO-1 (HIGH, NEW) — a keyless encryption-mode oracle.
//
// manifest.secretsPart.Mode is sealed under the CONTENT key (core/manifest/sealed.go). A
// caretaker holding only a care link is not supposed to read it; that is the entire purpose
// of the two-layer seal (core/link/link.go). But pipeline.ManifestFrameSize frames a
// single-chunk sealed manifest at len(blob)+chunk.HeaderSize, and the blob is canonical CBOR,
// so its length is a deterministic function of what is inside it. Convergent mode carries 34
// bytes of ChunkSecrets per data chunk where private mode carries one 34-byte FileKey.
//
// The frame length therefore publishes, to anyone who can dial one node and fetch one chunk,
// a function of a field the seal exists to hide — and by cmd/silt/main.go's own framing
// (private is the default, convergent carries a loud warning) that field is the publisher's
// own "is this secret" classification of its content, joined to Entry.Publisher and the
// commit height.
// ══════════════════════════════════════════════════════════════════════════════

// rtSFO1MeasuredLengths is the measurement from this tree, reproduced by these gates. It is
// byte-for-byte what the red-team pass recorded at 00082b8.
var rtSFO1MeasuredLengths = []struct {
	L          int
	conv, priv int
}{
	{1, 345, 341},
	{100, 347, 343},
	{1024, 349, 345},
	{65536, 353, 349},
	{262136, 353, 349},  // the last single-frame object at the 256 KiB default
	{262137, 421, 383},  // the first two-frame object: the delta widens, per data chunk
	{600000, 489, 417},  //
	{2097152, 898, 621}, // 9 chunks: 277 bytes of separation
}

// rtSFO1Pin is the pin predicate. It returns "" while the DEFECT IS STILL PRESENT — that is
// the state this gate pins — and a non-empty instruction the moment the two modes stop being
// separable at a measured size. Factored out so its teeth are themselves testable
// (TestRT_SFO_1_PinRedensWhenTheModesConverge), the gateFStampPin pattern.
func rtSFO1Pin(L, conv, priv, wantConv, wantPriv int) string {
	if conv == priv {
		return fmt.Sprintf(
			"RT-SFO-1 PIN IS RED — THE DEFECT LOOKS FIXED, WHICH IS GOOD NEWS THAT MUST BE RECORDED, NOT SWALLOWED.\n"+
				"  At FileSize=%d the sealed-manifest frame is now %d bytes in BOTH convergent and private mode.\n"+
				"  It was %d vs %d at 00082b8, which is the keyless encryption-mode oracle the red-team pass\n"+
				"  measured at 26/26 and 66/66 (RED-TEAM-R-SUBFRAME-SIZE-ORACLE-00082b8-2026-09-11, RT-SFO-1).\n"+
				"  DO THIS, in the same commit that reddened it:\n"+
				"    1. Confirm the convergence is a DELIBERATE FIX and not a coincidence of some other encoding\n"+
				"       change — check that TestRT_SFO_1_ModeOracleIsAliasFreeAcrossGeometry also reddened. If it\n"+
				"       did NOT, the modes collide at ONE size only and the oracle still works everywhere else.\n"+
				"    2. Confirm the owner has ruled on RT-SFO-1's disposition. It was an OPEN OWNER CALL when this\n"+
				"       pin was written (privacy is a Part-0 corner; docs/TENETS.md Part 0) and it is research-gated.\n"+
				"    3. REPLACE this pin with the positive assertion (frame lengths are EQUAL across modes) and keep\n"+
				"       TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes as the guard on the mechanism.",
			L, conv, wantConv, wantPriv)
	}
	if conv != wantConv || priv != wantPriv {
		return fmt.Sprintf(
			"RT-SFO-1 PIN IS RED — THE ORACLE STILL WORKS, BUT ITS SHAPE MOVED, SO SOMETHING RE-ENCODED THE SEAL.\n"+
				"  At FileSize=%d the sealed-manifest frame is convergent=%d private=%d; this pin holds %d / %d\n"+
				"  as measured at 00082b8 (RT-SFO-1). The modes are STILL separable (delta %d), so this is drift,\n"+
				"  not a fix.\n"+
				"  DO THIS: find what changed the sealed manifest's CBOR — manifest.Layout, manifest.secretsPart,\n"+
				"  crypto.SealBox overhead, or pipeline.ManifestFrameSize — confirm the change was intended, confirm\n"+
				"  it does not move core/genesis TestGenesisBlockHashIsPinned, and update these numbers here.",
			L, conv, priv, wantConv, wantPriv, conv-priv)
	}
	return ""
}

// TestRT_SFO_1_ModeIsRecoverableFromTheManifestFrameLength_PINNED_DEFECT pins the live
// defect. GREEN today BY DESIGN: it asserts the oracle still works. See the idiom note above.
func TestRT_SFO_1_ModeIsRecoverableFromTheManifestFrameLength_PINNED_DEFECT(t *testing.T) {
	cs := pipeline.DefaultChunkSize

	// (a) THE SYMPTOM: the exact measured frame lengths, both modes, eight sizes.
	for _, tc := range rtSFO1MeasuredLengths {
		conv := rtManifestFrameBytes(t, tc.L, crypto.Convergent, cs, 0, erasure.Params{})
		priv := rtManifestFrameBytes(t, tc.L, crypto.Private, cs, 0, erasure.Params{})
		if msg := rtSFO1Pin(tc.L, conv, priv, tc.conv, tc.priv); msg != "" {
			t.Fatal(msg)
		}
	}

	// (b) THE CLASSIFIER, which is the actual capability, stated at the strength it actually
	// has. A keyless attacker fetches the victim's manifest chunk, measures its length, and
	// looks the length up in a table it built by publishing its own dummy objects. That table
	// is INDEXED BY FileSize, and the attacker gets the victim's FileSize for free out of the
	// same ports.Entry that gave it ManifestChunks (RT-SFO-5). So the decision procedure is:
	// given FileSize, the frame length decides the mode. Assert exactly that, densely.
	//
	// CORRECTION OF RECORD, measured here, stated by neither source document. The two findings
	// COMPOSE, and the composition is load-bearing for the fix decision:
	//   - Per FileSize the modes are separable: 0 collisions over 429 sizes (arm below).
	//   - ACROSS FileSizes the length sets ALIAS: a 345-byte frame is a convergent object of
	//     1 byte OR a private object of 1,024 bytes; a 349-byte frame is convergent@1,024 or
	//     private@65,536. So an attacker who does NOT know FileSize cannot classify.
	// RT-SFO-1's oracle is therefore CONDITIONED on RT-SFO-5's field. Blinding or bucketing
	// Entry.FileSize — the RT-SFO-5 remedy — degrades this classifier as a side effect, and
	// re-padding the manifest frame — the RT-SFO-1 remedy — does not touch RT-SFO-5. That
	// asymmetry is a reason to sequence RT-SFO-5 first and it is not in the report or the
	// ruling. Do NOT let this pin be read as "the mode is public unconditionally".
	for L := 1; L <= 3000; L += 7 {
		conv := rtManifestFrameBytes(t, L, crypto.Convergent, cs, 0, erasure.Params{})
		priv := rtManifestFrameBytes(t, L, crypto.Private, cs, 0, erasure.Params{})
		if conv == priv {
			t.Fatalf("RT-SFO-1 PIN IS RED (classifier arm) — at FileSize=%d both modes now frame at %d bytes, so "+
				"the length no longer decides the mode AT A KNOWN FileSize. That is the fix. Confirm it is deliberate, "+
				"confirm the owner ruled (open call, Part-0 corner), then replace this pin with the positive assertion "+
				"and keep the padded-framing ablation as the mechanism guard.", L, conv)
		}
	}

	// (b2) THE BOUND, pinned so the finding cannot be over-claimed later. The cross-FileSize
	// aliases above are a REAL limit on the attack, and a limit that disappears silently is as
	// bad as a defect that appears silently. If these two aliases stop existing, the oracle got
	// STRONGER — it would then classify without knowing FileSize at all — and that is an
	// escalation somebody must see.
	aliases := 0
	convByLen, privByLen := map[int]int{}, map[int]int{}
	for _, tc := range rtSFO1MeasuredLengths {
		convByLen[rtManifestFrameBytes(t, tc.L, crypto.Convergent, cs, 0, erasure.Params{})] = tc.L
		privByLen[rtManifestFrameBytes(t, tc.L, crypto.Private, cs, 0, erasure.Params{})] = tc.L
	}
	for v := range convByLen {
		if _, ok := privByLen[v]; ok {
			aliases++
		}
	}
	if aliases == 0 {
		t.Fatalf("RT-SFO-1 ESCALATED — the convergent and private frame-length sets are now DISJOINT across the "+
			"eight pinned FileSizes (%d aliases, pinned 2). The classifier no longer needs the victim's FileSize, so "+
			"the oracle works against an attacker who has only the manifest bytes. This is a WORSENING, not a fix: "+
			"re-run the red-team pass and re-open RT-SFO-1's severity with the owner.", aliases)
	}
	// (c) THE MECHANISM. Pinning the numbers alone would pass for the wrong reason if the
	// cause moved. The cause is that the sealed blob's LENGTH is mode-dependent and the frame
	// is that length plus a header. Assert that directly, against the store, so a fix that
	// equalises the blob (padding secretsPart) and a fix that equalises the frame (padding
	// the manifest frame) are both caught, and neither can leave (a) green by coincidence.
	small := rtStage(t, 1024, crypto.Convergent, cs, 0, erasure.Params{})
	blobConv, err := pipeline.LoadBlob(context.Background(), small.store, small.entry)
	if err != nil {
		t.Fatalf("LoadBlob (convergent): %v", err)
	}
	smallPriv := rtStage(t, 1024, crypto.Private, cs, 0, erasure.Params{})
	blobPriv, err := pipeline.LoadBlob(context.Background(), smallPriv.store, smallPriv.entry)
	if err != nil {
		t.Fatalf("LoadBlob (private): %v", err)
	}
	if len(blobConv) == len(blobPriv) {
		t.Fatalf("RT-SFO-1 PIN IS RED (mechanism arm) — the SEALED BLOB is now %d bytes in both modes. The "+
			"length of the sealed manifest no longer depends on which branch of secretsPart is populated. That "+
			"is the root fix, not a framing change. Confirm it, confirm the owner ruled, and replace this pin.",
			len(blobConv))
	}
	if got := rtManifestFrameBytes(t, 1024, crypto.Convergent, cs, 0, erasure.Params{}); got != len(blobConv)+chunk.HeaderSize {
		t.Fatalf("RT-SFO-1 PIN IS RED (mechanism arm) — the manifest frame is %d bytes for a %d-byte blob, not "+
			"blob+%d. pipeline.ManifestFrameSize's true-length framing is what publishes the blob length to a "+
			"keyless observer; if the frame is no longer the blob's length, re-read RT-SFO-1 before assuming the "+
			"oracle is closed — a frame that is padded UP is a fix, a frame that grew for another reason is not.",
			got, len(blobConv), chunk.HeaderSize)
	}
}

// TestRT_SFO_1_PinRedensWhenTheModesConverge is the TEETH. It never publishes anything: it
// feeds rtSFO1Pin the post-fix value and the drifted value and asserts the predicate speaks.
// Without this, the pin above is a green gate with no demonstrated red — decoration under
// simplicity rule 7 — because nothing would ever have checked that it CAN fail.
func TestRT_SFO_1_PinRedensWhenTheModesConverge(t *testing.T) {
	// The post-fix state: both modes frame identically. Must redden.
	if msg := rtSFO1Pin(1, 345, 345, 345, 341); msg == "" {
		t.Fatal("rtSFO1Pin stayed silent when convergent and private framed identically — the pin cannot detect " +
			"the fix it exists to detect, so it is decoration (simplicity rule 7)")
	}
	// The drift state: still separable, but the numbers moved. Must redden.
	if msg := rtSFO1Pin(1, 349, 341, 345, 341); msg == "" {
		t.Fatal("rtSFO1Pin stayed silent when the measured convergent frame moved from 345 to 349 — an encoding " +
			"change under the seal would pass unnoticed")
	}
	// The pinned state: must stay silent, or the gate is a permanent false alarm.
	if msg := rtSFO1Pin(1, 345, 341, 345, 341); msg != "" {
		t.Fatalf("rtSFO1Pin fired on the pinned state it is supposed to accept: %s", msg)
	}
}

// TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes is the ABLATION, and the brief is
// right that it is the valuable half. Restoring the pre-4′ padded framing
// (Options.ManifestFrameBytes = ChunkSize, the field the dup-publish gate already uses) kills
// the classifier dead. PIN THAT TOO: without this arm, a future revert to padded framing looks
// like a regression in the pins above rather than the remedy it is, and — worse — someone
// could "fix" RT-SFO-1 by re-padding without any test recording that re-padding is what
// closed it. RT-SFO-5 is why that matters: re-padding the sub-frame closes the MANIFEST
// oracle and does NOT close the length oracle, because Entry.FileSize publishes the exact
// length anyway. Two different leaks, one framing knob.
func TestRT_SFO_1_PaddedFramingAblationCollapsesBothModes(t *testing.T) {
	cs := pipeline.DefaultChunkSize
	for _, L := range []int{1, 1024, 262135} {
		conv := rtManifestFrameBytes(t, L, crypto.Convergent, cs, cs, erasure.Params{})
		priv := rtManifestFrameBytes(t, L, crypto.Private, cs, cs, erasure.Params{})
		if conv != priv {
			t.Fatalf("ABLATION FAILED TO ABLATE — under padded framing (ManifestFrameBytes=%d) a %d-byte object "+
				"still frames convergent=%d private=%d. The ablation is what proves the true-length framing is the "+
				"CAUSE of RT-SFO-1 rather than a correlate; if padding no longer collapses the modes, the oracle has "+
				"a second source and RT-SFO-1's fix analysis is wrong. Re-run the red-team pass before proceeding.",
				cs, L, conv, priv)
		}
		if conv != cs {
			t.Fatalf("ABLATION FAILED TO ABLATE — padded framing produced %d bytes, not the chunk size %d. "+
				"Options.ManifestFrameBytes no longer pins the frame, so this ablation is not exercising the pre-4′ "+
				"behaviour it claims to and its green result means nothing (the silt-ablation-noop-guard scar).",
				conv, cs)
		}
	}
}

// TestRT_SFO_1_ModeOracleIsAliasFreeAcrossGeometry pins the survivability the red-team
// measured: the oracle does not need the publisher's (k,n). Over a seven-point grid the
// convergent and private length SETS stay disjoint, because the mode delta (4 B at one chunk)
// is not a multiple of the parity step (34 B). This is the arm that says an attacker needs
// neither -chunk-size (RT-SFO-2) nor -erasure.
func TestRT_SFO_1_ModeOracleIsAliasFreeAcrossGeometry(t *testing.T) {
	cs := pipeline.DefaultChunkSize
	grid := []erasure.Params{{K: 2, N: 4}, {K: 3, N: 5}, {K: 4, N: 8}, {K: 6, N: 10}, {K: 10, N: 16}, {K: 12, N: 20}, {K: 16, N: 24}}
	for _, L := range []int{1024, 40000} {
		conv, priv := map[int]erasure.Params{}, map[int]erasure.Params{}
		for _, g := range grid {
			conv[rtManifestFrameBytes(t, L, crypto.Convergent, cs, 0, g)] = g
			priv[rtManifestFrameBytes(t, L, crypto.Private, cs, 0, g)] = g
		}
		for v, gc := range conv {
			if gp, ok := priv[v]; ok {
				t.Fatalf("RT-SFO-1 GEOMETRY PIN IS RED at FileSize=%d — frame length %d is produced by convergent "+
					"k=%d,n=%d AND private k=%d,n=%d. The red-team measured ALIASES=0 over this grid, which is what "+
					"makes the oracle survive an unknown erasure geometry. An alias appearing is either a fix or a "+
					"change to erasure.DefaultParams / the parity step; establish which, and record it.",
					L, v, gc.K, gc.N, gp.K, gp.N)
			}
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
