# Fix: Preserve shell script fallback and shell-local PATH lookup

**Date:** 2026-10-17

## Summary

Shell execution again supports executable text files without a shebang and commands found
through a shell-local, unexported `PATH`. Both fixes apply to every edition.

## Context

Comparing v1.232.0 (`26edaff2cc`) with main (`858aa56eec`) exposed two regressions in
the process-group shell middleware introduced by #3320:

- Executable text helpers that the release interpreted as shell scripts failed with
  `exec format error`. The fatal start error also bypassed shell `||` recovery.
- After `unset PATH; PATH="$PWD/bin"`, a bare command found by the shell failed a
  second lookup against the exported child environment.

Both failures reproduced twice in custom commands, workflows, YAML `!exec`, and legacy
Git command hooks, including older edition pins. These are accidental execution failures,
not intentional behavior transitions.

## Changes

- Preserve the shell's resolved executable separately in `TaskSpec.Path`, keeping the
  original command spelling in `argv[0]` and leaving the exported environment unchanged.
- Restore the shell interpreter's Unix `ENOEXEC` fallback with a fresh interpreter,
  exported variables, positional arguments, current directory, and input/output streams.
  Nested external commands still use the process-group runner for cancellation cleanup.
- Preserve shell status 126 for binary input, status 2 for malformed shell syntax, and
  the original start error for failures other than `ENOEXEC`.
- Add subprocess tests for both shell entry points, shell recovery, environment isolation,
  nested fallback and stdin, process descendants on cancellation, and comparisons against
  mvdan's default handler for `$0`, relative paths, directory changes, and `exec`.

## Validation

- Before the fix, both new subprocess regressions failed in both shell entry points.
- `go test ./pkg/process -count=1` passed.
- `go test -race ./pkg/process -run '^TestShellCompatibility' -count=1` passed for
  the initial compatibility, malformed-input, nested-script, and cancellation cases.
- `go test ./pkg/process -run '^TestShellCompatibility' -count=1` passed after adding
  the default-handler comparison and missing-interpreter tests.
- Windows test cross-compilation passed with `GOOS=windows CGO_ENABLED=0 go test -c ./pkg/process`.
- Red/green logs and the original paired release/main evidence are retained under
  `.context/regression-test/2026-10-17-pass2/`.

## Follow-ups

Windows execution remains a CI check; Unix `ENOEXEC` fallback tests skip on Windows.
