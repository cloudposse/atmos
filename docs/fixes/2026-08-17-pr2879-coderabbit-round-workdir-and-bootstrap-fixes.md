# Fix: Address CodeRabbit round on PR #2879 (workdir path safety, CleanWorkdir, CI clone bootstrap)

**Date:** 2026-08-17

## Summary

CodeRabbit left 8 review comments on PR #2879 (branch `osterman/test-container-fields-ignored`).
Each was verified against current code (not just the diff CodeRabbit saw) before acting, since
comments on a moving branch are often stale. 6 were real and fixed here; 2 were stale (the code
already did what CodeRabbit wanted, or the underlying claim didn't hold) and need only a thread
reply, not a code change.

## Context

- **Fixed, minor:** `pkg/provisioner/source/source.go`'s `validateWithinComponentBasePath`
  discarded the underlying filesystem error (`EACCES`, `ENOTDIR`, etc.) when
  `resolveExistingSymlinks` failed, losing troubleshooting detail.
- **Fixed, security:** `workdir.BuildPath` validated `stack` only against escaping `basePath`
  entirely, missing an in-bounds collision: a stack value containing a real `/` — e.g.
  `team/../prod` — folded to the same workdir as stack `prod` via `filepath.Join`'s implicit
  `Clean()`, letting two distinct stack configs silently share one workdir's files, metadata,
  and Terraform state.
- **Fixed, correctness:** `pkg/provisioner/workdir.CleanWorkdir` passed `nil` for
  `componentConfig` to `BuildPath`, so it never honored an `atmos_component` instance-name
  override — for a component provisioned under such an override, cleanup derived the *base*
  component's path, found nothing there, and silently reported success without removing the
  real workdir. Investigating this surfaced a deeper, related bug: the actual CLI-wired
  implementation (`cmd/terraform/workdir/workdir_helpers.go`'s `DefaultWorkdirManager`) doesn't
  call `pkg/provisioner/workdir.CleanWorkdir`/`BuildPath` at all — it has its own separate,
  never-updated `fmt.Sprintf("%s-%s", stack, component)` formula in `CleanWorkdir`,
  `GetWorkdirInfo`, and (transitively) `DescribeWorkdir`, meaning `atmos terraform workdir
  clean/get/describe` couldn't find *any* hyphenated component's real workdir, not just the
  `atmos_component`-override case CodeRabbit flagged.
- **Fixed, scoped:** the hyphen/separator-escaping `BuildPath` change (from an earlier round of
  this same PR, see `docs/fixes/2026-08-14-workdir-buildpath-collision-and-stack-traversal.md`)
  changes the on-disk path for any component name needing escaping, so a workdir an earlier
  commit of this branch created under the pre-escaping formula becomes unreachable the next time
  that component is provisioned — silent state loss for a local-backend component, since the JIT
  workdir is Terraform's actual working directory in that case.
- **Fixed, small:** `cmd/git/bootstrap.go`'s `CIGitCloneBootstrapRequestedFromRawArgs` returned
  `false` on a clone flag-parse error, with a comment claiming this "defers the actual error to
  the command's own RunE." It didn't: `cmd/root.go`'s `handleConfigInitErrorWithArgs` only
  swallows an unrelated config/profile error and lets control reach Cobra's real flag parser
  when this function returns `true`; returning `false` let the unrelated error win instead,
  and `Execute()` returns it immediately without Cobra ever seeing the malformed flag.
- **Fixed, changelog:** the strict-YAML-decoding change (rejecting unknown fields under
  container `with:`/`driver:`) and the workdir path-encoding change are both real, user-facing
  breaking changes from this PR with no changelog/blog post; added one.
- **Stale, no fix:** `docs/fixes/2026-08-07-config-load-and-container-validation-error-visibility.md`'s
  `run.pull: sometimes` error example — verified `pkg/runner/step/container.go`'s
  `invalidContainerField` really does build the error via sentinel `ErrStepFieldRequired`
  ("required field missing for step") for *all* invalid container field values, not just
  missing ones, so the doc already matches current code output exactly.
- **Stale, no fix:** `pkg/schema/task.go`'s `yamlNodeFromMapValue` and Viper key lowercasing —
  verified `commands:` (which carries `with:`/`container:` for custom commands) is extracted
  separately in `pkg/config/load.go`, bypassing Viper's key-insensitive merge entirely, so
  nested `env:`/`build_args:` keys already arrive case-preserved; confirmed by an existing
  passing test (`TestCustomCommandContainerStepMappingOverrideDecodesFully`) that loads through
  the real `InitCliConfig` path.

## Changes

- `pkg/provisioner/source/source.go`: added `.WithCause(err)` to both
  `resolveExistingSymlinks` error branches in `validateWithinComponentBasePath`.
- `pkg/provisioner/workdir/types.go`: `BuildPath` now rejects a `stack` value containing a
  literal `.` or `..` path segment outright (new `validateStackForPath` helper) instead of only
  checking containment after the fact. Rejecting the specific dot segments rather than escaping
  the whole value (unlike the component name's `escapeComponentNameForPath`), or rejecting every
  `/`/`\` (this fix's first revision — see Follow-ups), avoids changing the on-disk path for the
  common case of a stack name containing a literal `-` (e.g. `us-east-1-dev`) or a real, existing
  "/"-nesting convention (e.g. `deploy/test`, used by `cmd/terraform/migrate`'s own test
  fixtures). Also extracted `resolveWorkdirComponentName` to keep `BuildPath` under the repo's
  function-length lint limit.
- `pkg/provisioner/workdir/clean.go`: `CleanWorkdir` now takes a `componentConfig map[string]any`
  parameter, forwarded to `BuildPath`; `CleanOptions` gained a matching `ComponentConfig` field.
- `cmd/terraform/workdir/workdir_helpers.go`: `WorkdirManager`'s `CleanWorkdir`, `GetWorkdirInfo`,
  and `DescribeWorkdir` now take `componentConfig` and delegate to `provWorkdir.BuildPath`
  instead of their own hand-rolled formula. New `resolveComponentConfig` helper (best-effort,
  via `internal/exec.ExecuteDescribeComponent`) resolves it from the current stack config,
  falling back to `nil` when the component can no longer be described — e.g. an orphaned workdir
  for a component since removed from stack manifests — so cleanup of stale workdirs still works.
  `cmd/terraform/workdir/workdir_clean.go`, `workdir_show.go`, `workdir_describe.go` updated to
  call it. Regenerated `mock_workdir_manager_test.go` via `go generate`.
- `pkg/provisioner/workdir/interfaces.go`, `fs.go`: added `FileSystem.Rename` (and
  `DefaultFileSystem` implementation); regenerated `mock_interfaces_test.go`.
- `pkg/provisioner/workdir/workdir.go`: `migrateLegacyWorkdir`, called from
  `createWorkdirDirectory` before creating a fresh directory, checks for a sibling at the
  pre-escaping path and renames it forward if the new (encoded) path doesn't already exist. The
  two "nothing to migrate" cases (the two formulas already agree, or no directory exists at the
  legacy path) return `nil`. A genuine `Rename` failure (permissions, filesystem error, etc.) is
  fail-closed: `migrateLegacyWorkdir` returns that error (via `fmt.Errorf`/`%w`, matching how
  `BuildPath` and `MkdirAll`'s failures reach this function) instead of logging and swallowing
  it, and `createWorkdirDirectory` propagates it — wrapped in `errUtils.ErrWorkdirCreation`,
  matching its other two error paths — instead of falling through to `MkdirAll`. (Updated
  2026-08-17, same day: the original best-effort behavior described above — logging and
  proceeding to a fresh `MkdirAll` on any `Rename` error — was a real data-loss bug caught by a
  later CodeRabbit review round on this same PR. Treating a real `Rename` failure the same as
  "nothing to migrate" let provisioning create a fresh, empty workdir at the new path and
  permanently orphan the legacy directory — which may hold real Terraform state, e.g. a
  local-backend component's `terraform.tfstate` — with Terraform then reinitializing against
  empty state.)
- `cmd/git/bootstrap.go`: `CIGitCloneBootstrapRequestedFromRawArgs` returns `true` (not `false`)
  on a clone flag-parse error, so the caller tolerates an unrelated config/profile error and
  lets Cobra's own parser report the malformed flag.
- `website/blog/2026-08-17-container-config-validation-and-workdir-path-encoding.mdx`: new
  changelog post covering both the strict-decode and workdir-path-encoding breaking changes.
- `docs/fixes/2026-08-14-workdir-buildpath-collision-and-stack-traversal.md`: added an "Update
  (2026-08-17)" section documenting the three points above that continue that fix, plus a
  Follow-ups note on the pre-existing sync-deletion gap found below.

## Validation

- `go build ./...` — clean, after every change.
- `go vet ./...` — clean.
- `atmos lint --changed` (patch-scoped `./custom-gcl run --new-from-rev=origin/main`) — 0 issues,
  re-run after each round of fixes.
- `go test ./pkg/provisioner/... ./cmd/terraform/workdir/... ./pkg/terraform/output/...
  ./internal/terraform_backend/... ./pkg/component/... ./pkg/schema/... ./pkg/runner/step/...
  ./cmd ./cmd/git/...` — all pass, including:
  - New: `TestBuildPath_RejectsStackContainingSlash`/`_RejectsStackContainingBackslash`/
    `_AllowsHyphenatedStackName`, `TestContainWithinBase_RejectsEscapingPath`
    (`pkg/provisioner/workdir/types_test.go`).
  - New: `TestCleanWorkdir_HonorsAtmosComponentOverride`/
    `_NilComponentConfigMissesAtmosComponentInstance` (`pkg/provisioner/workdir/clean_test.go`)
    — the latter documents the pre-fix failure mode as a regression guard.
  - New: `TestServiceProvision_MigratesPreExistingLegacyWorkdir`
    (`pkg/provisioner/workdir/integration_test.go`) — end-to-end: pre-creates a workdir at the
    legacy path with a marker provider-lock file, provisions, and asserts the directory was
    renamed (not recreated) with its contents intact.
  - New: `TestMigrateLegacyWorkdir_SkipsWhenNewPathAlreadyExists`/`_SkipsWhenLegacyPathMissing`/
    `_ReturnsErrorOnRenameFailure` (renamed same day from `_ToleratesRenameError` once the
    fail-closed contract landed — see the Update note above) (`pkg/provisioner/workdir/
    workdir_test.go`) — mock-level edge cases; the rename-failure case now asserts an error is
    returned rather than swallowed. Also new same day:
    `TestServiceProvision_MigrateLegacyWorkdirRenameFails_DoesNotCreateFreshWorkdir` — an
    end-to-end regression test proving `Provision` returns `errUtils.ErrWorkdirCreation` and
    never calls `MkdirAll` when the legacy-workdir rename fails.
  - New: two rows on `TestHandleConfigInitError_CIGitCloneBootstrap` and two on
    `TestCIGitCloneBootstrapRequestedFromRawArgs` covering the malformed-`--depth`-with-
    invalid-profile scenario, both in and out of a detected CI provider.
  - Updated ~40 existing call sites across 5 `cmd/terraform/workdir/*_test.go` files and
    `pkg/provisioner/workdir/{clean,integration,workdir}_test.go` for the new
    `componentConfig`/`Rename` mock signatures.
- `cd website && npm run build` — succeeds; the one broken-anchor warning it reports is
  pre-existing and on an unrelated page (`/changelog/mcp-for-ai-coding-assistants`).

## Follow-ups

None currently open. Two issues surfaced after this fix's first commit, both fixed directly in
follow-up commits on the same branch rather than left open:

- Writing the legacy-workdir-migration regression test surfaced a separate, pre-existing gap —
  `SyncDir` deleting a component's local-backend `terraform.tfstate` on every re-provision —
  initially deferred here as out of scope for a workdir-*path* fix. See
  `docs/fixes/2026-08-17-workdir-sync-deletes-local-backend-state.md`.
- CI (`cmd/terraform/migrate`'s own tests, and `tests/cli_workdir_test.go`'s
  `TestCLIWorkdirCommands/clean_specific`) caught a real regression in `validateStackForPath`'s
  first revision: rejecting *every* stack value containing `/` or `\` broke the legitimate,
  already-tested `deploy/test`-style stack-nesting convention, since only a `.`/`..` *segment*
  is actually a collision/traversal risk — plain `/`-nesting is not. Narrowed to reject only
  `.`/`..` segments (see the Changes section above); `tests/cli_workdir_test.go`'s
  `testWorkdirShow`/`testWorkdirDescribe`/`testWorkdirCleanSpecific` fixtures, which hand-rolled
  a pre-escaping workdir path for a hyphenated component name, were also updated to compute
  their expected path via `BuildPath` instead (the same class of fixture drift `CleanWorkdir`'s
  fix above addressed) — `testWorkdirShow`/`testWorkdirDescribe` had been silently masking their
  own breakage via a weak `assert.Contains` check that passed on the error output too, now
  tightened to `require.NoError`.
