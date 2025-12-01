# PRD: Input/Output (I/O) and UI Handling Strategy

**Status**: Adopted
**Created**: 2025-10-24
**Updated**: 2025-10-31
**Owner**: Engineering Team

## Executive Summary

This PRD defines a clear separation between three distinct concerns in Atmos output:

1. **I/O Layer** (`pkg/io/`) - Raw stream access and terminal capabilities (NO formatting)
2. **Presentation Layer** (`pkg/ui/`) - Visual formatting (markdown, colors, styles)
3. **Application Layer** (commands) - Business logic that uses both

The goal is to make it **effortless for developers** to know:
- Where to send data (stdout vs stderr)
- When to use formatting (UI) vs raw output (data)
- How to handle terminal capabilities (color, width, TTY)

## Historical Context

### Problem Solved

The previous implementation had conceptual confusion where the `ui.Output` interface mixed responsibilities:

```go
// OLD: Mixed responsibilities (deprecated)
out.Print("data")           // → stdout (data channel)
out.Success("done!")        // → stderr (UI channel) with formatting
out.Markdown("# Doc")       // → stdout (data? UI? both?)
```

**The confusion:**
- Is markdown "data" or "UI"?
- Why does `ui.Output` handle both data and UI?
- What's the relationship between I/O and UI?

### Root Cause Addressed
The old design conflated "where to write" (I/O concern) with "how to format" (UI concern).

**Markdown rendering is UI/presentation**, not I/O. But markdown can be:
- **Data output**: Help text, documentation, API responses → stdout
- **UI output**: Interactive prompts, status messages → stderr

The current implementation makes this distinction clear through package-level functions.

## Conceptual Model

### Three Orthogonal Concerns

```
┌─────────────────────────────────────────────────────────────────┐
│                       APPLICATION LAYER                          │
│                     (Commands in cmd/*/internal/exec/)           │
│                                                                   │
│  Business logic decides:                                         │
│  - What to output (data, messages, help)                        │
│  - Where to send it (data vs UI channel)                        │
│  - How to present it (plain, formatted, markdown)               │
└────────────┬──────────────────────────────────┬─────────────────┘
             │                                  │
             │ uses for presentation            │ uses for I/O
             ▼                                  ▼
┌─────────────────────────────┐  ┌─────────────────────────────────┐
│   PRESENTATION LAYER        │  │        I/O LAYER                │
│   (pkg/ui/)                 │  │        (pkg/io/)                │
│                             │  │                                 │
│  Formatting & Rendering:    │  │  Stream Management:             │
│  - Markdown rendering       │  │  - stdout (data channel)        │
│  - Color/style application  │  │  - stderr (UI channel)          │
│  - Theme integration        │  │  - stdin (input)                │
│  - Text formatting          │  │                                 │
│                             │  │  Terminal Capabilities:         │
│  Provides:                  │  │  - TTY detection                │
│  - Formatter (colors)       │  │  - Color profile                │
│  - MarkdownRenderer         │  │  - Width/height                 │
│  - StyleSet (from theme)    │  │  - Title/alerts                 │
│                             │  │                                 │
│  NEVER touches streams      │  │  Output Masking:                │
│  Returns formatted strings  │  │  - Automatic redaction          │
└─────────────────────────────┘  │  - Secret patterns              │
                                 │                                 │
                                 │  NEVER does formatting          │
                                 │  Provides primitives only       │
                                 └─────────────────────────────────┘
```

### Key Insight: Separation of Concerns

**I/O Layer answers:** "Where does it go?" (stdout/stderr) and "What can the terminal do?" (color/width)

**Presentation Layer answers:** "How should it look?" (markdown/colors/styles)

**Application Layer answers:** "What do I show?" (data/messages/help)

## Current Implementation

### Simplified Developer Interface

Commands use **package-level functions** that handle both formatting and channel selection:

```go
import (
    "github.com/cloudposse/atmos/pkg/data"
    "github.com/cloudposse/atmos/pkg/ui"
)

// ===== DATA CHANNEL (stdout) - pipeable =====
// Plain data
data.Write("result")
data.Writef("Component: %s", name)
data.Writeln("result")  // Automatic newline
data.WriteJSON(structData)
data.WriteYAML(structData)

// Formatted data (markdown help, docs)
ui.Markdown("# Help\n\nUsage instructions...")  // → stdout

// ===== UI CHANNEL (stderr) - human messages =====
// Plain messages (no icons, no colors)
ui.Write("Loading configuration...")
ui.Writef("Processing %d items...", count)
ui.Writeln("Done")  // Automatic newline

// Formatted messages with automatic icons and colors
ui.Success("Deployment complete!")               // ✓ Deployment complete! → stderr
ui.Error("Configuration failed")                 // ✗ Configuration failed → stderr
ui.Warning("Stack is deprecated")                // ⚠ Stack is deprecated → stderr
ui.Info("Processing components...")              // ℹ Processing components... → stderr

// Formatted markdown for UI
ui.MarkdownMessage("**Error:** Invalid config") // → stderr
```

