package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/ports"
)

// The retrievability gates, at the COMPOSED level: the oracle is gradeAnswers' full
// three-leg grade, never por.VerifyOpenings alone.
//
// ⚠ READ THE POLARITY BEFORE YOU READ A RESULT. These were three PINNED_DEFECT
// gates — tests that asserted the tree's BROKEN behaviour and went RED when it was
// fixed. All three are now ordinary positive assertions: GREEN when the rule holds,
// RED when it breaks. The pins were kept as pins only while the defects were open.
//
// The last of them recorded that a care-link holder storing ZERO bytes passed the
// composed audit and was paid for it. The PoR verification key rode the care link,
// so every caretaker, every full-link reader and the publisher could set every mu to
// zero and solve the verification equation for sigma directly. It is closed by a
// scheme with no key: the prover answers with the shard's own bytes, and a party
// that does not hold them has nothing to send. Gate 1 below is that rule, asserted.

// ---------------------------------------------------------------------------
// The fixtures. Both sides of every gate are built from the PRODUCT's own
// derivations — por.ShardRoot for the commitment, por.Open for the prover,
// gradeAnswers for the verdict — so no leg is asserted by the test.
// ---------------------------------------------------------------------------

// spotShard builds a real full-frame shard, the commitment a publisher writes for
// it, and a real inclusion proof under a 4-leaf object root.
func spotShard(t *testing.T) (id ports.ChunkID, data []byte, sp ports.StorageProof,
	objectRoot, shardRoot ports.Hash, leaves int) {
	t.Helper()
	data = make([]byte, pipeline.DefaultChunkSize+ctOverhead)
	for i := range data {
		data[i] = byte(i*167 + 29)
	}
	id = ports.HashBytes(data)
	siblings := []ports.Hash{
		ports.HashBytes([]byte("sibling-0")), id,
		ports.HashBytes([]byte("sibling-2")), ports.HashBytes([]byte("sibling-3")),
	}
	objectRoot = manifest.MerkleRoot(siblings)
	mp, err := manifest.Prove(siblings, 1)
	if err != nil {
		t.Fatalf("merkle prove: %v", err)
	}
	sp = ports.StorageProof{Root: objectRoot, Index: mp.Index, Total: mp.Total,
		Path: mp.Path, Column: -1, LeafBytes: por.SpotLeafBytes}
	shardRoot = por.ShardRoot(data, por.SpotLeafBytes)
	leaves = por.SpotLeaves(len(data), por.SpotLeafBytes)
	return
}

// openReply is the answer an honest holder of `data` returns for `prover`'s
// challenge in round `base`. It goes through the product's own encoder, so a
// fixture cannot pass by sending a shape the wire could not carry.
func openReply(t *testing.T, data []byte, sp ports.StorageProof, base [32]byte,
	prover ports.NodeID, leaves int) ports.Message {
	t.Helper()
	ops, err := por.Open(data, por.SpotLeafBytes, porProverSeed(base, prover), porSampleCount)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := ports.Message{Kind: ports.MsgChallengeReply, Found: true, Proof: &sp, PorBlocks: leaves}
	m.PorOpen, m.PorPaths = flattenOpenings(ops)
	return m
}

// gradeOne runs the PRODUCT's composed three-leg grade over exactly one answer,
// building the `valid` leg the way auditLeaf builds it (por.go) — via the product's
// own verifyStorageProofAgainst, under the AUDITED root the caller supplies.
func gradeOne(t *testing.T, n *Node, id ports.ChunkID, objectRoot, shardRoot ports.Hash,
	base [32]byte, want int, prover ports.NodeID, resp ports.Message) AuditReport {
	t.Helper()
	valid := resp.Found && resp.Proof != nil && verifyStorageProofAgainst(*resp.Proof, id, objectRoot)
	answers := []challengeAnswer{{
		prover: prover, valid: valid, blocks: resp.PorBlocks,
		opens: parseOpenings(resp, min(porSampleCount, want)),
	}}
	var report AuditReport
	report.Challenges++
	fired := false
	n.gradeAnswers(id, shardRoot, base, want, por.SpotLeafBytes, answers, &report, func() { fired = true })
	if !fired {
		t.Fatal("gradeAnswers did not call done")
	}
	return report
}

