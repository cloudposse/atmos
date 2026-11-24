# Fix: Regressions found by the 1.235.0 regression test against v1.234.1

**Date:** 2026-11-22

## Summary

A regression and field test compared the 1.235.0 candidate with v1.234.1, the latest published release.
This PR fixes the seven regressions it found that came from changes already on `main`:

- Gitea Actions runs failed when the API URL was missing or plain HTTP (R1).
- Gitea runs without a token printed warnings for every skipped API write (R2).
- `git push` failures no longer named the remote and branch (R3).
- Three edition journal entries were dated before v1.234.1 shipped (G3).
- A source that ships `backend.tf.json` failed to provision (T1).
- A user's own `settings.configuration` was read as configuration-provider ownership (G1).
- Unknown sections under `components.terraform` were read as configuration providers (G2).

It also fixes five bugs that already existed in v1.234.1 and that the same pass found: update notices
that compared versions as strings, the `describe affected --ref`/`--sha` deprecation notice on stdout, a
panic on `null:` stack manifest keys, `packer --dry-run` running a real build, and HCL files that were
not truncated before writing. Every fix has a test that failed on the unfixed code.

The work is tracked under #3720. The Atmos Auth regressions (A1-A8) and the other #3720 fixes land in
separate PRs.

## Context

**Method.** The comparison was frozen at two commits:

- Baseline: v1.234.1 at `b148adc244`, published 2026-11-10.
- Candidate: branch `aknysh/fix-auth-review-3720` at `c0f3af7f57`, which is `main` at `2d7822f214`
  plus the #3720 review fixes.

Both binaries were rebuilt from those SHAs with the same toolchain: Go 1.27.4, CGO off, the release
ldflags. Five investigators each covered one area and ran both binaries on identical fixture copies:

- Atmos Auth commands.
- Auth integrations and credential files.
- Terraform/OpenTofu execution.
- Configuration, stacks and the CLI surface.
- Helmfile, Packer, containers, git, CI and docs generate.

Every run used an isolated HOME, XDG directories, a file keyring and its own Git root under `env -i`.
Local fakes or emulators stood in for STS, EKS, ECR, Kubernetes, Vault, S3 and the CI runners, and no
live cloud was used. Each finding was reproduced at least twice from clean state. Evidence stayed in the
local `.context/regression-test/` run directory and is not committed.

The native Terragrunt work introduced configuration providers, and their selection and ownership checks
read user-editable configuration as provider markers: `settings.configuration` (G1) and unknown sections
under `components.terraform` (G2). That one root cause explains both configuration regressions.

**End-to-end rechecks from the regression-test run.** The same isolated fixtures were run on the
v1.234.1 baseline, the frozen candidate, and the fixed build of the original branch:

- G1: `describe component` JSON is byte-identical to the baseline.
- G2: `list stacks` output is byte-identical to the baseline.
- R1 and T1: each matches the baseline.

**Timing from the regression-test run.** `go tool mage bench:fieldTest v1.234.1,5a1bf052a6` compared
v1.234.1 with the fixed build of the original branch. It was run twice: once with the default workload,
then with `ATMOS_BENCH_FIELD_FILTER='^BenchmarkCLI$'` for the whole CLI suite. Both builds used the same
harness and Go 1.27.4, on darwin/arm64 (Apple M3 Max), `GOMAXPROCS=2`, with 10 samples each and nothing
else running.

- No significant regressions. The threshold is 10% and p < 0.05.
- Backend reads are unchanged: 22 for `describe affected` store lookups, 22 for caller-stack store
  lookups, and 11 for `describe stacks` store lookups.
- The largest timing change is `list instances` with 1,000 instances: 0.685 s to 0.709 s (+3.5%,
  p < 0.001).
- Most other workloads changed by 0% to 1.5%. For example, `describe affected` with 10,000 instances
  went from 17.36 s to 17.42 s (+0.4%), and `describe stacks` with 1,000 instances from 1.084 s to
  1.094 s (+1.0%).

**Website from the regression-test run.** `cd website && npm run build` succeeded on the original
branch with no broken links. It reported broken anchors only on AI skill pages that these changes do not
touch (`atmos-aws-cloudformation`, `atmos-cast`, `atmos-ci`, `atmos-helm`, `atmos-hooks`).

## Changes

