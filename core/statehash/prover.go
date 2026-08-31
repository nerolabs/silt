package statehash

import (
	"crypto/sha256"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// Prover is the PROVIDER-side complement to Resolve (the verifier-side accessor). A
// semi-stateless floor box holds only the committed StateRoot; it fetches witnesses from an
// any-of-N provider that DOES hold the committed set. Prover is that provider's primitive: it
// commits a leaf set into the SMT (the same non-sum SHA-256 spec Root and verifySpec use) and
// issues per-key inclusion / non-inclusion proofs a box then verifies against the committed root.
//
// It reuses the ONE audited pokt-network/smt implementation and the pinned SHA-256 spec, so a
// proof it issues verifies under Resolve's verifySpec by construction — the prover and the
// verifier cannot drift on the trie spec (the failure mode witness.go:152-157 warns of).
//
// Root() returns the committed root the box holds; it MUST equal the block's committed StateRoot
// for the box's verification to succeed. A caller builds a Prover from exactly the leaf set that
// produced the committed root (e.g. Chain.stateRootLeavesV5 for a v5 block).
type Prover struct {
	trie *smt.SMT
	root ports.Hash
}

// NewProver commits the given leaves into a fresh SMT and returns a Prover over the committed
// root. It is the provider-side mirror of Root: Root computes only the root scalar; NewProver
// keeps the committed trie so it can issue proofs. A duplicate key is a caller marshalling bug
// (the same contract as Root) and is reported.
func NewProver(leaves []Leaf) (*Prover, error) {
	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	seen := make(map[string]struct{}, len(leaves))
	for _, lf := range leaves {
		if _, dup := seen[string(lf.Key)]; dup {
			return nil, &DuplicateKeyError{Key: append([]byte(nil), lf.Key...)}
		}
		seen[string(lf.Key)] = struct{}{}
		if err := trie.Update(lf.Key, lf.Value); err != nil {
			return nil, err
		}
	}
	if err := trie.Commit(); err != nil {
		return nil, err
	}
	var root ports.Hash
	copy(root[:], trie.Root())
	return &Prover{trie: trie, root: root}, nil
}

// Root returns the committed SMT root this Prover proves against. It equals what Root(leaves)
// returns for the same leaf set, and must equal the block's committed StateRoot.
func (p *Prover) Root() ports.Hash { return p.root }

// Prove issues the SMT proof for a field-tagged key (built with Key(tag, rawKey)). For a
// committed key it is an inclusion proof (verifies as ProvenPresent with the committed value);
// for an absent key it is a non-inclusion proof (verifies as ProvenAbsent). The returned Witness
// is ready to hand to Resolve against this Prover's Root.
func (p *Prover) Prove(key []byte) (Witness, error) {
	proof, err := p.trie.Prove(key)
	if err != nil {
		return Witness{}, err
	}
	return NewWitness(proof), nil
}
