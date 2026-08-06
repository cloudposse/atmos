# Fix: `atmos terraform migrate` reports "no migrations" cleanly; unknown hook `kind` and invalid `on_failure` are hard errors

**Date:** 2026-08-03

## Summary

Three fixes surfaced by a manual field-test pass of `atmos terraform migrate`:

1. Zero-config `tfmigrate` history mode no longer crashes for a component with no `migrations/`
  directory yet — it now prints a friendly informational message and skips invoking `tfmigrate`.
2. A `kind: tfmigrate` (or any other) hook whose `kind:` value is not registered now fails hard
  with an actionable error, instead of silently no-opping.
3. A hook's `on_failure:` value that is not `warn`, `fail`, or `ignore` now fails hard at
  preflight, instead of silently behaving like `warn`.

## Context

A field-test pass of `atmos terraform migrate` (hands-on manual DX testing, not automated-test
coverage) found that the documented "zero-config, nothing to set up" history-mode workflow
crashed on the very first real use: any component without a pre-existing `migrations/`
subdirectory hit `MigrationDirFor`'s component-root fallback, which made `tfmigrate`'s own
history-mode file scan try (and fail) to parse Atmos's generated `backend.tf.json` and
`<component>.terraform.tfvars.json` as migration files. Since "no migrations directory" really
does mean "no migration to run," the fix is to detect that case up front and report it cleanly
rather than let `tfmigrate` choke on unrelated generated files.

Two related preflight gaps were found in the same pass and confirmed by the user as separate,
authorized fixes: an unregistered hook `kind:` (e.g. a typo like `tfmigrat`) silently no-op'd the
hook with no signal to the user, even for a state-affecting hook kind; and an invalid
`on_failure:` value (e.g. `waarn`) fell through to the same behavior as `warn` with no validation
error, even though the stack manifest JSON schema (`pkg/datafetcher/schema/atmos/manifest/1.0.json`)
already declares an enum of `warn`/`fail`/`ignore` for this field — the runtime just never enforced
it. Both are now hard preflight errors, consistent with the existing `hooks.go` preflight pattern
that already surfaces a missing-binary failure before terraform runs rather than mid-lifecycle.

## Changes

- `pkg/terraform/tfmigrate/default_config.go`: added `HasMigrationsDir` (extracted from
  `MigrationDirFor`) and `NoMigrationsToRun(migration, componentDir string) bool`.
- `cmd/terraform/migrate/migrate.go`: `executeTfmigrateSingle` now checks
  `tfmigrate.NoMigrationsToRun` right after generating the zero-config default `.tfmigrate.hcl`
  (only when Atmos, not the user, controls `migration_dir`) and returns early with a `ui.Info`
  message instead of invoking `tfmigrate`.
- `errors/errors.go`: added `ErrUnknownHookKind` and `ErrInvalidHookOnFailure` sentinels.
- `pkg/hooks/hooks.go`:
  - `runHookIfMatch`'s unknown-kind branch now returns `unknownHookKindError(name, hook.Kind)`
    instead of `log.Debug` + `return nil`.
  - `verifyHookBinary` (the preflight path, run once per event before any hook executes) now
    also returns `unknownHookKindError` for an unregistered kind, and calls a new
    `verifyOnFailureValue` first, which rejects any `hook.OnFailure` value outside
    `"", warn, fail, ignore`.
  - New helpers `unknownHookKindError` (hint lists `ListKinds()`) and `verifyOnFailureValue`
    (hint lists the three valid literals).
- Tests: `pkg/terraform/tfmigrate/default_config_test.go` (`TestHasMigrationsDir`,
  `TestNoMigrationsToRun`), `cmd/terraform/migrate/migrate_test.go`
  (`TestRunTerraformMigratePlan_NoMigrationsDirSkipsCleanly`, plus a new hermetic
  `testdata/nomigrations/` fixture with no `migrations/` directory), `pkg/hooks/hooks_test.go`
  (`TestHooksVerifyAllBinaries` split into separate "unregistered kind errors" and "invalid
  on_failure errors" subtests; the previous "skips ... unknown ... hooks" case was updated since
  unknown kinds no longer skip).
- `examples/hooks-tfmigrate-advanced/`: kept five new regression fixtures built during the field
  test (`mode-apply-mismatch-demo`, `mode-plan-mismatch-demo`, `history-nodir-demo`,
  `onfailure-ignore-demo`, `multi-state-diffvars-{source,target}`) and updated the README/stack
  comments to describe current (fixed) behavior instead of the original bug repro.
- `cmd/terraform/migrate/migrate.go`: the initial version of the Fix 1 change nested the new
  check inside `executeTfmigrateSingle`, pushing it past this repo's `nestif`/cyclomatic-complexity
  lint thresholds. Extracted the whole default-config-generation-and-skip-check block into
  `resolveTfmigrateDefaultConfig`, returning a small `tfmigrateDefaultConfigResolution` struct
  (`Options`, `Cleanup`, `Skip`) to stay within the 3-return-value limit, per CLAUDE.md's mandatory
  cyclomatic-complexity refactoring guidance (extract into a named helper, keep the orchestrator a
  flat pipeline).

## Validation

- `go build ./...` — clean.
- `go test ./errors/... ./pkg/hooks/... ./pkg/terraform/tfmigrate/... ./cmd/terraform/migrate/...`
  — all pass, including the new/updated tests above.
- `go test ./...` (full repo) — all packages pass except `github.com/cloudposse/atmos/tests`,
  which hit the documented pre-existing local hang (emulator/toolchain preflight taking longer
  than the 10-minute per-package default under plain `go test ./...`, not a regression from this
  change - see the "CLI tests hang: podman auto-start" note in project memory). No test-cases
  under `tests/test-cases/` reference `tfmigrate` or hooks, so this package is not expected to be
  affected by these changes.
- Manually rebuilt `./build/atmos` and re-ran all three original crash repros live:
  - `atmos terraform plan history-nodir-demo -s test` (examples/hooks-tfmigrate-advanced) now
    prints `No tfmigrate migrations found for history-nodir-demo in stack test - nothing to do. ...`
    and the underlying `terraform plan` proceeds normally (previously: crashed decoding
    `backend.tf.json` as a migration file).
  - A hook with `kind: tfmigrat` (typo) now fails with `unknown hook kind` / hint listing valid
    kinds (previously: silent no-op, only visible via `--logs-level Debug`).
  - A hook with `on_failure: waarn` (typo) now fails with `invalid hook on_failure value` / hint
    listing `warn, fail, ignore` (previously: silently behaved like `warn`).
  - Confirmed both hard-error cases fail at preflight, before any state file is created (no
    partial mutation).
- Lint (`./custom-gcl run --new-from-rev=origin/main`, this repo's real CI gate) is clean on this
  patch's own changes after the `resolveTfmigrateDefaultConfig` extraction above (re-scoped to
  `./cmd/terraform/migrate/...`: `0 issues`). A full patch-scoped run still surfaces one
  pre-existing `cyclomatic complexity` finding on `ActionForMode`
  (`pkg/terraform/tfmigrate/tfmigrate.go`), which this patch does not touch; left for separate
  triage.

## Follow-ups

None.