### CI and Git (regressions from `main`)

- **R1. Gitea Actions runs keep going when the API URL is missing or HTTP**
  (`pkg/ci/providers/gitea/client.go`, `pkg/ci/providers/gitea/provider.go`,
  `errors/errors_integrations.go`).
  - Problem: Gitea runners export `GITHUB_ACTIONS` and `GITEA_ACTIONS`. With `ci.enabled` and a missing
    or HTTP server URL, `ValidateConfig` returned `ErrInvalidURL`, so every command aborted before it ran
    and no job summary or outputs were written.
  - Fix: a missing URL, or an HTTP URL without `ATMOS_CI_GITEA_ALLOW_INSECURE=true`, now returns the new
    `ErrGiteaAPIUnavailable`. `ValidateConfig` logs the reason once per provider (a warning when a token
    is configured, debug otherwise) and lets the run continue. The job summary and outputs still go
    through the runner's files; only commit statuses, check runs and comments are skipped.
  - Requests are still never sent over HTTP without the opt-in. A malformed URL (embedded credentials,
    query, fragment, or a scheme other than `http`/`https`) still stops the run with `ErrInvalidURL`.
  - Tests: `TestUnusableAPIURLDisablesAPIFeaturesWithoutFailing`,
    `TestHTTPAPIURLWithOptInSendsAuthenticatedRequests`, `TestHTTPSAPIURLIsUsableWithoutOptIn`,
    `TestAPIUnavailableIsLoggedOnce`, `TestExecuteWithUnusableAPIURLKeepsSummaryAndOutputs`,
    `TestExecuteNeverSendsTokenOverHTTPWithoutOptIn`, `TestExecuteSendsStatusesOverHTTPWithOptIn`, and
    the updated `TestAPIRejectsUnsafeConfigAndRedirects`.
- **R2. A missing Gitea token is skipped quietly**, at debug level like GitHub
  (`pkg/ci/plugins/terraform/skip_reason.go`, `handlers.go`, `comments.go`).
  - Problem: over HTTPS without `ATMOS_CI_GITEA_TOKEN` or `GITEA_TOKEN`, every plan printed "CI check run
    creation/update skipped" warnings with a wrapped error dump.
  - Fix: `configurationGapReason` treats a missing GitHub token, a missing Gitea token, and
    `ErrGiteaAPIUnavailable` as configuration gaps. Check runs, comments and per-operation statuses log
    them at debug level. Per-operation statuses previously warned for a missing token on both providers,
    so that is quieter too. Unexpected API errors still log as warnings.
  - Tests: `TestSkippedAPIWritesLogConfigurationGapsQuietly`,
    `TestCreatePerOperationStatusesWithoutGiteaTokenIsQuiet` (drives the real Gitea provider), and
    `TestExecuteWithoutTokenSkipsStatusesQuietly`.
- **R3. `git push` failures name the remote and branch again** (`pkg/git/providers/cli/push.go`).
  - Problem: the retry-with-backoff rewrite changed the rejection error from
    `git push rejected: non-fast-forward: origin/main after 0 retries` to
    `git push after 1 attempt(s), 0 retries, elapsed 26ms: max attempts (1) exceeded, last error: ...`.
    That dropped the remote and branch and varied from run to run.
  - Fix: the message reads `<cause>: <remote>/<branch> after N attempt(s), M retries`, for example
    `git push rejected: non-fast-forward: origin/main after 1 attempt(s), 0 retries`. The retry
    executor's generic "max attempts exceeded" wrapper is dropped, the elapsed time moved to a debug log,
    and a remote given as a URL is redacted. Without a branch, only the remote is named. Sentinels and
    the exit code are unchanged.
  - Tests: `TestPushRejectionMessageNamesTargetDeterministically`,
    `TestPushRejectionMessageRedactsRemoteURL`, `TestPushTargetWithoutBranch`,
    `TestPushFailureLogsElapsedAtDebug`, `TestPushRejectionMessageAgainstBareRepository`.

### Editions (regression from `main`)

