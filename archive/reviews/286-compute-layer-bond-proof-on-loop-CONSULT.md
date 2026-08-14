# Research consult — the #286 genesis wedge has a compute layer: bond-proof work saturates the single consensus loop

**From:** silt build team
**To:** research team
**Re:** #286 (objective-chain cold-start genesis wedge), reopened after the first
*confirming* 3-region GCP field-test run (2026-08-12). Target `main @ 9f4403a`
(L1 async publish #328, L2a `-persistent-peers` #331, L2b byte-budget genesis #336
all merged).

---

## Why this doc exists

The confirming multi-region run **disproved** "genesis is fixed." After the three
*network* layers were merged, a fresh 4-validator objective chain **still wedged at
height 0** over real WAN. We root-caused it live (build-immutable #6) to a **fourth,
compute layer** the prior in-process tests could not see: **bond proof-of-space-time
work runs on the single serialized consensus loop (B2), and at a production bond size
on a modest validator it saturates the CPU and starves the consensus gather.** No
block ever forms.

The **fix direction touches things we do not own unilaterally** — the single-loop core
invariant (B2), the Sybil-cost meaning of bond size (C1 / immutable #4), and possibly
the proof construction itself (#299). Hence this consult: mechanism with code refs,
why it's partly research's call, the options, our lean, and the specific asks.

---

## Ground truth first (what the run proved, and the decisive experiment)

**Setup.** 13-node cloudtest, 4 objective validators (`-quorum 2`, byzantine-quorum on,
`-mature-validators 4`, all anchors), `-bond 64M`, `BOND_MODE=fast`, `-bond-audit 30s`,
across us-west1 / us-east1 / europe-west1 on **e2-small (2 vCPU)** VMs.

**Confirmed, not guessed:**
- **NOT a network layer.** L2a is live-confirmed: `discovery: 3 peer(s) via -persistent-peers`,
  `persistent-peers: 3 configured (static, never-evicted)`; cross-region TCP reachable.
  Bonds & standing are recognized (`standing self … reputation=1024`,
  `bond challenge … passed=true`). The gather **never starts** (zero gather markers in
  `debug.log`), so it is not L2b block size either.
- **A CPU-saturated core loop.** All 4 validators pin ~2 full cores (94 / 83 / 82 / 34 %).
  val-a's loop went **silent for 5+ minutes** while burning 2 cores — one blocking
  computation, not a retry storm (0 log lines emitted). **Every** bond challenge is
  `late=true`, firing *minutes* apart against the 30 s `-bond-audit` interval — the loop
  cannot keep pace with its own audit, let alone run consensus.
- **Decisive single-variable proof.** Reduce the bond **64M → 2M** on all four validators
  (32× less proof compute, nothing else changed): **CPU ~90 % → ~3 %, and genesis committed
  immediately** — `head height: 1` on all four validators across all three regions, publish
  returned a `silt:v1:` link. Same WAN, only the bond-proof compute changed.

**Conclusion:** the remaining genesis blocker is compute, and **bond size (⇒ proof compute)
is the lever.**

---

## The mechanism (code-level, honest about what we did and didn't isolate)

The proof is asymmetric by design, and the asymmetry is the problem:

- **Verification is cheap and sub-linear** — `O((Samples + 5k)·log n)`, sampled, no fetch,
  no VDF re-run (`core/bond/bond.go` header; `DefaultLabelSamples=64`). So the *verifier*
  side is **not** the bottleneck.
- **The prover and the sealer are expensive and run on the node loop:**
  - **Prover:** `Node.answerBondChallenge` (`core/node/bondaudit.go:271`) →
    `Commitment.AnswerSpaceTime` (`core/bond/bond.go`) runs a **sequential VDF**
    (`vdf.Eval(p, …, BondVDFDelay)`) on **every** challenge, on the loop. Fixed-time (not
    size-scaling), but it is deliberately sequential wall-clock work executed inline, once
    per challenging peer per `-bond-audit` epoch.
  - **Sealer:** producing the depth-robust-labeled plot is the **Ω(n) memory-hard recompute**
    the design intends (`core/bond/bond.go:40-52`), and it scales with bond size — the natural
    explanation for the 32× lever and the multi-minute post-restart silence.

**What we confirmed:** bond size is the decisive lever; the loop is CPU-saturated; the gather
never starts; verification is not the cost.

**What we did NOT isolate (needs a pprof pass, build team can do this):** the exact split
between *one-time sealing* (Ω(n), at boot) and *ongoing per-challenge VDF/plot work* (every
30 s), and whether `BOND_MODE=fast` trades stored plot for on-demand recompute (which would
make every challenge size-scaling). **This does not change the fix direction** — both live on
the consensus loop — but it changes *which* mitigation is primary.

---

## Why this is (partly) research's call, not just a build fix

The obvious build reflex — "move the crypto off the loop" — is probably right, but three
constraints make the framing research's:

1. **B2 (single-loop core) is a build-immutable.** "Node logic runs on one serialized loop:
   no locks, no goroutines in the core." Is bond prove/seal **core logic** (must stay on the
   loop, determinism-critical) or an **adapter/effect** (like disk or network — legitimately
   off-loop, delivering a verified result back as an event)? We read it as the latter (it is a
   pure function of a sealed blob + a nonce, and its *result* is what the loop needs), but this
   is an invariant call, not ours to make silently.
2. **Bond size is a security parameter (C1 / immutable #4).** We cannot "just shrink the bond"
   — bond size *is* the Sybil cost. And immutable #4 says cheap honest participation is a
   security constraint: **if a production-size bond pins a 2-vCPU hobbyist validator and
   wedges consensus, that is a regression against silt's reason to exist**, even though the
   crypto is sound. The proof-compute-vs-bond-size curve is a soundness/economics tradeoff you
   own.
3. **#299 (succinct proof) is the structural close.** A SNARK/aggregated proof would make both
   the payload (the original #299 motivation) *and* — if applicable — the prover/sealer cost
   bond-size-independent. Whether #299 subsumes this compute layer is a construction question.

---

## Options

- **A — Off-loop bond prover/sealer (build-led, B2 blessing needed).** Run `AnswerSpaceTime`
  and plot sealing on a dedicated worker; the loop dispatches a challenge and consumes the
  answer as an event, never blocking. Preserves determinism (results are content-addressed).
  Fastest unblock; needs research/owner sign-off that this honors B2.
- **B — Size-independent proof cost (research-led).** Cap or decouple the per-challenge /
  per-seal compute from bond size via proof parameters (sampling the plot rather than
  recomputing, tuning `BondVDFDelay` / `BondLabelSamples`, or a stored-plot fast path that
  never recomputes), holding the Sybil-cost meaning of the bond fixed. This is the honest
  fix if the compute *should not* scale with size in the first place.
- **C — #299 succinct/aggregated proof (structural, larger).** Fold this into #299 so
  verification *and* proving are constant-ish and the compute layer disappears with the
  payload layer.
- **D — Do nothing / raise the field-test VM size.** Rejected as a *fix* (it only masks the
  immutable-#4 concern), but we note it so the confirming run can be *re-greened* on a bigger
  machine type to unblock the rest of the field-test suite while the real fix lands.

---

## Our lean

**A now, B/C as the durable answer.** Getting bond prove/seal off the consensus loop is the
immediate, B2-compatible unblock and is almost certainly correct regardless — heavy,
size-scaling crypto on the serialized consensus loop is the actual defect the run exposed.
But **A alone is insufficient if the per-challenge cost genuinely scales with bond size**:
off-loop or not, a validator that spends minutes per audit epoch re-deriving a large plot
violates immutable #4's spirit and will not sustain a real network. So we want **A to unblock
+ your ruling on B/C** for the size-independence question.

---

## Specific asks

1. **B2 ruling:** is moving bond prove/seal off the consensus loop (results returned as
   content-addressed events) consistent with B2, or does determinism require it stay on-loop?
   (If off-loop is blessed, we build A immediately.)
2. **Size-independence:** *should* honest per-challenge and per-seal bond compute scale with
   bond size at all? If not, is the fix a stored-plot fast path (never recompute), a parameter
   retune, or does it require #299? Give us the direction; we will not guess proof params.
3. **Immutable-#4 envelope:** what is the intended honest-validator hardware floor, so we can
   set a bond-size / proof-cost envelope that a hobbyist node sustains while `-bond-audit`
   runs *and* consensus proceeds? (Today, 64M on 2 vCPU does not.)
4. **#299 scope:** does the succinct-proof work already subsume this compute layer, or is an
   off-loop/stored-plot fix needed independently of it?

---

## Building without consult (for your awareness — no decision needed)

- The **pprof isolation** (sealing-vs-challenge, fast-mode recompute) — build team, to sharpen
  the mechanism before implementing A.
- **Re-greening the confirming run** on a larger machine type to certify the *rest* of the
  field-test suite (everything downstream of a working chain) in parallel with the real fix —
  purely a test-substrate change, not a product fix.
- The **network layers (L1/L2a/L2b)** stand; L2a is cloud-confirmed. This consult does not
  reopen them.

---

## Provenance

Confirming GCP run 2026-08-12, `main @ 9f4403a`, 13-node 3-region cloudtest, e2-small.
Live root-cause + the 64M→2M decisive experiment recorded on **#286** (reopen comment) and
**#338**. All GCP resources torn down (25 destroyed, 0 residual). Code refs:
`core/node/bondaudit.go` (`answerBondChallenge`, `bondAuditOnce`), `core/bond/bond.go`
(`AnswerSpaceTime`, the Ω(n) recompute + `O((Samples+5k)·log n)` verify), `core/node/node.go`
(`BondVDFDelay`, `BondLabelSamples=64`). Standing process rule that produced this:
[read research before inventing perf/network numbers] + build-immutable #6 (root-cause first).
