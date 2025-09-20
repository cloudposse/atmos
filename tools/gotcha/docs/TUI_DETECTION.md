# TUI Detection and Signal Handling

## Overview

Gotcha automatically detects whether it can run in TUI (Terminal User Interface) mode with a progress bar or should fall back to simple streaming output. This document explains how this detection works and how signal handling operates in both modes.

## TUI Detection Logic

Gotcha uses the following logic to determine whether to use TUI mode:

1. **TTY Detection**: Checks if stdout, stdin, and stderr are all connected to a terminal using `isatty`
2. **Format Check**: The format must be "terminal" or "stream" (not "json" or "markdown")
3. **CI Mode Check**: TUI is disabled in CI environments unless forced

### Detection Code
```go
// From pkg/utils/helpers.go
func IsTTY() bool {
    // Check if explicitly disabled
    if config.ForceNoTTY() {
        return false
    }
    
    // Check all three file descriptors
    stdoutTTY := isatty.IsTerminal(os.Stdout.Fd())
    stdinTTY := isatty.IsTerminal(os.Stdin.Fd())
    stderrTTY := isatty.IsTerminal(os.Stderr.Fd())
    
    // All three must be TTY for TUI mode
    return stdoutTTY && stdinTTY && stderrTTY
}
```

## Why TUI Mode May Not Activate

TUI mode will not activate in the following scenarios:

1. **Piped Output**: When output is piped to another command (e.g., `gotcha | grep ...`)
2. **CI Environments**: GitHub Actions, Jenkins, etc. typically don't provide TTY
3. **Non-Terminal Environments**: Docker containers without `-it`, SSH without `-t`
4. **IDE/Editor Terminals**: Some integrated terminals don't provide full TTY support
5. **Conductor Workspaces**: Automated environments may not have TTY available

When TUI is not available, you'll see this message:
```
Terminal format requested but no TTY detected, using non-interactive mode
```

## Signal Handling

### TUI Mode
- Handled by Bubble Tea framework
- Escape key and Ctrl+C immediately abort test execution
- Provides graceful shutdown with summary

### Non-TUI Mode (Streaming)
- Custom signal handler for SIGINT (Ctrl+C) and SIGTERM
- Prints "✗ Test run aborted" message
- Kills test process and all child processes
- Exits with code 130 (standard for SIGINT)

## Environment Variables

### Forcing Modes
- `GOTCHA_FORCE_TUI=true` - Force TUI mode (will fail if no TTY available)
- `GOTCHA_FORCE_NO_TTY=true` - Force non-TUI streaming mode

### Debug Logging
- `GOTCHA_DEBUG_FILE=/path/to/log` - Enable debug logging for TTY detection

Example debug output:
```
[IsTTY] stdout: false, stdin: false, stderr: false, cwd: /current/directory
```

## Troubleshooting

### "Could not open a new TTY" Error
This occurs when forcing TUI mode in an environment without TTY support:
```
error running TUI: could not open a new TTY: open /dev/tty: device not configured
```

**Solution**: Don't use `GOTCHA_FORCE_TUI=true` in non-TTY environments.

### TUI Not Appearing in Terminal
1. Check if you're piping output
2. Try running directly without pipes: `gotcha ./...`
3. Check debug log: `GOTCHA_DEBUG_FILE=/tmp/debug.log gotcha ./...`
4. Verify your terminal supports TTY: `tty` command should show a device

### Different Behavior in Different Directories
This is usually due to:
1. Different `.gotcha.yaml` configurations
2. Different environment variable settings
3. Terminal state differences (background jobs, redirections)

## Best Practices

1. **For Interactive Use**: Run gotcha directly in a terminal without pipes
2. **For CI/Automation**: Use `--format=json` or `--format=markdown` for structured output
3. **For Scripts**: Use `--format=json` and parse the output programmatically
4. **Signal Handling**: Both modes now properly handle Ctrl+C for clean abort

## Technical Details

The TUI detection has been enhanced to:
1. Check all three standard file descriptors (stdout, stdin, stderr)
2. Provide debug logging for troubleshooting
3. Work correctly across Linux, macOS, and Windows
4. Handle signal interruption gracefully in both TUI and non-TUI modes