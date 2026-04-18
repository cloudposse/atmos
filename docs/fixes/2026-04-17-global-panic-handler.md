# Fix: Global panic handler for friendly crash output

**Date:** 2026-04-17

**Issue:**

- When any code path inside Atmos panics (the ambient nil-credential
  SIGSEGV fixed in
  `docs/fixes/2026-04-17-ambient-identity-nil-credentials.md` is a
  recent example), Go prints a raw stack trace, the process crashes,
  and the user has no idea what happened or how to report it.
- Production users are dumped a wall of `runtime.gopanic` /
  `runtime.mapaccess1` / goroutine dumps that are meaningless to them.
- The same stack trace that is a nightmare for users is exactly what
  we need to debug the bug — it must be preserved, not discarded.

## Status

**Fixed.** Implemented in `pkg/panics/` and wired into `main.run()`. A
deferred `panics.Recover(&exitCode)` at the top of `run()` intercepts
any panic on the main goroutine, prints a friendly headline + body
through `pkg/ui`, writes a full crash report (panic value, stack,
version, OS/arch, Go version, PID, command-line) to
`$TMPDIR/atmos-crash-<timestamp>-<pid>.txt`, routes the panic through
`errUtils.CaptureError` for Sentry, and returns exit code 1. The raw
stack trace is printed inline only when `ATMOS_LOGS_LEVEL=Debug` or
`=Trace` (case-insensitive).

## Goals

1. Catch panics at the main-goroutine entry point and replace the raw
   Go crash output with a short, actionable message rendered through
   `pkg/ui`.
2. Preserve the panic value and full stack trace in a crash-report
   file so a user can attach it to a GitHub issue without needing to
   reproduce the panic.
3. Keep the full stack trace inline when the developer asks for it
   (`ATMOS_LOGS_LEVEL=Debug` or `=Trace`) so local development and CI
   troubleshooting are not regressed.
4. Route the panic through the existing error-reporting pipeline so
   Sentry (when configured) receives a proper event with breadcrumbs.
5. Exit with a non-zero code that matches the existing error-exit
   convention (1) so CI/pre-commit logic is unchanged.

## Non-goals

- **Goroutine-local panics.** `recover()` in `main()` only catches
  panics on the main goroutine. Panics raised on spawned goroutines
  (the signal handler in `main.go`, async telemetry flushes, any
  background work a command starts) still crash the process. Those
  need their own `defer panics.Recover()` at each goroutine entry
  point — out of scope for this fix, tracked as a follow-up.
- **Catch-all "Atmos never crashes"** guarantees. `recover()` cannot
  catch stack overflows, out-of-memory, or external signals like
  SIGSEGV that the Go runtime cannot convert to a panic (e.g. a C
  library segfault via cgo). Those remain hard crashes by design.
- **Changing existing error handling.** Errors returned normally
  through `cmd.Execute()` continue to use
  `errUtils.Format` → `ioLayer.MaskWriter(os.Stderr)` as they do
  today. Only uncaught panics are intercepted.

## Implementation

### Where the handler runs

One `defer panics.Recover(&exitCode)` at the top of `main.run()` in
`main.go`. It is installed **before** `defer cmd.Cleanup()` so Go
unwinds defers in LIFO order: `Cleanup` runs first on normal exit, and
`Recover` catches anything that escapes either the main call chain or
`Cleanup` itself.

The handler's scope is every code path reachable synchronously from
`cmd.Execute()` — all command handlers, the `internal/exec/` pipeline,
`pkg/auth/`, `pkg/stack/`, etc. — plus the early-exit `--version`
path (`cmd.ExecuteVersion`).

### What the handler does

1. Calls `recover()`. If nil (no panic), returns silently.
2. Captures the panic value and `debug.Stack()` immediately.
3. Wraps the panic value into a `cockroachdb/errors.WithStack` error
   and forwards it to `errUtils.CaptureError` so Sentry (when
   configured) gets the crash event through the same pipeline as
   ordinary errors.
4. Emits a structured log entry at `Error` level
   (`"atmos panic recovered"` with `summary`, `version`, `report`
   fields) for observability pipelines.
5. Prints a friendly message via `pkg/ui`:
   - `ui.Error("Atmos crashed unexpectedly")` — the red ✗ headline.
   - `ui.MarkdownMessage(...)` — body with a one-line panic summary,
     version / OS-arch / Go version / command-line / crash-report
     path, a link to the issue tracker, and a hint about
     `ATMOS_LOGS_LEVEL=Debug` for the full stack.
6. Writes a crash report (see below) to `$TMPDIR`.
7. When `ATMOS_LOGS_LEVEL=Debug` or `=Trace` (case-insensitive),
   additionally prints the full stack trace inline after the
   friendly message via `ui.Write`. In that mode the "re-run with
   ATMOS_LOGS_LEVEL=Debug" hint is suppressed to avoid noise.
8. Returns exit code 1. `main.run()` is declared with a named return
   value `(exitCode int)` so `Recover` can populate it via pointer.

