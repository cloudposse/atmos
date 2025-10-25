# UI Output Guide

Simple guide for outputting data and UI messages in Atmos commands.

## Core Concept

**Use `ui` package functions directly - no setup required.**

- **UI messages** (status, errors, warnings) → `ui.Success()`, `ui.Error()`, etc. → **stderr**
- **Data output** (JSON, YAML, results) → `ui.Data()` → **stdout**
- **Help/docs** → `ui.Markdown()` → **stdout**

## UI vs Logging: When to Use What

**Don't confuse UI output with logging** - they serve different purposes:

| Purpose | For | Use | Example |
|---------|-----|-----|---------|
| **UI Output** | End users | `ui.Success()`, `ui.Error()`, `ui.Data()` | "✓ Configuration loaded", JSON output |
| **Logging** | Developers/operators | `log.Debug()`, `log.Error()` | "config.LoadAtmosYAML: parsing file" |

**Key distinction:**
- **UI output is required** - Without it, the user can't use the command
- **Logging is optional metadata** - Adds diagnostic context but user doesn't need it

**Quick test:** "What happens if I disable logging?"
- **Breaks user experience** → It's UI output, use `ui` package
- **User unaffected** → It's logging, use `log` package

**Examples:**

```go
// ❌ WRONG - using logger for user-facing output
log.Info("Configuration loaded successfully")

// ✅ CORRECT - UI for user, logging for diagnostics
ui.Success("Configuration loaded")
log.Debug("config.LoadAtmosYAML: loaded 42 stacks from atmos.yaml")
```

**When to use each:**

- **`ui` package**: Status updates, error messages users need to act on, command results
- **`log` package**: Debug traces, internal state, diagnostic information

See [Logging Guidelines](logging.md) for logging details.

## API Reference

### UI Messages (→ stderr)

```go
ui.Success("Done!")                   // ✓ Done! (green)
ui.Successf("Loaded %d items", n)     // ✓ Loaded 5 items (green)

ui.Error("Failed!")                   // ✗ Failed! (red)
ui.Errorf("Error: %v", err)          // ✗ Error: ... (red)

ui.Warning("Deprecated")              // ⚠ Deprecated (yellow)
ui.Warningf("Slow: %s", duration)    // ⚠ Slow: 2s (yellow)

ui.Info("Processing...")              // ℹ Processing... (cyan)
ui.Infof("Found %d files", count)    // ℹ Found 10 files (cyan)
```

### Data Output (→ stdout)

```go
ui.Data(jsonOutput)                   // Plain text → stdout
ui.Dataf("%s\n", content)            // Formatted → stdout
```

### Markdown (→ stdout)

```go
ui.Markdown("# Help\n\nUsage...")     // Rendered markdown → stdout
```

### Advanced (get string without writing)

```go
text := ui.Format.Success("Done!")    // Returns "✓ Done!" - doesn't write
text := ui.Format.RenderMarkdown(md)  // Returns rendered string - doesn't write
```

## Examples

### Example 1: JSON Output

```go
func ExecuteDescribeComponent(cmd *cobra.Command, args []string) error {
    config := getComponentConfig(args[0])
    jsonOutput, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return err
    }
    return ui.Data(string(jsonOutput) + "\n")
}
```

Pipeable: `atmos describe component vpc | jq .`

### Example 2: Status Messages

```go
func ExecuteTerraformApply(cmd *cobra.Command, args []string) error {
    ui.Info("Planning Terraform changes...")

    err := runTerraformPlan()
    if err != nil {
        ui.Errorf("Terraform plan failed: %v", err)
        return err
    }

    ui.Success("Terraform plan complete")
    return nil
}
```

Messages go to stderr, don't interfere with piped output.

### Example 3: Help/Documentation

```go
func ExecuteAbout(cmd *cobra.Command, args []string) error {
    return ui.Markdown(markdown.AboutMarkdown)
}
```

One line. Renders beautifully, degrades gracefully when piped.

### Pattern 4: Error Messages with Rich Context (stderr)

**Use case:** Detailed error explanations with formatting

