# PRD: `atmos describe affected --upload` Flag

## Overview

Update the existing `--upload` flag on `atmos describe affected` to produce a minimal payload optimized for Atmos Pro. This reduces payload size to stay within Inngest's 256KB event payload limit and Vercel's 4.5MB request body limit by stripping fields that aren't used in downstream processing.

## Background

### Problem Statement

1. GitHub Action runs `atmos describe affected` in customer infrastructure repos
2. Action POSTs the full output to Atmos Pro's `/api/v1/affected-stacks` endpoint
3. Atmos Pro forwards the payload to Inngest for async processing
4. **Problem:** Large repos with many stacks and deep dependency graphs exceed Inngest's 256KB event payload limit
5. When exceeded, Inngest returns a 500 error and the affected stacks event is lost
6. Additionally, Vercel has a 4.5MB request body limit that could be hit with extremely large payloads

### Current Payload Structure

Each stack in the `atmos describe affected` output contains:

```json
{
  "component": "vpc",
  "component_type": "terraform",
  "component_path": "components/terraform/vpc",
  "namespace": "ex1",
  "tenant": "plat",
  "environment": "use2",
  "stage": "dev",
  "stack": "plat-use2-dev",
  "stack_slug": "plat-use2-dev-vpc",
  "affected": "stack.vars",
  "dependents": [...],
  "included_in_dependents": false,
  "settings": {
    "depends_on": { ... },
    "github": { ... },
    "pro": { ... }
  }
}
```

### Analysis

Testing against realistic fixtures from the Atmos Pro codebase:

| Fixture | Original Size | With `--upload` | Reduction |
|---------|---------------|-----------------|-----------|
| affected-stacks.json | 328.4 KB | 85.1 KB | **74.1%** |
| affected-stacks-with-dependents.json | 4.8 KB | 1.4 KB | **70.7%** |

The larger fixture (328 KB) exceeds Inngest's 256KB event payload limit. After stripping, it's comfortably under at 85 KB.

### Why CLI-Side?

Payload stripping belongs in the CLI, not at the Atmos Pro API level. The CLI is the right place to decide what data to send.

Benefits of CLI-side stripping:
- Unused data never leaves the customer's GitHub Action
- Reduces network transfer to Atmos Pro
- Avoids hitting Vercel's 4.5MB body limit
- Single source of truth for what fields Atmos Pro needs
- Customers can inspect the minimal payload locally

### Why Strip After Construction?

The `schema.Affected` struct is built incrementally across multiple files (`describe_affected_components.go`, `describe_affected_deleted.go`, `describe_affected_utils_2.go`). There is no single DTO constructor — fields like `Settings`, `SpaceliftStack`, `ComponentPath`, and `StackSlug` are added at different stages in `appendToAffected()` and its callers.

An alternative approach would be to skip populating unused fields at the source, but that would require threading the `Upload` flag through `appendToAffected()` and all construction sites, adding conditionals throughout the already-spread-out build logic. Stripping at the end keeps the `--upload` concern isolated to a single function and avoids complicating the construction pipeline. The performance difference is negligible at these data sizes.

## Proposed Solution

Update the existing `--upload` flag on `atmos describe affected` to strip fields not required by Atmos Pro, producing a minimal payload optimized for upload.

### Usage

```bash
# Full output (current behavior)
atmos describe affected

# Minimal output for Atmos Pro upload
atmos describe affected --upload
```

### Output Format

With `--upload`, each stack contains only:

```json
{
  "component": "vpc",
  "stack": "plat-use2-dev",
  "included_in_dependents": false,
  "dependents": [...],
  "settings": {
    "pro": { ... }
  }
}
```

### Fields Analysis

#### Fields to Keep

| Field | Reason |
|-------|--------|
| `component` | Stack identification |
| `stack` | Stack identification |
| `included_in_dependents` | Used in filtering logic |
| `dependents` | Nested stack processing (recursively stripped) |
| `settings.pro` | Workflow dispatch configuration |

#### Fields to Remove