// ---------------------------------------------------------------------------
// GATE 1 — a care-link holder storing ZERO bytes FAILS the composed audit.
//
// THE RULE, now asserted rather than pinned absent: only a party that actually
// holds the bytes passes a proof-of-retrievability audit. It was absent because the
// PoR verification key rode the care link (link.Handle.Care → LayoutKey → a derived
// Shacham-Waters key), and Definition 2.1's soundness assumes that key is "not
// known to the prover or other parties"; in silt it was a published capability.
//
// THE CLOSE IS A SCHEME WITH NO KEY. The prover returns sampled leaves' BYTES
// against a root the publisher committed in the sealed layout. A care-link holder
// reads that root — it must, to audit — and reading it grants nothing, because
// answering requires a second preimage of a Merkle leaf rather than a solution to
// an equation.
//
// ADVERSARY-HOLDS: CareLinkWithoutBytes -- the forger is handed EVERYTHING the care
// link yields: the layout key, the object root, the shard's committed spot-check
// root, the shard's chunk id, its honest inclusion proof, and the exact committed
// geometry. It holds ZERO shard bytes. That is a strictly stronger adversary than
// the retired pin's, which was handed the layout key alone.
// CAPABILITY-CONTROL: CareLinkWithoutBytes -- see the note below. Where a defence
// HOLDS, removing a capability cannot discriminate, so the arms below are a
// no-over-rejection control (the same party WITH the bytes passes) and a detection
// control (a holder missing ONE leaf fails exactly when that leaf is sampled).
//
// ADVERSARY-HOLDS: RetainedLeafHashes -- the detection control's cheater is granted
// the shard's WHOLE leaf-hash list, which is what lets it build an honest Merkle path
// for every leaf it still holds after dropping one. That is the capability the
// "cheating costs at least the hash" claim assumes an adversary has, and the whole
// point of the claim is that even with it the cheater cannot get below 32 bytes a leaf.
// CAPABILITY-CONTROL: RetainedLeafHashes -- and THIS one discriminates, which is rare
// in this tree. The same cheater WITHOUT the hash list recomputes the tree over the
// bytes it actually holds, and fails even on rounds that never sample what it dropped,
// because the sibling hash on every other leaf's path is computed over the missing
// part. Capability present -> passes the unsampled rounds; capability removed -> fails
// them. The claim is therefore covered rather than recorded.
// ---------------------------------------------------------------------------

// rtPOR1 returns "" while a care-link holder storing ZERO bytes FAILS the composed
// three-leg grade and is not paid for it, and the escalation once it does not.
// TEETH: TestRetrievabilityAssertionsFireOnTheirDefects.
func rtPOR1(fr AuditReport, minted int64) string {
	if fr.Passed == 0 && fr.Failed == 1 && minted <= 0 {
		return ""
	}
	return fmt.Sprintf("A ZERO-BYTE CARE-LINK HOLDER PASSED THE AUDIT — graded %+v, balance moved by %d; "+
		"want Passed=0, Failed=1 and a delta that is not positive.\n"+
		"  IF Passed IS 1, THE KEY-DISTRIBUTION BREAK IS BACK IN SOME FORM: something in the grade is satisfiable without the\n"+
		"  shard's bytes. Check, in this order, that gradeAnswers still verifies against the COMMITTED shard root (the layout's,\n"+
		"  never the response's), that the auditor still fixes the leaf count from committed geometry, and that por.VerifyOpenings\n"+
		"  still derives the sampled indices ITSELF rather than trusting the order it was sent.\n"+
		"  IF THE DELTA IS POSITIVE, the grade held but the payment path did not, which is the half a grade-only assertion misses.\n"+
		"  IF Failed IS 0 WITH Passed 0, the answer never reached the grade; find what dropped it before reading this as a refusal.",
		fr, minted)
}

// rtPOR1V6Firewall returns "" while a forged audit pass buys no Sybil-resistant
// STANDING, and the escalation once it does.
//
// THE PREDICATE IS rep > 0, NOT rep != 0, AND THE DIFFERENCE IS THE WHOLE POINT.
// credit.Ledger.Reputation is bonded bytes MINUS four penalty counters
// (auditsFailed, bondFails, falseRepairs, equivocations). A prover that FAILS its
// audit — which is the success path this file exists to produce — increments
// auditsFailed and goes NEGATIVE. A "!= 0" test therefore fires a false M0
// escalation exactly when the system is working. The property being asserted is "a
// forged PASS buys no STANDING", and that is rep > 0.
// TEETH: TestRetrievabilityAssertionsFireOnTheirDefects.
func rtPOR1V6Firewall(rep int64) string {
	if rep <= 0 {
		return ""
	}
	return fmt.Sprintf("V6 FIREWALL BREACHED — a data-less prover's Reputation is %d after the audit, want <= 0. "+
		"This is not the retrievability grade widening; it is a different and worse finding: a PoR result now buys "+
		"Sybil-resistant standing, which reaches the gamma->1/N firewall and M0. credit.Reputation never reads auditsPassed — "+
		"its only POSITIVE term is bonded bytes, and everything else it reads is a penalty counter — so a POSITIVE reputation "+
		"here cannot have come from the audit unless that changed. ESCALATE before touching this file.", rep)
}

