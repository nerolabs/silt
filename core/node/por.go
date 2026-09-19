// Proof of retrievability: storage spot-checked against a commitment the prover
// does not get to choose.
//
// The pieces and core/por:
//
// - Every chunk is distributed WITH its Merkle inclusion proof, so a
// host can always show "this chunk is leaf i of root R" — binding
// what it claims to store to a root the whole network agrees on.
// - Every data/parity shard's own bytes are committed too: the publisher
// writes each shard's spot-check root into the file's sealed layout
// (manifest.ShardRoots). Who may write that commitment, and why an
// auditor must take it from there rather than from a response, is at
// por.VerifyOpenings.
// - A challenge names a seed and a sample count. The prover returns the
// sampled leaves' BYTES with their Merkle paths, and the auditor checks
// each path against the committed shard root under its own derived
// index list. There is no key in the scheme, so there is no party that
// can answer without the bytes.
//
// WHAT THIS REPLACED AND WHY. The scheme here used to be Shacham-Waters private
// verification under a key derived from the file's LAYOUT key. That key is what a
// care link publishes, so the party that had to VERIFY was a party that could
// FORGE: with the key and no bytes, an adversary set every mu to zero and solved
// the verification equation for sigma. The break was in the key's distribution, not
// in the arithmetic, and no repair inside the primitive could reach it. A keyless
// scheme has no key to distribute. The production cost that made this the entry, and
// the geometry that sets its parameters, are measured in core/por.
//
// WHAT IT COSTS. The audit is no longer size-independent in its response: it moves
// SpotSampleCount leaves and their paths instead of one aggregate. At the shipped
// geometry that is fewer bytes than the aggregate it replaced, but it is bytes
// rather than arithmetic, and a much larger sample count would not stay so.
package node

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// ctOverhead is the byte cost AES-256-GCM adds to a chunk: ciphertext =
// plaintext + this (see core/crypto ConvergentEncrypt/PrivateEncrypt). The
// auditor uses it to recompute a full shard's expected leaf count from the
// layout's plaintext ChunkSize (the spot-check tree is over ciphertext). A guard
// test pins this to the real overhead so it can't drift.
const ctOverhead = 16

// porSampleCount is how many leaves one storage challenge samples. It is
// core/por's shipped geometry, named here because the challenge carries it and the
// audit path reads it in several places; the number and the reasoning that sets it
// live with the scheme.
const porSampleCount = por.SpotSampleCount

func copyProof(p ports.StorageProof) ports.StorageProof {
	p.Path = append([]ports.Hash(nil), p.Path...)
	return p
}

// shardLeaves is the leaf count the auditor DEMANDS for every shard of one object.
// Every stored shard of an object is the same size and that size is committed
// (m.ChunkSize is the frame the object was actually split at), so the count is a
// function of committed data and never of what a prover reports.
func shardLeaves(chunkSize int64, leafBytes int) int {
	return por.SpotLeaves(int(chunkSize)+ctOverhead, leafBytes)
}

// verifyStorageProofAgainst is the BINDING check: does this inclusion proof put
// `leaf` under the root the verifier is auditing? The root is the verifier's own,
// never the response's, and a proof naming a different root is refused before the
// path is walked.
//
// THE ROOT ARGUMENT IS THE WHOLE POINT. The check used to read p.Root — the root
// the PROVER supplied — which makes the leg a tautology: anyone holding a chunk id
// can name a one-leaf tree over it and produce a self-consistent proof that the
// shard is in a tree of its own devising. What the audit needs to know is the
// opposite question, whether the shard is in THIS object, and only the auditor's
// root can answer it.
//
// p.Root is still compared rather than ignored, so a response that names a foreign
// root is refused for naming it, not merely failed on the path walk — the two are
// the same verdict here but not the same evidence, and a caller reading a log wants
// the first.
//
// ADVERSARY-SHAPE: capability=ProverSuppliedRoot UNCOVERED: TestForeignRootedProofIsRefused DOES grant the capability -- it hands the verifier a self-rooted one-leaf tree over a chunk id, which is the proof this claim says the auditor's root refuses -- but it carries no control that DISCRIMINATES. Removing the capability means proving under the AUDITED root, which needs the bytes and is therefore the honest path rather than the same attack, so the fixture's second arm is a no-over-rejection control (the honest proof still passes) and not a capability control. Same structural reason as ForeignSeedProof further down this file (recorded 2026-09-19): a CAPABILITY-CONTROL is only well defined where the defence is BROKEN, and this one has held since the root binding landed.
func verifyStorageProofAgainst(p ports.StorageProof, leaf ports.ChunkID, want ports.Hash) bool {
	if p.Root != want {
		return false
	}
	return manifest.VerifyProof(want, leaf, manifest.Proof{Index: p.Index, Total: p.Total, Path: p.Path})
}