- **G3. Unreleased edition journal entries are dated after v1.234.1** (`pkg/edition/journal.yaml`).
  - Problem: `plugins.default_alias`, `list.instances.engine_columns` and
    `components.source.working_directory_precedence` were dated 2026-10-28/29, when their PRs were
    opened. They merged on 2026-11-21, after v1.234.1 was published on 2026-11-10. Pins are inclusive,
    so a project pinned to any date up to 2026-11-10 while running v1.234.1 got the new behavior after
    upgrading.
  - Fix: the three entries are dated 2026-11-21, the day they merged. A release-boundary guard
    (`pkg/edition/journal_release_test.go`, data in `pkg/edition/testdata/latest-release.yaml`) fails when
    an entry that is not in the latest release is dated on or before it, or when a shipped entry is
    re-dated or removed. The file is regenerated after each release with
    `ATMOS_REGENERATE_EDITION_RELEASE=<tag>@<date>`.
  - The dating rule and release step are documented in `docs/prd/editions.md` ("Release boundary") and
    `.claude/skills/editions/SKILL.md`. `docs/prd/source-provisioner.md`, two draft blog posts and the
    `list instances`, `plugin install`, `plugin` usage and `terraform source` pages give the new date.
  - Tests: `TestJournalReleaseBoundary`, `TestLatestReleasePinKeepsReleaseBehavior`,
    `TestReleaseBoundaryViolations`, and the updated `TestDefaultAliasFollowsEdition` and
    `TestExplicitAliasIgnoresEdition` in `pkg/plugin/manage`.

### Component sources (regression from `main`)

- **T1. A source that ships `backend.tf.json` provisions again** (`pkg/provisioner/source/refresh.go`,
  `pkg/provisioner/source/source.go`).
  - Problem: with `components.terraform.auto_generate_backend_file` enabled, a component source whose
    module ships `backend.tf.json` failed to provision, so fresh checkouts and existing workdirs could not
    run Terraform. v1.234.1 copied the source and then wrote the stack's backend over it.
  - Fix: `yieldGeneratedBackend` drops the shipped file from the staged copy before the ownership
    inventory is taken, with a warning, and the generated backend replaces it. The path is never recorded
    as source-owned, and a copy recorded by an earlier refresh is removed as obsolete. A directory named
    `backend.tf.json` and every other ownership conflict still fail.
  - Only a path that exists in the staged source counts as shipped. A module root that is absent, or
    that is a file in the source (`ENOTDIR`), ships nothing and no longer fails provisioning. Duplicate
    module roots are checked once.
  - The "verify source URI and credentials" hint now appears only for fetch failures, not for local
    copy, ownership, target or file system failures (`localSourceFailure`).
  - Tests: `TestSourceRefreshYieldsShippedBackendToGeneration` (replaces
    `TestSourceRefreshRejectsGeneratedBackendCollision`), `TestSourceRefreshTakesOverPreviouslyOwnedBackend`,
    `TestSourceRefreshStillRejectsOtherCollisions`, `TestYieldGeneratedBackendModuleRoots`,
    `TestShippedBackendWarning`, `TestAutoProvisionSourceShippedBackendWithGeneration`,
    `TestAutoProvisionSourceLocalConflictHasNoFetchHint`, `TestProvisionFailureHintOnlyForFetchFailures`.
  - `docs/fixes/2026-10-28-terraform-source-refresh-safety.md` now describes this behavior, and
    `docs/fixes/2026-10-28-source-identity-disable-and-workdir-precedence.md` gives the new G3 date.

### Configuration providers (regressions from `main`)