func TestCareLinkHolderWithZeroBytesFailsTheAudit(t *testing.T) {
	n, _ := aloneNode(t, 0)
	led := credit.New(1, 500_000)
	n.SetLedger(led)

	base := porChallengeSeed(4242)
	id, data, sp, objectRoot, shardRoot, leaves := spotShard(t)
	want := shardLeaves(pipeline.DefaultChunkSize, por.SpotLeafBytes)
	if want != leaves {
		t.Fatalf("setup: the auditor demands %d leaves, the shard has %d", want, leaves)
	}
	t.Logf("regime: shard=%d B, leaves=%d, leaf=%d B, sample=%d (p=%.4f per leaf)",
		len(data), leaves, por.SpotLeafBytes, min(porSampleCount, leaves),
		float64(min(porSampleCount, leaves))/float64(leaves))

	// --- POSITIVE CONTROL: an honest holder of the bytes PASSES. ---
	honest := ports.HashBytes([]byte("honest-holder"))
	t0 := time.Now()
	hr := gradeOne(t, n, id, objectRoot, shardRoot, base, want, honest,
		openReply(t, data, sp, base, honest, leaves))
	honestNs := time.Since(t0).Nanoseconds()
	if hr.Passed != 1 {
		t.Fatalf("FIXTURE BROKEN: an honest holder of every byte was graded %+v — "+
			"the forgery result below would be meaningless", hr)
	}

	// --- THE FORGERY: everything the care link yields, and no bytes. ---
	// It sends the HONEST inclusion proof, which it is entitled to derive from the
	// layout, so leg 1 passes honestly and the grade turns purely on whether a
	// party with the commitment but no bytes can open the sampled leaves. It also
	// reports the CORRECT leaf count, so the count leg passes too. Both other legs
	// are conceded deliberately: a forgery that failed on leg 1 or on the count
	// would say nothing about the leaf openings, which are the thing under test.
	forger := ports.HashBytes([]byte("care-link-holder-with-no-disk"))
	t1 := time.Now()
	forged := forgeWithoutBytes(sp, leaves)
	forgeNs := time.Since(t1).Nanoseconds()

	before := led.Balance(forger)
	fr := gradeOne(t, n, id, objectRoot, shardRoot, base, want, forger, forged)
	minted := led.Balance(forger) - before

	t.Logf("MEASURED asymmetry: honest answer %d ns over %d B read; forgery %d ns over 0 B read",
		honestNs, len(data), forgeNs)
	t.Logf("grade of the forged answer: %+v; credit minted to the forger: %d", fr, minted)

	// THE RULE: the zero-byte forger fails and is paid nothing.
	if msg := rtPOR1(fr, minted); msg != "" {
		t.Fatal(msg)
	}

	// --- DETECTION CONTROL: the ADVERSARY that keeps the hashes. This is what makes
	// the gate an assertion about the SCHEME rather than about garbage being
	// rejected, and it must model the right cheater. A holder that simply LOST a
	// leaf fails every challenge, because the sibling hash on every other leaf's
	// path is computed over the part it lost — a strictly worse position than the
	// one below, and not the one the sample count is priced against. The cheater
	// that pays keeps the 32-byte hash of each leaf it drops, so it can still build
	// an honest path for every leaf it kept, and fails only on the rounds that look
	// at what it threw away.
	//
	// THE RETAINED HASHES ARE THEMSELVES THE FLOOR ON WHAT CHEATING SAVES. At a
	// 128-byte leaf, keeping the hash costs a quarter of the leaf, so a holder that
	// drops EVERY byte of a shard and keeps the list still stores 25% of it — and is
	// caught on the first sample with certainty.
	// The two arms are driven DETERMINISTICALLY rather than by waiting for a
	// probability: each round derives the index list the auditor will use, drops a
	// leaf that IS in it, and separately drops one that is NOT. Sampling until a
	// 1-in-2049 leaf happens to be drawn would need thousands of rounds to say
	// anything, and a flaky arm is a dead arm.
	hashes := por.LeafHashes(data, por.SpotLeafBytes)
	rounds := 16
	for r := 0; r < rounds; r++ {
		rb := porChallengeSeed(uint64(90000 + r))
		idx := por.SpotIndices(porProverSeed(rb, honest), leaves, porSampleCount)
		if len(idx) != porSampleCount {
			t.Fatalf("round %d drew %d indices, want %d", r, len(idx), porSampleCount)
		}
		in := map[int]bool{}
		for _, i := range idx {
			in[i] = true
		}
		out := 0
		for ; in[out]; out++ {
		}

		// CAUGHT: it dropped a leaf this round samples.
		if rr := gradeOne(t, n, id, objectRoot, shardRoot, rb, want, honest,
			cheatReply(t, data, hashes, idx[0], sp, idx, leaves)); rr.Passed != 0 {
			t.Fatalf("round %d: a holder missing sampled leaf %d was graded %+v — a sampled leaf the prover "+
				"does not hold must fail every time", r, idx[0], rr)
		}
		// NOT CAUGHT, and that is the specified behaviour: it dropped a leaf this
		// round does not look at. An audit that failed here would be rejecting on
		// something other than what it sampled, and the sample count would price
		// nothing.
		if rr := gradeOne(t, n, id, objectRoot, shardRoot, rb, want, honest,
			cheatReply(t, data, hashes, out, sp, idx, leaves)); rr.Passed != 1 {
			t.Fatalf("round %d: a holder missing UNSAMPLED leaf %d was graded %+v — the audit is not checking "+
				"what it says it checks", r, out, rr)
		}
		// CAPABILITY CONTROL: the SAME cheater without the retained hash list. It
		// rebuilds the tree over the bytes it actually holds, so its path for every
		// other leaf carries a sibling computed over the part it dropped, and the
		// round it was passing above now fails. This is what prices the 32 bytes a
		// leaf: without them, dropping one leaf costs the whole shard's answerability.
		lossy := append([]byte(nil), data...)
		for b := 0; b < por.SpotLeafBytes; b++ {
			lossy[out*por.SpotLeafBytes+b] = 0
		}
		if rr := gradeOne(t, n, id, objectRoot, shardRoot, rb, want, honest,
			cheatReply(t, lossy, por.LeafHashes(lossy, por.SpotLeafBytes), out, sp, idx, leaves)); rr.Passed != 0 {
			t.Fatalf("round %d: a holder that dropped leaf %d AND its hash still passed a round that samples "+
				"neither (%+v) — then retaining the hash list buys the cheater nothing and the floor on what "+
				"cheating saves is not real", r, out, rr)
		}
	}
	t.Logf("detection control: over %d rounds, dropping a SAMPLED leaf failed every time and dropping an "+
		"UNSAMPLED one passed every time — the spot check's probabilistic bound, driven at both ends. "+
		"Dropping one leaf saves the cheater %d B of %d B, because it must keep that leaf's 32-byte hash "+
		"to answer for the others; a cheater that dropped EVERY leaf's bytes would still store %d%% of the shard. "+
		"The same cheater WITHOUT the retained hashes failed those rounds too, which is what makes the 32 bytes load-bearing.",
		rounds, por.SpotLeafBytes-32, len(data), 100*32/por.SpotLeafBytes)

	// The gamma->1/N firewall. A failed audit mints no credit AND no standing.
	// It runs after the rule above, deliberately: ordered before it, this arm
	// fires first on every regression and the guidance above becomes unreachable.
	if msg := rtPOR1V6Firewall(led.Reputation(forger)); msg != "" {
		t.Fatal(msg)
	}
	t.Logf("V6 firewall HOLDS: forger Reputation after the failed audit = %d (its only positive term is bonded bytes)",
		led.Reputation(forger))
}