**Mental model:**
1. **Choose what to output:** Data (JSON/YAML/results) vs messages (status/errors)
2. **Use the right function:** `data.*` for stdout, `ui.*` for stderr
3. **Formatting is automatic:** Functions handle styling, icons, colors, and masking

**Why package-level functions?**
- ✅ Simple, discoverable API (`ui.Success()` vs `fmt.Fprintf(io.UI(), formatter.Success(...))`)
- ✅ Automatic I/O initialization (no context retrieval needed)
- ✅ Enforced by linter (prevents direct `fmt.Fprintf` to streams)
- ✅ Consistent usage across codebase
- ✅ Easy to mock for testing

### Core Interfaces - I/O Layer

**Purpose:** Stream management and terminal capabilities (primitives only)

```go
package io

// Context provides access to I/O primitives.
type Context interface {
	// Stream access
	Data() io.Writer    // stdout - for pipeable data
	UI() io.Writer      // stderr - for human messages
	Input() io.Reader   // stdin - for user input

	// Raw streams (unmasked - requires justification)
	RawData() io.Writer
	RawUI() io.Writer

	// Terminal capabilities
	Terminal() Terminal

	// Configuration
	Config() *Config

	// Output masking
	Masker() Masker
}

// Terminal provides terminal capability detection.
// NO FORMATTING - only capabilities.
type Terminal interface {
	// Capability detection
	IsTTY(stream Stream) bool
	ColorProfile() ColorProfile
	Width(stream Stream) int
	Height(stream Stream) int

	// Terminal control
	SetTitle(title string)
	RestoreTitle()
	Alert()

	// Environment detection
	IsCI() bool
	IsPiped(stream Stream) bool
}

// Stream identifies an I/O stream for writing output.
type Stream int

const (
	DataStream Stream = iota  // stdout - for pipeable data
	UIStream                  // stderr - for human messages
)

// ColorProfile represents terminal color capabilities.
type ColorProfile int

const (
	ColorNone  ColorProfile = iota
	Color16
	Color256
	ColorTrue
)
```

**Key principles:**
- `io.Context` provides **channels** (Data/UI/Input), not Print methods
- `io.Terminal` provides **capabilities** (color/width/TTY), not formatting
- Everything returns primitives (`io.Writer`, `int`, `bool`)
- NO `Print*()`, `Success()`, `Markdown()` methods - those are application concerns

### Output Masking Configuration

The I/O layer provides automatic masking of sensitive data (secrets, credentials, tokens) in all output. Masking can be controlled at multiple levels:

#### Configuration Precedence

Masking configuration follows this precedence (highest to lowest):

1. **`--mask` flag** - Enables/disables masking for current command
2. **`ATMOS_MASK` environment variable** - Global masking control
3. **`settings.terminal.mask.enabled`** in atmos.yaml - Project-wide default
4. **Default** - Masking enabled (true)

#### Command Line Flag

```bash
# Enable masking (default)
atmos terraform apply --mask

# Disable masking temporarily
atmos terraform apply --mask=false

# View raw output (e.g., for debugging)
atmos describe component vpc --mask=false
```

#### Environment Variable

```bash
# Disable masking globally
export ATMOS_MASK=false

# Enable masking globally
export ATMOS_MASK=true
```

#### Configuration File

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      enabled: true                    # Enable/disable masking
      replacement: "***REDACTED***"    # Custom replacement string (optional)
      patterns:                        # Custom regex patterns to mask (optional)
        - 'password=\S+'
        - 'token:\s*\S+'
      literals:                        # Custom literal values to mask (optional)
        - "my-secret-key"
```

**Configuration options:**
- `enabled` (bool): Enable or disable masking (default: true)
- `replacement` (string): Custom replacement string (default: `***MASKED***`)
- `patterns` ([]string): Additional regex patterns to mask
- `literals` ([]string): Additional literal values to mask

#### Per-Call Bypass

For code that needs to bypass masking (e.g., logging, debugging):

```go
// Access unmasked streams directly
rawData := ioCtx.RawData()   // Unmasked stdout
rawUI := ioCtx.RawUI()       // Unmasked stderr

