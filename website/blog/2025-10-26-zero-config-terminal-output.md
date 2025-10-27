---
title: "Zero-Configuration Terminal Output: Write Once, Works Everywhere"
description: "Atmos now features intelligent terminal output that automatically adapts to any environment - from rich interactive terminals to plain CI logs - without any code changes or capability checking"
slug: zero-config-terminal-output
authors: [osterman]
tags: [feature, enhancement, contributors]
---

# Zero-Configuration Terminal Output: Write Once, Works Everywhere

We're excited to announce a major enhancement to Atmos's terminal output system. Developers can now write code assuming a full-featured terminal, and Atmos automatically handles degradation for all environments - no capability checking, no manual color detection, no secret masking code. Just write clean, simple output code and it works everywhere.

<!--truncate-->

## The Problem with Traditional CLI Output

Most CLI tools force developers to make painful choices:

```go
// Traditional approach - painful!
if isatty.IsTerminal(os.Stdout.Fd()) {
    if supportsColor() {
        if supportsTrueColor() {
            fmt.Println("\x1b[38;2;255;0;0mSuccess!\x1b[0m")
        } else {
            fmt.Println("\x1b[32mSuccess!\x1b[0m")
        }
    } else {
        fmt.Println("Success!")
    }
} else {
    fmt.Println("Success!")  // Plain for pipes
}

// And don't forget to mask secrets!
if containsSecret(output) {
    output = maskSecrets(output)
}
fmt.Println(output)
```

This leads to:
- 🚫 Duplicated capability checking throughout the codebase
- 🚫 Inconsistent output behavior across commands
- 🚫 Secrets accidentally leaked to logs
- 🚫 Broken pipelines when output assumptions change
- 🚫 Difficult testing (mocking TTY detection is painful)

## The Atmos Solution: Write Once, Works Everywhere

With Atmos's new I/O system, developers write code once:

```go
// Atmos approach - simple!
ui.Success("Deployment complete!")
```

That's it. No capability checking, no color detection, no TTY handling. The system automatically:

### 🎨 Color Degradation
- **TrueColor terminal** (iTerm2, Windows Terminal): Full 24-bit colors
- **256-color terminal**: 256-color palette
- **16-color terminal** (basic xterm): ANSI colors
- **No color** (CI, `NO_COLOR=1`, pipes): Plain text

### 📏 Width Adaptation
- **Wide terminal** (120+ cols): Uses full width with proper wrapping
- **Narrow terminal** (80 cols): Wraps at 80 characters
- **Config override**: Respects `atmos.yaml` `settings.terminal.max_width`
- **Unknown width**: Sensible defaults

### 🔍 TTY Detection
- **Interactive terminal**: Full styling, colors, icons, formatting
- **Piped** (`atmos deploy | tee`): Plain text automatically
- **Redirected** (`atmos > file`): Plain text automatically
- **CI environment**: Detects CI and disables interactivity

### 🎭 Markdown Rendering
```go
ui.Markdown("# Deployment Report\n\n**Status:** Success")
```

- **Color terminal**: Styled markdown with colors, bold, headers
- **No-color terminal**: Plain text formatting (notty style)
- **Render failure**: Gracefully falls back to plain content

### 🔒 Automatic Secret Masking

```go
data.WriteJSON(config)  // Contains AWS_SECRET_ACCESS_KEY
```

**Output automatically masked:**
```json
{
  "aws_access_key_id": "AKIAIOSFODNN7EXAMPLE",
  "aws_secret_access_key": "***MASKED***"
}
```

No manual redaction needed. The system automatically detects and masks:
- AWS access keys and secrets (AKIA*, ASIA*)
- Sensitive environment variable patterns
- Common token formats
- JSON/YAML quoted variants

### 🎯 Channel Separation

```go
// Data to stdout (pipeable)
data.WriteJSON(result)

// Messages to stderr (human-readable)
ui.Info("Processing components...")
ui.Success("Deployment complete!")
```

Users can now safely pipe data while seeing status:
```bash
atmos terraform output | jq .vpc_id
# Still sees progress on stderr:
# ℹ Loading configuration...
# ✓ Output retrieved!
```

## Real-World Examples

### Before: Manual Everything

```go
func deploy(cmd *cobra.Command, args []string) error {
    // Capability checking
    isTTY := isatty.IsTerminal(os.Stderr.Fd())
    hasColor := supportsColor()

    // Choose output format
    if isTTY && hasColor {
        fmt.Fprintf(os.Stderr, "\x1b[36m%s\x1b[0m\n", "ℹ Starting deployment...")
    } else {
        fmt.Fprintf(os.Stderr, "Starting deployment...\n")
    }

    // Do deployment
    result, err := performDeploy()
    if err != nil {
        if isTTY && hasColor {
            fmt.Fprintf(os.Stderr, "\x1b[31m%s\x1b[0m\n", "✗ Deployment failed")
        } else {
            fmt.Fprintf(os.Stderr, "Deployment failed\n")
        }
        return err
    }

    // Mask secrets before output
    sanitized := maskSecrets(result)

    // Output data
    json.NewEncoder(os.Stdout).Encode(sanitized)

    if isTTY && hasColor {
        fmt.Fprintf(os.Stderr, "\x1b[32m%s\x1b[0m\n", "✓ Deployment complete!")
    } else {
        fmt.Fprintf(os.Stderr, "Deployment complete!\n")
    }

    return nil
}
```