// forgeWithoutBytes is the best answer a party holding the care link and zero shard
// bytes can assemble. It is deliberately an INDEPENDENT construction: it never calls
// por.Open and no shard bytes exist anywhere in its call graph. It concedes the two
// legs a care-link holder genuinely satisfies — the honest inclusion proof and the
// true leaf count — and fabricates the leaves and paths, which is the only thing it
// could do without a second preimage of a Merkle leaf.
func forgeWithoutBytes(sp ports.StorageProof, leaves int) ports.Message {
	m := ports.Message{Kind: ports.MsgChallengeReply, Found: true, Proof: &sp, PorBlocks: leaves}
	pathHashes := 0
	for k := leaves; k > 1; k = (k + 1) / 2 {
		pathHashes++
	}
	for i := 0; i < porSampleCount; i++ {
		m.PorOpen = append(m.PorOpen, make([]byte, por.SpotLeafBytes))
		m.PorPaths = append(m.PorPaths, make([]byte, pathHashes*32))
	}
	return m
}

// ---------------------------------------------------------------------------
// GATE 2 — challenge OUTSOURCING. B holds no bytes; it forwards the auditor's
// verbatim MsgChallenge to a real holder A and returns A's answer as its own.
//
// THE RULE, asserted rather than pinned absent: a prover cannot have someone else
// answer its identity-bound challenge. It was absent because answerChallenge took
// no view of WHO the challenge was addressed to, so A computed under B's seed on
// request and could not tell. The close is the auditor sending the UNBOUND base
// beside the derived seed and A folding in its OWN id before answering.
//
// THE PIN PREDICTED `from` AND `from` WOULD HAVE BEEN WRONG. Its fix-case pointed
// at Node.handle's MsgChallenge case having `from` in scope. But `from` is the
// FORWARDER — B — so binding to it would have authorised the attack precisely. The
// identity that must reach the prover is SELF. Recorded because the pin's own
// guidance was the misleading part, and the next reader deserves to know the
// prediction was checked rather than followed.
//
// ADVERSARY-HOLDS: HonestHolderAsOracle -- B is granted a real holder A that HAS the
// bytes and is WILLING to answer on request, which is the capability the defence
// assumes no adversary has. A is still willing here; what changed is that it now
// declines a seed naming somebody else, which is the thing under test.
// CAPABILITY-CONTROL: HonestHolderAsOracle -- the relay-only arm below removes the
// oracle (A answers its OWN challenge, genuinely opening leaves, and B forwards
// that) and asserts Passed=0; the empty-reply arm asserts B alone passes nothing.
//
// ⚠ THE CONTROL DISCRIMINATED WHILE THE DEFENCE WAS BROKEN, AND NO LONGER DOES.
// Pre-fix: capability present -> B PASSED and was paid 1000; capability removed ->
// B failed. That is a textbook capability control and it is the evidence that the
// oracle was load-bearing. Post-fix both arms fail, which is the structural state
// this gate's docstring describes for a defence that HOLDS. The discrimination is
// not lost, it is HISTORICAL, and it stays checkable: the teeth test feeds
// por2Outsourcing the recorded pre-fix grade (Passed=1, 25000 minted) and requires
// it to fire.
// ---------------------------------------------------------------------------