- **G1. A user's `settings.configuration` no longer marks provider ownership**
  (`pkg/schema/component_configuration.go`, `internal/exec/stack_processor_merge.go`).
  - Problem: `settings` is free-form, but any non-nil `settings.configuration` marked a component as
    owned by a configuration provider. A v1.234.1 component with its own `settings.configuration` then
    failed `terraform plan` with a bare "unsupported plugin extension", had its varfile renamed to
    `atmos-<hash>.terraform.tfvars.json`, and got an extra backend ownership receipt. With
    `settings.configuration.provider` plus a component `backend_type`, every stack command failed as
    "owned by the configuration provider".
  - Fix: ownership is the reserved marker `settings.configuration.atmos_provider_owned: true`
    (`schema.ConfigurationOwnedKey`). Only the stack processor sets it, through
    `MarkConfigurationOwnership`, for a Terraform component whose `settings.configuration.provider` is
    selected by an enabled section under `components.terraform`. A marker written in a manifest is
    removed. Discovered and imported units name their provider, so they stay owned.
  - Every ownership reader now uses the marker: `ConfigurationOwned`, `OwnsComponent`,
    `CheckBackendOverrides`, `WatchExcludes` (`pkg/component/configuration`), the backend file receipt
    (`pkg/terraform/backend/file.go`), saved-plan ownership
    (`pkg/component/terraform/configuration_plan.go`), executable-mode routing
    (`pkg/component/terragruntexec/terragruntexec.go`) and the `list instances` engine
    (`pkg/list/instance_engine.go`). Test fixtures that set ownership by hand now set the marker.
  - Tests: `TestUserSettingsConfigurationIsNotProviderOwnership`,
    `TestUserSettingsConfigurationCannotClaimOwnership`, `TestDiscoveredUnitsRemainProviderOwned`,
    `TestSelectsConfigurationProvider`, `TestMarkConfigurationOwnership`,
    `TestOwnsComponentReadsTheOwnershipMarker`, `TestGenerateFileIgnoresUserSettingsConfiguration`.
- **G2. Unknown sections under `components.terraform` are ignored**
  (`pkg/component/configuration/providers.go`, `registry.go`,
  `pkg/plugin/bootstrap/components_catalog.go`).
  - Problem: any mapping with `enabled` or `provider` under `components.terraform` was treated as a
    configuration-provider section. A project's own mapping such as
    `team_defaults: {enabled: true, owner: platform}`, which v1.234.1 ignored, made every stack command
    fail with "unsupported plugin extension".
  - Fix: `Attach` first keeps only sections that select a built-in provider (`terragrunt`) or a provider
    that an enabled plugin package exports for component discovery. The exports are read from the lock
    file or a local build's manifest, without installing or starting anything. Other sections are
    dropped with a debug log, so they start no provider session and `describe config` no longer lists
    them. When a package's exports are unknown until it is installed, every section is kept and the
    provider reports the problem.
  - Tests: `TestSelectProvidersDropsUnknownSections`, `TestSelectProvidersSkipsCatalogForBuiltins`,
    `TestSelectProvidersKeepsSectionsWhenExportsAreUnknown`, `TestAttachIgnoresUnknownSections`,
    `TestRegisterProviderCatalog`, `TestConfigurationProvidersReadsDeclaredExports`,
    `TestConfigurationProvidersWithoutPackagesReadsNothing`,
    `TestConfigurationProvidersReportsUnknownExports`, `TestAttachKeepsDeclaredPluginProviderSection`,
    `TestUnknownEnabledTerraformSectionIsNotAProvider`.

### Pre-existing bugs fixed (present in v1.234.1)

- **`atmos version` compares versions semantically** (`pkg/upgrade/compare.go`,
  `internal/exec/version.go`, `cmd/cmd_utils.go`).
  - Problem: `atmos version`, `--check`, `--format json/yaml` and the periodic update notice compared
    versions as strings, so a newer build or prerelease such as `1.235.0-candidate` was told to update
    to 1.234.1.
  - Fix: `upgrade.CompareVersions` and `upgrade.NewerRelease` use semantic versions and accept a leading
    `v`. An update is suggested only when the latest release is strictly newer. Equal, older and
    unparsable versions (such as the development version `test`) get no suggestion; a prerelease is still
    offered its final release.
  - Tests: `TestCompareVersions`, `TestNewerRelease`, `TestGetLatestVersionComparesSemantically`,
    `TestCheckReleaseNeverSuggestsOlderRelease`, `TestDisplayVersionInFormatOmitsOlderUpdateVersion`,
    `TestCheckForAtmosUpdateNeverSuggestsOlderRelease`. Existing notice tests now set a released version.
- **The `describe affected --ref`/`--sha` deprecation notice goes to stderr** (`cmd/describe_affected.go`).
  - Problem: Cobra's `MarkDeprecated` prints through the command's output writer, which Atmos routes to
    stdout, so the notice broke `--format json` (the default) and `yaml` output.
  - Fix: the flags are hidden from help and their use is reported with `ui.Warningf` on stderr.
  - Tests: `TestDescribeAffectedDeprecatedFlagsWarnOnStderr`,
    `TestDescribeAffectedWithoutDeprecatedFlagsDoesNotWarn`, `TestDescribeAffectedDeprecatedFlagsStayHidden`.
