# Fix: `list dependencies` renders templated `required` values

**Date:** 2026-09-09

## Summary

Scoped `list dependencies --process-templates` now defers templated dependency optionality until the selected component is rendered.

## Context

The lightweight closure-discovery pass parsed an unresolved `required` template as a boolean before Phase C rendering, causing valid manifests to fail.

## Changes

- Deferred unresolved `required` values only during structural graph discovery while preserving the conservative required default.
- Kept strict boolean parsing after Phase C rendering.
- Restored the manifest-schema fixture's `required` property and added embedded/fixture parity coverage.
- Documented templated optionality and optional-edge output markers.

```mermaid
flowchart TD
    A[Read the dependency] --> B{Is the dependency valid?}
    B -->|no| C[Stop and show a configuration error]
    B -->|yes| D[Find component dependencies<br/>Treat an unresolved required value as required for now]

    D --> E[Choose the affected components]
    E --> F[Render templates for those components]
    F --> G{Did required become true or false?}

    G -->|no| C
    G -->|yes| H{Is the target component available?}
    H -->|yes| I[Include the dependency]
    H -->|no| J{Is the dependency optional?}
    J -->|yes| K[Ignore this dependency]
    J -->|no| L[Stop and show that the target is unavailable]
```

## Validation

- `go test ./pkg/list/dependencies -run TestResolveScopedClosureRendersTemplatedRequiredBeforeValidation -count=1`
- `go test ./pkg/datafetcher -run TestManifestSchema_ComponentDependencyRequiredForms -count=1`

## Follow-ups

None.
