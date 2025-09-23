# Atmos Go Code Style Guide

This style guide provides guidelines for developers working on the Atmos Go codebase on how to use the theming and styling system consistently across all terminal output.

## Overview

Atmos uses a comprehensive theming system built on `github.com/charmbracelet/lipgloss` to provide consistent, accessible, and customizable terminal output. This guide explains when and how to use each style for different types of output in the Atmos codebase.

## Semantic Style Usage

### Status Styles

These styles convey the status or outcome of operations:

#### Success Style
Use for successful operations, confirmations, and positive outcomes.

```go
styles := theme.GetCurrentStyles()
fmt.Println(styles.Success.Render("✓ Component deployed successfully"))
```

**When to use:**
- Operation completed successfully
- Validation passed
- Positive confirmation messages
- Success badges (e.g., "★ Recommended")

#### Error Style
Use for errors, failures, and critical problems.

```go
fmt.Println(styles.Error.Render("✗ Failed to deploy component"))
```

**When to use:**
- Operation failed
- Validation errors
- Critical errors that stop execution
- Error indicators

#### Warning Style
Use for warnings, cautions, and non-critical issues.

```go
fmt.Println(styles.Warning.Render("⚠ Configuration deprecated"))
```

**When to use:**
- Deprecation notices
- Non-critical validation issues
- Warnings that don't stop execution
- Cautionary messages

#### Info Style
Use for informational messages, tips, and neutral status.

```go
fmt.Println(styles.Info.Render("ℹ Using default configuration"))
```

**When to use:**
- Informational messages
- Tips and hints
- Status updates
- Neutral information

### Text Styles

#### Title Style
Use for main titles and primary headings.

```go
fmt.Println(styles.Title.Render("Atmos Configuration"))
```

**When to use:**
- Main command output titles
- Primary section headers
- Top-level headings

#### Heading Style
Use for section headings and sub-titles.

```go
fmt.Println(styles.Heading.Render("COMPONENT SETTINGS"))
```

**When to use:**
- Section headers
- Sub-titles
- Group headers

#### Label Style
Use for field labels, section labels, and non-status headers.

```go
fmt.Println(styles.Label.Render("Configuration File:"))
fmt.Println("atmos.yaml")
```

**When to use:**
- Field labels (e.g., "Type:", "Source:", "Version:")
- Section labels (e.g., "Status Messages:", "Command Examples:")
- Any label that precedes a value
- Headers that aren't status-related

**Important:** Do NOT use Success, Error, or other status styles for labels. Use the Label style for semantic correctness.

#### Body Style
Use for regular body text.

```go
fmt.Println(styles.Body.Render("This is regular paragraph text"))
```

**When to use:**
- Regular paragraph text
- Descriptions
- General content

#### Muted Style
Use for de-emphasized or secondary text.

```go
fmt.Println(styles.Muted.Render("(optional)"))
```

**When to use:**
- Secondary information
- Optional indicators
- Less important details
- Timestamps

### UI Element Styles

#### Command Style
Use for CLI commands and executable names.

```go
fmt.Println(styles.Command.Render("atmos terraform plan"))
```

**When to use:**
- CLI commands
- Executable names
- Command examples

#### Description Style
Use for descriptions of commands, flags, or features.

```go
fmt.Println(styles.Description.Render("Plan terraform changes"))
```

**When to use:**
- Command descriptions
- Flag descriptions
- Feature explanations

#### Link Style
Use for URLs and links.

```go
fmt.Println(styles.Link.Render("https://atmos.tools"))
```

**When to use:**
- URLs
- Documentation links
- External references

#### Selected Style
Use for selected items in interactive lists.

```go
fmt.Println(styles.Selected.Render("> vpc-component"))
```

**When to use:**
- Selected menu items
- Active selections
- Highlighted choices

### Special Styles

#### Footer Style
Use for footer text and additional notes.

```go
fmt.Println(styles.Footer.Render("Run 'atmos help' for more information"))
```

**When to use:**
- Footer messages
- Additional notes
- Help text at the bottom

#### Code Style (Command)
Use for inline code, file names, and paths.

```go
fmt.Println(styles.Command.Render("atmos.yaml"))
```

**When to use:**
- File names
- Configuration keys
- Inline code snippets
- Paths

## Output Conventions

### Standard Output vs Error Output

#### Use stdout for:
- Data and results meant for piping
- Command output that might be processed by other tools
- JSON/YAML output

```go
fmt.Println(componentData) // Data goes to stdout
```

#### Use stderr for:
- UI messages and prompts
- Progress indicators
- Status messages
- Error messages

```go
fmt.Fprintf(os.Stderr, "Processing component...\n")
```

### Consistency Guidelines

#### Choose the appropriate output stream
Use the correct stream based on the type of content:

```go
// For data/results (stdout):
fmt.Println(componentData)

// For UI/status messages (stderr):
fmt.Fprintf(os.Stderr, styles.Info.Render("Processing component...\n"))

// Avoid mixing output methods unnecessarily:
// GOOD: Consistent approach
fmt.Fprintf(os.Stderr, styles.Label.Render("Component:"))
fmt.Fprintf(os.Stderr, " vpc\n")

// AVOID: Mixed output methods
fmt.Fprintln(os.Stdout, styles.Label.Render("Component:"))
u.PrintMessage("vpc")
```
## Code Examples

