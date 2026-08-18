# Fix: PR #2879 CodeRabbit round 4 — vendoring/sync/migration path-identity gaps

**Date:** 2026-08-17

## Summary

CodeRabbit's 4th review round on PR #2879 flagged 9 items. Five needed real changes: a test-infra
correctness bug (a test helper that silently swallowed unrelated panics) and four independent
path-identity gaps in the workdir/vendoring subsystem this PR has been hardening all along —
each a variant of the same class of bug this branch keeps finding: something treats two distinct
identities (components, stacks, or a component and the shared directory it lives under) as
interchangeable when they should be distinguished. Three items were doc-only corrections (stale
CLI command name, a stale "still a live vector" claim, a stale test-name list). One item (adding
`perf.Track` to `pkg/schema/workflow.go`'s `MarshalJSON`/`UnmarshalJSON`) was invalid — verified
it would create an import cycle (`pkg/perf` imports `pkg/schema` for `perf.Track`'s
`*schema.AtmosConfiguration` parameter) and left it as-is.

## Context

- **Fixed:** `cmd/custom_command_integration_test.go`'s `cancellationOsExitStub` (added in the
  previous round) absorbed *every* panic in its `recover()`, not just the expected
  `errUtils.OsExit` interception. A real regression panicking in the same goroutine after the
  helper subprocess wrote its `started` marker would be silently swallowed, and the cancellation
  test could pass for the wrong reason (goroutine ended early, `completed` never written, which
  is exactly what a correct cancellation *should* look like too).
- **Fixed, data integrity:** `pkg/provisioner/source/source.go`'s
  `validateWithinComponentBasePath`/`isWithinBase` permits `target == base`. A component name of
  `"."` or `"child/.."` makes `DetermineTargetDirectory`'s default vendoring-target branch
  resolve to the shared `components/<type>/` directory itself, not a component-specific
  subdirectory — `Provision` would then vendor into (and other steps would operate on) that
  shared directory as if it belonged to one component.
- **Fixed, correctness:** `pkg/provisioner/workdir/fs.go`'s `shouldSkipSyncFile` used
  `filepath.Base(relPath)` for the three local-backend state filenames
  (`terraform.tfstate`/`.backup`/`.terraform.tfstate.lock.info`), so it also matched a *nested*
  source file with the same basename anywhere under the workdir (e.g. a real fixture at
  `examples/terraform.tfstate`) — silently excluding legitimate source content from sync instead
  of only protecting the workdir's own root-level state.
