# Contributing to Silt

Thanks for wanting to help build Silt. It's neutral storage
infrastructure — content-addressed, erasure-coded, encrypted at every
level, run by its participants and owned by none. This guide is how work
gets in.

## Ground rules

- **The infrastructure is not the content.** Keep it that way: core code
  must never be able to read, identify, or attach meaning to what the
  network carries. See [GOVERNANCE.md](GOVERNANCE.md) and immutable #6 in
  [docs/TENETS.md](docs/TENETS.md).
- **Core stays pure.** Packages under `core/` and `ports/` import no
  adapters and no effects (`os`, `net`, `time`, ambient randomness) —
  enforced by `go test ./internal/depcheck`. Effects live in `adapters/`
  behind interfaces in `ports/`.
- **Determinism is sacred.** The simulator runs single-threaded and
  seeded; a given seed reproduces byte-for-byte. Don't add goroutines or
  map-iteration-order dependence to core, node, dht, or sim paths.

## Build and test

```sh
go build ./...
go test -timeout 40m ./...    # the whole suite, incl. deterministic sims; -timeout: core/chain has a ~5-min measurement test
go test -short ./...          # what CI runs (the measurement tests are skipped under -short)
go vet ./... && gofmt -l .     # must be clean
go test -bench . ./core/...    # throughput numbers, if you touched hot paths
```

Try a change end to end with the sims, e.g. `go run ./cmd/silt sim run
churn` or `... takedown`.

## The flow

Nothing reaches `main` or a release without a pull request, green CI, and
review. The branch is protected to enforce it.

1. **Branch** off `main` (`feat/…`, `fix/…`, `chore/…`), or fork.
2. **Open a PR.** CI runs automatically: `go vet`, `gofmt`, the full test
   suite with its coverage floor, a race-detector build, the multi-process
   e2e suite, the cross-NAT integration jobs, and the repo lints under
   `scripts/`. All must pass.
3. **Keep the canon true.** If you change behaviour, make sure
   [docs/TENETS.md](docs/TENETS.md) and [docs/VISION.md](docs/VISION.md) still
   describe what the system does. They are the only two documents that govern
   this project; there are no others.
4. **Review.** Every PR gets a maintainer review (currently
   [@nerolabs](https://github.com/nerolabs)). Address the findings.
5. **Merge.** Squash-merge once approved and green. Delete the branch.

## Commits & PRs

- Small, focused PRs review faster than large ones.
- Write commit messages that explain *why*, not just *what*.
- The PR template asks what changed, how you tested it, which tiers you
  covered, and any safety or abuse implications — fill it in honestly.

## Safety

Silt is designed to be governable without silt itself becoming a surveillance
tool — access-privacy is pursued to the anonymity trilemma's metadata-layer
limit, not claimed as an absolute (immutable #4 in
[docs/TENETS.md](docs/TENETS.md)). If a change touches
storage, serving, the chain, or the takedown path, call out the
safety implications in your PR. To report a security vulnerability,
**do not open a public issue** — a private disclosure path will be
published; until then, contact the maintainer directly.

## Scope

Silt is infrastructure only. Anything that resolves opaque identifiers
into human meaning — search, browsing, catalogs, names — belongs to a
separate layer, not this repository. PRs that blur that line will be
asked to move.
