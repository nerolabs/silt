# silt

## The canon

Two documents govern this project. There are no others.

- `docs/VISION.md` — what silt is for.
- `docs/TENETS.md` — what it must never trade away.

`integration/rc/RC-PROPOSED.md` is a proposed release-candidate list. It is a proposal, not
canon, and it is expected to be cut apart.

## The build

Build the vision. Honor the tenets. Decide for yourself. Say when the release candidate is
ready to review.

## Code carries no process

Comments describe the product: what the code does, why this approach, what an invariant
means, and citations to external papers or specifications.

Comments never describe the decision process. No task identifiers, no lane or milestone
names, no session or pull-request numbers, no references to rulings, reviews, or
certifications. If a comment would only make sense to someone who read a document that no
longer exists, delete it.

The same rule binds identifiers, test names, and file names.

## Load discipline

This machine is shared with its owner's other work, and memory is the binding constraint,
not CPU.

- Every heavy command runs throttled: `taskpolicy -c background nice -n 19 go test …`, and
  the same prefix for `go build`, `go vet`, `go run`, and long scripts. On macOS `nice`
  alone does not throttle; the background QoS class does.
- `GOFLAGS=-p=1` and `GOMAXPROCS=2` are set in the session environment. Use `-short`
  locally; full suites are CI's job.
- Three heavy jobs at a time, maximum. Check load and swap headroom before starting one.
- Never leave a background process running. Sweep for the product binary at the end of a
  session, not just for test runners.

## Ground truth

Correctness is a command result, in this order: unit → consensus model-check →
integration → end-to-end under network impairment → field.

Capture evidence before teardown. A failure that cannot be reproduced gets instrumented,
not retried. A field run confirms a fix; it never discovers an invariant.

A demonstration that could not be driven is a failure, not a pass. "Skipped," "gap," and
"not run" are all failures.
