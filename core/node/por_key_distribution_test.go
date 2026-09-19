package node

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/ports"
)

// The PoR key-distribution break, landed as three PINNED_DEFECT gates at the COMPOSED
// level: the oracle is gradeAnswers' full three-leg grade, never por.Key.Verify alone.
//
// ⚠⚠ READ THE POLARITY BEFORE YOU READ A RESULT.
//
// These three tests assert the tree's CURRENT, BROKEN behaviour. They are GREEN
// while the break is present and go RED when it is FIXED. A RED here is not a
// regression; it is the pin doing its job. It is the same posture as core/pipeline's
// TestSingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT and
// TestEntryFileSizeIsTheExactByteCount_PINNED_DEFECT. Skip-until-fixed is refused: a t.Skip
// costs the same lines and reports green either way.
//
// THE POLARITY WAS FLIPPED WHEN THESE GATES WERE PINNED. Before the flip the tests asserted
// the INTENDED rule and all three were RED; the ablation ran before the conversion below.
// Under the pinned polarity, GREEN means the
// forgery still works and the research still stands. The falsifier's modern form is the
// inverse: if a pin below goes RED, the break it holds has been remediated, and the V1/V2/V3
// verdicts and the remediation pricing must be re-read before the pin is retired.
//
// A pin records behaviour. It does NOT ratify that behaviour as correct and it
// does not price a fix; the remediation options are the, not this file's.

// ---------------------------------------------------------------------------
// The forger. It is deliberately an INDEPENDENT reimplementation of the three
// derivations a care-link holder needs. It imports nothing unexported from
// core/por and it NEVER calls por.Tags or por.Prove. Its only inputs are the
// care link's layout key, the chunk id, the seed it was handed, and the block
// count — no file bytes exist anywhere in its call graph.
// ---------------------------------------------------------------------------

var forgePrime = func() *big.Int {
	p := new(big.Int).Lsh(big.NewInt(1), 255)
	return p.Sub(p, big.NewInt(19))
}()

// forgePRFKey re-derives por.Key's PRF key from the care link's layout key:
// crypto.DeriveKey(layoutKey, "silt/por/v1") seeds por.DeriveKey, whose
// detReader hands Keygen its first 32 bytes as k.prf.
func forgePRFKey(layoutKey [32]byte) [32]byte {
	seed := crypto.DeriveKey(layoutKey, porKeyDomain)
	mac := hmac.New(sha256.New, seed[:])
	mac.Write([]byte("silt/por/v1/derive"))
	var cb [4]byte
	binary.BigEndian.PutUint32(cb[:], 0)
	mac.Write(cb[:])
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func forgePRFBytes(seed [32]byte, domain string, ctr uint32) [32]byte {
	mac := hmac.New(sha256.New, seed[:])
	mac.Write([]byte("silt/por/v1/"))
	mac.Write([]byte(domain))
	var cb [4]byte
	binary.BigEndian.PutUint32(cb[:], ctr)
	mac.Write(cb[:])
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func forgeBlockTerm(prfKey [32]byte, unitID []byte, i int) *big.Int {
	mac := hmac.New(sha256.New, prfKey[:])
	mac.Write([]byte("silt/por/v1/blk"))
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(unitID)))
	mac.Write(lb[:])
	mac.Write(unitID)
	var ib [8]byte
	binary.BigEndian.PutUint64(ib[:], uint64(i))
	mac.Write(ib[:])
	return new(big.Int).Mod(new(big.Int).SetBytes(mac.Sum(nil)), forgePrime)
}

