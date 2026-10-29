# Data Model: `--filter` for `describe affected`

No persisted data. These are the in-memory entities the change touches.

## Affected component (existing: `schema.Affected`)

| Field | Role in this feature |
|-------|----------------------|
| `stack`, `component`, `component_type`, `component_path` | Identity, and the four matrix-entry fields |
| `settings` | Visible to the filter only with `--include-settings` |
| `dependents` | Nested `Dependent` list, pruned recursively by the filter |
| `deleted` | Deleted items are evaluated like any other item |

Unchanged: no new fields and no changes to JSON/YAML output shape.

## Dependent (existing: `schema.Dependent`)

Same identity fields plus `settings` and nested `dependents`. The filter is evaluated on each dependent at every depth.

## Matrix entry (existing: `matrix.Entry`)

`stack`, `component`, `component_path`, `component_type`. Unchanged.

## DescribeAffectedCmdArgs (existing, extended)

| New field | Type | Meaning |
|-----------|------|---------|
| `Filter` | `string` | YQ predicate from `--filter` / `ATMOS_DESCRIBE_AFFECTED_FILTER`. Empty means no filtering. |
| `FilterEnvVar` | `string` | Name of the environment variable that supplied `Filter`; empty when it came from the command line. Used for the upload rule. |

## Predicate (new, internal)

`func(item map[string]any) (bool, error)`, built from `Filter` through `filter.NewYQPredicateFilter(...).Matches`.

## Rules and state transitions

- Pruning is a pure function of the affected forest and the predicate:
  1. For each top-level item: evaluate. If it fails, drop it and its whole `dependents` subtree.
  2. If it passes, recurse into `dependents` with the same rule.
  3. No promotion of dependents of dropped items (differs from `--tags`/`--labels`).
- Order of operations: compute affected → resolve dependents → apply `--tags`/`--labels` → apply `--filter` → flatten (if set) → `--query` (non-matrix) or matrix conversion.
- `--filter` unset or empty: identity.
- Evaluation errors (invalid expression) abort with an error before any output.
