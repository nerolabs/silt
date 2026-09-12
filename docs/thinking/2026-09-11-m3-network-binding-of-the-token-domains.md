# M3 — binding the non-consensus signature domains to the network

- **Date:** 2026-09-11
- **Seat:** Builder
- **Certification (Layer 3, GATED):**
  `silt-agent-memory/researcher/reviews/research-outcome/NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11.md`
  §3, plus Amendment 1 (Layer 1 only) and the Amendment 2 rider (M2 era-floor scope; no
  Layer 3 content).
- **Order:** M1 (`26b2f69`) and M2 (`9130832`) have landed. M3 touches no genesis, no block
  field, no cbor key and no committed leaf.

## The defect, in one paragraph

`demandMsg = epoch ‖ serial` names no network. The credit domain signs a bare `serial`. So
under a shared issuer key a token minted on network X verifies on network Y: a credit bought
on X spends on Y, a demand token bought on X redeems on Y for delivery credit, a relay
prepayment bought on X funds a session on Y. The fix puts the chain id — the genesis block
hash, 32 bytes — at the FRONT of the signed message and retires the old domain version, so a
verifier on Y computes a different FDH input and the signature fails. This is EIP-155's
lesson, which covered transactions only; EIP-712 and Gnosis Safe v1.3.0 each had to add
`chainId` separately, after real losses.

## Options considered

| # | Option | Verdict |
|---|---|---|
| 1 | Per-network FDH **domain constant** (`silt/blinddemand/fdh/v2/<chainid>`) | **Refused.** The certification refutes it at §3.5 and so does the tree: `relayAnchorDomain`'s own doc calls itself *"A FORMAT CONSTANT the T-6 proof depends on"*, pinned byte-exact — a constant whose bytes vary per network is not a constant. It also walks toward `R0.4b-FDH` (the domain has no length prefix), not away from it. |
| 2 | Chain id as a **field of the token / the request** | **Refused, and this is the hard gate.** A field is an attacker input; the binding becomes decoration. The epoch is not a precedent: the requester names the epoch and the issuer bounds it to its own clock ±1, and there is no ±1 for a chain id. |
| 3 | Chain id in the **message**, domain version bumped, chain id a **required parameter** | **CHOSEN.** It is the schema silt already proved when `demandDomain` went v1 → v2 for the epoch, and it is RFC 9578 Privacy Pass's own move (the key's identity inside the signed message). A required parameter turns a missed call site into a compile error — the same lesson M2 proved. |

## What shipped

**Three domains bound, not four.** `chainBoundMsg(chainID, rest) = chainID(32B) ‖ rest`:

| Domain | Was | Now | Message |
|---|---|---|---|
| publish credit | `silt/blindcredit/fdh/v1` | `silt/blindcredit/fdh/v2` | `chainID ‖ serial` |
| demand token | `silt/blinddemand/fdh/v2` | `silt/blinddemand/fdh/v3` | `chainID ‖ epoch ‖ serial` |
| relay anchor | `silt/blindrelay/fdh/v1` | `silt/blindrelay/fdh/v2` | `chainID ‖ epoch ‖ serial` |
| publish token | `silt/blindtoken/fdh/v1` | **unchanged** | `serial` |

Injectivity holds: fixed-width fields followed by exactly one trailing variable field.

**The fourth domain is NOT bound, and that is the finding this build reports.** See below.
It is named at `fdhDomain` in the source, not only here.

**PayWord is not a target.** `core/relaypay/payword.go` has no signature and no domain, only
`sha256.Sum256`. The certification refutes the advisory on this at §3.6; verified at source.

**The leaf session domains are not bound, by the certification's inheritance argument.**
`sessionOpenDomain`, `sessionFundDomain`, `receiptDomainV3` and `"silt/relay/open/v1"` bind
`serverID` / `relayID`, which are not network-scoped — but no session opens without a
chain-bound anchor (`verifyDeliveryAnchors` → `errDeliveryNoAnchor`; the relay twin →
`errRelayNoAnchor`). A cross-network session dies at the anchor, before any session
signature is evaluated. `core/node/TestDeliveryAnchorFromAnotherNetworkIsRefused` drives it.

**The zero-chain-id refusal (G-3b).** A zero chain id means "this node holds no chain", and
every chainless node holds the same zero, so accepting it would make *network zero* a real
shared network. Refused at every bound entry point (`ErrZeroChainID`), and at the issuer:
`answerDemandTokenRequest` returns `OK=false` before any charge when `(*Node).chainID()` is
zero, and `demand.SignWithdrawal` takes the same arm. Same direction `(*Node).chainID`'s own
doc already states for era-4 signatures.

