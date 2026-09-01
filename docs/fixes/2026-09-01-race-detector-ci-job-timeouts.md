# Fix: `[race] full test suite` CI job — timeouts, shuffle-order test bugs, and real data races

**Date:** 2026-09-01

## Summary

The new `[race] full test suite` CI job (added to run `atmos test race` on pull requests)
failed on its first five real runs. Rounds 1–2: the package list included the CLI acceptance
suite (deliberately sharded elsewhere because it takes ~90 minutes unsharded),
`pkg/toolchain`'s real-network registry tests didn't fit the per-package timeout once running
unsharded and unauthenticated, and `-shuffle=on` exposed a pre-existing test-isolation bug in
`pkg/utils`. Round 3: a genuine production data race in `pkg/toolchain`'s concurrent batch
installer (exactly what this job exists to catch), plus a second `-shuffle=on`-exposed
test-isolation bug, this time in `pkg/toolchain` itself. Round 4, once the job stopped timing
out and started running the full suite to completion: seven more independent failures surfaced at
once, spread across unrelated packages — two real data races (an LSP document-manager race and a
package-var-capture race in a trust-store installer), four more shuffle-order test-isolation bugs
(a reset-without-reinitialize in a test I/O helper, a backend-registry wipe-without-restore, a
viper.Set-vs-env-var-tracking conflict, and a style cache left seeded with a partial scheme), and
one dead/unused test-only field that was itself racing for no reason. Round 5: bumping the job's
runner (once every timeout was fixed, this became the slowest check in the PR) picked the wrong
RunsOn family on the first attempt -- fewer cores than before, and its AMI's older Ubuntu broke an
apt-mirror workaround copied from a GitHub-hosted-runner job.

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

Round 4, once the job ran the full suite to completion instead of timing out partway through, one
run surfaced seven more independent failures:

- `pkg/runner/step`'s `TestCastHandlerExecuteWithWorkflowRecordsSimulatedSteps` panicked with
  `data.InitWriter() must be called before using data package functions`. This package's
  `TestMain` initializes `pkg/data`'s global I/O context once for the whole binary, but
  `output_mode_execution_test.go`'s `setupOutputModeCapture` helper (used by 3 tests to capture
  redirected stdout/stderr) called `iolib.Reset()`/`ui.Reset()`/`data.Reset()` in its `cleanup`
  closure to undo its own setup — and never re-initialized afterward, leaving the package-level
  I/O context permanently nil for whatever test ran next under `-shuffle=on`.
- `pkg/lsp/server`'s `TestTextDocumentConcurrentOperations` (a test that deliberately drives
  concurrent `TextDocumentDidChange` calls) hit a real `WARNING: DATA RACE`:
  `DocumentManager.Update` mutated an existing `*Document`'s `Text`/`Version` fields in place
  under its own lock, but `Handler.validateDocument` (called synchronously right after `Update`
  returns, per that code's own comment) reads those same fields through the returned pointer with
  no lock held at all — so a second, overlapping `Update` for the same URI could mutate the exact
  struct an earlier caller was still reading.
- `pkg/terraform/cache`'s `TestInstallTrust_WindowsTimeoutsBlockingTrustStore` hit a real
  `WARNING: DATA RACE`: `runTrustOperation` runs the install function in a background goroutine
  racing a timer, and on timeout returns to the caller while that goroutine keeps running (there's
  no context to cancel a plain `func(string) error` with). `nativeWindowsTrustInstall`'s closure
  read the package-level `installWindowsTrustFunc` var *inside* that still-running goroutine,
  so the test's `t.Cleanup` (restoring the var after the test function returns, well before the
  10-second fake install finishes) raced against it.
- `pkg/terraform/registry`'s `TestProviderMirror_VersionListsAllPlatforms` hit a real
  `WARNING: DATA RACE`: its `fakeRegistry` test helper incremented `dlHits`/`verHits` int fields
  from `httptest.Server` HTTP handlers, which `net/http` dispatches one goroutine per connection —
  concurrent platform-version requests (the test's own point) raced on the plain `int++`.
- `pkg/provisioner/backend`'s `TestAzurermBackendRegisteredInRegistry` failed
  (`GetBackendCreate(azurerm)` etc. all nil) — the same "reset without restore" shape as round 3's
  `github-token` binding, but for the backend registry: `azurerm.go`'s `init()` registers azurerm
  exactly once at process start, and roughly 15 sibling tests in `backend_test.go` call
  `ResetRegistryForTesting()`/`resetBackendRegistry()` (many via `t.Cleanup`) to get an empty
  registry for their own isolated fixtures. Any of those landing before this test under
  `-shuffle=on` leaves the registry permanently empty for the rest of the process.
- `pkg/scanners/sarif`'s `TestHandler_RichTerminalBodyIncludesSourceExcerpt` failed (source
  excerpt missing from the rendered output entirely). `normalize_test.go`'s
  `TestNormalizeArtifactURIsRewritesNestedSARIFLocations` captured
  `viper.GetString(githubWorkspaceViperKey)` and restored it via `viper.Set(...)` in cleanup, on
  the assumption that this "restores" the pre-test state. It doesn't: `githubWorkspace()` resolves
  this key by binding it to `GITHUB_WORKSPACE` and reading it live via `viper.BindEnv`+`GetString`
  on every call; `viper.Set` installs a literal override that outranks the env binding in viper's
  precedence and is never cleared by `t.Setenv`. Once that cleanup ran (capturing whatever
  `GITHUB_WORKSPACE` happened to be — the real GitHub Actions runner value, in CI), every later
  call to `githubWorkspace()` for the rest of the process returned that frozen value regardless of
  `t.Setenv("GITHUB_WORKSPACE", "")`, so the excerpt reader looked for the source file under the
  real CI workspace path instead of the test's own temp dir and silently found nothing (by
  design: `pkg/validation`'s `writeRichDiagnosticSource` is a documented no-op on a read error).

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
- `pkg/runner/step/output_mode_execution_test.go`: `setupOutputModeCapture`'s `cleanup` closure
  now re-initializes `iolib`/`ui`/`data` against the restored `os.Stdout`/`os.Stderr` after
  resetting them, mirroring `TestMain`'s own setup, instead of leaving the package-level I/O
  context nil for the rest of the process.
