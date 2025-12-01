# Help System Architecture PRD

## Executive Summary

This document defines the architecture and behavior of Atmos help system, distinguishing between interactive help commands, flag-based help output, and usage messages. It establishes clear patterns for colored output, pager usage, and environment variable handling.

## Problem Statement

### Background
CLI tools typically provide help in multiple forms:
- **Flag-based help** (`--help`, `-h`): Quick reference, non-interactive, machine-parseable
- **Interactive help** (`help` command): Enhanced UX, may use pagers or TUI
- **Usage messages**: Brief error guidance when commands are misused

### Current Challenges
1. **Conflation of help types**: Interactive `help` command and flag-based `--help` were treated identically
2. **Pager confusion**: `--help` was incorrectly using pager by default, breaking pipe-ability
3. **Color support inconsistency**: Different approaches for different environments (TTY vs non-TTY)
4. **Dependency complexity**: Boa library required /dev/tty access, causing issues in CI/screengrabs

## Design Goals

1. **Distinguish help types**: Clear separation between interactive and non-interactive help
2. **Preserve UNIX conventions**: `--help` output should be pipeable without pager interference
3. **Universal color support**: Colors work in terminals, CI, and screengrabs
4. **Simplicity**: Single rendering path, minimal dependencies
5. **Standard compliance**: Support standard color environment variables

## Technical Specification

### Help Types

#### 1. Flag-Based Help (`--help`, `-h`)
**Purpose**: Quick reference, designed for piping and scripting

**Characteristics**:
- **Output**: Directly to stdout (no buffering)
- **Pager**: NEVER used by default
- **Colors**: Enabled via environment variables when output is redirected
- **Interactivity**: None - static output only

**Usage**:
```bash
atmos --help                    # Root help
atmos terraform --help          # Subcommand help
atmos list vars --help          # Nested subcommand help

# Explicit pager override (rare)
atmos --help --pager            # Use pager for flag help
atmos --help --pager=less       # Use specific pager
```

**Implementation**:
- Detected via: `Contains(os.Args, "--help") || Contains(os.Args, "-h")`
- Renders using: `applyColoredHelpTemplate(command)` directly to stdout
- Pager: Only when `--pager` flag explicitly set

#### 2. Interactive Help (`help` command)
**Purpose**: Enhanced help experience for exploratory learning

**Characteristics**:
- **Output**: Buffered, formatted for pager
- **Pager**: Used by default (respects config/env)
- **Colors**: Always enabled in TTY
- **Interactivity**: May include enhanced TUI elements in future

**Usage**:
```bash
atmos help                      # Interactive help with pager
atmos help terraform            # Subcommand interactive help
atmos help --pager=false        # Disable pager explicitly
```

**Implementation**:
- Detected via: `Contains(os.Args, "help") && !(Contains(os.Args, "--help") || Contains(os.Args, "-h"))`
- Renders using: `applyColoredHelpTemplate(command)` to buffer
- Pager: Controlled by `ATMOS_PAGER`, `settings.terminal.pager`, or `--pager` flag

#### 3. Usage Messages
**Purpose**: Brief guidance when commands are misused

**Characteristics**:
- **Output**: Directly to stderr
- **Pager**: Never
- **Colors**: Only in TTY
- **Interactivity**: None

**Usage**:
```bash
atmos terraform                 # Missing required args → usage message
atmos invalid-command           # Unknown command → usage message
```

**Implementation**:
- Triggered by: Invalid arguments, missing required flags
- Renders using: `showUsageAndExit()` or `showFlagUsageAndExit()`
- Output: stderr (errors should go to stderr, not stdout)

### Color Rendering Architecture

#### Color Profile System
Atmos uses Charmbracelet's `colorprofile` library combined with `lipgloss` for universal color support.

**Key Components**:

1. **colorprofile.Writer**: Detects terminal color capabilities from environment
2. **lipgloss.Renderer**: Renders styled text using detected color profile
3. **Force Color Support**: Overrides auto-detection when needed

**Implementation** (`applyColoredHelpTemplate`):
```go
func applyColoredHelpTemplate(cmd *cobra.Command) {
    // Create colorprofile.Writer (detects from environment)
    w := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())

    // Force TrueColor when environment variables set
    forceColor := os.Getenv("ATMOS_FORCE_COLOR") != "" ||
                  os.Getenv("CLICOLOR_FORCE") == "1" ||
                  os.Getenv("FORCE_COLOR") != ""

    if forceColor {
        w.Profile = colorprofile.TrueColor
    }

    // Create lipgloss renderer with color profile
    renderer := lipgloss.NewRenderer(w)
    if forceColor {
        renderer.SetColorProfile(termenv.TrueColor)
    }

    // Define colors
    green := renderer.NewStyle().Foreground(lipgloss.Color("#00ff00"))
    white := renderer.NewStyle().Foreground(lipgloss.Color("#ffffff"))

    // Render help sections with colors...
}
```

#### Environment Variables

**Standard Color Variables** (in precedence order):
1. **`ATMOS_FORCE_COLOR`** - Atmos-specific, any non-empty value enables
2. **`CLICOLOR_FORCE=1`** - Standard CLI convention (de facto standard)
3. **`FORCE_COLOR`** - Node.js/web tooling convention
4. **`NO_COLOR`** - Disables colors (checked by colorprofile automatically)
5. **`TERM=dumb`** - Disables colors (checked by colorprofile automatically)

