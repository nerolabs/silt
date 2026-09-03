package main

// R0.4b C3 close — the per-epoch demand-key ROTATION SCHEDULE.
//
// THE BREAK THIS CLOSES (red-team probe B, 2026-09-02). The daemon installed the
// PERSISTED PUBLISH key as the demand key for its boot epoch, once, and nothing ever
// rotated it. Two failures followed. From boot+1 the demand lane refused to issue at
// all (no key for the current epoch). From boot+W+1 the bank rejected every token the
// lane had ever signed, while the fee-charging withdrawal path kept charging — a fee
// burned for a token the system would never honour. The lane's whole life was W+1
// epochs, and nothing in the code said so.
//
// THE SCHEDULE. At boot and on every epoch turn, ensure a key exists for each epoch in
// [cur, cur+W] and stage its commitment (SetDemandIssuerKey signs an IssuerKeyReg the
// proposer folds), while RETAINING the keys back to cur−W so an in-window past epoch
// stays signable. Pre-publishing to cur+W is what the chain's registration window
// allows and what makes a withdrawal at an epoch boundary find key_E already
// committed; retaining backwards is what makes the boundary race a served request
// instead of a burnt fee.
//
// THE PUBLISH KEY IS NOT IN THIS BAND, ever. It cannot rotate — committed publish
// tokens re-verify against it on every replay — so it is structurally the wrong key
// for a per-epoch lane. Keeping the lanes separate is what makes "a demand blind
// bought on the publish lane" simply not a demand token, rather than a token that
// dies at boot+W.

import (
	"crypto/rsa"
	"io"

	"github.com/nerolabs/silt/adapters/diskissuer"
	"github.com/nerolabs/silt/core/demand"
)

// demandKeyInstaller is the node half of the schedule: install key_E for epoch E and
// stage its on-chain commitment.
type demandKeyInstaller interface {
	SetDemandIssuerKey(rng io.Reader, epoch uint64, priv *rsa.PrivateKey)
}

// installDemandKeys runs one rotation step for consensus epoch cur. It is the single
// place the daemon binds the schedule to silt's W, so the band arithmetic (in the
// adapter, window-agnostic) and the window value (core/demand) meet exactly once.
func installDemandKeys(nd demandKeyInstaller, es *diskissuer.EpochStore, rng io.Reader, cur uint64) error {
	return es.RotateWindow(rng, cur, demand.DefaultWindow, func(e uint64, k *rsa.PrivateKey) {
		// The SAME injected reader signs with the key it installs: the issuer blinds
		// its private-key operation with it (advisory C-2).
		nd.SetDemandIssuerKey(rng, e, k)
	})
}