// por2Outsourcing: the rule asserted is that the proxied answer is graded FAILED
// and the proxy is NOT PAID for it.
//
// ⚠ "NOT PAID" IS THE PROPERTY, AND IT IS NOT THE SAME AS "NOTHING HAPPENS". The
// measured delta is NEGATIVE — B is SLASHED — and that is the audit working rather
// than an overreach: B announced itself as a provider of a shard it does not hold
// and could not prove possession, which is exactly what an audit is for. Only the
// ORACLE is new. The bound is therefore `minted <= 0`, which states the security
// property (a data-less prover is never paid) without pinning a ledger figure that
// this project's record shows moving with every re-pricing.
//
// The slash lands on B, the identity being graded, never on A. A is not audited by
// this sweep at all; it only declined to answer.
// TEETH: TestRetrievabilityAssertionsFireOnTheirDefects.
func por2Outsourcing(r AuditReport, minted int64) string {
	if r.Passed == 0 && r.Failed == 1 && minted <= 0 {
		return ""
	}
	return fmt.Sprintf("OUTSOURCING IS NO LONGER REFUSED — a data-less identity B that forwarded the auditor's verbatim challenge to "+
		"holder A was graded %+v and its balance moved by %d; want Passed=0, Failed=1 and a delta that is not positive.\n"+
		"  IF Passed IS 1, THE ORIGINAL DEFECT IS BACK: A answered under a seed bound to B and could not tell it was answering for\n"+
		"  someone else. Check that the auditor still sends PorBase (auditLeaf) AND that answerChallenge still calls\n"+
		"  challengeIsAddressedToMe — dropping EITHER restores the oracle, and dropping the sender's half is the quieter of the two,\n"+
		"  because the prover-side check then silently has nothing to check against and every challenge is answered verbatim again.\n"+
		"  IF THE DELTA IS POSITIVE, the grade moved but the payment path did not, which is the half this assertion exists to catch.\n"+
		"  IF Failed IS 0 WITH Passed 0, the answer never reached the grade at all; find what dropped it before reading this as a\n"+
		"  refusal, because an unreachable prover and a refusing one are not the same result.",
		r, minted)
}