// Use for debugging only - requires justification
fmt.Fprint(rawData, sensitiveData)
```

**When to disable masking:**
- Debugging credential resolution issues
- Viewing raw Terraform state for troubleshooting
- Examining full error messages with credentials
- Development environments where secrets are test values

**Security note:** Always re-enable masking after debugging. Never disable masking in CI/CD or production environments.

### Core Interfaces - Presentation Layer

**Purpose:** Formatting and output to UI channel (stderr)

```go
package ui

// ===== Package-level functions (what developers use) =====

// Toast notifications (stderr) - status messages with custom or themed icons
// This is the primary pattern for all toast-style notifications
func Toast(icon, message string) error           // {icon} {message} → stderr
func Toastf(icon, format string, a ...any) error // {icon} {formatted} → stderr

// Convenience wrappers for common toast types (implemented as Toast calls)
func Success(text string) error              // ✓ {text} in green → stderr
func Successf(format string, a ...any) error // ✓ {formatted} in green → stderr
func Error(text string) error                // ✗ {text} in red → stderr
func Errorf(format string, a ...any) error   // ✗ {formatted} in red → stderr
func Warning(text string) error              // ⚠ {text} in yellow → stderr
func Warningf(format string, a ...any) error // ⚠ {formatted} in yellow → stderr
func Info(text string) error                 // ℹ {text} in cyan → stderr
func Infof(format string, a ...any) error    // ℹ {formatted} in cyan → stderr

// Raw UI output (stderr) - no icons, no automatic styling
func Write(text string) error                // Plain text → stderr
func Writef(format string, a ...any) error   // Formatted text → stderr
func Writeln(text string) error              // Plain text with newline → stderr

// Markdown rendering
func Markdown(content string) error          // Rendered markdown → stdout (data channel)
func Markdownf(format string, a ...any) error
func MarkdownMessage(content string) error   // Rendered markdown → stderr (UI channel)
func MarkdownMessagef(format string, a ...any) error

// Initialization (called by cmd/root.go)
func InitFormatter(ioCtx io.Context)

// ===== Formatter interface (internal) =====

// Formatter provides text formatting.
// Returns formatted strings - NEVER writes to streams.
// Commands should use package-level functions (Success, Error, etc.) instead.
type Formatter interface {
	// Semantic formatting (uses theme) - returns strings
	Success(text string) string
	Warning(text string) string
	Error(text string) string
	Info(text string) string
	Muted(text string) string

	// Text formatting - returns strings
	Bold(text string) string
	Heading(text string) string
	Label(text string) string

	// Markdown rendering - returns string
	Markdown(content string) (string, error)

	// Theme access
	Styles() *StyleSet

	// Capability queries (delegates to io.Terminal)
	SupportsColor() bool
	ColorProfile() terminal.ColorProfile
}

// StyleSet provides lipgloss styles (from theme system).
type StyleSet struct {
	Title   lipgloss.Style
	Heading lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style
	Muted   lipgloss.Style
	// ... more from theme system
}
```

**Key principles:**
- **Developers use package-level functions** (`ui.Success()`, `ui.Error()`, etc.)
- **Formatter interface is internal** - returns formatted strings, never writes
- **Package-level functions handle I/O** - write to appropriate channel (stdout/stderr)
- **Automatic initialization** - `InitFormatter()` called in `cmd/root.go` after flag parsing
- **Enforced by linter** - prevents direct `fmt.Fprintf` to streams

### Usage Patterns

#### Pattern 1: Data Output (stdout)

```go
import (
    "github.com/cloudposse/atmos/pkg/data"
    "github.com/cloudposse/atmos/pkg/ui"
)

// Plain data to stdout
data.Write("result")
data.Writef("Component: %s", componentName)
data.Writeln("result")  // Automatic newline

// Structured data to stdout
data.WriteJSON(result)
data.WriteYAML(config)

// Formatted help text → stdout (pipeable)
ui.Markdown("# Usage\n\nThis command...")
```

#### Pattern 2: UI Messages (stderr)

```go
import "github.com/cloudposse/atmos/pkg/ui"

// Plain messages (no icon, no color)
ui.Write("Loading configuration...")
ui.Writef("Processing %d items...", count)
ui.Writeln("Done")  // Automatic newline

// Toast notifications with custom icons
ui.Toast("📦", "Using latest version: 1.2.3")
ui.Toastf("🔧", "Tool %s is not installed. Installing...", toolName)
ui.Toastf("✓", "Set %s@%s in %s", tool, version, file)