```go
func ExecuteValidateStacks(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // Validate stacks
    errors := validateAllStacks()
    if len(errors) > 0 {
        // Error title → stderr
        title := formatter.Error("✗ Stack validation failed")
        fmt.Fprintf(ioCtx.UI(), "%s\n\n", title)

        // Rich error details with markdown → stderr
        for _, err := range errors {
            errorDetail := fmt.Sprintf(`**Stack:** %s
**Issue:** %s

**Location:**
\`\`\`yaml
%s
\`\`\`

**Suggestion:** %s
`,
                err.Stack,
                err.Message,
                err.YAMLSnippet,
                err.Suggestion,
            )

            rendered, _ := formatter.RenderMarkdown(errorDetail)
            fmt.Fprint(ioCtx.UI(), rendered)
            fmt.Fprintln(ioCtx.UI(), "---")
        }

        return fmt.Errorf("validation failed with %d errors", len(errors))
    }

    // Success → stderr
    success := formatter.Success("✓ All stacks validated successfully")
    fmt.Fprintf(ioCtx.UI(), "%s\n", success)

    return nil
}
```

**Why UI channel?**
- ✅ Errors with context for humans
- ✅ Markdown rendering for better readability
- ✅ Doesn't interfere with data output if command also produces results

### Pattern 5: Progress Indicators (conditional)

**Use case:** Show progress only when stderr is a TTY

```go
func ExecuteListComponents(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // Show progress only if UI channel is a TTY
    if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
        // Info() automatically adds "ℹ" icon in cyan
        progress := formatter.Info("Scanning components...")
        fmt.Fprintf(ioCtx.UI(), "%s\n", progress)
    }

    // Get components
    components := scanAllComponents()

    // Output results → stdout (always)
    for _, comp := range components {
        fmt.Fprintf(ioCtx.Data(), "%s\n", comp)
    }

    // Success only if TTY
    if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
        // Successf() automatically adds "✓" icon in green and formats arguments
        success := formatter.Successf("Found %d components", len(components))
        fmt.Fprintf(ioCtx.UI(), "%s\n", success)
    }

    return nil
}
```

**Why conditional?**
- ✅ Interactive terminals see progress
- ✅ Scripts/pipes don't see UI clutter
- ✅ Clean output for automation

### Pattern 6: Mixed Data and UI

**Use case:** Command that outputs data with status messages

```go
func ExecuteDescribeAffected(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // Status → stderr
    status := formatter.Info("Analyzing affected components...")
    fmt.Fprintf(ioCtx.UI(), "%s\n", status)

    // Get affected components
    affected := getAffectedComponents()

    // Warning if many affected → stderr
    if len(affected) > 10 {
        // Warningf() automatically adds "⚠" icon in yellow and formats arguments
        warning := formatter.Warningf("%d components affected", len(affected))
        fmt.Fprintf(ioCtx.UI(), "%s\n", warning)
    }

    // Output data → stdout (pipeable)
    jsonOutput, _ := json.MarshalIndent(affected, "", "  ")
    fmt.Fprintf(ioCtx.Data(), "%s\n", jsonOutput)

    // Summary → stderr
    // Success() automatically adds "✓" icon in green
    summary := formatter.Success("Analysis complete")
    fmt.Fprintf(ioCtx.UI(), "%s\n", summary)

    return nil
}
```

**Result:**
```bash
# Interactive use - see all messages
$ atmos describe affected
Analyzing affected components...
⚠ 15 components affected
[JSON output]
✓ Analysis complete