- **A `null:` stack manifest key is a validation error** (`internal/exec/stack_manifest_null_keys.go`,
  `internal/exec/stack_processor_utils.go`).
  - Problem: YAML decodes an unquoted `null:` or `~:` key as a nil map key. JSON Schema validation then
    panicked, so `validate stacks`, `describe stacks` and every stack command crashed.
  - Fix: right after a manifest is decoded, a null mapping key returns `ErrInvalidStackManifest` naming
    the file and key path, with a hint to quote the key (`"null":`).
  - Tests: `TestStackManifestNullKeyIsValidationError`, `TestStackManifestQuotedNullKeyIsAllowed`,
    `TestStackManifestNullValueIsAllowed`.
- **`packer --dry-run` previews the command instead of running Packer** (`cmd/packer.go`,
  `internal/exec/packer.go`).
  - Problem: the packer command never registered `--dry-run`, so its `FParseErrWhitelist` silently
    dropped the flag and `packer build --dry-run` ran a real build and wrote the variable file.
  - Fix: `--dry-run` is registered on the packer command through `flags.NewStandardParser`, as helmfile
    does. With it, Atmos prints the masked command that would run and returns before running Packer.
    `--dry-run=false` and omitting the flag still run Packer, and the flag is never forwarded to Packer.
    The packer `build` and usage pages document it, and the `atmos packer --help` screengrab cast
    (`website/static/casts/screengrabs/atmos-packer--help.cast`) was regenerated.
  - Tests: `TestPackerDryRun` (build and validate, with `--dry-run`, `--dry-run=true`,
    `--dry-run=false` and no flag).
- **HCL varfiles and backend files are truncated before writing** (`pkg/utils/hcl_utils.go`).
  - Problem: `WriteToFileAsHcl` and `WriteTerraformBackendConfigToFileAsHcl` opened files without
    `os.O_TRUNC`, so regenerating shorter content with `generate varfiles --format hcl` or
    `generate backends` left the old tail behind and produced invalid HCL.
  - Fix: both writers open the file with `os.O_TRUNC`.
  - Tests: `TestWriteToFileAsHclReplacesExistingContent`, `TestWriteToFileAsHclCreatesFile`,
    `TestWriteTerraformBackendConfigToFileAsHclReplacesExistingContent`, `TestWriteHclRejectsUnwritablePath`.

### Other documentation

- `website/docs/integrations/gitea-actions/setup.mdx` describes the R1 behavior.
- `website/docs/cli/commands/git/git-push.mdx` shows the R3 message and the debug-level elapsed time.

## Validation

All checks ran in this PR's worktree on darwin/arm64 with Go 1.27.4.

**Build and static checks.**

- `go build ./...` passes.
- `go vet` passes on the 22 changed Go packages.

**Tests.**

- `go test -count=1` passes on the 20 changed packages other than `./cmd` and `./internal/exec`:
  `cmd/terraform`, `cmd/terraform/planfile`, `errors`, `pkg/ci/plugins/terraform`,
  `pkg/ci/providers/gitea`, `pkg/component/configuration`, `pkg/component/terraform`,
  `pkg/component/terragruntexec`, `pkg/edition`, `pkg/git/providers/cli`, `pkg/list`,
  `pkg/plugin/adapter/configuration`, `pkg/plugin/bootstrap`, `pkg/plugin/manage`,
  `pkg/provisioner/source`, `pkg/schema`, `pkg/terraform/backend`, `pkg/terraform/output`,
  `pkg/upgrade`, `pkg/utils`.
- `./cmd`: the tests in the new and changed test files pass
  (`TestCheckForAtmosUpdateNoticeAndDailyCache`, `TestCheckForAtmosUpdateNeverSuggestsOlderRelease`,
  `TestDescribeAffectedDeprecatedFlags*`, `TestDescribeAffectedWithoutDeprecatedFlagsDoesNotWarn`,
  `TestPackerDryRun`).
- `./internal/exec`: the 40 test functions in the new and changed test files pass (the configuration imports,
  ownership and settings-ownership tests, the describe-affected configuration and watch-cache tests, the
  null-key tests, and the version compare and notice tests).
- The full `./cmd` and `./internal/exec` suites take about 17 minutes each and run in CI.

