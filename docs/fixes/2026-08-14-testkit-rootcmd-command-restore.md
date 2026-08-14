# Fix: `cmd.NewTestKit(t)` now restores `RootCmd.Commands()` between tests

**Date:** 2026-08-14

## Summary

`cmd.NewTestKit(t)`'s cleanup restored flags, args, and other `RootCmd`
state, but never restored `RootCmd.Commands()`. Any `cmd` test that loaded
real custom commands via `InitCliConfig` + `processCustomCommands(atmosConfig,
atmosConfig.Commands, RootCmd)` left those commands registered on the shared
global `RootCmd` for whichever test ran next, contradicting CLAUDE.md's claim
that `NewTestKit` "Auto-cleans RootCmd state (flags, args)."

## Context

Flagged by a CodeRabbit review comment on PR #2879 against
`docs/fixes/2026-08-05-custom-command-container-with-block-dropped.md`,
which documents a resulting collision: `TestCustomCommandContainerBuildPassesWithBlockToDocker`
(`cmd/custom_command_container_build_test.go`) registers this repo's real
`.atmos.d/build.yaml` "build" custom command onto `RootCmd` via
`InitCliConfig` + `processCustomCommands`, and that registration survived
past the test because `restoreRootCmdState` never removed it. That doc
worked around one resulting failure by renaming the colliding test's own
custom command away from `build`, and explicitly called the underlying
pollution "pre-existing and out of scope" rather than fixing it.

CodeRabbit's comment mis-identified `TestProcessCustomCommands`
(`cmd/cmd_utils_test.go`) as the polluter. Verified against current code:
that test registers its commands onto a locally-scoped
`parentCmd := &cobra.Command{...}`, never onto the package-global `RootCmd`,
so it does not cause this pollution. The actual mechanism is any test that
registers real custom commands onto `RootCmd` itself — several exist in the
`cmd` package (`custom_command_container_build_test.go`,
`custom_command_collision_test.go`, `custom_command_dependency_test.go`,
`custom_command_integration_test.go`, `custom_command_control_test.go`,
`custom_command_flag_conflict_test.go`, and others).

`cobra.Command.RemoveCommand` was already used ad hoc in a few places
(`cmd/custom_command_collision_warning_test.go`, `cmd/root_helpers_test.go`,
`cmd/root_test.go`) as a per-test manual cleanup pattern, but nothing
generalized it into the shared `NewTestKit` cleanup path that CLAUDE.md
mandates for all `cmd` tests.

## Changes

- `cmd/testing_helpers_test.go`: added a `commands []*cobra.Command` field to
  `cmdStateSnapshot`, populated in `snapshotRootCmdState` via
  `append([]*cobra.Command(nil), RootCmd.Commands()...)`. Added a new
  `restoreRootCmdCommands` helper, called from `restoreRootCmdState`, that
  removes (via `RootCmd.RemoveCommand`) any command present on `RootCmd` now
  that wasn't present in the snapshot — restoring `RootCmd`'s command set to
  what it was when the test started, regardless of which test(s) registered
  commands in between.
- `docs/fixes/2026-08-05-custom-command-container-with-block-dropped.md`:
  removed the "pre-existing and out of scope" claim and corrected the vague
  "another test in the cmd package" attribution to name the actual general
  mechanism (any test registering real custom commands onto the shared
  `RootCmd` via `processCustomCommands`), now that this fix closes it at the
  source instead of only avoiding one specific collision.

## Validation

- `go build ./...` — clean.
- `gofumpt -l cmd/testing_helpers_test.go` — clean.
- `go test ./cmd/... -short` (all `cmd` subpackages, not just the changed
  one) — all pass, including the top-level `cmd` package (~106s, covers the
  custom-command tests this change targets).
- `go test ./pkg/schema/... ./pkg/container/... ./pkg/config/adapters/...` —
  pass (unrelated packages touched by other fixes landed in the same pass).
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues in any file
  changed by this fix. (6 pre-existing issues were reported in
  `pkg/auth/cloud/kube/config.go`, a file untouched by this change.)
- Did not add a dedicated regression test asserting that two independent
  tests each registering a command onto `RootCmd` no longer see each other's
  commands — the existing `cmd` package test suite already exercises this
  exact scenario in practice (multiple tests registering real custom
  commands via `processCustomCommands` in the same `go test` run), and it
  passed cleanly with this change in place.

## Follow-ups

None.