func TestChallengeOutsourcingIsRefusedByTheProver(t *testing.T) {
	auditor, _ := aloneNode(t, 0)
	led := credit.New(1, 500_000)
	auditor.SetLedger(led)

	base := porChallengeSeed(7777)
	id, data, sp, objectRoot, shardRoot, _ := spotShard(t)
	want := shardLeaves(pipeline.DefaultChunkSize, por.SpotLeafBytes)

	// A is a REAL node that really holds the shard and its proof. It is given a
	// distinct identity, because the defence turns on WHOSE seed a prover is
	// answering and a fixture where every node shares aloneNode's id cannot tell
	// the honest case from the attack. A is never driven over the network here —
	// answerChallenge is called directly — so re-identifying it after construction
	// touches nothing else.
	holderA, _ := aloneNode(t, 0)
	holderIDA := ports.HashBytes([]byte("real-holder-A"))
	holderA.id = holderIDA
	if err := holderA.store.Put(bg(), ports.Chunk{ID: id, Data: data}); err != nil {
		t.Fatalf("A store: %v", err)
	}
	if err := holderA.proofs.Put(id, sp); err != nil {
		t.Fatalf("A proofs: %v", err)
	}

	// B announced itself as a provider and holds nothing.
	proxyB := ports.HashBytes([]byte("data-less-proxy-B"))
	bSeed := porProverSeed(base, proxyB)

	// THE AUDITOR'S MESSAGE TO B, FORWARDED BY B TO A VERBATIM — and "verbatim" is
	// why PorBase is here. auditLeaf sends the base beside the derived seed, so a
	// forwarded copy carries it too; a fixture that omitted it would be relaying a
	// message no auditor sends and would grade a strawman.
	relayed := holderA.answerChallenge(ports.Message{Kind: ports.MsgChallenge, ChunkID: id,
		PorSeed: bSeed[:], PorBase: base[:], PorCount: porSampleCount})

	// --- CONTROL: the RELAY attack (which porProverSeed DOES deny) must fail.
	// A answers ITS OWN legitimate challenge — seed bound to A, so A really does
	// open its leaves — and B returns that answer as its own. This is the arm that
	// proves the refusal above is about outsourcing and not about A having stopped
	// answering anything: A DOES answer here, and B still fails, because the leaves
	// opened are the ones A's seed named and B is graded under B's.
	aSeed := porProverSeed(base, holderIDA)
	relayOnly := holderA.answerChallenge(ports.Message{Kind: ports.MsgChallenge, ChunkID: id,
		PorSeed: aSeed[:], PorBase: base[:], PorCount: porSampleCount})
	if !relayOnly.Found {
		t.Fatal("CONTROL BROKEN: A refused its OWN identity-bound challenge — the prover-side check is rejecting honest " +
			"challenges, which would fail every honest audit while making the outsourcing arm below pass for the wrong reason")
	}
	if rc := gradeOne(t, auditor, id, objectRoot, shardRoot, base, want, proxyB, relayOnly); rc.Passed != 0 {
		t.Fatalf("CONTROL BROKEN: a plain RELAY of A's own-seed answer also passed (%+v) — "+
			"porProverSeed's identity binding is not holding, so this test is not "+
			"measuring challenge OUTSOURCING", rc)
	}
	// --- CONTROL: B with no proxy at all holds nothing and must fail.
	if rn := gradeOne(t, auditor, id, objectRoot, shardRoot, base, want, proxyB,
		ports.Message{Kind: ports.MsgChallengeReply}); rn.Passed != 0 {
		t.Fatal("CONTROL BROKEN: an empty reply was graded PASSED")
	}
	// --- CONTROL: A answering A's own challenge is graded PASSED for A. Without
	// this arm every assertion above is satisfiable by an audit that passes nobody.
	if ra := gradeOne(t, auditor, id, objectRoot, shardRoot, base, want, holderIDA, relayOnly); ra.Passed != 1 {
		t.Fatalf("CONTROL BROKEN: the real holder A was graded %+v for its own answer — "+
			"the audit fails honest holders and the refusals above mean nothing", ra)
	}

	before := led.Balance(proxyB)
	r := gradeOne(t, auditor, id, objectRoot, shardRoot, base, want, proxyB, relayed)
	minted := led.Balance(proxyB) - before
	t.Logf("grade of B's proxied answer: %+v; credit minted to the data-less proxy: %d "+
		"(A refused to be the oracle: ProxiedChallengesRefused=%d)", r, minted, holderA.Stats.ProxiedChallengesRefused)

	// THE RULE: the data-less proxy fails its audit and is paid nothing.
	if msg := por2Outsourcing(r, minted); msg != "" {
		t.Fatal(msg)
	}
	// AND THE REFUSAL IS ATTRIBUTED, not merely absent. A grade of FAILED could also
	// come from A being broken, offline, or holding nothing; the counter says A was
	// present, understood the challenge, and declined it because the seed named
	// somebody else.
	if holderA.Stats.ProxiedChallengesRefused != 1 {
		t.Fatalf("A recorded %d proxied-challenge refusals, want exactly 1 — the proxied answer graded FAILED for some other "+
			"reason, so this test is not measuring the outsourcing defence", holderA.Stats.ProxiedChallengesRefused)
	}
}