# Piped use - only data
$ atmos describe affected | jq '.components[]'
[Clean JSON output only]
```

---

## Masking and Security

### Automatic Masking

**Both Data and UI channels automatically mask secrets** when masking is enabled (default).

```go
func ExecuteShowConfig(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)

    // Register secrets for masking
    ioCtx.Masker().RegisterValue(os.Getenv("AWS_SECRET_ACCESS_KEY"))
    ioCtx.Masker().RegisterSecret(dbPassword)
    ioCtx.Masker().RegisterPattern(`token:\s*\S+`)

    // Output config - secrets automatically masked
    fmt.Fprintf(ioCtx.Data(), "AWS_SECRET_ACCESS_KEY=%s\n", secretKey)
    // → Output: AWS_SECRET_ACCESS_KEY=***MASKED***

    return nil
}
```

### Masking Applies To:

- ✅ `ioCtx.Data()` - stdout (masked)
- ✅ `ioCtx.UI()` - stderr (masked)
- ❌ `ioCtx.RawData()` - stdout (unmasked, requires justification)
- ❌ `ioCtx.RawUI()` - stderr (unmasked, requires justification)

### When to Use Raw Channels

**Use `RawData()` or `RawUI()` ONLY when:**
- Binary output (e.g., image files)
- Pre-masked content
- Explicit user request to disable masking (`--disable-masking`)

**Requires explicit justification in code review.**

```go
// BAD - unnecessarily bypasses masking
fmt.Fprintf(ioCtx.RawData(), "%s\n", jsonOutput)

// GOOD - masked by default
fmt.Fprintf(ioCtx.Data(), "%s\n", jsonOutput)

// ACCEPTABLE - binary data with justification
// Justification: Binary PNG data cannot be masked
imageData := generateDiagram()
ioCtx.RawData().Write(imageData)
```

---

## Terminal Capabilities

### Checking TTY

**Use case:** Adapt behavior based on whether output is a terminal

```go
func ExecuteStatus(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    status := getStatus()

    // Interactive terminal - rich formatting
    if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
        msg := formatter.Success("✓ Status: %s", status)
        fmt.Fprintf(ioCtx.UI(), "%s\n", msg)
    } else {
        // Non-interactive - plain text
        fmt.Fprintf(ioCtx.UI(), "Status: %s\n", status)
    }

    return nil
}
```

### Checking Color Support

**Use case:** Use colors only when supported

```go
func ExecuteReport(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    report := generateReport()

    // Format based on color support
    if formatter.SupportsColor() {
        // Use semantic formatting
        title := formatter.Heading("Deployment Report")
        success := formatter.Success("✓ %d succeeded", report.Success)
        error := formatter.Error("✗ %d failed", report.Failed)

        fmt.Fprintf(ioCtx.UI(), "%s\n\n%s\n%s\n", title, success, error)
    } else {
        // Plain text fallback
        fmt.Fprintf(ioCtx.UI(), "Deployment Report\n\n")
        fmt.Fprintf(ioCtx.UI(), "Succeeded: %d\n", report.Success)
        fmt.Fprintf(ioCtx.UI(), "Failed: %d\n", report.Failed)
    }

    return nil
}
```

### Adaptive Width

**Use case:** Format tables based on terminal width

```go
func ExecuteListStacks(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)

    stacks := getAllStacks()

    // Get terminal width
    width := ioCtx.Terminal().Width(iolib.DataChannel)
    if width == 0 {
        width = 80 // Default if can't detect
    }

    // Generate table with adaptive width
    table := generateTable(stacks, width)
    fmt.Fprint(ioCtx.Data(), table)

    return nil
}
```

### Terminal Title

**Use case:** Set terminal title for long-running operations

```go
func ExecuteTerraformApply(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)

    // Set title (automatically checks if supported)
    ioCtx.Terminal().SetTitle("Atmos - Terraform Apply")
    defer ioCtx.Terminal().RestoreTitle()

    // ... run terraform ...

    return nil
}
```

---

## Best Practices

### 1. Always Choose Channel Explicitly

```go
// ❌ BAD - unclear where this goes
fmt.Println("output")

// ✅ GOOD - explicit channel
fmt.Fprintf(ioCtx.Data(), "output\n")  // Pipeable data
fmt.Fprintf(ioCtx.UI(), "output\n")    // UI message
```

### 2. Format Before Writing

```go
// ❌ BAD - formatting mixed with I/O
fmt.Fprintf(ioCtx.UI(), "\033[32m%s\033[0m\n", "Success!")