// Multiline toast notifications - automatically indented
ui.Toast("✓", "Installation complete\nVersion: 1.2.3\nLocation: /usr/local/bin")
// Output:
// ✓ Installation complete
//   Version: 1.2.3
//   Location: /usr/local/bin

ui.Toastf("📦", "Installed: %s\nVersion: %s\nSize: %dMB", name, version, size)
// Output:
// 📦 Installed: atmos
//   Version: 1.2.3
//   Size: 42MB

// Toast notifications with themed icons (convenience wrappers)
ui.Success("Configuration loaded!")
ui.Successf("Installed %s/%s@%s", owner, repo, version)
ui.Error("Failed to load configuration")
ui.Errorf("Install failed %s/%s@%s: %v", owner, repo, version, err)
ui.Warning("Stack is deprecated")
ui.Warningf("Slow operation took %s", duration)
ui.Info("Processing 150 components...")
ui.Infof("Found %d matching files", count)
```

#### Pattern 3: Markdown (Context-Dependent)

```go
import "github.com/cloudposse/atmos/pkg/ui"

// Help text → stdout (pipeable, can be saved to file)
ui.Markdown(helpContent)

// Error explanation → stderr (UI message)
ui.MarkdownMessage("**Error:** Invalid stack config\n\nSee docs...")
```

#### Pattern 4: Formatted Variants

```go
import "github.com/cloudposse/atmos/pkg/ui"

// Printf-style formatting
ui.Successf("Deployed %d components successfully", count)
ui.Errorf("Failed to process component %s", name)
ui.Warningf("Stack %s will be deprecated in version %s", stack, version)
ui.Infof("Processing %d/%d components...", current, total)
ui.Markdownf("# Component: %s\n\nStatus: %s", name, status)
```

#### Pattern 5: Mixed Data and UI Output

```go
import (
    "github.com/cloudposse/atmos/pkg/data"
    "github.com/cloudposse/atmos/pkg/ui"
)

func deployComponents(cmd *cobra.Command, args []string) error {
    // UI message to stderr (human-readable status)
    ui.Info("Starting deployment...")

    // Process components
    result, err := processDeployment()
    if err != nil {
        ui.Error("Deployment failed")
        return err
    }

    // Data output to stdout (machine-readable result)
    data.WriteJSON(result)

    // Success message to stderr
    ui.Successf("Deployed %d components", result.Count)

    return nil
}
```

#### Pattern 6: Implementation Details (Not for Commands)

For internal package implementation, the Formatter interface is available:

```go
// Low-level access (internal packages only)
import (
    iolib "github.com/cloudposse/atmos/pkg/io"
    "github.com/cloudposse/atmos/pkg/ui"
)

// Get I/O context if needed for terminal capabilities
ioCtx, err := iolib.NewContext()

// Get formatter instance (returns formatted strings, doesn't write)
formatter, err := ui.GetFormatter()
formatted := formatter.Success("message")  // Returns string, doesn't write

// Write to specific stream (use package functions instead in commands)
fmt.Fprint(ioCtx.UI(), formatted)
```

**Important:** Commands should use package-level functions (`ui.Success()`, `data.Println()`) instead of accessing I/O context or formatter directly.

## Developer Mental Model

### Simple Decision Tree

```
When I need to output something:

1. WHAT am I outputting?
   ├─ Pipeable data (JSON, YAML, results)      → Use data.Write/WriteJSON/WriteYAML()
   ├─ Human messages (status, errors, warnings) → Use ui.Success/Error/Warning/Info()
   ├─ Help/documentation                        → Use ui.Markdown() (stdout)
   └─ Error details with formatting             → Use ui.MarkdownMessage() (stderr)

2. Which package function?
   ├─ data.Write(text)            → Plain text to stdout
   ├─ data.Writef(format, ...)    → Formatted text to stdout
   ├─ data.Writeln(text)          → Plain text with newline to stdout
   ├─ data.WriteJSON(v)           → JSON to stdout
   ├─ data.WriteYAML(v)           → YAML to stdout
   ├─ ui.Write(text)              → Plain text to stderr (no icon/color)
   ├─ ui.Writef(format, ...)      → Formatted text to stderr (no icon/color)
   ├─ ui.Writeln(text)            → Plain text with newline to stderr (no icon/color)
   ├─ ui.Success(text)            → ✓ message in green to stderr
   ├─ ui.Error(text)              → ✗ message in red to stderr
   ├─ ui.Warning(text)            → ⚠ message in yellow to stderr
   ├─ ui.Info(text)               → ℹ message in cyan to stderr
   ├─ ui.Markdown(content)        → Rendered markdown to stdout
   └─ ui.MarkdownMessage(content) → Rendered markdown to stderr