// forgeZeroMu is forgery: every Mu[j] = 32 zero bytes, so Σⱼ αⱼ·μⱼ vanishes
// and σ is just Σᵢ νᵢ·f_k(i), which the key holder computes directly. Returns
// the proof plus the number of field multiplications it performed, for the
// asymmetry table.
func forgeZeroMu(layoutKey [32]byte, unitID []byte, seed [32]byte, blocks, count int) (por.Proof, int) {
	prfKey := forgePRFKey(layoutKey)
	if count > blocks {
		count = blocks
	}
	sigma := new(big.Int)
	tmp := new(big.Int)
	used := make(map[int]bool, count)
	mults := 0
	var ctr uint32
	for n := 0; n < count; {
		idx := int(binary.BigEndian.Uint64(sliceOf(forgePRFBytes(seed, "index", ctr))) % uint64(blocks))
		ctr++
		if used[idx] {
			continue
		}
		used[idx] = true
		nu := new(big.Int).Mod(new(big.Int).SetBytes(sliceOf(forgePRFBytes(seed, "coeff", uint32(idx)))), forgePrime)
		if nu.Sign() == 0 {
			nu.SetInt64(1)
		}
		tmp.Mul(nu, forgeBlockTerm(prfKey, unitID, idx))
		mults++
		sigma.Add(sigma, tmp)
		sigma.Mod(sigma, forgePrime)
		n++
	}
	mu := make([][]byte, por.DefaultParams.SectorsPerBlock)
	for j := range mu {
		mu[j] = make([]byte, por.ElemBytes) // 32 zero bytes
	}
	sig := make([]byte, por.ElemBytes)
	sigma.FillBytes(sig)
	return por.Proof{Mu: mu, Sigma: sig}, mults
}

func sliceOf(a [32]byte) []byte { return a[:] }

func sliceOfSeed(a [32]byte) []byte { return a[:] }

// selfRoot is the zero-knowledge Merkle root a forger names for leaf id:
// RFC6962 leafHash(id), which VerifyProof recomputes from {Index:0,Total:1,
// Path:nil}. It requires nothing but the chunk id.
func selfRoot(id ports.ChunkID) ports.Hash {
	h := sha256.New()
	h.Write([]byte{0x00})
	h.Write(id[:])
	var out ports.Hash
	h.Sum(out[:0])
	return out
}

// gradeOne runs the PRODUCT's composed three-leg grade over exactly one
// answer, building the `valid` leg the way auditLeaf builds it (por.go)
// — via the product's own verifyStorageProofAgainst, under the AUDITED root the
// caller supplies — so no leg is asserted by the test.
func gradeOne(t *testing.T, n *Node, id ports.ChunkID, root ports.Hash, porKey *por.Key, base [32]byte,
	want int, prover ports.NodeID, resp ports.Message) AuditReport {
	t.Helper()
	valid := resp.Found && resp.Proof != nil && verifyStorageProofAgainst(*resp.Proof, id, root)
	answers := []challengeAnswer{{
		prover: prover, valid: valid, blocks: resp.PorBlocks,
		proof: por.Proof{Mu: resp.PorMu, Sigma: resp.PorSigma},
	}}
	var report AuditReport
	report.Challenges++
	fired := false
	n.gradeAnswers(id, porKey, base, want, answers, &report, func() { fired = true })
	if !fired {
		t.Fatal("gradeAnswers did not call done")
	}
	return report
}

// honestShard builds a real 256 KiB-frame shard, its tags, and a real
// inclusion proof under a 4-leaf root.
func honestShard(t *testing.T, porKey *por.Key) (id ports.ChunkID, data []byte, tags [][]byte, sp ports.StorageProof, root ports.Hash) {
	t.Helper()
	data = make([]byte, pipeline.DefaultChunkSize+ctOverhead)
	for i := range data {
		data[i] = byte(i*167 + 29)
	}
	id = ports.HashBytes(data)
	leaves := []ports.Hash{
		ports.HashBytes([]byte("sibling-0")), id,
		ports.HashBytes([]byte("sibling-2")), ports.HashBytes([]byte("sibling-3")),
	}
	root = manifest.MerkleRoot(leaves)
	mp, err := manifest.Prove(leaves, 1)
	if err != nil {
		t.Fatalf("merkle prove: %v", err)
	}
	tags = porKey.Tags(id[:], data)
	sp = ports.StorageProof{Root: root, Index: mp.Index, Total: mp.Total, Path: mp.Path, Column: -1, PorTags: tags}
	return
}

