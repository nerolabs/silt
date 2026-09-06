# 2026-09-06 — Relay lane re-price: `RelayIncrementBytes` 4 KiB → 512 KiB, ceilings derived

**Decision.** Move `RelayIncrementBytes` from 4,096 to 524,288 and make `MaxChainLength` and
`MaxSessionBytes` derived from the anchor face, in one PR across the nine sites the certification
names (§4.1). Owner-ratified 2026-09-06 ("2. ratified") on the Researcher's certified value
(`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/G-R212-2-relay-lane-reprice-RESEARCH-CERTIFICATION-2026-09-06.md`
§8). Recorded in `docs/decisions.md` (D-R2.9a-RUN-CALLS, the G-R212-2 paragraph).

## The problem

The relay lane's price is `RelayIncrementCredit / RelayIncrementBytes` credits per byte. At the
2026-08-30 pin (1 credit per 4 KiB) the 500,000-credit starter grant bought 1.9 GiB of relayed
fetch. The 64 GiB `grant/r` pin (D-R2.9a-RUN-CALLS) and its 44.7 GiB structural floor read on the
SUM of the prices a NAT'd fetcher pays, so the relay price alone put a NAT'd pony 23.4× below the
floor. G-R212-2 blocked era-4 activation of the lane until this was resolved.

## Options weighed

| Option | Cost | Why not / why |
|---|---|---|
| (a) Raise the price constant alone | Per-anchor yield is `min(face·B/credit, MaxSessionBytes)` with the remainder BURNED; against a fixed 1 GiB cap a 24× raise burns 95.6 % of every payment | REFUTED by the certification (T-RELAY-GRAN): a price change must move the cap with it |
| (b) Certified derivation that #4 does not bind a relayed fetch | Route (b) holds only while the free splice is unconditional and no production path can open a paid session | CERTIFIED as the reason the lane no longer blocks era-4, but it is a load-bearing condition, not a price; a differential or forced paid path re-arms #4 |
| (c) Re-price AND derive the ceilings from the face | Nine sites move together; `MaxAnchorsPerSession` collapses to 1; two tests lose their "S above face" premise | CHOSEN: closes the geometry (244 GiB per grant), keeps every ratified sentence honest, is face-neutral (ONE-FACE not engaged) |

Bounds on the value (cert §4.2): below by the 64 GiB pin on the summed prices and by Don't #7 (at
256 KiB the relay out-earns the server per byte); above by T-AR (`B ≤ 1,048,576`). 512 KiB is the
one power of two inside the interval.

## What the derivation changes structurally

- `MaxChainLength = ShippedAnchorFace / RelayIncrementCredit = 50,000`: one anchor funds the
  longest admissible chain, so `MaxAnchorsPerSession` derives to 1 and a chain can never exceed
  one face. The "S × inc over-pays a single anchor" surface R2.14 closed by bound is now closed by
  construction; the node-tier gate pins the identity instead of the ablation.
- `MaxSessionBytes = MaxChainLength × RelayIncrementBytes = 24.4 GiB`: exactly what a full chain
  authorizes, so nothing paid for is unforwardable.
- The relay adapter's per-splice cap (free AND paid) defaults to the protocol ceiling and `Serve`
  refuses a lower explicit cap. The certification is explicit that this is a coherence refusal, not
  a differential cap; a free/paid differential is what D-POD-RELAY-COEXIST refuses and what would
  re-arm #4 on the relay price.
- Fetcher-side chain state falls from 8 MB to 1.6 MB (S_max · 32 B); the AdvanceTo walk bound
  falls from ~262K to 50K hashes.

## Pressure-test

- **Does the 4 KiB pump chunk still hold?** Yes: `paidPump` releases at authorized-BYTE
  granularity, so the read buffer is a sub-increment detail; the stiff stays bounded to 4 KiB.
- **Integer width:** `MaxSessionBytes` is 2.6×10¹⁰, fine in `int`/`int64` on the 64-bit targets;
  `authBytes` is `int64`.
- **e2e:** `e2e/relay_paid_test.go` builds its object as S = 6 increments, now 3 MiB instead of
  24 KiB, forwarded over pipes; the run confirms.
- **Residual:** `R-LAMBDA-DUST` / G-R212-7 is NOT touched here — the strict-parity collision on the
  delivery lane stays with the owner.