**Examples**:
```bash
# Enable colors for screengrabs (redirected output)
export ATMOS_FORCE_COLOR=true
atmos --help > help.ansi

# Use standard convention
export CLICOLOR_FORCE=1
atmos list stacks --help | aha > stacks.html

# Disable colors
export NO_COLOR=1
atmos --help > help.txt
```

### Pager Behavior

#### Pager Precedence (highest to lowest)
1. **`--pager` flag**: Explicit command-line override
2. **`ATMOS_PAGER` environment variable**: Session-specific setting
3. **`settings.terminal.pager` config**: Project/user default
4. **Default behavior**: Depends on help type (see Help Types above)

#### Pager Values
- **`--pager`** or **`--pager=true`**: Enable with default pager (less/more)
- **`--pager=false`**: Explicitly disable pager
- **`--pager=less`**: Use specific pager command
- **`ATMOS_PAGER=false`**: Disable via environment
- **`ATMOS_PAGER=bat`**: Use specific pager via environment

### Decision Matrix

| Invocation | Pager Default | Colors (TTY) | Colors (Redirect) | Output Destination |
|------------|---------------|--------------|-------------------|-------------------|
| `atmos --help` | ❌ No | ✅ Yes | ✅ Yes (with force) | stdout |
| `atmos help` | ✅ Yes* | ✅ Yes | ✅ Yes (with force) | stdout (via pager) |
| `atmos --help --pager` | ✅ Yes | ✅ Yes | ✅ Yes (with force) | stdout (via pager) |
| `atmos invalid` (usage) | ❌ No | ✅ Yes | ❌ No | stderr |

\* Subject to config/env overrides

## Implementation Details

### Key Files
- **`cmd/root.go`**: `SetHelpFunc()` implementation, help type detection
- **`cmd/root.go`**: `applyColoredHelpTemplate()` rendering function
- **`pkg/pager/pager.go`**: Pager logic with TTY checks
- **`pkg/config/load.go`**: Default pager configuration

### Removed Dependencies
- **`github.com/elewis787/boa`**: Replaced with direct colorprofile/lipgloss usage
  - **Reason**: Required /dev/tty access, causing CI/screengrab failures
  - **Benefit**: Simpler code, universal color support, single rendering path

### Testing Strategy

#### Manual Testing
```bash
# Test flag help (no pager)
atmos --help | head -5                 # Should show output immediately
atmos list vars --help | grep Flags    # Should be pipeable

# Test interactive help (with pager if configured)
atmos help                             # May use pager
atmos help terraform                   # May use pager

# Test color forcing
export ATMOS_FORCE_COLOR=true
atmos --help > help.ansi
xxd help.ansi | grep " 1b"             # Should show ESC codes

# Test pager override
atmos --help --pager                   # Should use pager
export ATMOS_PAGER=false
atmos help                             # Should NOT use pager
```

#### Screengrab Generation
```bash
# Set in demo/screengrabs/build-all.sh
export ATMOS_FORCE_COLOR=true
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
export ATMOS_PAGER=false

# Generate screengrabs
bash build-all.sh demo-stacks.txt

# Verify colors in HTML
grep "color:#00ff00" website/src/components/Screengrabs/*.html
```

## Design Rationale

### Why Not Use Pager for `--help` by Default?

**UNIX Convention**: The `--help` flag is designed for quick reference and piping:
```bash
atmos terraform --help | grep apply     # Common pattern
atmos list --help > docs/reference.txt  # Documentation generation
```

Using a pager by default would break these workflows.

**Precedent**: Standard tools follow this pattern:
- `git --help` → direct output
- `git help` → man page (pager)
- `docker --help` → direct output
- `docker help` → same as --help (Docker doesn't distinguish)

### Why Remove Boa?

**Problems with Boa**:
1. Required /dev/tty access (fails in CI, screengrabs, Docker)
2. Alternate screen mode not fully controllable
3. Added complexity with conditional logic
4. Two rendering paths to maintain

**Benefits of Removal**:
1. Single rendering path (simpler code)
2. Works everywhere (TTY, non-TTY, CI, screengrabs)
3. Direct control over color behavior
4. Fewer dependencies

### Why Support Multiple Color Environment Variables?

**Ecosystem Compatibility**:
- `CLICOLOR_FORCE=1`: Standard used by many CLI tools (gh, bat, etc.)
- `FORCE_COLOR`: Node.js/web tooling ecosystem standard
- `ATMOS_FORCE_COLOR`: Atmos-specific, more discoverable for users
- `NO_COLOR`: Universal standard for disabling colors

Supporting all variants ensures Atmos works well in different environments and CI systems.

## Future Enhancements

### Potential Improvements
1. **Rich interactive help**: Enhanced TUI for `atmos help` with navigation
2. **Man pages**: Generate traditional man pages for `atmos help` command
3. **Contextual examples**: Show examples relevant to current stack/component
4. **Search functionality**: Interactive search in help output
5. **Help caching**: Cache rendered help for faster display

### Backward Compatibility
- Existing `--help` behavior preserved
- Configuration keys remain unchanged
- Environment variables additive (don't break existing usage)

## References

- [Charmbracelet colorprofile](https://github.com/charmbracelet/colorprofile)
- [Charmbracelet lipgloss](https://github.com/charmbracelet/lipgloss)
- [CLICOLOR standard](https://bixense.com/clicolors/)
- [NO_COLOR standard](https://no-color.org/)
- [Cobra documentation](https://github.com/spf13/cobra/blob/master/user_guide.md)