- `.github/workflows/test.yml`: the `race` job now runs on the RunsOn `runner=terraform` family
  (the same one the `build` job's linux leg already uses for CPU-heavy Go work), not
  `ubuntu-latest`, since `go test`'s package-level concurrency scales with cores and 4 cores was
  the bottleneck. `Harden Runner` (doesn't cover RunsOn) is replaced with the same
  `runs-on/action` setup step the build job's linux leg uses.
- `pkg/lsp/server/documents.go`: `DocumentManager.Update` now builds a new `*Document` (copying
  `URI`/`LanguageID` from the existing entry) instead of mutating the existing struct's fields in
  place, so a caller still holding an earlier `Update`/`Open` call's returned pointer keeps
  reading a frozen, private snapshot no matter what a later `Update` does to the map.
- `pkg/terraform/cache/trust_install.go`: `nativeWindowsTrustInstall`/`nativeWindowsTrustRemove`
  now snapshot `installWindowsTrustFunc`/`removeWindowsTrustFunc` into a local variable before
  `runTrustOperation` spawns its background goroutine, so that goroutine only ever touches its own
  private copy, never the shared package var a test's `t.Cleanup` might reassign mid-flight.
- `pkg/terraform/registry/provider_mirror_test.go`: removed `fakeRegistry`'s `dlHits`/`verHits`
  fields — write-only, never read anywhere in the codebase; deleting the dead counters removes the
  race along with the pointless state.
- `pkg/provisioner/backend/azurerm_test.go`: `TestAzurermBackendRegisteredInRegistry` now re-runs
  azurerm's four `RegisterBackend*` calls (the same ones `init()` makes) before asserting, instead
  of assuming `init()`'s registrations survived every sibling test that resets the registry.
- `pkg/scanners/sarif/normalize_test.go`: `TestNormalizeArtifactURIsRewritesNestedSARIFLocations`
  now uses `t.Setenv("GITHUB_WORKSPACE", workspace)` instead of `viper.Set`/`viper.GetString`
  capture-and-restore, matching how every other test in this codebase controls this env-bound key.
- `pkg/ui/theme/styles_test.go`: `TestInitializeStyles` now calls `t.Cleanup(InvalidateStyleCache)`
  after seeding the package-level style cache with a partial `ColorScheme` (no `Border` set),
  matching the sibling `TestComponentLabelStyleCyclesPalette` (log_styles_test.go), which already
  does this for the same reason — `TestGetBorderColor` was asserting an empty string.

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
- Round 4: `go build ./...` and `go vet` clean; `./custom-gcl run --new-from-rev=origin/main` — 0
  issues; `gofumpt -l` — no output on every changed file.
- `go test -race -shuffle=on ./pkg/lsp/server/... -run TestTextDocumentConcurrentOperations -count=5`
  and the full package (`-count=1`) — all pass.
- `go test -race -shuffle=on ./pkg/terraform/cache/... -run 'TestInstallTrust|TestRemoveTrust' -count=3`
  — all pass (3 full shuffled passes over every install/remove test in the file).
- `go test -race -shuffle=on ./pkg/terraform/registry/... -count=3` — full package passes.
- `go test -race -shuffle=on ./pkg/provisioner/backend/... -count=3` — full package passes.
- `go test -race -shuffle=on ./pkg/scanners/sarif/... -count=3` — full package passes.
- `go test -race -shuffle=on ./pkg/ui/theme/... -count=5` — full package passes.
- `go test -race -shuffle=on ./pkg/runner/step/... -count=3` — full package passes.
- Did not reproduce the runner-swap's actual speedup locally (no access to RunsOn from this
  sandbox); the next real CI run is the validation for that change specifically.

Round 5: the runner swap itself failed immediately, before any tests ran. The "Install Linux
build dependencies" step's `sed -i .../ubuntu.sources` errored with `sed: can't read
/etc/apt/sources.list.d/ubuntu.sources: No such file or directory` (job exit code 2) — the RunsOn
"terraform" runner's AMI is Ubuntu 22.04 (`runs-on-v2.2-ubuntu22-full-x64-...`), which doesn't
have the DEB822 `.sources` file at all (that's a GitHub-hosted-runner-image thing, Ubuntu 24.04+);
this `sed` line was copied from the `floci-go` job, which runs on `ubuntu-latest`. Separately,
the same log's runner-details table showed the "terraform" family is an `i4i.large`: 2 cores,
15.7GB RAM -- *fewer* cores than `ubuntu-latest`, a downgrade for this CPU/memory-bound workload,
not the upgrade intended. Checked the `release` job's `goreleaser` step (also RunsOn) for
comparison: its "large" family is an `r7a.xlarge`, 4 cores, 31GB RAM.

- `.github/workflows/test.yml`: switched `race`'s `runs-on` from `runner=terraform` to
  `runner=large`; guarded the `ubuntu.sources` `sed` behind a `[ -f ... ]` check so the step
  works on either runner image instead of erroring outright when the file doesn't exist.

## Follow-ups

None.
