# Fix: `[race] full test suite` CI job — timeouts, shuffle-order test bugs, and real data races

**Date:** 2026-09-01

## Summary

The new `[race] full test suite` CI job (added to run `atmos test race` on pull requests)
failed on its first seven real runs. Rounds 1–2: the package list included the CLI acceptance
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
apt-mirror workaround copied from a GitHub-hosted-runner job. Round 6, once the runner was fixed
and the job ran to completion for the first time: a single root cause in `pkg/perf`'s hot-path
performance-tracking code (used by nearly every function in the codebase) explained 21 of 24
data races and, once combined with three tests that left tracking permanently enabled, most of
~20 fanned-out `cmd`-package test failures. One further failure in that batch was unrelated: a
pflag `Value.Set` vs `Flags().Set` distinction that silently didn't mark a flag as changed. Round
7: a genuine upstream data race in `charmbracelet/bubbles`'s progress-bar animation, a widespread
`--help`-flag-leak pattern (cobra checks a flag's *current* value on every `Execute()`, not just
whether it was in that call's own args) found in three packages, plus two more
reset-without-restore leaks. One more failure could not be pinned down -- it didn't reproduce
twice with the same `-shuffle` seed, pointing to genuine goroutine-timing nondeterminism rather
than simple test-order dependence -- and is left as an open item.

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
- `internal/exec/vendor_model.go`: `handleInstalledPkgMsg` now only calls
  `m.progress.SetPercent(...)` (and returns its `tea.Cmd`) when `m.isTTY` — working around the
  upstream `bubbles` `progress.Model` race documented above, and skipping animation work that
  never renders to anyone when there's no TTY regardless.
- `cmd/init/init_test.go`: `TestInitCmd_Integration_Help` now resets `initCmd`'s `help` flag via
  `t.Cleanup` after setting it, instead of leaving it `true` for every later test that calls
  `initCmd.Execute()`.
- `cmd/scaffold/scaffold_test.go`: added a shared `resetHelpFlag(t, cmd)` helper and applied it to
  all four `TestScaffold*Cmd_Integration_Help` tests, same fix as `cmd/init`'s.
- `cmd/version/list_test.go`: `TestListCommand_FormatValidation` now resets the package-level
  `listFormat` var to `"table"` (its real flag default) via `t.Cleanup`, instead of leaving it at
  `"invalid"` (its last test case's value) for whatever test runs next.
- `cmd/validate_editorconfig_test.go`: `TestEditorConfigCmdCIFlagRegisteredThroughStandardParser`
  now re-runs `ciFlagsParser.BindFlagsToViper(editorConfigCmd, viper.GetViper())` (the same call
  `init()` makes) before asserting, rather than assuming that one-time binding survived every
  sibling test's `viper.Reset()`.

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

Round 7, once the round-6 fixes landed and the job ran to completion again with a real 4-core
runner: a fresh run surfaced a smaller but still varied set of failures:

- `internal/exec`'s `TestExecuteComponentVendorPullBatch_PullsAllComponentsInOneCall` hit a real
  `WARNING: DATA RACE` inside `github.com/charmbracelet/bubbles@v1.0.0`'s `progress.Model`:
  `SetPercent` mutates `m.tag`/`m.targetPercent` directly and returns a `tea.Cmd`
  (`nextFrame`) whose closure reads `m.tag`/`m.id` back off the same `*Model` pointer when the
  tick fires -- on bubbletea's own command-execution goroutine, not the model's owning goroutine.
  Calling `SetPercent` again (a second package finishing) before that tick fires -- which
  completing package installs faster than one animation frame, as this test's mocked installs
  do, reliably triggers -- races. This is an upstream library bug, not an Atmos usage defect;
  vendoring/patching `bubbles` was out of scope, so the fix avoids triggering it instead: nothing
  renders `m.progress.View()`'s animation without a TTY, so `SetPercent` (and the `tea.Cmd` it
  returns) is now only called when `m.isTTY`. This closes the CI failure but not every
  theoretical production case (a real TTY session installing several same-machine components
  faster than one frame could still hit it) -- an upstream fix or report would close it fully.
- Three packages had the *same* latent bug shape, previously undetected because nothing had ever
  run their `--help` integration test before another test that also calls the same command's
  `Execute()`: cobra's `execute()` checks the `help` pflag's *current* value on every call
  (`c.Flags().GetBool("help")`), not whether `-h`/`--help` was in that specific invocation's own
  args. `initCmd.SetArgs([]string{"--help"})` (`cmd/init`) and the four
  `scaffold*Cmd.SetArgs([]string{"--help"})` calls (`cmd/scaffold`) never reset the flag
  afterward, so once any of them ran, every later test in the same package's binary that called
  that command's `Execute()` got `nil` back having silently printed help -- `RunE` never ran, no
  matter what args that later test passed. This is exactly why `TestExecuteInit_ArgumentParsing`
  (fixed already once for a different reason, in round 6) kept reappearing: reproducing it needed
  the *same* fixed `-shuffle` seed to confirm, since `--help`-leak and ordering both had to align.
  `cmd/root_test.go`'s two `RootCmd.SetArgs([]string{"--help"})` sites don't have this problem --
  they already call `NewTestKit(t)`, which snapshots and restores all of `RootCmd.Flags()`,
  including `help`.
- `cmd/version/list_test.go`'s `TestListCommand_ValidationErrors` failed with the wrong error
  (`"invalid format: invalid ..."` instead of the expected date-format error) because
  `TestListCommand_FormatValidation`'s last case sets the package-level `listFormat` var (which
  `listCmd`'s `RunE` reads directly, not through a bound flag) to `"invalid"` and never restores
  it, so a later test's own unrelated validation check hit that stale value first.
- `cmd/validate_editorconfig_test.go`'s `TestEditorConfigCmdCIFlagRegisteredThroughStandardParser`
  failed for the now-familiar reason: `editorConfigCmd`'s `ciFlagsParser.BindFlagsToViper(...)`
  runs once in `init()`; some other test's `viper.Reset()` discards it, and nothing rebinds.
- One more failure, `cmd/list`'s `TestListStacksWithOptions_CoverageIntegration` ("authentication
  requires at least one identity configured"), reproduced twice via full-package `-shuffle=on`
  scans but did **not** reproduce on either retry using the exact seed that had just produced it
  -- ruling out a simple ordering/leftover-value explanation (which would be seed-deterministic)
  in favor of genuine goroutine-timing nondeterminism between an earlier test and this one. Ruled
  out during investigation: `cmd/list`'s only two `t.Parallel()` tests
  (`cmd/list/closure_test.go`) touch no auth/config/viper state at all, and `t.Chdir` (used by
  `chdirToCompleteFixture`) has Go's own built-in serialization against concurrent use. Left
  unresolved -- see Follow-ups.

Round 6: with the runner fixed, the job ran to completion (~40 minutes) and failed with ~20
distinct `cmd` package test failures and 24 `WARNING: DATA RACE` blocks. 21 of the 24 race blocks
traced back to a single root cause: `pkg/perf.finishSimpleStackTracking`, the "simple stack"
performance-tracking fast path used by the `defer perf.Track(...)` call at the top of nearly
every public function repo-wide. `trackWithSimpleStack`'s own comment already documents a "known
limitation" -- it only verifies goroutine ownership of the shared global `simpleStack` at call
depth 0 or 1, "trusting" ownership at deeper nesting for speed, so a second goroutine's calls can
silently start sharing that stack undetected. The resulting cross-goroutine frame mixing wasn't
just producing wrong metrics (the accepted tradeoff) -- `StackFrame.childTime`, read and written
via plain `time.Duration` field access with no synchronization at all, was a genuine, unguarded
data race once two goroutines' frames were actually interleaved on the same stack.

That still leaves the question of why so many otherwise-unrelated `cmd` tests hit this: perf
tracking is off (`Track` a no-op) unless something calls `perf.EnableTracking(true)`, and normal
test runs never do. `cmd/root_heatmap_test.go`'s `TestDisplayPerformanceHeatmap` (both table-driven
cases) and `TestHeatmapNonTTYOutput` do call it directly to exercise the heatmap display, with
misleading comments claiming to "Reset perf registry" (no such function existed) -- and, unlike
the well-behaved sibling `TestEnableHeatmapIfRequested` (`cmd/cmd_utils_test.go`), never called
`perf.EnableTracking(false)` afterward. Once any of the three ran under `-shuffle=on`, tracking
stayed permanently on for the rest of the `cmd` package's test binary, so every real
`perf.Track()` call in every subsequent test -- hundreds of them, many touching goroutines via
`internal/exec`'s concurrent YAML/stack processing -- became live and exposed to the race above.
`TestEnableHeatmapIfRequested` failed itself for a related but distinct reason: with no registry
reset ever available, its own few tracked calls got crowded out of the heatmap's top-N display by
the (now real) flood of accumulated metrics from whatever ran before it.

