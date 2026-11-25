package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// TestFalseRepairProofConvictsOnlyASignedFalseClaim: the claimant's signed claim that
// shard X is leaf L of root R, beside a Merkle path showing leaf L is Y != X, convicts
// the claimant, and every variant that does not prove exactly that is refused: a true
// claim, an unsigned or re-signed one, another network's, a proof for another leaf,
// a culprit other than the signer, and a record that also carries blocks.
func TestFalseRepairProofConvictsOnlyASignedFalseClaim(t *testing.T) {
	leaves := make([]ports.Hash, 23)
	for i := range leaves {
		leaves[i] = ports.HashBytes([]byte{byte(i), 'l'})
	}
	root := manifest.MerkleRoot(leaves)
	const leaf = 14
	pr, err := manifest.Prove(leaves, leaf)
	if err != nil {
		t.Fatal(err)
	}
	chainID := ports.HashBytes([]byte("network"))
	_, priv, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil)
	floor := EraFloor(func(uint64) uint64 { return 0 })

	claim := repairproof.RepairClaim{Root: root, Stripe: 1, ShardPos: 1, LeafIndex: leaf,
		ShardID: ports.ChunkID(ports.HashBytes([]byte("not the committed shard"))), Holder: ports.NodeID{9}}
	signed, err := repairproof.SignClaim(priv, chainID, claim)
	if err != nil {
		t.Fatal(err)
	}
	proof := func(s repairproof.SignedClaim) *Equivocation {
		return &Equivocation{Culprit: s.Pub, FalseRepair: &FalseRepairProof{
			Claim: s, Leaf: leaves[leaf], Total: pr.Total, Path: pr.Path}}
	}

	if err := CheckEquivocation(proof(signed), chainID, floor); err != nil {
		t.Fatalf("a signed claim refuted by its own root must convict: %v", err)
	}
	if got := proof(signed).CulpritID(); got != signed.Claimant() {
		t.Fatalf("the culprit is not the claimant: %x vs %x", got[:4], signed.Claimant())
	}

	refuse := func(name string, e *Equivocation, id ports.Hash) {
		t.Helper()
		if err := CheckEquivocation(e, id, floor); !errors.Is(err, ErrNotFalseRepair) {
			t.Fatalf("%s: want ErrNotFalseRepair, got %v", name, err)
		}
	}
	trueClaim := claim
	trueClaim.ShardID = ports.ChunkID(leaves[leaf])
	ts, _ := repairproof.SignClaim(priv, chainID, trueClaim)
	refuse("a TRUE claim", proof(ts), chainID)

	forged := signed
	forged.Sig = append([]byte(nil), signed.Sig...)
	forged.Sig[0] ^= 1
	refuse("a forged signature", proof(forged), chainID)

	refuse("another network's claim", proof(signed), ports.HashBytes([]byte("elsewhere")))

	framed := proof(signed)
	framed.Culprit = other.Public().(ed25519.PublicKey)
	refuse("a culprit other than the signer", framed, chainID)

	wrongLeaf := proof(signed)
	pr2, _ := manifest.Prove(leaves, leaf+1)
	wrongLeaf.FalseRepair.Leaf, wrongLeaf.FalseRepair.Path = leaves[leaf+1], pr2.Path
	refuse("a proof of a different leaf", wrongLeaf, chainID)

	withBlocks := proof(signed)
	withBlocks.A = Block{Version: 1, Height: 3}
	refuse("a record that also carries a block", withBlocks, chainID)

	if err := CheckEquivocation(proof(signed), chainID, nil); err == nil {
		t.Fatal("a node with no era floor (no chain) must convict nobody")
	}
}
