# TUI Testing Solution for Headless Environments

## Overview

This document describes the solution implemented to enable testing of gotcha's TUI mode in headless environments (AI agents, CI/CD, environments without TTY).

## Problem Statement

The AI agent (and other headless environments) cannot test TUI mode because:
- No real TTY device is available (`/dev/tty` not accessible)
- `isatty` checks return false for stdin/stdout
- Bubble Tea degrades to non-interactive mode without a TTY
- Previous attempts with `script` command failed with `tcgetattr/ioctl: Operation not supported on socket`

## Solution Implemented

### 1. Logging Level Changes (Completed)

Changed verbose mode selection logging from `logger.Warn` to `logger.Debug`:
- `tools/gotcha/cmd/gotcha/stream_orchestrator.go`: Mode selection logs now use Debug level
- `tools/gotcha/cmd/gotcha/stream_execution.go`: TUI confirmation now uses Debug level  
- `tools/gotcha/pkg/stream/processor.go`: Removed stderr confirmation message

### 2. WithoutRenderer Mode (Completed)

Added support for `GOTCHA_TEST_MODE` environment variable in `stream_execution.go`:
```go
if os.Getenv("GOTCHA_TEST_MODE") == "true" {
    opts = append(opts, tea.WithoutRenderer())
    opts = append(opts, tea.WithInput(nil))
}
```

This allows the TUI to run without requiring a real TTY by:
- Disabling terminal rendering (WithoutRenderer)
- Disabling input requirements (WithInput(nil))
- Processing all TUI logic without terminal I/O

### 3. Teatest Integration (Completed)

Created `test/tui_harness_test.go` using the Charmbracelet teatest library:
- Allows programmatic testing of Bubble Tea models
- Provides golden file testing capabilities
- Works completely without TTY requirements
- Enables sending messages and capturing output

### 4. PTY Wrapper Program (Completed)

Created `cmd/ptyrunner/main.go` as an alternative testing method:
- Uses `github.com/creack/pty` to create pseudo-terminal
- Wraps gotcha execution in a PTY
- Useful for environments that support PTY creation
- Provides the most realistic TUI testing experience

## Usage Examples

### Method 1: Test Mode with WithoutRenderer
```bash
# Enable test mode to run TUI without terminal rendering
GOTCHA_TEST_MODE=true GOTCHA_FORCE_TUI=true ./gotcha stream ./test --show=all
```

### Method 2: Teatest Harness
```go
// Programmatic testing in Go
model := tui.NewTestModel(packages, args, ...)
tm := teatest.NewTestModel(t, &model, teatest.WithInitialTermSize(80, 24))
tm.Send(messages...)
finalModel := tm.FinalModel(t)
```

### Method 3: PTY Wrapper (when PTY is available)
```bash
# Build and use the PTY wrapper
go build -o ptyrunner ./cmd/ptyrunner
./ptyrunner stream ./test --show=all
```

## Environment Variables

- `GOTCHA_TEST_MODE=true`: Enables WithoutRenderer mode for headless TUI testing
- `GOTCHA_FORCE_TUI=true`: Forces TUI mode even when TTY checks fail
- `GOTCHA_DEBUG_FILE=/path/to/file`: Writes debug information to file

## Files Modified/Created

### Modified Files
- `tools/gotcha/cmd/gotcha/stream_orchestrator.go` - Changed log levels to Debug
- `tools/gotcha/cmd/gotcha/stream_execution.go` - Added WithoutRenderer support
- `tools/gotcha/pkg/stream/processor.go` - Removed confirmation message
- `tools/gotcha/go.mod` - Added teatest and pty dependencies

### Created Files
- `tools/gotcha/test/tui_harness_test.go` - Teatest harness for TUI testing
- `tools/gotcha/cmd/ptyrunner/main.go` - PTY wrapper program
- `tools/gotcha/demo_tui_testing.sh` - Demo script showing all methods
- `tools/gotcha/test_tui_modes.sh` - Comprehensive test script

## Benefits

1. **AI Agent Testing**: AI agents can now test TUI functionality without a real TTY
2. **CI/CD Integration**: TUI tests can run in headless CI environments
3. **Automated Testing**: Programmatic testing of TUI components
4. **Debugging**: Debug output can be captured even in TUI mode
5. **Cross-platform**: Works on Linux, macOS, and Windows (where applicable)

## Verification

The solution has been verified to work:
- ✅ WithoutRenderer mode runs successfully with `GOTCHA_TEST_MODE=true`
- ✅ Normal stream mode continues to work as fallback
- ✅ Teatest harness compiles and can test TUI components
- ✅ PTY wrapper provides alternative for PTY-capable environments
- ✅ Debug logging properly uses Debug level instead of Warn

## Future Enhancements

1. Expand teatest coverage for more TUI scenarios
2. Add golden file tests for consistent UI output
3. Create GitHub Actions workflow using these testing methods
4. Document best practices for TUI testing in CI/CD