## How the chain id reaches each site, and why it is never a request field

- **Verifiers** read it from their own chain at the call: `Keyset.VerifyInWindow` /
  `VerifyAnchorInWindow` take it as the first parameter, and `verifyDeliveryAnchors`,
  `OpenRelaySession` and `tokenChargeFor` pass `n.chainID()`.
- **Withdrawers** read it from their own chain: `AcquireDemandTokenInWindow`,
  `AcquireRelayAnchors`, `AcquireCredits`.
- **The one exception is the D3 private path, and it is the interesting one.**
  `client.WithdrawDemandTokenPrivately` builds an EPHEMERAL node that holds no chain by
  construction, so `n.chainID()` there is the zero hash and the withdrawal would refuse.
  The chain id now travels down beside `issuerPub` and `epoch` — which that function's own
  doc already describes as *"the PARENT'S RESOLUTION, NOT THE ISSUER'S SAY-SO"*. It is still
  supplied by a chain-holder, never by the request, and the wire is unchanged: it carries
  the blinded value and the epoch, exactly as before.
- **Structurally enforced:** every bound function takes the chain id as a REQUIRED
  parameter, so a missed site is a compile error, and
  `core/demand/TestTokenCarriesNoChainIDField` fails if anyone adds a `ChainID` field to
  `demand.Token` — the natural way to "make the binding explicit on the wire", and exactly
  the decoration §3.4 forbids.

## Version-bump observability

A retired domain string is part of the FDH input, so an old signature simply fails under the
new domain — refusal, not silent re-interpretation.
`core/blindtoken/TestRetiredDomainVersionsAreRefused` hand-mints a signature under each
retired domain and layout, asserts it is genuine **in its own domain** first, then asserts
the live verifier refuses it, and pins the live constant to the new literal. The literals are
written out in the test, not read from the package.

## Blindness, unforgeability, unlinkability — not re-litigated

Settled by the certification §3.1–§3.3 and not re-opened here: blindness rests entirely on
the distribution of `r` and no term mentions the message; unforgeability under one key is the
BNPS 2003 bound the tree already cites in `relayAnchorDomain`'s doc; the anonymity-set
partition nets to zero at every verifier because a verifier only ever evaluates under its own
keyset.

## FINDING — the publish-token domain is a CONSENSUS rule, and the certification classifies it as economic

The certification's landing table calls M3 *"economic mechanism. No genesis, no consensus"*
and §3.6 lists `fdhDomain` (the quorum publish token) as one of four domains to bind. **For
that one domain the classification is wrong at source.** `blindtoken.Verify` is reached from
`publishtoken.Verify`, which is called from **block-validity predicates**:

- `(*Chain).ValidateEntry` — `core/chain/chain.go`, under `c.tokenQuorum > 0`;
- `v5ValidateEntry` — `core/chain/validate_v5_predicates.go`, under `p.TokenQuorum > 0`.

Changing what a publish-token signature covers changes **which blocks are valid**. Two honest
operators on different builds would disagree on entry validity — canon rule 8's scheduled
fork, not a latent defect. It is therefore a consensus-rule change: research-gated, and it
requires editing `core/chain/`, which this build was explicitly told not to touch.

**Shipped:** the three non-consensus domains. **Not shipped, and named in the source at
`fdhDomain`'s doc:** the publish-token binding, with the open cross-network consequence
stated — a publish token minted on network X is a free publish on network Y under a shared
issuer key. It is NOT filed as an `R-*` residual: canon rule 4 requires an owner, a closer and
a Boulder for a register row, and `ROADMAP.md` is outside this change's file scope. It is
named at the constant and reported to the planner to route.

The three shipped bindings are independent of the fourth: none of them touches consensus,
each closes a live cross-network replay on its own, and shipping them does not foreclose or
complicate the fourth.

## Honest-path compositions M3 changed, each found by a driven test

1. **The D3 ephemeral withdrawal** (above) — the chain id now travels with `issuerPub`.
2. **A chainless credit issuer.** `core/node`'s `newIssuerNode` fixture and `sim`'s
   `TestPrepaidCreditDecouplesFeeOverTheNetwork` ran publish-credit issuance on nodes with
   no chain. The credit lane is a network's lane now, so both fixtures were given one. In
   production a publisher holds a chain — it needs one to publish an entry at all.
3. **Fixtures that mint a token by hand and present it to a node** must mint under THAT
   node's chain id. Where two arms of a test build different genesis blocks
   (`open_no_issuer_key_test.go`), the anchor is now minted on the network that will accept
   it, and the test says why.