// ---------------------------------------------------------------------------
// GATE 5a — the Merkle leg binds to the AUDITED root.
//
// THE RULE, asserted positively rather than pinned in its absence: leg 1 proves the
// shard is the one the audit is asking about. The verifier used to take p.Root FROM
// THE RESPONSE and never compare it to the audited root, and with
// {Index:0, Total:1, Path:nil} manifest.VerifyProof reduces to leafHash(leaf)==root
// — so any party knowing only the chunk id satisfied leg 1 with zero knowledge.
// Both call sites now supply a root of their own: Node.auditLeaf threads the layout
// root it already computed for colKey, and Node.challengeHolderRetrievability uses
// the root it recomputes from the layout it loaded, never claim.Root (which the
// claimant supplies) and never the response's.
// ---------------------------------------------------------------------------

// TestForeignRootedProofIsRefused asserts three halves, and they must stay
// independently observable: leg 1 refusing in isolation, the composed grade
// failing, and — because a verifier that refused everything would satisfy both —
// the honest proof still passing.
func TestForeignRootedProofIsRefused(t *testing.T) {
	n, _ := aloneNode(t, 0)
	n.SetLedger(credit.New(1, 500_000))

	base := porChallengeSeed(31337)
	id, data, sp, objectRoot, shardRoot, leaves := spotShard(t)
	want := shardLeaves(pipeline.DefaultChunkSize, por.SpotLeafBytes)

	// A party that knows only the chunk id names its OWN root: a one-leaf tree over
	// the shard. Under the old reader this verified against itself and the leg passed.
	bogus := ports.StorageProof{Root: selfRoot(id), Index: 0, Total: 1, Path: nil,
		Column: -1, LeafBytes: por.SpotLeafBytes}
	if bogus.Root == objectRoot {
		t.Fatal("setup: the self-root collided with the audited root")
	}

	// LEG 1, IN ISOLATION. This is the assertion the remediation is about.
	if verifyStorageProofAgainst(bogus, id, objectRoot) {
		t.Fatalf("a self-rooted inclusion proof was accepted against the AUDITED root — the leg is still a "+
			"tautology.\n  audited root = %x, claimed root = %x", objectRoot[:6], bogus.Root[:6])
	}

	// NO OVER-REJECTION: the honest holder's real proof still passes leg 1 under the
	// same root. A verifier that refuses everything would satisfy the arm above.
	if !verifyStorageProofAgainst(sp, id, objectRoot) {
		t.Fatal("the honest inclusion proof was refused against its own audited root — the binding check " +
			"over-rejects, which fails the claim in the other direction")
	}

	// COMPOSED: a party carrying the self-rooted proof is graded FAILED even when it
	// GENUINELY HOLDS THE BYTES and opens every sampled leaf correctly. That is what
	// isolates this gate from gate 1: the retrievability legs all pass here, and the
	// answer still fails, because the shard it proved possession of is not bound to
	// the object under audit.
	rooter := ports.HashBytes([]byte("zero-knowledge-rooter"))
	answer := openReply(t, data, sp, base, rooter, leaves)
	answer.Proof = &bogus
	r := gradeOne(t, n, id, objectRoot, shardRoot, base, want, rooter, answer)
	if r.Passed != 0 || r.Failed != 1 {
		t.Fatalf("a self-rooted proof was not graded FAILED by the composed grade: %+v", r)
	}
}

// ---------------------------------------------------------------------------
// THE TEETH. Every assertion above is a pure predicate, and this test feeds each one
// the value it is supposed to FIRE on and the value it is supposed to accept.
//
// Why this exists: an assertion that cannot fire reports green forever and is
// indistinguishable from one that is working. It matters more now than when these
// were pins: a pin's polarity made a silent one visible the day it was remediated,
// and a positive assertion has no such day. The recorded PRE-FIX grades are the
// values fed here, so each predicate is shown to still see the defect it replaced.
//
// THIS TEST IS NOT A PIN. Its polarity is ordinary: GREEN when the assertions work,
// RED when one of them stops being able to fire.
// ---------------------------------------------------------------------------