// ---------------------------------------------------------------------------
// GATE 1 — a care-link holder storing ZERO bytes PASSES the composed audit.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: only a party that actually holds the
// bytes should pass a proof-of-retrievability audit. The PoR verification key
// rides the care link (link.Handle.Care → LayoutKey → DerivePorKey), so every
// caretaker, every full-link reader and the publisher can set Mu=0 and solve for
// Sigma directly. Shacham-Waters Definition 2.1 soundness assumes the key is
// "not known to the prover or other parties"; here it is a published capability.
//
// PIN. GREEN today. It goes RED when key distribution is remediated.
//
// ADVERSARY-HOLDS: LayoutKey -- the forger is handed the care link's layout key
// (forgeZeroMu(layoutKey,.)) and holds ZERO file bytes and ZERO tags. It is the
// capability-holding fixture row F1 asks for, on the storage-proof surface.
// CAPABILITY-CONTROL: LayoutKey -- the wrongKey arm below runs the SAME forgery
// WITHOUT the layout key and asserts it is graded Passed=0. That is what makes the
// capability load-bearing rather than incidental.
// ---------------------------------------------------------------------------

// rtPOR1Pin returns "" while a care-link holder storing ZERO bytes still passes the
// composed three-leg grade and is paid for it, and the instruction once it does not.
// TEETH: TestRetrievabilityPinsFireOnTheirRemediations.
func rtPOR1Pin(fr AuditReport, minted int64) string {
	if fr.Passed == 1 && fr.Failed == 0 && minted > 0 {
		return ""
	}
	return fmt.Sprintf("PIN IS RED — a care-link holder storing ZERO bytes was graded %+v and minted %d credit; "+
		"pinned at Passed=1, Failed=0 and a positive mint.\n"+
		"  THE FIX CASE: Passed=0 means a zero-byte forger no longer satisfies the composed three-leg audit — the PoR key no longer\n"+
		"  rides the care link, or the grade no longer accepts a proof the key holder can solve for. That is the remediation this pin was\n"+
		"  waiting for. Before retiring it: re-read the rule\n"+
		"  POR-KEY-DISTRIBUTION-BREAK-REMEDIATION-OPTIONS-75c0f89-RESEARCH-CERTIFICATION-2026-09-12.md (V1/V2/V3 and the remediation\n"+
		"  pricing addendum), confirm WHICH option shipped, and replace this pin with the positive assertion that a prover without the\n"+
		"  bytes fails. Check the positive control above still passes, so the fix is not simply over-rejection.\n"+
		"  THE COUPLED CASE — READ THIS BEFORE YOU CONCLUDE THE KEY MOVED. This pin is NOT independent of. The forgery above\n"+
		"  also rides leg 1, which is a tautology today (verifyStorageProof takes p.Root from the response), so binding leg 1 to the\n"+
		"  AUDITED root reddens this pin without touching key distribution at all. Measured: ablating EITHER the key derivation or leg 1\n"+
		"  reddens BOTH pins. Read in the SAME run before attributing this RED: 5a red with isolation=false is the leg-1 fix;\n"+
		"  5a still accepting the self-rooted proof in isolation means the key distribution is what moved.\n"+
		"  THE OTHER CASE: Passed=1 with a zero mint means the grade still accepts the forgery but the payment path changed; the audit\n"+
		"  break is untouched and only the economics moved. Re-derive which before re-pinning — this pin is about the GRADE.",
		fr, minted)
}

// rtPOR1V6Firewall returns "" while a forged audit pass buys no Sybil-resistant
// STANDING, and the escalation once it does.
//
// THE PREDICATE IS rep > 0, NOT rep != 0, AND THE DIFFERENCE IS THE WHOLE POINT.
// credit.Ledger.Reputation is bonded bytes MINUS four penalty counters
// (auditsFailed, bondFails, falseRepairs, equivocations). The moment any
// remediation makes the forger FAIL its audit — which is the success path this
// whole file exists to produce — auditsFailed increments and the reputation goes
// NEGATIVE (measured under both ablations: -250). A "!= 0" test therefore fires a
// false M0 escalation exactly when the system is working. The property being
// asserted is "a forged PASS buys no STANDING", and that is rep > 0.
// TEETH: TestRetrievabilityPinsFireOnTheirRemediations.
func rtPOR1V6Firewall(rep int64) string {
	if rep <= 0 {
		return ""
	}
	return fmt.Sprintf("V6 FIREWALL BREACHED — the zero-byte forger's Reputation is %d after a forged audit pass, want <= 0. "+
		"This is NOT the pinned defect widening quietly; it is a different and worse finding: a forged PoR pass now buys "+
		"Sybil-resistant standing, which reaches the gamma->1/N firewall and M0. credit.Reputation never reads auditsPassed — "+
		"its only POSITIVE term is bonded bytes, and everything else it reads is a penalty counter — so a POSITIVE reputation "+
		"here cannot have come from the forged pass unless that changed. ESCALATE before touching this file.", rep)
}