### UI choice rationale

Per `CLAUDE.md` § "I/O and UI Usage (MANDATORY)":

- **Headline** — `ui.Error(...)`. The user needs a visually obvious
  red ✗ marker so the crash is not buried in normal output.
- **Body** — `ui.MarkdownMessage(...)`. Renders on the UI channel
  (stderr) so crash output never pollutes the data channel (stdout)
  of a pipeline.
- **Stack trace (debug mode)** — `ui.Write(...)`. Stack is plain text
  that should not compete visually with the headline.
- **Fallback** — if `pkg/ui` is not initialized (panic happened
  before `ui.InitFormatter`), the handler writes plain text directly
  to the stderr writer supplied via `Options.stderr`. `fmt.Fprintf`
  is never called against global streams.

### Package structure

```text
pkg/panics/
├── panics.go          — Recover(), HandlePanic(), crash report writer
├── options.go         — Options struct, defaults, env-var gate
└── panics_test.go     — unit tests (13 cases, no main, no os.Exit)
```

`pkg/panics/` is its own package so any goroutine entry point in the
repo can `defer panics.Recover(...)` without pulling in the larger
`pkg/ui/` graph transitively.

### Handler API

```go
package panics

// Recover is the standard deferred panic handler. Call as the first
// deferred statement in any goroutine whose crash should show the
// friendly message. Populates *exitCode on recovery; left untouched
// when no panic occurred. Passing nil for exitCode is permitted when
// the caller only wants the side effects.
func Recover(exitCode *int)

// HandlePanic is the testable core. Formats the crash message,
// writes the report, fires Sentry, returns the exit code. Does NOT
// call os.Exit — callers own process termination.
func HandlePanic(panicValue any, stack []byte, opts Options) int
```

### Crash report file

Path:
`filepath.Join(os.TempDir(), fmt.Sprintf("atmos-crash-%s-%d.txt", time.Now().UTC().Format("20060102-150405"), os.Getpid()))`

File mode: `0o600` (invoking user only) because a crash report can
contain argv / path values that may leak sensitive data.

`os.TempDir()` is chosen over `$XDG_STATE_HOME` because it is
universally writable, has predictable OS-level cleanup, and keeps the
path easy to paste into a GitHub issue.

## Testing

### Unit tests — `pkg/panics/panics_test.go`

All tests use `Options.useUI = false` with a `bytes.Buffer` stderr
writer so assertions are deterministic and do not depend on
`pkg/ui` initialization.

1. `TestHandlePanic_StringValue` — panic with a string,
   headline + summary in stderr, stack NOT inline, debug hint shown.
2. `TestHandlePanic_ErrorValue` — panic with an `error`, summary is
   the error's `.Error()`.
3. `TestHandlePanic_RuntimeError` — panic with a `runtime.Error`
   (fake), summary matches Go's standard wording.
4. `TestHandlePanic_DebugModeIncludesStackInline` — `showStackInline`
   true, stack appears in stderr; debug hint suppressed.
5. `TestHandlePanic_NonDebugModeHidesStackButWritesReport` — non-debug
   mode hides stack in stderr but crash file contains the stack,
   `Command:`, `os.GOOS`, and Go version.
6. `TestHandlePanic_CrashFileUnwritable` — crash dir points at a path
   under an existing file (so `os.WriteFile` fails). Handler does
   NOT re-panic, friendly message still emitted.
7. `TestHandlePanic_AppliesDefaults` — zero-valued `Options` gets
   defaults filled in (args, now, exitCode).
8. `TestSummarize_FallsBackToGenericFormat` — non-string, non-error
   panic values render via `fmt.Sprintf("%v", ...)`.
9. `TestRecover_NoPanic` — `defer Recover(&code)` with no panic
   leaves the code pointer untouched.
10. `TestRecover_CapturesPanic` — `defer Recover(&code)` around a
    panicking function populates the code and does not re-panic.
11. `TestRecover_NilExitCodePointer` — passing `nil` is tolerated.
12. `TestRecover_UsesRealDebugStack` — sanity-check that
    `debug.Stack()` returns non-empty bytes in the test environment.
13. `TestStackInlineFromEnv` — table-driven coverage of the
    `ATMOS_LOGS_LEVEL` gate: unset, `Debug`/`Trace` (canonical case),
    lowercase/uppercase variants, `Info`/`Warning`/`Error` (false),
    whitespace-padded values.

### Manual verification

Injected `var p *int; _ = *p` into the `version` command's `RunE`,
rebuilt, and ran `./build/atmos version` in both default and
`ATMOS_LOGS_LEVEL=Debug` modes. Output matches the "Sample output"
section below. Injection reverted before committing.

## Sample output

**Default mode** (no `ATMOS_LOGS_LEVEL` or set to `Info`/`Warning`/`Error`):