// storageProofSelfConsistent is the WEAKER check, for the one place that cannot ask
// the binding question: accepting a chunk someone pushed at us. A receiving node
// holds no independent root for content it has not been asked to care for, so all
// it can establish is that the proof it was handed is internally well-formed for
// the root it names.
//
// IT IS NOT A BINDING CHECK AND MUST NOT BE READ AS ONE — that conflation is the
// defect this split exists to end. What actually binds a stored shard to an object
// is the audit, which runs verifyStorageProofAgainst under a root the auditor
// derived from the care link. This function's only job is to refuse a proof that
// could not be defended under ANY root, so the node does not take custody of bytes
// whose accompanying proof is malformed.
func storageProofSelfConsistent(p ports.StorageProof, leaf ports.ChunkID) bool {
	return manifest.VerifyProof(p.Root, leaf, manifest.Proof{Index: p.Index, Total: p.Total, Path: p.Path})
}

// porChallengeSeed turns the auditor's monotonic request counter into a per-
// sweep BASE seed. The old comment claimed "a prover lacking the bytes cannot
// answer any seed" — falsified that: a data-less identity cannot COMPUTE a
// proof, but it can RELAY the proof of an honest holder that can. porProverSeed
// (below) folds the challenged identity into the seed so a relayed proof fails.
//
// RELAY was always denied by the seed binding. OUTSOURCING was NOT, until the
// prover-identity binding landed on 2026-09-19: a willing holder A computed under
// B's OWN seed on request because answerChallenge took no view of who the challenge
// was addressed to. The fixture below is that adversary — A is granted to B as a
// real oracle — and the challenge now reaches A carrying the auditor's base, so A
// folds in its own id, sees a seed bound to B, and declines.
//
// ADVERSARY-SHAPE: capability=HonestHolderAsOracle fixture=TestChallengeOutsourcingIsRefusedByTheProver
func porChallengeSeed(nonce uint64) [32]byte {
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], nonce)
	return sha256.Sum256(append([]byte("silt/por/challenge/v1"), nb[:]...))
}