3. Benefits of this approach:
   ├─ Automatic secret masking           → All output is masked
   ├─ Respects user flags                 → --no-color, --redirect-stderr work
   ├─ Testable                            → Mock data.Writer() and ui functions
   ├─ No boilerplate                      → No context retrieval needed
   └─ Enforced by linter                  → Prevents direct fmt.Fprintf usage
```

### Examples by Use Case

```go
// ===== USE CASE: Command help =====
// Help is DATA (can be saved, piped) but uses markdown formatting
helpContent := "# atmos terraform apply\n\nApplies terraform..."
rendered := ui.RenderMarkdown(helpContent)
fmt.Fprint(io.Data(), rendered)

// ===== USE CASE: Processing status =====
// Status is UI (human-readable only) with formatting
status := ui.Info("⏳ Processing 150 components...")
fmt.Fprintf(io.UI(), "%s\n", status)

// ===== USE CASE: Success message =====
// Success is UI with semantic formatting
msg := ui.Success("✓ Deployment complete!")
fmt.Fprintf(io.UI(), "%s\n", msg)

// ===== USE CASE: JSON output =====
// JSON is DATA (no formatting)
fmt.Fprintf(io.Data(), "%s\n", jsonString)

// ===== USE CASE: Error with explanation =====
// Error is UI with markdown for rich explanation
errorTitle := ui.Error("Failed to load stack configuration")
errorDetails := ui.RenderMarkdown("**Reason:** Invalid YAML syntax\n\n```yaml\n...\n```")
fmt.Fprintf(io.UI(), "%s\n\n%s\n", errorTitle, errorDetails)

// ===== USE CASE: Table output =====
// Table can be DATA or UI depending on command context
table := generateTable(data)
if outputToStdout {
	fmt.Fprint(io.Data(), table)  // Pipeable
} else {
	fmt.Fprint(io.UI(), table)    // Human display
}
```

## Comparison: Old vs New

### Old Approach (Current Implementation)

```go
// Mixed responsibilities - unclear where output goes
out := cmd.Context().Value(uiOutputKey).(ui.Output)

out.Print("data")           // Where does this go? stdout? stderr?
out.Success("done!")        // This goes to stderr, but not obvious
out.Markdown("# Doc")       // Is this data or UI? stdout or stderr?
```

**Problems:**
- `ui.Output` mixes I/O (where to write) with UI (how to format)
- Unclear which methods write to stdout vs stderr
- Markdown can be data or UI, but interface doesn't reflect this

### New Approach (Proposed)

```go
// Clear separation - explicit channels
io := cmd.Context().Value(ioContextKey).(io.Context)
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

// Data channel (stdout) - explicit
fmt.Fprintf(io.Data(), "%s\n", "data")

// UI channel (stderr) - explicit
fmt.Fprintf(io.UI(), "%s\n", ui.Success("done!"))

// Markdown - developer chooses channel based on context
helpMarkdown := ui.RenderMarkdown("# Doc")
fmt.Fprint(io.Data(), helpMarkdown)  // Help text → stdout (pipeable)

errorMarkdown := ui.RenderMarkdown("**Error:** ...")
fmt.Fprint(io.UI(), errorMarkdown)   // Error details → stderr (UI)
```

**Benefits:**
- **Explicit channels:** `io.Data()` vs `io.UI()` - always clear where output goes
- **Separation:** I/O (channels) vs UI (formatting) are distinct concepts
- **Flexibility:** Markdown can go to either channel depending on context
- **Simplicity:** Just use `fmt.Fprintf()` - no need to learn new Print methods

## Global Writers (Enhancement)

### Problem: Third-Party Library Integration

The package-level functions (`ui.Success()`, `data.Write()`) work great for direct output in commands. However, when integrating with third-party libraries that expect `io.Writer` interfaces (like loggers, progress bars, TUI frameworks), we need a way to pass file handles that automatically apply masking.

**Use cases:**
- Passing file handles to Charmbracelet logger
- Integrating with progress bar libraries
- Writing to custom file handles (logs, reports)
- Using with any library expecting `io.Writer`

### Solution: Global Writers

Provide package-level `io.Writer` instances that can be passed to any third-party library:

```go
import iolib "github.com/cloudposse/atmos/pkg/io"

