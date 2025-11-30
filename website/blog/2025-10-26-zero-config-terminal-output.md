---
title: 'Zero-Configuration Terminal Output: Write Once, Works Everywhere'
slug: zero-config-terminal-output
authors:
  - osterman
tags:
  - feature
  - dx
release: v1.198.0
---

Atmos now features intelligent terminal output that adapts to any environment automatically. Developers can write code assuming a full-featured terminal, and Atmos handles the rest - capability detection, color adaptation, and secret masking happen transparently. No more capability checking, manual color detection, or masking code. Just write clean, simple output code and it works everywhere.

<!--truncate-->

## The Problem with Traditional CLI Output

Most CLI tools force developers to make painful choices:

```go
// Traditional approach - painful!
if isatty.IsTerminal(os.Stdout.Fd()) {
    // Using Charm Bracelet's lipgloss for styling
    successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
    fmt.Println(successStyle.Render("Success!"))
} else {
    fmt.Println("Success!")  // Plain for pipes
}

// And don't forget to mask secrets!
if containsSecret(output) {
    output = maskSecrets(output)
}
fmt.Println(output)
```

### What Existing Solutions Don't Solve

While [Charm Bracelet's Lip Gloss](https://github.com/charmbracelet/lipgloss) and similar libraries handle **rendering** beautifully (styled components, layouts, colors), they don't solve critical infrastructure CLI challenges:

- **Secret Masking**: No automatic redaction of sensitive data across all output channels
- **Centralized I/O Control**: Output scattered across stdout/stderr without unified masking
- **Security-First Design**: Secrets can leak through unmasked channels or error messages
- **Atmos-Specific Requirements**: Infrastructure tools handle AWS keys, API tokens, and sensitive configs that must never appear in logs

This leads to:
- 🚫 Duplicated capability checking throughout the codebase
- 🚫 Inconsistent output behavior across commands
- 🚫 **Secrets accidentally leaked to logs** (the primary driver for this work)
- 🚫 Broken pipelines when output assumptions change
- 🚫 Difficult testing (mocking TTY detection is painful)

## The Atmos Solution: Write Once, Works Everywhere

**Atmos's I/O system complements Charm Bracelet** by adding the infrastructure-critical layer that rendering libraries don't provide: centralized I/O control with automatic secret masking. Lip Gloss handles the beautiful rendering; Atmos ensures that rendering never exposes sensitive data.

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

### 📝 Logging vs Terminal Output

**Important distinction:** This I/O system is for **terminal output** (user-facing data and messages), not **logging** (system events and debugging).

- **Terminal Output** (`ui.*`, `data.*`): User-facing messages, status updates, command results
  - Goes to **stdout/stderr**
  - Formatted for humans
  - Respects TTY detection and color settings
  - Automatically masked for secrets

- **Logging** (`log.*`): System events, debugging, internal state
  - Goes to **log files** (or `/dev/stderr` if configured)
  - Machine-readable format
  - Controlled by `--logs-level` flag
  - Not affected by terminal capabilities

Read more in the [CLI Configuration](/cli/configuration) documentation (see `logs` section) and [Global Flags](/cli/global-flags) for `--logs-level` and `--logs-file` options.

## Real-World Examples

### Before: Manual Everything

```go
func deploy(cmd *cobra.Command, args []string) error {
    // Capability checking
    isTTY := isatty.IsTerminal(os.Stderr.Fd())

    // Using Charm Bracelet for styling
    var infoStyle, errorStyle, successStyle lipgloss.Style
    if isTTY {
        infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
        errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
        successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
    }

    // Choose output format
    if isTTY {
        fmt.Fprintf(os.Stderr, "%s\n", infoStyle.Render("ℹ Starting deployment..."))
    } else {
        fmt.Fprintf(os.Stderr, "Starting deployment...\n")
    }

    // Do deployment
    result, err := performDeploy()
    if err != nil {
        if isTTY {
            fmt.Fprintf(os.Stderr, "%s\n", errorStyle.Render("✗ Deployment failed"))
        } else {
            fmt.Fprintf(os.Stderr, "Deployment failed\n")
        }
        return err
    }

    // Mask secrets before output
    sanitized := maskSecrets(result)

    // Output data
    json.NewEncoder(os.Stdout).Encode(sanitized)

    if isTTY {
        fmt.Fprintf(os.Stderr, "%s\n", successStyle.Render("✓ Deployment complete!"))
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

**Result: Dramatically less code, zero capability checking, automatic secret masking, perfect degradation.**

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

### Old Pattern (Atmos main branch before this PR)
```go
// Old: Direct fmt.Fprintf with explicit stream access
fmt.Fprintf(os.Stderr, "Starting...\n")
fmt.Fprintf(os.Stdout, "%s\n", jsonOutput)

// Or with context retrieval
ioCtx, _ := io.NewContext()
fmt.Fprintf(ioCtx.UI(), "Starting...\n")
fmt.Fprintf(ioCtx.Data(), "%s\n", jsonOutput)
```

### New Pattern
```go
// New: Package-level functions with automatic I/O setup
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
- **Progress bars** - Automatic for TTY, plain for pipes
- **Interactive prompts** - Automatic TTY detection
- **Spinner animations** - Show in TTY, silent in CI

## Try It Now

Update to the latest Atmos version and start using the new I/O system:

```go
// Replace manual TTY checking and Lip Gloss styling
- if isatty.IsTerminal(os.Stderr.Fd()) {
-     style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
-     fmt.Fprintf(os.Stderr, "%s\n", style.Render("✓ Done"))
- } else {
-     fmt.Fprintf(os.Stderr, "Done\n")
- }
+ ui.Success("Done")

// Replace manual JSON output
- json.NewEncoder(os.Stdout).Encode(data)
+ data.WriteJSON(data)

// Replace manual secret masking
- fmt.Println(maskSecrets(output))
+ data.Writeln(output)  // Automatic masking
```

## Feedback

We'd love to hear your feedback on the new I/O system! [Open an issue on GitHub](https://github.com/cloudposse/atmos/issues/new) or join the conversation in [Slack](https://slack.cloudposse.com).

---

**Tags:** #feature #enhancement #contributors
