# List Commands Pager Integration PRD

## Executive Summary

This document describes the integration of the pager feature into Atmos list commands (`list stacks`, `list components`, etc.), providing users with a scrollable TUI for navigating large outputs. The integration leverages existing pager infrastructure from describe commands, ensuring consistent UX across the CLI.

## Problem Statement

### Current State

- The `--pager` flag is defined globally in `pkg/flags/global_registry.go`
- The pager package (`pkg/pager/`) provides `PageCreator` interface with `Run(title, content)` method
- Describe commands (e.g., `describe component`) use the pager via:
  1. Check `atmosConfig.Settings.Terminal.IsPagerEnabled()`
  2. Call `pageCreator.Run(title, formattedContent)`
- Previously, list commands rendered directly via `renderer.Render()` (which calls `output.Write()`); pager support is now being rolled out to list commands.

### Challenges

1. **Long output is unwieldy** - List commands can produce hundreds of lines that scroll past the terminal viewport
2. **No navigation** - Users cannot scroll back through output without terminal scroll history
3. **Inconsistent UX** - Describe commands support pager but list commands do not
4. **Pipeline integration** - List commands need to detect TTY vs pipe and behave appropriately

## Design Goals

1. **Consistent UX** - Provide the same pager experience for list commands as describe commands
2. **Graceful degradation** - Automatically disable pager when output is piped or in non-TTY environments
3. **Minimal changes** - Leverage existing pager infrastructure with minimal code additions
4. **Backwards compatible** - Default behavior unchanged; pager only activates when explicitly enabled

## Technical Specification

### Architecture

The integration follows a simple pattern:

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  List Command   │ ──▶ │    Renderer      │ ──▶ │  Pager/Direct   │
│  (components)   │     │  RenderToString  │     │    Output       │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │
                               ▼
                        Check IsPagerEnabled()
                               │
                    ┌──────────┴──────────┐
                    ▼                     ▼
             Pager enabled          Pager disabled
                    │                     │
                    ▼                     ▼
            pageCreator.Run()      renderer.Render()
```

### API Changes

#### New Method: `RenderToString`

**File:** `pkg/list/renderer/renderer.go`

```go
// RenderToString executes the pipeline and returns formatted output as a string.
// This enables the caller to pass the content to a pager or other consumers.
func (r *Renderer) RenderToString(data []map[string]any) (string, error) {
    // ... same pipeline steps as Render() ...
    return formatted, nil
}
```

The existing `Render()` method becomes a wrapper that calls `RenderToString()` then writes to output.

## Implementation Plan

### Step 1: Add RenderToString Method to Renderer

**File:** `pkg/list/renderer/renderer.go`

Add a new method that returns the formatted string instead of writing directly:

```go
// RenderToString executes the pipeline and returns formatted output.
func (r *Renderer) RenderToString(data []map[string]any) (string, error) {
    // ... same pipeline steps 1-4 ...
    return formatted, nil
}
```

Keep `Render()` as a wrapper that calls `RenderToString()` then writes.

### Step 2: Wire Pager Support to List Commands

**Files:**
- `cmd/list/components.go`
- `cmd/list/stacks.go`
- (Other list commands as needed)

Pattern to follow (from `describe_component.go`):

```go
import (
    "github.com/cloudposse/atmos/pkg/pager"
)

func renderComponents(atmosConfig *schema.AtmosConfiguration, opts *ComponentsOptions, components []map[string]any) error {
    // ... existing filter/column/sort setup ...

    // Create renderer
    r := renderer.New(filters, selector, sorters, outputFormat, "")

    // Check if pager is enabled
    if atmosConfig.Settings.Terminal.IsPagerEnabled() {
        content, err := r.RenderToString(components)
        if err != nil {
            return err
        }

        pageCreator := pager.NewWithAtmosConfig(true)
        if err := pageCreator.Run("List Components", content); err != nil {
            // Pager failed, fall back to direct output
            return r.Render(components)
        }
        return nil
    }

    // No pager - render directly
    return r.Render(components)
}
```

### Step 3: Pass AtmosConfig to Render Functions

The `atmosConfig` is already available in `listComponentsWithOptions()`. Ensure it's passed through to `renderComponents()` (it already is).

### Step 4: Handle TTY Detection

The pager package already handles TTY detection internally:
- If not a TTY, it falls back to direct print
- If content fits terminal, it prints directly without pager

No changes needed - this is automatic.

### Step 5: Update All List Commands

Apply the same pattern to:
- `cmd/list/stacks.go`
- `cmd/list/affected.go`
- `cmd/list/workflows.go`
- `cmd/list/vendor.go`
- `cmd/list/values.go`
- `cmd/list/instances.go`
- `cmd/list/settings.go`

### Step 6: Helper Function (Optional DRY Refactor)

For DRY code, consider creating a helper in `cmd/list/utils.go`:

```go
func renderWithPager(atmosConfig *schema.AtmosConfiguration, title string, r *renderer.Renderer, data []map[string]any) error {
    if atmosConfig.Settings.Terminal.IsPagerEnabled() {
        content, err := r.RenderToString(data)
        if err != nil {
            return err
        }

        pageCreator := pager.NewWithAtmosConfig(true)
        if err := pageCreator.Run(title, content); err != nil {
            return r.Render(data)
        }
        return nil
    }

    return r.Render(data)
}
```

## Files to Modify

1. `pkg/list/renderer/renderer.go` - Add `RenderToString()` method
2. `cmd/list/components.go` - Wire up pager
3. `cmd/list/stacks.go` - Wire up pager
4. `cmd/list/utils.go` - (Optional) Add `renderWithPager()` helper
5. Other list commands as needed

## Dependencies

- `pkg/pager` package (already exists)
- `atmosConfig.Settings.Terminal.IsPagerEnabled()` (already exists)
- Global `--pager` flag (already registered)

## Testing

### Unit Tests

1. Test `RenderToString()` method returns correct formatted output
2. Test pager integration with mock `PageCreator`
3. Test fallback behavior when pager fails

### Manual Testing

| Command | Expected Behavior |
|---------|-------------------|
| `ATMOS_PAGER=true atmos list components` | Pager with navigation |
| `atmos list components --pager` | Pager with navigation |
| `atmos list components` | Direct output (no pager) |
| `atmos list components \| head` | Direct output (piped, no pager) |

## Success Criteria

- [ ] All list commands support `--pager` flag
- [ ] `ATMOS_PAGER=true` environment variable works for all list commands
- [ ] Pager gracefully falls back to direct output when not TTY
- [ ] Content fits terminal → prints directly (no pager scroll)
- [ ] No behavior change when pager is disabled (default)
- [ ] All existing tests pass
- [ ] New unit tests for `RenderToString()` method

## References

- `pkg/pager/` package implementation
- `cmd/describe/describe_component.go` - Reference pager integration
- `pkg/list/renderer/renderer.go` - Current renderer implementation

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-01-20 | Initial PRD with proper format |
