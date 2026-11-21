package chain

import (
	"crypto/ed25519"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// ValidateSubmission runs the checks a single-entry proposal of e by the holder of
// proposer would apply to the entry and its proposer at the next height: proposer
// qualification, then ValidateEntry. It refuses what ValidateProposal would refuse
// for such a block, for the same stated reason, apart from the block-level rules
// (signature, version, digests, carrier and post-apply roots), which follow from how
// the proposer builds the block, not from e.
//
// It reads committed state and copies none of it, so its cost does not grow with the
// chain. Building and validating the candidate block instead would clone the state
// and rebuild its tree twice for every submitted publish.
func (c *Chain) ValidateSubmission(proposer ed25519.PublicKey, e ports.Entry) error {
	_, height := c.Head()
	// The proposal's own qualification rule, with its attribution of the branch
	// that disqualifies, read from the live chain.
	if out, err := v5RequireProposerQualified(liveView{c}, &Block{Height: height, Proposer: proposer}); out != Accept {
		if err == nil {
			err = fmt.Errorf("chain: proposer qualification returned %s with no reason", out)
		}
		return err
	}
	return c.ValidateEntry(e)
}
