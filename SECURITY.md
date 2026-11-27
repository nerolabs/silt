# Security policy

silt is experimental, unaudited 0.x software. Its three central claims — that a publish cannot be
linked to the identity that publishes it, that removal is real but never global or by identity,
and that standing cannot be farmed at a discount — are claims under test, stated in
[`ADVERSARY.md`](ADVERSARY.md). Breaking one is the most useful thing you can do for this project.

## Reporting a vulnerability

Report privately through GitHub: on this repository's **Security** tab, choose **Report a
vulnerability**. Please do not open a public issue for a vulnerability.

Include what you did, what you expected, what happened, and how to reproduce it. A failing Go test
is the best reproduction; the repository's own adversarial tests show the shape.

## What to expect

- An acknowledgement within 7 days.
- A fix, or a reasoned reply, as soon as the work allows. We aim to publish within 90 days of your
  report, and sooner once a fix ships. We will agree the timing with you.
- Credit in the fix and the release notes, unless you ask not to be named.

There is no bug bounty.

## Scope

- The three claims in `ADVERSARY.md`, attacked any way you can construct.
- The `silt` node, daemon, CLI and web UI in this repository.

Out of scope: denial of service by volume alone, attacks that need control of the reporter's own
machine, and findings in third-party dependencies without a path through silt (report those
upstream).

## Safe harbour

Research done in good faith — on your own nodes or a local test network, without harming other
people's data, nodes or privacy — is welcome, and we will not pursue it. If in doubt, ask first
through the private report.
