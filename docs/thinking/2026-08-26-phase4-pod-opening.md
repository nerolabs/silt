# Phase 4 opening — PoD spec-first: the options and the call

**Date:** 2026-08-26. **Context:** Phases 1–3 are banked (`fe2376a-deep`, 30/1/0).
The ROADMAP's post-Phase-3 order names the opening move: "the PoD spec + research
consult (spec first, never a switch-flip; the receipt-forgeability residual B3 is
the prerequisite to close on paper)." This note records the deliberation before any
document or code is produced (PACE BEFORE CODE).

## The options

**A — Build the neutral lane now.** Wire the inert `DeliveryReceipt` into a
balance-lane bandwidth credit (the D-TIERING §8 "neutral form"), then spec.

- Benefit: fastest visible motion; the seams are verified additive
  (`MEMO-pod-sequencing-and-accommodation-2026-08-25.md`).
- Cost: violates the ratified sequencing (D-M1-PIVOT: spec first). Wiring a
  consumer is exactly the step that makes B3 live — the receipt is inert *only*
  while demand has no consumer. Economy parameters (fee routing, skim
  interaction) are research-gated by standing rule.

**B — Spec + research consult first, no code.** Write the PoD design doc closing
B3 on paper, file the consult, build only after certification.

- Benefit: honors the ratified order; the B3 close turns out to be an
  *argument* (a conservation invariant) more than a mechanism, which is exactly
  what a consult can certify cheaply; zero risk of shipping an economy knob
  research later moves.
- Cost: one research round-trip of latency before the (small) build.

**C — Verifiable-escrow desk study only.** Run the PE-recommended 1–2 day study
on the strong form's crypto wall and defer the neutral lane.

- Benefit: retires the roadmap's biggest unknown-unknown.
- Cost: answers the *strong* form only, which is double-gated (crypto + #182)
  and off the critical path; leaves the actual Phase 4 prerequisite (B3 on
  paper) untouched.

## The call

**B, with C folded in as a consult question.** The spec and consult are one
deliverable; the desk study is a question in the same consult rather than a
separate engagement. This matches the ROADMAP line verbatim and the PE memo's
recommendation ("spend it on a bounded desk study … even that is not urgent").

## What the code evidence forced into the spec (the non-obvious findings)

Verified against HEAD `d9635c4` while drafting:

1. **The serve path already pays for bandwidth — unwitnessed.**
   `core/node/node.go:1543-1545` credits 1 credit/byte on every chunk serve
   (`RecordServe` / `RecordServeToObject`, less the 1/8 durability skim,
   `core/credit/escrow.go:128`). The ledger is per-node bookkeeping; nothing
   attests the serve happened. PoD's neutral lane is therefore not a new
   payment — it is the *witnessed* form of an existing one, for the surfaces
   where self-recording is worthless (relay/gateway compensation across
   operators).
2. **B3's forgeability is narrower than the residual's one-liner reads.** The
   receipt binds three ways (`core/demand/demand.go:145-149`): token spend
   (issuer-signed, fee at withdrawal), fetcher signature, and the PoR proof.
   Only the PoR leg is forgeable (public per-object key seed,
   `demand.go:104-110`). A zero-byte forgery still costs a real token and a
   willing fetcher signature — so the exposure is collusion economics, not
   open minting.
3. **The fetcher already verifies the bytes it acks.** Every read re-verifies
   against the content address (tenet B3). A fetcher-signed receipt is
   therefore already an attestation of *correct* delivery by a party that just
   checked; the PoR leg adds only "held at ack time," whose anti-collusion
   value is nil (a colluding fetcher signs anyway). Whether the PoR leg is
   load-bearing *at all* in the neutral lane is a real question — put to
   research rather than assumed either way.

These three facts produce the spec's central claim: **the neutral lane's B3
close is a conservation invariant (no minting per receipt; credits only move,
minus skim), with the crypto close needed only where the strong form was
already gated.** If research confirms it, the paper prerequisite closes without
new cryptography. If research refutes it, we learn which leg must be built
before any consumer is wired — still on paper, still pre-build.

## What ships in this PR

- `docs/design/pod.md` — the PoD spec draft (status: research-gated).
- This deliberation note.
- The consult filed at
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/PoD-neutral-lane-B3-close-CONSULT-2026-08-26.md`
  (outside the repo; referenced from the spec).

No code. The build items (mode flags, the receipt wiring) stay behind the
consult per the ROADMAP order.
