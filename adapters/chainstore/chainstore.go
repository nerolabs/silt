// Package chainstore persists a chain replica to disk (canonical CBOR,
// atomic replace) so a restarted validator daemon rejoins with its
// history. On load it re-verifies every block's cryptographic integrity —
// hashes and signatures — because trusting your own disk is still trusting;
// but it does NOT re-gate our own committed history on the live reputation
// view (which is empty at boot), or a restart would be stranded at genesis
// (F1). Reputation is re-earned live via bond audits; catch-up on blocks
// missed while down is a separate, peer-facing path (Node.SyncChain).
package chainstore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nerolabs/silt/core/chain"
)

// Load reads blocks from path; a missing file is an empty history.
func Load(path string) ([]chain.Block, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return chain.DecodeBlocks(raw)
}

// Save writes the full chain DURABLY and atomically (#558 / Lane B8, scope
// call S3): the bytes go to a temp file in the same directory, are fsync'd,
// the temp file is closed, renamed over path, and the directory is fsync'd so
// the rename itself is durable — the markstore pattern (the #183 C-2 work).
// Before this the temp file was written without a sync: a SIGKILL could not
// tear the renamed file, but a power loss or kernel OOM at the wrong moment
// could leave chain.cbor truncated on disk, and the daemon then replayed a
// torn tail (run a434494-deep: 87 MiB of finalized history at h83 discarded).
// Chains of registry entries are small (that's the design); rewriting whole
// is simpler than appending safely.
func Save(path string, blocks []chain.Block) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".chain-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(chain.EncodeBlocks(blocks)); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		df.Close()
	}
	return nil
}

// LossError reports that replaying path would DISCARD finalized history: the
// file is torn (undecodable — no valid prefix survives, Restored = 0) or a
// block inside it fails structural verification (the valid prefix of
// Restored blocks is kept, the suffix is lost). It is returned by Recover
// when the operator has not accepted the loss.
type LossError struct {
	Restored int   // blocks of valid prefix restored into the chain
	Cause    error // the replay failure
}

func (e *LossError) Error() string {
	return fmt.Sprintf("chain replay would discard finalized history (%d-block valid prefix kept): %v", e.Restored, e.Cause)
}

func (e *LossError) Unwrap() error { return e.Cause }

// Recover is the daemon's boot-time replay with the #558 refuse-to-start
// rule (Lane B8, scope call S3, ratified 2026-09-07): a replay that would
// discard finalized history — a torn chain.cbor, or a block that fails
// structural verification — is a LossError and the caller must NOT start,
// unless acceptLoss is set (the operator's explicit acknowledgement that the
// suffix will be re-synced from peers, which is impossible below the swarm's
// prune horizon without a fresh -ws-checkpoint, #559). With acceptLoss the
// valid prefix is kept and the loss is reported through the returned
// LossError's Restored count with a nil refusal. A missing file is an empty
// history, never a loss.
func Recover(path string, c *chain.Chain, acceptLoss bool) (restored int, loss *LossError, refused error) {
	n, err := Replay(path, c)
	if err == nil {
		return n, nil, nil
	}
	loss = &LossError{Restored: n, Cause: err}
	if acceptLoss {
		return n, loss, nil
	}
	return n, loss, loss
}

// Replay loads path into c and reloads our own persisted history via
// chain.Reload — re-verifying each block's structure and signatures, but not
// the live reputation gate (see the package doc and chain.appendStructural).
// Returns how many blocks were restored.
func Replay(path string, c *chain.Chain) (int, error) {
	blocks, err := Load(path)
	if err != nil {
		return 0, err
	}
	return c.Reload(blocks)
}
