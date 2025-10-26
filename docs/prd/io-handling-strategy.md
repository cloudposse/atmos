# PRD: Input/Output (I/O) and UI Handling Strategy

**Status**: Draft v2
**Created**: 2025-10-24
**Updated**: 2025-10-25
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

## Problem Statement

Current implementation has conceptual confusion:

### What We Have Now
```go
// Mixed responsibilities in ui.Output
out.Print("data")           // → stdout (data channel)
out.Success("done!")        // → stderr (UI channel) with formatting
out.Markdown("# Doc")       // → stdout (data? UI? both?)
```

**The confusion:**
- Is markdown "data" or "UI"?
- Why does `ui.Output` handle both data and UI?
- What's the relationship between I/O and UI?

### Root Cause
We conflated "where to write" (I/O concern) with "how to format" (UI concern).

**Markdown rendering is UI/presentation**, not I/O. But markdown can be:
- **Data output**: Help text, documentation, API responses → stdout
- **UI output**: Interactive prompts, status messages → stderr

The current design doesn't make this distinction clear.

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

## Proposed Solution

### Simplified Developer Interface

Commands use **package-level functions** that handle both formatting and channel selection:

```go
import (
    "github.com/cloudposse/atmos/pkg/data"
    "github.com/cloudposse/atmos/pkg/ui"
)

// ===== DATA CHANNEL (stdout) - pipeable =====
// Plain data
data.Write("result\n")
data.Writef("Component: %s\n", name)
data.WriteJSON(structData)
data.WriteYAML(structData)

// Formatted data (markdown help, docs)
ui.Markdown("# Help\n\nUsage instructions...")  // → stdout

// ===== UI CHANNEL (stderr) - human messages =====
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
	IsTTY(channel Channel) bool
	ColorProfile() ColorProfile
	Width(channel Channel) int
	Height(channel Channel) int

	// Terminal control
	SetTitle(title string)
	RestoreTitle()
	Alert()

	// Environment detection
	IsCI() bool
	IsPiped(channel Channel) bool
}

// Channel identifies an I/O channel.
type Channel int

const (
	DataChannel  Channel = iota  // stdout
	UIChannel                    // stderr
	InputChannel                 // stdin
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

### Core Interfaces - Presentation Layer

**Purpose:** Formatting and output to UI channel (stderr)

```go
package ui

// ===== Package-level functions (what developers use) =====

// UI channel output (stderr) - formatted messages with icons
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
data.Write("result\n")
data.Writef("Component: %s\n", componentName)

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
ui.Write("Loading configuration...\n")
ui.Writef("Processing %d items...\n", count)

// Formatted messages (with icons and colors)
ui.Success("Configuration loaded!")
ui.Error("Failed to load configuration")
ui.Warning("Stack is deprecated")
ui.Info("Processing 150 components...")
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
   ├─ data.WriteJSON(v)           → JSON to stdout
   ├─ data.WriteYAML(v)           → YAML to stdout
   ├─ ui.Write(text)              → Plain text to stderr (no icon/color)
   ├─ ui.Writef(format, ...)      → Formatted text to stderr (no icon/color)
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
		markdown.WithWidth(f.ioCtx.Terminal().Width(io.DataChannel)),
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
