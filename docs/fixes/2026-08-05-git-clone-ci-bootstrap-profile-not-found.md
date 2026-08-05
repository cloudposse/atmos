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

- `cmd/root.go`: added `isCIGitCloneBootstrapArgs(args)`, which isolates the
  clone-specific arguments from raw `os.Args` (stripping leading root flags
  and the `atmos git clone` tokens) and wires the result into
  `handleConfigInitErrorWithArgs` as a new tolerance branch alongside the
  existing version/help/config-validation branches.
- `cmd/git/bootstrap.go`: added `CIGitCloneBootstrapRequestedFromRawArgs`,
  which parses those clone-specific arguments against a throwaway
  `*cobra.Command` carrying the real clone flag set (a fresh
  `newCloneParser()` instance, never the shared package-level `cloneParser`
  singleton, to avoid disturbing its registered command) via real `pflag`
  parsing, then defers to the existing `CICloneBootstrapRequested`.
- Iteration note: a first version of this fix used a hand-rolled
  `"-"`-prefix heuristic (and a separate `CIGitCloneModeRequestedFromEnv`
  env-only helper) instead of real flag parsing. Dogfooding against the
  exact reported reproduction (`atmos git clone --ci --depth 0`) caught that
  the heuristic misread the space-separated value `0` of `--depth` as a
  positional repo argument and wrongly fell back to the "profile not found"
  error. Replacing the heuristic with real `pflag` parsing (this fix's final
  form) fixes that and, as a side benefit, also lets an explicit
  `--ci`/`--ci=false` in the raw args be honored before Cobra resolves the
  command.

## Validation

- New regression tests: `TestHandleConfigInitError_CIGitCloneBootstrap`
  (`cmd/root_helpers_test.go`, including the exact `--ci --depth 0`
  reproduction) and `TestCIGitCloneBootstrapRequestedFromRawArgs`
  (`cmd/git/bootstrap_test.go`, covering space- and equals-form value flags,
  `--branch`, positional args, `--all`, no-CI-provider, and malformed flag
  values) — confirmed both fail against the pre-fix code and pass post-fix.
- `go build ./...`, `go vet ./cmd/...`, `gofmt` — clean.
- Full `go test ./cmd/ ./cmd/git/...` — all pass.
- `./custom-gcl run` via the repo's pre-commit hook — pass (had to build
  `./custom-gcl` first via `atmos lint custom-gcl`; it wasn't prebuilt in
  this worktree).
- Manual reproduction against built binaries in an empty directory, for both
  `atmos git clone` (bare) and the exact reported
  `atmos git clone --ci --depth 0` with
  `ATMOS_PROFILE=github ATMOS_CI=true GITHUB_ACTIONS=true GITHUB_REPOSITORY=acme/repo`:
  neither prints `**Error:** profile not found`; both log the missing
  profile as a warning and proceed into the real clone logic (which then
  fails only because `acme/repo` doesn't exist — expected for the synthetic
  repo used in this check). Also confirmed the disqualifying cases
  (positional repo argument, `--all`) still correctly show the profile
  error.

## Follow-ups

None.
