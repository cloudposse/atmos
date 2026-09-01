# Fix: `[race] full test suite` CI job timeouts and a shuffle-order test-isolation bug

**Date:** 2026-09-01

## Summary

The new `[race] full test suite` CI job (added to run `atmos test race` on pull requests)
failed on its first two real runs, for three separate reasons: the package list included the
CLI acceptance suite (which is deliberately sharded elsewhere because it takes ~90 minutes
unsharded), `pkg/toolchain`'s real-network registry tests didn't fit the per-package timeout
once running unsharded and unauthenticated, and `-shuffle=on` exposed a pre-existing
test-isolation bug in `pkg/utils`.

## Context

`atmos test race` (`.atmos.d/test.yaml`) runs `go test -race -shuffle=on $(go list ./...)`.
Wiring it into CI (`.github/workflows/test.yml`) surfaced three latent problems that had never
been exercised together before:

1. `$(go list ./...)` included `github.com/cloudposse/atmos/tests` and `tests/testhelpers` —
   the CLI acceptance suite. The `test` job's own matrix comment measures that suite's
   unsharded runtime at ~90 minutes on Linux, which is exactly why that job shards it 10 ways
   per OS. Run whole inside a single `-timeout 10m` package budget, it panicked with
   `test timed out after 10m0s` inside `tests/testhelpers`'s `TestAtmosRunner_buildWithCoverage`
   (stuck waiting on a `go build` subprocess) and separately in `tests` itself. It also builds
   and shells out to a plain (non-`-race`) `atmos` binary, so instrumenting the driving test
   process with `-race` provided no race coverage on the binary under test anyway.
2. With `tests/...` excluded, the next run still timed out: `github.com/cloudposse/atmos/pkg/toolchain`
   hit `test timed out after 10m0s`. That package's tests install real tool binaries from real
   registries (no mock seam — `resolveLatestVersionWithSpinner` calls `NewAquaRegistry()` with
   no test-double override) and hit the real GitHub API. Unlike the `test` job's acceptance
   steps (which already set `GITHUB_TOKEN` for exactly this reason), the new race job ran every
   package unauthenticated and unsharded, so every package's network calls competed for the
   same runner and the same IP-wide 60/hr unauthenticated GitHub rate limit, instead of getting
   a shard's worth of headroom the way the `test` job's packages do.
3. That same run's log also showed a genuine (unrelated) bug: `pkg/utils`'s
   `TestClearInternPool` asserted `GetInternStats().Requests == 3` without first clearing the
   package-level intern pool, silently relying on running before any other test in the package
   interned a string. `-shuffle=on` randomizes test order, so once another test ran first the
   assertion failed (`expected: 3, actual: 17`). This is the first time `-shuffle=on` had run
   against the full unit-test suite in CI — nothing before now enabled shuffle for anything but
   `tests/cli_test.go`'s acceptance suite.

## Changes

- `.atmos.d/test.yaml`: exclude `./tests/...` from the `race` command's package list
  (`go list ./... | grep -v '^github.com/cloudposse/atmos/tests'`); raise its `-timeout` from
  the default 10m to 20m to give unsharded, contention-heavy packages like `pkg/toolchain`
  headroom; updated the `--race` flag/subcommand description to match (previously said "quick
  tests" despite always running the full suite, then said "full test suite" before this fix
  scoped it down to excluding `tests/...`).
- `.github/workflows/test.yml`: added the `race` job (ubuntu-latest, `libudev-dev`/`pkg-config`
  installed for the `CGO_ENABLED=1` build — see the job's own comment for that trap); its
  "Run atmos test race" step now sets `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`, matching the
  `test` job's acceptance steps.
- `pkg/utils/string_utils_test.go`: `TestClearInternPool` now calls `ClearInternPool()` before
  interning anything, matching the pattern already used by the adjacent `TestResetInternStats`.

## Validation

- `go test -race -shuffle=on ./pkg/utils/... -run 'TestClearInternPool|TestIntern|TestResetInternStats' -v -count=3`
  — all pass across 3 shuffled orderings (previously failed under some orderings).
- `go test -race -shuffle=on ./pkg/utils/... -count=1` — full package passes.
- `go build ./...` — clean.
- `python3 -c "import yaml; yaml.safe_load(...)"` and `actionlint .github/workflows/test.yml`
  — both workflow/command YAML files parse and lint clean.
- Did not reproduce the `pkg/toolchain` timeout locally: a local `go test -race -shuffle=on
  ./pkg/toolchain/...` run stalled at near-zero CPU usage in this sandboxed environment (likely
  restricted/proxied network egress here, unrelated to GitHub Actions), so it was killed rather
  than trusted as a timing signal. The `-timeout 20m` and `GITHUB_TOKEN` changes are reasoned
  from the CI log evidence (bounded retry/backoff math, the `test` job's own precedent for
  needing `GITHUB_TOKEN`) rather than confirmed by a clean local repro. The next real CI run of
  this job is the actual validation and should be checked.

## Follow-ups

None.