// ✅ GOOD - format first, write second
msg := formatter.Success("Success!")
fmt.Fprintf(ioCtx.UI(), "%s\n", msg)
```

### 3. Markdown Goes to Either Channel

```go
// ✅ Help text → stdout (pipeable)
helpText, _ := formatter.RenderMarkdown(helpContent)
fmt.Fprint(ioCtx.Data(), helpText)

// ✅ Error details → stderr (UI message)
errorText, _ := formatter.RenderMarkdown(errorDetails)
fmt.Fprint(ioCtx.UI(), errorText)
```

### 4. Respect TTY Detection

```go
// ✅ GOOD - conditional UI elements
if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
    spinner := showSpinner()
    defer spinner.Stop()
}

// ✅ GOOD - always output data
fmt.Fprintf(ioCtx.Data(), "%s\n", result)
```

### 5. Use Semantic Formatting

```go
// ❌ BAD - hardcoded colors
fmt.Fprintf(ioCtx.UI(), "\033[32mSuccess\033[0m\n")

// ✅ GOOD - semantic formatting
msg := formatter.Success("Success")
fmt.Fprintf(ioCtx.UI(), "%s\n", msg)
```

### 6. Register Secrets Early

```go
func ExecuteCommand(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)

    // Register secrets BEFORE any output
    ioCtx.Masker().RegisterSecret(apiKey)
    ioCtx.Masker().RegisterPattern(`password:\s*\S+`)

    // Now output is safe
    fmt.Fprintf(ioCtx.Data(), "config with password: %s\n", password)
    // → Output: config with password: ***MASKED***

    return nil
}
```

### 7. Handle Markdown Errors Gracefully

```go
// ✅ GOOD - fallback to plain text
helpText, err := formatter.RenderMarkdown(content)
if err != nil {
    // Fallback to plain text
    fmt.Fprint(ioCtx.Data(), content)
    return nil
}
fmt.Fprint(ioCtx.Data(), helpText)
```

---

## Examples

### Example 1: Simple Version Command

```go
func ExecuteVersion(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // UI message
    loading := formatter.Info("Loading version information...")
    fmt.Fprintf(ioCtx.UI(), "%s\n", loading)

    // Data output (pipeable)
    version := getVersionString()
    fmt.Fprintf(ioCtx.Data(), "%s\n", version)

    // Success message
    success := formatter.Success("✓ Version displayed")
    fmt.Fprintf(ioCtx.UI(), "%s\n", success)

    return nil
}
```

### Example 2: Complex Validation Command

```go
func ExecuteValidate(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // Status
    status := formatter.Info("Validating stacks...")
    fmt.Fprintf(ioCtx.UI(), "%s\n", status)

    // Run validation
    results := validateAllStacks()

    // Report errors
    if len(results.Errors) > 0 {
        errorTitle := formatter.Error("✗ Validation failed")
        fmt.Fprintf(ioCtx.UI(), "%s\n\n", errorTitle)

        for _, err := range results.Errors {
            errorDetail := fmt.Sprintf("**File:** %s\n**Error:** %s\n", err.File, err.Message)
            rendered, _ := formatter.RenderMarkdown(errorDetail)
            fmt.Fprint(ioCtx.UI(), rendered)
        }

        return fmt.Errorf("validation failed")
    }

    // Success
    success := formatter.Success("✓ All stacks valid")
    fmt.Fprintf(ioCtx.UI(), "%s\n", success)

    return nil
}
```

### Example 3: Data Export with Progress

```go
func ExecuteExport(cmd *cobra.Command, args []string) error {
    ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
    formatter := ui.NewFormatter(ioCtx)

    // Progress (only if TTY)
    if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
        progress := formatter.Info("⏳ Exporting configuration...")
        fmt.Fprintf(ioCtx.UI(), "%s\n", progress)
    }

    // Export data
    config := exportConfiguration()

    // Mask secrets before output
    ioCtx.Masker().RegisterSecret(config.APIKey)

    // Output to stdout (pipeable)
    jsonOutput, _ := json.MarshalIndent(config, "", "  ")
    fmt.Fprintf(ioCtx.Data(), "%s\n", jsonOutput)

    // Success (only if TTY)
    if ioCtx.Terminal().IsTTY(iolib.UIChannel) {
        success := formatter.Success("✓ Configuration exported")
        fmt.Fprintf(ioCtx.UI(), "%s\n", success)
    }

    return nil
}
```

---

## Migration Guide

### Old Pattern (Deprecated)

```go
// OLD - ui.Output interface (being phased out)
out := ui.NewOutput(ioCtx)
out.Print("data")           // Where does this go?
out.Success("done!")        // This goes to stderr, but not obvious
out.Markdown("# Doc")       // Is this data or UI?
```

**Problems:**
- ❌ Unclear where output goes
- ❌ Markdown can be data or UI but interface doesn't show this
- ❌ Mixed responsibilities (I/O + formatting)

### New Pattern (Preferred)

```go
// NEW - explicit channels + formatter
ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
formatter := ui.NewFormatter(ioCtx)

