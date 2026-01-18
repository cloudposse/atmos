# Custom Markdown Styles

Atmos provides a custom markdown renderer with extended syntax support for enhanced terminal formatting. This document describes the available syntax and when to use each style.

## Overview

The custom markdown renderer wraps glamour's ANSI renderer with goldmark extensions. It supports all standard GitHub Flavored Markdown (GFM) plus custom syntax for:

| Syntax | Name | Rendering | Use Case |
|--------|------|-----------|----------|
| `((text))` | Muted | Dark gray text | Subtle/secondary info like "(already installed)" |
| `~~text~~` | Strikethrough | Dark gray text | Alternative muted syntax (GFM strikethrough restyled) |
| `==text==` | Highlight | Yellow background | Emphasis, warnings |
| `[!BADGE text]` | Badge | Colored bg + bold | Status indicators like EXPERIMENTAL, BETA |
| `> [!NOTE]` | Admonition | Styled block with icon | Tips, warnings, important info |

## Syntax Reference

### Muted Text (`((text))`)

Use double parentheses to render text in dark gray (muted). This is the preferred syntax for muted text.

```go
// Example
ui.Successf("Skipped `%s` ((already installed))", name)
// Renders as: ✓ Skipped `terraform` (already installed)
//                                      ^^^^^^^^^^^^^^^^^ dark gray
```

**When to use:**
- Secondary or supplemental information
- Explanatory parentheticals
- "Skip" reasons or conditions
- Any text that should be visually de-emphasized

### Strikethrough (`~~text~~`)

GFM strikethrough syntax is also rendered as muted gray text. This is an alternative to `((text))`.

```go
// Example
ui.Info("This feature is ~~deprecated~~")
// Renders with "deprecated" in dark gray
```

**Note:** Both `((text))` and `~~text~~` render identically as muted gray text. Choose based on preference or context.

### Highlight (`==text==`)

Use double equals to highlight text with a yellow background.

```go
// Example
ui.Warningf("This feature is ==deprecated== and will be removed in v2.0")
// Renders with yellow background on "deprecated"
```

**When to use:**
- Drawing attention to important terms
- Warning keywords
- Key values that need to stand out

### Badge (`[!BADGE text]` or `[!BADGE:variant text]`)

Create styled badges with colored backgrounds for status indicators.

**Variants:**
- `[!BADGE text]` - Default (purple background)
- `[!BADGE:warning text]` - Orange background
- `[!BADGE:success text]` - Green background
- `[!BADGE:error text]` - Red background
- `[!BADGE:info text]` - Blue background

```go
// Example
ui.Info("[!BADGE EXPERIMENTAL] This feature is experimental")
ui.Info("[!BADGE:success STABLE] This API is production-ready")
// Renders with colored badge before the text
```

**When to use:**
- Feature stability indicators (ALPHA, BETA, STABLE)
- Release status (NEW, DEPRECATED)
- Environment labels (PRODUCTION, STAGING)
- Permission levels (ADMIN, READ-ONLY)

### Admonitions (`> [!TYPE]`)

Create GitHub-style alert blocks for important callouts.

**Types:**
- `> [!NOTE]` - Blue, info icon (ℹ)
- `> [!WARNING]` - Orange, warning icon (⚠)
- `> [!TIP]` - Green, lightbulb icon (💡)
- `> [!IMPORTANT]` - Purple, exclamation icon (❗)
- `> [!CAUTION]` - Red, fire icon (🔥)

```go
// Example - single line
ui.Markdown("> [!NOTE] Configuration changes require restart")

// Example - multi-line
ui.Markdown(`> [!WARNING]
> This operation is irreversible.
> Make sure to backup your data first.`)
```

**When to use:**
- Important notices in help text
- Warnings about dangerous operations
- Tips for better usage
- Critical information that shouldn't be missed

## Standard Markdown

All standard GFM syntax is also supported:

- **Bold**: `**text**`
- *Italic*: `*text*`
- `Inline code`: `` `code` ``
- [Links](url): `[text](url)`
- Headings: `# H1`, `## H2`, etc.
- Lists: `- item` or `1. item`
- Blockquotes: `> text`
- Code blocks: Triple backticks

## Usage in Code

### UI Functions

Use with any `ui.*` function that supports markdown:

```go
import "github.com/cloudposse/atmos/pkg/ui"

// Success with muted text
ui.Successf("Installed `%s@%s` ((already existed))", tool, version)

// Info with badge
ui.Info("[!BADGE:warning BETA] New feature enabled")

// Warning with highlight
ui.Warning("The ==--force== flag will skip confirmation")

// Markdown block with admonition
ui.Markdown(`
# Help

> [!TIP]
> Use --verbose for more details
`)
```

### Direct Renderer Usage

For custom rendering:

```go
import "github.com/cloudposse/atmos/pkg/ui/markdown"

renderer, err := markdown.NewCustomRenderer(
    markdown.WithWordWrap(80),
    markdown.WithColorProfile(termenv.TrueColor),
)
if err != nil {
    return err
}

result, err := renderer.Render("Hello ==world==")
```

## Architecture

The custom renderer is built on:

1. **goldmark** - Markdown parser with extension support
2. **glamour/ansi** - ANSI terminal renderer from Charm
3. **Custom extensions** in `pkg/ui/markdown/extensions/`:
   - `muted.go` - `((text))` syntax
   - `highlight.go` - `==text==` syntax
   - `badge.go` - `[!BADGE text]` syntax
   - `admonition.go` - `> [!TYPE]` syntax

### Extension Priority

Extensions are registered with specific priorities to ensure correct parsing order:

- Muted parser: priority 50 (highest, runs before other inline parsers)
- Badge parser: priority 50 (runs before link parser)
- Highlight parser: priority 500
- Admonition parser: priority 50 (block parser, separate from inline)
- Glamour ANSI renderer: priority 1000 (lowest, fallback)

## Color Degradation

The renderer automatically adapts to terminal capabilities:

- **TrueColor** - Full 24-bit color support
- **ANSI256** - 256-color palette
- **ANSI16** - Basic 16 colors
- **No color** - Plain text fallback

Use `--no-color` flag or set `NO_COLOR=1` to disable colors.

## Testing

Run the markdown tests:

```bash
go test ./pkg/ui/markdown/... -v
```

Test custom extensions:

```bash
go test ./pkg/ui/markdown/extensions/... -v
```
