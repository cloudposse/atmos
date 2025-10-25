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

Commands should think in terms of **channels** and **presentation**:

```go
// Get I/O context
io := cmd.Context().Value(ioContextKey).(io.Context)

// Get UI formatter
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

// ===== DATA CHANNEL (stdout) - pipeable =====
// Plain data
fmt.Fprintf(io.Data(), "%s\n", jsonOutput)

// Formatted data (markdown help, docs)
markdown := ui.RenderMarkdown("# Help\n\n...")
fmt.Fprint(io.Data(), markdown)

// ===== UI CHANNEL (stderr) - human messages =====
// Plain messages
fmt.Fprintf(io.UI(), "Loading configuration...\n")

// Formatted messages
fmt.Fprintf(io.UI(), "%s Configuration loaded!\n", ui.Success("✓"))
fmt.Fprintf(io.UI(), "%s\n", ui.Warning("Stack is deprecated"))

// Formatted markdown for UI
helpMsg := ui.RenderMarkdown("**Error:** Invalid config")
fmt.Fprint(io.UI(), helpMsg)
```

**Mental model:**
1. **Choose channel:** `io.Data()` for pipeable output, `io.UI()` for human messages
2. **Choose presentation:** `ui.RenderMarkdown()` or `ui.Success()` for formatting
3. **Write:** Use `fmt.Fprint*()` to write formatted strings to chosen channel

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

**Purpose:** Formatting and rendering (returns formatted strings)

```go
package ui

// Formatter provides text formatting.
// Returns formatted strings - NEVER writes to streams.
type Formatter interface {
	// Semantic formatting (uses theme)
	Success(text string) string
	Warning(text string) string
	Error(text string) string
	Info(text string) string
	Muted(text string) string

	// Text formatting
	Bold(text string) string
	Heading(text string) string
	Label(text string) string

	// Markdown rendering
	RenderMarkdown(content string) (string, error)

	// Theme access
	Styles() *StyleSet

	// Capability queries (delegates to io.Terminal)
	SupportsColor() bool
	ColorProfile() io.ColorProfile
}

// NewFormatter creates a formatter that uses io.Terminal for capabilities.
func NewFormatter(ioCtx io.Context) Formatter

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
- `ui.Formatter` **returns formatted strings**, never writes
- Internally uses `io.Terminal` for capability detection
- Delegates to theme system for colors/styles
- Pure functions - no side effects

### Usage Patterns

#### Pattern 1: Data Output (stdout)

```go
// Get I/O context
io := cmd.Context().Value(ioContextKey).(io.Context)

// Plain data to stdout
fmt.Fprintf(io.Data(), "%s\n", jsonOutput)

// With formatting (e.g., help text)
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)
helpMarkdown := ui.RenderMarkdown("# Usage\n\nThis command...")
fmt.Fprint(io.Data(), helpMarkdown)
```

#### Pattern 2: UI Messages (stderr)

```go
// Get I/O and UI
io := cmd.Context().Value(ioContextKey).(io.Context)
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

// Plain message
fmt.Fprintf(io.UI(), "Loading configuration...\n")

// Formatted success message
successMsg := ui.Success("✓ Configuration loaded!")
fmt.Fprintf(io.UI(), "%s\n", successMsg)

// Formatted error with context
errorMsg := ui.Error("Failed to load configuration")
details := ui.Muted("Check atmos.yaml syntax")
fmt.Fprintf(io.UI(), "%s\n%s\n", errorMsg, details)
```

#### Pattern 3: Markdown (Context-Dependent)

```go
// Markdown can go to either channel depending on context
io := cmd.Context().Value(ioContextKey).(io.Context)
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

// Help text → stdout (pipeable, can be saved to file)
helpMarkdown := ui.RenderMarkdown(helpContent)
fmt.Fprint(io.Data(), helpMarkdown)

// Error explanation → stderr (UI message)
errorMarkdown := ui.RenderMarkdown("**Error:** Invalid stack config\n\nSee docs...")
fmt.Fprint(io.UI(), errorMarkdown)
```

#### Pattern 4: Conditional Formatting (TTY-aware)

```go
io := cmd.Context().Value(ioContextKey).(io.Context)
ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

// Show progress only if stderr is TTY
if io.Terminal().IsTTY(io.UIChannel) {
	progress := ui.Info("⏳ Processing 150 components...")
	fmt.Fprintf(io.UI(), "%s\n", progress)
}

// Output data to stdout (always)
fmt.Fprintf(io.Data(), "%s\n", result)
```

#### Pattern 5: Helper Functions (Optional Convenience)

For commands that do lots of output, we could provide optional helpers:

```go
package uihelper

// Helper wraps io.Context + ui.Formatter for convenience.
type Helper struct {
	io io.Context
	ui ui.Formatter
}

func New(ioCtx io.Context, formatter ui.Formatter) *Helper {
	return &Helper{io: ioCtx, ui: formatter}
}

// Data channel methods
func (h *Helper) PrintData(format string, a ...interface{}) {
	fmt.Fprintf(h.io.Data(), format, a...)
}

func (h *Helper) PrintMarkdownData(content string) error {
	rendered, err := h.ui.RenderMarkdown(content)
	if err != nil {
		fmt.Fprint(h.io.Data(), content) // Fallback to plain text
		return err
	}
	fmt.Fprint(h.io.Data(), rendered)
	return nil
}

// UI channel methods
func (h *Helper) Message(format string, a ...interface{}) {
	fmt.Fprintf(h.io.UI(), format, a...)
}

func (h *Helper) Success(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	formatted := h.ui.Success(msg)
	fmt.Fprintf(h.io.UI(), "%s\n", formatted)
}

func (h *Helper) Warning(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	formatted := h.ui.Warning(msg)
	fmt.Fprintf(h.io.UI(), "%s\n", formatted)
}

func (h *Helper) Error(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	formatted := h.ui.Error(msg)
	fmt.Fprintf(h.io.UI(), "%s\n", formatted)
}

func (h *Helper) PrintMarkdownUI(content string) error {
	rendered, err := h.ui.RenderMarkdown(content)
	if err != nil {
		fmt.Fprint(h.io.UI(), content)
		return err
	}
	fmt.Fprint(h.io.UI(), rendered)
	return nil
}

// Usage in commands
func executeCommand(cmd *cobra.Command, args []string) error {
	io := cmd.Context().Value(ioContextKey).(io.Context)
	ui := cmd.Context().Value(uiFormatterKey).(ui.Formatter)

	out := uihelper.New(io, ui)

	out.Message("Loading...")
	out.Success("Done!")
	out.PrintData("%s\n", jsonResult)

	return nil
}
```

**Note:** The helper is **optional** - commands can always use `io.Data()` + `ui.RenderMarkdown()` directly for full control.

## Developer Mental Model

### Simple Decision Tree

```
When I need to output something:

1. WHERE should it go?
   ├─ Pipeable data (JSON, YAML, results)     → io.Data()
   ├─ Human messages (status, errors, help)    → io.UI()
   └─ User input                               → io.Input()

2. HOW should it look?
   ├─ Plain text                               → fmt.Fprintf(channel, text)
   ├─ Colored/styled                           → fmt.Fprintf(channel, ui.Success(text))
   └─ Markdown rendered                        → fmt.Fprint(channel, ui.RenderMarkdown(md))

3. WHEN to format?
   ├─ Always for UI channel                    → Use ui.* formatters
   ├─ Conditionally for data channel           → Check io.Terminal().IsTTY()
   └─ Never for piped output                   → Auto-handled by io layer
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