// Global writers - available after Initialize()
iolib.Data   // io.Writer for stdout (automatically masked)
iolib.UI     // io.Writer for stderr (automatically masked)

// Usage with third-party libraries
logger := log.New(iolib.UI, "[APP] ", log.LstdFlags)
logger.Printf("Using token: %s", token)  // Automatically masked

// Direct usage
fmt.Fprintf(iolib.Data, `{"result": "success"}`)
fmt.Fprintf(iolib.UI, "Processing...\n")
```

### Implementation

```go
// pkg/io/global.go

var (
    // Data is the global writer for machine-readable output (stdout).
    // All writes are automatically masked based on registered secrets.
    Data io.Writer

    // UI is the global writer for human-readable output (stderr).
    // All writes are automatically masked based on registered secrets.
    UI io.Writer

    globalContext Context
    initOnce      sync.Once
)

// Initialize sets up global writers (called by cmd/root.go).
func Initialize() error {
    initOnce.Do(func() {
        globalContext, initErr = NewContext()
        if initErr != nil {
            Data = os.Stdout
            UI = os.Stderr
            return
        }
        registerCommonSecrets(globalContext.Masker())
        Data = globalContext.Streams().Output()
        UI = globalContext.Streams().Error()
    })
    return initErr
}

// MaskWriter wraps any io.Writer with automatic masking.
// Use this to add masking to custom file handles.
func MaskWriter(w io.Writer) io.Writer {
    if globalContext == nil {
        _ = Initialize()
    }
    if globalContext == nil {
        return w
    }
    return &maskedWriter{
        underlying: w,
        masker:     globalContext.Masker(),
    }
}

// RegisterSecret registers a secret value for masking.
// The secret and its encodings (base64, URL, JSON) will be masked.
func RegisterSecret(secret string) {
    if globalContext == nil {
        _ = Initialize()
    }
    if globalContext != nil {
        globalContext.Masker().RegisterSecret(secret)
    }
}

// RegisterValue registers a literal value for masking (without encodings).
func RegisterValue(value string) {
    if globalContext == nil {
        _ = Initialize()
    }
    if globalContext != nil {
        globalContext.Masker().RegisterValue(value)
    }
}

// RegisterPattern registers a regex pattern for masking.
func RegisterPattern(pattern string) error {
    if globalContext == nil {
        _ = Initialize()
    }
    if globalContext == nil {
        return errors.New("failed to initialize I/O context")
    }
    return globalContext.Masker().RegisterPattern(pattern)
}

// GetContext returns the global I/O context for advanced usage.
func GetContext() Context {
    if globalContext == nil {
        _ = Initialize()
    }
    return globalContext
}
```

### Auto-Registration of Common Secrets

The `Initialize()` function automatically registers common secrets from environment variables:

```go
func registerCommonSecrets(masker Masker) {
    // AWS credentials
    if key := os.Getenv("AWS_ACCESS_KEY_ID"); key != "" {
        masker.RegisterValue(key)
    }
    if secret := os.Getenv("AWS_SECRET_ACCESS_KEY"); secret != "" {
        masker.RegisterSecret(secret)
    }
    if token := os.Getenv("AWS_SESSION_TOKEN"); token != "" {
        masker.RegisterSecret(token)
    }

    // GitHub tokens
    if token := os.Getenv("GITHUB_TOKEN"); token != "" {
        masker.RegisterSecret(token)
    }
    if token := os.Getenv("GH_TOKEN"); token != "" {
        masker.RegisterSecret(token)
    }

    // GitLab tokens
    if token := os.Getenv("GITLAB_TOKEN"); token != "" {
        masker.RegisterSecret(token)
    }

    // Datadog API keys
    if key := os.Getenv("DATADOG_API_KEY"); key != "" {
        masker.RegisterSecret(key)
    }
    if key := os.Getenv("DD_API_KEY"); key != "" {
        masker.RegisterSecret(key)
    }

    // Common secret patterns
    _ = masker.RegisterPattern(`ghp_[A-Za-z0-9]{36}`)             // GitHub PAT
    _ = masker.RegisterPattern(`gho_[A-Za-z0-9]{36}`)             // GitHub OAuth
    _ = masker.RegisterPattern(`Bearer [A-Za-z0-9\-._~+/]+=*`)    // Bearer tokens
}
```

### Usage Patterns

#### Pattern 1: Third-Party Logger Integration

```go
import (
    "log"
    iolib "github.com/cloudposse/atmos/pkg/io"
)

// Initialize once (done in cmd/root.go)
_ = iolib.Initialize()

