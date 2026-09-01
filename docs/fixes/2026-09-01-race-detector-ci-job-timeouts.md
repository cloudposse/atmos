# Fix: `[race] full test suite` CI job — timeouts, a shuffle-order test bug, and a real data race

**Date:** 2026-09-01

## Summary

The new `[race] full test suite` CI job (added to run `atmos test race` on pull requests)
failed on its first three real runs. Rounds 1–2: the package list included the CLI acceptance
suite (deliberately sharded elsewhere because it takes ~90 minutes unsharded),
`pkg/toolchain`'s real-network registry tests didn't fit the per-package timeout once running
unsharded and unauthenticated, and `-shuffle=on` exposed a pre-existing test-isolation bug in
`pkg/utils`. Round 3, with the first two rounds' fixes in place: the job caught a genuine
production data race in `pkg/toolchain`'s concurrent batch installer (exactly what this job
exists to catch), plus a second `-shuffle=on`-exposed test-isolation bug, this time in
`pkg/toolchain` itself.

## Context

`atmos test race` (`.atmos.d/test.yaml`) runs `go test -race -shuffle=on $(go list ./...)`.
Wiring it into CI (`.github/workflows/test.yml`) surfaced three latent problems that had never
been exercised together before:

- `$(go list ./...)` included `github.com/cloudposse/atmos/tests` and `tests/testhelpers` —
  the CLI acceptance suite. The `test` job's own matrix comment measures that suite's
  unsharded runtime at ~90 minutes on Linux, which is exactly why that job shards it 10 ways
  per OS. Run whole inside a single `-timeout 10m` package budget, it panicked with
  `test timed out after 10m0s` inside `tests/testhelpers`'s `TestAtmosRunner_buildWithCoverage`
  (stuck waiting on a `go build` subprocess) and separately in `tests` itself. It also builds
  and shells out to a plain (non-`-race`) `atmos` binary, so instrumenting the driving test
  process with `-race` provided no race coverage on the binary under test anyway.
- With `tests/...` excluded, the next run still timed out: `github.com/cloudposse/atmos/pkg/toolchain`
  hit `test timed out after 10m0s`. That package's tests install real tool binaries from real
  registries (no mock seam — `resolveLatestVersionWithSpinner` calls `NewAquaRegistry()` with
  no test-double override) and hit the real GitHub API. Unlike the `test` job's acceptance
  steps (which already set `GITHUB_TOKEN` for exactly this reason), the new race job ran every
  package unauthenticated and unsharded, so every package's network calls competed for the
  same runner and the same IP-wide 60/hr unauthenticated GitHub rate limit, instead of getting
  a shard's worth of headroom the way the `test` job's packages do.
- That same run's log also showed a genuine (unrelated) bug: `pkg/utils`'s
  `TestClearInternPool` asserted `GetInternStats().Requests == 3` without first clearing the
  package-level intern pool, silently relying on running before any other test in the package
  interned a string. `-shuffle=on` randomizes test order, so once another test ran first the
  assertion failed (`expected: 3, actual: 17`). This is the first time `-shuffle=on` had run
  against the full unit-test suite in CI — nothing before now enabled shuffle for anything but
  `tests/cli_test.go`'s acceptance suite.
