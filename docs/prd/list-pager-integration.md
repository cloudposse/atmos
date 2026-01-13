# List Commands Pager Integration

## Overview

Wire up the pager to list commands (`list stacks`, `list components`, etc.) so that when `ATMOS_PAGER=true` or `--pager` flag is set, the output is displayed in a scrollable TUI pager.

## Current State

- The `--pager` flag is defined globally in `pkg/flags/global_registry.go`
- The pager package (`pkg/pager/`) provides `PageCreator` interface with `Run(title, content)` method
- Describe commands (e.g., `describe component`) use the pager via:
  1. Check `atmosConfig.Settings.Terminal.IsPagerEnabled()`
  2. Call `pageCreator.Run(title, formattedContent)`
- List commands do NOT use the pager - they render directly via `renderer.Render()` which calls `output.Write()`

## Implementation Plan

### Step 1: Modify Renderer to Return Content (Instead of Writing)

**File:** `pkg/list/renderer/renderer.go`

Currently `Render()` writes directly to output. Add a new method that returns the formatted string:

```go
// RenderToString executes the pipeline and returns formatted output.
func (r *Renderer) RenderToString(data []map[string]any) (string, error) {
    // ... same pipeline steps 1-4 ...
    return formatted, nil
}
```

Keep `Render()` as a wrapper that calls `RenderToString()` then writes.

### Step 2: Add Pager Support to List Commands

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

### Step 6: Consider Factory Pattern (Optional)

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

## Testing

1. Unit test `RenderToString()` method
2. Test pager integration with mock `PageCreator`
3. Manual testing:
   - `ATMOS_PAGER=true atmos list components` - should show pager with navigation
   - `atmos list components --pager` - same behavior
   - `atmos list components` - no pager (default)
   - `atmos list components | head` - no pager (piped)

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