- **Fixed, data integrity:** `pkg/provisioner/workdir/workdir.go`'s `legacyWorkdirName(stack,
  component) = fmt.Sprintf("%s-%s", stack, component)` is not injective — `("dev-a", "b")` and
  `("dev", "a-b")` both produce legacy name `"dev-a-b"`. `migrateLegacyWorkdir` now verifies the
  legacy directory's on-disk `WorkdirMetadata` actually matches the component/stack being
  migrated before renaming, failing closed (with a manual-migration hint) when metadata is
  missing, unreadable, or belongs to a different identity — otherwise a second identity
  provisioned under the new encoding could silently rename the first identity's workdir (and any
  real Terraform state inside it) out from under it.
- **Fixed, data integrity:** `pkg/provisioner/workdir/types.go`'s `validateStackForPath` split
  `stack` with `strings.FieldsFunc`, which silently drops empty segments, so `"deploy//test"` and
  `"/deploy/test"` both produced the same segment list as `"deploy/test"` — no `.`/`..` segment
  was ever visible to the check, even though `filepath.Join`'s `Clean()` collapses all three to
  the identical workdir path. Now rejects a leading or repeated `/` (a single trailing `/` is
  fine — it doesn't alias its non-trailing counterpart, since `BuildPath` appends `-<component>`
  directly with no separator) and rejects `\` outright (Windows treats `\`/`/` interchangeably,
  so an unrejected `deploy\test` would alias `deploy/test`'s workdir on that platform — `/` is
  the one documented supported nesting notation, `\` was never a second one).
- **Invalid, skipped:** `pkg/schema/workflow.go`'s `MarshalJSON`/`UnmarshalJSON` missing
  `perf.Track` — adding it creates an import cycle (confirmed via an actual build attempt); no
  other file in `pkg/schema` uses `perf.Track` for the same reason.
- **Doc-only:** corrected a stale `workdir get` → `workdir show` reference, a stale test-name list
  (`_RejectsStackContainingSlash`/`_RejectsStackContainingBackslash` → the current
  dot-segment-only rejection tests) in
  `docs/fixes/2026-08-17-pr2879-coderabbit-round-workdir-and-bootstrap-fixes.md`, and marked
  `docs/fixes/2026-08-06-terraform-output-containment-guard-test-stale-vector.md`'s "stack
  traversal remains a live vector" claim as historical (closed by `validateStackForPath`, added
  in `docs/fixes/2026-08-14-workdir-buildpath-collision-and-stack-traversal.md`).

## Changes

- `cmd/custom_command_integration_test.go`: `cancellationOsExitStub` now panics with a typed
  `cancellationOsExitPanic{}` sentinel and its returned recovery func re-panics anything else.
- `pkg/provisioner/source/source.go`: added `validateTargetIsComponentSubdirectory` (used only by
  `DetermineTargetDirectory`'s default vendoring-target branch) that rejects `target == base`
  after running the existing containment checks — kept separate from `isWithinBase`/
  `validateWithinComponentBasePath` rather than changing their shared contract, since other
  callers/tests (`TestValidateWithinComponentBasePath_RootBase`) intentionally rely on equality
  being permitted there. New tests: component names `"."` and `"child/.."` now return
  `errUtils.ErrPathTraversal`.
- `pkg/provisioner/workdir/fs.go`: `shouldSkipSyncFile`'s three state-filename checks now compare
  `filepath.Clean(relPath)` (root-scoped) instead of `filepath.Base(relPath)`; the
  `terraformLockFileSuffix` check stays basename-based (per-instance lock files legitimately nest
  at any depth). New tests prove a nested `examples/terraform.tfstate` syncs and deletes like an
  ordinary file, while root-level `terraform.tfstate` is still preserved in the same pass.
- `pkg/provisioner/workdir/workdir.go`: added `verifyLegacyWorkdirIdentity(legacyPath, component,
  stack)`, called from `migrateLegacyWorkdir` before `Rename`. Fails closed
  (`errUtils.ErrWorkdirCreation` with a manual-investigation hint) on a `ReadMetadata` error, nil
  metadata (pre-metadata-era legacy workdir), or a component/stack mismatch. New tests cover a
  metadata mismatch and a missing-metadata legacy directory (both mock-level and one end-to-end
  via `ProvisionWorkdir`, asserting the colliding identity's real on-disk state is untouched).
  Existing tests that relied on mocked `Exists`/`Rename` without a real legacy directory
  (`TestMigrateLegacyWorkdir_ReturnsErrorOnRenameFailure`,
  `TestServiceProvision_MigrateLegacyWorkdirRenameFails_DoesNotCreateFreshWorkdir`,
  `TestServiceProvision_MigratesPreExistingLegacyWorkdir`) updated to seed matching metadata.
- `pkg/provisioner/workdir/types.go`: `validateStackForPath` rewritten to split on `strings.Split`
  (preserves empty segments, unlike `FieldsFunc`) and reject any non-trailing empty segment
  (leading/repeated `/`) plus any `\` character outright; removed the now-unused
  `isPathSeparator`. New tests for `"deploy//test"`, `"/deploy/test"`, `"deploy\test"` (all
  rejected) and `"deploy/test/"` (trailing slash, still allowed).

## Validation

- `go build ./...` — clean, after every change and combined.
- `go vet` on every touched package — clean.
- Targeted package tests (`pkg/provisioner/source/...`, `pkg/provisioner/workdir/...`,
  `cmd/terraform/workdir/...`, `pkg/terraform/output/...`, `internal/terraform_backend/...`,
  `cmd`) — all pass, `-count=1` (not relying on cache), including every new regression test.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (fixed one `godot` sentence-start
  finding in a new comment along the way).
- `atmos test` (full short-mode suite) run as part of final verification.

## Follow-ups

None.
