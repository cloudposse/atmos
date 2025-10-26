# Output Guide

Simple guide for outputting data and UI messages in Atmos commands.

## Core Concept

**Four layers: Commands, UI/Data, Terminal, and I/O.**

All output flows through `io.Write()` for automatic secret masking:

- **UI messages** (status, errors, warnings) → `ui.Success()` → `terminal.Write()` → `io.Write(UIStream)` → **stderr**
- **Data output** (JSON, YAML, text) → `data.Write()` → `io.Write(DataStream)` → **stdout**
- **Automatic masking** - All secrets registered with `io.Masker()` are automatically masked in ALL output

All use the same I/O context initialized at startup - no setup required in commands.

## Architecture

```mermaid
graph TD
    Commands[Commands Layer]

    Commands -->|ui.Success/Error/etc| UI[UI Package]
    Commands -->|data.Write/JSON/etc| Data[Data Package]

    UI -->|formatted text| Terminal[Terminal]
    Terminal -->|Write| IO[I/O Context]
    Data -->|content| IO

    IO -->|Write UIStream| Masking[Secret Masking]
    IO -->|Write DataStream| Masking

    Masking -->|masked| Stderr[stderr]
    Masking -->|masked| Stdout[stdout]

    Terminal -.capabilities.-> TTY[TTY Detection<br/>Color Profile<br/>Width/Height]

    style IO fill:#e1f5ff
    style Masking fill:#fff3cd
    style Commands fill:#d4edda
```

**Key Flow:**
1. Commands call `ui.Success()` or `data.Write()`
2. UI formats text and writes through `terminal.Write()`
3. Data writes directly through `io.Write(DataStream)`
4. Terminal writes through `io.Write(UIStream)`
5. **I/O layer applies masking** to ALL output
6. Masked output goes to stderr (UI) or stdout (Data)

## UI vs Logging: When to Use What

**Don't confuse UI output with logging** - they serve different purposes:

| Purpose | For | Use | Example |
|---------|-----|-----|---------|
| **UI Output** | End users | `ui.Success()`, `ui.Error()` | "✓ Configuration loaded" |
| **Data Output** | Machines/pipes | `data.Write()`, `data.WriteJSON()` | JSON, YAML, text |
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
data.Write(content)                  // Plain text → stdout
data.Writef("Value: %s\n", value)   // Formatted → stdout
data.WriteJSON(object)               // JSON → stdout
data.WriteYAML(object)               // YAML → stdout
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
    return data.WriteJSON(config)
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

## Secret Masking

**All output is automatically masked** - both UI and Data channels flow through `io.Write()` which applies masking transparently.

### How It Works

```mermaid
graph LR
    UI[ui.Success] -->|formatted text| Term[terminal.Write]
    Data[data.Write] -->|content| IOWrite[io.Write]
    Term -->|content| IOWrite

    IOWrite -->|1. Apply masking| Mask[masker.Mask]
    Mask -->|2. Replace secrets| Masked[***MASKED***]
    Masked -->|3. Write to stream| Output[stdout/stderr]

    style Mask fill:#fff3cd
    style IOWrite fill:#e1f5ff
```

**The flow:**
1. All output goes through `io.Write(stream, content)`
2. I/O layer calls `masker.Mask(content)`
3. Secrets are replaced with `***MASKED***`
4. Masked content written to stdout/stderr

### Registering Secrets

Secrets are registered at the I/O context level and automatically apply to ALL output:

```go
// In command initialization or runtime
ioCtx := getIOContext() // From cobra context

// Register a secret value
ioCtx.Masker().RegisterValue("my-secret-key-123")

// Register with encoding variations (base64, URL, JSON)
ioCtx.Masker().RegisterSecret("password123")

// Register pattern
ioCtx.Masker().RegisterPattern(`SECRET_\w+`)

// AWS access keys get special handling
ioCtx.Masker().RegisterAWSAccessKey("AKIAIOSFODNN7EXAMPLE")
```

**After registration, the secret is masked in ALL output:**

```go
ui.Info("Connecting with key my-secret-key-123")
// Output: "ℹ Connecting with key ***MASKED***"

data.WriteJSON(map[string]string{"password": "password123"})
// Output: {"password": "***MASKED***"}
```

### No Action Required

**For command developers**: Just use `ui.Success()` or `data.Write()`. Masking happens automatically - no special handling needed.

### Advanced Use Cases

For advanced scenarios requiring direct I/O context access (TTY detection, terminal capabilities, custom masking), see the [I/O Handling Strategy PRD](prd/io-handling-strategy.md).

## See Also

- [PRD: I/O Handling Strategy](prd/io-handling-strategy.md) - Architecture and advanced patterns
- [Logging Guidelines](logging.md) - When to use logging vs UI output
- [Developing Atmos Commands](developing-atmos-commands.md) - Command development guide
