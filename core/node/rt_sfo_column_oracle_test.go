package node

// RED-TEAM R-SUBFRAME-SIZE-ORACLE — RT-SFO-3 (the column-key half) and RT-SFO-6 (the
// MsgChallenge scope question, as RE-SPECIFIED by the PE). Confirmed at origin/main @ 00082b8,
// 2026-09-11.
//
// Source: silt-agent-memory/red-team/reviews/RED-TEAM-R-SUBFRAME-SIZE-ORACLE-00082b8-2026-09-11.md
// PE ruling: silt-agent-memory/principal-engineer/reviews/ruling-rt-sfo-6-msgchallenge-scope-2026-09-11.md
//
// The pipeline-side halves of this pass (RT-SFO-1, 2, 4, 5, and RT-SFO-3's reconstruction arm)
// are in core/pipeline/rt_sfo_manifest_oracle_test.go and rt_sfo_stripe_test.go, together with
// the full note on the pin idiom used for the live defects.

import (
	"bytes"
	"context"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

// rtsfoPublish stages a sub-frame object through the REAL publisher and returns the handle,
// entry, opened layout and the store it landed in. Nothing here is hand-built: the
// one-data-shard stripe, the chunk IDs and the seal are all pipeline.Stage's output.
func rtsfoPublish(t *testing.T, payload []byte, mode crypto.Mode) (link.Handle, ports.Entry, *manifest.Layout, *memstore.Store) {
	t.Helper()
	st := memstore.New()
	h, entry, err := pipeline.Stage(context.Background(), st, bytes.NewReader(payload),
		pipeline.Options{ChunkSize: pipeline.DefaultChunkSize, Mode: mode, Rand: rtsfoRand()})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	blob, err := pipeline.LoadBlob(context.Background(), st, entry)
	if err != nil {
		t.Fatalf("LoadBlob: %v", err)
	}
	l, err := manifest.OpenLayout(blob, h.LayoutKey())
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	return h, entry, l, st
}

type rtsfoDetRand struct{ n byte }

func (d *rtsfoDetRand) Read(b []byte) (int, error) {
	for i := range b {
		d.n++
		b[i] = d.n
	}
	return len(b), nil
}

func rtsfoRand() *rtsfoDetRand { return &rtsfoDetRand{} }

// rtsfoPayload is deliberately NON-UNIFORM. A bytes.Repeat payload would let a wrong
// derivation look right: every chunk of a uniform object convergently encrypts to the same
// ciphertext, so a key-derivation bug that ignored the chunk index would still produce
// matching IDs. This payload varies per 8-byte word.
func rtsfoPayload(n int, seed byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i*31) ^ seed ^ byte(i>>8)
	}
	return out
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-3 (MED, NEW) — one plaintext guess yields seven DHT column keys.
//
// In convergent mode the ciphertext is a deterministic function of the plaintext alone
// (RT-SFO-2 removed the chunk size as an accidental salt), so an attacker holding a candidate
// plaintext re-derives the root, the data chunk ID and all six parity chunk IDs OFFLINE. It
// then computes colKey(root, j) for each of the seven columns and queries MsgGetProviders,
// which core/node/node.go serves unauthenticated and answers with SIGNED provider records
// (ports.ProviderSigningBytes). Those records are transferable: a third party re-derives the
// key from the same guess and verifies the signature, so the attacker mints non-repudiable
// evidence that a named node holds named content, for free.
//
// This gate asserts the DERIVATION, which is the capability. The signature/transferability
// half is a property of ports.ProviderSigningBytes and is not re-tested here.
// ══════════════════════════════════════════════════════════════════════════════