func TestCareLinkHolderForgesWithZeroBytes_PINNED_DEFECT(t *testing.T) {
	n, _ := aloneNode(t, 0)
	led := credit.New(1, 500_000)
	n.SetLedger(led)

	layoutKey := linkLayoutKey(t) // the value a CARE LINK carries, by design
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(4242)

	id, data, tags, sp, auditedRoot := honestShard(t, porKey)
	want := por.DefaultParams.Blocks(pipeline.DefaultChunkSize + ctOverhead)
	if want != len(tags) {
		t.Fatalf("setup: auditor wants %d blocks, honest shard has %d", want, len(tags))
	}
	t.Logf("regime: shard=%d B, want=%d por-blocks, sample=%d (p=%.4f)",
		len(data), want, min(porSampleCount, want), float64(min(porSampleCount, want))/float64(want))

	// --- POSITIVE CONTROL: an honest holder of the bytes PASSES. ---
	honest := ports.HashBytes([]byte("honest-holder"))
	hSeed := porProverSeed(base, honest)
	t0 := time.Now()
	hp, err := por.Prove(por.DefaultParams, data, tags, porChallenge(hSeed, want, porSampleCount))
	if err != nil {
		t.Fatalf("honest prove: %v", err)
	}
	honestNs := time.Since(t0).Nanoseconds()
	hr := gradeOne(t, n, id, auditedRoot, porKey, base, want, honest, ports.Message{
		Kind: ports.MsgChallengeReply, Found: true, Proof: &sp,
		PorBlocks: want, PorMu: hp.Mu, PorSigma: hp.Sigma,
	})
	if hr.Passed != 1 {
		t.Fatalf("FIXTURE BROKEN: an honest holder of every byte was graded %+v — "+
			"the forgery result below would be meaningless", hr)
	}

	// --- THE FORGERY: care link only. No data. No tags. Tags never called. ---
	forger := ports.HashBytes([]byte("care-link-holder-with-no-disk"))
	fSeed := porProverSeed(base, forger)
	t1 := time.Now()
	fp, mults := forgeZeroMu(layoutKey, id[:], fSeed, want, porSampleCount)
	forgeNs := time.Since(t1).Nanoseconds()
	// THE FORGER CARRIES THE HONEST INCLUSION PROOF, AND THAT IS WHAT MAKES THIS PIN
	// ABOUT KEY DISTRIBUTION RATHER THAN ABOUT LEG 1. It used to carry a self-rooted
	// one-leaf proof, which leg 1 accepted while leg 1 was a tautology — so binding leg
	// 1 to the audited root reddened this pin without touching the PoR key at all,
	// exactly the coupling the FIX CASE below warns about. Leg 1 now binds, so the
	// forger is handed `sp`: a real, audited-root inclusion proof, which any care-link
	// holder can derive from the layout it is entitled to read. Leg 1 therefore passes
	// honestly and the grade turns purely on whether a party with the key but no bytes
	// can satisfy the PoR equation. That is the break this pin exists to hold, now
	// isolated from the one next door.
	fsp := sp

	before := led.Balance(forger)
	fr := gradeOne(t, n, id, auditedRoot, porKey, base, want, forger, ports.Message{
		Kind: ports.MsgChallengeReply, Found: true, Proof: &fsp,
		PorBlocks: want, PorMu: fp.Mu, PorSigma: fp.Sigma,
	})
	minted := led.Balance(forger) - before

	t.Logf("MEASURED asymmetry: honest prove %d ns over %d B read, %d field mults; "+
		"forge %d ns over 0 B read, %d field mults (ratio %.1f× work, %.1f× time)",
		honestNs, len(data), min(porSampleCount, want)*por.DefaultParams.SectorsPerBlock+min(porSampleCount, want),
		forgeNs, mults,
		float64(min(porSampleCount, want)*por.DefaultParams.SectorsPerBlock+min(porSampleCount, want))/float64(mults),
		float64(honestNs)/float64(forgeNs))
	t.Logf("grade of the forged answer: %+v; credit minted to the forger: %d", fr, minted)

	// --- NEGATIVE CONTROL: the SAME attack WITHOUT the care link must fail. ---
	var wrongKey [32]byte
	wrongKey[0] = 0x99
	wp, _ := forgeZeroMu(wrongKey, id[:], fSeed, want, porSampleCount)
	wr := gradeOne(t, n, id, auditedRoot, porKey, base, want,
		ports.HashBytes([]byte("no-care-link")), ports.Message{
			Kind: ports.MsgChallengeReply, Found: true, Proof: &fsp,
			PorBlocks: want, PorMu: wp.Mu, PorSigma: wp.Sigma,
		})
	if wr.Passed != 0 {
		t.Fatalf("CONTROL BROKEN: a forger WITHOUT the layout key also passed (%+v) — "+
			"then the finding is not about key distribution", wr)
	}

	// PIN: the zero-byte forger is graded PASSED and is paid for it.
	if msg := rtPOR1Pin(fr, minted); msg != "" {
		t.Fatal(msg)
	}

	// V6 of the research: the gamma->1/N firewall. THIS ARM IS NOT A PIN — it
	// asserts the INTENDED rule and passes because the rule HOLDS. A forged
	// PASS mints spendable credit but no Sybil-resistant STANDING, because
	// credit.Reputation never reads auditsPassed. It is asserted, not merely
	// logged, so that the pinned break above cannot silently widen from a
	// credit mint into a standing mint without a RED.
	//
	// IT RUNS AFTER THE PIN, DELIBERATELY. Ordered before it, this arm t.Fatalf'd
	// first on every remediation, so the pin's FIX CASE guidance — the entire point
	// of the pin — was unreachable on the success path. Keep it last.
	if msg := rtPOR1V6Firewall(led.Reputation(forger)); msg != "" {
		t.Fatal(msg)
	}
	t.Logf("V6 firewall HOLDS: forger Reputation after the forged pass = %d (its only positive term is bonded bytes; auditsPassed is never read)",
		led.Reputation(forger))
}

