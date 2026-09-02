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

## Round 9 addendum

Two more shuffle-order bugs were fixed alongside Round 9's `cmd/list` root cause, in the same commit, since
they surfaced in the same CI log and follow the identical pattern:

- **`cmd/describe_workflows_test.go`'s `TestDescribeWorkflows`** panicked with `workflows flag redefined:
  pager`. The test unconditionally calls `describeWorkflowsCmd.Flags().StringP("pager", ...)`; `--pager` is
  also a `RootCmd` persistent flag, and cobra's `mergePersistentFlags()` (itself `Lookup`-guarded, unlike a
  raw `StringP`/`AddFlag` call) merges it into `describeWorkflowsCmd`'s local `FlagSet` the first time some
  *other* test drives the command through the full `Execute()` pipeline. Under `-shuffle=on`, if that other
  test runs first, the flag already exists and the direct `StringP` call panics. Fixed by guarding it with a
  `Lookup` check first, matching `AddFlagSet`'s own safety.
- **`pkg/auth/manager_test.go`'s `TestManager_Whoami_FallbackAuthenticationFails`** expected an authentication
  failure but got a *success* result with credentials from a completely different test's provider.
  `pkg/auth/manager_chain.go`'s `processCredentialCache` (a package-level `sync.Map`, intentionally
  process-scoped so it doesn't hold data across separate CLI invocations) was never reset between tests that
  reuse the same provider/identity names (`"p"`/`"dev"`, used throughout this file) — a passing test's cached
  credentials leaked into a later test asserting failure. Fixed by adding `resetProcessCredentialCache()` +
  `t.Cleanup(resetProcessCredentialCache)` to the 11 `TestManager_Whoami*`/`TestManager_Authenticate*` tests
  that build a `manager` and call `Authenticate`/`Whoami`/`AuthenticateProvider`, matching the pattern already
  used in this package's other test files (`manager_chain_process_cache_test.go`,
  `manager_ambient_provider_test.go`, `manager_chain_ambient_test.go`).
- **`cmd/terraform/cache/mirror_test.go`'s `TestMirrorCmdRunSingle`** expected `Options.All == false` but got
  `true`. `TestMirrorCmdRunAll` (a sibling test) passes `--all`, which cobra parses onto the package-level
  `mirrorCmd`'s own `--all` flag with `Changed = true`; `Options.All` is read via `v.GetBool("all")` (viper's
  flag binding, which honors `Changed`, not just the flag's default), so a later test that never passes
  `--all` still observed `All = true` if it ran after `TestMirrorCmdRunAll` under `-shuffle=on`. Fixed by
  capturing the flag's original value and `Changed` state before `TestMirrorCmdRunAll` mutates it, and
  restoring both (not just the value) in cleanup — the same pattern CodeRabbit flagged for
  `cmd/ai/skill/uninstall_test.go`'s `force` flag in this same PR's review.

## Round 10 (seven more independent fixes, including one production crash bug)

The push containing Round 9's fixes produced a new CI run (`33574641692`) with nine `--- FAIL` entries. The
Round 9 addendum fixes above were confirmed resolved (none of those three tests appear in this run's failure
list). Two of the nine need a real `tofu`/`packer` binary the race job's runner doesn't install —
`TestPackerValidateCmd` was already a documented pre-existing gap; `TestRunTerraformMigratePlan_NoMigrationsDirSkipsCleanly`
passed in isolation and across several full-package `-shuffle=on` local reruns (even with `tofu`/`terraform`
removed from `PATH`, which would make an erroneous invocation loud), so its CI-only failure could not be
reproduced or root-caused locally — left open below rather than force a guess-fix. `TestContextWriteRecordsMaskedOutput`
is also left open below: it did not reproduce locally, and its adjacent CI log line looks like unrelated
output interleaved from a concurrently-running package's own test binary rather than genuine contamination
of its own captured stdout. The other six were root-caused and fixed. (A separate CI run, `33576003604`, for
the commit that already contained Round 9's fixes but not yet this round's, independently re-confirmed two of
these: `TestInstallCmd_RunE_AlreadyInstalledOmitsLocationWhenNothingInstalled` reappeared, and its sibling
`TestUserIdentity_LoadCredentials` hit the exact same `us-east-2` symptom as `TestPermissionSetIdentity_LoadCredentials`
below, confirming the `setupAWSEnv` fix covers more than the one test that first exposed it.)

- **`pkg/auth/identities/aws/permission_set_test.go`'s `TestPermissionSetIdentity_LoadCredentials`** expected
  region `us-east-1` (from its own written SSO config file) but got `us-east-2`. Root cause in production
  code: `pkg/auth/identities/aws/credentials_loader.go`'s `setupAWSEnv` only added `AWS_REGION` to the
  save/restore map when the identity resolved a non-empty region, so when it didn't (this test's identity has
  no configured region), any ambient `AWS_REGION` left over from an *earlier* identity's credential load in
  the same process was never cleared, and the AWS SDK gives an explicit env var precedence over the shared
  config file's per-profile region. Fixed by always tracking `AWS_REGION` in `setupAWSEnv`'s save/restore map
  and explicitly `os.Unsetenv`-ing it when the resolved region is empty, instead of leaving it untouched —
  this is a real production correctness fix, not just a test-isolation one: region resolution must not depend
  on whichever other identity's credentials were loaded earlier in the process.
- **`pkg/auth/identities/azure/subscription_test.go`'s `TestSubscriptionIdentity_PostAuthenticate`** expected
  `credentials.json` under a sandboxed `HOME` but got "no such file or directory". `pkg/config/homedir` caches
  the resolved home directory across calls; the test sandboxes `HOME` via `t.Setenv` but never called
  `homedir.Reset()` + `homedir.DisableCache = true`, so a prior test's cached (real) home directory could
  outlive the `t.Setenv` and `SetupFiles` would write somewhere other than the test's temp dir. Fixed with the
  same `homedir.Reset()`/`DisableCache`/cleanup pattern already used in `cmd/ai/skill/uninstall_test.go`.
- **`pkg/generator/generator_test.go`'s `TestGenerate`** ("runs single generator by name" subtest) failed with
  `generator not found: single` immediately after the subtest assigned that exact generator into the
  package-level `registry` var. Root cause: `GetRegistry()` lazily initializes `registry` via `sync.Once`
  (`registryOnce`) on its first-ever call in the whole test binary process. Several tests in this file
  (`TestGeneratorRegistry`, `TestGenerateAll`, `TestGenerate`, ...) assign `registry` directly and then call a
  function (`Register`, `Generate`, `GenerateAll`) that reaches `GetRegistry()` internally; if that call is the
  first `GetRegistry()` call in the process, the `Once` fires there and silently overwrites the test's
  manually-assigned registry with a fresh empty one, discarding whatever it just registered. Fixed with a
  package-level `init()` in the test file that calls `GetRegistry()` once, before any test runs, so the
  `Once` is always already settled.
- **`cmd/ci/validate_test.go`'s `TestWorkflowValidationErrorOwnsDiagnostics`** expected the rendered error to
  contain the literal string `"GitHub Actions workflow validation failed"` but it didn't. Root cause:
  `errUtils.DefaultFormatterConfig()`'s `MaxLineLength` is `0`, which auto-detects wrapping width from the
  terminal; the race job's CI runner apparently detects a narrower width than local dev, which wrapped
  "validation" and "failed" onto separate lines, breaking the single-line substring match. Fixed by pinning
  `MaxLineLength: 200` in this test (wide enough that this short message never wraps), matching the existing
  precedent of pinning a fixed width in `errors/examples_test.go` and `errors/formatter_test.go` rather than
  relying on auto-detection in a test assertion.
- **`cmd/root_help_routing_test.go`'s `TestRootHelpFunc_RealTree_UnknownSubcommandErrors`** ("toolchain
  versions --help" case) expected an "Unknown command" error but got silent success (no panic, exit 0, empty
  output). Root cause: `TestRootHelpFunc_RealTree_ValidCasesStillRenderHelp` (a sibling test in the same file)
  genuinely invokes `atmos toolchain --help` through the real `RootCmd` tree, which cobra parses onto
  `toolchain`'s own `--help` flag; `NewTestKit` does not reach nested subcommands' flags (only `RootCmd`'s
  own), so that flag stayed `true` afterward. Cobra's `execute()` checks `helpVal, _ :=
  c.Flags().GetBool("help")` on *every* call regardless of that call's own args — the same
  leaked-`--help`-flag mechanism already fixed for `cmd/init` and `cmd/scaffold` earlier in this incident, this
  time on `cmd/toolchain`. Fixed by resetting the invoked command's (and its child's, where applicable)
  `--help` flag in both this test and its sibling.
- **`cmd/ai/skill/install_test.go`'s `TestInstallCmd_RunE_AlreadyInstalledOmitsLocationWhenNothingInstalled`**
  expected "0 skills installed" on a second run against an already-populated fake `HOME`, but got "52 skills
  updated successfully" — and its sibling `TestInstallCmd_RunE_NoArgsInstallsEveryBundledSkill` intermittently
  failed the opposite way, missing "skills installed successfully in" from its output. Both tests'
  `resetFlags` closures only reset `yes` (and, inconsistently, sometimes `force`) via a direct
  `flag.Value.Set("false")` call, which does *not* clear `Changed` (only `Flags().Set` does) and left every
  other flag `installCmd` registers (`path`, `client`, `all-clients`, `scope`, `global`) completely untouched.
  `installCmd` is a package-level singleton; a later test in this same file leaking any of those flags'
  `Changed` state changed which skills the next test's run considered already-installed or where it
  distributed them. This file already had the correct pattern established elsewhere
  (`resetFlagChangedForTest`, used by `TestInstallCmd_RunE_PathWithClientWarns` and its sibling) but these two
  older tests predated it. Fixed by adding a `resetInstallCmdFlagsForTest` helper that resets every flag
  `installCmd` registers via the existing `resetFlagChangedForTest`/`resetStringSliceFlagForTest` helpers, and
  using it in both tests.

A seventh fix, found while running the full local suite once with the exact CI command
(`go test -race -shuffle=on $(go list ./... | grep -v '^github.com/cloudposse/atmos/tests') -timeout 20m`),
is a genuine **production crash bug**, not just test isolation:

- **`internal/tui/utils/utils.go`'s `PrintStyledText`/`PrintStyledTextToSpecifiedOutput`** (used by the
  `atmos version` banner and help templates) call `figurine.Write`, which renders via
  `github.com/common-nighthawk/go-figure` in *strict* mode (hardcoded `true` inside figurine, not
  configurable from Atmos's side). Strict mode's `Slicify` calls `log.Fatal("invalid input.")` — a hard,
  unrecoverable `os.Exit`, not a returned error — on the first character outside printable ASCII (`' '`
  through `'~'`), which includes a plain `'\n'`. Any styled text containing a newline or control character,
  rendered while color is enabled (`--force-color`, `FORCE_COLOR`, `CLICOLOR_FORCE`, or auto-detected color
  support), crashes the whole `atmos` process instead of erroring gracefully. `internal/tui/utils/utils_test.go`'s
  own `TestPrintStyledText`/`TestPrintStyledTextToSpecifiedOutput` tables already covered "multiline text" and
  "text with special characters" cases expecting `wantErr: false`, so this was a real, if narrow, latent
  crash — masked locally because it only reproduces when the color-support path is actually taken (this
  environment's terminal-color auto-detection is not fully deterministic across otherwise-identical runs, so
  the crash surfaced intermittently rather than every time even before this fix). Fixed with a
  `sanitizeForFigurine` helper that replaces out-of-range characters with `'?'` before calling
  `figurine.Write`, mirroring go-figure's own non-strict fallback behavior (`figure.go`'s `Slicify`: `else {
  char = '?' }`) since figurine's strict flag itself can't be turned off from here. Verified directly: forcing
  `viper.Set("force-color", true)` and calling `PrintStyledTextToSpecifiedOutput` with `"Line1\nLine2\nLine3"`
  now renders successfully instead of crashing.

Validation for all seven: `go build ./...`, `go vet ./...` — clean. `./custom-gcl run
--new-from-rev=origin/main` — 0 issues. Each fixed package's own tests pass across 3–15 `-race -shuffle=on`
reruns locally (`pkg/auth/identities/aws`, `pkg/auth/identities/azure`, `pkg/generator`, `cmd/ci`, `cmd`,
`cmd/ai/skill`, `internal/tui/utils`). A full local run of the exact CI command across the entire package
set (minus `tests/`, `-timeout 20m`) completed with 390 of 391 testable packages passing; the one failure was
`internal/tui/utils` before this round's fix, now also passing across 15 reruns.

## Round 12 (a genuine data race, caught exactly as intended)

The push containing Round 11's fixes (plus a merge from `main` and Dependabot remediation) produced a CI
run with a real `WARNING: DATA RACE` block, alongside `TestRunTerraformMigratePlan_NoMigrationsDirSkipsCleanly`
(already documented above, unchanged) and `TestInitializeAIComponents_WithToolLists`, whose only failure
message was `race detected during execution of test` — i.e. the race *was* the failure, not a separate bug.

**`pkg/ai/tools/atmos/list_commands.go`'s `commandTreeProvider`** is a package-level variable injected once
at MCP server startup (`SetCommandTreeProvider`, called from `cmd/mcp/server`'s `initializeAIComponents`) and
read on every `atmos_list_commands`/`atmos_command_help` tool call (`commandTreeRoots`), with no
synchronization at all. `cmd/mcp/server`'s own tests call `SetCommandTreeProvider` from more than one
goroutine — one directly (`TestInitializeAIComponents_WithToolLists`), another indirectly through
`setupMCPServer`'s own config-loading path — and the write in each goroutine raced the other. This is a real,
narrow-but-genuine production bug: two concurrent MCP server initializations (or an initialization racing a
tool call) could corrupt or silently drop the provider assignment. Fixed with a `sync.RWMutex`
(`commandTreeProviderMu`) guarding both the write (`SetCommandTreeProvider`) and the read
(`commandTreeRoots`); the two same-package test files that read the raw variable directly for save/restore
(`command_help_test.go`, `list_commands_test.go`) were updated to take the read lock too.

Validation: `go build ./...`, `go vet ./...` — clean. `./custom-gcl run --new-from-rev=origin/main` — 0
issues. `go test -race -shuffle=on ./cmd/mcp/server/...` — passes cleanly (previously reproduced the race).
`go test -race -shuffle=on ./pkg/ai/tools/atmos/... -run 'TestListCommandsTool|TestCommandHelpTool'` — passes
cleanly across the tests that touch `commandTreeProvider`/the new mutex directly. (A broader, unfiltered
`./pkg/ai/tools/atmos/...` run separately hit an unrelated real-network test hang in this sandbox — the same
established "local sandbox network unreliability" pattern documented earlier in this incident, not a
regression from this fix.)

## Follow-ups

- `pkg/io/recorder_test.go`'s `TestContextWriteRecordsMaskedOutput` failed once in CI with
  `recorder received unmasked output`, and the failing CI log's very next line (unindented, not part of the
  test framework's own `--- FAIL` output block) is a `WARN Skipping invalid mask pattern from atmos.yaml`
  line whose exact pattern (`[invalid(`) matches a completely unrelated table-driven case in
  `pkg/io/masker_test.go`, which builds its own fully-isolated masker and config and cannot reach this
  test's global state. Did not reproduce across 5 local `-race -shuffle=on` reruns of the whole `pkg/io`
  package. Most likely explanation: the CI log aggregates multiple concurrently-running `go test` package
  processes' stdout, and that adjacent line is simply interleaved output from a different package's test
  binary, not genuine contamination of this test's own `os.Stdout`-redirecting pipe — but this wasn't
  confirmed, so treat the failure itself (not the theory) as still open. If it recurs, capture the CI log
  with `##[group]`/timestamps intact (this repo's log fetch already includes per-line timestamps) and check
  whether the `pkg/io` package's own timestamp range genuinely contains that warning line, or whether it
  falls in a different package's timestamp window.
- `cmd/terraform/migrate`'s `TestRunTerraformMigratePlan_NoMigrationsDirSkipsCleanly` has now failed in at
  least two separate CI runs (Rounds 12 and 16) with `exec: "tofu": executable file not found in $PATH`, even
  though the test's own comment states it must succeed "without ever needing a real tfmigrate/opentofu
  binary." Both times, retrying locally with the exact CI-failing `-test.shuffle` seed for
  `cmd/terraform/migrate` (unfiltered, per Round 9's lesson that `-run`-narrowing changes the deterministic
  order) passed cleanly — still not reproducible locally after two full rounds of dedicated effort. Round 16
  considered making the "tofu never invoked" guarantee deterministic by deliberately breaking `PATH` (mirroring
  the sibling `TestRunTerraformMigrate_SelectWorkspaceErrorPropagates`'s own idiom) but backed out: the test's
  only assertion is `require.NoError`, which doesn't verify tofu was never invoked at all — if a real,
  narrow invocation-skip bug exists, forcing `PATH` to always break would convert today's intermittent CI
  flake into a deterministic CI failure instead of fixing anything, which is the wrong trade under this level
  of uncertainty. Left open; if it recurs again, the next investigation should focus on why CI's ambient PATH
  differs run-to-run in a way this local sandbox doesn't reproduce (e.g. runner image variance in where `tofu`
  is installed) rather than re-attempting local `-shuffle` reproduction a third time.
- `cmd/root_test.go`'s `TestApplyCIGitCloneBootstrap_CICloneExplicitFalseOptsOut` and
  `TestApplyCIGitCloneBootstrap_NoCIProviderDetected` failed in a full local `-race -shuffle=on` run of the
  entire package set with `atmosConfig.CI.Enabled` unexpectedly `true` (`assert.False` on that field, not on
  `applied` or `tmpConfig.CI.Enabled`, which both passed). `applyCIGitCloneBootstrap` (`cmd/root.go`) only
  ever sets the package-level `atmosConfig.CI.Enabled = true` on its "bootstrap applied" branch — the branch
  these two tests exercise returns early without touching it — so `atmosConfig.CI.Enabled` was already `true`
  *before* either test ran. All three tests that call `applyCIGitCloneBootstrap` directly correctly wrap
  themselves in `saveRestoreAtmosConfig(t)` (save-before/restore-after, not reset-to-clean), so the leak's
  source is some *other* test elsewhere in the large `cmd` package that sets `atmosConfig.CI.Enabled = true`
  (directly, or indirectly via a real `Execute()`/`InitCliConfig()` call that detects a real CI environment
  variable) without using that same helper — not identified within this round's time budget. Neither test
  appeared in any real CI run's failure list in this incident (rounds 9–11), only in this round's local
  full-suite reproductions — left open rather than force a guess-fix across an unbounded search space.
- `cmd/ci/validate_test.go`'s `TestWorkflowValidationErrorOwnsDiagnostics` (Round 10's `MaxLineLength: 200`
  fix) reappeared in the CI log that also contained Round 13's gomonkey hang. Not yet re-investigated to
  confirm whether the fix regressed, was affected by an unrelated change, or a new failure mode emerged —
  flagged here rather than guessed at, pending the next CI run once Round 13's fix lands.
- `TestValidateStacksCmd_Failure` surfaced a genuine `WARNING: DATA RACE` in a local full-package
  `-race -shuffle=on` rerun during Round 15's validation, in `pkg/ui/markdown.(*CustomRenderer).Render`
  reached via `pkg/ui/spinner.(*Spinner).Error` — concurrent writes to shared renderer state from the
  spinner's animation goroutine racing the actual error-render call. Unrelated to Round 15's diff (which only
  touches `cmd` package command-tree/flag/helpFunc test restoration); not reproduced in isolation or
  root-caused within this round's budget — left open.

## Round 11 (the `--help`-flag leak fix, generalized)

`TestRootHelpFunc_RealTree_UnknownSubcommandErrors` reappeared in a full local `-race -shuffle=on` run of the
*entire* `cmd` package (461s, run directly rather than filtered to just `root_help_routing_test.go`) — this
time both the "toolchain versions --help" *and* "terraform bogus-subcommand --help" cases failed, even though
Round 10 had already reset both implicated tests' own `--help` flags. The `cmd` package has 461 seconds worth
of tests; evidently some *other* test elsewhere in the package also drives a real `atmos toolchain --help` or
`atmos terraform --help` invocation through `RootCmd.ExecuteC()`, leaking that flag the same way, and
per-test whack-a-mole fixes don't scale to "some other test, somewhere in a very large package."

Fixed at the root instead: `cmd/testing_helpers_test.go`'s `snapshotRootCmdState`/`restoreRootCmdState`
(the mechanism behind every test's `NewTestKit(t)` call) previously only snapshotted and restored `RootCmd`'s
*own* `Flags()`/`PersistentFlags()` — never any subcommand's. A real `--help` invocation against any
subcommand (`toolchain`, `terraform`, `version`, or anything else) parses onto *that command's own* FlagSet,
which is just as much a package-level singleton as `RootCmd`'s, and was never covered. Added
`walkCommandTree`, which recursively visits `RootCmd` and every command reachable from it, and used it in
both the snapshot and restore paths so every command's flags (not just `RootCmd`'s) are captured and put
back after each `NewTestKit`-protected test — closing this leak for the whole command tree at once instead
of one flag-and-command pair at a time. `cmdStateSnapshot.flags` changed from `map[string]flagSnapshot`
(flag name only, ambiguous across commands) to `map[*cobra.Command]map[string]flagSnapshot`; the three
direct-field-access tests in `cmd/testing_helpers_snapshot_test.go` were updated to index by `RootCmd`
explicitly (`snapshot.flags[RootCmd]["chdir"]`, etc.) since they only ever exercised `RootCmd`'s own flags.

Validation: `go build ./...`, `go vet ./...` — clean. `./custom-gcl run --new-from-rev=origin/main` — 0
issues. `go test -race -shuffle=on ./cmd/... -run 'TestSnapshotRootCmdState|TestTestKit_|TestRootHelpFunc_RealTree'`
— all pass, including both previously-flaking subtests. A full `go test -race -shuffle=on ./cmd/ -timeout
900s` (the entire package, no `-run` filter, matching the exact shape that exposed this) completed in 461s
with zero failures. A subsequent full local run of the entire package set (minus `tests/`) completed with
390 of 391 packages passing; the one remaining failure (`cmd`, two different tests than the ones this round
fixed) is the separate, still-open `atmosConfig.CI.Enabled` leak documented in Follow-ups — the `--help`-flag
leak this round targeted did not recur.

## Round 13 (gomonkey's runtime patching hangs forever under `-race`, not just crashes on ARM64)

The push containing Rounds 11–12 produced a CI run where `internal/exec` never finished: the job hit its
20-minute per-package `go test` timeout inside `TestGetAffectedComponents`, which had already been (correctly)
skipped on `darwin/arm64` via an inline `runtime.GOARCH == "arm64"` check, but CI runs on `linux/amd64` — a
platform that check never covered.

`gomonkey.ApplyFunc`/`gomonkey.NewPatches` mock functions by overwriting the target function's compiled
machine code in-place with a jump instruction to the replacement, sized for that function's normal
(non-instrumented) layout. `go test -race` recompiles every package with race-detector instrumentation, which
changes each function's compiled layout. The two interact badly: the patch still writes, but at the wrong
offsets/size for the now-larger instrumented function, corrupting it. Unlike the ARM64 case (an immediate
SIGBUS from macOS memory protection), the corruption here left the patched call in a state where it neither
executed the replacement nor errored — it simply hung, forever, taking the whole package's `go test` run down
with it once the package `-timeout` fired. This is a second, independent gomonkey/`-race` incompatibility,
undocumented anywhere in gomonkey's own issue tracker as of this fix; the existing ARM64 skip checks in this
repo were never designed to cover it.

Fixed by generalizing the skip: added `tests.RaceEnabled` (a `//go:build race`/`//go:build !race`-gated
`const bool`, `tests/race_enabled.go` / `tests/race_disabled.go` — Go has no direct runtime API to detect
`-race` at test time) and `tests.SkipIfGomonkeyUnsafe(t testing.TB, reason string)` (`tests/preconditions.go`),
which checks `RaceEnabled` first, then falls through to the pre-existing `darwin/arm64` check. Replaced every
inline `runtime.GOARCH == "arm64"` (and one package-local `skipGomonkeyOnDarwinARM64` helper that only checked
ARM64) gomonkey guard across the four files that use `gomonkey.ApplyFunc`/`gomonkey.NewPatches`:
`internal/exec/terraform_affected_test.go` (`TestGetAffectedComponents`, `TestExecuteTerraformAffected`, and
`BenchmarkGetAffectedComponents` — the benchmark is why `SkipIfGomonkeyUnsafe` takes `testing.TB` rather than
`*testing.T`, since `*testing.B` doesn't satisfy the latter), `internal/exec/terraform_utils_test.go` (updated
the shared `skipGomonkeyOnDarwinARM64` helper's body instead of each of its 3 call sites), `cmd/terraform/lint_test.go`
(2 call sites), and `pkg/vendoring/install/copy_glob_test.go` (4 call sites).

Validation: `go build ./...`, `go vet ./...` — clean. `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
`gofumpt -l` on all touched files — no output. `go test -race -run 'TestGetAffectedComponents|TestExecuteTerraformAffected'
./internal/exec/...` — both skip cleanly under `-race` (previously hung indefinitely). `go test -run
'TestGetAffectedComponents|TestExecuteTerraformAffected|TestRunTerraformLintDispatchesDirectAndAffectedModes|TestRunTerraformLintReturnsPreparationErrors'
./internal/exec/... ./cmd/terraform/...` and the four `pkg/vendoring/install` gomonkey tests — all skip on this
local darwin/arm64 dev machine exactly as before (confirming the generalized helper preserves the pre-existing
ARM64 behavior, not just adding the new `-race` path).

See Follow-ups above for a `cmd/ci` test that reappeared in the same CI log as this round's gomonkey hang.

## Round 14 (root-caused the `pkg/provisioner` "test-hygiene gap" left open in Round 12's Follow-ups)

The same CI run that exposed Round 13's gomonkey hang also failed `pkg/provisioner`'s
`TestAutoProvisionBackendWrapsCreationError` with `An error is expected but got nil` — a different test than
the one Round 12's Follow-ups already flagged (`TestAutoProvisionBackendWritesWarningsToOutputWriter`), in the
same file, with the same symptom shape (expects a mocked backend-create function's error/warnings to surface,
gets a silent no-op instead). Reproduced locally with the CI log's exact `-test.shuffle` seed
(`go test -race -shuffle=1788356795585686757 ./pkg/provisioner/`) — this time actually root-caused instead of
re-deferred, since the same failure shape recurring on a *second* test in the same file (previously guessed to
be "some other test incidentally provides missing setup") pointed at a real, general bug rather than one
test's isolated gap.

`autoProvisionBackend` (`pkg/provisioner/backend_hook.go`) calls `backend.BackendExists` before ever invoking
the registered create function; if that check errors, `autoProvisionBackend` silently returns `nil` ("defer to
terraform init" — `backend_hook.go:69-74`), never calling create. `backend.BackendExists` looks up an
exists-checker from a *separate* package-level registry (`backendExistsCheckers`,
`pkg/provisioner/backend/backend.go`) that s3.go's own `init()` populates at process start with the real,
network-calling `S3BackendExists`. Both `TestAutoProvisionBackendWrapsCreationError` and
`TestAutoProvisionBackendWritesWarningsToOutputWriter` register a mock *create* function but never touch the
*exists* registry, and only call `backend.ResetRegistryForTesting()` via `t.Cleanup` (i.e. after the test, not
before). Whichever of these two tests happens to run *first* under `-shuffle=on` — before anything else in the
package has called `ResetRegistryForTesting`, which wipes `backendExistsCheckers` back to empty — inherits the
real `S3BackendExists` checker. With no AWS credentials/network in CI, that checker errors, `BackendExists`
propagates the error, and `autoProvisionBackend` silently no-ops instead of ever calling the mock create
function the test means to exercise. Every other order works "by accident," relying on an earlier test's
cleanup having already emptied the registry — exactly the ambient-setup dependency Round 12's Follow-ups
suspected, just not yet traced to its actual mechanism.

Fixed by calling `backend.ResetRegistryForTesting()` at the *start* of both tests (in addition to the existing
`t.Cleanup`), so each is self-sufficient regardless of what ran before it: with the registry empty,
`backend.GetBackendExists("s3")` returns `nil`, `BackendExists` takes its documented "no exists checker
registered → assume backend doesn't exist" default (`(false, nil)`), and the mock create function the test
actually registered gets called every time.

Validation: `go build ./pkg/provisioner/...`, `go vet ./pkg/provisioner/...` — clean. `go test -race
-shuffle=1788356795585686757 ./pkg/provisioner/` (the exact CI-failing seed) — passes (previously failed on
`TestAutoProvisionBackendWrapsCreationError`). 5 additional `go test -race -shuffle=on` reruns of the full
package — all pass. Both fixed tests also pass individually via `-run '^Test...$'` (previously
`TestAutoProvisionBackendWritesWarningsToOutputWriter` failed in isolation per Round 12's Follow-ups; it now
passes, confirming the fix removes the dependency on run order rather than papering over this one seed).

## Round 15 (root-caused the recurring `--help` flag leak, for real this time)

Rounds 10 and 11 both attempted to close `TestRootHelpFunc_RealTree_UnknownSubcommandErrors`'s recurring
failure (`toolchain`/`terraform`/`version` + an unrecognized subcommand + `--help` silently succeeding
instead of reporting "Unknown command" and exiting 1), and Round 11 in particular generalized the fix to walk
the *entire* command tree's flags, not just `RootCmd`'s own. It reappeared a third time in the CI run that
also contained Round 13's gomonkey hang. This round finally instrumented the actual failing dispatch (temporary
`fmt.Fprintf(os.Stderr, ...)` debug prints inside `rootHelpFunc`/`rejectUnknownSubcommandForHelp`, removed
before committing) rather than reasoning about it statically, and found the real mechanism -- two distinct,
previously-undiscovered bugs, neither of which is a leaked `pflag.Flag`:

- **A lazily-created flag with no snapshot to restore from.** Cobra's `InitDefaultHelpFlag()` /
  `InitDefaultVersionFlag()` / `InitDefaultCompletionCmd()` all add their flags *lazily*, inside
  `Command.execute()` on a command's first real dispatch -- not at registration time. `NewTestKit`'s
  `restoreFlagsOn` (Round 11) looked up each live flag by name in the snapshot map and silently skipped
  restoring any flag not found there (`if !ok { return }`). A flag that didn't exist yet when a given test's
  snapshot was taken has no "before" state in that snapshot; skipping it means whatever that same test's own
  invocation left the flag at (e.g. `RootCmd`'s own `--help`, `Value: true, Changed: true` after
  `TestPagerDoesNotRunWithoutTTY`'s first subtest) leaks forward to every later test, forever, since no
  *earlier* snapshot ever captured it either. Traced to its origin with a temporary debug print in
  `snapshotRootCmdState`/`restoreRootCmdState` gated behind `ATMOS_DEBUG_HELP_LEAK` (also removed before
  committing), confirming `RootCmd.help` first flips to `Value: true` and never resets starting from that
  exact subtest, in a shuffle-ordered full-package run using the CI log's own `-test.shuffle` seed. Fixed:
  `restoreFlagsOn` now resets any live flag absent from the snapshot to `f.DefValue`/`Changed: false` instead
  of skipping it -- there being no "before" state to restore *is* the fix's premise, since the correct
  baseline for a flag your own snapshot never saw is its own registered default.
- **`SetHelpFunc` leaking across real, package-level singleton commands (the actual proximate cause of the
  observed failure).** `applyColoredHelpTemplateForTopic` (`cmd/help_template.go`) calls
  `cmd.SetHelpFunc(func(c, args) { printHelpForTopic(...) })` on whatever command it renders help for --
  this runs on *every* successful, valid `--help` render (`renderFlagHelp`/`renderInteractiveHelp`/the
  default branch, and `rootHelpFunc`'s own normal completion path), including real commands like
  `toolchain`/`terraform`/`version`. `Command.helpFunc` is an *unexported, per-command function-pointer
  field* on `cobra.Command` -- not a `pflag.Flag` -- so nothing in the flag-walk mechanism (Rounds 10-11)
  ever touched it. The very first test anywhere in the whole binary that successfully renders real
  `--help` for e.g. "toolchain" (any of `TestRootHelpFunc_RealTree_ValidCasesStillRenderHelp`'s "toolchain
  --help" cases, or any other test exercising the same real command) permanently overwrites
  `toolchainCmd.helpFunc` with that one-off closure. From that point on, `Command.HelpFunc()` (which returns
  the local `helpFunc` if non-nil, walking up to the parent only when it's nil) stops walking up to
  `RootCmd`'s `rootHelpFunc` -- Cobra's dispatch for `atmos toolchain <bogus> --help` still resolves and
  parses correctly, but the actual help rendering silently bypasses `rejectUnknownSubcommandForHelp`
  entirely, explaining every observed symptom at once: no call into `rootHelpFunc` (confirmed via the debug
  prints -- zero hits for the failing subtests despite `--help` genuinely being present and recognized),
  `OsExit` never invoked (so `assert.Panics` fails with a nil panic value), and empty captured output (the
  leaked closure renders through `cmd.OutOrStdout()`/its own `ctx.writer`, not the test's swapped
  `os.Stderr` pipe). Fixed by resetting every command's `helpFunc` in the same tree walk: `RootCmd` gets
  `rootHelpFunc` re-set explicitly (defensive, matches its own real registration); every other command gets
  `SetHelpFunc(nil)`, clearing back to "inherits from parent" -- exactly the state a freshly registered
  command that has never rendered help would be in.

Fixing bug 2 exposed a **third, previously-masked issue**: with `rootHelpFunc` finally reachable again,
`TestRootHelpFunc_RealTree_UnknownSubcommandErrors`'s own `os.Pipe()`-based stderr capture deadlocked --
`showUsageAndExit`'s fully-styled "Unknown command" error (hints + the full subcommand list; worst case
"terraform" with 40 children) is large enough to exceed the OS pipe buffer (64KB on macOS/Linux), and the test
only started draining the pipe (`io.Copy`) *after* `RootCmd.ExecuteC()` returned -- which itself couldn't
return until the blocked write completed, a classic unbuffered-pipe deadlock. This had never surfaced before
because bug 2 always intercepted the dispatch first, so this code path had effectively never run to
completion in CI. Fixed by starting the `io.Copy` drain in a goroutine before invoking `ExecuteC()`, so reads
happen concurrently with the write instead of after it.

Files: `cmd/testing_helpers_test.go` (`restoreFlagsOn`'s two fixes above), `cmd/root_help_routing_test.go`
(concurrent pipe drain in `TestRootHelpFunc_RealTree_UnknownSubcommandErrors`).

Validation: `go build ./cmd/...`, `go vet ./cmd/...` -- clean. `./custom-gcl run --new-from-rev=origin/main` --
0 issues. `gofumpt -l` on both touched files -- no output. `go test -race -shuffle=1788355849065485169 ./cmd/`
(the exact CI-failing seed, extracted from the failing job's log) -- passes (previously failed on all three
`TestRootHelpFunc_RealTree_UnknownSubcommandErrors` subtests, then hit the pipe deadlock and timed out at
15m once bug 2 alone was fixed). `go test -race ./cmd/ -run
'TestSnapshotRootCmdState|TestTestKit_|TestRootHelpFunc_RealTree'` -- all pass. 5 additional full-package `go
test -race -shuffle=on ./cmd/` reruns -- 3 passed cleanly; 2 failed on tests unrelated to this round's diff
(see Follow-ups: `TestApplyCIGitCloneBootstrap_*`, already a documented open issue; and a newly-surfaced flake
in an unrelated subsystem, `TestValidateStacksCmd_Failure`, not touching command-tree/flag/helpFunc state this
round's diff modifies). A second newly-surfaced failure from this same validation batch,
`TestTerraformGenerateVarfileCmdNoColor`, was root-caused and fixed in Round 16 below rather than left open.

## Round 16 (root-caused `TestTerraformGenerateVarfileCmdNoColor`, and it turned out to matter for real CI)

Round 15's own validation batch surfaced `TestTerraformGenerateVarfileCmdNoColor` failing intermittently under
`-race -shuffle=on` and it was provisionally logged as an unrelated, deferred flake. The very next real CI run
(after Round 15's fix landed) failed on this exact test, confirming it isn't a local-sandbox curiosity -- it
actively blocks the race job. Root-caused: `TestSetupColorProfileFromEnv` (`cmd/root_test.go`) exercises the
real `setupColorProfileFromEnvWithArgs` (`cmd/root.go`) for its "ATMOS_FORCE_COLOR set" and "force-color flag"
cases, and that function has two genuine, permanent side effects when force-color is detected:
`lipgloss.SetColorProfile(termenv.TrueColor)` and a raw `os.Setenv("CLICOLOR_FORCE", "1")` (not `t.Setenv` --
this env var backs Boa's help-renderer color detection, so it's deliberately process-wide in production, not
scoped to any one command invocation). The test never wrapped itself in `NewTestKit(t)` and never restored
either side effect. Once either subtest ran, `CLICOLOR_FORCE=1` stayed set in the process environment for the
rest of the test binary's life. `TestTerraformGenerateVarfileCmdNoColor` sets `NO_COLOR=1` and expects zero
ANSI codes in its captured output; `configureEarlyColorProfile` (which checks `NO_COLOR` first) still worked
correctly, but a *different*, later-executing path -- `atmosConfig.Settings.Terminal.ForceColor`, populated
from `viper.GetBool("force-color")` during real config load, which is bound to both `ATMOS_FORCE_COLOR` and
`CLICOLOR_FORCE` -- picked up the leaked `CLICOLOR_FORCE=1` and forced the logger back to `TrueColor`,
overriding the test's own explicit `ui.SetColorProfile(termenv.Ascii)` call. Fixed by having
`TestSetupColorProfileFromEnv` wrap itself in `NewTestKit(t)` (restores the lipgloss/theme side of the leak,
matching this codebase's established pattern) and explicitly save/restore `CLICOLOR_FORCE` around each subtest
via `t.Cleanup` (the raw `os.Setenv` call is production code's own side effect, not something the test can
scope with `t.Setenv` itself).

A second, already-repeatedly-documented flake, `TestRunTerraformMigratePlan_NoMigrationsDirSkipsCleanly`,
failed in the same CI run. Investigated again this round with the exact CI-failing `-test.shuffle` seed for
`cmd/terraform/migrate` -- still does not reproduce locally (matching every prior attempt across Rounds 9-15).
Considered deliberately breaking `PATH` in the test (mirroring its sibling
`TestRunTerraformMigrate_SelectWorkspaceErrorPropagates`'s own idiom) to make the "tofu is never invoked"
guarantee deterministic, but backed out of that change: the test's assertion is only `require.NoError`, which
doesn't verify tofu was never invoked in the first place -- if a real (rare) invocation-skip bug exists,
forcing `PATH` to always break would convert an intermittent CI flake into a deterministic CI failure, making
things worse under the exact uncertainty that makes this worth investigating further rather than guessing.
Left open, still tracked below.

Files: `cmd/root_test.go` (`TestSetupColorProfileFromEnv`'s `NewTestKit` + `CLICOLOR_FORCE` save/restore).

Validation: `go build ./cmd/...`, `go vet ./cmd/...` -- clean. `./custom-gcl run --new-from-rev=origin/main` --
0 issues. `gofumpt -l cmd/root_test.go` -- no output. `go test -race ./cmd/ -run
'TestSetupColorProfileFromEnv|TestTerraformGenerateVarfileCmdNoColor'` -- both pass. 4 additional full-package
`go test -race -shuffle=on ./cmd/` reruns -- `TestTerraformGenerateVarfileCmdNoColor` did not fail in any of
them (previously failed in roughly half of comparable reruns); the one failure across the 4 reruns was
`TestValidateStacksCmd_Failure` + `TestApplyCIGitCloneBootstrap_NoCIProviderDetected`, both already-documented,
unrelated pre-existing flakes (see Follow-ups).
