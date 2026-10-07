# Fix: Git hook steps share env, run from the repo root, and validate at install

**Date:** 2026-10-07

## Summary

Git hooks built from `steps:` now behave like custom commands and like Git itself. `type: env` values
reach later steps, hooks run from the repository root, `install` rejects broken hooks, `uninstall`
removes orphan shims, every error names its hook, and the resource-usage line no longer pollutes
`git commit` output.

## Context

A field test of `git.hooks.<name>.steps` found these problems:

- `type: env` values never reached later steps (shell child, Starlark `exec.run` child, templates, declared `env:`). Two causes. The hook's `StepCall` carried no `ProcessEnv`, so after an env step the next child got only the exported keys and no `PATH`. Separately, Viper lowercased the keys of a hook step's `vars:` and `outputs:` mappings, so `SHARED_VAR` became `shared_var`. Custom commands avoid the second problem because their steps are not merged through Viper maps.
- `atmos git hooks run <hook>` used the current directory, so a 40,179-byte `CLAUDE.md` failed from the repo root and passed from `sub/`. Git runs hooks from the root.
- `atmos git hooks uninstall` with no names removed only configured hooks, leaving orphan shims that failed every `git push` with "git hook not configured". `uninstall <unknown>` exited 0.
- `install` wrote shims for every configured name, including broken hooks and names Git never invokes.
- Errors lacked the hook name, and a step timeout surfaced as a bare `context deadline exceeded` instead of the step-timeout error custom commands report.
- The `Total for this invocation` metrics line printed after any hook that spawned a subprocess.
- `run --help` said "command" only, and the `install --help` shim example lost the hook name because `<hook>` was rendered as markup.

## Changes

- `pkg/git/hooks/steps.go`: seed `StepCall.ProcessEnv` from `os.Environ()` merged with `atmosConfig.Env`; resolve the working directory with `atmosgit.GetRoot()` (cwd fallback with a debug log); prefix errors with the hook name; convert a step's own `timeout:` expiry into `ErrStepTimeout` with the step name and duration, leaving caller cancellation untouched.
- `pkg/git/hooks/run.go`: share `validateHookConfig`; run command hooks from the repository root; wrap errors with the hook name.
- `pkg/git/hooks/validate.go` (new): `validateHookConfig`, exported `ValidateHook`, `IsKnownGitHookName`, and an aggregate validator that lists every broken hook.
- `pkg/git/hooks/install.go`: validate all targeted hooks before writing any shim; warn (not fail) for names Git never invokes; report "already installed" for an unchanged shim.
- `pkg/git/hooks/uninstall.go`: with no names, scan the hooks directory for files carrying `ShimMarker`; with names, return `ErrGitHookNotConfigured` unless the name is configured or an installed Atmos shim (so an orphan stays removable by name).
- `pkg/runner/step/automation_sequence.go`: add `AutomationLibrary.ValidateSteps`, which runs the same preflight as `RunSteps` without executing anything.
- `pkg/config/casemap/casemap.go`: generalize the env-only walker to `CollectKeysRecursive(rawYAML, fieldName)`; `CollectEnvKeysRecursive` is now a thin wrapper.
- `pkg/config/load.go`: `mergeRecursiveStepCaseKeys` folds the authored case of every `vars:` and `outputs:` mapping into the `steps.vars` and `steps.outputs` case maps, next to `mergeRecursiveEnvCaseKeys`. These are not hook-specific, since any step list merged through Viper is lowercased.
- `pkg/config/git_hook_steps.go`: `restoreGitHookStepEnv` applies those two case maps (plus env) to hook steps.
- `cmd/git/hooks/run.go`: default `settings.metrics.enabled` to false unless set explicitly; update the help text. `cmd/git/hooks/install.go`, `uninstall.go`, and `hooks.go`: help text for steps, validation, and orphan removal; fix the shim example.
- Tests: `pkg/git/hooks/hook_behavior_test.go`, `pkg/config/git_hook_steps_test.go`, `pkg/config/casemap/casemap_test.go`, `cmd/git/hooks/run_test.go`, `pkg/runner/step/automation_validate_steps_test.go`, and `tests/test-cases/starlark-githooks.yaml`. Bare `require.Error` assertions in `pkg/git/hooks/steps_test.go` and `pkg/config/git_hook_steps_test.go` now assert the real sentinel.

## Validation

- `go test ./pkg/git/hooks/... ./cmd/git/... ./pkg/config/... -count=1` passes.
- `go test ./tests -run 'TestCLICommands/starlark_githooks' -count=1` passes (8 cases).
- Manual run in a throwaway `git init` repo under `$TMPDIR` (never in the worktree): `install pre-commit commit-msg` wrote both shims, a re-run printed "Hook shim already installed", `install` with no names listed all 13 broken fixture hooks and wrote nothing, a 40,179-byte `CLAUDE.md` failed `atmos git hooks run pre-commit` and the shim from `sub/`, and `uninstall` with no names removed both shims plus an orphan `ghost` shim while leaving a user-authored `post-merge`. `git commit` itself could not be exercised because the sandbox denied `git commit`.
- Not run: `golangci-lint` and `custom-gcl` (denied by the sandbox); `go vet` and `gofumpt` are clean.

## Follow-ups

- `pkg/runner/step/automation_library.go` reports a step timeout as a bare `context deadline exceeded` for steps run through `RunSteps`. The hook path now maps it to `ErrStepTimeout`, but converting it inside the automation library would fix every caller.
- The Starlark `env` global holds only the step's declared `env:` mapping, not values set by earlier `type: env` steps. Child processes and templates see them. Whether the global should reflect them belongs with the Starlark context work.