- Round 3, after the above landed: `pkg/toolchain` reported `WARNING: DATA RACE` inside
  `TestRunInstallWithNoArgs`, between two of the concurrent batch installer's own worker
  goroutines — one calling `pkg/ui/theme.getActiveThemeName()` (writes via `viper.BindEnv`,
  called "on demand" on every styled render) and another calling
  `pkg/http.GetGitHubTokenFromEnv()` (reads via `viper.GetString`), both against the process-wide
  global `viper` singleton. `pkg/config` already has a `SafeViper` mutex-guard for exactly this
  class of problem ("spf13/viper has no internal locking of its own", per its own doc comment,
  written for the DAG scheduler's concurrent `LoadConfig` calls), but `pkg/http` and
  `pkg/ui/theme` sit *below* `pkg/config` in the import graph (confirmed via `go list -deps`:
  `pkg/config` already transitively depends on both), so they cannot import `pkg/config` to reach
  it without an import cycle — which is exactly why these two call sites were still calling
  `viper.*` directly.
- The same round's log also showed `TestGitHubTokenEnvBinding/TestMain_binds_environment_correctly`
  failing (`expected: "<MASKED>", actual: ""`) — a second, unrelated `-shuffle=on` test-isolation
  bug. `main_test.go`'s `TestMain` binds `"github-token"` to `ATMOS_GITHUB_TOKEN`/`GITHUB_TOKEN`
  once at process start; `set_test.go`'s `teardownTest()` calls `viper.Reset()`, which discards
  that binding for every test that runs afterward in the same process. Previously this was inert
  because neither env var was ever set in CI; adding `GITHUB_TOKEN` in round 2 (above) made the
  previously-dormant assertion in `TestGitHubTokenEnvBinding` actually run, and `-shuffle=on`
  meant `set_test.go`'s reset could land before it.

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
- New `pkg/viperguard` package: a leaf package (no Atmos-internal imports) mutex-guarding the
  global `viper` singleton's `Set`/`BindEnv`/`GetString`/`GetBool`/`GetStringSlice`/`IsSet`/`View`,
  usable from packages below `pkg/config` in the import graph. `pkg/config/global_viper.go`'s
  `SafeViper` now delegates every method to `pkg/viperguard` instead of keeping its own
  independent mutex — two separate locks guarding the same underlying viper singleton would not
  exclude each other, leaving the exact cross-package race this closes. All 8 existing
  `cfg.GlobalViper()` call sites (`cmd/terraform/utils.go`, `internal/exec/shell_utils.go`,
  `pkg/config/edition.go`, `pkg/config/load.go`, `pkg/auth/profile_fallback.go`) are unchanged —
  `SafeViper`'s public method signatures and `ViperReader` (now a type alias) are unchanged.
- `pkg/ui/theme/styles.go`: `getActiveThemeName()`'s `viper.BindEnv`/`IsSet`/`GetString` calls
  now go through `pkg/viperguard`.
- `pkg/http/client.go`: `GetGitHubTokenFromEnv()`'s global-instance `viper.GetString("github-token")`
  call now goes through `pkg/viperguard`; the caller-supplied-`*viper.Viper` override path (used
  in tests to avoid mutating shared global state) is untouched, since that instance is never
  shared and was never part of the race.
- `pkg/toolchain/set_test.go`: `teardownTest()` now re-binds `"github-token"` after
  `viper.Reset()`, so it no longer leaves the global instance de-bound for whatever test the
  shuffled order runs next.
- `pkg/toolchain/github_token_test.go`: `TestMain_binds_environment_correctly` now re-binds
  `"github-token"` defensively before asserting on it, rather than assuming `TestMain`'s
  one-time binding survived every sibling test that happened to run first.

## Validation

- `go test -race -shuffle=on ./pkg/utils/... -run 'TestClearInternPool|TestIntern|TestResetInternStats' -v -count=3`
  — all pass across 3 shuffled orderings (previously failed under some orderings).
- `go test -race -shuffle=on ./pkg/utils/... -count=1` — full package passes.
- `go build ./...` — clean.
- `python3 -c "import yaml; yaml.safe_load(...)"` and `actionlint .github/workflows/test.yml`
  — both workflow/command YAML files parse and lint clean.
- Did not reproduce the `pkg/toolchain` timeout locally (both rounds 2 and 3): a full
  `go test -race -shuffle=on ./pkg/toolchain/...` run stalls at near-zero CPU usage in this
  sandboxed environment (likely restricted/proxied network egress here, unrelated to GitHub
  Actions), so it was killed rather than trusted as a timing signal. The `-timeout 20m` and
  `GITHUB_TOKEN` changes are reasoned from the CI log evidence rather than confirmed by a clean
  local full-package repro. The next real CI run of this job is the actual validation for that
  part of the fix and should be checked.
- `go build ./...` — clean (confirms no import cycle from `pkg/http`/`pkg/ui/theme` importing
  `pkg/viperguard`, and `pkg/config`'s delegation compiles).
- `go vet ./pkg/viperguard/... ./pkg/config/... ./pkg/http/... ./pkg/ui/theme/...` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (includes the `lintroller`
  `perf.Track` mandate on every new `pkg/viperguard` public function).
- `gofumpt -l` on every changed/new Go file — no output (already formatted).
- New `pkg/viperguard/viperguard_test.go`'s `TestConcurrentBindEnvAndGet` reproduces the exact
  shape of the caught race (concurrent `BindEnv` + `GetString` + `Set` + `GetStringSlice`/`IsSet`
  against the global singleton) — passes under `go test -race -shuffle=on -count=3`. Sanity-checked
  the test actually detects this class of race: a throwaway copy calling bare `viper.BindEnv`/
  `viper.GetString` directly (bypassing `pkg/viperguard`) reliably fails with `WARNING: DATA RACE`
  under `-race`; the real test, going through `pkg/viperguard`, does not.
- `go test -race -shuffle=on ./pkg/config/... ./pkg/http/... ./pkg/ui/theme/... -count=1` — all
  packages pass, including the pre-existing `pkg/config/global_viper_test.go` suite (proving
  `SafeViper`'s delegation preserves its documented locking guarantees: `View` is atomic,
  `GetStringSlice` still clones, `ViperReader` still can't be type-asserted back to `*viper.Viper`).
- `go test -race -shuffle=on ./pkg/toolchain/... -run 'TestGitHubTokenEnvBinding' -v -count=3` —
  all pass across 3 shuffled orderings (previously failed under some orderings/once `GITHUB_TOKEN`
  was actually set).
- Did not get a clean full-package `pkg/toolchain` run locally (see above) to directly confirm
  `TestRunInstallWithNoArgs` no longer races; the next real CI run is the actual validation for
  that specific test, though `TestConcurrentBindEnvAndGet` exercises the identical race shape.

## Follow-ups

None.
