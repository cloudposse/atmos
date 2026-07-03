# PRD: YAML `!append` Function

## Overview

Add an `!append` YAML tag function that concatenates a list onto the inherited list during
stack merging, instead of replacing it. This gives users per-field control over list-merge
behavior without changing the global `list_merge_strategy`.

## Problem Statement

Atmos builds configuration by deep-merging layered imports and inheritance. By default, when
two layers define the same list, the later layer **replaces** the earlier one. There is no
way to say "keep the inherited items and add these" for a single field without one of two
blunt instruments:

1. Re-declaring the entire inherited list in the override (brittle — the base and override
   drift out of sync).
2. Switching the global `list_merge_strategy` to `append`, which changes merge behavior for
   **every** list in the configuration, not just the one you intended.

### Real-World Use Cases

- **Dependency management** — append additional `depends_on` items without restating base deps.
- **Security groups** — add environment-specific rules on top of a shared base.
- **IAM policies** — extend base policy statements.
- **Tags/labels** — add extra tags while preserving organizational defaults.
- **EKS node groups** — extend a base cluster with additional node pools.

## Proposed Solution

A YAML explicit tag, `!append`, applied to a sequence:

```yaml title="stacks/base.yaml"
components:
  terraform:
    eks:
      settings:
        depends_on:
          - vpc
          - iam-role
```

```yaml title="stacks/prod.yaml"
import:
  - base
components:
  terraform:
    eks:
      settings:
        depends_on: !append
          - rds
          - elasticache
        # Result: [vpc, iam-role, rds, elasticache]
```

### Behavior

- Applies to **sequences only**; on any other node kind it is a graceful no-op (the tag is
  cleared and the value decodes normally).
- Works **per-field** — it does not affect other lists.
- Operates **during merge** — base items first, appended items after (order preserved).
- Independent of the global `list_merge_strategy`: with `replace` (default) the tagged field
  appends while other fields replace; with `append`/`merge` the tagged field still appends
  exactly once (no duplication).

### Implementation Approach

`!append` is unusual among YAML functions because it influences the **merge**, not value
resolution. It is implemented as a parse-time wrapper plus a merge-time unwrap:

1. **Parse phase (wrap).** An `!append`-tagged sequence is rewritten into a reserved wrapper
   `map[string]any{ "__atmos_append__": [items...] }` (the `AppendTagMetadataKey` constant).
   This happens in **two** parsing paths, which must stay in sync:
   - **atmos.yaml** — `handleAppend` in `pkg/config/process_yaml_append.go`
     (`preprocessAtmosYamlFunc`).
   - **Stack manifests** — `rewriteAppendNode` in `pkg/utils/yaml_utils.go`
     (`processCustomTagsInner`). `hasCustomTags` also recognizes `!append` so the
     early-exit optimization does not skip `!append`-only subtrees. Custom tags inside the
     list items are still processed for later resolution.
2. **Merge phase (unwrap + append).** `pkg/merge` (`processAppendTags`) detects the wrapper
   via `ExtractAppendListValue` before each native deep-merge. Under the global append
   strategy it returns only the new items so the native append adds them once; otherwise it
   returns base+new concatenated so the override replaces with the appended list.

This keeps the default ("local" nested inheritance) semantics unchanged and isolates inputs
(the merge deep-copies values, so caller maps are never mutated).

## Why It's Worth Doing

- **Developer experience** — declarative "add to inherited list" without copy-paste drift.
- **Targeted** — per-field control instead of a global strategy switch.
- **Aligns with existing patterns** — complements `!unset` (remove an inherited key) as the
  other half of fine-grained inheritance control.

## Alternatives Considered

- **Global `list_merge_strategy: append`** — too coarse; changes every list.
- **Restating the full list in the override** — brittle and defeats inheritance.

## Success Metrics

- `!append` works identically in `atmos.yaml` and stack manifests.
- Default merge behavior for untagged lists is unchanged.
- No duplication when combined with the global `append` strategy.

## Documentation Requirements

- YAML function reference page: `/functions/yaml/append`.
- Listed in the YAML functions index.

## Status

Implemented. See `website/docs/functions/yaml/append.mdx`, `pkg/utils/yaml_func_append.go`,
`pkg/config/process_yaml_append.go`, `pkg/utils/yaml_utils.go` (`rewriteAppendNode`), and
`pkg/merge/merge.go` (`processAppendTags`).