// ---------------------------------------------------------------------------
// GATE 2 — challenge OUTSOURCING. B holds no bytes; it forwards the auditor's
// verbatim MsgChallenge to a real holder A and returns A's answer as its own.
//
// THE RULE, now ASSERTED rather than pinned absent: a prover cannot have someone
// else answer its identity-bound challenge. It was absent because answerChallenge
// took no view of WHO the challenge was addressed to, so A computed under B's seed
// on request and could not tell. The close is the auditor sending the UNBOUND base
// beside the derived seed and A folding in its OWN id before answering.
//
// THE PIN PREDICTED `from` AND `from` WOULD HAVE BEEN WRONG. Its fix-case pointed at
// Node.handle's MsgChallenge case having `from` in scope. But `from` is the
// FORWARDER — B — so binding to it would have authorised the attack precisely. The
// identity that must reach the prover is SELF. Recorded because the pin's own
// guidance was the misleading part, and the next reader deserves to know the
// prediction was checked rather than followed.
//
// CONVERTED FROM A PIN, 2026-09-19. It is now an ordinary positive assertion.
//
// ADVERSARY-HOLDS: HonestHolderAsOracle -- B is granted a real holder A that HAS the
// bytes, HAS the tags and is WILLING to compute on request, which is the capability
// the defence assumes no adversary has. A is still willing here; what changed is
// that it now declines a seed naming somebody else, which is the thing under test.
// CAPABILITY-CONTROL: HonestHolderAsOracle -- the relay-only arm below removes the
// oracle (A answers its OWN challenge, genuinely computing, and B forwards that) and
// asserts Passed=0; the empty-reply arm asserts B alone passes nothing.
//
// ⚠ THE CONTROL DISCRIMINATED WHILE THE DEFENCE WAS BROKEN, AND NO LONGER DOES.
// Pre-fix: capability present -> B PASSED and was paid 1000; capability removed ->
// B failed. That is a textbook capability control and it is the evidence that the
// oracle was load-bearing. Post-fix both arms fail, which is the structural state
// this gate's docstring describes for a defence that HOLDS. The discrimination is
// not lost, it is HISTORICAL, and it stays checkable: the teeth test feeds
// por2Outsourcing the recorded pre-fix grade (Passed=1, 25000 minted) and requires
// it to fire. That is why this stays `fixture=` rather than joining the UNCOVERED
// backlog — the capability was granted AND shown to decide the outcome.
// ---------------------------------------------------------------------------