### After: Clean and Simple

```go
func deploy(cmd *cobra.Command, args []string) error {
    ui.Info("Starting deployment...")

    result, err := performDeploy()
    if err != nil {
        ui.Error("Deployment failed")
        return err
    }

    data.WriteJSON(result)  // Secrets automatically masked
    ui.Success("Deployment complete!")

    return nil
}
```

**60% less code, zero capability checking, automatic secret masking, perfect degradation.**

## Environment Support

The system automatically respects all standard conventions:

### Environment Variables
- `NO_COLOR=1` - Disables all colors
- `CLICOLOR=0` - Disables colors
- `FORCE_COLOR=1` - Forces color even when piped
- `TERM=dumb` - Uses plain text output
- `CI=true` - Detects CI environment
- `ATMOS_FORCE_TTY=true` - Forces TTY mode with sane defaults (for screenshots)
- `ATMOS_FORCE_COLOR=true` - Forces TrueColor even for non-TTY (for screenshots)

### CLI Flags
- `--no-color` - Disables colors
- `--color` - Enables color (only if TTY)
- `--force-color` - Forces TrueColor even for non-TTY (for screenshots)
- `--force-tty` - Forces TTY mode with sane defaults (for screenshots)
- `--redirect-stderr` - Redirects UI to stdout

### Terminal Detection
- TTY/PTY detection via `isatty`
- Color profile via `termenv`
- Width via `ioctl TIOCGWINSZ`
- CI detection via standard env vars

## Testing Benefits

Testing becomes trivial:

```go
func TestDeployCommand(t *testing.T) {
    // Setup test I/O with buffers
    stdout, stderr, cleanup := setupTestUI(t)
    defer cleanup()

    // Run command
    err := deploy(cmd, args)

    // Verify output went to correct channels
    assert.Contains(t, stderr.String(), "Deployment complete")
    assert.Contains(t, stdout.String(), `"status":"success"`)
}
```

No TTY mocking, no color detection stubbing, no complex test fixtures.

## Migration Guide

### Old Pattern
```go
// Old: Manual stream management
io := cmd.Context().Value(ioContextKey).(io.Context)
fmt.Fprintf(io.UI(), "%s\n", "Starting...")
fmt.Fprintf(io.Data(), "%s\n", jsonOutput)
```

### New Pattern
```go
// New: Package-level functions
ui.Writeln("Starting...")
data.Writeln(jsonOutput)
```

### Available Functions

**Data Output (stdout):**
```go
data.Write(text)        // Plain text
data.Writef(fmt, ...)   // Formatted
data.Writeln(text)      // With newline
data.WriteJSON(v)       // JSON
data.WriteYAML(v)       // YAML
```

**UI Output (stderr):**
```go
ui.Write(text)             // Plain (no icon/color)
ui.Writef(fmt, ...)        // Plain formatted
ui.Writeln(text)           // Plain with newline
ui.Success(text)           // ✓ in green
ui.Error(text)             // ✗ in red
ui.Warning(text)           // ⚠ in yellow
ui.Info(text)              // ℹ in cyan
ui.Markdown(content)       // Rendered → stdout
ui.MarkdownMessage(content)// Rendered → stderr
```

## Architecture

The magic happens through clean separation of concerns:

```
Developer Code
    ↓
Package Functions (data.*, ui.*)
    ↓
Formatter (color/style selection)
    ↓
Terminal (capability detection)
    ↓
I/O Layer (masking + routing)
    ↓
stdout/stderr
```

Each layer handles one responsibility:
- **Package functions** - Simple API for developers
- **Formatter** - Returns styled strings (pure, no I/O)
- **Terminal** - Detects capabilities (TTY, color, width)
- **I/O Layer** - Masks secrets, routes to correct stream

## Performance

Zero overhead for capability detection:
- Capabilities detected once at startup
- Results cached for lifetime of command
- No per-call TTY checks
- No per-call color detection

## What's Next

This foundation enables exciting future enhancements:
- **Theme system** (PR #1433) - User-configurable color schemes
- **Progress bars** - Automatic for TTY, plain for pipes
- **Interactive prompts** - Automatic TTY detection
- **Spinner animations** - Show in TTY, silent in CI

## Try It Now

Update to the latest Atmos version and start using the new I/O system:

```go
// Replace manual formatting
- fmt.Fprintf(os.Stderr, "\x1b[32m✓ Done\x1b[0m\n")
+ ui.Success("Done")

// Replace manual JSON output
- json.NewEncoder(os.Stdout).Encode(data)
+ data.WriteJSON(data)

// Replace manual secret masking
- fmt.Println(maskSecrets(output))
+ data.Writeln(output)  // Automatic masking
```

## Feedback

We'd love to hear your feedback on the new I/O system! Join the discussion on [GitHub](https://github.com/cloudposse/atmos/discussions).

---

**Tags:** #feature #enhancement #contributors
