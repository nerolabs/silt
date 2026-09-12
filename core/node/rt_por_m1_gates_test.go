package node

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
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

// M1 of the PoR key-distribution certification (§8), landed as three
// PINNED_DEFECT gates at the COMPOSED level the certification demands: the
// oracle is gradeAnswers' full three-leg grade, never por.Key.Verify alone.
//
// ⚠⚠ READ THE POLARITY BEFORE YOU READ A RESULT, AND BEFORE YOU READ THE
// CERTIFICATION'S FALSIFIER.
//
// These three tests assert the tree's CURRENT, BROKEN behaviour. They are GREEN
// while the break is present and go RED when it is FIXED. A RED here is not a
// regression; it is the pin doing its job. This follows the posture ratified in
// D-REPAIR-CLAIM-GATES-PINNED-2026-09-12 and the mechanism shipped in #817
// (core/pipeline's TestRT_SFO_4_..._PINNED_DEFECT and
// TestRT_SFO_5_..._PINNED_DEFECT). Skip-until-fixed is refused: a t.Skip costs
// the same lines and reports green either way
// (scar:short-run-is-zero-execution).
//
// THE POLARITY FLIPPED WHEN THESE GATES WERE PINNED, AND THE CERTIFICATION'S
// FALSIFIER IS WRITTEN IN THE OLD POLARITY. The certification
// (POR-KEY-DISTRIBUTION-BREAK-REMEDIATION-OPTIONS-75c0f89-RESEARCH-CERTIFICATION-2026-09-12.md,
// §M1) says: "Expected at 75c0f89: all three RED. If M1's first test passes at
// 75c0f89, my V1/V2/V3 verdicts are wrong and this certification is withdrawn."
// That sentence is about the ORIGINAL tests, which asserted the INTENDED rule.
// It was satisfied: all three were RED at 75c0f89, and the ablation ran before
// the conversion below. DO NOT READ A GREEN HERE AS TRIPPING THAT FALSIFIER —
// under the pinned polarity, GREEN means the forgery still works and the
// certification still stands. The falsifier's modern form is the inverse: if a
// pin below goes RED, the break it holds has been remediated, and the
// certification's V1/V2/V3 verdicts and the remediation pricing must be re-read
// before the pin is retired.
//
// A pin records behaviour. It does NOT ratify that behaviour as correct and it
// does not price a fix; the remediation options are the certification's, not
// this file's.

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