One further failure, `TestUninstallCmd_RunE_MultipleSkills`, was unrelated to the perf issue
entirely: it set the `force` flag via `uninstallCmd.Flags().Lookup("force").Value.Set("true")`,
which updates the flag's value but -- unlike `Flags().Set("force", "true")`, which every other
force-flag test in the same file correctly uses -- does not mark the pflag as `Changed`. Since
`uninstall.go` reads the value through viper (`v.GetBool("force")`, per the flag-handling
mandate), not the raw flag, and viper's precedence favors an explicitly-changed flag, an unmarked
"true" could resolve to whatever unrelated value was left over from a prior test instead, which
under `-shuffle=on` could genuinely be "prompt for confirmation" -- and the test always ran
headless, so that prompt itself errors immediately as impossible.

- `pkg/perf/perf.go`: `StackFrame.childTime` changed from `time.Duration` to `atomic.Int64`
  (nanoseconds), with `.Load()`/`.Add()` at both read/write sites (shared by both the simple-stack
  and goroutine-local-stack code paths, which use the same struct). New `ResetForTesting()` clears
  the metrics registry, matching what the misleading pre-existing comments already claimed to do.
- `cmd/root_heatmap_test.go`: all three call sites now pair `perf.EnableTracking(true)` with
  `t.Cleanup(func() { perf.EnableTracking(false) })` and call `perf.ResetForTesting()` first.
