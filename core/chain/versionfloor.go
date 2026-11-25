package chain

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/advisory"
	"github.com/nerolabs/silt/ports"
)

// The version-floor advisory on the chain (TENETS R4; core/advisory).
//
// A block may carry one threshold-signed advisory. Validity admits it only when the genesis-
// committed maintainer keys signed it to their threshold, for this network, with a sequence above
// the one in force; apply records it, with the height that committed it, as the advisory in force.
// That record is one committed state leaf (tagVersionFloor), so a node that starts from a
// checkpoint, and a floor box that holds only roots, read the same advisory a full replay does,
// and enforcement's grace period is counted from a height every replica agrees on.
//
// Nothing here enforces anything. The chain records what the maintainers declared; what a node
// does about it (warn, or refuse consensus messages from old releases once the grace has passed)
// is node behaviour over this record.

// versionFloorState is the advisory in force and the height that committed it.
type versionFloorState struct {
	Advisory    advisory.Advisory `cbor:"1,keyasint"`
	CommittedAt uint64            `cbor:"2,keyasint"`
}

var (
	// ErrAdvisoryEra is an advisory on a block below the witnessable version: the era-3 leaf set
	// does not commit the advisory in force, so such a block would write state its own root omits.
	ErrAdvisoryEra = errors.New("chain: a version-floor advisory rides only a witnessable block")
	// ErrAdvisoryAtGenesis is an advisory on the genesis block. A network starts with no floor.
	ErrAdvisoryAtGenesis = errors.New("chain: the genesis block may not carry a version-floor advisory")
	// ErrAdvisoryRefused is an advisory that is malformed or below the maintainers' threshold.
	ErrAdvisoryRefused = errors.New("chain: version-floor advisory refused")
	// ErrAdvisoryStale is an advisory whose sequence does not exceed the one in force: a replay,
	// or an older advisory arriving late.
	ErrAdvisoryStale = errors.New("chain: version-floor advisory does not supersede the one in force")
)

var versionFloorEnc = func() cbor.EncMode {
	m, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return m
}()

func encodeVersionFloor(s versionFloorState) []byte {
	raw, err := versionFloorEnc.Marshal(s)
	if err != nil {
		panic(err) // canonical encoding of our own struct cannot fail
	}
	return raw
}

// decodeVersionFloor parses a committed leaf, and refuses one that is not the canonical encoding
// of a well-formed advisory: a committed value is checked, not trusted.
func decodeVersionFloor(raw []byte) (versionFloorState, bool) {
	var s versionFloorState
	if err := cbor.Unmarshal(raw, &s); err != nil {
		return versionFloorState{}, false
	}
	if s.Advisory.Validate() != nil || string(encodeVersionFloor(s)) != string(raw) {
		return versionFloorState{}, false
	}
	return s, true
}

// admitAdvisory is the advisory's validity rule, shared by the full-node and floor-box paths:
// each supplies the sequence in force from its own source.
func admitAdvisory(b *Block, chainID ports.Hash, maintainers [][]byte, threshold int, inForceSeq uint64) error {
	if b.Version < BlockVersionWitnessable {
		return fmt.Errorf("%w: block version %d", ErrAdvisoryEra, b.Version)
	}
	if err := advisory.Verify(*b.Advisory, chainID, maintainers, threshold); err != nil {
		return fmt.Errorf("%w: %v", ErrAdvisoryRefused, err)
	}
	if b.Advisory.Advisory.Seq <= inForceSeq {
		return fmt.Errorf("%w: sequence %d, in force %d", ErrAdvisoryStale, b.Advisory.Advisory.Seq, inForceSeq)
	}
	return nil
}

// validateVersionFloor is the full node's check of a block's advisory.
func (c *Chain) validateVersionFloor(b *Block) error {
	if b.Advisory == nil {
		return nil
	}
	var seq uint64
	if c.versionFloor != nil {
		seq = c.versionFloor.Advisory.Seq
	}
	return admitAdvisory(b, c.ChainID(), c.cfg.Maintainers, c.cfg.MaintainerThreshold, seq)
}

// applyVersionFloor records a block's advisory as the one in force. A recall is recorded too:
// it carries no floor, and its sequence is what refuses a replay of the advisory it withdrew.
func (c *Chain) applyVersionFloor(b Block) {
	if b.Advisory == nil {
		return
	}
	c.versionFloor = &versionFloorState{Advisory: b.Advisory.Advisory, CommittedAt: b.Height}
}

// VersionFloor returns the advisory in force and the height that committed it. ok is false when
// no advisory has ever been committed.
func (c *Chain) VersionFloor() (a advisory.Advisory, committedAt uint64, ok bool) {
	if c.versionFloor == nil {
		return advisory.Advisory{}, 0, false
	}
	return c.versionFloor.Advisory, c.versionFloor.CommittedAt, true
}