| Field | Reason |
|-------|--------|
| `settings.depends_on` | Dependency graph data; not used in downstream processing; largest contributor to size |
| `settings.github` | Not used by downstream handlers |
| `component_type` | Not used in downstream processing |
| `component_path` | Not used in downstream processing |
| `namespace` | Redundant (encoded in stack name) |
| `tenant` | Redundant (encoded in stack name) |
| `environment` | Redundant (encoded in stack name) |
| `stage` | Redundant (encoded in stack name) |
| `stack_slug` | Not used in downstream processing |
| `affected` | Not used in downstream processing |

### Implementation

#### Flag Definition

In `cmd/describe_affected.go`:

```go
describeAffectedCmd.PersistentFlags().Bool("upload", false,
    "Output minimal payload optimized for Atmos Pro upload")
```

#### Stripping Logic

```go
func stripForUpload(stack map[string]interface{}) map[string]interface{} {
    result := map[string]interface{}{
        "component":             stack["component"],
        "stack":                 stack["stack"],
        "included_in_dependents": stack["included_in_dependents"],
    }

    // Recursively strip dependents
    if dependents, ok := stack["dependents"].([]interface{}); ok {
        strippedDependents := make([]interface{}, len(dependents))
        for i, dep := range dependents {
            if depMap, ok := dep.(map[string]interface{}); ok {
                strippedDependents[i] = stripForUpload(depMap)
            }
        }
        result["dependents"] = strippedDependents
    } else {
        result["dependents"] = []interface{}{}
    }

    // Keep only settings.pro if present
    if settings, ok := stack["settings"].(map[string]interface{}); ok {
        if pro, ok := settings["pro"]; ok {
            result["settings"] = map[string]interface{}{"pro": pro}
        }
    }

    return result
}
```

#### Integration Point

Apply stripping after affected stacks are computed, before JSON serialization:

```go
if uploadFlag {
    for i, stack := range affectedStacks {
        affectedStacks[i] = stripForUpload(stack)
    }
}
```

## Requirements

### Functional Requirements

1. **FR1:** Add `--upload` flag to `atmos describe affected`
2. **FR2:** When `--upload` is set, output only fields required by Atmos Pro
3. **FR3:** Apply stripping recursively to nested `dependents`
4. **FR4:** Preserve all fields when `--upload` is not set (backward compatible)

### Non-Functional Requirements

1. **NFR1:** No performance regression for non-upload use cases
2. **NFR2:** Flag should work with all existing `describe affected` options
3. **NFR3:** Output should be valid JSON

## Testing

### Unit Tests

1. `TestStripForUpload_RemovesUnusedFields` - Verify correct fields are removed
2. `TestStripForUpload_PreservesRequiredFields` - Verify required fields are kept
3. `TestStripForUpload_RecursiveDependents` - Verify nested dependents are stripped
4. `TestStripForUpload_MissingSettingsPro` - Handle stacks without settings.pro
5. `TestStripForUpload_EmptyDependents` - Handle empty dependents array

### Integration Tests

1. Test `--upload` flag is recognized
2. Test output size is smaller with `--upload`
3. Test output structure matches expected format
4. Test compatibility with `--format json`

### Manual Testing

```bash
# Compare sizes
atmos describe affected | wc -c
atmos describe affected --upload | wc -c

# Verify structure
atmos describe affected --upload | jq '.[0] | keys'
# Should output: ["component", "dependents", "included_in_dependents", "settings", "stack"]
```

## Migration

### GitHub Action Update

The `atmos-affected-stacks` GitHub Action should be updated to use `--upload`:

```yaml
# Before
- run: atmos describe affected --format json > affected.json

# After
- run: atmos describe affected --upload --format json > affected.json
```

### Backward Compatibility

- Existing behavior unchanged when `--upload` is not specified
- No breaking changes to current integrations
- Action can be updated independently of CLI rollout

## Success Metrics

- Payload size reduction of 50-80% for typical repos
- Zero 500 errors from Inngest payload size limits
- No regression in Atmos Pro functionality

## Limitations

- Does not guarantee all payloads fit within Inngest's 256KB limit (extremely large repos may still exceed after stripping)
- Server-side blob storage handles arbitrarily large payloads (see Atmos Pro PRD)
- Future work: payload chunking or alternative transport for edge cases

## References

- Linear: DEV-3940
- Atmos Pro Apps PR: cloudposse-corp/apps#683 (private)
- Inngest event payload limit: 256KB (free), 3MB (paid)
- Vercel request body limit: 4.5MB