// Pass UI writer to logger - output goes to stderr, automatically masked
logger := log.New(iolib.UI, "[APP] ", log.LstdFlags)

// Register secret
apiKey := os.Getenv("API_KEY")
iolib.RegisterSecret(apiKey)

// Logger output is automatically masked
logger.Printf("Connecting with key: %s", apiKey)
// Output: [APP] 2025/10/31 10:30:00 Connecting with key: ***MASKED***
```

#### Pattern 2: Custom File Handle with Masking

```go
import iolib "github.com/cloudposse/atmos/pkg/io"

// Create file handle
f, _ := os.Create("output.log")
defer f.Close()

// Wrap with automatic masking
maskedFile := iolib.MaskWriter(f)

// Register secret
dbPassword := "super-secret-password"
iolib.RegisterSecret(dbPassword)

// Writes to file are automatically masked
fmt.Fprintf(maskedFile, "DB Password: %s\n", dbPassword)
// File contains: DB Password: ***MASKED***
```

#### Pattern 3: Simple Direct Usage

```go
import (
    "fmt"
    iolib "github.com/cloudposse/atmos/pkg/io"
)

// Write data to stdout
fmt.Fprintf(iolib.Data, `{"status":"success"}`)

// Write UI message to stderr
fmt.Fprintf(iolib.UI, "Processing...\n")

// Register and mask secret
token := "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
iolib.RegisterSecret(token)

fmt.Fprintf(iolib.Data, "Token: %s\n", token)
// Output: Token: ***MASKED***
```

#### Pattern 4: Pattern-Based Masking

```go
import iolib "github.com/cloudposse/atmos/pkg/io"

// Register pattern for AWS access keys
_ = iolib.RegisterPattern(`AKIA[0-9A-Z]{16}`)

// Any matching pattern is automatically masked
fmt.Fprintf(iolib.Data, "Key: AKIAIOSFODNN7EXAMPLE\n")
// Output: Key: ***MASKED***
```

### Benefits

1. **Simplified Integration**: Pass `iolib.Data` or `iolib.UI` to any library expecting `io.Writer`
2. **Automatic Masking**: All output through global writers is masked automatically
3. **Zero Boilerplate**: No context retrieval, no manual masking setup
4. **Thread-Safe**: `sync.Once` ensures single initialization, `sync.RWMutex` in masker
5. **Auto-Discovery**: Common secrets automatically registered from environment
6. **Flexible**: Can wrap custom file handles with `MaskWriter()`

### Design Decisions

**Q: Why global variables instead of dependency injection?**

A: For third-party library integration, we need simple `io.Writer` instances that can be passed anywhere. Global writers provide the logging-style simplicity needed for this use case, while the existing `io.Context` interface remains available for more controlled usage.

**Q: How does this relate to package-level functions (`ui.Success()`, `data.Write()`)?**

A: These complement each other:
- **Package-level functions**: Best for direct output in commands (simpler API)
- **Global writers**: Best for third-party library integration (standard `io.Writer`)

Commands should prefer package-level functions. Use global writers when you need to pass file handles to libraries.

**Q: Is this safe for concurrent access?**

A: Yes. Initialization uses `sync.Once` to ensure it happens exactly once. The underlying masker uses `sync.RWMutex` for thread-safe secret registration and masking operations.

## Implementation Strategy

### Phase 1: I/O Layer - Rename Methods for Clarity

```go
// Current (confusing)
type Streams interface {
	Output() io.Writer  // Is this for data or UI?
	Error() io.Writer   // Error stream or error messages?
}

// Proposed (clear)
type Context interface {
	Data() io.Writer    // stdout - pipeable data
	UI() io.Writer      // stderr - human messages
	Input() io.Reader   // stdin - user input
}
```

### Phase 2: Presentation Layer - Pure Functions

```go
// Current (mixed concerns - writes to streams)
type Output interface {
	Print(a ...interface{})           // Writes to stdout
	Success(format string, a ...interface{})  // Writes to stderr
	Markdown(content string)          // Writes to stdout
}

// Proposed (pure formatting - returns strings)
type Formatter interface {
	Success(text string) string       // Returns formatted string
	RenderMarkdown(content string) (string, error)  // Returns rendered string
}

// Commands write using fmt.Fprintf:
fmt.Fprintf(io.UI(), "%s\n", ui.Success("done!"))
fmt.Fprint(io.Data(), ui.RenderMarkdown(help))
```

### Phase 3: Migration - Backward Compatibility

Provide compatibility layer during migration:

```go
// pkg/ui/compat.go - Temporary compatibility layer