// forgeZeroMu is the certification §1.3 forgery: every Mu[j] = 32 zero bytes,
// so Σⱼ αⱼ·μⱼ vanishes and σ is just Σᵢ νᵢ·f_k(i), which the key holder
// computes directly. Returns the proof plus the number of field
// multiplications it performed, for the asymmetry table.
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
// answer, building the `valid` leg the way auditLeaf builds it (por.go:322-323)
// — via the product's own verifyStorageProof — so no leg is asserted by the
// test.
func gradeOne(t *testing.T, n *Node, id ports.ChunkID, porKey *por.Key, base [32]byte,
	want int, prover ports.NodeID, resp ports.Message) AuditReport {
	t.Helper()
	valid := resp.Found && resp.Proof != nil && verifyStorageProof(*resp.Proof, id)
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
// ---------------------------------------------------------------------------

func TestRT_POR_1_CareLinkHolderForgesWithZeroBytes_PINNED_DEFECT(t *testing.T) {
	n, _ := aloneNode(t, 0)
	led := credit.New(1, 500_000)
	n.SetLedger(led)

	layoutKey := linkLayoutKey(t) // the value a CARE LINK carries, by design
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(4242)

	id, data, tags, sp, _ := honestShard(t, porKey)
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
	hr := gradeOne(t, n, id, porKey, base, want, honest, ports.Message{
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
	fsp := ports.StorageProof{Root: selfRoot(id), Index: 0, Total: 1, Path: nil, Column: -1}

	before := led.Balance(forger)
	fr := gradeOne(t, n, id, porKey, base, want, forger, ports.Message{
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

	// V6 of the certification: the gamma->1/N firewall. THIS ARM IS NOT A PIN — it
	// asserts the INTENDED rule and passes because the rule HOLDS. A forged PASS
	// mints spendable credit but no Sybil-resistant STANDING, because
	// credit.Reputation never reads auditsPassed. It is asserted, not merely
	// logged, so that the pinned break below cannot silently widen from a credit
	// mint into a standing mint without a RED.
	if rep := led.Reputation(forger); rep != 0 {
		t.Fatalf("V6 FIREWALL BREACHED — the zero-byte forger's Reputation is %d after a forged audit pass, want 0. "+
			"This is NOT the pinned defect widening quietly; it is a different and worse finding: a forged PoR pass now buys "+
			"Sybil-resistant standing, which reaches the gamma->1/N firewall and M0. credit.Reputation must read bond only. "+
			"ESCALATE before touching this file.", rep)
	}
	t.Logf("V6 firewall HOLDS: forger Reputation after the forged pass = %d (bond is the only mint)",
		led.Reputation(forger))

	// --- NEGATIVE CONTROL: the SAME attack WITHOUT the care link must fail. ---
	var wrongKey [32]byte
	wrongKey[0] = 0x99
	wp, _ := forgeZeroMu(wrongKey, id[:], fSeed, want, porSampleCount)
	wr := gradeOne(t, n, id, porKey, base, want,
		ports.HashBytes([]byte("no-care-link")), ports.Message{
			Kind: ports.MsgChallengeReply, Found: true, Proof: &fsp,
			PorBlocks: want, PorMu: wp.Mu, PorSigma: wp.Sigma,
		})
	if wr.Passed != 0 {
		t.Fatalf("CONTROL BROKEN: a forger WITHOUT the layout key also passed (%+v) — "+
			"then the finding is not about key distribution", wr)
	}

	// PIN: the zero-byte forger is graded PASSED and is paid for it.
	if fr.Passed != 1 || fr.Failed != 0 || minted <= 0 {
		t.Fatalf("RT-POR-1 PIN IS RED — a care-link holder storing ZERO bytes was graded %+v and minted %d credit; "+
			"pinned at Passed=1, Failed=0 and a positive mint.\n"+
			"  THE FIX CASE: Passed=0 means a zero-byte forger no longer satisfies the composed three-leg audit — the PoR key no longer\n"+
			"  rides the care link, or the grade no longer accepts a proof the key holder can solve for. That is the remediation this pin was\n"+
			"  waiting for. Before retiring it: re-read the certification\n"+
			"  POR-KEY-DISTRIBUTION-BREAK-REMEDIATION-OPTIONS-75c0f89-RESEARCH-CERTIFICATION-2026-09-12.md (V1/V2/V3 and the remediation\n"+
			"  pricing addendum), confirm WHICH option shipped, and replace this pin with the positive assertion that a prover without the\n"+
			"  bytes fails. Check the positive control above still passes, so the fix is not simply over-rejection.\n"+
			"  THE OTHER CASE: Passed=1 with a zero mint means the grade still accepts the forgery but the payment path changed; the audit\n"+
			"  break is untouched and only the economics moved. Re-derive which before re-pinning — this pin is about the GRADE.",
			fr, minted)
	}
}

// ---------------------------------------------------------------------------
// GATE 2 — challenge OUTSOURCING. B holds no bytes; it forwards the auditor's
// verbatim MsgChallenge to a real holder A, whose REAL answerChallenge computes
// under B's seed, and returns A's answer as its own.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: a prover should not be able to have
// someone else answer its identity-bound challenge. Node.handle's MsgChallenge
// case has `from` in scope and calls n.answerChallenge(msg) without it, and
// answerChallenge takes no prover parameter, so A computes under B's seed on
// request and cannot tell it is answering for someone else. Contrast
// answerBondChallenge, which does take `from`. The m0.md S2 row claims "proof
// outsourcing/relay" is hardened: RELAY is genuinely denied by porProverSeed —
// the control below proves it — but OUTSOURCING is not.
//
// PIN. GREEN today. It goes RED when the prover identity reaches answerChallenge.
// ---------------------------------------------------------------------------

func TestRT_POR_2_ChallengeProxyPassesAudit_PINNED_DEFECT(t *testing.T) {
	auditor, _ := aloneNode(t, 0)
	led := credit.New(1, 500_000)
	auditor.SetLedger(led)

	layoutKey := linkLayoutKey(t)
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(7777)

	id, data, tags, sp, _ := honestShard(t, porKey)
	want := por.DefaultParams.Blocks(pipeline.DefaultChunkSize + ctOverhead)

	// A is a REAL node that really holds the shard and its proof.
	holderA, _ := aloneNode(t, 0)
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

	// The auditor's message to B, forwarded by B to A VERBATIM. answerChallenge
	// takes no `from`, so A cannot tell it is answering for someone else.
	challenge := ports.Message{Kind: ports.MsgChallenge, ChunkID: id, PorSeed: bSeed[:], PorCount: porSampleCount}
	relayed := holderA.answerChallenge(challenge)

	// --- CONTROL: the RELAY attack (which porProverSeed DOES deny) must fail.
	// A answers under A's OWN seed and B returns it. If this passed too, the
	// identity binding is broken outright and the finding below would be
	// mis-attributed to outsourcing.
	holderIDA := ports.HashBytes([]byte("real-holder-A"))
	relayOnly := holderA.answerChallenge(ports.Message{
		Kind: ports.MsgChallenge, ChunkID: id,
		PorSeed: sliceOfSeed(porProverSeed(base, holderIDA)), PorCount: porSampleCount,
	})
	if rc := gradeOne(t, auditor, id, porKey, base, want, proxyB, relayOnly); rc.Passed != 0 {
		t.Fatalf("CONTROL BROKEN: a plain RELAY of A's own-seed proof also passed (%+v) — "+
			"porProverSeed's identity binding is not holding, so this test is not "+
			"measuring challenge OUTSOURCING", rc)
	}
	// --- CONTROL: B with no proxy at all holds nothing and must fail.
	if rn := gradeOne(t, auditor, id, porKey, base, want, proxyB,
		ports.Message{Kind: ports.MsgChallengeReply}); rn.Passed != 0 {
		t.Fatal("CONTROL BROKEN: an empty reply was graded PASSED")
	}

	before := led.Balance(proxyB)
	r := gradeOne(t, auditor, id, porKey, base, want, proxyB, relayed)
	minted := led.Balance(proxyB) - before
	t.Logf("grade of B's proxied answer: %+v; credit minted to the data-less proxy: %d", r, minted)

	// PIN: the data-less proxy passes its audit and is paid for it.
	if r.Passed != 1 || r.Failed != 0 || minted <= 0 {
		t.Fatalf("RT-POR-2 PIN IS RED — a data-less identity B that forwarded the auditor's verbatim challenge to holder A was graded %+v "+
			"and minted %d credit; pinned at Passed=1, Failed=0 and a positive mint.\n"+
			"  THE FIX CASE: Passed=0 means outsourcing is now denied. The expected shape is that the prover identity reaches\n"+
			"  Node.answerChallenge — Node.handle's MsgChallenge case already has `from` in scope, and answerBondChallenge already takes it —\n"+
			"  so A refuses to compute under a seed that is not bound to A. Confirm THAT is what landed, then replace this pin with the\n"+
			"  positive assertion and update the m0.md S2 row, which claims outsourcing is hardened and was over-claiming while this pin was green.\n"+
			"  Check the two controls above still hold, so the fix is not over-rejection of honest holders.\n"+
			"  THE OTHER CASE: Passed=1 with a zero mint means the grade is unchanged and only the payment path moved. This pin is about the GRADE.",
			r, minted)
	}
}

// ---------------------------------------------------------------------------
// GATE 5a — the Merkle leg does NOT bind to the audited root.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: leg 1 should prove the shard is the
// one the audit is asking about. verifyStorageProof takes p.Root FROM THE
// RESPONSE and never compares it to the audited root, and with
// {Index:0, Total:1, Path:nil} manifest.VerifyProof reduces to
// leafHash(leaf)==root — so any party knowing only the chunk id satisfies leg 1
// with zero knowledge. Neither call site binds p.Root to the root it is auditing:
// Node.auditLeaf passes m.Root() nowhere into the check, and
// Node.challengeHolderRetrievability does not bind claim.Root. Leg 1 is a
// tautology and buys nothing.
//
// PIN. GREEN today. It goes RED when leg 1 binds to the audited root.
// ---------------------------------------------------------------------------

func TestRT_POR_5a_MerkleLegDoesNotBindToAuditedRoot_PINNED_DEFECT(t *testing.T) {
	n, _ := aloneNode(t, 0)
	n.SetLedger(credit.New(1, 500_000))

	layoutKey := linkLayoutKey(t)
	porKey := DerivePorKey(layoutKey)
	base := porChallengeSeed(31337)
	id, _, tags, sp, auditedRoot := honestShard(t, porKey)
	want := len(tags)

	// A party that knows only the chunk id names its OWN root.
	bogus := ports.StorageProof{Root: selfRoot(id), Index: 0, Total: 1, Path: nil, Column: -1}
	if bogus.Root == auditedRoot {
		t.Fatal("setup: the self-root collided with the audited root")
	}

	// Leg 1, in isolation: the product's own verifier accepts it.
	acceptedInIsolation := verifyStorageProof(bogus, id)
	t.Logf("verifyStorageProof(self-rooted, id) = %v; audited root = %x, claimed root = %x",
		acceptedInIsolation, auditedRoot[:6], bogus.Root[:6])

	// Composed: the same self-rooted proof, carried by a zero-byte forger.
	fSeedProver := ports.HashBytes([]byte("zero-knowledge-rooter"))
	fp, _ := forgeZeroMu(layoutKey, id[:], porProverSeed(base, fSeedProver), want, porSampleCount)
	r := gradeOne(t, n, id, porKey, base, want, fSeedProver, ports.Message{
		Kind: ports.MsgChallengeReply, Found: true, Proof: &bogus,
		PorBlocks: want, PorMu: fp.Mu, PorSigma: fp.Sigma,
	})

	// The honest holder's REAL proof must keep passing leg 1 (no over-rejection).
	if !verifyStorageProof(sp, id) {
		t.Fatal("FIXTURE BROKEN: the honest inclusion proof failed leg 1")
	}

	// PIN: leg 1 accepts a self-rooted proof both in isolation and composed.
	if !acceptedInIsolation || r.Passed != 1 || r.Failed != 0 {
		t.Fatalf("RT-POR-5a PIN IS RED — a self-rooted proof built from the chunk id alone was accepted in isolation=%v and graded %+v; "+
			"pinned at isolation=true, Passed=1, Failed=0.\n"+
			"  THE FIX CASE: isolation=false means verifyStorageProof now compares p.Root to the root it is auditing rather than taking it from\n"+
			"  the response. Confirm BOTH call sites were fixed — Node.auditLeaf against the layout root and Node.challengeHolderRetrievability\n"+
			"  against claim.Root — because fixing one leaves the other a tautology and this pin cannot tell them apart from a single RED.\n"+
			"  The FIXTURE BROKEN arm above already asserts the honest inclusion proof still passes leg 1, so a RED here is not over-rejection.\n"+
			"  Then replace this pin with the positive assertion that a proof naming a foreign root is refused.\n"+
			"  THE OTHER CASE: isolation=true but Passed!=1 means leg 1 is still a tautology and a LATER leg changed. Leg 1 is what this pin holds;\n"+
			"  re-derive which leg moved before re-pinning.",
			acceptedInIsolation, r)
	}
}