### Getting and Using Styles

```go
package cmd

import (
    "fmt"
    "github.com/cloudposse/atmos/pkg/ui/theme"
)

func displayComponentInfo(name, stack string) {
    // Get current theme styles
    styles := theme.GetCurrentStyles()
    if styles == nil {
        // Fallback to plain text if theme not available
        fmt.Printf("Component: %s\n", name)
        fmt.Printf("Stack: %s\n", stack)
        return
    }

    // Use theme-aware styles
    fmt.Println(styles.Title.Render("Component Information"))
    fmt.Println()

    fmt.Print(styles.Label.Render("Name:"))
    fmt.Printf(" %s\n", name)

    fmt.Print(styles.Label.Render("Stack:"))
    fmt.Printf(" %s\n", stack)
}
```

### Formatting Status Messages

```go
func reportOperationStatus(success bool, message string) {
    styles := theme.GetCurrentStyles()
    if styles == nil {
        fmt.Println(message)
        return
    }

    if success {
        fmt.Println(styles.Success.Render("✓ " + message))
    } else {
        fmt.Println(styles.Error.Render("✗ " + message))
    }
}
```

### Creating Consistent Section Output

```go
func displaySection(title string, items []string) {
    styles := theme.GetCurrentStyles()
    if styles == nil {
        fmt.Printf("%s:\n", title)
        for _, item := range items {
            fmt.Printf("  - %s\n", item)
        }
        return
    }

    // Use Label style for section headers
    fmt.Println(styles.Label.Render(title + ":"))
    for _, item := range items {
        fmt.Printf("  • %s\n", item)
    }
}
```

## Common Patterns

### Pattern: Label-Value Pairs

```go
// CORRECT: Use Label style for the label
fmt.Print(styles.Label.Render("Version:"))
fmt.Printf(" %s\n", version)

// WRONG: Don't use Success style for labels
fmt.Print(styles.Success.Render("Version:"))
fmt.Printf(" %s\n", version)
```

### Pattern: Section Headers

```go
// CORRECT: Use Label style for section headers
fmt.Println(styles.Label.Render("Configuration Options:"))

// WRONG: Don't use plain text for headers
fmt.Println("Configuration Options:")
```

### Pattern: Status Indicators

```go
// CORRECT: Use appropriate status style
if isRecommended {
    fmt.Println(styles.Success.Render("★ Recommended"))
}

// WRONG: Don't use Label style for status
if isRecommended {
    fmt.Println(styles.Label.Render("★ Recommended"))
}
```

## Do's and Don'ts

### Do's
- ✅ Use semantic styles based on meaning, not just color preference
- ✅ Provide fallbacks when styles are not available
- ✅ Use consistent output functions (prefer `fmt.Println`)
- ✅ Use Label style for all non-status headers and labels
- ✅ Test output with different themes to ensure readability

### Don'ts
- ❌ Don't use Success style for non-success related labels
- ❌ Don't mix stdout and stderr without clear reason
- ❌ Don't hardcode colors - always use the theme system
- ❌ Don't assume a style is available - always check for nil
- ❌ Don't use status styles (Success, Error, Warning) for structural elements

## Testing Your Output

When developing new features, test your output with different themes:

```bash
# Test with default theme
atmos your-command

# Test with a light theme
ATMOS_THEME=solarized-light atmos your-command

# Test with a high-contrast theme
ATMOS_THEME=github-dark atmos your-command

# Test with no color
atmos your-command --no-color
```

## Theme System Architecture

The theme system consists of several components:

1. **Theme Registry** (`pkg/ui/theme/registry.go`): Manages available themes
2. **Color Scheme** (`pkg/ui/theme/scheme.go`): Maps theme colors to semantic purposes
3. **Style Set** (`pkg/ui/theme/styles.go`): Pre-configured lipgloss styles
4. **Theme Converter** (`pkg/ui/theme/converter.go`): Converts themes to color schemes

### Adding New Styles

If you need to add a new style to the system:

1. Add the field to the appropriate struct in `pkg/ui/theme/styles.go`
2. Initialize it in the `GetStyles()` function
3. Add a helper function if commonly used
4. Document its usage in this guide

Example:
```go
// In StyleSet struct
type StyleSet struct {
    // ... existing fields ...
    YourNewStyle lipgloss.Style
}

// In GetStyles function
YourNewStyle: lipgloss.NewStyle().
    Foreground(lipgloss.Color(scheme.Primary)).
    Bold(true),
```

## Color Accessibility

When using the theme system:

1. **Always use semantic colors** - Don't reference specific colors like "blue" or "green"
2. **Test contrast** - Ensure text is readable on various backgrounds
3. **Provide alternatives** - Support `--no-color` flag for accessibility
4. **Use WCAG guidelines** - Aim for 4.5:1 contrast ratio for normal text

## References

- [Lipgloss Documentation](https://github.com/charmbracelet/lipgloss)
- [WCAG Contrast Guidelines](https://www.w3.org/WAI/WCAG21/Understanding/contrast-minimum.html)