func TestRT_SFO_3_ColumnKeysAreDerivableFromThePlaintextAlone(t *testing.T) {
	plaintext := rtsfoPayload(4096, 0x5a)

	// THE VICTIM publishes. The attacker never sees this store, this handle or this entry.
	_, _, victimLayout, _ := rtsfoPublish(t, plaintext, crypto.Convergent)

	// THE ATTACKER, holding only a GUESS at the plaintext, re-derives everything offline. It
	// uses a different chunk size on purpose: RT-SFO-2 is what makes the guess sufficient
	// WITHOUT the victim's -chunk-size, and if that ever stops being true this arm reddens
	// here rather than silently narrowing the attack.
	attackerStore := memstore.New()
	guess := append([]byte(nil), plaintext...)
	attackerHandle, _, err := pipeline.Stage(context.Background(), attackerStore, bytes.NewReader(guess),
		pipeline.Options{ChunkSize: 64 << 10, Mode: crypto.Convergent})
	if err != nil {
		t.Fatalf("attacker stage: %v", err)
	}
	attackerRoot := attackerHandle.Root

	if attackerRoot != victimLayout.Root() {
		t.Fatalf("RT-SFO-3: the attacker's offline root %s does not match the victim's %s. A plaintext guess no "+
			"longer names the object — either the chunk-size salt is back (see "+
			"TestRT_SFO_2_SubFrameRootIsChunkSizeIndependentAcrossTheLegalRange, which pins the collapse) or "+
			"convergent derivation changed. Establish which before assuming RT-SFO-3 is closed; a narrowed attack "+
			"is not a closed one.", attackerRoot, victimLayout.Root())
	}

	// The seven columns a sub-frame object occupies: the data leaf at column 0 and the six
	// parity leaves at columns k..k+5, per columnAt. Derived with the NODE's own functions so
	// the gate cannot drift from the placement it claims to predict.
	p := erasure.DefaultParams
	leaves := victimLayout.Leaves()
	if len(leaves) != 1+p.ParityShards() {
		t.Fatalf("fixture precondition: %d leaves, want %d", len(leaves), 1+p.ParityShards())
	}

	var attackerKeys, realKeys []ports.Hash
	for i, id := range leaves {
		col := columnAt(i, len(victimLayout.Chunks), victimLayout.K, victimLayout.N)
		realKeys = append(realKeys, placementKey(victimLayout.Root(), id, col))
		// The attacker computes the same key from the root it derived from its GUESS.
		attackerKeys = append(attackerKeys, colKey(attackerRoot, col))
	}
	for i := range realKeys {
		if attackerKeys[i] != realKeys[i] {
			t.Fatalf("RT-SFO-3: at leaf %d the attacker-derived DHT key %s differs from the real placement key %s. "+
				"The red-team measured 7 of 7 derivable from the plaintext alone; if that is no longer true, "+
				"provider discovery stopped being root-keyed or columnAt changed, and the placement analysis in the "+
				"PE's RT-SFO-6 ruling §2 needs re-deriving with it.", i, attackerKeys[i], realKeys[i])
		}
	}

	// The columns really are the seven the finding names, not one key repeated. Without this,
	// an implementation that collapsed every leaf onto a single key would pass the loop above.
	distinct := map[ports.Hash]bool{}
	for _, k := range realKeys {
		distinct[k] = true
	}
	if len(distinct) != 1+p.ParityShards() {
		t.Fatalf("RT-SFO-3: the %d leaves map to %d distinct column keys, want %d. RT-SFO-3's "+
			"'seven holder populations' claim depends on the count; RT-SFO-4's duplicate PARITY SHARDS do not "+
			"collapse the KEYS (the keys are derived from the column index, the shards from the matrix), so a "+
			"change here is separate from RT-SFO-4 and needs its own reading.",
			len(realKeys), len(distinct), 1+p.ParityShards())
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RT-SFO-6 (HIGH, PRE-EXISTING) — the PE's re-specified gate.
//
// WHAT THE RED-TEAM ASKED FOR AND WHY IT IS NOT BUILT. The pass asked for a
// "challenge does not disclose the root to an unrelated peer" test (named verbatim at
// ruling-rt-sfo-6-msgchallenge-scope-2026-09-11.md:167, deliberately NOT spelled as a symbol
// here: it is a test that must never exist, and scripts/check_cited_tests.py is right to flag
// a Test-name in this tree that names nothing). The PE ruled that test VACUOUS:
// it would pin a property that is vacuously true today, because no unrelated peer can name a
// coded shard's chunk ID in the first place — Layout.Chunks and Layout.Parity are inside
// crypto.SealBox under the layout key, and provider discovery is ROOT-keyed (placementKey →
// colKey(root, j)), not chunk-keyed. The test would pass without exercising anything, which is
// precisely the decoration simplicity rule 7 forbids. I agree with the ruling; it is also
// consistent with what TestRT_SFO_3_ColumnKeysAreDerivableFromThePlaintextAlone measures above
// (the attacker derives its keys from the ROOT, never from a chunk id).
//
// WHAT IS BUILT INSTEAD, per the ruling: the ENUMERATION, with a Layout-unseal ABLATION. The
// PE's §2 enumeration is an argument that the property holds today; per [[silt-proof-vs-structure]]
// an argument is not a structure that prevents its violation. The ablation is what converts it:
// the SAME search that finds nothing through the seal finds every shard id once the seal is
// removed, so if the outer seal is ever dropped this gate reddens instead of staying quietly
// green for a new reason.
// ══════════════════════════════════════════════════════════════════════════════

// rtsfoSearch reports which of `needles` appear as a byte substring of `haystack`. It is the
// SAME procedure in both arms of the gate below; only the haystack changes. That is what makes
// the ablation an ablation rather than two different tests.
func rtsfoSearch(haystack []byte, needles []ports.Hash) []ports.Hash {
	var found []ports.Hash
	for _, n := range needles {
		if bytes.Contains(haystack, n[:]) {
			found = append(found, n)
		}
	}
	return found
}

func TestRT_SFO_6_ShardChunkIDsAreNotDerivableWithoutTheLayoutKey(t *testing.T) {
	_, entry, layout, store := rtsfoPublish(t, rtsfoPayload(4096, 0x11), crypto.Convergent)
	shardIDs := layout.Leaves()
	if len(shardIDs) == 0 {
		t.Fatal("fixture precondition: the layout has no leaves")
	}

	// THE PUBLIC SURFACE, assembled exactly as a keyless stranger gets it:
	//   - ports.Entry, served by MsgGetChain unauthenticated (core/node/chainrole.go).
	//   - the sealed manifest chunk bytes, served by MsgFetchChunk unauthenticated
	//     (core/node/node.go). This is the blob, still ciphertext.
	public := bytes.NewBuffer(nil)
	public.Write(entry.Root[:])
	for _, id := range entry.ManifestChunks {
		public.Write(id[:])
		c, err := store.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("manifest chunk %s: %v", id, err)
		}
		public.Write(c.Data)
	}
	public.Write(entry.Publisher[:])

	// (a) THE PROPERTY. No shard id is reachable from the public surface.
	if found := rtsfoSearch(public.Bytes(), shardIDs); len(found) != 0 {
		t.Fatalf("RT-SFO-6 BREAK — %d of %d shard chunk IDs are present in the PUBLIC surface (Entry + the sealed "+
			"manifest bytes a stranger can fetch over MsgFetchChunk). First: %s.\n"+
			"  The PE's scope ruling turns on exactly this: 'no party can name a coded shard's chunk ID without "+
			"already holding that shard's root', which is why MsgChallenge is NOT a stranger-facing chunk -> root "+
			"map and why the finding was ruled not RC-blocking on the privacy axis.\n"+
			"  If this is RED, that ruling's premise is false and RT-SFO-6's severity must be re-opened with the PE "+
			"and the red-team before anything else is decided.", len(found), len(shardIDs), found[0])
	}

	// (b) THE ABLATION — remove ONE condition (the outer seal) and re-run the SAME search.
	// It must find EVERY shard id. Without this arm, (a) is green for an unknown reason: an
	// empty haystack, a broken search, or a layout with no leaves would all pass it.
	// The ablated haystack is the EXACT plaintext the outer seal protects: manifest.Seal
	// encrypts encMode.Marshal(Layout{...}) under the layout key (core/manifest/sealed.go), and
	// encMode is cbor.CanonicalEncOptions(). Re-encoding the opened Layout the same way
	// reproduces those bytes, so this is "the seal removed" and not "a different haystack".
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		t.Fatalf("ablation setup: %v", err)
	}
	unsealed, err := enc.Marshal(layout)
	if err != nil {
		t.Fatalf("ablation setup: re-encoding the opened layout: %v", err)
	}
	found := rtsfoSearch(unsealed, shardIDs)
	if len(found) != len(shardIDs) {
		t.Fatalf("ABLATION FAILED TO ABLATE — with the outer seal REMOVED the same search found only %d of %d "+
			"shard chunk IDs. The search procedure cannot see the ids it is supposed to be unable to find through "+
			"the seal, so arm (a)'s green result proves nothing about the seal (the silt-ablation-noop-guard scar: "+
			"a probe that silently fails to probe is indistinguishable from a passing one).",
			len(found), len(shardIDs))
	}

	// (c) THE DISCOVERY KEY IS ROOT-KEYED, NOT CHUNK-KEYED — the ruling's second premise,
	// asserted rather than inherited (coordination rule 3: verify before you assert). If
	// placementKey ever returns the bare chunk id for a coded shard, a chunk-keyed provider
	// lookup becomes possible and the enumeration in §2 of the ruling loses a case.
	for i, id := range shardIDs {
		col := columnAt(i, len(layout.Chunks), layout.K, layout.N)
		if col == noColumn {
			t.Fatalf("RT-SFO-6 arm (c): leaf %d of a CODED object mapped to noColumn, so placementKey falls back "+
				"to the chunk's own id and provider discovery becomes chunk-keyed. The PE ruling notes this path "+
				"exists in code (K==0) but is unreachable in the shipped publisher; if it is now reachable, the "+
				"stranger CAN form the question and RT-SFO-6 changes severity.", i)
		}
		if got := placementKey(layout.Root(), id, col); got == ports.Hash(id) {
			t.Fatalf("RT-SFO-6 arm (c): placementKey returned the bare chunk id for leaf %d at column %d. "+
				"Discovery is supposed to be colKey(root, j); a chunk-keyed provider record would let a party that "+
				"learned one chunk id find its holders without the root.", i, col)
		}
	}
}