- `cmd/cmd_utils_test.go`: `TestEnableHeatmapIfRequested` now also calls
  `perf.ResetForTesting()` before its own assertions, for the same reason.
- `cmd/ai/skill/uninstall_test.go`: `TestUninstallCmd_RunE_MultipleSkills` now sets the force flag
  via `Flags().Set("force", "true")` (matching every sibling test in the file) instead of
  `Lookup("force").Value.Set("true")`, and resets it to `"false"` via `t.Cleanup`.

Round 6 validation:

- `go build ./...`, `go vet ./cmd/... ./pkg/perf/...` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (one `godot` finding on the new
  `StackFrame` doc comment, fixed).
- `gofumpt -l` on every changed file — no output.
- `go test -race -shuffle=on ./pkg/perf/... -count=3` — full package passes.
- `go test -race -shuffle=on ./cmd/... -run 'TestEnableHeatmapIfRequested|TestDisplayPerformanceHeatmap|TestHeatmapNonTTYOutput' -count=3`
  — passes (exit 0 across the whole `./cmd/...` tree, no FAIL anywhere).
- `go test -race -shuffle=on ./cmd/ai/skill/... -count=3` — full package passes.
- Re-ran the exact set of 18 originally-failing top-level test names (everything from the CI log
  except `TestPackerValidateCmd`, a separate, pre-existing environment issue -- `packer init` was
  never run, so its plugins aren't installed; unrelated to this incident) across the whole
  `./cmd/...` tree with `-race -shuffle=on`: exit 0, no FAIL lines anywhere in ~19000 lines of
  output. This is the strongest signal yet that the fan-out is resolved, though (as with every
  other round) the actual CI run against the real RunsOn `large` runner is the final check.

Round 7 validation:

- `go build ./...`, `go vet ./cmd/...` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (one `godot` finding, fixed).
- `gofumpt -l` on every changed file — no output.
- `go test -race -shuffle=1788300210151906000 ./cmd/init/... -v` (the exact seed that reproduced
  the failure) — passes; 25 further `-shuffle=on` scans of the full package — all clean.
- `go test -race -shuffle=on ./cmd/scaffold/...` — clean.
- `go test -race -shuffle=on ./internal/exec/... -run TestExecuteComponentVendorPullBatch -count=5`
  — all pass; sanity-checked the fix actually addresses the race by confirming the mechanism
  (`SetPercent`'s returned `tea.Cmd` is genuinely what races, per the upstream source read).
- `go test -race -shuffle=on ./cmd/version/... -count=3` and
  `go test -race -shuffle=on ./cmd/... -run TestEditorConfigCmdCIFlagRegisteredThroughStandardParser -count=3`
  — both clean.
- Full `go test -race -shuffle=on ./cmd/...` (one complete pass, ~45 minutes locally) — one
  failure: `cmd/list`'s `TestListStacksWithOptions_CoverageIntegration`, investigated and left
  open (see Follow-ups) after it didn't reproduce on retries with the seed that had just produced
  it.

## Follow-ups

None.

## Round 9 (resolved `cmd/list` flake)

The `cmd/list` failure documented in the prior round's Follow-ups (`TestListStacksWithOptions_CoverageIntegration`
and, in a later CI run, three siblings — `TestExecuteListInstancesCmd_TreeFormat`,
`TestExecuteListInstancesCmd_MatrixFormat`, `TestListStacksWithOptions_TreeFormatWithProvenance` — all
failing with `authentication requires at least one identity configured in atmos.yaml`) is a genuine
test-order leak, not goroutine-timing nondeterminism as the previous round concluded (that conclusion was
wrong: retrying with the same `-shuffle` seed via `-run <name>` narrows the executed test set, which changes
the deterministic order and hides the leak — it doesn't prove the failure is non-order-related).

Root cause: `cmd/list/affected_test.go`'s `TestAffectedIdentityFlagParsing`, `cmd/list/instances_test.go`'s
`TestInstancesIdentityFlagLogic`, and `cmd/list/utils_test.go`'s `TestGetIdentityFromCommand` and
`TestGetIdentityFromCommand_NormalizesIdentityEnvFalse` all call `viper.Reset()` then
`viper.Set("identity", "viper-identity"|"env-identity"|"no")` against the global viper singleton with no
`t.Cleanup` to restore it. Under `-shuffle=on`, if any of these run before an executor-integration test that
builds a fresh `cmd` (no `--identity` flag set), `getIdentityFromCommand`'s viper fallback
(`cmd/list/utils.go`) picks up the leaked identity name. Since a non-empty `identityName` short-circuits
`resolveIdentityName` (`pkg/auth/manager_helpers.go`) without checking whether auth is even configured, the
leaked value reaches `CreateAndAuthenticateManagerWithAtmosConfigForStack`'s `isAuthConfigured` check — which
then correctly fails, because the `complete` fixture (used by `chdirToCompleteFixture`) has no `auth:` section
at all. `cmd/list/settings_test.go`'s `TestSettingsCmd_RunE_CoverageIntegration` already carried a comment
describing this exact mechanism and worked around it locally (`cmd.Flags().Set("identity", "false")`); the
other executor-integration tests never got the same treatment, which is why only they flaked.

Fix: added `t.Cleanup(viper.Reset)` to each of the four leaking tests, matching the pattern already used
elsewhere in this package and throughout this incident.

Validation: `go build ./cmd/list/...`, `go vet ./cmd/list/...` — clean. `go test -race -shuffle=on
./cmd/list/... -timeout 300s`, run twice (each with its own random shuffle order) — both pass, no
`authentication requires at least one identity` failures. A separate `-count=3` invocation at the same
300s timeout is **not** part of this validation record: it was interrupted by the timeout before all three
passes finished (`cmd/list` under `-race` takes ~90-110s per single pass locally, so three consecutive
passes need a longer timeout than 300s) and produced no result, passing or failing — the goroutine dump it
printed was ordinary `t.Parallel()` tests waiting their turn, not a deadlock, but the run itself proves
nothing either way and would need to be rerun with a longer timeout to count as evidence.