```text
 ERRO  atmos panic recovered summary="runtime error: invalid memory address or nil pointer dereference" version=1.215.0 report=/var/folders/.../atmos-crash-20260418-041017-56205.txt
✗ Atmos crashed unexpectedly

**Summary:** runtime error: invalid memory address or nil pointer dereference

This is a bug in Atmos. Please report it at:

  https://github.com/cloudposse/atmos/issues

Please include the following when you file the issue:

• **Version:** 1.215.0
• **OS / Arch:** darwin/arm64
• **Built with:** go1.26.0
• **Command:** atmos version
• **Crash report:** /var/folders/.../atmos-crash-20260418-041017-56205.txt

Re-run with ATMOS_LOGS_LEVEL=Debug (or --logs-level=Debug) to see the full stack
trace inline.
```

Exit code: `1`.

**Debug mode** (`ATMOS_LOGS_LEVEL=Debug` or `=Trace`):

Same headline and body, plus the full Go stack trace appended after
the "please include" block, and the "re-run with
`ATMOS_LOGS_LEVEL=Debug`" hint is suppressed:

```text
✗ Atmos crashed unexpectedly

**Summary:** runtime error: invalid memory address or nil pointer dereference

[... same body as default mode ...]

Stack trace:
goroutine 1 [running]:
runtime/debug.Stack()
	/.../runtime/debug/stack.go:26 +0x64
github.com/cloudposse/atmos/pkg/panics.Recover(0x195a231bfe20)
	/.../pkg/panics/panics.go:72 +0x38
panic({0x10e183b00?, 0x10f59b860?})
	/.../runtime/panic.go:860 +0x12c
github.com/cloudposse/atmos/cmd/version.init.func3(0x10f660800, {0x10f895f00, 0x0, 0x0})
	/.../cmd/version/version.go:55 +0x60
[... remainder of stack ...]
main.run()
	/.../main.go:82 +0xb4
main.main()
	/.../main.go:38 +0x110
```

**Crash report file** (`$TMPDIR/atmos-crash-<UTC>-<pid>.txt`, always
written regardless of log level):

```text
Atmos crashed unexpectedly.

Version:    1.215.0
Built with: go1.26.0
OS/Arch:    darwin/arm64
Time:       2026-04-18T04:10:17Z
PID:        56205
Command:    atmos version

Panic: runtime error: invalid memory address or nil pointer dereference

Stack:
goroutine 1 [running]:
runtime/debug.Stack()
	/.../runtime/debug/stack.go:26 +0x64
[... full stack ...]
main.main()
	/.../main.go:38 +0x110
```

## Progress checklist

- [x] Fix doc (this file).
- [x] `pkg/panics/` package with `Recover` + `HandlePanic`
  (`pkg/panics/panics.go`, `pkg/panics/options.go`).
- [x] `defer panics.Recover(&exitCode)` wired into `main.run()`.
- [x] Unit tests for `HandlePanic`: string / error / runtime-error
  values, debug on/off, crash file written, write failure graceful,
  defaults applied on zero-valued `Options`, generic `%v` fallback.
- [x] Unit tests for `Recover`: no panic, panic with recovery, nil
  exit-code pointer, real `debug.Stack()` sanity check.
- [x] Unit tests for the env gate: canonical / lowercase / uppercase
  / whitespace-padded values, non-debug log levels.
- [x] Manual verification with a forced panic. Injected
  `var p *int; _ = *p` into the `version` command's `RunE`, rebuilt,
  and ran `./build/atmos version` in both default and
  `ATMOS_LOGS_LEVEL=Debug` modes. Output matches the "Sample output"
  section. Injection reverted before committing.

## Follow-ups

- **Goroutine-local Recover.** Audit known goroutine entry points
  (the signal-handler goroutine in `main.go`, telemetry flush paths,
  any async auth / stack work) and add `defer panics.Recover(nil)`
  at each. Separate PR.
- **Distinct crash exit code.** CI could benefit from an exit code
  that distinguishes "Atmos said no" (1) from "Atmos crashed" (2).
  Not done here to avoid surprising anyone grepping for `exit 1`;
  `Options.exitCode` is the parameterization point if we change our
  minds.

---

## Related

- `docs/fixes/2026-04-17-ambient-identity-nil-credentials.md` — the
  SIGSEGV bug that motivated this fix. The bug itself is fixed; this
  change ensures any *future* bug of the same shape produces a
  friendly crash message instead of a stack dump.
- `.claude/agents/tui-expert.md` — `pkg/ui` usage rules, output
  channel conventions (stderr vs stdout), and forbidden patterns
  (`fmt.Fprintf(os.Stderr, ...)`).
- `main.go:run()` — the entry point where the deferred handler is
  installed.
- `errors/sentry.go:CaptureError` — the existing Sentry pipeline the
  panic handler reuses.
- `errors/formatter.go:Format` — the error formatter used for
  returned errors; the panic handler routes through the same helper
  after wrapping the panic value in an error.
- `pkg/version/` — source of the version string embedded in the
  crash report.
