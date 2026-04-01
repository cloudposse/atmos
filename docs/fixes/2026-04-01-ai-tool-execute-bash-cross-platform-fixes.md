# AI Tool: execute_bash Cross-Platform Fixes

**Date:** 2026-04-01
**Severity:** High — nil dereference panic + POSIX-only test suite
**Files changed:**
- `pkg/ai/tools/atmos/execute_bash.go`
- `pkg/ai/tools/atmos/execute_command.go`
- `pkg/ai/tools/atmos/execute_bash_test.go`
- `pkg/ai/tools/atmos/testmain_test.go` (new)

---

## Issues Fixed

### 1. Nil `ProcessState` panic in `ExecuteAtmosCommandTool` (`execute_command.go`)

**Symptom:** Calling `execute_atmos_command` with a binary that fails to start (e.g., binary
not found, invalid working directory) panics at runtime:

```text
panic: runtime error: invalid memory address or nil pointer dereference
goroutine ... [running]:
github.com/cloudposse/atmos/pkg/ai/tools/atmos.(*ExecuteAtmosCommandTool).Execute(...)
    execute_command.go:93
```

**Root Cause:** `cmd.ProcessState` is only populated when the process has started and exited.
When `cmd.CombinedOutput()` returns an error because the binary was not found or could not be
started, `cmd.ProcessState` is `nil`. The original code called `cmd.ProcessState.ExitCode()`
unconditionally:

```go
// Before (panics when ProcessState == nil):
result := &tools.Result{
    Data: map[string]interface{}{
        "exit_code": cmd.ProcessState.ExitCode(), // PANIC
    },
}
if err != nil {
    result.Error = fmt.Errorf("... %d: %w", cmd.ProcessState.ExitCode(), err) // PANIC
}
```

**Fix:** Added nil check; use `-1` as the sentinel exit code for start failures (consistent
with the existing nil-check pattern already present in `execute_bash.go`):

```go
// After (safe):
exitCode := -1
if cmd.ProcessState != nil {
    exitCode = cmd.ProcessState.ExitCode()
}
```

The `-1` sentinel makes start failures distinguishable from process exit code `0` (success)
or any positive exit code from the subprocess itself.

---

### 2. `validateCommand` did not normalize full-path binary names (`execute_bash.go`)

**Symptom:** Invoking a binary by its full path (e.g., `/bin/rm`, `/usr/bin/git`) bypassed
the allowlist and blacklist checks entirely:

```text
/bin/rm -rf /  → passed allowlist (args[0] == "/bin/rm", not "rm")
/sbin/shutdown → passed blacklist check (args[0] != "shutdown")
```

**Root Cause:** `validateCommand` used `args[0]` directly as `baseCommand` without extracting
the basename:

```go
// Before:
baseCommand := args[0]  // "/bin/rm" — does not match "rm" in allowedCommands
```

**Fix:** Applied `filepath.Base` normalization:

```go
// After:
baseCommand := filepath.Base(args[0])  // "/bin/rm" → "rm" — correctly checked
```

All log messages still reference the original `command` string for context so alerts remain
traceable to the raw user input.

---

### 3. POSIX-dependent test suite (`execute_bash_test.go`)

**Symptom:** Eight tests used platform-specific binaries (`echo`, `pwd`, `ls`, `git`) and
hardcoded `/tmp` as the base path. On Windows, these tests fail because the binaries don't
exist and `/tmp` is not a valid path:

```go
// Before (POSIX only):
tool := NewExecuteBashCommandTool(&schema.AtmosConfiguration{BasePath: "/tmp"})
result, _ := tool.Execute(ctx, map[string]interface{}{"command": "echo hello"})
```

**Fix:** Introduced a `TestMain` helper (`testmain_test.go`) that re-uses the compiled test
binary itself as a cross-platform subprocess. The test binary exits immediately when
`ATMOS_BASH_TOOL_TEST_MODE` is set, performing the requested helper operation:

| Mode | Behavior |
|------|----------|
| `echoargs` | Print `argv[1:]` space-joined and exit 0 (cross-platform echo) |
| `pwd` | Print the working directory and exit 0 (cross-platform pwd) |
| `exitone` | Exit with code 1 (simulates a failing command) |

Tests that previously called `echo` or `pwd` now use the test binary via helper functions:

```go
// After (cross-platform):
dir := t.TempDir()
exe := testHelperBin(t)
t.Setenv("ATMOS_BASH_TOOL_TEST_MODE", "echoargs")

tool := NewExecuteBashCommandTool(&schema.AtmosConfiguration{BasePath: dir})
tool.allowedCmds = testHelperAllowed(t)  // allow test binary in the allowlist
result, _ := tool.Execute(ctx, map[string]interface{}{
    "command": quoteCmdArg(exe) + " hello",
})
```

Tests requiring real system binaries (`rm`, `cat`, `git`, `sleep`) retain
`runtime.GOOS == "windows"` skip guards since those operations are inherently
platform-specific.

The `allowedCmds` DI field added to `ExecuteBashCommandTool` (nil by default, falls back to
the package-level `allowedCommands`) lets tests inject the test binary into the allowlist
without mutating global state.

---

## Helper Functions Added (`testmain_test.go`)

| Function | Purpose |
|----------|---------|
| `TestMain` | Handles subprocess helper modes and calls `m.Run()` for normal tests |
| `testHelperBin(t)` | Returns `os.Executable()` path — the test binary itself |
| `testHelperAllowed(t)` | Returns `allowedCmds` map with the test binary's basename |
| `quoteCmdArg(s)` | Single-quotes a string for safe embedding in `shell.Fields` command strings |

---

## Affected Tests (cross-platform conversion)

The following tests were converted from POSIX-only to cross-platform:

- `TestExecuteBashCommandTool_Execute_ValidCommand`
- `TestExecuteBashCommandTool_Execute_SingleQuotedArgs`
- `TestExecuteBashCommandTool_Execute_DoubleQuotedArgs`
- `TestExecuteBashCommandTool_Execute_QuotedOperatorsAreAllowed`
- `TestExecuteBashCommandTool_Execute_WorkingDirectory`
- `TestExecuteBashCommandTool_Execute_CommandFails`
- `TestExecuteBashCommandTool_Execute_ResultData`
- `TestExecuteBashCommandTool_Execute_WorkingDirOutsideBasePathFallsBack`
- `TestExecuteBashCommandTool_Execute_SingleQuotedDollarAllowed`

---

## Verification

```bash
cd /path/to/atmos
go test ./pkg/ai/tools/atmos/ -count=1 -timeout 120s
```

All 60+ tests in the package must pass on Linux, macOS, and Windows.
