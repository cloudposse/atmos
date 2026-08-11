# Fix: `type: shell` steps nested in `type: parallel`/`type: matrix` groups corrupted quoted commands on Windows

**Date:** 2026-08-07

## Summary

`TestCustomCommandIntegration_ParallelStepWithNeeds` failed on the `Acceptance Tests (windows)`
CI job with `"...\cmd.test.exe\" is not recognized as an internal or external command, operable
program or batch file."`. Two layered bugs contributed: a test helper quoted a Windows path with
Go's `%q` (which escapes backslashes), and — after fixing that — the production code path that
executes `type: shell` children of a `type: parallel`/`type: matrix` group on Windows still
mangled any command string containing quote characters, because it relied on `os/exec`'s default
per-argument Windows escaping instead of the verbatim-command-line approach the codebase already
uses elsewhere for the identical problem.

## Context

Custom-command and workflow `type: shell` steps normally run through the in-process `mvdan/sh`
interpreter (`pkg/runner/step/shell.go` → `pkg/utils/shell_utils.go`), which parses POSIX-style
double-quote escaping correctly and cross-platform. But `type: shell` steps nested inside a
`type: parallel`/`type: matrix` group route through `pkg/workflow/control_executor.go`'s
`executeShell`, which — absent a wired `ShellRunner` (the custom-command and workflow control
adapters in `internal/exec/` don't set one) — falls back to `controlShellInvocationForOS`,
spawning the **real** host shell: `cmd.exe /C <command>` on Windows, `sh -c <command>` elsewhere.

That request reaches `pkg/process.DefaultRunner.Run`, which built the subprocess with plain
`exec.CommandContext(ctx, spec.Command, spec.Args...)`. On Windows, Go's `os/exec` applies its own
per-argument escaping when it has no `SysProcAttr.CmdLine` override: since `spec.Args[1]` (the
whole shell command string) contains spaces, Go wraps it in an outer pair of quotes and escapes
any quote characters already inside it as `\"`. `cmd.exe` does not understand `\"` as an escaped
quote — it just toggles in/out of quoted mode on every literal `"` — so a command string that
itself contains a quoted substring (e.g. a quoted executable path) gets its token boundaries
corrupted before `cmd.exe` ever parses it.

`pkg/process/shell_command_windows.go`'s `NewShellCommand` (used by the session-attached path,
`RunShellSession`) already solved this exact problem for TTY/interactive commands, by bypassing
`os/exec`'s Args-based escaping entirely and setting `cmd.SysProcAttr.CmdLine` to a verbatim
`"<shell>" /S /C "<command>"` string — `/S` makes `cmd.exe` strip only the outer quote pair and
treat everything inside literally. `process.DefaultRunner.Run` (the non-interactive path used by
`ControlCommandExecutor.RunCommand`) never got the same treatment.

## Changes

Landed in two commits after the first proved insufficient on real Windows CI:

- `3dec2e991a` — `cmd/custom_command_integration_test.go`: replaced `%q` with a plain,
  unescaped double-quote wrap (`quoteExecutablePath`) when embedding `os.Executable()`'s path in
  six test-helper-built shell command strings. `%q`'s Go-syntax escaping doubled every backslash
  in the Windows path, which `cmd.exe` doesn't collapse back (unlike `mvdan/sh`'s correct
  double-quote parsing on the top-level shell-step path). This alone fixed the backslash-doubling
  but not the deeper quote-corruption bug below (the CI failure persisted, now with correctly
  single-backslashed but still garbled output).
- `906b9d9edc`:
  - `pkg/process/shell_command_windows.go`: added `applyWindowsCmdExeQuoting`, which detects the
    exact `cmd.exe /C <command>` shape `controlShellInvocationForOS` produces and rewrites the
    `*exec.Cmd` to the same verbatim `SysProcAttr.CmdLine` approach `NewShellCommand` already
    uses, instead of `os/exec`'s default per-argument escaping.
  - `pkg/process/shell_command_unix.go`: added a no-op stub (POSIX `exec.Cmd` passes `Args` to
    `execve` verbatim, so this class of bug doesn't exist there).
  - `pkg/process/process.go`: `DefaultRunner.Run` now calls `applyWindowsCmdExeQuoting` right
    after constructing `cmd`.
  - `pkg/process/shell_command_windows_test.go` (new, `//go:build windows`): unit tests covering
    the `cmd.exe /C` rewrite (including a case-insensitive/full-COMSPEC-path variant) and that
    every other `Program`/`Args` shape is left untouched.

## Validation

- `go build ./...` and `GOOS=windows GOARCH=amd64 go build ./...` — both clean.
- `GOOS=windows GOARCH=amd64 go vet ./pkg/process/...` and
  `GOOS=windows GOARCH=amd64 go test -c -o /tmp/process_windows_test.exe ./pkg/process/` — both
  clean (the new Windows-only test file type-checks and compiles; it cannot execute outside
  Windows CI).
- `go test ./cmd/... ./pkg/workflow/... ./pkg/process/...` — all pass on macOS (darwin/arm64),
  including `TestCustomCommandIntegration_ParallelStepWithNeeds` and
  `TestCustomCommandIntegration_MatrixStepResolvesMatrixValues`, which exercise the identical
  `controlShellInvocationForOS` code path via the POSIX `sh -c` branch (a real host shell, not
  `mvdan/sh`), giving real (non-Windows) coverage of the same "real shell, not the interpreter"
  execution path this fix targets.
- `atmos fix lint` (patch-scoped vs `origin/main`) — 0 issues on both commits.
- Not independently reproduced on live Windows (no Windows environment available in this
  session); the diagnosis is based on two consecutive real `Acceptance Tests (windows)` CI
  failure logs (job IDs `93009534804` and `93024199892`), the second of which showed the exact
  predicted symptom (single, not doubled, backslashes still corrupting the same way) after the
  first fix landed, confirming the deeper cause before writing the second fix. Follow-up: confirm
  the next `Acceptance Tests (windows)` CI run on this branch goes green.

## Follow-ups

None — self-verifying on the next Windows CI run.