// TestRT_SFO_6_ManifestChunksCarryNoProofSoChallengeDisclosesNothing is the second half of the
// PE's gate: answerChallenge on the PUBLIC, on-chain manifest chunk ids must return Found=false.
//
// The proof-attachment decision belongs to distributeFrom (a proof is built only when
// li := grp.members[k] - manifestN is >= 0, so manifest chunks get proof=nil), so this gate
// drives the REAL Distribute path and then asks the REAL handler, rather than declining to
// store proofs itself. A fixture that simply omitted the proofs would construct the safe state
// whose production absence is the whole question.
func TestRT_SFO_6_ManifestChunksCarryNoProofSoChallengeDisclosesNothing(t *testing.T) {
	// A real bootstrapped ring, because Distribute is the producer under test: whether a
	// manifest chunk carries a proof is decided by distributeFrom (a proof is built only when
	// li := grp.members[k] - manifestN is >= 0). A fixture that simply declined to store proofs
	// for manifest chunks would construct the safe state whose production absence is the entire
	// question, so the placement has to be the shipped one.
	const N = 16
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())
	reg := registry.New()
	cfg := DefaultConfig()
	cfg.Replication = 1

	var nodes []*Node
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(7700 + i)).NodeID()
		nodes = append(nodes, New(id, cfg, sched, net.Endpoint(id), memstore.New()))
	}
	for i, nd := range nodes {
		if i == 0 {
			continue
		}
		var seeds []ports.NodeID
		for j := 0; j < i && j < 3; j++ {
			seeds = append(seeds, nodes[j].ID())
		}
		nd.Bootstrap(seeds, func() {})
	}
	sched.Run()

	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(rtsfoPayload(4096, 0x22)),
		pipeline.Options{ChunkSize: pipeline.DefaultChunkSize, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	full, err := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if len(full.Chunks) != 1 {
		t.Fatalf("fixture precondition: %d data chunks, want the one-data-shard stripe this pass is about", len(full.Chunks))
	}

	distErr := make(chan error, 1)
	nodes[0].Distribute(entry, full, false, DerivePorKey(h.LayoutKey()), func(_ int, err error) { distErr <- err })
	sched.Run()
	select {
	case err := <-distErr:
		if err != nil {
			t.Fatalf("Distribute reported failure: %v — the fixture cannot assert anything about proofs it never produced", err)
		}
	default:
		t.Fatal("Distribute never completed; the fixture cannot assert anything about proofs it never produced")
	}

	// challengeAll asks EVERY node in the ring, because placement decides who holds what and
	// this gate must not assume it.
	challengeAll := func(id ports.ChunkID) (bool, *ports.StorageProof) {
		for _, nd := range nodes {
			reply := nd.answerChallenge(ports.Message{Kind: ports.MsgChallenge, ChunkID: id, PorCount: 1})
			if reply.Found || reply.Proof != nil {
				return reply.Found, reply.Proof
			}
		}
		return false, nil
	}

	// (a) THE PROPERTY: challenging a PUBLIC manifest chunk id discloses nothing. Manifest
	// chunk ids are in ports.Entry.ManifestChunks, inside the hashed block, served
	// unauthenticated by MsgGetChain — so a proof attached to one would turn a public id into
	// the object's root, index, total, column and PoR authenticators, for any stranger.
	for i, cid := range entry.ManifestChunks {
		if found, proof := challengeAll(cid); found || proof != nil {
			t.Fatalf("RT-SFO-6 BREAK — answerChallenge on manifest chunk %d (%s) returned Found=%v proof=%v.\n"+
				"  Manifest chunk ids are PUBLIC. A proof attached to one is a stranger-facing chunk -> root oracle,\n"+
				"  which is exactly the capability the PE's scope ruling found does NOT exist and on which it based\n"+
				"  'not RC-blocking on the privacy axis'. Re-open the scope call with the PE and the red-team before\n"+
				"  anything else is decided.", i, cid, found, proof)
		}
	}

	// (b) THE ABLATION. Arm (a) is green whenever the ring holds no proofs at all — if
	// Distribute placed nothing, if answerChallenge were deleted, if the ids were wrong. Prove
	// the SAME call on the SAME handler DOES answer for the shard leaves, which legitimately
	// carry proofs. Exactly one condition changes: which chunk id is named.
	answered := 0
	for _, raw := range full.Leaves() {
		if found, _ := challengeAll(ports.ChunkID(raw)); found {
			answered++
		}
	}
	if answered == 0 {
		t.Fatalf("ABLATION FAILED TO ABLATE — answerChallenge returned Found=false for ALL %d SHARD leaves too, so\n"+
			"  arm (a) proves nothing: this ring holds no proofs at all and would have passed with the handler\n"+
			"  deleted (the silt-gates-hold-the-seam scar: force the fixture into the branch). Fix the fixture\n"+
			"  before trusting the green.", len(full.Leaves()))
	}
	t.Logf("RT-SFO-6 ablation drove the branch: %d of %d shard leaves answered a challenge, while 0 of %d public "+
		"manifest chunk ids did", answered, len(full.Leaves()), len(entry.ManifestChunks))
}