**Coverage.** Package statement coverage from `go test -count=1 -cover`, before (`origin/main` at
`2d7822f214`) and after (this PR). `./cmd` and `./internal/exec` are not measured here.

| Package | Before | After |
| --- | --- | --- |
| `errors` | 87.3% | 87.3% |
| `pkg/ci/plugins/terraform` | 93.6% | 94.0% |
| `pkg/ci/providers/gitea` | 96.5% | 96.6% |
| `pkg/component/configuration` | 85.4% | 86.2% |
| `pkg/component/terraform` | 0.4% | 0.4% |
| `pkg/component/terragruntexec` | 98.4% | 98.4% |
| `pkg/edition` | 99.1% | 99.1% |
| `pkg/git/providers/cli` | 90.4% | 90.6% |
| `pkg/list` | 76.0% | 76.0% |
| `pkg/plugin/bootstrap` | 91.0% | 91.2% |
| `pkg/provisioner/source` | 90.7% | 91.0% |
| `pkg/schema` | 91.0% | 91.1% |
| `pkg/terraform/backend` | 74.0% | 74.0% |
| `pkg/upgrade` | 100.0% | 100.0% |
| `pkg/utils` | 83.6% | 85.8% |

`pkg/component/terraform` is low because its logic is mostly exercised from `internal/exec`; its own tests
pass. Packages showing no change got one-line fixes whose lines were already covered, or (`pkg/edition`)
only a data change.

**Not run.**

- golangci-lint; CI lint is the first lint pass.
- The full `./cmd` and `./internal/exec` suites, and `TestCLICommands`.
- `cd website && npm run build` and screengrab regeneration in this worktree; the results under
  Context are from the regression-test run on the original branch.
- Linux and Windows test runs.

## Follow-ups

**Choices made in these fixes that can be revisited.**

- A Gitea HTTP URL without the opt-in logs a warning only when a token is configured; otherwise the
  skip is logged at debug level.
- Edition journal entries are dated on the day they merge into `main`, and
  `pkg/edition/testdata/latest-release.yaml` must be regenerated after each release (documented in
  `docs/prd/editions.md`).
- `settings.configuration.atmos_provider_owned` is visible in describe output for discovered units.

**Release notes.** `list instances` shows the Type and Engine columns by default for projects that are
unpinned or pinned on or after 2026-11-21. That change is edition-gated and was intended; only its date
moved.

**Help cast.** `atmos packer --help` now lists `--dry-run`, and its cast was regenerated with
`atmos --chdir=demo/casts casts generate screengrabs cli --filter "atmos packer"`. The packer subcommand
casts render the same text as before, so they were left unchanged.

**Bugs present in both v1.234.1 and the candidate that were not fixed here** (outside Atmos Auth). Each
needs a decision (fix now, or open an issue):

- Stores: the SSM `!store.get` key is prefixed twice. The Vault error path is nondeterministic.
- Terraform:
  - With a local backend, `init_run_reconfigure: true` and existing workspaces, a forced
    `init -reconfigure` hangs on Terraform's migration prompt without a terminal.
  - `generate varfiles -s` is silently ignored (the filter is `--stacks`).
  - After-apply hooks run for `apply --dry-run`.
  - `plan-diff` ignores `-var`.
  - Discovery commands that evaluate functions select workspaces and rewrite `.terraform/environment`.
  - `resources_to_replace` is always 0.
- CLI and config:
  - `--profile <name>` fails when the name equals a command (for example `ci`).
  - `--base-path /nonexistent list stacks` exits 0.
  - An `atmos.yaml` `!exec` runs 6 times per invocation.
  - `describe workflows`/`list workflows` order, and duplicate-component resolution in `list values`,
    are nondeterministic.
  - The quick-start-advanced example workflows fail their own schema validation.
- Runtimes:
  - Helmfile flags placed after the component without `--` are dropped.
  - `docker build` ignores a custom `DOCKER_HOST`.
  - A workflow container step's `with.env` does not reach `docker exec`.
  - A git rebase conflict leaves the workdir mid-rebase.

**Not covered by the regression test.**

- Live Kubernetes, live cloud and CI APIs.
- Terraform Cloud and remote backends.
- Plugin, Terragrunt and artifact features (checked only for absence of side effects).
- Linux and Windows.