// porProverSeed binds a challenge to the identity being challenged (M0 hardening
// H1 /, Invariant A): the seed a prover answers is H(base ‖ proverID), so its
// coefficients are prover-specific. A data-less identity B that relays honest
// holder A's aggregated (μ, σ) — computed under A's seed — fails B's verify,
// since B is graded under H(base ‖ B) ≠ H(base ‖ A). This stops the trivial
// one-proof-to-N-Sybils relay; the deeper "colluding holder recomputes a fresh
// proof per Sybil" residual is why PoR grants no STANDING at all (see
// credit.Reputation) — plain PoR over shared content is not Sybil- resistant §4
// S2.
func porProverSeed(base [32]byte, prover ports.NodeID) [32]byte {
	h := sha256.New()
	h.Write([]byte("silt/por/challenge/prover/v2"))
	h.Write(base[:])
	h.Write(prover[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// answerChallenge is the prover side. An honest holder aggregates its stored
// bytes + tags into a passing proof. A liar kept the tags but ditched the
// bytes: it can still form a response over its (absent) data, but the μ it
// produces cannot satisfy the verification equation — and no fetch is needed
// to expose that.
// challengeIsAddressedToMe reports whether msg.PorSeed is the seed bound to THIS
// node's identity. It is the answer to challenge OUTSOURCING: the seed binding
// already made a RELAYED proof useless (a proof computed under A's seed fails B's
// verify), but nothing stopped a data-less identity B from forwarding the auditor's
// verbatim challenge to a real holder A, which computed under B's seed on request
// and could not tell it was answering for someone else. B returned A's work as its
// own, was graded PASSED, and was paid — measured at 1000 credit to a prover holding
// zero bytes.
//
// THE PROVER IDENTITY NOW REACHES THE PROVER, which is the structural close. It
// arrives as the UNBOUND base rather than as a name, so nothing has to be trusted:
// the node folds its OWN id into the base and compares. A forwarded challenge
// carries the forwarder's derived seed, which cannot equal this node's, so the
// oracle refuses. No latency gate, no new secret, nothing an adversary's own path
// can move.
//
// BOTH BINDINGS ARE TESTED because two callers challenge through this one handler
// with different domain separators — the audit sweep with porProverSeed, the repair
// claim's retrievability leg with repairproof.RepairChallengeSeed. Testing the pair
// against this node's own identity says "this challenge is addressed to me" without
// the message having to declare which caller sent it, and a cross-domain collision
// is a SHA-256 preimage coincidence. Both honest callers send a self-bound seed to
// the node they are challenging, so neither honest path is narrowed.
//
// NO BASE, NO OPINION. An auditor that sends no base gets the old behaviour — the
// seed is answered verbatim. That is what keeps this additive: an old auditor is
// still served, and the defeat closes for every pair whose PROVER is current, which
// is as far as a change on this side can reach.
//
// ADVERSARY-SHAPE: capability=HonestHolderAsOracle fixture=TestChallengeOutsourcingIsRefusedByTheProver
func (n *Node) challengeIsAddressedToMe(msg ports.Message) bool {
	if len(msg.PorBase) == 0 || len(msg.PorSeed) == 0 {
		return true // no base supplied: nothing to check against, answer as before
	}
	var base, got [32]byte
	copy(base[:], msg.PorBase)
	copy(got[:], msg.PorSeed)
	audit := porProverSeed(base, n.id)
	repair := repairproof.RepairChallengeSeed(base, n.id)
	return got == audit || got == repair
}

func (n *Node) answerChallenge(msg ports.Message) ports.Message {
	reply := ports.Message{Kind: ports.MsgChallengeReply}
	if !n.challengeIsAddressedToMe(msg) {
		// Found=false, and that is the correct grade rather than a dropped frame:
		// the party graded on this answer is the FORWARDER, not this node, so the
		// refusal lands on the identity that tried to borrow a prover. Contrast the
		// per-challenger rate limit in bondaudit.go, which must DROP because there
		// the refusal would be graded against the honest holder itself.
		n.logf(ports.LogInfo, "storage challenge refused — its seed is bound to another identity",
			"chunk", msg.ChunkID, "self", n.id)
		n.Stats.ProxiedChallengesRefused++
		return reply
	}
	// The full proof (the Merkle Path) lives in the backing; page it in. Same
	// tradeoff as serving a cold chunk — the audited proof is by definition on
	// the serve/audit path, not a hot-loop iterate.
	stored, hasProof, err := n.proofs.Get(msg.ChunkID)
	if err != nil {
		n.logf(ports.LogWarn, "proof read failed for audit", "chunk", msg.ChunkID, "err", err)
	}
	if !hasProof {
		return reply // Found=false: nothing to prove
	}
	p := copyProof(stored)
	reply.Proof = &p
	var seed [32]byte
	copy(seed[:], msg.PorSeed)

	if n.liar {
		// Kept the receipt, ditched the goods. It has no bytes, so it has nothing
		// to open: it claims possession and sends an empty answer, which is the
		// most a data-less holder can do under a keyless scheme. Under the
		// aggregate scheme it could still COMPUTE a doomed response over absent
		// data; here there is no arithmetic to perform, and the difference is the
		// break closing rather than the fixture weakening.
		reply.Found = true
		reply.PorBlocks = 0
		return reply
	}

	ck, err := n.store.Get(bg(), msg.ChunkID)
	if err != nil {
		return ports.Message{Kind: ports.MsgChallengeReply} // lost it; admit it
	}

	// THE LEAF WIDTH IS THE ONE THE PUBLISHER COMMITTED, read from the proof that
	// arrived with the shard — never from the challenge. A challenger that could
	// name it could name a one-byte leaf and make a 110-byte frame cost the prover
	// a tree over a quarter-million leaves, which is the amplification shape
	// persona 14 exists to refuse. A prover that was handed no width answers
	// nothing, because it cannot know the geometry it is being asked about.
	//
	// ADVERSARY-SHAPE: capability=ChallengerNamedGeometry UNCOVERED: no fixture GRANTS a challenger the power to name the prover's leaf width, and none can without first adding the wire field the protocol deliberately does not have. The claim is enforced by the ABSENCE of that field rather than by a check, so the thing to guard is any future challenge field that reaches this decomposition. If one is ever added, this becomes a coverable claim and should get a fixture in the same change.
	leafBytes := stored.LeafBytes
	if leafBytes <= 0 {
		return ports.Message{Kind: ports.MsgChallengeReply}
	}
	data := ck.Data

	// NOR DOES THE CHALLENGER SET HOW MANY SAMPLES IT GETS. PorCount is an
	// attacker-controlled number and this scheme's response grows with it, which the
	// aggregate scheme's did not: unclamped, a 110-byte frame asking for every leaf
	// buys a ~1 MB reply and one Merkle proof per leaf. The prover answers at most
	// the protocol's own sample count, so the response is bounded by a constant
	// whatever the frame says. The honest path is untouched — an auditor sends
	// exactly this many — and an answer with more openings than the auditor derived
	// indices fails its grade anyway; the point of clamping HERE is that the cost is
	// otherwise already spent by the time the grade sees it.
	count := msg.PorCount
	if count > porSampleCount || count <= 0 {
		count = porSampleCount
	}

	if n.shrinkLiar {
		// F4: keep ONLY the first leaf, report a leaf count of 1, and open the
		// challenge against that single leaf. Under a naive auditor (one that
		// trusts the self-reported count) this passes while holding a sliver of
		// the shard. The auditor demands the shard's COMMITTED leaf count and
		// derives the sampled indices over it, so both legs refuse this.
		if len(data) > leafBytes {
			data = data[:leafBytes]
		}
		ops, err := por.Open(data, leafBytes, seed, count)
		if err != nil {
			return ports.Message{Kind: ports.MsgChallengeReply}
		}
		reply.Found = true
		reply.PorBlocks = 1
		reply.PorOpen, reply.PorPaths = flattenOpenings(ops)
		return reply
	}

	ops, err := por.Open(data, leafBytes, seed, count)
	if err != nil {
		return ports.Message{Kind: ports.MsgChallengeReply}
	}
	reply.Found = true
	reply.PorBlocks = por.SpotLeaves(len(data), leafBytes)
	reply.PorOpen, reply.PorPaths = flattenOpenings(ops)
	return reply
}

// flattenOpenings puts openings on the wire: one entry per sample in draw order,
// the leaf bytes in PorOpen and that sample's sibling hashes concatenated in
// PorPaths. Concatenated rather than nested because every path in one answer has
// the same length — the tree's — so the split is arithmetic and a nested array
// would only add a decoder shape to bound.
func flattenOpenings(ops []por.Opening) (leaves [][]byte, paths [][]byte) {
	leaves = make([][]byte, 0, len(ops))
	paths = make([][]byte, 0, len(ops))
	for _, o := range ops {
		leaves = append(leaves, o.Leaf)
		buf := make([]byte, 0, len(o.Path)*32)
		for _, h := range o.Path {
			buf = append(buf, h[:]...)
		}
		paths = append(paths, buf)
	}
	return leaves, paths
}

// parseOpenings is the auditor's side of that encoding, and it is a DECODER of
// attacker-controlled bytes: every length is checked before anything is allocated
// against it (B7). A malformed answer yields nil, which fails the grade exactly as
// a wrong answer does — there is no "malformed" verdict to be lenient with.
func parseOpenings(msg ports.Message, wantSamples int) []por.Opening {
	if len(msg.PorOpen) != wantSamples || len(msg.PorPaths) != wantSamples {
		return nil
	}
	ops := make([]por.Opening, 0, wantSamples)
	for i := range msg.PorOpen {
		raw := msg.PorPaths[i]
		if len(raw)%32 != 0 || len(raw)/32 > maxSpotPathHashes {
			return nil
		}
		path := make([]ports.Hash, 0, len(raw)/32)
		for off := 0; off < len(raw); off += 32 {
			var h ports.Hash
			copy(h[:], raw[off:off+32])
			path = append(path, h)
		}
		ops = append(ops, por.Opening{Leaf: msg.PorOpen[i], Path: path})
	}
	return ops
}

// maxSpotPathHashes bounds a declared Merkle path. A shard is bounded by
// manifest.MaxChunkSize and a leaf is at least one byte, so no honest path can be
// longer than the depth of a tree over that many leaves; a declared path past it is
// a number that has not been checked.
const maxSpotPathHashes = 64

// AuditReport summarizes one audit sweep.
type AuditReport struct {
	Challenges int
	Passed     int
	Failed     int
	NoTruth    int // leaves where no provider produced a passing proof
	// Unaudited counts shards the sweep could not check at all because the
	// object committed no spot-check roots. It is reported rather than folded
	// into Failed: nobody lied, and nobody was checked, and a report that
	// showed either as the other would be the dashboard that flatters (S5).
	Unaudited int
}

// Audit spot-checks every data/parity shard of root: challenge each
// provider and verify the PoR response against the key derived from the
// care-link — no ground-truth fetch. Results settle into the ledger — rent
// for the honest, slashes for the liars. The auditor needs the manifest (it
// fetches it first) and a ledger to settle into.
func (n *Node) Audit(reg ports.Registry, ch link.CareHandle, done func(AuditReport)) {
	n.lookupEntryAsync(reg, ch.Root, func(entry ports.Entry, ok bool, err error) {
		var report AuditReport
		if err != nil || !ok {
			done(report)
			return
		}
		n.auditEntry(entry, ch, done)
	})
}

// auditEntry is Audit past the (async) registry resolution.
func (n *Node) auditEntry(entry ports.Entry, ch link.CareHandle, done func(AuditReport)) {
	var report AuditReport
	n.fetchAll(entry.ManifestChunks, func(missing []ports.ChunkID) {
		if len(missing) > 0 {
			done(report)
			return
		}
		m, err := pipeline.LoadLayout(bg(), n.store, entry, ch)
		if err != nil {
			done(report)
			return
		}
		leaves := m.Leaves()
		root := m.Root()
		dataN := len(m.Chunks)
		// THE COMMITMENT OR NOTHING. An object whose layout carries no shard
		// roots cannot be spot-checked: the auditor would have no number of its
		// own to check a sampled leaf against, and the only alternative — taking
		// the root from the answer — is the tautology this scheme exists to end.
		// Such an object is reported as unaudited rather than graded, so nobody
		// is paid or slashed on a check that did not happen (S3: no silent-loss
		// shape, and no silent-pass shape either).
		if len(m.ShardRoots) != len(leaves) || m.LeafBytes <= 0 {
			n.logf(ports.LogWarn, "object carries no shard-root commitment — not auditable",
				"root", root, "shards", len(leaves), "roots", len(m.ShardRoots))
			report.Unaudited = len(leaves)
			done(report)
			return
		}
		// EVERY stored shard of one object is the SAME size, and that
		// size is COMMITTED: chunk.Split zero-pads a frame that shares
		// a stripe, erasure pads short stripes, and a single-frame
		// object is framed at its true length — in all three cases
		// m.ChunkSize is the frame that was used, so on the wire there
		// is no short tail WITHIN an object. The auditor therefore
		// demands the same leaf count for every shard —
		// m.ChunkSize+GCM ciphertext bytes — and a prover cannot
		// shrink the challenge by under-reporting its leaf count: the
		// old tail-leniency branch accepted any
		// 1.wantFull for the last leaf, which is actually full-size,
		// letting a liar report PorBlocks=1 and pass while holding one
		// leaf. What keeps F4 closed is that the AUDITOR fixes the
		// number from committed data, not that the number is large; a
		// sub-frame object's shard is one leaf, which is fully
		// sampled.
		//
		// ADVERSARY-SHAPE: capability=UnderReportedBlockCount UNCOVERED: no fixture GRANTS AND CONTROLS FOR a prover a self-reported PorBlocks the auditor reads. The F4 closure rests on the auditor fixing the number from committed data; no fixture drives an adversary that tries to move it.
		want := shardLeaves(m.ChunkSize, m.LeafBytes)
		var nextLeaf func(i int)
		nextLeaf = func(i int) {
			if i == len(leaves) {
				done(report)
				return
			}
			// Providers live under the column key for coded shards; resolve
			// the audit there, but challenge the individual shard by id.
			key := ports.Hash(leaves[i])
			if col := columnAt(i, dataN, m.K, m.N); col != noColumn {
				key = colKey(root, col)
			}
			var shardRoot ports.Hash
			copy(shardRoot[:], m.ShardRoots[i])
			n.auditLeaf(leaves[i], key, root, shardRoot, want, m.LeafBytes, &report, func() { nextLeaf(i + 1) })
		}
		nextLeaf(0)
	})
}

// blocksOK cross-checks a prover's reported leaf count against the count the auditor RECOMPUTED for the shard —
// exactly, for every leaf. Every stored shard of one object is the same size and that size is committed in
// m.ChunkSize, so the count is the same want for all of its leaves
// (TestAuditorAndHonestProverAgreeOnEveryShardLength drives both regimes). Letting any shard report a smaller count
// let a liar advertise one leaf and be challenged on that leaf alone, passing while holding a sliver of the shard.
// The auditor, not the prover, fixes the sample space.
func blocksOK(reported, want int) bool {
	return reported >= 1 && reported == want
}

type challengeAnswer struct {
	prover ports.NodeID
	valid  bool // Merkle proof present and verifies for this leaf
	blocks int  // prover-reported leaf count
	opens  []por.Opening
}

func (n *Node) auditLeaf(id ports.ChunkID, key ports.Hash, root ports.Hash, shardRoot ports.Hash,
	want, leafBytes int, report *AuditReport, done func()) {

	n.resolveProviders(key, func(provs []ports.NodeID) {
		n.rid++ // draw a fresh, deterministic base seed from the request counter
		base := porChallengeSeed(n.rid)
		var answers []challengeAnswer
		var challengeNext func(i int)
		challengeNext = func(i int) {
			if i == len(provs) {
				n.gradeAnswers(id, shardRoot, base, want, leafBytes, answers, report, done)
				return
			}
			p := provs[i]
			if p == n.id {
				challengeNext(i + 1)
				return
			}
			// Per-prover seed (H1): p is challenged under H(base ‖ p), so a proof
			// relayed from another holder (computed under a different identity's
			// seed) fails p's verify. p uses PorSeed verbatim, so no prover-side
			// change is needed.
			pseed := porProverSeed(base, p)
			// The BASE travels with the derived seed so p can confirm the challenge
			// is addressed to it (challengeIsAddressedToMe). The derived seed is
			// unchanged and still what p answers under, so an old prover that
			// ignores the base is graded exactly as before.
			n.request(p, ports.Message{Kind: ports.MsgChallenge, ChunkID: id, PorSeed: pseed[:], PorBase: base[:], PorCount: porSampleCount},
				func(resp ports.Message, err error) {
					if err == nil {
						report.Challenges++
						valid := resp.Found && resp.Proof != nil &&
							verifyStorageProofAgainst(*resp.Proof, id, root)
						answers = append(answers, challengeAnswer{
							prover: p, valid: valid, blocks: resp.PorBlocks,
							opens: parseOpenings(resp, min(porSampleCount, want)),
						})
					}
					challengeNext(i + 1)
				})
		}
		challengeNext(0)
	})
}

// gradeAnswers verifies each response against the shard root the OBJECT committed —
// no ground-truth fetch, and no key. An answer passes iff its Merkle proof binds the
// shard to the object's root, its reported leaf count checks out, and every sampled
// leaf it opened carries to the committed shard root. Passing earns rent; failing is
// slashed.
func (n *Node) gradeAnswers(id ports.ChunkID, shardRoot ports.Hash, base [32]byte,
	want, leafBytes int, answers []challengeAnswer, report *AuditReport, done func()) {

	anyValid := false
	for _, a := range answers {
		// Verify over the AUTHORITATIVE leaf count (want), not the prover's
		// self-reported one — the prover cannot shrink its own sample space (F4) —
		// against the COMMITTED shard root, and under this prover's OWN seed
		// H(base ‖ prover) (H1), so an answer opened for another identity's
		// indices fails here.
		//
		// ADVERSARY-SHAPE: capability=ForeignSeedProof UNCOVERED: no fixture GRANTS AND CONTROLS FOR a prover an answer opened under ANOTHER identity's seed. TestChallengeOutsourcingIsRefusedByTheProver DOES drive one -- its relayOnly arm is exactly a foreign-seed answer, with the holder genuinely answering its OWN challenge so the arm tests what it says -- but it is NOT declared as cover here, because the only way to run that attack WITHOUT the capability is to send an empty reply, and an empty reply fails for every capability. A control that cannot discriminate cannot show the capability is load-bearing.
		passed := a.valid && blocksOK(a.blocks, want) &&
			por.VerifyOpenings(shardRoot, want, leafBytes, porProverSeed(base, a.prover), porSampleCount, a.opens)
		if n.ledger != nil {
			n.ledger.RecordAudit(a.prover, id, passed)
		}
		if passed {
			report.Passed++
			anyValid = true
		} else {
			report.Failed++
		}
	}
	if len(answers) > 0 && !anyValid {
		// Nobody demonstrably held the bytes: retrievability unproven.
		report.NoTruth++
	}
	done()
}