type LegacyOutput struct {
	io io.Context
	ui Formatter
}

func NewLegacyOutput(ioCtx io.Context) *LegacyOutput {
	return &LegacyOutput{
		io: ioCtx,
		ui: NewFormatter(ioCtx),
	}
}

// Old interface methods - delegate to new pattern
func (o *LegacyOutput) Print(a ...interface{}) {
	fmt.Fprint(o.io.Data(), fmt.Sprint(a...))
}

func (o *LegacyOutput) Success(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	formatted := o.ui.Success(msg)
	fmt.Fprintf(o.io.UI(), "%s\n", formatted)
}

func (o *LegacyOutput) Markdown(content string) {
	rendered, _ := o.ui.RenderMarkdown(content)
	fmt.Fprint(o.io.Data(), rendered)
}
```

## Open Questions & Decisions

### Q1: Should we provide convenience helpers like `uihelper.Helper`?

**Option A:** Minimal - developers always use `fmt.Fprintf(io.Data(), ...)` directly
- ✅ Simple, explicit, no magic
- ❌ More verbose, repetitive

**Option B:** Provide `uihelper.Helper` with convenience methods
- ✅ Less boilerplate, easier for common cases
- ❌ Another abstraction to learn

**Recommendation:** Start with Option A (minimal). Add Option B only if developers request it.

### Q2: How to handle trailing whitespace trimming?

**Current PRD:** `ui.Output.SetTrimTrailingWhitespace(bool)`

**Problem:** This is a stream concern (I/O), not a formatter concern (UI).

**Proposed:** Move to I/O layer as a writer wrapper:

```go
// pkg/io/trimming_writer.go
type trimmingWriter struct {
	underlying io.Writer
}

func (tw *trimmingWriter) Write(p []byte) (n int, err error) {
	trimmed := trimTrailingSpaces(string(p))
	return tw.underlying.Write([]byte(trimmed))
}

// In io.Context creation
func NewContext(opts ...Option) Context {
	stdout := os.Stdout
	if cfg.TrimTrailingWhitespace {
		stdout = &trimmingWriter{underlying: stdout}
	}
	// ...
}
```

**Benefit:** Configuration happens once at I/O layer, transparent to all code.

### Q3: What about the existing `pkg/ui/markdown/` package?

**Current:** Separate markdown package with its own renderer

**Proposed:** Keep it, but have `ui.Formatter.RenderMarkdown()` delegate to it

```go
func (f *formatter) RenderMarkdown(content string) (string, error) {
	// Delegate to existing markdown package
	renderer, err := markdown.NewRenderer(
		f.ioCtx.Config().AtmosConfig,
		markdown.WithWidth(f.ioCtx.Terminal().Width(terminal.Stdout)),
		markdown.WithColorProfile(f.ioCtx.Terminal().ColorProfile()),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(content)
}
```

**Benefit:** Existing markdown package stays, but accessible via `ui.Formatter`.

## Success Criteria

### Developer Experience
- ✅ Developers can explain the difference between `io.Data()` and `io.UI()` in one sentence
- ✅ No confusion about where markdown output goes (developer decides explicitly)
- ✅ 100% of new code uses `io.Data()` / `io.UI()` pattern
- ✅ Clear examples in `CLAUDE.md` for all common output scenarios

### Code Quality
- ✅ I/O layer has ZERO formatting logic (only returns `io.Writer`, capabilities)
- ✅ UI layer has ZERO direct stream access (only returns formatted strings)
- ✅ Commands always explicitly choose channel (data vs UI)
- ✅ Lint rules prevent mixing concerns

### Testability
- ✅ All I/O is mockable (via `io.Context` interface)
- ✅ All formatting is testable (pure functions returning strings)
- ✅ 90%+ test coverage for both `pkg/io/` and `pkg/ui/`

## Next Steps

1. **Review this PRD** with team for conceptual clarity
2. **Update implementation** to match new mental model:
   - Rename `Streams().Output()` → `Data()`
   - Rename `Streams().Error()` → `UI()`
   - Make `ui.Formatter` return strings instead of writing
3. **Update `CLAUDE.md`** with new decision tree and examples
4. **Migrate one command** as proof-of-concept
5. **Gather feedback** before rolling out broadly

## References

- [NO_COLOR Standard](https://no-color.org/)
- [CLICOLOR Conventions](https://bixense.com/clicolors/)
- [Charmbracelet termenv](https://github.com/charmbracelet/termenv)
- [Charmbracelet glamour](https://github.com/charmbracelet/glamour)
- PR #1433: Theme System