func TestRetrievabilityAssertionsFireOnTheirDefects(t *testing.T) {
	// The two shapes: what a working audit produces, and what each defect produced
	// when it was live.
	fixed := AuditReport{Challenges: 1, Passed: 0, Failed: 1, NoTruth: 1}
	broken := AuditReport{Challenges: 1, Passed: 1}

	// --- rtPOR1, which replaced the care-link forgery pin.
	if msg := rtPOR1(fixed, -25000); msg != "" {
		t.Fatalf("rtPOR1 fired on the state it now asserts (zero-byte holder FAILED and slashed): %s", msg)
	}
	if rtPOR1(broken, 25000) == "" {
		t.Fatal("rtPOR1 stayed silent on the PRE-FIX grade (Passed=1, 25000 minted) — the assertion cannot see the " +
			"key-distribution break coming back, which is the only thing it is here to do")
	}
	if rtPOR1(fixed, 25000) == "" {
		t.Fatal("rtPOR1 stayed silent on a FAILED grade that still minted credit — the payment half of the assertion is dead")
	}

	// --- the V6 firewall arm. THIS IS THE REGRESSION TEST FOR THE FALSE
	// ESCALATION. A working audit makes the data-less prover FAIL, auditsFailed
	// increments and Reputation goes NEGATIVE — measured at -250. The arm must stay
	// SILENT there. An earlier draft used `rep != 0`, which fired an M0 ESCALATE on
	// precisely the success path and suppressed the guidance above it.
	if msg := rtPOR1V6Firewall(-250); msg != "" {
		t.Fatalf("rtPOR1V6Firewall fired on a NEGATIVE reputation (-250), which is the prover being "+
			"correctly PENALISED — the firewall working, not a breach: %s", msg)
	}
	if msg := rtPOR1V6Firewall(0); msg != "" {
		t.Fatalf("rtPOR1V6Firewall fired on a zero reputation: %s", msg)
	}
	if rtPOR1V6Firewall(250) == "" {
		t.Fatal("rtPOR1V6Firewall stayed silent on a POSITIVE reputation — a forged PoR pass buying " +
			"Sybil-resistant standing would reach the gamma->1/N firewall unnoticed")
	}

	// --- por2Outsourcing.
	if msg := por2Outsourcing(fixed, -25000); msg != "" {
		t.Fatalf("por2Outsourcing fired on the state it asserts (proxy graded FAILED and slashed, never paid): %s", msg)
	}
	if por2Outsourcing(broken, 25000) == "" {
		t.Fatal("por2Outsourcing stayed silent on the PRE-FIX grade (proxy Passed=1, 25000 minted) — the assertion cannot " +
			"see the defect coming back")
	}

	// GATE 5a carries no predicate: TestForeignRootedProofIsRefused asserts leg 1
	// in isolation AND the composed grade, so a composed FAIL alone cannot pass it
	// off as a root binding that landed. Nothing to feed here.
}

// selfRoot is the zero-knowledge Merkle root a party knowing only a chunk id can
// name for it: RFC 6962 leafHash(id), which manifest.VerifyProof recomputes from
// {Index:0, Total:1, Path:nil}.
func selfRoot(id ports.ChunkID) ports.Hash {
	return manifest.MerkleRoot([]ports.Hash{ports.Hash(id)})
}

// cheatReply is the answer from a holder that dropped ONE leaf's bytes and kept the
// shard's whole leaf-hash list. It is the economically rational cheater: retaining
// 32 bytes per leaf lets it build an honest Merkle path for everything it still
// holds, so it is caught only on a round that samples what it dropped.
func cheatReply(t *testing.T, data []byte, hashes []ports.Hash, dropped int,
	sp ports.StorageProof, idx []int, leaves int) ports.Message {
	t.Helper()
	m := ports.Message{Kind: ports.MsgChallengeReply, Found: true, Proof: &sp, PorBlocks: leaves}
	for _, i := range idx {
		p, err := manifest.Prove(hashes, i)
		if err != nil {
			t.Fatalf("cheat prove %d: %v", i, err)
		}
		leaf := make([]byte, por.SpotLeafBytes)
		if i != dropped {
			lo := i * por.SpotLeafBytes
			hi := lo + por.SpotLeafBytes
			if hi > len(data) {
				hi = len(data)
			}
			leaf = append([]byte(nil), data[lo:hi]...)
		}
		buf := make([]byte, 0, len(p.Path)*32)
		for _, h := range p.Path {
			buf = append(buf, h[:]...)
		}
		m.PorOpen = append(m.PorOpen, leaf)
		m.PorPaths = append(m.PorPaths, buf)
	}
	return m
}