// Data → stdout (explicit)
fmt.Fprintf(ioCtx.Data(), "data\n")

// UI → stderr (explicit)
msg := formatter.Success("done!")
fmt.Fprintf(ioCtx.UI(), "%s\n", msg)

// Markdown - developer chooses channel
helpMarkdown, _ := formatter.RenderMarkdown("# Doc")
fmt.Fprint(ioCtx.Data(), helpMarkdown)  // Help → stdout (pipeable)

errorMarkdown, _ := formatter.RenderMarkdown("**Error:**...")
fmt.Fprint(ioCtx.UI(), errorMarkdown)   // Error → stderr (UI)
```

**Benefits:**
- ✅ Always clear where output goes
- ✅ Markdown is formatting, not I/O
- ✅ Separation of concerns
- ✅ Easy to test

### Migration Steps

1. **Replace `ui.Output` with `io.Context` + `ui.Formatter`**

```go
// Before
out := ui.NewOutput(ioCtx)

// After
ioCtx := cmd.Context().Value(ioContextKey).(iolib.Context)
formatter := ui.NewFormatter(ioCtx)
```

2. **Replace data output methods**

```go
// Before
out.Print("data")
out.Printf("data: %s", value)
out.Println("data")

// After
fmt.Fprintf(ioCtx.Data(), "data")
fmt.Fprintf(ioCtx.Data(), "data: %s", value)
fmt.Fprintf(ioCtx.Data(), "data\n")
```

3. **Replace UI message methods**

```go
// Before
out.Success("done!")
out.Warning("be careful")
out.Error("failed")

// After
msg := formatter.Success("done!")
fmt.Fprintf(ioCtx.UI(), "%s\n", msg)

warn := formatter.Warning("be careful")
fmt.Fprintf(ioCtx.UI(), "%s\n", warn)

err := formatter.Error("failed")
fmt.Fprintf(ioCtx.UI(), "%s\n", err)
```

4. **Replace markdown methods**

```go
// Before
out.Markdown("# Help")      // → stdout
out.MarkdownUI("**Error**") // → stderr

// After
helpText, _ := formatter.RenderMarkdown("# Help")
fmt.Fprint(ioCtx.Data(), helpText)  // Explicit stdout

errorText, _ := formatter.RenderMarkdown("**Error**")
fmt.Fprint(ioCtx.UI(), errorText)   // Explicit stderr
```

### Compatibility

The old `ui.Output` interface is still available but **deprecated**. It will be removed in a future version. All new code should use the new pattern.

---

## See Also

- [PRD: I/O Handling Strategy](prd/io-handling-strategy.md)
- [Developing Atmos Commands](developing-atmos-commands.md)
- [CLAUDE.md](../CLAUDE.md) - Full development guide

---

## Summary

**Three simple rules:**

1. **Choose channel:** `ioCtx.Data()` or `ioCtx.UI()`
2. **Format (optional):** `formatter.Success()` or `formatter.RenderMarkdown()`
3. **Write:** `fmt.Fprintf(channel, formatted)`

**Remember:**
- Both channels automatically mask secrets
- Markdown is formatting (UI layer), not I/O
- Always be explicit about where output goes
- Use semantic formatting (`Success()`, `Warning()`, etc.)
- Respect TTY detection for adaptive behavior
