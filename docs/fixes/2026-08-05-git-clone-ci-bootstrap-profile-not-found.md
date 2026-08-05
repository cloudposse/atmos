# Fix: `atmos git clone` CI bootstrap no longer aborts on a missing profile

**Date:** 2026-08-05

## Summary

`atmos git clone` in a fresh CI workspace (no `atmos.yaml` yet, e.g. a config
profile referenced before checkout) failed with `**Error:** profile not
found` before ever attempting the clone, and `ATMOS_CI=true` had no effect
on the failure. The pre-Cobra config-init-error handler in `cmd/root.go` now
recognizes the same no-argument CI bootstrap clone shape the later,
Cobra-aware handler already tolerated, so the process reaches the real
clone logic instead of aborting early.

## Context

`cmd/root.go`'s `Execute()` calls `cfg.InitCliConfig` twice: once before
Cobra resolves any subcommand, and once inside `PersistentPreRun`. Only the
second call's error handling (`applyCIGitCloneBootstrap` /
`gitcmd.CICloneBootstrapRequested`) knew how to tolerate a missing or
invalid `atmos.yaml`/profile for the CI bootstrap clone, which by design
runs in an empty workspace (replacing `actions/checkout`) where no config
can exist yet. The first call's handler
(`handleConfigInitErrorWithArgs`) had no such tolerance, so any non-`cfg.NotFound`
error — including `errUtils.ErrProfileNotFound` — fell through to the
generic "return other errors as-is" branch and aborted `Execute()` before
Cobra, and therefore before `PersistentPreRun`, ever ran. Setting
`ATMOS_CI=true` had no effect because the code path that reads it
(`resolveCICloneMode`, invoked from `CICloneBootstrapRequested`) never
executed.

## Changes

- `cmd/git/bootstrap.go`: added `CIGitCloneModeRequestedFromEnv()`, an
  exported helper that reports whether a detected CI provider plus
  `ATMOS_CI` request CI checkout mode, without needing a resolved Cobra
  command. It defers to the existing `resolveCICloneMode` so the
  Cobra-aware and pre-Cobra code paths can't drift on ATMOS_CI/CI-provider
  precedence.
- `cmd/root.go`: added `isCIGitCloneBootstrapArgs(args)`, an `os.Args`-based
  equivalent of `gitcmd.CICloneBootstrapRequested` (checked before Cobra has
  parsed anything), and wired it into `handleConfigInitErrorWithArgs` as a
  new tolerance branch alongside the existing version/help/config-validation
  branches.

## Validation

- New regression test `TestHandleConfigInitError_CIGitCloneBootstrap`
  (`cmd/root_helpers_test.go`) — confirmed it fails against the pre-fix code
  (`ErrProfileNotFound` returned instead of tolerated) and passes post-fix,
  with negative cases (explicit repo argument, `--all`, no CI provider
  detected) confirming the new branch doesn't over-tolerate.
- `go build ./...`, `go vet ./cmd/...`, `gofmt` — clean.
- `go test ./cmd/... ./cmd/git/...` (targeted: `TestHandleConfigInitError*`,
  `TestApplyCIGitCloneBootstrap*`, `TestIsBuiltinConfigValidationCommand`,
  `TestCICloneBootstrapRequested*`) and full `go test ./cmd/` — all pass.
- `./custom-gcl run` via the repo's pre-commit hook — pass (had to build
  `./custom-gcl` first via `atmos lint custom-gcl`; it wasn't prebuilt in
  this worktree).
- Manual reproduction against a built binary in an empty directory:
  `ATMOS_PROFILE=github ATMOS_CI=true GITHUB_ACTIONS=true GITHUB_REPOSITORY=acme/repo atmos git clone`
  no longer prints `**Error:** profile not found`; it logs the missing
  profile as a warning and proceeds into the real clone logic (which then
  fails only because `acme/repo` doesn't exist — expected for the synthetic
  repo used in this check).

## Follow-ups

None.
