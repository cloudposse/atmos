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

## Notes

### Automatic Secret Masking

All output through `ui` package functions is automatically masked for secrets. This happens transparently - no developer action required.

### Advanced Use Cases

For advanced scenarios requiring direct I/O context access (TTY detection, terminal capabilities, custom masking), see the [I/O Handling Strategy PRD](prd/io-handling-strategy.md).

## See Also

- [PRD: I/O Handling Strategy](prd/io-handling-strategy.md) - Architecture and advanced patterns
- [Logging Guidelines](logging.md) - When to use logging vs UI output
- [Developing Atmos Commands](developing-atmos-commands.md) - Command development guide
