# Fix: Settings can't refer to locals anymore (1.205 regression)

**Date**: 2025-01-28

**GitHub Issue**: [#2032](https://github.com/cloudposse/atmos/issues/2032)

## Problem

After PR #1994, locals could reference settings but settings could no longer reference locals. This was a regression from version 1.204 where settings could reference locals but locals couldn't reference settings.

### Example

```yaml
# With atmos 1.205, this configuration had issues:
locals:
  stage: dev
  stage_from_setting: "{{ .settings.context.stage }}"  # Works in 1.205

settings:
  context:
    stage: dev
    stage_from_local: "{{ .locals.stage }}"  # BROKEN in 1.205, worked in 1.204

vars:
  stage: dev
  setting_referring_to_local: "{{ .settings.context.stage_from_local }}"  # Got "{{ .locals.stage }}" instead of "dev"
  local_referring_to_setting: "{{ .locals.stage_from_setting }}"          # Works in 1.205
```

**Expected behavior**: Bidirectional references between settings and locals should work:
- Settings can refer to locals, vars, and env
- Locals can refer to settings, vars, and env
- Vars can refer to settings, locals, and env

**Actual behavior (before fix)**: Settings templates that referenced locals remained unresolved, causing vars that referenced those settings to get the raw template string instead of the resolved value.

## Root Cause

The template processing order was:
1. Parse raw YAML
2. Resolve locals (with access to raw settings/vars/env)
3. Add raw settings/vars/env to context
4. Process templates in full manifest

The problem was that settings were added to the context with their raw template values (like `{{ .locals.stage }}`), not resolved values. When vars later tried to access `{{ .settings.context.stage_from_local }}`, they got the raw template string instead of the resolved value.

Go templates don't recursively process template strings that exist in data values, so the nested template was never expanded.

## Solution

Modified `extractAndAddLocalsToContext()` in `internal/exec/stack_processor_utils.go` to process templates in settings, vars, and env sections AFTER locals are resolved:

1. First, resolve locals (which can reference raw settings/vars/env)
2. Add resolved locals to context
3. Process templates in settings using the resolved locals context
4. Process templates in vars using the resolved locals AND processed settings context
5. Process templates in env using the resolved locals, processed settings, AND processed vars context
6. Add all processed sections to the final context

This ensures that:
- Locals can reference settings (resolved during locals processing with raw settings)
- Settings can reference locals (resolved by new template processing step)
- Vars can reference both locals and processed settings
- Env can reference locals, processed settings, and processed vars

### New Helper Function

Added `processTemplatesInSection()` that:
- Converts a section to YAML
- Checks if it contains template markers (`{{`)
- Processes templates using the provided context
- Parses the result back to a map

## Files Changed

- `internal/exec/stack_processor_utils.go`: Modified `extractAndAddLocalsToContext()` and added `processTemplatesInSection()` helper
- `internal/exec/stack_processor_utils_test.go`: Added `TestExtractAndAddLocalsToContext_BidirectionalReferences`

## Testing

Added `TestExtractAndAddLocalsToContext_BidirectionalReferences` with test cases for:
- Settings referencing locals
- Vars referencing settings that reference locals
- Full bidirectional references (the exact scenario from the issue)

## Usage

After the fix, bidirectional references work correctly:

```yaml
locals:
  stage: dev
  stage_from_setting: "{{ .settings.context.stage }}"  # Works: "dev"

settings:
  context:
    stage: dev
    stage_from_local: "{{ .locals.stage }}"  # Now works: "dev"

vars:
  stage: dev
  setting_referring_to_local: "{{ .settings.context.stage_from_local }}"  # Now works: "dev"
  local_referring_to_setting: "{{ .locals.stage_from_setting }}"          # Works: "dev"
```