// por2Outsourcing: THE POSITIVE ASSERTION THAT REPLACED THE PIN, 2026-09-19.
// The pin it replaces recorded that a data-less identity B could forward the
// auditor's verbatim challenge to a real holder A, have A compute under B's OWN
// seed, and return the answer as its own — graded Passed=1 and paid 1000 credit
// while holding zero bytes. It went red when the prover identity reached the
// prover, which is what a pin is for.
//
// The shape that landed is the one the pin predicted, with one correction it could
// not have known: the identity that must reach answerChallenge is SELF, not `from`.
// `from` is the FORWARDER, so binding to it would have authorised exactly the attack.
// The auditor now sends the UNBOUND base beside the derived seed and the prover
// folds its own id in and compares, so a challenge addressed to somebody else is
// refused without the prover having to trust anything in the message.
//
// The rule asserted now: the proxied answer is graded FAILED and the proxy is NOT
// PAID for it.
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
// this sweep at all; it only declined to compute.
// TEETH: TestRetrievabilityPinsFireOnTheirRemediations.
func por2Outsourcing(r AuditReport, minted int64) string {
	if r.Passed == 0 && r.Failed == 1 && minted <= 0 {
		return ""
	}
	return fmt.Sprintf("OUTSOURCING IS NO LONGER REFUSED — a data-less identity B that forwarded the auditor's verbatim challenge to "+
		"holder A was graded %+v and its balance moved by %d; want Passed=0, Failed=1 and a delta that is not positive.\n"+
		"  IF Passed IS 1, THE ORIGINAL DEFECT IS BACK: A computed under a seed bound to B and could not tell it was answering for\n"+
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

	layoutKey := linkLayoutKey(t)
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(7777)

	id, data, tags, sp, auditedRoot := honestShard(t, porKey)
	want := por.DefaultParams.Blocks(pipeline.DefaultChunkSize + ctOverhead)

	// A is a REAL node that really holds the shard and its proof. It is given a
	// distinct identity, because the defence now turns on WHOSE seed a prover is
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
	_ = tags

	// B announced itself as a provider and holds nothing.
	proxyB := ports.HashBytes([]byte("data-less-proxy-B"))
	bSeed := porProverSeed(base, proxyB)

	// THE AUDITOR'S MESSAGE TO B, FORWARDED BY B TO A VERBATIM — and "verbatim" is
	// why PorBase is here. auditLeaf sends the base beside the derived seed, so a
	// forwarded copy carries it too; a fixture that omitted it would be relaying a
	// message no auditor sends and would grade a strawman.
	challenge := ports.Message{Kind: ports.MsgChallenge, ChunkID: id,
		PorSeed: bSeed[:], PorBase: base[:], PorCount: porSampleCount}
	relayed := holderA.answerChallenge(challenge)

	// --- CONTROL: the RELAY attack (which porProverSeed DOES deny) must fail.
	// A answers ITS OWN legitimate challenge — seed bound to A, so A really does
	// compute — and B returns that answer as its own. This is the arm that proves
	// the refusal above is about outsourcing and not about A having stopped
	// answering anything: A DOES answer here, and B still fails, because the proof
	// is bound to A's seed and B is graded under B's.
	relayOnly := holderA.answerChallenge(ports.Message{
		Kind: ports.MsgChallenge, ChunkID: id,
		PorSeed: sliceOfSeed(porProverSeed(base, holderIDA)), PorBase: base[:], PorCount: porSampleCount,
	})
	if !relayOnly.Found {
		t.Fatal("CONTROL BROKEN: A refused its OWN identity-bound challenge — the prover-side check is rejecting honest " +
			"challenges, which would fail every honest audit while making the outsourcing arm below pass for the wrong reason")
	}
	if rc := gradeOne(t, auditor, id, auditedRoot, porKey, base, want, proxyB, relayOnly); rc.Passed != 0 {
		t.Fatalf("CONTROL BROKEN: a plain RELAY of A's own-seed proof also passed (%+v) — "+
			"porProverSeed's identity binding is not holding, so this test is not "+
			"measuring challenge OUTSOURCING", rc)
	}
	// --- CONTROL: B with no proxy at all holds nothing and must fail.
	if rn := gradeOne(t, auditor, id, auditedRoot, porKey, base, want, proxyB,
		ports.Message{Kind: ports.MsgChallengeReply}); rn.Passed != 0 {
		t.Fatal("CONTROL BROKEN: an empty reply was graded PASSED")
	}

	before := led.Balance(proxyB)
	r := gradeOne(t, auditor, id, auditedRoot, porKey, base, want, proxyB, relayed)
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
// GATE 5a — CLOSED. The Merkle leg binds to the audited root.
//
// THE RULE, now asserted positively rather than pinned in its absence: leg 1 proves
// the shard is the one the audit is asking about. The verifier used to take p.Root
// FROM THE RESPONSE and never compare it to the audited root, and with
// {Index:0, Total:1, Path:nil} manifest.VerifyProof reduces to leafHash(leaf)==root
// — so any party knowing only the chunk id satisfied leg 1 with zero knowledge.
// Both call sites now supply a root of their own: Node.auditLeaf threads the layout
// root it already computed for colKey, and Node.challengeHolderRetrievability uses
// the root it recomputes from the layout it loaded, never claim.Root (which the
// claimant supplies) and never the response's.
// ---------------------------------------------------------------------------

// TestForeignRootedProofIsRefused asserts the rule the gate above now holds, in the
// place a pin used to record its absence.
//
// IT ASSERTS TWO HALVES, AND THEY MUST STAY INDEPENDENTLY OBSERVABLE: leg 1 refusing
// in isolation, and the composed grade failing. A composed FAIL alone would not prove
// the root binding landed, because a later leg can produce one on its own — which is
// why the isolation arm is asserted first and separately. The no-over-rejection arm
// is the third: a verifier that refused everything would satisfy the first two.
func TestForeignRootedProofIsRefused(t *testing.T) {
	n, _ := aloneNode(t, 0)
	n.SetLedger(credit.New(1, 500_000))

	layoutKey := linkLayoutKey(t)
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(31337)
	id, _, tags, sp, auditedRoot := honestShard(t, porKey)
	want := len(tags)

	// A party that knows only the chunk id names its OWN root: a one-leaf tree over
	// the shard. Under the old reader this verified against itself and the leg passed.
	bogus := ports.StorageProof{Root: selfRoot(id), Index: 0, Total: 1, Path: nil, Column: -1}
	if bogus.Root == auditedRoot {
		t.Fatal("setup: the self-root collided with the audited root")
	}

	// LEG 1, IN ISOLATION. This is the assertion the remediation is about.
	if verifyStorageProofAgainst(bogus, id, auditedRoot) {
		t.Fatalf("a self-rooted inclusion proof was accepted against the AUDITED root — the leg is still a "+
			"tautology.\n  audited root = %x, claimed root = %x", auditedRoot[:6], bogus.Root[:6])
	}

	// NO OVER-REJECTION: the honest holder's real proof still passes leg 1 under the
	// same root. A verifier that refuses everything would satisfy the arm above.
	if !verifyStorageProofAgainst(sp, id, auditedRoot) {
		t.Fatal("the honest inclusion proof was refused against its own audited root — the binding check " +
			"over-rejects, which fails the claim in the other direction")
	}

	// COMPOSED: the same self-rooted proof carried by a zero-byte forger is graded
	// FAILED rather than passed, and mints nothing.
	fSeedProver := ports.HashBytes([]byte("zero-knowledge-rooter"))
	fp, _ := forgeZeroMu(layoutKey, id[:], porProverSeed(base, fSeedProver), want, porSampleCount)
	r := gradeOne(t, n, id, auditedRoot, porKey, base, want, fSeedProver, ports.Message{
		Kind: ports.MsgChallengeReply, Found: true, Proof: &bogus,
		PorBlocks: want, PorMu: fp.Mu, PorSigma: fp.Sigma,
	})
	if r.Passed != 0 || r.Failed != 1 {
		t.Fatalf("a self-rooted proof was not graded FAILED by the composed grade: %+v", r)
	}
}

// ---------------------------------------------------------------------------
// THE TEETH. Every pin above is a pure predicate, and this test feeds each one the
// value it is supposed to FIRE on and the value it is supposed to accept.
//
// Why this exists: a pin that cannot fire reports green forever and is
// indistinguishable from a pin that is working. Ablating the product to prove a pin
// reddens is the right evidence, but it is not encoded here and does not
// survive a context reset. This is the same shape shipped with
// (TestPinRedensWhenFileSizeIsBlinded) and with the shipped-default lane's
// TheStockArgvGuardHasTeeth.
//
// THIS TEST IS NOT A PIN. Its polarity is ordinary: it is GREEN when the pins work
// and RED when one of them stops being able to fire. It must keep passing after the
// pins above are retired and replaced with positive assertions — at which point
// delete it in the same commit.
// ---------------------------------------------------------------------------

func TestRetrievabilityPinsFireOnTheirRemediations(t *testing.T) {
	pinned := AuditReport{Challenges: 1, Passed: 1}
	// The shape every remediation produces: the forgery is graded FAILED.
	fixed := AuditReport{Challenges: 1, Passed: 0, Failed: 1, NoTruth: 1}

	// --- --
	if msg := rtPOR1Pin(pinned, 25000); msg != "" {
		t.Fatalf("rtPOR1Pin fired on the state it is pinned to accept: %s", msg)
	}
	if rtPOR1Pin(fixed, -25000) == "" {
		t.Fatal("rtPOR1Pin stayed silent when the zero-byte forger was graded FAILED — the pin cannot " +
			"detect the key-distribution remediation it exists to detect")
	}
	if rtPOR1Pin(pinned, 0) == "" {
		t.Fatal("rtPOR1Pin stayed silent on a PASS that minted nothing — the payment half of the pin is dead")
	}

	// --- the V6 firewall arm. THIS IS THE REGRESSION TEST FOR THE FALSE
	// ESCALATION. A remediation makes the forger fail, auditsFailed increments and
	// Reputation goes NEGATIVE — measured at -250 under both ablations. The arm must
	// stay SILENT there. An earlier draft used `rep != 0`, which fired an M0 ESCALATE
	// on precisely the success path and suppressed the pin's own FIX CASE guidance.
	if msg := rtPOR1V6Firewall(-250); msg != "" {
		t.Fatalf("rtPOR1V6Firewall fired on a NEGATIVE reputation (-250), which is the forger being "+
			"correctly PENALISED after a remediation — the firewall working, not a breach: %s", msg)
	}
	if msg := rtPOR1V6Firewall(0); msg != "" {
		t.Fatalf("rtPOR1V6Firewall fired on the pinned state (rep 0): %s", msg)
	}
	if rtPOR1V6Firewall(250) == "" {
		t.Fatal("rtPOR1V6Firewall stayed silent on a POSITIVE reputation — a forged pass buying " +
			"Sybil-resistant standing would reach the gamma->1/N firewall unnoticed")
	}

	// --- por2Outsourcing, the POSITIVE assertion that replaced the outsourcing pin.
	// Its polarity is now the ordinary one, so the two arms swap: it must stay SILENT
	// on the fixed state and FIRE on the pinned one. Kept here rather than deleted
	// with the pin, because a positive assertion can rot into a tautology the same
	// way a pin can — one that accepted the old Passed=1 grade would be green forever.
	if msg := por2Outsourcing(fixed, -25000); msg != "" {
		t.Fatalf("por2Outsourcing fired on the state it now asserts (proxy graded FAILED and slashed, never paid): %s", msg)
	}
	if por2Outsourcing(pinned, 25000) == "" {
		t.Fatal("por2Outsourcing stayed silent on the PRE-FIX grade (proxy Passed=1, 25000 minted) — the assertion cannot " +
			"see the defect coming back, which is the only thing it is here to do")
	}

	// GATE 5a's arms are GONE, not silenced: rtPOR5aPin was retired when the root
	// binding landed, and its replacement (TestForeignRootedProofIsRefused) is an
	// ordinary positive assertion that needs no teeth here. Both halves it insisted
	// stay independently observable are asserted there — leg 1 refusing in isolation
	// AND the composed grade failing — because a composed FAIL alone can be a later
	// leg moving.
}
