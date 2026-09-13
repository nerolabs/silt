# The chainless client carries 32 bytes — the supply route, and what it is NOT

**Date:** 2026-09-13 · **Seat:** Builder · **Branch:** `builder/chainless-client-chain-id`
**Ratified:** owner call 7, 2026-09-12 · **Ledger:** `D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12`
**Certification:**
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/TOKEN-DOMAIN-GENESIS-COVERAGE-AND-THE-CHAINLESS-CLIENT-RESEARCH-CERTIFICATION-2026-09-12.md`

## The mechanism, in one paragraph

`cmd/silt`'s `joinSwarm` builds the `swarm add` client with `node.New` / `SetSigner` /
`SetEphemeral(true)` and never calls `EnableChain`, so `(*Node).chainID()` returns the zero hash for
the whole life of that process. Under M3 the prepaid-credit lane prepends the chain id to the FDH
input (`blindtoken.BlindCredit`), and a zero chain id there produces a credit no validator will
verify. The failure then travels silently: `(*Node).AcquireCredits` reports the error, `mintCredits`
binds it to `_`, and `(*Node).acquireToken` *skips* an issuer it holds no credit for rather than
reporting one, so all the operator sees is `node: could not gather enough publish-token signatures`
(measured — that is the exact string the ablation produced). This change addresses the cause by
giving the client a route to declare the 32 bytes an operator already has, and by keeping the mint
error instead of discarding it.

## Options weighed

| Route | Cost | Taken? |
|---|---|---|
| **(a) Operator flag `-chain-id`** | one more flag on a token-gated publish; the operator already supplies `-peers` and `-registry`, and the daemon already prints the genesis hash at start-up | **YES** — ratified |
| (b) Fetch from peers, unanimity-or-refuse | no new flag; a new k-of-k liveness dependency and a new wire probe (`ports/net.go` has none today) | **NO** — the owner's call was route (a). The first-success form is explicitly barred: one stale peer would deny every publish with no signal |
| (c) A durable parent hands it down | inapplicable — a one-shot client has no parent | **NO** |
| (d) Fall back to the credit-free `(*Node).AcquireToken` | re-opens red-team re-verification #4, a published claim | **NO** — owner-only, and the cert declines to certify the price |

## The one design decision that was mine

**Two accessors, not one.** The obvious cheap move is to let `(*Node).chainID()` fall back to the
declared value, which makes every existing bound lane work with no new symbol. I refused it.

`chainID` is the **verifier's** value. Its own doc says the zero hash is the safe direction because
`verifyAtt` refuses every era-4 signature form under it, so a chainless node convicts nobody and
mints nothing a peer takes. A caller-supplied value there is a **wrong-accept** surface: it widens
what this node admits. The requester side is the opposite direction — a wrong value can only deny
the requester its own token, because every verifier checks under its own chain id. Same 32 bytes,
opposite failure mode, so they get different functions and the split is pinned by a test that
ablates to RED (`TestDeclaringANetworkIdentityDoesNotMoveTheVerifierSideChainID`).

Two consequences fall out of the same reasoning and are gated too: `SetNetworkIdentity` refuses a
node that HOLDS a chain (a chain-holder derives its identity; a flag must never override it), and
`RequesterChainID` lets a chain outrank a declaration, which is what makes consumer 3's refusal
window — a joining daemon — genuinely transient.

## What this deliberately does not do

- **It does not bind `fdhDomain`.** The publish token is verified inside block validity
  (`v5ValidateEntry`, `(*Chain).ValidateEntry`) against a third party's key, so binding it is a
  consensus-rule change, research-gated, and nothing here licenses it.
- **It does not close `R-CLIENT-HAS-NO-CHAIN` or `R-E2E-ERA4-FIXTURE`.** Those ride the committed
  `E ↦ key_E` binding through `(*Chain).IssuerKeyCommitment`, which is re-resolved on every read and
  genuinely is most of a chain. One rule, two mechanisms — the cert refutes "one answer serves all
  four consumers".
- **It does not build peer-fetch**, in either form.
- **It moves no format and no genesis.** **CORRECTED 2026-09-13 by blind review** — this sentence
  originally read *"`core/genesis` does not reach any symbol this change touches"*, which invited a
  false premise into the record. Measured: `core/genesis` **does** reach `cmd/silt`, via
  `cmd/silt/genesis.go` and `cmd/silt/daemon.go`, and it did so before this change; it does **not**
  reach `core/node` (`go list -deps ./core/node` finds it zero times). The correction cuts in the
  flag's favour: `cmdGenesis` prints the **paramless** hash, and a daemon-minted genesis commits its
  consensus config, so a client linked against `core/genesis` still cannot derive a real network's
  genesis hash. The flag is not redundant with the embedded manifesto. No format moves and no
  genesis moves — that part stands.

## The honest weakness

On this branch the flag's value is **carried and never blinded** — the credit lane binds it in #828.
Measured with `go tool nm` on the linked `./cmd/silt`: `RequesterChainID` and `HasNetworkIdentity`
are ABSENT, `SetNetworkIdentity` and `swarmAdd`'s posted install closure are present. The supply
route links; the consumer does not exist. Filed as `R-CHAINID-INSTALL-SOURCE-GATED`.

## Fold-in, 2026-09-13 — what the blind review changed, and the one thing it changed my mind about

The review returned MERGEABLE-WITH-FIXES on five findings. The one worth writing down is **F-A**.

I had reasoned that because no runtime consumer exists, a source gate is the honest ceiling for the
install site, and the deliberation above says so. The review agreed with the ceiling and then
**falsified the gate anyway**: ablation A9 left the install call written in `swarmAdd` and made its
guard impossible, and the whole `cmd/silt` suite stayed GREEN. That is verbatim the defect the
gate's `t.Fatal` message claimed to catch. **A source gate proves a call is WRITTEN, never that it
RUNS**, and I had let a true statement about the CALL stand in for a false statement about the
VALUE.

The fix is not a re-wording, though the re-wording happened too. **The guard moved.** `swarmAdd` now
makes one unconditional call to `declareNetworkIdentity`, and that function owns the
declared/not-declared decision — in a function a test can call with a node it holds. A9's defect
class is now RED at the unit tier, four arms and four ablations. What no gate of this shape can
still see is `swarmAdd`'s unconditional call being made unreachable, which is a louder and narrower
corruption, and it is the residual row rather than a claim.

The general shape, and it is the part that generalises past this PR: **when a gate cannot reach the
thing it asserts, move the thing to where a gate can reach it.** Narrowing the message is what you
do after you have taken that as far as it goes, not instead.

The other four: `HasNetworkIdentity` closes the absent-vs-zero ambiguity the setter refuses and the
accessor re-opened (F-1); `ErrCreditIssuerKeyUnknown` splits a sentinel that was doing two jobs, so
the *"first cause"* clause stops printing the outer sentence back (F-2); the inert mechanism gets
its register row (F-4); and the install is POSTED onto the node's loop rather than written from the
main goroutine after `Bootstrap` has already made the loop live (F-5).
